package overture

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/mapdata"
)

// TestLive reads downtown Roanoke from the real bucket: BD_LIVE=1 go test -run Live ./internal/overture
func TestLive(t *testing.T) {
	if os.Getenv("BD_LIVE") == "" {
		t.Skip("set BD_LIVE=1 to read Overture's bucket")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	n := 0
	var last Progress
	start := time.Now()
	err := Client{Agent: "bitdispatcher-test"}.Read(ctx, "", mapdata.Area{Lat: 37.271, Lon: -79.9414, RadiusM: 1000}, func(p Progress) { last = p }, func(f mapdata.GISFeature) {
		if n < 3 {
			t.Logf("%+v %v", f, *f.Point)
		}
		n++
	})
	t.Logf("%d addresses, %+v, %s", n, last, time.Since(start))
	if err != nil || n == 0 {
		t.Fatal(err, n)
	}
}
