package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// A fire incident is sent to NERIS when it closes; NERIS's ID is kept and a second send updates it.
func TestNERISOnClose(t *testing.T) {
	ts, s, pw := newServer(t)
	admin := login(t, ts, "admin", pw["admin"])
	d1 := login(t, ts, "d1", pw["d1"])
	var mu sync.Mutex
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
		case r.Method == "POST" && r.URL.Path == "/incident/FD51161234":
			b, _ := io.ReadAll(r.Body)
			mu.Lock()
			bodies = append(bodies, string(b))
			mu.Unlock()
			w.WriteHeader(201)
			fmt.Fprint(w, `{"neris_id":"NX-1"}`)
		case r.Method == "PUT" && r.URL.Path == "/incident/FD51161234/NX-1":
			fmt.Fprint(w, `{"neris_id":"NX-1"}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	s.nerisClient().Base = srv.URL
	if code, b := admin.do("PUT", "/api/admin/neris", map[string]any{"on": true, "department": "51161234", "clientId": "c", "secret": "x"}, true); code != 422 {
		t.Fatalf("a department ID without FD: %d %s", code, b)
	}
	if code, b := admin.do("PUT", "/api/admin/neris", map[string]any{"on": true, "department": "fd51161234", "clientId": "c", "secret": "x", "auto": true, "center": "Test ECC"}, true); code != 200 || strings.Contains(string(b), `"x"`) {
		t.Fatalf("neris: %d %s", code, b)
	}
	admin.do("PUT", "/api/admin/stations/st1", cad.Station{Name: "Station 1", Lat: 40.27, Lon: -76.88}, true)
	if code, b := admin.do("PUT", "/api/admin/units/e1", cad.UnitDef{Type: "engine", Station: "ST1"}, true); code != 204 {
		t.Fatalf("unit: %d %s", code, b)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.nerisAuto(ctx)
	time.Sleep(30 * time.Millisecond)
	_, b := d1.do("POST", "/api/incidents", map[string]any{"type": "SF", "location": map[string]any{"lat": 40.27, "lon": -76.88, "address": "100 Market St, Harrisburg PA"}}, true)
	var inc struct{ Number string }
	json.Unmarshal(b, &inc)
	if _, b := d1.do("POST", "/api/command", map[string]string{"line": "D E1 #" + inc.Number + " / test"}, true); !strings.Contains(string(b), `"ok":true`) {
		t.Fatalf("dispatch: %s", b)
	}
	if code, b := d1.do("POST", "/api/command", map[string]string{"line": "CL #" + inc.Number + " Handled"}, true); code != 200 || !strings.Contains(string(b), `"ok":true`) {
		t.Fatalf("close: %s", b)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		var rec *cad.NERISRecord
		s.Eng.Read(func(st *cad.State) { rec = st.Incidents[inc.Number].NERIS })
		if rec != nil {
			if !rec.OK || rec.ID != "NX-1" {
				t.Fatalf("record %+v", rec)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not sent to NERIS on close")
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	body := bodies[0]
	mu.Unlock()
	for _, want := range []string{`"department_neris_id":"FD51161234"`, `"reported_unit_id":"E1"`, `"type":"UNDETERMINED"`, `"center_id":"Test ECC"`, `"disposition":"Handled"`, `"street":"Market St"`} {
		if !strings.Contains(body, want) {
			t.Errorf("payload lacks %s: %s", want, body)
		}
	}
	// Sent again by hand: an update to the same NERIS record.
	if code, b := d1.do("POST", "/api/incidents/"+inc.Number+"/neris", nil, true); code != 403 {
		t.Fatalf("a dispatcher sent to NERIS: %d %s", code, b)
	}
	if code, b := admin.do("POST", "/api/incidents/"+inc.Number+"/neris", nil, true); code != 200 || !strings.Contains(string(b), `"ok":true`) {
		t.Fatalf("resend: %d %s", code, b)
	}
}
