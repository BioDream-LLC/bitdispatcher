package api

// Continuity (cad/continuity.go): the map's scheduled updates, the watch on the links the centre depends
// on, moving to the backup site with its checklist, and the continuity pack, a printable page of the open
// work that is written on a schedule to folders that outlive this server.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/mapdata"
)

// continuity is the server's memory of scheduled work that is not dispatch state.
type continuity struct {
	mu        sync.Mutex
	mapWindow string    // the schedule window (agency-local hour) last started, so it starts once
	mapLast   time.Time // the last scheduled run's end
	mapResult string    // its outcome: updated, up to date, skipped, failed
	mapError  string
	packAt    time.Time
	packErr   []string
}

// MapAutoHealth is the scheduled map update's state, shown in Admin › Map and on the consoles.
type MapAutoHealth struct {
	Mode   string     `json:"mode"`
	Next   *time.Time `json:"next,omitempty"`
	Last   *time.Time `json:"last,omitempty"`
	Result string     `json:"result,omitempty"`
	Error  string     `json:"error,omitempty"`
}

// ContinuityHealth is what every console shows of continuity: links down, the backup site, overflow.
type ContinuityHealth struct {
	Internet   string          `json:"internet"` // up, down or unknown (no checks configured)
	Down       []cad.LinkState `json:"down,omitempty"`
	Relocation *cad.Relocation `json:"relocation,omitempty"`
	Overflow   []OverflowBrief `json:"overflow,omitempty"`
	MapAuto    *MapAutoHealth  `json:"mapAuto,omitempty"`
}

// OverflowBrief is a live overflow, for the console banner.
type OverflowBrief struct {
	Partner   string    `json:"partner"`
	Name      string    `json:"name"`
	Direction string    `json:"direction"`
	Status    string    `json:"status"`
	Until     time.Time `json:"until"`
}

func (s *Server) continuityHealth() *ContinuityHealth {
	h := &ContinuityHealth{Internet: "unknown"}
	now := s.Eng.Now()
	s.Eng.Read(func(st *cad.State) {
		if len(st.Settings.Continuity.Checks) > 0 {
			h.Internet = "up"
		}
		for _, l := range st.Links {
			if !l.Up {
				h.Down = append(h.Down, *l)
				if strings.HasSuffix(l.Name, "Internet") && len(st.Settings.Continuity.Checks) > 0 {
					h.Internet = "down"
				}
			}
		}
		if r := st.Relocation; r != nil && r.Active {
			c := *r
			h.Relocation = &c
		}
		for _, o := range st.Overflows {
			if (o.Status == "active" || o.Status == "asked") && now.Before(o.Until) {
				h.Overflow = append(h.Overflow, OverflowBrief{Partner: o.Partner, Name: o.PartnerName, Direction: o.Direction, Status: o.Status, Until: o.Until})
			}
		}
	})
	sort.Slice(h.Down, func(i, j int) bool { return h.Down[i].Name < h.Down[j].Name })
	sort.Slice(h.Overflow, func(i, j int) bool {
		return h.Overflow[i].Partner+h.Overflow[i].Direction < h.Overflow[j].Partner+h.Overflow[j].Direction
	})
	h.MapAuto = s.mapAutoHealth()
	return h
}

func (s *Server) mapAutoHealth() *MapAutoHealth {
	var m cad.MapSchedule
	var loc *time.Location
	s.Eng.Read(func(st *cad.State) { m, loc = st.Settings.MapUpdates, st.Settings.Shift.Location() })
	h := &MapAutoHealth{Mode: m.Mode}
	if h.Mode == "" {
		h.Mode = "off"
	}
	if n := m.Next(s.Eng.Now().In(loc)); !n.IsZero() {
		h.Next = &n
	}
	s.cont.mu.Lock()
	if !s.cont.mapLast.IsZero() {
		t := s.cont.mapLast
		h.Last = &t
	}
	h.Result, h.Error = s.cont.mapResult, s.cont.mapError
	s.cont.mu.Unlock()
	return h
}

func (s *Server) continuityRoutes() {
	s.handle("GET /api/continuity", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out := map[string]any{"health": s.continuityHealth()}
		s.Eng.Read(func(st *cad.State) {
			links := []cad.LinkState{}
			for _, l := range st.Links {
				links = append(links, *l)
			}
			sort.Slice(links, func(i, j int) bool { return links[i].Name < links[j].Name })
			outages := st.Outages
			if len(outages) > 50 {
				outages = outages[len(outages)-50:]
			}
			out["links"], out["outages"], out["steps"], out["contacts"] = links, outages, st.Settings.Continuity.Steps, st.Settings.Continuity.Contacts
			out["relocation"] = st.Relocation
			out["packMinutes"], out["checks"] = st.Settings.Continuity.PackMinutes, len(st.Settings.Continuity.Checks)
		})
		s.cont.mu.Lock()
		if !s.cont.packAt.IsZero() {
			out["packAt"] = s.cont.packAt
		}
		out["packErrors"] = s.cont.packErr
		s.cont.mu.Unlock()
		out["sites"], out["site"] = s.siteHealth(), s.Eng.Site()
		writeJSON(w, 200, out)
	})
	s.handle("POST /api/continuity/relocation", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if !isSupervisor(a) {
			fail(w, 403, "a supervisor declares the move to the backup site")
			return
		}
		var b struct {
			Active bool   `json:"active"`
			Note   string `json:"note"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		if err := s.Eng.SetRelocation(a, b.Active, b.Note); err != nil {
			writeErr(w, err)
			return
		}
		subject := "The centre is working from the backup site"
		if !b.Active {
			subject = "The centre is back in its own building"
		}
		body := subject + "."
		if site := s.Eng.Site(); site != "" && b.Active {
			body = fmt.Sprintf("Declared by %s at site %s. %s", a.User, site, strings.TrimSpace(b.Note))
		}
		s.Eng.SendComm(cad.SystemActor, cad.Comm{Kind: "admin", Priority: "high", Subject: subject, Body: body,
			To: []string{cad.ToDuty + "supervisor", cad.ToDuty + "dispatcher", cad.ToDuty + "calltaker", cad.ToDuty + "police", cad.ToDuty + "fire", cad.ToDuty + "ems"}})
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("POST /api/continuity/relocation/steps/{i}", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if !isSupervisor(a) {
			fail(w, 403, "a supervisor works the relocation checklist")
			return
		}
		i, err := strconv.Atoi(r.PathValue("i"))
		if err != nil {
			fail(w, 400, "which step?")
			return
		}
		var b struct {
			Done bool `json:"done"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		if err := s.Eng.TickRelocationStep(a, i, b.Done); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("GET /api/continuity/pack", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		b, err := s.continuityPack()
		if err != nil {
			writeErr(w, err)
			return
		}
		s.Eng.Record(a, cad.EvRecordsViewed, map[string]any{"continuityPack": true})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(b)
	})
	s.handle("POST /api/continuity/pack/write", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if !isSupervisor(a) {
			fail(w, 403, "a supervisor writes the continuity pack")
			return
		}
		s.writePack()
		s.cont.mu.Lock()
		out := map[string]any{"at": s.cont.packAt, "errors": s.cont.packErr}
		s.cont.mu.Unlock()
		writeJSON(w, 200, out)
	})
}

// ---- scheduled map updates ----

// mapSchedule starts the map's update in its window, once per window. A window missed while the server
// was down, a run refused because a recording is under way, or one that fails waits for the next window;
// the map in use is untouched by any of them.
func (s *Server) mapSchedule(ctx context.Context) {
	if s.Maps == nil || s.isTraining {
		return
	}
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		s.mapTick(s.Eng.Now())
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Server) mapTick(now time.Time) {
	var m cad.MapSchedule
	var loc *time.Location
	s.Eng.Read(func(st *cad.State) { m, loc = st.Settings.MapUpdates, st.Settings.Shift.Location() })
	lt := now.In(loc)
	if !m.Due(lt) {
		return
	}
	window := lt.Format("2006-01-02T15")
	s.cont.mu.Lock()
	if s.cont.mapWindow == window {
		s.cont.mu.Unlock()
		return
	}
	s.cont.mapWindow = window
	s.cont.mu.Unlock()
	actor := cad.Actor{User: "schedule", Role: auth.Admin}
	s.Maps.SetMaxDrop(m.MaxDrop())
	if err := s.Maps.Update("schedule"); err != nil {
		s.mapOutcome("skipped", err.Error())
		s.Eng.Record(actor, cad.EvMapChecked, map[string]any{"result": "skipped", "reason": err.Error()})
		return
	}
	s.Eng.Record(actor, cad.EvMapChecked, map[string]any{"result": "started", "window": window})
}

func (s *Server) mapOutcome(result, why string) {
	s.cont.mu.Lock()
	s.cont.mapLast, s.cont.mapResult, s.cont.mapError = s.Eng.Now(), result, why
	s.cont.mu.Unlock()
}

// mapScheduleEvents adds the schedule's outcomes to the map's callbacks: a scheduled run that fails tells
// the supervisors.
func (s *Server) mapScheduleEvents() {
	if s.Maps == nil {
		return
	}
	rec, failed := s.Maps.Recorded, s.Maps.Failed
	s.Maps.Recorded = func(j mapdata.Job) {
		if rec != nil {
			rec(j)
		}
		if j.By == "schedule" {
			s.mapOutcome("updated", fmt.Sprintf("%d changes", j.Changes))
		}
	}
	s.Maps.Failed = func(j mapdata.Job) {
		if failed != nil {
			failed(j)
		}
		if j.By == "schedule" {
			s.mapOutcome("failed", j.Error)
			s.Eng.SendComm(cad.SystemActor, cad.Comm{Kind: "admin", Priority: "high", Subject: "The scheduled map update failed",
				Body: "The map in use is unchanged and keeps working. " + j.Error, To: []string{cad.ToDuty + "supervisor", cad.ToDuty + "admin"}})
		}
	}
	s.Maps.UpToDate = func(j mapdata.Job) {
		if j.By == "schedule" {
			s.mapOutcome("up to date", "")
			s.Eng.Record(cad.Actor{User: "schedule", Role: auth.Admin}, cad.EvMapChecked, map[string]any{"result": "up to date"})
		}
	}
}

// ---- the links ----

// linkClient checks links; it does not follow redirects, so a check is an answer from the address given.
var linkClient = &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// linkWatch checks the internet, BitVoice, the other sites and the overflow partners, records each change
// in the ledger, and tells the supervisors when the internet goes and comes back.
func (s *Server) linkWatch(ctx context.Context) {
	if s.isTraining {
		return
	}
	for {
		var p cad.ContinuityPolicy
		s.Eng.Read(func(st *cad.State) { p = st.Settings.Continuity })
		s.checkLinks(ctx, p)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(p.CheckSecondsOr()) * time.Second):
		}
	}
}

func (s *Server) checkLinks(ctx context.Context, p cad.ContinuityPolicy) {
	if len(p.Checks) > 0 {
		up, why := false, []string{}
		for _, c := range p.Checks {
			err := probe(ctx, c.URL, nil)
			if err == nil {
				up = true
				break
			}
			why = append(why, c.Name+": "+err.Error())
		}
		if changed, _ := s.Eng.SetLink("Internet", up, strings.Join(why, "; ")); changed {
			subject := "The internet is down"
			body := "Dispatch, call entry, the recorded map and routing keep working on this server. Links that need the internet " +
				"(online map tiles, Street View, traffic, map updates, neighbours, NERIS, PSC PIP) wait until it is back. " + strings.Join(why, "; ")
			if up {
				subject, body = "The internet is back", "Links that need the internet are working again."
			}
			s.Eng.SendComm(cad.SystemActor, cad.Comm{Kind: "admin", Priority: map[bool]string{true: "medium", false: "high"}[up], Subject: subject, Body: body,
				To: []string{cad.ToDuty + "supervisor"}})
		}
	}
	if h := s.BV.Health(); h.Configured {
		s.Eng.SetLink("BitVoice", h.Connected, h.LastError)
	}
	for _, h := range s.siteHealth() {
		if !h.Last.IsZero() || h.Error != "" {
			s.Eng.SetLink("Site "+h.Name, h.OK, h.Error)
		}
	}
	for _, pt := range s.partners() {
		if !pt.Overflow.Ask && !pt.Overflow.Help {
			continue
		}
		base, us, err := pt.overflowBase()
		if err != nil {
			s.Eng.SetLink("Neighbour "+pt.Name, false, err.Error())
			continue
		}
		err = probeWith(ctx, pt.client(), base+"/api/overflow-link/"+us+"/ping", map[string]string{"Authorization": "Bearer " + pt.OutToken})
		s.Eng.SetLink("Neighbour "+pt.Name, err == nil, errText(err))
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// probe asks an address for an answer. For the internet any HTTP answer will do; with headers (a
// neighbour), only a success.
func probe(ctx context.Context, url string, h map[string]string) error {
	return probeWith(ctx, linkClient, url, h)
}

func probeWith(ctx context.Context, c *http.Client, url string, h map[string]string) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	for k, v := range h {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		var ue interface{ Timeout() bool }
		if errors.As(err, &ue) && ue.Timeout() {
			return errors.New("no answer in 8 seconds")
		}
		return err
	}
	resp.Body.Close()
	if h != nil && resp.StatusCode >= 300 {
		return fmt.Errorf("answered %s", resp.Status)
	}
	return nil
}

// ---- the continuity pack ----

var packTmpl = template.Must(template.New("pack").Funcs(template.FuncMap{"t": func(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("Jan 2 15:04")
}}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Continuity pack {{.At}}</title>
<style>body{font:13px/1.4 system-ui,sans-serif;margin:24px;color:#111}h1{font-size:20px;margin:0}h2{font-size:15px;margin:22px 0 6px;border-bottom:2px solid #111}
table{border-collapse:collapse;width:100%}td,th{border:1px solid #999;padding:3px 6px;text-align:left;vertical-align:top}th{background:#eee}
.warn{border:2px solid #b00;padding:6px 10px;margin:10px 0}.muted{color:#555}ol li{margin:2px 0}@media print{h2{break-after:avoid}tr{break-inside:avoid}}</style></head><body>
<h1>{{.Agency}}: continuity pack</h1>
<p class="muted">Written {{.At}}{{if .Site}} at site {{.Site}}{{end}}. This is a copy of the open work at that moment, for working on paper if the CAD cannot be reached.
It holds callers' names and numbers: keep it as you keep the CAD's records, and destroy old copies.</p>
{{if .Relocation}}<div class="warn">The centre declared it was working from its backup site at {{t .Relocation.Since}} ({{.Relocation.By}}). {{.Relocation.Note}}</div>{{end}}
{{if .Down}}<div class="warn">Links down: {{range $i, $l := .Down}}{{if $i}}; {{end}}{{$l.Name}} since {{t $l.Since}}{{end}}</div>{{end}}
<h2>Open incidents ({{len .Incidents}})</h2>
<table><tr><th>Incident</th><th>Type</th><th>P</th><th>Status</th><th>Location</th><th>Units</th><th>Caller</th><th>Created</th></tr>
{{range .Incidents}}<tr><td>{{.Number}}</td><td>{{.Type}}</td><td>{{.Priority}}</td><td>{{.Status}}</td><td>{{.Location}}</td><td>{{.Units}}</td><td>{{.Caller}}</td><td>{{t .Created}}</td></tr>{{else}}<tr><td colspan="8">None open.</td></tr>{{end}}</table>
<h2>Units ({{len .Units}})</h2>
<table><tr><th>Unit</th><th>Type</th><th>Station</th><th>Status</th><th>Incident</th></tr>
{{range .Units}}<tr><td>{{.ID}}</td><td>{{.Type}}</td><td>{{.Station}}</td><td>{{.Status}}</td><td>{{.Incident}}</td></tr>{{end}}</table>
{{if .Steps}}<h2>Relocation checklist</h2><ol>{{range .Steps}}<li>☐ {{.}}</li>{{end}}</ol>{{end}}
{{if .Contacts}}<h2>Contacts</h2><ul>{{range .Contacts}}<li>{{.}}</li>{{end}}</ul>{{end}}
{{if .Partners}}<h2>Neighbouring centres</h2><ul>{{range .Partners}}<li>{{.}}</li>{{end}}</ul>{{end}}
<h2>Paper log</h2><table><tr><th style="width:12%">Time</th><th style="width:14%">Incident / unit</th><th>What happened</th><th style="width:12%">By</th></tr>
{{range .Blank}}<tr><td>&nbsp;</td><td></td><td></td><td></td></tr>{{end}}</table>
</body></html>`))

type packIncident struct {
	Number, Type, Status, Location, Units, Caller string
	Priority                                      int
	Created                                       time.Time
}

type packUnit struct{ ID, Type, Station, Status, Incident string }

func (s *Server) continuityPack() ([]byte, error) {
	d := struct {
		Agency, Site, At string
		Relocation       *cad.Relocation
		Down             []cad.LinkState
		Incidents        []packIncident
		Units            []packUnit
		Steps, Contacts  []string
		Partners         []string
		Blank            []int
	}{Site: s.Eng.Site(), At: s.Eng.Now().Local().Format("Mon Jan 2 2006 15:04 MST"), Blank: make([]int, 25)}
	h := s.continuityHealth()
	d.Down = h.Down
	s.Eng.Read(func(st *cad.State) {
		d.Agency = firstNonEmptyStr(st.Settings.Agency, "The centre")
		if r := st.Relocation; r != nil && r.Active {
			c := *r
			d.Relocation = &c
		}
		d.Steps, d.Contacts = st.Settings.Continuity.Steps, st.Settings.Continuity.Contacts
		for _, in := range st.Incidents {
			if in.Status == cad.Closed {
				continue
			}
			m := maskIncident(in)
			pi := packIncident{Number: m.Number, Type: m.Type, Status: m.Status, Priority: m.Priority, Created: m.Created}
			if ct := st.CallTypes[m.Type]; ct != nil {
				pi.Type = m.Type + " " + ct.Name
			}
			if m.Location != nil {
				pi.Location = m.Location.Address
				if pi.Location == "" && m.Location.HasPoint() {
					pi.Location = fmt.Sprintf("%.5f, %.5f", *m.Location.Lat, *m.Location.Lon)
				}
			}
			var us []string
			for _, as := range m.Units {
				if as.Cleared.IsZero() {
					us = append(us, as.Unit+" "+as.Status)
				}
			}
			pi.Units = strings.Join(us, ", ")
			if m.Caller != nil {
				pi.Caller = strings.TrimSpace(m.Caller.Name + " " + m.Caller.Callback)
			}
			d.Incidents = append(d.Incidents, pi)
		}
		for _, u := range st.Units {
			d.Units = append(d.Units, packUnit{ID: u.ID, Type: u.Type, Station: u.Station, Status: u.Status, Incident: u.Incident})
		}
	})
	sort.Slice(d.Incidents, func(i, j int) bool {
		if d.Incidents[i].Priority != d.Incidents[j].Priority {
			return d.Incidents[i].Priority < d.Incidents[j].Priority
		}
		return d.Incidents[i].Number < d.Incidents[j].Number
	})
	sort.Slice(d.Units, func(i, j int) bool { return unitLess(d.Units[i].ID, d.Units[j].ID) })
	for _, p := range s.partners() {
		d.Partners = append(d.Partners, p.Name)
	}
	var b bytes.Buffer
	err := packTmpl.Execute(&b, d)
	return b.Bytes(), err
}

// packWriter writes the continuity pack on its schedule.
func (s *Server) packWriter(ctx context.Context) {
	if s.isTraining {
		return
	}
	for {
		var mins int
		s.Eng.Read(func(st *cad.State) { mins = st.Settings.Continuity.PackMinutes })
		wait := time.Minute
		if mins > 0 {
			s.writePack()
			wait = time.Duration(mins) * time.Minute
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// writePack writes the pack to the data directory and each folder set; a folder that cannot be written
// is reported, and the others are still written.
func (s *Server) writePack() {
	b, err := s.continuityPack()
	var errs []string
	if err != nil {
		errs = append(errs, err.Error())
	} else {
		var dirs []string
		if s.DataDir != "" {
			dirs = append(dirs, filepath.Join(s.DataDir, "continuity"))
		}
		s.Eng.Read(func(st *cad.State) { dirs = append(dirs, st.Settings.Continuity.PackDirs...) })
		for _, d := range dirs {
			if err := writeFileAtomic(d, "continuity-pack.html", b); err != nil {
				errs = append(errs, d+": "+err.Error())
			}
		}
	}
	s.cont.mu.Lock()
	s.cont.packAt, s.cont.packErr = s.Eng.Now(), errs
	s.cont.mu.Unlock()
}

func writeFileAtomic(dir, name string, b []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(dir, "."+name+".tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, name))
}
