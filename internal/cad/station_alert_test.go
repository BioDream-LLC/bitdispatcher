package cad

import (
	"strings"
	"testing"

	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

func TestStationAlerts(t *testing.T) {
	e, _, _ := testEngine(t)
	loc := ng911.Point(40.27, -76.88)
	loc.Address = "215 Church Ave SW, Roanoke VA"
	num, err := e.CreateIncident(dispatcher, NewIncident{Type: "SF", Location: loc})
	if err != nil {
		t.Fatal(err)
	}
	ids, err := e.AlertStations(dispatcher, num, []string{"E1", "M1", "E2", "P1"}, false)
	if err != nil || len(ids) != 2 {
		t.Fatalf("%v %v", ids, err)
	}
	var st1 *StationAlert
	e.Read(func(s *State) { st1 = s.stationAlert(ids[0]) })
	if st1.Station != "ST1" || strings.Join(st1.Units, ",") != "E1,M1" {
		t.Fatalf("%+v", st1)
	}
	if !strings.HasPrefix(st1.Speech, "Engine 1, Medic 1. ") || !strings.Contains(st1.Speech, "215 Church Avenue Southwest.") || strings.Contains(st1.Speech, "Roanoke") {
		t.Fatalf("speech %q", st1.Speech)
	}
	if err := e.AckStationAlert(Actor{User: "st1"}, ids[0]); err != nil {
		t.Fatal(err)
	}
	if err := e.Dispatch(dispatcher, num, []string{"L1"}, nil); err != nil {
		t.Fatal(err)
	}
	if r := e.Execute(dispatcher, "TONE", num); !r.OK {
		t.Fatal(r.Message)
	}
	e.Read(func(s *State) {
		last := s.StationAlerts[len(s.StationAlerts)-1]
		if !last.Again || last.Station != "ST1" || s.stationAlert(ids[0]).AckedBy != "st1" {
			t.Fatalf("%+v", last)
		}
	})
}
