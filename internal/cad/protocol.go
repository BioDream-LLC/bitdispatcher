package cad

// Call-taking protocols: the agency's own question sets that lead the call-taker from what the caller
// says to a determinant (a code, a priority, the response, what to tell the caller). They are the
// agency's text, approved by its medical director or chief; bitdispatcher ships only a marked example.
// Licensed protocol software (ProQA and others) reports its determinant through a hook instead.

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvProtocolSaved   = "protocol.saved"
	EvProtocolRemoved = "protocol.removed"
	EvProtocolAnswer  = "protocol.answer"      // a question answered on an incident
	EvDeterminant     = "protocol.determinant" // a determinant reached (by a protocol here, or reported by protocol software)
)

// Protocol is one question set.
type Protocol struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Version      string        `json:"version"`
	Approved     bool          `json:"approved"` // only approved protocols are offered on calls
	Example      bool          `json:"example,omitempty"`
	CallTypes    []string      `json:"callTypes"` // the call types it is offered for
	Start        string        `json:"start"`     // the first question's ID
	Questions    []Question    `json:"questions"`
	Determinants []Determinant `json:"determinants"`
	Updated      time.Time     `json:"updated"`
	By           string        `json:"by"`
}

// Question is one question with its answers.
type Question struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	Answers []Answer `json:"answers"`
}

// Answer leads to the next question, or ends at a determinant.
type Answer struct {
	Label       string `json:"label"`
	Next        string `json:"next,omitempty"`
	Determinant string `json:"determinant,omitempty"`
}

// Determinant is where a protocol ends.
type Determinant struct {
	Code         string   `json:"code"`
	Text         string   `json:"text"`
	Priority     int      `json:"priority,omitempty"` // sets the incident's priority when reached
	CallType     string   `json:"callType,omitempty"` // changes the call type when reached
	Instructions []string `json:"instructions,omitempty"`
}

// ProtocolRun is a protocol worked on an incident.
type ProtocolRun struct {
	Protocol    string           `json:"protocol"`
	Version     string           `json:"version"`
	System      string           `json:"system,omitempty"` // protocol software that reported it, if not this
	Answers     []ProtocolAnswer `json:"answers"`
	Current     string           `json:"current,omitempty"` // the question waiting for an answer
	Determinant *Determinant     `json:"determinant,omitempty"`
	At          time.Time        `json:"at,omitzero"`
	By          string           `json:"by,omitempty"`
}

// ProtocolAnswer is one answer given.
type ProtocolAnswer struct {
	Question string    `json:"question"`
	Text     string    `json:"text"` // the question as asked
	Answer   string    `json:"answer"`
	At       time.Time `json:"at"`
	By       string    `json:"by"`
}

var reProtoID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,40}$`)

// CheckProtocol says what is wrong with a protocol: every question reachable has answers, every answer
// goes somewhere that exists, and there are no loops.
func CheckProtocol(p *Protocol) error {
	p.Name, p.Version = strings.TrimSpace(p.Name), strings.TrimSpace(p.Version)
	if !reProtoID.MatchString(p.ID) {
		return problem("id", "an id is up to 40 letters, digits, dots, dashes and underscores")
	}
	if p.Name == "" || p.Version == "" {
		return problem("name", "give the protocol's name and version")
	}
	qs := map[string]*Question{}
	for i := range p.Questions {
		q := &p.Questions[i]
		if q.ID == "" || q.Text == "" || len(q.Answers) == 0 {
			return problem("questions", "question %q needs an id, its text and answers", q.ID)
		}
		if qs[q.ID] != nil {
			return problem("questions", "two questions are %s", q.ID)
		}
		qs[q.ID] = q
	}
	ds := map[string]bool{}
	for _, d := range p.Determinants {
		if d.Code == "" || d.Text == "" {
			return problem("determinants", "a determinant needs its code and text")
		}
		if d.Priority < 0 || d.Priority > 5 {
			return problem("determinants", "%s: priority is 1 to 5", d.Code)
		}
		ds[d.Code] = true
	}
	if qs[p.Start] == nil {
		return problem("start", "the first question %q is not in the protocol", p.Start)
	}
	for _, q := range p.Questions {
		for _, a := range q.Answers {
			switch {
			case a.Label == "":
				return problem("questions", "%s has an answer without words", q.ID)
			case (a.Next == "") == (a.Determinant == ""):
				return problem("questions", "%s / %s: an answer leads to one next question or one determinant", q.ID, a.Label)
			case a.Next != "" && qs[a.Next] == nil:
				return problem("questions", "%s / %s leads to %s, which is not a question here", q.ID, a.Label, a.Next)
			case a.Determinant != "" && !ds[a.Determinant]:
				return problem("questions", "%s / %s ends at %s, which is not a determinant here", q.ID, a.Label, a.Determinant)
			}
		}
	}
	// No loops: walk from the start.
	var walk func(id string, path []string) error
	walk = func(id string, path []string) error {
		if slices.Contains(path, id) {
			return problem("questions", "the questions loop: %s", strings.Join(append(path, id), " → "))
		}
		for _, a := range qs[id].Answers {
			if a.Next != "" {
				if err := walk(a.Next, append(path, id)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(p.Start, nil)
}

// SaveProtocol adds or changes a protocol.
func (e *Engine) SaveProtocol(a Actor, p Protocol) error {
	if err := CheckProtocol(&p); err != nil {
		return err
	}
	p.Updated, p.By = e.now(), a.User
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := e.write(a, EvProtocolSaved, refs{}, p)
	return err
}

// RemoveProtocol removes one.
func (e *Engine) RemoveProtocol(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.Protocols[id] == nil {
		return problem("id", "no protocol %s", id)
	}
	_, err := e.write(a, EvProtocolRemoved, refs{}, idData{ID: id})
	return err
}

// AnswerProtocol records an answer on an incident's protocol (starting it with the first answer).
func (e *Engine) AnswerProtocol(a Actor, number, protocol, question, label string) error {
	if err := mayEnter(a); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	in, ok := e.st.Incidents[number]
	if !ok {
		return problem("incident", "no incident %s", number)
	}
	p := e.st.Protocols[protocol]
	if p == nil || !p.Approved {
		return problem("protocol", "no approved protocol %s", protocol)
	}
	want := p.Start
	if run := in.Protocol; run != nil && run.Protocol == protocol && run.System == "" {
		if run.Determinant != nil {
			return problem("protocol", "the protocol is finished: %s", run.Determinant.Code)
		}
		want = run.Current
	}
	if question != want {
		return problem("question", "the question to answer is %s", want)
	}
	q := p.question(question)
	i := slices.IndexFunc(q.Answers, func(x Answer) bool { return x.Label == label })
	if i < 0 {
		return problem("answer", "%q is not an answer to %s", label, question)
	}
	ans := q.Answers[i]
	if _, err := e.write(a, EvProtocolAnswer, refs{Incident: number}, map[string]string{"protocol": p.ID, "version": p.Version, "question": q.ID, "text": q.Text, "answer": label, "next": ans.Next}); err != nil {
		return err
	}
	if ans.Determinant != "" {
		d := p.determinant(ans.Determinant)
		return e.reachDeterminant(a, number, p.ID, p.Version, "", *d)
	}
	return nil
}

// SetDeterminant takes a determinant from protocol software (ProQA and others) for an incident.
func (e *Engine) SetDeterminant(a Actor, number, system string, d Determinant) error {
	d.Code, d.Text = strings.TrimSpace(d.Code), strings.TrimSpace(d.Text)
	if d.Code == "" {
		return problem("code", "give the determinant code")
	}
	if d.Priority < 0 || d.Priority > 5 {
		return problem("priority", "priority is 1 to 5")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.st.Incidents[number]; !ok {
		return problem("incident", "no incident %s", number)
	}
	if d.CallType != "" && e.st.CallTypes[d.CallType] == nil {
		d.CallType = "" // a code the agency has no call type for changes nothing
	}
	return e.reachDeterminant(a, number, "", "", system, d)
}

// reachDeterminant records a determinant; the caller holds e.mu.
func (e *Engine) reachDeterminant(a Actor, number, protocol, version, system string, d Determinant) error {
	_, err := e.write(a, EvDeterminant, refs{Incident: number}, map[string]any{"protocol": protocol, "version": version, "system": system, "determinant": d})
	return err
}

func (p *Protocol) question(id string) *Question {
	for i := range p.Questions {
		if p.Questions[i].ID == id {
			return &p.Questions[i]
		}
	}
	return nil
}

func (p *Protocol) determinant(code string) *Determinant {
	for i := range p.Determinants {
		if p.Determinants[i].Code == code {
			return &p.Determinants[i]
		}
	}
	return nil
}

// ProtocolsFor are the approved protocols offered for a call type.
func (s *State) ProtocolsFor(callType string) []*Protocol {
	var out []*Protocol
	for _, p := range s.Protocols {
		if p.Approved && (len(p.CallTypes) == 0 || slices.Contains(p.CallTypes, callType)) {
			out = append(out, p)
		}
	}
	slices.SortFunc(out, func(a, b *Protocol) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func (s *State) applyProtocol(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvProtocolSaved:
		p, err := decode[Protocol](e)
		if err != nil {
			return true, err
		}
		if s.Protocols == nil {
			s.Protocols = map[string]*Protocol{}
		}
		s.Protocols[p.ID] = &p
		return true, nil
	case EvProtocolRemoved:
		v, _ := decode[idData](e)
		delete(s.Protocols, v.ID)
		return true, nil
	case EvProtocolAnswer:
		v, _ := decode[map[string]string](e)
		in, ok := s.Incidents[e.Incident]
		if !ok {
			return true, nil
		}
		if in.Protocol == nil || in.Protocol.Protocol != v["protocol"] || in.Protocol.System != "" {
			in.Protocol = &ProtocolRun{Protocol: v["protocol"], Version: v["version"]}
		}
		in.Protocol.Answers = append(in.Protocol.Answers, ProtocolAnswer{Question: v["question"], Text: v["text"], Answer: v["answer"], At: e.At, By: e.Actor})
		in.Protocol.Current = v["next"]
		in.add(e, "protocol", fmt.Sprintf("%s: %s", v["text"], v["answer"]))
		return true, nil
	case EvDeterminant:
		v, err := decode[struct {
			Protocol, Version, System string
			Determinant               Determinant
		}](e)
		if err != nil {
			return true, err
		}
		in, ok := s.Incidents[e.Incident]
		if !ok {
			return true, nil
		}
		if in.Protocol == nil || v.System != "" || in.Protocol.Protocol != v.Protocol {
			in.Protocol = &ProtocolRun{Protocol: v.Protocol, Version: v.Version, System: v.System}
		}
		d := v.Determinant
		in.Protocol.Determinant, in.Protocol.Current, in.Protocol.At, in.Protocol.By = &d, "", e.At, e.Actor
		by := firstNonEmpty(v.System, v.Protocol+" v"+v.Version)
		in.add(e, "priority-note", fmt.Sprintf("Determinant %s: %s (%s)", d.Code, d.Text, by))
		if d.Priority > 0 && d.Priority != in.Priority {
			in.add(e, "priority", fmt.Sprintf("priority P%d → P%d by determinant %s", in.Priority, d.Priority, d.Code))
			in.Priority = d.Priority
		}
		if d.CallType != "" && d.CallType != in.Type {
			if _, ok := s.CallTypes[d.CallType]; ok {
				in.add(e, "type", fmt.Sprintf("call type %s → %s by determinant %s", in.Type, d.CallType, d.Code))
				in.Type = d.CallType
			}
		}
		return true, nil
	}
	return false, nil
}

// ExampleProtocol is a short, generic example to show how a protocol is written. It is not a medical
// protocol and is not approved for use: an agency writes its own under its medical director.
func ExampleProtocol() Protocol {
	return Protocol{ID: "EX-BREATHING", Name: "EXAMPLE: breathing problem (not for use)", Version: "example-1", Example: true, Start: "q1",
		Questions: []Question{
			{ID: "q1", Text: "Is the patient awake and breathing?", Answers: []Answer{{Label: "Yes", Next: "q2"}, {Label: "No / not sure", Determinant: "EX-1"}}},
			{ID: "q2", Text: "Can the patient speak in full sentences?", Answers: []Answer{{Label: "Yes", Determinant: "EX-3"}, {Label: "No", Determinant: "EX-2"}}},
		},
		Determinants: []Determinant{
			{Code: "EX-1", Text: "EXAMPLE: not breathing or unknown", Priority: 1, Instructions: []string{"EXAMPLE TEXT: the agency's own instructions go here."}},
			{Code: "EX-2", Text: "EXAMPLE: difficulty speaking", Priority: 2},
			{Code: "EX-3", Text: "EXAMPLE: speaking normally", Priority: 3},
		}}
}
