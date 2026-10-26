package mapdata

import (
	"bytes"
	"context"
	"crypto/fips140"
	"crypto/md5"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/osm"
)

//go:embed fonts
var fontFS embed.FS

// Glyphs returns a MapLibre glyph range for a font stack, or nil. The first font of a stack is used.
func Glyphs(stack, rng string) []byte {
	if i := strings.Index(stack, ","); i >= 0 {
		stack = stack[:i]
	}
	stack = strings.TrimSpace(stack)
	if strings.ContainsAny(stack, `/\.`) || strings.ContainsAny(rng, `/\`) {
		return nil
	}
	b, err := fs.ReadFile(fontFS, "fonts/"+stack+"/"+strings.TrimSuffix(rng, ".pbf")+".pbf")
	if err != nil {
		return nil
	}
	return b
}

// DefaultIndex is Geofabrik's list of extracts, with their outlines.
const DefaultIndex = "https://download.geofabrik.de/index-v1.json"

// Extract is one downloadable OpenStreetMap extract.
type Extract struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Parent string `json:"parent,omitempty"`
	URL    string `json:"url"`
	Bytes  int64  `json:"bytes,omitempty"`
}

type indexFile struct {
	Features []struct {
		Properties struct {
			ID     string            `json:"id"`
			Parent string            `json:"parent"`
			Name   string            `json:"name"`
			URLs   map[string]string `json:"urls"`
		} `json:"properties"`
		Geometry struct {
			Type        string          `json:"type"`
			Coordinates json.RawMessage `json:"coordinates"`
		} `json:"geometry"`
	} `json:"features"`
}

// Choose picks the smallest extracts that together cover the area: those with no sub-extracts whose
// outline comes within the circle.
func Choose(index []byte, a Area) ([]Extract, error) {
	var f indexFile
	if err := json.Unmarshal(index, &f); err != nil {
		return nil, fmt.Errorf("mapdata: extract index: %w", err)
	}
	parents := map[string]bool{}
	for _, x := range f.Features {
		if x.Properties.Parent != "" {
			parents[x.Properties.Parent] = true
		}
	}
	var out []Extract
	var boxes []BBox
	for _, x := range f.Features {
		p := x.Properties
		if parents[p.ID] || p.URLs["pbf"] == "" {
			continue
		}
		var polys [][][][2]float64
		switch x.Geometry.Type {
		case "MultiPolygon":
			json.Unmarshal(x.Geometry.Coordinates, &polys)
		case "Polygon":
			var one [][][2]float64
			json.Unmarshal(x.Geometry.Coordinates, &one)
			polys = append(polys, one)
		}
		if touches(polys, a) {
			out = append(out, Extract{ID: p.ID, Name: p.Name, Parent: p.Parent, URL: p.URLs["pbf"]})
			b := emptyBox()
			for _, poly := range polys {
				if len(poly) > 0 {
					for _, c := range poly[0] {
						b.add(PtDeg(c[1], c[0]))
					}
				}
			}
			boxes = append(boxes, b)
		}
	}
	// Geofabrik also publishes groupings (a whole country, "US South") beside their parts. A grouping
	// that contains another chosen extract is redundant: every part of it the circle touches is itself
	// chosen.
	keep := out[:0:0]
	for i := range out {
		redundant := false
		for j := range out {
			if i != j && inside(boxes[j], boxes[i]) && !inside(boxes[i], boxes[j]) {
				redundant = true
				break
			}
		}
		if !redundant {
			keep = append(keep, out[i])
		}
	}
	out = keep
	// Some regions are published twice (a country and the same country split up); keep the one in the
	// deepest part of the tree, which the leaf rule already did, and sort for a stable display.
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) == 0 {
		return nil, errors.New("mapdata: no extract covers that area")
	}
	return out, nil
}

// inside reports whether box a lies within box b.
func inside(a, b BBox) bool {
	return a.MinLon >= b.MinLon && a.MaxLon <= b.MaxLon && a.MinLat >= b.MinLat && a.MaxLat <= b.MaxLat
}

// touches reports whether any polygon contains the centre or has an edge within the radius.
func touches(polys [][][][2]float64, a Area) bool {
	kx := metresPerDeg * math.Cos(a.Lat*math.Pi/180)
	for _, poly := range polys {
		if len(poly) == 0 {
			continue
		}
		outer := poly[0]
		ring := make([]Pt, len(outer))
		for i, c := range outer {
			ring[i] = PtDeg(c[1], c[0])
		}
		if inRing(PtDeg(a.Lat, a.Lon), ring) {
			return true
		}
		for i := 1; i < len(outer); i++ {
			ax, ay := (outer[i-1][0]-a.Lon)*kx, (outer[i-1][1]-a.Lat)*metresPerDeg
			bx, by := (outer[i][0]-a.Lon)*kx, (outer[i][1]-a.Lat)*metresPerDeg
			if math.Sqrt(segDist2(fpt{0, 0}, fpt{ax, ay}, fpt{bx, by})) <= a.RadiusM {
				return true
			}
		}
	}
	return false
}

// Job is a recording in progress or finished.
type Job struct {
	Area     Area       `json:"area"`
	Extracts []Extract  `json:"extracts"`
	Started  time.Time  `json:"started"`
	Finished *time.Time `json:"finished,omitempty"`
	Progress Progress   `json:"progress"`
	Error    string     `json:"error,omitempty"`
	Stats    *Stats     `json:"stats,omitempty"`
	By       string     `json:"by"`
	Update   bool       `json:"update,omitempty"`  // brought up to date from change files rather than recorded afresh
	Changes  int        `json:"changes,omitempty"` // elements the change files touched
	cancel   context.CancelFunc
	prev     []string // extracts updated in place, their old copies kept as .prev until the map is built
}

// unreachable says plainly that OpenStreetMap could not be reached, which is most often no internet.
func unreachable(err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}
	return fmt.Errorf("could not reach OpenStreetMap's change files (is the internet down?): %w; the map in use is unchanged", err)
}

// settle keeps or puts back the extracts an update changed, once the update has finished.
func (j *Job) settle(ok bool) {
	for _, f := range j.prev {
		if ok {
			os.Remove(f + ".prev")
		} else {
			os.Rename(f+".prev", f)
		}
	}
	j.prev = nil
}

// Service owns the data directory's map: the store being served and any recording under way.
type Service struct {
	Dir      string
	IndexURL string
	Client   *http.Client
	Agent    string
	Log      *slog.Logger
	// Recorded is called after a recording replaces the store; Failed when one does not finish; UpToDate
	// when an update found nothing to apply.
	Recorded func(j Job)
	Failed   func(j Job)
	UpToDate func(j Job)
	// MaxDrop refuses an update whose new map has this fraction fewer road segments or addresses than the
	// map in use (0.1: ten percent); the map in use stays. 0 checks nothing.
	MaxDrop float64

	mu      sync.RWMutex
	cur     *Store
	curPath string
	job     *Job
}

// NewService opens the store in dir if there is one.
func NewService(dir, agent string, log *slog.Logger) *Service {
	s := &Service{Dir: dir, IndexURL: DefaultIndex, Client: &http.Client{Timeout: 0}, Agent: agent, Log: log}
	// The newest store that opens is the one in use; older ones are left from before a recording.
	stores, _ := filepath.Glob(filepath.Join(dir, "map-*.db"))
	sort.Sort(sort.Reverse(sort.StringSlice(stores)))
	for _, p := range stores {
		st, err := Open(p)
		if err != nil {
			log.Warn("map: a recorded map could not be opened", "file", p, "err", err)
			continue
		}
		s.cur, s.curPath = st, p
		s.checkTileVersion(st)
		break
	}
	for _, p := range stores {
		if p != s.curPath {
			removeStore(p)
		}
	}
	return s
}

// removeStore deletes a store file and SQLite's files beside it.
func removeStore(p string) {
	for _, x := range []string{p, p + "-wal", p + "-shm", p + "-journal"} {
		os.Remove(x)
	}
}

// TileVersion changes when tiles are built differently, so tiles made by an older build are remade.
const TileVersion = "7"

func (s *Service) checkTileVersion(st *Store) {
	var v string
	st.db.QueryRow(`SELECT v FROM meta WHERE k = 'tiles'`).Scan(&v)
	if v != TileVersion {
		st.ClearTiles(context.Background())
		st.db.Exec(`INSERT OR REPLACE INTO meta(k, v) VALUES('tiles', ?)`, TileVersion)
	}
}

// newStorePath is a new file for each recording: a store in use is never replaced underneath SQLite,
// whose write-ahead log beside it belongs to that file alone.
func (s *Service) newStorePath() string {
	return filepath.Join(s.Dir, "map-"+time.Now().UTC().Format("20060102T150405")+".db")
}

func (s *Service) srcDir() string { return filepath.Join(s.Dir, "osm") }

// SetMaxDrop sets how much an update may lose before it is refused.
func (s *Service) SetMaxDrop(f float64) {
	s.mu.Lock()
	s.MaxDrop = f
	s.mu.Unlock()
}

// Store is the map being served, or nil.
func (s *Service) Store() *Store {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// Job is the current or last recording.
func (s *Service) Job() *Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.job == nil {
		return nil
	}
	j := *s.job
	return &j
}

// Index fetches the extract list, keeping a copy for a week.
func (s *Service) Index(ctx context.Context) ([]byte, error) {
	p := filepath.Join(s.Dir, "osm-index.json")
	if st, err := os.Stat(p); err == nil && time.Since(st.ModTime()) < 7*24*time.Hour {
		return os.ReadFile(p)
	}
	b, err := s.get(ctx, s.IndexURL, 64<<20, nil)
	if err != nil {
		if old, rerr := os.ReadFile(p); rerr == nil {
			return old, nil
		}
		return nil, err
	}
	os.MkdirAll(s.Dir, 0o700)
	os.WriteFile(p, b, 0o600)
	return b, nil
}

// Plan lists the extracts a recording of the area needs, with their sizes.
func (s *Service) Plan(ctx context.Context, a Area) ([]Extract, error) {
	idx, err := s.Index(ctx)
	if err != nil {
		return nil, err
	}
	ex, err := Choose(idx, a)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	for i := range ex {
		wg.Add(1)
		go func(e *Extract) {
			defer wg.Done()
			req, err := http.NewRequestWithContext(ctx, http.MethodHead, e.URL, nil)
			if err != nil {
				return
			}
			req.Header.Set("User-Agent", s.Agent)
			c := &http.Client{Timeout: 15 * time.Second}
			if resp, err := c.Do(req); err == nil {
				resp.Body.Close()
				e.Bytes = resp.ContentLength
			}
		}(&ex[i])
	}
	wg.Wait()
	return ex, nil
}

func (s *Service) get(ctx context.Context, url string, limit int64, w io.Writer) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.Agent)
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	if w != nil {
		_, err := io.Copy(w, resp.Body)
		return nil, err
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// Record starts recording an area in the background. Only one recording runs at a time; the map being
// served stays in use until the new one is complete.
func (s *Service) Record(a Area, ex []Extract, by string) error {
	if a.RadiusM < 1000 || a.RadiusM > 400e3 {
		return errors.New("the radius must be between 1 km and 400 km (about 250 miles)")
	}
	if a.Lat < -85 || a.Lat > 85 || a.Lon < -180 || a.Lon > 180 {
		return errors.New("the centre is not a valid position")
	}
	if len(ex) == 0 {
		return errors.New("no extracts chosen")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job != nil && s.job.Finished == nil {
		return errors.New("a recording is already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &Job{Area: a, Extracts: ex, Started: time.Now().UTC(), By: by, cancel: cancel, Progress: Progress{Phase: "Starting"}}
	s.job = j
	go s.run(ctx, j)
	return nil
}

// Update brings the recorded map up to date from the extracts' daily change files (Geofabrik publishes
// one a day per extract): a few megabytes to download instead of the whole extracts, then the store is
// built again from the updated extracts as a recording is.
func (s *Service) Update(by string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == nil {
		return errors.New("record the map first")
	}
	if s.job != nil && s.job.Finished == nil {
		return errors.New("a recording is already running")
	}
	var ex []Extract
	for _, f := range s.cur.Files {
		ex = append(ex, Extract{ID: strings.TrimSuffix(filepath.Base(f), ".osm.pbf"), Name: filepath.Base(f), URL: f})
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &Job{Area: s.cur.Area, Extracts: ex, Started: time.Now().UTC(), By: by, cancel: cancel, Update: true, Progress: Progress{Phase: "Checking for changes"}}
	s.job = j
	go s.run(ctx, j)
	return nil
}

// ErrUpToDate means there were no change files to apply.
var ErrUpToDate = errors.New("the map is already up to date with OpenStreetMap")

// replication reads a state.txt: the newest sequence number and its time.
func parseState(b []byte) (int64, time.Time, error) {
	var seq int64 = -1
	var ts time.Time
	for _, l := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(l), "=")
		if !ok {
			continue
		}
		switch k {
		case "sequenceNumber":
			seq, _ = strconv.ParseInt(v, 10, 64)
		case "timestamp":
			ts, _ = time.Parse(time.RFC3339, strings.ReplaceAll(v, "\\", ""))
		}
	}
	if seq < 0 {
		return 0, ts, errors.New("no sequence number in the replication state")
	}
	return seq, ts, nil
}

// applyChanges downloads and applies each extract's change files since the extract was made.
func (s *Service) applyChanges(ctx context.Context, j *Job) ([]string, error) {
	var files []string
	changed := false
	for i, e := range j.Extracts {
		f := e.URL
		files = append(files, f)
		h, err := osm.ReadHeader(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name, err)
		}
		base := strings.TrimSuffix(h.BaseURL, "/")
		if !strings.HasPrefix(base, "https://download.geofabrik.de/") || h.Sequence <= 0 {
			return nil, fmt.Errorf("%s has no Geofabrik change files to follow: record the area again", e.Name)
		}
		s.setProgress(j, Progress{Phase: "Checking for changes", Detail: e.Name, Fraction: float64(i) / float64(len(j.Extracts))})
		st, err := s.get(ctx, base+"/state.txt", 4096, nil)
		if err != nil {
			return nil, unreachable(err)
		}
		seq, ts, err := parseState(st)
		if err != nil {
			return nil, err
		}
		if seq <= h.Sequence {
			continue
		}
		if seq-h.Sequence > 400 {
			return nil, fmt.Errorf("%s is %d days behind: record the area again instead", e.Name, seq-h.Sequence)
		}
		c := osm.NewChange()
		for q := h.Sequence + 1; q <= seq; q++ {
			s.setProgress(j, Progress{Phase: "Downloading changes", Detail: fmt.Sprintf("%s: %d of %d", e.Name, q-h.Sequence, seq-h.Sequence),
				Fraction: float64(q-h.Sequence) / float64(seq-h.Sequence)})
			u := fmt.Sprintf("%s/%03d/%03d/%03d.osc.gz", base, q/1000000, q/1000%1000, q%1000)
			b, err := s.get(ctx, u, 512<<20, nil)
			if err != nil {
				return nil, unreachable(err)
			}
			if err := c.ReadChange(bytes.NewReader(b), true); err != nil {
				return nil, fmt.Errorf("%s: %w", u, err)
			}
		}
		s.setProgress(j, Progress{Phase: "Applying changes", Detail: fmt.Sprintf("%s: %d elements", e.Name, c.Size())})
		// The updated extract is written beside the old one, which is kept as .prev until the new map is
		// built and checked: a failure puts it back, so the next update starts from the same place.
		tmp := f + ".new"
		if err := osm.Apply(f, tmp, c, osm.Header{Sequence: seq, Timestamp: ts.Unix(), BaseURL: base}, runtime.NumCPU()); err != nil {
			os.Remove(tmp)
			return nil, err
		}
		if err := os.Rename(f, f+".prev"); err != nil {
			os.Remove(tmp)
			return nil, err
		}
		if err := os.Rename(tmp, f); err != nil {
			os.Rename(f+".prev", f)
			return nil, err
		}
		j.prev = append(j.prev, f)
		s.mu.Lock()
		j.Changes += c.Size()
		s.mu.Unlock()
		changed = true
	}
	if !changed {
		return nil, ErrUpToDate
	}
	return files, nil
}

// Cancel stops the running recording.
func (s *Service) Cancel() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil || s.job.Finished != nil {
		return false
	}
	s.job.cancel()
	return true
}

func (s *Service) setProgress(j *Job, p Progress) {
	s.mu.Lock()
	j.Progress = p
	s.mu.Unlock()
}

func (s *Service) run(ctx context.Context, j *Job) {
	err := s.record(ctx, j)
	j.settle(err == nil)
	now := time.Now().UTC()
	s.mu.Lock()
	j.Finished = &now
	if errors.Is(err, ErrUpToDate) {
		j.Progress = Progress{Phase: "Already up to date", Fraction: 1}
		s.mu.Unlock()
		if s.UpToDate != nil {
			s.UpToDate(*j)
		}
		return
	}
	if err != nil {
		j.Error = err.Error()
		if errors.Is(err, context.Canceled) {
			j.Error = "cancelled"
		}
	}
	s.mu.Unlock()
	if err != nil {
		s.Log.Warn("map: recording failed", "err", err)
		if s.Failed != nil {
			s.Failed(*j)
		}
		return
	}
	s.Log.Info("map: recording finished", "area", j.Area.Name, "features", j.Stats.Features, "seconds", j.Stats.Seconds)
	if s.Recorded != nil {
		s.Recorded(*j)
	}
}

func (s *Service) record(ctx context.Context, j *Job) error {
	if err := os.MkdirAll(s.srcDir(), 0o700); err != nil {
		return err
	}
	var files []string
	if j.Update {
		var err error
		if files, err = s.applyChanges(ctx, j); err != nil {
			return err
		}
	} else {
		for i, e := range j.Extracts {
			p := filepath.Join(s.srcDir(), safeName(e.ID)+".osm.pbf")
			if err := s.download(ctx, j, e, p, i); err != nil {
				return fmt.Errorf("%s: %w", e.Name, err)
			}
			files = append(files, p)
		}
	}
	var src []string
	for _, e := range j.Extracts {
		src = append(src, e.URL)
	}
	path := s.newStorePath()
	st, err := Build(ctx, path, j.Area, files, "OpenStreetMap via "+strings.Join(src, ", "), func(p Progress) { s.setProgress(j, p) })
	if err != nil {
		return err
	}
	s.mu.RLock()
	cur, drop := s.cur, s.MaxDrop
	s.mu.RUnlock()
	if err := checkDrop(j, cur, st, drop); err != nil {
		removeStore(path)
		return err
	}
	ns, err := Open(path)
	if err != nil {
		removeStore(path)
		return err
	}
	s.checkTileVersion(ns)
	s.mu.Lock()
	old, oldPath := s.cur, s.curPath
	s.cur, s.curPath = ns, path
	j.Stats = &st
	j.Progress = Progress{Phase: "Done", Fraction: 1}
	s.mu.Unlock()
	if old != nil {
		// Requests already holding the old store finish first.
		go func() {
			time.Sleep(30 * time.Second)
			old.Close()
			removeStore(oldPath)
		}()
	}
	return nil
}

// checkDrop refuses an update that lost too much: a damaged or truncated change file shows as roads or
// addresses gone, and a dispatch map should not lose them overnight without anyone deciding so.
func checkDrop(j *Job, cur *Store, st Stats, maxDrop float64) error {
	if !j.Update || cur == nil || maxDrop <= 0 {
		return nil
	}
	for _, c := range []struct {
		what     string
		old, new int64
	}{{"road segments", cur.Stats.RoadSegments, st.RoadSegments}, {"addresses", cur.Stats.Addresses, st.Addresses}} {
		if c.old > 0 && float64(c.new) < float64(c.old)*(1-maxDrop) {
			return fmt.Errorf("the updated map has %d %s where the map in use has %d (more than %.0f%% fewer): kept the map in use",
				c.new, c.what, c.old, maxDrop*100)
		}
	}
	return nil
}

func safeName(id string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, id)
}

// download fetches an extract unless the copy on disk already matches the published checksum.
func (s *Service) download(ctx context.Context, j *Job, e Extract, path string, i int) error {
	phase := fmt.Sprintf("Downloading %s (%d of %d)", e.Name, i+1, len(j.Extracts))
	s.setProgress(j, Progress{Phase: phase, Detail: "checking the copy on disk"})
	want := ""
	if b, err := s.get(ctx, e.URL+".md5", 4096, nil); err == nil {
		if f := strings.Fields(string(b)); len(f) > 0 && len(f[0]) == 32 {
			want = strings.ToLower(f[0])
		}
	}
	if want != "" {
		if got, err := md5File(path); err == nil && got == want {
			return nil
		}
	}
	tmp := path + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	pw := &progressWriter{w: f, total: e.Bytes, report: func(n, total int64) {
		frac := 0.0
		if total > 0 {
			frac = float64(n) / float64(total)
		}
		s.setProgress(j, Progress{Phase: phase, Detail: fmt.Sprintf("%.0f of %.0f MB", float64(n)/1e6, float64(total)/1e6), Fraction: frac})
	}}
	_, err = s.get(ctx, e.URL, 0, pw)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if want != "" {
		got, err := md5File(tmp)
		if err != nil {
			return err
		}
		if got != want {
			os.Remove(tmp)
			return fmt.Errorf("the download does not match its published checksum")
		}
	}
	return os.Rename(tmp, path)
}

// md5File is the MD5 the extract's publisher lists beside it. It only catches a download cut short or
// corrupted (the download itself is over HTTPS); it protects nothing, so it is allowed in FIPS 140-only
// mode, where MD5 is otherwise refused.
func md5File(p string) (sum string, err error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	fips140.WithoutEnforcement(func() {
		h := md5.New()
		if _, err = io.Copy(h, f); err == nil {
			sum = hex.EncodeToString(h.Sum(nil))
		}
	})
	return sum, err
}

type progressWriter struct {
	w      io.Writer
	n      int64
	total  int64
	last   time.Time
	report func(n, total int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.n += int64(n)
	if time.Since(p.last) > 300*time.Millisecond {
		p.last = time.Now()
		p.report(p.n, p.total)
	}
	return n, err
}
