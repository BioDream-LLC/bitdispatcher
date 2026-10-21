package api

import (
	"maps"
	"net/http"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// Messages between the floor and the crews' terminals, and crews attaching themselves to incidents.
func (s *Server) messageRoutes() {
	s.handle("POST /api/messages", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			To       []string `json:"to"`
			Incident string   `json:"incident"`
			Text     string   `json:"text"`
			Urgent   bool     `json:"urgent"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		id, err := s.Eng.SendMessage(a, "", b.To, b.Incident, b.Text, b.Urgent)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("POST /api/messages/{id}/read", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		if err := s.Eng.ReadMessage(a, r.PathValue("id"), cad.ToDispatch); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("POST /api/mdt/message", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		unit, ok := s.mdtUnit(w, r, se)
		if !ok {
			return
		}
		var b struct {
			To     []string `json:"to"`
			Text   string   `json:"text"`
			Urgent bool     `json:"urgent"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		if len(b.To) == 0 {
			b.To = []string{cad.ToDispatch}
		}
		inc := ""
		s.Eng.Read(func(st *cad.State) { inc = st.Units[unit].Incident })
		id, err := s.Eng.SendMessage(a, unit, b.To, inc, b.Text, b.Urgent)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("POST /api/mdt/message/{id}/read", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		unit, ok := s.mdtUnit(w, r, se)
		if !ok {
			return
		}
		if err := s.Eng.ReadMessage(a, r.PathValue("id"), unit); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.handle("POST /api/mdt/attach", auth.Viewer, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		unit, ok := s.mdtUnit(w, r, se)
		if !ok {
			return
		}
		var b struct {
			Incident string `json:"incident"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		if err := s.Eng.SelfAttach(a, unit, b.Incident); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// messageWindow is how far back the screens show messages.
const messageWindow = 24 * time.Hour

func copyMessages(ms []*cad.Message) []*cad.Message {
	out := make([]*cad.Message, 0, len(ms))
	for _, m := range ms {
		c := *m
		c.To = append([]string(nil), m.To...)
		c.Read = maps.Clone(m.Read)
		out = append(out, &c)
	}
	return out
}

// floorMessages are the last day's messages, for the dispatcher's screen.
func floorMessages(st *cad.State, now time.Time) []*cad.Message {
	var ms []*cad.Message
	for _, m := range st.Messages {
		if now.Sub(m.At) <= messageWindow {
			ms = append(ms, m)
		}
	}
	return copyMessages(ms)
}
