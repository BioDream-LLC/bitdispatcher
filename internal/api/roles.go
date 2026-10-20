package api

// Sign-in with a duty and a one-time code, console locking, and enrolled workstations.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// tokenHash is what the ledger keeps of a workstation token.
func tokenHash(t string) string {
	if t == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("bitdispatcher-workstation:" + t))
	return hex.EncodeToString(sum[:16])
}

// enrolCodes are one-time codes an administrator made to enrol another browser as a workstation.
type enrolCodes struct {
	mu    sync.Mutex
	codes map[string]enrolCode
}

type enrolCode struct {
	ws      string
	by      string
	expires time.Time
}

// signIn is the sign-in page's form: a password, then (when needed) a one-time code and the duty. The
// position comes from the workstation the browser is enrolled as, never from the person.
func (s *Server) signIn(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(csrfHeader) == "" {
		fail(w, http.StatusForbidden, "missing "+csrfHeader+" header")
		return
	}
	var b struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		Code        string `json:"code"`
		Duty        string `json:"duty"`
		Workstation string `json:"workstation"` // the browser's workstation token, when enrolled
		Console     string `json:"console"`     // a browser's own name when not enrolled
		Unit        string `json:"unit"`        // a field crew's unit (a mobile terminal signs in as it)
		Training    bool   `json:"training"`
		Exercise    string `json:"exercise"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	console, ws := strings.ToUpper(strings.TrimSpace(b.Console)), ""
	var wsDuties []string
	s.Eng.Read(func(st *cad.State) {
		if x := st.WorkstationByHash(tokenHash(b.Workstation)); x != nil {
			ws, wsDuties = x.ID, x.Duties
		}
	})
	if ws != "" {
		console = ws
	}
	se, need, err := s.Auth.SignIn(auth.Request{Username: b.Username, Password: b.Password, Code: b.Code, Duty: b.Duty, Console: console, From: clientIP(r)})
	if err != nil {
		s.Eng.Record(cad.Actor{User: strings.ToLower(b.Username)}, cad.EvSignInFailed, map[string]string{"from": clientIP(r), "reason": err.Error(), "workstation": ws})
		if need != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"message": err.Error(), "need": need})
			return
		}
		fail(w, http.StatusUnauthorized, err.Error())
		return
	}
	if need != nil {
		// The workstation's own duties are offered first.
		if len(need.Duties) > 0 && len(wsDuties) > 0 {
			need.Duties = append(intersect(wsDuties, need.Duties), minus(need.Duties, wsDuties)...)
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"need": need, "workstation": ws})
		return
	}
	// A field crew signs in as its unit.
	if se.Duty == auth.DutyField {
		unit := firstNonEmpty(strings.ToUpper(strings.TrimSpace(b.Unit)), console)
		known := false
		s.Eng.Read(func(st *cad.State) { _, known = st.Units[unit] })
		if !known {
			s.Auth.Logout(se.ID)
			writeJSON(w, http.StatusAccepted, map[string]any{"need": map[string]any{"unit": true}, "message": "type your unit's call sign"})
			return
		}
		s.Auth.SetConsole(se.ID, unit)
		se.Console = unit
	} else if console == "" || len(console) > 24 {
		console = "WEB-" + strings.ToUpper(auth.RandomToken(3))[:4]
		s.Auth.SetConsole(se.ID, console)
		se.Console = console
	}
	if ws != "" {
		s.Auth.SetWorkstation(se.ID, ws)
		se.Workstation = ws
	}
	if b.Training {
		s.Auth.SetTraining(se.ID, b.Exercise)
		se.Training, se.Exercise = true, b.Exercise
	}
	s.signedIn(w, se, map[string]string{"duty": se.Duty, "workstation": ws})
}

func intersect(a, b []string) []string {
	out := []string{}
	for _, x := range a {
		for _, y := range b {
			if x == y {
				out = append(out, x)
			}
		}
	}
	return out
}

func minus(a, b []string) []string {
	out := []string{}
	for _, x := range a {
		keep := true
		for _, y := range b {
			if x == y {
				keep = false
			}
		}
		if keep {
			out = append(out, x)
		}
	}
	return out
}

func (s *Server) sessionOf(r *http.Request) (*auth.Session, string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, "", false
	}
	se, ok := s.Auth.Session(c.Value)
	return se, c.Value, ok
}

func actorOf(se *auth.Session) cad.Actor {
	return cad.Actor{User: se.Username, Role: se.Role, Session: se.Hash, Console: se.Console, Duty: se.Duty}
}

// lockRoutes: lock now, unlock with the password (and code), and console input for the inactivity lock.
func (s *Server) lockRoutes() {
	s.mux.HandleFunc("POST /api/auth/lock", func(w http.ResponseWriter, r *http.Request) {
		_, id, ok := s.sessionOf(r)
		if !ok || r.Header.Get(csrfHeader) == "" {
			fail(w, http.StatusUnauthorized, "sign in")
			return
		}
		if se, ok := s.Auth.Lock(id); ok {
			s.Eng.Record(actorOf(se), cad.EvLocked, map[string]string{"how": "on demand"})
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.mux.HandleFunc("POST /api/auth/unlock", func(w http.ResponseWriter, r *http.Request) {
		_, id, ok := s.sessionOf(r)
		if !ok || r.Header.Get(csrfHeader) == "" {
			fail(w, http.StatusUnauthorized, "your session ended: sign in again")
			return
		}
		var b struct {
			Password string `json:"password"`
			Code     string `json:"code"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		se, err := s.Auth.Unlock(id, b.Password, b.Code)
		if err != nil {
			if cur, ok := s.Auth.Session(id); ok {
				s.Eng.Record(actorOf(cur), cad.EvSignInFailed, map[string]string{"from": clientIP(r), "reason": "unlock: " + err.Error()})
			}
			fail(w, http.StatusUnauthorized, err.Error())
			return
		}
		s.Eng.Record(actorOf(se), cad.EvUnlocked, nil)
		writeJSON(w, 200, se)
	})
	s.mux.HandleFunc("POST /api/auth/active", func(w http.ResponseWriter, r *http.Request) {
		if _, id, ok := s.sessionOf(r); ok && r.Header.Get(csrfHeader) != "" {
			s.Auth.Touch(id)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// Enrolling a browser with a code an administrator made elsewhere: before signing in.
	s.mux.HandleFunc("POST /api/workstations/enrol", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(csrfHeader) == "" {
			fail(w, http.StatusForbidden, "missing "+csrfHeader+" header")
			return
		}
		var b struct {
			Code  string `json:"code"`
			Label string `json:"label"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		code := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(b.Code), "-", ""))
		s.enrol.mu.Lock()
		c, ok := s.enrol.codes[code]
		if ok {
			delete(s.enrol.codes, code)
		}
		s.enrol.mu.Unlock()
		if !ok || time.Now().After(c.expires) {
			time.Sleep(500 * time.Millisecond) // a guess costs time
			fail(w, http.StatusUnauthorized, "that enrolment code is not right or has expired: ask an administrator for a new one")
			return
		}
		tok := auth.RandomToken(32)
		if err := s.Eng.EnrolWorkstation(cad.Actor{User: c.by, Console: c.ws}, c.ws, tokenHash(tok), b.Label); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"token": tok, "workstation": c.ws})
	})
}

// workstationRoutes are the administrator's.
func (s *Server) workstationRoutes() {
	s.handle("GET /api/admin/workstations", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out := []*cad.Workstation{}
		s.Eng.Read(func(st *cad.State) {
			for _, x := range st.Workstations {
				cp := *x
				out = append(out, &cp)
			}
		})
		sortBy(out, func(x *cad.Workstation) string { return x.ID })
		writeJSON(w, 200, out)
	})
	s.handle("PUT /api/admin/workstations/{id}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b cad.Workstation
		if !readJSON(w, r, &b) {
			return
		}
		b.ID = r.PathValue("id")
		if err := s.Eng.SaveWorkstation(a, b); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("DELETE /api/admin/workstations/{id}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if err := s.Eng.RemoveWorkstation(a, strings.ToUpper(r.PathValue("id"))); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// Enrol this very browser: the token goes back to it, to keep.
	s.handle("POST /api/admin/workstations/{id}/enrol", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Label string `json:"label"`
		}
		readJSON(w, r, &b)
		tok := auth.RandomToken(32)
		id := strings.ToUpper(r.PathValue("id"))
		if err := s.Eng.EnrolWorkstation(a, id, tokenHash(tok), b.Label); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"token": tok, "workstation": id})
	})
	// A one-time code to enrol another browser, valid for a day.
	s.handle("POST /api/admin/workstations/{id}/code", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		id := strings.ToUpper(r.PathValue("id"))
		known := false
		s.Eng.Read(func(st *cad.State) { _, known = st.Workstations[id] })
		if !known {
			fail(w, 404, "no workstation "+id)
			return
		}
		const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
		raw := auth.RandomToken(12)
		var code strings.Builder
		for i := 0; i < 10; i++ {
			code.WriteByte(alphabet[int(raw[i])%len(alphabet)])
		}
		c := code.String()
		s.enrol.mu.Lock()
		if s.enrol.codes == nil {
			s.enrol.codes = map[string]enrolCode{}
		}
		s.enrol.codes[c] = enrolCode{ws: id, by: se.Username, expires: time.Now().Add(24 * time.Hour)}
		s.enrol.mu.Unlock()
		s.Eng.Record(a, cad.EvWorkstationCode, map[string]string{"id": id})
		writeJSON(w, 200, map[string]string{"code": c[:5] + "-" + c[5:], "expires": time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)})
	})
	s.handle("DELETE /api/admin/workstations/{id}/enrolled/{hash}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if err := s.Eng.RevokeEnrolment(a, strings.ToUpper(r.PathValue("id")), r.PathValue("hash")); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// Duties and one-time codes for a person.
	s.handle("PUT /api/admin/users/{name}/duties", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Duties []string `json:"duties"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		name := strings.ToLower(r.PathValue("name"))
		if name == se.Username {
			fail(w, 422, "you cannot change your own duties: ask another administrator")
			return
		}
		if err := s.Auth.SetDuties(name, b.Duties); err != nil {
			fail(w, 422, err.Error())
			return
		}
		s.Eng.Record(a, cad.EvUserSaved, cad.UserInfo{Username: name, Role: s.Auth.UserRole(name)})
		s.Eng.Record(a, cad.EvUserDuties, map[string]any{"username": name, "duties": b.Duties})
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("POST /api/admin/users/{name}/reset-mfa", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		name := strings.ToLower(r.PathValue("name"))
		if err := s.Auth.ResetMFA(name); err != nil {
			fail(w, 422, err.Error())
			return
		}
		s.Eng.Record(a, cad.EvUserMFAReset, map[string]string{"username": name})
		w.WriteHeader(http.StatusNoContent)
	})
}

// policyWatch keeps the sign-in policy in step with the agency's settings.
func (s *Server) policyWatch(ctx context.Context) {
	apply := func() {
		var sec cad.Security
		var agency string
		s.Eng.Read(func(st *cad.State) { sec, agency = st.Settings.Security, st.Settings.Agency })
		s.Auth.SetPolicy(sec.MFA, time.Duration(sec.LockMinutes)*time.Minute, firstNonEmpty(agency, "bitdispatcher"))
	}
	apply()
	ch := s.Eng.Subscribe()
	defer s.Eng.Unsubscribe(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-ch:
			if n.Type == cad.EvSettings {
				apply()
			}
		}
	}
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}

func sortBy[T any](xs []T, key func(T) string) {
	sort.Slice(xs, func(i, j int) bool { return key(xs[i]) < key(xs[j]) })
}
