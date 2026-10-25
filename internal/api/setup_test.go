package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/paging"
)

func setupOf(t *testing.T, c *client) SetupView {
	t.Helper()
	code, b := c.do("GET", "/api/admin/setup", nil, false)
	var v SetupView
	json.Unmarshal(b, &v)
	if code != 200 || len(v.Steps) == 0 {
		t.Fatalf("setup: %d %s", code, b)
	}
	return v
}

func stepOf(v SetupView, id string) SetupStep {
	for _, x := range v.Steps {
		if x.ID == id {
			return x
		}
	}
	return SetupStep{}
}

// Two stations on the grid, each with an engine.
func gridStations(t *testing.T, a *client, s *Server) {
	t.Helper()
	for _, x := range []cad.Station{{ID: "S1", Name: "Station 1", Lat: 37.26, Lon: -79.95}, {ID: "S2", Name: "Station 2", Lat: 37.28, Lon: -79.93}} {
		if code, b := a.do("PUT", "/api/admin/stations/"+x.ID, x, true); code != 204 {
			t.Fatalf("station: %d %s", code, b)
		}
	}
	for _, u := range []cad.UnitDef{{ID: "E1", Type: "engine", Station: "S1"}, {ID: "E2", Type: "engine", Station: "S2"}} {
		if code, b := a.do("PUT", "/api/admin/units/"+u.ID, u, true); code != 204 {
			t.Fatalf("unit: %d %s", code, b)
		}
		if err := s.Eng.SetStatus(cad.SystemActor, u.ID, cad.Available, "", ""); err != nil {
			t.Fatal(err)
		}
	}
}

// The checklist follows what is configured; only a person can tick training, and a step bitdispatcher
// checks itself cannot be ticked.
func TestSetupChecklist(t *testing.T) {
	ts, s, pw := gridServer(t)
	a := login(t, ts, "admin", pw["admin"])
	d := login(t, ts, "d1", pw["d1"])
	if code, _ := d.do("GET", "/api/admin/setup", nil, false); code != 403 {
		t.Fatalf("a dispatcher saw the checklist: %d", code)
	}
	v := setupOf(t, a)
	if stepOf(v, "map").Status != "done" || stepOf(v, "stations").Status != "todo" || stepOf(v, "training").Status != "todo" {
		t.Fatalf("a new system: %+v", v.Steps)
	}
	if !strings.Contains(stepOf(v, "users").Detail, "administrators without an authenticator: admin") {
		t.Fatalf("users: %+v", stepOf(v, "users"))
	}
	if v.Left == 0 || v.Required == 0 || v.Support != DefaultSupport {
		t.Fatalf("counts: %+v", v)
	}
	gridStations(t, a, s)
	v = setupOf(t, a)
	if stepOf(v, "stations").Status != "done" || stepOf(v, "units").Status != "done" {
		t.Fatalf("with stations and units: %+v %+v", stepOf(v, "stations"), stepOf(v, "units"))
	}
	if code, _ := a.do("PUT", "/api/admin/setup/stations", map[string]any{"done": true}, true); code != 422 {
		t.Fatalf("a checked step was ticked: %d", code)
	}
	if code, _ := a.do("PUT", "/api/admin/setup/gis", map[string]any{"done": true}, true); code != 422 {
		t.Fatalf("not needed without a reason: %d", code)
	}
	if code, b := a.do("PUT", "/api/admin/setup/gis", map[string]any{"done": true, "note": "the open map's addresses are enough here"}, true); code != 200 {
		t.Fatalf("gis not needed: %d %s", code, b)
	}
	if code, b := a.do("PUT", "/api/admin/setup/training", map[string]any{"done": true, "note": "all three shifts"}, true); code != 200 {
		t.Fatalf("training: %d %s", code, b)
	}
	v = setupOf(t, a)
	if x := stepOf(v, "training"); x.Status != "done" || x.Tick == nil || x.Tick.By != "admin" || x.Tick.Note != "all three shifts" {
		t.Fatalf("training ticked: %+v", x)
	}
	if stepOf(v, "gis").Status != "done" {
		t.Fatalf("gis: %+v", stepOf(v, "gis"))
	}
	a.do("PUT", "/api/admin/setup/training", map[string]any{"done": false}, true)
	if stepOf(setupOf(t, a), "training").Status != "todo" {
		t.Fatal("training still ticked")
	}
	// The support contact is a setting.
	st := cad.DefaultSettings()
	s.Eng.Read(func(x *cad.State) { st = x.Settings })
	st.Next.Support = "County IT help desk, 24 hours"
	if code, b := a.do("PUT", "/api/admin/settings", st, true); code != 204 {
		t.Fatalf("settings: %d %s", code, b)
	}
	if v := setupOf(t, a); v.Support != "County IT help desk, 24 hours" || stepOf(v, "support").Status != "done" {
		t.Fatalf("support: %+v", stepOf(v, "support"))
	}
}

// The go-live check: the ledger, a restore, test calls at each station that write nothing, and what is
// missing (an authenticator, a second site).
func TestGoLiveCheck(t *testing.T) {
	ts, s, pw := gridServer(t)
	a := login(t, ts, "admin", pw["admin"])
	gridStations(t, a, s)
	var before uint64
	var incidents int
	s.Eng.Read(func(st *cad.State) { before, incidents = st.Seq, len(st.Incidents) })
	code, b := a.do("POST", "/api/admin/golive", nil, true)
	var g cad.GoLiveResult
	json.Unmarshal(b, &g)
	if code != 200 || len(g.Checks) == 0 {
		t.Fatalf("go-live: %d %s", code, b)
	}
	res := map[string]cad.GoLiveItem{}
	for _, c := range g.Checks {
		res[c.ID] = c
	}
	for id, want := range map[string]string{"ledger": "pass", "restore": "pass", "demo": "pass", "mfa": "fail", "map": "pass", "sites": "warn"} {
		if res[id].Result != want {
			t.Errorf("%s: %+v, want %s", id, res[id], want)
		}
	}
	if !strings.Contains(res["restore"].Detail, "replayed") {
		t.Errorf("restore: %+v", res["restore"])
	}
	// Each station's own engine comes first for a call at the station; the grid map gives road times.
	if c := res["calls"]; c.Result == "fail" || strings.Contains(c.Detail, "before the station's own") || strings.Contains(c.Detail, "Straight line") {
		t.Errorf("test calls: %+v", c)
	}
	var after uint64
	s.Eng.Read(func(st *cad.State) {
		after = st.Seq
		if len(st.Incidents) != incidents || st.Incidents["GO-LIVE-TEST"] != nil {
			t.Errorf("a test call was left behind: %d incidents", len(st.Incidents))
		}
		if st.GoLive == nil || st.GoLive.Fail != g.Fail || st.GoLive.By != "admin" {
			t.Errorf("not recorded: %+v", st.GoLive)
		}
	})
	if after != before+1 {
		t.Errorf("the check wrote %d events; one (its result) expected", after-before)
	}
	if x := stepOf(setupOf(t, a), "golive"); x.Status != "todo" || !strings.Contains(x.Detail, "failed") {
		t.Errorf("checklist after a failing check: %+v", x)
	}
	if code, _ := login(t, ts, "d1", pw["d1"]).do("POST", "/api/admin/golive", nil, true); code != 403 {
		t.Errorf("a dispatcher ran the check: %d", code)
	}
}

// The support bundle holds no secrets, no usernames and no numbers, and its download is recorded.
func TestSupportBundle(t *testing.T) {
	ts, s, pw := gridServer(t)
	s.Auth.Save("zeldaroot", "Zelda Quartermain", auth.Admin, "a-long-password-here-1", false)
	a := login(t, ts, "admin", pw["admin"])
	gridStations(t, a, s)
	var logged bytes.Buffer
	s.Logs = NewLogRing(slog.NewTextHandler(&logged, nil), 10)
	slog.New(s.Logs).Warn("bitvoice: refused", "url", "https://bv.example.gov:8443/api?token=hunter2hunter2", "err", "user ops@county.example.gov token bv_0123456789abcdef")
	s.mu.Lock()
	s.integ.LIS = LIS{On: true, Name: "State LIS", URL: "https://lis.example.gov/held", Token: "lis-secret-value-9", Password: "pw-secret-value-9"}
	s.integ.Paging.Gateways = []paging.Gateway{{ID: "g1", Name: "Volunteers", Kind: "webhook", URL: "https://pager.example.gov/hook", Token: "pager-secret-value-9"}}
	s.mu.Unlock()
	st := cad.DefaultSettings()
	s.Eng.Read(func(x *cad.State) { st = x.Settings })
	st.SupportLine, st.Agency = "540-555-0100", "Grid County 911"
	if code, b := a.do("PUT", "/api/admin/settings", st, true); code != 204 {
		t.Fatalf("settings: %d %s", code, b)
	}
	a.do("POST", "/api/admin/golive", nil, true)
	if code, _ := login(t, ts, "d1", pw["d1"]).do("POST", "/api/admin/support/bundle", nil, true); code != 403 {
		t.Fatalf("a dispatcher downloaded a bundle: %d", code)
	}
	code, b := a.do("POST", "/api/admin/support/bundle", nil, true)
	if code != 200 {
		t.Fatalf("bundle: %d %s", code, b)
	}
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range z.File {
		r, _ := f.Open()
		data, _ := io.ReadAll(r)
		r.Close()
		files[f.Name] = string(data)
	}
	for _, want := range []string{"README.txt", "system.json", "setup.json", "golive.json", "health.json", "counts.json", "settings.json", "integrations.json", "config.json", "log.txt"} {
		if _, ok := files[want]; !ok {
			t.Errorf("no %s in %v", want, files)
		}
	}
	for name, data := range files {
		if name == "README.txt" {
			continue
		}
		for _, bad := range []string{"lis-secret", "pw-secret", "pager-secret", "hunter2", "bv_0123", "ops@county", "540-555-0100", "zeldaroot", "Zelda", pw["admin"]} {
			if strings.Contains(data, bad) {
				t.Errorf("%s holds %q", name, bad)
			}
		}
	}
	if !strings.Contains(files["integrations.json"], `"(set)"`) || !strings.Contains(files["integrations.json"], "lis.example.gov") {
		t.Errorf("integrations: %s", files["integrations.json"])
	}
	if !strings.Contains(files["log.txt"], "bitvoice: refused") || !strings.Contains(files["config.json"], `"E1"`) || !strings.Contains(files["settings.json"], "Grid County 911") {
		t.Errorf("log %q", files["log.txt"])
	}
	if !strings.Contains(files["counts.json"], `"admin": 2`) {
		t.Errorf("counts: %s", files["counts.json"])
	}
	if !strings.Contains(logged.String(), "hunter2") {
		t.Error("the ring did not pass the record on")
	}
	evs, _ := s.Eng.Events(0, 100000, nil)
	found := false
	for _, e := range evs {
		if e.Type == cad.EvSupportBundle && e.Actor == "admin" {
			found = true
		}
	}
	if !found {
		t.Error("the download was not recorded")
	}
}

func TestRedactText(t *testing.T) {
	for in, want := range map[string]string{
		"time=2026-10-25T16:20:01.123-05:00 lat=37.2710 lon=-79.9414":  "time=2026-10-25T16:20:01.123-05:00 lat=37.2710 lon=-79.9414",
		"call from (540) 555-0100 and +15405550100":                    "call from (number) and (number)",
		"GET https://u:p@x.example/a?b=c failed":                       "GET https://x.example/a?… failed",
		"path=/opt/bitdispatcher/data/map/map-12345678901234567890.db": "path=/opt/bitdispatcher/data/map/map-12345678901234567890.db",
	} {
		if got := redactText(in); got != want {
			t.Errorf("redactText(%q) = %q, want %q", in, got, want)
		}
	}
}
