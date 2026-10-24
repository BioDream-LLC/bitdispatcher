package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// scopeRefused answers a request about an incident or unit outside the agencies the person may see, and
// records it. A person who may work every agency is never refused here, whatever their board shows.
func (s *Server) scopeRefused(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) bool {
	inc, unit := "", ""
	switch {
	case strings.Contains(r.Pattern, "/api/incidents/{n}"):
		inc = r.PathValue("n")
	case strings.Contains(r.Pattern, "/api/units/{id}"):
		unit = r.PathValue("id")
	default:
		return false
	}
	ok := true
	s.Eng.Read(func(st *cad.State) {
		sc := st.AccessScope(se.Username, se.Console, se.Duty)
		if sc == nil {
			return
		}
		if in := st.Incidents[inc]; inc != "" && in != nil {
			ok = st.IncidentIn(sc, in)
		}
		if u := st.Units[unit]; unit != "" && u != nil {
			ok = st.UnitIn(sc, u)
		}
	})
	if ok {
		return false
	}
	s.Eng.Record(a, cad.EvAccessRefused, map[string]string{"path": r.URL.Path, "why": "another agency's"})
	fail(w, http.StatusForbidden, "this is not one of your agencies' incidents or units")
	return true
}

// outOfScope names the first incident or unit in a command (or the incident it is given for) that the person
// may not see, or is empty. Words are split on spaces and the field-command delimiters.
func (s *Server) outOfScope(se *auth.Session, incident, line string) string {
	what := ""
	s.Eng.Read(func(st *cad.State) {
		sc := st.AccessScope(se.Username, se.Console, se.Duty)
		if sc == nil {
			return
		}
		if in := st.Incidents[incident]; in != nil && !st.IncidentIn(sc, in) {
			what = incident
			return
		}
		delim := st.Settings.Commands.Delim
		for _, w := range strings.FieldsFunc(strings.ToUpper(line), func(r rune) bool {
			return r == ' ' || r == '\t' || r == ',' || r == '/' || r == '.' || (delim != "" && strings.ContainsRune(delim, r))
		}) {
			if in := st.Incidents[w]; in != nil && !st.IncidentIn(sc, in) {
				what = w
				return
			}
			if u := st.Units[w]; u != nil && !st.UnitIn(sc, u) {
				what = w
				return
			}
		}
	})
	return what
}

// noticeFor is a change notice as the person may see it: about another agency's incident or unit, only that
// something changed (the board they reload is scoped).
func (s *Server) noticeFor(se *auth.Session, n cad.Notice) cad.Notice {
	if s.noticeIn(se, n) {
		return n
	}
	return cad.Notice{Seq: n.Seq, Type: "other"}
}

// noticeIn reports whether a change notice is about something the person may see on their board.
func (s *Server) noticeIn(se *auth.Session, n cad.Notice) bool {
	if n.Inc == "" && n.Unit == "" {
		return true
	}
	ok := true
	s.Eng.Read(func(st *cad.State) {
		sc := st.BoardScope(se.Username, se.Console, se.Duty)
		if sc == nil {
			return
		}
		if in := st.Incidents[n.Inc]; n.Inc != "" && in != nil && !st.IncidentIn(sc, in) {
			ok = false
		}
		if u := st.Units[n.Unit]; n.Unit != "" && u != nil && !st.UnitIn(sc, u) {
			ok = false
		}
	})
	return ok
}

// markOutWarnings tells supervisors, once each, of a unit on a mark-out longer than its type allows.
func (s *Server) markOutWarnings(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		if over, err := s.Eng.WarnMarkOuts(); err == nil {
			for _, x := range over {
				s.Eng.SendComm(cad.SystemActor, cad.Comm{Kind: "admin", Priority: "high", Agency: x.Agency,
					Subject: fmt.Sprintf("%s over its %s limit", x.Unit, x.Name),
					Body: fmt.Sprintf("%s has been on %s (%s) for %d minutes since %s; the agency allows %d. Check on the unit.",
						x.Unit, x.Name, x.Code, x.Minutes, x.Since.Local().Format("15:04"), x.Max),
					To: []string{cad.ToRole + "supervisor"}})
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
