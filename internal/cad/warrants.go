package cad

// Warrants and civil process: the papers a sheriff's office or police department holds to serve — arrest
// and bench warrants, protective orders, evictions, summonses and subpoenas — with the person, the
// address, the court, who tried to serve them and when. An incident near the address of an active paper
// says so (with any caution, such as armed), so the officers sent know before they arrive; a paper is
// served, recalled or returned to the court, and every attempt is kept.

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/geo"
	"github.com/biodream-llc/bitdispatcher/internal/ledger"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

const (
	EvWarrantSaved   = "warrant.saved"   // a warrant or civil paper was entered or changed
	EvWarrantAttempt = "warrant.attempt" // an attempt to serve it
	EvWarrantStatus  = "warrant.status"  // served, recalled, returned, or active again
)

// Paper kinds.
var PaperKinds = []string{"arrest", "bench", "protective-order", "eviction", "summons", "subpoena", "civil-other"}

// Paper statuses.
const (
	PaperActive   = "active"
	PaperServed   = "served"
	PaperRecalled = "recalled" // the court withdrew it
	PaperReturned = "returned" // returned to the court unserved
)

// Paper is a warrant or a civil paper to serve.
type Paper struct {
	ID       string          `json:"id"`
	Kind     string          `json:"kind"`
	Number   string          `json:"number,omitempty"` // the court's or agency's number
	Court    string          `json:"court,omitempty"`
	Subject  PaperSubject    `json:"subject"`
	Location *ng911.Location `json:"location,omitempty"` // where to serve it
	Charge   string          `json:"charge,omitempty"`   // the offence, or what the paper is
	Issued   time.Time       `json:"issued,omitzero"`
	Expires  time.Time       `json:"expires,omitzero"` // a court date or last day to serve
	Caution  string          `json:"caution,omitempty"`
	Notes    string          `json:"notes,omitempty"`
	Status   string          `json:"status"`
	StatusAt time.Time       `json:"statusAt,omitzero"`
	StatusBy string          `json:"statusBy,omitempty"`
	Attempts []PaperAttempt  `json:"attempts"`
	Entered  time.Time       `json:"entered"`
	By       string          `json:"by"`
}

// PaperSubject is the person a paper names.
type PaperSubject struct {
	Name        string `json:"name"`
	DOB         string `json:"dob,omitempty"` // YYYY-MM-DD
	Sex         string `json:"sex,omitempty"`
	Description string `json:"description,omitempty"`
}

// PaperAttempt is one attempt to serve a paper.
type PaperAttempt struct {
	At       time.Time `json:"at"`
	By       string    `json:"by"`
	Unit     string    `json:"unit,omitempty"`
	Result   string    `json:"result"` // no answer, not there, moved, refused, served
	Note     string    `json:"note,omitempty"`
	Incident string    `json:"incident,omitempty"`
}

type paperStatus struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
	Unit   string `json:"unit,omitempty"`
}

type paperAttempt struct {
	ID string `json:"id"`
	PaperAttempt
}

// PaperRadius is how near an incident must be to a paper's address for the incident to show it.
const PaperRadius = 60.0

// SavePaper enters or changes a warrant or civil paper.
func (e *Engine) SavePaper(a Actor, p Paper) (string, error) {
	p.Subject.Name = strings.TrimSpace(p.Subject.Name)
	switch {
	case !slices.Contains(PaperKinds, p.Kind):
		return "", problem("kind", "choose the kind: %s", strings.Join(PaperKinds, ", "))
	case p.Subject.Name == "":
		return "", problem("subject", "give the name of the person it names")
	case p.Subject.DOB != "" && !validDate(p.Subject.DOB):
		return "", problem("dob", "a date of birth is YYYY-MM-DD")
	case p.Location == nil || (!p.Location.HasPoint() && strings.TrimSpace(p.Location.Address) == ""):
		return "", problem("location", "give where to serve it")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if old := e.st.Papers[p.ID]; old != nil {
		p.Status, p.StatusAt, p.StatusBy, p.Attempts, p.Entered, p.By = old.Status, old.StatusAt, old.StatusBy, old.Attempts, old.Entered, old.By
	} else {
		if p.ID != "" {
			return "", problem("id", "no paper %s", p.ID)
		}
		p.ID = fmt.Sprintf("WR%s%d", e.sitePrefix, e.st.paperCount+1)
		p.Status = PaperActive
		p.Attempts = []PaperAttempt{}
	}
	_, err := e.write(a, EvWarrantSaved, refs{}, p)
	return p.ID, err
}

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// AttemptPaper records an attempt to serve a paper. A result of "served" serves it.
func (e *Engine) AttemptPaper(a Actor, id, unit, result, note, incident string) error {
	result = strings.TrimSpace(result)
	if result == "" {
		return problem("result", "say what happened: no answer, not there, moved, refused, served")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	p := e.st.Papers[id]
	if p == nil {
		return problem("id", "no paper %s", id)
	}
	if p.Status != PaperActive {
		return problem("id", "%s is %s", id, p.Status)
	}
	if _, err := e.write(a, EvWarrantAttempt, refs{Unit: strings.ToUpper(unit), Incident: incident},
		paperAttempt{ID: id, PaperAttempt: PaperAttempt{Unit: strings.ToUpper(unit), Result: result, Note: strings.TrimSpace(note), Incident: incident}}); err != nil {
		return err
	}
	if strings.EqualFold(result, "served") {
		_, err := e.write(a, EvWarrantStatus, refs{Unit: strings.ToUpper(unit), Incident: incident}, paperStatus{ID: id, Status: PaperServed, Note: note, Unit: strings.ToUpper(unit)})
		return err
	}
	return nil
}

// SetPaperStatus serves, recalls, returns or makes active again a paper.
func (e *Engine) SetPaperStatus(a Actor, id, status, note, unit string) error {
	switch status {
	case PaperActive, PaperServed, PaperRecalled, PaperReturned:
	default:
		return problem("status", "a paper is active, served, recalled or returned")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	p := e.st.Papers[id]
	if p == nil {
		return problem("id", "no paper %s", id)
	}
	if p.Status == status {
		return problem("status", "%s is already %s", id, status)
	}
	if status != PaperServed && strings.TrimSpace(note) == "" {
		return problem("note", "say why")
	}
	_, err := e.write(a, EvWarrantStatus, refs{Unit: strings.ToUpper(unit)}, paperStatus{ID: id, Status: status, Note: strings.TrimSpace(note), Unit: strings.ToUpper(unit)})
	return err
}

// PapersNear are the active papers within PaperRadius of a point, or at the same address.
func (s *State) PapersNear(loc *ng911.Location) []*Paper {
	if loc == nil {
		return nil
	}
	addr := normAddr(loc.Address)
	var out []*Paper
	for _, p := range s.Papers {
		if p.Status != PaperActive || p.Location == nil {
			continue
		}
		near := addr != "" && normAddr(p.Location.Address) == addr
		if !near && loc.HasPoint() && p.Location.HasPoint() {
			near = geo.Distance(*loc.Lat, *loc.Lon, *p.Location.Lat, *p.Location.Lon) <= PaperRadius
		}
		if near {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// PapersNamed are the active papers whose subject has a name like one of the names given (each word of
// the paper's name appears in the name).
func (s *State) PapersNamed(names []string) []*Paper {
	var out []*Paper
	for _, p := range s.Papers {
		if p.Status != PaperActive {
			continue
		}
		pw := strings.Fields(strings.ToLower(strings.ReplaceAll(p.Subject.Name, ",", " ")))
		for _, n := range names {
			nl := " " + strings.ToLower(strings.ReplaceAll(n, ",", " ")) + " "
			all := len(pw) > 1
			for _, w := range pw {
				if !strings.Contains(nl, " "+w+" ") {
					all = false
				}
			}
			if all {
				out = append(out, p)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// PaperList is the papers of a status ("" for all), newest first.
func (s *State) PaperList(status string) []*Paper {
	var out []*Paper
	for _, p := range s.Papers {
		if status == "" || p.Status == status {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Entered.After(out[j].Entered) })
	return out
}

// normAddr is an address's street part, for matching: lower case, no city, no apartment or suite.
func normAddr(a string) string {
	a = strings.ToLower(strings.TrimSpace(a))
	if i := strings.Index(a, ","); i > 0 {
		a = a[:i]
	}
	f := strings.Fields(a)
	for i, w := range f {
		if i > 1 && (strings.HasPrefix(w, "#") || w == "apt" || w == "unit" || w == "ste" || w == "suite" || w == "lot" || w == "rm") {
			f = f[:i]
			break
		}
	}
	return strings.Join(f, " ")
}

func (s *State) applyPapers(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvWarrantSaved:
		p, err := decode[Paper](e)
		if err != nil {
			return true, err
		}
		if s.Papers[p.ID] == nil {
			p.Entered, p.By = e.At, e.Actor
			s.paperCount++
		}
		s.Papers[p.ID] = &p
	case EvWarrantAttempt:
		v, err := decode[paperAttempt](e)
		if err != nil {
			return true, err
		}
		if p := s.Papers[v.ID]; p != nil {
			at := v.PaperAttempt
			at.At, at.By = e.At, e.Actor
			p.Attempts = append(p.Attempts, at)
			if in := s.Incidents[at.Incident]; in != nil {
				in.add(e, "paper", fmt.Sprintf("Service attempt %s (%s): %s%s", p.ID, p.Subject.Name, at.Result, suffix(at.Note)))
			}
		}
	case EvWarrantStatus:
		v, err := decode[paperStatus](e)
		if err != nil {
			return true, err
		}
		if p := s.Papers[v.ID]; p != nil {
			p.Status, p.StatusAt, p.StatusBy = v.Status, e.At, e.Actor
			if v.Note != "" {
				p.Notes = strings.TrimSpace(p.Notes + "\n" + e.At.Format("2006-01-02") + " " + v.Status + ": " + v.Note)
			}
			if in := s.Incidents[e.Incident]; in != nil && e.Incident != "" {
				in.add(e, "paper", fmt.Sprintf("%s (%s) %s", p.ID, p.Subject.Name, v.Status))
			}
		}
	default:
		return false, nil
	}
	return true, nil
}
