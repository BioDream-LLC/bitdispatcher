package cad

// People on an incident entered top to bottom: role, first, middle and last name, where they are, their
// telephone numbers, their description, then their identifiers. Each name part is its own field so a
// search for the first name John finds every John. A Social Security number is kept but shown masked
// except to those the agency allows, and opening it is recorded. An officer-safety concern must say what
// the concern is, why it applies, its source and what responders should do; a permanent warning goes to
// the agency's review as a person flag rather than becoming one silently.

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// PersonPhone is one of a person's telephone numbers.
type PersonPhone struct {
	Number   string `json:"number"`
	Type     string `json:"type,omitempty"` // mobile, home, work, other
	Ext      string `json:"ext,omitempty"`
	Verified bool   `json:"verified,omitempty"`
	Source   string `json:"source,omitempty"` // caller, ani, ali, records, other
}

// SafetyConcern explains an officer-safety concern about a person.
type SafetyConcern struct {
	Title       string `json:"title"`
	Reason      string `json:"reason"`            // the specific reason or behaviour seen
	Weapons     string `json:"weapons,omitempty"` // known weapons or threat information
	Instruction string `json:"instruction"`       // what responders should do
	Source      string `json:"source"`            // how it was reported
	Reliability string `json:"reliability,omitempty"`
	// Scope: incident (a note that follows this incident only) or permanent (proposed as a person flag for
	// the agency's review).
	Scope string `json:"scope"`
	Flag  string `json:"flag,omitempty"` // the person flag proposed
}

// VehicleLink relates a vehicle to a person on the incident.
type VehicleLink struct {
	Person   string `json:"person"`   // the person's ID on the incident (P1)
	Relation string `json:"relation"` // driver, owner, lessee, passenger, occupant, other
}

// VehicleRelations are the relations a person can have to a vehicle.
var VehicleRelations = []string{"driver", "owner", "lessee", "passenger", "occupant", "other"}

// PersonRoles and VehicleRoles are the roles offered (agencies keep their own words in notes).
var (
	PersonRoles  = []string{"caller", "victim", "suspect", "witness", "complainant", "subject", "patient", "other"}
	VehicleRoles = []string{"suspect", "victim", "involved", "stolen", "recovered", "abandoned", "wanted", "traffic-stop", "evidence", "impounded", "towed", "other"}
)

// PersonKey identifies a person across incidents: last and first name and date of birth.
func PersonKey(p Person) string {
	last, first := p.Last, p.First
	if last == "" && first == "" {
		last, first, _ = splitName(p.Name)
	}
	if last == "" {
		return ""
	}
	return "P:" + normPart(last) + "|" + normPart(first) + "|" + strings.TrimSpace(p.DOB)
}

// splitName reads "LAST, FIRST MIDDLE" or "First Middle Last".
func splitName(n string) (last, first, middle string) {
	n = strings.TrimSpace(n)
	if n == "" {
		return
	}
	if i := strings.Index(n, ","); i >= 0 {
		last = strings.TrimSpace(n[:i])
		rest := strings.Fields(n[i+1:])
		if len(rest) > 0 {
			first = rest[0]
		}
		if len(rest) > 1 {
			middle = strings.Join(rest[1:], " ")
		}
		return
	}
	f := strings.Fields(n)
	switch len(f) {
	case 1:
		last = f[0]
	case 2:
		first, last = f[0], f[1]
	default:
		first, middle, last = f[0], strings.Join(f[1:len(f)-1], " "), f[len(f)-1]
	}
	return
}

// fullName makes "LAST, FIRST MIDDLE SUFFIX".
func fullName(p Person) string {
	if p.Last == "" && p.First == "" {
		return p.Name
	}
	s := strings.ToUpper(p.Last)
	rest := strings.TrimSpace(strings.Join([]string{p.First, p.Middle, p.Suffix}, " "))
	rest = strings.Join(strings.Fields(rest), " ")
	if rest != "" {
		if s != "" {
			s += ", "
		}
		s += strings.ToUpper(rest)
	}
	return s
}

func digitsOnly(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}

// MaskSSN shows only the last four digits.
func MaskSSN(s string) string {
	d := digitsOnly(s)
	if d == "" {
		return ""
	}
	if len(d) < 4 {
		return "***"
	}
	return "***-**-" + d[len(d)-4:]
}

// MaskID shows only the last four characters of a licence or other number.
func MaskID(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 4 {
		return strings.Repeat("*", len(s))
	}
	return strings.Repeat("*", len(s)-4) + s[len(s)-4:]
}

// AgeFrom is the age on a day from a date of birth (YYYY-MM-DD), or -1.
func AgeFrom(dob string, on time.Time) int {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(dob))
	if err != nil {
		return -1
	}
	age := on.Year() - t.Year()
	if on.YearDay() < t.YearDay() {
		age--
	}
	return age
}

// normPerson tidies a person and checks what the call-taker must give.
func (e *Engine) normPerson(p *Person) error {
	clean := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	p.First, p.Middle, p.Last, p.Suffix = clean(p.First), clean(p.Middle), clean(p.Last), clean(p.Suffix)
	if p.First == "" && p.Last == "" && p.Middle == "" && p.Name != "" {
		p.Last, p.First, p.Middle = splitName(p.Name)
	}
	p.Name = fullName(*p)
	p.Roles = cleanList(append([]string{p.Role}, p.Roles...))
	for i := range p.Roles {
		p.Roles[i] = strings.ToLower(p.Roles[i])
	}
	p.Roles = dedupe(p.Roles)
	if len(p.Roles) > 0 {
		p.Role = p.Roles[0]
	}
	switch p.LocKind {
	case "", "incident", "other", "unknown", "transient":
	default:
		return problem("locKind", "where is the person: at the incident, somewhere else, unknown or transient")
	}
	var phones []PersonPhone
	for _, ph := range p.Phones {
		ph.Number = strings.TrimSpace(ph.Number)
		if ph.Number == "" {
			continue
		}
		if len(digitsOnly(ph.Number)) < 3 {
			return problem("phones", "%q is not a telephone number", ph.Number)
		}
		phones = append(phones, ph)
	}
	p.Phones = phones
	if p.Phone != "" && !slices.ContainsFunc(p.Phones, func(x PersonPhone) bool { return digitsOnly(x.Number) == digitsOnly(p.Phone) }) {
		p.Phones = append([]PersonPhone{{Number: p.Phone, Source: "caller"}}, p.Phones...)
	}
	if len(p.Phones) > 0 {
		p.Phone = p.Phones[0].Number
	}
	if p.SSN != "" {
		d := digitsOnly(p.SSN)
		if len(d) != 9 {
			return problem("ssn", "a Social Security number has 9 digits")
		}
		p.SSN = d[:3] + "-" + d[3:5] + "-" + d[5:]
	}
	p.DL, p.DLState = strings.ToUpper(strings.TrimSpace(p.DL)), strings.ToUpper(strings.TrimSpace(p.DLState))
	if p.DOB != "" {
		if _, err := time.Parse("2006-01-02", p.DOB); err != nil {
			if n, err2 := strconv.Atoi(strings.TrimSpace(p.DOB)); err2 == nil && n > 0 && n < 130 {
				// An age given where the date of birth goes is kept as the approximate age.
				p.ApproxAge, p.DOB = strconv.Itoa(n), ""
			} else {
				return problem("dob", "a date of birth is a date (YYYY-MM-DD); an age goes in the approximate age")
			}
		}
	}
	if p.Safety {
		n := p.SafetyNote
		switch {
		case n == nil || strings.TrimSpace(n.Title) == "":
			return problem("safety.title", "give the officer-safety alert a short title")
		case strings.TrimSpace(n.Reason) == "":
			return problem("safety.reason", "say what the concern is and why it applies")
		case strings.TrimSpace(n.Source) == "":
			return problem("safety.source", "say how it was reported")
		case strings.TrimSpace(n.Instruction) == "":
			return problem("safety.instruction", "say what responders should do")
		}
		if n.Scope == "" {
			n.Scope = "incident"
		}
		if n.Scope != "incident" && n.Scope != "permanent" {
			return problem("safety.scope", "an incident-only note or a proposed permanent warning")
		}
		if n.Scope == "permanent" && p.Last == "" {
			return problem("last", "a permanent warning needs the person's last name")
		}
	} else {
		p.SafetyNote = nil
	}
	return nil
}

// proposePersonFlag writes the person flag for a permanent officer-safety warning (the caller holds e.mu).
func (e *Engine) proposePersonFlag(a Actor, number string, p *Person) {
	n := p.SafetyNote
	if n == nil || n.Scope != "permanent" || n.Flag != "" {
		return
	}
	key := PersonKey(*p)
	for _, f := range e.st.Flags {
		if f.Scope.Person == key && f.Category == "officer-safety" && (f.State == FlagActive || f.State == FlagProposed) {
			n.Flag = f.ID
			return
		}
	}
	cat, _ := e.st.Settings.Flags.Category("officer-safety")
	f := Flag{ID: e.newID("FL", func(id string) bool { return e.st.Flags[id] != nil }), Version: 1, Category: "officer-safety",
		Title: n.Title, Warning: strings.TrimSpace(n.Reason + "; " + n.Weapons), Instruction: n.Instruction, Severity: firstNonEmpty(cat.Severity, "critical"),
		Source: n.Source, Reliability: n.Reliability, Scope: FlagScope{Person: key, PersonName: p.Name}, State: FlagProposed, Incident: number,
		Creator: a.User, Created: e.now(), Effective: e.now()}
	f.Warning = strings.TrimSuffix(strings.TrimSpace(f.Warning), ";")
	if cat.mayApprove(a) && !e.st.StaffOf(a.User).Trainee {
		f.State = FlagActive
	}
	if cat.ReviewDays > 0 {
		f.Review = e.now().Add(time.Duration(cat.ReviewDays) * 24 * time.Hour)
	}
	if _, err := e.write(a, EvFlagSaved, refs{Incident: number}, flagData{Flag: f}); err == nil {
		n.Flag = f.ID
	}
}

// PersonQuery is a structured person search: each name part on its own, and the identifiers that narrow it.
type PersonQuery struct {
	First    string `json:"first,omitempty"`
	Middle   string `json:"middle,omitempty"`
	Last     string `json:"last,omitempty"`
	Alias    string `json:"alias,omitempty"`
	DOB      string `json:"dob,omitempty"`
	Age      int    `json:"age,omitempty"` // approximate: within 5 years
	Phone    string `json:"phone,omitempty"`
	Address  string `json:"address,omitempty"`
	SSN      string `json:"ssn,omitempty"` // for those allowed: matched, never shown
	DL       string `json:"dl,omitempty"`
	Sex      string `json:"sex,omitempty"`
	Race     string `json:"race,omitempty"`
	Plate    string `json:"plate,omitempty"` // a vehicle the person is linked to
	Phonetic bool   `json:"phonetic,omitempty"`
	Partial  bool   `json:"partial,omitempty"` // name parts match the start of a name
}

// PersonHit is one person found: their latest record, masked, with the incidents they are on.
type PersonHit struct {
	Key       string      `json:"key"`
	Name      string      `json:"name"`
	First     string      `json:"first,omitempty"`
	Middle    string      `json:"middle,omitempty"`
	Last      string      `json:"last,omitempty"`
	DOB       string      `json:"dob,omitempty"`
	ApproxAge string      `json:"approxAge,omitempty"`
	Sex       string      `json:"sex,omitempty"`
	Race      string      `json:"race,omitempty"`
	Address   string      `json:"address,omitempty"`
	Phone     string      `json:"phone,omitempty"`
	SSN       string      `json:"ssn,omitempty"` // masked
	DL        string      `json:"dl,omitempty"`  // masked
	Aliases   []string    `json:"aliases,omitempty"`
	Incidents []PersonRef `json:"incidents"`
	Flags     []string    `json:"flags,omitempty"` // titles of active warnings
	Why       []string    `json:"why"`             // what matched
}

// PersonRef is a person on one incident.
type PersonRef struct {
	Incident string    `json:"incident"`
	Person   string    `json:"person"`
	Role     string    `json:"role"`
	At       time.Time `json:"at"`
}

// soundex is the American Soundex code of a name, for sounds-like searching.
func soundex(s string) string {
	s = strings.ToUpper(normPart(s))
	if s == "" {
		return ""
	}
	code := func(r rune) byte {
		switch r {
		case 'B', 'F', 'P', 'V':
			return '1'
		case 'C', 'G', 'J', 'K', 'Q', 'S', 'X', 'Z':
			return '2'
		case 'D', 'T':
			return '3'
		case 'L':
			return '4'
		case 'M', 'N':
			return '5'
		case 'R':
			return '6'
		case 'H', 'W':
			return 'h'
		}
		return '0'
	}
	var out []byte
	rs := []rune(s)
	var first rune
	for _, r := range rs {
		if r >= 'A' && r <= 'Z' {
			first = r
			break
		}
	}
	if first == 0 {
		return ""
	}
	out = append(out, byte(first))
	last := code(first)
	started := false
	for _, r := range rs {
		if !started {
			if r == first {
				started = true
			}
			continue
		}
		if r < 'A' || r > 'Z' {
			continue
		}
		c := code(r)
		if c == 'h' {
			continue
		}
		if c != '0' && c != last {
			out = append(out, c)
			if len(out) == 4 {
				break
			}
		}
		last = c
	}
	for len(out) < 4 {
		out = append(out, '0')
	}
	return string(out)
}

// FindPersons searches the people on incidents. Each name part is matched on its own; the other fields
// narrow the result. It returns one hit per person (by PersonKey), newest record first.
func (s *State) FindPersons(q PersonQuery, limit int) []PersonHit {
	if limit <= 0 {
		limit = 50
	}
	nameMatch := func(want, have string) bool {
		if want == "" {
			return true
		}
		w, h := normPart(want), normPart(have)
		if h == "" {
			return false
		}
		if w == h || (q.Partial && strings.HasPrefix(h, w)) {
			return true
		}
		return q.Phonetic && soundex(w) == soundex(h)
	}
	type acc struct {
		hit  PersonHit
		at   time.Time
		why  map[string]bool
		flag bool
	}
	found := map[string]*acc{}
	plateOf := func(in *Incident, pid string) []string {
		var out []string
		for _, v := range in.Vehicles {
			for _, l := range v.Links {
				if l.Person == pid && v.Plate != "" {
					out = append(out, v.Plate)
				}
			}
		}
		return out
	}
	for _, in := range s.Incidents {
		for _, p := range in.Persons {
			if p.Removed {
				continue
			}
			last, first, middle := p.Last, p.First, p.Middle
			if last == "" && first == "" {
				last, first, middle = splitName(p.Name)
			}
			why := []string{}
			ok := true
			check := func(cond bool, w string, given bool) {
				if !given {
					return
				}
				if cond {
					why = append(why, w)
				} else {
					ok = false
				}
			}
			check(nameMatch(q.First, first), "first name", q.First != "")
			check(nameMatch(q.Middle, middle), "middle name", q.Middle != "")
			check(nameMatch(q.Last, last), "last name", q.Last != "")
			check(slices.ContainsFunc(p.Aliases, func(a string) bool { return nameMatch(q.Alias, a) || strings.Contains(normPart(a), normPart(q.Alias)) }), "alias", q.Alias != "")
			check(p.DOB == q.DOB, "date of birth", q.DOB != "")
			if q.Age > 0 {
				a := AgeFrom(p.DOB, s.lastAt)
				if a < 0 {
					a, _ = strconv.Atoi(p.ApproxAge)
				}
				check(a > 0 && a >= q.Age-5 && a <= q.Age+5, "age", true)
			}
			check(q.Phone != "" && slices.ContainsFunc(append(p.Phones, PersonPhone{Number: p.Phone}), func(x PersonPhone) bool {
				d := digitsOnly(x.Number)
				return d != "" && strings.HasSuffix(d, digitsOnly(q.Phone))
			}), "telephone", q.Phone != "")
			check(strings.Contains(normKey(p.Address), normKey(q.Address)), "address", q.Address != "")
			check(digitsOnly(p.SSN) != "" && digitsOnly(p.SSN) == digitsOnly(q.SSN), "Social Security number", q.SSN != "")
			check(p.DL != "" && strings.EqualFold(p.DL, strings.TrimSpace(q.DL)), "licence", q.DL != "")
			check(strings.EqualFold(p.Sex, q.Sex), "sex", q.Sex != "")
			check(strings.EqualFold(p.Race, q.Race), "race", q.Race != "")
			check(slices.ContainsFunc(plateOf(in, p.ID), func(pl string) bool { return strings.EqualFold(pl, q.Plate) }), "vehicle", q.Plate != "")
			if !ok || len(why) == 0 {
				continue
			}
			key := PersonKey(p)
			if key == "" {
				key = in.Number + "/" + p.ID
			}
			a := found[key]
			if a == nil {
				a = &acc{why: map[string]bool{}}
				found[key] = a
			}
			at := p.Added
			if !p.Updated.IsZero() {
				at = p.Updated
			}
			if at.After(a.at) || a.hit.Name == "" {
				a.at = at
				ssn, dl := MaskSSN(p.SSN), ""
				if p.DL != "" {
					dl = MaskID(p.DL) + " " + p.DLState
				}
				a.hit = PersonHit{Key: key, Name: p.Name, First: first, Middle: middle, Last: last, DOB: p.DOB, ApproxAge: p.ApproxAge, Sex: p.Sex,
					Race: p.Race, Address: p.Address, Phone: p.Phone, SSN: ssn, DL: strings.TrimSpace(dl), Aliases: p.Aliases, Incidents: a.hit.Incidents}
			}
			a.hit.Incidents = append(a.hit.Incidents, PersonRef{Incident: in.Number, Person: p.ID, Role: p.Role, At: p.Added})
			for _, w := range why {
				a.why[w] = true
			}
		}
	}
	out := make([]PersonHit, 0, len(found))
	for k, a := range found {
		for w := range a.why {
			a.hit.Why = append(a.hit.Why, w)
		}
		sort.Strings(a.hit.Why)
		sort.Slice(a.hit.Incidents, func(i, j int) bool { return a.hit.Incidents[i].At.After(a.hit.Incidents[j].At) })
		for _, f := range s.FlagsForPerson(k, s.lastAt) {
			a.hit.Flags = append(a.hit.Flags, f.Title)
		}
		out = append(out, a.hit)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Last != out[j].Last {
			return strings.ToUpper(out[i].Last) < strings.ToUpper(out[j].Last)
		}
		return strings.ToUpper(out[i].First) < strings.ToUpper(out[j].First)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// PossibleMatches are people already on record who may be this person: shown before a duplicate is
// made, never forcing a merge during the call.
func (s *State) PossibleMatches(p Person) []PersonHit {
	last, first, _ := p.Last, p.First, ""
	if last == "" && first == "" {
		last, first, _ = splitName(p.Name)
	}
	if last == "" {
		return nil
	}
	hits := s.FindPersons(PersonQuery{Last: last, First: first, Phonetic: true}, 10)
	if p.DOB != "" {
		var keep []PersonHit
		for _, h := range hits {
			if h.DOB == "" || h.DOB == p.DOB {
				keep = append(keep, h)
			}
		}
		hits = keep
	}
	return hits
}

// PersonIdent is a person's protected identifiers, for those allowed to open them (the opening is recorded).
type PersonIdent struct {
	Incident string `json:"incident"`
	Person   string `json:"person"`
	SSN      string `json:"ssn,omitempty"`
	DL       string `json:"dl,omitempty"`
	DLState  string `json:"dlState,omitempty"`
}

// EvIdentOpened records someone opening a person's protected identifiers.
const EvIdentOpened = "person.identifiers-opened"

// MayOpenIdentifiers is who may see a full Social Security or licence number (Settings.Persons).
func (s *State) MayOpenIdentifiers(a Actor) bool {
	d := s.Settings.Persons.IdentDuties
	if len(d) == 0 {
		d = []string{DutyPolice, DutySupervisor, DutyRecords, DutyDispatcher}
	}
	return a.Role == "admin" || slices.Contains(d, DutyOf(a))
}

// PersonPolicy is the agency's rule for people's protected identifiers.
type PersonPolicy struct {
	IdentDuties []string `json:"identDuties,omitempty"` // may open full identifiers (empty: police, dispatchers, supervisors, records)
}

// OpenIdentifiers returns a person's full identifiers to someone allowed, and records it.
func (e *Engine) OpenIdentifiers(a Actor, number, pid, purpose string) (PersonIdent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	in := e.st.Incidents[number]
	if in == nil {
		return PersonIdent{}, problem("incident", "no incident %s", number)
	}
	i := slices.IndexFunc(in.Persons, func(p Person) bool { return p.ID == pid })
	if i < 0 {
		return PersonIdent{}, problem("person", "no person %s on %s", pid, number)
	}
	if !e.st.MayOpenIdentifiers(a) {
		e.write(a, EvAccessRefused, refs{Incident: number}, map[string]string{"person": pid, "why": "identifiers"})
		return PersonIdent{}, dutyProblem("person", "your duty (%s) cannot open full identifiers", DutyOf(a))
	}
	if strings.TrimSpace(purpose) == "" {
		return PersonIdent{}, problem("purpose", "say why you are opening them")
	}
	p := in.Persons[i]
	if _, err := e.write(a, EvIdentOpened, refs{Incident: number}, map[string]string{"person": pid, "purpose": strings.TrimSpace(purpose)}); err != nil {
		return PersonIdent{}, err
	}
	return PersonIdent{Incident: number, Person: pid, SSN: p.SSN, DL: p.DL, DLState: p.DLState}, nil
}

// MaskPerson is a person as the board shows them: protected identifiers masked.
func MaskPerson(p Person) Person {
	if p.SSN != "" {
		p.SSN = MaskSSN(p.SSN)
	}
	if p.DL != "" {
		p.DL = MaskID(p.DL)
	}
	return p
}

// fmtPerson is a person on one line for the timeline.
func fmtPerson(p Person) string {
	s := p.Name
	if s == "" {
		s = "(no name)"
	}
	return fmt.Sprintf("%s (%s)", s, strings.Join(p.Roles, ", "))
}

// without is xs without v.
func without(xs []string, v string) []string {
	var out []string
	for _, x := range xs {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}
