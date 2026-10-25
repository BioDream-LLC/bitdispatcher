package cad

// Road closures: a road a dispatcher or supervisor closes (a crash blocking it, a flooded underpass, road
// work, a parade). A closure is a line drawn along the road or a circle around a place; while it is in
// force, drive times and unit recommendations go around it, the map draws it, and crews see it. Each
// closure has a reason, may have an end time and a detour, and is an attributed event like everything
// else. Which road segments a closure takes is worked out over the recorded map by the API, not here.

import (
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvClosureSaved   = "road.closure.saved"
	EvClosureLifted  = "road.closure.lifted"
	EvClosureRemoved = "road.closure.removed"
)

// RoadClosure is a closed road.
type RoadClosure struct {
	ID     string       `json:"id"`
	Reason string       `json:"reason"`         // what closed it: "crash", "flooding", "road work"
	Line   [][2]float64 `json:"line,omitempty"` // [lon, lat] points along the road, for a stretch
	// Center ([lat, lon]) and Radius (metres) close every road in a circle instead.
	Center   *[2]float64 `json:"center,omitempty"`
	Radius   float64     `json:"radius,omitempty"`
	From     time.Time   `json:"from"`            // in force from
	Until    *time.Time  `json:"until,omitempty"` // lifted on its own at; empty: until lifted
	Detour   string      `json:"detour,omitempty"`
	Incident string      `json:"incident,omitempty"` // the incident that closed it
	By       string      `json:"by"`
	At       time.Time   `json:"at"`
	Lifted   *time.Time  `json:"lifted,omitempty"`
	LiftedBy string      `json:"liftedBy,omitempty"`
}

// InForce reports whether the closure applies at a time.
func (c *RoadClosure) InForce(t time.Time) bool {
	if c.Lifted != nil || t.Before(c.From) {
		return false
	}
	return c.Until == nil || t.Before(*c.Until)
}

// SaveClosure closes a road, or changes a closure.
func (e *Engine) SaveClosure(a Actor, c RoadClosure) (string, error) {
	c.Reason = strings.TrimSpace(c.Reason)
	c.Detour = strings.TrimSpace(c.Detour)
	switch {
	case c.Reason == "":
		return "", problem("reason", "say why the road is closed")
	case len(c.Reason) > 200 || len(c.Detour) > 400:
		return "", problem("reason", "keep the reason under 200 characters and the detour under 400")
	case c.Center == nil && len(c.Line) < 2:
		return "", problem("line", "draw the closed stretch (at least two points along the road) or choose a circle")
	case c.Center != nil && len(c.Line) > 0:
		return "", problem("line", "a closure is a stretch of road or a circle, not both")
	case c.Center != nil && (c.Radius < 5 || c.Radius > 3000):
		return "", problem("radius", "a circle is 5 m to 3 km across its radius")
	case len(c.Line) > 200:
		return "", problem("line", "up to 200 points")
	}
	for _, p := range c.Line {
		if p[0] < -180 || p[0] > 180 || p[1] < -90 || p[1] > 90 {
			return "", problem("line", "a point is not a position")
		}
	}
	if c.Center != nil && (c.Center[0] < -90 || c.Center[0] > 90 || c.Center[1] < -180 || c.Center[1] > 180) {
		return "", problem("center", "the centre is not a position")
	}
	now := e.now()
	if c.From.IsZero() {
		c.From = now
	}
	if c.Until != nil && !c.Until.After(c.From) {
		return "", problem("until", "the end is after the start")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if c.Incident != "" && e.st.Incidents[c.Incident] == nil {
		return "", problem("incident", "no incident %s", c.Incident)
	}
	if c.ID == "" {
		c.ID = e.newID("RC", func(id string) bool { return e.st.Closures[id] != nil })
	} else if old := e.st.Closures[c.ID]; old == nil {
		return "", problem("id", "no closure %s", c.ID)
	} else if old.Lifted != nil {
		return "", problem("id", "%s was lifted; close the road again instead", c.ID)
	}
	c.By, c.At, c.Lifted, c.LiftedBy = a.User, now, nil, ""
	_, err := e.write(a, EvClosureSaved, refs{Incident: c.Incident}, c)
	return c.ID, err
}

// LiftClosure opens the road again; the closure is kept, lifted, for the record.
func (e *Engine) LiftClosure(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	c := e.st.Closures[id]
	if c == nil {
		return problem("id", "no closure %s", id)
	}
	if c.Lifted != nil {
		return problem("id", "%s is already lifted", id)
	}
	_, err := e.write(a, EvClosureLifted, refs{Incident: c.Incident}, idData{id})
	return err
}

// RemoveClosure removes a closure entered by mistake (a supervisor's correction); it stays in the ledger.
func (e *Engine) RemoveClosure(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.Closures[id] == nil {
		return problem("id", "no closure %s", id)
	}
	_, err := e.write(a, EvClosureRemoved, refs{}, idData{id})
	return err
}

// ClosuresInForce are the closures that apply now.
func (s *State) ClosuresInForce(t time.Time) []*RoadClosure {
	var out []*RoadClosure
	for _, c := range s.Closures {
		if c.InForce(t) {
			out = append(out, c)
		}
	}
	return out
}

func (s *State) applyClosures(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvClosureSaved:
		c, err := decode[RoadClosure](e)
		if err != nil {
			return true, err
		}
		s.Closures[c.ID] = &c
		if in := s.Incidents[c.Incident]; in != nil {
			in.add(e, "road", "Road closed: "+c.Reason+closureDetour(c.Detour))
		}
	case EvClosureLifted:
		v, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		if c := s.Closures[v.ID]; c != nil {
			at := e.At
			c.Lifted, c.LiftedBy = &at, e.Actor
			if in := s.Incidents[c.Incident]; in != nil {
				in.add(e, "road", "Road open again: "+c.Reason)
			}
		}
	case EvClosureRemoved:
		v, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		delete(s.Closures, v.ID)
	default:
		return false, nil
	}
	return true, nil
}

func closureDetour(d string) string {
	if d == "" {
		return ""
	}
	return " · detour: " + d
}
