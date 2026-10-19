package api

import (
	"bufio"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/bitvoice"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

type client struct {
	t    *testing.T
	base string
	c    *http.Client
}

func (c *client) do(method, path string, body any, csrf bool) (int, []byte) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	if csrf {
		req.Header.Set(csrfHeader, "1")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.c.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func newServer(t *testing.T) (*httptest.Server, *Server, map[string]string) {
	t.Helper()
	dir := t.TempDir()
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
	s, err := New(&Server{Eng: eng, Auth: users, BV: bitvoice.New(eng, log), Log: log, DataDir: dir, Version: "test",
		Tiles: NewTileCache(dir+"/tiles", "", "test")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	ts := httptest.NewTLSServer(s)
	t.Cleanup(ts.Close)
	return ts, s, pw
}

func login(t *testing.T, ts *httptest.Server, user, pw string) *client {
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{Transport: ts.Client().Transport, Jar: jar}
	c := &client{t: t, base: ts.URL, c: hc}
	if code, b := c.do("POST", "/api/auth/login", map[string]string{"username": user, "password": pw, "console": "D1"}, true); code != 200 {
		t.Fatalf("login %s: %d %s", user, code, b)
	}
	return c
}

func TestAPIFlow(t *testing.T) {
	ts, s, pw := newServer(t)
	anon := &client{t: t, base: ts.URL, c: ts.Client()}
	if code, _ := anon.do("GET", "/api/state", nil, false); code != 401 {
		t.Fatalf("anonymous state: %d", code)
	}
	if code, b := anon.do("GET", "/api/auth/me", nil, false); code != 200 || !strings.Contains(string(b), `"signedIn":false`) {
		t.Fatalf("me signed out: %d %s", code, b)
	}
	admin := login(t, ts, "admin", pw["admin"])
	if code, b := admin.do("PUT", "/api/admin/stations/st1", cad.Station{Name: "Station 1", Lat: 40.27, Lon: -76.88}, true); code != 204 {
		t.Fatalf("station: %d %s", code, b)
	}
	if code, b := admin.do("PUT", "/api/admin/units/m1", cad.UnitDef{Type: "medic", Station: "ST1"}, true); code != 204 {
		t.Fatalf("unit: %d %s", code, b)
	}
	// Writes without the CSRF header are refused.
	if code, _ := admin.do("PUT", "/api/admin/units/m2", cad.UnitDef{Type: "medic"}, false); code != 403 {
		t.Fatalf("no csrf: %d", code)
	}
	viewer := login(t, ts, "v1", pw["v1"])
	if code, _ := viewer.do("POST", "/api/incidents", cad.NewIncident{Type: "CHEST"}, true); code != 403 {
		t.Fatalf("viewer created an incident: %d", code)
	}
	d := login(t, ts, "d1", pw["d1"])
	if code, _ := d.do("PUT", "/api/admin/units/m3", cad.UnitDef{Type: "medic"}, true); code != 403 {
		t.Fatalf("dispatcher changed configuration: %d", code)
	}
	// The stream tells the dispatcher's screen about changes.
	req, _ := http.NewRequest("GET", ts.URL+"/api/stream", nil)
	resp, err := d.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	lines := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	if code, b := d.do("POST", "/api/command", map[string]string{"line": "AV M1"}, true); code != 200 || !strings.Contains(string(b), `"ok":true`) {
		t.Fatalf("command: %d %s", code, b)
	}
	lat, lon := 40.2633, -76.8823
	code, b := d.do("POST", "/api/incidents", map[string]any{"type": "chest", "location": map[string]any{"lat": lat, "lon": lon, "address": "123 N Front St"}}, true)
	if code != 201 {
		t.Fatalf("create: %d %s", code, b)
	}
	var created struct{ Number string }
	json.Unmarshal(b, &created)
	code, b = d.do("GET", "/api/incidents/"+created.Number+"/recommend", nil, false)
	var rec cad.Recommendation
	json.Unmarshal(b, &rec)
	if code != 200 || len(rec.Units) != 1 || rec.Units[0] != "M1" {
		t.Fatalf("recommend: %d %s", code, b)
	}
	if code, b := d.do("POST", "/api/incidents/"+created.Number+"/dispatch", map[string]any{"units": rec.Units, "recommended": rec.Units}, true); code != 204 {
		t.Fatalf("dispatch: %d %s", code, b)
	}
	deadline := time.After(3 * time.Second)
	sawDispatch := false
	for !sawDispatch {
		select {
		case l := <-lines:
			sawDispatch = strings.Contains(l, cad.EvIncDispatch)
		case <-deadline:
			t.Fatal("no change event for the dispatch")
		}
	}
	code, b = d.do("GET", "/api/state", nil, false)
	var v View
	json.Unmarshal(b, &v)
	if code != 200 || len(v.Incidents) != 1 || v.Incidents[0].Status != cad.Active || v.Units[0].Status != cad.Dispatched || len(v.Units[0].Caps) == 0 {
		t.Fatalf("state: %d %+v", code, v.Incidents)
	}
	// Supervisors can read the ledger; it verifies.
	code, b = admin.do("GET", "/api/ledger/verify", nil, false)
	if code != 200 || !strings.Contains(string(b), `"ok":true`) {
		t.Fatalf("verify: %d %s", code, b)
	}
	code, b = admin.do("GET", "/api/ledger?type=session.", nil, false)
	if code != 200 || !strings.Contains(string(b), cad.EvSignIn) {
		t.Fatalf("ledger: %d %s", code, b)
	}
	// Integrations: secrets are write-only.
	code, b = admin.do("PUT", "/api/admin/integrations", map[string]string{"bitvoiceUrl": "https://bv.example:8443", "bitvoiceToken": "bv_secret", "eidoToken": "eido-1"}, true)
	if code != 200 || strings.Contains(string(b), "bv_secret") || !strings.Contains(string(b), `"bitvoiceTokenSet":true`) {
		t.Fatalf("integrations: %d %s", code, b)
	}
	fi, _ := os.Stat(s.integrationsPath())
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("integrations.json mode %v", fi.Mode())
	}
	// The EIDO webhook takes BitVoice's EIDO with the token.
	eido, _ := os.ReadFile("../ng911/testdata/eido-bitvoice.json")
	req, _ = http.NewRequest("POST", ts.URL+"/api/ng911/eido", strings.NewReader(string(eido)))
	req.Header.Set("Authorization", "Bearer eido-1")
	resp2, err := ts.Client().Do(req)
	if err != nil || resp2.StatusCode != 204 {
		t.Fatalf("eido webhook: %v %v", err, resp2.Status)
	}
	code, b = d.do("GET", "/api/state", nil, false)
	if !strings.Contains(string(b), `"abc123"`) {
		t.Fatalf("the EIDO call is not in the state: %s", b)
	}
}

func TestALISpillBecomesCall(t *testing.T) {
	ts, s, pw := newServer(t)
	admin := login(t, ts, "admin", pw["admin"])
	if code, b := admin.do("PUT", "/api/admin/integrations", map[string]string{"aliListen": "127.0.0.1:0"}, true); code != 200 {
		t.Fatalf("ali: %d %s", code, b)
	}
	if code, b := admin.do("POST", "/api/admin/integrations/ali/test", nil, true); code != 200 {
		t.Fatalf("ali test: %d %s", code, b)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		found := false
		s.Eng.Read(func(st *cad.State) {
			for _, c := range st.Calls {
				found = found || c.Callback == "+15555550100"
			}
		})
		if found {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the test spill did not become a call")
}
