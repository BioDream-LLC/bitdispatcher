package cad

// The full Critical Incident Review, assembled when a supervisor asks for it: the source inventory, one
// synchronized timeline (agency time, with each source's original time and clock), the people and their
// duties, call-handling milestones, every unit's status times, tagged radio traffic, NCIC and state
// requests (metadata only), the QA matrix with each computed interval's start and end events, and the
// evidence limitations. Facts, calculated times, machine tags, supervisor observations and final human
// findings are kept apart and labelled.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// SourceItem is one evidence source in the inventory.
type SourceItem struct {
	Name    string `json:"name"`
	Present bool   `json:"present"`
	Note    string `json:"note,omitempty"`
}

// TimelineItem is one moment on the synchronized timeline.
type TimelineItem struct {
	At       time.Time `json:"at"` // agency time standard (UTC, shown in the agency's zone)
	Source   string    `json:"source"`
	Seq      uint64    `json:"seq,omitempty"` // the ledger event
	Actor    string    `json:"actor,omitempty"`
	Duty     string    `json:"duty,omitempty"`
	Console  string    `json:"console,omitempty"`
	Text     string    `json:"text"`
	Original string    `json:"original,omitempty"` // the source's own time, when it is not the ledger's
	Clock    string    `json:"clock"`              // which clock the time came from
	Kind     string    `json:"kind"`               // fact, calculated, machine, supervisor
}

// Milestone is a call-handling moment with the event it comes from.
type Milestone struct {
	Name   string    `json:"name"`
	At     time.Time `json:"at,omitzero"`
	Seq    uint64    `json:"seq,omitempty"`
	Source string    `json:"source"`
	Note   string    `json:"note,omitempty"`
}

// UnitActivity is one unit's part in the incident.
type UnitActivity struct {
	Unit        string               `json:"unit"`
	Recommended bool                 `json:"recommended"`
	Times       map[string]time.Time `json:"times"`
	Seqs        map[string]uint64    `json:"seqs"`
}

// QARow is one QA measure in the matrix: what was calculated, from which events, and the human answer.
type QARow struct {
	Measure  QAMeasure `json:"measure"`
	Seconds  *float64  `json:"seconds,omitempty"`
	FromSeq  uint64    `json:"fromSeq,omitempty"`
	ToSeq    uint64    `json:"toSeq,omitempty"`
	FromAt   time.Time `json:"fromAt,omitzero"`
	ToAt     time.Time `json:"toAt,omitzero"`
	Within   *bool     `json:"within,omitempty"`
	Evidence []string  `json:"evidence,omitempty"` // machine-tagged evidence: a prompt, not a finding
	Prompt   string    `json:"prompt,omitempty"`   // a possible gap for the reviewer
	Answer   *QAAnswer `json:"answer,omitempty"`
}

// CriticalReport is the assembled full review.
type CriticalReport struct {
	Case        CriticalCase   `json:"case"`
	Incident    Incident       `json:"incident"`
	Version     int            `json:"version"` // the version it will be when issued
	Cutoff      time.Time      `json:"cutoff"`
	Generated   time.Time      `json:"generated"`
	TimeZone    string         `json:"timeZone"`
	Sources     []SourceItem   `json:"sources"`
	Timeline    []TimelineItem `json:"timeline"`
	Personnel   []CaseRole     `json:"personnel"`
	Milestones  []Milestone    `json:"milestones"`
	Units       []UnitActivity `json:"units"`
	Radio       []RadioTag     `json:"radio"`
	RadioLines  []RadioLine    `json:"radioLines"`
	Requests    []Request      `json:"requests"`
	QA          []QARow        `json:"qa"`
	Limitations []string       `json:"limitations"`
	Flags       []FlagShown    `json:"flags,omitempty"`
}

// BuildCriticalReport assembles the full review of a case from the incident's ledger events up to the
// cutoff (zero: now).
func (s *State) BuildCriticalReport(id string, events []ledger.Event, cutoff, now time.Time) (CriticalReport, error) {
	c := s.CriticalCases[id]
	if c == nil {
		return CriticalReport{}, problem("id", "no critical review %s", id)
	}
	in := s.Incidents[c.Incident]
	if in == nil {
		return CriticalReport{}, problem("incident", "no incident %s", c.Incident)
	}
	if cutoff.IsZero() {
		cutoff = now
	}
	r := CriticalReport{Case: *c, Incident: *in, Version: len(c.Reports) + 1, Cutoff: cutoff, Generated: now, TimeZone: s.Settings.Shift.loc().String(), Flags: in.Flags}
	r.Incident.Persons = nil
	for _, p := range in.Persons {
		r.Incident.Persons = append(r.Incident.Persons, MaskPerson(p))
	}
	// The calls.
	var calls []*Call
	for _, cid := range in.Calls {
		if cl := s.Calls[cid]; cl != nil {
			calls = append(calls, cl)
		}
	}
	// Sources.
	hasTranscript, recording := false, ""
	for _, cl := range calls {
		hasTranscript = hasTranscript || len(cl.Transcript) > 0
		if cl.Handling != nil && cl.Handling.Recording != "" {
			recording = cl.Handling.Recording
		}
	}
	for _, l := range s.Radio {
		if l.Incident == in.Number && !l.At.After(cutoff) {
			r.RadioLines = append(r.RadioLines, l)
		}
	}
	for _, q := range s.Requests {
		if q.Incident == in.Number && !q.RequestedAt.After(cutoff) {
			r.Requests = append(r.Requests, *q)
		}
	}
	sort.Slice(r.Requests, func(i, j int) bool { return r.Requests[i].RequestedAt.Before(r.Requests[j].RequestedAt) })
	r.Sources = []SourceItem{
		{Name: "CAD record and audit (ledger)", Present: true, Note: fmt.Sprintf("%d events, hash-chained", countFor(events, in.Number, cutoff))},
		{Name: "Call handling", Present: len(calls) > 0, Note: fmt.Sprintf("%d calls", len(calls))},
		{Name: "Call recording", Present: recording != "", Note: firstNonEmpty(recording, "no recording reference from the call-handling system")},
		{Name: "Call transcript", Present: hasTranscript, Note: map[bool]string{true: "speech-to-text: an aid to finding things, not the record", false: "none"}[hasTranscript]},
		{Name: "Radio traffic (text)", Present: len(r.RadioLines) > 0, Note: fmt.Sprintf("%d transmissions", len(r.RadioLines))},
		{Name: "Radio audio", Present: false, Note: "held by the radio or call-handling system; listen there at the times shown"},
		{Name: "NCIC and state requests", Present: len(r.Requests) > 0, Note: fmt.Sprintf("%d requests (metadata; returns only in the protected record)", len(r.Requests))},
		{Name: "Flags shown", Present: len(in.Flags) > 0, Note: fmt.Sprintf("%d flag versions", len(in.Flags))},
	}
	// Timeline and units from the ledger.
	units := map[string]*UnitActivity{}
	unit := func(id string) *UnitActivity {
		u := units[id]
		if u == nil {
			u = &UnitActivity{Unit: id, Times: map[string]time.Time{}, Seqs: map[string]uint64{}}
			units[id] = u
		}
		return u
	}
	mile := map[string]Milestone{}
	setMile := func(name string, at time.Time, seq uint64, source, note string) {
		if m, ok := mile[name]; ok && !m.At.IsZero() && !at.Before(m.At) {
			return
		}
		mile[name] = Milestone{Name: name, At: at, Seq: seq, Source: source, Note: note}
	}
	for _, cl := range calls {
		if !cl.Start.IsZero() {
			setMile("received", cl.Start, 0, "call handling", "call "+cl.ID+" ("+firstNonEmpty(cl.Callback, "no callback")+")")
			r.Timeline = append(r.Timeline, TimelineItem{At: cl.Start, Source: "call", Text: "Call " + cl.ID + " received", Original: cl.Start.Format(time.RFC3339Nano), Clock: "call-handling system", Kind: "fact"})
		}
		if !cl.Answer.IsZero() {
			setMile("answered", cl.Answer, 0, "call handling", "by "+firstNonEmpty(cl.Agent, "the call-handling system"))
			r.Timeline = append(r.Timeline, TimelineItem{At: cl.Answer, Source: "call", Actor: cl.Agent, Text: "Call " + cl.ID + " answered", Original: cl.Answer.Format(time.RFC3339Nano), Clock: "call-handling system", Kind: "fact"})
		}
		for _, l := range cl.Transcript {
			if !l.At.After(cutoff) {
				r.Timeline = append(r.Timeline, TimelineItem{At: l.At, Source: "transcript", Text: l.Speaker + ": " + l.Text, Clock: "transcript", Kind: "machine"})
			}
		}
	}
	for _, e := range events {
		if e.Incident != in.Number || e.At.After(cutoff) {
			continue
		}
		text := e.Type
		switch e.Type {
		case EvIncCreated:
			setMile("created", e.At, e.Seq, "CAD", "by "+e.Actor)
			setMile("typed", e.At, e.Seq, "CAD", "type selected when the incident was made")
			text = "Incident created"
		case EvIncVerified:
			setMile("located", e.At, e.Seq, "CAD", "location verified")
			text = "Location verified"
		case EvIncPlace:
			d, _ := decode[placeData](e)
			if d.Place.Verify == PlaceVerified {
				setMile("located", e.At, e.Seq, "CAD", "location verified")
			} else {
				setMile("located", e.At, e.Seq, "CAD", "location "+d.Place.Verify+": "+d.Place.Entered)
			}
			text = "Place: " + d.Place.Verify + " " + d.Place.Label()
		case EvIncDispatch:
			d, _ := decode[dispatchData](e)
			setMile("dispatched", e.At, e.Seq, "CAD", strings.Join(d.Units, ", "))
			for _, u := range d.Units {
				ua := unit(u)
				ua.Recommended = ua.Recommended || slices.Contains(d.Recommended, u)
				if _, ok := ua.Times["dispatched"]; !ok {
					ua.Times["dispatched"], ua.Seqs["dispatched"] = e.At, e.Seq
				}
			}
			text = "Dispatched " + strings.Join(d.Units, ", ")
			if d.Reason != "" {
				text += " (instead of the recommendation: " + d.Reason + ")"
			}
		case EvUnitStatus:
			d, _ := decode[statusData](e)
			ua := unit(e.Unit)
			if _, ok := ua.Times[d.Status]; !ok {
				ua.Times[d.Status], ua.Seqs[d.Status] = e.At, e.Seq
			}
			if d.Status == Enroute {
				setMile("enroute", e.At, e.Seq, "CAD", e.Unit)
			}
			if d.Status == OnScene {
				setMile("onscene", e.At, e.Seq, "CAD", e.Unit)
			}
			text = e.Unit + " " + d.Status
		case EvUnitAcked:
			ua := unit(e.Unit)
			if _, ok := ua.Times["acknowledged"]; !ok {
				ua.Times["acknowledged"], ua.Seqs["acknowledged"] = e.At, e.Seq
			}
			text = e.Unit + " acknowledged"
		default:
			// The incident's own timeline line for this event, when it has one.
			for _, t := range in.Timeline {
				if t.Seq == e.Seq {
					text = t.Text
					break
				}
			}
		}
		r.Timeline = append(r.Timeline, TimelineItem{At: e.At, Source: "cad", Seq: e.Seq, Actor: e.Actor, Duty: e.Duty, Console: e.Console, Text: text, Clock: "bitdispatcher server", Kind: "fact"})
	}
	for _, l := range r.RadioLines {
		r.Timeline = append(r.Timeline, TimelineItem{At: l.At, Source: "radio", Actor: firstNonEmpty(l.Unit, l.RadioUnit), Text: firstNonEmpty(l.ChannelName, l.Channel) + ": " + l.Text, Clock: "radio system", Kind: "machine"})
	}
	for _, q := range r.Requests {
		r.Timeline = append(r.Timeline, TimelineItem{At: q.RequestedAt, Source: "request", Actor: q.Owner, Text: fmt.Sprintf("%s %s %s requested by %s: %s", q.ID, q.Type, q.Category, firstNonEmpty(q.Requester, "-"), s.Settings.Requests.Label(q.Status)), Clock: "bitdispatcher server", Kind: "fact"})
	}
	if c.Ack != nil {
		r.Timeline = append(r.Timeline, TimelineItem{At: c.Ack.At, Source: "review", Actor: c.Ack.By, Text: "Supervisor acknowledged the critical alert", Clock: "bitdispatcher server", Kind: "supervisor"})
	}
	sort.SliceStable(r.Timeline, func(i, j int) bool { return r.Timeline[i].At.Before(r.Timeline[j].At) })
	for _, name := range []string{"received", "answered", "located", "typed", "created", "dispatched", "enroute", "onscene"} {
		m, ok := mile[name]
		if !ok {
			m = Milestone{Name: name, Source: "-", Note: "no event: not recorded"}
		}
		r.Milestones = append(r.Milestones, m)
	}
	for _, u := range units {
		r.Units = append(r.Units, *u)
	}
	sort.Slice(r.Units, func(i, j int) bool { return r.Units[i].Unit < r.Units[j].Unit })
	r.Personnel = slices.Clone(c.Roles)
	// Radio tags, with requests matched to the queue.
	r.Radio = s.RadioTags(in.Number)
	r.Radio = slices.DeleteFunc(r.Radio, func(t RadioTag) bool { return t.At.After(cutoff) })
	// Limitations.
	if recording == "" {
		r.Limitations = append(r.Limitations, "No call recording reference: listen in the call-handling system.")
	}
	if !hasTranscript {
		r.Limitations = append(r.Limitations, "No call transcript.")
	}
	if len(r.RadioLines) == 0 {
		r.Limitations = append(r.Limitations, "No radio traffic text for this incident; radio audio, if any, is in the radio system.")
	} else {
		r.Limitations = append(r.Limitations, "Radio speaker identity is the radio ID or unit the radio system reported; transcription confidence is not reported by the source.")
	}
	for _, a := range s.PhraseAlerts {
		if a.Incident == in.Number && (a.Uncertain || a.Negated) {
			r.Limitations = append(r.Limitations, fmt.Sprintf("Phrase alert %s (%q) is %s: it needs human validation.", a.ID, a.Found, map[bool]string{true: "uncertain", false: "negated"}[a.Uncertain]))
		}
	}
	// QA matrix.
	for _, m := range c.Template.Measures {
		row := QARow{Measure: m}
		if m.Kind == "interval" {
			f, t := mile[m.From], mile[m.To]
			if !f.At.IsZero() && !t.At.IsZero() {
				sec := t.At.Sub(f.At).Seconds()
				row.Seconds, row.FromSeq, row.ToSeq, row.FromAt, row.ToAt = &sec, f.Seq, t.Seq, f.At, t.At
				if m.StandardSec > 0 {
					w := sec <= float64(m.StandardSec)
					row.Within = &w
				}
			} else {
				row.Prompt = "cannot be calculated: " + map[bool]string{true: m.From, false: m.To}[f.At.IsZero()] + " was not recorded"
			}
		}
		if m.Expect != "" {
			for _, t := range r.Radio {
				if t.Tag == m.Expect && !t.Negated {
					row.Evidence = append(row.Evidence, fmt.Sprintf("radio %s %s: %q", t.At.In(s.Settings.Shift.loc()).Format("15:04:05"), t.Unit, t.Context))
				}
			}
			for _, tl := range in.Timeline {
				if tl.At.After(cutoff) {
					continue
				}
				if strings.Contains(strings.ToLower(tl.Text), m.Expect) {
					row.Evidence = append(row.Evidence, fmt.Sprintf("CAD #%d %s: %s", tl.Seq, tl.At.In(s.Settings.Shift.loc()).Format("15:04:05"), tl.Text))
				}
			}
			if m.Expect == "request" {
				for _, t := range r.Radio {
					if t.Tag != "request" || t.Negated {
						continue
					}
					matched := ""
					for _, q := range r.Requests {
						if !q.RequestedAt.Before(t.At) && q.RequestedAt.Sub(t.At) < 10*time.Minute {
							matched = q.ID
							break
						}
					}
					if matched == "" {
						row.Prompt = strings.TrimSpace(row.Prompt + fmt.Sprintf(" Possible gap to review: %s asked (%q) at %s and no request was logged within 10 minutes.", t.Unit, t.Found, t.At.In(s.Settings.Shift.loc()).Format("15:04:05")))
					}
				}
			}
			if len(row.Evidence) == 0 && row.Prompt == "" {
				row.Prompt = "no " + m.Expect + " found in the radio tags or the CAD record: check the audio"
			}
		}
		for i := range c.QA {
			if c.QA[i].Measure == m.ID {
				a := c.QA[i]
				row.Answer = &a
			}
		}
		r.QA = append(r.QA, row)
	}
	return r, nil
}

func countFor(events []ledger.Event, number string, cutoff time.Time) int {
	n := 0
	for _, e := range events {
		if e.Incident == number && !e.At.After(cutoff) {
			n++
		}
	}
	return n
}

// Text is the report as plain text, for print, export and the hash of what was issued.
func (r CriticalReport) Text(loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	tf := func(t time.Time) string {
		if t.IsZero() {
			return "-"
		}
		return t.In(loc).Format("2006-01-02 15:04:05")
	}
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	c := r.Case
	w("CRITICAL INCIDENT REVIEW %s, version %d", c.ID, r.Version)
	w("Incident %s %s P%d at %s", r.Incident.Number, r.Incident.Type, r.Incident.Priority, locLabel(r.Incident.Location))
	w("Evidence cutoff %s (%s). Generated %s.", tf(r.Cutoff), r.TimeZone, tf(r.Generated))
	w("Times are the agency standard (%s); each source's own clock is named.", r.TimeZone)
	w("")
	w("TRIGGERS")
	for _, t := range c.Triggers {
		w("  %s  %s: %s %s", tf(t.At), t.Kind, t.Name, t.Detail)
	}
	w("Designated critical: %v. Supervisor acknowledgement: %s", c.Designated, stampText(c.Ack, tf))
	w("")
	w("SOURCE INVENTORY")
	for _, s := range r.Sources {
		w("  [%s] %s: %s", map[bool]string{true: "x", false: " "}[s.Present], s.Name, s.Note)
	}
	w("")
	w("PERSONNEL AND DUTIES")
	for _, p := range r.Personnel {
		w("  %s: %s", p.User, p.Role)
	}
	w("")
	w("CALL-HANDLING MILESTONES (fact; source event in brackets)")
	for _, m := range r.Milestones {
		w("  %-10s %s  [%s #%d] %s", m.Name, tf(m.At), m.Source, m.Seq, m.Note)
	}
	w("")
	w("UNITS")
	for _, u := range r.Units {
		var parts []string
		for _, k := range []string{"dispatched", "acknowledged", Enroute, OnScene, Transporting, AtDestination, Available} {
			if t, ok := u.Times[k]; ok {
				parts = append(parts, fmt.Sprintf("%s %s", k, t.In(loc).Format("15:04:05")))
			}
		}
		w("  %s%s: %s", u.Unit, map[bool]string{true: " (recommended)", false: ""}[u.Recommended], strings.Join(parts, ", "))
	}
	w("")
	w("TAGGED RADIO TRAFFIC (machine tags: aids to review, not findings)")
	for _, t := range r.Radio {
		neg := ""
		if t.Negated {
			neg = " NEGATED"
		}
		w("  %s %s [%s%s] %q", t.At.In(loc).Format("15:04:05"), t.Unit, t.Name, neg, t.Text)
	}
	w("")
	w("REQUESTS AND INQUIRIES (metadata only)")
	for _, q := range r.Requests {
		w("  %s %s %s %s: requested by %s %s, owner %s, sent %s to %s, %s", q.ID, q.Type, q.Category, q.Subject, firstNonEmpty(q.Requester, "-"), tf(q.RequestedAt), q.Owner, tf(q.SentAt), firstNonEmpty(q.Destination, "-"), q.Status)
	}
	w("")
	w("QA MATRIX (%s, %s)", c.Template.Name, c.Template.Policy)
	for _, q := range r.QA {
		calc := ""
		if q.Seconds != nil {
			calc = fmt.Sprintf(" calculated %.0f s from #%d to #%d", *q.Seconds, q.FromSeq, q.ToSeq)
			if q.Within != nil {
				calc += map[bool]string{true: " (within", false: " (over"}[*q.Within] + fmt.Sprintf(" %d s)", q.Measure.StandardSec)
			}
		}
		ans := "not answered"
		if q.Answer != nil {
			ans = fmt.Sprintf("%s by %s: %s [%s]", q.Answer.Result, q.Answer.By, q.Answer.Comment, q.Answer.Citation)
		}
		w("  %s:%s; human finding: %s", q.Measure.Name, calc, ans)
		for _, ev := range q.Evidence {
			w("      evidence: %s", ev)
		}
		if q.Prompt != "" {
			w("      prompt: %s", q.Prompt)
		}
	}
	w("")
	w("EVIDENCE LIMITATIONS")
	for _, l := range r.Limitations {
		w("  - %s", l)
	}
	w("")
	w("SUPERVISOR NARRATIVE (observation)")
	w("  %s", firstNonEmpty(c.Narrative, "(none)"))
	w("FINDINGS (human)")
	w("  %s", firstNonEmpty(c.Findings, "(none)"))
	w("ACTIONS")
	for _, a := range c.Actions {
		w("  %s %s %s: %s", tf(a.At), a.By, a.What, a.Note)
	}
	w("")
	w("SYNCHRONIZED TIMELINE")
	for _, t := range r.Timeline {
		w("  %s [%s/%s] %s%s %s", tf(t.At), t.Source, t.Kind, t.Actor, dutyText(t.Duty), t.Text)
	}
	w("")
	w("VERSIONS")
	for _, v := range c.Reports {
		w("  v%d %s by %s, cutoff %s, sha256 %s %s", v.Version, v.Kind, v.By, tf(v.Cutoff), v.Hash, v.Note)
	}
	return b.String()
}

func stampText(s *Stamp, tf func(time.Time) string) string {
	if s == nil {
		return "not yet"
	}
	return s.By + " " + tf(s.At)
}

func dutyText(d string) string {
	if d == "" {
		return ""
	}
	return " (" + d + ")"
}

// HashText is the SHA-256 of an issued report's text.
func HashText(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}
