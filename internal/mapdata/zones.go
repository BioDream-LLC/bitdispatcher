package mapdata

import "sort"

// Zone is one emergency service zone, response area or beat from the county's GIS, as polygons of
// [lon, lat] rings: the first ring of each polygon is its outline, the rest its holes.
type Zone struct {
	Name  string `json:"name"`
	polys [][][]Pt
}

// Zones are the county's zones in the recorded map, read once. A map without county zones has none.
func (s *Store) Zones() ([]Zone, error) {
	s.zmu.Lock()
	defer s.zmu.Unlock()
	if s.zones != nil {
		return *s.zones, nil
	}
	rows, err := s.db.Query(`SELECT name, geom FROM feat WHERE layer = ? AND kind = 'zone'`, LBoundary)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byName := map[string]*Zone{}
	for rows.Next() {
		var name string
		var b []byte
		if err := rows.Scan(&name, &b); err != nil {
			return nil, err
		}
		g, err := DecodeGeom(b)
		if err != nil {
			continue
		}
		z := byName[name]
		if z == nil {
			z = &Zone{Name: name}
			byName[name] = z
		}
		z.polys = append(z.polys, groupRings(g.Parts)...)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []Zone{}
	for _, z := range byName {
		out = append(out, *z)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	s.zones = &out
	return out, nil
}

// GeoJSON is the zones as a feature collection of multipolygons, for the map.
func ZonesGeoJSON(zs []Zone) map[string]any {
	feats := []any{}
	for _, z := range zs {
		var mp [][][][2]float64
		for _, p := range z.polys {
			var poly [][][2]float64
			for _, r := range p {
				ring := make([][2]float64, 0, len(r)+1)
				for _, pt := range r {
					la, lo := pt.Deg()
					ring = append(ring, [2]float64{lo, la})
				}
				if len(ring) > 0 && ring[0] != ring[len(ring)-1] {
					ring = append(ring, ring[0])
				}
				poly = append(poly, ring)
			}
			mp = append(mp, poly)
		}
		feats = append(feats, map[string]any{"type": "Feature", "properties": map[string]any{"name": z.Name},
			"geometry": map[string]any{"type": "MultiPolygon", "coordinates": mp}})
	}
	return map[string]any{"type": "FeatureCollection", "features": feats}
}

// ZoneAt is the name of the zone a point is in, or "".
func (s *Store) ZoneAt(lat, lon float64) string {
	zs, err := s.Zones()
	if err != nil {
		return ""
	}
	p := PtDeg(lat, lon)
	for _, z := range zs {
		for _, poly := range z.polys {
			if len(poly) > 0 && inRing(p, poly[0]) {
				in := true
				for _, h := range poly[1:] {
					if inRing(p, h) {
						in = false
					}
				}
				if in {
					return z.Name
				}
			}
		}
	}
	return ""
}

// groupRings makes polygons from rings: a ring inside another (by its first point) is that one's hole.
func groupRings(rings [][]Pt) [][][]Pt {
	var outers []int
	holeOf := map[int]int{}
	for i, r := range rings {
		if len(r) < 3 {
			continue
		}
		inside := -1
		for j, o := range rings {
			if i != j && len(o) >= 3 && inRing(r[0], o) && ringArea(o) > ringArea(r) {
				inside = j
			}
		}
		if inside >= 0 {
			holeOf[i] = inside
		} else {
			outers = append(outers, i)
		}
	}
	var out [][][]Pt
	for _, o := range outers {
		poly := [][]Pt{rings[o]}
		for h, of := range holeOf {
			if of == o {
				poly = append(poly, rings[h])
			}
		}
		out = append(out, poly)
	}
	return out
}
