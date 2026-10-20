// Package views finds pictures of a place from the side: oblique aerial photos (EagleView, on the
// agency's own licence) and street-level photos (Panoramax, Mapillary), so a dispatcher and the crews
// can see the back door, the fence and the side of the house before they arrive.
//
// Every provider is reached by the server, never by the browser: the agency's keys stay on the
// server, and the browser only ever asks for a picture by a reference this package made. Nothing is
// on unless an administrator turns it on, because looking a place up tells the provider where the
// incident is.
package views

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Kinds of view.
const (
	Oblique = "oblique" // an aerial photo at an angle, looking toward a compass direction
	Ortho   = "ortho"   // an aerial photo straight down
	Street  = "street"  // a photo from the street
)

// Config is what an administrator sets in Admin › Integrations › Views.
type Config struct {
	EagleView EagleViewConfig `json:"eagleview"`
	Panoramax PanoramaxConfig `json:"panoramax"`
	Mapillary MapillaryConfig `json:"mapillary"`
	// GoogleLink offers "Open in Google Street View" as a link in a new tab (nothing is fetched).
	GoogleLink bool `json:"googleLink"`
}

// EagleViewConfig is the agency's EagleView Imagery API application (client credentials).
type EagleViewConfig struct {
	On           bool   `json:"on"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	// Sandbox uses EagleView's sandbox (a small test area in Omaha) instead of production.
	Sandbox bool `json:"sandbox"`
}

// PanoramaxConfig is a Panoramax instance (open street-level photos, CC-BY-SA).
type PanoramaxConfig struct {
	On  bool   `json:"on"`
	URL string `json:"url"` // the instance's API; empty: the federated catalogue
}

// MapillaryConfig is a Mapillary client token (street-level photos, CC-BY-SA).
type MapillaryConfig struct {
	On    bool   `json:"on"`
	Token string `json:"token"`
}

// Any reports whether any provider is on.
func (c Config) Any() bool { return c.EagleView.On || c.Panoramax.On || c.Mapillary.On }

// View is one picture of the place.
type View struct {
	Ref         string    `json:"ref"`      // ask for the picture with this
	Provider    string    `json:"provider"` // EagleView, Panoramax, Mapillary
	Kind        string    `json:"kind"`
	Facing      string    `json:"facing,omitempty"`  // N, E, S, W: the direction the camera looks
	Azimuth     *float64  `json:"azimuth,omitempty"` // degrees from north the camera looks
	Distance    float64   `json:"distance"`          // metres from the camera (street) or 0
	Captured    time.Time `json:"captured,omitzero"`
	Attribution string    `json:"attribution"`
	License     string    `json:"license,omitempty"`
}

// Result is what was found, and what a provider could not answer.
type Result struct {
	Views    []View            `json:"views"`
	Problems map[string]string `json:"problems,omitempty"` // provider: why
	Google   string            `json:"google,omitempty"`   // a Street View link, when offered
}

// provider is one source of pictures.
type provider interface {
	name() string
	find(ctx context.Context, lat, lon float64) ([]found, error)
	fetch(ctx context.Context, f found) (*http.Response, error)
}

// found is a view and what the provider needs to fetch its picture again.
type found struct {
	View
	target string // provider-specific: an image URN or a picture URL the provider returned
	lat    float64
	lon    float64
}

// Service finds views and serves their pictures.
type Service struct {
	Client *http.Client
	Agent  string // User-Agent sent to every provider
	// Hosts overrides providers' addresses (tests).
	Hosts map[string]string

	mu     sync.Mutex
	cfg    Config
	refs   map[string]refEntry
	token  evToken
	public *http.Client
}

type refEntry struct {
	f   found
	p   provider
	exp time.Time
}

// refTTL is how long a reference stays good: long enough to work a call, short enough to forget.
const refTTL = 2 * time.Hour

// Configure replaces the configuration.
func (s *Service) Configure(c Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.EagleView != s.cfg.EagleView {
		s.token = evToken{}
	}
	s.cfg = c
}

// Config is the configuration.
func (s *Service) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *Service) host(key, def string) string {
	if h := s.Hosts[key]; h != "" {
		return h
	}
	return def
}

func (s *Service) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.public == nil {
		s.public = publicClient()
	}
	return s.public
}

// publicClient only connects to addresses on the internet. Picture addresses come from a provider's
// answer, so a provider (or someone in its data) must not be able to point this server at the
// agency's own network.
func publicClient() *http.Client {
	d := &net.Dialer{Timeout: 10 * time.Second, Control: func(network, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip, err := netip.ParseAddr(host)
		if err != nil {
			return err
		}
		if !ip.IsGlobalUnicast() || ip.IsPrivate() {
			return fmt.Errorf("%s is not on the internet", ip)
		}
		return nil
	}}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = d.DialContext
	tr.Proxy = nil
	return &http.Client{Timeout: 20 * time.Second, Transport: tr}
}

func (s *Service) providers() []provider {
	c := s.Config()
	var ps []provider
	if c.EagleView.On && c.EagleView.ClientID != "" && c.EagleView.ClientSecret != "" {
		ps = append(ps, &eagleView{s: s, c: c.EagleView})
	}
	if c.Panoramax.On {
		ps = append(ps, &panoramax{s: s, c: c.Panoramax})
	}
	if c.Mapillary.On && c.Mapillary.Token != "" {
		ps = append(ps, &mapillary{s: s, c: c.Mapillary})
	}
	return ps
}

// Find asks every provider that is on for views of the place, at once.
func (s *Service) Find(ctx context.Context, lat, lon float64) Result {
	res := Result{Views: []View{}}
	if s.Config().GoogleLink {
		res.Google = GoogleLink(lat, lon)
	}
	ps := s.providers()
	type out struct {
		p   provider
		fs  []found
		err error
	}
	ch := make(chan out, len(ps))
	for _, p := range ps {
		go func() {
			fs, err := p.find(ctx, lat, lon)
			ch <- out{p, fs, err}
		}()
	}
	now := time.Now()
	for range ps {
		o := <-ch
		if o.err != nil {
			if res.Problems == nil {
				res.Problems = map[string]string{}
			}
			res.Problems[o.p.name()] = o.err.Error()
			continue
		}
		s.mu.Lock()
		if s.refs == nil {
			s.refs = map[string]refEntry{}
		}
		for k, e := range s.refs {
			if now.After(e.exp) {
				delete(s.refs, k)
			}
		}
		for _, f := range o.fs {
			f.Ref = newRef()
			s.refs[f.Ref] = refEntry{f: f, p: o.p, exp: now.Add(refTTL)}
			res.Views = append(res.Views, f.View)
		}
		s.mu.Unlock()
	}
	sort.SliceStable(res.Views, func(i, j int) bool {
		a, b := res.Views[i], res.Views[j]
		if kindRank(a.Kind) != kindRank(b.Kind) {
			return kindRank(a.Kind) < kindRank(b.Kind)
		}
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		if a.Facing != b.Facing {
			return facingRank(a.Facing) < facingRank(b.Facing)
		}
		return a.Distance < b.Distance
	})
	return res
}

func kindRank(k string) int {
	switch k {
	case Oblique:
		return 0
	case Street:
		return 1
	}
	return 2
}

func facingRank(f string) int { return strings.Index("NESW", f) }

// ErrUnknown is a reference this server did not make, or one that has expired.
var ErrUnknown = errors.New("that picture is no longer available: look the place up again")

// Picture writes the picture for a reference. Only references made by Find are served.
func (s *Service) Picture(ctx context.Context, ref string, w http.ResponseWriter) error {
	s.mu.Lock()
	e, ok := s.refs[ref]
	s.mu.Unlock()
	if !ok || time.Now().After(e.exp) {
		return ErrUnknown
	}
	resp, err := e.p.fetch(ctx, e.f)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s: %s", e.p.name(), resp.Status)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		return fmt.Errorf("%s did not send a picture", e.p.name())
	}
	w.Header().Set("Content-Type", ct)
	// Kept by the browser for the call, not by any shared cache (the provider's terms decide the rest).
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, err = io.Copy(w, io.LimitReader(resp.Body, 40<<20))
	return err
}

// Test looks one place up with each provider that is on, for the administrator's Test button.
func (s *Service) Test(ctx context.Context, lat, lon float64) map[string]string {
	out := map[string]string{}
	for _, p := range s.providers() {
		fs, err := p.find(ctx, lat, lon)
		switch {
		case err != nil:
			out[p.name()] = "failed: " + err.Error()
		case len(fs) == 0:
			out[p.name()] = "connected; no pictures of this place"
		default:
			out[p.name()] = fmt.Sprintf("connected; %d pictures of this place", len(fs))
		}
	}
	return out
}

// GoogleLink is Google Maps' documented link to Street View at a place (opened by the user's browser).
func GoogleLink(lat, lon float64) string {
	return fmt.Sprintf("https://www.google.com/maps/@?api=1&map_action=pano&viewpoint=%.6f,%.6f", lat, lon)
}

func newRef() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// get makes a GET with the service's User-Agent.
func (s *Service) get(ctx context.Context, url string, hdr map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	if s.Agent != "" {
		req.Header.Set("User-Agent", s.Agent)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	return s.client().Do(req)
}

// --- geometry for street photos ---

const earthR = 6371008.8

func distance(lat1, lon1, lat2, lon2 float64) float64 {
	p1, p2 := lat1*math.Pi/180, lat2*math.Pi/180
	dp, dl := p2-p1, (lon2-lon1)*math.Pi/180
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * earthR * math.Asin(math.Sqrt(a))
}

func bearing(lat1, lon1, lat2, lon2 float64) float64 {
	p1, p2 := lat1*math.Pi/180, lat2*math.Pi/180
	dl := (lon2 - lon1) * math.Pi / 180
	y := math.Sin(dl) * math.Cos(p2)
	x := math.Cos(p1)*math.Sin(p2) - math.Sin(p1)*math.Cos(p2)*math.Cos(dl)
	return math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360)
}

func angleOff(a, b float64) float64 {
	d := math.Abs(math.Mod(a-b+540, 360) - 180)
	return d
}

// compass is the compass direction a camera looking along azimuth faces (looking north shows the
// south side of a building).
func compass(az float64) string {
	return string("NESW"[int(math.Mod(az+45, 360)/90)%4])
}

// box is a bounding box of radius metres around a point: min lon, min lat, max lon, max lat.
func box(lat, lon, radius float64) (float64, float64, float64, float64) {
	dlat := radius / earthR * 180 / math.Pi
	dlon := dlat / math.Cos(lat*math.Pi/180)
	return lon - dlon, lat - dlat, lon + dlon, lat + dlat
}

// streetPick keeps the street photos that look at the place, nearest first, at most per compass
// direction, so the dispatcher sees the front and the sides, not twenty pictures of one side.
func streetPick(c []found, lat, lon float64, perSide, max int) []found {
	var keep []found
	for _, f := range c {
		d := distance(f.lat, f.lon, lat, lon)
		if d > streetRadius {
			continue
		}
		f.Distance = math.Round(d)
		if f.Azimuth != nil && d > 4 {
			// The camera has to be pointing at the place, give or take the width of a phone's view.
			if angleOff(*f.Azimuth, bearing(f.lat, f.lon, lat, lon)) > 50 {
				continue
			}
		}
		if f.Azimuth != nil {
			f.Facing = compass(*f.Azimuth)
		}
		keep = append(keep, f)
	}
	sort.SliceStable(keep, func(i, j int) bool { return keep[i].Distance < keep[j].Distance })
	per := map[string]int{}
	var out []found
	for _, f := range keep {
		if per[f.Facing] >= perSide {
			continue
		}
		per[f.Facing]++
		out = append(out, f)
		if len(out) == max {
			break
		}
	}
	return out
}

// streetRadius is how far from the place a street photo may be taken: about a house and its yard.
const streetRadius = 60
