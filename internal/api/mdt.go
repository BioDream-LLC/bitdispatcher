package api

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// The mobile data terminal: what a crew sees and does from the vehicle. A field user signs in with the
// unit's call sign as the position; every action is the crew's own, in the ledger as theirs.

func (s *Server) mdtRoutes() {
	s.handle("GET /api/mdt", auth.Viewer, s.mdt)
	s.handle("POST /api/mdt/status", auth.Viewer, s.mdtStatus)
	s.handle("POST /api/mdt/markout", auth.Viewer, s.mdtMarkOut)
	s.handle("POST /api/mdt/note", auth.Viewer, s.mdtNote)
	s.handle("POST /api/mdt/position", auth.Viewer, s.mdtPosition)
	s.handle("POST /api/mdt/emergency", auth.Viewer, s.mdtEmergency)
}

// mdtUnit is the unit a session works as: a field crew's own; a dispatcher may name one (for testing a
// terminal or working as a unit that has none); a viewer none.
func (s *Server) mdtUnit(w http.ResponseWriter, r *http.Request, se *auth.Session) (string, bool) {
	unit := ""
	switch {
	case se.Role == auth.Field:
		unit = se.Console
	case auth.AtLeast(se.Role, auth.Dispatcher):
		unit = strings.ToUpper(firstNonEmptyStr(r.URL.Query().Get("unit"), se.Console))
	default:
		fail(w, http.StatusForbidden, "the mobile terminal is for field crews and dispatchers")
		return "", false
	}
	known := false
	s.Eng.Read(func(st *cad.State) { _, known = st.Units[unit] })
	if !known {
		fail(w, http.StatusNotFound, "no unit "+unit)
		return "", false
	}
	return unit, true
}

func firstNonEmptyStr(v ...string) string {
	for _, x := range v {
		if x != "" {
			return x
		}
	}
	return ""
}

// MDTView is the terminal's screen.
type MDTView struct {
	ServerTime time.Time       `json:"serverTime"`
	Unit       UnitView        `json:"unit"`
	Station    *cad.Station    `json:"station,omitempty"`
	Incident   *cad.Incident   `json:"incident,omitempty"`
	TypeName   string          `json:"typeName,omitempty"`
	Summary    string          `json:"summary,omitempty"`   // the incident in a paragraph
	Partners   []UnitView      `json:"partners"`            // other units on the incident
	Premises   []*cad.Premise  `json:"premises"`            // hazards and notes at the incident's place
	Emergency  *cad.Emergency  `json:"emergency,omitempty"` // the unit's open emergency
	Pending    []*cad.Incident `json:"pending"`             // waiting for a unit, nearest first is the client's job
	Statuses   []string        `json:"statuses"`
	BOLOs      []*cad.BOLO     `json:"bolos"` // active BOLOs for the unit's discipline
	Settings   struct {
		Agency    string     `json:"agency"`
		MapCenter [2]float64 `json:"mapCenter"`
		MapZoom   float64    `json:"mapZoom"`
		AVLStale  int        `json:"avlStaleSeconds"`
		// MarkOuts are the mark-outs this unit may use.
		MarkOuts []cad.MarkOutType `json:"markOuts"`
	} `json:"settings"`
	Health     HealthView     `json:"health"`
	PlaceViews bool           `json:"placeViews,omitempty"` // pictures of the place from the side can be looked up
	Messages   []*cad.Message `json:"messages"`             // the last day's messages to and from this unit
	SelfAttach bool           `json:"selfAttach,omitempty"` // the crew may attach itself to a waiting incident
	// The roster to pick the crew from, the incident's alarm, cautions of the area and warrants at the
	// address (police units only).
	Roster   []MemberBrief  `json:"roster"`
	Staffing []string       `json:"staffing,omitempty"`
	Alarm    string         `json:"alarm,omitempty"`
	Cautions []CautionBrief `json:"cautions,omitempty"`
	Papers   *PaperFlag     `json:"papers,omitempty"`
}

func (s *Server) mdt(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	unit, ok := s.mdtUnit(w, r, se)
	if !ok {
		return
	}
	v := MDTView{ServerTime: time.Now().UTC(), Partners: []UnitView{}, Premises: []*cad.Premise{}, Pending: []*cad.Incident{}, Statuses: cad.Statuses, Health: s.health()}
	if vc := s.Views.Config(); vc.Any() || vc.GoogleLink {
		v.PlaceViews = true
	}
	s.Eng.Read(func(st *cad.State) {
		u := st.Units[unit]
		cp := *u
		v.Unit = UnitView{Unit: &cp, Caps: st.Capabilities(u)}
		if x, ok := st.Stations[firstNonEmptyStr(u.Covering, u.Station)]; ok {
			sc := *x
			v.Station = &sc
		}
		v.BOLOs = []*cad.BOLO{}
		for _, b := range st.ActiveBOLOs(time.Now()) {
			if len(b.Disciplines) == 0 || slices.Contains(b.Disciplines, discOf(st, u.Type)) {
				v.BOLOs = append(v.BOLOs, b)
			}
		}
		if in, ok := st.Incidents[u.Incident]; ok {
			ic := *in
			v.Incident = maskIncident(&ic)
			v.TypeName = in.Type
			v.Summary = st.Summary(in)
			if in.Location != nil && in.Location.Lat != nil && in.Location.Lon != nil {
				for _, p := range st.PremisesNear(*in.Location.Lat, *in.Location.Lon) {
					pc := *p
					v.Premises = append(v.Premises, &pc)
				}
			}
			if ct, ok := st.CallTypes[in.Type]; ok {
				v.TypeName = ct.Name
			}
			for _, as := range in.Units {
				if as.Unit != unit && as.Cleared.IsZero() {
					if p, ok := st.Units[as.Unit]; ok {
						pc := *p
						v.Partners = append(v.Partners, UnitView{Unit: &pc})
					}
				}
			}
		}
		if em, ok := st.Emergencies[u.Emergency]; ok {
			ec := *em
			v.Emergency = &ec
		}
		for _, in := range st.Incidents {
			if in.Status == cad.Pending {
				ic := *in
				v.Pending = append(v.Pending, maskIncident(&ic))
			}
		}
		v.Messages = copyMessages(st.MessagesFor(unit, time.Now().Add(-messageWindow)))
		v.Roster = []MemberBrief{}
		for _, m := range st.Roster() {
			if !m.Inactive {
				v.Roster = append(v.Roster, MemberBrief{ID: m.ID, Name: m.Name, Rank: m.Rank, Quals: m.Quals, Station: m.Station, Volunteer: m.Volunteer, Riding: st.RidingOn(m.ID)})
			}
		}
		v.Staffing = st.Staffing(st.Units[unit])
		if in := st.Incidents[st.Units[unit].Incident]; in != nil {
			if in.AlarmLevel() > 1 {
				v.Alarm = st.AlarmName(in)
			}
			if in.Location.HasPoint() {
				for _, g := range st.CautionsAt(*in.Location.Lat, *in.Location.Lon) {
					v.Cautions = append(v.Cautions, CautionBrief{ID: g.ID, Name: g.Name, Note: g.Note})
				}
			}
			if ps := st.PapersNear(in.Location); len(ps) > 0 {
				f := PaperFlag{Count: len(ps)}
				for _, p := range ps {
					if !slices.Contains(f.Kinds, p.Kind) {
						f.Kinds = append(f.Kinds, p.Kind)
					}
					if p.Caution != "" {
						f.Caution = strings.TrimPrefix(f.Caution+"; "+p.Caution, "; ")
					}
				}
				v.Papers = &f
			}
		}
		v.SelfAttach = st.Settings.Duties.SelfAttach
		v.Settings.Agency, v.Settings.MapCenter, v.Settings.MapZoom, v.Settings.AVLStale = st.Settings.Agency, st.Settings.MapCenter, st.Settings.MapZoom, st.Settings.AVLStaleSeconds
		v.Settings.MarkOuts = []cad.MarkOutType{}
		d := st.Discipline(u)
		for _, t := range st.Settings.MarkOuts {
			if t.Discipline == "" || t.Discipline == d {
				v.Settings.MarkOuts = append(v.Settings.MarkOuts, t)
			}
		}
	})
	writeJSON(w, 200, v)
}

func (s *Server) mdtStatus(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	unit, ok := s.mdtUnit(w, r, se)
	if !ok {
		return
	}
	var b struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	if err := s.Eng.SetStatus(a, unit, b.Status, "", b.Reason); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// mdtMarkOut marks the crew's own unit out, where its AVL (or the address it gives) says it is.
func (s *Server) mdtMarkOut(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	unit, ok := s.mdtUnit(w, r, se)
	if !ok {
		return
	}
	var b struct {
		Code    string `json:"code"`
		Address string `json:"address"`
		Note    string `json:"note"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	line := "MO " + unit + " " + strings.Fields(b.Code + " X")[0]
	if b.Address = strings.ReplaceAll(strings.TrimSpace(b.Address), "/", " "); b.Address != "" {
		line += " " + b.Address
	}
	if b.Note = strings.TrimSpace(b.Note); b.Note != "" {
		line += " / " + b.Note
	}
	res := s.Eng.Execute(a, line, "")
	if !res.OK {
		fail(w, http.StatusBadRequest, res.Message)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) mdtNote(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	unit, ok := s.mdtUnit(w, r, se)
	if !ok {
		return
	}
	var b struct {
		Text string `json:"text"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	inc := ""
	s.Eng.Read(func(st *cad.State) { inc = st.Units[unit].Incident })
	if inc == "" {
		fail(w, http.StatusConflict, unit+" is not on an incident")
		return
	}
	if err := s.Eng.AddNote(a, inc, unit+": "+strings.TrimSpace(b.Text), false); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) mdtPosition(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	unit, ok := s.mdtUnit(w, r, se)
	if !ok {
		return
	}
	var p cad.Position
	if !readJSON(w, r, &p) {
		return
	}
	if p.Lat < -90 || p.Lat > 90 || p.Lon < -180 || p.Lon > 180 || (p.Lat == 0 && p.Lon == 0) {
		fail(w, http.StatusBadRequest, "not a position")
		return
	}
	p.Source, p.At = "mdt", time.Time{}
	if err := s.Eng.UnitPosition(a, unit, p); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) mdtEmergency(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	unit, ok := s.mdtUnit(w, r, se)
	if !ok {
		return
	}
	key, err := s.Eng.RaiseEmergency(a, cad.Emergency{Unit: unit, Alias: unit, ChannelName: "mobile terminal"})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"key": key})
}

func discOf(st *cad.State, unitType string) string {
	for _, t := range st.Settings.UnitTypes {
		if t.Code == unitType {
			return t.Discipline
		}
	}
	return "other"
}
