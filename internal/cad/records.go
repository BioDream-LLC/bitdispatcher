package cad

// Records: the review views of an incident (as first entered, summary, detailed, full audit) and records
// requests (FOIA): the agency's disclosure rules, each request with its incidents, the proposed and decided
// redactions, the approval and the released copies. Every step is in the ledger.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/foia"
	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// Events of records requests.
const (
	EvFOIARuleSaved   = "foia.rule.saved"
	EvFOIARuleRemoved = "foia.rule.removed"
	EvFOIARequest     = "foia.request.saved"
	EvFOIAAssembled   = "foia.assembled"
	EvFOIADecided     = "foia.decided"
	EvFOIAReleased    = "foia.released"
)

// FOIARequest is one records request and its work.
type FOIARequest struct {
	ID          string     `json:"id"`
	Requester   string     `json:"requester"`
	Contact     string     `json:"contact,omitempty"`
	Received    time.Time  `json:"received"`
	Due         time.Time  `json:"due"`
	Description string     `json:"description,omitempty"`
	Incidents   []string   `json:"incidents"`
	View        string     `json:"view"`   // summary or detailed: what is released
	Status      string     `json:"status"` // received, in-review, awaiting-approval, released, denied, closed
	Assigned    string     `json:"assigned,omitempty"`
	Items       []FOIAItem `json:"items"`
	Notes       []Entry    `json:"notes,omitempty"`
	Released    time.Time  `json:"released,omitzero"`
	ApprovedBy  string     `json:"approvedBy,omitempty"`
	Started     time.Time  `json:"started,omitzero"` // first assembled: the time the work took counts from it
	Updated     time.Time  `json:"updated"`
}

// FOIAItem is one incident's record in a request: the text assembled for release, the proposed redactions,
// the reviewer's decisions, and once approved the released copy and its hash.
type FOIAItem struct {
	Incident  string          `json:"incident"`
	Text      string          `json:"text"`
	Proposals []foia.Proposal `json:"proposals"`
	Decisions []foia.Decision `json:"decisions"`
	Released  string          `json:"released,omitempty"`
	Hash      string          `json:"hash,omitempty"`
	Assembled time.Time       `json:"assembled"`
}

// WorkingDaysAfter is the date n working days (Monday to Friday) after t.
func WorkingDaysAfter(t time.Time, n int) time.Time {
	d := t
	for n > 0 {
		d = d.AddDate(0, 0, 1)
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			n--
		}
	}
	return d
}

func (s *State) initRecords() {
	if s.FOIARules == nil {
		s.FOIARules = map[string]*foia.Rule{}
		s.FOIA = map[string]*FOIARequest{}
	}
}

// SaveFOIARule adds or changes a disclosure rule.
func (e *Engine) SaveFOIARule(a Actor, r foia.Rule) error {
	r.ID = strings.TrimSpace(r.ID)
	if r.ID == "" || strings.TrimSpace(r.Name) == "" {
		return problem("id", "a rule needs an ID and a name")
	}
	if !slices.Contains([]string{foia.KindPattern, foia.KindWords, foia.KindCategory}, r.Kind) {
		return problem("kind", "a rule is a pattern, a word list or a category")
	}
	if r.Kind == foia.KindCategory && !slices.Contains(foia.Categories(), r.Category) {
		return problem("category", "no category %q", r.Category)
	}
	if strings.TrimSpace(r.Basis) == "" {
		return problem("basis", "give the legal basis your counsel relies on")
	}
	if _, errs := foia.Propose("", []foia.Rule{{ID: "t", Kind: r.Kind, Pattern: r.Pattern, Words: r.Words, Category: r.Category, Basis: "t", Enabled: true}}, foia.Context{}); len(errs) > 0 {
		return problem("pattern", "%v", errs[0])
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := e.write(a, EvFOIARuleSaved, refs{}, r)
	return err
}

// RemoveFOIARule removes a disclosure rule.
func (e *Engine) RemoveFOIARule(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.st.FOIARules[id]; !ok {
		return problem("rule", "no rule %s", id)
	}
	_, err := e.write(a, EvFOIARuleRemoved, refs{}, map[string]string{"id": id})
	return err
}

// SaveFOIARequest records a new request or changes one (requester, incidents, view, status, assignment).
func (e *Engine) SaveFOIARequest(a Actor, q FOIARequest, note string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.st.initRecords()
	now := e.now()
	if q.ID == "" {
		if strings.TrimSpace(q.Requester) == "" {
			return "", problem("requester", "who asked?")
		}
		q.ID = fmt.Sprintf("R%02d-%04d", now.Year()%100, len(e.st.FOIA)+1)
		if q.Received.IsZero() {
			q.Received = now
		}
		days := e.st.Settings.FOIADays
		if days <= 0 {
			days = 5
		}
		if q.Due.IsZero() {
			q.Due = WorkingDaysAfter(q.Received, days)
		}
		q.Status = "received"
		q.Items = []FOIAItem{}
	} else {
		old, ok := e.st.FOIA[q.ID]
		if !ok {
			return "", problem("request", "no request %s", q.ID)
		}
		merged := *old
		if q.Requester != "" {
			merged.Requester = q.Requester
		}
		if q.Contact != "" {
			merged.Contact = q.Contact
		}
		if q.Description != "" {
			merged.Description = q.Description
		}
		if q.Incidents != nil {
			merged.Incidents = q.Incidents
		}
		if q.View != "" {
			merged.View = q.View
		}
		if q.Status != "" {
			if q.Status == "released" {
				return "", problem("status", "a request is released by approving it")
			}
			merged.Status = q.Status
		}
		if q.Assigned != "" {
			merged.Assigned = q.Assigned
		}
		if !q.Due.IsZero() {
			merged.Due = q.Due
		}
		q = merged
	}
	if q.View == "" {
		q.View = "summary"
	}
	if q.View != "summary" && q.View != "detailed" {
		return "", problem("view", "release the summary or the detailed view")
	}
	for i, n := range q.Incidents {
		q.Incidents[i] = strings.TrimSpace(n)
		if _, ok := e.st.Incidents[q.Incidents[i]]; !ok {
			return "", problem("incidents", "no incident %s", n)
		}
	}
	if note = strings.TrimSpace(note); note != "" {
		q.Notes = append(q.Notes, Entry{At: now, Actor: a.User, Kind: "note", Text: note})
	}
	q.Updated = now
	_, err := e.write(a, EvFOIARequest, refs{}, q)
	return q.ID, err
}

// AssembleFOIA builds the disclosure candidate for each incident of a request from the chosen view, and
// proposes redactions by the agency's rules (and any extra proposals, from a language model). The
// untouched record stays in the ledger; the candidate is a copy.
func (e *Engine) AssembleFOIA(a Actor, id string, texts map[string]string, extra map[string][]foia.Proposal) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	q, ok := e.st.FOIA[id]
	if !ok {
		return problem("request", "no request %s", id)
	}
	if q.Status == "released" {
		return problem("status", "%s was released: start a new request", id)
	}
	rules := e.st.foiaRuleList()
	now := e.now()
	items := []FOIAItem{}
	for _, n := range q.Incidents {
		text := texts[n]
		names := []string{}
		if in, ok := e.st.Incidents[n]; ok {
			for _, p := range in.Persons {
				names = append(names, p.Name)
			}
			if in.Caller != nil && in.Caller.Name != "" {
				names = append(names, in.Caller.Name)
			}
		}
		props, _ := foia.Propose(text, rules, foia.Context{RecordType: "incident", At: now, Names: names})
		props = append(props, extra[n]...)
		if props == nil {
			props = []foia.Proposal{}
		}
		items = append(items, FOIAItem{Incident: n, Text: text, Proposals: props, Decisions: []foia.Decision{}, Assembled: now})
	}
	_, err := e.write(a, EvFOIAAssembled, refs{}, map[string]any{"id": id, "items": items})
	return err
}

// DecideFOIA records a reviewer's decisions for one incident of a request: each proposal accepted or
// rejected (with the basis, which the reviewer may change), and passages they marked themselves.
func (e *Engine) DecideFOIA(a Actor, id, number string, ds []foia.Decision) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	q, ok := e.st.FOIA[id]
	if !ok {
		return problem("request", "no request %s", id)
	}
	i := slices.IndexFunc(q.Items, func(x FOIAItem) bool { return x.Incident == number })
	if i < 0 {
		return problem("incident", "%s is not assembled in %s", number, id)
	}
	text := q.Items[i].Text
	for k := range ds {
		d := &ds[k]
		if d.Start < 0 || d.End > len(text) || d.Start >= d.End || text[d.Start:d.End] != d.Text {
			return problem("decisions", "a decision does not match the record's text (it may have changed): assemble again")
		}
		if d.Accept && strings.TrimSpace(d.Basis) == "" {
			return problem("basis", "every redaction needs its legal basis")
		}
		d.By, d.At = a.User, e.now()
	}
	_, err := e.write(a, EvFOIADecided, refs{Incident: number}, map[string]any{"id": id, "incident": number, "decisions": ds})
	return err
}

// ApproveFOIA releases a request: a person authorises it; every incident's accepted redactions are applied
// irreversibly, the copies are checked for leaks, and the copies and their hashes are kept.
func (e *Engine) ApproveFOIA(a Actor, id string) ([]foia.Leak, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	q, ok := e.st.FOIA[id]
	if !ok {
		return nil, problem("request", "no request %s", id)
	}
	if len(q.Items) == 0 {
		return nil, problem("items", "assemble the records first")
	}
	type released struct {
		Incident string `json:"incident"`
		Text     string `json:"text"`
		Hash     string `json:"hash"`
	}
	var out []released
	var leaks []foia.Leak
	for _, it := range q.Items {
		for _, p := range it.Proposals {
			if !slices.ContainsFunc(it.Decisions, func(d foia.Decision) bool { return d.ID == p.ID || d.Start == p.Start && d.End == p.End }) {
				return nil, problem("decisions", "%s: decide every proposed redaction first (%q is undecided)", it.Incident, p.Text)
			}
		}
		text, errs := foia.Apply(it.Text, it.Decisions)
		if len(errs) > 0 {
			return nil, problem("decisions", "%s: %v", it.Incident, errs[0])
		}
		text = fmt.Sprintf("Released record of incident %s (records request %s)\n\n%s", it.Incident, q.ID, text)
		meta := map[string]string{"title": "Incident " + it.Incident, "filename": q.ID + "-" + it.Incident + ".txt"}
		leaks = append(leaks, foia.Validate(text, meta, it.Decisions)...)
		sum := sha256.Sum256([]byte(text))
		out = append(out, released{Incident: it.Incident, Text: text, Hash: hex.EncodeToString(sum[:])})
	}
	if len(leaks) > 0 {
		return leaks, problem("leaks", "redacted text is still in the copy: %q", leaks[0].Text)
	}
	_, err := e.write(a, EvFOIAReleased, refs{}, map[string]any{"id": id, "released": out})
	return nil, err
}

func (s *State) foiaRuleList() []foia.Rule {
	ids := make([]string, 0, len(s.FOIARules))
	for id := range s.FOIARules {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]foia.Rule, 0, len(ids))
	for _, id := range ids {
		out = append(out, *s.FOIARules[id])
	}
	return out
}

// FOIARuleList is the agency's disclosure rules, by ID.
func (s *State) FOIARuleList() []foia.Rule { return s.foiaRuleList() }

func (s *State) applyRecords(e ledger.Event) (bool, error) {
	s.initRecords()
	switch e.Type {
	case EvFOIARuleSaved:
		v, err := decode[foia.Rule](e)
		if err != nil {
			return true, err
		}
		r := v
		s.FOIARules[r.ID] = &r
	case EvFOIARuleRemoved:
		v, _ := decode[map[string]string](e)
		delete(s.FOIARules, v["id"])
	case EvFOIARequest:
		v, err := decode[FOIARequest](e)
		if err != nil {
			return true, err
		}
		q := v
		s.FOIA[q.ID] = &q
	case EvFOIAAssembled:
		v, err := decode[struct {
			ID    string     `json:"id"`
			Items []FOIAItem `json:"items"`
		}](e)
		if err != nil {
			return true, err
		}
		if q, ok := s.FOIA[v.ID]; ok {
			q.Items = v.Items
			if q.Started.IsZero() {
				q.Started = e.At
			}
			if q.Status == "received" {
				q.Status = "in-review"
			}
			q.Updated = e.At
		}
	case EvFOIADecided:
		v, err := decode[struct {
			ID        string          `json:"id"`
			Incident  string          `json:"incident"`
			Decisions []foia.Decision `json:"decisions"`
		}](e)
		if err != nil {
			return true, err
		}
		if q, ok := s.FOIA[v.ID]; ok {
			for i := range q.Items {
				if q.Items[i].Incident == v.Incident {
					q.Items[i].Decisions = v.Decisions
				}
			}
			q.Updated = e.At
		}
	case EvFOIAReleased:
		var v struct {
			ID       string `json:"id"`
			Released []struct {
				Incident string `json:"incident"`
				Text     string `json:"text"`
				Hash     string `json:"hash"`
			} `json:"released"`
		}
		if err := json.Unmarshal(e.Data, &v); err != nil {
			return true, err
		}
		if q, ok := s.FOIA[v.ID]; ok {
			for _, r := range v.Released {
				for i := range q.Items {
					if q.Items[i].Incident == r.Incident {
						q.Items[i].Released, q.Items[i].Hash = r.Text, r.Hash
					}
				}
			}
			q.Status, q.Released, q.ApprovedBy, q.Updated = "released", e.At, e.Actor, e.At
		}
	default:
		return false, nil
	}
	return true, nil
}
