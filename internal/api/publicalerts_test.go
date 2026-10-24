package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/alerting"
	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/paging"
)

func TestPublicAlertsAPI(t *testing.T) {
	ts, s, pw := newServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.alerter(ctx)
	for _, u := range []string{"sup1", "sup2"} {
		pw[u] = auth.GeneratePassword()
		s.Auth.Save(u, u, auth.Supervisor, pw[u], false)
	}
	admin := login(t, ts, "admin", pw["admin"])
	sup1 := login(t, ts, "sup1", pw["sup1"])
	sup2 := login(t, ts, "sup2", pw["sup2"])
	d1 := login(t, ts, "d1", pw["d1"])
	ok := func(code int, b []byte) []byte {
		t.Helper()
		if code >= 300 {
			t.Fatalf("%d %s", code, b)
		}
		return b
	}

	// Stand-ins: a CAP receiver (a siren controller, say) and an ntfy server for the subscribers.
	var mu sync.Mutex
	var capGot [][]byte
	var capAuth string
	var pushes []string
	rx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		capGot, capAuth = append(capGot, b), r.Header.Get("Authorization")
		mu.Unlock()
	}))
	defer rx.Close()
	push := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		pushes = append(pushes, r.URL.Path+" "+r.Header.Get("Title")+" | "+string(b))
		mu.Unlock()
	}))
	defer push.Close()
	ok(admin.do("PUT", "/api/admin/paging", Paging{PublicURL: "https://cad.example.gov", Gateways: []paging.Gateway{{ID: "push", Name: "Push", Kind: paging.Ntfy, URL: push.URL + "/county-alerts"}}}, true))
	if code, _ := admin.do("PUT", "/api/admin/alerting", Alerting{Sender: "alerts@example.gov", Geocodes: []string{"51770"}}, true); code != 422 {
		t.Fatalf("a five-digit county code: %d", code)
	}
	ok(admin.do("PUT", "/api/admin/alerting", Alerting{Sender: "alerts@example.gov", SenderName: "Example City ECC", Jurisdiction: "Example City", TimeZone: "America/New_York",
		Geocodes: []string{"051770"}, Feed: true, TwoPerson: true, Receivers: []alerting.Receiver{{ID: "siren", Name: "Siren controller", URL: rx.URL + "/cap", Token: "rx-secret"}}}, true))
	if _, b := sup1.do("GET", "/api/admin/alerting", nil, false); strings.Contains(string(b), "rx-secret") || !strings.Contains(string(b), "/api/public/alerts.atom") {
		t.Fatalf("settings: %s", b)
	}
	if code, _ := sup1.do("PUT", "/api/admin/alerting", Alerting{}, true); code != 403 {
		t.Fatalf("a supervisor changed the alert settings: %d", code)
	}

	inLat, inLon, outLat, outLon := 37.2710, -79.9414, 37.40, -79.70
	listID := struct {
		ID string `json:"id"`
	}{}
	json.Unmarshal(ok(sup1.do("PUT", "/api/admin/alertlists", cad.AlertList{Name: "Downtown residents", Gateways: []string{"push"}, People: []cad.Subscriber{
		{Name: "In", Topic: "res-in", Lat: &inLat, Lon: &inLon}, {Name: "Out", Topic: "res-out", Lat: &outLat, Lon: &outLon}, {Name: "Spanish", Topic: "res-es", Lang: "es"},
	}}, true)), &listID)
	if code, _ := sup1.do("PUT", "/api/admin/alertlists", cad.AlertList{Name: "x", Gateways: []string{"nope"}}, true); code != 422 {
		t.Fatalf("an unknown gateway: %d", code)
	}

	ring := [][2]float64{{-79.95, 37.265}, {-79.93, 37.265}, {-79.93, 37.28}, {-79.95, 37.28}}
	body := map[string]any{"status": "Actual", "event": "SPW", "response": "Shelter",
		"text":    map[string]string{"headline": "Shelter in place downtown", "description": "A chlorine leak at the water plant.", "instruction": "Go inside, shut windows, turn off fans.", "short": "Chlorine leak downtown: go inside, shut windows, turn off fans now."},
		"spanish": map[string]string{"headline": "Refúgiese en casa en el centro", "description": "Fuga de cloro en la planta de agua.", "short": "Fuga de cloro en el centro: entre y cierre ventanas."},
		"area":    map[string]any{"desc": "Downtown", "polygon": ring}, "lists": []string{listID.ID}, "receivers": []string{"siren"},
		"expires": time.Now().Add(2 * time.Hour).Format(time.RFC3339)}
	if code, _ := d1.do("POST", "/api/alerts", body, true); code != 403 {
		t.Fatalf("a dispatcher sent a public alert: %d", code)
	}
	var chk struct {
		Problems []string `json:"problems"`
		CAP      string   `json:"cap"`
		Reach    int      `json:"reach"`
	}
	json.Unmarshal(ok(sup1.do("POST", "/api/alerts/check", body, true)), &chk)
	if len(chk.Problems) > 0 || chk.Reach != 2 || !strings.Contains(chk.CAP, "<code>IPAWSv1.0</code>") || !strings.Contains(chk.CAP, "<value>051770</value>") {
		t.Fatalf("check: %+v", chk)
	}
	var made struct{ ID, State string }
	json.Unmarshal(ok(sup1.do("POST", "/api/alerts", body, true)), &made)
	if made.State != cad.AlertPending {
		t.Fatalf("with two-person approval the alert went at once: %+v", made)
	}
	pub := &client{t: t, base: ts.URL, c: ts.Client()}
	if code, _ := pub.do("GET", "/api/public/alerts/"+made.ID+".xml", nil, false); code != 404 {
		t.Fatalf("a pending alert is public: %d", code)
	}
	if code, _ := sup1.do("POST", "/api/alerts/"+made.ID+"/approve", nil, true); code != 422 {
		t.Fatalf("the author approved their own alert: %d", code)
	}
	ok(sup2.do("POST", "/api/alerts/"+made.ID+"/approve", nil, true))

	wait := func(id string, n int) cad.PublicAlert {
		t.Helper()
		var a cad.PublicAlert
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			s.Eng.Read(func(st *cad.State) { a = *st.Alert(id) })
			if len(a.Results) >= n {
				return a
			}
		}
		t.Fatalf("alert %s was not delivered: %+v", id, a)
		return a
	}
	a := wait(made.ID, 3)
	mu.Lock()
	if len(capGot) != 1 || capAuth != "Bearer rx-secret" || len(pushes) != 2 || a.Reached != 2 {
		t.Fatalf("delivered: %d CAP (%q), pushes %q, reached %d, results %+v", len(capGot), capAuth, pushes, a.Reached, a.Results)
	}
	if !strings.Contains(strings.Join(pushes, "\n"), "/res-es Refúgiese") || strings.Contains(strings.Join(pushes, "\n"), "res-out") {
		t.Fatalf("pushes %q", pushes)
	}
	sent := capGot[0]
	mu.Unlock()
	if m, err := alerting.Decode(sent); err != nil || len(alerting.Check(m)) > 0 || m.Sender != "alerts@example.gov" || m.Status != "Actual" {
		t.Fatalf("the CAP sent: %v %v\n%s", err, alerting.Check(m), sent)
	}

	// The public feed lists it; the message is there to fetch.
	code, feed := pub.do("GET", "/api/public/alerts.atom", nil, false)
	if code != 200 || !strings.Contains(string(feed), "https://cad.example.gov/api/public/alerts/"+made.ID+".xml") || !strings.Contains(string(feed), "Shelter in place downtown") {
		t.Fatalf("feed %d %s", code, feed)
	}
	code, msg := pub.do("GET", "/api/public/alerts/"+made.ID+".xml", nil, false)
	if code != 200 || !strings.Contains(string(msg), "<identifier>"+a.Identifier+"</identifier>") {
		t.Fatalf("message %d %s", code, msg)
	}
	validate(t, "alert", msg)
	// The IPAWS-style feed carries the message itself; EAS Station's poller reads it when EAS_PARSER is set.
	code, ipaws := pub.do("GET", "/api/public/alerts.xml", nil, false)
	if code != 200 || !strings.Contains(string(ipaws), `<alerts xmlns="http://gov.fema.ipaws.services/feed">`) || !strings.Contains(string(ipaws), "<identifier>"+a.Identifier+"</identifier>") {
		t.Fatalf("IPAWS-style feed %d %s", code, ipaws)
	}
	if cmd := os.Getenv("EAS_PARSER"); cmd != "" {
		f := filepath.Join(t.TempDir(), "feed.xml")
		os.WriteFile(f, ipaws, 0o644)
		out, err := exec.Command(cmd, f).CombinedOutput()
		if err != nil || !strings.Contains(string(out), `"identifier": "`+a.Identifier+`"`) || !strings.Contains(string(out), `"same": ["SPW"]`) ||
			!strings.Contains(string(out), `"polygon": true`) || !strings.Contains(string(out), `"geocode": {"SAME": ["051770"]}`) {
			t.Fatalf("EAS Station read the feed as (%v):\n%s", err, out)
		}
		t.Logf("EAS Station: %s", out)
	}

	// Cancel it: also approved by the second supervisor; the feed then has only the all-clear.
	json.Unmarshal(ok(sup1.do("POST", "/api/alerts", map[string]any{"msgType": "Cancel", "of": made.ID}, true)), &made)
	cancelID := made.ID
	ok(sup2.do("POST", "/api/alerts/"+cancelID+"/approve", nil, true))
	c := wait(cancelID, 3)
	var first cad.PublicAlert
	s.Eng.Read(func(st *cad.State) { first = *st.Alert(a.ID) })
	if first.State != cad.AlertCancelled || c.MsgType != "Cancel" {
		t.Fatalf("after the cancel: %s %s", first.State, c.MsgType)
	}
	_, feed = pub.do("GET", "/api/public/alerts.atom", nil, false)
	if strings.Contains(string(feed), "/alerts/"+a.ID+".xml") || !strings.Contains(string(feed), "CANCELLED: Shelter in place downtown") {
		t.Fatalf("feed after the cancel %s", feed)
	}
	_, msg = pub.do("GET", "/api/public/alerts/"+cancelID+".xml", nil, false)
	if !strings.Contains(string(msg), "<references>alerts@example.gov,"+a.Identifier+",") {
		t.Fatalf("the cancel does not name the alert: %s", msg)
	}
	validate(t, "cancel", msg)

	// A rejected alert goes nowhere.
	json.Unmarshal(ok(sup1.do("POST", "/api/alerts", body, true)), &made)
	ok(sup2.do("POST", "/api/alerts/"+made.ID+"/reject", map[string]string{"reason": "wrong area"}, true))
	if code, _ := sup2.do("POST", "/api/alerts/"+made.ID+"/approve", nil, true); code != 422 {
		t.Fatalf("a rejected alert was approved: %d", code)
	}

	// Subscribers from a CSV.
	req, _ := http.NewRequest("POST", ts.URL+"/api/admin/alertlists/"+listID.ID+"/import?mode=replace",
		strings.NewReader("Name,Phone,Email,Lang,Lat,Lon\nAna,(540) 555-0101,,es,37.27,-79.94\nBo,,bo@example.org,,,\nNobody,,,,,\n"))
	req.Header.Set(csrfHeader, "1")
	resp, err := sup1.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	rb, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(rb), `"imported":2`) {
		t.Fatalf("import %d %s", resp.StatusCode, rb)
	}
	s.Eng.Read(func(st *cad.State) {
		if p := st.AlertLists[listID.ID].People; len(p) != 2 || p[0].Phone != "+15405550101" || p[0].Lat == nil || p[0].Lang != "es" {
			t.Fatalf("imported %+v", p)
		}
	})

	// The feed is off unless turned on.
	ok(admin.do("PUT", "/api/admin/alerting", Alerting{Sender: "alerts@example.gov"}, true))
	if code, _ := pub.do("GET", "/api/public/alerts.atom", nil, false); code != 404 {
		t.Fatalf("the feed is on when off: %d", code)
	}
}

// validate runs a CAP message through Google's CAP library and its IPAWS profile when CAP_VALIDATOR is set
// (scripts/cap-validator.sh).
func validate(t *testing.T, name string, msg []byte) {
	t.Helper()
	cmd := os.Getenv("CAP_VALIDATOR")
	if cmd == "" {
		return
	}
	f := filepath.Join(t.TempDir(), name+".xml")
	os.WriteFile(f, msg, 0o644)
	out, err := exec.Command(cmd, f).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: the CAP validator: %v\n%s\n%s", name, err, out, msg)
	}
	t.Logf("%s: %s", name, out)
}
