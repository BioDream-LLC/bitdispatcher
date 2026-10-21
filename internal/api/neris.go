package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/neris"
)

// NERIS: fire and EMS incidents sent to the national reporting system as CAD records (docs in the
// manual, article admin-integrations). Off until an administrator turns it on.

func (s *Server) nerisRoutes() {
	s.handle("GET /api/admin/neris", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		writeJSON(w, 200, s.nerisView())
	})
	s.handle("PUT /api/admin/neris", auth.Admin, s.saveNERIS)
	s.handle("POST /api/admin/neris/test", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		c := s.nerisConfig()
		in, ok := s.lastFireIncident()
		if !ok {
			fail(w, 409, "no closed fire or EMS incident to check yet")
			return
		}
		res, err := s.nerisClient().Validate(r.Context(), c, in)
		writeJSON(w, 200, map[string]any{"incident": in.Number, "ok": err == nil, "status": res.Status, "answer": res.Body, "error": errString(err)})
	})
	s.handle("GET /api/incidents/{n}/neris", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		in, ok := s.nerisIncident(r.PathValue("n"))
		if !ok {
			fail(w, 404, "no incident "+r.PathValue("n"))
			return
		}
		writeJSON(w, 200, neris.Payload(s.nerisConfig(), in))
	})
	s.handle("POST /api/incidents/{n}/neris", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		rec, err := s.sendNERIS(r.Context(), a, r.PathValue("n"))
		if err != nil && rec == nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, rec)
	})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *Server) nerisConfig() neris.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.integ.NERIS
}

func (s *Server) nerisClient() *neris.Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.neris == nil {
		s.neris = &neris.Client{Agent: "bitdispatcher/" + s.Version + " (+https://github.com/biodream-llc/bitdispatcher)"}
	}
	return s.neris
}

// NERISView is the NERIS configuration with its secret replaced by whether it is set.
type NERISView struct {
	On         bool   `json:"on"`
	ClientID   string `json:"clientId"`
	Secret     bool   `json:"secretSet"`
	Department string `json:"department"`
	Center     string `json:"center"`
	Test       bool   `json:"test"`
	Auto       bool   `json:"auto"`
}

func (s *Server) nerisView() NERISView {
	c := s.nerisConfig()
	return NERISView{On: c.On, ClientID: c.ClientID, Secret: c.ClientSecret != "", Department: c.Department, Center: c.Center, Test: c.Test, Auto: c.Auto}
}

func (s *Server) saveNERIS(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		NERISView
		Secret string `json:"secret"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	s.mu.Lock()
	n := s.integ
	c := &n.NERIS
	c.On, c.ClientID, c.Department, c.Center, c.Test, c.Auto = b.On, strings.TrimSpace(b.ClientID), strings.ToUpper(strings.TrimSpace(b.Department)), strings.TrimSpace(b.Center), b.Test, b.Auto
	switch v := strings.TrimSpace(b.Secret); v {
	case "":
	case "-":
		c.ClientSecret = ""
	default:
		c.ClientSecret = v
	}
	if err := c.Check(); err != nil {
		s.mu.Unlock()
		fail(w, 422, err.Error())
		return
	}
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
	s.Eng.Record(a, cad.EvIntegration, map[string]any{"neris": map[string]any{"on": c.On, "department": c.Department, "test": c.Test, "auto": c.Auto}})
	writeJSON(w, 200, s.nerisView())
}

// fireOrEMS reports whether an incident is the fire department's: a fire or EMS call type, or a fire
// or EMS unit on it.
func fireOrEMS(st *cad.State, in *cad.Incident) bool {
	if ct, ok := st.CallTypes[in.Type]; ok && (ct.Discipline == "fire" || ct.Discipline == "ems") {
		return true
	}
	return slices.ContainsFunc(in.Units, func(as cad.Assignment) bool {
		u := st.Units[as.Unit]
		return u != nil && (st.Discipline(u) == "fire" || st.Discipline(u) == "ems")
	})
}

// nerisIncident is an incident as NERIS wants it.
func (s *Server) nerisIncident(number string) (neris.Incident, bool) {
	var out neris.Incident
	ok := false
	s.Eng.Read(func(st *cad.State) {
		in := st.Incidents[number]
		if in == nil {
			return
		}
		ok = true
		out = neris.Incident{Number: in.Number, Type: in.Type, Created: in.Created, Closed: in.Closed, Disposition: in.Disposition}
		if ct, ok := st.CallTypes[in.Type]; ok {
			out.TypeName = ct.Name
		}
		if l := in.Location; l != nil {
			out.Address, out.Lat, out.Lon = l.Address, l.Lat, l.Lon
			if c := l.Civic; c != nil {
				out.Civic = &neris.Civic{Number: c.Number + c.NumberSuf, Street: strings.TrimSpace(strings.Join([]string{c.PreDir, c.Street, c.StreetType, c.PostDir}, " ")),
					City: c.City, County: c.County, State: c.State, Postal: c.PostalCode}
			}
		}
		for _, id := range in.Calls {
			c := st.Calls[id]
			if c == nil {
				continue
			}
			if out.CallArrival.IsZero() || (!c.Start.IsZero() && c.Start.Before(out.CallArrival)) {
				out.CallArrival = c.Start
				out.CallAnswer = c.Answer
			}
			if c.Alert != nil && c.Alert.Kind == cad.AlertAlarm {
				out.Alarm = true
			}
		}
		for _, as := range in.Units {
			u := st.Units[as.Unit]
			if u != nil && st.Discipline(u) == "police" {
				continue // NERIS is the fire department's record
			}
			out.Units = append(out.Units, neris.Unit{ID: as.Unit, Dispatched: as.Dispatched, Enroute: as.Enroute, OnScene: as.OnScene, Cleared: as.Cleared,
				CanceledEnroute: as.OnScene.IsZero() && !as.Enroute.IsZero()})
		}
		for _, e := range in.Timeline {
			if e.Kind == "note" || e.Kind == "priority-note" {
				out.Comments = append(out.Comments, neris.Comment{At: e.At, Text: e.Text})
			}
		}
	})
	return out, ok
}

func (s *Server) lastFireIncident() (neris.Incident, bool) {
	number := ""
	s.Eng.Read(func(st *cad.State) {
		for _, in := range st.Incidents {
			if in.Status == cad.Closed && fireOrEMS(st, in) && (number == "" || in.Closed.After(st.Incidents[number].Closed)) {
				number = in.Number
			}
		}
	})
	if number == "" {
		return neris.Incident{}, false
	}
	return s.nerisIncident(number)
}

// sendNERIS sends one incident (creating it in NERIS, or updating the record NERIS already has) and
// keeps the answer on the incident.
func (s *Server) sendNERIS(ctx context.Context, a cad.Actor, number string) (*cad.NERISRecord, error) {
	c := s.nerisConfig()
	if !c.On {
		return nil, &cad.Problem{Field: "neris", Message: "NERIS is not on: Admin › Integrations › NERIS"}
	}
	in, ok := s.nerisIncident(number)
	if !ok {
		return nil, &cad.Problem{Field: "incident", Message: "no incident " + number}
	}
	prev := ""
	s.Eng.Read(func(st *cad.State) {
		if x := st.Incidents[number]; x != nil && x.NERIS != nil {
			prev = x.NERIS.ID
		}
	})
	res, err := s.nerisClient().Send(ctx, c, in, prev)
	rec := cad.NERISRecord{ID: res.ID, OK: err == nil, Status: res.Status}
	if err != nil {
		rec.Error = err.Error()
		if res.Body != "" {
			rec.Error += ": " + trimTo(res.Body, 600)
		}
	}
	if e2 := s.Eng.RecordNERIS(a, number, rec); e2 != nil {
		return nil, e2
	}
	return &rec, err
}

func trimTo(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// nerisAuto sends each fire or EMS incident to NERIS when it closes (again when it is closed after a
// reopening, as an update), where the administrator chose that.
func (s *Server) nerisAuto(ctx context.Context) {
	ch := s.Eng.Subscribe()
	defer s.Eng.Unsubscribe(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-ch:
			if n.Type != cad.EvIncClosed || n.Inc == "" {
				continue
			}
			c := s.nerisConfig()
			if !c.On || !c.Auto {
				continue
			}
			want := false
			s.Eng.Read(func(st *cad.State) {
				if in := st.Incidents[n.Inc]; in != nil {
					want = fireOrEMS(st, in)
				}
			})
			if want {
				go func(number string) {
					if _, err := s.sendNERIS(ctx, cad.SystemActor, number); err != nil {
						s.Log.Warn("neris: not sent", "incident", number, "err", err)
					}
				}(n.Inc)
			}
		}
	}
}
