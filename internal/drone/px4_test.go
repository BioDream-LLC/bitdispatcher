package drone_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/drone"
)

// TestRealPX4 flies PX4's own simulator (the real autopilot software), when BD_PX4 names its MAVLink
// address, such as udp:127.0.0.1:14550 for `docker run -p 14550:14550/udp -e HEADLESS=1
// px4io/px4-sitl-gazebo`. It is not run in CI.
func TestRealPX4(t *testing.T) {
	addr := os.Getenv("BD_PX4")
	if addr == "" {
		t.Skip("BD_PX4 not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	l, err := drone.Dial(drone.LinkConfig{Endpoint: addr})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	waitFor(t, "PX4's heartbeat and position", 90*time.Second, func() bool { t := l.Telemetry(); return t.Connected && t.HavePos })
	home := l.Telemetry()
	t.Logf("connected: %+v", home)
	// Arming waits for PX4's preflight checks (GPS, estimator), which take a while after it starts.
	var ferr error
	tLat, tLon := home.Lat+0.0018, home.Lon+0.0012 // about 230 m away
	start := time.Now()
	for time.Since(start) < 90*time.Second {
		if ferr = l.FlyTo(ctx, tLat, tLon, 30); ferr == nil {
			break
		}
		t.Logf("not yet: %v", ferr)
		time.Sleep(5 * time.Second)
	}
	if ferr != nil {
		t.Fatal(ferr)
	}
	t.Logf("took off and climbed in %s; going to the point", time.Since(start).Round(time.Second))
	go2 := time.Now()
	waitFor(t, "PX4 over the point", 120*time.Second, func() bool {
		x := l.Telemetry()
		return drone.Distance(x.Lat, x.Lon, tLat, tLon) < 5
	})
	x := l.Telemetry()
	t.Logf("over the point after %s at %.1f m, battery %d%%", time.Since(go2).Round(time.Second), x.AltRel, x.Battery)
	if err := l.Hold(ctx); err != nil {
		t.Fatal(err)
	}
	if err := l.Return(ctx); err != nil {
		t.Fatal(err)
	}
	back := time.Now()
	waitFor(t, "PX4 home and landed", 150*time.Second, func() bool {
		x := l.Telemetry()
		return !x.Airborne() && !x.Armed && drone.Distance(x.Lat, x.Lon, home.Lat, home.Lon) < 5
	})
	t.Logf("home and disarmed after %s", time.Since(back).Round(time.Second))
}
