package api

// Responder duty, unit staffing and the personnel skills file (cad/staffing.go, cad/quals.go). Operational
// capability (who is on duty, on which unit, qualified for what now) is visible to dispatchers and crews;
// credential details (issuer, dates, attachments, verification) only to supervisors and administrators,
// and to the person themselves.

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// PersonBrief is a person as the staffing search shows them.
type PersonBrief struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Rank         string           `json:"rank,omitempty"`
	Agency       string           `json:"agency,omitempty"`
	Station      string           `json:"station,omitempty"`
	RadioID      string           `json:"radioId,omitempty"`
	OnDuty       bool             `json:"onDuty"`
	Duty         *cad.DutyPeriod  `json:"duty,omitempty"`
	Units        []string         `json:"units,omitempty"`
	Quals        []string         `json:"quals"`           // valid now
	Creds        []cad.Credential `json:"creds,omitempty"` // details: supervisors and the person only
	CredProblems []string         `json:"credProblems,omitempty"`
}

func mayPersonnel(se *auth.Session) bool { return auth.AtLeast(se.Role, auth.Supervisor) }

func (s *Server) personBrief(st *cad.State, m *cad.Member, se *auth.Session, now time.Time) PersonBrief {
	p := PersonBrief{ID: m.ID, Name: m.Name, Rank: m.Rank, Agency: m.Agency, Station: m.Station, RadioID: m.RadioID, OnDuty: m.Duty != nil, Duty: m.Duty, Quals: st.ValidQuals(m, now)}
	for _, u := range st.Units {
		if slices.ContainsFunc(u.Crew, func(c cad.CrewSeat) bool { return c.Member == m.ID }) {
			p.Units = append(p.Units, u.ID)
		}
	}
	sort.Strings(p.Units)
	if mayPersonnel(se) || st.StaffOf(se.Username).Member == m.ID {
		p.Creds = m.Creds
		for _, c := range m.Creds {
			if ok, why := st.CredValid(c, now); !ok {
				p.CredProblems = append(p.CredProblems, c.Qual+": "+why)
			}
		}
	}
	return p
}

func (s *Server) staffingRoutes() {
	// Who is on duty and the search: by name, qualification, agency, station, unit, availability.
	s.handle("GET /api/staffing/people", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		q := r.URL.Query()
		text, qual, ag, station, unit, avail := strings.ToLower(q.Get("q")), strings.ToLower(q.Get("qual")), strings.ToUpper(q.Get("agency")), strings.ToUpper(q.Get("station")), strings.ToUpper(q.Get("unit")), q.Get("availability")
		onDuty := q.Get("onduty") == "1" || q.Get("onduty") == "true"
		now := s.Eng.Now()
		out := []PersonBrief{}
		s.Eng.Read(func(st *cad.State) {
			for _, m := range st.Roster() {
				if m.Inactive || onDuty && m.Duty == nil || ag != "" && m.Agency != ag || station != "" && m.Station != station {
					continue
				}
				if text != "" && !strings.Contains(strings.ToLower(m.Name+" "+m.ID+" "+m.RadioID), text) {
					continue
				}
				if avail != "" && (m.Duty == nil || m.Duty.Availability != avail) {
					continue
				}
				p := s.personBrief(st, m, se, now)
				if qual != "" && !slices.Contains(p.Quals, qual) {
					continue
				}
				if unit != "" && !slices.Contains(p.Units, unit) {
					continue
				}
				out = append(out, p)
			}
		})
		writeJSON(w, 200, map[string]any{"items": out})
	})
	// The supervisors' coverage view: minimum complements, expiring credentials, each unit's capability.
	s.handle("GET /api/staffing/coverage", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		now := s.Eng.Now()
		type unitCap struct {
			Unit       string             `json:"unit"`
			Agency     string             `json:"agency,omitempty"`
			Status     string             `json:"status"`
			Crew       []cad.CrewSeat     `json:"crew"`
			Capability cad.UnitCapability `json:"capability"`
			Override   string             `json:"override,omitempty"`
		}
		var gaps []cad.CoverageGap
		var exp []cad.ExpiringCredential
		units := []unitCap{}
		var reqs []*cad.CrewRequest
		s.Eng.Read(func(st *cad.State) {
			gaps, exp = st.StaffCoverage(now), st.Expiring(now)
			for _, u := range st.Units {
				units = append(units, unitCap{Unit: u.ID, Agency: u.Agency, Status: u.Status, Crew: u.Crew, Capability: st.CapabilityOf(u), Override: u.StaffOverride})
			}
			for _, x := range st.CrewRequests {
				if x.Decided == "" {
					reqs = append(reqs, x)
				}
			}
		})
		sort.Slice(units, func(i, j int) bool { return unitLess(units[i].Unit, units[j].Unit) })
		sort.Slice(reqs, func(i, j int) bool { return reqs[i].At.Before(reqs[j].At) })
		if reqs == nil {
			reqs = []*cad.CrewRequest{}
		}
		writeJSON(w, 200, map[string]any{"gaps": gaps, "expiring": exp, "units": units, "requests": reqs})
	})
	s.handle("POST /api/members/{id}/on", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var d cad.DutyPeriod
		if !readJSON(w, r, &d) {
			return
		}
		done(w, s.Eng.SignOn(a, r.PathValue("id"), d))
	})
	s.handle("POST /api/members/{id}/off", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Reason    string    `json:"reason"`
			Override  string    `json:"override"`
			Effective time.Time `json:"effective"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.SignOff(a, r.PathValue("id"), b.Reason, b.Override, b.Effective))
	})
	s.handle("PUT /api/members/{id}/availability", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct{ Availability, Note string }
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.SetAvailability(a, r.PathValue("id"), b.Availability, b.Note))
	})
	s.handle("POST /api/units/{id}/crew/correct", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Incident  string         `json:"incident"`
			Crew      []cad.CrewSeat `json:"crew"`
			Effective time.Time      `json:"effective"`
			Reason    string         `json:"reason"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.CorrectCrew(a, r.PathValue("id"), b.Incident, b.Crew, b.Effective, b.Reason))
	})
	s.handle("POST /api/crewrequests/{id}", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Approve bool   `json:"approve"`
			Why     string `json:"why"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.DecideCrew(a, r.PathValue("id"), b.Approve, b.Why))
	})
	s.handle("GET /api/crewtemplates", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out := []*cad.CrewTemplate{}
		s.Eng.Read(func(st *cad.State) {
			for _, t := range st.CrewTemplates {
				out = append(out, t)
			}
		})
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		writeJSON(w, 200, map[string]any{"items": out})
	})
	s.handle("PUT /api/crewtemplates", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var t cad.CrewTemplate
		if !readJSON(w, r, &t) {
			return
		}
		id, err := s.Eng.SaveCrewTemplate(a, t)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("DELETE /api/crewtemplates/{id}", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		done(w, s.Eng.RemoveCrewTemplate(a, r.PathValue("id")))
	})
	s.handle("POST /api/staffing/shift", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Templates []string                  `json:"templates"`
			Crews     map[string][]cad.CrewSeat `json:"crews"`
			Effective time.Time                 `json:"effective"`
			Reason    string                    `json:"reason"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		n, err := s.Eng.ShiftChange(a, b.Crews, b.Templates, b.Effective, b.Reason)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]int{"units": n})
	})

	// ---- qualifications ----
	s.handle("GET /api/quals", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out := []*cad.QualDef{}
		eqs := []*cad.Equivalency{}
		s.Eng.Read(func(st *cad.State) {
			for _, q := range st.Quals {
				out = append(out, q)
			}
			for _, q := range st.Equivalencies {
				eqs = append(eqs, q)
			}
		})
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		sort.Slice(eqs, func(i, j int) bool { return eqs[i].ID < eqs[j].ID })
		writeJSON(w, 200, map[string]any{"items": out, "equivalencies": eqs, "categories": cad.QualCategories, "states": cad.CredStates})
	})
	s.handle("PUT /api/admin/quals/{code}", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var q cad.QualDef
		if !readJSON(w, r, &q) {
			return
		}
		q.Code = r.PathValue("code")
		if !s.agencyMay(w, se, q.Agency) {
			return
		}
		done(w, s.Eng.SaveQual(a, q))
	})
	s.handle("DELETE /api/admin/quals/{code}", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var ag string
		s.Eng.Read(func(st *cad.State) {
			if q := st.Quals[r.PathValue("code")]; q != nil {
				ag = q.Agency
			}
		})
		if !s.agencyMay(w, se, ag) {
			return
		}
		done(w, s.Eng.RemoveQual(a, r.PathValue("code")))
	})
	s.handle("PUT /api/admin/equivalencies", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var q cad.Equivalency
		if !readJSON(w, r, &q) {
			return
		}
		if !s.agencyMay(w, se, q.Agency) {
			return
		}
		done(w, s.Eng.SaveEquivalency(a, q))
	})
	s.handle("DELETE /api/admin/equivalencies/{id}", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		done(w, s.Eng.RemoveEquivalency(a, r.PathValue("id")))
	})
	// A credential: supervisors keep anyone's; a person may enter their own, unverified.
	s.handle("PUT /api/members/{id}/credentials/{qual}", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var c cad.Credential
		if !readJSON(w, r, &c) {
			return
		}
		c.Qual = r.PathValue("qual")
		id := r.PathValue("id")
		if !mayPersonnel(se) {
			var own bool
			s.Eng.Read(func(st *cad.State) { own = st.StaffOf(se.Username).Member == id })
			if !own {
				fail(w, http.StatusForbidden, "a supervisor keeps other people's credentials")
				return
			}
		}
		done(w, s.Eng.SaveCredential(a, id, c))
	})
	s.handle("DELETE /api/members/{id}/credentials/{qual}", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		done(w, s.Eng.EndCredential(a, r.PathValue("id"), r.PathValue("qual"), r.URL.Query().Get("reason")))
	})
	s.handle("GET /api/members/{id}/file", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		id := r.PathValue("id")
		var p *PersonBrief
		allowed := mayPersonnel(se)
		s.Eng.Read(func(st *cad.State) {
			if !allowed && st.StaffOf(se.Username).Member != id {
				return
			}
			allowed = true
			if m := st.Members[id]; m != nil {
				x := s.personBrief(st, m, se, s.Eng.Now())
				p = &x
			}
		})
		if !allowed {
			fail(w, http.StatusForbidden, "a person's skills file is for supervisors and the person")
			return
		}
		if p == nil {
			fail(w, 404, "no such member")
			return
		}
		writeJSON(w, 200, p)
	})
}

// agencyMay checks that a person may work an agency's records (an agency of "" is the centre's: only for
// people not limited to some agencies).
func (s *Server) agencyMay(w http.ResponseWriter, se *auth.Session, agency string) bool {
	var ok bool
	s.Eng.Read(func(st *cad.State) {
		ok = st.AllAgencies(se.Username) || agency != "" && st.MayWork(se.Username, strings.ToUpper(agency))
	})
	if !ok {
		fail(w, http.StatusForbidden, "that belongs to an agency you do not work for")
	}
	return ok
}

// credentialReminders tells supervisors, once each, of credentials whose renewal came due or that expired.
func (s *Server) credentialReminders(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if rem, err := s.Eng.RemindCredentials(); err == nil && len(rem) > 0 {
			var lines []string
			for _, x := range rem {
				what := "renewal due by " + x.Expires.Format("2006-01-02")
				if x.Expired {
					what = "EXPIRED " + x.Expires.Format("2006-01-02")
				}
				lines = append(lines, fmt.Sprintf("%s (%s): %s %s", x.Name, x.Member, x.Qual, what))
			}
			s.Eng.SendComm(cad.SystemActor, cad.Comm{Kind: "admin", Priority: "medium", Subject: fmt.Sprintf("Credentials: %d need attention", len(rem)),
				Body: strings.Join(lines, "\n"), To: []string{cad.ToRole + "supervisor"}})
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
