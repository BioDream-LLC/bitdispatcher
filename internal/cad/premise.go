package cad

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/geo"
)

// Premises are what a centre knows about a place before anyone calls: hazards to the crews (a dog, oxygen
// in use, an armed occupant, a Knox box), notes and contacts. A premise is shown with every incident at
// it, on the console and on the mobile terminal. History is the incidents an earlier CAD system kept,
// brought over so that "this address has called before" includes the years before bitdispatcher.

// Premise is one place's information.
type Premise struct {
	ID       string    `json:"id"`
	Name     string    `json:"name,omitempty"`
	Address  string    `json:"address"`
	Lat      float64   `json:"lat"`
	Lon      float64   `json:"lon"`
	Hazards  []string  `json:"hazards,omitempty"`
	Notes    string    `json:"notes,omitempty"`
	Contacts []string  `json:"contacts,omitempty"`
	Expires  string    `json:"expires,omitempty"` // YYYY-MM-DD; after it the premise is shown as expired
	Source   string    `json:"source,omitempty"`  // where it was imported from
	Updated  time.Time `json:"updated"`
	By       string    `json:"by"`
}

// History is one incident from an earlier system.
type History struct {
	Number      string    `json:"number"`
	Type        string    `json:"type,omitempty"`
	Address     string    `json:"address,omitempty"`
	Lat         float64   `json:"lat"`
	Lon         float64   `json:"lon"`
	At          time.Time `json:"at"`
	Disposition string    `json:"disposition,omitempty"`
	Note        string    `json:"note,omitempty"`
	Source      string    `json:"source,omitempty"`
}

type premiseBatch struct {
	Source   string    `json:"source"`
	Premises []Premise `json:"premises"`
}

type historyBatch struct {
	Source string    `json:"source"`
	Items  []History `json:"items"`
}

// grid finds things near a point: cells of about 110 m.
type grid map[[2]int32][]int

func cell(lat, lon float64) [2]int32 {
	return [2]int32{int32(math.Floor(lat * 1000)), int32(math.Floor(lon * 1000))}
}

func (g grid) near(lat, lon float64, fn func(i int)) {
	c := cell(lat, lon)
	for dy := int32(-1); dy <= 1; dy++ {
		for dx := int32(-1); dx <= 1; dx++ {
			for _, i := range g[[2]int32{c[0] + dy, c[1] + dx}] {
				fn(i)
			}
		}
	}
}

var rePremiseID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,40}$`)

// PremiseRadius is how near an incident a premise is shown.
const PremiseRadius = 40.0

func checkPremise(p *Premise) error {
	p.Name, p.Address, p.Notes = strings.TrimSpace(p.Name), strings.TrimSpace(p.Address), strings.TrimSpace(p.Notes)
	if p.Address == "" {
		return problem("address", "give the address")
	}
	if p.ID != "" && !rePremiseID.MatchString(p.ID) {
		return problem("id", "an id is up to 40 letters, digits, dots, dashes and underscores")
	}
	if p.Lat == 0 && p.Lon == 0 || math.Abs(p.Lat) > 90 || math.Abs(p.Lon) > 180 {
		return problem("address", "%s has no position: pick it from the search, or give its latitude and longitude", p.Address)
	}
	var hz []string
	for _, h := range p.Hazards {
		if h = strings.TrimSpace(h); h != "" && !slices.Contains(hz, h) {
			hz = append(hz, h)
		}
	}
	p.Hazards = hz
	var cs []string
	for _, c := range p.Contacts {
		if c = strings.TrimSpace(c); c != "" {
			cs = append(cs, c)
		}
	}
	p.Contacts = cs
	if p.Expires != "" {
		if _, err := time.Parse("2006-01-02", p.Expires); err != nil {
			return problem("expires", "the expiry date is YYYY-MM-DD")
		}
	}
	if len(p.Notes) > 4000 || len(p.Hazards) > 20 {
		return problem("notes", "too long")
	}
	return nil
}

// SavePremise adds or changes a premise.
func (e *Engine) SavePremise(a Actor, p Premise) (string, error) {
	if err := checkPremise(&p); err != nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if p.ID == "" {
		e.st.premCount++
		p.ID = fmt.Sprintf("P%s%06d", e.sitePrefix, e.st.premCount)
		for e.st.Premises[p.ID] != nil {
			e.st.premCount++
			p.ID = fmt.Sprintf("P%s%06d", e.sitePrefix, e.st.premCount)
		}
	}
	p.Updated, p.By = e.now(), a.User
	_, err := e.write(a, EvPremiseSaved, refs{}, p)
	return p.ID, err
}

// RemovePremise deletes a premise.
func (e *Engine) RemovePremise(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.Premises[id] == nil {
		return problem("id", "no premise %s", id)
	}
	_, err := e.write(a, EvPremiseRemoved, refs{}, idData{ID: id})
	return err
}

// ImportPremises adds many premises, in batches of a few hundred per ledger event. A premise with the
// id of one already there replaces it, so a second import of the same file changes nothing twice.
func (e *Engine) ImportPremises(a Actor, source string, ps []Premise) (int, error) {
	now := e.now()
	for i := range ps {
		if err := checkPremise(&ps[i]); err != nil {
			return 0, fmt.Errorf("row %d: %w", i+1, err)
		}
		ps[i].Source, ps[i].Updated, ps[i].By = source, now, a.User
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range ps {
		if ps[i].ID == "" {
			e.st.premCount++
			ps[i].ID = fmt.Sprintf("P%s%06d", e.sitePrefix, e.st.premCount)
		}
	}
	for i := 0; i < len(ps); i += 500 {
		if _, err := e.write(a, EvPremisesImported, refs{}, premiseBatch{Source: source, Premises: ps[i:min(i+500, len(ps))]}); err != nil {
			return i, err
		}
	}
	return len(ps), nil
}

// ImportHistory adds incidents from an earlier system.
func (e *Engine) ImportHistory(a Actor, source string, hs []History) (int, error) {
	for i := range hs {
		h := &hs[i]
		if h.At.IsZero() {
			return 0, fmt.Errorf("row %d: no date", i+1)
		}
		if h.Lat == 0 && h.Lon == 0 {
			return 0, fmt.Errorf("row %d: no position", i+1)
		}
		h.Source = source
		if len(h.Note) > 2000 {
			h.Note = h.Note[:2000]
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := 0; i < len(hs); i += 1000 {
		if _, err := e.write(a, EvHistoryImported, refs{}, historyBatch{Source: source, Items: hs[i:min(i+1000, len(hs))]}); err != nil {
			return i, err
		}
	}
	return len(hs), nil
}

func (s *State) putPremise(p Premise) {
	if old, ok := s.Premises[p.ID]; ok {
		s.dropPremise(old)
	}
	pp := p
	s.Premises[p.ID] = &pp
	c := cell(p.Lat, p.Lon)
	s.premGrid[c] = append(s.premGrid[c], s.premIndex(p.ID))
	var n int
	if _, err := fmt.Sscanf(strings.TrimLeft(p.ID, "PABCDEFGHIJKLMNOQRSTUVWXYZ"), "%d", &n); err == nil && strings.HasPrefix(p.ID, "P") && n > s.premCount {
		s.premCount = n
	}
}

// premIndex keeps premise ids in a slice so the grid holds small numbers.
func (s *State) premIndex(id string) int {
	if i, ok := s.premIdx[id]; ok {
		return i
	}
	s.premIDs = append(s.premIDs, id)
	s.premIdx[id] = len(s.premIDs) - 1
	return len(s.premIDs) - 1
}

func (s *State) dropPremise(p *Premise) {
	c := cell(p.Lat, p.Lon)
	i := s.premIdx[p.ID]
	s.premGrid[c] = slices.DeleteFunc(s.premGrid[c], func(x int) bool { return x == i })
	delete(s.Premises, p.ID)
}

func (s *State) addHistory(h History) {
	s.History = append(s.History, h)
	c := cell(h.Lat, h.Lon)
	s.histGrid[c] = append(s.histGrid[c], len(s.History)-1)
}

// PremisesNear are the premises within PremiseRadius of a point, nearest first.
func (s *State) PremisesNear(lat, lon float64) []*Premise {
	if lat == 0 && lon == 0 {
		return nil
	}
	type hit struct {
		p *Premise
		d float64
	}
	var hs []hit
	s.premGrid.near(lat, lon, func(i int) {
		if p := s.Premises[s.premIDs[i]]; p != nil {
			if d := geo.Distance(lat, lon, p.Lat, p.Lon); d <= PremiseRadius {
				hs = append(hs, hit{p, d})
			}
		}
	})
	sort.Slice(hs, func(i, j int) bool { return hs[i].d < hs[j].d })
	out := make([]*Premise, len(hs))
	for i, h := range hs {
		out[i] = h.p
	}
	return out
}

// HistoryNear are earlier-system incidents within metres of a point, newest first.
func (s *State) HistoryNear(lat, lon, metres float64, since time.Time) []History {
	if lat == 0 && lon == 0 {
		return nil
	}
	var out []History
	s.histGrid.near(lat, lon, func(i int) {
		h := s.History[i]
		if h.At.After(since) && geo.Distance(lat, lon, h.Lat, h.Lon) <= metres {
			out = append(out, h)
		}
	})
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}
