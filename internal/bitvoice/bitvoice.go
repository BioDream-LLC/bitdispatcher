// Package bitvoice connects bitdispatcher to BitVoice through every hook BitVoice offers a CAD
// (docs/BITVOICE.md): its event stream and REST API (pulled), its EIDO and radio webhooks (pushed), and the
// actions a dispatcher can take on a 911 call. bitdispatcher never needs BitVoice; with no address
// configured this package does nothing.
package bitvoice

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// Config is how bitdispatcher reaches BitVoice and how BitVoice proves itself to bitdispatcher.
type Config struct {
	URL         string `json:"url"`         // https://bitvoice.example:8443
	Token       string `json:"token"`       // BitVoice API token (bv_...), secret
	Fingerprint string `json:"fingerprint"` // SHA-256 of BitVoice's certificate when it is self-signed; empty trusts the system roots
	EIDOToken   string `json:"eidoToken"`   // what BitVoice sends with an EIDO (its psap.eidoToken), secret
	RadioSecret string `json:"radioSecret"` // what BitVoice sends with radio events (its radio.cadSecret), secret
}

// Health is what the dispatcher and the administrator see about the link.
type Health struct {
	Configured  bool      `json:"configured"`
	Connected   bool      `json:"connected"`
	Since       time.Time `json:"since,omitzero"`
	LastEvent   time.Time `json:"lastEvent,omitzero"`
	LastError   string    `json:"lastError,omitempty"`
	LastErrorAt time.Time `json:"lastErrorAt,omitzero"`
	LastEIDO    time.Time `json:"lastEido,omitzero"`
	LastPush    time.Time `json:"lastPush,omitzero"` // an EIDO BitVoice pushed (not pulled)
	LastRadio   time.Time `json:"lastRadio,omitzero"`
	LastRadioWH time.Time `json:"lastRadioWebhook,omitzero"`
	Emergency   bool      `json:"emergencyMode"` // BitVoice's emergency mode
	EmergencyBy string    `json:"emergencyModeBy,omitempty"`
	Failures    []Failure `json:"failures"` // BitVoice could not deliver to bitdispatcher
	Events      int       `json:"events"`
	Version     string    `json:"version,omitempty"`
	// PSAPs is the NENA state of the PSAPs BitVoice's ESRP routes to (queue and service), when it routes.
	PSAPs []PSAPState `json:"psaps,omitempty"`
}

// PSAPState is one PSAP's NENA queue and service state, as BitVoice's ESRP sees it.
type PSAPState struct {
	URI     string    `json:"uri"`
	Name    string    `json:"name"`
	Queue   string    `json:"queue,omitempty"` // Active, Inactive, Disabled, Full, Standby, Unreachable ...
	Length  int       `json:"length"`
	Max     int       `json:"max"`
	Service string    `json:"service,omitempty"` // Normal, Unstaffed, Down ...
	Reason  string    `json:"reason,omitempty"`
	QueueAt time.Time `json:"queueAt,omitzero"`
}

// Failure is a delivery BitVoice reported it could not make.
type Failure struct {
	At    time.Time `json:"at"`
	What  string    `json:"what"` // ALI, EIDO, radio.<type>
	Call  string    `json:"call,omitempty"`
	Error string    `json:"error"`
}

// Bridge is the live link.
type Bridge struct {
	eng *cad.Engine
	log *slog.Logger

	mu     sync.Mutex
	cfg    Config
	client *http.Client
	health Health
	cancel context.CancelFunc
	wake   chan struct{}
	lvf    map[string]bool // call|address|position already checked with the LVF
}

// New makes a bridge. Call Configure, then Run.
func New(eng *cad.Engine, log *slog.Logger) *Bridge {
	return &Bridge{eng: eng, log: log, wake: make(chan struct{}, 1), health: Health{Failures: []Failure{}}}
}

// Configure changes the configuration; the stream reconnects with it.
func (b *Bridge) Configure(c Config) {
	c.URL = strings.TrimRight(strings.TrimSpace(c.URL), "/")
	c.Fingerprint = normFingerprint(c.Fingerprint)
	b.mu.Lock()
	b.cfg = c
	b.client = newClient(c.Fingerprint)
	b.health.Configured = c.URL != "" && c.Token != ""
	b.health.Connected = false
	if b.cancel != nil {
		b.cancel()
	}
	b.mu.Unlock()
	select {
	case b.wake <- struct{}{}:
	default:
	}
}

// Config returns the configuration.
func (b *Bridge) Config() Config {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cfg
}

// Health returns a copy of the link's health.
func (b *Bridge) Health() Health {
	b.mu.Lock()
	defer b.mu.Unlock()
	h := b.health
	h.Failures = append([]Failure{}, b.health.Failures...)
	return h
}

func normFingerprint(f string) string {
	f = strings.ToLower(strings.NewReplacer(":", "", " ", "", "sha256", "").Replace(strings.TrimSpace(f)))
	return f
}

// newClient trusts the system roots, or exactly one certificate by fingerprint (BitVoice makes a
// self-signed certificate on first start; pinning it is safer than turning verification off).
func newClient(fingerprint string) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if fingerprint != "" {
		tr.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, // replaced by the pin below
			VerifyPeerCertificate: func(raw [][]byte, _ [][]*x509.Certificate) error {
				if len(raw) == 0 {
					return errors.New("bitvoice: no certificate")
				}
				sum := sha256.Sum256(raw[0])
				if hex.EncodeToString(sum[:]) != fingerprint {
					return errors.New("bitvoice: the certificate is not the pinned one")
				}
				return nil
			},
		}
	}
	return &http.Client{Transport: tr}
}

// Fingerprint connects to a BitVoice address and returns its certificate's SHA-256 fingerprint and
// subject, for an administrator to compare with what BitVoice shows before pinning it.
func Fingerprint(ctx context.Context, rawURL string) (string, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", "", errors.New("give BitVoice's https:// address")
	}
	host := u.Host
	if u.Port() == "" {
		host += ":443"
	}
	d := tls.Dialer{Config: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}} // read only; nothing is sent
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return "", "", err
	}
	defer c.Close()
	certs := c.(*tls.Conn).ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return "", "", errors.New("no certificate")
	}
	sum := sha256.Sum256(certs[0].Raw)
	return hex.EncodeToString(sum[:]), certs[0].Subject.String(), nil
}

func (b *Bridge) setErr(err error) {
	b.mu.Lock()
	b.health.LastError, b.health.LastErrorAt = err.Error(), time.Now()
	b.mu.Unlock()
}

// ---- REST ----

type onBehalfKey struct{}

// OnBehalf names the person a request to BitVoice is made for, sent as X-BitVoice-On-Behalf-Of so
// BitVoice's own records (the 911 exposure log, text calls taken, its audit log) name the dispatcher.
// BitVoice honours it only from a token made to name the person it acts for, and ignores it otherwise.
func OnBehalf(ctx context.Context, person string) context.Context {
	return context.WithValue(ctx, onBehalfKey{}, person)
}

func setOnBehalf(req *http.Request) {
	if p, _ := req.Context().Value(onBehalfKey{}).(string); p != "" {
		req.Header.Set("X-BitVoice-On-Behalf-Of", p)
	}
}

func (b *Bridge) do(ctx context.Context, method, path string, body, out any) error {
	b.mu.Lock()
	cfg, client := b.cfg, b.client
	b.mu.Unlock()
	if cfg.URL == "" || cfg.Token == "" {
		return errors.New("BitVoice is not configured (Admin › Integrations)")
	}
	var rd io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(buf)
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, cfg.URL+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	setOnBehalf(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode/100 != 2 {
		var e struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		json.Unmarshal(data, &e)
		msg := strings.TrimSpace(e.Message + " " + e.Error)
		if msg == "" {
			msg = strings.TrimSpace(string(data))
		}
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return fmt.Errorf("BitVoice said %s: %s", resp.Status, msg)
	}
	if out == nil {
		return nil
	}
	if p, ok := out.(*[]byte); ok {
		*p = data
		return nil
	}
	return json.Unmarshal(data, out)
}

// Test checks the address, the certificate and the token, and returns BitVoice's health summary.
func (b *Bridge) Test(ctx context.Context) (string, error) {
	var calls struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := b.do(ctx, http.MethodGet, "/api/calls", nil, &calls); err != nil {
		return "", err
	}
	var radio struct {
		Items []json.RawMessage `json:"items"`
	}
	radioNote := ""
	if err := b.do(ctx, http.MethodGet, "/api/radio/locations", nil, &radio); err != nil {
		radioNote = "; radio locations not readable (" + err.Error() + ")"
	} else {
		radioNote = fmt.Sprintf("; %d radio positions", len(radio.Items))
	}
	return fmt.Sprintf("Connected: %d calls in progress%s", len(calls.Items), radioNote), nil
}

// ---- the event stream ----

// Run keeps the event stream connected until ctx ends, reconnecting with backoff.
func (b *Bridge) Run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		b.mu.Lock()
		configured := b.cfg.URL != "" && b.cfg.Token != ""
		sctx, cancel := context.WithCancel(ctx)
		b.cancel = cancel
		b.mu.Unlock()
		if !configured {
			cancel()
			select {
			case <-ctx.Done():
				return
			case <-b.wake:
			}
			continue
		}
		start := time.Now()
		err := b.stream(sctx)
		cancel()
		b.mu.Lock()
		b.health.Connected = false
		b.mu.Unlock()
		if ctx.Err() != nil {
			return
		}
		if err != nil && sctx.Err() == nil {
			b.setErr(err)
			b.log.Warn("bitvoice: event stream", "err", err)
		}
		if time.Since(start) > 30*time.Second {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-b.wake:
			backoff = time.Second
		case <-time.After(backoff):
			backoff = min(backoff*2, 30*time.Second)
		}
	}
}

func (b *Bridge) stream(ctx context.Context) error {
	b.mu.Lock()
	cfg, client := b.cfg, b.client
	b.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL+"/api/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("event stream: BitVoice said %s", resp.Status)
	}
	b.mu.Lock()
	b.health.Connected, b.health.Since, b.health.LastError = true, time.Now(), ""
	b.mu.Unlock()
	b.log.Info("bitvoice: event stream connected", "url", cfg.URL)
	go b.resync(ctx)
	// A ping arrives every 20 seconds; silence for a minute means the link is gone.
	alive := make(chan struct{}, 1)
	go func() {
		t := time.NewTimer(60 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-alive:
				t.Reset(60 * time.Second)
			case <-t.C:
				resp.Body.Close()
				return
			}
		}
	}()
	r := bufio.NewReaderSize(resp.Body, 64<<10)
	var event string
	var data bytes.Buffer
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("event stream ended: %w", err)
		}
		select {
		case alive <- struct{}{}:
		default:
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			if event != "" || data.Len() > 0 {
				b.handle(ctx, event, data.Bytes())
			}
			event = ""
			data.Reset()
		case strings.HasPrefix(line, ":"):
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(line[6:])
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(line[5:], " "))
		}
	}
}

// resync reloads calls in progress and radio positions, after a connect or a resync.
func (b *Bridge) resync(ctx context.Context) {
	var calls struct {
		Items []CallInfo `json:"items"`
	}
	if err := b.do(ctx, http.MethodGet, "/api/calls", nil, &calls); err != nil {
		b.setErr(fmt.Errorf("reading calls in progress: %w", err))
	} else {
		live := map[string]bool{}
		for _, c := range calls.Items {
			live[c.ID] = true
			b.call(c)
			if c.Emergency != nil {
				b.pullEvidence(ctx, c.ID) // photos and clips sent while the link was down
			}
		}
		// 911 calls bitdispatcher thinks are live but BitVoice no longer has ended while the link was down.
		var stale []string
		b.eng.Read(func(s *cad.State) {
			for id, c := range s.Calls {
				if c.State != "ended" && !live[id] && containsStr(c.Sources, "bitvoice") {
					stale = append(stale, id)
				}
			}
		})
		for _, id := range stale {
			b.eng.EndCall(cad.BitVoiceActor, id)
		}
	}
	var fixes struct {
		Items []RadioFix `json:"items"`
	}
	if err := b.do(ctx, http.MethodGet, "/api/radio/locations", nil, &fixes); err == nil {
		for _, f := range fixes.Items {
			b.location(f)
		}
	}
	b.pullStates(ctx)
}

func containsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// envelope is one event on BitVoice's stream.
type envelope struct {
	Seq  uint64          `json:"seq"`
	At   time.Time       `json:"at"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func (b *Bridge) handle(ctx context.Context, event string, raw []byte) {
	b.mu.Lock()
	b.health.LastEvent = time.Now()
	b.health.Events++
	b.mu.Unlock()
	switch event {
	case "hello":
		return
	case "resync":
		go b.resync(ctx)
		return
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return
	}
	if env.Type == "" {
		env.Type = event
	}
	switch env.Type {
	case "call.started", "call.updated":
		var c CallInfo
		if json.Unmarshal(env.Data, &c) == nil {
			b.call(c)
		}
	case "call.ended":
		var c CallInfo
		if json.Unmarshal(env.Data, &c) == nil && c.ID != "" {
			if c.Cause != "" || c.EndedBy != "" {
				// Why it ended, kept on the call (BitVoice ends a call whose caller's audio stopped).
				b.upsertKnown(c.ID, cad.CallData{Source: "bitvoice", Handling: &cad.Handling{EndCause: c.Cause, EndedBy: c.EndedBy}})
			}
			b.eng.EndCall(cad.BitVoiceActor, c.ID)
		}
	case "emergency.answered":
		var a struct {
			Call  string `json:"call"`
			Agent string `json:"agent"`
			From  string `json:"from"`
		}
		if json.Unmarshal(env.Data, &a) == nil && a.Call != "" {
			go b.pullEIDO(ctx, a.Call)
		}
	case "call.media", "call.video":
		var m struct {
			Call string `json:"call"`
		}
		if json.Unmarshal(env.Data, &m) == nil && m.Call != "" {
			known := false
			b.eng.Read(func(s *cad.State) { _, known = s.Calls[m.Call] })
			if known {
				go b.pullEIDO(ctx, m.Call)
				if env.Type == "call.media" {
					go b.pullEvidence(ctx, m.Call)
				}
			}
		}
	case "transcript.done":
		// BitVoice's transcript and summary of a recorded call, after it ends.
		var t struct {
			Recording string `json:"recording"`
			CallID    string `json:"callId"`
			Status    string `json:"status"`
			Summary   string `json:"summary"`
			Lines     []struct {
				Second float64 `json:"second"`
				Side   string  `json:"side"`
				Text   string  `json:"text"`
			} `json:"lines"`
		}
		if json.Unmarshal(env.Data, &t) == nil && t.CallID != "" {
			known, have := false, 0
			b.eng.Read(func(s *cad.State) {
				if c, ok := s.Calls[t.CallID]; ok {
					known, have = true, len(c.Transcript)
				}
			})
			if !known {
				return
			}
			d := cad.CallData{Source: "bitvoice", Handling: &cad.Handling{Recording: t.Recording, Summary: strings.TrimSpace(t.Summary), SummaryBy: "BitVoice (AI)"}}
			if have == 0 { // a live transcript wins over the one made afterwards
				for _, l := range t.Lines {
					sp := "taker"
					if l.Side == "caller" {
						sp = "caller"
					}
					if s := strings.TrimSpace(l.Text); s != "" {
						d.Transcript = append(d.Transcript, cad.TranscriptLine{Speaker: sp, Text: s})
					}
				}
			}
			b.eng.UpsertCall(cad.BitVoiceActor, t.CallID, d)
		}
	case "call.briefing":
		var br struct {
			CallID  string `json:"callId"`
			Summary string `json:"summary"`
			By      string `json:"by"`
		}
		if json.Unmarshal(env.Data, &br) == nil && br.CallID != "" && br.Summary != "" {
			b.upsertKnown(br.CallID, cad.CallData{Source: "bitvoice", Handling: &cad.Handling{Summary: br.Summary, SummaryBy: "BitVoice briefing (AI)"}})
		}
	case "call.recorded":
		var r struct {
			ID        string `json:"id"`
			Recording string `json:"recording"`
		}
		if json.Unmarshal(env.Data, &r) == nil && r.ID != "" && r.Recording != "" {
			b.upsertKnown(r.ID, cad.CallData{Source: "bitvoice", Handling: &cad.Handling{Recording: r.Recording}})
		}
	case "esrp.route":
		var r cad.Route
		var raw struct {
			Call string `json:"call"`
		}
		if json.Unmarshal(env.Data, &r) == nil && json.Unmarshal(env.Data, &raw) == nil && raw.Call != "" && r.Why != "" {
			if r.At.IsZero() {
				r.At = time.Now()
			}
			b.upsertKnown(raw.Call, cad.CallData{Source: "bitvoice", Handling: &cad.Handling{Routes: []cad.Route{r}}})
		}
	case "core.states":
		go b.pullStates(ctx)
	case "emergency.cad_failed":
		var f struct {
			Call  string `json:"call"`
			What  string `json:"what"`
			Error string `json:"error"`
		}
		if json.Unmarshal(env.Data, &f) == nil {
			b.failure(Failure{At: env.At, What: f.What, Call: f.Call, Error: f.Error})
			if f.Call != "" {
				go b.pullEIDO(ctx, f.Call) // fill what the push could not deliver
			}
		}
	case "radio.cad_failed":
		var f struct {
			Type  string `json:"type"`
			Error string `json:"error"`
		}
		if json.Unmarshal(env.Data, &f) == nil {
			b.failure(Failure{At: env.At, What: f.Type, Error: f.Error})
		}
	case "emergency.mode":
		var m struct {
			On bool   `json:"on"`
			By string `json:"by"`
		}
		if json.Unmarshal(env.Data, &m) == nil {
			b.mu.Lock()
			b.health.Emergency, b.health.EmergencyBy = m.On, m.By
			b.mu.Unlock()
		}
	case "radio.unit", "radio.emergency", "radio.emergency.ack", "radio.transmission", "radio.location":
		b.Radio(env.Type, env.Data, false)
	case "call.transcript":
		// Live lines of what is said on a call, when the call-handling system transcribes as it goes.
		// BitVoice: {call, lines[{speaker ("Caller" or the call-taker's name), side (caller, taker), text, second}]}.
		var t struct {
			Call  string `json:"call"`
			Lines []struct {
				Speaker string `json:"speaker"`
				Side    string `json:"side"`
				Text    string `json:"text"`
			} `json:"lines"`
		}
		if json.Unmarshal(env.Data, &t) == nil && t.Call != "" {
			var lines []cad.TranscriptLine
			for _, l := range t.Lines {
				if strings.TrimSpace(l.Text) == "" {
					continue
				}
				sp := l.Side
				if sp == "" {
					sp = strings.ToLower(l.Speaker)
				}
				if sp != "caller" && sp != "taker" {
					sp = "other"
				}
				at := env.At
				if at.IsZero() {
					at = time.Now().UTC()
				}
				lines = append(lines, cad.TranscriptLine{At: at, Speaker: sp, Text: strings.TrimSpace(l.Text)})
			}
			if len(lines) > 0 {
				b.upsertKnown(t.Call, cad.CallData{Source: "bitvoice", Transcript: lines})
			}
		}
	case "call.summarized":
		// BitVoice's transcript of a recorded call, after it ends: kept on the call when CAD knows it.
		var t struct {
			CallID     string `json:"callId"`
			Summary    string `json:"summary"`
			Transcript string `json:"transcript"`
		}
		if json.Unmarshal(env.Data, &t) == nil && t.CallID != "" && strings.TrimSpace(t.Transcript) != "" {
			known := false
			b.eng.Read(func(s *cad.State) { c, ok := s.Calls[t.CallID]; known = ok && len(c.Transcript) == 0 })
			if known {
				var lines []cad.TranscriptLine
				for _, l := range strings.Split(t.Transcript, "\n") {
					if l = strings.TrimSpace(l); l == "" {
						continue
					}
					sp := "other"
					if k, v, ok := strings.Cut(l, ":"); ok && len(k) < 24 {
						switch strings.ToLower(strings.TrimSpace(k)) {
						case "caller", "customer", "remote":
							sp, l = "caller", strings.TrimSpace(v)
						case "agent", "taker", "call-taker", "dispatcher", "local":
							sp, l = "taker", strings.TrimSpace(v)
						}
					}
					lines = append(lines, cad.TranscriptLine{Speaker: sp, Text: l})
				}
				b.eng.UpsertCall(cad.BitVoiceActor, t.CallID, cad.CallData{Source: "bitvoice", Transcript: lines})
			}
		}
	}
}

func (b *Bridge) failure(f Failure) {
	if f.At.IsZero() {
		f.At = time.Now()
	}
	b.mu.Lock()
	b.health.Failures = append([]Failure{f}, b.health.Failures...)
	if len(b.health.Failures) > 20 {
		b.health.Failures = b.health.Failures[:20]
	}
	b.mu.Unlock()
	b.log.Warn("bitvoice: could not deliver to bitdispatcher", "what", f.What, "call", f.Call, "err", f.Error)
}

// pullEIDO fetches a call's EIDO, so bitdispatcher has it even when BitVoice's push did not arrive.
func (b *Bridge) pullEIDO(ctx context.Context, call string) {
	var raw []byte
	if err := b.do(ctx, http.MethodGet, "/api/calls/"+url.PathEscape(call)+"/eido", nil, &raw); err != nil {
		b.setErr(fmt.Errorf("EIDO for %s: %w", call, err))
		return
	}
	b.eido(raw, "eido")
}

func (b *Bridge) eido(raw []byte, source string) error {
	c, err := ng911.ParseEIDO(raw)
	if err != nil {
		return err
	}
	b.mu.Lock()
	b.health.LastEIDO = time.Now()
	b.mu.Unlock()
	d := cad.CallData{Source: source, Callback: c.Callback, Agent: c.Agent, Position: c.Position, Start: c.Start, Answer: c.Answer,
		Location: c.Location, Media: c.Media, Notes: c.Notes, Emergency: true}
	if c.State == "callAnswered" {
		d.State = "answered"
	}
	if a := c.Additional; a != nil {
		d.Class = a.Class()
		if a.Subscriber != nil {
			d.CallerName = a.Subscriber.Name
		}
		if len(a.Providers) > 0 {
			d.Provider = a.Providers[0].Name
		}
	}
	for _, m := range c.Media {
		if m.Purpose == "caller-video-live" {
			d.Video = &cad.Video{Live: true, ViewURL: m.URL}
		}
	}
	return b.eng.UpsertCall(cad.BitVoiceActor, c.CallID, d)
}

// ---- calls ----

// Location is BitVoice's i3.Location as JSON.
type Location struct {
	Civic      *ng911.Civic `json:"civic,omitempty"`
	Lat        *float64     `json:"lat,omitempty"`
	Lon        *float64     `json:"lon,omitempty"`
	Radius     float64      `json:"radius,omitempty"`
	Confidence int          `json:"confidence,omitempty"`
	Method     string       `json:"method,omitempty"`
	Time       time.Time    `json:"time,omitempty"`
	Altitude   *float64     `json:"altitude,omitempty"` // metres, from a 3-D (z-axis) fix
	VRadius    float64      `json:"vRadius,omitempty"`
}

func (l *Location) loc(source string) *ng911.Location {
	if l == nil || (l.Civic == nil && (l.Lat == nil || l.Lon == nil)) {
		return nil
	}
	out := &ng911.Location{Civic: l.Civic, Lat: l.Lat, Lon: l.Lon, Radius: l.Radius, Confidence: l.Confidence, Method: l.Method, Source: source, At: l.Time,
		Altitude: l.Altitude, VRadius: l.VRadius}
	if l.Civic != nil {
		out.Address = l.Civic.String()
	}
	return out
}

// CallInfo is BitVoice's call snapshot (pbx.CallInfo), the fields a CAD uses.
type CallInfo struct {
	ID        string    `json:"id"`
	State     string    `json:"state"`
	Direction string    `json:"direction"`
	From      string    `json:"from"`
	FromName  string    `json:"fromName"`
	To        string    `json:"to"`
	Start     time.Time `json:"start"`
	Answer    time.Time `json:"answer"`
	Cause     string    `json:"cause,omitempty"`   // on call.ended: why it ended
	EndedBy   string    `json:"endedBy,omitempty"` // on call.ended: caller, callee, system, or a person
	Screen    *struct {
		Attest   string `json:"attest"`
		Verified string `json:"verified"`
		Why      string `json:"why"`
	} `json:"screen,omitempty"`
	Emergency *struct {
		Location    *Location        `json:"location,omitempty"`
		LocationURI string           `json:"locationUri,omitempty"`
		Additional  ng911.Additional `json:"additional"`
		Service     string           `json:"service,omitempty"`
		Problems    []string         `json:"problems,omitempty"`
		ESInet      bool             `json:"esinet"`
		TakenBy     string           `json:"takenBy,omitempty"`
		TakenName   string           `json:"takenName,omitempty"`
		Text        []cad.TextLine   `json:"text,omitempty"`
		TextOnly    bool             `json:"textOnly,omitempty"`
		ALIClass    string           `json:"aliClass,omitempty"`
		TextVia     []string         `json:"textVia,omitempty"`
		CAMA        string           `json:"cama,omitempty"`
		ALILookup   string           `json:"aliLookup,omitempty"`
		LocCheck    *struct {
			Show     string    `json:"show"`
			Text     string    `json:"text"`
			Reasons  []string  `json:"reasons"`
			Previous *Location `json:"previous,omitempty"`
		} `json:"locationCheck,omitempty"`
		Video *struct {
			ViewToken string `json:"viewToken,omitempty"`
			Live      bool   `json:"live"`
			Source    string `json:"source,omitempty"`
		} `json:"video,omitempty"`
	} `json:"emergency,omitempty"`
}

func (b *Bridge) call(c CallInfo) {
	em := c.Emergency
	if em == nil || c.ID == "" {
		return // only 911 calls belong in CAD
	}
	state := c.State
	if state == "held" {
		state = "answered"
	}
	d := cad.CallData{Source: "bitvoice", State: state, Callback: c.From, CallerName: c.FromName, Start: c.Start, Answer: c.Answer,
		Location: em.Location.loc("bitvoice"), Text: em.Text, TextOnly: em.TextOnly, CanRebid: em.LocationURI != "", Problems: em.Problems,
		Agent: firstNonEmpty(em.TakenName, em.TakenBy), Emergency: true}
	if c.To != "" && c.To != "911" {
		d.Line = c.To // a call on a non-emergency line BitVoice also hands to CAD
	}
	if d.Problems == nil {
		d.Problems = []string{}
	}
	d.Class = firstNonEmpty(em.ALIClass, em.Additional.Class())
	if s := em.Additional.Subscriber; s != nil && s.Name != "" {
		d.CallerName = s.Name
	}
	if len(em.Additional.Providers) > 0 {
		d.Provider = em.Additional.Providers[0].Name
	}
	h := &cad.Handling{ESInet: em.ESInet, Service: em.Service, CAMA: em.CAMA, ALILookup: em.ALILookup, TextVia: em.TextVia, Taker: firstNonEmpty(em.TakenName, em.TakenBy)}
	if s := c.Screen; s != nil && (s.Verified != "" || s.Attest != "") {
		h.CallerID = &cad.CallerIDCheck{Verified: s.Verified, Attest: s.Attest, Why: s.Why}
	}
	if lc := em.LocCheck; lc != nil && lc.Text != "" {
		h.LocCheck = &cad.LocCheck{Show: lc.Show, Text: lc.Text, Reasons: lc.Reasons, Previous: lc.Previous.loc("bitvoice")}
	}
	d.Handling = h
	if v := em.Video; v != nil && v.ViewToken != "" {
		b.mu.Lock()
		base := b.cfg.URL
		b.mu.Unlock()
		d.Video = &cad.Video{Live: v.Live, Source: v.Source, ViewURL: base + "/whep/" + v.ViewToken}
	}
	b.eng.UpsertCall(cad.BitVoiceActor, c.ID, d)
	b.checkAddress(c.ID, d.Location)
}

// checkAddress validates a caller's civic address and position with BitVoice's LVF, once per change.
func (b *Bridge) checkAddress(call string, l *ng911.Location) {
	if l == nil || l.Civic == nil {
		return
	}
	key := call + "|" + l.Civic.String()
	if l.HasPoint() {
		key += fmt.Sprintf("|%.5f,%.5f", *l.Lat, *l.Lon)
	}
	b.mu.Lock()
	if b.lvf == nil || len(b.lvf) > 5000 {
		b.lvf = map[string]bool{}
	}
	seen := b.lvf[key]
	b.lvf[key] = true
	b.mu.Unlock()
	if seen {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		r, err := b.Validate(ctx, l)
		if err != nil || r == nil {
			return
		}
		b.upsertKnown(call, cad.CallData{Source: "bitvoice", Handling: &cad.Handling{LVF: r}})
	}()
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// ---- radio ----

// RadioUnitEvent is BitVoice's radio.unit and radio.emergency payload.
type RadioUnitEvent struct {
	Channel     string    `json:"channel"`
	ChannelName string    `json:"channelName"`
	Unit        string    `json:"unit"`
	UnitID      int       `json:"unitId"`
	Emergency   bool      `json:"emergency"`
	Alias       string    `json:"alias,omitempty"`
	At          time.Time `json:"at"`
}

// RadioAck is BitVoice's radio.emergency.ack payload.
type RadioAck struct {
	Channel     string    `json:"channel"`
	ChannelName string    `json:"channelName"`
	Unit        string    `json:"unit"`
	Alias       string    `json:"alias,omitempty"`
	RaisedAt    time.Time `json:"raisedAt"`
	By          string    `json:"by"`
	At          time.Time `json:"at"`
}

// RadioFix is BitVoice's radio.location payload.
type RadioFix struct {
	Unit     int       `json:"unit"`
	Alias    string    `json:"alias,omitempty"`
	Lat      float64   `json:"lat"`
	Lon      float64   `json:"lon"`
	Accuracy float64   `json:"accuracy,omitempty"`
	Speed    float64   `json:"speed,omitempty"`
	Heading  float64   `json:"heading,omitempty"`
	At       time.Time `json:"at"`
	Source   string    `json:"source,omitempty"`
}

// Transmission is BitVoice's radio.transmission payload.
type Transmission struct {
	Channel   string    `json:"channel"`
	Direction string    `json:"direction"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	Unit      string    `json:"unit,omitempty"`
	UnitName  string    `json:"unitName,omitempty"`
	Emergency bool      `json:"emergency,omitempty"`
	// Text is what was said, when BitVoice (or the radio system) transcribes transmissions.
	Text        string `json:"text,omitempty"`
	ChannelName string `json:"channelName,omitempty"`
}

// Radio handles one radio event, from the webhook or the stream.
func (b *Bridge) Radio(typ string, data json.RawMessage, webhook bool) error {
	b.mu.Lock()
	b.health.LastRadio = time.Now()
	if webhook {
		b.health.LastRadioWH = time.Now()
	}
	b.mu.Unlock()
	switch typ {
	case "radio.unit", "radio.emergency":
		var ev RadioUnitEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return err
		}
		unit := b.eng.UnitByRadio(ev.UnitID, ev.Alias)
		at := ev.At
		if at.IsZero() {
			at = time.Now()
		}
		if unit != "" {
			b.eng.Heard(unit, at)
		}
		if typ == "radio.emergency" || ev.Emergency {
			_, err := b.eng.RaiseEmergency(cad.RadioActor, cad.Emergency{Unit: unit, RadioUnit: ev.Unit, RadioID: ev.UnitID, Alias: ev.Alias,
				Channel: ev.Channel, ChannelName: ev.ChannelName, Raised: at})
			return err
		}
	case "radio.emergency.ack":
		var ev RadioAck
		if err := json.Unmarshal(data, &ev); err != nil {
			return err
		}
		if key := b.eng.OpenEmergencyFor(ev.Unit, ev.Channel); key != "" {
			return b.eng.AckEmergency(cad.Actor{User: firstNonEmpty(ev.By, "bitvoice")}, key, "bitvoice")
		}
	case "radio.transmission":
		var ev Transmission
		if err := json.Unmarshal(data, &ev); err != nil {
			return err
		}
		if ev.Direction == "rx" {
			unit := b.eng.UnitByRadio(0, ev.UnitName)
			if unit == "" {
				if id := radioIDOf(ev.Unit); id != 0 {
					unit = b.eng.UnitByRadio(id, "")
				}
			}
			if unit != "" {
				b.eng.Heard(unit, firstTime(ev.End, ev.Start))
			}
			if strings.TrimSpace(ev.Text) != "" {
				return b.eng.RadioText(cad.RadioActor, cad.RadioLine{At: firstTime(ev.Start, ev.End), Unit: unit, RadioUnit: firstNonEmpty(ev.UnitName, ev.Unit),
					Channel: ev.Channel, ChannelName: ev.ChannelName, Text: ev.Text})
			}
		}
	case "radio.location":
		var f RadioFix
		if err := json.Unmarshal(data, &f); err != nil {
			return err
		}
		b.location(f)
	}
	return nil
}

func firstTime(t ...time.Time) time.Time {
	for _, x := range t {
		if !x.IsZero() {
			return x
		}
	}
	return time.Now()
}

// radioIDOf reads the unit ID from BitVoice's WACN.system.unit hex form.
func radioIDOf(s string) int {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return 0
	}
	var n int
	if _, err := fmt.Sscanf(parts[2], "%x", &n); err != nil {
		return 0
	}
	return n
}

func (b *Bridge) location(f RadioFix) {
	unit := b.eng.UnitByRadio(f.Unit, f.Alias)
	if unit == "" || (f.Lat == 0 && f.Lon == 0) {
		return
	}
	b.eng.UnitPosition(cad.RadioActor, unit, cad.Position{Lat: f.Lat, Lon: f.Lon, Accuracy: f.Accuracy, Speed: f.Speed, Heading: f.Heading,
		Source: firstNonEmpty(f.Source, "radio"), At: f.At})
}

// ---- webhooks BitVoice posts to ----

func bearerOK(r *http.Request, secret string) bool {
	if secret == "" {
		return false
	}
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return ok && subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1
}

// EIDOHandler takes EIDOs BitVoice posts (psap.eidoUrl).
func (b *Bridge) EIDOHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !bearerOK(r, b.Config().EIDOToken) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
		if err != nil {
			http.Error(w, "too large", http.StatusRequestEntityTooLarge)
			return
		}
		if err := b.eido(raw, "eido"); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		b.mu.Lock()
		b.health.LastPush = time.Now()
		b.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
}

// RadioHandler takes radio events BitVoice posts (radio.cadUrl).
func (b *Bridge) RadioHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !bearerOK(r, b.Config().RadioSecret) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var env struct {
			Type   string          `json:"type"`
			Agency string          `json:"agency"`
			Data   json.RawMessage `json:"data"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&env); err != nil || env.Type == "" {
			http.Error(w, "send {type, agency, data}", http.StatusBadRequest)
			return
		}
		if err := b.Radio(env.Type, env.Data, true); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// ---- dispatcher actions ----

// Rebid asks BitVoice to fetch the caller's location again and records what came back.
func (b *Bridge) Rebid(ctx context.Context, call string) (*ng911.Location, error) {
	var out struct {
		Location *Location `json:"location"`
	}
	if err := b.do(ctx, http.MethodPost, "/api/calls/"+url.PathEscape(call)+"/rebid", map[string]any{}, &out); err != nil {
		return nil, err
	}
	loc := out.Location.loc("rebid")
	if loc == nil {
		return nil, errors.New("the rebid returned no location")
	}
	if loc.At.IsZero() {
		loc.At = time.Now()
	}
	return loc, b.eng.UpsertCall(cad.BitVoiceActor, call, cad.CallData{Source: "bitvoice", Location: loc})
}

// Text sends text to the caller (TTY, RTT, MSRP or SIP; empty lets BitVoice choose).
func (b *Bridge) Text(ctx context.Context, call, via, text string) error {
	return b.do(ctx, http.MethodPost, "/api/calls/"+url.PathEscape(call)+"/text", map[string]string{"via": via, "text": text}, nil)
}

// VideoLink is BitVoice's answer to a video-link request.
type VideoLink struct {
	URL     string `json:"url"`
	ViewURL string `json:"viewUrl"`
	Sent    bool   `json:"sent"`
	Problem string `json:"problem,omitempty"`
}

// RequestVideo asks BitVoice to text the caller a link that streams their camera to 911.
func (b *Bridge) RequestVideo(ctx context.Context, call string) (VideoLink, error) {
	var v VideoLink
	err := b.do(ctx, http.MethodPost, "/api/calls/"+url.PathEscape(call)+"/video/link", map[string]any{}, &v)
	if err == nil && v.ViewURL != "" {
		b.eng.UpsertCall(cad.BitVoiceActor, call, cad.CallData{Source: "bitvoice", Video: &cad.Video{ViewURL: v.ViewURL, Source: "link"}})
	}
	return v, err
}

// RefreshEIDO pulls a call's EIDO now.
func (b *Bridge) RefreshEIDO(ctx context.Context, call string) error {
	var raw []byte
	if err := b.do(ctx, http.MethodGet, "/api/calls/"+url.PathEscape(call)+"/eido", nil, &raw); err != nil {
		return err
	}
	return b.eido(raw, "eido")
}

// AckRadio acknowledges a radio emergency on BitVoice's console as well.
func (b *Bridge) AckRadio(ctx context.Context, channel string) error {
	return b.do(ctx, http.MethodPost, "/api/radio/channels/"+url.PathEscape(channel)+"/ack", map[string]any{}, nil)
}

// Fetch reads a URL on the configured BitVoice (caller photos, clips) with the pinned client, for the
// dispatcher's browser. Any other host is refused, so this is never an open proxy.
func (b *Bridge) Fetch(ctx context.Context, raw string) (*http.Response, error) {
	b.mu.Lock()
	base, client := b.cfg.URL, b.client
	b.mu.Unlock()
	bu, err1 := url.Parse(base)
	u, err2 := url.Parse(raw)
	if base == "" || err1 != nil || err2 != nil || u.Scheme != bu.Scheme || u.Host != bu.Host {
		return nil, errors.New("that address is not on the configured BitVoice")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

// WHEP forwards a browser's WebRTC offer to BitVoice's WHEP endpoint for a call's live video and returns
// the answer. The media then flows from BitVoice to the browser directly.
func (b *Bridge) WHEP(ctx context.Context, viewURL string, offer []byte) ([]byte, string, error) {
	b.mu.Lock()
	base, client := b.cfg.URL, b.client
	b.mu.Unlock()
	bu, _ := url.Parse(base)
	u, err := url.Parse(viewURL)
	if base == "" || err != nil || u.Host != bu.Host || !strings.HasPrefix(u.Path, "/whep/") {
		return nil, "", errors.New("that is not a BitVoice video view")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(offer))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/sdp")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	answer, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if resp.StatusCode/100 != 2 {
		return nil, "", fmt.Errorf("BitVoice said %s", resp.Status)
	}
	return answer, resp.Header.Get("Location"), nil
}

// upsertKnown updates a call bitdispatcher already has (a 911 call), and ignores the rest.
func (b *Bridge) upsertKnown(id string, d cad.CallData) {
	known := false
	b.eng.Read(func(s *cad.State) { _, known = s.Calls[id] })
	if known {
		b.eng.UpsertCall(cad.BitVoiceActor, id, d)
	}
}

// pullEvidence reads a call's photos and clips: their hash, the camera's time and GPS, and whether
// they are held as evidence. The files stay in BitVoice.
func (b *Bridge) pullEvidence(ctx context.Context, call string) {
	var ix struct {
		Held  bool               `json:"held"`
		Items []cad.EvidenceItem `json:"items"`
	}
	if err := b.do(ctx, "GET", "/api/calls/"+url.PathEscape(call)+"/media", nil, &ix); err != nil {
		b.log.Warn("bitvoice: caller media", "call", call, "err", err)
		return
	}
	if ix.Items == nil {
		ix.Items = []cad.EvidenceItem{}
	}
	b.upsertKnown(call, cad.CallData{Source: "bitvoice", Handling: &cad.Handling{Evidence: &cad.Evidence{Held: ix.Held, Items: ix.Items}}})
}

// pullStates reads the PSAPs' NENA states from BitVoice's ESRP.
func (b *Bridge) pullStates(ctx context.Context) {
	var v struct {
		Items []PSAPState `json:"items"`
	}
	if err := b.do(ctx, "GET", "/api/core/states", nil, &v); err != nil {
		return
	}
	b.mu.Lock()
	b.health.PSAPs = v.Items
	b.mu.Unlock()
}

// Evidence fetches one of a call's photos or clips from BitVoice's own store (BitVoice logs the look).
func (b *Bridge) Evidence(ctx context.Context, call string, n int) (*http.Response, error) {
	b.mu.Lock()
	cfg, client := b.cfg, b.client
	b.mu.Unlock()
	if cfg.URL == "" || cfg.Token == "" {
		return nil, errors.New("BitVoice is not configured (Admin › Integrations)")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/calls/%s/media/%d", cfg.URL, url.PathEscape(call), n), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	setOnBehalf(req)
	return client.Do(req)
}

// HoldEvidence marks a call's photos and clips as held (kept past the retention), or not, in BitVoice.
func (b *Bridge) HoldEvidence(ctx context.Context, call string, hold bool) error {
	if err := b.do(ctx, http.MethodPost, "/api/calls/"+url.PathEscape(call)+"/media/hold", map[string]bool{"hold": hold}, nil); err != nil {
		return err
	}
	b.pullEvidence(ctx, call)
	return nil
}

// RevealVideo tells BitVoice a person in CAD unblurred the caller's live video, for its exposure record.
func (b *Bridge) RevealVideo(ctx context.Context, call string) error {
	return b.do(ctx, http.MethodPost, "/api/calls/"+url.PathEscape(call)+"/video/reveal", map[string]string{"item": "live"}, nil)
}

// TakeText takes a text-only 911 call that is waiting on BitVoice's 911 screen, so replies can go to it.
func (b *Bridge) TakeText(ctx context.Context, call string) error {
	return b.do(ctx, http.MethodPost, "/api/calls/"+url.PathEscape(call)+"/take", map[string]any{}, nil)
}

// Validate checks a civic address (and a position against it) with BitVoice's LVF. It returns nil when
// BitVoice has no address data loaded.
func (b *Bridge) Validate(ctx context.Context, l *ng911.Location) (*cad.LVFCheck, error) {
	if l == nil || l.Civic == nil {
		return nil, nil
	}
	c := l.Civic
	in := map[string]string{"country": firstNonEmpty(c.Country, "US"), "A1": c.State, "A2": c.County, "A3": c.City, "PRD": c.PreDir, "RD": c.Street,
		"STS": c.StreetType, "POD": c.PostDir, "HNO": c.Number, "HNS": c.NumberSuf, "PC": c.PostalCode}
	for k, v := range in {
		if v == "" {
			delete(in, k)
		}
	}
	if l.HasPoint() {
		in["lat"], in["lon"] = fmt.Sprint(*l.Lat), fmt.Sprint(*l.Lon)
		if l.Radius > 0 {
			in["radius"] = fmt.Sprint(l.Radius)
		}
	}
	var out struct {
		Invalid  []string `json:"invalid"`
		Matched  string   `json:"matched"`
		Position *struct {
			Checked bool   `json:"checked"`
			Within  bool   `json:"within"`
			Why     string `json:"why"`
		} `json:"position"`
		Suggestions []struct {
			Civic map[string]string `json:"civic"`
			Why   string            `json:"why"`
		} `json:"suggestions"`
	}
	if err := b.do(ctx, http.MethodPost, "/api/core/validate", in, &out); err != nil {
		if strings.Contains(err.Error(), "409") {
			return nil, nil // no address data in BitVoice
		}
		return nil, err
	}
	r := &cad.LVFCheck{Address: c.String(), Valid: out.Matched != "" && len(out.Invalid) == 0, Matched: out.Matched, Invalid: out.Invalid, At: time.Now()}
	if p := out.Position; p != nil {
		r.Position = p.Why
		if p.Checked {
			w := p.Within
			r.Within = &w
		}
	}
	for _, s := range out.Suggestions {
		v := s.Civic
		addr := strings.Join(strings.Fields(strings.Join([]string{v["HNO"] + v["HNS"], v["PRD"], v["RD"], v["STS"], v["POD"]}, " ")), " ")
		if v["A3"] != "" {
			addr += ", " + v["A3"]
		}
		if v["A1"] != "" {
			addr += " " + v["A1"]
		}
		r.Suggestions = append(r.Suggestions, cad.LVFSuggestion{Address: addr, Why: s.Why})
	}
	return r, nil
}

// Records reads one of BitVoice's 911 records for CAD's reports: the answer-time report, the exposure
// record (who looked at callers' photos and video), or the i3 log events received. The answer is passed on
// as BitVoice wrote it.
// ErrNoRecord: Records was asked for a kind it does not offer.
var ErrNoRecord = errors.New("no such record: answer, exposure, i3log or states")

func (b *Bridge) Records(ctx context.Context, kind string, q url.Values) (json.RawMessage, error) {
	path := map[string]string{"answer": "/api/psap/report", "exposure": "/api/psap/exposure", "i3log": "/api/core/i3log", "states": "/api/core/states"}[kind]
	if path == "" {
		return nil, ErrNoRecord
	}
	keep := url.Values{}
	for _, k := range []string{"from", "to", "day"} {
		if v := q.Get(k); v != "" {
			keep.Set(k, v)
		}
	}
	if len(keep) > 0 {
		path += "?" + keep.Encode()
	}
	var out json.RawMessage
	err := b.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
