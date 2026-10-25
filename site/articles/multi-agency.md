---
title: One center dispatching for many agencies
tile: Many agencies, one floor
summary: A consolidated center dispatches for many agencies, each with its own numbering, units and board, enforced on the server.
description: How bitdispatcher runs a consolidated center for many agencies: per-agency numbering, units, boards scoped on the server, mutual aid and staged publishing.
icon: users
---

## What it is

A consolidated communications center dispatches for more than one agency from one room. A single county center may answer for several police departments, a few fire departments, an EMS agency and a public works department, all at once. Each of those is a separate agency with its own radios, its own incident numbering, its own units and its own way of doing things.

Consolidation has been the trend in public safety for decades, driven by the cost of running separate centers and the need for agencies to work the same call together. The hard part is that consolidating the center does not consolidate the agencies. Each keeps its own identity, its own records, and its own rules, and the CAD has to keep them straight while letting a dispatcher work several at once.

There is no single standard for this. What matters is that every agency is a first-class thing in the CAD, that its configuration and its board are scoped to it, and that mutual aid between agencies is recorded without erasing which agency a unit belongs to.

## How it works

A multi-agency CAD models each agency as a configured object: an identifier, a numbering scheme, a set of units and stations, a set of call types and response plans, and its own timers and dispositions. Everything that belongs to an agency names that agency, so a unit, a call type or a plan is never ambiguous about whose it is.

A dispatcher works for one agency at a time but can cover several at a position. The board shows the agencies the dispatcher covers and, for awareness only, the ones they watch. Scoping is enforced on the server, not just hidden in the interface: a request for an agency the dispatcher is not authorized to work is refused, even by a direct link.

An incident has a primary agency and may have assisting agencies. A unit sent from another agency keeps its home agency and shows as assisting. Mutual aid is recorded as mutual aid, not by pretending the unit belongs to the receiving agency.

## Why it matters in a 911 center

Incident numbers are the backbone of records and reporting. If two agencies share a number space, their records collide and neither can run its own statistics. Separate per-agency numbering keeps each agency's file clean.

Coverage changes through the night. A dispatcher may cover three agencies at the start of a shift and pick up a fourth when a neighbor goes dark. The board has to follow that without showing a dispatcher calls they have no business seeing, and without hiding a call they need to act on.

Mutual aid is routine, and getting the agency wrong on a unit corrupts the record of who responded. When a fire department sends an engine to help the next department over, the engine is still that first department's engine, and the report has to say so.

## Where CAD systems usually fall short

Some CAD products treat multiple agencies as an afterthought, bolting a department code onto incidents rather than modeling agencies properly. The result is leaky: configuration from one agency bleeds into another, and a dispatcher sees more than their authorization should allow.

Enforcement is often cosmetic. The interface hides another agency's incidents, but the server will still return them to a crafted request, which is a problem when the agencies have different legal custodians of the same records.

Publishing configuration is another weak spot. A change meant for one agency goes live for all of them, or a center cannot test a change before it affects a live board, or there is no way to roll a bad change back.

## How bitdispatcher does it

Every organization the center dispatches for is a configured **agency** in Admin › Agencies — police, fire and EMS are disciplines, not agencies. A center with no agencies configured simply works as one with center-wide settings.

**Configured agencies.** Each agency has its code and name, its disciplines, its ORI, its NERIS ID and other identifiers, its own incident-number prefix, its dispositions, status-check timers, talkgroups and records rules. Its units, stations, call types and response plans each name their agency, chosen from the list rather than typed. An agency's response plan is only for its incidents, and its timers only for its units.

**Coverage: covered versus watched.** The status bar always shows the one agency you are **working** for; its rules apply to what you do, including its incident numbers, call types, timers and response plans. Clicking it shows the agencies you **cover** at this position and the agencies you **watch** for awareness only. Your usual coverage loads by itself at sign-in, from your profile and the workstation's configuration, and any change is recorded. You may only cover agencies you are authorized to work.

**The board, scoped and enforced.** Someone limited to some agencies sees only their part of the shared board: those agencies' incidents and units, incidents with no agency yet, units of other agencies sent to their incidents, and 911 calls until they become another agency's incident. The server enforces it — another agency's incident or unit is refused by a direct link or on the command line, and the attempt is recorded. Someone who works every agency sees everything and can choose **The board shows › Only the agencies covered and watched here** to narrow their own view without losing access.

**Primary and assisting.** An incident has a **primary** agency, taken from its call type or the dispatcher's working agency, and may have **assisting** agencies. A new primary agency needs a reason. The **Agencies…** control under the units changes the mix.

**Mutual aid with the home agency kept.** Every unit keeps its home agency. A unit of another agency sent to the incident shows as assisting by itself, so the record of who responded is always correct: the assisting agency's engine is still the assisting agency's engine.

**Staged, per-agency publishing.** Configuration changes can be drafted on the training board, tried there, and **published** to the live board with a reason — all at once or for one agency only. A publish for one agency never carries another agency's or the center's changes. Each publish keeps the versions it replaced, so **Roll back** puts the earlier approved version back, with a reason, and the rollback is itself recorded.

A worked example makes the scoping concrete. Suppose a county center dispatches for two police departments, two fire departments and one EMS agency:

| Agency | Discipline | Number prefix | Owns |
|---|---|---|---|
| RPD | Police | `26-` | Its cars, its call types, its plans |
| RCPD | Police | `RC26-` | Its cars, its call types, its plans |
| RFD | Fire | `F26-` | Its apparatus, its stations, its run cards |
| RCFR | Fire | `RCF26-` | Its apparatus, its stations, its run cards |
| EMS | EMS | `M26-` | Its medic units, its plans |

A police dispatcher covering RPD and RCPD sees both departments' cars and incidents, plus any 911 call that has not yet become one agency's incident, plus a medic sent to assist on one of their calls. They do not see an RFD structure fire unless an RFD unit is sent to one of their incidents. The server, not the interface, decides that: a crafted request for an RCFR incident is refused and recorded. When the EMS dispatcher goes on break, a supervisor can extend another dispatcher's coverage to EMS for the duration, and the change is recorded with the time.

The same discipline applies to configuration. A change to RPD's call types is drafted on the training board, tried, and published to RPD alone, with a reason such as *new disposition for welfare checks*. RCPD's configuration is untouched, and if the change turns out to be wrong, Roll back restores RPD's previous approved version — again with a reason, again recorded. Because publishing is per agency, a center can correct one department's run cards at three in the morning without any risk to the other departments on the same board.

## Why bitdispatcher fits

Agencies, coverage and the board are all set up in the browser, with no configuration file to edit. The scoping is enforced on the server, so a limited dispatcher cannot reach another agency's records even by a direct link. Every coverage change, publish and rollback is a hash-chained ledger event. And because the whole center runs from one binary against a recorded map, a small consolidated center can dispatch for many agencies without a server room.

## What has been tested, and what has not

The agency features, including the agency-scoped board, are tested in the Go suites and in the browser against the built-in simulators; the agency-scoped board is tested through the API. Sending and receiving incidents across neighboring agencies' CADs is tested bitdispatcher-to-bitdispatcher as NENA EIDOs, not yet against another vendor's CAD. bitdispatcher is version 0.0.0, built and tested but not yet in service, and it is not certified.

## Try it

Sign in to the [live demo](https://demo.bitdispatcher.com) and click the working-agency indicator to change your coverage. The relevant manual chapters are [The agency you are working for](/manual/#working-agency), [Agencies](/manual/#admin-agencies), and [Incidents with neighbouring agencies](/manual/#cad-to-cad).

Related: [Command Line and Keys](/tech/command-line-and-keys/), [Staffing and Qualifications](/tech/staffing-and-qualifications/), [NENA i3 and EIDO](/tech/nena-i3-eido/), [Training and Staged Changes](/tech/training-and-staged-changes/)
