// Package osm reads OpenStreetMap PBF extracts (https://wiki.openstreetmap.org/wiki/PBF_Format) with the
// standard library only: the protobuf wire format is decoded by hand and blobs are inflated with zlib.
// Blocks are decoded on several goroutines; a 400 MB state extract reads in seconds.
package osm

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
)

// Node is a point. Lat and Lon are in units of 1e-7 degrees, OpenStreetMap's own precision.
type Node struct {
	ID       int64
	Lat, Lon int32
	Tags     map[string]string // nil when the node has none
}

// Way is an ordered list of node references.
type Way struct {
	ID   int64
	Refs []int64
	Tags map[string]string
}

// Member is one member of a relation.
type Member struct {
	Type byte // 'n', 'w' or 'r'
	ID   int64
	Role string
}

// Relation groups members, such as the outer and inner ways of a lake.
type Relation struct {
	ID      int64
	Members []Member
	Tags    map[string]string
}

// Block is one decoded primitive block.
type Block struct {
	Nodes     []Node
	Ways      []Way
	Relations []Relation
}

// Want chooses what a scan decodes; the rest is skipped without building it.
type Want struct{ Nodes, Ways, Relations bool }

// Progress reports bytes read so far of the file's total.
type Progress func(read, total int64)

// Scan reads the extract at path and calls fn for every block holding something wanted. fn is called from
// several goroutines at once; worker (0..workers-1) lets it keep per-goroutine results without locking.
func Scan(path string, want Want, workers int, progress Progress, fn func(worker int, b *Block) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	r := bufio.NewReaderSize(f, 1<<20)
	jobs := make(chan []byte, workers*2)
	var wg sync.WaitGroup
	var once sync.Once
	var firstErr error
	failed := make(chan struct{})
	setErr := func(err error) {
		once.Do(func() {
			firstErr = err
			close(failed)
		})
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			var buf bytes.Buffer
			for blob := range jobs {
				raw, err := inflate(blob, &buf)
				if err != nil {
					setErr(err)
					continue
				}
				b, err := decodeBlock(raw, want)
				if err != nil {
					setErr(err)
					continue
				}
				if len(b.Nodes)+len(b.Ways)+len(b.Relations) == 0 {
					continue
				}
				if err := fn(w, b); err != nil {
					setErr(err)
				}
			}
		}(w)
	}
	var read int64
	var hdr [4]byte
	sawHeader := false
loop:
	for {
		select {
		case <-failed:
			break loop
		default:
		}
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			setErr(fmt.Errorf("osm: %w", err))
			break
		}
		n := binary.BigEndian.Uint32(hdr[:])
		if n > 64<<10 {
			setErr(fmt.Errorf("osm: blob header of %d bytes is too large", n))
			break
		}
		hb := make([]byte, n)
		if _, err := io.ReadFull(r, hb); err != nil {
			setErr(fmt.Errorf("osm: %w", err))
			break
		}
		typ, size, err := blobHeader(hb)
		if err != nil {
			setErr(err)
			break
		}
		if size > 32<<20 {
			setErr(fmt.Errorf("osm: blob of %d bytes is too large", size))
			break
		}
		blob := make([]byte, size)
		if _, err := io.ReadFull(r, blob); err != nil {
			setErr(fmt.Errorf("osm: %w", err))
			break
		}
		read += int64(4 + n + uint32(size))
		if progress != nil {
			progress(read, st.Size())
		}
		switch typ {
		case "OSMHeader":
			var buf bytes.Buffer
			raw, err := inflate(blob, &buf)
			if err == nil {
				err = checkHeader(raw)
			}
			if err != nil {
				setErr(err)
				break loop
			}
			sawHeader = true
		case "OSMData":
			if !sawHeader {
				setErr(errors.New("osm: data before the header: not an OpenStreetMap PBF file"))
				break loop
			}
			select {
			case jobs <- blob:
			case <-failed:
				break loop
			}
		}
	}
	close(jobs)
	wg.Wait()
	if firstErr == nil && !sawHeader {
		return errors.New("osm: no header: not an OpenStreetMap PBF file")
	}
	return firstErr
}

func blobHeader(b []byte) (typ string, size int, err error) {
	p := pb{b: b}
	for p.more() {
		f, wt := p.key()
		switch {
		case f == 1 && wt == 2:
			typ = string(p.bytes())
		case f == 3 && wt == 0:
			size = int(p.varint())
		default:
			p.skip(wt)
		}
	}
	if p.err != nil {
		return "", 0, p.err
	}
	return typ, size, nil
}

func inflate(blob []byte, buf *bytes.Buffer) ([]byte, error) {
	p := pb{b: blob}
	var raw, z []byte
	rawSize := 0
	for p.more() {
		f, wt := p.key()
		switch {
		case f == 1 && wt == 2:
			raw = p.bytes()
		case f == 2 && wt == 0:
			rawSize = int(p.varint())
		case f == 3 && wt == 2:
			z = p.bytes()
		case (f == 4 || f == 5 || f == 6 || f == 7) && wt == 2:
			return nil, errors.New("osm: blob compression other than zlib is not supported")
		default:
			p.skip(wt)
		}
	}
	if p.err != nil {
		return nil, p.err
	}
	if raw != nil {
		return raw, nil
	}
	if z == nil {
		return nil, errors.New("osm: empty blob")
	}
	if rawSize > 64<<20 {
		return nil, fmt.Errorf("osm: block of %d bytes is too large", rawSize)
	}
	zr, err := zlib.NewReader(bytes.NewReader(z))
	if err != nil {
		return nil, fmt.Errorf("osm: %w", err)
	}
	defer zr.Close()
	buf.Reset()
	buf.Grow(rawSize)
	if _, err := io.Copy(buf, io.LimitReader(zr, 64<<20)); err != nil {
		return nil, fmt.Errorf("osm: %w", err)
	}
	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return out, nil
}

// checkHeader refuses files that need a feature this reader does not have.
func checkHeader(b []byte) error {
	p := pb{b: b}
	for p.more() {
		f, wt := p.key()
		if f == 4 && wt == 2 {
			switch feat := string(p.bytes()); feat {
			case "OsmSchema-V0.6", "DenseNodes", "HistoricalInformation":
			default:
				return fmt.Errorf("osm: the file needs %q, which this reader does not support", feat)
			}
			continue
		}
		p.skip(wt)
	}
	return p.err
}

type blockCtx struct {
	strings    [][]byte
	gran       int64
	latOff     int64
	lonOff     int64
	want       Want
	internMemo map[string]string
}

func (c *blockCtx) str(i uint64) string {
	if i >= uint64(len(c.strings)) {
		return ""
	}
	b := c.strings[i]
	if s, ok := c.internMemo[string(b)]; ok {
		return s
	}
	s := string(b)
	c.internMemo[s] = s
	return s
}

// coord converts a block's raw coordinate to 1e-7 degrees.
func (c *blockCtx) coord(v, off int64) int32 { return int32((off + c.gran*v) / 100) }

func decodeBlock(b []byte, want Want) (*Block, error) {
	c := &blockCtx{gran: 100, want: want, internMemo: map[string]string{}}
	var groups [][]byte
	p := pb{b: b}
	for p.more() {
		f, wt := p.key()
		switch {
		case f == 1 && wt == 2:
			st := pb{b: p.bytes()}
			for st.more() {
				f2, wt2 := st.key()
				if f2 == 1 && wt2 == 2 {
					c.strings = append(c.strings, st.bytes())
				} else {
					st.skip(wt2)
				}
			}
			if st.err != nil {
				return nil, st.err
			}
		case f == 2 && wt == 2:
			groups = append(groups, p.bytes())
		case f == 17 && wt == 0:
			c.gran = int64(p.varint())
		case f == 19 && wt == 0:
			c.latOff = int64(p.varint())
		case f == 20 && wt == 0:
			c.lonOff = int64(p.varint())
		default:
			p.skip(wt)
		}
	}
	if p.err != nil {
		return nil, p.err
	}
	out := &Block{}
	for _, g := range groups {
		if err := c.group(g, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (c *blockCtx) group(b []byte, out *Block) error {
	p := pb{b: b}
	for p.more() {
		f, wt := p.key()
		if wt != 2 {
			p.skip(wt)
			continue
		}
		v := p.bytes()
		switch f {
		case 1:
			if c.want.Nodes {
				if err := c.node(v, out); err != nil {
					return err
				}
			}
		case 2:
			if c.want.Nodes {
				if err := c.dense(v, out); err != nil {
					return err
				}
			}
		case 3:
			if c.want.Ways {
				if err := c.way(v, out); err != nil {
					return err
				}
			}
		case 4:
			if c.want.Relations {
				if err := c.relation(v, out); err != nil {
					return err
				}
			}
		}
	}
	return p.err
}

func (c *blockCtx) tags(keys, vals []uint64) map[string]string {
	if len(keys) == 0 {
		return nil
	}
	m := make(map[string]string, len(keys))
	for i, k := range keys {
		if i < len(vals) {
			m[c.str(k)] = c.str(vals[i])
		}
	}
	return m
}

func (c *blockCtx) node(b []byte, out *Block) error {
	p := pb{b: b}
	var n Node
	var keys, vals []uint64
	var lat, lon int64
	for p.more() {
		f, wt := p.key()
		switch {
		case f == 1 && wt == 0:
			n.ID = zigzag(p.varint())
		case f == 2 && wt == 2:
			keys = p.packed(keys[:0])
		case f == 3 && wt == 2:
			vals = p.packed(vals[:0])
		case f == 8 && wt == 0:
			lat = zigzag(p.varint())
		case f == 9 && wt == 0:
			lon = zigzag(p.varint())
		default:
			p.skip(wt)
		}
	}
	n.Lat, n.Lon = c.coord(lat, c.latOff), c.coord(lon, c.lonOff)
	n.Tags = c.tags(keys, vals)
	out.Nodes = append(out.Nodes, n)
	return p.err
}

func (c *blockCtx) dense(b []byte, out *Block) error {
	p := pb{b: b}
	var ids, lats, lons, kv []uint64
	for p.more() {
		f, wt := p.key()
		switch {
		case f == 1 && wt == 2:
			ids = p.packed(nil)
		case f == 8 && wt == 2:
			lats = p.packed(nil)
		case f == 9 && wt == 2:
			lons = p.packed(nil)
		case f == 10 && wt == 2:
			kv = p.packed(nil)
		default:
			p.skip(wt)
		}
	}
	if p.err != nil {
		return p.err
	}
	if len(lats) != len(ids) || len(lons) != len(ids) {
		return errors.New("osm: dense nodes with mismatched columns")
	}
	var id, lat, lon int64
	k := 0
	start := len(out.Nodes)
	out.Nodes = append(out.Nodes, make([]Node, len(ids))...)
	for i := range ids {
		id += zigzag(ids[i])
		lat += zigzag(lats[i])
		lon += zigzag(lons[i])
		n := &out.Nodes[start+i]
		n.ID, n.Lat, n.Lon = id, c.coord(lat, c.latOff), c.coord(lon, c.lonOff)
		// keys_vals: key, value pairs for each node, each node's list ended by 0.
		for k < len(kv) && kv[k] != 0 {
			if k+1 >= len(kv) {
				return errors.New("osm: dense node tags truncated")
			}
			if n.Tags == nil {
				n.Tags = map[string]string{}
			}
			n.Tags[c.str(kv[k])] = c.str(kv[k+1])
			k += 2
		}
		k++ // the 0
	}
	return nil
}

func (c *blockCtx) way(b []byte, out *Block) error {
	p := pb{b: b}
	var w Way
	var keys, vals, refs []uint64
	for p.more() {
		f, wt := p.key()
		switch {
		case f == 1 && wt == 0:
			w.ID = int64(p.varint())
		case f == 2 && wt == 2:
			keys = p.packed(nil)
		case f == 3 && wt == 2:
			vals = p.packed(nil)
		case f == 8 && wt == 2:
			refs = p.packed(nil)
		default:
			p.skip(wt)
		}
	}
	w.Refs = make([]int64, len(refs))
	var id int64
	for i, r := range refs {
		id += zigzag(r)
		w.Refs[i] = id
	}
	w.Tags = c.tags(keys, vals)
	out.Ways = append(out.Ways, w)
	return p.err
}

func (c *blockCtx) relation(b []byte, out *Block) error {
	p := pb{b: b}
	var r Relation
	var keys, vals, roles, mems, types []uint64
	for p.more() {
		f, wt := p.key()
		switch {
		case f == 1 && wt == 0:
			r.ID = int64(p.varint())
		case f == 2 && wt == 2:
			keys = p.packed(nil)
		case f == 3 && wt == 2:
			vals = p.packed(nil)
		case f == 8 && wt == 2:
			roles = p.packed(nil)
		case f == 9 && wt == 2:
			mems = p.packed(nil)
		case f == 10 && wt == 2:
			types = p.packed(nil)
		default:
			p.skip(wt)
		}
	}
	if len(roles) != len(mems) || len(types) != len(mems) {
		return errors.New("osm: relation with mismatched member columns")
	}
	var id int64
	r.Members = make([]Member, len(mems))
	for i := range mems {
		id += zigzag(mems[i])
		t := byte('n')
		switch types[i] {
		case 1:
			t = 'w'
		case 2:
			t = 'r'
		}
		r.Members[i] = Member{Type: t, ID: id, Role: c.str(roles[i])}
	}
	r.Tags = c.tags(keys, vals)
	out.Relations = append(out.Relations, r)
	return p.err
}

func zigzag(v uint64) int64 { return int64(v>>1) ^ -int64(v&1) }

// pb is a protobuf wire-format cursor. Errors stick: after one, every read returns zero.
type pb struct {
	b   []byte
	i   int
	err error
}

func (p *pb) more() bool { return p.err == nil && p.i < len(p.b) }

func (p *pb) fail() {
	if p.err == nil {
		p.err = errors.New("osm: malformed protobuf")
	}
	p.i = len(p.b)
}

func (p *pb) varint() uint64 {
	var v uint64
	for s := uint(0); s < 64; s += 7 {
		if p.i >= len(p.b) {
			p.fail()
			return 0
		}
		c := p.b[p.i]
		p.i++
		v |= uint64(c&0x7f) << s
		if c < 0x80 {
			return v
		}
	}
	p.fail()
	return 0
}

func (p *pb) key() (field, wireType int) {
	k := p.varint()
	return int(k >> 3), int(k & 7)
}

func (p *pb) bytes() []byte {
	n := p.varint()
	if p.err != nil || n > uint64(len(p.b)-p.i) {
		p.fail()
		return nil
	}
	v := p.b[p.i : p.i+int(n)]
	p.i += int(n)
	return v
}

func (p *pb) packed(dst []uint64) []uint64 {
	q := pb{b: p.bytes()}
	for q.more() {
		dst = append(dst, q.varint())
	}
	if q.err != nil {
		p.err = q.err
	}
	return dst
}

func (p *pb) skip(wt int) {
	switch wt {
	case 0:
		p.varint()
	case 1:
		p.i += 8
	case 2:
		p.bytes()
	case 5:
		p.i += 4
	default:
		p.fail()
	}
	if p.i > len(p.b) {
		p.fail()
	}
}
