package osm

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"sync"
)

// Header is what an extract says about itself: where its daily change files are and which one it is
// up to (Geofabrik writes these osmosis replication fields into every extract).
type Header struct {
	Timestamp int64  // seconds since 1970 (field 32)
	Sequence  int64  // replication sequence number (field 33)
	BaseURL   string // replication base URL (field 34)
}

// ReadHeader reads an extract's header block.
func ReadHeader(path string) (Header, error) {
	f, err := os.Open(path)
	if err != nil {
		return Header{}, err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	var n [4]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return Header{}, fmt.Errorf("osm: %w", err)
	}
	size := int(n[0])<<24 | int(n[1])<<16 | int(n[2])<<8 | int(n[3])
	if size <= 0 || size > 64<<10 {
		return Header{}, errors.New("osm: not an OpenStreetMap PBF file")
	}
	hb := make([]byte, size)
	if _, err := io.ReadFull(r, hb); err != nil {
		return Header{}, err
	}
	typ, bsize, err := blobHeader(hb)
	if err != nil || typ != "OSMHeader" || bsize > 32<<20 {
		return Header{}, errors.New("osm: the file does not start with a header")
	}
	blob := make([]byte, bsize)
	if _, err := io.ReadFull(r, blob); err != nil {
		return Header{}, err
	}
	var buf bytes.Buffer
	raw, err := inflate(blob, &buf)
	if err != nil {
		return Header{}, err
	}
	var h Header
	p := pb{b: raw}
	for p.more() {
		f, wt := p.key()
		switch {
		case f == 32 && wt == 0:
			h.Timestamp = int64(p.varint())
		case f == 33 && wt == 0:
			h.Sequence = int64(p.varint())
		case f == 34 && wt == 2:
			h.BaseURL = string(p.bytes())
		default:
			p.skip(wt)
		}
	}
	return h, p.err
}

// Change is what a set of change files does: the final version of every element they touch, and which
// they delete. Applied in order, a later file wins.
type Change struct {
	Nodes     map[int64]*Node // nil: deleted
	Ways      map[int64]*Way
	Relations map[int64]*Relation
}

// NewChange is an empty change.
func NewChange() *Change {
	return &Change{Nodes: map[int64]*Node{}, Ways: map[int64]*Way{}, Relations: map[int64]*Relation{}}
}

// Size counts the elements touched.
func (c *Change) Size() int { return len(c.Nodes) + len(c.Ways) + len(c.Relations) }

// ReadChange adds an osmChange file (.osc, or .osc.gz when gzipped) to c.
func (c *Change) ReadChange(r io.Reader, gzipped bool) error {
	if gzipped {
		zr, err := gzip.NewReader(r)
		if err != nil {
			return fmt.Errorf("osm: change file: %w", err)
		}
		defer zr.Close()
		r = zr
	}
	d := xml.NewDecoder(io.LimitReader(r, 4<<30))
	action := ""
	var node *Node
	var way *Way
	var rel *Relation
	attr := func(se xml.StartElement, k string) string {
		for _, a := range se.Attr {
			if a.Name.Local == k {
				return a.Value
			}
		}
		return ""
	}
	tag := func(m *map[string]string, se xml.StartElement) {
		if *m == nil {
			*m = map[string]string{}
		}
		(*m)[attr(se, "k")] = attr(se, "v")
	}
	for {
		t, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("osm: change file: %w", err)
		}
		switch e := t.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "create", "modify", "delete":
				action = e.Name.Local
			case "node":
				id, _ := strconv.ParseInt(attr(e, "id"), 10, 64)
				lat, _ := strconv.ParseFloat(attr(e, "lat"), 64)
				lon, _ := strconv.ParseFloat(attr(e, "lon"), 64)
				node = &Node{ID: id, Lat: int32(math.Round(lat * 1e7)), Lon: int32(math.Round(lon * 1e7))}
			case "way":
				id, _ := strconv.ParseInt(attr(e, "id"), 10, 64)
				way = &Way{ID: id}
			case "relation":
				id, _ := strconv.ParseInt(attr(e, "id"), 10, 64)
				rel = &Relation{ID: id}
			case "nd":
				if way != nil {
					ref, _ := strconv.ParseInt(attr(e, "ref"), 10, 64)
					way.Refs = append(way.Refs, ref)
				}
			case "member":
				if rel != nil {
					ref, _ := strconv.ParseInt(attr(e, "ref"), 10, 64)
					t := byte('n')
					switch attr(e, "type") {
					case "way":
						t = 'w'
					case "relation":
						t = 'r'
					}
					rel.Members = append(rel.Members, Member{Type: t, ID: ref, Role: attr(e, "role")})
				}
			case "tag":
				switch {
				case node != nil:
					tag(&node.Tags, e)
				case way != nil:
					tag(&way.Tags, e)
				case rel != nil:
					tag(&rel.Tags, e)
				}
			}
		case xml.EndElement:
			switch e.Name.Local {
			case "node":
				if action == "delete" {
					c.Nodes[node.ID] = nil
				} else {
					c.Nodes[node.ID] = node
				}
				node = nil
			case "way":
				if action == "delete" {
					c.Ways[way.ID] = nil
				} else {
					c.Ways[way.ID] = way
				}
				way = nil
			case "relation":
				if action == "delete" {
					c.Relations[rel.ID] = nil
				} else {
					c.Relations[rel.ID] = rel
				}
				rel = nil
			}
		}
	}
}

// Writer writes a PBF file element by element, a block at a time.
type Writer struct {
	w     io.Writer
	nodes []Node
	ways  []Way
	rels  []Relation
	err   error
}

const perBlock = 8000

// NewWriter starts a file with a header carrying h.
func NewWriter(w io.Writer, h Header) (*Writer, error) {
	hdr := pbBytesW(nil, 4, []byte("OsmSchema-V0.6"))
	hdr = pbBytesW(hdr, 4, []byte("DenseNodes"))
	hdr = pbBytesW(hdr, 16, []byte("bitdispatcher"))
	if h.Timestamp > 0 {
		hdr = pbVarintW(hdr, 32, uint64(h.Timestamp))
	}
	if h.Sequence > 0 {
		hdr = pbVarintW(hdr, 33, uint64(h.Sequence))
	}
	if h.BaseURL != "" {
		hdr = pbBytesW(hdr, 34, []byte(h.BaseURL))
	}
	if err := writeBlob(w, "OSMHeader", hdr); err != nil {
		return nil, err
	}
	return &Writer{w: w}, nil
}

func (w *Writer) flush(force bool) {
	if w.err != nil {
		return
	}
	if len(w.nodes) >= perBlock || (force && len(w.nodes) > 0) {
		w.err = writeBlob(w.w, "OSMData", block(w.nodes, nil, nil))
		w.nodes = w.nodes[:0]
	}
	if w.err == nil && (len(w.ways) >= perBlock || (force && len(w.ways) > 0)) {
		w.err = writeBlob(w.w, "OSMData", block(nil, w.ways, nil))
		w.ways = w.ways[:0]
	}
	if w.err == nil && (len(w.rels) >= perBlock || (force && len(w.rels) > 0)) {
		w.err = writeBlob(w.w, "OSMData", block(nil, nil, w.rels))
		w.rels = w.rels[:0]
	}
}

// Node adds a node.
func (w *Writer) Node(n Node) { w.nodes = append(w.nodes, n); w.flush(false) }

// Way adds a way.
func (w *Writer) Way(x Way) { w.ways = append(w.ways, x); w.flush(false) }

// Relation adds a relation.
func (w *Writer) Relation(r Relation) { w.rels = append(w.rels, r); w.flush(false) }

// Close writes what is left.
func (w *Writer) Close() error { w.flush(true); return w.err }

// Apply writes src with the change applied to dst, with header h (the new replication state). Elements are
// written in the order they are read, then the new ones: bitdispatcher's importer does not need a sorted
// file.
func Apply(src, dst string, c *Change, h Header, workers int) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	bw := bufio.NewWriterSize(out, 1<<20)
	w, err := NewWriter(bw, h)
	if err != nil {
		out.Close()
		return err
	}
	var mu sync.Mutex
	seenN, seenW, seenR := map[int64]bool{}, map[int64]bool{}, map[int64]bool{}
	err = Scan(src, Want{Nodes: true, Ways: true, Relations: true}, workers, nil, func(_ int, b *Block) error {
		mu.Lock()
		defer mu.Unlock()
		for _, n := range b.Nodes {
			if x, ok := c.Nodes[n.ID]; ok {
				seenN[n.ID] = true
				if x != nil {
					w.Node(*x)
				}
				continue
			}
			w.Node(n)
		}
		for _, x := range b.Ways {
			if y, ok := c.Ways[x.ID]; ok {
				seenW[x.ID] = true
				if y != nil {
					w.Way(*y)
				}
				continue
			}
			w.Way(x)
		}
		for _, x := range b.Relations {
			if y, ok := c.Relations[x.ID]; ok {
				seenR[x.ID] = true
				if y != nil {
					w.Relation(*y)
				}
				continue
			}
			w.Relation(x)
		}
		return w.err
	})
	if err == nil {
		// New elements, and changed ones the extract did not have (they crossed into it).
		for id, n := range c.Nodes {
			if n != nil && !seenN[id] {
				w.Node(*n)
			}
		}
		for id, x := range c.Ways {
			if x != nil && !seenW[id] {
				w.Way(*x)
			}
		}
		for id, x := range c.Relations {
			if x != nil && !seenR[id] {
				w.Relation(*x)
			}
		}
		err = w.Close()
	}
	if err == nil {
		err = bw.Flush()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dst)
	}
	return err
}
