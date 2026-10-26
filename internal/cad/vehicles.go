package cad

// Vehicles on an incident as an operational record: one or more roles, the plate with the agency's state
// by default and an approved plate type, the VIN checked without refusing a documented partial one, the
// DMV or NCIC returns attached beside what the caller reported (a difference is flagged, never
// overwritten), the people related to it in both directions, and a tow for that exact vehicle from the
// agency's rotation with every company contacted, each step's time and where it went.

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
	"github.com/biodream-llc/bitdispatcher/internal/vehcodes"
)

// EvTowDetail records a tow's progress and details: a step's time, the wrecker, where the vehicle went.
const EvTowDetail = "tow.detail"

// VehiclePolicy is the agency's vehicle entry rules.
type VehiclePolicy struct {
	// DefaultState is the plate state offered first (empty: the agency's ORI's first two letters, else VA).
	DefaultState string `json:"defaultState,omitempty"`
	// Codes are agency additions to the code lists (by list ID: state, plateType, color, body, make…).
	Codes map[string][]vehcodes.Code `json:"codes,omitempty"`
}

// StateDefault is the plate state offered first.
func (s Settings) StateDefault() string {
	if s.Vehicles.DefaultState != "" {
		return s.Vehicles.DefaultState
	}
	if len(s.ORI) >= 2 {
		return strings.ToUpper(s.ORI[:2])
	}
	return "VA"
}

// CodeList is a controlled list with the agency's additions.
func (s Settings) CodeList(id string) (vehcodes.List, bool) {
	l, ok := vehcodes.Get(id)
	if !ok {
		return l, false
	}
	return vehcodes.Merge(l, s.Vehicles.Codes[id], nil), true
}

// CodeLists are every controlled list with the agency's additions.
func (s Settings) CodeLists() []vehcodes.List {
	var out []vehcodes.List
	for _, l := range vehcodes.Lists() {
		out = append(out, vehcodes.Merge(l, s.Vehicles.Codes[l.ID], nil))
	}
	return out
}

// normVehicle tidies a vehicle and checks its codes and links.
func (e *Engine) normVehicle(in *Incident, v *Vehicle) error {
	v.Roles = cleanList(append([]string{v.Role}, v.Roles...))
	for i := range v.Roles {
		v.Roles[i] = strings.ToLower(v.Roles[i])
	}
	v.Roles = dedupe(v.Roles)
	if len(v.Roles) > 0 {
		v.Role = v.Roles[0]
	}
	st := e.st.Settings
	if v.Plate != "" && v.State == "" {
		v.State = st.StateDefault()
	}
	check := func(list, val, field string) error {
		if val == "" {
			return nil
		}
		l, ok := st.CodeList(list)
		if ok && !l.Has(val) {
			return problem(field, "%s is not on the %s list: choose one of the approved codes", val, strings.ToLower(l.Name))
		}
		return nil
	}
	v.PlateType, v.PlateCondition = strings.ToUpper(strings.TrimSpace(v.PlateType)), strings.ToUpper(strings.TrimSpace(v.PlateCondition))
	if err := check("state", v.State, "state"); err != nil {
		return err
	}
	if err := check("plateType", v.PlateType, "plateType"); err != nil {
		return err
	}
	if err := check("plateCondition", v.PlateCondition, "plateCondition"); err != nil {
		return err
	}
	v.VINNote = ""
	if v.VIN != "" {
		c := vehcodes.CheckVIN(v.VIN)
		v.VIN = c.VIN
		v.VINNote = strings.Join(c.Problems, "; ")
		if c.Valid && c.Year != "" {
			v.VINNote = "model year " + c.Year
		}
	}
	var links []VehicleLink
	for _, l := range v.Links {
		l.Relation = strings.ToLower(strings.TrimSpace(l.Relation))
		if l.Person == "" {
			continue
		}
		if !slices.ContainsFunc(in.Persons, func(p Person) bool { return p.ID == l.Person && !p.Removed }) {
			return problem("links", "no person %s on %s: add them in Persons first", l.Person, in.Number)
		}
		if !slices.Contains(VehicleRelations, l.Relation) {
			return problem("links", "the relation: %s", strings.Join(VehicleRelations, ", "))
		}
		if !slices.ContainsFunc(links, func(x VehicleLink) bool { return x == l }) {
			links = append(links, l)
		}
	}
	v.Links = links
	return nil
}

// Discrepancies are where the latest official return differs from what was reported for the vehicle.
func (v Vehicle) Discrepancies() []string {
	var ret *InqReturn
	for i := len(v.Returns) - 1; i >= 0; i-- {
		if !v.Returns[i].Superseded {
			ret = &v.Returns[i]
			break
		}
	}
	if ret == nil {
		return nil
	}
	var out []string
	cmp := func(label, reported, key string) {
		off := strings.TrimSpace(ret.Fields[key])
		if reported == "" || off == "" {
			return
		}
		if !strings.EqualFold(normPart(reported), normPart(off)) {
			out = append(out, fmt.Sprintf("%s: reported %s, return says %s", label, reported, off))
		}
	}
	cmp("Plate", v.Plate, "plate")
	cmp("VIN", v.VIN, "vin")
	cmp("Color", v.Color, "color")
	cmp("Make", v.Make, "make")
	cmp("Year", v.Year, "year")
	cmp("Owner", v.Owner, "owner")
	return out
}

// VehicleDuplicates are other vehicles with the same plate or VIN: on this incident, or on incidents of
// the last 30 days.
func (s *State) VehicleDuplicates(number string, v Vehicle) []string {
	if v.Plate == "" && v.VIN == "" {
		return nil
	}
	var out []string
	since := s.lastAt.Add(-30 * 24 * time.Hour)
	for _, in := range s.Incidents {
		if in.Number != number && in.Created.Before(since) {
			continue
		}
		for _, x := range in.Vehicles {
			if x.Removed || (in.Number == number && x.ID == v.ID) {
				continue
			}
			same := (v.Plate != "" && strings.EqualFold(x.Plate, v.Plate) && strings.EqualFold(x.State, v.State)) || (v.VIN != "" && strings.EqualFold(x.VIN, v.VIN))
			if !same {
				continue
			}
			if in.Number == number {
				out = append(out, fmt.Sprintf("already on this call as %s", x.ID))
			} else {
				out = append(out, fmt.Sprintf("on %s (%s) as %s, %s", in.Number, in.Type, x.Role, in.Created.Local().Format("Jan 2")))
			}
		}
	}
	slices.Sort(out)
	return out
}

// VehiclesOf are the vehicles a person on the incident is related to, with the relation.
func (in *Incident) VehiclesOf(pid string) []string {
	var out []string
	for _, v := range in.Vehicles {
		for _, l := range v.Links {
			if l.Person == pid && !v.Removed {
				out = append(out, fmt.Sprintf("%s %s (%s)", v.ID, strings.TrimSpace(v.Plate+" "+v.State), l.Relation))
			}
		}
	}
	return out
}

// TowAsk is a tow asked for one vehicle on an incident.
type TowAsk struct {
	Incident  string `json:"incident"`
	Vehicle   string `json:"vehicle"` // the vehicle's ID on the incident (V1)
	Requester string `json:"requester,omitempty"`
	Reason    string `json:"reason,omitempty"`
	RequestNo string `json:"requestNo,omitempty"`
	Class     string `json:"class"`
	Zone      string `json:"zone,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Company   string `json:"company,omitempty"`
	Note      string `json:"note,omitempty"`
	Override  string `json:"override,omitempty"` // why the rotation was not followed (a supervisor's)
}

// RequestVehicleTow asks for a tow for one vehicle on an incident: when the incident has vehicles the tow
// must name one, so no tow is left at the call level with the towed vehicle unclear.
func (e *Engine) RequestVehicleTow(a Actor, t TowAsk) (Tow, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	in := e.st.Incidents[t.Incident]
	if in == nil {
		return Tow{}, problem("incident", "no incident %s", t.Incident)
	}
	label := t.Vehicle
	if len(in.Vehicles) > 0 {
		i := slices.IndexFunc(in.Vehicles, func(v Vehicle) bool { return v.ID == t.Vehicle && !v.Removed })
		if i < 0 {
			return Tow{}, problem("vehicle", "choose which vehicle on the call is towed")
		}
		v := in.Vehicles[i]
		label = strings.TrimSpace(v.ID + " " + v.Plate + " " + v.State + " " + v.Make + " " + v.Model)
		for _, x := range e.st.Tows {
			if x.Incident == t.Incident && strings.HasPrefix(x.Vehicle, v.ID+" ") || x.Incident == t.Incident && x.Vehicle == v.ID {
				if x.Result == TowCalled || x.Result == TowAccepted {
					return Tow{}, problem("vehicle", "%s already has tow %s (%s): answer it first", v.ID, x.ID, x.Name)
				}
			}
		}
	} else if t.Vehicle == "" {
		return Tow{}, problem("vehicle", "add the vehicle to the call first, then ask for its tow")
	}
	if t.Kind == TowPolice || t.Kind == TowOwner {
		if t.Kind == TowPolice && strings.TrimSpace(t.Override) == "" {
			return Tow{}, problem("override", "say why the rotation is not followed")
		}
	}
	return e.requestTow(a, t.Incident, t.Class, t.Zone, t.Kind, t.Company, label, strings.TrimSpace(t.Note),
		&TowAsk{Requester: strings.ToUpper(strings.TrimSpace(t.Requester)), Reason: strings.TrimSpace(t.Reason), RequestNo: strings.TrimSpace(t.RequestNo), Override: strings.TrimSpace(t.Override)})
}

// TowSteps are the steps of a tow whose times are kept.
var TowSteps = []string{"requested", "accepted", "dispatched", "enroute", "arrived", "hooked", "transport", "completed", "cancelled", "refused", "reassigned"}

// TowDetail is a change to a tow's record.
type TowDetail struct {
	ID          string    `json:"id"`
	Step        string    `json:"step,omitempty"` // a step done now (or at At)
	At          time.Time `json:"at,omitzero"`
	Driver      string    `json:"driver,omitempty"`
	Truck       string    `json:"truck,omitempty"`
	Destination string    `json:"destination,omitempty"`
	Impound     string    `json:"impound,omitempty"`
	Hold        string    `json:"hold,omitempty"`
	Release     string    `json:"release,omitempty"`
	RequestNo   string    `json:"requestNo,omitempty"`
	Note        string    `json:"note,omitempty"`
}

// UpdateTow records a tow's step or details.
func (e *Engine) UpdateTow(a Actor, d TowDetail) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	t := e.st.tow(d.ID)
	if t == nil {
		return problem("id", "no tow %s", d.ID)
	}
	if d.Step != "" && !slices.Contains(TowSteps, d.Step) {
		return problem("step", "the steps: %s", strings.Join(TowSteps, ", "))
	}
	if d.At.IsZero() {
		d.At = e.now()
	}
	_, err := e.write(a, EvTowDetail, refs{Incident: t.Incident}, d)
	return err
}

func (s *State) applyTowDetail(e ledger.Event) (bool, error) {
	if e.Type != EvTowDetail {
		return false, nil
	}
	d, err := decode[TowDetail](e)
	if err != nil {
		return true, err
	}
	t := s.tow(d.ID)
	if t == nil {
		return true, nil
	}
	set := func(p *string, v string) {
		if v = strings.TrimSpace(v); v != "" {
			*p = v
		}
	}
	set(&t.Driver, d.Driver)
	set(&t.Truck, d.Truck)
	set(&t.Destination, d.Destination)
	set(&t.Impound, d.Impound)
	set(&t.Hold, d.Hold)
	set(&t.Release, d.Release)
	set(&t.RequestNo, d.RequestNo)
	if d.Note != "" {
		t.Note = strings.TrimSpace(t.Note + " " + d.Note)
	}
	if d.Step != "" {
		if t.Times == nil {
			t.Times = map[string]time.Time{}
		}
		t.Times[d.Step] = d.At
		switch d.Step {
		case "cancelled":
			t.Result, t.ResultAt = TowCancelled, e.At
		case "refused":
			t.Result, t.ResultAt = TowDeclined, e.At
		case "arrived":
			t.Result, t.ResultAt = TowArrived, e.At
		case "accepted":
			t.Result, t.ResultAt = TowAccepted, e.At
		}
	}
	if in := s.Incidents[t.Incident]; in != nil {
		var bits []string
		if d.Step != "" {
			bits = append(bits, d.Step+" "+d.At.Local().Format("15:04"))
		}
		for _, x := range [][2]string{{"driver", d.Driver}, {"truck", d.Truck}, {"to", d.Destination}, {"impound", d.Impound}, {"hold", d.Hold}, {"release", d.Release}} {
			if x[1] != "" {
				bits = append(bits, x[0]+" "+x[1])
			}
		}
		in.add(e, "tow", fmt.Sprintf("Tow %s (%s, %s): %s%s", t.ID, t.Name, t.Vehicle, strings.Join(bits, ", "), suffix(d.Note)))
	}
	return true, nil
}

// TowsOf are the tows for one vehicle on an incident, oldest first: every company contacted.
func (s *State) TowsOf(number, vehicle string) []*Tow {
	var out []*Tow
	for _, t := range s.Tows {
		if t.Incident == number && (t.Vehicle == vehicle || strings.HasPrefix(t.Vehicle, vehicle+" ")) {
			out = append(out, t)
		}
	}
	return out
}
