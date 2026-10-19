package overture

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/biodream-llc/bitdispatcher/internal/mapdata"
)

func str(s string) *string { return &s }

// A release of two files on a fake bucket: one far away (its row group is never fetched), one with
// two addresses near the area's centre and one outside it.
func TestRead(t *testing.T) {
	mk := func(rows []row) []byte {
		var b bytes.Buffer
		w := parquet.NewGenericWriter[row](&b, parquet.MaxRowsPerRowGroup(2))
		if _, err := w.Write(rows); err != nil {
			t.Fatal(err)
		}
		w.Close()
		return b.Bytes()
	}
	at := func(lat, lon float32, num, street string) row {
		r := row{Number: str(num), Street: str(street), Postcode: str("24016-1234"), PostalCity: str("Roanoke")}
		r.BBox.XMin, r.BBox.XMax, r.BBox.YMin, r.BBox.YMax = lon, lon, lat, lat
		return r
	}
	files := map[string][]byte{
		"release/2026-09-23.1/theme=addresses/type=address/part-0.parquet": mk([]row{at(48.85, 2.35, "1", "Rue de Rivoli"), at(48.86, 2.35, "2", "Rue de Rivoli")}),
		"release/2026-09-23.1/theme=addresses/type=address/part-1.parquet": mk([]row{at(37.2710, -79.9414, "210", "Church Avenue Southwest"), at(37.2711, -79.9415, "212", "Church Avenue Southwest"), at(38.0, -78.0, "9", "Far Rd")}),
	}
	ranges := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			p := r.URL.Query().Get("prefix")
			fmt.Fprint(w, `<ListBucketResult>`)
			if r.URL.Query().Get("delimiter") == "/" {
				fmt.Fprint(w, `<CommonPrefixes><Prefix>release/2026-08-19.0/</Prefix></CommonPrefixes><CommonPrefixes><Prefix>release/2026-09-23.1/</Prefix></CommonPrefixes>`)
			}
			for k, b := range files {
				if strings.HasPrefix(k, p) {
					fmt.Fprintf(w, `<Contents><Key>%s</Key><Size>%d</Size></Contents>`, k, len(b))
				}
			}
			fmt.Fprint(w, `<IsTruncated>false</IsTruncated></ListBucketResult>`)
			return
		}
		b, ok := files[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		var from, to int
		fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &from, &to)
		ranges[r.URL.Path]++
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, to, len(b)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(b[from : to+1])
	}))
	defer srv.Close()
	var got []mapdata.GISFeature
	var last Progress
	err := Client{Base: srv.URL}.Read(context.Background(), "", mapdata.Area{Lat: 37.271, Lon: -79.9414, RadiusM: 2000}, func(p Progress) { last = p }, func(f mapdata.GISFeature) { got = append(got, f) })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Num != "210" || got[0].Zip != "24016" || got[0].City != "Roanoke" || got[0].Src != "overture" || got[0].Kind != mapdata.GISAddress {
		t.Fatalf("got %+v", got)
	}
	if last.Release != "2026-09-23.1" || last.FilesDone != 2 || last.Groups != 1 {
		t.Fatalf("progress %+v (the row groups far away are skipped by their statistics)", last)
	}
}
