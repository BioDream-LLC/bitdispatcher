package cad

// A unit's status history, read back from the ledger: every status it was in over a period, how long,
// on which incident and who changed it, with the totals a supervisor asks for (time committed, incidents
// handled), and its crew changes, status checks and move-ups on the same line.

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// StatusSpan is one stretch of a unit in one status.
type StatusSpan struct {
	From     time.Time `json:"from"`
	To       time.Time `json:"to"` // the end of the period when still in it
	Status   string    `json:"status"`
	Incident string    `json:"incident,omitempty"`
	MarkOut  string    `json:"markOut,omitempty"`
	Reason   string    `json:"reason,omitempty"`
	By       string    `json:"by,omitempty"`
	Seconds  float64   `json:"seconds"`
	Open     bool      `json:"open,omitempty"` // still in it at the end of the period
}

// UnitMark is something else that happened to the unit: a status check, crew change or move-up.
type UnitMark struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"` // check, crew, cover, emergency
	Text string    `json:"text"`
	By   string    `json:"by,omitempty"`
}

// UnitHistory is a unit's statuses over a period.
type UnitHistory struct {
	Unit      string             `json:"unit"`
	From      time.Time          `json:"from"`
	To        time.Time          `json:"to"`
	Spans     []StatusSpan       `json:"spans"`
	Marks     []UnitMark         `json:"marks"`
	Totals    map[string]float64 `json:"totals"` // seconds in each status
	Incidents []string           `json:"incidents"`
	Committed float64            `json:"committed"` // seconds on incidents
	Known     float64            `json:"known"`     // seconds whose status is known (from its first event)
}

// HistoryOf builds a unit's status history between from and to from ledger events (in ledger order).
func HistoryOf(events []ledger.Event, unit string, from, to time.Time) UnitHistory {
	h := UnitHistory{Unit: unit, From: from, To: to, Spans: []StatusSpan{}, Marks: []UnitMark{}, Totals: map[string]float64{}, Incidents: []string{}}
	var cur *StatusSpan
	seen := map[string]bool{}
	closeAt := func(t time.Time) {
		if cur == nil {
			return
		}
		cur.To = t
		if cur.To.After(from) {
			sp := *cur
			if sp.From.Before(from) {
				sp.From = from
			}
			sp.Seconds = sp.To.Sub(sp.From).Seconds()
			if sp.Seconds > 0 {
				h.Spans = append(h.Spans, sp)
			}
		}
		cur = nil
	}
	for _, e := range events {
		if e.Unit != unit || e.At.After(to) {
			continue
		}
		switch e.Type {
		case EvUnitStatus:
			var v statusData
			if json.Unmarshal(e.Data, &v) != nil {
				continue
			}
			closeAt(e.At)
			cur = &StatusSpan{From: e.At, Status: v.Status, Incident: v.Incident, MarkOut: v.MarkOut, Reason: v.Reason, By: e.Actor}
			if v.Incident != "" && !e.At.Before(from) && !seen[v.Incident] {
				seen[v.Incident] = true
				h.Incidents = append(h.Incidents, v.Incident)
			}
		case EvUnitSaved:
			if cur == nil { // a new unit starts out of service
				cur = &StatusSpan{From: e.At, Status: OutOfService, By: e.Actor}
			}
		case EvUnitChecked, EvUnitCrew, EvUnitCover, EvEmergency:
			if e.At.Before(from) {
				continue
			}
			m := UnitMark{At: e.At, By: e.Actor}
			switch e.Type {
			case EvUnitChecked:
				var v checkData
				json.Unmarshal(e.Data, &v)
				m.Kind, m.Text = "check", strings.TrimSpace("Status check: all right "+v.Note)
			case EvUnitCrew:
				var v crewData
				json.Unmarshal(e.Data, &v)
				names := make([]string, len(v.Crew))
				for i, c := range v.Crew {
					names[i] = c.Name
				}
				m.Kind, m.Text = "crew", "Crew: "+strings.Join(names, ", ")
				if len(v.Crew) == 0 {
					m.Text = "Crew cleared"
				}
			case EvUnitCover:
				var v coverData
				json.Unmarshal(e.Data, &v)
				m.Kind, m.Text = "cover", "Back to its own station"
				if v.Station != "" {
					m.Text = "Moved up to cover " + v.Station
				}
			case EvEmergency:
				m.Kind, m.Text = "emergency", "Emergency button"
			}
			h.Marks = append(h.Marks, m)
		}
	}
	if cur != nil {
		cur.Open = true
		closeAt(to)
		if n := len(h.Spans); n > 0 {
			h.Spans[n-1].Open = true
		}
	}
	for _, sp := range h.Spans {
		h.Totals[sp.Status] += sp.Seconds
		h.Known += sp.Seconds
		if Committed(sp.Status) {
			h.Committed += sp.Seconds
		}
	}
	sort.SliceStable(h.Marks, func(i, j int) bool { return h.Marks[i].At.Before(h.Marks[j].At) })
	return h
}

// UnitHistory reads a unit's status history from the ledger.
func (e *Engine) UnitHistory(unit string, from, to time.Time) (UnitHistory, error) {
	unit = strings.ToUpper(unit)
	ok := false
	e.Read(func(st *State) { _, ok = st.Units[unit] })
	if !ok {
		return UnitHistory{}, problem("unit", "no unit %s", unit)
	}
	if !to.After(from) {
		return UnitHistory{}, problem("to", "the period ends before it starts")
	}
	if to.Sub(from) > 32*24*time.Hour {
		return UnitHistory{}, problem("to", "a history is up to 31 days")
	}
	events, err := e.allEvents()
	if err != nil {
		return UnitHistory{}, err
	}
	return HistoryOf(events, unit, from, to), nil
}
