package api

// The final round of wishes (cad/places.go, flags.go, persons.go, vehicles.go, requests.go, shift.go,
// critical.go, critreport.go, keytests.go): structured places and location exceptions, flags, people's
// protected identifiers, vehicle codes, returns and tows, the NCIC request queue, Shift Intelligence,
// critical incident reviews, phrase alerts, key tests and the docked Street View check.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/foia"
	"github.com/biodream-llc/bitdispatcher/internal/ledger"
	"github.com/biodream-llc/bitdispatcher/internal/ncic"
	"github.com/biodream-llc/bitdispatcher/internal/phrase"
	"github.com/biodream-llc/bitdispatcher/internal/pip"
	"github.com/biodream-llc/bitdispatcher/internal/vehcodes"
)

func (s *Server) finalRoutes() {
	s.placeRoutes()
	s.flagRoutes()
	s.personRoutes()
	s.vehicleRoutes()
	s.requestRoutes()
	s.shiftRoutes()
	s.criticalRoutes()
	s.keyTestRoutes()
	s.pipRoutes()
	s.handle("GET /api/views/streetview", auth.Viewer, s.streetView)
}

// refused records a request someone may not make, and answers 403.
func (s *Server) refused(w http.ResponseWriter, a cad.Actor, what string, err error) {
	s.Eng.Record(a, cad.EvAccessRefused, map[string]string{"what": what, "why": err.Error()})
	fail(w, http.StatusForbidden, err.Error())
}

func isSupervisor(a cad.Actor) bool {
	return a.Role == "admin" || a.Role == "supervisor" || cad.DutyOf(a) == cad.DutySupervisor
}

// ---- places and location exceptions ----

func (s *Server) placeRoutes() {
	s.handle("GET /api/places/policy", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var p cad.PlacePolicy
		s.Eng.Read(func(st *cad.State) { p = st.Settings.Places })
		writeJSON(w, 200, map[string]any{"kinds": cad.PlaceKinds, "reasons": p.Reasons(), "dispositions": p.DispositionList(), "unitKinds": p.UnitKindList(),
			"noteAlways": p.ForceNoteAlways, "queue": []string{cad.QueueNew, cad.QueueReview, cad.QueueVerified, cad.QueueRejected, cad.QueueMerged}})
	})
	s.handle("PUT /api/incidents/{n}/place", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var p cad.Place
		if !readJSON(w, r, &p) {
			return
		}
		n := r.PathValue("n")
		var had bool
		s.Eng.Read(func(st *cad.State) {
			for _, x := range st.LocExceptions {
				if x.Incident == n && x.Open() {
					had = true
				}
			}
		})
		x, err := s.Eng.SetPlace(a, n, p)
		if err != nil {
			writeErr(w, err)
			return
		}
		if x != "" && !had {
			s.exceptionAlert(x)
		}
		writeJSON(w, 200, map[string]any{"exception": x})
	})
	s.handle("GET /api/exceptions", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if !isSupervisor(a) && cad.DutyOf(a) != cad.DutyRecords {
			s.refused(w, a, "location exceptions", fmt.Errorf("supervisors review location exceptions"))
			return
		}
		all := r.URL.Query().Get("all") != ""
		type row struct {
			*cad.LocException
			Open     bool     `json:"open"`
			Status   string   `json:"status"`
			Units    []string `json:"units"`
			Location string   `json:"location"`
		}
		out := []row{}
		s.Eng.Read(func(st *cad.State) {
			for _, x := range st.Exceptions(all) {
				cp := *x
				rw := row{LocException: &cp, Open: x.Open()}
				if in := st.Incidents[x.Incident]; in != nil {
					rw.Status = in.Status
					for _, u := range in.Units {
						rw.Units = append(rw.Units, u.Unit)
					}
					if in.Location != nil {
						rw.Location = in.Location.Address
					}
				}
				out = append(out, rw)
			}
		})
		writeJSON(w, 200, map[string]any{"items": out})
	})
	s.handle("POST /api/exceptions/{id}/review", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b cad.ExceptionReview
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.ReviewException(a, r.PathValue("id"), b))
	})
	s.handle("POST /api/exceptions/{id}/queue", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			State string         `json:"state"`
			Owner string         `json:"owner"`
			Note  string         `json:"note"`
			Link  *cad.PlaceLink `json:"link"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.QueueException(a, r.PathValue("id"), b.State, b.Owner, b.Note, b.Link))
	})
}

// exceptionAlert tells the supervisors at once: a high (or, when the agency asks for approval, urgent)
// communication to everyone on supervisor duty. It never holds up the dispatch.
func (s *Server) exceptionAlert(id string) {
	var x cad.LocException
	var urgent bool
	s.Eng.Read(func(st *cad.State) {
		if p := st.LocExceptions[id]; p != nil {
			x = *p
		}
		urgent = st.Settings.Places.SupervisorApprove
	})
	if x.ID == "" {
		return
	}
	pr := "high"
	if urgent {
		pr = "urgent"
	}
	s.Eng.SendComm(cad.SystemActor, cad.Comm{Kind: "admin", Priority: pr, Incident: x.Incident,
		Subject: fmt.Sprintf("Location %s on %s (%s)", strings.ToUpper(x.Kind), x.Incident, x.ID),
		Body: fmt.Sprintf("%s set %q as %s%s on %s. Review it in Supervisor › Location exceptions: was the best information used, and does the address data need follow-up.",
			x.By, x.Entered, x.Kind, map[bool]string{true: " (" + x.Reason + ")", false: ""}[x.Reason != ""], x.Incident),
		To: []string{cad.ToDuty + "supervisor"}})
	s.pipEvent(pip.Event{ID: "exception-" + x.ID, Class: pip.ClassException, Period: x.Period, At: x.At, Version: 1, People: []string{x.By}, Roles: []string{x.Duty},
		Sources: []pip.Ref{{Kind: "exception", ID: x.ID}, {Kind: "incident", ID: x.Incident}}, Facts: map[string]string{"kind": x.Kind, "callType": x.Type}})
}

// ---- flags ----

func (s *Server) flagRoutes() {
	s.handle("GET /api/flags/categories", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var cs []cad.FlagCategory
		s.Eng.Read(func(st *cad.State) { cs = st.Settings.Flags.CategoryList() })
		writeJSON(w, 200, map[string]any{"categories": cs})
	})
	// The flags at a verified address and sublocation (for call entry, before the incident is sent), or
	// possible matches near an unverified point.
	s.handle("GET /api/flags", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		q := r.URL.Query()
		duty := cad.DutyOf(a)
		now := s.Eng.Now()
		out := map[string]any{"flags": []*cad.Flag{}, "possible": []*cad.Flag{}}
		s.Eng.Read(func(st *cad.State) {
			key := cad.AddressKey(q.Get("address"))
			p := &cad.Place{Name: q.Get("name"), Complex: q.Get("complex"), Building: q.Get("building"), Floor: q.Get("floor"), Unit: q.Get("unit")}
			var fl []*cad.Flag
			if q.Get("verified") == "1" {
				for _, f := range st.FlagsAt(key, p, now) {
					fl = append(fl, st.FlagFor(f, duty, a.Role))
				}
			}
			var near []*cad.Flag
			if lat, err1 := strconv.ParseFloat(q.Get("lat"), 64); err1 == nil {
				if lon, err2 := strconv.ParseFloat(q.Get("lon"), 64); err2 == nil && q.Get("verified") != "1" {
					for _, f := range st.FlagsNear(lat, lon, now) {
						near = append(near, st.FlagFor(f, duty, a.Role))
					}
				}
			}
			if fl == nil {
				fl = []*cad.Flag{}
			}
			if near == nil {
				near = []*cad.Flag{}
			}
			out = map[string]any{"flags": fl, "possible": near}
		})
		writeJSON(w, 200, out)
	})
	s.handle("GET /api/incidents/{n}/flags", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		duty := cad.DutyOf(a)
		now := s.Eng.Now()
		var cur, near []*cad.Flag
		var shown []cad.FlagShown
		var ok bool
		s.Eng.Read(func(st *cad.State) {
			in := st.Incidents[r.PathValue("n")]
			if in == nil {
				return
			}
			ok = true
			for _, f := range st.IncidentFlags(in, now) {
				cur = append(cur, st.FlagFor(f, duty, a.Role))
			}
			shown = in.Flags
			if in.Place.Unverified() && in.Location != nil && in.Location.Lat != nil && in.Location.Lon != nil {
				for _, f := range st.FlagsNear(*in.Location.Lat, *in.Location.Lon, now) {
					near = append(near, st.FlagFor(f, duty, a.Role))
				}
			}
		})
		if !ok {
			fail(w, 404, "no incident "+r.PathValue("n"))
			return
		}
		writeJSON(w, 200, map[string]any{"flags": none(cur), "possible": none(near), "shown": shown})
	})
	s.handle("GET /api/flags/{id}", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var f *cad.Flag
		var cat cad.FlagCategory
		s.Eng.Read(func(st *cad.State) {
			if x := st.Flags[r.PathValue("id")]; x != nil {
				c := *x
				f = &c
				cat, _ = st.Settings.Flags.Category(x.Category)
			}
		})
		if f == nil {
			fail(w, 404, "no flag "+r.PathValue("id"))
			return
		}
		if !cat.MayView(cad.DutyOf(a), a.Role) {
			s.refused(w, a, "flag "+f.ID, fmt.Errorf("the detail of %s flags is for %s", strings.ToLower(cat.Name), strings.Join(cat.View, ", ")))
			return
		}
		if cat.Restricted {
			s.Eng.Record(a, cad.EvRecordsViewed, map[string]string{"flag": f.ID, "category": f.Category})
		}
		writeJSON(w, 200, f)
	})
	s.handle("GET /api/admin/flags", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		state := r.URL.Query().Get("state")
		text := strings.ToLower(r.URL.Query().Get("q"))
		out := []*cad.Flag{}
		s.Eng.Read(func(st *cad.State) {
			for _, f := range st.Flags {
				if state != "" && f.State != state {
					continue
				}
				if text != "" && !strings.Contains(strings.ToLower(f.Title+" "+f.Scope.Address+" "+f.Scope.Label()+" "+f.Category), text) {
					continue
				}
				out = append(out, st.FlagFor(f, cad.DutyOf(a), a.Role))
			}
		})
		slices.SortFunc(out, func(x, y *cad.Flag) int {
			return strings.Compare(y.Changed.Format(time.RFC3339Nano), x.Changed.Format(time.RFC3339Nano))
		})
		writeJSON(w, 200, map[string]any{"items": out})
	})
	s.handle("PUT /api/flags", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			cad.Flag
			ConflictOK bool `json:"conflictOk"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		id, err := s.Eng.SaveFlag(a, b.Flag, b.ConflictOK)
		if err != nil {
			writeErr(w, err)
			return
		}
		var st string
		s.Eng.Read(func(x *cad.State) { st = x.Flags[id].State })
		writeJSON(w, 200, map[string]string{"id": id, "state": st})
	})
	s.handle("POST /api/flags/{id}/decide", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Action  string    `json:"action"`
			Note    string    `json:"note"`
			Expires time.Time `json:"expires"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.DecideFlag(a, r.PathValue("id"), b.Action, b.Note, b.Expires))
	})
}

func none[T any](xs []T) []T {
	if xs == nil {
		return []T{}
	}
	return xs
}

// ---- people ----

func (s *Server) personRoutes() {
	s.handle("POST /api/persons/search", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var q cad.PersonQuery
		if !readJSON(w, r, &q) {
			return
		}
		var allowed bool
		s.Eng.Read(func(st *cad.State) { allowed = st.MayOpenIdentifiers(a) })
		if (q.SSN != "" || q.DL != "") && !allowed {
			s.refused(w, a, "person search by identifier", fmt.Errorf("your duty cannot search by Social Security or licence number"))
			return
		}
		var hits []cad.PersonHit
		s.Eng.Read(func(st *cad.State) { hits = st.FindPersons(q, 50) })
		terms := []string{}
		for k, v := range map[string]string{"first": q.First, "middle": q.Middle, "last": q.Last, "dob": q.DOB, "phone": q.Phone, "address": q.Address, "ssn": q.SSN, "dl": q.DL, "alias": q.Alias, "plate": q.Plate} {
			if v != "" {
				terms = append(terms, k)
			}
		}
		slices.Sort(terms)
		// The search is on the record by which fields were used, never the values of identifiers.
		s.Eng.Record(a, cad.EvSearched, map[string]any{"kind": "person", "fields": terms, "results": len(hits)})
		writeJSON(w, 200, map[string]any{"items": none(hits)})
	})
	s.handle("POST /api/persons/matches", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var p cad.Person
		if !readJSON(w, r, &p) {
			return
		}
		var hits []cad.PersonHit
		s.Eng.Read(func(st *cad.State) { hits = st.PossibleMatches(p) })
		writeJSON(w, 200, map[string]any{"items": none(hits)})
	})
	s.handle("POST /api/incidents/{n}/persons/{p}/identifiers", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Purpose string `json:"purpose"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		id, err := s.Eng.OpenIdentifiers(a, r.PathValue("n"), r.PathValue("p"), b.Purpose)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, id)
	})
}

// maskIncident is an incident as the shared board carries it: identifiers masked, returns without their
// content (those allowed open them from the record, and that is recorded).
func maskIncident(in *cad.Incident) *cad.Incident {
	if len(in.Persons) == 0 && len(in.Vehicles) == 0 {
		return in
	}
	c := *in
	c.Persons = make([]cad.Person, len(in.Persons))
	for i, p := range in.Persons {
		p = cad.MaskPerson(p)
		p.Returns = stripReturns(p.Returns)
		c.Persons[i] = p
	}
	c.Vehicles = make([]cad.Vehicle, len(in.Vehicles))
	for i, v := range in.Vehicles {
		v.Returns = stripReturns(v.Returns)
		c.Vehicles[i] = v
	}
	return &c
}

func stripReturns(rs []cad.InqReturn) []cad.InqReturn {
	if len(rs) == 0 {
		return rs
	}
	out := make([]cad.InqReturn, len(rs))
	for i, r := range rs {
		r.Text, r.Fields, r.HitDetail = "", nil, ""
		out[i] = r
	}
	return out
}

// ---- vehicles, returns and tows ----

func (s *Server) vehicleRoutes() {
	s.handle("GET /api/codes", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var lists []vehcodes.List
		var def string
		s.Eng.Read(func(st *cad.State) { lists, def = st.Settings.CodeLists(), st.Settings.StateDefault() })
		writeJSON(w, 200, map[string]any{"lists": lists, "defaultState": def, "relations": cad.VehicleRelations, "vehicleRoles": cad.VehicleRoles,
			"personRoles": cad.PersonRoles, "towSteps": cad.TowSteps})
	})
	s.handle("POST /api/vin/check", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			VIN string `json:"vin"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		writeJSON(w, 200, vehcodes.CheckVIN(b.VIN))
	})
	s.handle("POST /api/incidents/{n}/vehicles/duplicates", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var v cad.Vehicle
		if !readJSON(w, r, &v) {
			return
		}
		v.Plate = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(v.Plate), " ", ""))
		var out []string
		s.Eng.Read(func(st *cad.State) {
			if v.State == "" {
				v.State = st.Settings.StateDefault()
			}
			out = st.VehicleDuplicates(r.PathValue("n"), v)
		})
		writeJSON(w, 200, map[string]any{"items": none(out)})
	})
	// The protected record: a record's returns in full, with the differences from what was reported.
	s.handle("GET /api/incidents/{n}/records/{rec}/returns", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var rets []cad.InqReturn
		var disc []string
		var allowed, found bool
		s.Eng.Read(func(st *cad.State) {
			allowed = st.MayOpenIdentifiers(a)
			in := st.Incidents[r.PathValue("n")]
			if in == nil {
				return
			}
			for _, p := range in.Persons {
				if p.ID == r.PathValue("rec") {
					rets, found = p.Returns, true
				}
			}
			for _, v := range in.Vehicles {
				if v.ID == r.PathValue("rec") {
					rets, disc, found = v.Returns, v.Discrepancies(), true
				}
			}
		})
		if !found {
			fail(w, 404, "no such record")
			return
		}
		if !allowed {
			s.refused(w, a, "returns of "+r.PathValue("rec"), fmt.Errorf("your duty cannot open inquiry returns"))
			return
		}
		s.Eng.Record(a, cad.EvRecordsViewed, map[string]string{"incident": r.PathValue("n"), "record": r.PathValue("rec"), "what": "returns"})
		writeJSON(w, 200, map[string]any{"returns": none(rets), "discrepancies": none(disc)})
	})
	// Run the record through the message switch (or the simulator): the request is logged, the return is
	// attached to the record.
	s.handle("POST /api/incidents/{n}/records/{rec}/run", auth.Dispatcher, s.runRecord)
	// A return entered by hand (a DMV answer read over the phone, another state's teletype).
	s.handle("POST /api/incidents/{n}/records/{rec}/returns", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b cad.InqReturn
		if !readJSON(w, r, &b) {
			return
		}
		id, err := s.Eng.AttachReturn(a, r.PathValue("n"), r.PathValue("rec"), b)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("POST /api/incidents/{n}/records/{rec}/hit-confirmation", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			cad.Request
			ReturnID string `json:"returnId"`
			Note     string `json:"note"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		id, err := s.Eng.SendHitConfirmation(a, r.PathValue("n"), r.PathValue("rec"), b.ReturnID, b.Request)
		if err != nil {
			writeErr(w, err)
			return
		}
		if b.Note != "" {
			s.Eng.UpdateRequest(a, id, cad.RequestUpdate{Note: b.Note})
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("POST /api/tows/vehicle", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b cad.TowAsk
		if !readJSON(w, r, &b) {
			return
		}
		t, err := s.Eng.RequestVehicleTow(a, b)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, t)
	})
	s.handle("POST /api/tows/{id}/detail", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b cad.TowDetail
		if !readJSON(w, r, &b) {
			return
		}
		b.ID = r.PathValue("id")
		done(w, s.Eng.UpdateTow(a, b))
	})
	s.handle("GET /api/incidents/{n}/tows", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out := map[string][]*cad.Tow{}
		var vehicles []cad.Vehicle
		var rot []cad.TowTurn
		var classes, zones []string
		s.Eng.Read(func(st *cad.State) {
			if in := st.Incidents[r.PathValue("n")]; in != nil {
				vehicles = in.Vehicles
				for _, v := range in.Vehicles {
					var ts []*cad.Tow
					for _, t := range st.TowsOf(in.Number, v.ID) {
						c := *t
						ts = append(ts, &c)
					}
					out[v.ID] = none(ts)
				}
			}
			classes, zones = st.Settings.TowClassList(), st.Settings.TowZones
			cl := r.URL.Query().Get("class")
			if cl == "" && len(classes) > 0 {
				cl = classes[0]
			}
			el, _ := st.TowRotationFor(cl, r.URL.Query().Get("zone"))
			for _, t := range el {
				c := *t.Company
				rot = append(rot, cad.TowTurn{Company: &c, LastTurn: t.LastTurn})
			}
		})
		writeJSON(w, 200, map[string]any{"byVehicle": out, "vehicles": none(vehicles), "rotation": none(rot), "classes": classes, "zones": none(zones)})
	})
}

// ncicSwitch is the configured message switch, or why there is none.
func (s *Server) ncicSwitch() (ncic.Switch, string, string, error) {
	var mode, ori, addr string
	s.Eng.Read(func(st *cad.State) { mode, ori, addr = st.Settings.NCICMode, st.Settings.ORI, st.Settings.NCICAddr })
	if ori == "" {
		ori = "VA0000000"
	}
	switch mode {
	case "sim":
		return ncic.Sim{}, mode, ori, nil
	case "line":
		if addr == "" {
			return nil, mode, ori, fmt.Errorf("no message switch address in Settings")
		}
		return ncic.Line{Addr: addr}, mode, ori, nil
	}
	return nil, mode, ori, fmt.Errorf("NCIC inquiries are off: an administrator sets the switch in Settings")
}

func (s *Server) runRecord(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	if !writer(w, se) {
		return
	}
	var b struct {
		Key       string `json:"key"` // QV, RQ, QW, DQ (empty: RQ for a vehicle, QW for a person)
		Purpose   string `json:"purpose"`
		Requester string `json:"requester"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	n, rec := r.PathValue("n"), r.PathValue("rec")
	var q ncic.Query
	var cat string
	var found bool
	s.Eng.Read(func(st *cad.State) {
		in := st.Incidents[n]
		if in == nil {
			return
		}
		for _, v := range in.Vehicles {
			if v.ID == rec {
				found, cat = true, "vehicle"
				q = ncic.Query{Key: firstNonEmptyStr(strings.ToUpper(b.Key), "RQ"), Fields: map[string]string{"LIC": v.Plate, "LIS": v.State, "LIT": v.PlateType, "VIN": v.VIN}}
			}
		}
		for _, p := range in.Persons {
			if p.ID == rec {
				found, cat = true, "person"
				dob := strings.ReplaceAll(p.DOB, "-", "")
				name := strings.TrimSpace(strings.ToUpper(p.Last) + "," + strings.ToUpper(strings.TrimSpace(p.First+" "+p.Middle)))
				if p.Last == "" {
					name = p.Name
				}
				q = ncic.Query{Key: firstNonEmptyStr(strings.ToUpper(b.Key), "QW"), Fields: map[string]string{"NAM": name, "DOB": dob, "SEX": p.Sex, "RAC": p.Race, "OLN": p.DL, "OLS": p.DLState}}
			}
		}
	})
	if !found {
		fail(w, 404, "no person or vehicle "+rec+" on "+n)
		return
	}
	q.Purpose, q.Reason = firstNonEmptyStr(b.Purpose, "C"), n
	for k, v := range q.Fields {
		if strings.TrimSpace(v) == "" {
			delete(q.Fields, k)
		}
	}
	if err := q.Check(); err != nil {
		fail(w, 422, err.Error())
		return
	}
	sw, mode, ori, err := s.ncicSwitch()
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	typ := "ncic"
	if q.Key == "RQ" || q.Key == "DQ" {
		typ = "nlets"
	}
	rq, err := s.Eng.LogRequest(a, cad.Request{Type: typ, Category: cat, Incident: n, Record: rec, Requester: strings.ToUpper(b.Requester), Destination: ori})
	if err != nil {
		writeErr(w, err)
		return
	}
	s.Eng.UpdateRequest(a, rq, cad.RequestUpdate{Sent: true, Method: map[string]string{"sim": "training simulator", "line": "message switch"}[mode]})
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	text, err := sw.Send(ctx, ncic.Format(q, ori))
	result := "no record"
	switch {
	case err != nil:
		result = "error"
	case ncic.IsHit(text):
		result = "hit"
	}
	s.Eng.Record(a, cad.EvJusticeQuery, map[string]any{"key": q.Key, "purpose": q.Purpose, "reason": q.Reason, "terms": ncic.Hash(q), "result": result, "simulated": mode == "sim", "request": rq})
	if err != nil {
		s.Eng.UpdateRequest(a, rq, cad.RequestUpdate{Status: cad.RqError, Note: err.Error()})
		fail(w, 502, err.Error())
		return
	}
	ret := returnFrom(q.Key, text, mode == "sim")
	ret.Request, ret.Requester, ret.Purpose = rq, strings.ToUpper(b.Requester), q.Purpose
	id, err := s.Eng.AttachReturn(a, n, rec, ret)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"request": rq, "return": id, "hit": ret.Hit, "simulated": ret.Simulated})
}

// returnFrom reads an answer into a return's official fields.
func returnFrom(key, text string, sim bool) cad.InqReturn {
	f := ncic.Fields(text)
	ret := cad.InqReturn{Source: "ncic", Text: text, Simulated: sim, Fields: map[string]string{}}
	if key == "RQ" || key == "DQ" {
		ret.Source = "dmv"
	}
	m := map[string]string{"LIC": "plate", "LIS": "state", "LIT": "plateType", "VIN": "vin", "VYR": "year", "VMA": "make", "VMO": "model", "VST": "body",
		"VCO": "color", "OWN": "owner", "ADR": "ownerAddress", "EXP": "expires", "REG": "registration", "NAM": "name", "DOB": "dob", "SEX": "sex",
		"OLN": "licence", "OLS": "licenceState", "STATUS": "status", "OFF": "offense", "OCA": "oca"}
	for k, v := range f {
		if name, ok := m[k]; ok {
			ret.Fields[name] = v
		}
	}
	if strings.Contains(strings.ToUpper(text), "NO NCIC") || strings.Contains(strings.ToUpper(text), "NO RECORD") {
		ret.Fields["noRecord"] = "yes"
	}
	if ncic.IsHit(text) {
		mke := strings.ToUpper(f["MKE"])
		switch {
		case strings.Contains(mke, "STOLEN"):
			ret.Hit = "stolen"
		case strings.Contains(mke, "WANTED"):
			ret.Hit = "wanted"
		default:
			ret.Hit = strings.ToLower(firstNonEmptyStr(mke, "record"))
		}
		ret.HitDetail = mke
		if ori := strings.Fields(f["ORI"]); len(ori) > 0 {
			ret.HitORI = ori[0]
		}
	}
	return ret
}

// ---- the NCIC request queue ----

func (s *Server) requestRoutes() {
	s.handle("GET /api/requests", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		q := r.URL.Query()
		f := cad.RequestFilter{Status: q.Get("status"), Owner: q.Get("owner"), Incident: q.Get("incident"), Type: q.Get("type"), Category: q.Get("category"), Text: q.Get("q")}
		if q.Get("mine") != "" {
			f.Owner = a.User
		}
		if t, err := time.Parse(time.RFC3339, q.Get("from")); err == nil {
			f.From = t
		}
		if t, err := time.Parse(time.RFC3339, q.Get("to")); err == nil {
			f.To = t
		}
		var items []*cad.Request
		var labels map[string]string
		var queues []string
		var warn int
		s.Eng.Read(func(st *cad.State) {
			items = st.RequestList(f)
			labels = map[string]string{}
			for _, x := range cad.RequestStatuses {
				labels[x] = st.Settings.Requests.Label(x)
			}
			queues = st.Settings.Requests.QueueList()
			warn = st.Settings.Requests.WarnMinutes
		})
		if warn <= 0 {
			warn = 3
		}
		writeJSON(w, 200, map[string]any{"items": none(items), "labels": labels, "statuses": cad.RequestStatuses, "outcomes": cad.HitOutcomes, "types": cad.RequestTypes,
			"categories": cad.RequestCategories, "queues": queues, "warnMinutes": warn, "now": s.Eng.Now()})
	})
	s.handle("POST /api/requests", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b cad.Request
		if !readJSON(w, r, &b) {
			return
		}
		id, err := s.Eng.LogRequest(a, b)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("POST /api/requests/{id}", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b cad.RequestUpdate
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.UpdateRequest(a, r.PathValue("id"), b))
	})
	s.handle("POST /api/requests/{id}/handoff", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			To   string `json:"to"`
			Note string `json:"note"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		if err := s.Eng.HandoffRequest(a, r.PathValue("id"), b.To, b.Note); err != nil {
			writeErr(w, err)
			return
		}
		if !strings.HasPrefix(b.To, "@queue:") {
			s.Eng.SendComm(cad.SystemActor, cad.Comm{Kind: "admin", Priority: "high", Subject: "Request " + r.PathValue("id") + " handed to you",
				Body: a.User + " handed you request " + r.PathValue("id") + ": " + b.Note + ". Accept it in Requests.", To: []string{cad.ToUser + b.To}})
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("POST /api/requests/{id}/accept", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		done(w, s.Eng.AcceptRequest(a, r.PathValue("id")))
	})
	// What the person must finish or hand over before signing off.
	s.handle("GET /api/me/open-requests", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var out []*cad.Request
		s.Eng.Read(func(st *cad.State) { out = st.SignOffBlockers(a.User) })
		writeJSON(w, 200, map[string]any{"items": none(out)})
	})
}

// requestsWatch warns of overdue requests: the owner and the supervisors, once each.
func (s *Server) requestsWatch(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		if over, err := s.Eng.WarnRequests(); err == nil {
			for _, x := range over {
				to := []string{cad.ToDuty + "supervisor"}
				if x.Owner != "" {
					to = append(to, cad.ToUser+x.Owner)
				}
				s.Eng.SendComm(cad.SystemActor, cad.Comm{Kind: "admin", Priority: map[bool]string{true: "urgent", false: "high"}[x.Type == "hit-confirmation"],
					Incident: x.Incident, Subject: fmt.Sprintf("%s %s overdue", x.ID, x.Type),
					Body: fmt.Sprintf("%s (%s %s) owned by %s was due %s and is %s.", x.ID, x.Type, x.Subject, x.Owner, x.Due.Local().Format("15:04"), x.Status), To: to})
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// ---- Shift Intelligence ----

func (s *Server) mayReadShift(a cad.Actor) bool {
	var rec []string
	s.Eng.Read(func(st *cad.State) { rec = st.Settings.Shift.Recipients })
	if isSupervisor(a) {
		return true
	}
	return slices.Contains(rec, cad.DutyOf(a))
}

func (s *Server) periodFrom(r *http.Request) (cad.Period, bool) {
	q := r.URL.Query()
	var p cad.Period
	ok := true
	s.Eng.Read(func(st *cad.State) {
		switch {
		case q.Get("from") != "":
			f, err1 := time.Parse(time.RFC3339, q.Get("from"))
			t, err2 := time.Parse(time.RFC3339, q.Get("to"))
			if err1 != nil || err2 != nil || !t.After(f) {
				ok = false
				return
			}
			p = cad.Period{ID: "custom:" + f.UTC().Format(time.RFC3339) + "/" + t.UTC().Format(time.RFC3339), Name: "Custom range", From: f, To: t}
		case q.Get("period") != "":
			p, ok = st.PeriodByID(q.Get("period"))
		default:
			p = st.PeriodAt(s.Eng.Now())
		}
	})
	return p, ok
}

func (s *Server) shiftRoutes() {
	s.handle("GET /api/shift/periods", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		now := s.Eng.Now()
		var out []cad.Period
		var issued map[string]int
		s.Eng.Read(func(st *cad.State) {
			p := st.PeriodAt(now)
			out = append(out, p)
			for i := 0; i < 6; i++ {
				p = st.PeriodAt(p.From.Add(-time.Minute))
				out = append(out, p)
			}
			issued = map[string]int{}
			for id, rs := range st.ShiftReports {
				issued[id] = len(rs)
			}
		})
		writeJSON(w, 200, map[string]any{"periods": out, "issued": issued, "sections": cad.ReportSections, "names": cad.SectionNames})
	})
	s.handle("GET /api/shift/report", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if !s.mayReadShift(a) {
			s.refused(w, a, "shift report", fmt.Errorf("the Shift Intelligence report is for supervisors and the duties the agency names"))
			return
		}
		p, ok := s.periodFrom(r)
		if !ok {
			fail(w, 422, "no such period or range")
			return
		}
		var draft cad.ShiftReport
		var issued []*cad.ShiftReport
		s.Eng.Read(func(st *cad.State) {
			draft = st.ShiftDraft(p, s.Eng.Now(), cad.DutyOf(a), a.Role)
			draft.Custom = strings.HasPrefix(p.ID, "custom:")
			issued = st.ShiftReports[p.ID]
		})
		writeJSON(w, 200, map[string]any{"draft": draft, "issued": none(issued)})
	})
	s.handle("POST /api/shift/report/finalize", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Period  string `json:"period"`
			Summary string `json:"summary"`
			Handoff string `json:"handoff"`
			Note    string `json:"note"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		v, err := s.Eng.FinalizeShift(a, b.Period, b.Summary, b.Handoff, b.Note)
		if err != nil {
			writeErr(w, err)
			return
		}
		s.pipPeriod(b.Period, v)
		writeJSON(w, 200, map[string]int{"version": v})
	})
	s.handle("POST /api/shift/report/ack", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Period   string            `json:"period"`
			Accept   []string          `json:"accept"`
			Reassign map[string]string `json:"reassign"`
			Note     string            `json:"note"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.AckShift(a, b.Period, b.Accept, b.Reassign, b.Note))
	})
	s.handle("GET /api/shift/people", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		p, ok := s.periodFrom(r)
		if !ok {
			fail(w, 422, "no such period or range")
			return
		}
		evs, err := s.Eng.Events(0, 0, func(e ledger.Event) bool { return !e.At.Before(p.From) && (p.To.IsZero() || e.At.Before(p.To)) })
		if err != nil {
			writeErr(w, err)
			return
		}
		var out []cad.PersonShift
		s.Eng.Read(func(st *cad.State) { out = st.IndividualShift(p, evs) })
		if !isSupervisor(a) {
			// Everyone else sees their own record only.
			out = slices.DeleteFunc(out, func(x cad.PersonShift) bool { return x.User != a.User })
		}
		writeJSON(w, 200, map[string]any{"period": p, "people": none(out)})
	})
	s.handle("POST /api/shift/notes", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b cad.ShiftNote
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.AddShiftNote(a, b))
	})
	s.handle("GET /api/followups", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		all := r.URL.Query().Get("all") != ""
		var out []*cad.TrackedFollowup
		s.Eng.Read(func(st *cad.State) {
			for _, f := range st.Followups {
				if all || !f.Done {
					c := *f
					out = append(out, &c)
				}
			}
		})
		slices.SortFunc(out, func(x, y *cad.TrackedFollowup) int { return strings.Compare(x.ID, y.ID) })
		writeJSON(w, 200, map[string]any{"items": none(out)})
	})
	s.handle("POST /api/followups", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Key      string    `json:"key"`
			Incident string    `json:"incident"`
			Title    string    `json:"title"`
			Owner    string    `json:"owner"`
			Due      time.Time `json:"due"`
			Done     bool      `json:"done"`
			Note     string    `json:"note"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.SaveFollowup(a, b.Key, b.Incident, b.Title, b.Owner, b.Due, b.Done, b.Note))
	})
}

// ---- critical incidents and phrase alerts ----

func (s *Server) criticalRoutes() {
	s.handle("GET /api/critical", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if !isSupervisor(a) && cad.DutyOf(a) != cad.DutyRecords {
			s.refused(w, a, "critical incidents", fmt.Errorf("critical incident reviews are for supervisors"))
			return
		}
		var out []*cad.CriticalCase
		var alerts []*cad.PhraseAlert
		var disp []string
		s.Eng.Read(func(st *cad.State) {
			for _, c := range st.CriticalList(r.URL.Query().Get("all") != "") {
				cp := *c
				out = append(out, &cp)
			}
			for _, x := range st.OpenPhraseAlerts() {
				cp := *x
				alerts = append(alerts, &cp)
			}
			disp = st.Settings.Critical.PhraseDispositionList()
		})
		writeJSON(w, 200, map[string]any{"items": none(out), "phrases": none(alerts), "dispositions": disp, "results": cad.QAResults})
	})
	s.handle("POST /api/critical/flag", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Incident string `json:"incident"`
			On       bool   `json:"on"`
			Reason   string `json:"reason"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		var had bool
		s.Eng.Read(func(st *cad.State) {
			for _, c := range st.CriticalCases {
				had = had || c.Incident == b.Incident
			}
		})
		id, err := s.Eng.FlagCritical(a, b.Incident, b.On, b.Reason)
		if err != nil {
			writeErr(w, err)
			return
		}
		if !had && id != "" {
			s.criticalAlert(id)
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("POST /api/critical/{id}/ack", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		done(w, s.Eng.AckCritical(a, r.PathValue("id")))
	})
	s.handle("POST /api/critical/{id}/qa", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b cad.QAAnswer
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.AnswerQA(a, r.PathValue("id"), b))
	})
	s.handle("POST /api/critical/{id}", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b cad.CaseUpdate
		if !readJSON(w, r, &b) {
			return
		}
		if err := s.Eng.UpdateCase(a, r.PathValue("id"), b); err != nil {
			writeErr(w, err)
			return
		}
		var c cad.CriticalCase
		s.Eng.Read(func(st *cad.State) {
			if x := st.CriticalCases[r.PathValue("id")]; x != nil {
				c = *x
			}
		})
		// Support offered and after-action workflows are the only things RESET and ARC hear of.
		if b.Support != "" {
			s.pipEvent(pip.Event{ID: fmt.Sprintf("support-%s-%d", c.ID, len(c.Actions)), Class: pip.ClassSupport, Period: c.Period, At: s.Eng.Now(), Version: 1,
				Sources: []pip.Ref{{Kind: "critical", ID: c.ID}}, Facts: map[string]string{"kind": "support offered"}})
		}
		if b.AfterAct != "" {
			s.pipEvent(pip.Event{ID: fmt.Sprintf("afteraction-%s-%d", c.ID, len(c.Actions)), Class: pip.ClassAfterAction, Period: c.Period, At: s.Eng.Now(), Version: 1,
				Sources: []pip.Ref{{Kind: "critical", ID: c.ID}}, Facts: map[string]string{"kind": "after-action opened"}})
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// The full review, assembled now (pulled only by those allowed).
	s.handle("GET /api/critical/{id}/report", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if !isSupervisor(a) && cad.DutyOf(a) != cad.DutyRecords {
			s.refused(w, a, "critical review "+r.PathValue("id"), fmt.Errorf("a supervisor or higher runs the full critical incident review"))
			return
		}
		rep, text, err := s.criticalReport(r.PathValue("id"), r.URL.Query().Get("cutoff"))
		if err != nil {
			writeErr(w, err)
			return
		}
		s.Eng.Record(a, cad.EvRecordsViewed, map[string]string{"critical": r.PathValue("id"), "incident": rep.Incident.Number, "view": "critical review"})
		switch r.URL.Query().Get("view") {
		case "foia", "accreditation":
			derived, log := s.derivedCopy(text, r.URL.Query().Get("view"), reportNames(rep))
			writeJSON(w, 200, map[string]any{"view": r.URL.Query().Get("view"), "text": derived, "redactions": log, "source": cad.HashText(text)})
			return
		}
		if r.URL.Query().Get("format") == "text" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			io.WriteString(w, text)
			return
		}
		writeJSON(w, 200, map[string]any{"report": rep, "text": text})
	})
	s.handle("POST /api/critical/{id}/issue", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Cutoff string `json:"cutoff"`
			Note   string `json:"note"`
			Kind   string `json:"kind"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		rep, text, err := s.criticalReport(r.PathValue("id"), b.Cutoff)
		if err != nil {
			writeErr(w, err)
			return
		}
		if b.Kind == "foia" || b.Kind == "accreditation" {
			text, _ = s.derivedCopy(text, b.Kind, reportNames(rep))
		}
		v, err := s.Eng.IssueReport(a, r.PathValue("id"), cad.ReportVersion{Cutoff: rep.Cutoff, Hash: cad.HashText(text), Note: b.Note, Kind: b.Kind})
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"version": v, "hash": cad.HashText(text)})
	})
	s.handle("POST /api/phrases/{id}/dispose", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Disposition string `json:"disposition"`
			Note        string `json:"note"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.DisposePhrase(a, r.PathValue("id"), b.Disposition, b.Note))
	})
	// Try the agency's phrases (or ones being written) on a text, without alerting anyone.
	s.handle("POST /api/admin/phrases/try", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Rules  []phrase.Rule `json:"rules"`
			Text   string        `json:"text"`
			Source string        `json:"source"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		if b.Rules == nil {
			s.Eng.Read(func(st *cad.State) { b.Rules = st.Settings.Critical.Phrases })
		}
		problems := map[string][]string{}
		for _, ru := range b.Rules {
			if p := phrase.Validate(ru); len(p) > 0 {
				problems[ru.ID] = p
			}
		}
		writeJSON(w, 200, map[string]any{"matches": none(phrase.Check(b.Rules, phrase.Text{Source: firstNonEmptyStr(b.Source, "call"), Text: b.Text})), "problems": problems})
	})
	s.handle("GET /api/admin/phrases/examples", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		writeJSON(w, 200, map[string]any{"phrases": phrase.Examples(), "tags": cad.DefaultTags(), "qa": cad.DefaultQA()})
	})
}

func (s *Server) criticalReport(id, cutoff string) (cad.CriticalReport, string, error) {
	var number string
	s.Eng.Read(func(st *cad.State) {
		if c := st.CriticalCases[id]; c != nil {
			number = c.Incident
		}
	})
	if number == "" {
		return cad.CriticalReport{}, "", httpErr{http.StatusNotFound, "no critical review " + id}
	}
	evs, err := s.Eng.Events(0, 0, func(e ledger.Event) bool { return e.Incident == number })
	if err != nil {
		return cad.CriticalReport{}, "", err
	}
	var ct time.Time
	if cutoff != "" {
		if ct, err = time.Parse(time.RFC3339, cutoff); err != nil {
			return cad.CriticalReport{}, "", fmt.Errorf("the cutoff is a time like 2026-10-25T22:00:00Z")
		}
	}
	var rep cad.CriticalReport
	var loc *time.Location
	s.Eng.Read(func(st *cad.State) {
		rep, err = st.BuildCriticalReport(id, evs, ct, s.Eng.Now())
		loc, _ = time.LoadLocation(rep.TimeZone)
	})
	if err != nil {
		return rep, "", err
	}
	return rep, rep.Text(loc), nil
}

// derivedCopy is a role-scoped copy of a report: the agency's disclosure rules applied (cad's FOIA rules),
// with a log of every redaction; the source report is never changed. A person verifies it before release
// through the records-request workflow.
func (s *Server) derivedCopy(text, view string, names []string) (string, []string) {
	var rules []foia.Rule
	s.Eng.Read(func(st *cad.State) {
		for _, r := range st.FOIARules {
			if r.Enabled {
				rules = append(rules, *r)
			}
		}
	})
	if len(rules) == 0 {
		for _, r := range foia.SampleRules() {
			r.Enabled = true
			rules = append(rules, r)
		}
	}
	props, _ := foia.Propose(text, rules, foia.Context{RecordType: "incident", At: s.Eng.Now(), Names: names})
	var ds []foia.Decision
	var log []string
	for _, p := range props {
		ds = append(ds, foia.Decision{ID: p.ID, Start: p.Start, End: p.End, Text: p.Text, Accept: true, Basis: p.Basis, By: "agency rules", At: s.Eng.Now()})
		log = append(log, fmt.Sprintf("%s (%s): %d characters, %s", p.Rule, p.Basis, p.End-p.Start, p.Explain))
	}
	out, _ := foia.Apply(text, ds)
	head := map[string]string{"foia": "DERIVED RECORDS-REQUEST COPY: redacted by the agency's rules; a person verifies it before release.",
		"accreditation": "DERIVED ACCREDITATION COPY: redacted by the agency's rules; the QA items map to the standard named in the template."}[view]
	return head + "\n\n" + out, log
}

func reportNames(rep cad.CriticalReport) []string {
	var out []string
	for _, p := range rep.Incident.Persons {
		if p.Name != "" {
			out = append(out, p.Name)
		}
		if p.First != "" && p.Last != "" {
			out = append(out, p.First+" "+p.Last)
		}
	}
	return out
}

func (s *Server) criticalAlert(id string) {
	var c cad.CriticalCase
	s.Eng.Read(func(st *cad.State) {
		if x := st.CriticalCases[id]; x != nil {
			c = *x
		}
	})
	if c.ID == "" {
		return
	}
	s.Eng.SendComm(cad.SystemActor, cad.Comm{Kind: "admin", Priority: "urgent", Incident: c.Incident, Subject: "Critical incident " + c.Incident + " (" + c.ID + ")",
		Body: c.Summary + ". Triggered by: " + strings.Join(c.TriggerNames(), "; ") + ". Acknowledge it in Supervisor › Critical incidents.", To: []string{cad.ToDuty + "supervisor"}})
	people := []string{}
	for _, r := range c.Roles {
		people = append(people, r.User)
	}
	s.pipEvent(pip.Event{ID: "critical-" + c.ID, Class: pip.ClassCritical, Period: c.Period, At: c.Opened, Version: 1, People: people,
		Sources: []pip.Ref{{Kind: "critical", ID: c.ID}, {Kind: "incident", ID: c.Incident}}, Facts: map[string]string{"trigger": firstTrigger(c)}})
}

func firstTrigger(c cad.CriticalCase) string {
	if len(c.Triggers) == 0 {
		return ""
	}
	return c.Triggers[0].Kind
}

// criticalWatch alerts supervisors of critical cases opened by the records (call types, phrases, unit
// emergencies) once each.
func (s *Server) criticalWatch(ctx context.Context) {
	seen := map[string]bool{}
	first := true
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		var ids []string
		s.Eng.Read(func(st *cad.State) {
			for id := range st.CriticalCases {
				ids = append(ids, id)
			}
		})
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				if !first {
					s.criticalAlert(id)
				}
			}
		}
		first = false
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// ---- key tests ----

func (s *Server) keyTestRoutes() {
	s.handle("POST /api/me/keytest", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b cad.KeyTest
		if !readJSON(w, r, &b) {
			return
		}
		b.Console = firstNonEmptyStr(b.Console, se.Console)
		done(w, s.Eng.RecordKeyTest(a, b))
	})
	s.handle("GET /api/admin/keytests", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var out []*cad.KeyTest
		s.Eng.Read(func(st *cad.State) {
			for _, t := range st.KeyTests {
				c := *t
				out = append(out, &c)
			}
		})
		slices.SortFunc(out, func(x, y *cad.KeyTest) int { return strings.Compare(x.Console+x.Duty, y.Console+y.Duty) })
		writeJSON(w, 200, map[string]any{"items": none(out)})
	})
}

// ---- Street View ----

// streetView says whether Google has Street View at a point (with the agency's key), and how to show it
// docked: never a blank page.
func (s *Server) streetView(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	lat, err1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lon, err2 := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		fail(w, 422, "give lat and lon")
		return
	}
	s.mu.Lock()
	vc := s.integ.Views
	s.mu.Unlock()
	out := map[string]any{"lat": lat, "lon": lon, "link": vc.GoogleLink, "mapLink": fmt.Sprintf("https://www.google.com/maps/search/?api=1&query=%.6f,%.6f", lat, lon)}
	if vc.GoogleLink {
		out["linkUrl"] = fmt.Sprintf("https://www.google.com/maps/@?api=1&map_action=pano&viewpoint=%.6f,%.6f", lat, lon)
	}
	if vc.GoogleKey == "" {
		out["available"] = nil
		out["why"] = "Google Street View is not set up in Admin › Integrations › Views (it needs a key); the street photos below are from the open providers"
		writeJSON(w, 200, out)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	u := "https://maps.googleapis.com/maps/api/streetview/metadata?location=" + url.QueryEscape(fmt.Sprintf("%.6f,%.6f", lat, lon)) + "&radius=50&key=" + url.QueryEscape(vc.GoogleKey)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		out["available"] = nil
		out["why"] = "Google did not answer in time: retry, or use the map and aerial photos"
		writeJSON(w, 200, out)
		return
	}
	defer resp.Body.Close()
	var m struct {
		Status   string `json:"status"`
		Date     string `json:"date"`
		PanoID   string `json:"pano_id"`
		Location struct {
			Lat float64 `json:"lat"`
			Lng float64 `json:"lng"`
		} `json:"location"`
	}
	json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&m)
	switch m.Status {
	case "OK":
		out["available"] = true
		out["date"], out["panoLat"], out["panoLon"] = m.Date, m.Location.Lat, m.Location.Lng
		out["embed"] = fmt.Sprintf("https://www.google.com/maps/embed/v1/streetview?key=%s&pano=%s&fov=90", url.QueryEscape(vc.GoogleKey), url.QueryEscape(m.PanoID))
	case "ZERO_RESULTS", "NOT_FOUND":
		out["available"] = false
		out["why"] = "Google has no Street View within 50 m of this point"
	default:
		out["available"] = nil
		out["why"] = "Google answered " + firstNonEmptyStr(m.Status, resp.Status) + ": check the key in Admin › Integrations › Views"
	}
	writeJSON(w, 200, out)
}

// ---- PSC PIP ----

func (s *Server) pipConfig() pip.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.integ.PIP
}

func (s *Server) pipRoutes() {
	s.handle("GET /api/admin/pip", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		c := s.pipConfig()
		writeJSON(w, 200, map[string]any{"on": c.On, "url": c.URL, "tokenSet": c.Token != "", "classes": none(c.Classes), "identified": c.Identified,
			"aggregate": c.Aggregate, "saltSet": c.Salt != "", "allClasses": []string{pip.ClassPeriod, pip.ClassException, pip.ClassCritical, pip.ClassAfterAction, pip.ClassRecovery, pip.ClassSupport, pip.ClassCorrection}})
	})
	s.handle("PUT /api/admin/pip", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			On         bool     `json:"on"`
			URL        string   `json:"url"`
			Token      string   `json:"token"`
			Classes    []string `json:"classes"`
			Identified bool     `json:"identified"`
			Aggregate  bool     `json:"aggregate"`
			Salt       string   `json:"salt"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		b.URL = strings.TrimSpace(b.URL)
		if b.On {
			if err := pip.CheckURL(b.URL); err != nil {
				fail(w, 422, err.Error())
				return
			}
		}
		s.mu.Lock()
		n := s.integ
		c := &n.PIP
		c.On, c.URL, c.Classes, c.Identified, c.Aggregate = b.On, b.URL, b.Classes, b.Identified, b.Aggregate
		keepSecret(&c.Token, b.Token)
		keepSecret(&c.Salt, b.Salt)
		if c.Salt == "" {
			c.Salt = newToken()
		}
		err := s.writeIntegrations(n)
		if err == nil {
			s.integ = n
		}
		s.mu.Unlock()
		if err != nil {
			writeErr(w, err)
			return
		}
		s.Eng.Record(a, cad.EvIntegration, map[string]any{"pip": map[string]any{"on": b.On, "url": b.URL, "classes": b.Classes, "identified": b.Identified, "aggregate": b.Aggregate}})
		w.WriteHeader(http.StatusNoContent)
	})
	// What would cross for a period, without sending: for the agency to inspect the payload.
	s.handle("GET /api/admin/pip/preview", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		p, ok := s.periodFrom(r)
		if !ok {
			fail(w, 422, "no such period")
			return
		}
		ev := s.periodEvent(p, 0)
		c := s.pipConfig()
		prep, sent := pip.Prepare(c, ev)
		writeJSON(w, 200, map[string]any{"event": prep, "wouldSend": sent && c.On})
	})
}

// pipEvent prepares and sends one event to PSC PIP when the connection is on, and records the transfer.
func (s *Server) pipEvent(ev pip.Event) {
	c := s.pipConfig()
	if !c.On {
		return
	}
	prep, ok := pip.Prepare(c, ev)
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cl := pip.Client{Config: c}
		code, err := cl.Send(ctx, []pip.Event{prep})
		s.Eng.Record(cad.SystemActor, cad.EvPIPSent, map[string]any{"id": prep.ID, "class": prep.Class, "modules": prep.Modules, "status": code, "error": errString(err)})
	}()
}

// periodEvent is the period's objective counts for CEDE.
func (s *Server) periodEvent(p cad.Period, version int) pip.Event {
	ev := pip.Event{ID: fmt.Sprintf("period-%s-v%d", p.ID, version), Class: pip.ClassPeriod, Period: p.ID, At: p.To, Version: version, Counts: map[string]float64{}}
	s.Eng.Read(func(st *cad.State) {
		in := func(t time.Time) bool { return !t.Before(p.From) && (p.To.IsZero() || t.Before(p.To)) }
		var res, nres float64
		for _, x := range st.Incidents {
			if in(x.Created) {
				ev.Counts["incidents"]++
			}
		}
		for _, x := range st.LocExceptions {
			if in(x.At) {
				ev.Counts["locationExceptions"]++
				if x.Followup != nil && x.Followup.Done {
					res += x.Followup.DoneAt.Sub(x.At).Minutes()
					nres++
				}
			}
		}
		if nres > 0 {
			ev.Counts["meanExceptionResolutionMinutes"] = res / nres
		}
		for _, c := range st.CriticalCases {
			if in(c.Opened) {
				ev.Counts["criticalIncidents"]++
			}
		}
		for _, f := range st.Followups {
			if !f.Done {
				ev.Counts["openFollowups"]++
			}
		}
		for _, x := range st.Incidents {
			for _, n := range x.Notices {
				if in(n.At) {
					ev.Counts["coordinationContacts"]++
				}
			}
		}
		ev.Sources = []pip.Ref{{Kind: "period", ID: p.ID, Version: version}}
	})
	if version > 1 {
		ev.Class = pip.ClassCorrection
		ev.Corrects = fmt.Sprintf("period-%s-v%d", p.ID, version-1)
		ev.Facts = map[string]string{"class": pip.ClassPeriod}
	}
	return ev
}

// pipPeriod sends the issued period's counts (a version), and ROC's documented recovery effects: people
// who worked past the agency's break interval with no formal break.
func (s *Server) pipPeriod(periodID string, version int) {
	if !s.pipConfig().On {
		return
	}
	var p cad.Period
	var ok bool
	var breakMin int
	s.Eng.Read(func(st *cad.State) { p, ok = st.PeriodByID(periodID); breakMin = st.Settings.BreakMinutes })
	if !ok {
		return
	}
	s.pipEvent(s.periodEvent(p, version))
	if version != 1 {
		return
	}
	if breakMin <= 0 {
		breakMin = 120
	}
	evs, err := s.Eng.Events(0, 0, func(e ledger.Event) bool { return !e.At.Before(p.From) && e.At.Before(p.To) })
	if err != nil {
		return
	}
	var people []cad.PersonShift
	s.Eng.Read(func(st *cad.State) { people = st.IndividualShift(p, evs) })
	for _, x := range people {
		if len(x.SignOns) == 0 || len(x.Breaks) > 0 || p.To.Sub(x.SignOns[0].At) < time.Duration(breakMin)*time.Minute {
			continue
		}
		s.pipEvent(pip.Event{ID: fmt.Sprintf("recovery-%s-%s", p.ID, x.User), Class: pip.ClassRecovery, Period: p.ID, At: p.To, Version: 1, People: []string{x.User},
			Roles: x.Duties, Sources: []pip.Ref{{Kind: "period", ID: p.ID}}, Facts: map[string]string{"effect": "missed-break", "minutes": strconv.Itoa(breakMin)}})
	}
}
