package advisory

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

var d1 = cad.Actor{User: "d1", Role: "dispatcher", Console: "D1"}

func engine(t *testing.T) *cad.Engine {
	t.Helper()
	e, err := cad.Open(filepath.Join(t.TempDir(), "l.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	e.SaveStation(cad.SystemActor, cad.Station{ID: "ST1", Name: "Station 1", Lat: 37.27, Lon: -79.94})
	for _, id := range []string{"E1", "E2"} {
		e.SaveUnit(cad.SystemActor, cad.UnitDef{ID: id, Type: "engine", Station: "ST1"})
		e.SetStatus(cad.SystemActor, id, cad.Available, "", "")
	}
	return e
}

func at(lat, lon float64, addr string) *ng911.Location {
	return &ng911.Location{Lat: &lat, Lon: &lon, Address: addr}
}

func eval(e *cad.Engine, c Codebook, later time.Duration) []Suggestion {
	var out []Suggestion
	e.Read(func(s *cad.State) { out = Evaluate(c, s, e.Now().Add(later)) })
	return out
}

func find(list []Suggestion, rule, inc string) *Suggestion {
	for i := range list {
		if list[i].Rule == rule && list[i].Incident == inc {
			return &list[i]
		}
	}
	return nil
}

func TestWordsAndPrecedence(t *testing.T) {
	e := engine(t)
	c := Default()
	fa, _ := e.CreateIncident(d1, cad.NewIncident{Type: "FA", Location: at(37.27, -79.94, "1 Main St")})
	if find(eval(e, c, 0), "fire-words", fa) != nil {
		t.Fatal("a fire alarm with nothing said is not a fire")
	}
	// The dispatcher writes what the caller says.
	e.AddNote(d1, fa, "Caller sees smoke from the second floor", false)
	sg := find(eval(e, c, 0), "fire-words", fa)
	if sg == nil || sg.Verdict != "suggest" || sg.Action == nil || sg.Action.Command != "T SF #"+fa {
		t.Fatalf("smoke on a fire alarm: %+v", sg)
	}
	if len(sg.Relied) != 1 || !strings.Contains(sg.Relied[0].Text, "smoke") || sg.Relied[0].Source != SrcDispatcher || len(sg.Absent) == 0 {
		t.Fatalf("evidence %+v absent %v", sg.Relied, sg.Absent)
	}
	// Another dispatcher note denies it: same rank, so the codebook cannot settle it.
	e.AddNote(d1, fa, "Second caller says no smoke", false)
	sg = find(eval(e, c, 0), "fire-words", fa)
	if sg == nil || sg.Verdict != "cannot-determine" {
		t.Fatalf("contradiction: %+v", sg)
	}
	// The crew on scene says there is none: a unit outweighs the dispatcher's notes.
	e.AddNote(cad.Actor{User: "crew", Role: "field", Console: "E1"}, fa, "E1: no smoke showing, alarm panel fault", false)
	if sg = find(eval(e, c, 0), "fire-words", fa); sg != nil {
		t.Fatalf("the unit's denial did not settle it: %+v", sg)
	}
	// Once typed as a structure fire, the words fit.
	sf, _ := e.CreateIncident(d1, cad.NewIncident{Type: "SF", Location: at(37.30, -79.90, "9 Oak St")})
	e.AddNote(d1, sf, "flames through the roof", false)
	if find(eval(e, c, 0), "fire-words", sf) != nil {
		t.Fatal("a structure fire with flames is not a conflict")
	}
}

func TestDuplicatesPendingQuietRepeat(t *testing.T) {
	e := engine(t)
	c := Default()
	a, _ := e.CreateIncident(d1, cad.NewIncident{Type: "MVA", Location: at(37.2700, -79.9400, "Main & 1st")})
	b, _ := e.CreateIncident(d1, cad.NewIncident{Type: "MVA", Location: at(37.2705, -79.9402, "Main St")})
	far, _ := e.CreateIncident(d1, cad.NewIncident{Type: "MVA", Location: at(37.30, -79.90, "Far Rd")})
	list := eval(e, c, 0)
	dup := find(list, "dup", b)
	if dup == nil || dup.Other != a || dup.Severity != "urgent" {
		t.Fatalf("duplicate: %+v", list)
	}
	if find(list, "dup", far) != nil {
		t.Fatal("a far incident flagged as a duplicate")
	}
	// Pending: not yet after 30 s, yes after two minutes, with REC as the action.
	if find(eval(e, c, 30*time.Second), "pending-p1", a) != nil {
		t.Fatal("pending too soon")
	}
	p := find(eval(e, c, 2*time.Minute), "pending-p1", a)
	if p == nil || p.Action.Command != "REC #"+a {
		t.Fatalf("pending: %+v", p)
	}
	// Quiet: E1 on scene and silent for 25 minutes; silence is listed as assumed absent, never as all well.
	e.Dispatch(d1, a, []string{"E1"}, nil)
	e.SetStatus(d1, "E1", cad.OnScene, "", "")
	q := eval(e, c, 25*time.Minute)
	var quiet *Suggestion
	for i := range q {
		if q[i].Rule == "quiet" && q[i].Unit == "E1" {
			quiet = &q[i]
		}
	}
	if quiet == nil || len(quiet.Absent) == 0 {
		t.Fatalf("quiet: %+v", q)
	}
	// Same state, same answer, same ids.
	x, y := eval(e, c, time.Minute), eval(e, c, time.Minute)
	bx, _ := json.Marshal(x)
	by, _ := json.Marshal(y)
	if string(bx) != string(by) {
		t.Fatal("not deterministic")
	}
	// Repeat address: two earlier incidents at the same place.
	for range 2 {
		n, _ := e.CreateIncident(d1, cad.NewIncident{Type: "WELF", Location: at(37.25, -79.95, "5 Elm St")})
		e.CloseIncident(d1, n, "Handled", "")
	}
	now, _ := e.CreateIncident(d1, cad.NewIncident{Type: "WELF", Location: at(37.25, -79.95, "5 Elm St")})
	if r := find(eval(e, c, 0), "repeat", now); r == nil || len(r.Relied) != 2 {
		t.Fatalf("repeat: %+v", r)
	}
	// A rule switched off says nothing.
	for i := range c.Rules {
		c.Rules[i].Enabled = false
	}
	if len(eval(e, c, time.Hour)) != 0 {
		t.Fatal("disabled rules spoke")
	}
}

func TestServiceShadowDecideAndTest(t *testing.T) {
	e := engine(t)
	s := New(e, nil)
	a, _ := e.CreateIncident(d1, cad.NewIncident{Type: "MVA", Location: at(37.27, -79.94, "Main")})
	e.CreateIncident(d1, cad.NewIncident{Type: "MVA", Location: at(37.2701, -79.9401, "Main")})
	s.step()
	// Shadow is the starting mode: dispatchers see nothing, supervisors see it.
	if len(s.Visible(false)) != 0 || len(s.Visible(true)) == 0 {
		t.Fatalf("shadow: %d %d", len(s.Visible(false)), len(s.Visible(true)))
	}
	raw, _ := json.Marshal(Default())
	if err := e.SaveCodebook(d1, cad.CodebookData{Mode: "live", Rules: raw, Note: "go live"}); err != nil {
		t.Fatal(err)
	}
	s.step()
	vis := s.Visible(false)
	if len(vis) == 0 {
		t.Fatal("live shows nothing")
	}
	sg := vis[0]
	if _, _, err := s.Decide(d1, sg.ID, false, false, "different crashes"); err != nil {
		t.Fatal(err)
	}
	for _, v := range s.Visible(false) {
		if v.ID == sg.ID {
			t.Fatal("rejected suggestion still shown")
		}
	}
	// A restart keeps the decision.
	s2 := New(e, nil)
	s2.step()
	for _, v := range s2.Visible(false) {
		if v.ID == sg.ID {
			t.Fatal("rejected suggestion back after a restart")
		}
	}
	// Testing the codebook over the recorded ledger finds the duplicate.
	r, err := s.Test(Default(), time.Time{}, e.Now().Add(time.Hour))
	if err != nil || r.ByRule["dup"] != 1 {
		t.Fatalf("test %+v %v", r, err)
	}
	_ = a
	if _, err := Parse([]byte(`{"rules":[{"id":"x","kind":"words","severity":"urgent"}]}`)); err == nil {
		t.Fatal("a words rule without words accepted")
	}
}

func TestScriptRules(t *testing.T) {
	e := engine(t)
	s := New(e, nil)
	inc, _ := e.CreateIncident(d1, cad.NewIncident{Type: "FALL", Location: at(37.27, -79.94, "1 Main St")})
	e.AddNote(d1, inc, "Caller says she is not breathing", false)
	c := Codebook{Precedence: Default().Precedence, Rules: []Rule{
		{ID: "js-breathing", Kind: KindScript, Name: "Not breathing", Enabled: true, Severity: "urgent", Lang: "javascript", Hook: "incident",
			Source: `function advise(i) { if (i.notes.some(n => /not breathing/i.test(n)) && i.type !== "CARD") return {title: i.number + ": not breathing", action: {label: "Cardiac arrest", command: "T CARD"}} }`},
		{ID: "lua-old", Kind: KindScript, Name: "Long wait", Enabled: true, Severity: "info", Lang: "lua", Hook: "incident",
			Source: `function advise(i) if i.minutes > 30 then return {title = "waiting long"} end end`},
		{ID: "js-broken", Kind: KindScript, Name: "Broken", Enabled: true, Severity: "info", Lang: "javascript", Hook: "incident", Source: `function advise(i) { return i.nothing.here }`},
		{ID: "js-prefer", Kind: KindScript, Name: "Prefer E2", Enabled: true, Severity: "info", Lang: "javascript", Hook: "recommend", Source: `function recommend(r) { return {prefer: ["E2"], notes: ["E2 carries the ladder kit"]} }`},
	}}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	bad := Codebook{Rules: []Rule{{ID: "x", Kind: KindScript, Severity: "info", Lang: "lua", Hook: "incident", Source: "function advise(i"}}}
	if err := bad.Check(); err == nil {
		t.Fatal("a script that does not compile was accepted")
	}
	raw, _ := json.Marshal(c)
	if err := e.SaveCodebook(d1, cad.CodebookData{Mode: "live", Rules: raw}); err != nil {
		t.Fatal(err)
	}
	s.step()
	vis := s.Visible(false)
	sg := find(vis, "js-breathing", inc)
	if sg == nil || sg.Action == nil || sg.Action.Command != "T CARD #"+inc || sg.Severity != "urgent" {
		t.Fatalf("script suggestion: %+v", vis)
	}
	if find(vis, "lua-old", inc) != nil {
		t.Fatal("the Lua script fired too early")
	}
	// A failing script is reported and the rest carry on.
	st := s.Status()
	if len(st.Faults) != 1 || st.Faults[0].Rule != "js-broken" || st.Degraded {
		t.Fatalf("faults %+v", st)
	}
	// Accepting runs the script's command, as the person.
	if _, res, err := s.Decide(d1, sg.ID, true, true, ""); err != nil || !res.OK {
		t.Fatalf("accept: %+v %v", res, err)
	}
	e.Read(func(st *cad.State) {
		if st.Incidents[inc].Type != "CARD" {
			t.Fatalf("type %s", st.Incidents[inc].Type)
		}
	})
	adv := s.AdviseRecommendation(t.Context(), RecommendInput{Incident: ScriptInput{Number: inc}})
	if len(adv.Prefer) != 1 || adv.Prefer[0] != "E2" || len(adv.Notes) != 1 || adv.Notes[0] != "Prefer E2: E2 carries the ladder kit" {
		t.Fatalf("recommend %+v", adv)
	}
	// Replaying the ledger runs scripts too.
	r, err := s.Test(c, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil || r.ByRule["js-breathing"] == 0 || len(r.Faults) == 0 {
		t.Fatalf("test: %+v %v", r, err)
	}
}

// A call near an incident is suggested for it, unless the call queue already groups it with a call on
// that incident (and offers to attach it there).
func TestCallNearNotTwice(t *testing.T) {
	e := engine(t)
	c := Default()
	sys := cad.SystemActor
	now := e.Now()
	inc, _ := e.CreateIncident(d1, cad.NewIncident{Type: "SF", Location: at(37.2700, -79.9400, "1 Main St")})
	// A lone caller 100 m away: suggested.
	e.UpsertCall(sys, "lone", cad.CallData{Source: "bitvoice", State: "answered", Callback: "+15405550001", Start: now, Location: at(37.2709, -79.9400, "")})
	if find(eval(e, c, 0), "call-near", inc) == nil {
		t.Fatal("a lone call near the incident was not suggested")
	}
	// The first caller is attached to the incident; the second calls from beside them: the queue groups
	// them and offers the attachment, so the advisory says nothing.
	e.UpsertCall(sys, "first", cad.CallData{Source: "bitvoice", State: "answered", Callback: "+15405550002", Start: now, Location: at(37.2708, -79.9401, "")})
	e.AttachCall(d1, "first", inc)
	if find(eval(e, c, 0), "call-near", inc) != nil {
		t.Fatal("the call the queue groups with an attached call was suggested a second time")
	}
}

// A silent 911 call on no incident is raised; once the caller says anything, or it is on an incident, it is not.
func TestSilentLine(t *testing.T) {
	e := engine(t)
	c := Default()
	e.UpsertCall(cad.BitVoiceActor, "s1", cad.CallData{Source: "bitvoice", State: "answered", Callback: "+15405550100", Emergency: true, Location: at(37.27, -79.94, "1 Main St")})
	e.UpsertCall(cad.BitVoiceActor, "s2", cad.CallData{Source: "bitvoice", State: "ringing", Callback: "+15405550101", Emergency: true})
	e.UpsertCall(cad.BitVoiceActor, "s3", cad.CallData{Source: "bitvoice", State: "ringing", Callback: "+15405550102", Emergency: true})
	e.EndCall(cad.BitVoiceActor, "s3") // a hang-up before answer
	got := map[string]*Suggestion{}
	for _, s := range eval(e, c, time.Minute) {
		if s.Rule == "silent" {
			got[s.Call] = &s
		}
	}
	if got["s1"] == nil || got["s3"] == nil || got["s2"] != nil {
		t.Fatalf("silent calls: %v", got)
	}
	if !strings.Contains(got["s3"].Detail, "hung up before it was answered") || len(got["s1"].Absent) == 0 {
		t.Fatalf("%+v", got["s3"])
	}
	e.UpsertCall(cad.BitVoiceActor, "s1", cad.CallData{Source: "bitvoice", Transcript: []cad.TranscriptLine{{Speaker: "caller", Text: "help"}}})
	for _, s := range eval(e, c, time.Minute) {
		if s.Rule == "silent" && s.Call == "s1" {
			t.Fatal("a caller who spoke is not silent")
		}
	}
	if len(eval(e, c, time.Hour)) != 0 {
		for _, s := range eval(e, c, time.Hour) {
			if s.Rule == "silent" {
				t.Fatal("silent calls are raised for 30 minutes only")
			}
		}
	}
}

// An alarm that says fire is a machine; a caller who sees no smoke outweighs it.
func TestMachineAgainstCaller(t *testing.T) {
	e := engine(t)
	c := Default()
	n, _ := e.CreateIncident(d1, cad.NewIncident{Type: "FA", Location: at(37.27, -79.94, "1 Main St")})
	al := cad.Alert{Kind: cad.AlertAlarm, Sender: "Valley Alarm", ID: "A1", Event: "Fire", Detail: "smoke detector zone 3", Status: "new"}
	e.UpsertCall(cad.BitVoiceActor, "a1", cad.CallData{Source: "alarm", State: "data", Alert: &al, Notes: []string{"smoke detector zone 3"}})
	e.AttachCall(d1, "a1", n)
	sg := find(eval(e, c, 0), "fire-words", n)
	if sg == nil || sg.Relied[0].Source != SrcMachine {
		t.Fatalf("the alarm's smoke: %+v", sg)
	}
	e.UpsertCall(cad.BitVoiceActor, "v1", cad.CallData{Source: "bitvoice", State: "answered", Emergency: true,
		Transcript: []cad.TranscriptLine{{Speaker: "caller", Text: "the alarm is going off but there is no smoke at all"}}})
	e.AttachCall(d1, "v1", n)
	if sg = find(eval(e, c, 0), "fire-words", n); sg != nil {
		t.Fatalf("the caller's no smoke outweighs the alarm: %+v", sg)
	}
}
