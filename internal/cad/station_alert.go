package cad

// Station alerting: when units are dispatched, their stations are toned out. A station's screen
// (bitdispatcher's own, in the firehouse) sounds the tones and reads the call aloud; an alerting
// gateway (a vendor's station controller) gets the same alert at its address. Each alert is a ledger
// event, and so is the station's acknowledgement.

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvStationAlert = "station.alert" // a station was toned out for units on an incident
	EvStationAck   = "station.ack"   // someone at the station acknowledged the alert
)

// StationAlert is one tone-out.
type StationAlert struct {
	ID       string    `json:"id"`
	At       time.Time `json:"at"`
	Station  string    `json:"station"`
	Units    []string  `json:"units"`
	Incident string    `json:"incident"`
	Type     string    `json:"type"`
	TypeName string    `json:"typeName"`
	Priority int       `json:"priority"`
	Address  string    `json:"address,omitempty"`
	Lat      *float64  `json:"lat,omitempty"`
	Lon      *float64  `json:"lon,omitempty"`
	Speech   string    `json:"speech"`          // what the station's voice says
	Again    bool      `json:"again,omitempty"` // a repeat, by a dispatcher
	Acked    time.Time `json:"acked,omitzero"`
	AckedBy  string    `json:"ackedBy,omitempty"`
}

const keepStationAlerts = 200

// AlertStations tones out the stations of the units just dispatched to an incident.
func (e *Engine) AlertStations(a Actor, number string, units []string, again bool) ([]string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	in, ok := e.st.Incidents[number]
	if !ok {
		return nil, problem("incident", "no incident %s", number)
	}
	byStation := map[string][]string{}
	var order []string
	for _, id := range units {
		u := e.st.Units[strings.ToUpper(id)]
		if u == nil {
			continue
		}
		st := firstNonEmpty(u.Covering, u.Station)
		if st == "" || e.st.Stations[st] == nil {
			continue
		}
		if _, seen := byStation[st]; !seen {
			order = append(order, st)
		}
		byStation[st] = append(byStation[st], u.ID)
	}
	var ids []string
	for _, st := range order {
		us := byStation[st]
		n := len(e.st.StationAlerts) + e.st.alertBase + 1
		al := StationAlert{ID: fmt.Sprintf("SA%s%d", e.sitePrefix, n), Station: st, Units: us, Incident: number, Type: in.Type, Priority: in.Priority, Again: again}
		if ct := e.st.CallTypes[in.Type]; ct != nil {
			al.TypeName = ct.Name
		}
		if l := in.Location; l != nil {
			al.Address, al.Lat, al.Lon = l.Address, l.Lat, l.Lon
		}
		al.Speech = e.st.speech(al)
		if _, err := e.write(a, EvStationAlert, refs{Incident: number}, al); err != nil {
			return ids, err
		}
		ids = append(ids, al.ID)
	}
	return ids, nil
}

// AckStationAlert records that the station heard the alert.
func (e *Engine) AckStationAlert(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	al := e.st.stationAlert(id)
	if al == nil {
		return problem("id", "no station alert %s", id)
	}
	if !al.Acked.IsZero() {
		return nil
	}
	_, err := e.write(a, EvStationAck, refs{Incident: al.Incident}, map[string]string{"id": id})
	return err
}

func (s *State) stationAlert(id string) *StationAlert {
	for i := len(s.StationAlerts) - 1; i >= 0; i-- {
		if s.StationAlerts[i].ID == id {
			return s.StationAlerts[i]
		}
	}
	return nil
}

func (s *State) applyStation(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvStationAlert:
		al, err := decode[StationAlert](e)
		if err != nil {
			return true, err
		}
		al.At = e.At
		s.StationAlerts = append(s.StationAlerts, &al)
		if len(s.StationAlerts) > keepStationAlerts {
			drop := len(s.StationAlerts) - keepStationAlerts
			s.alertBase += drop
			s.StationAlerts = slices.Clone(s.StationAlerts[drop:])
		}
		if in, ok := s.Incidents[al.Incident]; ok {
			word := "toned out"
			if al.Again {
				word = "toned out again"
			}
			in.add(e, "station", fmt.Sprintf("%s %s for %s", al.Station, word, strings.Join(al.Units, ", ")))
		}
		return true, nil
	case EvStationAck:
		v, _ := decode[map[string]string](e)
		if al := s.stationAlert(v["id"]); al != nil && al.Acked.IsZero() {
			al.Acked, al.AckedBy = e.At, e.Actor
			if in, ok := s.Incidents[al.Incident]; ok {
				in.add(e, "station", al.Station+" acknowledged the alert")
			}
		}
		return true, nil
	}
	return false, nil
}

// unitWords are how unit types are spoken.
var unitWords = map[string]string{"E": "Engine", "L": "Ladder", "T": "Truck", "M": "Medic", "R": "Rescue", "BC": "Battalion", "BR": "Brush", "TK": "Tanker", "SQ": "Squad", "A": "Ambulance", "EMS": "E M S"}

var reUnit = regexp.MustCompile(`^([A-Z]+)(\d+)$`)

func spokenUnit(id string) string {
	if m := reUnit.FindStringSubmatch(id); m != nil {
		if w, ok := unitWords[m[1]]; ok {
			return w + " " + m[2]
		}
	}
	return strings.Join(strings.Split(id, ""), " ")
}

// streetWords expand what addresses abbreviate, so the voice says "Avenue Southwest", not "ave S W".
var streetWords = map[string]string{"ST": "Street", "AVE": "Avenue", "AV": "Avenue", "RD": "Road", "DR": "Drive", "LN": "Lane", "BLVD": "Boulevard", "CT": "Court",
	"PL": "Place", "PKWY": "Parkway", "HWY": "Highway", "CIR": "Circle", "TER": "Terrace", "TRL": "Trail", "WAY": "Way", "SQ": "Square",
	"N": "North", "S": "South", "E": "East", "W": "West", "NE": "Northeast", "NW": "Northwest", "SE": "Southeast", "SW": "Southwest",
	"APT": "Apartment", "STE": "Suite", "#": "number"}

// SpokenAddress is an address as it should be read aloud: the street part only, abbreviations expanded.
func SpokenAddress(a string) string {
	first := strings.TrimSpace(strings.SplitN(a, ",", 2)[0])
	var out []string
	for _, w := range strings.Fields(first) {
		k := strings.ToUpper(strings.Trim(w, "."))
		if x, ok := streetWords[k]; ok && len(out) > 0 {
			out = append(out, x)
			continue
		}
		out = append(out, w)
	}
	return strings.Join(out, " ")
}

func (s *State) speech(al StationAlert) string {
	var us []string
	for _, u := range al.Units {
		us = append(us, spokenUnit(u))
	}
	var b strings.Builder
	b.WriteString(strings.Join(us, ", "))
	b.WriteString(". ")
	b.WriteString(firstNonEmpty(al.TypeName, al.Type))
	if al.Priority == 1 {
		b.WriteString(", priority one")
	}
	if al.Address != "" {
		b.WriteString(". " + SpokenAddress(al.Address))
	}
	b.WriteString(".")
	return b.String()
}
