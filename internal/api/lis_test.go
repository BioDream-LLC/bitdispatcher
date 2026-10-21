package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// A dispatcher asks the location server about a call; its answer is the call's newest location.
func TestLIS(t *testing.T) {
	var got string
	lis := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b) + "|" + r.Header.Get("Authorization") + "|" + r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/held+xml")
		if strings.Contains(string(b), "5550199") {
			io.WriteString(w, `<error xmlns="urn:ietf:params:xml:ns:geopriv:held" code="locationUnknown"/>`)
			return
		}
		io.WriteString(w, `<locationResponse xmlns="urn:ietf:params:xml:ns:geopriv:held"><presence xmlns="urn:ietf:params:xml:ns:pidf" entity="pres:a@b"><tuple id="t"><status><geopriv xmlns="urn:ietf:params:xml:ns:pidf:geopriv10"><location-info><Point xmlns="http://www.opengis.net/gml"><pos>37.27312 -79.94021</pos></Point></location-info><method>DBH</method></geopriv></status></tuple></presence></locationResponse>`)
	}))
	defer lis.Close()
	lisClient = lis.Client()
	ts, s, pw := newServer(t)
	admin := login(t, ts, "admin", pw["admin"])
	d1 := login(t, ts, "d1", pw["d1"])
	if code, b := admin.do("PUT", "/api/admin/lis", map[string]any{"on": true, "name": "RapidSOS", "url": lis.URL, "token": "lis-tok"}, true); code != 200 || strings.Contains(string(b), "lis-tok") {
		t.Fatalf("config %d %s", code, b)
	}
	s.Eng.UpsertCall(cad.SystemActor, "c1", cad.CallData{Source: "bitvoice", State: "ringing", Callback: "+15405550100"})
	s.Eng.UpsertCall(cad.SystemActor, "c2", cad.CallData{Source: "bitvoice", State: "ringing", Callback: "+15405550199"})
	code, b := d1.do("POST", "/api/calls/c1/lis", map[string]any{}, true)
	if code != 200 || !strings.Contains(got, "<uri>tel:+15405550100</uri>") || !strings.Contains(got, "Bearer lis-tok") || !strings.Contains(got, "application/held+xml") {
		t.Fatalf("%d %s | %s", code, b, got)
	}
	s.Eng.Read(func(st *cad.State) {
		l := st.Calls["c1"].Location
		if l == nil || *l.Lat != 37.27312 || l.Source != "lis:rapidsos" {
			t.Fatalf("%+v", l)
		}
	})
	if code, b := d1.do("POST", "/api/calls/c2/lis", map[string]any{}, true); code != 404 || !strings.Contains(string(b), "locationUnknown") {
		t.Fatalf("%d %s", code, b)
	}
	var v map[string]any
	_, b = admin.do("POST", "/api/admin/lis/test", map[string]string{"callback": "+15405550100"}, true)
	json.Unmarshal(b, &v)
	if v["ok"] != true {
		t.Fatalf("%s", b)
	}
}
