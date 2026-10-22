package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/biodream-llc/bitdispatcher/internal/bitvoice"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// fakeBV answers the BitVoice calls behind evidence, video reveal, records and take, and remembers
// who each request said it was for (X-BitVoice-On-Behalf-Of).
type fakeBV struct {
	mu     sync.Mutex
	paths  []string
	behalf []string
	held   bool
}

var photo = []byte("\xff\xd8\xff\xe0 a caller's photo")

func (f *fakeBV) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	note := func(r *http.Request) {
		f.mu.Lock()
		f.paths = append(f.paths, r.Method+" "+r.URL.Path)
		f.behalf = append(f.behalf, r.Header.Get("X-BitVoice-On-Behalf-Of"))
		f.mu.Unlock()
	}
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer bv_test" {
				http.Error(w, `{"message":"sign in"}`, 401)
				return
			}
			note(r)
			h(w, r)
		}
	}
	mux.HandleFunc("GET /api/calls/{id}/media/{n}", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("n") != "1" {
			http.Error(w, `{"message":"no such item"}`, 404)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(photo)
	}))
	mux.HandleFunc("GET /api/calls/{id}/media", auth(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		held := f.held
		f.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"held": held, "items": []any{map[string]any{"n": 1, "name": "p.jpg", "type": "image/jpeg", "kind": "photo", "size": len(photo), "sha256": "ab12"}}})
	}))
	mux.HandleFunc("POST /api/calls/{id}/media/hold", auth(func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Hold bool }
		json.NewDecoder(r.Body).Decode(&b)
		f.mu.Lock()
		f.held = b.Hold
		f.mu.Unlock()
		w.Write([]byte(`{}`))
	}))
	mux.HandleFunc("POST /api/calls/{id}/video/reveal", auth(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{}`)) }))
	mux.HandleFunc("GET /api/psap/report", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"calls":3,"answered":3,"target":15,"withinTarget":100,"from":"` + r.URL.Query().Get("from") + `"}`))
	}))
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestBitVoiceEvidenceAndRecords(t *testing.T) {
	ts, s, pw := newServer(t)
	f := &fakeBV{}
	srv := f.server(t)
	sum := sha256.Sum256(srv.Certificate().Raw)
	s.BV.Configure(bitvoice.Config{URL: srv.URL, Token: "bv_test", Fingerprint: hex.EncodeToString(sum[:])})
	s.Eng.UpsertCall(cad.BitVoiceActor, "c1", cad.CallData{Source: "bitvoice", Callback: "+15405550100",
		Handling: &cad.Handling{Evidence: &cad.Evidence{Items: []cad.EvidenceItem{{N: 1, Name: "p.jpg", Type: "image/jpeg", Kind: "photo", SHA256: "ab12"}}}}})

	d := login(t, ts, "d1", pw["d1"])
	v := login(t, ts, "v1", pw["v1"])
	admin := login(t, ts, "admin", pw["admin"])

	// A viewer cannot reveal; a dispatcher can, and gets the bytes in a sandbox.
	if code, _ := v.do("GET", "/api/calls/c1/evidence/1", nil, false); code != 403 {
		t.Fatalf("viewer reveal: %d", code)
	}
	req, _ := http.NewRequest("GET", ts.URL+"/api/calls/c1/evidence/1", nil)
	resp, err := d.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/jpeg" || !strings.Contains(resp.Header.Get("Content-Security-Policy"), "sandbox") ||
		resp.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("reveal: %d %v", resp.StatusCode, resp.Header)
	}
	// An item the call does not have is not fetched.
	if code, _ := d.do("GET", "/api/calls/c1/evidence/7", nil, false); code != 404 {
		t.Fatalf("unknown item: %d", code)
	}
	// Hold, and live video reveal.
	if code, b := d.do("POST", "/api/calls/c1/evidence/hold", map[string]bool{"hold": true}, true); code != 204 {
		t.Fatalf("hold: %d %s", code, b)
	}
	var held bool
	s.Eng.Read(func(st *cad.State) { held = st.Calls["c1"].Handling.Evidence.Held })
	if !held {
		t.Fatal("the hold is not on the call")
	}
	if code, b := d.do("POST", "/api/calls/c1/video/reveal", map[string]any{}, true); code != 204 {
		t.Fatalf("video reveal: %d %s", code, b)
	}
	// Each look is in the ledger with the SHA-256 and who looked.
	code, b := admin.do("GET", "/api/ledger?type=call.action", nil, false)
	if code != 200 || !strings.Contains(string(b), "evidence revealed") || !strings.Contains(string(b), "SHA-256 ab12") ||
		!strings.Contains(string(b), "evidence held") || !strings.Contains(string(b), "live video revealed") || !strings.Contains(string(b), `"d1"`) {
		t.Fatalf("ledger: %d %s", code, b)
	}
	// BitVoice was told who each was for.
	f.mu.Lock()
	if len(f.paths) < 4 {
		t.Errorf("BitVoice saw only %v", f.paths)
	}
	for i, p := range f.paths {
		if f.behalf[i] != "d1" {
			t.Errorf("%s on behalf of %q", p, f.behalf[i])
		}
	}
	f.mu.Unlock()

	// Records: supervisors only; the query passes through, unknown kinds do not.
	if code, _ := d.do("GET", "/api/bitvoice/records/answer", nil, false); code != 403 {
		t.Fatalf("dispatcher records: %d", code)
	}
	if code, b := admin.do("GET", "/api/bitvoice/records/answer?from=2026-10-01&evil=1", nil, false); code != 200 || !strings.Contains(string(b), `"from":"2026-10-01"`) {
		t.Fatalf("records: %d %s", code, b)
	}
	if code, _ := admin.do("GET", "/api/bitvoice/records/users", nil, false); code != 404 {
		t.Fatalf("unknown record: %d", code)
	}
}

// The plain-HTTP hooks listener offers the push endpoints and nothing else.
func TestHooksHandlerOffersOnlyPushes(t *testing.T) {
	_, s, _ := newServer(t)
	h := httptest.NewServer(s.HooksHandler())
	defer h.Close()
	for _, p := range []string{"/api/state", "/api/auth/login", "/", "/api/admin/integrations"} {
		resp, err := http.Get(h.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 404 && resp.StatusCode != 405 {
			t.Errorf("%s: %d", p, resp.StatusCode)
		}
	}
	// Pushes still need their token.
	resp, err := http.Post(h.URL+"/api/ng911/eido", "application/emergency.eido+json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 && resp.StatusCode != 403 && resp.StatusCode != 503 {
		t.Fatalf("an EIDO without a token: %d", resp.StatusCode)
	}
}

// A second BitVoice: added from Integrations, its calls' actions go to it (not the main one), its
// pushes are told apart by their secrets, and it shows on the status screens.
func TestMoreBitVoice(t *testing.T) {
	ts, s, pw := newServer(t)
	mainBV, otherBV := &fakeBV{}, &fakeBV{}
	ms, os2 := mainBV.server(t), otherBV.server(t)
	pin := func(srv *httptest.Server) string {
		sum := sha256.Sum256(srv.Certificate().Raw)
		return hex.EncodeToString(sum[:])
	}
	admin := login(t, ts, "admin", pw["admin"])
	if code, b := admin.do("PUT", "/api/admin/integrations", map[string]any{"bitvoiceUrl": ms.URL, "bitvoiceFingerprint": pin(ms),
		"bitvoiceToken": "bv_test", "eidoToken": "bd_e1"}, true); code != 200 {
		t.Fatalf("main: %d %s", code, b)
	}
	// Its secrets must differ from the main one's.
	if code, _ := admin.do("POST", "/api/admin/integrations/more", map[string]any{"name": "Neighbour 911", "url": os2.URL, "eidoToken": "bd_e1"}, true); code != 422 {
		t.Fatalf("a shared EIDO token: %d", code)
	}
	code, b := admin.do("POST", "/api/admin/integrations/more", map[string]any{"name": "Neighbour 911", "url": os2.URL, "fingerprint": pin(os2),
		"token": "bv_test", "eidoToken": "bd_e2", "radioSecret": "bd_r2"}, true)
	var out struct {
		ID   string     `json:"id"`
		More []MoreView `json:"more"`
	}
	if json.Unmarshal(b, &out); code != 200 || out.ID == "" || len(out.More) != 1 || !out.More[0].Token || out.More[0].Name != "Neighbour 911" {
		t.Fatalf("add: %d %s", code, b)
	}
	if strings.Contains(string(b), "bd_e2") || strings.Contains(string(b), "bv_test") {
		t.Fatal("a secret came back")
	}
	// A call on the second BitVoice: holding its evidence goes there.
	s.Eng.UpsertCall(cad.BitVoiceActor, "n1", cad.CallData{Source: "bitvoice", System: out.ID, Callback: "+15405550101"})
	d := login(t, ts, "d1", pw["d1"])
	if code, b := d.do("POST", "/api/calls/n1/evidence/hold", map[string]bool{"hold": true}, true); code != 204 {
		t.Fatalf("hold: %d %s", code, b)
	}
	otherBV.mu.Lock()
	got := strings.Join(otherBV.paths, ",")
	otherBV.mu.Unlock()
	mainBV.mu.Lock()
	gotMain := len(mainBV.paths)
	mainBV.mu.Unlock()
	if !strings.Contains(got, "POST /api/calls/n1/media/hold") || gotMain != 0 {
		t.Fatalf("the second BitVoice saw %q, the main one %d requests", got, gotMain)
	}
	// Pushes: each secret is taken (the empty EIDO is then refused as an EIDO), others are not.
	push := func(path, secret string) int {
		req, _ := http.NewRequest("POST", ts.URL+path, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+secret)
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if c1, c2, c3, c4 := push("/api/ng911/eido", "bd_e1"), push("/api/ng911/eido", "bd_e2"), push("/api/ng911/eido", "nope"), push("/api/radio/events", "bd_r2"); c1 != 400 || c2 != 400 || c3 != 401 || c4 != 400 {
		t.Fatalf("pushes: main %d, second %d, wrong %d, radio %d", c1, c2, c3, c4)
	}
	if h := s.health(); len(h.More) != 1 || h.More[0].Name != "Neighbour 911" || !h.More[0].Health.Configured {
		t.Fatalf("health: %+v", h.More)
	}
	// Removed: its calls' actions no longer go anywhere.
	if code, _ := admin.do("DELETE", "/api/admin/integrations/more/"+out.ID, nil, true); code != 204 {
		t.Fatalf("remove: %d", code)
	}
	if code, _ := d.do("POST", "/api/calls/n1/evidence/hold", map[string]bool{"hold": false}, true); code == 204 {
		t.Fatal("an action on a removed system's call")
	}
	if len(s.health().More) != 0 {
		t.Fatal("still in health")
	}
}
