package cad

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// roadmapCommand runs the command-line verbs of the roadmap features: alarms, roll calls, benchmarks,
// tows, pages and the directory.
func (e *Engine) roadmapCommand(a Actor, verb string, args []string, selected string) (CommandResult, bool) {
	fail := func(err error) (CommandResult, bool) { return CommandResult{Message: err.Error()}, true }
	inc := selected
	var rest []string
	for _, w := range args {
		if strings.HasPrefix(w, "#") {
			n := ""
			e.Read(func(s *State) { n = s.FindIncident(w) })
			if n == "" {
				return CommandResult{Message: "no open incident " + w}, true
			}
			inc = n
			continue
		}
		rest = append(rest, w)
	}
	head, tail := rest, ""
	if i := slices.Index(rest, "/"); i >= 0 {
		head, tail = rest[:i], strings.Join(rest[i+1:], " ")
	}
	needInc := func() bool { return inc != "" }
	switch verb {
	case "ALARM", "2ND", "3RD":
		if !needInc() {
			return CommandResult{Message: "Select the incident, or name it: ALARM #26-000123"}, true
		}
		level := 0
		switch verb {
		case "2ND":
			level = 2
		case "3RD":
			level = 3
		}
		if len(head) > 0 {
			n, err := strconv.Atoi(strings.TrimRight(strings.ToLower(head[0]), "ndrsth"))
			if err != nil {
				return CommandResult{Message: "ALARM takes a level: ALARM 2, ALARM 3, or just ALARM for the next"}, true
			}
			level = n
		}
		var err error
		if level == 0 {
			err = e.UpgradeAlarm(a, inc, tail)
		} else {
			err = e.SetAlarm(a, inc, level, tail)
		}
		if err != nil {
			return fail(err)
		}
		name := ""
		e.Read(func(s *State) { name = s.AlarmName(s.Incidents[inc]) })
		return CommandResult{OK: true, Message: inc + " at the " + name + ": REC sends the extra units", Incident: inc}, true
	case "PAR":
		if !needInc() {
			return CommandResult{Message: "Select the incident, or name it: PAR #26-000123"}, true
		}
		sub := ""
		if len(head) > 0 {
			sub = strings.ToUpper(head[0])
		}
		units := func(ws []string) []string {
			out := make([]string, len(ws))
			for i, w := range ws {
				out[i] = strings.ToUpper(w)
			}
			return out
		}
		switch sub {
		case "START":
			m := 0
			if len(head) > 1 {
				m, _ = strconv.Atoi(head[1])
			}
			if err := e.StartFireground(a, inc, m); err != nil {
				return fail(err)
			}
			return CommandResult{OK: true, Message: "Accountability clock running on " + inc, Incident: inc}, true
		case "END", "STOP":
			if err := e.EndFireground(a, inc); err != nil {
				return fail(err)
			}
			return CommandResult{OK: true, Message: "Accountability clock stopped on " + inc, Incident: inc}, true
		case "FOUND":
			if len(head) < 2 {
				return CommandResult{Message: "Name the unit: PAR FOUND E3"}, true
			}
			if err := e.FoundUnit(a, inc, head[1], tail); err != nil {
				return fail(err)
			}
			return CommandResult{OK: true, Message: strings.ToUpper(head[1]) + " accounted for", Incident: inc}, true
		case "MISSING":
			if len(head) < 2 {
				return CommandResult{Message: "Name who did not answer: PAR MISSING E3"}, true
			}
			miss := units(head[1:])
			if err := e.RecordPAR(a, inc, nil, miss, nil, tail); err != nil {
				return fail(err)
			}
			return CommandResult{OK: true, Message: "PAR: NOT ACCOUNTED FOR " + strings.Join(miss, ", ") + " — tell command now", Incident: inc}, true
		}
		if err := e.RecordPAR(a, inc, units(head), nil, nil, tail); err != nil {
			return fail(err)
		}
		return CommandResult{OK: true, Message: "PAR recorded on " + inc + ": all accounted for", Incident: inc}, true
	case "BM":
		if !needInc() {
			return CommandResult{Message: "Select the incident first"}, true
		}
		if len(head) == 0 {
			var codes []string
			e.Read(func(s *State) {
				for _, b := range s.Settings.BenchmarkList() {
					codes = append(codes, b.Code+" "+b.Name)
				}
			})
			return CommandResult{Message: "Say which: " + strings.Join(codes, " · ")}, true
		}
		if err := e.MarkBenchmark(a, inc, strings.Join(head, " ")); err != nil {
			return fail(err)
		}
		return CommandResult{OK: true, Message: "Benchmark recorded on " + inc, Incident: inc}, true
	case "TOW":
		if !needInc() {
			return CommandResult{Message: "Select the incident the tow is for"}, true
		}
		class, zone := "", ""
		e.Read(func(s *State) {
			classes := s.Settings.TowClassList()
			class = classes[0]
			for _, w := range head {
				for _, c := range classes {
					if strings.EqualFold(c, w) {
						class = c
					}
				}
				for _, z := range s.Settings.TowZones {
					if strings.EqualFold(z, w) {
						zone = z
					}
				}
			}
			if zone == "" && len(s.Settings.TowZones) == 1 {
				zone = s.Settings.TowZones[0]
			}
		})
		t, err := e.RequestTow(a, inc, class, zone, TowRotation, "", tail, "")
		if err != nil {
			return fail(err)
		}
		return CommandResult{OK: true, Message: fmt.Sprintf("Call %s %s (%s, next on the %s rotation)", t.Name, t.Phone, t.ID, t.Class), Incident: inc}, true
	case "PAGE":
		if len(head) == 0 {
			return CommandResult{Message: "Name the groups to page: PAGE <group> / text"}, true
		}
		var groups []string
		var miss []string
		e.Read(func(s *State) {
			for _, w := range head {
				found := ""
				for _, g := range s.PageGroups {
					if strings.EqualFold(g.ID, w) || strings.EqualFold(g.Name, w) || strings.HasPrefix(strings.ToLower(g.Name), strings.ToLower(w)) {
						found = g.ID
					}
				}
				if found == "" {
					miss = append(miss, w)
				} else {
					groups = append(groups, found)
				}
			}
		})
		if len(miss) > 0 {
			return CommandResult{Message: "No paging group " + strings.Join(miss, ", ")}, true
		}
		// The delivery is the server's: it watches for pages and sends them (internal/api).
		id, _, err := e.SendPage(a, inc, groups, nil, "", tail, false)
		if err != nil {
			return fail(err)
		}
		return CommandResult{OK: true, Message: id + " sent", Incident: inc}, true
	case "DIR":
		var hits []*DirEntry
		e.Read(func(s *State) { hits = s.SearchDirectory(strings.Join(rest, " ")) })
		if len(hits) == 0 {
			return CommandResult{Message: "Nothing in the directory for " + strings.Join(rest, " ")}, true
		}
		var lines []string
		for i, d := range hits {
			if i == 5 {
				lines = append(lines, fmt.Sprintf("…and %d more: open the directory", len(hits)-5))
				break
			}
			var ps []string
			for _, p := range d.Phones {
				ps = append(ps, strings.TrimSpace(p.Label+" "+p.Number))
			}
			lines = append(lines, d.Name+suffixOrg(d.Org)+" — "+strings.Join(ps, " · "))
		}
		return CommandResult{OK: true, Message: strings.Join(lines, "\n")}, true
	}
	return CommandResult{}, false
}

func suffixOrg(o string) string {
	if o == "" {
		return ""
	}
	return " (" + o + ")"
}
