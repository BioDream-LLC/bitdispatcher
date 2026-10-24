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
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
	"github.com/biodream-llc/bitdispatcher/internal/paging"
)

func TestRoadmapAPI(t *testing.T) {
	ts, s, pw := newServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.pager(ctx)
	go s.epcrAuto(ctx)
	admin := login(t, ts, "admin", pw["admin"])
	d1 := login(t, ts, "d1", pw["d1"])
	v1 := login(t, ts, "v1", pw["v1"])
	ok := func(code int, b []byte) []byte {
		t.Helper()
		if code >= 300 {
			t.Fatalf("%d %s", code, b)
		}
		return b
	}
	sys := cad.SystemActor
	lat, lon := 37.2710, -79.9414
	if err := s.Eng.SaveStation(sys, cad.Station{ID: "ST1", Name: "Station 1", Lat: lat, Lon: lon}); err != nil {
		t.Fatal(err)
	}
	for _, u := range []cad.UnitDef{{ID: "E1", Type: "engine", Station: "ST1"}, {ID: "M1", Type: "medic", Station: "ST1"}, {ID: "P1", Type: "patrol"}} {
		s.Eng.SaveUnit(sys, u)
		s.Eng.SetStatus(sys, u.ID, cad.Available, "", "")
	}

	// Paging: a webhook stand-in receives the page; a responder answers on the public link.
	var mu sync.Mutex
	var got []map[string]any
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var v map[string]any
		json.Unmarshal(b, &v)
		mu.Lock()
		got = append(got, v)
		mu.Unlock()
	}))
	defer hook.Close()
	// http on loopback is allowed for a webhook.
	ok(admin.do("PUT", "/api/admin/paging", Paging{PublicURL: "https://cad.example.gov", Gateways: []paging.Gateway{{ID: "hook", Name: "Pager hook", Kind: paging.Webhook, URL: hook.URL, Token: "s3cret"}}}, true))
	code, b := admin.do("GET", "/api/admin/paging", nil, false)
	if code != 200 || strings.Contains(string(b), "s3cret") {
		t.Fatalf("paging view shows the secret: %s", b)
	}
	ok(admin.do("PUT", "/api/admin/members", cad.Member{ID: "301", Name: "Gus Lane", Email: "gus@example.com", Volunteer: true}, true))
	ok(admin.do("PUT", "/api/admin/pagegroups", cad.PageGroup{Name: "Station 1 volunteers", Members: []string{"301"}, Gateways: []string{"hook"}, Stations: []string{"ST1"}}, true))
	if code, _ := admin.do("PUT", "/api/admin/pagegroups", cad.PageGroup{Name: "x", Gateways: []string{"nope"}}, true); code != 422 {
		t.Fatalf("unknown gateway: %d", code)
	}
	num, err := s.Eng.CreateIncident(sys, cad.NewIncident{Type: "SF", Location: &ng911.Location{Lat: &lat, Lon: &lon, Address: "215 Church Ave SW"}})
	if err != nil {
		t.Fatal(err)
	}
	ok(d1.do("POST", "/api/incidents/"+num+"/dispatch", map[string]any{"units": []string{"E1"}, "confirm": true, "reason": "test"}, true))
	var pageID string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.Eng.Read(func(st *cad.State) {
			if ps := st.PagesFor(num); len(ps) > 0 && len(ps[0].Results) > 0 {
				pageID = ps[0].ID
			}
		})
		if pageID != "" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if pageID == "" {
		t.Fatal("dispatching E1 did not page station 1's volunteers")
	}
	mu.Lock()
	if len(got) != 1 || got[0]["subject"] != "STRUCTURE FIRE 215 Church Ave SW" {
		t.Fatalf("webhook got %+v", got)
	}
	mu.Unlock()
	// The answer link: the webhook kind has no per-person link, so take a token from a page by name.
	_, to, err := s.Eng.SendPage(sys, num, nil, []string{"301"}, "", "second page", false)
	if err != nil || len(to) != 1 {
		t.Fatal(err)
	}
	var pid string
	s.Eng.Read(func(st *cad.State) { ps := st.PagesFor(num); pid = ps[len(ps)-1].ID })
	pub := &client{t: t, base: ts.URL, c: ts.Client()}
	if code, _ := pub.do("GET", "/api/page-reply/"+pid+"?t=wrong", nil, false); code != 404 {
		t.Fatalf("a wrong token: %d", code)
	}
	code, b = pub.do("GET", "/api/page-reply/"+pid+"?t="+to[0].Token, nil, false)
	if code != 200 || !strings.Contains(string(b), "Gus Lane") {
		t.Fatalf("reply page %d %s", code, b)
	}
	if code, _ := pub.do("POST", "/api/page-reply/"+pid, map[string]any{"t": to[0].Token, "answer": "responding", "eta": 6}, false); code != 403 {
		t.Fatalf("answer without the header: %d", code)
	}
	ok(pub.do("POST", "/api/page-reply/"+pid, map[string]any{"t": to[0].Token, "answer": "responding", "eta": 6}, true))
	var view View
	json.Unmarshal(ok(v1.do("GET", "/api/state", nil, false)), &view)
	if rb := view.Roadmap.Responders[num]; rb.Responding != 1 || rb.Paged != 1 || rb.Replies[0].ETA != 6 {
		t.Fatalf("responders %+v", rb)
	}
	if strings.Contains(string(ok(v1.do("GET", "/api/state", nil, false))), "gus@example.com") {
		t.Fatal("the console view shows a member's email")
	}
	if a := view.Roadmap.Alarms[num]; a.NextName != "2nd alarm" {
		t.Fatalf("alarm brief %+v", a)
	}

	// Alarm upgrade returns the bigger recommendation.
	var rec cad.Recommendation
	json.Unmarshal(ok(d1.do("POST", "/api/incidents/"+num+"/alarm", map[string]any{}, true)), &rec)
	if !strings.Contains(strings.Join(rec.Notes, " "), "2nd alarm") {
		t.Fatalf("alarm rec %+v", rec.Notes)
	}

	// Patient-care reports: a stand-in ePCR system takes M1's response when it clears.
	var epcr []string
	epcrSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		epcr = append(epcr, r.Header.Get("Authorization")+"|"+string(b))
		mu.Unlock()
		w.WriteHeader(201)
	}))
	defer epcrSrv.Close()
	ok(admin.do("PUT", "/api/admin/nemsis", map[string]any{"on": true, "url": epcrSrv.URL, "token": "tok", "agency": "A123", "agencyStateId": "VA-1", "state": "VA", "auto": true}, true))
	med, _ := s.Eng.CreateIncident(sys, cad.NewIncident{Type: "CHEST", Location: &ng911.Location{Lat: &lat, Lon: &lon, Address: "1 Main St"}})
	s.Eng.Dispatch(sys, med, []string{"M1"}, nil)
	s.Eng.SetStatus(sys, "M1", cad.Enroute, "", "")
	s.Eng.SetStatus(sys, "M1", cad.OnScene, "", "")
	code, b = d1.do("GET", "/api/incidents/"+med+"/nemsis?unit=M1", nil, false)
	if code != 200 || !strings.Contains(string(b), "<eTimes.03>") || !strings.Contains(string(b), "http://www.nemsis.org") {
		t.Fatalf("nemsis export %d %.300s", code, b)
	}
	s.Eng.SetStatus(sys, "M1", cad.Available, "", "")
	var sent *cad.EPCRRecord
	for i := 0; i < 100 && sent == nil; i++ {
		time.Sleep(50 * time.Millisecond)
		s.Eng.Read(func(st *cad.State) {
			if r := st.Incidents[med].EPCR["M1"]; r != nil {
				c := *r
				sent = &c
			}
		})
	}
	if sent == nil || !sent.OK {
		t.Fatalf("auto ePCR %+v", sent)
	}
	mu.Lock()
	if len(epcr) != 1 || !strings.HasPrefix(epcr[0], "Bearer tok|") || !strings.Contains(epcr[0], "<eResponse.03>"+med) {
		t.Fatalf("ePCR got %.300v", epcr)
	}
	mu.Unlock()

	// Unit history, as JSON and CSV.
	code, b = d1.do("GET", "/api/units/M1/history", nil, false)
	if code != 200 || !strings.Contains(string(b), `"onscene"`) {
		t.Fatalf("history %d %s", code, b)
	}
	code, b = d1.do("GET", "/api/units/M1/history?format=csv", nil, false)
	if code != 200 || !strings.HasPrefix(string(b), "unit,from,to,status") {
		t.Fatalf("csv %d %s", code, b)
	}

	// Warrants: a viewer cannot read them; the console flags the incident with the caution.
	ok(d1.do("PUT", "/api/papers", cad.Paper{Kind: "arrest", Subject: cad.PaperSubject{Name: "Jordan Q Example"}, Location: &ng911.Location{Lat: &lat, Lon: &lon, Address: "215 Church Ave SW"}, Caution: "armed"}, true))
	if code, _ := v1.do("GET", "/api/papers", nil, false); code != 403 {
		t.Fatalf("viewer papers %d", code)
	}
	if code, _ := v1.do("GET", "/api/incidents/"+num+"/papers", nil, false); code != 403 {
		t.Fatalf("viewer incident papers %d", code)
	}
	code, b = d1.do("GET", "/api/incidents/"+num+"/papers", nil, false)
	if code != 200 || !strings.Contains(string(b), "Jordan Q Example") {
		t.Fatalf("dispatcher papers %d %s", code, b)
	}
	json.Unmarshal(ok(v1.do("GET", "/api/state", nil, false)), &view)
	if f := view.Roadmap.Papers[num]; f.Count != 1 || f.Caution != "armed" {
		t.Fatalf("paper flag %+v", f)
	}
	if strings.Contains(string(ok(v1.do("GET", "/api/state", nil, false))), "Jordan Q Example") {
		t.Fatal("the console view names the person on a warrant")
	}

	// Tows by rotation.
	ok(admin.do("PUT", "/api/admin/tows", cad.TowCompany{Name: "Acme Towing", Phone: "540-555-0101", Classes: []string{"light"}}, true))
	var tw cad.Tow
	json.Unmarshal(ok(d1.do("POST", "/api/tows", map[string]any{"incident": num, "class": "light"}, true)), &tw)
	if tw.Name != "Acme Towing" {
		t.Fatalf("tow %+v", tw)
	}
	ok(d1.do("POST", "/api/tows/"+tw.ID+"/answer", map[string]any{"result": "accepted"}, true))

	// Crews from the terminal's dispatcher side, and the roster masks contact details for dispatchers.
	ok(d1.do("PUT", "/api/units/E1/crew", map[string]any{"crew": []map[string]string{{"member": "301", "role": "driver"}}}, true))
	code, b = d1.do("GET", "/api/roster", nil, false)
	if code != 200 || strings.Contains(string(b), "gus@example.com") || !strings.Contains(string(b), `"riding":"E1"`) {
		t.Fatalf("roster %d %s", code, b)
	}
}
