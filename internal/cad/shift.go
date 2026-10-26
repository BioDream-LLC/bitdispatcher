package cad

// Shift Intelligence: a report for each operational period (watch, shift, tour), assembled from the
// records instead of typed again. It lists the period's major and open incidents, location exceptions and
// their review, new or critical flags, BOLOs and urgent messages, units short of staff, overdue status
// checks, open NCIC requests, critical incidents and the follow-up still owed, each with its source, time,
// status, owner, due date and next action. A supervisor adds a summary and handoff notes (kept apart from
// the facts), finalizes it (a version), and later changes are addenda, never edits. The incoming
// supervisor acknowledges it and accepts or reassigns the open follow-up, which carries forward, one item
// per source, until done.

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvShiftReport   = "shift.report"   // a supervisor finalized the period's report, or added an addendum
	EvShiftAck      = "shift.ack"      // the incoming supervisor acknowledged the report and took the follow-up
	EvFollowup      = "followup.saved" // a tracked follow-up opened, changed hands or was done
	EvShiftNote     = "shift.note"     // a person's correction or context note on their own shift record
	EvShiftReminded = "shift.reminded" // (reserved)
)

// ShiftPolicy is the agency's operational periods and report rules.
type ShiftPolicy struct {
	// Periods are the operational periods by name and start time, "Day" 07:00 (empty: Day 07:00, Night 19:00).
	Periods []ShiftPeriod `json:"periods,omitempty"`
	// TimeZone is the agency's time zone for periods (empty: the server's).
	TimeZone string `json:"timeZone,omitempty"`
	// Sections are the report's sections in order (empty: all).
	Sections []string `json:"sections,omitempty"`
	// MajorPriority: incidents of this priority or more urgent are listed as major (0: 2).
	MajorPriority int `json:"majorPriority,omitempty"`
	// RequireSummary asks the supervisor for a summary before finalizing.
	RequireSummary bool `json:"requireSummary,omitempty"`
	// Recipients are the duties that may read the report (empty: supervisors).
	Recipients []string `json:"recipients,omitempty"`
	// WorkloadRatio: a person handling this many times the median of their duty is a review prompt (0: 2).
	WorkloadRatio float64 `json:"workloadRatio,omitempty"`
}

// ShiftPeriod is one operational period of the day.
type ShiftPeriod struct {
	Name  string `json:"name"`
	Start string `json:"start"` // HH:MM
}

// ReportSections are the sections a Shift Intelligence report can have.
var ReportSections = []string{"major", "exceptions", "critical", "flags", "comms", "staffing", "checks", "requests", "followups"}

// SectionNames are their headings.
var SectionNames = map[string]string{"major": "Major and open incidents", "exceptions": "Forced and unverified locations", "critical": "Critical incidents",
	"flags": "New and critical flags", "comms": "BOLOs and urgent messages", "staffing": "Units short of staff", "checks": "Overdue status checks",
	"requests": "Open NCIC and state requests", "followups": "Follow-up owed"}

func (p ShiftPolicy) periods() []ShiftPeriod {
	if len(p.Periods) == 0 {
		return []ShiftPeriod{{Name: "Day", Start: "07:00"}, {Name: "Night", Start: "19:00"}}
	}
	out := slices.Clone(p.Periods)
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

func (p ShiftPolicy) loc() *time.Location {
	if p.TimeZone != "" {
		if l, err := time.LoadLocation(p.TimeZone); err == nil {
			return l
		}
	}
	return time.Local
}

// Period is one operational period: its stable ID ("2026-10-25/Day"), name and time range.
type Period struct {
	ID   string    `json:"id"`
	Name string    `json:"name"`
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// PeriodAt is the operational period a time falls in.
func (s *State) PeriodAt(t time.Time) Period {
	pol := s.Settings.Shift
	loc := pol.loc()
	lt := t.In(loc)
	ps := pol.periods()
	start := func(day time.Time, hm string) time.Time {
		var h, m int
		fmt.Sscanf(hm, "%d:%d", &h, &m)
		return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, loc)
	}
	day := time.Date(lt.Year(), lt.Month(), lt.Day(), 0, 0, 0, 0, loc)
	// Candidates: each period's start yesterday and today; the latest start not after t.
	var best time.Time
	var bestP ShiftPeriod
	for _, d := range []time.Time{day.AddDate(0, 0, -1), day} {
		for _, p := range ps {
			st := start(d, p.Start)
			if !st.After(lt) && st.After(best) {
				best, bestP = st, p
			}
		}
	}
	// It ends at the next period's start.
	var end time.Time
	for _, d := range []time.Time{best, best.AddDate(0, 0, 1)} {
		dd := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc)
		for _, p := range ps {
			st := start(dd, p.Start)
			if st.After(best) && (end.IsZero() || st.Before(end)) {
				end = st
			}
		}
		if !end.IsZero() {
			break
		}
	}
	return Period{ID: best.Format("2006-01-02") + "/" + bestP.Name, Name: bestP.Name, From: best, To: end}
}

// PeriodByID finds a period from its ID.
func (s *State) PeriodByID(id string) (Period, bool) {
	day, name, ok := strings.Cut(id, "/")
	if !ok {
		return Period{}, false
	}
	loc := s.Settings.Shift.loc()
	d, err := time.ParseInLocation("2006-01-02", day, loc)
	if err != nil {
		return Period{}, false
	}
	for _, p := range s.Settings.Shift.periods() {
		if p.Name == name {
			var h, m int
			fmt.Sscanf(p.Start, "%d:%d", &h, &m)
			return s.PeriodAt(time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, loc).Add(time.Second)), true
		}
	}
	return Period{}, false
}

// Followup is tracked work owed after the period: one per source, carried forward until done.
type TrackedFollowup struct {
	ID       string    `json:"id"`
	Key      string    `json:"key"` // the source: exception:LX3, critical:CR1, request:RQ4…
	Incident string    `json:"incident,omitempty"`
	Title    string    `json:"title"`
	Owner    string    `json:"owner,omitempty"`
	Due      time.Time `json:"due,omitzero"`
	Opened   time.Time `json:"opened"`
	Period   string    `json:"period"` // the period it opened in
	Done     bool      `json:"done,omitempty"`
	DoneAt   time.Time `json:"doneAt,omitzero"`
	History  []ExNote  `json:"history"`
}

type followupData struct {
	Key      string    `json:"key"`
	Incident string    `json:"incident,omitempty"`
	Title    string    `json:"title,omitempty"`
	Owner    string    `json:"owner,omitempty"`
	Due      time.Time `json:"due,omitzero"`
	Done     bool      `json:"done,omitempty"`
	Note     string    `json:"note,omitempty"`
}

// trackFollowup opens or updates the follow-up of a source (in apply: deterministic).
func (s *State) trackFollowup(e ledger.Event, key, incident, title, owner string, due time.Time, done bool) {
	f := s.followupByKey(key)
	if f == nil {
		if done && title == "" {
			return
		}
		s.fuCount++
		f = &TrackedFollowup{ID: fmt.Sprintf("FU%d", s.fuCount), Key: key, Incident: incident, Title: title, Opened: e.At, Period: s.PeriodAt(e.At).ID}
		s.Followups[f.ID] = f
		f.History = append(f.History, ExNote{At: e.At, By: e.Actor, What: "opened", Note: title})
	}
	if title != "" {
		f.Title = title
	}
	if owner != "" && owner != f.Owner {
		f.History = append(f.History, ExNote{At: e.At, By: e.Actor, What: "owner " + owner})
		f.Owner = owner
	}
	if !due.IsZero() {
		f.Due = due
	}
	if done && !f.Done {
		f.Done, f.DoneAt = true, e.At
		f.History = append(f.History, ExNote{At: e.At, By: e.Actor, What: "done"})
	}
}

func (s *State) followupByKey(key string) *TrackedFollowup {
	for _, f := range s.Followups {
		if f.Key == key {
			return f
		}
	}
	return nil
}

// SaveFollowup opens a follow-up by hand, reassigns it or marks it done.
func (e *Engine) SaveFollowup(a Actor, key, incident, title, owner string, due time.Time, done bool, note string) error {
	if err := mayReview(a); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if strings.TrimSpace(key) == "" {
		key = "manual:" + e.newID("M", func(id string) bool { return e.st.followupByKey("manual:"+id) != nil })
		if strings.TrimSpace(title) == "" {
			return problem("title", "say what is owed")
		}
	} else if e.st.followupByKey(key) == nil && strings.TrimSpace(title) == "" {
		return problem("key", "no follow-up %s", key)
	}
	_, err := e.write(a, EvFollowup, refs{Incident: incident}, followupData{Key: key, Incident: incident, Title: strings.TrimSpace(title), Owner: strings.TrimSpace(owner), Due: due, Done: done, Note: strings.TrimSpace(note)})
	return err
}

// ReportItem is one line of a Shift Intelligence report.
type ReportItem struct {
	Key          string    `json:"key"` // the source, for de-duplication and links: incident:26-000001, exception:LX2…
	Section      string    `json:"section"`
	Incident     string    `json:"incident,omitempty"`
	At           time.Time `json:"at"`
	Title        string    `json:"title"`
	Significance string    `json:"significance,omitempty"`
	Status       string    `json:"status"`
	Owner        string    `json:"owner,omitempty"`
	Due          time.Time `json:"due,omitzero"`
	Next         string    `json:"next,omitempty"` // the next action
	Restricted   bool      `json:"restricted,omitempty"`
}

// ShiftReport is a period's report: the draft assembled from the records, or a finalized version.
type ShiftReport struct {
	Period    Period       `json:"period"`
	Version   int          `json:"version"` // 0: a draft
	Addendum  bool         `json:"addendum,omitempty"`
	Items     []ReportItem `json:"items"`
	Summary   string       `json:"summary,omitempty"` // the supervisor's observations, apart from the facts
	Handoff   string       `json:"handoff,omitempty"`
	Note      string       `json:"note,omitempty"` // why an addendum
	By        string       `json:"by,omitempty"`
	At        time.Time    `json:"at,omitzero"`
	Acks      []ShiftAck   `json:"acks,omitempty"`
	Custom    bool         `json:"custom,omitempty"` // run for a range that is not a period
	Generated time.Time    `json:"generated"`
}

// ShiftAck is the incoming supervisor's acknowledgement.
type ShiftAck struct {
	By       string            `json:"by"`
	At       time.Time         `json:"at"`
	Version  int               `json:"version"`
	Accepted []string          `json:"accepted,omitempty"` // follow-up IDs they took
	Reassign map[string]string `json:"reassign,omitempty"` // follow-up ID: new owner
	Note     string            `json:"note,omitempty"`
}

type shiftReportData struct {
	Report ShiftReport `json:"report"`
}

type shiftAckData struct {
	Period string   `json:"period"`
	Ack    ShiftAck `json:"ack"`
}

// ShiftDraft assembles the report for a time range from the records. duty and role decide restricted
// detail: a restricted item shows its existence, not its content, to those not allowed.
func (s *State) ShiftDraft(p Period, now time.Time, duty, role string) ShiftReport {
	r := ShiftReport{Period: p, Generated: now}
	in := func(t time.Time) bool { return !t.Before(p.From) && (p.To.IsZero() || t.Before(p.To)) }
	major := s.Settings.Shift.MajorPriority
	if major <= 0 {
		major = 2
	}
	add := func(it ReportItem) {
		if !slices.ContainsFunc(r.Items, func(x ReportItem) bool { return x.Key == it.Key && x.Section == it.Section }) {
			r.Items = append(r.Items, it)
		}
	}
	for _, x := range s.Incidents {
		if !in(x.Created) && !(x.Status != Closed && x.Created.Before(p.To)) {
			continue
		}
		if x.Priority <= major || (x.Status != Closed && x.Created.Before(p.From)) {
			st := x.Status
			if x.Status == Closed {
				st = "closed: " + x.Disposition
			}
			add(ReportItem{Key: "incident:" + x.Number, Section: "major", Incident: x.Number, At: x.Created, Title: fmt.Sprintf("%s %s P%d at %s", x.Number, x.Type, x.Priority, locLabel(x.Location)),
				Significance: map[bool]string{true: "priority " + fmt.Sprint(x.Priority), false: "still open from before the period"}[x.Priority <= major], Status: st,
				Next: map[bool]string{true: "", false: "follow it to close"}[x.Status == Closed]})
		}
	}
	for _, x := range s.LocExceptions {
		if !in(x.At) && !x.Open() {
			continue
		}
		st := "open: needs the supervisor"
		switch {
		case !x.Open():
			st = "reviewed"
		case x.Ack != nil && (x.Located == nil || x.Followup == nil):
			st = "acknowledged: decisions owed"
		case x.Followup != nil && !x.Followup.Done:
			st = "follow-up: " + x.Followup.Disposition
		}
		it := ReportItem{Key: "exception:" + x.ID, Section: "exceptions", Incident: x.Incident, At: x.At, Title: fmt.Sprintf("%s %s location %q on %s", x.ID, strings.ToUpper(x.Kind), x.Entered, x.Incident),
			Significance: firstNonEmpty(x.Reason, "not an authoritative address point"), Status: st + "; address queue " + x.Queue}
		if x.Followup != nil {
			it.Owner, it.Due = x.Followup.Owner, x.Followup.Due
		}
		if x.Open() {
			it.Next = "acknowledge, answer both questions and close the follow-up"
		}
		add(it)
	}
	for _, c := range s.CriticalCases {
		if !in(c.Opened) && c.Review == ReviewComplete {
			continue
		}
		it := ReportItem{Key: "critical:" + c.ID, Section: "critical", Incident: c.Incident, At: c.Opened, Title: c.ID + " " + c.Summary,
			Significance: strings.Join(c.TriggerNames(), "; "), Status: "full review " + c.Review}
		if c.Ack == nil {
			it.Next = "acknowledge"
		} else if c.Review != ReviewComplete {
			it.Next = "run the full review"
		}
		add(it)
	}
	for _, f := range s.Flags {
		if in(f.Created) || (in(f.Changed) && f.Severity == "critical") || f.State == FlagProposed {
			cat, _ := s.Settings.Flags.Category(f.Category)
			it := ReportItem{Key: "flag:" + f.ID, Section: "flags", Incident: f.Incident, At: f.Changed, Title: fmt.Sprintf("%s %s flag: %s (%s)", f.ID, cat.Name, f.Title, f.Scope.Label()),
				Significance: f.Severity, Status: f.State}
			if f.State == FlagProposed {
				it.Next = "approve or reject"
			}
			if !cat.MayView(duty, role) {
				it.Title, it.Restricted = fmt.Sprintf("%s %s flag (restricted)", f.ID, cat.Name), true
			}
			add(it)
		}
	}
	for _, c := range s.Comms {
		if !in(c.Issued) || (c.Priority != "urgent" && c.Priority != "high" && c.Kind != "bolo") {
			continue
		}
		add(ReportItem{Key: "comm:" + c.Thread, Section: "comms", Incident: c.Incident, At: c.Issued, Title: fmt.Sprintf("%s %s: %s", strings.ToUpper(c.Priority), c.Kind, c.Subject), Status: "version " + fmt.Sprint(c.Version)})
	}
	for _, u := range s.Units {
		if u.Status == OutOfService {
			continue
		}
		if c := s.CapabilityOf(u); c.State != CapFull && c.State != "" {
			add(ReportItem{Key: "unit:" + u.ID, Section: "staffing", At: now, Title: u.ID + " " + c.State + ": " + strings.Join(c.Reasons, "; "), Status: u.Status, Next: "staff the unit or keep it in service with a supervisor's override"})
		}
		if due, ok := u.CheckDue(s.Settings); ok && now.After(due) {
			add(ReportItem{Key: "check:" + u.ID, Section: "checks", At: due, Title: fmt.Sprintf("%s status check overdue since %s", u.ID, due.Local().Format("15:04")), Status: u.Status, Next: "check on the unit"})
		}
	}
	for _, q := range s.Requests {
		if q.Open() || in(q.RequestedAt) && q.Type == "hit-confirmation" {
			add(ReportItem{Key: "request:" + q.ID, Section: "requests", Incident: q.Incident, At: q.RequestedAt, Title: fmt.Sprintf("%s %s %s %s", q.ID, q.Type, q.Category, q.Subject),
				Status: s.Settings.Requests.Label(q.Status), Owner: q.Owner, Due: q.Due, Next: map[bool]string{true: "finish or hand over", false: ""}[q.Open()]})
		}
	}
	for _, f := range s.Followups {
		if f.Done && !in(f.DoneAt) {
			continue
		}
		if !f.Done || in(f.DoneAt) {
			st := "open"
			if f.Done {
				st = "done " + f.DoneAt.Local().Format("Jan 2 15:04")
			}
			if f.Period != p.ID && !f.Done {
				st += " (carried from " + f.Period + ")"
			}
			add(ReportItem{Key: "followup:" + f.ID, Section: "followups", Incident: f.Incident, At: f.Opened, Title: f.ID + " " + f.Title, Status: st, Owner: f.Owner, Due: f.Due,
				Next: map[bool]string{true: "", false: "finish it or reassign it"}[f.Done]})
		}
	}
	keep := s.Settings.Shift.Sections
	if len(keep) > 0 {
		r.Items = slices.DeleteFunc(r.Items, func(it ReportItem) bool { return !slices.Contains(keep, it.Section) })
	}
	order := ReportSections
	if len(keep) > 0 {
		order = keep
	}
	sort.SliceStable(r.Items, func(i, j int) bool {
		a, b := slices.Index(order, r.Items[i].Section), slices.Index(order, r.Items[j].Section)
		if a != b {
			return a < b
		}
		return r.Items[i].At.Before(r.Items[j].At)
	})
	return r
}

// FinalizeShift issues the period's report (version 1), or an addendum (the next version) with why.
func (e *Engine) FinalizeShift(a Actor, periodID, summary, handoff, note string) (int, error) {
	if err := mayReview(a); err != nil {
		return 0, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	p, ok := e.st.PeriodByID(periodID)
	if !ok {
		return 0, problem("period", "no operational period %s", periodID)
	}
	prev := e.st.ShiftReports[p.ID]
	if e.st.Settings.Shift.RequireSummary && strings.TrimSpace(summary) == "" {
		return 0, problem("summary", "write the supervisor's summary")
	}
	r := e.st.ShiftDraft(p, e.now(), DutySupervisor, "supervisor")
	r.Summary, r.Handoff, r.Note = strings.TrimSpace(summary), strings.TrimSpace(handoff), strings.TrimSpace(note)
	r.Version = 1
	if len(prev) > 0 {
		r.Version, r.Addendum = len(prev)+1, true
		if r.Note == "" {
			return 0, problem("note", "the report was issued: say why this addendum")
		}
	}
	_, err := e.write(a, EvShiftReport, refs{}, shiftReportData{Report: r})
	return r.Version, err
}

// AckShift is the incoming supervisor acknowledging a period's report and taking or reassigning the open
// follow-up.
func (e *Engine) AckShift(a Actor, periodID string, accept []string, reassign map[string]string, note string) error {
	if err := mayReview(a); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	reps := e.st.ShiftReports[periodID]
	if len(reps) == 0 {
		return problem("period", "the report for %s is not issued yet", periodID)
	}
	for _, id := range accept {
		if e.st.Followups[id] == nil {
			return problem("accept", "no follow-up %s", id)
		}
	}
	for id, to := range reassign {
		if e.st.Followups[id] == nil {
			return problem("reassign", "no follow-up %s", id)
		}
		if !e.knownUser(to) {
			return problem("reassign", "no user %s", to)
		}
	}
	_, err := e.write(a, EvShiftAck, refs{}, shiftAckData{Period: periodID, Ack: ShiftAck{Version: reps[len(reps)-1].Version, Accepted: accept, Reassign: reassign, Note: strings.TrimSpace(note)}})
	return err
}

func (s *State) applyShift(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvShiftReport:
		d, err := decode[shiftReportData](e)
		if err != nil {
			return true, err
		}
		r := d.Report
		r.By, r.At = e.Actor, e.At
		s.ShiftReports[r.Period.ID] = append(s.ShiftReports[r.Period.ID], &r)
		return true, nil
	case EvShiftAck:
		d, err := decode[shiftAckData](e)
		if err != nil {
			return true, err
		}
		reps := s.ShiftReports[d.Period]
		if len(reps) == 0 {
			return true, nil
		}
		ack := d.Ack
		ack.By, ack.At = e.Actor, e.At
		last := reps[len(reps)-1]
		last.Acks = append(last.Acks, ack)
		for _, id := range ack.Accepted {
			if f := s.Followups[id]; f != nil {
				f.History = append(f.History, ExNote{At: e.At, By: e.Actor, What: "accepted at shift change"})
				f.Owner = e.Actor
			}
		}
		for id, to := range ack.Reassign {
			if f := s.Followups[id]; f != nil {
				f.History = append(f.History, ExNote{At: e.At, By: e.Actor, What: "reassigned to " + to + " at shift change"})
				f.Owner = to
			}
		}
		return true, nil
	case EvFollowup:
		d, err := decode[followupData](e)
		if err != nil {
			return true, err
		}
		s.trackFollowup(e, d.Key, d.Incident, d.Title, d.Owner, d.Due, d.Done)
		if f := s.followupByKey(d.Key); f != nil && d.Note != "" {
			f.History = append(f.History, ExNote{At: e.At, By: e.Actor, What: "note", Note: d.Note})
		}
		return true, nil
	case EvShiftNote:
		d, err := decode[ShiftNote](e)
		if err != nil {
			return true, err
		}
		d.At, d.By = e.At, e.Actor
		s.ShiftNotes = append(s.ShiftNotes, d)
		return true, nil
	}
	return false, nil
}

// ShiftNote is a person's (or supervisor's) correction or context on someone's shift record: kept,
// versioned by time, never changing the events themselves.
type ShiftNote struct {
	Person string    `json:"person"`
	Period string    `json:"period"`
	Text   string    `json:"text"`
	Kind   string    `json:"kind"` // context, correction
	At     time.Time `json:"at"`
	By     string    `json:"by"`
}

// AddShiftNote records a note on a person's shift record: by the person or a supervisor.
func (e *Engine) AddShiftNote(a Actor, n ShiftNote) error {
	if n.Person != a.User {
		if err := mayReview(a); err != nil {
			return dutyProblem("person", "you add notes to your own record; a supervisor adds them to others'")
		}
	}
	if strings.TrimSpace(n.Text) == "" {
		return problem("text", "write the note")
	}
	if n.Kind != "correction" {
		n.Kind = "context"
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := e.write(a, EvShiftNote, refs{}, n)
	return err
}

// PersonShift is one person's operational record for a period: what they were assigned and what they
// handled, for coaching and awareness. It is not a ranking: counts come with the duties, boards and
// critical exposure that explain them, and a possible workload gap is only a prompt for the supervisor.
type PersonShift struct {
	User       string      `json:"user"`
	Name       string      `json:"name,omitempty"`
	Duties     []string    `json:"duties"`
	Consoles   []string    `json:"consoles,omitempty"`
	SignOns    []Stamp     `json:"signOns,omitempty"`
	SignOffs   []Stamp     `json:"signOffs,omitempty"`
	Calls      int         `json:"calls"`                // calls answered
	AnswerSecs float64     `json:"answerSecs,omitempty"` // mean seconds to answer, when known
	CallSecs   float64     `json:"callSecs,omitempty"`   // mean call length
	Created    int         `json:"created"`              // incidents created
	CreateSecs float64     `json:"createSecs,omitempty"` // mean call answered to incident made
	Dispatches int         `json:"dispatches"`
	Statuses   int         `json:"statuses"` // unit status changes made
	Radio      int         `json:"radio"`    // radio-related actions (acknowledgements)
	Queries    int         `json:"queries"`  // NCIC and state requests
	Critical   []string    `json:"critical,omitempty"`
	Breaks     []Span      `json:"breaks,omitempty"` // formal breaks, from break events only
	BreakMins  float64     `json:"breakMins"`
	Locks      []Span      `json:"locks,omitempty"` // console locks: kept apart, never counted as breaks
	Trainee    bool        `json:"trainee,omitempty"`
	Notes      []ShiftNote `json:"notes,omitempty"`
	Prompt     string      `json:"prompt,omitempty"` // a possible workload-sharing gap for the supervisor to look at
}

// Span is a time from and to (To empty: still going).
type Span struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to,omitzero"`
	Why  string    `json:"why,omitempty"`
}

// IndividualShift builds every person's record for a period from the ledger events in it.
func (s *State) IndividualShift(p Period, events []ledger.Event) []PersonShift {
	by := map[string]*PersonShift{}
	get := func(u string) *PersonShift {
		if u == "" || u == "system" || u == "bitvoice" || u == "ali" || u == "radio" {
			return nil
		}
		x := by[u]
		if x == nil {
			x = &PersonShift{User: u}
			if ui := s.Users[u]; ui != nil {
				x.Name = ui.Name
			}
			x.Trainee = s.StaffOf(u).Trainee
			by[u] = x
		}
		return x
	}
	sums := map[string]*[3]float64{} // user: answer total, length total, create total
	openBreak, openLock := map[string]int{}, map[string]int{}
	for _, e := range events {
		x := get(e.Actor)
		if x == nil {
			continue
		}
		if e.Duty != "" && !slices.Contains(x.Duties, e.Duty) {
			x.Duties = append(x.Duties, e.Duty)
		}
		if e.Console != "" && !slices.Contains(x.Consoles, e.Console) {
			x.Consoles = append(x.Consoles, e.Console)
		}
		acc := sums[e.Actor]
		if acc == nil {
			acc = &[3]float64{}
			sums[e.Actor] = acc
		}
		switch e.Type {
		case EvSignIn:
			x.SignOns = append(x.SignOns, Stamp{By: e.Actor, At: e.At})
		case EvSignOut:
			x.SignOffs = append(x.SignOffs, Stamp{By: e.Actor, At: e.At})
		case EvIncCreated:
			x.Created++
			if in := s.Incidents[e.Incident]; in != nil {
				for _, id := range in.Calls {
					if c := s.Calls[id]; c != nil && !c.Answer.IsZero() && e.At.After(c.Answer) {
						acc[2] += e.At.Sub(c.Answer).Seconds()
						break
					}
				}
			}
		case EvIncDispatch:
			x.Dispatches++
		case EvUnitStatus:
			x.Statuses++
		case EvUnitAcked, EvEmergencyAck:
			x.Radio++
		case EvRequestLogged, EvJusticeQuery:
			x.Queries++
		case EvBreak:
			v, _ := decode[breakData](e)
			if v.On {
				x.Breaks = append(x.Breaks, Span{From: e.At, Why: "break"})
				openBreak[e.Actor] = len(x.Breaks)
			} else if i := openBreak[e.Actor]; i > 0 {
				x.Breaks[i-1].To = e.At
				x.BreakMins += e.At.Sub(x.Breaks[i-1].From).Minutes()
				openBreak[e.Actor] = 0
			}
		case EvLocked:
			v, _ := decode[map[string]string](e)
			x.Locks = append(x.Locks, Span{From: e.At, Why: v["how"]})
			openLock[e.Actor] = len(x.Locks)
		case EvUnlocked:
			if i := openLock[e.Actor]; i > 0 {
				x.Locks[i-1].To = e.At
				openLock[e.Actor] = 0
			}
		}
	}
	// Calls answered: by the call's answering person when the call says.
	for _, c := range s.Calls {
		if c.Answer.IsZero() || c.Answer.Before(p.From) || (!p.To.IsZero() && !c.Answer.Before(p.To)) {
			continue
		}
		u := c.Agent
		if u == "" {
			continue
		}
		x := get(u)
		if x == nil {
			continue
		}
		x.Calls++
		acc := sums[u]
		if acc == nil {
			acc = &[3]float64{}
			sums[u] = acc
		}
		if !c.Start.IsZero() {
			acc[0] += c.Answer.Sub(c.Start).Seconds()
		}
		if !c.End.IsZero() {
			acc[1] += c.End.Sub(c.Answer).Seconds()
		}
	}
	for _, c := range s.CriticalCases {
		for _, r := range c.Roles {
			if x := by[r.User]; x != nil && !slices.Contains(x.Critical, c.Incident+" "+r.Role) {
				x.Critical = append(x.Critical, c.Incident+" "+r.Role)
			}
		}
	}
	for _, n := range s.ShiftNotes {
		if n.Period == p.ID {
			if x := by[n.Person]; x != nil {
				x.Notes = append(x.Notes, n)
			}
		}
	}
	out := make([]PersonShift, 0, len(by))
	for u, x := range by {
		if acc := sums[u]; acc != nil {
			if x.Calls > 0 {
				x.AnswerSecs, x.CallSecs = acc[0]/float64(x.Calls), acc[1]/float64(x.Calls)
			}
			if x.Created > 0 {
				x.CreateSecs = acc[2] / float64(x.Created)
			}
		}
		out = append(out, *x)
	}
	// A possible workload gap: someone doing much more than the median of the same duty. A prompt, with
	// what explains it, never a score.
	ratio := s.Settings.Shift.WorkloadRatio
	if ratio <= 0 {
		ratio = 2
	}
	byDuty := map[string][]int{}
	work := func(x PersonShift) int { return x.Calls + x.Created + x.Dispatches + x.Statuses/2 }
	for _, x := range out {
		if len(x.Duties) > 0 {
			byDuty[x.Duties[0]] = append(byDuty[x.Duties[0]], work(x))
		}
	}
	for i, x := range out {
		if len(x.Duties) == 0 {
			continue
		}
		ws := slices.Clone(byDuty[x.Duties[0]])
		if len(ws) < 2 {
			continue
		}
		slices.Sort(ws)
		med := ws[len(ws)/2]
		if med > 0 && float64(work(x)) >= ratio*float64(med) {
			ctx := []string{fmt.Sprintf("%d actions against a median of %d for %s", work(x), med, x.Duties[0])}
			if len(x.Critical) > 0 {
				ctx = append(ctx, fmt.Sprintf("%d critical incidents", len(x.Critical)))
			}
			if len(x.Consoles) > 0 {
				ctx = append(ctx, "positions "+strings.Join(x.Consoles, ", "))
			}
			out[i].Prompt = "Possible workload-sharing gap to review: " + strings.Join(ctx, "; ") + ". Check board assignments, training and relief before drawing any conclusion."
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].User < out[j].User })
	return out
}
