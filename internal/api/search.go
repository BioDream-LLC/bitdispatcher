package api

// Unified search (cad/find.go) and opening a result: both recorded in the ledger with the person, duty,
// agency, query and filters. The record searched is never changed.

import (
	"net/http"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

func (s *Server) searchRoutes() {
	s.handle("POST /api/search", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var q cad.FindQuery
		if !readJSON(w, r, &q) {
			return
		}
		q.Text = strings.TrimSpace(q.Text)
		if len(q.Text) < 2 {
			fail(w, 422, "search for at least two characters")
			return
		}
		if len(q.Text) > 200 {
			fail(w, 422, "a search is up to 200 characters")
			return
		}
		if se.Role == auth.Field {
			fail(w, 403, "search from the console")
			return
		}
		now := s.Eng.Now()
		var res cad.FindResults
		q.FileText = s.fileText
		s.Eng.Read(func(st *cad.State) { res = st.Find(q, s.readerOf(st, se), now) })
		s.Eng.Record(a, cad.EvSearched, map[string]any{"q": q.Text, "types": q.Types, "agency": q.Agency, "from": q.From, "to": q.To, "status": q.Status,
			"allAgencies": q.AllAgencies, "counts": res.Counts, "scope": res.Scope})
		writeJSON(w, 200, res)
	})
	s.handle("POST /api/search/opened", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Q, Group, Ref, Field string
		}
		if !readJSON(w, r, &b) {
			return
		}
		s.Eng.Record(a, cad.EvSearchOpened, map[string]any{"q": b.Q, "group": b.Group, "ref": b.Ref, "field": b.Field, "at": time.Now().UTC()})
		w.WriteHeader(http.StatusNoContent)
	})
}
