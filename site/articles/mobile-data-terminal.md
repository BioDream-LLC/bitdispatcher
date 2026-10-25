---
title: The mobile data terminal the crew works from
tile: Mobile data terminal
summary: A browser terminal for crews: assignment, route, premises, one-tap statuses, GPS, an emergency button, and acknowledged messages.
description: How bitdispatcher's mobile data terminal gives crews their assignment, route, statuses, GPS, emergency button, and acknowledged messages and BOLOs.
icon: truck
---

## What it is

A mobile data terminal, or MDT, is the screen a field crew works from in the vehicle. Instead of taking every assignment and status over the radio, the crew sees the call on a laptop or tablet, acknowledges it, changes its own status with a tap, and reads notes and warnings without tying up a radio channel.

The MDT grew out of the mobile data computers that arrived in patrol cars and fire apparatus in the 1980s and 1990s. The idea was simple: give the crew the same incident the dispatcher sees, and let them update it directly. That cuts radio traffic, keeps the channel clear for genuine voice work, and puts the address and route in front of the driver in large type.

There is no single MDT standard. What matters is that the terminal shows the live incident, that statuses flow back to the console immediately, and that the device reports its own position so the console can route and recommend by where the units actually are.

## How it works

An MDT is a client connected to the CAD. It subscribes to the crew's assignment and receives updates as they happen: a new note, a priority change, another unit added. The crew's status buttons send events back, and those events are the same ones a dispatcher would enter from the console, so there is one record of the incident regardless of who moved it.

Position comes from the device's GPS. The terminal reads it and reports it to the CAD every few seconds as automatic vehicle location, or AVL. The console uses that position to draw the unit on the map, to route it to the call, and to recommend the closest right units by road.

Messages travel the same connection. Dispatch writes to a unit, the terminal shows it, and the crew acknowledges. Urgent messages hold the top of the screen and sound until the crew confirms, so a warning is never missed in the noise of a scene.

## Why it matters in a 911 center

A crew that can mark itself en route and on scene with one tap frees the radio for the traffic that has to be spoken. On a busy channel that is the difference between a clear transmission and a stepped-on one.

The address in large type, with the route drawn and the premises warnings shown, gets the crew to the right door faster and safer. A note that there is oxygen in the house, a large dog, or a Knox box is in front of them before they arrive, not read out as they pull up.

The emergency button is the sharpest reason of all. A crew in trouble holds one button and the dispatcher sees it at once. Seconds matter, and a button on the terminal is faster than keying a radio and speaking.

## Where CAD systems usually fall short

Field terminals are often a separate product from the dispatch console, bought from a different vendor and bolted on. The two drift apart: a status the crew sets does not always show the same way on the console, and a note typed on one side arrives late or not at all on the other.

Many terminals need a thick client installed and maintained on every laptop, which is a burden for a small agency with no dedicated IT staff. Others assume a constant data connection and behave badly at the edge of coverage.

Acknowledgement is frequently weak. A message is sent and the console assumes it was read, with no proof that the crew ever saw it. For an officer-safety warning that assumption is dangerous.

## How bitdispatcher does it

The MDT is part of bitdispatcher, not a separate product, served from the same binary at `/mdt`. A crew signs in on the vehicle's laptop or tablet with a **field** account and the unit's call sign as the position, such as `E1`, and the terminal opens at once. It runs in the browser, so there is nothing to install.

**The assignment** shows the type, the priority, and the address in large type, with the road route drawn on the map, its time and the roads it takes, the callback number, the other units on the call, and the notes from dispatch newest first. Priority notes stand out.

**One-tap statuses.** The status buttons are one tap each, with the usual next status highlighted, and `F1` to `F6` on a keyboard give en route, on scene, transporting, at hospital, available and out of service. **Mark-outs** are available the same way, so a crew can mark out for a meal, fuel or a report. Notes the crew writes to dispatch go on the incident's timeline as the unit's.

**GPS and AVL.** The device's position goes to bitdispatcher as AVL every few seconds once the crew allows location in the browser. That position feeds the map, the routing and the road-time recommendation.

**Emergency.** The crew holds the emergency button for two seconds. Dispatch sees the emergency banner at once and acknowledges it, and the acknowledgement flows back to the crew.

**Messages and BOLOs.** What dispatch or another unit writes to this unit appears with a reply line and quick replies such as Copy, Delayed and Need PD. The communications addressed to the unit or its agency's terminals — messages, BOLOs and officer-safety notices — are shown highest priority first, with the agency's own priority tones, repeats and acknowledgement, exactly as on the console. An urgent or high communication that needs acknowledging stays at the top of the screen and sounds until the crew presses **Got it** or **Acknowledge**. Opening one shows every version, the plate and description, and the attachments. Opening and acknowledging are recorded against the unit's sign-on, so there is proof the crew saw it.

**Pre-plan photos.** Crews take the agency's own pre-plan photos of each premise from the terminal, placed on the map; each records where and when it was taken. Those photos then show on the incident, in the call-taker's Hazards tab, and on the console, so the next crew to the address sees the back of the building before they arrive.

**Crew sign-on.** Where the agency allows it, a crew signs itself onto its unit from the terminal, naming who rides it this shift and in which position; the agency can require a supervisor's approval. The unit's capability then follows the qualified people aboard.

**Take it.** Where the agency allows crews to attach themselves, an available unit takes a waiting incident from the list in one press, recorded as a dispatch by the crew. A crew acts only as its own unit, and a dispatcher can open the terminal to see exactly what a unit sees.

**The route and the premises.** The route drawn on the terminal comes from bitdispatcher's own recorded road network, the same routing the console uses to recommend by road time, so the crew and the dispatcher are looking at the same path and the same estimate. The premises warnings — oxygen in use, a large dog, a Knox box, a hazard noted on an earlier call — ride with the assignment, so a crew knows what waits at the address before they open the vehicle door. Because the map, the route and the aerial photos are recorded into bitdispatcher's store rather than fetched from the internet on each call, the terminal keeps showing the address and the route where the data signal fades at the edge of coverage.

## Why bitdispatcher fits

The terminal and the console are one program, so a status the crew taps and a status the dispatcher types are the same event in the same ledger. There is nothing to install: a browser on the vehicle's device is enough. The route and the premises photos come from bitdispatcher's own recorded map, so the terminal keeps working where the data signal is weak. And every status, message, acknowledgement and photo is a hash-chained ledger event tied to the unit's sign-on.

## What has been tested, and what has not

The mobile terminal is exercised end to end by the Playwright browser tests, including messages and photos from the terminal. The messages, BOLOs and staffing features it draws on are tested in the Go suites and in the browser against the built-in simulators; the attachment virus scan was run against ClamAV, and the priority tones were measured as rendered by the browser's audio engine but have not yet been judged by ear on a dispatch floor. bitdispatcher is version 0.0.0, built and tested but not yet in service, and it is not certified.

## Try it

Open the [live demo](https://demo.bitdispatcher.com) and visit the mobile terminal to see a crew's view. The relevant manual chapters are [Mobile data terminal](/manual/#mdt), [Messages to and from the crews](/manual/#messages), and [Messages and BOLOs: the inbox](/manual/#inbox).

Related: [Command Line and Keys](/tech/command-line-and-keys/), [Messages and BOLOs](/tech/messages-and-bolos/), [Staffing and Qualifications](/tech/staffing-and-qualifications/), [Road-Time Recommendation](/tech/road-time-recommendation/)
