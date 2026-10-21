package cad

// Call entry and the work of each role: incident modifiers, the people and vehicles on an incident, where
// the caller is when not at the incident, notifications and logged contacts, unit acknowledgement, BOLOs,
// facility (hospital) status, the shift passdown log, response plans, the approved-guidance library, and
// what each role may do. Every change is a ledger event, like the rest of the board.

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/geo"
	"github.com/biodream-llc/bitdispatcher/internal/guidance"
	"github.com/biodream-llc/bitdispatcher/internal/ledger"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// Duties: the work a person signs in to do. A person may hold several (their qualifications) and chooses
// one at sign-in; what they may do follows the one chosen.
const (
	DutyCallTaker  = "calltaker"
	DutyPolice     = "police"
	DutyFire       = "fire"
	DutyEMS        = "ems"
	DutyDispatcher = "dispatcher" // every discipline, for a centre where one person does it all
	DutySupervisor = "supervisor"
	DutyAdmin      = "admin"
	DutyRecords    = "records" // records staff: incident review and records requests
	DutyField      = "field"
	DutyViewer     = "viewer"
)

// Events of call entry and the roles' work.
const (
	EvIncModifiers    = "incident.modifiers"
	EvIncPerson       = "incident.person"
	EvIncVehicle      = "incident.vehicle"
	EvIncCallerAt     = "incident.caller-location"
	EvIncNotice       = "incident.notice"
	EvIncTalkgroup    = "incident.talkgroup"
	EvIncLocOffered   = "incident.location-offered"
	EvIncLocDecided   = "incident.location-decided"
	EvIncVerified     = "incident.location-verified"
	EvUnitAcked       = "unit.acked"
	EvBOLOSaved       = "bolo.saved"
	EvBOLOCancelled   = "bolo.cancelled"
	EvFacilitySaved   = "facility.saved"
	EvFacilityStatus  = "facility.status"
	EvFacilityGone    = "facility.removed"
	EvPassdown        = "passdown.added"
	EvPlanSaved       = "plan.saved"
	EvPlanRemoved     = "plan.removed"
	EvGuidanceSaved   = "guidance.saved"
	EvGuidanceGone    = "guidance.removed"
	EvGuidanceShown   = "guidance.shown"
	EvLocked          = "session.locked"
	EvUnlocked        = "session.unlocked"
	EvReviewExported  = "review.exported"
	EvConfigStaged    = "config.published" // configuration published from the staging (training) board
	EvWorkstationCode = "workstation.code"
	EvUserDuties      = "user.duties"
	EvUserMFAReset    = "user.mfa-reset"
	EvRecordsViewed   = "records.viewed" // an incident's record opened for review
	EvTrainingInject  = "training.inject"
)

// Modifier is one of the agency's incident modifiers: weapons involved, in progress, entrapment. A modifier
// may raise the priority, and may name people to notify.
type Modifier struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Priority    int      `json:"priority,omitempty"`    // raises the incident to at least this priority (1 highest)
	Disciplines []string `json:"disciplines,omitempty"` // offered for call types of these disciplines; empty for all
	Notify      []string `json:"notify,omitempty"`
	Safety      bool     `json:"safety,omitempty"` // an officer-safety modifier: shown as a critical alert
}

// DutyRules are what each role may do; the server enforces them, not only the screen.
type DutyRules struct {
	// CallTakersDispatch lets call-takers send and change units (a small centre where they also dispatch).
	CallTakersDispatch bool `json:"callTakersDispatch,omitempty"`
	// CrossDiscipline lets a police, fire or EMS dispatcher send and change units of other disciplines.
	CrossDiscipline bool `json:"crossDiscipline,omitempty"`
	// SelfAttach lets a crew attach its own unit to a waiting incident from the mobile terminal.
	SelfAttach bool `json:"selfAttach,omitempty"`
}

// Security is the sign-in policy.
type Security struct {
	MFA         string `json:"mfa,omitempty"`         // off, admins (supervisors and administrators) or everyone
	LockMinutes int    `json:"lockMinutes,omitempty"` // lock a console after this long without input (0: never)
	// StagedChanges sends configuration changes through the staging board: the live board's configuration
	// is changed only by publishing from staging.
	StagedChanges bool `json:"stagedChanges,omitempty"`
}

// Person is someone on an incident: the caller, a victim, a suspect, a witness.
type Person struct {
	ID       string    `json:"id"`
	Role     string    `json:"role"` // caller, victim, suspect, witness, complainant, subject, patient, other
	Name     string    `json:"name,omitempty"`
	Aliases  []string  `json:"aliases,omitempty"`
	DOB      string    `json:"dob,omitempty"` // as given: 1988-03-14, or an age
	Sex      string    `json:"sex,omitempty"`
	Race     string    `json:"race,omitempty"`
	Height   string    `json:"height,omitempty"`
	Weight   string    `json:"weight,omitempty"`
	Hair     string    `json:"hair,omitempty"`
	Eyes     string    `json:"eyes,omitempty"`
	Clothing string    `json:"clothing,omitempty"`
	Address  string    `json:"address,omitempty"`
	Phone    string    `json:"phone,omitempty"`
	IDType   string    `json:"idType,omitempty"` // DL, state ID, passport
	IDNumber string    `json:"idNumber,omitempty"`
	IDState  string    `json:"idState,omitempty"`
	Notes    string    `json:"notes,omitempty"`
	Safety   bool      `json:"safety,omitempty"` // an officer-safety concern about this person
	Added    time.Time `json:"added"`
	AddedBy  string    `json:"addedBy"`
	Updated  time.Time `json:"updated,omitzero"`
	Removed  bool      `json:"removed,omitempty"`
}

// Vehicle is a vehicle on an incident.
type Vehicle struct {
	ID          string    `json:"id"`
	Role        string    `json:"role"` // suspect, victim, involved, witness, abandoned, other
	Plate       string    `json:"plate,omitempty"`
	State       string    `json:"state,omitempty"`
	VIN         string    `json:"vin,omitempty"`
	Year        string    `json:"year,omitempty"`
	Make        string    `json:"make,omitempty"`
	Model       string    `json:"model,omitempty"`
	Color       string    `json:"color,omitempty"`
	Body        string    `json:"body,omitempty"`
	Description string    `json:"description,omitempty"`
	Driver      string    `json:"driver,omitempty"`
	Owner       string    `json:"owner,omitempty"`
	Contact     string    `json:"contact,omitempty"`
	Direction   string    `json:"direction,omitempty"` // direction of travel
	Notes       string    `json:"notes,omitempty"`
	Added       time.Time `json:"added"`
	AddedBy     string    `json:"addedBy"`
	Updated     time.Time `json:"updated,omitzero"`
	Removed     bool      `json:"removed,omitempty"`
}

// Contact is a notification, a mutual-aid or resource request, a transfer or referral, an outbound call or
// instructions given: something done for an incident besides sending its own units, with whether it is done.
type Contact struct {
	ID     string    `json:"id"`
	Kind   string    `json:"kind"` // notify, mutualaid, resource, transfer, referral, outbound, instructions
	Who    string    `json:"who"`
	Detail string    `json:"detail,omitempty"`
	Status string    `json:"status"` // pending, done, cancelled
	At     time.Time `json:"at"`
	By     string    `json:"by"`
	Done   time.Time `json:"done,omitzero"`
	DoneBy string    `json:"doneBy,omitempty"`
	Auto   bool      `json:"auto,omitempty"` // listed by the call type or a modifier, not added by hand
}

// LocOffer is a newer caller location (a rebid, a supplemental location provider, a new ALI) that differs
// from the incident's: the call-taker decides whether it becomes the incident's location.
type LocOffer struct {
	Location *ng911.Location `json:"location"`
	Call     string          `json:"call,omitempty"`
	Metres   float64         `json:"metres"`
	At       time.Time       `json:"at"`
}

// LocChange is the last accepted change of an incident's location, kept so dispatchers and units see it.
type LocChange struct {
	From *ng911.Location `json:"from,omitempty"`
	To   *ng911.Location `json:"to"`
	At   time.Time       `json:"at"`
	By   string          `json:"by"`
	Why  string          `json:"why,omitempty"`
}

// Shown is a passage of approved guidance shown for an incident, by version, so a reviewer later sees what
// was available at the time.
type Shown struct {
	Doc     string    `json:"doc"`
	Version string    `json:"version"`
	Anchor  string    `json:"anchor"`
	Heading string    `json:"heading"`
	At      time.Time `json:"at"`
	By      string    `json:"by"`
}

// BOLO is a be-on-the-lookout (or attempt-to-locate) broadcast to units.
type BOLO struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"` // vehicle, person, property, other
	Text        string    `json:"text"`
	Plate       string    `json:"plate,omitempty"`
	Vehicle     string    `json:"vehicle,omitempty"`
	Person      string    `json:"person,omitempty"`
	Incident    string    `json:"incident,omitempty"`
	Disciplines []string  `json:"disciplines,omitempty"` // who it is for; empty for everyone
	Issued      time.Time `json:"issued"`
	By          string    `json:"by"`
	Expires     time.Time `json:"expires,omitzero"`
	Cancelled   time.Time `json:"cancelled,omitzero"`
	CancelledBy string    `json:"cancelledBy,omitempty"`
	Updates     []Entry   `json:"updates,omitempty"`
}

// Active reports whether the BOLO is still out.
func (b *BOLO) Active(now time.Time) bool {
	return b.Cancelled.IsZero() && (b.Expires.IsZero() || now.Before(b.Expires))
}

// Facility is a receiving facility (a hospital, a trauma or burn centre) and whether it is taking patients.
type Facility struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Kind    string    `json:"kind,omitempty"` // hospital, trauma, burn, stroke, cardiac, psychiatric
	Lat     float64   `json:"lat,omitempty"`
	Lon     float64   `json:"lon,omitempty"`
	Phone   string    `json:"phone,omitempty"`
	Status  string    `json:"status"` // open, diversion, closed
	Note    string    `json:"note,omitempty"`
	Until   time.Time `json:"until,omitzero"`
	Updated time.Time `json:"updated,omitzero"`
	By      string    `json:"by,omitempty"`
}

// Passdown is an entry in the shift passdown log: what the next shift needs to know.
type Passdown struct {
	ID    string    `json:"id"`
	Text  string    `json:"text"`
	For   string    `json:"for,omitempty"` // a duty (police, fire...) or empty for everyone
	At    time.Time `json:"at"`
	By    string    `json:"by"`
	Until time.Time `json:"until,omitzero"`
}

// ResponsePlan is an agency's run order for some call types in an area or at some addresses, at some
// times: which units go first, second, third due. The recommendation follows it, showing where a planned
// unit cannot go and who goes instead; a person always decides.
type ResponsePlan struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CallTypes []string  `json:"callTypes,omitempty"` // empty for every type of its discipline
	Modifiers []string  `json:"modifiers,omitempty"` // only when the incident has all of these
	Area      string    `json:"area,omitempty"`      // a station's area (first due by distance), or empty for anywhere
	Addresses []string  `json:"addresses,omitempty"` // address-specific: the incident address starts with one of these
	From      string    `json:"from,omitempty"`      // "07:00": in effect from, local time; with To
	To        string    `json:"to,omitempty"`
	Order     []string  `json:"order"` // units in due order
	Note      string    `json:"note,omitempty"`
	Updated   time.Time `json:"updated,omitzero"`
}

// GuidanceDoc is one document in the approved-guidance library.
type GuidanceDoc = guidance.Doc

func (s *State) initEntry() {
	if s.BOLOs == nil {
		s.BOLOs = map[string]*BOLO{}
		s.Facilities = map[string]*Facility{}
		s.Plans = map[string]*ResponsePlan{}
		s.Guidance = map[string]*GuidanceDoc{}
	}
}

// ---- settings helpers ----

// AckAfter is how long a dispatched unit has to acknowledge before it is flagged.
func (s Settings) AckAfter() time.Duration {
	if s.AckSeconds > 0 {
		return time.Duration(s.AckSeconds) * time.Second
	}
	return time.Minute
}

// MoveRule is how far a newer caller location must be from the incident's to be offered.
func (s Settings) MoveRule() float64 {
	if s.MoveMeters > 0 {
		return s.MoveMeters
	}
	return 50
}

// Modifier finds an incident modifier by code.
func (s Settings) Modifier(code string) (Modifier, bool) {
	for _, m := range s.Modifiers {
		if strings.EqualFold(m.Code, code) {
			return m, true
		}
	}
	return Modifier{}, false
}

// DefaultModifiers are offered until the agency sets its own.
var DefaultModifiers = []Modifier{
	{Code: "WEAPONS", Name: "Weapons involved", Priority: 1, Safety: true},
	{Code: "INPROG", Name: "In progress", Priority: 1},
	{Code: "INJURY", Name: "Injuries"},
	{Code: "TRAPPED", Name: "Entrapment", Priority: 1, Disciplines: []string{"fire", "ems"}},
	{Code: "DUI", Name: "Impaired driver", Disciplines: []string{"police"}},
	{Code: "JUVENILE", Name: "Juvenile involved"},
	{Code: "DV", Name: "Domestic violence", Safety: true, Disciplines: []string{"police"}},
	{Code: "HAZMAT", Name: "Hazardous materials", Notify: []string{"Hazardous materials team"}},
	{Code: "ELDERLY", Name: "Elderly or vulnerable person"},
	{Code: "MENTAL", Name: "Mental health crisis", Notify: []string{"Crisis intervention team"}},
}

// ---- duties ----

// DutyOf is an actor's duty: the one they signed in to do, or one their role implies.
func DutyOf(a Actor) string {
	if a.Duty != "" {
		return a.Duty
	}
	switch a.Role {
	case "admin":
		return DutyAdmin
	case "supervisor":
		return DutySupervisor
	case "dispatcher":
		return DutyDispatcher
	case "field":
		return DutyField
	case "viewer":
		return DutyViewer
	}
	return "" // a system actor: bitvoice, ali, radio
}

// disciplineOf is a unit's discipline: its type's.
func (s *State) disciplineOf(u *Unit) string {
	for _, t := range s.Settings.UnitTypes {
		if t.Code == u.Type {
			return t.Discipline
		}
	}
	return "other"
}

// mayCommand reports whether an actor may send or change a unit, by the duty they signed in to do.
// Supervisors, administrators, all-discipline dispatchers, field crews (their own unit, checked elsewhere)
// and the system may; a call-taker only when the agency allows it; a police, fire or EMS dispatcher their
// own discipline's units (and "other"), or any when the agency allows it.
func (s *State) mayCommand(a Actor, u *Unit) error {
	switch d := DutyOf(a); d {
	case "", DutyDispatcher, DutySupervisor, DutyAdmin, DutyField:
		return nil
	case DutyCallTaker:
		if s.Settings.Duties.CallTakersDispatch {
			return nil
		}
		return problem("duty", "signed in as a call-taker, you cannot send or change units: the dispatcher does (an administrator can allow call-takers to dispatch in Settings › Roles)")
	case DutyPolice, DutyFire, DutyEMS:
		disc := s.disciplineOf(u)
		if disc == d || disc == "other" || s.Settings.Duties.CrossDiscipline {
			return nil
		}
		return problem("duty", "%s is a %s unit and you are signed in as the %s dispatcher: ask the %s dispatcher, or a supervisor (Settings › Roles can allow any discipline)", u.ID, disciplineName(disc), disciplineName(d), disciplineName(disc))
	default:
		return problem("duty", "signed in as %s, you cannot send or change units", d)
	}
}

func disciplineName(d string) string {
	switch d {
	case "ems":
		return "EMS"
	case "":
		return "other"
	}
	return d
}

// mayEnter reports whether an actor may enter and change incidents (call entry): everyone who works the
// floor, not records staff or viewers.
func mayEnter(a Actor) error {
	switch DutyOf(a) {
	case DutyRecords, DutyViewer:
		return problem("duty", "signed in for records or viewing, you cannot change incidents")
	}
	return nil
}

// ---- events' data ----

type personData struct {
	Person Person `json:"person"`
}
type vehicleData struct {
	Vehicle Vehicle `json:"vehicle"`
}
type noticeData struct {
	Contact Contact `json:"notice"`
}
type locDecision struct {
	Accept bool   `json:"accept"`
	Why    string `json:"why,omitempty"`
}

// ---- engine commands ----

func cleanList(xs []string) []string {
	out := []string{}
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x != "" && !slices.Contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}

// SetModifiers sets an incident's modifiers. A modifier that raises the priority raises it, and one that
// names people to notify adds them to the incident's checklist.
func (e *Engine) SetModifiers(a Actor, number string, mods []string) error {
	if err := mayEnter(a); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	in, err := e.openIncident(number)
	if err != nil {
		return err
	}
	set := e.st.Settings
	clean := []string{}
	for _, m := range mods {
		m = strings.ToUpper(strings.TrimSpace(m))
		if m == "" || slices.Contains(clean, m) {
			continue
		}
		if _, ok := set.modifierOrDefault(m); !ok {
			return problem("modifiers", "no modifier %s: add it in Settings › Call entry", m)
		}
		clean = append(clean, m)
	}
	if _, err := e.write(a, EvIncModifiers, refs{Incident: number}, map[string][]string{"modifiers": clean}); err != nil {
		return err
	}
	return e.modifierEffects(a, in, clean)
}

// modifierEffects raises an incident's priority as its modifiers ask, and lists the people they (and its
// call type) name to notify. The caller holds e.mu.
func (e *Engine) modifierEffects(a Actor, in *Incident, clean []string) error {
	set := e.st.Settings
	number := in.Number
	// The highest priority a modifier asks for.
	want := 0
	for _, m := range clean {
		if md, _ := set.modifierOrDefault(m); md.Priority > 0 && (want == 0 || md.Priority < want) {
			want = md.Priority
		}
	}
	if want > 0 && want < in.Priority {
		p := want
		if _, err := e.write(a, EvIncChanged, refs{Incident: number}, incidentChanged{Priority: &p, Reason: "raised by a modifier"}); err != nil {
			return err
		}
	}
	type who struct{ name, why string }
	var list []who
	if ct, ok := e.st.CallTypes[in.Type]; ok {
		for _, w := range ct.Notify {
			list = append(list, who{w, "for " + ct.Name})
		}
	}
	for _, m := range clean {
		md, _ := set.modifierOrDefault(m)
		for _, w := range md.Notify {
			list = append(list, who{w, "for " + md.Name})
		}
	}
	for _, w := range list {
		if !hasNotice(in, w.name) {
			n := Contact{ID: fmt.Sprintf("N%d", len(in.Notices)+1), Kind: "notify", Who: w.name, Status: "pending", At: e.now(), By: a.User, Auto: true, Detail: w.why}
			if _, err := e.write(a, EvIncNotice, refs{Incident: number}, noticeData{Contact: n}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s Settings) modifierOrDefault(code string) (Modifier, bool) {
	list := s.Modifiers
	if len(list) == 0 {
		list = DefaultModifiers
	}
	for _, m := range list {
		if strings.EqualFold(m.Code, code) {
			return m, true
		}
	}
	return Modifier{}, false
}

// ModifierList is the agency's modifiers, or the defaults.
func (s Settings) ModifierList() []Modifier {
	if len(s.Modifiers) == 0 {
		return DefaultModifiers
	}
	return s.Modifiers
}

func hasNotice(in *Incident, who string) bool {
	for _, n := range in.Notices {
		if strings.EqualFold(n.Who, who) && n.Status != "cancelled" {
			return true
		}
	}
	return false
}

// SavePerson adds a person to an incident, or changes one (by ID); Removed takes them off (the ledger keeps them).
func (e *Engine) SavePerson(a Actor, number string, p Person) (string, error) {
	if err := mayEnter(a); err != nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	in, ok := e.st.Incidents[number]
	if !ok {
		return "", problem("incident", "no incident %s", number)
	}
	p.Name, p.Role = strings.TrimSpace(p.Name), strings.ToLower(strings.TrimSpace(p.Role))
	if p.Role == "" {
		p.Role = "other"
	}
	if p.Name == "" && p.Clothing == "" && p.Notes == "" && !p.Removed {
		return "", problem("name", "give a name, or a description in the clothing or notes")
	}
	now := e.now()
	if p.ID == "" {
		p.ID = fmt.Sprintf("P%d", len(in.Persons)+1)
		p.Added, p.AddedBy = now, a.User
	} else {
		i := slices.IndexFunc(in.Persons, func(x Person) bool { return x.ID == p.ID })
		if i < 0 {
			return "", problem("person", "no person %s on %s", p.ID, number)
		}
		p.Added, p.AddedBy, p.Updated = in.Persons[i].Added, in.Persons[i].AddedBy, now
	}
	p.Aliases = cleanList(p.Aliases)
	_, err := e.write(a, EvIncPerson, refs{Incident: number}, personData{Person: p})
	return p.ID, err
}

// SaveVehicle adds a vehicle to an incident, or changes one (by ID).
func (e *Engine) SaveVehicle(a Actor, number string, v Vehicle) (string, error) {
	if err := mayEnter(a); err != nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	in, ok := e.st.Incidents[number]
	if !ok {
		return "", problem("incident", "no incident %s", number)
	}
	v.Plate = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(v.Plate), " ", ""))
	v.VIN = strings.ToUpper(strings.TrimSpace(v.VIN))
	v.State = strings.ToUpper(strings.TrimSpace(v.State))
	v.Role = strings.ToLower(strings.TrimSpace(v.Role))
	if v.Role == "" {
		v.Role = "involved"
	}
	if v.Plate == "" && v.VIN == "" && v.Make == "" && v.Model == "" && v.Color == "" && v.Description == "" && !v.Removed {
		return "", problem("plate", "give a plate, VIN or a description")
	}
	now := e.now()
	if v.ID == "" {
		v.ID = fmt.Sprintf("V%d", len(in.Vehicles)+1)
		v.Added, v.AddedBy = now, a.User
	} else {
		i := slices.IndexFunc(in.Vehicles, func(x Vehicle) bool { return x.ID == v.ID })
		if i < 0 {
			return "", problem("vehicle", "no vehicle %s on %s", v.ID, number)
		}
		v.Added, v.AddedBy, v.Updated = in.Vehicles[i].Added, in.Vehicles[i].AddedBy, now
	}
	_, err := e.write(a, EvIncVehicle, refs{Incident: number}, vehicleData{Vehicle: v})
	return v.ID, err
}

// SetCallerLocation records where the caller is when it is not where the incident is (nil: the same place).
func (e *Engine) SetCallerLocation(a Actor, number string, loc *ng911.Location) error {
	if err := mayEnter(a); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.st.Incidents[number]; !ok {
		return problem("incident", "no incident %s", number)
	}
	_, err := e.write(a, EvIncCallerAt, refs{Incident: number}, map[string]*ng911.Location{"location": loc})
	return err
}

// SaveContact adds a notification, request, transfer, outbound call or instructions to an incident, or
// marks one done or cancelled (by ID).
func (e *Engine) SaveContact(a Actor, number string, n Contact) (string, error) {
	if err := mayEnter(a); err != nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	in, ok := e.st.Incidents[number]
	if !ok {
		return "", problem("incident", "no incident %s", number)
	}
	now := e.now()
	if n.ID == "" {
		n.Kind = strings.ToLower(strings.TrimSpace(n.Kind))
		if !slices.Contains([]string{"notify", "mutualaid", "resource", "transfer", "referral", "outbound", "instructions"}, n.Kind) {
			return "", problem("kind", "a notice is notify, mutualaid, resource, transfer, referral, outbound or instructions")
		}
		if strings.TrimSpace(n.Who) == "" {
			return "", problem("who", "say who: the agency, person or number")
		}
		n.ID, n.At, n.By = fmt.Sprintf("N%d", len(in.Notices)+1), now, a.User
		if n.Status == "" {
			n.Status = "pending"
		}
	} else {
		i := slices.IndexFunc(in.Notices, func(x Contact) bool { return x.ID == n.ID })
		if i < 0 {
			return "", problem("notice", "no notice %s on %s", n.ID, number)
		}
		old := in.Notices[i]
		status := n.Status
		detail := n.Detail
		n = old
		if detail != "" {
			n.Detail = detail
		}
		if status != "" {
			n.Status = status
		}
	}
	if n.Status == "done" && n.Done.IsZero() {
		n.Done, n.DoneBy = now, a.User
	}
	if !slices.Contains([]string{"pending", "done", "cancelled"}, n.Status) {
		return "", problem("status", "a notice is pending, done or cancelled")
	}
	_, err := e.write(a, EvIncNotice, refs{Incident: number}, noticeData{Contact: n})
	return n.ID, err
}

// SetTalkgroup sets the talkgroup an incident is worked on.
func (e *Engine) SetTalkgroup(a Actor, number, tg string) error {
	if err := mayEnter(a); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, err := e.openIncident(number); err != nil {
		return err
	}
	_, err := e.write(a, EvIncTalkgroup, refs{Incident: number}, map[string]string{"talkgroup": strings.TrimSpace(tg)})
	return err
}

// VerifyLocation records that the call-taker confirmed an incident's location on the map.
func (e *Engine) VerifyLocation(a Actor, number string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, err := e.openIncident(number); err != nil {
		return err
	}
	_, err := e.write(a, EvIncVerified, refs{Incident: number}, nil)
	return err
}

// DecideLocation accepts or keeps away a newer caller location offered for an incident. Accepted, it becomes
// the incident's location (the old one stays in the timeline) and dispatchers and units are told.
func (e *Engine) DecideLocation(a Actor, number string, accept bool, why string) error {
	if err := mayEnter(a); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	in, err := e.openIncident(number)
	if err != nil {
		return err
	}
	if in.NewLocation == nil {
		return problem("location", "no newer caller location is waiting for %s", number)
	}
	_, err = e.write(a, EvIncLocDecided, refs{Incident: number}, locDecision{Accept: accept, Why: strings.TrimSpace(why)})
	return err
}

// AckUnit records that a unit acknowledged its assignment.
func (e *Engine) AckUnit(a Actor, unit, how string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	u, ok := e.st.Units[strings.ToUpper(unit)]
	if !ok {
		return problem("unit", "no unit %s", unit)
	}
	if u.Incident == "" || !Committed(u.Status) {
		return problem("unit", "%s is not on an incident", u.ID)
	}
	_, err := e.write(a, EvUnitAcked, refs{Incident: u.Incident, Unit: u.ID}, map[string]string{"how": how})
	return err
}

// SaveBOLO issues or updates a BOLO; text added to an existing one is kept as an update.
func (e *Engine) SaveBOLO(a Actor, b BOLO) (string, error) {
	if err := mayEnter(a); err != nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	b.Text = strings.TrimSpace(b.Text)
	b.Plate = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(b.Plate), " ", ""))
	if b.ID == "" {
		if b.Text == "" {
			return "", problem("text", "say what to look out for")
		}
		e.st.initEntry()
		b.ID = fmt.Sprintf("BOLO-%d", len(e.st.BOLOs)+1)
		b.Issued, b.By = e.now(), a.User
		if b.Kind == "" {
			b.Kind = "other"
			if b.Plate != "" || b.Vehicle != "" {
				b.Kind = "vehicle"
			}
		}
	} else if old, ok := e.st.BOLOs[b.ID]; ok {
		upd := b.Text
		b = *old
		if upd != "" {
			b.Updates = append(b.Updates, Entry{At: e.now(), Actor: a.User, Kind: "update", Text: upd})
		}
	} else {
		return "", problem("bolo", "no BOLO %s", b.ID)
	}
	_, err := e.write(a, EvBOLOSaved, refs{Incident: b.Incident}, b)
	return b.ID, err
}

// CancelBOLO cancels a BOLO.
func (e *Engine) CancelBOLO(a Actor, id, why string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	b, ok := e.st.BOLOs[id]
	if !ok {
		return problem("bolo", "no BOLO %s", id)
	}
	_, err := e.write(a, EvBOLOCancelled, refs{Incident: b.Incident}, map[string]string{"id": id, "why": why})
	return err
}

// SaveFacility adds or changes a receiving facility.
func (e *Engine) SaveFacility(a Actor, f Facility) error {
	f.ID, f.Name = strings.ToUpper(strings.TrimSpace(f.ID)), strings.TrimSpace(f.Name)
	if f.ID == "" || f.Name == "" {
		return problem("id", "a facility needs a short ID and a name")
	}
	if f.Status == "" {
		f.Status = "open"
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	f.Updated, f.By = e.now(), a.User
	_, err := e.write(a, EvFacilitySaved, refs{}, f)
	return err
}

// RemoveFacility takes a facility off the list.
func (e *Engine) RemoveFacility(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.st.Facilities[id]; !ok {
		return problem("facility", "no facility %s", id)
	}
	_, err := e.write(a, EvFacilityGone, refs{}, map[string]string{"id": id})
	return err
}

// SetFacilityStatus records a facility open, on diversion or closed, until a time if given.
func (e *Engine) SetFacilityStatus(a Actor, id, status, note string, until time.Time) error {
	if !slices.Contains([]string{"open", "diversion", "closed"}, status) {
		return problem("status", "a facility is open, diversion or closed")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	f, ok := e.st.Facilities[id]
	if !ok {
		return problem("facility", "no facility %s", id)
	}
	_, err := e.write(a, EvFacilityStatus, refs{}, Facility{ID: f.ID, Status: status, Note: strings.TrimSpace(note), Until: until})
	return err
}

// AddPassdown adds an entry to the shift passdown log.
func (e *Engine) AddPassdown(a Actor, text, forDuty string, until time.Time) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return problem("text", "write what the next shift needs to know")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	p := Passdown{ID: fmt.Sprintf("PD%d", len(e.st.Passdown)+1), Text: text, For: forDuty, At: e.now(), By: a.User, Until: until}
	_, err := e.write(a, EvPassdown, refs{}, p)
	return err
}

// SavePlan adds or changes a response plan.
func (e *Engine) SavePlan(a Actor, p ResponsePlan) (string, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return "", problem("name", "name the plan")
	}
	for i, u := range p.Order {
		p.Order[i] = strings.ToUpper(strings.TrimSpace(u))
	}
	p.Order = cleanList(p.Order)
	if len(p.Order) == 0 {
		return "", problem("order", "list the units in due order")
	}
	for i, c := range p.CallTypes {
		p.CallTypes[i] = strings.ToUpper(strings.TrimSpace(c))
	}
	for i, m := range p.Modifiers {
		p.Modifiers[i] = strings.ToUpper(strings.TrimSpace(m))
	}
	for _, t := range []string{p.From, p.To} {
		if t != "" {
			if _, err := time.Parse("15:04", t); err != nil {
				return "", problem("from", "times are HH:MM, like 07:00")
			}
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, u := range p.Order {
		if _, ok := e.st.Units[u]; !ok {
			return "", problem("order", "no unit %s", u)
		}
	}
	if p.ID == "" {
		p.ID = fmt.Sprintf("RP%d", len(e.st.Plans)+1)
		for e.st.Plans[p.ID] != nil {
			p.ID += "x"
		}
	}
	p.Updated = e.now()
	_, err := e.write(a, EvPlanSaved, refs{}, p)
	return p.ID, err
}

// RemovePlan removes a response plan.
func (e *Engine) RemovePlan(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.st.Plans[id]; !ok {
		return problem("plan", "no plan %s", id)
	}
	_, err := e.write(a, EvPlanRemoved, refs{}, map[string]string{"id": id})
	return err
}

// SaveGuidance adds or replaces a document in the approved-guidance library.
func (e *Engine) SaveGuidance(a Actor, d GuidanceDoc) error {
	d.ID = strings.TrimSpace(d.ID)
	if probs := guidance.Validate(d); len(probs) > 0 {
		return problem("doc", "%s", strings.Join(probs, "; "))
	}
	for i, c := range d.CallTypes {
		d.CallTypes[i] = strings.ToUpper(strings.TrimSpace(c))
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if d.Approved && d.ApprovedBy == "" {
		d.ApprovedBy, d.ApprovedAt = a.User, e.now()
	}
	if !d.Approved {
		d.ApprovedBy, d.ApprovedAt = "", time.Time{}
	}
	_, err := e.write(a, EvGuidanceSaved, refs{}, d)
	return err
}

// RemoveGuidance removes a document from the library (the ledger keeps every version).
func (e *Engine) RemoveGuidance(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.st.Guidance[id]; !ok {
		return problem("doc", "no document %s", id)
	}
	_, err := e.write(a, EvGuidanceGone, refs{}, map[string]string{"id": id})
	return err
}

// GuidanceShown records which approved passages were shown for an incident; a passage already recorded at
// the same version is not recorded again.
func (e *Engine) GuidanceShown(a Actor, number string, ps []Shown) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	in, ok := e.st.Incidents[number]
	if !ok {
		return problem("incident", "no incident %s", number)
	}
	var fresh []Shown
	for _, p := range ps {
		if !slices.ContainsFunc(in.Guidance, func(x Shown) bool { return x.Doc == p.Doc && x.Version == p.Version && x.Anchor == p.Anchor }) &&
			!slices.ContainsFunc(fresh, func(x Shown) bool { return x.Doc == p.Doc && x.Version == p.Version && x.Anchor == p.Anchor }) {
			p.At, p.By = e.now(), a.User
			fresh = append(fresh, p)
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	_, err := e.write(a, EvGuidanceShown, refs{Incident: number}, map[string][]Shown{"shown": fresh})
	return err
}

// MatchGuidance is the approved guidance for an incident as it stands.
func (s *State) MatchGuidance(number string, now time.Time, limit int) guidance.Result {
	docs := make([]guidance.Doc, 0, len(s.Guidance))
	for _, d := range s.Guidance {
		docs = append(docs, *d)
	}
	cx := guidance.Context{At: now}
	if in, ok := s.Incidents[number]; ok {
		cx.CallType, cx.Modifiers, cx.Priority = in.Type, in.Modifiers, in.Priority
		var b strings.Builder
		for _, t := range in.Timeline {
			if t.Kind == "note" || t.Kind == "priority-note" || t.Kind == "created" {
				b.WriteString(t.Text + "\n")
			}
		}
		for _, id := range in.Calls {
			if c, ok := s.Calls[id]; ok {
				for _, l := range c.Transcript {
					b.WriteString(l.Text + "\n")
				}
				for _, d := range c.Details {
					b.WriteString(d.Text + "\n")
				}
			}
		}
		cx.Text = b.String()
	}
	return guidance.Match(docs, cx, limit)
}

// ---- applying ----

func (s *State) applyEntry(e ledger.Event) (bool, error) {
	s.initEntry()
	if ok, err := s.applyWorkstation(e); ok {
		return true, err
	}
	if ok, err := s.applyRecords(e); ok {
		return true, err
	}
	switch e.Type {
	case EvIncModifiers:
		v, err := decode[map[string][]string](e)
		if err != nil {
			return true, err
		}
		if in, ok := s.Incidents[e.Incident]; ok {
			in.Modifiers = v["modifiers"]
			names := []string{}
			for _, m := range in.Modifiers {
				md, _ := s.Settings.modifierOrDefault(m)
				names = append(names, firstNonEmpty(md.Name, m))
			}
			text := "Modifiers: none"
			if len(names) > 0 {
				text = "Modifiers: " + strings.Join(names, ", ")
			}
			in.add(e, "change", text)
		}
	case EvIncPerson:
		v, err := decode[personData](e)
		if err != nil {
			return true, err
		}
		if in, ok := s.Incidents[e.Incident]; ok {
			p := v.Person
			i := slices.IndexFunc(in.Persons, func(x Person) bool { return x.ID == p.ID })
			verb := "Added"
			if i >= 0 {
				in.Persons[i] = p
				verb = "Changed"
			} else {
				in.Persons = append(in.Persons, p)
			}
			if p.Removed {
				verb = "Removed"
			}
			in.add(e, "person", fmt.Sprintf("%s %s %s", verb, p.Role, firstNonEmpty(p.Name, p.Clothing, p.ID)))
		}
	case EvIncVehicle:
		v, err := decode[vehicleData](e)
		if err != nil {
			return true, err
		}
		if in, ok := s.Incidents[e.Incident]; ok {
			x := v.Vehicle
			i := slices.IndexFunc(in.Vehicles, func(y Vehicle) bool { return y.ID == x.ID })
			verb := "Added"
			if i >= 0 {
				in.Vehicles[i] = x
				verb = "Changed"
			} else {
				in.Vehicles = append(in.Vehicles, x)
			}
			if x.Removed {
				verb = "Removed"
			}
			desc := strings.Join(strings.Fields(strings.Join([]string{x.Color, x.Year, x.Make, x.Model}, " ")), " ")
			if x.Plate != "" {
				desc = strings.TrimSpace(desc + " plate " + strings.TrimSpace(x.State+" "+x.Plate))
			}
			in.add(e, "vehicle", fmt.Sprintf("%s %s vehicle %s", verb, x.Role, firstNonEmpty(desc, x.Description, x.ID)))
		}
	case EvIncCallerAt:
		v, err := decode[map[string]*ng911.Location](e)
		if err != nil {
			return true, err
		}
		if in, ok := s.Incidents[e.Incident]; ok {
			in.CallerAt = v["location"]
			if in.CallerAt == nil {
				in.add(e, "change", "The caller is at the incident")
			} else {
				in.add(e, "change", "The caller is elsewhere: "+locLabel(in.CallerAt))
			}
		}
	case EvIncNotice:
		v, err := decode[noticeData](e)
		if err != nil {
			return true, err
		}
		if in, ok := s.Incidents[e.Incident]; ok {
			n := v.Contact
			i := slices.IndexFunc(in.Notices, func(x Contact) bool { return x.ID == n.ID })
			if i >= 0 {
				in.Notices[i] = n
			} else {
				in.Notices = append(in.Notices, n)
			}
			in.add(e, "notice", noticeText(n, i < 0))
		}
	case EvIncTalkgroup:
		v, err := decode[map[string]string](e)
		if err != nil {
			return true, err
		}
		if in, ok := s.Incidents[e.Incident]; ok {
			in.Talkgroup = v["talkgroup"]
			in.add(e, "change", "Talkgroup "+firstNonEmpty(in.Talkgroup, "cleared"))
		}
	case EvIncVerified:
		if in, ok := s.Incidents[e.Incident]; ok {
			in.LocVerified = true
			in.add(e, "change", "Location verified on the map")
		}
	case EvIncLocOffered:
		v, err := decode[LocOffer](e)
		if err != nil {
			return true, err
		}
		if in, ok := s.Incidents[e.Incident]; ok {
			o := v
			in.NewLocation = &o
			in.add(e, "location", fmt.Sprintf("Newer caller location from %s, %.0f m from the incident's: waiting for the call-taker", firstNonEmpty(v.Location.Source, "the call"), v.Metres))
		}
	case EvIncLocDecided:
		v, err := decode[locDecision](e)
		if err != nil {
			return true, err
		}
		if in, ok := s.Incidents[e.Incident]; ok && in.NewLocation != nil {
			if v.Accept {
				in.Changed = &LocChange{From: in.Location, To: in.NewLocation.Location, At: e.At, By: e.Actor, Why: v.Why}
				in.add(e, "location", fmt.Sprintf("Location changed to %s (was %s)%s", locLabel(in.NewLocation.Location), locLabel(in.Location), suffix(v.Why)))
				in.Location = in.NewLocation.Location
				in.LocVerified = false
			} else {
				in.add(e, "location", "Kept the incident's location; the newer caller location was not used"+suffix(v.Why))
			}
			in.NewLocation = nil
		}
	case EvUnitAcked:
		v, _ := decode[map[string]string](e)
		if in, ok := s.Incidents[e.Incident]; ok {
			for i := range in.Units {
				if in.Units[i].Unit == e.Unit && in.Units[i].Cleared.IsZero() && in.Units[i].Acked.IsZero() {
					in.Units[i].Acked, in.Units[i].AckedBy = e.At, e.Actor
				}
			}
			in.add(e, "status", e.Unit+" acknowledged"+suffix(v["how"]))
		}
	case EvBOLOSaved:
		v, err := decode[BOLO](e)
		if err != nil {
			return true, err
		}
		b := v
		_, existed := s.BOLOs[b.ID]
		s.BOLOs[b.ID] = &b
		if in, ok := s.Incidents[b.Incident]; ok {
			if existed {
				in.add(e, "bolo", b.ID+" updated")
			} else {
				in.add(e, "bolo", b.ID+" issued: "+b.Text)
			}
		}
	case EvBOLOCancelled:
		v, _ := decode[map[string]string](e)
		if b, ok := s.BOLOs[v["id"]]; ok {
			b.Cancelled, b.CancelledBy = e.At, e.Actor
			if in, ok := s.Incidents[b.Incident]; ok {
				in.add(e, "bolo", b.ID+" cancelled"+suffix(v["why"]))
			}
		}
	case EvFacilitySaved:
		v, err := decode[Facility](e)
		if err != nil {
			return true, err
		}
		f := v
		s.Facilities[f.ID] = &f
	case EvFacilityGone:
		v, _ := decode[map[string]string](e)
		delete(s.Facilities, v["id"])
	case EvFacilityStatus:
		v, err := decode[Facility](e)
		if err != nil {
			return true, err
		}
		if f, ok := s.Facilities[v.ID]; ok {
			f.Status, f.Note, f.Until, f.Updated, f.By = v.Status, v.Note, v.Until, e.At, e.Actor
		}
	case EvPassdown:
		v, err := decode[Passdown](e)
		if err != nil {
			return true, err
		}
		s.Passdown = append(s.Passdown, v)
	case EvPlanSaved:
		v, err := decode[ResponsePlan](e)
		if err != nil {
			return true, err
		}
		p := v
		s.Plans[p.ID] = &p
	case EvPlanRemoved:
		v, _ := decode[map[string]string](e)
		delete(s.Plans, v["id"])
	case EvGuidanceSaved:
		v, err := decode[GuidanceDoc](e)
		if err != nil {
			return true, err
		}
		d := v
		s.Guidance[d.ID] = &d
	case EvGuidanceGone:
		v, _ := decode[map[string]string](e)
		delete(s.Guidance, v["id"])
	case EvGuidanceShown:
		v, err := decode[map[string][]Shown](e)
		if err != nil {
			return true, err
		}
		if in, ok := s.Incidents[e.Incident]; ok {
			in.Guidance = append(in.Guidance, v["shown"]...)
		}
	case EvLocked, EvUnlocked, EvReviewExported, EvConfigStaged, EvWorkstationCode, EvUserDuties, EvUserMFAReset, EvRecordsViewed, EvTrainingInject:
		// Recorded for the audit; they change nothing on the board.
	default:
		return false, nil
	}
	return true, nil
}

func suffix(why string) string {
	if strings.TrimSpace(why) == "" {
		return ""
	}
	return ": " + strings.TrimSpace(why)
}

func locLabel(l *ng911.Location) string {
	if l == nil {
		return "no location"
	}
	if l.Address != "" {
		return l.Address
	}
	if l.HasPoint() {
		return fmt.Sprintf("%.5f, %.5f", *l.Lat, *l.Lon)
	}
	return "no location"
}

func noticeText(n Contact, added bool) string {
	what := map[string]string{"notify": "Notify", "mutualaid": "Mutual aid", "resource": "Resource request", "transfer": "Transfer",
		"referral": "Referral", "outbound": "Outbound call", "instructions": "Instructions"}[n.Kind]
	t := what + " " + n.Who
	if n.Detail != "" {
		t += " (" + n.Detail + ")"
	}
	if !added || n.Status != "pending" {
		t += ": " + n.Status
	}
	return t
}

// afterEntry offers a newer caller location to the incidents a call is on, when it is far enough from the
// incident's own: rebids, supplemental location providers and new ALI all arrive as call updates.
func (s *State) offerLocation(e ledger.Event) (number string, offer *LocOffer) {
	if e.Type != EvCallUpdated || e.Call == "" {
		return "", nil
	}
	c, ok := s.Calls[e.Call]
	if !ok || c.Incident == "" || !c.Location.HasPoint() {
		return "", nil
	}
	in, ok := s.Incidents[c.Incident]
	if !ok || in.Status == Closed || !in.Location.HasPoint() {
		return "", nil
	}
	// Only a location newer than the incident's, and newer than any already offered.
	if !c.Location.At.IsZero() && !in.Location.At.IsZero() && !c.Location.At.After(in.Location.At) {
		return "", nil
	}
	if in.NewLocation != nil && in.NewLocation.Location.HasPoint() &&
		geo.Distance(*in.NewLocation.Location.Lat, *in.NewLocation.Location.Lon, *c.Location.Lat, *c.Location.Lon) < 5 {
		return "", nil
	}
	d := geo.Distance(*c.Location.Lat, *c.Location.Lon, *in.Location.Lat, *in.Location.Lon)
	if d < s.Settings.MoveRule() {
		return "", nil
	}
	loc := *c.Location
	return in.Number, &LocOffer{Location: &loc, Call: c.ID, Metres: d, At: e.At}
}

// ---- reading ----

// ActiveBOLOs are the BOLOs still out, newest first.
func (s *State) ActiveBOLOs(now time.Time) []*BOLO {
	out := []*BOLO{}
	for _, b := range s.BOLOs {
		if b.Active(now) {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Issued.After(out[j].Issued) })
	return out
}

// PlanFor is the response plan that applies to an incident now, if any: the most specific (addresses,
// then modifiers, then area, then call types).
func (s *State) PlanFor(in *Incident, now time.Time) *ResponsePlan {
	var best *ResponsePlan
	bestScore := -1
	addr := ""
	if in.Location != nil {
		addr = strings.ToUpper(in.Location.Address)
	}
	area := s.firstDueStation(in)
	ids := make([]string, 0, len(s.Plans))
	for id := range s.Plans {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		p := s.Plans[id]
		score := 0
		if len(p.CallTypes) > 0 {
			if !slices.Contains(p.CallTypes, in.Type) {
				continue
			}
			score++
		}
		if len(p.Modifiers) > 0 {
			ok := true
			for _, m := range p.Modifiers {
				if !slices.Contains(in.Modifiers, m) {
					ok = false
				}
			}
			if !ok {
				continue
			}
			score += 4
		}
		if p.Area != "" {
			if !strings.EqualFold(p.Area, area) {
				continue
			}
			score += 2
		}
		if len(p.Addresses) > 0 {
			hit := false
			for _, a := range p.Addresses {
				if a = strings.ToUpper(strings.TrimSpace(a)); a != "" && strings.HasPrefix(addr, a) {
					hit = true
				}
			}
			if !hit {
				continue
			}
			score += 8
		}
		if p.From != "" && p.To != "" && !inWindow(now, p.From, p.To) {
			continue
		}
		if score > bestScore {
			best, bestScore = p, score
		}
	}
	return best
}

// firstDueStation is the station nearest an incident in a straight line: its first-due area.
func (s *State) firstDueStation(in *Incident) string {
	if !in.Location.HasPoint() {
		return ""
	}
	best, bd := "", 0.0
	for _, st := range s.Stations {
		d := geo.Distance(*in.Location.Lat, *in.Location.Lon, st.Lat, st.Lon)
		if best == "" || d < bd {
			best, bd = st.ID, d
		}
	}
	return best
}

func inWindow(now time.Time, from, to string) bool {
	f, err1 := time.Parse("15:04", from)
	t, err2 := time.Parse("15:04", to)
	if err1 != nil || err2 != nil {
		return true
	}
	m := now.Local().Hour()*60 + now.Local().Minute()
	fm, tm := f.Hour()*60+f.Minute(), t.Hour()*60+t.Minute()
	if fm <= tm {
		return m >= fm && m < tm
	}
	return m >= fm || m < tm // across midnight
}

// NeedReason asks why, when units other than the recommendation are sent and the agency wants a reason.
func (e *Engine) NeedReason(number string, units, recommended []string, why string) error {
	var off bool
	e.mu.Lock()
	off = e.st.Settings.OverrideReasonOff
	e.mu.Unlock()
	if off || strings.TrimSpace(why) != "" || len(recommended) == 0 {
		return nil
	}
	for _, u := range units {
		if !slices.Contains(recommended, strings.ToUpper(u)) {
			return problem("reason", "%s is not recommended: say why (D %s / closer by radio)", strings.ToUpper(u), strings.Join(units, " "))
		}
	}
	return nil
}

// ---- workstations ----

// EvWorkstation and friends record the agency's workstations and the browsers enrolled as them.
const (
	EvWorkstationSaved   = "workstation.saved"
	EvWorkstationRemoved = "workstation.removed"
	EvWorkstationEnrol   = "workstation.enrolled"
	EvWorkstationRevoke  = "workstation.revoked"
)

// Workstation is a console position the agency has set up: a browser enrolled as it signs in at it, so
// nobody is asked which console they are at.
type Workstation struct {
	ID       string      `json:"id"` // the position name: CT-3, FIRE-1
	Name     string      `json:"name,omitempty"`
	Duties   []string    `json:"duties,omitempty"` // the duties worked here, offered first
	Enrolled []Enrolment `json:"enrolled,omitempty"`
}

// Enrolment is one browser enrolled as a workstation, by a hash of its token (never the token).
type Enrolment struct {
	Hash  string    `json:"hash"`
	At    time.Time `json:"at"`
	By    string    `json:"by"`
	Label string    `json:"label,omitempty"` // the browser, as the enrolling person saw it
}

// SaveWorkstation adds or changes a workstation.
func (e *Engine) SaveWorkstation(a Actor, w Workstation) error {
	w.ID = strings.ToUpper(strings.TrimSpace(w.ID))
	if w.ID == "" || len(w.ID) > 24 || strings.ContainsAny(w.ID, " /#") {
		return problem("id", "a workstation is a short name without spaces: CT-3, FIRE-1")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if old, ok := e.st.Workstations[w.ID]; ok {
		w.Enrolled = old.Enrolled
	} else {
		w.Enrolled = nil
	}
	_, err := e.write(a, EvWorkstationSaved, refs{}, w)
	return err
}

// RemoveWorkstation removes a workstation and every browser enrolled as it.
func (e *Engine) RemoveWorkstation(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.st.Workstations[id]; !ok {
		return problem("workstation", "no workstation %s", id)
	}
	_, err := e.write(a, EvWorkstationRemoved, refs{}, map[string]string{"id": id})
	return err
}

// EnrolWorkstation records a browser enrolled as a workstation, by its token's hash.
func (e *Engine) EnrolWorkstation(a Actor, id, hash, label string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.st.Workstations[id]; !ok {
		return problem("workstation", "no workstation %s", id)
	}
	_, err := e.write(a, EvWorkstationEnrol, refs{}, map[string]any{"id": id, "enrolment": Enrolment{Hash: hash, At: e.now(), By: a.User, Label: label}})
	return err
}

// RevokeEnrolment stops a browser signing in as a workstation.
func (e *Engine) RevokeEnrolment(a Actor, id, hash string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.st.Workstations[id]; !ok {
		return problem("workstation", "no workstation %s", id)
	}
	_, err := e.write(a, EvWorkstationRevoke, refs{}, map[string]string{"id": id, "hash": hash})
	return err
}

// WorkstationByHash is the workstation a browser token's hash is enrolled as.
func (s *State) WorkstationByHash(hash string) *Workstation {
	if hash == "" {
		return nil
	}
	for _, w := range s.Workstations {
		for _, en := range w.Enrolled {
			if en.Hash == hash {
				return w
			}
		}
	}
	return nil
}

func (s *State) applyWorkstation(e ledger.Event) (bool, error) {
	if s.Workstations == nil {
		s.Workstations = map[string]*Workstation{}
	}
	switch e.Type {
	case EvWorkstationSaved:
		v, err := decode[Workstation](e)
		if err != nil {
			return true, err
		}
		w := v
		s.Workstations[w.ID] = &w
	case EvWorkstationRemoved:
		v, _ := decode[map[string]string](e)
		delete(s.Workstations, v["id"])
	case EvWorkstationEnrol:
		v, err := decode[struct {
			ID        string    `json:"id"`
			Enrolment Enrolment `json:"enrolment"`
		}](e)
		if err != nil {
			return true, err
		}
		if w, ok := s.Workstations[v.ID]; ok {
			w.Enrolled = append(w.Enrolled, v.Enrolment)
		}
	case EvWorkstationRevoke:
		v, _ := decode[map[string]string](e)
		if w, ok := s.Workstations[v["id"]]; ok {
			w.Enrolled = slices.DeleteFunc(w.Enrolled, func(x Enrolment) bool { return x.Hash == v["hash"] })
		}
	default:
		return false, nil
	}
	return true, nil
}
