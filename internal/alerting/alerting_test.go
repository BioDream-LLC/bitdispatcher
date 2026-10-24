package alerting

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var est = time.FixedZone("EDT", -4*3600)

func sample() Spec {
	sent := time.Date(2026, 10, 24, 9, 15, 0, 0, est)
	return Spec{Sender: "alerts@example.gov", SenderName: "Example County Emergency Communications", Identifier: "AL1-20261024T131500",
		Sent: sent, Expires: sent.Add(2 * time.Hour), Status: "Actual", MsgType: "Alert", Incident: "26-000123",
		Event: "SPW", EventName: "Shelter in place warning", Category: "Env", Response: "Shelter",
		Urgency: "Immediate", Severity: "Severe", Certainty: "Observed",
		Texts: []Text{
			{Language: "en-US", Headline: "Shelter in place near the chemical plant on Main St",
				Description: "An ammonia leak at the plant on Main St.", Instruction: "Go inside, close windows and doors, and turn off fans and air conditioning.",
				Short: "Ammonia leak near Main St: go inside, close windows, turn off fans now.", Long: "An ammonia leak at the plant on Main St. Go inside now, close windows and doors, and turn off fans and air conditioning until told it is safe."},
			{Language: "es-US", Headline: "Refúgiese en casa cerca de la planta química de Main St", Description: "Una fuga de amoníaco en la planta de Main St.",
				Instruction: "Entre, cierre puertas y ventanas y apague ventiladores y aire acondicionado.",
				Short:       "Fuga de amoníaco cerca de Main St: entre, cierre ventanas, apague ventiladores."},
		},
		AreaDesc: "Within half a mile of 100 Main St", Polygon: [][2]float64{{-79.95, 37.27}, {-79.93, 37.27}, {-79.93, 37.285}, {-79.95, 37.285}},
		Geocodes: []string{"051770"}, Web: "https://example.gov/alerts"}
}

func TestBuildCheckEncode(t *testing.T) {
	m := Build(sample())
	if p := Check(m); len(p) > 0 {
		t.Fatalf("problems: %v", p)
	}
	b, err := Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{`<alert xmlns="urn:oasis:names:tc:emergency:cap:1.2">`, "<sent>2026-10-24T09:15:00-04:00</sent>",
		"<code>IPAWSv1.0</code>", "<valueName>SAME</valueName>", "<value>SPW</value>", "<valueName>CMAMtext</valueName>",
		"<valueName>EAS-ORG</valueName>", "<polygon>37.27000,-79.95000 37.27000,-79.93000 37.28500,-79.93000 37.28500,-79.95000 37.27000,-79.95000</polygon>",
		"<value>051770</value>", "<language>es-US</language>"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %s in\n%s", want, s)
		}
	}
	// The order of elements is the schema's.
	if strings.Index(s, "<identifier>") > strings.Index(s, "<sender>") || strings.Index(s, "<category>") > strings.Index(s, "<event>") ||
		strings.Index(s, "<expires>") > strings.Index(s, "<headline>") || strings.Index(s, "<parameter>") > strings.Index(s, "<area>") {
		t.Fatal("elements out of order")
	}
	back, err := Decode(b)
	if err != nil || back.Identifier != m.Identifier || len(back.Info) != 2 || back.Info[1].Area[0].Geocode[0].Value != "051770" {
		t.Fatalf("decoded %+v %v", back, err)
	}
	if Time(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) != "2026-01-02T03:04:05-00:00" {
		t.Fatal("UTC is written -00:00")
	}
}

func TestCheckFindsProblems(t *testing.T) {
	s := sample()
	s.Texts[0].Short = strings.Repeat("x", 91)
	s.Event = "spw"
	s.Geocodes = []string{"51770"}
	s.MsgType = "Update"
	s.Expires = s.Sent.Add(-time.Minute)
	m := Build(s)
	m.Info[1].Category = []string{"Safety"}
	p := strings.Join(Check(m), "\n")
	for _, want := range []string{"limit is 90", "three capital letters", "six digits", "names the messages it follows", "expires before", "same event code and category"} {
		if !strings.Contains(p, want) {
			t.Errorf("no %q in\n%s", want, p)
		}
	}
	m = Build(sample())
	m.Info[0].Area[0].Polygon = []string{"37.27,-79.95 37.27,-79.93 37.285,-79.93"}
	if p := strings.Join(Check(m), "\n"); !strings.Contains(p, "four points") {
		t.Fatalf("an open three-point polygon: %s", p)
	}
}

func TestFeed(t *testing.T) {
	at := time.Date(2026, 10, 24, 13, 15, 0, 0, time.UTC)
	b := Feed("https://cad.example.gov/cap/feed.xml", "Example County public alerts", "Example County", at,
		[]Entry{{ID: "AL1-x", Title: "Shelter in place", Updated: at, CAPURL: "https://cad.example.gov/cap/AL1.xml"}})
	var f struct {
		XMLName xml.Name
		Entry   []struct {
			Link struct {
				Href string `xml:"href,attr"`
				Type string `xml:"type,attr"`
			} `xml:"link"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(b, &f); err != nil || f.XMLName.Space != "http://www.w3.org/2005/Atom" || len(f.Entry) != 1 ||
		f.Entry[0].Link.Type != "application/cap+xml" || !strings.HasSuffix(f.Entry[0].Link.Href, "/cap/AL1.xml") {
		t.Fatalf("feed %+v %v\n%s", f, err, b)
	}
}

func TestPost(t *testing.T) {
	var got []byte
	var auth, ct string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		auth, ct = r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		if strings.Contains(string(got), "REFUSE") {
			http.Error(w, "bad message", 400)
		}
	}))
	defer ts.Close()
	r := Receiver{Name: "Siren controller", URL: ts.URL + "/cap", Token: "s3cret"}
	res := Post(context.Background(), ts.Client(), r, []byte("<alert/>"))
	if !res.OK || string(got) != "<alert/>" || auth != "Bearer s3cret" || !strings.HasPrefix(ct, "application/cap+xml") {
		t.Fatalf("%+v %q %q %q", res, got, auth, ct)
	}
	if res := Post(context.Background(), ts.Client(), r, []byte("REFUSE")); res.OK || !strings.Contains(res.Detail, "HTTP 400: bad message") {
		t.Fatalf("refused: %+v", res)
	}
	if err := (Receiver{Name: "x", URL: "http://example.org/cap"}).Check(); err == nil {
		t.Fatal("a plain http receiver off this machine was allowed")
	}
	if r.Redacted().Token != "••••" {
		t.Fatal("the token shows")
	}
}

// TestValidators checks messages with Google's CAP library and its IPAWS profile when CAP_VALIDATOR names
// the command (scripts/cap-validator.sh builds it; CI does not have Java): an alert in two languages, an
// update and a cancel, a test message, one for a whole county, and one kept off the EAS.
func TestValidators(t *testing.T) {
	cmd := os.Getenv("CAP_VALIDATOR")
	if cmd == "" {
		t.Skip("CAP_VALIDATOR is not set (scripts/cap-validator.sh)")
	}
	first := Build(sample())
	update := sample()
	update.Identifier, update.MsgType, update.References = "AL2-20261024T134500", "Update", []string{Reference(first)}
	update.Sent = update.Sent.Add(30 * time.Minute)
	cancel := update
	cancel.Identifier, cancel.MsgType, cancel.Response, cancel.Urgency, cancel.Severity = "AL3-20261024T141500", "Cancel", "AllClear", "Past", "Minor"
	cancel.References = []string{Reference(first), Reference(Build(update))}
	test := sample()
	test.Status, test.Event, test.EventName, test.Category, test.Response = "Test", "DMO", "Practice / demonstration warning", "Other", "None"
	county := sample()
	county.Polygon, county.Texts = nil, county.Texts[:1]
	county.Event, county.EventName, county.Category, county.Response = "TOE", "911 telephone outage emergency", "Infra", "None"
	county.Texts[0].Headline, county.Texts[0].Short = "911 is down in the county: call 540-555-0100", "911 is down. For emergencies call 540-555-0100."
	blocked := sample()
	blocked.Block = []string{"EAS", "NWEM"}
	for name, m := range map[string]Message{"alert": first, "update": Build(update), "cancel": Build(cancel), "test": Build(test), "county": Build(county), "blocked": Build(blocked)} {
		if p := Check(m); len(p) > 0 {
			t.Fatalf("%s: our own checks: %v", name, p)
		}
		b, _ := Encode(m)
		f := filepath.Join(t.TempDir(), name+".xml")
		if d := os.Getenv("CAP_DUMP"); d != "" {
			os.WriteFile(filepath.Join(d, name+".xml"), b, 0o644)
		}
		os.WriteFile(f, b, 0o644)
		out, err := exec.Command(cmd, f).CombinedOutput()
		if err != nil || !strings.Contains(string(out), "checked "+m.Identifier) {
			t.Errorf("%s: the validator found errors (%v):\n%s\n%s", name, err, out, b)
			continue
		}
		t.Logf("%s:\n%s", name, out)
	}
	// And it finds what is wrong: no expiry and no SAME event code.
	broken := Build(sample())
	broken.Info[0].Expires, broken.Info[0].EventCode = "", nil
	b, _ := Encode(broken)
	f := filepath.Join(t.TempDir(), "broken.xml")
	os.WriteFile(f, b, 0o644)
	out, err := exec.Command(cmd, f).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "ERROR ipaws") {
		t.Fatalf("the validator passed a message with no expiry or event code:\n%s", out)
	}
	t.Logf("broken:\n%s", out)
}
