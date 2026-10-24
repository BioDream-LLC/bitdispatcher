package api

// Training beyond the built-in scenarios: shared exercises an instructor runs, with calls and changing
// conditions injected as they go; the exercise's timeline kept for evaluation; and staging: an
// administrator changes the configuration on a training board, tries it, and publishes it to the live board.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

var reSafe = regexp.MustCompile(`[^a-z0-9-]+`)

func safeName(s string) string {
	s = reSafe.ReplaceAllString(strings.ToLower(s), "-")
	if len(s) > 40 {
		s = s[:40]
	}
	return strings.Trim(s, "-")
}

// mayInstruct is whether a session may run exercises: supervisors and administrators.
func mayInstruct(se *auth.Session) bool { return auth.AtLeast(se.Role, auth.Supervisor) }

// Inject is something an instructor puts into an exercise.
type Inject struct {
	Kind string `json:"kind"` // call, say, move, unit, emergency, radio
	// A call: who, where, and what they say as the exercise goes (each line after so many seconds).
	Call     string   `json:"call,omitempty"`
	Callback string   `json:"callback,omitempty"`
	Name     string   `json:"name,omitempty"`
	Class    string   `json:"class,omitempty"`
	Lat      *float64 `json:"lat,omitempty"`
	Lon      *float64 `json:"lon,omitempty"`
	Radius   float64  `json:"radius,omitempty"`
	Method   string   `json:"method,omitempty"`
	Address  string   `json:"address,omitempty"`
	Lines    []struct {
		After   int    `json:"after"`
		Speaker string `json:"speaker"`
		Text    string `json:"text"`
	} `json:"lines,omitempty"`
	// A unit: a status change (out of service, available), an emergency button, or words on the radio.
	Unit   string `json:"unit,omitempty"`
	Status string `json:"status,omitempty"`
	Text   string `json:"text,omitempty"`
}

func (s *Server) trainingMoreRoutes() {
	s.handle("POST /api/training/inject", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if !mayInstruct(se) {
			fail(w, 403, "an instructor (a supervisor or administrator) injects into an exercise")
			return
		}
		var in Inject
		if !readJSON(w, r, &in) {
			return
		}
		if err := s.inject(a, in); err != nil {
			writeErr(w, err)
			return
		}
		s.Eng.Record(a, cad.EvTrainingInject, in)
		w.WriteHeader(204)
	})
	// The exercise's timeline so far: every event on this board, for the debrief.
	s.handle("GET /api/training/timeline", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		evs, err := s.Eng.Events(0, 0, nil)
		if err != nil {
			writeErr(w, err)
			return
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="exercise-%s-%s.json"`, firstNonEmpty(safeName(s.exercise), "practice"), time.Now().Format("20060102-1504")))
		writeJSON(w, 200, map[string]any{"exercise": s.exercise, "events": evs})
	})
	// Staging: what differs from the live board, and publishing it.
	s.handle("GET /api/training/staged", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if s.liveServer == nil {
			fail(w, 404, "staging is on a training board")
			return
		}
		writeJSON(w, 200, stagedDiff(s.Eng, s.liveServer.Eng))
	})
	s.handle("POST /api/training/publish", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if s.liveServer == nil {
			fail(w, 404, "staging is on a training board")
			return
		}
		var b struct {
			Parts  []string `json:"parts"`
			Why    string   `json:"why"`
			Agency string   `json:"agency"` // publish only this agency's changes
		}
		if !readJSON(w, r, &b) {
			return
		}
		if strings.TrimSpace(b.Why) == "" {
			fail(w, 422, "say why: the change is recorded on the live board")
			return
		}
		b.Agency = strings.ToUpper(strings.TrimSpace(b.Agency))
		var refuse bool
		s.liveServer.Eng.Read(func(st *cad.State) {
			refuse = !st.AllAgencies(se.Username) && (b.Agency == "" || !st.MayWork(se.Username, b.Agency))
		})
		if refuse {
			fail(w, http.StatusForbidden, "publish the changes of an agency you work for: choose it")
			return
		}
		n, err := publish(a, s.Eng, s.liveServer.Eng, b.Parts, b.Why, b.Agency)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]int{"changes": n})
	})
}

// exerciseRoutes are on the live server: the kept timelines of finished exercises.
func (s *Server) exerciseRoutes() {
	dir := func() string { return filepath.Join(s.DataDir, "exercises") }
	s.handle("GET /api/exercises", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		ents, _ := os.ReadDir(dir())
		out := []map[string]any{}
		for _, e := range ents {
			if info, err := e.Info(); err == nil && strings.HasSuffix(e.Name(), ".jsonl") {
				out = append(out, map[string]any{"name": e.Name(), "bytes": info.Size(), "at": info.ModTime().UTC()})
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i]["name"].(string) > out[j]["name"].(string) })
		writeJSON(w, 200, out)
	})
	s.handle("GET /api/exercises/{name}", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		name := filepath.Base(r.PathValue("name"))
		if !strings.HasSuffix(name, ".jsonl") || strings.Contains(name, "..") {
			fail(w, 404, "no exercise")
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		http.ServeFile(w, r, filepath.Join(dir(), name))
	})
}

func (s *Server) inject(a cad.Actor, in Inject) error {
	actor := cad.Actor{User: "training"}
	switch in.Kind {
	case "call":
		if in.Lat == nil || in.Lon == nil {
			return fmt.Errorf("place the caller: an address or a point on the map")
		}
		id := firstNonEmpty(in.Call, fmt.Sprintf("t-ex-%d", time.Now().UnixNano()%1e8))
		loc := &ng911.Location{Lat: in.Lat, Lon: in.Lon, Radius: max(in.Radius, 5), Method: firstNonEmpty(in.Method, "DBH"), Address: in.Address, Source: "training", At: time.Now()}
		if err := s.Eng.UpsertCall(actor, id, cad.CallData{Source: "training", State: "ringing", Callback: firstNonEmpty(in.Callback, "+15405550100"),
			CallerName: in.Name, Class: firstNonEmpty(in.Class, "WRLS"), Location: loc, Emergency: true}); err != nil {
			return err
		}
		return s.sayLater(id, in, true)
	case "say":
		if in.Call == "" {
			return fmt.Errorf("say which call")
		}
		return s.sayLater(in.Call, in, false)
	case "move":
		if in.Call == "" || in.Lat == nil || in.Lon == nil {
			return fmt.Errorf("say which call, and where the caller is now")
		}
		return s.Eng.UpsertCall(actor, in.Call, cad.CallData{Source: "rebid", Location: &ng911.Location{Lat: in.Lat, Lon: in.Lon, Radius: max(in.Radius, 5),
			Method: firstNonEmpty(in.Method, "DBH"), Source: "rebid", At: time.Now()}})
	case "unit":
		return s.Eng.SetStatus(cad.SystemActor, in.Unit, firstNonEmpty(in.Status, cad.OutOfService), "", "the instructor's inject")
	case "emergency":
		_, err := s.Eng.RaiseEmergency(cad.RadioActor, cad.Emergency{Unit: strings.ToUpper(in.Unit), Channel: "training", ChannelName: "Training"})
		return err
	case "radio":
		return s.Eng.RadioText(cad.RadioActor, cad.RadioLine{Unit: strings.ToUpper(in.Unit), Text: in.Text, Channel: "training"})
	}
	return fmt.Errorf("an inject is a call, say, move, unit, emergency or radio")
}

// sayLater plays a call's lines as the exercise goes; the first answers the call.
func (s *Server) sayLater(id string, in Inject, answer bool) error {
	quit := s.quit
	go func() {
		start := time.Now()
		answered := !answer
		for _, l := range in.Lines {
			select {
			case <-quit:
				return
			case <-time.After(time.Until(start.Add(time.Duration(l.After) * time.Second))):
			}
			d := cad.CallData{Source: "training", Transcript: []cad.TranscriptLine{{Speaker: firstNonEmpty(l.Speaker, "caller"), Text: l.Text, At: time.Now()}}}
			if !answered {
				d.State, d.Answer, answered = "answered", time.Now(), true
			}
			s.Eng.UpsertCall(cad.Actor{User: "training"}, id, d)
		}
	}()
	return nil
}

// stagedDiff says, part by part, how the staging board's configuration differs from the live board's.
func stagedDiff(staged, live *cad.Engine) map[string][]string {
	var a, b *cad.State
	staged.Read(func(st *cad.State) { a = snapshot(st) })
	live.Read(func(st *cad.State) { b = snapshot(st) })
	out := map[string][]string{}
	diffMap(out, "calltypes", keys(a.CallTypes), keys(b.CallTypes), func(k string) bool { return jsonEq(a.CallTypes[k], b.CallTypes[k]) })
	diffMap(out, "units", keys(a.Units), keys(b.Units), func(k string) bool { return jsonEq(a.Units[k].UnitDef, b.Units[k].UnitDef) })
	diffMap(out, "stations", keys(a.Stations), keys(b.Stations), func(k string) bool { return jsonEq(a.Stations[k], b.Stations[k]) })
	diffMap(out, "plans", keys(a.Plans), keys(b.Plans), func(k string) bool {
		x, y := *a.Plans[k], *b.Plans[k]
		x.Updated, y.Updated = time.Time{}, time.Time{}
		return jsonEq(x, y)
	})
	diffMap(out, "guidance", keys(a.Guidance), keys(b.Guidance), func(k string) bool { return jsonEq(a.Guidance[k], b.Guidance[k]) })
	diffMap(out, "facilities", keys(a.Facilities), keys(b.Facilities), func(k string) bool {
		return a.Facilities[k].Name == b.Facilities[k].Name && a.Facilities[k].Kind == b.Facilities[k].Kind && a.Facilities[k].Phone == b.Facilities[k].Phone
	})
	diffMap(out, "agencies", keys(a.Agencies), keys(b.Agencies), func(k string) bool {
		x, y := *a.Agencies[k], *b.Agencies[k]
		x.Updated, y.Updated, x.By, y.By = time.Time{}, time.Time{}, "", ""
		return jsonEq(x, y)
	})
	if !jsonEq(a.Settings, b.Settings) {
		out["settings"] = []string{"changed"}
	}
	return out
}

func snapshot(st *cad.State) *cad.State {
	cp := *st
	return &cp
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func diffMap(out map[string][]string, part string, a, b []string, same func(string) bool) {
	in := map[string]bool{}
	for _, k := range b {
		in[k] = true
	}
	var lines []string
	for _, k := range a {
		switch {
		case !in[k]:
			lines = append(lines, "added "+k)
		case !same(k):
			lines = append(lines, "changed "+k)
		}
		delete(in, k)
	}
	for _, k := range b {
		if in[k] {
			lines = append(lines, "removed "+k)
		}
	}
	if len(lines) > 0 {
		out[part] = lines
	}
}

// publish copies parts of the staging board's configuration to the live board, as the publishing person.
func publish(a cad.Actor, staged, live *cad.Engine, parts []string, why, agency string) (int, error) {
	diff := stagedDiff(staged, live)
	var src, dst *cad.State
	staged.Read(func(st *cad.State) { src = snapshot(st) })
	live.Read(func(st *cad.State) { dst = snapshot(st) })
	if agency != "" {
		diff = onlyAgency(diff, agency, src, dst)
	}
	before := configBefore(diff, dst)
	n := 0
	want := map[string]bool{}
	for _, p := range parts {
		want[p] = true
	}
	for _, part := range configParts {
		lines := diff[part]
		if len(parts) > 0 && !want[part] {
			continue
		}
		for _, l := range lines {
			verb, k, _ := strings.Cut(l, " ")
			var err error
			switch part {
			case "settings":
				err = live.SaveSettings(a, src.Settings)
			case "calltypes":
				if verb == "removed" {
					err = live.RemoveCallType(a, k)
				} else {
					err = live.SaveCallType(a, *src.CallTypes[k])
				}
			case "units":
				if verb == "removed" {
					err = live.RemoveUnit(a, k)
				} else {
					err = live.SaveUnit(a, src.Units[k].UnitDef)
				}
			case "stations":
				if verb == "removed" {
					err = live.RemoveStation(a, k)
				} else {
					err = live.SaveStation(a, *src.Stations[k])
				}
			case "plans":
				if verb == "removed" {
					err = live.RemovePlan(a, k)
				} else {
					_, err = live.SavePlan(a, *src.Plans[k])
				}
			case "guidance":
				if verb == "removed" {
					err = live.RemoveGuidance(a, k)
				} else {
					err = live.SaveGuidance(a, *src.Guidance[k])
				}
			case "facilities":
				if verb == "removed" {
					err = live.RemoveFacility(a, k)
				} else {
					err = live.SaveFacility(a, *src.Facilities[k])
				}
			case "agencies":
				if verb == "removed" {
					err = live.RemoveAgency(a, k)
				} else {
					err = live.SaveAgency(a, *src.Agencies[k])
				}
			}
			if err != nil {
				return n, fmt.Errorf("%s %s: %w", part, k, err)
			}
			n++
		}
	}
	live.Record(a, cad.EvConfigStaged, map[string]any{"parts": diff, "why": why, "changes": n, "agency": agency, "before": before})
	return n, nil
}

func jsonEq(a, b any) bool {
	x, _ := jsonMarshal(a)
	y, _ := jsonMarshal(b)
	return string(x) == string(y)
}

var _ = context.Background

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }
