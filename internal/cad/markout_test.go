package cad

import (
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

func TestMarkOutAndStatusCheck(t *testing.T) {
	e, c, path := testEngine(t)
	// The kinds are the agency's; an unknown one says which there are.
	if r := e.Execute(dispatcher, "MO P1 NAP", ""); r.OK || !strings.Contains(r.Message, "MEAL") {
		t.Fatalf("unknown kind: %+v", r)
	}
	// A police-only kind is refused for an engine.
	if r := e.Execute(dispatcher, "MO E1 TS", ""); r.OK || !strings.Contains(r.Message, "police") {
		t.Fatalf("discipline: %+v", r)
	}
	if err := e.SetStatus(dispatcher, "P1", MarkedOut, "", ""); err == nil {
		t.Fatal("a plain status change made a mark-out with no kind")
	}
	if r := e.Execute(dispatcher, "MO P1 meal 315 Church Ave / with 2B12", ""); !r.OK || !strings.Contains(r.Message, "Meal break") {
		t.Fatalf("mark out: %+v", r)
	}
	var u Unit
	e.Read(func(s *State) { u = *s.Units["P1"] })
	if u.Status != MarkedOut || u.MarkOut == nil || u.MarkOut.Code != "MEAL" || u.MarkOut.Note != "with 2B12" || u.MarkOut.Location.Address != "315 Church Ave" {
		t.Fatalf("unit %+v %+v", u, u.MarkOut)
	}
	var set Settings
	e.Read(func(s *State) { set = s.Settings })
	due, ok := u.CheckDue(set)
	if !ok || !due.Equal(u.StatusAt.Add(30*time.Minute)) {
		t.Fatalf("due %v %v", due, ok)
	}
	// Twenty minutes in, a status check restarts the timer.
	c.t = c.t.Add(20 * time.Minute)
	if r := e.Execute(dispatcher, "SC P1 code 4", ""); !r.OK {
		t.Fatalf("check: %+v", r)
	}
	e.Read(func(s *State) { u = *s.Units["P1"] })
	if due2, _ := u.CheckDue(set); !due2.Equal(c.t.Add(30*time.Minute)) || u.CheckedBy != "jlee" {
		t.Fatalf("after the check %v %+v", due2, u)
	}
	// A shooting with every car but this one busy: the marked-out car is offered, not sent.
	lat, lon := 40.27, -76.88
	in, _ := e.CreateIncident(dispatcher, NewIncident{Type: "DOM", Location: &ng911.Location{Lat: &lat, Lon: &lon, Address: "1 Main"}})
	rec, err := e.Recommend(in)
	if err != nil || len(rec.Units) != 0 || len(rec.Lines) == 0 || len(rec.Lines[0].Next) != 1 || rec.Lines[0].Next[0].Unit != "P1" ||
		!strings.Contains(strings.Join(rec.Notes, " "), "P1 is marked out and can be pulled") {
		t.Fatalf("recommend %+v %v", rec, err)
	}
	// Court cannot be interrupted.
	e.Execute(dispatcher, "MO P1 COURT", "")
	rec, _ = e.Recommend(in)
	if len(rec.Lines[0].Next) != 0 {
		t.Fatalf("court offered %+v", rec.Lines[0].Next)
	}
	e.Read(func(s *State) { u = *s.Units["P1"] })
	if u.MarkOut.Code != "COURT" || u.MarkOut.Location != nil {
		t.Fatalf("re-marked %+v", u.MarkOut)
	}
	// Available clears the mark-out.
	e.Execute(dispatcher, "AV P1", "")
	e.Read(func(s *State) { u = *s.Units["P1"] })
	if u.MarkOut != nil {
		t.Fatal("mark-out kept after available")
	}
	// The report counts the time marked out and the timer that ran out.
	c.t = c.t.Add(3 * time.Hour)
	e.Execute(dispatcher, "MO P1 FUEL", "")
	c.t = c.t.Add(20 * time.Minute)
	e.Execute(dispatcher, "AV P1", "")
	e.Close()
	e2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()
	e2.Read(func(s *State) { u = *s.Units["P1"] })
	if u.Status != Available || u.Checked.IsZero() {
		t.Fatalf("replayed %+v", u)
	}
	r, err := e2.Report(c.t.Add(-6*time.Hour), c.t.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	var mo float64
	for _, d := range r.Units {
		if d.Unit == "P1" {
			mo = d.MarkedOut
		}
	}
	fuel := 0
	for _, x := range r.Timeouts {
		if x.Unit == "P1" && x.Status == MarkedOut {
			fuel++
		}
	}
	if mo != 2400 || fuel != 1 { // meal 20 min, court none, fuel 20 min (its 15 ran out)
		t.Fatalf("report: marked out %.0f s, %d timeouts %+v", mo, fuel, r.Timeouts)
	}
}
