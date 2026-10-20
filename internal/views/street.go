package views

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// --- Panoramax: open street-level photos (STAC API, CC-BY-SA) ---

// PanoramaxDefault is the federated catalogue of every Panoramax instance.
const PanoramaxDefault = "https://api.panoramax.xyz"

type panoramax struct {
	s *Service
	c PanoramaxConfig
}

func (p *panoramax) name() string { return "Panoramax" }

func (p *panoramax) api() string {
	u := strings.TrimRight(strings.TrimSpace(p.c.URL), "/")
	if u == "" {
		u = PanoramaxDefault
	}
	return p.s.host("panoramax", u)
}

type pxSearch struct {
	Features []struct {
		ID       string `json:"id"`
		Geometry struct {
			Coordinates []float64 `json:"coordinates"`
		} `json:"geometry"`
		Properties struct {
			Datetime string   `json:"datetime"`
			Azimuth  *float64 `json:"view:azimuth"`
			License  string   `json:"license"`
		} `json:"properties"`
		Providers []struct {
			Name string `json:"name"`
		} `json:"providers"`
		Assets map[string]struct {
			Href string `json:"href"`
		} `json:"assets"`
	} `json:"features"`
}

func (p *panoramax) find(ctx context.Context, lat, lon float64) ([]found, error) {
	a, b, c, d := box(lat, lon, streetRadius)
	resp, err := p.s.get(ctx, fmt.Sprintf("%s/api/search?bbox=%.6f,%.6f,%.6f,%.6f&limit=100", p.api(), a, b, c, d), map[string]string{"Accept": "application/geo+json"})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Panoramax: %s", resp.Status)
	}
	var r pxSearch
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&r); err != nil {
		return nil, fmt.Errorf("Panoramax: unreadable answer: %w", err)
	}
	var cand []found
	for _, f := range r.Features {
		if len(f.Geometry.Coordinates) < 2 {
			continue
		}
		href := f.Assets["sd"].Href
		if href == "" {
			href = f.Assets["hd"].Href
		}
		if !strings.HasPrefix(href, "https://") {
			continue
		}
		by := "Panoramax"
		if len(f.Providers) > 0 && f.Providers[0].Name != "" {
			by = f.Providers[0].Name + " on Panoramax"
		}
		lic := f.Properties.License
		if lic == "" {
			lic = "CC-BY-SA-4.0"
		}
		cand = append(cand, found{
			View:   View{Provider: "Panoramax", Kind: Street, Azimuth: f.Properties.Azimuth, Captured: parseTime(f.Properties.Datetime), Attribution: "© " + by, License: lic},
			target: href, lat: f.Geometry.Coordinates[1], lon: f.Geometry.Coordinates[0],
		})
	}
	return streetPick(cand, lat, lon, 2, 8), nil
}

func (p *panoramax) fetch(ctx context.Context, f found) (*http.Response, error) {
	return p.s.get(ctx, f.target, nil)
}

// --- Mapillary: street-level photos (Graph API v4, CC-BY-SA) ---

const mapillaryAPI = "https://graph.mapillary.com"

type mapillary struct {
	s *Service
	c MapillaryConfig
}

func (m *mapillary) name() string { return "Mapillary" }

type mlImages struct {
	Data []struct {
		ID         string   `json:"id"`
		CapturedAt int64    `json:"captured_at"` // milliseconds
		Compass    *float64 `json:"compass_angle"`
		Geometry   struct {
			Coordinates []float64 `json:"coordinates"`
		} `json:"computed_geometry"`
		Thumb   string `json:"thumb_1024_url"`
		Creator struct {
			Username string `json:"username"`
		} `json:"creator"`
	} `json:"data"`
}

func (m *mapillary) find(ctx context.Context, lat, lon float64) ([]found, error) {
	a, b, c, d := box(lat, lon, streetRadius)
	q := url.Values{}
	q.Set("bbox", fmt.Sprintf("%.6f,%.6f,%.6f,%.6f", a, b, c, d))
	q.Set("fields", "id,captured_at,compass_angle,computed_geometry,thumb_1024_url,creator")
	q.Set("limit", "100")
	// The token goes in a header, not the address, so it is not written to anyone's logs.
	resp, err := m.s.get(ctx, m.s.host("mapillary", mapillaryAPI)+"/images?"+q.Encode(), map[string]string{"Authorization": "OAuth " + m.c.Token})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, errors.New("Mapillary refused the token")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Mapillary: %s", resp.Status)
	}
	var r mlImages
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&r); err != nil {
		return nil, fmt.Errorf("Mapillary: unreadable answer: %w", err)
	}
	var cand []found
	for _, im := range r.Data {
		if len(im.Geometry.Coordinates) < 2 || !strings.HasPrefix(im.Thumb, "https://") {
			continue
		}
		by := "Mapillary"
		if im.Creator.Username != "" {
			by = im.Creator.Username + " on Mapillary"
		}
		v := View{Provider: "Mapillary", Kind: Street, Azimuth: im.Compass, Attribution: "© " + by, License: "CC-BY-SA-4.0"}
		if im.CapturedAt > 0 {
			v.Captured = time.UnixMilli(im.CapturedAt).UTC()
		}
		cand = append(cand, found{View: v, target: im.Thumb, lat: im.Geometry.Coordinates[1], lon: im.Geometry.Coordinates[0]})
	}
	return streetPick(cand, lat, lon, 2, 8), nil
}

func (m *mapillary) fetch(ctx context.Context, f found) (*http.Response, error) {
	return m.s.get(ctx, f.target, nil)
}
