package mapdata

import (
	"math"
	"slices"
	"sync"
)

// Router finds driving times over the recorded road network.
type Router struct {
	g     *Graph
	inOff []uint32 // per junction, into inSeg
	inSeg []uint32 // segment | 1<<31 when driven V→U
	grid  map[[2]int32][]uint32
	cell  float64
	pool  sync.Pool
}

const revBit = 1 << 31

func newRouter(g *Graph) *Router {
	r := &Router{g: g, cell: 0.005}
	count := make([]uint32, len(g.Verts)+1)
	for _, s := range g.Segs {
		if s.Fwd {
			count[s.V]++
		}
		if s.Bwd {
			count[s.U]++
		}
	}
	r.inOff = make([]uint32, len(g.Verts)+1)
	for i := range g.Verts {
		r.inOff[i+1] = r.inOff[i] + count[i]
	}
	r.inSeg = make([]uint32, r.inOff[len(g.Verts)])
	fill := slices.Clone(r.inOff[:len(g.Verts)])
	for i, s := range g.Segs {
		if s.Fwd {
			r.inSeg[fill[s.V]] = uint32(i)
			fill[s.V]++
		}
		if s.Bwd {
			r.inSeg[fill[s.U]] = uint32(i) | revBit
			fill[s.U]++
		}
	}
	r.grid = map[[2]int32][]uint32{}
	for i, s := range g.Segs {
		var last [2]int32
		for k := s.Shape; k < s.Shape+s.N; k++ {
			c := r.cellOf(g.Pts[k])
			if k > s.Shape && c == last {
				continue
			}
			last = c
			l := r.grid[c]
			if len(l) == 0 || l[len(l)-1] != uint32(i) {
				r.grid[c] = append(l, uint32(i))
			}
		}
	}
	return r
}

func (r *Router) cellOf(p Pt) [2]int32 {
	lat, lon := p.Deg()
	return [2]int32{int32(math.Floor(lon / r.cell)), int32(math.Floor(lat / r.cell))}
}

// Snap is a point's nearest place on the road network.
type Snap struct {
	Seg   uint32
	Along float64 // metres from the segment's U end
	Off   float64 // metres from the point to the road
	At    Pt
	OK    bool
}

// snap finds the nearest road within about 2 km.
func (r *Router) snap(lat, lon float64) Snap {
	c := r.cellOf(PtDeg(lat, lon))
	kx := metresPerDeg * math.Cos(lat*math.Pi/180)
	best := Snap{Off: math.Inf(1)}
	seen := map[uint32]bool{}
	for ring := int32(0); ring <= 4; ring++ {
		for dx := -ring; dx <= ring; dx++ {
			for dy := -ring; dy <= ring; dy++ {
				if max(abs32(dx), abs32(dy)) != ring {
					continue
				}
				for _, si := range r.grid[[2]int32{c[0] + dx, c[1] + dy}] {
					if seen[si] {
						continue
					}
					seen[si] = true
					s := r.g.Segs[si]
					along := 0.0
					for k := s.Shape; k+1 < s.Shape+s.N; k++ {
						a, b := r.g.Pts[k], r.g.Pts[k+1]
						alat, alon := a.Deg()
						blat, blon := b.Deg()
						ax, ay := (alon-lon)*kx, (alat-lat)*metresPerDeg
						bx, by := (blon-lon)*kx, (blat-lat)*metresPerDeg
						dxs, dys := bx-ax, by-ay
						l2 := dxs*dxs + dys*dys
						t := 0.0
						if l2 > 0 {
							t = math.Max(0, math.Min(1, -(ax*dxs+ay*dys)/l2))
						}
						px, py := ax+t*dxs, ay+t*dys
						d := math.Hypot(px, py)
						segLen := math.Sqrt(l2)
						if d < best.Off {
							best = Snap{Seg: si, Along: along + t*segLen, Off: d, OK: true,
								At: PtDeg(lat+py/metresPerDeg, lon+px/kx)}
						}
						along += segLen
					}
				}
			}
		}
		// Once something is found, one more ring settles which is nearest.
		if best.OK && best.Off < float64(ring)*r.cell*metresPerDeg*0.6 {
			break
		}
	}
	if best.OK {
		// Along was measured on a flat approximation; scale it to the stored length.
		s := r.g.Segs[best.Seg]
		flat := r.flatLen(s)
		if flat > 0 {
			best.Along = best.Along / flat * float64(s.Metres)
		}
	}
	return best
}

func (r *Router) flatLen(s Seg) float64 {
	m := 0.0
	for k := s.Shape; k+1 < s.Shape+s.N; k++ {
		a, b := r.g.Pts[k], r.g.Pts[k+1]
		alat, alon := a.Deg()
		blat, blon := b.Deg()
		kx := metresPerDeg * math.Cos(alat*math.Pi/180)
		m += math.Hypot((blon-alon)*kx, (blat-alat)*metresPerDeg)
	}
	return m
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// offRoadKmh is the speed assumed for the distance between a point and its road, such as up a driveway.
const offRoadKmh = 15.0

// Tree is every junction's driving time to one destination, from a search backwards from it.
type Tree struct {
	r      *Router
	dest   Snap
	secs   []float32
	metres []float32
	next   []uint32 // the segment to drive next, with revBit; terminal when term
	term   []bool
	Max    float64
}

const inf = float32(math.MaxFloat32)

// To builds the tree for a destination, searching up to maxSecs of driving.
func (r *Router) To(lat, lon, maxSecs float64) *Tree {
	d := r.snap(lat, lon)
	n := len(r.g.Verts)
	t := &Tree{r: r, dest: d, Max: maxSecs, secs: make([]float32, n), metres: make([]float32, n), next: make([]uint32, n), term: make([]bool, n)}
	for i := range t.secs {
		t.secs[i] = inf
	}
	if !d.OK {
		return t
	}
	var h heap
	s := r.g.Segs[d.Seg]
	frac := 0.0
	if s.Metres > 0 {
		frac = d.Along / float64(s.Metres)
	}
	if s.Fwd { // from U, drive along the segment to the point
		t.secs[s.U], t.metres[s.U], t.term[s.U] = float32(frac*float64(s.Secs)), float32(d.Along), true
		h.push(t.secs[s.U], s.U)
	}
	if s.Bwd {
		c := float32((1 - frac) * float64(s.Secs))
		if c < t.secs[s.V] {
			t.secs[s.V], t.metres[s.V], t.term[s.V] = c, float32(float64(s.Metres)-d.Along), true
			h.push(c, s.V)
		}
	}
	for h.len() > 0 {
		c, v := h.pop()
		if c > t.secs[v] || float64(c) > maxSecs {
			continue
		}
		for _, e := range r.inSeg[r.inOff[v]:r.inOff[v+1]] {
			sg := r.g.Segs[e&^revBit]
			from := sg.U
			if e&revBit != 0 {
				from = sg.V
			}
			nc := c + sg.Secs
			if nc < t.secs[from] {
				t.secs[from], t.metres[from], t.next[from], t.term[from] = nc, t.metres[v]+sg.Metres, e, false
				h.push(nc, from)
			}
		}
	}
	return t
}

// Route is a drive from one point to the tree's destination.
type Route struct {
	Seconds float64
	Metres  float64
	Path    []Pt
	Via     []string // the main roads, in order
}

// From is the drive from a point to the destination, if the road network connects them.
func (t *Tree) From(lat, lon float64, withPath bool) (Route, bool) {
	if !t.dest.OK {
		return Route{}, false
	}
	o := t.r.snap(lat, lon)
	if !o.OK {
		return Route{}, false
	}
	g := t.r.g
	s := g.Segs[o.Seg]
	frac := 0.0
	if s.Metres > 0 {
		frac = o.Along / float64(s.Metres)
	}
	best := math.Inf(1)
	var bestM float64
	var start uint32
	startVia := -1 // 0: along the segment to V, 1: to U, 2: same segment straight to the destination
	if s.Fwd && t.secs[s.V] < inf {
		if c := (1-frac)*float64(s.Secs) + float64(t.secs[s.V]); c < best {
			best, bestM, start, startVia = c, float64(s.Metres)-o.Along+float64(t.metres[s.V]), s.V, 0
		}
	}
	if s.Bwd && t.secs[s.U] < inf {
		if c := frac*float64(s.Secs) + float64(t.secs[s.U]); c < best {
			best, bestM, start, startVia = c, o.Along+float64(t.metres[s.U]), s.U, 1
		}
	}
	if o.Seg == t.dest.Seg {
		df := t.dest.Along / math.Max(float64(s.Metres), 1e-9)
		if (s.Fwd && df >= frac) || (s.Bwd && df <= frac) {
			if c := math.Abs(df-frac) * float64(s.Secs); c <= best {
				best, bestM, startVia = c, math.Abs(t.dest.Along-o.Along), 2
			}
		}
	}
	if math.IsInf(best, 1) {
		return Route{}, false
	}
	off := o.Off + t.dest.Off
	rt := Route{Seconds: best + off/(offRoadKmh/3.6), Metres: bestM + off}
	if !withPath {
		return rt, true
	}
	rt.Path = append(rt.Path, PtDeg(lat, lon), o.At)
	// Via lists the roads that carry most of the drive, in order.
	var order []uint32
	length := map[uint32]float64{}
	via := func(n uint32, m float64) {
		if g.Names[n] == "" {
			return
		}
		if _, ok := length[n]; !ok {
			order = append(order, n)
		}
		length[n] += m
	}
	finish := func() (Route, bool) {
		for _, n := range order {
			if length[n] >= 0.15*rt.Metres && len(rt.Via) < 3 {
				rt.Via = append(rt.Via, g.Names[n])
			}
		}
		return rt, true
	}
	via(s.Name, 0)
	switch startVia {
	case 2:
		via(s.Name, rt.Metres)
		rt.Path = append(rt.Path, t.r.partial(o.Seg, o.Along, t.dest.Along)...)
		rt.Path = append(rt.Path, t.dest.At)
		return finish()
	case 0:
		rt.Path = append(rt.Path, t.r.partial(o.Seg, o.Along, float64(s.Metres))...)
	case 1:
		rt.Path = append(rt.Path, t.r.partial(o.Seg, o.Along, 0)...)
	}
	rt.Path = append(rt.Path, g.Verts[start]) // partial leaves out the junction itself
	v := start
	for guard := 0; !t.term[v] && guard < len(g.Verts); guard++ {
		e := t.next[v]
		sg := g.Segs[e&^revBit]
		pts := g.Pts[sg.Shape : sg.Shape+sg.N]
		if e&revBit != 0 {
			for k := len(pts) - 1; k >= 0; k-- {
				rt.Path = append(rt.Path, pts[k])
			}
			v = sg.U
		} else {
			rt.Path = append(rt.Path, pts...)
			v = sg.V
		}
		via(sg.Name, float64(sg.Metres))
	}
	ds := g.Segs[t.dest.Seg]
	if v == ds.U {
		rt.Path = append(rt.Path, t.r.partial(t.dest.Seg, 0, t.dest.Along)...)
	} else {
		rt.Path = append(rt.Path, t.r.partial(t.dest.Seg, float64(ds.Metres), t.dest.Along)...)
	}
	via(ds.Name, 0)
	rt.Path = append(rt.Path, t.dest.At)
	return finish()
}

// partial is the stretch of a segment's shape between two distances from its U end, in that direction.
func (r *Router) partial(si uint32, from, to float64) []Pt {
	s := r.g.Segs[si]
	pts := r.g.Pts[s.Shape : s.Shape+s.N]
	scale := 1.0
	if fl := r.flatLen(s); fl > 0 {
		scale = float64(s.Metres) / fl
	}
	// Distances of each shape point from U.
	d := make([]float64, len(pts))
	for k := 1; k < len(pts); k++ {
		alat, alon := pts[k-1].Deg()
		blat, blon := pts[k].Deg()
		kx := metresPerDeg * math.Cos(alat*math.Pi/180)
		d[k] = d[k-1] + math.Hypot((blon-alon)*kx, (blat-alat)*metresPerDeg)*scale
	}
	var out []Pt
	if from <= to {
		for k := range pts {
			if d[k] > from && d[k] < to {
				out = append(out, pts[k])
			}
		}
	} else {
		for k := len(pts) - 1; k >= 0; k-- {
			if d[k] < from && d[k] > to {
				out = append(out, pts[k])
			}
		}
	}
	return out
}

// heap is a binary min-heap of (cost, junction).
type heap struct {
	c []float32
	v []uint32
}

func (h *heap) len() int { return len(h.c) }

func (h *heap) push(c float32, v uint32) {
	h.c = append(h.c, c)
	h.v = append(h.v, v)
	i := len(h.c) - 1
	for i > 0 {
		p := (i - 1) / 2
		if h.c[p] <= h.c[i] {
			break
		}
		h.c[p], h.c[i] = h.c[i], h.c[p]
		h.v[p], h.v[i] = h.v[i], h.v[p]
		i = p
	}
}

func (h *heap) pop() (float32, uint32) {
	c, v := h.c[0], h.v[0]
	n := len(h.c) - 1
	h.c[0], h.v[0] = h.c[n], h.v[n]
	h.c, h.v = h.c[:n], h.v[:n]
	i := 0
	for {
		l, r, m := 2*i+1, 2*i+2, i
		if l < n && h.c[l] < h.c[m] {
			m = l
		}
		if r < n && h.c[r] < h.c[m] {
			m = r
		}
		if m == i {
			break
		}
		h.c[m], h.c[i] = h.c[i], h.c[m]
		h.v[m], h.v[i] = h.v[i], h.v[m]
		i = m
	}
	return c, v
}

// NearestRoad is the name of the road nearest a point and how far it is.
func (r *Router) NearestRoad(lat, lon float64) (string, float64, bool) {
	s := r.snap(lat, lon)
	if !s.OK {
		return "", 0, false
	}
	return r.g.Names[r.g.Segs[s.Seg].Name], s.Off, true
}
