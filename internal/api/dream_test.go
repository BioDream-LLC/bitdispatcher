package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

func hookPost(t *testing.T, ts *httptest.Server, path, token string, body any) (int, []byte) {
	t.Helper()
	var rd io.Reader
	switch v := body.(type) {
	case string:
		rd = strings.NewReader(v)
	default:
		b, _ := json.Marshal(v)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest("POST", ts.URL+path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// The hooks, translation and replies, screening, care, breaks, the warning, the workload board, the quality
// review and the drone request, end to end over the API.
func TestDreamAPI(t *testing.T) {
	ts, s, pw := newServer(t)
	admin := login(t, ts, "admin", pw["admin"])
	d := login(t, ts, "d1", pw["d1"])
	if code, _ := hookPost(t, ts, "/api/hooks/acn", "", "{}"); code != 503 {
		t.Fatalf("hooks with no token: %d", code)
	}
	var drones atomic.Int32
	droneSys := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer dt" {
			w.WriteHeader(401)
			return
		}
		drones.Add(1)
		w.Write([]byte(`{"detail":"launching","drone":"Skydio 2"}`))
	}))
	defer droneSys.Close()
	if code, b := admin.do("PUT", "/api/admin/connections", map[string]any{"hookToken": "ht", "droneUrl": "http://127.0.0.1:1/x", "droneToken": "dt"}, true); code != 200 || !strings.Contains(string(b), `"hookTokenSet":true`) || strings.Contains(string(b), `"ht"`) {
		t.Fatalf("connections: %d %s", code, b)
	}
	// The test drone system is http on a random port of 127.0.0.1: allowed for a program on this server.
	if code, b := admin.do("PUT", "/api/admin/connections", map[string]any{"droneUrl": "http://127.0.0.1" + strings.TrimPrefix(droneSys.URL, "http://127.0.0.1")}, true); code != 200 {
		t.Fatalf("drone url: %d %s", code, b)
	}
	if code, _ := hookPost(t, ts, "/api/hooks/acn", "wrong", "{}"); code != 401 {
		t.Fatalf("wrong token: %d", code)
	}
	code, b := hookPost(t, ts, "/api/hooks/acn", "ht", `{"delta_v_mph": 38, "pdof": 0, "rollover": true, "latitude": 40.27, "longitude": -76.88, "vehicle_make": "Ford", "number_of_occupants": 2}`)
	if code != 200 || !strings.Contains(string(b), `"risk":"high"`) {
		t.Fatalf("acn: %d %s", code, b)
	}
	var acnCall struct {
		Call string `json:"call"`
	}
	json.Unmarshal(b, &acnCall)
	s.Eng.UpsertCall(cad.BitVoiceActor, "v1", cad.CallData{Source: "bitvoice", State: "answered", Callback: "+15405550100", Agent: "d1", Answer: time.Now()})
	if code, b := hookPost(t, ts, "/api/hooks/transcript", "ht", map[string]any{"call": "v1", "speaker": "caller", "text": "Mi esposo no respira, por favor"}); code != 204 {
		t.Fatalf("transcript: %d %s", code, b)
	}
	if code, _ := hookPost(t, ts, "/api/hooks/transcript", "ht", map[string]any{"call": "nope", "text": "x"}); code != 404 {
		t.Fatalf("transcript for an unknown call: %d", code)
	}
	code, b = d.do("POST", "/api/calls/v1/reply", map[string]any{"text": "Help is on the way"}, true)
	if code != 200 || !strings.Contains(string(b), "La ayuda va en camino") {
		t.Fatalf("reply: %d %s", code, b)
	}
	if code, b := d.do("POST", "/api/translate", map[string]any{"text": "¿Hay alguien adentro?"}, true); code != 200 || !strings.Contains(string(b), "Is anyone inside?") {
		t.Fatalf("translate: %d %s", code, b)
	}
	if code, _ := d.do("POST", "/api/calls/v1/screen", map[string]any{"emergency": true}, true); code != 204 {
		t.Fatalf("screen: %d", code)
	}
	// An incident from the call: a critical call, so the supervisor's care list has it.
	code, b = d.do("POST", "/api/incidents", map[string]any{"type": "card", "call": "v1", "location": map[string]any{"lat": 40.27, "lon": -76.88, "address": "1 Main St"}}, true)
	if code != 200 && code != 201 {
		t.Fatalf("incident: %d %s", code, b)
	}
	var inc struct {
		Number string `json:"number"`
	}
	json.Unmarshal(b, &inc)
	if code, b := d.do("POST", "/api/calls/attach-all", map[string]any{"calls": []string{acnCall.Call}, "incident": inc.Number}, true); code != 200 || !strings.Contains(string(b), `"attached":1`) {
		t.Fatalf("attach all: %d %s", code, b)
	}
	if code, b := d.do("POST", "/api/incidents/"+inc.Number+"/drone", nil, true); code != 200 || !strings.Contains(string(b), "launching (Skydio 2)") || drones.Load() != 1 {
		t.Fatalf("drone: %d %s", code, b)
	}
	if code, _ := hookPost(t, ts, "/api/hooks/drone", "ht", map[string]any{"id": "SK2", "name": "Skydio 2", "status": "onscene", "incident": inc.Number, "lat": 40.27, "lon": -76.88, "videoUrl": "https://drone.example/live"}); code != 204 {
		t.Fatalf("drone hook: %d", code)
	}
	if code, _ := hookPost(t, ts, "/api/hooks/radio-text", "ht", map[string]any{"unit": "M1", "text": "on scene"}); code != 204 {
		t.Fatalf("radio text: %d", code)
	}
	code, b = d.do("GET", "/api/state", nil, false)
	for _, want := range []string{`"care":[{"incident":"` + inc.Number, `"summaries":{"` + inc.Number, `"drones":[{"id":"SK2"`, `"crashRisk":{"risk":"high"`, `"english":"`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("state lacks %s", want)
		}
	}
	sup := login(t, ts, "admin", pw["admin"])
	if code, _ := d.do("POST", "/api/care/"+inc.Number, map[string]any{"action": "break", "user": "d1"}, true); code != 403 {
		t.Fatalf("a dispatcher used the care list: %d", code)
	}
	if code, b := sup.do("POST", "/api/care/"+inc.Number, map[string]any{"action": "break", "user": "d1"}, true); code != 204 {
		t.Fatalf("care: %d %s", code, b)
	}
	if code, _ := d.do("POST", "/api/me/break", map[string]any{"on": true}, true); code != 204 {
		t.Fatalf("break: %d", code)
	}
	code, b = sup.do("GET", "/api/workload", nil, false)
	if code != 200 || !strings.Contains(string(b), `"username":"d1"`) || !strings.Contains(string(b), `"onBreak":true`) {
		t.Fatalf("workload: %d %s", code, b)
	}
	from, to := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339), time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	code, b = sup.do("GET", "/api/reports/quality?from="+from+"&to="+to, nil, false)
	if code != 200 || !strings.Contains(string(b), `"incident":"`+inc.Number) || !strings.Contains(string(b), `"takers":[{"taker":"d1"`) {
		t.Fatalf("quality: %d %s", code, b)
	}
	if code, _ := sup.do("POST", "/api/tdos/ack", nil, true); code != 422 {
		t.Fatalf("ack with no warning: %d", code)
	}
}

// A session signed in to train works on a board of its own: scenarios play into it, the live board never
// sees them, administration is closed, and a reset starts it again.
func TestTrainingEnvironment(t *testing.T) {
	ts, s, pw := newServer(t)
	s.Eng.SaveUnit(cad.SystemActor, cad.UnitDef{ID: "E1", Type: "engine"})
	s.Eng.SetStatus(cad.SystemActor, "E1", cad.Available, "", "")
	jar, _ := cookiejar.New(nil)
	tr := &client{t: t, base: ts.URL, c: &http.Client{Transport: ts.Client().Transport, Jar: jar}}
	if code, b := tr.do("POST", "/api/auth/login", map[string]any{"username": "d1", "password": pw["d1"], "console": "T1", "training": true}, true); code != 200 || !strings.Contains(string(b), `"training":true`) {
		t.Fatalf("training sign-in: %d %s", code, b)
	}
	live := login(t, ts, "d1", pw["d1"])
	code, b := tr.do("GET", "/api/state", nil, false)
	if code != 200 || !strings.Contains(string(b), `"training":{`) || !strings.Contains(string(b), `"id":"E1"`) {
		t.Fatalf("training state: %d %s", code, b[:min(len(b), 300)])
	}
	if code, b := tr.do("POST", "/api/training/start", map[string]string{"id": "robbery"}, true); code != 200 || !strings.Contains(string(b), `"scenario":"robbery"`) {
		t.Fatalf("start: %d %s", code, b)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, b = tr.do("GET", "/api/state", nil, false)
		if strings.Contains(string(b), "t-rob-1") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the scenario's call never arrived")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, b := live.do("GET", "/api/state", nil, false); strings.Contains(string(b), "t-rob-1") || strings.Contains(string(b), `"training":{`) {
		t.Fatal("the training call reached the live board")
	}
	// A second trainee on the same account has a board of their own: no scenario, and their reset leaves
	// the first trainee's board alone.
	jar2, _ := cookiejar.New(nil)
	tr2 := &client{t: t, base: ts.URL, c: &http.Client{Transport: ts.Client().Transport, Jar: jar2}}
	if code, b := tr2.do("POST", "/api/auth/login", map[string]any{"username": "d1", "password": pw["d1"], "console": "T2", "training": true}, true); code != 200 {
		t.Fatalf("second training sign-in: %d %s", code, b)
	}
	if _, b := tr2.do("GET", "/api/state", nil, false); strings.Contains(string(b), "t-rob-1") || !strings.Contains(string(b), `"training":{`) {
		t.Fatal("the second trainee sees the first one's board")
	}
	if code, _ := tr2.do("POST", "/api/training/reset", nil, true); code != 204 {
		t.Fatalf("second reset: %d", code)
	}
	if _, b := tr.do("GET", "/api/state", nil, false); !strings.Contains(string(b), "t-rob-1") {
		t.Fatal("the second trainee's reset cleared the first one's board")
	}
	if code, _ := tr.do("PUT", "/api/admin/settings", cad.DefaultSettings(), true); code != 403 {
		t.Fatalf("administration in training: %d", code)
	}
	if code, b := tr.do("GET", "/api/training/score", nil, false); code != 200 || !strings.Contains(string(b), `"name":"Call type"`) {
		t.Fatalf("score: %d %s", code, b)
	}
	if code, _ := live.do("GET", "/api/training/score", nil, false); code != 404 && code != 405 {
		t.Fatalf("training routes on the live server: %d", code)
	}
	if code, _ := tr.do("POST", "/api/training/reset", nil, true); code != 204 {
		t.Fatalf("reset: %d", code)
	}
	if _, b := tr.do("GET", "/api/state", nil, false); strings.Contains(string(b), "t-rob-1") {
		t.Fatal("the reset kept the scenario's call")
	}
	_ = auth.Admin
}
