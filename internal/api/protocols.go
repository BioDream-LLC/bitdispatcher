package api

import (
	"net/http"
	"slices"
	"strings"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// Call-taking protocols: the agency's own question sets (Admin › Protocols), worked on the call-taker's
// screen, and determinants reported by licensed protocol software through a hook (docs/PROTOCOLS.md).

func (s *Server) protocolRoutes() {
	s.handle("GET /api/protocols", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var out []*cad.Protocol
		s.Eng.Read(func(st *cad.State) {
			for _, p := range st.ProtocolsFor(r.URL.Query().Get("type")) {
				c := *p
				out = append(out, &c)
			}
		})
		if out == nil {
			out = []*cad.Protocol{}
		}
		writeJSON(w, 200, out)
	})
	s.handle("POST /api/incidents/{n}/protocol", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct{ Protocol, Question, Answer string }
		if !readJSON(w, r, &b) {
			return
		}
		if err := s.Eng.AnswerProtocol(a, r.PathValue("n"), b.Protocol, b.Question, b.Answer); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("GET /api/admin/protocols", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out := []*cad.Protocol{}
		s.Eng.Read(func(st *cad.State) {
			for _, p := range st.Protocols {
				c := *p
				out = append(out, &c)
			}
		})
		slices.SortFunc(out, func(a, b *cad.Protocol) int { return strings.Compare(a.Name, b.Name) })
		writeJSON(w, 200, out)
	})
	s.handle("PUT /api/admin/protocols", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var p cad.Protocol
		if !readJSON(w, r, &p) {
			return
		}
		if err := s.Eng.SaveProtocol(a, p); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("DELETE /api/admin/protocols/{id}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if err := s.Eng.RemoveProtocol(a, r.PathValue("id")); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("GET /api/admin/protocols/example", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		writeJSON(w, 200, cad.ExampleProtocol())
	})
	// Protocol software (ProQA and others) reports the determinant for an incident, or for the call it is on.
	s.mux.HandleFunc("POST /api/hooks/determinant", s.hook(func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			cad.Determinant
			Incident string `json:"incident"`
			Call     string `json:"call"`
			System   string `json:"system"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		if b.Incident == "" && b.Call != "" {
			s.Eng.Read(func(st *cad.State) {
				if c := st.Calls[b.Call]; c != nil {
					b.Incident = c.Incident
				}
			})
		}
		if b.Incident == "" {
			fail(w, 404, "no incident for that call yet")
			return
		}
		sys := strings.TrimSpace(b.System)
		if sys == "" {
			sys = "protocol software"
		}
		if err := s.Eng.SetDeterminant(cad.Actor{User: "protocol:" + sys}, b.Incident, sys, b.Determinant); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
