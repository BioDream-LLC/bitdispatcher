package cad

import (
	"testing"

	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

func TestMoveUp(t *testing.T) {
	e, _, path := testEngine(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	// A third station with two engines, so it can spare one; ST2's only engine stays to cover ST2.
	must(e.SaveStation(SystemActor, Station{ID: "ST3", Name: "Station 3", Lat: 40.2800, Lon: -76.9000}))
	for _, id := range []string{"E3", "E4"} {
		must(e.SaveUnit(SystemActor, UnitDef{ID: id, Type: "engine", Station: "ST3"}))
		must(e.SetStatus(SystemActor, id, Available, "", ""))
	}
	if c := e.Coverage(); len(c.Gaps) != 0 {
		t.Fatalf("gaps with everyone in quarters: %+v", c.Gaps)
	}
	lat, lon := 40.27, -76.88
	num, err := e.CreateIncident(dispatcher, NewIncident{Type: "SF", Location: &ng911.Location{Lat: &lat, Lon: &lon}})
	must(err)
	must(e.Dispatch(dispatcher, num, []string{"E1"}, nil))
	c := e.Coverage()
	if len(c.Gaps) != 1 || c.Gaps[0].Station != "ST1" || c.Gaps[0].Type != "engine" {
		t.Fatalf("gaps %+v", c.Gaps)
	}
	g := c.Gaps[0]
	// E2 is ST2's only engine; E3 and E4 are at ST3, which keeps one.
	if g.Move == nil || (g.Move.Unit != "E3" && g.Move.Unit != "E4") || g.Move.From != "ST3" || g.Move.Left != 1 {
		t.Fatalf("move %+v", g.Move)
	}
	// Emptying ST2 to fill ST1 is offered only after the moves that empty nothing, and says so.
	for _, m := range g.Others {
		if m.Unit == "E2" && !m.Opens {
			t.Fatal("moving E2 does not say it empties ST2")
		}
	}
	if g.Others[len(g.Others)-1].Unit != "E2" {
		t.Fatalf("E2 not last: %+v", g.Others)
	}
	r := e.Execute(dispatcher, "MU "+g.Move.Unit+" ST1", "")
	if !r.OK {
		t.Fatal(r.Message)
	}
	if c := e.Coverage(); len(c.Gaps) != 0 {
		t.Fatalf("still a gap after the move-up: %+v", c.Gaps)
	}
	// The covering unit is recommended from ST1 now.
	num2, _ := e.CreateIncident(dispatcher, NewIncident{Type: "SF", Location: &ng911.Location{Lat: &lat, Lon: &lon}})
	rec, _ := e.Recommend(num2)
	firstEngine := ""
	for _, id := range rec.Units {
		if id[0] == 'E' && firstEngine == "" {
			firstEngine = id
		}
	}
	if firstEngine != g.Move.Unit {
		t.Fatalf("covering unit not the first engine: %+v", rec.Units)
	}
	// E1 back: the cover can go home.
	must(e.SetStatus(dispatcher, "E1", Available, "", ""))
	c = e.Coverage()
	if len(c.Returns) != 1 || c.Returns[0].Unit != g.Move.Unit {
		t.Fatalf("returns %+v", c.Returns)
	}
	if r := e.Execute(dispatcher, "RET "+g.Move.Unit, ""); !r.OK {
		t.Fatal(r.Message)
	}
	// Replayed from the ledger, the cover is the same.
	e.Execute(dispatcher, "MU E4 ST2", "")
	e.Close()
	e2, err := Open(path)
	must(err)
	defer e2.Close()
	e2.Read(func(s *State) {
		if s.Units["E4"].Covering != "ST2" || s.Units[g.Move.Unit].Covering != "" && g.Move.Unit != "E4" {
			t.Fatalf("replay: E4 covering %q", s.Units["E4"].Covering)
		}
	})
	if r := e2.Execute(dispatcher, "MU E4 ST9", ""); r.OK {
		t.Fatal("moved to a station that does not exist")
	}
}
