package mapdata

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/osm"
)

// Area is the circle a map store holds.
type Area struct {
	Name    string  `json:"name"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	RadiusM float64 `json:"radiusM"`
}

// Box is the area's bounding box.
func (a Area) Box() BBox {
	dLat := a.RadiusM / metresPerDeg
	dLon := a.RadiusM / (metresPerDeg * math.Cos(a.Lat*math.Pi/180))
	return BBox{a.Lon - dLon, a.Lat - dLat, a.Lon + dLon, a.Lat + dLat}
}

// Contains reports whether a point is inside the circle.
func (a Area) Contains(lat, lon float64) bool { return Distance(a.Lat, a.Lon, lat, lon) <= a.RadiusM }

// Progress is an import's state, for the administration screen.
type Progress struct {
	Phase    string           `json:"phase"`
	Detail   string           `json:"detail"`
	Fraction float64          `json:"fraction"` // of the phase, 0..1
	Counts   map[string]int64 `json:"counts"`
}

// Stats summarises a finished store.
type Stats struct {
	Features      int64            `json:"features"`
	ByLayer       map[string]int64 `json:"byLayer"`
	Addresses     int64            `json:"addresses"`
	Intersections int64            `json:"intersections"`
	RoadSegments  int64            `json:"roadSegments"`
	RoadKm        float64          `json:"roadKm"`
	Nodes         int64            `json:"nodes"`
	Seconds       float64          `json:"seconds"`
}

// feature IDs pack the OpenStreetMap element and a sub-feature: id<<4 | type<<2 | sub. Insert-or-ignore
// on them removes the copies that overlapping extracts both hold.
func fid(typ byte, id int64, sub int) int64 {
	t := int64(0)
	switch typ {
	case 'w':
		t = 1
	case 'r':
		t = 2
	}
	return id<<4 | t<<2 | int64(sub&3)
}

const schema = `
PRAGMA journal_mode=OFF;
PRAGMA synchronous=OFF;
PRAGMA page_size=8192;
CREATE TABLE meta(k TEXT PRIMARY KEY, v TEXT NOT NULL);
CREATE TABLE feat(id INTEGER PRIMARY KEY, layer TEXT NOT NULL, kind TEXT NOT NULL, minzoom INTEGER NOT NULL, name TEXT NOT NULL DEFAULT '', attrs TEXT NOT NULL DEFAULT '', geom BLOB NOT NULL);
CREATE VIRTUAL TABLE rt_lo USING rtree(id, minx, maxx, miny, maxy);
CREATE VIRTUAL TABLE rt_mid USING rtree(id, minx, maxx, miny, maxy);
CREATE VIRTUAL TABLE rt_hi USING rtree(id, minx, maxx, miny, maxy);
CREATE TABLE addr(id INTEGER PRIMARY KEY, num TEXT NOT NULL, street TEXT NOT NULL, skey TEXT NOT NULL, unit TEXT NOT NULL DEFAULT '', city TEXT NOT NULL DEFAULT '', state TEXT NOT NULL DEFAULT '', zip TEXT NOT NULL DEFAULT '', lat REAL NOT NULL, lon REAL NOT NULL, src TEXT NOT NULL DEFAULT '');
CREATE VIRTUAL TABLE names USING fts5(norm, name UNINDEXED, kind UNINDEXED, layer UNINDEXED, lat UNINDEXED, lon UNINDEXED, tokenize='unicode61');
CREATE TABLE xing(a TEXT NOT NULL, b TEXT NOT NULL, an TEXT NOT NULL, bn TEXT NOT NULL, lat REAL NOT NULL, lon REAL NOT NULL);
CREATE TABLE blob(name TEXT PRIMARY KEY, data BLOB NOT NULL);
CREATE TABLE rng(street TEXT NOT NULL, skey TEXT NOT NULL, lf INTEGER NOT NULL, lt INTEGER NOT NULL, rf INTEGER NOT NULL, rt INTEGER NOT NULL,
  cityl TEXT NOT NULL DEFAULT '', cityr TEXT NOT NULL DEFAULT '', zipl TEXT NOT NULL DEFAULT '', zipr TEXT NOT NULL DEFAULT '', pts TEXT NOT NULL,
  minlat REAL NOT NULL, maxlat REAL NOT NULL, minlon REAL NOT NULL, maxlon REAL NOT NULL);
CREATE TABLE tiles(z INTEGER NOT NULL, x INTEGER NOT NULL, y INTEGER NOT NULL, data BLOB NOT NULL, PRIMARY KEY(z, x, y)) WITHOUT ROWID;
`

const indexes = `
CREATE INDEX addr_num ON addr(num, skey);
CREATE INDEX addr_skey ON addr(skey);
CREATE INDEX xing_ab ON xing(a, b);
CREATE INDEX rng_skey ON rng(skey);
CREATE INDEX rng_box ON rng(minlat, minlon);
ANALYZE;
`

// band is the R-tree a feature goes in, so a low-zoom tile never walks a city's buildings.
func band(minzoom int) string {
	switch {
	case minzoom <= 9:
		return "rt_lo"
	case minzoom <= 12:
		return "rt_mid"
	}
	return "rt_hi"
}

// nodeIndex holds the coordinates of every node near the area, sorted by ID.
type nodeIndex struct {
	ids []int64
	pts []Pt
}

func (n *nodeIndex) get(id int64) (Pt, bool) {
	i, ok := slices.BinarySearch(n.ids, id)
	if !ok {
		return Pt{}, false
	}
	return n.pts[i], true
}

type row struct {
	feat *featRow
	addr *addrRow
	name *nameRow
	rng  *rngRow
}

type featRow struct {
	id      int64
	layer   string
	kind    string
	minzoom int
	name    string
	attrs   map[string]string
	g       Geom
}

type addrRow struct {
	id                                  int64
	num, street, unit, city, state, zip string
	src                                 string
	p                                   Pt
}

type nameRow struct {
	name, kind, layer string
	p                 Pt
}

type roadWay struct {
	refs     []int64
	fwd, bwd bool
	kmh      float32
	name     string
	ref      string
}

type memberWay struct {
	refs []int64
	pts  []Pt
}

type relRec struct {
	id    int64
	tags  map[string]string
	outer []int64
	inner []int64
	sp    spec
	bound int // admin level for boundaries, 0 otherwise
}

// Build makes a map store at path from OpenStreetMap extracts, keeping what lies inside area. It writes
// to path+".building" and renames at the end, so a store being served is replaced only by a whole one.
func Build(ctx context.Context, path string, area Area, files []string, source string, prog func(Progress)) (Stats, error) {
	start := time.Now()
	if prog == nil {
		prog = func(Progress) {}
	}
	tmp := path + ".building"
	os.Remove(tmp)
	db, err := sql.Open("sqlite", tmp)
	if err != nil {
		return Stats{}, err
	}
	db.SetMaxOpenConns(1)
	defer func() {
		db.Close()
		os.Remove(tmp)
	}()
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return Stats{}, fmt.Errorf("mapdata: schema: %w", err)
	}
	b := &builder{ctx: ctx, area: area, box: area.Box(), prog: prog, counts: map[string]*atomic.Int64{}}
	// Nodes a little outside the circle keep the roads that cross its edge whole as far as the edge.
	b.near = area
	b.near.RadiusM += 5000
	b.nearBox = b.near.Box()
	w := newWriter(ctx, db)
	b.out = w.in
	errc := make(chan error, 1)
	go func() { errc <- w.run() }()
	stage := func(fn func() error) error {
		if err := fn(); err != nil {
			close(b.out)
			<-errc
			return err
		}
		return ctx.Err()
	}
	if err := stage(func() error { return b.nodes(files) }); err != nil {
		return Stats{}, err
	}
	if err := stage(func() error { return b.relations(files) }); err != nil {
		return Stats{}, err
	}
	if err := stage(func() error { return b.ways(files) }); err != nil {
		return Stats{}, err
	}
	if err := stage(b.assemble); err != nil {
		return Stats{}, err
	}
	b.report("Building the road network", "", 0)
	g, xings, err := buildGraph(b.roads, &b.idx)
	if err != nil {
		close(b.out)
		<-errc
		return Stats{}, err
	}
	close(b.out)
	if err := <-errc; err != nil {
		return Stats{}, err
	}
	b.report("Writing the road network", fmt.Sprintf("%d road segments", len(g.Segs)), 0)
	st := Stats{ByLayer: map[string]int64{}, Nodes: int64(len(b.idx.ids))}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Stats{}, err
	}
	if _, err := tx.Exec(`INSERT INTO blob(name, data) VALUES('graph', ?)`, g.encode()); err != nil {
		tx.Rollback()
		return Stats{}, err
	}
	xs, err := tx.Prepare(`INSERT INTO xing(a, b, an, bn, lat, lon) VALUES(?, ?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return Stats{}, err
	}
	for _, x := range xings {
		lat, lon := x.p.Deg()
		if _, err := xs.Exec(Normalize(x.a), Normalize(x.b), x.a, x.b, lat, lon); err != nil {
			tx.Rollback()
			return Stats{}, err
		}
		if _, err := xs.Exec(Normalize(x.b), Normalize(x.a), x.b, x.a, lat, lon); err != nil {
			tx.Rollback()
			return Stats{}, err
		}
	}
	xs.Close()
	for _, s := range g.Segs {
		st.RoadKm += float64(s.Metres) / 1000
	}
	st.RoadSegments = int64(len(g.Segs))
	st.Intersections = int64(len(xings))
	if err := tx.Commit(); err != nil {
		return Stats{}, err
	}
	b.report("Indexing", "addresses, names and intersections", 0)
	if _, err := db.ExecContext(ctx, indexes); err != nil {
		return Stats{}, fmt.Errorf("mapdata: indexes: %w", err)
	}
	rows, err := db.QueryContext(ctx, `SELECT layer, count(*) FROM feat GROUP BY layer`)
	if err != nil {
		return Stats{}, err
	}
	for rows.Next() {
		var l string
		var n int64
		rows.Scan(&l, &n)
		st.ByLayer[l] = n
		st.Features += n
	}
	rows.Close()
	db.QueryRowContext(ctx, `SELECT count(*) FROM addr`).Scan(&st.Addresses)
	st.Seconds = math.Round(time.Since(start).Seconds())
	meta := map[string]any{"area": area, "built": time.Now().UTC().Format(time.RFC3339), "source": source, "files": files, "stats": st, "format": 1}
	for k, v := range meta {
		j, _ := json.Marshal(v)
		if _, err := db.ExecContext(ctx, `INSERT INTO meta(k, v) VALUES(?, ?)`, k, string(j)); err != nil {
			return Stats{}, err
		}
	}
	b.report("Compacting", "", 0)
	if _, err := db.ExecContext(ctx, `VACUUM`); err != nil {
		return Stats{}, err
	}
	if err := db.Close(); err != nil {
		return Stats{}, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return Stats{}, err
	}
	return st, nil
}

type builder struct {
	ctx          context.Context
	area, near   Area
	box, nearBox BBox
	prog         func(Progress)
	out          chan row
	idx          nodeIndex
	rels         []relRec
	memberSet    map[int64]bool
	members      map[int64]memberWay
	roads        []roadWay
	mu           sync.Mutex
	countsMu     sync.Mutex
	counts       map[string]*atomic.Int64
	phase        string
}

func (b *builder) count(k string, n int64) {
	b.countsMu.Lock()
	c, ok := b.counts[k]
	if !ok {
		c = &atomic.Int64{}
		b.counts[k] = c
	}
	b.countsMu.Unlock()
	c.Add(n)
}

func (b *builder) report(phase, detail string, frac float64) {
	b.phase = phase
	m := map[string]int64{}
	b.countsMu.Lock()
	for k, v := range b.counts {
		m[k] = v.Load()
	}
	b.countsMu.Unlock()
	b.prog(Progress{Phase: phase, Detail: detail, Fraction: frac, Counts: m})
}

func (b *builder) scan(files []string, want osm.Want, phase string, fn func(w int, blk *osm.Block) error) error {
	for i, f := range files {
		name := baseName(f)
		last := time.Time{}
		err := osm.Scan(f, want, 0, func(read, total int64) {
			if time.Since(last) > 500*time.Millisecond {
				last = time.Now()
				b.report(phase, fmt.Sprintf("%s (%d of %d)", name, i+1, len(files)), (float64(i)+float64(read)/float64(total))/float64(len(files)))
			}
		}, func(w int, blk *osm.Block) error {
			if err := b.ctx.Err(); err != nil {
				return err
			}
			return fn(w, blk)
		})
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func (b *builder) inNear(p Pt) bool {
	lat, lon := p.Deg()
	if lon < b.nearBox.MinLon || lon > b.nearBox.MaxLon || lat < b.nearBox.MinLat || lat > b.nearBox.MaxLat {
		return false
	}
	return b.near.Contains(lat, lon)
}

func (b *builder) inArea(p Pt) bool {
	lat, lon := p.Deg()
	return b.area.Contains(lat, lon)
}

// nodes keeps the coordinates of nodes near the area and records tagged nodes as places, POIs and
// addresses.
func (b *builder) nodes(files []string) error {
	workers := runtime.NumCPU()
	ids := make([][]int64, workers)
	pts := make([][]Pt, workers)
	err := b.scan(files, osm.Want{Nodes: true}, "Reading map points", func(w int, blk *osm.Block) error {
		for i := range blk.Nodes {
			n := &blk.Nodes[i]
			p := Pt{Lon: n.Lon, Lat: n.Lat}
			if !b.inNear(p) {
				continue
			}
			ids[w] = append(ids[w], n.ID)
			pts[w] = append(pts[w], p)
			if n.Tags != nil && b.inArea(p) {
				b.pointFeatures('n', n.ID, n.Tags, p)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	total := 0
	for _, s := range ids {
		total += len(s)
	}
	b.report("Sorting map points", fmt.Sprintf("%d points", total), 0)
	type pair struct {
		id int64
		p  Pt
	}
	all := make([]pair, 0, total)
	for w := range ids {
		for i := range ids[w] {
			all = append(all, pair{ids[w][i], pts[w][i]})
		}
		ids[w], pts[w] = nil, nil
	}
	sort.Slice(all, func(i, j int) bool { return all[i].id < all[j].id })
	b.idx.ids = make([]int64, 0, len(all))
	b.idx.pts = make([]Pt, 0, len(all))
	for i, a := range all {
		if i > 0 && all[i-1].id == a.id {
			continue // the same node from two overlapping extracts
		}
		b.idx.ids = append(b.idx.ids, a.id)
		b.idx.pts = append(b.idx.pts, a.p)
	}
	b.count("points", int64(len(b.idx.ids)))
	return nil
}

// pointFeatures records what a tagged point is: a place or POI, and its address.
func (b *builder) pointFeatures(typ byte, id int64, t map[string]string, p Pt) {
	if sp, ok := pointSpec(t); ok {
		b.out <- row{feat: &featRow{id: fid(typ, id, 0), layer: sp.layer, kind: sp.kind, minzoom: sp.minzoom, name: name(t), attrs: attrs(sp.layer, t),
			g: Geom{Kind: GPoint, Parts: [][]Pt{{p}}}}}
		b.count(sp.layer, 1)
		if n := name(t); n != "" {
			b.out <- row{name: &nameRow{name: n, kind: sp.kind, layer: sp.layer, p: p}}
		}
	}
	if num, street := t["addr:housenumber"], t["addr:street"]; num != "" && street != "" {
		b.out <- row{addr: &addrRow{id: fid(typ, id, 1), num: strings.ToUpper(num), street: street, unit: firstOf(t, "addr:unit", "addr:flats"),
			city: t["addr:city"], state: t["addr:state"], zip: t["addr:postcode"], p: p}}
		// House numbers on the map, unless the point is a building that shows its own.
		b.out <- row{feat: &featRow{id: fid(typ, id, 2), layer: LAddr, kind: "num", minzoom: 16, name: num, attrs: map[string]string{"street": street},
			g: Geom{Kind: GPoint, Parts: [][]Pt{{p}}}}}
		b.count("addresses", 1)
	}
}

// relations finds the multipolygons (lakes, forests) and boundaries (states, counties, towns) whose ways
// are needed.
func (b *builder) relations(files []string) error {
	var mu sync.Mutex
	b.memberSet = map[int64]bool{}
	err := b.scan(files, osm.Want{Relations: true}, "Reading areas and boundaries", func(w int, blk *osm.Block) error {
		for _, r := range blk.Relations {
			rec := relRec{id: r.ID, tags: r.Tags}
			switch r.Tags["type"] {
			case "multipolygon":
				sp, ok := areaSpec(r.Tags)
				if !ok {
					continue
				}
				rec.sp = sp
			case "boundary":
				if r.Tags["boundary"] == "administrative" {
					switch r.Tags["admin_level"] {
					case "4":
						rec.bound = 4
					case "6":
						rec.bound = 6
					case "8":
						rec.bound = 8
					}
				}
				if rec.bound == 0 {
					if sp, ok := areaSpec(r.Tags); ok && sp.layer == LLand {
						rec.sp = sp // national forests and parks
					} else {
						continue
					}
				}
			default:
				continue
			}
			for _, m := range r.Members {
				if m.Type != 'w' {
					continue
				}
				if m.Role == "inner" {
					rec.inner = append(rec.inner, m.ID)
				} else if m.Role == "outer" || m.Role == "" {
					rec.outer = append(rec.outer, m.ID)
				}
			}
			mu.Lock()
			for _, id := range rec.outer {
				b.memberSet[id] = true
			}
			for _, id := range rec.inner {
				b.memberSet[id] = true
			}
			b.rels = append(b.rels, rec)
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Overlapping extracts hold the same relation twice.
	sort.Slice(b.rels, func(i, j int) bool { return b.rels[i].id < b.rels[j].id })
	b.rels = slices.CompactFunc(b.rels, func(a, c relRec) bool { return a.id == c.id })
	b.count("relations", int64(len(b.rels)))
	return nil
}

// resolve looks up a way's points, dropping nodes that lie outside the area. It reports whether any point
// lies inside the circle itself.
func (b *builder) resolve(refs []int64) ([]Pt, bool) {
	pts := make([]Pt, 0, len(refs))
	inside := false
	for _, id := range refs {
		p, ok := b.idx.get(id)
		if !ok {
			continue
		}
		pts = append(pts, p)
		if !inside && b.inArea(p) {
			inside = true
		}
	}
	return pts, inside
}

func (b *builder) ways(files []string) error {
	workers := runtime.NumCPU()
	roads := make([][]roadWay, workers)
	members := make([]map[int64]memberWay, workers)
	for i := range members {
		members[i] = map[int64]memberWay{}
	}
	err := b.scan(files, osm.Want{Ways: true}, "Reading roads, buildings and water", func(w int, blk *osm.Block) error {
		for i := range blk.Ways {
			wy := &blk.Ways[i]
			member := b.memberSet[wy.ID]
			if wy.Tags == nil && !member {
				continue
			}
			pts, inside := b.resolve(wy.Refs)
			if len(pts) < 2 {
				continue
			}
			if member {
				members[w][wy.ID] = memberWay{refs: wy.Refs, pts: pts}
			}
			if !inside || wy.Tags == nil {
				continue
			}
			t := wy.Tags
			closed := len(wy.Refs) > 3 && wy.Refs[0] == wy.Refs[len(wy.Refs)-1] && len(pts) == len(wy.Refs)
			made := false
			if ls, ok := lineSpec(t); ok && !(closed && t["area"] == "yes") {
				b.out <- row{feat: &featRow{id: fid('w', wy.ID, 0), layer: ls.layer, kind: ls.kind, minzoom: ls.minzoom, name: name(t), attrs: attrs(ls.layer, t),
					g: Geom{Kind: GLine, Parts: [][]Pt{pts}}}}
				b.count(ls.layer, 1)
				made = true
				if ls.layer == LRoad {
					if kmh, ok := speedKmh(t); ok {
						ow := oneway(t)
						roads[w] = append(roads[w], roadWay{refs: wy.Refs, fwd: ow >= 0, bwd: ow <= 0, kmh: float32(kmh), name: t["name"], ref: t["ref"]})
					}
					if n := t["name"]; n != "" {
						// One search entry per street per 5 km, so Main Street in two towns is two results.
						b.out <- row{name: &nameRow{name: n, kind: "street", layer: LRoad, p: pts[len(pts)/2]}}
					}
				}
			}
			if closed && !made {
				if as, ok := areaSpec(t); ok {
					poly := [][][]Pt{{pts}}
					mz := as.minzoom
					if as.byArea {
						mz = minzoomForArea(ringArea(pts), as.minzoom)
					}
					b.out <- row{feat: &featRow{id: fid('w', wy.ID, 0), layer: as.layer, kind: as.kind, minzoom: mz, name: name(t), attrs: attrs(as.layer, t),
						g: Geom{Kind: GPolygon, Polys: poly}}}
					b.count(as.layer, 1)
					made = true
					b.areaExtras('w', wy.ID, t, poly, mz, as)
				}
			}
			if !made && closed {
				// An area that is only a POI or an address, such as a fire station drawn as an outline.
				b.pointFeatures('w', wy.ID, t, labelPoint([][][]Pt{{pts}}))
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	b.members = map[int64]memberWay{}
	for _, m := range members {
		for k, v := range m {
			b.members[k] = v
		}
	}
	for _, r := range roads {
		b.roads = append(b.roads, r...)
	}
	// Overlapping extracts hold the same road twice; the graph builder ignores repeated segments by node
	// pair, but dropping exact repeats here saves the work.
	sort.Slice(b.roads, func(i, j int) bool {
		a, c := b.roads[i].refs, b.roads[j].refs
		if a[0] != c[0] {
			return a[0] < c[0]
		}
		return len(a) < len(c)
	})
	b.roads = slices.CompactFunc(b.roads, func(a, c roadWay) bool { return slices.Equal(a.refs, c.refs) })
	return nil
}

// areaExtras adds an area's label, POI and address points.
func (b *builder) areaExtras(typ byte, id int64, t map[string]string, poly [][][]Pt, mz int, as spec) {
	lp := labelPoint(poly)
	if n := name(t); n != "" && as.layer != LBuilding {
		b.out <- row{feat: &featRow{id: fid(typ, id, 1), layer: LLabel, kind: as.kind, minzoom: max(mz, 10), name: n, g: Geom{Kind: GPoint, Parts: [][]Pt{{lp}}}}}
		b.out <- row{name: &nameRow{name: n, kind: as.kind, layer: as.layer, p: lp}}
	}
	if sp, ok := pointSpec(t); ok && sp.layer == LPOI {
		b.out <- row{feat: &featRow{id: fid(typ, id, 3), layer: LPOI, kind: sp.kind, minzoom: sp.minzoom, name: name(t), attrs: attrs(LPOI, t), g: Geom{Kind: GPoint, Parts: [][]Pt{{lp}}}}}
		b.count(LPOI, 1)
		if n := name(t); n != "" && as.layer == LBuilding {
			b.out <- row{name: &nameRow{name: n, kind: sp.kind, layer: LPOI, p: lp}}
		}
	}
	if num, street := t["addr:housenumber"], t["addr:street"]; num != "" && street != "" {
		b.out <- row{addr: &addrRow{id: fid(typ, id, 2), num: strings.ToUpper(num), street: street, unit: t["addr:unit"], city: t["addr:city"], state: t["addr:state"], zip: t["addr:postcode"], p: lp}}
		b.count("addresses", 1)
	}
}

// assemble joins relation members into polygons and boundary lines.
func (b *builder) assemble() error {
	b.report("Assembling lakes, forests and boundaries", fmt.Sprintf("%d relations", len(b.rels)), 0)
	boundLevel := map[int64]int{}
	for i, r := range b.rels {
		if i%200 == 0 {
			b.report("Assembling lakes, forests and boundaries", fmt.Sprintf("%d of %d", i, len(b.rels)), float64(i)/float64(len(b.rels)))
			if err := b.ctx.Err(); err != nil {
				return err
			}
		}
		if r.bound != 0 {
			for _, id := range r.outer {
				if l, ok := boundLevel[id]; !ok || r.bound < l {
					boundLevel[id] = r.bound
				}
			}
			if r.sp.layer == "" {
				continue
			}
		}
		outer := b.rings(r.outer)
		if len(outer) == 0 {
			continue
		}
		inner := b.rings(r.inner)
		polys := make([][][]Pt, 0, len(outer))
		for _, o := range outer {
			polys = append(polys, [][]Pt{o})
		}
		for _, h := range inner {
			// A hole belongs to the outer ring that contains it.
			for k := range polys {
				if inRing(h[0], polys[k][0]) {
					polys[k] = append(polys[k], h)
					break
				}
			}
		}
		inside := false
		for _, p := range polys {
			for _, q := range p[0] {
				if b.inArea(q) {
					inside = true
					break
				}
			}
			if inside {
				break
			}
		}
		if !inside {
			continue
		}
		mz := r.sp.minzoom
		if r.sp.byArea {
			mz = minzoomForArea(polyArea(polys), r.sp.minzoom)
		}
		b.out <- row{feat: &featRow{id: fid('r', r.id, 0), layer: r.sp.layer, kind: r.sp.kind, minzoom: mz, name: name(r.tags), attrs: attrs(r.sp.layer, r.tags),
			g: Geom{Kind: GPolygon, Polys: polys}}}
		b.count(r.sp.layer, 1)
		b.areaExtras('r', r.id, r.tags, polys, mz, r.sp)
	}
	zoomFor := map[int]int{4: 3, 6: 7, 8: 10}
	for id, lvl := range boundLevel {
		m, ok := b.members[id]
		if !ok {
			continue
		}
		in := false
		for _, p := range m.pts {
			if b.inArea(p) {
				in = true
				break
			}
		}
		if !in {
			continue
		}
		b.out <- row{feat: &featRow{id: fid('w', id, 1), layer: LBoundary, kind: map[int]string{4: "state", 6: "county", 8: "town"}[lvl], minzoom: zoomFor[lvl],
			attrs: map[string]string{"level": fmt.Sprint(lvl)}, g: Geom{Kind: GLine, Parts: [][]Pt{m.pts}}}}
		b.count(LBoundary, 1)
	}
	return nil
}

// rings joins ways end to end into closed rings; ways that cannot be closed (cut by the area's edge) are
// closed straight across, which only matters at the very edge of the area.
func (b *builder) rings(ids []int64) [][]Pt {
	type seg struct {
		refs []int64
		pts  []Pt
	}
	var segs []seg
	for _, id := range ids {
		m, ok := b.members[id]
		if !ok || len(m.pts) < 2 {
			continue
		}
		if len(m.pts) != len(m.refs) {
			// Partly outside the area: the points no longer line up with the references, so it cannot be
			// joined by node; keep it as its own ring.
			segs = append(segs, seg{refs: []int64{-id, -id - 1}, pts: m.pts})
			continue
		}
		segs = append(segs, seg{refs: m.refs, pts: m.pts})
	}
	var out [][]Pt
	used := make([]bool, len(segs))
	for i := range segs {
		if used[i] {
			continue
		}
		used[i] = true
		refs := slices.Clone(segs[i].refs)
		pts := slices.Clone(segs[i].pts)
		for refs[0] != refs[len(refs)-1] {
			found := false
			for j := range segs {
				if used[j] {
					continue
				}
				s := segs[j]
				end := refs[len(refs)-1]
				switch {
				case s.refs[0] == end:
					refs = append(refs, s.refs[1:]...)
					pts = append(pts, s.pts[1:]...)
				case s.refs[len(s.refs)-1] == end:
					rr, rp := slices.Clone(s.refs), slices.Clone(s.pts)
					slices.Reverse(rr)
					slices.Reverse(rp)
					refs = append(refs, rr[1:]...)
					pts = append(pts, rp[1:]...)
				default:
					continue
				}
				used[j] = true
				found = true
				break
			}
			if !found {
				break
			}
		}
		if len(pts) >= 3 {
			if pts[0] != pts[len(pts)-1] {
				pts = append(pts, pts[0])
			}
			out = append(out, pts)
		}
	}
	return out
}

// ---- the writer ----

type writer struct {
	ctx context.Context
	db  *sql.DB
	in  chan row
}

func newWriter(ctx context.Context, db *sql.DB) *writer {
	return &writer{ctx: ctx, db: db, in: make(chan row, 8192)}
}

// run writes rows in large transactions until in closes. On an error it keeps draining so senders do not
// block, and returns the first error.
func (w *writer) run() error {
	var firstErr error
	var tx *sql.Tx
	var fs, as, ns, rs *sql.Stmt
	rts := map[string]*sql.Stmt{}
	n := 0
	seenName := map[string]bool{}
	begin := func() error {
		var err error
		if tx, err = w.db.BeginTx(w.ctx, nil); err != nil {
			return err
		}
		if fs, err = tx.Prepare(`INSERT OR IGNORE INTO feat(id, layer, kind, minzoom, name, attrs, geom) VALUES(?, ?, ?, ?, ?, ?, ?)`); err != nil {
			return err
		}
		if as, err = tx.Prepare(`INSERT OR IGNORE INTO addr(id, num, street, skey, unit, city, state, zip, lat, lon, src) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`); err != nil {
			return err
		}
		if ns, err = tx.Prepare(`INSERT INTO names(norm, name, kind, layer, lat, lon) VALUES(?, ?, ?, ?, ?, ?)`); err != nil {
			return err
		}
		if rs, err = tx.Prepare(`INSERT INTO rng(street, skey, lf, lt, rf, rt, cityl, cityr, zipl, zipr, pts, minlat, maxlat, minlon, maxlon) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`); err != nil {
			return err
		}
		for _, t := range []string{"rt_lo", "rt_mid", "rt_hi"} {
			if rts[t], err = tx.Prepare(`INSERT OR IGNORE INTO ` + t + `(id, minx, maxx, miny, maxy) VALUES(?, ?, ?, ?, ?)`); err != nil {
				return err
			}
		}
		return nil
	}
	commit := func() error {
		if tx == nil {
			return nil
		}
		err := tx.Commit()
		tx = nil
		return err
	}
	for r := range w.in {
		if firstErr != nil {
			continue
		}
		if tx == nil {
			if err := begin(); err != nil {
				firstErr = err
				continue
			}
		}
		var err error
		switch {
		case r.feat != nil:
			f := r.feat
			a := ""
			if len(f.attrs) > 0 {
				j, _ := json.Marshal(f.attrs)
				a = string(j)
			}
			var res sql.Result
			res, err = fs.Exec(f.id, f.layer, f.kind, f.minzoom, f.name, a, f.g.Encode())
			if err == nil {
				if k, _ := res.RowsAffected(); k > 0 {
					bx := f.g.Box()
					_, err = rts[band(f.minzoom)].Exec(f.id, bx.MinLon, bx.MaxLon, bx.MinLat, bx.MaxLat)
				}
			}
		case r.addr != nil:
			a := r.addr
			lat, lon := a.p.Deg()
			_, err = as.Exec(a.id, a.num, a.street, Normalize(a.street), a.unit, a.city, a.state, a.zip, lat, lon, a.src)
		case r.rng != nil:
			x := r.rng
			gm := Geom{Kind: GLine, Parts: [][]Pt{x.pts}}
			b := gm.Box()
			_, err = rs.Exec(x.street, Normalize(x.street), x.lf, x.lt, x.rf, x.rt, x.cityL, x.cityR, x.zipL, x.zipR, encodePts(x.pts), b.MinLat, b.MaxLat, b.MinLon, b.MaxLon)
		case r.name != nil:
			x := r.name
			lat, lon := x.p.Deg()
			key := fmt.Sprintf("%s|%s|%d|%d", x.kind, x.name, int(math.Floor(lat/0.05)), int(math.Floor(lon/0.05)))
			if seenName[key] {
				continue
			}
			seenName[key] = true
			_, err = ns.Exec(Normalize(x.name), x.name, x.kind, x.layer, lat, lon)
		default:
			continue
		}
		if err != nil {
			firstErr = err
			continue
		}
		if n++; n%100000 == 0 {
			if err := commit(); err != nil {
				firstErr = err
			}
		}
	}
	if firstErr != nil {
		if tx != nil {
			tx.Rollback()
		}
		return firstErr
	}
	return commit()
}

// ---- the road graph ----

// Seg is one stretch of road between two junctions.
type Seg struct {
	U, V     uint32  // junctions
	Shape    uint32  // first point in Graph.Pts
	N        uint32  // number of points, ends included
	Metres   float32 // length
	Secs     float32 // driving time
	Fwd, Bwd bool    // may be driven U→V, V→U
	Name     uint32  // index into Graph.Names
}

// Graph is the drivable road network.
type Graph struct {
	Verts []Pt
	Segs  []Seg
	Pts   []Pt
	Names []string
}

type xing struct {
	a, b string
	p    Pt
}

// buildGraph cuts roads into segments at every node two roads share, and lists the intersections of
// differently named roads.
func buildGraph(roads []roadWay, idx *nodeIndex) (*Graph, []xing, error) {
	use := map[int64]uint16{}
	for _, r := range roads {
		for i, id := range r.refs {
			k := uint16(1)
			if i == 0 || i == len(r.refs)-1 {
				k = 2 // a way's ends are always junctions
			}
			if v := use[id]; v < 1000 {
				use[id] = v + k
			}
		}
	}
	g := &Graph{Names: []string{""}}
	nameIdx := map[string]uint32{"": 0}
	vert := map[int64]uint32{}
	vid := func(id int64, p Pt) uint32 {
		if v, ok := vert[id]; ok {
			return v
		}
		v := uint32(len(g.Verts))
		vert[id] = v
		g.Verts = append(g.Verts, p)
		return v
	}
	names := map[int64][]string{} // street names at each junction, for intersections
	seen := map[[2]uint32]bool{}
	for _, r := range roads {
		label := r.name
		if label == "" {
			label = strings.ReplaceAll(r.ref, ";", " / ")
		}
		ni, ok := nameIdx[label]
		if !ok {
			ni = uint32(len(g.Names))
			nameIdx[label] = ni
			g.Names = append(g.Names, label)
		}
		var cur []Pt
		var startV uint32
		have := false
		for i, id := range r.refs {
			p, ok := idx.get(id)
			if !ok {
				have = false // outside the area: the road is cut here
				cur = nil
				continue
			}
			junction := use[id] >= 2
			if junction && r.name != "" {
				ns := names[id]
				if !slices.Contains(ns, r.name) {
					names[id] = append(ns, r.name)
				}
			}
			if !have {
				startV = vid(id, p)
				cur = []Pt{p}
				have = true
				continue
			}
			cur = append(cur, p)
			if junction || i == len(r.refs)-1 {
				v := vid(id, p)
				key := [2]uint32{min(startV, v), max(startV, v)}
				if startV != v && !(seen[key] && len(cur) == 2) {
					seen[key] = true
					m := lineLength(cur)
					s := Seg{U: startV, V: v, Shape: uint32(len(g.Pts)), N: uint32(len(cur)), Metres: float32(m),
						Secs: float32(m / (float64(r.kmh) / 3.6)), Fwd: r.fwd, Bwd: r.bwd, Name: ni}
					g.Pts = append(g.Pts, cur...)
					g.Segs = append(g.Segs, s)
				}
				startV = v
				cur = []Pt{p}
			}
		}
	}
	if len(g.Segs) > math.MaxUint32/2 {
		return nil, nil, errors.New("mapdata: road network too large")
	}
	var xs []xing
	for id, ns := range names {
		if len(ns) < 2 {
			continue
		}
		p, _ := idx.get(id)
		for i := range ns {
			for j := i + 1; j < len(ns); j++ {
				xs = append(xs, xing{a: ns[i], b: ns[j], p: p})
			}
		}
	}
	return g, xs, nil
}

func (g *Graph) encode() []byte {
	var out []byte
	u32 := func(v uint32) { out = binary.LittleEndian.AppendUint32(out, v) }
	f32 := func(v float32) { u32(math.Float32bits(v)) }
	pt := func(p Pt) { u32(uint32(p.Lon)); u32(uint32(p.Lat)) }
	out = append(out, "BDG1"...)
	u32(uint32(len(g.Verts)))
	for _, p := range g.Verts {
		pt(p)
	}
	u32(uint32(len(g.Pts)))
	for _, p := range g.Pts {
		pt(p)
	}
	u32(uint32(len(g.Segs)))
	for _, s := range g.Segs {
		u32(s.U)
		u32(s.V)
		u32(s.Shape)
		u32(s.N)
		f32(s.Metres)
		f32(s.Secs)
		fl := uint32(0)
		if s.Fwd {
			fl |= 1
		}
		if s.Bwd {
			fl |= 2
		}
		u32(fl)
		u32(s.Name)
	}
	j, _ := json.Marshal(g.Names)
	u32(uint32(len(j)))
	out = append(out, j...)
	return out
}

func decodeGraph(b []byte) (*Graph, error) {
	bad := errors.New("mapdata: malformed road network")
	if len(b) < 4 || string(b[:4]) != "BDG1" {
		return nil, bad
	}
	i := 4
	u32 := func() (uint32, bool) {
		if i+4 > len(b) {
			return 0, false
		}
		v := binary.LittleEndian.Uint32(b[i:])
		i += 4
		return v, true
	}
	pts := func() ([]Pt, bool) {
		n, ok := u32()
		if !ok || uint64(n)*8 > uint64(len(b)-i) {
			return nil, false
		}
		r := make([]Pt, n)
		for k := range r {
			x, _ := u32()
			y, _ := u32()
			r[k] = Pt{Lon: int32(x), Lat: int32(y)}
		}
		return r, true
	}
	g := &Graph{}
	var ok bool
	if g.Verts, ok = pts(); !ok {
		return nil, bad
	}
	if g.Pts, ok = pts(); !ok {
		return nil, bad
	}
	n, ok := u32()
	if !ok || uint64(n)*32 > uint64(len(b)-i) {
		return nil, bad
	}
	g.Segs = make([]Seg, n)
	for k := range g.Segs {
		s := &g.Segs[k]
		s.U, _ = u32()
		s.V, _ = u32()
		s.Shape, _ = u32()
		s.N, _ = u32()
		m, _ := u32()
		t, _ := u32()
		fl, _ := u32()
		s.Name, _ = u32()
		s.Metres, s.Secs = math.Float32frombits(m), math.Float32frombits(t)
		s.Fwd, s.Bwd = fl&1 != 0, fl&2 != 0
		if s.U >= uint32(len(g.Verts)) || s.V >= uint32(len(g.Verts)) || uint64(s.Shape)+uint64(s.N) > uint64(len(g.Pts)) {
			return nil, bad
		}
	}
	jn, ok := u32()
	if !ok || int(jn) > len(b)-i {
		return nil, bad
	}
	if err := json.Unmarshal(b[i:i+int(jn)], &g.Names); err != nil {
		return nil, bad
	}
	for k := range g.Segs {
		if int(g.Segs[k].Name) >= len(g.Names) {
			return nil, bad
		}
	}
	return g, nil
}
