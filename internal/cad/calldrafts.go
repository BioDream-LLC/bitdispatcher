package cad

// Working calls: a call-taker (or dispatcher, or supervisor) may keep a few calls in progress at once,
// each an independent draft in its own tab, and move between them without losing anything. What is typed
// is kept on the server with its owner (not in the ledger, which records only each draft's life): opened,
// moved to another person (a transfer, or a supervisor recovering one left behind), submitted as an
// incident, or deliberately discarded. A draft is recovered by its owner after an interrupted session.
// The agency sets whether people may keep several and how many, by duty.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvDraftOpened    = "calldraft.opened"    // a working call was started
	EvDraftMoved     = "calldraft.moved"     // given to another person (transfer or recovery)
	EvDraftClosed    = "calldraft.closed"    // submitted as an incident or discarded
	EvDraftRecovered = "calldraft.recovered" // picked up again by its owner after an interruption
)

// MultiCall is the agency's rule for working calls.
type MultiCall struct {
	Off bool `json:"off,omitempty"` // one working call at a time
	// Max working calls by duty (calltaker, police, fire, ems, dispatcher, supervisor) and for trainees;
	// a duty not listed has DefaultMax.
	Max        map[string]int `json:"max,omitempty"`
	TraineeMax int            `json:"traineeMax,omitempty"`
	DefaultMax int            `json:"defaultMax,omitempty"`
	// AbandonMinutes: a draft untouched this long while its owner is signed out is offered to supervisors
	// to recover (0: 30).
	AbandonMinutes int `json:"abandonMinutes,omitempty"`
}

// CallDraft is a working call's life (its contents are kept by the server beside the ledger).
type CallDraft struct {
	ID       string    `json:"id"`
	Owner    string    `json:"owner"`
	Duty     string    `json:"duty"`
	Console  string    `json:"console,omitempty"`
	Agency   string    `json:"agency,omitempty"`
	Call     string    `json:"call,omitempty"` // the 911 call being taken
	Opened   time.Time `json:"opened"`
	State    string    `json:"state"` // open, submitted, discarded
	Incident string    `json:"incident,omitempty"`
	Closed   time.Time `json:"closed,omitzero"`
	By       string    `json:"by,omitempty"` // who closed or last moved it
	Why      string    `json:"why,omitempty"`
	Moves    int       `json:"moves,omitempty"`
}

// MaxDrafts is how many working calls a person may keep for a duty.
func (s *State) MaxDrafts(user, duty string) int {
	m := s.Settings.MultiCall
	if m.Off {
		return 1
	}
	if s.StaffOf(user).Trainee {
		return max(1, m.TraineeMax)
	}
	if n, ok := m.Max[duty]; ok && n > 0 {
		return min(n, 9)
	}
	if m.DefaultMax > 0 {
		return min(m.DefaultMax, 9)
	}
	switch duty {
	case "calltaker":
		return 3
	case "supervisor":
		return 4
	}
	return 2
}

// DraftsOf are a person's open working calls, oldest first.
func (s *State) DraftsOf(user string) []*CallDraft {
	var out []*CallDraft
	for _, d := range s.CallDrafts {
		if d.State == "open" && strings.EqualFold(d.Owner, user) {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return draftLess(out[i], out[j]) })
	return out
}

func draftLess(a, b *CallDraft) bool {
	if !a.Opened.Equal(b.Opened) {
		return a.Opened.Before(b.Opened)
	}
	if len(a.ID) != len(b.ID) {
		return len(a.ID) < len(b.ID)
	}
	return a.ID < b.ID
}

// OpenDraft starts a working call for a person, within the agency's limit.
func (e *Engine) OpenDraft(a Actor, call string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	open := e.st.DraftsOf(a.User)
	if n := e.st.MaxDrafts(a.User, a.Duty); len(open) >= n {
		return "", problem("drafts", "you have %d working calls, the most the agency allows for this duty: submit or close one first", n)
	}
	if call != "" {
		if e.st.Calls[call] == nil {
			return "", problem("call", "no call %s", call)
		}
		for _, d := range e.st.CallDrafts {
			if d.State == "open" && d.Call == call {
				return "", problem("call", "that call is already being taken in %s's working call %s", d.Owner, d.ID)
			}
		}
	}
	id := fmt.Sprintf("W%s%d", e.sitePrefix, e.st.draftSeq+1)
	_, err := e.write(a, EvDraftOpened, refs{Call: call}, CallDraft{ID: id, Owner: strings.ToLower(a.User), Duty: a.Duty, Console: a.Console, Agency: a.Agency, Call: call})
	return id, err
}

// CloseDraft ends a working call: submitted as an incident, or discarded (a populated draft needs a reason).
func (e *Engine) CloseDraft(a Actor, id, how, incident, why string, populated bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	d := e.st.CallDrafts[id]
	if d == nil || d.State != "open" {
		return problem("id", "no open working call %s", id)
	}
	if !strings.EqualFold(d.Owner, a.User) && a.Role != "supervisor" && a.Role != "admin" {
		return problem("id", "that working call is %s's", d.Owner)
	}
	switch how {
	case "submitted":
		if e.st.Incidents[incident] == nil {
			return problem("incident", "no incident %s", incident)
		}
	case "discarded":
		if populated && strings.TrimSpace(why) == "" {
			return problem("why", "say why a working call with information in it is discarded")
		}
	default:
		return problem("how", "submitted or discarded")
	}
	_, err := e.write(a, EvDraftClosed, refs{Incident: incident, Call: d.Call}, map[string]any{"id": id, "how": how, "incident": incident, "why": strings.TrimSpace(why), "populated": populated})
	return err
}

// MoveDraft gives a working call to another person: a transfer by its owner, or a supervisor recovering
// one (who may move only drafts of agencies they work).
func (e *Engine) MoveDraft(a Actor, id, to, why string) error {
	to = strings.ToLower(strings.TrimSpace(to))
	e.mu.Lock()
	defer e.mu.Unlock()
	d := e.st.CallDrafts[id]
	if d == nil || d.State != "open" {
		return problem("id", "no open working call %s", id)
	}
	sup := a.Role == "supervisor" || a.Role == "admin"
	if !strings.EqualFold(d.Owner, a.User) && !sup {
		return problem("id", "that working call is %s's", d.Owner)
	}
	if sup && d.Agency != "" && !e.st.MayWork(a.User, d.Agency) {
		return problem("id", "that working call is %s's", d.Agency)
	}
	if to == "" || to == d.Owner {
		return problem("to", "say who takes it")
	}
	if strings.TrimSpace(why) == "" {
		return problem("why", "say why it moves")
	}
	if n := e.st.MaxDrafts(to, d.Duty); len(e.st.DraftsOf(to)) >= n {
		return problem("to", "%s already has %d working calls", to, n)
	}
	_, err := e.write(a, EvDraftMoved, refs{Call: d.Call}, map[string]string{"id": id, "to": to, "why": strings.TrimSpace(why)})
	return err
}

// RecoveredDraft records that the owner picked a working call up again at a console after an interruption.
func (e *Engine) RecoveredDraft(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	d := e.st.CallDrafts[id]
	if d == nil || d.State != "open" || !strings.EqualFold(d.Owner, a.User) {
		return problem("id", "no open working call %s of yours", id)
	}
	if d.Console == a.Console {
		return nil
	}
	_, err := e.write(a, EvDraftRecovered, refs{Call: d.Call}, map[string]string{"id": id, "console": a.Console})
	return err
}

func (s *State) applyDrafts(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvDraftOpened:
		d, err := decode[CallDraft](e)
		if err != nil {
			return true, err
		}
		d.Opened, d.State = e.At, "open"
		s.draftSeq++
		s.CallDrafts[d.ID] = &d
	case EvDraftClosed:
		v, err := decode[map[string]any](e)
		if err != nil {
			return true, err
		}
		id, _ := v["id"].(string)
		if d := s.CallDrafts[id]; d != nil {
			d.State, _ = v["how"].(string)
			d.Incident, _ = v["incident"].(string)
			d.Why, _ = v["why"].(string)
			d.Closed, d.By = e.At, e.Actor
		}
		s.trimDrafts(e.At)
	case EvDraftMoved:
		v, err := decode[map[string]string](e)
		if err != nil {
			return true, err
		}
		if d := s.CallDrafts[v["id"]]; d != nil {
			d.Owner, d.By, d.Why = v["to"], e.Actor, v["why"]
			d.Moves++
		}
	case EvDraftRecovered:
		v, err := decode[map[string]string](e)
		if err != nil {
			return true, err
		}
		if d := s.CallDrafts[v["id"]]; d != nil {
			d.Console = v["console"]
		}
	default:
		return false, nil
	}
	return true, nil
}

// trimDrafts forgets closed drafts after a day (the ledger keeps them).
func (s *State) trimDrafts(now time.Time) {
	for id, d := range s.CallDrafts {
		if d.State != "open" && now.Sub(d.Closed) > 24*time.Hour {
			delete(s.CallDrafts, id)
		}
	}
}

// OpenDrafts are every open working call, for supervisors.
func (s *State) OpenDrafts() []*CallDraft {
	var out []*CallDraft
	for _, d := range s.CallDrafts {
		if d.State == "open" {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return draftLess(out[i], out[j]) })
	return out
}
