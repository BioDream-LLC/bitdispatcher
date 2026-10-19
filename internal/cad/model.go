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
	AtDest      time.Time `json:"atDest,omitzero"`
	Cleared     time.Time `json:"cleared,omitzero"`
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
	Source     string          `json:"source"` // eido, ali, bitvoice
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
}

// Call is a call from call handling, as bitdispatcher knows it from every source.
type Call struct {
	ID         string           `json:"id"`
	Sources    []string         `json:"sources"`
	State      string           `json:"state"` // ringing, answered, ended
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
	Fixes      []Fix            `json:"fixes,omitempty"`
	Screen     *Screen          `json:"screen,omitempty"`
}

// Emergency is an emergency button or duress alert from a radio, open until a person acknowledges it.
type Emergency struct {
	Key         string    `json:"key"`
	Unit        string    `json:"unit,omitempty"` // the bitdispatcher unit, when the radio is mapped to one
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
	PhotosRecordedOnly bool          `json:"photosRecordedOnly,omitempty"`
	Dispositions       []string      `json:"dispositions"`
	Shifts             []string      `json:"shifts,omitempty"`   // shift change times, "07:00", for reports
	ORI                string        `json:"ori,omitempty"`      // the agency's originating agency identifier, for NCIC
	NCICMode           string        `json:"ncicMode,omitempty"` // off, sim (training simulator) or line (a switch adapter)
	NCICAddr           string        `json:"ncicAddr,omitempty"` // host:port of the switch adapter
	UnitTypes          []UnitType    `json:"unitTypes"`
	MarkOuts           []MarkOutType `json:"markOuts"`

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
