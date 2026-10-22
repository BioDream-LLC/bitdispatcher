// Package api is bitdispatcher's public API: REST plus a server-sent event stream. The web interface uses
// exactly this API (DESIGN.md rule 8), so anything a dispatcher can do on screen a program can do too.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/advisory"
	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/bitvoice"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/gis"
	"github.com/biodream-llc/bitdispatcher/internal/imagery"
	"github.com/biodream-llc/bitdispatcher/internal/ledger"
	"github.com/biodream-llc/bitdispatcher/internal/mapdata"
	"github.com/biodream-llc/bitdispatcher/internal/neris"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
	"github.com/biodream-llc/bitdispatcher/internal/views"
)

// Integrations is the integration configuration kept in the data directory (with its secrets, mode 0600).
type Integrations struct {
	BitVoice   bitvoice.Config   `json:"bitvoice"`
	More       []bitvoice.Config `json:"moreBitvoice"` // other BitVoice systems followed besides the main one
	ALIListen  string            `json:"aliListen"`    // host:port to accept ALI spills on; empty: off
	Extra      Extra             `json:"extra"`        // hooks, the drone system, the language model
	Views      views.Config      `json:"views"`        // pictures of a place from the side
	NERIS      neris.Config      `json:"neris"`        // fire and EMS incidents to the national reporting system
	Partners   []Partner         `json:"partners"`     // neighbouring agencies' CADs (CAD-to-CAD)
	PublicFeed PublicFeed        `json:"publicFeed"`   // what is happening, for the public and apps like PulsePoint
	LIS        LIS               `json:"lis"`          // a location server asked by HELD (supplemental location)
}

// Server is the API.
type Server struct {
	enrol          enrolCodes // one-time workstation enrolment codes
	neris          *neris.Client
	Eng            *cad.Engine
	Auth           *auth.Store
	BV             *bitvoice.Bridge
	moreBV         moreBridges // other BitVoice systems (Integrations.More)
	Log            *slog.Logger
	DataDir        string
	Version        string
	Web            http.Handler
	Manual         http.Handler      // the printed manual at /manual
	Tiles          *TileCache        // online map tiles, for outside the recorded map
	Maps           *mapdata.Service  // the recorded map
	Photos         *imagery.Service  // recorded aerial photos
	Advisory       *advisory.Service // the advisory panel
	GIS            *gis.Service      // the county's GIS layers
	Views          *views.Service    // oblique and street-level pictures of a place
	trees          treeCache
	sites          sites
	TLSFingerprint string // this site's certificate, for other sites to pin
	photoStats     statsCache

	mu     sync.Mutex // integ, ali, aliErr
	integ  Integrations
	ali    *ng911.ALIListener
	aliErr string
	aliMu  sync.Mutex // aliAt, aliN: taken by spill, which runs while ali.Close waits
	aliAt  time.Time
	aliN   int
	mux    *http.ServeMux

	// The training environment: a server of its own on its own ledger, for sessions signed in to train.
	trainer    *trainer
	isTraining bool
	quit       chan struct{} // closed when a training environment is reset, ending its streams
	player     *player
	// liveServer is the live server a training board belongs to (for publishing staged configuration), and
	// exercise the shared exercise it is, when it is one.
	liveServer *Server
	exercise   string
	// resetTraining rebuilds the training environment (set on a training server).
	resetTraining func() (*Server, error)
}

// SpillWait is how long an unmatched ALI spill waits for the same call to arrive from another source.
var SpillWait = 2 * time.Second

const sessionCookie = "bd_session"
const csrfHeader = "X-BitDispatcher"

// New builds the server and starts the configured integrations.
func New(s *Server) (*Server, error) {
	s.mux = http.NewServeMux()
	if s.Views == nil {
		s.Views = &views.Service{Agent: "bitdispatcher/" + s.Version + " (+https://github.com/biodream-llc/bitdispatcher)"}
	}
	if err := s.loadIntegrations(); err != nil {
		return nil, err
	}
	if err := s.LoadSites(); err != nil {
		return nil, err
	}
	s.routes()
	if s.Photos != nil && !s.isTraining {
		s.photoEvents()
	}
	if !s.isTraining {
		s.trainer = &trainer{live: s}
		if s.DataDir != "" {
			os.RemoveAll(filepath.Join(s.DataDir, "training")) // boards from before a restart are gone
		}
	}
	if s.Maps != nil {
		if !s.isTraining {
			s.mapEvents()
		}
		if s.Maps.Store() != nil {
			s.Eng.SetRouter(mapRouter{s})
			s.Eng.SetGeocoder(mapGeocoder{s})
		}
	}
	return s, nil
}

func (s *Server) integrationsPath() string { return filepath.Join(s.DataDir, "integrations.json") }

func (s *Server) loadIntegrations() error {
	b, err := os.ReadFile(s.integrationsPath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.integ); err != nil {
			return fmt.Errorf("integrations.json: %w", err)
		}
	}
	s.applyIntegrations()
	return nil
}

func (s *Server) applyIntegrations() {
	s.BV.Configure(s.integ.BitVoice)
	if !s.isTraining {
		s.applyMore(s.integ.More)
	}
	if !s.isTraining {
		s.Views.Configure(s.integ.Views) // a training board shares the live board's
	}
	if s.ali != nil {
		s.ali.Close()
		s.ali = nil
	}
	s.aliErr = ""
	if s.integ.ALIListen != "" {
		l, err := ng911.ListenALI(s.integ.ALIListen, s.spill)
		if err != nil {
			s.aliErr = err.Error()
			s.Log.Warn("ali: cannot listen", "addr", s.integ.ALIListen, "err", err)
		} else {
			s.ali = l
			s.Log.Info("ali: listening for spills", "addr", l.Addr())
		}
	}
}

// Close stops the integrations.
func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ali != nil {
		s.ali.Close()
	}
}

// spill takes one ALI spill. A spill for a callback number BitVoice is already handling belongs to that
// call; otherwise it is a call of its own.
func (s *Server) spill(a ng911.ALI, from string) {
	s.aliMu.Lock()
	s.aliAt, s.aliN = time.Now(), s.aliN+1
	s.aliMu.Unlock()
	match := func() (id string) {
		s.Eng.Read(func(st *cad.State) {
			var best *cad.Call
			for _, c := range st.Calls {
				if c.State != "ended" && a.Callback != "" && c.Callback == a.Callback && (best == nil || c.Start.After(best.Start)) {
					best = c
				}
			}
			if best != nil {
				id = best.ID
			}
		})
		return id
	}
	id := match()
	if id == "" && a.Callback != "" {
		// Call handling sends the spill and the EIDO at the same moment; give the EIDO a moment to arrive
		// so one call does not appear twice.
		time.Sleep(SpillWait)
		id = match()
	}
	if id == "" {
		id = fmt.Sprintf("ali-%s-%d", strings.TrimPrefix(a.Callback, "+"), time.Now().Unix())
	}
	d := cad.CallData{Source: "ali", Callback: a.Callback, CallerName: a.Name, Class: a.Class, Provider: a.Provider, Location: a.Location,
		Position: a.Position, ALI: a.Raw, State: "answered", Emergency: true}
	if a.NoLocation {
		d.Problems = []string{"ALI: no location received"}
	}
	if err := s.Eng.UpsertCall(cad.ALIActor, id, d); err != nil {
		s.Log.Warn("ali: could not record spill", "from", from, "err", err)
	}
}

// ServeHTTP serves the API and the web interface, with security headers on everything.
// HooksHandler serves only the endpoints a call-handling system pushes to (EIDO, radio events), each
// bearer-authenticated, for a plain-HTTP listener on this computer's loopback (-hooks-addr) when the
// sender cannot be made to trust bitdispatcher's certificate. Nothing else is reachable through it.
func (s *Server) HooksHandler() http.Handler {
	m := http.NewServeMux()
	m.Handle("POST /api/ng911/eido", s.eidoPush())
	m.Handle("POST /api/radio/events", s.radioPush())
	return m
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Strict-Transport-Security", "max-age=31536000")
	h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; "+
		"media-src 'self' blob:; worker-src 'self' blob:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	if t := s.trainingFor(r); t != nil {
		t.ServeHTTP(w, r)
		return
	}
	if s.isTraining && (strings.HasPrefix(r.URL.Path, "/api/admin/") && !staged(r.URL.Path) || strings.HasPrefix(r.URL.Path, "/api/sites")) {
		fail(w, http.StatusForbidden, "not in training: sign in without training to change this (users, connections and maps are the live board's)")
		return
	}
	// With staged changes on, the live board's configuration changes only by publishing from staging.
	if !s.isTraining && r.Method != http.MethodGet && staged(r.URL.Path) && r.URL.Path != "/api/admin/settings" {
		var on bool
		s.Eng.Read(func(st *cad.State) { on = st.Settings.Security.StagedChanges })
		if on {
			fail(w, http.StatusConflict, "configuration changes go through staging here: sign in with Training ticked as an administrator, make the change, try it, then publish it (Settings › Security)")
			return
		}
	}
	s.mux.ServeHTTP(w, r)
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	var p *cad.Problem
	if errors.As(err, &p) {
		writeJSON(w, http.StatusUnprocessableEntity, p)
		return
	}
	writeJSON(w, http.StatusInternalServerError, cad.Problem{Message: err.Error()})
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, cad.Problem{Message: msg})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil && !errors.Is(err, io.EOF) {
		fail(w, http.StatusBadRequest, "the request is not valid JSON: "+err.Error())
		return false
	}
	return true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type handler func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor)

// handle registers a route that needs a session with at least role; writes also need the CSRF header.
func (s *Server) handle(pattern, role string, h handler) {
	s.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			fail(w, http.StatusUnauthorized, "sign in")
			return
		}
		se, ok := s.Auth.Session(c.Value)
		if !ok {
			fail(w, http.StatusUnauthorized, "your session ended: sign in again")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get(csrfHeader) == "" {
			fail(w, http.StatusForbidden, "missing "+csrfHeader+" header")
			return
		}
		// A locked console shows and changes nothing until its person unlocks it.
		if se.Locked {
			writeJSON(w, http.StatusLocked, map[string]any{"message": "this console is locked: unlock it with your password", "locked": true})
			return
		}
		if !auth.AtLeast(se.Role, role) {
			fail(w, http.StatusForbidden, "your role ("+se.Role+") cannot do this; it needs "+role)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			s.Auth.Touch(c.Value) // a change is input at the console
		}
		h(w, r.WithContext(bitvoice.OnBehalf(r.Context(), personOf(se))), se, actorOf(se))
	})
}

// personOf names a signed-in person for another system's records: their name and username.
func personOf(se *auth.Session) string {
	if se.Name != "" && se.Name != se.Username {
		return se.Name + " (" + se.Username + ")"
	}
	return se.Username
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"name": "bitdispatcher", "version": s.Version})
	})
	m.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true, "time": time.Now().UTC()})
	})
	m.HandleFunc("POST /api/auth/login", s.signIn)
	s.entryRoutes()
	s.viewRoutes()
	s.messageRoutes()
	s.alertRoutes()
	s.nerisRoutes()
	s.stationRoutes()
	s.partnerRoutes()
	s.protocolRoutes()
	s.fileRoutes()
	s.publicFeedRoutes()
	s.draftRoutes()
	s.replayRoutes()
	s.lisRoutes()
	s.recordsRoutes()
	s.lockRoutes()
	s.workstationRoutes()
	m.HandleFunc("POST /api/auth/logout", s.logout)
	m.HandleFunc("POST /api/auth/demo", s.loginDemo)
	m.HandleFunc("GET /api/auth/me", s.me)
	// Integration endpoints BitVoice (or any EIDO / radio-data sender) posts to; bearer-authenticated.
	m.Handle("POST /api/ng911/eido", s.eidoPush())
	m.Handle("POST /api/radio/events", s.radioPush())

	s.handle("GET /api/state", auth.Viewer, s.state)
	s.handle("GET /api/stream", auth.Viewer, s.stream)
	s.handle("GET /api/incidents/{n}/recommend", auth.Viewer, s.recommend)
	s.handle("GET /api/coverage", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		writeJSON(w, 200, s.Eng.Coverage())
	})
	s.handle("GET /api/incidents/{n}/events", auth.Viewer, s.incidentEvents)
	s.handle("GET /api/incidents/history", auth.Viewer, s.history)
	s.handle("POST /api/incidents", auth.Dispatcher, s.createIncident)
	s.handle("PATCH /api/incidents/{n}", auth.Dispatcher, s.changeIncident)
	s.handle("POST /api/incidents/{n}/notes", auth.Dispatcher, s.addNote)
	s.handle("POST /api/incidents/{n}/dispatch", auth.Dispatcher, s.dispatch)
	s.handle("POST /api/incidents/{n}/close", auth.Dispatcher, s.closeIncident)
	s.handle("POST /api/incidents/{n}/reopen", auth.Supervisor, s.reopen)
	s.handle("POST /api/incidents/{n}/correct", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var c cad.Correction
		if !readJSON(w, r, &c) {
			return
		}
		ch, err := s.Eng.CorrectIncident(a, r.PathValue("n"), c)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"changes": ch})
	})
	s.handle("POST /api/units/{id}/status", auth.Dispatcher, s.unitStatus)
	s.handle("POST /api/units/{id}/position", auth.Dispatcher, s.unitPosition)
	s.handle("POST /api/command", auth.Dispatcher, s.command)
	s.handle("POST /api/calls/{id}/attach", auth.Dispatcher, s.attachCall)
	s.handle("POST /api/calls/{id}/rebid", auth.Dispatcher, s.rebid)
	s.handle("POST /api/calls/{id}/text", auth.Dispatcher, s.textCaller)
	s.handle("GET /api/bitvoice/records/{kind}", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out, err := s.BV.Records(r.Context(), r.PathValue("kind"), r.URL.Query())
		if errors.Is(err, bitvoice.ErrNoRecord) {
			fail(w, 404, err.Error())
			return
		}
		if err != nil {
			fail(w, http.StatusBadGateway, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(out)
	})
	s.handle("POST /api/calls/{id}/take", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		id := r.PathValue("id")
		s.callAction(w, a, id, "text taken", func() (string, error) { return "taken by " + a.User, s.bvFor(id).TakeText(r.Context(), id) })
	})
	s.handle("POST /api/calls/{id}/video-link", auth.Dispatcher, s.videoLink)
	s.handle("POST /api/calls/{id}/eido", auth.Dispatcher, s.refreshEIDO)
	s.handle("POST /api/calls/{id}/whep", auth.Dispatcher, s.whep)
	s.handle("GET /api/calls/{id}/media/{n}", auth.Dispatcher, s.media)
	s.handle("GET /api/calls/{id}/evidence/{n}", auth.Dispatcher, s.evidence)
	s.handle("POST /api/calls/{id}/evidence/hold", auth.Dispatcher, s.holdEvidence)
	s.handle("POST /api/calls/{id}/video/reveal", auth.Dispatcher, s.revealVideo)
	s.handle("POST /api/emergencies/{key}/ack", auth.Dispatcher, s.ackEmergency)

	s.handle("PUT /api/admin/settings", auth.Admin, s.saveSettings)
	s.handle("PUT /api/admin/stations/{id}", auth.Admin, s.saveStation)
	s.handle("DELETE /api/admin/stations/{id}", auth.Admin, s.removeStation)
	s.handle("PUT /api/admin/units/{id}", auth.Admin, s.saveUnit)
	s.handle("DELETE /api/admin/units/{id}", auth.Admin, s.removeUnit)
	s.handle("PUT /api/admin/calltypes/{code}", auth.Admin, s.saveCallType)
	s.handle("DELETE /api/admin/calltypes/{code}", auth.Admin, s.removeCallType)
	s.handle("GET /api/admin/users", auth.Admin, s.listUsers)
	s.handle("PUT /api/admin/users/{name}", auth.Admin, s.saveUser)
	s.handle("DELETE /api/admin/users/{name}", auth.Admin, s.removeUser)
	s.handle("GET /api/admin/integrations", auth.Admin, s.getIntegrations)
	s.handle("PUT /api/admin/integrations", auth.Admin, s.saveIntegrations)
	s.handle("POST /api/admin/integrations/bitvoice/test", auth.Admin, s.testBitVoice)
	s.handle("POST /api/admin/integrations/bitvoice/fingerprint", auth.Admin, s.fingerprint)
	s.handle("POST /api/admin/integrations/secret", auth.Admin, s.newSecret)
	s.handle("POST /api/admin/integrations/ali/test", auth.Admin, s.testALI)
	s.moreRoutes()
	s.handle("GET /api/ledger", auth.Supervisor, s.ledgerEvents)
	s.handle("GET /api/ledger/verify", auth.Supervisor, s.verify)
	s.handle("GET /api/reports/operations", auth.Supervisor, s.report)
	s.handle("POST /api/reports/certify", auth.Supervisor, s.certify)
	s.handle("GET /api/positions", auth.Supervisor, s.positions)

	if s.Tiles != nil {
		s.handle("GET /api/tiles/{z}/{x}/{y}", auth.Viewer, s.tile)
	}
	if s.Maps != nil {
		s.handle("GET /api/map", auth.Viewer, s.mapInfo)
		s.handle("GET /api/map/tiles/{z}/{x}/{y}", auth.Viewer, s.vectorTile)
		s.handle("GET /api/map/fonts/{stack}/{range}", auth.Viewer, s.glyphs)
		s.handle("GET /api/map/search", auth.Viewer, s.mapSearch)
		s.handle("GET /api/map/reverse", auth.Viewer, s.mapReverse)
		s.handle("GET /api/map/zones", auth.Viewer, s.mapZones)
		s.handle("POST /api/map/routes", auth.Viewer, s.mapRoutes)
		s.handle("POST /api/admin/map/plan", auth.Admin, s.mapPlan)
		s.handle("POST /api/admin/map/record", auth.Admin, s.mapRecord)
		s.handle("POST /api/admin/map/cancel", auth.Admin, s.mapCancel)
		s.handle("POST /api/admin/map/update", auth.Admin, s.mapUpdate)
	}
	if s.Photos != nil {
		s.photoRoutes()
	}
	s.mdtRoutes()
	s.dreamRoutes()
	if s.isTraining {
		s.trainingRoutes()
		s.trainingMoreRoutes()
	} else {
		s.exerciseRoutes()
	}
	s.siteRoutes()
	s.rmsRoutes()
	s.migrateRoutes()
	if s.GIS != nil {
		s.gisRoutes()
	}
	if s.Advisory != nil {
		s.advisoryRoutes()
	}
	if s.Manual != nil {
		// The manual is documentation, not data: it is readable before signing in.
		m.Handle("GET /manual", s.Manual)
		m.Handle("GET /manual/", s.Manual)
	}
	if s.Web != nil {
		m.Handle("/", s.Web)
	}
}

// ---- sign-in ----

// signedIn records a sign-in and gives the browser its session.
func (s *Server) signedIn(w http.ResponseWriter, se *auth.Session, extra map[string]string) {
	data := map[string]string{"from": se.From}
	for k, v := range extra {
		data[k] = v
	}
	s.Eng.Record(actorOf(se), cad.EvSignIn, data)
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: se.ID, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, 200, se)
}

// loginDemo signs in as the public demonstration account, when an administrator has named one: the sign-in
// page's "Sign in as the demo user" button. Without a position, the visitor gets one of their own.
func (s *Server) loginDemo(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(csrfHeader) == "" {
		fail(w, http.StatusForbidden, "missing "+csrfHeader+" header")
		return
	}
	var b struct {
		Console  string `json:"console"`
		Training bool   `json:"training"`
		Duty     string `json:"duty"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	console := strings.TrimSpace(b.Console)
	if console == "" || len(console) > 24 {
		console = "WEB-" + strings.ToUpper(auth.RandomToken(3))[:4]
	}
	se, err := s.Auth.LoginDemo(console, clientIP(r), b.Duty)
	if err != nil {
		fail(w, http.StatusNotFound, err.Error())
		return
	}
	if b.Training {
		s.Auth.SetTraining(se.ID, "")
		se.Training = true
	}
	s.signedIn(w, se, map[string]string{"demo": "true"})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if se, ok := s.Auth.Logout(c.Value); ok {
			s.Eng.Record(actorOf(se), cad.EvSignOut, nil)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

// me answers 200 either way, so a signed-out browser has no error to log.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if se, ok := s.Auth.Session(c.Value); ok {
			writeJSON(w, 200, map[string]any{"signedIn": true, "session": se})
			return
		}
	}
	out := map[string]any{"signedIn": false}
	// The sign-in page offers the public demonstration account, when there is one.
	if d, ok := s.Auth.Demo(); ok {
		out["demo"] = map[string]any{"name": d.Name, "role": d.Role, "duties": d.Duties}
	}
	// The agency's system use notice, shown before anyone signs in.
	s.Eng.Read(func(st *cad.State) {
		if b := strings.TrimSpace(st.Settings.SignInNotice); b != "" {
			out["notice"] = b
		}
	})
	writeJSON(w, 200, out)
}

// ---- the state and its stream ----

// UnitView is a unit with its full capability list.
type UnitView struct {
	*cad.Unit
	Caps []string `json:"caps"`
}

// View is everything the dispatcher screen shows, in one document.
type View struct {
	ServerTime    time.Time           `json:"serverTime"`
	Seq           uint64              `json:"seq"`
	Settings      cad.Settings        `json:"settings"`
	Stations      []*cad.Station      `json:"stations"`
	Units         []UnitView          `json:"units"`
	CallTypes     []*cad.CallType     `json:"callTypes"`
	Incidents     []*cad.Incident     `json:"incidents"`
	Calls         []*cad.Call         `json:"calls"`
	Emergencies   []*cad.Emergency    `json:"emergencies"`
	Health        HealthView          `json:"health"`
	PlaceViews    bool                `json:"placeViews,omitempty"` // a provider of pictures from the side is on
	Messages      []*cad.Message      `json:"messages"`             // the last day's messages to and from units
	StationAlerts []*cad.StationAlert `json:"stationAlerts"`        // the last day's tone-outs
	Verbs         []cad.Verb          `json:"verbs"`
	Statuses      []string            `json:"statuses"`
	// What is known about each incident's place: premises (hazards first) and the earlier system's
	// incidents there.
	Premises map[string][]*cad.Premise `json:"premises"`
	Prior    map[string][]cad.History  `json:"prior"`
	// The wishlist: each open incident in a paragraph, drones, radio words, critical calls and their care,
	// breaks, the denial-of-service warning, and whether this is the training environment.
	Summaries map[string]string     `json:"summaries"`
	Drones    []*cad.Drone          `json:"drones"`
	Radio     []cad.RadioLine       `json:"radio"`
	Care      []*cad.CareCase       `json:"care"`
	Breaks    map[string]*cad.Break `json:"breaks"`
	TDoS      cad.TDoS              `json:"tdos"`
	Training  *TrainingView         `json:"training,omitempty"`
	Entry     EntryView             `json:"entry"`
}

// HealthView is every interface's health.
type HealthView struct {
	BitVoice bitvoice.Health  `json:"bitvoice"`
	More     []MoreHealth     `json:"moreBitvoice"` // the other BitVoice systems' links
	ALI      ALIHealth        `json:"ali"`
	Ledger   uint64           `json:"ledger"`
	Tiles    bool             `json:"tiles"`
	Map      *MapHealth       `json:"map,omitempty"`
	Photos   *PhotoHealth     `json:"photos,omitempty"`
	Site     string           `json:"site,omitempty"`
	Sites    []PeerHealth     `json:"sites,omitempty"`
	Advisory *advisory.Status `json:"advisory,omitempty"`
	Drone    bool             `json:"drone"`         // a drone system is configured
	Hooks    bool             `json:"hooks"`         // the inbound hooks are on
	Model    bool             `json:"model"`         // a language model translates
	NERIS    bool             `json:"neris"`         // fire and EMS incidents go to NERIS
	LIS      string           `json:"lis,omitempty"` // the location server's name, when one is on
}

// ALIHealth is the ALI listener's health.
type ALIHealth struct {
	Listening string    `json:"listening,omitempty"`
	Error     string    `json:"error,omitempty"`
	Last      time.Time `json:"last,omitzero"`
	Count     int       `json:"count"`
}

func (s *Server) health() HealthView {
	s.mu.Lock()
	ali := ALIHealth{Error: s.aliErr}
	if s.ali != nil {
		ali.Listening = s.ali.Addr()
	}
	s.mu.Unlock()
	s.aliMu.Lock()
	ali.Last, ali.Count = s.aliAt, s.aliN
	s.aliMu.Unlock()
	tiles := s.Tiles != nil && s.Tiles.Configured()
	var seq uint64
	s.Eng.Read(func(st *cad.State) { seq = st.Seq })
	x := s.extra()
	return HealthView{BitVoice: s.BV.Health(), More: s.moreHealth(), ALI: ali, Ledger: seq, Tiles: tiles, Map: s.mapHealth(), Photos: s.photoHealth(), Advisory: s.advisoryHealth(), Site: s.Eng.Site(), Sites: s.siteHealth(),
		Drone: x.DroneURL != "", Hooks: x.HookToken != "", Model: x.ModelOn && x.ModelURL != "", NERIS: s.nerisConfig().On, LIS: lisName(s.lisConfig())}
}

func (s *Server) view() ([]byte, error) {
	vc := s.Views.Config()
	now := s.Eng.Now()
	h := s.health()
	var out []byte
	var err error
	s.Eng.Read(func(st *cad.State) {
		v := View{ServerTime: now.UTC(), Seq: st.Seq, Settings: st.Settings, Health: h, PlaceViews: vc.Any() || vc.GoogleLink, Verbs: cad.Verbs, Statuses: cad.Statuses,
			Stations: []*cad.Station{}, Units: []UnitView{}, CallTypes: []*cad.CallType{}, Incidents: []*cad.Incident{}, Calls: []*cad.Call{}, Emergencies: []*cad.Emergency{}}
		v.Messages = floorMessages(st, now)
		v.StationAlerts = stationAlertsView(st, now)
		for _, x := range st.Stations {
			v.Stations = append(v.Stations, x)
		}
		sort.Slice(v.Stations, func(i, j int) bool { return v.Stations[i].ID < v.Stations[j].ID })
		for _, u := range st.Units {
			v.Units = append(v.Units, UnitView{Unit: u, Caps: st.Capabilities(u)})
		}
		sort.Slice(v.Units, func(i, j int) bool { return unitLess(v.Units[i].ID, v.Units[j].ID) })
		for _, c := range st.CallTypes {
			v.CallTypes = append(v.CallTypes, c)
		}
		sort.Slice(v.CallTypes, func(i, j int) bool { return v.CallTypes[i].Name < v.CallTypes[j].Name })
		for _, in := range st.Incidents {
			if in.Status != cad.Closed || now.Sub(in.Closed) < 12*time.Hour {
				v.Incidents = append(v.Incidents, in)
			}
		}
		sort.Slice(v.Incidents, func(i, j int) bool { return v.Incidents[i].Number > v.Incidents[j].Number })
		v.Premises, v.Prior = map[string][]*cad.Premise{}, map[string][]cad.History{}
		for _, in := range v.Incidents {
			if in.Location == nil || in.Location.Lat == nil || in.Location.Lon == nil {
				continue
			}
			if ps := st.PremisesNear(*in.Location.Lat, *in.Location.Lon); len(ps) > 0 {
				v.Premises[in.Number] = ps
			}
			if hs := st.HistoryNear(*in.Location.Lat, *in.Location.Lon, cad.PremiseRadius, now.AddDate(-10, 0, 0)); len(hs) > 0 {
				v.Prior[in.Number] = hs[:min(len(hs), 5)]
			}
		}
		for _, c := range st.Calls {
			if c.State != "ended" || now.Sub(c.End) < time.Hour {
				v.Calls = append(v.Calls, c)
			}
		}
		sort.Slice(v.Calls, func(i, j int) bool { return v.Calls[i].Start.After(v.Calls[j].Start) })
		v.Training = s.trainingView()
		v.Entry = s.entryView(st, now)
		v.Summaries, v.Drones, v.Care, v.Breaks, v.TDoS = map[string]string{}, []*cad.Drone{}, []*cad.CareCase{}, st.Breaks, st.TDoS
		for _, in := range v.Incidents {
			if in.Status != cad.Closed {
				v.Summaries[in.Number] = st.Summary(in)
			}
		}
		for _, d := range st.Drones {
			if now.Sub(d.Updated) < 12*time.Hour {
				v.Drones = append(v.Drones, d)
			}
		}
		sort.Slice(v.Drones, func(i, j int) bool { return v.Drones[i].ID < v.Drones[j].ID })
		v.Radio = st.Radio[max(0, len(st.Radio)-60):]
		for _, cc := range st.Care {
			if cc.Status != "closed" || now.Sub(cc.At) < 24*time.Hour {
				v.Care = append(v.Care, cc)
			}
		}
		sort.Slice(v.Care, func(i, j int) bool { return v.Care[i].At.After(v.Care[j].At) })
		for _, e := range st.Emergencies {
			if e.AckedAt.IsZero() || now.Sub(e.AckedAt) < time.Hour {
				v.Emergencies = append(v.Emergencies, e)
			}
		}
		sort.Slice(v.Emergencies, func(i, j int) bool { return v.Emergencies[i].Raised.After(v.Emergencies[j].Raised) })
		out, err = json.Marshal(v)
	})
	return out, err
}

// unitLess orders call signs naturally: E2 before E10.
func unitLess(a, b string) bool {
	pa, na := splitNum(a)
	pb, nb := splitNum(b)
	if pa != pb {
		return pa < pb
	}
	return na < nb
}

func splitNum(s string) (string, int) {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	n, _ := strconv.Atoi(s[i:])
	return s[:i], n
}

func (s *Server) state(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	b, err := s.view()
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(b)
}

// stream sends a notice for every change and the interfaces' health every five seconds. The interface
// reloads the state when told; nothing polls.
func (s *Server) stream(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	fl, ok := w.(http.Flusher)
	if !ok {
		fail(w, 500, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	ch := s.Eng.Subscribe()
	defer s.Eng.Unsubscribe(ch)
	fmt.Fprint(w, "event: hello\ndata: {}\n\n")
	fl.Flush()
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.quit:
			return
		case n := <-ch:
			b, _ := json.Marshal(n)
			fmt.Fprintf(w, "event: change\ndata: %s\n\n", b)
			// Coalesce a burst (a dispatch writes several events) into the frames already queued.
			for drain := true; drain; {
				select {
				case n := <-ch:
					b, _ := json.Marshal(n)
					fmt.Fprintf(w, "event: change\ndata: %s\n\n", b)
				default:
					drain = false
				}
			}
		case <-tick.C:
			if _, ok := s.Auth.Session(sessionOf(r)); !ok {
				fmt.Fprint(w, "event: signedout\ndata: {}\n\n")
				fl.Flush()
				return
			}
			b, _ := json.Marshal(s.health())
			fmt.Fprintf(w, "event: health\ndata: %s\n\n", b)
		}
		fl.Flush()
	}
}

func sessionOf(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value
	}
	return ""
}

// ---- incidents ----

func (s *Server) recommend(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	rec, err := s.Eng.Recommend(r.PathValue("n"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if s.Advisory != nil && s.Advisory.HasScripts("recommend") {
		s.scriptRecommendation(r.Context(), &rec)
	}
	writeJSON(w, 200, rec)
}

// scriptRecommendation lets the agency's recommendation scripts prefer units and add notes. A preferred
// unit among a line's next best takes the place of the last one not preferred; a failing script leaves
// the recommendation as it was, and says so.
func (s *Server) scriptRecommendation(ctx context.Context, rec *cad.Recommendation) {
	in := advisory.RecommendInput{Units: rec.Units}
	found := false
	s.Eng.Read(func(st *cad.State) {
		if x, ok := st.Incidents[rec.Incident]; ok {
			in.Incident, found = advisory.IncidentInput(st, x, s.Eng.Now()), true
		}
	})
	if !found {
		return
	}
	for _, l := range rec.Lines {
		rl := advisory.RecommendLine{Capability: l.Capability, Count: l.Count, Candidates: []map[string]any{}}
		for _, c := range append(append([]cad.Candidate{}, l.Units...), l.Next...) {
			rl.Candidates = append(rl.Candidates, map[string]any{"unit": c.Unit, "type": c.Type, "seconds": c.Seconds, "metres": c.Metres})
		}
		in.Lines = append(in.Lines, rl)
	}
	adv := s.Advisory.AdviseRecommendation(ctx, in)
	rec.Notes = append(rec.Notes, adv.Notes...)
	for _, f := range adv.Faults {
		rec.Notes = append(rec.Notes, "Script "+f.Name+" failed ("+f.Message+"): recommended without it")
	}
	if len(adv.Prefer) == 0 {
		return
	}
	pref := map[string]bool{}
	for _, u := range adv.Prefer {
		pref[u] = true
	}
	for li := range rec.Lines {
		l := &rec.Lines[li]
		for ni := 0; ni < len(l.Next); ni++ {
			if !pref[l.Next[ni].Unit] || slices.Contains(rec.Units, l.Next[ni].Unit) {
				continue
			}
			for ui := len(l.Units) - 1; ui >= 0; ui-- {
				if !pref[l.Units[ui].Unit] {
					out := l.Units[ui]
					l.Units[ui], l.Next[ni] = l.Next[ni], out
					l.Units[ui].Reasons = append(l.Units[ui].Reasons, "preferred by the agency's script")
					break
				}
			}
		}
	}
	rec.Units = rec.Units[:0]
	for _, l := range rec.Lines {
		for _, c := range l.Units {
			if !slices.Contains(rec.Units, c.Unit) {
				rec.Units = append(rec.Units, c.Unit)
			}
		}
	}
}

func (s *Server) incidentEvents(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	n := r.PathValue("n")
	evs, err := s.Eng.Events(0, 0, func(e ledger.Event) bool { return e.Incident == n })
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": evs})
}

// history searches closed incidents older than the live view.
func (s *Server) history(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	var out []*cad.Incident
	s.Eng.Read(func(st *cad.State) {
		for _, in := range st.Incidents {
			hay := strings.ToLower(in.Number + " " + in.Type + " " + st.TypeName(in.Type) + " " + in.Disposition)
			if in.Location != nil {
				hay += " " + strings.ToLower(in.Location.Address)
			}
			if in.Caller != nil {
				hay += " " + in.Caller.Callback + " " + strings.ToLower(in.Caller.Name)
			}
			for _, u := range in.Units {
				hay += " " + strings.ToLower(u.Unit)
			}
			if q == "" || strings.Contains(hay, q) {
				cp := *in
				cp.Timeline = nil
				out = append(out, &cp)
			}
		}
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Number > out[j].Number })
	if len(out) > 500 {
		out = out[:500]
	}
	if out == nil {
		out = []*cad.Incident{}
	}
	writeJSON(w, 200, map[string]any{"items": out})
}

func (s *Server) createIncident(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var n cad.NewIncident
	if !readJSON(w, r, &n) {
		return
	}
	num, err := s.Eng.CreateIncident(a, n)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 201, map[string]string{"number": num})
}

func (s *Server) changeIncident(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var c cad.IncidentChange
	if !readJSON(w, r, &c) {
		return
	}
	if err := s.Eng.ChangeIncident(a, r.PathValue("n"), c); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) addNote(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		Text     string `json:"text"`
		Priority bool   `json:"priority"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	if err := s.Eng.AddNote(a, r.PathValue("n"), b.Text, b.Priority); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) dispatch(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		Units       []string `json:"units"`
		Recommended []string `json:"recommended"`
		Reason      string   `json:"reason"`  // why other units than recommended
		Confirm     bool     `json:"confirm"` // the dispatcher confirmed the recommendation's warnings
	}
	if !readJSON(w, r, &b) {
		return
	}
	n := r.PathValue("n")
	rec, _ := s.Eng.Recommend(n)
	if len(rec.Warnings) > 0 && !b.Confirm {
		writeJSON(w, 422, map[string]any{"message": "Confirm first: " + strings.Join(rec.Warnings, "; "), "field": "confirm", "warnings": rec.Warnings})
		return
	}
	if len(b.Recommended) == 0 {
		b.Recommended = rec.Units
	}
	if err := s.Eng.NeedReason(n, b.Units, b.Recommended, b.Reason); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.Eng.DispatchWhy(a, n, b.Units, b.Recommended, b.Reason, rec.Plan); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) closeIncident(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		Disposition string `json:"disposition"`
		Text        string `json:"text"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	if err := s.Eng.CloseIncident(a, r.PathValue("n"), b.Disposition, b.Text); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) reopen(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		Reason string `json:"reason"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	if err := s.Eng.ReopenIncident(a, r.PathValue("n"), b.Reason); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) unitStatus(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		Status   string `json:"status"`
		Incident string `json:"incident"`
		Reason   string `json:"reason"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	if err := s.Eng.SetStatus(a, r.PathValue("id"), b.Status, b.Incident, b.Reason); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// unitPosition takes a position from a mobile terminal or an AVL gateway.
func (s *Server) unitPosition(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var p cad.Position
	if !readJSON(w, r, &p) {
		return
	}
	if p.Lat < -90 || p.Lat > 90 || p.Lon < -180 || p.Lon > 180 {
		fail(w, 422, "latitude or longitude out of range")
		return
	}
	if p.Source == "" {
		p.Source = "manual"
	}
	if err := s.Eng.UnitPosition(a, strings.ToUpper(r.PathValue("id")), p); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) command(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		Line     string `json:"line"`
		Incident string `json:"incident"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	writeJSON(w, 200, s.Eng.Execute(a, b.Line, b.Incident))
}

// ---- calls and call handling ----

func (s *Server) attachCall(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		Incident string `json:"incident"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	if err := s.Eng.AttachCall(a, r.PathValue("id"), b.Incident); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) callAction(w http.ResponseWriter, a cad.Actor, call, action string, run func() (string, error)) {
	detail, err := run()
	if err != nil {
		s.Eng.RecordCallAction(a, call, action, false, err.Error())
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	s.Eng.RecordCallAction(a, call, action, true, detail)
	writeJSON(w, 200, map[string]string{"detail": detail})
}

func (s *Server) rebid(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	id := r.PathValue("id")
	s.callAction(w, a, id, "rebid", func() (string, error) {
		loc, err := s.bvFor(id).Rebid(r.Context(), id)
		if err != nil {
			return "", err
		}
		d := "location updated"
		if loc.Radius > 0 {
			d += fmt.Sprintf(", ±%.0f m", loc.Radius)
		}
		return d, nil
	})
}

func (s *Server) textCaller(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		Via  string `json:"via"`
		Text string `json:"text"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	b.Text = strings.TrimSpace(b.Text)
	if b.Text == "" || len(b.Text) > 1000 {
		fail(w, 422, "write 1 to 1000 characters")
		return
	}
	id := r.PathValue("id")
	s.callAction(w, a, id, "text", func() (string, error) {
		return "sent: " + b.Text, s.bvFor(id).Text(r.Context(), id, b.Via, b.Text)
	})
}

func (s *Server) videoLink(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	id := r.PathValue("id")
	var v bitvoice.VideoLink
	detail, err := func() (string, error) {
		var err error
		v, err = s.bvFor(id).RequestVideo(r.Context(), id)
		if err != nil {
			return "", err
		}
		if v.Sent {
			return "link texted to the caller", nil
		}
		if v.Problem == "" {
			return "BitVoice has no texting number for video links (its 911 settings): read the link to the caller", nil
		}
		if strings.Contains(strings.ToLower(v.Problem), "read the link") {
			return "link not texted: " + v.Problem, nil
		}
		return "link not texted (" + v.Problem + "): read it to the caller", nil
	}()
	if err != nil {
		s.Eng.RecordCallAction(a, id, "video link", false, err.Error())
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	s.Eng.RecordCallAction(a, id, "video link", true, detail)
	writeJSON(w, 200, map[string]any{"detail": detail, "url": v.URL, "sent": v.Sent})
}

func (s *Server) refreshEIDO(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	id := r.PathValue("id")
	s.callAction(w, a, id, "EIDO refresh", func() (string, error) { return "", s.bvFor(id).RefreshEIDO(r.Context(), id) })
}

// whep forwards the browser's WebRTC offer for a call's live video to BitVoice.
func (s *Server) whep(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	id := r.PathValue("id")
	view := ""
	s.Eng.Read(func(st *cad.State) {
		if c, ok := st.Calls[id]; ok && c.Video != nil {
			view = c.Video.ViewURL
		}
	})
	if view == "" {
		fail(w, 404, "this call has no video")
		return
	}
	offer, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256<<10))
	if err != nil {
		fail(w, 400, "offer too large")
		return
	}
	answer, _, err := s.bvFor(id).WHEP(r.Context(), view, offer)
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	s.Eng.RecordCallAction(a, id, "video viewed", true, "")
	w.Header().Set("Content-Type", "application/sdp")
	w.WriteHeader(201)
	w.Write(answer)
}

// media serves a caller photo or clip named in the EIDO, fetched from BitVoice.
func (s *Server) media(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	id := r.PathValue("id")
	n, err := strconv.Atoi(r.PathValue("n"))
	src := ""
	s.Eng.Read(func(st *cad.State) {
		if c, ok := st.Calls[id]; ok && err == nil && n >= 0 && n < len(c.Media) {
			src = c.Media[n].URL
		}
	})
	if src == "" {
		fail(w, 404, "no such media")
		return
	}
	resp, err := s.bvFor(id).Fetch(r.Context(), src)
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fail(w, http.StatusBadGateway, "BitVoice said "+resp.Status)
		return
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") && !strings.HasPrefix(ct, "video/") {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Disposition", "inline")
	io.Copy(w, io.LimitReader(resp.Body, 200<<20))
}

// evidence serves one of a caller's photos or clips from BitVoice, once a person chose to reveal it; each
// look is in the ledger with who looked (and in BitVoice's exposure record).
func (s *Server) evidence(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	id := r.PathValue("id")
	n, err := strconv.Atoi(r.PathValue("n"))
	var item *cad.EvidenceItem
	s.Eng.Read(func(st *cad.State) {
		if c, ok := st.Calls[id]; ok && err == nil && c.Handling != nil && c.Handling.Evidence != nil {
			for _, x := range c.Handling.Evidence.Items {
				if x.N == n {
					v := x
					item = &v
				}
			}
		}
	})
	if item == nil {
		fail(w, 404, "no such photo or clip")
		return
	}
	resp, err := s.bvFor(id).Evidence(r.Context(), id, n)
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fail(w, http.StatusBadGateway, "BitVoice said "+resp.Status)
		return
	}
	s.Eng.RecordCallAction(a, id, "evidence revealed", true, fmt.Sprintf("%s (SHA-256 %s)", item.Name, item.SHA256))
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") && !strings.HasPrefix(ct, "video/") {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("Content-Disposition", "inline")
	io.Copy(w, io.LimitReader(resp.Body, 200<<20))
}

func (s *Server) holdEvidence(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		Hold bool `json:"hold"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	id := r.PathValue("id")
	err := s.bvFor(id).HoldEvidence(r.Context(), id, b.Hold)
	s.Eng.RecordCallAction(a, id, map[bool]string{true: "evidence held", false: "evidence released"}[b.Hold], err == nil, errString(err))
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) revealVideo(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	id := r.PathValue("id")
	err := s.bvFor(id).RevealVideo(r.Context(), id)
	s.Eng.RecordCallAction(a, id, "live video revealed", true, errString(err))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) ackEmergency(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	key := r.PathValue("key")
	channel := ""
	s.Eng.Read(func(st *cad.State) {
		if em, ok := st.Emergencies[key]; ok {
			channel = em.Channel
		}
	})
	if err := s.Eng.AckEmergency(a, key, "bitdispatcher"); err != nil {
		writeErr(w, err)
		return
	}
	// Acknowledge on BitVoice's radio console too, so the latched alert clears there.
	out := map[string]string{"bitvoice": "not configured"}
	if channel != "" && s.BV.Health().Configured {
		if err := s.BV.AckRadio(r.Context(), channel); err != nil {
			out["bitvoice"] = "not acknowledged on the radio console: " + err.Error()
		} else {
			out["bitvoice"] = "acknowledged on the radio console"
		}
	}
	writeJSON(w, 200, out)
}

// ---- administration ----

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var v cad.Settings
	if !readJSON(w, r, &v) {
		return
	}
	if err := s.Eng.SaveSettings(a, v); err != nil {
		writeErr(w, err)
		return
	}
	if s.Tiles != nil {
		s.Tiles.SetTemplate(v.MapTiles)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) saveStation(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var v cad.Station
	if !readJSON(w, r, &v) {
		return
	}
	v.ID = r.PathValue("id")
	if err := s.Eng.SaveStation(a, v); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeStation(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	if err := s.Eng.RemoveStation(a, r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) saveUnit(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var v cad.UnitDef
	if !readJSON(w, r, &v) {
		return
	}
	v.ID = r.PathValue("id")
	if err := s.Eng.SaveUnit(a, v); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeUnit(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	if err := s.Eng.RemoveUnit(a, r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) saveCallType(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var v cad.CallType
	if !readJSON(w, r, &v) {
		return
	}
	v.Code = r.PathValue("code")
	if err := s.Eng.SaveCallType(a, v); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeCallType(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	if err := s.Eng.RemoveCallType(a, r.PathValue("code")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	writeJSON(w, 200, map[string]any{"items": s.Auth.List(), "sessions": s.Auth.Active()})
}

func (s *Server) saveUser(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		Name     string `json:"name"`
		Role     string `json:"role"`
		Password string `json:"password"`
		Disabled bool   `json:"disabled"`
		Generate bool   `json:"generate"`
		Demo     *bool  `json:"demo"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	name := strings.ToLower(r.PathValue("name"))
	if name == se.Username && (b.Role != s.Auth.UserRole(se.Username) || b.Disabled) {
		fail(w, 422, "you cannot change your own role or disable yourself")
		return
	}
	if b.Generate {
		b.Password = auth.GeneratePassword()
	}
	demo := false
	for _, u := range s.Auth.List() {
		if u.Username == name {
			demo = u.Demo
		}
	}
	// Stopping it being the demonstration account comes first, so the same save can make it an administrator.
	if b.Demo != nil && !*b.Demo && demo {
		if err := s.Auth.SetDemo(name, false); err != nil {
			fail(w, 422, err.Error())
			return
		}
		demo = false
	}
	if err := s.Auth.Save(name, b.Name, b.Role, b.Password, b.Disabled); err != nil {
		fail(w, 422, err.Error())
		return
	}
	if b.Demo != nil && *b.Demo && !demo {
		if err := s.Auth.SetDemo(name, true); err != nil {
			fail(w, 422, err.Error())
			return
		}
		demo = true
	}
	s.Eng.Record(a, cad.EvUserSaved, cad.UserInfo{Username: name, Name: b.Name, Role: b.Role, Disabled: b.Disabled, Demo: demo})
	out := map[string]string{}
	if b.Generate {
		out["password"] = b.Password // shown once, to the administrator who made it
	}
	writeJSON(w, 200, out)
}

func (s *Server) removeUser(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	name := r.PathValue("name")
	if name == se.Username {
		fail(w, 422, "you cannot remove yourself")
		return
	}
	if err := s.Auth.Remove(name); err != nil {
		fail(w, 422, err.Error())
		return
	}
	s.Eng.Record(a, cad.EvUserRemoved, map[string]string{"id": name})
	w.WriteHeader(http.StatusNoContent)
}

// IntegrationsView is the integration configuration with secrets replaced by whether they are set.
type IntegrationsView struct {
	BitVoiceURL         string     `json:"bitvoiceUrl"`
	BitVoiceFingerprint string     `json:"bitvoiceFingerprint"`
	BitVoiceToken       bool       `json:"bitvoiceTokenSet"`
	EIDOToken           bool       `json:"eidoTokenSet"`
	RadioSecret         bool       `json:"radioSecretSet"`
	ALIListen           string     `json:"aliListen"`
	More                []MoreView `json:"more"` // the other BitVoice systems
}

func (s *Server) integrationsView() IntegrationsView {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.integ.BitVoice
	return IntegrationsView{BitVoiceURL: c.URL, BitVoiceFingerprint: c.Fingerprint, BitVoiceToken: c.Token != "", EIDOToken: c.EIDOToken != "",
		RadioSecret: c.RadioSecret != "", ALIListen: s.integ.ALIListen, More: moreViews(s.integ.More)}
}

func (s *Server) getIntegrations(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	writeJSON(w, 200, s.integrationsView())
}

// saveIntegrations changes the integrations. A secret left empty keeps the current one; "-" clears it.
func (s *Server) saveIntegrations(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	// A field left out keeps its value (so a script can change one thing); the address, pin and ALI
	// listener sent empty clear, the secrets sent empty keep and "-" clears them.
	var in struct {
		BitVoiceURL         *string `json:"bitvoiceUrl"`
		BitVoiceFingerprint *string `json:"bitvoiceFingerprint"`
		BitVoiceToken       string  `json:"bitvoiceToken"`
		EIDOToken           string  `json:"eidoToken"`
		RadioSecret         string  `json:"radioSecret"`
		ALIListen           *string `json:"aliListen"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	s.mu.Lock()
	cur := s.integ
	s.mu.Unlock()
	or := func(p *string, v string) string {
		if p == nil {
			return v
		}
		return *p
	}
	b := struct{ BitVoiceURL, BitVoiceFingerprint, BitVoiceToken, EIDOToken, RadioSecret, ALIListen string }{
		or(in.BitVoiceURL, cur.BitVoice.URL), or(in.BitVoiceFingerprint, cur.BitVoice.Fingerprint), in.BitVoiceToken, in.EIDOToken, in.RadioSecret, or(in.ALIListen, cur.ALIListen)}
	b.BitVoiceURL = strings.TrimRight(strings.TrimSpace(b.BitVoiceURL), "/")
	if b.BitVoiceURL != "" && !strings.HasPrefix(b.BitVoiceURL, "https://") {
		fail(w, 422, "BitVoice's address starts with https://")
		return
	}
	if b.ALIListen != "" {
		if _, _, err := net.SplitHostPort(b.ALIListen); err != nil {
			fail(w, 422, "the ALI listener is host:port, such as 0.0.0.0:5001")
			return
		}
	}
	s.mu.Lock()
	n := s.integ
	var changed []string
	keep := func(name string, dst *string, v string) {
		switch v {
		case "":
		case "-":
			if *dst != "" {
				changed = append(changed, name+" cleared")
			}
			*dst = ""
		default:
			*dst = v
			changed = append(changed, name+" changed")
		}
	}
	if n.BitVoice.URL != b.BitVoiceURL {
		changed = append(changed, "BitVoice address "+b.BitVoiceURL)
	}
	n.BitVoice.URL, n.BitVoice.Fingerprint, n.ALIListen = b.BitVoiceURL, b.BitVoiceFingerprint, b.ALIListen
	keep("BitVoice token", &n.BitVoice.Token, strings.TrimSpace(b.BitVoiceToken))
	keep("EIDO token", &n.BitVoice.EIDOToken, strings.TrimSpace(b.EIDOToken))
	keep("radio secret", &n.BitVoice.RadioSecret, strings.TrimSpace(b.RadioSecret))
	data, _ := json.MarshalIndent(n, "", "  ")
	tmp := s.integrationsPath() + ".tmp"
	err := os.WriteFile(tmp, data, 0o600)
	if err == nil {
		err = os.Rename(tmp, s.integrationsPath())
	}
	if err != nil {
		s.mu.Unlock()
		writeErr(w, err)
		return
	}
	s.integ = n
	s.applyIntegrations()
	s.mu.Unlock()
	s.Eng.Record(a, cad.EvIntegration, map[string]any{"bitvoiceUrl": n.BitVoice.URL, "aliListen": n.ALIListen, "fingerprint": n.BitVoice.Fingerprint, "changes": changed})
	writeJSON(w, 200, s.integrationsView())
}

func (s *Server) testBitVoice(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	msg, err := s.BV.Test(r.Context())
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"detail": msg})
}

func (s *Server) fingerprint(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		URL string `json:"url"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	fp, subject, err := bitvoice.Fingerprint(r.Context(), b.URL)
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"fingerprint": fp, "subject": subject})
}

// newSecret makes a random secret for BitVoice to send (eidoToken, radio.cadSecret).
func (s *Server) newSecret(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	writeJSON(w, 200, map[string]string{"secret": "bd_" + auth.RandomToken(24)})
}

// testALI sends a sample spill to the ALI listener, end to end.
func (s *Server) testALI(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	s.mu.Lock()
	l := s.ali
	s.mu.Unlock()
	if l == nil {
		fail(w, 409, "the ALI listener is off")
		return
	}
	addr := l.Addr()
	if host, port, err := net.SplitHostPort(addr); err == nil && (host == "0.0.0.0" || host == "::") {
		addr = net.JoinHostPort("127.0.0.1", port)
	}
	text := "POS TEST  " + time.Now().Format("01/02 15:04:05") + "\r\n(555) 555-0100  TEST\r\nALI LISTENER TEST\r\nNO LOCATION RECEIVED\r\n"
	if err := ng911.SendSpill(r.Context(), addr, text); err != nil {
		fail(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"detail": "A test spill was sent to " + addr + "; it appears as a call from (555) 555-0100"})
}

func (s *Server) ledgerEvents(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	q := r.URL.Query()
	after, _ := strconv.ParseUint(q.Get("after"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	typ, actor, search := q.Get("type"), q.Get("actor"), strings.ToLower(q.Get("q"))
	// seqs=1,5,9 picks events by number: a report's drill-down.
	var seqs map[uint64]bool
	if v := q.Get("seqs"); v != "" {
		seqs = map[uint64]bool{}
		for _, p := range strings.Split(v, ",") {
			if n, err := strconv.ParseUint(strings.TrimSpace(p), 10, 64); err == nil {
				seqs[n] = true
			}
		}
	}
	evs, err := s.Eng.Events(after, 0, func(e ledger.Event) bool {
		if seqs != nil && !seqs[e.Seq] {
			return false
		}
		if typ != "" && !strings.HasPrefix(e.Type, typ) {
			return false
		}
		if actor != "" && e.Actor != actor {
			return false
		}
		if search != "" && !strings.Contains(strings.ToLower(e.Incident+" "+e.Unit+" "+e.Call+" "+string(e.Data)), search) {
			return false
		}
		return true
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	// Newest first, at most limit.
	for i, j := 0, len(evs)-1; i < j; i, j = i+1, j-1 {
		evs[i], evs[j] = evs[j], evs[i]
	}
	if len(evs) > limit {
		evs = evs[:limit]
	}
	writeJSON(w, 200, map[string]any{"items": evs})
}

func (s *Server) verify(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	writeJSON(w, 200, ledger.Verify(s.Eng.LedgerPath()))
}

func (s *Server) positions(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	writeJSON(w, 200, map[string]any{"items": s.Auth.Active()})
}

// Run starts the background work (the BitVoice stream) until ctx ends.
func (s *Server) Run(ctx context.Context) {
	go s.BV.Run(ctx)
	s.runMore(ctx)
	go s.runSites(ctx)
	go s.droneRule(ctx)
	go s.endDataCalls(ctx)
	go s.alarmAnswers(ctx)
	go s.nerisAuto(ctx)
	go s.stationAlerter(ctx)
	go s.publicFeedPusher(ctx)
	go s.autoDispatcher(ctx)
	go s.lisAuto(ctx)
	go s.policyWatch(ctx)
	go s.translator(ctx)
	if s.Advisory != nil {
		go s.Advisory.Run(ctx)
	}
}

func (s *Server) report(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	from, e1 := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	to, e2 := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if e1 != nil || e2 != nil {
		fail(w, http.StatusBadRequest, "from and to are RFC 3339 times")
		return
	}
	rep, err := s.Eng.Report(from, to)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, rep)
}

func (s *Server) certify(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		From    time.Time `json:"from"`
		To      time.Time `json:"to"`
		Hash    string    `json:"hash"`
		Comment string    `json:"comment"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	if err := s.Eng.CertifyReport(a, b.From, b.To, b.Hash, b.Comment); err != nil {
		writeErr(w, err)
		return
	}
	rep, err := s.Eng.Report(b.From, b.To)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, rep)
}

func (s *Server) advisoryHealth() *advisory.Status {
	if s.Advisory == nil {
		return nil
	}
	st := s.Advisory.Status()
	return &st
}

func (s *Server) siteHealth() []PeerHealth {
	s.sites.mu.Lock()
	defer s.sites.mu.Unlock()
	var out []PeerHealth
	for _, p := range s.sites.cfg.Peers {
		if h := s.sites.health[p.Name]; h != nil {
			out = append(out, *h)
		}
	}
	return out
}

// staged reports whether an administration path is configuration a staging board may change and publish.
func staged(path string) bool {
	for _, p := range []string{"/api/admin/settings", "/api/admin/calltypes", "/api/admin/units", "/api/admin/stations", "/api/admin/plans",
		"/api/admin/guidance", "/api/admin/facilities"} {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}
