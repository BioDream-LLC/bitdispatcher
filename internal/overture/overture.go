// Package overture reads address points from the Overture Maps Foundation's addresses theme: GeoParquet
// files on a public bucket, read over https by byte range so that only the row groups whose bounding
// box meets the area are fetched (the files are sorted in space, so that is a small part of each).
//
// Overture's addresses are gathered from open government sources (in the United States largely the
// National Address Database); they fill in where the county has not published its own.
package overture

import (
	"context"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/format"

	"github.com/biodream-llc/bitdispatcher/internal/mapdata"
)

// Bucket is where Overture publishes its releases.
const Bucket = "https://overturemaps-us-west-2.s3.us-west-2.amazonaws.com"

// Client reads Overture releases.
type Client struct {
	Base  string // Bucket, or a test server
	HTTP  *http.Client
	Agent string
}

func (c Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

func (c Client) base() string { return strings.TrimRight(firstNonEmpty(c.Base, Bucket), "/") }

type listResult struct {
	Prefixes []struct {
		Prefix string `xml:"Prefix"`
	} `xml:"CommonPrefixes"`
	Contents []struct {
		Key  string `xml:"Key"`
		Size int64  `xml:"Size"`
	} `xml:"Contents"`
	Truncated bool   `xml:"IsTruncated"`
	Next      string `xml:"NextContinuationToken"`
}

func (c Client) list(ctx context.Context, prefix, delim string) (listResult, error) {
	var all listResult
	token := ""
	for {
		q := url.Values{"list-type": {"2"}, "prefix": {prefix}}
		if delim != "" {
			q.Set("delimiter", delim)
		}
		if token != "" {
			q.Set("continuation-token", token)
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", c.base()+"/?"+q.Encode(), nil)
		req.Header.Set("User-Agent", c.Agent)
		res, err := c.http().Do(req)
		if err != nil {
			return all, err
		}
		var lr listResult
		err = xml.NewDecoder(res.Body).Decode(&lr)
		res.Body.Close()
		if res.StatusCode != 200 {
			return all, fmt.Errorf("overture: listing %s: %s", prefix, res.Status)
		}
		if err != nil {
			return all, fmt.Errorf("overture: listing %s: %w", prefix, err)
		}
		all.Prefixes = append(all.Prefixes, lr.Prefixes...)
		all.Contents = append(all.Contents, lr.Contents...)
		if !lr.Truncated || lr.Next == "" {
			return all, nil
		}
		token = lr.Next
	}
}

// Latest is the newest release name, such as 2026-09-23.1.
func (c Client) Latest(ctx context.Context) (string, error) {
	lr, err := c.list(ctx, "release/", "/")
	if err != nil {
		return "", err
	}
	var rs []string
	for _, p := range lr.Prefixes {
		if r := strings.TrimSuffix(strings.TrimPrefix(p.Prefix, "release/"), "/"); r != "" {
			rs = append(rs, r)
		}
	}
	if len(rs) == 0 {
		return "", errors.New("overture: no release published")
	}
	sort.Strings(rs)
	return rs[len(rs)-1], nil
}

// file is one GeoParquet file of a release.
type file struct {
	key  string
	size int64
}

func (c Client) files(ctx context.Context, release string) ([]file, error) {
	lr, err := c.list(ctx, "release/"+release+"/theme=addresses/type=address/", "")
	if err != nil {
		return nil, err
	}
	var out []file
	for _, o := range lr.Contents {
		if strings.HasSuffix(o.Key, ".parquet") {
			out = append(out, file{o.Key, o.Size})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("overture: release %s has no address files", release)
	}
	return out, nil
}

// rangeReader reads a remote file by byte range, keeping what it has read in 1 MiB blocks.
type rangeReader struct {
	ctx    context.Context
	c      Client
	url    string
	size   int64
	mu     sync.Mutex
	blocks map[int64][]byte
	order  []int64
	read   int64 // bytes fetched
}

const block = 1 << 20

func (r *rangeReader) ReadAt(p []byte, off int64) (int, error) {
	if off >= r.size {
		return 0, io.EOF
	}
	n := 0
	for n < len(p) && off+int64(n) < r.size {
		at := off + int64(n)
		b, err := r.block(at / block)
		if err != nil {
			return n, err
		}
		n += copy(p[n:], b[at%block:])
	}
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (r *rangeReader) block(i int64) ([]byte, error) {
	r.mu.Lock()
	if b, ok := r.blocks[i]; ok {
		r.mu.Unlock()
		return b, nil
	}
	r.mu.Unlock()
	from := i * block
	to := min(from+block, r.size) - 1
	var b []byte
	var err error
	for try := 0; try < 4; try++ {
		if try > 0 {
			select {
			case <-r.ctx.Done():
				return nil, r.ctx.Err()
			case <-time.After(time.Duration(try) * 2 * time.Second):
			}
		}
		b, err = r.fetch(from, to)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.read += int64(len(b))
	r.blocks[i] = b
	r.order = append(r.order, i)
	// Keep at most 256 MiB.
	for len(r.order) > 256 {
		delete(r.blocks, r.order[0])
		r.order = r.order[1:]
	}
	return b, nil
}

func (r *rangeReader) fetch(from, to int64) ([]byte, error) {
	req, _ := http.NewRequestWithContext(r.ctx, "GET", r.url, nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", from, to))
	req.Header.Set("User-Agent", r.c.Agent)
	res, err := r.c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("overture: %s: %s", r.url, res.Status)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, to-from+1))
	if err == nil && int64(len(b)) != to-from+1 {
		err = io.ErrUnexpectedEOF
	}
	return b, err
}

// row is the part of Overture's address schema used here.
type row struct {
	BBox struct {
		XMin float32 `parquet:"xmin"`
		XMax float32 `parquet:"xmax"`
		YMin float32 `parquet:"ymin"`
		YMax float32 `parquet:"ymax"`
	} `parquet:"bbox"`
	Country    *string `parquet:"country,optional"`
	Postcode   *string `parquet:"postcode,optional"`
	Street     *string `parquet:"street,optional"`
	Number     *string `parquet:"number,optional"`
	Unit       *string `parquet:"unit,optional"`
	PostalCity *string `parquet:"postal_city,optional"`
	Levels     []struct {
		Value *string `parquet:"value,optional"`
	} `parquet:"address_levels,optional,list"`
}

// Progress is how a read is going.
type Progress struct {
	Release   string `json:"release"`
	Files     int    `json:"files"`
	FilesDone int    `json:"filesDone"`
	Groups    int    `json:"groups"` // row groups in the area
	Bytes     int64  `json:"bytes"`  // fetched
	Found     int    `json:"found"`
}

// Read sends every address point of a release inside the area; release empty is the latest.
func (c Client) Read(ctx context.Context, release string, area mapdata.Area, prog func(Progress), fn func(mapdata.GISFeature)) error {
	if release == "" {
		var err error
		if release, err = c.Latest(ctx); err != nil {
			return err
		}
	}
	files, err := c.files(ctx, release)
	if err != nil {
		return err
	}
	box := area.Box()
	p := Progress{Release: release, Files: len(files)}
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		rr := &rangeReader{ctx: ctx, c: c, url: c.base() + "/" + (&url.URL{Path: f.key}).EscapedPath(), size: f.size, blocks: map[int64][]byte{}}
		pf, err := parquet.OpenFile(rr, f.size, parquet.SkipPageIndex(true), parquet.SkipBloomFilters(true), parquet.ReadBufferSize(block))
		if err != nil {
			return fmt.Errorf("overture: %s: %w", f.key, err)
		}
		idx := columns(pf, "bbox.xmin", "bbox.xmax", "bbox.ymin", "bbox.ymax")
		for gi, rg := range pf.RowGroups() {
			if idx != nil {
				st := pf.Metadata().RowGroups[gi].Columns
				xmin, ok1 := statFloat(st[idx[0]].MetaData.Statistics, false)
				xmax, ok2 := statFloat(st[idx[1]].MetaData.Statistics, true)
				ymin, ok3 := statFloat(st[idx[2]].MetaData.Statistics, false)
				ymax, ok4 := statFloat(st[idx[3]].MetaData.Statistics, true)
				if ok1 && ok2 && ok3 && ok4 && (xmax < box.MinLon || xmin > box.MaxLon || ymax < box.MinLat || ymin > box.MaxLat) {
					continue
				}
			}
			p.Groups++
			r := parquet.NewGenericRowGroupReader[row](rg)
			buf := make([]row, 2048)
			for {
				n, err := r.Read(buf)
				for _, x := range buf[:n] {
					lat, lon := float64(x.BBox.YMin+x.BBox.YMax)/2, float64(x.BBox.XMin+x.BBox.XMax)/2
					if !area.Contains(lat, lon) || x.Number == nil || x.Street == nil {
						continue
					}
					g := mapdata.GISFeature{Kind: mapdata.GISAddress, Num: *x.Number, Street: *x.Street, Point: &[2]float64{lon, lat}, Src: "overture"}
					if x.Unit != nil {
						g.Unit = *x.Unit
					}
					if x.Postcode != nil {
						g.Zip = (*x.Postcode)[:min(5, len(*x.Postcode))]
					}
					switch {
					case x.PostalCity != nil:
						g.City = *x.PostalCity
					case len(x.Levels) > 1 && x.Levels[len(x.Levels)-1].Value != nil:
						g.City = *x.Levels[len(x.Levels)-1].Value
					}
					p.Found++
					fn(g)
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					r.Close()
					return fmt.Errorf("overture: %s: %w", f.key, err)
				}
				if ctx.Err() != nil {
					r.Close()
					return ctx.Err()
				}
			}
			r.Close()
			p.Bytes += rr.read
			rr.read = 0
			if prog != nil {
				prog(p)
			}
		}
		p.FilesDone++
		p.Bytes += rr.read
		if prog != nil {
			prog(p)
		}
	}
	return nil
}

// columns finds the leaf column indexes for dotted paths, or nil.
func columns(f *parquet.File, paths ...string) []int {
	var out []int
	for _, p := range paths {
		leaf, ok := f.Schema().Lookup(strings.Split(p, ".")...)
		if !ok {
			return nil
		}
		out = append(out, leaf.ColumnIndex)
	}
	return out
}

// statFloat reads a FLOAT or DOUBLE column's minimum (or maximum) from its statistics, PLAIN encoded.
func statFloat(s format.Statistics, max bool) (float64, bool) {
	b := s.MinValue
	if max {
		b = s.MaxValue
	}
	if len(b) == 0 {
		b = s.Min
		if max {
			b = s.Max
		}
	}
	switch len(b) {
	case 4:
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(b))), true
	case 8:
		return math.Float64frombits(binary.LittleEndian.Uint64(b)), true
	}
	return 0, false
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}
