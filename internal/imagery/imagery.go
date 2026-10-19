// Package imagery records aerial photos for the dispatch map: Web Mercator photo tiles from public
// orthoimagery services, kept in bitdispatcher's own store so the photos show with no internet.
//
// Dispatchers navigate by rooftops ("the white house behind the church, second driveway"), so the photo
// base matters as much as the street map. Photos are recorded in tiers: full detail near the centre,
// where most calls are, and less detail out to the edge of the recorded map, because full detail over a
// 100-mile circle is hundreds of gigabytes.
package imagery

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png" // some services send PNG
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// Source is a public photo tile service.
type Source struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	URL         string     `json:"url"` // {z}, {y}, {x}
	MaxZoom     int        `json:"maxZoom"`
	Resolution  string     `json:"resolution"`
	Attribution string     `json:"attribution"`
	Short       string     `json:"short"`
	Terms       string     `json:"terms"`
	BBox        [4]float64 `json:"bbox"` // west, south, east, north
	// DateIndex is an ArcGIS map service whose layers hold the photo footprints with Year and Product
	// fields (VGIN's tile index), so the console can say how old the photo under the cursor is.
	DateIndex string `json:"-"`
}

// Catalog lists the sources bitdispatcher knows, best first. A tile is taken from the first source that
// covers it and has it.
var Catalog = []Source{
	{
		ID: "vgin", Name: "Virginia Base Mapping Program (VGIN)",
		URL:     "https://vginmaps.vdem.virginia.gov/arcgis/rest/services/VBMP_Imagery/MostRecentImagery_WGS/MapServer/tile/{z}/{y}/{x}",
		MaxZoom: 19, Resolution: "1 ft statewide, 6 and 3 in in some cities; spring 2022 to 2025",
		Attribution: "Imagery © Virginia Geographic Information Network (VGIN)", Short: "© VGIN",
		Terms: "Commonwealth of Virginia orthophotography, administered by VGIN and provided to Virginia localities " +
			"at no cost. Use it for a Virginia answering point; ask VGIN before using it elsewhere.",
		BBox:      [4]float64{-83.70, 36.53, -75.15, 39.47},
		DateIndex: "https://vginmaps.vdem.virginia.gov/arcgis/rest/services/VBMP_Imagery/MostRecentImagery_WGS_Tile_Index/MapServer",
	},
	{
		ID: "usgs", Name: "USGS The National Map (NAIP and partner imagery)",
		URL:     "https://basemap.nationalmap.gov/arcgis/rest/services/USGSImageryOnly/MapServer/tile/{z}/{y}/{x}",
		MaxZoom: 16, Resolution: "about 1 m (NAIP), finer in some places",
		Attribution: "Imagery: USGS The National Map, USDA NAIP", Short: "USGS/NAIP",
		Terms: "Public domain (US Government work).",
		BBox:  [4]float64{-179.9, 17.5, -64.5, 71.5},
	},
}

// SourceByID finds a catalogue source.
func SourceByID(id string) (Source, bool) {
	for _, s := range Catalog {
		if s.ID == id {
			return s, true
		}
	}
	return Source{}, false
}

func (s Source) covers(lat, lon float64) bool {
	return lon >= s.BBox[0] && lon <= s.BBox[2] && lat >= s.BBox[1] && lat <= s.BBox[3]
}

// Tile maths.

func tileLon(x, z int) float64 { return float64(x)/math.Exp2(float64(z))*360 - 180 }
func tileLat(y, z int) float64 {
	n := math.Pi - 2*math.Pi*float64(y)/math.Exp2(float64(z))
	return 180 / math.Pi * math.Atan(math.Sinh(n))
}
func tileXY(lat, lon float64, z int) (int, int) {
	n := math.Exp2(float64(z))
	x := int((lon + 180) / 360 * n)
	r := lat * math.Pi / 180
	y := int((1 - math.Log(math.Tan(r)+1/math.Cos(r))/math.Pi) / 2 * n)
	return clampInt(x, 0, int(n)-1), clampInt(y, 0, int(n)-1)
}
func clampInt(v, lo, hi int) int { return max(lo, min(hi, v)) }

// distance in metres (haversine).
func distance(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371008.8
	p1, p2 := lat1*math.Pi/180, lat2*math.Pi/180
	dp, dl := p2-p1, (lon2-lon1)*math.Pi/180
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}

// tileDist is the distance from a point to the nearest point of a tile.
func tileDist(lat, lon float64, x, y, z int) float64 {
	w, e := tileLon(x, z), tileLon(x+1, z)
	n, s := tileLat(y, z), tileLat(y+1, z)
	return distance(lat, lon, math.Max(s, math.Min(n, lat)), math.Max(w, math.Min(e, lon)))
}

// Circle lists the tiles at zoom z that touch a circle, nearest the centre first.
func Circle(lat, lon, radiusM float64, z int) [][2]int {
	dLat := radiusM / 111320
	dLon := radiusM / (111320 * math.Cos(lat*math.Pi/180))
	x0, y0 := tileXY(lat+dLat, lon-dLon, z)
	x1, y1 := tileXY(lat-dLat, lon+dLon, z)
	var out [][2]int
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if tileDist(lat, lon, x, y, z) <= radiusM {
				out = append(out, [2]int{x, y})
			}
		}
	}
	cx, cy := tileXY(lat, lon, z)
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		return (a[0]-cx)*(a[0]-cx)+(a[1]-cy)*(a[1]-cy) < (b[0]-cx)*(b[0]-cx)+(b[1]-cy)*(b[1]-cy)
	})
	return out
}

// countCircle estimates the tiles at zoom z touching a circle without listing them.
func countCircle(lat, radiusM float64, z int) int {
	side := 40075016.7 * math.Cos(lat*math.Pi/180) / math.Exp2(float64(z))
	r := radiusM + side*0.5
	return int(math.Pi*r*r/(side*side)) + 1
}

// Plan is what a recording will fetch.
type Plan struct {
	Lat         float64  `json:"lat"`
	Lon         float64  `json:"lon"`
	RadiusM     float64  `json:"radiusM"`     // the whole area, at OuterZoom
	CoreRadiusM float64  `json:"coreRadiusM"` // full detail, at CoreZoom
	OuterZoom   int      `json:"outerZoom"`
	CoreZoom    int      `json:"coreZoom"`
	Rate        float64  `json:"rate"` // requests per second
	Sources     []string `json:"sources"`
	Tiles       int      `json:"tiles"`
	Bytes       int64    `json:"bytes"`   // estimate
	Seconds     float64  `json:"seconds"` // estimate
	Levels      []Level  `json:"levels"`
	Date        string   `json:"date,omitempty"` // of the photos at the centre, when the source says
}

// Level is one zoom level of a plan.
type Level struct {
	Zoom    int     `json:"zoom"`
	RadiusM float64 `json:"radiusM"`
	Tiles   int     `json:"tiles"`
	Metres  float64 `json:"metresPerPixel"`
}

// Typical JPEG photo tile, from VGIN and USGS samples around Roanoke.
const avgTileBytes = 15000

// MakePlan works out the tiers. Zoom levels below OuterZoom cover the whole area too; they are few.
func MakePlan(p Plan) (Plan, error) {
	if p.RadiusM < 500 || p.RadiusM > 400e3 {
		return p, errors.New("the radius must be between 500 m and 400 km")
	}
	if p.CoreRadiusM < 0 || p.CoreRadiusM > p.RadiusM {
		return p, errors.New("the full-detail radius must be within the area")
	}
	if p.OuterZoom < 10 || p.OuterZoom > 19 || p.CoreZoom < p.OuterZoom || p.CoreZoom > 20 {
		return p, errors.New("zoom levels: outer 10 to 19, full detail from the outer level to 20")
	}
	if p.Rate <= 0 {
		p.Rate = 20
	}
	p.Rate = math.Min(p.Rate, 50)
	p.Sources = nil
	for _, s := range Catalog {
		if s.covers(p.Lat, p.Lon) {
			p.Sources = append(p.Sources, s.ID)
		}
	}
	if len(p.Sources) == 0 {
		return p, errors.New("no photo source covers this centre")
	}
	p.Levels, p.Tiles = nil, 0
	for z := 8; z <= p.CoreZoom; z++ {
		r := p.RadiusM
		if z > p.OuterZoom {
			r = p.CoreRadiusM
		}
		if r <= 0 {
			continue
		}
		n := countCircle(p.Lat, r, z)
		mpp := 40075016.7 * math.Cos(p.Lat*math.Pi/180) / math.Exp2(float64(z)) / 256
		p.Levels = append(p.Levels, Level{Zoom: z, RadiusM: r, Tiles: n, Metres: mpp})
		p.Tiles += n
	}
	p.Bytes = int64(p.Tiles) * avgTileBytes
	p.Seconds = float64(p.Tiles) / p.Rate
	return p, nil
}

// Store keeps photo tiles. Unlike the map, photos are recorded into the store in use, tile by tile: a
// recording can take hours, resumes where it stopped, and every tile already fetched is useful at once.
type Store struct {
	db *sql.DB
}

// OpenStore opens or creates the photo store.
func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(10000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS tiles (z INTEGER, x INTEGER, y INTEGER, src TEXT, at INTEGER, data BLOB, PRIMARY KEY (z, x, y)) WITHOUT ROWID`,
		`CREATE TABLE IF NOT EXISTS meta (k TEXT PRIMARY KEY, v TEXT)`,
	} {
		if _, err := db.Exec(q); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Store{db: db}, nil
}

// Close closes the store.
func (s *Store) Close() error { return s.db.Close() }

// get returns a tile; found reports a row (an empty blob means the sources had no photo there).
func (s *Store) get(ctx context.Context, z, x, y int) (data []byte, found bool, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT data FROM tiles WHERE z=? AND x=? AND y=?`, z, x, y).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return data, err == nil, err
}

func (s *Store) has(ctx context.Context, z, x, y int) bool {
	var one int
	return s.db.QueryRowContext(ctx, `SELECT 1 FROM tiles WHERE z=? AND x=? AND y=?`, z, x, y).Scan(&one) == nil
}

func (s *Store) put(ctx context.Context, z, x, y int, src string, data []byte) error {
	if data == nil {
		data = []byte{}
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO tiles (z, x, y, src, at, data) VALUES (?, ?, ?, ?, ?, ?)`, z, x, y, src, time.Now().Unix(), data)
	return err
}

// Meta reads a stored value.
func (s *Store) Meta(k string) string {
	var v string
	s.db.QueryRow(`SELECT v FROM meta WHERE k=?`, k).Scan(&v)
	return v
}

func (s *Store) setMeta(k, v string) {
	s.db.Exec(`INSERT OR REPLACE INTO meta (k, v) VALUES (?, ?)`, k, v)
}

// Stats counts what is recorded.
type Stats struct {
	Tiles   int            `json:"tiles"`
	Empty   int            `json:"empty"`
	Bytes   int64          `json:"bytes"`
	ByZoom  map[int]int    `json:"byZoom"`
	BySrc   map[string]int `json:"bySource"`
	MaxZoom int            `json:"maxZoom"`
}

// Stats reads the store's totals.
func (s *Store) Stats(ctx context.Context) (Stats, error) {
	st := Stats{ByZoom: map[int]int{}, BySrc: map[string]int{}}
	rows, err := s.db.QueryContext(ctx, `SELECT z, src, count(*), sum(length(data)), sum(length(data) = 0) FROM tiles GROUP BY z, src`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var z, n, empty int
		var src string
		var b sql.NullInt64
		if err := rows.Scan(&z, &src, &n, &b, &empty); err != nil {
			return st, err
		}
		st.Tiles += n - empty
		st.Empty += empty
		st.Bytes += b.Int64
		st.ByZoom[z] += n - empty
		if src != "" {
			st.BySrc[src] += n - empty
		}
		if n > empty && z > st.MaxZoom {
			st.MaxZoom = z
		}
	}
	return st, rows.Err()
}

// Job is a recording.
type Job struct {
	Plan     Plan       `json:"plan"`
	By       string     `json:"by"`
	Started  time.Time  `json:"started"`
	Finished *time.Time `json:"finished,omitempty"`
	Done     int        `json:"done"`    // tiles handled, including those already recorded
	Fetched  int        `json:"fetched"` // tiles downloaded this time
	Missing  int        `json:"missing"` // tiles no source had
	Failed   int        `json:"failed"`  // tiles that could not be fetched (tried again next time)
	Bytes    int64      `json:"bytes"`
	Zoom     int        `json:"zoom"` // the level being recorded
	Error    string     `json:"error,omitempty"`
	cancel   context.CancelFunc
}

// Service owns the photo store and its recordings.
type Service struct {
	Dir    string
	Agent  string
	Client *http.Client
	// Recorded and Failed report a finished recording, for the ledger.
	Recorded func(Job)
	Failed   func(Job)
	// Online lets the server fetch a photo it does not have when a console asks for it (and keep it).
	Online func() bool

	store *Store
	mu    sync.Mutex
	job   *Job
	dates sync.Map // cached photo dates by rounded position
	last  atomic.Pointer[Plan]
	// base overrides Catalog URLs, for tests.
	base map[string]string
}

// NewService opens the photo store in dir.
func NewService(dir, agent string) (*Service, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	st, err := OpenStore(filepath.Join(dir, "photos.db"))
	if err != nil {
		return nil, err
	}
	return &Service{Dir: dir, Agent: agent, Client: &http.Client{Timeout: 20 * time.Second}, store: st}, nil
}

// Close closes the store.
func (s *Service) Close() error {
	s.Cancel()
	return s.store.Close()
}

// Store is the photo store.
func (s *Service) Store() *Store { return s.store }

// Job is the running or last recording.
func (s *Service) Job() *Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil {
		return nil
	}
	j := *s.job
	return &j
}

// Last is the plan of the last finished recording, if any.
func (s *Service) Last() *Plan {
	if p := s.last.Load(); p != nil {
		return p
	}
	v := s.store.Meta("plan")
	if v == "" {
		return nil
	}
	var p Plan
	if json.Unmarshal([]byte(v), &p) != nil {
		return nil
	}
	s.last.Store(&p)
	return &p
}

// Record starts a recording in the background.
func (s *Service) Record(p Plan, by string) error {
	p, err := MakePlan(p)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job != nil && s.job.Finished == nil {
		return errors.New("photos are already being recorded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.job = &Job{Plan: p, By: by, Started: time.Now().UTC(), cancel: cancel}
	go s.run(ctx, s.job)
	return nil
}

// Cancel stops the recording; what was fetched stays.
func (s *Service) Cancel() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil || s.job.Finished != nil {
		return false
	}
	s.job.cancel()
	return true
}

func (s *Service) run(ctx context.Context, j *Job) {
	err := s.record(ctx, j)
	now := time.Now().UTC()
	s.mu.Lock()
	j.Finished = &now
	if err != nil {
		j.Error = err.Error()
		if errors.Is(err, context.Canceled) {
			j.Error = "cancelled"
		}
	}
	jj := *j
	s.mu.Unlock()
	if err == nil {
		b, _ := json.Marshal(j.Plan)
		s.store.setMeta("plan", string(b))
		s.store.setMeta("recorded", now.Format(time.RFC3339))
		p := j.Plan
		s.last.Store(&p)
		if s.Recorded != nil {
			s.Recorded(jj)
		}
	} else if s.Failed != nil {
		s.Failed(jj)
	}
}

func (s *Service) record(ctx context.Context, j *Job) error {
	p := j.Plan
	if d := s.DateAt(ctx, p.Lat, p.Lon); d != "" {
		s.mu.Lock()
		j.Plan.Date = d
		s.mu.Unlock()
	}
	interval := time.Duration(float64(time.Second) / p.Rate)
	tick := time.NewTicker(interval)
	defer tick.Stop()
	type item struct{ z, x, y int }
	work := make(chan item)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range work {
				data, src, err := s.fetch(ctx, it.z, it.x, it.y)
				s.mu.Lock()
				switch {
				case err != nil:
					j.Failed++
				case data == nil:
					j.Missing++
				default:
					j.Fetched++
					j.Bytes += int64(len(data))
				}
				j.Done++
				s.mu.Unlock()
				if err == nil {
					s.store.put(ctx, it.z, it.x, it.y, src, data)
				}
			}
		}()
	}
	var err error
levels:
	for _, l := range p.Levels {
		s.mu.Lock()
		j.Zoom = l.Zoom
		s.mu.Unlock()
		for _, t := range Circle(p.Lat, p.Lon, l.RadiusM, l.Zoom) {
			if s.store.has(ctx, l.Zoom, t[0], t[1]) {
				s.mu.Lock()
				j.Done++
				s.mu.Unlock()
				continue
			}
			select {
			case <-ctx.Done():
				err = ctx.Err()
				break levels
			case <-tick.C:
			}
			select {
			case <-ctx.Done():
				err = ctx.Err()
				break levels
			case work <- item{l.Zoom, t[0], t[1]}:
			}
		}
	}
	close(work)
	wg.Wait()
	if err != nil {
		return err
	}
	s.mu.Lock()
	failed := j.Failed
	s.mu.Unlock()
	if failed > 0 && failed*20 > j.Plan.Tiles {
		return fmt.Errorf("%d tiles could not be fetched; record again to fetch them", failed)
	}
	return nil
}

func (s *Service) urlFor(src Source, z, x, y int) string {
	u := src.URL
	if b, ok := s.base[src.ID]; ok {
		u = b
	}
	return strings.NewReplacer("{z}", strconv.Itoa(z), "{x}", strconv.Itoa(x), "{y}", strconv.Itoa(y)).Replace(u)
}

// fetch takes a tile from the first source that covers it and has it. A nil result with no error means no
// source has a photo there.
func (s *Service) fetch(ctx context.Context, z, x, y int) ([]byte, string, error) {
	lat, lon := (tileLat(y, z)+tileLat(y+1, z))/2, (tileLon(x, z)+tileLon(x+1, z))/2
	var lastErr error
	for _, src := range Catalog {
		if z > src.MaxZoom || !src.covers(lat, lon) {
			continue
		}
		b, err := s.get(ctx, s.urlFor(src, z, x, y))
		if err != nil {
			lastErr = err
			continue
		}
		if b != nil {
			return b, src.ID, nil
		}
	}
	return nil, "", lastErr
}

var errNotFound = errors.New("not found")

// get returns nil, nil for a tile the service does not have.
func (s *Service) get(ctx context.Context, u string) ([]byte, error) { return s.getAs(ctx, u, true) }

func (s *Service) getAs(ctx context.Context, u string, image bool) ([]byte, error) {
	var err error
	for attempt := range 3 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}
		var b []byte
		b, err = s.get1(ctx, u, image)
		if err == nil || errors.Is(err, errNotFound) {
			if err != nil {
				return nil, nil
			}
			return b, nil
		}
	}
	return nil, err
}

func (s *Service) get1(ctx context.Context, u string, wantImage bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.Agent)
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 || resp.StatusCode == 400 {
		return nil, errNotFound
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if !wantImage {
		return b, nil
	}
	ct := http.DetectContentType(b)
	if !strings.HasPrefix(ct, "image/") {
		return nil, errNotFound
	}
	if blank(b) {
		return nil, errNotFound
	}
	return b, nil
}

// blank spots the single-colour tiles some services send for "no photo here".
func blank(b []byte) bool {
	if len(b) > 4000 {
		return false
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return true
	}
	r := img.Bounds()
	c0 := color.RGBAModel.Convert(img.At(r.Min.X, r.Min.Y)).(color.RGBA)
	for y := r.Min.Y; y < r.Max.Y; y += 16 {
		for x := r.Min.X; x < r.Max.X; x += 16 {
			c := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			if absDiff(c.R, c0.R) > 6 || absDiff(c.G, c0.G) > 6 || absDiff(c.B, c0.B) > 6 {
				return false
			}
		}
	}
	return true
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

// ErrNoTile means there is no photo for the tile.
var ErrNoTile = errors.New("no photo")

// Tile returns a photo tile for the console: recorded, or made from a recorded tile of a lower level
// (enlarged, so the photo stays on screen when zoomed in past what was recorded), or fetched now and kept
// when Online allows.
func (s *Service) Tile(ctx context.Context, z, x, y int) ([]byte, error) {
	if z < 0 || z > 22 || x < 0 || y < 0 || x >= 1<<z || y >= 1<<z {
		return nil, ErrNoTile
	}
	if b, found, err := s.store.get(ctx, z, x, y); err != nil {
		return nil, err
	} else if found && len(b) > 0 {
		return b, nil
	} else if !found && s.Online != nil && s.Online() && z <= 19 {
		fctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		b, src, err := s.fetch(fctx, z, x, y)
		cancel()
		if err == nil {
			s.store.put(ctx, z, x, y, src, b)
			if b != nil {
				return b, nil
			}
		}
	}
	// Enlarge the nearest recorded ancestor.
	for up := 1; up <= 6 && z-up >= 0; up++ {
		az, ax, ay := z-up, x>>up, y>>up
		b, found, err := s.store.get(ctx, az, ax, ay)
		if err != nil {
			return nil, err
		}
		if found && len(b) > 0 {
			return enlarge(b, up, x-(ax<<up), y-(ay<<up))
		}
	}
	return nil, ErrNoTile
}

// enlarge cuts the part of a tile that a descendant `levels` deeper covers and scales it to a full tile.
func enlarge(b []byte, levels, dx, dy int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	r := src.Bounds()
	n := 1 << levels
	cw, ch := float64(r.Dx())/float64(n), float64(r.Dy())/float64(n)
	ox, oy := float64(r.Min.X)+float64(dx)*cw, float64(r.Min.Y)+float64(dy)*ch
	const size = 256
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	rgba := image.NewRGBA(r)
	draw.Draw(rgba, r, src, r.Min, draw.Src)
	at := func(x, y int) color.RGBA {
		x = clampInt(x, r.Min.X, r.Max.X-1)
		y = clampInt(y, r.Min.Y, r.Max.Y-1)
		return rgba.RGBAAt(x, y)
	}
	for py := range size {
		fy := oy + (float64(py)+0.5)*ch/size - 0.5
		y0 := int(math.Floor(fy))
		ty := fy - float64(y0)
		for px := range size {
			fx := ox + (float64(px)+0.5)*cw/size - 0.5
			x0 := int(math.Floor(fx))
			tx := fx - float64(x0)
			c00, c10, c01, c11 := at(x0, y0), at(x0+1, y0), at(x0, y0+1), at(x0+1, y0+1)
			mix := func(a, b, c, d uint8) uint8 {
				top := float64(a)*(1-tx) + float64(b)*tx
				bot := float64(c)*(1-tx) + float64(d)*tx
				return uint8(top*(1-ty) + bot*ty + 0.5)
			}
			dst.SetRGBA(px, py, color.RGBA{mix(c00.R, c10.R, c01.R, c11.R), mix(c00.G, c10.G, c01.G, c11.G), mix(c00.B, c10.B, c01.B, c11.B), 255})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// DateAt says when the photo at a point was taken, for sources that publish it ("2023, 12 inch ortho").
func (s *Service) DateAt(ctx context.Context, lat, lon float64) string {
	key := fmt.Sprintf("%.3f,%.3f", lat, lon)
	if v, ok := s.dates.Load(key); ok {
		return v.(string)
	}
	var out string
	for _, src := range Catalog {
		if !src.covers(lat, lon) {
			continue
		}
		if src.DateIndex == "" {
			continue
		}
		out = s.arcgisDate(ctx, src.DateIndex, lat, lon)
		if out != "" {
			break
		}
	}
	if out != "" {
		s.dates.Store(key, out)
	}
	return out
}

func (s *Service) arcgisDate(ctx context.Context, base string, lat, lon float64) string {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	b, err := s.getAs(ctx, base+"?f=json", false)
	if err != nil || b == nil {
		return ""
	}
	var info struct {
		Layers []struct {
			ID int `json:"id"`
		} `json:"layers"`
	}
	if json.Unmarshal(b, &info) != nil {
		return ""
	}
	best, bestYear := "", ""
	for _, l := range info.Layers {
		q := url.Values{"geometry": {fmt.Sprintf("%f,%f", lon, lat)}, "geometryType": {"esriGeometryPoint"}, "inSR": {"4326"},
			"spatialRel": {"esriSpatialRelIntersects"}, "outFields": {"Year,Product"}, "returnGeometry": {"false"}, "f": {"json"}}
		b, err := s.getAs(ctx, fmt.Sprintf("%s/%d/query?%s", base, l.ID, q.Encode()), false)
		if err != nil || b == nil {
			continue
		}
		var r struct {
			Features []struct {
				Attributes struct {
					Year    string `json:"Year"`
					Product string `json:"Product"`
				} `json:"attributes"`
			} `json:"features"`
		}
		if json.Unmarshal(b, &r) != nil {
			continue
		}
		for _, f := range r.Features {
			// The service shows the newest photo where years overlap.
			if a := f.Attributes; a.Year > bestYear {
				bestYear, best = a.Year, a.Year
				if a.Product != "" {
					best += ", " + a.Product
				}
			}
		}
	}
	return best
}
