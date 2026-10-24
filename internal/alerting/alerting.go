// Package alerting writes public alerts as OASIS Common Alerting Protocol 1.2 messages, in the form the
// CAP-v1.2 USA IPAWS Profile v1.0 asks for, and publishes them: an Atom feed of current alerts that alert
// aggregators, sirens and broadcasters' encoders can poll, and a POST of each message to CAP receivers an
// agency names (its alert origination software, a siren controller, a neighbouring centre).
//
// Sending to FEMA's IPAWS-OPEN itself needs a Memorandum of Agreement with FEMA, a COG certificate and a
// signed SOAP exchange; that is not built. The messages are written to its profile so an agency's IPAWS
// alert origination software, or a future direct connection, can take them as they are.
//
// What was tested: the messages are checked against the OASIS CAP 1.2 schema and Google's CAP library
// with its IPAWS profile (scripts/cap-validator.sh, TestValidators), and POSTed to local test receivers.
// No message has been sent to IPAWS, the EAS or Wireless Emergency Alerts.
package alerting

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Namespace is CAP 1.2's.
const Namespace = "urn:oasis:names:tc:emergency:cap:1.2"

// ProfileCode marks a message as written to the IPAWS profile.
const ProfileCode = "IPAWSv1.0"

// Message is one CAP alert message.
type Message struct {
	XMLName    xml.Name `xml:"urn:oasis:names:tc:emergency:cap:1.2 alert"`
	Identifier string   `xml:"identifier"`
	Sender     string   `xml:"sender"`
	Sent       string   `xml:"sent"`
	Status     string   `xml:"status"`  // Actual, Exercise, System, Test, Draft
	MsgType    string   `xml:"msgType"` // Alert, Update, Cancel
	Source     string   `xml:"source,omitempty"`
	Scope      string   `xml:"scope"` // Public
	Code       []string `xml:"code,omitempty"`
	Note       string   `xml:"note,omitempty"`
	References string   `xml:"references,omitempty"` // sender,identifier,sent of each earlier message, space-separated
	Incidents  string   `xml:"incidents,omitempty"`
	Info       []Info   `xml:"info"`
}

// Info is one language's block.
type Info struct {
	Language     string      `xml:"language"`
	Category     []string    `xml:"category"`
	Event        string      `xml:"event"`
	ResponseType []string    `xml:"responseType,omitempty"`
	Urgency      string      `xml:"urgency"`
	Severity     string      `xml:"severity"`
	Certainty    string      `xml:"certainty"`
	EventCode    []ValuePair `xml:"eventCode,omitempty"`
	Expires      string      `xml:"expires,omitempty"`
	SenderName   string      `xml:"senderName,omitempty"`
	Headline     string      `xml:"headline,omitempty"`
	Description  string      `xml:"description,omitempty"`
	Instruction  string      `xml:"instruction,omitempty"`
	Web          string      `xml:"web,omitempty"`
	Contact      string      `xml:"contact,omitempty"`
	Parameter    []ValuePair `xml:"parameter,omitempty"`
	Area         []Area      `xml:"area"`
}

// ValuePair is CAP's name and value.
type ValuePair struct {
	ValueName string `xml:"valueName"`
	Value     string `xml:"value"`
}

// Area is where an alert applies.
type Area struct {
	AreaDesc string      `xml:"areaDesc"`
	Polygon  []string    `xml:"polygon,omitempty"`
	Circle   []string    `xml:"circle,omitempty"`
	Geocode  []ValuePair `xml:"geocode,omitempty"`
}

// Encode writes the message as CAP 1.2 XML.
func Encode(m Message) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(xml.Header)
	enc := xml.NewEncoder(&b)
	enc.Indent("", "  ")
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	b.WriteString("\n")
	return b.Bytes(), nil
}

// Decode reads a CAP 1.2 message.
func Decode(b []byte) (Message, error) {
	var m Message
	err := xml.Unmarshal(b, &m)
	if err == nil && m.XMLName.Space != Namespace {
		err = fmt.Errorf("not a CAP 1.2 message (namespace %q)", m.XMLName.Space)
	}
	return m, err
}

// Time writes a CAP date and time: seconds, and the offset written out (UTC is -00:00, never Z).
func Time(t time.Time) string {
	s := t.Truncate(time.Second).Format("2006-01-02T15:04:05-07:00")
	return strings.Replace(s, "+00:00", "-00:00", 1)
}

// ParseTime reads one.
func ParseTime(s string) (time.Time, error) {
	return time.Parse("2006-01-02T15:04:05-07:00", s)
}

// Reference is how a later message names an earlier one.
func Reference(m Message) string { return m.Sender + "," + m.Identifier + "," + m.Sent }

// EventCode is a SAME event code a local authority uses (47 CFR 11.31, and the FCC's additions since).
type EventCode struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Category string `json:"category"` // the CAP category it usually takes
	Warning  bool   `json:"warning"`  // a warning (urgent) rather than a statement or test
}

// EventCodes are the non-weather codes a 911 centre or emergency manager issues. Weather warnings come
// from the National Weather Service and are not here.
var EventCodes = []EventCode{
	{"CEM", "Civil emergency message", "Safety", true},
	{"CDW", "Civil danger warning", "Safety", true},
	{"EVI", "Evacuation immediate", "Safety", true},
	{"SPW", "Shelter in place warning", "Safety", true},
	{"LEW", "Law enforcement warning", "Security", true},
	{"CAE", "Child abduction emergency", "Rescue", true},
	{"BLU", "Blue alert", "Security", true},
	{"HMW", "Hazardous materials warning", "Env", true},
	{"FRW", "Fire warning", "Fire", true},
	{"RHW", "Radiological hazard warning", "CBRNE", true},
	{"NUW", "Nuclear power plant warning", "CBRNE", true},
	{"EQW", "Earthquake warning", "Geo", true},
	{"LAE", "Local area emergency", "Safety", false},
	{"TOE", "911 telephone outage emergency", "Infra", false},
	{"ADR", "Administrative message", "Other", false},
	{"DMO", "Practice / demonstration warning", "Other", false},
	{"RMT", "Required monthly test", "Other", false},
	{"RWT", "Required weekly test", "Other", false},
}

// Event finds a SAME code.
func Event(code string) (EventCode, bool) {
	for _, e := range EventCodes {
		if e.Code == code {
			return e, true
		}
	}
	return EventCode{}, false
}

// Channels an IPAWS message can be kept off with its BLOCKCHANNEL parameter.
var Channels = []string{"EAS", "NWEM", "CMAS", "PUBLIC"}

var (
	reSAME       = regexp.MustCompile(`^\d{6}$`)
	reCode3      = regexp.MustCompile(`^[A-Z]{3}$`)
	reIdentifier = regexp.MustCompile(`^[^\s,<&]+$`)
	statuses     = []string{"Actual", "Exercise", "System", "Test", "Draft"}
	msgTypes     = []string{"Alert", "Update", "Cancel"}
	categories   = []string{"Geo", "Met", "Safety", "Security", "Rescue", "Fire", "Health", "Env", "Transport", "Infra", "CBRNE", "Other"}
	responses    = []string{"Shelter", "Evacuate", "Prepare", "Execute", "Avoid", "Monitor", "Assess", "AllClear", "None"}
	urgencies    = []string{"Immediate", "Expected", "Future", "Past", "Unknown"}
	severities   = []string{"Extreme", "Severe", "Moderate", "Minor", "Unknown"}
	certainties  = []string{"Observed", "Likely", "Possible", "Unlikely", "Unknown"}
)

func in(v string, set []string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// Check returns what is wrong with a message: CAP 1.2's rules, then the IPAWS profile's. Nothing means
// it is ready to send.
func Check(m Message) []string {
	var p []string
	add := func(f string, a ...any) { p = append(p, fmt.Sprintf(f, a...)) }
	if !reIdentifier.MatchString(m.Identifier) {
		add("the identifier is required, with no spaces, commas, < or &")
	}
	if !reIdentifier.MatchString(m.Sender) {
		add("the sender is required, with no spaces, commas, < or & (Admin › Integrations › Public alerts)")
	}
	sent, err := ParseTime(m.Sent)
	if err != nil {
		add("the time sent is written 2026-10-24T08:15:00-05:00")
	} else if strings.HasSuffix(m.Sent, "-00:00") {
		add("the times are in UTC: set the agency's time zone (the IPAWS profile expects a US offset)")
	}
	if !in(m.Status, statuses) {
		add("status %q is not a CAP status", m.Status)
	}
	if !in(m.MsgType, msgTypes) {
		add("message type %q is not Alert, Update or Cancel", m.MsgType)
	}
	if m.Scope != "Public" {
		add("a public alert's scope is Public")
	}
	if !in(ProfileCode, m.Code) {
		add("the IPAWS profile's code %s is missing", ProfileCode)
	}
	if m.MsgType != "Alert" && m.References == "" {
		add("an update or cancel names the messages it follows (references)")
	}
	for _, r := range strings.Fields(m.References) {
		if strings.Count(r, ",") != 2 {
			add("reference %q is sender,identifier,sent", r)
		}
	}
	if len(m.Info) == 0 {
		add("a message needs at least one info block")
	}
	var codes, cats string
	for i, f := range m.Info {
		at := ""
		if len(m.Info) > 1 {
			at = " (" + f.Language + ")"
		}
		if f.Language == "" {
			add("info %d: the language is required (en-US, es-US)", i+1)
		}
		if len(f.Category) == 0 {
			add("the category is required%s", at)
		}
		for _, c := range f.Category {
			if !in(c, categories) {
				add("category %q is not a CAP category%s", c, at)
			}
		}
		for _, r := range f.ResponseType {
			if !in(r, responses) {
				add("response %q is not a CAP response type%s", r, at)
			}
		}
		if strings.TrimSpace(f.Event) == "" {
			add("say what the event is%s", at)
		}
		if !in(f.Urgency, urgencies) || !in(f.Severity, severities) || !in(f.Certainty, certainties) {
			add("urgency, severity and certainty must each be one of CAP's values%s", at)
		}
		same := 0
		var cs []string
		for _, ec := range f.EventCode {
			if ec.ValueName == "SAME" {
				same++
				if !reCode3.MatchString(ec.Value) {
					add("the SAME event code %q is three capital letters%s", ec.Value, at)
				}
			}
			cs = append(cs, ec.ValueName+"="+ec.Value)
		}
		if same != 1 {
			add("an IPAWS message has exactly one SAME event code%s", at)
		}
		if i == 0 {
			codes, cats = strings.Join(cs, " "), strings.Join(f.Category, " ")
		} else if strings.Join(cs, " ") != codes || strings.Join(f.Category, " ") != cats {
			add("every language must have the same event code and category%s", at)
		}
		exp, err := ParseTime(f.Expires)
		if err != nil {
			add("an IPAWS message needs its expiry time%s", at)
		} else if !sent.IsZero() && !exp.After(sent) {
			add("it expires before it is sent%s", at)
		}
		if strings.TrimSpace(f.Headline) == "" {
			add("write the headline%s", at)
		} else if utf8.RuneCountInString(f.Headline) > 160 {
			add("the headline is up to 160 characters%s", at)
		}
		if strings.TrimSpace(f.Description) == "" && strings.TrimSpace(f.Instruction) == "" {
			add("say what is happening or what to do%s", at)
		}
		for _, pv := range f.Parameter {
			switch pv.ValueName {
			case "CMAMtext":
				if n := utf8.RuneCountInString(pv.Value); n > 90 {
					add("the phone alert (WEA) text is %d characters; the limit is 90%s", n, at)
				}
			case "CMAMlongtext":
				if n := utf8.RuneCountInString(pv.Value); n > 360 {
					add("the long phone alert text is %d characters; the limit is 360%s", n, at)
				}
			case "BLOCKCHANNEL":
				if !in(pv.Value, Channels) {
					add("BLOCKCHANNEL %q is EAS, NWEM, CMAS or PUBLIC%s", pv.Value, at)
				}
			}
		}
		if len(f.Area) == 0 {
			add("say where the alert applies%s", at)
		}
		for _, a := range f.Area {
			if strings.TrimSpace(a.AreaDesc) == "" {
				add("describe the area%s", at)
			}
			if len(a.Polygon) == 0 && len(a.Circle) == 0 && len(a.Geocode) == 0 {
				add("the area %q has no shape or county code%s", a.AreaDesc, at)
			}
			for _, pg := range a.Polygon {
				if err := checkPolygon(pg); err != nil {
					add("the area %q: %v%s", a.AreaDesc, err, at)
				}
			}
			for _, g := range a.Geocode {
				if g.ValueName == "SAME" && !reSAME.MatchString(g.Value) {
					add("county code %q is six digits: 0, the state's FIPS code and the county's (051770)%s", g.Value, at)
				}
			}
		}
	}
	return p
}

func checkPolygon(s string) error {
	pts := strings.Fields(s)
	if len(pts) < 4 {
		return errors.New("a polygon needs at least four points, the last the same as the first")
	}
	if pts[0] != pts[len(pts)-1] {
		return errors.New("a polygon ends where it starts")
	}
	for _, p := range pts {
		la, lo, ok := strings.Cut(p, ",")
		lat, e1 := strconv.ParseFloat(la, 64)
		lon, e2 := strconv.ParseFloat(lo, 64)
		if !ok || e1 != nil || e2 != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			return fmt.Errorf("%q is not latitude,longitude", p)
		}
	}
	return nil
}

// Polygon writes [lon, lat] points (the map's order) as a CAP polygon (latitude,longitude pairs),
// closing it.
func Polygon(pts [][2]float64) string {
	if len(pts) == 0 {
		return ""
	}
	if pts[0] != pts[len(pts)-1] {
		pts = append(pts, pts[0])
	}
	var b strings.Builder
	for i, p := range pts {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strconv.FormatFloat(p[1], 'f', 5, 64) + "," + strconv.FormatFloat(p[0], 'f', 5, 64))
	}
	return b.String()
}

// Entry is one alert in the feed.
type Entry struct {
	ID       string // the CAP identifier
	Title    string
	Summary  string
	Updated  time.Time
	Expires  time.Time
	CAPURL   string // where the message itself is
	Status   string
	Severity string
}

// Feed writes an Atom feed of current alerts, each linking to its CAP message.
func Feed(self, title, author string, updated time.Time, es []Entry) []byte {
	type link struct {
		Rel  string `xml:"rel,attr,omitempty"`
		Type string `xml:"type,attr,omitempty"`
		Href string `xml:"href,attr"`
	}
	type entry struct {
		ID      string `xml:"id"`
		Title   string `xml:"title"`
		Updated string `xml:"updated"`
		Author  struct {
			Name string `xml:"name"`
		} `xml:"author"`
		Summary string `xml:"summary,omitempty"`
		Link    link   `xml:"link"`
	}
	type feed struct {
		XMLName xml.Name `xml:"http://www.w3.org/2005/Atom feed"`
		ID      string   `xml:"id"`
		Title   string   `xml:"title"`
		Updated string   `xml:"updated"`
		Author  struct {
			Name string `xml:"name"`
		} `xml:"author"`
		Link    link    `xml:"link"`
		Entries []entry `xml:"entry"`
	}
	f := feed{ID: self, Title: title, Updated: updated.UTC().Format(time.RFC3339), Link: link{Rel: "self", Href: self}}
	f.Author.Name = author
	for _, e := range es {
		x := entry{ID: e.CAPURL, Title: e.Title, Updated: e.Updated.UTC().Format(time.RFC3339), Summary: e.Summary,
			Link: link{Rel: "alternate", Type: "application/cap+xml", Href: e.CAPURL}}
		x.Author.Name = author
		f.Entries = append(f.Entries, x)
	}
	var b bytes.Buffer
	b.WriteString(xml.Header)
	enc := xml.NewEncoder(&b)
	enc.Indent("", "  ")
	enc.Encode(f)
	b.WriteString("\n")
	return b.Bytes()
}

// Receiver is a system each message is POSTed to as CAP XML.
type Receiver struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	URL   string `json:"url"`
	Token string `json:"token,omitempty"` // sent as Authorization: Bearer
	Off   bool   `json:"off,omitempty"`
}

// Check says what is wrong with a receiver: it needs a name and an https address (or one on this
// machine), so a token is never sent in the clear.
func (r Receiver) Check() error {
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("give the receiver a name")
	}
	u, err := url.Parse(r.URL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("the receiver's address is https://…")
	}
	if u.Scheme == "http" && !loopback(u.Hostname()) {
		return errors.New("the receiver's address must be https:// (http only to this machine)")
	}
	return nil
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Redacted is the receiver with its token shown as dots.
func (r Receiver) Redacted() Receiver {
	if r.Token != "" {
		r.Token = "••••"
	}
	return r
}

// Result is how one delivery went.
type Result struct {
	To     string
	OK     bool
	Detail string
	At     time.Time
}

// Post sends one message to a receiver.
func Post(ctx context.Context, c *http.Client, r Receiver, msg []byte) Result {
	if c == nil {
		c = &http.Client{Timeout: 15 * time.Second}
	}
	res := Result{To: r.Name, At: time.Now()}
	if err := r.Check(); err != nil {
		res.Detail = err.Error()
		return res
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL, bytes.NewReader(msg))
	if err != nil {
		res.Detail = err.Error()
		return res
	}
	req.Header.Set("Content-Type", "application/cap+xml; charset=utf-8")
	if r.Token != "" {
		req.Header.Set("Authorization", "Bearer "+r.Token)
	}
	resp, err := c.Do(req)
	if err != nil {
		res.Detail = err.Error()
		return res
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
	resp.Body.Close()
	res.OK = resp.StatusCode/100 == 2
	res.Detail = fmt.Sprintf("HTTP %d", resp.StatusCode)
	if !res.OK && len(bytes.TrimSpace(body)) > 0 {
		res.Detail += ": " + strings.TrimSpace(string(body))
	}
	return res
}

// Text is what an alert says in one language.
type Text struct {
	Language    string // en-US, es-US
	Headline    string
	Description string
	Instruction string
	Short       string // the phone alert (WEA), up to 90 characters
	Long        string // the longer phone alert, up to 360
}

// Spec is an alert to write as a CAP message.
type Spec struct {
	Sender, SenderName string // the agency's CAP sender ID (an address it owns) and its name
	Identifier         string
	Sent, Expires      time.Time
	Status, MsgType    string
	References         []string // earlier messages, as Reference gives them
	Incident           string
	Event, EventName   string // SAME code and its name
	Category, Response string
	Urgency, Severity  string
	Certainty          string
	Texts              []Text
	AreaDesc           string
	Polygon            [][2]float64 // [lon, lat]
	Geocodes           []string     // SAME county codes
	Block              []string     // BLOCKCHANNEL values
	Web, Contact       string
}

// Build writes the CAP message for an alert, to the IPAWS profile: its code, the SAME event code, the
// civil authority's EAS-ORG, the phone texts as CMAMtext and CMAMlongtext, and the area's polygon and
// SAME county codes, in every language.
func Build(s Spec) Message {
	m := Message{Identifier: s.Identifier, Sender: s.Sender, Sent: Time(s.Sent), Status: s.Status, MsgType: s.MsgType,
		Source: s.SenderName, Scope: "Public", Code: []string{ProfileCode}, References: strings.Join(s.References, " "), Incidents: s.Incident}
	area := Area{AreaDesc: s.AreaDesc}
	if len(s.Polygon) >= 3 {
		area.Polygon = []string{Polygon(s.Polygon)}
	}
	for _, g := range s.Geocodes {
		area.Geocode = append(area.Geocode, ValuePair{"SAME", g})
	}
	for _, t := range s.Texts {
		f := Info{Language: t.Language, Category: []string{s.Category}, Event: s.EventName, Urgency: s.Urgency, Severity: s.Severity,
			Certainty: s.Certainty, EventCode: []ValuePair{{"SAME", s.Event}}, Expires: Time(s.Expires), SenderName: s.SenderName,
			Headline: t.Headline, Description: t.Description, Instruction: t.Instruction, Web: s.Web, Contact: s.Contact, Area: []Area{area}}
		if s.Response != "" {
			f.ResponseType = []string{s.Response}
		}
		f.Parameter = append(f.Parameter, ValuePair{"EAS-ORG", "CIV"})
		if t.Short != "" {
			f.Parameter = append(f.Parameter, ValuePair{"CMAMtext", t.Short})
		}
		if t.Long != "" {
			f.Parameter = append(f.Parameter, ValuePair{"CMAMlongtext", t.Long})
		}
		for _, b := range s.Block {
			f.Parameter = append(f.Parameter, ValuePair{"BLOCKCHANNEL", b})
		}
		m.Info = append(m.Info, f)
	}
	return m
}

// FeedNamespace is the IPAWS All-Hazards Information Feed's.
const FeedNamespace = "http://gov.fema.ipaws.services/feed"

// MessagesFeed writes current alerts the way the IPAWS All-Hazards Information Feed does: the CAP messages
// themselves inside one <alerts> element, for readers built for that feed (EAS encoders and their
// software poll it).
func MessagesFeed(ms []Message) []byte {
	type alerts struct {
		XMLName xml.Name  `xml:"http://gov.fema.ipaws.services/feed alerts"`
		Alerts  []Message `xml:"alert"`
	}
	var b bytes.Buffer
	b.WriteString(xml.Header)
	enc := xml.NewEncoder(&b)
	enc.Indent("", "  ")
	enc.Encode(alerts{Alerts: ms})
	b.WriteString("\n")
	return b.Bytes()
}
