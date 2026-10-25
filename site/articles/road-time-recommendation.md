---
title: Run cards and road time, not the straight-line nearest unit
tile: Units by road time
summary: Recommend the right units by capability and real driving time, counting who is on scene and what runs short.
description: How bitdispatcher recommends units by run card, capability and real road time, with alarm levels, due order, move-ups and optional auto-dispatch.
icon: route
---

## What it is

When a call comes in, someone has to decide which units go. The traditional answer is the **run card**: a pre-built list, per call type and per area, of what to send, how many and in what order. A structure fire is not one engine; it is a first-alarm assignment of engines, a ladder, a rescue and a chief, and a second alarm adds more.

The question underneath the run card is which specific units fill it right now, and the right measure is not how close a unit looks on the map but how long it takes to drive there. The nearest unit by straight-line distance can be across a river with no bridge for miles, or on the wrong side of a limited-access highway. **Road time** over the actual network is what gets help to the door.

bitdispatcher recommends units by **capability and road time**, built on the run card, the current status of every unit and the recorded routing map.

## How it works

A recommendation is assembled from the incident's run card. Each run-card line asks for a capability and a count, for example two engines and one ladder. For each line bitdispatcher:

- Counts the units **already on scene** that fill it, so a second call for the same incident does not re-send what is already there.
- Finds the best available units by **road time** over the recorded map, labeling each with how the time was found: a real drive over the network, a straight-line estimate when routing cannot connect, from live **AVL**, from a stale AVL position, or from the unit's station.
- Reports what is **short**: how many of a line could not be filled, and the next-best units a dispatcher can swap in.
- Flags **which stations would be left empty** if these units go, so the dispatcher sees the coverage cost of the assignment.

A recommendation also carries **warnings** that must be confirmed before dispatching, when the location is unverified, uncertain or missing, and it never sends anything on its own. The dispatcher accepts it, changes it, or sends different units and is asked why.

**Alarm levels** extend the run card. Second and third alarms, any number and per call type, each add units; the `ALARM` command upgrades the incident and the recommendation fills the larger card. **Response plans** give a **due order**, first-due, second-due and so on, and when a planned unit cannot go, the recommendation shows who goes in its place and why the planned one could not, such as already on scene at another incident or out of service.

**Capability, not just a label.** A run-card line asks for a capability, and bitdispatcher fills it with a unit that actually has it now. A unit's capability comes in part from the qualified people aboard this shift: a medic with no paramedic riding is not offered as an ALS resource. So the recommendation reflects not only where units are but what they can do at this moment, which keeps a card from being filled on paper by a unit that cannot deliver the care the call needs.

**Move-ups** use the same road-time logic in reverse: when a station is left with a gap, the fire-house screen offers the best unit to move up and cover, in one click, chosen by driving time.

**Automatic dispatch** is available for the low-priority call types an agency chooses. The recommended units are sent only when no dispatcher has acted within the wait, only when the recommendation is complete and carries no warnings, and it is logged. It is off for every call type until an administrator turns it on. The guardrails are deliberate: auto-dispatch handles the routine, high-volume, low-risk traffic, an animal complaint, a routine transport, so a dispatcher's attention stays on the calls that need judgment, and it refuses to fire the moment anything is uncertain.

## Why it matters in a 911 center

Response time is the number an agency is judged on, and it is set in the first seconds, before a unit even moves. A recommendation that is right the first time saves the dispatcher from building the assignment by hand on every call, which matters most when several calls land at once.

Counting units already on scene prevents double-dispatch on multi-caller incidents. Showing what runs short keeps a dispatcher from believing a card is filled when it is one engine light. Flagging empty stations makes the coverage trade-off visible at the moment it is made, instead of being discovered when the next call comes in to a stripped district.

Road time over straight line is not a refinement; it is often the difference between the correct closest unit and a worse one, in exactly the terrain, rivers, highways, mountains, where it matters most. Two units can be the same distance from a call as the crow flies while one is four minutes away and the other is twelve, because one has a direct road and the other has to reach a bridge or an interchange. Straight-line distance cannot see that; a routing graph over the real network can.

Due order matters for fairness and for coverage planning as much as for speed. A response plan that names first-due, second-due and so on reflects how a district is meant to be covered, and when the first-due unit is already committed, seeing who steps up, and why the planned one could not, keeps the dispatcher's mental model of the district correct.

## Where CAD systems usually fall short

Some systems still recommend by straight-line distance, or by a coarse zone-to-station table that ignores where units actually are. Routing, when present, may depend on an online service, so it fails in the same outage that takes the map. Recommendations sometimes present a clean list without saying what could not be filled, so a dispatcher only learns a card is short when a unit arrives alone.

The coverage picture is often missing entirely: the CAD recommends units to the call without showing which stations go empty, leaving move-ups to the dispatcher's memory. And automatic dispatch, where offered, can be blunt, firing without the guardrails that keep a person in control of anything that is not low-priority and routine, so agencies either avoid it or accept more risk than they should.

## How bitdispatcher does it

The recommendation names each candidate unit with the run-card line it fills, the driving distance and time, where the position came from, and the reasons it was chosen, and it labels whether the time is a routed drive or an estimate. Run cards and call types are set up in **Admin › Call types & run cards**, response plans in **Admin › Response plans**, and the routing comes from the same recorded offline map used everywhere else, so recommendations keep working with no internet.

The dispatcher stays in charge. A reason is asked when units other than the recommended ones are sent, and an unverified location must be confirmed. Everything, the recommendation, the acceptance or change, the reason, and any auto-dispatch, is written to the ledger.

## Why bitdispatcher fits

Recommendation by road time is a core feature of the one program, not an add-on, and it runs on the recorded map so it survives an outage. It is free and open, set up in the GUI, and recorded in the ledger. The design rule that recommendations advise and a person decides runs through the whole system: nothing dispatches, closes or re-prioritizes an incident on its own, except the low-priority auto-dispatch an administrator deliberately turns on, with its guardrails.

## What has been tested, and what has not

The Go test suite covers recommendations and the road-time routing over the recorded map, including the run-card fill, short lines, move-ups and mark-out logic. Browser tests exercise call-to-incident-to-dispatch. Auto-dispatch is covered by tests and is **off** for every call type until an administrator enables it. bitdispatcher is **not certified** and not in live service. AVL positions in the demonstration come from the simulator, not a live radio system.

## Try it

On the [demonstration](https://demo.bitdispatcher.com), make an incident and watch units recommended by road, then raise an alarm level and see the larger card fill. The relevant manual chapters are [Unit recommendations](/manual/#recommend), [Call types and run cards](/manual/#admin-calltypes), [Move-up and cover](/manual/#moveup) and [Alarm levels](/manual/#alarms).

Related: [The offline map](/tech/offline-map/), [Where the caller is](/tech/caller-location/), [Staffing and qualifications](/tech/staffing-and-qualifications/), [The event ledger](/tech/event-ledger/)
