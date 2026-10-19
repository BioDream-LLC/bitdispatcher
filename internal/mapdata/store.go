package mapdata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // the store is SQLite, through a pure-Go driver
)

// Store is a recorded map: features for tiles, addresses and names for search, and the road network.
type Store struct {
	db     *sql.DB
	Area   Area
	Built  time.Time
	Source string
	Files  []string
	Stats  Stats

	tmu      sync.Mutex
	inflight map[tileKey]*tileCall

	srcCol string // "src" when addresses say where they come from (older stores: "''")

	rmu    sync.Mutex
	router *Router
	rerr   error

	places []place

	zmu   sync.Mutex
	zones *[]Zone
}

type place struct {
	name, kind string
	lat, lon   float64
}

// Open opens a store made by Build.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	s := &Store{db: db, inflight: map[tileKey]*tileCall{}}
	meta := map[string]string{}
	rows, err := db.Query(`SELECT k, v FROM meta`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("mapdata: %s is not a map store: %w", path, err)
	}
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		meta[k] = v
	}
	rows.Close()
	if meta["format"] != "1" {
		db.Close()
		return nil, fmt.Errorf("mapdata: %s has an unknown format", path)
	}
	s.srcCol = "''"
	var hasSrc int
	if db.QueryRow(`SELECT count(*) FROM pragma_table_info('addr') WHERE name = 'src'`).Scan(&hasSrc) == nil && hasSrc > 0 {
		s.srcCol = "src"
	}
	json.Unmarshal([]byte(meta["area"]), &s.Area)
	json.Unmarshal([]byte(meta["source"]), &s.Source)
	json.Unmarshal([]byte(meta["files"]), &s.Files)
	json.Unmarshal([]byte(meta["stats"]), &s.Stats)
	var built string
	json.Unmarshal([]byte(meta["built"]), &built)
	s.Built, _ = time.Parse(time.RFC3339, built)
	prow, err := db.Query(`SELECT name, kind, geom FROM feat WHERE layer = 'place' AND name != ''`)
	if err == nil {
		for prow.Next() {
			var p place
			var g []byte
			prow.Scan(&p.name, &p.kind, &g)
			if gm, err := DecodeGeom(g); err == nil && gm.Kind == GPoint {
				p.lat, p.lon = gm.Parts[0][0].Deg()
				s.places = append(s.places, p)
			}
		}
		prow.Close()
	}
	return s, nil
}

// Close closes the store.
func (s *Store) Close() error { return s.db.Close() }

// Router loads the road network the first time it is needed.
func (s *Store) Router() (*Router, error) {
	s.rmu.Lock()
	defer s.rmu.Unlock()
	if s.router != nil || s.rerr != nil {
		return s.router, s.rerr
	}
	var b []byte
	if err := s.db.QueryRow(`SELECT data FROM blob WHERE name = 'graph'`).Scan(&b); err != nil {
		s.rerr = fmt.Errorf("mapdata: no road network: %w", err)
		return nil, s.rerr
	}
	g, err := decodeGraph(b)
	if err != nil {
		s.rerr = err
		return nil, err
	}
	s.router = newRouter(g)
	return s.router, nil
}

// near is the nearest settlement name to a point, within 12 km.
func (s *Store) near(lat, lon float64) string {
	best, bd := "", 12000.0
	rank := map[string]float64{"city": 0.6, "town": 0.75, "village": 0.9, "suburb": 0.9, "hamlet": 1, "neighbourhood": 1.1, "locality": 1.3}
	for _, p := range s.places {
		w, ok := rank[p.kind]
		if !ok {
			continue
		}
		if d := Distance(lat, lon, p.lat, p.lon) * w; d < bd {
			best, bd = p.name, d
		}
	}
	return best
}

// Result is one search answer.
type Result struct {
	Label  string  `json:"label"`
	Detail string  `json:"detail,omitempty"` // town, or what kind of place
	Kind   string  `json:"kind"`             // address, intersection, street, place, poi, area
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Exact  bool    `json:"exact"`            // the house number or intersection itself was found
	Note   string  `json:"note,omitempty"`   // such as "nearest number: no 1234 on file"
	Source string  `json:"source,omitempty"` // county: from the county's GIS rather than OpenStreetMap
	Ranged bool    `json:"ranged,omitempty"` // placed by a centreline address range, not a point of its own
	dist   float64
	rank   float64
}

// Search finds addresses, intersections, streets, places and landmarks matching what a dispatcher typed.
// near (when set) ranks closer answers first.
func (s *Store) Search(ctx context.Context, q string, nearLat, nearLon float64, limit int) ([]Result, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []Result{}, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 12
	}
	if nearLat == 0 && nearLon == 0 {
		nearLat, nearLon = s.Area.Lat, s.Area.Lon
	}
	var out []Result
	add := func(r Result) {
		r.dist = Distance(nearLat, nearLon, r.Lat, r.Lon)
		out = append(out, r)
	}
	// Intersections: "williamson & orange", "williamson and orange", "williamson / orange".
	if a, b, ok := splitIntersection(q); ok {
		rs, err := s.intersections(ctx, Normalize(a), Normalize(b))
		if err != nil {
			return nil, err
		}
		for _, r := range rs {
			add(r)
		}
	}
	num, rest := splitNumber(q)
	rest, unit := splitUnit(rest)
	if num != "" && rest != "" {
		rs, err := s.addresses(ctx, num, rest, unit)
		if err != nil {
			return nil, err
		}
		for _, r := range rs {
			add(r)
		}
	}
	if len(out) < limit {
		rs, err := s.names(ctx, q, limit*3)
		if err != nil {
			return nil, err
		}
		for _, r := range rs {
			add(r)
		}
	}
	for i := range out {
		if out[i].Detail == "" {
			out[i].Detail = s.near(out[i].Lat, out[i].Lon)
		}
	}
	// Exact answers first, then by kind, then nearest.
	kindRank := map[string]float64{"address": 0, "intersection": 0, "poi": 1, "place": 1, "street": 2, "area": 2}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Exact != b.Exact {
			return a.Exact
		}
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		if kindRank[a.Kind] != kindRank[b.Kind] {
			return kindRank[a.Kind] < kindRank[b.Kind]
		}
		return a.dist < b.dist
	})
	// The same street a few metres apart from two extracts is one answer.
	var dedup []Result
	for _, r := range out {
		dup := false
		for _, d := range dedup {
			// The same address written two ways ("Rd SW", "Road Southwest") is one answer too.
			if (d.Label == r.Label || d.Kind == "address" && r.Kind == "address" && Normalize(d.Label) == Normalize(r.Label)) && Distance(d.Lat, d.Lon, r.Lat, r.Lon) < 1500 {
				dup = true
				break
			}
		}
		if !dup {
			dedup = append(dedup, r)
		}
		if len(dedup) == limit {
			break
		}
	}
	if dedup == nil {
		dedup = []Result{}
	}
	return dedup, nil
}

func splitIntersection(q string) (string, string, bool) {
	l := strings.ToLower(q)
	for _, sep := range []string{"&", " and ", "/", " at ", " x "} {
		if i := strings.Index(l, sep); i > 0 {
			a, b := strings.TrimSpace(q[:i]), strings.TrimSpace(q[i+len(sep):])
			if a != "" && b != "" {
				return a, b, true
			}
		}
	}
	return "", "", false
}

// prefixRange is the key range of strings starting with p.
func prefixRange(p string) (string, string) { return p, p + "\uffff" }

func (s *Store) intersections(ctx context.Context, a, b string) ([]Result, error) {
	alo, ahi := prefixRange(a)
	blo, bhi := prefixRange(b)
	rows, err := s.db.QueryContext(ctx, `SELECT an, bn, lat, lon FROM xing WHERE a >= ? AND a < ? AND b >= ? AND b < ? LIMIT 60`, alo, ahi, blo, bhi)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Result
	for rows.Next() {
		var an, bn string
		var lat, lon float64
		rows.Scan(&an, &bn, &lat, &lon)
		dup := false
		for _, r := range out {
			if r.Label == an+" & "+bn && Distance(r.Lat, r.Lon, lat, lon) < 300 {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, Result{Label: an + " & " + bn, Kind: "intersection", Lat: lat, Lon: lon, Exact: true})
		}
	}
	return out, rows.Err()
}

// addresses finds a house number on a street; a town written after the street narrows it, and a unit
// ("#B", "Suite 300") puts that unit first. When the number is not on file, the nearest numbers on that
// street are offered, marked as not exact.
func (s *Store) addresses(ctx context.Context, num, rest, unit string) ([]Result, error) {
	words := strings.Fields(Normalize(rest))
	var out []Result
	for drop := 0; drop <= 3 && drop < len(words); drop++ {
		street := strings.Join(words[:len(words)-drop], " ")
		town := strings.Join(words[len(words)-drop:], " ")
		lo, hi := prefixRange(street)
		rows, err := s.db.QueryContext(ctx, `SELECT num, street, unit, city, state, zip, lat, lon, `+s.srcCol+` FROM addr WHERE num = ? AND skey >= ? AND skey < ? LIMIT 40`, num, lo, hi)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var r Result
			var n, st, unit, city, state, zip, src string
			rows.Scan(&n, &st, &unit, &city, &state, &zip, &r.Lat, &r.Lon, &src)
			if src != "" {
				// Points from a further source come after the county's own.
				r.Source, r.rank = src, 0.1
			}
			if town != "" && !strings.HasPrefix(Normalize(city), town) && !strings.HasPrefix(Normalize(zip), town) {
				continue
			}
			r.Label = n + " " + st
			if unit != "" {
				r.Label += " #" + unit
			}
			r.Detail = strings.TrimSpace(strings.Join(nonEmpty(city, state, zip), ", "))
			r.Kind, r.Exact = "address", true
			out = append(out, r)
		}
		rows.Close()
		if len(out) > 0 {
			if unit != "" {
				// The unit asked for first, then the building's own point, then its other units.
				order := func(r Result) int {
					i := strings.LastIndex(r.Label, " #")
					switch {
					case i >= 0 && unitKey(r.Label[i+2:]) == unit:
						return 0
					case i < 0:
						return 1
					}
					return 2
				}
				sort.SliceStable(out, func(i, j int) bool { return order(out[i]) < order(out[j]) })
				if order(out[0]) != 0 {
					// The unit itself is not on file: placed at the building, still written with its unit.
					base := out[0].Label
					if i := strings.LastIndex(base, " #"); i >= 0 {
						base = base[:i]
					}
					out[0].Label = base + " #" + strings.ToUpper(unit)
					out[0].Note = "Unit " + strings.ToUpper(unit) + " is not on file: placed at the building"
				}
			}
			return out, nil
		}
	}
	// Not on file: placed by the street's address ranges, when the county's centrelines have them.
	if rs, err := s.ranged(ctx, num, words); err != nil || len(rs) > 0 {
		return rs, err
	}
	// Else the nearest numbers on the same side of that street.
	n := leadingInt(num)
	if n < 0 || len(words) == 0 {
		return nil, nil
	}
	lo, hi := prefixRange(strings.Join(words, " "))
	rows, err := s.db.QueryContext(ctx, `SELECT num, street, city, lat, lon FROM addr WHERE skey >= ? AND skey < ? LIMIT 20000`, lo, hi)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type cand struct {
		r Result
		d int
	}
	var cs []cand
	for rows.Next() {
		var r Result
		var nm, st, city string
		rows.Scan(&nm, &st, &city, &r.Lat, &r.Lon)
		v := leadingInt(nm)
		if v < 0 || v%2 != n%2 {
			continue
		}
		d := v - n
		if d < 0 {
			d = -d
		}
		if d > 400 {
			continue
		}
		r.Label, r.Detail, r.Kind = nm+" "+st, city, "address"
		r.Note = fmt.Sprintf("Nearest number on file: %s is not recorded", num)
		r.rank = 1 + float64(d)/1000
		cs = append(cs, cand{r, d})
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].d < cs[j].d })
	for i := 0; i < len(cs) && i < 3; i++ {
		out = append(out, cs[i].r)
	}
	return out, rows.Err()
}

func leadingInt(s string) int {
	v, any := 0, false
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		v = v*10 + int(c-'0')
		any = true
		if v > 1e7 {
			return -1
		}
	}
	if !any {
		return -1
	}
	return v
}

func nonEmpty(v ...string) []string {
	var out []string
	for _, s := range v {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// names searches streets, places and landmarks by the start of each word.
func (s *Store) names(ctx context.Context, q string, limit int) ([]Result, error) {
	words := strings.Fields(Normalize(q))
	if len(words) == 0 {
		return nil, nil
	}
	var m []string
	for _, w := range words {
		m = append(m, `"`+strings.ReplaceAll(w, `"`, ``)+`"*`)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT name, kind, layer, lat, lon, bm25(names) FROM names WHERE names MATCH ? ORDER BY bm25(names) LIMIT ?`,
		"norm : "+strings.Join(m, " "), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Result
	for rows.Next() {
		var r Result
		var kind, layer string
		var score float64
		rows.Scan(&r.Label, &kind, &layer, &r.Lat, &r.Lon, &score)
		switch layer {
		case LRoad:
			r.Kind = "street"
		case LPlace:
			r.Kind = "place"
			r.Detail = kind
		case LPOI:
			r.Kind = "poi"
			r.Detail = poiLabel[kind]
		default:
			r.Kind = "area"
			r.Detail = kind
		}
		// Whole-word matches beat prefixes.
		if strings.EqualFold(Normalize(r.Label), strings.Join(words, " ")) {
			r.rank = -1
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

var poiLabel = map[string]string{
	"fire": "fire station", "police": "police", "hospital": "hospital", "ems": "EMS station", "school": "school", "care": "care home",
	"government": "government", "prison": "prison", "worship": "place of worship", "fuel": "fuel", "clinic": "clinic", "airport": "airport",
	"helipad": "helipad", "business": "business", "civic": "civic", "hydrant": "hydrant", "pharmacy": "pharmacy",
}

// Reverse describes a point: the nearest recorded address within 60 m, else the nearest road.
func (s *Store) Reverse(ctx context.Context, lat, lon float64) (Result, error) {
	d := 0.0008
	rows, err := s.db.QueryContext(ctx, `SELECT num, street, unit, city, lat, lon, `+s.srcCol+` FROM addr WHERE lat BETWEEN ? AND ? AND lon BETWEEN ? AND ?`,
		lat-d, lat+d, lon-d/math.Cos(lat*math.Pi/180), lon+d/math.Cos(lat*math.Pi/180))
	if err != nil {
		return Result{}, err
	}
	best := Result{dist: math.Inf(1)}
	for rows.Next() {
		var r Result
		var n, st, unit, city, src string
		rows.Scan(&n, &st, &unit, &city, &r.Lat, &r.Lon, &src)
		r.Source = src
		if dd := Distance(lat, lon, r.Lat, r.Lon); dd < best.dist && dd < 60 {
			r.Label, r.Detail, r.Kind, r.Exact, r.dist = n+" "+st, city, "address", true, dd
			if unit != "" {
				r.Label += " #" + unit
			}
			best = r
		}
	}
	rows.Close()
	if best.Exact {
		if best.Detail == "" {
			best.Detail = s.near(lat, lon)
		}
		return best, nil
	}
	if rr, ok := s.reverseRanged(ctx, lat, lon); ok {
		if rr.Detail == "" {
			rr.Detail = s.near(lat, lon)
		}
		return rr, nil
	}
	r, err := s.Router()
	if err != nil {
		return Result{}, err
	}
	name, off, ok := r.NearestRoad(lat, lon)
	if !ok {
		return Result{}, errors.New("no road nearby")
	}
	if name == "" {
		name = "unnamed road"
	}
	res := Result{Label: name, Kind: "street", Lat: lat, Lon: lon, Detail: s.near(lat, lon)}
	if off > 25 {
		res.Note = fmt.Sprintf("%.0f m from %s", off, name)
		res.Label = "Near " + name
	}
	return res, nil
}

// Contains reports whether a point is inside the recorded area.
func (s *Store) Contains(lat, lon float64) bool { return s.Area.Contains(lat, lon) }

// ClearTiles forgets made tiles, after a style-affecting change to how tiles are built.
func (s *Store) ClearTiles(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM tiles`)
	return err
}
