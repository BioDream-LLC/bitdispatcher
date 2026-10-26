---
title: Continuity and overflow between 911 centers
tile: Continuity and overflow
summary: Working through a lost internet link or a lost building, and a neighboring center taking calls as incidents in your own CAD when you are overwhelmed.
description: How bitdispatcher keeps a 911 center working through internet outages and building loss, and lets a neighboring center take its calls into its CAD.
icon: server
---

## What it is

Every 911 center plans for three bad days: a link goes down, its systems go down, or it has to leave its building. NENA's continuity-of-operations guidance and the state plans built on it ask the center to know, for each, what keeps working, where the work goes, and who does it. A fourth bad day is the storm: the lines fill faster than the floor can answer, and the center needs another room's hands.

bitdispatcher answers all four in the web interface, without a separate product: the center's own server keeps dispatching when the internet goes; a second bitdispatcher site takes over when the building goes; and **overflow** lets a neighboring center that also runs bitdispatcher take calls for it.

## How it works

**Local first.** bitdispatcher runs on the center's own server with its own recorded street map, routing and ledger, so a cut internet circuit does not stop call entry or dispatch. Industry writing on cloud CAD calls this local survivability, and names its absence as the main reason centers hesitate to move dispatch entirely to the cloud. The center lists a few internet addresses it relies on; bitdispatcher checks them, BitVoice, the other sites and each neighbor every 30 seconds. When the internet goes, every console shows **Internet down** and the Continuity page says what keeps working and what waits for the internet.

**A second site for the building.** The backup is another bitdispatcher site, active-active, in a building the county owns or in the cloud. It already holds every event, so the people who move there carry on where they left off. A supervisor declares the move; every console at every site shows **At backup site**, and the center's checklist (reroute 911, radio control stations, tell the field) is ticked and recorded step by step.

**A continuity pack for paper.** A printable page of the open incidents with callers and callbacks, every unit and its status, the checklist, the contacts and blank log lines is written on a schedule to folders the center chooses, such as a shared drive or a USB disk on another machine, so a recent copy survives the server.

**Overflow for the load.** An overwhelmed center's supervisor asks a neighbor to take its calls. The neighbor's supervisor accepts, or its settings accept on their own under a standing agreement. The neighbor's call takers then enter calls on a form of the asking center's call types; each becomes an incident in the asking center's CAD, under its numbers, marked as taken by the neighbor. The asking center's dispatchers send its units, unless it has let the neighbor assign them. Status, units, notes and the close come back to the neighbor for those calls only.

## Why it matters in a 911 center

When the storm comes, the floor is not short of software; it is short of people. Neighboring counties already lean on each other by phone, reading addresses across a line that is also ringing. Taking the call straight into the other county's CAD removes the relay: the incident is in the right system, with the right numbers, before anyone has to read it back. And because the neighbor sees only the calls it took, nothing else of the asking county's records leaves its system.

## Where CAD systems usually fall short

Continuity in many CAD products depends on a vendor-hosted failover service, a licensed database cluster, or a standby that someone must bring up by hand. CAD-to-CAD links between neighbors usually exchange incidents one at a time, with updates flowing back only where both vendors have built the same interface. Overflow, where it exists, is often a phone-routing arrangement with nothing behind it in the CAD.

## How bitdispatcher does it

Everything is set in the web interface. Admin › Continuity holds the internet checks, the relocation checklist, the contacts, the continuity pack's schedule and folders, and, per neighbor, the overflow agreed: whether we may ask them, whether they may assign our units, whether we take their calls and accept on our own, and which of our agencies and call types are shared. The neighbor's address and a token each way are set in Admin › Integrations, with an optional certificate pin for a neighbor whose certificate is its own. Every request, answer, call, note and assignment is in both centers' ledgers.

The 911 calls themselves move only when the phone network moves them: the 911 authority's alternate routing, or BitVoice's routing policy in NG911. bitdispatcher carries the work once the calls arrive; it does not pull calls from another center's lines.

**When the neighbor cannot be reached.** A call taken for a neighbor whose CAD does not answer is not lost. It stays at the helping center, marked as not delivered with the reason, and is sent again every 20 seconds until the asking center accepts it; the call taker is told to pass anything urgent by radio or phone in the meantime. A call taken while the overflow was live and delivered after it ended is still accepted, and the same call delivered twice, as when an answer is lost on the way back, makes one incident, not two. Notes the helper adds are kept and retried the same way until they are delivered.

**What crosses, and what does not.** The helper sends the call as its call taker heard it: the location, the call type chosen from the asking center's own list, the caller, the callback and what happened. The asking center sends back only what became of that call: its status, the units on it, its notes and its close. The helper never sees the asking center's other incidents, its records, its person history or its NCIC returns, and the asking center sees nothing of the helper's own work. Both sides record every step under the name of the person who took it, so a review after the storm can follow any call from the first ring at one center to the last unit cleared at the other.

**Ending it.** An overflow lasts four hours unless the asking supervisor chooses another length or either side ends it sooner. Ending is a button at either center; the other is told at once, and if it cannot be reached it is told by phone, as the screen says.

## Why bitdispatcher fits

Continuity usually arrives as a list of separate products: a hosted failover service, a replication licence, a paper binder and a phone tree. In bitdispatcher they are parts of one program the center already runs. The backup site is the same binary with the same ledger; the continuity pack is drawn from the same records the consoles show; overflow uses the same neighbor address and tokens as incident sharing, and writes to the same ledger as everything else. Because it is free and open, a second site or a neighbor's link costs no extra licence.

## What has been tested, and what has not

Overflow is tested in the Go suite between two centers over HTTPS: a request refused before the helper agreed, accepted, a call becoming the asking center's incident, a repeated delivery not making a second incident, a note, a unit assigned by the helper, status and the close coming back, a call kept while the asking center could not be reached and delivered afterward, and no calls after the end. The browser suite runs the same path through the screens with one test server as its own neighbor, its certificate pinned. The link watch, the move to the backup site and the continuity pack are tested in Go and in the browser. Not yet tested: two centers on different networks, a real line cut at a center, or a 911 authority's alternate routing. bitdispatcher is version 0.0.0 and not in service anywhere.

## Try it

The demonstration is a single center, so overflow there has no neighbor to ask; the manual chapters show the screens.

- [Continuity: outages and the backup site](/manual/#continuity)
- [Overflow to a neighboring centre](/manual/#overflow)
- [Admin: continuity and overflow](/manual/#admin-continuity)
- [Several sites](/manual/#admin-sites)

Related: [Active-active sites](/tech/active-active-sites/), [The offline map](/tech/offline-map/), [NENA i3 and EIDO](/tech/nena-i3-eido/), [Multi-agency dispatch](/tech/multi-agency/)
