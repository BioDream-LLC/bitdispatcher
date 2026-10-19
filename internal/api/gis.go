package api

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/gis"
	"github.com/biodream-llc/bitdispatcher/internal/mapdata"
)

func (s *Server) gisRoutes() {
	s.handle("GET /api/map/gis/{z}/{x}/{y}", auth.Viewer, s.gisTile)
	s.handle("GET /api/admin/gis", auth.Admin, s.gisInfo)
	s.handle("PUT /api/admin/gis/layers", auth.Admin, s.gisSaveLayers)
	s.handle("POST /api/admin/gis/inspect", auth.Admin, s.gisInspect)
	s.handle("POST /api/admin/gis/upload", auth.Admin, s.gisUpload)
	s.handle("POST /api/admin/gis/import", auth.Admin, s.gisImport)
	s.handle("POST /api/admin/gis/cancel", auth.Admin, s.gisCancel)
	s.GIS.Imported = func(j gis.Job) {
		s.Eng.Record(cad.Actor{User: j.By, Role: auth.Admin}, cad.EvGISImported, map[string]any{"stats": j.Stats})
	}
	s.GIS.Failed = func(j gis.Job) {
		s.Eng.Record(cad.Actor{User: j.By, Role: auth.Admin}, cad.EvGISFailed, map[string]any{"error": j.Error, "layer": j.Layer})
	}
}

// gisArea is the area GIS layers are read for: the recorded map's.
func (s *Server) gisArea() (mapdata.Area, bool) {
	if s.Maps == nil || s.Maps.Store() == nil {
		return mapdata.Area{}, false
	}
	return s.Maps.Store().Area, true
}

func (s *Server) gisTile(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	z, e1 := strconv.Atoi(r.PathValue("z"))
	x, e2 := strconv.Atoi(r.PathValue("x"))
	y, e3 := strconv.Atoi(strings.TrimSuffix(r.PathValue("y"), ".pbf"))
	st := s.GIS.Store()
	if e1 != nil || e2 != nil || e3 != nil || st == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	b, err := st.Tile(r.Context(), z, x, y)
	if err != nil {
		http.Error(w, "tile error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=600")
	if len(b) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.mapbox-vector-tile")
	w.Header().Set("Content-Encoding", "gzip")
	w.Write(b)
}

// GISInfo is the admin screen's picture.
type GISInfo struct {
	Layers  []gis.Layer         `json:"layers"`
	Kinds   map[string][]string `json:"kinds"`
	Job     *gis.Job            `json:"job,omitempty"`
	Built   *time.Time          `json:"built,omitempty"`
	Stats   *mapdata.Stats      `json:"stats,omitempty"`
	Source  string              `json:"source,omitempty"`
	Uploads []string            `json:"uploads"`
	HasArea bool                `json:"hasArea"`
}

func (s *Server) gisInfo(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	v := GISInfo{Layers: s.GIS.Layers(), Kinds: gis.KindFields, Job: s.GIS.Job(), Uploads: []string{}}
	if st := s.GIS.Store(); st != nil {
		b, stats := st.Built, st.Stats
		v.Built, v.Stats, v.Source = &b, &stats, st.Source
	}
	if es, err := os.ReadDir(s.GIS.Dir + "/uploads"); err == nil {
		for _, e := range es {
			v.Uploads = append(v.Uploads, e.Name())
		}
	}
	_, v.HasArea = s.gisArea()
	writeJSON(w, 200, v)
}

func (s *Server) gisSaveLayers(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var ls []gis.Layer
	if !readJSON(w, r, &ls) {
		return
	}
	if err := s.GIS.SaveLayers(ls); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	s.Eng.Record(a, cad.EvIntegration, map[string]any{"gisLayers": ls})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) gisInspect(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		URL  string `json:"url"`
		File string `json:"file"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	var info gis.Info
	var err error
	if b.File != "" {
		p, perr := s.GIS.UploadPath(b.File)
		if perr != nil {
			fail(w, http.StatusBadRequest, perr.Error())
			return
		}
		info, err = gis.InspectGeoJSON(p)
	} else {
		area, ok := s.gisArea()
		if !ok {
			fail(w, http.StatusConflict, "record the map first: GIS layers are read for the recorded area")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		info, err = gis.InspectArcGIS(ctx, s.GIS.Agent, b.URL, area)
	}
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, 200, info)
}

func (s *Server) gisUpload(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	p, err := s.GIS.UploadPath(r.URL.Query().Get("name"))
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	body := http.MaxBytesReader(w, r.Body, 1<<30)
	tmp := p + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		writeErr(w, err)
		return
	}
	_, err = io.Copy(f, body)
	f.Close()
	if err != nil {
		os.Remove(tmp)
		fail(w, http.StatusBadRequest, "the upload did not finish: "+err.Error())
		return
	}
	if err := os.Rename(tmp, p); err != nil {
		writeErr(w, err)
		return
	}
	var info gis.Info
	if ext := strings.ToLower(filepath.Ext(p)); ext == ".csv" || ext == ".tsv" || ext == ".txt" {
		info, err = gis.InspectCSV(p)
	} else {
		info, err = gis.InspectGeoJSON(p)
	}
	if err != nil {
		os.Remove(p)
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, 200, info)
}

func (s *Server) gisImport(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	area, ok := s.gisArea()
	if !ok {
		fail(w, http.StatusConflict, "record the map first: GIS layers are read for the recorded area")
		return
	}
	if err := s.GIS.Import(area, a.User); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	s.Eng.Record(a, cad.EvGISImporting, map[string]any{"layers": s.GIS.Layers()})
	writeJSON(w, 202, s.GIS.Job())
}

func (s *Server) gisCancel(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	if !s.GIS.Cancel() {
		fail(w, http.StatusConflict, "no import is running")
		return
	}
	writeJSON(w, 200, s.GIS.Job())
}
