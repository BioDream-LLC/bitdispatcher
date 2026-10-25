---
title: Messages and BOLOs: an operational inbox inside CAD
tile: Messages and BOLOs
summary: An inbox inside CAD with four prioritized tones, acknowledgement, escalation, hashed and virus-scanned attachments, and versions.
description: How bitdispatcher's inbox handles messages and BOLOs: four priorities with alarm-sound tones, acknowledgement, escalation, hashed attachments and versions.
icon: mail
---

## What it is

Dispatch floors run on short written notices as much as on radio traffic: a be-on-the-lookout for a vehicle, an officer-safety warning about an address, an administrative notice about a road closure. For years those moved on paper, on a whiteboard, or in email that no one could prove had been read. An operational inbox inside the CAD turns them into first-class records that can be prioritized, acknowledged, updated and searched.

A BOLO — be on the lookout — is the most familiar kind. It describes a person or a vehicle to watch for, often with a plate and a direction of travel. An officer-safety notice warns about a hazard at an address or a person. Both have to reach the right people quickly, stand out by urgency, and prove they were received.

The design question for the tones is a safety one. When several alerts can sound, they must be told apart under stress. The principles in the medical-device alarm standard IEC 60601-1-8 are a good guide: distinguish alarms by rhythm and loudness, not pitch alone, so they carry through a noisy room and a headset and are still distinguishable to someone who is not listening closely.

## How it works

An inbox stores communications with a subject, a kind, a priority, a body, a sender and an audience. The audience is expressed as To and CC lists that can name units, roles, duties, people, agencies or everyone. Delivery and reading are recorded so the sender can see who received and acknowledged a notice.

Priority drives behavior. The highest priority interrupts and repeats until acknowledged; lower ones show a banner, a tone, or just an unread mark. Acknowledgement can be required, repeated and escalated, so a notice no one confirms is raised to a supervisor rather than silently ignored.

Attachments are checked by their content, hashed so the file on the record is provably the file that was sent, and scanned for malware where a scanner is configured. The words inside attachments are indexed so a search finds them later. Updates and cancellations are new versions that keep the earlier ones, so the history of a BOLO is intact and an old warning never looks current.

## Why it matters in a 911 center

A BOLO that reaches the wrong desks, or reaches the right desks too late, is a missed chance to catch a fleeing vehicle. Addressing by role, duty, unit or agency puts the notice in front of exactly the people who can act on it.

Acknowledgement is the point where safety meets accountability. When a supervisor issues an officer-safety warning, they need proof the officers saw it, and escalation when they did not. A notice that is merely sent is not enough.

The tones matter because a floor is loud. If every alert sounds the same, dispatchers learn to ignore them. Tones that differ by rhythm and loudness let a dispatcher know, without looking, whether the sound is an urgent alarm or a routine notice.

## Where CAD systems usually fall short

Many CAD products keep BOLOs as free text in a window with no priority, no acknowledgement and no expiry, so a canceled lookout lingers for days and an urgent one is lost among stale ones. Others put the essential warning in an attachment that a busy dispatcher never opens.

Attachment handling is often unsafe: files are stored without a hash, so there is no proof a record was not altered, and without a virus scan, so an infected file can spread from the one place everyone opens. Search rarely reaches inside a PDF or an image, so a notice that exists cannot be found.

Alerting is frequently crude — one tone for everything, or color alone to mark urgency, which fails for a dispatcher who is color-blind or not looking at the screen.

## How bitdispatcher does it

bitdispatcher's inbox sits inside the CAD, reached from the envelope in the status bar. It carries **messages**, **BOLOs**, **officer-safety** and **administrative** notices, and the agency can add its own kinds.

**Composing.** A notice has a kind, a priority, a subject and a body — the essential warning goes in the text, never only in an attachment — plus To and CC, the owning agency, the related incident, and for a BOLO the plate, vehicle, person and description. Audiences can be units, `ALL`, a discipline such as `ALL-FIRE`, `DISPATCH`, an agency, a role, a duty, a named person or a console. A BOLO with no one named goes to every position and terminal of the owning agency.

**Four priorities with designed tones.**

| Priority | Shows and sounds |
|---|---|
| **Urgent** | An interruptive banner and a repeating alarm until acknowledged; supervisors are told when no one acknowledges in time |
| **High** | A banner and a distinct tone; acknowledgement may be required, repeated and escalated |
| **Medium** | Highlighted, one tone; unread until opened |
| **Low** | An ordinary unread mark, a soft tick or silence |

The tones follow the alarm-sound principles of IEC 60601-1-8: they differ by rhythm and loudness, not only pitch. The alarm is five quick pulses, the alert three, the chime two soft notes, the tick one quiet pulse, each with harmonics so it carries over a room and through a headset. Several arriving together sound once, with the highest one's tone. Every priority is shown in words and with an icon, never by color alone, and only levels the agency allows can be silenced. The agency sets each level's tone, repeat, acknowledgement and escalation in Admin › Operations rules, with a **Play** button to hear each one.

Because browsers keep a page silent until someone has used it, after a reload the first key or click turns sound on, and until then an urgent banner says the console is being kept silent, with a button to enable sound.

**Attachments.** Pictures, PDFs and the types the agency allows are checked by their content, scanned where the agency runs a virus scanner (ClamAV where it is set up), and kept with their hash. Their words are searchable — plain text, the text of PDFs, and the words in pictures where the server reads them.

**Acknowledgement, audiences, versions and dates.** Delivery, opening and acknowledgement are recorded for the person and the duty they were working, not just the screen. **Effective** and **expires** dates bound a notice, with no expiration where the agency allows it. **Update** sends a new version and **Cancel** a cancellation; earlier versions are kept and shown in the history. A notice stays on the incident's timeline with its attachments, and everything is in the ledger. A person sees a communication only when it is for them, or as a supervisor or records staff of its agency.

**A worked example.** A supervisor issues an urgent officer-safety BOLO for a wanted person: a photograph, a description, a last-known vehicle with a plate. The text carries the essential warning — armed, do not approach alone — because the warning must never live only in the attached photo. The audience is `ALL-POLICE` for the owning agency, so it reaches every police position and every patrol terminal. The urgent tone is five quick pulses, repeating, and the banner holds the top of each console until a dispatcher acknowledges. A patrol terminal that has not been touched since a reload shows the silent-console banner first; the officer's first tap enables sound and the alarm plays. The supervisor watches acknowledgements come in by person and duty, and the two officers who have not confirmed are escalated after the configured time. An hour later the person is in custody; the supervisor presses **Cancel**, the BOLO moves to Archived labeled as canceled, the earlier versions stay in the history, and the whole sequence — issue, deliveries, acknowledgements, escalation, cancellation — is in the ledger and on the incident's timeline.

## Why bitdispatcher fits

The inbox is part of the CAD, not a separate tool, so a BOLO lives beside the incident it came from and shows on its timeline. The kinds, priorities, tones and allowed attachments are all configured in the browser. Attachments are hashed and virus-scanned, and every delivery and acknowledgement is a hash-chained ledger event tied to a person and a duty. The tones are built to a recognized alarm-sound principle rather than picked at random.

## What has been tested, and what has not

The messages and BOLOs features are tested in the Go suites and in the browser against the built-in simulators. The attachment virus scan was run against ClamAV 1.5.4's clamd with full signatures: a clean file passes and the standard EICAR test file is refused on both upload paths. OCR was checked with Tesseract on a rendered plate picture, and PDF text on a browser-made PDF. The priority tones are measured as rendered by the browser's audio engine for rhythm, level and absence of clipping, and have not yet been judged by ear on a dispatch floor. bitdispatcher is version 0.0.0, built and tested but not yet in service, and it is not certified.

## Try it

Open the [live demo](https://demo.bitdispatcher.com) and compose a BOLO from the inbox. The relevant manual chapters are [Messages and BOLOs: the inbox](/manual/#inbox), [Messages to and from the crews](/manual/#messages), and [Operations rules](/manual/#admin-operations).

Related: [Search and Recall](/tech/search-and-recall/), [Mobile Data Terminal](/tech/mobile-data-terminal/), [Multi-Agency](/tech/multi-agency/), [Command Line and Keys](/tech/command-line-and-keys/)
