package cad

import (
	"fmt"
	"strings"
)

// Alert is what an outside system reports that is not a person calling: an alarm company's signal, a
// licence-plate reader's hotlist hit, a gunshot detection, a camera's analytic. It arrives as a call in
// the queue, with no voice, so a dispatcher makes the incident from it (or attaches it) as from any
// call; nothing creates an incident on its own.
type Alert struct {
	Kind   string `json:"kind"`             // alarm, lpr, gunshot, camera, other
	Sender string `json:"sender"`           // the alarm company, Flock, SoundThinking…
	ID     string `json:"id"`               // the sender's event ID, for updates and answers
	Event  string `json:"event"`            // what: burglary, fire, medical, panic, holdup, hotlist hit, gunfire…
	Detail string `json:"detail,omitempty"` // free text from the sender
	Status string `json:"status,omitempty"` // new, updated, cancelled (by the sender), verified

	// Alarms.
	Account   string `json:"account,omitempty"`   // the subscriber's account at the alarm company
	Premise   string `json:"premise,omitempty"`   // the name of the place (business, residence)
	Zone      string `json:"zone,omitempty"`      // the zone or sensor
	Permit    string `json:"permit,omitempty"`    // the local alarm permit
	Verified  bool   `json:"verified,omitempty"`  // the alarm company verified it (video, audio, a person)
	Callback  string `json:"callback,omitempty"`  // the alarm company's operator line
	Operator  string `json:"operator,omitempty"`  // the operator at the alarm company
	Keyholder string `json:"keyholder,omitempty"` // who has the key, and how to reach them

	// Plate readers.
	Plate   string `json:"plate,omitempty"`
	State   string `json:"state,omitempty"`
	Vehicle string `json:"vehicle,omitempty"` // make, model, colour as read
	Hotlist string `json:"hotlist,omitempty"` // the list and its reason (stolen vehicle, wanted person…)
	Camera  string `json:"camera,omitempty"`
	Heading string `json:"heading,omitempty"` // direction of travel
	Image   string `json:"image,omitempty"`   // a link to the picture at the sender (opened by the user)

	// Gunshot detection.
	Rounds     int    `json:"rounds,omitempty"`
	Confidence string `json:"confidence,omitempty"`
}

// Alert kinds.
const (
	AlertAlarm   = "alarm"
	AlertLPR     = "lpr"
	AlertGunshot = "gunshot"
	AlertCamera  = "camera"
	AlertCAD     = "cad" // an incident shared by a neighbouring agency's CAD
)

// CheckAlert tidies an alert and says what is missing.
func CheckAlert(a *Alert) error {
	a.Kind = strings.ToLower(strings.TrimSpace(a.Kind))
	switch a.Kind {
	case AlertAlarm, AlertLPR, AlertGunshot, AlertCamera, AlertCAD, "other":
	default:
		return problem("kind", "kind is alarm, lpr, gunshot, camera or other")
	}
	a.ID, a.Sender, a.Event = strings.TrimSpace(a.ID), strings.TrimSpace(a.Sender), strings.TrimSpace(a.Event)
	if a.ID == "" || a.Sender == "" {
		return problem("id", "give the sender and its event id")
	}
	if len(a.ID) > 100 || len(a.Sender) > 100 || len(a.Detail) > 4000 {
		return problem("id", "too long")
	}
	if a.Image != "" && !strings.HasPrefix(a.Image, "https://") {
		a.Image = "" // only links a browser can open safely
	}
	a.Plate = strings.ToUpper(strings.ReplaceAll(a.Plate, " ", ""))
	return nil
}

// CallID is the call an alert makes: one per sender and event, so updates land on the same call.
func (a *Alert) CallID() string {
	clean := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
				return r
			}
			return '-'
		}, s)
	}
	return fmt.Sprintf("%s-%s-%s", a.Kind, clean(strings.ToLower(a.Sender)), clean(a.ID))
}

// Summary is the alert in one line, for the queue and the incident's timeline.
func (a *Alert) Summary() string {
	var b []string
	switch a.Kind {
	case AlertAlarm:
		b = append(b, strings.ToUpper(firstNonEmpty(a.Event, "alarm"))+" alarm")
		if a.Zone != "" {
			b = append(b, "zone "+a.Zone)
		}
		if a.Verified {
			b = append(b, "VERIFIED by "+a.Sender)
		}
	case AlertLPR:
		b = append(b, "Plate reader: "+strings.TrimSpace(a.Plate+" "+a.State))
		if a.Hotlist != "" {
			b = append(b, a.Hotlist)
		}
		if a.Vehicle != "" {
			b = append(b, a.Vehicle)
		}
		if a.Heading != "" {
			b = append(b, "heading "+a.Heading)
		}
	case AlertCAD:
		b = append(b, "From "+a.Sender+": "+firstNonEmpty(a.Event, "incident")+" ("+a.ID+")")
	case AlertGunshot:
		s := "Gunshots detected"
		if a.Rounds > 0 {
			s = fmt.Sprintf("Gunshots detected: %d rounds", a.Rounds)
		}
		b = append(b, s)
	default:
		b = append(b, firstNonEmpty(a.Event, a.Kind))
	}
	if a.Premise != "" {
		b = append(b, a.Premise)
	}
	return strings.Join(b, " · ")
}

// SuggestedType is the call type an alert most likely is, by the agency's own codes when they exist.
func (s *State) SuggestedType(a *Alert) string {
	var want []string
	ev := strings.ToLower(a.Event)
	switch a.Kind {
	case AlertGunshot:
		want = []string{"SHOTS", "SHOTSFIRED", "SHOT", "WEAPON"}
	case AlertLPR:
		want = []string{"STOLENVEH", "STOLEN", "WANTED", "BOLO", "SUSP"}
	case AlertAlarm:
		switch {
		case strings.Contains(ev, "fire") || strings.Contains(ev, "smoke"):
			want = []string{"FALARM", "FIREALARM", "ALARMF", "FA"}
		case strings.Contains(ev, "medical"):
			want = []string{"MALARM", "MEDALARM", "MEDICAL"}
		case strings.Contains(ev, "panic") || strings.Contains(ev, "holdup") || strings.Contains(ev, "duress"):
			want = []string{"PANIC", "HOLDUP", "ROBALARM", "ALARMP"}
		case strings.Contains(ev, "co") || strings.Contains(ev, "carbon"):
			want = []string{"CO", "COALARM"}
		default:
			want = []string{"ALARM", "BALARM", "BURGALARM", "ALARMB"}
		}
	}
	for _, w := range want {
		if _, ok := s.CallTypes[w]; ok {
			return w
		}
	}
	return ""
}
