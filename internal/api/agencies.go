package api

// Agencies, people's profiles and coverage, saved duty presets and the supervisory views (cad/agency.go,
// cad/presets.go). Configuration that belongs to an agency (units, stations, call types, response plans)
// is changed only by people who may work that agency; a person limited to some agencies sees the
// supervisory views and configuration of those only.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// workingAgency is the agency a session is working for (empty with no agencies configured).
func (s *Server) workingAgency(se *auth.Session) string {
	ag := ""
	s.Eng.Read(func(st *cad.State) {
		if len(st.Agencies) > 0 {
			ag = st.CoverageOf(se.Username, se.Console, se.Duty).Working
		}
	})
	return ag
}

// viewRefused answers a request for a supervisory view the person may not see, and records it.
func (s *Server) viewRefused(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) bool {
	id := cad.ViewForPath(r.URL.Path)
	if id == "" {
		return false
	}
	ok := true
	s.Eng.Read(func(st *cad.State) { ok = st.MaySeeView(se.Username, se.Role, id) })
	if ok {
		return false
	}
	s.Eng.Record(a, cad.EvAccessRefused, map[string]string{"view": id, "path": r.URL.Path})
	fail(w, http.StatusForbidden, "the agency has not given you this view")
	return true
}

// agencyScoped wraps a configuration change of one kind (units, stations, calltypes, plans): the person
// must be allowed to work the agencies of the item as it was and as it will be.
func (s *Server) agencyScoped(kind string, h handler) handler {
	return func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var body []byte
		if r.Body != nil {
			body, _ = io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		var nw struct {
			Agency   string   `json:"agency"`
			Agencies []string `json:"agencies"`
		}
		json.Unmarshal(body, &nw)
		id := r.PathValue("id")
		if id == "" {
			id = r.PathValue("code")
		}
		var refuse string
		s.Eng.Read(func(st *cad.State) {
			if st.AllAgencies(se.Username) {
				return
			}
			var codes []string
			switch kind {
			case "units":
				if u := st.Units[strings.ToUpper(id)]; u != nil {
					codes = append(codes, u.Agency)
				}
			case "stations":
				if x := st.Stations[strings.ToUpper(id)]; x != nil {
					codes = append(codes, x.Agency)
				}
			case "calltypes":
				if x := st.CallTypes[strings.ToUpper(id)]; x != nil {
					codes = append(codes, x.Agencies...)
					if len(x.Agencies) == 0 {
						codes = append(codes, "")
					}
				}
			case "plans":
				if x := st.Plans[id]; x != nil {
					codes = append(codes, x.Agency)
				}
			}
			if r.Method != http.MethodDelete {
				if kind == "calltypes" {
					codes = append(codes, nw.Agencies...)
					if len(nw.Agencies) == 0 {
						codes = append(codes, "")
					}
				} else {
					codes = append(codes, nw.Agency)
				}
			}
			for _, c := range codes {
				if c == "" {
					refuse = "this is centre-wide configuration: a person limited to some agencies cannot change it"
					return
				}
				if !st.MayWork(se.Username, strings.ToUpper(c)) {
					refuse = "you may not change " + strings.ToUpper(c) + "'s configuration"
					return
				}
			}
		})
		if refuse != "" {
			s.Eng.Record(a, cad.EvAccessRefused, map[string]string{"config": kind, "id": id, "why": refuse})
			fail(w, http.StatusForbidden, refuse)
			return
		}
		h(w, r, se, a)
	}
}

// AgencyView is what a console knows of the agencies: the list, and what this person is working.
type AgencyView struct {
	Agencies []*cad.Agency    `json:"agencies"`
	Coverage cad.WorkCoverage `json:"coverage"`
	Staff    cad.Staff        `json:"staff"`
	May      []string         `json:"may"`      // the agencies the person may work
	MayWatch []string         `json:"mayWatch"` // and those they may watch (scope.go)
}

func (s *Server) agencyRoutes() {
	s.handle("GET /api/agencies", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var v AgencyView
		s.Eng.Read(func(st *cad.State) {
			v = AgencyView{Agencies: st.AgencyList(), Coverage: st.CoverageOf(se.Username, se.Console, se.Duty), Staff: st.StaffOf(se.Username), May: []string{}, MayWatch: []string{}}
			for _, g := range v.Agencies {
				if !g.Inactive && st.MayWork(se.Username, g.Code) {
					v.May = append(v.May, g.Code)
				}
				if st.MayWatch(se.Username, se.Console, g.Code) {
					v.MayWatch = append(v.MayWatch, g.Code)
				}
			}
		})
		writeJSON(w, 200, v)
	})
	s.handle("PUT /api/me/coverage", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var c cad.WorkCoverage
		if !readJSON(w, r, &c) {
			return
		}
		if err := s.Eng.SetCoverage(a, c); err != nil {
			writeErr(w, err)
			return
		}
		s.Eng.Read(func(st *cad.State) { c = st.CoverageOf(se.Username, se.Console, se.Duty) })
		writeJSON(w, 200, c)
	})
	s.handle("PUT /api/admin/agencies/{code}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var g cad.Agency
		if !readJSON(w, r, &g) {
			return
		}
		g.Code = strings.ToUpper(r.PathValue("code"))
		var refuse bool
		s.Eng.Read(func(st *cad.State) {
			refuse = !st.AllAgencies(se.Username) && (st.Agencies[g.Code] == nil || !st.MayWork(se.Username, g.Code))
		})
		if refuse {
			fail(w, http.StatusForbidden, "you may change only the agencies you work for")
			return
		}
		if err := s.Eng.SaveAgency(a, g); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("DELETE /api/admin/agencies/{code}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var refuse bool
		s.Eng.Read(func(st *cad.State) { refuse = !st.AllAgencies(se.Username) })
		if refuse {
			fail(w, http.StatusForbidden, "a person limited to some agencies cannot remove one")
			return
		}
		if err := s.Eng.RemoveAgency(a, strings.ToUpper(r.PathValue("code"))); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// People's profiles: the agencies they may work, their usual coverage, trainee, roster entry.
	s.handle("GET /api/admin/staff", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		type row struct {
			auth.Public
			Staff cad.Staff `json:"staff"`
		}
		out := []row{}
		users := s.Auth.List()
		s.Eng.Read(func(st *cad.State) {
			for _, u := range users {
				out = append(out, row{Public: u, Staff: st.StaffOf(u.Username)})
			}
		})
		writeJSON(w, 200, map[string]any{"items": out})
	})
	s.handle("PUT /api/admin/staff/{username}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var p cad.Staff
		if !readJSON(w, r, &p) {
			return
		}
		p.Username = strings.ToLower(r.PathValue("username"))
		known := slices.ContainsFunc(s.Auth.List(), func(u auth.Public) bool { return u.Username == p.Username })
		if !known {
			fail(w, http.StatusNotFound, "no such user")
			return
		}
		var refuse bool
		s.Eng.Read(func(st *cad.State) { refuse = !st.AllAgencies(se.Username) })
		if refuse {
			fail(w, http.StatusForbidden, "a person limited to some agencies cannot change people's agencies")
			return
		}
		if err := s.Eng.SaveStaff(a, p); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("PUT /api/incidents/{n}/agency", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Agency    string   `json:"agency"`
			Assisting []string `json:"assisting"`
			Reason    string   `json:"reason"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		if err := s.Eng.SetIncidentAgency(a, r.PathValue("n"), b.Agency, b.Assisting, b.Reason); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// ---- presets ----
	type presetOut struct {
		Preset  *cad.Preset `json:"preset"`
		Source  string      `json:"source"` // own, assigned or ""
		MayEdit bool        `json:"mayEdit"`
		Why     string      `json:"why,omitempty"`
		Trainee bool        `json:"trainee"`
	}
	s.handle("GET /api/me/preset", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		duty := r.URL.Query().Get("duty")
		if duty == "" {
			duty = se.Duty
		}
		var out presetOut
		s.Eng.Read(func(st *cad.State) {
			out.Preset, out.Source = st.PresetOf(se.Username, duty)
			out.MayEdit, out.Why = st.MayEditPreset(se.Username, se.Role, se.Username, duty)
			out.Trainee = st.StaffOf(se.Username).Trainee
		})
		writeJSON(w, 200, out)
	})
	s.handle("PUT /api/me/preset", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var p cad.Preset
		if !readJSON(w, r, &p) {
			return
		}
		p.User = se.Username
		if p.Duty == "" {
			p.Duty = se.Duty
		}
		if err := s.Eng.SavePreset(a, p); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("DELETE /api/me/preset", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		duty := r.URL.Query().Get("duty")
		if duty == "" {
			duty = se.Duty
		}
		if err := s.Eng.ClearPreset(a, se.Username, duty); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("GET /api/admin/presets", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out := []*cad.Preset{}
		s.Eng.Read(func(st *cad.State) {
			for _, p := range st.Presets {
				out = append(out, p)
			}
		})
		slices.SortFunc(out, func(x, y *cad.Preset) int { return strings.Compare(x.User+"|"+x.Duty, y.User+"|"+y.Duty) })
		writeJSON(w, 200, map[string]any{"items": out, "views": cad.SupervisoryViews})
	})
	s.handle("PUT /api/admin/presets/{user}/{duty}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var p cad.Preset
		if !readJSON(w, r, &p) {
			return
		}
		p.User, p.Duty = strings.ToLower(r.PathValue("user")), r.PathValue("duty")
		if p.User == "assigned" {
			p.User = cad.AssignedUser
		}
		if len(p.Workspace) == 0 { // protecting or unprotecting keeps what was saved
			s.Eng.Read(func(st *cad.State) {
				if old := st.Presets[strings.ToLower(p.User)+"|"+p.Duty]; old != nil {
					p.Workspace, p.Screens, p.Monitors = old.Workspace, old.Screens, old.Monitors
				}
			})
		}
		if err := s.Eng.SavePreset(a, p); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// Key profiles: the one this duty uses, the stock ones, and a check for conflicts.
	s.handle("GET /api/me/keys", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var p cad.KeyProfile
		var locked bool
		s.Eng.Read(func(st *cad.State) { p, locked = st.Settings.KeyProfileFor(se.Duty), st.StaffOf(se.Username).Trainee })
		writeJSON(w, 200, map[string]any{"profile": p, "actions": cad.KeyActions, "locked": locked})
	})
	s.handle("GET /api/admin/keys", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		writeJSON(w, 200, map[string]any{"stock": []cad.KeyProfile{cad.StandardKeys(), cad.ClassicKeys()}, "actions": cad.KeyActions, "statusCodes": cad.DefaultStatusCodes()})
	})
	s.handle("POST /api/admin/keys/check", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var p cad.KeyProfile
		if !readJSON(w, r, &p) {
			return
		}
		writeJSON(w, 200, map[string]any{"problems": orEmpty(cad.KeyConflicts(p))})
	})
	s.handle("GET /api/me/views", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out := []cad.ViewDef{}
		s.Eng.Read(func(st *cad.State) {
			for _, v := range cad.SupervisoryViews {
				if st.MaySeeView(se.Username, se.Role, v.ID) {
					out = append(out, v)
				}
			}
		})
		writeJSON(w, 200, map[string]any{"items": out, "all": cad.SupervisoryViews})
	})
}
