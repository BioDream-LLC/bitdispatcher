package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// Report drafts: a first narrative for an incident report, from what the incident's record says (the
// type, the place, the times, the units, the notes). It is a proposal for the writer to correct and finish:
// it goes into the report only when the writer takes it, and says it is a draft. Without the language model
// it is written plainly from the record; with the model on (Integrations), the model writes it from the
// same facts and is told to add nothing.

func (s *Server) draftRoutes() {
	s.handle("POST /api/incidents/{n}/draft-narrative", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if se.Role != auth.Field && !auth.AtLeast(se.Role, auth.Dispatcher) {
			fail(w, 403, "report writers draft narratives")
			return
		}
		facts, ok := s.incidentFacts(r.PathValue("n"))
		if !ok {
			fail(w, 404, "no incident")
			return
		}
		text, by := strings.Join(facts, "\n"), "record"
		if x := s.extra(); x.ModelOn && x.ModelURL != "" && x.ModelName != "" {
			if t, err := modelDraft(r.Context(), x, facts); err == nil && t != "" {
				text, by = t, "model"
			} else if err != nil {
				s.Log.Warn("report draft: the model failed; drafting from the record", "err", err)
			}
		}
		s.Eng.Record(a, cad.EvIntegration, map[string]any{"draftNarrative": r.PathValue("n"), "by": by})
		writeJSON(w, 200, map[string]any{"text": text, "by": by, "facts": facts})
	})
}

// incidentFacts is what the record says, in order, one line each.
func (s *Server) incidentFacts(n string) ([]string, bool) {
	var out []string
	found := false
	s.Eng.Read(func(st *cad.State) {
		in := st.Incidents[n]
		if in == nil {
			return
		}
		found = true
		loc := func(t time.Time) string { return t.Local().Format("15:04:05") }
		name := in.Type
		if ct := st.CallTypes[in.Type]; ct != nil {
			name = ct.Name
		}
		place := "no location"
		if in.Location != nil {
			place = firstNonEmptyStr(in.Location.Address, fmt.Sprintf("%.5f, %.5f", deref(in.Location.Lat), deref(in.Location.Lon)))
		}
		out = append(out, fmt.Sprintf("On %s at %s, %s received a %s call (incident %s, priority %d) at %s.",
			in.Created.Local().Format("Monday, January 2, 2006"), loc(in.Created), firstNonEmptyStr(st.Settings.Agency, "the centre"), name, in.Number, in.Priority, place))
		if in.Protocol != nil && in.Protocol.Determinant != nil {
			out = append(out, fmt.Sprintf("The call was coded %s: %s.", in.Protocol.Determinant.Code, in.Protocol.Determinant.Text))
		}
		for _, as := range in.Units {
			parts := []string{fmt.Sprintf("%s was dispatched at %s", as.Unit, loc(as.Dispatched))}
			if !as.Enroute.IsZero() {
				parts = append(parts, "en route at "+loc(as.Enroute))
			}
			if !as.OnScene.IsZero() {
				parts = append(parts, "on scene at "+loc(as.OnScene))
			}
			if !as.Transport.IsZero() {
				parts = append(parts, "transporting at "+loc(as.Transport))
			}
			if !as.AtDest.IsZero() {
				parts = append(parts, "at the destination at "+loc(as.AtDest))
			}
			if !as.Cleared.IsZero() {
				parts = append(parts, "cleared at "+loc(as.Cleared))
			}
			out = append(out, strings.Join(parts, ", ")+".")
		}
		for _, p := range in.Persons {
			if !p.Removed && p.Name != "" {
				out = append(out, fmt.Sprintf("Person: %s (%s).", p.Name, firstNonEmptyStr(p.Role, "involved")))
			}
		}
		for _, v := range in.Vehicles {
			if !v.Removed {
				out = append(out, "Vehicle: "+strings.Join(strings.Fields(strings.Join([]string{v.Color, v.Make, v.Model, v.Plate, v.State, "(" + firstNonEmptyStr(v.Role, "involved") + ")"}, " ")), " ")+".")
			}
		}
		for _, e := range in.Timeline {
			if slices.Contains([]string{"note", "priority-note"}, e.Kind) && !strings.HasPrefix(e.Text, "Determinant ") {
				out = append(out, fmt.Sprintf("%s (%s): %s", loc(e.At), e.Actor, e.Text))
			}
		}
		if in.Status == cad.Closed {
			out = append(out, fmt.Sprintf("The incident was closed at %s: %s.", loc(in.Closed), firstNonEmptyStr(in.Disposition, "no disposition")))
		}
	})
	return out, found
}

func deref(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// modelDraft asks the model for a narrative from the facts only.
func modelDraft(ctx context.Context, x Extra, facts []string) (string, error) {
	body, _ := json.Marshal(map[string]any{"model": x.ModelName, "temperature": 0, "messages": []map[string]string{
		{"role": "system", "content": "You draft the narrative of a public-safety incident report from the facts given. Use only those facts; " +
			"add nothing, guess nothing, and keep every time, name, number and address exactly. Write plain past-tense paragraphs in the third person. " +
			"Where something a report needs is not in the facts, write [to be completed] instead of inventing it."},
		{"role": "user", "content": strings.Join(facts, "\n")}}})
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(x.ModelURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if x.ModelKey != "" {
		req.Header.Set("Authorization", "Bearer "+x.ModelKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("model: %s", resp.Status)
	}
	var v struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &v); err != nil || len(v.Choices) == 0 {
		return "", errors.New("model: no answer")
	}
	return strings.TrimSpace(v.Choices[0].Message.Content), nil
}
