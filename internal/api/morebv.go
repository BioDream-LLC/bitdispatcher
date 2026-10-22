package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/bitvoice"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// More BitVoice systems: besides the main BitVoice (Integrations), bitdispatcher can follow others,
// such as a neighbouring PSAP's or a second site's. Each has its own link and its own push secrets; a
// call remembers which system it is on, so what a dispatcher does with it goes back to that one.

type moreBV struct {
	br     *bitvoice.Bridge
	cancel context.CancelFunc
}

type moreBridges struct {
	mu  sync.Mutex
	ctx context.Context // set by Run; bridges made before it start then
	by  map[string]*moreBV
}

// applyMore brings the running bridges in line with the configured systems.
func (s *Server) applyMore(list []bitvoice.Config) {
	m := &s.moreBV
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.by == nil {
		m.by = map[string]*moreBV{}
	}
	keep := map[string]bool{}
	for _, c := range list {
		keep[c.ID] = true
		mb := m.by[c.ID]
		if mb == nil {
			mb = &moreBV{br: bitvoice.New(s.Eng, s.Log.With("bitvoice", c.Name))}
			m.by[c.ID] = mb
			if m.ctx != nil {
				m.start(mb)
			}
		}
		mb.br.Configure(c)
	}
	for id, mb := range m.by {
		if !keep[id] {
			if mb.cancel != nil {
				mb.cancel()
			}
			delete(m.by, id)
		}
	}
}

func (m *moreBridges) start(mb *moreBV) {
	ctx, cancel := context.WithCancel(m.ctx)
	mb.cancel = cancel
	go mb.br.Run(ctx)
}

func (s *Server) runMore(ctx context.Context) {
	m := &s.moreBV
	m.mu.Lock()
	m.ctx = ctx
	for _, mb := range m.by {
		m.start(mb)
	}
	m.mu.Unlock()
}

// bvFor is the BitVoice a call is on. A call on a system since removed gets an unconfigured bridge,
// so the action fails plainly instead of going to the wrong BitVoice.
func (s *Server) bvFor(call string) *bitvoice.Bridge {
	sys := ""
	s.Eng.Read(func(st *cad.State) {
		if c := st.Calls[call]; c != nil {
			sys = c.System
		}
	})
	if sys == "" {
		return s.BV
	}
	s.moreBV.mu.Lock()
	defer s.moreBV.mu.Unlock()
	if mb := s.moreBV.by[sys]; mb != nil {
		return mb.br
	}
	return bitvoice.New(s.Eng, s.Log)
}

// bvAll is the main BitVoice, then the others in the order configured.
func (s *Server) bvAll() []*bitvoice.Bridge {
	out := []*bitvoice.Bridge{s.BV}
	s.mu.Lock()
	list := slices.Clone(s.integ.More)
	s.mu.Unlock()
	s.moreBV.mu.Lock()
	defer s.moreBV.mu.Unlock()
	for _, c := range list {
		if mb := s.moreBV.by[c.ID]; mb != nil {
			out = append(out, mb.br)
		}
	}
	return out
}

// eidoPush and radioPush take a push from whichever BitVoice's secret it carries.
func (s *Server) eidoPush() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, b := range s.bvAll() {
			if b.TakesEIDO(r) {
				b.EIDOHandler().ServeHTTP(w, r)
				return
			}
		}
		s.BV.EIDOHandler().ServeHTTP(w, r) // unauthorized
	})
}

func (s *Server) radioPush() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, b := range s.bvAll() {
			if b.TakesRadio(r) {
				b.RadioHandler().ServeHTTP(w, r)
				return
			}
		}
		s.BV.RadioHandler().ServeHTTP(w, r)
	})
}

// MoreHealth is one more BitVoice's link, for the status screens.
type MoreHealth struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	URL    string          `json:"url"`
	Health bitvoice.Health `json:"health"`
}

func (s *Server) moreHealth() []MoreHealth {
	out := []MoreHealth{}
	for _, b := range s.bvAll()[1:] {
		c := b.Config()
		out = append(out, MoreHealth{ID: c.ID, Name: c.Name, URL: c.URL, Health: b.Health()})
	}
	return out
}

// MoreView is one more BitVoice's settings, secrets replaced by whether they are set.
type MoreView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Fingerprint string `json:"fingerprint"`
	Token       bool   `json:"tokenSet"`
	EIDOToken   bool   `json:"eidoTokenSet"`
	RadioSecret bool   `json:"radioSecretSet"`
}

func moreViews(list []bitvoice.Config) []MoreView {
	out := []MoreView{}
	for _, c := range list {
		out = append(out, MoreView{ID: c.ID, Name: c.Name, URL: c.URL, Fingerprint: c.Fingerprint,
			Token: c.Token != "", EIDOToken: c.EIDOToken != "", RadioSecret: c.RadioSecret != ""})
	}
	return out
}

func (s *Server) moreRoutes() {
	s.handle("POST /api/admin/integrations/more", auth.Admin, s.saveMore)
	s.handle("PUT /api/admin/integrations/more/{id}", auth.Admin, s.saveMore)
	s.handle("DELETE /api/admin/integrations/more/{id}", auth.Admin, s.removeMore)
	s.handle("POST /api/admin/integrations/more/{id}/test", auth.Admin, s.testMore)
}

// saveMore adds a BitVoice system (POST) or changes one (PUT). Secrets left empty keep the current
// one, "-" clears it.
func (s *Server) saveMore(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var in struct {
		Name        string `json:"name"`
		URL         string `json:"url"`
		Fingerprint string `json:"fingerprint"`
		Token       string `json:"token"`
		EIDOToken   string `json:"eidoToken"`
		RadioSecret string `json:"radioSecret"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Name, in.URL = strings.TrimSpace(in.Name), strings.TrimRight(strings.TrimSpace(in.URL), "/")
	if in.Name == "" || len(in.Name) > 60 {
		fail(w, 422, "name the BitVoice system, such as Botetourt 911")
		return
	}
	if !strings.HasPrefix(in.URL, "https://") {
		fail(w, 422, "BitVoice's address starts with https://")
		return
	}
	id := r.PathValue("id")
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.integ
	n.More = slices.Clone(n.More)
	i := slices.IndexFunc(n.More, func(c bitvoice.Config) bool { return c.ID == id })
	if id == "" {
		id = "bv" + auth.RandomToken(4)
		n.More = append(n.More, bitvoice.Config{ID: id})
		i = len(n.More) - 1
	} else if i < 0 {
		fail(w, 404, "no such BitVoice system")
		return
	}
	c := &n.More[i]
	c.Name, c.URL, c.Fingerprint = in.Name, in.URL, strings.TrimSpace(in.Fingerprint)
	keep := func(dst *string, v string) {
		switch v = strings.TrimSpace(v); v {
		case "":
		case "-":
			*dst = ""
		default:
			*dst = v
		}
	}
	keep(&c.Token, in.Token)
	keep(&c.EIDOToken, in.EIDOToken)
	keep(&c.RadioSecret, in.RadioSecret)
	// A push is told apart by its secret, so no two systems may share one.
	for _, o := range append([]bitvoice.Config{n.BitVoice}, n.More...) {
		if o.ID == c.ID {
			continue
		}
		if c.EIDOToken != "" && c.EIDOToken == o.EIDOToken || c.RadioSecret != "" && c.RadioSecret == o.RadioSecret {
			fail(w, 422, "each BitVoice needs its own EIDO token and radio secret: make new ones")
			return
		}
	}
	if err := s.writeIntegrationsLocked(n); err != nil {
		writeErr(w, err)
		return
	}
	s.Eng.Record(a, cad.EvIntegration, map[string]any{"bitvoice": c.Name, "bitvoiceUrl": c.URL, "changes": []string{"BitVoice system " + c.Name + " saved"}})
	writeJSON(w, 200, map[string]any{"id": id, "more": moreViews(n.More)})
}

func (s *Server) removeMore(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	id := r.PathValue("id")
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.integ
	i := slices.IndexFunc(n.More, func(c bitvoice.Config) bool { return c.ID == id })
	if i < 0 {
		fail(w, 404, "no such BitVoice system")
		return
	}
	name := n.More[i].Name
	n.More = slices.Delete(slices.Clone(n.More), i, i+1)
	if err := s.writeIntegrationsLocked(n); err != nil {
		writeErr(w, err)
		return
	}
	s.Eng.Record(a, cad.EvIntegration, map[string]any{"bitvoice": name, "changes": []string{"BitVoice system " + name + " removed"}})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testMore(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	id := r.PathValue("id")
	s.moreBV.mu.Lock()
	mb := s.moreBV.by[id]
	s.moreBV.mu.Unlock()
	if mb == nil {
		fail(w, 404, "no such BitVoice system")
		return
	}
	msg, err := mb.br.Test(r.Context())
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"detail": msg})
}

// writeIntegrationsLocked saves and applies the integrations; s.mu is held.
func (s *Server) writeIntegrationsLocked(n Integrations) error {
	data, _ := json.MarshalIndent(n, "", "  ")
	tmp := s.integrationsPath() + ".tmp"
	err := os.WriteFile(tmp, data, 0o600)
	if err == nil {
		err = os.Rename(tmp, s.integrationsPath())
	}
	if err != nil {
		return err
	}
	s.integ = n
	s.applyIntegrations()
	return nil
}
