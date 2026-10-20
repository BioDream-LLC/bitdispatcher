package cad

import (
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// What each role may do is enforced by the engine, not only by the screen.
func TestDutiesLimitWhoSendsUnits(t *testing.T) {
	e, _, _ := testEngine(t)
	num, err := e.CreateIncident(dispatcher, NewIncident{Type: "SF", Location: ng911.Point(40.27, -76.88)})
	if err != nil {
		t.Fatal(err)
	}
	taker := Actor{User: "ct", Role: "dispatcher", Duty: DutyCallTaker}
	police := Actor{User: "pd", Role: "dispatcher", Duty: DutyPolice}
	fire := Actor{User: "fd", Role: "dispatcher", Duty: DutyFire}
	if err := e.Dispatch(taker, num, []string{"E1"}, nil); err == nil || !strings.Contains(err.Error(), "call-taker") {
		t.Fatalf("a call-taker dispatched: %v", err)
	}
	if err := e.Dispatch(police, num, []string{"E1"}, nil); err == nil || !strings.Contains(err.Error(), "fire unit") {
		t.Fatalf("police sent a fire unit: %v", err)
	}
	if err := e.Dispatch(fire, num, []string{"E1"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := e.SetStatus(police, "E1", Enroute, "", ""); err == nil {
		t.Fatal("police changed a fire unit's status")
	}
	// The agency can allow both.
	var st Settings
	e.Read(func(s *State) { st = s.Settings })
	st.Duties = DutyRules{CallTakersDispatch: true, CrossDiscipline: true}
	if err := e.SaveSettings(SystemActor, st); err != nil {
		t.Fatal(err)
	}
	if err := e.Dispatch(police, num, []string{"E2"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := e.Dispatch(taker, num, []string{"L1"}, nil); err != nil {
		t.Fatal(err)
	}
	// Records staff never change incidents.
	if _, err := e.SavePerson(Actor{User: "rec", Duty: DutyRecords}, num, Person{Name: "X"}); err == nil {
		t.Fatal("records staff changed an incident")
	}
}

// Call entry: modifiers raise the priority and add notifications; people, vehicles, contacts, the caller's
// own location; a newer caller location is offered and decided; a unit acknowledges.
func TestCallEntryRecords(t *testing.T) {
	e, c, _ := testEngine(t)
	lat, lon := 40.2633, -76.8823
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(e.UpsertCall(BitVoiceActor, "c1", CallData{Source: "bitvoice", State: "answered", Callback: "+15405550100",
		Location: &ng911.Location{Lat: &lat, Lon: &lon, Radius: 8, At: c.t}}))
	num, err := e.CreateIncident(dispatcher, NewIncident{Type: "FALL", Call: "c1", Modifiers: []string{"ELDERLY"}, LocVerified: true})
	must(err)
	must(e.SetModifiers(dispatcher, num, []string{"WEAPONS", "MENTAL"}))
	pid, err := e.SavePerson(dispatcher, num, Person{Role: "Suspect", Name: "DOE, JOHN", Clothing: "black hoodie", Safety: true})
	must(err)
	_, err = e.SavePerson(dispatcher, num, Person{ID: pid, Role: "suspect", Name: "DOE, JOHN Q"})
	must(err)
	_, err = e.SaveVehicle(dispatcher, num, Vehicle{Role: "suspect", Plate: "ktr 4471", State: "va", Make: "Honda", Model: "Civic", Color: "silver"})
	must(err)
	_, err = e.SaveContact(dispatcher, num, Contact{Kind: "transfer", Who: "Poison control", Detail: "warm transfer"})
	must(err)
	must(e.SetCallerLocation(dispatcher, num, &ng911.Location{Address: "12 Oak St"}))
	// The phone moves 300 m: offered, not applied.
	c.t = c.t.Add(time.Minute)
	lat2 := lat + 0.0027
	must(e.UpsertCall(BitVoiceActor, "c1", CallData{Source: "rebid", Location: &ng911.Location{Lat: &lat2, Lon: &lon, Radius: 6, Source: "rebid", At: c.t}}))
	var in Incident
	e.Read(func(s *State) { in = *s.Incidents[num] })
	if in.Priority != 1 || len(in.Modifiers) != 2 || len(in.Persons) != 1 || in.Persons[0].Name != "DOE, JOHN Q" || in.Vehicles[0].Plate != "KTR4471" {
		t.Fatalf("entry: %+v", in)
	}
	if len(in.Notices) != 2 || in.Notices[0].Who != "Crisis intervention team" || !in.Notices[0].Auto || in.CallerAt == nil {
		t.Fatalf("notices %+v callerAt %+v", in.Notices, in.CallerAt)
	}
	if in.NewLocation == nil || in.NewLocation.Metres < 250 || *in.Location.Lat != lat {
		t.Fatalf("offer %+v", in.NewLocation)
	}
	must(e.DecideLocation(dispatcher, num, true, "caller walked to the corner"))
	e.Read(func(s *State) { in = *s.Incidents[num] })
	if *in.Location.Lat != lat2 || in.NewLocation != nil || in.Changed == nil || *in.Changed.From.Lat != lat || in.LocVerified {
		t.Fatalf("accepted: %+v", in)
	}
	// Ack.
	must(e.Dispatch(dispatcher, num, []string{"M1"}, nil))
	must(e.AckUnit(dispatcher, "M1", "radio"))
	e.Read(func(s *State) { in = *s.Incidents[num] })
	if in.Units[0].Acked.IsZero() {
		t.Fatal("not acknowledged")
	}
}

// A response plan puts its units first, and says who goes when a planned unit cannot.
func TestResponsePlanOrder(t *testing.T) {
	e, _, _ := testEngine(t)
	_, err := e.SavePlan(SystemActor, ResponsePlan{Name: "Station 1 structure fire", CallTypes: []string{"SF"}, Area: "ST1", Order: []string{"E2", "E1"}})
	if err != nil {
		t.Fatal(err)
	}
	num, err := e.CreateIncident(dispatcher, NewIncident{Type: "SF", Location: &ng911.Location{Lat: ptr(40.2701), Lon: ptr(-76.8801), Address: "1 Main St"}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Recommend(num)
	if err != nil {
		t.Fatal(err)
	}
	if r.Plan == "" || len(r.Planned) != 2 || r.Lines[0].Units[0].Unit != "E2" || !strings.Contains(r.Lines[0].Units[0].Reasons[0], "1st due") {
		t.Fatalf("plan: %+v", r)
	}
	// E2 goes elsewhere: E1 is next due, instead of E2.
	other, _ := e.CreateIncident(dispatcher, NewIncident{Type: "SF", Location: ng911.Point(40.30, -76.80)})
	if err := e.Dispatch(dispatcher, other, []string{"E2"}, nil); err != nil {
		t.Fatal(err)
	}
	r, _ = e.Recommend(num)
	if r.Lines[0].Units[0].Unit != "E1" || r.Planned[0].Can || !strings.Contains(strings.Join(r.Lines[0].Units[0].Reasons, " "), "instead of E2") {
		t.Fatalf("next due: %+v", r)
	}
	if err := e.DispatchWhy(dispatcher, num, []string{"L1"}, r.Units, "crew asked for the ladder", r.Plan); err != nil {
		t.Fatal(err)
	}
	var tl []Entry
	e.Read(func(s *State) { tl = s.Incidents[num].Timeline })
	found := false
	for _, x := range tl {
		found = found || strings.Contains(x.Text, "why: crew asked") && strings.Contains(x.Text, "plan: Station 1")
	}
	if !found {
		t.Fatalf("reason not recorded: %+v", tl)
	}
}

// A crash notification is data only: it ends when it is put on an incident.
func TestDataCallEndsOnIncident(t *testing.T) {
	e, _, _ := testEngine(t)
	if err := e.UpsertCall(Actor{User: "acn"}, "acn1", CallData{Source: "acn", State: "data", Location: ng911.Point(40.27, -76.88)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.CreateIncident(dispatcher, NewIncident{Type: "MVA", Call: "acn1"}); err != nil {
		t.Fatal(err)
	}
	var st string
	e.Read(func(s *State) { st = s.Calls["acn1"].State })
	if st != "ended" {
		t.Fatalf("state %s", st)
	}
}

func ptr(f float64) *float64 { return &f }
