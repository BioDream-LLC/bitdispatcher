package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// Replay: an incident played back from the ledger: its timeline and the recorded positions of the units
// that were on it, from five minutes before it was created to when it closed (or now). The ledger keeps a
// unit's position about every 30 seconds, so the tracks are as fine as that.

// ReplayTrack is one unit's positions.
type ReplayTrack struct {
	Unit   string         `json:"unit"`
	Points []cad.Position `json:"points"`
}

// Replay is what the console plays.
type Replay struct {
	Incident string        `json:"incident"`
	Start    time.Time     `json:"start"`
	End      time.Time     `json:"end"`
	Lat      *float64      `json:"lat,omitempty"`
	Lon      *float64      `json:"lon,omitempty"`
	Timeline []cad.Entry   `json:"timeline"`
	Tracks   []ReplayTrack `json:"tracks"`
}

func (s *Server) replayRoutes() {
	s.handle("GET /api/incidents/{n}/replay", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		n := r.PathValue("n")
		var rp Replay
		units := map[string]bool{}
		found := false
		s.Eng.Read(func(st *cad.State) {
			in := st.Incidents[n]
			if in == nil {
				return
			}
			found = true
			rp = Replay{Incident: n, Start: in.Created.Add(-5 * time.Minute), End: time.Now(), Timeline: slices.Clone(in.Timeline), Tracks: []ReplayTrack{}}
			if !in.Closed.IsZero() {
				rp.End = in.Closed
				// Late entries (a note, a correction) play after the close.
				if n := len(in.Timeline); n > 0 && in.Timeline[n-1].At.After(rp.End) {
					rp.End = in.Timeline[n-1].At
				}
			}
			if in.Location != nil {
				rp.Lat, rp.Lon = in.Location.Lat, in.Location.Lon
			}
			for _, as := range in.Units {
				units[as.Unit] = true
			}
		})
		if !found {
			fail(w, 404, "no incident")
			return
		}
		evs, err := s.Eng.Events(0, 0, func(e ledger.Event) bool {
			return e.Type == cad.EvUnitLocation && units[e.Unit] && !e.At.Before(rp.Start) && !e.At.After(rp.End)
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		byUnit := map[string]*ReplayTrack{}
		for _, e := range evs {
			var p cad.Position
			if err := json.Unmarshal(e.Data, &p); err != nil {
				continue
			}
			if p.At.IsZero() {
				p.At = e.At
			}
			t := byUnit[e.Unit]
			if t == nil {
				t = &ReplayTrack{Unit: e.Unit}
				byUnit[e.Unit] = t
			}
			t.Points = append(t.Points, p)
		}
		for u := range units {
			if t := byUnit[u]; t != nil {
				rp.Tracks = append(rp.Tracks, *t)
			}
		}
		slices.SortFunc(rp.Tracks, func(a, b ReplayTrack) int {
			if a.Unit < b.Unit {
				return -1
			}
			return 1
		})
		writeJSON(w, 200, rp)
	})
}
