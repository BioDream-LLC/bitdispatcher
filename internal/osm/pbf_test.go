package osm

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	nodes := []Node{
		{ID: 1, Lat: 372710000, Lon: -799414000},
		{ID: 2, Lat: 372720000, Lon: -799400000, Tags: map[string]string{"amenity": "fire_station", "name": "Station 1"}},
		{ID: 5, Lat: -10, Lon: 10},
	}
	ways := []Way{{ID: 10, Refs: []int64{1, 2, 5}, Tags: map[string]string{"highway": "residential", "name": "Kirk Avenue Southwest"}}}
	rels := []Relation{{ID: 100, Members: []Member{{Type: 'w', ID: 10, Role: "outer"}, {Type: 'n', ID: 2, Role: ""}}, Tags: map[string]string{"type": "multipolygon"}}}
	var buf bytes.Buffer
	if err := Write(&buf, nodes, ways, rels); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "x.osm.pbf")
	os.WriteFile(p, buf.Bytes(), 0o600)
	var mu sync.Mutex
	var got Block
	err := Scan(p, Want{Nodes: true, Ways: true, Relations: true}, 2, nil, func(_ int, b *Block) error {
		mu.Lock()
		defer mu.Unlock()
		got.Nodes = append(got.Nodes, b.Nodes...)
		got.Ways = append(got.Ways, b.Ways...)
		got.Relations = append(got.Relations, b.Relations...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(got.Nodes, func(i, j int) bool { return got.Nodes[i].ID < got.Nodes[j].ID })
	if len(got.Nodes) != 3 || got.Nodes[0].Lat != 372710000 || got.Nodes[0].Lon != -799414000 || got.Nodes[2].Lat != -10 {
		t.Fatalf("nodes: %+v", got.Nodes)
	}
	if got.Nodes[1].Tags["name"] != "Station 1" || got.Nodes[0].Tags != nil {
		t.Fatalf("node tags: %+v", got.Nodes)
	}
	if len(got.Ways) != 1 || len(got.Ways[0].Refs) != 3 || got.Ways[0].Refs[2] != 5 || got.Ways[0].Tags["highway"] != "residential" {
		t.Fatalf("ways: %+v", got.Ways)
	}
	r := got.Relations
	if len(r) != 1 || len(r[0].Members) != 2 || r[0].Members[0] != (Member{'w', 10, "outer"}) || r[0].Members[1].Type != 'n' {
		t.Fatalf("relations: %+v", r)
	}
	// Asking for nodes only builds no ways.
	n := 0
	Scan(p, Want{Nodes: true}, 1, nil, func(_ int, b *Block) error { n += len(b.Ways) + len(b.Relations); return nil })
	if n != 0 {
		t.Fatal("ways or relations decoded when not wanted")
	}
}

func TestNotPBF(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x")
	os.WriteFile(p, []byte("this is not a pbf file at all"), 0o600)
	if err := Scan(p, Want{Nodes: true}, 1, nil, func(int, *Block) error { return nil }); err == nil {
		t.Fatal("garbage accepted")
	}
	os.WriteFile(p, nil, 0o600)
	if err := Scan(p, Want{Nodes: true}, 1, nil, func(int, *Block) error { return nil }); err == nil {
		t.Fatal("empty file accepted")
	}
}

// FuzzDecodeBlock: malformed blocks fail, never panic.
func FuzzDecodeBlock(f *testing.F) {
	f.Add(block([]Node{{ID: 1, Lat: 1, Lon: 1, Tags: map[string]string{"a": "b"}}}, []Way{{ID: 2, Refs: []int64{1}}}, nil))
	f.Fuzz(func(t *testing.T, b []byte) {
		decodeBlock(b, Want{Nodes: true, Ways: true, Relations: true})
	})
}

func TestChangeApply(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.osm.pbf")
	var buf bytes.Buffer
	nodes := []Node{{ID: 1, Lat: 10, Lon: 10}, {ID: 2, Lat: 20, Lon: 20}, {ID: 3, Lat: 30, Lon: 30}}
	ways := []Way{{ID: 10, Refs: []int64{1, 2}, Tags: map[string]string{"highway": "residential", "name": "Old Name"}}}
	f, _ := os.Create(src)
	w, _ := NewWriter(f, Header{Sequence: 100, BaseURL: "https://download.geofabrik.de/x-updates", Timestamp: 1700000000})
	for _, n := range nodes {
		w.Node(n)
	}
	for _, x := range ways {
		w.Way(x)
	}
	w.Close()
	f.Close()
	_ = buf
	h, err := ReadHeader(src)
	if err != nil || h.Sequence != 100 || h.BaseURL != "https://download.geofabrik.de/x-updates" || h.Timestamp != 1700000000 {
		t.Fatalf("header %+v %v", h, err)
	}
	osc := `<?xml version="1.0"?><osmChange version="0.6">
<modify><way id="10"><nd ref="1"/><nd ref="2"/><nd ref="4"/><tag k="highway" v="residential"/><tag k="name" v="New Name"/></way></modify>
<create><node id="4" lat="0.000004" lon="0.000004"/></create>
<delete><node id="3"/></delete></osmChange>`
	c := NewChange()
	if err := c.ReadChange(strings.NewReader(osc), false); err != nil {
		t.Fatal(err)
	}
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write([]byte(`<osmChange><modify><node id="2" lat="0.0000021" lon="0.0000021"><tag k="amenity" v="fire_station"/></node></modify></osmChange>`))
	zw.Close()
	if err := c.ReadChange(&gz, true); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "b.osm.pbf")
	if err := Apply(src, dst, c, Header{Sequence: 102, BaseURL: h.BaseURL}, 2); err != nil {
		t.Fatal(err)
	}
	got := map[int64]Node{}
	var way Way
	Scan(dst, Want{Nodes: true, Ways: true}, 1, nil, func(_ int, b *Block) error {
		for _, n := range b.Nodes {
			got[n.ID] = n
		}
		if len(b.Ways) > 0 {
			way = b.Ways[0]
		}
		return nil
	})
	if _, ok := got[3]; ok || got[4].Lat != 40 || got[2].Lat != 21 || got[2].Tags["amenity"] != "fire_station" || len(got) != 3 {
		t.Fatalf("nodes %+v", got)
	}
	if way.Tags["name"] != "New Name" || len(way.Refs) != 3 {
		t.Fatalf("way %+v", way)
	}
	if h2, _ := ReadHeader(dst); h2.Sequence != 102 {
		t.Fatalf("new header %+v", h2)
	}
}
