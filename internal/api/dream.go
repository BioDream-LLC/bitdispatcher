package api

// The wishlist's API: inbound hooks for transcripts, crash data, drones and radio words; translation and
// replies to callers; screening; care after critical calls; breaks; the denial-of-service warning; the
// workload board and the quality review; and the drone launch rule.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/acn"
	"github.com/biodream-llc/bitdispatcher/internal/assist"
	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// Extra is the configuration of the connections the wishlist added, kept with the integrations.
type Extra struct {
	// HookToken is what senders of transcripts, crash data, drone reports and radio words present
	// (Authorization: Bearer). Empty: the hooks are off.
	HookToken string `json:"hookToken"`
	// DroneURL receives a launch request for an incident (POST, JSON); DroneToken is sent with it.
	DroneURL   string `json:"droneUrl"`
	DroneToken string `json:"droneToken"`
	// DroneAuto sends the request on its own for call types marked for a drone.
	DroneAuto bool `json:"droneAuto"`
	// A language model on an OpenAI-compatible chat API translates what the phrasebook cannot. Off unless
	// an administrator sets it: what callers say is sent to it.
	ModelURL  string `json:"modelUrl"`
	ModelKey  string `json:"modelKey"`
	ModelName string `json:"modelName"`
	ModelOn   bool   `json:"modelOn"`
	// FOIAModel lets records requests ask the same model for passages that may be exempt (proposals only,
	// for a person to decide). Off unless an administrator sets it: the record is sent to the model.
	FOIAModel bool `json:"foiaModel"`
	// AlertAnswerURL receives what became of an alarm company's alarms (accepted, dispatched, on scene,
	// closed), with AlertAnswerToken as Bearer; see docs/ALERTS.md. Empty: no answers are sent.
	AlertAnswerURL   string `json:"alertAnswerUrl"`
	AlertAnswerToken string `json:"alertAnswerToken"`
	// StationAlerts tones out the stations of dispatched units; StationGatewayURL (with its token as
	// Bearer) receives each alert for a station alerting controller (docs/STATION-ALERTING.md).
	StationAlerts       bool   `json:"stationAlerts"`
	StationGatewayURL   string `json:"stationGatewayUrl"`
	StationGatewayToken string `json:"stationGatewayToken"`
	// Location providers (a supplemental location service like RapidSOS) post newer caller locations to
	// /api/hooks/location with the hook token; see docs/LOCATION-HOOK.md. Nothing here is a vendor's API.
}

// ExtraView is Extra with its secrets replaced by whether they are set.
type ExtraView struct {
	HookToken  bool   `json:"hookTokenSet"`
	DroneURL   string `json:"droneUrl"`
	DroneToken bool   `json:"droneTokenSet"`
	DroneAuto  bool   `json:"droneAuto"`
	ModelURL   string `json:"modelUrl"`
	ModelKey   bool   `json:"modelKeySet"`
	ModelName  string `json:"modelName"`
	ModelOn    bool   `json:"modelOn"`
	FOIAModel  bool   `json:"foiaModel"`
	AlertURL   string `json:"alertAnswerUrl"`
	AlertToken bool   `json:"alertAnswerTokenSet"`
	Stations   bool   `json:"stationAlerts"`
	GatewayURL string `json:"stationGatewayUrl"`
	GatewayTok bool   `json:"stationGatewayTokenSet"`
}

var hookActor = cad.Actor{User: "hook"}

func (s *Server) extra() Extra {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.integ.Extra
}

func (s *Server) dreamRoutes() {
	m := s.mux
	m.HandleFunc("POST /api/hooks/transcript", s.hook(s.hookTranscript))
	m.HandleFunc("POST /api/hooks/acn", s.hook(s.hookACN))
	m.HandleFunc("POST /api/hooks/drone", s.hook(s.hookDrone))
	m.HandleFunc("POST /api/hooks/radio-text", s.hook(s.hookRadioText))
	m.HandleFunc("POST /api/hooks/location", s.hook(s.hookLocation))

	s.handle("GET /api/phrases", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		writeJSON(w, 200, map[string]any{"items": assist.Phrases})
	})
	s.handle("POST /api/translate", auth.Dispatcher, s.translate)
	s.handle("POST /api/calls/{id}/reply", auth.Dispatcher, s.reply)
	s.handle("POST /api/calls/{id}/screen", auth.Dispatcher, s.screenCall)
	s.handle("POST /api/calls/attach-all", auth.Dispatcher, s.attachAll)
	s.handle("POST /api/incidents/{n}/drone", auth.Dispatcher, s.requestDrone)
	s.handle("POST /api/care/{n}", auth.Supervisor, s.care)
	s.handle("POST /api/me/break", auth.Viewer, s.setBreak)
	s.handle("POST /api/tdos/ack", auth.Supervisor, s.ackTDoS)
	s.handle("GET /api/workload", auth.Supervisor, s.workload)
	s.handle("GET /api/reports/quality", auth.Supervisor, s.quality)
	s.handle("GET /api/admin/connections", auth.Admin, s.getExtra)
	s.handle("PUT /api/admin/connections", auth.Admin, s.saveExtra)
}

// hook wraps an inbound hook: off without a token, and the token checked in constant time.
func (s *Server) hook(h func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := s.extra().HookToken
		if tok == "" {
			fail(w, http.StatusServiceUnavailable, "the hooks are off: an administrator sets a hook token in Admin › Integrations")
			return
		}
		if !bearerEqual(r, tok) {
			fail(w, http.StatusUnauthorized, "wrong or missing bearer token")
			return
		}
		h(w, r)
	}
}

func bearerEqual(r *http.Request, secret string) bool {
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return len(got) == len(secret) && subtleEq(got, secret)
}

func subtleEq(a, b string) bool {
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// hookTranscript takes lines of a call's transcript from any speech-to-text engine.
func (s *Server) hookTranscript(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Call    string               `json:"call"`
		Speaker string               `json:"speaker"`
		Text    string               `json:"text"`
		Lang    string               `json:"lang"`
		English string               `json:"english"`
		At      time.Time            `json:"at"`
		Lines   []cad.TranscriptLine `json:"lines"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	if b.Call == "" {
		fail(w, 422, "call is the call's ID")
		return
	}
	lines := b.Lines
	if b.Text != "" {
		lines = append(lines, cad.TranscriptLine{Speaker: b.Speaker, Text: b.Text, Lang: b.Lang, English: b.English, At: b.At})
	}
	if len(lines) == 0 {
		fail(w, 422, "send text, or lines")
		return
	}
	known := false
	s.Eng.Read(func(st *cad.State) { _, known = st.Calls[b.Call] })
	if !known {
		fail(w, 404, "no call "+b.Call)
		return
	}
	if err := s.Eng.UpsertCall(hookActor, b.Call, cad.CallData{Source: "transcript", Transcript: lines}); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// hookLocation takes a newer caller location from a supplemental location provider (or a rebid relayed
// by another system): for a call by its ID, or the open call from a callback number. It becomes the call's
// location; an incident the call is on is offered it, for the call-taker to accept.
func (s *Server) hookLocation(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Call     string    `json:"call"`
		Callback string    `json:"callback"`
		Lat      *float64  `json:"lat"`
		Lon      *float64  `json:"lon"`
		Radius   float64   `json:"radius"`
		Altitude *float64  `json:"altitude"`
		VRadius  float64   `json:"vRadius"`
		Method   string    `json:"method"`
		Address  string    `json:"address"`
		Source   string    `json:"source"` // the provider: rapidsos, ali-rebid, ...
		At       time.Time `json:"at"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	if b.Lat == nil || b.Lon == nil || *b.Lat < -90 || *b.Lat > 90 || *b.Lon < -180 || *b.Lon > 180 {
		fail(w, 422, "lat and lon are the caller's position")
		return
	}
	id := b.Call
	s.Eng.Read(func(st *cad.State) {
		if id != "" {
			if _, ok := st.Calls[id]; !ok {
				id = ""
			}
			return
		}
		want := digitsOnly(b.Callback)
		var best *cad.Call
		for _, c := range st.Calls {
			if c.State != "ended" && want != "" && digitsOnly(c.Callback) == want && (best == nil || c.Start.After(best.Start)) {
				best = c
			}
		}
		if best != nil {
			id = best.ID
		}
	})
	if id == "" {
		fail(w, 404, "no open call for that call or callback")
		return
	}
	if b.At.IsZero() {
		b.At = time.Now().UTC()
	}
	src := firstNonEmpty(strings.ToLower(strings.TrimSpace(b.Source)), "provider")
	loc := &ng911.Location{Lat: b.Lat, Lon: b.Lon, Radius: b.Radius, Altitude: b.Altitude, VRadius: b.VRadius, Method: firstNonEmpty(b.Method, "provider"),
		Address: b.Address, Source: src, At: b.At}
	if err := s.Eng.UpsertCall(cad.Actor{User: src}, id, cad.CallData{Source: src, Location: loc}); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"call": id})
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	if len(d) == 11 && d[0] == '1' {
		d = d[1:]
	}
	return d
}

// hookACN takes a crash notification from a vehicle or a telematics provider.
func (s *Server) hookACN(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	cr, err := acn.Parse(raw)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	id := r.URL.Query().Get("call")
	if id == "" {
		id = fmt.Sprintf("acn-%d", time.Now().UnixMilli())
	}
	d := cad.CallData{Source: "acn", State: "data", Crash: &cr, Callback: cr.Callback, Class: "ACN", Provider: cr.Provider, Emergency: true}
	if cr.Lat != nil && cr.Lon != nil {
		lat, lon := *cr.Lat, *cr.Lon
		d.Location = pointLoc(lat, lon, 10, "GPS", "acn")
	}
	if err := s.Eng.UpsertCall(cad.Actor{User: "acn"}, id, d); err != nil {
		writeErr(w, err)
		return
	}
	as := acn.Assess(cr, s.settings().CrashThresholds())
	writeJSON(w, 200, map[string]any{"call": id, "assessment": as})
}

// hookDrone takes a drone's status, position and live picture from the drone system.
func (s *Server) hookDrone(w http.ResponseWriter, r *http.Request) {
	var d cad.Drone
	if !readJSON(w, r, &d) {
		return
	}
	if err := s.Eng.UpdateDrone(cad.Actor{User: "drone"}, d); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// hookRadioText takes the words of a radio transmission, from a radio system's speech-to-text.
func (s *Server) hookRadioText(w http.ResponseWriter, r *http.Request) {
	var b struct {
		cad.RadioLine
		RadioID int    `json:"radioId"`
		Alias   string `json:"alias"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	if b.Unit == "" && (b.RadioID != 0 || b.Alias != "") {
		b.Unit = s.Eng.UnitByRadio(b.RadioID, b.Alias)
	}
	if err := s.Eng.RadioText(cad.RadioActor, b.RadioLine); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func pointLoc(lat, lon, radius float64, method, source string) *ng911.Location {
	return &ng911.Location{Lat: &lat, Lon: &lon, Radius: radius, Method: method, Source: source}
}

func (s *Server) settings() cad.Settings {
	var st cad.Settings
	s.Eng.Read(func(x *cad.State) { st = x.Settings })
	return st
}

// ---- translation and replies ----

// translateText translates with the language model when it is on, and the phrasebook otherwise.
func (s *Server) translateText(ctx context.Context, text, from, to string) (out, by string, coverage float64) {
	if x := s.extra(); x.ModelOn && x.ModelURL != "" && x.ModelName != "" {
		if t, err := modelTranslate(ctx, x, text, from, to); err == nil && t != "" {
			return t, "model", 1
		} else if err != nil {
			s.Log.Warn("translation model failed; using the phrasebook", "err", err)
		}
	}
	tr := assist.Translate(text, from, to)
	return tr.Text, "phrasebook", tr.Coverage
}

var langNames = map[string]string{"en": "English", "es": "Spanish"}

func modelTranslate(ctx context.Context, x Extra, text, from, to string) (string, error) {
	name := func(l string) string { return firstNonEmptyStr(langNames[l], l) }
	body, _ := json.Marshal(map[string]any{"model": x.ModelName, "temperature": 0, "messages": []map[string]string{
		{"role": "system", "content": "You translate for a 911 emergency call centre. Translate the user's words from " + name(from) +
			" to " + name(to) + ". Keep addresses, names and numbers exactly. Reply with the translation only."},
		{"role": "user", "content": text}}})
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(x.ModelURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if x.ModelKey != "" {
		req.Header.Set("Authorization", "Bearer "+x.ModelKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("model: %s", resp.Status)
	}
	var v struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &v); err != nil || len(v.Choices) == 0 {
		return "", errors.New("model: no answer")
	}
	return strings.TrimSpace(v.Choices[0].Message.Content), nil
}

func (s *Server) translate(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct{ Text, From, To string }
	if !readJSON(w, r, &b) {
		return
	}
	if b.From == "" {
		b.From = firstNonEmptyStr(assist.DetectLang(b.Text), "en")
	}
	if b.To == "" {
		b.To = "en"
	}
	out, by, cov := s.translateText(r.Context(), b.Text, b.From, b.To)
	writeJSON(w, 200, map[string]any{"text": out, "by": by, "coverage": cov})
}

// reply is the call-taker's words to a caller in the caller's language: translated, recorded on the call's
// transcript, and texted when asked and the call can take a text.
func (s *Server) reply(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		Text string `json:"text"`
		Lang string `json:"lang"`
		Send bool   `json:"send"`
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
	var lang string
	known := false
	s.Eng.Read(func(st *cad.State) {
		if c, ok := st.Calls[id]; ok {
			known, lang = true, c.Lang
		}
	})
	if !known {
		fail(w, 404, "no call "+id)
		return
	}
	lang = firstNonEmptyStr(b.Lang, lang, "en")
	out, by, cov := b.Text, "", 1.0
	if lang != "en" {
		out, by, cov = s.translateText(r.Context(), b.Text, "en", lang)
	}
	line := cad.TranscriptLine{Speaker: "taker", Text: out, Lang: lang, By: by}
	if lang != "en" {
		line.English = b.Text
	}
	if err := s.Eng.UpsertCall(a, id, cad.CallData{Source: "bitdispatcher", Transcript: []cad.TranscriptLine{line}}); err != nil {
		writeErr(w, err)
		return
	}
	sent := ""
	if b.Send {
		if err := s.BV.Text(r.Context(), id, "", out); err != nil {
			sent = "not texted: " + err.Error()
		} else {
			sent = "texted"
		}
	}
	writeJSON(w, 200, map[string]any{"text": out, "lang": lang, "by": by, "coverage": cov, "sent": sent})
}

func (s *Server) screenCall(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		Emergency bool `json:"emergency"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	if err := s.Eng.ScreenCall(a, r.PathValue("id"), b.Emergency); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// attachAll attaches a group of callers about the same thing to one incident.
func (s *Server) attachAll(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		Calls    []string `json:"calls"`
		Incident string   `json:"incident"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	if len(b.Calls) == 0 || b.Incident == "" {
		fail(w, 422, "name the calls and the incident")
		return
	}
	n := 0
	var errs []string
	for _, id := range b.Calls {
		if err := s.Eng.AttachCall(a, id, b.Incident); err != nil {
			errs = append(errs, id+": "+err.Error())
			continue
		}
		n++
	}
	writeJSON(w, 200, map[string]any{"attached": n, "errors": errs})
}

// ---- drones ----

func (s *Server) requestDrone(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	detail, err := s.launchDrone(r.Context(), a, r.PathValue("n"), false)
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"detail": detail})
}

// launchDrone asks the drone system to fly to an incident, and records how it went.
func (s *Server) launchDrone(ctx context.Context, a cad.Actor, number string, auto bool) (string, error) {
	x := s.extra()
	if x.DroneURL == "" {
		return "", errors.New("no drone system is configured (Admin › Integrations)")
	}
	var payload map[string]any
	s.Eng.Read(func(st *cad.State) {
		in, ok := st.Incidents[number]
		if !ok || !in.Location.HasPoint() {
			return
		}
		payload = map[string]any{"incident": in.Number, "type": in.Type, "typeName": func() string {
			if ct, ok := st.CallTypes[in.Type]; ok {
				return ct.Name
			}
			return in.Type
		}(), "priority": in.Priority, "lat": *in.Location.Lat, "lon": *in.Location.Lon, "address": in.Location.Address,
			"summary": st.Summary(in), "requestedBy": a.User, "auto": auto}
	})
	if payload == nil {
		return "", errors.New("the incident is not on the map yet: place it first")
	}
	body, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, x.DroneURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if x.DroneToken != "" {
		req.Header.Set("Authorization", "Bearer "+x.DroneToken)
	}
	resp, err := droneClient.Do(req)
	detail := ""
	ok := err == nil && resp.StatusCode/100 == 2
	if err != nil {
		detail = err.Error()
	} else {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		var v struct {
			Detail string `json:"detail"`
			Drone  string `json:"drone"`
		}
		json.Unmarshal(raw, &v)
		detail = strings.TrimSpace(firstNonEmptyStr(v.Detail, map[bool]string{true: "accepted", false: resp.Status}[ok]))
		if v.Drone != "" {
			detail += " (" + v.Drone + ")"
		}
	}
	s.Eng.DroneRequested(a, number, cad.DroneRequest{OK: ok, Auto: auto, Detail: detail})
	if !ok {
		return "", errors.New("the drone system did not accept it: " + detail)
	}
	return detail, nil
}

var droneClient = &http.Client{Timeout: 10 * time.Second}

// droneRule watches for incidents of call types marked for a drone and asks for one, once each.
// endDataCalls ends crash notifications nobody has acted on after half an hour: a data-only call has no
// hang-up of its own, and left alone it would wait in the call queue for ever. Attaching one to an incident
// ends it at once (in the engine). The record stays in the ledger and the incident history.
func (s *Server) endDataCalls(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		now := s.Eng.Now()
		var old []string
		s.Eng.Read(func(st *cad.State) {
			for id, c := range st.Calls {
				if c.State == "data" && c.Incident == "" && now.Sub(c.Start) > 30*time.Minute {
					old = append(old, id)
				}
			}
		})
		for _, id := range old {
			s.Eng.EndCall(cad.Actor{User: "acn"}, id)
		}
	}
}

func (s *Server) droneRule(ctx context.Context) {
	ch := s.Eng.Subscribe()
	defer s.Eng.Unsubscribe(ch)
	asked := map[string]bool{}
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-ch:
			if n.Inc == "" || asked[n.Inc] || (n.Type != cad.EvIncCreated && n.Type != cad.EvIncChanged) {
				continue
			}
			x := s.extra()
			if !x.DroneAuto || x.DroneURL == "" {
				continue
			}
			want := false
			s.Eng.Read(func(st *cad.State) {
				in, ok := st.Incidents[n.Inc]
				if !ok || in.Status == cad.Closed || !in.Location.HasPoint() {
					return
				}
				ct, ok := st.CallTypes[in.Type]
				want = ok && ct.Drone
			})
			if want {
				asked[n.Inc] = true
				go s.launchDrone(ctx, cad.SystemActor, n.Inc, true)
			}
		}
	}
}

// translator asks the language model for what the phrasebook could only give the gist of.
func (s *Server) translator(ctx context.Context) {
	ch := s.Eng.Subscribe()
	defer s.Eng.Unsubscribe(ch)
	done := map[string]bool{}
	var mu sync.Mutex
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-ch:
			if n.Call == "" || (n.Type != cad.EvCallReceived && n.Type != cad.EvCallUpdated) {
				continue
			}
			if x := s.extra(); !x.ModelOn || x.ModelURL == "" {
				continue
			}
			type job struct {
				i          int
				text, lang string
			}
			var jobs []job
			s.Eng.Read(func(st *cad.State) {
				c, ok := st.Calls[n.Call]
				if !ok {
					return
				}
				for i, l := range c.Transcript {
					key := fmt.Sprintf("%s/%d", c.ID, i)
					mu.Lock()
					d := done[key]
					mu.Unlock()
					if !d && l.Lang != "" && l.Lang != "en" && l.By == "phrasebook" && l.Speaker == "caller" {
						jobs = append(jobs, job{i, l.Text, l.Lang})
						mu.Lock()
						done[key] = true
						mu.Unlock()
					}
				}
			})
			for _, j := range jobs {
				go func(call string, j job) {
					t, err := modelTranslate(ctx, s.extra(), j.text, j.lang, "en")
					if err != nil || t == "" {
						return
					}
					s.Eng.UpsertCall(cad.Actor{User: "translator"}, call, cad.CallData{Source: "translator", Translations: []cad.LineTranslation{{Index: j.i, English: t, By: "model"}}})
				}(n.Call, j)
			}
		}
	}
}

// ---- care, breaks, the warning ----

func (s *Server) care(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct{ Action, User, Note string }
	if !readJSON(w, r, &b) {
		return
	}
	if err := s.Eng.Care(a, r.PathValue("n"), b.Action, b.User, b.Note); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setBreak(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		On bool `json:"on"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	if err := s.Eng.SetBreak(a, b.On); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) ackTDoS(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	if err := s.Eng.AckTDoS(a); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Workload is one signed-in person's last hours, for the supervisor.
type Workload struct {
	Username   string    `json:"username"`
	Name       string    `json:"name"`
	Role       string    `json:"role"`
	Console    string    `json:"console"`
	SignedIn   time.Time `json:"signedIn"`
	Calls      int       `json:"calls"`      // calls taken in the last hour
	Incidents  int       `json:"incidents"`  // incidents created in the last hour
	Actions    int       `json:"actions"`    // entries they made on incidents in the last hour
	Critical   []string  `json:"critical"`   // critical calls worked in the last four hours
	OnBreak    bool      `json:"onBreak"`    // on a break now
	SinceBreak float64   `json:"sinceBreak"` // minutes worked since the last break (or since signing in)
	Level      string    `json:"level"`      // ok, watch, relieve
	Reasons    []string  `json:"reasons"`
}

// Workloads is everyone's workload, and the people least busy for relief.
func (s *Server) Workloads(now time.Time) []Workload {
	sessions := s.Auth.Active()
	first := map[string]auth.Session{}
	for _, se := range sessions {
		if f, ok := first[se.Username]; !ok || se.Created.Before(f.Created) {
			first[se.Username] = se
		}
	}
	out := []Workload{}
	s.Eng.Read(func(st *cad.State) {
		after := st.Settings.BreakAfter()
		for user, se := range first {
			w := Workload{Username: user, Name: se.Name, Role: se.Role, Console: se.Console, SignedIn: se.Created, Critical: []string{}, Reasons: []string{}}
			hour := now.Add(-time.Hour)
			for _, c := range st.Calls {
				if c.Agent == user && c.Answer.After(hour) {
					w.Calls++
				}
			}
			for _, in := range st.Incidents {
				if in.CreatedBy == user && in.Created.After(hour) {
					w.Incidents++
				}
				if in.Updated.Before(hour) {
					continue
				}
				for _, t := range in.Timeline {
					if t.Actor == user && t.At.After(hour) {
						w.Actions++
					}
				}
			}
			for _, cc := range st.Care {
				if cc.At.After(now.Add(-4*time.Hour)) && contains(cc.People, user) {
					w.Critical = append(w.Critical, cc.Incident)
				}
			}
			sort.Strings(w.Critical)
			from := se.Created
			if b := st.Breaks[user]; b != nil {
				w.OnBreak = b.On
				if b.LastEnd.After(from) {
					from = b.LastEnd
				}
			}
			mins := now.Sub(from).Minutes()
			if w.OnBreak {
				mins = 0
			}
			w.SinceBreak = float64(int(mins))
			w.Level = "ok"
			if len(w.Critical) >= 2 {
				w.Level = "relieve"
				w.Reasons = append(w.Reasons, fmt.Sprintf("%d critical calls in four hours", len(w.Critical)))
			} else if len(w.Critical) == 1 {
				w.Level = "watch"
				w.Reasons = append(w.Reasons, "a critical call: "+w.Critical[0])
			}
			if !w.OnBreak && mins >= after.Minutes()*1.5 {
				w.Level = "relieve"
				w.Reasons = append(w.Reasons, fmt.Sprintf("%.0f minutes without a break", mins))
			} else if !w.OnBreak && mins >= after.Minutes() {
				if w.Level == "ok" {
					w.Level = "watch"
				}
				w.Reasons = append(w.Reasons, fmt.Sprintf("%.0f minutes without a break", mins))
			}
			if w.Calls+w.Actions >= 60 {
				if w.Level == "ok" {
					w.Level = "watch"
				}
				w.Reasons = append(w.Reasons, fmt.Sprintf("%d calls and actions in the last hour", w.Calls+w.Actions))
			}
			out = append(out, w)
		}
	})
	rank := map[string]int{"relieve": 0, "watch": 1, "ok": 2}
	sort.Slice(out, func(i, j int) bool {
		if rank[out[i].Level] != rank[out[j].Level] {
			return rank[out[i].Level] < rank[out[j].Level]
		}
		return out[i].Console < out[j].Console
	})
	return out
}

func contains(v []string, x string) bool {
	for _, s := range v {
		if s == x {
			return true
		}
	}
	return false
}

func (s *Server) workload(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	items := s.Workloads(s.Eng.Now())
	// Relief: the least busy person not on a break and not the one in need.
	var relief []string
	busy := append([]Workload(nil), items...)
	sort.Slice(busy, func(i, j int) bool {
		return busy[i].Calls+busy[i].Actions+len(busy[i].Critical)*20 < busy[j].Calls+busy[j].Actions+len(busy[j].Critical)*20
	})
	for _, b := range busy {
		if b.Level == "ok" && !b.OnBreak && b.Role != auth.Viewer && b.Role != auth.Field {
			relief = append(relief, b.Username)
		}
	}
	writeJSON(w, 200, map[string]any{"items": items, "relief": relief})
}

// Quality is the quality review of a period: each incident's review, and each call-taker's average.
type Quality struct {
	From    time.Time         `json:"from"`
	To      time.Time         `json:"to"`
	Reviews []cad.Review      `json:"reviews"`
	Takers  []TakerQuality    `json:"takers"`
	Checks  map[string][2]int `json:"checks"` // name: passed, applied
}

// TakerQuality is one call-taker's reviews in a period.
type TakerQuality struct {
	Taker     string `json:"taker"`
	Incidents int    `json:"incidents"`
	Average   int    `json:"average"`
}

func (s *Server) quality(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	from, e1 := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	to, e2 := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if e1 != nil || e2 != nil {
		fail(w, http.StatusBadRequest, "from and to are RFC 3339 times")
		return
	}
	q := Quality{From: from, To: to, Reviews: []cad.Review{}, Takers: []TakerQuality{}, Checks: map[string][2]int{}}
	s.Eng.Read(func(st *cad.State) {
		for _, in := range st.Incidents {
			if in.Created.Before(from) || !in.Created.Before(to) {
				continue
			}
			q.Reviews = append(q.Reviews, st.Review(in))
		}
	})
	sort.Slice(q.Reviews, func(i, j int) bool { return q.Reviews[i].Incident > q.Reviews[j].Incident })
	sum := map[string][2]int{}
	for _, rv := range q.Reviews {
		t := sum[rv.Taker]
		sum[rv.Taker] = [2]int{t[0] + rv.Score, t[1] + 1}
		for _, c := range rv.Checks {
			if c.Skip {
				continue
			}
			v := q.Checks[c.Name]
			v[1]++
			if c.OK {
				v[0]++
			}
			q.Checks[c.Name] = v
		}
	}
	for t, v := range sum {
		q.Takers = append(q.Takers, TakerQuality{Taker: firstNonEmptyStr(t, "(unknown)"), Incidents: v[1], Average: (v[0] + v[1]/2) / v[1]})
	}
	sort.Slice(q.Takers, func(i, j int) bool { return q.Takers[i].Taker < q.Takers[j].Taker })
	writeJSON(w, 200, q)
}

// ---- configuration ----

func (s *Server) extraView() ExtraView {
	x := s.extra()
	return ExtraView{HookToken: x.HookToken != "", DroneURL: x.DroneURL, DroneToken: x.DroneToken != "", DroneAuto: x.DroneAuto,
		ModelURL: x.ModelURL, ModelKey: x.ModelKey != "", ModelName: x.ModelName, ModelOn: x.ModelOn, FOIAModel: x.FOIAModel,
		AlertURL: x.AlertAnswerURL, AlertToken: x.AlertAnswerToken != "",
		Stations: x.StationAlerts, GatewayURL: x.StationGatewayURL, GatewayTok: x.StationGatewayToken != ""}
}

func (s *Server) getExtra(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	writeJSON(w, 200, s.extraView())
}

// saveExtra changes the connections. A secret left empty keeps the current one; "-" clears it.
func (s *Server) saveExtra(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		HookToken  string `json:"hookToken"`
		DroneURL   string `json:"droneUrl"`
		DroneToken string `json:"droneToken"`
		DroneAuto  bool   `json:"droneAuto"`
		ModelURL   string `json:"modelUrl"`
		ModelKey   string `json:"modelKey"`
		ModelName  string `json:"modelName"`
		ModelOn    bool   `json:"modelOn"`
		FOIAModel  bool   `json:"foiaModel"`
		AlertURL   string `json:"alertAnswerUrl"`
		AlertToken string `json:"alertAnswerToken"`
		Stations   bool   `json:"stationAlerts"`
		GatewayURL string `json:"stationGatewayUrl"`
		GatewayTok string `json:"stationGatewayToken"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	for _, u := range []string{b.DroneURL, b.ModelURL, b.AlertURL, b.GatewayURL} {
		if u = strings.TrimSpace(u); u != "" && !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://127.0.0.1") && !strings.HasPrefix(u, "http://localhost") {
			fail(w, 422, "addresses start with https:// (or http://127.0.0.1 for a program on this server)")
			return
		}
	}
	if (b.ModelOn || b.FOIAModel) && (strings.TrimSpace(b.ModelURL) == "" || strings.TrimSpace(b.ModelName) == "") {
		fail(w, 422, "the language model needs its address and model name")
		return
	}
	s.mu.Lock()
	n := s.integ
	keep := func(dst *string, v string) {
		switch v = strings.TrimSpace(v); v {
		case "":
		case "-":
			*dst = ""
		default:
			*dst = v
		}
	}
	keep(&n.Extra.HookToken, b.HookToken)
	keep(&n.Extra.DroneToken, b.DroneToken)
	keep(&n.Extra.ModelKey, b.ModelKey)
	keep(&n.Extra.AlertAnswerToken, b.AlertToken)
	n.Extra.AlertAnswerURL = strings.TrimSpace(b.AlertURL)
	keep(&n.Extra.StationGatewayToken, b.GatewayTok)
	n.Extra.StationAlerts, n.Extra.StationGatewayURL = b.Stations, strings.TrimSpace(b.GatewayURL)
	n.Extra.DroneURL, n.Extra.DroneAuto = strings.TrimSpace(b.DroneURL), b.DroneAuto
	n.Extra.ModelURL, n.Extra.ModelName, n.Extra.ModelOn = strings.TrimRight(strings.TrimSpace(b.ModelURL), "/"), strings.TrimSpace(b.ModelName), b.ModelOn
	n.Extra.FOIAModel = b.FOIAModel
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
	s.Eng.Record(a, cad.EvIntegration, map[string]any{"connections": true, "hooks": n.Extra.HookToken != "", "droneUrl": n.Extra.DroneURL,
		"droneAuto": n.Extra.DroneAuto, "modelUrl": n.Extra.ModelURL, "modelName": n.Extra.ModelName, "modelOn": n.Extra.ModelOn, "foiaModel": n.Extra.FOIAModel})
	writeJSON(w, 200, s.extraView())
}
