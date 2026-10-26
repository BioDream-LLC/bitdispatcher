package cad

import (
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ng911"
	"github.com/biodream-llc/bitdispatcher/internal/phrase"
)

var (
	taker = Actor{User: "ctaker", Role: "dispatcher", Console: "CT1", Duty: DutyCallTaker}
	sup   = Actor{User: "sup1", Role: "supervisor", Console: "S1", Duty: DutySupervisor}
	cop   = Actor{User: "pd1", Role: "dispatcher", Console: "PD1", Duty: DutyPolice}
)

func f64(v float64) *float64 { return &v }

func finalEngine(t *testing.T) (*Engine, *clock, string) {
	e, c, path := testEngine(t)
	for _, u := range []string{"ctaker", "sup1", "pd1", "trainee1"} {
		must(t, e.Record(SystemActor, EvUserSaved, UserInfo{Username: u, Name: strings.ToUpper(u), Role: "dispatcher"}))
	}
	must(t, e.SaveStaff(SystemActor, Staff{Username: "trainee1", Trainee: true}))
	return e, c, path
}

func TestForcedLocationExceptionAndQueue(t *testing.T) {
	e, c, path := finalEngine(t)
	num, err := e.CreateIncident(taker, NewIncident{Type: "FALL", Location: &ng911.Location{Address: "900 NEW FARM RD", Lat: f64(40.27), Lon: f64(-76.88)}})
	must(t, err)
	// A forced location needs a reason and a map point.
	if _, err := e.SetPlace(taker, num, Place{Verify: PlaceForced, Entered: "900 New Farm Rd"}); err == nil || !strings.Contains(err.Error(), "map") {
		t.Fatalf("forced without map point: %v", err)
	}
	if _, err := e.SetPlace(taker, num, Place{Verify: PlaceForced, Entered: "900 New Farm Rd", Lat: f64(40.27), Lon: f64(-76.88)}); err == nil {
		t.Fatal("forced without reason")
	}
	// A trainee cannot force, and the attempt is on the record.
	trainee := Actor{User: "trainee1", Role: "dispatcher", Duty: DutyCallTaker}
	if _, err := e.SetPlace(trainee, num, Place{Verify: PlaceForced, Entered: "900 New Farm Rd", Reason: DefaultForceReasons[0], Lat: f64(40.27), Lon: f64(-76.88)}); err == nil || !isDuty(err) {
		t.Fatalf("trainee forced: %v", err)
	}
	x, err := e.SetPlace(taker, num, Place{Verify: PlaceForced, Entered: "900 New Farm Rd", Reason: DefaultForceReasons[0], Lat: f64(40.27), Lon: f64(-76.88),
		Name: "New Farm subdivision", Building: "Lot 12", Nearest: "880 NEW FARM RD"})
	must(t, err)
	if x == "" {
		t.Fatal("no exception")
	}
	var in Incident
	var ex LocException
	e.Read(func(s *State) { in, ex = *s.Incidents[num], *s.LocExceptions[x] })
	if in.Place.Entered != "900 New Farm Rd" || in.Place.Verify != PlaceForced || ex.Kind != PlaceForced || ex.Queue != QueueNew || !ex.Open() {
		t.Fatalf("place %+v ex %+v", in.Place, ex)
	}
	if !strings.Contains(timelineText(in), "could not force") {
		t.Fatal("refusal not on the incident's record")
	}
	// The call-taker cannot review; acknowledging answers neither question.
	if err := e.ReviewException(taker, x, ExceptionReview{Ack: true}); err == nil {
		t.Fatal("taker reviewed")
	}
	must(t, e.ReviewException(sup, x, ExceptionReview{Ack: true}))
	e.Read(func(s *State) { ex = *s.LocExceptions[x] })
	if !ex.Open() || ex.Located != nil || ex.Followup != nil {
		t.Fatal("ack closed it")
	}
	if err := e.ReviewException(sup, x, ExceptionReview{Followup: &Followup{Disposition: DispSubmitGIS}}); err == nil {
		t.Fatal("follow-up without owner")
	}
	must(t, e.ReviewException(sup, x, ExceptionReview{Located: &Decision{Answer: "yes"}, Followup: &Followup{Disposition: DispSubmitGIS, Owner: "gis-team"}}))
	var draft ShiftReport
	e.Read(func(s *State) {
		ex = *s.LocExceptions[x]
		draft = s.ShiftDraft(s.PeriodAt(c.t), c.t, DutySupervisor, "supervisor")
	})
	if !ex.Open() || ex.Followup.Due.IsZero() {
		t.Fatalf("follow-up %+v", ex.Followup)
	}
	n := 0
	for _, it := range draft.Items {
		if it.Key == "exception:"+x || strings.HasPrefix(it.Key, "followup:") {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("draft items %+v", draft.Items)
	}
	// Merged later: the incident links to the master point, the entered address stays.
	must(t, e.QueueException(sup, x, QueueMerged, "gis-team", "added to the county address points", &PlaceLink{Address: "900 NEW FARM RD", Lat: f64(40.2705), Lon: f64(-76.8805)}))
	must(t, e.ReviewException(sup, x, ExceptionReview{Done: true}))
	e.Read(func(s *State) { in, ex = *s.Incidents[num], *s.LocExceptions[x] })
	if in.Place.Linked == nil || in.Place.Entered != "900 New Farm Rd" || in.Changed == nil || ex.Open() || ex.Queue != QueueMerged {
		t.Fatalf("merged %+v %+v", in.Place, ex)
	}
	replayCheck(t, e, path)
}

func timelineText(in Incident) string {
	var b strings.Builder
	for _, t := range in.Timeline {
		b.WriteString(t.Text + "\n")
	}
	return b.String()
}

func replayCheck(t *testing.T, e *Engine, path string) {
	t.Helper()
	before := snapshotJSON(t, e)
	e.Close()
	e2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()
	if after := snapshotJSON(t, e2); after != before {
		t.Fatal("replay differs")
	}
}

func TestFlagsScopeVersionsAndRestriction(t *testing.T) {
	e, c, _ := finalEngine(t)
	addr := "1210 WILLIAMSON RD NE"
	sc := FlagScope{Address: addr, Lat: f64(40.27), Lon: f64(-76.88), Verified: true, Complex: "Oak Ridge Apartments"}
	whole, err := e.SaveFlag(sup, Flag{Category: "hazard", Title: "Oxygen in use", Warning: "Home oxygen in the leasing office", Instruction: "No open flame", Scope: sc}, false)
	must(t, err)
	apt := sc
	apt.Building, apt.Unit = "4", "204"
	dv, err := e.SaveFlag(sup, Flag{Category: "domestic-violence", Title: "Protective order", Warning: "Active protective order against a former partner", Instruction: "Two units", Scope: apt}, false)
	must(t, err)
	// The same scope and category again is a possible duplicate.
	if _, err := e.SaveFlag(sup, Flag{Category: "hazard", Title: "Oxygen", Warning: "x", Instruction: "y", Scope: sc}, false); err == nil || !strings.Contains(err.Error(), whole) {
		t.Fatalf("duplicate: %v", err)
	}
	// No permanent flag on an unverified address.
	if _, err := e.SaveFlag(sup, Flag{Category: "premise", Title: "x", Warning: "x", Instruction: "x", Scope: FlagScope{Address: "somewhere"}}, false); err == nil {
		t.Fatal("unverified flag")
	}
	// A trainee only proposes.
	tr := Actor{User: "trainee1", Role: "dispatcher", Duty: DutySupervisor}
	prop, err := e.SaveFlag(tr, Flag{Category: "dangerous-animal", Title: "Two dogs", Warning: "Pit bulls in the yard", Instruction: "Call owner", Scope: apt}, false)
	must(t, err)
	num, err := e.CreateIncident(taker, NewIncident{Type: "FALL", Location: &ng911.Location{Address: addr, Lat: f64(40.27), Lon: f64(-76.88)}, LocVerified: true})
	must(t, err)
	// The whole complex: only the complex flag.
	_, err = e.SetPlace(taker, num, Place{Verify: PlaceVerified, Complex: "Oak Ridge Apartments", Building: "2", Unit: "101"})
	must(t, err)
	var in Incident
	e.Read(func(s *State) { in = *s.Incidents[num] })
	if len(in.Flags) != 1 || in.Flags[0].ID != whole {
		t.Fatalf("complex flags %+v", in.Flags)
	}
	// Apartment 204 of building 4: both.
	_, err = e.SetPlace(taker, num, Place{Verify: PlaceVerified, Complex: "Oak Ridge Apartments", Building: "4", Unit: "204"})
	must(t, err)
	e.Read(func(s *State) { in = *s.Incidents[num] })
	if len(in.Flags) != 2 || in.Flags[1].ID != dv && in.Flags[0].ID != dv {
		t.Fatalf("apartment flags %+v", in.Flags)
	}
	// Restricted detail: the call-taker sees the warning title and instruction, not the reason.
	e.Read(func(s *State) {
		f := s.FlagFor(s.Flags[dv], DutyCallTaker, "dispatcher")
		if f.Warning != "" || f.Instruction == "" {
			t.Fatalf("restricted %+v", f)
		}
		if f := s.FlagFor(s.Flags[dv], DutyPolice, "dispatcher"); f.Warning == "" {
			t.Fatal("police cannot see it")
		}
		if s.Flags[prop].State != FlagProposed {
			t.Fatal("trainee published")
		}
	})
	// Retiring makes a new version; the incident keeps the one it showed.
	c.t = c.t.Add(time.Hour)
	if err := e.DecideFlag(taker, dv, "retire", "order expired", time.Time{}); err == nil {
		t.Fatal("taker retired")
	}
	must(t, e.DecideFlag(sup, dv, "retire", "order expired", time.Time{}))
	e.Read(func(s *State) {
		f := s.Flags[dv]
		if f.Version != 2 || len(f.History) != 1 || f.State != FlagRetired {
			t.Fatalf("versions %+v", f)
		}
		if got := s.IncidentFlags(s.Incidents[num], c.t); len(got) != 1 {
			t.Fatalf("still shown %d", len(got))
		}
		if s.Incidents[num].Flags[1].Version != 1 && s.Incidents[num].Flags[0].Version != 1 {
			t.Fatal("history rewritten")
		}
		if near := s.FlagsNear(40.2701, -76.8801, c.t); len(near) != 1 {
			t.Fatalf("near %d", len(near))
		}
	})
}

func TestPersonsSearchSafetyAndIdentifiers(t *testing.T) {
	e, _, _ := finalEngine(t)
	num, err := e.CreateIncident(taker, NewIncident{Type: "FALL", Location: &ng911.Location{Address: "1 MAIN ST"}})
	must(t, err)
	if _, err := e.SavePerson(taker, num, Person{Role: "suspect", First: "John", Last: "Smith", Safety: true}); err == nil || !strings.Contains(err.Error(), "title") {
		t.Fatalf("safety without detail: %v", err)
	}
	if _, err := e.SavePerson(taker, num, Person{Role: "suspect", First: "John", Last: "Smith", Safety: true, SafetyNote: &SafetyConcern{Title: "Knife", Source: "caller", Instruction: "stage"}}); err == nil || !strings.Contains(err.Error(), "concern") {
		t.Fatalf("safety without reason: %v", err)
	}
	pid, err := e.SavePerson(taker, num, Person{Role: "suspect", Roles: []string{"witness"}, First: "John", Middle: "Q", Last: "Smith", DOB: "1980-05-01", SSN: "123456789",
		Phones: []PersonPhone{{Number: "540-555-0101", Type: "mobile"}}, Safety: true,
		SafetyNote: &SafetyConcern{Title: "Threatened officers", Reason: "Said he would fight police", Source: "caller", Instruction: "Wait for backup", Scope: "permanent"}})
	must(t, err)
	_, err = e.SavePerson(taker, num, Person{Role: "victim", First: "Johnny", Last: "Jones", ApproxAge: "40"})
	must(t, err)
	_, err = e.SavePerson(taker, num, Person{Role: "witness", First: "Mary", Last: "Johnson"})
	must(t, err)
	e.Read(func(s *State) {
		p := s.Incidents[num].Persons[0]
		if p.Name != "SMITH, JOHN Q" || p.SSN != "123-45-6789" || len(p.Roles) != 2 || p.SafetyNote.Flag == "" {
			t.Fatalf("person %+v", p)
		}
		if f := s.Flags[p.SafetyNote.Flag]; f.State != FlagProposed || f.Scope.Person == "" {
			t.Fatalf("person flag %+v", f)
		}
		hits := s.FindPersons(PersonQuery{First: "John"}, 0)
		if len(hits) != 1 || hits[0].Last != "Smith" || hits[0].SSN != "***-**-6789" {
			t.Fatalf("first-name search %+v", hits)
		}
		if h := s.FindPersons(PersonQuery{First: "Jon", Phonetic: true}, 0); len(h) != 2 {
			t.Fatalf("phonetic %+v", h)
		}
		if h := s.FindPersons(PersonQuery{Phone: "5550101"}, 0); len(h) != 1 {
			t.Fatalf("phone %+v", h)
		}
		if m := s.PossibleMatches(Person{First: "John", Last: "Smith"}); len(m) != 1 {
			t.Fatalf("matches %+v", m)
		}
	})
	if _, err := e.OpenIdentifiers(taker, num, pid, "check"); err == nil {
		t.Fatal("call-taker opened the SSN")
	}
	id, err := e.OpenIdentifiers(cop, num, pid, "warrant check")
	must(t, err)
	if id.SSN != "123-45-6789" {
		t.Fatal(id)
	}
}

func TestVehiclesReturnsHitConfirmationAndTows(t *testing.T) {
	e, c, _ := finalEngine(t)
	num, err := e.CreateIncident(cop, NewIncident{Type: "FALL", Location: &ng911.Location{Address: "1 MAIN ST"}})
	must(t, err)
	p1, _ := e.SavePerson(cop, num, Person{Role: "involved", First: "Ann", Last: "Lee"})
	p2, _ := e.SavePerson(cop, num, Person{Role: "involved", First: "Bob", Last: "Ray"})
	if _, err := e.SaveVehicle(cop, num, Vehicle{Role: "suspect", Plate: "abc1234", PlateType: "ZZ"}); err == nil {
		t.Fatal("bad plate type")
	}
	v1, err := e.SaveVehicle(cop, num, Vehicle{Role: "suspect", Roles: []string{"stolen"}, Plate: "abc1234", PlateType: "PC", Color: "BLU", VIN: "1HGCM82633A004352",
		Links: []VehicleLink{{Person: p1, Relation: "driver"}}})
	must(t, err)
	v2, err := e.SaveVehicle(cop, num, Vehicle{Role: "victim", Plate: "XYZ9", State: "NC", Links: []VehicleLink{{Person: p2, Relation: "owner"}}})
	must(t, err)
	rq, err := e.LogRequest(cop, Request{Type: "ncic", Category: "vehicle", Incident: num, Record: v1, Requester: "AD6"})
	must(t, err)
	rt, err := e.AttachReturn(cop, num, v1, InqReturn{Request: rq, Source: "ncic", Requester: "AD6", Fields: map[string]string{"color": "GRY", "plate": "ABC1234", "owner": "DOE, JANE"},
		Hit: "stolen", HitORI: "VA0000000"})
	must(t, err)
	e.Read(func(s *State) {
		in := s.Incidents[num]
		v := in.Vehicles[0]
		if v.State != "VA" || v.Color != "BLU" || len(v.Returns) != 1 || len(v.Discrepancies()) != 1 || s.Requests[rq].Status != RqPossibleHit {
			t.Fatalf("vehicle %+v %v %s", v, v.Discrepancies(), s.Requests[rq].Status)
		}
		if got := in.VehiclesOf(p1); len(got) != 1 || !strings.Contains(got[0], "driver") {
			t.Fatalf("both directions %v", got)
		}
	})
	hc, err := e.SendHitConfirmation(cop, num, v1, rt, Request{Requester: "AD6"})
	must(t, err)
	if err := e.UpdateRequest(cop, hc, RequestUpdate{Status: RqConfirmed}); err == nil {
		t.Fatal("confirmed without responder")
	}
	e.Read(func(s *State) {
		if r := s.Requests[hc]; r.Due.Sub(r.SentAt) != 10*time.Minute || r.Destination != "VA0000000" {
			t.Fatalf("hit confirmation %+v", r)
		}
		if findReturn(s.Incidents[num], v1, rt).Confirmed != "" {
			t.Fatal("confirmed by sending")
		}
	})
	must(t, e.UpdateRequest(cop, hc, RequestUpdate{Status: RqConfirmed, Responder: "Officer at VA0000000", Note: "hold the vehicle"}))
	e.Read(func(s *State) {
		if findReturn(s.Incidents[num], v1, rt).Confirmed != RqConfirmed || s.Requests[rq].Status != RqConfirmed {
			t.Fatal("not confirmed")
		}
	})
	// Tows: one per vehicle.
	must(t, e.SaveSettings(SystemActor, func() Settings { var st Settings; e.Read(func(s *State) { st = s.Settings }); return st }()))
	_, err = e.SaveTowCompany(SystemActor, TowCompany{Name: "Acme Towing", Phone: "555-0100", Classes: []string{"light"}})
	must(t, err)
	_, err = e.SaveTowCompany(SystemActor, TowCompany{Name: "Best Towing", Phone: "555-0200", Classes: []string{"light"}})
	must(t, err)
	if _, err := e.RequestVehicleTow(cop, TowAsk{Incident: num, Class: "light"}); err == nil {
		t.Fatal("tow with no vehicle")
	}
	t1, err := e.RequestVehicleTow(cop, TowAsk{Incident: num, Vehicle: v1, Class: "light", Requester: "ad6", Reason: "stolen recovered"})
	must(t, err)
	must(t, e.TowAnswer(cop, t1.ID, TowDeclined, "no truck free"))
	t2, err := e.RequestVehicleTow(cop, TowAsk{Incident: num, Vehicle: v1, Class: "light", Requester: "AD6"})
	must(t, err)
	if t2.Company == t1.Company {
		t.Fatal("declined company offered again")
	}
	must(t, e.UpdateTow(cop, TowDetail{ID: t2.ID, Step: "accepted"}))
	c.t = c.t.Add(20 * time.Minute)
	must(t, e.UpdateTow(cop, TowDetail{ID: t2.ID, Step: "arrived", Driver: "Sam", Destination: "Acme lot", Hold: "evidence hold"}))
	t3, err := e.RequestVehicleTow(cop, TowAsk{Incident: num, Vehicle: v2, Class: "light"})
	must(t, err)
	e.Read(func(s *State) {
		if got := s.TowsOf(num, v1); len(got) != 2 || got[1].Destination != "Acme lot" || got[0].Requester != "AD6" || got[1].Times["arrived"].IsZero() {
			t.Fatalf("v1 tows %+v", got)
		}
		if got := s.TowsOf(num, v2); len(got) != 1 || got[0].ID != t3.ID {
			t.Fatalf("v2 tows %+v", got)
		}
	})
	// SE.AD6 then F10 opens the tow form for AD6.
	if r := e.Execute(cop, "SE.AD6", num); r.Form != "tow" || r.Unit != "AD6" {
		t.Fatalf("SE.AD6: %+v", r)
	}
}

func TestRequestsHandoffAndSignOff(t *testing.T) {
	e, c, _ := finalEngine(t)
	rq, err := e.LogRequest(cop, Request{Type: "dmv", Category: "plate", Subject: "ABC1234 VA", Requester: "AD6"})
	must(t, err)
	var block []*Request
	e.Read(func(s *State) { block = s.SignOffBlockers("pd1") })
	if len(block) != 1 {
		t.Fatal("no blocker")
	}
	if err := e.HandoffRequest(cop, rq, "nobody", "x"); err == nil {
		t.Fatal("handed to nobody")
	}
	must(t, e.HandoffRequest(cop, rq, "sup1", "awaiting DMV return"))
	e.Read(func(s *State) { block = s.SignOffBlockers("pd1") })
	if len(block) != 0 {
		t.Fatal("still blocked after handoff")
	}
	must(t, e.AcceptRequest(sup, rq))
	c.t = c.t.Add(16 * time.Minute)
	over, err := e.WarnRequests()
	must(t, err)
	e.Read(func(s *State) {
		r := s.Requests[rq]
		if r.Owner != "sup1" || len(r.Chain) != 3 || len(over) != 1 || !r.Overdue {
			t.Fatalf("handoff %+v", r)
		}
	})
	if again, _ := e.WarnRequests(); len(again) != 0 {
		t.Fatal("warned twice")
	}
}

func TestCriticalCaseOneAlertTwoTriggers(t *testing.T) {
	e, c, path := finalEngine(t)
	var st Settings
	e.Read(func(s *State) { st = s.Settings })
	st.Critical.Phrases = []phrase.Rule{{ID: "shots", Name: "Shots fired", Phrases: []string{"shots fired"}}}
	st.Critical.Rules = []CriticalRule{{ID: "r1", Name: "Robbery", Kind: "calltype", Match: "FALL"}, {ID: "r2", Name: "Shots fired heard", Kind: "phrase", Match: "shots"}}
	must(t, e.SaveSettings(SystemActor, st))
	must(t, e.UpsertCall(BitVoiceActor, "c9", CallData{Source: "bitvoice", State: "answered", Callback: "+15405550100", Start: c.t, Answer: c.t.Add(8 * time.Second), Agent: "ctaker"}))
	c.t = c.t.Add(40 * time.Second)
	num, err := e.CreateIncident(taker, NewIncident{Type: "FALL", Call: "c9", Location: &ng911.Location{Address: "1 MAIN ST"}, LocVerified: true})
	must(t, err)
	must(t, e.AddNote(taker, num, "caller says there are no shots fired yet", false))
	c.t = c.t.Add(time.Minute)
	must(t, e.Dispatch(cop, num, []string{"P1"}, nil))
	must(t, e.RadioText(SystemActor, RadioLine{Unit: "P1", Text: "shots fired, suspect northbound on foot, run a plate for me"}))
	must(t, e.RadioText(SystemActor, RadioLine{Unit: "P1", Text: "shots fired again"}))
	var cs []*CriticalCase
	var alerts []*PhraseAlert
	e.Read(func(s *State) {
		cs = s.CriticalList(true)
		alerts = s.PhraseAlerts
	})
	if len(cs) != 1 || len(cs[0].Triggers) != 2 {
		t.Fatalf("cases %+v", cs[0].Triggers)
	}
	if len(alerts) != 2 || !alerts[0].Negated || alerts[1].Negated || alerts[1].Count != 2 {
		t.Fatalf("alerts %+v %+v", alerts[0], alerts)
	}
	must(t, e.AckCritical(sup, cs[0].ID))
	evs, err := e.LedgerEvents()
	must(t, err)
	var rep CriticalReport
	e.Read(func(s *State) { rep, err = s.BuildCriticalReport(cs[0].ID, evs, time.Time{}, c.t) })
	must(t, err)
	var answer, dispatch *QARow
	for i := range rep.QA {
		switch rep.QA[i].Measure.ID {
		case "answer":
			answer = &rep.QA[i]
		case "dispatch":
			dispatch = &rep.QA[i]
		case "requests":
			if !strings.Contains(rep.QA[i].Prompt, "Possible gap") {
				t.Fatalf("request gap %+v", rep.QA[i])
			}
		}
	}
	if answer == nil || answer.Seconds == nil || *answer.Seconds != 8 || dispatch == nil || dispatch.Seconds == nil || *dispatch.Seconds != 92 || dispatch.ToSeq == 0 {
		t.Fatalf("intervals %+v %+v", answer, dispatch)
	}
	if !strings.Contains(rep.Text(time.UTC), "QA MATRIX") || len(rep.Radio) == 0 {
		t.Fatal("report text")
	}
	if err := e.UpdateCase(sup, cs[0].ID, CaseUpdate{Review: ReviewComplete}); err == nil {
		t.Fatal("completed with QA unanswered")
	}
	if err := e.AnswerQA(sup, cs[0].ID, QAAnswer{Measure: "answer", Result: "met"}); err == nil {
		t.Fatal("answer without citation")
	}
	must(t, e.AnswerQA(sup, cs[0].ID, QAAnswer{Measure: "answer", Result: "met", Citation: "call c9 answered"}))
	replayCheck(t, e, path)
}

func TestShiftPeriodsFinalizeAddendumAndCarryForward(t *testing.T) {
	e, c, _ := finalEngine(t)
	var st Settings
	e.Read(func(s *State) { st = s.Settings })
	st.Shift.TimeZone = "UTC"
	must(t, e.SaveSettings(SystemActor, st))
	var p Period
	e.Read(func(s *State) { p = s.PeriodAt(c.t) }) // 22:00 UTC: the night that began 19:00
	if p.ID != "2026-10-18/Night" || p.To.Hour() != 7 {
		t.Fatalf("period %+v", p)
	}
	must(t, e.SaveFollowup(sup, "", "", "Call the county GIS office", "sup1", time.Time{}, false, ""))
	v, err := e.FinalizeShift(sup, p.ID, "Quiet night", "GIS item open", "")
	must(t, err)
	if v != 1 {
		t.Fatal(v)
	}
	if _, err := e.FinalizeShift(sup, p.ID, "", "", ""); err == nil {
		t.Fatal("addendum without why")
	}
	var fu string
	e.Read(func(s *State) {
		for id := range s.Followups {
			fu = id
		}
	})
	c.t = c.t.Add(10 * time.Hour) // the next day period
	must(t, e.AckShift(Actor{User: "pd1", Role: "supervisor", Duty: DutySupervisor}, p.ID, []string{fu}, nil, "took it"))
	e.Read(func(s *State) {
		next := s.PeriodAt(c.t)
		d := s.ShiftDraft(next, c.t, DutySupervisor, "supervisor")
		n := 0
		for _, it := range d.Items {
			if it.Section == "followups" {
				n++
				if !strings.Contains(it.Status, "carried") || it.Owner != "pd1" {
					t.Fatalf("carried %+v", it)
				}
			}
		}
		if n != 1 || len(s.ShiftReports[p.ID][0].Acks) != 1 {
			t.Fatalf("carry forward %d", n)
		}
	})
}
