package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/foia"
	"github.com/biodream-llc/bitdispatcher/internal/guidance"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// Call entry over the API: duplicates are offered before a second incident, people and vehicles are kept,
// previous incidents are found by plate, CAD search finds the person, approved guidance is matched and
// recorded as shown, a newer location is offered and accepted; then the record is reviewed and released
// under a records request with its redactions.
func TestCallEntryAndRecords(t *testing.T) {
	ts, s, pw := newServer(t)
	admin := login(t, ts, "admin", pw["admin"])
	d := login(t, ts, "d1", pw["d1"])
	lat, lon := 37.2700, -79.9400
	mk := func(addr string) string {
		code, b := d.do("POST", "/api/incidents", cad.NewIncident{Type: "ROB", Modifiers: []string{"WEAPONS"}, LocVerified: true,
			Location: &ng911.Location{Lat: &lat, Lon: &lon, Address: addr}, Caller: &cad.Caller{Name: "QUICK MART", Callback: "+15405550111"}}, true)
		var x struct{ Number string }
		json.Unmarshal(b, &x)
		if code != 201 {
			t.Fatalf("create: %d %s", code, b)
		}
		return x.Number
	}
	first := mk("517 Highland Ave SW")
	// Before a second: the first is a possible duplicate.
	code, b := d.do("GET", "/api/duplicates?type=ROB&address=517+Highland+Ave+SW&lat=37.27&lon=-79.94&callback=%2B15405550111", nil, false)
	if code != 200 || !strings.Contains(string(b), first) || !strings.Contains(string(b), "same address") || !strings.Contains(string(b), "same callback") {
		t.Fatalf("duplicates: %d %s", code, b)
	}
	if code, b := d.do("POST", "/api/incidents/"+first+"/persons", cad.Person{Role: "suspect", Name: "DOE, JOHN", DOB: "1990-01-02", Clothing: "black hoodie"}, true); code != 200 {
		t.Fatalf("person: %d %s", code, b)
	}
	if code, b := d.do("POST", "/api/incidents/"+first+"/vehicles", cad.Vehicle{Role: "suspect", Plate: "KTR-4471", State: "VA", Make: "Honda", Model: "Civic"}, true); code != 200 {
		t.Fatalf("vehicle: %d %s", code, b)
	}
	second := mk("900 Other St")
	code, b = d.do("GET", "/api/previous?number="+second+"&plate=KTR-4471", nil, false)
	if code != 200 || !strings.Contains(string(b), first) || !strings.Contains(string(b), `"confidence":"confirmed"`) {
		t.Fatalf("previous: %d %s", code, b)
	}
	code, b = d.do("GET", "/api/search?q=john+doe", nil, false)
	if code != 200 || !strings.Contains(string(b), `"kind":"person"`) || !strings.Contains(string(b), `"match":"exact"`) {
		t.Fatalf("search: %d %s", code, b)
	}
	// Guidance: an approved policy for robberies with weapons.
	doc := guidance.Doc{ID: "SOP-210", Title: "Weapons calls", Kind: guidance.Policy, Version: "3", Effective: time.Now().Add(-time.Hour), Approved: true,
		Text: "# Officer safety\nApplies: ROB; modifiers: WEAPONS\nAsk where the weapon is now. Do not let the caller approach the suspect.", Source: "SOP manual §210"}
	if code, b := admin.do("PUT", "/api/admin/guidance/SOP-210", doc, true); code != 204 {
		t.Fatalf("guidance: %d %s", code, b)
	}
	code, b = d.do("GET", "/api/guidance/match?type=ROB&modifier=WEAPONS", nil, false)
	if code != 200 || !strings.Contains(string(b), "Ask where the weapon is now") || strings.Contains(string(b), `"noMatch":true`) {
		t.Fatalf("guidance match: %d %s", code, b)
	}
	if code, _ := d.do("GET", "/api/guidance/match?type=FALL", nil, false); code != 200 {
		t.Fatal("no-match")
	}
	if code, b := d.do("POST", "/api/incidents/"+first+"/guidance/shown", map[string]any{"shown": []cad.Shown{{Doc: "SOP-210", Version: "3", Anchor: "officer-safety", Heading: "Officer safety"}}}, true); code != 204 {
		t.Fatalf("shown: %d %s", code, b)
	}
	// A newer location from a provider is offered, then accepted.
	s.Eng.UpsertCall(cad.BitVoiceActor, "c9", cad.CallData{Source: "bitvoice", State: "answered", Callback: "+15405550199", Location: ng911.Point(lat, lon)})
	s.Eng.AttachCall(cad.SystemActor, "c9", first)
	ext := Extra{HookToken: "bd_hook_t"}
	s.mu.Lock()
	s.integ.Extra = ext
	s.mu.Unlock()
	hc := &client{t: t, base: ts.URL, c: ts.Client()}
	if code, b := hc.doAuth("POST", "/api/hooks/location", map[string]any{"callback": "+1 540 555 0199", "lat": lat + 0.003, "lon": lon, "radius": 5, "source": "rapidsos"}, "bd_hook_t"); code != 200 {
		t.Fatalf("location hook: %d %s", code, b)
	}
	var in cad.Incident
	s.Eng.Read(func(st *cad.State) { in = *st.Incidents[first] })
	if in.NewLocation == nil || in.NewLocation.Location.Source != "rapidsos" {
		t.Fatalf("offer: %+v", in.NewLocation)
	}
	if code, b := d.do("POST", "/api/incidents/"+first+"/location", map[string]any{"accept": true, "why": "caller moved"}, true); code != 204 {
		t.Fatalf("accept: %d %s", code, b)
	}
	// Review views.
	for _, v := range []string{"initial", "summary", "detailed", "audit"} {
		code, b := d.do("GET", "/api/review/"+first+"?view="+v, nil, false)
		if code != 200 {
			t.Fatalf("view %s: %d %s", v, code, b)
		}
		if v == "initial" && (!strings.Contains(string(b), "As submitted") || strings.Contains(string(b), "DOE, JOHN")) {
			t.Fatalf("initial shows later changes: %s", b)
		}
		if v == "detailed" && (!strings.Contains(string(b), "DOE, JOHN") || !strings.Contains(string(b), "SOP-210 version 3")) {
			t.Fatalf("detailed: %s", b)
		}
		if v == "audit" && !strings.Contains(string(b), `"ok":true`) {
			t.Fatalf("audit chain: %s", b)
		}
	}
	// A records request: a rule for names, assemble, decide, approve, released without the name.
	if code, b := admin.do("PUT", "/api/admin/foia/rules/names", foia.Rule{Name: "Names of people", Kind: foia.KindCategory, Category: "name", Basis: "Agency rule 1 (counsel approved)", Enabled: true}, true); code != 204 {
		t.Fatalf("rule: %d %s", code, b)
	}
	code, b = admin.do("POST", "/api/foia/requests", map[string]any{"requester": "A Reporter", "incidents": []string{first}, "view": "detailed"}, true)
	var rq struct{ ID string }
	json.Unmarshal(b, &rq)
	if code != 200 {
		t.Fatalf("request: %d %s", code, b)
	}
	if code, b := d.do("GET", "/api/foia/requests", nil, false); code != 403 {
		t.Fatalf("a dispatcher saw records requests: %d %s", code, b)
	}
	if code, b := admin.do("POST", "/api/foia/requests/"+rq.ID+"/assemble", nil, true); code != 204 {
		t.Fatalf("assemble: %d %s", code, b)
	}
	code, b = admin.do("GET", "/api/foia/requests/"+rq.ID, nil, false)
	var q cad.FOIARequest
	json.Unmarshal(b, &q)
	if len(q.Items) != 1 || len(q.Items[0].Proposals) == 0 {
		t.Fatalf("proposals: %s", b)
	}
	if code, _ := admin.do("POST", "/api/foia/requests/"+rq.ID+"/approve", nil, true); code != 422 {
		t.Fatalf("approved with undecided proposals: %d", code)
	}
	var ds []foia.Decision
	for _, p := range q.Items[0].Proposals {
		ds = append(ds, foia.Decision{ID: p.ID, Start: p.Start, End: p.End, Text: p.Text, Accept: true, Basis: p.Basis})
	}
	if code, b := admin.do("POST", "/api/foia/requests/"+rq.ID+"/decide", map[string]any{"incident": first, "decisions": ds}, true); code != 204 {
		t.Fatalf("decide: %d %s", code, b)
	}
	if code, b := admin.do("POST", "/api/foia/requests/"+rq.ID+"/approve", nil, true); code != 204 {
		t.Fatalf("approve: %d %s", code, b)
	}
	code, b = admin.do("GET", "/api/foia/requests/"+rq.ID+"/released/"+first, nil, false)
	if code != 200 || strings.Contains(string(b), "DOE, JOHN") || !strings.Contains(string(b), "[REDACTED: Agency rule 1") {
		t.Fatalf("released: %d %s", code, b)
	}
}

func (c *client) doAuth(method, path string, body any, token string) (int, []byte) {
	c.t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, c.base+path, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.c.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}
