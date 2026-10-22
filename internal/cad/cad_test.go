package cad

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

var dispatcher = Actor{User: "jlee", Role: "dispatcher", Console: "D1"}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func testEngine(t *testing.T) (*Engine, *clock, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	e, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c := &clock{time.Date(2026, 10, 18, 22, 0, 0, 0, time.UTC)}
	e.now = c.now
	t.Cleanup(func() { e.Close() })
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(e.SaveStation(SystemActor, Station{ID: "ST1", Name: "Station 1", Lat: 40.2700, Lon: -76.8800}))
	must(e.SaveStation(SystemActor, Station{ID: "ST2", Name: "Station 2", Lat: 40.3000, Lon: -76.8000}))
	for _, u := range []UnitDef{
		{ID: "E1", Type: "engine", Station: "ST1"}, {ID: "E2", Type: "engine", Station: "ST2"},
		{ID: "M1", Type: "medic", Station: "ST1", RadioID: 101}, {ID: "M2", Type: "medic", Station: "ST2"},
		{ID: "L1", Type: "ladder", Station: "ST1"}, {ID: "P1", Type: "patrol", RadioID: 197, RadioAlias: "Adam 1"},
	} {
		must(e.SaveUnit(SystemActor, u))
		must(e.SetStatus(SystemActor, u.ID, Available, "", ""))
	}
	return e, c, path
}

func TestCallToIncidentToClose(t *testing.T) {
	e, c, path := testEngine(t)
	lat, lon := 40.2633, -76.8823
	err := e.UpsertCall(BitVoiceActor, "abc123", CallData{Source: "bitvoice", State: "ringing", Callback: "+17175550100", Emergency: true,
		Location: &ng911.Location{Lat: &lat, Lon: &lon, Radius: 35, Address: "123 N Front St", Source: "bitvoice"}})
	if err != nil {
		t.Fatal(err)
	}
	// The same snapshot again writes nothing.
	seq := e.led.Seq()
	e.UpsertCall(BitVoiceActor, "abc123", CallData{Source: "bitvoice", State: "ringing", Callback: "+17175550100"})
	if e.led.Seq() != seq {
		t.Fatal("an unchanged call snapshot was written")
	}
	c.t = c.t.Add(10 * time.Second)
	e.UpsertCall(BitVoiceActor, "abc123", CallData{Source: "eido", State: "answered", Agent: "jlee", Answer: c.t})
	num, err := e.CreateIncident(dispatcher, NewIncident{Type: "chest", Call: "abc123", Narrative: "62M chest pain"})
	if err != nil {
		t.Fatal(err)
	}
	if num != "26-000001" {
		t.Fatalf("number %s", num)
	}
	var in Incident
	e.Read(func(s *State) { in = *s.Incidents[num] })
	if in.Priority != 1 || in.Caller.Callback != "+17175550100" || !in.Location.HasPoint() || in.Calls[0] != "abc123" || in.Status != Pending {
		t.Fatalf("incident %+v", in)
	}
	rec, err := e.Recommend(num)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Units) != 1 || rec.Units[0] != "M1" || rec.Lines[0].Units[0].From != "station" {
		t.Fatalf("recommendation %+v", rec)
	}
	if len(rec.Lines[0].Next) != 1 || rec.Lines[0].Next[0].Unit != "M2" {
		t.Fatalf("next %+v", rec.Lines[0].Next)
	}
	// The dispatcher sends M2 as well as the recommended M1.
	if err := e.Dispatch(dispatcher, num, []string{"m1", "M2"}, rec.Units); err != nil {
		t.Fatal(err)
	}
	c.t = c.t.Add(time.Minute)
	if r := e.Execute(dispatcher, "ER M1 M2", num); !r.OK {
		t.Fatal(r.Message)
	}
	c.t = c.t.Add(4 * time.Minute)
	if r := e.Execute(dispatcher, "OS M1", num); !r.OK {
		t.Fatal(r.Message)
	}
	if r := e.Execute(dispatcher, "AV M2", num); !r.OK {
		t.Fatal(r.Message)
	}
	if r := e.Execute(dispatcher, "N patient conscious, breathing", num); !r.OK {
		t.Fatal(r.Message)
	}
	e.Read(func(s *State) { in = *s.Incidents[num] })
	if in.Status != Active || len(in.Units) != 2 || in.Units[0].OnScene.IsZero() || in.Units[1].Cleared.IsZero() {
		t.Fatalf("after statuses %+v", in.Units)
	}
	var timeline []string
	for _, x := range in.Timeline {
		timeline = append(timeline, x.Text)
	}
	joined := strings.Join(timeline, " | ")
	for _, want := range []string{"Dispatched M1 M2 (not recommended: M2)", "M1 en route", "M1 on scene", "M2 cleared", "patient conscious"} {
		if !strings.Contains(joined, want) {
			t.Errorf("timeline lacks %q: %s", want, joined)
		}
	}
	if r := e.Execute(dispatcher, "CL transported", num); !r.OK {
		t.Fatal(r.Message)
	}
	var m1 Unit
	e.Read(func(s *State) { in, m1 = *s.Incidents[num], *s.Units["M1"] })
	if in.Status != Closed || in.Disposition != "Transported" || m1.Status != Available || m1.Incident != "" {
		t.Fatalf("closed %+v %+v", in.Status, m1)
	}
	// Replaying the ledger gives the same state.
	before := snapshotJSON(t, e)
	e.Close()
	e2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()
	if after := snapshotJSON(t, e2); after != before {
		t.Fatalf("replay differs\nbefore %s\nafter  %s", before, after)
	}
}

func snapshotJSON(t *testing.T, e *Engine) string {
	var b []byte
	e.Read(func(s *State) { b, _ = json.Marshal(s) })
	return string(b)
}

func TestRecommendationCountsUnitsOnScene(t *testing.T) {
	e, _, _ := testEngine(t)
	lat, lon := 40.271, -76.881
	num, err := e.CreateIncident(dispatcher, NewIncident{Type: "SF", Location: &ng911.Location{Lat: &lat, Lon: &lon}})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Dispatch(dispatcher, num, []string{"E1"}, nil); err != nil {
		t.Fatal(err)
	}
	rec, _ := e.Recommend(num)
	var engine Line
	for _, l := range rec.Lines {
		if l.Capability == "engine" {
			engine = l
		}
	}
	// SF needs 3 engines: E1 is on it, E2 is the only other, so one short.
	if len(engine.Covered) != 1 || engine.Covered[0] != "E1" || len(engine.Units) != 1 || engine.Units[0].Unit != "E2" || engine.Short != 1 {
		t.Fatalf("engine line %+v", engine)
	}
	joined := strings.Join(rec.Notes, " | ")
	if !strings.Contains(joined, "1 × engine") || !strings.Contains(joined, "mutual aid") {
		t.Fatalf("notes %s", joined)
	}
}

func TestStaleAVLFallsBackToStation(t *testing.T) {
	e, c, _ := testEngine(t)
	// M2's AVL puts it next to the incident, but 10 minutes ago: the recommendation uses its station.
	e.UnitPosition(RadioActor, "M2", Position{Lat: 40.2634, Lon: -76.8824, Source: "radio", At: c.t.Add(-10 * time.Minute)})
	lat, lon := 40.2633, -76.8823
	num, _ := e.CreateIncident(dispatcher, NewIncident{Type: "BRTH", Location: &ng911.Location{Lat: &lat, Lon: &lon}})
	rec, _ := e.Recommend(num)
	if rec.Units[0] != "M1" {
		t.Fatalf("stale AVL won: %+v", rec.Lines[0].Units)
	}
	e.UnitPosition(RadioActor, "M2", Position{Lat: 40.2634, Lon: -76.8824, Source: "radio", At: c.t.Add(-5 * time.Second)})
	rec, _ = e.Recommend(num)
	if rec.Units[0] != "M2" || rec.Lines[0].Units[0].From != "avl" {
		t.Fatalf("fresh AVL lost: %+v", rec.Lines[0].Units)
	}
}

func TestEmergencyRaiseAndAck(t *testing.T) {
	e, _, _ := testEngine(t)
	unit := e.UnitByRadio(197, "")
	if unit != "P1" {
		t.Fatalf("radio 197 is %q", unit)
	}
	k1, err := e.RaiseEmergency(RadioActor, Emergency{Unit: unit, RadioUnit: "bee07.ad5.0000c5", RadioID: 197, Channel: "tg1"})
	if err != nil {
		t.Fatal(err)
	}
	// A repeat while open is the same emergency (BitVoice retries emergencies three times).
	k2, _ := e.RaiseEmergency(RadioActor, Emergency{Unit: unit, RadioUnit: "bee07.ad5.0000c5", RadioID: 197, Channel: "tg1"})
	if k1 != k2 {
		t.Fatal("a repeated emergency opened a second one")
	}
	if r := e.Execute(dispatcher, "ACK", ""); !r.OK {
		t.Fatal(r.Message)
	}
	e.Read(func(s *State) {
		if em := s.Emergencies[k1]; em.AckedBy != "jlee" || s.Units["P1"].Emergency != "" {
			t.Fatalf("after ack %+v", em)
		}
	})
}

func TestCommandLineErrors(t *testing.T) {
	e, _, _ := testEngine(t)
	for line, want := range map[string]string{
		"D E1":             "select an incident",
		"ER X9":            "No unit X9",
		"NEW NOPE 1 Main":  "choose a call type",
		"FROB":             "Unknown command",
		"ER E1":            "has no incident",
		"CL handled #9999": "no open incident",
	} {
		if r := e.Execute(dispatcher, line, ""); r.OK || !strings.Contains(r.Message, want) {
			t.Errorf("%q: %+v, want %q", line, r, want)
		}
	}
	r := e.Execute(dispatcher, "NEW SUSP P2 100 Market St", "")
	if !r.OK || r.Incident == "" {
		t.Fatalf("NEW: %+v", r)
	}
	e.Read(func(s *State) {
		in := s.Incidents[r.Incident]
		if in.Priority != 2 || in.Location.Address != "100 Market St" {
			t.Fatalf("NEW made %+v", in)
		}
	})
	if r := e.Execute(dispatcher, "D P1 #1", ""); !r.OK {
		t.Fatalf("D by short number: %+v", r)
	}
}

// A TTY or real-time text line grows a character at a time: each growth reaches the call.
func TestCallTextLineGrows(t *testing.T) {
	e, _, _ := testEngine(t)
	at := time.Date(2026, 10, 21, 19, 33, 50, 0, time.UTC)
	for _, s := range []string{"f", "fi", "fire in the kitchen"} {
		if err := e.UpsertCall(BitVoiceActor, "rtt1", CallData{Source: "bitvoice", State: "answered", Callback: "+15405550304",
			Text: []TextLine{{At: at, From: "caller", Via: "rtt", Text: s}}}); err != nil {
			t.Fatal(err)
		}
	}
	var got string
	e.Read(func(s *State) { got = s.Calls["rtt1"].Text[0].Text })
	if got != "fire in the kitchen" {
		t.Fatalf("the line is %q", got)
	}
}
