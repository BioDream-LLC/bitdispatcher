package cad

// Unified search: one permission-aware search across incidents (number, type, address, narrative,
// comments, timeline), communications (BOLOs, messages, notices: subject, body, every version), people
// (names, aliases, birth dates, phones, addresses, identifiers, descriptions), vehicles (plate, partial
// plate, VIN, year, make, model, colour), attachments (file names and captions) and units (call sign,
// radio ID, the people who staffed them). Each result says why it matched (same plate, partial VIN,
// shared phone, similar description) and how strongly (exact, partial, similar), and whether it is the
// record itself (confirmed) or only a possible match. A search never links anything to anything.
//
// What a person may not see is filtered out before anything is counted or shown: incidents of agencies
// they neither cover nor watch (unless they may search across agencies), communications not for them,
// juvenile names and medical notes for people without the right. Every search and every record opened from
// one is recorded in the ledger.

import (
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// FindQuery is what is searched for.
type FindQuery struct {
	Text   string    `json:"q"`
	Types  []string  `json:"types,omitempty"` // incidents, comms, people, vehicles, attachments, units
	Agency string    `json:"agency,omitempty"`
	From   time.Time `json:"from,omitzero"`
	To     time.Time `json:"to,omitzero"`
	Status string    `json:"status,omitempty"` // open, closed, active, expired, cancelled
	// AllAgencies searches beyond the person's covered and watched agencies, where they may.
	AllAgencies bool `json:"allAgencies,omitempty"`
	Fuzzy       bool `json:"fuzzy,omitempty"` // include similar spellings (default on when the text is a name)
	// FileText gives the words inside an incident's file (incident set) or a communication's attachment,
	// read when it was stored (the api's textract); nil: names and captions only.
	FileText func(incident, id string) string `json:"-"`
}

// Found is one result.
type Found struct {
	Group     string    `json:"group"` // incidents, comms, people, vehicles, attachments, units
	Ref       string    `json:"ref"`   // what to open: an incident number, a communication ID, a unit
	Incident  string    `json:"incident,omitempty"`
	Title     string    `json:"title"`
	Detail    string    `json:"detail,omitempty"`
	Agency    string    `json:"agency,omitempty"`
	Status    string    `json:"status,omitempty"`
	At        time.Time `json:"at,omitzero"`
	Field     string    `json:"field"`    // what matched: plate, VIN, phone, name, description…
	Strength  string    `json:"strength"` // exact, partial, similar
	Why       string    `json:"why"`
	Confirmed bool      `json:"confirmed"` // the record itself, not a possible match
	Limited   bool      `json:"limited,omitempty"`
}

// FindResults are the results, grouped.
type FindResults struct {
	Query  FindQuery          `json:"query"`
	Groups map[string][]Found `json:"groups"`
	Counts map[string]int     `json:"counts"`
	Scope  []string           `json:"scope"` // the agencies searched (empty: every agency)
}

// SearchGroups in the order they are shown.
var SearchGroups = []string{"incidents", "comms", "people", "vehicles", "attachments", "units"}

var reWS = regexp.MustCompile(`\s+`)

// lev is the edit distance of two short strings.
func lev(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			c := 1
			if ra[i-1] == rb[j-1] {
				c = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+c)
		}
		prev = cur
	}
	return prev[len(rb)]
}

// matcher compares the search text with a field.
type matcher struct {
	q      string   // upper case, single spaces
	words  []string // its words
	digits string
	alnum  string
	fuzzy  bool
}

func newMatcher(text string, fuzzy bool) matcher {
	q := strings.ToUpper(reWS.ReplaceAllString(strings.TrimSpace(text), " "))
	m := matcher{q: q, words: strings.Fields(norm(q)), digits: digits(q), fuzzy: fuzzy}
	m.alnum = strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, q)
	return m
}

// text matches free text: exact (the whole field), partial (every word of the search in it), or similar
// (every word within one or two letters of a word in it).
func (m matcher) text(field string) string {
	if field == "" || len(m.words) == 0 {
		return ""
	}
	f := strings.ToUpper(field)
	if norm(f) == norm(m.q) {
		return "exact"
	}
	fw := strings.Fields(norm(f))
	all := true
	for _, w := range m.words {
		if !strings.Contains(norm(f), w) {
			all = false
			break
		}
	}
	if all {
		return "partial"
	}
	if !m.fuzzy {
		return ""
	}
	for _, w := range m.words {
		ok := false
		for _, x := range fw {
			d := lev(w, x)
			if len(w) >= 3 && d <= 1 || len(w) >= 8 && d <= 2 {
				ok = true
				break
			}
		}
		if !ok {
			return ""
		}
	}
	return "similar"
}

// ident matches an identifier (plate, VIN, ID number): exact, or partial (the search is part of it, with
// ? for an unknown character).
func (m matcher) ident(field string) string {
	if field == "" || len(m.alnum) < 3 {
		return ""
	}
	f := strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		if r >= 'a' && r <= 'z' {
			return r - 32
		}
		return -1
	}, field)
	if f == m.alnum {
		return "exact"
	}
	if strings.Contains(m.q, "?") || strings.Contains(m.q, "*") {
		pat := "^" + strings.NewReplacer("?", "[A-Z0-9]", "*", "[A-Z0-9]*").Replace(regexp.QuoteMeta(strings.ReplaceAll(strings.ReplaceAll(m.q, " ", ""), "-", "")))
		pat = strings.ReplaceAll(strings.ReplaceAll(pat, `\?`, "[A-Z0-9]"), `\*`, "[A-Z0-9]*")
		if re, err := regexp.Compile(pat); err == nil && re.MatchString(f) {
			return "partial"
		}
		return ""
	}
	if len(m.alnum) >= 3 && strings.Contains(f, m.alnum) {
		return "partial"
	}
	return ""
}

// phone matches phone numbers by their last ten digits.
func (m matcher) phone(field string) string {
	d := digits(field)
	if len(m.digits) < 7 || len(d) < 7 {
		return ""
	}
	tail := func(s string) string { return s[max(0, len(s)-10):] }
	if tail(d) == tail(m.digits) {
		return "exact"
	}
	if strings.Contains(d, m.digits) {
		return "partial"
	}
	return ""
}

// name matches people's names in any order ("DOE, JOHN" and "John Doe"), or similar spellings.
func (m matcher) name(field string) string {
	if field == "" {
		return ""
	}
	if nameKey(field) == nameKey(m.q) {
		return "exact"
	}
	return m.text(field)
}

// Find searches for a reader.
func (s *State) Find(q FindQuery, r Reader, now time.Time) FindResults {
	res := FindResults{Query: q, Groups: map[string][]Found{}, Counts: map[string]int{}, Scope: []string{}}
	if len(strings.TrimSpace(q.Text)) < 2 {
		return res
	}
	want := func(g string) bool { return len(q.Types) == 0 || slices.Contains(q.Types, g) }
	m := newMatcher(q.Text, q.Fuzzy || len(digits(q.Text)) == 0)
	// The agencies searched.
	all := len(s.Agencies) == 0 || q.AllAgencies && s.AllAgencies(r.User) || s.AllAgencies(r.User) && len(r.Agencies) == 0
	scope := map[string]bool{}
	if !all {
		for _, a := range r.Agencies {
			scope[a] = true
		}
		if q.AllAgencies {
			for code := range s.Agencies {
				if s.MayWork(r.User, code) {
					scope[code] = true
				}
			}
		}
		for a := range scope {
			res.Scope = append(res.Scope, a)
		}
		sort.Strings(res.Scope)
	}
	inScope := func(agency string) bool {
		if q.Agency != "" && agency != q.Agency {
			return false
		}
		return all || agency == "" || scope[agency]
	}
	inTime := func(t time.Time) bool {
		return (q.From.IsZero() || !t.Before(q.From)) && (q.To.IsZero() || !t.After(q.To))
	}
	medical := r.Duty == "ems" || r.Duty == "dispatcher" || r.Duty == "records" || r.Role == "supervisor" || r.Role == "admin"
	juvenileOK := r.Duty == "records" || r.Role == "supervisor" || r.Role == "admin"
	add := func(f Found) {
		res.Groups[f.Group] = append(res.Groups[f.Group], f)
	}
	for _, in := range s.Incidents {
		if !inScope(in.Agency) || !inTime(in.Created) {
			continue
		}
		st := "open"
		if in.Status == Closed {
			st = "closed"
		}
		if q.Status != "" && q.Status != st && !(q.Status == "active" && st == "open") {
			continue
		}
		addr := ""
		if in.Location != nil {
			addr = in.Location.Address
		}
		title := in.Number + " " + s.typeLabel(in.Type)
		juvenile := slices.Contains(in.Modifiers, "JUVENILE")
		ems := false
		if ct := s.CallTypes[in.Type]; ct != nil && ct.Discipline == "ems" {
			ems = true
		}
		base := Found{Ref: in.Number, Incident: in.Number, Title: title, Detail: addr, Agency: in.Agency, Status: st, At: in.Created}
		if want("incidents") {
			hit := func(field, how, why string) {
				f := base
				f.Group, f.Field, f.Strength, f.Why, f.Confirmed = "incidents", field, how, why, how == "exact"
				add(f)
			}
			switch {
			case strings.EqualFold(in.Number, strings.TrimPrefix(m.q, "#")):
				hit("number", "exact", "incident number")
			case m.text(addr) != "":
				hit("address", m.text(addr), "same address")
			case m.text(s.typeLabel(in.Type)) == "exact" || strings.EqualFold(in.Type, m.q):
				hit("type", "exact", "call type")
			case in.Disposition != "" && strings.EqualFold(in.Disposition, m.q):
				hit("disposition", "exact", "disposition")
			default:
				for _, e := range in.Timeline {
					if how := m.text(e.Text); how == "exact" || how == "partial" {
						f := base
						f.Group, f.Field, f.Strength, f.Why = "incidents", "comment", how, "in the narrative or a comment"
						if ems && !medical {
							f.Limited = true
						} else {
							f.Detail = clip(e.Text, 160)
						}
						add(f)
						break
					}
				}
			}
		}
		if want("people") {
			for _, p := range in.Persons {
				limited := juvenile && p.Role != "caller" && !juvenileOK
				try := func(field, how, why string) bool {
					if how == "" {
						return false
					}
					f := base
					f.Group, f.Field, f.Strength, f.Why = "people", field, how, why
					f.Title = p.Name + " (" + p.Role + ")"
					f.Detail = in.Number + " " + addr
					f.Confirmed = how == "exact" && field != "description"
					if limited {
						f.Title, f.Limited = "A juvenile ("+p.Role+")", true
					}
					add(f)
					return true
				}
				_ = try("name", m.name(p.Name), "same name") ||
					slices.ContainsFunc(p.Aliases, func(a string) bool { return try("alias", m.name(a), "same alias") }) ||
					try("phone", m.phone(p.Phone), "shared phone") ||
					try("dob", exactIf(p.DOB != "" && strings.EqualFold(p.DOB, m.q)), "same date of birth") ||
					try("id", m.ident(p.IDNumber), "same identifier") ||
					try("address", m.text(p.Address), "same address") ||
					try("description", partialOnly(m.text(p.Clothing+" "+p.Hair+" "+p.Notes)), "similar description")
			}
			if in.Caller != nil {
				f := base
				if how := m.phone(in.Caller.Callback); how != "" {
					f.Group, f.Field, f.Strength, f.Why, f.Title, f.Confirmed = "people", "phone", how, "the caller's callback number", firstNonEmpty(in.Caller.Name, "Caller"), how == "exact"
					add(f)
				} else if how := m.name(in.Caller.Name); how != "" {
					f.Group, f.Field, f.Strength, f.Why, f.Title, f.Confirmed = "people", "name", how, "the caller", in.Caller.Name, how == "exact"
					add(f)
				}
			}
		}
		if want("vehicles") {
			for _, v := range in.Vehicles {
				f := base
				f.Group, f.Title = "vehicles", strings.TrimSpace(v.Color+" "+v.Year+" "+v.Make+" "+v.Model+" "+v.Plate+" "+v.State)
				f.Detail = in.Number + " " + addr
				switch {
				case m.ident(v.Plate) != "":
					f.Field, f.Strength = "plate", m.ident(v.Plate)
					f.Why = map[string]string{"exact": "same plate", "partial": "partial plate"}[f.Strength]
				case m.ident(v.VIN) != "":
					f.Field, f.Strength = "vin", m.ident(v.VIN)
					f.Why = map[string]string{"exact": "same VIN", "partial": "partial VIN"}[f.Strength]
				case m.text(v.Year+" "+v.Make+" "+v.Model+" "+v.Color+" "+v.Body+" "+v.Description) != "":
					f.Field, f.Strength, f.Why = "description", m.text(v.Year+" "+v.Make+" "+v.Model+" "+v.Color+" "+v.Body+" "+v.Description), "similar vehicle"
				default:
					continue
				}
				f.Confirmed = f.Strength == "exact" && f.Field != "description"
				add(f)
			}
		}
		if want("attachments") {
			for _, x := range in.Files {
				if x.Hidden {
					continue
				}
				if how := m.text(x.Name + " " + x.Caption); how != "" {
					f := base
					f.Group, f.Field, f.Strength, f.Why, f.Title, f.Detail = "attachments", "file", how, "file name or caption", x.Name, in.Number+" "+x.Caption
					add(f)
				} else if txt := fileText(q, in.Number, x.ID); txt != "" {
					if how := partialOnly(m.inText(txt)); how != "" {
						f := base
						f.Group, f.Field, f.Strength, f.Why, f.Title, f.Detail = "attachments", "content", how, "in the file's text", x.Name, in.Number+": …"+snippet(txt, m.words)+"…"
						add(f)
					}
				}
			}
		}
	}
	// Communications: every version the reader may see, labelled with its state.
	for _, c := range s.Comms {
		if !inScope(c.Agency) || !inTime(c.Issued) || !s.MaySeeComm(c, r) {
			continue
		}
		st := c.State(now)
		if q.Status != "" && q.Status != st {
			continue
		}
		k, _ := s.Settings.commKind(c.Kind)
		base := Found{Ref: c.ID, Incident: c.Incident, Title: firstNonEmpty(k.Name, c.Kind) + " " + c.Thread + ": " + c.Subject, Agency: c.Agency, Status: st, At: c.Issued}
		if c.Version > 1 {
			base.Title += " (version " + itoa(c.Version) + ")"
		}
		if want("comms") {
			for _, t := range []struct{ field, val, why string }{{"subject", c.Subject, "in the subject"}, {"body", c.Body, "in the text"}, {"incident", c.Incident, "about the incident"}} {
				how := m.text(t.val)
				if t.field == "incident" {
					how = exactIf(c.Incident != "" && strings.EqualFold(c.Incident, strings.TrimPrefix(m.q, "#")))
				}
				if how != "" {
					f := base
					f.Group, f.Field, f.Strength, f.Why, f.Confirmed, f.Detail = "comms", t.field, how, t.why, how == "exact", clip(c.Body, 160)
					add(f)
					break
				}
			}
		}
		if want("vehicles") && (c.Plate != "" || c.Vehicle != "") {
			f := base
			f.Group, f.Detail = "vehicles", strings.TrimSpace(c.Vehicle+" "+c.Plate+" "+c.PlateState)
			if how := m.ident(c.Plate); how != "" {
				f.Field, f.Strength, f.Why = "plate", how, map[string]string{"exact": "same plate in a BOLO", "partial": "partial plate in a BOLO"}[how]
				f.Confirmed = how == "exact"
				add(f)
			} else if how := partialOnly(m.text(c.Vehicle)); how != "" {
				f.Field, f.Strength, f.Why = "description", how, "similar vehicle in a BOLO"
				add(f)
			}
		}
		if want("people") && (c.Person != "" || c.Description != "") {
			if how := m.text(c.Person + " " + c.Description); how != "" {
				f := base
				f.Group, f.Field, f.Strength, f.Why, f.Detail = "people", "description", partialOnly(how), "similar person described in a communication", clip(c.Person+" "+c.Description, 160)
				add(f)
			}
		}
		if want("attachments") {
			for _, id := range c.Files {
				if x := s.CommFiles[id]; x != nil && !x.Removed {
					if how := m.text(x.Name); how != "" {
						f := base
						f.Group, f.Field, f.Strength, f.Why, f.Title = "attachments", "file", how, "an attachment's name", x.Name
						add(f)
					} else if txt := fileText(q, "", x.ID); txt != "" {
						if how := partialOnly(m.inText(txt)); how != "" {
							f := base
							f.Group, f.Field, f.Strength, f.Why, f.Title, f.Detail = "attachments", "content", how, "in the attachment's text", x.Name, "…"+snippet(txt, m.words)+"…"
							add(f)
						}
					}
				}
			}
		}
	}
	// Units, and the people who staff them now.
	if want("units") {
		for _, u := range s.Units {
			if !inScope(u.Agency) {
				continue
			}
			f := Found{Group: "units", Ref: u.ID, Title: u.ID + " " + StatusLabel(u.Status), Agency: u.Agency, Status: u.Status, Incident: u.Incident}
			switch {
			case strings.EqualFold(u.ID, m.q):
				f.Field, f.Strength, f.Why = "call sign", "exact", "call sign"
			case u.RadioID != 0 && itoa(u.RadioID) == m.q:
				f.Field, f.Strength, f.Why = "radio", "exact", "radio ID"
			case u.RadioAlias != "" && m.text(u.RadioAlias) != "":
				f.Field, f.Strength, f.Why = "radio", m.text(u.RadioAlias), "radio alias"
			case u.Station != "" && strings.EqualFold(u.Station, m.q) || u.Area != "" && strings.EqualFold(u.Area, m.q):
				f.Field, f.Strength, f.Why = "station", "exact", "its station or beat"
			default:
				found := false
				for _, c := range u.Crew {
					if how := m.name(c.Name); how != "" || strings.EqualFold(c.Member, m.q) {
						f.Field, f.Strength, f.Why, found = "crew", firstNonEmpty(how, "exact"), c.Name+" is aboard", true
						break
					}
				}
				if !found {
					continue
				}
			}
			f.Confirmed = f.Strength == "exact"
			add(f)
		}
	}
	rank := map[string]int{"exact": 0, "partial": 1, "similar": 2}
	for g, xs := range res.Groups {
		sort.SliceStable(xs, func(i, j int) bool {
			if rank[xs[i].Strength] != rank[xs[j].Strength] {
				return rank[xs[i].Strength] < rank[xs[j].Strength]
			}
			return xs[i].At.After(xs[j].At)
		})
		res.Counts[g] = len(xs)
		if len(xs) > 50 {
			res.Groups[g] = xs[:50]
		}
	}
	return res
}

func exactIf(b bool) string {
	if b {
		return "exact"
	}
	return ""
}

// partialOnly: a description is never an exact match of a person or a vehicle.
func partialOnly(how string) string {
	if how == "exact" {
		return "partial"
	}
	return how
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func fileText(q FindQuery, incident, id string) string {
	if q.FileText == nil {
		return ""
	}
	return q.FileText(incident, id)
}

// inText matches the words in a long text (an attachment's): every word present is partial; similar
// spellings only when asked, word by word.
func (m matcher) inText(txt string) string {
	if len(m.words) == 0 {
		return ""
	}
	u := norm(strings.ToUpper(txt))
	all := true
	for _, w := range m.words {
		if !strings.Contains(u, w) {
			all = false
			break
		}
	}
	if all {
		return "partial"
	}
	if !m.fuzzy || len(txt) > 32<<10 {
		return ""
	}
	return m.text(txt)
}

// snippet is the text around the first word found, for the result's detail.
func snippet(txt string, words []string) string {
	u := strings.ToUpper(txt)
	at := -1
	for _, w := range words {
		if i := strings.Index(u, w); i >= 0 && (at < 0 || i < at) {
			at = i
		}
	}
	if at < 0 {
		return clip(txt, 120)
	}
	from, to := max(0, at-50), min(len(txt), at+90)
	for from > 0 && !utf8.RuneStart(txt[from]) {
		from--
	}
	for to < len(txt) && !utf8.RuneStart(txt[to]) {
		to++
	}
	return strings.TrimSpace(txt[from:to])
}
