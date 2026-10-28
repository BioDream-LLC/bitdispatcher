# Drones as first responders

bitdispatcher can send a drone to an incident, show its live video in the
incident panel, and record that video as evidence. It talks to the drone's
autopilot over MAVLink (PX4 or ArduPilot) and reads the camera over RTSP.

Status: step 1. Tested only against simulators. It has not flown a real drone.

## How it works

1. A dispatcher presses "Send a drone" on an incident, or a launch rule asks
   for one. bitdispatcher picks the nearest ready drone (connected, landed,
   battery at or above the minimum).
2. If the drone needs approval, the launch waits for a pilot. Listed pilots
   may approve; with no pilots listed, supervisors approve. A launch rule
   never flies a drone by itself: it always waits for a person.
3. bitdispatcher arms the drone, takes off to the set height, and sends it to
   the incident (MAV_CMD_DO_REPOSITION). It holds there.
4. The live picture plays in the incident panel. Dispatchers can press Hold
   or Return home. The pilot's own controller can take over at any time.
5. While the drone is on an incident, its video is recorded.

Training boards never fly real drones.

## Setup

Admin > Drones. Each drone has:

- MAVLink link: `udp-listen:0.0.0.0:14550`, `udp:host:port`, `tcp:host:port`
  or `serial:/dev/ttyUSB0:57600`
- MAVLink 2 signing key (64 hex digits). Strongly recommended.
- System id of the autopilot (0 = first one heard)
- Camera RTSP URL, for example `rtsp://user:pass@10.0.0.20:8554/camera`
  (H.264)
- Flying height, 10 to 120 m (default 60)
- Minimum battery for launch, 10 to 95 % (default 40)
- Needs pilot approval, the pilot list, record video, and off

### Try it with the simulator

`make sim` builds `bin/dronesim`, a simple flight model that answers MAVLink
like PX4 and serves a test camera. It is not PX4 and does not model real
flight physics.

    bin/dronesim -mavlink udp:127.0.0.1:14550 -rtsp 127.0.0.1:8654 \
      -lat 40.0 -lon -75.0

Then in Admin > Drones use link `udp-listen:127.0.0.1:14550` and camera
`rtsp://127.0.0.1:8654/camera`. `-video -` reads H.264 from stdin, so ffmpeg
can feed it any picture.

### PX4's simulator

`internal/drone/px4_test.go` flies PX4 SITL when `BD_PX4` names its MAVLink
endpoint. A first try with the `px4io/px4-sitl-gazebo` Docker image did not
connect: that image keeps MAVLink on localhost inside the container. Running
PX4 SITL built from source is the next step.

## Evidence

- Recordings go to `<data>/drone-video/<incident>/<UTC>-<drone>.mp4`. Files
  are opened with O_EXCL and are never overwritten. A new file starts every
  30 minutes.
- Every 10 s the file is synced and its size and SHA-256 so far are written to
  the ledger. The final hash is written when recording stops. The incident
  timeline shows each recording.
- When a recording is served, its SHA-256 is checked against the ledger. A
  changed file gets 409 and is not served.

## Security

- Turn on MAVLink 2 signing. Without it, anyone who can reach the link can
  command the drone. A drone signing with a different key is ignored.
- Keep the MAVLink and camera links on a private network or a VPN. Never put
  them on the internet.
- The signing key and camera password are masked in Admin and never logged.
- Video and recordings can contain CJI and PII. Field crews see only their own
  incident's video. Set retention to your agency's policy and law.

## Law (United States)

This is not legal advice.

- FAA Part 107 applies, with a certificated remote pilot, and Remote ID.
  Flying beyond visual line of sight needs a waiver. The proposed Part 108 rule
  is not final.
- DJI, Autel and other foreign drones are on the FCC Covered List. Use a
  drone that is allowed, such as a US or allied PX4 or ArduPilot airframe.
- Many states limit police drone use and how long video may be kept. Check
  yours.
- The remote pilot stays in command. bitdispatcher asks; the pilot decides.

## What was tested

- Go tests: flight out and back, wrong signing key ignored, live video and
  recording (checked with ffprobe), the full API flow.
- Browser test: the whole flow in the console against dronesim.
- Glass-to-glass latency, 720p30 at 2.5 Mbps on one PC: about 0.77 s.
- FIPS-only mode: the drone and engine tests pass.

Not tested: a real drone, PX4 SITL, ArduPilot, cellular links, H.265, KLV
metadata.
