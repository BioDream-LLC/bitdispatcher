package cad

// Tow rotation: police call wreckers from a list of approved tow companies in turn, so each gets a fair
// share. Companies tow some classes (light, medium, heavy) in some zones. Asking for the next tow takes the
// company whose last turn in that class and zone is the oldest; a company that declines or does not answer
// has had its turn, and the next is offered. An owner's own choice of company is recorded but takes no
// one's turn.

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvTowSaved     = "tow.saved"     // a tow company was added or changed
	EvTowRemoved   = "tow.removed"   // a tow company left the rotation list
	EvTowRequested = "tow.requested" // a tow was asked for: the company called, and why it was its turn
	EvTowResult    = "tow.result"    // what the company said, or that it arrived
)

// TowCompany is one company on the rotation list.
type TowCompany struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Phone   string   `json:"phone"`
	Address string   `json:"address,omitempty"`
	Classes []string `json:"classes"`         // the classes it tows
	Zones   []string `json:"zones,omitempty"` // the zones it serves; empty: every zone
	Off     bool     `json:"off,omitempty"`   // suspended from the rotation
	Note    string   `json:"note,omitempty"`
}

// Tow is one tow asked for.
type Tow struct {
	ID       string    `json:"id"`
	At       time.Time `json:"at"`
	By       string    `json:"by"`
	Incident string    `json:"incident,omitempty"`
	Company  string    `json:"company"`
	Name     string    `json:"name"` // the company's name when called
	Phone    string    `json:"phone"`
	Class    string    `json:"class"`
	Zone     string    `json:"zone,omitempty"`
	Kind     string    `json:"kind"`              // rotation, owner (the owner's choice), or police (an officer's choice: no turn)
	Skipped  []string  `json:"skipped,omitempty"` // companies passed over, and why
	Vehicle  string    `json:"vehicle,omitempty"`
	Result   string    `json:"result"` // called, accepted, declined, no-answer, cancelled, arrived
	ResultAt time.Time `json:"resultAt,omitzero"`
	Note     string    `json:"note,omitempty"`
}

// Tow kinds and results.
const (
	TowRotation = "rotation"
	TowOwner    = "owner"
	TowPolice   = "police"

	TowCalled    = "called"
	TowAccepted  = "accepted"
	TowDeclined  = "declined"
	TowNoAnswer  = "no-answer"
	TowCancelled = "cancelled"
	TowArrived   = "arrived"
)

// keepTows is how many tows the live state keeps (the ledger keeps all of them).
const keepTows = 1000

// TowClassList is the agency's classes of tow.
func (s Settings) TowClassList() []string {
	if len(s.TowClasses) == 0 {
		return []string{"light", "medium", "heavy"}
	}
	return s.TowClasses
}

type towResult struct {
	ID     string `json:"id"`
	Result string `json:"result"`
	Note   string `json:"note,omitempty"`
}

// SaveTowCompany adds or changes a tow company.
func (e *Engine) SaveTowCompany(a Actor, c TowCompany) (string, error) {
	c.Name, c.Phone = strings.TrimSpace(c.Name), strings.TrimSpace(c.Phone)
	if c.Name == "" {
		return "", problem("name", "give the company's name")
	}
	if c.Phone == "" {
		return "", problem("phone", "give the number dispatch calls")
	}
	c.Classes, c.Zones = dedupe(c.Classes), dedupe(c.Zones)
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(c.Classes) == 0 {
		return "", problem("classes", "choose the classes it tows: %s", strings.Join(e.st.Settings.TowClassList(), ", "))
	}
	for _, k := range c.Classes {
		if !slices.Contains(e.st.Settings.TowClassList(), k) {
			return "", problem("classes", "no tow class %s (Admin › Settings lists them)", k)
		}
	}
	for _, z := range c.Zones {
		if len(e.st.Settings.TowZones) > 0 && !slices.Contains(e.st.Settings.TowZones, z) {
			return "", problem("zones", "no tow zone %s (Admin › Settings lists them)", z)
		}
	}
	if c.ID == "" {
		c.ID = e.newID("TOW", func(id string) bool { return e.st.TowCompanies[id] != nil })
	}
	_, err := e.write(a, EvTowSaved, refs{}, c)
	return c.ID, err
}

// RemoveTowCompany takes a company off the list (its past tows stay in the ledger).
func (e *Engine) RemoveTowCompany(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.TowCompanies[id] == nil {
		return problem("id", "no tow company %s", id)
	}
	_, err := e.write(a, EvTowRemoved, refs{}, idData{id})
	return err
}

// TowTurn is where a company stands in a rotation.
type TowTurn struct {
	Company  *TowCompany `json:"company"`
	LastTurn time.Time   `json:"lastTurn,omitzero"`
	Why      string      `json:"why,omitempty"` // why it cannot take this tow
}

// TowRotationFor is the rotation for a class and zone: the companies that can take it, next first, and
// those that cannot, with why.
func (s *State) TowRotationFor(class, zone string) (eligible, passed []TowTurn) {
	last := map[string]time.Time{}
	for _, t := range s.Tows {
		if t.Kind == TowRotation && t.Class == class && t.Zone == zone {
			last[t.Company] = t.At
		}
	}
	for _, c := range s.TowCompanies {
		turn := TowTurn{Company: c, LastTurn: last[c.ID]}
		switch {
		case c.Off:
			turn.Why = "suspended from the rotation"
		case !slices.Contains(c.Classes, class):
			turn.Why = "does not tow " + class
		case zone != "" && len(c.Zones) > 0 && !slices.Contains(c.Zones, zone):
			turn.Why = "does not serve " + zone
		}
		if turn.Why == "" {
			eligible = append(eligible, turn)
		} else {
			passed = append(passed, turn)
		}
	}
	sort.Slice(eligible, func(i, j int) bool {
		a, b := eligible[i], eligible[j]
		if !a.LastTurn.Equal(b.LastTurn) {
			return a.LastTurn.Before(b.LastTurn) // never had a turn, then the longest ago
		}
		return a.Company.Name < b.Company.Name
	})
	sort.Slice(passed, func(i, j int) bool { return passed[i].Company.Name < passed[j].Company.Name })
	return eligible, passed
}

// RequestTow asks for a tow. Kind rotation takes the next company for the class and zone; owner and police
// name a company and take no turn.
func (e *Engine) RequestTow(a Actor, incident, class, zone, kind, company, vehicle, note string) (Tow, error) {
	if kind == "" {
		kind = TowRotation
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if incident != "" && e.st.Incidents[incident] == nil {
		return Tow{}, problem("incident", "no incident %s", incident)
	}
	if !slices.Contains(e.st.Settings.TowClassList(), class) {
		return Tow{}, problem("class", "choose the class of tow: %s", strings.Join(e.st.Settings.TowClassList(), ", "))
	}
	if zone == "" && len(e.st.Settings.TowZones) > 0 {
		return Tow{}, problem("zone", "choose the tow zone: %s", strings.Join(e.st.Settings.TowZones, ", "))
	}
	t := Tow{Incident: incident, Class: class, Zone: zone, Kind: kind, Vehicle: strings.TrimSpace(vehicle), Note: strings.TrimSpace(note), Result: TowCalled}
	switch kind {
	case TowRotation:
		el, _ := e.st.TowRotationFor(class, zone)
		// A company already called for this incident and class that declined is not offered again.
		for len(el) > 0 && incident != "" && slices.ContainsFunc(e.st.Tows, func(x *Tow) bool {
			return x.Incident == incident && x.Class == class && x.Company == el[0].Company.ID && (x.Result == TowDeclined || x.Result == TowNoAnswer)
		}) {
			t.Skipped = append(t.Skipped, el[0].Company.Name+": already declined this tow")
			el = el[1:]
		}
		if len(el) == 0 {
			return Tow{}, problem("class", "no company on the rotation tows %s%s", class, inZone(zone))
		}
		t.Company = el[0].Company.ID
	case TowOwner, TowPolice:
		if e.st.TowCompanies[company] == nil {
			return Tow{}, problem("company", "choose the company")
		}
		t.Company = company
	default:
		return Tow{}, problem("kind", "a tow is by rotation, the owner's choice or the officer's")
	}
	c := e.st.TowCompanies[t.Company]
	t.Name, t.Phone = c.Name, c.Phone
	t.ID = fmt.Sprintf("TW%s%d", e.sitePrefix, e.st.towCount+1)
	_, err := e.write(a, EvTowRequested, refs{Incident: incident}, t)
	return t, err
}

func inZone(z string) string {
	if z == "" {
		return ""
	}
	return " in " + z
}

// TowAnswer records what the company said (accepted, declined, no-answer), that the tow was cancelled, or
// that the wrecker arrived.
func (e *Engine) TowAnswer(a Actor, id, result, note string) error {
	switch result {
	case TowAccepted, TowDeclined, TowNoAnswer, TowCancelled, TowArrived:
	default:
		return problem("result", "say accepted, declined, no-answer, cancelled or arrived")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	t := e.st.tow(id)
	if t == nil {
		return problem("id", "no tow %s", id)
	}
	_, err := e.write(a, EvTowResult, refs{Incident: t.Incident}, towResult{ID: id, Result: result, Note: strings.TrimSpace(note)})
	return err
}

func (s *State) tow(id string) *Tow {
	for i := len(s.Tows) - 1; i >= 0; i-- {
		if s.Tows[i].ID == id {
			return s.Tows[i]
		}
	}
	return nil
}

// OpenTows are the tows not yet arrived or cancelled, and those of the last hours, newest first.
func (s *State) OpenTows(now time.Time) []*Tow {
	var out []*Tow
	for i := len(s.Tows) - 1; i >= 0; i-- {
		t := s.Tows[i]
		done := t.Result == TowArrived || t.Result == TowCancelled || t.Result == TowDeclined || t.Result == TowNoAnswer
		if !done || now.Sub(t.ResultAt) < 6*time.Hour {
			out = append(out, t)
		}
		if len(out) >= 100 {
			break
		}
	}
	return out
}

func (s *State) applyTows(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvTowSaved:
		c, err := decode[TowCompany](e)
		if err != nil {
			return true, err
		}
		s.TowCompanies[c.ID] = &c
	case EvTowRemoved:
		v, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		delete(s.TowCompanies, v.ID)
	case EvTowRequested:
		t, err := decode[Tow](e)
		if err != nil {
			return true, err
		}
		t.At, t.By = e.At, e.Actor
		s.Tows = append(s.Tows, &t)
		if len(s.Tows) > keepTows {
			s.Tows = slices.Clone(s.Tows[len(s.Tows)-keepTows:])
		}
		s.towCount++
		if in := s.Incidents[t.Incident]; in != nil {
			why := map[string]string{TowRotation: "next on the " + t.Class + " rotation" + inZone(t.Zone), TowOwner: "the owner's choice", TowPolice: "the officer's choice"}[t.Kind]
			text := fmt.Sprintf("Tow %s: called %s %s (%s)", t.ID, t.Name, t.Phone, why)
			if len(t.Skipped) > 0 {
				text += "; passed over " + strings.Join(t.Skipped, "; ")
			}
			if t.Vehicle != "" {
				text += " for " + t.Vehicle
			}
			in.add(e, "tow", text)
		}
	case EvTowResult:
		v, err := decode[towResult](e)
		if err != nil {
			return true, err
		}
		t := s.tow(v.ID)
		if t == nil {
			return true, nil
		}
		t.Result, t.ResultAt = v.Result, e.At
		if v.Note != "" {
			t.Note = strings.TrimSpace(t.Note + " " + v.Note)
		}
		if in := s.Incidents[t.Incident]; in != nil {
			in.add(e, "tow", fmt.Sprintf("Tow %s: %s %s%s", t.ID, t.Name, map[string]string{TowAccepted: "accepted", TowDeclined: "declined (lost its turn)", TowNoAnswer: "did not answer (lost its turn)", TowCancelled: "cancelled", TowArrived: "arrived"}[v.Result], suffix(v.Note)))
		}
	default:
		return false, nil
	}
	return true, nil
}
