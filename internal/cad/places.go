package cad

// Structured call locations: the place responders go to, not only its street address. The verified base
// address stays the incident's location; the place adds the business, property or complex name, the
// building, floor and apartment or room, and response detail such as the entrance or gate code, each in its
// own field so it can be searched and so flags can be scoped to it.
//
// A call is never held up because its address is not an authoritative address point. The call-taker can
// take an estimated range result, a caller or map point, or force the location for this incident (when
// the agency lets their duty force one, with a reason). Whatever was typed is kept exactly as typed with
// the map point, the nearest verified alternatives, the source and the confidence, and every such call
// opens a location exception: the supervisor's review item (did the call-taker use the best information
// there was, and does the agency's address data or workflow need fixing) and the address queue's item
// (new, under review, verified, rejected, merged). Verifying or merging links the incident to the master
// point later without rewriting what was entered or what responders were shown at the time.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

const (
	EvIncPlace      = "incident.place"         // the incident's structured place and how its location was verified
	EvExceptionNote = "location.exception"     // a supervisor's acknowledgement, decisions or follow-up on a location exception
	EvAddressQueue  = "address.queue"          // the address queue's state of an exception: review, verified, rejected, merged
	EvPlaceRefused  = "incident.place-refused" // someone tried to force a location their duty may not force
)

// How an incident's location was settled.
const (
	PlaceVerified   = "verified"   // an authoritative address point (agency GIS or the approved address source)
	PlaceEstimated  = "estimated"  // an address range estimate
	PlaceMap        = "map"        // a point the call-taker placed on the map
	PlaceCaller     = "caller"     // the caller's device location, with no exact address
	PlaceForced     = "forced"     // forced for this incident: not in the address source
	PlaceUnverified = "unverified" // typed and not yet checked
)

// PlaceKinds are the ways a location can be settled, in the order the call-taker is offered them.
var PlaceKinds = []string{PlaceVerified, PlaceEstimated, PlaceCaller, PlaceMap, PlaceForced, PlaceUnverified}

// Place is the structured place of an incident.
type Place struct {
	Name     string `json:"name,omitempty"`     // business, building, property or campus name
	Complex  string `json:"complex,omitempty"`  // apartment complex or campus, when not the same as Name
	Building string `json:"building,omitempty"` // building number or identifier
	Floor    string `json:"floor,omitempty"`    // floor or level
	UnitKind string `json:"unitKind,omitempty"` // apartment, suite, room, unit, lot, space…
	Unit     string `json:"unit,omitempty"`     // its number
	Detail   string `json:"detail,omitempty"`   // entrance, gate, access, stairwell and other response detail

	// How the location was settled, and what was typed and offered.
	Verify       string   `json:"verify"`
	Entered      string   `json:"entered,omitempty"`      // the address exactly as the call-taker typed it
	Source       string   `json:"source,omitempty"`       // gis, osm, range, caller, map, typed
	Confidence   string   `json:"confidence,omitempty"`   // exact, range, approximate, none
	Reason       string   `json:"reason,omitempty"`       // why it was forced (the agency's list)
	ReasonNote   string   `json:"reasonNote,omitempty"`   // the justification, when the reason needs one
	Nearest      string   `json:"nearest,omitempty"`      // the nearest verified address or the cross streets
	Alternatives []string `json:"alternatives,omitempty"` // the verified results that were offered
	Lat          *float64 `json:"lat,omitempty"`          // the map point accepted
	Lon          *float64 `json:"lon,omitempty"`
	AddrKey      string   `json:"addrKey,omitempty"` // the verified base address's stable identifier
	// Linked is the master address the agency later verified or merged this location into.
	Linked *PlaceLink `json:"linked,omitempty"`
	At     time.Time  `json:"at,omitzero"`
	By     string     `json:"by,omitempty"`
}

// PlaceLink is a later link from an unverified location to the agency's master address.
type PlaceLink struct {
	How     string    `json:"how"` // verified or merged
	Address string    `json:"address"`
	Lat     *float64  `json:"lat,omitempty"`
	Lon     *float64  `json:"lon,omitempty"`
	AddrKey string    `json:"addrKey,omitempty"`
	At      time.Time `json:"at"`
	By      string    `json:"by"`
}

// Label is the place as one line: "Oak Ridge Apartments, Bldg 4, Floor 2, Apt 204".
func (p *Place) Label() string {
	if p == nil {
		return ""
	}
	var parts []string
	add := func(prefix, v string) {
		if v = strings.TrimSpace(v); v != "" {
			parts = append(parts, prefix+v)
		}
	}
	add("", p.Name)
	if !strings.EqualFold(p.Complex, p.Name) {
		add("", p.Complex)
	}
	add("Bldg ", p.Building)
	add("Floor ", p.Floor)
	if p.Unit != "" {
		add(firstNonEmpty(unitWord(p.UnitKind), "Unit")+" ", p.Unit)
	}
	return strings.Join(parts, ", ")
}

func unitWord(k string) string {
	switch strings.ToLower(k) {
	case "apartment", "apt":
		return "Apt"
	case "suite", "ste":
		return "Suite"
	case "room", "rm":
		return "Room"
	case "lot":
		return "Lot"
	case "space":
		return "Space"
	case "unit":
		return "Unit"
	}
	if k == "" {
		return ""
	}
	return strings.ToUpper(k[:1]) + k[1:]
}

// Unverified says the location is not an authoritative address point.
func (p *Place) Unverified() bool { return p != nil && p.Verify != "" && p.Verify != PlaceVerified }

// PlacePolicy is the agency's rule for unverified locations and their review.
type PlacePolicy struct {
	// ForceDuties may force a location for an incident (empty: call-takers, dispatchers and supervisors).
	ForceDuties []string `json:"forceDuties,omitempty"`
	// TraineesForce lets trainees force a location (they cannot by default).
	TraineesForce bool `json:"traineesForce,omitempty"`
	// ForceReasons are the reasons offered (empty: the defaults); those ending in "(explain)" need a note.
	ForceReasons []string `json:"forceReasons,omitempty"`
	// ForceNoteAlways asks for a written justification with every forced location.
	ForceNoteAlways bool `json:"forceNoteAlways,omitempty"`
	// SupervisorApprove asks a supervisor to acknowledge a forced location at once (it never holds up dispatch).
	SupervisorApprove bool `json:"supervisorApprove,omitempty"`
	// Dispositions are the follow-up dispositions (empty: the defaults).
	Dispositions []string `json:"dispositions,omitempty"`
	// FollowupDays is the default due date of a follow-up, in days (0: 7).
	FollowupDays int `json:"followupDays,omitempty"`
	// UnitKinds are the sublocation kinds offered (empty: apartment, suite, room, unit, lot, space).
	UnitKinds []string `json:"unitKinds,omitempty"`
}

// DefaultForceReasons are the usual reasons a location is forced.
var DefaultForceReasons = []string{
	"New construction, not in the address data yet",
	"Address not found in the address data",
	"Rural or unaddressed location",
	"Caller cannot give an address",
	"The address data shows the wrong position",
	"Highway, mile marker or trail",
	"Other (explain)",
}

// The follow-up dispositions of a location exception.
const (
	DispNoAction  = "No further action"
	DispSubmitGIS = "Submit address or GIS update"
	DispMerge     = "Correct or merge master record"
	DispRefer     = "Refer to another jurisdiction"
	DispTraining  = "Training or workflow review"
	DispDefect    = "System defect"
	DispPending   = "Pending follow-up"
)

// DefaultDispositions are the follow-up dispositions.
var DefaultDispositions = []string{DispNoAction, DispSubmitGIS, DispMerge, DispRefer, DispTraining, DispDefect, DispPending}

func (p PlacePolicy) forceDuties() []string {
	if len(p.ForceDuties) == 0 {
		return []string{DutyCallTaker, DutyDispatcher, DutyPolice, DutyFire, DutyEMS, DutySupervisor, DutyAdmin}
	}
	return p.ForceDuties
}

// Reasons are the reasons offered for forcing a location.
func (p PlacePolicy) Reasons() []string {
	if len(p.ForceReasons) == 0 {
		return DefaultForceReasons
	}
	return p.ForceReasons
}

// DispositionList is the follow-up dispositions.
func (p PlacePolicy) DispositionList() []string {
	if len(p.Dispositions) == 0 {
		return DefaultDispositions
	}
	return p.Dispositions
}

// UnitKindList is the sublocation kinds.
func (p PlacePolicy) UnitKindList() []string {
	if len(p.UnitKinds) == 0 {
		return []string{"apartment", "suite", "room", "unit", "lot", "space"}
	}
	return p.UnitKinds
}

// AddressKey is the stable identifier of a base street address: the same for "1210 Williamson Rd NE" and
// "1210  WILLIAMSON RD. NE".
func AddressKey(addr string) string {
	n := normKey(addr)
	if n == "" {
		return ""
	}
	h := sha256.Sum256([]byte(n))
	return "A" + hex.EncodeToString(h[:6])
}

func normKey(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToUpper(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		default:
			space = true
		}
	}
	return b.String()
}

func normPart(s string) string { return normKey(s) }

// SubKey identifies a sublocation of a base address: building, floor and unit.
func SubKey(addrKey, building, floor, unit string) string {
	return addrKey + "|" + normPart(building) + "|" + normPart(floor) + "|" + normPart(unit)
}

// LocException is a forced, estimated or otherwise unverified incident location: the supervisor's review
// item and the address queue's item.
type LocException struct {
	ID           string    `json:"id"`
	Incident     string    `json:"incident"`
	Type         string    `json:"type,omitempty"` // the incident type when it opened
	Kind         string    `json:"kind"`           // forced, estimated, map, caller, unverified
	Entered      string    `json:"entered,omitempty"`
	Accepted     string    `json:"accepted,omitempty"` // the operational location responders were given
	Lat          *float64  `json:"lat,omitempty"`
	Lon          *float64  `json:"lon,omitempty"`
	CallerLat    *float64  `json:"callerLat,omitempty"`
	CallerLon    *float64  `json:"callerLon,omitempty"`
	Source       string    `json:"source,omitempty"`
	Confidence   string    `json:"confidence,omitempty"`
	Nearest      string    `json:"nearest,omitempty"`
	Alternatives []string  `json:"alternatives,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	ReasonNote   string    `json:"reasonNote,omitempty"`
	By           string    `json:"by"`
	Duty         string    `json:"duty,omitempty"`
	Console      string    `json:"console,omitempty"`
	At           time.Time `json:"at"`
	Period       string    `json:"period,omitempty"` // the operational period it opened in (shift.go)

	// The supervisor's review: an acknowledgement, then two separate decisions.
	Ack      *Stamp    `json:"ack,omitempty"`
	Located  *Decision `json:"located,omitempty"`  // was the best available location information used?
	Followup *Followup `json:"followup,omitempty"` // does the agency's address source or workflow need follow-up?
	// The address queue.
	Queue      string     `json:"queue"` // new, review, verified, rejected, merged
	QueueOwner string     `json:"queueOwner,omitempty"`
	Linked     *PlaceLink `json:"linked,omitempty"`
	Notes      []ExNote   `json:"notes,omitempty"` // every change, with who and when
}

// Stamp is who did something and when.
type Stamp struct {
	By string    `json:"by"`
	At time.Time `json:"at"`
}

// Decision is a supervisor's yes or no with a note.
type Decision struct {
	Answer string    `json:"answer"` // yes or no
	Note   string    `json:"note,omitempty"`
	By     string    `json:"by"`
	At     time.Time `json:"at"`
}

// Followup is the agency's follow-up of an exception: a disposition with an owner, a due date and whether
// it is done.
type Followup struct {
	Disposition string    `json:"disposition"`
	Owner       string    `json:"owner,omitempty"`
	Due         time.Time `json:"due,omitzero"`
	Note        string    `json:"note,omitempty"`
	By          string    `json:"by"`
	At          time.Time `json:"at"`
	Done        bool      `json:"done,omitempty"`
	DoneAt      time.Time `json:"doneAt,omitzero"`
	DoneBy      string    `json:"doneBy,omitempty"`
}

// ExNote is one change to an exception.
type ExNote struct {
	At   time.Time `json:"at"`
	By   string    `json:"by"`
	What string    `json:"what"`
	Note string    `json:"note,omitempty"`
}

// Address queue states.
const (
	QueueNew      = "new"
	QueueReview   = "review"
	QueueVerified = "verified"
	QueueRejected = "rejected"
	QueueMerged   = "merged"
)

// Open says the exception still needs the supervisor: until it is acknowledged and both decisions are
// recorded and the follow-up is done. Opening the incident does not close it.
func (x *LocException) Open() bool {
	return x.Ack == nil || x.Located == nil || x.Followup == nil || !x.Followup.Done
}

// placeData is an incident.place event: the place, and the exception it opened (if any).
type placeData struct {
	Place     Place  `json:"place"`
	Exception string `json:"exception,omitempty"`
}

// exceptionData is a location.exception event: what the supervisor did.
type exceptionData struct {
	ID       string    `json:"id"`
	Ack      bool      `json:"ack,omitempty"`
	Located  *Decision `json:"located,omitempty"`
	Followup *Followup `json:"followup,omitempty"`
	Done     bool      `json:"done,omitempty"` // the follow-up is done
	Note     string    `json:"note,omitempty"`
}

type queueData struct {
	ID    string     `json:"id"`
	State string     `json:"state"`
	Owner string     `json:"owner,omitempty"`
	Note  string     `json:"note,omitempty"`
	Link  *PlaceLink `json:"link,omitempty"`
}

// mayForce says whether this person may force a location.
func (s *State) mayForce(a Actor) error {
	if s.StaffOf(a.User).Trainee && !s.Settings.Places.TraineesForce {
		return dutyProblem("place", "trainees cannot force a location: ask your trainer or a supervisor")
	}
	if a.Role == "admin" || slices.Contains(s.Settings.Places.forceDuties(), DutyOf(a)) {
		return nil
	}
	return dutyProblem("place", "your duty (%s) cannot force a location; the agency lets %s", DutyOf(a), strings.Join(s.Settings.Places.forceDuties(), ", "))
}

// dutyProblem is a refusal because of who the person is: shown and recorded as refused.
func dutyProblem(_ string, format string, a ...any) error {
	return &Problem{Field: "duty", Message: fmt.Sprintf(format, a...)}
}

// SetPlace records the incident's structured place and how its location was settled. A location that is
// not verified opens a location exception for the supervisor (or adds to the open one); the exception's ID
// is returned.
func (e *Engine) SetPlace(a Actor, number string, p Place) (string, error) {
	if err := mayEnter(a); err != nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	in := e.st.Incidents[number]
	if in == nil {
		return "", problem("incident", "no incident %s", number)
	}
	clean := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	p.Name, p.Complex, p.Building, p.Floor = clean(p.Name), clean(p.Complex), clean(p.Building), clean(p.Floor)
	p.Unit, p.UnitKind, p.Detail = clean(p.Unit), strings.ToLower(clean(p.UnitKind)), strings.TrimSpace(p.Detail)
	p.Entered, p.Reason, p.ReasonNote, p.Nearest = strings.TrimSpace(p.Entered), clean(p.Reason), strings.TrimSpace(p.ReasonNote), clean(p.Nearest)
	p.Alternatives = cleanList(p.Alternatives)
	if p.Verify == "" {
		p.Verify = PlaceVerified
		if !in.LocVerified {
			p.Verify = PlaceUnverified
		}
	}
	if !slices.Contains(PlaceKinds, p.Verify) {
		return "", problem("verify", "how was the location settled: %s", strings.Join(PlaceKinds, ", "))
	}
	pol := e.st.Settings.Places
	switch p.Verify {
	case PlaceForced:
		if err := e.st.mayForce(a); err != nil {
			e.write(a, EvPlaceRefused, refs{Incident: number}, map[string]string{"why": err.Error(), "entered": p.Entered})
			return "", err
		}
		if p.Entered == "" {
			return "", problem("entered", "type the address as the caller gave it")
		}
		if p.Lat == nil || p.Lon == nil {
			return "", problem("map", "place the point on the map: a forced location needs one")
		}
		if p.Reason == "" || !slices.Contains(pol.Reasons(), p.Reason) {
			return "", problem("reason", "choose why it is forced: %s", strings.Join(pol.Reasons(), "; "))
		}
		if (pol.ForceNoteAlways || strings.HasSuffix(p.Reason, "(explain)")) && p.ReasonNote == "" {
			return "", problem("reasonNote", "explain why the location is forced")
		}
	case PlaceEstimated, PlaceMap, PlaceCaller:
		if p.Lat == nil || p.Lon == nil {
			if l := in.Location; l != nil && l.Lat != nil && l.Lon != nil {
				p.Lat, p.Lon = l.Lat, l.Lon
			} else {
				return "", problem("map", "a %s location needs a point on the map", p.Verify)
			}
		}
	}
	if p.Entered == "" && in.Location != nil {
		p.Entered = in.Location.Address
	}
	if p.Verify == PlaceVerified && in.Location != nil {
		p.AddrKey = AddressKey(in.Location.Address)
		if p.Source == "" {
			p.Source = "gis"
		}
		if p.Confidence == "" {
			p.Confidence = "exact"
		}
	}
	if old := in.Place; old != nil {
		p.Linked = old.Linked // a later change keeps the agency's link
	}
	x := ""
	if p.Unverified() {
		if open := e.st.openException(number); open != nil {
			x = open.ID
		} else {
			x = fmt.Sprintf("LX%d", e.st.lxCount+1)
		}
	}
	_, err := e.write(a, EvIncPlace, refs{Incident: number}, placeData{Place: p, Exception: x})
	return x, err
}

func (s *State) openException(number string) *LocException {
	for _, x := range s.LocExceptions {
		if x.Incident == number && x.Open() && x.Queue != QueueVerified && x.Queue != QueueMerged {
			return x
		}
	}
	return nil
}

// ExceptionReview is what a supervisor does with an exception.
type ExceptionReview struct {
	Ack      bool      `json:"ack,omitempty"`
	Located  *Decision `json:"located,omitempty"`
	Followup *Followup `json:"followup,omitempty"`
	Done     bool      `json:"done,omitempty"`
	Note     string    `json:"note,omitempty"`
}

// mayReview says whether the person reviews exceptions and critical incidents: a supervisor or above.
func mayReview(a Actor) error {
	if a.Role == "admin" || a.Role == "supervisor" || DutyOf(a) == DutySupervisor {
		return nil
	}
	return dutyProblem("review", "a supervisor reviews this")
}

// ReviewException records a supervisor's acknowledgement, decisions or follow-up on an exception. The two
// decisions are separate: acknowledging answers neither.
func (e *Engine) ReviewException(a Actor, id string, r ExceptionReview) error {
	if err := mayReview(a); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	x := e.st.LocExceptions[id]
	if x == nil {
		return problem("id", "no location exception %s", id)
	}
	if !r.Ack && r.Located == nil && r.Followup == nil && !r.Done && strings.TrimSpace(r.Note) == "" {
		return problem("review", "acknowledge it, answer a question, set the follow-up or add a note")
	}
	if r.Located != nil {
		if r.Located.Answer != "yes" && r.Located.Answer != "no" {
			return problem("located", "was the best available location information used: yes or no")
		}
		if r.Located.Answer == "no" && strings.TrimSpace(r.Located.Note) == "" {
			return problem("locatedNote", "say what better information there was")
		}
	}
	if f := r.Followup; f != nil {
		if !slices.Contains(e.st.Settings.Places.DispositionList(), f.Disposition) {
			return problem("disposition", "choose the follow-up: %s", strings.Join(e.st.Settings.Places.DispositionList(), "; "))
		}
		if f.Disposition != DispNoAction && strings.TrimSpace(f.Owner) == "" {
			return problem("owner", "name who owns the follow-up")
		}
		if f.Due.IsZero() && f.Disposition != DispNoAction {
			d := e.st.Settings.Places.FollowupDays
			if d <= 0 {
				d = 7
			}
			f.Due = e.now().Add(time.Duration(d) * 24 * time.Hour)
		}
	}
	if r.Done && x.Followup == nil && r.Followup == nil {
		return problem("disposition", "set the follow-up before marking it done")
	}
	_, err := e.write(a, EvExceptionNote, refs{Incident: x.Incident}, exceptionData{ID: id, Ack: r.Ack, Located: r.Located, Followup: r.Followup, Done: r.Done, Note: strings.TrimSpace(r.Note)})
	return err
}

// QueueException moves an exception through the address queue. Verified and merged link the incident to
// the master address (given), keeping what was entered and what responders were shown.
func (e *Engine) QueueException(a Actor, id, state, owner, note string, link *PlaceLink) error {
	if err := mayReview(a); err != nil && DutyOf(a) != DutyRecords {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	x := e.st.LocExceptions[id]
	if x == nil {
		return problem("id", "no location exception %s", id)
	}
	if !slices.Contains([]string{QueueNew, QueueReview, QueueVerified, QueueRejected, QueueMerged}, state) {
		return problem("state", "the address queue's states: new, review, verified, rejected, merged")
	}
	if (state == QueueVerified || state == QueueMerged) && (link == nil || strings.TrimSpace(link.Address) == "") {
		return problem("address", "give the verified master address it becomes")
	}
	if state == QueueRejected && strings.TrimSpace(note) == "" {
		return problem("note", "say why it is rejected")
	}
	if link != nil {
		link.How = state
		link.Address = strings.TrimSpace(link.Address)
		link.AddrKey = AddressKey(link.Address)
	}
	_, err := e.write(a, EvAddressQueue, refs{Incident: x.Incident}, queueData{ID: id, State: state, Owner: strings.TrimSpace(owner), Note: strings.TrimSpace(note), Link: link})
	return err
}

func (s *State) applyPlaces(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvIncPlace:
		d, err := decode[placeData](e)
		if err != nil {
			return true, err
		}
		in := s.Incidents[e.Incident]
		if in == nil {
			return true, nil
		}
		p := d.Place
		p.At, p.By = e.At, e.Actor
		in.Place = &p
		in.Updated = e.At
		switch {
		case p.Verify == PlaceForced:
			in.add(e, "place", fmt.Sprintf("Location FORCED for this incident: %q (%s)%s", p.Entered, p.Reason, suffix(p.ReasonNote)))
		case p.Unverified():
			in.add(e, "place", fmt.Sprintf("Location %s: %q", strings.ToUpper(p.Verify), p.Entered))
		default:
			if l := p.Label(); l != "" {
				in.add(e, "place", "Place: "+l)
			}
		}
		if p.Verify == PlaceVerified {
			in.LocVerified = true
		}
		if d.Exception != "" {
			s.openOrAddException(e, in, d.Exception, &p)
		}
		s.noteFlagsShown(e, in)
		return true, nil
	case EvExceptionNote:
		d, err := decode[exceptionData](e)
		if err != nil {
			return true, err
		}
		x := s.LocExceptions[d.ID]
		if x == nil {
			return true, nil
		}
		st := Stamp{By: e.Actor, At: e.At}
		if d.Ack && x.Ack == nil {
			x.Ack = &st
			x.Notes = append(x.Notes, ExNote{At: e.At, By: e.Actor, What: "acknowledged", Note: d.Note})
		}
		if d.Located != nil {
			v := *d.Located
			v.By, v.At = e.Actor, e.At
			x.Located = &v
			x.Notes = append(x.Notes, ExNote{At: e.At, By: e.Actor, What: "best information used: " + v.Answer, Note: v.Note})
		}
		if d.Followup != nil {
			f := *d.Followup
			f.By, f.At = e.Actor, e.At
			if f.Disposition == DispNoAction {
				f.Done, f.DoneAt, f.DoneBy = true, e.At, e.Actor
			}
			x.Followup = &f
			x.Notes = append(x.Notes, ExNote{At: e.At, By: e.Actor, What: "follow-up: " + f.Disposition + ownerDue(f), Note: f.Note})
			s.trackFollowup(e, "exception:"+x.ID, x.Incident, "Location exception "+x.ID+": "+f.Disposition, f.Owner, f.Due, f.Done)
		}
		if d.Done && x.Followup != nil && !x.Followup.Done {
			x.Followup.Done, x.Followup.DoneAt, x.Followup.DoneBy = true, e.At, e.Actor
			x.Notes = append(x.Notes, ExNote{At: e.At, By: e.Actor, What: "follow-up done", Note: d.Note})
			s.trackFollowup(e, "exception:"+x.ID, x.Incident, "", "", time.Time{}, true)
		}
		if !d.Ack && d.Located == nil && d.Followup == nil && !d.Done && d.Note != "" {
			x.Notes = append(x.Notes, ExNote{At: e.At, By: e.Actor, What: "note", Note: d.Note})
		}
		if in := s.Incidents[x.Incident]; in != nil {
			in.add(e, "place", "Location exception "+x.ID+" reviewed by "+e.Actor+lastWhat(x))
		}
		return true, nil
	case EvAddressQueue:
		d, err := decode[queueData](e)
		if err != nil {
			return true, err
		}
		x := s.LocExceptions[d.ID]
		if x == nil {
			return true, nil
		}
		x.Queue = d.State
		if d.Owner != "" {
			x.QueueOwner = d.Owner
		}
		x.Notes = append(x.Notes, ExNote{At: e.At, By: e.Actor, What: "address queue: " + d.State, Note: d.Note})
		if d.Link != nil {
			l := *d.Link
			l.At, l.By = e.At, e.Actor
			x.Linked = &l
			if in := s.Incidents[x.Incident]; in != nil {
				if in.Place != nil {
					in.Place.Linked = &l
				}
				in.add(e, "place", fmt.Sprintf("Location %s as %q by %s; the address entered (%q) is kept", d.State, l.Address, e.Actor, x.Entered))
				// An active incident's dispatchers and units see the corrected location as a change.
				if in.Status != Closed && l.Lat != nil && l.Lon != nil {
					to := &ng911.Location{Address: l.Address, Lat: l.Lat, Lon: l.Lon, Source: "address queue"}
					in.Changed = &LocChange{From: in.Location, To: to, At: e.At, By: e.Actor, Why: "address " + d.State}
					in.Location = to
					in.LocVerified = true
				}
			}
		}
		return true, nil
	case EvPlaceRefused:
		if in := s.Incidents[e.Incident]; in != nil {
			v, _ := decode[map[string]string](e)
			in.add(e, "refused", e.Actor+" could not force the location: "+v["why"])
		}
		return true, nil
	}
	return false, nil
}

func ownerDue(f Followup) string {
	s := ""
	if f.Owner != "" {
		s += ", owner " + f.Owner
	}
	if !f.Due.IsZero() {
		s += ", due " + f.Due.Format("2006-01-02")
	}
	return s
}

func lastWhat(x *LocException) string {
	if len(x.Notes) == 0 {
		return ""
	}
	return ": " + x.Notes[len(x.Notes)-1].What
}

func (s *State) openOrAddException(e ledger.Event, in *Incident, id string, p *Place) {
	x := s.LocExceptions[id]
	if x == nil {
		x = &LocException{ID: id, Incident: in.Number, Queue: QueueNew, At: e.At, By: e.Actor, Duty: e.Duty, Console: e.Console, Period: s.PeriodAt(e.At).ID}
		s.LocExceptions[id] = x
		s.lxCount++
		x.Notes = append(x.Notes, ExNote{At: e.At, By: e.Actor, What: "opened: " + p.Verify})
	} else {
		x.Notes = append(x.Notes, ExNote{At: e.At, By: e.Actor, What: "location changed: " + p.Verify, Note: p.Entered})
	}
	x.Type, x.Kind, x.Entered = in.Type, p.Verify, p.Entered
	x.Lat, x.Lon, x.Source, x.Confidence = p.Lat, p.Lon, p.Source, p.Confidence
	x.Nearest, x.Alternatives, x.Reason, x.ReasonNote = p.Nearest, p.Alternatives, p.Reason, p.ReasonNote
	if in.Location != nil {
		x.Accepted = in.Location.Address
	}
	if in.CallerAt != nil {
		x.CallerLat, x.CallerLon = in.CallerAt.Lat, in.CallerAt.Lon
	}
	for _, id := range in.Calls {
		if c := s.Calls[id]; c != nil && c.Location != nil && c.Location.Lat != nil && x.CallerLat == nil {
			x.CallerLat, x.CallerLon = c.Location.Lat, c.Location.Lon
		}
	}
	in.add(e, "place", "Location exception "+id+" opened for the supervisor")
}

// Exceptions lists location exceptions, newest first: open ones only, or every one.
func (s *State) Exceptions(all bool) []*LocException {
	out := []*LocException{}
	for _, x := range s.LocExceptions {
		if all || x.Open() || x.Queue == QueueNew || x.Queue == QueueReview {
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}
