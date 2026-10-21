package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// An alarm company's alarm arrives as a call; the dispatcher makes the incident; the company is told it
// was accepted, dispatched and closed; a cancel from the company goes on the incident as a priority note.
func TestAlarmAlert(t *testing.T) {
	ts, s, pw := newServer(t)
	admin := login(t, ts, "admin", pw["admin"])
	d1 := login(t, ts, "d1", pw["d1"])

	var mu sync.Mutex
	var got []string
	company := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ans" {
			t.Errorf("answer without its token")
		}
		var a struct{ ID, Status, Incident string }
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &a)
		mu.Lock()
		got = append(got, a.ID+":"+a.Status)
		mu.Unlock()
	}))
	defer company.Close()
	if code, b := admin.do("PUT", "/api/admin/connections", map[string]any{"hookToken": "ht", "alertAnswerUrl": "http://127.0.0.1" + strings.TrimPrefix(company.URL, "http://127.0.0.1"), "alertAnswerToken": "ans"}, true); code != 200 || strings.Contains(string(b), `"ans"`) {
		t.Fatalf("connections: %d %s", code, b)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.alarmAnswers(ctx)
	time.Sleep(50 * time.Millisecond)

	alarm := map[string]any{"sender": "Acme Monitoring", "id": "EV-1001", "event": "burglary", "zone": "3 rear door", "premise": "Main Street Pharmacy",
		"account": "A-77", "permit": "P-2026-14", "callback": "+18005550100", "keyholder": "J. Doe +15405550111", "lat": 37.27, "lon": -79.94, "address": "12 Main St"}
	if code, _ := hookPost(t, ts, "/api/hooks/alarm", "wrong", alarm); code != 401 {
		t.Fatalf("an alarm without the hook token: %d", code)
	}
	code, b := hookPost(t, ts, "/api/hooks/alarm", "ht", alarm)
	var r struct{ Call, Status string }
	json.Unmarshal(b, &r)
	if code != 200 || r.Status != "new" || r.Call != "alarm-acme-monitoring-EV-1001" {
		t.Fatalf("alarm: %d %s", code, b)
	}
	_, b = d1.do("GET", "/api/state", nil, false)
	if !strings.Contains(string(b), "BURGLARY alarm · zone 3 rear door") || !strings.Contains(string(b), `"class":"ALARM"`) || !strings.Contains(string(b), "Keyholder: J. Doe") {
		t.Fatalf("the alarm is not in the queue as a call: %.400s", b)
	}
	// The same event again is an update to the same call.
	alarm["verified"] = true
	if _, b := hookPost(t, ts, "/api/hooks/alarm", "ht", alarm); !strings.Contains(string(b), `"status":"updated"`) {
		t.Fatalf("update: %s", b)
	}
	code, b = d1.do("POST", "/api/incidents", map[string]any{"type": "card", "call": r.Call, "location": map[string]any{"lat": 37.27, "lon": -79.94, "address": "12 Main St"}}, true)
	var inc struct{ Number string }
	json.Unmarshal(b, &inc)
	if code != 201 || inc.Number == "" {
		t.Fatalf("incident: %d %s", code, b)
	}
	// The company cancels: a priority note on the incident.
	alarm["cancel"] = true
	alarm["detail"] = "keyholder on scene, code given"
	if _, b := hookPost(t, ts, "/api/hooks/alarm", "ht", alarm); !strings.Contains(string(b), `"status":"cancelled"`) || !strings.Contains(string(b), inc.Number) {
		t.Fatalf("cancel: %s", b)
	}
	found := false
	s.Eng.Read(func(st *cad.State) {
		for _, x := range st.Incidents[inc.Number].Timeline {
			found = found || strings.Contains(x.Text, "ALARM CANCELLED by Acme Monitoring keyholder on scene")
		}
	})
	if !found {
		s.Eng.Read(func(st *cad.State) {
			for _, x := range st.Incidents[inc.Number].Timeline {
				t.Log(x.Kind, x.Text)
			}
		})
		t.Fatal("the cancel is not on the incident")
	}
	if code, b := d1.do("POST", "/api/command", map[string]string{"line": "CL #" + inc.Number + " False alarm"}, true); code != 200 || !strings.Contains(string(b), `"ok":true`) {
		t.Fatalf("close: %d %s", code, b)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		all := strings.Join(got, ",")
		mu.Unlock()
		if strings.Contains(all, "EV-1001:accepted") && strings.Contains(all, "EV-1001:closed") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("answers to the company: %s", all)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// A plate-reader hit and a gunshot detection, each its own call.
	if code, b := hookPost(t, ts, "/api/hooks/lpr", "ht", map[string]any{"sender": "Flock", "id": "h-9", "plate": "ktr 4471", "state": "VA", "hotlist": "Stolen vehicle (NCIC)", "vehicle": "silver Honda Civic", "heading": "northbound", "camera": "Williamson Rd at Orange", "lat": 37.29, "lon": -79.95, "image": "javascript:alert(1)"}); code != 200 {
		t.Fatalf("lpr: %d %s", code, b)
	}
	if code, b := hookPost(t, ts, "/api/hooks/gunshot", "ht", map[string]any{"sender": "SoundThinking", "id": "g-1", "rounds": 4, "lat": 37.26, "lon": -79.93}); code != 200 {
		t.Fatalf("gunshot: %d %s", code, b)
	}
	_, b = d1.do("GET", "/api/state", nil, false)
	if !strings.Contains(string(b), "Plate reader: KTR4471 VA · Stolen vehicle (NCIC)") || !strings.Contains(string(b), "Gunshots detected: 4 rounds") || strings.Contains(string(b), "javascript:") {
		t.Fatal("plate reader or gunshot not in the queue (or an unsafe link kept)")
	}
}

// Protocol software reports a determinant through the hook; it sets the priority and is on the incident.
func TestDeterminantHook(t *testing.T) {
	ts, s, pw := newServer(t)
	admin := login(t, ts, "admin", pw["admin"])
	d1 := login(t, ts, "d1", pw["d1"])
	admin.do("PUT", "/api/admin/connections", map[string]any{"hookToken": "ht"}, true)
	_, b := d1.do("POST", "/api/incidents", map[string]any{"type": "card", "location": map[string]any{"lat": 40.27, "lon": -76.88}}, true)
	var inc struct{ Number string }
	json.Unmarshal(b, &inc)
	if code, b := hookPost(t, ts, "/api/hooks/determinant", "ht", map[string]any{"incident": inc.Number, "system": "ProQA", "code": "09-E-1", "text": "Not breathing", "priority": 1}); code != 204 {
		t.Fatalf("determinant: %d %s", code, b)
	}
	s.Eng.Read(func(st *cad.State) {
		in := st.Incidents[inc.Number]
		if in.Protocol == nil || in.Protocol.Determinant.Code != "09-E-1" || in.Protocol.System != "ProQA" || in.Priority != 1 {
			t.Fatalf("%+v", in.Protocol)
		}
	})
	if code, _ := d1.do("PUT", "/api/admin/protocols", cad.ExampleProtocol(), true); code != 403 {
		t.Fatalf("a dispatcher wrote a protocol: %d", code)
	}
	ex := cad.ExampleProtocol()
	ex.Approved = true
	if code, b := admin.do("PUT", "/api/admin/protocols", ex, true); code != 204 {
		t.Fatalf("protocol: %d %s", code, b)
	}
	if code, b := d1.do("POST", "/api/incidents/"+inc.Number+"/protocol", map[string]string{"protocol": ex.ID, "question": "q1", "answer": "No / not sure"}, true); code != 204 {
		t.Fatalf("answer: %d %s", code, b)
	}
}
