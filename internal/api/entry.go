package api

// Call entry and the roles' tools: modifiers, people, vehicles, contacts, the caller's own location,
// newer caller locations, previous incidents, CAD search, possible duplicates, approved guidance, unit
// acknowledgement, BOLOs, facilities, the passdown log and response plans.

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/guidance"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// EntryView is what the board carries for call entry and the roles' tools.
type EntryView struct {
	Modifiers  []cad.Modifier  `json:"modifiers"`
	BOLOs      []*cad.BOLO     `json:"bolos"`
	Facilities []*cad.Facility `json:"facilities"`
	Passdown   []cad.Passdown  `json:"passdown"`
	Guidance   int             `json:"guidance"` // approved documents in the library
}

func (s *Server) entryView(st *cad.State, now time.Time) EntryView {
	v := EntryView{Modifiers: st.Settings.ModifierList(), BOLOs: st.ActiveBOLOs(now), Facilities: []*cad.Facility{}, Passdown: []cad.Passdown{}}
	for _, f := range st.Facilities {
		v.Facilities = append(v.Facilities, f)
	}
	sort.Slice(v.Facilities, func(i, j int) bool { return v.Facilities[i].Name < v.Facilities[j].Name })
	for _, p := range st.Passdown {
		if now.Sub(p.At) < 48*time.Hour && (p.Until.IsZero() || now.Before(p.Until)) {
			v.Passdown = append(v.Passdown, p)
		}
	}
	sort.Slice(v.Passdown, func(i, j int) bool { return v.Passdown[i].At.After(v.Passdown[j].At) })
	for _, d := range st.Guidance {
		if d.Approved {
			v.Guidance++
		}
	}
	return v
}

func (s *Server) entryRoutes() {
	ok := func(w http.ResponseWriter, err error) {
		if err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
	s.handle("PUT /api/incidents/{n}/modifiers", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Modifiers []string `json:"modifiers"`
		}
		if readJSON(w, r, &b) {
			ok(w, s.Eng.SetModifiers(a, r.PathValue("n"), b.Modifiers))
		}
	})
	savePerson := func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var p cad.Person
		if !readJSON(w, r, &p) {
			return
		}
		p.ID = r.PathValue("id")
		id, err := s.Eng.SavePerson(a, r.PathValue("n"), p)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	}
	s.handle("POST /api/incidents/{n}/persons", auth.Dispatcher, savePerson)
	s.handle("PUT /api/incidents/{n}/persons/{id}", auth.Dispatcher, savePerson)
	saveVehicle := func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var v cad.Vehicle
		if !readJSON(w, r, &v) {
			return
		}
		v.ID = r.PathValue("id")
		id, err := s.Eng.SaveVehicle(a, r.PathValue("n"), v)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	}
	s.handle("POST /api/incidents/{n}/vehicles", auth.Dispatcher, saveVehicle)
	s.handle("PUT /api/incidents/{n}/vehicles/{id}", auth.Dispatcher, saveVehicle)
	saveContact := func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var c cad.Contact
		if !readJSON(w, r, &c) {
			return
		}
		c.ID = r.PathValue("id")
		id, err := s.Eng.SaveContact(a, r.PathValue("n"), c)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	}
	s.handle("POST /api/incidents/{n}/contacts", auth.Dispatcher, saveContact)
	s.handle("PUT /api/incidents/{n}/contacts/{id}", auth.Dispatcher, saveContact)
	s.handle("PUT /api/incidents/{n}/caller-location", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Location *ng911.Location `json:"location"`
		}
		if readJSON(w, r, &b) {
			ok(w, s.Eng.SetCallerLocation(a, r.PathValue("n"), b.Location))
		}
	})
	s.handle("POST /api/incidents/{n}/location", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Accept bool   `json:"accept"`
			Why    string `json:"why"`
		}
		if readJSON(w, r, &b) {
			ok(w, s.Eng.DecideLocation(a, r.PathValue("n"), b.Accept, b.Why))
		}
	})
	s.handle("POST /api/incidents/{n}/verify", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		ok(w, s.Eng.VerifyLocation(a, r.PathValue("n")))
	})
	s.handle("PUT /api/incidents/{n}/talkgroup", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Talkgroup string `json:"talkgroup"`
		}
		if readJSON(w, r, &b) {
			ok(w, s.Eng.SetTalkgroup(a, r.PathValue("n"), b.Talkgroup))
		}
	})
	s.handle("POST /api/units/{id}/ack", auth.Field, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		id := strings.ToUpper(r.PathValue("id"))
		if se.Role == auth.Viewer {
			fail(w, 403, "viewers cannot acknowledge for units")
			return
		}
		how := "dispatcher"
		if se.Role == auth.Field {
			if !strings.EqualFold(se.Console, id) {
				fail(w, 403, "a crew acknowledges for its own unit")
				return
			}
			how = "mobile terminal"
		}
		ok(w, s.Eng.AckUnit(a, id, how))
	})

	// Looking things up.
	s.handle("GET /api/previous", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		q := r.URL.Query()
		pq := cad.PrevQuery{Number: q.Get("number"), Address: q.Get("address"), Phones: q["phone"], Names: q["name"], Plates: q["plate"], VINs: q["vin"]}
		if lat, err := strconv.ParseFloat(q.Get("lat"), 64); err == nil {
			if lon, err := strconv.ParseFloat(q.Get("lon"), 64); err == nil {
				pq.Lat, pq.Lon = &lat, &lon
			}
		}
		var out []cad.PrevMatch
		s.Eng.Read(func(st *cad.State) {
			// The incident's own address, people and vehicles count too.
			if in, ok := st.Incidents[pq.Number]; ok {
				if in.Location != nil {
					pq.Address = firstNonEmpty(pq.Address, in.Location.Address)
					if pq.Lat == nil && in.Location.HasPoint() {
						pq.Lat, pq.Lon = in.Location.Lat, in.Location.Lon
					}
				}
				if in.Caller != nil {
					pq.Phones = append(pq.Phones, in.Caller.Callback)
				}
				for _, p := range in.Persons {
					pq.Names, pq.Phones = append(pq.Names, p.Name), append(pq.Phones, p.Phone)
				}
				for _, v := range in.Vehicles {
					pq.Plates, pq.VINs = append(pq.Plates, v.Plate), append(pq.VINs, v.VIN)
				}
			}
			out = st.Previous(pq, s.Eng.Now())
		})
		writeJSON(w, 200, out)
	})
	s.handle("GET /api/search", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var out []cad.Hit
		s.Eng.Read(func(st *cad.State) { out = st.Search(r.URL.Query().Get("q"), 60) })
		writeJSON(w, 200, out)
	})
	s.handle("GET /api/duplicates", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		q := r.URL.Query()
		var lat, lon *float64
		if la, err := strconv.ParseFloat(q.Get("lat"), 64); err == nil {
			if lo, err := strconv.ParseFloat(q.Get("lon"), 64); err == nil {
				lat, lon = &la, &lo
			}
		}
		var out []cad.Dup
		s.Eng.Read(func(st *cad.State) {
			out = st.Duplicates(q.Get("type"), q.Get("address"), lat, lon, q.Get("callback"), s.Eng.Now())
		})
		writeJSON(w, 200, out)
	})

	s.handle("GET /api/premises/near", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		lat, e1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
		lon, e2 := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
		if e1 != nil || e2 != nil {
			fail(w, 400, "lat and lon")
			return
		}
		out := []cad.Premise{}
		s.Eng.Read(func(st *cad.State) {
			for _, p := range st.PremisesNear(lat, lon) {
				out = append(out, *p)
			}
		})
		writeJSON(w, 200, out)
	})

	// Approved guidance: for an incident, for call entry before there is one, a whole document.
	s.handle("GET /api/incidents/{n}/guidance", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var out guidance.Result
		s.Eng.Read(func(st *cad.State) { out = st.MatchGuidance(r.PathValue("n"), s.Eng.Now(), 20) })
		writeJSON(w, 200, out)
	})
	s.handle("GET /api/guidance/match", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		q := r.URL.Query()
		var docs []guidance.Doc
		s.Eng.Read(func(st *cad.State) {
			for _, d := range st.Guidance {
				docs = append(docs, *d)
			}
		})
		p, _ := strconv.Atoi(q.Get("priority"))
		writeJSON(w, 200, guidance.Match(docs, guidance.Context{CallType: strings.ToUpper(q.Get("type")), Modifiers: q["modifier"], Text: q.Get("text"), Priority: p, At: s.Eng.Now()}, 20))
	})
	s.handle("POST /api/incidents/{n}/guidance/shown", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Shown []cad.Shown `json:"shown"`
		}
		if readJSON(w, r, &b) {
			ok(w, s.Eng.GuidanceShown(a, r.PathValue("n"), b.Shown))
		}
	})
	s.handle("GET /api/guidance/{id}", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var d *guidance.Doc
		s.Eng.Read(func(st *cad.State) {
			if x, ok := st.Guidance[r.PathValue("id")]; ok {
				cp := *x
				d = &cp
			}
		})
		if d == nil || (!d.Approved && !auth.AtLeast(se.Role, auth.Admin)) {
			fail(w, 404, "no approved document "+r.PathValue("id"))
			return
		}
		writeJSON(w, 200, map[string]any{"doc": d, "sections": guidance.Sections(*d)})
	})
	s.handle("GET /api/admin/guidance", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out := []*guidance.Doc{}
		s.Eng.Read(func(st *cad.State) {
			for _, d := range st.Guidance {
				cp := *d
				out = append(out, &cp)
			}
		})
		sortBy(out, func(d *guidance.Doc) string { return d.ID })
		writeJSON(w, 200, map[string]any{"items": out, "examples": guidance.Examples()})
	})
	s.handle("PUT /api/admin/guidance/{id}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var d guidance.Doc
		if !readJSON(w, r, &d) {
			return
		}
		d.ID = r.PathValue("id")
		ok(w, s.Eng.SaveGuidance(a, d))
	})
	s.handle("DELETE /api/admin/guidance/{id}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		ok(w, s.Eng.RemoveGuidance(a, r.PathValue("id")))
	})

	// BOLOs.
	s.handle("POST /api/bolos", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b cad.BOLO
		if !readJSON(w, r, &b) {
			return
		}
		b.ID = ""
		id, err := s.Eng.SaveBOLO(a, b)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("PUT /api/bolos/{id}", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b cad.BOLO
		if !readJSON(w, r, &b) {
			return
		}
		b.ID = r.PathValue("id")
		_, err := s.Eng.SaveBOLO(a, b)
		ok(w, err)
	})
	s.handle("POST /api/bolos/{id}/cancel", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Why string `json:"why"`
		}
		if readJSON(w, r, &b) {
			ok(w, s.Eng.CancelBOLO(a, r.PathValue("id"), b.Why))
		}
	})

	// Facilities: set up by administrators, status kept by dispatchers.
	s.handle("PUT /api/admin/facilities/{id}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var f cad.Facility
		if !readJSON(w, r, &f) {
			return
		}
		f.ID = r.PathValue("id")
		ok(w, s.Eng.SaveFacility(a, f))
	})
	s.handle("DELETE /api/admin/facilities/{id}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		ok(w, s.Eng.RemoveFacility(a, strings.ToUpper(r.PathValue("id"))))
	})
	s.handle("POST /api/facilities/{id}/status", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Status string    `json:"status"`
			Note   string    `json:"note"`
			Until  time.Time `json:"until"`
		}
		if readJSON(w, r, &b) {
			ok(w, s.Eng.SetFacilityStatus(a, strings.ToUpper(r.PathValue("id")), b.Status, b.Note, b.Until))
		}
	})

	// The passdown log.
	s.handle("POST /api/passdown", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Text  string    `json:"text"`
			For   string    `json:"for"`
			Until time.Time `json:"until"`
		}
		if readJSON(w, r, &b) {
			ok(w, s.Eng.AddPassdown(a, b.Text, b.For, b.Until))
		}
	})

	// Response plans.
	s.handle("GET /api/admin/plans", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out := []*cad.ResponsePlan{}
		s.Eng.Read(func(st *cad.State) {
			for _, p := range st.Plans {
				cp := *p
				out = append(out, &cp)
			}
		})
		sortBy(out, func(p *cad.ResponsePlan) string { return p.Name })
		writeJSON(w, 200, out)
	})
	s.handle("PUT /api/admin/plans/{id}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var p cad.ResponsePlan
		if !readJSON(w, r, &p) {
			return
		}
		if id := r.PathValue("id"); id != "new" {
			p.ID = id
		}
		id, err := s.Eng.SavePlan(a, p)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("DELETE /api/admin/plans/{id}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		ok(w, s.Eng.RemovePlan(a, r.PathValue("id")))
	})
}
