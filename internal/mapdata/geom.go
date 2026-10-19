package mapdata

import (
	"encoding/binary"
	"errors"
	"math"
)

// Pt is a point in 1e-7 degrees, OpenStreetMap's precision (about 1 cm).
type Pt struct{ Lon, Lat int32 }

// Deg returns the point in degrees.
func (p Pt) Deg() (lat, lon float64) { return float64(p.Lat) / 1e7, float64(p.Lon) / 1e7 }

// PtDeg makes a point from degrees.
func PtDeg(lat, lon float64) Pt {
	return Pt{Lon: int32(math.Round(lon * 1e7)), Lat: int32(math.Round(lat * 1e7))}
}

// Geometry kinds.
const (
	GPoint   = 1
	GLine    = 2 // one or more line strings
	GPolygon = 3 // one or more polygons, each an outer ring then its holes
)

// Geom is a feature's shape. For GPoint, Parts[0][0]; for GLine, each part is a line; for GPolygon, Polys.
type Geom struct {
	Kind  byte
	Parts [][]Pt   // points and lines
	Polys [][][]Pt // polygons: rings, the first outer
}

// BBox is a box in degrees.
type BBox struct{ MinLon, MinLat, MaxLon, MaxLat float64 }

func emptyBox() BBox { return BBox{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)} }

func (b *BBox) add(p Pt) {
	lat, lon := p.Deg()
	b.MinLon, b.MaxLon = math.Min(b.MinLon, lon), math.Max(b.MaxLon, lon)
	b.MinLat, b.MaxLat = math.Min(b.MinLat, lat), math.Max(b.MaxLat, lat)
}

// Box is the geometry's bounding box.
func (g *Geom) Box() BBox {
	b := emptyBox()
	for _, part := range g.Parts {
		for _, p := range part {
			b.add(p)
		}
	}
	for _, poly := range g.Polys {
		if len(poly) > 0 {
			for _, p := range poly[0] {
				b.add(p)
			}
		}
	}
	return b
}

// Encode is the store's compact format: a kind byte, then counts and zigzag-delta varints of the
// coordinates. A typical road segment takes 3 or 4 bytes a point.
func (g *Geom) Encode() []byte {
	out := make([]byte, 0, 64)
	out = append(out, g.Kind)
	var last Pt
	pts := func(r []Pt) {
		out = binary.AppendUvarint(out, uint64(len(r)))
		for _, p := range r {
			out = binary.AppendVarint(out, int64(p.Lon)-int64(last.Lon))
			out = binary.AppendVarint(out, int64(p.Lat)-int64(last.Lat))
			last = p
		}
	}
	switch g.Kind {
	case GPoint, GLine:
		out = binary.AppendUvarint(out, uint64(len(g.Parts)))
		for _, part := range g.Parts {
			pts(part)
		}
	case GPolygon:
		out = binary.AppendUvarint(out, uint64(len(g.Polys)))
		for _, poly := range g.Polys {
			out = binary.AppendUvarint(out, uint64(len(poly)))
			for _, ring := range poly {
				pts(ring)
			}
		}
	}
	return out
}

var errGeom = errors.New("mapdata: malformed geometry")

// DecodeGeom reads Encode's format.
func DecodeGeom(b []byte) (Geom, error) {
	if len(b) < 2 {
		return Geom{}, errGeom
	}
	g := Geom{Kind: b[0]}
	i := 1
	uv := func() (uint64, bool) {
		v, n := binary.Uvarint(b[i:])
		if n <= 0 {
			return 0, false
		}
		i += n
		return v, true
	}
	var last Pt
	pts := func() ([]Pt, bool) {
		n, ok := uv()
		if !ok || n > uint64(len(b)) {
			return nil, false
		}
		r := make([]Pt, n)
		for k := range r {
			dx, n1 := binary.Varint(b[i:])
			if n1 <= 0 {
				return nil, false
			}
			i += n1
			dy, n2 := binary.Varint(b[i:])
			if n2 <= 0 {
				return nil, false
			}
			i += n2
			last = Pt{Lon: int32(int64(last.Lon) + dx), Lat: int32(int64(last.Lat) + dy)}
			r[k] = last
		}
		return r, true
	}
	switch g.Kind {
	case GPoint, GLine:
		n, ok := uv()
		if !ok || n > uint64(len(b)) {
			return Geom{}, errGeom
		}
		for range n {
			r, ok := pts()
			if !ok {
				return Geom{}, errGeom
			}
			g.Parts = append(g.Parts, r)
		}
	case GPolygon:
		n, ok := uv()
		if !ok || n > uint64(len(b)) {
			return Geom{}, errGeom
		}
		for range n {
			nr, ok := uv()
			if !ok || nr > uint64(len(b)) {
				return Geom{}, errGeom
			}
			poly := make([][]Pt, 0, nr)
			for range nr {
				r, ok := pts()
				if !ok {
					return Geom{}, errGeom
				}
				poly = append(poly, r)
			}
			g.Polys = append(g.Polys, poly)
		}
	default:
		return Geom{}, errGeom
	}
	return g, nil
}

const earthR = 6371008.8

// metresPerDeg is the length of a degree of latitude.
const metresPerDeg = earthR * math.Pi / 180

// Distance is the great-circle distance in metres between two points in degrees.
func Distance(lat1, lon1, lat2, lon2 float64) float64 {
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthR * math.Asin(math.Min(1, math.Sqrt(a)))
}

func ptDist(a, b Pt) float64 {
	la, loa := a.Deg()
	lb, lob := b.Deg()
	return Distance(la, loa, lb, lob)
}

// lineLength is a line's length in metres.
func lineLength(r []Pt) float64 {
	m := 0.0
	for i := 1; i < len(r); i++ {
		m += ptDist(r[i-1], r[i])
	}
	return m
}

// ringArea is a ring's area in square metres (unsigned), on a local flat projection.
func ringArea(r []Pt) float64 {
	if len(r) < 3 {
		return 0
	}
	lat0 := float64(r[0].Lat) / 1e7
	kx := metresPerDeg * math.Cos(lat0*math.Pi/180) / 1e7
	ky := metresPerDeg / 1e7
	s := 0.0
	for i := range r {
		j := (i + 1) % len(r)
		s += float64(r[i].Lon)*kx*float64(r[j].Lat)*ky - float64(r[j].Lon)*kx*float64(r[i].Lat)*ky
	}
	return math.Abs(s) / 2
}

// polyArea is the area of polygons in square metres: outer rings less holes.
func polyArea(polys [][][]Pt) float64 {
	a := 0.0
	for _, p := range polys {
		for i, r := range p {
			if i == 0 {
				a += ringArea(r)
			} else {
				a -= ringArea(r)
			}
		}
	}
	return math.Max(0, a)
}

// labelPoint is where a polygon's name goes: the centroid of its largest outer ring when that falls
// inside, otherwise a point on the ring's middle row that does.
func labelPoint(polys [][][]Pt) Pt {
	best, bestA := -1, -1.0
	for i, p := range polys {
		if len(p) > 0 {
			if a := ringArea(p[0]); a > bestA {
				best, bestA = i, a
			}
		}
	}
	if best < 0 {
		return Pt{}
	}
	ring := polys[best][0]
	var cx, cy, a float64
	for i := range ring {
		j := (i + 1) % len(ring)
		x0, y0 := float64(ring[i].Lon), float64(ring[i].Lat)
		x1, y1 := float64(ring[j].Lon), float64(ring[j].Lat)
		f := x0*y1 - x1*y0
		cx += (x0 + x1) * f
		cy += (y0 + y1) * f
		a += f
	}
	if a != 0 {
		c := Pt{Lon: int32(cx / (3 * a)), Lat: int32(cy / (3 * a))}
		if inRing(c, ring) {
			return c
		}
	}
	// Scan the horizontal line through the box's middle for the widest inside span.
	b := emptyBox()
	for _, p := range ring {
		b.add(p)
	}
	y := (b.MinLat + b.MaxLat) / 2
	var xs []float64
	for i := range ring {
		j := (i + 1) % len(ring)
		la, loa := ring[i].Deg()
		lb, lob := ring[j].Deg()
		if (la > y) != (lb > y) {
			xs = append(xs, loa+(y-la)/(lb-la)*(lob-loa))
		}
	}
	sortFloats(xs)
	bx, bw := (b.MinLon+b.MaxLon)/2, -1.0
	for k := 0; k+1 < len(xs); k += 2 {
		if w := xs[k+1] - xs[k]; w > bw {
			bx, bw = (xs[k]+xs[k+1])/2, w
		}
	}
	return PtDeg(y, bx)
}

func sortFloats(a []float64) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

// inRing is the even-odd point-in-polygon test.
func inRing(p Pt, r []Pt) bool {
	in := false
	for i, j := 0, len(r)-1; i < len(r); j, i = i, i+1 {
		a, b := r[i], r[j]
		if (a.Lat > p.Lat) != (b.Lat > p.Lat) {
			x := float64(a.Lon) + float64(p.Lat-a.Lat)/float64(b.Lat-a.Lat)*float64(b.Lon-a.Lon)
			if float64(p.Lon) < x {
				in = !in
			}
		}
	}
	return in
}

// ---- Web Mercator tile arithmetic ----

// TileBox is a tile's bounds in degrees.
func TileBox(z, x, y int) BBox {
	n := math.Exp2(float64(z))
	lon0 := float64(x)/n*360 - 180
	lon1 := float64(x+1)/n*360 - 180
	lat0 := math.Atan(math.Sinh(math.Pi*(1-2*float64(y+1)/n))) * 180 / math.Pi
	lat1 := math.Atan(math.Sinh(math.Pi*(1-2*float64(y)/n))) * 180 / math.Pi
	return BBox{lon0, lat0, lon1, lat1}
}

// mercX and mercY are a point's position in the world at zoom 0, from 0 to 1.
func mercX(lon float64) float64 { return (lon + 180) / 360 }
func mercY(lat float64) float64 {
	lat = math.Max(-85.0511, math.Min(85.0511, lat))
	s := math.Sin(lat * math.Pi / 180)
	return 0.5 - math.Log((1+s)/(1-s))/(4*math.Pi)
}

// ---- clipping and simplification in tile coordinates ----

type fpt struct{ x, y float64 }

// simplify is Douglas-Peucker with tolerance tol, keeping the ends.
func simplify(p []fpt, tol float64) []fpt {
	if len(p) < 3 || tol <= 0 {
		return p
	}
	keep := make([]bool, len(p))
	keep[0], keep[len(p)-1] = true, true
	type span struct{ a, b int }
	stack := []span{{0, len(p) - 1}}
	t2 := tol * tol
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		best, bi := -1.0, -1
		for i := s.a + 1; i < s.b; i++ {
			if d := segDist2(p[i], p[s.a], p[s.b]); d > best {
				best, bi = d, i
			}
		}
		if best > t2 {
			keep[bi] = true
			stack = append(stack, span{s.a, bi}, span{bi, s.b})
		}
	}
	out := p[:0:0]
	for i, k := range keep {
		if k {
			out = append(out, p[i])
		}
	}
	return out
}

func segDist2(p, a, b fpt) float64 {
	dx, dy := b.x-a.x, b.y-a.y
	if dx == 0 && dy == 0 {
		return (p.x-a.x)*(p.x-a.x) + (p.y-a.y)*(p.y-a.y)
	}
	t := ((p.x-a.x)*dx + (p.y-a.y)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	x, y := a.x+t*dx, a.y+t*dy
	return (p.x-x)*(p.x-x) + (p.y-y)*(p.y-y)
}

// clipLine cuts a line to the square [lo,hi]², returning the pieces inside.
func clipLine(p []fpt, lo, hi float64) [][]fpt {
	var out [][]fpt
	var cur []fpt
	flush := func() {
		if len(cur) >= 2 {
			out = append(out, cur)
		}
		cur = nil
	}
	for i := 1; i < len(p); i++ {
		a, b, ok := clipSeg(p[i-1], p[i], lo, hi)
		if !ok {
			flush()
			continue
		}
		if len(cur) == 0 || cur[len(cur)-1] != a {
			flush()
			cur = append(cur, a)
		}
		cur = append(cur, b)
		if b != p[i] {
			flush()
		}
	}
	flush()
	return out
}

// clipSeg is Liang-Barsky.
func clipSeg(a, b fpt, lo, hi float64) (fpt, fpt, bool) {
	t0, t1 := 0.0, 1.0
	dx, dy := b.x-a.x, b.y-a.y
	for _, e := range [4][2]float64{{-dx, a.x - lo}, {dx, hi - a.x}, {-dy, a.y - lo}, {dy, hi - a.y}} {
		pp, q := e[0], e[1]
		if pp == 0 {
			if q < 0 {
				return a, b, false
			}
			continue
		}
		r := q / pp
		if pp < 0 {
			if r > t1 {
				return a, b, false
			}
			t0 = math.Max(t0, r)
		} else {
			if r < t0 {
				return a, b, false
			}
			t1 = math.Min(t1, r)
		}
	}
	na, nb := a, b
	if t0 > 0 {
		na = fpt{a.x + t0*dx, a.y + t0*dy}
	}
	if t1 < 1 {
		nb = fpt{a.x + t1*dx, a.y + t1*dy}
	}
	return na, nb, true
}

// clipRing is Sutherland-Hodgman against the square [lo,hi]².
func clipRing(r []fpt, lo, hi float64) []fpt {
	edges := []func(p fpt) bool{
		func(p fpt) bool { return p.x >= lo },
		func(p fpt) bool { return p.x <= hi },
		func(p fpt) bool { return p.y >= lo },
		func(p fpt) bool { return p.y <= hi },
	}
	cross := []func(a, b fpt) fpt{
		func(a, b fpt) fpt { t := (lo - a.x) / (b.x - a.x); return fpt{lo, a.y + t*(b.y-a.y)} },
		func(a, b fpt) fpt { t := (hi - a.x) / (b.x - a.x); return fpt{hi, a.y + t*(b.y-a.y)} },
		func(a, b fpt) fpt { t := (lo - a.y) / (b.y - a.y); return fpt{a.x + t*(b.x-a.x), lo} },
		func(a, b fpt) fpt { t := (hi - a.y) / (b.y - a.y); return fpt{a.x + t*(b.x-a.x), hi} },
	}
	out := r
	for k := range edges {
		if len(out) == 0 {
			break
		}
		in := out
		out = make([]fpt, 0, len(in)+4)
		for i := range in {
			cur, prev := in[i], in[(i+len(in)-1)%len(in)]
			ci, pi := edges[k](cur), edges[k](prev)
			if ci {
				if !pi {
					out = append(out, cross[k](prev, cur))
				}
				out = append(out, cur)
			} else if pi {
				out = append(out, cross[k](prev, cur))
			}
		}
	}
	return out
}
