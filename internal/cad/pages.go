package cad

// Paging responders: volunteer and off-duty firefighters and EMTs are called by page (a text message,
// an email, a push notification, or through services like IamResponding and Active911). Paging groups
// say who is paged and how; a group may be paged by hand or whenever one of its units or stations is
// dispatched, or an incident of one of its call types gets its first units. Each person paged gets a link
// to answer: responding to the station, going straight to the scene, or not available, with when they
// expect to arrive. The answers are on the incident for the dispatcher and the officer.

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvPageGroupSaved   = "pagegroup.saved"
	EvPageGroupRemoved = "pagegroup.removed"
	EvPageSent         = "page.sent"   // a page was sent to groups or people
	EvPageResult       = "page.result" // how each delivery went
	EvPageReply        = "page.reply"  // a person paged answered
)

// PageGroup is who is paged together, and how.
type PageGroup struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`               // "Station 3 volunteers"
	Members  []string `json:"members"`            // roster member IDs
	Gateways []string `json:"gateways"`           // the paging gateways to use (Admin › Integrations › Paging)
	Units    []string `json:"units,omitempty"`    // page when one of these units is dispatched
	Stations []string `json:"stations,omitempty"` // page when a unit of one of these stations is dispatched
	Types    []string `json:"types,omitempty"`    // page when an incident of one of these call types gets units
	Note     string   `json:"note,omitempty"`
	Off      bool     `json:"off,omitempty"` // paged by hand only
}

// Page is one page.
type Page struct {
	ID       string               `json:"id"`
	At       time.Time            `json:"at"`
	By       string               `json:"by"`
	Incident string               `json:"incident,omitempty"`
	Groups   []string             `json:"groups"`
	Subject  string               `json:"subject"`
	Text     string               `json:"text"`
	Auto     bool                 `json:"auto,omitempty"` // sent by a group's rule when units were dispatched
	To       []PageTo             `json:"to"`
	Results  []PageResult         `json:"results"`
	Replies  map[string]PageReply `json:"replies"` // by member
}

// PageTo is one person paged.
type PageTo struct {
	Member string `json:"member"`
	Name   string `json:"name"`
	token  string // the SHA-256 of their answer link's token
}

// PageResult is how one delivery went.
type PageResult struct {
	Gateway string    `json:"gateway"`
	Kind    string    `json:"kind"`
	To      string    `json:"to"` // masked
	OK      bool      `json:"ok"`
	Detail  string    `json:"detail,omitempty"`
	At      time.Time `json:"at"`
}

// Answers to a page.
const (
	PageResponding  = "responding"  // to the station
	PageToScene     = "scene"       // straight to the scene
	PageUnavailable = "unavailable" // not coming
)

// PageReply is a person's answer.
type PageReply struct {
	Answer string    `json:"answer"`
	ETA    int       `json:"eta,omitempty"` // minutes
	At     time.Time `json:"at"`
	Name   string    `json:"name"`
}

type pageSent struct {
	Page
	Tokens []string `json:"tokens"` // the SHA-256 of each person's answer token, in To's order
}

type pageResults struct {
	ID      string       `json:"id"`
	Results []PageResult `json:"results"`
}

type pageReply struct {
	ID     string `json:"id"`
	Member string `json:"member"`
	Answer string `json:"answer"`
	ETA    int    `json:"eta,omitempty"`
}

// keepPages is how many pages the live state keeps (the ledger keeps all of them).
const keepPages = 300

// SavePageGroup adds or changes a paging group.
func (e *Engine) SavePageGroup(a Actor, g PageGroup) (string, error) {
	g.Name = strings.TrimSpace(g.Name)
	if g.Name == "" {
		return "", problem("name", "give the group a name")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if g.ID == "" {
		g.ID = e.newID("GRP", func(id string) bool { return e.st.PageGroups[id] != nil })
	}
	g.Members = dedupe(g.Members)
	for _, m := range g.Members {
		if e.st.Members[m] == nil {
			return "", problem("members", "no member %s on the roster", m)
		}
	}
	for i, u := range g.Units {
		g.Units[i] = strings.ToUpper(u)
		if e.st.Units[g.Units[i]] == nil {
			return "", problem("units", "no unit %s", u)
		}
	}
	for _, st := range g.Stations {
		if e.st.Stations[st] == nil {
			return "", problem("stations", "no station %s", st)
		}
	}
	for _, t := range g.Types {
		if e.st.CallTypes[t] == nil {
			return "", problem("types", "no call type %s", t)
		}
	}
	if len(g.Gateways) == 0 {
		return "", problem("gateways", "choose how the group is paged (a paging gateway in Admin › Integrations › Paging)")
	}
	_, err := e.write(a, EvPageGroupSaved, refs{}, g)
	return g.ID, err
}

// RemovePageGroup removes a paging group.
func (e *Engine) RemovePageGroup(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.PageGroups[id] == nil {
		return problem("id", "no paging group %s", id)
	}
	_, err := e.write(a, EvPageGroupRemoved, refs{}, idData{id})
	return err
}

// PageRecipient is a person to deliver a page to, with their answer token.
type PageRecipient struct {
	Member Member
	Token  string
}

// SendPage records a page to groups and people, and returns who to deliver it to with each person's
// answer token (only its hash is kept). The delivery itself is the server's (internal/paging).
func (e *Engine) SendPage(a Actor, incident string, groups, members []string, subject, text string, auto bool) (string, []PageRecipient, error) {
	subject, text = strings.TrimSpace(subject), strings.TrimSpace(text)
	e.mu.Lock()
	defer e.mu.Unlock()
	if incident != "" && e.st.Incidents[incident] == nil {
		return "", nil, problem("incident", "no incident %s", incident)
	}
	if subject == "" && incident != "" {
		in := e.st.Incidents[incident]
		subject = strings.ToUpper(e.st.typeLabel(in.Type))
		if in.Location != nil && in.Location.Address != "" {
			subject += " " + in.Location.Address
		}
	}
	if subject == "" {
		return "", nil, problem("subject", "say what the page is about")
	}
	if len(subject) > 140 || len(text) > 1200 {
		return "", nil, problem("text", "a page is up to 140 characters of subject and 1,200 of text")
	}
	var who []string
	for _, g := range dedupe(groups) {
		pg := e.st.PageGroups[g]
		if pg == nil {
			return "", nil, problem("groups", "no paging group %s", g)
		}
		who = append(who, pg.Members...)
	}
	who = dedupe(append(who, members...))
	p := pageSent{Page: Page{ID: fmt.Sprintf("PG%s%d", e.sitePrefix, e.st.pageCount+1), Incident: incident, Groups: dedupe(groups), Subject: subject, Text: text, Auto: auto}}
	var out []PageRecipient
	for _, id := range who {
		m := e.st.Members[id]
		if m == nil {
			return "", nil, problem("members", "no member %s on the roster", id)
		}
		if m.Inactive {
			continue
		}
		tok := randomToken()
		p.To = append(p.To, PageTo{Member: m.ID, Name: m.Name})
		p.Tokens = append(p.Tokens, hashToken(tok))
		out = append(out, PageRecipient{Member: *m, Token: tok})
	}
	if len(p.Groups) == 0 && len(p.To) == 0 {
		return "", nil, problem("groups", "choose who to page")
	}
	if e.pageOut == nil {
		e.pageOut = map[string][]PageRecipient{}
	}
	e.pageOut[p.ID] = out // before the event, so a subscriber woken by it finds them
	if _, err := e.write(a, EvPageSent, refs{Incident: incident}, p); err != nil {
		delete(e.pageOut, p.ID)
		return "", nil, err
	}
	return p.ID, out, nil
}

// TakePage hands over a page written but not yet delivered, with whom to deliver it to; ok is false
// when it was taken already (or this server did not write it).
func (e *Engine) TakePage(id string) (Page, []PageRecipient, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	to, ok := e.pageOut[id]
	if !ok {
		return Page{}, nil, false
	}
	delete(e.pageOut, id)
	p := e.st.page(id)
	if p == nil {
		return Page{}, nil, false
	}
	c := *p
	c.Replies = nil
	return c, to, true
}

// RecordPageResults keeps how a page's deliveries went.
func (e *Engine) RecordPageResults(a Actor, id string, rs []PageResult) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	p := e.st.page(id)
	if p == nil {
		return problem("id", "no page %s", id)
	}
	_, err := e.write(a, EvPageResult, refs{Incident: p.Incident}, pageResults{ID: id, Results: rs})
	return err
}

// PageFor finds the page and person an answer link's token belongs to.
func (e *Engine) PageFor(id, token string) (Page, PageTo, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p := e.st.page(id)
	if p == nil || token == "" {
		return Page{}, PageTo{}, false
	}
	h := hashToken(token)
	for _, t := range p.To {
		if subtle.ConstantTimeCompare([]byte(t.token), []byte(h)) == 1 {
			c := *p
			c.Replies = nil
			return c, t, true
		}
	}
	return Page{}, PageTo{}, false
}

// ReplyPage records a person's answer to a page, by their answer link's token.
func (e *Engine) ReplyPage(id, token, answer string, eta int) error {
	switch answer {
	case PageResponding, PageToScene, PageUnavailable:
	default:
		return problem("answer", "answer responding, scene or unavailable")
	}
	if eta < 0 || eta > 240 {
		return problem("eta", "minutes to arrive are 0 to 240")
	}
	_, to, ok := e.PageFor(id, token)
	if !ok {
		return problem("token", "this page link is not valid")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	p := e.st.page(id)
	if answer == PageUnavailable {
		eta = 0
	}
	_, err := e.write(Actor{User: "page:" + to.Member, Role: "responder"}, EvPageReply, refs{Incident: p.Incident}, pageReply{ID: id, Member: to.Member, Answer: answer, ETA: eta})
	return err
}

func (s *State) page(id string) *Page {
	for i := len(s.Pages) - 1; i >= 0; i-- {
		if s.Pages[i].ID == id {
			return s.Pages[i]
		}
	}
	return nil
}

// PagesFor are an incident's pages, oldest first.
func (s *State) PagesFor(incident string) []*Page {
	var out []*Page
	for _, p := range s.Pages {
		if p.Incident == incident {
			out = append(out, p)
		}
	}
	return out
}

// GroupsToPage are the paging groups whose rules match units dispatched to an incident: one of the
// units, a station of one, or the incident's call type.
func (s *State) GroupsToPage(incident string, units []string) []string {
	in := s.Incidents[incident]
	if in == nil {
		return nil
	}
	var out []string
	for _, g := range s.PageGroups {
		if g.Off {
			continue
		}
		hit := slices.Contains(g.Types, in.Type)
		for _, id := range units {
			u := s.Units[id]
			if u == nil {
				continue
			}
			if slices.Contains(g.Units, id) || (u.Station != "" && slices.Contains(g.Stations, u.Station)) {
				hit = true
			}
		}
		if hit {
			out = append(out, g.ID)
		}
	}
	slices.Sort(out)
	return out
}

func (s *State) applyPages(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvPageGroupSaved:
		g, err := decode[PageGroup](e)
		if err != nil {
			return true, err
		}
		s.PageGroups[g.ID] = &g
	case EvPageGroupRemoved:
		v, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		delete(s.PageGroups, v.ID)
	case EvPageSent:
		v, err := decode[pageSent](e)
		if err != nil {
			return true, err
		}
		p := v.Page
		p.At, p.By, p.Results, p.Replies = e.At, e.Actor, []PageResult{}, map[string]PageReply{}
		for i := range p.To {
			if i < len(v.Tokens) {
				p.To[i].token = v.Tokens[i]
			}
		}
		s.Pages = append(s.Pages, &p)
		if len(s.Pages) > keepPages {
			s.Pages = slices.Clone(s.Pages[len(s.Pages)-keepPages:])
		}
		s.pageCount++
		if in := s.Incidents[p.Incident]; in != nil {
			var names []string
			for _, g := range p.Groups {
				if pg := s.PageGroups[g]; pg != nil {
					names = append(names, pg.Name)
				}
			}
			who := strings.Join(names, ", ")
			if who == "" {
				who = fmt.Sprintf("%d people", len(p.To))
			}
			how := "Paged "
			if p.Auto {
				how = "Paged automatically "
			}
			in.add(e, "page", fmt.Sprintf("%s%s (%d people): %s", how, who, len(p.To), p.Subject))
		}
	case EvPageResult:
		v, err := decode[pageResults](e)
		if err != nil {
			return true, err
		}
		if p := s.page(v.ID); p != nil {
			p.Results = append(p.Results, v.Results...)
			bad := 0
			for _, r := range v.Results {
				if !r.OK {
					bad++
				}
			}
			if in := s.Incidents[p.Incident]; in != nil && bad > 0 {
				in.add(e, "page", fmt.Sprintf("Page %s: %d of %d deliveries failed", p.ID, bad, len(v.Results)))
			}
		}
	case EvPageReply:
		v, err := decode[pageReply](e)
		if err != nil {
			return true, err
		}
		p := s.page(v.ID)
		if p == nil {
			return true, nil
		}
		name := v.Member
		for _, t := range p.To {
			if t.Member == v.Member {
				name = t.Name
			}
		}
		p.Replies[v.Member] = PageReply{Answer: v.Answer, ETA: v.ETA, At: e.At, Name: name}
		if in := s.Incidents[p.Incident]; in != nil {
			text := name + " " + map[string]string{PageResponding: "responding to the station", PageToScene: "responding to the scene", PageUnavailable: "not available"}[v.Answer]
			if v.ETA > 0 {
				text += fmt.Sprintf(", %d min", v.ETA)
			}
			in.add(e, "page", text)
		}
	default:
		return false, nil
	}
	return true, nil
}

func randomToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

func dedupe(xs []string) []string {
	var out []string
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x != "" && !slices.Contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}

// newID makes a short ID with a prefix that taken does not report as used.
func (e *Engine) newID(prefix string, taken func(string) bool) string {
	for i := 1; ; i++ {
		id := fmt.Sprintf("%s%s%d", prefix, e.sitePrefix, i)
		if !taken(id) {
			return id
		}
	}
}
