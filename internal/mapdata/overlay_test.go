package mapdata

import (
	"slices"
	"testing"
)

// A closure along Main Street sends the drive around it on Low Road; the side streets crossing it stay
// open; a slowdown changes the time but not the way; an area closure takes every road in it; and a drive
// that starts or ends on the closed road still gets there.
func TestOverlay(t *testing.T) {
	s := buildTown(t)
	r, err := s.Router()
	if err != nil {
		t.Fatal(err)
	}
	drive := func(fromLat, fromLon, toLat, toLon float64) Route {
		t.Helper()
		rt, ok := r.To(toLat, toLon, 3600).From(fromLat, fromLon, false)
		if !ok {
			t.Fatalf("no drive from %v,%v to %v,%v", fromLat, fromLon, toLat, toLon)
		}
		return rt
	}
	open := drive(37.27, -79.95, 37.27, -79.93) // along Main Street
	if open.Metres > 1900 {
		t.Fatalf("Main Street is about 1770 m, the drive is %.0f", open.Metres)
	}

	main := r.AlongLine([][2]float64{{-79.948, 37.27}, {-79.932, 37.27}}, 25)
	names := map[string]bool{}
	for _, si := range main {
		names[r.Seg(si).Name] = true
	}
	if !names["Main Street"] || names["2nd Street"] {
		t.Fatalf("a line along Main Street closes %v", names)
	}
	f := make([]float32, r.Segments())
	for i := range f {
		f[i] = 1
	}
	for _, si := range main {
		f[si] = Closed
	}
	v0 := r.OverlayVersion()
	if v := r.SetFactors(f); v == v0 {
		t.Fatal("the overlay's version did not change")
	}
	// Through traffic goes around: from 1st Street at Main to 3rd Street at Main, via Low Road or Oak Avenue.
	around := drive(37.265, -79.95, 37.275, -79.93)
	if around.Metres < open.Metres+500 {
		t.Errorf("with Main Street closed the drive is %.0f m; the open drive was %.0f", around.Metres, open.Metres)
	}
	// A unit sent to a house on the closed street still gets a drive.
	if _, ok := r.To(37.27, -79.945, 3600).From(37.26, -79.95, false); !ok {
		t.Error("a place on the closed road could not be reached")
	}

	// Traffic: twice as slow along Main Street, same distance.
	for _, si := range main {
		f[si] = 2
	}
	r.SetFactors(slices.Clone(f))
	slow := drive(37.27, -79.95, 37.27, -79.93)
	if slow.Seconds < open.Seconds*1.5 || slow.Metres > open.Metres+50 {
		t.Errorf("slow %+v, open %+v", slow, open)
	}

	// An area closure around the middle junction takes the roads meeting there.
	mid := r.InCircle(37.27, -79.94, 60)
	if len(mid) < 4 {
		t.Errorf("a 60 m circle at the middle junction holds %d segments, want the 4 meeting there", len(mid))
	}
	r.SetFactors(nil)
	if back := drive(37.27, -79.95, 37.27, -79.93); back.Seconds != open.Seconds {
		t.Errorf("without the overlay the drive is %v s, was %v", back.Seconds, open.Seconds)
	}
}
