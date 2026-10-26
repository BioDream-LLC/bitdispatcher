package cad

// One-line field commands in the agency's own grammar, as a function key sends them: Field Initiate (F7)
// and Unit Status (F11) in the classic CAD layout. The person types the identifiers on the command line,
// separated by the agency's delimiter in the agency's order (two delimiters together skip an optional
// field), and the key supplies the command:
//
//	UNIT1.8TH/STEWART..ABC1234  F7   a traffic stop by UNIT1 at 8th and Stewart, plate ABC1234
//	UNIT1.AS                    F11  UNIT1 arrived on scene of its incident
//
// A traffic stop made in one pass is a real officer-initiated incident (type, unit on scene, plate,
// location, the stop's safety timer) that can become a full incident without retyping anything; in two
// passes the unit is only marked out on the stop. Status codes are the agency's: a retired or unknown code
// is refused and the refusal recorded, like any other command the person may not give. Key profiles map
// keys (with Shift) to these actions per duty and screen; the PremierOne-style profile is offered as a
// starting point, and every key does exactly what the same command typed or clicked does on the server.

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// CommandRules are the agency's command-line grammar.
type CommandRules struct {
	Delim string `json:"delim,omitempty"` // between identifiers (empty: ".")
	// FieldInitiate is the order of a field-initiated stop's identifiers: unit, location, type, plate,
	// state, comment, platetype, status (empty: that order).
	FieldInitiate []string `json:"fieldInitiate,omitempty"`
	FIType        string   `json:"fiType,omitempty"`    // the incident type of a stop (empty: TS)
	TwoPass       bool     `json:"twoPass,omitempty"`   // only mark the unit out on the stop, no incident
	FIMarkOut     string   `json:"fiMarkOut,omitempty"` // the mark-out for two passes (empty: TS)
	AutoQuery     bool     `json:"autoQuery,omitempty"` // query the plate (NCIC) when a stop is made
	// UnitStatus is the order of a unit status command's identifiers: unit, status, disposition, location,
	// type, comment (empty: that order); MultiUnit allows several units (UNIT1,UNIT2.AS).
	UnitStatus []string `json:"unitStatus,omitempty"`
	MultiUnit  bool     `json:"multiUnit,omitempty"`
	// StatusCodes are the agency's status codes (empty: the defaults).
	StatusCodes []StatusCode `json:"statusCodes,omitempty"`
	// KeyProfiles map keys to actions; Keys says which profile each duty uses (empty: the standard one).
	KeyProfiles []KeyProfile      `json:"keyProfiles,omitempty"`
	Keys        map[string]string `json:"keys,omitempty"`
}

// StatusCode is one of the agency's status codes.
type StatusCode struct {
	Code    string `json:"code"`
	Status  string `json:"status"` // the status it sets (enroute, onscene…); a mark-out code marks out
	Name    string `json:"name,omitempty"`
	Retired bool   `json:"retired,omitempty"`
}

// DefaultStatusCodes are the usual codes.
func DefaultStatusCodes() []StatusCode {
	return []StatusCode{
		{Code: "ER", Status: Enroute, Name: "En route"}, {Code: "ENRT", Status: Enroute, Name: "En route"},
		{Code: "AS", Status: OnScene, Name: "Arrived on scene"}, {Code: "OS", Status: OnScene, Name: "On scene"},
		{Code: "TR", Status: Transporting, Name: "Transporting"}, {Code: "AD", Status: AtDestination, Name: "At destination"},
		{Code: "AV", Status: Available, Name: "Available"}, {Code: "CLR", Status: Available, Name: "Clear"},
		{Code: "OOS", Status: OutOfService, Name: "Out of service"},
	}
}

// KeyBinding maps a key on a screen to an action.
type KeyBinding struct {
	Key    string `json:"key"`              // F1…F12, Shift+F1…, Ctrl+K
	Screen string `json:"screen,omitempty"` // console, taker, mdt or empty for every screen
	Action string `json:"action"`
	Label  string `json:"label,omitempty"`
}

// KeyProfile is a set of key bindings.
type KeyProfile struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Bindings []KeyBinding `json:"bindings"`
}

// KeyActions are what a key can do.
var KeyActions = map[string]string{
	"home": "Go to the main screen", "help": "Open the manual", "queries": "NCIC inquiry", "atm": "Unit-to-unit message",
	"update": "Update the incident", "recall": "Recall (search incidents, units and BOLOs)", "audit": "Incident history (audit trail)",
	"comments": "Comments", "clear": "Clear the form (next call)", "premise": "Premise information", "email": "Messages and BOLOs",
	"pending": "Refresh the pending list", "fieldInitiate": "Field initiate (process the command line)", "previous": "Previous incidents",
	"initiate": "New incident", "nextPage": "Next incident", "dispatch": "Dispatch the recommended units", "prevPage": "Previous incident",
	"command": "Process the command line", "premiseMenu": "Premise and location menu", "unitStatus": "Unit status (process the command line)",
	"display911": "The 911 call's information", "submit": "Submit the form", "plates": "Previous plates", "search": "Search",
	"aerial": "Aerial imagery", "ncic": "NCIC inquiry for the incident", "palette": "Command palette", "rebid": "Rebid the caller's location",
	"narrative": "Go to the narrative", "markout": "Mark the unit out",
	"newCall": "New call-entry screen (keeps this call as it is)", "closeCall": "Close out this call (after it was sent)", "requests": "NCIC requests",
}

// ClassicKeys is the PremierOne-style layout (the manual's defaults, which agencies customise).
func ClassicKeys() KeyProfile {
	k := func(key, action string) KeyBinding { return KeyBinding{Key: key, Action: action} }
	return KeyProfile{ID: "classic", Name: "Classic CAD (PremierOne-style)", Bindings: []KeyBinding{
		k("F1", "home"), k("Shift+F1", "help"), k("F2", "queries"), k("Shift+F2", "atm"), k("F3", "update"), k("Shift+F3", "recall"),
		k("F4", "audit"), k("Shift+F4", "comments"), k("F5", "clear"), k("Shift+F5", "premise"), k("F6", "email"), k("Shift+F6", "pending"),
		k("F7", "fieldInitiate"), k("Shift+F7", "previous"), k("F8", "initiate"), k("Shift+F8", "nextPage"), k("F9", "dispatch"),
		k("Shift+F9", "prevPage"), k("F10", "command"), k("Shift+F10", "premiseMenu"), k("F11", "unitStatus"), k("Shift+F11", "display911"),
		k("F12", "submit"), k("Shift+F12", "plates"),
	}}
}

// StandardKeys is bitdispatcher's own layout.
func StandardKeys() KeyProfile {
	k := func(key, screen, action string) KeyBinding {
		return KeyBinding{Key: key, Screen: screen, Action: action}
	}
	return KeyProfile{ID: "standard", Name: "bitdispatcher standard", Bindings: []KeyBinding{
		k("F2", "taker", "clear"), k("F3", "taker", "search"), k("F4", "taker", "submit"), k("F5", "taker", "newCall"), k("F6", "taker", "rebid"), k("F8", "taker", "narrative"),
		k("Ctrl+Enter", "taker", "submit"), k("Alt+N", "taker", "newCall"),
		k("F2", "console", "initiate"), k("F7", "console", "aerial"), k("F9", "console", "ncic"), k("Ctrl+K", "", "palette"),
		k("Shift+F3", "", "recall"), k("Shift+F6", "", "email"), k("Shift+F7", "console", "fieldInitiate"), k("Shift+F11", "console", "unitStatus"),
	}}
}

// KeyProfileFor is the profile a duty uses.
func (s Settings) KeyProfileFor(duty string) KeyProfile {
	id := s.Commands.Keys[duty]
	if id == "" {
		id = s.Commands.Keys["*"]
	}
	for _, p := range s.Commands.KeyProfiles {
		if p.ID == id {
			return p
		}
	}
	if id == "classic" {
		return ClassicKeys()
	}
	return StandardKeys()
}

// KeyConflicts are the problems with a profile: one key doing two things on a screen, keys the browser
// keeps for itself, unknown actions.
func KeyConflicts(p KeyProfile) []string {
	var out []string
	seen := map[string]string{}
	reserved := map[string]string{"Ctrl+W": "closes the browser tab", "Ctrl+T": "opens a tab", "Ctrl+N": "opens a window", "Ctrl+Tab": "changes tab", "Alt+F4": "closes the window"}
	for _, b := range p.Bindings {
		if _, ok := KeyActions[b.Action]; !ok {
			out = append(out, fmt.Sprintf("%s: no action %q", b.Key, b.Action))
		}
		if why, ok := reserved[b.Key]; ok {
			out = append(out, fmt.Sprintf("%s %s in the browser: choose another key", b.Key, why))
		}
		for _, sc := range []string{b.Screen} {
			k := b.Key + "|" + sc
			if prev, ok := seen[k]; ok && prev != b.Action {
				out = append(out, fmt.Sprintf("%s does both %s and %s on %s", b.Key, prev, b.Action, firstNonEmpty(sc, "every screen")))
			}
			seen[k] = b.Action
		}
		if b.Screen != "" {
			if prev, ok := seen[b.Key+"|"]; ok && prev != b.Action {
				out = append(out, fmt.Sprintf("%s does %s everywhere and %s on %s", b.Key, prev, b.Action, b.Screen))
			}
		}
	}
	if b := slicesF(p.Bindings, "F5"); b != "" && b != "clear" {
		out = append(out, "F5 reloads the page in a browser: the console stops it, but a key map that relies on it may surprise people")
	}
	return out
}

func slicesF(bs []KeyBinding, key string) string {
	for _, b := range bs {
		if b.Key == key {
			return b.Action
		}
	}
	return ""
}

func (r CommandRules) delim() string { return firstNonEmpty(r.Delim, ".") }

func (r CommandRules) fiOrder() []string {
	if len(r.FieldInitiate) > 0 {
		return r.FieldInitiate
	}
	return []string{"unit", "location", "type", "plate", "state", "comment", "platetype", "status"}
}

func (r CommandRules) usOrder() []string {
	if len(r.UnitStatus) > 0 {
		return r.UnitStatus
	}
	return []string{"unit", "status", "disposition", "location", "type", "comment"}
}

func (r CommandRules) codes() []StatusCode {
	if len(r.StatusCodes) > 0 {
		return r.StatusCodes
	}
	return DefaultStatusCodes()
}

// fields splits a one-line string into the agency's identifiers.
func (r CommandRules) fields(line string, order []string) map[string]string {
	parts := strings.Split(strings.TrimSpace(line), r.delim())
	out := map[string]string{}
	for i, p := range parts {
		if i >= len(order) {
			// Extra identifiers go into the comment.
			out["comment"] = strings.TrimSpace(out["comment"] + " " + strings.Join(parts[i:], r.delim()))
			break
		}
		if p = strings.TrimSpace(p); p != "" {
			out[order[i]] = p
		}
	}
	return out
}

// place turns "8TH/STEWART" into an intersection address.
func place(s string) string {
	if a, b, ok := strings.Cut(s, "/"); ok {
		return strings.TrimSpace(a) + " & " + strings.TrimSpace(b)
	}
	return s
}

// fieldInitiate handles FI <string>.
func (e *Engine) fieldInitiate(a Actor, line string) CommandResult {
	var rules CommandRules
	e.Read(func(s *State) { rules = s.Settings.Commands })
	f := rules.fields(line, rules.fiOrder())
	unit := strings.ToUpper(f["unit"])
	if unit == "" {
		return CommandResult{Message: "Field initiate: give the unit first: UNIT1" + rules.delim() + "8TH/STEWART" + rules.delim() + rules.delim() + "ABC1234", Form: "field-initiate"}
	}
	var u *Unit
	var timer int
	e.Read(func(s *State) {
		if x := s.Units[unit]; x != nil {
			c := *x
			u = &c
		}
	})
	if u == nil {
		return CommandResult{Message: "No unit " + unit, Refused: true}
	}
	if f["location"] == "" {
		return CommandResult{Message: "Field initiate: where? " + unit + rules.delim() + "8TH/STEWART", Form: "field-initiate", Unit: unit}
	}
	loc := place(strings.ToUpper(f["location"]))
	plate, state := strings.ToUpper(f["plate"]), strings.ToUpper(f["state"])
	note := strings.TrimSpace(f["comment"])
	if rules.TwoPass {
		mo := firstNonEmpty(rules.FIMarkOut, "TS")
		text := strings.TrimSpace(strings.Join([]string{plate, state, note}, " "))
		if err := e.MarkOutUnit(a, unit, mo, locAt(loc), text); err != nil {
			return CommandResult{Message: err.Error(), Refused: isDuty(err)}
		}
		return CommandResult{OK: true, Message: fmt.Sprintf("%s marked out %s at %s %s", unit, mo, loc, text)}
	}
	typ := strings.ToUpper(firstNonEmpty(f["type"], rules.FIType, "TS"))
	ok := false
	e.Read(func(s *State) { _, ok = s.CallTypes[typ] })
	if !ok {
		return CommandResult{Message: "No call type " + typ + " for a field-initiated stop (Admin › Call types)"}
	}
	if Committed(u.Status) && u.Incident != "" {
		return CommandResult{Message: fmt.Sprintf("%s is on %s: clear it or stack it first", unit, u.Incident), Incident: u.Incident}
	}
	if err := func() error { e.mu.Lock(); defer e.mu.Unlock(); return e.st.mayCommand(a, u) }(); err != nil {
		return CommandResult{Message: err.Error(), Refused: true}
	}
	n, err := e.CreateIncident(a, NewIncident{Type: typ, Location: locAt(loc), Narrative: firstNonEmpty(note, "Field-initiated by "+unit), Agency: u.Agency})
	if err != nil {
		return CommandResult{Message: err.Error()}
	}
	if err := e.Dispatch(a, n, []string{unit}, []string{unit}); err != nil {
		return CommandResult{Message: err.Error(), Incident: n}
	}
	status := OnScene
	if c := f["status"]; c != "" {
		st, _, refuse := e.statusCode(c)
		if refuse != "" {
			return CommandResult{Message: refuse, Incident: n, Refused: true}
		}
		status = st
	}
	if err := e.SetStatus(a, unit, status, n, "field-initiated"); err != nil {
		return CommandResult{Message: err.Error(), Incident: n}
	}
	if plate != "" {
		e.SaveVehicle(a, n, Vehicle{Role: "involved", Plate: plate, State: state, Description: plateType(f["platetype"])})
	}
	e.Read(func(s *State) {
		if x := s.Units[unit]; x != nil {
			timer = s.TimerForUnit(x)
		}
	})
	msg := fmt.Sprintf("%s %s: %s at %s", n, typ, unit, loc)
	if plate != "" {
		msg += ", plate " + strings.TrimSpace(plate+" "+state)
	}
	if timer > 0 {
		msg += fmt.Sprintf("; status check in %d min", timer)
	}
	return CommandResult{OK: true, Message: msg, Incident: n, Plate: plate, PlateState: state}
}

func locAt(addr string) *ng911.Location { return &ng911.Location{Address: addr, Source: "dispatcher"} }

// statusCode finds the agency's status code: the status it sets, a mark-out code, or why it is refused.
func (e *Engine) statusCode(code string) (status, markout, refuse string) {
	code = strings.ToUpper(strings.TrimSpace(code))
	e.Read(func(s *State) {
		for _, c := range s.Settings.Commands.codes() {
			if strings.EqualFold(c.Code, code) {
				if c.Retired {
					refuse = code + " is a retired status code"
					return
				}
				status = c.Status
				return
			}
		}
		if t, ok := s.Settings.MarkOutType(code); ok {
			markout = t.Code
			return
		}
		refuse = "No status code " + code
	})
	if status != "" && status != MarkedOut && !slices.Contains(Statuses, status) {
		return "", "", code + " sets no known status"
	}
	return
}

// unitStatus handles US <string>.
func (e *Engine) unitStatus(a Actor, line string) CommandResult {
	var rules CommandRules
	e.Read(func(s *State) { rules = s.Settings.Commands })
	f := rules.fields(line, rules.usOrder())
	units := strings.Split(strings.ToUpper(f["unit"]), ",")
	if f["unit"] == "" {
		return CommandResult{Message: "Unit status: give the unit and status: UNIT1" + rules.delim() + "AS", Form: "unit-status"}
	}
	if len(units) > 1 && !rules.MultiUnit {
		return CommandResult{Message: "One unit at a time (the agency does not allow several in one status command)", Refused: true}
	}
	if f["status"] == "" {
		// Only the unit: the details form opens; nothing is guessed.
		return CommandResult{Message: "Unit status for " + units[0] + ": choose the status", Form: "unit-status", Unit: units[0]}
	}
	st, mo, refuse := e.statusCode(f["status"])
	if refuse != "" {
		return CommandResult{Message: refuse, Refused: true}
	}
	var done []string
	for _, u := range units {
		u = strings.TrimSpace(u)
		var err error
		if mo != "" {
			var l *ng911.Location
			if f["location"] != "" {
				l = locAt(place(strings.ToUpper(f["location"])))
			}
			err = e.MarkOutUnit(a, u, mo, l, f["comment"])
		} else {
			reason := f["comment"]
			err = e.SetStatus(a, u, st, "", reason)
			if err == nil && st == Available && f["disposition"] != "" {
				reason = "disposition " + strings.ToUpper(f["disposition"])
			}
		}
		if err != nil {
			return CommandResult{Message: err.Error(), Refused: isDuty(err)}
		}
		done = append(done, u)
	}
	label := StatusLabel(st)
	if mo != "" {
		label = "marked out " + mo
	}
	inc := ""
	e.Read(func(s *State) {
		if x := s.Units[done[0]]; x != nil {
			inc = x.Incident
		}
	})
	return CommandResult{OK: true, Message: strings.Join(done, ", ") + " " + label, Incident: inc}
}

// isDuty reports whether an error is a refusal of the person's right (their duty or role).
func isDuty(err error) bool {
	var p *Problem
	return errors.As(err, &p) && (p.Field == "duty" || p.Field == "role" || p.Field == "retired")
}

func plateType(t string) string {
	if t == "" {
		return ""
	}
	return "plate type " + strings.ToUpper(t)
}
