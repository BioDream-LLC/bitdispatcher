package bitvoice

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// fakeBitVoice serves the parts of BitVoice's API a CAD uses, with BitVoice's payload shapes.
type fakeBitVoice struct {
	mu      sync.Mutex
	events  chan string
	calls   []map[string]any
	acked   []string
	rebids  int
	texts   []string
	eido    []byte
	authBad int
	behalf  []string
}

func (f *fakeBitVoice) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer bv_test" {
				f.mu.Lock()
				f.authBad++
				f.mu.Unlock()
				http.Error(w, `{"message":"sign in"}`, 401)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("GET /api/events", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		fmt.Fprint(w, "event: hello\ndata: {}\n\n")
		fl.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case ev := <-f.events:
				fmt.Fprint(w, ev)
				fl.Flush()
			}
		}
	}))
	mux.HandleFunc("GET /api/calls", auth(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"items": f.calls})
	}))
	mux.HandleFunc("GET /api/radio/locations", auth(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"items": []any{map[string]any{"unit": 101, "lat": 40.27, "lon": -76.88, "at": time.Now()}}})
	}))
	mux.HandleFunc("GET /api/calls/{id}/eido", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/emergency.eido+json")
		w.Write(f.eido)
	}))
	mux.HandleFunc("POST /api/calls/{id}/rebid", auth(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.rebids++
		f.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"location": map[string]any{"lat": 40.2640, "lon": -76.8830, "radius": 12, "method": "GPS"}})
	}))
	mux.HandleFunc("POST /api/calls/{id}/text", auth(func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Via, Text string }
		json.NewDecoder(r.Body).Decode(&b)
		f.mu.Lock()
		f.texts = append(f.texts, b.Text)
		f.mu.Unlock()
		w.Write([]byte(`{}`))
	}))
	mux.HandleFunc("POST /api/radio/channels/{id}/ack", auth(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.acked = append(f.acked, r.PathValue("id"))
		f.mu.Unlock()
		w.Write([]byte(`{}`))
	}))
	mux.HandleFunc("GET /api/calls/{id}/media", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"call":"x","held":false,"items":[{"n":1,"name":"p.jpg","type":"image/jpeg","kind":"photo","size":2280,"sha256":"d81a","received":"2026-10-21T09:02:27Z","takenLocal":"2026-10-15 14:05:30","hasGps":true,"lat":40.2633,"lon":-76.8823}]}`))
	}))
	mux.HandleFunc("POST /api/core/validate", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"invalid":["RD"],"matched":"","position":{"checked":false,"within":false,"why":"no point"},"suggestions":[{"civic":{"HNO":"215","RD":"Church","STS":"Ave","POD":"SW","A3":"Roanoke","A1":"VA"},"why":"a street of a similar name"}]}`))
	}))
	mux.HandleFunc("POST /api/calls/{id}/take", auth(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.behalf = append(f.behalf, r.Header.Get("X-BitVoice-On-Behalf-Of"))
		f.mu.Unlock()
		w.Write([]byte(`{}`))
	}))
	mux.HandleFunc("GET /api/core/states", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[{"uri":"sip:sos@east","name":"East","queue":"Full","length":8,"max":8,"service":"Normal"}]}`))
	}))
	return mux
}

func sse(typ string, data any) string {
	b, _ := json.Marshal(map[string]any{"seq": 1, "at": time.Now(), "type": typ, "data": data})
	return "event: " + typ + "\ndata: " + string(b) + "\n\n"
}

func setup(t *testing.T) (*cad.Engine, *Bridge, *fakeBitVoice, *httptest.Server) {
	t.Helper()
	eng, err := cad.Open(filepath.Join(t.TempDir(), "ledger.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { eng.Close() })
	eng.SaveStation(cad.SystemActor, cad.Station{ID: "ST1", Name: "Station 1", Lat: 40.27, Lon: -76.88})
	eng.SaveUnit(cad.SystemActor, cad.UnitDef{ID: "M1", Type: "medic", Station: "ST1", RadioID: 101})
	eng.SaveUnit(cad.SystemActor, cad.UnitDef{ID: "P1", Type: "patrol", RadioID: 197})
	eido, err := os.ReadFile("../ng911/testdata/eido-bitvoice.json")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeBitVoice{events: make(chan string, 16), eido: eido}
	srv := httptest.NewTLSServer(f.handler(t))
	t.Cleanup(srv.Close)
	sum := sha256.Sum256(srv.Certificate().Raw)
	b := New(eng, slog.New(slog.NewTextHandler(io.Discard, nil)))
	b.Configure(Config{URL: srv.URL, Token: "bv_test", Fingerprint: hex.EncodeToString(sum[:]), EIDOToken: "eido-secret", RadioSecret: "cad-shared-secret"})
	return eng, b, f, srv
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestStreamCallsEIDOAndRadio(t *testing.T) {
	eng, b, f, _ := setup(t)
	// A ringing 911 call is already in progress when bitdispatcher connects.
	f.calls = []map[string]any{{"id": "ring1", "state": "ringing", "from": "+17175550199", "start": time.Now(),
		"emergency": map[string]any{"location": map[string]any{"lat": 40.25, "lon": -76.87, "radius": 50, "method": "Cell"}, "locationUri": "https://lis.example/x",
			"additional": map[string]any{"service": map[string]any{"types": []string{"wireless"}}}, "esinet": true}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go b.Run(ctx)
	waitFor(t, "the ringing call", func() bool {
		var ok bool
		eng.Read(func(s *cad.State) {
			c := s.Calls["ring1"]
			ok = c != nil && c.State == "ringing" && c.Class == "WRLS" && c.CanRebid && c.Location.Radius == 50
		})
		return ok
	})
	waitFor(t, "the radio position", func() bool {
		var ok bool
		eng.Read(func(s *cad.State) { ok = s.Units["M1"].Position != nil })
		return ok
	})
	// The call is answered: BitVoice says so, bitdispatcher pulls the EIDO.
	f.events <- sse("emergency.answered", map[string]any{"call": "abc123", "agent": "jlee", "from": "+17175550100"})
	waitFor(t, "the pulled EIDO", func() bool {
		var ok bool
		eng.Read(func(s *cad.State) {
			c := s.Calls["abc123"]
			ok = c != nil && c.Callback == "+17175550100" && c.CallerName == "Jordan Example" && len(c.Media) == 2 && c.Video != nil
		})
		return ok
	})
	// An emergency button on the radio, then BitVoice's console acknowledges it.
	f.events <- sse("radio.emergency", map[string]any{"channel": "tg", "channelName": "County fire", "unit": "bee07.ad5.0000c5", "unitId": 197, "emergency": true})
	waitFor(t, "the emergency", func() bool {
		var ok bool
		eng.Read(func(s *cad.State) { ok = s.Units["P1"].Emergency != "" })
		return ok
	})
	f.events <- sse("radio.emergency.ack", map[string]any{"channel": "tg", "unit": "bee07.ad5.0000c5", "by": "console1"})
	waitFor(t, "the acknowledgement", func() bool {
		var ok bool
		eng.Read(func(s *cad.State) {
			for _, em := range s.Emergencies {
				ok = em.AckedBy == "console1" && em.AckedWhere == "bitvoice"
			}
		})
		return ok
	})
	// BitVoice reports it could not deliver the ALI spill.
	f.events <- sse("emergency.cad_failed", map[string]any{"call": "abc123", "what": "ALI", "error": "connection refused"})
	waitFor(t, "the delivery failure", func() bool { h := b.Health(); return len(h.Failures) == 1 && h.Failures[0].What == "ALI" })
	f.events <- sse("call.ended", map[string]any{"id": "ring1"})
	waitFor(t, "the call to end", func() bool {
		var ok bool
		eng.Read(func(s *cad.State) { ok = s.Calls["ring1"].State == "ended" })
		return ok
	})
	if h := b.Health(); !h.Connected || h.Events < 5 {
		t.Fatalf("health %+v", h)
	}
}

func TestWebhooks(t *testing.T) {
	eng, b, _, _ := setup(t)
	eido, _ := os.ReadFile("../ng911/testdata/eido-bitvoice.json")
	h := b.EIDOHandler()
	req := httptest.NewRequest("POST", "/api/ng911/eido", bytes.NewReader(eido))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("no token: %d", w.Code)
	}
	req = httptest.NewRequest("POST", "/api/ng911/eido", bytes.NewReader(eido))
	req.Header.Set("Authorization", "Bearer eido-secret")
	req.Header.Set("Content-Type", "application/emergency.eido+json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Fatalf("EIDO push: %d %s", w.Code, w.Body)
	}
	// BitVoice's own radio-to-CAD test payload (internal/api/radiocad_test.go).
	body := `{"type":"radio.emergency","agency":"psap.example.gov","data":{"channel":"tg","channelName":"County fire","unit":"bee07.ad5.0000c5","unitId":197,"wacn":"bee07","system":"ad5","emergency":true,"at":"2026-10-18T22:00:00Z"}}`
	rh := b.RadioHandler()
	req = httptest.NewRequest("POST", "/api/radio/events", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer cad-shared-secret")
	w = httptest.NewRecorder()
	rh.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Fatalf("radio push: %d %s", w.Code, w.Body)
	}
	loc := `{"type":"radio.location","agency":"x","data":{"unit":101,"lat":40.3,"lon":-76.9,"speed":40,"at":"2026-10-18T22:00:01Z"}}`
	req = httptest.NewRequest("POST", "/api/radio/events", strings.NewReader(loc))
	req.Header.Set("Authorization", "Bearer cad-shared-secret")
	w = httptest.NewRecorder()
	rh.ServeHTTP(w, req)
	eng.Read(func(s *cad.State) {
		if s.Calls["abc123"] == nil || s.Units["P1"].Emergency == "" || s.Units["M1"].Position == nil || s.Units["M1"].Position.Speed != 40 {
			t.Fatalf("after webhooks: call %v, P1 %+v, M1 %+v", s.Calls["abc123"], s.Units["P1"], s.Units["M1"].Position)
		}
	})
	if h := b.Health(); h.LastPush.IsZero() || h.LastRadioWH.IsZero() {
		t.Fatalf("health %+v", h)
	}
}

func TestActions(t *testing.T) {
	eng, b, f, _ := setup(t)
	ctx := context.Background()
	if msg, err := b.Test(ctx); err != nil || !strings.Contains(msg, "Connected") {
		t.Fatalf("test: %q %v", msg, err)
	}
	eng.UpsertCall(cad.BitVoiceActor, "abc123", cad.CallData{Source: "bitvoice", State: "answered", Callback: "+17175550100"})
	loc, err := b.Rebid(ctx, "abc123")
	if err != nil || loc.Radius != 12 || loc.Source != "rebid" {
		t.Fatalf("rebid %+v %v", loc, err)
	}
	if err := b.Text(ctx, "abc123", "", "Are you safe?"); err != nil || f.texts[0] != "Are you safe?" {
		t.Fatalf("text %v %v", err, f.texts)
	}
	if err := b.AckRadio(ctx, "tg"); err != nil || f.acked[0] != "tg" {
		t.Fatalf("ack %v", err)
	}
	if _, err := b.Fetch(ctx, "https://evil.example/x"); err == nil {
		t.Fatal("fetched from another host")
	}
	// A wrong pin refuses the connection.
	cfg := b.Config()
	cfg.Fingerprint = strings.Repeat("ab", 32)
	b.Configure(cfg)
	if _, err := b.Test(ctx); err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("wrong pin: %v", err)
	}
}

func TestFingerprint(t *testing.T) {
	_, _, _, srv := setup(t)
	fp, _, err := Fingerprint(context.Background(), srv.URL)
	sum := sha256.Sum256(srv.Certificate().Raw)
	if err != nil || fp != hex.EncodeToString(sum[:]) {
		t.Fatalf("%s %v", fp, err)
	}
}

// What BitVoice says beyond the basics, in its real shapes (from live testing): the caller ID's check,
// the location check, text channels, photos as evidence, the LVF, transcripts, routing and PSAP states.
func TestHandlingFromBitVoice(t *testing.T) {
	eng, b, f, _ := setup(t)
	f.calls = []map[string]any{{"id": "c9", "state": "answered", "from": "+15405550144", "start": time.Now(),
		"screen": map[string]any{"verified": "passed", "attest": "A", "why": "the provider verified the caller ID"},
		"emergency": map[string]any{"esinet": true, "service": "urn:service:sos", "textVia": []string{"msrp"}, "takenName": "Call taker 1",
			"location": map[string]any{"lat": 37.28, "lon": -79.90, "radius": 8, "method": "GPS",
				"civic": map[string]any{"country": "US", "state": "VA", "city": "Roanoke", "number": "215", "street": "Chruch", "streetType": "Ave", "postDir": "SW"}},
			"locationCheck": map[string]any{"show": "new", "text": "Show the new location first", "reasons": []string{"the latest"}},
			"additional":    map[string]any{}}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go b.Run(ctx)
	get := func() *cad.Handling {
		var h *cad.Handling
		eng.Read(func(s *cad.State) {
			if c := s.Calls["c9"]; c != nil && c.Handling != nil {
				v := *c.Handling
				h = &v
			}
		})
		return h
	}
	waitFor(t, "the handling, evidence and LVF", func() bool {
		h := get()
		return h != nil && h.CallerID != nil && h.CallerID.Verified == "passed" && h.LocCheck != nil && h.Evidence != nil && len(h.Evidence.Items) == 1 &&
			h.LVF != nil && !h.LVF.Valid && len(h.LVF.Suggestions) == 1 && h.LVF.Suggestions[0].Address == "215 Church Ave SW, Roanoke VA"
	})
	if h := get(); !h.ESInet || h.TextVia[0] != "msrp" || h.Taker != "Call taker 1" || !h.Evidence.Items[0].HasGPS {
		t.Fatalf("%+v", h)
	}
	// A briefing when the call is passed on, then the summary after it: both stay.
	f.events <- sse("call.briefing", map[string]any{"callId": "c9", "target": "101", "by": "100 Call taker 1", "summary": " A woman fell at home. "})
	waitFor(t, "the briefing", func() bool {
		h := get()
		return h.Briefing == "A woman fell at home." && h.BriefedBy == "100 Call taker 1"
	})
	f.events <- sse("transcript.done", map[string]any{"callId": "c9", "recording": "2026-10/c9.wav", "status": "done", "summary": "A caller reports a fall.",
		"lines": []any{map[string]any{"second": 1.2, "side": "caller", "text": "my mother fell"}}})
	f.events <- sse("esrp.route", map[string]any{"call": "c9", "service": "urn:service:sos", "psap": "sip:sos@east", "why": "the caller's location is in East", "at": time.Now()})
	f.events <- sse("core.states", nil)
	// The ESRP routes a call before BitVoice announces it: the decision waits for the call.
	f.events <- sse("esrp.route", map[string]any{"call": "c10", "service": "urn:service:sos", "psap": "sip:sos@psap-b",
		"why": "default: the call brought no location; passed over PSAP A (its queue says Full)", "at": time.Now()})
	f.events <- sse("call.started", map[string]any{"id": "c10", "state": "ringing", "from": "+15405550351", "start": time.Now(),
		"emergency": map[string]any{"esinet": true, "additional": map[string]any{}}})
	waitFor(t, "the early route", func() bool {
		var rs []cad.Route
		eng.Read(func(s *cad.State) {
			if c := s.Calls["c10"]; c != nil && c.Handling != nil {
				rs = c.Handling.Routes
			}
		})
		return len(rs) == 1 && rs[0].PSAP == "sip:sos@psap-b"
	})
	waitFor(t, "the transcript, summary and route", func() bool {
		h := get()
		var lines int
		eng.Read(func(s *cad.State) { lines = len(s.Calls["c9"].Transcript) })
		return h.Summary == "A caller reports a fall." && h.Briefing == "A woman fell at home." && h.Recording != "" && len(h.Routes) == 1 && lines == 1
	})
	waitFor(t, "the PSAP states", func() bool { p := b.Health().PSAPs; return len(p) == 1 && p[0].Queue == "Full" })

	// A live transcript line: BitVoice's speaker is "Caller" or the taker's name, its side caller or taker.
	f.events <- sse("call.transcript", map[string]any{"call": "c9", "lines": []any{map[string]any{"speaker": "Call taker 1", "side": "taker", "text": "where are you", "second": 4.5}}})
	waitFor(t, "the live line", func() bool {
		var l []cad.TranscriptLine
		eng.Read(func(s *cad.State) { l = s.Calls["c9"].Transcript })
		return len(l) == 2 && l[1].Speaker == "taker" && l[1].Text == "where are you" && !l[1].At.IsZero()
	})
	// The dispatcher is named to BitVoice.
	if err := b.TakeText(OnBehalf(ctx, "Pat Doe (pdoe)"), "c9"); err != nil {
		t.Fatal(err)
	}
	if err := b.TakeText(ctx, "c9"); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	if len(f.behalf) != 2 || f.behalf[0] != "Pat Doe (pdoe)" || f.behalf[1] != "" {
		t.Fatalf("on behalf of: %q", f.behalf)
	}
	f.mu.Unlock()
	// Why the call ended.
	f.events <- sse("call.ended", map[string]any{"id": "c9", "state": "ended", "cause": "the caller's audio stopped", "endedBy": "system"})
	waitFor(t, "the end and its cause", func() bool {
		var st, cause string
		eng.Read(func(s *cad.State) { c := s.Calls["c9"]; st, cause = c.State, c.Handling.EndCause })
		return st == "ended" && cause == "the caller's audio stopped"
	})
}
