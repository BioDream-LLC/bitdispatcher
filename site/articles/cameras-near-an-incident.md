---
title: Cameras near an incident, off by default and audited
tile: Cameras near
summary: The agency's own camera list, offered near an incident, off until switched on, with a required reason recorded for every look.
description: How bitdispatcher offers nearby cameras near an incident from the agency's own list, off by default, with a required purpose and a full audit.
icon: eye
---

## What it is

A call comes in for a crash at an intersection, and there is a traffic camera on the corner. A dispatcher who can glance at it for a moment knows how many vehicles are involved and whether the road is blocked before the first unit arrives. That is a real help on a real call. It is also, handled badly, the beginning of a surveillance system, so the question is not only whether a center can show a camera but how the power to watch is bounded.

bitdispatcher keeps the **agency's own list of cameras**, the ones it owns or that a partner agreed to share, and offers the nearby ones next to an incident. The feature is **off until an administrator switches it on**, opening a camera **requires a reason**, and every look is recorded. The design is deliberately narrow: a list the agency controls, off by default, a purpose required, and a full audit of who looked and why.

## How it works

The list is the agency's own: its traffic cameras, its buildings, its schools, and partners that agreed to share. Each camera carries a name, an owner, a position, a **heading** in degrees from north (left empty if the camera pans), a **width** across its view that defaults to **70 degrees**, a **range** it usefully sees that defaults to **150 meters**, and an address for viewing it. That address can be a **live view**, opened in a new window in the camera system itself, which signs the user in on its own, and/or a **still picture** that bitdispatcher fetches as a JPEG. Addresses must be **https**, or plain http only on the agency's own network.

When an incident has a location and the feature is on, bitdispatcher offers the cameras **within a radius** of the incident, **400 meters by default** and up to **5 kilometers**, with the ones **looking toward the place** first. A camera is counted as looking toward the place when the bearing to the incident falls within half its width of its heading and inside its range.

Opening a camera asks **why**. The dispatcher gives the incident or a reason, and it is required. The look is written three ways: as a **ledger event**, as a line on the **incident timeline**, and into a **"Who looked, and why"** view for supervisors, each recording who opened the camera, when, why, and for which incident. The still picture is served only to the person who opened that camera, and only **within the last 30 minutes**; while the view is open the picture refreshes every **5 seconds**. **bitdispatcher stores no video**. The live view is handled by the camera system in its own window, and the stills are passed through without being kept.

## Why it matters in a 911 center

On the calls where it helps, a glance at a nearby camera answers the questions that decide the response: how bad the crash is, whether a fire is showing, which way someone ran. Having the right camera offered automatically, nearest first and facing the scene, saves a dispatcher from hunting through a list by hand while a call is live.

The constraints matter just as much as the capability. A 911 center handles people on the worst day of their lives, and a camera tool that is always on, that anyone can open without saying why, and that keeps no record of who watched is a tool that will eventually be misused. Requiring a reason puts a small, deliberate pause in front of every look. Recording who looked and why means a supervisor can answer the question that a court, a reporter or a resident will eventually ask. Keeping no video means there is no quiet archive to be subpoenaed, leaked or repurposed.

Serving a still only to the person who opened the camera, and only for a short window, keeps a casual look from becoming a standing feed left open on a spare monitor. Off by default means a center that has not decided how it wants to use cameras simply does not have them, rather than finding them switched on by a default nobody chose.

Ranking by what a camera can actually see is a practical help, not a detail. A camera's heading and width describe the cone it covers, and its range describes how far that cone is useful; offering the ones whose cone points at the scene first means the dispatcher is shown the camera that is likely to answer the question, not the nearest camera pointed the other way. A camera that pans has no fixed heading, so it is offered on distance alone, which is honest about what the agency knows. None of this requires the dispatcher to understand the geometry: they see the nearest useful cameras, facing the scene first, with the distance marked.

## Where CAD systems usually fall short

Real-time crime center products from large vendors put camera feeds on a map and make watching easy, which is the point and also the risk. Several cities have stepped back from camera and plate-reader networks after misuse: one city ended a plate-reader camera contract in 2026, and a police department limited officer access to a camera network after an officer used it for personal reasons. The pattern is consistent. A watching tool with weak limits and weak records gets used in ways the agency never intended, and the agency finds out after the harm.

Many of these systems are built to maximize coverage: ingest every feed available, keep recordings, make search fast. Audit and purpose are often afterthoughts, bolted on or absent, and the default posture is on. The result is a system that is technically impressive and institutionally fragile, because it depends on everyone with access behaving well with no friction and no record.

## How bitdispatcher does it

The camera list is set up in **Admin › Cameras**, each with its owner, position, heading, width, range and viewing address, and the whole feature stays off until an administrator turns it on. Near an incident, the console offers the cameras inside the radius, those facing the scene first, and marks how far each one is.

Opening a camera is a small dialog that asks for the incident or a reason, and will not proceed without one. The still is fetched through bitdispatcher and shown only to the person who opened it, for a short window, refreshing every few seconds; the live view opens in the camera system's own window. The look lands in the ledger, on the incident timeline, and in the supervisor's "Who looked, and why". No video is kept.

## Why bitdispatcher fits

Cameras near an incident are part of the one program, not a separate real-time crime center platform, and they run on the agency's own list rather than a vendor's ingest of everything in reach. The feature is free, set up in the GUI, and off until the agency chooses it. It fits the project's rule that power over people is bounded and recorded: a dispatcher is offered what helps the call in front of them, is asked why before looking, and leaves a record a supervisor can read. The choice to keep no video, to require a purpose, and to default to off is the point, not a limitation to be configured away.

## What has been tested, and what has not

The Go test suite covers the core rules: a camera is not offered until an administrator switches the feature on, a purpose is required to open one, and cameras are ranked with those facing the place first. An API test with a stand-in camera serving a JPEG confirms the still is refused before the camera is opened and served after, and that the audit records the user and the incident. A browser test covers opening a camera near an incident. bitdispatcher is **not certified** and not in live service. It has **not** been tested with a real video management system or a real DOT camera.

## Try it

On the [demonstration](https://demo.bitdispatcher.com), switch cameras on in admin, make an incident near one, and open the camera to see the required reason and the record it leaves. The relevant manual chapters are [Cameras](/manual/#cameras) and [Cameras](/manual/#admin-cameras). The source is on [GitHub](https://github.com/BioDream-LLC/bitdispatcher).

Related: [Road closures and traffic](/tech/road-closures-and-traffic/), [The event ledger](/tech/event-ledger/), [Where the caller is](/tech/caller-location/), [Security](/tech/security/)