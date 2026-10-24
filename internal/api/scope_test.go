package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// A dispatcher limited to the city sees only the city's part of the shared board, and is refused another
// agency's incidents and units by path or on the command line; narrowing the board to coverage is a choice.
func TestBoardScopeAPI(t *testing.T) {
	ts, s, pw := newServer(t)
	pw["tim"] = auth.GeneratePassword()
	s.Auth.Save("tim", "tim", auth.Dispatcher, pw["tim"], false)
	admin := login(t, ts, "admin", pw["admin"])
	ok := func(code int, b []byte) []byte {
		t.Helper()
		if code >= 300 {
			t.Fatalf("%d %s", code, b)
		}
		return b
	}
	ok(admin.do("PUT", "/api/admin/agencies/RPD", cad.Agency{Name: "City police", Disciplines: []string{"police"}}, true))
	ok(admin.do("PUT", "/api/admin/agencies/RCPD", cad.Agency{Name: "County police", Disciplines: []string{"police"}}, true))
	ok(admin.do("PUT", "/api/admin/units/2A11", cad.UnitDef{Type: "patrol", Agency: "RPD"}, true))
	ok(admin.do("PUT", "/api/admin/units/C21", cad.UnitDef{Type: "patrol", Agency: "RCPD"}, true))
	ok(admin.do("PUT", "/api/admin/staff/tim", cad.Staff{Agencies: []string{"RPD"}, Home: "RPD"}, true))
	county, err := s.Eng.CreateIncident(cad.SystemActor, cad.NewIncident{Type: "CHEST"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Eng.SetIncidentAgency(cad.SystemActor, county, "RCPD", nil, "county"); err != nil {
		t.Fatal(err)
	}
	city, _ := s.Eng.CreateIncident(cad.SystemActor, cad.NewIncident{Type: "CHEST"})
	s.Eng.SetIncidentAgency(cad.SystemActor, city, "RPD", nil, "city")

	tim := login(t, ts, "tim", pw["tim"])
	var v struct {
		Units     []struct{ ID string }
		Incidents []struct{ Number string }
	}
	json.Unmarshal(ok(tim.do("GET", "/api/state", nil, false)), &v)
	has := func(id string) bool {
		for _, u := range v.Units {
			if u.ID == id {
				return true
			}
		}
		for _, in := range v.Incidents {
			if in.Number == id {
				return true
			}
		}
		return false
	}
	if has("C21") || has(county) || !has("2A11") || !has(city) {
		t.Fatalf("tim's board %+v", v)
	}
	if code, _ := tim.do("GET", "/api/incidents/"+county+"/events", nil, false); code != 403 {
		t.Fatalf("tim read a county incident: %d", code)
	}
	if code, _ := tim.do("POST", "/api/units/C21/status", map[string]string{"status": "available"}, true); code != 403 {
		t.Fatalf("tim changed a county unit: %d", code)
	}
	_, b := tim.do("POST", "/api/command", map[string]string{"line": "OS C21"}, true)
	if !strings.Contains(string(b), `"refused":true`) {
		t.Fatalf("command on a county unit: %s", b)
	}
	ok(tim.do("GET", "/api/incidents/"+city+"/events", nil, false))
	// The administrator sees everything; narrowing to coverage hides the rest of the board for them only.
	json.Unmarshal(ok(admin.do("GET", "/api/state", nil, false)), &v)
	if !has("C21") || !has(county) {
		t.Fatal("the administrator's board is scoped")
	}
	ok(admin.do("PUT", "/api/me/coverage", cad.WorkCoverage{Working: "RPD", Scope: cad.ScopeCoverage}, true))
	v.Units, v.Incidents = nil, nil
	json.Unmarshal(ok(admin.do("GET", "/api/state", nil, false)), &v)
	if has("C21") || has(county) {
		t.Fatal("narrowing to coverage did not hide the county")
	}
	ok(admin.do("GET", "/api/incidents/"+county+"/events", nil, false)) // still allowed: only the view narrowed
	if b := ok(admin.do("GET", "/api/ledger?limit=200", nil, false)); !strings.Contains(string(b), "another agency") {
		t.Fatal("the refusals are not in the ledger")
	}
}
