package auth

// Duties, multi-factor sign-in and console locking.
//
// A person holds one or more qualifications (duties): call-taker, police, fire or EMS dispatcher, an
// all-discipline dispatcher, supervisor, administrator, records staff, a field crew, or a viewer. At sign-in
// they choose the one they are working; the session's rights follow it, so an administrator covering a
// call-taking position works as a call-taker (a supervisor keeps supervisory rights wherever they sit).
//
// Multi-factor sign-in is a time-based one-time code (RFC 6238, the kind any authenticator app makes),
// required by the agency's policy for everyone or for supervisors and administrators. A console locks on
// demand or after the policy's minutes without input, and unlocks for the same person with their password
// (and code), keeping everything open on the screen.

import (
	"crypto/fips140"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Duties a person can hold.
const (
	DutyCallTaker  = "calltaker"
	DutyPolice     = "police"
	DutyFire       = "fire"
	DutyEMS        = "ems"
	DutyDispatcher = "dispatcher"
	DutySupervisor = "supervisor"
	DutyAdmin      = "admin"
	DutyRecords    = "records"
	DutyField      = "field"
	DutyViewer     = "viewer"
)

// AllDuties in the order they are offered.
var AllDuties = []string{DutyCallTaker, DutyPolice, DutyFire, DutyEMS, DutyDispatcher, DutySupervisor, DutyAdmin, DutyRecords, DutyField, DutyViewer}

// DutyName is a duty in words.
func DutyName(d string) string {
	return map[string]string{DutyCallTaker: "Call-taker", DutyPolice: "Police dispatcher", DutyFire: "Fire dispatcher", DutyEMS: "EMS dispatcher",
		DutyDispatcher: "Dispatcher (all disciplines)", DutySupervisor: "Supervisor", DutyAdmin: "Administrator", DutyRecords: "Records",
		DutyField: "Field crew (mobile terminal)", DutyViewer: "Viewer"}[d]
}

// dutyRank is the role a duty needs a person to have at least.
var dutyRank = map[string]string{DutyCallTaker: Dispatcher, DutyPolice: Dispatcher, DutyFire: Dispatcher, DutyEMS: Dispatcher,
	DutyDispatcher: Dispatcher, DutySupervisor: Supervisor, DutyAdmin: Admin, DutyRecords: Viewer, DutyField: Field, DutyViewer: Viewer}

// DutiesFor are the duties a role holds when an administrator has not chosen them.
func DutiesFor(role string) []string {
	switch role {
	case Admin:
		return []string{DutyAdmin, DutySupervisor, DutyDispatcher, DutyCallTaker, DutyPolice, DutyFire, DutyEMS, DutyRecords}
	case Supervisor:
		return []string{DutySupervisor, DutyDispatcher, DutyCallTaker, DutyPolice, DutyFire, DutyEMS, DutyRecords}
	case Dispatcher:
		return []string{DutyDispatcher, DutyCallTaker, DutyPolice, DutyFire, DutyEMS}
	case Field:
		return []string{DutyField}
	case Viewer:
		return []string{DutyViewer}
	}
	return nil
}

// RoleForDuties is the least role that holds every duty: what a person with these qualifications is.
func RoleForDuties(duties []string) string {
	best := ""
	for _, d := range duties {
		r := dutyRank[d]
		if r == Field && best == "" {
			best = Field
			continue
		}
		if rank[r] > rank[best] || best == Field && r != Field {
			best = r
		}
	}
	return best
}

// SessionRole is the rights a session working a duty has. A supervisor (or administrator) covering an
// operational position keeps supervisory rights; an administrator works configuration only as one.
func SessionRole(duty, userRole string) string {
	switch duty {
	case DutyAdmin:
		return Admin
	case DutySupervisor:
		return Supervisor
	case DutyCallTaker, DutyPolice, DutyFire, DutyEMS, DutyDispatcher:
		if AtLeast(userRole, Supervisor) {
			return Supervisor
		}
		return Dispatcher
	case DutyField:
		return Field
	}
	return Viewer
}

func (u *User) duties() []string {
	if len(u.Duties) > 0 {
		return u.Duties
	}
	return DutiesFor(u.Role)
}

// defaultDuty is the duty an old-style sign-in (no choice) works: the highest the person holds.
func (u *User) defaultDuty() string {
	ds := u.duties()
	for _, want := range []string{DutyAdmin, DutySupervisor, DutyDispatcher, DutyCallTaker, DutyPolice, DutyFire, DutyEMS, DutyRecords, DutyField, DutyViewer} {
		if slices.Contains(ds, want) {
			return want
		}
	}
	return DutyViewer
}

// SetDuties sets the duties a person holds; their role becomes the least that holds them all.
func (s *Store) SetDuties(username string, duties []string) error {
	clean := []string{}
	for _, d := range duties {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" || slices.Contains(clean, d) {
			continue
		}
		if _, ok := dutyRank[d]; !ok {
			return fmt.Errorf("no duty %q", d)
		}
		clean = append(clean, d)
	}
	if len(clean) == 0 {
		return errors.New("choose at least one duty")
	}
	if slices.Contains(clean, DutyField) && len(clean) > 1 {
		return errors.New("a field crew signs in as its unit: give field crews no other duty")
	}
	role := RoleForDuties(clean)
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[strings.ToLower(username)]
	if !ok {
		return errors.New("no such user")
	}
	if u.Role == Admin && role != Admin && s.countAdmins() == 1 {
		return errors.New("keep at least one administrator")
	}
	if u.Demo && !demoRole(role) {
		return errors.New("the public demonstration account cannot be an administrator or a field crew")
	}
	u.Duties, u.Role = clean, role
	for _, se := range s.sessions {
		if se.Username == u.Username && !slices.Contains(clean, se.Duty) {
			se.Duty = u.defaultDuty()
			se.Role = SessionRole(se.Duty, role)
		}
	}
	return s.save()
}

// ---- sign-in with a duty and a code ----

// Need is what a sign-in still needs: a duty chosen, a one-time code, or multi-factor set up.
type Need struct {
	Duties []string `json:"duties,omitempty"` // choose one of these
	Last   string   `json:"last,omitempty"`   // the one chosen last time
	Code   bool     `json:"code,omitempty"`   // type the code from the authenticator app
	Enrol  *Enrol   `json:"enrol,omitempty"`  // set up an authenticator app first, then type its code
}

// Enrol is what an authenticator app needs: the secret, as text and as an otpauth link.
type Enrol struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
}

// Request is a sign-in.
type Request struct {
	Username, Password, Code, Duty, Console, From string
}

// mfaRequired is whether the policy asks this person for a one-time code.
func (s *Store) mfaRequired(u *User) bool {
	switch s.MFA {
	case "everyone":
		return true
	case "admins":
		return AtLeast(u.Role, Supervisor)
	}
	return u.TOTP != "" // someone who set it up uses it even when the policy does not ask
}

// SignIn checks a password, then the one-time code and the duty: it opens a session, or says what it still
// needs. A right password with a missing code or duty is not a failure; a wrong code is.
func (s *Store) SignIn(r Request) (*Session, *Need, error) {
	username := strings.ToLower(strings.TrimSpace(r.Username))
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.checkPassword(username, r.Password)
	if err != nil {
		return nil, nil, err
	}
	if s.mfaRequired(u) {
		if u.TOTP == "" {
			// First sign-in under the policy: set up the app now.
			if u.PendingTOTP == "" {
				u.PendingTOTP = newSecret()
				s.save()
			}
			if strings.TrimSpace(r.Code) == "" {
				return nil, &Need{Enrol: &Enrol{Secret: plainSecret(u.PendingTOTP), URI: otpURI(s.Issuer, u.Username, u.PendingTOTP)}}, nil
			}
			step, ok := checkTOTP(u.PendingTOTP, r.Code, s.now(), 0)
			if !ok {
				s.fail(username)
				return nil, &Need{Enrol: &Enrol{Secret: plainSecret(u.PendingTOTP), URI: otpURI(s.Issuer, u.Username, u.PendingTOTP)}}, ErrBadCode
			}
			u.TOTP, u.PendingTOTP, u.TOTPStep = u.PendingTOTP, "", step
			s.save()
		} else {
			if strings.TrimSpace(r.Code) == "" {
				return nil, &Need{Code: true}, nil
			}
			if err := fipsRefuses(u.TOTP); err != nil {
				return nil, nil, err
			}
			step, ok := checkTOTP(u.TOTP, r.Code, s.now(), u.TOTPStep)
			if !ok {
				s.fail(username)
				return nil, &Need{Code: true}, ErrBadCode
			}
			u.TOTPStep = step
		}
	}
	duties := u.duties()
	duty := strings.ToLower(strings.TrimSpace(r.Duty))
	if duty == "" {
		if len(duties) > 1 {
			return nil, &Need{Duties: duties, Last: u.LastDuty}, nil
		}
		duty = duties[0]
	}
	if !slices.Contains(duties, duty) {
		return nil, &Need{Duties: duties, Last: u.LastDuty}, fmt.Errorf("%s is not one of your duties: choose one of yours", DutyName(duty))
	}
	delete(s.fails, username)
	u.LastDuty = duty
	se := s.open(u, r.Console, r.From)
	se.Duty, se.Role = duty, SessionRole(duty, u.Role)
	cp := *se
	return &cp, nil, nil
}

// ErrBadCode is a wrong or reused one-time code.
var ErrBadCode = errors.New("that code is not right (or was already used): type the current one")

// checkPassword checks a user's password and the lockout. The caller holds s.mu.
func (s *Store) checkPassword(username, password string) (*User, error) {
	now := s.now()
	recent := s.fails[username][:0:0]
	for _, t := range s.fails[username] {
		if now.Sub(t) < 15*time.Minute {
			recent = append(recent, t)
		}
	}
	s.fails[username] = recent
	if len(recent) >= 5 {
		return nil, ErrLocked
	}
	u, ok := s.users[username]
	salt, iter, want := "x", iterations, strings.Repeat("0", 64)
	if ok {
		salt, iter, want = u.Salt, u.Iter, u.Hash
	}
	got := hash(password, salt, iter)
	if !ok || u.Disabled || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		s.fail(username)
		return nil, ErrBadLogin
	}
	return u, nil
}

func (s *Store) fail(username string) { s.fails[username] = append(s.fails[username], s.now()) }

// ResetMFA removes a person's authenticator, so they set it up again at their next sign-in.
func (s *Store) ResetMFA(username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[strings.ToLower(username)]
	if !ok {
		return errors.New("no such user")
	}
	u.TOTP, u.PendingTOTP, u.TOTPStep = "", "", 0
	return s.save()
}

// ---- locking ----

// Lock locks a session's console: nothing is shown or changed until the same person unlocks it.
func (s *Store) Lock(id string) (*Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	se, ok := s.sessions[id]
	if !ok {
		return nil, false
	}
	if !se.Locked {
		se.Locked, se.LockedAt = true, s.now()
	}
	cp := *se
	return &cp, true
}

// Unlock unlocks a session with its person's password (and code, when they use one).
func (s *Store) Unlock(id, password, code string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	se, ok := s.sessions[id]
	if !ok {
		return nil, errors.New("signed out")
	}
	u, err := s.checkPassword(se.Username, password)
	if err != nil {
		return nil, err
	}
	if u.TOTP != "" {
		if err := fipsRefuses(u.TOTP); err != nil {
			return nil, err
		}
		step, ok := checkTOTP(u.TOTP, code, s.now(), u.TOTPStep)
		if !ok {
			s.fail(se.Username)
			return nil, ErrBadCode
		}
		u.TOTPStep = step
	}
	delete(s.fails, se.Username)
	se.Locked, se.LockedAt, se.Active = false, time.Time{}, s.now()
	cp := *se
	return &cp, nil
}

// Touch records input at a console (a key, a click): the inactivity lock counts from it.
func (s *Store) Touch(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if se, ok := s.sessions[id]; ok && !se.Locked {
		se.Active = s.now()
	}
}

// ---- one-time codes (RFC 6238) ----

// A secret is kept as its base32 text, which means HMAC-SHA-1 (what every authenticator app does), or as
// "sha256:" and the text for HMAC-SHA-256. A server in FIPS 140-only mode (GODEBUG=fips140=only) makes
// SHA-256 secrets, because the Go Cryptographic Module refuses HMAC-SHA-1 there; not every authenticator
// app supports them (docs/COMPLIANCE.md).
const sha256Prefix = "sha256:"

// ErrTOTPNotFIPS is a SHA-1 authenticator on a server in FIPS 140-only mode.
var ErrTOTPNotFIPS = errors.New("this authenticator uses SHA-1, which this server's FIPS 140-only mode does not allow: an administrator resets it, and the next sign-in sets up a new one")

func newSecret() string {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	text := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	if fips140.Enforced() {
		return sha256Prefix + text
	}
	return text
}

// plainSecret is the secret as an authenticator app takes it.
func plainSecret(stored string) string { return strings.TrimPrefix(stored, sha256Prefix) }

func fipsRefuses(stored string) error {
	if fips140.Enforced() && !strings.HasPrefix(stored, sha256Prefix) {
		return ErrTOTPNotFIPS
	}
	return nil
}

func otpURI(issuer, user, secret string) string {
	if issuer == "" {
		issuer = "bitdispatcher"
	}
	label := url.PathEscape(issuer + ":" + user)
	alg := "SHA1"
	if strings.HasPrefix(secret, sha256Prefix) {
		alg = "SHA256"
	}
	q := url.Values{"secret": {plainSecret(secret)}, "issuer": {issuer}, "algorithm": {alg}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// TOTP is the code for a secret at a time step (30 seconds). A secret starting "sha256:" uses HMAC-SHA-256.
func TOTP(secret string, step int64) string {
	h := sha1.New
	if strings.HasPrefix(secret, sha256Prefix) {
		h, secret = sha256.New, plainSecret(secret)
	} else if fips140.Enforced() {
		return "" // HMAC-SHA-1 is refused (fipsRefuses says why)
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimRight(secret, "=")))
	if err != nil {
		return ""
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	m := hmac.New(h, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", v%1_000_000)
}

// checkTOTP accepts the code for now or one step either side (clocks drift), never a step already used.
func checkTOTP(secret, code string, now time.Time, used int64) (int64, bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 {
		return 0, false
	}
	step := now.Unix() / 30
	for _, s := range []int64{step, step - 1, step + 1} {
		if s > used && subtle.ConstantTimeCompare([]byte(TOTP(secret, s)), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

// SetPolicy sets the agency's sign-in policy: who needs a one-time code, the inactivity lock, and the name
// authenticator apps show.
func (s *Store) SetPolicy(mfa string, lockAfter time.Duration, issuer string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.MFA, s.LockAfter, s.Issuer = mfa, lockAfter, issuer
}

// UserRole is a person's role (their highest rights), whatever duty they are working now.
func (s *Store) UserRole(username string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u, ok := s.users[strings.ToLower(username)]; ok {
		return u.Role
	}
	return ""
}
