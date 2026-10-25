package api

// Road closures and live traffic over the recorded map. Closures are events in the ledger (cad/closures.go);
// traffic comes from a feed the agency subscribes to: Waze for Cities (the free data-sharing programme for
// public agencies; its JSON feed of jams), or any GeoJSON feed of slow or closed roads. Both become factors
// on the recorded road segments (mapdata/overlay.go), so recommendations and drive times use them at once.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/mapdata"
)

// NextConfig is the configuration of the features added in October 2026 that are not in the ledger:
// connections with secrets.
type NextConfig struct {
	Traffic        Traffic        `json:"traffic"`
	AlarmCompanies []AlarmCompany `json:"alarmCompanies"`
	// IntakeToken is what a voice or text bot on the non-emergency line presents (Bearer); empty: off.
	IntakeToken string `json:"intakeToken,omitempty"`
}

// Traffic is a live traffic feed.
type Traffic struct {
	On    bool   `json:"on"`
	Kind  string `json:"kind"` // waze or geojson
	URL   string `json:"url"`
	Token string `json:"token,omitempty"` // sent as Bearer, when the feed needs one
	Every int    `json:"every,omitempty"` // seconds between fetches (0: 120)
}

// nextRuntime is the live state of the October 2026 features.
type nextRuntime struct {
	mu      sync.Mutex
	roads   roadsNow
	kick    chan struct{} // recompute the overlay now
	traffic trafficNow
	intake  map[string]*intakeRun
	here    map[string]*hereRun // location links, by the hash of their secret
}

// roadsNow is the overlay as last applied.
type roadsNow struct {
	version  uint64
	store    *mapdata.Store
	sig      string
	closures map[string]closureGeo
}

type closureGeo struct {
	Segments int            `json:"segments"`
	Shapes   [][][2]float64 `json:"shapes"`
	Roads    []string       `json:"roads"`
}

// trafficNow is the last traffic fetched.
type trafficNow struct {
	At    time.Time `json:"at"`
	Error string    `json:"error,omitempty"`
	Jams  []jam     `json:"jams"`
	Kind  string    `json:"kind"`
}

type jam struct {
	Line   [][2]float64 `json:"line"`
	Kmh    float64      `json:"kmh"`
	Closed bool         `json:"closed,omitempty"`
	Street string       `json:"street,omitempty"`
	Delay  int          `json:"delay,omitempty"` // seconds, when the feed says
	Factor float64      `json:"factor"`          // how much slower than recorded, worked out per road
	given  float64      // a factor the feed gave instead of a speed
	speed  bool         // the feed gave a speed
}

func (n *nextRuntime) kicker() chan struct{} {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.kick == nil {
		n.kick = make(chan struct{}, 1)
	}
	return n.kick
}

func (s *Server) kickRoads() {
	select {
	case s.next.kicker() <- struct{}{}:
	default:
	}
}

func (s *Server) nextConfig() NextConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.integ.Next
}

// runRoads keeps the router's overlay up to date: when a closure changes, one starts or ends, traffic
// arrives, or a new map is recorded.
func (s *Server) runRoads(ctx context.Context) {
	ch := s.Eng.Subscribe()
	defer s.Eng.Unsubscribe(ch)
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	kick := s.next.kicker()
	s.applyRoads(true)
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-ch:
			if strings.HasPrefix(n.Type, "road.closure.") {
				s.applyRoads(true)
			}
		case <-kick:
			s.applyRoads(true)
		case <-t.C:
			s.applyRoads(false)
		}
	}
}

// applyRoads works out the overlay and gives it to the router, when something changed (or always, forced).
func (s *Server) applyRoads(force bool) {
	if s.Maps == nil {
		return
	}
	st := s.Maps.Store()
	if st == nil {
		return
	}
	r, err := st.Router()
	if err != nil {
		return
	}
	now := s.Eng.Now()
	var in []*cad.RoadClosure
	s.Eng.Read(func(x *cad.State) {
		for _, c := range x.ClosuresInForce(now) {
			cc := *c
			in = append(in, &cc)
		}
	})
	sort.Slice(in, func(i, j int) bool { return in[i].ID < in[j].ID })
	s.next.mu.Lock()
	tr := s.next.traffic
	tr.Jams = append([]jam(nil), tr.Jams...) // worked on outside the lock
	var sig strings.Builder
	for _, c := range in {
		fmt.Fprintf(&sig, "%s@%d;", c.ID, c.At.UnixNano())
	}
	fmt.Fprintf(&sig, "traffic@%d", tr.At.UnixNano())
	same := s.next.roads.store == st && s.next.roads.sig == sig.String() && r.OverlayVersion() == s.next.roads.version
	s.next.mu.Unlock()
	if same && !force {
		return
	}
	var f []float32
	ones := func() {
		if f == nil {
			f = make([]float32, r.Segments())
			for i := range f {
				f[i] = 1
			}
		}
	}
	for i := range tr.Jams {
		j := &tr.Jams[i]
		segs := r.AlongLine(j.Line, 20)
		if len(segs) == 0 {
			continue
		}
		ones()
		worst := 1.0
		for _, si := range segs {
			if j.Closed {
				f[si] = mapdata.Closed
				continue
			}
			rec := r.Seg(si).Kmh
			k := 1.0
			switch {
			case j.given >= 1:
				k = math.Min(10, j.given)
			case j.speed && j.Kmh > 0 && rec > j.Kmh:
				k = math.Min(10, rec/j.Kmh)
			case j.speed && j.Kmh <= 0:
				k = 10 // a standstill
			}
			if float32(k) > f[si] && f[si] != mapdata.Closed {
				f[si] = float32(k)
			}
			worst = math.Max(worst, k)
		}
		j.Factor = math.Round(worst*10) / 10
	}
	geos := map[string]closureGeo{}
	for _, c := range in {
		var segs []uint32
		if c.Center != nil {
			segs = r.InCircle(c.Center[0], c.Center[1], c.Radius)
		} else {
			segs = r.AlongLine(c.Line, 18)
		}
		g := closureGeo{Segments: len(segs), Shapes: [][][2]float64{}, Roads: []string{}}
		names := map[string]bool{}
		for _, si := range segs {
			ones()
			f[si] = mapdata.Closed
			if len(g.Shapes) < 400 {
				g.Shapes = append(g.Shapes, r.Shape(si))
			}
			if n := r.Seg(si).Name; n != "" && !names[n] {
				names[n] = true
				g.Roads = append(g.Roads, n)
			}
		}
		sort.Strings(g.Roads)
		geos[c.ID] = g
	}
	v := r.SetFactors(f)
	s.next.mu.Lock()
	s.next.roads = roadsNow{version: v, store: st, sig: sig.String(), closures: geos}
	if s.next.traffic.At.Equal(tr.At) && len(s.next.traffic.Jams) == len(tr.Jams) {
		s.next.traffic.Jams = tr.Jams
	}
	s.next.mu.Unlock()
}

// roadsLabel says, after a drive time's source, what the overlay changed.
func (s *Server) roadsLabel() string {
	s.next.mu.Lock()
	defer s.next.mu.Unlock()
	var parts []string
	if n := len(s.next.roads.closures); n == 1 {
		parts = append(parts, "around 1 road closure")
	} else if n > 1 {
		parts = append(parts, fmt.Sprintf("around %d road closures", n))
	}
	if t := s.next.traffic; !t.At.IsZero() && t.Error == "" && time.Since(t.At) < 15*time.Minute {
		parts = append(parts, fmt.Sprintf("with live traffic (%s, %d slowdowns, %s)", trafficName(t.Kind), len(t.Jams), t.At.Local().Format("15:04")))
	}
	if len(parts) == 0 {
		return ""
	}
	return "; " + strings.Join(parts, ", ")
}

func trafficName(kind string) string {
	if kind == "waze" {
		return "Waze for Cities"
	}
	return "traffic feed"
}

// runTraffic fetches the traffic feed while it is on.
func (s *Server) runTraffic(ctx context.Context) {
	client := &http.Client{Timeout: 20 * time.Second}
	var last time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
		c := s.nextConfig().Traffic
		if !c.On || c.URL == "" {
			s.next.mu.Lock()
			had := !s.next.traffic.At.IsZero()
			s.next.traffic = trafficNow{}
			s.next.mu.Unlock()
			if had {
				s.kickRoads()
			}
			continue
		}
		every := time.Duration(max(30, c.Every)) * time.Second
		if c.Every == 0 {
			every = 2 * time.Minute
		}
		if time.Since(last) < every {
			continue
		}
		last = time.Now()
		jams, err := fetchTraffic(ctx, client, c)
		s.next.mu.Lock()
		if err != nil {
			s.next.traffic.Error, s.next.traffic.Kind = err.Error(), c.Kind
			s.Log.Warn("traffic feed", "err", err)
		} else {
			s.next.traffic = trafficNow{At: time.Now(), Jams: jams, Kind: c.Kind}
		}
		s.next.mu.Unlock()
		s.kickRoads()
	}
}

// fetchTraffic reads a feed: Waze for Cities' JSON (its "jams") or GeoJSON lines.
func fetchTraffic(ctx context.Context, client *http.Client, c Traffic) ([]jam, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("the feed answered %s", resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if c.Kind == "waze" {
		return parseWaze(raw)
	}
	return parseGeoJSONTraffic(raw)
}

// parseWaze reads Waze for Cities' traffic view: each jam's line, speed (km/h), level (5: standstill or
// blocked) and street.
func parseWaze(raw []byte) ([]jam, error) {
	var v struct {
		Jams []struct {
			Line []struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
			} `json:"line"`
			SpeedKMH *float64 `json:"speedKMH"`
			Speed    *float64 `json:"speed"` // metres a second, in older feeds
			Level    int      `json:"level"`
			Street   string   `json:"street"`
			Delay    int      `json:"delay"`
			Type     string   `json:"blockingAlertType"`
		} `json:"jams"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("not a Waze for Cities feed: %w", err)
	}
	out := []jam{}
	for _, j := range v.Jams {
		if len(j.Line) < 2 || len(out) >= 5000 {
			continue
		}
		jm := jam{Street: j.Street, Delay: j.Delay}
		for _, p := range j.Line {
			jm.Line = append(jm.Line, [2]float64{p.X, p.Y})
		}
		switch {
		case j.SpeedKMH != nil:
			jm.Kmh, jm.speed = *j.SpeedKMH, true
		case j.Speed != nil:
			jm.Kmh, jm.speed = *j.Speed*3.6, true
		}
		// Level 5 is a standstill; a jam caused by a road-closed report is a closure.
		jm.Closed = j.Level >= 5 && (strings.Contains(strings.ToUpper(j.Type), "ROAD_CLOSED") || j.Delay < 0)
		out = append(out, jm)
	}
	return out, nil
}

// parseGeoJSONTraffic reads a FeatureCollection of LineStrings, each with speed_kmh, or factor (how many
// times slower), or closed: true; name (or street) for the screen.
func parseGeoJSONTraffic(raw []byte) ([]jam, error) {
	var v struct {
		Type     string `json:"type"`
		Features []struct {
			Geometry struct {
				Type        string          `json:"type"`
				Coordinates json.RawMessage `json:"coordinates"`
			} `json:"geometry"`
			Properties map[string]any `json:"properties"`
		} `json:"features"`
	}
	if err := json.Unmarshal(raw, &v); err != nil || v.Type != "FeatureCollection" {
		return nil, fmt.Errorf("not a GeoJSON FeatureCollection")
	}
	out := []jam{}
	for _, f := range v.Features {
		var lines [][][2]float64
		switch f.Geometry.Type {
		case "LineString":
			var l [][2]float64
			if json.Unmarshal(f.Geometry.Coordinates, &l) == nil {
				lines = append(lines, l)
			}
		case "MultiLineString":
			json.Unmarshal(f.Geometry.Coordinates, &lines)
		}
		num := func(k string) (float64, bool) {
			x, ok := f.Properties[k].(float64)
			return x, ok
		}
		name, _ := f.Properties["name"].(string)
		if name == "" {
			name, _ = f.Properties["street"].(string)
		}
		closed, _ := f.Properties["closed"].(bool)
		for _, l := range lines {
			if len(l) < 2 || len(out) >= 5000 {
				continue
			}
			j := jam{Line: l, Street: name, Closed: closed}
			if k, ok := num("speed_kmh"); ok && k >= 0 {
				j.Kmh, j.speed = k, true
			} else if k, ok := num("factor"); ok && k >= 1 {
				j.given, j.Factor = k, k
			} else if !closed {
				continue // says nothing about the road
			}
			out = append(out, j)
		}
	}
	return out, nil
}

// ---- the API ----

type roadsView struct {
	Version  uint64             `json:"version"`
	Closures []closureView      `json:"closures"`
	Traffic  trafficView        `json:"traffic"`
	Lifted   []*cad.RoadClosure `json:"lifted"` // lifted in the last day, for the record
}

type closureView struct {
	*cad.RoadClosure
	closureGeo
}

type trafficView struct {
	On    bool      `json:"on"`
	Kind  string    `json:"kind,omitempty"`
	At    time.Time `json:"at,omitzero"`
	Error string    `json:"error,omitempty"`
	Jams  []jam     `json:"jams"`
}

func (s *Server) roadsNowView(withGeometry bool) roadsView {
	now := s.Eng.Now()
	v := roadsView{Closures: []closureView{}, Lifted: []*cad.RoadClosure{}}
	s.Eng.Read(func(st *cad.State) {
		for _, c := range st.Closures {
			cc := *c
			if c.InForce(now) || c.Lifted == nil && c.From.After(now) {
				v.Closures = append(v.Closures, closureView{RoadClosure: &cc})
			} else if c.Lifted != nil && now.Sub(*c.Lifted) < 24*time.Hour || c.Until != nil && now.Sub(*c.Until) < 24*time.Hour {
				v.Lifted = append(v.Lifted, &cc)
			}
		}
	})
	sort.Slice(v.Closures, func(i, j int) bool { return v.Closures[i].At.After(v.Closures[j].At) })
	cfg := s.nextConfig().Traffic
	s.next.mu.Lock()
	v.Version = s.next.roads.version
	for i := range v.Closures {
		g := s.next.roads.closures[v.Closures[i].ID]
		if g.Roads == nil {
			g.Roads, g.Shapes = []string{}, [][][2]float64{}
		}
		if !withGeometry {
			g.Shapes = nil
		}
		v.Closures[i].closureGeo = g
	}
	t := s.next.traffic
	v.Traffic = trafficView{On: cfg.On, Kind: cfg.Kind, At: t.At, Error: t.Error, Jams: []jam{}}
	if withGeometry {
		v.Traffic.Jams = append(v.Traffic.Jams, t.Jams...)
	}
	s.next.mu.Unlock()
	return v
}

// roadsBrief is what the board carries: closures without their geometry, and how many slowdowns.
type roadsBrief struct {
	Version  uint64        `json:"version"`
	Closures []closureView `json:"closures"`
	Traffic  trafficView   `json:"traffic"`
	Jams     int           `json:"jams"`
}

func (s *Server) roadsBriefView() roadsBrief {
	v := s.roadsNowView(false)
	s.next.mu.Lock()
	n := len(s.next.traffic.Jams)
	s.next.mu.Unlock()
	return roadsBrief{Version: v.Version, Closures: v.Closures, Traffic: v.Traffic, Jams: n}
}

func (s *Server) roadRoutes() {
	s.handle("GET /api/roads", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		writeJSON(w, 200, s.roadsNowView(true))
	})
	s.handle("PUT /api/roads/closures", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var c cad.RoadClosure
		if !readJSON(w, r, &c) {
			return
		}
		id, err := s.Eng.SaveClosure(a, c)
		if err != nil {
			writeErr(w, err)
			return
		}
		s.applyRoads(true)
		s.next.mu.Lock()
		g := s.next.roads.closures[id]
		s.next.mu.Unlock()
		writeJSON(w, 200, map[string]any{"id": id, "segments": g.Segments, "roads": g.Roads})
	})
	s.handle("POST /api/roads/closures/{id}/lift", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		err := s.Eng.LiftClosure(a, r.PathValue("id"))
		if err == nil {
			s.applyRoads(true)
		}
		done(w, err)
	})
	s.handle("DELETE /api/roads/closures/{id}", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		err := s.Eng.RemoveClosure(a, r.PathValue("id"))
		if err == nil {
			s.applyRoads(true)
		}
		done(w, err)
	})
	s.handle("GET /api/admin/traffic", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		c := s.nextConfig().Traffic
		writeJSON(w, 200, map[string]any{"on": c.On, "kind": c.Kind, "url": c.URL, "tokenSet": c.Token != "", "every": c.Every, "status": s.roadsNowView(false).Traffic})
	})
	s.handle("PUT /api/admin/traffic", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b Traffic
		if !readJSON(w, r, &b) {
			return
		}
		b.URL, b.Kind = strings.TrimSpace(b.URL), strings.TrimSpace(b.Kind)
		if b.Kind != "waze" && b.Kind != "geojson" {
			fail(w, 422, "the feed is Waze for Cities (waze) or GeoJSON (geojson)")
			return
		}
		if b.On && !validOutURL(b.URL) {
			fail(w, 422, "the feed's address starts with https:// (or http://127.0.0.1 for a program on this server)")
			return
		}
		if b.Every != 0 && (b.Every < 30 || b.Every > 3600) {
			fail(w, 422, "fetch it every 30 seconds to an hour")
			return
		}
		s.mu.Lock()
		n := s.integ
		keepSecret(&n.Next.Traffic.Token, b.Token)
		n.Next.Traffic.On, n.Next.Traffic.Kind, n.Next.Traffic.URL, n.Next.Traffic.Every = b.On, b.Kind, b.URL, b.Every
		err := s.writeIntegrations(n)
		if err == nil {
			s.integ = n
		}
		s.mu.Unlock()
		if err != nil {
			writeErr(w, err)
			return
		}
		s.Eng.Record(a, cad.EvIntegration, map[string]any{"traffic": map[string]any{"on": b.On, "kind": b.Kind, "url": b.URL}})
		w.WriteHeader(http.StatusNoContent)
	})
}

// validOutURL is an address bitdispatcher may call: https, or a program on this server.
func validOutURL(u string) bool {
	return strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://127.0.0.1") || strings.HasPrefix(u, "http://localhost")
}
