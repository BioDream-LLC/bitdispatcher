package cad

// The state of the features added after the competitor review (October 2026): cameras near an incident,
// links that let a caller share their location, and calls answered by a non-emergency intake. Road
// closures are in closures.go.

import (
	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

type nextState struct {
	Cameras     map[string]*Camera       `json:"cameras"`
	CameraLooks []CameraLook             `json:"cameraLooks"`
	LocLinks    map[string]*LocationLink `json:"locationLinks"` // by call
	setupState
}

func (s *State) initNext() {
	s.Closures = map[string]*RoadClosure{}
	s.Cameras, s.CameraLooks, s.LocLinks = map[string]*Camera{}, []CameraLook{}, map[string]*LocationLink{}
	s.SetupTicks = map[string]SetupTick{}
	s.initFinal()
}

func (s *State) applyNext(e ledger.Event) (bool, error) {
	for _, f := range []func(ledger.Event) (bool, error){s.applyCameras, s.applyLocLinks, s.applySetup, s.applyFinal} {
		if ok, err := f(e); ok {
			return true, err
		}
	}
	return false, nil
}
