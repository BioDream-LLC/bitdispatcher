package cad

// Alarm levels: a working fire grows past its first run card. Each call type may have further alarms
// (2nd, 3rd…), each adding units to what the incident needs; upgrading an incident to the next alarm makes
// the recommendation fill the larger run card, and one keystroke sends the extra units.

import (
	"fmt"
	"strings"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// EvIncAlarm records an incident moved to another alarm level.
const EvIncAlarm = "incident.alarm"

// AlarmLevel is one further alarm of a call type: its name and the units it adds to the run card.
type AlarmLevel struct {
	Name    string        `json:"name"` // "2nd alarm"
	RunCard []Requirement `json:"runCard"`
}

type alarmData struct {
	Level  int    `json:"level"` // 1 is the first alarm (the call type's own run card)
	Name   string `json:"name"`
	Reason string `json:"reason,omitempty"`
}

// AlarmName is the name of an incident's alarm level: "1st alarm" for the first.
func (s *State) AlarmName(in *Incident) string {
	return s.alarmName(in.Type, in.AlarmLevel())
}

func (s *State) alarmName(typ string, level int) string {
	if level <= 1 {
		return "1st alarm"
	}
	if ct, ok := s.CallTypes[typ]; ok && level-2 < len(ct.Alarms) && ct.Alarms[level-2].Name != "" {
		return ct.Alarms[level-2].Name
	}
	return ordinal(level) + " alarm"
}

// AlarmLevel is the incident's alarm level, 1 for the first alarm.
func (in *Incident) AlarmLevel() int { return max(1, in.Alarm) }

// RunCardFor is the run card an incident needs at its alarm level: the call type's own, and each further
// alarm's units added to it.
func (s *State) RunCardFor(in *Incident) []Requirement {
	ct, ok := s.CallTypes[in.Type]
	if !ok {
		return nil
	}
	return mergeRunCards(ct, in.AlarmLevel())
}

func mergeRunCards(ct *CallType, level int) []Requirement {
	out := append([]Requirement(nil), ct.RunCard...)
	for i := 0; i < level-1 && i < len(ct.Alarms); i++ {
		for _, r := range ct.Alarms[i].RunCard {
			found := false
			for j := range out {
				if out[j].Capability == r.Capability {
					out[j].Count += r.Count
					found = true
				}
			}
			if !found {
				out = append(out, r)
			}
		}
	}
	return out
}

// NextAlarm says what upgrading an incident one alarm would add, for the button: the next level's name and
// its run card, or ok false when the call type has no further alarm.
func (s *State) NextAlarm(in *Incident) (name string, adds []Requirement, ok bool) {
	ct, found := s.CallTypes[in.Type]
	lv := in.AlarmLevel()
	if !found || lv-1 >= len(ct.Alarms) {
		return "", nil, false
	}
	return s.alarmName(in.Type, lv+1), ct.Alarms[lv-1].RunCard, true
}

// SetAlarm moves an incident to an alarm level: up to the call type's last further alarm, or back down
// (with a reason). Upgrading is a priority entry on the timeline that every dispatcher and crew sees.
func (e *Engine) SetAlarm(a Actor, number string, level int, reason string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	in, err := e.openIncident(number)
	if err != nil {
		return err
	}
	ct := e.st.CallTypes[in.Type]
	most := 1
	if ct != nil {
		most = 1 + len(ct.Alarms)
	}
	switch {
	case level < 1:
		return problem("level", "the first alarm is level 1")
	case level > most && most == 1:
		return problem("level", "%s has no further alarms: add them in Admin › Call types, or send units by hand", e.st.typeLabel(in.Type))
	case level > most:
		return problem("level", "%s goes up to the %s", e.st.typeLabel(in.Type), e.st.alarmName(in.Type, most))
	case level == in.AlarmLevel():
		return problem("level", "%s is already at the %s", number, e.st.alarmName(in.Type, level))
	case level < in.AlarmLevel() && strings.TrimSpace(reason) == "":
		return problem("reason", "say why the alarm is brought down")
	}
	_, err = e.write(a, EvIncAlarm, refs{Incident: number}, alarmData{Level: level, Name: e.st.alarmName(in.Type, level), Reason: strings.TrimSpace(reason)})
	return err
}

// UpgradeAlarm moves an incident one alarm up.
func (e *Engine) UpgradeAlarm(a Actor, number, reason string) error {
	lv := 1
	e.Read(func(st *State) {
		if in := st.Incidents[number]; in != nil {
			lv = in.AlarmLevel()
		}
	})
	return e.SetAlarm(a, number, lv+1, reason)
}

func (s *State) applyAlarm(e ledger.Event) error {
	v, err := decode[alarmData](e)
	if err != nil {
		return err
	}
	in, ok := s.Incidents[e.Incident]
	if !ok {
		return nil
	}
	old := in.AlarmLevel()
	up := v.Level > old
	in.Alarm = v.Level
	text := "Upgraded to the " + v.Name
	if !up {
		text = "Brought down to the " + v.Name
	}
	if ct, ok := s.CallTypes[in.Type]; ok && up {
		var adds []string
		// The alarms between the old level and the new one, each adding its units.
		for lv := old + 1; lv <= v.Level && lv-2 < len(ct.Alarms); lv++ {
			for _, r := range ct.Alarms[lv-2].RunCard {
				adds = append(adds, fmt.Sprintf("%d × %s", r.Count, r.Capability))
			}
		}
		if len(adds) > 0 {
			text += ": adds " + strings.Join(adds, ", ")
		}
	}
	if v.Reason != "" {
		text += " (" + v.Reason + ")"
	}
	in.Timeline = append(in.Timeline, Entry{Seq: e.Seq, At: e.At, Actor: e.Actor, Kind: "alarm", Text: text, Priority: up})
	in.Updated = e.At
	return nil
}

func checkAlarms(c CallType) error {
	if len(c.Alarms) > 8 {
		return problem("alarms", "up to 8 further alarms")
	}
	for i, al := range c.Alarms {
		if len(al.RunCard) == 0 {
			return problem(fmt.Sprintf("alarms[%d]", i), "the %s adds no units: give it at least one line", ordinal(i+2)+" alarm")
		}
		for j, r := range al.RunCard {
			if r.Capability == "" || r.Count < 1 || r.Count > 20 {
				return problem(fmt.Sprintf("alarms[%d].runCard[%d]", i, j), "each line needs a capability and 1 to 20 units")
			}
		}
	}
	return nil
}
