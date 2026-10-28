package drone_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/drone"
	"github.com/biodream-llc/bitdispatcher/internal/drone/sim"
)

func freePort(t *testing.T, network string) string {
	t.Helper()
	if network == "udp" {
		c, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		return c.LocalAddr().String()
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

func waitFor(t *testing.T, what string, d time.Duration, ok func() bool) {
	t.Helper()
	end := time.Now().Add(d)
	for !ok() {
		if time.Now().After(end) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// A simulated PX4 drone is flown out and back over MAVLink, with MAVLink 2 signing.
func TestFlyOutAndBack(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr := freePort(t, "udp")
	key := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	l, err := drone.Dial(drone.LinkConfig{Endpoint: "udp-listen:" + addr, SigningKey: key})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	m := sim.New(37.2707, -79.9414, 285, 60) // fast, for the test
	go m.Run(ctx, "udp:"+addr, 1, key)
	waitFor(t, "the drone's heartbeat and position", 5*time.Second, func() bool { t := l.Telemetry(); return t.Connected && t.HavePos })
	if tel := l.Telemetry(); tel.Autopilot != "PX4" || tel.Airborne() || tel.Battery != 100 {
		t.Fatalf("telemetry on the ground: %+v", tel)
	}
	tLat, tLon := 37.2740, -79.9380 // about 470 m away
	if err := l.FlyTo(ctx, tLat, tLon, 40); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the drone over the place", 30*time.Second, func() bool {
		tel := l.Telemetry()
		return drone.Distance(tel.Lat, tel.Lon, tLat, tLon) < 5 && tel.AltRel > 39
	})
	if err := l.Return(ctx); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the drone home and landed", 40*time.Second, func() bool {
		tel := l.Telemetry()
		return !tel.Airborne() && !tel.Armed && drone.Distance(tel.Lat, tel.Lon, 37.2707, -79.9414) < 2
	})
}

// A drone with a different signing key is not heard.
func TestWrongKeyNotHeard(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr := freePort(t, "udp")
	l, err := drone.Dial(drone.LinkConfig{Endpoint: "udp-listen:" + addr, SigningKey: "aa00000000000000000000000000000000000000000000000000000000000000"})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go sim.New(37.27, -79.94, 285, 15).Run(ctx, "udp:"+addr, 1, "bb00000000000000000000000000000000000000000000000000000000000000")
	time.Sleep(2 * time.Second)
	if l.Telemetry().Connected {
		t.Fatal("a drone signing with another key was accepted")
	}
}

// The camera's picture comes in over RTSP, is published as MP4 fragments, and is recorded into a file
// whose checkpoints hash its bytes; the file is a valid video.
func TestVideoLiveAndRecorded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr := freePort(t, "tcp")
	if err := sim.Camera(ctx, addr, nil); err != nil {
		t.Fatal(err)
	}
	h := drone.NewHub()
	go drone.ReadRTSP(ctx, "rtsp://"+addr+"/camera", h)
	waitFor(t, "live video", 10*time.Second, h.Live)
	st, gop, ch, stop := h.Subscribe()
	if st == nil || !strings.HasPrefix(st.Codec, "avc1.42") || len(st.Init) == 0 {
		t.Fatalf("stream codec %q", st.Codec)
	}
	if len(gop) > 0 && !gop[0].Key {
		t.Fatal("a new viewer does not start at a key frame")
	}
	path := filepath.Join(t.TempDir(), "clip.mp4")
	rec, err := drone.NewRecorder(path, st)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range gop {
		rec.Add(f)
	}
	n, end := 0, time.After(4*time.Second)
	var mid drone.Checkpoint
loop:
	for {
		select {
		case f, ok := <-ch:
			if !ok {
				t.Fatal("the stream ended")
			}
			rec.Add(f)
			if n++; n == 20 {
				mid, _ = rec.Checkpoint()
			}
		case <-end:
			break loop
		}
	}
	stop()
	fin, err := rec.Close()
	if err != nil {
		t.Fatal(err)
	}
	if n < 45 || n > 75 { // 15 frames a second
		t.Fatalf("%d frames in 4 seconds", n)
	}
	b, _ := os.ReadFile(path)
	sum := sha256.Sum256(b)
	if int64(len(b)) != fin.Size || hex.EncodeToString(sum[:]) != fin.SHA256 {
		t.Fatal("the final checkpoint does not match the file")
	}
	prefix := sha256.Sum256(b[:mid.Size])
	if hex.EncodeToString(prefix[:]) != mid.SHA256 {
		t.Fatal("a checkpoint does not match the start of the file")
	}
	if _, err := drone.NewRecorder(path, st); err == nil {
		t.Fatal("a recording was overwritten")
	}
	if ff, err := exec.LookPath("ffprobe"); err == nil {
		out, err := exec.Command(ff, "-v", "error", "-count_frames", "-show_entries", "stream=codec_name,width,height,nb_read_frames", "-of", "csv=p=0", path).CombinedOutput()
		if err != nil || !bytes.HasPrefix(out, []byte("h264,640,360,")) {
			t.Fatalf("ffprobe: %v %s", err, out)
		}
		t.Logf("recorded clip: %s", bytes.TrimSpace(out))
	}
}
