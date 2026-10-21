package cad

// Messages between the floor and the crews: a dispatcher to one unit, several, a whole discipline or
// everyone; a crew to dispatch or to another unit. Each message is a ledger event, so what was said and
// when it was read is kept like everything else. Crews may also attach their own unit to an incident
// that is waiting, where the agency allows it.

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvMessage     = "message.sent" // a message to units or to dispatch
	EvMessageRead = "message.read" // a message was read by a unit or a dispatcher
)

// Message addresses: a unit's ID, or one of these.
const (
	ToDispatch = "DISPATCH" // the dispatch floor
	ToAll      = "ALL"      // every unit
	ToFire     = "ALL-FIRE" // every fire unit
	ToEMS      = "ALL-EMS"
	ToPolice   = "ALL-POLICE"
)

// Message is one message.
type Message struct {
	ID       string               `json:"id"`
	At       time.Time            `json:"at"`
	From     string               `json:"from"`     // a unit ID, or the dispatcher's user name
	FromUnit bool                 `json:"fromUnit"` // sent from a unit's terminal
	To       []string             `json:"to"`
	Incident string               `json:"incident,omitempty"`
	Text     string               `json:"text"`
	Urgent   bool                 `json:"urgent,omitempty"`
	Read     map[string]time.Time `json:"read,omitempty"` // reader (unit ID or DISPATCH) -> when
}

// keepMessages is how many messages the live state keeps (the ledger keeps all of them).
const keepMessages = 500

// For reports whether a message is addressed to a unit of a discipline.
func (m *Message) For(unit, discipline string) bool {
	for _, t := range m.To {
		switch {
		case t == unit, t == ToAll:
			return true
		case t == ToFire && discipline == "fire", t == ToEMS && discipline == "ems", t == ToPolice && discipline == "police":
			return true
		}
	}
	return false
}

// ToFloor reports whether a message is for the dispatch floor.
func (m *Message) ToFloor() bool { return slices.Contains(m.To, ToDispatch) }

// SendMessage sends a message. A unit's crew (Actor duty field, fromUnit set) may write to dispatch or
// to other units; the floor to any unit, discipline or everyone.
func (e *Engine) SendMessage(a Actor, fromUnit string, to []string, incident, text string, urgent bool) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", problem("text", "write the message")
	}
	if len(text) > 1000 {
		return "", problem("text", "a message is up to 1,000 characters")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	var dest []string
	for _, t := range to {
		t = strings.ToUpper(strings.TrimSpace(t))
		switch t {
		case "":
			continue
		case ToDispatch, ToAll, ToFire, ToEMS, ToPolice:
		default:
			if _, ok := e.st.Units[t]; !ok {
				return "", problem("to", "no unit %s", t)
			}
		}
		if !slices.Contains(dest, t) {
			dest = append(dest, t)
		}
	}
	if incident != "" {
		in, ok := e.st.Incidents[incident]
		if !ok {
			return "", problem("incident", "no incident %s", incident)
		}
		if len(dest) == 0 { // to an incident: the units on it
			for _, as := range in.Units {
				if as.Cleared.IsZero() && !slices.Contains(dest, as.Unit) && as.Unit != fromUnit {
					dest = append(dest, as.Unit)
				}
			}
		}
	}
	if len(dest) == 0 {
		return "", problem("to", "say who the message is for: a unit, ALL, ALL-FIRE, ALL-EMS, ALL-POLICE or DISPATCH")
	}
	if fromUnit != "" && slices.ContainsFunc(dest, func(t string) bool { return strings.HasPrefix(t, "ALL") }) {
		return "", problem("to", "a crew writes to dispatch or to units, not to everyone")
	}
	m := Message{ID: fmt.Sprintf("M%s%d", e.sitePrefix, e.st.msgCount+1), From: a.User, FromUnit: fromUnit != "", To: dest, Incident: incident, Text: text, Urgent: urgent}
	if fromUnit != "" {
		m.From = fromUnit
	}
	_, err := e.write(a, EvMessage, refs{Incident: incident, Unit: fromUnit}, m)
	return m.ID, err
}

// ReadMessage marks a message read by a reader: a unit ID, or DISPATCH for the floor.
func (e *Engine) ReadMessage(a Actor, id, reader string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	m := e.st.message(id)
	if m == nil {
		return problem("id", "no message %s", id)
	}
	if _, done := m.Read[reader]; done {
		return nil
	}
	_, err := e.write(a, EvMessageRead, refs{}, map[string]string{"id": id, "reader": reader})
	return err
}

func (s *State) message(id string) *Message {
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if s.Messages[i].ID == id {
			return s.Messages[i]
		}
	}
	return nil
}

// MessagesFor are the recent messages to or from a unit, oldest first.
func (s *State) MessagesFor(unit string, since time.Time) []*Message {
	u := s.Units[unit]
	disc := ""
	if u != nil {
		disc = s.disciplineOf(u)
	}
	out := []*Message{}
	for _, m := range s.Messages {
		if m.At.Before(since) {
			continue
		}
		if m.From == unit && m.FromUnit || m.For(unit, disc) {
			out = append(out, m)
		}
	}
	return out
}

func (s *State) applyMessages(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvMessage:
		m, err := decode[Message](e)
		if err != nil {
			return true, err
		}
		m.At = e.At
		s.Messages = append(s.Messages, &m)
		if len(s.Messages) > keepMessages {
			s.Messages = slices.Clone(s.Messages[len(s.Messages)-keepMessages:])
		}
		s.msgCount++
		if in, ok := s.Incidents[m.Incident]; ok {
			in.add(e, "message", fmt.Sprintf("%s → %s: %s", m.From, strings.Join(m.To, ", "), m.Text))
		}
		return true, nil
	case EvNERISSent:
		r, err := decode[NERISRecord](e)
		if err != nil {
			return true, err
		}
		r.At, r.By = e.At, e.Actor
		if in, ok := s.Incidents[e.Incident]; ok {
			if r.ID == "" && in.NERIS != nil {
				r.ID = in.NERIS.ID
			}
			in.NERIS = &r
			if r.OK {
				in.add(e, "neris", "sent to NERIS")
			} else {
				in.add(e, "neris", "NERIS refused it: "+r.Error)
			}
		}
		return true, nil
	case EvShared:
		v, _ := decode[map[string]string](e)
		if in, ok := s.Incidents[e.Incident]; ok {
			if v["error"] != "" {
				in.add(e, "shared", "not sent to "+v["partner"]+": "+v["error"])
			} else {
				in.add(e, "shared", "sent to "+v["partner"]+suffix(v["reason"]))
			}
		}
		return true, nil
	case EvMessageRead:
		v, _ := decode[map[string]string](e)
		if m := s.message(v["id"]); m != nil {
			if m.Read == nil {
				m.Read = map[string]time.Time{}
			}
			if _, ok := m.Read[v["reader"]]; !ok {
				m.Read[v["reader"]] = e.At
			}
		}
		return true, nil
	}
	return false, nil
}

// SelfAttach attaches a crew's own unit to an incident that is waiting, where the agency allows it
// (Settings › Roles). It is a dispatch by the crew, with the reason "self-attached".
func (e *Engine) SelfAttach(a Actor, unit, number string) error {
	var why error
	e.Read(func(st *State) {
		switch {
		case !st.Settings.Duties.SelfAttach:
			why = problem("duty", "crews do not attach themselves here: dispatch sends units (an administrator can allow it in Settings › Roles)")
		case st.Incidents[number] == nil:
			why = problem("incident", "no incident %s", number)
		case st.Units[unit] == nil:
			why = problem("unit", "no unit %s", unit)
		case st.Incidents[number].Status == Closed:
			why = problem("incident", "%s is closed", number)
		case st.Units[unit].Status != Available:
			why = problem("unit", "%s is %s: clear first", unit, st.Units[unit].Status)
		}
	})
	if why != nil {
		return why
	}
	return e.DispatchWhy(a, number, []string{unit}, nil, "self-attached from the terminal", "")
}

// EvNERISSent records an incident sent to NERIS (the national fire and EMS reporting system).
const EvNERISSent = "neris.sent"

// NERISRecord is what NERIS last said about an incident.
type NERISRecord struct {
	ID     string    `json:"id,omitempty"` // NERIS's ID, for updates
	At     time.Time `json:"at"`
	OK     bool      `json:"ok"`
	Status int       `json:"status"`
	Error  string    `json:"error,omitempty"`
	By     string    `json:"by"`
}

// RecordNERIS keeps what NERIS answered for an incident.
func (e *Engine) RecordNERIS(a Actor, number string, r NERISRecord) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.st.Incidents[number]; !ok {
		return problem("incident", "no incident %s", number)
	}
	_, err := e.write(a, EvNERISSent, refs{Incident: number}, r)
	return err
}

// EvShared records an incident sent to a neighbouring agency's CAD.
const EvShared = "incident.shared"

// RecordShare keeps that an incident was sent to a neighbouring agency, and what it answered.
func (e *Engine) RecordShare(a Actor, number, partner, reason, failure string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.st.Incidents[number]; !ok {
		return problem("incident", "no incident %s", number)
	}
	_, err := e.write(a, EvShared, refs{Incident: number}, map[string]string{"partner": partner, "reason": reason, "error": failure})
	return err
}
