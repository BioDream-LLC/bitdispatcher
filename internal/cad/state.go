package cad

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// Event types. Each is written by exactly one command and read by apply.
const (
	EvSettings          = "settings.changed"
	EvStationSaved      = "station.saved"
	EvStationRemoved    = "station.removed"
	EvUnitSaved         = "unit.saved"
	EvUnitRemoved       = "unit.removed"
	EvCallTypeSaved     = "calltype.saved"
	EvCallTypeRemoved   = "calltype.removed"
	EvUserSaved         = "user.saved"
	EvUserRemoved       = "user.removed"
	EvIntegration       = "integration.changed"
	EvSignIn            = "session.signin"
	EvSignInFailed      = "session.signin.failed"
	EvSignOut           = "session.signout"
	EvCallReceived      = "call.received"
	EvCallUpdated       = "call.updated"
	EvCallEnded         = "call.ended"
	EvCallAttached      = "call.attached"
	EvCallAction        = "call.action"
	EvIncCreated        = "incident.created"
	EvIncChanged        = "incident.changed"
	EvIncNote           = "incident.note"
	EvIncDispatch       = "incident.dispatch"
	EvIncClosed         = "incident.closed"
	EvIncReopened       = "incident.reopened"
	EvUnitStatus        = "unit.status"
	EvUnitLocation      = "unit.location"
	EvUnitChecked       = "unit.checked" // a status check: the dispatcher confirmed the unit is all right
	EvEmergency         = "radio.emergency"
	EvEmergencyAck      = "radio.emergency.ack"
	EvRecommendation    = "recommendation.presented"
	EvMapRecording      = "map.recording"      // an administrator started recording the map of an area
	EvMapRecorded       = "map.recorded"       // the recorded map was replaced
	EvMapFailed         = "map.failed"         // a recording did not finish
	EvUnitCover         = "unit.cover"         // a unit was moved up to cover another station, or sent home
	EvAdvisoryCodebook  = "advisory.codebook"  // the advisory codebook (rules and mode) was changed
	EvAdvisorySuggested = "advisory.suggested" // the advisory panel made a suggestion (shown, or in shadow mode only kept)
	EvAdvisoryDecided   = "advisory.decided"   // a person accepted or rejected a suggestion
	EvAdvisoryWithdrawn = "advisory.withdrawn" // the evidence for a suggestion no longer holds
	EvGISImporting      = "gis.importing"      // an administrator started importing the county's GIS layers
	EvGISImported       = "gis.imported"       // a GIS import went into use
	EvGISFailed         = "gis.failed"         // a GIS import did not finish
	EvReportSaved       = "rms.report.saved"   // an incident report was written or changed
	EvReportStatus      = "rms.report.status"  // a report was submitted, approved or returned
	EvJusticeQuery      = "justice.query"      // an NCIC or state query was sent (terms hashed, never stored)
	EvReportCertified   = "report.certified"   // a supervisor certified a report for a period
	EvIBRExported       = "ibr.exported"       // a month's incident-based reporting file was made for the state
	EvPremiseSaved      = "premise.saved"      // a premise (hazards, notes, contacts at a place) was added or changed
	EvPremiseRemoved    = "premise.removed"
	EvPremisesImported  = "premise.imported" // premises brought over from an earlier system, a batch
	EvHistoryImported   = "history.imported" // incidents from an earlier system, a batch
	EvPhotosRecording   = "photos.recording" // an administrator started recording aerial photos
	EvPhotosRecorded    = "photos.recorded"  // a photo recording finished
	EvPhotosFailed      = "photos.failed"    // a photo recording stopped before the end
)

type idData struct {
	ID string `json:"id"`
}

type incidentCreated struct {
	Number    string          `json:"number"`
	Type      string          `json:"type"`
	Priority  int             `json:"priority"`
	Location  *ng911.Location `json:"location,omitempty"`
	Caller    *Caller         `json:"caller,omitempty"`
	Narrative string          `json:"narrative,omitempty"`
	Call      string          `json:"call,omitempty"`
}

type incidentChanged struct {
	Type     *string         `json:"type,omitempty"`
	Priority *int            `json:"priority,omitempty"`
	Location *ng911.Location `json:"location,omitempty"`
	Caller   *Caller         `json:"caller,omitempty"`
	Reason   string          `json:"reason,omitempty"`
}

type noteData struct {
	Text     string `json:"text"`
	Priority bool   `json:"priority,omitempty"`
}

type dispatchData struct {
	Units       []string `json:"units"`
	Recommended []string `json:"recommended"`
}

type statusData struct {
	Status       string          `json:"status"`
	Prev         string          `json:"prev"`
	Incident     string          `json:"incident,omitempty"`
	PrevIncident string          `json:"prevIncident,omitempty"`
	Recommended  bool            `json:"recommended,omitempty"`
	Reason       string          `json:"reason,omitempty"`
	MarkOut      string          `json:"markOut,omitempty"` // the mark-out type, for markout
	Location     *ng911.Location `json:"location,omitempty"`
}

type checkData struct {
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
}

type closeData struct {
	Disposition string `json:"disposition"`
	Text        string `json:"text,omitempty"`
}

type reasonData struct {
	Reason string `json:"reason,omitempty"`
}

type actionData struct {
	Action string `json:"action"` // rebid, text, video-link, eido
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// State is everything bitdispatcher knows, rebuilt from the ledger.
type State struct {
	Seq         uint64                `json:"seq"`
	Settings    Settings              `json:"settings"`
	Stations    map[string]*Station   `json:"stations"`
	Units       map[string]*Unit      `json:"units"`
	CallTypes   map[string]*CallType  `json:"callTypes"`
	Incidents   map[string]*Incident  `json:"incidents"`
	Calls       map[string]*Call      `json:"calls"`
	Emergencies map[string]*Emergency `json:"emergencies"`
	Users       map[string]*UserInfo  `json:"users"`
	Advisory    AdvisoryState         `json:"advisory"`
	Reports     map[string]*RMSReport `json:"reports"`
	Premises    map[string]*Premise   `json:"premises"`
	History     []History             `json:"-"` // incidents from an earlier system
	rptCount    map[string]int        // "site prefix|yy"
	premCount   int
	premGrid    grid
	premIDs     []string
	premIdx     map[string]int
	histGrid    grid
	yearCount   map[string]int // incidents per site and year, for numbering
}

func newState() *State {
	return &State{
		Settings: DefaultSettings(), Stations: map[string]*Station{}, Units: map[string]*Unit{},
		CallTypes: map[string]*CallType{}, Incidents: map[string]*Incident{}, Calls: map[string]*Call{},
		Emergencies: map[string]*Emergency{}, Users: map[string]*UserInfo{}, yearCount: map[string]int{},
		Reports: map[string]*RMSReport{}, rptCount: map[string]int{},
		Premises: map[string]*Premise{}, premGrid: grid{}, premIdx: map[string]int{}, histGrid: grid{},
	}
}

func decode[T any](e ledger.Event) (T, error) {
	var v T
	if len(e.Data) == 0 {
		return v, nil
	}
	err := json.Unmarshal(e.Data, &v)
	return v, err
}

// apply changes the state by one event. It never fails on a well-formed ledger; an event it cannot read is
// an error, because the ledger was written by commands that validated it.
func (s *State) apply(e ledger.Event) error {
	s.Seq++ // events applied: the ledger's sequence on one site, the merged count on several
	switch e.Type {
	case EvSettings:
		v, err := decode[Settings](e)
		if err != nil {
			return err
		}
		if v.MarkOuts == nil {
			v.MarkOuts = DefaultMarkOuts() // settings saved before mark-outs existed
		}
		s.Settings = v
	case EvStationSaved:
		v, err := decode[Station](e)
		if err != nil {
			return err
		}
		s.Stations[v.ID] = &v
	case EvStationRemoved:
		v, err := decode[idData](e)
		if err != nil {
			return err
		}
		delete(s.Stations, v.ID)
		for _, u := range s.Units {
			if u.Covering == v.ID {
				u.Covering = ""
			}
		}
	case EvUnitSaved:
		v, err := decode[UnitDef](e)
		if err != nil {
			return err
		}
		if u, ok := s.Units[v.ID]; ok {
			u.UnitDef = v
		} else {
			s.Units[v.ID] = &Unit{UnitDef: v, Status: OutOfService, StatusAt: e.At}
		}
	case EvUnitRemoved:
		v, err := decode[idData](e)
		if err != nil {
			return err
		}
		delete(s.Units, v.ID)
	case EvCallTypeSaved:
		v, err := decode[CallType](e)
		if err != nil {
			return err
		}
		s.CallTypes[v.Code] = &v
	case EvCallTypeRemoved:
		v, err := decode[idData](e)
		if err != nil {
			return err
		}
		delete(s.CallTypes, v.ID)
	case EvUserSaved:
		v, err := decode[UserInfo](e)
		if err != nil {
			return err
		}
		s.Users[v.Username] = &v
	case EvUserRemoved:
		v, err := decode[idData](e)
		if err != nil {
			return err
		}
		delete(s.Users, v.ID)
	case EvCallReceived, EvCallUpdated:
		v, err := decode[CallData](e)
		if err != nil {
			return err
		}
		s.mergeCall(e.Call, v, e.At)
	case EvCallEnded:
		if c, ok := s.Calls[e.Call]; ok {
			c.State, c.End, c.Updated = "ended", e.At, e.At
		}
	case EvCallAttached:
		if c, ok := s.Calls[e.Call]; ok {
			c.Incident = e.Incident
		}
		if in, ok := s.Incidents[e.Incident]; ok {
			if !slices.Contains(in.Calls, e.Call) {
				in.Calls = append(in.Calls, e.Call)
			}
			in.add(e, "call", "Call "+s.callLabel(e.Call)+" attached")
		}
	case EvCallAction:
		v, err := decode[actionData](e)
		if err != nil {
			return err
		}
		if c, ok := s.Calls[e.Call]; ok && c.Incident != "" {
			if in, ok := s.Incidents[c.Incident]; ok {
				res := "done"
				if !v.OK {
					res = "failed: " + v.Detail
				} else if v.Detail != "" {
					res = v.Detail
				}
				in.add(e, "call", "Caller "+v.Action+" "+res)
			}
		}
	case EvIncCreated:
		v, err := decode[incidentCreated](e)
		if err != nil {
			return err
		}
		in := &Incident{Number: v.Number, Type: v.Type, Priority: v.Priority, Status: Pending, Location: v.Location,
			Caller: v.Caller, Calls: []string{}, Units: []Assignment{}, Timeline: []Entry{}, Created: e.At, CreatedBy: e.Actor, Updated: e.At}
		s.Incidents[v.Number] = in
		s.yearCount[fmt.Sprintf("%s|%d", e.Site, e.At.UTC().Year())]++
		text := "Created " + s.typeLabel(v.Type) + " P" + fmt.Sprint(v.Priority)
		if v.Location != nil && v.Location.Address != "" {
			text += " at " + v.Location.Address
		}
		in.add(e, "created", text)
		if v.Narrative != "" {
			in.Timeline = append(in.Timeline, Entry{Seq: e.Seq, At: e.At, Actor: e.Actor, Kind: "note", Text: v.Narrative})
		}
		if v.Call != "" {
			in.Calls = append(in.Calls, v.Call)
			if c, ok := s.Calls[v.Call]; ok {
				c.Incident = v.Number
			}
		}
	case EvIncChanged:
		v, err := decode[incidentChanged](e)
		if err != nil {
			return err
		}
		in, ok := s.Incidents[e.Incident]
		if !ok {
			return nil
		}
		var what []string
		if v.Type != nil && *v.Type != in.Type {
			what = append(what, "type "+s.typeLabel(in.Type)+" → "+s.typeLabel(*v.Type))
			in.Type = *v.Type
		}
		if v.Priority != nil && *v.Priority != in.Priority {
			what = append(what, fmt.Sprintf("priority P%d → P%d", in.Priority, *v.Priority))
			in.Priority = *v.Priority
		}
		if v.Location != nil {
			what = append(what, "location "+firstNonEmpty(v.Location.Address, "moved on map"))
			in.Location = v.Location
		}
		if v.Caller != nil {
			what = append(what, "caller "+strings.TrimSpace(v.Caller.Name+" "+v.Caller.Callback))
			in.Caller = v.Caller
		}
		text := "Changed " + strings.Join(what, ", ")
		if v.Reason != "" {
			text += " (" + v.Reason + ")"
		}
		in.add(e, "change", text)
	case EvIncNote:
		v, err := decode[noteData](e)
		if err != nil {
			return err
		}
		if in, ok := s.Incidents[e.Incident]; ok {
			kind := "note"
			if v.Priority {
				kind = "priority-note"
			}
			in.Timeline = append(in.Timeline, Entry{Seq: e.Seq, At: e.At, Actor: e.Actor, Kind: kind, Text: v.Text, Priority: v.Priority})
			in.Updated = e.At
		}
	case EvIncDispatch:
		v, err := decode[dispatchData](e)
		if err != nil {
			return err
		}
		if in, ok := s.Incidents[e.Incident]; ok {
			text := "Dispatched " + strings.Join(v.Units, " ")
			var off []string
			for _, u := range v.Units {
				if !slices.Contains(v.Recommended, u) {
					off = append(off, u)
				}
			}
			if len(v.Recommended) > 0 && len(off) > 0 {
				text += " (not recommended: " + strings.Join(off, " ") + ")"
			}
			in.add(e, "dispatch", text)
		}
	case EvUnitStatus:
		v, err := decode[statusData](e)
		if err != nil {
			return err
		}
		s.applyStatus(e, v)
	case EvUnitChecked:
		v, err := decode[checkData](e)
		if err != nil {
			return err
		}
		if u, ok := s.Units[e.Unit]; ok {
			u.Checked, u.CheckedBy = e.At, e.Actor
			if in, ok := s.Incidents[u.Incident]; ok {
				in.add(e, "status", strings.TrimSpace(u.ID+" status check: all right "+v.Note))
			}
		}
	case EvPremiseSaved:
		v, err := decode[Premise](e)
		if err != nil {
			return err
		}
		s.putPremise(v)
	case EvPremiseRemoved:
		v, err := decode[idData](e)
		if err != nil {
			return err
		}
		if p, ok := s.Premises[v.ID]; ok {
			s.dropPremise(p)
		}
	case EvPremisesImported:
		v, err := decode[premiseBatch](e)
		if err != nil {
			return err
		}
		for _, p := range v.Premises {
			s.putPremise(p)
		}
	case EvHistoryImported:
		v, err := decode[historyBatch](e)
		if err != nil {
			return err
		}
		for _, h := range v.Items {
			s.addHistory(h)
		}
	case EvReportSaved:
		v, err := decode[rptData](e)
		if err != nil {
			return err
		}
		s.applyReport(e.At, e.Actor, v)
	case EvReportStatus:
		v, err := decode[rptStatus](e)
		if err != nil {
			return err
		}
		s.applyReportStatus(e.At, e.Actor, v)
	case EvAdvisoryCodebook:
		v, err := decode[CodebookData](e)
		if err != nil {
			return err
		}
		s.Advisory.Version++
		s.Advisory.Mode, s.Advisory.Rules = v.Mode, v.Rules
		s.Advisory.History = append(s.Advisory.History, CodebookVersion{Version: s.Advisory.Version, Mode: v.Mode, Note: v.Note, By: e.Actor, At: e.At, Seq: e.Seq, Rules: v.Rules})
	case EvUnitCover:
		v, err := decode[coverData](e)
		if err != nil {
			return err
		}
		s.applyCover(e.Unit, v)
	case EvUnitLocation:
		v, err := decode[Position](e)
		if err != nil {
			return err
		}
		if u, ok := s.Units[e.Unit]; ok {
			u.Position = &v
		}
	case EvIncClosed:
		v, err := decode[closeData](e)
		if err != nil {
			return err
		}
		if in, ok := s.Incidents[e.Incident]; ok {
			in.Status, in.Closed, in.Disposition = Closed, e.At, v.Disposition
			text := "Closed: " + v.Disposition
			if v.Text != "" {
				text += ". " + v.Text
			}
			in.add(e, "close", text)
		}
	case EvIncReopened:
		v, err := decode[reasonData](e)
		if err != nil {
			return err
		}
		if in, ok := s.Incidents[e.Incident]; ok {
			in.Closed, in.Disposition = time.Time{}, ""
			in.Status = in.liveStatus()
			in.add(e, "reopen", "Reopened: "+v.Reason)
		}
	case EvEmergency:
		v, err := decode[Emergency](e)
		if err != nil {
			return err
		}
		em := v
		s.Emergencies[v.Key] = &em
		if u, ok := s.Units[v.Unit]; ok {
			u.Emergency = v.Key
			if u.Incident != "" {
				if in, ok := s.Incidents[u.Incident]; ok {
					in.add(e, "emergency", "EMERGENCY button: "+u.ID+" on "+firstNonEmpty(v.ChannelName, v.Channel))
				}
			}
		}
	case EvEmergencyAck:
		v, err := decode[Emergency](e)
		if err != nil {
			return err
		}
		if em, ok := s.Emergencies[v.Key]; ok {
			em.AckedBy, em.AckedAt, em.AckedWhere = v.AckedBy, e.At, v.AckedWhere
			if u, ok := s.Units[em.Unit]; ok && u.Emergency == v.Key {
				u.Emergency = ""
				if u.Incident != "" {
					if in, ok := s.Incidents[u.Incident]; ok {
						in.add(e, "emergency", "Emergency of "+u.ID+" acknowledged by "+v.AckedBy)
					}
				}
			}
		}
	}
	return nil
}

func (s *State) applyStatus(e ledger.Event, v statusData) {
	u, ok := s.Units[e.Unit]
	if !ok {
		return
	}
	// Leaving an incident: mark the assignment cleared. The incident it leaves is the one the state has it
	// on, not the one the writer saw, so events merged from several sites always leave the same state.
	prevInc := u.Incident
	if prevInc != "" && prevInc != v.Incident {
		if in, ok := s.Incidents[prevInc]; ok {
			for i := range in.Units {
				if in.Units[i].Unit == u.ID && in.Units[i].Cleared.IsZero() {
					in.Units[i].Cleared, in.Units[i].Status = e.At, v.Status
				}
			}
			why := "cleared"
			if v.Incident != "" {
				why = "reassigned to " + v.Incident
			}
			in.add(e, "status", u.ID+" "+why)
			if in.Status != Closed {
				in.Status = in.liveStatus()
			}
		}
	}
	u.Status, u.StatusAt, u.Incident = v.Status, e.At, v.Incident
	u.MarkOut = nil
	if v.Status == MarkedOut {
		u.MarkOut = &MarkOut{Code: v.MarkOut, Location: v.Location, Note: v.Reason}
	}
	if v.Incident == "" {
		return
	}
	in, ok := s.Incidents[v.Incident]
	if !ok {
		return
	}
	idx := -1
	for i := range in.Units {
		if in.Units[i].Unit == u.ID && in.Units[i].Cleared.IsZero() {
			idx = i
		}
	}
	if idx < 0 {
		in.Units = append(in.Units, Assignment{Unit: u.ID, Dispatched: e.At, Recommended: v.Recommended})
		idx = len(in.Units) - 1
	}
	a := &in.Units[idx]
	a.Status = v.Status
	switch v.Status {
	case Enroute:
		a.Enroute = e.At
	case OnScene:
		a.OnScene = e.At
	case Transporting:
		a.Transport = e.At
	case AtDestination:
		a.AtDest = e.At
	}
	if v.Status != Dispatched || v.Reason != "" {
		text := u.ID + " " + StatusLabel(v.Status)
		if v.Reason != "" {
			text += " (" + v.Reason + ")"
		}
		in.add(e, "status", text)
	}
	if in.Status != Closed {
		in.Status = in.liveStatus()
	}
}

func (in *Incident) liveStatus() string {
	for _, a := range in.Units {
		if a.Cleared.IsZero() {
			return Active
		}
	}
	return Pending
}

func (in *Incident) add(e ledger.Event, kind, text string) {
	in.Timeline = append(in.Timeline, Entry{Seq: e.Seq, At: e.At, Actor: e.Actor, Kind: kind, Text: text, Unit: e.Unit})
	in.Updated = e.At
}

func (s *State) mergeCall(id string, d CallData, at time.Time) {
	c, ok := s.Calls[id]
	if !ok {
		c = &Call{ID: id, Sources: []string{}, State: "answered", Start: at}
		s.Calls[id] = c
	}
	if d.Source != "" && !slices.Contains(c.Sources, d.Source) {
		c.Sources = append(c.Sources, d.Source)
	}
	if d.State != "" && c.State != "ended" {
		c.State = d.State
	}
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&c.Callback, d.Callback)
	set(&c.CallerName, d.CallerName)
	set(&c.Class, d.Class)
	set(&c.Provider, d.Provider)
	set(&c.Agent, d.Agent)
	set(&c.Position, d.Position)
	set(&c.ALI, d.ALI)
	if !d.Start.IsZero() {
		c.Start = d.Start
	}
	if !d.Answer.IsZero() {
		c.Answer = d.Answer
	}
	if d.Location != nil && (d.Location.HasPoint() || d.Location.Civic != nil) {
		if c.Location == nil || !sameLocation(c.Location, d.Location) {
			c.Locations++
		}
		loc := *d.Location
		if loc.At.IsZero() {
			loc.At = at
		}
		// A civic address from one source and a point from another describe the same caller: keep both.
		if c.Location != nil && loc.Civic == nil && c.Location.Civic != nil {
			loc.Civic, loc.Address = c.Location.Civic, c.Location.Address
		}
		c.Location = &loc
	}
	if len(d.Media) > 0 {
		c.Media = d.Media
	}
	if d.Video != nil {
		c.Video = d.Video
	}
	if len(d.Text) > 0 {
		c.Text = d.Text
	}
	if d.TextOnly {
		c.TextOnly = true
	}
	if len(d.Notes) > 0 {
		c.Notes = d.Notes
	}
	if d.CanRebid {
		c.CanRebid = true
	}
	if d.Problems != nil {
		c.Problems = d.Problems
	}
	c.Updated = at
}

func sameLocation(a, b *ng911.Location) bool {
	if a.HasPoint() != b.HasPoint() {
		return false
	}
	if a.HasPoint() && (*a.Lat != *b.Lat || *a.Lon != *b.Lon || a.Radius != b.Radius) {
		return false
	}
	return a.Address == b.Address || b.Address == ""
}

func (s *State) callLabel(id string) string {
	if c, ok := s.Calls[id]; ok && c.Callback != "" {
		return c.Callback
	}
	return id
}

func (s *State) typeLabel(code string) string {
	if t, ok := s.CallTypes[code]; ok {
		return t.Name
	}
	return code
}

// StatusLabel is a status as a dispatcher reads it.
func StatusLabel(s string) string {
	switch s {
	case Available:
		return "available"
	case Dispatched:
		return "dispatched"
	case Enroute:
		return "en route"
	case OnScene:
		return "on scene"
	case Transporting:
		return "transporting"
	case AtDestination:
		return "at destination"
	case MarkedOut:
		return "marked out"
	case OutOfService:
		return "out of service"
	}
	return s
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// Discipline is the unit's discipline from its type: fire, ems, police or other.
func (s *State) Discipline(u *Unit) string {
	for _, t := range s.Settings.UnitTypes {
		if t.Code == u.Type {
			return t.Discipline
		}
	}
	return "other"
}
