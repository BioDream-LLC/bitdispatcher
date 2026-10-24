package cad

import (
	"testing"
	"time"
)

func TestPublicAlerts(t *testing.T) {
	e, c, path := testEngine(t)
	sup := Actor{User: "msmith", Role: "supervisor", Console: "S1"}
	sup2 := Actor{User: "kjones", Role: "supervisor", Console: "S2"}
	lat, lon, latOut, lonOut := 40.2715, -76.8815, 40.40, -76.70
	fence, err := e.SaveGeofence(SystemActor, Geofence{Name: "Chemical plant area", Kind: FenceCaution,
		Note: "Ammonia stored on site.", Ring: [][2]float64{{-76.885, 40.268}, {-76.877, 40.268}, {-76.877, 40.275}, {-76.885, 40.275}}})
	must(t, err)
	if _, err := e.SaveAlertList(SystemActor, AlertList{Name: "Residents", Gateways: []string{"sms"}, People: []Subscriber{{Name: "No way to reach"}}}); err == nil {
		t.Fatal("a subscriber with no phone, email or topic was saved")
	}
	list, err := e.SaveAlertList(SystemActor, AlertList{Name: "Residents", Gateways: []string{"sms", "push"}, People: []Subscriber{
		{Name: "Inside", Phone: "+15405550101", Lat: &lat, Lon: &lon},
		{Name: "Outside", Phone: "+15405550102", Lat: &latOut, Lon: &lonOut},
		{Name: "Anywhere", Email: "a@example.org", Lang: "es"},
	}})
	must(t, err)

	base := PublicAlert{Status: "Actual", Event: "SPW", EventName: "Shelter in place warning", Category: "Env", Response: "Shelter",
		Urgency: "Immediate", Severity: "Severe", Certainty: "Observed",
		Text:    AlertText{Headline: "Shelter in place near the chemical plant", Instruction: "Go inside, close windows, turn off fans.", Short: "Chemical leak near Main St: go inside, shut windows, turn off fans now."},
		Spanish: &AlertText{Headline: "Refúgiese en casa cerca de la planta química", Short: "Fuga química: entre, cierre ventanas y apague ventiladores."},
		Area:    AlertArea{Fence: fence}, Lists: []string{list}, Expires: c.t.Add(2 * time.Hour)}

	// Checks.
	bad := base
	bad.Text.Short = "This phone alert text is far too long to be sent as a wireless emergency alert because it runs past ninety"
	if _, err := e.DraftAlert(sup, bad, false); err == nil {
		t.Fatal("a 100-character phone alert was taken")
	}
	bad = base
	bad.Expires = c.t.Add(72 * time.Hour)
	if _, err := e.DraftAlert(sup, bad, false); err == nil {
		t.Fatal("a 72-hour alert was taken")
	}

	// With approval: waits, and the author cannot approve it.
	id, err := e.DraftAlert(sup, base, true)
	must(t, err)
	if got := e.UndeliveredAlerts(); len(got) != 0 {
		t.Fatalf("a pending alert is due for delivery: %v", got)
	}
	if err := e.ApproveAlert(sup, id); err == nil {
		t.Fatal("the author approved their own alert")
	}
	must(t, e.ApproveAlert(sup2, id))
	var a PublicAlert
	e.Read(func(s *State) { a = *s.Alert(id) })
	if a.State != AlertIssued || a.ApprovedBy != "kjones" || a.By != "msmith" || a.Area.Desc != "Chemical plant area" || len(a.Area.Polygon) != 4 {
		t.Fatalf("issued %+v", a)
	}
	got, ok := e.TakeAlert(id)
	if !ok || got.ID != id {
		t.Fatal("the issued alert was not handed over for delivery")
	}
	if _, ok := e.TakeAlert(id); ok {
		t.Fatal("an alert was handed over twice")
	}
	var to map[string][]Subscriber
	e.Read(func(s *State) { to = s.AlertRecipients(s.Alert(id)) })
	if len(to["sms"]) != 2 || len(to["push"]) != 2 || to["sms"][0].Name != "Inside" || to["sms"][1].Name != "Anywhere" {
		t.Fatalf("recipients %+v", to)
	}
	must(t, e.RecordAlertResults(SystemActor, id, []PageResult{{Gateway: "sms", OK: true, To: "+1540•••0101"}}, 2))

	// A rejected one.
	r, err := e.DraftAlert(sup, base, true)
	must(t, err)
	must(t, e.RejectAlert(sup2, r, "wrong area"))
	if err := e.ApproveAlert(sup2, r); err == nil {
		t.Fatal("a rejected alert was approved")
	}

	// An update, then a cancel.
	c.t = c.t.Add(30 * time.Minute)
	up := base
	up.MsgType, up.Of = "Update", id
	up.Text.Headline = "Shelter in place continues near the chemical plant"
	uid, err := e.DraftAlert(sup, up, false)
	must(t, err)
	e.Read(func(s *State) {
		if s.Alert(id).State != AlertUpdated || s.Alert(id).SupersededBy != uid || len(s.AlertsInForce(c.t)) != 1 {
			t.Fatalf("after the update: %+v", *s.Alert(id))
		}
	})
	if _, err := e.DraftAlert(sup, PublicAlert{MsgType: "Cancel", Of: id}, false); err == nil {
		t.Fatal("a replaced alert was cancelled")
	}
	cid, err := e.DraftAlert(sup, PublicAlert{MsgType: "Cancel", Of: uid}, false)
	must(t, err)
	e.Read(func(s *State) {
		x := s.Alert(cid)
		if x.MsgType != "Cancel" || x.Response != "AllClear" || x.Event != "SPW" || x.Spanish == nil || s.Alert(uid).State != AlertCancelled {
			t.Fatalf("cancel %+v", *x)
		}
		if n := len(s.AlertsInForce(c.t)); n != 1 { // the cancel itself, for an hour
			t.Fatalf("%d in force after the cancel", n)
		}
	})
	if got := e.UndeliveredAlerts(); len(got) != 2 {
		t.Fatalf("the update and cancel are not due for delivery: %v", got)
	}
	replayed(t, e, path)
}
