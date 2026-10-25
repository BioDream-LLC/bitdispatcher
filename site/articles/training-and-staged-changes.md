---
title: Training scenarios and staged changes that never touch the live board
tile: Training and staged changes
summary: A scored training board on the agency's own map, instructor-led exercises, and config changes tried on a training board before publishing.
description: How a CAD trains safely: scored scenarios on the agency's own map, instructor-led exercises, and config changes tried on a training board, then published.
icon: flask
---

## What it is

Two everyday needs on a dispatch floor are usually treated as separate problems: training people, and changing the system they work in. Both are risky to do on the live board, because the live board is sending help to real emergencies. A trainee's mistake must not dispatch a real unit, and a mistyped run card must not reach the floor at shift change without anyone knowing who changed it or why.

bitdispatcher treats both with the same idea: a **training board** that mirrors the agency's real configuration but runs on a ledger of its own, so nothing done there reaches live. Training runs scenarios on it; administrators try configuration changes on it and then publish the chosen parts to live with a reason. This is change control and practice sharing one safe space.

## How it works

A person ticks **Training** at sign-in. The console opens a board of its own, marked TRAINING in violet, with the agency's own units, stations, call types, premises and map, on a separate ledger. Each trainee gets their own board, even on a shared account, and a board left idle for two hours is cleared. Nothing done on it reaches the live board.

**Scenarios** play calls, transcripts, crash data and radio traffic into the training board on the agency's own map: a house fire with three callers, a cardiac arrest in Spanish, an interstate crash with crash data, an armed robbery, or a flood of calls on the line. **Score** then checks what was done against the center's standards: every caller placed on an incident, the right call type, the units sent, a reply in the caller's language, and each incident's quality review. **Reset** empties one trainee's board and starts again, leaving everyone else's untouched.

**Shared exercises** take this further. An exercise is one training board for everyone who joins it: tick Training at sign-in and type the exercise's name, and the first to join creates it. Trainees sign in with their own duties, so a call-taker, dispatchers and a supervisor work the same board together, and crews can join from the mobile terminal. An instructor, a supervisor or administrator, presses **Inject** to introduce a new caller with a location and what they say, to have a caller say more or move, to take a unit out of service, to fire an emergency button, or to put a unit on the radio. **Timeline** downloads every event for the debrief, and the timeline is kept when the exercise ends. Because the exercise is a full board rather than a single-seat drill, it exercises the handoffs that go wrong in real life: the call-taker who places the address, the dispatcher who reads the recommendation, the supervisor who sees the queue building. Practicing alone is the same feature with the name left blank, which gives one person their own private board to drill on.

**Staged changes** use the same board for configuration. An administrator signed in with Training changes call types, units, stations, response plans, guidance or facilities on the training board, tries them there, then opens **Staged changes**, which lists what differs from the live board. The administrator chooses the parts to apply, gives a reason, and presses **Publish**. With *Change configuration only through staging* turned on in Settings, those changes cannot be made on the live board directly at all. Because every configuration change is a ledger event, a published change is versioned, reversible and attributable.

## Why it matters in a 911 center

A trainee learns fastest on the center's own map, with its own station names and unit call signs, running the kinds of calls the center actually gets. A generic simulator teaches generic dispatch; the agency's own board teaches this job. And because the training board cannot touch live, a supervisor can hand a new call-taker a cardiac arrest with three frantic callers and let them make every mistake safely.

Configuration is the other half. Run cards, call types and response plans are changed often, and a wrong change goes straight to how units are recommended for real calls. Trying a change on a training board first, seeing exactly what differs, publishing with a reason, and being able to roll it back, turns a nervous shift-change edit into a reviewed, reversible act. When something does go wrong later, the ledger says who changed what, when and why.

There is a quieter benefit, too. A training environment that matches the floor lets a center rehearse the rare, high-stakes events it almost never sees for real: a mass-casualty crash, a working fire with a mayday, a flood of calls from one incident. New hires meet those scenarios in training rather than for the first time on a live console, and experienced staff keep their edge on calls that do not come often enough to stay sharp on. The same board lets a center validate a configuration change against those same scenarios before publishing it, so a new run card is proven against the hardest calls, not just the routine ones.

## Where CAD systems usually fall short

Training in many CAD products is an afterthought: a separate, stripped-down simulator with generic data that does not match the floor, or no safe environment at all, so new staff learn on the live system under supervision and the occasional real mistake slips through. Shared, instructor-driven exercises where a whole watch works one scenario together are rarer still. On the configuration side, changes are often made directly on the live system with no staging, no record of who changed what, and no easy way back, so a bad edit is discovered only when units are recommended wrongly for a real call.

## How bitdispatcher does it

Everything above is in the product, set up and run from the browser. The training board shares the live board's users, connections and maps but keeps its own ledger for incidents, so practice and real work never mix. On the training board, administration *is* staging: an administrator's changes stay there until published. Scenarios, scoring and reset are on the training screen; exercises, injects and the timeline are run by an instructor; staged changes and the publish step are reached from the same place an administrator would make the change. The quality review that scores a scenario is the same review used on live incidents, against the center's own standards and the questions heard on the transcript, so a trainee is measured the way a real shift is.

One caution is stated plainly in the manual: the training board is not a backup of the live board. It shares the live board's users, connections and maps, and it keeps its own incident ledger, so it is a safe place to practice and to stage changes, but it is not a place work survives if the live server is lost. Continuity is a separate, tested arrangement of several sites, and a center should treat the two as different plans with different purposes.

## Why bitdispatcher fits

It is free and open and ships as one binary, so a center gets a full training environment and a staging-and-publish workflow without buying a separate simulator or a change-management add-on. Both run on the agency's own real configuration, so training is realistic and staged changes are exactly what will go live. Everything is set up in the GUI and recorded in the ledger, so published changes are versioned, reversible and attributable. And because nothing on the training board touches live, practice and change control are safe by construction.

## What has been tested, and what has not

Sign-in with Training, the separate ledger, and the console lock are exercised end to end with Playwright, and the state-from-ledger equality that makes a replayed scenario trustworthy is checked in the Go suites. The scenarios are the built-in set rather than an exhaustive library, and the scoring and quality review are coaching aids, not judgments; a missed question may simply not be on the transcript. Version 0.0.0 is built and tested but not yet in service anywhere, so this has not been proven across a full training academy or a live change-management program.

## Try it

On the demonstration, sign in with Training ticked to open a violet training board, play a scenario, and see it scored. An administrator can change a call type on the training board and open Staged changes to see the publish step.

- [Training](/manual/#training)
- [Exercises, injects and staging](/manual/#exercises)
- [Quality review](/manual/#quality)
- [Agencies](/manual/#admin-agencies)

Related: [Agency scripts and advisories](/tech/scripts-and-advisories/), [Responder wellbeing](/tech/responder-wellbeing/), [Security](/tech/security/), [Staffing and qualifications](/tech/staffing-and-qualifications/)
