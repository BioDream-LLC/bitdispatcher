package mapdata

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"
)

// The county's own GIS (address points, hydrants, emergency service zones, common place names) is kept
// in a second store of the same format beside the recorded map, so it survives recording the map again
// and is searched first: the county's address points are the authoritative ones.

// GIS feature kinds.
const (
	GISAddress = "address"
	GISHydrant = "hydrant"
	GISZone    = "zone"  // emergency service zone, response area, beat
	GISPlace   = "place" // common place name: a school, a business, a park
	GISRoad    = "road"  // a road centreline: its name is searchable, its address ranges place addresses
)

// GISFeature is one feature from a county layer, in degrees.
type GISFeature struct {
	Kind   string
	Num    string // address: house number
	Street string // address: street, in full
	Unit   string
	City   string
	Zip    string
	Name   string // hydrant id, zone name, place name, road name
	Detail string // zone: its ESN or agencies; hydrant: flow or colour
	// Road centrelines: the house numbers at each end of each side, and the right side's town (the
	// left side's is City and Zip).
	LF, LT, RF, RT int
	CityR, ZipR    string
	Src            string // address: where the point comes from when not the county ("overture")
	Point          *[2]float64
	Lines          [][][2]float64
	Polys          [][][][2]float64 // polygons, each a list of rings
}

// GISStats counts what a GIS store holds.
type GISStats struct {
	ByKind  map[string]int64 `json:"byKind"`
	Skipped int64            `json:"skipped"` // outside the area or without the fields needed
	Ranges  int64            `json:"ranges"`  // centreline segments with address ranges
	Seconds float64          `json:"seconds"`
}

func toPts(ps [][2]float64) []Pt {
	out := make([]Pt, len(ps))
	for i, p := range ps {
		out[i] = PtDeg(p[1], p[0])
	}
	return out
}

// BuildGIS writes a GIS store at path from the features feed sends. Features outside the area are left out.
func BuildGIS(ctx context.Context, path string, area Area, source string, feed func(emit func(GISFeature)) error) (GISStats, error) {
	start := time.Now()
	tmp := path + ".building"
	os.Remove(tmp)
	db, err := sql.Open("sqlite", tmp)
	if err != nil {
		return GISStats{}, err
	}
	db.SetMaxOpenConns(1)
	defer func() {
		db.Close()
		os.Remove(tmp)
	}()
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return GISStats{}, fmt.Errorf("mapdata: schema: %w", err)
	}
	st := GISStats{ByKind: map[string]int64{}}
	w := newWriter(ctx, db)
	errc := make(chan error, 1)
	go func() { errc <- w.run() }()
	var id int64
	next := func() int64 { id++; return id }
	inside := func(p Pt) bool { lat, lon := p.Deg(); return area.Contains(lat, lon) }
	emit := func(f GISFeature) {
		switch f.Kind {
		case GISAddress:
			if f.Point == nil || strings.TrimSpace(f.Num) == "" || strings.TrimSpace(f.Street) == "" {
				st.Skipped++
				return
			}
			p := PtDeg(f.Point[1], f.Point[0])
			if !inside(p) {
				st.Skipped++
				return
			}
			n := next()
			w.in <- row{addr: &addrRow{id: n, num: strings.TrimSpace(f.Num), street: titleCase(f.Street), unit: f.Unit, city: titleCase(f.City), zip: f.Zip, src: f.Src, p: p}}
			w.in <- row{feat: &featRow{id: n, layer: LAddr, kind: "num", minzoom: 17, name: strings.TrimSpace(f.Num), g: Geom{Kind: GPoint, Parts: [][]Pt{{p}}}}}
		case GISHydrant:
			if f.Point == nil {
				st.Skipped++
				return
			}
			p := PtDeg(f.Point[1], f.Point[0])
			if !inside(p) {
				st.Skipped++
				return
			}
			attrs := map[string]string{}
			if f.Detail != "" {
				attrs["detail"] = f.Detail
			}
			w.in <- row{feat: &featRow{id: next(), layer: LPOI, kind: "hydrant", minzoom: 15, name: f.Name, attrs: attrs, g: Geom{Kind: GPoint, Parts: [][]Pt{{p}}}}}
		case GISPlace:
			if f.Point == nil && len(f.Polys) > 0 && len(f.Polys[0]) > 0 {
				c := centroid(f.Polys[0][0])
				f.Point = &c
			}
			if f.Point == nil || f.Name == "" {
				st.Skipped++
				return
			}
			p := PtDeg(f.Point[1], f.Point[0])
			if !inside(p) {
				st.Skipped++
				return
			}
			w.in <- row{feat: &featRow{id: next(), layer: LPOI, kind: "civic", minzoom: 14, name: f.Name, g: Geom{Kind: GPoint, Parts: [][]Pt{{p}}}}}
			w.in <- row{name: &nameRow{name: f.Name, kind: "poi", layer: LPOI, p: p}}
		case GISZone:
			if len(f.Polys) == 0 {
				st.Skipped++
				return
			}
			var parts [][]Pt
			var poly [][][]Pt
			touches := false
			for _, pg := range f.Polys {
				var rings [][]Pt
				for _, r := range pg {
					pts := toPts(r)
					for _, p := range pts {
						touches = touches || inside(p)
					}
					parts = append(parts, pts)
					rings = append(rings, pts)
				}
				poly = append(poly, rings)
			}
			if !touches {
				st.Skipped++
				return
			}
			name := strings.TrimSpace(strings.TrimSpace(f.Name + " " + f.Detail))
			w.in <- row{feat: &featRow{id: next(), layer: LBoundary, kind: "zone", minzoom: 10, name: name, g: Geom{Kind: GLine, Parts: parts}}}
			if name != "" {
				w.in <- row{feat: &featRow{id: next(), layer: LLabel, kind: "zone", minzoom: 11, name: name, g: Geom{Kind: GPoint, Parts: [][]Pt{{labelPoint(poly)}}}}}
			}
		case GISRoad:
			if len(f.Lines) == 0 || f.Name == "" {
				st.Skipped++
				return
			}
			pts := toPts(f.Lines[0])
			mid := pts[len(pts)/2]
			if !inside(mid) {
				st.Skipped++
				return
			}
			w.in <- row{name: &nameRow{name: titleCase(f.Name), kind: "street", layer: LRoad, p: mid}}
			if f.LF > 0 || f.LT > 0 || f.RF > 0 || f.RT > 0 {
				for _, l := range f.Lines {
					if len(l) < 2 {
						continue
					}
					w.in <- row{rng: &rngRow{street: titleCase(f.Name), lf: f.LF, lt: f.LT, rf: f.RF, rt: f.RT, cityL: titleCase(f.City), cityR: titleCase(f.CityR),
						zipL: f.Zip, zipR: f.ZipR, pts: toPts(l)}}
					st.Ranges++
				}
			}
		default:
			st.Skipped++
			return
		}
		st.ByKind[f.Kind]++
	}
	ferr := feed(emit)
	close(w.in)
	if err := <-errc; err != nil {
		return st, err
	}
	if ferr != nil {
		return st, ferr
	}
	if _, err := db.ExecContext(ctx, indexes); err != nil {
		return st, fmt.Errorf("mapdata: indexes: %w", err)
	}
	st.Seconds = math.Round(time.Since(start).Seconds())
	stats := Stats{ByLayer: map[string]int64{}}
	db.QueryRowContext(ctx, `SELECT count(*) FROM addr`).Scan(&stats.Addresses)
	meta := map[string]any{"area": area, "built": time.Now().UTC().Format(time.RFC3339), "source": source, "stats": stats, "gis": st, "format": 1}
	for k, v := range meta {
		j, _ := json.Marshal(v)
		if _, err := db.ExecContext(ctx, `INSERT INTO meta(k, v) VALUES(?, ?)`, k, string(j)); err != nil {
			return st, err
		}
	}
	if err := db.Close(); err != nil {
		return st, err
	}
	return st, os.Rename(tmp, path)
}

func centroid(r [][2]float64) [2]float64 {
	var x, y float64
	for _, p := range r {
		x += p[0]
		y += p[1]
	}
	n := float64(len(r))
	return [2]float64{x / n, y / n}
}

// titleCase turns the upper-case names county data often uses ("KING GEORGE AVE SW") into the form the
// map uses ("King George Ave SW"): directions and short codes stay upper case.
func titleCase(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if strings.ToUpper(s) != s {
		// Already mixed case: only the quadrant suffixes some sources write "Sw" go back to capitals.
		f := strings.Fields(s)
		for i, w := range f {
			switch strings.ToUpper(w) {
			case "NE", "NW", "SE", "SW":
				f[i] = strings.ToUpper(w)
			}
		}
		return strings.Join(f, " ")
	}
	keep := map[string]bool{"NE": true, "NW": true, "SE": true, "SW": true, "N": true, "S": true, "E": true, "W": true, "US": true, "VA": true, "NC": true, "WV": true, "II": true, "III": true}
	f := strings.Fields(strings.ToLower(s))
	for i, w := range f {
		if keep[strings.ToUpper(w)] {
			f[i] = strings.ToUpper(w)
			continue
		}
		if len(w) > 0 && w[0] >= '0' && w[0] <= '9' {
			continue // 1st, 10th
		}
		f[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(f, " ")
}
