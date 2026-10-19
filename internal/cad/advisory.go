package cad

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// AdvisoryState is the advisory panel's configuration as the ledger holds it: the codebook (its rules are
// read by internal/advisory) and the mode. Every version is kept, so any can be restored.
type AdvisoryState struct {
	Version int               `json:"version"`
	Mode    string            `json:"mode"` // off, shadow, live
	Rules   json.RawMessage   `json:"rules,omitempty"`
	History []CodebookVersion `json:"history,omitempty"`
}

// CodebookVersion is one saved codebook.
type CodebookVersion struct {
	Version int             `json:"version"`
	Mode    string          `json:"mode"`
	Note    string          `json:"note,omitempty"`
	By      string          `json:"by"`
	At      time.Time       `json:"at"`
	Seq     uint64          `json:"seq"`
	Rules   json.RawMessage `json:"rules,omitempty"`
}

// CodebookData is a codebook change.
type CodebookData struct {
	Mode  string          `json:"mode"`
	Rules json.RawMessage `json:"rules"`
	Note  string          `json:"note,omitempty"`
}

// SaveCodebook writes a new codebook version. The rules are checked by the caller (internal/advisory).
func (e *Engine) SaveCodebook(a Actor, d CodebookData) error {
	switch d.Mode {
	case "off", "shadow", "live":
	default:
		return problem("mode", "the mode is off, shadow or live")
	}
	if !json.Valid(d.Rules) {
		return problem("rules", "the rules are not valid JSON")
	}
	d.Note = strings.TrimSpace(d.Note)
	if len(d.Note) > 500 {
		return problem("note", "the note is too long")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := e.write(a, EvAdvisoryCodebook, refs{}, d)
	return err
}

// Replay rebuilds the state from events, calling each after every event is applied. It is for looking
// back (reports, testing a codebook against recorded incidents); the live state is never touched.
func Replay(events []ledger.Event, each func(*State, ledger.Event)) error {
	st := newState()
	for _, ev := range events {
		if err := st.apply(ev); err != nil {
			return fmt.Errorf("event %d: %w", ev.Seq, err)
		}
		if each != nil {
			each(st, ev)
		}
	}
	return nil
}

// LedgerEvents reads the whole ledger.
func (e *Engine) LedgerEvents() ([]ledger.Event, error) { return e.allEvents() }
