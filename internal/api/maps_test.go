package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/bitvoice"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/mapdata"
	"github.com/biodream-llc/bitdispatcher/internal/osm"
)

// mapServer is a server with a recorded map: two crossing streets and an address.
func mapServer(t *testing.T) (*httptest.Server, *Server, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	nodes := []osm.Node{
		{ID: 1, Lat: 372700000, Lon: -799500000}, {ID: 2, Lat: 372700000, Lon: -799400000}, {ID: 3, Lat: 372700000, Lon: -799300000},
		{ID: 4, Lat: 372600000, Lon: -799400000}, {ID: 5, Lat: 372800000, Lon: -799400000},
		{ID: 9, Lat: 372701000, Lon: -799350000, Tags: map[string]string{"addr:housenumber": "500", "addr:street": "Main Street"}},
	}
	ways := []osm.Way{
		{ID: 10, Refs: []int64{1, 2, 3}, Tags: map[string]string{"highway": "secondary", "name": "Main Street"}},
		{ID: 11, Refs: []int64{4, 2, 5}, Tags: map[string]string{"highway": "residential", "name": "Elm Street"}},
	}
	var buf bytes.Buffer
	osm.Write(&buf, nodes, ways, nil)
	src := filepath.Join(dir, "t.osm.pbf")
	os.WriteFile(src, buf.Bytes(), 0o600)
	mapDir := filepath.Join(dir, "map")
	os.MkdirAll(mapDir, 0o700)
	if _, err := mapdata.Build(context.Background(), filepath.Join(mapDir, "map-1.db"), mapdata.Area{Name: "T", Lat: 37.27, Lon: -79.94, RadiusM: 10000}, []string{src}, "test", nil); err != nil {
		t.Fatal(err)
	}
	eng, err := cad.Open(dir + "/ledger.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { eng.Close() })
	users, _ := auth.Open(dir)
	pw := map[string]string{}
	for _, u := range [][2]string{{"admin", auth.Admin}, {"d1", auth.Dispatcher}, {"v1", auth.Viewer}} {
		p := auth.GeneratePassword()
		pw[u[0]] = p
		users.Save(u[0], u[0], u[1], p, false)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := mapdata.NewService(mapDir, "test", log)
	if svc.Store() == nil {
		t.Fatal("the recorded map did not open")
	}
	s, err := New(&Server{Eng: eng, Auth: users, BV: bitvoice.New(eng, log), Log: log, DataDir: dir, Version: "test", Maps: svc})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	ts := httptest.NewTLSServer(s)
	t.Cleanup(ts.Close)
	return ts, s, pw
}

func TestMapAPI(t *testing.T) {
	ts, s, pw := mapServer(t)
	v := login(t, ts, "v1", pw["v1"])
	code, b := v.do("GET", "/api/map/search?q=500+main+st", nil, false)
	var res []mapdata.Result
	json.Unmarshal(b, &res)
	if code != 200 || len(res) == 0 || res[0].Label != "500 Main Street" || !res[0].Exact {
		t.Fatalf("search: %d %s", code, b)
	}
	code, b = v.do("GET", "/api/map/search?q=main+%26+elm", nil, false)
	if code != 200 || !strings.Contains(string(b), `Main Street \u0026 Elm Street`) {
		t.Fatalf("intersection: %d %s", code, b)
	}
	if code, b = v.do("GET", "/api/map/reverse?lat=37.2701&lon=-79.9351", nil, false); code != 200 || !strings.Contains(string(b), "500 Main Street") {
		t.Fatalf("reverse: %d %s", code, b)
	}
	if code, _ = v.do("GET", "/api/map/tiles/14/4553/6361", nil, false); code != 200 && code != 204 {
		t.Fatalf("tile: %d", code)
	}
	if code, b = v.do("GET", "/api/map/fonts/Noto%20Sans%20Regular/0-255.pbf", nil, false); code != 200 || len(b) < 1000 {
		t.Fatalf("glyphs: %d %d bytes", code, len(b))
	}
	code, b = v.do("POST", "/api/map/routes", map[string]any{"to": map[string]float64{"lat": 37.27, "lon": -79.93}, "from": []map[string]any{{"id": "E1", "lat": 37.28, "lon": -79.94}}}, true)
	var rts []RouteView
	json.Unmarshal(b, &rts)
	if code != 200 || len(rts) != 1 || !rts[0].OK || len(rts[0].Path) < 3 || rts[0].Metres < 1500 {
		t.Fatalf("routes: %d %s", code, b)
	}
	// Recording is for administrators, and only from Geofabrik.
	if code, _ = v.do("POST", "/api/admin/map/plan", map[string]any{"lat": 37.27, "lon": -79.94, "radiusMiles": 10}, true); code != 403 {
		t.Fatalf("viewer could plan a recording: %d", code)
	}
	a := login(t, ts, "admin", pw["admin"])
	code, _ = a.do("POST", "/api/admin/map/record", map[string]any{"lat": 37.27, "lon": -79.94, "radiusMiles": 10,
		"extracts": []map[string]string{{"id": "x", "url": "http://169.254.169.254/latest.osm.pbf"}}}, true)
	if code != 400 {
		t.Fatalf("a non-Geofabrik extract was accepted: %d", code)
	}

	// The command line places an address it finds exactly, and recommendations then drive by road.
	eng := s.Eng
	if err := eng.SaveStation(cad.SystemActor, cad.Station{ID: "ST1", Name: "Station 1", Lat: 37.28, Lon: -79.94}); err != nil {
		t.Fatal(err)
	}
	if err := eng.SaveUnit(cad.SystemActor, cad.UnitDef{ID: "E1", Type: "engine", Station: "ST1"}); err != nil {
		t.Fatal(err)
	}
	eng.SetStatus(cad.SystemActor, "E1", cad.Available, "", "test")
	d := login(t, ts, "d1", pw["d1"])
	code, b = d.do("POST", "/api/command", map[string]string{"line": "NEW SF 500 Main St"}, true)
	var cr cad.CommandResult
	json.Unmarshal(b, &cr)
	if code != 200 || !cr.OK || !strings.Contains(cr.Message, "500 Main Street") {
		t.Fatalf("NEW: %d %s", code, b)
	}
	code, b = d.do("GET", "/api/incidents/"+cr.Incident+"/recommend", nil, false)
	var rec cad.Recommendation
	json.Unmarshal(b, &rec)
	if code != 200 || !strings.Contains(rec.Estimate, "road network") {
		t.Fatalf("recommend: %d %s", code, b)
	}
	routed := false
	for _, l := range rec.Lines {
		for _, c := range l.Units {
			routed = routed || (c.Unit == "E1" && c.Routed)
		}
	}
	if !routed {
		t.Fatalf("E1 was not routed by road: %s", b)
	}
}
