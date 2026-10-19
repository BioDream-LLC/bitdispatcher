package cad

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Move-up and cover: when a station has none of a kind of unit it normally houses available (all its
// engines are out, say), bitdispatcher suggests moving an available unit of that kind from a station that
// can spare one, nearest first by road. The dispatcher moves it with one click (or MU E4 ST1); the unit
// stays available and is posted at the station it covers until it is sent home (RET E4).

// Gap is a kind of unit a station normally has and has none of available now.
type Gap struct {
	Station     string    `json:"station"`
	StationName string    `json:"stationName"`
	Type        string    `json:"type"`
	TypeName    string    `json:"typeName"`
	Discipline  string    `json:"discipline"`
	Homed       []string  `json:"homed"` // the units based there, all busy
	Since       time.Time `json:"since,omitzero"`
	Move        *Move     `json:"move,omitempty"` // the suggested move-up
	Others      []Move    `json:"others"`         // the next choices
}

// Move is a suggested move-up.
type Move struct {
	Unit     string  `json:"unit"`
	From     string  `json:"from"` // the station it is posted at now
	FromName string  `json:"fromName"`
	To       string  `json:"to"`
	Metres   float64 `json:"metres,omitempty"`
	Seconds  float64 `json:"seconds,omitempty"`
	Routed   bool    `json:"routed,omitempty"`
	Left     int     `json:"left"`            // available units of the type left at From after the move
	Opens    bool    `json:"opens,omitempty"` // the move leaves From with none of the type
	Reason   string  `json:"reason"`
}

// Return is a covering unit that can go home: its own station has units of its kind again, or the station
// it covers does.
type Return struct {
	Unit     string `json:"unit"`
	Covering string `json:"covering"`
	Home     string `json:"home"`
	Reason   string `json:"reason"`
}

// Coverage is the move-up picture.
type Coverage struct {
	Gaps      []Gap     `json:"gaps"`
	Returns   []Return  `json:"returns"`
	Estimate  string    `json:"estimate"`
	Generated time.Time `json:"generated"`
}

// post is where a unit is based now: the station it covers, or its own.
func (u *Unit) post() string {
	if u.Covering != "" {
		return u.Covering
	}
	return u.Station
}

// availableAt counts available units of a type posted at a station.
func (s *State) availableAt(station, typ string) []string {
	var out []string
	for _, u := range s.Units {
		if u.Type == typ && u.Status == Available && u.post() == station {
			out = append(out, u.ID)
		}
	}
	sort.Strings(out)
	return out
}

// gaps lists the stations and types with nothing available, before any travel times.
func (s *State) gaps() []Gap {
	covered := map[string]bool{}
	for _, t := range s.Settings.UnitTypes {
		covered[t.Code] = t.Covered()
	}
	homed := map[[2]string][]string{}
	for _, u := range s.Units {
		if u.Station != "" && u.Status != OutOfService && covered[u.Type] {
			k := [2]string{u.Station, u.Type}
			homed[k] = append(homed[k], u.ID)
		}
	}
	var out []Gap
	for k, ids := range homed {
		if len(s.availableAt(k[0], k[1])) > 0 {
			continue
		}
		st, ok := s.Stations[k[0]]
		if !ok {
			continue
		}
		sort.Strings(ids)
		g := Gap{Station: st.ID, StationName: st.Name, Type: k[1], TypeName: k[1], Homed: ids, Others: []Move{}}
		for _, t := range s.Settings.UnitTypes {
			if t.Code == k[1] {
				g.TypeName, g.Discipline = t.Name, t.Discipline
			}
		}
		// Since the last of them left.
		for _, id := range ids {
			if u := s.Units[id]; u.StatusAt.After(g.Since) {
				g.Since = u.StatusAt
			}
		}
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Since != out[j].Since {
			return out[i].Since.Before(out[j].Since)
		}
		return out[i].Station+out[i].Type < out[j].Station+out[j].Type
	})
	return out
}

// Coverage works out the gaps and the move-ups that would fill them.
func (e *Engine) Coverage() Coverage {
	e.mu.Lock()
	gaps := e.st.gaps()
	r := e.router
	e.mu.Unlock()
	// Drive times to each gap's station, found without the lock.
	tfs := map[string]TravelFunc{}
	label := ""
	if r != nil {
		for _, g := range gaps {
			if _, ok := tfs[g.Station]; ok {
				continue
			}
			e.mu.Lock()
			st := e.st.Stations[g.Station]
			e.mu.Unlock()
			if st != nil {
				tfs[g.Station], label = r.TravelTo(st.Lat, st.Lon)
			}
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.st.coverage(gaps, tfs, label, e.now())
}

func (s *State) coverage(gaps []Gap, tfs map[string]TravelFunc, label string, now time.Time) Coverage {
	c := Coverage{Gaps: []Gap{}, Returns: []Return{}, Generated: now,
		Estimate: fmt.Sprintf("Straight line × %.1f at %.0f km/h; no road network loaded", s.Settings.RoadFactor, s.Settings.SpeedKmh)}
	if label != "" {
		c.Estimate = label
	}
	taken := map[string]bool{}
	for _, g := range gaps {
		to := s.Stations[g.Station]
		var moves []Move
		for _, u := range s.Units {
			if u.Type != g.Type || u.Status != Available || taken[u.ID] {
				continue
			}
			from := u.post()
			left := len(s.availableAt(from, g.Type)) - 1
			m := Move{Unit: u.ID, From: from, To: g.Station, Left: left}
			// A move that opens a new gap is offered only after the ones that do not.
			m.Opens = from != "" && left < 1 && s.homesType(from, g.Type)
			if st, ok := s.Stations[from]; ok {
				m.FromName = st.Name
			}
			lat, lon, ok := s.unitPoint(u, now)
			if ok {
				if tf := tfs[g.Station]; tf != nil {
					if metres, secs, _, ok := tf(lat, lon); ok {
						m.Metres, m.Seconds, m.Routed = metres, secs, true
					}
				}
				if !m.Routed {
					d := distanceLL(lat, lon, to.Lat, to.Lon) * s.Settings.RoadFactor
					m.Metres, m.Seconds = d, d/(s.Settings.SpeedKmh/3.6)
				}
			}
			m.Reason = fmt.Sprintf("%s leaves %d %s at %s", u.ID, max(left, 0), strings.ToLower(g.TypeName), firstNonEmpty(m.FromName, from, "no station"))
			if m.Opens {
				m.Reason = fmt.Sprintf("%s leaves %s with no %s", u.ID, firstNonEmpty(m.FromName, from), strings.ToLower(g.TypeName))
			}
			moves = append(moves, m)
		}
		sort.SliceStable(moves, func(i, j int) bool {
			a, b := moves[i], moves[j]
			if a.Opens != b.Opens {
				return !a.Opens
			}
			if (a.Seconds == 0) != (b.Seconds == 0) {
				return a.Seconds != 0
			}
			if a.Seconds != b.Seconds {
				return a.Seconds < b.Seconds
			}
			return a.Unit < b.Unit
		})
		if len(moves) > 0 {
			best := moves[0]
			g.Move = &best
			taken[best.Unit] = true
			for _, m := range moves[1:] {
				if len(g.Others) < 3 {
					g.Others = append(g.Others, m)
				}
			}
		}
		c.Gaps = append(c.Gaps, g)
	}
	// Covering units that can go home.
	ids := make([]string, 0, len(s.Units))
	for id := range s.Units {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		u := s.Units[id]
		if u.Covering == "" || u.Status != Available {
			continue
		}
		own := 0
		for _, o := range s.Units {
			if o.ID != u.ID && o.Type == u.Type && o.Status == Available && o.Station == u.Covering && o.Covering == "" {
				own++
			}
		}
		homeEmpty := u.Station != "" && len(s.availableAt(u.Station, u.Type)) == 0
		switch {
		case own > 0:
			c.Returns = append(c.Returns, Return{Unit: u.ID, Covering: u.Covering, Home: u.Station, Reason: fmt.Sprintf("%s has its own %s available again", s.stationName(u.Covering), u.Type)})
		case homeEmpty:
			c.Returns = append(c.Returns, Return{Unit: u.ID, Covering: u.Covering, Home: u.Station, Reason: fmt.Sprintf("%s has no %s available now", s.stationName(u.Station), u.Type)})
		}
	}
	return c
}

func (s *State) homesType(station, typ string) bool {
	for _, u := range s.Units {
		if u.Station == station && u.Type == typ {
			return true
		}
	}
	return false
}

func (s *State) stationName(id string) string {
	if st, ok := s.Stations[id]; ok {
		return st.Name
	}
	return id
}

// unitPoint is where a unit is for travel times: fresh AVL, else its post.
func (s *State) unitPoint(u *Unit, now time.Time) (float64, float64, bool) {
	stale := time.Duration(s.Settings.AVLStaleSeconds) * time.Second
	if u.Position != nil && now.Sub(u.Position.At) <= stale {
		return u.Position.Lat, u.Position.Lon, true
	}
	if st, ok := s.Stations[u.post()]; ok {
		return st.Lat, st.Lon, true
	}
	if u.Position != nil {
		return u.Position.Lat, u.Position.Lon, true
	}
	return 0, 0, false
}

type coverData struct {
	Station string `json:"station"` // empty: back to its own station
	Prev    string `json:"prev,omitempty"`
}

// Cover posts a unit at another station (a move-up); station "" sends it home.
func (e *Engine) Cover(a Actor, unit, station string) error {
	unit, station = strings.ToUpper(unit), strings.ToUpper(station)
	e.mu.Lock()
	defer e.mu.Unlock()
	u, ok := e.st.Units[unit]
	if !ok {
		return problem("unit", "no unit %s", unit)
	}
	if station != "" {
		if _, ok := e.st.Stations[station]; !ok {
			return problem("station", "no station %s", station)
		}
		if u.Status == OutOfService {
			return problem("unit", "%s is out of service", unit)
		}
	}
	if station == u.Station {
		station = ""
	}
	if station == u.Covering {
		return nil
	}
	_, err := e.write(a, EvUnitCover, refs{Unit: unit}, coverData{Station: station, Prev: u.Covering})
	return err
}

func (s *State) applyCover(unit string, v coverData) {
	if u, ok := s.Units[unit]; ok {
		u.Covering = v.Station
	}
}

func distanceLL(lat1, lon1, lat2, lon2 float64) float64 {
	return distance(Position{Lat: lat1, Lon: lon1}, Position{Lat: lat2, Lon: lon2})
}
