package cad

// The state of the features from the final round of wishes (October 2026): structured places and location
// exceptions, flags, NCIC and state requests, vehicle tows, Shift Intelligence and critical incidents.

import (
	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// EvPIPSent records a transfer to PSC PIP: the event IDs, class, modules and the answer (never the people).
const EvPIPSent = "pip.sent"

type finalState struct {
	LocExceptions map[string]*LocException    `json:"locExceptions"`
	Flags         map[string]*Flag            `json:"flags"`
	Requests      map[string]*Request         `json:"requests"`
	Followups     map[string]*TrackedFollowup `json:"followups"`
	ShiftReports  map[string][]*ShiftReport   `json:"shiftReports"` // by period ID: every version
	ShiftNotes    []ShiftNote                 `json:"shiftNotes"`
	CriticalCases map[string]*CriticalCase    `json:"criticalCases"`
	PhraseAlerts  []*PhraseAlert              `json:"phraseAlerts"`
	KeyTests      map[string]*KeyTest         `json:"keyTests"` // by workstation or console
	continuityState
	lxCount int
	rqCount int
	rtCount int
	fuCount int
	crCount int
	paCount int
}

func (s *State) initFinal() {
	s.LocExceptions, s.Flags, s.Requests = map[string]*LocException{}, map[string]*Flag{}, map[string]*Request{}
	s.Followups, s.ShiftReports, s.ShiftNotes = map[string]*TrackedFollowup{}, map[string][]*ShiftReport{}, []ShiftNote{}
	s.CriticalCases, s.PhraseAlerts, s.KeyTests = map[string]*CriticalCase{}, []*PhraseAlert{}, map[string]*KeyTest{}
	s.initContinuity()
}

func (s *State) applyFinal(e ledger.Event) (bool, error) {
	for _, f := range []func(ledger.Event) (bool, error){s.applyPlaces, s.applyFlags, s.applyRequests, s.applyTowDetail, s.applyShift, s.applyCritical, s.applyKeyTests, s.applyContinuity} {
		if ok, err := f(e); ok {
			return true, err
		}
	}
	return false, nil
}

// finalAfter is the final round's work after each event.
func (s *State) finalAfter(e ledger.Event) {
	s.criticalAfter(e)
}

func (e *Engine) knownUser(name string) bool {
	if e.KnownUser != nil {
		return e.KnownUser(name)
	}
	return e.st.Users[name] != nil
}
