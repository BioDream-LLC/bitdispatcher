package cad

// Geofences: areas drawn on the map. An alert area tells dispatch when a unit enters or leaves it (a
// unit leaving the county, entering a hazard zone, arriving at the hospital's ramp); a caution area puts its
// note on every incident inside it (a school zone, a range, a known hostile block). Whether a unit is
// inside is worked out as each position is applied, so a replay of the ledger gives the same alerts.

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/geo"
	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvGeofenceSaved   = "geofence.saved"
	EvGeofenceRemoved = "geofence.removed"
)

// Geofence kinds.
const (
	FenceAlerting = "alert"
	FenceCaution  = "caution"
)

// Geofence is an area on the map.
type Geofence struct {
	ID    string       `json:"id"`
	Name  string       `json:"name"`
	Kind  string       `json:"kind"` // alert or caution
	Ring  [][2]float64 `json:"ring"` // [lon, lat] corners
	Note  string       `json:"note,omitempty"`
	Enter bool         `json:"enter,omitempty"` // alert when a unit enters
	Leave bool         `json:"leave,omitempty"` // alert when a unit leaves
	// Disciplines and Units narrow which units it alerts for; empty: every unit.
	Disciplines []string `json:"disciplines,omitempty"`
	Units       []string `json:"units,omitempty"`
	Color       string   `json:"color,omitempty"`
	Off         bool     `json:"off,omitempty"`
}

// FenceAlert is a unit entering or leaving an alert area.
type FenceAlert struct {
	At    time.Time `json:"at"`
	Unit  string    `json:"unit"`
	Fence string    `json:"fence"`
	Name  string    `json:"name"`
	Enter bool      `json:"enter"`
	Seq   uint64    `json:"seq"`
}

// keepFenceAlerts is how many area alerts the live state keeps.
const keepFenceAlerts = 300

// SaveGeofence adds or changes an area.
func (e *Engine) SaveGeofence(a Actor, g Geofence) (string, error) {
	g.Name = strings.TrimSpace(g.Name)
	switch {
	case g.Name == "":
		return "", problem("name", "give the area a name")
	case g.Kind != FenceAlerting && g.Kind != FenceCaution:
		return "", problem("kind", "an area is an alert area or a caution area")
	case len(g.Ring) < 3:
		return "", problem("ring", "draw the area: at least three corners")
	case len(g.Ring) > 500:
		return "", problem("ring", "up to 500 corners")
	case g.Kind == FenceAlerting && !g.Enter && !g.Leave:
		return "", problem("enter", "say whether to alert when units enter, leave, or both")
	case g.Kind == FenceCaution && strings.TrimSpace(g.Note) == "":
		return "", problem("note", "say what dispatchers and crews should know in this area")
	}
	for i, p := range g.Ring {
		if p[0] < -180 || p[0] > 180 || p[1] < -90 || p[1] > 90 {
			return "", problem("ring", "corner %d is not a position", i+1)
		}
	}
	if n := len(g.Ring); n > 3 && g.Ring[0] == g.Ring[n-1] {
		g.Ring = g.Ring[:n-1] // stored open
	}
	g.Units = dedupe(g.Units)
	e.mu.Lock()
	defer e.mu.Unlock()
	if g.ID == "" {
		g.ID = e.newID("GF", func(id string) bool { return e.st.Geofences[id] != nil })
	} else if e.st.Geofences[g.ID] == nil {
		return "", problem("id", "no area %s", g.ID)
	}
	_, err := e.write(a, EvGeofenceSaved, refs{}, g)
	return g.ID, err
}

// RemoveGeofence removes an area.
func (e *Engine) RemoveGeofence(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.Geofences[id] == nil {
		return problem("id", "no area %s", id)
	}
	_, err := e.write(a, EvGeofenceRemoved, refs{}, idData{id})
	return err
}

// Contains reports whether a point is in the area.
func (g *Geofence) Contains(lat, lon float64) bool { return geo.InPolygon(lat, lon, g.Ring) }

func (g *Geofence) watches(s *State, u *Unit) bool {
	if len(g.Units) > 0 && !slices.Contains(g.Units, u.ID) {
		return false
	}
	if len(g.Disciplines) > 0 && !slices.Contains(g.Disciplines, s.Discipline(u)) {
		return false
	}
	return true
}

// CautionsAt are the caution areas a point is in.
func (s *State) CautionsAt(lat, lon float64) []*Geofence {
	var out []*Geofence
	for _, g := range s.Geofences {
		if !g.Off && g.Kind == FenceCaution && g.Contains(lat, lon) {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// InFences are the areas a unit is in now, by ID.
func (s *State) InFences(unit string) []string { return s.inFence[unit] }

func (s *State) applyGeofences(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvGeofenceSaved:
		g, err := decode[Geofence](e)
		if err != nil {
			return true, err
		}
		s.Geofences[g.ID] = &g
		s.refence(g.ID) // where units are now, without alerts: an area drawn around a unit is not an arrival
	case EvGeofenceRemoved:
		v, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		delete(s.Geofences, v.ID)
		for u, ids := range s.inFence {
			s.inFence[u] = slices.DeleteFunc(ids, func(id string) bool { return id == v.ID })
		}
	default:
		return false, nil
	}
	return true, nil
}

func (s *State) refence(id string) {
	g := s.Geofences[id]
	for _, u := range s.Units {
		ids := slices.DeleteFunc(s.inFence[u.ID], func(x string) bool { return x == id })
		if g != nil && !g.Off && u.Position != nil && g.Contains(u.Position.Lat, u.Position.Lon) {
			ids = append(ids, id)
		}
		s.inFence[u.ID] = ids
	}
}

// fenceAfter works out, after a unit's position, which areas it entered or left.
func (s *State) fenceAfter(e ledger.Event) {
	if e.Type != EvUnitLocation || len(s.Geofences) == 0 {
		return
	}
	u := s.Units[e.Unit]
	if u == nil || u.Position == nil {
		return
	}
	was := s.inFence[u.ID]
	var now []string
	for _, g := range s.Geofences {
		if !g.Off && g.Contains(u.Position.Lat, u.Position.Lon) {
			now = append(now, g.ID)
		}
	}
	slices.Sort(now)
	for _, id := range now {
		if !slices.Contains(was, id) {
			s.fenceAlert(e, u, id, true)
		}
	}
	for _, id := range was {
		if !slices.Contains(now, id) {
			s.fenceAlert(e, u, id, false)
		}
	}
	s.inFence[u.ID] = now
}

func (s *State) fenceAlert(e ledger.Event, u *Unit, id string, enter bool) {
	g := s.Geofences[id]
	if g == nil || g.Kind != FenceAlerting || (enter && !g.Enter) || (!enter && !g.Leave) || !g.watches(s, u) {
		return
	}
	s.FenceAlerts = append(s.FenceAlerts, FenceAlert{At: e.At, Unit: u.ID, Fence: id, Name: g.Name, Enter: enter, Seq: e.Seq})
	if len(s.FenceAlerts) > keepFenceAlerts {
		s.FenceAlerts = slices.Clone(s.FenceAlerts[len(s.FenceAlerts)-keepFenceAlerts:])
	}
	if in := s.Incidents[u.Incident]; in != nil && u.Incident != "" {
		verb := "entered"
		if !enter {
			verb = "left"
		}
		in.Timeline = append(in.Timeline, Entry{Seq: e.Seq, At: e.At, Actor: "system", Kind: "area", Text: fmt.Sprintf("%s %s %s", u.ID, verb, g.Name), Unit: u.ID})
	}
}
