package api

// The communications inbox (cad/comms.go): compose, address, attach, send, receive, open, acknowledge,
// update, cancel and find. Folders are Inbox, Drafts, Sent, Active BOLOs and Archived. What a person sees
// is decided on the server from who they are, their duty, console, unit and the agencies they cover.

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// readerOf is the person a session is, for addressing.
func (s *Server) readerOf(st *cad.State, se *auth.Session) cad.Reader {
	r := cad.Reader{User: se.Username, Role: se.Role, Duty: se.Duty, Console: se.Console, Since: se.Created}
	if se.Role == auth.Field {
		r.Unit = se.Console
		if u := st.Units[se.Console]; u != nil && u.Agency != "" {
			r.Agencies = []string{u.Agency}
		}
		return r
	}
	c := st.CoverageOf(se.Username, se.Console, se.Duty)
	r.Agencies = slices.Concat(c.Covered, c.Monitored)
	if len(st.Agencies) > 0 && st.AllAgencies(se.Username) && len(r.Agencies) == 0 {
		for code := range st.Agencies {
			r.Agencies = append(r.Agencies, code)
		}
	}
	return r
}

// CommView is a communication as a person sees it.
type CommView struct {
	*cad.Comm
	State    string `json:"state"`
	Opened   bool   `json:"openedByMe"`
	AckedMe  bool   `json:"ackedByMe"`
	NeedsAck bool   `json:"needsAck"`
	Mine     bool   `json:"mine"`
}

func commView(st *cad.State, c *cad.Comm, user string, now time.Time) CommView {
	v := CommView{Comm: c, State: c.State(now), Mine: strings.EqualFold(c.From, user)}
	_, v.Opened = c.Opened[strings.ToLower(user)]
	_, v.AckedMe = c.Acked[strings.ToLower(user)]
	for _, r := range st.Settings.PriorityRuleList() {
		if r.Level == c.Priority {
			v.NeedsAck = r.Ack && !v.AckedMe && !v.Mine && v.State == "active"
		}
	}
	return v
}

func (s *Server) commFileDir() string {
	dir := s.DataDir
	if s.liveServer != nil {
		dir = s.liveServer.DataDir
	}
	return filepath.Join(dir, "comm-files")
}

func (s *Server) commRoutes() {
	s.handle("GET /api/comms", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		folder := firstNonEmpty(r.URL.Query().Get("folder"), "inbox")
		now := s.Eng.Now()
		out := []CommView{}
		var drafts []*cad.CommDraft
		var kinds []cad.CommKind
		var rules []cad.PriorityRule
		var deliver []string
		s.Eng.Read(func(st *cad.State) {
			rd := s.readerOf(st, se)
			kinds, rules = st.Settings.CommKindList(), st.Settings.PriorityRuleList()
			switch folder {
			case "inbox":
				for _, c := range st.Inbox(rd, now) {
					out = append(out, commView(st, c, se.Username, now))
					if _, ok := c.Delivered[strings.ToLower(se.Username)]; !ok {
						deliver = append(deliver, c.ID)
					}
				}
			case "sent":
				for i := len(st.Comms) - 1; i >= 0 && len(out) < 300; i-- {
					if c := st.Comms[i]; strings.EqualFold(c.From, se.Username) {
						out = append(out, commView(st, c, se.Username, now))
					}
				}
			case "bolos":
				for _, c := range st.Comms {
					if (c.Kind == cad.CommBOLO || c.Kind == cad.CommSafety) && c.SupersededBy == "" && c.Active(now) && st.MaySeeComm(c, rd) {
						out = append(out, commView(st, c, se.Username, now))
					}
				}
			case "archived":
				for i := len(st.Comms) - 1; i >= 0 && len(out) < 300; i-- {
					c := st.Comms[i]
					if c.SupersededBy == "" && !c.Active(now) && st.MaySeeComm(c, rd) {
						out = append(out, commView(st, c, se.Username, now))
					}
				}
			case "drafts":
				for _, d := range st.CommDrafts {
					if strings.EqualFold(d.User, se.Username) {
						drafts = append(drafts, d)
					}
				}
			}
		})
		for _, id := range deliver {
			s.Eng.CommReceipt(a, id, "delivered", s.readerFor(se))
		}
		if drafts == nil {
			drafts = []*cad.CommDraft{}
		}
		slices.SortFunc(drafts, func(x, y *cad.CommDraft) int { return y.Updated.Compare(x.Updated) })
		writeJSON(w, 200, map[string]any{"items": out, "drafts": drafts, "kinds": kinds, "priorities": rules})
	})
	s.handle("GET /api/comms/{id}", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		now := s.Eng.Now()
		var versions []CommView
		var files []*cad.CommFile
		var ok bool
		s.Eng.Read(func(st *cad.State) {
			c := st.CommByID(r.PathValue("id"))
			if c == nil || !st.MaySeeComm(c, s.readerOf(st, se)) {
				return
			}
			ok = true
			for _, v := range st.CommThread(c.Thread) {
				versions = append(versions, commView(st, v, se.Username, now))
				for _, f := range v.Files {
					if x := st.CommFiles[f]; x != nil && !slices.ContainsFunc(files, func(y *cad.CommFile) bool { return y.ID == f }) {
						files = append(files, x)
					}
				}
			}
		})
		if !ok {
			fail(w, 404, "no such communication")
			return
		}
		if files == nil {
			files = []*cad.CommFile{}
		}
		writeJSON(w, 200, map[string]any{"versions": versions, "files": files})
	})
	s.handle("POST /api/comms", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var c cad.Comm
		if !readJSON(w, r, &c) {
			return
		}
		if d, ok := s.Auth.Demo(); ok && d.Username == se.Username && c.Priority == "urgent" {
			fail(w, 403, "the demonstration account cannot send urgent communications")
			return
		}
		draft := r.URL.Query().Get("draft")
		id, err := s.Eng.SendComm(a, c)
		if err != nil {
			writeErr(w, err)
			return
		}
		if draft != "" {
			s.Eng.RemoveCommDraft(a, draft)
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("POST /api/comms/{id}/{what}", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		what := r.PathValue("what")
		if what == "ack" {
			what = "acked"
		}
		if what == "open" {
			what = "opened"
		}
		done(w, s.Eng.CommReceipt(a, r.PathValue("id"), what, s.readerFor(se)))
	})
	s.handle("PUT /api/comms/drafts", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var d cad.CommDraft
		if !readJSON(w, r, &d) {
			return
		}
		id, err := s.Eng.SaveCommDraft(a, d)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("DELETE /api/comms/drafts/{id}", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		done(w, s.Eng.RemoveCommDraft(a, r.PathValue("id")))
	})
	// Attachments: validated by their content, optionally scanned, kept with their hash.
	s.handle("POST /api/comms/files", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if d, ok := s.Auth.Demo(); ok && d.Username == se.Username {
			fail(w, 403, "the demonstration account cannot add files")
			return
		}
		if !auth.AtLeast(se.Role, auth.Dispatcher) && se.Role != auth.Field {
			fail(w, 403, "a crew, a dispatcher or a supervisor attaches files")
			return
		}
		var types []string
		limit := int64(maxFile)
		s.Eng.Read(func(st *cad.State) {
			types = st.Settings.CommFileTypes
			if st.Settings.CommFileMB > 0 && int64(st.Settings.CommFileMB)<<20 < limit {
				limit = int64(st.Settings.CommFileMB) << 20
			}
		})
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
		if err != nil {
			fail(w, 413, fmt.Sprintf("an attachment is up to %d MB", limit>>20))
			return
		}
		if len(body) == 0 {
			fail(w, 422, "the file is empty")
			return
		}
		typ, _, _ := mime.ParseMediaType(http.DetectContentType(body))
		if !fileTypes[typ] || len(types) > 0 && !slices.Contains(types, typ) {
			fail(w, 415, "that kind of file is not allowed by the agency")
			return
		}
		if addr := s.extra().ClamAV; addr != "" {
			if verdict, err := clamScan(r.Context(), addr, body); err != nil {
				fail(w, 503, "the file could not be scanned: "+err.Error())
				return
			} else if verdict != "" {
				s.Eng.Record(a, cad.EvAccessRefused, map[string]string{"file": r.URL.Query().Get("name"), "why": "virus scan: " + verdict})
				fail(w, 422, "the file was refused by the virus scan: "+verdict)
				return
			}
		}
		sum := sha256.Sum256(body)
		f := cad.CommFile{ID: auth.RandomToken(9), Name: r.URL.Query().Get("name"), Type: typ, Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])}
		if err := os.MkdirAll(s.commFileDir(), 0o750); err != nil {
			writeErr(w, err)
			return
		}
		path := filepath.Join(s.commFileDir(), f.ID)
		if err := os.WriteFile(path, body, 0o640); err != nil {
			writeErr(w, err)
			return
		}
		if err := s.Eng.AddCommFile(a, f); err != nil {
			os.Remove(path)
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, f)
	})
	s.handle("GET /api/comms/files/{id}", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		id := r.PathValue("id")
		var f *cad.CommFile
		allowed := false
		s.Eng.Read(func(st *cad.State) {
			f = st.CommFiles[id]
			if f == nil {
				return
			}
			if strings.EqualFold(f.By, se.Username) {
				allowed = true
				return
			}
			rd := s.readerOf(st, se)
			for _, c := range st.Comms {
				if slices.Contains(c.Files, id) && st.MaySeeComm(c, rd) {
					allowed = true
					return
				}
			}
		})
		if f == nil || !allowed || f.Removed && !auth.AtLeast(se.Role, auth.Supervisor) {
			fail(w, 404, "no such attachment")
			return
		}
		b, err := os.ReadFile(filepath.Join(s.commFileDir(), f.ID))
		if err != nil {
			fail(w, 404, "the file is missing on this server")
			return
		}
		if sum := sha256.Sum256(b); hex.EncodeToString(sum[:]) != f.SHA256 {
			fail(w, 409, "the file was changed since it was attached")
			return
		}
		s.Eng.OpenedCommFile(a, id)
		disp := "attachment"
		if strings.HasPrefix(f.Type, "image/") || strings.HasPrefix(f.Type, "audio/") || strings.HasPrefix(f.Type, "video/") {
			disp = "inline"
		}
		w.Header().Set("Content-Type", f.Type)
		w.Header().Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": f.Name}))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; img-src 'self'; media-src 'self'")
		w.Header().Set("Cache-Control", "private, no-store")
		w.Write(b)
	})
	s.handle("DELETE /api/comms/files/{id}", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		done(w, s.Eng.RemoveCommFile(a, r.PathValue("id"), r.URL.Query().Get("why")))
	})
}

// readerFor is the reader a session is, read from the state.
func (s *Server) readerFor(se *auth.Session) cad.Reader {
	var rd cad.Reader
	s.Eng.Read(func(st *cad.State) { rd = s.readerOf(st, se) })
	return rd
}

// clamScan sends a file to a ClamAV daemon (clamd, host:port or a unix socket path) and returns what it
// found ("" when clean).
func clamScan(ctx context.Context, addr string, body []byte) (string, error) {
	network := "tcp"
	if strings.HasPrefix(addr, "/") {
		network = "unix"
	}
	d := net.Dialer{Timeout: 5 * time.Second}
	c, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return "", err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(30 * time.Second))
	if _, err := c.Write([]byte("zINSTREAM\x00")); err != nil {
		return "", err
	}
	for i := 0; i < len(body); i += 1 << 16 {
		chunk := body[i:min(len(body), i+1<<16)]
		n := len(chunk)
		if _, err := c.Write([]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}); err != nil {
			return "", err
		}
		if _, err := c.Write(chunk); err != nil {
			return "", err
		}
	}
	if _, err := c.Write([]byte{0, 0, 0, 0}); err != nil {
		return "", err
	}
	reply, err := bufio.NewReader(c).ReadString(0)
	if err != nil && reply == "" {
		return "", err
	}
	reply = strings.TrimRight(reply, "\x00\n")
	switch {
	case strings.HasSuffix(reply, " OK"):
		return "", nil
	case strings.HasSuffix(reply, " FOUND"):
		return strings.TrimSuffix(strings.TrimPrefix(reply, "stream: "), " FOUND"), nil
	}
	return "", fmt.Errorf("clamd said %q", reply)
}

// commWatcher escalates urgent and high communications not acknowledged in time: a notice to the
// supervisors on the floor.
func (s *Server) commWatcher(ctx context.Context) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		esc, err := s.Eng.EscalateComms()
		if err != nil {
			continue
		}
		for _, c := range esc {
			s.Eng.SendComm(cad.SystemActor, cad.Comm{Kind: cad.CommAdmin, Priority: "high", Subject: "Not acknowledged: " + c.Subject,
				Body: fmt.Sprintf("%s %s sent %s by %s to %s has not been acknowledged.", strings.ToUpper(c.Priority), c.Thread, c.Issued.Format("15:04"), c.From, strings.Join(c.To, ", ")),
				To:   []string{cad.ToRole + "supervisor"}, Incident: c.Incident})
		}
	}
}
