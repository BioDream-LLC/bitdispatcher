package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// Several sites (cad/site.go): each site serves its own events to the others, and copies theirs. Sites
// authenticate with a token each way and pin each other's certificates.

// SiteConfig is this site's part in a multi-site system, kept in the data directory (it holds secrets).
type SiteConfig struct {
	Name   string `json:"name"`   // north, backup, county-b
	Prefix string `json:"prefix"` // N: incident numbers N26-000001
	Token  string `json:"token"`  // what the other sites present to copy from this one
	Peers  []Peer `json:"peers"`
}

// Peer is another site.
type Peer struct {
	Name        string `json:"name"`
	URL         string `json:"url"`         // https://other-site:9443
	Token       string `json:"token"`       // the other site's token
	Fingerprint string `json:"fingerprint"` // its certificate's SHA-256
}

// PeerHealth is how a link to another site is.
type PeerHealth struct {
	Name   string    `json:"name"`
	URL    string    `json:"url"`
	OK     bool      `json:"ok"`
	Last   time.Time `json:"last,omitzero"` // last successful copy
	Error  string    `json:"error,omitempty"`
	Copied uint64    `json:"copied"` // events copied from it in all
}

type sites struct {
	mu     sync.Mutex
	cfg    SiteConfig
	health map[string]*PeerHealth
	wake   chan struct{}
}

func (s *Server) siteFile() string { return filepath.Join(s.DataDir, "site.json") }

var reSiteName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,30}$`)
var reSitePrefix = regexp.MustCompile(`^[A-Z]{1,3}$`)

// LoadSites reads site.json; with a name, the engine becomes that site and opens its copies of the
// others. Called once at start.
func (s *Server) LoadSites() error {
	s.sites.health = map[string]*PeerHealth{}
	s.sites.wake = make(chan struct{}, 1)
	b, err := os.ReadFile(s.siteFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var c SiteConfig
	if err := json.Unmarshal(b, &c); err != nil {
		return fmt.Errorf("site.json: %w", err)
	}
	return s.applySites(c)
}

func (s *Server) applySites(c SiteConfig) error {
	if c.Name == "" {
		return nil
	}
	if cur := s.Eng.Site(); cur == "" {
		s.Eng.SetSite(c.Name, c.Prefix)
	}
	var names []string
	for _, p := range c.Peers {
		names = append(names, p.Name)
	}
	if err := s.Eng.OpenPeers(filepath.Join(s.DataDir, "peers"), names); err != nil {
		return err
	}
	s.sites.mu.Lock()
	s.sites.cfg = c
	for _, p := range c.Peers {
		if s.sites.health[p.Name] == nil {
			s.sites.health[p.Name] = &PeerHealth{Name: p.Name}
		}
		s.sites.health[p.Name].URL = p.URL
	}
	s.sites.mu.Unlock()
	return nil
}

func (s *Server) siteRoutes() {
	s.mux.HandleFunc("GET /api/peer/events", s.peerEvents)
	s.handle("GET /api/admin/sites", auth.Admin, s.sitesGet)
	s.handle("PUT /api/admin/sites", auth.Admin, s.sitesSave)
	s.handle("POST /api/admin/sites/fingerprint", auth.Admin, s.fingerprint)
}

// peerEvents serves this site's own events to another site that presents this site's token.
func (s *Server) peerEvents(w http.ResponseWriter, r *http.Request) {
	s.sites.mu.Lock()
	tok := s.sites.cfg.Token
	s.sites.mu.Unlock()
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if tok == "" || subtle.ConstantTimeCompare([]byte(got), []byte(tok)) != 1 {
		fail(w, http.StatusUnauthorized, "not a known site")
		return
	}
	after, _ := strconv.ParseUint(r.URL.Query().Get("after"), 10, 64)
	evs, err := s.Eng.LocalEvents(after, 2000)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"site": s.Eng.Site(), "events": evs})
}

func pinnedClient(fp string) *http.Client {
	fp = strings.ToLower(strings.NewReplacer(":", "", " ", "").Replace(fp))
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true, // replaced by the pin
		VerifyPeerCertificate: func(raw [][]byte, _ [][]*x509.Certificate) error {
			if len(raw) == 0 {
				return errors.New("no certificate")
			}
			sum := sha256.Sum256(raw[0])
			if hex.EncodeToString(sum[:]) != fp {
				return errors.New("the other site's certificate is not the pinned one")
			}
			return nil
		}}
	return &http.Client{Transport: tr, Timeout: 20 * time.Second}
}

// runSites copies events from every other site, every second while there is nothing new and at once
// while there is.
func (s *Server) runSites(ctx context.Context) {
	for {
		s.sites.mu.Lock()
		peers := append([]Peer{}, s.sites.cfg.Peers...)
		s.sites.mu.Unlock()
		more := false
		for _, p := range peers {
			n, err := s.pullPeer(ctx, p)
			s.sites.mu.Lock()
			h := s.sites.health[p.Name]
			if h == nil {
				h = &PeerHealth{Name: p.Name, URL: p.URL}
				s.sites.health[p.Name] = h
			}
			if err != nil {
				h.OK, h.Error = false, err.Error()
			} else {
				h.OK, h.Error, h.Last = true, "", time.Now()
				h.Copied += uint64(n)
			}
			s.sites.mu.Unlock()
			more = more || n >= 2000
		}
		if more {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		case <-s.sites.wake:
		}
	}
}

func (s *Server) pullPeer(ctx context.Context, p Peer) (int, error) {
	u, err := url.Parse(strings.TrimSuffix(p.URL, "/") + "/api/peer/events")
	if err != nil || u.Scheme != "https" {
		return 0, errors.New("the site's address must be https://")
	}
	q := u.Query()
	q.Set("after", strconv.FormatUint(s.Eng.PeerSeq(p.Name), 10))
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	resp, err := pinnedClient(p.Fingerprint).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return 0, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var body struct {
		Site   string         `json:"site"`
		Events []ledger.Event `json:"events"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 256<<20)).Decode(&body); err != nil {
		return 0, err
	}
	if body.Site != p.Name {
		return 0, fmt.Errorf("the site at that address says it is %q, not %q", body.Site, p.Name)
	}
	return s.Eng.AddRemote(p.Name, body.Events)
}

// SitesView is the admin screen's picture (tokens of other sites are not sent back).
type SitesView struct {
	Name        string       `json:"name"`
	Prefix      string       `json:"prefix"`
	Token       string       `json:"token"`
	Fingerprint string       `json:"fingerprint"` // this site's certificate, for the others to pin
	Peers       []Peer       `json:"peers"`
	Health      []PeerHealth `json:"health"`
	Fixed       bool         `json:"fixed"` // the name and prefix are in use and cannot change
}

func (s *Server) sitesGet(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	writeJSON(w, 200, s.sitesView())
}

func (s *Server) sitesView() SitesView {
	s.sites.mu.Lock()
	defer s.sites.mu.Unlock()
	v := SitesView{Name: s.sites.cfg.Name, Prefix: s.sites.cfg.Prefix, Token: s.sites.cfg.Token, Fingerprint: s.TLSFingerprint, Peers: []Peer{}, Health: []PeerHealth{}, Fixed: s.Eng.Site() != ""}
	for _, p := range s.sites.cfg.Peers {
		p.Token = strings.Repeat("•", 8)
		v.Peers = append(v.Peers, p)
		if h := s.sites.health[p.Name]; h != nil {
			v.Health = append(v.Health, *h)
		}
	}
	return v
}

func (s *Server) sitesSave(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	var c SiteConfig
	if !readJSON(w, r, &c) {
		return
	}
	c.Name, c.Prefix = strings.ToLower(strings.TrimSpace(c.Name)), strings.ToUpper(strings.TrimSpace(c.Prefix))
	s.sites.mu.Lock()
	old := s.sites.cfg
	s.sites.mu.Unlock()
	if cur := s.Eng.Site(); cur != "" && (c.Name != old.Name || c.Prefix != old.Prefix) {
		fail(w, http.StatusConflict, "this site's name and prefix are in use in the ledger and cannot change")
		return
	}
	if !reSiteName.MatchString(c.Name) || !reSitePrefix.MatchString(c.Prefix) {
		fail(w, http.StatusBadRequest, "the site name is lower-case letters, digits and dashes; the prefix one to three capital letters")
		return
	}
	if c.Token == "" {
		c.Token = "bds_" + auth.RandomToken(24)
	}
	seen := map[string]bool{c.Name: true}
	for i, p := range c.Peers {
		p.Name = strings.ToLower(strings.TrimSpace(p.Name))
		if !reSiteName.MatchString(p.Name) || seen[p.Name] {
			fail(w, http.StatusBadRequest, fmt.Sprintf("site %d: each site needs its own name", i+1))
			return
		}
		seen[p.Name] = true
		if !strings.HasPrefix(p.URL, "https://") {
			fail(w, http.StatusBadRequest, p.Name+": the address must be https://")
			return
		}
		// A token shown masked is kept as it was.
		if strings.Trim(p.Token, "•") == "" {
			for _, o := range old.Peers {
				if o.Name == p.Name {
					p.Token = o.Token
				}
			}
		}
		if p.Token == "" || len(strings.ReplaceAll(p.Fingerprint, ":", "")) != 64 {
			fail(w, http.StatusBadRequest, p.Name+": give its token and its certificate fingerprint (Read it)")
			return
		}
		c.Peers[i] = p
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	if err := os.WriteFile(s.siteFile(), b, 0o600); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.applySites(c); err != nil {
		writeErr(w, err)
		return
	}
	var names []string
	for _, p := range c.Peers {
		names = append(names, p.Name)
	}
	s.Eng.Record(a, cad.EvIntegration, map[string]any{"site": c.Name, "prefix": c.Prefix, "peers": names})
	select {
	case s.sites.wake <- struct{}{}:
	default:
	}
	writeJSON(w, 200, s.sitesView())
}
