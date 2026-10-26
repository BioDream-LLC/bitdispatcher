---
title: Active-active sites that keep dispatching when cut off
tile: Active-active sites
summary: Several sites all live at once, each holding a verified copy of the others' ledgers, each able to keep dispatching alone through an outage.
description: How active-active CAD sites stay dispatching: each site holds a verified copy of the others' ledgers, works when cut off, and settles on one state.
icon: server
---

## What it is

Continuity of operations is the plan for keeping a 911 center dispatching when something fails: a server, a network link, a building, or the whole site. The usual answers are a cold standby that someone must bring up by hand, or a warm failover where a primary system hands off to a secondary when it detects trouble. Both have a single moment of truth: the instant the failover decides to switch, and the risk that the decision is wrong, late, or that the standby is stale.

An **active-active** arrangement removes that moment. Two or more sites run the same software at the same time, all of them live, all accepting work. There is no primary to fail over from, because none of them is the primary. If a site loses contact with the others, it keeps dispatching on its own; when contact returns, the sites exchange what each missed and settle back onto the same picture.

## How it works

The hard part of active-active is agreement: when two sites both change things while separated, they must end up in the same state once they can talk again. bitdispatcher solves this with the same append-only, hash-chained ledger that records everything else.

- **Each site writes its own ledger** and copies the others' over HTTPS, roughly every second. Every incoming event is checked against its originating site's hash chain before it is kept, so a corrupted or tampered event is rejected at the boundary.
- **The same picture everywhere.** Every site applies all events in the same deterministic order, by timestamp and then by site identifier. Sites that hold the same set of events therefore show the same state, no matter what order the events actually arrived in.
- **Cut off, a site keeps working** alone. When the link comes back, the sites exchange the events each missed and both settle on the same state. If two sites changed the same unit while separated, the later change wins, at both sites, by the same rule.
- **Incident numbers carry each site's prefix** (for example `N26-000001` and `S26-000001`), so two sites that are working independently never hand out the same number.

A site's name and prefix are written into every event it makes, so they are fixed once the site is in use and cannot drift.

## Why it matters in a 911 center

A dispatch floor cannot pause. If the link between a main center and its backup drops at 02:00 during a working fire, both rooms must keep taking calls and moving units, and neither can wait for an operator to decide which one is now in charge. Active-active means both rooms were already in charge, so there is nothing to switch. When the link heals, the fire that the backup worked and the crash that the main center worked are both in the merged record, in order, with no manual reconciliation and no lost minutes.

The failure that most often strands a center is not a dramatic one. It is a cut fiber, a misconfigured firewall, a power event at one building, a routine maintenance window that runs long. In those cases the ability of each site to keep going alone, and to catch up cleanly afterward with no operator decision and no data lost, is the difference between a quiet shift and a scramble. An active-active pair also lets a center move work deliberately: during a planned outage at one building, the other site is already carrying the floor, and the return is simply the link coming up and the events exchanging.

## Where CAD systems usually fall short

Replication in many CAD products is bolted on at the database layer, so continuity depends on a database cluster that an agency has to license, size and nurse, and on a failover script whose behavior is rarely tested under a real split. Standby sites are often warm, not live, so the backup is minutes or hours behind and someone must promote it by hand under pressure. When two sites do accept writes at once, conflict handling is frequently undefined, and operators discover after the fact that the two rooms disagree about which units are where. And because the whole arrangement rides on heavyweight infrastructure, small and mid-size centers simply go without it.

## How bitdispatcher does it

Sites are configured entirely in the browser, in Admin › Several sites. Give the local site a name and a prefix and save; copy its token and certificate fingerprint to the other sites. On each site, add every other site with its address, its token and its fingerprint, using **Read it** to read the certificate and checking it against what that site shows. The status bar then shows how many sites are linked. There is nothing else to stand up: no cluster, no replication service, no load balancer, because each site is a complete, single binary that already holds everything it needs.

Because continuity rests on the ledger, the same **Verify** that checks a single site's chain also means every copied-in event was validated against its source chain before being kept. The replication is the ordinary ledger, copied and checked, not a separate subsystem with its own failure modes.

**Working through an outage at the console.** The sites feature handles a lost center; a lost *server* is handled at the browser. Each console keeps the last board it had, and when the server cannot be reached an amber bar says so and shows that board as it was, with the time. A console reloaded during the outage still opens. New incidents, notes and unit statuses are kept in that browser in order as an offline log. When the server returns, the bar offers **Send them now**: each entry goes in marked, for example, "entered offline at 14:02 by jlee," and anything the server refuses stays with the reason shown. The offline log is a bridge over a short gap at one position; running two sites is the answer when the server itself must never be the single point of failure.

**A telephony denial-of-service warning.** Continuity is not only about servers and links. A flood of calls can take a center out as surely as a power cut. bitdispatcher watches the call stream and raises **Possible telephony denial-of-service** on every console and the supervisor board when far more calls than usual arrive in a minute, when one number calls five or more times in ten minutes, or when eight or more silent or very short calls arrive in five minutes. A supervisor acknowledges it and it clears after ten quiet minutes, with a reminder to keep answering because a real emergency can be among them.

**One binary simplifies continuity.** Because bitdispatcher is a single 44 MB file with no separate database or application server, standing up another site is copying a binary and its data directory and pointing the two at each other. The continuity plan has fewer moving parts to test, patch and reason about, which is exactly what a continuity plan needs.

## Why bitdispatcher fits

It is free and open, so a second or third site costs no extra license. It is one program, set up in the GUI, so neighboring counties can share a system without a server room each. Everything rides on the ledger that already records every action, so continuity and audit are the same mechanism rather than two systems to keep in step. And each site works offline by design, which is the whole point of active-active.

## What has been tested, and what has not

The sites feature is covered by the Go suites, which check that a state rebuilt from the ledger equals the live state and that sites converge on the same ordering. This is bitdispatcher talking to bitdispatcher, in a lab, not a geographically separated pair of production centers under a real network partition, and version 0.0.0 is not yet in service anywhere. The training board, worth stressing, is not a backup; a continuity site is a separate, tested arrangement. The telephony denial-of-service warning is driven by the call stream from BitVoice or its simulator in a lab.

## Try it

The demonstration runs as a single site, so the sites screen shows the setup rather than a live pair. Read the chapter to see the fields and the fingerprint check, and watch the offline bar by stopping and restarting the demo server locally.

- [Several sites](/manual/#admin-sites)
- [Working through an outage](/manual/#offline)
- [Line attacks (TDoS)](/manual/#tdos)
- [Ledger and audit](/manual/#ledger)

Related: [The event ledger](/tech/event-ledger/), [Security](/tech/security/), [The offline map](/tech/offline-map/), [Multi-agency dispatch](/tech/multi-agency/)
