package api

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/drone/sim"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

func freeAddr(t *testing.T, network string) string {
	t.Helper()
	if network == "udp" {
		c, _ := net.ListenPacket("udp", "127.0.0.1:0")
		defer c.Close()
		return c.LocalAddr().String()
	}
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	return l.Addr().String()
}

// A dispatcher asks for a drone; a supervisor (no pilots named) approves; the simulated drone flies to
// the incident with its picture live and recorded; it is sent home; the recording is checked against the
// ledger and refused once changed.
func TestDroneFlow(t *testing.T) {
	ts, s, pw := newServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mav, cam := freeAddr(t, "udp"), freeAddr(t, "tcp")
	if err := sim.Camera(ctx, cam, nil); err != nil {
		t.Fatal(err)
	}
	go sim.New(37.2707, -79.9414, 285, 60).Run(ctx, "udp:"+mav, 1, "")
	s.drones.ctx = nil
	go s.runDrones(ctx)
	admin := login(t, ts, "admin", pw["admin"])
	d := login(t, ts, "d1", pw["d1"])
	conf := DroneConf{ID: "d1", Name: "North drone", MAVLink: "udp-listen:" + mav, Video: "rtsp://pilot:secret@" + cam + "/camera", AltM: 20, Approval: true, Record: true}
	if code, b := d.do("PUT", "/api/admin/drones", map[string]any{"drones": []DroneConf{conf}}, true); code != 403 {
		t.Fatalf("a dispatcher set up a drone: %d %s", code, b)
	}
	bad := conf
	bad.AltM = 150
	if code, _ := admin.do("PUT", "/api/admin/drones", map[string]any{"drones": []DroneConf{bad}}, true); code != 422 {
		t.Fatalf("150 m accepted: %d", code)
	}
	code, b := admin.do("PUT", "/api/admin/drones", map[string]any{"drones": []DroneConf{conf}}, true)
	if code != 200 || strings.Contains(string(b), "secret") || !strings.Contains(string(b), `"id":"D1"`) {
		t.Fatalf("save: %d %s", code, b)
	}
	// Saving again with the masked password keeps it.
	var saved struct{ Drones []DroneConf }
	json.Unmarshal(b, &saved)
	if code, _ := admin.do("PUT", "/api/admin/drones", saved, true); code != 200 || s.droneConfs()[0].Video != "rtsp://pilot:secret@"+cam+"/camera" {
		t.Fatalf("the video password was not kept: %d %q", code, s.droneConfs()[0].Video)
	}
	board := func() (dr cad.Drone) {
		s.Eng.Read(func(st *cad.State) {
			if x := st.Drones["D1"]; x != nil {
				dr = *x
			}
		})
		return
	}
	wait := func(what string, sec int, ok func(cad.Drone) bool) {
		t.Helper()
		end := time.Now().Add(time.Duration(sec) * time.Second)
		for !ok(board()) {
			if time.Now().After(end) {
				t.Fatalf("timed out waiting for %s: %+v", what, board())
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	wait("the drone docked with its picture", 15, func(x cad.Drone) bool { return x.Status == "docked" && x.Live && x.Direct })
	lat, lon := 37.2740, -79.9380
	num, err := s.Eng.CreateIncident(cad.SystemActor, cad.NewIncident{Type: "SF", Location: &ng911.Location{Address: "1 TEST ST", Lat: &lat, Lon: &lon}})
	if err != nil {
		t.Fatal(err)
	}
	if code, b := d.do("POST", "/api/incidents/"+num+"/drone", nil, true); code != 200 || !strings.Contains(string(b), "waiting for its remote pilot") {
		t.Fatalf("ask: %d %s", code, b)
	}
	wait("the launch waiting", 5, func(x cad.Drone) bool { return x.Pending == num })
	if code, _ := d.do("POST", "/api/drones/D1/approve", nil, true); code != 403 {
		t.Fatalf("a dispatcher approved a launch: %d", code)
	}
	if code, b := admin.do("POST", "/api/drones/D1/approve", nil, true); code != 204 {
		t.Fatalf("approve: %d %s", code, b)
	}
	wait("the drone on scene and recording", 40, func(x cad.Drone) bool { return x.Status == "onscene" && x.Incident == num && x.Recording })
	// The live picture: an MP4 header, then fragments.
	req, _ := http.NewRequest("GET", ts.URL+"/api/drones/D1/live", nil)
	resp, err := d.c.Do(req)
	if err != nil || resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("X-Video-Codec"), "avc1.") {
		t.Fatalf("live: %v %v", err, resp)
	}
	head := make([]byte, 4096)
	n, _ := io.ReadAtLeast(resp.Body, head, 4096)
	resp.Body.Close()
	if n < 4096 || string(head[4:8]) != "ftyp" || !strings.Contains(string(head), "moof") {
		t.Fatal("the live picture is not fragmented MP4")
	}
	time.Sleep(11 * time.Second) // a checkpoint
	if code, b := d.do("POST", "/api/drones/D1/return", nil, true); code != 204 {
		t.Fatalf("return: %d %s", code, b)
	}
	wait("the drone home", 40, func(x cad.Drone) bool { return x.Status == "docked" && x.Incident == "" && !x.Recording })
	var clips []cad.DroneClip
	var timeline []string
	s.Eng.Read(func(st *cad.State) {
		clips = st.Incidents[num].DroneClips
		for _, e := range st.Incidents[num].Timeline {
			if e.Kind == "drone" {
				timeline = append(timeline, e.Text)
			}
		}
	})
	if len(clips) != 1 || !clips[0].Final || clips[0].Size < 50000 {
		t.Fatalf("clips: %+v", clips)
	}
	all := strings.Join(timeline, " | ")
	for _, w := range []string{"waiting for its remote pilot", "approved by admin", "video recording started", "sent home by d1", "video recorded"} {
		if !strings.Contains(all, w) {
			t.Fatalf("the timeline lacks %q: %s", w, all)
		}
	}
	get := func(rng string) (int, []byte) {
		req, _ := http.NewRequest("GET", ts.URL+"/api/incidents/"+num+"/drone-video/"+clips[0].ID, nil)
		if rng != "" {
			req.Header.Set("Range", rng)
		}
		resp, err := d.c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b
	}
	if code, b := get(""); code != 200 || int64(len(b)) != clips[0].Size {
		t.Fatalf("clip: %d %d bytes", code, len(b))
	}
	if code, b := get("bytes=1000-1999"); code != 206 || len(b) != 1000 {
		t.Fatalf("clip range: %d %d bytes", code, len(b))
	}
	path := filepath.Join(s.droneVideoDir(num), clips[0].ID+".mp4")
	f, _ := os.OpenFile(path, os.O_RDWR, 0)
	f.WriteAt([]byte{0xff}, 5000)
	f.Close()
	if code, _ := get(""); code != 409 {
		t.Fatalf("a changed recording was served: %d", code)
	}
}
