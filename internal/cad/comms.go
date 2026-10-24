package cad

// Operational communications: messages, BOLOs, officer-safety notices and administrative notices, as an
// agency-controlled mail system inside CAD. Each has a sender (person, duty, agency, console), addressees
// (To and CC: agencies, positions, roles, named people, disciplines, units, the units on an incident), a
// subject, a type, one of four priorities, a body, an incident and related incidents, attachments, and
// issued, effective and expiry dates. Opened and acknowledged are recorded by person and duty, never just
// by the screen that showed it.
//
// A sent communication is never rewritten: an update or a cancellation is a new version of the same
// thread, the earlier versions kept, so what each person received stays on record (and on the incident's
// timeline). Expired and cancelled ones leave the active views but stay in history and search, labelled.
// A BOLO also feeds the plate and name checks (entry.go) for as long as it is out.

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
	EvCommSent      = "comm.sent"  // a communication (or a new version of one) was sent
	EvCommDraft     = "comm.draft" // a draft was saved
	EvCommDraftGone = "comm.draft.removed"
	EvCommReceipt   = "comm.receipt"   // delivered to, opened or acknowledged by a person
	EvCommFile      = "comm.file"      // an attachment was stored
	EvCommFileOpen  = "comm.file.open" // an attachment was opened or downloaded
	EvCommFileGone  = "comm.file.removed"
	EvCommEscalated = "comm.escalated" // an urgent one not acknowledged in time went to supervisors
)

// Communication kinds the agency can extend (Settings.CommKinds).
const (
	CommMessage = "message"
	CommBOLO    = "bolo"
	CommSafety  = "officer-safety"
	CommAdmin   = "admin"
)

// Priorities, highest first.
var Priorities = []string{"urgent", "high", "medium", "low"}

// Address forms besides unit IDs and ALL, ALL-FIRE, ALL-EMS, ALL-POLICE and DISPATCH.
const (
	ToUser     = "@user:"     // a named person
	ToRole     = "@role:"     // everyone signed in with a role: supervisor, dispatcher
	ToDuty     = "@duty:"     // everyone working a duty: calltaker, police, fire, ems
	ToAgency   = "@agency:"   // every CAD position covering the agency and its units' terminals
	ToPosition = "@position:" // one console
	ToIncident = "@incident:" // the units on an incident
)

// CommKind is a type of communication and its rules.
type CommKind struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Priority string `json:"priority,omitempty"` // the default priority
	// Send are the roles that may send it (empty: dispatchers and above; a crew may send messages).
	Send []string `json:"send,omitempty"`
	// Hours it lasts by default (0: no default expiry); NoExpiry allows "no expiration".
	Hours    int  `json:"hours,omitempty"`
	NoExpiry bool `json:"noExpiry,omitempty"`
	// Waits: people signed off when it was sent still find it in their inbox at their next sign-on.
	Waits bool `json:"waits,omitempty"`
}

// PriorityRule is how a priority is presented: its tone, how often it repeats until acknowledged, whether
// acknowledgement is required, and after how long an unacknowledged one goes to the supervisors.
type PriorityRule struct {
	Level    string `json:"level"`
	Tone     string `json:"tone"`               // alarm, alert, chime, tick or none
	Repeat   int    `json:"repeat,omitempty"`   // seconds between repeats until acknowledged (0: once)
	Ack      bool   `json:"ack,omitempty"`      // acknowledgement required
	Escalate int    `json:"escalate,omitempty"` // minutes unacknowledged before supervisors are told (0: never)
	// Silenceable lets a person turn the tone off at their console (never for a level that must sound).
	Silenceable bool `json:"silenceable,omitempty"`
}

// DefaultCommKinds are the kinds a centre starts with.
func DefaultCommKinds() []CommKind {
	return []CommKind{
		{Code: CommMessage, Name: "Message", Priority: "medium", Send: []string{"field", "dispatcher", "supervisor", "admin"}, Hours: 24, NoExpiry: true},
		{Code: CommBOLO, Name: "BOLO", Priority: "high", Hours: 72, NoExpiry: true, Waits: true},
		{Code: CommSafety, Name: "Officer safety", Priority: "urgent", Hours: 168, NoExpiry: true, Waits: true},
		{Code: CommAdmin, Name: "Administrative", Priority: "low", Send: []string{"supervisor", "admin"}, Hours: 336, NoExpiry: true, Waits: true},
	}
}

// DefaultPriorityRules: urgent interrupts and repeats until acknowledged and goes to supervisors after
// two minutes; high sounds once and asks for acknowledgement; medium chimes once; low ticks.
func DefaultPriorityRules() []PriorityRule {
	return []PriorityRule{
		{Level: "urgent", Tone: "alarm", Repeat: 10, Ack: true, Escalate: 2},
		{Level: "high", Tone: "alert", Repeat: 60, Ack: true, Escalate: 10},
		{Level: "medium", Tone: "chime", Silenceable: true},
		{Level: "low", Tone: "tick", Silenceable: true},
	}
}

// CommKinds are the agency's kinds (the defaults when none are set).
func (s Settings) CommKindList() []CommKind {
	if len(s.CommKinds) == 0 {
		return DefaultCommKinds()
	}
	return s.CommKinds
}

// PriorityRuleList are the agency's priority rules (the defaults for any level not set).
func (s Settings) PriorityRuleList() []PriorityRule {
	out := DefaultPriorityRules()
	for _, r := range s.PriorityRules {
		for i := range out {
			if out[i].Level == r.Level {
				out[i] = r
			}
		}
	}
	return out
}

func (s Settings) commKind(code string) (CommKind, bool) {
	for _, k := range s.CommKindList() {
		if k.Code == code {
			return k, true
		}
	}
	return CommKind{}, false
}

func (s Settings) priorityRule(level string) PriorityRule {
	for _, r := range s.PriorityRuleList() {
		if r.Level == level {
			return r
		}
	}
	return PriorityRule{Level: level}
}

// Receipt is one person's delivery, opening or acknowledgement.
type Receipt struct {
	At      time.Time `json:"at"`
	Duty    string    `json:"duty,omitempty"`
	Console string    `json:"console,omitempty"`
	Unit    string    `json:"unit,omitempty"`
}

// Comm is one version of a communication.
type Comm struct {
	ID      string `json:"id"`     // this version
	Thread  string `json:"thread"` // the first version's ID: the communication
	Version int    `json:"version"`
	Of      string `json:"of,omitempty"` // the version it replaces
	Kind    string `json:"kind"`
	// Priority: urgent, high, medium or low.
	Priority string   `json:"priority"`
	Subject  string   `json:"subject"`
	Body     string   `json:"body"`
	To       []string `json:"to"`
	CC       []string `json:"cc,omitempty"`
	Incident string   `json:"incident,omitempty"`
	Related  []string `json:"related,omitempty"`
	Files    []string `json:"files,omitempty"` // attachment IDs (CommFile)
	// What a BOLO looks for, for the plate and name checks and the search.
	Plate       string `json:"plate,omitempty"`
	PlateState  string `json:"plateState,omitempty"`
	Vehicle     string `json:"vehicle,omitempty"`
	Person      string `json:"person,omitempty"`
	Description string `json:"description,omitempty"`
	Agency      string `json:"agency,omitempty"` // the owning agency
	// From: the person, their duty, agency and console, or a unit's terminal.
	From        string    `json:"from"`
	FromName    string    `json:"fromName,omitempty"`
	FromDuty    string    `json:"fromDuty,omitempty"`
	FromAgency  string    `json:"fromAgency,omitempty"`
	FromConsole string    `json:"fromConsole,omitempty"`
	FromUnit    string    `json:"fromUnit,omitempty"`
	Issued      time.Time `json:"issued"`
	Effective   time.Time `json:"effective,omitzero"`
	Expires     time.Time `json:"expires,omitzero"`
	NoExpiry    bool      `json:"noExpiry,omitempty"`
	Cancelled   bool      `json:"cancelled,omitempty"` // this version cancels the communication
	Why         string    `json:"why,omitempty"`       // why it was updated or cancelled
	// SupersededBy is the next version, once there is one.
	SupersededBy string             `json:"supersededBy,omitempty"`
	Delivered    map[string]Receipt `json:"delivered,omitempty"` // by person (user name)
	Opened       map[string]Receipt `json:"opened,omitempty"`
	Acked        map[string]Receipt `json:"acked,omitempty"`
	Escalated    time.Time          `json:"escalated,omitzero"`
}

// CommFile is an attachment, kept beside the ledger with its hash.
type CommFile struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Type    string    `json:"type"`
	Size    int64     `json:"size"`
	SHA256  string    `json:"sha256"`
	By      string    `json:"by"`
	At      time.Time `json:"at"`
	Removed bool      `json:"removed,omitempty"`
	Why     string    `json:"why,omitempty"`
}

// CommDraft is a communication being written, kept with its author.
type CommDraft struct {
	ID      string    `json:"id"`
	User    string    `json:"user"`
	Comm    Comm      `json:"comm"`
	Updated time.Time `json:"updated"`
}

// State of a version now: draft, active, updated (a later version replaced it), expired or cancelled.
func (c *Comm) State(now time.Time) string {
	switch {
	case c.Cancelled:
		return "cancelled"
	case c.SupersededBy != "":
		return "updated"
	case !c.NoExpiry && !c.Expires.IsZero() && !now.Before(c.Expires):
		return "expired"
	case !c.Effective.IsZero() && now.Before(c.Effective):
		return "scheduled"
	}
	return "active"
}

// Active reports whether it is the current version, in effect and not expired or cancelled.
func (c *Comm) Active(now time.Time) bool { return c.State(now) == "active" }

// Reader is a person reading communications: who, with what rights, where, for which agencies.
type Reader struct {
	User     string
	Role     string
	Duty     string
	Console  string
	Unit     string   // a crew's terminal
	Agencies []string // covered (a CAD position), or the unit's agency
	Since    time.Time
}

// addressed reports whether a list of addresses reaches the reader.
func (s *State) addressed(list []string, r Reader) bool {
	disc := ""
	if u := s.Units[r.Unit]; u != nil {
		disc = s.Discipline(u)
	}
	for _, t := range list {
		switch {
		case r.Unit != "" && (t == r.Unit || t == ToAll || t == ToFire && disc == "fire" || t == ToEMS && disc == "ems" || t == ToPolice && disc == "police"):
			return true
		case strings.HasPrefix(t, ToIncident) && r.Unit != "":
			if in := s.Incidents[strings.TrimPrefix(t, ToIncident)]; in != nil && slices.ContainsFunc(in.Units, func(a Assignment) bool { return a.Unit == r.Unit }) {
				return true
			}
		case r.Unit == "" && t == ToDispatch:
			return true
		case strings.EqualFold(t, ToUser+r.User):
			return true
		case t == ToRole+r.Role && r.Unit == "":
			return true
		case t == ToDuty+r.Duty && r.Unit == "":
			return true
		case strings.EqualFold(t, ToPosition+r.Console) && r.Unit == "":
			return true
		case strings.HasPrefix(t, ToAgency) && slices.Contains(r.Agencies, strings.TrimPrefix(t, ToAgency)):
			return true
		}
	}
	return false
}

// For reports whether a communication is for the reader (addressed in To or CC, or theirs).
func (s *State) CommFor(c *Comm, r Reader) bool {
	return s.addressed(c.To, r) || s.addressed(c.CC, r)
}

// Inbox are the communications for a reader that are current: the latest version of each thread, active,
// and (for kinds that do not wait) sent while they were signed in or to them by name.
func (s *State) Inbox(r Reader, now time.Time) []*Comm {
	out := []*Comm{}
	for _, c := range s.Comms {
		if c.SupersededBy != "" || !s.CommFor(c, r) || c.State(now) == "expired" || c.State(now) == "scheduled" {
			continue
		}
		if c.Cancelled && now.Sub(c.Issued) > 24*time.Hour {
			continue // a cancellation shows for a day
		}
		k, _ := s.Settings.commKind(c.Kind)
		if !k.Waits && !r.Since.IsZero() && c.Issued.Before(r.Since) && !s.addressed(append(slices.Clone(c.To), c.CC...), Reader{User: r.User}) {
			continue
		}
		out = append(out, c)
	}
	sortComms(out)
	return out
}

func sortComms(cs []*Comm) {
	rank := func(p string) int { return slices.Index(Priorities, p) }
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].Cancelled != cs[j].Cancelled {
			return !cs[i].Cancelled
		}
		if rank(cs[i].Priority) != rank(cs[j].Priority) {
			return rank(cs[i].Priority) < rank(cs[j].Priority)
		}
		return cs[i].Issued.After(cs[j].Issued)
	})
}

// CommThread are a communication's versions, oldest first.
func (s *State) CommThread(thread string) []*Comm {
	var out []*Comm
	for _, c := range s.Comms {
		if c.Thread == thread {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out
}

// CommLatest is the current version of a thread.
func (s *State) CommLatest(thread string) *Comm {
	vs := s.CommThread(thread)
	if len(vs) == 0 {
		return nil
	}
	return vs[len(vs)-1]
}

// MaySeeComm reports whether a reader may see a communication: addressed to them, theirs, or a supervisor
// or records staff of an agency it belongs to (or of the centre).
func (s *State) MaySeeComm(c *Comm, r Reader) bool {
	if strings.EqualFold(c.From, r.User) || s.CommFor(c, r) {
		return true
	}
	for _, v := range s.CommThread(c.Thread) {
		if s.CommFor(v, r) || strings.EqualFold(v.From, r.User) {
			return true
		}
	}
	if r.Role == "supervisor" || r.Role == "admin" || r.Duty == "records" {
		return c.Agency == "" || s.AllAgencies(r.User) || s.MayWork(r.User, c.Agency)
	}
	return false
}

var rePlate = regexp.MustCompile(`^[A-Z0-9 -]{1,10}$`)

// SendComm sends a communication, or a new version of one (c.Of set: an update; with Cancelled, a
// cancellation). It returns the version's ID.
func (e *Engine) SendComm(a Actor, c Comm) (string, error) {
	c.Subject, c.Body = strings.TrimSpace(c.Subject), strings.TrimSpace(c.Body)
	c.Kind = strings.ToLower(strings.TrimSpace(c.Kind))
	if c.Kind == "" {
		c.Kind = CommMessage
	}
	c.Plate = strings.ToUpper(strings.TrimSpace(c.Plate))
	if c.Plate != "" && !rePlate.MatchString(c.Plate) {
		return "", problem("plate", "a plate is up to 10 letters and digits")
	}
	if len(c.Body) > 8000 || len(c.Subject) > 200 {
		return "", problem("body", "a subject is up to 200 characters and the body up to 8,000")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	set := e.st.Settings
	now := e.now()
	var prev *Comm
	if c.Of != "" {
		prev = e.st.CommByID(c.Of)
		if prev == nil {
			return "", problem("of", "no communication %s", c.Of)
		}
		if prev.SupersededBy != "" {
			return "", problem("of", "%s was already updated by %s: update the latest version", c.Of, prev.SupersededBy)
		}
		if prev.Cancelled {
			return "", problem("of", "that communication is cancelled")
		}
		if strings.TrimSpace(c.Why) == "" {
			return "", problem("why", "say why it is updated or cancelled")
		}
		if !strings.EqualFold(prev.From, a.User) && a.Role != "supervisor" && a.Role != "admin" {
			return "", problem("of", "the sender or a supervisor updates or cancels it")
		}
		// A new version keeps what it does not change.
		if c.Kind == CommMessage && prev.Kind != CommMessage {
			c.Kind = prev.Kind
		}
		if c.Subject == "" {
			c.Subject = prev.Subject
		}
		if c.Body == "" {
			c.Body = prev.Body
		}
		if len(c.To) == 0 {
			c.To, c.CC = prev.To, prev.CC
		}
		if c.Incident == "" {
			c.Incident = prev.Incident
		}
		if c.Related == nil {
			c.Related = prev.Related
		}
		if c.Files == nil {
			c.Files = prev.Files
		}
		if c.Priority == "" {
			c.Priority = prev.Priority
		}
		if c.Agency == "" {
			c.Agency = prev.Agency
		}
		if c.Expires.IsZero() && !c.NoExpiry {
			c.Expires, c.NoExpiry = prev.Expires, prev.NoExpiry
		}
		if c.Plate == "" && c.Vehicle == "" && c.Person == "" && c.Description == "" {
			c.Plate, c.PlateState, c.Vehicle, c.Person, c.Description = prev.Plate, prev.PlateState, prev.Vehicle, prev.Person, prev.Description
		}
		c.Thread, c.Version = prev.Thread, prev.Version+1
	}
	k, ok := set.commKind(c.Kind)
	if !ok {
		return "", problem("kind", "no kind of communication %s", c.Kind)
	}
	send := k.Send
	if len(send) == 0 {
		send = []string{"dispatcher", "supervisor", "admin"}
	}
	if a.User != SystemActor.User && !slices.Contains(send, a.Role) {
		return "", problem("kind", "your role (%s) may not send a %s", a.Role, k.Name)
	}
	if c.Priority == "" {
		c.Priority = firstNonEmpty(k.Priority, "medium")
	}
	if !slices.Contains(Priorities, c.Priority) {
		return "", problem("priority", "priority is urgent, high, medium or low")
	}
	if c.Subject == "" {
		return "", problem("subject", "give it a subject")
	}
	if c.Body == "" && !c.Cancelled {
		return "", problem("body", "write the message: the essential warning goes in the text, not only in an attachment")
	}
	clean := func(xs []string) ([]string, error) {
		var out []string
		for _, t := range xs {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			if !strings.HasPrefix(t, "@") {
				t = strings.ToUpper(t)
			}
			switch {
			case t == ToDispatch, t == ToAll, t == ToFire, t == ToEMS, t == ToPolice:
			case strings.HasPrefix(t, ToUser), strings.HasPrefix(t, ToPosition):
				t = strings.ToLower(t[:strings.Index(t, ":")+1]) + t[strings.Index(t, ":")+1:]
			case strings.HasPrefix(t, ToRole):
				if !slices.Contains([]string{"admin", "supervisor", "dispatcher", "viewer", "field"}, strings.TrimPrefix(t, ToRole)) {
					return nil, problem("to", "no role %s", strings.TrimPrefix(t, ToRole))
				}
			case strings.HasPrefix(t, ToDuty):
			case strings.HasPrefix(t, ToAgency):
				g := strings.ToUpper(strings.TrimPrefix(t, ToAgency))
				if e.st.Agencies[g] == nil {
					return nil, problem("to", "no agency %s", g)
				}
				t = ToAgency + g
			case strings.HasPrefix(t, ToIncident):
				if e.st.Incidents[strings.TrimPrefix(t, ToIncident)] == nil {
					return nil, problem("to", "no incident %s", strings.TrimPrefix(t, ToIncident))
				}
			default:
				if e.st.Units[t] == nil {
					return nil, problem("to", "no unit %s (address a unit, ALL, DISPATCH, @agency:RPD, @role:supervisor, @duty:police, @user:name or @position:CT-1)", t)
				}
			}
			if !slices.Contains(out, t) {
				out = append(out, t)
			}
		}
		return out, nil
	}
	var err error
	if c.To, err = clean(c.To); err != nil {
		return "", err
	}
	if c.CC, err = clean(c.CC); err != nil {
		return "", err
	}
	if c.Agency = strings.ToUpper(strings.TrimSpace(c.Agency)); c.Agency == "" {
		c.Agency = a.Agency
	}
	if err := e.checkAgency("agency", c.Agency); err != nil {
		return "", err
	}
	if len(c.To) == 0 {
		// A BOLO or officer-safety notice goes by default to every CAD position and terminal of the agency.
		if (c.Kind == CommBOLO || c.Kind == CommSafety) && c.Agency != "" {
			c.To = []string{ToAgency + c.Agency}
		} else if c.Kind == CommBOLO || c.Kind == CommSafety {
			c.To = []string{ToDispatch, ToAll}
		} else {
			return "", problem("to", "say who it is for")
		}
	}
	if c.Incident != "" && e.st.Incidents[c.Incident] == nil {
		return "", problem("incident", "no incident %s", c.Incident)
	}
	for _, n := range c.Related {
		if e.st.Incidents[n] == nil {
			return "", problem("related", "no incident %s", n)
		}
	}
	for _, f := range c.Files {
		if x := e.st.CommFiles[f]; x == nil || x.Removed {
			return "", problem("files", "no attachment %s", f)
		}
	}
	if c.Expires.IsZero() && !c.NoExpiry && k.Hours > 0 && prev == nil {
		c.Expires = firstTime(c.Effective, now).Add(time.Duration(k.Hours) * time.Hour)
	}
	if c.NoExpiry && !k.NoExpiry {
		return "", problem("expires", "a %s needs an expiry", k.Name)
	}
	if c.NoExpiry {
		c.Expires = time.Time{}
	}
	if !c.Expires.IsZero() && !c.Expires.After(firstTime(c.Effective, now)) {
		return "", problem("expires", "it expires after it takes effect")
	}
	c.ID = fmt.Sprintf("C%s%d", e.sitePrefix, e.st.commCount+1)
	if prev == nil {
		c.Thread, c.Version = c.ID, 1
	}
	c.From, c.FromDuty, c.FromAgency, c.FromConsole = a.User, a.Duty, a.Agency, a.Console
	if u := e.st.userName(a.User); u != "" {
		c.FromName = u
	}
	if a.Role == "field" {
		c.FromUnit = a.Console
	}
	c.SupersededBy, c.Delivered, c.Opened, c.Acked, c.Escalated = "", nil, nil, nil, time.Time{}
	_, err = e.write(a, EvCommSent, refs{Incident: c.Incident, Unit: c.FromUnit}, c)
	return c.ID, err
}

// userName is a user's name as the ledger has it.
func (s *State) userName(u string) string {
	if x := s.Users[strings.ToLower(u)]; x != nil {
		return x.Name
	}
	return ""
}

// CommByID finds a version.
func (s *State) CommByID(id string) *Comm {
	if i, ok := s.commIdx[id]; ok && i < len(s.Comms) && s.Comms[i].ID == id {
		return s.Comms[i]
	}
	for _, c := range s.Comms {
		if c.ID == id {
			return c
		}
	}
	return nil
}

type receiptData struct {
	ID     string  `json:"id"`
	User   string  `json:"user"`
	What   string  `json:"what"` // delivered, opened, acked
	Rcpt   Receipt `json:"receipt"`
	Thread bool    `json:"thread,omitempty"`
}

// CommReceipt records that a person had a communication delivered, opened it or acknowledged it.
func (e *Engine) CommReceipt(a Actor, id, what string, r Reader) error {
	if what != "delivered" && what != "opened" && what != "acked" {
		return problem("what", "delivered, opened or acked")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	c := e.st.CommByID(id)
	if c == nil {
		return problem("id", "no communication %s", id)
	}
	if !e.st.MaySeeComm(c, r) {
		return problem("id", "that communication is not for you")
	}
	have := map[string]map[string]Receipt{"delivered": c.Delivered, "opened": c.Opened, "acked": c.Acked}[what]
	if _, done := have[strings.ToLower(r.User)]; done {
		return nil
	}
	_, err := e.write(a, EvCommReceipt, refs{Incident: c.Incident}, receiptData{ID: id, User: strings.ToLower(r.User), What: what, Rcpt: Receipt{Duty: r.Duty, Console: r.Console, Unit: r.Unit}})
	return err
}

// SaveCommDraft keeps a draft for its author.
func (e *Engine) SaveCommDraft(a Actor, d CommDraft) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if d.ID == "" {
		d.ID = fmt.Sprintf("D%s%d", e.sitePrefix, e.st.draftCount+1)
	} else if old := e.st.CommDrafts[d.ID]; old == nil || old.User != a.User {
		return "", problem("id", "no draft %s of yours", d.ID)
	}
	d.User = a.User
	if len(d.Comm.Body) > 8000 {
		return "", problem("body", "the body is up to 8,000 characters")
	}
	_, err := e.write(a, EvCommDraft, refs{}, d)
	return d.ID, err
}

// RemoveCommDraft discards a draft.
func (e *Engine) RemoveCommDraft(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if d := e.st.CommDrafts[id]; d == nil || d.User != a.User {
		return problem("id", "no draft %s of yours", id)
	}
	_, err := e.write(a, EvCommDraftGone, refs{}, idData{id})
	return err
}

// AddCommFile records a stored attachment.
func (e *Engine) AddCommFile(a Actor, f CommFile) error {
	f.Name = cleanName(f.Name)
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := e.write(a, EvCommFile, refs{}, f)
	return err
}

// OpenedCommFile records that a person opened or downloaded an attachment.
func (e *Engine) OpenedCommFile(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := e.write(a, EvCommFileOpen, refs{}, idData{id})
	return err
}

// RemoveCommFile hides an attachment (a supervisor, with a reason); the record and the file are kept.
func (e *Engine) RemoveCommFile(a Actor, id, why string) error {
	if strings.TrimSpace(why) == "" {
		return problem("why", "say why the attachment is removed")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.CommFiles[id] == nil {
		return problem("id", "no attachment %s", id)
	}
	_, err := e.write(a, EvCommFileGone, refs{}, map[string]string{"id": id, "why": why})
	return err
}

// EscalateComms tells the supervisors about urgent and high communications not acknowledged within their
// priority's time. It returns those escalated.
func (e *Engine) EscalateComms() ([]*Comm, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	var out []*Comm
	for _, c := range e.st.Comms {
		r := e.st.Settings.priorityRule(c.Priority)
		if r.Escalate <= 0 || !r.Ack || !c.Escalated.IsZero() || !c.Active(now) || len(c.Acked) > 0 {
			continue
		}
		if now.Sub(c.Issued) < time.Duration(r.Escalate)*time.Minute {
			continue
		}
		if _, err := e.write(SystemActor, EvCommEscalated, refs{Incident: c.Incident}, idData{c.ID}); err != nil {
			return out, err
		}
		out = append(out, c)
	}
	return out, nil
}

// commTimeline is how a communication reads on its incident's timeline.
func (s *State) commTimeline(c *Comm) string {
	k, _ := s.Settings.commKind(c.Kind)
	name := firstNonEmpty(k.Name, c.Kind)
	verb := "sent"
	switch {
	case c.Cancelled:
		verb = "cancelled"
	case c.Version > 1:
		verb = fmt.Sprintf("updated (version %d)", c.Version)
	}
	text := fmt.Sprintf("%s %s %s [%s] by %s to %s: %s", name, c.Thread, verb, strings.ToUpper(c.Priority), c.From, strings.Join(append(slices.Clone(c.To), c.CC...), ", "), c.Subject)
	if c.Body != "" {
		text += " — " + c.Body
	}
	if c.Why != "" {
		text += " (" + c.Why + ")"
	}
	if len(c.Files) > 0 {
		var names []string
		for _, f := range c.Files {
			if x := s.CommFiles[f]; x != nil {
				names = append(names, x.Name)
			}
		}
		text += "; attached: " + strings.Join(names, ", ")
	}
	return text
}

func (s *State) applyComms(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvCommSent:
		c, err := decode[Comm](e)
		if err != nil {
			return true, err
		}
		c.Issued = e.At
		s.commCount++
		if c.Of != "" {
			if p := s.CommByID(c.Of); p != nil {
				p.SupersededBy = c.ID
			}
		}
		s.Comms = append(s.Comms, &c)
		s.commIdx[c.ID] = len(s.Comms) - 1
		for _, n := range append([]string{c.Incident}, c.Related...) {
			if in := s.Incidents[n]; in != nil {
				in.add(e, "comm", s.commTimeline(&c))
			}
		}
		// A BOLO feeds the plate and name checks while it is out.
		if c.Kind == CommBOLO {
			b := &BOLO{ID: c.Thread, Kind: "other", Text: c.Subject + ": " + c.Body, Plate: c.Plate, Vehicle: c.Vehicle, Person: c.Person, Incident: c.Incident,
				Issued: e.At, By: c.From, Expires: c.Expires}
			if old := s.BOLOs[c.Thread]; old != nil {
				b.Issued, b.By = old.Issued, old.By
				b.Updates = append(old.Updates, Entry{At: e.At, Actor: e.Actor, Kind: "update", Text: c.Why})
			}
			switch {
			case c.Plate != "" || c.Vehicle != "":
				b.Kind = "vehicle"
			case c.Person != "":
				b.Kind = "person"
			}
			if c.Cancelled {
				b.Cancelled, b.CancelledBy = e.At, e.Actor
			}
			s.BOLOs[c.Thread] = b
		}
	case EvCommReceipt:
		v, err := decode[receiptData](e)
		if err != nil {
			return true, err
		}
		c := s.CommByID(v.ID)
		if c == nil {
			return true, nil
		}
		r := v.Rcpt
		r.At = e.At
		m := map[string]*map[string]Receipt{"delivered": &c.Delivered, "opened": &c.Opened, "acked": &c.Acked}[v.What]
		if m == nil {
			return true, nil
		}
		if *m == nil {
			*m = map[string]Receipt{}
		}
		(*m)[v.User] = r
		if v.What == "acked" {
			if c.Opened == nil {
				c.Opened = map[string]Receipt{}
			}
			if _, ok := c.Opened[v.User]; !ok {
				c.Opened[v.User] = r
			}
		}
	case EvCommDraft:
		d, err := decode[CommDraft](e)
		if err != nil {
			return true, err
		}
		d.Updated = e.At
		s.draftCount++
		s.CommDrafts[d.ID] = &d
	case EvCommDraftGone:
		v, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		delete(s.CommDrafts, v.ID)
	case EvCommFile:
		f, err := decode[CommFile](e)
		if err != nil {
			return true, err
		}
		f.At, f.By = e.At, e.Actor
		s.CommFiles[f.ID] = &f
	case EvCommFileOpen:
	case EvCommFileGone:
		v, err := decode[map[string]string](e)
		if err != nil {
			return true, err
		}
		if f := s.CommFiles[v["id"]]; f != nil {
			f.Removed, f.Why = true, v["why"]
		}
	case EvCommEscalated:
		v, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		if c := s.CommByID(v.ID); c != nil {
			c.Escalated = e.At
			if in := s.Incidents[c.Incident]; in != nil {
				in.add(e, "comm", fmt.Sprintf("%s %s not acknowledged in time: supervisors told", strings.ToUpper(c.Priority), c.Thread))
			}
		}
	default:
		return false, nil
	}
	return true, nil
}
