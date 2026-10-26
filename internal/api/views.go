package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/views"
)

// Views: pictures of a place from the side (oblique aerial photos, street-level photos) and the
// agency's own pre-plan photos of each premise.
func (s *Server) viewRoutes() {
	s.handle("GET /api/views", auth.Viewer, s.findViews)
	s.handle("GET /api/views/picture/{ref}", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if err := s.Views.Picture(r.Context(), r.PathValue("ref"), w); err != nil {
			if errors.Is(err, views.ErrUnknown) {
				fail(w, 404, err.Error())
				return
			}
			fail(w, 502, err.Error())
		}
	})
	s.handle("GET /api/admin/views", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		writeJSON(w, 200, s.viewsConfigView())
	})
	s.handle("PUT /api/admin/views", auth.Admin, s.saveViewsConfig)
	s.handle("POST /api/admin/views/test", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct{ Lat, Lon float64 }
		if !readJSON(w, r, &b) {
			return
		}
		if b.Lat == 0 && b.Lon == 0 {
			s.Eng.Read(func(st *cad.State) { b.Lat, b.Lon = st.Settings.MapCenter[0], st.Settings.MapCenter[1] })
		}
		writeJSON(w, 200, map[string]any{"results": s.Views.Test(r.Context(), b.Lat, b.Lon)})
	})

	s.handle("POST /api/premises/{id}/photos", auth.Viewer, s.addPremisePhoto)
	s.handle("GET /api/premises/{id}/photos/{photo}", auth.Viewer, s.premisePhoto)
	s.handle("DELETE /api/premises/{id}/photos/{photo}", auth.Supervisor, s.removePremisePhoto)
}

// findViews looks a place up with each provider that is on. The answer says which providers could
// not answer, so a dispatcher knows a missing side is missing, not hidden.
func (s *Server) findViews(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	lat, err1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lon, err2 := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 || (lat == 0 && lon == 0) {
		fail(w, 422, "give the place's lat and lon")
		return
	}
	writeJSON(w, 200, s.Views.Find(r.Context(), lat, lon))
}

// ViewsConfigView is the views configuration with its secrets replaced by whether they are set.
type ViewsConfigView struct {
	EagleViewOn       bool   `json:"eagleviewOn"`
	EagleViewClientID string `json:"eagleviewClientId"`
	EagleViewSecret   bool   `json:"eagleviewSecretSet"`
	EagleViewSandbox  bool   `json:"eagleviewSandbox"`
	PanoramaxOn       bool   `json:"panoramaxOn"`
	PanoramaxURL      string `json:"panoramaxUrl"`
	MapillaryOn       bool   `json:"mapillaryOn"`
	MapillaryToken    bool   `json:"mapillaryTokenSet"`
	GoogleLink        bool   `json:"googleLink"`
	GoogleKey         bool   `json:"googleKeySet"`
}

func (s *Server) viewsConfigView() ViewsConfigView {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.integ.Views
	return ViewsConfigView{EagleViewOn: c.EagleView.On, EagleViewClientID: c.EagleView.ClientID, EagleViewSecret: c.EagleView.ClientSecret != "",
		EagleViewSandbox: c.EagleView.Sandbox, PanoramaxOn: c.Panoramax.On, PanoramaxURL: c.Panoramax.URL,
		MapillaryOn: c.Mapillary.On, MapillaryToken: c.Mapillary.Token != "", GoogleLink: c.GoogleLink, GoogleKey: c.GoogleKey != ""}
}

// saveViewsConfig changes the providers. A secret left empty keeps the current one; "-" clears it.
func (s *Server) saveViewsConfig(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		EagleViewOn       bool   `json:"eagleviewOn"`
		EagleViewClientID string `json:"eagleviewClientId"`
		EagleViewSecret   string `json:"eagleviewSecret"`
		EagleViewSandbox  bool   `json:"eagleviewSandbox"`
		PanoramaxOn       bool   `json:"panoramaxOn"`
		PanoramaxURL      string `json:"panoramaxUrl"`
		MapillaryOn       bool   `json:"mapillaryOn"`
		MapillaryToken    string `json:"mapillaryToken"`
		GoogleLink        bool   `json:"googleLink"`
		GoogleKey         string `json:"googleKey"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	b.PanoramaxURL = strings.TrimRight(strings.TrimSpace(b.PanoramaxURL), "/")
	if b.PanoramaxURL != "" && !strings.HasPrefix(b.PanoramaxURL, "https://") {
		fail(w, 422, "the Panoramax address starts with https://")
		return
	}
	s.mu.Lock()
	n := s.integ
	keep := func(dst *string, v string) {
		switch v = strings.TrimSpace(v); v {
		case "":
		case "-":
			*dst = ""
		default:
			*dst = v
		}
	}
	v := &n.Views
	v.EagleView.On, v.EagleView.ClientID, v.EagleView.Sandbox = b.EagleViewOn, strings.TrimSpace(b.EagleViewClientID), b.EagleViewSandbox
	keep(&v.EagleView.ClientSecret, b.EagleViewSecret)
	v.Panoramax.On, v.Panoramax.URL = b.PanoramaxOn, b.PanoramaxURL
	v.Mapillary.On = b.MapillaryOn
	keep(&v.Mapillary.Token, b.MapillaryToken)
	v.GoogleLink = b.GoogleLink
	keep(&v.GoogleKey, b.GoogleKey)
	if v.EagleView.On && (v.EagleView.ClientID == "" || v.EagleView.ClientSecret == "") {
		s.mu.Unlock()
		fail(w, 422, "EagleView needs the application's client ID and secret")
		return
	}
	if v.Mapillary.On && v.Mapillary.Token == "" {
		s.mu.Unlock()
		fail(w, 422, "Mapillary needs a client token")
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
	s.Views.Configure(n.Views)
	s.Eng.Record(a, cad.EvIntegration, map[string]any{"views": map[string]any{"eagleview": v.EagleView.On, "sandbox": v.EagleView.Sandbox,
		"panoramax": v.Panoramax.On, "panoramaxUrl": v.Panoramax.URL, "mapillary": v.Mapillary.On, "googleLink": v.GoogleLink}})
	writeJSON(w, 200, s.viewsConfigView())
}

// premisePhotoDir is where a premise's pictures are kept: beside the live board's ledger, so a
// training board shows the same pictures.
func (s *Server) premisePhotoDir(id string) string {
	dir := s.DataDir
	if s.liveServer != nil {
		dir = s.liveServer.DataDir
	}
	if id == "." || id == ".." {
		id = "_" + id // a premise ID may hold dots; never a directory above
	}
	return filepath.Join(dir, "premise-photos", id)
}

// maxPhoto is the largest picture taken: a phone's photo at full size.
const maxPhoto = 15 << 20

var photoTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}

// addPremisePhoto stores a picture sent as the request body (a crew's phone or the admin's upload).
// Crews, dispatchers and up may add pictures; the public demonstration account may not.
func (s *Server) addPremisePhoto(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	if se.Role != auth.Field && !auth.AtLeast(se.Role, auth.Dispatcher) {
		fail(w, 403, "a crew, a dispatcher or a supervisor adds pictures")
		return
	}
	if d, ok := s.Auth.Demo(); ok && d.Username == se.Username {
		fail(w, 403, "the demonstration account cannot add pictures")
		return
	}
	id := r.PathValue("id")
	if s.Eng.PremiseByID(id) == nil {
		fail(w, 404, "no premise "+id)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPhoto))
	if err != nil {
		fail(w, 413, "a picture is up to 15 MB")
		return
	}
	typ := http.DetectContentType(body)
	if !photoTypes[typ] {
		fail(w, 415, "send a JPEG, PNG or WebP picture")
		return
	}
	sum := sha256.Sum256(body)
	ph := cad.PremisePhoto{ID: auth.RandomToken(9), Caption: r.URL.Query().Get("caption"), Facing: strings.ToUpper(r.URL.Query().Get("facing")),
		Type: typ, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(body))}
	dir := s.premisePhotoDir(id)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		writeErr(w, err)
		return
	}
	path := filepath.Join(dir, ph.ID)
	if err := os.WriteFile(path, body, 0o640); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.Eng.AddPremisePhoto(a, id, ph); err != nil {
		os.Remove(path)
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, s.Eng.PremiseByID(id))
}

// premisePhoto serves a picture, checked against the SHA-256 the ledger has for it.
func (s *Server) premisePhoto(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	p := s.Eng.PremiseByID(r.PathValue("id"))
	if p == nil {
		fail(w, 404, "no premise")
		return
	}
	for _, ph := range p.Photos {
		if ph.ID != r.PathValue("photo") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.premisePhotoDir(p.ID), ph.ID))
		if err != nil {
			fail(w, 404, "the picture's file is missing on this server")
			return
		}
		if sum := sha256.Sum256(b); hex.EncodeToString(sum[:]) != ph.SHA256 {
			fail(w, 409, "the picture's file was changed since it was added")
			return
		}
		w.Header().Set("Content-Type", ph.Type)
		w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
		w.Write(b)
		return
	}
	fail(w, 404, "no such picture")
}

func (s *Server) removePremisePhoto(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	id := r.PathValue("id")
	gone, err := s.Eng.RemovePremisePhoto(a, id, r.PathValue("photo"))
	if err != nil {
		writeErr(w, err)
		return
	}
	os.Remove(filepath.Join(s.premisePhotoDir(id), gone.ID))
	writeJSON(w, 200, s.Eng.PremiseByID(id))
}
