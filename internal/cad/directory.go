package cad

// The contacts directory: the numbers dispatch calls — utilities, other agencies, the medical examiner,
// interpreters, public works — and on-call lists that say who answers tonight. A directory entry may be
// on the call-out list of some call types, so an incident of that type lists it with its number to
// call, and the dispatcher logs each call on the incident.

import (
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvDirSaved      = "directory.saved"
	EvDirRemoved    = "directory.removed"
	EvOnCallSaved   = "oncall.saved"
	EvOnCallRemoved = "oncall.removed"
)

// DirEntry is one entry in the contacts directory.
type DirEntry struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`               // a person or a desk: "Water department after hours"
	Org      string   `json:"org,omitempty"`      // the organisation
	Category string   `json:"category,omitempty"` // utility, agency, medical, public works, interpreter, other
	Phones   []Phone  `json:"phones"`
	Email    string   `json:"email,omitempty"`
	Address  string   `json:"address,omitempty"`
	Notes    string   `json:"notes,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	// Types are the call types whose incidents list this entry to call (the call-out list).
	Types []string `json:"types,omitempty"`
}

// Phone is one number of an entry.
type Phone struct {
	Label  string `json:"label,omitempty"` // main, after hours, cell, fax
	Number string `json:"number"`
}

// OnCallList is a rota: who is on call for something, by date and time.
type OnCallList struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`             // "Water main breaks", "Public works on call"
	Types  []string      `json:"types,omitempty"`  // call types whose incidents list who is on call
	Shifts []OnCallShift `json:"shifts"`           // in any order
	Backup string        `json:"backup,omitempty"` // a directory entry to call when no shift covers now
	Notes  string        `json:"notes,omitempty"`
}

// OnCallShift is one person's turn on a rota: a directory entry from one time to another.
type OnCallShift struct {
	Entry string    `json:"entry"`
	From  time.Time `json:"from"`
	To    time.Time `json:"to"`
}

// Directory categories offered on the screen.
var DirCategories = []string{"utility", "agency", "medical", "public works", "interpreter", "towing", "media", "other"}

// SaveDirEntry adds or changes a directory entry.
func (e *Engine) SaveDirEntry(a Actor, d DirEntry) (string, error) {
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" {
		return "", problem("name", "give the entry a name")
	}
	var ps []Phone
	for _, p := range d.Phones {
		p.Number, p.Label = strings.TrimSpace(p.Number), strings.TrimSpace(p.Label)
		if p.Number != "" {
			ps = append(ps, p)
		}
	}
	if len(ps) == 0 && strings.TrimSpace(d.Email) == "" {
		return "", problem("phones", "give at least one number or an email address")
	}
	d.Phones, d.Tags, d.Types = ps, dedupe(d.Tags), dedupe(d.Types)
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, t := range d.Types {
		if e.st.CallTypes[t] == nil {
			return "", problem("types", "no call type %s", t)
		}
	}
	if d.ID == "" {
		d.ID = e.newID("C", func(id string) bool { return e.st.Directory[id] != nil })
	} else if e.st.Directory[d.ID] == nil {
		return "", problem("id", "no directory entry %s", d.ID)
	}
	_, err := e.write(a, EvDirSaved, refs{}, d)
	return d.ID, err
}

// RemoveDirEntry removes a directory entry that no on-call list uses.
func (e *Engine) RemoveDirEntry(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.Directory[id] == nil {
		return problem("id", "no directory entry %s", id)
	}
	for _, l := range e.st.OnCall {
		if l.Backup == id || slices.ContainsFunc(l.Shifts, func(s OnCallShift) bool { return s.Entry == id }) {
			return problem("id", "%s is on the on-call list %s: take it off first", e.st.Directory[id].Name, l.Name)
		}
	}
	_, err := e.write(a, EvDirRemoved, refs{}, idData{id})
	return err
}

// SaveOnCall adds or changes an on-call list.
func (e *Engine) SaveOnCall(a Actor, l OnCallList) (string, error) {
	l.Name = strings.TrimSpace(l.Name)
	if l.Name == "" {
		return "", problem("name", "give the list a name")
	}
	l.Types = dedupe(l.Types)
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, s := range l.Shifts {
		if e.st.Directory[s.Entry] == nil {
			return "", problem("shifts", "shift %d: choose who from the directory", i+1)
		}
		if !s.To.After(s.From) {
			return "", problem("shifts", "shift %d ends before it starts", i+1)
		}
	}
	if l.Backup != "" && e.st.Directory[l.Backup] == nil {
		return "", problem("backup", "no directory entry %s", l.Backup)
	}
	for _, t := range l.Types {
		if e.st.CallTypes[t] == nil {
			return "", problem("types", "no call type %s", t)
		}
	}
	sort.Slice(l.Shifts, func(i, j int) bool { return l.Shifts[i].From.Before(l.Shifts[j].From) })
	if l.ID == "" {
		l.ID = e.newID("OC", func(id string) bool { return e.st.OnCall[id] != nil })
	} else if e.st.OnCall[l.ID] == nil {
		return "", problem("id", "no on-call list %s", l.ID)
	}
	_, err := e.write(a, EvOnCallSaved, refs{}, l)
	return l.ID, err
}

// RemoveOnCall removes an on-call list.
func (e *Engine) RemoveOnCall(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.OnCall[id] == nil {
		return problem("id", "no on-call list %s", id)
	}
	_, err := e.write(a, EvOnCallRemoved, refs{}, idData{id})
	return err
}

// OnCallNow is who is on call on a list at a time: the directory entry, until when, and whether it is
// the backup because no shift covers the time.
func (s *State) OnCallNow(l *OnCallList, at time.Time) (entry *DirEntry, until time.Time, backup bool) {
	for _, sh := range l.Shifts {
		if !at.Before(sh.From) && at.Before(sh.To) {
			return s.Directory[sh.Entry], sh.To, false
		}
	}
	if l.Backup != "" {
		return s.Directory[l.Backup], time.Time{}, true
	}
	return nil, time.Time{}, false
}

// CallOut is one line of an incident's call-out list: whom to call, with the number.
type CallOut struct {
	Entry  *DirEntry `json:"entry,omitempty"`
	List   string    `json:"list,omitempty"`   // the on-call list it comes from
	Until  time.Time `json:"until,omitzero"`   // on call until
	Backup bool      `json:"backup,omitempty"` // no one is on call: the list's backup
	Notify string    `json:"notify,omitempty"` // the call type's notification it answers
}

// CallOutFor is an incident's call-out list: the directory entries and on-call lists for its call type,
// and the call type's notifications matched to entries by name.
func (s *State) CallOutFor(in *Incident, at time.Time) []CallOut {
	var out []CallOut
	seen := map[string]bool{}
	for _, l := range s.OnCall {
		if slices.Contains(l.Types, in.Type) {
			d, until, backup := s.OnCallNow(l, at)
			out = append(out, CallOut{Entry: d, List: l.Name, Until: until, Backup: backup})
			if d != nil {
				seen[d.ID] = true
			}
		}
	}
	for _, d := range s.Directory {
		if slices.Contains(d.Types, in.Type) && !seen[d.ID] {
			out = append(out, CallOut{Entry: d})
			seen[d.ID] = true
		}
	}
	if ct := s.CallTypes[in.Type]; ct != nil {
		for _, n := range ct.Notify {
			for _, d := range s.Directory {
				if !seen[d.ID] && (strings.EqualFold(d.Name, n) || strings.EqualFold(d.Org, n) || slices.ContainsFunc(d.Tags, func(t string) bool { return strings.EqualFold(t, n) })) {
					out = append(out, CallOut{Entry: d, Notify: n})
					seen[d.ID] = true
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].List > out[j].List })
	return out
}

// SearchDirectory finds entries whose name, organisation, category, tags, numbers or notes contain every
// word of q; an empty q is every entry. By name.
func (s *State) SearchDirectory(q string) []*DirEntry {
	words := strings.Fields(strings.ToLower(q))
	var out []*DirEntry
	for _, d := range s.Directory {
		hay := strings.ToLower(strings.Join(append([]string{d.Name, d.Org, d.Category, d.Notes, d.Email, d.Address}, d.Tags...), " "))
		for _, p := range d.Phones {
			hay += " " + strings.ToLower(p.Label) + " " + onlyDigits(p.Number)
		}
		ok := true
		for _, w := range words {
			if !strings.Contains(hay, w) && !(onlyDigits(w) != "" && strings.Contains(hay, onlyDigits(w))) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *State) applyDirectory(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvDirSaved:
		d, err := decode[DirEntry](e)
		if err != nil {
			return true, err
		}
		s.Directory[d.ID] = &d
	case EvDirRemoved:
		v, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		delete(s.Directory, v.ID)
	case EvOnCallSaved:
		l, err := decode[OnCallList](e)
		if err != nil {
			return true, err
		}
		s.OnCall[l.ID] = &l
	case EvOnCallRemoved:
		v, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		delete(s.OnCall, v.ID)
	default:
		return false, nil
	}
	return true, nil
}
