package api

// Alarm companies and permits, and the non-emergency intake.
//
// Alarm companies: each company the agency works with gets its own token and its own answer address, so
// its alarms arrive named and its answers (accepted, dispatched, on scene, closed) go back to it alone.
// The agency's alarm permits (imported from a spreadsheet) are checked as each alarm arrives, with the
// premise's false alarms in the last year. A call-taker taking an alarm company's phone call fills the same
// card, so a phoned alarm and a sent one look the same. ASAP over Nlets, the national standard, still needs
// the standard's schema and the state's message switch; this is the same exchange over HTTPS.
//
// The non-emergency intake (cad/intake.go): a voice or text bot asks the agency's questions through
// these endpoints; an emergency is handed to a person at once; a finished report is a call in the
// non-emergency list.

import (
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// AlarmCompany is a central station the agency takes alarms from.
type AlarmCompany struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Phone       string `json:"phone,omitempty"` // its operators' line
	Token       string `json:"token,omitempty"` // what it presents (Bearer); shown once when made
	AnswerURL   string `json:"answerUrl,omitempty"`
	AnswerToken string `json:"answerToken,omitempty"`
	Off         bool   `json:"off,omitempty"`
}

// AlarmPermit is one alarm permit on the agency's register.
type AlarmPermit struct {
	Number  string `json:"number"`
	Address string `json:"address"`
	Holder  string `json:"holder,omitempty"`
	Company string `json:"company,omitempty"`
	Expires string `json:"expires,omitempty"` // YYYY-MM-DD; empty: does not expire
	Status  string `json:"status,omitempty"`  // active, suspended, revoked (empty: active)
}

func (s *Server) permitsPath() string { return filepath.Join(s.DataDir, "alarm-permits.json") }

func (s *Server) loadPermits() []AlarmPermit {
	var p []AlarmPermit
	if b, err := os.ReadFile(s.permitsPath()); err == nil {
		json.Unmarshal(b, &p)
	}
	return p
}

var addrSpace = regexp.MustCompile(`[^a-z0-9]+`)

// normAddr is an address for matching: lower case, the common street words shortened, no punctuation.
func normAddr(a string) string {
	a = strings.ToLower(a)
	if i := strings.Index(a, ","); i > 0 {
		a = a[:i] // the street address only
	}
	f := strings.Fields(addrSpace.ReplaceAllString(a, " "))
	short := map[string]string{"street": "st", "avenue": "ave", "road": "rd", "drive": "dr", "boulevard": "blvd", "lane": "ln", "court": "ct",
		"place": "pl", "highway": "hwy", "parkway": "pkwy", "circle": "cir", "north": "n", "south": "s", "east": "e", "west": "w", "terrace": "ter"}
	for i, w := range f {
		if x, ok := short[w]; ok {
			f[i] = x
		}
	}
	return strings.Join(f, " ")
}

// alarmChecks are the notes an alarm gets: its permit, and the premise's false alarms in the last year.
func (s *Server) alarmChecks(al *cad.Alert, address string) []string {
	var notes []string
	permits := s.loadPermits()
	now := s.Eng.Now()
	var hit *AlarmPermit
	na := normAddr(address)
	for i := range permits {
		p := &permits[i]
		if al.Permit != "" && strings.EqualFold(strings.TrimSpace(p.Number), strings.TrimSpace(al.Permit)) || na != "" && normAddr(p.Address) == na {
			hit = p
			break
		}
	}
	if len(permits) > 0 {
		switch {
		case hit == nil:
			notes = append(notes, "Alarm permit: none on the register for this address")
		case hit.Status != "" && !strings.EqualFold(hit.Status, "active"):
			notes = append(notes, "Alarm permit "+hit.Number+": "+strings.ToUpper(hit.Status))
		case hit.Expires != "" && hit.Expires < now.Format("2006-01-02"):
			notes = append(notes, "Alarm permit "+hit.Number+": EXPIRED "+hit.Expires)
		default:
			notes = append(notes, "Alarm permit "+hit.Number+": valid"+map[bool]string{true: " until " + hit.Expires, false: ""}[hit.Expires != ""])
		}
	}
	if na == "" {
		return notes
	}
	var pol cad.AlarmPolicy
	count := 0
	s.Eng.Read(func(st *cad.State) {
		pol = st.Settings.Next.Alarms
		words := pol.FalseWords
		if len(words) == 0 {
			words = []string{"false"}
		}
		for _, in := range st.Incidents {
			if in.Location == nil || now.Sub(in.Created) > 365*24*time.Hour || normAddr(in.Location.Address) != na {
				continue
			}
			d := strings.ToLower(in.Disposition)
			for _, w := range words {
				if w = strings.ToLower(strings.TrimSpace(w)); w != "" && strings.Contains(d, w) {
					count++
					break
				}
			}
		}
	})
	limit := pol.FalseLimit
	if limit <= 0 {
		limit = 3
	}
	switch {
	case count >= limit:
		notes = append(notes, fmt.Sprintf("REPEAT FALSE ALARMS: %d at this address in the last year (the agency's limit is %d)", count, limit))
	case count > 0:
		notes = append(notes, fmt.Sprintf("False alarms at this address in the last year: %d", count))
	}
	return notes
}

func newToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) alarmCompany(id string) (AlarmCompany, bool) {
	for _, c := range s.nextConfig().AlarmCompanies {
		if c.ID == id {
			return c, true
		}
	}
	return AlarmCompany{}, false
}

func (s *Server) alarmRoutes() {
	// A company's own address: its token, its name on every alarm.
	s.mux.HandleFunc("POST /api/hooks/alarm/{company}", func(w http.ResponseWriter, r *http.Request) {
		c, ok := s.alarmCompany(r.PathValue("company"))
		if !ok || c.Off || c.Token == "" {
			fail(w, http.StatusNotFound, "no alarm company "+r.PathValue("company")+" in service")
			return
		}
		if !bearerEqual(r, c.Token) {
			fail(w, http.StatusUnauthorized, "wrong or missing bearer token")
			return
		}
		s.takeAlert(w, r, cad.AlertAlarm, &c, nil)
	})
	s.handle("GET /api/admin/alarms", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out := []map[string]any{}
		for _, c := range s.nextConfig().AlarmCompanies {
			out = append(out, map[string]any{"id": c.ID, "name": c.Name, "phone": c.Phone, "answerUrl": c.AnswerURL, "answerTokenSet": c.AnswerToken != "", "off": c.Off,
				"address": "/api/hooks/alarm/" + c.ID})
		}
		p := s.loadPermits()
		writeJSON(w, 200, map[string]any{"companies": out, "permits": len(p)})
	})
	s.handle("PUT /api/admin/alarms", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b AlarmCompany
		if !readJSON(w, r, &b) {
			return
		}
		b.Name, b.AnswerURL = strings.TrimSpace(b.Name), strings.TrimSpace(b.AnswerURL)
		if b.Name == "" {
			fail(w, 422, "give the company's name")
			return
		}
		if b.AnswerURL != "" && !validOutURL(b.AnswerURL) {
			fail(w, 422, "the answer address starts with https://")
			return
		}
		token := ""
		s.mu.Lock()
		n := s.integ
		list := slices.Clone(n.Next.AlarmCompanies)
		i := slices.IndexFunc(list, func(c AlarmCompany) bool { return c.ID == b.ID && b.ID != "" })
		if i < 0 {
			b.ID = strings.Trim(addrSpace.ReplaceAllString(strings.ToLower(b.Name), "-"), "-")
			for k := 2; slices.ContainsFunc(list, func(c AlarmCompany) bool { return c.ID == b.ID }); k++ {
				b.ID = fmt.Sprintf("%s-%d", strings.TrimRight(b.ID, "-0123456789"), k)
			}
			token = newToken()
			b.Token = token
			keepSecret(&b.AnswerToken, b.AnswerToken)
			list = append(list, b)
		} else {
			old := list[i]
			keepSecret(&old.AnswerToken, b.AnswerToken)
			old.Name, old.Phone, old.AnswerURL, old.Off = b.Name, strings.TrimSpace(b.Phone), b.AnswerURL, b.Off
			if b.Token == "new" {
				token = newToken()
				old.Token = token
			}
			list[i] = old
		}
		n.Next.AlarmCompanies = list
		err := s.writeIntegrations(n)
		if err == nil {
			s.integ = n
		}
		s.mu.Unlock()
		if err != nil {
			writeErr(w, err)
			return
		}
		s.Eng.Record(a, cad.EvIntegration, map[string]any{"alarmCompany": map[string]any{"id": b.ID, "name": b.Name, "answerUrl": b.AnswerURL, "off": b.Off, "newToken": token != ""}})
		writeJSON(w, 200, map[string]any{"id": b.ID, "token": token, "address": "/api/hooks/alarm/" + b.ID})
	})
	s.handle("DELETE /api/admin/alarms/{id}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		id := r.PathValue("id")
		s.mu.Lock()
		n := s.integ
		n.Next.AlarmCompanies = slices.DeleteFunc(slices.Clone(n.Next.AlarmCompanies), func(c AlarmCompany) bool { return c.ID == id })
		err := s.writeIntegrations(n)
		if err == nil {
			s.integ = n
		}
		s.mu.Unlock()
		if err == nil {
			s.Eng.Record(a, cad.EvIntegration, map[string]any{"alarmCompanyRemoved": id})
		}
		done(w, err)
	})
	// The permit register, as CSV: number, address, holder, company, expires (YYYY-MM-DD), status.
	s.handle("PUT /api/admin/alarms/permits", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		rd := csv.NewReader(io.LimitReader(r.Body, 32<<20))
		rd.FieldsPerRecord = -1
		rows, err := rd.ReadAll()
		if err != nil {
			fail(w, 422, "not a CSV file: "+err.Error())
			return
		}
		var out []AlarmPermit
		col := map[string]int{"number": 0, "address": 1, "holder": 2, "company": 3, "expires": 4, "status": 5}
		for i, row := range rows {
			if i == 0 {
				head := map[string]int{}
				for k, h := range row {
					head[strings.ToLower(strings.TrimSpace(h))] = k
				}
				if _, ok := head["number"]; ok {
					for k := range col {
						if x, ok := head[k]; ok {
							col[k] = x
						} else {
							col[k] = -1
						}
					}
					continue
				}
			}
			get := func(k string) string {
				if c := col[k]; c >= 0 && c < len(row) {
					return strings.TrimSpace(row[c])
				}
				return ""
			}
			p := AlarmPermit{Number: get("number"), Address: get("address"), Holder: get("holder"), Company: get("company"), Expires: get("expires"), Status: get("status")}
			if p.Number == "" || p.Address == "" {
				continue
			}
			if p.Expires != "" {
				if _, err := time.Parse("2006-01-02", p.Expires); err != nil {
					fail(w, 422, fmt.Sprintf("row %d: the expiry is YYYY-MM-DD, not %q", i+1, p.Expires))
					return
				}
			}
			out = append(out, p)
		}
		if len(out) > 200000 {
			fail(w, 422, "up to 200,000 permits")
			return
		}
		data, _ := json.Marshal(out)
		tmp := s.permitsPath() + ".tmp"
		if err := os.WriteFile(tmp, data, 0o600); err != nil {
			writeErr(w, err)
			return
		}
		if err := os.Rename(tmp, s.permitsPath()); err != nil {
			writeErr(w, err)
			return
		}
		s.Eng.Record(a, cad.EvIntegration, map[string]any{"alarmPermits": len(out)})
		writeJSON(w, 200, map[string]any{"permits": len(out)})
	})
	// An alarm company's phone call, typed by the call-taker on the alarm card.
	s.handle("POST /api/alarms/phoned", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		s.takeAlert(w, r, cad.AlertAlarm, nil, &a)
	})
}

// ---- the non-emergency intake ----

type intakeRun struct {
	ss      cad.IntakeSession
	channel string
	from    string
	started time.Time
	seen    time.Time
	by      string
	call    string
}

const maxIntakes = 200

func (s *Server) intakeAuth(h func(w http.ResponseWriter, r *http.Request, by string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := s.nextConfig().IntakeToken
		if tok == "" {
			fail(w, http.StatusServiceUnavailable, "the intake is not connected: an administrator makes its token in Admin › Non-emergency intake")
			return
		}
		if !bearerEqual(r, tok) {
			fail(w, http.StatusUnauthorized, "wrong or missing bearer token")
			return
		}
		h(w, r, "intake bot")
	}
}

func (s *Server) intakeRoutes() {
	start := func(w http.ResponseWriter, r *http.Request, by string) {
		var b struct {
			Channel string `json:"channel"` // voice or text
			From    string `json:"from"`    // the caller's number
		}
		if !readJSON(w, r, &b) {
			return
		}
		var sc cad.IntakeScript
		var on bool
		s.Eng.Read(func(st *cad.State) { sc, on = st.Settings.IntakeOrDefault(), st.Settings.Next.Intake.On })
		if !on && by == "intake bot" {
			fail(w, http.StatusServiceUnavailable, "the intake is switched off (Admin › Non-emergency intake): hand the caller to a person")
			return
		}
		run := &intakeRun{channel: firstNonEmptyStr(b.Channel, "voice"), from: strings.TrimSpace(b.From), started: time.Now(), seen: time.Now(), by: by}
		t := sc.Start(&run.ss)
		id := newToken()[:20]
		s.next.mu.Lock()
		if s.next.intake == nil {
			s.next.intake = map[string]*intakeRun{}
		}
		if len(s.next.intake) >= maxIntakes {
			s.next.mu.Unlock()
			fail(w, http.StatusServiceUnavailable, "too many conversations at once: hand the caller to a person")
			return
		}
		s.next.intake[id] = run
		s.next.mu.Unlock()
		writeJSON(w, 200, map[string]any{"session": id, "turn": t})
	}
	say := func(w http.ResponseWriter, r *http.Request, by string) {
		var b struct {
			Text string `json:"text"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		id := r.PathValue("id")
		s.next.mu.Lock()
		run := s.next.intake[id]
		s.next.mu.Unlock()
		if run == nil || run.by != by {
			fail(w, 404, "no conversation "+id+" (it ends after 30 minutes without a word)")
			return
		}
		var sc cad.IntakeScript
		s.Eng.Read(func(st *cad.State) { sc = st.Settings.IntakeOrDefault() })
		s.next.mu.Lock()
		run.seen = time.Now()
		t := sc.Hear(&run.ss, b.Text)
		s.next.mu.Unlock()
		if t.Done {
			ref, err := s.finishIntake(r.Context(), id, run, sc)
			if err != nil {
				writeErr(w, err)
				return
			}
			if !t.Transfer {
				t.Say = strings.ReplaceAll(sc.Closing, "{ref}", ref)
			}
			s.next.mu.Lock()
			delete(s.next.intake, id)
			s.next.mu.Unlock()
			writeJSON(w, 200, map[string]any{"turn": t, "call": run.call, "ref": ref})
			return
		}
		writeJSON(w, 200, map[string]any{"turn": t})
	}
	s.mux.HandleFunc("POST /api/intake/sessions", s.intakeAuth(start))
	s.mux.HandleFunc("POST /api/intake/sessions/{id}", s.intakeAuth(say))
	// An administrator trying the script: the same conversation, from the admin page.
	s.handle("POST /api/admin/intake/try", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		start(w, r, "try:"+a.User)
	})
	s.handle("POST /api/admin/intake/try/{id}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		say(w, r, "try:"+a.User)
	})
	s.handle("GET /api/admin/intake", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var sc cad.IntakeScript
		var on bool
		var words []string
		s.Eng.Read(func(st *cad.State) {
			sc, on, words = st.Settings.IntakeOrDefault(), st.Settings.Next.Intake.On, st.Settings.Next.Intake.EmergencyWords
		})
		sc.On, sc.EmergencyWords = on, words // empty: the built-in list, shown as the field's placeholder
		writeJSON(w, 200, map[string]any{"script": sc, "tokenSet": s.nextConfig().IntakeToken != "", "defaults": cad.DefaultIntake(), "emergencyWords": cad.DefaultEmergencyWords})
	})
	s.handle("PUT /api/admin/intake/token", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Clear bool `json:"clear"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		tok := ""
		if !b.Clear {
			tok = newToken()
		}
		s.mu.Lock()
		n := s.integ
		n.Next.IntakeToken = tok
		err := s.writeIntegrations(n)
		if err == nil {
			s.integ = n
		}
		s.mu.Unlock()
		if err != nil {
			writeErr(w, err)
			return
		}
		s.Eng.Record(a, cad.EvIntegration, map[string]any{"intakeToken": tok != ""})
		writeJSON(w, 200, map[string]any{"token": tok})
	})
}

// finishIntake makes the conversation a call in the non-emergency list (a report) or, when it was handed
// to a person, a call marked urgent with what was said so far. The training try-out makes nothing.
func (s *Server) finishIntake(ctx context.Context, id string, run *intakeRun, sc cad.IntakeScript) (string, error) {
	ref := "NE-" + strings.ToUpper(id[:6])
	if strings.HasPrefix(run.by, "try:") {
		return ref + " (a try-out: nothing was made)", nil
	}
	notes := []string{}
	for _, q := range sc.Questions {
		if v := run.ss.Answers[q.Key]; v != "" {
			notes = append(notes, q.Ask+" "+v)
		}
	}
	if run.ss.Handoff {
		notes = append([]string{"HANDED TO A PERSON: " + run.ss.Reason}, notes...)
	}
	d := cad.CallData{Source: "intake", State: "data", Line: cad.IntakeLine, Class: "REPORT", Callback: firstNonEmptyStr(run.ss.Answers["phone"], run.from),
		CallerName: run.ss.Answers["name"], Notes: append([]string{"Non-emergency report " + ref + " (" + run.channel + ")"}, notes...),
		Transcript: run.ss.Lines, Emergency: run.ss.Handoff, Start: run.started}
	if where := run.ss.Answers["where"]; where != "" {
		d.Location = &ng911.Location{Address: where, Source: "intake", Method: "Said by the caller"}
		if lat, lon, label, ok := (mapGeocoder{s}).Geocode(where); ok {
			d.Location.Lat, d.Location.Lon, d.Location.Address = &lat, &lon, label
		}
	}
	call := "intake-" + id
	run.call = call
	if err := s.Eng.UpsertCall(cad.Actor{User: "intake"}, call, d); err != nil {
		return "", err
	}
	return ref, nil
}

// sweepIntake forgets conversations nobody has spoken in for half an hour.
func (s *Server) sweepIntake(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s.next.mu.Lock()
		for id, run := range s.next.intake {
			if time.Since(run.seen) > 30*time.Minute {
				delete(s.next.intake, id)
			}
		}
		s.next.mu.Unlock()
	}
}
