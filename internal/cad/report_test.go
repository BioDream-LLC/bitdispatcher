package cad

import (
	"slices"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

func TestReport(t *testing.T) {
	e, c, _ := testEngine(t)
	from := c.t
	step := func(d time.Duration) { c.t = c.t.Add(d) }
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	// One incident left over from before the period.
	lat, lon := 40.27, -76.88
	old, err := e.CreateIncident(dispatcher, NewIncident{Type: "SF", Location: &ng911.Location{Lat: &lat, Lon: &lon, Address: "1 Old St"}})
	must(err)
	step(time.Hour)
	from = c.t
	// A P1 fire: dispatched after 60 s, en route after 30 s more, on scene 5 minutes after creation.
	step(time.Minute)
	sf, _ := e.CreateIncident(dispatcher, NewIncident{Type: "SF", Priority: 1, Location: &ng911.Location{Lat: &lat, Lon: &lon, Address: "2 Fire Rd"}})
	step(60 * time.Second)
	must(e.Dispatch(dispatcher, sf, []string{"E1", "L1"}, []string{"E1", "L1"}))
	step(30 * time.Second)
	must(e.SetStatus(dispatcher, "E1", Enroute, "", ""))
	step(210 * time.Second)
	must(e.SetStatus(dispatcher, "E1", OnScene, "", ""))
	// L1 never goes en route: past its 2-minute dispatched timer, once.
	step(20 * time.Minute)
	must(e.Execute(dispatcher, "MU E2 ST1", "").err())
	must(e.CloseIncident(dispatcher, sf, "Handled", ""))
	step(time.Minute)
	to := c.t
	step(time.Hour)
	// After the period: not counted.
	e.CreateIncident(dispatcher, NewIncident{Type: "SF", Location: &ng911.Location{Lat: &lat, Lon: &lon}})

	r, err := e.Report(from, to)
	must(err)
	get := func(cs []Count, k string) Count {
		for _, x := range cs {
			if x.Key == k {
				return x
			}
		}
		return Count{}
	}
	if n := get(r.Totals, "created").N; n != 1 {
		t.Fatalf("created %d", n)
	}
	if get(r.Totals, "closed").N != 1 || get(r.ByDisposition, "Handled").N != 1 {
		t.Fatalf("closed %+v", r.Totals)
	}
	if len(r.CarriedIn) != 1 || r.CarriedIn[0].Number != old || len(r.CarriedOut) != 1 || r.CarriedOut[0].Number != old {
		t.Fatalf("carried in %+v out %+v", r.CarriedIn, r.CarriedOut)
	}
	if len(r.Significant) != 1 || r.Significant[0].Number != sf {
		t.Fatalf("significant %+v", r.Significant)
	}
	var all ResponseRow
	for _, row := range r.Response {
		if row.Key == "all" {
			all = row
		}
	}
	if all.Incidents != 1 || all.ToDispatch.Median != 60 || all.ToEnroute.Median != 30 || all.ToOnScene.Median != 300 {
		t.Fatalf("response %+v", all)
	}
	// Every total can be opened to its events.
	if ev := get(r.Totals, "created").Events; len(ev) != 1 {
		t.Fatalf("drill-down %v", ev)
	}
	timeouts := 0
	for _, x := range r.Timeouts {
		if x.Unit == "L1" && x.Status == Dispatched {
			timeouts++
		}
	}
	if timeouts != 1 {
		t.Fatalf("timeouts %+v", r.Timeouts)
	}
	var e1 *UnitDemand
	for i := range r.Units {
		if r.Units[i].Unit == "E1" {
			e1 = &r.Units[i]
		}
	}
	if e1 == nil || e1.ByPhase[Enroute] != 210 || e1.ByPhase[Dispatched] != 30 || e1.Incidents != 1 {
		t.Fatalf("E1 demand %+v", e1)
	}
	var jlee *PositionActivity
	for i := range r.Positions {
		if r.Positions[i].Actor == "jlee" {
			jlee = &r.Positions[i]
		}
	}
	if jlee == nil || jlee.Created != 1 || jlee.Dispatched != 1 || jlee.Closed != 1 || jlee.MoveUps != 1 || jlee.Touched != 1 || !slices.Contains(jlee.Consoles, "D1") {
		t.Fatalf("activity %+v", jlee)
	}
	// Certify, then the same report says so; a stale hash is refused.
	if err := e.CertifyReport(dispatcher, from, to, "nope", ""); err == nil {
		t.Fatal("certified with the wrong hash")
	}
	must(e.CertifyReport(dispatcher, from, to, r.Hash, "Quiet shift."))
	r2, _ := e.Report(from, to)
	if r2.Hash != r.Hash || len(r2.Certifications) != 1 || !r2.Certifications[0].Current || r2.Certifications[0].Comment != "Quiet shift." {
		t.Fatalf("certifications %+v (hash %s vs %s)", r2.Certifications, r2.Hash, r.Hash)
	}
	if err := e.CertifyReport(dispatcher, from, c.t.Add(time.Hour), r.Hash, ""); err == nil {
		t.Fatal("certified a period that has not ended")
	}
}

func (r CommandResult) err() error {
	if r.OK {
		return nil
	}
	return problem("command", "%s", r.Message)
}
