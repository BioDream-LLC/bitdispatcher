package mapdata

import (
	"context"
	"path/filepath"
	"testing"
)

// Two zones side by side; the west one has a hole that belongs to neither.
func TestZones(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gis.db")
	area := Area{Name: "test", Lat: 37.27, Lon: -79.94, RadiusM: 5000}
	sq := func(w, s, e, n float64) [][2]float64 { return [][2]float64{{w, s}, {e, s}, {e, n}, {w, n}, {w, s}} }
	_, err := BuildGIS(context.Background(), path, area, "test", func(emit func(GISFeature)) error {
		emit(GISFeature{Kind: GISZone, Name: "Beat 1", Polys: [][][][2]float64{{sq(-79.96, 37.26, -79.94, 37.28), sq(-79.952, 37.268, -79.948, 37.272)}}})
		emit(GISFeature{Kind: GISZone, Name: "Beat 2", Polys: [][][][2]float64{{sq(-79.94, 37.26, -79.92, 37.28)}}})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	zs, err := st.Zones()
	if err != nil || len(zs) != 2 {
		t.Fatalf("zones %v %v", zs, err)
	}
	for _, c := range []struct {
		lat, lon float64
		want     string
	}{{37.27, -79.957, "Beat 1"}, {37.27, -79.93, "Beat 2"}, {37.27, -79.95, ""}, {37.30, -79.95, ""}} {
		if got := st.ZoneAt(c.lat, c.lon); got != c.want {
			t.Errorf("%v,%v: %q, want %q", c.lat, c.lon, got, c.want)
		}
	}
	gj := ZonesGeoJSON(zs)
	f := gj["features"].([]any)[0].(map[string]any)
	mp := f["geometry"].(map[string]any)["coordinates"].([][][][2]float64)
	if len(mp) != 1 || len(mp[0]) != 2 {
		t.Fatalf("Beat 1 is one polygon with one hole: %d", len(mp))
	}
}
