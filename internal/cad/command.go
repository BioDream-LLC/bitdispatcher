package cad

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// CommandResult is what a command line did.
type CommandResult struct {
	OK       bool   `json:"ok"`
	Message  string `json:"message"`
	Incident string `json:"incident,omitempty"` // the incident to select afterwards
}

// Verb is one command-line verb, for the help and completion.
type Verb struct {
	Words   []string `json:"words"`
	Usage   string   `json:"usage"`
	Meaning string   `json:"meaning"`
}

// Verbs is the command-line language.
var Verbs = []Verb{
	{[]string{"D", "DISP"}, "D <units…> [#incident]", "Dispatch units to the selected (or named) incident"},
	{[]string{"REC"}, "REC [#incident]", "Dispatch the recommended units"},
	{[]string{"ER", "ENRT"}, "ER <units…>", "En route"},
	{[]string{"OS", "ONS"}, "OS <units…>", "On scene"},
	{[]string{"TR"}, "TR <units…>", "Transporting"},
	{[]string{"AD", "AH"}, "AD <units…>", "At destination (hospital)"},
	{[]string{"AV", "AVL", "CLR"}, "AV <units…>", "Available, clears the unit from its incident"},
	{[]string{"OOS"}, "OOS <units…> [reason]", "Out of service"},
	{[]string{"MO", "MARK"}, "MO <unit> <kind> [address] [/ note]", "Mark a unit out: MEAL, COURT, FUEL… with where it is"},
	{[]string{"SC", "CHECK"}, "SC <units…> [note]", "Status check: the unit is all right, its timer starts again"},
	{[]string{"N", "NOTE"}, "N <text>", "Add a note to the selected incident"},
	{[]string{"PN"}, "PN <text>", "Add a priority note every dispatcher and unit sees"},
	{[]string{"NEW", "I"}, "NEW <type> [P1-5] <address>", "Create an incident"},
	{[]string{"P", "PRI"}, "P <1-5>", "Change the selected incident's priority"},
	{[]string{"T", "TYPE"}, "T <type>", "Change the selected incident's call type"},
	{[]string{"CL", "CLOSE"}, "CL <disposition> [text]", "Close the selected incident"},
	{[]string{"RO", "REOPEN"}, "RO #incident <reason>", "Reopen a closed incident"},
	{[]string{"ACK"}, "ACK", "Acknowledge the oldest open emergency"},
	{[]string{"MU", "COVER"}, "MU <unit> <station>", "Move a unit up to cover another station"},
	{[]string{"RET", "HOME"}, "RET <units…>", "Send covering units back to their own station"},
	{[]string{"ATTACH"}, "ATTACH <call> [#incident]", "Attach a 911 call to the selected (or named) incident"},
}

// Execute runs one command line. selected is the incident the dispatcher has selected.
func (e *Engine) Execute(a Actor, line, selected string) CommandResult {
	f := strings.Fields(line)
	if len(f) == 0 {
		return CommandResult{Message: "Type a command; HELP lists them"}
	}
	verb := strings.ToUpper(f[0])
	args := f[1:]
	fail := func(err error) CommandResult { return CommandResult{Message: err.Error()} }
	// A trailing or leading #incident picks the incident.
	pickInc := func() (string, []string, error) {
		var rest []string
		inc := ""
		for _, w := range args {
			if strings.HasPrefix(w, "#") {
				var n string
				e.Read(func(s *State) { n = s.FindIncident(w) })
				if n == "" {
					return "", nil, fmt.Errorf("no open incident %s", w)
				}
				inc = n
				continue
			}
			rest = append(rest, w)
		}
		if inc == "" {
			inc = selected
		}
		return inc, rest, nil
	}
	needInc := func(inc string) error {
		if inc == "" {
			return fmt.Errorf("select an incident first, or name it with #number")
		}
		return nil
	}
	upper := func(ws []string) []string {
		out := make([]string, len(ws))
		for i, w := range ws {
			out[i] = strings.ToUpper(w)
		}
		return out
	}
	status := statusWords
	switch verb {
	case "HELP", "?":
		var b strings.Builder
		for _, v := range Verbs {
			b.WriteString(v.Usage + " — " + v.Meaning + "\n")
		}
		return CommandResult{OK: true, Message: strings.TrimSpace(b.String())}
	case "D", "DISP":
		inc, rest, err := pickInc()
		if err == nil {
			err = needInc(inc)
		}
		if err != nil {
			return fail(err)
		}
		if len(rest) == 0 {
			return CommandResult{Message: "Name the units: D E1 M2"}
		}
		rec, _ := e.Recommend(inc)
		if err := e.Dispatch(a, inc, upper(rest), rec.Units); err != nil {
			return fail(err)
		}
		return CommandResult{OK: true, Message: "Dispatched " + strings.Join(upper(rest), " ") + " to " + inc, Incident: inc}
	case "REC":
		inc, _, err := pickInc()
		if err == nil {
			err = needInc(inc)
		}
		if err != nil {
			return fail(err)
		}
		rec, err := e.Recommend(inc)
		if err != nil {
			return fail(err)
		}
		if len(rec.Units) == 0 {
			return CommandResult{Message: "Nothing to recommend: " + strings.Join(rec.Notes, "; "), Incident: inc}
		}
		if err := e.Dispatch(a, inc, rec.Units, rec.Units); err != nil {
			return fail(err)
		}
		return CommandResult{OK: true, Message: "Dispatched the recommendation " + strings.Join(rec.Units, " ") + " to " + inc, Incident: inc}
	case "ER", "ENRT", "OS", "ONS", "TR", "AD", "AH", "AV", "AVL", "CLR", "OOS":
		if len(args) == 0 {
			return CommandResult{Message: "Name the units: " + verb + " E1"}
		}
		st := status[verb]
		var units []string
		reason := ""
		for i, w := range args {
			var known bool
			e.Read(func(s *State) { _, known = s.Units[strings.ToUpper(w)] })
			if !known {
				if st == OutOfService && i > 0 {
					reason = strings.Join(args[i:], " ")
					break
				}
				return CommandResult{Message: "No unit " + strings.ToUpper(w)}
			}
			units = append(units, strings.ToUpper(w))
		}
		for _, u := range units {
			if err := e.SetStatus(a, u, st, "", reason); err != nil {
				return fail(err)
			}
		}
		return CommandResult{OK: true, Message: strings.Join(units, " ") + " " + StatusLabel(st), Incident: selected}
	case "MO", "MARK":
		if len(args) < 2 {
			var codes []string
			e.Read(func(s *State) {
				for _, t := range s.Settings.MarkOuts {
					codes = append(codes, t.Code+" "+t.Name)
				}
			})
			return CommandResult{Message: "MO <unit> <kind> [address] [/ note]. Kinds: " + strings.Join(codes, ", ")}
		}
		unit, code := strings.ToUpper(args[0]), strings.ToUpper(args[1])
		rest := strings.Join(args[2:], " ")
		where, note, _ := strings.Cut(rest, "/")
		where, note = strings.TrimSpace(where), strings.TrimSpace(note)
		var loc *ng911.Location
		located := ""
		if where != "" {
			loc = &ng911.Location{Address: where, Source: "dispatcher"}
			e.mu.Lock()
			g := e.geocoder
			e.mu.Unlock()
			if g != nil {
				if lat, lon, label, ok := g.Geocode(where); ok {
					loc.Lat, loc.Lon, loc.Address = &lat, &lon, label
				}
			}
			located = " at " + loc.Address
		} else {
			// No place given: where its AVL says it is now.
			e.Read(func(s *State) {
				if u, ok := s.Units[unit]; ok && u.Position != nil {
					la, lo := u.Position.Lat, u.Position.Lon
					loc = &ng911.Location{Lat: &la, Lon: &lo, Source: "avl"}
				}
			})
		}
		if err := e.MarkOutUnit(a, unit, code, loc, note); err != nil {
			return fail(err)
		}
		name := code
		e.Read(func(s *State) {
			if t, ok := s.Settings.MarkOutType(code); ok {
				name = t.Name
			}
		})
		return CommandResult{OK: true, Message: unit + " marked out: " + name + located, Incident: selected}
	case "SC", "CHECK":
		if len(args) == 0 {
			return CommandResult{Message: "SC <units…> [note]"}
		}
		var units []string
		note := ""
		for i, w := range args {
			var known bool
			e.Read(func(s *State) { _, known = s.Units[strings.ToUpper(w)] })
			if !known {
				if i > 0 {
					note = strings.Join(args[i:], " ")
					break
				}
				return CommandResult{Message: "No unit " + strings.ToUpper(w)}
			}
			units = append(units, strings.ToUpper(w))
		}
		for _, u := range units {
			if err := e.CheckUnit(a, u, note); err != nil {
				return fail(err)
			}
		}
		return CommandResult{OK: true, Message: strings.Join(units, " ") + " status checked: all right", Incident: selected}
	case "ATTACH":
		inc, rest, err := pickInc()
		if err == nil {
			err = needInc(inc)
		}
		if err != nil {
			return fail(err)
		}
		if len(rest) != 1 {
			return CommandResult{Message: "ATTACH <call> [#incident]"}
		}
		if err := e.AttachCall(a, rest[0], inc); err != nil {
			return fail(err)
		}
		return CommandResult{OK: true, Message: "Call attached to " + inc, Incident: inc}
	case "MU", "COVER":
		if len(args) != 2 {
			return CommandResult{Message: "MU <unit> <station>: MU E4 ST1"}
		}
		if err := e.Cover(a, args[0], args[1]); err != nil {
			return fail(err)
		}
		return CommandResult{OK: true, Message: strings.ToUpper(args[0]) + " covering " + strings.ToUpper(args[1]), Incident: selected}
	case "RET", "HOME":
		if len(args) == 0 {
			return CommandResult{Message: "RET <units…>"}
		}
		for _, u := range upper(args) {
			if err := e.Cover(a, u, ""); err != nil {
				return fail(err)
			}
		}
		return CommandResult{OK: true, Message: strings.Join(upper(args), " ") + " back to quarters", Incident: selected}
	case "N", "NOTE", "PN":
		inc, rest, err := pickInc()
		if err == nil {
			err = needInc(inc)
		}
		if err != nil {
			return fail(err)
		}
		if err := e.AddNote(a, inc, strings.Join(rest, " "), verb == "PN"); err != nil {
			return fail(err)
		}
		return CommandResult{OK: true, Message: "Note added to " + inc, Incident: inc}
	case "NEW", "I":
		if len(args) == 0 {
			return CommandResult{Message: "NEW <type> [P1-5] <address>"}
		}
		n := NewIncident{Type: strings.ToUpper(args[0])}
		rest := args[1:]
		if len(rest) > 0 && len(rest[0]) == 2 && (rest[0][0] == 'P' || rest[0][0] == 'p') {
			if p, err := strconv.Atoi(rest[0][1:]); err == nil {
				n.Priority, rest = p, rest[1:]
			}
		}
		located := ""
		if addr := strings.Join(rest, " "); addr != "" {
			n.Location = &ng911.Location{Address: addr, Source: "dispatcher"}
			e.mu.Lock()
			g := e.geocoder
			e.mu.Unlock()
			// An address or intersection the recorded map knows exactly gets its position, so units can be
			// ranked by road at once; anything less stays text for the dispatcher to place.
			if g != nil {
				if lat, lon, label, ok := g.Geocode(addr); ok {
					n.Location.Lat, n.Location.Lon, n.Location.Address = &lat, &lon, label
					located = " at " + label
				}
			}
		}
		num, err := e.CreateIncident(a, n)
		if err != nil {
			return fail(err)
		}
		return CommandResult{OK: true, Message: "Created " + num + located, Incident: num}
	case "P", "PRI":
		inc, rest, err := pickInc()
		if err == nil {
			err = needInc(inc)
		}
		if err != nil {
			return fail(err)
		}
		if len(rest) == 0 {
			return CommandResult{Message: "P <1-5>"}
		}
		p, err := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(rest[0]), "P"))
		if err != nil {
			return CommandResult{Message: "Priority is 1 to 5"}
		}
		if err := e.ChangeIncident(a, inc, IncidentChange{Priority: &p, Reason: strings.Join(rest[1:], " ")}); err != nil {
			return fail(err)
		}
		return CommandResult{OK: true, Message: fmt.Sprintf("%s is now P%d", inc, p), Incident: inc}
	case "T", "TYPE":
		inc, rest, err := pickInc()
		if err == nil {
			err = needInc(inc)
		}
		if err != nil {
			return fail(err)
		}
		if len(rest) == 0 {
			return CommandResult{Message: "T <type>"}
		}
		t := strings.ToUpper(rest[0])
		if err := e.ChangeIncident(a, inc, IncidentChange{Type: &t, Reason: strings.Join(rest[1:], " ")}); err != nil {
			return fail(err)
		}
		return CommandResult{OK: true, Message: inc + " is now " + t, Incident: inc}
	case "CL", "CLOSE":
		inc, rest, err := pickInc()
		if err == nil {
			err = needInc(inc)
		}
		if err != nil {
			return fail(err)
		}
		if len(rest) == 0 {
			return CommandResult{Message: "CL <disposition> [text]"}
		}
		// The disposition is the longest run of leading words that names one.
		var disps []string
		e.Read(func(s *State) { disps = s.Settings.Dispositions })
		disp, text := "", ""
		for i := len(rest); i > 0; i-- {
			try := strings.Join(rest[:i], " ")
			for _, d := range disps {
				if strings.EqualFold(d, try) || (i == 1 && strings.HasPrefix(strings.ToUpper(d), strings.ToUpper(try))) {
					disp, text = d, strings.Join(rest[i:], " ")
				}
			}
			if disp != "" {
				break
			}
		}
		if disp == "" {
			return CommandResult{Message: "Dispositions: " + strings.Join(disps, ", ")}
		}
		if err := e.CloseIncident(a, inc, disp, text); err != nil {
			return fail(err)
		}
		return CommandResult{OK: true, Message: inc + " closed: " + disp}
	case "RO", "REOPEN":
		inc := ""
		var rest []string
		for _, w := range args {
			if strings.HasPrefix(w, "#") && inc == "" {
				e.Read(func(s *State) {
					ref := strings.ToUpper(strings.TrimPrefix(w, "#"))
					for n := range s.Incidents {
						if n == ref || strings.HasSuffix(n, ref) {
							inc = n
						}
					}
				})
				continue
			}
			rest = append(rest, w)
		}
		if inc == "" {
			inc = selected
		}
		if err := needInc(inc); err != nil {
			return fail(err)
		}
		if err := e.ReopenIncident(a, inc, strings.Join(rest, " ")); err != nil {
			return fail(err)
		}
		return CommandResult{OK: true, Message: inc + " reopened", Incident: inc}
	case "ACK":
		key := ""
		e.Read(func(s *State) {
			var oldest *Emergency
			for _, em := range s.Emergencies {
				if em.AckedAt.IsZero() && (oldest == nil || em.Raised.Before(oldest.Raised)) {
					oldest = em
				}
			}
			if oldest != nil {
				key = oldest.Key
			}
		})
		if key == "" {
			return CommandResult{Message: "No open emergency"}
		}
		if err := e.AckEmergency(a, key, "bitdispatcher"); err != nil {
			return fail(err)
		}
		return CommandResult{OK: true, Message: "Emergency acknowledged"}
	}
	return CommandResult{Message: "Unknown command " + verb + ": HELP lists them"}
}
