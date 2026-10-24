package api

// Public alerts (mass notification): the settings (Admin › Integrations › Public alerts), the subscriber
// lists, writing, approving, updating and cancelling alerts, the public CAP feed, and delivery.

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/alerting"
	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/paging"
)

// Alerting is how public alerts are written and where they go.
type Alerting struct {
	Sender       string              `json:"sender"`             // the CAP sender: an address the agency owns (alerts@county.gov)
	SenderName   string              `json:"senderName"`         // "Example County Emergency Communications"
	Jurisdiction string              `json:"jurisdiction"`       // "Example County": the area of an alert to everyone
	Geocodes     []string            `json:"geocodes"`           // the jurisdiction's SAME county codes (051770)
	Feed         bool                `json:"feed"`               // publish the CAP feed at /api/public/alerts.atom
	TwoPerson    bool                `json:"twoPerson"`          // a second supervisor approves each alert
	TimeZone     string              `json:"timeZone,omitempty"` // America/New_York; empty: the server's
	Web          string              `json:"web,omitempty"`
	Contact      string              `json:"contact,omitempty"`
	Receivers    []alerting.Receiver `json:"receivers"`
}

// Loc is the agency's time zone, which CAP times are written in.
func (c Alerting) Loc() *time.Location {
	if c.TimeZone != "" {
		if l, err := time.LoadLocation(c.TimeZone); err == nil {
			return l
		}
	}
	return time.Local
}

var reSAMECode = regexp.MustCompile(`^\d{6}$`)

func (s *Server) alertingConfig() Alerting {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.integ.Alerting
}

// AlertingView is the settings with receivers' tokens shown as dots, and what the screens need.
type AlertingView struct {
	Alerting
	FeedURL  string               `json:"feedUrl"`
	Events   []alerting.EventCode `json:"events"`
	Channels []string             `json:"channels"`
	Ready    []string             `json:"ready"` // what is missing before an alert can go
}

func (s *Server) alertingView(r *http.Request) AlertingView {
	c := s.alertingConfig()
	v := AlertingView{Alerting: c, FeedURL: s.publicBase(r) + "/api/public/alerts.atom", Events: alerting.EventCodes, Channels: alerting.Channels, Ready: []string{}}
	v.Receivers = []alerting.Receiver{}
	for _, x := range c.Receivers {
		v.Receivers = append(v.Receivers, x.Redacted())
	}
	if v.Geocodes == nil {
		v.Geocodes = []string{}
	}
	if c.Sender == "" {
		v.Ready = append(v.Ready, "set the sender (an address the agency owns) in Admin › Integrations › Public alerts")
	}
	if _, off := time.Now().In(c.Loc()).Zone(); off == 0 {
		v.Ready = append(v.Ready, "set the agency's time zone: CAP times are written with a US offset")
	}
	return v
}

// publicBase is the address the public reaches this server at: the one set for paging links, else the
// one this request came to.
func (s *Server) publicBase(r *http.Request) string {
	if u := s.pagingConfig().PublicURL; u != "" {
		return u
	}
	if r == nil {
		return ""
	}
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

func (s *Server) saveAlerting(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b Alerting
	if !readJSON(w, r, &b) {
		return
	}
	b.Sender, b.SenderName, b.Jurisdiction, b.TimeZone = strings.TrimSpace(b.Sender), strings.TrimSpace(b.SenderName), strings.TrimSpace(b.Jurisdiction), strings.TrimSpace(b.TimeZone)
	if b.TimeZone != "" {
		if _, err := time.LoadLocation(b.TimeZone); err != nil {
			fail(w, 422, "the time zone is a name like America/New_York")
			return
		}
	}
	if b.Sender != "" && strings.ContainsAny(b.Sender, " ,<&") {
		fail(w, 422, "the sender has no spaces, commas, < or & (an address like alerts@county.gov)")
		return
	}
	for i, g := range b.Geocodes {
		b.Geocodes[i] = strings.TrimSpace(g)
		if !reSAMECode.MatchString(b.Geocodes[i]) {
			fail(w, 422, fmt.Sprintf("county code %q is six digits: 0, the state's FIPS code and the county's (051770)", g))
			return
		}
	}
	s.mu.Lock()
	n := s.integ
	old := map[string]alerting.Receiver{}
	for _, x := range n.Alerting.Receivers {
		old[x.ID] = x
	}
	ids := map[string]bool{}
	for i := range b.Receivers {
		x := &b.Receivers[i]
		x.ID, x.Name, x.URL = strings.TrimSpace(x.ID), strings.TrimSpace(x.Name), strings.TrimSpace(x.URL)
		if x.ID == "" {
			x.ID = fmt.Sprintf("rx%d", i+1)
			for ids[x.ID] || old[x.ID].ID != "" {
				x.ID += "x"
			}
		}
		if ids[x.ID] {
			s.mu.Unlock()
			fail(w, 422, "two receivers have the id "+x.ID)
			return
		}
		ids[x.ID] = true
		switch x.Token {
		case "", "••••":
			x.Token = old[x.ID].Token
		case "-":
			x.Token = ""
		}
		if err := x.Check(); err != nil {
			s.mu.Unlock()
			fail(w, 422, firstNonEmptyStr(x.Name, x.ID)+": "+err.Error())
			return
		}
	}
	n.Alerting = b
	err := s.writeIntegrations(n)
	if err == nil {
		s.integ = n
	}
	s.mu.Unlock()
	if err != nil {
		writeErr(w, err)
		return
	}
	var names []string
	for _, x := range b.Receivers {
		names = append(names, x.Name)
	}
	s.Eng.Record(a, cad.EvIntegration, map[string]any{"alerting": map[string]any{"sender": b.Sender, "feed": b.Feed, "twoPerson": b.TwoPerson, "receivers": names, "geocodes": b.Geocodes}})
	writeJSON(w, 200, s.alertingView(r))
}

// capMessage is an alert as its CAP message, its times in the agency's time zone (the IPAWS profile
// expects a US offset, not UTC).
func capMessage(st *cad.State, p *cad.PublicAlert, c Alerting) alerting.Message {
	spec := alerting.Spec{Sender: c.Sender, SenderName: firstNonEmptyStr(c.SenderName, st.Settings.Agency), Identifier: p.Identifier,
		Sent: firstTime(p.Sent, p.Created).In(c.Loc()), Expires: p.Expires.In(c.Loc()), Status: p.Status, MsgType: p.MsgType, Incident: p.Incident,
		Event: p.Event, EventName: p.EventName, Category: p.Category, Response: p.Response, Urgency: p.Urgency, Severity: p.Severity,
		Certainty: p.Certainty, AreaDesc: p.Area.Desc, Polygon: p.Area.Polygon, Geocodes: p.Area.Geocodes, Block: p.Block, Web: c.Web, Contact: c.Contact}
	if spec.Sender == "" {
		spec.Sender = "bitdispatcher" // checked before sending: an alert needs the agency's own sender
	}
	// An update or cancel names every message before it in the chain.
	for of := st.Alert(p.Of); of != nil; of = st.Alert(of.Of) {
		spec.References = append([]string{alerting.Reference(alerting.Message{Sender: spec.Sender, Identifier: of.Identifier, Sent: alerting.Time(firstTime(of.Sent, of.Created).In(c.Loc()))})}, spec.References...)
	}
	spec.Texts = []alerting.Text{{Language: "en-US", Headline: p.Text.Headline, Description: p.Text.Description, Instruction: p.Text.Instruction, Short: p.Text.Short, Long: p.Text.Long}}
	if p.Spanish != nil {
		spec.Texts = append(spec.Texts, alerting.Text{Language: "es-US", Headline: p.Spanish.Headline, Description: p.Spanish.Description, Instruction: p.Spanish.Instruction, Short: p.Spanish.Short, Long: p.Spanish.Long})
	}
	return alerting.Build(spec)
}

func firstTime(ts ...time.Time) time.Time {
	for _, t := range ts {
		if !t.IsZero() {
			return t
		}
	}
	return time.Time{}
}

// publicAlertBody is what a person writing an alert sends.
type publicAlertBody struct {
	cad.PublicAlert
	Whole bool `json:"whole"` // the whole jurisdiction: its county codes and name
}

// prepare fills an alert from the settings: the event's name and category, the whole jurisdiction.
func (s *Server) preparePublicAlert(b publicAlertBody) (cad.PublicAlert, error) {
	c := s.alertingConfig()
	p := b.PublicAlert
	p.MsgType = firstNonEmptyStr(p.MsgType, "Alert")
	p.Event = strings.ToUpper(strings.TrimSpace(p.Event))
	if ev, ok := alerting.Event(p.Event); ok {
		p.EventName = firstNonEmptyStr(p.EventName, ev.Name)
		p.Category = firstNonEmptyStr(p.Category, ev.Category)
	} else if p.MsgType != "Cancel" {
		return p, &cad.Problem{Field: "event", Message: "choose what kind of alert it is"}
	}
	p.Urgency, p.Severity, p.Certainty = firstNonEmptyStr(p.Urgency, "Immediate"), firstNonEmptyStr(p.Severity, "Severe"), firstNonEmptyStr(p.Certainty, "Observed")
	if b.Whole {
		if len(c.Geocodes) == 0 {
			return p, &cad.Problem{Field: "area", Message: "set the jurisdiction's county codes first (Admin › Integrations › Public alerts)"}
		}
		p.Area = cad.AlertArea{Desc: firstNonEmptyStr(c.Jurisdiction, "The whole jurisdiction"), Geocodes: c.Geocodes}
	} else if len(p.Area.Geocodes) == 0 && len(c.Geocodes) > 0 {
		p.Area.Geocodes = c.Geocodes // the counties it is in, for EAS, with the polygon for phones
	}
	known := map[string]bool{}
	for _, x := range c.Receivers {
		known[x.ID] = true
	}
	for _, id := range p.Receivers {
		if !known[id] {
			return p, &cad.Problem{Field: "receivers", Message: "no CAP receiver " + id}
		}
	}
	return p, nil
}

// checkCAP is what the CAP message itself would be found wanting for.
func (s *Server) checkCAP(p cad.PublicAlert) ([]string, []byte) {
	c := s.alertingConfig()
	now := s.Eng.Now()
	if p.Identifier == "" {
		p.Identifier = "PREVIEW-" + now.UTC().Format("20060102T150405")
	}
	p.Created = now
	var m alerting.Message
	s.Eng.Read(func(st *cad.State) { m = capMessage(st, &p, c) })
	probs := alerting.Check(m)
	if probs == nil {
		probs = []string{}
	}
	if c.Sender == "" {
		probs = append([]string{"set the agency's sender in Admin › Integrations › Public alerts"}, probs...)
	}
	b, _ := alerting.Encode(m)
	return probs, b
}

func (s *Server) publicAlertRoutes() {
	s.handle("GET /api/admin/alerting", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		writeJSON(w, 200, s.alertingView(r))
	})
	s.handle("PUT /api/admin/alerting", auth.Admin, s.saveAlerting)
	s.handle("POST /api/admin/alerting/test", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Receiver string `json:"receiver"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		c := s.alertingConfig()
		for _, x := range c.Receivers {
			if x.ID != b.Receiver {
				continue
			}
			now := time.Now()
			m := alerting.Build(alerting.Spec{Sender: firstNonEmptyStr(c.Sender, "bitdispatcher"), SenderName: c.SenderName, Identifier: "TEST-" + now.UTC().Format("20060102T150405"),
				Sent: now, Expires: now.Add(10 * time.Minute), Status: "Test", MsgType: "Alert", Event: "DMO", EventName: "Practice / demonstration warning",
				Category: "Other", Response: "None", Urgency: "Unknown", Severity: "Minor", Certainty: "Unknown",
				Texts:    []alerting.Text{{Language: "en-US", Headline: "TEST of the CAP connection from bitdispatcher", Description: "A test message. No action is needed."}},
				AreaDesc: firstNonEmptyStr(c.Jurisdiction, "Test area"), Geocodes: c.Geocodes, Polygon: [][2]float64{{0, 0}, {0.001, 0}, {0.001, 0.001}}})
			body, _ := alerting.Encode(m)
			res := alerting.Post(r.Context(), nil, x, body)
			writeJSON(w, 200, map[string]any{"ok": res.OK, "detail": res.Detail})
			return
		}
		fail(w, 404, "no CAP receiver "+b.Receiver)
	})
	s.handle("PUT /api/admin/alertlists", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var l cad.AlertList
		if !readJSON(w, r, &l) {
			return
		}
		if err := s.checkGateways(l.Gateways); err != nil {
			writeErr(w, err)
			return
		}
		id, err := s.Eng.SaveAlertList(a, l)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("DELETE /api/admin/alertlists/{id}", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		done(w, s.Eng.RemoveAlertList(a, r.PathValue("id")))
	})
	s.handle("GET /api/admin/alertlists", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out := []*cad.AlertList{}
		s.Eng.Read(func(st *cad.State) {
			for _, l := range st.AlertLists {
				c := *l
				out = append(out, &c)
			}
		})
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		writeJSON(w, 200, map[string]any{"items": out})
	})
	// A CSV of subscribers (name, phone, email, topic, lang, lat, lon) replaces or adds to a list's people.
	s.handle("POST /api/admin/alertlists/{id}/import", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var l *cad.AlertList
		s.Eng.Read(func(st *cad.State) {
			if x := st.AlertLists[r.PathValue("id")]; x != nil {
				c := *x
				c.People = slices.Clone(x.People)
				l = &c
			}
		})
		if l == nil {
			fail(w, 404, "no alert list "+r.PathValue("id"))
			return
		}
		people, err := readSubscribers(io.LimitReader(r.Body, 8<<20))
		if err != nil {
			fail(w, 422, err.Error())
			return
		}
		if r.URL.Query().Get("mode") == "replace" {
			l.People = people
		} else {
			l.People = append(l.People, people...)
		}
		if _, err := s.Eng.SaveAlertList(a, *l); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"imported": len(people), "total": len(l.People)})
	})

	s.handle("GET /api/alerts", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out := []cad.PublicAlert{}
		s.Eng.Read(func(st *cad.State) {
			for i := len(st.Alerts) - 1; i >= 0 && len(out) < 100; i-- {
				out = append(out, *st.Alerts[i])
			}
		})
		writeJSON(w, 200, map[string]any{"items": out, "settings": s.alertingView(r)})
	})
	// Check shows the CAP message an alert would make, and what is wrong with it, without writing it.
	s.handle("POST /api/alerts/check", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b publicAlertBody
		if !readJSON(w, r, &b) {
			return
		}
		p, err := s.preparePublicAlert(b)
		if err != nil {
			writeErr(w, err)
			return
		}
		var reached int
		s.Eng.Read(func(st *cad.State) {
			seen := map[string]bool{}
			for _, ss := range st.AlertRecipients(&p) {
				for _, x := range ss {
					k := x.Phone + "|" + x.Email + "|" + x.Topic
					if !seen[k] {
						seen[k] = true
						reached++
					}
				}
			}
		})
		probs, xml := s.checkCAP(p)
		writeJSON(w, 200, map[string]any{"problems": probs, "cap": string(xml), "reach": reached, "twoPerson": s.alertingConfig().TwoPerson})
	})
	s.handle("POST /api/alerts", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b publicAlertBody
		if !readJSON(w, r, &b) {
			return
		}
		p, err := s.preparePublicAlert(b)
		if err != nil {
			writeErr(w, err)
			return
		}
		if p.MsgType != "Cancel" {
			if probs, _ := s.checkCAP(p); len(probs) > 0 {
				fail(w, 422, probs[0])
				return
			}
		} else if s.alertingConfig().Sender == "" {
			fail(w, 422, "set the agency's sender in Admin › Integrations › Public alerts")
			return
		}
		id, err := s.Eng.DraftAlert(a, p, s.alertingConfig().TwoPerson)
		if err != nil {
			writeErr(w, err)
			return
		}
		var state string
		s.Eng.Read(func(st *cad.State) { state = st.Alert(id).State })
		writeJSON(w, 200, map[string]string{"id": id, "state": state})
	})
	s.handle("POST /api/alerts/{id}/approve", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		done(w, s.Eng.ApproveAlert(a, r.PathValue("id")))
	})
	s.handle("POST /api/alerts/{id}/reject", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Reason string `json:"reason"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.RejectAlert(a, r.PathValue("id"), b.Reason))
	})
	s.handle("GET /api/alerts/{id}/cap", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		b, ok := s.alertCAP(r.PathValue("id"), false)
		if !ok {
			fail(w, 404, "no alert "+r.PathValue("id"))
			return
		}
		w.Header().Set("Content-Type", "application/cap+xml; charset=utf-8")
		w.Write(b)
	})

	// The public CAP feed: alerts in force, each as its CAP message. Public by design (they are warnings
	// to the public), and only when the agency turns it on.
	s.mux.HandleFunc("GET /api/public/alerts.atom", func(w http.ResponseWriter, r *http.Request) {
		c := s.alertingConfig()
		if !c.Feed {
			fail(w, 404, "the public alert feed is off")
			return
		}
		base := s.publicBase(r)
		now := s.Eng.Now()
		var es []alerting.Entry
		updated := time.Time{}
		agency := ""
		s.Eng.Read(func(st *cad.State) {
			agency = firstNonEmptyStr(c.SenderName, st.Settings.Agency, "bitdispatcher")
			for _, p := range st.AlertsInForce(now) {
				t := firstTime(p.Sent, p.Created)
				if t.After(updated) {
					updated = t
				}
				es = append(es, alerting.Entry{ID: p.Identifier, Title: p.Text.Headline, Summary: p.Text.Short, Updated: t, Expires: p.Expires,
					CAPURL: base + "/api/public/alerts/" + p.ID + ".xml", Status: p.Status, Severity: p.Severity})
			}
		})
		if updated.IsZero() {
			updated = now
		}
		w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=30")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(alerting.Feed(base+"/api/public/alerts.atom", agency+" public alerts", agency, updated, es))
	})
	// The same alerts with the CAP messages inside, as the IPAWS All-Hazards Information Feed carries them.
	s.mux.HandleFunc("GET /api/public/alerts.xml", func(w http.ResponseWriter, r *http.Request) {
		c := s.alertingConfig()
		if !c.Feed {
			fail(w, 404, "the public alert feed is off")
			return
		}
		now := s.Eng.Now()
		var ms []alerting.Message
		s.Eng.Read(func(st *cad.State) {
			for _, p := range st.AlertsInForce(now) {
				ms = append(ms, capMessage(st, p, c))
			}
		})
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=30")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(alerting.MessagesFeed(ms))
	})
	s.mux.HandleFunc("GET /api/public/alerts/{file}", func(w http.ResponseWriter, r *http.Request) {
		if !s.alertingConfig().Feed {
			fail(w, 404, "the public alert feed is off")
			return
		}
		b, ok := s.alertCAP(strings.TrimSuffix(r.PathValue("file"), ".xml"), true)
		if !ok {
			fail(w, 404, "no such alert")
			return
		}
		w.Header().Set("Content-Type", "application/cap+xml; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=60")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(b)
	})
}

// alertCAP is an alert's CAP message; public asks only for one that was sent.
func (s *Server) alertCAP(id string, public bool) ([]byte, bool) {
	c := s.alertingConfig()
	var b []byte
	ok := false
	s.Eng.Read(func(st *cad.State) {
		p := st.Alert(id)
		if p == nil || public && p.Sent.IsZero() {
			return
		}
		b, _ = alerting.Encode(capMessage(st, p, c))
		ok = true
	})
	return b, ok
}

func (s *Server) checkGateways(ids []string) error {
	known := map[string]bool{}
	for _, x := range s.pagingConfig().Gateways {
		known[x.ID] = true
	}
	for _, id := range ids {
		if !known[id] {
			return &cad.Problem{Field: "gateways", Message: "no paging gateway " + id + " (Admin › Integrations › Paging)"}
		}
	}
	return nil
}

// readSubscribers reads a CSV with a header row naming its columns: name, phone, email, topic, lang, lat,
// lon (any order, any case; others are ignored).
func readSubscribers(r io.Reader) ([]cad.Subscriber, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	head, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("the file is empty or not CSV")
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))] = i
	}
	if _, ok := col["phone"]; !ok {
		if _, ok := col["email"]; !ok {
			if _, ok := col["topic"]; !ok {
				return nil, fmt.Errorf("the first row names the columns: name, phone, email, topic, lang, lat, lon")
			}
		}
	}
	get := func(row []string, k string) string {
		if i, ok := col[k]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	var out []cad.Subscriber
	for line := 2; ; line++ {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("line %d: %v", line, err)
		}
		sub := cad.Subscriber{Name: get(row, "name"), Phone: normPhone(get(row, "phone")), Email: get(row, "email"), Topic: get(row, "topic"), Lang: strings.ToLower(get(row, "lang"))}
		if sub.Phone == "" && sub.Email == "" && sub.Topic == "" {
			continue
		}
		if la, lo := get(row, "lat"), get(row, "lon"); la != "" || lo != "" {
			lat, e1 := strconv.ParseFloat(la, 64)
			lon, e2 := strconv.ParseFloat(lo, 64)
			if e1 != nil || e2 != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
				return nil, fmt.Errorf("line %d: latitude and longitude are numbers (37.27, -79.94)", line)
			}
			sub.Lat, sub.Lon = &lat, &lon
		}
		out = append(out, sub)
		if len(out) > 200000 {
			return nil, fmt.Errorf("more than 200,000 people: split the list")
		}
	}
	return out, nil
}

// normPhone writes a US number as E.164: (540) 555-0101 is +15405550101.
func normPhone(s string) string {
	if s == "" {
		return ""
	}
	plus := strings.HasPrefix(s, "+")
	var d strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			d.WriteRune(r)
		}
	}
	n := d.String()
	switch {
	case plus:
		return "+" + n
	case len(n) == 10:
		return "+1" + n
	case len(n) == 11 && n[0] == '1':
		return "+" + n
	}
	return s
}

// alerter delivers each alert issued here: its CAP message to the receivers, and the phone text to the
// people on its lists through the paging gateways.
func (s *Server) alerter(ctx context.Context) {
	ch := s.Eng.Subscribe()
	defer s.Eng.Unsubscribe(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-ch:
			if n.Type != cad.EvAlertIssued {
				continue
			}
			for _, id := range s.Eng.UndeliveredAlerts() {
				if p, ok := s.Eng.TakeAlert(id); ok {
					go s.deliverAlert(ctx, p)
				}
			}
		}
	}
}

func (s *Server) deliverAlert(ctx context.Context, p cad.PublicAlert) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	c := s.alertingConfig()
	var msg []byte
	var to map[string][]cad.Subscriber
	s.Eng.Read(func(st *cad.State) {
		if x := st.Alert(p.ID); x != nil {
			msg, _ = alerting.Encode(capMessage(st, x, c))
			to = st.AlertRecipients(x)
		}
	})
	var results []cad.PageResult
	client := &http.Client{Timeout: 15 * time.Second}
	for _, id := range p.Receivers {
		for _, rx := range c.Receivers {
			if rx.ID == id && !rx.Off {
				res := alerting.Post(ctx, client, rx, msg)
				results = append(results, cad.PageResult{Gateway: rx.Name, Kind: "cap", To: rx.Name, OK: res.OK, Detail: res.Detail, At: res.At})
			}
		}
	}
	gw := map[string]paging.Gateway{}
	for _, g := range s.pagingConfig().Gateways {
		gw[g.ID] = g
	}
	sender := s.pageSender()
	reached := map[string]bool{}
	text := func(t cad.AlertText) string {
		return firstNonEmptyStr(t.Long, t.Short, strings.TrimSpace(t.Description+" "+t.Instruction))
	}
	listGateways := map[string]bool{}
	s.Eng.Read(func(st *cad.State) {
		for _, l := range p.Lists {
			if x := st.AlertLists[l]; x != nil {
				for _, g := range x.Gateways {
					listGateways[g] = true
				}
			}
		}
	})
	ids := make([]string, 0, len(listGateways))
	for id := range listGateways {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, gid := range ids {
		g, ok := gw[gid]
		if !ok {
			results = append(results, cad.PageResult{Gateway: gid, OK: false, Detail: "no such paging gateway", At: time.Now()})
			continue
		}
		people := to[gid]
		page := func(spanish bool) paging.Page {
			t := p.Text
			prefix := ""
			if p.Status != "Actual" {
				prefix = strings.ToUpper(p.Status) + ": "
			}
			if spanish && p.Spanish != nil {
				t = *p.Spanish
			}
			return paging.Page{ID: p.ID, Incident: p.Incident, Subject: prefix + t.Headline, Text: prefix + text(t), Address: p.Area.Desc, Priority: 1, At: time.Now()}
		}
		switch g.Kind {
		case paging.Webhook, paging.IAmResponding, paging.Active911:
			for _, res := range sender.Send(ctx, g, page(false), nil) {
				results = append(results, cad.PageResult{Gateway: g.Name, Kind: g.Kind, To: res.To, OK: res.OK, Detail: res.Detail, At: res.At})
			}
			continue
		}
		if len(people) == 0 && g.Kind == paging.Ntfy {
			// A push gateway with no one listed publishes to its own topic: a public channel people follow.
			for _, res := range sender.Send(ctx, g, page(false), nil) {
				results = append(results, cad.PageResult{Gateway: g.Name, Kind: g.Kind, To: res.To, OK: res.OK, Detail: res.Detail, At: res.At})
			}
			continue
		}
		for _, x := range people {
			if ctx.Err() != nil {
				break
			}
			es := x.Lang == "es" && p.Spanish != nil
			for _, res := range sender.Send(ctx, g, page(es), []paging.Recipient{{Name: x.Name, Phone: x.Phone, Email: x.Email, Topic: x.Topic}}) {
				results = append(results, cad.PageResult{Gateway: g.Name, Kind: g.Kind, To: res.To, OK: res.OK, Detail: res.Detail, At: res.At})
				if res.OK {
					reached[x.Phone+"|"+x.Email+"|"+x.Topic] = true
				}
			}
		}
	}
	if len(results) == 0 {
		results = append(results, cad.PageResult{Gateway: "none", Detail: "no receivers or lists chosen: the alert is only in the public feed", OK: c.Feed, At: time.Now()})
	}
	// Keep the record short for a big list: every failure, and successes counted.
	results = squeeze(results)
	if err := s.Eng.RecordAlertResults(cad.Actor{User: "alerting", Role: "system"}, p.ID, results, len(reached)); err != nil {
		s.Log.Warn("alert results", "alert", p.ID, "err", err)
	}
}

// squeeze keeps up to 50 failures and the receivers' results, and sums the rest by gateway.
func squeeze(rs []cad.PageResult) []cad.PageResult {
	if len(rs) <= 60 {
		return rs
	}
	var out []cad.PageResult
	ok := map[string]int{}
	bad := map[string]int{}
	order := []string{}
	for _, r := range rs {
		k := r.Gateway + "\x00" + r.Kind
		if ok[k] == 0 && bad[k] == 0 {
			order = append(order, k)
		}
		switch {
		case r.Kind == "cap":
			out = append(out, r)
		case r.OK:
			ok[k]++
		case len(out) < 50:
			out = append(out, r)
			bad[k]++
		default:
			bad[k]++
		}
	}
	for _, k := range order {
		g, kind, _ := strings.Cut(k, "\x00")
		if ok[k] > 0 {
			out = append(out, cad.PageResult{Gateway: g, Kind: kind, To: fmt.Sprintf("%d people", ok[k]), OK: true, Detail: "sent", At: time.Now()})
		}
	}
	return out
}
