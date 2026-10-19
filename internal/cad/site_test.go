package cad

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

func siteEngine(t *testing.T, name, prefix, other string, c *clock) *Engine {
	t.Helper()
	dir := t.TempDir()
	e, err := Open(filepath.Join(dir, "ledger.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	e.now = c.now
	e.SetSite(name, prefix)
	if err := e.OpenPeers(filepath.Join(dir, "peers"), []string{other}); err != nil {
		t.Fatal(err)
	}
	return e
}

func sync2(t *testing.T, a, b *Engine) {
	t.Helper()
	for _, p := range [][2]*Engine{{a, b}, {b, a}} {
		from, to := p[0], p[1]
		evs, err := from.LocalEvents(to.PeerSeq(from.Site()), 100000)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := to.AddRemote(from.Site(), evs); err != nil {
			t.Fatal(err)
		}
	}
}

func snapshot(e *Engine) string {
	var b []byte
	e.Read(func(s *State) {
		b, _ = json.Marshal(struct {
			I map[string]*Incident
			U map[string]*Unit
			S map[string]*Station
		}{s.Incidents, s.Units, s.Stations})
	})
	return string(b)
}

func TestTwoSites(t *testing.T) {
	c := &clock{time.Date(2026, 10, 18, 22, 0, 0, 0, time.UTC)}
	a := siteEngine(t, "north", "N", "south", c)
	b := siteEngine(t, "south", "S", "north", c)
	step := func() { c.t = c.t.Add(time.Second) }
	// Configuration made at one site reaches the other.
	a.SaveStation(SystemActor, Station{ID: "ST1", Name: "Station 1", Lat: 37.27, Lon: -79.94})
	step()
	a.SaveUnit(SystemActor, UnitDef{ID: "E1", Type: "engine", Station: "ST1"})
	step()
	a.SetStatus(SystemActor, "E1", Available, "", "")
	step()
	sync2(t, a, b)
	if snapshot(a) != snapshot(b) {
		t.Fatal("configuration differs after sync")
	}
	// Cut off: each site takes a call and sends E1, north a little before south.
	lat, lon := 37.27, -79.94
	na, _ := a.CreateIncident(dispatcher, NewIncident{Type: "SF", Location: &ng911.Location{Lat: &lat, Lon: &lon}})
	step()
	if err := a.Dispatch(dispatcher, na, []string{"E1"}, nil); err != nil {
		t.Fatal(err)
	}
	step()
	sb, _ := b.CreateIncident(dispatcher, NewIncident{Type: "MVA", Location: &ng911.Location{Lat: &lat, Lon: &lon}})
	step()
	if err := b.Dispatch(dispatcher, sb, []string{"E1"}, nil); err != nil {
		t.Fatal(err)
	}
	step()
	if na == sb || na[0] == sb[0] && na[:1] != "N" {
		t.Fatalf("numbers %s %s", na, sb)
	}
	if na != "N26-000001" || sb != "S26-000001" {
		t.Fatalf("numbers %s %s", na, sb)
	}
	// The link returns: both sites end the same, with the later dispatch (south's) holding E1.
	sync2(t, a, b)
	sa, sbs := snapshot(a), snapshot(b)
	if sa != sbs {
		t.Fatalf("sites differ after the link returned:\n%s\n%s", sa, sbs)
	}
	a.Read(func(s *State) {
		if s.Units["E1"].Incident != sb {
			t.Fatalf("E1 on %s, want %s", s.Units["E1"].Incident, sb)
		}
		in := s.Incidents[na]
		if len(in.Units) != 1 || in.Units[0].Cleared.IsZero() {
			t.Fatalf("north's assignment not cleared: %+v", in.Units)
		}
	})
	// Restarting rebuilds the same state from the ledger and the copy.
	dirA := filepath.Dir(a.LedgerPath())
	a.Close()
	a2, err := Open(filepath.Join(dirA, "ledger.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer a2.Close()
	a2.SetSite("north", "N")
	if err := a2.OpenPeers(filepath.Join(dirA, "peers"), []string{"south"}); err != nil {
		t.Fatal(err)
	}
	if snapshot(a2) != sbs {
		t.Fatal("restart gives a different state")
	}
	// A tampered event is refused.
	evs, _ := b.LocalEvents(0, 10)
	evs[0].Seq = a2.PeerSeq("south") + 1
	evs[0].Actor = "mallory"
	if _, err := a2.AddRemote("south", evs[:1]); err == nil {
		t.Fatal("tampered event accepted")
	}
}
