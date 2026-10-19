// Package advisory is the advisory panel: deterministic evidence rules from a written, versioned codebook
// (DESIGN.md 3.3, OPERATIONS.md 5). It proposes and explains; it never acts. Every suggestion names the
// evidence it relied on, the evidence it assumed absent, and the rule that applied, and "cannot determine"
// is an answer. The same state always gives the same suggestions: nothing here learns or changes itself.
//
// This is an independent implementation for bitdispatcher (Apache-2.0).
package advisory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/geo"
)

// Rule kinds.
const (
	KindDuplicate = "duplicate"  // two open incidents close in place and time
	KindCallNear  = "call-near"  // a call not attached to anything, near an open incident
	KindWords     = "words"      // what was said does not fit the call type, judged by source precedence
	KindRepeat    = "repeat"     // the address has had incidents recently
	KindPending   = "pending"    // a high-priority incident still has no unit
	KindNoUnit    = "unit-quiet" // a committed unit not heard from for a while
)

// Rule is one codebook rule. Only the fields its kind uses matter.
type Rule struct {
	ID       string  `json:"id"`
	Kind     string  `json:"kind"`
	Name     string  `json:"name"`
	Enabled  bool    `json:"enabled"`
	Severity string  `json:"severity"` // urgent, info
	Explain  string  `json:"explain"`  // the rule in plain words, shown with each suggestion
	Metres   float64 `json:"metres,omitempty"`
	Minutes  float64 `json:"minutes,omitempty"`
	Days     int     `json:"days,omitempty"`
	Count    int     `json:"count,omitempty"`
	Priority int     `json:"priority,omitempty"` // the highest priority number the rule applies to (1 = only P1)
	// words: the phrases, what they mean, and the call types that already fit them.
	Words []string `json:"words,omitempty"`
	Means string   `json:"means,omitempty"`
	Fits  []string `json:"fits,omitempty"`
	// Suggest is the call type proposed when the words are affirmed (optional).
	Suggest string `json:"suggest,omitempty"`
	// script: the code, its language (javascript, lua), its hook (incident, recommend, report) and its
	// time limit.
	Lang      string `json:"lang,omitempty"`
	Hook      string `json:"hook,omitempty"`
	Source    string `json:"source,omitempty"`
	TimeoutMs int    `json:"timeoutMs,omitempty"`
}

// Codebook is the agency's rules with the source precedence used to settle conflicts.
type Codebook struct {
	Rules []Rule `json:"rules"`
	// Precedence lists evidence sources from strongest to weakest. A statement from a stronger source
	// outweighs a contradicting one from a weaker; equal ranks are not settled ("cannot determine").
	Precedence []string `json:"precedence"`
}

// Sources of evidence.
const (
	SrcUnit       = "unit"       // a crew on scene, by terminal or radio
	SrcCaller     = "caller"     // the caller's own words (text to 911, call notes)
	SrcDispatcher = "dispatcher" // what the call-taker or dispatcher wrote
	SrcMachine    = "machine"    // alarm panels, crash notification, telematics
)

// Default is the codebook agencies start from.
func Default() Codebook {
	return Codebook{
		Precedence: []string{SrcUnit, SrcCaller, SrcDispatcher, SrcMachine},
		Rules: []Rule{
			{ID: "dup", Kind: KindDuplicate, Name: "Possible duplicate incident", Enabled: true, Severity: "urgent", Metres: 150, Minutes: 20,
				Explain: "Two open incidents within 150 m of each other created within 20 minutes may be the same emergency reported twice. Only a person can merge them."},
			{ID: "call-near", Kind: KindCallNear, Name: "Call near an open incident", Enabled: true, Severity: "info", Metres: 250, Minutes: 60,
				Explain: "A 911 call not attached to any incident, from within 250 m of an incident opened in the last hour, may be about it."},
			{ID: "fire-words", Kind: KindWords, Name: "Fire described", Enabled: true, Severity: "urgent", Means: "fire",
				Words: []string{"smoke", "flames", "fire showing", "on fire", "house fire", "fire in", "burning"}, Fits: []string{"SF", "VF", "BF", "HAZ"}, Suggest: "SF",
				Explain: "Smoke or flames reported on an incident that is not typed as a fire. An alarm panel that has reset never outweighs a person who can see smoke."},
			{ID: "trapped", Kind: KindWords, Name: "Someone trapped", Enabled: true, Severity: "urgent", Means: "entrapment",
				Words: []string{"trapped", "pinned", "can't get out", "cannot get out", "entrapment", "stuck inside"}, Fits: []string{"MVAPIN", "SF"},
				Explain: "A person reported trapped on an incident whose type does not bring rescue or extrication."},
			{ID: "not-breathing", Kind: KindWords, Name: "Not breathing", Enabled: true, Severity: "urgent", Means: "cardiac or respiratory arrest",
				Words: []string{"not breathing", "unresponsive", "unconscious", "no pulse", "cpr", "turning blue"}, Fits: []string{"CARD", "BRTH", "OD", "STROKE"}, Suggest: "CARD",
				Explain: "A patient described as not breathing or unresponsive on an incident not typed as an arrest or breathing problem."},
			{ID: "weapon", Kind: KindWords, Name: "Weapon mentioned", Enabled: true, Severity: "urgent", Means: "a weapon",
				Words: []string{"gun", "shots fired", "shot", "knife", "stabbed", "weapon", "armed"}, Fits: []string{"SHOOT", "STAB", "ROB", "BURG", "PURSUIT"},
				Explain: "A weapon mentioned on an incident whose type does not warn responders of one."},
			{ID: "repeat", Kind: KindRepeat, Name: "Repeat address", Enabled: true, Severity: "info", Days: 30, Count: 2,
				Explain: "The address has had two or more incidents in the last 30 days; the history may matter to responders."},
			{ID: "pending-p1", Kind: KindPending, Name: "Priority 1 waiting", Enabled: true, Severity: "urgent", Minutes: 1, Priority: 1,
				Explain: "A priority 1 incident has had no unit for more than a minute."},
			{ID: "quiet", Kind: KindNoUnit, Name: "Unit not heard", Enabled: true, Severity: "info", Minutes: 20,
				Explain: "A unit on scene that has not been heard on the radio or given a status for 20 minutes may need a status check. Not hearing from a unit is never taken as evidence that it is fine."},
		},
	}
}

// Parse reads a codebook and checks it.
func Parse(b []byte) (Codebook, error) {
	var c Codebook
	if len(b) == 0 {
		return Default(), nil
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("the codebook is not valid: %w", err)
	}
	return c, c.Check()
}

// Check validates a codebook.
func (c Codebook) Check() error {
	seen := map[string]bool{}
	for _, r := range c.Rules {
		if r.ID == "" || seen[r.ID] {
			return fmt.Errorf("each rule needs its own id (%q)", r.ID)
		}
		seen[r.ID] = true
		switch r.Kind {
		case KindDuplicate, KindCallNear:
			if r.Metres <= 0 || r.Minutes <= 0 {
				return fmt.Errorf("rule %s needs a distance and a time", r.ID)
			}
		case KindWords:
			if len(r.Words) == 0 {
				return fmt.Errorf("rule %s needs words", r.ID)
			}
		case KindRepeat:
			if r.Days <= 0 || r.Count <= 0 {
				return fmt.Errorf("rule %s needs days and a count", r.ID)
			}
		case KindPending, KindNoUnit:
			if r.Minutes <= 0 {
				return fmt.Errorf("rule %s needs minutes", r.ID)
			}
		case KindScript:
			if r.TimeoutMs < 0 || r.TimeoutMs > 1000 {
				return fmt.Errorf("rule %s: a script's time limit is up to 1000 ms", r.ID)
			}
			if _, err := compileRule(r); err != nil {
				return fmt.Errorf("rule %s: %w", r.ID, err)
			}
		default:
			return fmt.Errorf("rule %s has an unknown kind %q", r.ID, r.Kind)
		}
		if r.Severity != "urgent" && r.Severity != "info" {
			return fmt.Errorf("rule %s: severity is urgent or info", r.ID)
		}
	}
	for _, p := range c.Precedence {
		if !slices.Contains([]string{SrcUnit, SrcCaller, SrcDispatcher, SrcMachine}, p) {
			return fmt.Errorf("unknown source %q in the precedence", p)
		}
	}
	return nil
}

// Evidence is one fact a suggestion relied on, with where it came from.
type Evidence struct {
	Text     string `json:"text"`
	Source   string `json:"source"`
	Incident string `json:"incident,omitempty"`
	Call     string `json:"call,omitempty"`
	Seq      uint64 `json:"seq,omitempty"` // the ledger event
}

// Suggestion is what the panel proposes.
type Suggestion struct {
	ID       string     `json:"id"` // stable for the same rule and subject
	Rule     string     `json:"rule"`
	RuleName string     `json:"ruleName"`
	Severity string     `json:"severity"`
	Verdict  string     `json:"verdict"` // suggest, cannot-determine
	Title    string     `json:"title"`
	Detail   string     `json:"detail"`
	Explain  string     `json:"explain"`
	Incident string     `json:"incident,omitempty"`
	Other    string     `json:"other,omitempty"` // a second incident, for duplicates
	Call     string     `json:"call,omitempty"`
	Unit     string     `json:"unit,omitempty"`
	Action   *Action    `json:"action,omitempty"` // what accepting would do, for the person to confirm
	Relied   []Evidence `json:"relied"`
	Absent   []string   `json:"assumedAbsent"`
	Since    time.Time  `json:"since"`
}

// Action is the command a person may run to accept a suggestion. The panel never runs it itself.
type Action struct {
	Label   string `json:"label"`
	Command string `json:"command"` // a command line, run as the person who accepts
}

// Evaluate applies the codebook to the state. It is pure: the same state and time give the same answer.
func Evaluate(c Codebook, s *cad.State, now time.Time) []Suggestion {
	var out []Suggestion
	open := []*cad.Incident{}
	for _, in := range s.Incidents {
		if in.Status != cad.Closed {
			open = append(open, in)
		}
	}
	sort.Slice(open, func(i, j int) bool { return open[i].Number < open[j].Number })
	for _, r := range c.Rules {
		if !r.Enabled {
			continue
		}
		switch r.Kind {
		case KindDuplicate:
			out = append(out, duplicates(r, open)...)
		case KindCallNear:
			out = append(out, callsNear(r, s, open, now)...)
		case KindWords:
			for _, in := range open {
				if sg, ok := words(c, r, s, in); ok {
					out = append(out, sg)
				}
			}
		case KindRepeat:
			out = append(out, repeats(r, s, open, now)...)
		case KindPending:
			for _, in := range open {
				if in.Status == cad.Pending && (r.Priority == 0 || in.Priority <= r.Priority) && now.Sub(in.Created) > minutes(r.Minutes) {
					out = append(out, Suggestion{Rule: r.ID, Title: fmt.Sprintf("P%d %s has no unit", in.Priority, typeName(s, in.Type)),
						Detail: fmt.Sprintf("Waiting %s since %s.", dur(now.Sub(in.Created)), in.Created.Local().Format("15:04:05")), Incident: in.Number,
						Relied: []Evidence{{Text: fmt.Sprintf("created %s, no unit assigned", in.Created.Local().Format("15:04:05")), Source: SrcDispatcher, Incident: in.Number}},
						Action: &Action{Label: "Dispatch the recommendation", Command: "REC #" + in.Number}, Since: in.Created.Add(minutes(r.Minutes))})
				}
			}
		case KindNoUnit:
			for _, u := range s.Units {
				if u.Status != cad.OnScene {
					continue
				}
				last := u.StatusAt
				if u.LastHeard.After(last) {
					last = u.LastHeard
				}
				if now.Sub(last) > minutes(r.Minutes) {
					out = append(out, Suggestion{Rule: r.ID, Title: fmt.Sprintf("%s not heard for %s", u.ID, dur(now.Sub(last))),
						Detail: fmt.Sprintf("On scene at %s; no status or radio since %s. Consider a status check.", u.Incident, last.Local().Format("15:04")), Incident: u.Incident, Unit: u.ID,
						Relied: []Evidence{{Text: "last status or transmission " + last.Local().Format("15:04:05"), Source: SrcUnit, Incident: u.Incident}},
						Absent: []string{"any transmission since then (silence is not taken as all well)"}, Since: last.Add(minutes(r.Minutes))})
				}
			}
		}
	}
	byID := map[string]Rule{}
	for _, r := range c.Rules {
		byID[r.ID] = r
	}
	for i := range out {
		sg := &out[i]
		r := byID[sg.Rule]
		sg.RuleName, sg.Severity, sg.Explain = r.Name, r.Severity, r.Explain
		if sg.Verdict == "" {
			sg.Verdict = "suggest"
		}
		if sg.Relied == nil {
			sg.Relied = []Evidence{}
		}
		if sg.Absent == nil {
			sg.Absent = []string{}
		}
		h := sha256.Sum256([]byte(sg.Rule + "|" + sg.Incident + "|" + sg.Other + "|" + sg.Call + "|" + sg.Unit + "|" + sg.Verdict))
		sg.ID = hex.EncodeToString(h[:8])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity == "urgent"
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func minutes(m float64) time.Duration { return time.Duration(m * float64(time.Minute)) }

func dur(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d min", int(d.Minutes()))
	}
	return fmt.Sprintf("%.1f h", d.Hours())
}

func typeName(s *cad.State, code string) string {
	if ct, ok := s.CallTypes[code]; ok {
		return ct.Name
	}
	return code
}

func point(in *cad.Incident) (float64, float64, bool) {
	if in.Location == nil || in.Location.Lat == nil || in.Location.Lon == nil {
		return 0, 0, false
	}
	return *in.Location.Lat, *in.Location.Lon, true
}

func duplicates(r Rule, open []*cad.Incident) []Suggestion {
	var out []Suggestion
	for i, a := range open {
		la, lo, ok := point(a)
		if !ok {
			continue
		}
		for _, b := range open[i+1:] {
			lb, lob, ok := point(b)
			if !ok {
				continue
			}
			d := geo.Distance(la, lo, lb, lob)
			gap := b.Created.Sub(a.Created)
			if gap < 0 {
				gap = -gap
			}
			if d > r.Metres || gap > minutes(r.Minutes) {
				continue
			}
			first, second := a, b
			if b.Created.Before(a.Created) {
				first, second = b, a
			}
			out = append(out, Suggestion{Rule: r.ID, Incident: second.Number, Other: first.Number,
				Title:  fmt.Sprintf("%s may be the same as %s", second.Number, first.Number),
				Detail: fmt.Sprintf("%.0f m apart, opened %s apart (%s and %s).", d, dur(gap), first.Type, second.Type),
				Relied: []Evidence{
					{Text: fmt.Sprintf("%s at %s, %s", first.Number, addr(first), first.Created.Local().Format("15:04")), Source: SrcDispatcher, Incident: first.Number},
					{Text: fmt.Sprintf("%s at %s, %s", second.Number, addr(second), second.Created.Local().Format("15:04")), Source: SrcDispatcher, Incident: second.Number},
				},
				Absent: []string{"a confirmation by either caller that they are reporting the same thing"},
				Since:  second.Created})
		}
	}
	return out
}

func addr(in *cad.Incident) string {
	if in.Location != nil && in.Location.Address != "" {
		return in.Location.Address
	}
	if la, lo, ok := point(in); ok {
		return fmt.Sprintf("%.5f, %.5f", la, lo)
	}
	return "no location"
}

func callsNear(r Rule, s *cad.State, open []*cad.Incident, now time.Time) []Suggestion {
	var out []Suggestion
	ids := make([]string, 0, len(s.Calls))
	for id := range s.Calls {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		c := s.Calls[id]
		if c.Incident != "" || c.State == "ended" || c.Location == nil || c.Location.Lat == nil {
			continue
		}
		// Only a call from the same few minutes: one left open for hours is not about a new incident.
		if !c.Start.IsZero() && now.Sub(c.Start) > minutes(r.Minutes) {
			continue
		}
		best, bestD := (*cad.Incident)(nil), math.Inf(1)
		for _, in := range open {
			la, lo, ok := point(in)
			if !ok || now.Sub(in.Created) > minutes(r.Minutes) {
				continue
			}
			if d := geo.Distance(*c.Location.Lat, *c.Location.Lon, la, lo); d <= r.Metres+c.Location.Radius && d < bestD {
				best, bestD = in, d
			}
		}
		if best == nil {
			continue
		}
		out = append(out, Suggestion{Rule: r.ID, Incident: best.Number, Call: c.ID,
			Title:  fmt.Sprintf("Call %s may be about %s", firstNonEmpty(c.Callback, c.ID), best.Number),
			Detail: fmt.Sprintf("The caller is %.0f m from %s (%s).", bestD, best.Number, typeName(s, best.Type)),
			Relied: []Evidence{
				{Text: fmt.Sprintf("caller location %.5f, %.5f ± %.0f m", *c.Location.Lat, *c.Location.Lon, c.Location.Radius), Source: SrcMachine, Call: c.ID},
				{Text: fmt.Sprintf("%s at %s", best.Number, addr(best)), Source: SrcDispatcher, Incident: best.Number},
			},
			Absent: []string{"what the caller says: ask before attaching"},
			Action: &Action{Label: "Attach the call to " + best.Number, Command: "ATTACH " + c.ID + " #" + best.Number},
			Since:  c.Start})
	}
	return out
}

func firstNonEmpty(v ...string) string {
	for _, x := range v {
		if x != "" {
			return x
		}
	}
	return ""
}

// statement is one thing said about an incident, with its source.
type statement struct {
	text, source string
	seq          uint64
	call         string
}

var reUnitNote = regexp.MustCompile(`^([A-Z0-9][A-Z0-9-]{0,15}): `)

// statements collects what has been said about an incident: notes (a note "E1: …" from a terminal is the
// unit's), the caller's texts and call notes, and machine sources on attached calls.
func statements(s *cad.State, in *cad.Incident) []statement {
	var out []statement
	for _, e := range in.Timeline {
		if e.Kind != "note" && e.Kind != "priority-note" {
			continue
		}
		src := SrcDispatcher
		if m := reUnitNote.FindStringSubmatch(e.Text); m != nil {
			if _, ok := s.Units[m[1]]; ok {
				src = SrcUnit
			}
		}
		out = append(out, statement{text: e.Text, source: src, seq: e.Seq})
	}
	for _, id := range in.Calls {
		c, ok := s.Calls[id]
		if !ok {
			continue
		}
		machine := strings.Contains(strings.ToLower(c.Class), "alarm") || strings.Contains(strings.ToLower(c.Class), "telematic") || strings.Contains(strings.ToLower(c.Class), "acn")
		for _, t := range c.Text {
			if t.From == "caller" {
				out = append(out, statement{text: t.Text, source: SrcCaller, call: id})
			}
		}
		for _, n := range c.Notes {
			src := SrcCaller
			if machine {
				src = SrcMachine
			}
			out = append(out, statement{text: n, source: src, call: id})
		}
	}
	return out
}

var negators = []string{"no", "not", "without", "denies", "deny", "negative", "isn't", "wasn't", "there's no", "nobody", "none"}

// mention finds a phrase in text and whether it is negated in the few words before it.
func mention(text string, words []string) (phrase string, found, negated bool) {
	t := " " + strings.ToLower(text) + " "
	t = strings.NewReplacer(",", " ", ".", " ", ";", " ", "!", " ", "?", " ").Replace(t)
	for _, w := range words {
		w = strings.ToLower(w)
		i := strings.Index(t, " "+w+" ")
		if i < 0 {
			continue
		}
		before := strings.Fields(t[:i])
		if len(before) > 3 {
			before = before[len(before)-3:]
		}
		neg := false
		for _, b := range before {
			if slices.Contains(negators, b) {
				neg = true
			}
		}
		return w, true, neg
	}
	return "", false, false
}

func rank(c Codebook, src string) int {
	if i := slices.Index(c.Precedence, src); i >= 0 {
		return i
	}
	return len(c.Precedence)
}

// words judges what has been said against the incident's type: the strongest source that speaks decides;
// two sources of the same rank that disagree leave it undetermined.
func words(c Codebook, r Rule, s *cad.State, in *cad.Incident) (Suggestion, bool) {
	if slices.Contains(r.Fits, in.Type) {
		return Suggestion{}, false
	}
	var yes, no []statement
	var phrase string
	for _, st := range statements(s, in) {
		p, found, neg := mention(st.text, r.Words)
		if !found {
			continue
		}
		phrase = p
		if neg {
			no = append(no, st)
		} else {
			yes = append(yes, st)
		}
	}
	if len(yes) == 0 {
		return Suggestion{}, false
	}
	best := func(list []statement) (statement, int) {
		b, br := list[0], rank(c, list[0].source)
		for _, x := range list[1:] {
			if rk := rank(c, x.source); rk < br {
				b, br = x, rk
			}
		}
		return b, br
	}
	ev := func(st statement) Evidence {
		return Evidence{Text: "\u201c" + st.text + "\u201d", Source: st.source, Incident: in.Number, Call: st.call, Seq: st.seq}
	}
	y, yr := best(yes)
	sg := Suggestion{Rule: r.ID, Incident: in.Number}
	if len(no) > 0 {
		n, nr := best(no)
		switch {
		case nr < yr:
			return Suggestion{}, false // a stronger source denies it: nothing to suggest
		case nr == yr:
			sg.Verdict = "cannot-determine"
			sg.Title = fmt.Sprintf("%s: %s is both reported and denied", in.Number, r.Means)
			sg.Detail = fmt.Sprintf("Two %s statements disagree about %q. The codebook does not settle it: ask.", y.source, phrase)
			sg.Relied = []Evidence{ev(y), ev(n)}
			return sg, true
		default:
			sg.Relied = []Evidence{ev(y), ev(n)}
			sg.Detail = fmt.Sprintf("The %s's report outweighs the %s's denial (precedence: %s).", y.source, n.source, strings.Join(c.Precedence, " > "))
		}
	} else {
		sg.Relied = []Evidence{ev(y)}
		sg.Absent = []string{"a denial from a source of the same or higher rank"}
	}
	sg.Title = fmt.Sprintf("%s mentions %s; it is %s (%s)", in.Number, r.Means, typeName(s, in.Type), in.Type)
	if sg.Detail == "" {
		sg.Detail = fmt.Sprintf("%s said %q.", strings.ToUpper(y.source[:1])+y.source[1:], phrase)
	}
	if r.Suggest != "" && r.Suggest != in.Type {
		if _, ok := s.CallTypes[r.Suggest]; ok {
			sg.Action = &Action{Label: "Change to " + typeName(s, r.Suggest), Command: "T " + r.Suggest + " #" + in.Number}
		}
	}
	return sg, true
}

func repeats(r Rule, s *cad.State, open []*cad.Incident, now time.Time) []Suggestion {
	var out []Suggestion
	for _, in := range open {
		la, lo, ok := point(in)
		if !ok {
			continue
		}
		var prior []*cad.Incident
		for _, o := range s.Incidents {
			if o.Number == in.Number || now.Sub(o.Created) > time.Duration(r.Days)*24*time.Hour || !o.Created.Before(in.Created) {
				continue
			}
			if lb, lob, ok := point(o); ok && geo.Distance(la, lo, lb, lob) < 30 {
				prior = append(prior, o)
			}
		}
		// The earlier system's incidents count too.
		old := s.HistoryNear(la, lo, 30, now.Add(-time.Duration(r.Days)*24*time.Hour))
		if len(prior)+len(old) < r.Count {
			continue
		}
		sort.Slice(prior, func(i, j int) bool { return prior[i].Created.After(prior[j].Created) })
		sg := Suggestion{Rule: r.ID, Incident: in.Number, Title: fmt.Sprintf("%s: %d earlier incidents at this address in %d days", in.Number, len(prior)+len(old), r.Days), Since: in.Created}
		var parts []string
		for i, p := range prior {
			if i < 4 {
				parts = append(parts, fmt.Sprintf("%s %s (%s)", p.Created.Local().Format("Jan 2"), typeName(s, p.Type), firstNonEmpty(p.Disposition, p.Status)))
			}
			sg.Relied = append(sg.Relied, Evidence{Text: p.Number + " " + typeName(s, p.Type), Source: SrcDispatcher, Incident: p.Number})
		}
		for i, h := range old {
			if len(prior)+i < 4 {
				parts = append(parts, fmt.Sprintf("%s %s (%s, earlier system)", h.At.Local().Format("Jan 2 2006"), firstNonEmpty(typeName(s, h.Type), h.Type), firstNonEmpty(h.Disposition, "closed")))
			}
			sg.Relied = append(sg.Relied, Evidence{Text: h.Number + " " + h.Type + " (" + h.Source + ")", Source: SrcDispatcher})
		}
		sg.Detail = strings.Join(parts, "; ")
		out = append(out, sg)
	}
	return out
}
