package api

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// Attachments on an incident: pictures, documents, audio and video clips, kept beside the ledger with
// their SHA-256 in it. Crews add them from the terminal to incidents they are on; dispatchers to any;
// a supervisor removes one with why (it is hidden, not deleted: an incident's files are records).

// maxFile is the largest file taken.
const maxFile = 25 << 20

// fileTypes are the kinds taken, by what the bytes are (not what the sender says).
var fileTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true, "image/gif": true, "application/pdf": true,
	"text/plain": true, "audio/mpeg": true, "audio/wave": true, "video/mp4": true, "video/webm": true, "audio/webm": true}

var reIncNumber = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

func (s *Server) fileRoutes() {
	s.handle("POST /api/incidents/{n}/files", auth.Viewer, s.addIncidentFile)
	s.handle("GET /api/incidents/{n}/files/{id}", auth.Viewer, s.incidentFile)
	s.handle("DELETE /api/incidents/{n}/files/{id}", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct{ Why string }
		if !readJSON(w, r, &b) {
			return
		}
		if err := s.Eng.HideIncidentFile(a, r.PathValue("n"), r.PathValue("id"), b.Why); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// incidentFileDir is where an incident's files are kept: beside the live board's ledger.
func (s *Server) incidentFileDir(number string) string {
	dir := s.DataDir
	if s.liveServer != nil {
		dir = s.liveServer.DataDir
	}
	return filepath.Join(dir, "incident-files", number)
}

func (s *Server) addIncidentFile(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	n := r.PathValue("n")
	if !reIncNumber.MatchString(n) {
		fail(w, 404, "no incident")
		return
	}
	if d, ok := s.Auth.Demo(); ok && d.Username == se.Username {
		fail(w, 403, "the demonstration account cannot add files")
		return
	}
	unit := ""
	switch {
	case se.Role == auth.Field:
		unit = se.Console
		on := false
		s.Eng.Read(func(st *cad.State) {
			if in := st.Incidents[n]; in != nil {
				on = slices.ContainsFunc(in.Units, func(as cad.Assignment) bool { return as.Unit == unit })
			}
		})
		if !on {
			fail(w, 403, "a crew adds files to incidents it is on")
			return
		}
	case !auth.AtLeast(se.Role, auth.Dispatcher):
		fail(w, 403, "a crew, a dispatcher or a supervisor adds files")
		return
	}
	known := false
	s.Eng.Read(func(st *cad.State) { _, known = st.Incidents[n] })
	if !known {
		fail(w, 404, "no incident "+n)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxFile))
	if err != nil {
		fail(w, 413, "a file is up to 25 MB")
		return
	}
	if len(body) == 0 {
		fail(w, 422, "the file is empty")
		return
	}
	typ, _, _ := mime.ParseMediaType(http.DetectContentType(body))
	if !fileTypes[typ] {
		fail(w, 415, "send a picture (JPEG, PNG, WebP, GIF), a PDF, plain text, or an audio or video clip (MP3, WAV, MP4, WebM)")
		return
	}
	if !s.scanned(w, r, a, body) {
		return
	}
	sum := sha256.Sum256(body)
	f := cad.IncidentFile{ID: auth.RandomToken(9), Name: r.URL.Query().Get("name"), Caption: r.URL.Query().Get("caption"), Type: typ,
		SHA256: hex.EncodeToString(sum[:]), Size: int64(len(body)), Unit: unit}
	dir := s.incidentFileDir(n)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		writeErr(w, err)
		return
	}
	path := filepath.Join(dir, f.ID)
	if err := os.WriteFile(path, body, 0o640); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.Eng.AddIncidentFile(a, n, f); err != nil {
		os.Remove(path)
		writeErr(w, err)
		return
	}
	go s.indexFile(path, typ, body)
	writeJSON(w, 200, map[string]string{"id": f.ID})
}

// incidentFile serves a file, checked against the SHA-256 the ledger has for it. Pictures, audio and video
// are shown in the page; anything else is downloaded. Hidden files are for supervisors.
func (s *Server) incidentFile(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	n, id := r.PathValue("n"), r.PathValue("id")
	var f *cad.IncidentFile
	s.Eng.Read(func(st *cad.State) {
		if in := st.Incidents[n]; in != nil {
			for _, x := range in.Files {
				if x.ID == id {
					c := x
					f = &c
				}
			}
		}
	})
	if f == nil || (f.Hidden && !auth.AtLeast(se.Role, auth.Supervisor)) {
		fail(w, 404, "no such file")
		return
	}
	b, err := os.ReadFile(filepath.Join(s.incidentFileDir(n), f.ID))
	if err != nil {
		fail(w, 404, "the file is missing on this server")
		return
	}
	if sum := sha256.Sum256(b); hex.EncodeToString(sum[:]) != f.SHA256 {
		fail(w, 409, "the file was changed since it was added")
		return
	}
	disp := "attachment"
	if strings.HasPrefix(f.Type, "image/") || strings.HasPrefix(f.Type, "audio/") || strings.HasPrefix(f.Type, "video/") {
		disp = "inline"
	}
	w.Header().Set("Content-Type", f.Type)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": f.Name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; img-src 'self'; media-src 'self'")
	w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
	w.Write(b)
}
