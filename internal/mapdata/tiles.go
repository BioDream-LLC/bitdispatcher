package mapdata

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"sort"
	"strconv"
)

// Extent is the vector tile grid: coordinates run 0..Extent across a tile.
const Extent = 4096

// MaxZoom is the deepest tile the store makes; the map scales these tiles beyond it, and they hold every
// feature, so house numbers and hydrants still appear at street level.
const MaxZoom = 15

const buffer = 80 // tile units drawn beyond the edge, so lines and labels meet cleanly between tiles

// ---- Mapbox Vector Tile encoding (https://github.com/mapbox/vector-tile-spec, version 2) ----

type mvtLayer struct {
	name     string
	keys     []string
	keyIdx   map[string]uint32
	vals     []string
	valIdx   map[string]uint32
	features [][]byte
}

func newLayer(name string) *mvtLayer {
	return &mvtLayer{name: name, keyIdx: map[string]uint32{}, valIdx: map[string]uint32{}}
}

func (l *mvtLayer) tag(k, v string) (uint32, uint32) {
	ki, ok := l.keyIdx[k]
	if !ok {
		ki = uint32(len(l.keys))
		l.keyIdx[k] = ki
		l.keys = append(l.keys, k)
	}
	vi, ok := l.valIdx[v]
	if !ok {
		vi = uint32(len(l.vals))
		l.valIdx[v] = vi
		l.vals = append(l.vals, v)
	}
	return ki, vi
}

func pbKey(b []byte, field, wt int) []byte { return binary.AppendUvarint(b, uint64(field<<3|wt)) }

func pbBytes(b []byte, field int, v []byte) []byte {
	b = pbKey(b, field, 2)
	b = binary.AppendUvarint(b, uint64(len(v)))
	return append(b, v...)
}

func pbPacked(b []byte, field int, v []uint32) []byte {
	var p []byte
	for _, x := range v {
		p = binary.AppendUvarint(p, uint64(x))
	}
	return pbBytes(b, field, p)
}

func zz(v int32) uint32 { return uint32((v << 1) ^ (v >> 31)) }

// add encodes one feature. props are written in sorted key order so tiles are reproducible.
func (l *mvtLayer) add(id uint64, typ int, geom []uint32, props map[string]string) {
	if len(geom) == 0 {
		return
	}
	keys := make([]string, 0, len(props))
	for k, v := range props {
		if v != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	tags := make([]uint32, 0, len(keys)*2)
	for _, k := range keys {
		ki, vi := l.tag(k, props[k])
		tags = append(tags, ki, vi)
	}
	var f []byte
	if id != 0 {
		f = pbKey(f, 1, 0)
		f = binary.AppendUvarint(f, id)
	}
	if len(tags) > 0 {
		f = pbPacked(f, 2, tags)
	}
	f = pbKey(f, 3, 0)
	f = binary.AppendUvarint(f, uint64(typ))
	f = pbPacked(f, 4, geom)
	l.features = append(l.features, f)
}

func (l *mvtLayer) encode() []byte {
	var b []byte
	b = pbKey(b, 15, 0)
	b = binary.AppendUvarint(b, 2)
	b = pbBytes(b, 1, []byte(l.name))
	for _, f := range l.features {
		b = pbBytes(b, 2, f)
	}
	for _, k := range l.keys {
		b = pbBytes(b, 3, []byte(k))
	}
	for _, v := range l.vals {
		b = pbBytes(b, 4, pbBytes(nil, 1, []byte(v)))
	}
	b = pbKey(b, 5, 0)
	b = binary.AppendUvarint(b, Extent)
	return b
}

// geomEncoder writes MoveTo/LineTo/ClosePath commands with the cursor carried between parts.
type geomEncoder struct {
	cmds   []uint32
	cx, cy int32
}

func (e *geomEncoder) moveTo(x, y int32) {
	e.cmds = append(e.cmds, 1|1<<3, zz(x-e.cx), zz(y-e.cy))
	e.cx, e.cy = x, y
}

func (e *geomEncoder) lineTo(pts [][2]int32) {
	e.cmds = append(e.cmds, 2|uint32(len(pts))<<3)
	for _, p := range pts {
		e.cmds = append(e.cmds, zz(p[0]-e.cx), zz(p[1]-e.cy))
		e.cx, e.cy = p[0], p[1]
	}
}

func (e *geomEncoder) close() { e.cmds = append(e.cmds, 7|1<<3) }

// quantise rounds tile coordinates to the grid, dropping repeated points.
func quantise(p []fpt) [][2]int32 {
	out := make([][2]int32, 0, len(p))
	for _, q := range p {
		v := [2]int32{int32(math.Round(q.x)), int32(math.Round(q.y))}
		if len(out) > 0 && out[len(out)-1] == v {
			continue
		}
		out = append(out, v)
	}
	return out
}

func signedArea(r [][2]int32) float64 {
	s := 0.0
	for i := range r {
		j := (i + 1) % len(r)
		s += float64(r[i][0])*float64(r[j][1]) - float64(r[j][0])*float64(r[i][1])
	}
	return s / 2
}

// ---- tile building ----

type tileCtx struct {
	z, x, y int
	scale   float64 // world units at zoom 0 to tile units
	ox, oy  float64
	tol     float64
}

func newTileCtx(z, x, y int) *tileCtx {
	s := math.Exp2(float64(z)) * Extent
	t := &tileCtx{z: z, x: x, y: y, scale: s, ox: float64(x) * Extent, oy: float64(y) * Extent, tol: 1.0}
	if z >= MaxZoom {
		t.tol = 0.4
	}
	return t
}

func (t *tileCtx) project(r []Pt) []fpt {
	out := make([]fpt, len(r))
	for i, p := range r {
		lat, lon := p.Deg()
		out[i] = fpt{mercX(lon)*t.scale - t.ox, mercY(lat)*t.scale - t.oy}
	}
	return out
}

func (t *tileCtx) line(parts [][]Pt) []uint32 {
	var e geomEncoder
	for _, part := range parts {
		p := simplify(t.project(part), t.tol)
		for _, piece := range clipLine(p, -buffer, Extent+buffer) {
			q := quantise(piece)
			if len(q) < 2 {
				continue
			}
			e.moveTo(q[0][0], q[0][1])
			e.lineTo(q[1:])
		}
	}
	return e.cmds
}

func (t *tileCtx) polygon(polys [][][]Pt, minArea float64) []uint32 {
	var e geomEncoder
	for _, poly := range polys {
		for ri, ring := range poly {
			p := t.project(ring)
			if len(p) > 1 && p[0] == p[len(p)-1] {
				p = p[:len(p)-1]
			}
			p = simplify(p, t.tol)
			p = clipRing(p, -buffer, Extent+buffer)
			q := quantise(p)
			if len(q) > 1 && q[0] == q[len(q)-1] {
				q = q[:len(q)-1]
			}
			if len(q) < 3 {
				if ri == 0 {
					break // no outer ring: skip its holes too
				}
				continue
			}
			a := signedArea(q)
			if math.Abs(a) < minArea {
				if ri == 0 {
					break
				}
				continue
			}
			// Outer rings clockwise on screen (positive area with y down), holes the other way.
			if (ri == 0) != (a > 0) {
				for i, j := 0, len(q)-1; i < j; i, j = i+1, j-1 {
					q[i], q[j] = q[j], q[i]
				}
			}
			e.moveTo(q[0][0], q[0][1])
			e.lineTo(q[1:])
			e.close()
		}
	}
	return e.cmds
}

func (t *tileCtx) point(p Pt) []uint32 {
	q := t.project([]Pt{p})[0]
	if q.x < -buffer || q.y < -buffer || q.x > Extent+buffer || q.y > Extent+buffer {
		return nil
	}
	var e geomEncoder
	e.moveTo(int32(math.Round(q.x)), int32(math.Round(q.y)))
	return e.cmds
}

// layerOrder is the order layers are written in; the style decides drawing order.
var layerOrder = []string{"area", LLand, LWater, LWaterway, LAeroway, LBuilding, LRail, LRoad, LBoundary, LLabel, LPlace, LPOI, LAddr}

// buildTile makes one vector tile from the store.
func (s *Store) buildTile(ctx context.Context, z, x, y int) ([]byte, error) {
	t := newTileCtx(z, x, y)
	box := TileBox(z, x, y)
	// The query box includes the buffer.
	padLon := (box.MaxLon - box.MinLon) * buffer / Extent
	padLat := (box.MaxLat - box.MinLat) * buffer / Extent
	q := BBox{box.MinLon - padLon, box.MinLat - padLat, box.MaxLon + padLon, box.MaxLat + padLat}
	layers := map[string]*mvtLayer{}
	get := func(n string) *mvtLayer {
		l, ok := layers[n]
		if !ok {
			l = newLayer(n)
			layers[n] = l
		}
		return l
	}
	ab := s.Area.Box()
	if q.MaxLon < ab.MinLon || q.MinLon > ab.MaxLon || q.MaxLat < ab.MinLat || q.MinLat > ab.MaxLat {
		return nil, nil
	}
	// The recorded area itself, so the map can show where recorded data ends.
	circle := s.areaRing()
	if g := t.polygon([][][]Pt{{circle}}, 0); len(g) > 0 {
		get("area").add(1, 3, g, map[string]string{"name": s.Area.Name})
	}
	bands := []string{"rt_lo"}
	if z >= 10 {
		bands = append(bands, "rt_mid")
	}
	if z >= 13 {
		bands = append(bands, "rt_hi")
	}
	zoomLimit := z
	if z >= MaxZoom {
		zoomLimit = 99
	}
	minArea := 2.0
	if z >= MaxZoom {
		minArea = 0.5
	}
	for _, bnd := range bands {
		rows, err := s.db.QueryContext(ctx, `SELECT f.id, f.layer, f.kind, f.name, f.attrs, f.geom FROM `+bnd+` r JOIN feat f ON f.id = r.id
			WHERE r.minx <= ? AND r.maxx >= ? AND r.miny <= ? AND r.maxy >= ? AND f.minzoom <= ?`, q.MaxLon, q.MinLon, q.MaxLat, q.MinLat, zoomLimit)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			var layer, kind, nm, at string
			var gb []byte
			if err := rows.Scan(&id, &layer, &kind, &nm, &at, &gb); err != nil {
				rows.Close()
				return nil, err
			}
			g, err := DecodeGeom(gb)
			if err != nil {
				continue
			}
			props := map[string]string{"kind": kind}
			if nm != "" {
				props["name"] = nm
			}
			if at != "" {
				var m map[string]string
				if json.Unmarshal([]byte(at), &m) == nil {
					for k, v := range m {
						props[k] = v
					}
				}
			}
			var cmds []uint32
			typ := 0
			switch g.Kind {
			case GPoint:
				cmds, typ = t.point(g.Parts[0][0]), 1
			case GLine:
				cmds, typ = t.line(g.Parts), 2
			case GPolygon:
				cmds, typ = t.polygon(g.Polys, minArea), 3
			}
			get(layer).add(uint64(id), typ, cmds, props)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	var out []byte
	for _, n := range layerOrder {
		if l, ok := layers[n]; ok && len(l.features) > 0 {
			out = pbBytes(out, 3, l.encode())
		}
	}
	return out, nil
}

// areaRing is the recorded circle as a 128-sided polygon.
func (s *Store) areaRing() []Pt {
	a := s.Area
	r := make([]Pt, 0, 129)
	for i := 0; i <= 128; i++ {
		th := float64(i) / 128 * 2 * math.Pi
		dLat := a.RadiusM / metresPerDeg * math.Sin(th)
		dLon := a.RadiusM / (metresPerDeg * math.Cos(a.Lat*math.Pi/180)) * math.Cos(th)
		r = append(r, PtDeg(a.Lat+dLat, a.Lon+dLon))
	}
	return r
}

func gz(b []byte) []byte {
	var buf bytes.Buffer
	w, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	w.Write(b)
	w.Close()
	return buf.Bytes()
}

type tileKey [3]int

type tileCall struct {
	done chan struct{}
	data []byte
	err  error
}

// Tile returns a gzip-compressed vector tile, or nil for a tile with nothing in it. Tiles are kept in the
// store after they are first made.
func (s *Store) Tile(ctx context.Context, z, x, y int) ([]byte, error) {
	if z < 0 || z > MaxZoom || x < 0 || y < 0 || x >= 1<<z || y >= 1<<z {
		return nil, nil
	}
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT data FROM tiles WHERE z = ? AND x = ? AND y = ?`, z, x, y).Scan(&data)
	if err == nil {
		if len(data) == 0 {
			return nil, nil
		}
		return data, nil
	}
	k := tileKey{z, x, y}
	s.tmu.Lock()
	if c, ok := s.inflight[k]; ok {
		s.tmu.Unlock()
		<-c.done
		return c.data, c.err
	}
	c := &tileCall{done: make(chan struct{})}
	s.inflight[k] = c
	s.tmu.Unlock()
	defer func() {
		s.tmu.Lock()
		delete(s.inflight, k)
		s.tmu.Unlock()
		close(c.done)
	}()
	raw, err := s.buildTile(context.WithoutCancel(ctx), z, x, y)
	if err != nil {
		c.err = err
		return nil, err
	}
	if len(raw) > 0 {
		c.data = gz(raw)
	}
	save := c.data
	if save == nil {
		save = []byte{} // remembered as empty
	}
	s.db.ExecContext(context.WithoutCancel(ctx), `INSERT OR REPLACE INTO tiles(z, x, y, data) VALUES(?, ?, ?, ?)`, z, x, y, save)
	return c.data, nil
}

// TileName is a tile's name in logs.
func TileName(z, x, y int) string {
	return strconv.Itoa(z) + "/" + strconv.Itoa(x) + "/" + strconv.Itoa(y)
}
