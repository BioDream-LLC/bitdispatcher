package cad

// NCIC and state requests: one working queue that accounts for every DMV, VCIC, NCIC, Nlets and other-state
// transaction (people, vehicles, plates, VINs, articles, guns, boats, securities) and every hit
// confirmation. The queue keeps metadata only: who asked, who sent it, where, who received the answer, its
// status and disposition. A return itself is attached to the exact person or vehicle record it was run on,
// as a new version beside what the caller reported, and only those allowed open it there. A possible hit
// stays a possible hit until a hit confirmation is answered Confirmed Valid. Every open request has one
// responsible dispatcher until it is done or handed to someone who accepts it, and nobody signs off with
// one silently left behind.

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvRequestLogged   = "request.logged"   // a request was logged (or a hit confirmation sent)
	EvRequestUpdated  = "request.updated"  // its status, sending, receipt, disposition or a follow-up attempt
	EvRequestHandoff  = "request.handoff"  // its owner handed it to someone else, or to a queue
	EvRequestAccepted = "request.accepted" // the receiver accepted it
	EvReturnAttached  = "return.attached"  // a return attached to a person or vehicle on an incident
	EvRequestOverdue  = "request.overdue"  // a request went past its due time (recorded once)
)

// Request statuses (agencies may rename them: Settings.Requests.Labels).
const (
	RqNew         = "new"
	RqAssigned    = "assigned"
	RqSent        = "sent"
	RqAwaiting    = "awaiting-return"
	RqPossibleHit = "possible-hit"
	RqHitSent     = "hit-confirmation-sent"
	RqConfirmed   = "confirmed-valid"
	RqNotValid    = "not-valid"
	RqNoExtradite = "will-not-extradite"
	RqExtLimited  = "extradition-limited"
	RqPendingInfo = "pending-information"
	RqNoRecord    = "no-record"
	RqError       = "error"
	RqCancelled   = "cancelled"
	RqReassigned  = "reassigned"
	RqComplete    = "complete"
)

// RequestStatuses are every status, in the order they are offered.
var RequestStatuses = []string{RqNew, RqAssigned, RqSent, RqAwaiting, RqPossibleHit, RqHitSent, RqConfirmed, RqNotValid, RqNoExtradite, RqExtLimited, RqPendingInfo, RqNoRecord, RqError, RqCancelled, RqReassigned, RqComplete}

// StatusLabels are the plain names of the statuses.
var StatusLabels = map[string]string{RqNew: "New", RqAssigned: "Assigned", RqSent: "Sent", RqAwaiting: "Awaiting return", RqPossibleHit: "Possible hit",
	RqHitSent: "Hit confirmation sent", RqConfirmed: "Confirmed valid", RqNotValid: "Not valid or not confirmed", RqNoExtradite: "Will not extradite",
	RqExtLimited: "Extradition limited", RqPendingInfo: "Pending further information", RqNoRecord: "No record", RqError: "Error", RqCancelled: "Cancelled",
	RqReassigned: "Reassigned", RqComplete: "Complete"}

// HitOutcomes are the answers to a hit confirmation.
var HitOutcomes = []string{RqConfirmed, RqNotValid, RqNoExtradite, RqExtLimited, RqCancelled, RqPendingInfo}

// RequestTypes and RequestCategories are what a request can be.
var (
	RequestTypes      = []string{"dmv", "vcic", "ncic", "nlets", "other-state", "hit-confirmation"}
	RequestCategories = []string{"person", "vehicle", "plate", "vin", "article", "gun", "boat", "securities", "other"}
)

func requestClosed(status string) bool {
	return slices.Contains([]string{RqConfirmed, RqNotValid, RqNoExtradite, RqExtLimited, RqNoRecord, RqCancelled, RqComplete}, status)
}

// RequestPolicy is the agency's rules for the queue.
type RequestPolicy struct {
	Labels map[string]string `json:"labels,omitempty"` // the agency's names for statuses
	// Queues are the supervisor-defined queues a request can be handed to when no one accepts it directly.
	Queues []string `json:"queues,omitempty"`
	// Minutes a hit confirmation has to be answered: urgent (0: 10) and routine (0: 60), as NCIC policy sets.
	UrgentMinutes  int `json:"urgentMinutes,omitempty"`
	RoutineMinutes int `json:"routineMinutes,omitempty"`
	// DueMinutes is how long an ordinary request may stay open before it is overdue (0: 15).
	DueMinutes int `json:"dueMinutes,omitempty"`
	// WarnMinutes before the due time a request is shown as nearly due (0: 3).
	WarnMinutes int `json:"warnMinutes,omitempty"`
	// SignOutCheckOff lets people sign off with open requests (the check is on by default).
	SignOutCheckOff bool `json:"signOutCheckOff,omitempty"`
}

func (p RequestPolicy) mins(v, def int) time.Duration {
	if v <= 0 {
		v = def
	}
	return time.Duration(v) * time.Minute
}

// Label is the agency's name for a status.
func (p RequestPolicy) Label(status string) string {
	if l := p.Labels[status]; l != "" {
		return l
	}
	return firstNonEmpty(StatusLabels[status], status)
}

// QueueList is the handoff queues (empty: Records desk, Supervisor).
func (p RequestPolicy) QueueList() []string {
	if len(p.Queues) == 0 {
		return []string{"Records desk", "Supervisor"}
	}
	return p.Queues
}

// InqReturn is an inquiry's answer attached to one record: a version beside what the caller reported.
type InqReturn struct {
	ID         string            `json:"id"`
	Request    string            `json:"request,omitempty"`
	Source     string            `json:"source"` // dmv, vcic, ncic, nlets, other-state
	Kind       string            `json:"kind"`   // vehicle or person
	At         time.Time         `json:"at"`
	By         string            `json:"by"`
	Requester  string            `json:"requester,omitempty"` // the unit or officer who asked
	Purpose    string            `json:"purpose,omitempty"`
	Fields     map[string]string `json:"fields,omitempty"` // the official values: owner, ownerAddress, year, make, model, body, color, plate, state, vin, name, dob…
	Hit        string            `json:"hit,omitempty"`    // stolen, wanted, warrant, missing, protection-order… : a POSSIBLE hit
	HitDetail  string            `json:"hitDetail,omitempty"`
	HitORI     string            `json:"hitOri,omitempty"`
	Confirmed  string            `json:"confirmed,omitempty"` // the hit confirmation's outcome, once answered
	Superseded bool              `json:"superseded,omitempty"`
	Simulated  bool              `json:"simulated,omitempty"`
	Text       string            `json:"text,omitempty"` // the return as received: shown only in the protected record
}

// Request is one entry in the queue.
type Request struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Category    string    `json:"category"`
	Incident    string    `json:"incident,omitempty"`
	Record      string    `json:"record,omitempty"`  // P1, V2 on the incident
	Subject     string    `json:"subject,omitempty"` // a short label of the record (no return content)
	Requester   string    `json:"requester,omitempty"`
	RequestedAt time.Time `json:"requestedAt"`
	Owner       string    `json:"owner"`
	SentBy      string    `json:"sentBy,omitempty"`
	SentAt      time.Time `json:"sentAt,omitzero"`
	Destination string    `json:"destination,omitempty"` // the destination agency or ORI
	Method      string    `json:"method,omitempty"`      // message switch, telephone, teletype
	Priority    string    `json:"priority,omitempty"`    // urgent or routine (hit confirmations)
	Due         time.Time `json:"due,omitzero"`
	ReceivedBy  string    `json:"receivedBy,omitempty"` // who received and recorded the answer
	ResponseAt  time.Time `json:"responseAt,omitzero"`
	Responder   string    `json:"responder,omitempty"` // who answered at the other agency
	Status      string    `json:"status"`
	Disposition string    `json:"disposition,omitempty"`
	Of          string    `json:"of,omitempty"`     // the request a hit confirmation confirms
	Return      string    `json:"return,omitempty"` // the return it produced or confirms
	Attempts    []ExNote  `json:"attempts,omitempty"`
	Notes       []ExNote  `json:"notes,omitempty"`
	Handoff     *Handoff  `json:"handoff,omitempty"` // a handoff waiting for the receiver
	Chain       []ExNote  `json:"chain"`             // every owner, who handed it over and why
	Overdue     bool      `json:"overdue,omitempty"` // went past its due time (warned)
	Updated     time.Time `json:"updated"`
}

// Handoff is a request handed to someone (or a queue) who has not yet accepted it.
type Handoff struct {
	From string    `json:"from"`
	To   string    `json:"to"` // a username, or @queue:Records desk
	At   time.Time `json:"at"`
	Note string    `json:"note,omitempty"`
}

// Open says the request still needs work.
func (r *Request) Open() bool { return !requestClosed(r.Status) }

// RequestUpdate is a change to a request.
type RequestUpdate struct {
	Status      string    `json:"status,omitempty"`
	Disposition string    `json:"disposition,omitempty"`
	Destination string    `json:"destination,omitempty"`
	Method      string    `json:"method,omitempty"`
	Sent        bool      `json:"sent,omitempty"`     // the person sent it now
	Received    bool      `json:"received,omitempty"` // the person received and recorded the answer now
	Responder   string    `json:"responder,omitempty"`
	Attempt     string    `json:"attempt,omitempty"` // a follow-up attempt, with what was done
	Note        string    `json:"note,omitempty"`
	Due         time.Time `json:"due,omitzero"`
}

type requestData struct {
	Request Request `json:"request"`
}

type requestUpdateData struct {
	ID string        `json:"id"`
	U  RequestUpdate `json:"u"`
}

type handoffData struct {
	ID   string `json:"id"`
	To   string `json:"to"`
	Note string `json:"note,omitempty"`
}

type returnData struct {
	Incident string    `json:"incident"`
	Record   string    `json:"record"`
	Return   InqReturn `json:"return"`
}

func mayRequest(a Actor) error {
	switch DutyOf(a) {
	case DutyPolice, DutyDispatcher, DutySupervisor, DutyRecords, DutyAdmin, DutyFire, DutyEMS, DutyCallTaker:
		return nil
	}
	if a.Role == "admin" {
		return nil
	}
	return dutyProblem("duty", "your duty (%s) does not work NCIC or state requests", DutyOf(a))
}

// recordLabel is a short label of a record on an incident.
func recordLabel(in *Incident, rec string) string {
	if in == nil || rec == "" {
		return ""
	}
	for _, p := range in.Persons {
		if p.ID == rec {
			return rec + " " + firstNonEmpty(p.Name, p.Role)
		}
	}
	for _, v := range in.Vehicles {
		if v.ID == rec {
			return strings.TrimSpace(rec + " " + v.Plate + " " + v.State)
		}
	}
	return rec
}

// LogRequest logs a request: owned by the person logging it unless they name someone.
func (e *Engine) LogRequest(a Actor, r Request) (string, error) {
	if err := mayRequest(a); err != nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.logRequest(a, r)
}

func (e *Engine) logRequest(a Actor, r Request) (string, error) {
	if !slices.Contains(RequestTypes, r.Type) {
		return "", problem("type", "the request type: %s", strings.Join(RequestTypes, ", "))
	}
	if r.Category == "" {
		r.Category = "other"
	}
	if !slices.Contains(RequestCategories, r.Category) {
		return "", problem("category", "what it is about: %s", strings.Join(RequestCategories, ", "))
	}
	var in *Incident
	if r.Incident != "" {
		if in = e.st.Incidents[r.Incident]; in == nil {
			return "", problem("incident", "no incident %s", r.Incident)
		}
	}
	if r.Record != "" && in != nil && recordLabel(in, r.Record) == r.Record {
		return "", problem("record", "no person or vehicle %s on %s", r.Record, r.Incident)
	}
	now := e.now()
	r.ID = fmt.Sprintf("RQ%d", e.st.rqCount+1)
	r.RequestedAt, r.Updated = now, now
	r.Owner = firstNonEmpty(strings.TrimSpace(r.Owner), a.User)
	if r.Status == "" {
		r.Status = RqNew
		if r.Owner != a.User {
			r.Status = RqAssigned
		}
	}
	if r.Subject == "" {
		r.Subject = recordLabel(in, r.Record)
	}
	pol := e.st.Settings.Requests
	if r.Due.IsZero() {
		switch {
		case r.Type == "hit-confirmation" && r.Priority == "routine":
			r.Due = now.Add(pol.mins(pol.RoutineMinutes, 60))
		case r.Type == "hit-confirmation":
			r.Priority = "urgent"
			r.Due = now.Add(pol.mins(pol.UrgentMinutes, 10))
		default:
			r.Due = now.Add(pol.mins(pol.DueMinutes, 15))
		}
	}
	r.Chain = []ExNote{{At: now, By: a.User, What: "owner " + r.Owner}}
	_, err := e.write(a, EvRequestLogged, refs{Incident: r.Incident}, requestData{Request: r})
	return r.ID, err
}

// UpdateRequest changes a request's status, sending, receipt, disposition or adds a follow-up attempt.
func (e *Engine) UpdateRequest(a Actor, id string, u RequestUpdate) error {
	if err := mayRequest(a); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	r := e.st.Requests[id]
	if r == nil {
		return problem("id", "no request %s", id)
	}
	if u.Status != "" && !slices.Contains(RequestStatuses, u.Status) {
		return problem("status", "no status %s", u.Status)
	}
	if r.Type == "hit-confirmation" && u.Status != "" && slices.Contains(HitOutcomes, u.Status) {
		if strings.TrimSpace(u.Responder) == "" && u.Status != RqCancelled {
			return problem("responder", "name who answered at the other agency")
		}
		u.Received = true
	}
	if (u.Status == RqNoExtradite || u.Status == RqExtLimited || u.Status == RqPendingInfo) && strings.TrimSpace(u.Note) == "" {
		return problem("note", "write the extradition limits, instructions or what is pending")
	}
	if u.Status == "" && u.Attempt == "" && u.Note == "" && !u.Sent && !u.Received && u.Disposition == "" && u.Destination == "" && u.Due.IsZero() {
		return problem("status", "nothing to change")
	}
	_, err := e.write(a, EvRequestUpdated, refs{Incident: r.Incident}, requestUpdateData{ID: id, U: u})
	return err
}

// HandoffRequest hands an open request to another person or a queue (@queue:Name); it stays the giver's
// until accepted.
func (e *Engine) HandoffRequest(a Actor, id, to, note string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	r := e.st.Requests[id]
	if r == nil {
		return problem("id", "no request %s", id)
	}
	if !r.Open() {
		return problem("id", "%s is done", id)
	}
	if r.Owner != a.User && mayReview(a) != nil {
		return dutyProblem("owner", "%s is %s's: they or a supervisor hand it over", id, r.Owner)
	}
	to = strings.TrimSpace(to)
	if to == "" {
		return problem("to", "name the dispatcher or queue taking it")
	}
	if q, ok := strings.CutPrefix(to, "@queue:"); ok {
		if !slices.Contains(e.st.Settings.Requests.QueueList(), q) {
			return problem("to", "no queue %s: %s", q, strings.Join(e.st.Settings.Requests.QueueList(), ", "))
		}
	} else if !e.knownUser(to) {
		return problem("to", "no user %s", to)
	}
	if strings.TrimSpace(note) == "" {
		return problem("note", "write the handoff note: what is outstanding")
	}
	_, err := e.write(a, EvRequestHandoff, refs{Incident: r.Incident}, handoffData{ID: id, To: to, Note: strings.TrimSpace(note)})
	return err
}

// AcceptRequest takes ownership of a request handed to the person (or to a queue they work).
func (e *Engine) AcceptRequest(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	r := e.st.Requests[id]
	if r == nil || r.Handoff == nil {
		return problem("id", "%s is not waiting to be accepted", id)
	}
	if r.Handoff.To != a.User && !strings.HasPrefix(r.Handoff.To, "@queue:") && mayReview(a) != nil {
		return dutyProblem("to", "%s was handed to %s", id, r.Handoff.To)
	}
	_, err := e.write(a, EvRequestAccepted, refs{Incident: r.Incident}, idData{id})
	return err
}

// OpenRequestsOf are the open requests a person owns (or was handed and has not accepted).
func (s *State) OpenRequestsOf(user string) []*Request {
	var out []*Request
	for _, r := range s.Requests {
		if r.Open() && (r.Owner == user || (r.Handoff != nil && r.Handoff.To == user)) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RequestedAt.Before(out[j].RequestedAt) })
	return out
}

// SignOffBlockers are the requests a person must finish or hand over before signing off.
func (s *State) SignOffBlockers(user string) []*Request {
	if s.Settings.Requests.SignOutCheckOff {
		return nil
	}
	var out []*Request
	for _, r := range s.OpenRequestsOf(user) {
		if r.Owner == user && r.Handoff == nil {
			out = append(out, r)
		}
	}
	return out
}

// AttachReturn attaches an inquiry's return to a person or vehicle on an incident: a new version, the
// earlier returns kept and marked superseded. A hit is only ever a possible hit here.
func (e *Engine) AttachReturn(a Actor, number, record string, ret InqReturn) (string, error) {
	if err := mayRequest(a); err != nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	in := e.st.Incidents[number]
	if in == nil {
		return "", problem("incident", "no incident %s", number)
	}
	switch {
	case slices.ContainsFunc(in.Persons, func(p Person) bool { return p.ID == record }):
		ret.Kind = "person"
	case slices.ContainsFunc(in.Vehicles, func(v Vehicle) bool { return v.ID == record }):
		ret.Kind = "vehicle"
	default:
		return "", problem("record", "no person or vehicle %s on %s", record, number)
	}
	if ret.Source == "" {
		ret.Source = "ncic"
	}
	ret.ID = fmt.Sprintf("RT%d", e.st.rtCount+1)
	ret.Confirmed = ""
	_, err := e.write(a, EvReturnAttached, refs{Incident: number}, returnData{Incident: number, Record: record, Return: ret})
	return ret.ID, err
}

// SendHitConfirmation logs a hit confirmation for a possible hit on a record (its return): to the entering
// agency's ORI, by a method, urgent (10 minutes) or routine (an hour).
func (e *Engine) SendHitConfirmation(a Actor, number, record, returnID string, r Request) (string, error) {
	if err := mayRequest(a); err != nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	in := e.st.Incidents[number]
	if in == nil {
		return "", problem("incident", "no incident %s", number)
	}
	ret := findReturn(in, record, returnID)
	if ret == nil {
		return "", problem("return", "no return %s on %s", returnID, record)
	}
	if ret.Hit == "" {
		return "", problem("return", "that return has no possible hit to confirm")
	}
	r.Type, r.Incident, r.Record, r.Return = "hit-confirmation", number, record, ret.ID
	if r.Category == "" {
		r.Category = ret.Kind
	}
	r.Destination = firstNonEmpty(strings.TrimSpace(r.Destination), ret.HitORI)
	if r.Destination == "" {
		return "", problem("destination", "give the entering agency's ORI")
	}
	if r.Priority != "routine" {
		r.Priority = "urgent"
	}
	r.Method = firstNonEmpty(r.Method, "message switch")
	r.Requester = firstNonEmpty(r.Requester, ret.Requester)
	r.Of = ret.Request
	r.Status, r.SentBy, r.SentAt = RqHitSent, a.User, e.now()
	r.Subject = recordLabel(in, record) + " " + ret.Hit
	return e.logRequest(a, r)
}

func findReturn(in *Incident, record, id string) *InqReturn {
	for i := range in.Persons {
		if in.Persons[i].ID == record {
			for j := range in.Persons[i].Returns {
				if in.Persons[i].Returns[j].ID == id {
					return &in.Persons[i].Returns[j]
				}
			}
		}
	}
	for i := range in.Vehicles {
		if in.Vehicles[i].ID == record {
			for j := range in.Vehicles[i].Returns {
				if in.Vehicles[i].Returns[j].ID == id {
					return &in.Vehicles[i].Returns[j]
				}
			}
		}
	}
	return nil
}

func (s *State) applyRequests(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvRequestLogged:
		d, err := decode[requestData](e)
		if err != nil {
			return true, err
		}
		r := d.Request
		s.Requests[r.ID] = &r
		s.rqCount++
		if in := s.Incidents[r.Incident]; in != nil {
			what := r.Type
			if r.Type == "hit-confirmation" {
				what = fmt.Sprintf("hit confirmation (%s) to %s by %s, due %s", r.Priority, r.Destination, r.Method, r.Due.Local().Format("15:04"))
			}
			in.add(e, "request", fmt.Sprintf("%s %s logged: %s%s", r.ID, what, r.Subject, suffix(r.Requester)))
		}
		return true, nil
	case EvRequestUpdated:
		d, err := decode[requestUpdateData](e)
		if err != nil {
			return true, err
		}
		r := s.Requests[d.ID]
		if r == nil {
			return true, nil
		}
		u := d.U
		r.Updated = e.At
		if u.Destination != "" {
			r.Destination = u.Destination
		}
		if u.Method != "" {
			r.Method = u.Method
		}
		if u.Sent {
			r.SentBy, r.SentAt = e.Actor, e.At
			if u.Status == "" && (r.Status == RqNew || r.Status == RqAssigned) {
				u.Status = RqAwaiting
			}
		}
		if u.Received {
			r.ReceivedBy, r.ResponseAt = e.Actor, e.At
		}
		if u.Responder != "" {
			r.Responder = u.Responder
		}
		if !u.Due.IsZero() {
			r.Due, r.Overdue = u.Due, false
		}
		if u.Attempt != "" {
			r.Attempts = append(r.Attempts, ExNote{At: e.At, By: e.Actor, What: "attempt", Note: u.Attempt})
		}
		if u.Note != "" {
			r.Notes = append(r.Notes, ExNote{At: e.At, By: e.Actor, What: "note", Note: u.Note})
		}
		if u.Disposition != "" {
			r.Disposition = u.Disposition
		}
		if u.Status != "" && u.Status != r.Status {
			r.Notes = append(r.Notes, ExNote{At: e.At, By: e.Actor, What: "status " + u.Status})
			r.Status = u.Status
			// A hit confirmation's answer goes onto the return it confirms.
			if r.Type == "hit-confirmation" && slices.Contains(HitOutcomes, u.Status) {
				if in := s.Incidents[r.Incident]; in != nil {
					if ret := findReturn(in, r.Record, r.Return); ret != nil {
						ret.Confirmed = u.Status
					}
					in.add(e, "request", fmt.Sprintf("Hit confirmation %s answered: %s by %s, recorded by %s%s", r.ID, StatusLabels[u.Status], r.Responder, e.Actor, suffix(u.Note)))
				}
				if of := s.Requests[r.Of]; of != nil && of.Open() {
					of.Status, of.Updated = u.Status, e.At
					of.Notes = append(of.Notes, ExNote{At: e.At, By: e.Actor, What: "hit confirmation " + r.ID + ": " + u.Status})
				}
			}
		}
		return true, nil
	case EvRequestHandoff:
		d, err := decode[handoffData](e)
		if err != nil {
			return true, err
		}
		if r := s.Requests[d.ID]; r != nil {
			r.Handoff = &Handoff{From: r.Owner, To: d.To, At: e.At, Note: d.Note}
			r.Chain = append(r.Chain, ExNote{At: e.At, By: e.Actor, What: "handed to " + d.To, Note: d.Note})
			r.Updated = e.At
		}
		return true, nil
	case EvRequestAccepted:
		d, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		if r := s.Requests[d.ID]; r != nil && r.Handoff != nil {
			from := r.Owner
			r.Owner = e.Actor
			r.Chain = append(r.Chain, ExNote{At: e.At, By: e.Actor, What: "accepted from " + from, Note: r.Handoff.Note})
			r.Handoff = nil
			r.Updated = e.At
			if r.Status == RqNew {
				r.Status = RqAssigned
			}
		}
		return true, nil
	case EvReturnAttached:
		d, err := decode[returnData](e)
		if err != nil {
			return true, err
		}
		s.rtCount++
		in := s.Incidents[d.Incident]
		if in == nil {
			return true, nil
		}
		ret := d.Return
		ret.At, ret.By = e.At, e.Actor
		for i := range in.Persons {
			if in.Persons[i].ID == d.Record {
				for j := range in.Persons[i].Returns {
					in.Persons[i].Returns[j].Superseded = true
				}
				in.Persons[i].Returns = append(in.Persons[i].Returns, ret)
			}
		}
		for i := range in.Vehicles {
			if in.Vehicles[i].ID == d.Record {
				for j := range in.Vehicles[i].Returns {
					in.Vehicles[i].Returns[j].Superseded = true
				}
				in.Vehicles[i].Returns = append(in.Vehicles[i].Returns, ret)
			}
		}
		hit := ""
		if ret.Hit != "" {
			hit = " POSSIBLE HIT (" + ret.Hit + "): not confirmed until the entering agency answers a hit confirmation"
		}
		in.add(e, "request", fmt.Sprintf("%s return %s attached to %s%s", strings.ToUpper(ret.Source), ret.ID, d.Record, hit))
		if r := s.Requests[ret.Request]; r != nil {
			r.Return, r.Updated = ret.ID, e.At
			if r.ResponseAt.IsZero() {
				r.ReceivedBy, r.ResponseAt = e.Actor, e.At
			}
			if r.Open() {
				switch {
				case ret.Hit != "":
					r.Status = RqPossibleHit
				case ret.Fields["noRecord"] == "yes":
					r.Status = RqNoRecord
				default:
					r.Status = RqComplete
				}
			}
		}
		return true, nil
	case EvRequestOverdue:
		d, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		if r := s.Requests[d.ID]; r != nil {
			r.Overdue = true
			r.Notes = append(r.Notes, ExNote{At: e.At, By: e.Actor, What: "overdue: the supervisor was told"})
		}
		return true, nil
	}
	return false, nil
}

// WarnRequests records once each open request past its due time, and returns those newly overdue.
func (e *Engine) WarnRequests() ([]Request, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	var out []Request
	for _, r := range e.st.Requests {
		if !r.Open() || r.Overdue || r.Due.IsZero() || now.Before(r.Due) {
			continue
		}
		if _, err := e.write(SystemActor, EvRequestOverdue, refs{Incident: r.Incident}, idData{r.ID}); err != nil {
			return out, err
		}
		out = append(out, *r)
	}
	return out, nil
}

// RequestList is the queue, filtered. Metadata only: never a return's text or fields.
type RequestFilter struct {
	Status   string `json:"status,omitempty"` // a status, "open" or "closed"
	Owner    string `json:"owner,omitempty"`
	Incident string `json:"incident,omitempty"`
	Type     string `json:"type,omitempty"`
	Category string `json:"category,omitempty"`
	Text     string `json:"text,omitempty"` // matches requester, subject, ORI
	From     time.Time
	To       time.Time
}

// RequestList returns the requests that match, the most urgent first.
func (s *State) RequestList(f RequestFilter) []*Request {
	var out []*Request
	for _, r := range s.Requests {
		switch {
		case f.Status == "open" && !r.Open(), f.Status == "closed" && r.Open(),
			f.Status != "" && f.Status != "open" && f.Status != "closed" && r.Status != f.Status,
			f.Owner != "" && r.Owner != f.Owner && (r.Handoff == nil || r.Handoff.To != f.Owner),
			f.Incident != "" && r.Incident != f.Incident, f.Type != "" && r.Type != f.Type, f.Category != "" && r.Category != f.Category,
			!f.From.IsZero() && r.RequestedAt.Before(f.From), !f.To.IsZero() && r.RequestedAt.After(f.To):
			continue
		}
		if t := strings.ToLower(strings.TrimSpace(f.Text)); t != "" &&
			!strings.Contains(strings.ToLower(r.Requester+" "+r.Subject+" "+r.Destination+" "+r.Owner+" "+r.ID), t) {
			continue
		}
		cp := *r
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Open() != b.Open() {
			return a.Open()
		}
		if !a.Due.Equal(b.Due) {
			return a.Due.Before(b.Due)
		}
		return a.ID < b.ID
	})
	return out
}
