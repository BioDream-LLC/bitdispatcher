package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// The final round through the API: a forced location alerts the supervisors, protected identifiers are
// masked on the shared board, a record is run through the simulator and its possible hit confirmed, an
// open request blocks signing off, and Street View says plainly when it is not set up.
func TestFinalWishesAPI(t *testing.T) {
	ts, s, pw := newServer(t)
	pw["sup"] = auth.GeneratePassword()
	s.Auth.Save("sup", "Sup", auth.Supervisor, pw["sup"], false)
	ok := func(code int, b []byte) []byte {
		t.Helper()
		if code >= 300 {
			t.Fatalf("%d %s", code, b)
		}
		return b
	}
	admin := login(t, ts, "admin", pw["admin"])
	var set cad.Settings
	s.Eng.Read(func(st *cad.State) { set = st.Settings })
	set.NCICMode = "sim"
	ok(admin.do("PUT", "/api/admin/settings", set, true))
	d1 := login(t, ts, "d1", pw["d1"])
	lat, lon := 37.27, -79.94
	num, err := s.Eng.CreateIncident(cad.Actor{User: "d1", Role: "dispatcher"}, cad.NewIncident{Type: "FALL", Location: &ng911.Location{Address: "1 NEW ST", Lat: &lat, Lon: &lon}})
	if err != nil {
		t.Fatal(err)
	}
	var px struct{ Exception string }
	json.Unmarshal(ok(d1.do("PUT", "/api/incidents/"+num+"/place", cad.Place{Verify: cad.PlaceForced, Entered: "1 New St", Reason: cad.DefaultForceReasons[0], Lat: &lat, Lon: &lon}, true)), &px)
	if px.Exception == "" {
		t.Fatal("no exception")
	}
	var alerted bool
	s.Eng.Read(func(st *cad.State) {
		for _, c := range st.Comms {
			alerted = alerted || (strings.Contains(c.Subject, px.Exception) && c.Priority == "high")
		}
	})
	if !alerted {
		t.Fatal("supervisors not alerted")
	}
	if code, _ := d1.do("GET", "/api/exceptions", nil, false); code != 403 {
		t.Fatalf("a dispatcher read the exceptions: %d", code)
	}
	sup := login(t, ts, "sup", pw["sup"])
	ok(sup.do("GET", "/api/exceptions", nil, false))
	// A person with a Social Security number: masked on the board.
	pid, err := s.Eng.SavePerson(cad.Actor{User: "d1", Role: "dispatcher"}, num, cad.Person{Role: "subject", First: "Test", Last: "Person", DOB: "1980-01-01", SSN: "123-45-6789"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.view()
	if strings.Contains(string(b), "123-45-6789") || !strings.Contains(string(b), "***-**-6789") {
		t.Fatal("the board shows the full SSN")
	}
	// Run a vehicle through the simulator: a TEST plate is a possible stolen hit, owner on the return.
	vid, err := s.Eng.SaveVehicle(cad.Actor{User: "d1", Role: "dispatcher"}, num, cad.Vehicle{Role: "suspect", Plate: "TEST123", Color: "BLU"})
	if err != nil {
		t.Fatal(err)
	}
	var run struct {
		Request, Return, Hit string
		Simulated            bool
	}
	json.Unmarshal(ok(d1.do("POST", "/api/incidents/"+num+"/records/"+vid+"/run", map[string]string{"requester": "ad6"}, true)), &run)
	if run.Hit != "stolen" || !run.Simulated || run.Request == "" {
		t.Fatalf("run %+v", run)
	}
	var rets struct {
		Returns       []cad.InqReturn
		Discrepancies []string
	}
	json.Unmarshal(ok(d1.do("GET", "/api/incidents/"+num+"/records/"+vid+"/returns", nil, false)), &rets)
	if len(rets.Returns) != 1 || rets.Returns[0].Fields["owner"] != "DOE, JANE A" || len(rets.Discrepancies) != 1 {
		t.Fatalf("returns %+v", rets)
	}
	b, _ = s.view()
	if strings.Contains(string(b), "DOE, JANE A") {
		t.Fatal("the board carries the return's content")
	}
	var hc struct{ ID string }
	json.Unmarshal(ok(d1.do("POST", "/api/incidents/"+num+"/records/"+vid+"/hit-confirmation", map[string]any{"returnId": run.Return, "requester": "AD6"}, true)), &hc)
	// Signing off with open requests is refused until they are finished or handed over.
	if code, body := d1.do("POST", "/api/auth/logout", nil, true); code != 409 || !strings.Contains(string(body), "hand it") {
		t.Fatalf("logout with open requests: %d %s", code, body)
	}
	ok(d1.do("POST", "/api/requests/"+hc.ID, cad.RequestUpdate{Status: cad.RqConfirmed, Responder: "Test PD records"}, true))
	var other struct{ ID string }
	json.Unmarshal(ok(d1.do("POST", "/api/requests", cad.Request{Type: "dmv", Category: "plate", Subject: "XYZ VA"}, true)), &other)
	if code, _ := d1.do("POST", "/api/auth/logout", nil, true); code != 409 {
		t.Fatal("signed off with an open request")
	}
	ok(d1.do("POST", "/api/requests/"+other.ID+"/handoff", map[string]string{"to": "sup", "note": "awaiting DMV"}, true))
	ok(d1.do("POST", "/api/auth/logout", nil, true))
	// Street View without a key: plainly not set up.
	var sv map[string]any
	json.Unmarshal(ok(sup.do("GET", "/api/views/streetview?lat=37.27&lon=-79.94", nil, false)), &sv)
	if sv["available"] != nil || !strings.Contains(sv["why"].(string), "not set up") {
		t.Fatalf("street view %+v", sv)
	}
	_ = pid
}
