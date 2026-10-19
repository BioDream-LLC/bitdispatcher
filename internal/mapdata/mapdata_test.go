package mapdata

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/osm"
)

// town is a small synthetic map: a 3×3 street grid around 37.27, -79.94 with one one-way street, an
// address, a fire station, a lake drawn as a multipolygon of two ways, a county line, and a fire station
// far outside the area that must not be recorded.
func town(t *testing.T) string {
	t.Helper()
	lats := []float64{37.26, 37.27, 37.28}
	lons := []float64{-79.95, -79.94, -79.93}
	id := func(i, j int) int64 { return int64(1 + i*3 + j) }
	var nodes []osm.Node
	for i, la := range lats {
		for j, lo := range lons {
			nodes = append(nodes, osm.Node{ID: id(i, j), Lat: int32(la * 1e7), Lon: int32(lo * 1e7)})
		}
	}
	tag := func(kv ...string) map[string]string {
		m := map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	nodes = append(nodes,
		osm.Node{ID: 50, Lat: 372701000, Lon: -799450000, Tags: tag("addr:housenumber", "100", "addr:street", "Main Street", "addr:city", "Testville", "addr:postcode", "24011")},
		osm.Node{ID: 51, Lat: 372702000, Lon: -799398000, Tags: tag("amenity", "fire_station", "name", "Test Fire Station 1")},
		osm.Node{ID: 52, Lat: 385000000, Lon: -799400000, Tags: tag("amenity", "fire_station", "name", "Far Away Station")},
		osm.Node{ID: 53, Lat: 372650000, Lon: -799350000, Tags: tag("emergency", "fire_hydrant")},
		osm.Node{ID: 54, Lat: 372700000, Lon: -799400000, Tags: tag("place", "town", "name", "Testville")},
		// The lake: four corners, joined by two ways.
		osm.Node{ID: 60, Lat: 372620000, Lon: -799480000}, osm.Node{ID: 61, Lat: 372620000, Lon: -799460000},
		osm.Node{ID: 62, Lat: 372640000, Lon: -799460000}, osm.Node{ID: 63, Lat: 372640000, Lon: -799480000},
	)
	row := func(i int) []int64 { return []int64{id(i, 0), id(i, 1), id(i, 2)} }
	col := func(j int) []int64 { return []int64{id(0, j), id(1, j), id(2, j)} }
	ways := []osm.Way{
		{ID: 100, Refs: row(0), Tags: tag("highway", "residential", "name", "Low Road")},
		{ID: 101, Refs: row(1), Tags: tag("highway", "primary", "name", "Main Street", "ref", "US 11")},
		{ID: 102, Refs: row(2), Tags: tag("highway", "residential", "name", "Oak Avenue", "oneway", "yes")},
		{ID: 103, Refs: col(0), Tags: tag("highway", "residential", "name", "1st Street")},
		{ID: 104, Refs: col(1), Tags: tag("highway", "residential", "name", "2nd Street")},
		{ID: 105, Refs: col(2), Tags: tag("highway", "residential", "name", "3rd Street")},
		{ID: 110, Refs: []int64{60, 61, 62}},
		{ID: 111, Refs: []int64{62, 63, 60}},
		{ID: 120, Refs: col(2), Tags: nil}, // also the county line
	}
	rels := []osm.Relation{
		{ID: 200, Members: []osm.Member{{Type: 'w', ID: 110, Role: "outer"}, {Type: 'w', ID: 111, Role: "outer"}}, Tags: tag("type", "multipolygon", "natural", "water", "name", "Test Lake")},
		{ID: 201, Members: []osm.Member{{Type: 'w', ID: 120, Role: "outer"}}, Tags: tag("type", "boundary", "boundary", "administrative", "admin_level", "6", "name", "Test County")},
	}
	var buf bytes.Buffer
	if err := osm.Write(&buf, nodes, ways, rels); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "town.osm.pbf")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

var testArea = Area{Name: "Testville", Lat: 37.27, Lon: -79.94, RadiusM: 20000}

func buildTown(t *testing.T) *Store {
	t.Helper()
	src := town(t)
	db := filepath.Join(t.TempDir(), "map-1.db")
	// The same extract twice, as overlapping state extracts hold the same data: nothing may double.
	st, err := Build(context.Background(), db, testArea, []string{src, src}, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.Addresses != 1 {
		t.Fatalf("addresses %d, want 1 (duplicates from overlapping extracts must be dropped)", st.Addresses)
	}
	if st.ByLayer[LRoad] != 6 || st.ByLayer[LWater] != 1 || st.ByLayer[LBoundary] != 1 {
		t.Fatalf("layers %v", st.ByLayer)
	}
	s, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSearch(t *testing.T) {
	s := buildTown(t)
	ctx := context.Background()
	cases := []struct {
		q, label, kind string
		exact          bool
	}{
		{"100 main st", "100 Main Street", "address", true},
		{"100 MAIN STREET TESTVILLE", "100 Main Street", "address", true},
		{"102 main", "100 Main Street", "address", false}, // not on file: the nearest number
		{"main & 2nd", "Main Street & 2nd Street", "intersection", true},
		{"oak ave and 3rd st", "Oak Avenue & 3rd Street", "intersection", true},
		{"oak", "Oak Avenue", "street", false},
		{"test fire", "Test Fire Station 1", "poi", false},
		{"test lake", "Test Lake", "area", false},
	}
	for _, c := range cases {
		r, err := s.Search(ctx, c.q, 0, 0, 5)
		if err != nil {
			t.Fatal(c.q, err)
		}
		if len(r) == 0 || r[0].Label != c.label || r[0].Kind != c.kind || r[0].Exact != c.exact {
			t.Errorf("%q: got %+v, want %s (%s, exact %v)", c.q, r, c.label, c.kind, c.exact)
		}
	}
	if r, _ := s.Search(ctx, "far away", 0, 0, 5); len(r) != 0 {
		t.Errorf("a place outside the area was recorded: %+v", r)
	}
	rv, err := s.Reverse(ctx, 37.2701, -79.9451)
	if err != nil || rv.Label != "100 Main Street" || !rv.Exact {
		t.Errorf("reverse: %+v %v", rv, err)
	}
	rv, err = s.Reverse(ctx, 37.2800, -79.9450)
	if err != nil || rv.Label != "Oak Avenue" {
		t.Errorf("reverse to a road: %+v %v", rv, err)
	}
}

func TestRouting(t *testing.T) {
	s := buildTown(t)
	r, err := s.Router()
	if err != nil {
		t.Fatal(err)
	}
	// Along Oak Avenue with its one-way, then against it.
	east := r.To(37.28, -79.93, 3600)
	with, ok := east.From(37.28, -79.95, true)
	if !ok {
		t.Fatal("no route with the one-way")
	}
	west := r.To(37.28, -79.95, 3600)
	against, ok := west.From(37.28, -79.93, true)
	if !ok {
		t.Fatal("no route against the one-way")
	}
	if against.Metres < with.Metres+1500 {
		t.Errorf("the route against the one-way (%.0f m) must go around the block (with it: %.0f m)", against.Metres, with.Metres)
	}
	if math.Abs(with.Metres-1770) > 60 {
		t.Errorf("Oak Avenue is about 1770 m, the route says %.0f", with.Metres)
	}
	if len(with.Via) == 0 || with.Via[0] != "Oak Avenue" {
		t.Errorf("via %v", with.Via)
	}
	// Around the block: the path passes the corner junctions.
	corner := PtDeg(37.27, -79.93)
	found := false
	for _, p := range against.Path {
		found = found || p == corner
	}
	if len(against.Path) < 4 || !found {
		t.Errorf("path %v", against.Path)
	}
	// Mid-block start and finish on the same street.
	mid := r.To(37.27, -79.935, 3600)
	rt, ok := mid.From(37.27, -79.945, false)
	if !ok || math.Abs(rt.Metres-885) > 60 {
		t.Errorf("mid-block drive %+v %v", rt, ok)
	}
	// Primary roads are faster than residential ones.
	if rt.Seconds > 885/(40/3.6) {
		t.Errorf("Main Street (primary) drove at residential speed: %.0f s", rt.Seconds)
	}
}

func TestTiles(t *testing.T) {
	s := buildTown(t)
	ctx := context.Background()
	x, y := tileOf(37.27, -79.94, 15)
	b, err := s.Tile(ctx, 15, x, y)
	if err != nil || len(b) == 0 {
		t.Fatalf("tile: %d bytes, %v", len(b), err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(zr)
	for _, l := range []string{"road", "area", "addr", "poi", "place"} {
		if !bytes.Contains(raw, []byte(l)) {
			t.Errorf("layer %s missing from the tile", l)
		}
	}
	if !bytes.Contains(raw, []byte("Main Street")) {
		t.Error("road name missing")
	}
	// Cached the second time, and identical.
	b2, _ := s.Tile(ctx, 15, x, y)
	if !bytes.Equal(b, b2) {
		t.Error("cached tile differs")
	}
	// Far outside the area: empty.
	if b, err := s.Tile(ctx, 10, 0, 0); err != nil || b != nil {
		t.Errorf("tile outside the area: %d bytes %v", len(b), err)
	}
	// Low zoom leaves residential streets out but keeps the primary road.
	x, y = tileOf(37.27, -79.94, 9)
	b, _ = s.Tile(ctx, 9, x, y)
	zr, _ = gzip.NewReader(bytes.NewReader(b))
	raw, _ = io.ReadAll(zr)
	if bytes.Contains(raw, []byte("Oak Avenue")) || !bytes.Contains(raw, []byte("primary")) {
		t.Error("zoom 9 tile has the wrong roads")
	}
}

func tileOf(lat, lon float64, z int) (int, int) {
	n := math.Exp2(float64(z))
	return int(mercX(lon) * n), int(mercY(lat) * n)
}

func TestGeomRoundTrip(t *testing.T) {
	g := Geom{Kind: GPolygon, Polys: [][][]Pt{{{{1, 2}, {300, -400}, {-5, 6}, {1, 2}}, {{10, 10}, {20, 20}, {10, 10}}}}}
	back, err := DecodeGeom(g.Encode())
	if err != nil || len(back.Polys) != 1 || len(back.Polys[0]) != 2 || back.Polys[0][0][1] != (Pt{300, -400}) {
		t.Fatalf("%+v %v", back, err)
	}
	if _, err := DecodeGeom([]byte{GLine, 200}); err == nil {
		t.Error("truncated geometry accepted")
	}
}

func FuzzDecodeGeom(f *testing.F) {
	f.Add((&Geom{Kind: GLine, Parts: [][]Pt{{{1, 1}, {2, 2}}}}).Encode())
	f.Fuzz(func(t *testing.T, b []byte) { DecodeGeom(b) })
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"123 Campbell Ave S.W.":     "123 campbell avenue southwest",
		"Campbell Avenue Southwest": "campbell avenue southwest",
		"I-581":                     "interstate 581",
		"Williamson Rd NE, Roanoke": "williamson road northeast roanoke",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string][2]string{
		"1234 Main St":  {"1234", "Main St"},
		"1234A Main St": {"1234A", "Main St"},
		"12th Street":   {"", "12th Street"},
		"Main St":       {"", "Main St"},
	} {
		n, r := splitNumber(in)
		if n != want[0] || r != want[1] {
			t.Errorf("%q: %q %q", in, n, r)
		}
	}
}

func TestChoose(t *testing.T) {
	// A country, the same country's grouping, and two states, one far away.
	idx := `{"features":[
	{"properties":{"id":"us","name":"United States","urls":{"pbf":"https://download.geofabrik.de/us.osm.pbf"}},"geometry":{"type":"Polygon","coordinates":[[[-125,25],[-66,25],[-66,49],[-125,49],[-125,25]]]}},
	{"properties":{"id":"us/virginia","parent":"north-america","name":"Virginia","urls":{"pbf":"https://download.geofabrik.de/va.osm.pbf"}},"geometry":{"type":"Polygon","coordinates":[[[-83.7,36.5],[-75.2,36.5],[-75.2,39.5],[-83.7,39.5],[-83.7,36.5]]]}},
	{"properties":{"id":"us/north-carolina","parent":"north-america","name":"North Carolina","urls":{"pbf":"https://download.geofabrik.de/nc.osm.pbf"}},"geometry":{"type":"Polygon","coordinates":[[[-84.3,33.8],[-75.4,33.8],[-75.4,36.6],[-84.3,36.6],[-84.3,33.8]]]}},
	{"properties":{"id":"us/oregon","parent":"north-america","name":"Oregon","urls":{"pbf":"https://download.geofabrik.de/or.osm.pbf"}},"geometry":{"type":"Polygon","coordinates":[[[-124.6,42],[-116.4,42],[-116.4,46.3],[-124.6,46.3],[-124.6,42]]]}}
	]}`
	ex, err := Choose([]byte(idx), Area{Lat: 37.27, Lon: -79.94, RadiusM: 160934})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, e := range ex {
		ids = append(ids, e.ID)
	}
	if strings.Join(ids, ",") != "us/north-carolina,us/virginia" {
		t.Errorf("chose %v", ids)
	}
}

func TestUpdateFromChangeFiles(t *testing.T) {
	// A Geofabrik-like update server with one day's change: Oak Avenue renamed.
	osc := `<osmChange><modify><way id="102"><nd ref="7"/><nd ref="8"/><nd ref="9"/><tag k="highway" v="residential"/><tag k="name" v="Elm Grove Road"/><tag k="oneway" v="yes"/></way></modify></osmChange>`
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write([]byte(osc))
	zw.Close()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x-updates/state.txt":
			fmt.Fprint(w, "#Sat Oct 18\nsequenceNumber=11\ntimestamp=2026-10-18T20\\:21\\:46Z\n")
		case "/x-updates/000/000/011.osc.gz":
			w.Write(gz.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	dir := t.TempDir()
	src := town(t)
	// Rewrite the extract with a replication header pointing at the test server.
	pbf := filepath.Join(dir, "osm", "town.osm.pbf")
	os.MkdirAll(filepath.Dir(pbf), 0o700)
	c := osm.NewChange()
	if err := osm.Apply(src, pbf, c, osm.Header{Sequence: 10, BaseURL: "https://download.geofabrik.de/x-updates"}, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(context.Background(), filepath.Join(dir, "map-1.db"), testArea, []string{pbf}, "test", nil); err != nil {
		t.Fatal(err)
	}
	s := NewService(dir, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	// The service only follows Geofabrik; the test server stands in for it.
	s.Client = &http.Client{Transport: rewrite{to: ts.URL}}
	if err := s.Update("admin"); err != nil {
		t.Fatal(err)
	}
	var j *Job
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if j = s.Job(); j.Finished != nil {
			break
		}
	}
	if j.Error != "" || j.Changes != 1 {
		t.Fatalf("job %+v", j)
	}
	r, _ := s.Store().Search(context.Background(), "elm grove", 0, 0, 3)
	if len(r) == 0 || r[0].Label != "Elm Grove Road" {
		t.Fatalf("renamed street not found: %+v", r)
	}
	if h, _ := osm.ReadHeader(pbf); h.Sequence != 11 {
		t.Fatalf("header %+v", h)
	}
	// Again: nothing new, and the map stays as it is.
	s.Update("admin")
	for j = s.Job(); j.Finished == nil; j = s.Job() {
		time.Sleep(10 * time.Millisecond)
	}
	if j.Error != "" || j.Progress.Phase != "Already up to date" {
		t.Fatalf("second update %+v", j)
	}
}

// rewrite sends every request to the test server, keeping the path.
type rewrite struct{ to string }

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	u, _ := url.Parse(r.to + req.URL.Path)
	req2 := req.Clone(req.Context())
	req2.URL, req2.Host = u, u.Host
	return http.DefaultTransport.RoundTrip(req2)
}
