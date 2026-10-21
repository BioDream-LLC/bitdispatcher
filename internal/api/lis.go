package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// Supplemental location by HELD (RFC 5985): bitdispatcher asks a location server where the caller's
// phone is, by its number: an NG9-1-1 LIS, or a clearinghouse that answers HELD for the centre (RapidSOS
// describes its NG911 Clearinghouse as answering NENA i3 queries; its credentials come from RapidSOS).
// The answer becomes the call's newest location through the same path as the location hook: the
// call-taker is offered it on an incident, nothing moves without a person (docs/SUPPLEMENTAL-LOCATION.md).

// LIS is its configuration.
type LIS struct {
	On       bool   `json:"on"`
	Name     string `json:"name"` // shown as the source: RapidSOS, the state LIS
	URL      string `json:"url"`
	Token    string `json:"token"`    // sent as Bearer, or
	User     string `json:"user"`     // with Password, as Basic
	Password string `json:"password"` //
	Auto     bool   `json:"auto"`     // ask for every new 911 call with a callback
}

// LISView is LIS without its secrets.
type LISView struct {
	On       bool   `json:"on"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Token    bool   `json:"tokenSet"`
	User     string `json:"user"`
	Password bool   `json:"passwordSet"`
	Auto     bool   `json:"auto"`
}

func (s *Server) lisConfig() LIS {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.integ.LIS
}

// lisClient asks location servers (replaced in tests).
var lisClient = &http.Client{Timeout: 10 * time.Second}

// askLIS asks the location server about a number.
func askLIS(ctx context.Context, c LIS, callback string) (*ng911.Location, error) {
	if c.URL == "" {
		return nil, errors.New("no location server is set")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.URL, bytes.NewReader(ng911.HELDRequest(callback, "emergencyDispatch")))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", ng911.HELDContentType)
	req.Header.Set("Accept", ng911.HELDContentType)
	switch {
	case c.Token != "":
		req.Header.Set("Authorization", "Bearer "+c.Token)
	case c.User != "":
		req.SetBasicAuth(c.User, c.Password)
	}
	resp, err := lisClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 && !bytes.Contains(raw, []byte("error")) {
		return nil, errors.New("the location server answered " + resp.Status)
	}
	return ng911.ParseHELD(raw)
}

// lisLocate asks about a call and puts the answer on it.
func (s *Server) lisLocate(ctx context.Context, callID string) (*ng911.Location, error) {
	c := s.lisConfig()
	if !c.On {
		return nil, &cad.Problem{Field: "lis", Message: "the location server is off (Admin › Integrations)"}
	}
	var cb string
	s.Eng.Read(func(st *cad.State) {
		if x := st.Calls[callID]; x != nil {
			cb = x.Callback
		}
	})
	if cb == "" {
		return nil, &cad.Problem{Field: "call", Message: "the call has no callback number to ask about"}
	}
	loc, err := askLIS(ctx, c, cb)
	if err != nil {
		return nil, err
	}
	src := "lis:" + strings.ToLower(firstNonEmptyStr(strings.TrimSpace(c.Name), "lis"))
	loc.Source = src
	if loc.Method == "" {
		loc.Method = "LIS"
	}
	if loc.At.IsZero() {
		loc.At = time.Now().UTC()
	}
	if err := s.Eng.UpsertCall(cad.Actor{User: src}, callID, cad.CallData{Source: src, Location: loc}); err != nil {
		return nil, err
	}
	return loc, nil
}

func (s *Server) lisRoutes() {
	s.handle("POST /api/calls/{id}/lis", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		loc, err := s.lisLocate(r.Context(), r.PathValue("id"))
		if err != nil {
			var he *ng911.HELDError
			if errors.As(err, &he) {
				fail(w, 404, "the location server has no location: "+he.Code)
				return
			}
			var p *cad.Problem
			if errors.As(err, &p) {
				writeErr(w, err)
				return
			}
			fail(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, loc)
	})
	s.handle("GET /api/admin/lis", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		writeJSON(w, 200, lisView(s.lisConfig()))
	})
	s.handle("PUT /api/admin/lis", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b LIS
		if !readJSON(w, r, &b) {
			return
		}
		b.URL = strings.TrimSpace(b.URL)
		if b.URL != "" && !strings.HasPrefix(b.URL, "https://") && !strings.HasPrefix(b.URL, "http://127.0.0.1") {
			fail(w, 422, "the location server's address starts with https://")
			return
		}
		if b.On && b.URL == "" {
			fail(w, 422, "give the location server's address")
			return
		}
		s.mu.Lock()
		n := s.integ
		c := n.LIS
		keepSecret(&c.Token, b.Token)
		keepSecret(&c.Password, b.Password)
		c.On, c.Name, c.URL, c.User, c.Auto = b.On, strings.TrimSpace(b.Name), b.URL, strings.TrimSpace(b.User), b.Auto
		n.LIS = c
		data, _ := json.MarshalIndent(n, "", "  ")
		tmp := s.integrationsPath() + ".tmp"
		err := os.WriteFile(tmp, data, 0o600)
		if err == nil {
			err = os.Rename(tmp, s.integrationsPath())
		}
		if err == nil {
			s.integ = n
		}
		s.mu.Unlock()
		if err != nil {
			writeErr(w, err)
			return
		}
		s.Eng.Record(a, cad.EvIntegration, map[string]any{"lis": c.On, "lisName": c.Name, "lisUrl": c.URL, "lisAuto": c.Auto})
		writeJSON(w, 200, lisView(c))
	})
	s.handle("POST /api/admin/lis/test", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct{ Callback string }
		if !readJSON(w, r, &b) {
			return
		}
		loc, err := askLIS(r.Context(), s.lisConfig(), b.Callback)
		if err != nil {
			writeJSON(w, 200, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "location": loc})
	})
}

func lisView(c LIS) LISView {
	return LISView{On: c.On, Name: c.Name, URL: c.URL, Token: c.Token != "", User: c.User, Password: c.Password != "", Auto: c.Auto}
}

// lisAuto asks the location server about every new 911 call with a callback, when set to.
func (s *Server) lisAuto(ctx context.Context) {
	ch := s.Eng.Subscribe()
	defer s.Eng.Unsubscribe(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-ch:
			if n.Type != cad.EvCallReceived || n.Call == "" {
				continue
			}
			c := s.lisConfig()
			if !c.On || !c.Auto {
				continue
			}
			ask := false
			s.Eng.Read(func(st *cad.State) {
				if x := st.Calls[n.Call]; x != nil {
					ask = x.Callback != "" && x.Alert == nil && !x.TextOnly
				}
			})
			if !ask {
				continue
			}
			go func(id string) {
				cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
				defer cancel()
				if _, err := s.lisLocate(cctx, id); err != nil {
					slog.Info("location server: no location", "call", id, "err", err)
				}
			}(n.Call)
		}
	}
}

func lisName(c LIS) string {
	if !c.On {
		return ""
	}
	return firstNonEmptyStr(strings.TrimSpace(c.Name), "the location server")
}
