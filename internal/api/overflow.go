package api

// Overflow (cad/continuity.go, docs/OVERFLOW.md): a neighbouring centre running bitdispatcher takes calls
// for this one while it is overwhelmed or out of its building, or this centre takes theirs. The asking
// centre's supervisor asks; the helping centre's supervisor accepts (or its settings accept on their own).
// While it lasts, the helper's call takers enter calls on a form of the asking centre's call types; each
// call becomes an incident in the asking centre's CAD, under its numbering, and the asking centre's
// dispatchers send its units, unless it let the helper assign them too. Status, units, notes and the
// close come back to the helper for the calls it took, and nothing else: no other incidents, no records,
// no NCIC returns. A call the asking centre cannot be reached for is kept and sent when it can.
//
// Each neighbour is a partner (partners.go) with the address of its CAD-to-CAD receiver,
// https://their-cad/api/cad/eido/<our ID there>, and a token each way; overflow uses the same address's
// server and tokens.

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// PartnerOverflow is what overflow this centre has agreed with a neighbour (Admin › Continuity).
type PartnerOverflow struct {
	Ask        bool     `json:"ask"`        // we may ask them to take our calls
	Help       bool     `json:"help"`       // we take their calls when they ask
	AutoAccept bool     `json:"autoAccept"` // their request is accepted without our supervisor
	Dispatch   bool     `json:"dispatch"`   // while they take our calls, they may also assign our units
	Agencies   []string `json:"agencies,omitempty"`
	CallTypes  []string `json:"callTypes,omitempty"` // our call types they may use (empty: all)
}

// overflowBase is the neighbour's server and the ID it knows us by, from its CAD-to-CAD address.
func (p Partner) overflowBase() (base, us string, err error) {
	u, err := url.Parse(p.URL)
	if err != nil || u.Host == "" {
		return "", "", errors.New("the neighbour has no address (Admin › Integrations › Neighbouring agencies)")
	}
	i := strings.Index(u.Path, "/api/cad/eido/")
	if i < 0 {
		return "", "", errors.New("the neighbour's address is not a bitdispatcher's (…/api/cad/eido/<our ID>); overflow needs bitdispatcher at both centres")
	}
	us = strings.Trim(u.Path[i+len("/api/cad/eido/"):], "/")
	if !rePartner.MatchString(us) {
		return "", "", errors.New("the neighbour's address does not end with the ID it knows us by")
	}
	return u.Scheme + "://" + u.Host + u.Path[:i], us, nil
}

// overflowRuntime is the pushes owed to neighbours: the incidents they took whose news has not gone yet.
type overflowRuntime struct {
	mu    sync.Mutex
	dirty map[string]bool // our incident numbers
	kick  chan struct{}
}

var reRef = regexp.MustCompile(`^[A-F0-9]{12}$`)

var overflowClient = &http.Client{Timeout: 15 * time.Second}

// client is how a neighbour is reached: pinned to its certificate when one is set.
func (p Partner) client() *http.Client {
	if p.Fingerprint != "" {
		return pinnedClient(p.Fingerprint)
	}
	return overflowClient
}

func (s *Server) partner(id string) (Partner, bool) {
	for _, p := range s.partners() {
		if p.ID == id {
			return p, true
		}
	}
	return Partner{}, false
}

// send posts to a neighbour's overflow address and decodes its answer.
func (s *Server) overflowSend(ctx context.Context, p Partner, method, path string, body, out any) error {
	base, us, err := p.overflowBase()
	if err != nil {
		return err
	}
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+"/api/overflow-link/"+us+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.OutToken)
	resp, err := p.client().Do(req)
	if err != nil {
		return fmt.Errorf("%s's CAD cannot be reached: %w", p.Name, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 300 {
		var e cad.Problem
		json.Unmarshal(raw, &e)
		return fmt.Errorf("%s's CAD refused: %s", p.Name, firstNonEmptyStr(e.Message, resp.Status))
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// fromPartner authenticates a neighbour's call by its token.
func (s *Server) fromPartner(w http.ResponseWriter, r *http.Request) (Partner, bool) {
	id := strings.ToUpper(r.PathValue("from"))
	p, ok := s.partner(id)
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || p.InToken == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(p.InToken)) != 1 {
		fail(w, 401, "unknown agency or wrong token")
		return Partner{}, false
	}
	if !p.Overflow.Ask && !p.Overflow.Help {
		fail(w, 403, "overflow is not set up with "+p.Name+" here")
		return Partner{}, false
	}
	return p, true
}

func newRef() string {
	b := make([]byte, 6)
	rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}

// overflowCallTypes are the call types a neighbour may use for us.
func overflowCallTypes(st *cad.State, allowed []string) []cad.OverflowType {
	out := []cad.OverflowType{}
	for _, ct := range st.CallTypes {
		if len(allowed) == 0 || slices.Contains(allowed, ct.Code) {
			out = append(out, cad.OverflowType{Code: ct.Code, Name: ct.Name, Priority: ct.Priority})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// sessionMsg is a session as one centre tells the other: Direction is the sender's.
type sessionMsg struct {
	Direction string             `json:"direction"`
	Status    string             `json:"status"`
	Note      string             `json:"note,omitempty"`
	By        string             `json:"by"`
	Until     time.Time          `json:"until"`
	Dispatch  bool               `json:"dispatch"`
	Agencies  []string           `json:"agencies,omitempty"`
	CallTypes []cad.OverflowType `json:"callTypes,omitempty"`
	Name      string             `json:"name,omitempty"` // the sender's agency name
}

// callMsg is a call a helper took, sent to the asking centre.
type callMsg struct {
	Ref        string   `json:"ref"`
	Type       string   `json:"type"`
	Priority   int      `json:"priority"`
	Address    string   `json:"address"`
	Lat        *float64 `json:"lat,omitempty"`
	Lon        *float64 `json:"lon,omitempty"`
	CallerName string   `json:"callerName,omitempty"`
	Callback   string   `json:"callback,omitempty"`
	Narrative  string   `json:"narrative,omitempty"`
	By         string   `json:"by"`
}

// updateMsg is what became of a call, sent back to the helper.
type updateMsg struct {
	Ref         string             `json:"ref"`
	Number      string             `json:"number"`
	Status      string             `json:"status"`
	Units       []cad.OverflowUnit `json:"units"`
	Closed      bool               `json:"closed"`
	Disposition string             `json:"disposition,omitempty"`
	Notes       []cad.OverflowNote `json:"notes,omitempty"`
}

func flip(d string) string {
	if d == "out" {
		return "in"
	}
	return "out"
}

func (s *Server) overflowRoutes() {
	// ---- what neighbours call ----
	s.mux.HandleFunc("GET /api/overflow-link/{from}/ping", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.fromPartner(w, r); !ok {
			return
		}
		var name string
		s.Eng.Read(func(st *cad.State) { name = st.Settings.Agency })
		writeJSON(w, 200, map[string]any{"ok": true, "name": name})
	})
	s.mux.HandleFunc("POST /api/overflow-link/{from}/session", func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.fromPartner(w, r)
		if !ok {
			return
		}
		var m sessionMsg
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&m); err != nil {
			fail(w, 400, err.Error())
			return
		}
		status, err := s.takeSession(p, m)
		if err != nil {
			writeOvf(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"status": status})
	})
	s.mux.HandleFunc("POST /api/overflow-link/{from}/calls", func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.fromPartner(w, r)
		if !ok {
			return
		}
		var m callMsg
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&m); err != nil {
			fail(w, 400, err.Error())
			return
		}
		number, err := s.takeCall(p, m)
		if err != nil {
			writeOvf(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"number": number})
	})
	s.mux.HandleFunc("POST /api/overflow-link/{from}/calls/{ref}/notes", func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.fromPartner(w, r)
		if !ok {
			return
		}
		var m struct{ Text, By string }
		if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&m); err != nil {
			fail(w, 400, err.Error())
			return
		}
		number, a, err := s.overflowIncident(p, r.PathValue("ref"), m.By)
		if err == nil {
			err = s.Eng.AddNote(a, number, fmt.Sprintf("[%s, %s] %s", p.Name, m.By, strings.TrimSpace(m.Text)), false)
		}
		if err != nil {
			writeOvf(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.mux.HandleFunc("POST /api/overflow-link/{from}/calls/{ref}/dispatch", func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.fromPartner(w, r)
		if !ok {
			return
		}
		var m struct {
			Units []string `json:"units"`
			By    string   `json:"by"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&m); err != nil {
			fail(w, 400, err.Error())
			return
		}
		var sess *cad.OverflowSession
		s.Eng.Read(func(st *cad.State) {
			if o := st.OverflowFor("out", p.ID); o != nil {
				c := *o
				sess = &c
			}
		})
		if sess == nil || !sess.Live(s.Eng.Now()) || !sess.Dispatch || !p.Overflow.Dispatch {
			fail(w, 403, "assigning our units is not part of this overflow")
			return
		}
		number, a, err := s.overflowIncident(p, r.PathValue("ref"), m.By)
		if err == nil {
			for _, u := range m.Units {
				if !s.overflowUnitAllowed(p, strings.ToUpper(u)) {
					err = &cad.Problem{Field: "units", Message: u + " is not one of the units shared for overflow"}
					break
				}
			}
		}
		if err == nil {
			err = s.Eng.DispatchWhy(a, number, m.Units, nil, "assigned by "+p.Name+" during overflow", "")
		}
		if err != nil {
			writeOvf(w, err)
			return
		}
		s.markDirty(number)
		w.WriteHeader(http.StatusNoContent)
	})
	s.mux.HandleFunc("GET /api/overflow-link/{from}/units", func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.fromPartner(w, r)
		if !ok {
			return
		}
		var sess *cad.OverflowSession
		out := []map[string]string{}
		s.Eng.Read(func(st *cad.State) {
			sess = st.OverflowFor("out", p.ID)
			if sess == nil || !sess.Dispatch {
				return
			}
			for _, u := range st.Units {
				if len(p.Overflow.Agencies) == 0 || slices.Contains(p.Overflow.Agencies, u.Agency) {
					out = append(out, map[string]string{"id": u.ID, "type": u.Type, "station": u.Station, "status": u.Status, "incident": u.Incident})
				}
			}
		})
		if sess == nil || !sess.Live(s.Eng.Now()) || !sess.Dispatch || !p.Overflow.Dispatch {
			fail(w, 403, "our units are not shared in this overflow")
			return
		}
		sort.Slice(out, func(i, j int) bool { return unitLess(out[i]["id"], out[j]["id"]) })
		writeJSON(w, 200, out)
	})
	s.mux.HandleFunc("POST /api/overflow-link/{from}/updates", func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.fromPartner(w, r)
		if !ok {
			return
		}
		var m updateMsg
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&m); err != nil {
			fail(w, 400, err.Error())
			return
		}
		c, ok := s.Eng.OverflowCallByRef(m.Ref)
		if !ok || c.Partner != p.ID {
			fail(w, 404, "no call "+m.Ref+" taken for "+p.Name)
			return
		}
		c.Number, c.Status, c.Units, c.Closed, c.Disposition = m.Number, m.Status, m.Units, m.Closed, m.Disposition
		for _, n := range m.Notes {
			n.Theirs, n.Delivered = true, true
			if !slices.ContainsFunc(c.Notes, func(x cad.OverflowNote) bool { return x.Theirs && x.At.Equal(n.At) && x.Text == n.Text }) {
				c.Notes = append(c.Notes, n)
			}
		}
		if err := s.Eng.SaveOverflowCall(cad.Actor{User: "overflow:" + p.ID}, c); err != nil {
			writeOvf(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// ---- what our people use ----
	s.handle("GET /api/overflow", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		ps := []map[string]any{}
		for _, p := range s.partners() {
			if !p.Overflow.Ask && !p.Overflow.Help {
				continue
			}
			_, _, err := p.overflowBase()
			ps = append(ps, map[string]any{"id": p.ID, "name": p.Name, "ask": p.Overflow.Ask, "help": p.Overflow.Help, "dispatch": p.Overflow.Dispatch,
				"autoAccept": p.Overflow.AutoAccept, "problem": errText(err)})
		}
		out := map[string]any{"partners": ps, "supervisor": isSupervisor(a)}
		s.Eng.Read(func(st *cad.State) {
			sessions := []*cad.OverflowSession{}
			for _, o := range st.Overflows {
				sessions = append(sessions, o)
			}
			sort.Slice(sessions, func(i, j int) bool { return sessions[i].Since.After(sessions[j].Since) })
			calls := []*cad.OverflowCall{}
			for _, c := range st.OverflowCalls {
				if s.Eng.Now().Sub(c.Taken) < 72*time.Hour {
					calls = append(calls, c)
				}
			}
			sort.Slice(calls, func(i, j int) bool { return calls[i].Taken.After(calls[j].Taken) })
			theirs := []map[string]any{}
			for n, o := range st.OverflowIncidents {
				if in := st.Incidents[n]; in != nil && (in.Status != cad.Closed || s.Eng.Now().Sub(in.Closed) < 12*time.Hour) {
					theirs = append(theirs, map[string]any{"number": n, "partner": o.Partner, "by": o.By, "type": in.Type, "status": in.Status, "created": in.Created})
				}
			}
			sort.Slice(theirs, func(i, j int) bool { return theirs[i]["number"].(string) > theirs[j]["number"].(string) })
			out["sessions"], out["calls"], out["takenForUs"] = sessions, calls, theirs
			out["minutes"] = st.Settings.Continuity.OverflowMinutesOr()
		})
		writeJSON(w, 200, out)
	})
	s.handle("POST /api/overflow/{partner}/ask", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Note    string `json:"note"`
			Minutes int    `json:"minutes"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		p, err := s.supervisorPartner(a, r.PathValue("partner"))
		if err == nil && !p.Overflow.Ask {
			err = &cad.Problem{Field: "partner", Message: "asking " + p.Name + " to take our calls is not set up (Admin › Continuity)"}
		}
		if err != nil {
			writeOvf(w, err)
			return
		}
		o := cad.OverflowSession{Partner: p.ID, PartnerName: p.Name, Direction: "out", Status: "asked", Note: strings.TrimSpace(b.Note), By: a.User,
			Since: s.Eng.Now(), Dispatch: p.Overflow.Dispatch, Agencies: p.Overflow.Agencies}
		var agency string
		s.Eng.Read(func(st *cad.State) {
			mins := b.Minutes
			if mins <= 0 || mins > 7*24*60 {
				mins = st.Settings.Continuity.OverflowMinutesOr()
			}
			o.Until = o.Since.Add(time.Duration(mins) * time.Minute)
			o.CallTypes = overflowCallTypes(st, p.Overflow.CallTypes)
			agency = st.Settings.Agency
		})
		var ans struct {
			Status string `json:"status"`
		}
		if err := s.overflowSend(r.Context(), p, "POST", "/session", sessionMsg{Direction: "out", Status: "asked", Note: o.Note, By: a.User, Until: o.Until,
			Dispatch: o.Dispatch, Agencies: o.Agencies, CallTypes: o.CallTypes, Name: agency}, &ans); err != nil {
			fail(w, 502, err.Error())
			return
		}
		if ans.Status == "active" {
			o.Status, o.Answered, o.AnsweredBy = "active", s.Eng.Now(), p.Name+" (accepted by its settings)"
		}
		if err := s.Eng.SaveOverflowSession(a, o); err != nil {
			writeOvf(w, err)
			return
		}
		writeJSON(w, 200, o)
	})
	s.handle("POST /api/overflow/{partner}/answer", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Accept bool `json:"accept"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		p, err := s.supervisorPartner(a, r.PathValue("partner"))
		if err != nil {
			writeOvf(w, err)
			return
		}
		var o cad.OverflowSession
		found := false
		s.Eng.Read(func(st *cad.State) {
			if x := st.OverflowFor("in", p.ID); x != nil && x.Status == "asked" {
				o, found = *x, true
			}
		})
		if !found {
			fail(w, 409, p.Name+" has not asked us to take their calls")
			return
		}
		o.Status, o.Answered, o.AnsweredBy = map[bool]string{true: "active", false: "declined"}[b.Accept], s.Eng.Now(), a.User
		if err := s.overflowSend(r.Context(), p, "POST", "/session", sessionMsg{Direction: "in", Status: o.Status, By: a.User, Until: o.Until}, nil); err != nil {
			fail(w, 502, err.Error())
			return
		}
		if err := s.Eng.SaveOverflowSession(a, o); err != nil {
			writeOvf(w, err)
			return
		}
		writeJSON(w, 200, o)
	})
	s.handle("POST /api/overflow/{partner}/end", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Direction string `json:"direction"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		p, err := s.supervisorPartner(a, r.PathValue("partner"))
		if err != nil {
			writeOvf(w, err)
			return
		}
		var o cad.OverflowSession
		found := false
		s.Eng.Read(func(st *cad.State) {
			if x := st.OverflowFor(b.Direction, p.ID); x != nil && (x.Status == "asked" || x.Status == "active") {
				o, found = *x, true
			}
		})
		if !found {
			fail(w, 409, "no overflow with "+p.Name+" to end")
			return
		}
		o.Status, o.Ended, o.EndedBy = "ended", s.Eng.Now(), a.User
		// Ended here whatever the neighbour says; one that cannot be reached is told by phone.
		sendErr := s.overflowSend(r.Context(), p, "POST", "/session", sessionMsg{Direction: o.Direction, Status: "ended", By: a.User, Until: o.Until}, nil)
		if err := s.Eng.SaveOverflowSession(a, o); err != nil {
			writeOvf(w, err)
			return
		}
		out := map[string]any{"session": o}
		if sendErr != nil {
			out["warning"] = "Ended here, but " + sendErr.Error() + ". Tell them by phone."
		}
		writeJSON(w, 200, out)
	})
	s.handle("POST /api/overflow/{partner}/calls", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var m callMsg
		if !readJSON(w, r, &m) {
			return
		}
		id := strings.ToUpper(r.PathValue("partner"))
		p, ok := s.partner(id)
		var sess *cad.OverflowSession
		s.Eng.Read(func(st *cad.State) {
			if o := st.OverflowFor("in", id); o != nil {
				c := *o
				sess = &c
			}
		})
		if !ok || sess == nil || !sess.Live(s.Eng.Now()) {
			fail(w, 409, "we are not taking calls for "+firstNonEmptyStr(p.Name, id)+" now")
			return
		}
		var tn string
		for _, t := range sess.CallTypes {
			if t.Code == m.Type {
				tn = t.Name
				if m.Priority == 0 {
					m.Priority = t.Priority
				}
			}
		}
		switch {
		case tn == "":
			fail(w, 422, "choose one of "+p.Name+"'s call types")
			return
		case strings.TrimSpace(m.Address) == "" && (m.Lat == nil || m.Lon == nil):
			fail(w, 422, "give the location")
			return
		case m.Priority < 1 || m.Priority > 5:
			fail(w, 422, "priority is 1 (highest) to 5")
			return
		}
		c := cad.OverflowCall{Ref: newRef(), Partner: id, Taken: s.Eng.Now(), By: a.User, Type: m.Type, TypeName: tn, Priority: m.Priority,
			Address: strings.TrimSpace(m.Address), Lat: m.Lat, Lon: m.Lon, CallerName: strings.TrimSpace(m.CallerName), Callback: strings.TrimSpace(m.Callback),
			Narrative: strings.TrimSpace(m.Narrative)}
		if err := s.Eng.SaveOverflowCall(a, c); err != nil {
			writeOvf(w, err)
			return
		}
		c = s.deliverCall(r.Context(), p, c)
		writeJSON(w, 200, c)
	})
	s.handle("POST /api/overflow/calls/{ref}/notes", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Text string `json:"text"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		c, ok := s.Eng.OverflowCallByRef(r.PathValue("ref"))
		if !ok {
			fail(w, 404, "no such overflow call")
			return
		}
		if t := strings.TrimSpace(b.Text); t == "" || len(t) > 2000 {
			fail(w, 422, "write the note, up to 2000 characters")
			return
		}
		c.Notes = append(c.Notes, cad.OverflowNote{At: s.Eng.Now(), By: a.User, Text: strings.TrimSpace(b.Text)})
		if err := s.Eng.SaveOverflowCall(a, c); err != nil {
			writeOvf(w, err)
			return
		}
		if p, ok := s.partner(c.Partner); ok {
			c = s.deliverCall(r.Context(), p, c)
		}
		writeJSON(w, 200, c)
	})
	s.handle("GET /api/overflow/{partner}/units", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		p, ok := s.partner(strings.ToUpper(r.PathValue("partner")))
		if !ok {
			fail(w, 404, "no such neighbour")
			return
		}
		var out []map[string]string
		if err := s.overflowSend(r.Context(), p, "GET", "/units", nil, &out); err != nil {
			fail(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, out)
	})
	s.handle("POST /api/overflow/calls/{ref}/dispatch", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Units []string `json:"units"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		c, ok := s.Eng.OverflowCallByRef(r.PathValue("ref"))
		p, pok := s.partner(c.Partner)
		if !ok || !pok {
			fail(w, 404, "no such overflow call")
			return
		}
		if !c.Delivered {
			fail(w, 409, p.Name+" has not received this call yet: assign their units by radio, as your agreement says")
			return
		}
		if err := s.overflowSend(r.Context(), p, "POST", "/calls/"+c.Ref+"/dispatch", map[string]any{"units": b.Units, "by": a.User}, nil); err != nil {
			fail(w, 502, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("PUT /api/admin/partners/{id}/overflow", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b PartnerOverflow
		if !readJSON(w, r, &b) {
			return
		}
		id := strings.ToUpper(r.PathValue("id"))
		p, ok := s.partner(id)
		if !ok {
			fail(w, 404, "no neighbouring agency "+id)
			return
		}
		if b.Ask || b.Help {
			if _, _, err := p.overflowBase(); err != nil {
				fail(w, 422, err.Error())
				return
			}
			if p.InToken == "" || p.OutToken == "" {
				fail(w, 422, "overflow needs both tokens set for "+p.Name)
				return
			}
		}
		if len(b.Agencies) > 50 || len(b.CallTypes) > 500 {
			fail(w, 422, "too many agencies or call types")
			return
		}
		if err := s.updatePartners(func(ps []Partner) []Partner {
			for i := range ps {
				if ps[i].ID == id {
					ps[i].Overflow = b
				}
			}
			return ps
		}); err != nil {
			writeOvf(w, err)
			return
		}
		s.Eng.Record(a, cad.EvIntegration, map[string]any{"partner": id, "overflow": b})
		w.WriteHeader(http.StatusNoContent)
	})
}

// httpErr is a refusal with its HTTP status.
type httpErr struct {
	code int
	msg  string
}

func (e httpErr) Error() string { return e.msg }

func writeOvf(w http.ResponseWriter, err error) {
	var h httpErr
	if errors.As(err, &h) {
		fail(w, h.code, h.msg)
		return
	}
	writeErr(w, err)
}

func (s *Server) supervisorPartner(a cad.Actor, id string) (Partner, error) {
	if !isSupervisor(a) {
		return Partner{}, httpErr{403, "a supervisor starts, answers and ends overflow"}
	}
	p, ok := s.partner(strings.ToUpper(id))
	if !ok {
		return Partner{}, httpErr{404, "no neighbouring agency " + id}
	}
	return p, nil
}

// takeSession records what a neighbour says of an overflow and says what it now is here.
func (s *Server) takeSession(p Partner, m sessionMsg) (string, error) {
	now := s.Eng.Now()
	dir := flip(m.Direction) // their out is our in
	var cur *cad.OverflowSession
	var minutes int
	s.Eng.Read(func(st *cad.State) {
		if o := st.OverflowFor(dir, p.ID); o != nil {
			c := *o
			cur = &c
		}
		minutes = st.Settings.Continuity.OverflowMinutesOr()
	})
	actor := cad.Actor{User: "overflow:" + p.ID}
	switch {
	case m.Direction == "out" && m.Status == "asked":
		if !p.Overflow.Help {
			return "", httpErr{403, "this centre does not take " + p.Name + "'s calls (its settings)"}
		}
		until := m.Until
		if until.Before(now) || until.After(now.Add(7*24*time.Hour)) {
			until = now.Add(time.Duration(minutes) * time.Minute)
		}
		o := cad.OverflowSession{Partner: p.ID, PartnerName: p.Name, Direction: "in", Status: "asked", Note: m.Note, By: m.By + " (" + p.Name + ")", Since: now, Until: until,
			Dispatch: m.Dispatch, Agencies: m.Agencies, CallTypes: m.CallTypes}
		if p.Overflow.AutoAccept {
			o.Status, o.Answered, o.AnsweredBy = "active", now, "accepted by our settings"
		}
		if err := s.Eng.SaveOverflowSession(actor, o); err != nil {
			return "", err
		}
		body := fmt.Sprintf("%s asks us to take their calls until %s. %s", p.Name, until.Local().Format("15:04"), m.Note)
		if o.Status == "active" {
			body = fmt.Sprintf("%s asked us to take their calls until %s; our settings accepted. Call takers: Overflow, Take a call for %s. %s", p.Name, until.Local().Format("15:04"), p.Name, m.Note)
		}
		s.Eng.SendComm(cad.SystemActor, cad.Comm{Kind: "admin", Priority: "urgent", Subject: "Overflow: " + p.Name + " asks for help", Body: body,
			To: []string{cad.ToDuty + "supervisor", cad.ToDuty + "calltaker", cad.ToDuty + "dispatcher"}})
		return o.Status, nil
	case cur == nil:
		return "", httpErr{409, "no overflow with " + p.Name + " here"}
	case m.Status == "active" || m.Status == "declined":
		if cur.Status != "asked" || dir != "out" {
			return cur.Status, nil
		}
		cur.Status, cur.Answered, cur.AnsweredBy = m.Status, now, m.By+" ("+p.Name+")"
	case m.Status == "ended":
		if cur.Status == "ended" {
			return cur.Status, nil
		}
		cur.Status, cur.Ended, cur.EndedBy = "ended", now, m.By+" ("+p.Name+")"
	default:
		return "", &cad.Problem{Field: "status", Message: "an overflow is asked, active, declined or ended"}
	}
	if err := s.Eng.SaveOverflowSession(actor, *cur); err != nil {
		return "", err
	}
	s.Eng.SendComm(cad.SystemActor, cad.Comm{Kind: "admin", Priority: "high", Subject: "Overflow with " + p.Name + ": " + cur.Status,
		Body: fmt.Sprintf("%s: the overflow is %s.", cur.EndedBy+cur.AnsweredBy, cur.Status), To: []string{cad.ToDuty + "supervisor", cad.ToDuty + "calltaker"}})
	return cur.Status, nil
}

// takeCall creates an incident for a call a neighbour took for us.
func (s *Server) takeCall(p Partner, m callMsg) (string, error) {
	var sess *cad.OverflowSession
	s.Eng.Read(func(st *cad.State) {
		if o := st.OverflowFor("out", p.ID); o != nil {
			c := *o
			sess = &c
		}
	})
	if sess == nil || !sess.Live(s.Eng.Now()) {
		// A call taken while the overflow was live and delivered late (our CAD was unreachable) is still
		// ours: refusing it would lose it. Only a call for an overflow never started is refused.
		var exists bool
		s.Eng.Read(func(st *cad.State) { exists = st.OverflowByRef(p.ID, m.Ref) != nil })
		if sess == nil && !exists {
			return "", httpErr{403, "we did not ask " + p.Name + " to take our calls"}
		}
	}
	if !reRef.MatchString(m.Ref) {
		return "", &cad.Problem{Field: "ref", Message: "a call's reference is 12 characters"}
	}
	if len(p.Overflow.CallTypes) > 0 && !slices.Contains(p.Overflow.CallTypes, strings.ToUpper(m.Type)) {
		return "", &cad.Problem{Field: "type", Message: m.Type + " is not a call type shared for overflow"}
	}
	by := strings.Map(func(r rune) rune {
		if r < ' ' || r == ':' {
			return -1
		}
		return r
	}, m.By)
	a := cad.Actor{User: p.ID + ":" + firstNonEmptyStr(by, "calltaker"), Role: auth.Dispatcher, Duty: "calltaker"}
	if sess != nil && len(sess.Agencies) == 1 {
		a.Agency = sess.Agencies[0]
	}
	loc := &ng911.Location{Address: strings.TrimSpace(m.Address), Lat: m.Lat, Lon: m.Lon, Source: "overflow:" + p.ID}
	n := cad.NewIncident{Type: m.Type, Priority: m.Priority, Location: loc, Narrative: m.Narrative, Agency: a.Agency}
	if m.CallerName != "" || m.Callback != "" {
		n.Caller = &cad.Caller{Name: m.CallerName, Callback: m.Callback}
	}
	number, created, err := s.Eng.CreateOverflowIncident(a, p.ID, p.Name, m.Ref, n)
	if err != nil {
		return "", err
	}
	if created {
		s.markDirty(number)
	}
	return number, nil
}

// overflowIncident finds our incident for a neighbour's call and the actor to write as.
func (s *Server) overflowIncident(p Partner, ref, by string) (string, cad.Actor, error) {
	var number string
	s.Eng.Read(func(st *cad.State) {
		if o := st.OverflowByRef(p.ID, ref); o != nil {
			number = o.Number
		}
	})
	if number == "" {
		return "", cad.Actor{}, httpErr{404, "no incident for " + p.Name + "'s call " + ref}
	}
	by = strings.Map(func(r rune) rune {
		if r < ' ' || r == ':' {
			return -1
		}
		return r
	}, by)
	return number, cad.Actor{User: p.ID + ":" + firstNonEmptyStr(by, "calltaker"), Role: auth.Dispatcher, Duty: "dispatcher"}, nil
}

func (s *Server) overflowUnitAllowed(p Partner, unit string) bool {
	ok := false
	s.Eng.Read(func(st *cad.State) {
		u := st.Units[unit]
		ok = u != nil && (len(p.Overflow.Agencies) == 0 || slices.Contains(p.Overflow.Agencies, u.Agency))
	})
	return ok
}

// deliverCall sends a call, and its notes not yet sent, to the centre it was taken for. A failure is kept
// on the call and tried again by the outbox.
func (s *Server) deliverCall(ctx context.Context, p Partner, c cad.OverflowCall) cad.OverflowCall {
	before := c.Error
	changed := false
	if !c.Delivered {
		var ans struct {
			Number string `json:"number"`
		}
		err := s.overflowSend(ctx, p, "POST", "/calls", callMsg{Ref: c.Ref, Type: c.Type, Priority: c.Priority, Address: c.Address, Lat: c.Lat, Lon: c.Lon,
			CallerName: c.CallerName, Callback: c.Callback, Narrative: c.Narrative, By: c.By}, &ans)
		c.Tries++
		if err != nil {
			c.Error = err.Error()
		} else {
			c.Delivered, c.Number, c.Error, changed = true, ans.Number, "", true
		}
	}
	if c.Delivered {
		for i := range c.Notes {
			n := &c.Notes[i]
			if n.Delivered || n.Theirs {
				continue
			}
			if err := s.overflowSend(ctx, p, "POST", "/calls/"+c.Ref+"/notes", map[string]string{"text": n.Text, "by": n.By}, nil); err != nil {
				c.Error = err.Error()
				break
			}
			n.Delivered, changed = true, true
		}
	}
	if changed || c.Error != before {
		s.Eng.SaveOverflowCall(cad.Actor{User: "overflow"}, c)
	}
	return c
}

// overflowOutbox tries again every 20 seconds the calls and notes not yet delivered.
func (s *Server) overflowOutbox(ctx context.Context) {
	if s.isTraining {
		return
	}
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		var owed []cad.OverflowCall
		s.Eng.Read(func(st *cad.State) {
			for _, c := range st.OverflowCalls {
				if !c.Delivered || slices.ContainsFunc(c.Notes, func(n cad.OverflowNote) bool { return !n.Delivered && !n.Theirs }) {
					owed = append(owed, *c)
				}
			}
		})
		for _, c := range owed {
			if p, ok := s.partner(c.Partner); ok {
				s.deliverCall(ctx, p, c)
			}
		}
	}
}

func (s *Server) markDirty(number string) {
	s.ovf.mu.Lock()
	if s.ovf.dirty == nil {
		s.ovf.dirty = map[string]bool{}
	}
	s.ovf.dirty[number] = true
	s.ovf.mu.Unlock()
}

// overflowUpdates sends a neighbour what became of the calls it took for us: status, units, notes, the
// close. Each incident is sent at most every 5 seconds; a send that fails is tried again.
func (s *Server) overflowUpdates(ctx context.Context) {
	if s.isTraining {
		return
	}
	ch := s.Eng.Subscribe()
	defer s.Eng.Unsubscribe(ch)
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-ch:
			if n.Inc == "" {
				continue
			}
			var ours bool
			s.Eng.Read(func(st *cad.State) { ours = st.OverflowIncidents[n.Inc] != nil })
			if ours {
				s.markDirty(n.Inc)
			}
		case <-t.C:
			s.ovf.mu.Lock()
			var numbers []string
			for n := range s.ovf.dirty {
				numbers = append(numbers, n)
			}
			s.ovf.mu.Unlock()
			for _, n := range numbers {
				if s.pushUpdate(ctx, n) == nil {
					s.ovf.mu.Lock()
					delete(s.ovf.dirty, n)
					s.ovf.mu.Unlock()
				}
			}
		}
	}
}

func (s *Server) pushUpdate(ctx context.Context, number string) error {
	var m updateMsg
	var partner string
	s.Eng.Read(func(st *cad.State) {
		o, in := st.OverflowIncidents[number], st.Incidents[number]
		if o == nil || in == nil {
			return
		}
		partner = o.Partner
		m = updateMsg{Ref: o.Ref, Number: number, Status: in.Status, Closed: in.Status == cad.Closed, Disposition: in.Disposition, Units: []cad.OverflowUnit{}}
		for _, as := range in.Units {
			st := as.Status
			if !as.Cleared.IsZero() {
				st = "cleared"
			}
			m.Units = append(m.Units, cad.OverflowUnit{Unit: as.Unit, Status: st})
		}
		// Notes written here, not the neighbour's own: theirs they already have.
		for _, e := range in.Timeline {
			if (e.Kind == "note" || e.Kind == "priority-note") && !strings.HasPrefix(e.Actor, o.Partner+":") {
				m.Notes = append(m.Notes, cad.OverflowNote{At: e.At, By: e.Actor, Text: e.Text})
			}
		}
		if len(m.Notes) > 50 {
			m.Notes = m.Notes[len(m.Notes)-50:]
		}
	})
	p, ok := s.partner(partner)
	if !ok {
		return nil // the neighbour is gone: nothing to send to
	}
	return s.overflowSend(ctx, p, "POST", "/updates", m, nil)
}
