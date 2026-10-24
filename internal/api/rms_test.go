package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ncic"
)

func TestRMSAndNCIC(t *testing.T) {
	ts, s, pw := newServer(t)
	d := login(t, ts, "d1", pw["d1"])
	// A report from an incident: submitting checks the NIBRS data elements first.
	code, b := d.do("POST", "/api/incidents", cad.NewIncident{Type: "THEFT"}, true)
	var inc struct{ Number string }
	json.Unmarshal(b, &inc)
	code, b = d.do("POST", "/api/rms/reports", cad.RMSReport{Incident: inc.Number, Narrative: "Bicycle taken from the porch.",
		Offenses: []cad.RMSOffense{{Code: "23D", Using: []string{"N"}, Bias: "88"}}, LocationType: "20",
		People: []cad.RMSPerson{{Role: "victim", Name: "DOE, JANE", Age: "41", Sex: "F", Race: "W"}}}, true)
	var saved struct {
		Report cad.RMSReport
		Checks []cad.RMSCheck
	}
	json.Unmarshal(b, &saved)
	if code != 200 || saved.Report.Number == "" || saved.Report.Occurred.IsZero() || len(saved.Checks) == 0 {
		t.Fatalf("save: %d %s", code, b)
	}
	n := saved.Report.Number
	if code, b := d.do("POST", "/api/rms/reports/"+n+"/status", map[string]string{"status": "submitted"}, true); code != 422 || !strings.Contains(string(b), "stolen property") {
		t.Fatalf("submitted without property: %d %s", code, b)
	}
	r := saved.Report
	r.Property = []cad.RMSProperty{{Loss: "stolen", Type: "04", Description: "Blue bicycle", Value: 350, Quantity: 1}}
	if code, b = d.do("POST", "/api/rms/reports", r, true); code != 200 {
		t.Fatalf("resave: %d %s", code, b)
	}
	if code, b := d.do("POST", "/api/rms/reports/"+n+"/status", map[string]string{"status": "submitted"}, true); code != 204 {
		t.Fatalf("submit: %d %s", code, b)
	}
	// The author cannot approve; a supervisor (here the admin) can.
	if code, _ := d.do("POST", "/api/rms/reports/"+n+"/status", map[string]string{"status": "approved"}, true); code != 422 {
		t.Fatalf("dispatcher approved: %d", code)
	}
	a := login(t, ts, "admin", pw["admin"])
	if code, b := a.do("POST", "/api/rms/reports/"+n+"/status", map[string]string{"status": "approved"}, true); code != 204 {
		t.Fatalf("approve: %d %s", code, b)
	}
	s.Eng.Read(func(st *cad.State) {
		if st.Reports[n].Status != cad.RptApproved || st.Reports[n].Approver != "admin" || st.Incidents[inc.Number].Reports[0] != n {
			t.Fatalf("report %+v", st.Reports[n])
		}
	})
	// NCIC: off until set; the simulator answers; the ledger keeps a hash of the terms, never the answer.
	q := ncic.Query{Key: "QV", Fields: map[string]string{"LIC": "test123", "LIS": "va"}, Purpose: "C", Reason: inc.Number}
	if code, _ := d.do("POST", "/api/ncic/query", q, true); code != 409 {
		t.Fatalf("query while off: %d", code)
	}
	var set cad.Settings
	s.Eng.Read(func(st *cad.State) { set = st.Settings })
	set.NCICMode, set.ORI = "sim", "VA0770000"
	if err := s.Eng.SaveSettings(cad.SystemActor, set); err != nil {
		t.Fatal(err)
	}
	code, b = d.do("POST", "/api/ncic/query", q, true)
	var ans ncic.Answer
	json.Unmarshal(b, &ans)
	if code != 200 || !ans.Hit || !ans.Simulated || ans.Message != "QV.VA0770000.LIC/TEST123.LIS/VA" {
		t.Fatalf("query: %d %s", code, b)
	}
	q.Fields = map[string]string{"LIC": "ABC1234"}
	if code, _ := d.do("POST", "/api/ncic/query", q, true); code != 400 {
		t.Fatalf("a plate without its state: %d", code)
	}
	evs, _ := s.Eng.Events(0, 0, nil)
	found := false
	for _, e := range evs {
		if e.Type == cad.EvJusticeQuery {
			found = true
			if strings.Contains(string(e.Data), "TEST123") || strings.Contains(string(e.Data), "STOLEN") {
				t.Fatalf("terms or answer in the ledger: %s", e.Data)
			}
		}
	}
	if !found {
		t.Fatal("no justice.query event")
	}
	_ = time.Now
}

func TestIBRFile(t *testing.T) {
	ts, s, pw := newServer(t)
	a := login(t, ts, "admin", pw["admin"])
	month := time.Now().Format("2006-01")
	// No ORI yet: the file cannot be made.
	if code, b := a.do("GET", "/api/rms/ibr?month="+month, nil, false); code != 422 || !strings.Contains(string(b), "ORI") {
		t.Fatalf("no ORI: %d %s", code, b)
	}
	var set cad.Settings
	s.Eng.Read(func(st *cad.State) { set = st.Settings })
	set.ORI = "VA0770000"
	s.Eng.SaveSettings(cad.SystemActor, set)
	code, b := a.do("GET", "/api/rms/ibr?month="+month, nil, false)
	if code != 200 || !strings.Contains(string(b), `"zero":true`) {
		t.Fatalf("preview: %d %s", code, b)
	}
	code, b = a.do("POST", "/api/rms/ibr?month="+month, nil, true)
	if code != 200 || len(b) != 45 || string(b[4:6]) != "0A" {
		t.Fatalf("file: %d %q", code, b)
	}
	if code, b = a.do("POST", "/api/rms/ibr?month="+month+"&format=fbi", nil, true); code != 200 || string(b[:6]) != "00430A" {
		t.Fatalf("FBI file: %d %q", code, b)
	}
	d := login(t, ts, "d1", pw["d1"])
	if code, _ := d.do("POST", "/api/rms/ibr?month="+month, nil, true); code != 403 {
		t.Fatalf("a dispatcher made the state file: %d", code)
	}
}
