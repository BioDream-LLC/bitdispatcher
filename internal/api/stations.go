package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// Station alerting: stations are toned out when their units are dispatched (where an administrator
// turned it on), shown and spoken on the station's screen (#/station), and posted to an alerting
// gateway's address when one is set (docs/STATION-ALERTING.md).

func (s *Server) stationRoutes() {
	// A read-only account opens a station's screen: its position becomes STATION-<id>, which the
	// inactivity lock leaves alone (a wall display in the firehouse).
	s.handle("POST /api/auth/station", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Station string `json:"station"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		id := strings.ToUpper(strings.TrimSpace(b.Station))
		known := false
		s.Eng.Read(func(st *cad.State) { _, known = st.Stations[id] })
		if !known {
			fail(w, 404, "no station "+id)
			return
		}
		if se.Role == auth.Viewer {
			s.Auth.SetConsole(se.ID, "STATION-"+id)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("POST /api/station-alerts/{id}/ack", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if err := s.Eng.AckStationAlert(a, r.PathValue("id")); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("POST /api/incidents/{n}/tone", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Units []string `json:"units"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		res := s.Eng.Execute(a, "TONE #"+r.PathValue("n")+" "+joinUnits(b.Units), "")
		if !res.OK {
			fail(w, 422, res.Message)
			return
		}
		writeJSON(w, 200, res)
	})
}

func joinUnits(us []string) string {
	out := ""
	for _, u := range us {
		out += " " + u
	}
	return out
}

// stationAlerts is the last day's tone-outs, for the screens.
func stationAlertsView(st *cad.State, now time.Time) []*cad.StationAlert {
	out := []*cad.StationAlert{}
	for _, al := range st.StationAlerts {
		if now.Sub(al.At) <= 24*time.Hour {
			c := *al
			c.Units = append([]string(nil), al.Units...)
			out = append(out, &c)
		}
	}
	return out
}

// stationAlerter tones out the stations of units as they are dispatched, and posts each alert to the
// alerting gateway when one is set.
func (s *Server) stationAlerter(ctx context.Context) {
	ch := s.Eng.Subscribe()
	defer s.Eng.Unsubscribe(ch)
	done := map[string]bool{} // incident|unit|dispatched time
	posted := map[string]bool{}
	client := &http.Client{Timeout: 10 * time.Second}
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-ch:
			x := s.extra()
			switch n.Type {
			case cad.EvIncDispatch:
				if !x.StationAlerts || n.Inc == "" {
					continue
				}
				var units []string
				s.Eng.Read(func(st *cad.State) {
					in := st.Incidents[n.Inc]
					if in == nil {
						return
					}
					for _, as := range in.Units {
						k := n.Inc + "|" + as.Unit + "|" + as.Dispatched.String()
						if as.Cleared.IsZero() && !done[k] {
							done[k] = true
							units = append(units, as.Unit)
						}
					}
				})
				if len(units) > 0 {
					if _, err := s.Eng.AlertStations(cad.SystemActor, n.Inc, units, false); err != nil {
						s.Log.Warn("station alert", "incident", n.Inc, "err", err)
					}
				}
			case cad.EvStationAlert:
				if x.StationGatewayURL == "" {
					continue
				}
				var out []cad.StationAlert
				s.Eng.Read(func(st *cad.State) {
					for _, al := range st.StationAlerts {
						if !posted[al.ID] && al.Incident == n.Inc {
							posted[al.ID] = true
							out = append(out, *al)
						}
					}
				})
				for _, al := range out {
					body, _ := json.Marshal(al)
					go func() {
						req, err := http.NewRequestWithContext(ctx, "POST", x.StationGatewayURL, bytes.NewReader(body))
						if err != nil {
							return
						}
						req.Header.Set("Content-Type", "application/json")
						if x.StationGatewayToken != "" {
							req.Header.Set("Authorization", "Bearer "+x.StationGatewayToken)
						}
						resp, err := client.Do(req)
						if err != nil {
							s.Log.Warn("station gateway: not delivered", "alert", al.ID, "err", err)
							return
						}
						resp.Body.Close()
						if resp.StatusCode >= 300 {
							s.Log.Warn("station gateway refused the alert", "alert", al.ID, "status", resp.Status)
						}
					}()
				}
			}
		}
	}
}

// autoDispatcher sends the recommended units to auto-dispatch calls no dispatcher has sent units to in time.
func (s *Server) autoDispatcher(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for _, n := range s.Eng.AutoDispatchDue(time.Now()) {
				sent, why, err := s.Eng.AutoDispatch(n)
				switch {
				case err != nil:
					slog.Warn("auto-dispatch", "incident", n, "err", err)
				case why != "":
					slog.Info("auto-dispatch: left for a dispatcher", "incident", n, "why", why)
				default:
					slog.Info("auto-dispatch: sent", "incident", n, "units", sent)
				}
			}
		}
	}
}
