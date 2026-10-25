package cad

import (
	"strings"
	"testing"
	"time"
)

// Road closures: a stretch or a circle, in force between its times, lifted for the record, written on the
// incident that closed it, and rebuilt the same from the ledger.
func TestClosures(t *testing.T) {
	e, c, path := testEngine(t)
	line := [][2]float64{{-76.885, 40.27}, {-76.875, 40.27}}
	if _, err := e.SaveClosure(dispatcher, RoadClosure{Line: line}); err == nil {
		t.Fatal("a closure with no reason accepted")
	}
	if _, err := e.SaveClosure(dispatcher, RoadClosure{Reason: "x", Line: line, Center: &[2]float64{40.27, -76.88}, Radius: 50}); err == nil {
		t.Fatal("a closure that is both a line and a circle accepted")
	}
	until := c.t.Add(2 * time.Hour)
	id, err := e.SaveClosure(dispatcher, RoadClosure{Reason: "Crash, all lanes", Line: line, Until: &until, Detour: "Use 2nd Street"})
	must(t, err)
	id2, err := e.SaveClosure(dispatcher, RoadClosure{Reason: "Flooded underpass", Center: &[2]float64{40.28, -76.87}, Radius: 80})
	must(t, err)
	var n int
	e.Read(func(s *State) { n = len(s.ClosuresInForce(c.t)) })
	if n != 2 {
		t.Fatalf("%d closures in force", n)
	}
	e.Read(func(s *State) { n = len(s.ClosuresInForce(c.t.Add(3 * time.Hour))) })
	if n != 1 {
		t.Fatalf("after its end, %d closures in force; want the open-ended one", n)
	}
	must(t, e.LiftClosure(dispatcher, id2))
	if err := e.LiftClosure(dispatcher, id2); err == nil {
		t.Fatal("a closure lifted twice")
	}
	e2, err := Open(path)
	must(t, err)
	defer e2.Close()
	e2.Read(func(s *State) {
		if s.Closures[id] == nil || s.Closures[id2] == nil || s.Closures[id2].Lifted == nil || s.Closures[id].Detour != "Use 2nd Street" {
			t.Fatalf("rebuilt closures %+v", s.Closures)
		}
	})
}

// Cameras: off until switched on; a look needs a purpose and is recorded; the ones looking at a place come
// before nearer ones that look away.
func TestCameras(t *testing.T) {
	e, _, _ := testEngine(t)
	north, south := 0.0, 180.0
	at := func(lat, lon float64, h *float64, name string) string {
		id, err := e.SaveCamera(SystemActor, Camera{Name: name, Lat: lat, Lon: lon, Heading: h, Still: "https://cams.example.gov/" + name + ".jpg"})
		must(t, err)
		return id
	}
	if _, err := e.SaveCamera(SystemActor, Camera{Name: "x", Lat: 40.27, Lon: -76.88}); err == nil {
		t.Fatal("a camera with no address accepted")
	}
	if _, err := e.SaveCamera(SystemActor, Camera{Name: "x", Lat: 40.27, Lon: -76.88, Live: "javascript:alert(1)"}); err == nil {
		t.Fatal("a script address accepted")
	}
	away := at(40.2701, -76.8800, &south, "away")     // 10 m north of the place, looking south: at it
	toward := at(40.2690, -76.8800, &north, "toward") // 110 m south, looking north: at it
	behind := at(40.2690, -76.8801, &south, "behind") // 110 m south, looking further south: not
	at(40.40, -76.88, nil, "far")                     // 14 km away: not offered
	place := [2]float64{40.2700, -76.8800}
	if _, err := e.LookAtCamera(dispatcher, toward, "", "check the crash"); err == nil {
		t.Fatal("a camera opened while cameras are off")
	}
	st := DefaultSettings()
	st.Next.Cameras.On = true
	must(t, e.SaveSettings(SystemActor, st))
	var near []CameraNear
	e.Read(func(s *State) { near = s.CamerasNear(place[0], place[1]) })
	if len(near) != 3 || near[0].ID != away || near[1].ID != toward || near[2].ID != behind || *near[2].Facing {
		for _, n := range near {
			t.Logf("%s %.0f m facing %v", n.Name, n.Metres, *n.Facing)
		}
		t.Fatal("cameras not ranked by whether they look at the place")
	}
	if _, err := e.LookAtCamera(dispatcher, toward, "", " "); err == nil {
		t.Fatal("a camera opened with no purpose")
	}
	_, err := e.LookAtCamera(dispatcher, toward, "", "check the crash")
	must(t, err)
	e.Read(func(s *State) {
		if len(s.CameraLooks) != 1 || s.CameraLooks[0].User != "jlee" || s.CameraLooks[0].Purpose != "check the crash" {
			t.Fatalf("looks %+v", s.CameraLooks)
		}
	})
}

// The non-emergency intake asks its questions, hands an emergency to a person at once (but not "no one is
// hurt"), and asks a required question again before moving on.
func TestIntake(t *testing.T) {
	sc := DefaultSettings().IntakeOrDefault()
	var ss IntakeSession
	first := sc.Start(&ss)
	if !strings.Contains(first.Say, "emergency") || first.Ask != "danger" {
		t.Fatalf("start %+v", first)
	}
	steps := []struct{ say, wantAsk string }{
		{"No, nobody is hurt", "what"},
		{"Someone keeps parking in front of my driveway", "where"},
		{"1200 Main Street", "when"},
		{"this morning", "name"},
		{"Pat Doe", "phone"},
		{"I don't know", "phone"}, // asked again
	}
	for _, s := range steps {
		tr := sc.Hear(&ss, s.say)
		if tr.Transfer || tr.Ask != s.wantAsk {
			t.Fatalf("after %q: %+v", s.say, tr)
		}
	}
	if tr := sc.Hear(&ss, "540 555 0100"); !tr.Done || tr.Transfer {
		t.Fatalf("end %+v", tr)
	}
	if ss.Answers["where"] != "1200 Main Street" || ss.Answers["danger"] != "no" || ss.Answers["phone"] != "540 555 0100" {
		t.Fatalf("answers %+v", ss.Answers)
	}
	for _, said := range []string{"yes", "my neighbour is bleeding", "he has a gun", "there is smoke in the hallway"} {
		var s2 IntakeSession
		sc.Start(&s2)
		if said != "yes" {
			sc.Hear(&s2, "no")
		}
		if tr := sc.Hear(&s2, said); !tr.Transfer || !tr.Done {
			t.Errorf("%q was not handed to a person: %+v", said, tr)
		}
	}
	if err := CheckIntake(IntakeScript{Questions: []IntakeQuestion{{Key: "a", Ask: "x", Kind: "text", Danger: true}}}); err == nil {
		t.Error("a danger question that is not yes/no accepted")
	}
}
