package views

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// EagleView's Imagery API (v3 paths), documented at developer.eagleview.com. The agency's own licence:
// an application's client ID and secret, exchanged for an hour-long token.
const (
	evTokenURL   = "https://apicenter.eagleview.com/oauth2/v1/token"
	evProduction = "https://apis.eagleview.com"
	evSandbox    = "https://sandbox.apis.eagleview.com"
	// evRadius is the circle the picture covers, in metres (the API takes 1 to 100): a house and its yard.
	evRadius = 40
)

type evToken struct {
	value string
	exp   time.Time
}

type eagleView struct {
	s *Service
	c EagleViewConfig
}

func (e *eagleView) name() string { return "EagleView" }

func (e *eagleView) base() string {
	if e.c.Sandbox {
		return e.s.host("eagleview", evSandbox)
	}
	return e.s.host("eagleview", evProduction)
}

// bearer is a token, reused until five minutes before it expires.
func (e *eagleView) bearer(ctx context.Context) (string, error) {
	e.s.mu.Lock()
	t := e.s.token
	e.s.mu.Unlock()
	if t.value != "" && time.Now().Before(t.exp) {
		return t.value, nil
	}
	req, err := http.NewRequestWithContext(ctx, "POST", e.s.host("eagleview-token", evTokenURL), strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(e.c.ClientID+":"+e.c.ClientSecret)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if e.s.Agent != "" {
		req.Header.Set("User-Agent", e.s.Agent)
	}
	resp, err := e.s.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 400 || resp.StatusCode == 401 {
		return "", errors.New("EagleView refused the client ID and secret")
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("EagleView sign-in: %s", resp.Status)
	}
	var b struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&b); err != nil || b.AccessToken == "" {
		return "", errors.New("EagleView sign-in: no token in the answer")
	}
	life := time.Duration(b.ExpiresIn) * time.Second
	if life <= 0 {
		life = time.Hour
	}
	t = evToken{value: b.AccessToken, exp: time.Now().Add(life - 5*time.Minute)}
	e.s.mu.Lock()
	e.s.token = t
	e.s.mu.Unlock()
	return t.value, nil
}

// evImage is the part of an oblique or ortho image bitdispatcher uses.
type evImage struct {
	URN      string `json:"urn"`
	ShotTime string `json:"shot_time"`
	LookAt   *struct {
		Azimuth float64 `json:"azimuth"`
	} `json:"look_at"`
}

type evImages struct {
	Images []evImage `json:"images"`
}

type evRank struct {
	Captures []struct {
		Capture struct {
			URN       string `json:"urn"`
			StartDate string `json:"start_date"`
		} `json:"capture"`
		Obliques struct {
			North evImages `json:"north"`
			East  evImages `json:"east"`
			South evImages `json:"south"`
			West  evImages `json:"west"`
		} `json:"obliques"`
		Orthos evImages `json:"orthos"`
	} `json:"captures"`
}

// rankRequest asks for the most recent capture with as many of the four obliques as there are, and
// the straight-down photo, one image of each.
func rankRequest(lat, lon float64) []byte {
	point, _ := json.Marshal(map[string]any{"type": "Point", "coordinates": []float64{lon, lat}})
	b, _ := json.Marshal(map[string]any{
		"center": map[string]any{
			"point":            map[string]any{"geojson": map[string]string{"value": string(point), "epsg": "EPSG:4326"}},
			"radius_in_meters": evRadius,
		},
		"view": map[string]any{
			"obliques":            map[string]any{"cardinals": map[string]bool{"north": true, "east": true, "south": true, "west": true}},
			"orthos":              map[string]any{},
			"max_images_per_view": 1,
		},
		"capture":        map[string]any{"from": map[string]any{"any": map[string]int{"result_count": 1}}},
		"response_props": map[string]any{"shot_time": true, "look_at": true},
	})
	return b
}

func (e *eagleView) find(ctx context.Context, lat, lon float64) ([]found, error) {
	tok, err := e.bearer(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", e.base()+"/imagery/v3/discovery/rank/location", bytes.NewReader(rankRequest(lat, lon)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	if e.s.Agent != "" {
		req.Header.Set("User-Agent", e.s.Agent)
	}
	resp, err := e.s.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == 401:
		e.s.mu.Lock()
		e.s.token = evToken{}
		e.s.mu.Unlock()
		return nil, errors.New("EagleView: the token was refused; try again")
	case resp.StatusCode == 403:
		return nil, errors.New("EagleView: this application has no Imagery API access for that place")
	case resp.StatusCode == 404:
		return nil, nil // no imagery there
	case resp.StatusCode == 429:
		return nil, errors.New("EagleView: too many requests; try again in a moment")
	case resp.StatusCode != 200:
		return nil, fmt.Errorf("EagleView: %s", resp.Status)
	}
	var r evRank
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&r); err != nil {
		return nil, fmt.Errorf("EagleView: unreadable answer: %w", err)
	}
	var out []found
	for _, c := range r.Captures {
		add := func(kind, facing string, ims evImages) {
			for _, im := range ims.Images {
				if im.URN == "" {
					continue
				}
				v := View{Provider: "EagleView", Kind: kind, Facing: facing, Attribution: "© EagleView", Captured: parseTime(im.ShotTime, c.Capture.StartDate)}
				if im.LookAt != nil {
					az := im.LookAt.Azimuth
					v.Azimuth = &az
				}
				out = append(out, found{View: v, target: im.URN, lat: lat, lon: lon})
				break
			}
		}
		add(Oblique, "N", c.Obliques.North)
		add(Oblique, "E", c.Obliques.East)
		add(Oblique, "S", c.Obliques.South)
		add(Oblique, "W", c.Obliques.West)
		add(Ortho, "", c.Orthos)
		break // the most recent capture
	}
	return out, nil
}

func (e *eagleView) fetch(ctx context.Context, f found) (*http.Response, error) {
	tok, err := e.bearer(ctx)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("center.x", fmt.Sprintf("%.7f", f.lon))
	q.Set("center.y", fmt.Sprintf("%.7f", f.lat))
	q.Set("center.radius", fmt.Sprint(evRadius))
	q.Set("epsg", "EPSG:4326")
	q.Set("format", "IMAGE_FORMAT_JPEG")
	return e.s.get(ctx, e.base()+"/imagery/v3/images/"+url.PathEscape(f.target)+"/location?"+q.Encode(), map[string]string{"Authorization": "Bearer " + tok})
}

func parseTime(vals ...string) time.Time {
	for _, v := range vals {
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
			if t, err := time.Parse(layout, v); err == nil {
				return t.UTC()
			}
		}
	}
	return time.Time{}
}
