package cad

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/assist"
)

// Summary is an incident in a paragraph, for the crews and for the next dispatcher: what, where, what the
// callers said that matters, the hazards on file, and who is going. It is built from the record by rule,
// not written by a model, so it never says anything the record does not.
func (s *State) Summary(in *Incident) string {
	var parts []string
	what := s.typeLabel(in.Type)
	head := fmt.Sprintf("%s, priority %d", what, in.Priority)
	if in.Location != nil && in.Location.Address != "" {
		head += ", at " + in.Location.Address
	}
	parts = append(parts, head+".")
	// What the callers said, the urgent first.
	var urgent, other, crash []string
	seen := map[string]bool{}
	callers := 0
	for _, id := range in.Calls {
		c, ok := s.Calls[id]
		if !ok {
			continue
		}
		callers++
		for _, d := range c.Details {
			if d.Kind == "address" || d.Kind == "name" || d.Kind == "count" && d.Quote == "crash data" {
				continue
			}
			k := strings.ToLower(d.Kind + d.Text)
			if seen[k] {
				continue
			}
			seen[k] = true
			if d.Urgent {
				urgent = append(urgent, detailWords(d))
			} else {
				other = append(other, detailWords(d))
			}
		}
		if c.CrashRisk != nil && c.CrashRisk.Summary != "" && !seen["crash"] {
			seen["crash"] = true
			crash = append(crash, "Vehicle crash data: "+lowerFirst(c.CrashRisk.Summary)+".")
		}
	}
	parts = append(parts, crash...)
	if said := append(urgent, other...); len(said) > 0 {
		who := "Caller reports"
		if callers > 1 {
			who = fmt.Sprintf("%d callers report", callers)
		}
		parts = append(parts, who+" "+strings.Join(said[:min(len(said), 8)], "; ")+".")
	}
	var pn []string
	for _, t := range in.Timeline {
		if t.Priority || t.Kind == "priority-note" {
			pn = append(pn, t.Text)
		}
	}
	if len(pn) > 0 {
		parts = append(parts, "Priority notes: "+strings.Join(pn[max(0, len(pn)-3):], "; ")+".")
	}
	if in.Location.HasPoint() {
		var hz []string
		for _, p := range s.PremisesNear(*in.Location.Lat, *in.Location.Lon) {
			for _, h := range p.Hazards {
				if !slices.Contains(hz, h) {
					hz = append(hz, h)
				}
			}
		}
		if len(hz) > 0 {
			parts = append(parts, "On file here: "+strings.Join(hz, ", ")+".")
		}
	}
	var going, there []string
	for _, a := range in.Units {
		if !a.Cleared.IsZero() {
			continue
		}
		if !a.OnScene.IsZero() {
			there = append(there, a.Unit)
		} else {
			going = append(going, a.Unit)
		}
	}
	if len(there) > 0 {
		parts = append(parts, "On scene: "+strings.Join(there, ", ")+".")
	}
	if len(going) > 0 {
		parts = append(parts, "Responding: "+strings.Join(going, ", ")+".")
	}
	for _, d := range s.Drones {
		if d.Incident == in.Number && d.Status == "onscene" {
			parts = append(parts, "Drone "+firstNonEmpty(d.Name, d.ID)+" over the scene.")
		}
	}
	if len(in.Units) == 0 && in.Status != Closed {
		parts = append(parts, "No units assigned yet.")
	}
	return strings.Join(parts, " ")
}

func detailWords(d assist.Detail) string {
	switch d.Kind {
	case "weapon":
		return "weapon: " + d.Text
	case "vehicle":
		return "vehicle: " + d.Text
	case "plate":
		return "plate " + d.Text
	case "direction":
		return "last seen " + d.Text
	}
	return d.Text
}

func lowerFirst(v string) string {
	if v == "" {
		return v
	}
	return strings.ToLower(v[:1]) + v[1:]
}

// ---- quality review ----

// Check is one item of a quality review.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Skip   bool   `json:"skip,omitempty"` // not applicable, or no evidence either way
	Detail string `json:"detail"`
}

// Review is an incident's quality review: what was done in time and what was asked, scored.
type Review struct {
	Incident string    `json:"incident"`
	Type     string    `json:"type"`
	Priority int       `json:"priority"`
	Created  time.Time `json:"created"`
	Taker    string    `json:"taker,omitempty"` // who took the call (the call's agent, or who created it)
	Score    int       `json:"score"`           // 0-100 over the checks that applied
	Checks   []Check   `json:"checks"`
	Critical string    `json:"critical,omitempty"` // a critical call: why
}

// Review checks one incident against the agency's standards: answer time, time to enter it, time to send a
// unit, that the location and callback were captured, a disposition, and the questions the call-taker
// was heard to ask (from the transcript, when there is one).
func (s *State) Review(in *Incident) Review {
	q := s.Settings.QASettings()
	r := Review{Incident: in.Number, Type: s.typeLabel(in.Type), Priority: in.Priority, Created: in.Created, Taker: in.CreatedBy}
	var call *Call
	for _, id := range in.Calls {
		if c, ok := s.Calls[id]; ok && (call == nil || c.Start.Before(call.Start)) {
			call = c
		}
	}
	add := func(c Check) { r.Checks = append(r.Checks, c) }
	secs := func(d time.Duration) string { return fmt.Sprintf("%.0f s", d.Seconds()) }
	if call != nil && !call.Answer.IsZero() && !call.Start.IsZero() {
		if call.Agent != "" {
			r.Taker = call.Agent
		}
		d := call.Answer.Sub(call.Start)
		add(Check{Name: "Answered in time", OK: d <= time.Duration(q.AnswerSeconds)*time.Second, Detail: fmt.Sprintf("%s (standard %d s)", secs(d), q.AnswerSeconds)})
		e := in.Created.Sub(call.Answer)
		add(Check{Name: "Entered in time", OK: e <= time.Duration(q.EntrySeconds)*time.Second, Detail: fmt.Sprintf("answer to incident %s (standard %d s)", secs(max(e, 0)), q.EntrySeconds)})
	} else {
		add(Check{Name: "Answered in time", Skip: true, Detail: "no call record with answer time"})
	}
	var first time.Time
	for _, a := range in.Units {
		if first.IsZero() || a.Dispatched.Before(first) {
			first = a.Dispatched
		}
	}
	switch {
	case in.Priority > 2:
		add(Check{Name: "Dispatched in time", Skip: true, Detail: fmt.Sprintf("priority %d: no standard", in.Priority)})
	case first.IsZero():
		add(Check{Name: "Dispatched in time", OK: false, Detail: "no unit was sent"})
	default:
		d := first.Sub(in.Created)
		add(Check{Name: "Dispatched in time", OK: d <= time.Duration(q.DispatchSeconds)*time.Second, Detail: fmt.Sprintf("%s from entry to the first unit (standard %d s)", secs(max(d, 0)), q.DispatchSeconds)})
	}
	add(Check{Name: "Location verified on the map", OK: in.Location.HasPoint(), Detail: map[bool]string{true: "placed on the map", false: "text only, never placed"}[in.Location.HasPoint()]})
	cb := in.Caller != nil && in.Caller.Callback != "" || call != nil && call.Callback != ""
	add(Check{Name: "Callback number", OK: cb, Detail: map[bool]string{true: "captured", false: "missing"}[cb]})
	if in.Status == Closed {
		add(Check{Name: "Disposition", OK: in.Disposition != "", Detail: firstNonEmpty(in.Disposition, "none")})
	}
	// The questions, from what the call-taker said.
	var taker strings.Builder
	for _, id := range in.Calls {
		if c, ok := s.Calls[id]; ok {
			taker.WriteString(strings.ToLower(c.TakerWords()))
		}
	}
	heard := taker.String()
	disc := ""
	if ct, ok := s.CallTypes[in.Type]; ok {
		disc = ct.Discipline
	}
	for _, set := range [][]string{q.Questions["all"], q.Questions[disc]} {
		for _, question := range set {
			words := strings.Split(question, "|")
			name := "Asked: " + strings.TrimSpace(words[0])
			if heard == "" {
				add(Check{Name: name, Skip: true, Detail: "no transcript of the call-taker"})
				continue
			}
			ok := false
			for _, w := range words {
				if w = strings.TrimSpace(strings.ToLower(w)); w != "" && strings.Contains(heard, w) {
					ok = true
					break
				}
			}
			add(Check{Name: name, OK: ok, Detail: map[bool]string{true: "heard", false: "not heard in the transcript"}[ok]})
		}
	}
	n, good := 0, 0
	for _, c := range r.Checks {
		if c.Skip {
			continue
		}
		n++
		if c.OK {
			good++
		}
	}
	if n > 0 {
		r.Score = (good*100 + n/2) / n
	}
	if cc, ok := s.Care[in.Number]; ok {
		r.Critical = cc.Reason
	}
	return r
}
