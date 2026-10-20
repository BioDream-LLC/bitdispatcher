package cad

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/geo"
)

// Candidate is one unit a recommendation names, with why.
type Candidate struct {
	Unit       string   `json:"unit"`
	Type       string   `json:"type"`
	Capability string   `json:"capability"` // the run card line it fills
	Metres     float64  `json:"metres,omitempty"`
	Seconds    float64  `json:"seconds,omitempty"`
	From       string   `json:"from"`             // avl, station, stale-avl, unknown
	Routed     bool     `json:"routed,omitempty"` // Seconds is a drive over the road network, not an estimate
	Reasons    []string `json:"reasons"`
}

// Line is one run card line and the units chosen for it.
type Line struct {
	Capability string      `json:"capability"`
	Count      int         `json:"count"`
	Covered    []string    `json:"covered"` // units already on the incident that fill it
	Units      []Candidate `json:"units"`
	Short      int         `json:"short"` // how many could not be found
	Next       []Candidate `json:"next"`  // the next best, for the dispatcher to swap in
}

// Recommendation is what bitdispatcher suggests sending. The dispatcher accepts or changes it; it never
// dispatches anything on its own.
type Recommendation struct {
	Incident  string    `json:"incident"`
	Type      string    `json:"type"`
	Lines     []Line    `json:"lines"`
	Units     []string  `json:"units"`
	Notes     []string  `json:"notes"`
	Estimate  string    `json:"estimate"` // how travel times were estimated
	Generated time.Time `json:"generated"`
	// Plan is the response plan followed, and Planned its units in due order with where each is: a planned
	// unit that cannot go is shown with who goes instead.
	Plan    string     `json:"plan,omitempty"`
	PlanID  string     `json:"planId,omitempty"`
	Planned []PlanUnit `json:"planned,omitempty"`
	// Warnings must be confirmed before dispatching: the location is not verified, is uncertain, or missing.
	Warnings []string `json:"warnings"`
}

// PlanUnit is one unit of a response plan's due order, and whether it can go.
type PlanUnit struct {
	Unit   string `json:"unit"`
	Due    int    `json:"due"` // 1 for first due
	Status string `json:"status"`
	Can    bool   `json:"can"`
	Why    string `json:"why,omitempty"` // why it cannot: on scene at 26-000004, out of service
}

func distance(a, b Position) float64 { return geo.Distance(a.Lat, a.Lon, b.Lat, b.Lon) }

// Router gives driving times over a road network. bitdispatcher's recorded map is one.
type Router interface {
	// TravelTo prepares driving times to a destination; label says how they are found, for the screen.
	TravelTo(lat, lon float64) (TravelFunc, string)
}

// TravelFunc is the drive from a point to the prepared destination; ok is false when the road network
// does not connect them (or the point is outside the recorded area).
type TravelFunc func(lat, lon float64) (metres, seconds float64, via []string, ok bool)

// Geocoder finds the exact position of an address or intersection a dispatcher typed.
type Geocoder interface {
	Geocode(text string) (lat, lon float64, label string, ok bool)
}

// SetGeocoder sets the address lookup the command line uses; nil for none.
func (e *Engine) SetGeocoder(g Geocoder) {
	e.mu.Lock()
	e.geocoder = g
	e.mu.Unlock()
}

// SetRouter sets the road network recommendations use; nil goes back to straight-line estimates.
func (e *Engine) SetRouter(r Router) {
	e.mu.Lock()
	e.router = r
	e.mu.Unlock()
}

// Recommend builds the recommendation for an incident from its call type's run card: for each line, the
// closest available units with that capability, counting units already on the incident.
func (e *Engine) Recommend(number string) (Recommendation, error) {
	e.mu.Lock()
	var lat, lon float64
	have := false
	if in, ok := e.st.Incidents[number]; ok && in.Location.HasPoint() {
		lat, lon, have = *in.Location.Lat, *in.Location.Lon, true
	}
	r := e.router
	e.mu.Unlock()
	// The road search runs without the lock: it takes a few tens of milliseconds.
	var tf TravelFunc
	label := ""
	if r != nil && have {
		tf, label = r.TravelTo(lat, lon)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.st.recommend(number, e.now(), tf, label)
}

func (s *State) recommend(number string, now time.Time, tf TravelFunc, label string) (Recommendation, error) {
	in, ok := s.Incidents[number]
	if !ok {
		return Recommendation{}, problem("incident", "no incident %s", number)
	}
	set := s.Settings
	r := Recommendation{Incident: number, Type: in.Type, Lines: []Line{}, Units: []string{}, Notes: []string{}, Warnings: []string{}, Generated: now,
		Estimate: fmt.Sprintf("Straight line × %.1f at %.0f km/h; no road network loaded", set.RoadFactor, set.SpeedKmh)}
	if tf != nil {
		r.Estimate = label
	}
	ct, ok := s.CallTypes[in.Type]
	if !ok || len(ct.RunCard) == 0 {
		r.Notes = append(r.Notes, "This call type has no run card: choose units by hand")
		return r, nil
	}
	hasPoint := in.Location.HasPoint()
	if !hasPoint {
		r.Notes = append(r.Notes, "The incident has no map position, so units are not ranked by distance: place it on the map")
		r.Warnings = append(r.Warnings, "No map position: the units are not ranked by distance")
	} else if in.Location.Radius > 150 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("The location is uncertain (±%.0f m): confirm it before sending", in.Location.Radius))
	} else if !in.LocVerified && in.Location.Address == "" {
		r.Warnings = append(r.Warnings, "The location is a caller position, not a verified address")
	}
	// A response plan for the area, address, type and time puts its units first, in due order.
	plan := s.PlanFor(in, now)
	due := map[string]int{}
	if plan != nil {
		r.Plan, r.PlanID = plan.Name, plan.ID
		for i, id := range plan.Order {
			due[id] = i + 1
			pu := PlanUnit{Unit: id, Due: i + 1}
			if u, ok := s.Units[id]; ok {
				pu.Status = u.Status
				switch {
				case u.Status == Available:
					pu.Can = true
				case u.Incident == number && Committed(u.Status):
					pu.Can, pu.Why = true, "already on this incident"
				case Committed(u.Status):
					pu.Why = fmt.Sprintf("%s at %s", u.Status, u.Incident)
				case u.Status == MarkedOut && u.MarkOut != nil:
					pu.Why = "marked out (" + u.MarkOut.Code + ")"
				default:
					pu.Why = u.Status
				}
			} else {
				pu.Why = "no such unit"
			}
			r.Planned = append(r.Planned, pu)
		}
	}
	// Units already on the incident fill lines first.
	onScene := []string{}
	for _, a := range in.Units {
		if a.Cleared.IsZero() {
			onScene = append(onScene, a.Unit)
		}
	}
	used := map[string]bool{}
	// Scarce capabilities are filled first, so a multi-role unit is not spent on a line others could fill.
	lines := make([]Line, len(ct.RunCard))
	order := make([]int, len(ct.RunCard))
	avail := map[string]int{}
	for i, req := range ct.RunCard {
		lines[i] = Line{Capability: req.Capability, Count: req.Count, Covered: []string{}, Units: []Candidate{}, Next: []Candidate{}}
		order[i] = i
		for _, u := range s.Units {
			if u.Status == Available && slices.Contains(s.Capabilities(u), req.Capability) {
				avail[req.Capability]++
			}
		}
	}
	sort.SliceStable(order, func(a, b int) bool {
		return avail[ct.RunCard[order[a]].Capability] < avail[ct.RunCard[order[b]].Capability]
	})
	for _, i := range order {
		l := &lines[i]
		for _, id := range onScene {
			if len(l.Covered) < l.Count && !used[id] && slices.Contains(s.Capabilities(s.Units[id]), l.Capability) {
				l.Covered = append(l.Covered, id)
				used[id] = true
			}
		}
		need := l.Count - len(l.Covered)
		var cands []Candidate
		for _, u := range s.Units {
			if used[u.ID] || u.Status != Available || !slices.Contains(s.Capabilities(u), l.Capability) {
				continue
			}
			cands = append(cands, s.candidate(u, in, l.Capability, now, tf))
		}
		sort.SliceStable(cands, func(a, b int) bool {
			ca, cb := cands[a], cands[b]
			// The plan's due order first: its units before any other, in their order.
			da, db := due[ca.Unit], due[cb.Unit]
			if (da > 0) != (db > 0) {
				return da > 0
			}
			if da > 0 && da != db {
				return da < db
			}
			if (ca.Seconds == 0) != (cb.Seconds == 0) {
				return ca.Seconds != 0 // known travel times before unknown
			}
			if ca.Seconds != cb.Seconds {
				return ca.Seconds < cb.Seconds
			}
			return ca.Unit < cb.Unit
		})
		for _, c := range cands {
			if need > 0 {
				used[c.Unit] = true
				l.Units = append(l.Units, c)
				r.Units = append(r.Units, c.Unit)
				need--
			} else if len(l.Next) < 3 {
				l.Next = append(l.Next, c)
			}
		}
		if need > 0 {
			l.Short = need
			// Units on a mark-out that can be interrupted (a meal, a report) are the next place to look.
			var pull []Candidate
			for _, u := range s.Units {
				if used[u.ID] || u.Status != MarkedOut || u.MarkOut == nil || !slices.Contains(s.Capabilities(u), l.Capability) {
					continue
				}
				if t, ok := set.MarkOutType(u.MarkOut.Code); ok && t.Interruptible {
					c := s.candidate(u, in, l.Capability, now, tf)
					c.Reasons = append([]string{"Marked out (" + t.Name + "): can be pulled"}, c.Reasons...)
					pull = append(pull, c)
				}
			}
			sort.SliceStable(pull, func(a, b int) bool { return pull[a].Seconds < pull[b].Seconds })
			for _, c := range pull {
				if len(l.Next) < 3 {
					l.Next = append(l.Next, c)
				}
			}
			msg := fmt.Sprintf("No available unit for %d × %s: consider mutual aid", need, l.Capability)
			if len(pull) > 0 {
				msg = fmt.Sprintf("No available unit for %d × %s: %s is marked out and can be pulled, or consider mutual aid", need, l.Capability, pull[0].Unit)
			}
			r.Notes = append(r.Notes, msg)
		}
	}
	for i := range lines {
		for j := range lines[i].Units {
			c := &lines[i].Units[j]
			if d := due[c.Unit]; d > 0 {
				c.Reasons = append([]string{fmt.Sprintf("%s due on %s", ordinal(d), plan.Name)}, c.Reasons...)
				// Who it replaces: planned units ahead of it that cannot go.
				for _, pu := range r.Planned {
					if pu.Due < d && !pu.Can && s.Units[pu.Unit] != nil && slices.Contains(s.Capabilities(s.Units[pu.Unit]), c.Capability) {
						c.Reasons = append(c.Reasons, fmt.Sprintf("instead of %s (%s)", pu.Unit, pu.Why))
					}
				}
			} else if j == 0 && len(lines[i].Units) > 0 {
				c.Reasons = append([]string{"Closest available with " + c.Capability}, c.Reasons...)
			}
		}
	}
	r.Lines = lines
	// Cover: stations the recommendation would leave with no available unit.
	byStation := map[string][]string{}
	for _, u := range s.Units {
		if u.post() != "" && u.Status == Available {
			byStation[u.post()] = append(byStation[u.post()], u.ID)
		}
	}
	for st, ids := range byStation {
		left := 0
		for _, id := range ids {
			if !slices.Contains(r.Units, id) {
				left++
			}
		}
		if left == 0 {
			name := st
			if x, ok := s.Stations[st]; ok {
				name = x.Name
			}
			r.Notes = append(r.Notes, "Sending these leaves "+name+" with no available unit: consider a move-up")
		}
	}
	return r, nil
}

func (s *State) candidate(u *Unit, in *Incident, capability string, now time.Time, tf TravelFunc) Candidate {
	c := Candidate{Unit: u.ID, Type: u.Type, Capability: capability, From: "unknown", Reasons: []string{}}
	var lat, lon float64
	have := false
	stale := time.Duration(s.Settings.AVLStaleSeconds) * time.Second
	st, hasStation := s.Stations[u.post()]
	switch {
	case u.Position != nil && now.Sub(u.Position.At) <= stale:
		lat, lon, have, c.From = u.Position.Lat, u.Position.Lon, true, "avl"
		c.Reasons = append(c.Reasons, "AVL "+ago(now.Sub(u.Position.At)))
	case hasStation:
		lat, lon, have, c.From = st.Lat, st.Lon, true, "station"
		if u.Position != nil {
			c.Reasons = append(c.Reasons, "AVL "+ago(now.Sub(u.Position.At))+" (stale): from "+st.Name)
		} else {
			c.Reasons = append(c.Reasons, "From "+st.Name+" (no AVL)")
		}
	case u.Position != nil:
		lat, lon, have, c.From = u.Position.Lat, u.Position.Lon, true, "stale-avl"
		c.Reasons = append(c.Reasons, "Only a stale AVL position ("+ago(now.Sub(u.Position.At))+")")
	default:
		c.Reasons = append(c.Reasons, "Position unknown: no AVL and no station")
	}
	if have && in.Location.HasPoint() {
		routed := false
		if tf != nil {
			if m, sec, via, ok := tf(lat, lon); ok {
				c.Metres, c.Seconds, c.Routed = math.Round(m), math.Max(1, math.Round(sec)), true
				reason := fmt.Sprintf("%.1f mi by road, %s", m/1609.344, mins(sec))
				if len(via) > 0 {
					reason += " via " + strings.Join(mainRoads(via, 2), ", ")
				}
				c.Reasons = append(c.Reasons, reason)
				routed = true
			}
		}
		if !routed {
			est := geo.Drive(lat, lon, *in.Location.Lat, *in.Location.Lon, s.Settings.RoadFactor, s.Settings.SpeedKmh)
			c.Metres, c.Seconds = math.Round(est.Metres), math.Max(1, math.Round(est.Seconds))
			note := "(straight-line estimate)"
			if tf != nil {
				note = "(straight-line estimate: no road route)"
			}
			c.Reasons = append(c.Reasons, fmt.Sprintf("%.1f mi, about %s %s", est.Metres/1609.344, mins(est.Seconds), note))
		}
	}
	if !u.LastHeard.IsZero() && now.Sub(u.LastHeard) < 10*time.Minute {
		c.Reasons = append(c.Reasons, "On the radio "+ago(now.Sub(u.LastHeard)))
	}
	return c
}

func ordinal(n int) string {
	switch n {
	case 1:
		return "1st"
	case 2:
		return "2nd"
	case 3:
		return "3rd"
	}
	return fmt.Sprintf("%dth", n)
}

func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%02dm ago", int(d.Hours()), int(d.Minutes())%60)
}

func mins(sec float64) string {
	if sec < 60 {
		return "under a minute"
	}
	m := int(math.Round(sec / 60))
	return fmt.Sprintf("%d min", m)
}

// TypeName is a call type's name.
func (s *State) TypeName(code string) string { return s.typeLabel(code) }

// FindIncident resolves a reference a dispatcher types: a full number, or its last digits.
func (s *State) FindIncident(ref string) string {
	ref = strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(ref)), "#")
	if ref == "" {
		return ""
	}
	if _, ok := s.Incidents[ref]; ok {
		return ref
	}
	var match []string
	for n, in := range s.Incidents {
		if in.Status != Closed && strings.HasSuffix(n, ref) && strings.Trim(ref, "0123456789") == "" {
			match = append(match, n)
		}
	}
	if len(match) == 1 {
		return match[0]
	}
	return ""
}

// mainRoads is the first n of the roads that carry most of a drive.
func mainRoads(via []string, n int) []string {
	if len(via) <= n {
		return via
	}
	return via[:n]
}
