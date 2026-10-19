package cad

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// Reports are built from the ledger alone (docs/OPERATIONS.md section 4): the ledger is replayed into a
// fresh state, and every total keeps the sequence numbers of the events that make it, so a supervisor can
// open any number and see what it is made of. Nothing is retyped and nothing is kept outside the ledger
// except the certification, which is itself a ledger event.

// Count is one total with its evidence.
type Count struct {
	Key    string   `json:"key"`
	Label  string   `json:"label"`
	N      int      `json:"n"`
	Events []uint64 `json:"events"`
}

// Durations summarises a set of intervals in seconds.
type Durations struct {
	N      int      `json:"n"`
	Median float64  `json:"median"`
	P90    float64  `json:"p90"`
	Max    float64  `json:"max"`
	Events []uint64 `json:"events"` // the events that end each interval
}

// ResponseRow is the response times for a group of incidents.
type ResponseRow struct {
	Key            string    `json:"key"`
	Label          string    `json:"label"`
	Incidents      int       `json:"incidents"`
	ToDispatch     Durations `json:"toDispatch"` // created to the first unit dispatched
	ToEnroute      Durations `json:"toEnroute"`  // first dispatched to the first en route
	ToOnScene      Durations `json:"toOnScene"`  // created to the first on scene
	OnSceneToClear Durations `json:"onSceneToClear"`
}

// IncidentLine is an incident as a report lists it.
type IncidentLine struct {
	Number      string    `json:"number"`
	Type        string    `json:"type"`
	TypeName    string    `json:"typeName"`
	Priority    int       `json:"priority"`
	Address     string    `json:"address,omitempty"`
	Created     time.Time `json:"created"`
	Closed      time.Time `json:"closed,omitzero"`
	Disposition string    `json:"disposition,omitempty"`
	Units       []string  `json:"units"`
	Seq         uint64    `json:"seq"`
}

// Timeout is one continuous episode of a unit past its status timer (counted once per episode).
type Timeout struct {
	Unit     string    `json:"unit"`
	Status   string    `json:"status"`
	Incident string    `json:"incident,omitempty"`
	Started  time.Time `json:"started"`
	Due      time.Time `json:"due"`
	Ended    time.Time `json:"ended,omitzero"` // zero: still running at the end of the period
	Over     float64   `json:"overSeconds"`
	Seq      uint64    `json:"seq"`
}

// EmergencyLine is one emergency-button activation.
type EmergencyLine struct {
	Unit    string    `json:"unit"`
	Alias   string    `json:"alias,omitempty"`
	Raised  time.Time `json:"raised"`
	AckedBy string    `json:"ackedBy,omitempty"`
	AckSecs float64   `json:"ackSeconds,omitempty"`
	Where   string    `json:"where,omitempty"`
	Seq     uint64    `json:"seq"`
}

// PositionActivity is what one person did, counted by the rules in OPERATIONS.md section 7.
type PositionActivity struct {
	Actor      string   `json:"actor"`
	Consoles   []string `json:"consoles"`
	Created    int      `json:"created"`
	Dispatched int      `json:"dispatched"` // dispatch actions
	Statuses   int      `json:"statuses"`
	Notes      int      `json:"notes"`
	Closed     int      `json:"closed"`
	MoveUps    int      `json:"moveUps"`
	Touched    int      `json:"touched"` // distinct incidents acted on
	Actions    int      `json:"actions"`
	Events     []uint64 `json:"events"`
}

// UnitDemand is a unit's committed time in the period, by phase.
type UnitDemand struct {
	Unit        string             `json:"unit"`
	Type        string             `json:"type"`
	Incidents   int                `json:"incidents"`
	Committed   float64            `json:"committedSeconds"`
	ByPhase     map[string]float64 `json:"byPhase"`
	Utilisation float64            `json:"utilisation"` // committed time over the period
	OOS         float64            `json:"oosSeconds"`
	MarkedOut   float64            `json:"markedOutSeconds"`
}

// SystemLine is a system or configuration event in the period.
type SystemLine struct {
	At   time.Time `json:"at"`
	Type string    `json:"type"`
	By   string    `json:"by"`
	Text string    `json:"text"`
	Seq  uint64    `json:"seq"`
}

// Certification is a supervisor's sign-off on a report.
type Certification struct {
	By      string    `json:"by"`
	At      time.Time `json:"at"`
	Hash    string    `json:"hash"`
	Comment string    `json:"comment,omitempty"`
	Current bool      `json:"current"` // the report has not changed since
	Seq     uint64    `json:"seq"`
}

// Report is the Daily Operations Report and shift debrief for a period.
type Report struct {
	From      time.Time `json:"from"`
	To        time.Time `json:"to"`
	Key       string    `json:"key"`
	Agency    string    `json:"agency"`
	Generated time.Time `json:"generated"`
	ToSeq     uint64    `json:"toSeq"` // the last ledger event counted

	Totals         []Count            `json:"totals"`
	ByType         []Count            `json:"byType"`
	ByPriority     []Count            `json:"byPriority"`
	ByHour         []Count            `json:"byHour"`
	ByDisposition  []Count            `json:"byDisposition"`
	Response       []ResponseRow      `json:"response"`
	Significant    []IncidentLine     `json:"significant"`
	CarriedIn      []IncidentLine     `json:"carriedIn"`
	CarriedOut     []IncidentLine     `json:"carriedOut"`
	Timeouts       []Timeout          `json:"timeouts"`
	Emergencies    []EmergencyLine    `json:"emergencies"`
	Positions      []PositionActivity `json:"positions"`
	Units          []UnitDemand       `json:"units"`
	System         []SystemLine       `json:"system"`
	Hash           string             `json:"hash"`
	Certifications []Certification    `json:"certifications"`
}

// ReportKey names a period, for certifications.
func ReportKey(from, to time.Time) string {
	return from.UTC().Format(time.RFC3339) + "/" + to.UTC().Format(time.RFC3339)
}

type certData struct {
	Key     string `json:"key"`
	Hash    string `json:"hash"`
	Comment string `json:"comment,omitempty"`
}

// Report builds the report for a period from the ledger.
func (e *Engine) Report(from, to time.Time) (Report, error) {
	if !to.After(from) {
		return Report{}, problem("to", "the period must end after it starts")
	}
	if to.Sub(from) > 93*24*time.Hour {
		return Report{}, problem("to", "a report covers at most 93 days")
	}
	events, err := e.allEvents()
	if err != nil {
		return Report{}, err
	}
	return buildReport(events, from, to, e.now())
}

// CertifyReport records a supervisor's sign-off on the report as it stands now.
func (e *Engine) CertifyReport(a Actor, from, to time.Time, hash, comment string) error {
	if to.After(e.now()) {
		return problem("to", "a report is certified once its period has ended")
	}
	r, err := e.Report(from, to)
	if err != nil {
		return err
	}
	if r.Hash != hash {
		return problem("hash", "the report changed while you read it: look at it again before certifying")
	}
	if len(comment) > 4000 {
		return problem("comment", "the comment is too long")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err = e.write(a, EvReportCertified, refs{}, certData{Key: r.Key, Hash: hash, Comment: strings.TrimSpace(comment)})
	return err
}

type durs struct {
	v   []float64
	seq []uint64
}

func (d *durs) add(sec float64, seq uint64) {
	if sec >= 0 {
		d.v, d.seq = append(d.v, sec), append(d.seq, seq)
	}
}

func (d *durs) summary() Durations {
	out := Durations{N: len(d.v), Events: d.seq}
	if out.Events == nil {
		out.Events = []uint64{}
	}
	if len(d.v) == 0 {
		return out
	}
	s := slices.Clone(d.v)
	sort.Float64s(s)
	q := func(p float64) float64 {
		i := p * float64(len(s)-1)
		lo, hi := int(math.Floor(i)), int(math.Ceil(i))
		return s[lo] + (s[hi]-s[lo])*(i-float64(lo))
	}
	out.Median, out.P90, out.Max = math.Round(q(0.5)), math.Round(q(0.9)), s[len(s)-1]
	return out
}

type counter struct {
	m     map[string]*Count
	order []string
}

func newCounter() *counter { return &counter{m: map[string]*Count{}} }

func (c *counter) add(key, label string, seq uint64) {
	x, ok := c.m[key]
	if !ok {
		x = &Count{Key: key, Label: label, Events: []uint64{}}
		c.m[key] = x
		c.order = append(c.order, key)
	}
	x.N++
	x.Events = append(x.Events, seq)
}

func (c *counter) list(byN bool) []Count {
	out := make([]Count, 0, len(c.m))
	for _, k := range c.order {
		out = append(out, *c.m[k])
	}
	if byN {
		sort.SliceStable(out, func(i, j int) bool { return out[i].N > out[j].N })
	} else {
		sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	}
	return out
}

// incTrack is what the report keeps per incident while replaying.
type incTrack struct {
	created, firstDisp, firstEnr, firstOS, lastClear time.Time
	dispSeq, enrSeq, osSeq, clearSeq                 uint64
}

// unitTrack is a unit's current status episode.
type unitTrack struct {
	status, incident string
	limit            int // minutes before a status check is due
	since            time.Time
	seq              uint64
	timedOut         bool
}

func buildReport(events []ledger.Event, from, to, now time.Time) (Report, error) {
	key := ReportKey(from, to)
	if to.After(now) {
		to = now // a period still running is reported to now
	}
	st := newState()
	r := Report{From: from, To: to, Key: key, Generated: now}
	in := func(t time.Time) bool { return !t.Before(from) && t.Before(to) }
	totals := newCounter()
	byType, byPri, byHour, byDisp := newCounter(), newCounter(), newCounter(), newCounter()
	incs := map[string]*incTrack{}
	units := map[string]*unitTrack{}
	demand := map[string]*UnitDemand{}
	unitIncs := map[string]map[string]bool{}
	pos := map[string]*PositionActivity{}
	touched := map[string]map[string]bool{}
	emerg := map[string]*EmergencyLine{}
	var emOrder []string
	carriedIn := map[string]bool{}
	snapped := false
	var certs []Certification

	// timers and committed time accrue between events; flush handles one unit up to a moment.
	flush := func(id string, u *unitTrack, until time.Time) {
		if u.status == "" {
			return
		}
		a, b := u.since, until
		if a.Before(from) {
			a = from
		}
		if b.After(to) {
			b = to
		}
		if b.After(a) {
			d := demand[id]
			if d == nil {
				typ := ""
				if x, ok := st.Units[id]; ok {
					typ = x.Type
				}
				d = &UnitDemand{Unit: id, Type: typ, ByPhase: map[string]float64{}}
				demand[id] = d
			}
			secs := b.Sub(a).Seconds()
			if Committed(u.status) {
				d.Committed += secs
				d.ByPhase[u.status] += secs
			} else if u.status == OutOfService {
				d.OOS += secs
			} else if u.status == MarkedOut {
				d.MarkedOut += secs
			}
		}
		// A timer that ran out during the episode, counted once.
		if mins := u.limit; mins > 0 && !u.timedOut {
			due := u.since.Add(time.Duration(mins) * time.Minute)
			if due.Before(until) && in(due) {
				u.timedOut = true
				t := Timeout{Unit: id, Status: u.status, Incident: u.incident, Started: u.since, Due: due, Seq: u.seq}
				if until.Before(to) {
					t.Ended = until
				}
				t.Over = math.Round(math.Min(until.Sub(due).Seconds(), to.Sub(due).Seconds()))
				r.Timeouts = append(r.Timeouts, t)
			}
		}
	}
	act := func(ev ledger.Event, f func(p *PositionActivity)) {
		if !in(ev.At) || ev.Actor == "" || ev.Role == "" {
			return
		}
		p := pos[ev.Actor]
		if p == nil {
			p = &PositionActivity{Actor: ev.Actor, Consoles: []string{}, Events: []uint64{}}
			pos[ev.Actor] = p
			touched[ev.Actor] = map[string]bool{}
		}
		if ev.Console != "" && !slices.Contains(p.Consoles, ev.Console) {
			p.Consoles = append(p.Consoles, ev.Console)
		}
		f(p)
		p.Actions++
		p.Events = append(p.Events, ev.Seq)
		if ev.Incident != "" {
			touched[ev.Actor][ev.Incident] = true
		}
	}
	line := func(n string, seq uint64) IncidentLine {
		x := st.Incidents[n]
		l := IncidentLine{Number: n, Type: x.Type, TypeName: st.typeLabel(x.Type), Priority: x.Priority, Created: x.Created, Closed: x.Closed, Disposition: x.Disposition, Units: []string{}, Seq: seq}
		if x.Location != nil {
			l.Address = x.Location.Address
		}
		for _, a := range x.Units {
			if !slices.Contains(l.Units, a.Unit) {
				l.Units = append(l.Units, a.Unit)
			}
		}
		return l
	}
	snapshot := func() {
		// Open at the start of the period: carried in.
		for n, x := range st.Incidents {
			if x.Status != Closed {
				carriedIn[n] = true
				r.CarriedIn = append(r.CarriedIn, line(n, 0))
			}
		}
		snapped = true
	}
	var last uint64
	for _, ev := range events {
		if ev.Type == EvReportCertified {
			if v, err := decode[certData](ev); err == nil && v.Key == r.Key {
				certs = append(certs, Certification{By: ev.Actor, At: ev.At, Hash: v.Hash, Comment: v.Comment, Seq: ev.Seq})
			}
		}
		if !ev.At.Before(to) {
			continue // later events are not counted (certifications above are read from the whole ledger)
		}
		if !snapped && !ev.At.Before(from) {
			snapshot()
		}
		var sd statusData
		var hadSD bool
		if ev.Type == EvUnitStatus {
			if v, err := decode[statusData](ev); err == nil {
				sd, hadSD = v, true
			}
		}
		if err := st.apply(ev); err != nil {
			return r, fmt.Errorf("event %d: %w", ev.Seq, err)
		}
		last = ev.Seq
		inPeriod := in(ev.At)
		switch ev.Type {
		case EvIncCreated:
			x := st.Incidents[ev.Incident]
			if x == nil {
				break
			}
			incs[ev.Incident] = &incTrack{created: ev.At}
			if inPeriod {
				totals.add("created", "Incidents created", ev.Seq)
				byType.add(x.Type, st.typeLabel(x.Type), ev.Seq)
				byPri.add(fmt.Sprintf("P%d", x.Priority), fmt.Sprintf("Priority %d", x.Priority), ev.Seq)
				byHour.add(fmt.Sprintf("%02d", ev.At.Local().Hour()), fmt.Sprintf("%02d:00", ev.At.Local().Hour()), ev.Seq)
				if x.Priority == 1 {
					r.Significant = append(r.Significant, line(ev.Incident, ev.Seq))
				}
			}
			act(ev, func(p *PositionActivity) { p.Created++ })
		case EvIncDispatch:
			if t := incs[ev.Incident]; t != nil && t.firstDisp.IsZero() {
				t.firstDisp, t.dispSeq = ev.At, ev.Seq
			}
			if inPeriod {
				totals.add("dispatches", "Dispatch actions", ev.Seq)
			}
			act(ev, func(p *PositionActivity) { p.Dispatched++ })
		case EvUnitChecked:
			// A status check starts the unit's timer again.
			if u := units[ev.Unit]; u != nil && u.status != "" {
				flush(ev.Unit, u, ev.At)
				u.since, u.timedOut, u.seq = ev.At, false, ev.Seq
			}
		case EvUnitStatus:
			if !hadSD {
				break
			}
			id := ev.Unit
			u := units[id]
			if u == nil {
				u = &unitTrack{}
				units[id] = u
			}
			flush(id, u, ev.At)
			var mo *MarkOut
			if sd.Status == MarkedOut {
				mo = &MarkOut{Code: sd.MarkOut}
			}
			*u = unitTrack{status: sd.Status, incident: sd.Incident, since: ev.At, seq: ev.Seq, limit: st.Settings.TimerFor(sd.Status, mo)}
			if t := incs[sd.Incident]; t != nil {
				switch sd.Status {
				case Dispatched:
					if t.firstDisp.IsZero() {
						t.firstDisp, t.dispSeq = ev.At, ev.Seq
					}
				case Enroute:
					if t.firstEnr.IsZero() {
						t.firstEnr, t.enrSeq = ev.At, ev.Seq
					}
				case OnScene:
					if t.firstOS.IsZero() {
						t.firstOS, t.osSeq = ev.At, ev.Seq
					}
				}
				if inPeriod && sd.Incident != "" {
					if unitIncs[id] == nil {
						unitIncs[id] = map[string]bool{}
					}
					unitIncs[id][sd.Incident] = true
				}
			}
			if t := incs[sd.PrevIncident]; t != nil && sd.PrevIncident != sd.Incident {
				t.lastClear, t.clearSeq = ev.At, ev.Seq
			}
			act(ev, func(p *PositionActivity) { p.Statuses++ })
		case EvIncNote:
			act(ev, func(p *PositionActivity) { p.Notes++ })
		case EvUnitCover:
			act(ev, func(p *PositionActivity) { p.MoveUps++ })
		case EvIncClosed:
			x := st.Incidents[ev.Incident]
			if inPeriod && x != nil {
				totals.add("closed", "Incidents closed", ev.Seq)
				byDisp.add(x.Disposition, x.Disposition, ev.Seq)
			}
			if t := incs[ev.Incident]; t != nil && t.lastClear.IsZero() {
				t.lastClear, t.clearSeq = ev.At, ev.Seq
			}
			act(ev, func(p *PositionActivity) { p.Closed++ })
		case EvIncChanged, EvIncReopened, EvCallAttached, EvCallAction:
			act(ev, func(p *PositionActivity) {})
		case EvCallReceived:
			if inPeriod {
				totals.add("calls", "Calls received", ev.Seq)
			}
		case EvEmergency:
			if inPeriod {
				v, _ := decode[Emergency](ev)
				l := &EmergencyLine{Unit: firstNonEmpty(v.Unit, v.RadioUnit), Alias: v.Alias, Raised: ev.At, Seq: ev.Seq}
				emerg[v.Key] = l
				emOrder = append(emOrder, v.Key)
				totals.add("emergencies", "Emergency activations", ev.Seq)
			}
		case EvEmergencyAck:
			v, _ := decode[Emergency](ev)
			if l := emerg[v.Key]; l != nil {
				l.AckedBy, l.AckSecs, l.Where = v.AckedBy, math.Round(ev.At.Sub(l.Raised).Seconds()), v.AckedWhere
			}
			act(ev, func(p *PositionActivity) {})
		case EvSettings, EvStationSaved, EvStationRemoved, EvUnitSaved, EvUnitRemoved, EvCallTypeSaved, EvCallTypeRemoved, EvUserSaved, EvUserRemoved,
			EvIntegration, EvMapRecording, EvMapRecorded, EvMapFailed, EvPhotosRecording, EvPhotosRecorded, EvPhotosFailed:
			if inPeriod {
				r.System = append(r.System, SystemLine{At: ev.At, Type: ev.Type, By: ev.Actor, Text: systemText(ev), Seq: ev.Seq})
			}
		}
	}
	if !snapped {
		snapshot()
	}
	r.ToSeq = last
	// Status episodes still running at the end of the period.
	for id, u := range units {
		flush(id, u, to)
	}
	// Response times, for incidents created in the period, by priority and by type.
	type group struct {
		label                      string
		n                          int
		disp, enr, onscene, oscClr durs
	}
	groups := map[string]*group{}
	var gorder []string
	add := func(key, label string, t *incTrack) {
		g := groups[key]
		if g == nil {
			g = &group{label: label}
			groups[key] = g
			gorder = append(gorder, key)
		}
		g.n++
		if !t.firstDisp.IsZero() {
			g.disp.add(t.firstDisp.Sub(t.created).Seconds(), t.dispSeq)
			if !t.firstEnr.IsZero() {
				g.enr.add(t.firstEnr.Sub(t.firstDisp).Seconds(), t.enrSeq)
			}
		}
		if !t.firstOS.IsZero() {
			g.onscene.add(t.firstOS.Sub(t.created).Seconds(), t.osSeq)
			if !t.lastClear.IsZero() && t.lastClear.After(t.firstOS) {
				g.oscClr.add(t.lastClear.Sub(t.firstOS).Seconds(), t.clearSeq)
			}
		}
	}
	nums := make([]string, 0, len(incs))
	for n, t := range incs {
		if in(t.created) {
			nums = append(nums, n)
		}
	}
	sort.Strings(nums)
	for _, n := range nums {
		x, t := st.Incidents[n], incs[n]
		add("all", "All incidents", t)
		add(fmt.Sprintf("P%d", x.Priority), fmt.Sprintf("Priority %d", x.Priority), t)
		add("type:"+x.Type, st.typeLabel(x.Type), t)
	}
	sort.SliceStable(gorder, func(i, j int) bool {
		rank := func(k string) string {
			switch {
			case k == "all":
				return "0"
			case strings.HasPrefix(k, "P"):
				return "1" + k
			}
			return "2" + k
		}
		return rank(gorder[i]) < rank(gorder[j])
	})
	for _, k := range gorder {
		g := groups[k]
		r.Response = append(r.Response, ResponseRow{Key: k, Label: g.label, Incidents: g.n, ToDispatch: g.disp.summary(), ToEnroute: g.enr.summary(),
			ToOnScene: g.onscene.summary(), OnSceneToClear: g.oscClr.summary()})
	}
	// Open at the end: carried out to the next shift.
	open := []string{}
	for n, x := range st.Incidents {
		if x.Status != Closed {
			open = append(open, n)
		}
	}
	sort.Strings(open)
	for _, n := range open {
		r.CarriedOut = append(r.CarriedOut, line(n, 0))
	}
	for _, k := range emOrder {
		r.Emergencies = append(r.Emergencies, *emerg[k])
	}
	for actor, p := range pos {
		p.Touched = len(touched[actor])
		r.Positions = append(r.Positions, *p)
	}
	sort.Slice(r.Positions, func(i, j int) bool {
		a, b := r.Positions[i], r.Positions[j]
		if a.Actions != b.Actions {
			return a.Actions > b.Actions
		}
		return a.Actor < b.Actor
	})
	period := to.Sub(from).Seconds()
	for id, d := range demand {
		d.Incidents = len(unitIncs[id])
		if period > 0 {
			d.Utilisation = math.Round(d.Committed/period*1000) / 1000
		}
		d.Committed, d.OOS, d.MarkedOut = math.Round(d.Committed), math.Round(d.OOS), math.Round(d.MarkedOut)
		for k, v := range d.ByPhase {
			d.ByPhase[k] = math.Round(v)
		}
		if d.Committed > 0 || d.OOS > 0 || d.MarkedOut > 0 {
			r.Units = append(r.Units, *d)
		}
	}
	sort.Slice(r.Units, func(i, j int) bool {
		a, b := r.Units[i], r.Units[j]
		if a.Committed != b.Committed {
			return a.Committed > b.Committed
		}
		return a.Unit < b.Unit
	})
	sort.Slice(r.Timeouts, func(i, j int) bool {
		a, b := r.Timeouts[i], r.Timeouts[j]
		if !a.Due.Equal(b.Due) {
			return a.Due.Before(b.Due)
		}
		return a.Unit < b.Unit
	})
	sort.Slice(r.CarriedIn, func(i, j int) bool { return r.CarriedIn[i].Number < r.CarriedIn[j].Number })
	tl := totals.list(false)
	order := []string{"calls", "created", "dispatches", "closed", "emergencies"}
	sort.SliceStable(tl, func(i, j int) bool { return slices.Index(order, tl[i].Key) < slices.Index(order, tl[j].Key) })
	r.Totals, r.ByType, r.ByPriority, r.ByHour, r.ByDisposition = tl, byType.list(true), byPri.list(false), byHour.list(false), byDisp.list(true)
	r.Agency = st.Settings.Agency
	for _, p := range []*[]IncidentLine{&r.Significant, &r.CarriedIn, &r.CarriedOut} {
		if *p == nil {
			*p = []IncidentLine{}
		}
	}
	if r.Timeouts == nil {
		r.Timeouts = []Timeout{}
	}
	if r.Emergencies == nil {
		r.Emergencies = []EmergencyLine{}
	}
	if r.Positions == nil {
		r.Positions = []PositionActivity{}
	}
	if r.Units == nil {
		r.Units = []UnitDemand{}
	}
	if r.System == nil {
		r.System = []SystemLine{}
	}
	if r.Response == nil {
		r.Response = []ResponseRow{}
	}
	// The hash covers what the report says, not when it was generated.
	b, _ := json.Marshal(withoutVolatile(r))
	h := sha256.Sum256(b)
	r.Hash = hex.EncodeToString(h[:])
	r.Certifications = []Certification{}
	for _, c := range certs {
		c.Current = c.Hash == r.Hash
		r.Certifications = append(r.Certifications, c)
	}
	return r, nil
}

func withoutVolatile(r Report) Report {
	r.Generated, r.Hash, r.Certifications = time.Time{}, "", nil
	return r
}

func systemText(ev ledger.Event) string {
	switch ev.Type {
	case EvSettings:
		return "Settings changed"
	case EvIntegration:
		return "Integration settings changed"
	case EvMapRecording:
		return "Map recording started"
	case EvMapRecorded:
		return "A new recorded map went into use"
	case EvMapFailed:
		return "A map recording failed"
	case EvPhotosRecording:
		return "Aerial photo recording started"
	case EvPhotosRecorded:
		return "Aerial photo recording finished"
	case EvPhotosFailed:
		return "Aerial photo recording stopped"
	}
	t := strings.ReplaceAll(ev.Type, ".", " ")
	if ev.Unit != "" {
		t += " " + ev.Unit
	}
	var d struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(ev.Data, &d) == nil && d.ID != "" {
		t += " " + d.ID
	}
	return t
}
