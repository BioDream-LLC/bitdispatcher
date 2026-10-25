package cad

// Self-service setup: the steps an agency ticks by hand on its setup checklist, the last go-live check,
// and a dry-run recommendation the go-live check uses. The checklist itself is worked out from the
// state by the API (api/setup.go); only what a person confirms is kept here.

import (
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

const (
	EvSetupTicked   = "setup.ticked"   // a person marked a setup step done, or not needed
	EvGoLiveChecked = "golive.checked" // the go-live check ran
	EvSupportBundle = "support.bundle" // an administrator downloaded a support bundle
)

// SetupTick is a step a person confirmed: done (training given, the parallel run finished) or not needed.
type SetupTick struct {
	Step string    `json:"step"`
	Done bool      `json:"done"`
	Note string    `json:"note,omitempty"`
	By   string    `json:"by,omitempty"`
	At   time.Time `json:"at,omitzero"`
}

// GoLiveItem is one check of the go-live check: pass, warn or fail.
type GoLiveItem struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Result string `json:"result"` // pass, warn, fail
	Detail string `json:"detail,omitempty"`
	Fix    string `json:"fix,omitempty"` // the admin page that fixes it
}

// GoLiveResult is a run of the go-live check.
type GoLiveResult struct {
	At     time.Time    `json:"at"`
	By     string       `json:"by"`
	Pass   int          `json:"pass"`
	Warn   int          `json:"warn"`
	Fail   int          `json:"fail"`
	Checks []GoLiveItem `json:"checks"`
}

// Ready is true when nothing failed.
func (g GoLiveResult) Ready() bool { return g.Fail == 0 && len(g.Checks) > 0 }

type setupState struct {
	SetupTicks map[string]SetupTick `json:"setupTicks"`
	GoLive     *GoLiveResult        `json:"goLive,omitempty"`
}

// TickSetup records that a person confirmed a setup step (done true) or took the confirmation back.
func (e *Engine) TickSetup(a Actor, step string, done bool, note string) error {
	step = strings.TrimSpace(step)
	if step == "" || len(step) > 40 {
		return problem("step", "which step?")
	}
	if len(note) > 500 {
		return problem("note", "the note is too long (500 characters at most)")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := e.write(a, EvSetupTicked, refs{}, SetupTick{Step: step, Done: done, Note: strings.TrimSpace(note)})
	return err
}

// SaveGoLive records a run of the go-live check.
func (e *Engine) SaveGoLive(a Actor, g GoLiveResult) error {
	g.By = a.User
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := e.write(a, EvGoLiveChecked, refs{}, g)
	return err
}

// TestCall is one dry-run call for the go-live check: a call type at a place.
type TestCall struct {
	Type string
	Lat  float64
	Lon  float64
}

// TryRecommend works out the recommendation for calls that are never made: nothing is written to the
// ledger and no unit is touched. Calls at the same place share one road search.
func (e *Engine) TryRecommend(calls []TestCall) []Recommendation {
	e.mu.Lock()
	r := e.router
	e.mu.Unlock()
	out := make([]Recommendation, 0, len(calls))
	var tf TravelFunc
	label := ""
	var at [2]float64
	for i, c := range calls {
		if r != nil && (i == 0 || at != [2]float64{c.Lat, c.Lon}) {
			tf, label = r.TravelTo(c.Lat, c.Lon)
			at = [2]float64{c.Lat, c.Lon}
		}
		lat, lon := c.Lat, c.Lon
		e.mu.Lock()
		const number = "GO-LIVE-TEST"
		e.st.Incidents[number] = &Incident{Number: number, Type: c.Type, Status: "pending",
			Location: &ng911.Location{Lat: &lat, Lon: &lon, Address: "go-live test"}, LocVerified: true, Calls: []string{}, Units: []Assignment{}}
		rec, err := e.st.recommend(number, e.now(), tf, label)
		delete(e.st.Incidents, number)
		e.mu.Unlock()
		if err != nil {
			rec = Recommendation{Type: c.Type, Warnings: []string{err.Error()}}
		}
		out = append(out, rec)
	}
	return out
}

func (s *State) applySetup(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvSetupTicked:
		t, err := decode[SetupTick](e)
		if err != nil {
			return true, err
		}
		if !t.Done {
			delete(s.SetupTicks, t.Step)
			return true, nil
		}
		t.By, t.At = e.Actor, e.At
		s.SetupTicks[t.Step] = t
	case EvGoLiveChecked:
		g, err := decode[GoLiveResult](e)
		if err != nil {
			return true, err
		}
		g.At = e.At
		s.GoLive = &g
	case EvSupportBundle:
	default:
		return false, nil
	}
	return true, nil
}
