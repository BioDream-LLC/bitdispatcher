package api

// Self-service setup: the setup checklist, the go-live check and the support bundle. An agency can set
// bitdispatcher up by itself, check it before the first live call, and send whoever supports it a
// bundle that holds no secrets and no caller data. Nothing here sends anything anywhere: the bundle is
// downloaded by the administrator, who reads it and decides whether to send it.

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/geo"
	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// SetupStep is one step of the setup checklist.
type SetupStep struct {
	ID      string         `json:"id"`
	Title   string         `json:"title"`
	Why     string         `json:"why"`
	Need    string         `json:"need"`   // required, recommended, optional
	Status  string         `json:"status"` // done, todo, warn (done, with something to look at)
	Detail  string         `json:"detail,omitempty"`
	Page    string         `json:"page,omitempty"`    // the admin page that does it
	Chapter string         `json:"chapter,omitempty"` // the manual chapter about it
	Manual  bool           `json:"manual,omitempty"`  // only a person can say it is done
	Tick    *cad.SetupTick `json:"tick,omitempty"`    // a person confirmed it, or said it is not needed
}

// SetupView is the checklist.
type SetupView struct {
	Steps    []SetupStep       `json:"steps"`
	Done     int               `json:"done"`
	Required int               `json:"required"` // required steps
	Left     int               `json:"left"`     // required steps not done
	GoLive   *cad.GoLiveResult `json:"goLive,omitempty"`
	Support  string            `json:"support"` // who the agency asks for help
}

// DefaultSupport is shown when the agency has not said who supports it.
const DefaultSupport = "Community support: questions and problems go to https://github.com/BioDream-LLC/bitdispatcher/issues (never post caller data, addresses or secrets there). If your agency has a support agreement, put its contact here."

func (s *Server) setupRoutes() {
	s.handle("GET /api/admin/setup", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		writeJSON(w, 200, s.setupView())
	})
	s.handle("PUT /api/admin/setup/{step}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Done bool   `json:"done"`
			Note string `json:"note"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		id := r.PathValue("step")
		var st *SetupStep
		for _, x := range s.setupView().Steps {
			if x.ID == id {
				st = &x
				break
			}
		}
		switch {
		case st == nil:
			fail(w, 404, "no such step")
			return
		case b.Done && !st.Manual && st.Need == "required":
			fail(w, 422, "bitdispatcher checks this step itself: "+st.Title+" is done when it is")
			return
		case b.Done && !st.Manual && strings.TrimSpace(b.Note) == "":
			fail(w, 422, "say why this step is not needed")
			return
		}
		if err := s.Eng.TickSetup(a, id, b.Done, b.Note); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, s.setupView())
	})
	s.handle("POST /api/admin/golive", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		g := s.goLiveCheck()
		if err := s.Eng.SaveGoLive(a, g); err != nil {
			writeErr(w, err)
			return
		}
		g.By, g.At = a.User, s.Eng.Now()
		writeJSON(w, 200, g)
	})
	s.handle("POST /api/admin/support/bundle", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		data, files, err := s.supportBundle()
		if err != nil {
			writeErr(w, err)
			return
		}
		s.Eng.Record(a, cad.EvSupportBundle, map[string]any{"files": files, "bytes": len(data)})
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="bitdispatcher-support-`+s.Eng.Now().UTC().Format("20060102-1504")+`.zip"`)
		w.Header().Set("Cache-Control", "no-store")
		w.Write(data)
	})
}

// demoLeft lists what is left of the demonstration data.
func demoLeft(st *cad.State) []string {
	var out []string
	if strings.Contains(strings.ToLower(st.Settings.Agency), "demo") {
		out = append(out, "the agency's name")
	}
	n := 0
	for _, x := range st.Stations {
		if strings.HasPrefix(strings.ToLower(x.Address), "demo ") {
			n++
		}
	}
	if n > 0 {
		out = append(out, plural(n, "demo station"))
	}
	n = 0
	for id := range st.Premises {
		if strings.HasPrefix(id, "DEMO-") {
			n++
		}
	}
	if n > 0 {
		out = append(out, plural(n, "demo premise"))
	}
	if st.Settings.NCICMode == "sim" {
		out = append(out, "NCIC on the training simulator")
	}
	return out
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	if strings.HasSuffix(what, "s") {
		return fmt.Sprintf("%d %ses", n, what)
	}
	return fmt.Sprintf("%d %ss", n, what)
}

func listSome(xs []string, n int) string {
	sort.Strings(xs)
	if len(xs) > n {
		return strings.Join(xs[:n], ", ") + fmt.Sprintf(" and %d more", len(xs)-n)
	}
	return strings.Join(xs, ", ")
}

func (s *Server) integCopy() Integrations {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.integ
}

// setupView works the checklist out from what is configured, plus what people have confirmed.
func (s *Server) setupView() SetupView {
	in := s.integCopy()
	users := s.Auth.List()
	var bv bool
	if s.BV != nil {
		bv = s.BV.Health().Configured
	}
	var mapArea string
	var addrs int64
	if s.Maps != nil {
		if m := s.Maps.Store(); m != nil {
			mapArea, addrs = m.Area.Name, m.Stats.Addresses
		}
	}
	gisLayers := 0
	if s.GIS != nil {
		for _, l := range s.GIS.Layers() {
			if l.On {
				gisLayers++
			}
		}
	}
	peers := len(s.sitesView().Peers)
	var steps []SetupStep
	var ticks map[string]cad.SetupTick
	var goLive *cad.GoLiveResult
	support := ""
	s.Eng.Read(func(st *cad.State) {
		ticks = map[string]cad.SetupTick{}
		for k, v := range st.SetupTicks {
			ticks[k] = v
		}
		if st.GoLive != nil {
			g := *st.GoLive
			goLive = &g
		}
		support = st.Settings.Next.Support
		add := func(x SetupStep, done bool, warn bool) {
			x.Status = "todo"
			if done {
				x.Status = "done"
				if warn {
					x.Status = "warn"
				}
			}
			steps = append(steps, x)
		}
		// 1. The agency.
		ag := strings.TrimSpace(st.Settings.Agency)
		add(SetupStep{ID: "agency", Title: "Name the agency and set the incident numbers", Need: "required", Page: "settings", Chapter: "admin-settings",
			Why: "The name is on every screen, report and printout; the prefix starts every incident number.", Detail: ag}, ag != "" && !strings.Contains(strings.ToLower(ag), "demo"), false)
		// 2. The map.
		x := SetupStep{ID: "map", Title: "Record the road map", Need: "required", Page: "map", Chapter: "admin-map",
			Why: "Drive times, unit recommendations and address search come from the recorded map. Without it, distances are straight lines."}
		if mapArea != "" {
			x.Detail = mapArea + ": " + plural(int(addrs), "address")
		}
		add(x, mapArea != "", mapArea != "" && addrs == 0)
		// 3. The county's GIS.
		add(SetupStep{ID: "gis", Title: "Import the county's addresses and zones (GIS)", Need: "recommended", Page: "gis", Chapter: "admin-gis",
			Why:    "The county's own address points match the 911 location database better than the open map does. Response zones come from here too.",
			Detail: plural(gisLayers, "layer") + " in use"}, gisLayers > 0, false)
		// 4. Stations.
		var noPlace []string
		for _, x := range st.Stations {
			if x.Lat == 0 && x.Lon == 0 {
				noPlace = append(noPlace, x.Name)
			}
		}
		x = SetupStep{ID: "stations", Title: "Add the stations", Need: "required", Page: "stations", Chapter: "admin-stations",
			Why: "Units are recommended from where their station is when the unit has no GPS position.", Detail: plural(len(st.Stations), "station")}
		if len(noPlace) > 0 {
			x.Detail += "; not on the map: " + listSome(noPlace, 5)
		}
		add(x, len(st.Stations) > 0 && len(noPlace) == 0, false)
		// 5. Units.
		types := map[string]bool{}
		for _, t := range st.Settings.UnitTypes {
			types[t.Code] = true
		}
		var badUnit []string
		for id, u := range st.Units {
			if !types[u.Type] || u.Station != "" && st.Stations[u.Station] == nil {
				badUnit = append(badUnit, id)
			}
		}
		x = SetupStep{ID: "units", Title: "Add the units", Need: "required", Page: "units", Chapter: "admin-units",
			Why: "Each unit's type gives its capabilities (engine, medic, patrol), which the run cards ask for.", Detail: plural(len(st.Units), "unit")}
		if len(badUnit) > 0 {
			x.Detail += "; an unknown type or station: " + listSome(badUnit, 5)
		}
		add(x, len(st.Units) > 0 && len(badUnit) == 0, false)
		// 6. Call types and run cards.
		var noCard []string
		for code, ct := range st.CallTypes {
			if len(ct.RunCard) == 0 {
				noCard = append(noCard, code)
			}
		}
		x = SetupStep{ID: "calltypes", Title: "Set the call types and run cards", Need: "required", Page: "calltypes", Chapter: "admin-calltypes",
			Why: "A run card says what each call type sends. The chiefs need to agree on them; this usually takes longest.", Detail: plural(len(st.CallTypes), "call type")}
		if len(noCard) > 0 {
			x.Detail += "; no run card (chosen by hand): " + listSome(noCard, 6)
		}
		add(x, len(st.CallTypes) > len(noCard), len(noCard) > 0)
		// 7. People.
		var adminNoMFA []string
		others := 0
		for _, u := range users {
			if u.Disabled || u.Demo {
				continue
			}
			if u.Role == auth.Admin && !u.MFA {
				adminNoMFA = append(adminNoMFA, u.Username)
			}
			if u.Role != auth.Admin {
				others++
			}
		}
		x = SetupStep{ID: "users", Title: "Add the people, with an authenticator for administrators", Need: "required", Page: "users", Chapter: "admin-users",
			Why:    "Everyone signs in as themselves, so the ledger says who did what. CJIS requires a second factor for administrators.",
			Detail: plural(others, "other account")}
		if len(adminNoMFA) > 0 {
			x.Detail += "; administrators without an authenticator: " + listSome(adminNoMFA, 5)
		}
		add(x, others > 0 && len(adminNoMFA) == 0, false)
		// 8. Demonstration data.
		left := demoLeft(st)
		for _, u := range users {
			if u.Demo {
				left = append(left, "the demonstration account "+u.Username)
			}
		}
		x = SetupStep{ID: "demo", Title: "Remove the demonstration data", Need: "required", Page: "stations", Chapter: "start",
			Why: "Demo stations, premises and accounts must not be mistaken for real ones on a live floor."}
		if len(left) > 0 {
			x.Detail = "Still here: " + strings.Join(left, "; ")
		}
		add(x, len(left) == 0, false)
		// 9. The phone system.
		var phone []string
		if bv {
			phone = append(phone, "BitVoice")
		}
		if in.ALIListen != "" {
			phone = append(phone, "ALI spills")
		}
		if in.LIS.On {
			phone = append(phone, "a location server")
		}
		add(SetupStep{ID: "phone", Title: "Connect the phone system", Need: "recommended", Page: "integrations", Chapter: "admin-integrations",
			Why: "Calls then arrive with the caller's number and location. Without it, call-takers type them in.", Detail: strings.Join(phone, ", ")}, len(phone) > 0, false)
		// 10. Station alerting and paging.
		var alerting []string
		if in.Extra.StationAlerts && in.Extra.StationGatewayURL != "" {
			alerting = append(alerting, "station alerting")
		}
		if n := len(in.Paging.Gateways); n > 0 {
			alerting = append(alerting, plural(n, "paging gateway"))
		}
		add(SetupStep{ID: "paging", Title: "Connect station alerting and paging", Need: "recommended", Page: "paging", Chapter: "admin-paging",
			Why: "Dispatching a unit then tones out its station and pages its people.", Detail: strings.Join(alerting, ", ")}, len(alerting) > 0, false)
		// 11. NCIC.
		x = SetupStep{ID: "ncic", Title: "Connect to the state message switch (NCIC)", Need: "optional", Page: "settings", Chapter: "ncic",
			Why: "Plate, person and gun queries from the console. The state's CJIS office has to approve the connection.", Detail: "off"}
		switch st.Settings.NCICMode {
		case "line":
			x.Detail = "connected through " + st.Settings.NCICAddr
		case "sim":
			x.Detail = "the training simulator"
		}
		add(x, st.Settings.NCICMode == "line", false)
		// 12. A second site.
		add(SetupStep{ID: "sites", Title: "Add a second site", Need: "recommended", Page: "sites", Chapter: "admin-sites",
			Why: "Two servers in different buildings both stay live. If one fails, the other carries on with every incident.", Detail: plural(peers, "other site")}, peers > 0, false)
		// 13–16. What only people can say.
		add(SetupStep{ID: "backups", Title: "Copy the data directory off this server every night", Need: "required", Manual: true, Chapter: "self-service",
			Why: "A second site is not a backup: a mistake is copied to both. Keep nightly copies somewhere else, and try restoring one."}, false, false)
		add(SetupStep{ID: "training", Title: "Train the dispatchers, supervisors and crews", Need: "required", Manual: true, Chapter: "training",
			Why: "Use Training mode: a copy of the system where nothing reaches real units."}, false, false)
		add(SetupStep{ID: "parallel", Title: "Run beside the old system", Need: "recommended", Manual: true, Chapter: "self-service",
			Why: "For a few weeks, enter calls in both and compare recommendations and times before switching over."}, false, false)
		gl := SetupStep{ID: "golive", Title: "Pass the go-live check", Need: "required", Page: "golive", Chapter: "admin-golive",
			Why: "Automatic tests of the ledger, a restore, test calls at every station, the connections and the second site."}
		ok := false
		if goLive != nil {
			gl.Detail = fmt.Sprintf("Last run %s by %s: %d passed, %d to look at, %d failed", goLive.At.Format("2 Jan 15:04"), goLive.By, goLive.Pass, goLive.Warn, goLive.Fail)
			ok = goLive.Ready() && s.Eng.Now().Sub(goLive.At) < 7*24*time.Hour
			if goLive.Ready() && !ok {
				gl.Detail += " (more than a week ago: run it again)"
			}
		}
		add(gl, ok, ok && goLive.Warn > 0)
		sp := strings.TrimSpace(support)
		add(SetupStep{ID: "support", Title: "Decide who supports you", Need: "recommended", Page: "support", Chapter: "admin-support",
			Why: "Write down who to call when something goes wrong at 3 a.m.: your IT staff, a support agreement, or community support.", Detail: sp}, sp != "", false)
	})
	v := SetupView{Steps: steps, GoLive: goLive, Support: support}
	if strings.TrimSpace(v.Support) == "" {
		v.Support = DefaultSupport
	}
	for i := range v.Steps {
		x := &v.Steps[i]
		if t, ok := ticks[x.ID]; ok && x.Status == "todo" && (x.Manual || x.Need != "required") {
			x.Tick, x.Status = &t, "done"
		}
		if x.Status != "todo" {
			v.Done++
		}
		if x.Need == "required" {
			v.Required++
			if x.Status == "todo" {
				v.Left++
			}
		}
	}
	return v
}

// goLiveCheck runs the go-live tests. None of them sends a page, a text or a query to anyone.
func (s *Server) goLiveCheck() cad.GoLiveResult {
	var g cad.GoLiveResult
	add := func(id, title, result, detail, fix string) {
		g.Checks = append(g.Checks, cad.GoLiveItem{ID: id, Title: title, Result: result, Detail: detail, Fix: fix})
		switch result {
		case "pass":
			g.Pass++
		case "warn":
			g.Warn++
		default:
			g.Fail++
		}
	}
	// The ledger: every event's hash and link.
	path := s.Eng.LedgerPath()
	rep := ledger.Verify(path)
	if rep.OK {
		add("ledger", "The ledger is intact", "pass", fmt.Sprintf("%d events, every hash and link checked", rep.Events), "")
	} else {
		add("ledger", "The ledger is intact", "fail", rep.Problem, "ledger")
	}
	// A restore: the ledger copied and replayed on its own gives the same system.
	add(s.restoreCheck(path))
	// Disk space.
	if free, ok := freeBytes(s.DataDir); ok {
		gb := float64(free) / (1 << 30)
		switch {
		case gb < 2:
			add("disk", "Room on the disk", "fail", fmt.Sprintf("%.1f GB free in the data directory", gb), "")
		case gb < 10:
			add("disk", "Room on the disk", "warn", fmt.Sprintf("%.1f GB free in the data directory; keep 10 GB or more", gb), "")
		default:
			add("disk", "Room on the disk", "pass", fmt.Sprintf("%.0f GB free", gb), "")
		}
	}
	// Demonstration data and sign-in.
	var left []string
	var stations []*cad.Station
	var types []string
	stationUnits := map[string][]string{}
	s.Eng.Read(func(st *cad.State) {
		left = demoLeft(st)
		for _, x := range st.Stations {
			c := *x
			stations = append(stations, &c)
		}
		for code, ct := range st.CallTypes {
			if len(ct.RunCard) > 0 {
				types = append(types, code)
			}
		}
		for id, u := range st.Units {
			if u.Station != "" {
				stationUnits[u.Station] = append(stationUnits[u.Station], id)
			}
		}
	})
	for _, u := range s.Auth.List() {
		if u.Demo {
			left = append(left, "the demonstration account "+u.Username)
		}
	}
	if len(left) > 0 {
		add("demo", "No demonstration data", "fail", "Still here: "+strings.Join(left, "; "), "stations")
	} else {
		add("demo", "No demonstration data", "pass", "", "")
	}
	var noMFA []string
	for _, u := range s.Auth.List() {
		if u.Role == auth.Admin && !u.MFA && !u.Disabled {
			noMFA = append(noMFA, u.Username)
		}
	}
	if len(noMFA) > 0 {
		add("mfa", "Administrators use an authenticator", "fail", "Without one: "+listSome(noMFA, 8), "users")
	} else {
		add("mfa", "Administrators use an authenticator", "pass", "", "")
	}
	if s.SelfSigned {
		add("tls", "A trusted certificate", "warn", "The server uses its own self-signed certificate, so browsers warn until each one trusts it. A certificate from the agency's or a public authority is better (-tls-cert, -tls-key).", "")
	} else {
		add("tls", "A trusted certificate", "pass", "", "")
	}
	// The map covers every station.
	var area struct {
		lat, lon, r float64
		name        string
	}
	if s.Maps != nil {
		if m := s.Maps.Store(); m != nil {
			area.lat, area.lon, area.r, area.name = m.Area.Lat, m.Area.Lon, m.Area.RadiusM, m.Area.Name
		}
	}
	if area.name == "" {
		add("map", "The recorded map covers every station", "fail", "No map is recorded: drive times are straight lines", "map")
	} else {
		var out []string
		for _, x := range stations {
			if (x.Lat != 0 || x.Lon != 0) && geo.Distance(area.lat, area.lon, x.Lat, x.Lon) > area.r {
				out = append(out, x.Name)
			}
		}
		if len(out) > 0 {
			add("map", "The recorded map covers every station", "fail", "Outside "+area.name+": "+listSome(out, 5), "map")
		} else {
			add("map", "The recorded map covers every station", "pass", area.name, "")
		}
	}
	// Test calls at every station: the station's own unit should come first for what it can do.
	add(s.testCalls(stations, types, stationUnits))
	// BitVoice.
	if s.BV != nil {
		h := s.BV.Health()
		switch {
		case !h.Configured:
			add("phone", "The phone system is connected", "warn", "BitVoice is not set up: call-takers type each caller's number and location", "integrations")
		case !h.Connected:
			add("phone", "The phone system is connected", "fail", "BitVoice is set up but not connected: "+redactText(h.LastError), "integrations")
		default:
			add("phone", "The phone system is connected", "pass", "BitVoice "+h.Version, "")
		}
	}
	// Paging: a test page that reached someone, in the last 30 days.
	in := s.integCopy()
	if len(in.Paging.Gateways) == 0 && !(in.Extra.StationAlerts && in.Extra.StationGatewayURL != "") {
		add("paging", "Station alerting and paging work", "warn", "Neither is set up: stations are alerted by radio", "paging")
	} else if len(in.Paging.Gateways) > 0 {
		var last time.Time
		s.Eng.Read(func(st *cad.State) {
			for _, p := range st.Pages {
				for _, r := range p.Results {
					if r.OK && r.At.After(last) {
						last = r.At
					}
				}
			}
		})
		if last.IsZero() || s.Eng.Now().Sub(last) > 30*24*time.Hour {
			add("paging", "Station alerting and paging work", "warn", "No page has been delivered in the last 30 days: send a test page from Paging groups", "paging")
		} else {
			add("paging", "Station alerting and paging work", "pass", "Last page delivered "+last.Format("2 Jan 15:04"), "")
		}
	} else {
		add("paging", "Station alerting and paging work", "pass", "Station alerting is set up", "")
	}
	// The other sites.
	sv := s.sitesView()
	switch {
	case len(sv.Peers) == 0:
		add("sites", "A second site is copying", "warn", "One site only: if this server fails, there is no CAD until it is back", "sites")
	default:
		var bad []string
		for _, p := range sv.Peers {
			okp := false
			for _, h := range sv.Health {
				if h.Name == p.Name && h.OK && s.Eng.Now().Sub(h.Last) < 2*time.Minute {
					okp = true
				}
			}
			if !okp {
				bad = append(bad, p.Name)
			}
		}
		if len(bad) > 0 {
			add("sites", "A second site is copying", "fail", "Not copying in the last 2 minutes: "+strings.Join(bad, ", "), "sites")
		} else {
			add("sites", "A second site is copying", "pass", plural(len(sv.Peers), "other site")+" up to date", "")
		}
	}
	return g
}

// restoreCheck copies the ledger, replays the copy on its own and compares it with the running system.
func (s *Server) restoreCheck(path string) (id, title, result, detail, fix string) {
	id, title = "restore", "The ledger restores"
	var seq uint64
	var counts [4]int
	s.Eng.Read(func(st *cad.State) {
		seq, counts = st.Seq, [4]int{len(st.Stations), len(st.Units), len(st.CallTypes), len(st.Incidents)}
	})
	dir, err := os.MkdirTemp("", "bitdispatcher-restore-")
	if err != nil {
		return id, title, "fail", err.Error(), ""
	}
	defer os.RemoveAll(dir)
	start := time.Now()
	src, err := os.Open(path)
	if err != nil {
		return id, title, "fail", err.Error(), ""
	}
	cp := filepath.Join(dir, "ledger.jsonl")
	dst, err := os.OpenFile(cp, os.O_CREATE|os.O_WRONLY, 0o600)
	if err == nil {
		_, err = io.Copy(dst, src)
		dst.Close()
	}
	src.Close()
	if err != nil {
		return id, title, "fail", "copying the ledger: " + err.Error(), ""
	}
	eng, err := cad.Open(cp)
	if err != nil {
		return id, title, "fail", "the copy does not replay: " + err.Error(), "ledger"
	}
	defer eng.Close()
	var rseq uint64
	var rc [4]int
	eng.Read(func(st *cad.State) {
		rseq, rc = st.Seq, [4]int{len(st.Stations), len(st.Units), len(st.CallTypes), len(st.Incidents)}
	})
	took := time.Since(start).Round(time.Millisecond)
	if rseq < seq {
		return id, title, "fail", fmt.Sprintf("the copy stops at event %d; the system is at %d", rseq, seq), ""
	}
	if rseq == seq && rc != counts {
		return id, title, "fail", fmt.Sprintf("the copy differs: %d stations, %d units, %d call types, %d incidents; the system has %d, %d, %d, %d",
			rc[0], rc[1], rc[2], rc[3], counts[0], counts[1], counts[2], counts[3]), ""
	}
	return id, title, "pass", fmt.Sprintf("A copy of %d events replayed in %s: %d stations, %d units, %d call types, %d incidents. This proves the ledger restores; it does not check copies kept elsewhere.",
		rseq, took, rc[0], rc[1], rc[2], rc[3]), ""
}

// testCalls makes a test call of every call type with a run card at every station, without writing
// anything, and checks that each is recommended units by road, with the station's own unit first for
// what it can do.
func (s *Server) testCalls(stations []*cad.Station, types []string, own map[string][]string) (id, title, result, detail, fix string) {
	id, title, fix = "calls", "Test calls at every station", "calltypes"
	sort.Slice(stations, func(i, j int) bool { return stations[i].Name < stations[j].Name })
	sort.Strings(types)
	var calls []cad.TestCall
	var at []*cad.Station
	for _, x := range stations {
		if (x.Lat == 0 && x.Lon == 0) || len(own[x.ID]) == 0 {
			continue
		}
		for _, t := range types {
			calls = append(calls, cad.TestCall{Type: t, Lat: x.Lat, Lon: x.Lon})
			at = append(at, x)
		}
	}
	if len(calls) == 0 {
		return id, title, "fail", "No station has both a place on the map and units, or no call type has a run card", "stations"
	}
	const most = 2000
	if len(calls) > most {
		calls, at = calls[:most], at[:most]
	}
	recs := s.Eng.TryRecommend(calls)
	var notOwn []string
	short := map[string]map[string]int{} // call type -> capability -> most short anywhere
	straight := false
	for i, rec := range recs {
		x := at[i]
		if strings.HasPrefix(rec.Estimate, "Straight line") {
			straight = true
		}
		for _, l := range rec.Lines {
			if l.Short > 0 {
				if short[rec.Type] == nil {
					short[rec.Type] = map[string]int{}
				}
				short[rec.Type][l.Capability] = max(short[rec.Type][l.Capability], l.Short)
			}
			if len(l.Units) == 0 {
				continue
			}
			// The station's own unit with this capability, if it has an available one, should come first.
			first := l.Units[0].Unit
			if slices.Contains(own[x.ID], first) {
				continue
			}
			for _, c := range append(append([]cad.Candidate{}, l.Units...), l.Next...) {
				if slices.Contains(own[x.ID], c.Unit) {
					notOwn = append(notOwn, fmt.Sprintf("%s at %s: %s first for %s, before the station's own %s", rec.Type, x.Name, first, l.Capability, c.Unit))
					break
				}
			}
		}
	}
	detail = fmt.Sprintf("%d test calls (%d call types at %d stations), nothing written to the ledger", len(recs), len(types), len(recs)/max(1, len(types)))
	result = "pass"
	if straight {
		result, detail = "warn", detail+". Drive times are straight lines: no road map"
	}
	if len(notOwn) > 0 {
		result, detail = "warn", detail+fmt.Sprintf(". %d where another station's unit comes before the station's own (a response plan, or a unit's position, may be the reason): %s", len(notOwn), listSome(notOwn, 4))
	}
	if len(short) > 0 {
		// The same at every station: the agency does not have the units the run card asks for.
		var ss []string
		for t, caps := range short {
			var cs []string
			for c, n := range caps {
				cs = append(cs, fmt.Sprintf("%d %s", n, c))
			}
			sort.Strings(cs)
			ss = append(ss, t+" ("+strings.Join(cs, ", ")+")")
		}
		result, detail = "warn", detail+fmt.Sprintf(". %s cannot be filled with the units available now, short of: %s. Change the run card, or remove call types the agency does not take",
			plural(len(ss), "call type"), listSome(ss, 6))
	}
	return
}

// The support bundle.

var (
	reSecretKey = regexp.MustCompile(`(?i)(token|secret|password|passwd|apikey|api_key|key$|hash|salt|totp|credential|cookie|signature|bearer|auth$|private)`)
	reEmail     = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	rePhone     = regexp.MustCompile(`\(?\b\d{3}\)?[-. ]\d{3}[-. ]\d{4}\b|\+\d{10,15}\b|\b1?\d{10}\b`)
	reURLQuery  = regexp.MustCompile(`(https?://)([^/\s@]+@)?([^\s?#"]+)\?[^\s"]*`)
	reURLUser   = regexp.MustCompile(`(https?://)[^/\s@]+@`)
	reTokenish  = regexp.MustCompile(`\b(bv_|sk-|AKIA|ghp_|xox)[A-Za-z0-9_\-]{6,}`)
	reLongHex   = regexp.MustCompile(`\b[A-Za-z0-9_\-]{40,}`)
)

// redactText removes what could be a secret or a person from a line of text: e-mail addresses, phone
// numbers, query strings and passwords in addresses, and anything that looks like a token.
func redactText(t string) string {
	t = reURLQuery.ReplaceAllString(t, "$1$3?…")
	t = reURLUser.ReplaceAllString(t, "$1")
	t = reTokenish.ReplaceAllString(t, "(token)")
	t = reLongHex.ReplaceAllString(t, "(long value)")
	t = reEmail.ReplaceAllString(t, "(e-mail)")
	t = rePhone.ReplaceAllString(t, "(number)")
	return t
}

// redactValue walks a decoded JSON value: a value under a key that names a secret becomes "(set)" (or
// stays empty), and every string is passed through redactText.
func redactValue(v any, key string) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = redactValue(e, k)
		}
		return x
	case []any:
		for i, e := range x {
			x[i] = redactValue(e, key)
		}
		return x
	case string:
		if x == "" {
			return x
		}
		if reSecretKey.MatchString(key) {
			return "(set)"
		}
		return redactText(x)
	case float64, bool, nil:
		if reSecretKey.MatchString(key) && x != nil && x != false && x != 0.0 {
			return "(set)"
		}
		return x
	}
	return v
}

func redactJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var g any
	if err := json.Unmarshal(raw, &g); err != nil {
		return nil, err
	}
	return json.MarshalIndent(redactValue(g, ""), "", "  ")
}

const bundleReadme = `bitdispatcher support bundle
============================

An administrator downloaded this from Admin > Support. bitdispatcher did not send it anywhere; whoever
downloaded it decides whether to send it, and to whom. Read the files before you send them.

What is in it
  system.json        version, operating system, uptime, the ledger's size and check, disk space
  setup.json         the setup checklist
  golive.json        the last go-live check
  health.json        the phone system link, the other sites, the map, paging gateways (names only)
  counts.json        how many of each thing there are, and the kinds of events in the last 7 days
  settings.json      the agency's settings
  integrations.json  the connections, with every token, password and key replaced by "(set)"
  config.json        stations, units, call types and response plans
  log.txt            the server's recent warnings and errors

What is not in it
  No calls, incidents, transcripts, recordings, caller names, numbers or locations, no premise notes,
  no usernames, no passwords, tokens or keys. E-mail addresses, phone numbers and anything that looks
  like a token are removed from every file.

What to read before sending
  settings.json and config.json hold whatever the agency typed into its settings, stations, units and
  call types (a call type's list of people to notify, for example). Numbers and e-mail addresses are
  removed, but a name typed there stays. Remove anything you would not send.

The bundle's download is recorded in the ledger (support.bundle), with who downloaded it.
`

// supportBundle builds the zip, and lists its files for the ledger.
func (s *Server) supportBundle() ([]byte, []string, error) {
	now := s.Eng.Now()
	in := s.integCopy()
	var settings cad.Settings
	var cfg struct {
		Stations  []*cad.Station      `json:"stations"`
		Units     []cad.UnitDef       `json:"units"`
		CallTypes []*cad.CallType     `json:"callTypes"`
		Plans     []*cad.ResponsePlan `json:"plans"`
	}
	counts := map[string]any{}
	var seq uint64
	s.Eng.Read(func(st *cad.State) {
		settings, seq = st.Settings, st.Seq
		for _, x := range st.Stations {
			cfg.Stations = append(cfg.Stations, x)
		}
		for _, u := range st.Units {
			cfg.Units = append(cfg.Units, u.UnitDef)
		}
		for _, c := range st.CallTypes {
			cfg.CallTypes = append(cfg.CallTypes, c)
		}
		for _, p := range st.Plans {
			cfg.Plans = append(cfg.Plans, p)
		}
		open := 0
		for _, x := range st.Incidents {
			if x.Closed.IsZero() {
				open++
			}
		}
		byStatus := map[string]int{}
		for _, u := range st.Units {
			byStatus[u.Status]++
		}
		counts["stations"], counts["units"], counts["unitsByStatus"], counts["callTypes"] = len(st.Stations), len(st.Units), byStatus, len(st.CallTypes)
		counts["incidents"], counts["incidentsOpen"], counts["calls"], counts["premises"] = len(st.Incidents), open, len(st.Calls), len(st.Premises)
		counts["plans"], counts["facilities"], counts["geofences"], counts["agencies"] = len(st.Plans), len(st.Facilities), len(st.Geofences), len(st.Agencies)
	})
	sort.Slice(cfg.Stations, func(i, j int) bool { return cfg.Stations[i].ID < cfg.Stations[j].ID })
	sort.Slice(cfg.Units, func(i, j int) bool { return cfg.Units[i].ID < cfg.Units[j].ID })
	sort.Slice(cfg.CallTypes, func(i, j int) bool { return cfg.CallTypes[i].Code < cfg.CallTypes[j].Code })
	roles, mfa := map[string]int{}, 0
	for _, u := range s.Auth.List() {
		roles[u.Role]++
		if u.MFA {
			mfa++
		}
	}
	counts["usersByRole"], counts["usersWithAuthenticator"] = roles, mfa
	// The kinds of events in the last 7 days: types and numbers only, no contents.
	path := s.Eng.LedgerPath()
	kinds := map[string]int{}
	if evs, err := ledger.ReadAll(path); err == nil {
		since := now.Add(-7 * 24 * time.Hour)
		for i := len(evs) - 1; i >= 0 && evs[i].At.After(since); i-- {
			kinds[evs[i].Type]++
		}
	}
	counts["eventsLast7Days"] = kinds
	var ledgerSize int64
	if fi, err := os.Stat(path); err == nil {
		ledgerSize = fi.Size()
	}
	system := map[string]any{
		"version": s.Version, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "cpus": runtime.NumCPU(),
		"now": now.Format(time.RFC3339), "timezone": now.Location().String(), "started": s.started.Format(time.RFC3339),
		"uptime": now.Sub(s.started).Round(time.Second).String(), "ledgerEvents": seq, "ledgerBytes": ledgerSize,
		"ledgerCheck": ledger.Verify(path), "site": s.Eng.Site(), "selfSignedCertificate": s.SelfSigned,
	}
	if free, ok := freeBytes(s.DataDir); ok {
		system["diskFreeGB"] = math.Round(float64(free)/(1<<30)*10) / 10
	}
	health := map[string]any{}
	if s.BV != nil {
		h := s.BV.Health()
		health["bitvoice"] = map[string]any{"configured": h.Configured, "connected": h.Connected, "since": h.Since, "lastEvent": h.LastEvent,
			"lastError": h.LastError, "lastErrorAt": h.LastErrorAt, "events": h.Events, "version": h.Version, "deliveryFailures": len(h.Failures)}
	}
	sv := s.sitesView()
	health["sites"] = map[string]any{"name": sv.Name, "prefix": sv.Prefix, "peers": len(sv.Peers), "health": sv.Health}
	if s.Maps != nil {
		if m := s.Maps.Store(); m != nil {
			health["map"] = map[string]any{"area": m.Area, "built": m.Built, "stats": m.Stats}
		}
	}
	gw := []map[string]string{}
	for _, g := range in.Paging.Gateways {
		gw = append(gw, map[string]string{"name": g.Name, "kind": g.Kind})
	}
	health["pagingGateways"] = gw
	health["stationAlerting"] = in.Extra.StationAlerts && in.Extra.StationGatewayURL != ""
	health["aliListen"] = in.ALIListen != ""
	health["ncic"] = settings.NCICMode
	// Who did what is a person: the bundle says how many, not who.
	var goLive *cad.GoLiveResult
	s.Eng.Read(func(st *cad.State) {
		if st.GoLive != nil {
			g := *st.GoLive
			g.By, g.Checks = "", append([]cad.GoLiveItem(nil), g.Checks...)
			for i, c := range g.Checks {
				if c.ID == "mfa" && c.Result != "pass" {
					g.Checks[i].Detail = fmt.Sprintf("%d administrators without one", strings.Count(c.Detail, ",")+1)
				}
			}
			goLive = &g
		}
	})
	setup := s.setupView()
	for i := range setup.Steps {
		x := &setup.Steps[i]
		x.Tick = nil
		switch x.ID {
		case "users":
			if j := strings.Index(x.Detail, "; administrators without"); j >= 0 {
				x.Detail = x.Detail[:j] + "; some administrators have no authenticator"
			}
		case "demo", "golive":
			x.Detail = ""
		}
	}
	setup.GoLive = nil
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	var files []string
	put := func(name string, data []byte) error {
		f, err := z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: now})
		if err != nil {
			return err
		}
		files = append(files, name)
		_, err = f.Write(data)
		return err
	}
	if err := put("README.txt", []byte(bundleReadme)); err != nil {
		return nil, nil, err
	}
	for _, f := range []struct {
		name string
		v    any
	}{{"system.json", system}, {"setup.json", setup}, {"golive.json", goLive}, {"health.json", health}, {"counts.json", counts},
		{"settings.json", settings}, {"integrations.json", in}, {"config.json", cfg}} {
		data, err := redactJSON(f.v)
		if err != nil {
			return nil, nil, err
		}
		if err := put(f.name, data); err != nil {
			return nil, nil, err
		}
	}
	var lines []string
	if s.Logs != nil {
		for _, l := range s.Logs.Lines() {
			lines = append(lines, redactText(l))
		}
	}
	if len(lines) == 0 {
		lines = []string{"(no warnings or errors since the server started)"}
	}
	if err := put("log.txt", []byte(strings.Join(lines, "\n")+"\n")); err != nil {
		return nil, nil, err
	}
	if err := z.Close(); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), files, nil
}
