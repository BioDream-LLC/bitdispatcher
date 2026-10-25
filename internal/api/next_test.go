package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/bitvoice"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/mapdata"
	"github.com/biodream-llc/bitdispatcher/internal/osm"
)

// gridServer is a server with a recorded 3×3 street grid around 37.27, -79.94: Low Road, Main Street and
// High Road running east-west, 1st, 2nd and 3rd Street north-south.
func gridServer(t *testing.T) (*httptest.Server, *Server, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	lats := []float64{37.26, 37.27, 37.28}
	lons := []float64{-79.95, -79.94, -79.93}
	id := func(i, j int) int64 { return int64(1 + i*3 + j) }
	var nodes []osm.Node
	for i, la := range lats {
		for j, lo := range lons {
			nodes = append(nodes, osm.Node{ID: id(i, j), Lat: int32(la * 1e7), Lon: int32(lo * 1e7)})
		}
	}
	nodes = append(nodes, osm.Node{ID: 50, Lat: 372701000, Lon: -799350000, Tags: map[string]string{"addr:housenumber": "500", "addr:street": "Main Street"}})
	row := func(i int) []int64 { return []int64{id(i, 0), id(i, 1), id(i, 2)} }
	col := func(j int) []int64 { return []int64{id(0, j), id(1, j), id(2, j)} }
	ways := []osm.Way{
		{ID: 100, Refs: row(0), Tags: map[string]string{"highway": "residential", "name": "Low Road"}},
		{ID: 101, Refs: row(1), Tags: map[string]string{"highway": "primary", "name": "Main Street"}},
		{ID: 102, Refs: row(2), Tags: map[string]string{"highway": "residential", "name": "High Road"}},
		{ID: 103, Refs: col(0), Tags: map[string]string{"highway": "residential", "name": "1st Street"}},
		{ID: 104, Refs: col(1), Tags: map[string]string{"highway": "residential", "name": "2nd Street"}},
		{ID: 105, Refs: col(2), Tags: map[string]string{"highway": "residential", "name": "3rd Street"}},
	}
	var buf bytes.Buffer
	osm.Write(&buf, nodes, ways, nil)
	src := filepath.Join(dir, "g.osm.pbf")
	os.WriteFile(src, buf.Bytes(), 0o600)
	mapDir := filepath.Join(dir, "map")
	os.MkdirAll(mapDir, 0o700)
	if _, err := mapdata.Build(context.Background(), filepath.Join(mapDir, "map-1.db"), mapdata.Area{Name: "G", Lat: 37.27, Lon: -79.94, RadiusM: 10000}, []string{src}, "test", nil); err != nil {
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
	s, err := New(&Server{Eng: eng, Auth: users, BV: bitvoice.New(eng, log), Log: log, DataDir: dir, Version: "test", Maps: mapdata.NewService(mapDir, "test", log)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	ts := httptest.NewTLSServer(s)
	t.Cleanup(ts.Close)
	return ts, s, pw
}

func driveSecs(t *testing.T, c *client, fromLat, fromLon, toLat, toLon float64) (float64, float64) {
	t.Helper()
	code, b := c.do("POST", "/api/map/routes", map[string]any{"to": map[string]float64{"lat": toLat, "lon": toLon}, "from": []map[string]any{{"id": "X", "lat": fromLat, "lon": fromLon}}}, true)
	var rts []RouteView
	json.Unmarshal(b, &rts)
	if code != 200 || len(rts) != 1 || !rts[0].OK {
		t.Fatalf("route: %d %s", code, b)
	}
	return rts[0].Seconds, rts[0].Metres
}

// A dispatcher closes Main Street: drives go around it, the recommendation says so, the map has its shape;
// lifting it brings the drive back. A Waze jam slows the drive without changing its way.
func TestRoadClosuresAndTraffic(t *testing.T) {
	ts, s, pw := gridServer(t)
	d := login(t, ts, "d1", pw["d1"])
	v := login(t, ts, "v1", pw["v1"])
	// From 1st Street just south of Main to 3rd Street just north of it: Main Street is the way.
	_, open := driveSecs(t, d, 37.268, -79.95, 37.272, -79.93)
	if code, _ := v.do("PUT", "/api/roads/closures", cad.RoadClosure{Reason: "crash", Line: [][2]float64{{-79.948, 37.27}, {-79.932, 37.27}}}, true); code != 403 {
		t.Fatalf("a viewer closed a road: %d", code)
	}
	code, b := d.do("PUT", "/api/roads/closures", cad.RoadClosure{Reason: "Crash, all lanes", Line: [][2]float64{{-79.948, 37.27}, {-79.932, 37.27}}, Detour: "High Road"}, true)
	var made struct {
		ID       string
		Segments int
		Roads    []string
	}
	json.Unmarshal(b, &made)
	if code != 200 || made.Segments != 2 || len(made.Roads) != 1 || made.Roads[0] != "Main Street" {
		t.Fatalf("closure: %d %s", code, b)
	}
	_, around := driveSecs(t, d, 37.268, -79.95, 37.272, -79.93)
	if around < open+500 {
		t.Fatalf("with Main Street closed the drive is %.0f m; open it was %.0f", around, open)
	}
	_, b = v.do("GET", "/api/roads", nil, false)
	if !strings.Contains(string(b), `"shapes":[[[`) || !strings.Contains(string(b), "Crash, all lanes") {
		t.Fatalf("roads: %s", b)
	}
	if !strings.Contains(s.roadsLabel(), "1 road closure") {
		t.Fatalf("label %q", s.roadsLabel())
	}
	if code, b := d.do("POST", "/api/roads/closures/"+made.ID+"/lift", nil, true); code != 204 && code != 200 {
		t.Fatalf("lift: %d %s", code, b)
	}
	if _, back := driveSecs(t, d, 37.268, -79.95, 37.272, -79.93); back != open {
		t.Fatalf("after lifting, %.0f m; open was %.0f", back, open)
	}

	// Traffic: a Waze for Cities feed with a jam along Main Street at 5 km/h.
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"jams":[{"line":[{"x":-79.949,"y":37.27},{"x":-79.931,"y":37.27}],"speedKMH":5,"level":4,"street":"Main Street","delay":300}]}`)
	}))
	defer feed.Close()
	a := login(t, ts, "admin", pw["admin"])
	if code, _ := a.do("PUT", "/api/admin/traffic", Traffic{On: true, Kind: "waze", URL: "http://example.com/feed"}, true); code != 422 {
		t.Fatalf("a plain-http feed off this server accepted: %d", code)
	}
	if code, b := a.do("PUT", "/api/admin/traffic", Traffic{On: true, Kind: "waze", URL: feed.URL}, true); code != 204 {
		t.Fatalf("traffic: %d %s", code, b)
	}
	slowBefore, _ := driveSecs(t, d, 37.27, -79.949, 37.27, -79.931)
	jams, err := fetchTraffic(context.Background(), http.DefaultClient, s.nextConfig().Traffic)
	if err != nil || len(jams) != 1 || jams[0].Kmh != 5 {
		t.Fatalf("waze: %+v %v", jams, err)
	}
	s.next.mu.Lock()
	s.next.traffic = trafficNow{At: time.Now(), Jams: jams, Kind: "waze"}
	s.next.mu.Unlock()
	s.applyRoads(true)
	slow, _ := driveSecs(t, d, 37.27, -79.949, 37.27, -79.931)
	if slow < slowBefore*2 {
		t.Fatalf("with a 5 km/h jam the drive is %.0f s; before %.0f s", slow, slowBefore)
	}
	if !strings.Contains(s.roadsLabel(), "live traffic (Waze for Cities, 1 slowdowns") {
		t.Fatalf("label %q", s.roadsLabel())
	}
	// GeoJSON: a closed road and a factor.
	g, err := parseGeoJSONTraffic([]byte(`{"type":"FeatureCollection","features":[
		{"type":"Feature","geometry":{"type":"LineString","coordinates":[[-79.95,37.26],[-79.93,37.26]]},"properties":{"closed":true,"name":"Low Road"}},
		{"type":"Feature","geometry":{"type":"LineString","coordinates":[[-79.95,37.28],[-79.93,37.28]]},"properties":{"factor":3}},
		{"type":"Feature","geometry":{"type":"LineString","coordinates":[[-79.95,37.28],[-79.93,37.28]]},"properties":{}}]}`))
	if err != nil || len(g) != 2 || !g[0].Closed || g[1].given != 3 {
		t.Fatalf("geojson: %+v %v", g, err)
	}
}

// Cameras: off until switched on; a dispatcher sees those near an incident, opens one with a reason, and
// only then gets its still picture; the supervisor sees who looked and why.
func TestCamerasNearIncident(t *testing.T) {
	ts, s, pw := newServer(t)
	a := login(t, ts, "admin", pw["admin"])
	d := login(t, ts, "d1", pw["d1"])
	pic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("\xff\xd8\xff\xe0 jpeg"))
	}))
	defer pic.Close()
	north := 0.0
	code, b := a.do("PUT", "/api/admin/cameras", cad.Camera{Name: "Main & 2nd", Owner: "City traffic", Lat: 40.2690, Lon: -76.8800, Heading: &north, Still: pic.URL + "/cam.jpg", Live: "https://vms.example.gov/cam/7"}, true)
	var cam struct{ ID string }
	json.Unmarshal(b, &cam)
	if code != 200 || cam.ID == "" {
		t.Fatalf("camera: %d %s", code, b)
	}
	if code, _ := d.do("PUT", "/api/admin/cameras", cad.Camera{Name: "x", Lat: 40.27, Lon: -76.88, Still: pic.URL}, true); code != 403 {
		t.Fatalf("a dispatcher added a camera: %d", code)
	}
	code, b = d.do("POST", "/api/incidents", map[string]any{"type": "card", "location": map[string]any{"lat": 40.2700, "lon": -76.8800, "address": "2nd & Main"}}, true)
	var inc struct{ Number string }
	json.Unmarshal(b, &inc)
	if code != 201 {
		t.Fatalf("incident: %d %s", code, b)
	}
	if _, b := d.do("GET", "/api/cameras/near?incident="+inc.Number, nil, false); !strings.Contains(string(b), `"on":false`) {
		t.Fatalf("cameras offered while off: %s", b)
	}
	st := cad.DefaultSettings()
	s.Eng.Read(func(x *cad.State) { st = x.Settings })
	st.Next.Cameras.On = true
	if code, b := a.do("PUT", "/api/admin/settings", st, true); code != 204 && code != 200 {
		t.Fatalf("settings: %d %s", code, b)
	}
	if _, b := d.do("GET", "/api/cameras/near?incident="+inc.Number, nil, false); !strings.Contains(string(b), `"name":"Main \u0026 2nd"`) || !strings.Contains(string(b), `"facing":true`) {
		t.Fatalf("near: %s", b)
	}
	if code, _ := d.do("GET", "/api/cameras/"+cam.ID+"/still", nil, false); code != 403 {
		t.Fatalf("a still picture before opening the camera: %d", code)
	}
	if code, b := d.do("POST", "/api/cameras/"+cam.ID+"/open", map[string]string{}, true); code != 422 {
		t.Fatalf("opened without a reason: %d %s", code, b)
	}
	if code, b := d.do("POST", "/api/cameras/"+cam.ID+"/open", map[string]string{"incident": inc.Number}, true); code != 200 || !strings.Contains(string(b), "vms.example.gov") {
		t.Fatalf("open: %d %s", code, b)
	}
	if code, b := d.do("GET", "/api/cameras/"+cam.ID+"/still", nil, false); code != 200 || !bytes.HasPrefix(b, []byte("\xff\xd8")) {
		t.Fatalf("still: %d %q", code, b)
	}
	if _, b := a.do("GET", "/api/admin/cameras/looks", nil, false); !strings.Contains(string(b), `"user":"d1"`) || !strings.Contains(string(b), "incident "+inc.Number) {
		t.Fatalf("looks: %s", b)
	}
}

// A location link: the call-taker makes it, the caller's page posts the phone's position, the call has it,
// too-fast positions are let go, and an ended link takes no more.
func TestLocationLink(t *testing.T) {
	ts, s, pw := newServer(t)
	d := login(t, ts, "d1", pw["d1"])
	if err := s.Eng.UpsertCall(cad.BitVoiceActor, "c1", cad.CallData{Source: "bitvoice", State: "connected", Callback: "+15405550100", Emergency: true}); err != nil {
		t.Fatal(err)
	}
	code, b := d.do("POST", "/api/calls/c1/location-link", map[string]bool{"send": false}, true)
	var l struct{ ID, URL, Sent string }
	json.Unmarshal(b, &l)
	if code != 200 || !strings.Contains(l.URL, "/here/") || l.Sent != "read it to the caller" {
		t.Fatalf("link: %d %s", code, b)
	}
	path := l.URL[strings.Index(l.URL, "/here/"):]
	anon := &client{t: t, base: ts.URL, c: ts.Client()}
	if code, b := anon.do("GET", path, nil, false); code != 200 || !strings.Contains(string(b), "Share my location") {
		t.Fatalf("page: %d %.200s", code, b)
	}
	if code, _ := anon.do("GET", "/here/not-a-real-secret-xyz", nil, false); code != 410 {
		t.Fatalf("a made-up link: %d", code)
	}
	if code, _ := anon.do("POST", path, map[string]any{"lat": 37.2712, "lon": -79.9411, "accuracy": 8}, false); code != 204 {
		t.Fatalf("fix: %d", code)
	}
	s.Eng.Read(func(st *cad.State) {
		c := st.Calls["c1"]
		if c.Location == nil || c.Location.Lat == nil || *c.Location.Lat != 37.2712 || !strings.Contains(c.Location.Method, "by link") {
			t.Fatalf("the call's location %+v", c.Location)
		}
	})
	if code, _ := anon.do("POST", path, map[string]any{"lat": 37.3, "lon": -79.9, "accuracy": 8}, false); code != 204 {
		t.Fatalf("a fast second fix: %d", code)
	}
	s.Eng.Read(func(st *cad.State) {
		if *st.Calls["c1"].Location.Lat != 37.2712 {
			t.Fatal("a position two seconds after the last was taken")
		}
	})
	if code, _ := d.do("POST", "/api/location-links/"+l.ID+"/end", nil, true); code != 204 && code != 200 {
		t.Fatalf("end: %d", code)
	}
	if code, _ := anon.do("POST", path, map[string]any{"lat": 37.3, "lon": -79.9, "accuracy": 8}, false); code != 410 {
		t.Fatalf("an ended link took a position: %d", code)
	}
}

// An alarm company with its own address: its token, its name on the alarm, the permit check and the
// repeat false alarms; a phoned alarm is typed by the call-taker.
func TestAlarmCompaniesAndPermits(t *testing.T) {
	ts, s, pw := newServer(t)
	a := login(t, ts, "admin", pw["admin"])
	d := login(t, ts, "d1", pw["d1"])
	code, b := a.do("PUT", "/api/admin/alarms", AlarmCompany{Name: "Acme Monitoring", Phone: "+18005550100"}, true)
	var co struct{ ID, Token, Address string }
	json.Unmarshal(b, &co)
	if code != 200 || co.ID != "acme-monitoring" || len(co.Token) < 40 {
		t.Fatalf("company: %d %s", code, b)
	}
	if _, b := a.do("GET", "/api/admin/alarms", nil, false); strings.Contains(string(b), co.Token) {
		t.Fatal("the company's token was shown again")
	}
	req, _ := http.NewRequest("PUT", ts.URL+"/api/admin/alarms/permits", strings.NewReader("number,address,holder,expires\nP-1,12 Main Street,Pharmacy,2099-01-01\nP-2,40 Oak Ave,Bakery,2020-01-01\n"))
	req.Header.Set(csrfHeader, "1")
	resp, err := a.c.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("permits: %v %v", resp.StatusCode, err)
	}
	resp.Body.Close()
	// Three false alarms at 40 Oak Ave this year.
	for i := 0; i < 3; i++ {
		code, b := d.do("POST", "/api/incidents", map[string]any{"type": "card", "location": map[string]any{"address": "40 Oak Avenue"}}, true)
		var inc struct{ Number string }
		json.Unmarshal(b, &inc)
		if code != 201 {
			t.Fatalf("incident: %d %s", code, b)
		}
		if err := s.Eng.CloseIncident(cad.SystemActor, inc.Number, "False alarm", ""); err != nil {
			t.Fatal(err)
		}
	}
	alarm := map[string]any{"id": "EV-9", "event": "burglary", "address": "40 Oak Ave"}
	if code, _ := hookPost(t, ts, "/api/hooks/alarm/acme-monitoring", "wrong", alarm); code != 401 {
		t.Fatalf("wrong token: %d", code)
	}
	code, b = hookPost(t, ts, "/api/hooks/alarm/acme-monitoring", co.Token, alarm)
	var r struct{ Call string }
	json.Unmarshal(b, &r)
	if code != 200 {
		t.Fatalf("alarm: %d %s", code, b)
	}
	s.Eng.Read(func(st *cad.State) {
		c := st.Calls[r.Call]
		notes := strings.Join(c.Notes, " | ")
		if c.Alert.Sender != "Acme Monitoring" || c.Alert.Company != "acme-monitoring" || !strings.Contains(notes, "Alarm permit P-2: EXPIRED 2020-01-01") ||
			!strings.Contains(notes, "REPEAT FALSE ALARMS: 3") {
			t.Fatalf("alarm call %+v notes %s", c.Alert, notes)
		}
	})
	// Phoned in: typed by the call-taker; the permit is valid.
	code, b = d.do("POST", "/api/alarms/phoned", map[string]any{"sender": "Other Alarm Co", "id": "77", "event": "fire", "address": "12 Main St", "operator": "Op 4"}, true)
	json.Unmarshal(b, &r)
	if code != 200 {
		t.Fatalf("phoned: %d %s", code, b)
	}
	s.Eng.Read(func(st *cad.State) {
		c := st.Calls[r.Call]
		notes := strings.Join(c.Notes, " | ")
		if !c.Alert.Phoned || !strings.Contains(notes, "Alarm permit P-1: valid") || !strings.Contains(notes, "typed by d1") {
			t.Fatalf("phoned alarm %+v %s", c.Alert, notes)
		}
	})
}

// The intake: a bot with the token runs the conversation; a report becomes a call in the non-emergency
// list that does not time out; an emergency is handed over.
func TestIntakeAPI(t *testing.T) {
	ts, s, pw := newServer(t)
	a := login(t, ts, "admin", pw["admin"])
	if code, _ := hookPost(t, ts, "/api/intake/sessions", "x", map[string]string{}); code != 503 {
		t.Fatalf("intake without a token: %d", code)
	}
	_, b := a.do("PUT", "/api/admin/intake/token", map[string]bool{}, true)
	var tk struct{ Token string }
	json.Unmarshal(b, &tk)
	st := cad.DefaultSettings()
	s.Eng.Read(func(x *cad.State) { st = x.Settings })
	st.Next.Intake.On = true
	if code, b := a.do("PUT", "/api/admin/settings", st, true); code != 204 && code != 200 {
		t.Fatalf("settings: %d %s", code, b)
	}
	talk := func(say ...string) (string, map[string]any) {
		_, b := hookPost(t, ts, "/api/intake/sessions", tk.Token, map[string]string{"channel": "voice", "from": "+15405550100"})
		var st struct{ Session string }
		json.Unmarshal(b, &st)
		var last map[string]any
		for _, x := range say {
			_, b = hookPost(t, ts, "/api/intake/sessions/"+st.Session, tk.Token, map[string]string{"text": x})
			last = map[string]any{}
			json.Unmarshal(b, &last)
		}
		return st.Session, last
	}
	_, out := talk("no", "my neighbour's dog barks all night", "300 Elm Street", "every night", "Sam", "540-555-0100")
	turn := out["turn"].(map[string]any)
	if turn["done"] != true || turn["transfer"] == true || !strings.Contains(turn["say"].(string), "report number is NE-") {
		t.Fatalf("report: %v", out)
	}
	call := out["call"].(string)
	s.Eng.Read(func(st *cad.State) {
		c := st.Calls[call]
		if c == nil || c.State != "data" || c.Screen == nil || !c.Screen.NonEmergency || c.Callback != "540-555-0100" {
			t.Fatalf("the report's call %+v", c)
		}
	})
	_, out = talk("no", "someone is breaking in next door right now")
	turn = out["turn"].(map[string]any)
	if turn["transfer"] != true {
		t.Fatalf("an emergency was not handed over: %v", out)
	}
}
