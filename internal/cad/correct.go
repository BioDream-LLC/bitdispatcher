package cad

// Corrections: a closed incident's record is put right without reopening it (a wrong call type, the
// disposition, the address, a unit's times given late by radio). Each change says what it was and what it
// became, with who and why, and stays on the timeline; reopening is for when the work goes on.

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

const EvIncCorrected = "incident.corrected"

// Correction is what to change on a closed incident; only what is given changes.
type Correction struct {
	Type        string                          `json:"type,omitempty"`
	Priority    int                             `json:"priority,omitempty"`
	Disposition string                          `json:"disposition,omitempty"`
	Address     string                          `json:"address,omitempty"`
	Lat         *float64                        `json:"lat,omitempty"`
	Lon         *float64                        `json:"lon,omitempty"`
	UnitTimes   map[string]map[string]time.Time `json:"unitTimes,omitempty"` // unit → dispatched|enroute|onScene|transport|atDest|cleared → time
	Reason      string                          `json:"reason"`
}

// Change is one field changed.
type Change struct {
	Field string `json:"field"` // type, priority, disposition, address, lat, lon, unit:<id>:<time>
	From  string `json:"from"`
	To    string `json:"to"`
}

var unitTimeFields = []string{"dispatched", "enroute", "onScene", "transport", "atDest", "cleared"}

func unitTime(as *Assignment, f string) *time.Time {
	switch f {
	case "dispatched":
		return &as.Dispatched
	case "enroute":
		return &as.Enroute
	case "onScene":
		return &as.OnScene
	case "transport":
		return &as.Transport
	case "atDest":
		return &as.AtDest
	case "cleared":
		return &as.Cleared
	}
	return nil
}

func tstr(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// CorrectIncident changes a closed incident's record. It returns the changes made.
func (e *Engine) CorrectIncident(a Actor, number string, c Correction) ([]Change, error) {
	c.Reason = strings.TrimSpace(c.Reason)
	if c.Reason == "" {
		return nil, problem("reason", "say why the record is corrected")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	in, ok := e.st.Incidents[number]
	if !ok {
		return nil, problem("incident", "no incident %s", number)
	}
	if in.Status != Closed {
		return nil, problem("incident", "%s is open: change it on the incident", number)
	}
	var ch []Change
	add := func(f, from, to string) {
		if from != to {
			ch = append(ch, Change{Field: f, From: from, To: to})
		}
	}
	if c.Type != "" {
		if e.st.CallTypes[c.Type] == nil {
			return nil, problem("type", "no call type %s", c.Type)
		}
		add("type", in.Type, c.Type)
	}
	if c.Priority != 0 {
		if c.Priority < 1 || c.Priority > 5 {
			return nil, problem("priority", "priority is 1 to 5")
		}
		add("priority", strconv.Itoa(in.Priority), strconv.Itoa(c.Priority))
	}
	if c.Disposition != "" {
		if !slices.Contains(e.st.Settings.Dispositions, c.Disposition) {
			return nil, problem("disposition", "no disposition %q", c.Disposition)
		}
		add("disposition", in.Disposition, c.Disposition)
	}
	loc := in.Location
	if loc == nil {
		loc = &ng911.Location{}
	}
	if a := strings.TrimSpace(c.Address); a != "" {
		add("address", loc.Address, a)
	}
	if (c.Lat == nil) != (c.Lon == nil) {
		return nil, problem("lat", "give both the latitude and the longitude")
	}
	if c.Lat != nil {
		if *c.Lat < -90 || *c.Lat > 90 || *c.Lon < -180 || *c.Lon > 180 {
			return nil, problem("lat", "that is not a place on Earth")
		}
		f := func(p *float64) string {
			if p == nil {
				return ""
			}
			return strconv.FormatFloat(*p, 'f', 6, 64)
		}
		add("lat", f(loc.Lat), f(c.Lat))
		add("lon", f(loc.Lon), f(c.Lon))
	}
	for unit, times := range c.UnitTimes {
		i := slices.IndexFunc(in.Units, func(as Assignment) bool { return as.Unit == unit })
		if i < 0 {
			return nil, problem("unitTimes", "%s was not on %s", unit, number)
		}
		as := in.Units[i]
		for _, f := range unitTimeFields {
			t, ok := times[f]
			if !ok {
				continue
			}
			if !t.IsZero() && (t.Before(in.Created.Add(-time.Hour)) || t.After(in.Closed.Add(24*time.Hour))) {
				return nil, problem("unitTimes", "%s %s is outside the incident's time", unit, f)
			}
			add("unit:"+unit+":"+f, tstr(*unitTime(&as, f)), tstr(t))
		}
		for f := range times {
			if !slices.Contains(unitTimeFields, f) {
				return nil, problem("unitTimes", "%s is not a unit time", f)
			}
		}
	}
	if len(ch) == 0 {
		return nil, problem("incident", "nothing to change")
	}
	slices.SortStableFunc(ch, func(x, y Change) int { return strings.Compare(x.Field, y.Field) })
	_, err := e.write(a, EvIncCorrected, refs{Incident: number}, map[string]any{"changes": ch, "reason": c.Reason})
	return ch, err
}

func (s *State) applyCorrection(e ledger.Event) (bool, error) {
	if e.Type != EvIncCorrected {
		return false, nil
	}
	v, err := decode[struct {
		Changes []Change
		Reason  string
	}](e)
	if err != nil {
		return true, err
	}
	in, ok := s.Incidents[e.Incident]
	if !ok {
		return true, nil
	}
	var parts []string
	for _, c := range v.Changes {
		switch {
		case c.Field == "type":
			in.Type = c.To
		case c.Field == "priority":
			in.Priority, _ = strconv.Atoi(c.To)
		case c.Field == "disposition":
			in.Disposition = c.To
		case c.Field == "address" || c.Field == "lat" || c.Field == "lon":
			if in.Location == nil {
				in.Location = &ng911.Location{}
			} else {
				l := *in.Location
				in.Location = &l
			}
			switch c.Field {
			case "address":
				in.Location.Address = c.To
			case "lat", "lon":
				f, err := strconv.ParseFloat(c.To, 64)
				if err == nil {
					if c.Field == "lat" {
						in.Location.Lat = &f
					} else {
						in.Location.Lon = &f
					}
				}
			}
		case strings.HasPrefix(c.Field, "unit:"):
			p := strings.SplitN(c.Field, ":", 3)
			if len(p) == 3 {
				for i := range in.Units {
					if in.Units[i].Unit == p[1] {
						if tp := unitTime(&in.Units[i], p[2]); tp != nil {
							t, _ := time.Parse(time.RFC3339, c.To)
							*tp = t
						}
					}
				}
			}
		}
		from, to := c.From, c.To
		if from == "" {
			from = "none"
		}
		if to == "" {
			to = "none"
		}
		parts = append(parts, fmt.Sprintf("%s %s → %s", strings.ReplaceAll(strings.TrimPrefix(c.Field, "unit:"), ":", " "), from, to))
	}
	in.add(e, "correction", "record corrected: "+strings.Join(parts, "; ")+" ("+v.Reason+")")
	return true, nil
}
