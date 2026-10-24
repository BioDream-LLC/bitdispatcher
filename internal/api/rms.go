package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ibr"
	"github.com/biodream-llc/bitdispatcher/internal/ncic"
)

func (s *Server) rmsRoutes() {
	s.handle("GET /api/rms/meta", auth.Viewer, s.rmsMeta)
	s.handle("GET /api/rms/reports", auth.Viewer, s.rmsList)
	s.handle("GET /api/rms/reports/{n}", auth.Viewer, s.rmsGet)
	s.handle("POST /api/rms/reports", auth.Viewer, s.rmsSave)
	s.handle("POST /api/rms/reports/{n}/status", auth.Viewer, s.rmsStatus)
	s.handle("POST /api/ncic/query", auth.Viewer, s.ncicQuery)
	s.handle("GET /api/rms/ibr", auth.Supervisor, s.ibrPreview)
	s.handle("POST /api/rms/ibr", auth.Supervisor, s.ibrFile)
}

// writer reports whether a session may write reports and make inquiries: dispatchers and up, and field
// crews (officers write their own reports from the car).
func writer(w http.ResponseWriter, se *auth.Session) bool {
	if se.Role == auth.Field || auth.AtLeast(se.Role, auth.Dispatcher) {
		return true
	}
	fail(w, http.StatusForbidden, "your role ("+se.Role+") cannot do this")
	return false
}

func (s *Server) rmsMeta(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var mode, ori string
	s.Eng.Read(func(st *cad.State) { mode, ori = st.Settings.NCICMode, st.Settings.ORI })
	writeJSON(w, 200, map[string]any{"offenses": cad.NIBRSOffenses, "locations": cad.NIBRSLocations, "codes": cad.Codes, "needs": cad.NIBRSNeeds, "ncic": map[string]any{
		"mode": firstNonEmptyStr(mode, "off"), "ori": ori, "kinds": ncic.Kinds, "fields": ncic.FieldNames, "purposes": ncic.Purposes}})
}

// ReportLine is a report as the list shows it.
type ReportLine struct {
	cad.RMSReport
	Problems int `json:"problems"`
}

func (s *Server) rmsList(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	status := r.URL.Query().Get("status")
	out := []ReportLine{}
	s.Eng.Read(func(st *cad.State) {
		for _, rp := range st.Reports {
			if status != "" && rp.Status != status {
				continue
			}
			// A field crew sees its own reports; others see all.
			if se.Role == auth.Field && rp.Author != se.Username {
				continue
			}
			line := ReportLine{RMSReport: *rp, Problems: len(cad.CheckReport(*rp))}
			line.Narrative = ""
			out = append(out, line)
		}
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	writeJSON(w, 200, out)
}

func (s *Server) rmsGet(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var rp *cad.RMSReport
	s.Eng.Read(func(st *cad.State) {
		if x, ok := st.Reports[r.PathValue("n")]; ok {
			c := *x
			rp = &c
		}
	})
	if rp == nil || (se.Role == auth.Field && rp.Author != se.Username) {
		fail(w, http.StatusNotFound, "no such report")
		return
	}
	writeJSON(w, 200, s.reportView(r.Context(), *rp))
}

// reportView is a report with what it lacks for the state, and the agency's own checks from its report
// scripts (advice: they do not stop a submission).
func (s *Server) reportView(ctx context.Context, rp cad.RMSReport) map[string]any {
	v := map[string]any{"report": rp, "checks": nonNil(cad.CheckReport(rp)), "agency": []cad.RMSCheck{}}
	if s.Advisory != nil && s.Advisory.HasScripts("report") {
		agency, faults := s.Advisory.CheckReport(ctx, rp)
		for _, f := range faults {
			agency = append(agency, cad.RMSCheck{Field: "agency", Message: "Script " + f.Name + " failed: " + f.Message})
		}
		v["agency"] = nonNil(agency)
	}
	return v
}

func nonNil(c []cad.RMSCheck) []cad.RMSCheck {
	if c == nil {
		return []cad.RMSCheck{}
	}
	return c
}

func (s *Server) rmsSave(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	if !writer(w, se) {
		return
	}
	var rp cad.RMSReport
	if !readJSON(w, r, &rp) {
		return
	}
	if rp.Number != "" {
		var author string
		s.Eng.Read(func(st *cad.State) {
			if x, ok := st.Reports[rp.Number]; ok {
				author = x.Author
			}
		})
		if se.Role == auth.Field && author != se.Username {
			fail(w, http.StatusForbidden, "a field crew changes only its own reports")
			return
		}
	}
	n, err := s.Eng.SaveReport(a, rp)
	if err != nil {
		writeErr(w, err)
		return
	}
	var saved cad.RMSReport
	s.Eng.Read(func(st *cad.State) { saved = *st.Reports[n] })
	writeJSON(w, 200, s.reportView(r.Context(), saved))
}

func (s *Server) rmsStatus(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	if !writer(w, se) {
		return
	}
	var b struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	if err := s.Eng.SetReportStatus(a, r.PathValue("n"), b.Status, b.Note, auth.AtLeast(se.Role, auth.Supervisor)); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) ncicQuery(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	if !writer(w, se) {
		return
	}
	var q ncic.Query
	if !readJSON(w, r, &q) {
		return
	}
	q.Key = strings.ToUpper(q.Key)
	if err := q.Check(); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	var mode, ori, addr string
	s.Eng.Read(func(st *cad.State) { mode, ori, addr = st.Settings.NCICMode, st.Settings.ORI, st.Settings.NCICAddr })
	var sw ncic.Switch
	switch mode {
	case "sim":
		sw = ncic.Sim{}
	case "line":
		if addr == "" {
			fail(w, http.StatusConflict, "no message switch address in Settings")
			return
		}
		sw = ncic.Line{Addr: addr}
	default:
		fail(w, http.StatusConflict, "NCIC inquiries are off: an administrator sets the switch in Settings")
		return
	}
	if ori == "" {
		ori = "VA0000000"
	}
	msg := ncic.Format(q, ori)
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	text, err := sw.Send(ctx, msg)
	ans := ncic.Answer{Text: text, Simulated: mode == "sim", At: time.Now().UTC(), Message: msg, Hit: err == nil && ncic.IsHit(text)}
	result := "no record"
	switch {
	case err != nil:
		result = "error"
	case ans.Hit:
		result = "hit"
	}
	// The ledger keeps who asked, why and what kind of answer came back: the terms only as a hash, the
	// answer not at all (OPERATIONS.md 3.3, 8).
	s.Eng.Record(a, cad.EvJusticeQuery, map[string]any{"key": q.Key, "purpose": q.Purpose, "reason": q.Reason, "terms": ncic.Hash(q), "result": result, "simulated": ans.Simulated})
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, 200, ans)
}

// ibrMonth is the submission a month makes: the reports approved in it (the state takes a month's data
// by the 15th of the next).
func (s *Server) ibrMonth(r *http.Request) (ibr.Submission, error) {
	m, err := time.ParseInLocation("2006-01", r.URL.Query().Get("month"), time.Local)
	if err != nil {
		return ibr.Submission{}, &cad.Problem{Field: "month", Message: "the month is YYYY-MM"}
	}
	sub := ibr.Submission{Year: m.Year(), Month: int(m.Month()), Loc: time.Local, FBI: r.URL.Query().Get("format") == "fbi"}
	s.Eng.Read(func(st *cad.State) {
		sub.ORI = strings.ToUpper(st.Settings.ORI)
		for _, rp := range st.Reports {
			if rp.Status == cad.RptApproved && rp.Decided.In(time.Local).Format("2006-01") == m.Format("2006-01") {
				sub.Reports = append(sub.Reports, *rp)
			}
		}
	})
	return sub, nil
}

func (s *Server) ibrPreview(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	sub, err := s.ibrMonth(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	_, sum, err := ibr.Write(sub)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	nums := []string{}
	for _, rp := range sub.Reports {
		nums = append(nums, rp.Number)
	}
	sort.Strings(nums)
	writeJSON(w, 200, map[string]any{"summary": sum, "reports": nums, "ori": sub.ORI, "due": time.Date(sub.Year, time.Month(sub.Month)+1, 15, 0, 0, 0, 0, time.Local).Format("2006-01-02")})
}

// ibrFile makes the file to upload to the Virginia Crime Repository, and records that it was made.
func (s *Server) ibrFile(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	sub, err := s.ibrMonth(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	b, sum, err := ibr.Write(sub)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	h := sha256.Sum256(b)
	nums := []string{}
	for _, rp := range sub.Reports {
		nums = append(nums, rp.Number)
	}
	sort.Strings(nums)
	s.Eng.Record(a, cad.EvIBRExported, map[string]any{"month": fmt.Sprintf("%04d-%02d", sub.Year, sub.Month), "ori": sub.ORI, "reports": nums,
		"incidents": sum.Incidents, "arrests": sum.Arrests, "zero": sum.Zero, "fbi": sub.FBI, "sha256": hex.EncodeToString(h[:])})
	w.Header().Set("Content-Type", "text/plain; charset=us-ascii")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%04d%02d.%s.txt"`, sub.ORI, sub.Year, sub.Month, map[bool]string{true: "nibrs", false: "ibr"}[sub.FBI]))
	w.Write(b)
}
