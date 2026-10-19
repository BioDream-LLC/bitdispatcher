package api

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/mapdata"
)

// MapHealth is the recorded map's state, shown on every console.
type MapHealth struct {
	Recorded bool          `json:"recorded"`
	Area     *mapdata.Area `json:"area,omitempty"`
	Built    *time.Time    `json:"built,omitempty"`
	Roads    bool          `json:"roads"`             // the road network is loaded for routing
	GIS      bool          `json:"gis,omitempty"`     // county GIS layers are imported
	Job      string        `json:"job,omitempty"`     // a recording under way: its phase
	JobFrac  float64       `json:"jobFrac,omitempty"` // and progress
}

func (s *Server) mapHealth() *MapHealth {
	if s.Maps == nil {
		return nil
	}
	h := &MapHealth{}
	if st := s.Maps.Store(); st != nil {
		a, b := st.Area, st.Built
		h.Recorded, h.Area, h.Built = true, &a, &b
		h.Roads = true
	}
	if j := s.Maps.Job(); j != nil && j.Finished == nil {
		h.Job, h.JobFrac = j.Progress.Phase, j.Progress.Fraction
	}
	h.GIS = s.GIS != nil && s.GIS.Store() != nil
	return h
}

// ---- road routing for recommendations and the map ----

// treeCache keeps recent searches backwards from a destination, so a recommendation and the route lines
// drawn for it share one search.
type treeCache struct {
	mu    sync.Mutex
	store *mapdata.Store
	trees map[[2]int32]treeEntry
}

type treeEntry struct {
	t  *mapdata.Tree
	at time.Time
}

// routeSeconds is how far a search reaches: two hours of driving covers mutual aid across the area.
const routeSeconds = 7200

func (c *treeCache) tree(st *mapdata.Store, lat, lon float64) (*mapdata.Tree, error) {
	r, err := st.Router()
	if err != nil {
		return nil, err
	}
	k := [2]int32{int32(math.Round(lat * 1e5)), int32(math.Round(lon * 1e5))}
	c.mu.Lock()
	if c.store != st {
		c.store, c.trees = st, map[[2]int32]treeEntry{}
	}
	if e, ok := c.trees[k]; ok && time.Since(e.at) < 2*time.Minute {
		c.mu.Unlock()
		return e.t, nil
	}
	c.mu.Unlock()
	t := r.To(lat, lon, routeSeconds)
	c.mu.Lock()
	for kk, e := range c.trees {
		if time.Since(e.at) > 2*time.Minute {
			delete(c.trees, kk)
		}
	}
	if len(c.trees) < 32 {
		c.trees[k] = treeEntry{t, time.Now()}
	}
	c.mu.Unlock()
	return t, nil
}

// mapRouter gives the CAD engine driving times over the recorded map.
type mapRouter struct{ s *Server }

func (m mapRouter) TravelTo(lat, lon float64) (cad.TravelFunc, string) {
	st := m.s.Maps.Store()
	if st == nil || !st.Contains(lat, lon) {
		return nil, ""
	}
	t, err := m.s.trees.tree(st, lat, lon)
	if err != nil {
		return nil, ""
	}
	label := fmt.Sprintf("Drive over the recorded road network (OpenStreetMap, recorded %s): posted or typical speeds, one-way streets obeyed",
		st.Built.Local().Format("2 Jan 2006"))
	return func(fl, fo float64) (float64, float64, []string, bool) {
		rt, ok := t.From(fl, fo, true)
		if !ok {
			return 0, 0, nil, false
		}
		return rt.Metres, rt.Seconds, rt.Via, true
	}, label
}

// ---- handlers ----

func (s *Server) mapStore(w http.ResponseWriter) *mapdata.Store {
	if s.Maps == nil {
		fail(w, http.StatusNotFound, "maps are not available")
		return nil
	}
	st := s.Maps.Store()
	if st == nil {
		fail(w, http.StatusNotFound, "no map is recorded yet: an administrator records one in Admin › Map")
		return nil
	}
	return st
}

func (s *Server) vectorTile(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	z, e1 := strconv.Atoi(r.PathValue("z"))
	x, e2 := strconv.Atoi(r.PathValue("x"))
	y, e3 := strconv.Atoi(strings.TrimSuffix(r.PathValue("y"), ".pbf"))
	if e1 != nil || e2 != nil || e3 != nil {
		http.NotFound(w, r)
		return
	}
	st := s.Maps.Store()
	if st == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	b, err := st.Tile(r.Context(), z, x, y)
	if err != nil {
		s.Log.Warn("map: tile", "tile", mapdata.TileName(z, x, y), "err", err)
		http.Error(w, "tile error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	if len(b) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.mapbox-vector-tile")
	w.Header().Set("Content-Encoding", "gzip")
	w.Write(b)
}

func (s *Server) glyphs(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	b := mapdata.Glyphs(r.PathValue("stack"), r.PathValue("range"))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Type", "application/x-protobuf")
	// A range the embedded fonts do not cover is an empty glyph set, which the map accepts.
	w.Write(b)
}

func (s *Server) mapInfo(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	type info struct {
		Recorded bool           `json:"recorded"`
		Area     *mapdata.Area  `json:"area,omitempty"`
		Built    *time.Time     `json:"built,omitempty"`
		Source   string         `json:"source,omitempty"`
		Stats    *mapdata.Stats `json:"stats,omitempty"`
		Job      *mapdata.Job   `json:"job,omitempty"`
		MaxZoom  int            `json:"maxZoom"`
	}
	v := info{MaxZoom: mapdata.MaxZoom}
	if s.Maps != nil {
		if st := s.Maps.Store(); st != nil {
			ar, b, stats := st.Area, st.Built, st.Stats
			v.Recorded, v.Area, v.Built, v.Source, v.Stats = true, &ar, &b, st.Source, &stats
		}
		v.Job = s.Maps.Job()
	}
	writeJSON(w, 200, v)
}

func floatParam(r *http.Request, k string) (float64, bool) {
	v, err := strconv.ParseFloat(r.URL.Query().Get(k), 64)
	return v, err == nil && !math.IsNaN(v) && !math.IsInf(v, 0)
}

func (s *Server) mapSearch(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	st := s.mapStore(w)
	if st == nil {
		return
	}
	lat, _ := floatParam(r, "lat")
	lon, _ := floatParam(r, "lon")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	q := r.URL.Query().Get("q")
	if len(q) > 200 {
		fail(w, http.StatusBadRequest, "the search is too long")
		return
	}
	res, err := s.search(r.Context(), st, q, lat, lon, limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, res)
}

// search asks the county GIS first and the recorded map second: a county address point is the
// authoritative one, and the same address from OpenStreetMap close by is left out.
func (s *Server) search(ctx context.Context, st *mapdata.Store, q string, lat, lon float64, limit int) ([]mapdata.Result, error) {
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	res, err := st.Search(ctx, q, lat, lon, limit)
	if err != nil || s.GIS == nil {
		return res, err
	}
	g := s.GIS.Store()
	if g == nil {
		return res, nil
	}
	county, err := g.Search(ctx, q, lat, lon, limit)
	if err != nil || len(county) == 0 {
		return res, nil
	}
	out := []mapdata.Result{}
	for _, c := range county {
		if c.Kind == "address" && c.Exact {
			c.Source = firstNonEmptyStr(c.Source, "county")
			out = append(out, c)
		}
	}
	// The map's own exact answers, then the county's placed by address range, then the rest.
	addMap := func(exact bool) {
		for _, r := range res {
			if r.Exact != exact {
				continue
			}
			dup := false
			for _, c := range out {
				if r.Kind == c.Kind && mapdata.Distance(r.Lat, r.Lon, c.Lat, c.Lon) < 60 {
					dup = true
				}
			}
			if !dup {
				out = append(out, r)
			}
		}
	}
	addMap(true)
	for _, c := range county {
		if c.Ranged {
			c.Source = "county"
			out = append(out, c)
		}
	}
	addMap(false)
	// County names and near-miss numbers after the map's, unless the map had nothing.
	for _, c := range county {
		if !(c.Kind == "address" && c.Exact) && !c.Ranged {
			c.Source = "county"
			dup := false
			for _, o := range out {
				if o.Label == c.Label && mapdata.Distance(o.Lat, o.Lon, c.Lat, c.Lon) < 200 {
					dup = true
				}
			}
			if !dup {
				out = append(out, c)
			}
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// mapZones are the county's emergency service zones, response areas or beats as GeoJSON; an empty
// collection when the recorded map has none (the boards then count by station area).
func (s *Server) mapZones(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	empty := map[string]any{"type": "FeatureCollection", "features": []any{}}
	var st *mapdata.Store
	if s.Maps != nil {
		st = s.Maps.Store()
	}
	if st == nil {
		writeJSON(w, 200, empty)
		return
	}
	zs, err := st.Zones()
	if err != nil {
		writeJSON(w, 200, empty)
		return
	}
	writeJSON(w, 200, mapdata.ZonesGeoJSON(zs))
}

func (s *Server) mapReverse(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	st := s.mapStore(w)
	if st == nil {
		return
	}
	lat, ok1 := floatParam(r, "lat")
	lon, ok2 := floatParam(r, "lon")
	if !ok1 || !ok2 {
		fail(w, http.StatusBadRequest, "lat and lon are needed")
		return
	}
	if !st.Contains(lat, lon) {
		fail(w, http.StatusNotFound, "that point is outside the recorded map")
		return
	}
	// A county address point first, then one of the map's, then the number the county's address
	// ranges give, then the nearest road.
	var ranged *mapdata.Result
	if s.GIS != nil {
		if g := s.GIS.Store(); g != nil {
			if c, err := g.Reverse(r.Context(), lat, lon); err == nil && (c.Exact || c.Ranged) {
				c.Source = firstNonEmptyStr(c.Source, "county")
				if c.Exact {
					writeJSON(w, 200, c)
					return
				}
				ranged = &c
			}
		}
	}
	res, err := st.Reverse(r.Context(), lat, lon)
	if ranged != nil && (err != nil || !res.Exact) {
		writeJSON(w, 200, ranged)
		return
	}
	if err != nil {
		fail(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

// RouteView is one drive for the map.
type RouteView struct {
	ID      string       `json:"id"`
	OK      bool         `json:"ok"`
	Seconds float64      `json:"seconds,omitempty"`
	Metres  float64      `json:"metres,omitempty"`
	Via     []string     `json:"via,omitempty"`
	Path    [][2]float64 `json:"path,omitempty"` // lon, lat
}

func (s *Server) mapRoutes(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	st := s.mapStore(w)
	if st == nil {
		return
	}
	var req struct {
		To   point   `json:"to"`
		From []point `json:"from"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if len(req.From) > 60 {
		fail(w, http.StatusBadRequest, "at most 60 routes at once")
		return
	}
	out := []RouteView{}
	if !st.Contains(req.To.Lat, req.To.Lon) {
		writeJSON(w, 200, out)
		return
	}
	t, err := s.trees.tree(st, req.To.Lat, req.To.Lon)
	if err != nil {
		writeErr(w, err)
		return
	}
	for _, f := range req.From {
		v := RouteView{ID: f.ID}
		if rt, ok := t.From(f.Lat, f.Lon, true); ok {
			v.OK, v.Seconds, v.Metres, v.Via = true, math.Round(rt.Seconds), math.Round(rt.Metres), rt.Via
			v.Path = make([][2]float64, 0, len(rt.Path))
			for _, p := range thin(rt.Path) {
				lat, lon := p.Deg()
				v.Path = append(v.Path, [2]float64{math.Round(lon*1e6) / 1e6, math.Round(lat*1e6) / 1e6})
			}
		}
		out = append(out, v)
	}
	writeJSON(w, 200, out)
}

type point struct {
	ID  string  `json:"id,omitempty"`
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// thin drops points closer than about a metre to the last one kept, for a smaller response.
func thin(p []mapdata.Pt) []mapdata.Pt {
	if len(p) < 3 {
		return p
	}
	out := []mapdata.Pt{p[0]}
	for _, q := range p[1 : len(p)-1] {
		l := out[len(out)-1]
		if abs(int64(q.Lat)-int64(l.Lat))+abs(int64(q.Lon)-int64(l.Lon)) > 100 {
			out = append(out, q)
		}
	}
	return append(out, p[len(p)-1])
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// ---- administration: recording an area ----

type areaReq struct {
	Name        string  `json:"name"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	RadiusMiles float64 `json:"radiusMiles"`
}

func (q areaReq) area() (mapdata.Area, error) {
	if q.RadiusMiles < 1 || q.RadiusMiles > 250 {
		return mapdata.Area{}, &cad.Problem{Field: "radiusMiles", Message: "the radius must be 1 to 250 miles"}
	}
	if q.Lat < -85 || q.Lat > 85 || q.Lon < -180 || q.Lon > 180 || (q.Lat == 0 && q.Lon == 0) {
		return mapdata.Area{}, &cad.Problem{Field: "lat", Message: "choose the centre of the area"}
	}
	name := strings.TrimSpace(q.Name)
	if name == "" {
		name = fmt.Sprintf("%.4f, %.4f", q.Lat, q.Lon)
	}
	if len(name) > 100 {
		return mapdata.Area{}, &cad.Problem{Field: "name", Message: "the name is too long"}
	}
	return mapdata.Area{Name: name, Lat: q.Lat, Lon: q.Lon, RadiusM: q.RadiusMiles * 1609.344}, nil
}

func (s *Server) mapPlan(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var q areaReq
	if !readJSON(w, r, &q) {
		return
	}
	ar, err := q.area()
	if err != nil {
		writeErr(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	ex, err := s.Maps.Plan(ctx, ar)
	if err != nil {
		fail(w, http.StatusBadGateway, "could not list the OpenStreetMap extracts: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"area": ar, "extracts": ex})
}

func (s *Server) mapRecord(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var q struct {
		areaReq
		Extracts []mapdata.Extract `json:"extracts"`
	}
	if !readJSON(w, r, &q) {
		return
	}
	ar, err := q.area()
	if err != nil {
		writeErr(w, err)
		return
	}
	// Extracts come from the plan; only Geofabrik's own download URLs are accepted, so the server is never
	// pointed at an arbitrary address.
	for _, e := range q.Extracts {
		if !strings.HasPrefix(e.URL, "https://download.geofabrik.de/") || !strings.HasSuffix(e.URL, ".osm.pbf") {
			fail(w, http.StatusBadRequest, "extracts must be Geofabrik downloads from the plan")
			return
		}
	}
	if err := s.Maps.Record(ar, q.Extracts, a.User); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	s.Eng.Record(a, cad.EvMapRecording, map[string]any{"area": ar, "extracts": q.Extracts})
	writeJSON(w, 202, s.Maps.Job())
}

func (s *Server) mapUpdate(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	if err := s.Maps.Update(a.User); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	j := s.Maps.Job()
	s.Eng.Record(a, cad.EvMapRecording, map[string]any{"area": j.Area, "update": true})
	writeJSON(w, 202, j)
}

func (s *Server) mapCancel(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	if !s.Maps.Cancel() {
		fail(w, http.StatusConflict, "no recording is running")
		return
	}
	writeJSON(w, 200, s.Maps.Job())
}

// mapEvents records the outcome of a recording in the ledger.
func (s *Server) mapEvents() {
	s.Maps.Recorded = func(j mapdata.Job) {
		s.Eng.Record(cad.Actor{User: j.By, Role: auth.Admin}, cad.EvMapRecorded, map[string]any{"area": j.Area, "extracts": j.Extracts, "stats": j.Stats})
		s.Eng.SetRouter(mapRouter{s})
		s.Eng.SetGeocoder(mapGeocoder{s})
	}
	s.Maps.Failed = func(j mapdata.Job) {
		s.Eng.Record(cad.Actor{User: j.By, Role: auth.Admin}, cad.EvMapFailed, map[string]any{"area": j.Area, "error": j.Error})
	}
}

// mapGeocoder lets the command line place an incident at an address the recorded map knows exactly.
type mapGeocoder struct{ s *Server }

func (m mapGeocoder) Geocode(text string) (float64, float64, string, bool) {
	st := m.s.Maps.Store()
	if st == nil {
		return 0, 0, "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	res, err := m.s.search(ctx, st, text, 0, 0, 2)
	if err != nil || len(res) == 0 || !res[0].Exact {
		return 0, 0, "", false
	}
	// Two exact answers far apart (the same number on two Main Streets) are for the dispatcher to choose.
	if len(res) > 1 && res[1].Exact && mapdata.Distance(res[0].Lat, res[0].Lon, res[1].Lat, res[1].Lon) > 200 {
		return 0, 0, "", false
	}
	label := res[0].Label
	if res[0].Detail != "" {
		label += ", " + res[0].Detail
	}
	return res[0].Lat, res[0].Lon, label, true
}
