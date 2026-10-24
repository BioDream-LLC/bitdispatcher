// Package cad is bitdispatcher's dispatch core: incidents, units, calls, stations, call types and
// settings, all kept as a projection of the event ledger. Every change is a command that validates, writes
// one or more ledger events and applies them; state is never changed any other way, so replaying the
// ledger rebuilds it exactly.
package cad

import (
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/acn"
	"github.com/biodream-llc/bitdispatcher/internal/assist"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// Unit statuses.
const (
	Available     = "available"
	Dispatched    = "dispatched"
	Enroute       = "enroute"
	OnScene       = "onscene"
	Transporting  = "transporting"
	AtDestination = "atdestination"
	MarkedOut     = "markout" // out on an activity of its own: a meal, court, fuel, a report, a follow-up
	OutOfService  = "oos"
)

// Statuses lists every unit status in working order.
var Statuses = []string{Available, Dispatched, Enroute, OnScene, Transporting, AtDestination, MarkedOut, OutOfService}

// statusWords are the command-line words for each status.
var statusWords = map[string]string{"ER": Enroute, "ENRT": Enroute, "OS": OnScene, "ONS": OnScene, "TR": Transporting,
	"AD": AtDestination, "AH": AtDestination, "AV": Available, "AVL": Available, "CLR": Available, "OOS": OutOfService}

// Committed reports whether a status means the unit is working an incident.
func Committed(s string) bool {
	switch s {
	case Dispatched, Enroute, OnScene, Transporting, AtDestination:
		return true
	}
	return false
}

// Incident statuses.
const (
	Pending = "pending" // no unit assigned yet
	Active  = "active"  // at least one unit assigned and not cleared
	Closed  = "closed"
)

// Station is a place units are based.
type Station struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Address string  `json:"address,omitempty"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	Agency  string  `json:"agency,omitempty"` // the agency that runs it (agency.go)
}

// UnitType is a kind of unit, with the capabilities a unit of that kind has unless it says otherwise.
type UnitType struct {
	Code         string   `json:"code"` // engine, ladder, medic, police...
	Name         string   `json:"name"`
	Discipline   string   `json:"discipline"` // fire, ems, police, other
	Capabilities []string `json:"capabilities"`
	// Cover asks for move-up suggestions when a station has none of this kind available. Unset means the
	// usual: front-line engines, ladders, medics, ambulances and patrol cars, not chiefs or specialty units.
	Cover *bool `json:"cover,omitempty"`
	// MinStaff is how many a unit of this kind needs aboard to be fully staffed (0: no minimum).
	MinStaff int `json:"minStaff,omitempty"`
	// Positions are the crew positions that must be filled (driver, officer…); SignOn is how its crew
	// signs on: self (from the terminal), approve (a supervisor approves) or dispatch (staffing.go).
	Positions []string `json:"positions,omitempty"`
	SignOn    string   `json:"signOn,omitempty"`
}

var defaultCover = map[string]bool{"engine": true, "ladder": true, "medic": true, "ambulance": true, "patrol": true}

// Covered reports whether stations should keep this kind of unit available.
func (t UnitType) Covered() bool {
	if t.Cover != nil {
		return *t.Cover
	}
	return defaultCover[t.Code]
}

// UnitDef is how a unit is configured.
type UnitDef struct {
	ID           string   `json:"id"` // the call sign, upper case: E1, M12, 2A31
	Name         string   `json:"name,omitempty"`
	Type         string   `json:"type"`
	Station      string   `json:"station,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"` // in addition to the type's
	RadioID      int      `json:"radioId,omitempty"`      // the radio's unit ID (P25 decimal)
	RadioAlias   string   `json:"radioAlias,omitempty"`
	Agency       string   `json:"agency,omitempty"`
	// Area is the unit's own beat, sector or district, for police backup and response plans.
	Area string `json:"area,omitempty"`
	// MinStaff overrides the unit type's minimum staffing (0: the type's).
	MinStaff int `json:"minStaff,omitempty"`
}

// Position is where a unit is, by AVL or radio.
type Position struct {
	Lat      float64   `json:"lat"`
	Lon      float64   `json:"lon"`
	Accuracy float64   `json:"accuracy,omitempty"`
	Speed    float64   `json:"speed,omitempty"`
	Heading  float64   `json:"heading,omitempty"`
	Source   string    `json:"source"`
	At       time.Time `json:"at"`
}

// Unit is a unit and its live state.
type Unit struct {
	UnitDef
	Status    string    `json:"status"`
	StatusAt  time.Time `json:"statusAt"`
	Incident  string    `json:"incident,omitempty"`
	Position  *Position `json:"position,omitempty"`
	LastHeard time.Time `json:"lastHeard,omitzero"` // last radio transmission
	Emergency string    `json:"emergency,omitempty"`
	Covering  string    `json:"covering,omitempty"` // the station it is posted at for a move-up
	// MarkOut is what a marked-out unit is doing and where.
	MarkOut *MarkOut `json:"markOut,omitempty"`
	// Checked is the last status check: a dispatcher confirmed the unit is all right, restarting its timer.
	Checked   time.Time `json:"checked,omitzero"`
	CheckedBy string    `json:"checkedBy,omitempty"`
	// Crew is who rides the unit this shift (crew.go); empty when staffing is not recorded.
	Crew []CrewSeat `json:"crew,omitempty"`
	// StaffOverride is a supervisor's reason for keeping the unit in service short-staffed.
	StaffOverride string `json:"staffOverride,omitempty"`
	crewKnown     bool   // its crew has been recorded
}

// MarkOut is a unit's own activity: its kind (a mark-out type's code), where, and a note.
type MarkOut struct {
	Code     string          `json:"code"`
	Location *ng911.Location `json:"location,omitempty"`
	Note     string          `json:"note,omitempty"`
}

// MarkOutType is a kind of mark-out the agency uses, with its own status-check time.
type MarkOutType struct {
	Code       string `json:"code"` // MEAL, COURT...: what the command line takes
	Name       string `json:"name"`
	Minutes    int    `json:"minutes"`              // status check after; 0 for none
	Discipline string `json:"discipline,omitempty"` // only for units of this discipline; empty for any
	// Interruptible units are offered by the recommendation when no available unit can fill a line.
	Interruptible bool `json:"interruptible,omitempty"`
	// Retired codes are refused (and the attempt recorded); kept so old records still read.
	Retired bool `json:"retired,omitempty"`
	// Roles that may use it (empty: everyone who may change units); MaxMinutes is the longest it may last
	// before a supervisor is told (0: no limit); Staffing: the unit counts as unavailable for coverage.
	Roles      []string `json:"roles,omitempty"`
	MaxMinutes int      `json:"maxMinutes,omitempty"`
	Staffing   bool     `json:"staffing,omitempty"`
}

// Requirement is one line of a run card: this many units with this capability.
type Requirement struct {
	Capability string `json:"capability"`
	Count      int    `json:"count"`
}

// CallType is an incident type and its run card.
type CallType struct {
	Code       string        `json:"code"`
	Name       string        `json:"name"`
	Priority   int           `json:"priority"` // 1 highest .. 5
	Discipline string        `json:"discipline"`
	RunCard    []Requirement `json:"runCard"`
	// Critical marks calls that are hard on the people who work them, for the supervisor's care list.
	Critical bool `json:"critical,omitempty"`
	// Drone asks the drone system to launch for this call type, when a drone system is configured.
	Drone bool `json:"drone,omitempty"`
	// Notify are the people or agencies to tell for this call type (a fire marshal, animal control),
	// listed on every incident of the type as a checklist.
	Notify []string `json:"notify,omitempty"`
	// Group is the agency's own heading for the type in long lists (Medical, Traffic, Property...).
	Group string `json:"group,omitempty"`
	// AutoDispatch sends the recommended units by itself when no dispatcher has within
	// Settings.AutoDispatchSeconds: only for priority 3 to 5, and only a complete recommendation without warnings.
	AutoDispatch bool `json:"autoDispatch,omitempty"`
	// Alarms are the further alarms (2nd, 3rd…), each adding units to the run card (alarm.go).
	Alarms []AlarmLevel `json:"alarms,omitempty"`
	// PAR starts the fire-ground accountability clock when the first unit arrives: roll calls every
	// PARMinutes (0: Settings.PARMinutes) and the incident's elapsed time (fireground.go).
	PAR        bool `json:"par,omitempty"`
	PARMinutes int  `json:"parMinutes,omitempty"`
	// NEMSIS is the national EMS dispatch reason (eDispatch.01) the type is sent as to patient-care
	// reports; empty: suggested from the name.
	NEMSIS string `json:"nemsis,omitempty"`
	// Agencies that use the type (agency.go); empty: every agency.
	Agencies []string `json:"agencies,omitempty"`
	// CheckMinutes is the status-check (safety) timer for units on scene of this type (0: the status's).
	CheckMinutes int `json:"checkMinutes,omitempty"`
}

// Caller is who reported an incident.
type Caller struct {
	Name     string `json:"name,omitempty"`
	Callback string `json:"callback,omitempty"`
}

// Assignment is one unit on an incident.
type Assignment struct {
	Unit        string    `json:"unit"`
	Status      string    `json:"status"`
	Recommended bool      `json:"recommended"`
	Dispatched  time.Time `json:"dispatched"`
	Enroute     time.Time `json:"enroute,omitzero"`
	OnScene     time.Time `json:"onScene,omitzero"`
	Transport   time.Time `json:"transport,omitzero"`
	// Acked is when the unit acknowledged the assignment (by radio, the mobile terminal or the dispatcher).
	Acked   time.Time `json:"acked,omitzero"`
	AckedBy string    `json:"ackedBy,omitempty"`
	AtDest  time.Time `json:"atDest,omitzero"`
	Cleared time.Time `json:"cleared,omitzero"`
	// Crew is who was aboard when the unit was assigned, with what they were qualified for then
	// (staffing.go); CrewCorrected marks a supervisor's correction (the original is in the timeline).
	Crew          []CrewSeat `json:"crew,omitempty"`
	CrewCorrected bool       `json:"crewCorrected,omitempty"`
	// Override is a supervisor's reason for sending it short of what the call needs.
	Override string `json:"override,omitempty"`
}

// Entry is one line of an incident's timeline.
type Entry struct {
	Seq      uint64    `json:"seq"`
	At       time.Time `json:"at"`
	Actor    string    `json:"actor"`
	Kind     string    `json:"kind"` // created, note, priority-note, change, dispatch, status, call, close, reopen
	Text     string    `json:"text"`
	Unit     string    `json:"unit,omitempty"`
	Priority bool      `json:"priority,omitempty"`
}

// Incident is one incident.
type Incident struct {
	Number      string          `json:"number"`
	Type        string          `json:"type"`
	Priority    int             `json:"priority"`
	Status      string          `json:"status"`
	Location    *ng911.Location `json:"location,omitempty"`
	Caller      *Caller         `json:"caller,omitempty"`
	Calls       []string        `json:"calls"`
	Units       []Assignment    `json:"units"`
	Timeline    []Entry         `json:"timeline"`
	Created     time.Time       `json:"created"`
	CreatedBy   string          `json:"createdBy"`
	Updated     time.Time       `json:"updated"`
	Closed      time.Time       `json:"closed,omitzero"`
	Disposition string          `json:"disposition,omitempty"`
	Reports     []string        `json:"reports,omitempty"` // incident reports written for it
	// Call entry: modifiers, people and vehicles, where the caller is when not at the incident, the
	// checklist of notifications and logged contacts, the talkgroup, and a newer caller location waiting
	// for the call-taker to accept it.
	Modifiers   []string        `json:"modifiers,omitempty"`
	Persons     []Person        `json:"persons,omitempty"`
	Vehicles    []Vehicle       `json:"vehicles,omitempty"`
	CallerAt    *ng911.Location `json:"callerAt,omitempty"`
	Notices     []Contact       `json:"notices,omitempty"`
	Talkgroup   string          `json:"talkgroup,omitempty"`
	LocVerified bool            `json:"locVerified,omitempty"` // the call-taker confirmed the address on the map
	NewLocation *LocOffer       `json:"newLocation,omitempty"`
	Guidance    []Shown         `json:"guidance,omitempty"` // approved guidance shown for it, by version
	Changed     *LocChange      `json:"changed,omitempty"`  // the last accepted location change, for dispatchers
	NERIS       *NERISRecord    `json:"neris,omitempty"`    // what NERIS last said about it (fire and EMS)
	Protocol    *ProtocolRun    `json:"protocol,omitempty"` // the call-taking protocol worked on it
	Files       []IncidentFile  `json:"files,omitempty"`    // attachments: pictures, documents
	Alarm       int             `json:"alarm,omitempty"`    // the alarm level; 0 or 1 is the first alarm
	Fireground  *Fireground     `json:"fireground,omitempty"`
	// ePCR is what each EMS unit's patient-care report system last said (nemsis), by unit.
	EPCR map[string]*EPCRRecord `json:"epcr,omitempty"`
	// Agency is the primary agency (agency.go); Assisting the agencies named as assisting (every unit
	// also keeps its own home agency).
	Agency    string   `json:"agency,omitempty"`
	Assisting []string `json:"assisting,omitempty"`
}

// TextLine is one line of a text conversation with a caller.
type TextLine struct {
	At   time.Time `json:"at"`
	From string    `json:"from"` // caller or taker
	Name string    `json:"name,omitempty"`
	Via  string    `json:"via"`
	Text string    `json:"text"`
}

// Video is a caller's video in call handling.
type Video struct {
	Live    bool   `json:"live"`
	ViewURL string `json:"viewUrl,omitempty"` // WHEP, for CAD and units
	Source  string `json:"source,omitempty"`
}

// CallData is what a source says about a call. Empty fields mean "not said", never "cleared".
type CallData struct {
	Source string `json:"source"` // eido, ali, bitvoice
	// System is which BitVoice the call is on, when bitdispatcher talks to more than one: "" is the
	// main one, else the other's id (Integrations, More BitVoice systems).
	System     string          `json:"system,omitempty"`
	State      string          `json:"state,omitempty"`
	Callback   string          `json:"callback,omitempty"`
	CallerName string          `json:"callerName,omitempty"`
	Class      string          `json:"class,omitempty"`
	Provider   string          `json:"provider,omitempty"`
	Location   *ng911.Location `json:"location,omitempty"`
	Agent      string          `json:"agent,omitempty"`
	Position   string          `json:"position,omitempty"`
	Start      time.Time       `json:"start,omitzero"`
	Answer     time.Time       `json:"answer,omitzero"`
	Media      []ng911.Media   `json:"media,omitempty"`
	Video      *Video          `json:"video,omitempty"`
	Text       []TextLine      `json:"text,omitempty"`
	TextOnly   bool            `json:"textOnly,omitempty"`
	Notes      []string        `json:"notes,omitempty"`
	CanRebid   bool            `json:"canRebid,omitempty"`
	Problems   []string        `json:"problems,omitempty"`
	ALI        string          `json:"ali,omitempty"` // the raw spill
	Emergency  bool            `json:"emergency,omitempty"`
	// Transcript is new lines of what was said, added to those already known (not a replacement).
	Transcript   []TranscriptLine  `json:"transcript,omitempty"`
	Translations []LineTranslation `json:"translations,omitempty"`
	Line         string            `json:"line,omitempty"` // the line it came in on: 911, or a non-emergency number
	Crash        *acn.Crash        `json:"crash,omitempty"`
	Alert        *Alert            `json:"alert,omitempty"` // an alarm, plate-reader hit, gunshot or camera alert
	Handling     *Handling         `json:"handling,omitempty"`
}

// Call is a call from call handling, as bitdispatcher knows it from every source.
type Call struct {
	ID         string           `json:"id"`
	Sources    []string         `json:"sources"`
	System     string           `json:"system,omitempty"` // which BitVoice it is on ("" the main one)
	State      string           `json:"state"`            // ringing, answered, ended
	Callback   string           `json:"callback,omitempty"`
	CallerName string           `json:"callerName,omitempty"`
	Class      string           `json:"class,omitempty"`
	Provider   string           `json:"provider,omitempty"`
	Location   *ng911.Location  `json:"location,omitempty"`
	Locations  int              `json:"locations"` // how many locations have been received
	Agent      string           `json:"agent,omitempty"`
	Position   string           `json:"position,omitempty"`
	Start      time.Time        `json:"start"`
	Answer     time.Time        `json:"answer,omitzero"`
	End        time.Time        `json:"end,omitzero"`
	Media      []ng911.Media    `json:"media,omitempty"`
	Video      *Video           `json:"video,omitempty"`
	Text       []TextLine       `json:"text,omitempty"`
	TextOnly   bool             `json:"textOnly,omitempty"`
	Notes      []string         `json:"notes,omitempty"`
	CanRebid   bool             `json:"canRebid,omitempty"`
	Problems   []string         `json:"problems,omitempty"`
	ALI        string           `json:"ali,omitempty"`
	Incident   string           `json:"incident,omitempty"`
	Updated    time.Time        `json:"updated"`
	Transcript []TranscriptLine `json:"transcript,omitempty"`
	Lang       string           `json:"lang,omitempty"` // the caller's language, when not English
	Details    []assist.Detail  `json:"details,omitempty"`
	Line       string           `json:"line,omitempty"`
	Crash      *acn.Crash       `json:"crash,omitempty"`
	CrashRisk  *acn.Assessment  `json:"crashRisk,omitempty"`
	Alert      *Alert           `json:"alert,omitempty"`
	Fixes      []Fix            `json:"fixes,omitempty"`
	Screen     *Screen          `json:"screen,omitempty"`
	Handling   *Handling        `json:"handling,omitempty"` // what the call-handling system knows beyond the basics
}

// Emergency is an emergency button or duress alert from a radio, open until a person acknowledges it.
type Emergency struct {
	Key         string    `json:"key"`
	Unit        string    `json:"unit,omitempty"`   // the bitdispatcher unit, when the radio is mapped to one
	System      string    `json:"system,omitempty"` // which BitVoice's radio it came from ("" the main one)
	RadioUnit   string    `json:"radioUnit,omitempty"`
	RadioID     int       `json:"radioId,omitempty"`
	Alias       string    `json:"alias,omitempty"`
	Channel     string    `json:"channel,omitempty"`
	ChannelName string    `json:"channelName,omitempty"`
	Raised      time.Time `json:"raised"`
	AckedBy     string    `json:"ackedBy,omitempty"`
	AckedAt     time.Time `json:"ackedAt,omitzero"`
	AckedWhere  string    `json:"ackedWhere,omitempty"` // bitdispatcher or bitvoice
}

// Settings are the agency's operating settings.
type Settings struct {
	Agency          string         `json:"agency"`
	IncidentPrefix  string         `json:"incidentPrefix"`
	Timers          map[string]int `json:"timers"` // minutes in a status before the dispatcher is prompted
	AVLStaleSeconds int            `json:"avlStaleSeconds"`
	LocStaleSeconds int            `json:"locationStaleSeconds"` // a caller location older than this is shown as stale
	RoadFactor      float64        `json:"roadFactor"`
	SpeedKmh        float64        `json:"speedKmh"`
	MapTiles        string         `json:"mapTiles"` // raster tile URL template, {z}/{x}/{y}; empty for none
	MapAttribution  string         `json:"mapAttribution"`
	MapCenter       [2]float64     `json:"mapCenter"` // lat, lon
	MapZoom         float64        `json:"mapZoom"`
	// PhotosRecordedOnly stops the server fetching aerial photos it has not recorded when a console asks.
	PhotosRecordedOnly bool     `json:"photosRecordedOnly,omitempty"`
	Dispositions       []string `json:"dispositions"`
	// SignInNotice is the system use notice on the sign-in page (CJIS AC-8): who may use the system, that
	// use is monitored and recorded.
	SignInNotice string `json:"signInNotice,omitempty"`
	// AutoDispatchSeconds is how long an auto-dispatch call waits for a dispatcher (0: 60 seconds).
	AutoDispatchSeconds int           `json:"autoDispatchSeconds,omitempty"`
	Shifts              []string      `json:"shifts,omitempty"`   // shift change times, "07:00", for reports
	ORI                 string        `json:"ori,omitempty"`      // the agency's originating agency identifier, for NCIC
	NCICMode            string        `json:"ncicMode,omitempty"` // off, sim (training simulator) or line (a switch adapter)
	NCICAddr            string        `json:"ncicAddr,omitempty"` // host:port of the switch adapter
	UnitTypes           []UnitType    `json:"unitTypes"`
	MarkOuts            []MarkOutType `json:"markOuts"`

	// NonEmergencyLines are the numbers or line names of non-emergency lines; calls on them go to the
	// non-emergency list. NonEmergencyWords are what callers say on calls that seldom are emergencies;
	// empty means the defaults. NonEmergencyReply is the text offered to send such a caller.
	NonEmergencyLines []string `json:"nonEmergencyLines,omitempty"`
	NonEmergencyWords []string `json:"nonEmergencyWords,omitempty"`
	NonEmergencyReply string   `json:"nonEmergencyReply,omitempty"`
	// CriticalWords mark an incident as a critical call (empty: the defaults); CareOff stops the words
	// (critical call types still count). SupportLine is the employee assistance number shown after one.
	CriticalWords []string `json:"criticalWords,omitempty"`
	CareOff       bool     `json:"careOff,omitempty"`
	SupportLine   string   `json:"supportLine,omitempty"`
	BreakMinutes  int      `json:"breakMinutes,omitempty"` // worked this long without a break: suggest one (0: 120)
	// Crash data thresholds (0: the defaults, 40 and 20 km/h of delta-V).
	CrashHighKmh   float64 `json:"crashHighKmh,omitempty"`
	CrashMediumKmh float64 `json:"crashMediumKmh,omitempty"`
	// TDoSPerMinute is the calls a minute that raise the denial-of-service warning (0: five times the usual, at least 12).
	TDoSPerMinute int `json:"tdosPerMinute,omitempty"`
	// Calls this near each other within this time are shown as one group (0: 300 m, 10 minutes).
	ClusterMeters  float64 `json:"clusterMeters,omitempty"`
	ClusterMinutes int     `json:"clusterMinutes,omitempty"`
	QA             QA      `json:"qa,omitzero"`

	// Call entry and the work of each role.
	Modifiers  []Modifier `json:"modifiers,omitempty"`  // the agency's incident modifiers (weapons, in progress...)
	Duties     DutyRules  `json:"duties,omitzero"`      // what each role may do
	Security   Security   `json:"security,omitzero"`    // sign-in, MFA and lock policy
	AckSeconds int        `json:"ackSeconds,omitempty"` // a dispatched unit not acknowledged after this is flagged (0: 60)
	// OverrideReasonOff stops asking dispatchers why they sent other units than recommended.
	OverrideReasonOff bool     `json:"overrideReasonOff,omitempty"`
	Talkgroups        []string `json:"talkgroups,omitempty"`
	MoveMeters        float64  `json:"moveMeters,omitempty"` // a newer caller location this far away is offered (0: 50 m)
	FOIADays          int      `json:"foiaDays,omitempty"`   // working days to answer a records request (0: 5)

	// Crews and staffing: the qualifications the roster uses, and what each capability needs of a crew
	// (nil: advanced life support needs a paramedic).
	Qualifications []string  `json:"qualifications,omitempty"`
	CapNeeds       []CapNeed `json:"capNeeds"`
	// Fire-ground accountability: minutes between roll calls (0: 20) and the benchmarks a commander
	// reports (empty: the defaults).
	PARMinutes int         `json:"parMinutes,omitempty"`
	Benchmarks []Benchmark `json:"benchmarks,omitempty"`
	// Tow rotation: the classes of tow and the rotation zones (empty: light, medium, heavy and one zone).
	TowClasses []string `json:"towClasses,omitempty"`
	TowZones   []string `json:"towZones,omitempty"`
	// Views says who may see each supervisory view (presets.go); a view with no rule is for supervisors.
	Views []ViewRule `json:"views,omitempty"`
	// Staffing is the agency's staffing policy (staffing.go).
	Staffing StaffingRules `json:"staffing,omitzero"`
	// Communications (comms.go): the kinds, how each priority is presented, and attachments allowed.
	CommKinds     []CommKind     `json:"commKinds,omitempty"`
	PriorityRules []PriorityRule `json:"priorityRules,omitempty"`
	CommFileTypes []string       `json:"commFileTypes,omitempty"` // MIME types allowed (empty: pictures, PDF, text, audio, video)
	CommFileMB    int            `json:"commFileMB,omitempty"`    // largest attachment (0: 25 MB)
	// MultiCall is how many working calls people may keep (calldrafts.go).
	MultiCall MultiCall `json:"multiCall,omitzero"`
	// Commands are the command-line grammar, status codes and key profiles (fieldcmd.go).
	Commands CommandRules `json:"commands,omitzero"`
}

// MarkOutType finds a mark-out type by its code, in any case.
func (s Settings) MarkOutType(code string) (MarkOutType, bool) {
	for _, t := range s.MarkOuts {
		if strings.EqualFold(t.Code, code) {
			return t, true
		}
	}
	return MarkOutType{}, false
}

// TimerFor is the minutes a unit may stay in a status (and mark-out) before a status check is due; 0 for none.
func (s Settings) TimerFor(status string, mo *MarkOut) int {
	if status == MarkedOut && mo != nil {
		if t, ok := s.MarkOutType(mo.Code); ok {
			return t.Minutes
		}
	}
	return s.Timers[status]
}

// CheckDue is when the unit's next status check is due, and whether it has one.
func (u *Unit) CheckDue(s Settings) (time.Time, bool) {
	m := s.TimerFor(u.Status, u.MarkOut)
	if m <= 0 || u.Status == Available || u.Status == OutOfService {
		return time.Time{}, false
	}
	from := u.StatusAt
	if u.Checked.After(from) {
		from = u.Checked
	}
	return from.Add(time.Duration(m) * time.Minute), true
}

// UserInfo is a user as the ledger records them (never their password).
type UserInfo struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Role     string `json:"role"` // admin, supervisor, dispatcher, viewer
	Disabled bool   `json:"disabled,omitempty"`
	Demo     bool   `json:"demo,omitempty"` // the public demonstration account, signed in without a password
}
