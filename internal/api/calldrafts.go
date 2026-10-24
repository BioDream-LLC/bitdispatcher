package api

// Working calls (cad/calldrafts.go): each draft's contents are kept here, beside the ledger, with its
// owner, so a reload, a lock or a lost connection loses nothing and the owner finds them at any console.
// The ledger records each draft's life; what is typed is not ledger history until it is submitted.

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

var reDraftID = regexp.MustCompile(`^W[A-Z0-9]*[0-9]+$`)

func (s *Server) draftDir() string {
	dir := s.DataDir
	if s.liveServer != nil {
		dir = s.liveServer.DataDir
	}
	return filepath.Join(dir, "call-drafts")
}

// DraftView is a working call with its contents.
type DraftView struct {
	*cad.CallDraft
	Content json.RawMessage `json:"content,omitempty"`
	Saved   time.Time       `json:"saved,omitzero"`
}

func (s *Server) draftContent(id string) (json.RawMessage, time.Time) {
	p := filepath.Join(s.draftDir(), id+".json")
	b, err := os.ReadFile(p)
	if err != nil || !json.Valid(b) {
		return nil, time.Time{}
	}
	fi, _ := os.Stat(p)
	return b, fi.ModTime()
}

func (s *Server) callDraftRoutes() {
	s.handle("GET /api/drafts", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var mine []*cad.CallDraft
		var all []*cad.CallDraft
		var maxN, abandon int
		s.Eng.Read(func(st *cad.State) {
			mine, maxN = st.DraftsOf(se.Username), st.MaxDrafts(se.Username, se.Duty)
			abandon = st.Settings.MultiCall.AbandonMinutes
			if auth.AtLeast(se.Role, auth.Supervisor) {
				for _, d := range st.OpenDrafts() {
					if !strings.EqualFold(d.Owner, se.Username) && (d.Agency == "" || st.MayWork(se.Username, d.Agency)) {
						all = append(all, d)
					}
				}
			}
		})
		if abandon == 0 {
			abandon = 30
		}
		out := []DraftView{}
		for _, d := range mine {
			c, at := s.draftContent(d.ID)
			out = append(out, DraftView{CallDraft: d, Content: c, Saved: at})
		}
		// Supervisors see who holds which working call, and which were left behind (no contents shown).
		signedIn := map[string]bool{}
		for _, x := range s.Auth.Active() {
			signedIn[x.Username] = true
		}
		type other struct {
			*cad.CallDraft
			Saved     time.Time `json:"saved,omitzero"`
			Abandoned bool      `json:"abandoned"`
		}
		others := []other{}
		now := s.Eng.Now()
		for _, d := range all {
			_, at := s.draftContent(d.ID)
			last := at
			if last.IsZero() {
				last = d.Opened
			}
			others = append(others, other{CallDraft: d, Saved: at, Abandoned: !signedIn[d.Owner] && now.Sub(last) > time.Duration(abandon)*time.Minute})
		}
		writeJSON(w, 200, map[string]any{"items": out, "max": maxN, "others": others})
	})
	s.handle("POST /api/drafts", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Call    string          `json:"call"`
			Content json.RawMessage `json:"content"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		id, err := s.Eng.OpenDraft(a, b.Call)
		if err != nil {
			writeErr(w, err)
			return
		}
		if len(b.Content) > 0 && json.Valid(b.Content) {
			os.MkdirAll(s.draftDir(), 0o750)
			os.WriteFile(filepath.Join(s.draftDir(), id+".json"), b.Content, 0o640)
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	owner := func(w http.ResponseWriter, se *auth.Session, id string, supervisorMay bool) (*cad.CallDraft, bool) {
		if !reDraftID.MatchString(id) {
			fail(w, 404, "no such working call")
			return nil, false
		}
		var d *cad.CallDraft
		var agencyOK bool
		s.Eng.Read(func(st *cad.State) {
			if x := st.CallDrafts[id]; x != nil && x.State == "open" {
				c := *x
				d = &c
				agencyOK = c.Agency == "" || st.MayWork(se.Username, c.Agency)
			}
		})
		if d == nil {
			fail(w, 404, "no open working call "+id)
			return nil, false
		}
		if !strings.EqualFold(d.Owner, se.Username) && !(supervisorMay && auth.AtLeast(se.Role, auth.Supervisor) && agencyOK) {
			fail(w, 403, "that working call is "+d.Owner+"'s")
			return nil, false
		}
		return d, true
	}
	s.handle("GET /api/drafts/{id}", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		d, ok := owner(w, se, r.PathValue("id"), true)
		if !ok {
			return
		}
		c, at := s.draftContent(d.ID)
		if !strings.EqualFold(d.Owner, se.Username) {
			s.Eng.Record(a, cad.EvRecordsViewed, map[string]string{"what": "working call", "id": d.ID, "owner": d.Owner})
		}
		writeJSON(w, 200, DraftView{CallDraft: d, Content: c, Saved: at})
	})
	s.handle("PUT /api/drafts/{id}", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		d, ok := owner(w, se, r.PathValue("id"), false)
		if !ok {
			return
		}
		b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256<<10))
		if err != nil || !json.Valid(b) {
			fail(w, 400, "a working call is a JSON document up to 256 KB")
			return
		}
		if err := os.MkdirAll(s.draftDir(), 0o750); err != nil {
			writeErr(w, err)
			return
		}
		tmp := filepath.Join(s.draftDir(), d.ID+".tmp")
		if err := os.WriteFile(tmp, b, 0o640); err != nil {
			writeErr(w, err)
			return
		}
		if err := os.Rename(tmp, filepath.Join(s.draftDir(), d.ID+".json")); err != nil {
			writeErr(w, err)
			return
		}
		if d.Console != se.Console {
			s.Eng.RecoveredDraft(a, d.ID)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("POST /api/drafts/{id}/close", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		d, ok := owner(w, se, r.PathValue("id"), true)
		if !ok {
			return
		}
		var b struct {
			How       string `json:"how"`
			Incident  string `json:"incident"`
			Why       string `json:"why"`
			Populated bool   `json:"populated"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		if err := s.Eng.CloseDraft(a, d.ID, b.How, b.Incident, b.Why, b.Populated); err != nil {
			writeErr(w, err)
			return
		}
		os.Remove(filepath.Join(s.draftDir(), d.ID+".json"))
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("POST /api/drafts/{id}/move", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		d, ok := owner(w, se, r.PathValue("id"), true)
		if !ok {
			return
		}
		var b struct{ To, Why string }
		if !readJSON(w, r, &b) {
			return
		}
		known := false
		for _, u := range s.Auth.List() {
			if strings.EqualFold(u.Username, b.To) && !u.Disabled {
				known = true
			}
		}
		if !known {
			fail(w, 422, "no such person to take it")
			return
		}
		done(w, s.Eng.MoveDraft(a, d.ID, b.To, b.Why))
	})
}
