package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (c *client) raw(method, path string, body []byte) (int, []byte) {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.base+path, bytes.NewReader(body))
	req.Header.Set(csrfHeader, "1")
	resp, err := c.c.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func TestViewsConfigAndLookup(t *testing.T) {
	ts, s, pw := newServer(t)
	admin := login(t, ts, "admin", pw["admin"])
	d1 := login(t, ts, "d1", pw["d1"])

	// A fake Panoramax with one photo across the street, looking at the place.
	var px *httptest.Server
	px = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/search" {
			fmt.Fprintf(w, `{"features":[{"id":"a","geometry":{"coordinates":[-79.9414,37.27125]},"properties":{"view:azimuth":180},
			  "providers":[{"name":"someone"}],"assets":{"sd":{"href":"%s/pic.jpg"}}}]}`, px.URL)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		fmt.Fprint(w, "THE PICTURE")
	}))
	defer px.Close()
	s.Views.Client, s.Views.Hosts = px.Client(), map[string]string{"panoramax": px.URL}

	// Nothing is on until an administrator turns it on.
	if code, b := d1.do("GET", "/api/views?lat=37.2710&lon=-79.9414", nil, false); code != 200 || !strings.Contains(string(b), `"views":[]`) {
		t.Fatalf("off: %d %s", code, b)
	}
	if code, _ := d1.do("PUT", "/api/admin/views", map[string]any{"panoramaxOn": true}, true); code != 403 {
		t.Fatalf("a dispatcher changed the views: %d", code)
	}
	if code, b := admin.do("PUT", "/api/admin/views", map[string]any{"eagleviewOn": true, "eagleviewClientId": "cid"}, true); code != 422 {
		t.Fatalf("EagleView without its secret: %d %s", code, b)
	}
	code, b := admin.do("PUT", "/api/admin/views", map[string]any{"panoramaxOn": true, "googleLink": true, "mapillaryToken": "MLY|secret",
		"eagleviewClientId": "cid", "eagleviewSecret": "shh"}, true)
	if code != 200 || strings.Contains(string(b), "shh") || strings.Contains(string(b), "MLY|secret") || !strings.Contains(string(b), `"eagleviewSecretSet":true`) {
		t.Fatalf("save: %d %s", code, b)
	}
	if code, b := admin.do("POST", "/api/admin/views/test", map[string]float64{"lat": 37.2710, "lon": -79.9414}, true); code != 200 || !strings.Contains(string(b), "1 pictures") {
		t.Fatalf("test: %d %s", code, b)
	}

	code, b = d1.do("GET", "/api/views?lat=37.2710&lon=-79.9414", nil, false)
	var res struct {
		Views []struct {
			Ref, Provider, Kind, Facing, Attribution string
		}
		Google string
	}
	json.Unmarshal(b, &res)
	if code != 200 || len(res.Views) != 1 || res.Views[0].Facing != "S" || res.Views[0].Attribution != "© someone on Panoramax" || res.Google == "" {
		t.Fatalf("lookup: %d %s", code, b)
	}
	if code, b := d1.do("GET", "/api/views/picture/"+res.Views[0].Ref, nil, false); code != 200 || string(b) != "THE PICTURE" {
		t.Fatalf("picture: %d %s", code, b)
	}
	if code, _ := d1.do("GET", "/api/views/picture/made-up", nil, false); code != 404 {
		t.Fatalf("made-up reference: %d", code)
	}
	if code, _ := d1.do("GET", "/api/views?lat=x&lon=1", nil, false); code != 422 {
		t.Fatalf("bad place: %d", code)
	}
}

func TestPremisePhotos(t *testing.T) {
	ts, s, pw := newServer(t)
	admin := login(t, ts, "admin", pw["admin"])
	d1 := login(t, ts, "d1", pw["d1"])
	v1 := login(t, ts, "v1", pw["v1"])
	code, b := admin.do("PUT", "/api/premises", map[string]any{"address": "1 Pre Plan Rd", "lat": 37.27, "lon": -79.94, "hazards": []string{"dog"}}, true)
	if code != 200 {
		t.Fatalf("premise: %d %s", code, b)
	}
	var p struct{ ID string }
	json.Unmarshal(b, &p)
	if p.ID == "" {
		t.Fatalf("no id in %s", b)
	}
	jpeg := append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F', 0}, bytes.Repeat([]byte{1}, 200)...)

	if code, _ := v1.raw("POST", "/api/premises/"+p.ID+"/photos", jpeg); code != 403 {
		t.Fatalf("a viewer added a picture: %d", code)
	}
	if code, b := d1.raw("POST", "/api/premises/"+p.ID+"/photos?caption=Back+door&facing=n", []byte("not a picture at all")); code != 415 {
		t.Fatalf("text taken as a picture: %d %s", code, b)
	}
	code, b = d1.raw("POST", "/api/premises/"+p.ID+"/photos?caption=Back+door&facing=n", jpeg)
	var got struct {
		Photos []struct{ ID, Caption, Facing, Type, SHA256, By string }
	}
	json.Unmarshal(b, &got)
	if code != 200 || len(got.Photos) != 1 || got.Photos[0].Facing != "N" || got.Photos[0].Type != "image/jpeg" || got.Photos[0].By != "d1" {
		t.Fatalf("add: %d %s", code, b)
	}
	ph := got.Photos[0].ID
	if code, b := v1.do("GET", "/api/premises/"+p.ID+"/photos/"+ph, nil, false); code != 200 || !bytes.Equal(b, jpeg) {
		t.Fatalf("view: %d", code)
	}

	// Saving the premise again (an edit, an import) keeps its pictures.
	if code, b := admin.do("PUT", "/api/premises", map[string]any{"id": p.ID, "address": "1 Pre Plan Rd", "lat": 37.27, "lon": -79.94, "notes": "gate code 1234"}, true); code != 200 {
		t.Fatalf("edit: %d %s", code, b)
	}
	if pr := s.Eng.PremiseByID(p.ID); pr == nil || len(pr.Photos) != 1 {
		t.Fatalf("the edit dropped the picture: %+v", pr)
	}

	// A file changed on disk is refused, not shown.
	path := filepath.Join(s.DataDir, "premise-photos", p.ID, ph)
	os.WriteFile(path, append(jpeg, 9), 0o640)
	if code, _ := v1.do("GET", "/api/premises/"+p.ID+"/photos/"+ph, nil, false); code != 409 {
		t.Fatalf("a changed file was served: %d", code)
	}

	if code, _ := d1.do("DELETE", "/api/premises/"+p.ID+"/photos/"+ph, nil, true); code != 403 {
		t.Fatalf("a dispatcher removed a picture: %d", code)
	}
	if code, b := admin.do("DELETE", "/api/premises/"+p.ID+"/photos/"+ph, nil, true); code != 200 {
		t.Fatalf("remove: %d %s", code, b)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the file stayed after the picture was removed")
	}
}
