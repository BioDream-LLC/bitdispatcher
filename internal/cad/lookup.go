package cad

// Looking things up for call entry: earlier incidents linked by address, phone, person and vehicle; a
// search across the record; and possible duplicates before a second incident is made.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/geo"
)

// PrevQuery is what call entry knows so far.
type PrevQuery struct {
	Number  string   // the incident being entered (left out of the answer)
	Address string   // as typed or verified
	Lat     *float64 // the incident's position
	Lon     *float64
	Phones  []string // the callback and any person's phone
	Names   []string // people's names as entered
	Plates  []string
	VINs    []string
	Since   time.Time // how far back (zero: two years)
}

// PrevMatch is one earlier incident (or earlier-system record) linked to the one being entered.
type PrevMatch struct {
	Incident   string    `json:"incident"`
	Type       string    `json:"type"`
	TypeName   string    `json:"typeName"`
	At         time.Time `json:"at"`
	Address    string    `json:"address,omitempty"`
	Status     string    `json:"status,omitempty"`
	Disp       string    `json:"disposition,omitempty"`
	By         []string  `json:"by"`         // address, phone, person, vehicle
	Why        []string  `json:"why"`        // "same address", "plate VA KTR4471", "name DOE, JOHN"
	Confidence string    `json:"confidence"` // confirmed (an exact identifier) or possible (near, or a similar name)
	Source     string    `json:"source"`     // bitdispatcher, or the earlier system
	Safety     bool      `json:"safety,omitempty"`
}

func norm(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '.' || r == ' ' || r == '#' }), " ")
}

// nameKey makes "DOE, JOHN Q" and "John Doe" alike: the words, sorted, without initials.
func nameKey(s string) string {
	var ws []string
	for _, w := range strings.Fields(norm(s)) {
		if len(w) > 1 {
			ws = append(ws, w)
		}
	}
	sort.Strings(ws)
	return strings.Join(ws, " ")
}

// Previous finds earlier incidents linked to the one being entered.
func (s *State) Previous(q PrevQuery, now time.Time) []PrevMatch {
	since := q.Since
	if since.IsZero() {
		since = now.AddDate(-2, 0, 0)
	}
	addr := norm(q.Address)
	phones, names, plates, vins := map[string]bool{}, map[string]string{}, map[string]bool{}, map[string]bool{}
	for _, p := range q.Phones {
		if d := digits(p); len(d) >= 7 {
			phones[d] = true
		}
	}
	for _, n := range q.Names {
		if k := nameKey(n); k != "" {
			names[k] = n
		}
	}
	for _, p := range q.Plates {
		if p = strings.ReplaceAll(norm(p), " ", ""); p != "" {
			plates[p] = true
		}
	}
	for _, v := range q.VINs {
		if v = norm(v); len(v) >= 11 {
			vins[v] = true
		}
	}
	byInc := map[string]*PrevMatch{}
	add := func(in *Incident, by, why string, confirmed bool) {
		m, ok := byInc[in.Number]
		if !ok {
			m = &PrevMatch{Incident: in.Number, Type: in.Type, TypeName: s.typeLabel(in.Type), At: in.Created, Status: in.Status, Disp: in.Disposition,
				Confidence: "possible", Source: "bitdispatcher", By: []string{}, Why: []string{}}
			if in.Location != nil {
				m.Address = in.Location.Address
			}
			byInc[in.Number] = m
		}
		if !containsStr(m.By, by) {
			m.By = append(m.By, by)
		}
		if !containsStr(m.Why, why) {
			m.Why = append(m.Why, why)
		}
		if confirmed {
			m.Confidence = "confirmed"
		}
	}
	for _, in := range s.Incidents {
		if in.Number == q.Number || in.Created.Before(since) {
			continue
		}
		if in.Location != nil {
			if addr != "" && norm(in.Location.Address) == addr {
				add(in, "address", "same address", true)
			} else if q.Lat != nil && q.Lon != nil && in.Location.HasPoint() {
				if d := geo.Distance(*q.Lat, *q.Lon, *in.Location.Lat, *in.Location.Lon); d <= 40 {
					add(in, "address", fmt.Sprintf("%.0f m away", d), false)
				}
			}
		}
		if in.Caller != nil && phones[digits(in.Caller.Callback)] {
			add(in, "phone", "same callback "+in.Caller.Callback, true)
		}
		for _, id := range in.Calls {
			if c, ok := s.Calls[id]; ok && phones[digits(c.Callback)] {
				add(in, "phone", "same callback "+c.Callback, true)
			}
		}
		for _, p := range in.Persons {
			if p.Removed {
				continue
			}
			if phones[digits(p.Phone)] && digits(p.Phone) != "" {
				add(in, "phone", "a person's phone "+p.Phone, true)
			}
			if k := nameKey(p.Name); k != "" {
				if _, ok := names[k]; ok {
					// A name alone is only a possible match: many people share one.
					add(in, "person", "name "+p.Name+" ("+p.Role+")", false)
					if p.Safety {
						byInc[in.Number].Safety = true
						add(in, "person", "officer-safety note on "+p.Name, false)
					}
				}
			}
		}
		for _, v := range in.Vehicles {
			if v.Removed {
				continue
			}
			if v.Plate != "" && plates[v.Plate] {
				add(in, "vehicle", "plate "+strings.TrimSpace(v.State+" "+v.Plate), true)
			}
			if v.VIN != "" && vins[v.VIN] {
				add(in, "vehicle", "VIN "+v.VIN, true)
			}
		}
	}
	out := make([]PrevMatch, 0, len(byInc))
	for _, m := range byInc {
		out = append(out, *m)
	}
	// The earlier system's incidents at the place.
	if q.Lat != nil && q.Lon != nil {
		for _, h := range s.HistoryNear(*q.Lat, *q.Lon, 40, since) {
			out = append(out, PrevMatch{Incident: h.Number, Type: h.Type, TypeName: firstNonEmpty(s.typeLabel(h.Type), h.Type), At: h.At, Address: h.Address,
				Disp: h.Disposition, By: []string{"address"}, Why: []string{firstNonEmpty(h.Note, "at this address")}, Confidence: "possible", Source: firstNonEmpty(h.Source, "earlier system")})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].Confidence == "confirmed") != (out[j].Confidence == "confirmed") {
			return out[i].Confidence == "confirmed"
		}
		return out[i].At.After(out[j].At)
	})
	if len(out) > 50 {
		out = out[:50]
	}
	return out
}

func containsStr(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// Hit is one search result.
type Hit struct {
	Kind   string    `json:"kind"` // incident, call, person, vehicle, premise, bolo, history
	Ref    string    `json:"ref"`  // the incident number, call ID, premise ID or BOLO ID
	Title  string    `json:"title"`
	Detail string    `json:"detail,omitempty"`
	At     time.Time `json:"at,omitzero"`
	Match  string    `json:"match"`  // exact, partial
	Source string    `json:"source"` // bitdispatcher, or the earlier system
}

// Search looks for text across incidents (number, address, notes), calls (callback), people, vehicles
// (plate, VIN), premises and BOLOs. Exact identifier matches come first.
func (s *State) Search(text string, limit int) []Hit {
	q := norm(text)
	if len(q) < 2 {
		return []Hit{}
	}
	qd := digits(text)
	qk := nameKey(text)
	var out []Hit
	add := func(h Hit) { out = append(out, h) }
	has := func(field string) string {
		f := norm(field)
		switch {
		case f == "":
			return ""
		case f == q:
			return "exact"
		case strings.Contains(f, q):
			return "partial"
		}
		return ""
	}
	for _, in := range s.Incidents {
		title := in.Number + " " + s.typeLabel(in.Type)
		addr := ""
		if in.Location != nil {
			addr = in.Location.Address
		}
		if m := has(in.Number); m != "" {
			add(Hit{Kind: "incident", Ref: in.Number, Title: title, Detail: addr, At: in.Created, Match: m, Source: "bitdispatcher"})
			continue
		}
		if m := has(addr); m != "" {
			add(Hit{Kind: "incident", Ref: in.Number, Title: title, Detail: addr, At: in.Created, Match: m, Source: "bitdispatcher"})
		}
		if in.Caller != nil && len(qd) >= 7 && strings.Contains(digits(in.Caller.Callback), qd) {
			add(Hit{Kind: "call", Ref: in.Number, Title: "Callback " + in.Caller.Callback, Detail: title, At: in.Created, Match: "exact", Source: "bitdispatcher"})
		}
		for _, p := range in.Persons {
			if p.Removed {
				continue
			}
			if (qk != "" && nameKey(p.Name) == qk) || has(p.Name) != "" || (len(qd) >= 7 && digits(p.Phone) == qd) || has(p.IDNumber) == "exact" {
				m := "partial"
				if nameKey(p.Name) == qk || digits(p.Phone) == qd || has(p.IDNumber) == "exact" {
					m = "exact"
				}
				add(Hit{Kind: "person", Ref: in.Number, Title: p.Name + " (" + p.Role + ")", Detail: strings.TrimSpace(p.DOB + " " + title), At: p.Added, Match: m, Source: "bitdispatcher"})
			}
		}
		for _, v := range in.Vehicles {
			if v.Removed {
				continue
			}
			plate := strings.ReplaceAll(q, " ", "")
			if (v.Plate != "" && strings.Contains(v.Plate, plate)) || (v.VIN != "" && strings.Contains(v.VIN, plate)) {
				m := "partial"
				if v.Plate == plate || v.VIN == plate {
					m = "exact"
				}
				add(Hit{Kind: "vehicle", Ref: in.Number, Title: strings.TrimSpace(v.State + " " + v.Plate + " " + v.Color + " " + v.Make + " " + v.Model), Detail: title, At: v.Added, Match: m, Source: "bitdispatcher"})
			}
		}
		for _, t := range in.Timeline {
			if (t.Kind == "note" || t.Kind == "priority-note") && len(q) >= 4 && strings.Contains(norm(t.Text), q) {
				add(Hit{Kind: "incident", Ref: in.Number, Title: title, Detail: t.Text, At: t.At, Match: "partial", Source: "bitdispatcher"})
				break
			}
		}
	}
	for _, c := range s.Calls {
		if c.Incident == "" && len(qd) >= 7 && strings.Contains(digits(c.Callback), qd) {
			add(Hit{Kind: "call", Ref: c.ID, Title: "Call from " + c.Callback, Detail: c.State, At: c.Start, Match: "exact", Source: "bitdispatcher"})
		}
	}
	for _, p := range s.Premises {
		if m := firstNonEmpty(has(p.Address), has(p.Name)); m != "" {
			add(Hit{Kind: "premise", Ref: p.ID, Title: firstNonEmpty(p.Name, p.Address), Detail: strings.Join(p.Hazards, ", "), At: p.Updated, Match: m, Source: firstNonEmpty(p.Source, "bitdispatcher")})
		}
	}
	for _, b := range s.BOLOs {
		if m := firstNonEmpty(has(b.Text), has(b.Plate)); m != "" {
			add(Hit{Kind: "bolo", Ref: b.ID, Title: b.ID + " " + b.Kind, Detail: b.Text, At: b.Issued, Match: m, Source: "bitdispatcher"})
		}
	}
	for _, h := range s.History {
		if m := firstNonEmpty(has(h.Address), has(h.Number)); m != "" {
			add(Hit{Kind: "history", Ref: h.Number, Title: h.Number + " " + firstNonEmpty(s.typeLabel(h.Type), h.Type), Detail: h.Address, At: h.At, Match: m, Source: firstNonEmpty(h.Source, "earlier system")})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Match != out[j].Match {
			return out[i].Match == "exact"
		}
		return out[i].At.After(out[j].At)
	})
	if limit <= 0 {
		limit = 50
	}
	if len(out) > limit {
		out = out[:limit]
	}
	if out == nil {
		out = []Hit{}
	}
	return out
}

// Dup is an open incident the call being entered may be about.
type Dup struct {
	Incident string    `json:"incident"`
	Type     string    `json:"type"`
	TypeName string    `json:"typeName"`
	Address  string    `json:"address,omitempty"`
	Created  time.Time `json:"created"`
	Metres   float64   `json:"metres"`
	Why      []string  `json:"why"`
	Units    []string  `json:"units"`
}

// Duplicates are open incidents the call being entered may be about: at the same address, within a few
// hundred metres, or from the same callback, in the last hours. Asked before a second incident is made.
func (s *State) Duplicates(typ, address string, lat, lon *float64, callback string, now time.Time) []Dup {
	out := []Dup{}
	addr, cb := norm(address), digits(callback)
	for _, in := range s.Incidents {
		if in.Status == Closed || now.Sub(in.Created) > 6*time.Hour {
			continue
		}
		d := Dup{Incident: in.Number, Type: in.Type, TypeName: s.typeLabel(in.Type), Created: in.Created, Metres: -1, Why: []string{}, Units: []string{}}
		if in.Location != nil {
			d.Address = in.Location.Address
			if addr != "" && norm(in.Location.Address) == addr {
				d.Why = append(d.Why, "same address")
			}
			if lat != nil && lon != nil && in.Location.HasPoint() {
				d.Metres = geo.Distance(*lat, *lon, *in.Location.Lat, *in.Location.Lon)
				if d.Metres <= 300 && !containsStr(d.Why, "same address") {
					d.Why = append(d.Why, fmt.Sprintf("%.0f m away", d.Metres))
				}
			}
		}
		if cb != "" && in.Caller != nil && digits(in.Caller.Callback) == cb {
			d.Why = append(d.Why, "same callback")
		}
		if len(d.Why) == 0 {
			continue
		}
		if typ != "" && strings.EqualFold(typ, in.Type) {
			d.Why = append(d.Why, "same call type")
		}
		for _, a := range in.Units {
			if a.Cleared.IsZero() {
				d.Units = append(d.Units, a.Unit)
			}
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].Why) != len(out[j].Why) {
			return len(out[i].Why) > len(out[j].Why)
		}
		return out[i].Created.After(out[j].Created)
	})
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}
