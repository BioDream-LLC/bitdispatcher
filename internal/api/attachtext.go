package api

import (
	"context"
	"errors"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/textract"
)

// The words inside attachments, for search: kept beside each stored file as <file>.txt (never in the ledger,
// which keeps the file's hash), read once, and looked up from memory after.
type fileTextIndex struct {
	mu   sync.Mutex
	text map[string]string // path -> text ("" known empty)
	busy sync.Mutex        // one backfill at a time
}

func (s *Server) textIndex() *fileTextIndex {
	s.textsOnce.Do(func() { s.texts = &fileTextIndex{text: map[string]string{}} })
	return s.texts
}

// indexFile reads the words in a stored file and keeps them beside it. It runs after the upload has been
// answered: a slow picture never holds up the person who sent it.
func (s *Server) indexFile(path, typ string, body []byte) {
	txt, err := textract.Extract(context.Background(), typ, body, textract.Options{OCR: s.extra().OCR})
	if errors.Is(err, textract.ErrNoReader) {
		return // nothing reads it now (a picture without OCR): tried again when OCR is turned on
	}
	if err != nil {
		s.Log.Warn("attachment text", "file", filepath.Base(path), "err", err)
		txt = ""
	}
	if os.WriteFile(path+".txt", []byte(txt), 0o640) == nil {
		x := s.textIndex()
		x.mu.Lock()
		x.text[path] = txt
		x.mu.Unlock()
	}
}

// fileText is the words inside an incident's file (incident set) or a communication's attachment.
func (s *Server) fileText(incident, id string) string {
	if id == "" || strings.ContainsAny(id, `/\.`) || strings.ContainsAny(incident, `/\.`) {
		return ""
	}
	path := filepath.Join(s.commFileDir(), id)
	if incident != "" {
		path = filepath.Join(s.incidentFileDir(incident), id)
	}
	x := s.textIndex()
	x.mu.Lock()
	defer x.mu.Unlock()
	if t, ok := x.text[path]; ok {
		return t
	}
	b, err := os.ReadFile(path + ".txt")
	if err != nil {
		return "" // not read yet: not remembered, so it is found once it is
	}
	x.text[path] = string(b)
	return string(b)
}

// backfillText reads the words in every stored file that has none yet: at start, and when OCR is turned on.
func (s *Server) backfillText(ctx context.Context) {
	x := s.textIndex()
	if !x.busy.TryLock() {
		return
	}
	defer x.busy.Unlock()
	n := 0
	for _, root := range []string{filepath.Join(filepath.Dir(s.commFileDir()), "incident-files"), s.commFileDir()} {
		filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || strings.HasSuffix(p, ".txt") || ctx.Err() != nil {
				return nil
			}
			if _, err := os.Stat(p + ".txt"); err == nil {
				return nil
			}
			body, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			typ, _, _ := mime.ParseMediaType(http.DetectContentType(body))
			s.indexFile(p, typ, body)
			n++
			return nil
		})
	}
	if n > 0 {
		s.Log.Info("attachment text read", "files", n)
	}
}

// scanned checks an upload with the ClamAV daemon when one is set. It answers the request and returns false
// when the file may not be kept.
func (s *Server) scanned(w http.ResponseWriter, r *http.Request, a cad.Actor, body []byte) bool {
	addr := s.extra().ClamAV
	if addr == "" {
		return true
	}
	verdict, err := clamScan(r.Context(), addr, body)
	if err != nil {
		fail(w, 503, "the file could not be scanned: "+err.Error())
		return false
	}
	if verdict != "" {
		s.Eng.Record(a, cad.EvAccessRefused, map[string]string{"file": r.URL.Query().Get("name"), "why": "virus scan: " + verdict})
		fail(w, 422, "the file was refused by the virus scan: "+verdict)
		return false
	}
	return true
}

// validClamAV is a clamd address: host:port, or the absolute path of its unix socket.
func validClamAV(addr string) bool {
	if addr == "" || filepath.IsAbs(addr) {
		return true
	}
	h, p, err := net.SplitHostPort(addr)
	n, perr := strconv.Atoi(p)
	return err == nil && h != "" && perr == nil && n > 0 && n < 65536
}

// eicar is the standard anti-virus test file, put together here so that this source file is not itself
// reported by a scanner.
func eicar() []byte {
	return []byte(`X5O!P%@AP[4\PZX54(P^)7CC)7}$` + "EICAR-STANDARD-ANTIVIRUS-TEST-FILE!" + `$H+H*`)
}

// clamVersion asks a clamd for its version.
func clamVersion(ctx context.Context, addr string) (string, error) {
	network := "tcp"
	if filepath.IsAbs(addr) {
		network = "unix"
	}
	d := net.Dialer{Timeout: 5 * time.Second}
	c, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return "", err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := c.Write([]byte("zVERSION\x00")); err != nil {
		return "", err
	}
	buf := make([]byte, 512)
	n, _ := c.Read(buf)
	return strings.TrimRight(string(buf[:n]), "\x00\n"), nil
}

func (s *Server) attachmentRoutes() {
	// Test a ClamAV daemon: its version, a clean file passes and the standard test file is caught.
	s.handle("POST /api/admin/connections/clamav/test", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Address string `json:"address"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		addr := strings.TrimSpace(b.Address)
		if addr == "" {
			addr = s.extra().ClamAV
		}
		if addr == "" || !validClamAV(addr) {
			fail(w, 422, "give clamd's address: host:port, or the path of its socket")
			return
		}
		out := map[string]any{"address": addr}
		v, err := clamVersion(r.Context(), addr)
		if err != nil {
			out["error"] = "could not reach clamd: " + err.Error()
			writeJSON(w, 200, out)
			return
		}
		out["version"] = v
		clean, err := clamScan(r.Context(), addr, []byte("bitdispatcher test: a clean file\n"))
		out["clean"] = err == nil && clean == ""
		found, err := clamScan(r.Context(), addr, eicar())
		out["testFile"] = found
		out["ok"] = err == nil && clean == "" && found != ""
		if err != nil {
			out["error"] = err.Error()
		}
		writeJSON(w, 200, out)
	})
	s.handle("GET /api/admin/connections/ocr", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		ok, v := textract.OCRAvailable()
		writeJSON(w, 200, map[string]any{"installed": ok, "version": v})
	})
}
