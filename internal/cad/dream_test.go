package cad

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/acn"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// A Spanish caller's words arrive line by line: each is translated, the details are pulled out, the call is
// not screened as a non-emergency, the incident becomes a critical call, and the summary and review say so.
func TestTranscriptTranslationDetailsCareSummary(t *testing.T) {
	e, c, path := testEngine(t)
	e.Record(SystemActor, EvUserSaved, UserInfo{Username: "jlee", Name: "J Lee", Role: "dispatcher"})
	lat, lon := 40.2633, -76.8823
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(e.UpsertCall(BitVoiceActor, "c1", CallData{Source: "bitvoice", State: "ringing", Callback: "+15405550100",
		Location: &ng911.Location{Lat: &lat, Lon: &lon, Radius: 8, Method: "DBH"}}))
	c.t = c.t.Add(6 * time.Second)
	must(e.UpsertCall(BitVoiceActor, "c1", CallData{Source: "bitvoice", State: "answered", Agent: "jlee", Answer: c.t,
		Transcript: []TranscriptLine{{Speaker: "taker", Text: "911, what is the address of the emergency?"},
			{Speaker: "caller", Text: "Hay un incendio en mi casa, mi esposo no respira"}}}))
	num, err := e.CreateIncident(dispatcher, NewIncident{Type: "chest", Call: "c1"})
	must(err)
	must(e.UpsertCall(BitVoiceActor, "c1", CallData{Source: "bitvoice", Transcript: []TranscriptLine{{Speaker: "taker", Text: "Is he breathing? Is he awake?"}}}))
	var call Call
	var cc *CareCase
	var sum string
	var rv Review
	e.Read(func(s *State) {
		call = *s.Calls["c1"]
		cc = s.Care[num]
		sum = s.Summary(s.Incidents[num])
		rv = s.Review(s.Incidents[num])
	})
	if call.Lang != "es" || !strings.Contains(call.Transcript[1].English, "not breathing") || call.Transcript[1].By != "phrasebook" {
		t.Fatalf("translation: %+v", call.Transcript)
	}
	found := map[string]bool{}
	for _, d := range call.Details {
		found[d.Kind+":"+d.Text] = d.Urgent
	}
	if !found["injury:not breathing"] {
		t.Fatalf("details %+v", call.Details)
	}
	if call.Screen != nil && call.Screen.NonEmergency {
		t.Fatalf("an urgent call was screened out: %+v", call.Screen)
	}
	if len(call.Fixes) != 1 || call.Fixes[0].Method != "DBH" {
		t.Fatalf("fixes %+v", call.Fixes)
	}
	if cc == nil || !strings.Contains(cc.Reason, "not breathing") || len(cc.People) != 1 || cc.People[0] != "jlee" || len(cc.Takers) != 1 {
		t.Fatalf("care %+v", cc)
	}
	if !strings.Contains(sum, "not breathing") || !strings.Contains(sum, "No units assigned") {
		t.Fatalf("summary %q", sum)
	}
	asked := map[string]bool{}
	for _, ch := range rv.Checks {
		asked[ch.Name] = ch.OK
	}
	if !asked["Asked: address"] || !asked["Asked: breathing"] || !asked["Asked: awake"] || asked["Asked: how old"] || rv.Score == 0 || rv.Critical == "" {
		t.Fatalf("review %+v", rv)
	}
	// The supervisor offers a break and arranges a debrief.
	must(e.Care(Actor{User: "sup"}, num, "break", "jlee", ""))
	must(e.Care(Actor{User: "sup"}, num, "debrief", "", "Thursday 0900"))
	if err := e.Care(Actor{User: "sup"}, "26-999999", "break", "", ""); err == nil {
		t.Fatal("care for an incident that is not a critical call")
	}
	// Replaying the ledger rebuilds the same.
	e.Close()
	e2, err := Open(path)
	must(err)
	defer e2.Close()
	e2.Read(func(s *State) {
		cc := s.Care[num]
		if cc == nil || cc.Status != "debrief" || len(cc.Offered) != 1 || s.Calls["c1"].Transcript[1].English != call.Transcript[1].English {
			t.Fatalf("after replay %+v", cc)
		}
	})
}

// A noise complaint on the non-emergency line is screened; a person can say otherwise.
func TestScreening(t *testing.T) {
	e, _, _ := testEngine(t)
	s := DefaultSettings()
	s.NonEmergencyLines = []string{"540-853-2121"}
	if err := e.SaveSettings(SystemActor, s); err != nil {
		t.Fatal(err)
	}
	e.UpsertCall(BitVoiceActor, "n1", CallData{Source: "bitvoice", Line: "+15408532121"})
	e.UpsertCall(BitVoiceActor, "n2", CallData{Source: "bitvoice", Transcript: []TranscriptLine{{Speaker: "caller", Text: "I want to make a noise complaint about my neighbour"}}})
	e.UpsertCall(BitVoiceActor, "n3", CallData{Source: "bitvoice", Transcript: []TranscriptLine{{Speaker: "caller", Text: "It's a noise complaint but he has a gun"}}})
	get := func(id string) *Screen {
		var sc *Screen
		e.Read(func(s *State) { sc = s.Calls[id].Screen })
		return sc
	}
	if sc := get("n1"); sc == nil || !sc.NonEmergency || !strings.Contains(sc.Reason, "non-emergency line") {
		t.Fatalf("n1 %+v", sc)
	}
	if sc := get("n2"); sc == nil || !sc.NonEmergency {
		t.Fatalf("n2 %+v", sc)
	}
	if sc := get("n3"); sc != nil && sc.NonEmergency {
		t.Fatalf("n3 with a gun screened out: %+v", sc)
	}
	if err := e.ScreenCall(dispatcher, "n2", true); err != nil {
		t.Fatal(err)
	}
	e.UpsertCall(BitVoiceActor, "n2", CallData{Source: "bitvoice", Transcript: []TranscriptLine{{Speaker: "caller", Text: "parking"}}})
	if sc := get("n2"); sc.NonEmergency || sc.By != "jlee" {
		t.Fatalf("the person's decision was overridden: %+v", sc)
	}
}

// Crash data from a vehicle is assessed and its details shown.
func TestCrashData(t *testing.T) {
	e, _, _ := testEngine(t)
	lat, lon := 37.3, -79.9
	f := false
	e.UpsertCall(Actor{User: "acn"}, "acn-1", CallData{Source: "acn", Crash: &acn.Crash{Provider: "OnStar", DeltaVKmh: 56, Direction: "front", Rollover: true, Occupants: 2,
		Belted: &f, Fuel: "electric", Callback: "+15405550111", Vehicle: acn.Vehicle{Year: 2024, Make: "Tesla", Model: "Model Y", Color: "white", Plate: "abc1234"}},
		Location: &ng911.Location{Lat: &lat, Lon: &lon, Radius: 10}})
	e.Read(func(s *State) {
		c := s.Calls["acn-1"]
		if c.CrashRisk == nil || c.CrashRisk.Risk != "high" || c.Callback != "+15405550111" {
			t.Fatalf("%+v", c.CrashRisk)
		}
		ok := false
		for _, d := range c.Details {
			ok = ok || d.Kind == "vehicle" && strings.Contains(d.Text, "Tesla")
		}
		if !ok {
			t.Fatalf("details %+v", c.Details)
		}
	})
}

// Many calls in a minute, or one number calling again and again, raise the warning; a supervisor acks it.
func TestTDoS(t *testing.T) {
	e, c, _ := testEngine(t)
	for i := 0; i < 6; i++ {
		c.t = c.t.Add(30 * time.Second)
		e.UpsertCall(BitVoiceActor, fmt.Sprintf("r%d", i), CallData{Source: "bitvoice", Callback: "+15405550199"})
	}
	var td TDoS
	e.Read(func(s *State) { td = s.TDoS })
	if !td.Active || len(td.Repeats) != 1 {
		t.Fatalf("repeat caller: %+v", td)
	}
	if err := e.AckTDoS(Actor{User: "sup"}); err != nil {
		t.Fatal(err)
	}
	// Quiet for 10 minutes, the next call clears it.
	c.t = c.t.Add(11 * time.Minute)
	e.UpsertCall(BitVoiceActor, "quiet", CallData{Source: "bitvoice", Callback: "+15405550100"})
	e.Read(func(s *State) { td = s.TDoS })
	if td.Active {
		t.Fatalf("still active: %+v", td)
	}
	for i := 0; i < 14; i++ {
		c.t = c.t.Add(3 * time.Second)
		e.UpsertCall(BitVoiceActor, fmt.Sprintf("f%d", i), CallData{Source: "bitvoice", Callback: fmt.Sprintf("+1540555%04d", i)})
	}
	e.Read(func(s *State) { td = s.TDoS })
	if !td.Active || td.PerMin < 12 {
		t.Fatalf("flood: %+v", td)
	}
}

// Radio words go on the unit's incident; drones report and are on the timeline; breaks are kept.
func TestRadioDroneBreak(t *testing.T) {
	e, _, _ := testEngine(t)
	num, _ := e.CreateIncident(dispatcher, NewIncident{Type: "chest"})
	e.Dispatch(dispatcher, num, []string{"M1"}, nil)
	if err := e.RadioText(RadioActor, RadioLine{Unit: "M1", Text: "Medic 1 on scene, patient conscious"}); err != nil {
		t.Fatal(err)
	}
	lat, lon := 40.27, -76.88
	if err := e.UpdateDrone(Actor{User: "drone"}, Drone{ID: "D1", Name: "Skydio 1", Status: "onscene", Incident: num, Lat: &lat, Lon: &lon, VideoURL: "https://example.test/live"}); err != nil {
		t.Fatal(err)
	}
	e.DroneRequested(dispatcher, num, DroneRequest{OK: true, Detail: "accepted"})
	e.SetBreak(dispatcher, true)
	e.SetBreak(dispatcher, true) // already on: nothing written
	e.SetBreak(dispatcher, false)
	e.Read(func(s *State) {
		in := s.Incidents[num]
		var kinds []string
		for _, t := range in.Timeline {
			kinds = append(kinds, t.Kind)
		}
		k := strings.Join(kinds, ",")
		if !strings.Contains(k, "radio") || strings.Count(k, "drone") != 2 || len(s.Radio) != 1 || s.Radio[0].Incident != num {
			t.Fatalf("timeline %s radio %+v", k, s.Radio)
		}
		if s.Drones["D1"].VideoURL == "" || !strings.Contains(s.Summary(in), "Drone Skydio 1 over the scene") {
			t.Fatalf("drone %+v %q", s.Drones["D1"], s.Summary(in))
		}
		if b := s.Breaks["jlee"]; b == nil || b.On || b.Count != 1 || b.LastEnd.IsZero() {
			t.Fatalf("break %+v", b)
		}
	})
}
