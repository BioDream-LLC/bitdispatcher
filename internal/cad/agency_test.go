package cad

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestAgencies(t *testing.T) {
	e, _, path := testEngine(t)
	admin := Actor{User: "boss", Role: "admin", Console: "ADM"}
	for _, g := range []Agency{
		{Code: "DISP", Name: "Emergency communications centre", Disciplines: []string{"dispatch"}},
		{Code: "RPD", Name: "City police", Disciplines: []string{"police"}, IncidentPrefix: "RPD", ORI: "VA1280000", Timers: map[string]int{"onscene": 20}},
		{Code: "RCPD", Name: "County police", Disciplines: []string{"police"}, IncidentPrefix: "RCP", Timers: map[string]int{"onscene": 45}},
		{Code: "RFD", Name: "City fire and EMS", Disciplines: []string{"fire", "ems"}},
	} {
		must(t, e.SaveAgency(admin, g))
	}
	if err := e.SaveAgency(admin, Agency{Code: "X", Name: "Bad", Disciplines: []string{"police"}}); err == nil {
		t.Fatal("a one-letter code was taken")
	}
	if err := e.SaveAgency(admin, Agency{Code: "SPD", Name: "Salem police", Disciplines: []string{"police"}, IncidentPrefix: "RPD"}); err == nil {
		t.Fatal("a second agency took RPD's incident prefix")
	}
	// Units and stations belong to configured agencies, not free text.
	if err := e.SaveUnit(admin, UnitDef{ID: "2A11", Type: "patrol", Agency: "NOPE"}); err == nil {
		t.Fatal("a unit took an agency that does not exist")
	}
	must(t, e.SaveUnit(admin, UnitDef{ID: "2A11", Type: "patrol", Agency: "rpd"}))
	must(t, e.SaveUnit(admin, UnitDef{ID: "C21", Type: "patrol", Agency: "RCPD"}))
	must(t, e.SetStatus(SystemActor, "2A11", Available, "", ""))
	must(t, e.SetStatus(SystemActor, "C21", Available, "", ""))
	// Different timers per agency: a change to one does not alter the other.
	var u1, u2 *Unit
	e.Read(func(s *State) {
		u1, u2 = s.Units["2A11"], s.Units["C21"]
		cp1, cp2 := *u1, *u2
		cp1.Status, cp2.Status = OnScene, OnScene
		if s.TimerForUnit(&cp1) != 20 || s.TimerForUnit(&cp2) != 45 {
			t.Fatalf("timers %d %d", s.TimerForUnit(&cp1), s.TimerForUnit(&cp2))
		}
	})
	must(t, e.SaveAgency(admin, Agency{Code: "RPD", Name: "City police", Disciplines: []string{"police"}, IncidentPrefix: "RPD", ORI: "VA1280000", Timers: map[string]int{"onscene": 15}}))
	e.Read(func(s *State) {
		cp := *s.Units["C21"]
		cp.Status = OnScene
		if s.TimerForUnit(&cp) != 45 {
			t.Fatal("changing RPD changed RCPD's timer")
		}
	})
	// Call types by agency, incidents numbered under the agency's prefix.
	must(t, e.SaveCallType(admin, CallType{Code: "TS", Name: "Traffic stop", Priority: 4, Discipline: "police", Agencies: []string{"RPD", "RCPD"}}))
	must(t, e.SaveCallType(admin, CallType{Code: "STF", Name: "Structure fire", Priority: 1, Discipline: "fire", Agencies: []string{"RFD"}}))
	county := Actor{User: "cdisp", Role: "dispatcher", Console: "P2", Agency: "RCPD"}
	n1, err := e.CreateIncident(county, NewIncident{Type: "TS", Location: at(40.27, -76.88, "1 Main St")})
	must(t, err)
	n2, err := e.CreateIncident(county, NewIncident{Type: "STF", Location: at(40.27, -76.88, "2 Main St")})
	must(t, err)
	if _, err := e.CreateIncident(county, NewIncident{Type: "STF", Agency: "RPD", Location: at(40.27, -76.88, "3 Main St")}); err == nil {
		t.Fatal("a fire call type was used for the police")
	}
	e.Read(func(s *State) {
		if s.Incidents[n1].Agency != "RCPD" || n1[:3] != "RCP" {
			t.Fatalf("traffic stop %s for %q", n1, s.Incidents[n1].Agency)
		}
		if s.Incidents[n2].Agency != "RFD" {
			t.Fatalf("fire for %q", s.Incidents[n2].Agency)
		}
	})
	// Mutual aid: RPD's unit on the county's incident keeps its home agency, and RPD shows as assisting.
	must(t, e.DispatchWhy(county, n1, []string{"C21", "2A11"}, nil, "backup", ""))
	e.Read(func(s *State) {
		p, as := s.IncidentAgencies(s.Incidents[n1])
		if p != "RCPD" || !slices.Equal(as, []string{"RPD"}) || s.Units["2A11"].Agency != "RPD" {
			t.Fatalf("agencies %s %v", p, as)
		}
	})
	if err := e.SetIncidentAgency(county, n1, "RPD", nil, ""); err == nil {
		t.Fatal("the primary agency changed without a reason")
	}
	must(t, e.SetIncidentAgency(county, n1, "RPD", []string{"RCPD"}, "inside the city limits"))
	// Response plans of an agency apply only to its incidents.
	_, err = e.SavePlan(admin, ResponsePlan{Name: "City stops", CallTypes: []string{"TS"}, Order: []string{"2A11"}, Agency: "RPD"})
	must(t, err)
	n3, err := e.CreateIncident(county, NewIncident{Type: "TS", Location: at(40.27, -76.88, "4 Main St")})
	must(t, err)
	e.Read(func(s *State) {
		if p := s.PlanFor(s.Incidents[n3], e.now()); p != nil {
			t.Fatalf("RPD's plan applied to a county incident: %s", p.Name)
		}
		if p := s.PlanFor(s.Incidents[n1], e.now()); p == nil || p.Name != "City stops" {
			t.Fatal("RPD's plan did not apply to its incident")
		}
	})

	// People's agencies and coverage: a county dispatcher may not work the city.
	must(t, e.SaveStaff(admin, Staff{Username: "cdisp", Agencies: []string{"RCPD", "DISP"}, Home: "RCPD", Monitor: []string{"RPD"}}))
	if err := e.SaveStaff(admin, Staff{Username: "x", Agencies: []string{"RCPD"}, Home: "RPD"}); err == nil {
		t.Fatal("a usual agency outside the person's agencies was taken")
	}
	must(t, e.SaveWorkstation(admin, Workstation{ID: "P2", Duties: []string{"police"}, Agencies: []string{"RCPD"}, Monitor: []string{"RPD", "RFD"}}))
	e.Read(func(s *State) {
		c := s.CoverageOf("cdisp", "P2", "police")
		if c.Working != "RCPD" || !slices.Equal(c.Covered, []string{"RCPD"}) || !slices.Equal(c.Monitored, []string{"RPD", "RFD"}) {
			t.Fatalf("coverage %+v", c)
		}
		if s.MayWork("cdisp", "RPD") || !s.MayWork("cdisp", "RCPD") || !s.MayWork("anyone", "RPD") {
			t.Fatal("may work")
		}
	})
	if err := e.SetCoverage(county, WorkCoverage{Working: "RPD"}); err == nil {
		t.Fatal("covered an agency the person may not work")
	}
	must(t, e.SetCoverage(Actor{User: "cdisp", Role: "dispatcher", Console: "P2", Duty: "police"}, WorkCoverage{Working: "DISP", Covered: []string{"RCPD"}, Monitored: []string{"RPD"}}))
	e.Read(func(s *State) {
		if c := s.CoverageOf("cdisp", "P2", "police"); c.Working != "DISP" || !slices.Equal(c.Covered, []string{"DISP", "RCPD"}) {
			t.Fatalf("set coverage %+v", c)
		}
	})
	if err := e.RemoveAgency(admin, "RPD"); err == nil {
		t.Fatal("removed an agency with units")
	}
	replayed(t, e, path)
}

func TestPresetsAndViews(t *testing.T) {
	e, _, path := testEngine(t)
	admin := Actor{User: "boss", Role: "admin"}
	ct := Actor{User: "amy", Role: "dispatcher", Duty: "calltaker", Console: "CT1"}
	ws := json.RawMessage(`{"root":{"t":"leaf","s":"queue"},"cmd":true,"locked":true}`)
	must(t, e.SavePreset(ct, Preset{Duty: "calltaker", Workspace: ws}))
	must(t, e.SavePreset(ct, Preset{Duty: "police", Workspace: json.RawMessage(`{"root":{"t":"leaf","s":"police"},"cmd":true,"locked":true}`)}))
	e.Read(func(s *State) {
		p, src := s.PresetOf("amy", "calltaker")
		if src != "own" || string(p.Workspace) != string(ws) || p.Version != 1 || p.By != "amy" {
			t.Fatalf("preset %s %+v", src, p)
		}
		if p2, _ := s.PresetOf("amy", "police"); string(p2.Workspace) == string(ws) {
			t.Fatal("the police preset is the call-taker's")
		}
	})
	if err := e.SavePreset(ct, Preset{User: "bob", Duty: "calltaker", Workspace: ws}); err == nil {
		t.Fatal("changed someone else's preset")
	}
	// The agency's assigned training preset: a trainee works it and cannot change it.
	must(t, e.SavePreset(admin, Preset{User: AssignedUser, Duty: "calltaker", Workspace: json.RawMessage(`{"root":{"t":"leaf","s":"incident"},"cmd":true,"locked":true}`)}))
	must(t, e.SaveStaff(admin, Staff{Username: "tim", Trainee: true}))
	tim := Actor{User: "tim", Role: "dispatcher", Duty: "calltaker"}
	if err := e.SavePreset(tim, Preset{Duty: "calltaker", Workspace: ws}); err == nil {
		t.Fatal("a trainee saved a preset")
	}
	e.Read(func(s *State) {
		if _, src := s.PresetOf("tim", "calltaker"); src != "assigned" {
			t.Fatalf("trainee source %s", src)
		}
		if _, src := s.PresetOf("newbie", "calltaker"); src != "assigned" {
			t.Fatal("someone without a preset does not start from the assigned one")
		}
	})
	// A protected preset is changed only by administrators.
	must(t, e.SavePreset(admin, Preset{User: "amy", Duty: "calltaker", Workspace: ws, Protected: true}))
	if err := e.SavePreset(ct, Preset{Duty: "calltaker", Workspace: ws}); err == nil {
		t.Fatal("changed a protected preset")
	}
	// Supervisory views: the agency decides.
	e.Read(func(s *State) {
		if !s.MaySeeView("sue", "supervisor", "ledger") || s.MaySeeView("amy", "dispatcher", "ledger") {
			t.Fatal("default view rights")
		}
	})
	var set Settings
	e.Read(func(s *State) { set = s.Settings })
	set.Views = []ViewRule{{ID: "ledger", Users: []string{"sue"}}, {ID: "board", Roles: []string{"supervisor", "dispatcher"}}}
	must(t, e.SaveSettings(admin, set))
	e.Read(func(s *State) {
		if !s.MaySeeView("sue", "supervisor", "ledger") || s.MaySeeView("sam", "supervisor", "ledger") || !s.MaySeeView("amy", "dispatcher", "board") {
			t.Fatal("view rules")
		}
		if !s.MaySeeView("boss", "admin", "ledger") {
			t.Fatal("administrators always see views")
		}
	})
	if ViewForPath("/api/ledger/verify") != "ledger" || ViewForPath("/api/ledgerx") != "" || ViewForPath("/api/review/26-1") != "" {
		t.Fatal("view paths")
	}
	replayed(t, e, path)
}
