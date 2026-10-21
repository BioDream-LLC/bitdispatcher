package cad

import (
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

func TestCorrectClosedIncident(t *testing.T) {
	e, _, path := testEngine(t)
	num, err := e.CreateIncident(dispatcher, NewIncident{Type: "SICK", Location: ng911.Point(40.27, -76.88)})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Dispatch(dispatcher, num, []string{"M1"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.CorrectIncident(dispatcher, num, Correction{Type: "FALL", Reason: "x"}); err == nil {
		t.Fatal("corrected an open incident")
	}
	if err := e.CloseIncident(dispatcher, num, "Handled", ""); err != nil {
		t.Fatal(err)
	}
	var created time.Time
	e.Read(func(st *State) { created = st.Incidents[num].Created })
	onScene := created.Add(7 * time.Minute)
	if _, err := e.CorrectIncident(dispatcher, num, Correction{Type: "FALL"}); err == nil {
		t.Fatal("corrected without a reason")
	}
	if _, err := e.CorrectIncident(dispatcher, num, Correction{UnitTimes: map[string]map[string]time.Time{"E1": {"onScene": onScene}}, Reason: "x"}); err == nil {
		t.Fatal("corrected a unit that was not on it")
	}
	ch, err := e.CorrectIncident(dispatcher, num, Correction{Type: "FALL", Disposition: "Transported", Address: "12 Main St",
		UnitTimes: map[string]map[string]time.Time{"M1": {"onScene": onScene}}, Reason: "M1 gave its on-scene time late by radio"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ch) != 4 {
		t.Fatalf("changes %+v", ch)
	}
	check := func(st *State) {
		t.Helper()
		in := st.Incidents[num]
		if in.Status != Closed || in.Type != "FALL" || in.Disposition != "Transported" || in.Location.Address != "12 Main St" || !in.Units[0].OnScene.Equal(onScene) {
			t.Fatalf("%+v", in)
		}
		last := in.Timeline[len(in.Timeline)-1]
		if last.Kind != "correction" || !strings.Contains(last.Text, "type SICK → FALL") || !strings.Contains(last.Text, "late by radio") {
			t.Fatalf("timeline %q", last.Text)
		}
	}
	e.Read(check)
	e.Close()
	e2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()
	e2.Read(check)
}
