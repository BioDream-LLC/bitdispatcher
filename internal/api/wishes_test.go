package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// The operational wishes through the API: agencies and who may change what, presets kept with the
// account, supervisory views, communications with attachments, working calls, search, the one-line
// commands and staffing.
func TestOperationalWishesAPI(t *testing.T) {
	ts, s, pw := newServer(t)
	for _, u := range [][2]string{{"county", auth.Admin}, {"sup", auth.Supervisor}, {"sup2", auth.Supervisor}, {"tim", auth.Dispatcher}} {
		pw[u[0]] = auth.GeneratePassword()
		s.Auth.Save(u[0], u[0], u[1], pw[u[0]], false)
	}
	admin := login(t, ts, "admin", pw["admin"])
	ok := func(code int, b []byte) []byte {
		t.Helper()
		if code >= 300 {
			t.Fatalf("%d %s", code, b)
		}
		return b
	}
	// Agencies, and a county administrator limited to the county.
	ok(admin.do("PUT", "/api/admin/agencies/RPD", cad.Agency{Name: "City police", Disciplines: []string{"police"}, IncidentPrefix: "RPD"}, true))
	ok(admin.do("PUT", "/api/admin/agencies/RCPD", cad.Agency{Name: "County police", Disciplines: []string{"police"}}, true))
	ok(admin.do("PUT", "/api/admin/units/2A11", cad.UnitDef{Type: "patrol", Agency: "RPD"}, true))
	ok(admin.do("PUT", "/api/admin/units/C21", cad.UnitDef{Type: "patrol", Agency: "RCPD"}, true))
	ok(admin.do("PUT", "/api/admin/staff/county", cad.Staff{Agencies: []string{"RCPD"}, Home: "RCPD"}, true))
	county := login(t, ts, "county", pw["county"])
	if code, _ := county.do("PUT", "/api/admin/units/2A11", cad.UnitDef{Type: "patrol", Agency: "RPD", Name: "changed"}, true); code != 403 {
		t.Fatalf("the county changed a city unit: %d", code)
	}
	if code, _ := county.do("PUT", "/api/admin/units/2A11", cad.UnitDef{Type: "patrol", Agency: "RCPD"}, true); code != 403 {
		t.Fatalf("the county took a city unit: %d", code)
	}
	ok(county.do("PUT", "/api/admin/units/C21", cad.UnitDef{Type: "patrol", Agency: "RCPD", Name: "County 21"}, true))
	if code, _ := county.do("PUT", "/api/admin/calltypes/XX", cad.CallType{Name: "Centre-wide", Priority: 3, Discipline: "police"}, true); code != 403 {
		t.Fatalf("the county changed centre-wide configuration: %d", code)
	}
	var av AgencyView
	json.Unmarshal(ok(county.do("GET", "/api/agencies", nil, false)), &av)
	if av.Coverage.Working != "RCPD" || len(av.May) != 1 {
		t.Fatalf("county coverage %+v", av)
	}

	// Supervisory views: the ledger only for sup; sup2 is refused and the refusal recorded.
	var set cad.Settings
	s.Eng.Read(func(st *cad.State) { set = st.Settings })
	set.Views = []cad.ViewRule{{ID: "ledger", Roles: []string{}, Users: []string{"sup"}}}
	ok(admin.do("PUT", "/api/admin/settings", set, true))
	sup, sup2 := login(t, ts, "sup", pw["sup"]), login(t, ts, "sup2", pw["sup2"])
	ok(sup.do("GET", "/api/ledger", nil, false))
	if code, _ := sup2.do("GET", "/api/ledger?limit=5", nil, false); code != 403 {
		t.Fatalf("a supervisor without the view read the ledger: %d", code)
	}
	var views struct{ Items []cad.ViewDef }
	json.Unmarshal(ok(sup2.do("GET", "/api/me/views", nil, false)), &views)
	for _, v := range views.Items {
		if v.ID == "ledger" {
			t.Fatal("the ledger view is offered to sup2")
		}
	}

	// Presets follow the person to another console; a trainee works the assigned one.
	d1 := login(t, ts, "d1", pw["d1"])
	ws := json.RawMessage(`{"root":{"t":"leaf","s":"police"},"cmd":true,"locked":true}`)
	ok(d1.do("PUT", "/api/me/preset", cad.Preset{Duty: "dispatcher", Workspace: ws}, true))
	d1b := login(t, ts, "d1", pw["d1"])
	var po struct {
		Preset *cad.Preset
		Source string
	}
	json.Unmarshal(ok(d1b.do("GET", "/api/me/preset?duty=dispatcher", nil, false)), &po)
	if po.Source != "own" || string(po.Preset.Workspace) != string(ws) {
		t.Fatalf("preset at another console %+v", po)
	}
	ok(admin.do("PUT", "/api/admin/presets/assigned/dispatcher", cad.Preset{Workspace: json.RawMessage(`{"root":{"t":"leaf","s":"incident"},"cmd":true,"locked":true}`)}, true))
	ok(admin.do("PUT", "/api/admin/staff/tim", cad.Staff{Trainee: true}, true))
	tim := login(t, ts, "tim", pw["tim"])
	json.Unmarshal(ok(tim.do("GET", "/api/me/preset?duty=dispatcher", nil, false)), &po)
	if po.Source != "assigned" {
		t.Fatalf("trainee preset %+v", po)
	}
	if code, _ := tim.do("PUT", "/api/me/preset", cad.Preset{Duty: "dispatcher", Workspace: ws}, true); code != 422 {
		t.Fatalf("a trainee saved a preset: %d", code)
	}

	// A BOLO with a picture, to the city's positions and terminals.
	req, _ := http.NewRequest("POST", ts.URL+"/api/comms/files?name=suspect.png", bytes.NewReader(append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)))
	req.Header.Set(csrfHeader, "1")
	resp, err := d1.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	fb, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("attachment %d %s", resp.StatusCode, fb)
	}
	var f cad.CommFile
	json.Unmarshal(fb, &f)
	var res struct{ ID string }
	json.Unmarshal(ok(d1.do("POST", "/api/comms", cad.Comm{Kind: "bolo", Subject: "Robbery suspect", Body: "Silver Civic ABC1234, male in red hoodie", Plate: "ABC1234", Agency: "RPD", Files: []string{f.ID}}, true)), &res)
	var inbox struct{ Items []CommView }
	json.Unmarshal(ok(sup.do("GET", "/api/comms?folder=bolos", nil, false)), &inbox)
	if len(inbox.Items) != 1 || inbox.Items[0].Thread != res.ID {
		t.Fatalf("active BOLOs %+v", inbox.Items)
	}
	if code, _ := county.do("GET", "/api/comms/"+res.ID, nil, false); code != 404 {
		t.Fatalf("the county read a city BOLO: %d", code)
	}
	ok(sup.do("GET", "/api/comms/files/"+f.ID, nil, false))
	ok(sup.do("POST", "/api/comms/"+res.ID+"/ack", nil, true))

	// Working calls: three at most for a call-taker; each keeps its own contents.
	var ids []string
	for i := 0; i < 2; i++ {
		var x struct{ ID string }
		json.Unmarshal(ok(d1.do("POST", "/api/drafts", map[string]any{}, true)), &x)
		ids = append(ids, x.ID)
		ok(d1.do("PUT", "/api/drafts/"+x.ID, map[string]any{"address": "draft " + x.ID}, true))
	}
	var dl struct {
		Items []DraftView
		Max   int
	}
	json.Unmarshal(ok(d1b.do("GET", "/api/drafts", nil, false)), &dl)
	if len(dl.Items) != 2 || !strings.Contains(string(dl.Items[1].Content), ids[1]) {
		t.Fatalf("drafts %+v", dl)
	}
	if code, _ := sup2.do("PUT", "/api/drafts/"+ids[0], map[string]any{"address": "x"}, true); code != 403 {
		t.Fatalf("someone else wrote in a working call: %d", code)
	}
	ok(d1.do("POST", "/api/drafts/"+ids[0]+"/close", map[string]any{"how": "discarded", "why": "duplicate", "populated": true}, true))

	// One-line commands: F7 and F11.
	ok(admin.do("PUT", "/api/admin/units/UNIT1", cad.UnitDef{Type: "patrol", Agency: "RPD"}, true))
	ok(admin.do("POST", "/api/command", map[string]string{"line": "AV UNIT1"}, true))
	var cr cad.CommandResult
	json.Unmarshal(ok(d1.do("POST", "/api/command?key=F7", map[string]string{"line": "FI UNIT1.8TH/STEWART..ABC1234"}, true)), &cr)
	if !cr.OK || !strings.HasPrefix(cr.Incident, "RPD") {
		t.Fatalf("F7 %+v", cr)
	}
	ct := login(t, ts, "tim", pw["tim"])
	json.Unmarshal(ok(ct.do("POST", "/api/command?key=F11", map[string]string{"line": "US UNIT1.10-99"}, true)), &cr)
	if cr.OK || !cr.Refused {
		t.Fatalf("unknown code %+v", cr)
	}

	// Search: a plate finds the stop's vehicle and the BOLO; the search is recorded.
	var found cad.FindResults
	json.Unmarshal(ok(d1.do("POST", "/api/search", cad.FindQuery{Text: "ABC1234"}, true)), &found)
	if found.Counts["vehicles"] < 2 || found.Counts["comms"] != 1 {
		t.Fatalf("search %+v", found.Counts)
	}
	var cfound cad.FindResults
	json.Unmarshal(ok(county.do("POST", "/api/search", cad.FindQuery{Text: "ABC1234"}, true)), &cfound)
	if cfound.Counts["comms"] != 0 || cfound.Counts["vehicles"] != 0 {
		t.Fatalf("the county found the city's records %+v", cfound.Counts)
	}

	// Staffing: on duty, the coverage view.
	ok(sup.do("PUT", "/api/admin/members", cad.Member{ID: "501", Name: "Pat Officer"}, true))
	ok(d1.do("POST", "/api/members/501/on", cad.DutyPeriod{Kind: "overtime"}, true))
	var cov struct{ Units []map[string]any }
	json.Unmarshal(ok(sup.do("GET", "/api/staffing/coverage", nil, false)), &cov)
	if len(cov.Units) == 0 {
		t.Fatal("coverage view empty")
	}

	evs, _ := s.Eng.Events(0, 0, func(e ledger.Event) bool {
		return e.Type == cad.EvAccessRefused || e.Type == cad.EvSearched || e.Type == cad.EvCommandRefused || e.Type == cad.EvPresetSaved
	})
	kinds := map[string]int{}
	for _, e := range evs {
		kinds[e.Type]++
		if e.Type == cad.EvSearched && e.Actor == "d1" && e.Agency == "" {
			// d1 has no agencies limit; fine.
		}
	}
	if kinds[cad.EvAccessRefused] < 3 || kinds[cad.EvSearched] != 2 || kinds[cad.EvCommandRefused] != 1 || kinds[cad.EvPresetSaved] < 2 {
		t.Fatalf("ledger %v", kinds)
	}
}
