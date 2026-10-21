package neris

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func sample() Incident {
	lat, lon := 37.2710, -79.9414
	t0 := time.Date(2026, 10, 20, 14, 0, 0, 0, time.UTC)
	return Incident{Number: "26-000123", Type: "SF", TypeName: "Structure fire", Created: t0.Add(40 * time.Second), CallArrival: t0, CallAnswer: t0.Add(8 * time.Second),
		Closed: t0.Add(2 * time.Hour), Disposition: "Fire out", Address: "215 Church Ave SW, Roanoke VA 24011", Lat: &lat, Lon: &lon,
		Units: []Unit{{ID: "E1", Dispatched: t0.Add(60 * time.Second), Enroute: t0.Add(90 * time.Second), OnScene: t0.Add(5 * time.Minute), Cleared: t0.Add(2 * time.Hour)},
			{ID: "L1", Dispatched: t0.Add(60 * time.Second), Enroute: t0.Add(2 * time.Minute), Cleared: t0.Add(4 * time.Minute), CanceledEnroute: true}},
		Comments: []Comment{{At: t0.Add(3 * time.Minute), Text: "Smoke showing from the second floor"}, {At: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), Text: "too old for NERIS"}}}
}

func TestPayloadIsACadRecord(t *testing.T) {
	p := Payload(Config{Department: "FD51161234", Center: "Roanoke ECC"}, sample())
	b, _ := json.Marshal(p)
	s := string(b)
	for _, want := range []string{
		`"department_neris_id":"FD51161234"`, `"incident_types":[{"type":"UNDETERMINED"}]`, `"call_arrival":"2026-10-20T14:00:00Z"`,
		`"call_answered":"2026-10-20T14:00:08Z"`, `"call_create":"2026-10-20T14:00:40Z"`, `"reported_unit_id":"E1"`, `"on_scene":"2026-10-20T14:05:00Z"`,
		`"canceled_enroute":"2026-10-20T14:04:00Z"`, `"complete_number":"215"`, `"street":"Church Ave SW"`, `"coordinates":[-79.9414,37.271]`,
		`"center_id":"Roanoke ECC"`, `"incident_code":"SF Structure fire"`, `"disposition":"Fire out"`, "Smoke showing",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("payload lacks %s\n%s", want, s)
		}
	}
	if strings.Contains(s, "too old for NERIS") {
		t.Error("a comment before 2025 was sent")
	}
}

func TestConfigCheck(t *testing.T) {
	if (Config{On: true, Department: "51161234", ClientID: "a", ClientSecret: "b"}).Check() == nil {
		t.Error("a department ID without FD was taken")
	}
	if (Config{On: true, Department: "FD51161234"}).Check() == nil {
		t.Error("no credentials, yet taken")
	}
	if err := (Config{On: true, Department: "FD51161234", ClientID: "a", ClientSecret: "b"}).Check(); err != nil {
		t.Error(err)
	}
}

func TestSendCreatesThenUpdates(t *testing.T) {
	tokens, created, updated := 0, 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			w.WriteHeader(403) // as NERIS's firewall does
			return
		}
		switch {
		case r.Method == "POST" && r.URL.Path == "/token":
			if u, p, _ := r.BasicAuth(); u != "cid" || p != "sec" || r.FormValue("grant_type") != "client_credentials" {
				w.WriteHeader(401)
				return
			}
			tokens++
			fmt.Fprint(w, `{"access_token":"tok","token_type":"Bearer","expires_in":3600}`)
		case r.Header.Get("Authorization") != "Bearer tok":
			w.WriteHeader(401)
		case r.Method == "POST" && r.URL.Path == "/incident/FD51161234/validate":
			fmt.Fprint(w, `{"valid":true}`)
		case r.Method == "POST" && r.URL.Path == "/incident/FD51161234":
			var p map[string]any
			if json.NewDecoder(r.Body).Decode(&p) != nil || p["dispatch"] == nil {
				w.WriteHeader(422)
				fmt.Fprint(w, `{"detail":"dispatch is required"}`)
				return
			}
			created++
			w.WriteHeader(201)
			fmt.Fprint(w, `{"neris_id":"FD51161234|26-000123|1760968800"}`)
		case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/incident/FD51161234/FD51161234|26-000123"):
			updated++
			fmt.Fprint(w, `{"neris_id":"FD51161234|26-000123|1760968800"}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	cl := &Client{Base: srv.URL, Agent: "bitdispatcher-test"}
	c := Config{On: true, ClientID: "cid", ClientSecret: "sec", Department: "FD51161234"}
	if r, err := cl.Validate(context.Background(), c, sample()); err != nil || r.Status != 200 {
		t.Fatalf("validate: %v %+v", err, r)
	}
	r, err := cl.Send(context.Background(), c, sample(), "")
	if err != nil || r.ID != "FD51161234|26-000123|1760968800" || created != 1 {
		t.Fatalf("create: %v %+v", err, r)
	}
	if _, err := cl.Send(context.Background(), c, sample(), r.ID); err != nil || updated != 1 {
		t.Fatalf("update: %v", err)
	}
	if tokens != 1 {
		t.Errorf("signed in %d times, want once", tokens)
	}
	if _, err := cl.Send(context.Background(), Config{On: true, ClientID: "cid", ClientSecret: "wrong", Department: "FD51161234"}, sample(), ""); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Errorf("a wrong secret: %v", err)
	}
}
