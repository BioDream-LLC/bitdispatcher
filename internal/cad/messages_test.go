package cad

import (
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// Messages reach the units they are addressed to, are kept in the ledger, and come back the same after
// the state is rebuilt; crews write to dispatch, not to everyone.
func TestMessages(t *testing.T) {
	e, _, path := testEngine(t)
	num, err := e.CreateIncident(dispatcher, NewIncident{Type: "SF", Location: ng911.Point(40.27, -76.88)})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Dispatch(dispatcher, num, []string{"E1", "M1"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SendMessage(dispatcher, "", []string{"ALL-FIRE"}, "", "Hydrant out at Main and 2nd", false); err != nil {
		t.Fatal(err)
	}
	// To the incident: the units on it.
	id, err := e.SendMessage(dispatcher, "", nil, num, "Caller says the back door is open", true)
	if err != nil {
		t.Fatal(err)
	}
	crew := Actor{User: "e1crew", Role: "field", Duty: DutyField}
	if _, err := e.SendMessage(crew, "E1", []string{"ALL"}, "", "hello", false); err == nil {
		t.Fatal("a crew wrote to everyone")
	}
	if _, err := e.SendMessage(crew, "E1", []string{ToDispatch}, num, "Need PD for traffic", false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SendMessage(dispatcher, "", []string{"X9"}, "", "who", false); err == nil {
		t.Fatal("a message to no such unit")
	}
	if err := e.ReadMessage(crew, id, "E1"); err != nil {
		t.Fatal(err)
	}
	check := func(st *State) {
		t.Helper()
		e1 := st.MessagesFor("E1", time.Time{})
		m1 := st.MessagesFor("M1", time.Time{})
		if len(e1) != 3 || len(m1) != 1 {
			t.Fatalf("E1 has %d, M1 %d messages", len(e1), len(m1))
		}
		if m := st.message(id); m == nil || !m.Urgent || m.Read["E1"].IsZero() || m.Incident != num {
			t.Fatalf("message %+v", m)
		}
		var line string
		for _, x := range st.Incidents[num].Timeline {
			if x.Kind == "message" {
				line += x.Text + "|"
			}
		}
		if !strings.Contains(line, "back door") || !strings.Contains(line, "E1 → DISPATCH: Need PD") {
			t.Fatalf("timeline %q", line)
		}
	}
	e.Read(check)
	e.Close()
	e2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()
	e2.Read(check)
	// The next message numbers on from the rebuilt state.
	if id2, _ := e2.SendMessage(dispatcher, "", []string{"E1"}, "", "again", false); id2 != "M4" {
		t.Fatalf("next id %s", id2)
	}
}

func TestMessageCommand(t *testing.T) {
	e, _, _ := testEngine(t)
	for _, tc := range []struct{ line, want string }{
		{"MSG E1 M1 / Stage at the north lot", "E1,M1"},
		{"MSG E1 Stage at the north lot", "E1"},
		{"MSG ALL-EMS ! Hospital diversion: Carilion", "ALL-EMS"},
	} {
		r := e.Execute(dispatcher, tc.line, "")
		if !r.OK {
			t.Fatalf("%s: %s", tc.line, r.Message)
		}
		var m *Message
		e.Read(func(st *State) { m = st.Messages[len(st.Messages)-1] })
		if strings.Join(m.To, ",") != tc.want || strings.Contains(m.Text, "E1") || m.Text == "" {
			t.Fatalf("%s: %+v", tc.line, m)
		}
		if strings.Contains(tc.line, "!") != m.Urgent {
			t.Fatalf("%s: urgent %v", tc.line, m.Urgent)
		}
	}
}

func TestSelfAttach(t *testing.T) {
	e, _, _ := testEngine(t)
	num, err := e.CreateIncident(dispatcher, NewIncident{Type: "SF", Location: ng911.Point(40.27, -76.88)})
	if err != nil {
		t.Fatal(err)
	}
	crew := Actor{User: "e1crew", Role: "field", Duty: DutyField}
	if err := e.SelfAttach(crew, "E1", num); err == nil || !strings.Contains(err.Error(), "Settings") {
		t.Fatalf("attached without the agency allowing it: %v", err)
	}
	var st Settings
	e.Read(func(s *State) { st = s.Settings })
	st.Duties.SelfAttach = true
	if err := e.SaveSettings(SystemActor, st); err != nil {
		t.Fatal(err)
	}
	if err := e.SelfAttach(crew, "E1", num); err != nil {
		t.Fatal(err)
	}
	e.Read(func(s *State) {
		if s.Units["E1"].Incident != num || s.Units["E1"].Status != Dispatched {
			t.Fatalf("E1 %+v", s.Units["E1"])
		}
	})
	if err := e.SelfAttach(crew, "E1", num); err == nil {
		t.Fatal("attached twice")
	}
}
