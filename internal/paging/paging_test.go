package paging

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fixedNow gives tests a stable clock.
func fixedNow() time.Time { return time.Date(2026, 10, 23, 20, 35, 0, 0, time.UTC) }

func samplePage() Page {
	lat, lon := 37.2710, -79.9414
	return Page{
		ID:       "pg-1",
		Incident: "26-000123",
		Subject:  "STRUCTURE FIRE 215 Church Ave SW",
		Text:     "Smoke showing from the second floor",
		Address:  "215 Church Ave SW, Roanoke VA 24011",
		Lat:      &lat, Lon: &lon,
		Units:    []string{"E1", "L1"},
		Priority: 1,
		ReplyURL: "https://example.com/r/pg-1",
		At:       fixedNow(),
	}
}

func testSender(c *http.Client) *Sender {
	return &Sender{HTTP: c, Now: fixedNow}
}

// --- webhook ---------------------------------------------------------------

func TestWebhookPostsJSON(t *testing.T) {
	var gotAuth, gotCT, gotPath, gotMethod string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotCT, gotPath, gotMethod = r.Header.Get("Authorization"), r.Header.Get("Content-Type"), r.URL.Path, r.Method
		json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(202)
	}))
	defer srv.Close()

	g := Gateway{Name: "hook", Kind: Webhook, URL: srv.URL + "/page", Token: "sekret"}
	rs := testSender(srv.Client()).Send(context.Background(), g, samplePage(),
		[]Recipient{{Name: "Pat", Phone: "+15555550101", Email: "pat@example.com"}})

	if len(rs) != 1 || !rs[0].OK {
		t.Fatalf("want one OK result, got %+v", rs)
	}
	if gotMethod != "POST" || gotPath != "/page" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if gotAuth != "Bearer sekret" {
		t.Errorf("auth = %q", gotAuth)
	}
	if !strings.HasPrefix(gotCT, "application/json") {
		t.Errorf("content-type = %q", gotCT)
	}
	if body["subject"] != "STRUCTURE FIRE 215 Church Ave SW" || body["incident"] != "26-000123" {
		t.Errorf("body missing fields: %+v", body)
	}
	recips, _ := body["recipients"].([]any)
	if len(recips) != 1 {
		t.Errorf("recipients = %+v", body["recipients"])
	}
}

func TestWebhook500IsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		io.WriteString(w, "boom")
	}))
	defer srv.Close()
	g := Gateway{Name: "hook", Kind: Webhook, URL: srv.URL}
	rs := testSender(srv.Client()).Send(context.Background(), g, samplePage(), nil)
	if len(rs) != 1 || rs[0].OK {
		t.Fatalf("want one failed result, got %+v", rs)
	}
	if !strings.Contains(rs[0].Detail, "500") {
		t.Errorf("detail = %q", rs[0].Detail)
	}
}

// --- Twilio ----------------------------------------------------------------

func TestTwilioSendsPerRecipient(t *testing.T) {
	type call struct {
		path, auth, to, from, body string
	}
	var mu sync.Mutex
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		mu.Lock()
		calls = append(calls, call{r.URL.Path, r.Header.Get("Authorization"), r.FormValue("To"), r.FormValue("From"), r.FormValue("Body")})
		mu.Unlock()
		w.WriteHeader(201)
		io.WriteString(w, `{"sid":"SMxxx","status":"queued"}`)
	}))
	defer srv.Close()

	g := Gateway{Name: "twilio", Kind: Twilio, Account: "AC" + strings.Repeat("a", 32), Token: "tok", From: "+15555550000"}
	s := &Sender{HTTP: srv.Client(), Now: fixedNow, TwilioBase: srv.URL}
	rs := s.Send(context.Background(), g, samplePage(), []Recipient{
		{Phone: "+15555550101"}, {Phone: "+15555550102"},
	})
	if len(rs) != 2 || !rs[0].OK || !rs[1].OK {
		t.Fatalf("want two OK, got %+v", rs)
	}
	if rs[0].To != "***-***-0101" {
		t.Errorf("masked To = %q", rs[0].To)
	}
	if len(calls) != 2 {
		t.Fatalf("want two API calls, got %d", len(calls))
	}
	want := "/2010-04-01/Accounts/AC" + strings.Repeat("a", 32) + "/Messages.json"
	for _, c := range calls {
		if c.path != want {
			t.Errorf("path = %q want %q", c.path, want)
		}
		if !strings.HasPrefix(c.auth, "Basic ") {
			t.Errorf("auth = %q", c.auth)
		}
		if c.from != "+15555550000" {
			t.Errorf("From = %q", c.from)
		}
		if !strings.Contains(c.body, "STRUCTURE FIRE") || !strings.Contains(c.body, "https://example.com/r/pg-1") {
			t.Errorf("body = %q", c.body)
		}
	}
}

func TestTwilioJSONError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		io.WriteString(w, `{"code":21211,"message":"The 'To' number is not a valid phone number","more_info":"https://www.twilio.com/docs/errors/21211","status":400}`)
	}))
	defer srv.Close()
	g := Gateway{Name: "twilio", Kind: Twilio, Account: "AC" + strings.Repeat("a", 32), Token: "tok", From: "+15555550000"}
	s := &Sender{HTTP: srv.Client(), Now: fixedNow, TwilioBase: srv.URL}
	rs := s.Send(context.Background(), g, samplePage(), []Recipient{{Phone: "+1555"}})
	if len(rs) != 1 || rs[0].OK {
		t.Fatalf("want one failure, got %+v", rs)
	}
	if !strings.Contains(rs[0].Detail, "21211") || !strings.Contains(rs[0].Detail, "not a valid phone number") {
		t.Errorf("detail = %q", rs[0].Detail)
	}
}

func TestTwilioMissingPhone(t *testing.T) {
	s := &Sender{Now: fixedNow, TwilioBase: "https://example.invalid"}
	g := Gateway{Name: "twilio", Kind: Twilio, Account: "AC" + strings.Repeat("a", 32), Token: "tok", From: "+15555550000"}
	rs := s.Send(context.Background(), g, samplePage(), []Recipient{{Name: "no phone"}})
	if len(rs) != 1 || rs[0].OK || !strings.Contains(rs[0].Detail, "no phone") {
		t.Fatalf("want a no-phone failure, got %+v", rs)
	}
}

// --- ntfy ------------------------------------------------------------------

func TestNtfyPerTopic(t *testing.T) {
	type pub struct {
		topic, title, prio, tags, click, auth, body string
	}
	var mu sync.Mutex
	var pubs []pub
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		pubs = append(pubs, pub{
			strings.TrimPrefix(r.URL.Path, "/"), r.Header.Get("Title"), r.Header.Get("Priority"),
			r.Header.Get("Tags"), r.Header.Get("Click"), r.Header.Get("Authorization"), string(b),
		})
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// Default topic "dispatch" in the URL path; one recipient overrides with "chief".
	g := Gateway{Name: "ntfy", Kind: Ntfy, URL: srv.URL + "/dispatch", Token: "tk_abc"}
	s := testSender(srv.Client())
	rs := s.Send(context.Background(), g, samplePage(), []Recipient{
		{Name: "crew"}, {Name: "chief", Topic: "chief"}, {Name: "crew2"},
	})
	// distinct topics: dispatch, chief
	if len(rs) != 2 {
		t.Fatalf("want two sends (two distinct topics), got %+v", rs)
	}
	if len(pubs) != 2 {
		t.Fatalf("want two publishes, got %d", len(pubs))
	}
	got := map[string]pub{}
	for _, p := range pubs {
		got[p.topic] = p
	}
	d, ok := got["dispatch"]
	if !ok {
		t.Fatalf("no publish to default topic, got topics %+v", got)
	}
	if d.title != "STRUCTURE FIRE 215 Church Ave SW" {
		t.Errorf("title = %q", d.title)
	}
	if d.prio != "5" { // priority-1 call -> ntfy 5 (max)
		t.Errorf("priority = %q want 5", d.prio)
	}
	if d.tags != "rotating_light" {
		t.Errorf("tags = %q", d.tags)
	}
	if d.click != "https://example.com/r/pg-1" {
		t.Errorf("click = %q", d.click)
	}
	if d.auth != "Bearer tk_abc" {
		t.Errorf("auth = %q", d.auth)
	}
	if d.body != "Smoke showing from the second floor" {
		t.Errorf("body = %q", d.body)
	}
	if _, ok := got["chief"]; !ok {
		t.Errorf("missing publish to recipient topic chief")
	}
}

func TestNtfyPriorityFourForNonPriorityOne(t *testing.T) {
	var prio string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prio = r.Header.Get("Priority")
		w.WriteHeader(200)
	}))
	defer srv.Close()
	g := Gateway{Name: "ntfy", Kind: Ntfy, URL: srv.URL + "/t"}
	p := samplePage()
	p.Priority = 3
	testSender(srv.Client()).Send(context.Background(), g, p, nil)
	if prio != "4" {
		t.Errorf("priority = %q want 4", prio)
	}
}

func TestNtfyNoTopic(t *testing.T) {
	// No default topic in the URL path and the recipient has none either.
	g := Gateway{Name: "ntfy", Kind: Ntfy, URL: "https://ntfy.sh"}
	s := testSender(&http.Client{})
	rs := s.Send(context.Background(), g, samplePage(), []Recipient{{Name: "x"}})
	if len(rs) != 1 || rs[0].OK || !strings.Contains(rs[0].Detail, "no topic") {
		t.Fatalf("want a no-topic failure, got %+v", rs)
	}
}

// --- fake SMTP server ------------------------------------------------------

// fakeSMTP is a minimal in-process SMTP server for tests. It understands EHLO, AUTH PLAIN, MAIL,
// RCPT, DATA and QUIT. It records the sender, recipients and message, and can be told to reject a
// given RCPT address to exercise the error path.
type fakeSMTP struct {
	ln         net.Listener
	rejectRcpt string

	mu    sync.Mutex
	from  string
	rcpts []string
	data  string
	auth  string // the raw base64 AUTH PLAIN argument we received, if any
}

func newFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{ln: ln}
	go f.serve()
	return f
}

func (f *fakeSMTP) addr() (host string, port int) {
	a := f.ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", a.Port
}

func (f *fakeSMTP) close() { f.ln.Close() }

func (f *fakeSMTP) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handle(conn)
	}
}

func (f *fakeSMTP) handle(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	w := func(s string) { fmt.Fprintf(conn, "%s\r\n", s) }
	w("220 fake ESMTP")
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		up := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
			w("250-fake greets you")
			w("250 AUTH PLAIN")
		case strings.HasPrefix(up, "AUTH PLAIN"):
			f.mu.Lock()
			f.auth = strings.TrimSpace(line[len("AUTH PLAIN"):])
			f.mu.Unlock()
			w("235 2.7.0 authentication succeeded")
		case strings.HasPrefix(up, "MAIL FROM:"):
			f.mu.Lock()
			f.from = line[len("MAIL FROM:"):]
			f.mu.Unlock()
			w("250 OK")
		case strings.HasPrefix(up, "RCPT TO:"):
			addr := line[len("RCPT TO:"):]
			if f.rejectRcpt != "" && strings.Contains(addr, f.rejectRcpt) {
				w("550 no such user")
				continue
			}
			f.mu.Lock()
			f.rcpts = append(f.rcpts, addr)
			f.mu.Unlock()
			w("250 OK")
		case up == "DATA":
			w("354 end with <CR><LF>.<CR><LF>")
			var sb strings.Builder
			for {
				dl, err := br.ReadString('\n')
				if err != nil {
					return
				}
				if dl == ".\r\n" || dl == ".\n" {
					break
				}
				sb.WriteString(dl)
			}
			f.mu.Lock()
			f.data = sb.String()
			f.mu.Unlock()
			w("250 OK queued")
		case up == "QUIT":
			w("221 bye")
			return
		case up == "RSET":
			w("250 OK")
		default:
			w("250 OK")
		}
	}
}

func (f *fakeSMTP) gatewayDial() func(ctx context.Context, host string, port int) (net.Conn, error) {
	return func(ctx context.Context, host string, port int) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", f.ln.Addr().String())
	}
}

func (f *fakeSMTP) snapshot() (from, data string, rcpts []string, auth string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.from, f.data, append([]string(nil), f.rcpts...), f.auth
}

// --- e-mail ----------------------------------------------------------------

func TestEmailPerRecipient(t *testing.T) {
	f := newFakeSMTP(t)
	defer f.close()
	host, port := f.addr()

	g := Gateway{Name: "pager", Kind: Email, From: "cad@example.com",
		SMTPHost: host, SMTPPort: port, SMTPUser: "u", SMTPPassword: "p"}
	s := &Sender{Now: fixedNow, Dial: f.gatewayDial()}
	rs := s.Send(context.Background(), g, samplePage(), []Recipient{
		{Email: "pat@example.com"},
	})
	if len(rs) != 1 || !rs[0].OK {
		t.Fatalf("want one OK, got %+v", rs)
	}
	if rs[0].To != "p***@example.com" {
		t.Errorf("masked To = %q", rs[0].To)
	}
	from, data, rcpts, auth := f.snapshot()
	if !strings.Contains(from, "cad@example.com") {
		t.Errorf("MAIL FROM = %q", from)
	}
	if len(rcpts) != 1 || !strings.Contains(rcpts[0], "pat@example.com") {
		t.Errorf("RCPT = %v", rcpts)
	}
	if auth == "" {
		t.Errorf("expected AUTH PLAIN to have been used over the loopback")
	}
	if !strings.Contains(data, "Subject:") || !strings.Contains(data, "STRUCTURE FIRE") {
		t.Errorf("message lacks subject: %q", data)
	}
	if !strings.Contains(data, "Message-ID:") || !strings.Contains(data, "Date:") {
		t.Errorf("message lacks Date/Message-ID: %q", data)
	}
	if !strings.Contains(data, "https://example.com/r/pg-1") {
		t.Errorf("message lacks reply link: %q", data)
	}
	if !strings.Contains(data, "215 Church Ave SW") {
		t.Errorf("message lacks address: %q", data)
	}
}

func TestEmailRcptRejected(t *testing.T) {
	f := newFakeSMTP(t)
	f.rejectRcpt = "bad@example.com"
	defer f.close()
	host, port := f.addr()
	g := Gateway{Name: "pager", Kind: Email, From: "cad@example.com", SMTPHost: host, SMTPPort: port}
	s := &Sender{Now: fixedNow, Dial: f.gatewayDial()}
	rs := s.Send(context.Background(), g, samplePage(), []Recipient{{Email: "bad@example.com"}})
	if len(rs) != 1 || rs[0].OK {
		t.Fatalf("want one failure, got %+v", rs)
	}
	if !strings.Contains(rs[0].Detail, "rejected the recipient") {
		t.Errorf("detail = %q", rs[0].Detail)
	}
}

func TestEmailMissingAddress(t *testing.T) {
	s := &Sender{Now: fixedNow}
	g := Gateway{Name: "pager", Kind: Email, From: "cad@example.com", SMTPHost: "127.0.0.1", SMTPPort: 25}
	rs := s.Send(context.Background(), g, samplePage(), []Recipient{{Name: "no email"}})
	if len(rs) != 1 || rs[0].OK || !strings.Contains(rs[0].Detail, "no e-mail") {
		t.Fatalf("want a no-address failure, got %+v", rs)
	}
}

// --- IamResponding / Active911 ---------------------------------------------

func TestIAmRespondingBodyFormat(t *testing.T) {
	f := newFakeSMTP(t)
	defer f.close()
	host, port := f.addr()
	g := Gateway{Name: "iar", Kind: IAmResponding, From: "cad@example.com", To: "agency@iamresponding.com",
		SMTPHost: host, SMTPPort: port}
	s := &Sender{Now: fixedNow, Dial: f.gatewayDial()}
	rs := s.Send(context.Background(), g, samplePage(), nil)
	if len(rs) != 1 || !rs[0].OK {
		t.Fatalf("want one OK, got %+v", rs)
	}
	_, data, rcpts, _ := f.snapshot()
	if len(rcpts) != 1 || !strings.Contains(rcpts[0], "agency@iamresponding.com") {
		t.Errorf("RCPT = %v", rcpts)
	}
	for _, want := range []string{
		"Nature: STRUCTURE FIRE 215 Church Ave SW",
		"Address: 215 Church Ave SW, Roanoke VA 24011",
		"Latitude: 37.271", "Longitude: -79.9414",
		"Units: E1, L1", "Incident ID: 26-000123", "Priority: 1",
		"Comments: Smoke showing from the second floor",
	} {
		if !strings.Contains(data, want) {
			t.Errorf("IamResponding body lacks %q\n%s", want, data)
		}
	}
}

func TestActive911BodyFormat(t *testing.T) {
	f := newFakeSMTP(t)
	defer f.close()
	host, port := f.addr()
	g := Gateway{Name: "a911", Kind: Active911, From: "cad@example.com", To: "agency@active911.com",
		SMTPHost: host, SMTPPort: port}
	s := &Sender{Now: fixedNow, Dial: f.gatewayDial()}
	rs := s.Send(context.Background(), g, samplePage(), nil)
	if len(rs) != 1 || !rs[0].OK {
		t.Fatalf("want one OK, got %+v", rs)
	}
	_, data, _, _ := f.snapshot()
	for _, want := range []string{
		"CALL: STRUCTURE FIRE 215 Church Ave SW",
		"ADDR: 215 Church Ave SW",
		"CITY: Roanoke VA 24011",
		"ID: 26-000123", "PRI: 1", "UNIT: E1, L1",
		"INFO: Smoke showing from the second floor",
	} {
		if !strings.Contains(data, want) {
			t.Errorf("Active911 body lacks %q\n%s", want, data)
		}
	}
}

func TestActive911UsesCoordsWhenNoAddress(t *testing.T) {
	lat, lon := 44.720515, -122.169883
	p := Page{Subject: "BOAT FIRE", Lat: &lat, Lon: &lon, Text: "info"}
	body := active911Body(p)
	if !strings.Contains(body, "ADDR: 44.720515,-122.169883") {
		t.Errorf("want lat,lon in ADDR, got:\n%s", body)
	}
}

// --- Check -----------------------------------------------------------------

func TestCheck(t *testing.T) {
	ac := "AC" + strings.Repeat("a", 32)
	cases := []struct {
		name string
		g    Gateway
		ok   bool
	}{
		{"webhook ok", Gateway{Kind: Webhook, URL: "https://hook.example.com"}, true},
		{"webhook plain http", Gateway{Kind: Webhook, URL: "http://hook.example.com"}, false},
		{"webhook localhost http ok", Gateway{Kind: Webhook, URL: "http://127.0.0.1:8080/x"}, true},
		{"webhook no url", Gateway{Kind: Webhook}, false},
		{"twilio ok", Gateway{Kind: Twilio, Account: ac, Token: "t", From: "+15555550000"}, true},
		{"twilio bad sid", Gateway{Kind: Twilio, Account: "XX", Token: "t", From: "+15555550000"}, false},
		{"twilio bad phone", Gateway{Kind: Twilio, Account: ac, Token: "t", From: "5555550000"}, false},
		{"twilio no token", Gateway{Kind: Twilio, Account: ac, From: "+15555550000"}, false},
		{"ntfy ok", Gateway{Kind: Ntfy, URL: "https://ntfy.sh"}, true},
		{"ntfy plain http", Gateway{Kind: Ntfy, URL: "http://ntfy.example.com"}, false},
		{"email ok tls", Gateway{Kind: Email, From: "a@b.com", SMTPHost: "mail.b.com", SMTPPort: 587, SMTPUser: "u", SMTPStartTLS: true}, true},
		{"email ok 465", Gateway{Kind: Email, From: "a@b.com", SMTPHost: "mail.b.com", SMTPPort: 465, SMTPUser: "u"}, true},
		{"email auth plaintext refused", Gateway{Kind: Email, From: "a@b.com", SMTPHost: "mail.b.com", SMTPPort: 587, SMTPUser: "u"}, false},
		{"email auth localhost ok", Gateway{Kind: Email, From: "a@b.com", SMTPHost: "localhost", SMTPPort: 25, SMTPUser: "u"}, true},
		{"email no host", Gateway{Kind: Email, From: "a@b.com", SMTPPort: 25}, false},
		{"email bad from", Gateway{Kind: Email, From: "nope", SMTPHost: "mail.b.com", SMTPPort: 25}, false},
		{"iar ok", Gateway{Kind: IAmResponding, From: "a@b.com", To: "agency@iar.com", SMTPHost: "mail.b.com", SMTPPort: 25}, true},
		{"iar no to", Gateway{Kind: IAmResponding, From: "a@b.com", SMTPHost: "mail.b.com", SMTPPort: 25}, false},
		{"a911 ok", Gateway{Kind: Active911, From: "a@b.com", To: "agency@a911.com", SMTPHost: "mail.b.com", SMTPPort: 25}, true},
		{"unknown kind", Gateway{Kind: "smoke signals"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.g.Check()
			if c.ok && err != nil {
				t.Errorf("want ok, got %v", err)
			}
			if !c.ok && err == nil {
				t.Errorf("want an error, got nil")
			}
		})
	}
}

func TestRedacted(t *testing.T) {
	g := Gateway{Kind: Twilio, Token: "secret-token", SMTPPassword: "hunter2"}
	r := g.Redacted()
	if r.Token == "secret-token" || r.SMTPPassword == "hunter2" {
		t.Errorf("secrets not blanked: %+v", r)
	}
	if g.Token != "secret-token" {
		t.Errorf("Redacted mutated the original")
	}
}

// --- Mask ------------------------------------------------------------------

func TestMask(t *testing.T) {
	cases := map[string]string{
		"+15555550123":     "***-***-0123",
		"(555) 555-0199":   "***-***-0199",
		"jane@example.com": "j***@example.com",
		"":                 "",
		"123":              "***",
		"plainstring":      "***",
	}
	for in, want := range cases {
		if got := Mask(in); got != want {
			t.Errorf("Mask(%q) = %q want %q", in, got, want)
		}
	}
}

// --- Kinds -----------------------------------------------------------------

func TestKinds(t *testing.T) {
	ks := Kinds()
	if len(ks) != 6 {
		t.Fatalf("want 6 kinds, got %d", len(ks))
	}
	seen := map[string]bool{}
	for _, k := range ks {
		if k.Kind == "" || k.Name == "" || len(k.Fields) == 0 || k.Note == "" {
			t.Errorf("incomplete KindInfo: %+v", k)
		}
		seen[k.Kind] = true
	}
	for _, want := range []string{Webhook, Email, Twilio, Ntfy, IAmResponding, Active911} {
		if !seen[want] {
			t.Errorf("Kinds is missing %q", want)
		}
	}
}
