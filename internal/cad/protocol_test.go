package cad

import (
	"strings"
	"testing"

	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

func TestProtocols(t *testing.T) {
	e, _, path := testEngine(t)
	p := ExampleProtocol()
	bad := p
	bad.Questions = append([]Question(nil), p.Questions...)
	bad.Questions[1] = Question{ID: "q2", Text: "loop?", Answers: []Answer{{Label: "Yes", Next: "q1"}}}
	if err := e.SaveProtocol(dispatcher, bad); err == nil || !strings.Contains(err.Error(), "loop") {
		t.Fatalf("a looping protocol was taken: %v", err)
	}
	if err := e.SaveProtocol(dispatcher, p); err != nil {
		t.Fatal(err)
	}
	num, err := e.CreateIncident(dispatcher, NewIncident{Type: "SICK", Location: ng911.Point(40.27, -76.88)})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.AnswerProtocol(dispatcher, num, p.ID, "q1", "Yes"); err == nil {
		t.Fatal("an unapproved protocol was used")
	}
	p.Approved = true
	if err := e.SaveProtocol(dispatcher, p); err != nil {
		t.Fatal(err)
	}
	if err := e.AnswerProtocol(dispatcher, num, p.ID, "q2", "No"); err == nil {
		t.Fatal("answered out of order")
	}
	for _, qa := range [][2]string{{"q1", "Yes"}, {"q2", "No"}} {
		if err := e.AnswerProtocol(dispatcher, num, p.ID, qa[0], qa[1]); err != nil {
			t.Fatal(err)
		}
	}
	check := func(st *State, wantPri int, wantCode string) {
		t.Helper()
		in := st.Incidents[num]
		if in.Protocol == nil || in.Protocol.Determinant == nil || in.Protocol.Determinant.Code != wantCode || in.Priority != wantPri {
			t.Fatalf("run %+v priority %d", in.Protocol, in.Priority)
		}
	}
	e.Read(func(st *State) { check(st, 2, "EX-2") })
	// Protocol software reports its own determinant.
	if err := e.SetDeterminant(Actor{User: "proqa"}, num, "ProQA", Determinant{Code: "10-D-1", Text: "Chest pain, not alert", Priority: 1}); err != nil {
		t.Fatal(err)
	}
	e.Read(func(st *State) {
		check(st, 1, "10-D-1")
		if st.Incidents[num].Protocol.System != "ProQA" {
			t.Fatal("system not kept")
		}
	})
	e.Close()
	e2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()
	e2.Read(func(st *State) { check(st, 1, "10-D-1") })
}
