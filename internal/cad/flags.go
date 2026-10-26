package cad

// Premise and safety flags: the agency's warnings about a verified address, scoped as narrowly as the
// facts allow (the whole property, a complex, a named business, a building, a floor, an apartment) or
// about a person. A flag appears as soon as the call-taker verifies the address, before the incident is
// sent, and the version shown is kept on the incident, so a later change, renewal or retirement never
// rewrites what dispatchers saw. Each category says who may create, approve and see it: medical,
// domestic-violence, officer-safety and keyholder detail is shown only to those the agency allows, and
// everyone else sees the operational warning (title, severity and what to do). Permanent flags are never
// put on an unverified, free-text address; flags near an unverified location are offered only as possible
// matches.

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvFlagSaved   = "flag.saved"   // a flag was proposed, added or changed: a new version
	EvFlagDecided = "flag.decided" // a flag was approved, rejected, retired, restored or renewed: a new version
	EvFlagViewed  = "flag.viewed"  // someone opened a restricted flag's detail
)

// Flag states.
const (
	FlagProposed = "proposed"
	FlagActive   = "active"
	FlagRetired  = "retired"
	FlagRejected = "rejected"
)

// FlagCategory is one of the agency's flag categories and who may do what with it.
type FlagCategory struct {
	Code       string   `json:"code"`
	Name       string   `json:"name"`
	Scope      string   `json:"scope,omitempty"` // address (default) or person or both
	Severity   string   `json:"severity"`        // default severity: critical, high, medium, low
	Restricted bool     `json:"restricted,omitempty"`
	Create     []string `json:"create,omitempty"`  // duties that may create (empty: call-takers, dispatchers, supervisors)
	Approve    []string `json:"approve,omitempty"` // duties whose flags are active at once and who approve others' (empty: supervisors)
	View       []string `json:"view,omitempty"`    // duties that see a restricted flag's detail (empty: supervisors and police)
	ReviewDays int      `json:"reviewDays,omitempty"`
	ExpireDays int      `json:"expireDays,omitempty"`
	Icon       string   `json:"icon,omitempty"`   // a lucide icon name
	Fields     []string `json:"fields,omitempty"` // extra details asked for (animal, capacity, election dates…)
	Off        bool     `json:"off,omitempty"`
}

// DefaultFlagCategories are the categories agencies start with.
func DefaultFlagCategories() []FlagCategory {
	r := []string{DutySupervisor, DutyPolice, DutyDispatcher}
	return []FlagCategory{
		{Code: "premise", Name: "Premise", Severity: "medium", Icon: "building", ReviewDays: 365},
		{Code: "location", Name: "Location", Severity: "medium", Icon: "map-pin", ReviewDays: 365},
		{Code: "hazard", Name: "Hazard", Severity: "high", Icon: "flame", ReviewDays: 365, Fields: []string{"hazard material", "location in structure"}},
		{Code: "officer-safety", Name: "Officer safety", Scope: "both", Severity: "critical", Restricted: true, View: r, Icon: "shield-alert", ReviewDays: 180, Fields: []string{"weapons", "threat"}},
		{Code: "medical", Name: "Medical", Scope: "both", Severity: "high", Restricted: true, View: []string{DutySupervisor, DutyEMS, DutyFire, DutyDispatcher}, Icon: "heart-pulse", ReviewDays: 180},
		{Code: "business", Name: "Business", Severity: "low", Icon: "store", ReviewDays: 365, Fields: []string{"hours", "alarm company"}},
		{Code: "keyholder", Name: "Keyholder or maintenance", Severity: "low", Restricted: true, View: []string{DutySupervisor, DutyDispatcher, DutyPolice, DutyFire, DutyEMS}, Icon: "key-round", ReviewDays: 180, Fields: []string{"contact", "phone", "availability"}},
		{Code: "unsafe-structure", Name: "Unsafe structure", Severity: "critical", Icon: "triangle-alert", ReviewDays: 90},
		{Code: "domestic-violence", Name: "Domestic violence", Scope: "both", Severity: "critical", Restricted: true, View: r, Icon: "house", ReviewDays: 180, ExpireDays: 365},
		{Code: "dangerous-animal", Name: "Dangerous animal", Severity: "high", Icon: "paw-print", ReviewDays: 180, Fields: []string{"animal", "number", "usual location", "containment"}},
		{Code: "polling-place", Name: "Polling place", Severity: "low", Icon: "vote", Fields: []string{"election date", "hours", "election contact"}},
		{Code: "evacuation", Name: "Evacuation location", Severity: "medium", Icon: "tent", Fields: []string{"site type", "activation", "capacity", "contact"}},
	}
}

// FlagPolicy is the agency's flag categories (empty: the defaults).
type FlagPolicy struct {
	Categories []FlagCategory `json:"categories,omitempty"`
	// NearMetres is how near an unverified location a flag is offered as a possible match (0: 80 m).
	NearMetres float64 `json:"nearMetres,omitempty"`
}

// CategoryList is the agency's flag categories.
func (p FlagPolicy) CategoryList() []FlagCategory {
	if len(p.Categories) == 0 {
		return DefaultFlagCategories()
	}
	return p.Categories
}

// Category finds a category by code.
func (p FlagPolicy) Category(code string) (FlagCategory, bool) {
	for _, c := range p.CategoryList() {
		if c.Code == code {
			return c, true
		}
	}
	return FlagCategory{}, false
}

func (c FlagCategory) mayCreate(a Actor) bool {
	if a.Role == "admin" {
		return true
	}
	d := c.Create
	if len(d) == 0 {
		d = []string{DutyCallTaker, DutyDispatcher, DutyPolice, DutyFire, DutyEMS, DutySupervisor, DutyRecords}
	}
	return slices.Contains(d, DutyOf(a))
}

func (c FlagCategory) mayApprove(a Actor) bool {
	if a.Role == "admin" {
		return true
	}
	d := c.Approve
	if len(d) == 0 {
		d = []string{DutySupervisor}
	}
	return slices.Contains(d, DutyOf(a))
}

// MayView says whether a duty sees a restricted flag's full detail.
func (c FlagCategory) MayView(duty, role string) bool {
	if !c.Restricted || role == "admin" {
		return true
	}
	d := c.View
	if len(d) == 0 {
		d = []string{DutySupervisor, DutyPolice}
	}
	return slices.Contains(d, duty)
}

// FlagScope is what a flag is about: an address and how much of it, or a person.
type FlagScope struct {
	AddrKey  string   `json:"addrKey,omitempty"`
	Address  string   `json:"address,omitempty"`
	Lat      *float64 `json:"lat,omitempty"`
	Lon      *float64 `json:"lon,omitempty"`
	Business string   `json:"business,omitempty"` // a named business or occupant at the address
	Complex  string   `json:"complex,omitempty"`
	Building string   `json:"building,omitempty"`
	Floor    string   `json:"floor,omitempty"`
	Unit     string   `json:"unit,omitempty"`
	// Person flags: the person's key (PersonKey) and how they are named.
	Person     string `json:"person,omitempty"`
	PersonName string `json:"personName,omitempty"`
	// Verified: the address came from the agency's address source (the server checks when it can).
	Verified bool `json:"verified,omitempty"`
}

// Label describes the scope: "the whole property", "Bldg 4, Apt 204".
func (sc FlagScope) Label() string {
	if sc.Person != "" {
		return "person: " + sc.PersonName
	}
	var parts []string
	for _, x := range [][2]string{{"", sc.Business}, {"", sc.Complex}, {"Bldg ", sc.Building}, {"Floor ", sc.Floor}, {"Unit ", sc.Unit}} {
		if x[1] != "" {
			parts = append(parts, x[0]+x[1])
		}
	}
	if len(parts) == 0 {
		return "the whole property"
	}
	return strings.Join(parts, ", ")
}

// Flag is one premise, location or safety flag.
type Flag struct {
	ID          string            `json:"id"`
	Version     int               `json:"version"`
	Category    string            `json:"category"`
	Title       string            `json:"title"`
	Warning     string            `json:"warning"`     // what the danger or information is, and why it applies
	Instruction string            `json:"instruction"` // what responders should do
	Severity    string            `json:"severity"`
	Source      string            `json:"source,omitempty"`      // how it was reported
	Reliability string            `json:"reliability,omitempty"` // verified, reported, unconfirmed
	Scope       FlagScope         `json:"scope"`
	Details     map[string]string `json:"details,omitempty"`
	State       string            `json:"state"`
	Incident    string            `json:"incident,omitempty"` // the incident it came from
	Creator     string            `json:"creator"`
	Created     time.Time         `json:"created"`
	Verifier    string            `json:"verifier,omitempty"`
	Verified    time.Time         `json:"verified,omitzero"`
	Effective   time.Time         `json:"effective,omitzero"`
	Review      time.Time         `json:"review,omitzero"`
	Expires     time.Time         `json:"expires,omitzero"`
	Changed     time.Time         `json:"changed"`
	ChangedBy   string            `json:"changedBy"`
	Note        string            `json:"note,omitempty"` // why this version
	History     []FlagVersion     `json:"history,omitempty"`
}

// FlagVersion is an earlier version of a flag.
type FlagVersion struct {
	Version     int       `json:"version"`
	Title       string    `json:"title"`
	Warning     string    `json:"warning"`
	Instruction string    `json:"instruction"`
	Severity    string    `json:"severity"`
	State       string    `json:"state"`
	Expires     time.Time `json:"expires,omitzero"`
	Changed     time.Time `json:"changed"`
	ChangedBy   string    `json:"changedBy"`
	Note        string    `json:"note,omitempty"`
}

// ActiveAt says the flag is in force at a time.
func (f *Flag) ActiveAt(t time.Time) bool {
	if f.State != FlagActive {
		return false
	}
	if !f.Effective.IsZero() && t.Before(f.Effective) {
		return false
	}
	return f.Expires.IsZero() || t.Before(f.Expires)
}

// FlagShown is a flag version an incident showed, kept so its history says what dispatchers saw.
type FlagShown struct {
	ID       string    `json:"id"`
	Version  int       `json:"version"`
	Category string    `json:"category"`
	Title    string    `json:"title"`
	Severity string    `json:"severity"`
	Scope    string    `json:"scope"`
	Person   string    `json:"person,omitempty"` // the incident person it matched (P1)
	At       time.Time `json:"at"`
}

type flagData struct {
	Flag Flag `json:"flag"`
}

type flagDecision struct {
	ID      string    `json:"id"`
	Action  string    `json:"action"` // approve, reject, retire, restore, renew
	Note    string    `json:"note,omitempty"`
	Expires time.Time `json:"expires,omitzero"`
	Review  time.Time `json:"review,omitzero"`
}

var severities = []string{"critical", "high", "medium", "low"}

// SaveFlag proposes, adds or changes a flag (a new version). It is active at once when the person may
// approve its category, else proposed for review; trainees only ever propose. A flag on the same scope in
// the same category is a possible duplicate: the person sees it and confirms (Conflict) rather than one
// silently replacing the other.
func (e *Engine) SaveFlag(a Actor, f Flag, conflictOK bool) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	cat, ok := e.st.Settings.Flags.Category(f.Category)
	if !ok || cat.Off {
		return "", problem("category", "choose a flag category")
	}
	if !cat.mayCreate(a) {
		e.write(a, EvAccessRefused, refs{Incident: f.Incident}, map[string]string{"flag": f.Category, "why": "may not create"})
		return "", dutyProblem("category", "your duty (%s) cannot add %s flags", DutyOf(a), strings.ToLower(cat.Name))
	}
	f.Title, f.Warning, f.Instruction = strings.TrimSpace(f.Title), strings.TrimSpace(f.Warning), strings.TrimSpace(f.Instruction)
	switch {
	case f.Title == "":
		return "", problem("title", "give the flag a short title")
	case f.Warning == "":
		return "", problem("warning", "say what the concern is and why it applies")
	case f.Instruction == "":
		return "", problem("instruction", "say what responders should do")
	}
	if f.Severity == "" {
		f.Severity = cat.Severity
	}
	if !slices.Contains(severities, f.Severity) {
		return "", problem("severity", "severity: critical, high, medium or low")
	}
	sc := &f.Scope
	sc.Address, sc.Business, sc.Complex = strings.TrimSpace(sc.Address), strings.TrimSpace(sc.Business), strings.TrimSpace(sc.Complex)
	sc.Building, sc.Floor, sc.Unit = strings.TrimSpace(sc.Building), strings.TrimSpace(sc.Floor), strings.TrimSpace(sc.Unit)
	switch {
	case sc.Person != "":
		if cat.Scope != "person" && cat.Scope != "both" {
			return "", problem("category", "%s flags are about a place, not a person", strings.ToLower(cat.Name))
		}
	case sc.Address == "":
		return "", problem("address", "choose the verified address the flag is for")
	default:
		if !sc.Verified || sc.Lat == nil || sc.Lon == nil {
			return "", problem("address", "a permanent flag needs a verified address: choose it from the address search (an unverified location can carry an incident note instead)")
		}
		sc.AddrKey = AddressKey(sc.Address)
	}
	now := e.now()
	var old *Flag
	if f.ID != "" {
		old = e.st.Flags[f.ID]
		if old == nil {
			return "", problem("id", "no flag %s", f.ID)
		}
	} else if !conflictOK {
		for _, x := range e.st.Flags {
			if x.Category == f.Category && x.State != FlagRetired && x.State != FlagRejected && sameScope(x.Scope, *sc) {
				return "", problem("conflict", "flag %s (%s) is already on this %s: change it, or confirm this one is separate", x.ID, x.Title, sc.Label())
			}
		}
	}
	f.State = FlagProposed
	if cat.mayApprove(a) && !e.st.StaffOf(a.User).Trainee {
		f.State = FlagActive
	}
	if old != nil {
		f.Version, f.Creator, f.Created, f.Incident = old.Version+1, old.Creator, old.Created, firstNonEmpty(f.Incident, old.Incident)
		if old.State == FlagActive && f.State == FlagProposed {
			return "", dutyProblem("state", "an active flag is changed by someone who may approve %s flags; propose a change in a note instead", strings.ToLower(cat.Name))
		}
	} else {
		f.ID = e.newID("FL", func(id string) bool { return e.st.Flags[id] != nil })
		f.Version, f.Creator, f.Created = 1, a.User, now
	}
	if f.Effective.IsZero() {
		f.Effective = now
	}
	if f.Review.IsZero() && cat.ReviewDays > 0 {
		f.Review = now.Add(time.Duration(cat.ReviewDays) * 24 * time.Hour)
	}
	if f.Expires.IsZero() && cat.ExpireDays > 0 {
		f.Expires = now.Add(time.Duration(cat.ExpireDays) * 24 * time.Hour)
	}
	_, err := e.write(a, EvFlagSaved, refs{Incident: f.Incident}, flagData{Flag: f})
	return f.ID, err
}

func sameScope(a, b FlagScope) bool {
	if a.Person != "" || b.Person != "" {
		return a.Person == b.Person
	}
	return a.AddrKey == b.AddrKey && normPart(a.Business) == normPart(b.Business) && normPart(a.Building) == normPart(b.Building) &&
		normPart(a.Floor) == normPart(b.Floor) && normPart(a.Unit) == normPart(b.Unit)
}

// DecideFlag approves, rejects, retires, restores or renews a flag: a new version each time.
func (e *Engine) DecideFlag(a Actor, id, action, note string, expires time.Time) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	f := e.st.Flags[id]
	if f == nil {
		return problem("id", "no flag %s", id)
	}
	cat, _ := e.st.Settings.Flags.Category(f.Category)
	if !cat.mayApprove(a) || e.st.StaffOf(a.User).Trainee {
		e.write(a, EvAccessRefused, refs{}, map[string]string{"flag": id, "why": "may not " + action})
		return dutyProblem("action", "your duty (%s) cannot %s %s flags", DutyOf(a), action, strings.ToLower(cat.Name))
	}
	switch action {
	case "approve", "reject":
		if f.State != FlagProposed {
			return problem("action", "flag %s is not waiting for approval", id)
		}
	case "retire":
		if f.State != FlagActive {
			return problem("action", "flag %s is not active", id)
		}
	case "restore":
		if f.State != FlagRetired {
			return problem("action", "flag %s is not retired", id)
		}
	case "renew":
		if f.State != FlagActive && f.State != FlagRetired {
			return problem("action", "only an active or retired flag is renewed")
		}
	default:
		return problem("action", "approve, reject, retire, restore or renew")
	}
	if (action == "reject" || action == "retire") && strings.TrimSpace(note) == "" {
		return problem("note", "say why")
	}
	d := flagDecision{ID: id, Action: action, Note: strings.TrimSpace(note), Expires: expires}
	if action == "renew" && cat.ReviewDays > 0 {
		d.Review = e.now().Add(time.Duration(cat.ReviewDays) * 24 * time.Hour)
	}
	_, err := e.write(a, EvFlagDecided, refs{}, d)
	return err
}

func (s *State) applyFlags(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvFlagSaved:
		d, err := decode[flagData](e)
		if err != nil {
			return true, err
		}
		f := d.Flag
		if old := s.Flags[f.ID]; old != nil {
			f.History = append(append([]FlagVersion{}, old.History...), old.version())
		}
		f.Changed, f.ChangedBy = e.At, e.Actor
		if f.State == FlagActive && f.Verifier == "" {
			f.Verifier, f.Verified = e.Actor, e.At
		}
		s.Flags[f.ID] = &f
		if in := s.Incidents[f.Incident]; in != nil && f.Version == 1 {
			in.add(e, "flag", fmt.Sprintf("%s flag %s %s: %s", f.Category, f.ID, map[bool]string{true: "added", false: "proposed for review"}[f.State == FlagActive], f.Title))
		}
		return true, nil
	case EvFlagDecided:
		d, err := decode[flagDecision](e)
		if err != nil {
			return true, err
		}
		f := s.Flags[d.ID]
		if f == nil {
			return true, nil
		}
		f.History = append(f.History, f.version())
		f.Version++
		f.Changed, f.ChangedBy, f.Note = e.At, e.Actor, d.Note
		switch d.Action {
		case "approve", "restore":
			f.State, f.Verifier, f.Verified = FlagActive, e.Actor, e.At
		case "reject":
			f.State = FlagRejected
		case "retire":
			f.State = FlagRetired
		case "renew":
			f.State = FlagActive
			if !d.Review.IsZero() {
				f.Review = d.Review
			}
		}
		if !d.Expires.IsZero() {
			f.Expires = d.Expires
		}
		return true, nil
	case EvFlagViewed:
		return true, nil
	}
	return false, nil
}

func (f *Flag) version() FlagVersion {
	return FlagVersion{Version: f.Version, Title: f.Title, Warning: f.Warning, Instruction: f.Instruction, Severity: f.Severity,
		State: f.State, Expires: f.Expires, Changed: f.Changed, ChangedBy: f.ChangedBy, Note: f.Note}
}

// FlagsAt are the active flags for a verified address and sublocation: the whole-property and complex
// flags always, a building's only in that building, a floor's only on that floor, an apartment's only in
// that apartment. Most severe first.
func (s *State) FlagsAt(addrKey string, p *Place, now time.Time) []*Flag {
	if addrKey == "" {
		return nil
	}
	var out []*Flag
	for _, f := range s.Flags {
		if f.Scope.AddrKey != addrKey || !f.ActiveAt(now) {
			continue
		}
		if !scopeFits(f.Scope, p) {
			continue
		}
		out = append(out, f)
	}
	sortFlags(out)
	return out
}

func scopeFits(sc FlagScope, p *Place) bool {
	var pb, pf, pu, pn string
	if p != nil {
		pb, pf, pu, pn = p.Building, p.Floor, p.Unit, firstNonEmpty(p.Name, p.Complex)
	}
	match := func(flag, place string) bool { return flag == "" || normPart(flag) == normPart(place) }
	if sc.Business != "" && p != nil && pn != "" && normPart(sc.Business) != normPart(p.Name) && normPart(sc.Business) != normPart(p.Complex) {
		return false
	}
	return match(sc.Building, pb) && match(sc.Floor, pf) && match(sc.Unit, pu)
}

func sortFlags(fs []*Flag) {
	rank := func(s string) int { return slices.Index(severities, s) }
	sort.Slice(fs, func(i, j int) bool {
		if rank(fs[i].Severity) != rank(fs[j].Severity) {
			return rank(fs[i].Severity) < rank(fs[j].Severity)
		}
		return fs[i].ID < fs[j].ID
	})
}

// FlagsNear are active address flags within the agency's distance of a point: offered as possible matches
// for an unverified location, never attached to it.
func (s *State) FlagsNear(lat, lon float64, now time.Time) []*Flag {
	m := s.Settings.Flags.NearMetres
	if m <= 0 {
		m = 80
	}
	var out []*Flag
	for _, f := range s.Flags {
		if f.Scope.Lat == nil || f.Scope.Lon == nil || !f.ActiveAt(now) {
			continue
		}
		if metres(lat, lon, *f.Scope.Lat, *f.Scope.Lon) <= m {
			out = append(out, f)
		}
	}
	sortFlags(out)
	return out
}

func metres(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371000.0
	p1, p2 := lat1*math.Pi/180, lat2*math.Pi/180
	dp, dl := (lat2-lat1)*math.Pi/180, (lon2-lon1)*math.Pi/180
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * r * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// FlagsForPerson are a person's active warnings.
func (s *State) FlagsForPerson(key string, now time.Time) []*Flag {
	if key == "" {
		return nil
	}
	var out []*Flag
	for _, f := range s.Flags {
		if f.Scope.Person == key && f.ActiveAt(now) {
			out = append(out, f)
		}
	}
	sortFlags(out)
	return out
}

// IncidentFlags are the flags that apply to an incident now: its verified place's, and its people's.
func (s *State) IncidentFlags(in *Incident, now time.Time) []*Flag {
	var out []*Flag
	if in.Place != nil && in.Place.Verify == PlaceVerified && in.Place.AddrKey != "" {
		out = append(out, s.FlagsAt(in.Place.AddrKey, in.Place, now)...)
	} else if in.Place == nil && in.LocVerified && in.Location != nil {
		out = append(out, s.FlagsAt(AddressKey(in.Location.Address), nil, now)...)
	}
	for _, p := range in.Persons {
		if p.Removed {
			continue
		}
		for _, f := range s.FlagsForPerson(PersonKey(p), now) {
			if !slices.Contains(out, f) {
				out = append(out, f)
			}
		}
	}
	return out
}

// noteFlagsShown keeps on the incident each flag version it now shows.
func (s *State) noteFlagsShown(e ledger.Event, in *Incident) {
	for _, f := range s.IncidentFlags(in, e.At) {
		if slices.ContainsFunc(in.Flags, func(x FlagShown) bool { return x.ID == f.ID && x.Version == f.Version }) {
			continue
		}
		in.Flags = append(in.Flags, FlagShown{ID: f.ID, Version: f.Version, Category: f.Category, Title: f.Title, Severity: f.Severity, Scope: f.Scope.Label(), Person: f.Scope.PersonName, At: e.At})
		in.add(e, "flag", fmt.Sprintf("%s flag shown: %s (%s, version %d)", strings.ToUpper(f.Severity), f.Title, f.Scope.Label(), f.Version))
	}
}

// FlagFor is a flag as one person may see it: a restricted category's detail (the reason, source and
// details) is left out for those the agency does not let see it; the operational warning stays.
func (s *State) FlagFor(f *Flag, duty, role string) *Flag {
	c := *f
	c.History = nil
	cat, _ := s.Settings.Flags.Category(f.Category)
	if !cat.MayView(duty, role) {
		c.Warning, c.Source, c.Details, c.Reliability, c.Scope.PersonName = "", "", nil, "", ""
		c.Note = "restricted: the detail is for " + strings.Join(cat.View, ", ")
	}
	return &c
}
