package cad

import (
	"slices"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// Scope is the agencies whose incidents and units a person sees on the shared board. A nil Scope is the whole
// board: the centre has no agencies, or the person may work every agency and has not narrowed the board.
type Scope map[string]bool

// Board scopes a person can choose (WorkCoverage.Scope).
const (
	ScopeAll      = ""         // everything the person may see
	ScopeCoverage = "coverage" // only the agencies covered and watched now
)

// AccessScope is what a person may see at all: the agencies they may work, and those an administrator gave
// them or the workstation to watch. It is what the server enforces on incidents and units.
func (s *State) AccessScope(username, console, duty string) Scope {
	if s.AllAgencies(username) {
		return nil
	}
	sc := Scope{}
	p := s.StaffOf(username)
	for _, x := range p.Agencies {
		sc[x] = true
	}
	for _, x := range s.grantedWatch(username, console) {
		sc[x] = true
	}
	return sc
}

// BoardScope is what the board shows a person: what they may see, narrowed to their coverage when they ask.
func (s *State) BoardScope(username, console, duty string) Scope {
	if len(s.Agencies) == 0 {
		return nil
	}
	c := s.CoverageOf(username, console, duty)
	if c.Scope != ScopeCoverage {
		return s.AccessScope(username, console, duty)
	}
	sc := Scope{}
	for _, x := range append(append([]string{}, c.Covered...), c.Monitored...) {
		sc[x] = true
	}
	return sc
}

// grantedWatch is the agencies an administrator lets a person watch beyond those they work: in their
// profile, or at the workstation they are signed in at.
func (s *State) grantedWatch(username, console string) []string {
	out := append([]string{}, s.StaffOf(username).Monitor...)
	if ws := s.Workstations[console]; console != "" && ws != nil {
		out = append(out, ws.Monitor...)
	}
	return out
}

// MayWatch reports whether a person may watch an agency: one they may work, or one given to them to watch.
func (s *State) MayWatch(username, console, code string) bool {
	return s.MayWork(username, code) || slices.Contains(s.grantedWatch(username, console), code)
}

func (sc Scope) has(code string) bool { return sc == nil || code == "" || sc[code] }

// IncidentIn reports whether an incident is on a scoped board: an incident with no agency, or whose primary
// or assisting agency, or the home agency of a unit on it, is in the scope.
func (s *State) IncidentIn(sc Scope, in *Incident) bool {
	if sc == nil || in == nil {
		return true
	}
	if sc.has(in.Agency) {
		return true
	}
	for _, x := range in.Assisting {
		if sc[x] {
			return true
		}
	}
	for _, as := range in.Units {
		if u := s.Units[as.Unit]; u != nil && u.Agency != "" && sc[u.Agency] {
			return true
		}
	}
	return false
}

// UnitIn reports whether a unit is on a scoped board: its agency is in the scope, it has none, or it is on an
// incident that is.
func (s *State) UnitIn(sc Scope, u *Unit) bool {
	if sc == nil || u == nil || sc.has(u.Agency) {
		return true
	}
	return u.Incident != "" && s.IncidentIn(sc, s.Incidents[u.Incident])
}

// Overdue mark-outs: a unit out longer than its mark-out type allows is told to supervisors once.
const EvMarkOutOverdue = "unit.markout_overdue"

// OverdueMarkOut is a unit on a mark-out longer than the agency allows.
type OverdueMarkOut struct {
	Unit    string    `json:"unit"`
	Agency  string    `json:"agency,omitempty"`
	Code    string    `json:"code"`
	Name    string    `json:"name"`
	Since   time.Time `json:"since"`
	Max     int       `json:"maxMinutes"`
	Minutes int       `json:"minutes"`
}

// MarkOutOverdue is how long past its limit a unit's mark-out is (0: not over, or no limit), and the type.
func (s *State) MarkOutOverdue(u *Unit, now time.Time) (int, *MarkOutType) {
	if u == nil || u.Status != MarkedOut || u.MarkOut == nil {
		return 0, nil
	}
	for i := range s.Settings.MarkOuts {
		t := &s.Settings.MarkOuts[i]
		if !strings.EqualFold(t.Code, u.MarkOut.Code) || t.MaxMinutes <= 0 {
			continue
		}
		if over := int(now.Sub(u.StatusAt).Minutes()) - t.MaxMinutes; over >= 0 {
			return over + t.MaxMinutes, t
		}
		return 0, t
	}
	return 0, nil
}

// OverdueMarkOuts is every unit out longer than its mark-out allows, longest first.
func (s *State) OverdueMarkOuts(now time.Time) []OverdueMarkOut {
	var out []OverdueMarkOut
	for _, u := range s.Units {
		if m, t := s.MarkOutOverdue(u, now); m > 0 {
			out = append(out, OverdueMarkOut{Unit: u.ID, Agency: u.Agency, Code: t.Code, Name: t.Name, Since: u.StatusAt, Max: t.MaxMinutes, Minutes: m})
		}
	}
	slices.SortFunc(out, func(a, b OverdueMarkOut) int { return a.Since.Compare(b.Since) })
	return out
}

// WarnMarkOuts records once each mark-out that went past its limit, and returns those newly over.
func (e *Engine) WarnMarkOuts() ([]OverdueMarkOut, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []OverdueMarkOut
	for _, x := range e.st.OverdueMarkOuts(e.now()) {
		k := "markout|" + x.Unit + "|" + x.Since.UTC().Format(time.RFC3339Nano)
		if e.st.reminded[k] {
			continue
		}
		if _, err := e.write(SystemActor, EvMarkOutOverdue, refs{Unit: x.Unit}, map[string]any{"key": k, "unit": x.Unit, "code": x.Code,
			"since": x.Since, "maxMinutes": x.Max, "minutes": x.Minutes}); err != nil {
			return out, err
		}
		out = append(out, x)
	}
	return out, nil
}

func (s *State) applyScope(e ledger.Event) (bool, error) {
	if e.Type != EvMarkOutOverdue {
		return false, nil
	}
	v, err := decode[map[string]any](e)
	if err != nil {
		return true, err
	}
	if k, ok := v["key"].(string); ok {
		s.reminded[k] = true
	}
	return true, nil
}
