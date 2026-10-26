package cad

// Critical incidents: the agency says which call types, modifiers, priorities, events and phrases make an
// incident critical. A trigger alerts the on-duty supervisor and opens one review case per incident (more
// triggers add to it). The case gets a line in the period's Shift Intelligence report; the full Critical
// Incident Review is assembled only when a supervisor asks for it (review.go's evidence, cut off at a
// time and versioned), with the agency's QA matrix for the call type. Phrases heard in call and radio
// transcripts make phrase alerts the supervisor disposes of; a negated phrase ("no shots fired") is shown
// for review, never treated as the real thing, and machine tags never become findings by themselves.

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
	"github.com/biodream-llc/bitdispatcher/internal/phrase"
)

const (
	EvCriticalFlag   = "critical.flag"   // a supervisor added or removed the critical designation, with why
	EvCriticalAck    = "critical.ack"    // a supervisor acknowledged a critical incident's alert
	EvCriticalQA     = "critical.qa"     // a reviewer answered a QA measure
	EvCriticalReport = "critical.report" // a full review was issued (a version with its evidence cutoff)
	EvCriticalStatus = "critical.status" // the review's state, findings, narrative or an action (support, after-action)
	EvPhraseDecided  = "phrase.disposed" // a supervisor disposed of a phrase alert
	EvRadioRequest   = "radio.request"   // (reserved)
)

// Review states.
const (
	ReviewPending  = "pending"
	ReviewOpen     = "open"
	ReviewComplete = "complete"
)

// CriticalRule is one way an incident becomes critical.
type CriticalRule struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`  // calltype, modifier, priority, event, phrase
	Match   string `json:"match"` // the call type code, modifier code, highest priority number, event (unit-emergency), or the phrase rule ID
	Version int    `json:"version,omitempty"`
	Off     bool   `json:"off,omitempty"`
}

// CriticalPolicy is the agency's critical-incident configuration.
type CriticalPolicy struct {
	Rules []CriticalRule `json:"rules,omitempty"`
	// Phrases are the alert phrases checked in call and radio transcripts and narrative (phrase rules).
	Phrases []phrase.Rule `json:"phrases,omitempty"`
	// Tags mark operational facts in radio traffic for the review: descriptions, weapons, direction of
	// travel, threats, use of force, CPR, hazards, requests, transports (phrase rules, never alerts).
	Tags []phrase.Rule `json:"tags,omitempty"`
	// QA are the QA templates, by call type.
	QA []QATemplate `json:"qa,omitempty"`
	// Dispositions of phrase alerts (empty: the defaults).
	PhraseDispositions []string `json:"phraseDispositions,omitempty"`
	// Version is the policy's version; a case keeps the version in force when it opened.
	Version int `json:"version,omitempty"`
}

// DefaultPhraseDispositions are what a supervisor decides about a phrase alert.
var DefaultPhraseDispositions = []string{"Reviewed, no action", "Critical incident opened", "Linked to existing review", "False positive", "Follow-up required"}

// PhraseDispositionList is the dispositions offered.
func (p CriticalPolicy) PhraseDispositionList() []string {
	if len(p.PhraseDispositions) == 0 {
		return DefaultPhraseDispositions
	}
	return p.PhraseDispositions
}

// TagList is the radio tag rules (empty: the defaults).
func (p CriticalPolicy) TagList() []phrase.Rule {
	if len(p.Tags) == 0 {
		return DefaultTags()
	}
	return p.Tags
}

// DefaultTags are the starter radio tags.
func DefaultTags() []phrase.Rule {
	t := func(id, name string, ph ...string) phrase.Rule {
		return phrase.Rule{ID: id, Name: name, Phrases: ph, Severity: "low", Negations: []string{"no", "not", "negative", "without"}}
	}
	return []phrase.Rule{
		t("weapon", "Weapon", "gun", "handgun", "rifle", "shotgun", "knife", "armed", "weapon"),
		t("direction", "Direction of travel", "northbound", "southbound", "eastbound", "westbound", "headed north", "headed south", "headed east", "headed west", "fled on foot"),
		t("description", "Description", "white male", "black male", "white female", "black female", "hispanic male", "hispanic female", "wearing", "hoodie", "jacket"),
		t("threat", "Threat to responders", "threatening officers", "threatened to shoot", "said he would shoot", "ambush"),
		t("force", "Use of force", "use of force", "taser deployed", "tased", "pepper spray", "o c spray", "k9 bite", "shots fired by officer"),
		t("cpr", "CPR", "cpr", "compressions", "aed", "not breathing"),
		t("hazard", "Hazard", "gas leak", "downed wires", "wires down", "hazmat", "smoke showing"),
		t("request", "Request", "run a plate", "run plate", "run the plate", "run this plate", "10 28", "10 29", "check for warrants", "run a name", "run him", "run her", "check his license", "check her license", "operator's license"),
		t("transport", "Transport", "transporting", "en route to jail", "en route to the jail", "prisoner transport", "en route to the hospital"),
		t("location", "Location change", "now at", "moving to", "relocating to", "staging at"),
	}
}

// QATemplate is the agency's QA matrix for a family of call types.
type QATemplate struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	CallTypes []string    `json:"callTypes"` // empty: any critical incident
	Measures  []QAMeasure `json:"measures"`
	Policy    string      `json:"policy,omitempty"` // the policy or standard and its version
	Version   int         `json:"version,omitempty"`
}

// QAMeasure is one measure of a QA template.
type QAMeasure struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Kind: interval (From and To events, against StandardSec), or check (a reviewer's judgement).
	Kind        string   `json:"kind"`
	From        string   `json:"from,omitempty"` // received, answered, located, typed, created, dispatched, enroute, onscene
	To          string   `json:"to,omitempty"`
	StandardSec int      `json:"standardSec,omitempty"`
	Scored      bool     `json:"scored,omitempty"`
	NAReasons   []string `json:"naReasons,omitempty"`
	Specialist  bool     `json:"specialist,omitempty"` // a QA specialist or supervisor must review it
	Expect      string   `json:"expect,omitempty"`     // a tag the incident is expected to carry (weapon, direction…)
}

// DefaultQA is a starter template.
func DefaultQA() QATemplate {
	return QATemplate{ID: "critical-default", Name: "Critical incident (starter)", Policy: "agency standard (edit in Admin › Critical incidents)", Version: 1, Measures: []QAMeasure{
		{ID: "answer", Name: "Call answered", Kind: "interval", From: "received", To: "answered", StandardSec: 15, Scored: true},
		{ID: "generate", Name: "Call answered to incident made", Kind: "interval", From: "answered", To: "created", StandardSec: 60, Scored: true},
		{ID: "dispatch", Name: "Call answered to first dispatch", Kind: "interval", From: "answered", To: "dispatched", StandardSec: 90, Scored: true},
		{ID: "location", Name: "Location verified or its exception explained", Kind: "check", Scored: true},
		{ID: "description", Name: "Suspect or vehicle description given to units", Kind: "check", Expect: "description", NAReasons: []string{"no suspect", "not a police call"}},
		{ID: "weapons", Name: "Weapon information relayed", Kind: "check", Expect: "weapon", NAReasons: []string{"no weapon reported"}},
		{ID: "requests", Name: "Responder requests acknowledged and processed", Kind: "check", Expect: "request"},
		{ID: "safety", Name: "Officer-safety and premise flags relayed", Kind: "check", Specialist: true},
	}}
}

// QATemplateFor is the template for a call type.
func (p CriticalPolicy) QATemplateFor(callType string) QATemplate {
	for _, t := range p.QA {
		if slices.Contains(t.CallTypes, callType) {
			return t
		}
	}
	for _, t := range p.QA {
		if len(t.CallTypes) == 0 {
			return t
		}
	}
	return DefaultQA()
}

// Trigger is one reason an incident is critical.
type Trigger struct {
	Kind   string    `json:"kind"` // calltype, modifier, priority, event, phrase, manual
	Rule   string    `json:"rule,omitempty"`
	Name   string    `json:"name"`
	Detail string    `json:"detail,omitempty"`
	Source string    `json:"source,omitempty"` // call, radio, narrative, cad
	At     time.Time `json:"at"`
	By     string    `json:"by,omitempty"`
}

// CaseRole is a person's part in a critical incident.
type CaseRole struct {
	User string `json:"user"`
	Role string `json:"role"` // calltaker, police, fire, ems, dispatcher, supervisor, trainee…
}

// CriticalCase is one incident's critical review.
type CriticalCase struct {
	ID            string          `json:"id"`
	Incident      string          `json:"incident"`
	Opened        time.Time       `json:"opened"`
	Period        string          `json:"period"`
	Summary       string          `json:"summary"` // type, priority and place, for the shift report
	Triggers      []Trigger       `json:"triggers"`
	Designated    bool            `json:"designated"` // still critical (a supervisor may remove it, with why)
	Changes       []ExNote        `json:"changes,omitempty"`
	Ack           *Stamp          `json:"ack,omitempty"`
	Review        string          `json:"review"` // pending, open, complete
	PolicyVersion int             `json:"policyVersion"`
	Template      QATemplate      `json:"template"` // the QA template in force when it opened
	QA            []QAAnswer      `json:"qa,omitempty"`
	Findings      string          `json:"findings,omitempty"`
	Narrative     string          `json:"narrative,omitempty"` // the supervisor's narrative
	Actions       []ExNote        `json:"actions,omitempty"`   // corrective or follow-up actions, support offered, after-action opened
	Reports       []ReportVersion `json:"reports,omitempty"`
	Roles         []CaseRole      `json:"roles,omitempty"`
}

// TriggerNames are the case's triggers as words.
func (c *CriticalCase) TriggerNames() []string {
	var out []string
	for _, t := range c.Triggers {
		if !slices.Contains(out, t.Name) {
			out = append(out, t.Name)
		}
	}
	return out
}

// QAAnswer is a reviewer's answer to one measure.
type QAAnswer struct {
	Measure  string    `json:"measure"`
	Result   string    `json:"result"` // met, not-met, na, unable, followup
	Comment  string    `json:"comment,omitempty"`
	Citation string    `json:"citation,omitempty"` // the event, audio time or transcript segment it rests on
	Reason   string    `json:"reason,omitempty"`   // the not-applicable reason
	By       string    `json:"by"`
	At       time.Time `json:"at"`
}

// QAResults are the answers a measure can have.
var QAResults = []string{"met", "not-met", "na", "unable", "followup"}

// ReportVersion is an issued full review.
type ReportVersion struct {
	Version int       `json:"version"`
	Cutoff  time.Time `json:"cutoff"` // the evidence it includes
	By      string    `json:"by"`
	At      time.Time `json:"at"`
	Hash    string    `json:"hash"` // of the text issued
	Note    string    `json:"note,omitempty"`
	Kind    string    `json:"kind,omitempty"` // full, foia, accreditation
}

// PhraseAlert is a configured phrase heard in a transcript or narrative.
type PhraseAlert struct {
	ID          string    `json:"id"`
	Rule        string    `json:"rule"`
	Name        string    `json:"name"`
	Severity    string    `json:"severity"`
	Incident    string    `json:"incident,omitempty"`
	Call        string    `json:"call,omitempty"`
	Source      string    `json:"source"` // call, radio, narrative
	Channel     string    `json:"channel,omitempty"`
	Speaker     string    `json:"speaker,omitempty"`
	At          time.Time `json:"at"`
	Confidence  float64   `json:"confidence,omitempty"`
	Found       string    `json:"found"`
	Context     string    `json:"context"`
	Negated     bool      `json:"negated,omitempty"`
	Negation    string    `json:"negation,omitempty"`
	Uncertain   bool      `json:"uncertain,omitempty"`
	Testing     bool      `json:"testing,omitempty"`
	Count       int       `json:"count"` // the same phrase again within ten minutes adds here
	Disposition string    `json:"disposition,omitempty"`
	DisposedBy  string    `json:"disposedBy,omitempty"`
	DisposedAt  time.Time `json:"disposedAt,omitzero"`
	Note        string    `json:"note,omitempty"`
}

// RadioTag is a tagged fact in a radio transmission, for the review (never a finding by itself).
type RadioTag struct {
	At      time.Time `json:"at"`
	Unit    string    `json:"unit,omitempty"`
	Channel string    `json:"channel,omitempty"`
	Tag     string    `json:"tag"`
	Name    string    `json:"name"`
	Found   string    `json:"found"`
	Context string    `json:"context"`
	Negated bool      `json:"negated,omitempty"`
	Text    string    `json:"text"`
}

// criticalAfter opens and adds to critical cases and phrase alerts as events happen (in apply).
func (s *State) criticalAfter(e ledger.Event) {
	pol := s.Settings.Critical
	num := e.Incident
	if num == "" && e.Call != "" {
		if c := s.Calls[e.Call]; c != nil {
			num = c.Incident
		}
	}
	in := s.Incidents[num]
	switch e.Type {
	case EvIncCreated, EvIncChanged:
		if in == nil {
			break
		}
		if ct := s.CallTypes[in.Type]; ct != nil && ct.Critical {
			s.trigger(e, in, Trigger{Kind: "calltype", Rule: "calltype:" + in.Type, Name: "Critical call type " + in.Type, Source: "cad"})
		}
		for _, r := range pol.Rules {
			if r.Off {
				continue
			}
			switch r.Kind {
			case "calltype":
				if strings.EqualFold(r.Match, in.Type) {
					s.trigger(e, in, Trigger{Kind: r.Kind, Rule: r.ID, Name: r.Name, Detail: in.Type, Source: "cad"})
				}
			case "priority":
				var p int
				fmt.Sscan(r.Match, &p)
				if p > 0 && in.Priority > 0 && in.Priority <= p {
					s.trigger(e, in, Trigger{Kind: r.Kind, Rule: r.ID, Name: r.Name, Detail: fmt.Sprintf("P%d", in.Priority), Source: "cad"})
				}
			}
		}
	case EvIncModifiers:
		if in == nil {
			break
		}
		for _, r := range pol.Rules {
			if !r.Off && r.Kind == "modifier" && slices.ContainsFunc(in.Modifiers, func(m string) bool { return strings.EqualFold(m, r.Match) }) {
				s.trigger(e, in, Trigger{Kind: r.Kind, Rule: r.ID, Name: r.Name, Detail: r.Match, Source: "cad"})
			}
		}
	case EvEmergency:
		v, _ := decode[Emergency](e)
		if u := s.Units[v.Unit]; u != nil && u.Incident != "" {
			in = s.Incidents[u.Incident]
		}
		if in != nil {
			for _, r := range pol.Rules {
				if !r.Off && r.Kind == "event" && r.Match == "unit-emergency" {
					s.trigger(e, in, Trigger{Kind: "event", Rule: r.ID, Name: r.Name, Detail: firstNonEmpty(v.Unit, v.RadioUnit) + " emergency", Source: "radio"})
				}
			}
		}
	case EvCallReceived, EvCallUpdated:
		d, _ := decode[CallData](e)
		c := s.Calls[e.Call]
		ct := ""
		if in != nil {
			ct = in.Type
		}
		for _, l := range d.Transcript {
			s.phrases(e, in, c, phrase.Text{Source: "call", CallType: ct, Text: l.Text}, l.Speaker, "")
		}
	case EvRadioText:
		v, _ := decode[RadioLine](e)
		if v.Incident != "" {
			in = s.Incidents[v.Incident]
		}
		ct := ""
		if in != nil {
			ct = in.Type
		}
		s.phrases(e, in, nil, phrase.Text{Source: "radio", CallType: ct, Text: v.Text}, firstNonEmpty(v.Unit, v.RadioUnit), firstNonEmpty(v.ChannelName, v.Channel))
	case EvIncNote:
		if in != nil {
			v, _ := decode[noteData](e)
			s.phrases(e, in, nil, phrase.Text{Source: "narrative", CallType: in.Type, Text: v.Text}, e.Actor, "")
		}
	}
	// Everyone who works a critical incident has a part in it.
	if in != nil {
		if c := s.caseFor(in.Number); c != nil && e.Actor != "" && human(s, e.Actor) {
			role := firstNonEmpty(e.Duty, DutyOf(Actor{Role: e.Role}))
			if s.StaffOf(e.Actor).Trainee {
				role = "trainee (" + role + ")"
			}
			if !slices.ContainsFunc(c.Roles, func(r CaseRole) bool { return r.User == e.Actor && r.Role == role }) {
				c.Roles = append(c.Roles, CaseRole{User: e.Actor, Role: role})
			}
		}
	}
}

func (s *State) caseFor(number string) *CriticalCase {
	for _, c := range s.CriticalCases {
		if c.Incident == number {
			return c
		}
	}
	return nil
}

// trigger adds a trigger to the incident's case, opening it the first time.
func (s *State) trigger(e ledger.Event, in *Incident, t Trigger) {
	t.At = e.At
	c := s.caseFor(in.Number)
	if c == nil {
		s.crCount++
		pol := s.Settings.Critical
		c = &CriticalCase{ID: fmt.Sprintf("CR%d", s.crCount), Incident: in.Number, Opened: e.At, Period: s.PeriodAt(e.At).ID, Designated: true,
			Review: ReviewPending, PolicyVersion: pol.Version, Template: pol.QATemplateFor(in.Type)}
		s.CriticalCases[c.ID] = c
		in.add(e, "critical", "Critical incident review "+c.ID+" opened: "+t.Name+" (the supervisor was alerted)")
		c.Roles = append(c.Roles, CaseRole{User: in.CreatedBy, Role: "incident created"})
	}
	if slices.ContainsFunc(c.Triggers, func(x Trigger) bool { return x.Rule == t.Rule && x.Rule != "" && x.Kind != "phrase" }) {
		return
	}
	c.Summary = fmt.Sprintf("%s %s P%d at %s", in.Number, in.Type, in.Priority, locLabel(in.Location))
	c.Triggers = append(c.Triggers, t)
	if len(c.Triggers) > 1 {
		in.add(e, "critical", "Critical incident "+c.ID+": also "+t.Name)
	}
}

// phrases checks a piece of text against the agency's phrases.
func (s *State) phrases(e ledger.Event, in *Incident, c *Call, t phrase.Text, speaker, channel string) {
	pol := s.Settings.Critical
	if len(pol.Phrases) == 0 || strings.TrimSpace(t.Text) == "" {
		return
	}
	for _, m := range phrase.Check(pol.Phrases, t) {
		num, call := "", ""
		if in != nil {
			num = in.Number
		}
		if c != nil {
			call = c.ID
		}
		// The same phrase on the same incident or call within ten minutes adds to the alert.
		dup := false
		for _, a := range s.PhraseAlerts {
			if a.Rule == m.Rule && (num != "" && a.Incident == num || call != "" && a.Call == call) && e.At.Sub(a.At) < 10*time.Minute && a.Negated == m.Negated {
				a.Count++
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		s.paCount++
		a := &PhraseAlert{ID: fmt.Sprintf("PA%d", s.paCount), Rule: m.Rule, Name: m.Name, Severity: m.Severity, Incident: num, Call: call, Source: t.Source,
			Channel: channel, Speaker: speaker, At: e.At, Confidence: t.Confidence, Found: m.Found, Context: m.Context, Negated: m.Negated, Negation: m.Negation,
			Uncertain: m.Uncertain, Testing: m.Testing, Count: 1}
		s.PhraseAlerts = append(s.PhraseAlerts, a)
		if len(s.PhraseAlerts) > 2000 {
			s.PhraseAlerts = s.PhraseAlerts[len(s.PhraseAlerts)-2000:]
		}
		if in != nil {
			label := "heard"
			if m.Negated {
				label = "heard NEGATED (\"" + m.Negation + "\"), for review"
			}
			if !m.Testing {
				in.add(e, "phrase", fmt.Sprintf("Phrase alert %s: %q %s in %s", a.ID, m.Found, label, t.Source))
			}
			// A phrase rule named by a critical rule opens the review (not when negated, uncertain or testing).
			for _, r := range pol.Rules {
				if !r.Off && r.Kind == "phrase" && r.Match == m.Rule && !m.Negated && !m.Testing {
					s.trigger(e, in, Trigger{Kind: "phrase", Rule: r.ID, Name: r.Name, Detail: m.Found + ": " + m.Context, Source: t.Source})
				}
			}
		}
	}
}

// RadioTags tags the radio traffic of an incident with the agency's tag rules.
func (s *State) RadioTags(number string) []RadioTag {
	var out []RadioTag
	tags := s.Settings.Critical.TagList()
	for _, l := range s.Radio {
		if l.Incident != number {
			continue
		}
		for _, m := range phrase.Check(tags, phrase.Text{Source: "radio", Text: l.Text}) {
			out = append(out, RadioTag{At: l.At, Unit: firstNonEmpty(l.Unit, l.RadioUnit), Channel: firstNonEmpty(l.ChannelName, l.Channel), Tag: m.Rule, Name: m.Name,
				Found: m.Found, Context: m.Context, Negated: m.Negated, Text: l.Text})
		}
	}
	return out
}

type criticalFlagData struct {
	Incident string `json:"incident"`
	On       bool   `json:"on"`
	Reason   string `json:"reason"`
}

// FlagCritical adds or removes the critical designation by hand, with why (the original triggers stay).
func (e *Engine) FlagCritical(a Actor, number string, on bool, reason string) (string, error) {
	if err := mayReview(a); err != nil && on == false {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	in := e.st.Incidents[number]
	if in == nil {
		return "", problem("incident", "no incident %s", number)
	}
	if strings.TrimSpace(reason) == "" {
		return "", problem("reason", "say why")
	}
	if !on && e.st.caseFor(number) == nil {
		return "", problem("incident", "%s is not critical", number)
	}
	_, err := e.write(a, EvCriticalFlag, refs{Incident: number}, criticalFlagData{Incident: number, On: on, Reason: strings.TrimSpace(reason)})
	c := e.st.caseFor(number)
	if c == nil {
		return "", err
	}
	return c.ID, err
}

// AckCritical acknowledges a critical incident's alert.
func (e *Engine) AckCritical(a Actor, id string) error {
	if err := mayReview(a); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.CriticalCases[id] == nil {
		return problem("id", "no critical review %s", id)
	}
	_, err := e.write(a, EvCriticalAck, refs{Incident: e.st.CriticalCases[id].Incident}, idData{id})
	return err
}

type qaData struct {
	Case   string   `json:"case"`
	Answer QAAnswer `json:"answer"`
}

// AnswerQA records a reviewer's answer to a QA measure, with the evidence it rests on.
func (e *Engine) AnswerQA(a Actor, id string, ans QAAnswer) error {
	if err := mayReview(a); err != nil && DutyOf(a) != DutyRecords {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	c := e.st.CriticalCases[id]
	if c == nil {
		return problem("id", "no critical review %s", id)
	}
	i := slices.IndexFunc(c.Template.Measures, func(m QAMeasure) bool { return m.ID == ans.Measure })
	if i < 0 {
		return problem("measure", "no measure %s in %s", ans.Measure, c.Template.Name)
	}
	m := c.Template.Measures[i]
	if !slices.Contains(QAResults, ans.Result) {
		return problem("result", "met, not-met, na, unable or followup")
	}
	if ans.Result == "na" && len(m.NAReasons) > 0 && !slices.Contains(m.NAReasons, ans.Reason) {
		return problem("reason", "the not-applicable reasons allowed: %s", strings.Join(m.NAReasons, "; "))
	}
	if ans.Result != "met" && strings.TrimSpace(ans.Comment) == "" {
		return problem("comment", "say why")
	}
	if strings.TrimSpace(ans.Citation) == "" && ans.Result != "na" {
		return problem("citation", "cite the event, audio time or transcript segment it rests on")
	}
	if m.Specialist && mayReview(a) != nil {
		return dutyProblem("measure", "a supervisor or QA specialist answers %s", m.Name)
	}
	_, err := e.write(a, EvCriticalQA, refs{Incident: c.Incident}, qaData{Case: id, Answer: ans})
	return err
}

// CaseUpdate is a change to a review: its state, findings, narrative, or an action.
type CaseUpdate struct {
	Review    string `json:"review,omitempty"`
	Findings  string `json:"findings,omitempty"`
	Narrative string `json:"narrative,omitempty"`
	Action    string `json:"action,omitempty"`      // a corrective or follow-up action
	Support   string `json:"support,omitempty"`     // support offered (an operational action only; never clinical)
	AfterAct  string `json:"afterAction,omitempty"` // an after-action or readiness workflow opened
}

type caseData struct {
	Case string     `json:"case"`
	U    CaseUpdate `json:"u"`
}

// UpdateCase changes a review's state, findings or narrative, or records an action.
func (e *Engine) UpdateCase(a Actor, id string, u CaseUpdate) error {
	if err := mayReview(a); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	c := e.st.CriticalCases[id]
	if c == nil {
		return problem("id", "no critical review %s", id)
	}
	if u.Review != "" && !slices.Contains([]string{ReviewPending, ReviewOpen, ReviewComplete}, u.Review) {
		return problem("review", "pending, open or complete")
	}
	if u.Review == ReviewComplete {
		for _, m := range c.Template.Measures {
			if !slices.ContainsFunc(c.QA, func(q QAAnswer) bool { return q.Measure == m.ID }) {
				return problem("qa", "answer every QA measure first (%s is not answered)", m.Name)
			}
		}
	}
	_, err := e.write(a, EvCriticalStatus, refs{Incident: c.Incident}, caseData{Case: id, U: u})
	return err
}

type reportData struct {
	Case    string        `json:"case"`
	Version ReportVersion `json:"version"`
}

// IssueReport records that a full review (or a derived FOIA or accreditation copy) was issued.
func (e *Engine) IssueReport(a Actor, id string, v ReportVersion) (int, error) {
	if err := mayReview(a); err != nil && DutyOf(a) != DutyRecords {
		return 0, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	c := e.st.CriticalCases[id]
	if c == nil {
		return 0, problem("id", "no critical review %s", id)
	}
	v.Version = len(c.Reports) + 1
	if v.Kind == "" {
		v.Kind = "full"
	}
	_, err := e.write(a, EvCriticalReport, refs{Incident: c.Incident}, reportData{Case: id, Version: v})
	return v.Version, err
}

type phraseDecision struct {
	ID          string `json:"id"`
	Disposition string `json:"disposition"`
	Note        string `json:"note,omitempty"`
}

// DisposePhrase records a supervisor's disposition of a phrase alert.
func (e *Engine) DisposePhrase(a Actor, id, disposition, note string) error {
	if err := mayReview(a); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !slices.ContainsFunc(e.st.PhraseAlerts, func(p *PhraseAlert) bool { return p.ID == id }) {
		return problem("id", "no phrase alert %s", id)
	}
	if !slices.Contains(e.st.Settings.Critical.PhraseDispositionList(), disposition) {
		return problem("disposition", "choose: %s", strings.Join(e.st.Settings.Critical.PhraseDispositionList(), "; "))
	}
	_, err := e.write(a, EvPhraseDecided, refs{}, phraseDecision{ID: id, Disposition: disposition, Note: strings.TrimSpace(note)})
	return err
}

func (s *State) applyCritical(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvCriticalFlag:
		d, err := decode[criticalFlagData](e)
		if err != nil {
			return true, err
		}
		in := s.Incidents[d.Incident]
		if in == nil {
			return true, nil
		}
		if d.On {
			s.trigger(e, in, Trigger{Kind: "manual", Rule: "manual:" + e.Actor + ":" + e.At.Format(time.RFC3339Nano), Name: "Marked critical by " + e.Actor, Detail: d.Reason, Source: "cad", By: e.Actor})
		}
		if c := s.caseFor(d.Incident); c != nil {
			c.Designated = d.On
			c.Changes = append(c.Changes, ExNote{At: e.At, By: e.Actor, What: map[bool]string{true: "designated critical", false: "critical designation removed"}[d.On], Note: d.Reason})
			if !d.On {
				in.add(e, "critical", "Critical designation removed by "+e.Actor+": "+d.Reason+" (the triggers stay on record)")
			}
		}
		return true, nil
	case EvCriticalAck:
		d, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		if c := s.CriticalCases[d.ID]; c != nil && c.Ack == nil {
			c.Ack = &Stamp{By: e.Actor, At: e.At}
			if c.Review == ReviewPending {
				c.Review = ReviewOpen
			}
		}
		return true, nil
	case EvCriticalQA:
		d, err := decode[qaData](e)
		if err != nil {
			return true, err
		}
		if c := s.CriticalCases[d.Case]; c != nil {
			ans := d.Answer
			ans.By, ans.At = e.Actor, e.At
			c.QA = slices.DeleteFunc(c.QA, func(q QAAnswer) bool { return q.Measure == ans.Measure })
			c.QA = append(c.QA, ans)
			if c.Review == ReviewPending {
				c.Review = ReviewOpen
			}
			if ans.Result == "followup" || ans.Result == "not-met" {
				s.trackFollowup(e, "critical:"+c.ID+":"+ans.Measure, c.Incident, fmt.Sprintf("%s QA %s: %s", c.ID, ans.Measure, ans.Comment), e.Actor, time.Time{}, false)
			}
		}
		return true, nil
	case EvCriticalStatus:
		d, err := decode[caseData](e)
		if err != nil {
			return true, err
		}
		c := s.CriticalCases[d.Case]
		if c == nil {
			return true, nil
		}
		u := d.U
		if u.Review != "" {
			c.Review = u.Review
			c.Changes = append(c.Changes, ExNote{At: e.At, By: e.Actor, What: "review " + u.Review})
			if u.Review == ReviewComplete {
				for _, f := range s.Followups {
					if strings.HasPrefix(f.Key, "critical:"+c.ID) && !f.Done {
						s.trackFollowup(e, f.Key, "", "", "", time.Time{}, true)
					}
				}
			}
		}
		if u.Findings != "" {
			c.Findings = u.Findings
		}
		if u.Narrative != "" {
			c.Narrative = u.Narrative
		}
		for _, x := range [][2]string{{"action", u.Action}, {"support offered", u.Support}, {"after-action opened", u.AfterAct}} {
			if x[1] != "" {
				c.Actions = append(c.Actions, ExNote{At: e.At, By: e.Actor, What: x[0], Note: x[1]})
			}
		}
		return true, nil
	case EvCriticalReport:
		d, err := decode[reportData](e)
		if err != nil {
			return true, err
		}
		if c := s.CriticalCases[d.Case]; c != nil {
			v := d.Version
			v.By, v.At = e.Actor, e.At
			c.Reports = append(c.Reports, v)
		}
		return true, nil
	case EvPhraseDecided:
		d, err := decode[phraseDecision](e)
		if err != nil {
			return true, err
		}
		for _, a := range s.PhraseAlerts {
			if a.ID == d.ID {
				a.Disposition, a.DisposedBy, a.DisposedAt, a.Note = d.Disposition, e.Actor, e.At, d.Note
				if d.Disposition == "Critical incident opened" {
					if in := s.Incidents[a.Incident]; in != nil {
						s.trigger(e, in, Trigger{Kind: "phrase", Rule: "phrase:" + a.ID, Name: "Phrase alert " + a.Name, Detail: a.Found + ": " + a.Context, Source: a.Source, By: e.Actor})
					}
				}
				if d.Disposition == "Follow-up required" {
					s.trackFollowup(e, "phrase:"+a.ID, a.Incident, "Phrase alert "+a.ID+" ("+a.Name+"): "+d.Note, e.Actor, time.Time{}, false)
				}
			}
		}
		return true, nil
	}
	return false, nil
}

// OpenPhraseAlerts are phrase alerts not yet disposed of (testing ones left out), newest first.
func (s *State) OpenPhraseAlerts() []*PhraseAlert {
	var out []*PhraseAlert
	for i := len(s.PhraseAlerts) - 1; i >= 0; i-- {
		if a := s.PhraseAlerts[i]; a.Disposition == "" && !a.Testing {
			out = append(out, a)
		}
	}
	return out
}

// CriticalList are the critical cases, newest first: open ones, or every one.
func (s *State) CriticalList(all bool) []*CriticalCase {
	var out []*CriticalCase
	for _, c := range s.CriticalCases {
		if all || c.Review != ReviewComplete || c.Ack == nil {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Opened.After(out[j].Opened) })
	return out
}
