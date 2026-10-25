package cad

// What 911 centres asked for: live transcripts with the details pulled out, translation, crash data from
// vehicles, grouped callers, non-emergency screening, radio transcripts, drones, care after critical calls,
// breaks, and a watch for telephony denial-of-service. All of it is kept as ledger events and the state
// derived from them, like everything else, so a replay rebuilds it exactly.

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/acn"
	"github.com/biodream-llc/bitdispatcher/internal/assist"
	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// Events added for the wishlist.
const (
	EvCallScreened  = "call.screened"   // a person said a call is, or is not, an emergency
	EvRadioText     = "radio.text"      // what a radio transmission said, as speech-to-text heard it
	EvDroneUpdated  = "drone.updated"   // a drone's status, position or video
	EvDroneRequest  = "drone.requested" // a drone was asked for an incident
	EvCareAction    = "care.action"     // a supervisor's care after a critical call: a break offered, a debrief
	EvBreak         = "position.break"  // a person went on a break, or came back
	EvTDoSAcked     = "tdos.acked"      // a supervisor acknowledged a possible telephony denial-of-service
	maxTranscript   = 400               // lines kept per call
	maxFixes        = 30                // caller locations kept per call, for the trail on the map
	maxRadioLog     = 200
	tdosWindow      = time.Hour
	tdosRepeatSpan  = 10 * time.Minute
	tdosShortSpan   = 5 * time.Minute
	tdosShortCall   = 20 * time.Second
	tdosQuietPeriod = 10 * time.Minute
)

// TranscriptLine is one thing said on a call, as a speech-to-text engine heard it.
type TranscriptLine struct {
	At      time.Time `json:"at"`
	Speaker string    `json:"speaker"` // caller, taker or other
	Text    string    `json:"text"`
	Lang    string    `json:"lang,omitempty"`    // es, en...; detected when the engine does not say
	English string    `json:"english,omitempty"` // the English, when it was said in another language
	By      string    `json:"by,omitempty"`      // who translated it: source, model or phrasebook
	Gist    bool      `json:"gist,omitempty"`    // the phrasebook knew too few of the words: only a gist
}

// LineTranslation is a translation of one transcript line that arrived later (from a language model).
type LineTranslation struct {
	Index   int    `json:"index"`
	English string `json:"english"`
	By      string `json:"by"`
}

// Fix is one location received for a caller.
type Fix struct {
	Lat      float64   `json:"lat"`
	Lon      float64   `json:"lon"`
	Radius   float64   `json:"radius,omitempty"`
	Altitude *float64  `json:"altitude,omitempty"`
	Method   string    `json:"method,omitempty"`
	At       time.Time `json:"at"`
}

// Screen is whether a call looks like an emergency, and why.
type Screen struct {
	NonEmergency bool   `json:"nonEmergency"`
	Reason       string `json:"reason,omitempty"`
	By           string `json:"by,omitempty"` // the person who decided, when one did
}

// RadioLine is one radio transmission's words.
type RadioLine struct {
	At          time.Time `json:"at"`
	Unit        string    `json:"unit,omitempty"`
	RadioUnit   string    `json:"radioUnit,omitempty"`
	Channel     string    `json:"channel,omitempty"`
	ChannelName string    `json:"channelName,omitempty"`
	Text        string    `json:"text"`
	Incident    string    `json:"incident,omitempty"`
}

// Drone is a drone-as-first-responder aircraft, as its system reports it.
type Drone struct {
	ID       string    `json:"id"`
	Name     string    `json:"name,omitempty"`
	Status   string    `json:"status"` // docked, launching, enroute, onscene, returning, offline
	Lat      *float64  `json:"lat,omitempty"`
	Lon      *float64  `json:"lon,omitempty"`
	AltM     float64   `json:"altM,omitempty"`
	Battery  int       `json:"battery,omitempty"` // percent
	Incident string    `json:"incident,omitempty"`
	VideoURL string    `json:"videoUrl,omitempty"` // the live picture, for the console to open
	Pilot    string    `json:"pilot,omitempty"`
	Updated  time.Time `json:"updated"`
}

// DroneRequest is a request sent to the drone system for an incident.
type DroneRequest struct {
	OK     bool   `json:"ok"`
	Auto   bool   `json:"auto,omitempty"` // sent by the launch rule, not a person
	Detail string `json:"detail,omitempty"`
}

// CareCase is a critical call and the people who worked it, so a supervisor can look after them.
type CareCase struct {
	Incident string     `json:"incident"`
	Reason   string     `json:"reason"`
	At       time.Time  `json:"at"`
	People   []string   `json:"people"` // usernames
	Takers   []string   `json:"takers,omitempty"`
	Status   string     `json:"status"` // open, debrief (a debrief is arranged), closed
	Offered  []string   `json:"offered,omitempty"`
	Actions  []CareNote `json:"actions,omitempty"`
}

// CareNote is one thing a supervisor did.
type CareNote struct {
	At     time.Time `json:"at"`
	By     string    `json:"by"`
	Action string    `json:"action"` // break, debrief, close, note, dismiss
	User   string    `json:"user,omitempty"`
	Note   string    `json:"note,omitempty"`
}

type careData struct {
	Action string `json:"action"`
	User   string `json:"user,omitempty"`
	Note   string `json:"note,omitempty"`
}

// Break is a person's breaks.
type Break struct {
	On      bool      `json:"on"`
	Since   time.Time `json:"since,omitzero"`   // when the current break began
	LastEnd time.Time `json:"lastEnd,omitzero"` // when the last break ended
	Count   int       `json:"count"`
}

type breakData struct {
	On bool `json:"on"`
}

// TDoS is a possible telephony denial-of-service: far more calls than usual, one number calling again and
// again, or a run of silent short calls.
type TDoS struct {
	Active   bool      `json:"active"`
	Since    time.Time `json:"since,omitzero"`
	Last     time.Time `json:"last,omitzero"` // the last call that met a condition
	Reasons  []string  `json:"reasons"`
	PerMin   int       `json:"perMin"`
	Baseline float64   `json:"baseline"` // calls a minute over the last hour
	Repeats  []string  `json:"repeats,omitempty"`
	AckedBy  string    `json:"ackedBy,omitempty"`
	AckedAt  time.Time `json:"ackedAt,omitzero"`
}

type tdosTrack struct {
	starts []time.Time
	by     map[string][]time.Time
	short  []time.Time
}

// ---- settings with defaults ----

// DefaultNonEmergencyWords are what callers say on calls that are seldom emergencies.
var DefaultNonEmergencyWords = []string{"noise complaint", "loud music", "barking dog", "parking", "parked in", "blocking my driveway",
	"abandoned vehicle", "fireworks", "lost property", "found property", "not an emergency", "non-emergency", "file a report",
	"report a theft", "stolen yesterday", "power is out", "power outage", "pothole", "question about", "what time", "phone number for"}

// DefaultCriticalWords mark an incident as a critical call for the people who work it.
var DefaultCriticalWords = []string{"not breathing", "cpr", "cardiac arrest", "officer down", "officer shot", "shots fired", "gunshot",
	"been shot", "stabbed", "suicide", "drowning", "drowned", "deceased", "dead body", "infant", "baby", "child not", "fatal",
	"hanging", "trapped", "burned alive", "sids"}

// QA is what the quality review checks.
type QA struct {
	AnswerSeconds   int                 `json:"answerSeconds,omitempty"`   // answered within; NENA: 90% in 15 s
	EntrySeconds    int                 `json:"entrySeconds,omitempty"`    // answer to incident created
	DispatchSeconds int                 `json:"dispatchSeconds,omitempty"` // created to first unit sent, priority 1-2
	Questions       map[string][]string `json:"questions,omitempty"`       // per discipline: the questions to hear the taker ask
}

// DefaultQuestions are the questions the review listens for, by discipline. Each entry is a set of words
// separated by |, any of which counts.
var DefaultQuestions = map[string][]string{
	"all":    {"address|where are you|location|what is the address", "phone number|call back|callback"},
	"ems":    {"breathing|breathe", "awake|conscious|responsive|talk to you", "how old|age"},
	"fire":   {"anyone inside|anybody inside|people inside|everyone out", "what is burning|what's on fire|smoke"},
	"police": {"weapon|gun|knife", "describe|description|wearing", "which way|direction|where did they go"},
}

// QASettings is the review's settings with defaults filled in.
func (s Settings) QASettings() QA {
	q := s.QA
	if q.AnswerSeconds <= 0 {
		q.AnswerSeconds = 15
	}
	if q.EntrySeconds <= 0 {
		q.EntrySeconds = 90
	}
	if q.DispatchSeconds <= 0 {
		q.DispatchSeconds = 60
	}
	if len(q.Questions) == 0 {
		q.Questions = DefaultQuestions
	}
	return q
}

func (s Settings) nonEmergencyWords() []string {
	if len(s.NonEmergencyWords) > 0 {
		return s.NonEmergencyWords
	}
	return DefaultNonEmergencyWords
}

func (s Settings) criticalWords() []string {
	if len(s.CriticalWords) > 0 {
		return s.CriticalWords
	}
	return DefaultCriticalWords
}

// BreakAfter is how long a person works before the supervisor board suggests a break.
func (s Settings) BreakAfter() time.Duration {
	if s.BreakMinutes > 0 {
		return time.Duration(s.BreakMinutes) * time.Minute
	}
	return 2 * time.Hour
}

// CrashThresholds are the delta-V thresholds for crash data.
func (s Settings) CrashThresholds() acn.Thresholds {
	t := acn.DefaultThresholds()
	if s.CrashHighKmh > 0 {
		t.HighDeltaVKmh = s.CrashHighKmh
	}
	if s.CrashMediumKmh > 0 {
		t.MediumDeltaVKmh = s.CrashMediumKmh
	}
	return t
}

// ClusterRule is how near and how close in time calls must be to be shown as one group.
func (s Settings) ClusterRule() (metres float64, window time.Duration) {
	metres, window = 300, 10*time.Minute
	if s.ClusterMeters > 0 {
		metres = s.ClusterMeters
	}
	if s.ClusterMinutes > 0 {
		window = time.Duration(s.ClusterMinutes) * time.Minute
	}
	return
}

// ---- calls ----

// mergeDream merges what the wishlist added to a call. The caller has merged the rest.
func (s *State) mergeDream(c *Call, d CallData, at time.Time) {
	if d.Line != "" {
		c.Line = d.Line
	}
	for _, l := range d.Transcript {
		l.Text = strings.TrimSpace(l.Text)
		if l.Text == "" {
			continue
		}
		if l.At.IsZero() {
			l.At = at
		}
		if l.Speaker == "" {
			l.Speaker = "caller"
		}
		if l.Lang == "" {
			l.Lang = assist.DetectLang(l.Text)
		}
		if l.Lang != "" && l.Lang != "en" && l.English == "" && assist.Knows(l.Lang) {
			tr := assist.Translate(l.Text, l.Lang, "en")
			// Only Spanish's phrasebook translates; the others give the emergency words: always a gist.
			l.English, l.By, l.Gist = tr.Text, "phrasebook", tr.Coverage < 0.6 || l.Lang != "es"
		}
		if l.English != "" && l.By == "" {
			l.By = "source"
		}
		if l.Speaker == "caller" && l.Lang != "" {
			c.Lang = l.Lang
		}
		c.Transcript = append(c.Transcript, l)
	}
	if len(c.Transcript) > maxTranscript {
		c.Transcript = c.Transcript[len(c.Transcript)-maxTranscript:]
	}
	for _, t := range d.Translations {
		if t.Index >= 0 && t.Index < len(c.Transcript) && t.English != "" {
			c.Transcript[t.Index].English, c.Transcript[t.Index].By, c.Transcript[t.Index].Gist = t.English, firstNonEmpty(t.By, "model"), false
		}
	}
	if d.Crash != nil {
		cr := *d.Crash
		c.Crash = &cr
		as := acn.Assess(cr, s.Settings.CrashThresholds())
		c.CrashRisk = &as
		if c.Callback == "" {
			c.Callback = cr.Callback
		}
	}
	if d.Alert != nil {
		al := *d.Alert
		if c.Alert != nil && al.Status == "" {
			al.Status = c.Alert.Status
		}
		c.Alert = &al
	}
	if len(d.Transcript) > 0 || len(d.Translations) > 0 || d.Crash != nil {
		c.Details = s.callDetails(c)
	}
	if c.Location.HasPoint() {
		f := Fix{Lat: *c.Location.Lat, Lon: *c.Location.Lon, Radius: c.Location.Radius, Altitude: c.Location.Altitude, Method: c.Location.Method, At: c.Location.At}
		if n := len(c.Fixes); n == 0 || c.Fixes[n-1].Lat != f.Lat || c.Fixes[n-1].Lon != f.Lon || c.Fixes[n-1].Radius != f.Radius {
			c.Fixes = append(c.Fixes, f)
			if len(c.Fixes) > maxFixes {
				c.Fixes = c.Fixes[len(c.Fixes)-maxFixes:]
			}
		}
	}
	if c.Screen == nil || c.Screen.By == "" {
		c.Screen = s.screen(c)
	}
}

// CallerWords is everything the caller said, in English where there is a translation.
func (c *Call) CallerWords() string {
	var b strings.Builder
	for _, l := range c.Transcript {
		if l.Speaker != "caller" {
			continue
		}
		b.WriteString(firstNonEmpty(l.English, l.Text))
		b.WriteString(". ")
	}
	for _, t := range c.Text {
		if t.From == "caller" {
			b.WriteString(t.Text)
			b.WriteString(". ")
		}
	}
	return b.String()
}

// TakerWords is everything the call-taker said or sent, in English.
func (c *Call) TakerWords() string {
	var b strings.Builder
	for _, l := range c.Transcript {
		if l.Speaker == "taker" {
			b.WriteString(firstNonEmpty(l.English, l.Text))
			b.WriteString(". ")
		}
	}
	for _, t := range c.Text {
		if t.From != "caller" {
			b.WriteString(t.Text)
			b.WriteString(". ")
		}
	}
	return b.String()
}

func (s *State) callDetails(c *Call) []assist.Detail {
	out := assist.Extract(c.CallerWords())
	if cr := c.Crash; cr != nil {
		v := strings.Join(strings.Fields(strings.Join([]string{cr.Vehicle.Color, yearText(cr.Vehicle.Year), cr.Vehicle.Make, cr.Vehicle.Model}, " ")), " ")
		if v != "" {
			out = append(out, assist.Detail{Kind: "vehicle", Text: v, Quote: "crash data"})
		}
		if cr.Vehicle.Plate != "" {
			out = append(out, assist.Detail{Kind: "plate", Text: strings.ToUpper(cr.Vehicle.Plate), Quote: "crash data"})
		}
		if cr.Occupants > 0 {
			out = append(out, assist.Detail{Kind: "count", Text: fmt.Sprintf("%d occupants", cr.Occupants), Quote: "crash data"})
		}
	}
	return out
}

func yearText(y int) string {
	if y <= 0 {
		return ""
	}
	return fmt.Sprint(y)
}

// screen decides whether a call looks like a non-emergency: the line it came in on, or what the caller
// said, unless something urgent was said too.
func (s *State) screen(c *Call) *Screen {
	for _, d := range c.Details {
		if d.Urgent {
			return &Screen{Reason: "urgent: " + d.Text}
		}
	}
	if c.Crash != nil {
		return &Screen{Reason: "crash data"}
	}
	if c.Line == IntakeLine {
		return &Screen{NonEmergency: true, Reason: "a report taken by the non-emergency intake"}
	}
	if c.Line != "" {
		for _, l := range s.Settings.NonEmergencyLines {
			if l != "" && (strings.EqualFold(l, c.Line) || digits(l) != "" && digits(l) == digits(c.Line)) {
				return &Screen{NonEmergency: true, Reason: "came in on the non-emergency line " + c.Line}
			}
		}
	}
	words := strings.ToLower(c.CallerWords())
	for _, w := range s.Settings.nonEmergencyWords() {
		if w = strings.ToLower(strings.TrimSpace(w)); w != "" && strings.Contains(words, w) {
			return &Screen{NonEmergency: true, Reason: "the caller said “" + w + "”"}
		}
	}
	return nil
}

func digits(v string) string {
	var b strings.Builder
	for _, r := range v {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	if len(d) == 11 && d[0] == '1' {
		d = d[1:]
	}
	return d
}

// ---- engine commands ----

// ScreenCall records a person's decision that a call is an emergency or is not.
func (e *Engine) ScreenCall(a Actor, id string, emergency bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.st.Calls[id]; !ok {
		return problem("call", "no call %s", id)
	}
	_, err := e.write(a, EvCallScreened, refs{Call: id}, map[string]bool{"emergency": emergency})
	return err
}

// RadioText records what a radio transmission said.
func (e *Engine) RadioText(a Actor, l RadioLine) error {
	l.Text = strings.TrimSpace(l.Text)
	if l.Text == "" {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if l.Unit != "" {
		if u, ok := e.st.Units[l.Unit]; ok {
			l.Incident = u.Incident
		}
	}
	_, err := e.write(a, EvRadioText, refs{Unit: l.Unit, Incident: l.Incident}, l)
	return err
}

// UpdateDrone records what the drone system says about a drone.
func (e *Engine) UpdateDrone(a Actor, d Drone) error {
	d.ID = strings.TrimSpace(d.ID)
	if d.ID == "" {
		return problem("id", "a drone needs an id")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if d.Incident != "" {
		if _, ok := e.st.Incidents[d.Incident]; !ok {
			d.Incident = ""
		}
	}
	_, err := e.write(a, EvDroneUpdated, refs{Incident: d.Incident}, d)
	return err
}

// DroneRequested records a request to the drone system for an incident, and how it went.
func (e *Engine) DroneRequested(a Actor, number string, r DroneRequest) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.st.Incidents[number]; !ok {
		return problem("incident", "no incident %s", number)
	}
	_, err := e.write(a, EvDroneRequest, refs{Incident: number}, r)
	return err
}

// Care records a supervisor's care for the people on a critical call.
func (e *Engine) Care(a Actor, number, action, user, note string) error {
	switch action {
	case "break", "debrief", "close", "note", "dismiss":
	default:
		return problem("action", "unknown care action %q", action)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.st.Care[number]; !ok {
		return problem("incident", "%s is not a critical call", number)
	}
	_, err := e.write(a, EvCareAction, refs{Incident: number}, careData{Action: action, User: user, Note: strings.TrimSpace(note)})
	return err
}

// SetBreak puts the signed-in person on a break, or brings them back.
func (e *Engine) SetBreak(a Actor, on bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if b, ok := e.st.Breaks[a.User]; ok && b.On == on || !ok && !on {
		return nil
	}
	_, err := e.write(a, EvBreak, refs{}, breakData{On: on})
	return err
}

// AckTDoS records that a supervisor has seen the denial-of-service warning.
func (e *Engine) AckTDoS(a Actor) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.st.TDoS.Active {
		return problem("tdos", "there is no warning to acknowledge")
	}
	_, err := e.write(a, EvTDoSAcked, refs{}, nil)
	return err
}

// ---- applying ----

// applyDream applies the wishlist's own events; it reports whether it knew the event.
func (s *State) applyDream(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvCallScreened:
		v, err := decode[map[string]bool](e)
		if err != nil {
			return true, err
		}
		if c, ok := s.Calls[e.Call]; ok {
			if v["emergency"] {
				c.Screen = &Screen{Reason: "an emergency, said " + e.Actor, By: e.Actor}
			} else {
				c.Screen = &Screen{NonEmergency: true, Reason: "not an emergency, said " + e.Actor, By: e.Actor}
			}
			c.Updated = e.At
		}
	case EvRadioText:
		v, err := decode[RadioLine](e)
		if err != nil {
			return true, err
		}
		v.At = firstTime(v.At, e.At)
		s.Radio = append(s.Radio, v)
		if len(s.Radio) > maxRadioLog {
			s.Radio = s.Radio[len(s.Radio)-maxRadioLog:]
		}
		if in, ok := s.Incidents[v.Incident]; ok {
			who := firstNonEmpty(v.Unit, v.RadioUnit, "radio")
			in.Timeline = append(in.Timeline, Entry{Seq: e.Seq, At: e.At, Actor: e.Actor, Kind: "radio", Text: who + ": " + v.Text, Unit: v.Unit})
			in.Updated = e.At
		}
	case EvDroneUpdated:
		v, err := decode[Drone](e)
		if err != nil {
			return true, err
		}
		v.Updated = e.At
		old := s.Drones[v.ID]
		if old != nil {
			if v.Name == "" {
				v.Name = old.Name
			}
			if v.Lat == nil {
				v.Lat, v.Lon = old.Lat, old.Lon
			}
			if v.VideoURL == "" && v.Incident == old.Incident {
				v.VideoURL = old.VideoURL
			}
		}
		s.Drones[v.ID] = &v
		if in, ok := s.Incidents[v.Incident]; ok && (old == nil || old.Status != v.Status || old.Incident != v.Incident) {
			in.add(e, "drone", "Drone "+firstNonEmpty(v.Name, v.ID)+" "+droneWords(v.Status))
		}
	case EvDroneRequest:
		v, err := decode[DroneRequest](e)
		if err != nil {
			return true, err
		}
		if in, ok := s.Incidents[e.Incident]; ok {
			text := "Drone requested"
			if v.Auto {
				text += " by the launch rule"
			}
			if !v.OK {
				text += ": failed"
			}
			if v.Detail != "" {
				text += ": " + v.Detail
			}
			in.add(e, "drone", text)
		}
	case EvCareAction:
		v, err := decode[careData](e)
		if err != nil {
			return true, err
		}
		cc, ok := s.Care[e.Incident]
		if !ok {
			return true, nil
		}
		cc.Actions = append(cc.Actions, CareNote{At: e.At, By: e.Actor, Action: v.Action, User: v.User, Note: v.Note})
		switch v.Action {
		case "break":
			if v.User != "" && !slices.Contains(cc.Offered, v.User) {
				cc.Offered = append(cc.Offered, v.User)
			}
		case "debrief":
			cc.Status = "debrief"
		case "close":
			cc.Status = "closed"
		}
	case EvBreak:
		v, err := decode[breakData](e)
		if err != nil {
			return true, err
		}
		b := s.Breaks[e.Actor]
		if b == nil {
			b = &Break{}
			s.Breaks[e.Actor] = b
		}
		if v.On && !b.On {
			b.On, b.Since, b.Count = true, e.At, b.Count+1
		} else if !v.On && b.On {
			b.On, b.LastEnd = false, e.At
		}
	case EvTDoSAcked:
		s.TDoS.AckedBy, s.TDoS.AckedAt = e.Actor, e.At
	default:
		return false, nil
	}
	return true, nil
}

func droneWords(st string) string {
	switch st {
	case "launching":
		return "launching"
	case "enroute":
		return "en route"
	case "onscene":
		return "on scene: live picture"
	case "returning":
		return "returning"
	case "docked":
		return "docked"
	}
	return st
}

func firstTime(t ...time.Time) time.Time {
	for _, v := range t {
		if !v.IsZero() {
			return v
		}
	}
	return time.Time{}
}

// afterApply derives what follows from an event: critical-call care and the denial-of-service watch.
func (s *State) afterApply(e ledger.Event) {
	switch e.Type {
	case EvCallReceived:
		s.tdosCall(e)
	case EvCallEnded:
		s.tdosEnded(e)
	}
	s.careCheck(e)
	s.roadmapAfter(e)
}

// ---- critical-call care ----

func human(s *State, actor string) bool {
	_, ok := s.Users[actor]
	return ok
}

// CriticalReason is why an incident is a critical call for the people working it, or "".
func (s *State) CriticalReason(in *Incident) string {
	if ct, ok := s.CallTypes[in.Type]; ok && ct.Critical {
		return ct.Name + " is a critical call type"
	}
	if s.Settings.CareOff {
		return ""
	}
	var b strings.Builder
	for _, t := range in.Timeline {
		if t.Kind == "note" || t.Kind == "priority-note" || t.Kind == "created" {
			b.WriteString(strings.ToLower(t.Text))
			b.WriteString(" ")
		}
	}
	for _, id := range in.Calls {
		if c, ok := s.Calls[id]; ok {
			b.WriteString(strings.ToLower(c.CallerWords()))
			for _, n := range c.Notes {
				b.WriteString(strings.ToLower(n) + " ")
			}
		}
	}
	text := b.String()
	for _, w := range s.Settings.criticalWords() {
		if w = strings.ToLower(strings.TrimSpace(w)); w != "" && containsWord(text, w) {
			return "“" + w + "” on the call"
		}
	}
	return ""
}

// containsWord is whether w appears in text at word boundaries.
func containsWord(text, w string) bool {
	for i := 0; ; {
		j := strings.Index(text[i:], w)
		if j < 0 {
			return false
		}
		j += i
		before := j == 0 || !isWordByte(text[j-1])
		after := j+len(w) >= len(text) || !isWordByte(text[j+len(w)])
		if before && after {
			return true
		}
		i = j + 1
	}
}

func isWordByte(b byte) bool { return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' }

func (s *State) careCheck(e ledger.Event) {
	num := e.Incident
	if num == "" && e.Call != "" {
		if c, ok := s.Calls[e.Call]; ok {
			num = c.Incident
		}
	}
	in, ok := s.Incidents[num]
	if !ok {
		return
	}
	cc := s.Care[num]
	if cc == nil {
		switch e.Type {
		case EvIncCreated, EvIncChanged, EvIncNote, EvCallAttached, EvCallReceived, EvCallUpdated:
		default:
			return
		}
		reason := s.CriticalReason(in)
		if reason == "" {
			return
		}
		cc = &CareCase{Incident: num, Reason: reason, At: e.At, People: []string{}, Status: "open"}
		s.Care[num] = cc
		addPerson(s, cc, in.CreatedBy)
		for _, t := range in.Timeline {
			addPerson(s, cc, t.Actor)
		}
		for _, id := range in.Calls {
			if c, ok := s.Calls[id]; ok && c.Agent != "" && !slices.Contains(cc.Takers, c.Agent) {
				cc.Takers = append(cc.Takers, c.Agent)
				addPerson(s, cc, c.Agent)
			}
		}
	}
	if e.Type == EvCareAction {
		return
	}
	addPerson(s, cc, e.Actor)
}

func addPerson(s *State, cc *CareCase, actor string) {
	if actor != "" && human(s, actor) && !slices.Contains(cc.People, actor) {
		cc.People = append(cc.People, actor)
	}
}

// ---- telephony denial-of-service ----

func (s *State) tdosCall(e ledger.Event) {
	t := &s.tdos
	if t.by == nil {
		t.by = map[string][]time.Time{}
	}
	cutoff := e.At.Add(-tdosWindow)
	t.starts = append(trimBefore(t.starts, cutoff), e.At)
	if c, ok := s.Calls[e.Call]; ok && c.Callback != "" {
		t.by[c.Callback] = append(trimBefore(t.by[c.Callback], e.At.Add(-tdosRepeatSpan)), e.At)
	}
	for k, v := range t.by {
		if v = trimBefore(v, e.At.Add(-tdosRepeatSpan)); len(v) == 0 {
			delete(t.by, k)
		} else {
			t.by[k] = v
		}
	}
	s.tdosEvaluate(e.At)
}

func (s *State) tdosEnded(e ledger.Event) {
	c, ok := s.Calls[e.Call]
	if !ok || c.Incident != "" || len(c.Text) > 0 || len(c.Transcript) > 0 || e.At.Sub(c.Start) > tdosShortCall {
		return
	}
	s.tdos.short = append(trimBefore(s.tdos.short, e.At.Add(-tdosShortSpan)), e.At)
	s.tdosEvaluate(e.At)
}

func trimBefore(v []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(v) && v[i].Before(cutoff) {
		i++
	}
	return v[i:]
}

func (s *State) tdosEvaluate(now time.Time) {
	t := &s.tdos
	perMin := 0
	for _, at := range t.starts {
		if now.Sub(at) <= time.Minute {
			perMin++
		}
	}
	older := len(t.starts) - perMin
	baseline := float64(older) / 59
	limit := s.Settings.TDoSPerMinute
	if limit <= 0 {
		limit = max(12, int(baseline*5+0.5))
	}
	var reasons, repeats []string
	if perMin >= limit {
		reasons = append(reasons, fmt.Sprintf("%d calls in the last minute (usual %.1f a minute, warning at %d)", perMin, baseline, limit))
	}
	for k, v := range t.by {
		if len(v) >= 5 {
			repeats = append(repeats, k)
		}
	}
	sort.Strings(repeats)
	if len(repeats) > 0 {
		reasons = append(reasons, fmt.Sprintf("%s called %d times or more in 10 minutes", strings.Join(repeats, ", "), 5))
	}
	if n := len(trimBefore(t.short, now.Add(-tdosShortSpan))); n >= 8 {
		reasons = append(reasons, fmt.Sprintf("%d silent or very short calls in 5 minutes", n))
	}
	td := &s.TDoS
	td.PerMin, td.Baseline = perMin, baseline
	if len(reasons) > 0 {
		if !td.Active {
			*td = TDoS{Active: true, Since: now, PerMin: perMin, Baseline: baseline}
		}
		td.Reasons, td.Repeats, td.Last = reasons, repeats, now
		return
	}
	if td.Active && now.Sub(td.Last) >= tdosQuietPeriod {
		*td = TDoS{PerMin: perMin, Baseline: baseline}
	}
}
