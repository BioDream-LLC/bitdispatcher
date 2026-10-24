package cad

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestStaffingAndQualifications(t *testing.T) {
	e, c, path := testEngine(t)
	sup := Actor{User: "sue", Role: "supervisor", Console: "SUP"}
	field := Actor{User: "e1crew", Role: "field", Duty: "field"}
	var set Settings
	e.Read(func(s *State) { set = s.Settings })
	for i := range set.UnitTypes {
		if set.UnitTypes[i].Code == "engine" {
			set.UnitTypes[i].MinStaff, set.UnitTypes[i].Positions = 3, []string{"officer", "driver"}
		}
		if set.UnitTypes[i].Code == "ladder" {
			set.UnitTypes[i].SignOn = "approve"
		}
	}
	set.Staffing = StaffingRules{Short: "oos", Dispatch: "override", Minimums: []StaffMinimum{{Qual: "hazmat-tech", Count: 2}}}
	set.CapNeeds = append(DefaultCapNeeds(), CapNeed{Capability: "hazmat", Qualification: "hazmat-tech", Count: 2})
	must(t, e.SaveSettings(SystemActor, set))
	for _, m := range []Member{{ID: "1", Name: "Ann Officer"}, {ID: "2", Name: "Bo Driver"}, {ID: "3", Name: "Cy Firefighter"}, {ID: "4", Name: "Di Relief"}, {ID: "5", Name: "Ed County", Agency: "RCFR"}} {
		must(t, e.SaveMember(SystemActor, m))
	}
	// Qualifications: HazMat technician needs verifying and lasts 24 months.
	must(t, e.SaveQual(sup, QualDef{Code: "hazmat-ops", Name: "HazMat operations", Category: "credential"}))
	must(t, e.SaveQual(sup, QualDef{Code: "hazmat-tech", Name: "HazMat technician", Category: "team", Months: 24, Prereqs: []string{"hazmat-ops"}, Verify: true, Levels: []string{"Technician", "Team Leader"}}))
	must(t, e.SaveQual(sup, QualDef{Code: "fto", Name: "Field training officer", Category: "training"}))
	if err := e.SaveCredential(sup, "1", Credential{Qual: "hazmat-tech", Issued: c.t.AddDate(-1, 0, 0)}); err == nil {
		t.Fatal("a credential was taken without its prerequisite")
	}
	must(t, e.SaveCredential(sup, "1", Credential{Qual: "hazmat-ops"}))
	must(t, e.SaveCredential(sup, "3", Credential{Qual: "hazmat-ops"}))
	must(t, e.SaveCredential(sup, "1", Credential{Qual: "hazmat-tech", Level: "Technician", Issued: c.t.AddDate(-1, 0, 0), Verify: true}))
	// A self-entry is not verified, so it does not count.
	must(t, e.SaveCredential(Actor{User: "cy", Role: "dispatcher"}, "3", Credential{Qual: "hazmat-tech", Issued: c.t.AddDate(-1, 0, 0), Verify: true}))
	var expires time.Time
	e.Read(func(s *State) {
		m1, m3 := s.Members["1"], s.Members["3"]
		if !slices.Contains(s.ValidQuals(m1, c.t), "hazmat-tech") || slices.Contains(s.ValidQuals(m3, c.t), "hazmat-tech") {
			t.Fatalf("valid quals %v %v", s.ValidQuals(m1, c.t), s.ValidQuals(m3, c.t))
		}
		expires = m1.Creds[slices.IndexFunc(m1.Creds, func(c Credential) bool { return c.Qual == "hazmat-tech" })].Expires
	})
	if !expires.Equal(c.t.AddDate(1, 0, 0)) {
		t.Fatalf("expiry %v", expires)
	}

	// Two responders on an engine in positions: it is short (2 of 3), so out of service by policy.
	must(t, e.SetCrewChange(dispatcher, "E1", CrewChange{Seats: []CrewSeat{{Member: "1", Role: "officer"}, {Member: "2", Role: "driver"}}}))
	e.Read(func(s *State) {
		u := s.Units["E1"]
		if u.Status != OutOfService || s.CapabilityOf(u).State != CapUnavailable {
			t.Fatalf("short engine %s %+v", u.Status, s.CapabilityOf(u))
		}
		if s.Members["1"].Duty == nil {
			t.Fatal("put on a unit, the officer is not on duty")
		}
	})
	must(t, e.SetStatus(dispatcher, "E1", Available, "", ""))
	must(t, e.SetCrewChange(dispatcher, "E1", CrewChange{Seats: []CrewSeat{{Member: "1", Role: "officer"}, {Member: "2", Role: "driver"}, {Member: "3", Role: "firefighter"}}}))
	e.Read(func(s *State) {
		if cp := s.CapabilityOf(s.Units["E1"]); cp.State != CapFull {
			t.Fatalf("full engine %+v", cp)
		}
	})
	// One person on two active units: refused, unless a secondary assignment with a reason.
	if err := e.SetCrewChange(dispatcher, "E2", CrewChange{Seats: []CrewSeat{{Member: "3"}}}); err == nil || !strings.Contains(err.Error(), "is on E1") {
		t.Fatalf("double assignment: %v", err)
	}
	if err := e.SetCrewChange(dispatcher, "E2", CrewChange{Seats: []CrewSeat{{Member: "3", Secondary: true}}}); err == nil {
		t.Fatal("a secondary assignment without a reason")
	}
	// Dispatch: the incident keeps the crew and their qualifications at that moment.
	num, err := e.CreateIncident(dispatcher, NewIncident{Type: "SF", Location: at(40.2701, -76.8801, "1 Front St")})
	if err != nil {
		e.SaveCallType(SystemActor, CallType{Code: "SF", Name: "Structure fire", Priority: 1, Discipline: "fire", RunCard: []Requirement{{"engine", 1}}})
		num, err = e.CreateIncident(dispatcher, NewIncident{Type: "SF", Location: at(40.2701, -76.8801, "1 Front St")})
		must(t, err)
	}
	must(t, e.Dispatch(dispatcher, num, []string{"E1"}, nil))
	// Relief during the incident: Di replaces Cy, effective five minutes ago.
	c.t = c.t.Add(10 * time.Minute)
	must(t, e.SetCrewChange(dispatcher, "E1", CrewChange{Seats: []CrewSeat{{Member: "1", Role: "officer"}, {Member: "2", Role: "driver"}, {Member: "4", Role: "firefighter"}}, Reason: "relief", Effective: c.t.Add(-5 * time.Minute)}))
	var in Incident
	e.Read(func(s *State) { in = *s.Incidents[num] })
	as := in.Units[0]
	if len(as.Crew) != 3 || as.Crew[2].Name != "Cy Firefighter" || !slices.Contains(as.Crew[0].Quals, "hazmat-tech") {
		t.Fatalf("crew at dispatch %+v", as.Crew)
	}
	if !slices.ContainsFunc(in.Timeline, func(x Entry) bool {
		return strings.Contains(x.Text, "joined: Di Relief (firefighter); left: Cy Firefighter") && strings.Contains(x.Text, "relief")
	}) {
		t.Fatalf("timeline %+v", in.Timeline[len(in.Timeline)-1])
	}
	// Signing off the last required person: below minimum; a supervisor override keeps it in.
	if err := e.SignOff(dispatcher, "2", "end of shift", "keep it", time.Time{}); err == nil {
		t.Fatal("a dispatcher overrode the staffing rule")
	}
	must(t, e.SignOff(sup, "2", "end of shift", "chief's orders until 1900", time.Time{}))
	e.Read(func(s *State) {
		u := s.Units["E1"]
		if s.Members["2"].Duty != nil || len(u.Crew) != 2 || u.StaffOverride == "" {
			t.Fatalf("sign off %+v %q", u.Crew, u.StaffOverride)
		}
	})
	// A supervisor corrects the crew record on the incident: the original stays in the timeline.
	must(t, e.CorrectCrew(sup, "E1", num, []CrewSeat{{Member: "1", Role: "officer"}, {Member: "2", Role: "driver"}, {Member: "4", Role: "firefighter"}}, c.t.Add(-20*time.Minute), "Di was aboard from the start"))
	e.Read(func(s *State) {
		in := s.Incidents[num]
		if !in.Units[0].CrewCorrected || in.Units[0].Crew[2].Name != "Di Relief" {
			t.Fatalf("corrected %+v", in.Units[0])
		}
		if !strings.Contains(in.Timeline[len(in.Timeline)-1].Text, "was Ann Officer, Bo Driver, Cy Firefighter") {
			t.Fatalf("correction %s", in.Timeline[len(in.Timeline)-1].Text)
		}
	})
	// Capability from people: HazMat needs two valid technicians; with one it is not hazmat-capable.
	must(t, e.SaveUnit(SystemActor, UnitDef{ID: "HM1", Type: "engine", Station: "ST1", Capabilities: []string{"hazmat"}, MinStaff: 1}))
	must(t, e.SetStatus(SystemActor, "HM1", Available, "", ""))
	must(t, e.SetCrewChange(sup, "HM1", CrewChange{Seats: []CrewSeat{{Member: "1", Role: "officer", Secondary: true, Reason: "hazmat team call-up"}, {Member: "3", Role: "driver", Move: true}}, Override: "positions covered"}))
	e.Read(func(s *State) {
		u := s.Units["HM1"]
		if slices.Contains(s.Capabilities(u), "hazmat") || s.CapabilityOf(u).State != CapLimited {
			t.Fatalf("hazmat with one tech %v %+v", s.Capabilities(u), s.CapabilityOf(u))
		}
		if !slices.Contains(s.unitsOf("1"), "E1") || !slices.Contains(s.unitsOf("1"), "HM1") {
			t.Fatalf("secondary assignment %v", s.unitsOf("1"))
		}
	})
	must(t, e.SaveCredential(sup, "3", Credential{Qual: "hazmat-tech", Issued: c.t.AddDate(-1, 0, 0), Verify: true}))
	e.Read(func(s *State) {
		if !slices.Contains(s.Capabilities(s.Units["HM1"]), "hazmat") {
			t.Fatal("two verified technicians aboard, still not hazmat-capable")
		}
		g := s.StaffCoverage(c.t)
		if len(g) != 1 || g[0].Have != 2 {
			t.Fatalf("coverage %+v", g)
		}
	})
	// Expiry: a year later the technicians have expired; the unit loses hazmat.
	c.t = c.t.AddDate(1, 0, 1)
	e.Read(func(s *State) {
		if slices.Contains(s.Capabilities(s.Units["HM1"]), "hazmat") {
			t.Fatal("expired technicians still count")
		}
		if x := s.Expiring(c.t); len(x) != 2 || !x[0].Expired {
			t.Fatalf("expiring %+v", x)
		}
	})
	rem, err := e.RemindCredentials()
	must(t, err)
	again, _ := e.RemindCredentials()
	if len(rem) != 2 || len(again) != 0 {
		t.Fatalf("reminders %d then %d", len(rem), len(again))
	}
	// Mutual aid: a county member's own qualification counts through an approved equivalency.
	must(t, e.SaveAgency(SystemActor, Agency{Code: "RCFR", Name: "County fire", Disciplines: []string{"fire"}}))
	must(t, e.SaveQual(sup, QualDef{Code: "rc-hazmat", Name: "County HazMat tech", Category: "team", Agency: "RCFR"}))
	must(t, e.SaveCredential(sup, "5", Credential{Qual: "rc-hazmat"}))
	e.Read(func(s *State) {
		if slices.Contains(s.ValidQuals(s.Members["5"], c.t), "hazmat-tech") {
			t.Fatal("an unaccepted qualification counted")
		}
	})
	must(t, e.SaveEquivalency(sup, Equivalency{FromAgency: "RCFR", From: "rc-hazmat", To: "hazmat-tech", Scope: "mutual aid", Expires: c.t.AddDate(1, 0, 0)}))
	e.Read(func(s *State) {
		m := s.Members["5"]
		if !slices.Contains(s.ValidQuals(m, c.t), "hazmat-tech") || len(m.Creds) != 1 || m.Creds[0].Qual != "rc-hazmat" {
			t.Fatalf("equivalency %v %+v", s.ValidQuals(m, c.t), m.Creds)
		}
	})
	// A crew's own sign-on for a ladder waits for a supervisor.
	id, err := e.RequestCrew(field, "L1", CrewChange{Seats: []CrewSeat{{Member: "5"}}})
	must(t, err)
	if id == "" {
		t.Fatal("ladder sign-on was not held for approval")
	}
	must(t, e.DecideCrew(sup, id, true, ""))
	e.Read(func(s *State) {
		if len(s.Units["L1"].Crew) != 1 || s.CrewRequests[id].Decided != "approved" {
			t.Fatal("approved sign-on")
		}
	})
	// Saved crews and a shift change.
	tid, err := e.SaveCrewTemplate(sup, CrewTemplate{Name: "B shift E2", Unit: "E2", Seats: []CrewSeat{{Member: "2", Role: "driver"}, {Member: "4", Role: "officer"}}})
	must(t, err)
	n, err := e.ShiftChange(sup, nil, []string{tid}, time.Time{}, "B shift")
	must(t, err)
	e.Read(func(s *State) {
		if n != 1 || len(s.Units["E2"].Crew) != 2 || slices.ContainsFunc(s.Units["E1"].Crew, func(c CrewSeat) bool { return c.Member == "4" }) {
			t.Fatalf("shift change %+v / %+v", s.Units["E2"].Crew, s.Units["E1"].Crew)
		}
	})
	replayed(t, e, path)
}
