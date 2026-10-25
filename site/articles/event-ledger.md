---
title: The append-only hash-chained ledger under every dispatch
tile: Event ledger
summary: An append-only, SHA-256 hash-chained record of every action, so the day cannot be quietly rewritten.
description: How bitdispatcher records every dispatch action in an append-only, SHA-256 hash-chained event ledger for audit, replay and reports you can verify.
icon: ledger
---

## What it is

A 911 center's record of what happened is evidence. It is read back in court, in internal reviews, in response-time audits and in the worst case after a death. For that record to be trusted, two things have to be true: nothing important can be missing, and nothing can be changed after the fact without it showing.

bitdispatcher is built on an **append-only, SHA-256 hash-chained event ledger**. Every action that takes a person's attention, changes operational state, creates accountability or feeds a required report becomes one structured, time-stamped, attributable event. Events are only ever added. A correction is a new event that links to the one it corrects; the original stays exactly as it was written.

This is the idea behind **event sourcing**: the authoritative record is the ordered list of things that happened, and the live picture on the dispatch floor is derived from replaying that list. The order is kept honest by a hash chain, the same construction used in version control and in tamper-evident logs: each event carries the SHA-256 hash of the event before it, so any edit, deletion or reordering breaks every link after it.

## How it works

Each event is a line of JSON. It records who did it (a username, or `system`, `bitvoice`, `ali`, `radio`), the active role and duty, the agency and console, the incident, unit or call it touches, the time, and a data payload. It also carries two hash fields: `prev`, the hash of the previous event, and `hash`, the SHA-256 of this event with its own hash field blanked.

Because each hash covers the previous hash, the events form a chain. Change one byte of an old event and its hash no longer matches, which breaks the `prev` link of the next event, and so on to the end. There is no way to alter history quietly: you would have to rewrite every event from the change to the present, and a verified copy held elsewhere would still disagree.

A few design choices make the ledger dependable:

- **Every append is synced to disk before it returns.** An event the caller was told about survives a power cut.
- **Action is kept separate from outcome.** Starting a query, receiving its result, viewing a record and relaying it are distinct events.
- **Batches are a parent plus children.** Eleven attachments added in one action are eleven events plus one batch event, so counts stay honest.
- **A torn final line is handled safely.** A write cut short by a crash, which was never acknowledged to anyone, is set aside; any other verification failure stops startup and asks for a person, because a broken chain is not something to repair automatically.

**Replay** is the direct benefit of event sourcing. Because the live state is only the sum of the events, the whole state can be rebuilt from the ledger, and any single incident can be played back on the map from its own events: created here, dispatched then, arrived there, corrected later.

**Reports are built from the ledger**, not typed a second time. One verified event serves the live response, the shift turnover, the Daily Operations Report, supervisory review, training and long-term analysis. Every total on a report links back to the events that produced it.

## Why it matters in a 911 center

A response-time dispute is decided on seconds, and those seconds come from status events: dispatched, en route, on scene, cleared. If a status can be back-dated with no trace, the whole report is suspect. A hash chain means a back-dated status is a new event with its own time and a reason, visible as a correction, not an invisible overwrite.

When a call goes wrong, the review needs the real sequence, including the things that are easy to lose: who acknowledged a hazard, when a location was offered and accepted, which unit was pulled from a meal break and why. If those are events rather than free text, they are all there, in order, attributable.

A records request or a subpoena asks the agency to prove that what it released is complete and unaltered. A verifiable ledger is a straight answer to that question.

It also changes how a center handles its own mistakes. Everyone who works dispatch long enough miskeys a status or sends the wrong unit. The honest response is to correct it and leave a trail, and an append-only ledger makes that the natural path: the correction is a new event with a reason, the original stays visible, and nobody has to choose between fixing the record and preserving it. A system that lets records be overwritten quietly tempts the opposite, and a culture that hides small errors is how large ones go unexamined.

## Where CAD systems usually fall short

Many CAD products keep operational data in a relational database where rows can be updated in place. An audit trail is bolted on as a second set of tables, often capturing only some changes, and an administrator with database access can edit either. The agency is then left to assemble the real history from audit tables, add-on logging and professional services, and to take on faith that nothing was missed.

Free text also tends to carry too much weight. When the only record that a hazard was relayed is a line in a comment field, it is easy to lose and hard to count. And because reporting is a separate subsystem, numbers are re-entered or re-derived, so a report can quietly disagree with the record it claims to summarize.

## How bitdispatcher does it

The ledger is the foundation, designed before any report or screen, because a report cannot recover an event that was never captured. Every feature writes to it: a status change, a dispatch, a note, a query, a file attachment, a location offered and the call-taker's decision to use it or keep the old one, a supervisor override with its reason.

From the browser you can **verify and search** the ledger. Verify re-reads the file from the start and checks every hash and link, reporting the event count and the last hash. The review views of an incident include a full audit view with the ledger checked, and every opening of a review view is itself recorded. A closed incident can be corrected without reopening it (type, priority, disposition, address, unit times), with what each value was and why, and any incident can be replayed on the map from its events.

Files and photos are covered too: each attachment's SHA-256 goes in the ledger, and a supervisor hides a file with a reason rather than deleting it, so the record that it existed remains.

The ledger is a plain file in the data directory chosen with `bitdispatcher -data <dir>`. There is no separate database server to stand up, and no second audit system to keep in step.

## Why bitdispatcher fits

The ledger is not an add-on; it is the center of the design. Because the live state is derived from it, there is no second source of truth to drift. Because it is a single file of hash-chained JSON lines, it is simple to copy, verify and keep for the long term. Because it is append-only, the honest answer to "has this been changed" is something the software can check, not something staff must vouch for.

It is also open. The ledger format is in the source, so an agency can read, verify and archive its own record with its own tools, with no dependence on a vendor to interpret it.

## What has been tested, and what has not

The Go test suite checks the ledger directly and checks that the live state rebuilt from the ledger equals the running state. The hash chain, append-and-sync, torn-line handling and verification are covered by unit tests, and the operations reports are tested to count from events.

What has not been done: bitdispatcher is **not certified**, and not CJIS-audited. Its SHA-256 comes from the Go Cryptographic Module; built with `make fips`, that is the module version validated under FIPS 140-3 certificate #5247, which is a validation of the module, not of bitdispatcher. The sign-in, lock and audit records have not been reviewed against the CJIS Security Policy; what the software does toward each control, and the gaps, are written down in the compliance notes. bitdispatcher is version 0.0.0 and not yet in service anywhere.

## Try it

On the [live demonstration](https://demo.bitdispatcher.com), make an incident, dispatch a unit, close it, then correct and replay it, and open the audit view to see the chain checked. The relevant manual chapters are [Ledger and audit](/manual/#ledger), [Incidents](/manual/#incidents), [Operations report](/manual/#reports) and [Records and requests](/manual/#records).

Related: [NG9-1-1, i3 and EIDO at the boundary](/tech/nena-i3-eido/), [Where the caller is](/tech/caller-location/), [Security](/tech/security/), [Several sites, active-active](/tech/active-active-sites/)
