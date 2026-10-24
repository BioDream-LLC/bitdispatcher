package cad

// The roadmap features' shared plumbing: applying their events, the work done after each event, and the
// record of patient-care reports sent to an ePCR system (the hand-off itself is internal/nemsis).

import (
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// EvEPCRSent records an EMS unit's response sent to the patient-care report system.
const EvEPCRSent = "epcr.sent"

// EPCRRecord is what the patient-care report system last said about a unit's response.
type EPCRRecord struct {
	Unit   string    `json:"unit"`
	At     time.Time `json:"at"`
	OK     bool      `json:"ok"`
	Status int       `json:"status,omitempty"`
	Error  string    `json:"error,omitempty"`
	By     string    `json:"by"`
}

// RecordEPCR keeps what the patient-care report system answered for a unit on an incident.
func (e *Engine) RecordEPCR(a Actor, number string, r EPCRRecord) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.Incidents[number] == nil {
		return problem("incident", "no incident %s", number)
	}
	_, err := e.write(a, EvEPCRSent, refs{Incident: number, Unit: r.Unit}, r)
	return err
}

func (s *State) applyRoadmap(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvIncAlarm:
		return true, s.applyAlarm(e)
	case EvEPCRSent:
		r, err := decode[EPCRRecord](e)
		if err != nil {
			return true, err
		}
		r.At, r.By = e.At, e.Actor
		if in := s.Incidents[e.Incident]; in != nil {
			if in.EPCR == nil {
				in.EPCR = map[string]*EPCRRecord{}
			}
			in.EPCR[r.Unit] = &r
			if r.OK {
				in.add(e, "epcr", r.Unit+" sent to the patient-care report system")
			} else {
				in.add(e, "epcr", r.Unit+" not taken by the patient-care report system: "+r.Error)
			}
		}
		return true, nil
	}
	for _, f := range []func(ledger.Event) (bool, error){s.applyCrew, s.applyFireground, s.applyPages, s.applyTows, s.applySchedule, s.applyDirectory, s.applyGeofences, s.applyPapers, s.applyAlerts} {
		if ok, err := f(e); ok {
			return true, err
		}
	}
	return false, nil
}

// roadmapAfter is the roadmap features' work after each event.
func (s *State) roadmapAfter(e ledger.Event) {
	s.fireAfter(e)
	s.fenceAfter(e)
}

// initRoadmap makes the roadmap features' maps.
func (s *State) initRoadmap() {
	s.Members, s.PageGroups, s.TowCompanies = map[string]*Member{}, map[string]*PageGroup{}, map[string]*TowCompany{}
	s.Scheduled, s.Directory, s.OnCall = map[string]*Scheduled{}, map[string]*DirEntry{}, map[string]*OnCallList{}
	s.Geofences, s.Papers, s.inFence = map[string]*Geofence{}, map[string]*Paper{}, map[string][]string{}
	s.AlertLists, s.Alerts = map[string]*AlertList{}, []*PublicAlert{}
}
