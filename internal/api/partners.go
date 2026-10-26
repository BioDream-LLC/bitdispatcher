package api

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// CAD-to-CAD: incidents sent to and received from neighbouring agencies' CADs as EIDOs (NENA-STA-024).
// Each neighbour has its own address and its own pair of tokens. A received incident arrives as a call in
// the queue, for a dispatcher to act on; nothing is created or dispatched on its own (docs/CAD-TO-CAD.md).

// Partner is a neighbouring agency.
type Partner struct {
	ID       string `json:"id"`       // short, in addresses: FRANKLIN, SALEM
	Name     string `json:"name"`     // Franklin County 911
	URL      string `json:"url"`      // where their CAD takes EIDOs
	OutToken string `json:"outToken"` // sent to them as Bearer
	InToken  string `json:"inToken"`  // what they send us as Bearer
	// Fingerprint pins their CAD's certificate (SHA-256, hex) for overflow, for a neighbour whose
	// certificate is its own rather than from an authority; empty trusts the usual authorities.
	Fingerprint string `json:"fingerprint,omitempty"`
	// Overflow is the overflow agreed with them (overflow.go).
	Overflow PartnerOverflow `json:"overflow,omitzero"`
}

// PartnerView is a partner without its tokens.
type PartnerView struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	URL         string          `json:"url"`
	OutToken    bool            `json:"outTokenSet"`
	InToken     bool            `json:"inTokenSet"`
	Receive     string          `json:"receive"` // the address they send to
	Overflow    PartnerOverflow `json:"overflow"`
	Fingerprint string          `json:"fingerprint,omitempty"`
	// OverflowProblem is why overflow cannot work with them as set up, if it cannot.
	OverflowProblem string `json:"overflowProblem,omitempty"`
}

// shareClient sends to neighbouring agencies (replaced in tests).
var shareClient = &http.Client{Timeout: 15 * time.Second}

var rePartner = regexp.MustCompile(`^[A-Z0-9_-]{2,20}$`)

var reFingerprint = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (s *Server) partnerRoutes() {
	s.mux.HandleFunc("POST /api/cad/eido/{partner}", s.receiveShared)
	s.handle("GET /api/partners", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out := []map[string]string{}
		for _, p := range s.partners() {
			out = append(out, map[string]string{"id": p.ID, "name": p.Name})
		}
		writeJSON(w, 200, out)
	})
	s.handle("GET /api/admin/partners", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out := []PartnerView{}
		for _, p := range s.partners() {
			_, _, perr := p.overflowBase()
			out = append(out, PartnerView{ID: p.ID, Name: p.Name, URL: p.URL, OutToken: p.OutToken != "", InToken: p.InToken != "", Receive: "/api/cad/eido/" + p.ID,
				Overflow: p.Overflow, Fingerprint: p.Fingerprint, OverflowProblem: errText(perr)})
		}
		writeJSON(w, 200, out)
	})
	s.handle("PUT /api/admin/partners/{id}", auth.Admin, s.savePartner)
	s.handle("DELETE /api/admin/partners/{id}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		id := strings.ToUpper(r.PathValue("id"))
		if err := s.updatePartners(func(ps []Partner) []Partner {
			out := ps[:0]
			for _, p := range ps {
				if p.ID != id {
					out = append(out, p)
				}
			}
			return out
		}); err != nil {
			writeErr(w, err)
			return
		}
		s.Eng.Record(a, cad.EvIntegration, map[string]any{"partnerRemoved": id})
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("POST /api/incidents/{n}/share", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Partner string `json:"partner"`
			Reason  string `json:"reason"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		if err := s.share(r, a, r.PathValue("n"), strings.ToUpper(b.Partner), b.Reason); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func (s *Server) partners() []Partner {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Partner(nil), s.integ.Partners...)
}

func (s *Server) updatePartners(f func([]Partner) []Partner) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.integ
	n.Partners = f(append([]Partner(nil), n.Partners...))
	data, _ := json.MarshalIndent(n, "", "  ")
	tmp := s.integrationsPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.integrationsPath()); err != nil {
		return err
	}
	s.integ = n
	return nil
}

func (s *Server) savePartner(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		Name        string  `json:"name"`
		URL         string  `json:"url"`
		OutToken    string  `json:"outToken"`
		InToken     string  `json:"inToken"`
		Fingerprint *string `json:"fingerprint"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	id := strings.ToUpper(strings.TrimSpace(r.PathValue("id")))
	var fp *string
	if b.Fingerprint != nil {
		f := strings.ToLower(strings.NewReplacer(":", "", " ", "").Replace(strings.TrimSpace(*b.Fingerprint)))
		if f != "" && !reFingerprint.MatchString(f) {
			fail(w, 422, "a certificate fingerprint is the SHA-256 in hex: 64 characters")
			return
		}
		fp = &f
	}
	b.URL = strings.TrimSpace(b.URL)
	switch {
	case !rePartner.MatchString(id):
		fail(w, 422, "an agency's ID is 2 to 20 letters, digits, - or _")
		return
	case strings.TrimSpace(b.Name) == "":
		fail(w, 422, "give the agency's name")
		return
	case b.URL != "" && !strings.HasPrefix(b.URL, "https://") && !strings.HasPrefix(b.URL, "http://127.0.0.1"):
		fail(w, 422, "the agency's address starts with https://")
		return
	}
	err := s.updatePartners(func(ps []Partner) []Partner {
		for i := range ps {
			if ps[i].ID == id {
				ps[i].Name, ps[i].URL = strings.TrimSpace(b.Name), b.URL
				if fp != nil {
					ps[i].Fingerprint = *fp
				}
				keepSecret(&ps[i].OutToken, b.OutToken)
				keepSecret(&ps[i].InToken, b.InToken)
				return ps
			}
		}
		p := Partner{ID: id, Name: strings.TrimSpace(b.Name), URL: b.URL}
		if fp != nil {
			p.Fingerprint = *fp
		}
		keepSecret(&p.OutToken, b.OutToken)
		keepSecret(&p.InToken, b.InToken)
		return append(ps, p)
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	s.Eng.Record(a, cad.EvIntegration, map[string]any{"partner": id, "url": b.URL})
	w.WriteHeader(http.StatusNoContent)
}

func keepSecret(dst *string, v string) {
	switch v = strings.TrimSpace(v); v {
	case "":
	case "-":
		*dst = ""
	default:
		*dst = v
	}
}

// share sends an incident to a neighbouring agency's CAD.
func (s *Server) share(r *http.Request, a cad.Actor, number, partner, reason string) error {
	var p *Partner
	for _, x := range s.partners() {
		if x.ID == partner {
			p = &x
			break
		}
	}
	if p == nil || p.URL == "" {
		return &cad.Problem{Field: "partner", Message: "no neighbouring agency " + partner + " with an address (Admin › Integrations)"}
	}
	var si ng911.SharedIncident
	found := false
	var agency string
	s.Eng.Read(func(st *cad.State) {
		in := st.Incidents[number]
		if in == nil {
			return
		}
		found = true
		agency = st.Settings.Agency
		si = ng911.SharedIncident{Agency: agency, Element: "bitdispatcher", Number: in.Number, Type: in.Type, Priority: in.Priority, Status: in.Status, Created: in.Created, Updated: in.Updated}
		if ct := st.CallTypes[in.Type]; ct != nil {
			si.Type = ct.Name
		}
		if in.Location != nil {
			l := *in.Location
			si.Location = &l
		}
		if in.Caller != nil {
			si.Callback = in.Caller.Callback
		}
		if strings.TrimSpace(reason) != "" {
			si.Notes = append(si.Notes, "Sent by "+agency+": "+strings.TrimSpace(reason))
		}
		for _, as := range in.Units {
			if as.Cleared.IsZero() {
				si.Notes = append(si.Notes, "Our unit on it: "+as.Unit+" ("+as.Status+")")
			}
		}
		for _, e := range in.Timeline {
			if e.Kind == "priority-note" || e.Kind == "note" {
				si.Notes = append(si.Notes, e.Text)
			}
		}
		if len(si.Notes) > 40 {
			si.Notes = si.Notes[:40]
		}
	})
	if !found {
		return &cad.Problem{Field: "incident", Message: "no incident " + number}
	}
	doc, err := ng911.EncodeEIDO(si)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(r.Context(), "POST", p.URL, bytes.NewReader(doc))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/emergency.eido+json")
	if p.OutToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.OutToken)
	}
	failure := ""
	resp, err := shareClient.Do(req)
	if err != nil {
		failure = err.Error()
	} else {
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			failure = "their CAD answered " + resp.Status
		}
	}
	if e := s.Eng.RecordShare(a, number, p.Name, reason, failure); e != nil {
		return e
	}
	if failure != "" {
		return &cad.Problem{Field: "partner", Message: "not sent to " + p.Name + ": " + failure}
	}
	return nil
}

// receiveShared takes an incident a neighbouring agency's CAD sends, as a call in the queue.
func (s *Server) receiveShared(w http.ResponseWriter, r *http.Request) {
	id := strings.ToUpper(r.PathValue("partner"))
	var p *Partner
	for _, x := range s.partners() {
		if x.ID == id {
			p = &x
			break
		}
	}
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if p == nil || p.InToken == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(p.InToken)) != 1 {
		fail(w, 401, "unknown agency or wrong token")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	c, err := ng911.ParseEIDO(raw)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	info := ng911.ParseIncidentInfo(raw)
	number := firstNonEmptyStr(info.Number, c.CallID)
	al := cad.Alert{Kind: cad.AlertCAD, Sender: p.Name, ID: number, Event: info.Type, Status: "new", Detail: strings.Join(c.Notes, " · ")}
	if err := cad.CheckAlert(&al); err != nil {
		writeErr(w, err)
		return
	}
	notes := append([]string{al.Summary()}, c.Notes...)
	if info.Priority != "" {
		notes = append(notes, "Their priority: "+info.Priority)
	}
	d := cad.CallData{Source: "cad", State: "data", Class: "CAD", Provider: p.Name, Callback: c.Callback, CallerName: p.Name, Location: c.Location, Emergency: true, Alert: &al, Notes: notes}
	callID := al.CallID()
	if err := s.Eng.UpsertCall(cad.Actor{User: "cad:" + p.ID}, callID, d); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"call": callID, "received": fmt.Sprintf("%s %s", p.Name, number)})
}
