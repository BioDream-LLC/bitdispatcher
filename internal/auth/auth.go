// Package auth keeps bitdispatcher's users and sessions. Passwords are stored as PBKDF2-SHA256 hashes in
// users.json (mode 0600) beside the ledger; the ledger records who was added or changed, never a hash.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Roles, least to most.
const (
	Viewer     = "viewer"
	Field      = "field" // a crew on a mobile data terminal: signs in as its unit
	Dispatcher = "dispatcher"
	Supervisor = "supervisor"
	Admin      = "admin"
)

var rank = map[string]int{Viewer: 1, Field: 1, Dispatcher: 2, Supervisor: 3, Admin: 4}

// AtLeast reports whether role has at least the rights of want.
func AtLeast(role, want string) bool { return rank[role] >= rank[want] && rank[role] > 0 }

// ValidRole reports a known role.
func ValidRole(r string) bool { return rank[r] > 0 }

const iterations = 600_000

// User is a person who can sign in.
type User struct {
	Username string    `json:"username"`
	Name     string    `json:"name"`
	Role     string    `json:"role"`
	Disabled bool      `json:"disabled,omitempty"`
	Demo     bool      `json:"demo,omitempty"` // the public demonstration account: signed in from the sign-in page, no password
	Salt     string    `json:"salt"`
	Hash     string    `json:"hash"`
	Iter     int       `json:"iter"`
	Created  time.Time `json:"created"`
	LastIn   time.Time `json:"lastIn,omitzero"`
}

// Public is a user without secrets.
type Public struct {
	Username string    `json:"username"`
	Name     string    `json:"name"`
	Role     string    `json:"role"`
	Disabled bool      `json:"disabled,omitempty"`
	Demo     bool      `json:"demo,omitempty"`
	Created  time.Time `json:"created"`
	LastIn   time.Time `json:"lastIn,omitzero"`
}

// Session is one signed-in browser.
type Session struct {
	ID       string    `json:"-"`
	Hash     string    `json:"hash"` // what the ledger records
	Username string    `json:"username"`
	Name     string    `json:"name"`
	Role     string    `json:"role"`
	Console  string    `json:"console"`
	From     string    `json:"from"`
	Created  time.Time `json:"created"`
	Last     time.Time `json:"last"`
}

// Store is the user file and the live sessions.
type Store struct {
	mu       sync.Mutex
	path     string
	users    map[string]*User
	sessions map[string]*Session
	fails    map[string][]time.Time
	now      func() time.Time
	Idle     time.Duration
	MaxAge   time.Duration
}

var reUser = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,31}$`)

// Open reads users.json in dir, creating an empty store if there is none.
func Open(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "users.json"), users: map[string]*User{}, sessions: map[string]*Session{},
		fails: map[string][]time.Time{}, now: time.Now, Idle: 12 * time.Hour, MaxAge: 14 * time.Hour}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var list []*User
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	for _, u := range list {
		s.users[u.Username] = u
	}
	return s, nil
}

func (s *Store) save() error {
	list := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		list = append(list, u)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Username < list[j].Username })
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func hash(password, salt string, iter int) string {
	k, err := pbkdf2.Key(sha256.New, password, []byte(salt), iter, 32)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(k)
}

// RandomToken is a random URL-safe string of n bytes of entropy.
func RandomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// GeneratePassword is a password a person can type: five groups of four.
func GeneratePassword() string {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	b := make([]byte, 20)
	rand.Read(b)
	var out strings.Builder
	for i, c := range b {
		if i > 0 && i%4 == 0 {
			out.WriteByte('-')
		}
		out.WriteByte(alphabet[int(c)%len(alphabet)])
	}
	return out.String()
}

// CheckPassword says what is wrong with a new password, or "".
func CheckPassword(p string) string {
	if len([]rune(p)) < 12 {
		return "use at least 12 characters"
	}
	seen := map[rune]bool{}
	for _, r := range strings.ToLower(p) {
		seen[r] = true
	}
	if len(seen) < 6 {
		return "use more different characters"
	}
	return ""
}

// Empty reports whether there are no users yet.
func (s *Store) Empty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.users) == 0
}

// Save adds or changes a user. An empty password keeps the current one (a new user needs one).
func (s *Store) Save(username, name, role, password string, disabled bool) error {
	username = strings.ToLower(strings.TrimSpace(username))
	if !reUser.MatchString(username) {
		return errors.New("a username is 2 to 32 lower-case letters, digits, dots, dashes or underscores")
	}
	if !ValidRole(role) {
		return errors.New("choose a role")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[username]
	if !ok {
		if password == "" {
			return errors.New("a new user needs a password")
		}
		u = &User{Username: username, Created: s.now().UTC()}
	}
	if u.Demo && !demoRole(role) {
		return errors.New("the public demonstration account cannot be an administrator or a field crew: stop it being the demonstration account first")
	}
	if !disabled && role != Admin && u.Role == Admin && s.countAdmins() == 1 {
		return errors.New("keep at least one administrator")
	}
	if disabled && u.Role == Admin && s.countAdmins() == 1 {
		return errors.New("keep at least one administrator")
	}
	if password != "" {
		if p := CheckPassword(password); p != "" {
			return errors.New(p)
		}
		u.Salt = RandomToken(16)
		u.Iter = iterations
		u.Hash = hash(password, u.Salt, u.Iter)
	}
	u.Name, u.Role, u.Disabled = strings.TrimSpace(name), role, disabled
	s.users[username] = u
	if disabled {
		s.endSessionsOf(username)
	}
	for _, se := range s.sessions {
		if se.Username == username {
			se.Role, se.Name = role, u.Name
		}
	}
	return s.save()
}

func (s *Store) countAdmins() int {
	n := 0
	for _, u := range s.users {
		if u.Role == Admin && !u.Disabled {
			n++
		}
	}
	return n
}

// Remove deletes a user and ends their sessions.
func (s *Store) Remove(username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[username]
	if !ok {
		return errors.New("no such user")
	}
	if u.Role == Admin && s.countAdmins() == 1 {
		return errors.New("keep at least one administrator")
	}
	delete(s.users, username)
	s.endSessionsOf(username)
	return s.save()
}

func (s *Store) endSessionsOf(username string) {
	for id, se := range s.sessions {
		if se.Username == username {
			delete(s.sessions, id)
		}
	}
}

// List returns every user without secrets.
func (s *Store) List() []Public {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Public{}
	for _, u := range s.users {
		out = append(out, Public{Username: u.Username, Name: u.Name, Role: u.Role, Disabled: u.Disabled, Demo: u.Demo, Created: u.Created, LastIn: u.LastIn})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out
}

// ErrLocked is returned while an account is locked after repeated failures.
var ErrLocked = errors.New("too many failed sign-ins: try again in 15 minutes")

// ErrBadLogin is returned for a wrong username or password, without saying which.
var ErrBadLogin = errors.New("wrong username or password")

// Login checks a password and opens a session.
func (s *Store) Login(username, password, console, from string) (*Session, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	s.mu.Lock()
	defer s.mu.Unlock()
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
	// Hash even for an unknown user, so timing does not say which usernames exist.
	salt, iter, want := "x", iterations, strings.Repeat("0", 64)
	if ok {
		salt, iter, want = u.Salt, u.Iter, u.Hash
	}
	got := hash(password, salt, iter)
	if !ok || u.Disabled || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		s.fails[username] = append(s.fails[username], now)
		return nil, ErrBadLogin
	}
	delete(s.fails, username)
	return s.open(u, console, from), nil
}

// demoRole is whether a role may be the public demonstration account's: never an administrator (anyone could
// change the configuration) nor a field crew (it signs in as a unit).
func demoRole(role string) bool { return role != Admin && role != Field && ValidRole(role) }

// SetDemo makes a user the public demonstration account, or stops it being one. There is one at most: making
// one user it stops any other.
func (s *Store) SetDemo(username string, on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[strings.ToLower(username)]
	if !ok {
		return errors.New("no such user")
	}
	if on && !demoRole(u.Role) {
		return errors.New("an administrator or a field crew cannot be the public demonstration account")
	}
	for _, o := range s.users {
		if on && o != u {
			o.Demo = false
		}
	}
	u.Demo = on
	return s.save()
}

// Demo is the public demonstration account, when there is one that may sign in.
func (s *Store) Demo() (Public, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.Demo && !u.Disabled && demoRole(u.Role) {
			return Public{Username: u.Username, Name: u.Name, Role: u.Role, Demo: true}, true
		}
	}
	return Public{}, false
}

// LoginDemo signs in as the public demonstration account, without its password. Wrong passwords typed for it
// elsewhere never lock this out.
func (s *Store) LoginDemo(console, from string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.Demo && !u.Disabled && demoRole(u.Role) {
			return s.open(u, console, from), nil
		}
	}
	return nil, errors.New("there is no demonstration account here")
}

// open starts a session for a user who has proved who they are. The caller holds s.mu.
func (s *Store) open(u *User, console, from string) *Session {
	now := s.now()
	u.LastIn = now.UTC()
	s.save()
	id := RandomToken(32)
	sum := sha256.Sum256([]byte(id))
	se := &Session{ID: id, Hash: hex.EncodeToString(sum[:8]), Username: u.Username, Name: u.Name, Role: u.Role,
		Console: strings.TrimSpace(console), From: from, Created: now, Last: now}
	s.sessions[id] = se
	return se
}

// Session returns the live session for a token, extending it.
func (s *Store) Session(id string) (*Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	se, ok := s.sessions[id]
	if !ok {
		return nil, false
	}
	now := s.now()
	if now.Sub(se.Last) > s.Idle || now.Sub(se.Created) > s.MaxAge {
		delete(s.sessions, id)
		return nil, false
	}
	se.Last = now
	cp := *se
	return &cp, true
}

// SetConsole changes the position a session is at (a field crew's unit, normalised).
func (s *Store) SetConsole(id, console string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if se, ok := s.sessions[id]; ok {
		se.Console = console
	}
}

// Logout ends a session.
func (s *Store) Logout(id string) (*Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	se, ok := s.sessions[id]
	delete(s.sessions, id)
	return se, ok
}

// Active lists signed-in sessions, for the supervisor's position view.
func (s *Store) Active() []Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Session{}
	now := s.now()
	for _, se := range s.sessions {
		if now.Sub(se.Last) <= s.Idle {
			out = append(out, *se)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Console < out[j].Console })
	return out
}
