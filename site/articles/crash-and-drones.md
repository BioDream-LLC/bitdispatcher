---
title: Crash telematics and the drone as first responder
tile: Crash data and drones
summary: Rate crash severity from telematics by delta-V and rollover, flag electric vehicles, and launch a drone by rule or a press.
description: How bitdispatcher reads automatic crash notifications, rates injury risk by delta-V and rollover, flags EVs, and launches a drone as first responder.
icon: drone
---

## What it is

Two newer data sources are reshaping the first minute of certain calls: crash telematics from vehicles, and drones launched as first responders. Both put information or eyes on a scene before a crew arrives, and both have to be handled with the same discipline as any other input: they advise, a person decides.

**Automatic crash notification (ACN)**, sometimes advanced ACN, is the data a modern vehicle sends when it detects a crash. The car's own sensors, the same ones that fire the airbags, measure the severity of the impact and report it, through a telematics provider, as a structured message. The richest versions carry **delta-V**, the change in the vehicle's speed during the crash, along with the principal direction of force, whether the vehicle rolled, which airbags deployed, how many occupants and belt use. Delta-V is the single best predictor of injury in a crash, which is why the field-triage world built guidance around it.

**Drone as first responder (DFR)** is a program where a drone is dispatched to a call the moment it comes in, flying autonomously to the scene and streaming video while units are still en route. It gives dispatch and responders a live view: is there actually a fire, how many vehicles, is the suspect still there, which way did they run. The drone does not replace responders; it informs them.

Neither ACN nor DFR has a single governing standard the way location or reporting does. ACN flows from telematics providers in their own formats; DFR is a mix of drone platforms and local authorizations. What matters for a CAD is a clean way to receive the data, a sound way to interpret crash severity, and a clear record of who launched the drone and why.

## How it works

A crash notification reaches the center as a message describing the vehicle and the crash physics. The useful interpretation is a triage estimate: given this delta-V, this direction, this rollover status, how likely is serious injury, and therefore what response should be considered. The thresholds come from field-triage criteria that correlate crash mechanics with injury, not from a diagnosis of anyone in the car.

A detail that increasingly matters is the vehicle's power source. An electric or hybrid vehicle in a serious crash is a different problem for the fire crew: damaged high-voltage batteries can reignite hours later and need far more water. Flagging the powertrain early lets the right resources be considered.

A drone-as-first-responder flow starts with a trigger. Either a rule launches the drone automatically for certain call types, or a dispatcher presses to launch. The CAD hands the drone system the incident, its location and a summary; the drone system, which owns airspace and flight safety, decides which drone flies. The drone then reports back its status, position, battery and live video, which the center watches on the map while the incident runs.

The guardrails are the same as everywhere else: the severity rating is a rule of thumb to pick a response, not a medical judgment, and the drone's video is an input to decisions, not a decision.

## Why it matters in a 911 center

A crash notification can beat the 911 call, or arrive when the occupants cannot call at all. A high delta-V with a rollover and no belt tells the dispatcher to consider extrication, ALS and air medical before anyone on scene has confirmed injuries. Those seconds set up the whole response. The electric-vehicle flag reaches the fire crew before they are standing next to a battery that may reignite.

A drone overhead changes the picture a dispatcher is working from. Instead of guessing from a caller's panicked description, the dispatcher and the responding units can see the scene: the real size of the fire, the number of vehicles, whether the reported person with a gun is still in the parking lot. That view can upgrade or downgrade the response honestly, and it can keep a crew from walking into something blind.

## Where CAD systems usually fall short

Crash telematics, where it is handled at all, often arrives as an undifferentiated alert with no interpretation, so the dispatcher sees that a car crashed but not how hard, and the severity judgment falls back on the caller or the first unit. The powertrain is rarely surfaced in a way the fire crew sees in time.

Drone programs are frequently run from a separate console entirely, with the drone's video and position in the drone vendor's app rather than on the incident the dispatcher is managing. The launch decision and the drone's track often do not make it into the incident record, so the review cannot see who launched, when, or what the drone showed.

## How bitdispatcher does it

A vehicle's automatic crash notification arrives as a call marked **CRASH DATA**, carrying the vehicle, **delta-V** (the change of speed in the crash), the direction of impact, rollover, airbags, occupants and belts, and the power source (electric, hybrid).

bitdispatcher rates **injury risk** by rule. It is high for a delta-V of 40 km/h (25 mph) or more, a rollover, or 20 km/h with no belt, several impacts or a side impact; medium for 20 km/h or any airbag deployment. Suggestions follow the rating: ALS, extrication, air medical, more than one ambulance, and for an electric car a word to the fire crews. The manual is explicit that this is a rule of thumb from field-triage criteria to help pick the response, not a diagnosis, and the thresholds are adjustable in Admin › Settings. Crash notifications reach bitdispatcher through the **crash hook**.

Drones work through a connected drone system (Admin › Integrations). An incident on the map shows **Send a drone**, and bitdispatcher hands the drone system the incident, its type, priority, position and summary; the **drone system decides which drone flies**. A **launch rule** can send a drone by itself: incidents of call types marked **Send a drone** ask once each. The drone reports back to the drone hook with its status, position, battery and live picture; it **flies on the map**, its status goes on the incident's timeline, and **Live picture** opens its video. The incident summary, built from the record, notes a drone over the scene.

bitdispatcher can also fly a PX4 or ArduPilot drone itself over MAVLink, with the live video in the incident panel and the recording kept as evidence; see [Flying a drone to the incident](/tech/drones/).

## Why bitdispatcher fits

Both features live inside the one program and on the incident the dispatcher is already working. The crash rating is transparent and adjustable: the thresholds are stated, the suggestions follow from them, and an administrator can tune them to local protocol rather than accept a vendor's hidden logic. The electric-vehicle flag reaches the fire crew through the incident summary and suggestions, not a buried field. The drone's launch, track and status land on the incident timeline and the hash-chained ledger, so the record shows who launched and what was seen. And bitdispatcher stays in its lane: it hands the drone system the facts and lets it own flight safety and aircraft selection.

## What is simulated, and what has not been tested

This is important to state plainly. **Crash data and drones are tested with the simulator's crash notifications and drone system, not yet a telematics provider or a drone system.** The simulated BitVoice environment generates crash notifications and a drone that flies on the map, which exercises the hooks, the injury-risk rating and the map display end to end. It does not prove integration with any real telematics provider (the crash hook is a documented JSON contract, not a provider's own API) or any real drone platform.

The injury-risk rating is an aid, not a judgment; it estimates from crash mechanics and does not assess the people in the vehicle. A telematics or drone vendor integration needs that vendor's endpoint and credentials, and has not been run against the live service. The practical reading for a center is that the mechanics are proven and the plumbing to a specific provider is the remaining work, which is the normal state of an integration that depends on an outside party's credentials.

It is worth being precise about what the simulator does and does not demonstrate. It confirms that a crash message entering the crash hook is parsed into its fields, scored by the injury-risk rules, and shown with suggestions, and that a drone launched by rule or by a press appears on the map, reports status to the drone hook, and writes to the timeline. It does not demonstrate that a given automaker's telematics format maps cleanly to the hook, nor that a given drone platform's airspace and flight controls behave as expected; those are questions only a live integration answers.

## Try it

On the [live demo](https://demo.bitdispatcher.com), watch for a crash-data call in the queue to see the delta-V, the injury rating and the suggestions, and open an incident on the map to send a simulated drone. The manual chapters are [Vehicle crash data](/manual/#crash), [Drones](/manual/#drones), and [Hooks and connections](/manual/#hooks).

Related: [Transcripts and Translation](/tech/transcripts-and-translation/), [Caller Location](/tech/caller-location/), [Aerial and Oblique Imagery](/tech/aerial-and-oblique-imagery/)
