package cad

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// Actor is who does something: a signed-in person at a position, or a system (bitvoice, ali, radio).
type Actor struct {
	User    string
	Role    string
	Session string
	Console string
	Duty    string // the work the person signed in to do: calltaker, police, fire, ems, dispatcher, supervisor, admin
	Agency  string // the agency the person is working for (agency.go), when agencies are configured
}

// System actors.
var (
	SystemActor   = Actor{User: "system"}
	BitVoiceActor = Actor{User: "bitvoice"}
	ALIActor      = Actor{User: "ali"}
	RadioActor    = Actor{User: "radio"}
)

// Problem is a command refused, with what to fix, for the dispatcher to read.
type Problem struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

func (p *Problem) Error() string { return p.Message }

func problem(field, format string, a ...any) error {
	return &Problem{Field: field, Message: fmt.Sprintf(format, a...)}
}

// Notice tells subscribers the state changed.
type Notice struct {
	Seq   uint64 `json:"seq"`
	Type  string `json:"type"`
	Inc   string `json:"incident,omitempty"`
	Unit  string `json:"unit,omitempty"`
	Call  string `json:"call,omitempty"`
	Actor string `json:"actor"`
}

// Engine is the dispatch core.
type Engine struct {
	mu   sync.Mutex
	led  *ledger.Ledger
	path string
	st   *State
	now  func() time.Time
	// lastPos limits how often a unit's AVL position is written to the ledger.
	lastPos  map[string]Position
	router   Router
	geocoder Geocoder

	// Several sites (site.go): this site's name and incident number prefix, the other sites' ledgers, and
	// the newest event time applied.
	site       string
	sitePrefix string
	replicas   map[string]*ledger.Replica
	remote     []ledger.Event
	lastAt     time.Time

	subMu sync.Mutex
	subs  map[chan Notice]struct{}

	// pageOut are pages written but not yet delivered, with each person's answer token (kept only here,
	// never in the ledger): the server takes them to deliver (TakePage).
	pageOut map[string][]PageRecipient
	// alertOut are public alerts issued here and not yet delivered.
	alertOut map[string]bool
}

// Open opens the ledger at path, replays it and seeds a new system with default settings and call types.
func Open(path string) (*Engine, error) {
	led, events, err := ledger.Open(path)
	if err != nil {
		return nil, err
	}
	e := &Engine{led: led, path: path, st: newState(), now: time.Now, lastPos: map[string]Position{}, subs: map[chan Notice]struct{}{}}
	e.st.clock = func() time.Time { return e.now() }
	for _, ev := range events {
		if err := e.st.apply(ev); err != nil {
			led.Close()
			return nil, fmt.Errorf("replaying event %d (%s): %w", ev.Seq, ev.Type, err)
		}
	}
	if n := len(events); n > 0 {
		e.lastAt = events[n-1].At
	}
	if len(events) == 0 {
		if err := e.seed(); err != nil {
			led.Close()
			return nil, err
		}
	}
	return e, nil
}

func (e *Engine) seed() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, err := e.write(SystemActor, EvSettings, refs{}, DefaultSettings()); err != nil {
		return err
	}
	for _, ct := range DefaultCallTypes() {
		if _, err := e.write(SystemActor, EvCallTypeSaved, refs{}, ct); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the ledger.
func (e *Engine) Close() error { return e.led.Close() }

// LedgerPath is the ledger file.
func (e *Engine) LedgerPath() string { return e.path }

type refs struct {
	Incident, Unit, Call, Parent string
}

// write appends and applies one event. The caller holds e.mu.
func (e *Engine) write(a Actor, typ string, r refs, data any) (ledger.Event, error) {
	var raw json.RawMessage
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return ledger.Event{}, err
		}
		raw = b
	}
	ev, err := e.led.Append(ledger.Event{Type: typ, Actor: a.User, Role: a.Role, Session: a.Session, Console: a.Console, Duty: a.Duty, Agency: a.Agency,
		Incident: r.Incident, Unit: r.Unit, Call: r.Call, Parent: r.Parent, Data: raw, At: e.now(), Site: e.site})
	if err != nil {
		return ledger.Event{}, err
	}
	if ev.At.After(e.lastAt) {
		e.lastAt = ev.At
	}
	if err := e.st.apply(ev); err != nil {
		return ev, err
	}
	e.notify(Notice{Seq: ev.Seq, Type: typ, Inc: r.Incident, Unit: r.Unit, Call: r.Call, Actor: a.User})
	return ev, nil
}

// Subscribe returns a channel of notices. A slow subscriber loses notices rather than slowing dispatch;
// the stream tells it to reload.
func (e *Engine) Subscribe() chan Notice {
	ch := make(chan Notice, 256)
	e.subMu.Lock()
	e.subs[ch] = struct{}{}
	e.subMu.Unlock()
	return ch
}

// Unsubscribe ends a subscription.
func (e *Engine) Unsubscribe(ch chan Notice) {
	e.subMu.Lock()
	delete(e.subs, ch)
	e.subMu.Unlock()
}

func (e *Engine) notify(n Notice) {
	e.subMu.Lock()
	defer e.subMu.Unlock()
	for ch := range e.subs {
		select {
		case ch <- n:
		default:
		}
	}
}

// Read runs f with the state locked, for reads.
func (e *Engine) Read(f func(*State)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	f(e.st)
}

// Events returns ledger events after seq, at most limit, by re-reading the ledger file.
func (e *Engine) Events(after uint64, limit int, match func(ledger.Event) bool) ([]ledger.Event, error) {
	events, err := e.allEvents()
	if err != nil {
		return nil, err
	}
	out := []ledger.Event{}
	for _, ev := range events {
		if ev.Seq <= after || (match != nil && !match(ev)) {
			continue
		}
		out = append(out, ev)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// ---- configuration ----

var reID = regexp.MustCompile(`^[A-Z0-9][A-Z0-9-]{0,15}$`)

// SaveSettings replaces the settings.
func (e *Engine) SaveSettings(a Actor, s Settings) error {
	for _, p := range s.Commands.KeyProfiles {
		if c := KeyConflicts(p); len(c) > 0 {
			return problem("commands", "key profile %s: %s", firstNonEmpty(p.Name, p.ID), strings.Join(c, "; "))
		}
	}
	for _, r := range s.PriorityRules {
		if !slices.Contains(Priorities, r.Level) {
			return problem("priorityRules", "a priority is urgent, high, medium or low")
		}
	}
	if s.RoadFactor < 1 || s.RoadFactor > 3 {
		return problem("roadFactor", "road factor is between 1 and 3 (1.4 suits most road networks)")
	}
	if s.SpeedKmh < 10 || s.SpeedKmh > 150 {
		return problem("speedKmh", "average speed is between 10 and 150 km/h")
	}
	if s.AVLStaleSeconds < 10 {
		return problem("avlStaleSeconds", "AVL is stale after at least 10 seconds")
	}
	if s.LocStaleSeconds < 10 {
		return problem("locationStaleSeconds", "a caller location is stale after at least 10 seconds")
	}
	for k, v := range s.Timers {
		if !slices.Contains(Statuses, k) || v < 0 || v > 600 {
			return problem("timers", "timer %s must be a status and 0 to 600 minutes", k)
		}
	}
	for _, sh := range s.Shifts {
		if t, err := time.Parse("15:04", sh); err != nil || t.Format("15:04") != sh {
			return problem("shifts", "shift changes are times like 07:00")
		}
	}
	if len(s.SignInNotice) > 4000 {
		return problem("signInNotice", "the sign-in notice is up to 4000 characters")
	}
	if s.AutoDispatchSeconds < 0 || s.AutoDispatchSeconds > 3600 {
		return problem("autoDispatchSeconds", "auto-dispatch waits 0 to 3600 seconds")
	}
	if len(s.Dispositions) == 0 {
		return problem("dispositions", "keep at least one disposition")
	}
	mo := map[string]bool{}
	for i, t := range s.MarkOuts {
		t.Code = strings.ToUpper(strings.TrimSpace(t.Code))
		s.MarkOuts[i].Code = t.Code
		if t.Code == "" || strings.ContainsAny(t.Code, " #") || mo[t.Code] {
			return problem("markOuts", "each mark-out needs its own one-word code")
		}
		if _, isStatus := statusWords[t.Code]; isStatus {
			return problem("markOuts", "mark-out %s is already a status command", t.Code)
		}
		if strings.TrimSpace(t.Name) == "" || t.Minutes < 0 || t.Minutes > 600 {
			return problem("markOuts", "mark-out %s needs a name and 0 to 600 minutes", t.Code)
		}
		mo[t.Code] = true
	}
	codes := map[string]bool{}
	for _, t := range s.UnitTypes {
		if t.Code == "" || codes[t.Code] {
			return problem("unitTypes", "each unit type needs its own code")
		}
		codes[t.Code] = true
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := e.write(a, EvSettings, refs{}, s)
	return err
}

// SaveStation adds or changes a station.
func (e *Engine) SaveStation(a Actor, s Station) error {
	s.ID = strings.ToUpper(strings.TrimSpace(s.ID))
	if !reID.MatchString(s.ID) {
		return problem("id", "a station ID is 1 to 16 letters, digits or dashes")
	}
	if strings.TrimSpace(s.Name) == "" {
		return problem("name", "give the station a name")
	}
	if s.Lat < -90 || s.Lat > 90 || s.Lon < -180 || s.Lon > 180 || (s.Lat == 0 && s.Lon == 0) {
		return problem("lat", "place the station on the map")
	}
	s.Agency = strings.ToUpper(strings.TrimSpace(s.Agency))
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.checkAgency("agency", s.Agency); err != nil {
		return err
	}
	_, err := e.write(a, EvStationSaved, refs{}, s)
	return err
}

// RemoveStation removes a station no unit uses.
func (e *Engine) RemoveStation(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.st.Stations[id]; !ok {
		return problem("id", "no station %s", id)
	}
	for _, u := range e.st.Units {
		if u.Station == id {
			return problem("id", "unit %s is based at %s; move it first", u.ID, id)
		}
	}
	_, err := e.write(a, EvStationRemoved, refs{}, idData{id})
	return err
}

// SaveUnit adds or changes a unit's configuration. A new unit starts out of service.
func (e *Engine) SaveUnit(a Actor, u UnitDef) error {
	u.ID = strings.ToUpper(strings.TrimSpace(u.ID))
	if !reID.MatchString(u.ID) {
		return problem("id", "a call sign is 1 to 16 letters, digits or dashes")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.unitType(u.Type) == nil {
		return problem("type", "choose a unit type")
	}
	if len(e.st.Agencies) > 0 {
		u.Agency = strings.ToUpper(strings.TrimSpace(u.Agency))
		if err := e.checkAgency("agency", u.Agency); err != nil {
			return err
		}
	}
	if u.Station != "" {
		if _, ok := e.st.Stations[u.Station]; !ok {
			return problem("station", "no station %s", u.Station)
		}
	}
	for id, other := range e.st.Units {
		if id == u.ID {
			continue
		}
		if u.RadioID != 0 && other.RadioID == u.RadioID {
			return problem("radioId", "radio %d is already %s", u.RadioID, id)
		}
	}
	_, err := e.write(a, EvUnitSaved, refs{Unit: u.ID}, u)
	return err
}

// RemoveUnit removes a unit that is not committed to an incident.
func (e *Engine) RemoveUnit(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	u, ok := e.st.Units[id]
	if !ok {
		return problem("id", "no unit %s", id)
	}
	if Committed(u.Status) {
		return problem("id", "%s is on %s; clear it first", id, u.Incident)
	}
	_, err := e.write(a, EvUnitRemoved, refs{Unit: id}, idData{id})
	return err
}

// SaveCallType adds or changes a call type and its run card.
func (e *Engine) SaveCallType(a Actor, c CallType) error {
	c.Code = strings.ToUpper(strings.TrimSpace(c.Code))
	if !reID.MatchString(c.Code) {
		return problem("code", "a call type code is 1 to 16 letters, digits or dashes")
	}
	if strings.TrimSpace(c.Name) == "" {
		return problem("name", "give the call type a name")
	}
	if c.Priority < 1 || c.Priority > 5 {
		return problem("priority", "priority is 1 (highest) to 5")
	}
	if c.AutoDispatch && c.Priority < 3 {
		return problem("autoDispatch", "only priority 3 to 5 call types are sent automatically")
	}
	for i, r := range c.RunCard {
		if r.Capability == "" || r.Count < 1 || r.Count > 20 {
			return problem(fmt.Sprintf("runCard[%d]", i), "each run card line needs a capability and 1 to 20 units")
		}
	}
	if err := checkAlarms(c); err != nil {
		return err
	}
	if c.PARMinutes < 0 || c.PARMinutes > 60 {
		return problem("parMinutes", "a roll-call interval is 0 (the setting) to 60 minutes")
	}
	for i, g := range c.Agencies {
		c.Agencies[i] = strings.ToUpper(strings.TrimSpace(g))
	}
	c.Agencies = cleanList(c.Agencies)
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, g := range c.Agencies {
		if err := e.checkAgency("agencies", g); err != nil {
			return err
		}
	}
	_, err := e.write(a, EvCallTypeSaved, refs{}, c)
	return err
}

// RemoveCallType removes a call type. Incidents of that type keep their code.
func (e *Engine) RemoveCallType(a Actor, code string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.st.CallTypes[code]; !ok {
		return problem("code", "no call type %s", code)
	}
	_, err := e.write(a, EvCallTypeRemoved, refs{}, idData{code})
	return err
}

// Record writes an event that changes no dispatch state: sign-ins, user and integration changes.
func (e *Engine) Record(a Actor, typ string, data any) error {
	switch typ {
	case EvUserSaved, EvUserRemoved, EvIntegration, EvSignIn, EvSignInFailed, EvSignOut, EvMapRecording, EvMapRecorded, EvMapFailed,
		EvPhotosRecording, EvPhotosRecorded, EvPhotosFailed,
		EvAdvisorySuggested, EvAdvisoryDecided, EvAdvisoryWithdrawn, EvGISImporting, EvGISImported, EvGISFailed, EvJusticeQuery, EvIBRExported,
		EvLocked, EvUnlocked, EvReviewExported, EvConfigStaged, EvWorkstationCode, EvUserDuties, EvUserMFAReset, EvRecordsViewed, EvTrainingInject,
		EvAccessRefused, EvSearched, EvSearchOpened, EvCommandRefused:
	default:
		return fmt.Errorf("cad: %s is not a record event", typ)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := e.write(a, typ, refs{}, data)
	return err
}

func (e *Engine) unitType(code string) *UnitType {
	for i := range e.st.Settings.UnitTypes {
		if e.st.Settings.UnitTypes[i].Code == code {
			return &e.st.Settings.UnitTypes[i]
		}
	}
	return nil
}

// Capabilities of a unit: its type's and its own, less any its recorded crew cannot staff (crew.go).
func (s *State) Capabilities(u *Unit) []string {
	out := s.baseCapabilities(u)
	if drop := s.crewDrops(u); len(drop) > 0 {
		out = slices.DeleteFunc(out, func(c string) bool { return slices.Contains(drop, c) })
	}
	return out
}

// baseCapabilities are a unit's type's capabilities and its own, whoever is aboard.
func (s *State) baseCapabilities(u *Unit) []string {
	var out []string
	for _, t := range s.Settings.UnitTypes {
		if t.Code == u.Type {
			out = append(out, t.Capabilities...)
		}
	}
	for _, c := range u.Capabilities {
		if !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// ---- calls ----

// UpsertCall records what a source says about a call: a new call, or a change to one already known.
// It writes nothing when nothing a dispatcher sees has changed, so a chatty source does not flood the ledger.
func (e *Engine) UpsertCall(a Actor, id string, d CallData) error {
	if id == "" {
		return errors.New("cad: a call needs an ID")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	c, known := e.st.Calls[id]
	if known && !changes(c, d) {
		return nil
	}
	typ := EvCallReceived
	if known {
		typ = EvCallUpdated
	}
	ev, err := e.write(a, typ, refs{Call: id}, d)
	if err != nil {
		return err
	}
	// A newer caller location (a rebid, a supplemental provider) is offered to the incident the call is on;
	// the call-taker decides whether it becomes the incident's.
	if number, offer := e.st.offerLocation(ev); offer != nil {
		_, err = e.write(a, EvIncLocOffered, refs{Incident: number, Call: id}, offer)
	}
	return err
}

func changes(c *Call, d CallData) bool {
	if handlingChanges(c, d.Handling) {
		return true
	}
	if d.Source != "" && !slices.Contains(c.Sources, d.Source) {
		return true
	}
	if (d.State != "" && d.State != c.State && c.State != "ended") || (d.Callback != "" && d.Callback != c.Callback) ||
		(d.CallerName != "" && d.CallerName != c.CallerName) || (d.Class != "" && d.Class != c.Class) ||
		(d.Agent != "" && d.Agent != c.Agent) || (!d.Answer.IsZero() && !d.Answer.Equal(c.Answer)) {
		return true
	}
	if d.Location != nil && (d.Location.HasPoint() || d.Location.Civic != nil) && (c.Location == nil || !sameLocation(c.Location, d.Location) || (!d.Location.At.IsZero() && !d.Location.At.Equal(c.Location.At))) {
		return true
	}
	// Text compared line by line: a TTY or real-time text line grows a character at a time.
	if len(d.Media) != len(c.Media) && len(d.Media) > 0 || len(d.Text) > 0 && !sameText(d.Text, c.Text) || len(d.Notes) > len(c.Notes) {
		return true
	}
	if d.Video != nil && (c.Video == nil || *d.Video != *c.Video) {
		return true
	}
	if d.CanRebid && !c.CanRebid || d.TextOnly && !c.TextOnly {
		return true
	}
	if d.Problems != nil && !slices.Equal(d.Problems, c.Problems) {
		return true
	}
	if len(d.Transcript) > 0 || len(d.Translations) > 0 || d.Crash != nil || d.Alert != nil || d.Line != "" && d.Line != c.Line {
		return true
	}
	if d.Location != nil && d.Location.Altitude != nil && (c.Location == nil || c.Location.Altitude == nil || *c.Location.Altitude != *d.Location.Altitude) {
		return true
	}
	return false
}

func sameText(a, b []TextLine) bool {
	return slices.EqualFunc(a, b, func(x, y TextLine) bool {
		return x.Text == y.Text && x.From == y.From && x.Via == y.Via && x.Name == y.Name && x.At.Equal(y.At)
	})
}

// EndCall records a call's end.
func (e *Engine) EndCall(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	c, ok := e.st.Calls[id]
	if !ok || c.State == "ended" {
		return nil
	}
	_, err := e.write(a, EvCallEnded, refs{Call: id}, nil)
	return err
}

// AttachCall links a call to an incident.
func (e *Engine) AttachCall(a Actor, callID, number string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	c, ok := e.st.Calls[callID]
	if !ok {
		return problem("call", "no call %s", callID)
	}
	if _, ok := e.st.Incidents[number]; !ok {
		return problem("incident", "no incident %s", number)
	}
	if c.Incident == number {
		return nil
	}
	if _, err := e.write(a, EvCallAttached, refs{Call: callID, Incident: number}, nil); err != nil {
		return err
	}
	if c.State == "data" {
		// A data-only call (a vehicle's crash notification) is done once it is on an incident.
		_, err := e.write(a, EvCallEnded, refs{Call: callID}, nil)
		return err
	}
	return nil
}

// RecordCallAction records something a dispatcher asked call handling to do for a call, and how it went.
func (e *Engine) RecordCallAction(a Actor, callID, action string, ok bool, detail string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	inc := ""
	if c, found := e.st.Calls[callID]; found {
		inc = c.Incident
	}
	_, err := e.write(a, EvCallAction, refs{Call: callID, Incident: inc}, actionData{Action: action, OK: ok, Detail: detail})
	return err
}

// ---- incidents ----

// NewIncident is what a dispatcher gives to create an incident.
type NewIncident struct {
	Modifiers   []string        `json:"modifiers,omitempty"`
	CallerAt    *ng911.Location `json:"callerAt,omitempty"` // where the caller is, when not at the incident
	LocVerified bool            `json:"locVerified,omitempty"`
	Type        string          `json:"type"`
	Priority    int             `json:"priority,omitempty"` // 0: the call type's
	Location    *ng911.Location `json:"location,omitempty"`
	Caller      *Caller         `json:"caller,omitempty"`
	Narrative   string          `json:"narrative,omitempty"`
	Call        string          `json:"call,omitempty"`   // create from this call
	Agency      string          `json:"agency,omitempty"` // the primary agency (empty: decided by agency.go)
}

// CreateIncident creates an incident. From a call, it takes the call's location and caller unless given.
func (e *Engine) CreateIncident(a Actor, n NewIncident) (string, error) {
	n.Type = strings.ToUpper(strings.TrimSpace(n.Type))
	e.mu.Lock()
	defer e.mu.Unlock()
	ct, ok := e.st.CallTypes[n.Type]
	if !ok {
		return "", problem("type", "choose a call type")
	}
	if n.Priority == 0 {
		n.Priority = ct.Priority
	}
	if n.Priority < 1 || n.Priority > 5 {
		return "", problem("priority", "priority is 1 (highest) to 5")
	}
	if n.Call != "" {
		c, ok := e.st.Calls[n.Call]
		if !ok {
			return "", problem("call", "no call %s", n.Call)
		}
		if c.Incident != "" {
			return "", problem("call", "that call is already on %s", c.Incident)
		}
		if n.Location == nil && c.Location != nil {
			loc := *c.Location
			n.Location = &loc
		}
		if n.Caller == nil {
			n.Caller = &Caller{Name: c.CallerName, Callback: c.Callback}
		}
	}
	if n.Location != nil && !n.Location.HasPoint() && strings.TrimSpace(n.Location.Address) == "" {
		n.Location = nil
	}
	if n.Location != nil && n.Location.Source == "" {
		n.Location.Source = "dispatcher"
	}
	now := e.now()
	agency, err := e.incidentAgency(a, n)
	if err != nil {
		return "", err
	}
	if ct := e.st.CallTypes[n.Type]; ct != nil && agency != "" && !ct.For(agency) {
		return "", problem("type", "%s is not a call type of %s", n.Type, agency)
	}
	// With several sites, each numbers its own incidents under its own prefix, so two sites never give
	// the same number; an agency with its own prefix numbers its incidents under it.
	prefix := e.st.incidentPrefix(agency)
	seqNo := e.st.yearCount[fmt.Sprintf("%s|%d", e.site, now.UTC().Year())] + 1
	number := fmt.Sprintf("%s%s%02d-%06d", prefix, e.sitePrefix, now.UTC().Year()%100, seqNo)
	for e.st.Incidents[number] != nil { // a prefix change or clock change could repeat a number
		seqNo++
		number = fmt.Sprintf("%s%s%02d-%06d", prefix, e.sitePrefix, now.UTC().Year()%100, seqNo)
	}
	_, err = e.write(a, EvIncCreated, refs{Incident: number, Call: n.Call}, incidentCreated{Number: number, Type: n.Type, Priority: n.Priority,
		Location: n.Location, Caller: n.Caller, Narrative: strings.TrimSpace(n.Narrative), Call: n.Call,
		Modifiers: n.Modifiers, CallerAt: n.CallerAt, LocVerified: n.LocVerified, Agency: agency})
	if err == nil && n.Call != "" {
		if c := e.st.Calls[n.Call]; c != nil && c.State == "data" {
			_, err = e.write(a, EvCallEnded, refs{Call: n.Call}, nil) // a crash notification, now on an incident
		}
	}
	if err == nil {
		err = e.modifierEffects(a, e.st.Incidents[number], n.Modifiers)
	}
	return number, err
}

// IncidentChange is a change to an incident's type, priority, location or caller.
type IncidentChange struct {
	Type     *string         `json:"type,omitempty"`
	Priority *int            `json:"priority,omitempty"`
	Location *ng911.Location `json:"location,omitempty"`
	Caller   *Caller         `json:"caller,omitempty"`
	Reason   string          `json:"reason,omitempty"`
}

// ChangeIncident changes an open incident.
func (e *Engine) ChangeIncident(a Actor, number string, c IncidentChange) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	in, err := e.openIncident(number)
	if err != nil {
		return err
	}
	if c.Type != nil {
		t := strings.ToUpper(*c.Type)
		if _, ok := e.st.CallTypes[t]; !ok {
			return problem("type", "no call type %s", t)
		}
		if t == in.Type {
			c.Type = nil
		} else {
			c.Type = &t
		}
	}
	if c.Priority != nil {
		if *c.Priority < 1 || *c.Priority > 5 {
			return problem("priority", "priority is 1 (highest) to 5")
		}
		if *c.Priority == in.Priority {
			c.Priority = nil
		}
	}
	if c.Location != nil && c.Location.Source == "" {
		c.Location.Source = "dispatcher"
	}
	if c.Type == nil && c.Priority == nil && c.Location == nil && c.Caller == nil {
		return nil
	}
	_, err = e.write(a, EvIncChanged, refs{Incident: number}, incidentChanged(c))
	return err
}

func (e *Engine) openIncident(number string) (*Incident, error) {
	in, ok := e.st.Incidents[number]
	if !ok {
		return nil, problem("incident", "no incident %s", number)
	}
	if in.Status == Closed {
		return nil, problem("incident", "%s is closed; reopen it first", number)
	}
	return in, nil
}

// AddNote adds a narrative note. A priority note is one every dispatcher and unit must see.
func (e *Engine) AddNote(a Actor, number, text string, priority bool) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return problem("text", "write the note")
	}
	if len(text) > 4000 {
		return problem("text", "a note is at most 4000 characters")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.st.Incidents[number]; !ok {
		return problem("incident", "no incident %s", number)
	}
	// Notes are allowed on closed incidents: late information is a linked late entry, never lost.
	_, err := e.write(a, EvIncNote, refs{Incident: number}, noteData{Text: text, Priority: priority})
	return err
}

// Dispatch assigns units to an incident. recommended is what the recommendation showed, so the ledger
// records where the dispatcher chose differently. A unit on another incident is reassigned.
func (e *Engine) Dispatch(a Actor, number string, units, recommended []string) error {
	return e.DispatchWhy(a, number, units, recommended, "", "")
}

// DispatchWhy dispatches units, recording why they differ from the recommendation and the response plan it
// followed. The duty the dispatcher signed in to do limits which units they may send.
func (e *Engine) DispatchWhy(a Actor, number string, units, recommended []string, reason, plan string) error {
	return e.DispatchOverride(a, number, units, recommended, reason, plan, "")
}

// DispatchOverride dispatches as DispatchWhy, with a supervisor's reason (override) for sending units short
// of their staffing where the agency asks for one (StaffingRules.Dispatch).
func (e *Engine) DispatchOverride(a Actor, number string, units, recommended []string, reason, plan, override string) error {
	if len(units) == 0 {
		return problem("units", "choose at least one unit")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, err := e.openIncident(number); err != nil {
		return err
	}
	var ids []string
	for _, id := range units {
		id = strings.ToUpper(id)
		u, ok := e.st.Units[id]
		if !ok {
			return problem("units", "no unit %s", id)
		}
		if u.Incident == number && Committed(u.Status) {
			return problem("units", "%s is already on %s", id, number)
		}
		if err := e.st.mayCommand(a, u); err != nil {
			return err
		}
		if short := e.st.Staffing(u); len(short) > 0 && u.StaffOverride == "" {
			switch e.st.Settings.Staffing.Dispatch {
			case "block":
				return problem("units", "%s is short-staffed (%s): the agency does not send short units", id, strings.Join(short, "; "))
			case "override":
				if strings.TrimSpace(override) == "" || a.Role != "supervisor" && a.Role != "admin" {
					return problem("override", "%s is short-staffed (%s): a supervisor's override and reason are needed to send it", id, strings.Join(short, "; "))
				}
			}
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	override = strings.TrimSpace(override)
	parent, err := e.write(a, EvIncDispatch, refs{Incident: number}, dispatchData{Units: ids, Recommended: recommended, Reason: strings.TrimSpace(reason), Plan: plan})
	if err != nil {
		return err
	}
	for _, id := range ids {
		u := e.st.Units[id]
		d := statusData{Status: Dispatched, Prev: u.Status, Incident: number, PrevIncident: u.Incident, Recommended: slices.Contains(recommended, id)}
		if u.Status == OutOfService {
			d.Reason = "was out of service"
		}
		if short := e.st.Staffing(u); len(short) > 0 && override != "" {
			d.Override = override
			d.Reason = strings.TrimSpace(d.Reason + " sent short-staffed (" + strings.Join(short, "; ") + ") by override: " + override)
		}
		if _, err := e.write(a, EvUnitStatus, refs{Incident: number, Unit: id, Parent: parent.ID}, d); err != nil {
			return err
		}
	}
	return nil
}

// SetStatus changes a unit's status. A committed status keeps the unit's incident unless one is given;
// available and out of service clear it from its incident.
func (e *Engine) SetStatus(a Actor, unit, status, number, reason string) error {
	unit = strings.ToUpper(unit)
	if !slices.Contains(Statuses, status) {
		return problem("status", "no status %s", status)
	}
	if status == MarkedOut {
		return problem("status", "a mark-out needs its kind: MO %s MEAL", unit)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	u, ok := e.st.Units[unit]
	if !ok {
		return problem("unit", "no unit %s", unit)
	}
	if err := e.st.mayCommand(a, u); err != nil {
		return err
	}
	inc := u.Incident
	if number != "" {
		if _, err := e.openIncident(number); err != nil {
			return err
		}
		inc = number
	}
	if !Committed(status) {
		inc = ""
	} else if inc == "" {
		return problem("unit", "%s has no incident: dispatch it to one first", unit)
	}
	if status == u.Status && inc == u.Incident {
		return nil
	}
	_, err := e.write(a, EvUnitStatus, refs{Incident: firstNonEmpty(inc, u.Incident), Unit: unit},
		statusData{Status: status, Prev: u.Status, Incident: inc, PrevIncident: u.Incident, Reason: strings.TrimSpace(reason)})
	return err
}

// MarkOutUnit marks a unit out on an activity of its own (a mark-out type), with where and a note. A unit
// on an incident leaves it. Marking out again changes the activity and restarts its timer.
func (e *Engine) MarkOutUnit(a Actor, unit, code string, loc *ng911.Location, note string) error {
	unit = strings.ToUpper(unit)
	e.mu.Lock()
	defer e.mu.Unlock()
	u, ok := e.st.Units[unit]
	if !ok {
		return problem("unit", "no unit %s", unit)
	}
	if err := e.st.mayCommand(a, u); err != nil {
		return err
	}
	t, ok := e.st.Settings.MarkOutType(code)
	if !ok {
		var codes []string
		for _, x := range e.st.Settings.MarkOuts {
			codes = append(codes, x.Code)
		}
		return problem("markOut", "no mark-out %s; the agency's are %s (Admin › Settings)", strings.ToUpper(code), strings.Join(codes, " "))
	}
	if t.Retired {
		return problem("retired", "%s (%s) is a retired mark-out code", t.Code, t.Name)
	}
	if len(t.Roles) > 0 && !slices.Contains(t.Roles, a.Role) && a.Role != "admin" {
		return problem("role", "your role (%s) may not use the %s mark-out", a.Role, t.Code)
	}
	if t.Discipline != "" {
		if d := e.st.Discipline(u); d != t.Discipline {
			return problem("markOut", "%s (%s) is for %s units; %s is %s", t.Code, t.Name, t.Discipline, unit, d)
		}
	}
	if loc != nil && loc.Address == "" && !loc.HasPoint() {
		loc = nil
	}
	_, err := e.write(a, EvUnitStatus, refs{Incident: u.Incident, Unit: unit},
		statusData{Status: MarkedOut, Prev: u.Status, PrevIncident: u.Incident, MarkOut: t.Code, Location: loc, Reason: strings.TrimSpace(note)})
	return err
}

// CheckUnit records a status check: the dispatcher heard from the unit and it is all right. Its timer
// starts again.
func (e *Engine) CheckUnit(a Actor, unit, note string) error {
	unit = strings.ToUpper(unit)
	e.mu.Lock()
	defer e.mu.Unlock()
	u, ok := e.st.Units[unit]
	if !ok {
		return problem("unit", "no unit %s", unit)
	}
	_, err := e.write(a, EvUnitChecked, refs{Incident: u.Incident, Unit: unit}, checkData{Status: u.Status, Note: strings.TrimSpace(note)})
	return err
}

// CloseIncident closes an incident. Units still on it are cleared to available.
func (e *Engine) CloseIncident(a Actor, number, disposition, text string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	in, err := e.openIncident(number)
	if err != nil {
		return err
	}
	if !slices.Contains(e.st.Settings.Dispositions, disposition) {
		return problem("disposition", "choose a disposition")
	}
	for _, as := range in.Units {
		if !as.Cleared.IsZero() {
			continue
		}
		u, ok := e.st.Units[as.Unit]
		if !ok || u.Incident != number {
			continue
		}
		if _, err := e.write(a, EvUnitStatus, refs{Incident: number, Unit: u.ID}, statusData{Status: Available, Prev: u.Status, PrevIncident: number, Reason: "incident closed"}); err != nil {
			return err
		}
	}
	_, err = e.write(a, EvIncClosed, refs{Incident: number}, closeData{Disposition: disposition, Text: strings.TrimSpace(text)})
	return err
}

// ReopenIncident reopens a closed incident.
func (e *Engine) ReopenIncident(a Actor, number, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return problem("reason", "say why the incident is reopened")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	in, ok := e.st.Incidents[number]
	if !ok {
		return problem("incident", "no incident %s", number)
	}
	if in.Status != Closed {
		return problem("incident", "%s is not closed", number)
	}
	_, err := e.write(a, EvIncReopened, refs{Incident: number}, reasonData{Reason: strings.TrimSpace(reason)})
	return err
}

// ---- units in the field ----

// UnitPosition records a unit's position. The live position always updates; the ledger gets one entry
// per unit per 30 seconds (sooner after a jump of a kilometre), which is enough to reconstruct a response.
func (e *Engine) UnitPosition(a Actor, unit string, p Position) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	u, ok := e.st.Units[unit]
	if !ok {
		return problem("unit", "no unit %s", unit)
	}
	if p.At.IsZero() {
		p.At = e.now()
	}
	last, seen := e.lastPos[unit]
	if seen && p.At.Sub(last.At) < 30*time.Second && distance(last, p) < 1000 {
		u.Position = &p
		e.notify(Notice{Type: EvUnitLocation, Unit: unit, Actor: a.User})
		return nil
	}
	e.lastPos[unit] = p
	_, err := e.write(a, EvUnitLocation, refs{Unit: unit}, p)
	return err
}

// Heard records a radio transmission from a unit (not written to the ledger: call handling keeps the
// radio log; bitdispatcher needs only the time, for loss-of-contact checks).
func (e *Engine) Heard(unit string, at time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if u, ok := e.st.Units[unit]; ok && at.After(u.LastHeard) {
		u.LastHeard = at
		e.notify(Notice{Type: "unit.heard", Unit: unit, Actor: "radio"})
	}
}

// UnitByRadio finds the unit with a radio ID or alias.
func (e *Engine) UnitByRadio(id int, alias string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, u := range e.st.Units {
		if (id != 0 && u.RadioID == id) || (alias != "" && strings.EqualFold(u.RadioAlias, alias)) {
			return u.ID
		}
	}
	return ""
}

// RaiseEmergency opens an emergency, unless one for the same radio is already open.
func (e *Engine) RaiseEmergency(a Actor, em Emergency) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, open := range e.st.Emergencies {
		if open.AckedAt.IsZero() && ((em.RadioUnit != "" && open.RadioUnit == em.RadioUnit) || (em.Unit != "" && open.Unit == em.Unit)) {
			return open.Key, nil
		}
	}
	if em.Key == "" {
		em.Key = ledger.NewID()
	}
	if em.Raised.IsZero() {
		em.Raised = e.now()
	}
	_, err := e.write(a, EvEmergency, refs{Unit: em.Unit, Incident: e.unitIncident(em.Unit)}, em)
	return em.Key, err
}

func (e *Engine) unitIncident(unit string) string {
	if u, ok := e.st.Units[unit]; ok {
		return u.Incident
	}
	return ""
}

// AckEmergency acknowledges an open emergency. where is bitdispatcher, or bitvoice when the radio console
// acknowledged it.
func (e *Engine) AckEmergency(a Actor, key, where string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	em, ok := e.st.Emergencies[key]
	if !ok {
		return problem("key", "no such emergency")
	}
	if !em.AckedAt.IsZero() {
		return nil
	}
	_, err := e.write(a, EvEmergencyAck, refs{Unit: em.Unit, Incident: e.unitIncident(em.Unit)}, Emergency{Key: key, AckedBy: a.User, AckedWhere: where})
	return err
}

// OpenEmergencyFor finds the open emergency of a radio unit or channel.
func (e *Engine) OpenEmergencyFor(radioUnit, channel string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var keys []*Emergency
	for _, em := range e.st.Emergencies {
		if em.AckedAt.IsZero() && ((radioUnit != "" && em.RadioUnit == radioUnit) || (radioUnit == "" && channel != "" && em.Channel == channel)) {
			keys = append(keys, em)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Raised.Before(keys[j].Raised) })
	if len(keys) == 0 {
		return ""
	}
	return keys[0].Key
}

// Now is the engine's clock.
func (e *Engine) Now() time.Time { return e.now() }
