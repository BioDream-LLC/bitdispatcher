package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// The public incident feed: what is happening, for the public and for apps such as PulsePoint, without
// what identifies anyone. Off until turned on. Addresses are given by the hundred block and points to
// about 100 m; no names, callbacks, notes or narrative; police incidents only when chosen; call types the
// agency leaves out are never shown. PulsePoint's own interface is given only to its partners: this is a
// documented feed a bridge or a partner reads (docs/PUBLIC-FEED.md).

// PublicFeed is its configuration.
type PublicFeed struct {
	On        bool     `json:"on"`
	Token     string   `json:"token"`     // when set, readers send it as Bearer; empty: anyone may read
	Police    bool     `json:"police"`    // include police incidents
	Leave     []string `json:"leave"`     // call types never shown
	CPRTypes  []string `json:"cprTypes"`  // call types that ask for CPR-trained bystanders
	PushURL   string   `json:"pushUrl"`   // receives the feed on each change
	PushToken string   `json:"pushToken"` // sent as Bearer
}

// PublicFeedView is PublicFeed without its secrets.
type PublicFeedView struct {
	On       bool     `json:"on"`
	Token    bool     `json:"tokenSet"`
	Police   bool     `json:"police"`
	Leave    []string `json:"leave"`
	CPRTypes []string `json:"cprTypes"`
	PushURL  string   `json:"pushUrl"`
	PushTok  bool     `json:"pushTokenSet"`
}

// PublicIncident is one incident in the feed.
type PublicIncident struct {
	ID         string       `json:"id"`
	Type       string       `json:"type"`
	Discipline string       `json:"discipline"`
	Address    string       `json:"address,omitempty"`
	Lat        *float64     `json:"lat,omitempty"`
	Lon        *float64     `json:"lon,omitempty"`
	Received   time.Time    `json:"received"`
	CPR        bool         `json:"cprNeeded,omitempty"`
	Units      []PublicUnit `json:"units"`
}

// PublicUnit is a unit on it.
type PublicUnit struct {
	Unit   string `json:"unit"`
	Status string `json:"status"`
}

// PublicFeedDoc is the feed.
type PublicFeedDoc struct {
	Agency    string           `json:"agency"`
	Generated time.Time        `json:"generated"`
	Incidents []PublicIncident `json:"incidents"`
}

var reHouse = regexp.MustCompile(`^\s*(\d+)(?:-?[A-Za-z]|-\d+)?\s+(.+)$`)

// BlockAddress gives an address by its hundred block: "215 Church Ave SW" is "200 block Church Ave SW".
// What is not a house number and street (a place's name, an intersection) is given as it is, without a
// unit or apartment.
func BlockAddress(a string) string {
	a = strings.TrimSpace(a)
	if i := strings.IndexAny(a, ","); i >= 0 {
		a = a[:i] // the city and the rest: the feed says the agency
	}
	for _, u := range []string{" APT ", " UNIT ", " STE ", " #", " LOT ", " RM "} {
		if i := strings.Index(strings.ToUpper(a), u); i >= 0 {
			a = strings.TrimSpace(a[:i])
		}
	}
	m := reHouse.FindStringSubmatch(a)
	if m == nil {
		return a
	}
	n, _ := strconv.Atoi(m[1])
	return fmt.Sprintf("%d block %s", n/100*100, m[2])
}

func round3(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := math.Round(*p*1000) / 1000
	return &v
}

func (s *Server) publicFeedConfig() PublicFeed {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.integ.PublicFeed
}

// publicFeed builds the feed from the board.
func (s *Server) publicFeed() PublicFeedDoc {
	cfg := s.publicFeedConfig()
	doc := PublicFeedDoc{Generated: time.Now().UTC(), Incidents: []PublicIncident{}}
	s.Eng.Read(func(st *cad.State) {
		doc.Agency = st.Settings.Agency
		for _, in := range st.Incidents {
			if in.Status == cad.Closed || slices.Contains(cfg.Leave, in.Type) {
				continue
			}
			ct := st.CallTypes[in.Type]
			disc, name := "", in.Type
			if ct != nil {
				disc, name = ct.Discipline, ct.Name
			}
			if disc == "police" && !cfg.Police {
				continue
			}
			pi := PublicIncident{ID: in.Number, Type: name, Discipline: disc, Received: in.Created.UTC(), CPR: slices.Contains(cfg.CPRTypes, in.Type), Units: []PublicUnit{}}
			if l := in.Location; l != nil {
				pi.Address, pi.Lat, pi.Lon = BlockAddress(l.Address), round3(l.Lat), round3(l.Lon)
			}
			for _, as := range in.Units {
				if as.Cleared.IsZero() {
					pi.Units = append(pi.Units, PublicUnit{Unit: as.Unit, Status: as.Status})
				}
			}
			doc.Incidents = append(doc.Incidents, pi)
		}
	})
	slices.SortFunc(doc.Incidents, func(a, b PublicIncident) int { return b.Received.Compare(a.Received) })
	return doc
}

func (s *Server) publicFeedRoutes() {
	s.mux.HandleFunc("GET /api/public/incidents", func(w http.ResponseWriter, r *http.Request) {
		cfg := s.publicFeedConfig()
		if !cfg.On {
			fail(w, 404, "the public feed is off")
			return
		}
		if cfg.Token != "" {
			tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(tok), []byte(cfg.Token)) != 1 {
				fail(w, 401, "send the feed's token")
				return
			}
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Cache-Control", "public, max-age=15")
		writeJSON(w, 200, s.publicFeed())
	})
	s.handle("GET /api/admin/public-feed", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		writeJSON(w, 200, publicFeedView(s.publicFeedConfig()))
	})
	s.handle("GET /api/admin/public-feed/preview", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		writeJSON(w, 200, s.publicFeed())
	})
	s.handle("PUT /api/admin/public-feed", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b PublicFeed
		if !readJSON(w, r, &b) {
			return
		}
		b.PushURL = strings.TrimSpace(b.PushURL)
		if b.PushURL != "" && !strings.HasPrefix(b.PushURL, "https://") && !strings.HasPrefix(b.PushURL, "http://127.0.0.1") {
			fail(w, 422, "the push address starts with https://")
			return
		}
		s.mu.Lock()
		n := s.integ
		old := n.PublicFeed
		keepSecret(&old.Token, b.Token)
		keepSecret(&old.PushToken, b.PushToken)
		old.On, old.Police, old.Leave, old.CPRTypes, old.PushURL = b.On, b.Police, b.Leave, b.CPRTypes, b.PushURL
		n.PublicFeed = old
		data, _ := json.MarshalIndent(n, "", "  ")
		tmp := s.integrationsPath() + ".tmp"
		err := os.WriteFile(tmp, data, 0o600)
		if err == nil {
			err = os.Rename(tmp, s.integrationsPath())
		}
		if err == nil {
			s.integ = n
		}
		s.mu.Unlock()
		if err != nil {
			writeErr(w, err)
			return
		}
		s.Eng.Record(a, cad.EvIntegration, map[string]any{"publicFeed": old.On, "police": old.Police, "pushUrl": old.PushURL, "open": old.Token == ""})
		writeJSON(w, 200, publicFeedView(old))
	})
}

func publicFeedView(c PublicFeed) PublicFeedView {
	return PublicFeedView{On: c.On, Token: c.Token != "", Police: c.Police, Leave: strs(c.Leave), CPRTypes: strs(c.CPRTypes), PushURL: c.PushURL, PushTok: c.PushToken != ""}
}

func strs(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// publicFeedPusher posts the feed to the push address when it changes, at most every 5 seconds.
func (s *Server) publicFeedPusher(ctx context.Context) {
	ch := s.Eng.Subscribe()
	defer s.Eng.Unsubscribe(ch)
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	dirty, last := false, ""
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			dirty = true
		case <-tick.C:
			cfg := s.publicFeedConfig()
			if !dirty || !cfg.On || cfg.PushURL == "" {
				continue
			}
			dirty = false
			doc := s.publicFeed()
			body, _ := json.Marshal(doc.Incidents)
			if string(body) == last {
				continue
			}
			full, _ := json.Marshal(doc)
			req, err := http.NewRequestWithContext(ctx, "POST", cfg.PushURL, bytes.NewReader(full))
			if err != nil {
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			if cfg.PushToken != "" {
				req.Header.Set("Authorization", "Bearer "+cfg.PushToken)
			}
			resp, err := shareClient.Do(req)
			if err != nil {
				slog.Warn("public feed: push failed", "err", err)
				dirty = true
				continue
			}
			resp.Body.Close()
			if resp.StatusCode < 300 {
				last = string(body)
			} else {
				slog.Warn("public feed: push refused", "status", resp.Status)
			}
		}
	}
}
