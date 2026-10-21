package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// Files on an incident: a dispatcher adds a picture, a crew not on the incident cannot, a crew on it can;
// the file comes back as stored, a disguised script is refused, and a removed file is hidden with why.
func TestIncidentFiles(t *testing.T) {
	ts, s, pw := newServer(t)
	d1 := login(t, ts, "d1", pw["d1"])
	sup := login(t, ts, "admin", pw["admin"])
	_, b := d1.do("POST", "/api/incidents", map[string]any{"type": "SF", "location": map[string]any{"lat": 40.27, "lon": -76.88}}, true)
	var inc struct{ Number string }
	json.Unmarshal(b, &inc)
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x02\x00\x00\x00\x90wS\xde")
	code, b := d1.upload("/api/incidents/"+inc.Number+"/files?name=../../etc/scene.png&caption=Side%20C", string(png))
	if code != 200 {
		t.Fatalf("upload: %d %s", code, b)
	}
	var up struct{ ID string }
	json.Unmarshal(b, &up)
	if code, _ := d1.upload("/api/incidents/"+inc.Number+"/files?name=x.png", "<html><script>alert(1)</script></html>"); code != 415 {
		t.Fatalf("html taken: %d", code)
	}
	req, _ := http.NewRequest("GET", ts.URL+"/api/incidents/"+inc.Number+"/files/"+up.ID, nil)
	resp, err := d1.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	got.ReadFrom(resp.Body)
	resp.Body.Close()
	if !bytes.Equal(got.Bytes(), png) || resp.Header.Get("Content-Type") != "image/png" || !strings.Contains(resp.Header.Get("Content-Disposition"), `filename=scene.png`) {
		t.Fatalf("download: %s %q", resp.Header, got.String())
	}
	// A crew adds only to incidents it is on.
	s.Eng.SaveStation(cad.SystemActor, cad.Station{ID: "ST1", Name: "Station 1", Lat: 40.27, Lon: -76.88})
	s.Eng.SaveUnit(cad.SystemActor, cad.UnitDef{ID: "E1", Type: "engine", Station: "ST1"})
	s.Eng.SetStatus(cad.SystemActor, "E1", cad.Available, "", "")
	s.Auth.Save("e1crew", "Engine 1 crew", auth.Field, "crew-pass-123456", false)
	jar, _ := cookiejar.New(nil)
	crew := &client{t: t, base: ts.URL, c: &http.Client{Transport: ts.Client().Transport, Jar: jar}}
	if code, b := crew.do("POST", "/api/auth/login", map[string]string{"username": "e1crew", "password": "crew-pass-123456", "console": "E1"}, true); code != 200 {
		t.Fatalf("crew login %d %s", code, b)
	}
	if code, _ := crew.upload("/api/incidents/"+inc.Number+"/files?name=a.png", string(png)); code != 403 {
		t.Fatalf("a crew not on it added a file: %d", code)
	}
	if err := s.Eng.Dispatch(cad.SystemActor, inc.Number, []string{"E1"}, nil); err != nil {
		t.Fatal(err)
	}
	if code, b := crew.upload("/api/incidents/"+inc.Number+"/files?name=a.png", string(png)); code != 200 {
		t.Fatalf("the crew on it: %d %s", code, b)
	}
	if code, _ := d1.do("DELETE", "/api/incidents/"+inc.Number+"/files/"+up.ID, map[string]string{"why": "wrong house"}, true); code != 403 {
		t.Fatalf("a dispatcher removed a file: %d", code)
	}
	if code, b := sup.do("DELETE", "/api/incidents/"+inc.Number+"/files/"+up.ID, map[string]string{"why": "wrong house"}, true); code != 204 {
		t.Fatalf("remove: %d %s", code, b)
	}
	if code, _ := d1.do("GET", "/api/incidents/"+inc.Number+"/files/"+up.ID, nil, false); code != 404 {
		t.Fatalf("a hidden file was served to a dispatcher: %d", code)
	}
	s.Eng.Read(func(st *cad.State) {
		fs := st.Incidents[inc.Number].Files
		if len(fs) != 2 || !fs[0].Hidden || fs[0].Why != "wrong house" || fs[0].Name != "scene.png" || fs[1].Unit != "E1" {
			t.Fatalf("%+v", fs)
		}
	})
}

// A supervisor corrects a closed incident; a dispatcher cannot.
func TestCorrectAPI(t *testing.T) {
	ts, s, pw := newServer(t)
	d1 := login(t, ts, "d1", pw["d1"])
	sup := login(t, ts, "admin", pw["admin"])
	_, b := d1.do("POST", "/api/incidents", map[string]any{"type": "SF", "location": map[string]any{"lat": 40.27, "lon": -76.88}}, true)
	var inc struct{ Number string }
	json.Unmarshal(b, &inc)
	if code, b := d1.do("POST", "/api/incidents/"+inc.Number+"/close", map[string]string{"disposition": "Handled"}, true); code != 204 {
		t.Fatalf("close %d %s", code, b)
	}
	body := map[string]any{"disposition": "False alarm", "reason": "the caller called back: smoke from a grill"}
	if code, _ := d1.do("POST", "/api/incidents/"+inc.Number+"/correct", body, true); code != 403 {
		t.Fatalf("a dispatcher corrected: %d", code)
	}
	if code, b := sup.do("POST", "/api/incidents/"+inc.Number+"/correct", body, true); code != 200 || !strings.Contains(string(b), `"to":"False alarm"`) {
		t.Fatalf("correct %d %s", code, b)
	}
	s.Eng.Read(func(st *cad.State) {
		if in := st.Incidents[inc.Number]; in.Disposition != "False alarm" || in.Status != cad.Closed {
			t.Fatalf("%+v", in)
		}
	})
}

// A narrative draft comes from the record when the model is off.
func TestDraftNarrative(t *testing.T) {
	ts, _, pw := newServer(t)
	d1 := login(t, ts, "d1", pw["d1"])
	_, b := d1.do("POST", "/api/incidents", map[string]any{"type": "SF", "location": map[string]any{"lat": 40.27, "lon": -76.88, "address": "1 Mill Rd"}}, true)
	var inc struct{ Number string }
	json.Unmarshal(b, &inc)
	d1.do("POST", "/api/incidents/"+inc.Number+"/notes", map[string]any{"text": "Smoke showing from the second floor"}, true)
	code, b := d1.do("POST", "/api/incidents/"+inc.Number+"/draft-narrative", map[string]any{}, true)
	if code != 200 || !strings.Contains(string(b), "1 Mill Rd") || !strings.Contains(string(b), "Smoke showing") || !strings.Contains(string(b), `"by":"record"`) {
		t.Fatalf("%d %s", code, b)
	}
}

// Replay gives an incident's timeline and its units' recorded positions.
func TestReplay(t *testing.T) {
	ts, s, pw := newServer(t)
	sup := login(t, ts, "admin", pw["admin"])
	d1 := login(t, ts, "d1", pw["d1"])
	s.Eng.SaveStation(cad.SystemActor, cad.Station{ID: "ST1", Name: "Station 1", Lat: 40.27, Lon: -76.88})
	s.Eng.SaveUnit(cad.SystemActor, cad.UnitDef{ID: "E1", Type: "engine", Station: "ST1"})
	s.Eng.SetStatus(cad.SystemActor, "E1", cad.Available, "", "")
	_, b := d1.do("POST", "/api/incidents", map[string]any{"type": "SF", "location": map[string]any{"lat": 40.28, "lon": -76.89}}, true)
	var inc struct{ Number string }
	json.Unmarshal(b, &inc)
	s.Eng.Dispatch(cad.SystemActor, inc.Number, []string{"E1"}, nil)
	s.Eng.UnitPosition(cad.SystemActor, "E1", cad.Position{Lat: 40.275, Lon: -76.885, Source: "avl"})
	if code, _ := d1.do("GET", "/api/incidents/"+inc.Number+"/replay", nil, false); code != 403 {
		t.Fatalf("a dispatcher replayed: %d", code)
	}
	code, b := sup.do("GET", "/api/incidents/"+inc.Number+"/replay", nil, false)
	var rp Replay
	json.Unmarshal(b, &rp)
	if code != 200 || len(rp.Tracks) != 1 || rp.Tracks[0].Unit != "E1" || len(rp.Tracks[0].Points) == 0 || len(rp.Timeline) < 2 {
		t.Fatalf("%d %s", code, b)
	}
}
