// Package paging delivers pages (short dispatch alerts) from the 911 CAD to the responders who
// turn out to calls: volunteer and off-duty firefighters and EMTs. A page is a one-line "what and
// where" with a tap-to-answer link, sent the moment units are dispatched.
//
// A Gateway is one configured way to send, picked in Admin › Integrations › Paging. The supported
// kinds are a generic JSON webhook, e-mail (SMTP), Twilio SMS, ntfy push, and two first-responder
// services that take dispatch by e-mail, IamResponding and Active911.
//
// What was tested: the HTTP kinds (webhook, Twilio, ntfy) and the SMTP e-mail kinds are exercised in
// paging_test.go against local httptest servers and an in-process fake SMTP server that speaks a
// minimal EHLO/AUTH/MAIL/RCPT/DATA/QUIT dialogue. The real Twilio, ntfy, IamResponding and Active911
// services were not contacted; the request shapes and the IamResponding/Active911 body formats follow
// those services' public documentation (URLs cited at the Sender methods). This is not an official,
// vendor-certified integration with any of them.
package paging

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// The channel kinds a Gateway can be. Each is a separate way to reach responders.
const (
	Webhook       = "webhook"
	Email         = "email"
	Twilio        = "twilio"
	Ntfy          = "ntfy"
	IAmResponding = "iamresponding"
	Active911     = "active911"
)

// Gateway is one configured way to send pages, set by an administrator.
type Gateway struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`

	URL   string `json:"url,omitempty"`   // webhook URL; ntfy server (default https://ntfy.sh)
	Token string `json:"token,omitempty"` // webhook Bearer token; ntfy access token; Twilio auth token

	Account string `json:"account,omitempty"` // Twilio account SID (starts AC)
	From    string `json:"from,omitempty"`    // e-mail From address; Twilio From number (+1...)

	SMTPHost     string `json:"smtpHost,omitempty"`
	SMTPPort     int    `json:"smtpPort,omitempty"`
	SMTPUser     string `json:"smtpUser,omitempty"`
	SMTPPassword string `json:"smtpPassword,omitempty"`
	SMTPStartTLS bool   `json:"smtpStartTls,omitempty"` // STARTTLS on a plain port; implicit TLS is used on 465

	To string `json:"to,omitempty"` // IamResponding / Active911: the agency's dispatch e-mail address the service gives you
}

var (
	rePhone = regexp.MustCompile(`^\+[1-9]\d{6,14}$`) // E.164: + then up to 15 digits, no leading zero
	reEmail = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	reSID   = regexp.MustCompile(`^AC[0-9a-fA-F]{32}$`) // Twilio account SID
)

// localHost reports whether a host part is the loopback, where sending a secret in the clear is fine
// because nothing leaves the machine. Anything else must use TLS.
func localHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// httpsOrLoopback reports whether a URL is https, or http pointed only at the loopback.
func httpsOrLoopback(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	return u.Scheme == "http" && localHost(u.Hostname())
}

// Check says, in plain English, what is wrong with a gateway, or nil when it is ready to use. It
// checks the fields each kind needs, insists URLs are https (except the loopback), and validates the
// Twilio SID and phone numbers.
func (g Gateway) Check() error {
	switch g.Kind {
	case Webhook:
		if g.URL == "" {
			return errors.New("a webhook needs its URL")
		}
		if !httpsOrLoopback(g.URL) {
			return errors.New("the webhook URL must be https (http is allowed only for 127.0.0.1 or localhost)")
		}
	case Email:
		if err := checkSMTP(g); err != nil {
			return err
		}
		if g.From == "" {
			return errors.New("e-mail paging needs a From address")
		}
		if !reEmail.MatchString(g.From) {
			return errors.New("the From address does not look like an e-mail address")
		}
	case Twilio:
		if g.Account == "" || g.Token == "" {
			return errors.New("Twilio needs the account SID and auth token")
		}
		if !reSID.MatchString(g.Account) {
			return errors.New("a Twilio account SID starts with AC and is 34 characters")
		}
		if !rePhone.MatchString(g.From) {
			return errors.New("the Twilio From must be a phone number in E.164 form, such as +15555550123")
		}
	case Ntfy:
		if g.URL == "" {
			return errors.New("ntfy needs its server URL, such as https://ntfy.sh")
		}
		if !httpsOrLoopback(g.URL) {
			return errors.New("the ntfy server URL must be https (http is allowed only for 127.0.0.1 or localhost)")
		}
	case IAmResponding, Active911:
		if err := checkSMTP(g); err != nil {
			return err
		}
		if g.From == "" || !reEmail.MatchString(g.From) {
			return errors.New("this service needs a From address to send from")
		}
		if g.To == "" || !reEmail.MatchString(g.To) {
			return errors.New("this service needs the agency's dispatch e-mail address it gave you")
		}
	default:
		return fmt.Errorf("unknown paging kind %q", g.Kind)
	}
	return nil
}

// checkSMTP checks the SMTP settings shared by the e-mail kinds.
func checkSMTP(g Gateway) error {
	if g.SMTPHost == "" {
		return errors.New("the SMTP server host is missing")
	}
	if g.SMTPPort <= 0 || g.SMTPPort > 65535 {
		return errors.New("the SMTP port must be between 1 and 65535")
	}
	// AUTH over a plain-text connection would leak the password, so refuse it unless the server is the
	// loopback or the connection is encrypted (STARTTLS, or implicit TLS on 465).
	if g.SMTPUser != "" && !g.SMTPStartTLS && g.SMTPPort != 465 && !localHost(g.SMTPHost) {
		return errors.New("SMTP sign-in over a plain connection is refused; turn on STARTTLS or use port 465")
	}
	return nil
}

// Redacted returns a copy of the gateway with its secrets blanked, for showing in the GUI.
func (g Gateway) Redacted() Gateway {
	if g.Token != "" {
		g.Token = "••••"
	}
	if g.SMTPPassword != "" {
		g.SMTPPassword = "••••"
	}
	return g
}

// Recipient is one responder a page can reach. A recipient need only have the fields their gateway
// uses: a phone for Twilio, an e-mail for e-mail, a topic (or the gateway default) for ntfy.
type Recipient struct {
	Name  string
	Phone string // E.164
	Email string
	Topic string // ntfy topic
}

// Page is the alert to deliver. Subject is the headline ("STRUCTURE FIRE 215 Church Ave SW"); Text is
// the full page body. ReplyURL, when set, is the link a responder taps to answer Responding / Not
// available.
type Page struct {
	ID       string
	Incident string
	Subject  string
	Text     string
	Address  string
	Lat, Lon *float64
	Units    []string
	Priority int
	ReplyURL string
	At       time.Time
}

// Result is the outcome of one delivery attempt, one per recipient (or one for the whole-gateway
// kinds). To is who it went to, with phones and e-mails masked.
type Result struct {
	Gateway string
	Kind    string
	To      string
	OK      bool
	Detail  string
	At      time.Time
}

// Sender sends pages. The zero value works; the fields let tests point SMTP and Twilio at local
// servers and fix the clock.
type Sender struct {
	HTTP *http.Client
	// Dial opens the SMTP connection. Tests set it to reach a local listener; in production it is nil
	// and a plain net.Dialer to the gateway's host:port is used.
	Dial       func(ctx context.Context, host string, port int) (net.Conn, error)
	Now        func() time.Time
	TwilioBase string // default https://api.twilio.com
}

func (s *Sender) httpClient() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (s *Sender) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Sender) twilioBase() string {
	if s.TwilioBase != "" {
		return strings.TrimRight(s.TwilioBase, "/")
	}
	return "https://api.twilio.com"
}

// Send delivers a page through one gateway and returns one Result per delivery attempt. It never
// panics: a bad gateway or a failing server becomes a Result with OK false and a plain-English
// Detail. Webhook, IamResponding and Active911 send once (one Result). E-mail, Twilio and ntfy send
// per recipient; ntfy groups recipients by topic (one send per distinct topic, falling back to the
// topic in the server URL path when a recipient has none).
func (s *Sender) Send(ctx context.Context, g Gateway, p Page, to []Recipient) []Result {
	switch g.Kind {
	case Webhook:
		return []Result{s.sendWebhook(ctx, g, p, to)}
	case Email:
		return s.sendEmail(ctx, g, p, to)
	case Twilio:
		return s.sendTwilio(ctx, g, p, to)
	case Ntfy:
		return s.sendNtfy(ctx, g, p, to)
	case IAmResponding:
		return []Result{s.sendServiceEmail(ctx, g, p, iamRespondingBody(p))}
	case Active911:
		return []Result{s.sendServiceEmail(ctx, g, p, active911Body(p))}
	default:
		return []Result{s.fail(g, "", fmt.Sprintf("unknown paging kind %q", g.Kind))}
	}
}

func (s *Sender) result(g Gateway, to string, ok bool, detail string) Result {
	return Result{Gateway: g.Name, Kind: g.Kind, To: to, OK: ok, Detail: detail, At: s.now()}
}

func (s *Sender) fail(g Gateway, to, detail string) Result {
	return s.result(g, to, false, detail)
}

// --- webhook ---------------------------------------------------------------

// sendWebhook POSTs the page as JSON. Any 2xx is success. A Bearer token is sent when set.
//
// Shape: {id, incident, subject, text, address, lat, lon, units, priority, replyUrl, at,
// recipients:[{name, phone, email}]}.
func (s *Sender) sendWebhook(ctx context.Context, g Gateway, p Page, to []Recipient) Result {
	rs := make([]map[string]string, 0, len(to))
	for _, r := range to {
		rs = append(rs, map[string]string{"name": r.Name, "phone": r.Phone, "email": r.Email})
	}
	body := map[string]any{
		"id": p.ID, "incident": p.Incident, "subject": p.Subject, "text": p.Text,
		"address": p.Address, "lat": p.Lat, "lon": p.Lon, "units": p.Units,
		"priority": p.Priority, "replyUrl": p.ReplyURL, "at": p.At.Format(time.RFC3339),
		"recipients": rs,
	}
	b, err := json.Marshal(body)
	if err != nil {
		return s.fail(g, "webhook", "could not build the JSON: "+err.Error())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.URL, bytes.NewReader(b))
	if err != nil {
		return s.fail(g, "webhook", err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return s.fail(g, "webhook", err.Error())
	}
	defer resp.Body.Close()
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	if resp.StatusCode/100 == 2 {
		return s.result(g, "webhook", true, fmt.Sprintf("HTTP %d", resp.StatusCode))
	}
	return s.fail(g, "webhook", fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet))))
}

// --- Twilio ----------------------------------------------------------------

// sendTwilio sends one SMS per recipient through the Twilio Programmable Messaging REST API:
// POST {base}/2010-04-01/Accounts/{AccountSid}/Messages.json, form-encoded To, From and Body, with
// HTTP Basic auth of SID:token. A 2xx is success; on failure the JSON error {code, message,
// more_info, status} is parsed for the message.
//
// See https://www.twilio.com/docs/sms/send-messages and the Messages resource
// https://www.twilio.com/docs/sms/api/message (checked October 2026).
func (s *Sender) sendTwilio(ctx context.Context, g Gateway, p Page, to []Recipient) []Result {
	body := smsBody(p)
	endpoint := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Messages.json", s.twilioBase(), g.Account)
	out := make([]Result, 0, len(to))
	for _, r := range to {
		masked := Mask(r.Phone)
		if r.Phone == "" {
			out = append(out, s.fail(g, masked, "this recipient has no phone number"))
			continue
		}
		form := url.Values{"To": {r.Phone}, "From": {g.From}, "Body": {body}}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
		if err != nil {
			out = append(out, s.fail(g, masked, err.Error()))
			continue
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetBasicAuth(g.Account, g.Token)
		resp, err := s.httpClient().Do(req)
		if err != nil {
			out = append(out, s.fail(g, masked, err.Error()))
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if resp.StatusCode/100 == 2 {
			out = append(out, s.result(g, masked, true, "queued"))
			continue
		}
		detail := fmt.Sprintf("HTTP %d", resp.StatusCode)
		var e struct {
			Code     int    `json:"code"`
			Message  string `json:"message"`
			MoreInfo string `json:"more_info"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Message != "" {
			detail = fmt.Sprintf("Twilio %d: %s", e.Code, e.Message)
		}
		out = append(out, s.fail(g, masked, detail))
	}
	return out
}

// smsBody builds the SMS text: subject, a short body and the reply link, trimmed to 1600 characters
// (Twilio's limit for a single logical message, sent as concatenated segments).
func smsBody(p Page) string {
	var b strings.Builder
	b.WriteString(p.Subject)
	if p.Text != "" {
		b.WriteString("\n")
		b.WriteString(p.Text)
	}
	if p.ReplyURL != "" {
		b.WriteString("\nReply: ")
		b.WriteString(p.ReplyURL)
	}
	return trim(b.String(), 1600)
}

// trim shortens s to at most n characters (runes), ending with an ellipsis when cut.
func trim(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// --- ntfy ------------------------------------------------------------------

// sendNtfy publishes to ntfy: POST {server}/{topic} with the page text as the body and the headers
// Title, Priority (5 for a priority-1 call, else 4), Tags (rotating_light) and Click (the reply URL);
// an access token, when set, goes in Authorization: Bearer. ntfy priorities run 1 (min) to 5 (max).
// One publish per distinct topic; a recipient with no topic uses the topic in the server URL path.
//
// See https://docs.ntfy.sh/publish/ (checked October 2026).
func (s *Sender) sendNtfy(ctx context.Context, g Gateway, p Page, to []Recipient) []Result {
	server, defTopic := splitNtfyURL(g.URL)

	// Collect the distinct topics, preferring recipient topics and falling back to the default.
	order := []string{}
	seen := map[string]bool{}
	add := func(t string) {
		if t == "" || seen[t] {
			return
		}
		seen[t] = true
		order = append(order, t)
	}
	for _, r := range to {
		if r.Topic != "" {
			add(r.Topic)
		} else {
			add(defTopic)
		}
	}
	if len(order) == 0 {
		add(defTopic)
	}
	if len(order) == 0 {
		return []Result{s.fail(g, "ntfy", "no topic: set the recipient's topic or put a default topic in the ntfy URL path")}
	}

	prio := "4"
	if p.Priority == 1 {
		prio = "5"
	}
	out := make([]Result, 0, len(order))
	for _, topic := range order {
		endpoint := server + "/" + topic
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(p.Text))
		if err != nil {
			out = append(out, s.fail(g, topic, err.Error()))
			continue
		}
		req.Header.Set("Title", oneLine(p.Subject))
		req.Header.Set("Priority", prio)
		req.Header.Set("Tags", "rotating_light")
		if p.ReplyURL != "" {
			req.Header.Set("Click", p.ReplyURL)
		}
		if g.Token != "" {
			req.Header.Set("Authorization", "Bearer "+g.Token)
		}
		resp, err := s.httpClient().Do(req)
		if err != nil {
			out = append(out, s.fail(g, topic, err.Error()))
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		if resp.StatusCode/100 == 2 {
			out = append(out, s.result(g, topic, true, fmt.Sprintf("HTTP %d", resp.StatusCode)))
			continue
		}
		out = append(out, s.fail(g, topic, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))))
	}
	return out
}

// splitNtfyURL splits an ntfy URL into the server base and an optional default topic in the path.
// "https://ntfy.sh" -> ("https://ntfy.sh", ""); "https://ntfy.sh/dispatch" -> (..., "dispatch").
func splitNtfyURL(raw string) (server, topic string) {
	raw = strings.TrimRight(raw, "/")
	u, err := url.Parse(raw)
	if err != nil {
		return raw, ""
	}
	p := strings.Trim(u.Path, "/")
	u.Path = ""
	server = strings.TrimRight(u.String(), "/")
	return server, p
}

// oneLine collapses newlines so a value is safe to put in an HTTP header (ntfy Title).
func oneLine(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}

// --- e-mail ----------------------------------------------------------------

// sendEmail sends one plain-text RFC 5322 message per recipient. One message per recipient (rather
// than one with everyone in BCC) is used on purpose: a page is personal, the To header then shows
// only that responder, and one recipient's bad address or a server rejecting one RCPT does not stop
// the others — each gets its own Result.
func (s *Sender) sendEmail(ctx context.Context, g Gateway, p Page, to []Recipient) []Result {
	out := make([]Result, 0, len(to))
	for _, r := range to {
		masked := Mask(r.Email)
		if r.Email == "" {
			out = append(out, s.fail(g, masked, "this recipient has no e-mail address"))
			continue
		}
		msg := s.buildEmail(g.From, r.Email, p.Subject, emailBody(p))
		if err := s.deliver(ctx, g, []string{r.Email}, msg); err != nil {
			out = append(out, s.fail(g, masked, err.Error()))
			continue
		}
		out = append(out, s.result(g, masked, true, "sent"))
	}
	return out
}

// sendServiceEmail sends one message to the agency's dispatch address (g.To) for IamResponding or
// Active911, with a body the service documents. One Result for the one delivery.
func (s *Sender) sendServiceEmail(ctx context.Context, g Gateway, p Page, body string) Result {
	masked := Mask(g.To)
	subject := p.Subject
	if subject == "" {
		subject = "Dispatch"
	}
	msg := s.buildEmail(g.From, g.To, subject, body)
	if err := s.deliver(ctx, g, []string{g.To}, msg); err != nil {
		return s.fail(g, masked, err.Error())
	}
	return s.result(g, masked, true, "sent")
}

// emailBody is the plain-text body of a page e-mail: the full text, the address, and the reply link.
func emailBody(p Page) string {
	var b strings.Builder
	b.WriteString(p.Text)
	if p.Address != "" {
		b.WriteString("\n\n")
		b.WriteString(p.Address)
	}
	if p.ReplyURL != "" {
		b.WriteString("\n\nRespond: ")
		b.WriteString(p.ReplyURL)
	}
	return b.String()
}

// buildEmail assembles an RFC 5322 plain-text message with a Date, a Message-ID, a Subject encoded
// for any non-ASCII characters, and a single To. Lines use CRLF as RFC 5322 requires.
func (s *Sender) buildEmail(from, to, subject, body string) []byte {
	now := s.now()
	var h strings.Builder
	fmt.Fprintf(&h, "From: %s\r\n", from)
	fmt.Fprintf(&h, "To: %s\r\n", to)
	fmt.Fprintf(&h, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	fmt.Fprintf(&h, "Date: %s\r\n", now.Format(time.RFC1123Z))
	fmt.Fprintf(&h, "Message-ID: <%s@bitdispatcher>\r\n", messageID())
	h.WriteString("MIME-Version: 1.0\r\n")
	h.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	h.WriteString("\r\n")
	h.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	return []byte(h.String())
}

// messageID returns a random token for the Message-ID header.
func messageID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// deliver opens an SMTP connection to the gateway's server and sends one message. It uses implicit
// TLS on port 465, STARTTLS when asked, and AUTH PLAIN when a user is set. The 10-second timeout
// covers the whole exchange.
func (s *Sender) deliver(ctx context.Context, g Gateway, rcpts []string, msg []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := s.dial(ctx, g.SMTPHost, g.SMTPPort)
	if err != nil {
		return fmt.Errorf("could not reach the SMTP server: %w", err)
	}
	// Close the raw conn on any early error; smtp.Client.Quit closes it on the happy path.
	closed := false
	defer func() {
		if !closed {
			conn.Close()
		}
	}()

	serverName := g.SMTPHost
	if g.SMTPPort == 465 { // implicit TLS: wrap the connection before speaking SMTP
		conn = tls.Client(conn, &tls.Config{ServerName: serverName})
	}
	if dl, ok := ctx.Deadline(); ok {
		conn.SetDeadline(dl)
	}

	c, err := smtp.NewClient(conn, serverName)
	if err != nil {
		return err
	}
	defer c.Close()

	if g.SMTPStartTLS && g.SMTPPort != 465 {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: serverName}); err != nil {
				return fmt.Errorf("STARTTLS failed: %w", err)
			}
		} else {
			return errors.New("the server does not offer STARTTLS")
		}
	}

	if g.SMTPUser != "" {
		// Guard again at send time: never AUTH in the clear off the loopback.
		secure := g.SMTPStartTLS || g.SMTPPort == 465 || localHost(g.SMTPHost)
		if !secure {
			return errors.New("refusing to sign in to SMTP over a plain connection")
		}
		auth := smtp.PlainAuth("", g.SMTPUser, g.SMTPPassword, serverName)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("SMTP sign-in was refused: %w", err)
		}
	}

	if err := c.Mail(g.From); err != nil {
		return fmt.Errorf("the server rejected the sender: %w", err)
	}
	for _, rcpt := range rcpts {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("the server rejected the recipient: %w", err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	if err := c.Quit(); err != nil {
		return err
	}
	closed = true
	return nil
}

// dial opens the TCP connection for SMTP, using the Sender's Dial hook when tests set one.
func (s *Sender) dial(ctx context.Context, host string, port int) (net.Conn, error) {
	if s.Dial != nil {
		return s.Dial(ctx, host, port)
	}
	var d net.Dialer
	return d.DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
}

// --- IamResponding and Active911 bodies ------------------------------------

// iamRespondingBody formats a page the way IamResponding's public "Best Practices for CAD SMTP/SNPP
// Dispatching" guide shows: labelled lines in the body, one message per dispatch, latitude and
// longitude when known. Their parser maps these labels on the agency's account; the labels here match
// their sample (Nature, Address, City/Township, State, Comments, Latitude, Longitude, Units, Incident
// ID, Priority).
//
// See https://support.iamresponding.com/hc/en-us/articles/51327833814036-Best-Practices-for-CAD-SMTP-SNPP-Dispatching
// (checked October 2026). This produces a well-labelled e-mail; it is not an official certified feed.
func iamRespondingBody(p Page) string {
	var b strings.Builder
	line := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%s: %s\n", k, v)
		}
	}
	line("Nature", p.Subject)
	line("Address", p.Address)
	if p.Lat != nil && p.Lon != nil {
		line("Latitude", ftoa(*p.Lat))
		line("Longitude", ftoa(*p.Lon))
	}
	if len(p.Units) > 0 {
		line("Units", strings.Join(p.Units, ", "))
	}
	line("Incident ID", p.Incident)
	if p.Priority > 0 {
		line("Priority", fmt.Sprintf("%d", p.Priority))
	}
	line("Comments", p.Text)
	return strings.TrimRight(b.String(), "\n")
}

// active911Body formats a page in Active911's documented "Cadpage" NAME: VALUE format: at least CALL,
// ADDR and CITY, with optional ID, PRI, UNIT, MAP and INFO. When there is no street address but there
// are coordinates, "lat,lon" goes in ADDR, which Active911 documents as an accepted location form.
// The agency tells Active911 it is sending Cadpage; the service's parser maps these fields.
//
// See https://active911.com/documentation/setting-a-messaging-format/ (checked October 2026). This is
// the common labelled format; it is not an official certified integration.
func active911Body(p Page) string {
	var b strings.Builder
	line := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%s: %s\n", k, v)
		}
	}
	call := p.Subject
	if call == "" {
		call = "Dispatch"
	}
	line("CALL", call)
	addr, city := splitAddressCity(p.Address)
	if addr == "" && p.Lat != nil && p.Lon != nil {
		addr = ftoa(*p.Lat) + "," + ftoa(*p.Lon)
	}
	line("ADDR", addr)
	line("CITY", city)
	line("ID", p.Incident)
	if p.Priority > 0 {
		line("PRI", fmt.Sprintf("%d", p.Priority))
	}
	if len(p.Units) > 0 {
		line("UNIT", strings.Join(p.Units, ", "))
	}
	line("INFO", p.Text)
	return strings.TrimRight(b.String(), "\n")
}

// splitAddressCity makes a best effort at pulling a city out of a one-line address such as
// "215 Church Ave SW, Roanoke VA 24011" -> ("215 Church Ave SW", "Roanoke VA 24011"). With no comma
// the whole string is the address and the city is blank.
func splitAddressCity(s string) (addr, city string) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, ","); i >= 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
	}
	return s, ""
}

// ftoa prints a coordinate without a trailing ".0" or exponent.
func ftoa(f float64) string {
	return fmt.Sprintf("%g", f)
}

// --- helpers for the GUI ---------------------------------------------------

// Mask hides most of a phone number or e-mail for showing in a Result: a phone becomes
// ***-***-1234 (last four digits) and an e-mail becomes j***@example.com. Anything else is returned
// unchanged, and the empty string stays empty.
func Mask(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.LastIndex(s, "@"); i > 0 {
		local, domain := s[:i], s[i:]
		first := local[:1]
		return first + "***" + domain
	}
	digits := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) >= 4 {
		return "***-***-" + string(digits[len(digits)-4:])
	}
	return "***"
}

// KindInfo describes a paging kind for the Admin › Integrations › Paging GUI: its machine kind, a
// friendly name, the Gateway fields it uses, and a short note.
type KindInfo struct {
	Kind   string
	Name   string
	Fields []string
	Note   string
}

// Kinds returns the paging kinds in the order to show them, with the fields each uses.
func Kinds() []KindInfo {
	return []KindInfo{
		{Webhook, "Webhook", []string{"URL", "Token"},
			"POSTs the page as JSON to your URL, with an optional Bearer token."},
		{Email, "Email / pager", []string{"From", "SMTPHost", "SMTPPort", "SMTPUser", "SMTPPassword", "SMTPStartTLS"},
			"Plain-text e-mail to each responder, good for carrier e-mail-to-SMS pager addresses."},
		{Twilio, "Twilio SMS", []string{"Account", "Token", "From"},
			"Text message to each responder's phone through your Twilio account."},
		{Ntfy, "ntfy push", []string{"URL", "Token"},
			"Push notification to an ntfy topic; set a default topic in the URL path or per responder."},
		{IAmResponding, "IamResponding", []string{"To", "From", "SMTPHost", "SMTPPort", "SMTPUser", "SMTPPassword", "SMTPStartTLS"},
			"E-mails the dispatch to your IamResponding agency address in their documented labelled format."},
		{Active911, "Active911", []string{"To", "From", "SMTPHost", "SMTPPort", "SMTPUser", "SMTPPassword", "SMTPStartTLS"},
			"E-mails the dispatch to your Active911 agency address in the Cadpage labelled format."},
	}
}
