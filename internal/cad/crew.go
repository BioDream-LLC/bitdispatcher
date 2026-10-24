package cad

// Crews and staffing: the agency's people (a roster of members with their qualifications), who rides each
// unit this shift, and each unit type's minimum staffing. A unit whose crew is recorded loses a capability
// its crew cannot staff (a medic with no paramedic aboard is not ALS), and a unit below its minimum says
// so in the recommendation. A unit with no crew recorded is taken as staffed, so an agency that does not
// keep rosters here sees no change.

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvMemberSaved   = "member.saved"   // a member of the agency was added or changed
	EvMemberRemoved = "member.removed" // a member left the roster
	EvUnitCrew      = "unit.crew"      // a unit's crew changed
)

// Member is one person on the agency's roster.
type Member struct {
	ID     string   `json:"id"` // the agency's employee or member number
	Name   string   `json:"name"`
	Rank   string   `json:"rank,omitempty"`
	Quals  []string `json:"quals,omitempty"` // qualifications: paramedic, emt, driver, officer, hazmat…
	Agency string   `json:"agency,omitempty"`
	// Station is the member's home station, for paging groups and the staffing screen.
	Station string `json:"station,omitempty"`
	// Volunteer members answer pages rather than riding a shift.
	Volunteer bool   `json:"volunteer,omitempty"`
	Phone     string `json:"phone,omitempty"` // for pages by text: +15405550100
	Email     string `json:"email,omitempty"`
	Topic     string `json:"topic,omitempty"` // a push topic (ntfy) for pages
	Inactive  bool   `json:"inactive,omitempty"`
	Note      string `json:"note,omitempty"`
}

// CrewSeat is one member riding a unit.
type CrewSeat struct {
	Member string    `json:"member"`
	Name   string    `json:"name"`
	Role   string    `json:"role,omitempty"` // officer, driver, firefighter, paramedic…
	Since  time.Time `json:"since"`
}

// CapNeed is what a capability needs of a unit's crew: this many members with a qualification.
type CapNeed struct {
	Capability    string `json:"capability"`
	Qualification string `json:"qualification"`
	Count         int    `json:"count"` // 0 means 1
}

// DefaultCapNeeds: advanced life support needs a paramedic aboard.
func DefaultCapNeeds() []CapNeed {
	return []CapNeed{{Capability: "als", Qualification: "paramedic", Count: 1}}
}

// DefaultQualifications are offered on the roster screen; an agency adds its own.
func DefaultQualifications() []string {
	return []string{"paramedic", "aemt", "emt", "firefighter", "driver", "officer", "hazmat", "technical-rescue", "dive", "k9", "swat"}
}

type crewData struct {
	Crew []CrewSeat `json:"crew"`
	Note string     `json:"note,omitempty"`
}

var reMember = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$`)

// SaveMember adds or changes a member of the roster.
func (e *Engine) SaveMember(a Actor, m Member) error {
	m.ID = strings.TrimSpace(m.ID)
	m.Name = strings.TrimSpace(m.Name)
	if !reMember.MatchString(m.ID) {
		return problem("id", "a member number is 1 to 32 letters, digits, dots, dashes or underscores")
	}
	if m.Name == "" {
		return problem("name", "give the member's name")
	}
	if m.Phone != "" && !rePhone.MatchString(m.Phone) {
		return problem("phone", "a phone number for texts is in international form: +15405550100")
	}
	if m.Email != "" && !strings.Contains(m.Email, "@") {
		return problem("email", "that is not an email address")
	}
	var qs []string
	for _, q := range m.Quals {
		q = strings.ToLower(strings.TrimSpace(q))
		if q != "" && !slices.Contains(qs, q) {
			qs = append(qs, q)
		}
	}
	m.Quals = qs
	e.mu.Lock()
	defer e.mu.Unlock()
	if m.Station != "" && e.st.Stations[m.Station] == nil {
		return problem("station", "no station %s", m.Station)
	}
	_, err := e.write(a, EvMemberSaved, refs{}, m)
	return err
}

var rePhone = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

// RemoveMember takes a member off the roster, and off any unit they ride.
func (e *Engine) RemoveMember(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.Members[id] == nil {
		return problem("id", "no member %s", id)
	}
	_, err := e.write(a, EvMemberRemoved, refs{}, idData{id})
	return err
}

// SetCrew sets who rides a unit. A member riding another unit moves to this one. An empty crew means
// staffing is not recorded for the unit.
func (e *Engine) SetCrew(a Actor, unit string, seats []CrewSeat, note string) error {
	unit = strings.ToUpper(unit)
	e.mu.Lock()
	defer e.mu.Unlock()
	u, ok := e.st.Units[unit]
	if !ok {
		return problem("unit", "no unit %s", unit)
	}
	var crew []CrewSeat
	now := e.now()
	for _, c := range seats {
		m := e.st.Members[c.Member]
		if m == nil {
			return problem("crew", "no member %s on the roster", c.Member)
		}
		if m.Inactive {
			return problem("crew", "%s is marked inactive on the roster", m.Name)
		}
		if slices.ContainsFunc(crew, func(x CrewSeat) bool { return x.Member == m.ID }) {
			continue
		}
		since := now
		for _, old := range u.Crew {
			if old.Member == m.ID {
				since = old.Since
			}
		}
		crew = append(crew, CrewSeat{Member: m.ID, Name: m.Name, Role: strings.TrimSpace(c.Role), Since: since})
	}
	if len(crew) > 12 {
		return problem("crew", "up to 12 on a unit")
	}
	_, err := e.write(a, EvUnitCrew, refs{Unit: unit, Incident: u.Incident}, crewData{Crew: crew, Note: strings.TrimSpace(note)})
	return err
}

func (s *State) applyCrew(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvMemberSaved:
		m, err := decode[Member](e)
		if err != nil {
			return true, err
		}
		if s.Members == nil {
			s.Members = map[string]*Member{}
		}
		s.Members[m.ID] = &m
		for _, u := range s.Units {
			for i := range u.Crew {
				if u.Crew[i].Member == m.ID {
					u.Crew[i].Name = m.Name
				}
			}
		}
	case EvMemberRemoved:
		v, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		delete(s.Members, v.ID)
		for _, u := range s.Units {
			u.Crew = slices.DeleteFunc(u.Crew, func(c CrewSeat) bool { return c.Member == v.ID })
		}
	case EvUnitCrew:
		v, err := decode[crewData](e)
		if err != nil {
			return true, err
		}
		u, ok := s.Units[e.Unit]
		if !ok {
			return true, nil
		}
		// A member is on one unit at a time.
		for _, c := range v.Crew {
			for _, o := range s.Units {
				if o.ID != u.ID {
					o.Crew = slices.DeleteFunc(o.Crew, func(x CrewSeat) bool { return x.Member == c.Member })
				}
			}
		}
		before := len(u.Crew)
		u.Crew = v.Crew
		if in, ok := s.Incidents[u.Incident]; ok && u.Incident != "" {
			names := make([]string, len(v.Crew))
			for i, c := range v.Crew {
				names[i] = c.Name
			}
			text := fmt.Sprintf("%s crew: %s", u.ID, strings.Join(names, ", "))
			if len(v.Crew) == 0 {
				text = fmt.Sprintf("%s crew cleared (was %d)", u.ID, before)
			}
			in.add(e, "crew", text)
		}
	default:
		return false, nil
	}
	return true, nil
}

// MinStaff is how many a unit needs aboard to be fully staffed: its own setting, else its type's; 0 for
// no minimum.
func (s *State) MinStaff(u *Unit) int {
	if u.MinStaff > 0 {
		return u.MinStaff
	}
	for _, t := range s.Settings.UnitTypes {
		if t.Code == u.Type {
			return t.MinStaff
		}
	}
	return 0
}

// Staffing says how a unit is staffed: "" when its crew is not recorded or it is fully staffed, else what
// is short ("2 of 3 aboard", "no paramedic for als").
func (s *State) Staffing(u *Unit) []string {
	if len(u.Crew) == 0 {
		return nil
	}
	var out []string
	if n := s.MinStaff(u); n > 0 && len(u.Crew) < n {
		out = append(out, fmt.Sprintf("%d of %d aboard", len(u.Crew), n))
	}
	return append(out, s.lostCaps(u)...)
}

// lostCaps are the capabilities a unit's crew cannot staff, as "no paramedic for als".
func (s *State) lostCaps(u *Unit) []string {
	if len(u.Crew) == 0 {
		return nil
	}
	var out []string
	base := s.baseCapabilities(u)
	for _, n := range s.capNeeds() {
		if !slices.Contains(base, n.Capability) {
			continue
		}
		if s.crewWith(u, n.Qualification) < max(1, n.Count) {
			out = append(out, fmt.Sprintf("no %s for %s", n.Qualification, n.Capability))
		}
	}
	return out
}

func (s *State) capNeeds() []CapNeed {
	if s.Settings.CapNeeds == nil {
		return DefaultCapNeeds()
	}
	return s.Settings.CapNeeds
}

func (s *State) crewWith(u *Unit, qual string) int {
	n := 0
	for _, c := range u.Crew {
		if m := s.Members[c.Member]; m != nil && slices.Contains(m.Quals, qual) {
			n++
		}
	}
	return n
}

// crewDrops are the capabilities a unit loses for its crew.
func (s *State) crewDrops(u *Unit) []string {
	if len(u.Crew) == 0 {
		return nil
	}
	var out []string
	base := s.baseCapabilities(u)
	for _, n := range s.capNeeds() {
		if slices.Contains(base, n.Capability) && s.crewWith(u, n.Qualification) < max(1, n.Count) {
			out = append(out, n.Capability)
		}
	}
	return out
}

// Roster is the members, by name.
func (s *State) Roster() []*Member {
	out := make([]*Member, 0, len(s.Members))
	for _, m := range s.Members {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// RidingOn is the unit a member is riding, or "".
func (s *State) RidingOn(member string) string {
	for _, u := range s.Units {
		for _, c := range u.Crew {
			if c.Member == member {
				return u.ID
			}
		}
	}
	return ""
}
