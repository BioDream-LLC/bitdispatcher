package cad

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCommunications(t *testing.T) {
	e, c, path := testEngine(t)
	must(t, e.SaveAgency(SystemActor, Agency{Code: "RPD", Name: "City police", Disciplines: []string{"police"}}))
	must(t, e.SaveAgency(SystemActor, Agency{Code: "RCPD", Name: "County police", Disciplines: []string{"police"}}))
	must(t, e.SaveUnit(SystemActor, UnitDef{ID: "2A11", Type: "patrol", Agency: "RPD"}))
	must(t, e.SaveUnit(SystemActor, UnitDef{ID: "C21", Type: "patrol", Agency: "RCPD"}))
	city := Actor{User: "pat", Role: "dispatcher", Duty: "police", Console: "P1", Agency: "RPD"}
	num, err := e.CreateIncident(city, NewIncident{Type: "ROB", Location: at(40.27, -76.88, "5 Main St")})
	if err != nil {
		must(t, e.SaveCallType(SystemActor, CallType{Code: "ROB", Name: "Robbery", Priority: 1, Discipline: "police"}))
		num, err = e.CreateIncident(city, NewIncident{Type: "ROB", Location: at(40.27, -76.88, "5 Main St")})
		must(t, err)
	}
	must(t, e.AddCommFile(city, CommFile{ID: "f1", Name: "suspect.jpg", Type: "image/jpeg", Size: 1200, SHA256: "ab"}))
	// A BOLO: to the owning agency's positions and terminals by default.
	id, err := e.SendComm(city, Comm{Kind: CommBOLO, Subject: "Robbery suspect vehicle", Body: "Silver Honda Civic, ABC1234 VA, male in red hoodie, armed",
		Plate: "abc1234", PlateState: "VA", Vehicle: "silver Honda Civic", Person: "male, red hoodie", Incident: num, Files: []string{"f1"}})
	must(t, err)
	var b Comm
	e.Read(func(s *State) { b = *s.CommByID(id) })
	if b.Priority != "high" || !slices.Equal(b.To, []string{"@agency:RPD"}) || b.Expires.IsZero() || b.FromAgency != "RPD" {
		t.Fatalf("bolo %+v", b)
	}
	cityCar := Reader{User: "u2a11", Role: "field", Unit: "2A11", Agencies: []string{"RPD"}}
	countyCar := Reader{User: "uc21", Role: "field", Unit: "C21", Agencies: []string{"RCPD"}}
	cityDesk := Reader{User: "pat2", Role: "dispatcher", Duty: "police", Console: "P2", Agencies: []string{"RPD"}}
	e.Read(func(s *State) {
		if len(s.Inbox(cityCar, c.t)) != 1 || len(s.Inbox(cityDesk, c.t)) != 1 || len(s.Inbox(countyCar, c.t)) != 0 {
			t.Fatal("the BOLO went to the wrong agency")
		}
		if s.MaySeeComm(&b, countyCar) {
			t.Fatal("a county car may see the city's BOLO")
		}
		in := s.Incidents[num]
		last := in.Timeline[len(in.Timeline)-1]
		if last.Kind != "comm" || !strings.Contains(last.Text, "suspect.jpg") || !strings.Contains(last.Text, "[HIGH]") {
			t.Fatalf("timeline %+v", last)
		}
		if bs := s.ActiveBOLOs(c.t); len(bs) != 1 || bs[0].Plate != "ABC1234" {
			t.Fatalf("plate check BOLO %+v", bs)
		}
	})
	// Opened and acknowledged by person and duty.
	must(t, e.CommReceipt(city, id, "acked", cityDesk))
	if err := e.CommReceipt(city, id, "opened", countyCar); err == nil {
		t.Fatal("a county car acknowledged a city BOLO")
	}
	// Update: a new version; the first is kept as it was.
	c.t = c.t.Add(time.Hour)
	if _, err := e.SendComm(city, Comm{Of: id, Body: "Vehicle seen eastbound on I-81"}); err == nil {
		t.Fatal("an update without a reason")
	}
	id2, err := e.SendComm(city, Comm{Of: id, Body: "Silver Honda Civic ABC1234 VA seen eastbound on I-81; armed", Priority: "urgent", Why: "new sighting"})
	must(t, err)
	if _, err := e.SendComm(city, Comm{Of: id, Body: "x", Why: "y"}); err == nil {
		t.Fatal("updated a version already updated")
	}
	id3, err := e.SendComm(city, Comm{Of: id2, Cancelled: true, Why: "vehicle stopped, suspect in custody"})
	must(t, err)
	e.Read(func(s *State) {
		vs := s.CommThread(id)
		if len(vs) != 3 || vs[0].Body != b.Body || vs[0].Priority != "high" || vs[1].Priority != "urgent" || vs[0].State(c.t) != "updated" || vs[2].State(c.t) != "cancelled" {
			t.Fatalf("versions %d %s %s", len(vs), vs[0].State(c.t), vs[2].State(c.t))
		}
		if vs[0].Acked["pat2"].Duty != "police" || vs[0].Acked["pat2"].Console != "P2" {
			t.Fatalf("ack %+v", vs[0].Acked)
		}
		if bs := s.ActiveBOLOs(c.t); len(bs) != 0 {
			t.Fatal("a cancelled BOLO is still checked")
		}
		if ib := s.Inbox(cityCar, c.t); len(ib) != 1 || ib[0].ID != id3 {
			t.Fatal("the cancellation is not what the car sees")
		}
	})
	// Urgent unacknowledged: escalated once after its time.
	sid, err := e.SendComm(city, Comm{Kind: CommSafety, Subject: "Officer safety: armed subject", Body: "Known to carry a knife", To: []string{"2A11"}})
	must(t, err)
	if x, _ := e.EscalateComms(); len(x) != 0 {
		t.Fatal("escalated at once")
	}
	c.t = c.t.Add(3 * time.Minute)
	x, _ := e.EscalateComms()
	y, _ := e.EscalateComms()
	if len(x) != 1 || x[0].ID != sid || len(y) != 0 {
		t.Fatalf("escalated %d then %d", len(x), len(y))
	}
	// Permissions: a crew may send a message, not an administrative notice.
	crew := Actor{User: "u2a11", Role: "field", Console: "2A11"}
	if _, err := e.SendComm(crew, Comm{Kind: CommAdmin, Subject: "x", Body: "y", To: []string{ToDispatch}}); err == nil {
		t.Fatal("a crew sent an administrative notice")
	}
	_, err = e.SendComm(crew, Comm{Subject: "Need a supervisor", Body: "At 5 Main St", To: []string{ToDispatch}})
	must(t, err)
	// Drafts are the author's.
	did, err := e.SaveCommDraft(city, CommDraft{Comm: Comm{Subject: "half written"}})
	must(t, err)
	if err := e.RemoveCommDraft(crew, did); err == nil {
		t.Fatal("removed someone else's draft")
	}
	replayed(t, e, path)
}
