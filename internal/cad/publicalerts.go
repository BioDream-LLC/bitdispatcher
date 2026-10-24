package cad

// Public alerts (mass notification): a supervisor warns the public of a danger in an area: shelter in
// place, evacuate, a hazardous spill, a missing child, 911 being down. An alert has its event (a SAME
// code), what is happening and what to do, a short text for phones, the area, and when it expires. When
// the agency asks for it, a second supervisor approves each alert before it goes. It is published as a
// CAP message (internal/alerting) to the agency's public alert feed and CAP receivers, and sent to the
// people on its subscriber lists through the paging gateways (text, email, push), those with a known
// place only when they are inside the area. An update or a cancel is a new message naming the first.

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/biodream-llc/bitdispatcher/internal/geo"
	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvAlertListSaved   = "alertlist.saved"
	EvAlertListRemoved = "alertlist.removed"
	EvAlertDrafted     = "alert.drafted"  // written, waiting for a second supervisor
	EvAlertIssued      = "alert.issued"   // approved and sent
	EvAlertRejected    = "alert.rejected" // the second supervisor turned it down
	EvAlertResult      = "alert.result"   // how its deliveries went
)

// Alert states.
const (
	AlertPending   = "pending"   // waiting for approval
	AlertIssued    = "issued"    // sent and in force
	AlertRejected  = "rejected"  // not approved
	AlertUpdated   = "updated"   // replaced by an update
	AlertCancelled = "cancelled" // called off
)

// AlertText is what an alert says in one language.
type AlertText struct {
	Headline    string `json:"headline"`
	Description string `json:"description,omitempty"` // what is happening
	Instruction string `json:"instruction,omitempty"` // what to do
	Short       string `json:"short,omitempty"`       // the phone alert (WEA): up to 90 characters
	Long        string `json:"long,omitempty"`        // the longer phone alert: up to 360
}

// AlertArea is where an alert applies.
type AlertArea struct {
	Desc     string       `json:"desc"`
	Polygon  [][2]float64 `json:"polygon,omitempty"`  // [lon, lat] corners, as the map's areas
	Geocodes []string     `json:"geocodes,omitempty"` // SAME county codes, six digits
	Fence    string       `json:"fence,omitempty"`    // the area on the map it was drawn from
}

// PublicAlert is one alert to the public.
type PublicAlert struct {
	ID           string       `json:"id"`         // AL1
	Identifier   string       `json:"identifier"` // the CAP identifier
	Status       string       `json:"status"`     // Actual, Exercise, Test
	MsgType      string       `json:"msgType"`    // Alert, Update, Cancel
	Of           string       `json:"of,omitempty"`
	Event        string       `json:"event"` // SAME code
	EventName    string       `json:"eventName"`
	Category     string       `json:"category"`
	Response     string       `json:"response,omitempty"` // Shelter, Evacuate, Avoid…
	Urgency      string       `json:"urgency"`
	Severity     string       `json:"severity"`
	Certainty    string       `json:"certainty"`
	Text         AlertText    `json:"text"`
	Spanish      *AlertText   `json:"spanish,omitempty"`
	Area         AlertArea    `json:"area"`
	Expires      time.Time    `json:"expires"`
	Block        []string     `json:"block,omitempty"`     // channels kept off: EAS, NWEM, CMAS, PUBLIC
	Lists        []string     `json:"lists,omitempty"`     // subscriber lists
	Receivers    []string     `json:"receivers,omitempty"` // CAP receivers
	Incident     string       `json:"incident,omitempty"`
	Created      time.Time    `json:"created"`
	By           string       `json:"by"`
	State        string       `json:"state"`
	Sent         time.Time    `json:"sent,omitzero"`
	ApprovedBy   string       `json:"approvedBy,omitempty"`
	Rejected     string       `json:"rejected,omitempty"` // why
	SupersededBy string       `json:"supersededBy,omitempty"`
	Results      []PageResult `json:"results"`
	Reached      int          `json:"reached"` // subscribers sent to
}

// Expired says whether an issued alert's time is up.
func (a *PublicAlert) Expired(now time.Time) bool { return !a.Expires.IsZero() && now.After(a.Expires) }

// InForce says whether it is issued, not replaced and not expired.
func (a *PublicAlert) InForce(now time.Time) bool { return a.State == AlertIssued && !a.Expired(now) }

// Subscriber is one person on an alert list.
type Subscriber struct {
	Name  string   `json:"name,omitempty"`
	Phone string   `json:"phone,omitempty"` // E.164
	Email string   `json:"email,omitempty"`
	Topic string   `json:"topic,omitempty"` // ntfy
	Lang  string   `json:"lang,omitempty"`  // es: sent the Spanish text when there is one
	Lat   *float64 `json:"lat,omitempty"`   // where they live or work: sent only alerts for there
	Lon   *float64 `json:"lon,omitempty"`
}

// AlertList is a list of people who asked for alerts, and how they are reached.
type AlertList struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Gateways []string     `json:"gateways"` // paging gateways (Admin › Integrations › Paging)
	People   []Subscriber `json:"people"`
	Note     string       `json:"note,omitempty"`
}

type alertIssued struct {
	ID       string `json:"id"`
	Approver string `json:"approver"`
}

type alertRejected struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type alertResults struct {
	ID      string       `json:"id"`
	Results []PageResult `json:"results"`
	Reached int          `json:"reached"`
}

// keepAlerts is how many alerts the live state keeps.
const keepAlerts = 500

// SaveAlertList adds or changes a subscriber list.
func (e *Engine) SaveAlertList(a Actor, l AlertList) (string, error) {
	l.Name = strings.TrimSpace(l.Name)
	if l.Name == "" {
		return "", problem("name", "give the list a name")
	}
	if len(l.Gateways) == 0 {
		return "", problem("gateways", "choose how the list is reached (a paging gateway in Admin › Integrations › Paging)")
	}
	for i, p := range l.People {
		p.Name, p.Phone, p.Email, p.Topic = strings.TrimSpace(p.Name), strings.TrimSpace(p.Phone), strings.TrimSpace(p.Email), strings.TrimSpace(p.Topic)
		if p.Phone == "" && p.Email == "" && p.Topic == "" {
			return "", problem("people", "person %d has no phone, email or push topic", i+1)
		}
		if (p.Lat == nil) != (p.Lon == nil) {
			return "", problem("people", "person %d: give both latitude and longitude, or neither", i+1)
		}
		l.People[i] = p
	}
	if l.People == nil {
		l.People = []Subscriber{}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if l.ID == "" {
		l.ID = e.newID("ALL", func(id string) bool { return e.st.AlertLists[id] != nil })
	} else if e.st.AlertLists[l.ID] == nil {
		return "", problem("id", "no alert list %s", l.ID)
	}
	_, err := e.write(a, EvAlertListSaved, refs{}, l)
	return l.ID, err
}

// RemoveAlertList removes one.
func (e *Engine) RemoveAlertList(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.AlertLists[id] == nil {
		return problem("id", "no alert list %s", id)
	}
	_, err := e.write(a, EvAlertListRemoved, refs{}, idData{id})
	return err
}

// AlertCheck is a check the engine makes before an alert is written; the CAP message's own checks are
// internal/alerting's.
func (st *State) checkAlert(p *PublicAlert, now time.Time) error {
	switch p.Status {
	case "Actual", "Exercise", "Test":
	default:
		return problem("status", "an alert is Actual, Exercise or Test")
	}
	if strings.TrimSpace(p.Text.Headline) == "" && p.MsgType != "Cancel" {
		return problem("headline", "write the headline")
	}
	if n := utf8.RuneCountInString(p.Text.Short); n > 90 {
		return problem("short", "the phone alert is %d characters; the limit is 90", n)
	}
	if n := utf8.RuneCountInString(p.Text.Long); n > 360 {
		return problem("long", "the longer phone alert is %d characters; the limit is 360", n)
	}
	if p.Spanish != nil {
		if strings.TrimSpace(p.Spanish.Headline) == "" {
			p.Spanish = nil
		} else if utf8.RuneCountInString(p.Spanish.Short) > 90 || utf8.RuneCountInString(p.Spanish.Long) > 360 {
			return problem("spanish", "the Spanish phone alert is up to 90 characters, the longer one 360")
		}
	}
	if !p.Expires.After(now) {
		return problem("expires", "the alert must expire after it is sent")
	}
	if p.Expires.Sub(now) > 48*time.Hour {
		return problem("expires", "an alert lasts at most 48 hours; send an update to keep it going")
	}
	if p.Area.Fence != "" {
		g := st.Geofences[p.Area.Fence]
		if g == nil {
			return problem("area", "no area %s on the map", p.Area.Fence)
		}
		if len(p.Area.Polygon) == 0 {
			p.Area.Polygon = slices.Clone(g.Ring)
		}
		if p.Area.Desc == "" {
			p.Area.Desc = g.Name
		}
	}
	p.Area.Desc = strings.TrimSpace(p.Area.Desc)
	if p.Area.Desc == "" {
		return problem("area", "describe the area")
	}
	if len(p.Area.Polygon) > 0 && len(p.Area.Polygon) < 3 {
		return problem("area", "draw the area with at least three corners")
	}
	if len(p.Area.Polygon) == 0 && len(p.Area.Geocodes) == 0 {
		return problem("area", "draw the area, choose one on the map, or alert the whole jurisdiction")
	}
	for _, l := range p.Lists {
		if st.AlertLists[l] == nil {
			return problem("lists", "no alert list %s", l)
		}
	}
	return nil
}

// DraftAlert writes an alert. With approval needed it waits for a second supervisor; otherwise it is
// issued at once. An update or cancel names the alert it follows (of), which must be in force.
func (e *Engine) DraftAlert(a Actor, p PublicAlert, needApproval bool) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	p.MsgType = firstNonEmptyStr(p.MsgType, "Alert")
	switch p.MsgType {
	case "Alert":
		p.Of = ""
	case "Update", "Cancel":
		of := e.st.alert(p.Of)
		if of == nil || !of.InForce(now) {
			return "", problem("of", "alert %s is not in force", p.Of)
		}
		if p.MsgType == "Cancel" {
			// A cancel says the same thing is over, to the same people.
			old := *of
			old.Text.Headline = "CANCELLED: " + of.Text.Headline
			if p.Text.Description != "" {
				old.Text.Description = p.Text.Description
			}
			old.Text.Instruction = firstNonEmptyStr(p.Text.Instruction, "The alert is over.")
			old.Text.Short = firstNonEmptyStr(p.Text.Short, trimRunes("ALL CLEAR: "+of.EventName+" alert cancelled. "+of.Area.Desc, 90))
			old.Text.Long, old.Spanish = "", nil
			if of.Spanish != nil {
				old.Spanish = &AlertText{Headline: "CANCELADO: " + of.Spanish.Headline, Instruction: "La alerta ha terminado.", Short: trimRunes("FIN DE LA ALERTA: "+of.Area.Desc, 90)}
			}
			old.Response, old.Urgency, old.Severity, old.Certainty = "AllClear", "Past", "Minor", "Observed"
			old.Expires = now.Add(time.Hour)
			p = PublicAlert{Status: of.Status, MsgType: "Cancel", Of: of.ID, Event: of.Event, EventName: of.EventName, Category: of.Category,
				Response: old.Response, Urgency: old.Urgency, Severity: old.Severity, Certainty: old.Certainty, Text: old.Text, Spanish: old.Spanish,
				Area: of.Area, Expires: old.Expires, Block: of.Block, Lists: of.Lists, Receivers: of.Receivers, Incident: of.Incident}
		}
		p.Status = of.Status // an exercise's update is an exercise
	default:
		return "", problem("msgType", "an alert is an Alert, an Update or a Cancel")
	}
	if p.Incident != "" && e.st.Incidents[p.Incident] == nil {
		return "", problem("incident", "no incident %s", p.Incident)
	}
	p.Event = strings.ToUpper(strings.TrimSpace(p.Event))
	if p.Event == "" {
		return "", problem("event", "choose what kind of alert it is")
	}
	if err := e.st.checkAlert(&p, now); err != nil {
		return "", err
	}
	p.ID = fmt.Sprintf("AL%s%d", e.sitePrefix, e.st.alertCount+1)
	p.Identifier = p.ID + "-" + now.UTC().Format("20060102T150405")
	p.Lists, p.Receivers, p.Block = dedupe(p.Lists), dedupe(p.Receivers), dedupe(p.Block)
	if _, err := e.write(a, EvAlertDrafted, refs{Incident: p.Incident}, p); err != nil {
		return "", err
	}
	if !needApproval {
		if _, err := e.write(a, EvAlertIssued, refs{Incident: p.Incident}, alertIssued{ID: p.ID, Approver: a.User}); err != nil {
			return p.ID, err
		}
		e.markAlertOut(p.ID)
	}
	return p.ID, nil
}

func (e *Engine) markAlertOut(id string) {
	if e.alertOut == nil {
		e.alertOut = map[string]bool{}
	}
	e.alertOut[id] = true
}

// ApproveAlert is the second supervisor's approval: someone other than its author issues it.
func (e *Engine) ApproveAlert(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	p := e.st.alert(id)
	if p == nil || p.State != AlertPending {
		return problem("id", "alert %s is not waiting for approval", id)
	}
	if p.By == a.User {
		return problem("id", "a second person approves an alert, not the one who wrote it")
	}
	if !p.Expires.After(e.now()) {
		return problem("expires", "the alert expired while it waited; write it again")
	}
	if p.Of != "" {
		if of := e.st.alert(p.Of); of == nil || !of.InForce(e.now()) {
			return problem("of", "alert %s is no longer in force", p.Of)
		}
	}
	e.markAlertOut(id) // before the event, so the worker woken by it finds it
	if _, err := e.write(a, EvAlertIssued, refs{Incident: p.Incident}, alertIssued{ID: id, Approver: a.User}); err != nil {
		delete(e.alertOut, id)
		return err
	}
	return nil
}

// RejectAlert turns down an alert waiting for approval (its author may withdraw it too).
func (e *Engine) RejectAlert(a Actor, id, reason string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	p := e.st.alert(id)
	if p == nil || p.State != AlertPending {
		return problem("id", "alert %s is not waiting for approval", id)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "not approved"
		if p.By == a.User {
			reason = "withdrawn"
		}
	}
	_, err := e.write(a, EvAlertRejected, refs{Incident: p.Incident}, alertRejected{ID: id, Reason: reason})
	return err
}

// TakeAlert hands over an issued alert not yet delivered; ok is false when it was taken already (or this
// server did not issue it: an alert is never sent twice, even after a restart).
func (e *Engine) TakeAlert(id string) (PublicAlert, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.alertOut[id] {
		return PublicAlert{}, false
	}
	delete(e.alertOut, id)
	p := e.st.alert(id)
	if p == nil {
		return PublicAlert{}, false
	}
	c := *p
	c.Results = nil
	return c, true
}

// UndeliveredAlerts are the alerts issued here and not yet taken.
func (e *Engine) UndeliveredAlerts() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for id := range e.alertOut {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// RecordAlertResults keeps how an alert's deliveries went.
func (e *Engine) RecordAlertResults(a Actor, id string, rs []PageResult, reached int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	p := e.st.alert(id)
	if p == nil {
		return problem("id", "no alert %s", id)
	}
	_, err := e.write(a, EvAlertResult, refs{Incident: p.Incident}, alertResults{ID: id, Results: rs, Reached: reached})
	return err
}

func (st *State) alert(id string) *PublicAlert {
	for i := len(st.Alerts) - 1; i >= 0; i-- {
		if st.Alerts[i].ID == id {
			return st.Alerts[i]
		}
	}
	return nil
}

// Alert finds an alert.
func (st *State) Alert(id string) *PublicAlert { return st.alert(id) }

// AlertsInForce are the alerts in force now, newest first.
func (st *State) AlertsInForce(now time.Time) []*PublicAlert {
	var out []*PublicAlert
	for i := len(st.Alerts) - 1; i >= 0; i-- {
		if st.Alerts[i].InForce(now) {
			out = append(out, st.Alerts[i])
		}
	}
	return out
}

// AlertRecipients are the people on an alert's lists to send it to, by gateway: everyone without a
// place, and those with one inside the area (or anywhere, for an alert to the whole jurisdiction).
func (st *State) AlertRecipients(p *PublicAlert) map[string][]Subscriber {
	out := map[string][]Subscriber{}
	seen := map[string]bool{}
	for _, id := range p.Lists {
		l := st.AlertLists[id]
		if l == nil {
			continue
		}
		for _, s := range l.People {
			if s.Lat != nil && len(p.Area.Polygon) >= 3 && !geo.InPolygon(*s.Lat, *s.Lon, p.Area.Polygon) {
				continue
			}
			for _, g := range l.Gateways {
				k := g + "|" + s.Phone + "|" + s.Email + "|" + s.Topic
				if seen[k] {
					continue
				}
				seen[k] = true
				out[g] = append(out[g], s)
			}
		}
	}
	return out
}

func (s *State) applyAlerts(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvAlertListSaved:
		l, err := decode[AlertList](e)
		if err != nil {
			return true, err
		}
		s.AlertLists[l.ID] = &l
	case EvAlertListRemoved:
		v, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		delete(s.AlertLists, v.ID)
	case EvAlertDrafted:
		p, err := decode[PublicAlert](e)
		if err != nil {
			return true, err
		}
		p.Created, p.By, p.State, p.Results = e.At, e.Actor, AlertPending, []PageResult{}
		s.Alerts = append(s.Alerts, &p)
		if len(s.Alerts) > keepAlerts {
			s.Alerts = slices.Clone(s.Alerts[len(s.Alerts)-keepAlerts:])
		}
		s.alertCount++
		if in := s.Incidents[p.Incident]; in != nil {
			in.add(e, "alert", fmt.Sprintf("Public alert %s written (%s): %s", p.ID, p.EventName, p.Text.Headline))
		}
	case EvAlertIssued:
		v, err := decode[alertIssued](e)
		if err != nil {
			return true, err
		}
		p := s.alert(v.ID)
		if p == nil {
			return true, nil
		}
		p.State, p.Sent, p.ApprovedBy = AlertIssued, e.At, v.Approver
		if of := s.alert(p.Of); of != nil {
			of.SupersededBy = p.ID
			of.State = map[bool]string{true: AlertCancelled, false: AlertUpdated}[p.MsgType == "Cancel"]
		}
		if in := s.Incidents[p.Incident]; in != nil {
			in.add(e, "alert", fmt.Sprintf("Public alert %s sent (%s %s): %s", p.ID, p.Status, p.MsgType, p.Text.Headline))
		}
	case EvAlertRejected:
		v, err := decode[alertRejected](e)
		if err != nil {
			return true, err
		}
		if p := s.alert(v.ID); p != nil {
			p.State, p.Rejected = AlertRejected, v.Reason
		}
	case EvAlertResult:
		v, err := decode[alertResults](e)
		if err != nil {
			return true, err
		}
		if p := s.alert(v.ID); p != nil {
			p.Results = append(p.Results, v.Results...)
			p.Reached += v.Reached
		}
	default:
		return false, nil
	}
	return true, nil
}

func firstNonEmptyStr(xs ...string) string {
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}

func trimRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
