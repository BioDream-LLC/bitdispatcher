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
	RadioID   string `json:"radioId,omitempty"` // the person's own radio ID
	// Duty is the person's current duty period (staffing.go), nil when off duty; Creds their credentials
	// (quals.go). Both are kept by their own events, not by saving the member.
	Duty  *DutyPeriod  `json:"duty,omitempty"`
	Creds []Credential `json:"creds,omitempty"`
}

// CrewSeat is one member riding a unit.
type CrewSeat struct {
	Member string    `json:"member"`
	Name   string    `json:"name"`
	Role   string    `json:"role,omitempty"` // the crew position: officer, driver, firefighter, paramedic…
	Since  time.Time `json:"since"`
	// Secondary: an authorised second assignment (with Reason) while also on another unit.
	Secondary bool   `json:"secondary,omitempty"`
	Reason    string `json:"reason,omitempty"`
	// Quals are what the person was validly qualified for, kept on an incident's record at dispatch.
	Quals []string `json:"quals,omitempty"`
	// Move (in a change only) moves the person from the unit they are on.
	Move bool `json:"move,omitempty"`
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
	Crew      []CrewSeat `json:"crew"`
	Note      string     `json:"note,omitempty"`
	Reason    string     `json:"reason,omitempty"`
	Effective time.Time  `json:"effective,omitzero"`
	Override  string     `json:"override,omitempty"`
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
	m.Duty, m.Creds = nil, nil // kept by their own events
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

// SetCrew sets who rides a unit (staffing.go applies the rules). An empty crew means staffing is not
// recorded for the unit.
func (e *Engine) SetCrew(a Actor, unit string, seats []CrewSeat, note string) error {
	return e.SetCrewChange(a, unit, CrewChange{Seats: seats, Note: note})
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
		if old := s.Members[m.ID]; old != nil {
			m.Duty, m.Creds = old.Duty, old.Creds
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
		// A member is on one unit at a time, unless this is an authorised secondary assignment.
		for _, c := range v.Crew {
			if c.Secondary {
				continue
			}
			for _, o := range s.Units {
				if o.ID != u.ID {
					o.Crew = slices.DeleteFunc(o.Crew, func(x CrewSeat) bool { return x.Member == c.Member && !x.Secondary })
				}
			}
		}
		old := u.Crew
		u.Crew = v.Crew
		u.crewKnown = true
		if v.Override == "" && len(s.Staffing(u)) == 0 {
			u.StaffOverride = ""
		}
		if in, ok := s.Incidents[u.Incident]; ok && u.Incident != "" {
			var joined, left []string
			for _, c := range v.Crew {
				if !slices.ContainsFunc(old, func(x CrewSeat) bool { return x.Member == c.Member }) {
					joined = append(joined, c.Name+roleIn(c.Role))
				}
			}
			for _, c := range old {
				if !slices.ContainsFunc(v.Crew, func(x CrewSeat) bool { return x.Member == c.Member }) {
					left = append(left, c.Name)
				}
			}
			var parts []string
			if len(joined) > 0 {
				parts = append(parts, "joined: "+strings.Join(joined, ", "))
			}
			if len(left) > 0 {
				parts = append(parts, "left: "+strings.Join(left, ", "))
			}
			if len(parts) == 0 {
				parts = append(parts, "positions changed")
			}
			text := fmt.Sprintf("%s crew %s", u.ID, strings.Join(parts, "; "))
			if !v.Effective.IsZero() && v.Effective.Before(e.At.Add(-time.Minute)) {
				text += " (effective " + v.Effective.UTC().Format("15:04") + ")"
			}
			in.add(e, "crew", text+suffix(v.Reason))
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
	out = append(out, s.missingPositions(u)...)
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
	now := s.now()
	for _, c := range u.Crew {
		if m := s.Members[c.Member]; m != nil && s.holds(m, qual, now) {
			n++
		}
	}
	return n
}

func roleIn(r string) string {
	if r == "" {
		return ""
	}
	return " (" + r + ")"
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
