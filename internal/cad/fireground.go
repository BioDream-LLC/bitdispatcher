package cad

// Fire-ground accountability: at a working fire the incident commander takes a roll call (a PAR, a
// personnel accountability report) of every crew on scene at set intervals, and dispatch keeps the clock:
// it says when a roll call is due, records who answered and who did not, and keeps the commander's
// benchmarks (primary search complete, fire under control). A call type marked for it starts the clock
// when the first unit arrives; a dispatcher can start it on any incident.

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvFiregroundStarted   = "fireground.started"
	EvFiregroundPAR       = "fireground.par"       // a roll call: who is accounted for, who is not
	EvFiregroundFound     = "fireground.found"     // a crew missing at a roll call is accounted for
	EvFiregroundBenchmark = "fireground.benchmark" // the commander reported a benchmark
	EvFiregroundEnded     = "fireground.ended"
)

// Fireground is an incident's accountability clock and record.
type Fireground struct {
	Started    time.Time       `json:"started"`
	StartedBy  string          `json:"startedBy"`
	Auto       bool            `json:"auto,omitempty"` // started by the first unit's arrival
	Minutes    int             `json:"minutes"`        // between roll calls
	LastPAR    time.Time       `json:"lastPar,omitzero"`
	Checks     []PARCheck      `json:"checks"`
	Missing    []string        `json:"missing,omitempty"` // units not accounted for at the last roll call, until found
	Benchmarks []BenchmarkMark `json:"benchmarks"`
	Ended      time.Time       `json:"ended,omitzero"`
}

// PARCheck is one roll call.
type PARCheck struct {
	At      time.Time      `json:"at"`
	By      string         `json:"by"`
	OK      []string       `json:"ok"`
	Missing []string       `json:"missing,omitempty"`
	PAX     map[string]int `json:"pax,omitempty"` // how many each crew reported
	Note    string         `json:"note,omitempty"`
}

// Benchmark is a fire-ground benchmark the agency uses.
type Benchmark struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// BenchmarkMark is a benchmark reported on an incident.
type BenchmarkMark struct {
	Code string    `json:"code"`
	Name string    `json:"name"`
	At   time.Time `json:"at"`
	By   string    `json:"by"`
}

// DefaultBenchmarks are the usual fire-ground benchmarks.
func DefaultBenchmarks() []Benchmark {
	return []Benchmark{
		{"PS", "Primary search complete"}, {"SS", "Secondary search complete"}, {"UC", "Fire under control"},
		{"LS", "Loss stopped"}, {"KD", "Knockdown"}, {"UT", "Utilities secured"}, {"OH", "Overhaul complete"},
	}
}

// BenchmarkList is the agency's benchmarks, or the defaults.
func (s Settings) BenchmarkList() []Benchmark {
	if len(s.Benchmarks) == 0 {
		return DefaultBenchmarks()
	}
	return s.Benchmarks
}

// PARInterval is the minutes between roll calls for an incident's type.
func (s *State) PARInterval(typ string) int {
	if ct, ok := s.CallTypes[typ]; ok && ct.PARMinutes > 0 {
		return ct.PARMinutes
	}
	if s.Settings.PARMinutes > 0 {
		return s.Settings.PARMinutes
	}
	return 20
}

// Due is when the next roll call is due, and whether the clock is running.
func (f *Fireground) Due() (time.Time, bool) {
	if f == nil || !f.Ended.IsZero() {
		return time.Time{}, false
	}
	from := f.Started
	if f.LastPAR.After(from) {
		from = f.LastPAR
	}
	return from.Add(time.Duration(f.Minutes) * time.Minute), true
}

type fgStart struct {
	Minutes int  `json:"minutes"`
	Auto    bool `json:"auto,omitempty"`
}

type fgPAR struct {
	OK      []string       `json:"ok"`
	Missing []string       `json:"missing,omitempty"`
	PAX     map[string]int `json:"pax,omitempty"`
	Note    string         `json:"note,omitempty"`
}

type fgUnit struct {
	Unit string `json:"unit"`
	Note string `json:"note,omitempty"`
}

// OnSceneUnits are the units at an incident's scene now: arrived and not cleared.
func (in *Incident) OnSceneUnits() []string {
	var out []string
	for _, a := range in.Units {
		if a.Cleared.IsZero() && !a.OnScene.IsZero() && !slices.Contains(out, a.Unit) {
			out = append(out, a.Unit)
		}
	}
	return out
}

// StartFireground starts the accountability clock on an incident (minutes 0: the call type's interval).
func (e *Engine) StartFireground(a Actor, number string, minutes int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	in, err := e.openIncident(number)
	if err != nil {
		return err
	}
	if in.Fireground != nil && in.Fireground.Ended.IsZero() {
		if minutes > 0 && minutes != in.Fireground.Minutes {
			_, err := e.write(a, EvFiregroundStarted, refs{Incident: number}, fgStart{Minutes: minutes})
			return err
		}
		return problem("incident", "the accountability clock is already running on %s", number)
	}
	if minutes == 0 {
		minutes = e.st.PARInterval(in.Type)
	}
	if minutes < 1 || minutes > 60 {
		return problem("minutes", "roll calls are 1 to 60 minutes apart")
	}
	_, err = e.write(a, EvFiregroundStarted, refs{Incident: number}, fgStart{Minutes: minutes})
	return err
}

// RecordPAR records a roll call. ok are the crews that answered, missing those that did not; units on
// scene in neither list are taken as accounted for when missing is given, and as not yet reported
// otherwise. With neither list, every unit on scene answered.
func (e *Engine) RecordPAR(a Actor, number string, ok, missing []string, pax map[string]int, note string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	in, err := e.openIncident(number)
	if err != nil {
		return err
	}
	scene := in.OnSceneUnits()
	if len(scene) == 0 {
		return problem("incident", "no unit is on scene at %s", number)
	}
	on := func(u string) bool {
		return slices.ContainsFunc(in.Units, func(x Assignment) bool { return x.Unit == u && x.Cleared.IsZero() })
	}
	norm := func(us []string) ([]string, error) {
		var out []string
		for _, u := range us {
			u = strings.ToUpper(strings.TrimSpace(u))
			if u == "" {
				continue
			}
			if !on(u) {
				return nil, problem("units", "%s is not on %s", u, number)
			}
			if !slices.Contains(out, u) {
				out = append(out, u)
			}
		}
		return out, nil
	}
	if ok, err = norm(ok); err != nil {
		return err
	}
	if missing, err = norm(missing); err != nil {
		return err
	}
	for _, u := range missing {
		if slices.Contains(ok, u) {
			return problem("units", "%s cannot be both accounted for and missing", u)
		}
	}
	if len(ok) == 0 || len(missing) > 0 {
		for _, u := range scene {
			if !slices.Contains(missing, u) && !slices.Contains(ok, u) {
				ok = append(ok, u)
			}
		}
	}
	if in.Fireground == nil || !in.Fireground.Ended.IsZero() {
		if _, err := e.write(a, EvFiregroundStarted, refs{Incident: number}, fgStart{Minutes: e.st.PARInterval(in.Type)}); err != nil {
			return err
		}
	}
	for u, n := range pax {
		if n < 0 || n > 50 {
			return problem("pax", "%s: a crew count is 0 to 50", u)
		}
	}
	_, err = e.write(a, EvFiregroundPAR, refs{Incident: number}, fgPAR{OK: ok, Missing: missing, PAX: pax, Note: strings.TrimSpace(note)})
	return err
}

// FoundUnit records that a crew missing at a roll call is accounted for.
func (e *Engine) FoundUnit(a Actor, number, unit, note string) error {
	unit = strings.ToUpper(unit)
	e.mu.Lock()
	defer e.mu.Unlock()
	in, err := e.openIncident(number)
	if err != nil {
		return err
	}
	if in.Fireground == nil || !slices.Contains(in.Fireground.Missing, unit) {
		return problem("unit", "%s is not missing on %s", unit, number)
	}
	_, err = e.write(a, EvFiregroundFound, refs{Incident: number, Unit: unit}, fgUnit{Unit: unit, Note: strings.TrimSpace(note)})
	return err
}

// MarkBenchmark records a benchmark the commander reported, by code or name.
func (e *Engine) MarkBenchmark(a Actor, number, code string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	in, err := e.openIncident(number)
	if err != nil {
		return err
	}
	var b *Benchmark
	for _, x := range e.st.Settings.BenchmarkList() {
		if strings.EqualFold(x.Code, code) || strings.EqualFold(x.Name, code) {
			b = &x
			break
		}
	}
	if b == nil {
		var codes []string
		for _, x := range e.st.Settings.BenchmarkList() {
			codes = append(codes, x.Code+" "+x.Name)
		}
		return problem("code", "no benchmark %s; the agency's are %s", code, strings.Join(codes, ", "))
	}
	if in.Fireground != nil && slices.ContainsFunc(in.Fireground.Benchmarks, func(m BenchmarkMark) bool { return m.Code == b.Code }) {
		return problem("code", "%s is already reported on %s", b.Name, number)
	}
	_, err = e.write(a, EvFiregroundBenchmark, refs{Incident: number}, Benchmark{Code: b.Code, Name: b.Name})
	return err
}

// EndFireground stops the accountability clock (the fire is out and crews are leaving).
func (e *Engine) EndFireground(a Actor, number string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	in, ok := e.st.Incidents[number]
	if !ok {
		return problem("incident", "no incident %s", number)
	}
	if in.Fireground == nil || !in.Fireground.Ended.IsZero() {
		return problem("incident", "no accountability clock is running on %s", number)
	}
	if len(in.Fireground.Missing) > 0 {
		return problem("incident", "%s is still not accounted for", strings.Join(in.Fireground.Missing, ", "))
	}
	_, err := e.write(a, EvFiregroundEnded, refs{Incident: number}, nil)
	return err
}

func (s *State) applyFireground(e ledger.Event) (bool, error) {
	in := s.Incidents[e.Incident]
	switch e.Type {
	case EvFiregroundStarted:
		v, err := decode[fgStart](e)
		if err != nil || in == nil {
			return true, err
		}
		if in.Fireground != nil && in.Fireground.Ended.IsZero() {
			in.Fireground.Minutes = v.Minutes
			in.add(e, "par", fmt.Sprintf("Roll calls every %d minutes", v.Minutes))
			return true, nil
		}
		in.Fireground = &Fireground{Started: e.At, StartedBy: e.Actor, Auto: v.Auto, Minutes: v.Minutes, Checks: []PARCheck{}, Benchmarks: []BenchmarkMark{}}
		in.add(e, "par", fmt.Sprintf("Accountability clock started: roll call (PAR) every %d minutes", v.Minutes))
	case EvFiregroundPAR:
		v, err := decode[fgPAR](e)
		if err != nil || in == nil || in.Fireground == nil {
			return true, err
		}
		f := in.Fireground
		f.Checks = append(f.Checks, PARCheck{At: e.At, By: e.Actor, OK: v.OK, Missing: v.Missing, PAX: v.PAX, Note: v.Note})
		f.Missing = append([]string(nil), v.Missing...)
		parts := []string{}
		for _, u := range v.OK {
			if n := v.PAX[u]; n > 0 {
				parts = append(parts, fmt.Sprintf("%s (%d)", u, n))
			} else {
				parts = append(parts, u)
			}
		}
		if len(v.Missing) == 0 {
			f.LastPAR = e.At
			in.add(e, "par", "PAR: all accounted for — "+strings.Join(parts, ", ")+suffix(v.Note))
		} else {
			text := "PAR: NOT ACCOUNTED FOR " + strings.Join(v.Missing, ", ")
			if len(parts) > 0 {
				text += "; accounted for " + strings.Join(parts, ", ")
			}
			in.Timeline = append(in.Timeline, Entry{Seq: e.Seq, At: e.At, Actor: e.Actor, Kind: "par", Text: text + suffix(v.Note), Priority: true})
			in.Updated = e.At
		}
	case EvFiregroundFound:
		v, err := decode[fgUnit](e)
		if err != nil || in == nil || in.Fireground == nil {
			return true, err
		}
		f := in.Fireground
		f.Missing = slices.DeleteFunc(f.Missing, func(u string) bool { return u == v.Unit })
		text := v.Unit + " accounted for" + suffix(v.Note)
		if len(f.Missing) == 0 {
			f.LastPAR = e.At
			text += ": all accounted for"
		}
		in.Timeline = append(in.Timeline, Entry{Seq: e.Seq, At: e.At, Actor: e.Actor, Kind: "par", Text: text, Priority: true, Unit: v.Unit})
		in.Updated = e.At
	case EvFiregroundBenchmark:
		v, err := decode[Benchmark](e)
		if err != nil || in == nil {
			return true, err
		}
		if in.Fireground == nil {
			in.Fireground = &Fireground{Started: e.At, StartedBy: e.Actor, Minutes: s.PARInterval(in.Type), Checks: []PARCheck{}, Benchmarks: []BenchmarkMark{}, Ended: e.At}
		}
		in.Fireground.Benchmarks = append(in.Fireground.Benchmarks, BenchmarkMark{Code: v.Code, Name: v.Name, At: e.At, By: e.Actor})
		in.add(e, "benchmark", fmt.Sprintf("Benchmark: %s (%s elapsed)", v.Name, elapsed(e.At.Sub(in.Created))))
	case EvFiregroundEnded:
		if in != nil && in.Fireground != nil {
			in.Fireground.Ended = e.At
			in.add(e, "par", "Accountability clock stopped")
		}
	default:
		return false, nil
	}
	return true, nil
}

// fireAfter starts the accountability clock when the first unit arrives at an incident whose call type
// asks for it, and stops it when the incident closes. It runs as each event is applied, so a replay
// arrives at the same clock.
func (s *State) fireAfter(e ledger.Event) {
	in := s.Incidents[e.Incident]
	if in == nil {
		return
	}
	switch e.Type {
	case EvUnitStatus:
		ct := s.CallTypes[in.Type]
		if ct == nil || !ct.PAR || in.Fireground != nil || in.Status == Closed || len(in.OnSceneUnits()) == 0 {
			return
		}
		m := s.PARInterval(in.Type)
		in.Fireground = &Fireground{Started: e.At, StartedBy: "system", Auto: true, Minutes: m, Checks: []PARCheck{}, Benchmarks: []BenchmarkMark{}}
		in.Timeline = append(in.Timeline, Entry{Seq: e.Seq, At: e.At, Actor: "system", Kind: "par", Text: fmt.Sprintf("First unit on scene: accountability clock started, roll call (PAR) every %d minutes", m)})
	case EvIncClosed:
		if in.Fireground != nil && in.Fireground.Ended.IsZero() {
			in.Fireground.Ended = e.At
		}
	}
}

func elapsed(d time.Duration) string {
	m := int(d.Minutes())
	if m < 60 {
		return fmt.Sprintf("%d min", m)
	}
	return fmt.Sprintf("%dh%02d", m/60, m%60)
}
