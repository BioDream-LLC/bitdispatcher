package cad

import (
	"slices"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// EvAutoDispatch records what auto-dispatch did or why it left the incident.
const EvAutoDispatch = "incident.autodispatch"

// autoEntry records it; the caller does not hold e.mu.
func (e *Engine) autoEntry(number, text string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.st.Incidents[number]; !ok {
		return problem("incident", "no incident %s", number)
	}
	_, err := e.write(AutoActor, EvAutoDispatch, refs{Incident: number}, map[string]string{"text": text})
	return err
}

func (s *State) applyAuto(e ledger.Event) (bool, error) {
	if e.Type != EvAutoDispatch {
		return false, nil
	}
	v, _ := decode[map[string]string](e)
	if in, ok := s.Incidents[e.Incident]; ok {
		in.add(e, "auto-dispatch", v["text"])
	}
	return true, nil
}

// Auto-dispatch: for call types the agency marks (priority 3 to 5 only), the recommended units are sent
// when no dispatcher has sent any within the wait. Only a complete recommendation without warnings is
// sent; anything else waits for a person. It is logged as auto-dispatch, and the dispatcher can change it.

// AutoActor is who auto-dispatch acts as.
var AutoActor = Actor{User: "auto-dispatch", Console: "SYSTEM"}

// AutoDispatchWait is how long a call waits for a dispatcher.
func (s *State) AutoDispatchWait() time.Duration {
	if s.Settings.AutoDispatchSeconds > 0 {
		return time.Duration(s.Settings.AutoDispatchSeconds) * time.Second
	}
	return time.Minute
}

// AutoDispatchDue are the incidents waiting longer than the wait for auto-dispatch.
func (e *Engine) AutoDispatchDue(now time.Time) []string {
	var out []string
	e.Read(func(s *State) {
		wait := s.AutoDispatchWait()
		for _, in := range s.Incidents {
			ct := s.CallTypes[in.Type]
			if ct == nil || !ct.AutoDispatch || in.Status != Pending || len(in.Units) > 0 || in.Priority < 3 || now.Sub(in.Created) < wait {
				continue
			}
			if slices.ContainsFunc(in.Timeline, func(t Entry) bool { return t.Kind == "auto-dispatch" }) {
				continue // tried once: a person has it now
			}
			out = append(out, in.Number)
		}
	})
	slices.Sort(out)
	return out
}

// AutoDispatch sends the recommended units to an incident, or says why it leaves it for a dispatcher.
// Either way it is on the timeline, once.
func (e *Engine) AutoDispatch(number string) (sent []string, why string, err error) {
	rec, err := e.Recommend(number)
	if err != nil {
		return nil, "", err
	}
	switch {
	case len(rec.Warnings) > 0:
		why = strings.Join(rec.Warnings, "; ")
	case len(rec.Units) == 0:
		why = "no unit to recommend"
	default:
		for _, l := range rec.Lines {
			if l.Short > 0 {
				why = "the recommendation is short of " + l.Capability
			}
		}
	}
	if why != "" {
		return nil, why, e.autoEntry(number, "not sent automatically: "+why+" (a dispatcher sends it)")
	}
	if err := e.autoEntry(number, "sent automatically: "+strings.Join(rec.Units, " ")+" (no dispatcher sent units in time; the call type is set to auto-dispatch)"); err != nil {
		return nil, "", err
	}
	return rec.Units, "", e.DispatchWhy(AutoActor, number, rec.Units, rec.Units, "", rec.Plan)
}
