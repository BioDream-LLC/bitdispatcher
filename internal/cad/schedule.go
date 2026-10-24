package cad

// Scheduled calls and special events: work known ahead of time. A scheduled call (a planned transport, a
// funeral escort, a business check) becomes an incident by itself at its time, with its units sent if they
// are available. A special event (a fair, a parade, a game) becomes a standby incident for its hours and
// is listed on every console while it runs, so dispatchers know what is happening. Either may repeat
// daily, on weekdays or weekly.

import (
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

const (
	EvScheduledSaved   = "scheduled.saved"
	EvScheduledRemoved = "scheduled.removed"
	EvScheduledStarted = "scheduled.started" // an occurrence became an incident
)

// Scheduled is a call or special event known ahead of time.
type Scheduled struct {
	ID       string          `json:"id"`
	Kind     string          `json:"kind"` // call or event
	Title    string          `json:"title"`
	Type     string          `json:"type"` // the call type of its incident
	Priority int             `json:"priority,omitempty"`
	Location *ng911.Location `json:"location,omitempty"`
	Start    time.Time       `json:"start"`
	Minutes  int             `json:"minutes,omitempty"` // how long an event lasts
	Lead     int             `json:"lead,omitempty"`    // minutes before the start the incident is made
	Units    []string        `json:"units,omitempty"`   // sent at the start, when available
	Notes    string          `json:"notes,omitempty"`
	Repeat   string          `json:"repeat,omitempty"` // "", daily, weekdays, weekly
	Until    time.Time       `json:"until,omitzero"`   // the last day it repeats
	Off      bool            `json:"off,omitempty"`
	// Last is the most recent occurrence made an incident, and Incident that incident.
	Last     time.Time `json:"last,omitzero"`
	Incident string    `json:"incident,omitempty"`
	By       string    `json:"by,omitempty"`
}

// Scheduled kinds and repeats.
const (
	SchedCall  = "call"
	SchedEvent = "event"
)

type schedStarted struct {
	ID         string    `json:"id"`
	Occurrence time.Time `json:"occurrence"`
	Incident   string    `json:"incident"`
	Sent       []string  `json:"sent,omitempty"`
	NotSent    []string  `json:"notSent,omitempty"`
}

// SaveScheduled adds or changes a scheduled call or event.
func (e *Engine) SaveScheduled(a Actor, s Scheduled) (string, error) {
	s.Title = strings.TrimSpace(s.Title)
	if s.Kind == "" {
		s.Kind = SchedCall
	}
	switch {
	case s.Kind != SchedCall && s.Kind != SchedEvent:
		return "", problem("kind", "a scheduled item is a call or an event")
	case s.Title == "":
		return "", problem("title", "give it a title")
	case s.Start.IsZero():
		return "", problem("start", "give the date and time it starts")
	case s.Kind == SchedEvent && (s.Minutes < 15 || s.Minutes > 14*24*60):
		return "", problem("minutes", "an event lasts 15 minutes to 14 days")
	case s.Lead < 0 || s.Lead > 24*60:
		return "", problem("lead", "make the incident up to 24 hours ahead")
	case s.Priority < 0 || s.Priority > 5:
		return "", problem("priority", "priority is 1 to 5, or 0 for the call type's")
	}
	switch s.Repeat {
	case "", "daily", "weekdays", "weekly":
	default:
		return "", problem("repeat", "it repeats daily, on weekdays or weekly, or not at all")
	}
	s.Units = dedupe(s.Units)
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.CallTypes[s.Type] == nil {
		return "", problem("type", "choose the call type of its incident")
	}
	for i, u := range s.Units {
		s.Units[i] = strings.ToUpper(u)
		if e.st.Units[s.Units[i]] == nil {
			return "", problem("units", "no unit %s", u)
		}
	}
	if old := e.st.Scheduled[s.ID]; old != nil {
		s.Last, s.Incident, s.By = old.Last, old.Incident, old.By
	} else {
		if s.ID != "" {
			return "", problem("id", "no scheduled item %s", s.ID)
		}
		s.ID = e.newID("S", func(id string) bool { return e.st.Scheduled[id] != nil })
		s.By = a.User
	}
	_, err := e.write(a, EvScheduledSaved, refs{}, s)
	return s.ID, err
}

// RemoveScheduled removes a scheduled item (incidents it already made stay).
func (e *Engine) RemoveScheduled(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.Scheduled[id] == nil {
		return problem("id", "no scheduled item %s", id)
	}
	_, err := e.write(a, EvScheduledRemoved, refs{}, idData{id})
	return err
}

// Occurrences are the times an item happens from 'from' up to 'to'.
func (s *Scheduled) Occurrences(from, to time.Time) []time.Time {
	var out []time.Time
	t := s.Start
	for i := 0; i < 5000 && !t.After(to); i++ {
		if !s.Until.IsZero() && t.After(s.Until.Add(24*time.Hour)) {
			break
		}
		if !t.Before(from) && (s.Repeat != "weekdays" || (t.Weekday() != time.Saturday && t.Weekday() != time.Sunday)) {
			out = append(out, t)
		}
		switch s.Repeat {
		case "daily", "weekdays":
			t = t.AddDate(0, 0, 1)
		case "weekly":
			t = t.AddDate(0, 0, 7)
		default:
			return out
		}
	}
	return out
}

// Next is the next occurrence not yet made an incident, or zero.
func (s *Scheduled) Next(now time.Time) time.Time {
	from := s.Start
	if !s.Last.IsZero() {
		from = s.Last.Add(time.Minute)
	}
	if occ := s.Occurrences(from, now.AddDate(1, 0, 0)); len(occ) > 0 {
		return occ[0]
	}
	return time.Time{}
}

// Upcoming are the items with an occurrence between now and the end of the window, soonest first, with
// the events running now.
func (s *State) Upcoming(now time.Time, window time.Duration) []UpcomingItem {
	var out []UpcomingItem
	for _, x := range s.Scheduled {
		if x.Off {
			continue
		}
		look := now.Add(-time.Duration(x.Minutes) * time.Minute)
		for _, t := range x.Occurrences(look, now.Add(window)) {
			end := t.Add(time.Duration(x.Minutes) * time.Minute)
			if x.Kind == SchedCall && t.Before(now.Add(-time.Hour)) {
				continue
			}
			if x.Kind == SchedEvent && !end.After(now) {
				continue
			}
			out = append(out, UpcomingItem{Item: x, At: t, End: end, Running: x.Kind == SchedEvent && !t.After(now) && end.After(now)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

// UpcomingItem is one occurrence of a scheduled item.
type UpcomingItem struct {
	Item    *Scheduled `json:"item"`
	At      time.Time  `json:"at"`
	End     time.Time  `json:"end"`
	Running bool       `json:"running,omitempty"`
}

// RunSchedule makes the incidents of scheduled items whose time (less their lead) has come, and sends
// their units that are available. It is called every few seconds; each occurrence is made once. An
// occurrence missed by more than an hour (the system was down) is skipped and said so.
func (e *Engine) RunSchedule() []string {
	now := e.now()
	type due struct {
		id  string
		occ time.Time
	}
	var todo []due
	e.Read(func(st *State) {
		for _, x := range st.Scheduled {
			if x.Off {
				continue
			}
			if n := x.Next(now); !n.IsZero() && !n.Add(-time.Duration(x.Lead)*time.Minute).After(now) {
				todo = append(todo, due{x.ID, n})
			}
		}
	})
	sort.Slice(todo, func(i, j int) bool { return todo[i].occ.Before(todo[j].occ) })
	var made []string
	for _, d := range todo {
		if n, err := e.startScheduled(d.id, d.occ, now); err == nil && n != "" {
			made = append(made, n)
		}
	}
	return made
}

func (e *Engine) startScheduled(id string, occ, now time.Time) (string, error) {
	var x Scheduled
	ok := false
	e.Read(func(st *State) {
		if p := st.Scheduled[id]; p != nil {
			x, ok = *p, true
		}
	})
	if !ok {
		return "", nil
	}
	a := Actor{User: "schedule", Role: "system"}
	end := occ.Add(time.Duration(x.Minutes) * time.Minute)
	stale := now.Sub(occ) > time.Hour && (x.Kind == SchedCall || !end.After(now))
	if stale {
		e.mu.Lock()
		_, err := e.write(a, EvScheduledStarted, refs{}, schedStarted{ID: id, Occurrence: occ})
		e.mu.Unlock()
		return "", err
	}
	narr := "Scheduled " + map[string]string{SchedCall: "call", SchedEvent: "event"}[x.Kind] + ": " + x.Title + " at " + occ.Local().Format("Mon 2 Jan 15:04")
	if x.Kind == SchedEvent {
		narr += " until " + end.Local().Format("15:04")
	}
	if x.Notes != "" {
		narr += ". " + x.Notes
	}
	var loc *ng911.Location
	if x.Location != nil {
		l := *x.Location // its own copy: making the incident fills in fields
		loc = &l
	}
	number, err := e.CreateIncident(a, NewIncident{Type: x.Type, Priority: x.Priority, Location: loc, Narrative: narr})
	if err != nil {
		return "", err
	}
	var sent, notSent []string
	e.Read(func(st *State) {
		for _, u := range x.Units {
			if un := st.Units[u]; un != nil && un.Status == Available {
				sent = append(sent, u)
			} else if un != nil {
				notSent = append(notSent, u+" ("+StatusLabel(un.Status)+")")
			}
		}
	})
	if len(sent) > 0 {
		if err := e.DispatchWhy(a, number, sent, sent, "scheduled", ""); err != nil {
			notSent = append(notSent, err.Error())
			sent = nil
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err = e.write(a, EvScheduledStarted, refs{Incident: number}, schedStarted{ID: id, Occurrence: occ, Incident: number, Sent: sent, NotSent: notSent})
	return number, err
}

func (s *State) applySchedule(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvScheduledSaved:
		x, err := decode[Scheduled](e)
		if err != nil {
			return true, err
		}
		s.Scheduled[x.ID] = &x
	case EvScheduledRemoved:
		v, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		delete(s.Scheduled, v.ID)
	case EvScheduledStarted:
		v, err := decode[schedStarted](e)
		if err != nil {
			return true, err
		}
		x := s.Scheduled[v.ID]
		if x == nil {
			return true, nil
		}
		x.Last = v.Occurrence
		if v.Incident != "" {
			x.Incident = v.Incident
		}
		if in := s.Incidents[v.Incident]; in != nil {
			text := "Made from the schedule (" + x.ID + ")"
			if len(v.Sent) > 0 {
				text += "; sent " + strings.Join(v.Sent, " ")
			}
			if len(v.NotSent) > 0 {
				text += "; not sent: " + strings.Join(v.NotSent, ", ")
			}
			in.add(e, "scheduled", text)
		}
	default:
		return false, nil
	}
	return true, nil
}

// ScheduleList is every scheduled item, by its next time.
func (s *State) ScheduleList(now time.Time) []*Scheduled {
	out := make([]*Scheduled, 0, len(s.Scheduled))
	for _, x := range s.Scheduled {
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Next(now), out[j].Next(now)
		if a.IsZero() != b.IsZero() {
			return !a.IsZero()
		}
		if !a.Equal(b) {
			return a.Before(b)
		}
		return out[i].ID < out[j].ID
	})
	return out
}
