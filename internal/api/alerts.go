package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// Outside alerts: alarm companies, plate readers, gunshot detection and camera analytics post here with
// the hook token (docs/ALERTS.md). Each becomes a call in the queue, data only, for a dispatcher to make
// an incident from; nothing is dispatched on its own. Alarm companies are told what happened to theirs.

type alertBody struct {
	cad.Alert
	Lat     *float64 `json:"lat"`
	Lon     *float64 `json:"lon"`
	Radius  float64  `json:"radius"`
	Address string   `json:"address"`
	Cancel  bool     `json:"cancel"` // the sender cancels it (a false alarm, a keyholder on scene)
}

func (s *Server) alertRoutes() {
	m := s.mux
	m.HandleFunc("POST /api/hooks/alert", s.hook(s.hookAlert(""))) // any kind, given in the body
	m.HandleFunc("POST /api/hooks/alarm", s.hook(s.hookAlert(cad.AlertAlarm)))
	m.HandleFunc("POST /api/hooks/lpr", s.hook(s.hookAlert(cad.AlertLPR)))
	m.HandleFunc("POST /api/hooks/gunshot", s.hook(s.hookAlert(cad.AlertGunshot)))
}

func (s *Server) hookAlert(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { s.takeAlert(w, r, kind, nil, nil) }
}

// takeAlert takes an outside alert: from the shared hook, from an alarm company's own address (company),
// or typed by a call-taker (by) from an alarm company's phone call.
func (s *Server) takeAlert(w http.ResponseWriter, r *http.Request, kind string, company *AlarmCompany, by *cad.Actor) {
	{
		var b alertBody
		if !readJSON(w, r, &b) {
			return
		}
		if kind != "" {
			b.Kind = kind
		}
		phoned := by != nil
		b.Alert.Company, b.Alert.Phoned = "", phoned
		if company != nil {
			b.Sender, b.Alert.Company = company.Name, company.ID
			if b.Callback == "" {
				b.Callback = company.Phone
			}
		}
		if err := cad.CheckAlert(&b.Alert); err != nil {
			writeErr(w, err)
			return
		}
		al := b.Alert
		id := al.CallID()
		actor := cad.Actor{User: "alert:" + al.Kind}
		if phoned {
			actor = *by
		}
		var existing *cad.Call
		s.Eng.Read(func(st *cad.State) {
			if c, ok := st.Calls[id]; ok {
				cc := *c
				existing = &cc
			}
		})
		if b.Cancel {
			al.Status = "cancelled"
		} else if existing != nil {
			al.Status = "updated"
		} else {
			al.Status = "new"
		}
		d := cad.CallData{Source: "alert", State: "data", Class: strings.ToUpper(al.Kind), Provider: al.Sender, Callback: al.Callback,
			CallerName: firstNonEmptyStr(al.Premise, al.Sender), Emergency: true, Alert: &al, Notes: alertNotes(&al)}
		if al.Kind == cad.AlertAlarm && existing == nil && !b.Cancel {
			d.Notes = append(d.Notes, s.alarmChecks(&al, b.Address)...)
		}
		if phoned {
			d.Notes = append(d.Notes, "Phoned in by the alarm company; typed by "+actor.User)
		}
		if existing != nil && existing.State == "ended" {
			d.State = "" // an update to an ended alert does not ring it again
		}
		if b.Lat != nil && b.Lon != nil && *b.Lat >= -90 && *b.Lat <= 90 && *b.Lon >= -180 && *b.Lon <= 180 {
			d.Location = pointLoc(*b.Lat, *b.Lon, b.Radius, map[string]string{cad.AlertAlarm: "Premise", cad.AlertLPR: "Camera", cad.AlertGunshot: "Acoustic"}[al.Kind], "alert")
			d.Location.Address = strings.TrimSpace(b.Address)
		} else if strings.TrimSpace(b.Address) != "" {
			d.Location = &ng911.Location{Address: strings.TrimSpace(b.Address), Source: "alert", Method: "Premise"}
		}
		if err := s.Eng.UpsertCall(actor, id, d); err != nil {
			writeErr(w, err)
			return
		}
		inc := ""
		s.Eng.Read(func(st *cad.State) {
			if c, ok := st.Calls[id]; ok {
				inc = c.Incident
			}
		})
		// On an incident already: the change goes on it, as a priority note when the sender cancels.
		if inc != "" {
			note := al.Sender + ": " + al.Summary()
			if b.Cancel {
				note = strings.TrimSpace("ALARM CANCELLED by " + al.Sender + " " + al.Detail)
			} else if al.Detail != "" {
				note += " · " + al.Detail
			}
			if err := s.Eng.AddNote(actor, inc, note, b.Cancel || al.Verified); err != nil {
				s.Log.Warn("alert note not added", "incident", inc, "err", err)
			}
		}
		var suggest string
		s.Eng.Read(func(st *cad.State) { suggest = st.SuggestedType(&al) })
		writeJSON(w, 200, map[string]any{"call": id, "status": al.Status, "incident": inc, "suggestedType": suggest})
	}
}

func alertNotes(a *cad.Alert) []string {
	n := []string{a.Summary()}
	add := func(label, v string) {
		if strings.TrimSpace(v) != "" {
			n = append(n, label+": "+strings.TrimSpace(v))
		}
	}
	add("Event", a.ID)
	add("Account", a.Account)
	add("Permit", a.Permit)
	add("Keyholder", a.Keyholder)
	add("Operator", a.Operator)
	add("Camera", a.Camera)
	add("Confidence", a.Confidence)
	add("Detail", a.Detail)
	return n
}

// alarmAnswers tells alarm companies what happened to their alarms (accepted, dispatched, on scene,
// closed), at the address an administrator set; each status once. ASAP over Nlets is the national
// standard for this; until it is connected, this is the same conversation over HTTPS (docs/ALERTS.md).
func (s *Server) alarmAnswers(ctx context.Context) {
	ch := s.Eng.Subscribe()
	defer s.Eng.Unsubscribe(ch)
	sent := map[string]string{} // call -> last status sent
	client := &http.Client{Timeout: 15 * time.Second}
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-ch:
			x := s.extra()
			companies := map[string]AlarmCompany{}
			for _, c := range s.nextConfig().AlarmCompanies {
				if !c.Off && c.AnswerURL != "" {
					companies[c.ID] = c
				}
			}
			if (x.AlertAnswerURL == "" && len(companies) == 0) || (n.Inc == "" && n.Call == "") {
				continue
			}
			type answer struct {
				company     string
				Sender      string    `json:"sender"`
				ID          string    `json:"id"`
				Status      string    `json:"status"` // accepted, dispatched, onscene, closed, ended
				Incident    string    `json:"incident,omitempty"`
				Units       []string  `json:"units,omitempty"`
				Disposition string    `json:"disposition,omitempty"`
				At          time.Time `json:"at"`
			}
			var out []answer
			s.Eng.Read(func(st *cad.State) {
				for id, c := range st.Calls {
					if c.Alert == nil || c.Alert.Kind != cad.AlertAlarm || (c.Incident != n.Inc && id != n.Call) {
						continue
					}
					if c.Alert.Phoned {
						continue // phoned in: the company is told by phone, as it told us
					}
					a := answer{company: c.Alert.Company, Sender: c.Alert.Sender, ID: c.Alert.ID, Incident: c.Incident, At: s.Eng.Now().UTC()}
					in := st.Incidents[c.Incident]
					switch {
					case in == nil && c.State == "ended":
						a.Status = "ended" // nobody made an incident of it
					case in == nil:
						continue
					case in.Status == cad.Closed:
						a.Status, a.Disposition = "closed", in.Disposition
					default:
						a.Status = "accepted"
						for _, as := range in.Units {
							a.Units = append(a.Units, as.Unit)
							if !as.OnScene.IsZero() {
								a.Status = "onscene"
							} else if a.Status != "onscene" {
								a.Status = "dispatched"
							}
						}
					}
					if sent[id] == a.Status {
						continue
					}
					sent[id] = a.Status
					out = append(out, a)
				}
			})
			for _, a := range out {
				body, _ := json.Marshal(a)
				to, tok := x.AlertAnswerURL, x.AlertAnswerToken
				if c, ok := companies[a.company]; ok {
					to, tok = c.AnswerURL, c.AnswerToken
				} else if a.company != "" {
					continue // a company on the list without an answer address
				}
				if to == "" {
					continue
				}
				go func() {
					req, err := http.NewRequestWithContext(ctx, "POST", to, bytes.NewReader(body))
					if err != nil {
						return
					}
					req.Header.Set("Content-Type", "application/json")
					if tok != "" {
						req.Header.Set("Authorization", "Bearer "+tok)
					}
					resp, err := client.Do(req)
					if err != nil {
						s.Log.Warn("alarm answer not delivered", "id", a.ID, "err", err)
						return
					}
					resp.Body.Close()
				}()
			}
		}
	}
}
