package api

// The roadmap features' API: alarms, crews and the roster, fire-ground accountability, paging (and the
// public page a responder answers on), tow rotation, the schedule, the contacts directory, areas on the
// map, unit status history, patient-care reports (NEMSIS) and warrants and civil papers.

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/nemsis"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
	"github.com/biodream-llc/bitdispatcher/internal/paging"
)

// Paging is how pages go out (Admin › Integrations › Paging): the gateways, and the address responders
// open their answer link at.
type Paging struct {
	Gateways  []paging.Gateway `json:"gateways"`
	PublicURL string           `json:"publicUrl,omitempty"` // https://cad.example.gov: answer links; empty: none
}

// RoadmapView is what the console shows of the roadmap features.
type RoadmapView struct {
	Members      []MemberBrief             `json:"members"`
	PageGroups   []*cad.PageGroup          `json:"pageGroups"`
	Gateways     []GatewayBrief            `json:"gateways"`
	Pages        []*cad.Page               `json:"pages"`
	TowCompanies []*cad.TowCompany         `json:"towCompanies"`
	Tows         []*cad.Tow                `json:"tows"`
	TowClasses   []string                  `json:"towClasses"`
	Upcoming     []cad.UpcomingItem        `json:"upcoming"`
	CallOut      map[string][]cad.CallOut  `json:"callOut"`
	Geofences    []*cad.Geofence           `json:"geofences"`
	FenceAlerts  []cad.FenceAlert          `json:"fenceAlerts"`
	Cautions     map[string][]CautionBrief `json:"cautions"`
	Papers       map[string]PaperFlag      `json:"papers"`
	Benchmarks   []cad.Benchmark           `json:"benchmarks"`
	Alarms       map[string]AlarmBrief     `json:"alarms"` // by incident: its level and the next
	Staffing     map[string][]string       `json:"staffing"`
	EPCR         bool                      `json:"epcr"` // patient-care reports go to an ePCR system
	PARDue       map[string]time.Time      `json:"parDue"`
	Responders   map[string]ResponderBrief `json:"responders"` // by incident: who answered its pages
	OnCallNow    []OnCallBrief             `json:"onCallNow"`
	PublicAlerts []PublicAlertBrief        `json:"publicAlerts"` // in force, or waiting for approval
}

// PublicAlertBrief is a public alert as every console sees it: what it says and where, not who it went to.
type PublicAlertBrief struct {
	ID         string        `json:"id"`
	State      string        `json:"state"`
	Status     string        `json:"status"`
	MsgType    string        `json:"msgType"`
	Event      string        `json:"event"`
	EventName  string        `json:"eventName"`
	Headline   string        `json:"headline"`
	Short      string        `json:"short,omitempty"`
	Area       cad.AlertArea `json:"area"`
	Expires    time.Time     `json:"expires"`
	Sent       time.Time     `json:"sent,omitzero"`
	By         string        `json:"by"`
	ApprovedBy string        `json:"approvedBy,omitempty"`
	Incident   string        `json:"incident,omitempty"`
}

// MemberBrief is a member as every console sees them: no phone or email.
type MemberBrief struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Rank      string   `json:"rank,omitempty"`
	Quals     []string `json:"quals,omitempty"`
	Station   string   `json:"station,omitempty"`
	Volunteer bool     `json:"volunteer,omitempty"`
	Inactive  bool     `json:"inactive,omitempty"`
	Riding    string   `json:"riding,omitempty"`
}

// GatewayBrief is a paging gateway without its settings.
type GatewayBrief struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// CautionBrief is a caution area an incident is in.
type CautionBrief struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Note string `json:"note"`
}

// PaperFlag says an incident is at the address of active warrants or papers: how many, the kinds and any
// caution. The papers themselves are for police and records (GET /api/incidents/{n}/papers).
type PaperFlag struct {
	Count   int      `json:"count"`
	Kinds   []string `json:"kinds"`
	Caution string   `json:"caution,omitempty"`
}

// AlarmBrief is an incident's alarm level and what upgrading adds.
type AlarmBrief struct {
	Level    int               `json:"level"`
	Name     string            `json:"name"`
	NextName string            `json:"nextName,omitempty"`
	NextAdds []cad.Requirement `json:"nextAdds,omitempty"`
}

// ResponderBrief counts who answered an incident's pages.
type ResponderBrief struct {
	Paged       int             `json:"paged"`
	Responding  int             `json:"responding"`
	ToScene     int             `json:"toScene"`
	Unavailable int             `json:"unavailable"`
	Replies     []cad.PageReply `json:"replies"` // soonest arrival first
}

// OnCallBrief is who is on call now on a list.
type OnCallBrief struct {
	List   string        `json:"list"`
	ID     string        `json:"id"`
	Entry  *cad.DirEntry `json:"entry,omitempty"`
	Until  time.Time     `json:"until,omitzero"`
	Backup bool          `json:"backup,omitempty"`
}

func (s *Server) pagingConfig() Paging {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.integ.Paging
}

func (s *Server) nemsisConfig() nemsis.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.integ.NEMSIS
}

func (s *Server) roadmapView(st *cad.State, now time.Time, open []*cad.Incident) RoadmapView {
	v := RoadmapView{Members: []MemberBrief{}, PageGroups: []*cad.PageGroup{}, Gateways: []GatewayBrief{}, Pages: []*cad.Page{},
		TowCompanies: []*cad.TowCompany{}, Tows: st.OpenTows(now), TowClasses: st.Settings.TowClassList(), Upcoming: st.Upcoming(now, 24*time.Hour),
		CallOut: map[string][]cad.CallOut{}, Geofences: []*cad.Geofence{}, FenceAlerts: []cad.FenceAlert{}, Cautions: map[string][]CautionBrief{},
		Papers: map[string]PaperFlag{}, Benchmarks: st.Settings.BenchmarkList(), Alarms: map[string]AlarmBrief{}, Staffing: map[string][]string{},
		PARDue: map[string]time.Time{}, Responders: map[string]ResponderBrief{}, OnCallNow: []OnCallBrief{}, PublicAlerts: []PublicAlertBrief{}}
	for i := len(st.Alerts) - 1; i >= 0; i-- {
		p := st.Alerts[i]
		if p.InForce(now) || p.State == cad.AlertPending && p.Expires.After(now) {
			v.PublicAlerts = append(v.PublicAlerts, PublicAlertBrief{ID: p.ID, State: p.State, Status: p.Status, MsgType: p.MsgType, Event: p.Event,
				EventName: p.EventName, Headline: p.Text.Headline, Short: p.Text.Short, Area: p.Area, Expires: p.Expires, Sent: p.Sent, By: p.By,
				ApprovedBy: p.ApprovedBy, Incident: p.Incident})
		}
	}
	if v.Tows == nil {
		v.Tows = []*cad.Tow{}
	}
	if v.Upcoming == nil {
		v.Upcoming = []cad.UpcomingItem{}
	}
	for _, m := range st.Roster() {
		v.Members = append(v.Members, MemberBrief{ID: m.ID, Name: m.Name, Rank: m.Rank, Quals: m.Quals, Station: m.Station, Volunteer: m.Volunteer, Inactive: m.Inactive, Riding: st.RidingOn(m.ID)})
	}
	for _, g := range st.PageGroups {
		v.PageGroups = append(v.PageGroups, g)
	}
	sort.Slice(v.PageGroups, func(i, j int) bool { return v.PageGroups[i].Name < v.PageGroups[j].Name })
	s.mu.Lock()
	for _, g := range s.integ.Paging.Gateways {
		v.Gateways = append(v.Gateways, GatewayBrief{ID: g.ID, Name: g.Name, Kind: g.Kind})
	}
	v.EPCR = s.integ.NEMSIS.On
	s.mu.Unlock()
	for _, p := range st.Pages {
		if now.Sub(p.At) < 24*time.Hour {
			v.Pages = append(v.Pages, p)
		}
	}
	for _, c := range st.TowCompanies {
		v.TowCompanies = append(v.TowCompanies, c)
	}
	sort.Slice(v.TowCompanies, func(i, j int) bool { return v.TowCompanies[i].Name < v.TowCompanies[j].Name })
	for _, g := range st.Geofences {
		if !g.Off {
			v.Geofences = append(v.Geofences, g)
		}
	}
	sort.Slice(v.Geofences, func(i, j int) bool { return v.Geofences[i].Name < v.Geofences[j].Name })
	for _, a := range st.FenceAlerts {
		if now.Sub(a.At) < 24*time.Hour {
			v.FenceAlerts = append(v.FenceAlerts, a)
		}
	}
	for _, u := range st.Units {
		if short := st.Staffing(u); len(short) > 0 {
			v.Staffing[u.ID] = short
		}
	}
	for _, l := range st.OnCall {
		d, until, backup := st.OnCallNow(l, now)
		v.OnCallNow = append(v.OnCallNow, OnCallBrief{List: l.Name, ID: l.ID, Entry: d, Until: until, Backup: backup})
	}
	sort.Slice(v.OnCallNow, func(i, j int) bool { return v.OnCallNow[i].List < v.OnCallNow[j].List })
	for _, in := range open {
		if in.Status == cad.Closed {
			continue
		}
		if co := st.CallOutFor(in, now); len(co) > 0 {
			v.CallOut[in.Number] = co
		}
		if in.Location.HasPoint() {
			for _, g := range st.CautionsAt(*in.Location.Lat, *in.Location.Lon) {
				v.Cautions[in.Number] = append(v.Cautions[in.Number], CautionBrief{ID: g.ID, Name: g.Name, Note: g.Note})
			}
		}
		ps := st.PapersNear(in.Location)
		var names []string
		for _, p := range in.Persons {
			names = append(names, p.Name)
		}
		for _, p := range st.PapersNamed(names) {
			if !slices.Contains(ps, p) {
				ps = append(ps, p)
			}
		}
		if len(ps) > 0 {
			f := PaperFlag{Count: len(ps)}
			var cautions []string
			for _, p := range ps {
				if !slices.Contains(f.Kinds, p.Kind) {
					f.Kinds = append(f.Kinds, p.Kind)
				}
				if p.Caution != "" && !slices.Contains(cautions, p.Caution) {
					cautions = append(cautions, p.Caution)
				}
			}
			f.Caution = strings.Join(cautions, "; ")
			v.Papers[in.Number] = f
		}
		ab := AlarmBrief{Level: in.AlarmLevel(), Name: st.AlarmName(in)}
		if n, adds, ok := st.NextAlarm(in); ok {
			ab.NextName, ab.NextAdds = n, adds
		}
		if ab.Level > 1 || ab.NextName != "" {
			v.Alarms[in.Number] = ab
		}
		if due, ok := in.Fireground.Due(); ok {
			v.PARDue[in.Number] = due
		}
		if pages := st.PagesFor(in.Number); len(pages) > 0 {
			rb := ResponderBrief{Replies: []cad.PageReply{}}
			seen := map[string]bool{}
			for _, p := range pages {
				for _, t := range p.To {
					if !seen[t.Member] {
						seen[t.Member] = true
						rb.Paged++
					}
				}
			}
			latest := map[string]cad.PageReply{}
			for _, p := range pages {
				for m, r := range p.Replies {
					if old, ok := latest[m]; !ok || r.At.After(old.At) {
						latest[m] = r
					}
				}
			}
			for _, r := range latest {
				switch r.Answer {
				case cad.PageResponding:
					rb.Responding++
				case cad.PageToScene:
					rb.ToScene++
				case cad.PageUnavailable:
					rb.Unavailable++
				}
				rb.Replies = append(rb.Replies, r)
			}
			sort.Slice(rb.Replies, func(i, j int) bool {
				a, b := rb.Replies[i], rb.Replies[j]
				if (a.Answer == cad.PageUnavailable) != (b.Answer == cad.PageUnavailable) {
					return b.Answer == cad.PageUnavailable
				}
				return a.At.Add(time.Duration(a.ETA) * time.Minute).Before(b.At.Add(time.Duration(b.ETA) * time.Minute))
			})
			v.Responders[in.Number] = rb
		}
	}
	return v
}

// mayPapers is whether a session may see and work warrants and civil papers: police and general
// dispatchers, call-takers, records staff, supervisors and administrators.
func mayPapers(se *auth.Session) bool {
	if auth.AtLeast(se.Role, auth.Supervisor) {
		return true
	}
	switch se.Duty {
	case auth.DutyPolice, auth.DutyDispatcher, auth.DutyCallTaker, auth.DutyRecords, "":
		return auth.AtLeast(se.Role, auth.Dispatcher)
	}
	return false
}

func (s *Server) roadmapRoutes() {
	// ---- 1. alarm levels ----
	s.handle("POST /api/incidents/{n}/alarm", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Level  int    `json:"level"`
			Reason string `json:"reason"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		var err error
		if b.Level == 0 {
			err = s.Eng.UpgradeAlarm(a, r.PathValue("n"), b.Reason)
		} else {
			err = s.Eng.SetAlarm(a, r.PathValue("n"), b.Level, b.Reason)
		}
		if err != nil {
			writeErr(w, err)
			return
		}
		rec, err := s.Eng.Recommend(r.PathValue("n"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, rec)
	})

	// ---- 2. crews and the roster ----
	s.handle("GET /api/roster", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		type row struct {
			*cad.Member
			Riding string `json:"riding,omitempty"`
		}
		var out []row
		var quals []string
		var needs []cad.CapNeed
		s.Eng.Read(func(st *cad.State) {
			for _, m := range st.Roster() {
				c := *m
				if !auth.AtLeast(se.Role, auth.Supervisor) {
					c.Phone, c.Email, c.Topic = mask(c.Phone), mask(c.Email), ""
				}
				out = append(out, row{Member: &c, Riding: st.RidingOn(m.ID)})
			}
			quals = st.Settings.Qualifications
			needs = st.Settings.CapNeeds
			if needs == nil {
				needs = cad.DefaultCapNeeds()
			}
		})
		if out == nil {
			out = []row{}
		}
		writeJSON(w, 200, map[string]any{"members": out, "qualifications": quals, "capNeeds": needs})
	})
	s.handle("PUT /api/admin/members", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var m cad.Member
		if !readJSON(w, r, &m) {
			return
		}
		if err := s.Eng.SaveMember(a, m); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("DELETE /api/admin/members/{id}", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if err := s.Eng.RemoveMember(a, r.PathValue("id")); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// A roster from a spreadsheet: id,name,rank,qualifications (separated by ;),station,phone,email,volunteer.
	s.handle("POST /api/admin/members/import", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		rd := csv.NewReader(http.MaxBytesReader(w, r.Body, 2<<20))
		rd.FieldsPerRecord = -1
		rows, err := rd.ReadAll()
		if err != nil {
			fail(w, 400, "not a CSV file: "+err.Error())
			return
		}
		n, problems := 0, []string{}
		for i, row := range rows {
			if len(row) < 2 || (i == 0 && strings.EqualFold(strings.TrimSpace(row[0]), "id")) {
				continue
			}
			col := func(k int) string {
				if k < len(row) {
					return strings.TrimSpace(row[k])
				}
				return ""
			}
			m := cad.Member{ID: col(0), Name: col(1), Rank: col(2), Quals: strings.Split(col(3), ";"), Station: col(4), Phone: col(5), Email: col(6),
				Volunteer: strings.EqualFold(col(7), "yes") || strings.EqualFold(col(7), "true") || col(7) == "1"}
			if err := s.Eng.SaveMember(a, m); err != nil {
				problems = append(problems, fmt.Sprintf("line %d: %v", i+1, err))
				continue
			}
			n++
		}
		writeJSON(w, 200, map[string]any{"saved": n, "problems": problems})
	})
	s.handle("PUT /api/units/{id}/crew", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Crew []cad.CrewSeat `json:"crew"`
			Note string         `json:"note"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		if err := s.Eng.SetCrew(a, r.PathValue("id"), b.Crew, b.Note); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("PUT /api/mdt/crew", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		unit, ok := s.mdtUnit(w, r, se)
		if !ok {
			return
		}
		var b struct {
			Crew []cad.CrewSeat `json:"crew"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		if err := s.Eng.SetCrew(a, unit, b.Crew, "from the terminal"); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// ---- 4. fire-ground accountability ----
	s.handle("POST /api/incidents/{n}/par/start", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Minutes int `json:"minutes"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.StartFireground(a, r.PathValue("n"), b.Minutes))
	})
	s.handle("POST /api/incidents/{n}/par", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			OK      []string       `json:"ok"`
			Missing []string       `json:"missing"`
			PAX     map[string]int `json:"pax"`
			Note    string         `json:"note"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.RecordPAR(a, r.PathValue("n"), b.OK, b.Missing, b.PAX, b.Note))
	})
	s.handle("POST /api/incidents/{n}/par/found", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Unit string `json:"unit"`
			Note string `json:"note"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.FoundUnit(a, r.PathValue("n"), b.Unit, b.Note))
	})
	s.handle("POST /api/incidents/{n}/par/end", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		done(w, s.Eng.EndFireground(a, r.PathValue("n")))
	})
	s.handle("POST /api/incidents/{n}/benchmark", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Code string `json:"code"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.MarkBenchmark(a, r.PathValue("n"), b.Code))
	})

	// ---- 3. paging ----
	s.handle("GET /api/admin/paging", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		writeJSON(w, 200, s.pagingView())
	})
	s.handle("PUT /api/admin/paging", auth.Admin, s.savePaging)
	s.handle("POST /api/admin/paging/test", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Gateway string `json:"gateway"`
			Member  string `json:"member"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		var g *paging.Gateway
		for _, x := range s.pagingConfig().Gateways {
			if x.ID == b.Gateway {
				g = &x
			}
		}
		if g == nil {
			fail(w, 404, "no paging gateway "+b.Gateway)
			return
		}
		var to []paging.Recipient
		s.Eng.Read(func(st *cad.State) {
			if m := st.Members[b.Member]; m != nil {
				to = append(to, recipient(*m))
			}
		})
		res := s.pageSender().Send(r.Context(), *g, paging.Page{ID: "TEST", Subject: "TEST PAGE from bitdispatcher", Text: "A test of the " + g.Name + " paging gateway. No response is needed.", At: time.Now()}, to)
		writeJSON(w, 200, map[string]any{"results": res})
	})
	s.handle("PUT /api/admin/pagegroups", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var g cad.PageGroup
		if !readJSON(w, r, &g) {
			return
		}
		known := map[string]bool{}
		for _, x := range s.pagingConfig().Gateways {
			known[x.ID] = true
		}
		for _, id := range g.Gateways {
			if !known[id] {
				fail(w, 422, "no paging gateway "+id+" (Admin › Integrations › Paging)")
				return
			}
		}
		id, err := s.Eng.SavePageGroup(a, g)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("DELETE /api/admin/pagegroups/{id}", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		done(w, s.Eng.RemovePageGroup(a, r.PathValue("id")))
	})
	s.handle("POST /api/pages", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Incident string   `json:"incident"`
			Groups   []string `json:"groups"`
			Members  []string `json:"members"`
			Subject  string   `json:"subject"`
			Text     string   `json:"text"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		id, to, err := s.Eng.SendPage(a, b.Incident, b.Groups, b.Members, b.Subject, b.Text, false)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"id": id, "people": len(to)})
	})
	// A responder's answer link: no sign-in, the token in the link is the key.
	s.mux.HandleFunc("GET /api/page-reply/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, who, ok := s.Eng.PageFor(r.PathValue("id"), r.URL.Query().Get("t"))
		if !ok {
			fail(w, 404, "this page link is not valid")
			return
		}
		var addr string
		var answer *cad.PageReply
		s.Eng.Read(func(st *cad.State) {
			if in := st.Incidents[p.Incident]; in != nil && in.Location != nil {
				addr = in.Location.Address
			}
			if pp := st.Pages; pp != nil {
				for _, x := range pp {
					if x.ID == p.ID {
						if rp, ok := x.Replies[who.Member]; ok {
							answer = &rp
						}
					}
				}
			}
		})
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{"id": p.ID, "at": p.At, "subject": p.Subject, "text": p.Text, "address": addr, "name": who.Name, "answer": answer, "agency": s.agencyName()})
	})
	s.mux.HandleFunc("POST /api/page-reply/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(csrfHeader) == "" {
			fail(w, http.StatusForbidden, "missing "+csrfHeader+" header")
			return
		}
		var b struct {
			Token  string `json:"t"`
			Answer string `json:"answer"`
			ETA    int    `json:"eta"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.ReplyPage(r.PathValue("id"), b.Token, b.Answer, b.ETA))
	})

	// ---- 5. tow rotation ----
	s.handle("GET /api/tows/rotation", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var el, passed []cad.TowTurn
		s.Eng.Read(func(st *cad.State) {
			el, passed = st.TowRotationFor(r.URL.Query().Get("class"), r.URL.Query().Get("zone"))
		})
		if el == nil {
			el = []cad.TowTurn{}
		}
		if passed == nil {
			passed = []cad.TowTurn{}
		}
		writeJSON(w, 200, map[string]any{"eligible": el, "passed": passed})
	})
	s.handle("GET /api/tows", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var out []*cad.Tow
		s.Eng.Read(func(st *cad.State) {
			for i := len(st.Tows) - 1; i >= 0 && len(out) < 500; i-- {
				out = append(out, st.Tows[i])
			}
		})
		if out == nil {
			out = []*cad.Tow{}
		}
		writeJSON(w, 200, map[string]any{"items": out})
	})
	s.handle("PUT /api/admin/tows", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var c cad.TowCompany
		if !readJSON(w, r, &c) {
			return
		}
		id, err := s.Eng.SaveTowCompany(a, c)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("DELETE /api/admin/tows/{id}", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		done(w, s.Eng.RemoveTowCompany(a, r.PathValue("id")))
	})
	s.handle("POST /api/tows", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Incident, Class, Zone, Kind, Company, Vehicle, Note string
		}
		if !readJSON(w, r, &b) {
			return
		}
		t, err := s.Eng.RequestTow(a, b.Incident, b.Class, b.Zone, b.Kind, b.Company, b.Vehicle, b.Note)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, t)
	})
	s.handle("POST /api/tows/{id}/answer", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Result string `json:"result"`
			Note   string `json:"note"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.TowAnswer(a, r.PathValue("id"), b.Result, b.Note))
	})

	// ---- 6. the schedule ----
	s.handle("GET /api/schedule", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		type row struct {
			*cad.Scheduled
			Next time.Time `json:"next,omitzero"`
		}
		now := s.Eng.Now()
		out := []row{}
		s.Eng.Read(func(st *cad.State) {
			for _, x := range st.ScheduleList(now) {
				out = append(out, row{Scheduled: x, Next: x.Next(now)})
			}
		})
		writeJSON(w, 200, map[string]any{"items": out})
	})
	s.handle("PUT /api/schedule", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var x cad.Scheduled
		if !readJSON(w, r, &x) {
			return
		}
		id, err := s.Eng.SaveScheduled(a, x)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("DELETE /api/schedule/{id}", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		done(w, s.Eng.RemoveScheduled(a, r.PathValue("id")))
	})

	// ---- 7. the contacts directory ----
	s.handle("GET /api/directory", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		now := s.Eng.Now()
		var items []*cad.DirEntry
		lists := []map[string]any{}
		s.Eng.Read(func(st *cad.State) {
			items = st.SearchDirectory(r.URL.Query().Get("q"))
			for _, l := range st.OnCall {
				d, until, backup := st.OnCallNow(l, now)
				lists = append(lists, map[string]any{"list": l, "now": d, "until": until, "backup": backup})
			}
		})
		if items == nil {
			items = []*cad.DirEntry{}
		}
		sort.Slice(lists, func(i, j int) bool {
			return lists[i]["list"].(*cad.OnCallList).Name < lists[j]["list"].(*cad.OnCallList).Name
		})
		writeJSON(w, 200, map[string]any{"items": items, "onCall": lists, "categories": cad.DirCategories})
	})
	s.handle("PUT /api/directory", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var d cad.DirEntry
		if !readJSON(w, r, &d) {
			return
		}
		id, err := s.Eng.SaveDirEntry(a, d)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("DELETE /api/directory/{id}", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		done(w, s.Eng.RemoveDirEntry(a, r.PathValue("id")))
	})
	s.handle("PUT /api/oncall", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var l cad.OnCallList
		if !readJSON(w, r, &l) {
			return
		}
		id, err := s.Eng.SaveOnCall(a, l)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("DELETE /api/oncall/{id}", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		done(w, s.Eng.RemoveOnCall(a, r.PathValue("id")))
	})

	// ---- 8. areas on the map ----
	s.handle("PUT /api/admin/geofences", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var g cad.Geofence
		if !readJSON(w, r, &g) {
			return
		}
		id, err := s.Eng.SaveGeofence(a, g)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("GET /api/admin/geofences", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out := []*cad.Geofence{}
		s.Eng.Read(func(st *cad.State) {
			for _, g := range st.Geofences {
				out = append(out, g)
			}
		})
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		writeJSON(w, 200, map[string]any{"items": out})
	})
	s.handle("DELETE /api/admin/geofences/{id}", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		done(w, s.Eng.RemoveGeofence(a, r.PathValue("id")))
	})

	// ---- 9. unit status history ----
	s.handle("GET /api/units/{id}/history", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		q := r.URL.Query()
		to := s.Eng.Now()
		from := to.Add(-24 * time.Hour)
		if v := q.Get("from"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				fail(w, 400, "from is an RFC 3339 time")
				return
			}
			from = t
		}
		if v := q.Get("to"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				fail(w, 400, "to is an RFC 3339 time")
				return
			}
			to = t
		}
		h, err := s.Eng.UnitHistory(r.PathValue("id"), from, to)
		if err != nil {
			writeErr(w, err)
			return
		}
		if q.Get("format") == "csv" {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-status-%s.csv"`, h.Unit, from.Format("2006-01-02")))
			cw := csv.NewWriter(w)
			cw.Write([]string{"unit", "from", "to", "status", "incident", "mark-out", "minutes", "by", "reason"})
			for _, sp := range h.Spans {
				cw.Write([]string{h.Unit, sp.From.Format(time.RFC3339), sp.To.Format(time.RFC3339), sp.Status, sp.Incident, sp.MarkOut, fmt.Sprintf("%.1f", sp.Seconds/60), sp.By, sp.Reason})
			}
			cw.Flush()
			return
		}
		writeJSON(w, 200, h)
	})

	// ---- 10. patient-care reports (NEMSIS) ----
	s.handle("GET /api/admin/nemsis", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		c := s.nemsisConfig()
		writeJSON(w, 200, map[string]any{"on": c.On, "url": c.URL, "tokenSet": c.Token != "", "agency": c.Agency, "agencyStateId": c.AgencyStateID, "state": c.State, "auto": c.Auto, "reasons": nemsis.ReasonCodes(), "version": nemsis.Version})
	})
	s.handle("PUT /api/admin/nemsis", auth.Admin, s.saveNEMSIS)
	s.handle("GET /api/incidents/{n}/nemsis", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		resp, err := s.nemsisResponse(r.PathValue("n"), r.URL.Query().Get("unit"))
		if err != nil {
			writeErr(w, err)
			return
		}
		doc, err := nemsis.Encode(s.nemsisConfig(), resp, s.Version)
		if err != nil {
			writeErr(w, err)
			return
		}
		if r.URL.Query().Get("check") != "" {
			writeJSON(w, 200, map[string]any{"problems": orEmpty(nemsis.Validate(resp))})
			return
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s-nemsis.xml"`, resp.Incident, resp.Unit))
		w.Write(doc)
	})
	s.handle("POST /api/incidents/{n}/nemsis", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Unit string `json:"unit"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		rec, err := s.sendNEMSIS(r.Context(), a, r.PathValue("n"), b.Unit)
		if err != nil && rec == nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, rec)
	})

	// ---- 11. warrants and civil papers ----
	guard := func(h handler) handler {
		return func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
			if !mayPapers(se) {
				fail(w, http.StatusForbidden, "warrants and papers are for police dispatch, records and supervisors")
				return
			}
			h(w, r, se, a)
		}
	}
	s.handle("GET /api/papers", auth.Dispatcher, guard(func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		out := []*cad.Paper{}
		s.Eng.Read(func(st *cad.State) {
			for _, p := range st.PaperList(r.URL.Query().Get("status")) {
				hay := strings.ToLower(p.ID + " " + p.Number + " " + p.Subject.Name + " " + p.Charge + " " + p.Kind)
				if p.Location != nil {
					hay += " " + strings.ToLower(p.Location.Address)
				}
				if q == "" || strings.Contains(hay, q) {
					out = append(out, p)
				}
			}
		})
		s.Eng.Record(a, cad.EvRecordsViewed, map[string]any{"what": "papers", "q": q, "shown": len(out)})
		writeJSON(w, 200, map[string]any{"items": out, "kinds": cad.PaperKinds})
	}))
	s.handle("PUT /api/papers", auth.Dispatcher, guard(func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var p cad.Paper
		if !readJSON(w, r, &p) {
			return
		}
		if p.Location != nil && !p.Location.HasPoint() && p.Location.Address != "" {
			if lat, lon, ok := s.geocode(r.Context(), p.Location.Address); ok {
				p.Location.Lat, p.Location.Lon = &lat, &lon
			}
		}
		id, err := s.Eng.SavePaper(a, p)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	}))
	s.handle("POST /api/papers/{id}/attempt", auth.Dispatcher, guard(func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct{ Unit, Result, Note, Incident string }
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.AttemptPaper(a, r.PathValue("id"), b.Unit, b.Result, b.Note, b.Incident))
	}))
	s.handle("POST /api/papers/{id}/status", auth.Dispatcher, guard(func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct{ Status, Note, Unit string }
		if !readJSON(w, r, &b) {
			return
		}
		done(w, s.Eng.SetPaperStatus(a, r.PathValue("id"), b.Status, b.Note, b.Unit))
	}))
	s.handle("GET /api/incidents/{n}/papers", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		// Field crews of a police unit on the incident see them too.
		allowed := mayPapers(se)
		var out []*cad.Paper
		s.Eng.Read(func(st *cad.State) {
			in := st.Incidents[r.PathValue("n")]
			if in == nil {
				return
			}
			if se.Role == auth.Field {
				if u := st.Units[se.Console]; u != nil && u.Incident == in.Number && st.Discipline(u) == "police" {
					allowed = true
				}
			}
			if !allowed {
				return
			}
			out = st.PapersNear(in.Location)
			var names []string
			for _, p := range in.Persons {
				names = append(names, p.Name)
			}
			for _, p := range st.PapersNamed(names) {
				if !slices.Contains(out, p) {
					out = append(out, p)
				}
			}
		})
		if !allowed {
			fail(w, http.StatusForbidden, "warrants and papers are for police")
			return
		}
		if out == nil {
			out = []*cad.Paper{}
		}
		s.Eng.Record(a, cad.EvRecordsViewed, map[string]any{"what": "papers", "incident": r.PathValue("n"), "shown": len(out)})
		writeJSON(w, 200, map[string]any{"items": out})
	})
}

func done(w http.ResponseWriter, err error) {
	if err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func orEmpty(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

func mask(s string) string { return paging.Mask(s) }

func recipient(m cad.Member) paging.Recipient {
	return paging.Recipient{Name: m.Name, Phone: m.Phone, Email: m.Email, Topic: m.Topic}
}

func (s *Server) agencyName() string {
	name := ""
	s.Eng.Read(func(st *cad.State) { name = st.Settings.Agency })
	return name
}

// ---- paging configuration and delivery ----

// PagingView is the paging configuration with secrets replaced by whether they are set.
type PagingView struct {
	Gateways  []paging.Gateway  `json:"gateways"`
	PublicURL string            `json:"publicUrl"`
	Kinds     []paging.KindInfo `json:"kinds"`
}

func (s *Server) pagingView() PagingView {
	c := s.pagingConfig()
	v := PagingView{Gateways: []paging.Gateway{}, PublicURL: c.PublicURL, Kinds: paging.Kinds()}
	for _, g := range c.Gateways {
		v.Gateways = append(v.Gateways, g.Redacted())
	}
	return v
}

func (s *Server) savePaging(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b Paging
	if !readJSON(w, r, &b) {
		return
	}
	b.PublicURL = strings.TrimRight(strings.TrimSpace(b.PublicURL), "/")
	if b.PublicURL != "" && !strings.HasPrefix(b.PublicURL, "https://") {
		fail(w, 422, "the address responders open is https://")
		return
	}
	s.mu.Lock()
	n := s.integ
	old := map[string]paging.Gateway{}
	for _, g := range n.Paging.Gateways {
		old[g.ID] = g
	}
	ids := map[string]bool{}
	for i := range b.Gateways {
		g := &b.Gateways[i]
		g.ID = strings.TrimSpace(g.ID)
		if g.ID == "" {
			g.ID = fmt.Sprintf("gw%d", i+1)
			for ids[g.ID] || old[g.ID].ID != "" {
				g.ID += "x"
			}
		}
		if ids[g.ID] {
			s.mu.Unlock()
			fail(w, 422, "two gateways have the id "+g.ID)
			return
		}
		ids[g.ID] = true
		// A secret shown as dots, or left empty, keeps the one set; "-" clears it.
		keep := func(v *string, was string) {
			switch *v {
			case "", "••••":
				*v = was
			case "-":
				*v = ""
			}
		}
		keep(&g.Token, old[g.ID].Token)
		keep(&g.SMTPPassword, old[g.ID].SMTPPassword)
		if err := g.Check(); err != nil {
			s.mu.Unlock()
			fail(w, 422, firstNonEmptyStr(g.Name, g.ID)+": "+err.Error())
			return
		}
	}
	n.Paging = b
	err := s.writeIntegrations(n)
	if err == nil {
		s.integ = n
	}
	s.mu.Unlock()
	if err != nil {
		writeErr(w, err)
		return
	}
	var names []string
	for _, g := range b.Gateways {
		names = append(names, g.Name+" ("+g.Kind+")")
	}
	s.Eng.Record(a, cad.EvIntegration, map[string]any{"paging": map[string]any{"gateways": names, "publicUrl": b.PublicURL}})
	writeJSON(w, 200, s.pagingView())
}

// writeIntegrations saves the integrations file. The caller holds s.mu.
func (s *Server) writeIntegrations(n Integrations) error {
	data, _ := json.MarshalIndent(n, "", "  ")
	tmp := s.integrationsPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.integrationsPath())
}

func (s *Server) pageSender() *paging.Sender {
	return &paging.Sender{HTTP: &http.Client{Timeout: 15 * time.Second}}
}

// pager pages groups whose rules match units as they are dispatched, and delivers every page written.
func (s *Server) pager(ctx context.Context) {
	ch := s.Eng.Subscribe()
	defer s.Eng.Unsubscribe(ch)
	paged := map[string]bool{} // incident|group: a group is paged once per incident by its rule
	seen := map[string]bool{}  // incident|unit|dispatched
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-ch:
			switch n.Type {
			case cad.EvIncDispatch:
				var groups []string
				s.Eng.Read(func(st *cad.State) {
					in := st.Incidents[n.Inc]
					if in == nil {
						return
					}
					var units []string
					for _, as := range in.Units {
						k := n.Inc + "|" + as.Unit + "|" + as.Dispatched.String()
						if as.Cleared.IsZero() && !seen[k] {
							seen[k] = true
							units = append(units, as.Unit)
						}
					}
					for _, g := range st.GroupsToPage(n.Inc, units) {
						if !paged[n.Inc+"|"+g] {
							paged[n.Inc+"|"+g] = true
							groups = append(groups, g)
						}
					}
				})
				if len(groups) > 0 {
					if _, _, err := s.Eng.SendPage(cad.Actor{User: "paging", Role: "system"}, n.Inc, groups, nil, "", s.pageText(n.Inc), true); err != nil {
						s.Log.Warn("auto page", "incident", n.Inc, "err", err)
					}
				}
			case cad.EvPageSent:
				for _, p := range s.undelivered() {
					go s.deliver(ctx, p)
				}
			}
		}
	}
}

type outPage struct {
	page cad.Page
	to   []cad.PageRecipient
}

// undelivered takes every page written but not yet delivered.
func (s *Server) undelivered() []outPage {
	var ids []string
	s.Eng.Read(func(st *cad.State) {
		for i := len(st.Pages) - 1; i >= 0 && i >= len(st.Pages)-20; i-- {
			ids = append(ids, st.Pages[i].ID)
		}
	})
	var out []outPage
	for _, id := range ids {
		if p, to, ok := s.Eng.TakePage(id); ok {
			out = append(out, outPage{p, to})
		}
	}
	return out
}

// pageText is the body of an automatic page: the incident's summary and its units.
func (s *Server) pageText(number string) string {
	text := ""
	s.Eng.Read(func(st *cad.State) {
		in := st.Incidents[number]
		if in == nil {
			return
		}
		var units []string
		for _, as := range in.Units {
			if as.Cleared.IsZero() {
				units = append(units, as.Unit)
			}
		}
		text = st.Summary(in)
		if len(units) > 0 {
			text += " Units: " + strings.Join(units, " ") + "."
		}
		text += " " + number
	})
	return strings.TrimSpace(text)
}

// deliver sends one page through each of its groups' gateways (the first gateway for people paged by
// name), each person with their own answer link, and records how each delivery went.
func (s *Server) deliver(ctx context.Context, op outPage) {
	cfg := s.pagingConfig()
	gw := map[string]paging.Gateway{}
	for _, g := range cfg.Gateways {
		gw[g.ID] = g
	}
	byGateway := map[string][]cad.PageRecipient{}
	var order []string
	add := func(g string, r cad.PageRecipient) {
		if _, ok := gw[g]; !ok {
			return
		}
		if !slices.Contains(order, g) {
			order = append(order, g)
		}
		if !slices.ContainsFunc(byGateway[g], func(x cad.PageRecipient) bool { return x.Member.ID == r.Member.ID }) {
			byGateway[g] = append(byGateway[g], r)
		}
	}
	var groups []*cad.PageGroup
	var addr string
	var lat, lon *float64
	var units []string
	prio := 3
	s.Eng.Read(func(st *cad.State) {
		for _, id := range op.page.Groups {
			if g := st.PageGroups[id]; g != nil {
				c := *g
				groups = append(groups, &c)
			}
		}
		if in := st.Incidents[op.page.Incident]; in != nil {
			prio = in.Priority
			if in.Location != nil {
				addr, lat, lon = in.Location.Address, in.Location.Lat, in.Location.Lon
			}
			for _, as := range in.Units {
				if as.Cleared.IsZero() {
					units = append(units, as.Unit)
				}
			}
		}
	})
	for _, r := range op.to {
		placed := false
		for _, g := range groups {
			if slices.Contains(g.Members, r.Member.ID) {
				for _, id := range g.Gateways {
					add(id, r)
					placed = true
				}
			}
		}
		if !placed && len(cfg.Gateways) > 0 {
			add(cfg.Gateways[0].ID, r)
		}
	}
	// A group paged through a service (a webhook, IamResponding, Active911) is paged even with no one on it.
	for _, g := range groups {
		for _, id := range g.Gateways {
			if k := gw[id].Kind; k == paging.Webhook || k == paging.IAmResponding || k == paging.Active911 {
				if !slices.Contains(order, id) {
					order = append(order, id)
				}
			}
		}
	}
	base := paging.Page{ID: op.page.ID, Incident: op.page.Incident, Subject: op.page.Subject, Text: op.page.Text, Address: addr, Lat: lat, Lon: lon, Units: units, Priority: prio, At: op.page.At}
	sender := s.pageSender()
	var results []cad.PageResult
	for _, id := range order {
		g := gw[id]
		people := byGateway[id]
		switch g.Kind {
		case paging.Webhook, paging.IAmResponding, paging.Active911:
			var to []paging.Recipient
			for _, r := range people {
				to = append(to, recipient(r.Member))
			}
			for _, res := range sender.Send(ctx, g, base, to) {
				results = append(results, cad.PageResult{Gateway: g.Name, Kind: g.Kind, To: res.To, OK: res.OK, Detail: res.Detail, At: res.At})
			}
		default:
			for _, r := range people {
				p := base
				if cfg.PublicURL != "" {
					p.ReplyURL = cfg.PublicURL + "/#/r/" + op.page.ID + "/" + r.Token
				}
				for _, res := range sender.Send(ctx, g, p, []paging.Recipient{recipient(r.Member)}) {
					results = append(results, cad.PageResult{Gateway: g.Name, Kind: g.Kind, To: res.To, OK: res.OK, Detail: res.Detail, At: res.At})
				}
			}
		}
	}
	if len(order) == 0 {
		results = append(results, cad.PageResult{Gateway: "none", OK: false, Detail: "no paging gateway is set up (Admin › Integrations › Paging)", At: time.Now()})
	}
	if err := s.Eng.RecordPageResults(cad.Actor{User: "paging", Role: "system"}, op.page.ID, results); err != nil {
		s.Log.Warn("page results", "page", op.page.ID, "err", err)
	}
}

// scheduler makes the incidents of scheduled calls and events when their time comes.
func (s *Server) scheduler(ctx context.Context) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		s.Eng.RunSchedule()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// ---- NEMSIS ----

func (s *Server) saveNEMSIS(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		nemsis.Config
		Token string `json:"token"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	s.mu.Lock()
	n := s.integ
	c := b.Config
	c.URL, c.Agency, c.AgencyStateID, c.State = strings.TrimSpace(c.URL), strings.TrimSpace(c.Agency), strings.TrimSpace(c.AgencyStateID), strings.ToUpper(strings.TrimSpace(c.State))
	switch v := strings.TrimSpace(b.Token); v {
	case "":
		c.Token = n.NEMSIS.Token
	case "-":
		c.Token = ""
	default:
		c.Token = v
	}
	if err := c.Check(); err != nil {
		s.mu.Unlock()
		fail(w, 422, err.Error())
		return
	}
	n.NEMSIS = c
	err := s.writeIntegrations(n)
	if err == nil {
		s.integ = n
	}
	s.mu.Unlock()
	if err != nil {
		writeErr(w, err)
		return
	}
	s.Eng.Record(a, cad.EvIntegration, map[string]any{"nemsis": map[string]any{"on": c.On, "url": c.URL, "agency": c.Agency, "state": c.State, "auto": c.Auto}})
	w.WriteHeader(http.StatusNoContent)
}

// nemsisResponse is what bitdispatcher knows of one EMS unit's response to an incident.
func (s *Server) nemsisResponse(number, unit string) (nemsis.Response, error) {
	unit = strings.ToUpper(unit)
	var r nemsis.Response
	var err error
	s.Eng.Read(func(st *cad.State) {
		in := st.Incidents[number]
		if in == nil {
			err = &cad.Problem{Field: "incident", Message: "no incident " + number}
			return
		}
		u := st.Units[unit]
		if u == nil {
			err = &cad.Problem{Field: "unit", Message: "no unit " + unit}
			return
		}
		var as *cad.Assignment
		for i := range in.Units {
			if in.Units[i].Unit == unit {
				as = &in.Units[i] // the latest assignment of the unit
			}
		}
		if as == nil {
			err = &cad.Problem{Field: "unit", Message: unit + " was not on " + number}
			return
		}
		r = nemsis.Response{Incident: number, Unit: unit, CallSign: unit, VehicleNumber: unit, UnitNotified: as.Dispatched, EnRoute: as.Enroute, OnScene: as.OnScene,
			LeftScene: as.Transport, AtDestination: as.AtDest, BackInService: as.Cleared, DispatchNotified: in.Created, PSAPCall: in.Created}
		caps := st.Capabilities(u)
		switch {
		case slices.Contains(caps, "als"):
			r.Capability = "als"
		case slices.Contains(caps, "bls"):
			r.Capability = "bls"
		}
		for _, id := range in.Calls {
			if c := st.Calls[id]; c != nil && !c.Start.IsZero() && c.Start.Before(r.PSAPCall) {
				r.PSAPCall = c.Start
			}
		}
		if ct := st.CallTypes[in.Type]; ct != nil {
			r.Reason, r.ReasonCode = ct.Name, ct.NEMSIS
			if r.ReasonCode == "" {
				r.ReasonCode = nemsis.SuggestReason(ct.Code, ct.Name)
			}
		}
		lights := in.Priority <= 2
		r.LightsSirens = &lights
		if in.Protocol != nil {
			r.EMD = "yes"
		}
		if l := in.Location; l != nil {
			r.Address, r.Lat, r.Lon = l.Address, l.Lat, l.Lon
			if c := l.Civic; c != nil {
				r.City, r.StateCode, r.ZIP, r.County = c.City, c.State, c.PostalCode, c.County
				if street := civicStreet(c); street != "" {
					r.Address = street
				}
			}
		}
	})
	return r, err
}

func civicStreet(c *ng911.Civic) string {
	return strings.Join(strings.Fields(strings.Join([]string{c.Number + c.NumberSuf, c.PreDir, c.Street, c.StreetType, c.PostDir}, " ")), " ")
}

// sendNEMSIS sends one unit's response to the ePCR system and records what it answered.
func (s *Server) sendNEMSIS(ctx context.Context, a cad.Actor, number, unit string) (*cad.EPCRRecord, error) {
	c := s.nemsisConfig()
	if !c.On {
		return nil, &cad.Problem{Field: "nemsis", Message: "patient-care reports are not set up (Admin › Integrations › Patient-care reports)"}
	}
	resp, err := s.nemsisResponse(number, unit)
	if err != nil {
		return nil, err
	}
	doc, err := nemsis.Encode(c, resp, s.Version)
	if err != nil {
		return nil, err
	}
	status, body, err := (&nemsis.Client{HTTP: &http.Client{Timeout: 20 * time.Second}}).Send(ctx, c, doc)
	rec := &cad.EPCRRecord{Unit: strings.ToUpper(unit), OK: err == nil, Status: status}
	if err != nil {
		rec.Error = err.Error()
		if body != "" && !strings.Contains(rec.Error, body) {
			rec.Error += ": " + body
		}
	}
	if e := s.Eng.RecordEPCR(a, number, *rec); e != nil {
		return rec, e
	}
	return rec, err
}

// epcrAuto sends each EMS unit's response when it clears from an incident, when set to.
func (s *Server) epcrAuto(ctx context.Context) {
	ch := s.Eng.Subscribe()
	defer s.Eng.Unsubscribe(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-ch:
			if n.Type != cad.EvUnitStatus || n.Unit == "" || n.Inc == "" {
				continue
			}
			c := s.nemsisConfig()
			if !c.On || !c.Auto {
				continue
			}
			send := false
			s.Eng.Read(func(st *cad.State) {
				u, in := st.Units[n.Unit], st.Incidents[n.Inc]
				if u == nil || in == nil || st.Discipline(u) != "ems" || u.Incident == n.Inc {
					return
				}
				for _, as := range in.Units {
					if as.Unit == n.Unit && !as.Cleared.IsZero() && !as.Dispatched.IsZero() {
						send = true
					}
				}
			})
			if send {
				go func(inc, unit string) {
					if _, err := s.sendNEMSIS(ctx, cad.Actor{User: "nemsis", Role: "system"}, inc, unit); err != nil {
						s.Log.Warn("ePCR", "incident", inc, "unit", unit, "err", err)
					}
				}(n.Inc, n.Unit)
			}
		}
	}
}
