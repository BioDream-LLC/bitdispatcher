package api

// Records: an incident's review views (opened and exported views are recorded), and records requests
// (FOIA) from assembly to an approved, redacted release.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/foia"
)

// mayRecords is whether a session may work records: records staff, supervisors and administrators.
func mayRecords(se *auth.Session) bool {
	return se.Duty == auth.DutyRecords || auth.AtLeast(se.Role, auth.Supervisor)
}

func (s *Server) recordsRoutes() {
	guard := func(h handler) handler {
		return func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
			if !mayRecords(se) {
				fail(w, 403, "records work needs the records duty, a supervisor or an administrator")
				return
			}
			h(w, r, se, a)
		}
	}
	// A view of an incident. Opening one is recorded; so is exporting it.
	s.handle("GET /api/review/{n}", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		view := firstNonEmpty(r.URL.Query().Get("view"), "summary")
		if (view == "audit" || view == "detailed") && !mayRecords(se) && !auth.AtLeast(se.Role, auth.Dispatcher) {
			fail(w, 403, "the detailed and audit views need a dispatcher, records staff or a supervisor")
			return
		}
		doc, err := s.Eng.Review(r.PathValue("n"), view)
		if err != nil {
			writeErr(w, err)
			return
		}
		ev := cad.EvRecordsViewed
		if r.URL.Query().Get("export") != "" {
			ev = cad.EvReviewExported
		}
		s.Eng.Record(a, ev, map[string]string{"incident": doc.Incident, "view": view, "as": r.URL.Query().Get("export")})
		if r.URL.Query().Get("export") == "text" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.txt"`, doc.Incident, view))
			io.WriteString(w, doc.Text())
			return
		}
		writeJSON(w, 200, doc)
	})

	// Disclosure rules: the agency's, kept by administrators.
	s.handle("GET /api/admin/foia/rules", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var rules []foia.Rule
		s.Eng.Read(func(st *cad.State) { rules = st.FOIARuleList() })
		writeJSON(w, 200, map[string]any{"items": rules, "categories": foia.Categories(), "samples": foia.SampleRules()})
	})
	s.handle("PUT /api/admin/foia/rules/{id}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var rule foia.Rule
		if !readJSON(w, r, &rule) {
			return
		}
		rule.ID = r.PathValue("id")
		if err := s.Eng.SaveFOIARule(a, rule); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(204)
	})
	s.handle("DELETE /api/admin/foia/rules/{id}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if err := s.Eng.RemoveFOIARule(a, r.PathValue("id")); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(204)
	})
	s.handle("POST /api/admin/foia/try", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Text  string      `json:"text"`
			Rules []foia.Rule `json:"rules"` // empty: the agency's
		}
		if !readJSON(w, r, &b) {
			return
		}
		if b.Rules == nil {
			s.Eng.Read(func(st *cad.State) { b.Rules = st.FOIARuleList() })
		}
		props, errs := foia.Propose(b.Text, b.Rules, foia.Context{RecordType: "incident", At: time.Now()})
		msgs := []string{}
		for _, e := range errs {
			msgs = append(msgs, e.Error())
		}
		if props == nil {
			props = []foia.Proposal{}
		}
		writeJSON(w, 200, map[string]any{"proposals": props, "errors": msgs})
	})

	// Requests.
	s.handle("GET /api/foia/requests", auth.Viewer, guard(func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out := []*cad.FOIARequest{}
		s.Eng.Read(func(st *cad.State) {
			for _, q := range st.FOIA {
				cp := *q
				// The list carries no record text: open a request for its items.
				cp.Items = nil
				out = append(out, &cp)
			}
		})
		sortBy(out, func(q *cad.FOIARequest) string { return q.ID })
		slices.Reverse(out)
		writeJSON(w, 200, out)
	}))
	s.handle("GET /api/foia/requests/{id}", auth.Viewer, guard(func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var q *cad.FOIARequest
		s.Eng.Read(func(st *cad.State) {
			if x, ok := st.FOIA[r.PathValue("id")]; ok {
				cp := *x
				q = &cp
			}
		})
		if q == nil {
			fail(w, 404, "no request "+r.PathValue("id"))
			return
		}
		writeJSON(w, 200, q)
	}))
	saveReq := guard(func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			cad.FOIARequest
			Note string `json:"note"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		b.ID = r.PathValue("id")
		id, err := s.Eng.SaveFOIARequest(a, b.FOIARequest, b.Note)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("POST /api/foia/requests", auth.Viewer, saveReq)
	s.handle("PUT /api/foia/requests/{id}", auth.Viewer, saveReq)
	s.handle("POST /api/foia/requests/{id}/assemble", auth.Viewer, guard(func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		id := r.PathValue("id")
		var q cad.FOIARequest
		found := false
		s.Eng.Read(func(st *cad.State) {
			if x, ok := st.FOIA[id]; ok {
				q, found = *x, true
			}
		})
		if !found {
			fail(w, 404, "no request "+id)
			return
		}
		texts := map[string]string{}
		for _, n := range q.Incidents {
			doc, err := s.Eng.Review(n, q.View)
			if err != nil {
				writeErr(w, err)
				return
			}
			texts[n] = doc.Text()
		}
		extra := map[string][]foia.Proposal{}
		if x := s.extra(); x.FOIAModel && x.ModelURL != "" {
			for n, t := range texts {
				extra[n] = s.modelProposals(r.Context(), t)
			}
		}
		if err := s.Eng.AssembleFOIA(a, id, texts, extra); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(204)
	}))
	s.handle("POST /api/foia/requests/{id}/decide", auth.Viewer, guard(func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Incident  string          `json:"incident"`
			Decisions []foia.Decision `json:"decisions"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		if err := s.Eng.DecideFOIA(a, r.PathValue("id"), b.Incident, b.Decisions); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(204)
	}))
	s.handle("POST /api/foia/requests/{id}/approve", auth.Viewer, guard(func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		leaks, err := s.Eng.ApproveFOIA(a, r.PathValue("id"))
		if err != nil {
			if len(leaks) > 0 {
				writeJSON(w, 422, map[string]any{"message": err.Error(), "leaks": leaks})
				return
			}
			writeErr(w, err)
			return
		}
		w.WriteHeader(204)
	}))
	s.handle("GET /api/foia/requests/{id}/released/{n}", auth.Viewer, guard(func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var text, hash string
		s.Eng.Read(func(st *cad.State) {
			if q, ok := st.FOIA[r.PathValue("id")]; ok {
				for _, it := range q.Items {
					if it.Incident == r.PathValue("n") {
						text, hash = it.Released, it.Hash
					}
				}
			}
		})
		if text == "" {
			fail(w, 404, "not released")
			return
		}
		s.Eng.Record(a, cad.EvReviewExported, map[string]string{"incident": r.PathValue("n"), "view": "foia", "request": r.PathValue("id"), "hash": hash})
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.txt"`, r.PathValue("id"), r.PathValue("n")))
		w.Header().Set("X-Content-SHA256", hash)
		io.WriteString(w, text)
	}))
}

// modelProposals asks the agency's language model (when an administrator has allowed it for records
// requests) for passages that may be exempt. They are proposals only, marked as the model's, for review.
func (s *Server) modelProposals(ctx context.Context, text string) []foia.Proposal {
	x := s.extra()
	prompt := "You help a 911 centre's records staff. List passages in the record below that may be exempt from public " +
		"release (personal identifiers, medical details, juveniles, victims' identities, investigative details). Answer only JSON: " +
		`[{"text":"exact passage","why":"reason"}]. Copy each passage exactly. Record:` + "\n\n" + text
	body, _ := json.Marshal(map[string]any{"model": firstNonEmpty(x.ModelName, "gpt-4o-mini"), "temperature": 0,
		"messages": []map[string]string{{"role": "user", "content": prompt}}})
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(x.ModelURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	if x.ModelKey != "" {
		req.Header.Set("Authorization", "Bearer "+x.ModelKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.Log.Warn("foia: language model", "err", err)
		return nil
	}
	defer resp.Body.Close()
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil || len(out.Choices) == 0 {
		return nil
	}
	c := out.Choices[0].Message.Content
	if i := strings.Index(c, "["); i >= 0 {
		c = c[i:]
	}
	if i := strings.LastIndex(c, "]"); i >= 0 {
		c = c[:i+1]
	}
	var items []struct{ Text, Why string }
	json.Unmarshal([]byte(c), &items)
	var props []foia.Proposal
	for _, it := range items {
		if it.Text == "" {
			continue
		}
		// Only passages that are really in the record: the model never adds text.
		if i := strings.Index(text, it.Text); i >= 0 {
			props = append(props, foia.Proposal{ID: fmt.Sprintf("m%x", i*31+len(it.Text)), Start: i, End: i + len(it.Text), Text: it.Text, Rule: "model",
				Basis: "for the reviewer to name", Explain: "Suggested by the language model: " + it.Why, Confidence: "review", Source: "model"})
		}
	}
	return props
}
