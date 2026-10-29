// Package gis imports a county's own GIS layers (address points, hydrants, emergency service zones,
// place names, road centrelines) from an ArcGIS REST service or a GeoJSON file into a store beside the
// recorded map. County data is authoritative where the county keeps it: its addresses are searched first.
package gis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/mapdata"
	"github.com/biodream-llc/bitdispatcher/internal/overture"
	"github.com/biodream-llc/bitdispatcher/internal/table"
)

// Layer is one configured layer.
type Layer struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Kind   string            `json:"kind"`           // address, hydrant, zone, place, road
	Source string            `json:"source"`         // arcgis, geojson, or overture
	URL    string            `json:"url,omitempty"`  // arcgis: the layer; overture: the release (empty: the latest)
	File   string            `json:"file,omitempty"` // an uploaded GeoJSON file, by name
	Where  string            `json:"where,omitempty"`
	Fields map[string]string `json:"fields"` // our field (num, street, full, unit, city, zip, name, detail) -> theirs
	On     bool              `json:"on"`
}

// Fields each kind uses.
var KindFields = map[string][]string{
	mapdata.GISAddress: {"num", "street", "full", "unit", "city", "zip"},
	mapdata.GISHydrant: {"name", "detail"},
	mapdata.GISZone:    {"name", "detail"},
	mapdata.GISPlace:   {"name"},
	mapdata.GISRoad:    {"name", "fromLeft", "toLeft", "fromRight", "toRight", "cityLeft", "cityRight", "zipLeft", "zipRight"},
}

// guesses are common county field names for each of ours, best first.
var guesses = map[string][]string{
	"num":    {"ADDRNUM", "ADD_NUMBER", "ADDNUM", "HOUSENUM", "HOUSE_NUM", "ST_NUM", "STNUM", "NUMBER", "ADDRESS_NUMBER", "ADDR_NUM"},
	"street": {"FULLNAME", "FULL_STREET", "ST_FULL", "STREETNAME", "STREET_NAME", "STREET", "ST_NAME", "FULLSTREET", "RD_NAME"},
	"full":   {"FULLADDR", "FULL_ADDRESS", "FULLADDRESS", "ADDRESS", "SITEADDR", "SITE_ADDR"},
	"unit":   {"UNITID", "UNIT", "UNIT_NUM", "APT", "SUITE"},
	"city":   {"MSAG_COMMUNITY", "MUNICIPALITY", "CITY", "POSTCOMM", "PO_NAME", "COMMUNITY", "PLACENAME"},
	"zip":    {"ZIP_5", "ZIP", "ZIPCODE", "POSTCODE", "ZIP5"},
	"name":   {"NAME", "FACILITYID", "HYDRANT_ID", "HYDRANTID", "ASSETID", "ESZ", "ESZ_NAME", "ESN", "ZONE", "BEAT", "DISTRICT", "LABEL", "FULLNAME", "PLACE_NAME", "PLACENAME", "COMMONNAME"},
	"detail": {"ESN", "AGENCY", "FIRE", "EMS", "LAW", "FLOW", "COLOR", "STATUS", "TYPE"},
	// Centreline address ranges: NENA's names first, then common county ones.
	"fromLeft":  {"ADD_LF", "FROMLEFT", "L_F_ADD", "LFROMADD", "FROM_LEFT", "LEFTFROM", "L_ADD_FROM", "FRADDL", "LF_ADD", "LOWLEFT", "LEFT_FROM"},
	"toLeft":    {"ADD_LT", "TOLEFT", "L_T_ADD", "LTOADD", "TO_LEFT", "LEFTTO", "L_ADD_TO", "TOADDL", "LT_ADD", "HIGHLEFT", "LEFT_TO"},
	"fromRight": {"ADD_RF", "FROMRIGHT", "R_F_ADD", "RFROMADD", "FROM_RIGHT", "RIGHTFROM", "R_ADD_FROM", "FRADDR", "RF_ADD", "LOWRIGHT", "RIGHT_FROM"},
	"toRight":   {"ADD_RT", "TORIGHT", "R_T_ADD", "RTOADD", "TO_RIGHT", "RIGHTTO", "R_ADD_TO", "TOADDR", "RT_ADD", "HIGHRIGHT", "RIGHT_TO"},
	"cityLeft":  {"MSAG_COMML", "PO_L", "POSTCOMM_L", "CITY_L", "MUNI_L", "COMM_L", "PLACE_L"},
	"cityRight": {"MSAG_COMMR", "PO_R", "POSTCOMM_R", "CITY_R", "MUNI_R", "COMM_R", "PLACE_R"},
	"zipLeft":   {"ZIP_L", "ZIPL", "ZIP_LEFT", "POSTCODE_L", "L_ZIP"},
	"zipRight":  {"ZIP_R", "ZIPR", "ZIP_RIGHT", "POSTCODE_R", "R_ZIP"},
}

// kindGuesses come before the general ones for a kind.
var kindGuesses = map[string][]string{
	"hydrant.name":   {"HYDRANT_ID", "HYDRANTID", "FACILITYID", "ASSETID", "HYD_NUM"},
	"hydrant.detail": {"FLOW", "GPM", "COLOR", "STATUS"},
	"zone.name":      {"ESZ", "ESZ_NAME", "ZONE", "BEAT", "DISTRICT", "NAME"},
	"zone.detail":    {"ESN", "AGENCY"},
	"road.name":      {"ST_FULL", "FULLNAME", "FULL_STREET", "STREETNAME", "STREET_NAME", "RD_NAME", "NAME", "LABEL"},
}

// Guess proposes a field mapping for a kind from a layer's field names.
func Guess(kind string, fields []string) map[string]string {
	up := map[string]string{}
	for _, f := range fields {
		up[strings.ToUpper(f)] = f
	}
	out := map[string]string{}
	used := map[string]bool{}
	for _, ours := range KindFields[kind] {
		list := guesses[ours]
		if k, ok := kindGuesses[kind+"."+ours]; ok {
			list = append(k, list...)
		}
		for _, g := range list {
			if f, ok := up[g]; ok && !used[f] {
				out[ours], used[f] = f, true
				break
			}
		}
	}
	return out
}

// Feature is one record read from a source, in degrees, before mapping.
type Feature struct {
	Props map[string]any
	Point *[2]float64
	Lines [][][2]float64
	Polys [][][][2]float64
}

// Info describes a source before it is imported.
type Info struct {
	Name     string            `json:"name"`
	Geometry string            `json:"geometry"` // point, line, polygon
	Fields   []string          `json:"fields"`
	Count    int               `json:"count"` // in the area, when the source can say
	Sample   []map[string]any  `json:"sample"`
	Guess    map[string]string `json:"guess"`
	Kind     string            `json:"kind"` // a guess from the geometry and the name
}

var client = &http.Client{Timeout: 60 * time.Second}

// allowLocal lets tests use a server on this machine.
var allowLocal = false

// CheckURL refuses addresses the server should never fetch: anything but https, and hosts on the machine
// itself or link-local (cloud metadata).
func CheckURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, errors.New("the address must be an https:// ArcGIS REST layer, such as https://…/FeatureServer/0")
	}
	host := u.Hostname()
	if allowLocal {
		return u, nil
	}
	if ips, err := net.LookupIP(host); err == nil {
		for _, ip := range ips {
			if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
				return nil, errors.New("that address is on this machine or link-local")
			}
		}
	}
	return u, nil
}

var reRelease = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\.\d+$`)

var reLayer = regexp.MustCompile(`/(FeatureServer|MapServer)/\d+/?$`)

// getJSON asks a GIS server, trying again after a pause when it does not answer or is busy: county
// servers often time out under load.
func getJSON(ctx context.Context, agent, u string, form url.Values, v any) error {
	var err error
	for try := 0; try < 4; try++ {
		if try > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(5*try) * time.Second):
			}
		}
		var retry bool
		if retry, err = getJSONOnce(ctx, agent, u, form, v); err == nil || !retry {
			return err
		}
	}
	return err
}

func getJSONOnce(ctx context.Context, agent, u string, form url.Values, v any) (retry bool, _ error) {
	var req *http.Request
	var err error
	if form != nil {
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
		if req != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	} else {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	}
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", agent)
	resp, err := client.Do(req)
	if err != nil {
		return ctx.Err() == nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return resp.StatusCode == 429 || resp.StatusCode >= 500, fmt.Errorf("%s answered %s", req.URL.Host, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	if err != nil {
		return ctx.Err() == nil, err
	}
	var e struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(b, &e) == nil && e.Error != nil {
		return false, fmt.Errorf("the service says: %s", e.Error.Message)
	}
	return false, json.Unmarshal(b, v)
}

type esriField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func envelope(a mapdata.Area) string {
	b := a.Box()
	return fmt.Sprintf("%f,%f,%f,%f", b.MinLon, b.MinLat, b.MaxLon, b.MaxLat)
}

// InspectArcGIS reads a layer's description and a few records inside the area.
func InspectArcGIS(ctx context.Context, agent, raw string, area mapdata.Area) (Info, error) {
	u, err := CheckURL(raw)
	if err != nil {
		return Info{}, err
	}
	base := strings.TrimSuffix(u.String(), "/")
	if !reLayer.MatchString(base) {
		return Info{}, errors.New("give the layer's address, ending in /FeatureServer/0 or /MapServer/0")
	}
	var d struct {
		Name         string      `json:"name"`
		GeometryType string      `json:"geometryType"`
		Fields       []esriField `json:"fields"`
	}
	if err := getJSON(ctx, agent, base+"?f=json", nil, &d); err != nil {
		return Info{}, err
	}
	info := Info{Name: d.Name, Fields: []string{}, Sample: []map[string]any{}}
	switch d.GeometryType {
	case "esriGeometryPoint", "esriGeometryMultipoint":
		info.Geometry = "point"
	case "esriGeometryPolyline":
		info.Geometry = "line"
	case "esriGeometryPolygon":
		info.Geometry = "polygon"
	default:
		return info, fmt.Errorf("the layer has no usable geometry (%s)", d.GeometryType)
	}
	for _, f := range d.Fields {
		info.Fields = append(info.Fields, f.Name)
	}
	var cnt struct {
		Count int `json:"count"`
	}
	q := url.Values{"where": {"1=1"}, "geometry": {envelope(area)}, "geometryType": {"esriGeometryEnvelope"}, "inSR": {"4326"},
		"spatialRel": {"esriSpatialRelIntersects"}, "returnCountOnly": {"true"}, "f": {"json"}}
	getJSON(ctx, agent, base+"/query", q, &cnt)
	info.Count = cnt.Count
	var s struct {
		Features []struct {
			Attributes map[string]any `json:"attributes"`
		} `json:"features"`
	}
	q = url.Values{"where": {"1=1"}, "geometry": {envelope(area)}, "geometryType": {"esriGeometryEnvelope"}, "inSR": {"4326"},
		"spatialRel": {"esriSpatialRelIntersects"}, "outFields": {"*"}, "returnGeometry": {"false"}, "resultRecordCount": {"5"}, "f": {"json"}}
	getJSON(ctx, agent, base+"/query", q, &s)
	for _, f := range s.Features {
		info.Sample = append(info.Sample, f.Attributes)
	}
	info.Kind = guessKind(info.Name, info.Geometry)
	info.Guess = Guess(info.Kind, info.Fields)
	return info, nil
}

func guessKind(name, geom string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "hydrant"):
		return mapdata.GISHydrant
	case strings.Contains(n, "address"), strings.Contains(n, "site"):
		return mapdata.GISAddress
	case geom == "polygon":
		return mapdata.GISZone
	case geom == "line":
		return mapdata.GISRoad
	}
	if geom == "point" {
		return mapdata.GISPlace
	}
	return mapdata.GISAddress
}

// readArcGIS fetches every record of a layer in the area: the ids first, then the records in batches, so
// services that cap a query's result still give everything.
func readArcGIS(ctx context.Context, agent string, l Layer, area mapdata.Area, prog func(done, total int), fn func(Feature)) error {
	u, err := CheckURL(l.URL)
	if err != nil {
		return err
	}
	base := strings.TrimSuffix(u.String(), "/")
	where := l.Where
	if strings.TrimSpace(where) == "" {
		where = "1=1"
	}
	var ids struct {
		ObjectIDs []int64 `json:"objectIds"`
	}
	q := url.Values{"where": {where}, "geometry": {envelope(area)}, "geometryType": {"esriGeometryEnvelope"}, "inSR": {"4326"},
		"spatialRel": {"esriSpatialRelIntersects"}, "returnIdsOnly": {"true"}, "f": {"json"}}
	if err := getJSON(ctx, agent, base+"/query", q, &ids); err != nil {
		return err
	}
	sort.Slice(ids.ObjectIDs, func(i, j int) bool { return ids.ObjectIDs[i] < ids.ObjectIDs[j] })
	total := len(ids.ObjectIDs)
	const batch = 500
	// Only the mapped fields are asked for: a statewide address layer has dozens.
	var want []string
	for _, f := range l.Fields {
		if f != "" && !slices.Contains(want, f) {
			want = append(want, f)
		}
	}
	sort.Strings(want)
	outFields := strings.Join(want, ",")
	if outFields == "" {
		outFields = "*"
	}
	type page struct {
		Features []struct {
			Attributes map[string]any `json:"attributes"`
			Geometry   *struct {
				X      *float64      `json:"x"`
				Y      *float64      `json:"y"`
				Points [][]float64   `json:"points"`
				Paths  [][][]float64 `json:"paths"`
				Rings  [][][]float64 `json:"rings"`
			} `json:"geometry"`
		} `json:"features"`
	}
	var mu sync.Mutex
	done := 0
	work := make(chan []int64)
	errc := make(chan error, 4)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for chunk := range work {
				parts := make([]string, len(chunk))
				for i, id := range chunk {
					parts[i] = strconv.FormatInt(id, 10)
				}
				var p page
				q := url.Values{"objectIds": {strings.Join(parts, ",")}, "outFields": {outFields}, "outSR": {"4326"}, "returnGeometry": {"true"}, "f": {"json"}}
				var err error
				for attempt := range 3 {
					if attempt > 0 {
						time.Sleep(time.Duration(attempt) * 2 * time.Second)
					}
					if err = getJSON(ctx, agent, base+"/query", q, &p); err == nil {
						break
					}
				}
				if err != nil {
					select {
					case errc <- err:
					default:
					}
					continue
				}
				mu.Lock()
				for _, f := range p.Features {
					ft := Feature{Props: f.Attributes}
					if g := f.Geometry; g != nil {
						switch {
						case g.X != nil && g.Y != nil:
							ft.Point = &[2]float64{*g.X, *g.Y}
						case len(g.Points) > 0 && len(g.Points[0]) >= 2:
							ft.Point = &[2]float64{g.Points[0][0], g.Points[0][1]}
						case len(g.Paths) > 0:
							for _, path := range g.Paths {
								ft.Lines = append(ft.Lines, pairs(path))
							}
						case len(g.Rings) > 0:
							// Esri rings: outer rings clockwise, holes anticlockwise, all in one list. Each
							// ring is kept as its own polygon outline; zones are drawn as lines and labelled.
							for _, r := range g.Rings {
								ft.Polys = append(ft.Polys, [][][2]float64{pairs(r)})
							}
						}
					}
					fn(ft)
				}
				done += len(chunk)
				if prog != nil {
					prog(done, total)
				}
				mu.Unlock()
			}
		}()
	}
	for i := 0; i < total; i += batch {
		select {
		case <-ctx.Done():
			close(work)
			wg.Wait()
			return ctx.Err()
		case work <- ids.ObjectIDs[i:min(total, i+batch)]:
		}
	}
	close(work)
	wg.Wait()
	select {
	case err := <-errc:
		return fmt.Errorf("%s: %w", l.Name, err)
	default:
	}
	return nil
}

func pairs(v [][]float64) [][2]float64 {
	out := make([][2]float64, 0, len(v))
	for _, p := range v {
		if len(p) >= 2 {
			out = append(out, [2]float64{p[0], p[1]})
		}
	}
	return out
}

// ReadGeoJSON reads a FeatureCollection in WGS 84 (what county open-data portals export).
func ReadGeoJSON(r io.Reader, fn func(Feature)) error {
	var fc struct {
		Type     string `json:"type"`
		Features []struct {
			Properties map[string]any `json:"properties"`
			Geometry   *struct {
				Type        string          `json:"type"`
				Coordinates json.RawMessage `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := json.NewDecoder(io.LimitReader(r, 1<<30)).Decode(&fc); err != nil {
		return fmt.Errorf("not GeoJSON: %w", err)
	}
	if fc.Type != "FeatureCollection" {
		return errors.New("the file must be a GeoJSON FeatureCollection")
	}
	for _, f := range fc.Features {
		ft := Feature{Props: f.Properties}
		if g := f.Geometry; g != nil {
			switch g.Type {
			case "Point":
				var c []float64
				if json.Unmarshal(g.Coordinates, &c) == nil && len(c) >= 2 {
					ft.Point = &[2]float64{c[0], c[1]}
				}
			case "MultiPoint":
				var c [][]float64
				if json.Unmarshal(g.Coordinates, &c) == nil && len(c) > 0 && len(c[0]) >= 2 {
					ft.Point = &[2]float64{c[0][0], c[0][1]}
				}
			case "LineString":
				var c [][]float64
				json.Unmarshal(g.Coordinates, &c)
				ft.Lines = append(ft.Lines, pairs(c))
			case "MultiLineString":
				var c [][][]float64
				json.Unmarshal(g.Coordinates, &c)
				for _, l := range c {
					ft.Lines = append(ft.Lines, pairs(l))
				}
			case "Polygon":
				var c [][][]float64
				json.Unmarshal(g.Coordinates, &c)
				var rings [][][2]float64
				for _, r := range c {
					rings = append(rings, pairs(r))
				}
				ft.Polys = append(ft.Polys, rings)
			case "MultiPolygon":
				var c [][][][]float64
				json.Unmarshal(g.Coordinates, &c)
				for _, pg := range c {
					var rings [][][2]float64
					for _, r := range pg {
						rings = append(rings, pairs(r))
					}
					ft.Polys = append(ft.Polys, rings)
				}
			}
		}
		fn(ft)
	}
	return nil
}

// InspectGeoJSON describes an uploaded file.
func InspectGeoJSON(path string) (Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return Info{}, err
	}
	defer f.Close()
	info := Info{Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), Fields: []string{}, Sample: []map[string]any{}}
	seen := map[string]bool{}
	err = ReadGeoJSON(f, func(ft Feature) {
		info.Count++
		if info.Geometry == "" {
			switch {
			case ft.Point != nil:
				info.Geometry = "point"
			case len(ft.Lines) > 0:
				info.Geometry = "line"
			case len(ft.Polys) > 0:
				info.Geometry = "polygon"
			}
		}
		for k := range ft.Props {
			if !seen[k] {
				seen[k] = true
				info.Fields = append(info.Fields, k)
			}
		}
		if len(info.Sample) < 5 {
			info.Sample = append(info.Sample, ft.Props)
		}
	})
	sort.Strings(info.Fields)
	info.Kind = guessKind(info.Name, info.Geometry)
	info.Guess = Guess(info.Kind, info.Fields)
	return info, err
}

func str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		return x.String()
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

var reNumStreet = regexp.MustCompile(`^\s*(\d+[A-Za-z]?(?:-\d+)?)\s+(.+?)\s*$`)

// Map turns a source feature into a store feature by the layer's field mapping.
func Map(l Layer, f Feature) mapdata.GISFeature {
	get := func(k string) string {
		if src, ok := l.Fields[k]; ok && src != "" {
			return str(f.Props[src])
		}
		return ""
	}
	g := mapdata.GISFeature{Kind: l.Kind, Point: f.Point, Lines: f.Lines, Polys: f.Polys}
	switch l.Kind {
	case mapdata.GISAddress:
		g.Num, g.Street, g.Unit, g.City, g.Zip = get("num"), get("street"), get("unit"), get("city"), get("zip")
		if full := get("full"); full != "" && (g.Num == "" || g.Street == "") {
			if m := reNumStreet.FindStringSubmatch(full); m != nil {
				if g.Num == "" {
					g.Num = m[1]
				}
				if g.Street == "" {
					g.Street = m[2]
				}
			}
		}
		if g.Num == "0" {
			g.Num = ""
		}
	case mapdata.GISRoad:
		g.Name = get("name")
		num := func(k string) int {
			n, _ := strconv.Atoi(strings.TrimSpace(strings.SplitN(get(k), ".", 2)[0]))
			return max(n, 0)
		}
		g.LF, g.LT, g.RF, g.RT = num("fromLeft"), num("toLeft"), num("fromRight"), num("toRight")
		g.City, g.CityR, g.Zip, g.ZipR = get("cityLeft"), get("cityRight"), get("zipLeft"), get("zipRight")
	default:
		g.Name, g.Detail = get("name"), get("detail")
	}
	return g
}

// Job is an import in progress or finished.
type Job struct {
	Started  time.Time         `json:"started"`
	Finished *time.Time        `json:"finished,omitempty"`
	By       string            `json:"by"`
	Layer    string            `json:"layer"` // the one being read
	Done     int               `json:"done"`
	Total    int               `json:"total"`
	Error    string            `json:"error,omitempty"`
	Stats    *mapdata.GISStats `json:"stats,omitempty"`
	cancel   context.CancelFunc
}

// Service keeps the layers, the uploads and the GIS store.
type Service struct {
	Dir          string
	Agent        string
	Log          *slog.Logger
	Imported     func(Job)
	Failed       func(Job)
	OvertureBase string // where Overture releases are read from; empty: its public bucket

	mu      sync.RWMutex
	layers  []Layer
	cur     *mapdata.Store
	curPath string
	job     *Job
	retired map[*mapdata.Store]string // replaced layers, closed and deleted once requests using them finish
}

// Close closes the imported layers in use, and deletes replaced ones at once.
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for st, p := range s.retired {
		st.Close()
		removeStore(p)
		delete(s.retired, st)
	}
	if s.cur == nil {
		return nil
	}
	err := s.cur.Close()
	s.cur, s.curPath = nil, ""
	return err
}

// retire closes and deletes replaced layers, unless Close already has.
func (s *Service) retire(st *mapdata.Store) {
	s.mu.Lock()
	p, ok := s.retired[st]
	delete(s.retired, st)
	s.mu.Unlock()
	if ok {
		st.Close()
		removeStore(p)
	}
}

// NewService opens dir (data/gis).
func NewService(dir, agent string, log *slog.Logger) (*Service, error) {
	if err := os.MkdirAll(filepath.Join(dir, "uploads"), 0o700); err != nil {
		return nil, err
	}
	s := &Service{Dir: dir, Agent: agent, Log: log, layers: []Layer{}}
	if b, err := os.ReadFile(filepath.Join(dir, "layers.json")); err == nil {
		json.Unmarshal(b, &s.layers)
	}
	stores, _ := filepath.Glob(filepath.Join(dir, "gis-*.db"))
	sort.Sort(sort.Reverse(sort.StringSlice(stores)))
	for _, p := range stores {
		if st, err := mapdata.Open(p); err == nil {
			s.cur, s.curPath = st, p
			break
		}
	}
	for _, p := range stores {
		if p != s.curPath {
			removeStore(p)
		}
	}
	return s, nil
}

func removeStore(p string) {
	for _, x := range []string{p, p + "-wal", p + "-shm"} {
		os.Remove(x)
	}
}

// Store is the GIS store, or nil.
func (s *Service) Store() *mapdata.Store {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// Layers lists the configured layers.
func (s *Service) Layers() []Layer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Layer{}, s.layers...)
}

var reID = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)

// SaveLayers replaces the layer list.
func (s *Service) SaveLayers(ls []Layer) error {
	for i, l := range ls {
		if !reID.MatchString(l.ID) {
			return fmt.Errorf("layer %d: the id is lower-case letters, digits and dashes", i+1)
		}
		if _, ok := KindFields[l.Kind]; !ok {
			return fmt.Errorf("%s: unknown kind %q", l.ID, l.Kind)
		}
		switch l.Source {
		case "arcgis":
			if _, err := CheckURL(l.URL); err != nil {
				return fmt.Errorf("%s: %w", l.ID, err)
			}
		case "geojson":
			if _, err := s.UploadPath(l.File); err != nil {
				return fmt.Errorf("%s: %w", l.ID, err)
			}
		case "csv":
			if _, err := s.UploadPath(l.File); err != nil {
				return fmt.Errorf("%s: %w", l.ID, err)
			}
			if l.Fields["lat"] == "" || l.Fields["lon"] == "" {
				return fmt.Errorf("%s: match the latitude and longitude columns", l.ID)
			}
		case "overture":
			if l.Kind != mapdata.GISAddress {
				return fmt.Errorf("%s: Overture gives address points", l.ID)
			}
			if l.URL != "" && !reRelease.MatchString(l.URL) {
				return fmt.Errorf("%s: an Overture release is named like 2026-09-23.1 (empty: the latest)", l.ID)
			}
		default:
			return fmt.Errorf("%s: the source is arcgis, geojson, csv or overture", l.ID)
		}
	}
	b, _ := json.MarshalIndent(ls, "", "  ")
	if err := os.WriteFile(filepath.Join(s.Dir, "layers.json"), b, 0o600); err != nil {
		return err
	}
	s.mu.Lock()
	s.layers = ls
	s.mu.Unlock()
	return nil
}

var reFile = regexp.MustCompile(`^[A-Za-z0-9._-]{1,80}\.(geojson|json|csv|tsv|txt)$`)

// UploadPath is where an uploaded file lives; the name is checked.
func (s *Service) UploadPath(name string) (string, error) {
	if !reFile.MatchString(name) || strings.Contains(name, "..") {
		return "", errors.New("upload a GeoJSON file (.geojson, .json) or a table with latitude and longitude columns (.csv)")
	}
	return filepath.Join(s.Dir, "uploads", name), nil
}

// Job is the running or last import.
func (s *Service) Job() *Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.job == nil {
		return nil
	}
	j := *s.job
	return &j
}

// Import reads every layer that is on into a new GIS store, in the background.
func (s *Service) Import(area mapdata.Area, by string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job != nil && s.job.Finished == nil {
		return errors.New("an import is already running")
	}
	var on []Layer
	for _, l := range s.layers {
		if l.On {
			on = append(on, l)
		}
	}
	if len(on) == 0 {
		return errors.New("switch on at least one layer")
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &Job{Started: time.Now().UTC(), By: by, cancel: cancel}
	s.job = j
	go s.run(ctx, j, area, on)
	return nil
}

// Cancel stops the import; the store in use stays.
func (s *Service) Cancel() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil || s.job.Finished != nil {
		return false
	}
	s.job.cancel()
	return true
}

func (s *Service) run(ctx context.Context, j *Job, area mapdata.Area, layers []Layer) {
	path := filepath.Join(s.Dir, "gis-"+time.Now().UTC().Format("20060102T150405")+".db")
	var names []string
	for _, l := range layers {
		names = append(names, l.Name)
	}
	st, err := mapdata.BuildGIS(ctx, path, area, "County GIS: "+strings.Join(names, ", "), func(emit func(mapdata.GISFeature)) error {
		for _, l := range layers {
			s.mu.Lock()
			j.Layer, j.Done, j.Total = l.Name, 0, 0
			s.mu.Unlock()
			each := func(f Feature) { emit(Map(l, f)) }
			switch l.Source {
			case "arcgis":
				if err := readArcGIS(ctx, s.Agent, l, area, func(d, t int) { s.mu.Lock(); j.Done, j.Total = d, t; s.mu.Unlock() }, each); err != nil {
					return err
				}
			case "csv":
				p, err := s.UploadPath(l.File)
				if err != nil {
					return err
				}
				if err := ReadCSV(p, l.Fields["lat"], l.Fields["lon"], each); err != nil {
					return fmt.Errorf("%s: %w", l.Name, err)
				}
			case "overture":
				// Overture's addresses: the latest release unless one is named.
				c := overture.Client{Agent: s.Agent, Base: s.OvertureBase}
				err := c.Read(ctx, l.URL, area, func(p overture.Progress) { s.mu.Lock(); j.Done, j.Total = p.FilesDone, p.Files; s.mu.Unlock() }, emit)
				if err != nil {
					return fmt.Errorf("%s: %w", l.Name, err)
				}
			case "geojson":
				p, err := s.UploadPath(l.File)
				if err != nil {
					return err
				}
				f, err := os.Open(p)
				if err != nil {
					return fmt.Errorf("%s: %w", l.Name, err)
				}
				err = ReadGeoJSON(f, each)
				f.Close()
				if err != nil {
					return fmt.Errorf("%s: %w", l.Name, err)
				}
			}
		}
		return nil
	})
	var ns *mapdata.Store
	if err == nil {
		ns, err = mapdata.Open(path)
	}
	now := time.Now().UTC()
	s.mu.Lock()
	j.Finished = &now
	if err != nil {
		j.Error = err.Error()
		if errors.Is(err, context.Canceled) {
			j.Error = "cancelled"
		}
		s.mu.Unlock()
		removeStore(path)
		if s.Failed != nil {
			s.Failed(*j)
		}
		return
	}
	j.Stats = &st
	old, oldPath := s.cur, s.curPath
	s.cur, s.curPath = ns, path
	jj := *j
	s.mu.Unlock()
	if old != nil {
		s.mu.Lock()
		if s.retired == nil {
			s.retired = map[*mapdata.Store]string{}
		}
		s.retired[old] = oldPath
		s.mu.Unlock()
		go func() {
			time.Sleep(30 * time.Second)
			s.retire(old)
		}()
	}
	if s.Imported != nil {
		s.Imported(jj)
	}
}

// ReadCSV reads a table of points: one row each, with latitude and longitude columns in degrees.
func ReadCSV(path, latCol, lonCol string, fn func(Feature)) error {
	t, err := table.Open(path)
	if err != nil {
		return err
	}
	defer t.Close()
	for {
		m, err := t.Map()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("row %d: %w", t.Row, err)
		}
		lat, e1 := strconv.ParseFloat(m[latCol], 64)
		lon, e2 := strconv.ParseFloat(m[lonCol], 64)
		if e1 != nil || e2 != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 || lat == 0 && lon == 0 {
			continue
		}
		props := make(map[string]any, len(m))
		for k, v := range m {
			props[k] = v
		}
		fn(Feature{Props: props, Point: &[2]float64{lon, lat}})
	}
}

// InspectCSV describes a table of points.
func InspectCSV(path string) (Info, error) {
	h, rows, n, err := table.Sample(path, 3)
	if err != nil {
		return Info{}, err
	}
	info := Info{Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), Geometry: "point", Fields: h, Count: n, Sample: []map[string]any{}}
	for _, r := range rows {
		m := map[string]any{}
		for k, v := range r {
			m[k] = v
		}
		info.Sample = append(info.Sample, m)
	}
	info.Kind = guessKind(info.Name, "point")
	if info.Kind == mapdata.GISPlace && len(Guess(mapdata.GISAddress, h)) >= 2 {
		info.Kind = mapdata.GISAddress
	}
	info.Guess = Guess(info.Kind, h)
	ll := table.Guess(h, map[string][]string{"lat": {"LATITUDE", "LAT", "Y", "POINT_Y", "YCOORD"}, "lon": {"LONGITUDE", "LON", "LONG", "LNG", "X", "POINT_X", "XCOORD"}}, []string{"lat", "lon"})
	if ll["lat"] == "" || ll["lon"] == "" {
		return info, errors.New("the table needs latitude and longitude columns, in degrees")
	}
	info.Guess["lat"], info.Guess["lon"] = ll["lat"], ll["lon"]
	return info, nil
}
