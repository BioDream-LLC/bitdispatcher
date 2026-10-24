package cad

// Saved duty presets and supervisory views. Each person keeps a workspace preset per duty, stored here
// with their account (so it follows them to any console), not in one browser. A preset is locked in
// normal work: it changes only through Edit preset, Save changes and Lock, and each save is a ledger event
// with who saved it and when. An administrator assigns the agency's preset for a duty (what a trainee
// works, and what anyone without their own starts from) and may protect a person's preset so only
// administrators change it. Workstations remember their monitors separately (Workstation.Monitors), so a
// preset is placed on whatever screens the console has, and a console with fewer or other screens falls
// back without the stored preset being overwritten.
//
// Supervisory views (the supervisor board, operations and quality reports, the ledger, staffing coverage)
// belong to the agency: it decides who may see each one. A person without the right neither sees the
// view nor reaches it by a direct link: the server refuses its requests.

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvPresetSaved   = "preset.saved"   // a person's (or the agency's assigned) preset for a duty was saved
	EvPresetCleared = "preset.cleared" // a person's preset was removed: they start from the assigned one
	EvAccessRefused = "access.refused" // a person asked for a view or record they may not see
	// Searches and what was opened from them (search.go in the API), and command lines refused.
	EvSearched       = "search.run"
	EvSearchOpened   = "search.opened"
	EvCommandRefused = "command.refused"
)

// AssignedUser is the owner of the agency's assigned presets.
const AssignedUser = "@assigned"

// MonitorInfo is one of a workstation's screens.
type MonitorInfo struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	Left   int `json:"left,omitempty"`
	Top    int `json:"top,omitempty"`
}

// Preset is a saved workspace for a person and a duty: the console's sections and how they share the
// screen (Workspace), the screens it opens on other monitors (Screens), and settings that go with the duty
// (how many working calls the call-taker keeps, the key profile).
type Preset struct {
	User      string          `json:"user"`
	Duty      string          `json:"duty"`
	Workspace json.RawMessage `json:"workspace,omitempty"`
	Screens   json.RawMessage `json:"screens,omitempty"`
	// Monitors is the monitor arrangement the preset was saved on; at a console with others the screens
	// fall back to what fits.
	Monitors []MonitorInfo `json:"monitors,omitempty"`
	// Protected presets are changed only by administrators.
	Protected bool      `json:"protected,omitempty"`
	Version   int       `json:"version"`
	Updated   time.Time `json:"updated"`
	By        string    `json:"by"`
	Console   string    `json:"console,omitempty"`
	Reason    string    `json:"reason,omitempty"`
}

func presetKey(user, duty string) string { return strings.ToLower(user) + "|" + duty }

// PresetOf is the preset a person works for a duty, and where it came from: their own ("own"), the
// agency's assigned one ("assigned", always for a trainee), or none.
func (s *State) PresetOf(user, duty string) (*Preset, string) {
	assigned := s.Presets[presetKey(AssignedUser, duty)]
	if s.StaffOf(user).Trainee {
		if assigned != nil {
			return assigned, "assigned"
		}
		return nil, "assigned"
	}
	if p := s.Presets[presetKey(user, duty)]; p != nil {
		return p, "own"
	}
	if assigned != nil {
		return assigned, "assigned"
	}
	return nil, ""
}

// MayEditPreset says whether a person (working with role) may change a person's preset for a duty, and
// why not.
func (s *State) MayEditPreset(editor, role, owner, duty string) (bool, string) {
	if role == "admin" {
		return true, ""
	}
	if owner == AssignedUser {
		return false, "only administrators set the agency's assigned presets"
	}
	if !strings.EqualFold(editor, owner) {
		return false, "a person changes only their own presets"
	}
	if s.StaffOf(owner).Trainee {
		return false, "a trainee works the agency's assigned preset"
	}
	if p := s.Presets[presetKey(owner, duty)]; p != nil && p.Protected {
		return false, "this preset is protected: an administrator changes it"
	}
	return true, ""
}

// SavePreset saves a preset (the person's own, or with owner AssignedUser the agency's for the duty).
func (e *Engine) SavePreset(a Actor, p Preset) error {
	p.Duty = strings.ToLower(strings.TrimSpace(p.Duty))
	if p.Duty == "" {
		return problem("duty", "say which duty the preset is for")
	}
	if p.User == "" {
		p.User = a.User
	}
	p.User = strings.ToLower(p.User)
	if len(p.Workspace) > 64<<10 || len(p.Screens) > 64<<10 {
		return problem("workspace", "that preset is too large")
	}
	for _, raw := range []json.RawMessage{p.Workspace, p.Screens} {
		if len(raw) > 0 && !json.Valid(raw) {
			return problem("workspace", "that is not a workspace")
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if ok, why := e.st.MayEditPreset(a.User, a.Role, p.User, p.Duty); !ok {
		return problem("preset", "%s", why)
	}
	old := e.st.Presets[presetKey(p.User, p.Duty)]
	if old != nil {
		p.Version = old.Version + 1
		if a.Role != "admin" {
			p.Protected = old.Protected
		}
	} else {
		p.Version = 1
		if a.Role != "admin" {
			p.Protected = false
		}
	}
	p.Console = a.Console
	_, err := e.write(a, EvPresetSaved, refs{}, p)
	return err
}

// ClearPreset removes a person's own preset for a duty: they start from the assigned one again.
func (e *Engine) ClearPreset(a Actor, user, duty string) error {
	user = strings.ToLower(user)
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.Presets[presetKey(user, duty)] == nil {
		return problem("preset", "no saved preset")
	}
	if ok, why := e.st.MayEditPreset(a.User, a.Role, user, duty); !ok {
		return problem("preset", "%s", why)
	}
	_, err := e.write(a, EvPresetCleared, refs{}, map[string]string{"user": user, "duty": duty})
	return err
}

// PresetsOf are a person's saved presets, by duty.
func (s *State) PresetsOf(user string) []*Preset {
	out := []*Preset{}
	for _, p := range s.Presets {
		if strings.EqualFold(p.User, user) {
			out = append(out, p)
		}
	}
	slices.SortFunc(out, func(a, b *Preset) int { return strings.Compare(a.Duty, b.Duty) })
	return out
}

func (s *State) applyPresets(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvPresetSaved:
		p, err := decode[Preset](e)
		if err != nil {
			return true, err
		}
		p.Updated, p.By = e.At, e.Actor
		s.Presets[presetKey(p.User, p.Duty)] = &p
	case EvPresetCleared:
		v, err := decode[map[string]string](e)
		if err != nil {
			return true, err
		}
		delete(s.Presets, presetKey(v["user"], v["duty"]))
	default:
		return false, nil
	}
	return true, nil
}

// ---- supervisory views ----

// ViewDef is one of the supervisory views the agency controls: its screen and the requests it makes.
type ViewDef struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Route string   `json:"route"`
	API   []string `json:"-"` // request paths (prefixes) that belong to the view
}

// SupervisoryViews are the views an agency controls.
var SupervisoryViews = []ViewDef{
	{ID: "board", Name: "Supervisor board", Route: "/board", API: []string{"/api/board"}},
	{ID: "operations", Name: "Operations report", Route: "/admin/reports", API: []string{"/api/reports/operations", "/api/reports/certify"}},
	{ID: "quality", Name: "Quality review and care", Route: "/admin/quality", API: []string{"/api/reports/quality", "/api/care/"}},
	{ID: "ledger", Name: "Ledger", Route: "/admin/ledger", API: []string{"/api/ledger"}},
	{ID: "coverage", Name: "Staffing and coverage", Route: "/staffing", API: []string{"/api/staffing/coverage"}},
}

// ViewRule is who may see a supervisory view. With no rule, supervisors and administrators may.
type ViewRule struct {
	ID    string   `json:"id"`
	Off   bool     `json:"off,omitempty"`   // nobody but administrators sees it
	Roles []string `json:"roles,omitempty"` // supervisor, dispatcher, viewer (administrators always may)
	Users []string `json:"users,omitempty"` // named people, besides the roles
	// Agencies limit it to people who may work one of these agencies (empty: any).
	Agencies []string `json:"agencies,omitempty"`
}

// MaySeeView reports whether a person working with role may see a supervisory view.
func (s *State) MaySeeView(user, role, id string) bool {
	if role == "admin" {
		return true
	}
	var rule *ViewRule
	for i := range s.Settings.Views {
		if s.Settings.Views[i].ID == id {
			rule = &s.Settings.Views[i]
		}
	}
	if rule == nil {
		return role == "supervisor"
	}
	if rule.Off {
		return false
	}
	if !slices.Contains(rule.Roles, role) && !slices.ContainsFunc(rule.Users, func(u string) bool { return strings.EqualFold(u, user) }) {
		return false
	}
	if len(rule.Agencies) > 0 && !s.AllAgencies(user) && !slices.ContainsFunc(rule.Agencies, func(g string) bool { return s.MayWork(user, g) }) {
		return false
	}
	return true
}

// ViewForPath is the supervisory view a request path belongs to, if any.
func ViewForPath(path string) string {
	for _, v := range SupervisoryViews {
		for _, p := range v.API {
			if path == p || strings.HasPrefix(path, p) && (strings.HasSuffix(p, "/") || strings.HasPrefix(path[len(p):], "/") || strings.HasPrefix(path[len(p):], "?")) {
				return v.ID
			}
		}
	}
	return ""
}
