package advisory

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// Service runs the codebook beside the CAD, off the critical path: it reads the state on its own
// goroutine, never holds the engine while it thinks, and core dispatch works the same without it.
type Service struct {
	Eng *cad.Engine
	Log *slog.Logger

	mu      sync.Mutex
	current map[string]Suggestion // what the rules say now
	decided map[string]Decision   // what people decided, by suggestion
	mode    string
	version int
	lastRun time.Time
	lastErr string
	runs    int
	wake    chan struct{}
	faults  []ScriptFault // scripts that failed on the last run
	// pendingWithdraw holds suggestions current before a restart, withdrawn unless the rules find them again.
	pendingWithdraw map[string]bool
}

// Decision is a person's answer to a suggestion.
type Decision struct {
	ID       string    `json:"id"`
	Rule     string    `json:"rule"`
	Accepted bool      `json:"accepted"`
	Reason   string    `json:"reason,omitempty"`
	By       string    `json:"by"`
	At       time.Time `json:"at"`
	Ran      string    `json:"ran,omitempty"` // the command run on acceptance
}

// New makes the service and reads past decisions from the ledger, so a rejected suggestion does not come
// back after a restart.
func New(eng *cad.Engine, log *slog.Logger) *Service {
	s := &Service{Eng: eng, Log: log, current: map[string]Suggestion{}, decided: map[string]Decision{}, wake: make(chan struct{}, 1)}
	if evs, err := eng.Events(0, 0, func(e ledger.Event) bool { return e.Type == cad.EvAdvisoryDecided }); err == nil {
		for _, e := range evs {
			var d Decision
			if json.Unmarshal(e.Data, &d) == nil {
				d.By, d.At = e.Actor, e.At
				s.decided[d.ID] = d
			}
		}
	}
	// What was current before a restart is not shown as current again until the rules find it afresh.
	if evs, err := eng.Events(0, 0, func(e ledger.Event) bool {
		return e.Type == cad.EvAdvisorySuggested || e.Type == cad.EvAdvisoryWithdrawn
	}); err == nil {
		open := map[string]bool{}
		for _, e := range evs {
			var v struct {
				Suggestion Suggestion `json:"suggestion"`
				ID         string     `json:"id"`
			}
			json.Unmarshal(e.Data, &v)
			if e.Type == cad.EvAdvisorySuggested {
				open[v.Suggestion.ID] = true
			} else {
				delete(open, v.ID)
			}
		}
		s.pendingWithdraw = open
	}
	return s
}

// Codebook returns the codebook in force and the mode.
func (s *Service) Codebook() (Codebook, string, int) {
	var raw json.RawMessage
	mode, version := "shadow", 0
	s.Eng.Read(func(st *cad.State) {
		raw, version = st.Advisory.Rules, st.Advisory.Version
		if st.Advisory.Mode != "" {
			mode = st.Advisory.Mode
		}
	})
	c, err := Parse(raw)
	if err != nil {
		c = Default()
	}
	return c, mode, version
}

// Run evaluates the rules whenever the CAD changes, and every few seconds for the rules about time.
func (s *Service) Run(ctx context.Context) {
	sub := s.Eng.Subscribe()
	defer s.Eng.Unsubscribe(sub)
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	s.step()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sub:
			// Several changes in a burst are one evaluation.
			time.Sleep(300 * time.Millisecond)
			for len(sub) > 0 {
				<-sub
			}
		case <-tick.C:
		case <-s.wake:
		}
		s.step()
	}
}

// Wake asks for an evaluation now (after a codebook change).
func (s *Service) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) step() {
	defer func() {
		if r := recover(); r != nil {
			s.mu.Lock()
			s.lastErr = "the rules failed: the panel is off until the next change"
			s.mu.Unlock()
			if s.Log != nil {
				s.Log.Error("advisory: panic", "err", r)
			}
		}
	}()
	c, mode, version := s.Codebook()
	var out []Suggestion
	var faults []ScriptFault
	if mode != "off" {
		now := s.Eng.Now()
		var ins []ScriptInput
		since := map[string]time.Time{}
		s.Eng.Read(func(st *cad.State) {
			out = Evaluate(c, st, now)
			ins = scriptInputs(c, st, now)
			for _, in := range ins {
				since[in.Number] = st.Incidents[in.Number].Created
			}
		})
		// Scripts run after the engine is released: a slow one never holds dispatch up.
		if len(ins) > 0 {
			sg, f := runScripts(context.Background(), c, ins, since)
			out, faults = append(out, sg...), f
		}
	}
	next := map[string]Suggestion{}
	for _, sg := range out {
		next[sg.ID] = sg
	}
	s.mu.Lock()
	var added []Suggestion
	var gone []string
	for id, sg := range next {
		if _, ok := s.current[id]; !ok {
			if _, was := s.pendingWithdraw[id]; was {
				delete(s.pendingWithdraw, id) // still holds after the restart: nothing to write
			} else {
				added = append(added, sg)
			}
		}
	}
	for id := range s.current {
		if _, ok := next[id]; !ok {
			gone = append(gone, id)
		}
	}
	for id := range s.pendingWithdraw {
		if _, ok := next[id]; !ok {
			gone = append(gone, id)
		}
	}
	s.pendingWithdraw = map[string]bool{}
	s.current, s.mode, s.version, s.lastRun, s.lastErr, s.faults = next, mode, version, time.Now(), "", faults
	s.runs++
	s.mu.Unlock()
	sort.Slice(added, func(i, j int) bool { return added[i].ID < added[j].ID })
	sort.Strings(gone)
	actor := cad.Actor{User: "advisory", Role: "system"}
	for _, sg := range added {
		s.Eng.Record(actor, cad.EvAdvisorySuggested, map[string]any{"suggestion": sg, "mode": mode, "codebook": version})
	}
	for _, id := range gone {
		s.Eng.Record(actor, cad.EvAdvisoryWithdrawn, map[string]any{"id": id})
	}
}

// Status is the panel's health, shown on every console.
type Status struct {
	Mode     string        `json:"mode"`
	Version  int           `json:"version"`
	LastRun  time.Time     `json:"lastRun,omitzero"`
	Error    string        `json:"error,omitempty"`
	Current  int           `json:"current"`
	Degraded bool          `json:"degraded"` // not evaluated recently
	Faults   []ScriptFault `json:"faults,omitempty"`
}

// Status reports the panel's state.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Status{Mode: s.mode, Version: s.version, LastRun: s.lastRun, Error: s.lastErr, Current: len(s.current), Faults: s.faults,
		Degraded: s.lastErr != "" || (s.mode != "off" && !s.lastRun.IsZero() && time.Since(s.lastRun) > 30*time.Second)}
}

// Visible is what a dispatcher sees: in live mode, current suggestions nobody has decided. In shadow mode
// only supervisors see them (shadow true), so the rules can be judged before anyone relies on them.
func (s *Service) Visible(shadow bool) []Suggestion {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mode == "off" || (s.mode == "shadow" && !shadow) {
		return []Suggestion{}
	}
	out := []Suggestion{}
	for id, sg := range s.current {
		if _, ok := s.decided[id]; !ok {
			out = append(out, sg)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity == "urgent"
		}
		return out[i].Since.Before(out[j].Since)
	})
	return out
}

// Decisions lists recent decisions, newest first.
func (s *Service) Decisions(n int) []Decision {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Decision, 0, len(s.decided))
	for _, d := range s.decided {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// ErrGone means the suggestion no longer holds.
var ErrGone = errors.New("this suggestion no longer holds")

// Decide records a person's answer. With run, accepting also runs the suggestion's command as that person:
// one click, still their decision.
func (s *Service) Decide(a cad.Actor, id string, accept, run bool, reason string) (Decision, cad.CommandResult, error) {
	s.mu.Lock()
	sg, ok := s.current[id]
	s.mu.Unlock()
	if !ok {
		return Decision{}, cad.CommandResult{}, ErrGone
	}
	d := Decision{ID: id, Rule: sg.Rule, Accepted: accept, Reason: reason, By: a.User, At: s.Eng.Now()}
	var res cad.CommandResult
	if accept && run && sg.Action != nil {
		res = s.Eng.Execute(a, sg.Action.Command, sg.Incident)
		if !res.OK {
			return d, res, errors.New(res.Message)
		}
		d.Ran = sg.Action.Command
	}
	if err := s.Eng.Record(a, cad.EvAdvisoryDecided, map[string]any{"id": id, "rule": sg.Rule, "accepted": accept, "reason": reason, "ran": d.Ran, "suggestion": sg}); err != nil {
		return d, res, err
	}
	s.mu.Lock()
	s.decided[id] = d
	s.mu.Unlock()
	return d, res, nil
}

// TestResult is what a codebook would have said over recorded incidents.
type TestResult struct {
	From        time.Time      `json:"from"`
	To          time.Time      `json:"to"`
	Events      int            `json:"events"`
	Evaluations int            `json:"evaluations"`
	ByRule      map[string]int `json:"byRule"`
	Undecided   int            `json:"cannotDetermine"`
	Sample      []Suggestion   `json:"sample"`
	Millis      int64          `json:"millis"`
	Faults      []ScriptFault  `json:"faults,omitempty"` // scripts that failed during the replay
}

// Test replays the ledger and applies a codebook after each incident or call event in the period, as if
// it had been live: each distinct suggestion is counted once, when it first appeared.
func (s *Service) Test(c Codebook, from, to time.Time) (TestResult, error) {
	t0 := time.Now()
	r := TestResult{From: from, To: to, ByRule: map[string]int{}, Sample: []Suggestion{}}
	events, err := s.Eng.LedgerEvents()
	if err != nil {
		return r, err
	}
	seen := map[string]bool{}
	err = cad.Replay(events, func(st *cad.State, ev ledger.Event) {
		if ev.At.Before(from) || !ev.At.Before(to) {
			return
		}
		r.Events++
		if ev.Incident == "" && ev.Call == "" {
			return
		}
		r.Evaluations++
		found := Evaluate(c, st, ev.At)
		if ins := scriptInputs(c, st, ev.At); len(ins) > 0 {
			since := map[string]time.Time{}
			for _, in := range ins {
				since[in.Number] = st.Incidents[in.Number].Created
			}
			sg, faults := runScripts(context.Background(), c, ins, since)
			found = append(found, sg...)
			for _, f := range faults {
				if len(r.Faults) < 20 {
					r.Faults = append(r.Faults, f)
				}
			}
		}
		for _, sg := range found {
			if seen[sg.ID] {
				continue
			}
			seen[sg.ID] = true
			r.ByRule[sg.Rule]++
			if sg.Verdict == "cannot-determine" {
				r.Undecided++
			}
			if len(r.Sample) < 100 {
				r.Sample = append(r.Sample, sg)
			}
		}
	})
	r.Millis = time.Since(t0).Milliseconds()
	return r, err
}
