package mapdata

// The road overlay: closures and traffic laid over the recorded road network. Each road segment has a
// factor on its recorded driving time: 1 as recorded, 2 twice as slow, Closed not driven at all. The
// router reads the overlay on every search, so a closure or a traffic jam changes drive times and unit
// recommendations at once, with no rebuild of the map. A unit can still reach a place on a closed road:
// the segments where a drive starts and ends are never refused, only priced.

import (
	"math"
	"sync/atomic"
)

// Closed is the factor of a road that is not driven.
var Closed = float32(math.Inf(1))

// overlay is one set of factors, with the version that set it.
type overlay struct {
	f       []float32
	version uint64
}

var overlayVersions atomic.Uint64

// SetFactors replaces the overlay: one factor per segment (see Segments), nil for none. It returns the
// overlay's version, which changes every time.
func (r *Router) SetFactors(f []float32) uint64 {
	if f != nil && len(f) != len(r.g.Segs) {
		f = nil
	}
	v := overlayVersions.Add(1)
	r.over.Store(&overlay{f: f, version: v})
	return v
}

// OverlayVersion is the version of the overlay in force; drive-time trees made under another are stale.
func (r *Router) OverlayVersion() uint64 {
	if o := r.over.Load(); o != nil {
		return o.version
	}
	return 0
}

// Segments is how many road segments the network has.
func (r *Router) Segments() int { return len(r.g.Segs) }

func (r *Router) factors() []float32 {
	if o := r.over.Load(); o != nil {
		return o.f
	}
	return nil
}

// cost is a segment's driving time under the overlay; ends says the drive starts or ends on it, so a
// closure does not refuse it.
func cost(f []float32, si uint32, secs float32, ends bool) float32 {
	if f == nil {
		return secs
	}
	k := f[si]
	if k == Closed {
		if ends {
			return secs
		}
		return inf
	}
	if k <= 0 {
		return secs
	}
	return secs * k
}

// SegInfo describes a segment for the overlay's makers.
type SegInfo struct {
	Name   string
	Metres float64
	Kmh    float64 // the recorded speed
}

// Seg describes one segment.
func (r *Router) Seg(i uint32) SegInfo {
	s := r.g.Segs[i]
	kmh := 0.0
	if s.Secs > 0 {
		kmh = float64(s.Metres) / float64(s.Secs) * 3.6
	}
	return SegInfo{Name: r.g.Names[s.Name], Metres: float64(s.Metres), Kmh: kmh}
}

// segMid is the point halfway along a segment's shape (by its points), and the segment's bearing in
// degrees from its first point to its last.
func (r *Router) segMid(i uint32) (lat, lon, bearing float64) {
	s := r.g.Segs[i]
	pts := r.g.Pts[s.Shape : s.Shape+s.N]
	total := r.flatLen(s)
	half, run := total/2, 0.0
	lat, lon = pts[0].Deg()
	for k := 0; k+1 < len(pts); k++ {
		alat, alon := pts[k].Deg()
		blat, blon := pts[k+1].Deg()
		kx := metresPerDeg * math.Cos(alat*math.Pi/180)
		d := math.Hypot((blon-alon)*kx, (blat-alat)*metresPerDeg)
		if run+d >= half && d > 0 {
			t := (half - run) / d
			lat, lon = alat+t*(blat-alat), alon+t*(blon-alon)
			break
		}
		run += d
		lat, lon = blat, blon
	}
	flat, flon := pts[0].Deg()
	llat, llon := pts[len(pts)-1].Deg()
	bearing = bearingDeg(flat, flon, llat, llon)
	return lat, lon, bearing
}

func bearingDeg(alat, alon, blat, blon float64) float64 {
	kx := math.Cos(alat * math.Pi / 180)
	b := math.Atan2((blon-alon)*kx, blat-alat) * 180 / math.Pi
	if b < 0 {
		b += 360
	}
	return b
}

// distToLine is the distance in metres from a point to a polyline of [lon, lat] points, and the
// bearing of the polyline's piece nearest it.
func distToLine(lat, lon float64, line [][2]float64) (float64, float64) {
	kx := metresPerDeg * math.Cos(lat*math.Pi/180)
	best, bearing := math.Inf(1), 0.0
	if len(line) == 1 {
		return math.Hypot((line[0][0]-lon)*kx, (line[0][1]-lat)*metresPerDeg), 0
	}
	for k := 0; k+1 < len(line); k++ {
		ax, ay := (line[k][0]-lon)*kx, (line[k][1]-lat)*metresPerDeg
		bx, by := (line[k+1][0]-lon)*kx, (line[k+1][1]-lat)*metresPerDeg
		dx, dy := bx-ax, by-ay
		l2 := dx*dx + dy*dy
		t := 0.0
		if l2 > 0 {
			t = math.Max(0, math.Min(1, -(ax*dx+ay*dy)/l2))
		}
		if d := math.Hypot(ax+t*dx, ay+t*dy); d < best {
			best, bearing = d, bearingDeg(line[k][1], line[k][0], line[k+1][1], line[k+1][0])
		}
	}
	return best, bearing
}

// parallel reports whether two bearings run the same way along a road (either direction), within tol degrees.
func parallel(a, b, tol float64) bool {
	d := math.Mod(math.Abs(a-b), 180)
	return d <= tol || 180-d <= tol
}

// candidates are the segments with a point in the grid cells around a box, with a margin in metres.
func (r *Router) candidates(minLat, minLon, maxLat, maxLon, margin float64) []uint32 {
	dLat := margin / metresPerDeg
	dLon := margin / (metresPerDeg * math.Max(0.1, math.Cos(minLat*math.Pi/180)))
	lo := r.cellOf(PtDeg(minLat-dLat, minLon-dLon))
	hi := r.cellOf(PtDeg(maxLat+dLat, maxLon+dLon))
	if int64(hi[0]-lo[0]+1)*int64(hi[1]-lo[1]+1) > 250000 { // more than about a 250 km square: refuse
		return nil
	}
	seen := map[uint32]bool{}
	var out []uint32
	for x := lo[0]; x <= hi[0]; x++ {
		for y := lo[1]; y <= hi[1]; y++ {
			for _, si := range r.grid[[2]int32{x, y}] {
				if !seen[si] {
					seen[si] = true
					out = append(out, si)
				}
			}
		}
	}
	return out
}

// AlongLine are the segments that run along a drawn line (of [lon, lat] points): their midpoint is
// within buffer metres of it and they run the same way, so the side streets crossing it are left out.
// A short segment (under 40 m, such as a junction's stub) only needs to be within the buffer.
func (r *Router) AlongLine(line [][2]float64, buffer float64) []uint32 {
	if len(line) == 0 {
		return nil
	}
	minLat, minLon, maxLat, maxLon := line[0][1], line[0][0], line[0][1], line[0][0]
	for _, p := range line {
		minLat, maxLat = math.Min(minLat, p[1]), math.Max(maxLat, p[1])
		minLon, maxLon = math.Min(minLon, p[0]), math.Max(maxLon, p[0])
	}
	var out []uint32
	for _, si := range r.candidates(minLat, minLon, maxLat, maxLon, buffer+200) {
		lat, lon, b := r.segMid(si)
		d, lb := distToLine(lat, lon, line)
		if d > buffer {
			continue
		}
		if len(line) == 1 || r.g.Segs[si].Metres < 40 || parallel(b, lb, 35) {
			out = append(out, si)
		}
	}
	return out
}

// InCircle are the segments with any part of their shape within radius metres of a point: an area
// closed by a flood, a fire or a scene.
func (r *Router) InCircle(lat, lon, radius float64) []uint32 {
	var out []uint32
	for _, si := range r.candidates(lat, lon, lat, lon, radius+200) {
		s := r.g.Segs[si]
		pts := r.g.Pts[s.Shape : s.Shape+s.N]
		line := make([][2]float64, len(pts))
		for k, p := range pts {
			plat, plon := p.Deg()
			line[k] = [2]float64{plon, plat}
		}
		if d, _ := distToLine(lat, lon, line); d <= radius {
			out = append(out, si)
		}
	}
	return out
}

// Shape is a segment's points as [lon, lat], for drawing.
func (r *Router) Shape(i uint32) [][2]float64 {
	s := r.g.Segs[i]
	pts := r.g.Pts[s.Shape : s.Shape+s.N]
	out := make([][2]float64, len(pts))
	for k, p := range pts {
		lat, lon := p.Deg()
		out[k] = [2]float64{lon, lat}
	}
	return out
}
