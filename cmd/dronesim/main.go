// Command dronesim is a stand-in for a drone, for trying bitdispatcher's drone features without one: a
// PX4-like quadcopter over MAVLink and a camera over RTSP (internal/drone/sim). It is a simple model, not
// a flight simulator; for the real autopilot software use PX4's own simulator (docs/DRONES.md).
//
//	dronesim                         # MAVLink to udp:127.0.0.1:14550, camera at rtsp://127.0.0.1:8654/camera
//	ffmpeg ... -f h264 - | dronesim -video -   # a live picture instead of the built-in one
package main

import (
	"context"
	"flag"
	"io"
	"log"
	"os"
	"os/signal"

	"github.com/biodream-llc/bitdispatcher/internal/drone/sim"
)

func main() {
	mav := flag.String("mavlink", "udp:127.0.0.1:14550", "where to send MAVLink (udp:host:port, udp-listen:host:port, tcp:host:port)")
	rtspAddr := flag.String("rtsp", "127.0.0.1:8654", "address to serve the camera on, as rtsp://ADDRESS/camera")
	video := flag.String("video", "", "H.264 Annex B from this file, or - for standard input, instead of the built-in picture")
	lat := flag.Float64("lat", 37.2707, "home latitude")
	lon := flag.Float64("lon", -79.9414, "home longitude")
	amsl := flag.Float64("amsl", 285, "home ground height above sea level, metres")
	speed := flag.Float64("speed", 15, "cruise speed, metres a second")
	sysid := flag.Int("sysid", 1, "MAVLink system id")
	key := flag.String("signing-key", "", "MAVLink 2 signing key, 64 hexadecimal digits")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var r io.Reader
	switch *video {
	case "":
	case "-":
		r = os.Stdin
	default:
		f, err := os.Open(*video)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		r = f
	}
	if err := sim.Camera(ctx, *rtspAddr, r); err != nil {
		log.Fatalf("dronesim: camera: %v", err)
	}
	m := sim.New(*lat, *lon, *amsl, *speed)
	m.Log = func(f string, a ...any) { log.Printf("dronesim: "+f, a...) }
	log.Printf("dronesim: MAVLink %s, camera rtsp://%s/camera, home %.5f,%.5f", *mav, *rtspAddr, *lat, *lon)
	if err := m.Run(ctx, *mav, *sysid, *key); err != nil {
		log.Fatal(err)
	}
}
