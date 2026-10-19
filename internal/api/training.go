package api

// The training environment: the same program on a ledger of its own, with a copy of the agency's units,
// stations, call types, premises and map, and scripted scenarios that play calls, transcripts, crash data
// and radio traffic into it. A session signed in to train sees only it; nothing reaches the live board.

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/acn"
	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/bitvoice"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// trainer holds the training environment, built when the first person signs in to train.
type trainer struct {
	live *Server
	mu   sync.Mutex
	srv  *Server
}

// TrainingView is what the console shows of the training environment.
type TrainingView struct {
	Scenario string    `json:"scenario,omitempty"`
	Name     string    `json:"name,omitempty"`
	Started  time.Time `json:"started,omitzero"`
	Steps    int       `json:"steps"`
	Played   int       `json:"played"`
}

// trainingFor is the training server for a request from a training session, or nil.
func (s *Server) trainingFor(r *http.Request) *Server {
	if s.trainer == nil || !strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/api/auth/") {
		return nil
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil
	}
	se, ok := s.Auth.Session(c.Value)
	if !ok || !se.Training {
		return nil
	}
	t, err := s.trainer.get()
	if err != nil {
		s.Log.Error("training: cannot start", "err", err)
		return nil
	}
	return t
}

func (t *trainer) get() (*Server, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.srv != nil {
		return t.srv, nil
	}
	srv, err := t.build()
	if err != nil {
		return nil, err
	}
	t.srv = srv
	return srv, nil
}

// reset throws the training board away and builds it again from the live configuration.
func (t *trainer) reset() (*Server, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if old := t.srv; old != nil {
		old.stopPlayer()
		close(old.quit)
		old.Close()
		old.Eng.Close()
		t.srv = nil
	}
	srv, err := t.build()
	if err == nil {
		t.srv = srv
	}
	return srv, err
}

func (t *trainer) build() (*Server, error) {
	live := t.live
	dir := filepath.Join(live.DataDir, "training")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "ledger.jsonl")
	os.Remove(path)
	eng, err := cad.Open(path)
	if err != nil {
		return nil, err
	}
	if err := copyConfig(live.Eng, eng); err != nil {
		eng.Close()
		return nil, err
	}
	srv, err := New(&Server{Eng: eng, Auth: live.Auth, BV: bitvoice.New(eng, live.Log), Log: live.Log.With("env", "training"),
		DataDir: dir, Version: live.Version, Tiles: live.Tiles, Maps: live.Maps, GIS: live.GIS, isTraining: true, quit: make(chan struct{})})
	if err != nil {
		eng.Close()
		return nil, err
	}
	srv.resetTraining = t.reset
	return srv, nil
}

// copyConfig gives the training ledger the live configuration: settings, stations, units (available, where
// they are), call types, premises and users.
func copyConfig(from, to *cad.Engine) error {
	var st cad.Settings
	var stations []cad.Station
	var units []cad.Unit
	var types []cad.CallType
	var prem []cad.Premise
	var users []cad.UserInfo
	from.Read(func(s *cad.State) {
		st = s.Settings
		for _, x := range s.Stations {
			stations = append(stations, *x)
		}
		for _, x := range s.Units {
			units = append(units, *x)
		}
		for _, x := range s.CallTypes {
			types = append(types, *x)
		}
		for _, x := range s.Premises {
			prem = append(prem, *x)
		}
		for _, x := range s.Users {
			users = append(users, *x)
		}
	})
	a := cad.Actor{User: "training"}
	if err := to.SaveSettings(a, st); err != nil {
		return err
	}
	for _, x := range stations {
		if err := to.SaveStation(a, x); err != nil {
			return err
		}
	}
	for _, x := range types {
		if err := to.SaveCallType(a, x); err != nil {
			return err
		}
	}
	for _, u := range units {
		if err := to.SaveUnit(a, u.UnitDef); err != nil {
			return err
		}
		status := cad.Available
		if u.Status == cad.OutOfService {
			status = cad.OutOfService
		}
		to.SetStatus(a, u.ID, status, "", "")
		if u.Position != nil {
			to.UnitPosition(a, u.ID, *u.Position)
		}
	}
	if len(prem) > 0 {
		to.ImportPremises(a, "live configuration", prem)
	}
	for _, u := range users {
		to.Record(a, cad.EvUserSaved, u)
	}
	return nil
}

// ---- scenarios ----

// Scenario is a scripted exercise.
type Scenario struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	About   string   `json:"about"`
	Teaches []string `json:"teaches"`
	Minutes int      `json:"minutes"`
	steps   []step
	expect  expect
}

type expect struct {
	types []string       // one of these call types
	caps  map[string]int // at least this many units with each capability sent
	reply bool           // a reply sent to the caller in their language
}

// step is one thing that happens in a scenario, at seconds after the start. Places are metres east and north
// of the map's centre, so a scenario works on any agency's map.
type step struct {
	at       int
	call     string
	east     float64
	north    float64
	radius   float64
	method   string
	floor    string
	alt      float64
	state    string
	callback string
	name     string
	line     string
	say      []cad.TranscriptLine
	crash    *acn.Crash
	end      bool
	radio    string // what a unit of the incident says
	radioCap string // the capability of the unit that says it
}

func caller(text string) cad.TranscriptLine { return cad.TranscriptLine{Speaker: "caller", Text: text} }
func taker(text string) cad.TranscriptLine  { return cad.TranscriptLine{Speaker: "taker", Text: text} }

func ptr[T any](v T) *T { return &v }

// Scenarios are the built-in exercises.
var Scenarios = []Scenario{
	{ID: "house-fire", Name: "House fire, people inside", Minutes: 4,
		About:   "A kitchen fire spreads; three neighbours call about the same house, one of them from a moving car.",
		Teaches: []string{"group the callers into one incident", "pick the structure fire run card", "notice people trapped (an urgent detail)"},
		expect:  expect{types: []string{"SF"}, caps: map[string]int{"engine": 2, "ladder": 1}},
		steps: []step{
			{at: 0, call: "t-fire-1", east: 420, north: -310, radius: 9, method: "DBH", floor: "2", alt: 318, state: "ringing", callback: "+15405550141", name: "DANIELS, K"},
			{at: 4, call: "t-fire-1", state: "answered", say: []cad.TranscriptLine{taker("911, what is the address of your emergency?"),
				caller("There's a fire in my kitchen, it's spreading, I'm upstairs with my two kids and we can't get down the stairs"),
				taker("Is anyone hurt? Can you get to a window?"), caller("No one's hurt yet but there's a lot of smoke, we're trapped")}},
			{at: 14, call: "t-fire-2", east: 455, north: -290, radius: 25, method: "Cell", state: "answered", callback: "+15405550177",
				say: []cad.TranscriptLine{caller("I'm across the street, the house next to mine is on fire, flames out the back windows")}},
			{at: 24, call: "t-fire-3", east: 520, north: -260, radius: 60, method: "Cell", state: "answered", callback: "+15405550190",
				say: []cad.TranscriptLine{caller("I'm driving past a house fire, lots of black smoke, somebody's yelling from upstairs")}},
			{at: 30, call: "t-fire-3", east: 700, north: -150, radius: 40, method: "Cell"},
			{at: 34, call: "t-fire-3", end: true},
			{at: 60, call: "t-fire-1", say: []cad.TranscriptLine{caller("I can hear the sirens, please hurry")}},
			{at: 120, radio: "On scene, two-storey house, heavy smoke from the second floor, kids reported trapped, starting a search", radioCap: "engine"},
		}},
	{ID: "cardiac-spanish", Name: "Cardiac arrest, Spanish-speaking caller", Minutes: 3,
		About:   "A woman calls in Spanish: her husband has collapsed and is not breathing. The words are translated as she speaks.",
		Teaches: []string{"use the translation and the reply phrases", "start CPR instructions", "a critical call: the supervisor is told"},
		expect:  expect{types: []string{"CARD", "BRTH"}, caps: map[string]int{"als": 1}, reply: true},
		steps: []step{
			{at: 0, call: "t-card-1", east: -380, north: 520, radius: 6, method: "DBH", state: "ringing", callback: "+15405550123", name: "GARCIA, M"},
			{at: 3, call: "t-card-1", state: "answered", say: []cad.TranscriptLine{taker("911, what is the address of your emergency?"),
				caller("Ayuda por favor, mi esposo se cayó y no respira")}},
			{at: 12, call: "t-card-1", say: []cad.TranscriptLine{caller("Estoy en la casa, está en el piso de la cocina, no responde")}},
			{at: 30, call: "t-card-1", say: []cad.TranscriptLine{caller("Tiene sesenta años, por favor rápido")}},
			{at: 90, radio: "Medic on scene, CPR in progress, attaching the monitor", radioCap: "als"},
		}},
	{ID: "crash-acn", Name: "Interstate crash with vehicle crash data", Minutes: 3,
		About:   "A car's crash notification arrives before anyone calls: a rollover at speed. A passing driver calls a minute later.",
		Teaches: []string{"read the crash data and its injury risk", "send a crash run card with extrication", "an electric car: tell the fire crews"},
		expect:  expect{types: []string{"MVA", "MVAPIN"}, caps: map[string]int{"als": 1}},
		steps: []step{
			{at: 0, call: "t-acn-1", east: 1500, north: 1200, radius: 8, method: "GPS", state: "ringing", callback: "+15405550166",
				crash: &acn.Crash{Provider: "Vehicle telematics", DeltaVKmh: 61, Direction: "front", Rollover: true, Airbags: []string{"front", "curtain"},
					Occupants: 2, Belted: ptr(true), SpeedKmh: 104, Fuel: "electric", Vehicle: acn.Vehicle{Year: 2024, Make: "Tesla", Model: "Model 3", Color: "blue", Plate: "TRN4821"}}},
			{at: 50, call: "t-acn-2", east: 1520, north: 1180, radius: 30, method: "Cell", state: "answered", callback: "+15405550101",
				say: []cad.TranscriptLine{caller("A car just flipped over on the interstate, it's on its roof, I think there are two people in it, one isn't moving")}},
			{at: 100, radio: "On scene, one vehicle on its roof, two patients, one trapped, need extrication", radioCap: "engine"},
		}},
	{ID: "robbery", Name: "Armed robbery, suspect fleeing", Minutes: 3,
		About:   "A store clerk calls: a man with a gun just robbed her and drove off. The vehicle, plate and direction come out as she talks.",
		Teaches: []string{"catch the weapon, vehicle, plate and direction", "send police and broadcast the vehicle", "ask for a drone"},
		expect:  expect{types: []string{"ROB"}, caps: map[string]int{"police": 2}},
		steps: []step{
			{at: 0, call: "t-rob-1", east: -900, north: -600, radius: 12, method: "DBH", state: "ringing", callback: "+15405550111", name: "QUICK MART"},
			{at: 3, call: "t-rob-1", state: "answered", say: []cad.TranscriptLine{taker("911, what is the address of your emergency?"),
				caller("I just got robbed, a guy with a gun, he took the money from the register")}},
			{at: 15, call: "t-rob-1", say: []cad.TranscriptLine{taker("Which way did he go? Can you describe the vehicle?"),
				caller("He left in a silver Honda Civic, Virginia plate KTR 4471, heading north on Williamson Rd"),
				caller("He's wearing a black hoodie and jeans, white guy about 30")}},
			{at: 60, radio: "Responding, any further on the vehicle description?", radioCap: "police"},
		}},
	{ID: "flood", Name: "A flood of calls: the line is attacked", Minutes: 3,
		About:   "One number calls again and again with silence, and many short calls arrive. A real emergency and a noise complaint are hidden among them.",
		Teaches: []string{"recognise a telephony denial-of-service and tell the supervisor", "find the real emergency", "move the noise complaint to non-emergency"},
		expect:  expect{types: []string{"FALL", "SICK", "BRTH", "CHEST", "STROKE"}, caps: map[string]int{"transport": 1}},
		steps:   floodSteps()},
}

func floodSteps() []step {
	var out []step
	for i := 0; i < 16; i++ {
		id := fmt.Sprintf("t-flood-%d", i)
		cb := "+15405559000"
		if i%3 == 1 {
			cb = fmt.Sprintf("+1540555%04d", 9100+i)
		}
		out = append(out, step{at: i * 4, call: id, east: float64(i*37%400 - 200), north: float64(i*53%400 - 200), radius: 900, method: "Cell", state: "answered", callback: cb},
			step{at: i*4 + 3, call: id, end: true})
	}
	out = append(out,
		step{at: 21, call: "t-flood-real", east: 260, north: 640, radius: 7, method: "DBH", state: "answered", callback: "+15405550133", name: "WHITE, E",
			say: []cad.TranscriptLine{taker("911, what is the address of your emergency?"), caller("My mother fell and she's having trouble breathing, she's 84, please send someone")}},
		step{at: 33, call: "t-flood-noise", east: -150, north: 300, radius: 20, method: "Cell", state: "answered", callback: "+15405550155", line: "non-emergency",
			say: []cad.TranscriptLine{caller("Yeah I want to make a noise complaint, my neighbours have loud music again")}},
	)
	sort.SliceStable(out, func(i, j int) bool { return out[i].at < out[j].at })
	return out
}

// player plays one scenario into the training engine.
type player struct {
	sc      Scenario
	started time.Time
	cancel  context.CancelFunc
	mu      sync.Mutex
	played  int
	calls   []string
}

func (s *Server) stopPlayer() {
	s.mu.Lock()
	p := s.player
	s.player = nil
	s.mu.Unlock()
	if p != nil {
		p.cancel()
	}
}

// startScenario plays a scenario on this (training) server.
func (s *Server) startScenario(sc Scenario) {
	s.stopPlayer()
	ctx, cancel := context.WithCancel(context.Background())
	p := &player{sc: sc, started: time.Now(), cancel: cancel}
	s.mu.Lock()
	s.player = p
	s.mu.Unlock()
	var centre [2]float64
	s.Eng.Read(func(st *cad.State) { centre = st.Settings.MapCenter })
	actor := cad.Actor{User: "training"}
	go func() {
		start := time.Now()
		addr := map[string]string{}
		for _, st := range sc.steps {
			wait := time.Until(start.Add(time.Duration(st.at) * time.Second))
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			p.mu.Lock()
			p.played++
			if st.call != "" && !slices.Contains(p.calls, st.call) {
				p.calls = append(p.calls, st.call)
			}
			p.mu.Unlock()
			switch {
			case st.end:
				s.Eng.EndCall(actor, st.call)
			case st.radio != "":
				s.playRadio(actor, p, st)
			case st.call != "":
				d := cad.CallData{Source: "training", State: st.state, Callback: st.callback, CallerName: st.name, Line: st.line, Transcript: st.say, Crash: st.crash, Emergency: true}
				if st.state == "answered" {
					d.Answer = time.Now()
				}
				if st.radius > 0 {
					lat, lon := offset(centre, st.east, st.north)
					loc := &ng911.Location{Lat: &lat, Lon: &lon, Radius: st.radius, Method: st.method, Source: "training", At: time.Now()}
					if st.alt != 0 {
						loc.Altitude, loc.VRadius = ptr(st.alt), 3
					}
					key := st.call
					if addr[key] == "" {
						addr[key] = s.reverseLabel(lat, lon)
					}
					if a := addr[key]; a != "" {
						civic := &ng911.Civic{Location: a, Floor: st.floor}
						loc.Civic, loc.Address = civic, a+map[bool]string{true: ", Floor " + st.floor, false: ""}[st.floor != ""]
					}
					d.Location = loc
					if st.crash != nil {
						c := *st.crash
						c.Lat, c.Lon, c.Callback = &lat, &lon, st.callback
						d.Crash = &c
					}
				}
				s.Eng.UpsertCall(actor, st.call, d)
			}
		}
	}()
}

// playRadio has a unit sent to the scenario's incident say something; if none was sent, nothing is said.
func (s *Server) playRadio(actor cad.Actor, p *player, st step) {
	p.mu.Lock()
	calls := append([]string(nil), p.calls...)
	p.mu.Unlock()
	var unit string
	s.Eng.Read(func(state *cad.State) {
		for _, id := range calls {
			c, ok := state.Calls[id]
			if !ok || c.Incident == "" {
				continue
			}
			in := state.Incidents[c.Incident]
			if in == nil {
				continue
			}
			for _, a := range in.Units {
				if u := state.Units[a.Unit]; u != nil && slices.Contains(state.Capabilities(u), st.radioCap) && a.Cleared.IsZero() {
					unit = u.ID
					return
				}
			}
		}
	})
	if unit != "" {
		s.Eng.RadioText(cad.RadioActor, cad.RadioLine{Unit: unit, Text: st.radio, Channel: "training"})
	}
}

func offset(centre [2]float64, east, north float64) (float64, float64) {
	lat := centre[0] + north/111320
	lon := centre[1] + east/(111320*math.Cos(centre[0]*math.Pi/180))
	return math.Round(lat*1e6) / 1e6, math.Round(lon*1e6) / 1e6
}

// reverseLabel is the recorded map's address for a point, or "".
func (s *Server) reverseLabel(lat, lon float64) string {
	if s.Maps == nil || s.Maps.Store() == nil || !s.Maps.Store().Contains(lat, lon) {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	res, err := s.Maps.Store().Reverse(ctx, lat, lon)
	if err != nil || !res.Exact && strings.HasPrefix(strings.ToLower(res.Label), "near") {
		return "" // no address there: the call shows its position only, as a wireless call would
	}
	return res.Label
}

func (s *Server) trainingView() *TrainingView {
	if !s.isTraining {
		return nil
	}
	v := &TrainingView{}
	s.mu.Lock()
	p := s.player
	s.mu.Unlock()
	if p != nil {
		p.mu.Lock()
		v.Scenario, v.Name, v.Started, v.Steps, v.Played = p.sc.ID, p.sc.Name, p.started, len(p.sc.steps), p.played
		p.mu.Unlock()
	}
	return v
}

func (s *Server) trainingRoutes() {
	s.handle("GET /api/training/scenarios", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		writeJSON(w, 200, map[string]any{"items": Scenarios})
	})
	s.handle("POST /api/training/start", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			ID string `json:"id"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		for _, sc := range Scenarios {
			if sc.ID == b.ID {
				s.startScenario(sc)
				writeJSON(w, 200, s.trainingView())
				return
			}
		}
		fail(w, 404, "no scenario "+b.ID)
	})
	s.handle("GET /api/training/score", auth.Viewer, s.trainingScore)
	s.handle("POST /api/training/reset", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		// The training board is rebuilt by the live server's trainer; this server is the old one.
		if s.resetTraining == nil {
			fail(w, 500, "no trainer")
			return
		}
		if _, err := s.resetTraining(); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// TrainingScore is how a trainee did on the scenario that is playing.
type TrainingScore struct {
	Scenario string       `json:"scenario"`
	Name     string       `json:"name"`
	Score    int          `json:"score"`
	Checks   []cad.Check  `json:"checks"`
	Reviews  []cad.Review `json:"reviews"`
}

func (s *Server) trainingScore(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	s.mu.Lock()
	p := s.player
	s.mu.Unlock()
	if p == nil {
		fail(w, 409, "no scenario is playing: start one")
		return
	}
	p.mu.Lock()
	calls := append([]string(nil), p.calls...)
	p.mu.Unlock()
	out := TrainingScore{Scenario: p.sc.ID, Name: p.sc.Name, Checks: []cad.Check{}, Reviews: []cad.Review{}}
	s.Eng.Read(func(st *cad.State) {
		incs := map[string]bool{}
		attached, total := 0, 0
		replied := false
		for _, id := range calls {
			c, ok := st.Calls[id]
			if !ok {
				continue
			}
			if len(c.Transcript) > 0 || c.Crash != nil {
				total++
				if c.Incident != "" {
					attached++
					incs[c.Incident] = true
				}
			}
			for _, l := range c.Transcript {
				if l.Speaker == "taker" && l.By != "" {
					replied = true
				}
			}
		}
		add := func(c cad.Check) { out.Checks = append(out.Checks, c) }
		add(cad.Check{Name: "Every caller with something to say is on an incident", OK: total > 0 && attached == total, Detail: fmt.Sprintf("%d of %d", attached, total)})
		typeOK, sent := false, map[string]int{}
		var names []string
		for n := range incs {
			in := st.Incidents[n]
			if slices.Contains(p.sc.expect.types, in.Type) {
				typeOK = true
			}
			names = append(names, st.Incidents[n].Type)
			for _, as := range in.Units {
				if u := st.Units[as.Unit]; u != nil {
					for _, c := range st.Capabilities(u) {
						sent[c]++
					}
				}
			}
			out.Reviews = append(out.Reviews, st.Review(in))
		}
		add(cad.Check{Name: "Call type", OK: typeOK, Detail: "chose " + firstNonEmptyStr(strings.Join(names, ", "), "none") + "; expected " + strings.Join(p.sc.expect.types, " or ")})
		keys := make([]string, 0, len(p.sc.expect.caps))
		for k := range p.sc.expect.caps {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			n := p.sc.expect.caps[k]
			add(cad.Check{Name: fmt.Sprintf("At least %d × %s sent", n, k), OK: sent[k] >= n, Detail: fmt.Sprintf("%d sent", sent[k])})
		}
		if p.sc.expect.reply {
			add(cad.Check{Name: "Spoke to the caller in their language", OK: replied, Detail: map[bool]string{true: "a translated reply was sent", false: "no translated reply"}[replied]})
		}
		if p.sc.ID == "flood" {
			add(cad.Check{Name: "Denial-of-service warning acknowledged", OK: st.TDoS.AckedBy != "", Detail: firstNonEmptyStr(st.TDoS.AckedBy, "not acknowledged (a supervisor's task)")})
			noise := st.Calls["t-flood-noise"]
			ok := noise != nil && noise.Screen != nil && noise.Screen.NonEmergency && noise.Incident == ""
			add(cad.Check{Name: "The noise complaint was kept off the emergency board", OK: ok, Detail: map[bool]string{true: "kept as non-emergency", false: "made an emergency incident"}[ok]})
		}
	})
	good := 0
	for _, c := range out.Checks {
		if c.OK {
			good++
		}
	}
	if len(out.Checks) > 0 {
		out.Score = good * 100 / len(out.Checks)
	}
	writeJSON(w, 200, out)
}
