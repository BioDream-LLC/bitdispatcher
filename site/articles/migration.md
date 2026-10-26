---
title: Moving from an earlier CAD without a risky cutover
tile: Migration
summary: Bring premises, address history and run cards from CSV exports, and run alongside the old CAD until the floor is ready.
description: How bitdispatcher imports premises, address history and run cards from an earlier CAD's CSV exports, and why running alongside beats a risky cutover.
icon: route
---

## What it is

Every agency that buys a new CAD faces the same problem: the old one holds years of accumulated knowledge, and that knowledge cannot simply be thrown away. The **premises** file knows which house has oxygen in use, which address has a dangerous dog, where the Knox boxes are. The **address history** is a record of what has happened at each location over years. The **run cards** encode which units respond to which call type in which area, the distilled judgment of how the agency responds. Losing any of it on the move to a new system is a safety problem, not just an inconvenience.

Migration is the work of carrying that knowledge across. The old CAD can usually export its data as spreadsheets, CSV or tab-separated files, so the practical task is to read those exports, match their columns to the new system's fields, and bring over the rows that are clean while flagging the ones that are not.

The larger question is how to switch over at all. A **cutover**, flipping the whole center from the old CAD to the new one at a single moment, is one of the highest-risk events in public safety IT, because the thing being replaced is the system people's lives depend on during the switch.

## How it works

CAD exports are rarely tidy. Column names differ between products, dates come in several formats, addresses may lack coordinates, and encodings vary between UTF-8 and older Windows code pages. A migration tool has to match columns by their names, check every row for problems, geocode addresses that arrive without a position, and report clearly which rows came in and which did not and why.

The three kinds of data behave differently. **Premises and hazards** need an address or a latitude and longitude; an address without coordinates has to be located against a map or address database before it is useful. **Incident history** needs at least a number and a date, and becomes a count of prior activity at a location rather than a full re-creation of old incidents. **Call types and run cards** need a code and a run-card definition, which has to be parsed into capabilities and counts (so many engines, a ladder, an ALS unit) and checked against the unit types the new system actually has.

On the cutover question, the safer pattern is well established: **run the new system alongside the old one**. The new CAD takes simulated and recorded traffic, or shadows the live floor on training positions, while the old system still runs real calls. Staff learn the new console on a **training board** that cannot touch live operations, scenarios are practiced, configuration is tried and refined, and only when the floor is genuinely ready does real traffic move over, often gradually rather than all at once.

## Why it matters in a 911 center

A dispatcher who sends a crew to an address needs to know about the oxygen and the dog **today**, not after someone re-enters the premises file by hand over the coming months. If the hazard knowledge does not come across with the move, there is a window where the floor is operating blind on addresses it used to know, and that window is exactly when an avoidable injury happens.

The cutover risk is just as sharp. There is no maintenance window for 911; the calls keep coming. A big-bang switch that goes wrong, a run card that does not behave as expected, a map that is not fully loaded, a dispatcher who has never driven the new console under pressure, turns into a response failure during live emergencies. Every hour the new system can run alongside the old one, on real-shaped practice traffic, is risk taken out of the switch.

## Where CAD systems usually fall short

Migration is often a professional-services project: the vendor quotes a price to map and import the old data, the agency has little visibility into what came across, and the result is handed back as a finished database rather than something the agency can inspect row by row. When a hazard is missing, there is no easy way to see why it did not import.

Cutovers are frequently pushed as events rather than processes, because running two systems in parallel is awkward when the new one needs its own servers, database and licenses before it can take any traffic at all. Standing up a realistic training environment is extra cost and extra infrastructure, so training gets compressed, and dispatchers meet the new system properly only after it is live.

## How bitdispatcher does it

Migration is a screen: Admin, Migration brings an earlier CAD's data over from the spreadsheets it exports, CSV or tab-separated, in UTF-8 or Windows encoding.

1. **Drop the file** on the page. Its columns are matched to bitdispatcher's from their names, and every row is checked at once.
2. **Choose what it holds** if the guess is wrong: premises and hazards, incident history, or call types and run cards. Any column can be changed in the table, with the first row's value shown beside it.
3. **Import** the rows that passed; the rows that did not are listed with why, such as no position and an address the map does not know, or a date it cannot read.

| Kind | Needs | Notes |
|---|---|---|
| Premises and hazards | an address (or latitude and longitude) | hazards and contacts separated by `;`; an address without a position is found on the map |
| Incident history | number and date | counts as calls before at an address; the incident panel shows the last five within 40 m over ten years |
| Call types and run cards | code and run card | `2 engine; ladder:1; als` becomes capabilities and counts; a capability no unit type has is pointed out |

Imported premises and run cards can be re-imported to replace what a file brought (premises by their id, call types by their code); history is added each time, so it is imported once. The imported **address history then shows with every incident**, and the advisory panel's repeat-address rule counts it, so the old system's record of a location is working from day one.

For the cutover, bitdispatcher is built to run alongside a current CAD. A **training board**, marked TRAINING in violet, runs the agency's own units, stations, call types, premises and map on a ledger of its own; each trainee has their own board, and nothing done there reaches the live board. Scenarios play calls, transcripts, crash data and radio traffic for practice, and a quality score checks what was done. Configuration changes are tried on the training board and **published to the live board with a reason**, and can be rolled back. This is exactly what lets an agency try bitdispatcher beside its existing CAD, on training positions, with simulated and recorded traffic, before anything real moves over.

## Why bitdispatcher fits

Standing a second system up to run alongside is usually the obstacle, and here it is small: bitdispatcher is **one 44 MB file** with no database server and no application server to install, and no per-seat license, so running it beside the current CAD costs almost nothing to try. Everything is **set up in the GUI**, the map, units, stations, call types and run cards, the migration itself, so the agency configures its own system and sees every imported row rather than receiving a black-box database. Because it is one program with the configuration in the browser, there is no separate migration product, no separate training-environment product and no separate records server to license; the same binary does the import, runs the training board and runs the live floor. And every import and every published configuration change is a recorded event, so the move is traceable.

## What has been tested, and what has not

The migration importer and the map router it relies on are covered by the Go test suite. The training board and staged configuration changes published to the live board are exercised in the browser tests, and the end-to-end suite includes a correction and a replay of a closed incident, which depend on the same history mechanisms.

The honest limits: migration reads the **CSV or tab-separated exports** an earlier CAD produces; it does not connect to another vendor's CAD directly. The **training board is not a backup** and is not a substitute for a tested failover plan. bitdispatcher is version 0.0.0 and not yet in service anywhere; it is ready for a center to try beside its current CAD, on training positions, with simulated and recorded traffic, not to carry live 911 traffic today.

## Try it

On the [live demonstration](https://demo.bitdispatcher.com), explore the training board and the console set up from the browser; the migration screen is under Admin. The relevant manual chapters are [Migration from an earlier CAD](/manual/#admin-migrate), [Premises and hazards](/manual/#admin-premises), [Training](/manual/#training) and [Exercises, injects and staging](/manual/#exercises).

Related: [The append-only event ledger](/tech/event-ledger/), [Training and staged changes](/tech/training-and-staged-changes/), [The offline dispatch map](/tech/offline-map/), [Road-time unit recommendation](/tech/road-time-recommendation/)
