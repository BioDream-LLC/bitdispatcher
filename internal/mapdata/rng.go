package mapdata

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Address ranges: a county's road centrelines carry the first and last house number on each side of
// each segment (NENA's from-left, to-left, from-right, to-right). An address with no point of its own is
// placed by interpolating along its segment, on its side of the road; a point on a road is given the
// number it would have. Both are marked as estimates.

type rngRow struct {
	street                   string
	lf, lt, rf, rt           int
	cityL, cityR, zipL, zipR string
	pts                      []Pt
}

// side is one side of a segment: its numbers and its town.
type side struct {
	from, to  int
	city, zip string
	left      bool
}

func (r rngRow) sides() []side {
	var out []side
	if r.lf > 0 || r.lt > 0 {
		out = append(out, side{r.lf, r.lt, r.cityL, r.zipL, true})
	}
	if r.rf > 0 || r.rt > 0 {
		out = append(out, side{r.rf, r.rt, r.cityR, r.zipR, false})
	}
	return out
}

// has reports whether n is in a side's range, with the same parity when the range keeps one side's
// numbers odd or even.
func (s side) has(n int) bool {
	lo, hi := min(s.from, s.to), max(s.from, s.to)
	if n < lo || n > hi {
		return false
	}
	if s.from%2 == s.to%2 && n%2 != s.from%2 {
		return false
	}
	return true
}

func decodePts(s string) []Pt {
	var ll [][2]float64
	if json.Unmarshal([]byte(s), &ll) != nil {
		return nil
	}
	return toPts(ll)
}

func encodePts(pts []Pt) string {
	ll := make([][2]float64, len(pts))
	for i, p := range pts {
		lat, lon := p.Deg()
		ll[i] = [2]float64{math.Round(lon*1e6) / 1e6, math.Round(lat*1e6) / 1e6}
	}
	b, _ := json.Marshal(ll)
	return string(b)
}

// metres are local planar coordinates around a latitude.
func toXY(lat0 float64, p Pt) (float64, float64) {
	lat, lon := p.Deg()
	return lon * 111320 * math.Cos(lat0*math.Pi/180), lat * 110540
}

func fromXY(lat0, x, y float64) (float64, float64) {
	return y / 110540, x / (111320 * math.Cos(lat0*math.Pi/180))
}

// along is the point a fraction t of the way along a line, set off metres to its left (or right).
func along(pts []Pt, t, off float64, left bool) (float64, float64) {
	lat0, _ := pts[0].Deg()
	xs, ys := make([]float64, len(pts)), make([]float64, len(pts))
	total := 0.0
	for i, p := range pts {
		xs[i], ys[i] = toXY(lat0, p)
		if i > 0 {
			total += math.Hypot(xs[i]-xs[i-1], ys[i]-ys[i-1])
		}
	}
	want := math.Max(0, math.Min(1, t)) * total
	for i := 1; i < len(pts); i++ {
		l := math.Hypot(xs[i]-xs[i-1], ys[i]-ys[i-1])
		if want <= l || i == len(pts)-1 {
			f := 0.0
			if l > 0 {
				f = math.Min(1, want/l)
			}
			dx, dy := xs[i]-xs[i-1], ys[i]-ys[i-1]
			x, y := xs[i-1]+f*dx, ys[i-1]+f*dy
			if l > 0 {
				nx, ny := -dy/l, dx/l // to the left of the direction of travel
				if !left {
					nx, ny = -nx, -ny
				}
				x, y = x+nx*off, y+ny*off
			}
			return fromXY(lat0, x, y)
		}
		want -= l
	}
	return pts[0].Deg()
}

// project finds where a point falls on a line: how far from it, how far along (0 to 1), and which side.
func project(pts []Pt, lat, lon float64) (dist, t float64, left bool) {
	lat0, _ := pts[0].Deg()
	px, py := toXY(lat0, PtDeg(lat, lon))
	xs, ys := make([]float64, len(pts)), make([]float64, len(pts))
	total := 0.0
	cum := make([]float64, len(pts))
	for i, p := range pts {
		xs[i], ys[i] = toXY(lat0, p)
		if i > 0 {
			total += math.Hypot(xs[i]-xs[i-1], ys[i]-ys[i-1])
		}
		cum[i] = total
	}
	dist = math.Inf(1)
	for i := 1; i < len(pts); i++ {
		dx, dy := xs[i]-xs[i-1], ys[i]-ys[i-1]
		l2 := dx*dx + dy*dy
		f := 0.0
		if l2 > 0 {
			f = math.Max(0, math.Min(1, ((px-xs[i-1])*dx+(py-ys[i-1])*dy)/l2))
		}
		cx, cy := xs[i-1]+f*dx, ys[i-1]+f*dy
		if d := math.Hypot(px-cx, py-cy); d < dist {
			dist = d
			if total > 0 {
				t = (cum[i-1] + f*math.Sqrt(l2)) / total
			}
			left = dx*(py-ys[i-1])-dy*(px-xs[i-1]) > 0
		}
	}
	return dist, t, left
}

func (s *Store) hasRanges() bool {
	var n int
	return s.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'rng'`).Scan(&n) == nil && n > 0
}

func scanRng(rows interface{ Scan(...any) error }) (rngRow, error) {
	var r rngRow
	var pts string
	err := rows.Scan(&r.street, &r.lf, &r.lt, &r.rf, &r.rt, &r.cityL, &r.cityR, &r.zipL, &r.zipR, &pts)
	r.pts = decodePts(pts)
	return r, err
}

const rngCols = `street, lf, lt, rf, rt, cityl, cityr, zipl, zipr, pts`

// ranged places a house number by the address ranges of the street's segments.
func (s *Store) ranged(ctx context.Context, num string, words []string) ([]Result, error) {
	n := leadingInt(num)
	if n <= 0 || len(words) == 0 || !s.hasRanges() {
		return nil, nil
	}
	for drop := 0; drop <= 3 && drop < len(words); drop++ {
		street := strings.Join(words[:len(words)-drop], " ")
		town := strings.Join(words[len(words)-drop:], " ")
		lo, hi := prefixRange(street)
		rows, err := s.db.QueryContext(ctx, `SELECT `+rngCols+` FROM rng WHERE skey >= ? AND skey < ? AND
			((? BETWEEN min(lf, lt) AND max(lf, lt)) OR (? BETWEEN min(rf, rt) AND max(rf, rt))) LIMIT 40`, lo, hi, n, n)
		if err != nil {
			return nil, err
		}
		var out []Result
		for rows.Next() {
			r, err := scanRng(rows)
			if err != nil || len(r.pts) < 2 {
				continue
			}
			for _, sd := range r.sides() {
				if !sd.has(n) {
					continue
				}
				if town != "" && !strings.HasPrefix(Normalize(sd.city), town) && !strings.HasPrefix(Normalize(sd.zip), town) {
					continue
				}
				t := 0.5
				if sd.to != sd.from {
					t = float64(n-sd.from) / float64(sd.to-sd.from)
				}
				lat, lon := along(r.pts, t, 9, sd.left)
				dup := false
				for _, o := range out {
					if Distance(o.Lat, o.Lon, lat, lon) < 30 {
						dup = true
					}
				}
				if dup {
					continue
				}
				out = append(out, Result{Label: fmt.Sprintf("%d %s", n, r.street), Detail: strings.Join(nonEmpty(sd.city, sd.zip), ", "), Kind: "address",
					Lat: lat, Lon: lon, Ranged: true, Note: fmt.Sprintf("Placed by the address range %d–%d", sd.from, sd.to), rank: 0.5})
			}
		}
		rows.Close()
		if len(out) > 0 {
			return out, nil
		}
	}
	return nil, nil
}

// reverseRanged gives a point on a road the number it would have, from the nearest segment's range.
func (s *Store) reverseRanged(ctx context.Context, lat, lon float64) (Result, bool) {
	if !s.hasRanges() {
		return Result{}, false
	}
	d := 0.0012
	dl := d / math.Cos(lat*math.Pi/180)
	rows, err := s.db.QueryContext(ctx, `SELECT `+rngCols+` FROM rng WHERE minlat <= ? AND maxlat >= ? AND minlon <= ? AND maxlon >= ?`, lat+d, lat-d, lon+dl, lon-dl)
	if err != nil {
		return Result{}, false
	}
	defer rows.Close()
	best, bd := Result{}, 80.0
	for rows.Next() {
		r, err := scanRng(rows)
		if err != nil || len(r.pts) < 2 {
			continue
		}
		dist, t, left := project(r.pts, lat, lon)
		if dist >= bd {
			continue
		}
		var sd *side
		for _, x := range r.sides() {
			if x.left == left {
				sd = &x
			}
		}
		if sd == nil {
			continue
		}
		n := int(math.Round(float64(sd.from) + t*float64(sd.to-sd.from)))
		if sd.from%2 == sd.to%2 && n%2 != sd.from%2 {
			if n < max(sd.from, sd.to) {
				n++
			} else {
				n--
			}
		}
		bd = dist
		best = Result{Label: fmt.Sprintf("%d %s", n, r.street), Detail: strings.Join(nonEmpty(sd.city, sd.zip), ", "), Kind: "address", Lat: lat, Lon: lon,
			Ranged: true, Note: fmt.Sprintf("About %d, by the address range %d–%d", n, sd.from, sd.to)}
	}
	return best, best.Label != ""
}
