package cad

// The review views of one incident: as first entered, a summary, the detailed record and the full audit,
// each built from the ledger, so a reviewer sees what was known at the time and what changed after.

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// Views a reviewer can open.
var ReviewViews = []string{"initial", "summary", "detailed", "audit"}

// ReviewDoc is one view of an incident, as headed sections of lines.
type ReviewDoc struct {
	Incident  string          `json:"incident"`
	View      string          `json:"view"`
	Title     string          `json:"title"`
	Generated time.Time       `json:"generated"`
	Sections  []ReviewSection `json:"sections"`
	Chain     *ledger.Report  `json:"chain,omitempty"` // the audit's check of the ledger's hash chain
}

// ReviewSection is a heading and its lines.
type ReviewSection struct {
	Heading string   `json:"heading"`
	Lines   []string `json:"lines"`
}

// Text is the view as plain text: what a records request releases, before redaction.
func (d ReviewDoc) Text() string {
	var b strings.Builder
	b.WriteString(d.Title + "\n")
	for _, s := range d.Sections {
		b.WriteString("\n" + strings.ToUpper(s.Heading) + "\n")
		for _, l := range s.Lines {
			b.WriteString(l + "\n")
		}
	}
	return b.String()
}

func when(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

// incidentEvents are the ledger events of an incident and its calls, in order.
func (e *Engine) incidentEvents(number string) ([]ledger.Event, error) {
	calls := map[string]bool{}
	e.mu.Lock()
	if in, ok := e.st.Incidents[number]; ok {
		for _, c := range in.Calls {
			calls[c] = true
		}
	}
	e.mu.Unlock()
	all, err := e.allEvents()
	if err != nil {
		return nil, err
	}
	var out []ledger.Event
	for _, ev := range all {
		if ev.Incident == number || (ev.Call != "" && calls[ev.Call]) {
			out = append(out, ev)
		}
	}
	return out, nil
}

// Review builds a view of an incident.
func (e *Engine) Review(number, view string) (ReviewDoc, error) {
	if !slices.Contains(ReviewViews, view) {
		return ReviewDoc{}, problem("view", "a view is initial, summary, detailed or audit")
	}
	evs, err := e.incidentEvents(number)
	if err != nil {
		return ReviewDoc{}, err
	}
	var doc ReviewDoc
	var verr error
	e.Read(func(s *State) {
		in, ok := s.Incidents[number]
		if !ok {
			verr = problem("incident", "no incident %s", number)
			return
		}
		doc = ReviewDoc{Incident: number, View: view, Generated: e.now(), Sections: []ReviewSection{}}
		name := s.typeLabel(in.Type)
		switch view {
		case "initial":
			doc.Title = fmt.Sprintf("Incident %s as first entered", number)
			doc.Sections = s.initialView(in, evs)
		case "summary":
			doc.Title = fmt.Sprintf("Incident %s — %s — summary", number, name)
			doc.Sections = s.summaryView(in)
		case "detailed":
			doc.Title = fmt.Sprintf("Incident %s — %s — detailed record", number, name)
			doc.Sections = s.detailedView(in)
		case "audit":
			doc.Title = fmt.Sprintf("Incident %s — full audit", number)
			doc.Sections = auditView(evs)
		}
	})
	if verr != nil {
		return ReviewDoc{}, verr
	}
	if view == "audit" {
		v := ledger.Verify(e.path)
		doc.Chain = &v
	}
	return doc, nil
}

func (s *State) initialView(in *Incident, evs []ledger.Event) []ReviewSection {
	var created *ledger.Event
	for i := range evs {
		if evs[i].Type == EvIncCreated && evs[i].Incident == in.Number {
			created = &evs[i]
			break
		}
	}
	if created == nil {
		return []ReviewSection{{Heading: "Not found", Lines: []string{"The creating event is not in this ledger (it may come from another site)."}}}
	}
	v, _ := decode[incidentCreated](*created)
	lines := []string{
		"Created " + when(created.At) + " by " + created.Actor + duty(created.Duty) + " at " + firstNonEmpty(created.Console, "—"),
		"Call type: " + s.typeLabel(v.Type) + " (" + v.Type + "), priority " + fmt.Sprint(v.Priority),
		"Location: " + locLabel(v.Location),
	}
	if v.Location != nil {
		lines = append(lines, "Location source: "+firstNonEmpty(v.Location.Source, v.Location.Method, "entered")+uncert(v.Location.Radius))
	}
	if v.Caller != nil {
		lines = append(lines, "Caller: "+firstNonEmpty(v.Caller.Name, "—")+", callback "+firstNonEmpty(v.Caller.Callback, "—"))
	}
	if v.CallerAt != nil {
		lines = append(lines, "Caller located at: "+locLabel(v.CallerAt))
	}
	if len(v.Modifiers) > 0 {
		lines = append(lines, "Modifiers: "+strings.Join(v.Modifiers, ", "))
	}
	if v.Narrative != "" {
		lines = append(lines, "First note: "+v.Narrative)
	}
	if v.Call != "" {
		lines = append(lines, "From 911 call "+v.Call)
	}
	out := []ReviewSection{{Heading: "As submitted", Lines: lines}}
	// The call as it stood when the incident was made.
	var callLines []string
	for _, ev := range evs {
		if ev.Seq >= created.Seq {
			break
		}
		if ev.Call != "" && (ev.Type == EvCallReceived || ev.Type == EvCallUpdated) {
			var d CallData
			json.Unmarshal(ev.Data, &d)
			l := when(ev.At) + " " + ev.Type + " from " + d.Source
			if d.Location != nil {
				l += ": " + locLabel(d.Location) + uncert(d.Location.Radius)
			}
			callLines = append(callLines, l)
		}
	}
	if len(callLines) > 0 {
		out = append(out, ReviewSection{Heading: "The call before submission", Lines: callLines})
	}
	out = append(out, ReviewSection{Heading: "Later changes", Lines: []string{"Every later update is in the detailed and audit views; this view shows only the first submission."}})
	return out
}

func duty(d string) string {
	if d == "" {
		return ""
	}
	return " (" + d + ")"
}

func uncert(r float64) string {
	if r <= 0 {
		return ""
	}
	return fmt.Sprintf(" ±%.0f m", r)
}

// keyTimes are an incident's first dispatch, en route, on scene and close.
func keyTimes(in *Incident) (disp, enr, ons time.Time) {
	for _, a := range in.Units {
		if !a.Dispatched.IsZero() && (disp.IsZero() || a.Dispatched.Before(disp)) {
			disp = a.Dispatched
		}
		if !a.Enroute.IsZero() && (enr.IsZero() || a.Enroute.Before(enr)) {
			enr = a.Enroute
		}
		if !a.OnScene.IsZero() && (ons.IsZero() || a.OnScene.Before(ons)) {
			ons = a.OnScene
		}
	}
	return
}

func (s *State) summaryView(in *Incident) []ReviewSection {
	disp, enr, ons := keyTimes(in)
	chron := []string{
		"Created " + when(in.Created) + " by " + in.CreatedBy,
		"First unit dispatched " + when(disp),
		"First unit en route " + when(enr),
		"First unit on scene " + when(ons),
		"Closed " + when(in.Closed),
	}
	details := []string{"Call type: " + s.typeLabel(in.Type) + ", priority " + fmt.Sprint(in.Priority), "Location: " + locLabel(in.Location)}
	if len(in.Modifiers) > 0 {
		details = append(details, "Modifiers: "+strings.Join(in.Modifiers, ", "))
	}
	if in.Caller != nil {
		details = append(details, "Caller: "+firstNonEmpty(in.Caller.Name, "—")+", callback "+firstNonEmpty(in.Caller.Callback, "—"))
	}
	units := []string{}
	for _, a := range in.Units {
		units = append(units, fmt.Sprintf("%s dispatched %s, on scene %s, cleared %s", a.Unit, when(a.Dispatched), when(a.OnScene), when(a.Cleared)))
	}
	if len(units) == 0 {
		units = []string{"No unit was sent"}
	}
	return []ReviewSection{
		{Heading: "Summary", Lines: []string{s.Summary(in)}},
		{Heading: "Chronology", Lines: chron},
		{Heading: "Essential details", Lines: details},
		{Heading: "Responding units", Lines: units},
		{Heading: "Disposition", Lines: []string{firstNonEmpty(in.Disposition, "open")}},
	}
}

func (s *State) detailedView(in *Incident) []ReviewSection {
	out := s.summaryView(in)[1:]
	if len(in.Persons) > 0 {
		var l []string
		for _, p := range in.Persons {
			if p.Removed {
				continue
			}
			parts := []string{p.Role + ": " + firstNonEmpty(p.Name, "unnamed")}
			for _, kv := range [][2]string{{"DOB", p.DOB}, {"sex", p.Sex}, {"race", p.Race}, {"height", p.Height}, {"weight", p.Weight}, {"hair", p.Hair},
				{"eyes", p.Eyes}, {"clothing", p.Clothing}, {"address", p.Address}, {"phone", p.Phone}, {p.IDType, p.IDNumber}, {"notes", p.Notes}} {
				if kv[1] != "" {
					parts = append(parts, firstNonEmpty(kv[0], "ID")+" "+kv[1])
				}
			}
			if len(p.Aliases) > 0 {
				parts = append(parts, "aliases "+strings.Join(p.Aliases, ", "))
			}
			l = append(l, strings.Join(parts, "; "))
		}
		out = append(out, ReviewSection{Heading: "People", Lines: l})
	}
	if len(in.Vehicles) > 0 {
		var l []string
		for _, v := range in.Vehicles {
			if v.Removed {
				continue
			}
			l = append(l, strings.Join(strings.Fields(fmt.Sprintf("%s: %s %s %s %s %s plate %s %s VIN %s %s %s", v.Role, v.Color, v.Year, v.Make, v.Model, v.Body, v.State, v.Plate, v.VIN, v.Direction, v.Description)), " "))
		}
		out = append(out, ReviewSection{Heading: "Vehicles", Lines: l})
	}
	if len(in.Notices) > 0 {
		var l []string
		for _, n := range in.Notices {
			l = append(l, when(n.At)+" "+noticeText(n, false)+" — "+n.By)
		}
		out = append(out, ReviewSection{Heading: "Notifications, requests and contacts", Lines: l})
	}
	var tl []string
	for _, t := range in.Timeline {
		tl = append(tl, when(t.At)+" ["+t.Kind+"] "+t.Actor+": "+t.Text)
	}
	out = append(out, ReviewSection{Heading: "Narrative and timeline", Lines: tl})
	if len(in.Guidance) > 0 {
		var l []string
		for _, g := range in.Guidance {
			l = append(l, fmt.Sprintf("%s %s version %s, %s (shown to %s)", when(g.At), g.Doc, g.Version, g.Heading, g.By))
		}
		out = append(out, ReviewSection{Heading: "Approved guidance shown", Lines: l})
	}
	for _, id := range in.Calls {
		if c, ok := s.Calls[id]; ok {
			l := []string{"Callback " + firstNonEmpty(c.Callback, "—") + ", class " + firstNonEmpty(c.Class, "—") + ", " + c.State}
			for _, f := range c.Fixes {
				l = append(l, when(f.At)+" location "+fmt.Sprintf("%.5f, %.5f", f.Lat, f.Lon)+uncert(f.Radius)+" "+f.Method)
			}
			for _, t := range c.Transcript {
				line := when(t.At) + " " + t.Speaker + ": " + t.Text
				if t.English != "" && t.English != t.Text {
					line += " [English: " + t.English + "]"
				}
				l = append(l, line)
			}
			out = append(out, ReviewSection{Heading: "911 call " + id, Lines: l})
		}
	}
	return out
}

func auditView(evs []ledger.Event) []ReviewSection {
	l := []string{}
	for _, ev := range evs {
		line := fmt.Sprintf("#%d %s %s by %s%s at %s", ev.Seq, when(ev.At), ev.Type, ev.Actor, duty(ev.Duty), firstNonEmpty(ev.Console, "—"))
		if len(ev.Data) > 0 && len(ev.Data) < 600 {
			line += " " + string(ev.Data)
		} else if len(ev.Data) >= 600 {
			line += fmt.Sprintf(" (%d bytes of data)", len(ev.Data))
		}
		line += " hash " + shortHash(ev.Hash)
		l = append(l, line)
	}
	return []ReviewSection{{Heading: fmt.Sprintf("Every ledger event (%d), with who, when, where and its hash", len(evs)), Lines: l}}
}

func shortHash(h string) string {
	if len(h) > 16 {
		return h[:16]
	}
	return h
}
