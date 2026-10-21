package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// One centre sends an incident to its neighbour, which gets it as a call in the queue; a wrong token is
// refused; the sender's incident shows it was sent.
func TestCADToCAD(t *testing.T) {
	tsA, sA, pwA := newServer(t)
	tsB, sB, pwB := newServer(t)
	shareClient = tsB.Client()
	adminA, adminB := login(t, tsA, "admin", pwA["admin"]), login(t, tsB, "admin", pwB["admin"])
	dA := login(t, tsA, "d1", pwA["d1"])
	if code, b := adminA.do("PUT", "/api/admin/partners/county-b", map[string]any{"name": "County B 911", "url": tsB.URL + "/api/cad/eido/COUNTY-A", "outToken": "a-to-b"}, true); code != 204 {
		t.Fatalf("partner on A: %d %s", code, b)
	}
	if code, b := adminB.do("PUT", "/api/admin/partners/county-a", map[string]any{"name": "County A 911", "inToken": "a-to-b"}, true); code != 204 {
		t.Fatalf("partner on B: %d %s", code, b)
	}
	if _, b := adminB.do("GET", "/api/admin/partners", nil, false); strings.Contains(string(b), "a-to-b") || !strings.Contains(string(b), `"inTokenSet":true`) {
		t.Fatalf("tokens shown: %s", b)
	}
	_, b := dA.do("POST", "/api/incidents", map[string]any{"type": "SF", "location": map[string]any{"lat": 37.27, "lon": -79.94, "address": "1 Line Rd"}}, true)
	var inc struct{ Number string }
	json.Unmarshal(b, &inc)
	if code, b := dA.do("POST", "/api/incidents/"+inc.Number+"/share", map[string]string{"partner": "county-b", "reason": "Mutual aid: need a ladder"}, true); code != 204 {
		t.Fatalf("share: %d %s", code, b)
	}
	var call *cad.Call
	sB.Eng.Read(func(st *cad.State) {
		for _, c := range st.Calls {
			if c.Alert != nil && c.Alert.Kind == cad.AlertCAD {
				cc := *c
				call = &cc
			}
		}
	})
	if call == nil || call.Alert.Sender != "County A 911" || call.Alert.ID != inc.Number || call.Location == nil || !strings.Contains(strings.Join(call.Notes, "|"), "need a ladder") {
		t.Fatalf("not received as a call on B: %+v", call)
	}
	sent := false
	sA.Eng.Read(func(st *cad.State) {
		for _, e := range st.Incidents[inc.Number].Timeline {
			sent = sent || (e.Kind == "shared" && strings.Contains(e.Text, "sent to County B 911"))
		}
	})
	if !sent {
		t.Fatal("A's incident does not show it was sent")
	}
	if code, _ := hookPost(t, tsB, "/api/cad/eido/COUNTY-A", "wrong", "{}"); code != 401 {
		t.Fatalf("a wrong token: %d", code)
	}
}
