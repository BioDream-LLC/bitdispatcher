package cad

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func at(lat, lon float64, addr string) *ng911.Location {
	return &ng911.Location{Lat: &lat, Lon: &lon, Address: addr}
}

// replayed checks that replaying the ledger gives the same state.
func replayed(t *testing.T, e *Engine, path string) {
	t.Helper()
	before := snapshotJSON(t, e)
	e2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()
	if after := snapshotJSON(t, e2); after != before {
		t.Fatalf("replay differs\nbefore %s\nafter  %s", before, after)
	}
}

func TestAlarmLevels(t *testing.T) {
	e, c, path := testEngine(t)
	for _, id := range []string{"E3", "E4", "E5"} {
		must(t, e.SaveUnit(SystemActor, UnitDef{ID: id, Type: "engine", Station: "ST2"}))
		must(t, e.SetStatus(SystemActor, id, Available, "", ""))
	}
	must(t, e.SaveCallType(SystemActor, CallType{Code: "SF", Name: "Structure fire", Priority: 1, Discipline: "fire",
		RunCard: []Requirement{{"engine", 2}, {"ladder", 1}},
		Alarms:  []AlarmLevel{{Name: "2nd alarm", RunCard: []Requirement{{"engine", 2}, {"als", 1}}}}}))
	num, err := e.CreateIncident(dispatcher, NewIncident{Type: "SF", Location: at(40.271, -76.881, "1 Main St")})
	must(t, err)
	r, err := e.Recommend(num)
	must(t, err)
	if len(r.Units) != 3 {
		t.Fatalf("1st alarm recommends %v", r.Units)
	}
	must(t, e.DispatchWhy(dispatcher, num, r.Units, r.Units, "", ""))
	c.t = c.t.Add(5 * time.Minute)
	if res := e.Execute(dispatcher, "ALARM", num); !res.OK {
		t.Fatal(res.Message)
	}
	var in Incident
	e.Read(func(s *State) { in = *s.Incidents[num] })
	if in.AlarmLevel() != 2 {
		t.Fatalf("level %d", in.AlarmLevel())
	}
	last := in.Timeline[len(in.Timeline)-1]
	if last.Kind != "alarm" || !last.Priority || !strings.Contains(last.Text, "2nd alarm") || !strings.Contains(last.Text, "2 × engine") {
		t.Fatalf("timeline %+v", last)
	}
	// The recommendation now fills the extra lines, counting the units already there.
	r, err = e.Recommend(num)
	must(t, err)
	var eng Line
	for _, l := range r.Lines {
		if l.Capability == "engine" {
			eng = l
		}
	}
	if eng.Count != 4 || len(eng.Covered) != 2 || len(eng.Units) != 2 {
		t.Fatalf("2nd alarm engine line %+v", eng)
	}
	if !slices.ContainsFunc(r.Notes, func(n string) bool { return strings.Contains(n, "2nd alarm") }) {
		t.Fatalf("notes %v", r.Notes)
	}
	// No 3rd alarm on this type; down needs a reason.
	if res := e.Execute(dispatcher, "ALARM", num); res.OK || !strings.Contains(res.Message, "goes up to the 2nd alarm") {
		t.Fatalf("3rd: %+v", res)
	}
	if err := e.SetAlarm(dispatcher, num, 1, ""); err == nil {
		t.Fatal("brought down without a reason")
	}
	must(t, e.SetAlarm(dispatcher, num, 1, "fire knocked down"))
	// A type without alarms says where to add them.
	n2, _ := e.CreateIncident(dispatcher, NewIncident{Type: "FALL", Location: at(40.27, -76.88, "2 Main St")})
	if err := e.UpgradeAlarm(dispatcher, n2, ""); err == nil || !strings.Contains(err.Error(), "Admin › Call types") {
		t.Fatalf("no alarms: %v", err)
	}
	// Bad alarm definitions are refused.
	if err := e.SaveCallType(SystemActor, CallType{Code: "X", Name: "X", Priority: 2, Discipline: "fire", Alarms: []AlarmLevel{{Name: "2nd"}}}); err == nil {
		t.Fatal("empty alarm accepted")
	}
	replayed(t, e, path)
}

func TestCrewsAndStaffing(t *testing.T) {
	e, _, path := testEngine(t)
	must(t, e.SaveMember(SystemActor, Member{ID: "101", Name: "Ana Diaz", Quals: []string{"Paramedic", "driver"}}))
	must(t, e.SaveMember(SystemActor, Member{ID: "102", Name: "Ben Ito", Quals: []string{"emt"}}))
	must(t, e.SaveMember(SystemActor, Member{ID: "103", Name: "Cy Park", Quals: []string{"emt"}}))
	if err := e.SaveMember(SystemActor, Member{ID: "104", Name: "X", Phone: "540-555-0100"}); err == nil {
		t.Fatal("a phone not in international form was accepted")
	}
	caps := func(u string) (out []string) {
		e.Read(func(s *State) { out = s.Capabilities(s.Units[u]) })
		return
	}
	// No crew recorded: the unit is as its type says.
	if !slices.Contains(caps("M1"), "als") {
		t.Fatal("unrecorded crew lost als")
	}
	// Two EMTs on a medic: no paramedic, so no ALS, and the recommendation does not send it for als.
	must(t, e.SetCrew(dispatcher, "M1", []CrewSeat{{Member: "102"}, {Member: "103"}}, ""))
	if slices.Contains(caps("M1"), "als") || !slices.Contains(caps("M1"), "bls") {
		t.Fatalf("caps %v", caps("M1"))
	}
	num, _ := e.CreateIncident(dispatcher, NewIncident{Type: "CHEST", Location: at(40.2701, -76.8801, "1 Front St")})
	r, _ := e.Recommend(num)
	if slices.Contains(r.Units, "M1") || !slices.Contains(r.Units, "M2") {
		t.Fatalf("recommended %v", r.Units)
	}
	// The paramedic moves from nowhere to M1; M1 is ALS again. Moving her to M2 takes her off M1.
	must(t, e.SetCrew(dispatcher, "M1", []CrewSeat{{Member: "101", Role: "paramedic"}, {Member: "102"}}, ""))
	if !slices.Contains(caps("M1"), "als") {
		t.Fatal("paramedic aboard, still no als")
	}
	if err := e.SetCrew(dispatcher, "M2", []CrewSeat{{Member: "101"}}, ""); err == nil {
		t.Fatal("a member was put on a second unit without a move")
	}
	must(t, e.SetCrew(dispatcher, "M2", []CrewSeat{{Member: "101", Move: true}}, ""))
	var m1, m2 Unit
	var short []string
	e.Read(func(s *State) { m1, m2 = *s.Units["M1"], *s.Units["M2"]; short = s.Staffing(s.Units["M2"]) })
	if len(m1.Crew) != 1 || m1.Crew[0].Member != "102" || len(m2.Crew) != 1 {
		t.Fatalf("move: M1 %+v M2 %+v", m1.Crew, m2.Crew)
	}
	if len(short) != 1 || short[0] != "1 of 2 aboard" {
		t.Fatalf("M2 staffing %v", short)
	}
	r, _ = e.Recommend(num)
	for _, l := range r.Lines {
		for _, c := range l.Units {
			if c.Unit == "M2" && !slices.ContainsFunc(c.Reasons, func(s string) bool { return strings.HasPrefix(s, "Short-staffed: 1 of 2") }) {
				t.Fatalf("reasons %v", c.Reasons)
			}
		}
	}
	// Removing a member takes them off their unit.
	must(t, e.RemoveMember(SystemActor, "101"))
	e.Read(func(s *State) { m2 = *s.Units["M2"] })
	if len(m2.Crew) != 0 {
		t.Fatalf("removed member still aboard %+v", m2.Crew)
	}
	replayed(t, e, path)
}

func TestFiregroundAccountability(t *testing.T) {
	e, c, path := testEngine(t)
	num, err := e.CreateIncident(dispatcher, NewIncident{Type: "SF", Location: at(40.271, -76.881, "1 Main St")})
	must(t, err)
	must(t, e.Dispatch(dispatcher, num, []string{"E1", "E2", "L1"}, nil))
	var in Incident
	get := func() { e.Read(func(s *State) { in = *s.Incidents[num] }) }
	get()
	if in.Fireground != nil {
		t.Fatal("clock started before anyone arrived")
	}
	c.t = c.t.Add(4 * time.Minute)
	must(t, e.SetStatus(dispatcher, "E1", OnScene, "", ""))
	get()
	if in.Fireground == nil || !in.Fireground.Auto || in.Fireground.Minutes != 20 {
		t.Fatalf("auto start %+v", in.Fireground)
	}
	due, ok := in.Fireground.Due()
	if !ok || !due.Equal(c.t.Add(20*time.Minute)) {
		t.Fatalf("due %v", due)
	}
	must(t, e.SetStatus(dispatcher, "L1", OnScene, "", ""))
	c.t = c.t.Add(19 * time.Minute)
	if res := e.Execute(dispatcher, "PAR MISSING L1", num); !res.OK {
		t.Fatal(res.Message)
	}
	get()
	f := in.Fireground
	if len(f.Missing) != 1 || f.Missing[0] != "L1" || !f.LastPAR.IsZero() || len(f.Checks) != 1 || !slices.Equal(f.Checks[0].OK, []string{"E1"}) {
		t.Fatalf("missing %+v", f)
	}
	last := in.Timeline[len(in.Timeline)-1]
	if !last.Priority || !strings.Contains(last.Text, "NOT ACCOUNTED FOR L1") {
		t.Fatalf("timeline %+v", last)
	}
	if err := e.EndFireground(dispatcher, num); err == nil {
		t.Fatal("stopped with a crew missing")
	}
	c.t = c.t.Add(time.Minute)
	if res := e.Execute(dispatcher, "PAR FOUND L1", num); !res.OK {
		t.Fatal(res.Message)
	}
	get()
	if len(in.Fireground.Missing) != 0 || !in.Fireground.LastPAR.Equal(c.t) {
		t.Fatalf("found %+v", in.Fireground)
	}
	// E2 is dispatched but not on scene: a roll call of "all" counts those on scene.
	must(t, e.RecordPAR(dispatcher, num, nil, nil, map[string]int{"E1": 4}, ""))
	get()
	if ch := in.Fireground.Checks[len(in.Fireground.Checks)-1]; len(ch.OK) != 2 || ch.PAX["E1"] != 4 {
		t.Fatalf("check %+v", ch)
	}
	if err := e.RecordPAR(dispatcher, num, []string{"M1"}, nil, nil, ""); err == nil {
		t.Fatal("a unit not on the incident was accounted for")
	}
	if res := e.Execute(dispatcher, "BM UC", num); !res.OK {
		t.Fatal(res.Message)
	}
	if res := e.Execute(dispatcher, "BM UC", num); res.OK {
		t.Fatal("benchmark twice")
	}
	get()
	if len(in.Fireground.Benchmarks) != 1 || in.Fireground.Benchmarks[0].Name != "Fire under control" {
		t.Fatalf("benchmarks %+v", in.Fireground.Benchmarks)
	}
	must(t, e.CloseIncident(dispatcher, num, "Handled", ""))
	get()
	if in.Fireground.Ended.IsZero() {
		t.Fatal("clock still running after close")
	}
	replayed(t, e, path)
}

func TestTowRotation(t *testing.T) {
	e, c, path := testEngine(t)
	s := DefaultSettings()
	s.TowZones = []string{"North", "South"}
	must(t, e.SaveSettings(SystemActor, s))
	a, _ := e.SaveTowCompany(SystemActor, TowCompany{Name: "Acme Towing", Phone: "540-555-0101", Classes: []string{"light", "medium"}})
	b, _ := e.SaveTowCompany(SystemActor, TowCompany{Name: "Blue Ridge Wrecker", Phone: "540-555-0102", Classes: []string{"light", "heavy"}, Zones: []string{"North"}})
	cc, _ := e.SaveTowCompany(SystemActor, TowCompany{Name: "City Tow", Phone: "540-555-0103", Classes: []string{"light"}, Zones: []string{"South"}})
	if _, err := e.SaveTowCompany(SystemActor, TowCompany{Name: "X", Phone: "1", Classes: []string{"boat"}}); err == nil {
		t.Fatal("unknown class accepted")
	}
	num, _ := e.CreateIncident(dispatcher, NewIncident{Type: "MVA", Location: at(40.27, -76.88, "I-81 mm 140")})
	next := func(class, zone string) string {
		t.Helper()
		tw, err := e.RequestTow(dispatcher, num, class, zone, TowRotation, "", "", "")
		must(t, err)
		c.t = c.t.Add(time.Minute)
		return tw.Company
	}
	// North light: Acme and Blue Ridge alternate; City Tow is south only.
	first, second, third := next("light", "North"), next("light", "North"), next("light", "North")
	if first != a || second != b || third != a {
		t.Fatalf("north rotation %s %s %s", first, second, third)
	}
	// Heavy in the north: only Blue Ridge.
	if got := next("heavy", "North"); got != b {
		t.Fatalf("heavy %s", got)
	}
	if _, err := e.RequestTow(dispatcher, num, "heavy", "South", TowRotation, "", "", ""); err == nil {
		t.Fatal("no heavy in the south, yet one was offered")
	}
	// A decline loses the turn: the next is offered.
	tw, _ := e.RequestTow(dispatcher, num, "light", "South", TowRotation, "", "VA ABC-1234", "")
	if tw.Company != a {
		t.Fatalf("south first %s", tw.Company)
	}
	must(t, e.TowAnswer(dispatcher, tw.ID, TowDeclined, "no driver"))
	c.t = c.t.Add(time.Minute)
	if got := next("light", "South"); got != cc {
		t.Fatalf("after decline %s", got)
	}
	// The owner's choice takes no turn.
	before, _ := func() (string, error) {
		var el []TowTurn
		e.Read(func(s *State) { el, _ = s.TowRotationFor("light", "North") })
		return el[0].Company.ID, nil
	}()
	_, err := e.RequestTow(dispatcher, num, "light", "North", TowOwner, b, "", "owner asked")
	must(t, err)
	var el []TowTurn
	e.Read(func(s *State) { el, _ = s.TowRotationFor("light", "North") })
	if el[0].Company.ID != before {
		t.Fatal("an owner's choice took a turn")
	}
	if res := e.Execute(dispatcher, "TOW light North", num); !res.OK || !strings.Contains(res.Message, "Call ") {
		t.Fatalf("TOW: %+v", res)
	}
	replayed(t, e, path)
}

func TestScheduledCallsAndEvents(t *testing.T) {
	e, c, path := testEngine(t)
	start := c.t.Add(30 * time.Minute)
	id, err := e.SaveScheduled(dispatcher, Scheduled{Kind: SchedCall, Title: "Dialysis transport", Type: "TRANSFER", Location: at(40.27, -76.88, "5 Hospital Dr"),
		Start: start, Lead: 10, Units: []string{"M2"}, Repeat: "daily"})
	must(t, err)
	ev, err := e.SaveScheduled(dispatcher, Scheduled{Kind: SchedEvent, Title: "County fair", Type: "EVENT", Location: at(40.28, -76.87, "Fairgrounds"),
		Start: c.t.Add(2 * time.Hour), Minutes: 600})
	must(t, err)
	if _, err := e.SaveScheduled(dispatcher, Scheduled{Kind: SchedEvent, Title: "x", Type: "EVENT", Start: c.t}); err == nil {
		t.Fatal("an event with no length accepted")
	}
	if made := e.RunSchedule(); len(made) != 0 {
		t.Fatalf("made early %v", made)
	}
	c.t = start.Add(-10 * time.Minute) // the lead
	made := e.RunSchedule()
	if len(made) != 1 {
		t.Fatalf("made %v", made)
	}
	var in Incident
	var m2 Unit
	var x Scheduled
	e.Read(func(s *State) { in, m2, x = *s.Incidents[made[0]], *s.Units["M2"], *s.Scheduled[id] })
	if in.Type != "TRANSFER" || m2.Incident != made[0] || !x.Last.Equal(start) || x.Incident != made[0] {
		t.Fatalf("incident %+v unit %s sched %+v", in.Type, m2.Incident, x)
	}
	if again := e.RunSchedule(); len(again) != 0 {
		t.Fatalf("made twice %v", again)
	}
	// Tomorrow's comes next; the event is upcoming, then running.
	if n := x.Next(c.t); !n.Equal(start.AddDate(0, 0, 1)) {
		t.Fatalf("next %v", n)
	}
	var up []UpcomingItem
	e.Read(func(s *State) { up = s.Upcoming(c.t, 24*time.Hour) })
	if len(up) < 2 {
		t.Fatalf("upcoming %+v", up)
	}
	c.t = c.t.Add(3 * time.Hour)
	made = e.RunSchedule()
	e.Read(func(s *State) { up = s.Upcoming(c.t, time.Hour) })
	if len(made) != 1 || !slices.ContainsFunc(up, func(u UpcomingItem) bool { return u.Item.ID == ev && u.Running }) {
		t.Fatalf("event made %v upcoming %+v", made, up)
	}
	// Missed by more than an hour (the system was down): skipped, not made late.
	c.t = start.AddDate(0, 0, 1).Add(3 * time.Hour)
	if made := e.RunSchedule(); len(made) != 0 {
		t.Fatalf("made a stale occurrence %v", made)
	}
	e.Read(func(s *State) { x = *s.Scheduled[id] })
	if !x.Last.Equal(start.AddDate(0, 0, 1)) {
		t.Fatalf("stale occurrence not marked %v", x.Last)
	}
	// Weekdays skip the weekend.
	w := Scheduled{Start: time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC), Repeat: "weekdays"} // a Friday
	occ := w.Occurrences(w.Start, w.Start.AddDate(0, 0, 4))
	if len(occ) != 3 || occ[1].Weekday() != time.Monday {
		t.Fatalf("weekdays %v", occ)
	}
	replayed(t, e, path)
}

func TestDirectoryAndOnCall(t *testing.T) {
	e, c, path := testEngine(t)
	water, err := e.SaveDirEntry(dispatcher, DirEntry{Name: "Water department", Org: "City of Roanoke", Category: "utility", Phones: []Phone{{"after hours", "540-555-0150"}}, Types: []string{"GAS"}})
	must(t, err)
	bob, _ := e.SaveDirEntry(dispatcher, DirEntry{Name: "Bob Shaw", Org: "Public works", Phones: []Phone{{"cell", "540-555-0160"}}})
	amy, _ := e.SaveDirEntry(dispatcher, DirEntry{Name: "Amy Cole", Org: "Public works", Phones: []Phone{{"cell", "540-555-0161"}}})
	me, _ := e.SaveDirEntry(dispatcher, DirEntry{Name: "Medical examiner", Phones: []Phone{{"", "540-555-0170"}}, Tags: []string{"ME"}})
	if _, err := e.SaveDirEntry(dispatcher, DirEntry{Name: "Nobody"}); err == nil {
		t.Fatal("an entry with no number accepted")
	}
	_, err = e.SaveOnCall(dispatcher, OnCallList{Name: "Public works on call", Types: []string{"GAS"}, Backup: water, Shifts: []OnCallShift{
		{Entry: bob, From: c.t.Add(-time.Hour), To: c.t.Add(8 * time.Hour)},
		{Entry: amy, From: c.t.Add(8 * time.Hour), To: c.t.Add(32 * time.Hour)},
	}})
	must(t, err)
	must(t, e.SaveCallType(SystemActor, CallType{Code: "DOA", Name: "Death investigation", Priority: 3, Discipline: "police", Notify: []string{"ME"}}))
	gas, _ := e.CreateIncident(dispatcher, NewIncident{Type: "GAS", Location: at(40.27, -76.88, "1 Elm St")})
	doa, _ := e.CreateIncident(dispatcher, NewIncident{Type: "DOA", Location: at(40.27, -76.88, "2 Elm St")})
	var co, co2, later []CallOut
	e.Read(func(s *State) {
		co = s.CallOutFor(s.Incidents[gas], c.t)
		later = s.CallOutFor(s.Incidents[gas], c.t.Add(9*time.Hour))
		co2 = s.CallOutFor(s.Incidents[doa], c.t)
	})
	if len(co) != 2 || co[0].List != "Public works on call" || co[0].Entry.ID != bob || co[1].Entry.ID != water {
		t.Fatalf("gas call-out %+v", co)
	}
	if later[0].Entry.ID != amy {
		t.Fatalf("after the shift change %+v", later[0])
	}
	if len(co2) != 1 || co2[0].Entry.ID != me || co2[0].Notify != "ME" {
		t.Fatalf("notify matched %+v", co2)
	}
	// After the last shift: the backup.
	var d *DirEntry
	var backup bool
	e.Read(func(s *State) {
		for _, l := range s.OnCall {
			d, _, backup = s.OnCallNow(l, c.t.Add(40*time.Hour))
		}
	})
	if d == nil || d.ID != water || !backup {
		t.Fatalf("backup %+v %v", d, backup)
	}
	if err := e.RemoveDirEntry(dispatcher, bob); err == nil {
		t.Fatal("removed an entry an on-call list uses")
	}
	var hits []*DirEntry
	e.Read(func(s *State) { hits = s.SearchDirectory("public works") })
	if len(hits) != 2 {
		t.Fatalf("search %d", len(hits))
	}
	e.Read(func(s *State) { hits = s.SearchDirectory("5550150") })
	if len(hits) != 1 || hits[0].ID != water {
		t.Fatalf("number search %+v", hits)
	}
	if res := e.Execute(dispatcher, "DIR medical", ""); !res.OK || !strings.Contains(res.Message, "540-555-0170") {
		t.Fatalf("DIR %+v", res)
	}
	replayed(t, e, path)
}

func TestGeofences(t *testing.T) {
	e, c, path := testEngine(t)
	ring := [][2]float64{{-76.90, 40.26}, {-76.86, 40.26}, {-76.86, 40.28}, {-76.90, 40.28}}
	id, err := e.SaveGeofence(dispatcher, Geofence{Name: "Downtown", Kind: FenceAlerting, Ring: ring, Enter: true, Leave: true, Disciplines: []string{"ems"}})
	must(t, err)
	_, err = e.SaveGeofence(dispatcher, Geofence{Name: "School zone", Kind: FenceCaution, Ring: ring, Note: "School: no sirens 07:30-08:30"})
	must(t, err)
	if _, err := e.SaveGeofence(dispatcher, Geofence{Name: "x", Kind: FenceCaution, Ring: ring}); err == nil {
		t.Fatal("a caution area with no note accepted")
	}
	move := func(u string, lat, lon float64) {
		c.t = c.t.Add(time.Minute)
		must(t, e.UnitPosition(RadioActor, u, Position{Lat: lat, Lon: lon, Source: "avl", At: c.t}))
	}
	move("M1", 40.30, -76.95) // outside
	move("M1", 40.27, -76.88) // in
	move("E1", 40.27, -76.88) // fire: not watched
	move("M1", 40.30, -76.95) // out
	var alerts []FenceAlert
	var in []string
	e.Read(func(s *State) { alerts, in = s.FenceAlerts, s.InFences("E1") })
	if len(alerts) != 2 || alerts[0].Unit != "M1" || !alerts[0].Enter || alerts[1].Enter || alerts[0].Fence != id {
		t.Fatalf("alerts %+v", alerts)
	}
	if len(in) != 2 {
		t.Fatalf("E1 in %v", in)
	}
	var cautions []*Geofence
	e.Read(func(s *State) { cautions = s.CautionsAt(40.27, -76.88) })
	if len(cautions) != 1 || cautions[0].Name != "School zone" {
		t.Fatalf("cautions %+v", cautions)
	}
	// Drawing an area around a unit already there is not an arrival.
	_, err = e.SaveGeofence(dispatcher, Geofence{Name: "Hospital", Kind: FenceAlerting, Ring: [][2]float64{{-76.96, 40.29}, {-76.94, 40.29}, {-76.94, 40.31}, {-76.96, 40.31}}, Enter: true})
	must(t, err)
	e.Read(func(s *State) { alerts = s.FenceAlerts })
	if len(alerts) != 2 {
		t.Fatalf("drawing alerted %+v", alerts)
	}
	replayed(t, e, path)
}

func TestPaging(t *testing.T) {
	e, _, path := testEngine(t)
	must(t, e.SaveMember(SystemActor, Member{ID: "201", Name: "Dee Fox", Phone: "+15405550201", Volunteer: true}))
	must(t, e.SaveMember(SystemActor, Member{ID: "202", Name: "Eli Gray", Email: "eli@example.com", Volunteer: true}))
	must(t, e.SaveMember(SystemActor, Member{ID: "203", Name: "Fay Hill", Inactive: true}))
	if _, err := e.SavePageGroup(SystemActor, PageGroup{Name: "No way"}); err == nil {
		t.Fatal("a group with no gateway accepted")
	}
	g, err := e.SavePageGroup(SystemActor, PageGroup{Name: "Station 2 volunteers", Members: []string{"201", "202", "203"}, Gateways: []string{"sms"}, Stations: []string{"ST2"}})
	must(t, err)
	num, _ := e.CreateIncident(dispatcher, NewIncident{Type: "SF", Location: at(40.27, -76.88, "1 Elm St")})
	var groups []string
	e.Read(func(s *State) { groups = s.GroupsToPage(num, []string{"E1"}) })
	if len(groups) != 0 {
		t.Fatalf("E1 is station 1: %v", groups)
	}
	e.Read(func(s *State) { groups = s.GroupsToPage(num, []string{"E2"}) })
	if len(groups) != 1 || groups[0] != g {
		t.Fatalf("E2 groups %v", groups)
	}
	id, to, err := e.SendPage(dispatcher, num, groups, nil, "", "Smoke showing", true)
	must(t, err)
	if len(to) != 2 { // the inactive member is not paged
		t.Fatalf("paged %d", len(to))
	}
	must(t, e.RecordPageResults(SystemActor, id, []PageResult{{Gateway: "sms", OK: true}, {Gateway: "sms", OK: false, Detail: "bad number"}}))
	if err := e.ReplyPage(id, "wrong", PageResponding, 5); err == nil {
		t.Fatal("a wrong token answered")
	}
	must(t, e.ReplyPage(id, to[0].Token, PageToScene, 7))
	must(t, e.ReplyPage(id, to[1].Token, PageUnavailable, 9))
	var p Page
	var in Incident
	e.Read(func(s *State) { p, in = *s.PagesFor(num)[0], *s.Incidents[num] })
	if p.Subject != "STRUCTURE FIRE 1 Elm St" || !p.Auto || len(p.Results) != 2 || p.Replies["201"].Answer != PageToScene || p.Replies["201"].ETA != 7 || p.Replies["202"].ETA != 0 {
		t.Fatalf("page %+v", p)
	}
	text := ""
	for _, t := range in.Timeline {
		text += t.Text + "\n"
	}
	for _, want := range []string{"Paged automatically Station 2 volunteers (2 people)", "1 of 2 deliveries failed", "Dee Fox responding to the scene, 7 min", "Eli Gray not available"} {
		if !strings.Contains(text, want) {
			t.Fatalf("timeline lacks %q:\n%s", want, text)
		}
	}
	if pp, who, ok := e.PageFor(id, to[0].Token); !ok || who.Member != "201" || pp.Replies != nil {
		t.Fatal("PageFor")
	}
	replayed(t, e, path)
}

func TestWarrantsAndPapers(t *testing.T) {
	e, _, path := testEngine(t)
	id, err := e.SavePaper(dispatcher, Paper{Kind: "arrest", Number: "GD26-1234", Court: "General District Court", Subject: PaperSubject{Name: "Jordan Q Example", DOB: "1990-04-02"},
		Location: at(40.2700, -76.8800, "12 Oak St"), Charge: "Failure to appear", Caution: "armed"})
	must(t, err)
	if _, err := e.SavePaper(dispatcher, Paper{Kind: "arrest", Subject: PaperSubject{Name: "x"}}); err == nil {
		t.Fatal("a paper with no place accepted")
	}
	num, _ := e.CreateIncident(dispatcher, NewIncident{Type: "DOM", Location: at(40.2702, -76.8801, "14 Oak St")})
	far, _ := e.CreateIncident(dispatcher, NewIncident{Type: "DOM", Location: at(40.30, -76.80, "9 Pine St")})
	same, _ := e.CreateIncident(dispatcher, NewIncident{Type: "DOM", Location: &ng911.Location{Address: "12 Oak St #B, Harrisburg"}})
	var near, nfar, nsame, named []*Paper
	e.Read(func(s *State) {
		near, nfar = s.PapersNear(s.Incidents[num].Location), s.PapersNear(s.Incidents[far].Location)
		nsame = s.PapersNear(s.Incidents[same].Location)
		named = s.PapersNamed([]string{"Example, Jordan Q"})
	})
	if len(near) != 1 || len(nfar) != 0 || len(nsame) != 1 || len(named) != 1 {
		t.Fatalf("near %d far %d same %d named %d", len(near), len(nfar), len(nsame), len(named))
	}
	must(t, e.AttemptPaper(dispatcher, id, "P1", "no answer", "car in the drive", num))
	must(t, e.AttemptPaper(dispatcher, id, "P1", "served", "in custody", num))
	var p Paper
	e.Read(func(s *State) { p = *s.Papers[id]; near = s.PapersNear(s.Incidents[num].Location) })
	if p.Status != PaperServed || len(p.Attempts) != 2 || len(near) != 0 {
		t.Fatalf("served %+v near %d", p, len(near))
	}
	if err := e.AttemptPaper(dispatcher, id, "P1", "x", "", ""); err == nil {
		t.Fatal("attempt on a served paper")
	}
	if err := e.SetPaperStatus(dispatcher, id, PaperActive, "", ""); err == nil {
		t.Fatal("made active again with no reason")
	}
	must(t, e.SetPaperStatus(dispatcher, id, PaperActive, "served on the wrong person", ""))
	replayed(t, e, path)
}

func TestUnitHistory(t *testing.T) {
	e, c, _ := testEngine(t)
	from := c.t
	num, _ := e.CreateIncident(dispatcher, NewIncident{Type: "CHEST", Location: at(40.27, -76.88, "1 Elm St")})
	c.t = c.t.Add(10 * time.Minute)
	must(t, e.Dispatch(dispatcher, num, []string{"M1"}, nil))
	c.t = c.t.Add(2 * time.Minute)
	must(t, e.SetStatus(dispatcher, "M1", Enroute, "", ""))
	c.t = c.t.Add(8 * time.Minute)
	must(t, e.SetStatus(dispatcher, "M1", OnScene, "", ""))
	must(t, e.CheckUnit(dispatcher, "M1", ""))
	c.t = c.t.Add(20 * time.Minute)
	must(t, e.SetStatus(dispatcher, "M1", Available, "", ""))
	c.t = c.t.Add(30 * time.Minute)
	h, err := e.UnitHistory("m1", from, c.t)
	must(t, err)
	var got []string
	for _, s := range h.Spans {
		got = append(got, s.Status)
	}
	if strings.Join(got, ",") != "available,dispatched,enroute,onscene,available" {
		t.Fatalf("spans %v", got)
	}
	if h.Totals[Dispatched] != 120 || h.Totals[OnScene] != 1200 || h.Committed != 1800 || h.Known != 4200 {
		t.Fatalf("totals %+v committed %v known %v", h.Totals, h.Committed, h.Known)
	}
	if len(h.Incidents) != 1 || len(h.Marks) != 1 || h.Marks[0].Kind != "check" || !h.Spans[len(h.Spans)-1].Open {
		t.Fatalf("incidents %v marks %+v", h.Incidents, h.Marks)
	}
	// A window that starts mid-span is clipped to it.
	h, _ = e.UnitHistory("M1", from.Add(15*time.Minute), from.Add(25*time.Minute))
	if len(h.Spans) != 2 || h.Spans[0].Status != Enroute || h.Spans[0].Seconds != 300 {
		t.Fatalf("clipped %+v", h.Spans)
	}
	if _, err := e.UnitHistory("M1", c.t, from); err == nil {
		t.Fatal("backwards period accepted")
	}
}
