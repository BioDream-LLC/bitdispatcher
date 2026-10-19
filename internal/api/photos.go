package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/imagery"
)

func (s *Server) photoRoutes() {
	s.handle("GET /api/photos", auth.Viewer, s.photoInfo)
	s.handle("GET /api/photos/tiles/{z}/{x}/{y}", auth.Viewer, s.photoTile)
	s.handle("GET /api/photos/date", auth.Viewer, s.photoDate)
	s.handle("POST /api/admin/photos/plan", auth.Admin, s.photoPlan)
	s.handle("POST /api/admin/photos/record", auth.Admin, s.photoRecord)
	s.handle("POST /api/admin/photos/cancel", auth.Admin, s.photoCancel)
}

func (s *Server) photoEvents() {
	s.Photos.Online = func() bool {
		var off bool
		s.Eng.Read(func(st *cad.State) { off = st.Settings.PhotosRecordedOnly })
		return !off
	}
	s.Photos.Recorded = func(j imagery.Job) {
		s.Eng.Record(cad.Actor{User: j.By, Role: auth.Admin}, cad.EvPhotosRecorded, map[string]any{"plan": j.Plan, "fetched": j.Fetched, "missing": j.Missing, "failed": j.Failed, "bytes": j.Bytes})
		s.photoStats.reset()
	}
	s.Photos.Failed = func(j imagery.Job) {
		s.Eng.Record(cad.Actor{User: j.By, Role: auth.Admin}, cad.EvPhotosFailed, map[string]any{"plan": j.Plan, "error": j.Error, "done": j.Done})
		s.photoStats.reset()
	}
}

// statsCache keeps the photo store's totals for a while: counting a million tiles takes a moment.
type statsCache struct {
	mu sync.Mutex
	at time.Time
	v  *imagery.Stats
}

func (c *statsCache) reset() {
	c.mu.Lock()
	c.v = nil
	c.mu.Unlock()
}

func (c *statsCache) get(ctx context.Context, st *imagery.Store, maxAge time.Duration) *imagery.Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.v != nil && time.Since(c.at) < maxAge {
		return c.v
	}
	v, err := st.Stats(ctx)
	if err != nil {
		return c.v
	}
	c.v, c.at = &v, time.Now()
	return c.v
}

// PhotoHealth tells consoles whether aerial photos can be shown.
type PhotoHealth struct {
	Available   bool    `json:"available"` // recorded, or fetched when asked
	Recorded    bool    `json:"recorded"`
	Attribution string  `json:"attribution"`
	Job         string  `json:"job,omitempty"`
	JobFrac     float64 `json:"jobFrac,omitempty"`
}

func (s *Server) photoHealth() *PhotoHealth {
	if s.Photos == nil {
		return nil
	}
	h := &PhotoHealth{}
	h.Recorded = s.Photos.Last() != nil
	h.Available = h.Recorded || s.Photos.Online()
	// Short, so the map's corner stays usable; the admin screen gives each source in full.
	var attr []string
	for _, src := range imagery.Catalog {
		attr = append(attr, src.Short)
	}
	h.Attribution = "Imagery " + strings.Join(attr, ", ")
	if j := s.Photos.Job(); j != nil && j.Finished == nil {
		h.Job = fmt.Sprintf("zoom %d", j.Zoom)
		if j.Plan.Tiles > 0 {
			h.JobFrac = float64(j.Done) / float64(j.Plan.Tiles)
		}
		h.Available = true
	}
	return h
}

// PhotoInfo is what the console and the admin screen show about the photos.
type PhotoInfo struct {
	Sources  []imagery.Source `json:"sources"`
	Recorded *imagery.Plan    `json:"recorded,omitempty"`
	Stats    *imagery.Stats   `json:"stats,omitempty"`
	Job      *imagery.Job     `json:"job,omitempty"`
	Online   bool             `json:"online"`
}

func (s *Server) photoInfo(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	age := 5 * time.Minute
	if j := s.Photos.Job(); j != nil && j.Finished == nil {
		age = 20 * time.Second
	}
	writeJSON(w, 200, PhotoInfo{Sources: imagery.Catalog, Recorded: s.Photos.Last(), Stats: s.photoStats.get(r.Context(), s.Photos.Store(), age),
		Job: s.Photos.Job(), Online: s.Photos.Online()})
}

func (s *Server) photoTile(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	z, e1 := strconv.Atoi(r.PathValue("z"))
	x, e2 := strconv.Atoi(r.PathValue("x"))
	y, e3 := strconv.Atoi(r.PathValue("y"))
	if e1 != nil || e2 != nil || e3 != nil {
		http.NotFound(w, r)
		return
	}
	b, err := s.Photos.Tile(r.Context(), z, x, y)
	if errors.Is(err, imagery.ErrNoTile) {
		w.Header().Set("Cache-Control", "private, max-age=600")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		http.Error(w, "photo error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(b))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(b)
}

func (s *Server) photoDate(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	lat, e1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lon, e2 := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	if e1 != nil || e2 != nil {
		fail(w, http.StatusBadRequest, "lat and lon")
		return
	}
	d := ""
	if s.Photos.Online() {
		d = s.Photos.DateAt(r.Context(), lat, lon)
	}
	writeJSON(w, 200, map[string]string{"date": d})
}

type photoReq struct {
	Lat             float64 `json:"lat"`
	Lon             float64 `json:"lon"`
	RadiusMiles     float64 `json:"radiusMiles"`
	CoreRadiusMiles float64 `json:"coreRadiusMiles"`
	OuterZoom       int     `json:"outerZoom"`
	CoreZoom        int     `json:"coreZoom"`
	Rate            float64 `json:"rate"`
}

func (q photoReq) plan() imagery.Plan {
	return imagery.Plan{Lat: q.Lat, Lon: q.Lon, RadiusM: q.RadiusMiles * 1609.344, CoreRadiusM: q.CoreRadiusMiles * 1609.344,
		OuterZoom: q.OuterZoom, CoreZoom: q.CoreZoom, Rate: q.Rate}
}

func (s *Server) photoPlan(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var q photoReq
	if !readJSON(w, r, &q) {
		return
	}
	p, err := imagery.MakePlan(q.plan())
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	p.Date = s.Photos.DateAt(ctx, p.Lat, p.Lon)
	writeJSON(w, 200, p)
}

func (s *Server) photoRecord(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var q photoReq
	if !readJSON(w, r, &q) {
		return
	}
	p := q.plan()
	if err := s.Photos.Record(p, a.User); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	j := s.Photos.Job()
	s.Eng.Record(a, cad.EvPhotosRecording, map[string]any{"plan": j.Plan})
	writeJSON(w, 202, j)
}

func (s *Server) photoCancel(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	if !s.Photos.Cancel() {
		fail(w, http.StatusConflict, "no photo recording is running")
		return
	}
	writeJSON(w, 200, s.Photos.Job())
}
