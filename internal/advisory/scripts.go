package advisory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/script"
)

// Script rules: an agency's own JavaScript or Lua in the codebook, versioned, tested against the ledger
// and run in shadow or live mode like every other rule. An incident script is called once for each open
// incident, with a plain copy of it (made under the engine's lock, run after it is released), and what it
// returns becomes suggestions. Recommendation and report scripts are run where those are made.

// KindScript is a rule that is a script.
const KindScript = "script"

// ScriptInput is what an incident script sees.
type ScriptInput struct {
	Number   string   `json:"number"`
	Type     string   `json:"type"`
	TypeName string   `json:"typeName"`
	Priority int      `json:"priority"`
	Status   string   `json:"status"`
	Address  string   `json:"address,omitempty"`
	Lat      float64  `json:"lat,omitempty"`
	Lon      float64  `json:"lon,omitempty"`
	Minutes  float64  `json:"minutes"` // since it was created
	Notes    []string `json:"notes"`
	Units    []string `json:"units"` // on it now
	Hazards  []string `json:"hazards"`
	Caller   string   `json:"caller,omitempty"`
	Calls    int      `json:"calls"`
}

// compiled keeps compiled scripts by their source's hash.
var compiled sync.Map

func compileRule(r Rule) (*script.Script, error) {
	h := sha256.Sum256([]byte(r.Lang + "\x00" + r.Hook + "\x00" + r.Source))
	key := hex.EncodeToString(h[:])
	if s, ok := compiled.Load(key); ok {
		return s.(*script.Script), nil
	}
	s, err := script.Compile(r.Lang, firstNonEmpty(r.Hook, "incident"), r.Source)
	if err != nil {
		return nil, err
	}
	compiled.Store(key, s)
	return s, nil
}

func (r Rule) timeout() time.Duration {
	if r.TimeoutMs > 0 {
		return time.Duration(r.TimeoutMs) * time.Millisecond
	}
	return script.DefaultTimeout
}

// scriptInputs copies what incident scripts see: call it holding the state.
func scriptInputs(c Codebook, s *cad.State, now time.Time) []ScriptInput {
	any := false
	for _, r := range c.Rules {
		any = any || r.Enabled && r.Kind == KindScript && firstNonEmpty(r.Hook, "incident") == "incident"
	}
	if !any {
		return nil
	}
	var out []ScriptInput
	for _, in := range s.Incidents {
		if in.Status != cad.Closed {
			out = append(out, IncidentInput(s, in, now))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

// ScriptFault is a script that failed; the panel carries on without it and shows this.
type ScriptFault struct {
	Rule    string `json:"rule"`
	Name    string `json:"name"`
	Message string `json:"message"`
}

// runScripts runs the incident scripts over the copied incidents.
func runScripts(ctx context.Context, c Codebook, ins []ScriptInput, since map[string]time.Time) ([]Suggestion, []ScriptFault) {
	var out []Suggestion
	var faults []ScriptFault
	for _, r := range c.Rules {
		if !r.Enabled || r.Kind != KindScript || firstNonEmpty(r.Hook, "incident") != "incident" {
			continue
		}
		sc, err := compileRule(r)
		if err != nil {
			faults = append(faults, ScriptFault{r.ID, r.Name, err.Error()})
			continue
		}
		for _, in := range ins {
			res, err := sc.Run(ctx, in, r.timeout())
			if err != nil {
				faults = append(faults, ScriptFault{r.ID, r.Name, fmt.Sprintf("%s: %v", in.Number, err)})
				break // one failure is enough to say; the rest of this script waits for the next run
			}
			for _, v := range listOf(res.Value) {
				m, ok := v.(map[string]any)
				if !ok {
					continue
				}
				title := str(m["title"])
				if title == "" {
					continue
				}
				h := sha256.Sum256([]byte(title))
				sg := Suggestion{ID: fmt.Sprintf("%s:%s:%s", r.ID, in.Number, hex.EncodeToString(h[:4])), Rule: r.ID, RuleName: r.Name, Severity: r.Severity, Verdict: "suggest",
					Title: title, Detail: str(m["detail"]), Explain: firstNonEmpty(r.Explain, "The agency's script "+r.Name+"."), Incident: in.Number,
					Relied: []Evidence{{Text: "script " + r.Name + " on " + in.Number, Source: SrcMachine, Incident: in.Number}}, Absent: []string{}, Since: since[in.Number]}
				if sv := str(m["severity"]); sv == "urgent" || sv == "info" {
					sg.Severity = sv
				}
				if a, ok := m["action"].(map[string]any); ok && str(a["command"]) != "" {
					sg.Action = &Action{Label: firstNonEmpty(str(a["label"]), "Accept"), Command: str(a["command"])}
					// The command runs on this incident unless it names one.
					if !strings.Contains(sg.Action.Command, "#") && !strings.Contains(sg.Action.Command, in.Number) {
						sg.Action.Command += " #" + in.Number
					}
				}
				out = append(out, sg)
			}
		}
	}
	return out, faults
}

func listOf(v any) []any {
	switch x := v.(type) {
	case nil:
		return nil
	case []any:
		return x
	}
	return []any{v}
}

func str(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// RecommendInput is what a recommendation script sees.
type RecommendInput struct {
	Incident ScriptInput     `json:"incident"`
	Lines    []RecommendLine `json:"lines"`
	Units    []string        `json:"units"` // what is recommended now
}

// RecommendLine is one run card line and its candidates.
type RecommendLine struct {
	Capability string           `json:"capability"`
	Count      int              `json:"count"`
	Candidates []map[string]any `json:"candidates"` // unit, type, seconds, metres
}

// RecommendAdvice is what the recommendation scripts said.
type RecommendAdvice struct {
	Prefer []string      `json:"prefer"`
	Notes  []string      `json:"notes"`
	Faults []ScriptFault `json:"faults"`
}

// AdviseRecommendation runs the live recommendation scripts: they may prefer units and add notes.
func (s *Service) AdviseRecommendation(ctx context.Context, in RecommendInput) RecommendAdvice {
	var out RecommendAdvice
	c, mode, _ := s.Codebook()
	if mode != "live" {
		return out
	}
	for _, r := range c.Rules {
		if !r.Enabled || r.Kind != KindScript || r.Hook != "recommend" {
			continue
		}
		sc, err := compileRule(r)
		if err == nil {
			var res script.Result
			if res, err = sc.Run(ctx, in, r.timeout()); err == nil {
				if m, ok := res.Value.(map[string]any); ok {
					for _, u := range listOf(m["prefer"]) {
						if x := str(u); x != "" {
							out.Prefer = append(out.Prefer, strings.ToUpper(x))
						}
					}
					for _, n := range listOf(m["notes"]) {
						if x := str(n); x != "" {
							out.Notes = append(out.Notes, r.Name+": "+x)
						}
					}
				}
				continue
			}
		}
		out.Faults = append(out.Faults, ScriptFault{r.ID, r.Name, err.Error()})
	}
	return out
}

// CheckReport runs the live report scripts: the agency's own checks, shown beside the state's.
func (s *Service) CheckReport(ctx context.Context, r cad.RMSReport) ([]cad.RMSCheck, []ScriptFault) {
	var out []cad.RMSCheck
	var faults []ScriptFault
	c, mode, _ := s.Codebook()
	if mode != "live" {
		return nil, nil
	}
	for _, rule := range c.Rules {
		if !rule.Enabled || rule.Kind != KindScript || rule.Hook != "report" {
			continue
		}
		sc, err := compileRule(rule)
		if err == nil {
			var res script.Result
			if res, err = sc.Run(ctx, r, rule.timeout()); err == nil {
				for _, v := range listOf(res.Value) {
					if m, ok := v.(map[string]any); ok && str(m["message"]) != "" {
						out = append(out, cad.RMSCheck{Field: firstNonEmpty(str(m["field"]), "agency"), Message: str(m["message"])})
					}
				}
				continue
			}
		}
		faults = append(faults, ScriptFault{rule.ID, rule.Name, err.Error()})
	}
	return out, faults
}

// TryScript runs a script once on sample input, for the editor's Test button.
func TryScript(ctx context.Context, r Rule, input any) (script.Result, error) {
	sc, err := compileRule(r)
	if err != nil {
		return script.Result{}, err
	}
	return sc.Run(ctx, input, r.timeout())
}

// IncidentInput is the plain copy of an incident a script sees: call it holding the state.
func IncidentInput(s *cad.State, in *cad.Incident, now time.Time) ScriptInput {
	x := ScriptInput{Number: in.Number, Type: in.Type, TypeName: typeName(s, in.Type), Priority: in.Priority, Status: in.Status, Minutes: now.Sub(in.Created).Minutes(),
		Notes: []string{}, Units: []string{}, Hazards: []string{}, Calls: len(in.Calls)}
	if in.Location != nil {
		x.Address = in.Location.Address
		if in.Location.Lat != nil && in.Location.Lon != nil {
			x.Lat, x.Lon = *in.Location.Lat, *in.Location.Lon
			for _, p := range s.PremisesNear(x.Lat, x.Lon) {
				x.Hazards = append(x.Hazards, p.Hazards...)
			}
		}
	}
	if in.Caller != nil {
		x.Caller = in.Caller.Name
	}
	for _, e := range in.Timeline {
		if e.Kind == "note" || e.Kind == "priority-note" {
			x.Notes = append(x.Notes, e.Text)
		}
	}
	for _, a := range in.Units {
		if a.Cleared.IsZero() {
			x.Units = append(x.Units, a.Unit)
		}
	}
	return x
}

// HasScripts reports whether a live script rule uses the hook.
func (s *Service) HasScripts(hook string) bool {
	c, mode, _ := s.Codebook()
	if mode != "live" {
		return false
	}
	for _, r := range c.Rules {
		if r.Enabled && r.Kind == KindScript && r.Hook == hook {
			return true
		}
	}
	return false
}
