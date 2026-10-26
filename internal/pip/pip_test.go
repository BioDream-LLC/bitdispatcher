package pip

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// contains reports whether s is in ss.
func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// sameSet reports whether got and want hold the same elements, order aside.
func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for _, w := range want {
		if !contains(got, w) {
			return false
		}
	}
	return true
}

func TestRouteTable(t *testing.T) {
	cases := []struct {
		name  string
		event Event
		want  []string
	}{
		{"period", Event{Class: ClassPeriod}, []string{ModCEDE}},
		{"exception", Event{Class: ClassException}, []string{ModCEDE}},
		{"critical not qualifying", Event{Class: ClassCritical}, []string{ModCEDE}},
		{"critical not qualifying explicit no", Event{Class: ClassCritical, Facts: map[string]string{"qualifying": "no"}}, []string{ModCEDE}},
		{"critical qualifying", Event{Class: ClassCritical, Facts: map[string]string{"qualifying": "yes"}}, []string{ModCEDE, ModRESET}},
		{"after action", Event{Class: ClassAfterAction}, []string{ModARC}},
		{"recovery", Event{Class: ClassRecovery}, []string{ModROC}},
		{"support", Event{Class: ClassSupport}, []string{ModRESET}},
		{"correction of period", Event{Class: ClassCorrection, Facts: map[string]string{"class": ClassPeriod}}, []string{ModCEDE}},
		{"correction of qualifying critical", Event{Class: ClassCorrection, Facts: map[string]string{"class": ClassCritical, "qualifying": "yes"}}, []string{ModCEDE, ModRESET}},
		{"correction of after action", Event{Class: ClassCorrection, Facts: map[string]string{"class": ClassAfterAction}}, []string{ModARC}},
		{"correction of recovery", Event{Class: ClassCorrection, Facts: map[string]string{"class": ClassRecovery}}, []string{ModROC}},
		{"correction of support", Event{Class: ClassCorrection, Facts: map[string]string{"class": ClassSupport}}, []string{ModRESET}},
		{"correction with no class", Event{Class: ClassCorrection}, []string{}},
		{"correction of itself", Event{Class: ClassCorrection, Facts: map[string]string{"class": ClassCorrection}}, []string{}},
		{"unknown class", Event{Class: "nope"}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := tc.event
			Route(&e)
			if !sameSet(e.Modules, tc.want) {
				t.Fatalf("Route modules = %v, want %v", e.Modules, tc.want)
			}
		})
	}
}

func TestPrepareDropsDisallowedClass(t *testing.T) {
	c := Config{Classes: []string{ClassPeriod}}
	_, ok := Prepare(c, Event{Class: ClassCritical})
	if ok {
		t.Fatal("critical should be dropped when only period is allowed")
	}
	_, ok = Prepare(c, Event{Class: ClassPeriod})
	if !ok {
		t.Fatal("period should be allowed")
	}
}

func TestPrepareCorrectionFollowsCorrectedClass(t *testing.T) {
	// A correction of a critical is allowed only when critical is allowed.
	allow := Config{Classes: []string{ClassCritical}}
	_, ok := Prepare(allow, Event{Class: ClassCorrection, Facts: map[string]string{"class": ClassCritical}})
	if !ok {
		t.Fatal("correction of critical should cross when critical is allowed")
	}
	deny := Config{Classes: []string{ClassPeriod}}
	_, ok = Prepare(deny, Event{Class: ClassCorrection, Facts: map[string]string{"class": ClassCritical}})
	if ok {
		t.Fatal("correction of critical should not cross when critical is not allowed")
	}
}

func TestPrepareAggregateMode(t *testing.T) {
	c := Config{Aggregate: true, Classes: []string{ClassPeriod, ClassCritical, ClassRecovery}}
	if _, ok := Prepare(c, Event{Class: ClassPeriod}); !ok {
		t.Fatal("period should cross in aggregate mode")
	}
	if _, ok := Prepare(c, Event{Class: ClassCritical}); ok {
		t.Fatal("critical should be dropped in aggregate mode")
	}
	if _, ok := Prepare(c, Event{Class: ClassRecovery}); ok {
		t.Fatal("recovery should be dropped in aggregate mode")
	}
}

func TestPreparePseudonymStableAndSaltDependent(t *testing.T) {
	c1 := Config{Classes: []string{ClassPeriod}, Salt: "salt-one"}
	e := Event{Class: ClassPeriod, People: []string{"dispatcher01"}}

	a, ok := Prepare(c1, e)
	if !ok {
		t.Fatal("event should cross")
	}
	b, ok := Prepare(c1, e)
	if !ok {
		t.Fatal("event should cross")
	}
	if a.People[0] != b.People[0] {
		t.Fatal("pseudonym should be stable for the same salt and username")
	}
	if a.People[0] == "dispatcher01" {
		t.Fatal("pseudonym should not equal the username")
	}

	c2 := Config{Classes: []string{ClassPeriod}, Salt: "salt-two"}
	d, _ := Prepare(c2, e)
	if a.People[0] == d.People[0] {
		t.Fatal("pseudonym should differ for a different salt")
	}
}

func TestPrepareIdentifiedPassesUsernames(t *testing.T) {
	c := Config{Classes: []string{ClassPeriod}, Identified: true, Salt: "ignored"}
	e := Event{Class: ClassPeriod, People: []string{"dispatcher01", "supervisor02"}}
	out, ok := Prepare(c, e)
	if !ok {
		t.Fatal("event should cross")
	}
	if out.People[0] != "dispatcher01" || out.People[1] != "supervisor02" {
		t.Fatalf("identified mode should pass usernames, got %v", out.People)
	}
}

func TestPrepareDoesNotMutateCaller(t *testing.T) {
	c := Config{Classes: []string{ClassPeriod}, Salt: "s"}
	e := Event{Class: ClassPeriod, People: []string{"dispatcher01"}, Facts: map[string]string{"kind": "forced", "secret": "x"}}
	_, _ = Prepare(c, e)
	if e.People[0] != "dispatcher01" {
		t.Fatal("Prepare must not mutate the caller's People slice")
	}
	if _, bad := e.Facts["_dropped"]; bad {
		t.Fatal("Prepare must not mutate the caller's Facts map")
	}
	if len(e.Facts) != 2 {
		t.Fatalf("caller's Facts should be untouched, got %v", e.Facts)
	}
}

func TestPrepareFactStripping(t *testing.T) {
	c := Config{Classes: []string{ClassCritical}, Salt: "s"}
	e := Event{
		Class: ClassCritical,
		Facts: map[string]string{
			"kind":        "forced",                                                        // allowed, kept
			"callType":    "SHOTS",                                                         // allowed, kept
			"reason":      "caller phone 5551234567",                                       // allowed key but digit run, dropped
			"status":      "ops@example.com",                                               // allowed key but has @, dropped
			"trigger":     "this is a long narrative with many many words indeed here now", // more than 8 words, dropped
			"disposition": strings.Repeat("x", 65),                                         // allowed key but longer than 64, dropped
			"notes":       "short",                                                         // unknown key, dropped
		},
	}
	out, ok := Prepare(c, e)
	if !ok {
		t.Fatal("event should cross")
	}
	if out.Facts["kind"] != "forced" || out.Facts["callType"] != "SHOTS" {
		t.Fatalf("allowed short facts should be kept, got %v", out.Facts)
	}
	for _, bad := range []string{"reason", "status", "trigger", "disposition", "notes"} {
		if _, present := out.Facts[bad]; present {
			t.Fatalf("fact %q should have been dropped, got %v", bad, out.Facts)
		}
	}
	dropped := out.Facts["_dropped"]
	for _, want := range []string{"reason", "status", "trigger", "disposition", "notes"} {
		if !strings.Contains(dropped, want) {
			t.Fatalf("_dropped %q should list %q", dropped, want)
		}
	}
	// _dropped should carry key names only, no values.
	if strings.Contains(dropped, "@") || strings.Contains(dropped, "5551234567") {
		t.Fatalf("_dropped must contain key names only, got %q", dropped)
	}
}

func TestSendHeadersBodyAnd2xx(t *testing.T) {
	var gotAuth, gotType string
	var gotBody struct {
		Events []Event `json:"events"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	c := &Client{Config: Config{On: true, URL: srv.URL, Token: "secret-token"}}
	ev := Event{ID: "e1", Class: ClassPeriod, Period: "2026-10-25/day", At: time.Now().UTC()}
	status, err := c.Send(context.Background(), []Event{ev})
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", status)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("Authorization = %q, want Bearer secret-token", gotAuth)
	}
	if gotType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotType)
	}
	if len(gotBody.Events) != 1 || gotBody.Events[0].ID != "e1" {
		t.Fatalf("body events = %v, want one event with ID e1", gotBody.Events)
	}
}

func TestSendNoTokenOmitsAuthHeader(t *testing.T) {
	var hadAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadAuth = r.Header["Authorization"]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &Client{Config: Config{On: true, URL: srv.URL}}
	if _, err := c.Send(context.Background(), []Event{{ID: "e1", Class: ClassPeriod}}); err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if hadAuth {
		t.Fatal("Authorization header should be absent when no token is set")
	}
}

func TestSend4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "bad event: missing period")
	}))
	defer srv.Close()

	c := &Client{Config: Config{On: true, URL: srv.URL}}
	status, err := c.Send(context.Background(), []Event{{ID: "e1", Class: ClassPeriod}})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if err == nil {
		t.Fatal("expected error for 4xx")
	}
	if !strings.Contains(err.Error(), "bad event: missing period") {
		t.Fatalf("error should include body snippet, got %v", err)
	}
}

func TestSendRefusesWhenOff(t *testing.T) {
	c := &Client{Config: Config{On: false, URL: "https://pip.example.com"}}
	if _, err := c.Send(context.Background(), nil); err == nil {
		t.Fatal("Send should refuse when connection is off")
	}
}

func TestSendRefusesBadURL(t *testing.T) {
	c := &Client{Config: Config{On: true, URL: "http://pip.example.com"}}
	if _, err := c.Send(context.Background(), nil); err == nil {
		t.Fatal("Send should refuse a remote http URL")
	}
}

func TestCheckURL(t *testing.T) {
	ok := []string{
		"https://pip.example.com/ingest",
		"http://127.0.0.1:8080/ingest",
		"http://localhost:9000/ingest",
	}
	for _, u := range ok {
		if err := CheckURL(u); err != nil {
			t.Fatalf("CheckURL(%q) = %v, want nil", u, err)
		}
	}
	bad := []string{
		"",
		"http://pip.example.com/ingest",
		"ftp://pip.example.com",
		"https://",
	}
	for _, u := range bad {
		if err := CheckURL(u); err == nil {
			t.Fatalf("CheckURL(%q) = nil, want error", u)
		}
	}
}
