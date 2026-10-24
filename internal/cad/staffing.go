package cad

// Responder duty and unit staffing: who is on duty, where they are assigned and who actually staffs each
// unit. A person signs on for a duty period (regular, overtime, holdover, a temporary detail) with an
// availability; a unit's crew is named people in crew positions, from the field terminal (self sign-on),
// the dispatcher or a supervisor, as the agency allows for the kind of unit. Every change records who made
// it, when it was entered and when it took effect, and why.
//
// One person is on one active unit at a time: putting them on a second needs an explicit move (relief, a
// unit swap) or an authorised secondary assignment with a reason. Signing a person off clears their
// assignment; a unit left below its minimum or without a required position becomes limited or out of
// service as the agency says, unless a supervisor records an override. A lost connection signs nobody off.
// When a unit is dispatched, the incident keeps who was aboard and what they were qualified for at that
// moment; later roster changes never rewrite it, and changes during the incident are on its timeline.

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvDutyOn          = "member.on"           // a person signed on for a duty period
	EvDutyOff         = "member.off"          // a person signed off
	EvAvailability    = "member.availability" // a person's availability changed
	EvCrewTemplate    = "crew.template"       // a saved crew (a template for shift change)
	EvCrewTemplateDel = "crew.template.removed"
	EvCrewCorrected   = "crew.corrected"    // an earlier crew record corrected, the original kept
	EvCrewRequest     = "crew.request"      // a crew's own sign-on waiting for approval
	EvCrewDecided     = "crew.decided"      // a supervisor approved or refused it
	EvStaffOverride   = "staffing.override" // a supervisor kept a short unit in service
)

// Duty kinds and availabilities.
var (
	DutyKinds      = []string{"regular", "overtime", "holdover", "detail", "callback"}
	Availabilities = []string{"available", "assigned", "unavailable", "training", "court", "meal", "station-duty", "admin"}
)

// DutyPeriod is a person's time on duty.
type DutyPeriod struct {
	Start        time.Time `json:"start"`
	Until        time.Time `json:"until,omitzero"` // the scheduled end
	Shift        string    `json:"shift,omitempty"`
	Kind         string    `json:"kind,omitempty"`
	Availability string    `json:"availability,omitempty"`
	Detail       string    `json:"detail,omitempty"` // a temporary detail: where
	Note         string    `json:"note,omitempty"`
	By           string    `json:"by,omitempty"`
	Entered      time.Time `json:"entered,omitzero"`
}

// StaffingRules are the agency's staffing policy.
type StaffingRules struct {
	// Short is what happens to an available unit left below its minimum or without a required position:
	// warn (shown only), limited (shown as limited capability) or oos (put out of service).
	Short string `json:"short,omitempty"`
	// Dispatch of a unit short of what a call needs: warn, override (a supervisor's reason) or block.
	Dispatch string `json:"dispatch,omitempty"`
	// Minimums are the qualified people the centre wants on duty, for the coverage view.
	Minimums []StaffMinimum `json:"minimums,omitempty"`
}

// StaffMinimum is a minimum on-duty complement: this many people valid in a qualification.
type StaffMinimum struct {
	Qual   string `json:"qual"`
	Count  int    `json:"count"`
	Agency string `json:"agency,omitempty"`
	Note   string `json:"note,omitempty"`
}

// CrewChange is a change to a unit's crew.
type CrewChange struct {
	Seats     []CrewSeat `json:"crew"`
	Note      string     `json:"note,omitempty"`
	Reason    string     `json:"reason,omitempty"`
	Effective time.Time  `json:"effective,omitzero"` // when it took effect (zero: now)
	Override  string     `json:"override,omitempty"` // a supervisor's reason for keeping a short unit in service
}

// CrewTemplate is a saved crew for a unit, applied at shift change.
type CrewTemplate struct {
	ID    string     `json:"id"`
	Name  string     `json:"name"`
	Unit  string     `json:"unit"`
	Seats []CrewSeat `json:"crew"`
	Shift string     `json:"shift,omitempty"`
}

// CrewRequest is a crew's own sign-on waiting for a supervisor.
type CrewRequest struct {
	ID      string     `json:"id"`
	Unit    string     `json:"unit"`
	Change  CrewChange `json:"change"`
	By      string     `json:"by"`
	At      time.Time  `json:"at"`
	Decided string     `json:"decided,omitempty"` // approved or refused
	Why     string     `json:"why,omitempty"`
}

// CrewCorrection corrects an earlier crew record: the original is kept beside the corrected one.
type CrewCorrection struct {
	Unit      string     `json:"unit"`
	Incident  string     `json:"incident,omitempty"`
	Original  []CrewSeat `json:"original"`
	Corrected []CrewSeat `json:"corrected"`
	Effective time.Time  `json:"effective"`
	Reason    string     `json:"reason"`
}

// SignOnPolicy is how a kind of unit's crew is signed on: self (from the terminal), approve (from the
// terminal, approved by a supervisor) or dispatch (only by a dispatcher or supervisor).
func (s *State) SignOnPolicy(u *Unit) string {
	for _, t := range s.Settings.UnitTypes {
		if t.Code == u.Type && t.SignOn != "" {
			return t.SignOn
		}
	}
	return "self"
}

// positionsFor are the crew positions a unit must have filled.
func (s *State) positionsFor(u *Unit) []string {
	for _, t := range s.Settings.UnitTypes {
		if t.Code == u.Type {
			return t.Positions
		}
	}
	return nil
}

// SignOn puts a person on duty.
func (e *Engine) SignOn(a Actor, member string, d DutyPeriod) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	m := e.st.Members[member]
	if m == nil {
		return problem("member", "no member %s on the roster", member)
	}
	if m.Inactive {
		return problem("member", "%s is inactive", m.Name)
	}
	if d.Kind == "" {
		d.Kind = "regular"
	}
	if !slices.Contains(DutyKinds, d.Kind) {
		return problem("kind", "a duty period is regular, overtime, holdover, detail or callback")
	}
	if d.Availability == "" {
		d.Availability = "available"
	}
	if !slices.Contains(Availabilities, d.Availability) {
		return problem("availability", "unknown availability %s", d.Availability)
	}
	if d.Kind == "detail" && strings.TrimSpace(d.Detail) == "" {
		return problem("detail", "say where the temporary detail is")
	}
	if d.Start.IsZero() {
		d.Start = e.now()
	}
	if !d.Until.IsZero() && !d.Until.After(d.Start) {
		return problem("until", "the duty ends after it starts")
	}
	if m.Duty != nil {
		return problem("member", "%s is already on duty since %s", m.Name, m.Duty.Start.Format("15:04"))
	}
	_, err := e.write(a, EvDutyOn, refs{}, map[string]any{"member": member, "duty": d})
	return err
}

// SetAvailability changes an on-duty person's availability.
func (e *Engine) SetAvailability(a Actor, member, avail, note string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	m := e.st.Members[member]
	if m == nil || m.Duty == nil {
		return problem("member", "%s is not on duty", member)
	}
	if !slices.Contains(Availabilities, avail) {
		return problem("availability", "unknown availability %s", avail)
	}
	_, err := e.write(a, EvAvailability, refs{}, map[string]string{"member": member, "availability": avail, "note": note})
	return err
}

// SignOff takes a person off duty and off any unit they staff. A unit left short is handled as the agency
// says, unless override (a supervisor's reason) keeps it in service.
func (e *Engine) SignOff(a Actor, member, reason, override string, effective time.Time) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	m := e.st.Members[member]
	if m == nil {
		return problem("member", "no member %s on the roster", member)
	}
	if m.Duty == nil && e.st.RidingOn(member) == "" {
		return problem("member", "%s is not on duty", m.Name)
	}
	if effective.IsZero() {
		effective = e.now()
	}
	for _, uid := range e.st.unitsOf(member) {
		u := e.st.Units[uid]
		seats := slices.DeleteFunc(slices.Clone(u.Crew), func(c CrewSeat) bool { return c.Member == member })
		if err := e.setCrew(a, uid, CrewChange{Seats: seats, Reason: firstNonEmpty(reason, m.Name+" signed off"), Effective: effective, Override: override}); err != nil {
			return err
		}
	}
	if m.Duty != nil {
		if _, err := e.write(a, EvDutyOff, refs{}, map[string]any{"member": member, "reason": reason, "effective": effective}); err != nil {
			return err
		}
	}
	return nil
}

// unitsOf are the units a member staffs (more than one only with a secondary assignment).
func (s *State) unitsOf(member string) []string {
	var out []string
	for _, u := range s.Units {
		if slices.ContainsFunc(u.Crew, func(c CrewSeat) bool { return c.Member == member }) {
			out = append(out, u.ID)
		}
	}
	sort.Strings(out)
	return out
}

// SetCrewChange changes a unit's crew under the staffing rules.
func (e *Engine) SetCrewChange(a Actor, unit string, c CrewChange) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.setCrew(a, strings.ToUpper(unit), c)
}

// RequestCrew records a crew's own sign-on where the agency wants it approved; for other units it is
// applied at once. It returns the request's ID when one waits.
func (e *Engine) RequestCrew(a Actor, unit string, c CrewChange) (string, error) {
	unit = strings.ToUpper(unit)
	e.mu.Lock()
	defer e.mu.Unlock()
	u := e.st.Units[unit]
	if u == nil {
		return "", problem("unit", "no unit %s", unit)
	}
	switch e.st.SignOnPolicy(u) {
	case "dispatch":
		return "", problem("crew", "%s's crew is signed on by dispatch: call it in", unit)
	case "approve":
		id := fmt.Sprintf("CR%d", len(e.st.CrewRequests)+1)
		_, err := e.write(a, EvCrewRequest, refs{Unit: unit}, CrewRequest{ID: id, Unit: unit, Change: c, By: a.User})
		return id, err
	}
	return "", e.setCrew(a, unit, c)
}

// DecideCrew approves or refuses a waiting sign-on.
func (e *Engine) DecideCrew(a Actor, id string, approve bool, why string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	r := e.st.CrewRequests[id]
	if r == nil || r.Decided != "" {
		return problem("id", "no waiting sign-on %s", id)
	}
	if !approve && strings.TrimSpace(why) == "" {
		return problem("why", "say why the sign-on is refused")
	}
	if approve {
		c := r.Change
		c.Reason = firstNonEmpty(c.Reason, "approved sign-on from "+r.By)
		if err := e.setCrew(a, r.Unit, c); err != nil {
			return err
		}
	}
	verdict := "refused"
	if approve {
		verdict = "approved"
	}
	_, err := e.write(a, EvCrewDecided, refs{Unit: r.Unit}, map[string]string{"id": id, "decided": verdict, "why": why})
	return err
}

// setCrew applies a crew change. The caller holds e.mu.
func (e *Engine) setCrew(a Actor, unit string, c CrewChange) error {
	u, ok := e.st.Units[unit]
	if !ok {
		return problem("unit", "no unit %s", unit)
	}
	now := e.now()
	eff := c.Effective
	if eff.IsZero() {
		eff = now
	}
	if eff.After(now.Add(time.Minute)) {
		return problem("effective", "a crew change takes effect now or earlier")
	}
	var crew []CrewSeat
	for _, s := range c.Seats {
		m := e.st.Members[s.Member]
		if m == nil {
			return problem("crew", "no member %s on the roster", s.Member)
		}
		if m.Inactive {
			return problem("crew", "%s is marked inactive on the roster", m.Name)
		}
		if slices.ContainsFunc(crew, func(x CrewSeat) bool { return x.Member == m.ID }) {
			continue
		}
		// One active unit at a time: a move, or a secondary assignment with a reason.
		for _, other := range e.st.unitsOf(m.ID) {
			if other == unit {
				continue
			}
			if !s.Move && !s.Secondary {
				return problem("crew", "%s is on %s: move them (relief or a swap) or record a secondary assignment with a reason", m.Name, other)
			}
			if s.Secondary && strings.TrimSpace(firstNonEmpty(s.Reason, c.Reason)) == "" {
				return problem("crew", "say why %s also staffs %s", m.Name, unit)
			}
		}
		since := eff
		for _, old := range u.Crew {
			if old.Member == m.ID {
				since = old.Since
			}
		}
		seat := CrewSeat{Member: m.ID, Name: m.Name, Role: strings.ToLower(strings.TrimSpace(s.Role)), Since: since, Secondary: s.Secondary, Reason: strings.TrimSpace(s.Reason)}
		crew = append(crew, seat)
	}
	if len(crew) > 12 {
		return problem("crew", "up to 12 on a unit")
	}
	if c.Override != "" && a.Role != "supervisor" && a.Role != "admin" {
		return problem("override", "a supervisor keeps a short unit in service")
	}
	ev, err := e.write(a, EvUnitCrew, refs{Unit: unit, Incident: u.Incident}, crewData{Crew: crew, Note: strings.TrimSpace(c.Note), Reason: strings.TrimSpace(c.Reason), Effective: eff, Override: strings.TrimSpace(c.Override)})
	if err != nil {
		return err
	}
	// Put on a unit, a person is on duty.
	for _, s := range crew {
		if m := e.st.Members[s.Member]; m != nil && m.Duty == nil {
			if _, err := e.write(a, EvDutyOn, refs{Parent: ev.ID}, map[string]any{"member": m.ID, "duty": DutyPeriod{Start: eff, Kind: "regular", Availability: "assigned"}}); err != nil {
				return err
			}
		}
	}
	return e.shortAfter(a, u, c.Override, ev.ID)
}

// shortAfter puts an available unit left short out of service when the agency says so. The caller holds e.mu.
func (e *Engine) shortAfter(a Actor, u *Unit, override, parent string) error {
	short := e.st.Staffing(u)
	if len(short) == 0 || len(u.Crew) == 0 && !e.st.crewRequired(u) {
		return nil
	}
	if override != "" {
		_, err := e.write(a, EvStaffOverride, refs{Unit: u.ID, Parent: parent}, map[string]string{"short": strings.Join(short, "; "), "why": override})
		return err
	}
	if e.st.Settings.Staffing.Short != "oos" || u.Status != Available {
		return nil
	}
	_, err := e.write(SystemActor, EvUnitStatus, refs{Unit: u.ID, Parent: parent}, statusData{Status: OutOfService, Prev: u.Status, Reason: "staffing: " + strings.Join(short, "; ")})
	return err
}

// crewRequired reports whether a unit with nobody recorded aboard counts as unstaffed: once its kind of
// unit has required positions or a minimum and the crew was recorded before.
func (s *State) crewRequired(u *Unit) bool {
	return u.crewKnown && (s.MinStaff(u) > 0 || len(s.positionsFor(u)) > 0)
}

// CorrectCrew corrects an earlier crew record (on an incident, or the unit's current crew), keeping the
// original beside the correction.
func (e *Engine) CorrectCrew(a Actor, unit, incident string, corrected []CrewSeat, effective time.Time, reason string) error {
	unit = strings.ToUpper(unit)
	if strings.TrimSpace(reason) == "" {
		return problem("reason", "say why the crew record is corrected")
	}
	if a.Role != "supervisor" && a.Role != "admin" {
		return problem("crew", "a supervisor corrects crew records")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	u := e.st.Units[unit]
	if u == nil {
		return problem("unit", "no unit %s", unit)
	}
	var orig []CrewSeat
	if incident != "" {
		in := e.st.Incidents[incident]
		if in == nil {
			return problem("incident", "no incident %s", incident)
		}
		i := slices.IndexFunc(in.Units, func(x Assignment) bool { return x.Unit == unit })
		if i < 0 {
			return problem("unit", "%s was not on %s", unit, incident)
		}
		orig = in.Units[i].Crew
	} else {
		orig = u.Crew
	}
	var seats []CrewSeat
	for _, s := range corrected {
		m := e.st.Members[s.Member]
		if m == nil {
			return problem("crew", "no member %s on the roster", s.Member)
		}
		seats = append(seats, CrewSeat{Member: m.ID, Name: m.Name, Role: s.Role, Since: firstTime(s.Since, effective), Quals: e.st.ValidQuals(m, firstTime(effective, e.now()))})
	}
	if effective.IsZero() {
		effective = e.now()
	}
	_, err := e.write(a, EvCrewCorrected, refs{Unit: unit, Incident: incident}, CrewCorrection{Unit: unit, Incident: incident, Original: orig, Corrected: seats, Effective: effective, Reason: strings.TrimSpace(reason)})
	return err
}

// SaveCrewTemplate saves a crew for a unit to apply at shift change.
func (e *Engine) SaveCrewTemplate(a Actor, t CrewTemplate) (string, error) {
	t.Unit = strings.ToUpper(strings.TrimSpace(t.Unit))
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		return "", problem("name", "name the crew")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.Units[t.Unit] == nil {
		return "", problem("unit", "no unit %s", t.Unit)
	}
	for _, s := range t.Seats {
		if e.st.Members[s.Member] == nil {
			return "", problem("crew", "no member %s on the roster", s.Member)
		}
	}
	if t.ID == "" {
		t.ID = fmt.Sprintf("CT%d", len(e.st.CrewTemplates)+1)
		for e.st.CrewTemplates[t.ID] != nil {
			t.ID += "x"
		}
	}
	_, err := e.write(a, EvCrewTemplate, refs{Unit: t.Unit}, t)
	return t.ID, err
}

// RemoveCrewTemplate removes a saved crew.
func (e *Engine) RemoveCrewTemplate(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.CrewTemplates[id] == nil {
		return problem("id", "no saved crew %s", id)
	}
	_, err := e.write(a, EvCrewTemplateDel, refs{}, idData{id})
	return err
}

// ShiftChange applies several units' crews at once (from saved crews or as given), each person and their
// effective time recorded. People moving between units are moved.
func (e *Engine) ShiftChange(a Actor, crews map[string][]CrewSeat, templates []string, effective time.Time, reason string) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	all := map[string][]CrewSeat{}
	for _, id := range templates {
		t := e.st.CrewTemplates[id]
		if t == nil {
			return 0, problem("templates", "no saved crew %s", id)
		}
		all[t.Unit] = t.Seats
	}
	for u, seats := range crews {
		all[strings.ToUpper(u)] = seats
	}
	units := make([]string, 0, len(all))
	for u := range all {
		units = append(units, u)
	}
	sort.Strings(units)
	// First clear everyone moving, so no one is briefly on two units.
	n := 0
	for _, u := range units {
		seats := make([]CrewSeat, len(all[u]))
		for i, s := range all[u] {
			s.Move = true
			seats[i] = s
		}
		if err := e.setCrew(a, u, CrewChange{Seats: seats, Reason: firstNonEmpty(reason, "shift change"), Effective: effective}); err != nil {
			return n, fmt.Errorf("%s: %w", u, err)
		}
		n++
	}
	return n, nil
}

// Capability states.
const (
	CapFull        = "full"
	CapLimited     = "limited"
	CapUnavailable = "unavailable"
)

// UnitCapability is a unit's live capability: full, limited or unavailable, and why.
type UnitCapability struct {
	State   string   `json:"state"`
	Reasons []string `json:"reasons,omitempty"`
}

// CapabilityOf says whether a unit is fully capable now, from its status and who is aboard.
func (s *State) CapabilityOf(u *Unit) UnitCapability {
	if u.Status == OutOfService {
		return UnitCapability{State: CapUnavailable, Reasons: []string{"out of service"}}
	}
	if len(u.Crew) == 0 {
		if s.crewRequired(u) {
			return UnitCapability{State: CapUnavailable, Reasons: []string{"no crew signed on"}}
		}
		return UnitCapability{State: CapFull}
	}
	if r := s.Staffing(u); len(r) > 0 {
		return UnitCapability{State: CapLimited, Reasons: r}
	}
	return UnitCapability{State: CapFull}
}

// missingPositions are the required crew positions nobody aboard fills.
func (s *State) missingPositions(u *Unit) []string {
	var out []string
	for _, p := range s.positionsFor(u) {
		if !slices.ContainsFunc(u.Crew, func(c CrewSeat) bool { return strings.EqualFold(c.Role, p) }) {
			out = append(out, "no "+p)
		}
	}
	return out
}

// crewSnapshot is who is aboard a unit at a moment, with what each was validly qualified for.
func (s *State) crewSnapshot(u *Unit, at time.Time) []CrewSeat {
	if len(u.Crew) == 0 {
		return nil
	}
	out := make([]CrewSeat, len(u.Crew))
	for i, c := range u.Crew {
		c.Quals = nil
		if m := s.Members[c.Member]; m != nil {
			c.Quals = s.ValidQuals(m, at)
		}
		out[i] = c
	}
	return out
}

// OnDuty are the people on duty, by name.
func (s *State) OnDuty() []*Member {
	var out []*Member
	for _, m := range s.Members {
		if m.Duty != nil {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// CoverageGap is a minimum complement not met.
type CoverageGap struct {
	StaffMinimum
	Have  int      `json:"have"`
	Names []string `json:"names"`
}

// Coverage compares the people on duty with the agency's minimums.
func (s *State) StaffCoverage(now time.Time) []CoverageGap {
	out := []CoverageGap{}
	for _, mn := range s.Settings.Staffing.Minimums {
		g := CoverageGap{StaffMinimum: mn, Names: []string{}}
		for _, m := range s.OnDuty() {
			if mn.Agency != "" && m.Agency != mn.Agency {
				continue
			}
			if s.holds(m, mn.Qual, now) {
				g.Have++
				g.Names = append(g.Names, m.Name)
			}
		}
		out = append(out, g)
	}
	return out
}

func (s *State) applyStaffing(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvDutyOn:
		v, err := decode[struct {
			Member string     `json:"member"`
			Duty   DutyPeriod `json:"duty"`
		}](e)
		if err != nil {
			return true, err
		}
		if m := s.Members[v.Member]; m != nil {
			d := v.Duty
			d.By, d.Entered = e.Actor, e.At
			m.Duty = &d
		}
	case EvDutyOff:
		v, err := decode[map[string]any](e)
		if err != nil {
			return true, err
		}
		if id, _ := v["member"].(string); id != "" {
			if m := s.Members[id]; m != nil {
				m.Duty = nil
			}
		}
	case EvAvailability:
		v, err := decode[map[string]string](e)
		if err != nil {
			return true, err
		}
		if m := s.Members[v["member"]]; m != nil && m.Duty != nil {
			m.Duty.Availability = v["availability"]
			if v["note"] != "" {
				m.Duty.Note = v["note"]
			}
		}
	case EvCrewTemplate:
		t, err := decode[CrewTemplate](e)
		if err != nil {
			return true, err
		}
		s.CrewTemplates[t.ID] = &t
	case EvCrewTemplateDel:
		v, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		delete(s.CrewTemplates, v.ID)
	case EvCrewRequest:
		r, err := decode[CrewRequest](e)
		if err != nil {
			return true, err
		}
		r.At = e.At
		s.CrewRequests[r.ID] = &r
	case EvCrewDecided:
		v, err := decode[map[string]string](e)
		if err != nil {
			return true, err
		}
		if r := s.CrewRequests[v["id"]]; r != nil {
			r.Decided, r.Why = v["decided"], v["why"]
		}
	case EvStaffOverride:
		v, err := decode[map[string]string](e)
		if err != nil {
			return true, err
		}
		if u := s.Units[e.Unit]; u != nil {
			u.StaffOverride = v["why"]
			if in := s.Incidents[u.Incident]; in != nil {
				in.add(e, "crew", fmt.Sprintf("%s kept in service short-staffed (%s): %s", u.ID, v["short"], v["why"]))
			}
		}
	case EvCrewCorrected:
		v, err := decode[CrewCorrection](e)
		if err != nil {
			return true, err
		}
		names := func(cs []CrewSeat) string {
			if len(cs) == 0 {
				return "nobody"
			}
			out := make([]string, len(cs))
			for i, c := range cs {
				out[i] = c.Name
			}
			return strings.Join(out, ", ")
		}
		if in := s.Incidents[v.Incident]; in != nil {
			for i := range in.Units {
				if in.Units[i].Unit == v.Unit {
					in.Units[i].Crew = v.Corrected
					in.Units[i].CrewCorrected = true
				}
			}
			in.add(e, "crew", fmt.Sprintf("%s crew corrected (effective %s): was %s; now %s — %s", v.Unit, v.Effective.UTC().Format("15:04"), names(v.Original), names(v.Corrected), v.Reason))
		} else if u := s.Units[v.Unit]; u != nil {
			u.Crew = v.Corrected
		}
	default:
		return false, nil
	}
	return true, nil
}
