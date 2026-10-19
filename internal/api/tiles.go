package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// TileCache serves map tiles to the dispatcher's browser from a local directory of tiles or a tile
// server, keeping every tile it fetches on disk. Browsers never contact a tile server themselves, so the map
// keeps working from the cache when the internet does not, and no dispatcher's screen depends on a CDN.
type TileCache struct {
	mu       sync.Mutex
	template string // https://tile.example/{z}/{x}/{y}.png, or file:/path/{z}/{x}/{y}.png
	dir      string
	client   *http.Client
	agent    string
}

// NewTileCache keeps fetched tiles under dir.
func NewTileCache(dir, template, version string) *TileCache {
	return &TileCache{dir: dir, template: template, client: &http.Client{Timeout: 10 * time.Second}, agent: "bitdispatcher/" + version + " (+https://github.com/biodream-llc/bitdispatcher)"}
}

// SetTemplate changes the source; tiles already cached stay.
func (t *TileCache) SetTemplate(tpl string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.template = strings.TrimSpace(tpl)
}

// Configured reports a tile source.
func (t *TileCache) Configured() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.template != ""
}

var errNoTile = errors.New("no tile")

// Get returns a tile's bytes.
func (t *TileCache) Get(ctx context.Context, z, x, y int) ([]byte, string, error) {
	t.mu.Lock()
	tpl := t.template
	t.mu.Unlock()
	if z < 0 || z > 22 || x < 0 || y < 0 || x >= 1<<z || y >= 1<<z {
		return nil, "", errNoTile
	}
	cached := filepath.Join(t.dir, strconv.Itoa(z), strconv.Itoa(x), strconv.Itoa(y))
	if b, err := os.ReadFile(cached); err == nil {
		return b, sniff(b), nil
	}
	if tpl == "" {
		return nil, "", errNoTile
	}
	src := strings.NewReplacer("{z}", strconv.Itoa(z), "{x}", strconv.Itoa(x), "{y}", strconv.Itoa(y)).Replace(tpl)
	var b []byte
	if path, ok := strings.CutPrefix(src, "file:"); ok {
		var err error
		if b, err = os.ReadFile(filepath.Clean(path)); err != nil {
			return nil, "", errNoTile
		}
		return b, sniff(b), nil // a local tile directory is its own cache
	}
	if !strings.HasPrefix(src, "https://") && !strings.HasPrefix(src, "http://") {
		return nil, "", errNoTile
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", t.agent)
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", errNoTile
	}
	b, err = io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, "", err
	}
	ct := sniff(b)
	if !strings.HasPrefix(ct, "image/") {
		return nil, "", errNoTile
	}
	if err := os.MkdirAll(filepath.Dir(cached), 0o700); err == nil {
		tmp := cached + ".tmp"
		if os.WriteFile(tmp, b, 0o600) == nil {
			os.Rename(tmp, cached)
		}
	}
	return b, ct, nil
}

func sniff(b []byte) string { return http.DetectContentType(b) }

func (s *Server) tile(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
	z, e1 := strconv.Atoi(r.PathValue("z"))
	x, e2 := strconv.Atoi(r.PathValue("x"))
	y, e3 := strconv.Atoi(strings.TrimSuffix(strings.TrimSuffix(r.PathValue("y"), ".png"), ".jpg"))
	if e1 != nil || e2 != nil || e3 != nil {
		http.NotFound(w, r)
		return
	}
	b, ct, err := s.Tiles.Get(r.Context(), z, x, y)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(b)
}
