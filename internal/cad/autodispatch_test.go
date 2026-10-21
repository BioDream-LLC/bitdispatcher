package cad

import (
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

func TestAutoDispatch(t *testing.T) {
	e, _, _ := testEngine(t)
	var ct CallType
	e.Read(func(s *State) { ct = *s.CallTypes["SICK"] })
	hi := ct
	hi.Priority, hi.AutoDispatch = 1, true
	if err := e.SaveCallType(dispatcher, hi); err == nil {
		t.Fatal("a priority 1 type was set to auto-dispatch")
	}
	ct.Priority, ct.AutoDispatch = 3, true
	if err := e.SaveCallType(dispatcher, ct); err != nil {
		t.Fatal(err)
	}
	loc := ng911.Point(40.2701, -76.8801)
	num, err := e.CreateIncident(dispatcher, NewIncident{Type: "SICK", Location: loc, LocVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	var at time.Time
	e.Read(func(s *State) { at = s.Incidents[num].Created })
	if due := e.AutoDispatchDue(at.Add(10 * time.Second)); len(due) != 0 {
		t.Fatalf("due before the wait: %v", due)
	}
	due := e.AutoDispatchDue(at.Add(2 * time.Minute))
	if len(due) != 1 || due[0] != num {
		t.Fatalf("due %v", due)
	}
	sent, why, err := e.AutoDispatch(num)
	if err != nil || len(sent) == 0 {
		t.Fatalf("sent %v why %q err %v", sent, why, err)
	}
	e.Read(func(s *State) {
		in := s.Incidents[num]
		if len(in.Units) == 0 || in.Units[0].Unit != sent[0] {
			t.Fatalf("%+v", in.Units)
		}
		if !strings.Contains(in.Timeline[len(in.Timeline)-2].Text+in.Timeline[len(in.Timeline)-1].Text, "sent automatically") {
			t.Fatalf("%+v", in.Timeline)
		}
	})
	if due := e.AutoDispatchDue(at.Add(time.Hour)); len(due) != 0 {
		t.Fatalf("due again: %v", due)
	}
	// Without a verified location it is left for a person, once.
	n2, _ := e.CreateIncident(dispatcher, NewIncident{Type: "SICK", Location: ng911.Point(40.2702, -76.8802)})
	if sent, why, _ := e.AutoDispatch(n2); len(sent) != 0 || why == "" {
		t.Fatalf("sent %v why %q", sent, why)
	}
	if due := e.AutoDispatchDue(at.Add(time.Hour)); len(due) != 0 {
		t.Fatalf("left one is due again: %v", due)
	}
}
