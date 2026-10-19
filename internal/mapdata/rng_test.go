package mapdata

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// A centreline running east along latitude 37.27 numbered 100-198 on the left (north) and 101-199 on
// the right: 150 is halfway along, on the north side; a point just south of the line near its east end
// is about 195.
func TestAddressRanges(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "gis.db")
	area := Area{Name: "test", Lat: 37.27, Lon: -79.94, RadiusM: 5000}
	line := [][2]float64{{-79.950, 37.27}, {-79.945, 37.27}, {-79.940, 37.27}}
	_, err := BuildGIS(ctx, path, area, "test", func(emit func(GISFeature)) error {
		emit(GISFeature{Kind: GISRoad, Name: "ELM ST SW", Lines: [][][2]float64{line}, LF: 100, LT: 198, RF: 101, RT: 199, City: "ROANOKE", CityR: "SALEM", Zip: "24016", ZipR: "24153"})
		emit(GISFeature{Kind: GISAddress, Num: "120", Street: "ELM ST SW", Point: &[2]float64{-79.948, 37.2701}})
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
	// A point on file is still the point.
	rs, _ := st.Search(ctx, "120 elm st", 0, 0, 5)
	if len(rs) == 0 || !rs[0].Exact || rs[0].Ranged {
		t.Fatalf("on file: %+v", rs)
	}
	rs, err = st.Search(ctx, "150 elm st", 0, 0, 5)
	if err != nil || len(rs) == 0 || !rs[0].Ranged || rs[0].Exact || rs[0].Label != "150 Elm St SW" || rs[0].Detail != "Roanoke, 24016" {
		t.Fatalf("ranged: %+v %v", rs, err)
	}
	if d := Distance(rs[0].Lat, rs[0].Lon, 37.27, -79.945); d > 15 || rs[0].Lat <= 37.27 {
		t.Fatalf("150 should be halfway, on the north side: %+v (%.0f m)", rs[0], d)
	}
	// Odd numbers are on the right, in the other town; a town narrows it.
	rs, _ = st.Search(ctx, "151 elm st salem", 0, 0, 5)
	if len(rs) == 0 || !rs[0].Ranged || rs[0].Lat >= 37.27 || !strings.HasPrefix(rs[0].Detail, "Salem") {
		t.Fatalf("right side: %+v", rs)
	}
	if rs, _ := st.Search(ctx, "151 elm st roanoke", 0, 0, 5); len(rs) > 0 && rs[0].Ranged {
		t.Fatalf("151 is not in Roanoke: %+v", rs)
	}
	if rs, _ := st.Search(ctx, "500 elm st", 0, 0, 5); len(rs) > 0 && rs[0].Ranged {
		t.Fatalf("500 is beyond the range: %+v", rs)
	}
	r, err := st.Reverse(ctx, 37.26995, -79.9405)
	if err != nil || !r.Ranged || r.Label != "195 Elm St SW" {
		t.Fatalf("reverse: %+v %v", r, err)
	}
}
