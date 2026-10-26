package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"io"
	"log/slog"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ledger"
	"github.com/biodream-llc/bitdispatcher/internal/mapdata"
)

// Two centres running bitdispatcher: A is overwhelmed and asks B to take its calls; B's supervisor
// accepts; B's call taker enters a call that becomes A's incident; B adds a note and assigns one of A's
// units; what happens at A comes back to B; a call taken while A cannot be reached is kept and delivered
// later; ending the overflow stops calls.
func TestOverflowBetweenTwoCentres(t *testing.T) {
	tsA, sA, pwA := newServer(t)
	tsB, sB, pwB := newServer(t)
	overflowClient = &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	adminA, adminB := login(t, tsA, "admin", pwA["admin"]), login(t, tsB, "admin", pwB["admin"])
	dB := login(t, tsB, "d1", pwB["d1"])
	put := func(c *client, path string, body any) {
		t.Helper()
		if code, b := c.do("PUT", path, body, true); code != 204 {
			t.Fatalf("%s: %d %s", path, code, b)
		}
	}
	put(adminA, "/api/admin/partners/bco", map[string]any{"name": "B County 911", "url": tsB.URL + "/api/cad/eido/ACO", "outToken": "a-to-b", "inToken": "b-to-a"})
	put(adminB, "/api/admin/partners/aco", map[string]any{"name": "A County 911", "url": tsA.URL + "/api/cad/eido/BCO", "outToken": "b-to-a", "inToken": "a-to-b"})
	put(adminA, "/api/admin/partners/BCO/overflow", PartnerOverflow{Ask: true, Dispatch: true})

	// B has not agreed to help: A's request is refused there.
	if code, b := adminA.do("POST", "/api/overflow/BCO/ask", map[string]any{"note": "Storm: 40 calls waiting"}, true); code != 502 || !strings.Contains(string(b), "not set up") {
		t.Fatalf("ask before B agreed: %d %s", code, b)
	}
	put(adminB, "/api/admin/partners/ACO/overflow", PartnerOverflow{Help: true})
	// A dispatcher may not ask; a supervisor may.
	dA := login(t, tsA, "d1", pwA["d1"])
	if code, _ := dA.do("POST", "/api/overflow/BCO/ask", map[string]any{}, true); code != 403 {
		t.Fatalf("a dispatcher asked: %d", code)
	}
	if code, b := adminA.do("POST", "/api/overflow/BCO/ask", map[string]any{"note": "Storm: 40 calls waiting", "minutes": 120}, true); code != 200 || !strings.Contains(string(b), `"status":"asked"`) {
		t.Fatalf("ask: %d %s", code, b)
	}
	// B sees the request, and a call cannot be taken before it is accepted.
	if code, _ := dB.do("POST", "/api/overflow/ACO/calls", map[string]any{"type": "SF", "address": "1 Main St"}, true); code != 409 {
		t.Fatalf("call before accepting: %d", code)
	}
	if code, b := adminB.do("POST", "/api/overflow/ACO/answer", map[string]any{"accept": true}, true); code != 200 {
		t.Fatalf("accept: %d %s", code, b)
	}
	var out cad.OverflowSession
	sA.Eng.Read(func(st *cad.State) { out = *st.OverflowFor("out", "BCO") })
	if out.Status != "active" || !out.Dispatch || len(out.CallTypes) == 0 {
		t.Fatalf("A's session %+v", out)
	}
	if h := sB.continuityHealth(); len(h.Overflow) != 1 || h.Overflow[0].Direction != "in" {
		t.Fatalf("B's banner %+v", h.Overflow)
	}

	// B's call taker takes a call for A: it becomes A's incident, numbered by A.
	code, b := dB.do("POST", "/api/overflow/ACO/calls", map[string]any{"type": "SF", "address": "200 Oak Ave", "callerName": "Pat Caller", "callback": "5405550100",
		"narrative": "Smoke from the roof"}, true)
	var c cad.OverflowCall
	json.Unmarshal(b, &c)
	if code != 200 || !c.Delivered || c.Number == "" {
		t.Fatalf("call: %d %s", code, b)
	}
	var in *cad.Incident
	sA.Eng.Read(func(st *cad.State) {
		if st.OverflowIncidents[c.Number] != nil {
			x := *st.Incidents[c.Number]
			in = &x
		}
	})
	if in == nil || in.Location == nil || in.Location.Address != "200 Oak Ave" || in.Caller == nil || in.Caller.Callback != "5405550100" {
		t.Fatalf("A's incident %+v", in)
	}
	if !strings.Contains(timelineText(in), "Taken for us by B County 911 (overflow), call taker d1") {
		t.Fatalf("A's timeline: %s", timelineText(in))
	}
	// The same call delivered again (a retry after a lost answer) does not make a second incident.
	if n, _ := sA.takeCall(mustPartner(t, sA, "BCO"), callMsg{Ref: c.Ref, Type: "SF", Address: "200 Oak Ave", By: "d1"}); n != c.Number {
		t.Fatalf("a repeat made %s", n)
	}

	// A note from B reaches A's incident.
	if code, b := dB.do("POST", "/api/overflow/calls/"+c.Ref+"/notes", map[string]string{"text": "Caller says everyone is out"}, true); code != 200 {
		t.Fatalf("note: %d %s", code, b)
	}
	sA.Eng.Read(func(st *cad.State) { x := *st.Incidents[c.Number]; in = &x })
	if !strings.Contains(timelineText(in), "[B County 911, d1] Caller says everyone is out") {
		t.Fatalf("A's timeline after the note: %s", timelineText(in))
	}

	// B assigns one of A's units, which A shared.
	if err := sA.Eng.SaveUnit(cad.SystemActor, cad.UnitDef{ID: "E1", Type: "engine"}); err != nil {
		t.Fatal(err)
	}
	if err := sA.Eng.SetStatus(cad.SystemActor, "E1", cad.Available, "", ""); err != nil {
		t.Fatal(err)
	}
	var units []map[string]string
	code, b = dB.do("GET", "/api/overflow/ACO/units", nil, false)
	json.Unmarshal(b, &units)
	if code != 200 || len(units) == 0 {
		t.Fatalf("A's units: %d %s", code, b)
	}
	var free string
	for _, u := range units {
		if u["status"] == cad.Available && u["type"] == "engine" {
			free = u["id"]
			break
		}
	}
	if free == "" {
		t.Fatalf("no available engine in %v", units)
	}
	if code, b := dB.do("POST", "/api/overflow/calls/"+c.Ref+"/dispatch", map[string]any{"units": []string{free}}, true); code != 204 {
		t.Fatalf("dispatch: %d %s", code, b)
	}
	// A's news comes back to B: the unit, then the close.
	ctx := context.Background()
	if err := sA.pushUpdate(ctx, c.Number); err != nil {
		t.Fatal(err)
	}
	got, _ := sB.Eng.OverflowCallByRef(c.Ref)
	if len(got.Units) != 1 || got.Units[0].Unit != free || got.Status == "" {
		t.Fatalf("B's copy after dispatch %+v", got)
	}
	var disp string
	sA.Eng.Read(func(st *cad.State) { disp = st.Settings.Dispositions[0] })
	if err := sA.Eng.CloseIncident(cad.SystemActor, c.Number, disp, "Out on arrival"); err != nil {
		t.Fatal(err)
	}
	sA.pushUpdate(ctx, c.Number)
	if got, _ = sB.Eng.OverflowCallByRef(c.Ref); !got.Closed || got.Disposition != disp {
		t.Fatalf("B's copy after close %+v", got)
	}

	// A cannot be reached (its token is changed, as when its server is down): the call is kept at B,
	// then delivered when A answers again.
	put(adminA, "/api/admin/partners/bco", map[string]any{"name": "B County 911", "url": tsB.URL + "/api/cad/eido/ACO", "inToken": "changed"})
	_, b = dB.do("POST", "/api/overflow/ACO/calls", map[string]any{"type": "SF", "address": "9 Pine St"}, true)
	var late cad.OverflowCall
	json.Unmarshal(b, &late)
	if late.Delivered || late.Error == "" || late.Ref == "" {
		t.Fatalf("an undeliverable call %s", b)
	}
	put(adminA, "/api/admin/partners/bco", map[string]any{"name": "B County 911", "url": tsB.URL + "/api/cad/eido/ACO", "inToken": "b-to-a"})
	if got := sB.deliverCall(ctx, mustPartner(t, sB, "ACO"), late); !got.Delivered || got.Number == "" {
		t.Fatalf("the retry %+v", got)
	}

	// A ends the overflow: B is told, and takes no more calls for A.
	if code, b := adminA.do("POST", "/api/overflow/BCO/end", map[string]string{"direction": "out"}, true); code != 200 || strings.Contains(string(b), "warning") {
		t.Fatalf("end: %d %s", code, b)
	}
	if code, _ := dB.do("POST", "/api/overflow/ACO/calls", map[string]any{"type": "SF", "address": "1 Main St"}, true); code != 409 {
		t.Fatalf("a call after the end: %d", code)
	}
	// Neither side's other incidents crossed: B has no incidents at all.
	sB.Eng.Read(func(st *cad.State) {
		if len(st.Incidents) != 0 {
			t.Fatalf("B has %d incidents", len(st.Incidents))
		}
	})
}

func timelineText(in *cad.Incident) string {
	var s []string
	for _, e := range in.Timeline {
		s = append(s, e.Text)
	}
	return strings.Join(s, " | ")
}

func mustPartner(t *testing.T, s *Server, id string) Partner {
	t.Helper()
	p, ok := s.partner(id)
	if !ok {
		t.Fatalf("no partner %s", id)
	}
	return p
}

// Continuity: the map schedule's window, the links watched, relocation with its checklist, the pack.
func TestContinuity(t *testing.T) {
	ts, s, pw := newServer(t)
	admin := login(t, ts, "admin", pw["admin"])
	d := login(t, ts, "d1", pw["d1"])
	var st cad.Settings
	s.Eng.Read(func(x *cad.State) { st = x.Settings })
	// Bad settings are refused.
	bad := st
	bad.MapUpdates = cad.MapSchedule{Mode: "hourly"}
	if code, _ := admin.do("PUT", "/api/admin/settings", bad, true); code != 422 {
		t.Fatalf("hourly accepted: %d", code)
	}
	bad = st
	bad.Continuity.Checks = []cad.ConnCheck{{Name: "x", URL: "http://example.com"}}
	if code, _ := admin.do("PUT", "/api/admin/settings", bad, true); code != 422 {
		t.Fatalf("an http check accepted: %d", code)
	}
	// The internet check: a test server that answers is up; one that is closed is down.
	up := ts.URL + "/api/health"
	st.MapUpdates = cad.MapSchedule{Mode: "weekly", Hour: 3, Weekday: 0, MaxDropPct: 15}
	st.Continuity = cad.ContinuityPolicy{Checks: []cad.ConnCheck{{Name: "own server", URL: up}}, Steps: []string{"Call the phone company to reroute 911", "Sign in at the backup site"},
		Contacts: []string{"Backup site: 540-555-0199"}, PackDirs: []string{filepath.Join(t.TempDir(), "share")}}
	if code, b := admin.do("PUT", "/api/admin/settings", st, true); code != 200 && code != 204 {
		t.Fatalf("settings: %d %s", code, b)
	}
	linkClient = &http.Client{Timeout: 5 * time.Second, Transport: ts.Client().Transport}
	s.checkLinks(context.Background(), st.Continuity)
	if h := s.continuityHealth(); h.Internet != "up" || len(h.Down) != 0 {
		t.Fatalf("health %+v", h)
	}
	st.Continuity.Checks[0].URL = "https://127.0.0.1:1/"
	s.checkLinks(context.Background(), st.Continuity)
	h := s.continuityHealth()
	if h.Internet != "down" || len(h.Down) != 1 {
		t.Fatalf("health with the line down %+v", h)
	}
	// The same state again writes nothing; coming back closes the outage.
	var before uint64
	s.Eng.Read(func(x *cad.State) { before = x.Seq })
	s.checkLinks(context.Background(), st.Continuity)
	s.Eng.Read(func(x *cad.State) {
		if x.Seq != before {
			t.Fatalf("an unchanged link wrote %d events", x.Seq-before)
		}
	})
	st.Continuity.Checks[0].URL = up
	s.checkLinks(context.Background(), st.Continuity)
	s.Eng.Read(func(x *cad.State) {
		if len(x.Outages) != 1 || x.Outages[0].To.IsZero() {
			t.Fatalf("outages %+v", x.Outages)
		}
	})

	// The schedule: due in its hour on its day only, and once per window.
	loc := st.Shift.Location()
	sun3 := time.Date(2026, 11, 1, 3, 20, 0, 0, loc) // a Sunday
	if !st.MapUpdates.Due(sun3) || st.MapUpdates.Due(sun3.Add(time.Hour)) || st.MapUpdates.Due(sun3.AddDate(0, 0, 1)) {
		t.Fatal("weekly window wrong")
	}
	if n := st.MapUpdates.Next(sun3); !n.Equal(time.Date(2026, 11, 8, 3, 0, 0, 0, loc)) {
		t.Fatalf("next %v", n)
	}
	// No map is recorded here, so the run is skipped, and recorded once for the window.
	s.Maps = mapdata.NewService(t.TempDir(), "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.mapTick(sun3)
	s.mapTick(sun3.Add(10 * time.Minute))
	evs, _ := s.Eng.Events(0, 10000, func(e ledger.Event) bool { return e.Type == cad.EvMapChecked })
	if len(evs) != 1 || !strings.Contains(string(evs[0].Data), "skipped") {
		t.Fatalf("the window ran %d times: %v", len(evs), evs)
	}
	if h := s.mapAutoHealth(); h.Result != "skipped" || h.Next == nil {
		t.Fatalf("map health %+v", h)
	}

	// Relocation: only a supervisor declares it; the checklist ticks; the board shows it.
	if code, _ := d.do("POST", "/api/continuity/relocation", map[string]any{"active": true}, true); code != 403 {
		t.Fatalf("a dispatcher declared relocation: %d", code)
	}
	if code, b := admin.do("POST", "/api/continuity/relocation", map[string]any{"active": true, "note": "Smoke in the building"}, true); code != 204 {
		t.Fatalf("relocate: %d %s", code, b)
	}
	if code, _ := admin.do("POST", "/api/continuity/relocation/steps/1", map[string]bool{"done": true}, true); code != 204 {
		t.Fatalf("tick: %d", code)
	}
	if h := s.continuityHealth(); h.Relocation == nil || len(h.Relocation.Done) != 1 || h.Relocation.Done[0] != 1 {
		t.Fatalf("relocation %+v", h.Relocation)
	}
	if code, b := d.do("GET", "/api/continuity", nil, false); code != 200 || !strings.Contains(string(b), `"steps":["Call the phone company`) {
		t.Fatalf("continuity page: %d %s", code, b)
	}

	// The pack: an open incident with its caller, the checklist and contacts; written to the folder.
	d.do("POST", "/api/incidents", map[string]any{"type": "SF", "location": map[string]any{"address": "12 Elm St", "lat": 37.27, "lon": -79.94},
		"caller": map[string]string{"name": "Sam", "callback": "5405550123"}}, true)
	code, b := d.do("GET", "/api/continuity/pack", nil, false)
	if code != 200 || !strings.Contains(string(b), "12 Elm St") || !strings.Contains(string(b), "5405550123") || !strings.Contains(string(b), "Call the phone company") ||
		!strings.Contains(string(b), "Smoke in the building") {
		t.Fatalf("pack: %d %s", code, b)
	}
	if code, b := admin.do("POST", "/api/continuity/pack/write", nil, true); code != 200 || strings.Contains(string(b), `"errors":["`) {
		t.Fatalf("write: %d %s", code, b)
	}
	f := filepath.Join(st.Continuity.PackDirs[0], "continuity-pack.html")
	if fi, err := os.Stat(f); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("pack file %v %v", fi, err)
	}
	if code, _ := admin.do("POST", "/api/continuity/relocation", map[string]any{"active": false}, true); code != 204 {
		t.Fatalf("back: %d", code)
	}
}
