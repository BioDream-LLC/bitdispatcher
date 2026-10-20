package views

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Roanoke's municipal building, roughly.
const lat, lon = 37.2710, -79.9414

func fakeProviders(t *testing.T) (*Service, *httptest.Server, *int) {
	t.Helper()
	tokens := 0
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth2/v1/token", func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "cid" || p != "secret" {
			w.WriteHeader(401)
			return
		}
		if r.FormValue("grant_type") != "client_credentials" {
			w.WriteHeader(400)
			return
		}
		tokens++
		fmt.Fprint(w, `{"access_token":"tok","token_type":"Bearer","expires_in":3600}`)
	})
	mux.HandleFunc("POST /imagery/v3/discovery/rank/location", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		var b struct {
			Center struct {
				Point struct {
					GeoJSON struct{ Value, EPSG string } `json:"geojson"`
				} `json:"point"`
				Radius float64 `json:"radius_in_meters"`
			} `json:"center"`
			View struct {
				Obliques struct{ Cardinals map[string]bool } `json:"obliques"`
			} `json:"view"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b.Center.Radius == 0 || !strings.Contains(b.Center.Point.GeoJSON.Value, "-79.9414") || !b.View.Obliques.Cardinals["west"] {
			t.Errorf("rank request: %v %+v", err, b)
		}
		fmt.Fprint(w, `{"captures":[{"capture":{"urn":"cap1","start_date":"2025-03-02"},
		  "obliques":{"north":{"images":[{"urn":"urn:n","shot_time":"2025-03-02T15:04:05Z","look_at":{"azimuth":2}}]},
		              "east":{"images":[{"urn":"urn:e"}]},"south":{"images":[]},"west":{"images":[{"urn":"urn:w"}]}},
		  "orthos":{"images":[{"urn":"urn:o"}]}}]}`)
	})
	mux.HandleFunc("GET /imagery/v3/images/{urn}/location", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" || r.URL.Query().Get("center.radius") == "" || r.URL.Query().Get("format") != "IMAGE_FORMAT_JPEG" {
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		fmt.Fprint(w, "JPEG:"+r.PathValue("urn"))
	})
	var srv *httptest.Server
	mux.HandleFunc("GET /api/search", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("bbox") == "" {
			w.WriteHeader(400)
			return
		}
		// Three photos: one across the street looking at the place, one nearby looking away, one too far.
		fmt.Fprintf(w, `{"features":[
		  {"id":"a","geometry":{"coordinates":[-79.9414,37.27125]},"properties":{"datetime":"2025-11-13T16:34:23Z","view:azimuth":180,"license":"CC-BY-SA-4.0"},
		   "providers":[{"name":"slinky"}],"assets":{"sd":{"href":"%[1]s/pic/a.jpg"}}},
		  {"id":"b","geometry":{"coordinates":[-79.9414,37.27125]},"properties":{"view:azimuth":0},"assets":{"sd":{"href":"%[1]s/pic/b.jpg"}}},
		  {"id":"c","geometry":{"coordinates":[-79.9414,37.2730]},"properties":{"view:azimuth":180},"assets":{"sd":{"href":"%[1]s/pic/c.jpg"}}}]}`, srv.URL)
	})
	mux.HandleFunc("GET /images", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "OAuth MLY|1" || r.URL.Query().Get("access_token") != "" {
			w.WriteHeader(401)
			return
		}
		fmt.Fprintf(w, `{"data":[{"id":"m1","captured_at":1700000000000,"compass_angle":90,"computed_geometry":{"coordinates":[-79.94160,37.2710]},
		  "thumb_1024_url":"%s/pic/m1.jpg","creator":{"username":"rider"}}]}`, srv.URL)
	})
	mux.HandleFunc("GET /pic/{name}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		fmt.Fprint(w, "PIC:"+r.PathValue("name"))
	})
	srv = httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	s := &Service{Client: srv.Client(), Agent: "test", Hosts: map[string]string{
		"eagleview": srv.URL, "eagleview-token": srv.URL + "/oauth2/v1/token", "panoramax": srv.URL, "mapillary": srv.URL,
	}}
	return s, srv, &tokens
}

func picture(t *testing.T, s *Service, ref string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	if err := s.Picture(context.Background(), ref, rec); err != nil {
		t.Fatalf("picture %s: %v", ref, err)
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "image/") || !strings.HasPrefix(rec.Header().Get("Cache-Control"), "private") {
		t.Errorf("headers %v", rec.Header())
	}
	b, _ := io.ReadAll(rec.Body)
	return string(b)
}

func TestEagleViewObliques(t *testing.T) {
	s, _, tokens := fakeProviders(t)
	s.Configure(Config{EagleView: EagleViewConfig{On: true, ClientID: "cid", ClientSecret: "secret"}, GoogleLink: true})
	r := s.Find(context.Background(), lat, lon)
	if len(r.Problems) > 0 {
		t.Fatal(r.Problems)
	}
	var got []string
	for _, v := range r.Views {
		got = append(got, v.Kind+":"+v.Facing)
	}
	if strings.Join(got, ",") != "oblique:N,oblique:E,oblique:W,ortho:" {
		t.Fatalf("views %v", got)
	}
	if r.Views[0].Captured.Format("2006-01-02") != "2025-03-02" || r.Views[1].Captured.IsZero() || r.Views[0].Azimuth == nil {
		t.Errorf("dates and azimuth %+v %+v", r.Views[0], r.Views[1])
	}
	if !strings.Contains(r.Google, "viewpoint=37.271000,-79.941400") {
		t.Error("google link", r.Google)
	}
	if b := picture(t, s, r.Views[2].Ref); b != "JPEG:urn:w" {
		t.Errorf("picture %q", b)
	}
	s.Find(context.Background(), lat, lon)
	if *tokens != 1 {
		t.Errorf("token fetched %d times, want once and reused", *tokens)
	}
}

func TestEagleViewBadSecret(t *testing.T) {
	s, _, _ := fakeProviders(t)
	s.Configure(Config{EagleView: EagleViewConfig{On: true, ClientID: "cid", ClientSecret: "wrong"}})
	r := s.Find(context.Background(), lat, lon)
	if !strings.Contains(r.Problems["EagleView"], "refused") || len(r.Views) != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestStreetPhotosLookAtThePlace(t *testing.T) {
	s, _, _ := fakeProviders(t)
	s.Configure(Config{Panoramax: PanoramaxConfig{On: true}, Mapillary: MapillaryConfig{On: true, Token: "MLY|1"}})
	r := s.Find(context.Background(), lat, lon)
	if len(r.Problems) > 0 {
		t.Fatal(r.Problems)
	}
	var got []string
	for _, v := range r.Views {
		got = append(got, fmt.Sprintf("%s:%s:%.0f", v.Provider, v.Facing, v.Distance))
	}
	// a looks south at the place from 28 m north; b looks away; c is 222 m off. m1 looks east from 18 m west.
	if strings.Join(got, ",") != "Mapillary:E:18,Panoramax:S:28" {
		t.Fatalf("views %v", got)
	}
	if r.Views[1].Attribution != "© slinky on Panoramax" || r.Views[1].License != "CC-BY-SA-4.0" || r.Views[0].Attribution != "© rider on Mapillary" {
		t.Errorf("attribution %+v", r.Views)
	}
	if b := picture(t, s, r.Views[1].Ref); b != "PIC:a.jpg" {
		t.Errorf("picture %q", b)
	}
}

func TestOnlyReferencesItMade(t *testing.T) {
	s, srv, _ := fakeProviders(t)
	s.Configure(Config{Panoramax: PanoramaxConfig{On: true}})
	rec := httptest.NewRecorder()
	if err := s.Picture(context.Background(), srv.URL+"/pic/a.jpg", rec); err != ErrUnknown {
		t.Fatalf("a made-up reference was served: %v", err)
	}
	if r := (&Service{}).Find(context.Background(), lat, lon); len(r.Views) != 0 || r.Google != "" {
		t.Errorf("nothing on, yet %+v", r)
	}
}

func TestPublicClientRefusesTheLocalNetwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "inside") }))
	defer srv.Close()
	if _, err := publicClient().Get(srv.URL); err == nil || !strings.Contains(err.Error(), "not on the internet") {
		t.Fatalf("reached %s: %v", srv.URL, err)
	}
}

func TestCompass(t *testing.T) {
	for az, want := range map[float64]string{0: "N", 44: "N", 46: "E", 180: "S", 271: "W", 359: "N"} {
		if c := compass(az); c != want {
			t.Errorf("compass(%v) = %s, want %s", az, c, want)
		}
	}
}
