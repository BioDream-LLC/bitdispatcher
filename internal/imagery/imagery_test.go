package imagery

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func photo(c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for y := range 256 {
		for x := range 256 {
			// A gradient, so it is not mistaken for a blank tile.
			img.Set(x, y, color.RGBA{c.R, uint8(x), uint8(y), 255})
		}
	}
	var b bytes.Buffer
	jpeg.Encode(&b, img, nil)
	return b.Bytes()
}

func TestRecordAndServe(t *testing.T) {
	var vginHits, usgsHits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "test-agent" {
			t.Error("no User-Agent")
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/vgin/"):
			vginHits.Add(1)
			if strings.Contains(r.URL.Path, "/17/") { // the state service has a gap at zoom 17
				http.NotFound(w, r)
				return
			}
			w.Write(photo(color.RGBA{R: 200}))
		case strings.HasPrefix(r.URL.Path, "/usgs/"):
			usgsHits.Add(1)
			w.Write(photo(color.RGBA{R: 10}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	s, err := NewService(t.TempDir(), "test-agent")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.base = map[string]string{"vgin": ts.URL + "/vgin/{z}/{y}/{x}", "usgs": ts.URL + "/usgs/{z}/{y}/{x}"}
	p := Plan{Lat: 37.27, Lon: -79.94, RadiusM: 1500, CoreRadiusM: 300, OuterZoom: 16, CoreZoom: 18, Rate: 1000}
	if err := s.Record(p, "admin"); err != nil {
		t.Fatal(err)
	}
	var j *Job
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if j = s.Job(); j.Finished != nil {
			break
		}
	}
	if j.Finished == nil || j.Error != "" {
		t.Fatalf("job %+v", j)
	}
	if j.Done != j.Plan.Tiles && j.Done < j.Plan.Tiles/2 {
		t.Fatalf("done %d of %d", j.Done, j.Plan.Tiles)
	}
	st, _ := s.Store().Stats(context.Background())
	if st.ByZoom[18] == 0 || st.ByZoom[16] == 0 || st.MaxZoom != 18 {
		t.Fatalf("stats %+v", st)
	}
	// Zoom 17 came from USGS? No: USGS stops at 16, so zoom 17 has no photo and is kept as empty.
	if st.ByZoom[17] != 0 || st.Empty == 0 {
		t.Fatalf("zoom 17 %+v", st)
	}
	if usgsHits.Load() != 0 {
		t.Fatalf("USGS asked %d times though VGIN had the tiles", usgsHits.Load())
	}
	// Recording again fetches nothing new.
	before := vginHits.Load()
	s.Record(p, "admin")
	for s.Job().Finished == nil {
		time.Sleep(10 * time.Millisecond)
	}
	if vginHits.Load() != before {
		t.Fatalf("recording again fetched %d tiles", vginHits.Load()-before)
	}
	if s.Last() == nil {
		t.Fatal("finished plan not kept")
	}

	ctx := context.Background()
	x, y := tileXY(37.27, -79.94, 18)
	if b, err := s.Tile(ctx, 18, x, y); err != nil || !bytes.Equal(b[:2], []byte{0xff, 0xd8}) {
		t.Fatalf("recorded tile: %v", err)
	}
	// Zoom 17 is empty, zoom 19 was not recorded: both are made from a lower level.
	x, y = tileXY(37.27, -79.94, 17)
	if b, err := s.Tile(ctx, 17, x, y); err != nil || len(b) < 1000 {
		t.Fatalf("zoom 17 from zoom 16: %v", err)
	}
	x, y = tileXY(37.27, -79.94, 20)
	b, err := s.Tile(ctx, 20, x, y)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil || img.Bounds().Dx() != 256 {
		t.Fatalf("enlarged tile: %v", err)
	}
	// Far away, offline: nothing.
	if _, err := s.Tile(ctx, 18, 10, 10); err != ErrNoTile {
		t.Fatalf("far tile: %v", err)
	}
	// Online: fetched on demand and kept.
	s.Online = func() bool { return true }
	x, y = tileXY(37.5, -79.0, 15)
	if _, err := s.Tile(ctx, 15, x, y); err != nil {
		t.Fatal(err)
	}
	if !s.Store().has(ctx, 15, x, y) {
		t.Fatal("online tile not kept")
	}
}

func TestPlan(t *testing.T) {
	p, err := MakePlan(Plan{Lat: 37.27, Lon: -79.94, RadiusM: 160934, CoreRadiusM: 16093, OuterZoom: 16, CoreZoom: 19})
	if err != nil {
		t.Fatal(err)
	}
	if p.Sources[0] != "vgin" || p.Sources[1] != "usgs" {
		t.Fatalf("sources %v", p.Sources)
	}
	// About 340,000 tiles at zoom 16 over the 100 miles and 220,000 at zoom 19 within 10 miles.
	if p.Tiles < 500000 || p.Tiles > 900000 {
		t.Fatalf("tiles %d", p.Tiles)
	}
	n := len(Circle(37.27, -79.94, 16093, 15))
	if c := countCircle(37.27, 16093, 15); float64(c) < float64(n)*0.85 || float64(c) > float64(n)*1.2 {
		t.Fatalf("estimate %d, listed %d", c, n)
	}
	if _, err := MakePlan(Plan{Lat: 0, Lon: 0, RadiusM: 10000, OuterZoom: 16, CoreZoom: 16}); err == nil {
		t.Fatal("no source covers the Gulf of Guinea")
	}
}

func TestBlank(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	var b bytes.Buffer
	jpeg.Encode(&b, img, nil)
	if !blank(b.Bytes()) {
		t.Fatal("white tile not blank")
	}
	if blank(photo(color.RGBA{R: 1})) {
		t.Fatal("photo blank")
	}
}
