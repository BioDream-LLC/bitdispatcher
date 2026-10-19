package osm

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"
	"sort"
)

// Write writes a small PBF file holding the given elements, one block each for nodes (dense), ways and
// relations. It is for tests and fixtures: it makes files this package and other PBF readers accept,
// without the size optimisations of a real extract writer.
func Write(w io.Writer, nodes []Node, ways []Way, rels []Relation) error {
	hdr := pbBytesW(nil, 4, []byte("OsmSchema-V0.6"))
	hdr = pbBytesW(hdr, 4, []byte("DenseNodes"))
	if err := writeBlob(w, "OSMHeader", hdr); err != nil {
		return err
	}
	if len(nodes) > 0 {
		if err := writeBlob(w, "OSMData", block(nodes, nil, nil)); err != nil {
			return err
		}
	}
	if len(ways) > 0 {
		if err := writeBlob(w, "OSMData", block(nil, ways, nil)); err != nil {
			return err
		}
	}
	if len(rels) > 0 {
		if err := writeBlob(w, "OSMData", block(nil, nil, rels)); err != nil {
			return err
		}
	}
	return nil
}

func writeBlob(w io.Writer, typ string, raw []byte) error {
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	zw.Write(raw)
	zw.Close()
	blob := pbVarintW(nil, 2, uint64(len(raw)))
	blob = pbBytesW(blob, 3, z.Bytes())
	h := pbBytesW(nil, 1, []byte(typ))
	h = pbVarintW(h, 3, uint64(len(blob)))
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(h)))
	for _, b := range [][]byte{n[:], h, blob} {
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	return nil
}

type strtab struct {
	idx  map[string]uint64
	list []string
}

func (s *strtab) id(v string) uint64 {
	if i, ok := s.idx[v]; ok {
		return i
	}
	i := uint64(len(s.list))
	s.idx[v] = i
	s.list = append(s.list, v)
	return i
}

func sortedKeys(m map[string]string) []string {
	k := make([]string, 0, len(m))
	for x := range m {
		k = append(k, x)
	}
	sort.Strings(k)
	return k
}

func zz64(v int64) uint64 { return uint64((v << 1) ^ (v >> 63)) }

func block(nodes []Node, ways []Way, rels []Relation) []byte {
	st := &strtab{idx: map[string]uint64{}}
	st.id("") // index 0 is reserved
	var group []byte
	if len(nodes) > 0 {
		var ids, lats, lons, kv []uint64
		var pid, plat, plon int64
		for _, n := range nodes {
			// Coordinates in the default granularity of 100 nanodegrees, which is 1e-7 degrees.
			ids = append(ids, zz64(n.ID-pid))
			lats = append(lats, zz64(int64(n.Lat)-plat))
			lons = append(lons, zz64(int64(n.Lon)-plon))
			pid, plat, plon = n.ID, int64(n.Lat), int64(n.Lon)
			for _, k := range sortedKeys(n.Tags) {
				kv = append(kv, st.id(k), st.id(n.Tags[k]))
			}
			kv = append(kv, 0)
		}
		d := pbPackedW(nil, 1, ids)
		d = pbPackedW(d, 8, lats)
		d = pbPackedW(d, 9, lons)
		d = pbPackedW(d, 10, kv)
		group = pbBytesW(group, 2, d)
	}
	for _, w := range ways {
		var keys, vals, refs []uint64
		for _, k := range sortedKeys(w.Tags) {
			keys = append(keys, st.id(k))
			vals = append(vals, st.id(w.Tags[k]))
		}
		var prev int64
		for _, r := range w.Refs {
			refs = append(refs, zz64(r-prev))
			prev = r
		}
		b := pbVarintW(nil, 1, uint64(w.ID))
		b = pbPackedW(b, 2, keys)
		b = pbPackedW(b, 3, vals)
		b = pbPackedW(b, 8, refs)
		group = pbBytesW(group, 3, b)
	}
	for _, r := range rels {
		var keys, vals, roles, mems, types []uint64
		for _, k := range sortedKeys(r.Tags) {
			keys = append(keys, st.id(k))
			vals = append(vals, st.id(r.Tags[k]))
		}
		var prev int64
		for _, m := range r.Members {
			roles = append(roles, st.id(m.Role))
			mems = append(mems, zz64(m.ID-prev))
			prev = m.ID
			t := uint64(0)
			switch m.Type {
			case 'w':
				t = 1
			case 'r':
				t = 2
			}
			types = append(types, t)
		}
		b := pbVarintW(nil, 1, uint64(r.ID))
		b = pbPackedW(b, 2, keys)
		b = pbPackedW(b, 3, vals)
		b = pbPackedW(b, 8, roles)
		b = pbPackedW(b, 9, mems)
		b = pbPackedW(b, 10, types)
		group = pbBytesW(group, 4, b)
	}
	var tab []byte
	for _, s := range st.list {
		tab = pbBytesW(tab, 1, []byte(s))
	}
	out := pbBytesW(nil, 1, tab)
	return pbBytesW(out, 2, group)
}

func pbVarintW(b []byte, field int, v uint64) []byte {
	b = binary.AppendUvarint(b, uint64(field<<3))
	return binary.AppendUvarint(b, v)
}

func pbBytesW(b []byte, field int, v []byte) []byte {
	b = binary.AppendUvarint(b, uint64(field<<3|2))
	b = binary.AppendUvarint(b, uint64(len(v)))
	return append(b, v...)
}

func pbPackedW(b []byte, field int, v []uint64) []byte {
	var p []byte
	for _, x := range v {
		p = binary.AppendUvarint(p, x)
	}
	return pbBytesW(b, field, p)
}
