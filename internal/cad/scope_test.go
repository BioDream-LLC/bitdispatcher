package cad

import (
	"testing"
	"time"
)

// A mark-out past its limit is reported once, survives a restart as reported, and a new mark-out starts again.
func TestMarkOutOverdue(t *testing.T) {
	e, c, path := testEngine(t)
	var set Settings
	e.Read(func(s *State) { set = s.Settings })
	set.MarkOuts = []MarkOutType{{Code: "MEAL", Name: "Meal", Minutes: 0, MaxMinutes: 45}, {Code: "COURT", Name: "Court"}}
	must(t, e.SaveSettings(SystemActor, set))
	must(t, e.MarkOutUnit(SystemActor, "P1", "MEAL", nil, ""))
	must(t, e.MarkOutUnit(SystemActor, "M1", "COURT", nil, ""))
	c.t = c.t.Add(44 * time.Minute)
	if over, _ := e.WarnMarkOuts(); len(over) != 0 {
		t.Fatalf("warned early: %+v", over)
	}
	c.t = c.t.Add(2 * time.Minute)
	over, err := e.WarnMarkOuts()
	must(t, err)
	if len(over) != 1 || over[0].Unit != "P1" || over[0].Max != 45 || over[0].Minutes != 46 {
		t.Fatalf("overdue %+v", over)
	}
	if again, _ := e.WarnMarkOuts(); len(again) != 0 {
		t.Fatalf("warned twice: %+v", again)
	}
	e.Close()
	e2, err := Open(path)
	must(t, err)
	defer e2.Close()
	e2.now = c.now
	if again, _ := e2.WarnMarkOuts(); len(again) != 0 {
		t.Fatalf("warned again after a restart: %+v", again)
	}
	var m int
	e2.Read(func(s *State) { m, _ = s.MarkOutOverdue(s.Units["P1"], c.t) })
	if m != 46 {
		t.Fatalf("board overdue %d", m)
	}
	// Back in service and out again: a new mark-out is a new limit.
	must(t, e2.SetStatus(SystemActor, "P1", Available, "", ""))
	must(t, e2.MarkOutUnit(SystemActor, "P1", "MEAL", nil, ""))
	c.t = c.t.Add(50 * time.Minute)
	if over, _ := e2.WarnMarkOuts(); len(over) != 1 {
		t.Fatalf("the second mark-out was not reported: %+v", over)
	}
}

// The board's scope: a person limited to an agency sees its incidents and units, units of other agencies on
// them, incidents without an agency, and what they were given to watch.
func TestBoardScope(t *testing.T) {
	e, _, _ := testEngine(t)
	must(t, e.SaveAgency(SystemActor, Agency{Code: "RPD", Name: "City police", Disciplines: []string{"police"}}))
	must(t, e.SaveAgency(SystemActor, Agency{Code: "RCPD", Name: "County police", Disciplines: []string{"police"}}))
	must(t, e.SaveAgency(SystemActor, Agency{Code: "RFD", Name: "City fire", Disciplines: []string{"fire"}}))
	var u UnitDef
	e.Read(func(s *State) { u = s.Units["P1"].UnitDef })
	u.Agency = "RCPD"
	must(t, e.SaveUnit(SystemActor, u))
	must(t, e.SaveStaff(SystemActor, Staff{Username: "tim", Agencies: []string{"RPD"}, Home: "RPD", Monitor: []string{"RFD"}}))
	tim := Actor{User: "tim", Role: "dispatcher"}
	e.Read(func(s *State) {
		sc := s.AccessScope("tim", "", "")
		if sc == nil || !sc["RPD"] || !sc["RFD"] || sc["RCPD"] {
			t.Fatalf("scope %v", sc)
		}
		if s.UnitIn(sc, s.Units["P1"]) || !s.UnitIn(sc, s.Units["E1"]) {
			t.Fatal("unit scope")
		}
		if s.AccessScope("someone", "", "") != nil {
			t.Fatal("a person with no agencies given should see the whole board")
		}
	})
	if err := e.SetCoverage(tim, WorkCoverage{Working: "RPD", Monitored: []string{"RCPD"}}); err == nil {
		t.Fatal("watched an agency not given")
	}
	must(t, e.SetCoverage(tim, WorkCoverage{Working: "RPD", Monitored: []string{"RFD"}, Scope: ScopeCoverage}))
	if err := e.SetCoverage(tim, WorkCoverage{Working: "RPD", Scope: "all-of-it"}); err == nil {
		t.Fatal("an unknown scope was taken")
	}
	n, err := e.CreateIncident(SystemActor, NewIncident{Type: "CHEST"})
	must(t, err)
	must(t, e.SetIncidentAgency(SystemActor, n, "RCPD", nil, "county call"))
	e.Read(func(s *State) {
		sc := s.AccessScope("tim", "", "")
		if s.IncidentIn(sc, s.Incidents[n]) {
			t.Fatal("tim sees a county incident")
		}
	})
	must(t, e.SetIncidentAgency(SystemActor, n, "RCPD", []string{"RPD"}, "city assists"))
	e.Read(func(s *State) {
		if !s.IncidentIn(s.AccessScope("tim", "", ""), s.Incidents[n]) {
			t.Fatal("tim cannot see an incident the city assists on")
		}
		if !s.UnitIn(s.AccessScope("tim", "", ""), s.Units["E1"]) {
			t.Fatal("unit without an agency hidden")
		}
	})
}
