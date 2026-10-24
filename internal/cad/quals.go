package cad

// Personnel qualifications and special teams: each agency's controlled skills file. A qualification (a
// team, a specialty, a training role, a credential) is defined by the agency that owns it, with its levels,
// how long it lasts, when renewal is due, its prerequisites and whether it must be verified. A person's
// credential has a state (trainee, qualified, instructor, evaluator, expired, suspended, restricted,
// temporarily unavailable), dates, the issuing authority and who verified it; every change is a ledger
// event, so the whole history is kept. Only a valid credential counts: qualified (or above), verified when
// the agency requires it, not expired or suspended. An unverified self-entry never makes a unit capable.
//
// A unit's live capability comes from the apparatus and the qualified people signed onto it now (crew.go):
// a HazMat unit without its required complement of HazMat technicians is not HazMat-capable, and shows
// Limited with the reason. The agency decides whether dispatching a short unit is only warned, needs a
// supervisor's override, or is blocked. A qualification from another agency counts only through an
// approved equivalency (its scope and expiry kept), never by merging records.

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
	EvQualSaved          = "qual.saved"
	EvQualRemoved        = "qual.removed"
	EvCredentialSaved    = "credential.saved"    // a person's credential was added, changed, verified, suspended or renewed
	EvEquivalencySaved   = "equivalency.saved"   // another agency's qualification accepted as one of ours
	EvEquivalencyRemoved = "equivalency.removed" //
	EvCredentialReminder = "credential.reminder" // a credential's renewal is due, or it expired
)

// Qualification categories.
var QualCategories = []string{"team", "specialty", "training", "credential"}

// Credential states.
const (
	CredTrainee     = "trainee"
	CredQualified   = "qualified"
	CredInstructor  = "instructor"
	CredEvaluator   = "evaluator"
	CredExpired     = "expired"
	CredSuspended   = "suspended"
	CredRestricted  = "restricted"
	CredUnavailable = "unavailable"
)

// CredStates in the order they are offered.
var CredStates = []string{CredTrainee, CredQualified, CredInstructor, CredEvaluator, CredRestricted, CredUnavailable, CredSuspended, CredExpired}

// QualDef is a qualification an agency tracks.
type QualDef struct {
	Code     string   `json:"code"` // hazmat-tech, swift-water, fto: what crews and capability needs name
	Name     string   `json:"name"`
	Category string   `json:"category"`         // team, specialty, training, credential
	Agency   string   `json:"agency,omitempty"` // the agency that owns it (empty: the centre's)
	Levels   []string `json:"levels,omitempty"` // Member, Technician, Specialist, Team Leader…
	// Months a credential lasts (0: does not expire); RenewDays before expiry renewal is due (0: 60).
	Months    int      `json:"months,omitempty"`
	RenewDays int      `json:"renewDays,omitempty"`
	Prereqs   []string `json:"prereqs,omitempty"` // qualifications a person must hold valid first
	// Verify: a credential counts only once a supervisor or administrator verified it.
	Verify bool `json:"verify,omitempty"`
	// SelfAvailable: members may declare themselves available for the team without approval.
	SelfAvailable bool      `json:"selfAvailable,omitempty"`
	Documents     string    `json:"documents,omitempty"` // what must be attached (a certificate, a roster)
	Note          string    `json:"note,omitempty"`
	Updated       time.Time `json:"updated,omitzero"`
	By            string    `json:"by,omitempty"`
}

// Credential is one person's qualification.
type Credential struct {
	Qual        string    `json:"qual"`
	Level       string    `json:"level,omitempty"`
	State       string    `json:"state"`
	Issuer      string    `json:"issuer,omitempty"`
	Issued      time.Time `json:"issued,omitzero"`
	Expires     time.Time `json:"expires,omitzero"`
	Restriction string    `json:"restriction,omitempty"`
	Files       []string  `json:"files,omitempty"` // attached certificates (file IDs)
	VerifiedBy  string    `json:"verifiedBy,omitempty"`
	VerifiedAt  time.Time `json:"verifiedAt,omitzero"`
	Note        string    `json:"note,omitempty"`
	Updated     time.Time `json:"updated,omitzero"`
	By          string    `json:"by,omitempty"`
	// Verify asks for the credential to be marked verified by the person saving it (a supervisor).
	Verify bool `json:"verify,omitempty"`
	// Reason is why it changed (suspended, restricted, corrected).
	Reason string `json:"reason,omitempty"`
}

// Equivalency accepts another agency's qualification as one of ours, within a scope and until a date.
type Equivalency struct {
	ID         string    `json:"id"`
	FromAgency string    `json:"fromAgency"`
	From       string    `json:"from"` // their qualification code
	To         string    `json:"to"`   // ours
	Agency     string    `json:"agency,omitempty"`
	Scope      string    `json:"scope,omitempty"` // mutual aid only, county-wide…
	Expires    time.Time `json:"expires,omitzero"`
	ApprovedBy string    `json:"approvedBy,omitempty"`
	At         time.Time `json:"at,omitzero"`
}

var reQual = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// SaveQual adds or changes a qualification definition.
func (e *Engine) SaveQual(a Actor, q QualDef) error {
	q.Code = strings.ToLower(strings.TrimSpace(q.Code))
	q.Name = strings.TrimSpace(q.Name)
	if !reQual.MatchString(q.Code) {
		return problem("code", "a qualification code is lower case letters, digits and dashes: hazmat-tech")
	}
	if q.Name == "" {
		return problem("name", "name the qualification")
	}
	if !slices.Contains(QualCategories, q.Category) {
		return problem("category", "a category is team, specialty, training or credential")
	}
	if q.Months < 0 || q.Months > 240 || q.RenewDays < 0 || q.RenewDays > 730 {
		return problem("months", "a qualification lasts up to 240 months, renewal up to 730 days ahead")
	}
	for i, p := range q.Prereqs {
		q.Prereqs[i] = strings.ToLower(strings.TrimSpace(p))
	}
	q.Prereqs = cleanList(q.Prereqs)
	q.Agency = strings.ToUpper(strings.TrimSpace(q.Agency))
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.checkAgency("agency", q.Agency); err != nil {
		return err
	}
	if slices.Contains(q.Prereqs, q.Code) {
		return problem("prereqs", "a qualification cannot need itself")
	}
	_, err := e.write(a, EvQualSaved, refs{}, q)
	return err
}

// RemoveQual removes a qualification no credential uses.
func (e *Engine) RemoveQual(a Actor, code string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.Quals[code] == nil {
		return problem("code", "no qualification %s", code)
	}
	for _, m := range e.st.Members {
		for _, c := range m.Creds {
			if c.Qual == code {
				return problem("code", "%s holds %s; end their credential first", m.Name, code)
			}
		}
	}
	_, err := e.write(a, EvQualRemoved, refs{}, idData{code})
	return err
}

type credData struct {
	Member string     `json:"member"`
	Cred   Credential `json:"cred"`
	Remove bool       `json:"remove,omitempty"`
}

// SaveCredential adds or changes a person's credential. A supervisor or administrator may mark it
// verified; anyone else's entry is unverified (and, where the qualification needs verifying, does not
// count until it is).
func (e *Engine) SaveCredential(a Actor, member string, c Credential) error {
	c.Qual = strings.ToLower(strings.TrimSpace(c.Qual))
	c.State = strings.ToLower(strings.TrimSpace(c.State))
	if c.State == "" {
		c.State = CredQualified
	}
	if !slices.Contains(CredStates, c.State) {
		return problem("state", "a credential is trainee, qualified, instructor, evaluator, restricted, unavailable, suspended or expired")
	}
	if (c.State == CredSuspended || c.State == CredRestricted) && strings.TrimSpace(c.Reason) == "" && strings.TrimSpace(c.Restriction) == "" {
		return problem("reason", "say why the credential is %s", c.State)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	m := e.st.Members[member]
	if m == nil {
		return problem("member", "no member %s on the roster", member)
	}
	q := e.st.Quals[c.Qual]
	if q == nil {
		return problem("qual", "no qualification %s: define it in Admin › Qualifications", c.Qual)
	}
	if c.Level != "" && len(q.Levels) > 0 && !slices.Contains(q.Levels, c.Level) {
		return problem("level", "%s's levels are %s", q.Name, strings.Join(q.Levels, ", "))
	}
	if c.Expires.IsZero() && q.Months > 0 && !c.Issued.IsZero() {
		c.Expires = c.Issued.AddDate(0, q.Months, 0)
	}
	if !c.Expires.IsZero() && !c.Issued.IsZero() && c.Expires.Before(c.Issued) {
		return problem("expires", "it expires before it was issued")
	}
	if validState(c.State) {
		now := e.now()
		for _, p := range q.Prereqs {
			if !e.st.holds(m, p, now) {
				pn := p
				if d := e.st.Quals[p]; d != nil {
					pn = d.Name
				}
				return problem("state", "%s needs %s first", q.Name, pn)
			}
		}
	}
	supervisor := a.Role == "supervisor" || a.Role == "admin"
	var old *Credential
	for i := range m.Creds {
		if m.Creds[i].Qual == c.Qual {
			old = &m.Creds[i]
		}
	}
	switch {
	case c.Verify && supervisor:
		c.VerifiedBy, c.VerifiedAt = a.User, e.now()
	case old != nil && supervisor && old.VerifiedBy != "" && !c.Verify:
		c.VerifiedBy, c.VerifiedAt = old.VerifiedBy, old.VerifiedAt // a supervisor's edit keeps the verification
	default:
		c.VerifiedBy, c.VerifiedAt = "", time.Time{}
	}
	c.Verify = false
	_, err := e.write(a, EvCredentialSaved, refs{}, credData{Member: member, Cred: c})
	return err
}

// EndCredential removes a credential from the person's current file (the ledger keeps it).
func (e *Engine) EndCredential(a Actor, member, qual, reason string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	m := e.st.Members[member]
	if m == nil || !slices.ContainsFunc(m.Creds, func(c Credential) bool { return c.Qual == qual }) {
		return problem("qual", "no such credential")
	}
	if strings.TrimSpace(reason) == "" {
		return problem("reason", "say why the credential ends")
	}
	_, err := e.write(a, EvCredentialSaved, refs{}, credData{Member: member, Cred: Credential{Qual: qual, Reason: reason}, Remove: true})
	return err
}

// SaveEquivalency accepts another agency's qualification as ours.
func (e *Engine) SaveEquivalency(a Actor, q Equivalency) error {
	q.FromAgency = strings.ToUpper(strings.TrimSpace(q.FromAgency))
	q.Agency = strings.ToUpper(strings.TrimSpace(q.Agency))
	q.From, q.To = strings.ToLower(strings.TrimSpace(q.From)), strings.ToLower(strings.TrimSpace(q.To))
	if q.FromAgency == "" || q.From == "" || q.To == "" {
		return problem("from", "say whose qualification is accepted as which of ours")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.checkAgency("fromAgency", q.FromAgency); err != nil {
		return err
	}
	if e.st.Quals[q.From] == nil || e.st.Quals[q.To] == nil {
		return problem("to", "both qualifications must be defined")
	}
	if q.FromAgency == q.Agency && q.Agency != "" {
		return problem("fromAgency", "an equivalency is between two agencies")
	}
	if q.ID == "" {
		q.ID = fmt.Sprintf("EQ%d", len(e.st.Equivalencies)+1)
		for e.st.Equivalencies[q.ID] != nil {
			q.ID += "x"
		}
	}
	q.ApprovedBy = a.User
	_, err := e.write(a, EvEquivalencySaved, refs{}, q)
	return err
}

// RemoveEquivalency ends an equivalency.
func (e *Engine) RemoveEquivalency(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.Equivalencies[id] == nil {
		return problem("id", "no equivalency %s", id)
	}
	_, err := e.write(a, EvEquivalencyRemoved, refs{}, idData{id})
	return err
}

func validState(s string) bool {
	return s == CredQualified || s == CredInstructor || s == CredEvaluator || s == CredRestricted
}

// CredValid reports whether a credential counts now, and if not, why.
func (s *State) CredValid(c Credential, now time.Time) (bool, string) {
	q := s.Quals[c.Qual]
	switch {
	case q == nil:
		return false, "no longer defined"
	case !validState(c.State):
		return false, c.State
	case !c.Expires.IsZero() && !now.Before(c.Expires):
		return false, "expired " + c.Expires.Format("2006-01-02")
	case q.Verify && c.VerifiedBy == "":
		return false, "not verified"
	}
	return true, ""
}

// holds reports whether a member holds a qualification validly now: a legacy roster qualification, their
// own credential, or another agency's accepted through an equivalency.
func (s *State) holds(m *Member, qual string, now time.Time) bool {
	if slices.Contains(m.Quals, qual) && s.Quals[qual] == nil {
		return true // a plain roster qualification, not managed as a credential
	}
	for _, c := range m.Creds {
		if ok, _ := s.CredValid(c, now); !ok {
			continue
		}
		if c.Qual == qual {
			return true
		}
		for _, q := range s.Equivalencies {
			if q.From == c.Qual && q.To == qual && (q.Expires.IsZero() || now.Before(q.Expires)) && (m.Agency == "" || m.Agency == q.FromAgency) {
				return true
			}
		}
	}
	return false
}

// ValidQuals are what a member is qualified for now (their own and accepted equivalents).
func (s *State) ValidQuals(m *Member, now time.Time) []string {
	var out []string
	for _, q := range m.Quals {
		if s.Quals[q] == nil && !slices.Contains(out, q) {
			out = append(out, q)
		}
	}
	for _, c := range m.Creds {
		if ok, _ := s.CredValid(c, now); ok {
			if !slices.Contains(out, c.Qual) {
				out = append(out, c.Qual)
			}
			for _, q := range s.Equivalencies {
				if q.From == c.Qual && (q.Expires.IsZero() || now.Before(q.Expires)) && (m.Agency == "" || m.Agency == q.FromAgency) && !slices.Contains(out, q.To) {
					out = append(out, q.To)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// ExpiringCredential is a credential whose renewal is due or that expired.
type ExpiringCredential struct {
	Member  string    `json:"member"`
	Name    string    `json:"name"`
	Agency  string    `json:"agency,omitempty"`
	Qual    string    `json:"qual"`
	Expires time.Time `json:"expires"`
	Expired bool      `json:"expired"`
}

// Expiring are the credentials due for renewal within their window, or expired in the last 90 days.
func (s *State) Expiring(now time.Time) []ExpiringCredential {
	out := []ExpiringCredential{}
	for _, m := range s.Members {
		for _, c := range m.Creds {
			q := s.Quals[c.Qual]
			if q == nil || c.Expires.IsZero() || c.State == CredSuspended {
				continue
			}
			win := q.RenewDays
			if win == 0 {
				win = 60
			}
			if now.After(c.Expires.AddDate(0, 0, 90)) || now.Before(c.Expires.AddDate(0, 0, -win)) {
				continue
			}
			out = append(out, ExpiringCredential{Member: m.ID, Name: m.Name, Agency: m.Agency, Qual: c.Qual, Expires: c.Expires, Expired: !now.Before(c.Expires)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Expires.Before(out[j].Expires) })
	return out
}

// RemindCredentials records a reminder once for each credential that came due or expired, so the
// supervisors' list and the notice go out once. It returns the reminders written.
func (e *Engine) RemindCredentials() ([]ExpiringCredential, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []ExpiringCredential
	for _, x := range e.st.Expiring(e.now()) {
		k := x.Member + "|" + x.Qual + "|" + x.Expires.Format(time.RFC3339)
		if x.Expired {
			k += "|expired"
		}
		if e.st.reminded[k] {
			continue
		}
		if _, err := e.write(SystemActor, EvCredentialReminder, refs{}, map[string]any{"key": k, "member": x.Member, "qual": x.Qual, "expires": x.Expires, "expired": x.Expired}); err != nil {
			return out, err
		}
		out = append(out, x)
	}
	return out, nil
}

func (s *State) applyQuals(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvQualSaved:
		q, err := decode[QualDef](e)
		if err != nil {
			return true, err
		}
		q.Updated, q.By = e.At, e.Actor
		s.Quals[q.Code] = &q
	case EvQualRemoved:
		v, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		delete(s.Quals, v.ID)
	case EvCredentialSaved:
		v, err := decode[credData](e)
		if err != nil {
			return true, err
		}
		m := s.Members[v.Member]
		if m == nil {
			return true, nil
		}
		m.Creds = slices.DeleteFunc(m.Creds, func(c Credential) bool { return c.Qual == v.Cred.Qual })
		if !v.Remove {
			c := v.Cred
			c.Updated, c.By = e.At, e.Actor
			m.Creds = append(m.Creds, c)
			sort.Slice(m.Creds, func(i, j int) bool { return m.Creds[i].Qual < m.Creds[j].Qual })
		}
	case EvEquivalencySaved:
		q, err := decode[Equivalency](e)
		if err != nil {
			return true, err
		}
		q.At = e.At
		s.Equivalencies[q.ID] = &q
	case EvEquivalencyRemoved:
		v, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		delete(s.Equivalencies, v.ID)
	case EvCredentialReminder:
		v, err := decode[map[string]any](e)
		if err != nil {
			return true, err
		}
		if k, ok := v["key"].(string); ok {
			s.reminded[k] = true
		}
	default:
		return false, nil
	}
	return true, nil
}
