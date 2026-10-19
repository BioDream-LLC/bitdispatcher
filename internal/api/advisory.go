package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/advisory"
	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

func (s *Server) advisoryRoutes() {
	s.handle("GET /api/advisory", auth.Viewer, s.advisoryList)
	s.handle("POST /api/advisory/{id}/decide", auth.Dispatcher, s.advisoryDecide)
	s.handle("GET /api/admin/advisory", auth.Supervisor, s.advisoryAdmin)
	s.handle("PUT /api/admin/advisory", auth.Admin, s.advisorySave)
	s.handle("POST /api/admin/advisory/test", auth.Supervisor, s.advisoryTest)
	s.handle("POST /api/admin/advisory/try", auth.Supervisor, s.advisoryTry)
}

// advisoryTry runs one script rule once on a real sample (the newest open incident, its recommendation,
// or the newest report) unless the editor sends its own input, and shows what it returned and logged.
func (s *Server) advisoryTry(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		Rule  advisory.Rule `json:"rule"`
		Input any           `json:"input"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	if b.Rule.Kind != advisory.KindScript {
		fail(w, http.StatusBadRequest, "only a script rule can be tried")
		return
	}
	input, about := b.Input, "your input"
	if input == nil {
		s.Eng.Read(func(st *cad.State) {
			var newest *cad.Incident
			for _, in := range st.Incidents {
				if in.Status != cad.Closed && (newest == nil || in.Created.After(newest.Created)) {
					newest = in
				}
			}
			switch b.Rule.Hook {
			case "report":
				var rp *cad.RMSReport
				for _, x := range st.Reports {
					if rp == nil || x.Updated.After(rp.Updated) {
						rp = x
					}
				}
				if rp != nil {
					input, about = *rp, "report "+rp.Number
				}
			default:
				if newest != nil {
					in := advisory.IncidentInput(st, newest, s.Eng.Now())
					input, about = in, "incident "+newest.Number
					if b.Rule.Hook == "recommend" {
						input = advisory.RecommendInput{Incident: in, Lines: []advisory.RecommendLine{}, Units: []string{}}
					}
				}
			}
		})
	}
	if input == nil {
		fail(w, http.StatusConflict, "nothing to try it on: no open incident or report; give an input")
		return
	}
	if b.Rule.Hook == "recommend" {
		if ri, ok := input.(advisory.RecommendInput); ok {
			if rec, err := s.Eng.Recommend(ri.Incident.Number); err == nil {
				for _, l := range rec.Lines {
					rl := advisory.RecommendLine{Capability: l.Capability, Count: l.Count, Candidates: []map[string]any{}}
					for _, c := range append(append([]cad.Candidate{}, l.Units...), l.Next...) {
						rl.Candidates = append(rl.Candidates, map[string]any{"unit": c.Unit, "type": c.Type, "seconds": c.Seconds, "metres": c.Metres})
					}
					ri.Lines = append(ri.Lines, rl)
				}
				ri.Units = rec.Units
				input = ri
			}
		}
	}
	res, err := advisory.TryScript(r.Context(), b.Rule, input)
	out := map[string]any{"on": about, "input": input, "value": res.Value, "log": res.Log, "ms": float64(res.Took.Microseconds()) / 1000}
	if err != nil {
		out["error"] = err.Error()
	}
	writeJSON(w, 200, out)
}

func (s *Server) advisoryList(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	shadow := auth.AtLeast(se.Role, auth.Supervisor)
	writeJSON(w, 200, map[string]any{"status": s.Advisory.Status(), "suggestions": s.Advisory.Visible(shadow)})
}

func (s *Server) advisoryDecide(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		Accept bool   `json:"accept"`
		Run    bool   `json:"run"`
		Reason string `json:"reason"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	d, res, err := s.Advisory.Decide(a, r.PathValue("id"), b.Accept, b.Run, b.Reason)
	if errors.Is(err, advisory.ErrGone) {
		fail(w, http.StatusGone, err.Error())
		return
	}
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"decision": d, "result": res})
}

func (s *Server) advisoryAdmin(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	c, mode, version := s.Advisory.Codebook()
	var hist []cad.CodebookVersion
	s.Eng.Read(func(st *cad.State) { hist = append(hist, st.Advisory.History...) })
	writeJSON(w, 200, map[string]any{"codebook": c, "mode": mode, "version": version, "history": hist, "default": advisory.Default(),
		"decisions": s.Advisory.Decisions(100), "status": s.Advisory.Status(), "shadow": s.Advisory.Visible(true)})
}

func (s *Server) advisorySave(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		Mode     string            `json:"mode"`
		Codebook advisory.Codebook `json:"codebook"`
		Note     string            `json:"note"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	if err := b.Codebook.Check(); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	raw, _ := json.Marshal(b.Codebook)
	if err := s.Eng.SaveCodebook(a, cad.CodebookData{Mode: b.Mode, Rules: raw, Note: b.Note}); err != nil {
		writeErr(w, err)
		return
	}
	s.Advisory.Wake()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) advisoryTest(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		Codebook advisory.Codebook `json:"codebook"`
		Days     int               `json:"days"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	if err := b.Codebook.Check(); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if b.Days <= 0 || b.Days > 90 {
		b.Days = 7
	}
	now := s.Eng.Now()
	res, err := s.Advisory.Test(b.Codebook, now.Add(-time.Duration(b.Days)*24*time.Hour), now)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, res)
}
