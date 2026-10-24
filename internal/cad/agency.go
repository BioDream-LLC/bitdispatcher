package cad

// Agencies: every organisation the centre dispatches for is a configured agency (RPD, RCPD, RFD…), not a
// discipline. An agency has its own identifiers (ORI, NERIS), incident numbering, dispositions, timers and
// talkgroups, and owns its stations, units, call types and response plans. A centre with no agencies
// configured works as before: one agency, the centre-wide settings.
//
// People are given the agencies they may work (their staff profile, kept here in the ledger rather than
// with their password), the one they normally work for, and those they watch. At sign-in their usual
// coverage loads by itself: the working agency (whose rules apply, always shown), the agencies covered and
// those only monitored. A trainee works the agency's assigned training preset (presets.go).

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
	EvAgencySaved   = "agency.saved"    // an agency was added or changed
	EvAgencyRemoved = "agency.removed"  // an agency was removed
	EvStaffSaved    = "staff.saved"     // a person's agencies, coverage and training status changed
	EvCoverage      = "coverage.set"    // the agencies a person is working, covering and watching now
	EvIncAgency     = "incident.agency" // an incident's primary or assisting agencies changed
)

// AgencyDisciplines are what an agency can do.
var AgencyDisciplines = []string{"dispatch", "police", "fire", "ems", "other"}

// Agency is one organisation the centre dispatches for.
type Agency struct {
	Code        string   `json:"code"` // RPD, RCFR: upper case, 2 to 8 letters or digits
	Name        string   `json:"name"`
	Disciplines []string `json:"disciplines"`
	Inactive    bool     `json:"inactive,omitempty"`
	// Identifiers in other systems: the ORI for NCIC and NIBRS, the NERIS department ID, and others by name.
	ORI   string            `json:"ori,omitempty"`
	NERIS string            `json:"neris,omitempty"`
	IDs   map[string]string `json:"ids,omitempty"`
	// IncidentPrefix numbers the agency's incidents (empty: the centre's prefix); Dispositions are its own
	// closing codes (empty: the centre's).
	IncidentPrefix string   `json:"incidentPrefix,omitempty"`
	Dispositions   []string `json:"dispositions,omitempty"`
	// Timers override the centre's status-check minutes for the agency's units, status by status.
	Timers     map[string]int `json:"timers,omitempty"`
	Talkgroups []string       `json:"talkgroups,omitempty"`
	// Records: the agency's own records rules, in words (retention, who receives its reports).
	Records string    `json:"records,omitempty"`
	Note    string    `json:"note,omitempty"`
	Updated time.Time `json:"updated,omitzero"`
	By      string    `json:"by,omitempty"`
}

// Staff is what bitdispatcher keeps about a person beyond their sign-in: the agencies they may work, the
// one they usually work for, those they watch, whether they are a trainee, and their roster entry.
type Staff struct {
	Username string `json:"username"`
	// Agencies the person may work; empty: every agency.
	Agencies []string `json:"agencies,omitempty"`
	Home     string   `json:"home,omitempty"`    // the agency they normally work for
	Covered  []string `json:"covered,omitempty"` // covered by default besides the home agency
	Monitor  []string `json:"monitor,omitempty"` // watched for awareness, by default
	// Trainee: works the agency's assigned training preset and cannot change the workspace.
	Trainee bool      `json:"trainee,omitempty"`
	Member  string    `json:"member,omitempty"` // their entry on the roster (crew.go), when they ride or are paged
	Note    string    `json:"note,omitempty"`
	Updated time.Time `json:"updated,omitzero"`
	By      string    `json:"by,omitempty"`
}

// WorkCoverage is what a person is working now: the working agency (whose rules apply), the agencies covered
// and those watched.
type WorkCoverage struct {
	Username  string    `json:"username"`
	Working   string    `json:"working"`
	Covered   []string  `json:"covered"`
	Monitored []string  `json:"monitored"`
	Console   string    `json:"console,omitempty"`
	Duty      string    `json:"duty,omitempty"`
	At        time.Time `json:"at"`
}

var reAgency = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,7}$`)

// SaveAgency adds or changes an agency.
func (e *Engine) SaveAgency(a Actor, g Agency) error {
	g.Code = strings.ToUpper(strings.TrimSpace(g.Code))
	g.Name = strings.TrimSpace(g.Name)
	if !reAgency.MatchString(g.Code) {
		return problem("code", "an agency code is 2 to 8 letters or digits, starting with a letter: RPD, RCFR")
	}
	if g.Name == "" {
		return problem("name", "give the agency's full name")
	}
	var ds []string
	for _, d := range g.Disciplines {
		d = strings.ToLower(strings.TrimSpace(d))
		if !slices.Contains(AgencyDisciplines, d) {
			return problem("disciplines", "a discipline is dispatch, police, fire, ems or other")
		}
		if !slices.Contains(ds, d) {
			ds = append(ds, d)
		}
	}
	if len(ds) == 0 {
		return problem("disciplines", "choose what the agency does: police, fire, ems, dispatch or other")
	}
	g.Disciplines = ds
	g.ORI = strings.ToUpper(strings.TrimSpace(g.ORI))
	if g.ORI != "" && !regexp.MustCompile(`^[A-Z0-9]{9}$`).MatchString(g.ORI) {
		return problem("ori", "an ORI is nine letters and digits")
	}
	g.IncidentPrefix = strings.ToUpper(strings.TrimSpace(g.IncidentPrefix))
	if len(g.IncidentPrefix) > 6 || strings.ContainsAny(g.IncidentPrefix, " /#") {
		return problem("incidentPrefix", "an incident prefix is up to 6 letters or digits")
	}
	for k, v := range g.Timers {
		if v < 0 || v > 600 {
			return problem("timers", "the %s timer is 0 to 600 minutes", k)
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for code, o := range e.st.Agencies {
		if code != g.Code && g.IncidentPrefix != "" && o.IncidentPrefix == g.IncidentPrefix {
			return problem("incidentPrefix", "%s already numbers its incidents %s", code, g.IncidentPrefix)
		}
	}
	_, err := e.write(a, EvAgencySaved, refs{}, g)
	return err
}

// RemoveAgency removes an agency nothing belongs to.
func (e *Engine) RemoveAgency(a Actor, code string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.Agencies[code] == nil {
		return problem("code", "no agency %s", code)
	}
	if uses := e.st.agencyUses(code); len(uses) > 0 {
		return problem("code", "%s still has %s; move them to another agency or mark it inactive", code, strings.Join(uses, ", "))
	}
	_, err := e.write(a, EvAgencyRemoved, refs{}, idData{code})
	return err
}

// agencyUses says what still belongs to an agency.
func (s *State) agencyUses(code string) []string {
	var out []string
	count := func(n int, what string) {
		if n > 0 {
			out = append(out, fmt.Sprintf("%d %s", n, what))
		}
	}
	n := 0
	for _, u := range s.Units {
		if u.Agency == code {
			n++
		}
	}
	count(n, "units")
	n = 0
	for _, st := range s.Stations {
		if st.Agency == code {
			n++
		}
	}
	count(n, "stations")
	n = 0
	for _, c := range s.CallTypes {
		if slices.Contains(c.Agencies, code) {
			n++
		}
	}
	count(n, "call types")
	n = 0
	for _, p := range s.Plans {
		if p.Agency == code {
			n++
		}
	}
	count(n, "response plans")
	return out
}

// checkAgency is a problem when agencies are configured and code is not one of them. The caller holds e.mu.
func (e *Engine) checkAgency(field, code string) error {
	if code == "" || len(e.st.Agencies) == 0 {
		return nil
	}
	if e.st.Agencies[code] == nil {
		return problem(field, "no agency %s: choose one from Admin › Agencies", code)
	}
	return nil
}

// SaveStaff sets a person's agencies, coverage and training status.
func (e *Engine) SaveStaff(a Actor, p Staff) error {
	p.Username = strings.ToLower(strings.TrimSpace(p.Username))
	if p.Username == "" {
		return problem("username", "say whose profile this is")
	}
	up := func(xs []string) []string {
		var out []string
		for _, x := range xs {
			x = strings.ToUpper(strings.TrimSpace(x))
			if x != "" && !slices.Contains(out, x) {
				out = append(out, x)
			}
		}
		return out
	}
	p.Agencies, p.Covered, p.Monitor, p.Home = up(p.Agencies), up(p.Covered), up(p.Monitor), strings.ToUpper(strings.TrimSpace(p.Home))
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, c := range slices.Concat(p.Agencies, p.Covered, p.Monitor, []string{p.Home}) {
		if err := e.checkAgency("agencies", c); err != nil {
			return err
		}
	}
	may := func(c string) bool { return len(p.Agencies) == 0 || slices.Contains(p.Agencies, c) }
	if p.Home != "" && !may(p.Home) {
		return problem("home", "the usual agency must be one the person may work")
	}
	for _, c := range p.Covered {
		if !may(c) {
			return problem("covered", "%s is not an agency the person may work", c)
		}
	}
	if p.Member != "" && e.st.Members[p.Member] == nil {
		return problem("member", "no member %s on the roster", p.Member)
	}
	_, err := e.write(a, EvStaffSaved, refs{}, p)
	return err
}

// StaffOf is a person's profile (an empty one when none was saved).
func (s *State) StaffOf(username string) Staff {
	if p := s.Staff[strings.ToLower(username)]; p != nil {
		return *p
	}
	return Staff{Username: strings.ToLower(username)}
}

// MayWork reports whether a person may work for an agency. With no agencies configured, or none given to
// the person, they may work every agency.
func (s *State) MayWork(username, code string) bool {
	if code == "" || len(s.Agencies) == 0 {
		return true
	}
	p := s.StaffOf(username)
	return len(p.Agencies) == 0 || slices.Contains(p.Agencies, code)
}

// AllAgencies reports whether a person is limited to no agencies in particular.
func (s *State) AllAgencies(username string) bool {
	return len(s.Agencies) == 0 || len(s.StaffOf(username).Agencies) == 0
}

// AgencyList is the agencies by code.
func (s *State) AgencyList() []*Agency {
	out := make([]*Agency, 0, len(s.Agencies))
	for _, g := range s.Agencies {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// DefaultCoverage is what a person works when they sign in at a console: their usual agency and coverage,
// limited to what the workstation covers when it says.
func (s *State) DefaultCoverage(username, console, duty string) WorkCoverage {
	p := s.StaffOf(username)
	c := WorkCoverage{Username: p.Username, Console: console, Duty: duty, Covered: []string{}, Monitored: []string{}}
	if len(s.Agencies) == 0 {
		return c
	}
	var ws *Workstation
	if console != "" {
		ws = s.Workstations[console]
	}
	may := func(code string) bool {
		g := s.Agencies[code]
		return g != nil && !g.Inactive && s.MayWork(username, code)
	}
	add := func(list *[]string, code string) {
		if may(code) && !slices.Contains(*list, code) {
			*list = append(*list, code)
		}
	}
	if p.Home != "" {
		add(&c.Covered, p.Home)
	}
	for _, x := range p.Covered {
		add(&c.Covered, x)
	}
	if ws != nil && len(ws.Agencies) > 0 {
		// At a workstation that covers given agencies, the person covers those they may work.
		c.Covered = slices.DeleteFunc(c.Covered, func(x string) bool { return !slices.Contains(ws.Agencies, x) })
		for _, x := range ws.Agencies {
			add(&c.Covered, x)
		}
		for _, x := range ws.Monitor {
			if !slices.Contains(c.Covered, x) && s.Agencies[x] != nil {
				c.Monitored = append(c.Monitored, x)
			}
		}
	}
	if len(c.Covered) == 0 && len(p.Agencies) > 0 {
		add(&c.Covered, p.Agencies[0])
	}
	for _, x := range p.Monitor {
		if !slices.Contains(c.Covered, x) && !slices.Contains(c.Monitored, x) && s.Agencies[x] != nil {
			c.Monitored = append(c.Monitored, x)
		}
	}
	if len(c.Covered) > 0 {
		c.Working = c.Covered[0]
		if p.Home != "" && slices.Contains(c.Covered, p.Home) {
			c.Working = p.Home
		}
	}
	return c
}

// CoverageOf is what a person is working now: what they set at this console, or their default.
func (s *State) CoverageOf(username, console, duty string) WorkCoverage {
	if c := s.Coverage[strings.ToLower(username)]; c != nil && c.Console == console && c.Duty == duty {
		return *c
	}
	return s.DefaultCoverage(username, console, duty)
}

// SetCoverage changes what a person is working: the working agency, those covered and those watched.
func (e *Engine) SetCoverage(a Actor, c WorkCoverage) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.st.Agencies) == 0 {
		return problem("working", "no agencies are configured")
	}
	c.Username, c.Console, c.Duty = a.User, a.Console, a.Duty
	c.Working = strings.ToUpper(strings.TrimSpace(c.Working))
	clean := func(xs []string) []string {
		out := []string{}
		for _, x := range xs {
			x = strings.ToUpper(strings.TrimSpace(x))
			if x != "" && !slices.Contains(out, x) {
				out = append(out, x)
			}
		}
		return out
	}
	c.Covered, c.Monitored = clean(c.Covered), clean(c.Monitored)
	if c.Working != "" && !slices.Contains(c.Covered, c.Working) {
		c.Covered = append([]string{c.Working}, c.Covered...)
	}
	for _, x := range c.Covered {
		g := e.st.Agencies[x]
		if g == nil || g.Inactive {
			return problem("covered", "no active agency %s", x)
		}
		if !e.st.MayWork(a.User, x) {
			return problem("covered", "you are not authorised to work for %s", x)
		}
	}
	for _, x := range c.Monitored {
		if e.st.Agencies[x] == nil {
			return problem("monitored", "no agency %s", x)
		}
	}
	c.Monitored = slices.DeleteFunc(c.Monitored, func(x string) bool { return slices.Contains(c.Covered, x) })
	if c.Working == "" && len(c.Covered) > 0 {
		c.Working = c.Covered[0]
	}
	_, err := e.write(a, EvCoverage, refs{}, c)
	return err
}

// AgencyOfUnit is a unit's home agency record, if it has one.
func (s *State) AgencyOfUnit(u *Unit) *Agency {
	if u == nil || u.Agency == "" {
		return nil
	}
	return s.Agencies[u.Agency]
}

// TimerForUnit is the minutes a unit may stay in its status (and mark-out) before a status check is due:
// its agency's timer when it has one, otherwise the centre's.
func (s *State) TimerForUnit(u *Unit) int {
	if u.Status == MarkedOut && u.MarkOut != nil {
		if t, ok := s.Settings.MarkOutType(u.MarkOut.Code); ok {
			return t.Minutes
		}
	}
	if u.Status == OnScene && u.Incident != "" {
		if in := s.Incidents[u.Incident]; in != nil {
			if ct := s.CallTypes[in.Type]; ct != nil && ct.CheckMinutes > 0 {
				return ct.CheckMinutes
			}
		}
	}
	if g := s.AgencyOfUnit(u); g != nil {
		if m, ok := g.Timers[u.Status]; ok {
			return m
		}
	}
	return s.Settings.Timers[u.Status]
}

// DispositionsFor are the closing codes for an incident: its agency's, or the centre's.
func (s *State) DispositionsFor(in *Incident) []string {
	if in != nil {
		if g := s.Agencies[in.Agency]; g != nil && len(g.Dispositions) > 0 {
			return g.Dispositions
		}
	}
	return s.Settings.Dispositions
}

// CallTypeFor reports whether a call type is used by an agency (a type for no agency in particular is
// for every agency).
func (c *CallType) For(agency string) bool {
	return len(c.Agencies) == 0 || agency == "" || slices.Contains(c.Agencies, agency)
}

// incidentAgency decides a new incident's agency: the one asked for, the call type's only agency, or the
// creator's working agency. The caller holds e.mu.
func (e *Engine) incidentAgency(a Actor, n NewIncident) (string, error) {
	if len(e.st.Agencies) == 0 {
		return "", nil
	}
	if n.Agency != "" {
		code := strings.ToUpper(n.Agency)
		if err := e.checkAgency("agency", code); err != nil {
			return "", err
		}
		return code, nil
	}
	if ct := e.st.CallTypes[n.Type]; ct != nil && len(ct.Agencies) == 1 {
		return ct.Agencies[0], nil
	}
	if ct := e.st.CallTypes[n.Type]; ct != nil && len(ct.Agencies) > 1 && slices.Contains(ct.Agencies, a.Agency) {
		return a.Agency, nil
	}
	if a.Agency != "" && e.st.Agencies[a.Agency] != nil {
		if ct := e.st.CallTypes[n.Type]; ct != nil && len(ct.Agencies) > 1 {
			return ct.Agencies[0], nil
		}
		return a.Agency, nil
	}
	if ct := e.st.CallTypes[n.Type]; ct != nil && len(ct.Agencies) > 0 {
		return ct.Agencies[0], nil
	}
	return "", nil
}

// incidentPrefix is the prefix an incident of an agency is numbered under.
func (s *State) incidentPrefix(agency string) string {
	if g := s.Agencies[agency]; g != nil && g.IncidentPrefix != "" {
		return g.IncidentPrefix
	}
	return s.Settings.IncidentPrefix
}

type incAgencyData struct {
	Agency    string   `json:"agency"`
	Assisting []string `json:"assisting"`
	Reason    string   `json:"reason,omitempty"`
}

// SetIncidentAgency sets an incident's primary agency and the agencies assisting it.
func (e *Engine) SetIncidentAgency(a Actor, number, primary string, assisting []string, reason string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	in := e.st.Incidents[number]
	if in == nil {
		return problem("incident", "no incident %s", number)
	}
	primary = strings.ToUpper(strings.TrimSpace(primary))
	if primary == "" {
		return problem("agency", "an incident has a primary agency")
	}
	if err := e.checkAgency("agency", primary); err != nil {
		return err
	}
	var as []string
	for _, x := range assisting {
		x = strings.ToUpper(strings.TrimSpace(x))
		if x == "" || x == primary || slices.Contains(as, x) {
			continue
		}
		if err := e.checkAgency("assisting", x); err != nil {
			return err
		}
		as = append(as, x)
	}
	if primary != in.Agency && in.Agency != "" && strings.TrimSpace(reason) == "" {
		return problem("reason", "say why the primary agency changes")
	}
	_, err := e.write(a, EvIncAgency, refs{Incident: number}, incAgencyData{Agency: primary, Assisting: as, Reason: strings.TrimSpace(reason)})
	return err
}

// Agencies on an incident: the primary, then every other agency with a unit on it or named assisting.
func (s *State) IncidentAgencies(in *Incident) (primary string, assisting []string) {
	assisting = append([]string{}, in.Assisting...)
	for _, as := range in.Units {
		if u := s.Units[as.Unit]; u != nil && u.Agency != "" && u.Agency != in.Agency && !slices.Contains(assisting, u.Agency) {
			assisting = append(assisting, u.Agency)
		}
	}
	return in.Agency, assisting
}

func (s *State) applyAgencies(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvAgencySaved:
		g, err := decode[Agency](e)
		if err != nil {
			return true, err
		}
		g.Updated, g.By = e.At, e.Actor
		s.Agencies[g.Code] = &g
	case EvAgencyRemoved:
		v, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		delete(s.Agencies, v.ID)
	case EvStaffSaved:
		p, err := decode[Staff](e)
		if err != nil {
			return true, err
		}
		p.Updated, p.By = e.At, e.Actor
		s.Staff[p.Username] = &p
	case EvCoverage:
		c, err := decode[WorkCoverage](e)
		if err != nil {
			return true, err
		}
		c.At = e.At
		s.Coverage[c.Username] = &c
	case EvIncAgency:
		v, err := decode[incAgencyData](e)
		if err != nil {
			return true, err
		}
		if in := s.Incidents[e.Incident]; in != nil {
			old := in.Agency
			in.Agency, in.Assisting = v.Agency, v.Assisting
			text := "Primary agency " + v.Agency
			if old != "" && old != v.Agency {
				text = "Primary agency " + old + " → " + v.Agency
			}
			if len(v.Assisting) > 0 {
				text += "; assisting " + strings.Join(v.Assisting, ", ")
			}
			in.add(e, "change", text+suffix(v.Reason))
		}
	default:
		return false, nil
	}
	return true, nil
}
