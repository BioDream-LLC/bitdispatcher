package cad

// Workstation key tests: before a position goes into service, someone presses each key the agency's
// profile publishes for the duty and the browser says which ones reached bitdispatcher. A laptop that
// sends F2 to its volume control or F4 to its microphone shows up here, with the combination that works
// (Fn plus the key, or the agency's alternate), instead of at the first emergency call.

import (
	"fmt"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// EvKeyTest records a workstation's key test.
const EvKeyTest = "keys.tested"

// KeyTest is one workstation's key test.
type KeyTest struct {
	Console  string          `json:"console"`
	Duty     string          `json:"duty"`
	Profile  string          `json:"profile"`
	Browser  string          `json:"browser,omitempty"`
	Platform string          `json:"platform,omitempty"`
	Results  []KeyTestResult `json:"results"`
	Passed   bool            `json:"passed"`
	At       time.Time       `json:"at"`
	By       string          `json:"by"`
}

// KeyTestResult is one key's result.
type KeyTestResult struct {
	Key    string `json:"key"`
	Action string `json:"action"`
	Got    string `json:"got,omitempty"` // what arrived instead (Fn+F2 arriving as F2 is fine)
	OK     bool   `json:"ok"`
	Fix    string `json:"fix,omitempty"` // what to use instead
}

// RecordKeyTest keeps a workstation's key test.
func (e *Engine) RecordKeyTest(a Actor, t KeyTest) error {
	if len(t.Results) == 0 {
		return problem("results", "press the keys first")
	}
	t.Console = firstNonEmpty(strings.TrimSpace(t.Console), a.Console)
	if t.Console == "" {
		return problem("console", "the position is not known")
	}
	t.Passed = true
	for _, r := range t.Results {
		t.Passed = t.Passed && r.OK
	}
	t.Duty = firstNonEmpty(t.Duty, DutyOf(a))
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := e.write(a, EvKeyTest, refs{}, t)
	return err
}

func (s *State) applyKeyTests(e ledger.Event) (bool, error) {
	if e.Type != EvKeyTest {
		return false, nil
	}
	t, err := decode[KeyTest](e)
	if err != nil {
		return true, err
	}
	t.At, t.By = e.At, e.Actor
	s.KeyTests[t.Console+"|"+t.Duty] = &t
	return true, nil
}

// KeyTestSummary is a key test in one line.
func (t *KeyTest) Summary() string {
	bad := 0
	for _, r := range t.Results {
		if !r.OK {
			bad++
		}
	}
	if bad == 0 {
		return fmt.Sprintf("all %d keys reach bitdispatcher", len(t.Results))
	}
	return fmt.Sprintf("%d of %d keys did not reach bitdispatcher", bad, len(t.Results))
}
