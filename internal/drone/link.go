// Package drone flies drones over MAVLink and carries their video: the link to a drone's autopilot
// (PX4 or ArduPilot), and its RTSP picture turned into fragmented MP4 for the consoles and for evidence.
//
// bitdispatcher sends a drone to a place and brings it back; it does not replace the remote pilot, who
// keeps the drone's own controller and can take over at any time (docs/DRONES.md).
package drone

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gomavlib/v3"
	"github.com/bluenviron/gomavlib/v3/pkg/dialects/common"
	"github.com/bluenviron/gomavlib/v3/pkg/frame"
	"github.com/bluenviron/gomavlib/v3/pkg/message"
)

// Telemetry is what the autopilot last said.
type Telemetry struct {
	Connected bool      `json:"connected"` // a heartbeat in the last 5 seconds
	Autopilot string    `json:"autopilot,omitempty"`
	Armed     bool      `json:"armed"`
	HavePos   bool      `json:"havePos"`
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	AltAMSL   float64   `json:"altAmsl"` // metres above sea level
	AltRel    float64   `json:"altRel"`  // metres above where it took off
	Heading   float64   `json:"heading"`
	SpeedMS   float64   `json:"speedMs"`
	Battery   int       `json:"battery"` // percent; -1 not known
	Landed    string    `json:"landed"`  // ground, air, takeoff, landing; empty: not known
	Heard     time.Time `json:"heard"`
}

// Airborne is whether the drone is flying.
func (t Telemetry) Airborne() bool {
	switch t.Landed {
	case "air", "takeoff", "landing":
		return true
	case "ground":
		return false
	}
	return t.Armed && t.AltRel > 1
}

// ParseEndpoint reads how to reach an autopilot: udp-listen:0.0.0.0:14550 (wait for it to send, as PX4's
// simulator and most radios do), udp:host:port, tcp:host:port, or serial:/dev/ttyUSB0:57600.
func ParseEndpoint(s string) (gomavlib.EndpointConf, error) {
	kind, rest, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok || rest == "" {
		return nil, errors.New("a MAVLink address is udp-listen:host:port, udp:host:port, tcp:host:port or serial:device:baud")
	}
	switch kind {
	case "udp-listen":
		return gomavlib.EndpointUDPServer{Address: rest}, nil
	case "udp":
		return gomavlib.EndpointUDPClient{Address: rest}, nil
	case "tcp":
		return gomavlib.EndpointTCPClient{Address: rest}, nil
	case "serial":
		i := strings.LastIndex(rest, ":")
		if i < 0 {
			return nil, errors.New("a serial MAVLink address is serial:device:baud")
		}
		baud, err := strconv.Atoi(rest[i+1:])
		if err != nil || baud <= 0 {
			return nil, errors.New("the serial baud rate is a number, such as 57600")
		}
		return gomavlib.EndpointSerial{Device: rest[:i], Baud: baud}, nil
	}
	return nil, fmt.Errorf("unknown MAVLink address kind %q", kind)
}

// ParseKey reads a MAVLink 2 signing key: 64 hexadecimal digits (32 bytes); empty for none.
func ParseKey(s string) (*frame.V2Key, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		return nil, errors.New("a MAVLink signing key is 64 hexadecimal digits")
	}
	return frame.NewV2Key(b), nil
}

// LinkConfig is how to reach one drone.
type LinkConfig struct {
	Endpoint   string
	SigningKey string // MAVLink 2 signing (both ways) when set
	SystemID   int    // the drone's system id; 0 takes the first autopilot heard
	OurID      int    // our system id; 0 is 250
}

// Link is a connection to one drone's autopilot.
type Link struct {
	node *gomavlib.Node

	mu     sync.Mutex
	tel    Telemetry
	sys    byte
	ch     *gomavlib.Channel
	acks   map[common.MAV_CMD]chan common.MAV_RESULT
	closed bool
	want   int
}

// Dial opens the link. It returns at once; Telemetry says when the drone is heard.
func Dial(cfg LinkConfig) (*Link, error) {
	ep, err := ParseEndpoint(cfg.Endpoint)
	if err != nil {
		return nil, err
	}
	key, err := ParseKey(cfg.SigningKey)
	if err != nil {
		return nil, err
	}
	our := cfg.OurID
	if our <= 0 || our > 255 {
		our = 250
	}
	n := &gomavlib.Node{
		Endpoints:              []gomavlib.EndpointConf{ep},
		Dialect:                common.Dialect,
		OutVersion:             gomavlib.V2,
		OutSystemID:            byte(our),
		OutComponentID:         190, // MAV_COMP_ID_MISSIONPLANNER: a ground station
		HeartbeatSystemType:    int(common.MAV_TYPE_GCS),
		HeartbeatAutopilotType: int(common.MAV_AUTOPILOT_INVALID),
		StreamRequestEnable:    true, // ArduPilot sends telemetry when asked; PX4 sends it anyway
		StreamRequestFrequency: 4,
		InKey:                  key,
		OutKey:                 key,
	}
	if err := n.Initialize(); err != nil {
		return nil, err
	}
	l := &Link{node: n, acks: map[common.MAV_CMD]chan common.MAV_RESULT{}, want: cfg.SystemID}
	l.tel.Battery = -1
	go l.read()
	return l, nil
}

// Close ends the link.
func (l *Link) Close() {
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()
	l.node.Close()
}

// Telemetry is the drone's last known state.
func (l *Link) Telemetry() Telemetry {
	l.mu.Lock()
	defer l.mu.Unlock()
	t := l.tel
	t.Connected = !t.Heard.IsZero() && time.Since(t.Heard) < 5*time.Second
	return t
}

func (l *Link) read() {
	for ev := range l.node.Events() {
		fr, ok := ev.(*gomavlib.EventFrame)
		if !ok {
			continue
		}
		l.mu.Lock()
		if l.sys == 0 {
			if hb, ok := fr.Message().(*common.MessageHeartbeat); ok && hb.Type != common.MAV_TYPE_GCS && hb.Autopilot != common.MAV_AUTOPILOT_INVALID &&
				(l.want == 0 || int(fr.SystemID()) == l.want) {
				l.sys, l.ch = fr.SystemID(), fr.Channel
			}
		}
		if fr.SystemID() != l.sys || l.sys == 0 {
			l.mu.Unlock()
			continue
		}
		l.ch = fr.Channel
		t := &l.tel
		switch m := fr.Message().(type) {
		case *common.MessageHeartbeat:
			if fr.ComponentID() == 1 { // the autopilot, not a camera or gimbal on the same system
				t.Heard = time.Now()
				t.Armed = m.BaseMode&common.MAV_MODE_FLAG_SAFETY_ARMED != 0
				t.Autopilot = map[common.MAV_AUTOPILOT]string{common.MAV_AUTOPILOT_PX4: "PX4", common.MAV_AUTOPILOT_ARDUPILOTMEGA: "ArduPilot"}[m.Autopilot]
			}
		case *common.MessageGlobalPositionInt:
			t.HavePos = m.Lat != 0 || m.Lon != 0
			t.Lat, t.Lon = float64(m.Lat)/1e7, float64(m.Lon)/1e7
			t.AltAMSL, t.AltRel = float64(m.Alt)/1000, float64(m.RelativeAlt)/1000
			if m.Hdg != math.MaxUint16 {
				t.Heading = float64(m.Hdg) / 100
			}
			t.SpeedMS = math.Hypot(float64(m.Vx), float64(m.Vy)) / 100
		case *common.MessageSysStatus:
			if m.BatteryRemaining >= 0 {
				t.Battery = int(m.BatteryRemaining)
			}
		case *common.MessageBatteryStatus:
			if m.BatteryRemaining >= 0 && m.Id == 0 {
				t.Battery = int(m.BatteryRemaining)
			}
		case *common.MessageExtendedSysState:
			t.Landed = map[common.MAV_LANDED_STATE]string{common.MAV_LANDED_STATE_ON_GROUND: "ground", common.MAV_LANDED_STATE_IN_AIR: "air",
				common.MAV_LANDED_STATE_TAKEOFF: "takeoff", common.MAV_LANDED_STATE_LANDING: "landing"}[m.LandedState]
		case *common.MessageCommandAck:
			if c, ok := l.acks[m.Command]; ok && m.Result != common.MAV_RESULT_IN_PROGRESS {
				select {
				case c <- m.Result:
				default:
				}
			}
		}
		l.mu.Unlock()
	}
}

// ErrNoDrone is returned when the drone has not been heard.
var ErrNoDrone = errors.New("the drone is not connected")

func (l *Link) target() (byte, *gomavlib.Channel, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.sys == 0 || time.Since(l.tel.Heard) > 5*time.Second {
		return 0, nil, ErrNoDrone
	}
	return l.sys, l.ch, nil
}

// command sends a command and waits for the autopilot to accept it, sending it again up to three times
// (the MAVLink command protocol).
func (l *Link) command(ctx context.Context, cmd common.MAV_CMD, build func(sys byte, attempt int) message.Message) error {
	sys, ch, err := l.target()
	if err != nil {
		return err
	}
	res := make(chan common.MAV_RESULT, 1)
	l.mu.Lock()
	l.acks[cmd] = res
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		delete(l.acks, cmd)
		l.mu.Unlock()
	}()
	for attempt := 0; attempt < 3; attempt++ {
		if err := l.node.WriteMessageTo(ch, build(sys, attempt)); err != nil {
			return err
		}
		select {
		case r := <-res:
			if r == common.MAV_RESULT_ACCEPTED {
				return nil
			}
			return fmt.Errorf("the drone refused %s: %s", cmdName(cmd), strings.ToLower(strings.TrimPrefix(r.String(), "MAV_RESULT_")))
		case <-time.After(1500 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return fmt.Errorf("the drone did not answer %s", cmdName(cmd))
}

func cmdName(c common.MAV_CMD) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(c.String(), "MAV_CMD_"), "_", " "))
}

func (l *Link) long(ctx context.Context, cmd common.MAV_CMD, p ...float32) error {
	var a [7]float32
	copy(a[:], p)
	return l.command(ctx, cmd, func(sys byte, attempt int) message.Message {
		return &common.MessageCommandLong{TargetSystem: sys, TargetComponent: 1, Command: cmd, Confirmation: uint8(attempt),
			Param1: a[0], Param2: a[1], Param3: a[2], Param4: a[3], Param5: a[4], Param6: a[5], Param7: a[6]}
	})
}

var nan = float32(math.NaN())

// FlyTo sends the drone to a place at a height above where it took off: from the ground it arms, takes
// off and climbs first. PX4 and ArduPilot are told in the way each understands.
func (l *Link) FlyTo(ctx context.Context, lat, lon, altRel float64) error {
	t := l.Telemetry()
	if !t.Connected {
		return ErrNoDrone
	}
	if !t.HavePos {
		return errors.New("the drone has no position fix yet")
	}
	ardu := t.Autopilot == "ArduPilot"
	if !t.Airborne() {
		if ardu {
			// GUIDED (copter mode 4) takes commands from a ground station.
			if err := l.long(ctx, common.MAV_CMD_DO_SET_MODE, float32(common.MAV_MODE_FLAG_CUSTOM_MODE_ENABLED), 4); err != nil {
				return err
			}
		}
		if err := l.long(ctx, common.MAV_CMD_COMPONENT_ARM_DISARM, 1); err != nil {
			return err
		}
		// PX4 takes the take-off height above sea level; ArduPilot above the take-off point.
		alt := float32(t.AltAMSL - t.AltRel + altRel)
		if ardu {
			alt = float32(altRel)
		}
		if err := l.long(ctx, common.MAV_CMD_NAV_TAKEOFF, -1, 0, 0, nan, nan, nan, alt); err != nil {
			return err
		}
		// Climb before heading off, so it does not fly low over the roofs.
		deadline := time.Now().Add(90 * time.Second)
		for l.Telemetry().AltRel < altRel*0.8 {
			if time.Now().After(deadline) {
				return errors.New("the drone did not climb to its height within 90 seconds")
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(250 * time.Millisecond):
			}
		}
	}
	return l.reposition(ctx, lat, lon, altRel)
}

// reposition is "go to this place and hold there" (MAV_CMD_DO_REPOSITION, as a ground station's
// "go here" does), switching the autopilot into the mode that does it.
func (l *Link) reposition(ctx context.Context, lat, lon, altRel float64) error {
	t := l.Telemetry()
	frameKind, z := common.MAV_FRAME_GLOBAL_INT, float32(t.AltAMSL-t.AltRel+altRel)
	if t.Autopilot == "ArduPilot" {
		frameKind, z = common.MAV_FRAME_GLOBAL_RELATIVE_ALT_INT, float32(altRel)
	}
	return l.command(ctx, common.MAV_CMD_DO_REPOSITION, func(sys byte, _ int) message.Message {
		return &common.MessageCommandInt{TargetSystem: sys, TargetComponent: 1, Frame: frameKind, Command: common.MAV_CMD_DO_REPOSITION,
			Param1: -1, Param2: float32(common.MAV_DO_REPOSITION_FLAGS_CHANGE_MODE), Param4: nan,
			X: int32(math.Round(lat * 1e7)), Y: int32(math.Round(lon * 1e7)), Z: z}
	})
}

// Hold stops the drone where it is.
func (l *Link) Hold(ctx context.Context) error {
	t := l.Telemetry()
	if !t.Connected {
		return ErrNoDrone
	}
	return l.reposition(ctx, t.Lat, t.Lon, t.AltRel)
}

// Return brings the drone home: the autopilot's own return-to-launch, which climbs to its set safe
// height, flies back and lands.
func (l *Link) Return(ctx context.Context) error {
	return l.long(ctx, common.MAV_CMD_NAV_RETURN_TO_LAUNCH)
}

// Distance is the distance in metres between two points.
func Distance(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371000.0
	p1, p2 := lat1*math.Pi/180, lat2*math.Pi/180
	dp, dl := p2-p1, (lon2-lon1)*math.Pi/180
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}
