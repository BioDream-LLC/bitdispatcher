---
title: Flying a drone to the incident, with live video and evidence
tile: Drones over MAVLink
summary: Send a PX4 or ArduPilot drone to an incident, watch its live video in the incident panel, and keep the recording as evidence.
description: How bitdispatcher flies a PX4 or ArduPilot drone to an incident over MAVLink, streams its video to the console, and records it with SHA-256 checkpoints.
icon: drone
---

## What it is

A drone as first responder (DFR) program puts a small aircraft over a call while ground units are still on the way. The drone launches from a dock or a station roof, flies to the address, holds overhead and streams video. The dispatcher and the responding crews see the scene before anyone arrives: is there smoke, how many vehicles, is someone still on the street, which way did they go.

Commercial DFR programs usually come as a closed package: a drone, a dock, a vendor's flight software and a vendor's video portal, joined to the CAD by an integration. That works, but it puts the drone in a separate world. The video lives in another app, the launch decision lives in another log, and the recording lives in another vendor's cloud.

bitdispatcher can now fly the drone itself. It talks directly to the drone's autopilot using **MAVLink**, the open protocol spoken by the two main open-source autopilots, **PX4** and **ArduPilot**, and it reads the drone's camera over **RTSP**, the common protocol for live video from cameras. No drone vendor's cloud sits in the middle. The launch, the flight, the picture and the recording all live on the incident the dispatcher is already working.

This is a first step, and it is important to say so at the top: **it has been tested with simulators only. It has not yet flown a real drone.** The section near the end says exactly what was tested.

## How it works

A drone is set up once in **Admin › Drones**. Each drone has a MAVLink link (a UDP address, a TCP address or a serial radio), an optional MAVLink 2 signing key, the address of its camera, a flying height, the lowest battery level allowed for a launch, a list of pilots, and two switches: whether a launch needs a pilot's approval, and whether video is recorded. Every one of these is a setting in the screen; nothing needs a configuration file.

The flight goes like this:

1. **A request.** A dispatcher presses **Send a drone** on an incident, or a launch rule asks for one for certain call types. bitdispatcher picks the nearest drone that is connected, on the ground, and above its minimum battery.
2. **Approval.** If the drone needs approval, the incident shows that the drone is waiting for the pilot. A listed pilot, or a supervisor when no pilots are listed, presses **Approve launch** or **Decline** and gives a reason. A launch rule never flies a drone by itself; it always waits for a person.
3. **Take-off and flight.** bitdispatcher arms the drone, commands a take-off to the set height, waits for the climb, then sends it to the incident's position. The drone holds over the scene.
4. **Live picture.** The video plays inside the incident panel. Anyone with access to the incident can open it; field crews only see video from incidents they are assigned to.
5. **Control.** Dispatchers can press **Hold** to stop the drone where it is, or **Return home**. The pilot's own controller stays in charge and can take over at any moment.
6. **Recording.** While the drone is on an incident, its video is written to a file for that incident.

The drone's position, height, heading, speed and battery show on the map and in the panel, and its state reads plainly: docked, launching, en route, over the scene, returning, or offline.

## Why it matters in a 911 center

A dispatcher works from what a caller says, and callers are frightened, partial and sometimes wrong. A picture from overhead settles many questions that otherwise wait for the first unit: whether the reported fire is a barbecue or a house, whether a crash has one car or four, whether the person reported with a weapon is still at the address. That changes what gets sent, how crews approach, and what the next caller is asked.

It also changes the record. If the drone's launch, flight and video are on the incident, the after-action review, the records request and the court case all start from one place, with one timeline, instead of a CAD log in one system and a video in another.

## Where CAD systems usually fall short

Most DFR setups treat the CAD as a source of addresses and nothing more. The CAD sends a location to the drone vendor; the drone vendor does the rest. The dispatcher has to look at a second screen to see the video, and the incident record holds, at best, a note that a drone was sent.

Evidence is the harder gap. Video that matters in court has to be shown to be unchanged since it was recorded. When the recording sits in a vendor's cloud, the agency depends on that vendor's word, its retention rules and its continued existence. When the CAD's own record says nothing about the file, nothing ties the two together.

Cost and choice are the third gap. Closed DFR packages bundle the aircraft, the dock and the software. Since December 2025, DJI and Autel drones and other foreign-made models are on the FCC Covered List, so many agencies need a different aircraft anyway, and a CAD that only works with one vendor's package narrows the choice further.

## How bitdispatcher does it

**Open protocols.** MAVLink and RTSP are open, documented and widely used. Any drone running PX4 or ArduPilot, with a camera that serves H.264 over RTSP, can be connected. bitdispatcher sends standard commands: arm, take off, reposition to a point, hold, return to launch. It supports the differences between PX4 and ArduPilot, such as how each one measures take-off height.

**Signed commands.** MAVLink 2 can sign every message with a shared key, so the drone only obeys a ground station that holds the key. bitdispatcher signs its commands when a key is set, and ignores a drone that signs with a different key. Without signing, anyone who can reach the radio or network link could command the drone, so the manual and the setup screen recommend turning it on. The signing key and any camera password are masked in the screen and never written to the logs.

**Video in the panel.** The camera's stream is repackaged on the server, without re-encoding, and sent to the browser in small pieces that the browser plays as they arrive. If the picture falls behind, the player jumps back to the live edge. On one test machine, with a 720p, 30-frame camera stream at 2.5 Mbps, the picture on screen trailed the camera by about 0.77 seconds.

**Evidence with checkpoints.** Each recording is a standard MP4 file kept on the agency's own server, in a folder for the incident. Files are never overwritten. Every ten seconds the server saves what has been written to disk and records the file's size and its SHA-256 fingerprint in the incident's ledger, the same tamper-evident record that holds every other change. When recording stops, the final fingerprint is recorded. The incident timeline shows each recording, its length and size.

When someone plays a recording, bitdispatcher first checks the file's fingerprint against the ledger. If even one byte has changed, it refuses to serve the file and says why. This was tested by changing a single byte of a saved recording.

**Rules that keep people in charge.** A launch rule can ask for a drone, but only a person can launch it. Training boards never fly real drones. Every request, approval, decline with its reason, hold, return and recording is in the ledger with who did it and when.

## The law, briefly

This is not legal advice, and agencies must check their own rules. In the United States, drone flights fall under the FAA's **Part 107** rules, with a certificated remote pilot and **Remote ID**. Flying beyond the pilot's line of sight, which most DFR programs need, requires an FAA waiver; the proposed **Part 108** rule for beyond-line-of-sight flight is not yet final. DJI, Autel and other foreign drones are on the **FCC Covered List**, and new models with their parts are blocked. Many states limit how police may use drones and how long video may be kept. The remote pilot stays in command of the aircraft; bitdispatcher asks, and the pilot decides.

Video from overhead can capture people who have nothing to do with the call. Recordings should be treated as criminal justice information and personal information: kept on a private network, limited to the people who need them, and kept or deleted on the agency's schedule.

## What is simulated, and what has not been tested

**Everything here was tested with simulators. bitdispatcher has not flown a real drone.**

The main test partner is **dronesim**, a small program that ships with bitdispatcher. It answers MAVLink the way PX4 does and serves a test video over RTSP. It is a simple model: it flies in straight lines at a set speed and does not model wind, physics or the many checks a real autopilot runs before arming. It is not PX4.

With dronesim, automated tests confirm that a drone is launched, flies out, holds over the scene and flies home; that a drone signing with the wrong key is ignored; that the live video plays and the recording is a valid H.264 file; and that the whole flow works in the console, from the button to the recordings list. The 0.77-second latency was measured on one PC with a clock burned into the video.

A first attempt to fly **PX4's own simulator** did not connect, because the Docker image we used keeps MAVLink inside its container. That test exists in the code and runs when pointed at a PX4 simulator; running it is the next step.

Not yet tested: a real drone, PX4's simulator, ArduPilot, cellular links, H.265 video, and KLV video metadata. A real flight will need a PX4 or ArduPilot aircraft that is allowed under the FCC rules, a Part 107 pilot, flights within sight, and MAVLink signing turned on.

## Try it

Build the simulator with `make sim` and run `bin/dronesim`, then add the drone in **Admin › Drones** using the addresses it prints. Open an incident, press **Send a drone**, approve the launch and watch it fly. The full setup, with the security and legal notes, is in [docs/DRONES.md](https://github.com/BioDream-LLC/bitdispatcher/blob/main/docs/DRONES.md) on GitHub. The older drone hookup, where a separate drone system flies the aircraft and bitdispatcher sends it the incident, is described in [Crash data and drones](/tech/crash-and-drones/).
