package gis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/mapdata"
)

var area = mapdata.Area{Name: "T", Lat: 37.27, Lon: -79.94, RadiusM: 5000}

// fakeArcGIS serves 1,203 address points (more than one batch) and answers like an ArcGIS layer.
func fakeArcGIS(t *testing.T) *httptest.Server {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch {
		case strings.HasSuffix(r.URL.Path, "/FeatureServer/0") && r.Form.Get("f") == "json":
			fmt.Fprint(w, `{"name":"Address Points","geometryType":"esriGeometryPoint","fields":[{"name":"OBJECTID"},{"name":"ADDRNUM"},{"name":"FULLNAME"},{"name":"FULLADDR"},{"name":"MSAG_COMMUNITY"},{"name":"ZIP_5"}]}`)
		case r.Form.Get("returnIdsOnly") == "true":
			ids := []int{}
			for i := 1; i <= 1203; i++ {
				ids = append(ids, i)
			}
			json.NewEncoder(w).Encode(map[string]any{"objectIds": ids})
		case r.Form.Get("returnCountOnly") == "true":
			fmt.Fprint(w, `{"count":1203}`)
		case r.Form.Get("objectIds") != "":
			var feats []map[string]any
			for _, id := range strings.Split(r.Form.Get("objectIds"), ",") {
				var n int
				fmt.Sscan(id, &n)
				feats = append(feats, map[string]any{
					"attributes": map[string]any{"OBJECTID": n, "ADDRNUM": fmt.Sprint(100 + n), "FULLNAME": "KING GEORGE AVE SW", "FULLADDR": fmt.Sprintf("%d KING GEORGE AVE SW", 100+n), "MSAG_COMMUNITY": "ROANOKE", "ZIP_5": 24016},
					"geometry":   map[string]any{"x": -79.94 + float64(n)*0.00001, "y": 37.27},
				})
			}
			json.NewEncoder(w).Encode(map[string]any{"features": feats})
		default:
			fmt.Fprint(w, `{"features":[]}`)
		}
	}))
	t.Cleanup(ts.Close)
	client = ts.Client()
	allowLocal = true
	t.Cleanup(func() { allowLocal = false })
	return ts
}

func TestImportArcGISAndGeoJSON(t *testing.T) {
	ts := fakeArcGIS(t)
	ctx := context.Background()
	info, err := InspectArcGIS(ctx, "test", ts.URL+"/arcgis/rest/services/X/FeatureServer/0", area)
	if err != nil || info.Kind != mapdata.GISAddress || info.Guess["num"] != "ADDRNUM" || info.Guess["street"] != "FULLNAME" || info.Guess["city"] != "MSAG_COMMUNITY" || info.Count != 1203 {
		t.Fatalf("inspect %+v %v", info, err)
	}
	dir := t.TempDir()
	s, err := NewService(dir, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	// Hydrants and a zone from a GeoJSON upload.
	up, _ := s.UploadPath("hydrants.geojson")
	os.WriteFile(up, []byte(`{"type":"FeatureCollection","features":[
		{"type":"Feature","properties":{"HYDRANT_ID":"H-17","FLOW":"1500 gpm"},"geometry":{"type":"Point","coordinates":[-79.941,37.271]}},
		{"type":"Feature","properties":{"HYDRANT_ID":"H-far"},"geometry":{"type":"Point","coordinates":[-70,40]}},
		{"type":"Feature","properties":{"NAME":"ESZ 4","ESN":"104"},"geometry":{"type":"Polygon","coordinates":[[[-79.95,37.26],[-79.93,37.26],[-79.93,37.28],[-79.95,37.28],[-79.95,37.26]]]}}]}`), 0o600)
	gi, err := InspectGeoJSON(up)
	if err != nil || gi.Count != 3 || gi.Kind != mapdata.GISHydrant || gi.Guess["name"] != "HYDRANT_ID" {
		t.Fatalf("geojson %+v %v", gi, err)
	}
	layers := []Layer{
		{ID: "addr", Name: "Address points", Kind: mapdata.GISAddress, Source: "arcgis", URL: info.Name, Fields: info.Guess, On: true},
		{ID: "hyd", Name: "Hydrants", Kind: mapdata.GISHydrant, Source: "geojson", File: "hydrants.geojson", Fields: map[string]string{"name": "HYDRANT_ID", "detail": "FLOW"}, On: true},
		{ID: "esz", Name: "ESZ", Kind: mapdata.GISZone, Source: "geojson", File: "hydrants.geojson", Fields: map[string]string{"name": "NAME", "detail": "ESN"}, On: true},
	}
	layers[0].URL = ts.URL + "/arcgis/rest/services/X/FeatureServer/0"
	if err := s.SaveLayers(layers); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveLayers([]Layer{{ID: "x", Kind: "address", Source: "arcgis", URL: "http://example.com/FeatureServer/0"}}); err == nil {
		t.Fatal("plain http accepted")
	}
	if err := s.Import(area, "admin"); err != nil {
		t.Fatal(err)
	}
	var j *Job
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if j = s.Job(); j.Finished != nil {
			break
		}
	}
	if j.Error != "" || j.Stats == nil || j.Stats.ByKind["address"] != 1203 || j.Stats.ByKind["hydrant"] != 1 || j.Stats.ByKind["zone"] != 1 {
		t.Fatalf("job %+v stats %+v", j, j.Stats)
	}
	st := s.Store()
	res, err := st.Search(ctx, "415 king george ave sw", 0, 0, 3)
	if err != nil || len(res) == 0 || !res[0].Exact || res[0].Label != "415 King George Ave SW" {
		t.Fatalf("search %+v %v", res, err)
	}
	// A restart opens the newest store.
	s2, _ := NewService(dir, "test", nil)
	t.Cleanup(func() { s2.Close() })
	if s2.Store() == nil || len(s2.Layers()) != 3 {
		t.Fatal("not reopened")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "gis-*.db"))
	if len(files) != 1 {
		t.Fatalf("stores %v", files)
	}
}

func TestMapFullAddress(t *testing.T) {
	l := Layer{Kind: mapdata.GISAddress, Fields: map[string]string{"full": "SITEADDR"}}
	g := Map(l, Feature{Props: map[string]any{"SITEADDR": "2001A CRYSTAL SPRING AVE SW"}})
	if g.Num != "2001A" || g.Street != "CRYSTAL SPRING AVE SW" {
		t.Fatalf("%+v", g)
	}
	if _, err := CheckURL("https://169.254.169.254/latest/FeatureServer/0"); err == nil {
		t.Fatal("link-local accepted")
	}
}
