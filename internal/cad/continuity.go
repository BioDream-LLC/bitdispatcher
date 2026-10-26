package cad

// Continuity: scheduled map updates, the links the centre depends on and its outages, moving to a backup
// site, and overflow, where a neighbouring centre running bitdispatcher takes calls for this one (or this
// one for it) while it is overwhelmed or out of its building. The overflow links' addresses and tokens are
// the neighbouring agencies' (Admin › Integrations); what is here is the state both sides record.

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// Continuity events.
const (
	EvMapChecked        = "map.checked"           // a scheduled map update ran: started, skipped, up to date (record event)
	EvContinuityLink    = "continuity.link"       // a link the centre depends on went down or came back
	EvRelocation        = "continuity.relocation" // the centre declared it is working from a backup site, or is back
	EvRelocationStep    = "continuity.step"       // a step of the relocation checklist ticked or unticked
	EvOverflowSession   = "overflow.session"      // an overflow request, its answer, its end
	EvOverflowCall      = "overflow.call"         // a call this centre took for a neighbour, and what became of it
	EvOverflowIncident  = "overflow.incident"     // an incident a neighbour's call taker created here
	DefaultOverflowMins = 240
)

// MapSchedule is when the recorded map brings itself up to date (Admin › Map).
type MapSchedule struct {
	Mode    string `json:"mode,omitempty"`    // off (empty), daily or weekly
	Hour    int    `json:"hour,omitempty"`    // local hour it starts, 0 to 23
	Weekday int    `json:"weekday,omitempty"` // for weekly: 0 Sunday to 6 Saturday
	// MaxDropPct refuses an update that loses more than this percentage of road segments or addresses
	// (0: 10).
	MaxDropPct int `json:"maxDropPct,omitempty"`
}

// MaxDrop is the fraction an update may lose before it is refused.
func (m MapSchedule) MaxDrop() float64 {
	if m.MaxDropPct <= 0 {
		return 0.10
	}
	return float64(m.MaxDropPct) / 100
}

// Due says whether a run starts in the hour holding t, in the agency's time zone.
func (m MapSchedule) Due(t time.Time) bool {
	switch m.Mode {
	case "daily":
		return t.Hour() == m.Hour
	case "weekly":
		return t.Hour() == m.Hour && int(t.Weekday()) == m.Weekday
	}
	return false
}

// Next is the start of the next run after t, or zero when off.
func (m MapSchedule) Next(t time.Time) time.Time {
	if m.Mode != "daily" && m.Mode != "weekly" {
		return time.Time{}
	}
	n := time.Date(t.Year(), t.Month(), t.Day(), m.Hour, 0, 0, 0, t.Location())
	for !n.After(t) || m.Mode == "weekly" && int(n.Weekday()) != m.Weekday {
		n = n.AddDate(0, 0, 1)
	}
	return n
}

// ConnCheck is an address whose answer shows the centre can reach the internet.
type ConnCheck struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// ContinuityPolicy is how the centre watches its links and works through an outage (Admin › Continuity).
type ContinuityPolicy struct {
	// Checks are addresses that show the internet is reachable; any one answering is enough.
	Checks []ConnCheck `json:"checks,omitempty"`
	// CheckSeconds is how often links are checked (0: 30).
	CheckSeconds int `json:"checkSeconds,omitempty"`
	// Steps are the relocation checklist, shown when the centre moves to its backup site.
	Steps []string `json:"steps,omitempty"`
	// Contacts are printed on the continuity pack: the backup site, the neighbours, the phone company.
	Contacts []string `json:"contacts,omitempty"`
	// PackMinutes is how often the continuity pack is written (0: not written on its own).
	PackMinutes int `json:"packMinutes,omitempty"`
	// PackDirs are the folders the pack is written to: a shared drive, a USB disk (empty: the data
	// directory only).
	PackDirs []string `json:"packDirs,omitempty"`
	// OverflowMinutes is how long an overflow lasts unless ended or extended (0: 240).
	OverflowMinutes int `json:"overflowMinutes,omitempty"`
}

// CheckSecondsOr is the check interval.
func (p ContinuityPolicy) CheckSecondsOr() int {
	if p.CheckSeconds <= 0 {
		return 30
	}
	return p.CheckSeconds
}

// OverflowMinutesOr is how long an overflow lasts.
func (p ContinuityPolicy) OverflowMinutesOr() int {
	if p.OverflowMinutes <= 0 {
		return DefaultOverflowMins
	}
	return p.OverflowMinutes
}

// CheckContinuity checks the map schedule and continuity settings.
func CheckContinuity(m MapSchedule, p ContinuityPolicy) error {
	switch m.Mode {
	case "", "off", "daily", "weekly":
	default:
		return problem("mapUpdates", "automatic map updates are off, daily or weekly")
	}
	if m.Hour < 0 || m.Hour > 23 || m.Weekday < 0 || m.Weekday > 6 {
		return problem("mapUpdates", "the hour is 0 to 23 and the day Sunday to Saturday")
	}
	if m.MaxDropPct < 0 || m.MaxDropPct > 90 {
		return problem("mapUpdates", "an update may lose 1 to 90 percent before it is refused")
	}
	if len(p.Checks) > 10 {
		return problem("continuity", "up to 10 internet checks")
	}
	for _, c := range p.Checks {
		if strings.TrimSpace(c.Name) == "" || !strings.HasPrefix(c.URL, "https://") || len(c.URL) > 300 {
			return problem("continuity", "each internet check has a name and an https:// address")
		}
	}
	if p.CheckSeconds != 0 && (p.CheckSeconds < 10 || p.CheckSeconds > 600) {
		return problem("continuity", "links are checked every 10 to 600 seconds")
	}
	if len(p.Steps) > 60 || len(p.Contacts) > 60 {
		return problem("continuity", "up to 60 checklist steps and 60 contacts")
	}
	for _, x := range append(slices.Clone(p.Steps), p.Contacts...) {
		if len(x) > 300 {
			return problem("continuity", "each step and contact fits in 300 characters")
		}
	}
	if p.PackMinutes < 0 || p.PackMinutes > 240 {
		return problem("continuity", "the continuity pack is written every 1 to 240 minutes, or 0 for not on its own")
	}
	if len(p.PackDirs) > 5 {
		return problem("continuity", "up to 5 folders for the continuity pack")
	}
	for _, d := range p.PackDirs {
		if !strings.HasPrefix(d, "/") && !(len(d) > 2 && d[1] == ':') {
			return problem("continuity", "a folder for the continuity pack is a full path, such as /mnt/share/cad")
		}
	}
	if p.OverflowMinutes < 0 || p.OverflowMinutes > 7*24*60 {
		return problem("continuity", "an overflow lasts up to a week")
	}
	return nil
}

// LinkState is a link the centre depends on.
type LinkState struct {
	Name  string    `json:"name"`
	Up    bool      `json:"up"`
	Since time.Time `json:"since"`
	Error string    `json:"error,omitempty"`
}

// Outage is a link down, from its start to its end.
type Outage struct {
	Name  string    `json:"name"`
	From  time.Time `json:"from"`
	To    time.Time `json:"to,omitzero"`
	Error string    `json:"error,omitempty"`
}

// Relocation is the centre working from its backup site.
type Relocation struct {
	Active bool      `json:"active"`
	Site   string    `json:"site,omitempty"` // the site that declared it
	Note   string    `json:"note,omitempty"`
	By     string    `json:"by"`
	Since  time.Time `json:"since"`
	Ended  time.Time `json:"ended,omitzero"`
	Done   []int     `json:"done"` // checklist steps ticked
}

// OverflowType is a call type the asking centre lets its helper use.
type OverflowType struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Priority int    `json:"priority"`
}

// OverflowSession is one overflow between this centre and a neighbour. Out: we asked them to take our
// calls. In: they asked us to take theirs.
type OverflowSession struct {
	Partner     string         `json:"partner"`
	PartnerName string         `json:"partnerName"`
	Direction   string         `json:"direction"` // out or in
	Status      string         `json:"status"`    // asked, active, declined, ended
	Note        string         `json:"note,omitempty"`
	By          string         `json:"by"`
	Since       time.Time      `json:"since"`
	Until       time.Time      `json:"until"`
	AnsweredBy  string         `json:"answeredBy,omitempty"`
	Answered    time.Time      `json:"answered,omitzero"`
	EndedBy     string         `json:"endedBy,omitempty"`
	Ended       time.Time      `json:"ended,omitzero"`
	Dispatch    bool           `json:"dispatch"` // the helper may assign the asking centre's units
	Agencies    []string       `json:"agencies,omitempty"`
	CallTypes   []OverflowType `json:"callTypes,omitempty"`
}

// Key is how the session is kept.
func (o OverflowSession) Key() string { return o.Direction + ":" + o.Partner }

// Live says whether calls may be taken under it at t.
func (o OverflowSession) Live(t time.Time) bool { return o.Status == "active" && t.Before(o.Until) }

// OverflowUnit is a unit of the asking centre on an overflow call.
type OverflowUnit struct {
	Unit   string `json:"unit"`
	Status string `json:"status"`
}

// OverflowNote is a note on an overflow call.
type OverflowNote struct {
	At        time.Time `json:"at"`
	By        string    `json:"by"`
	Text      string    `json:"text"`
	Delivered bool      `json:"delivered"`
	Theirs    bool      `json:"theirs,omitempty"` // written at the asking centre
}

// OverflowCall is a call this centre took for a neighbour, and what became of it there.
type OverflowCall struct {
	Ref         string         `json:"ref"`
	Partner     string         `json:"partner"`
	Taken       time.Time      `json:"taken"`
	By          string         `json:"by"`
	Type        string         `json:"type"`
	TypeName    string         `json:"typeName,omitempty"`
	Priority    int            `json:"priority"`
	Address     string         `json:"address"`
	Lat         *float64       `json:"lat,omitempty"`
	Lon         *float64       `json:"lon,omitempty"`
	CallerName  string         `json:"callerName,omitempty"`
	Callback    string         `json:"callback,omitempty"`
	Narrative   string         `json:"narrative,omitempty"`
	Delivered   bool           `json:"delivered"`
	Tries       int            `json:"tries,omitempty"`
	Error       string         `json:"error,omitempty"`
	Number      string         `json:"number,omitempty"` // their incident number
	Status      string         `json:"status,omitempty"` // their incident's status
	Units       []OverflowUnit `json:"units,omitempty"`
	Closed      bool           `json:"closed,omitempty"`
	Disposition string         `json:"disposition,omitempty"`
	Notes       []OverflowNote `json:"notes,omitempty"`
	Updated     time.Time      `json:"updated"`
}

// OverflowIncident is an incident here a neighbour's call taker created.
type OverflowIncident struct {
	Number  string `json:"number"`
	Partner string `json:"partner"`
	Ref     string `json:"ref"`
	By      string `json:"by"`
}

type continuityState struct {
	Links             map[string]*LinkState        `json:"links"`
	Outages           []Outage                     `json:"outages"`
	Relocation        *Relocation                  `json:"relocation,omitempty"`
	Overflows         map[string]*OverflowSession  `json:"overflows"`
	OverflowCalls     map[string]*OverflowCall     `json:"overflowCalls"`
	OverflowIncidents map[string]*OverflowIncident `json:"overflowIncidents"` // by our incident number
}

func (s *State) initContinuity() {
	s.Links, s.Outages = map[string]*LinkState{}, []Outage{}
	s.Overflows, s.OverflowCalls, s.OverflowIncidents = map[string]*OverflowSession{}, map[string]*OverflowCall{}, map[string]*OverflowIncident{}
}

func (s *State) applyContinuity(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvContinuityLink:
		var l LinkState
		if err := json.Unmarshal(e.Data, &l); err != nil {
			return true, err
		}
		l.Since = e.At
		prev := s.Links[l.Name]
		s.Links[l.Name] = &l
		if !l.Up && (prev == nil || prev.Up) {
			s.Outages = append(s.Outages, Outage{Name: l.Name, From: e.At, Error: l.Error})
			if len(s.Outages) > 200 {
				s.Outages = s.Outages[len(s.Outages)-200:]
			}
		}
		if l.Up {
			for i := len(s.Outages) - 1; i >= 0; i-- {
				if s.Outages[i].Name == l.Name && s.Outages[i].To.IsZero() {
					s.Outages[i].To = e.At
					break
				}
			}
		}
	case EvRelocation:
		var r Relocation
		if err := json.Unmarshal(e.Data, &r); err != nil {
			return true, err
		}
		r.By = e.Actor
		if r.Active {
			r.Since, r.Done = e.At, []int{}
			s.Relocation = &r
		} else if s.Relocation != nil {
			s.Relocation.Active, s.Relocation.Ended = false, e.At
		}
	case EvRelocationStep:
		var d struct {
			Step int  `json:"step"`
			Done bool `json:"done"`
		}
		if err := json.Unmarshal(e.Data, &d); err != nil {
			return true, err
		}
		if r := s.Relocation; r != nil {
			r.Done = slices.DeleteFunc(r.Done, func(i int) bool { return i == d.Step })
			if d.Done {
				r.Done = append(r.Done, d.Step)
				slices.Sort(r.Done)
			}
		}
	case EvOverflowSession:
		var o OverflowSession
		if err := json.Unmarshal(e.Data, &o); err != nil {
			return true, err
		}
		s.Overflows[o.Key()] = &o
	case EvOverflowCall:
		var c OverflowCall
		if err := json.Unmarshal(e.Data, &c); err != nil {
			return true, err
		}
		c.Updated = e.At
		s.OverflowCalls[c.Ref] = &c
	case EvOverflowIncident:
		var o OverflowIncident
		if err := json.Unmarshal(e.Data, &o); err != nil {
			return true, err
		}
		s.OverflowIncidents[o.Number] = &o
	default:
		return false, nil
	}
	return true, nil
}

// OverflowFor is the session with a partner in a direction, or nil.
func (s *State) OverflowFor(direction, partner string) *OverflowSession {
	return s.Overflows[direction+":"+partner]
}

// OverflowByRef is the incident here a partner's call created, by their reference.
func (s *State) OverflowByRef(partner, ref string) *OverflowIncident {
	for _, o := range s.OverflowIncidents {
		if o.Partner == partner && o.Ref == ref {
			return o
		}
	}
	return nil
}

// SetLink records a link's state when it changes and says whether it did; the same state again writes
// nothing. With several sites, each site's links are its own: the name is prefixed with the site.
func (e *Engine) SetLink(name string, up bool, why string) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.site != "" {
		name = e.site + " · " + name
	}
	if len(why) > 300 {
		why = why[:300]
	}
	if up {
		why = ""
	}
	if l := e.st.Links[name]; l != nil && l.Up == up && l.Error == why {
		return false, nil
	}
	_, err := e.write(SystemActor, EvContinuityLink, refs{}, LinkState{Name: name, Up: up, Error: why})
	return err == nil, err
}

// Location is the agency's time zone (empty: the server's).
func (p ShiftPolicy) Location() *time.Location { return p.loc() }

// SetRelocation declares the centre working from this site as its backup, or back in its building.
func (e *Engine) SetRelocation(a Actor, active bool, note string) error {
	note = strings.TrimSpace(note)
	if len(note) > 1000 {
		return problem("note", "the note fits in 1000 characters")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	cur := e.st.Relocation
	if active && cur != nil && cur.Active {
		return problem("relocation", "the centre is already working from %s", firstNonEmpty(cur.Site, "its backup site"))
	}
	if !active && (cur == nil || !cur.Active) {
		return problem("relocation", "the centre is not working from a backup site")
	}
	_, err := e.write(a, EvRelocation, refs{}, Relocation{Active: active, Site: e.site, Note: note})
	return err
}

// TickRelocationStep ticks or unticks a checklist step.
func (e *Engine) TickRelocationStep(a Actor, step int, done bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r := e.st.Relocation; r == nil || !r.Active {
		return problem("relocation", "the centre is not working from a backup site")
	}
	if step < 0 || step >= len(e.st.Settings.Continuity.Steps) {
		return problem("step", "no such step")
	}
	_, err := e.write(a, EvRelocationStep, refs{}, map[string]any{"step": step, "done": done})
	return err
}

// SaveOverflowSession records an overflow request, answer or end.
func (e *Engine) SaveOverflowSession(a Actor, o OverflowSession) error {
	if o.Direction != "out" && o.Direction != "in" {
		return problem("direction", "an overflow is out (they take our calls) or in (we take theirs)")
	}
	switch o.Status {
	case "asked", "active", "declined", "ended":
	default:
		return problem("status", "an overflow is asked, active, declined or ended")
	}
	if o.Partner == "" || len(o.Note) > 1000 || len(o.CallTypes) > 500 {
		return problem("overflow", "an overflow names its partner; its note fits in 1000 characters")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := e.write(a, EvOverflowSession, refs{}, o)
	return err
}

// SaveOverflowCall records a call taken for a neighbour, or what became of it.
func (e *Engine) SaveOverflowCall(a Actor, c OverflowCall) error {
	if c.Ref == "" || c.Partner == "" {
		return problem("overflow", "an overflow call has a reference and a partner")
	}
	if len(c.Narrative) > 8000 || len(c.Address) > 300 || len(c.Notes) > 500 {
		return problem("overflow", "the call is too long")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := e.write(a, EvOverflowCall, refs{}, c)
	return err
}

// OverflowCallByRef is a copy of a call taken for a neighbour.
func (e *Engine) OverflowCallByRef(ref string) (OverflowCall, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	c := e.st.OverflowCalls[ref]
	if c == nil {
		return OverflowCall{}, false
	}
	cp := *c
	cp.Units, cp.Notes = slices.Clone(c.Units), slices.Clone(c.Notes)
	return cp, true
}

// CreateOverflowIncident creates an incident from a call a neighbour's call taker took for us, under their
// name, and links it to their reference. The same reference again returns the incident already made.
func (e *Engine) CreateOverflowIncident(a Actor, partner, partnerName, ref string, n NewIncident) (string, bool, error) {
	e.mu.Lock()
	if o := e.st.OverflowByRef(partner, ref); o != nil {
		e.mu.Unlock()
		return o.Number, false, nil
	}
	e.mu.Unlock()
	number, err := e.CreateIncident(a, n)
	if err != nil {
		return "", false, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, err := e.write(a, EvOverflowIncident, refs{Incident: number}, OverflowIncident{Number: number, Partner: partner, Ref: ref, By: a.User}); err != nil {
		return number, true, err
	}
	_, err = e.write(a, EvIncNote, refs{Incident: number}, noteData{Text: fmt.Sprintf("Taken for us by %s (overflow), call taker %s, their reference %s",
		partnerName, strings.TrimPrefix(a.User, partner+":"), ref)})
	return number, true, err
}
