package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/table"
)

// Moving from an earlier CAD system: its premises and hazards, its run cards and its incident history,
// from the spreadsheets such systems export. Each file is uploaded, its columns matched to ours (guessed,
// shown against sample values), checked row by row, and then imported; the ledger records every import.
// Address points go to County GIS, which reads CSV as well as GeoJSON and ArcGIS.

func (s *Server) migrateRoutes() {
	s.handle("POST /api/admin/migrate/upload", auth.Admin, s.migrateUpload)
	s.handle("POST /api/admin/migrate/import", auth.Admin, s.migrateImport)
	s.handle("GET /api/premises", auth.Viewer, s.premiseList)
	s.handle("PUT /api/premises", auth.Supervisor, s.premiseSave)
	s.handle("DELETE /api/premises/{id}", auth.Supervisor, s.premiseRemove)
}

// MigrateKinds are what can be brought over, with our fields (the first ones needed) and likely column names.
var MigrateKinds = map[string]struct {
	Fields []string            `json:"fields"`
	Need   []string            `json:"need"`
	Likely map[string][]string `json:"-"`
}{
	"premises": {
		Fields: []string{"address", "lat", "lon", "name", "hazards", "notes", "contacts", "expires", "id"},
		Need:   []string{"address"},
		Likely: map[string][]string{
			"id": {"PREMISE_ID", "PREMISEID", "LOCATION_ID", "SITE_ID", "CAUTION_ID", "ID"}, "name": {"PREMISE_NAME", "NAME", "COMMONNAME", "COMMON_NAME", "BUSINESS", "BUSINESS_NAME", "PLACE_NAME"},
			"address": {"ADDRESS", "FULLADDRESS", "FULL_ADDRESS", "FULLADDR", "STREET_ADDRESS", "LOCATION", "ADDR", "SITE_ADDRESS"},
			"lat":     latNames, "lon": lonNames,
			"hazards":  {"HAZARDS", "HAZARD", "CAUTIONS", "CAUTION", "ALERTS", "ALERT", "WARNINGS", "WARNING", "FLAGS", "OFFICER_SAFETY", "SPECIAL_HAZARDS"},
			"notes":    {"NOTES", "NOTE", "PREMISE_INFO", "COMMENTS", "COMMENT", "REMARKS", "DESCRIPTION", "INFO"},
			"contacts": {"CONTACTS", "CONTACT", "KEYHOLDER", "KEY_HOLDER", "KEYHOLDERS", "EMERGENCY_CONTACT", "OWNER", "PHONE"},
			"expires":  {"EXPIRES", "EXPIRATION", "EXPIRATION_DATE", "EXPIRE_DATE", "END_DATE", "PURGE_DATE"},
		},
	},
	"history": {
		Fields: []string{"number", "at", "address", "lat", "lon", "type", "disposition", "note"},
		Need:   []string{"number", "at"},
		Likely: map[string][]string{
			"number":  {"INCIDENT_NUMBER", "INCIDENTNUMBER", "INCIDENT_NO", "INC_NUM", "INCNUM", "CFS_NUMBER", "CFS", "EVENT_NUMBER", "CALL_NUMBER", "MASTER_INCIDENT_NUMBER", "INCIDENT", "NUMBER"},
			"at":      {"CREATED", "DATETIME", "DATE_TIME", "RECEIVED", "CALL_RECEIVED", "TIME_RECEIVED", "RESPONSE_DATE", "CREATE_TIME", "CALLDATE", "CALL_DATE", "START_TIME", "ENTRY_DATE", "DATE"},
			"address": {"ADDRESS", "FULLADDRESS", "FULL_ADDRESS", "LOCATION", "INCIDENT_ADDRESS", "ADDR"}, "lat": latNames, "lon": lonNames,
			"type":        {"CALL_TYPE", "CALLTYPE", "INCIDENT_TYPE", "NATURE_CODE", "NATURE", "PROBLEM", "TYPE"},
			"disposition": {"DISPOSITION", "DISPO", "CLOSE_CODE", "CLEARANCE", "OUTCOME", "FINAL_DISPOSITION"},
			"note":        {"NARRATIVE", "NOTES", "COMMENTS", "REMARKS", "NOTE"},
		},
	},
	"runcards": {
		Fields: []string{"code", "name", "priority", "discipline", "runcard"},
		Need:   []string{"code", "runcard"},
		Likely: map[string][]string{
			"code": {"CALL_TYPE", "CALLTYPE", "NATURE_CODE", "INCIDENT_TYPE", "TYPE_CODE", "CODE", "TYPE"}, "name": {"DESCRIPTION", "NAME", "DESC", "NATURE", "TITLE"},
			"priority":   {"PRIORITY", "PRI", "PRIORITY_CODE", "DEFAULT_PRIORITY"},
			"discipline": {"DISCIPLINE", "AGENCY_TYPE", "SERVICE", "AGENCY", "DEPARTMENT"},
			"runcard":    {"RUN_CARD", "RUNCARD", "RESPONSE_PLAN", "RESPONSE", "RECOMMENDATION", "ASSIGNMENT", "UNITS"},
		},
	},
}

var (
	latNames = []string{"LATITUDE", "LAT", "Y", "POINT_Y", "YCOORD", "Y_COORD"}
	lonNames = []string{"LONGITUDE", "LON", "LONG", "LNG", "X", "POINT_X", "XCOORD", "X_COORD"}
	reUpload = regexp.MustCompile(`^[A-Za-z0-9._ -]{1,80}\.(csv|txt|tsv)$`)
)

func (s *Server) migrateDir() string { return filepath.Join(s.DataDir, "migrate") }

// MigrateFile is an uploaded file as the screen sees it.
type MigrateFile struct {
	Name    string                       `json:"name"`
	Headers []string                     `json:"headers"`
	Sample  []map[string]string          `json:"sample"`
	Rows    int                          `json:"rows"`
	Kind    string                       `json:"kind"` // the best guess
	Guess   map[string]map[string]string `json:"guess"`
	Kinds   map[string][]string          `json:"kinds"` // each kind's fields, in order
	Need    map[string][]string          `json:"need"`
}

func (s *Server) migrateUpload(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	name := filepath.Base(r.URL.Query().Get("name"))
	if !reUpload.MatchString(name) {
		fail(w, http.StatusBadRequest, "a CSV or tab-separated file (.csv, .tsv, .txt)")
		return
	}
	os.MkdirAll(s.migrateDir(), 0o700)
	p := filepath.Join(s.migrateDir(), name)
	f, err := os.Create(p + ".part")
	if err != nil {
		writeErr(w, err)
		return
	}
	_, err = io.Copy(f, http.MaxBytesReader(w, r.Body, 512<<20))
	f.Close()
	if err == nil {
		err = os.Rename(p+".part", p)
	}
	if err != nil {
		os.Remove(p + ".part")
		fail(w, http.StatusBadRequest, "the upload did not finish: "+err.Error())
		return
	}
	h, sample, n, err := table.Sample(p, 5)
	if err != nil {
		os.Remove(p)
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	mf := MigrateFile{Name: name, Headers: h, Sample: sample, Rows: n, Guess: map[string]map[string]string{}, Kinds: map[string][]string{}, Need: map[string][]string{}}
	best := -1
	for _, k := range []string{"premises", "history", "runcards"} {
		def := MigrateKinds[k]
		g := table.Guess(h, def.Likely, def.Fields)
		mf.Guess[k], mf.Kinds[k], mf.Need[k] = g, def.Fields, def.Need
		score := len(g)
		for _, f := range def.Need {
			if g[f] == "" {
				score -= 10
			}
		}
		if score > best {
			best, mf.Kind = score, k
		}
	}
	writeJSON(w, 200, mf)
}

// RowProblem is a row that cannot be imported.
type RowProblem struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

// MigrateResult is what a check or an import found.
type MigrateResult struct {
	Rows     int          `json:"rows"`
	Good     int          `json:"good"`
	Imported int          `json:"imported"`
	Located  int          `json:"located"` // positions found by searching the address
	Problems []RowProblem `json:"problems"`
	Notes    []string     `json:"notes,omitempty"`
	More     int          `json:"more,omitempty"` // problems not listed
}

func (m *MigrateResult) bad(row int, f string, a ...any) {
	if len(m.Problems) < 100 {
		m.Problems = append(m.Problems, RowProblem{row, fmt.Sprintf(f, a...)})
	} else {
		m.More++
	}
}

func (s *Server) migrateImport(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var b struct {
		File   string            `json:"file"`
		Kind   string            `json:"kind"`
		Fields map[string]string `json:"fields"`
		Check  bool              `json:"check"` // only check, import nothing
	}
	if !readJSON(w, r, &b) {
		return
	}
	def, ok := MigrateKinds[b.Kind]
	if !ok {
		fail(w, http.StatusBadRequest, "the kind is premises, history or runcards")
		return
	}
	for _, f := range def.Need {
		if b.Fields[f] == "" {
			fail(w, 422, "match a column to "+f)
			return
		}
	}
	name := filepath.Base(b.File)
	if !reUpload.MatchString(name) {
		fail(w, http.StatusBadRequest, "no such file")
		return
	}
	t, err := table.Open(filepath.Join(s.migrateDir(), name))
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	defer t.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	res := MigrateResult{Problems: []RowProblem{}}
	get := func(m map[string]string, f string) string {
		if c := b.Fields[f]; c != "" {
			return strings.TrimSpace(m[c])
		}
		return ""
	}
	source := "import " + name
	var premises []cad.Premise
	var history []cad.History
	var types []cad.CallType
	located := map[string][2]float64{}
	locate := func(row int, m map[string]string) (float64, float64, bool) {
		if la, lo := get(m, "lat"), get(m, "lon"); la != "" && lo != "" {
			lat, e1 := strconv.ParseFloat(la, 64)
			lon, e2 := strconv.ParseFloat(lo, 64)
			if e1 != nil || e2 != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
				res.bad(row, "the latitude and longitude (%s, %s) are not degrees", la, lo)
				return 0, 0, false
			}
			if lat != 0 || lon != 0 {
				return lat, lon, true
			}
		}
		addr := get(m, "address")
		if addr == "" {
			res.bad(row, "no address and no position")
			return 0, 0, false
		}
		if p, ok := located[strings.ToLower(addr)]; ok {
			return p[0], p[1], p[0] != 0
		}
		lat, lon, ok := s.geocode(ctx, addr)
		located[strings.ToLower(addr)] = [2]float64{lat, lon}
		if !ok {
			res.bad(row, "%s was not found on the map: give its position, or correct it", addr)
			return 0, 0, false
		}
		res.Located++
		return lat, lon, true
	}
	var caps map[string]bool
	s.Eng.Read(func(st *cad.State) {
		caps = map[string]bool{}
		for _, ut := range st.Settings.UnitTypes {
			for _, c := range ut.Capabilities {
				caps[c] = true
			}
		}
	})
	unknownCaps := map[string]bool{}
	for {
		m, err := t.Map()
		if errors.Is(err, io.EOF) {
			break
		}
		row := t.Row
		res.Rows++
		if err != nil {
			res.bad(row, "%v", err)
			continue
		}
		switch b.Kind {
		case "premises":
			lat, lon, ok := locate(row, m)
			if !ok {
				continue
			}
			p := cad.Premise{ID: strings.ReplaceAll(get(m, "id"), " ", "-"), Name: get(m, "name"), Address: firstNonEmptyStr(get(m, "address"), fmt.Sprintf("%.6f, %.6f", lat, lon)), Lat: lat, Lon: lon,
				Hazards: splitList(get(m, "hazards")), Notes: get(m, "notes"), Contacts: splitList(get(m, "contacts"))}
			if e := get(m, "expires"); e != "" {
				d, ok := parseWhen(e)
				if !ok {
					res.bad(row, "the expiry %q is not a date", e)
					continue
				}
				p.Expires = d.Format("2006-01-02")
			}
			premises = append(premises, p)
		case "history":
			at, ok := parseWhen(get(m, "at"))
			if !ok {
				res.bad(row, "the date %q is not one bitdispatcher reads (2024-03-01 14:05, 03/01/2024 2:05 PM…)", get(m, "at"))
				continue
			}
			lat, lon, ok := locate(row, m)
			if !ok {
				continue
			}
			history = append(history, cad.History{Number: get(m, "number"), Type: strings.ToUpper(get(m, "type")), Address: get(m, "address"), Lat: lat, Lon: lon, At: at,
				Disposition: get(m, "disposition"), Note: get(m, "note")})
		case "runcards":
			ct := cad.CallType{Code: strings.ToUpper(get(m, "code")), Name: firstNonEmptyStr(get(m, "name"), get(m, "code")), Priority: 2, Discipline: discipline(get(m, "discipline"))}
			if p := get(m, "priority"); p != "" {
				n, err := strconv.Atoi(strings.TrimLeft(strings.ToUpper(p), "P"))
				if err != nil || n < 1 || n > 5 {
					res.bad(row, "the priority %q is 1 to 5", p)
					continue
				}
				ct.Priority = n
			}
			rc, err := parseRunCard(get(m, "runcard"))
			if err != nil {
				res.bad(row, "%s: %v", ct.Code, err)
				continue
			}
			for _, q := range rc {
				if !caps[q.Capability] {
					unknownCaps[q.Capability] = true
				}
			}
			ct.RunCard = rc
			types = append(types, ct)
		}
	}
	res.Good = len(premises) + len(history) + len(types)
	if len(unknownCaps) > 0 {
		var u []string
		for c := range unknownCaps {
			u = append(u, c)
		}
		sort.Strings(u)
		res.Notes = append(res.Notes, "No unit type has "+strings.Join(u, ", ")+": add it to a unit type in Settings, or those lines will never be filled")
	}
	if b.Check || res.Good == 0 {
		writeJSON(w, 200, res)
		return
	}
	switch b.Kind {
	case "premises":
		res.Imported, err = s.Eng.ImportPremises(a, source, premises)
	case "history":
		res.Imported, err = s.Eng.ImportHistory(a, source, history)
	case "runcards":
		for _, ct := range types {
			if err = s.Eng.SaveCallType(a, ct); err != nil {
				err = fmt.Errorf("%s: %w", ct.Code, err)
				break
			}
			res.Imported++
		}
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, res)
}

// geocode finds an address: a county or map address point, or one placed by an address range.
func (s *Server) geocode(ctx context.Context, addr string) (float64, float64, bool) {
	if s.Maps == nil || s.Maps.Store() == nil {
		return 0, 0, false
	}
	rs, err := s.search(ctx, s.Maps.Store(), addr, 0, 0, 3)
	if err != nil {
		return 0, 0, false
	}
	for _, r := range rs {
		if r.Kind == "address" && (r.Exact || r.Ranged) {
			return r.Lat, r.Lon, true
		}
	}
	return 0, 0, false
}

func splitList(s string) []string {
	var out []string
	for _, x := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == '|' || r == '\n' || r == '\r' }) {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func discipline(s string) string {
	switch u := strings.ToUpper(strings.TrimSpace(s)); {
	case u == "":
		return "other"
	case strings.Contains(u, "EMS") || strings.Contains(u, "MED") || strings.Contains(u, "AMB"):
		return "ems"
	case strings.Contains(u, "FIRE") || u == "FD" || u == "F":
		return "fire"
	case strings.Contains(u, "POL") || strings.Contains(u, "LAW") || u == "PD" || u == "LE" || u == "SO" || strings.Contains(u, "SHERIFF"):
		return "police"
	}
	return "other"
}

var reRunItem = regexp.MustCompile(`^(?:(\d+)\s*[x×]?\s+)?([A-Za-z][A-Za-z0-9 _/-]*?)(?:\s*(?:[:=x×]|\s)\s*(\d+))?$`)

// parseRunCard reads "2 engine, 1 ladder; als:1", "engine x2 + medic": capabilities with counts.
func parseRunCard(s string) ([]cad.Requirement, error) {
	var out []cad.Requirement
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '+' || r == '|' || r == '/' }) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		m := reRunItem.FindStringSubmatch(part)
		if m == nil {
			return nil, fmt.Errorf("%q is not a count and a capability (2 engine, ladder:1)", part)
		}
		n := 1
		if m[1] != "" {
			n, _ = strconv.Atoi(m[1])
		} else if m[3] != "" {
			n, _ = strconv.Atoi(m[3])
		}
		if n < 1 || n > 20 {
			return nil, fmt.Errorf("%q: 1 to 20 units", part)
		}
		out = append(out, cad.Requirement{Capability: strings.ToLower(strings.ReplaceAll(strings.TrimSpace(m[2]), " ", "-")), Count: n})
	}
	if len(out) == 0 {
		return nil, errors.New("no run card")
	}
	return out, nil
}

var whenLayouts = []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02", "01/02/2006 15:04:05", "01/02/2006 15:04",
	"1/2/2006 3:04:05 PM", "1/2/2006 3:04 PM", "1/2/2006 15:04:05", "1/2/2006 15:04", "01/02/2006", "1/2/2006", "20060102150405", "20060102", "2006/01/02 15:04:05", "2006/01/02"}

// parseWhen reads the dates and times spreadsheets hold, in local time.
func parseWhen(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, l := range whenLayouts {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func (s *Server) premiseList(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	out := []*cad.Premise{}
	total := 0
	s.Eng.Read(func(st *cad.State) {
		total = len(st.Premises)
		for _, p := range st.Premises {
			if q != "" && !strings.Contains(strings.ToLower(p.Address+" "+p.Name+" "+strings.Join(p.Hazards, " ")+" "+p.ID), q) {
				continue
			}
			c := *p
			out = append(out, &c)
		}
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Address < out[j].Address })
	writeJSON(w, 200, map[string]any{"total": total, "premises": out[:min(len(out), 500)], "matched": len(out)})
}

func (s *Server) premiseSave(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var p cad.Premise
	if !readJSON(w, r, &p) {
		return
	}
	if p.Lat == 0 && p.Lon == 0 && p.Address != "" {
		p.Lat, p.Lon, _ = s.geocode(r.Context(), p.Address)
	}
	id, err := s.Eng.SavePremise(a, p)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"id": id})
}

func (s *Server) premiseRemove(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	if err := s.Eng.RemovePremise(a, r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
