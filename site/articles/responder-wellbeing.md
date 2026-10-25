---
title: Looking after the telecommunicators who work the hardest calls
tile: Responder wellbeing
summary: Critical calls put everyone who worked them on the supervisor's care list for a break and a debrief, with workload and quality on the board.
description: How a CAD supports telecommunicator wellbeing: critical calls trigger a care list, breaks and debriefs, workload tracking and quality review.
icon: heart
---

## What it is

A 911 telecommunicator is on the line for the worst moments of other people's lives: the cardiac arrest, the child not breathing, the officer calling for help. They do this without the physical release of going to the scene, and often without ever learning how it ended. The general research picture, drawn from studies of emergency communications staff over the past two decades, is consistent in its direction even where exact figures vary between studies and populations: telecommunicators carry real occupational stress, a meaningful share report symptoms consistent with post-traumatic stress, and critical incidents are a recognized contributor. The precise percentages differ by study and should not be quoted as settled; the pattern that this is a population at risk is not seriously disputed.

Wellbeing support for this work usually means a few things together: recognizing which calls were hard, offering a break and a debrief close to the event, watching workload and fatigue across a shift, and reviewing quality in a way that coaches rather than punishes. A CAD cannot provide care, but it can make sure the hard calls are noticed and the supervisor is prompted to act.

## How it works

The core idea is to turn "someone should check on them" from a hope into a prompt. Certain calls are marked critical, by call type or by what is said on them. The people who worked those calls are surfaced to a supervisor, who is given concrete actions: offer a break, arrange a debrief, have a quiet word. Separately, the system watches how hard each position is working across the shift and flags when someone is due relief. And a quality review, run against the center's own standards, is framed as coaching so that looking back at a call does not itself become a source of stress.

Each of these is recorded, not because the feelings are anyone's business, but so that the center can show that its own care process actually happened: that a break was offered, that a debrief was arranged, that the person was not simply left on the next call.

## Why it matters in a 911 center

The moments after a brutal call are when support matters most and are the easiest to skip. The queue is still full, another line is ringing, and the telecommunicator who just talked a parent through CPR is expected to answer it. Without a deliberate prompt, the hard call is absorbed and the next one is taken, shift after shift. Over a career that accumulates. Fatigue is the same story: a position that has run calls for hours without a break is slower and more error-prone exactly when accuracy saves lives, and the person working it is often the last to say so.

A system that notices the critical call, names everyone who worked it, and puts a break and a debrief in front of the supervisor changes the default from "cope and carry on" to "we saw that one, take a moment." That is a small thing per call and a large thing over a career.

The point about *everyone who worked a call* is worth drawing out. A difficult incident is rarely one person's. The call-taker who first heard it, the dispatcher who sent the units, a second dispatcher who added a note partway through, all carried part of it, and all can be affected. A care process that only flags the call-taker misses the people around them. By putting every person who touched a critical incident on the list, the system makes sure the quiet dispatcher who said little but heard everything is not overlooked. And because workload and critical-call counts are tracked per position across the shift, a supervisor can see when the same few people have absorbed call after hard call and are due relief before they ask for it, which they often will not.

## Where CAD systems usually fall short

Wellbeing is often outside the CAD entirely, left to a separate peer-support program that depends on someone remembering to trigger it. The CAD knows which calls were cardiac arrests, shootings or officers down, and who worked them, but does not use that knowledge to prompt anyone. Workload is visible as raw call counts, if at all, with no sense of how long a person has gone without relief or how many critical calls they have taken. And quality review, where it exists, is frequently experienced as scoring and discipline rather than coaching, which makes people dread it and undermines the very standards it is meant to raise.

## How bitdispatcher does it

An incident is a **critical call** when its call type is marked critical, cardiac arrest, shooting and stabbing to begin with, or when words such as not breathing, CPR, officer down or infant are said on it. Everyone who worked it, the call-taker and the dispatchers who entered, sent or noted, goes on the **care list** on the supervisor board, with the reason. The supervisor offers a break to each person, arranges a debrief with a note of when and where, and marks it done; each step is recorded. Once the incident closes, each person who worked it sees a quiet line on their own console: that was a hard call, take a moment, with the support line the center set in Settings, and a **Take a break** button that puts them on a break. The critical words, the support line and the critical call types are all set by the agency in Settings and Call types.

**Breaks and workload** sit on the same supervisor board. The cup on the status bar puts a person on a break and back. The board's People panel lists every signed-in position with calls in the last hour, actions, time since the last break, and critical calls in the last four hours, and gives each a state: **watch** after two hours without a break, a critical call, or a very busy hour; **relieve** after three hours without a break or two critical calls. It also names the least busy positions free to step in, so the supervisor is not left guessing who can relieve whom.

**Quality review** is run against the center's standards: answered in time (15 seconds, NENA's call-answering standard), entered in time, dispatched in time for priority one and two, location verified, callback and disposition captured, and the right questions heard on the transcript for the discipline. Each incident gets a score over the checks that applied, and the page shows each call-taker's average and each check's pass rate. The manual is explicit that a review is a coaching aid and that a missed question may simply not be on the transcript, which keeps the tool pointed at improvement rather than blame.

## Why bitdispatcher fits

The CAD already knows which calls were critical, who worked them, how long each position has gone without a break, and whether the center's standards were met, because all of it is in the ledger the system writes anyway. bitdispatcher uses that knowledge to prompt care rather than leaving it to memory. Everything is set in the GUI, so a center defines its own critical words, its own support line and its own standards. It is free and open and ships as one binary, so even a small center that cannot fund a separate wellbeing platform gets the prompts, the care list and the coaching-oriented review out of the box.

## What has been tested, and what has not

The critical-call care list, the break and workload panel, and the quality review are built and exercised in the browser against the simulator, and they read from the same ledger as the rest of the system. They are prompts and coaching aids, not clinical tools: bitdispatcher does not diagnose stress, provide counseling or replace a peer-support or employee-assistance program, and the research picture cited above is general rather than a basis for any claim about a given center. Version 0.0.0 is not yet in service anywhere, so this has not been run through a real wellbeing program on a live floor.

## Try it

On the demonstration, work a call type marked critical, close it, and watch the care list appear on the supervisor board and the quiet line on the console. Open the People panel to see workload and break states.

- [Critical calls and care](/manual/#care)
- [Breaks and workload](/manual/#breaks)
- [Quality review](/manual/#quality)
- [The supervisor board](/manual/#board)

Related: [Training and staged changes](/tech/training-and-staged-changes/), [Transcripts and translation](/tech/transcripts-and-translation/), [Staffing and qualifications](/tech/staffing-and-qualifications/), [The event ledger](/tech/event-ledger/)
