package api

// Drones bitdispatcher flies itself: each is reached over MAVLink (PX4 or ArduPilot) and its camera over
// RTSP. A dispatcher asks for a drone on an incident; the drone's remote pilot approves the launch (unless
// the agency turns approval off for a drone, and never for the launch rule); bitdispatcher sends it to
// the incident, shows its picture on the incident and the map, records the picture for the incident with
// checkpoints in the ledger, and brings it back when told. The pilot keeps the drone's own controller and
// can take over at any time. docs/DRONES.md.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/drone"
)

// DroneConf is one drone, set in Admin › Drones.
type DroneConf struct {
	ID         string   `json:"id"`   // D1: short, on the map and in the record
	Name       string   `json:"name"` // Drone 1, North station
	MAVLink    string   `json:"mavlink"`
	SigningKey string   `json:"signingKey,omitempty"` // MAVLink 2 signing, 64 hex digits
	SystemID   int      `json:"systemId,omitempty"`   // 0: the first autopilot heard
	Video      string   `json:"video,omitempty"`      // rtsp://...
	AltM       float64  `json:"altM"`                 // flying height above the take-off point
	MinBattery int      `json:"minBattery"`           // not sent below this
	Approval   bool     `json:"approval"`             // a remote pilot approves each launch
	Pilots     []string `json:"pilots,omitempty"`     // usernames who approve and fly it; empty: supervisors
	Record     bool     `json:"record"`               // record the picture for the incident
	Off        bool     `json:"off,omitempty"`
}

func (c DroneConf) alt() float64 {
	if c.AltM <= 0 {
		return 60
	}
	return c.AltM
}

func (c DroneConf) minBattery() int {
	if c.MinBattery <= 0 {
		return 40
	}
	return c.MinBattery
}

const masked = "••••••••"

var reDroneID = regexp.MustCompile(`^[A-Z0-9-]{1,12}$`)

// maskVideo hides a password in a video address.
func maskVideo(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.User == nil {
		return s
	}
	if _, ok := u.User.Password(); !ok {
		return s
	}
	u.User = url.UserPassword(u.User.Username(), "PASSWORD")
	return strings.Replace(u.String(), "PASSWORD", masked, 1)
}

// unmaskVideo puts back the password an administrator did not retype.
func unmaskVideo(s, old string) string {
	if !strings.Contains(s, masked) {
		return s
	}
	o, err := url.Parse(old)
	if err != nil || o.User == nil {
		return strings.Replace(s, ":"+masked, "", 1)
	}
	p, _ := o.User.Password()
	return strings.Replace(s, masked, url.UserPassword("x", p).String()[2:], 1)
}

func (c DroneConf) view() DroneConf {
	v := c
	if v.SigningKey != "" {
		v.SigningKey = masked
	}
	v.Video = maskVideo(v.Video)
	return v
}

func (c *DroneConf) check() error {
	c.ID, c.Name = strings.ToUpper(strings.TrimSpace(c.ID)), strings.TrimSpace(c.Name)
	if !reDroneID.MatchString(c.ID) {
		return problemf("id", "a drone's id is 1 to 12 capital letters, digits or dashes, such as D1")
	}
	if c.Name == "" {
		c.Name = "Drone " + c.ID
	}
	if _, err := drone.ParseEndpoint(c.MAVLink); err != nil {
		return problemf("mavlink", "%s", err)
	}
	if _, err := drone.ParseKey(c.SigningKey); err != nil {
		return problemf("signingKey", "%s", err)
	}
	if c.SystemID < 0 || c.SystemID > 255 {
		return problemf("systemId", "a MAVLink system id is 1 to 255, or 0 for the first heard")
	}
	if c.Video != "" {
		u, err := url.Parse(c.Video)
		if err != nil || (u.Scheme != "rtsp" && u.Scheme != "rtsps") || u.Host == "" {
			return problemf("video", "the video address is rtsp://host:port/path")
		}
	}
	// Part 107 keeps a drone at or below 400 feet (about 120 m) above the ground.
	if c.AltM != 0 && (c.AltM < 10 || c.AltM > 120) {
		return problemf("altM", "the flying height is 10 to 120 metres (400 feet)")
	}
	if c.MinBattery != 0 && (c.MinBattery < 10 || c.MinBattery > 95) {
		return problemf("minBattery", "the lowest battery to send it with is 10 to 95 percent")
	}
	ps := []string{}
	for _, p := range c.Pilots {
		if p = strings.TrimSpace(p); p != "" && !slices.Contains(ps, p) {
			ps = append(ps, p)
		}
	}
	c.Pilots = ps
	return nil
}

// ---- the running drones ----

type droneRun struct {
	cfg  DroneConf
	link *drone.Link
	hub  *drone.Hub
	stop context.CancelFunc

	mu        sync.Mutex
	incident  string // the incident it is flying for
	tLat      float64
	tLon      float64
	mission   bool // sent to the incident and not yet back
	flying    bool // FlyTo is under way
	returning bool
	wasUp     bool // it has been in the air on this mission
	pending   string
	pendBy    string
	pilot     string // who approved the launch
	detail    string
	recording bool
	stopRec   chan struct{}
	recDone   chan struct{}
	sent      cad.Drone
	sentAt    time.Time
}

type droneRuntime struct {
	mu   sync.Mutex
	runs map[string]*droneRun
	ctx  context.Context
	// verified caches clip files whose bytes were checked against the ledger: path → size, sha256.
	verified sync.Map
}

func (s *Server) droneConfs() []DroneConf {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]DroneConf(nil), s.integ.Drones...)
}

func (s *Server) droneRunFor(id string) *droneRun {
	s.drones.mu.Lock()
	defer s.drones.mu.Unlock()
	return s.drones.runs[id]
}

func (s *Server) droneRunList() []*droneRun {
	s.drones.mu.Lock()
	defer s.drones.mu.Unlock()
	out := []*droneRun{}
	for _, r := range s.drones.runs {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].cfg.ID < out[j].cfg.ID })
	return out
}

func (s *Server) directDrones() bool {
	for _, c := range s.droneConfs() {
		if !c.Off {
			return true
		}
	}
	return false
}

// runDrones starts the configured drones and keeps their state on the board.
func (s *Server) runDrones(ctx context.Context) {
	if s.isTraining {
		return // a training board never flies a real drone
	}
	s.drones.mu.Lock()
	s.drones.ctx = ctx
	s.drones.mu.Unlock()
	s.applyDrones()
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			for _, r := range s.droneRunList() {
				s.endRecording(r)
				r.stop()
				r.link.Close()
			}
			return
		case <-t.C:
		}
		for _, r := range s.droneRunList() {
			s.droneTick(r)
		}
	}
}

// applyDrones starts, stops and restarts drones to match the configuration.
func (s *Server) applyDrones() {
	s.drones.mu.Lock()
	defer s.drones.mu.Unlock()
	if s.drones.ctx == nil {
		return
	}
	if s.drones.runs == nil {
		s.drones.runs = map[string]*droneRun{}
	}
	want := map[string]DroneConf{}
	for _, c := range s.droneConfs() {
		if !c.Off {
			want[c.ID] = c
		}
	}
	for id, r := range s.drones.runs {
		c, ok := want[id]
		same := ok && c.MAVLink == r.cfg.MAVLink && c.SigningKey == r.cfg.SigningKey && c.SystemID == r.cfg.SystemID && c.Video == r.cfg.Video
		if same {
			r.mu.Lock()
			r.cfg = c
			r.mu.Unlock()
			delete(want, id)
			continue
		}
		go func(r *droneRun) {
			s.endRecording(r)
			r.stop()
			r.link.Close()
		}(r)
		delete(s.drones.runs, id)
		if !ok {
			lat, lon := r.sent.Lat, r.sent.Lon
			s.Eng.UpdateDrone(cad.Actor{User: "drone"}, cad.Drone{ID: id, Name: r.cfg.Name, Status: "offline", Lat: lat, Lon: lon, Direct: true, Detail: "taken out of service in Admin › Drones"})
		}
	}
	for id, c := range want {
		l, err := drone.Dial(drone.LinkConfig{Endpoint: c.MAVLink, SigningKey: c.SigningKey, SystemID: c.SystemID})
		if err != nil {
			s.Log.Warn("drone link", "drone", id, "err", err)
			continue
		}
		ctx, cancel := context.WithCancel(s.drones.ctx)
		r := &droneRun{cfg: c, link: l, hub: drone.NewHub(), stop: cancel}
		if c.Video != "" {
			go drone.ReadRTSP(ctx, c.Video, r.hub)
		}
		s.drones.runs[id] = r
	}
}

// droneTick works out what the drone is doing, puts it on the board, and starts or stops recording.
func (s *Server) droneTick(r *droneRun) {
	tel := r.link.Telemetry()
	r.mu.Lock()
	air := tel.Airborne()
	if air && r.mission {
		r.wasUp = true
	}
	status := "docked"
	switch {
	case !tel.Connected:
		status = "offline"
	case r.returning && !air && !tel.Armed:
		r.returning, r.mission, r.wasUp, r.incident, r.pilot = false, false, false, "", ""
	case r.returning:
		status = "returning"
	case r.mission && r.wasUp && !air && !tel.Armed:
		// Landed during a mission: the pilot took it over and brought it down.
		r.mission, r.wasUp, r.incident, r.pilot = false, false, "", ""
		r.detail = "landed before returning: the pilot has it"
	case r.mission && (!air || tel.AltRel < r.cfg.alt()*0.8 && drone.Distance(tel.Lat, tel.Lon, r.tLat, r.tLon) > 60):
		status = "launching"
	case r.mission && drone.Distance(tel.Lat, tel.Lon, r.tLat, r.tLon) > 40:
		status = "enroute"
	case r.mission:
		status = "onscene"
	case air:
		status = "airborne"
	}
	if status == "docked" && r.returning {
		status = "returning"
	}
	d := cad.Drone{ID: r.cfg.ID, Name: r.cfg.Name, Status: status, Incident: r.incident, Direct: true, Live: r.hub.Live(), Recording: r.recording,
		Heading: tel.Heading, SpeedMS: tel.SpeedMS, AltM: tel.AltRel, Battery: max(tel.Battery, 0), Detail: r.detail, Pilot: r.pilot,
		Pending: r.pending, Pilots: r.cfg.Pilots, Approval: r.cfg.Approval}
	if r.cfg.Video != "" {
		d.VideoURL = "/api/drones/" + r.cfg.ID + "/live"
	}
	if tel.HavePos {
		lat, lon := tel.Lat, tel.Lon
		d.Lat, d.Lon = &lat, &lon
	} else if r.sent.Lat != nil {
		d.Lat, d.Lon = r.sent.Lat, r.sent.Lon
	}
	wantRec := r.cfg.Record && r.incident != "" && d.Live && !r.recording && status != "docked" && status != "offline"
	stopRec := r.recording && (r.incident == "" || !d.Live)
	if r.incident != "" && !d.Live && r.cfg.Video != "" {
		if _, _, e := r.hub.Status(); e != "" {
			d.Detail = firstNonEmptyStr(d.Detail, "video: "+e)
		}
	}
	// Send only changes, and the position at most once a second.
	changed := !sameDrone(d, r.sent) || (d.Lat != nil && time.Since(r.sentAt) >= time.Second && (r.sent.Lat == nil || *d.Lat != *r.sent.Lat || *d.Lon != *r.sent.Lon || d.Battery != r.sent.Battery))
	inc := r.incident
	if changed {
		r.sent, r.sentAt = d, time.Now()
	}
	r.mu.Unlock()
	if changed {
		if err := s.Eng.UpdateDrone(cad.Actor{User: "drone"}, d); err != nil {
			s.Log.Warn("drone update", "drone", d.ID, "err", err)
		}
	}
	if stopRec {
		s.endRecording(r)
	}
	if wantRec {
		s.startRecording(r, inc)
	}
}

func sameDrone(a, b cad.Drone) bool {
	return a.Status == b.Status && a.Incident == b.Incident && a.Live == b.Live && a.Recording == b.Recording && a.Detail == b.Detail &&
		a.Pilot == b.Pilot && a.Pending == b.Pending && a.Name == b.Name && a.VideoURL == b.VideoURL && a.Approval == b.Approval &&
		slices.Equal(a.Pilots, b.Pilots) && (a.Lat == nil) == (b.Lat == nil)
}

// ---- recording ----

func (s *Server) droneVideoDir(number string) string {
	dir := s.DataDir
	if s.liveServer != nil {
		dir = s.liveServer.DataDir
	}
	return filepath.Join(dir, "drone-video", number)
}

func (s *Server) startRecording(r *droneRun, number string) {
	st, gop, ch, cancel := r.hub.Subscribe()
	if st == nil {
		return
	}
	id := time.Now().UTC().Format("20060102T150405Z") + "-" + r.cfg.ID
	rec, err := drone.NewRecorder(filepath.Join(s.droneVideoDir(number), id+".mp4"), st)
	if err != nil {
		cancel()
		s.Log.Warn("drone recording", "drone", r.cfg.ID, "err", err)
		return
	}
	stop, done := make(chan struct{}), make(chan struct{})
	r.mu.Lock()
	r.recording, r.stopRec, r.recDone = true, stop, done
	name := r.cfg.Name
	r.mu.Unlock()
	clip := func(final bool) {
		var ck drone.Checkpoint
		if final {
			ck, err = rec.Close()
		} else {
			ck, err = rec.Checkpoint()
		}
		if err != nil {
			s.Log.Warn("drone recording", "drone", r.cfg.ID, "err", err)
		}
		if !rec.Started() {
			if final {
				os.Remove(rec.Path)
			}
			return
		}
		if err := s.Eng.DroneClip(cad.Actor{User: "drone"}, number, cad.DroneClip{ID: id, Drone: r.cfg.ID, Name: name, Start: ck.Start, End: ck.End,
			Size: ck.Size, SHA256: ck.SHA256, Final: final, Type: "video/mp4", Codec: rec.Codec}); err != nil {
			s.Log.Warn("drone recording", "drone", r.cfg.ID, "err", err)
		}
	}
	go func() {
		defer func() {
			cancel()
			clip(true)
			r.mu.Lock()
			if r.recDone == done {
				r.recording, r.stopRec, r.recDone = false, nil, nil
			}
			r.mu.Unlock()
			close(done)
		}()
		for _, f := range gop {
			rec.Add(f)
		}
		tick := time.NewTicker(10 * time.Second)
		defer tick.Stop()
		started := time.Now()
		for {
			select {
			case f, ok := <-ch:
				if !ok {
					return
				}
				if err := rec.Add(f); err != nil {
					s.Log.Warn("drone recording", "drone", r.cfg.ID, "err", err)
					return
				}
			case <-tick.C:
				if time.Since(started) > 30*time.Minute {
					return // a new clip every half hour keeps each file a manageable size
				}
				clip(false)
			case <-stop:
				return
			}
		}
	}()
}

func (s *Server) endRecording(r *droneRun) {
	r.mu.Lock()
	stop, done := r.stopRec, r.recDone
	r.stopRec = nil
	r.mu.Unlock()
	if stop != nil {
		close(stop)
	}
	// Wait for the clip to be closed and its last checkpoint written.
	if done != nil {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}
}

// ---- launching ----

// launchDirect picks the nearest ready drone for an incident and sends it, or asks its pilot to.
func (s *Server) launchDirect(ctx context.Context, a cad.Actor, number, want string, auto bool) (string, error) {
	var lat, lon float64
	ok := false
	s.Eng.Read(func(st *cad.State) {
		if in, found := st.Incidents[number]; found && in.Location.HasPoint() && in.Status != cad.Closed {
			lat, lon, ok = *in.Location.Lat, *in.Location.Lon, true
		}
	})
	if !ok {
		return "", errors.New("the incident is not on the map yet, or it is closed: place it first")
	}
	var best *droneRun
	bestD, why := 0.0, []string{}
	for _, r := range s.droneRunList() {
		if want != "" && r.cfg.ID != want {
			continue
		}
		tel := r.link.Telemetry()
		r.mu.Lock()
		busy := r.mission || r.pending != "" || r.returning
		r.mu.Unlock()
		switch {
		case !tel.Connected:
			why = append(why, r.cfg.ID+" is not connected")
		case !tel.HavePos:
			why = append(why, r.cfg.ID+" has no position fix")
		case busy:
			why = append(why, r.cfg.ID+" is busy")
		case tel.Airborne():
			why = append(why, r.cfg.ID+" is already flying")
		case tel.Battery >= 0 && tel.Battery < r.cfg.minBattery():
			why = append(why, fmt.Sprintf("%s has %d%% battery (needs %d%%)", r.cfg.ID, tel.Battery, r.cfg.minBattery()))
		default:
			if d := drone.Distance(tel.Lat, tel.Lon, lat, lon); best == nil || d < bestD {
				best, bestD = r, d
			}
		}
	}
	if best == nil {
		if len(why) == 0 {
			why = []string{"no drone is set up in Admin › Drones"}
		}
		detail := "no drone ready: " + strings.Join(why, "; ")
		s.Eng.DroneRequested(a, number, cad.DroneRequest{OK: false, Auto: auto, Detail: detail})
		return "", errors.New(detail)
	}
	r := best
	away := fmt.Sprintf("%.1f km away", bestD/1000)
	r.mu.Lock()
	needs := r.cfg.Approval || auto
	if needs {
		r.pending, r.pendBy = number, a.User
	}
	r.mu.Unlock()
	if needs {
		detail := fmt.Sprintf("%s (%s, %s) is waiting for its remote pilot to approve the launch", r.cfg.ID, r.cfg.Name, away)
		s.Eng.DroneRequested(a, number, cad.DroneRequest{OK: true, Auto: auto, Detail: detail})
		s.droneTick(r)
		return detail, nil
	}
	detail := fmt.Sprintf("%s (%s, %s) launched by %s", r.cfg.ID, r.cfg.Name, away, a.User)
	s.Eng.DroneRequested(a, number, cad.DroneRequest{OK: true, Auto: auto, Detail: detail})
	s.fly(r, number, lat, lon, a.User)
	return detail, nil
}

// fly sends the drone to the incident, in the background (taking off and climbing takes a while).
func (s *Server) fly(r *droneRun, number string, lat, lon float64, pilot string) {
	r.mu.Lock()
	r.incident, r.tLat, r.tLon, r.mission, r.flying, r.returning, r.wasUp = number, lat, lon, true, true, false, false
	r.pending, r.pendBy, r.pilot, r.detail = "", "", pilot, ""
	alt := r.cfg.alt()
	r.mu.Unlock()
	s.droneTick(r)
	go func() {
		ctx, cancel := context.WithTimeout(s.drones.ctx, 2*time.Minute)
		defer cancel()
		err := r.link.FlyTo(ctx, lat, lon, alt)
		r.mu.Lock()
		r.flying = false
		if err != nil {
			r.detail = err.Error()
			if !r.link.Telemetry().Airborne() {
				r.mission, r.incident = false, ""
			}
		}
		r.mu.Unlock()
		if err != nil {
			s.Eng.DroneRequested(cad.Actor{User: "drone"}, number, cad.DroneRequest{OK: false, Detail: r.cfg.ID + ": " + err.Error()})
		}
		s.droneTick(r)
	}()
}

// mayFly is whether a person may approve, return or hold a drone: its pilots, or supervisors when it
// has none named; dispatchers may also bring a drone back or hold it.
func (r *droneRun) mayFly(se *auth.Session, approving bool) bool {
	if slices.Contains(r.cfg.Pilots, se.Username) {
		return true
	}
	if len(r.cfg.Pilots) == 0 && auth.AtLeast(se.Role, auth.Supervisor) {
		return true
	}
	return !approving && auth.AtLeast(se.Role, auth.Dispatcher)
}

func (s *Server) droneRoutes() {
	s.handle("GET /api/admin/drones", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out := []DroneConf{}
		for _, c := range s.droneConfs() {
			out = append(out, c.view())
		}
		writeJSON(w, 200, map[string]any{"drones": out, "telemetry": s.droneTelemetry()})
	})
	s.handle("PUT /api/admin/drones", auth.Admin, s.saveDrones)
	s.handle("POST /api/drones/{id}/{action}", auth.Dispatcher, s.droneAction)
	s.handle("GET /api/drones/{id}/live", auth.Viewer, s.droneLive)
	s.handle("GET /api/incidents/{n}/drone-video/{clip}", auth.Viewer, s.droneClip)
}

func (s *Server) droneTelemetry() map[string]drone.Telemetry {
	out := map[string]drone.Telemetry{}
	for _, r := range s.droneRunList() {
		out[r.cfg.ID] = r.link.Telemetry()
	}
	return out
}

func (s *Server) saveDrones(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		Drones []DroneConf `json:"drones"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	old := map[string]DroneConf{}
	for _, c := range s.droneConfs() {
		old[c.ID] = c
	}
	seen := map[string]bool{}
	for i := range b.Drones {
		c := &b.Drones[i]
		c.ID = strings.ToUpper(strings.TrimSpace(c.ID))
		if c.SigningKey == masked {
			c.SigningKey = old[c.ID].SigningKey
		}
		c.Video = unmaskVideo(c.Video, old[c.ID].Video)
		if err := c.check(); err != nil {
			writeErr(w, err)
			return
		}
		if seen[c.ID] {
			fail(w, 422, "two drones have the id "+c.ID)
			return
		}
		seen[c.ID] = true
	}
	s.mu.Lock()
	n := s.integ
	n.Drones = b.Drones
	data, _ := json.MarshalIndent(n, "", "  ")
	tmp := s.integrationsPath() + ".tmp"
	err := os.WriteFile(tmp, data, 0o600)
	if err == nil {
		err = os.Rename(tmp, s.integrationsPath())
	}
	if err == nil {
		s.integ = n
	}
	s.mu.Unlock()
	if err != nil {
		writeErr(w, err)
		return
	}
	ids := []string{}
	for _, c := range b.Drones {
		ids = append(ids, c.ID)
	}
	s.Eng.Record(a, "admin.drones.changed", map[string]any{"drones": ids})
	s.applyDrones()
	out := []DroneConf{}
	for _, c := range b.Drones {
		out = append(out, c.view())
	}
	writeJSON(w, 200, map[string]any{"drones": out, "telemetry": s.droneTelemetry()})
}

func (s *Server) droneAction(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	run := s.droneRunFor(strings.ToUpper(r.PathValue("id")))
	if run == nil {
		fail(w, 404, "no such drone in service")
		return
	}
	var b struct {
		Why string `json:"why"`
	}
	if r.ContentLength > 0 && !readJSON(w, r, &b) {
		return
	}
	run.mu.Lock()
	pending, inc := run.pending, run.incident
	run.mu.Unlock()
	act := r.PathValue("action")
	switch act {
	case "approve", "decline":
		if !run.mayFly(se, true) {
			fail(w, 403, "only "+run.cfg.ID+"'s remote pilots approve its launches")
			return
		}
		if pending == "" {
			fail(w, 409, run.cfg.ID+" has no launch waiting")
			return
		}
		if act == "decline" {
			why := strings.TrimSpace(b.Why)
			if why == "" {
				fail(w, 422, "say why the launch is declined (weather, airspace, battery...)")
				return
			}
			run.mu.Lock()
			run.pending, run.pendBy = "", ""
			run.mu.Unlock()
			s.Eng.DroneRequested(a, pending, cad.DroneRequest{OK: false, Detail: run.cfg.ID + " launch declined by " + a.User + ": " + why})
			s.droneTick(run)
			w.WriteHeader(204)
			return
		}
		var lat, lon float64
		ok := false
		s.Eng.Read(func(st *cad.State) {
			if in := st.Incidents[pending]; in != nil && in.Location.HasPoint() && in.Status != cad.Closed {
				lat, lon, ok = *in.Location.Lat, *in.Location.Lon, true
			}
		})
		if !ok {
			run.mu.Lock()
			run.pending = ""
			run.mu.Unlock()
			fail(w, 409, pending+" is closed or no longer on the map")
			return
		}
		s.Eng.DroneRequested(a, pending, cad.DroneRequest{OK: true, Detail: run.cfg.ID + " launch approved by " + a.User})
		s.fly(run, pending, lat, lon, a.User)
		w.WriteHeader(204)
	case "return", "hold":
		if !run.mayFly(se, false) {
			fail(w, 403, "a dispatcher or the drone's pilot does this")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		var err error
		if act == "return" {
			err = run.link.Return(ctx)
		} else {
			err = run.link.Hold(ctx)
		}
		if err != nil {
			fail(w, http.StatusBadGateway, err.Error())
			return
		}
		run.mu.Lock()
		if act == "return" {
			run.returning = true
		} else {
			tel := run.link.Telemetry()
			run.tLat, run.tLon = tel.Lat, tel.Lon
		}
		run.mu.Unlock()
		if inc != "" {
			word := map[string]string{"return": "sent home", "hold": "told to hold where it is"}[act]
			s.Eng.DroneRequested(a, inc, cad.DroneRequest{OK: true, Detail: run.cfg.ID + " " + word + " by " + a.User})
		}
		s.droneTick(run)
		w.WriteHeader(204)
	default:
		fail(w, 404, "no such drone action")
	}
}

// onIncident is whether a field session's unit is on an incident.
func (s *Server) onIncident(se *auth.Session, number string) bool {
	on := false
	s.Eng.Read(func(st *cad.State) {
		if in := st.Incidents[number]; in != nil {
			on = slices.ContainsFunc(in.Units, func(as cad.Assignment) bool { return as.Unit == se.Console })
		}
	})
	return on
}

// droneLive streams a drone's picture as fragmented MP4, from its last key frame, for Media Source
// Extensions in the browser. The codec is in X-Video-Codec.
func (s *Server) droneLive(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	run := s.droneRunFor(strings.ToUpper(r.PathValue("id")))
	if run == nil {
		fail(w, 404, "no such drone in service")
		return
	}
	run.mu.Lock()
	inc := run.incident
	run.mu.Unlock()
	if se.Role == auth.Field && (inc == "" || !s.onIncident(se, inc)) {
		fail(w, 403, "a crew sees a drone's picture over an incident it is on")
		return
	}
	st, gop, ch, cancel := run.hub.Subscribe()
	defer cancel()
	if st == nil {
		fail(w, 404, "no picture from "+run.cfg.ID+" now")
		return
	}
	fl, _ := w.(http.Flusher)
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("X-Video-Codec", st.Codec)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(200)
	write := func(b []byte) bool {
		rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
		_, err := w.Write(b)
		return err == nil
	}
	if !write(st.Init) {
		return
	}
	for _, f := range gop {
		if !write(f.Data) {
			return
		}
	}
	if fl != nil {
		fl.Flush()
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.quit:
			return
		case f, ok := <-ch:
			if !ok || !write(f.Data) {
				return
			}
			if fl != nil {
				fl.Flush()
			}
		}
	}
}

// droneClip serves a recorded clip, checked against the SHA-256 in its last checkpoint in the ledger,
// with ranges so a player can seek.
func (s *Server) droneClip(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	n, id := r.PathValue("n"), r.PathValue("clip")
	if !reIncNumber.MatchString(n) {
		fail(w, 404, "no incident")
		return
	}
	var c *cad.DroneClip
	s.Eng.Read(func(st *cad.State) {
		if in := st.Incidents[n]; in != nil {
			for _, x := range in.DroneClips {
				if x.ID == id {
					v := x
					c = &v
				}
			}
		}
	})
	if c == nil {
		fail(w, 404, "no such recording")
		return
	}
	if se.Role == auth.Field && !s.onIncident(se, n) {
		fail(w, 403, "a crew sees recordings of incidents it is on")
		return
	}
	path := filepath.Join(s.droneVideoDir(n), c.ID+".mp4")
	f, err := os.Open(path)
	if err != nil {
		fail(w, 404, "the recording is missing on this server")
		return
	}
	defer f.Close()
	// A player's first request (no range, or from the start) checks every byte; the ranges it asks for
	// while seeking use that check while the file's size and time are unchanged.
	fi, _ := f.Stat()
	key := fmt.Sprintf("%s|%d|%s|%d|%d", path, c.Size, c.SHA256, fi.Size(), fi.ModTime().UnixNano())
	rg := r.Header.Get("Range")
	if _, ok := s.drones.verified.Load(key); !ok || rg == "" || strings.HasPrefix(rg, "bytes=0-") {
		h := sha256.New()
		if m, err := io.Copy(h, io.LimitReader(f, c.Size)); err != nil || m != c.Size || hex.EncodeToString(h.Sum(nil)) != c.SHA256 {
			fail(w, 409, "the recording was changed since it was made: it does not match the ledger")
			return
		}
		s.drones.verified.Store(key, true)
		f.Seek(0, io.SeekStart)
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; media-src 'self'")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", n+"-"+c.ID+".mp4"))
	http.ServeContent(w, r, "", c.End, io.NewSectionReader(f, 0, c.Size))
}

func problemf(field, format string, a ...any) error {
	return &cad.Problem{Field: field, Message: fmt.Sprintf(format, a...)}
}
