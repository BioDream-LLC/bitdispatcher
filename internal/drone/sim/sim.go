// Package sim is a stand-in for a drone, for trying and testing bitdispatcher's drone features without
// one. It speaks MAVLink like a PX4 quadcopter (heartbeat, position, battery, landed state; arm, take off,
// go to, return) and serves its camera over RTSP: a built-in test picture, or H.264 from a reader such as
// ffmpeg's output. It is a simple model, not a flight simulator: for the real autopilot software use
// PX4's own simulator (docs/DRONES.md).
package sim

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"io"
	"math"
	"sync"
	"time"

	"github.com/bluenviron/gomavlib/v3"
	"github.com/bluenviron/gomavlib/v3/pkg/dialects/common"
	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/pion/rtp"

	"github.com/biodream-llc/bitdispatcher/internal/drone"
)

// clip is ten seconds of a synthetic test pattern, 640×360 at 15 frames a second, made with ffmpeg's
// testsrc2 (H.264 constrained baseline, a key frame with its parameter sets each second).
//
//go:embed clip.h264
var clip []byte

// Model is the simulated aircraft.
type Model struct {
	mu                       sync.Mutex
	lat, lon, alt            float64 // alt: above home
	homeLat, homeLon, ground float64
	speed                    float64
	armed                    bool
	mode                     string // ground, takeoff, hold, goto, rtl, land
	tLat, tLon, tAlt         float64
	battery                  float64
	heading, vel             float64
	// Log, when set, is told what the drone does.
	Log func(format string, args ...any)
}

// New makes a drone on the ground at home, with ground the height of home above sea level and speed its
// cruise speed in metres a second.
func New(lat, lon, ground, speed float64) *Model {
	return &Model{lat: lat, lon: lon, homeLat: lat, homeLon: lon, ground: ground, speed: speed, battery: 100, mode: "ground", Log: func(string, ...any) {}}
}

// State is where the simulated drone is and what it is doing.
func (m *Model) State() (lat, lon, alt float64, mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lat, m.lon, m.alt, m.mode
}

// Run flies the model, talking MAVLink on endpoint, until ctx ends.
func (m *Model) Run(ctx context.Context, endpoint string, sysid int, keyHex string) error {
	ep, err := drone.ParseEndpoint(endpoint)
	if err != nil {
		return err
	}
	k, err := drone.ParseKey(keyHex)
	if err != nil {
		return err
	}
	n := &gomavlib.Node{Endpoints: []gomavlib.EndpointConf{ep}, Dialect: common.Dialect, OutVersion: gomavlib.V2, OutSystemID: byte(sysid),
		OutComponentID: 1, HeartbeatDisable: true, InKey: k, OutKey: k}
	if err := n.Initialize(); err != nil {
		return err
	}
	defer n.Close()
	go func() {
		for ev := range n.Events() {
			if fr, ok := ev.(*gomavlib.EventFrame); ok {
				m.handle(n, fr)
			}
		}
	}()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
		m.step(0.1)
		if i%2 == 0 {
			m.send(n, i%10 == 0)
		}
	}
}

func (m *Model) step(dt float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	climb := func(to float64) bool {
		d := to - m.alt
		if math.Abs(d) <= 4*dt {
			m.alt = to
			return true
		}
		m.alt += math.Copysign(4*dt, d)
		return false
	}
	fly := func(lat, lon float64) bool {
		d := drone.Distance(m.lat, m.lon, lat, lon)
		if d < 0.5 {
			m.vel = 0
			return true
		}
		step := math.Min(d, m.speed*dt)
		m.heading = bearing(m.lat, m.lon, lat, lon)
		f := step / d
		m.lat += (lat - m.lat) * f
		m.lon += (lon - m.lon) * f
		m.vel = step / dt
		return false
	}
	switch m.mode {
	case "takeoff":
		if climb(m.tAlt) {
			m.mode = "hold"
		}
	case "goto":
		if climb(m.tAlt) || m.alt > 5 {
			if fly(m.tLat, m.tLon) {
				m.mode = "hold"
			}
		}
	case "rtl":
		if m.alt < 30 && drone.Distance(m.lat, m.lon, m.homeLat, m.homeLon) > 1 {
			climb(30)
		} else if fly(m.homeLat, m.homeLon) {
			m.mode = "land"
		}
	case "land":
		if climb(0) {
			m.mode, m.armed = "ground", false
		}
	}
	if m.armed {
		m.battery = math.Max(0, m.battery-0.03*dt*10) // about 55 minutes of flight from full
	}
}

func bearing(lat1, lon1, lat2, lon2 float64) float64 {
	p1, p2, dl := lat1*math.Pi/180, lat2*math.Pi/180, (lon2-lon1)*math.Pi/180
	b := math.Atan2(math.Sin(dl)*math.Cos(p2), math.Cos(p1)*math.Sin(p2)-math.Sin(p1)*math.Cos(p2)*math.Cos(dl)) * 180 / math.Pi
	return math.Mod(b+360, 360)
}

func (m *Model) send(n *gomavlib.Node, slow bool) {
	m.mu.Lock()
	base := common.MAV_MODE_FLAG_CUSTOM_MODE_ENABLED
	if m.armed {
		base |= common.MAV_MODE_FLAG_SAFETY_ARMED
	}
	landed := common.MAV_LANDED_STATE_ON_GROUND
	switch {
	case m.mode == "takeoff":
		landed = common.MAV_LANDED_STATE_TAKEOFF
	case m.mode == "land" && m.alt > 0:
		landed = common.MAV_LANDED_STATE_LANDING
	case m.alt > 0.3:
		landed = common.MAV_LANDED_STATE_IN_AIR
	}
	pos := &common.MessageGlobalPositionInt{TimeBootMs: uint32(time.Now().UnixMilli()), Lat: int32(m.lat * 1e7), Lon: int32(m.lon * 1e7),
		Alt: int32((m.ground + m.alt) * 1000), RelativeAlt: int32(m.alt * 1000), Hdg: uint16(m.heading * 100),
		Vx: int16(m.vel * 100 * math.Cos(m.heading*math.Pi/180)), Vy: int16(m.vel * 100 * math.Sin(m.heading*math.Pi/180))}
	batt := int8(m.battery)
	m.mu.Unlock()
	n.WriteMessageAll(pos)
	if slow {
		n.WriteMessageAll(&common.MessageHeartbeat{Type: common.MAV_TYPE_QUADROTOR, Autopilot: common.MAV_AUTOPILOT_PX4, BaseMode: base,
			SystemStatus: common.MAV_STATE_ACTIVE, MavlinkVersion: 3})
		n.WriteMessageAll(&common.MessageSysStatus{BatteryRemaining: batt, VoltageBattery: 16000})
		n.WriteMessageAll(&common.MessageExtendedSysState{LandedState: landed})
	}
}

func (m *Model) handle(n *gomavlib.Node, fr *gomavlib.EventFrame) {
	ack := func(c common.MAV_CMD, r common.MAV_RESULT) {
		n.WriteMessageTo(fr.Channel, &common.MessageCommandAck{Command: c, Result: r})
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	switch c := fr.Message().(type) {
	case *common.MessageCommandLong:
		switch c.Command {
		case common.MAV_CMD_COMPONENT_ARM_DISARM:
			if c.Param1 == 1 {
				m.armed = true
			} else if m.alt < 0.3 {
				m.armed = false
			}
			ack(c.Command, common.MAV_RESULT_ACCEPTED)
		case common.MAV_CMD_NAV_TAKEOFF:
			if !m.armed {
				ack(c.Command, common.MAV_RESULT_DENIED)
				return
			}
			m.tAlt = float64(c.Param7) - m.ground // PX4: above sea level
			if math.IsNaN(m.tAlt) || m.tAlt < 2 {
				m.tAlt = 30
			}
			m.mode = "takeoff"
			m.Log("taking off to %.0f m", m.tAlt)
			ack(c.Command, common.MAV_RESULT_ACCEPTED)
		case common.MAV_CMD_NAV_RETURN_TO_LAUNCH:
			if m.alt < 0.3 {
				ack(c.Command, common.MAV_RESULT_DENIED)
				return
			}
			m.mode = "rtl"
			m.Log("returning home")
			ack(c.Command, common.MAV_RESULT_ACCEPTED)
		case common.MAV_CMD_DO_SET_MODE:
			ack(c.Command, common.MAV_RESULT_ACCEPTED)
		default:
			ack(c.Command, common.MAV_RESULT_UNSUPPORTED)
		}
	case *common.MessageCommandInt:
		if c.Command != common.MAV_CMD_DO_REPOSITION {
			ack(c.Command, common.MAV_RESULT_UNSUPPORTED)
			return
		}
		if m.alt < 1 {
			ack(c.Command, common.MAV_RESULT_DENIED) // as PX4: on the ground it needs a take-off first
			return
		}
		m.tLat, m.tLon = float64(c.X)/1e7, float64(c.Y)/1e7
		m.tAlt = float64(c.Z) - m.ground
		if c.Frame == common.MAV_FRAME_GLOBAL_RELATIVE_ALT_INT {
			m.tAlt = float64(c.Z)
		}
		m.mode = "goto"
		m.Log("going to %.5f,%.5f at %.0f m (%.0f m away)", m.tLat, m.tLon, m.tAlt, drone.Distance(m.lat, m.lon, m.tLat, m.tLon))
		ack(c.Command, common.MAV_RESULT_ACCEPTED)
	}
}

// ---- the camera ----

type rtspHandler struct{ stream *gortsplib.ServerStream }

func (h *rtspHandler) OnDescribe(*gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
	return &base.Response{StatusCode: base.StatusOK}, h.stream, nil
}

func (h *rtspHandler) OnSetup(*gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	return &base.Response{StatusCode: base.StatusOK}, h.stream, nil
}

func (h *rtspHandler) OnPlay(*gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	return &base.Response{StatusCode: base.StatusOK}, nil
}

// Camera serves video as rtsp://addr/camera until ctx ends: the built-in picture when r is nil, else
// H.264 Annex B read from r as it comes. It returns once the server is listening.
func Camera(ctx context.Context, addr string, r io.Reader) error {
	h := &rtspHandler{}
	srv := &gortsplib.Server{Handler: h, RTSPAddress: addr} // TCP only
	forma := &format.H264{PayloadTyp: 96, PacketizationMode: 1}
	desc := &description.Session{Medias: []*description.Media{{Type: description.MediaTypeVideo, Formats: []format.Format{forma}}}}
	h.stream = &gortsplib.ServerStream{Server: srv, Desc: desc}
	if err := srv.Start(); err != nil {
		return err
	}
	if err := h.stream.Initialize(); err != nil {
		srv.Close()
		return err
	}
	enc, err := forma.CreateEncoder()
	if err != nil {
		srv.Close()
		return err
	}
	go func() {
		defer srv.Close()
		defer h.stream.Close()
		feed(ctx, h, enc, desc, r)
	}()
	return nil
}

func feed(ctx context.Context, h *rtspHandler, enc interface {
	Encode([][]byte) ([]*rtp.Packet, error)
}, desc *description.Session, r io.Reader) {
	medi := desc.Medias[0]
	write := func(au [][]byte, pts int64) {
		pkts, err := enc.Encode(au)
		if err != nil {
			return
		}
		for _, p := range pkts {
			p.Timestamp = uint32(pts)
			h.stream.WritePacketRTP(medi, p)
		}
	}
	if r == nil {
		aus := splitAUs(clip)
		const fps = 15
		t := time.NewTicker(time.Second / fps)
		defer t.Stop()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			write(aus[i%len(aus)], int64(i)*90000/fps)
		}
	}
	start := time.Now()
	readAUs(r, func(au [][]byte) {
		write(au, int64(time.Since(start)*90000/time.Second))
	})
}

// splitAUs splits H.264 Annex B into access units at access unit delimiters.
func splitAUs(b []byte) [][][]byte {
	var out [][][]byte
	readAUs(bytes.NewReader(b), func(au [][]byte) { out = append(out, au) })
	return out
}

// readAUs reads H.264 Annex B and calls f with each access unit as it completes.
func readAUs(r io.Reader, f func([][]byte)) {
	br := bufio.NewReaderSize(r, 1<<20)
	var cur [][]byte
	var nal []byte
	zeros := 0
	flushNAL := func() {
		for len(nal) > 0 && nal[len(nal)-1] == 0 {
			nal = nal[:len(nal)-1]
		}
		if len(nal) == 0 {
			return
		}
		if h264.NALUType(nal[0]&0x1f) == h264.NALUTypeAccessUnitDelimiter && len(cur) > 0 {
			f(cur)
			cur = nil
		}
		cur = append(cur, nal)
		nal = nil
	}
	for {
		c, err := br.ReadByte()
		if err != nil {
			break
		}
		if c == 1 && zeros >= 2 {
			nal = nal[:len(nal)-zeros]
			flushNAL()
			zeros = 0
			continue
		}
		if c == 0 {
			zeros++
		} else {
			zeros = 0
		}
		nal = append(nal, c)
	}
	flushNAL()
	if len(cur) > 0 {
		f(cur)
	}
}
