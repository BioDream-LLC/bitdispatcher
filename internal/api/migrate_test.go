package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

func (c *client) upload(path, body string) (int, []byte) {
	c.t.Helper()
	req, _ := http.NewRequest("POST", c.base+path, strings.NewReader(body))
	req.Header.Set(csrfHeader, "1")
	resp, err := c.c.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func TestMigrate(t *testing.T) {
	ts, s, pw := newServer(t)
	a := login(t, ts, "admin", pw["admin"])
	// Premises from an earlier CAD's export: semicolon lists, a bad row, Windows line ends.
	csv := "PremiseID,Common Name,Full_Address,Latitude,Longitude,Cautions,Remarks\r\n" +
		"A-17,Valley Feed,12 Mill Rd,37.2701,-79.9402,Dog on premises; Oxygen in use,Knox box at the side door\r\n" +
		"A-18,,9 Nowhere Ln,,,Armed occupant,\r\n" +
		"A-19,Old School,40 School St,37.2801,-79.9502,,Vacant\r\n"
	code, b := a.upload("/api/admin/migrate/upload?name=premises.csv", csv)
	var mf MigrateFile
	json.Unmarshal(b, &mf)
	if code != 200 || mf.Kind != "premises" || mf.Rows != 3 || mf.Guess["premises"]["hazards"] != "Cautions" || mf.Guess["premises"]["address"] != "Full_Address" || mf.Guess["premises"]["id"] != "PremiseID" {
		t.Fatalf("upload: %d %s", code, b)
	}
	req := map[string]any{"file": "premises.csv", "kind": "premises", "fields": mf.Guess["premises"], "check": true}
	code, b = a.do("POST", "/api/admin/migrate/import", req, true)
	var res MigrateResult
	json.Unmarshal(b, &res)
	if code != 200 || res.Rows != 3 || res.Good != 2 || res.Imported != 0 || len(res.Problems) != 1 || res.Problems[0].Row != 3 {
		t.Fatalf("check: %d %s", code, b)
	}
	req["check"] = false
	code, b = a.do("POST", "/api/admin/migrate/import", req, true)
	json.Unmarshal(b, &res)
	if code != 200 || res.Imported != 2 {
		t.Fatalf("import: %d %s", code, b)
	}
	// The same file again replaces them by their ids.
	a.do("POST", "/api/admin/migrate/import", req, true)
	s.Eng.Read(func(st *cad.State) {
		if len(st.Premises) != 2 || len(st.Premises["A-17"].Hazards) != 2 || st.Premises["A-17"].Source != "import premises.csv" {
			t.Fatalf("premises %+v", st.Premises)
		}
	})

	// History and run cards.
	code, b = a.upload("/api/admin/migrate/upload?name=history.csv", "Incident Number\tCall Type\tAddress\tLat\tLon\tCreated\tDisposition\n"+
		"2019-001\tALARM\t12 Mill Rd\t37.27012\t-79.94021\t03/04/2019 2:05 PM\tFalse alarm\n"+
		"2020-002\tSICK\t12 Mill Rd\t37.27011\t-79.94020\t2020-06-01 08:00\tTransported\n")
	json.Unmarshal(b, &mf)
	if code != 200 || mf.Kind != "history" || mf.Guess["history"]["at"] != "Created" {
		t.Fatalf("history upload: %d %s", code, b)
	}
	code, b = a.do("POST", "/api/admin/migrate/import", map[string]any{"file": "history.csv", "kind": "history", "fields": mf.Guess["history"]}, true)
	if json.Unmarshal(b, &res); code != 200 || res.Imported != 2 {
		t.Fatalf("history: %d %s", code, b)
	}
	code, b = a.upload("/api/admin/migrate/upload?name=types.csv", "Nature Code,Description,Priority,Agency Type,Response Plan\nCO,CO alarm,P2,Fire,2 engine; als:1\nBAD,Bad,9,Fire,engine\n")
	json.Unmarshal(b, &mf)
	code, b = a.do("POST", "/api/admin/migrate/import", map[string]any{"file": "types.csv", "kind": "runcards", "fields": mf.Guess["runcards"]}, true)
	if json.Unmarshal(b, &res); code != 200 || res.Imported != 1 || len(res.Problems) != 1 {
		t.Fatalf("run cards: %d %s", code, b)
	}
	s.Eng.Read(func(st *cad.State) {
		ct := st.CallTypes["CO"]
		if ct == nil || ct.Priority != 2 || ct.Discipline != "fire" || len(ct.RunCard) != 2 || ct.RunCard[0] != (cad.Requirement{Capability: "engine", Count: 2}) {
			t.Fatalf("call type %+v", ct)
		}
	})

	// An incident at the place shows its premise and its history.
	lat, lon := 37.27009, -79.94018
	inc, err := s.Eng.CreateIncident(cad.SystemActor, cad.NewIncident{Type: "FA", Location: &ng911.Location{Address: "12 Mill Rd", Lat: &lat, Lon: &lon}})
	if err != nil {
		t.Fatal(err)
	}
	vb, _ := s.view()
	var v View
	json.Unmarshal(vb, &v)
	if len(v.Premises[inc]) != 1 || v.Premises[inc][0].ID != "A-17" || len(v.Prior[inc]) != 2 || v.Prior[inc][0].Number != "2020-002" {
		t.Fatalf("view: %+v %+v", v.Premises, v.Prior)
	}
	// Supervisors edit premises; dispatchers see them.
	d := login(t, ts, "d1", pw["d1"])
	if code, _ := d.do("PUT", "/api/premises", cad.Premise{Address: "1 Elm", Lat: 37.2, Lon: -79.9}, true); code != 403 {
		t.Fatalf("a dispatcher saved a premise: %d", code)
	}
	if code, b := d.do("GET", "/api/premises?q=dog", nil, false); code != 200 || !strings.Contains(string(b), "A-17") || strings.Contains(string(b), "A-19") {
		t.Fatalf("search: %d %s", code, b)
	}
	if code, _ := a.do("DELETE", "/api/premises/A-19", nil, true); code != 204 {
		t.Fatalf("remove: %d", code)
	}
}
