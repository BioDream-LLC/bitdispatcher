package cad

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestFieldInitiateAndUnitStatus(t *testing.T) {
	e, c, path := testEngine(t)
	must(t, e.SaveUnit(SystemActor, UnitDef{ID: "UNIT1", Type: "patrol"}))
	must(t, e.SetStatus(SystemActor, "UNIT1", Available, "", ""))
	var set Settings
	e.Read(func(s *State) { set = s.Settings })
	set.Commands.StatusCodes = append(DefaultStatusCodes(), StatusCode{Code: "10-97", Status: OnScene, Retired: true})
	for i := range set.MarkOuts {
		if set.MarkOuts[i].Code == "COURT" {
			set.MarkOuts[i].Retired = true
		}
	}
	must(t, e.SaveSettings(SystemActor, set))
	ct := CallType{}
	e.Read(func(s *State) { ct = *s.CallTypes["TS"] })
	ct.CheckMinutes = 5
	must(t, e.SaveCallType(SystemActor, ct))
	// F7: the traffic stop in one pass, the third field skipped by two delimiters.
	res := e.Execute(dispatcher, "FI UNIT1.8TH/STEWART..ABC1234", "")
	if !res.OK || res.Incident == "" || res.Plate != "ABC1234" {
		t.Fatalf("field initiate: %+v", res)
	}
	e.Read(func(s *State) {
		in, u := s.Incidents[res.Incident], s.Units["UNIT1"]
		if in.Type != "TS" || in.Location.Address != "8TH & STEWART" || len(in.Vehicles) != 1 || in.Vehicles[0].Plate != "ABC1234" {
			t.Fatalf("stop %+v %+v", in.Location, in.Vehicles)
		}
		if u.Status != OnScene || u.Incident != res.Incident || s.TimerForUnit(u) != 5 {
			t.Fatalf("unit %s %s timer %d", u.Status, u.Incident, s.TimerForUnit(u))
		}
	})
	if !strings.Contains(res.Message, "status check in 5 min") {
		t.Fatalf("message %q", res.Message)
	}
	// It becomes a full incident without retyping: change the type, the unit stays.
	if r := e.Execute(dispatcher, "T ROB", res.Incident); !r.OK && !strings.Contains(r.Message, "no call type") {
		t.Fatalf("convert: %+v", r)
	}
	// F11: only the unit opens the form, nothing guessed.
	if r := e.Execute(dispatcher, "US UNIT1", ""); r.OK || r.Form != "unit-status" || r.Unit != "UNIT1" {
		t.Fatalf("unit only: %+v", r)
	}
	c.t = c.t.Add(time.Minute)
	must(t, e.SetStatus(dispatcher, "UNIT1", Enroute, res.Incident, ""))
	if r := e.Execute(dispatcher, "US UNIT1.AS", ""); !r.OK || r.Incident != res.Incident {
		t.Fatalf("UNIT1.AS: %+v", r)
	}
	e.Read(func(s *State) {
		if s.Units["UNIT1"].Status != OnScene {
			t.Fatal("not on scene")
		}
	})
	// Retired codes and an unauthorised duty are refused.
	if r := e.Execute(dispatcher, "US UNIT1.10-97", ""); r.OK || !r.Refused {
		t.Fatalf("retired code: %+v", r)
	}
	if r := e.Execute(dispatcher, "MO UNIT1 COURT", ""); r.OK || !r.Refused {
		t.Fatalf("retired mark-out: %+v", r)
	}
	calltaker := Actor{User: "ct", Role: "dispatcher", Duty: "calltaker"}
	if r := e.Execute(calltaker, "US UNIT1.AV", ""); r.OK || !r.Refused {
		t.Fatalf("call-taker status: %+v", r)
	}
	// Administrative mark-out in the F11 grammar, a status check, back available.
	if r := e.Execute(dispatcher, "US UNIT1.ADMIN.  .HQ ANNEX.  .paperwork", ""); !r.OK {
		t.Fatalf("admin mark-out: %+v", r)
	}
	e.Read(func(s *State) {
		u := s.Units["UNIT1"]
		if u.Status != MarkedOut || u.MarkOut.Code != "ADMIN" || u.MarkOut.Note != "paperwork" {
			t.Fatalf("mark-out %+v", u.MarkOut)
		}
	})
	if r := e.Execute(dispatcher, "SC UNIT1", ""); !r.OK {
		t.Fatal(r.Message)
	}
	if r := e.Execute(dispatcher, "US UNIT1.AV", ""); !r.OK {
		t.Fatal(r.Message)
	}
	// Two passes: only the mark-out.
	set.Commands.TwoPass = true
	must(t, e.SaveSettings(SystemActor, set))
	if r := e.Execute(dispatcher, "FI UNIT1.MAIN/ELM..XYZ9", ""); !r.OK || r.Incident != "" {
		t.Fatalf("two pass: %+v", r)
	}
	e.Read(func(s *State) {
		if u := s.Units["UNIT1"]; u.Status != MarkedOut || u.MarkOut.Code != "TS" {
			t.Fatalf("two-pass unit %+v", u.MarkOut)
		}
	})
	if p := ClassicKeys(); len(KeyConflicts(p)) != 0 || p.Bindings[21].Action != "display911" {
		t.Fatalf("classic keys %v", KeyConflicts(p))
	}
	bad := KeyProfile{Bindings: []KeyBinding{{Key: "F7", Action: "fieldInitiate"}, {Key: "F7", Action: "aerial"}, {Key: "Ctrl+W", Action: "help"}}}
	if len(KeyConflicts(bad)) != 2 {
		t.Fatalf("conflicts %v", KeyConflicts(bad))
	}
	replayed(t, e, path)
}

func TestWorkingCallsAndSearch(t *testing.T) {
	e, _, path := testEngine(t)
	ct := Actor{User: "amy", Role: "dispatcher", Duty: "calltaker", Console: "CT1"}
	var ids []string
	for i := 0; i < 3; i++ {
		id, err := e.OpenDraft(ct, "")
		must(t, err)
		ids = append(ids, id)
	}
	if _, err := e.OpenDraft(ct, ""); err == nil {
		t.Fatal("a fourth working call over the limit")
	}
	if err := e.CloseDraft(ct, ids[1], "discarded", "", "", true); err == nil {
		t.Fatal("discarded a populated draft without a reason")
	}
	n, err := e.CreateIncident(ct, NewIncident{Type: "TS", Location: at(40.27, -76.88, "10 Market St")})
	must(t, err)
	must(t, e.CloseDraft(ct, ids[1], "submitted", n, "", true))
	e.Read(func(s *State) {
		open := s.DraftsOf("amy")
		if len(open) != 2 || open[0].ID != ids[0] || open[1].ID != ids[2] {
			t.Fatalf("open drafts %+v", open)
		}
	})
	sup := Actor{User: "sue", Role: "supervisor"}
	must(t, e.MoveDraft(sup, ids[2], "bob", "amy went home sick"))
	must(t, e.SaveStaff(SystemActor, Staff{Username: "tim", Trainee: true}))
	e.Read(func(s *State) {
		if len(s.DraftsOf("bob")) != 1 || s.MaxDrafts("tim", "calltaker") != 1 {
			t.Fatal("move or trainee limit")
		}
	})
	// Search: a plate, a partial plate, a name in another order, a similar spelling, a description.
	_, err = e.SaveVehicle(ct, n, Vehicle{Role: "suspect", Plate: "ABC1234", State: "VA", Make: "Honda", Model: "Civic", Color: "Silver"})
	must(t, err)
	_, err = e.SavePerson(ct, n, Person{Role: "suspect", Name: "DOE, JOHN", Phone: "540-555-0199", Clothing: "red hoodie, jeans"})
	must(t, err)
	_, err = e.SendComm(Actor{User: "pat", Role: "dispatcher"}, Comm{Kind: CommBOLO, Subject: "Robbery suspect", Body: "Male in a red hoodie", Plate: "ABC1234", Vehicle: "silver Honda Civic", Person: "male, red hoodie"})
	must(t, err)
	r := Reader{User: "amy", Role: "dispatcher", Duty: "calltaker", Console: "CT1"}
	e.Read(func(s *State) {
		res := s.Find(FindQuery{Text: "ABC1234"}, r, e.now())
		if len(res.Groups["vehicles"]) != 2 || !res.Groups["vehicles"][0].Confirmed || res.Groups["vehicles"][0].Why == "" {
			t.Fatalf("plate %+v", res.Groups["vehicles"])
		}
		if res := s.Find(FindQuery{Text: "ABC12"}, r, e.now()); len(res.Groups["vehicles"]) == 0 || res.Groups["vehicles"][0].Strength != "partial" {
			t.Fatalf("partial plate %+v", res.Groups["vehicles"])
		}
		if res := s.Find(FindQuery{Text: "John Doe"}, r, e.now()); len(res.Groups["people"]) != 1 || res.Groups["people"][0].Strength != "exact" {
			t.Fatalf("name %+v", res.Groups["people"])
		}
		if res := s.Find(FindQuery{Text: "Jon Doe", Fuzzy: true}, r, e.now()); len(res.Groups["people"]) != 1 || res.Groups["people"][0].Strength != "similar" || res.Groups["people"][0].Confirmed {
			t.Fatalf("similar name %+v", res.Groups["people"])
		}
		res = s.Find(FindQuery{Text: "red hoodie"}, r, e.now())
		if len(res.Groups["people"]) < 2 || slices.ContainsFunc(res.Groups["people"], func(f Found) bool { return f.Field == "description" && f.Confirmed }) {
			t.Fatalf("description %+v", res.Groups["people"])
		}
		if len(res.Groups["comms"]) != 1 {
			t.Fatalf("bolo by description %+v", res.Groups["comms"])
		}
		if res := s.Find(FindQuery{Text: "5405550199"}, r, e.now()); len(res.Groups["people"]) != 1 || res.Groups["people"][0].Why != "shared phone" {
			t.Fatalf("phone %+v", res.Groups["people"])
		}
	})
	replayed(t, e, path)
}
