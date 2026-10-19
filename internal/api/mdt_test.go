package api

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

func TestMDT(t *testing.T) {
	ts, s, pw := newServer(t)
	eng := s.Eng
	eng.SaveStation(cad.SystemActor, cad.Station{ID: "ST1", Name: "Station 1", Lat: 37.27, Lon: -79.94})
	eng.SaveUnit(cad.SystemActor, cad.UnitDef{ID: "E1", Type: "engine", Station: "ST1"})
	eng.SetStatus(cad.SystemActor, "E1", cad.Available, "", "")
	crew := auth.GeneratePassword()
	s.Auth.Save("e1crew", "Engine 1 crew", auth.Field, crew, false)
	signIn := func(console string) (*client, int) {
		jar, _ := cookiejar.New(nil)
		c := &client{t: t, base: ts.URL, c: &http.Client{Transport: ts.Client().Transport, Jar: jar}}
		code, _ := c.do("POST", "/api/auth/login", map[string]string{"username": "e1crew", "password": crew, "console": console}, true)
		return c, code
	}
	if _, code := signIn("D1"); code != 401 {
		t.Fatalf("a field crew signed in at a console that is not a unit: %d", code)
	}
	c, code := signIn("e1")
	if code != 200 {
		t.Fatalf("sign in as E1: %d", code)
	}
	// The crew cannot dispatch or change configuration.
	if code, _ := c.do("POST", "/api/incidents", cad.NewIncident{Type: "CHEST"}, true); code != 403 {
		t.Fatalf("field crew created an incident: %d", code)
	}
	d := login(t, ts, "d1", pw["d1"])
	code, b := d.do("POST", "/api/incidents", cad.NewIncident{Type: "CHEST", Location: nil}, true)
	var created struct{ Number string }
	json.Unmarshal(b, &created)
	if code != 201 && code != 200 {
		t.Fatalf("incident: %d %s", code, b)
	}
	if r := eng.Execute(cad.Actor{User: "d1", Role: auth.Dispatcher}, "D E1 #"+created.Number, ""); !r.OK {
		t.Fatal(r.Message)
	}
	code, b = c.do("GET", "/api/mdt", nil, false)
	var v MDTView
	json.Unmarshal(b, &v)
	if code != 200 || v.Unit.ID != "E1" || v.Incident == nil || v.Incident.Number != created.Number {
		t.Fatalf("mdt: %d %s", code, b)
	}
	if code, b := c.do("POST", "/api/mdt/status", map[string]string{"status": "enroute"}, true); code != 204 {
		t.Fatalf("en route: %d %s", code, b)
	}
	if code, _ := c.do("POST", "/api/mdt/note", map[string]string{"text": "Two minutes out"}, true); code != 204 {
		t.Fatalf("note: %d", code)
	}
	if code, _ := c.do("POST", "/api/mdt/position", map[string]float64{"lat": 37.2705, "lon": -79.95}, true); code != 204 {
		t.Fatalf("position: %d", code)
	}
	eng.Read(func(st *cad.State) {
		u := st.Units["E1"]
		if u.Status != cad.Enroute || u.Position == nil || u.Position.Source != "mdt" {
			t.Fatalf("unit %+v", u)
		}
		in := st.Incidents[created.Number]
		found := false
		for _, e := range in.Timeline {
			found = found || strings.Contains(e.Text, "E1: Two minutes out")
		}
		if !found {
			t.Fatal("note missing")
		}
	})
	// The crew acts only as its own unit, whatever it asks for.
	eng.SaveUnit(cad.SystemActor, cad.UnitDef{ID: "E2", Type: "engine", Station: "ST1"})
	eng.SetStatus(cad.SystemActor, "E2", cad.Available, "", "")
	c.do("POST", "/api/mdt/status?unit=E2", map[string]string{"status": "oos"}, true)
	eng.Read(func(st *cad.State) {
		if st.Units["E2"].Status == cad.OutOfService {
			t.Fatal("field crew changed another unit")
		}
	})
	// A viewer has no terminal.
	vw := login(t, ts, "v1", pw["v1"])
	if code, _ := vw.do("POST", "/api/mdt/status?unit=E1", map[string]string{"status": "available"}, true); code != 403 {
		t.Fatalf("viewer changed a status: %d", code)
	}
	if code, _ := c.do("POST", "/api/mdt/emergency", nil, true); code != 200 {
		t.Fatalf("emergency: %d", code)
	}
	eng.Read(func(st *cad.State) {
		if st.Units["E1"].Emergency == "" {
			t.Fatal("no emergency")
		}
	})
	// The crew marks itself out: only the kinds for its discipline are offered, and others are refused.
	code, b = c.do("GET", "/api/mdt", nil, false)
	json.Unmarshal(b, &v)
	for _, k := range v.Settings.MarkOuts {
		if k.Discipline == "police" {
			t.Fatalf("an engine was offered %s", k.Code)
		}
	}
	if code, _ := c.do("POST", "/api/mdt/markout", map[string]string{"code": "TS"}, true); code != 400 {
		t.Fatalf("an engine marked out on a traffic stop: %d", code)
	}
	if code, b := c.do("POST", "/api/mdt/markout", map[string]string{"code": "INSP", "address": "Somewhere / odd", "note": "pre-plan"}, true); code != 200 {
		t.Fatalf("mark out: %d %s", code, b)
	}
	eng.Read(func(st *cad.State) {
		u := st.Units["E1"]
		if u.Status != cad.MarkedOut || u.MarkOut.Code != "INSP" || u.MarkOut.Note != "pre-plan" || u.MarkOut.Location.Address != "Somewhere odd" {
			t.Fatalf("marked out %+v %+v", u, u.MarkOut)
		}
	})
}
