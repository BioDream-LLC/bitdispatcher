---
title: One permission-aware search across the whole CAD
tile: Search and unit recall
summary: One search across incidents, BOLOs, people, vehicles, attachments and units, with match reasons, partial plates, and a unit recall timeline.
description: How bitdispatcher's Search spans incidents, BOLOs, people, vehicles, attachments and units with match reasons, partial plates, audit and unit recall.
icon: search
---

## What it is

A dispatcher constantly needs to find something: a person by a partial name, a vehicle by a partial plate, an earlier incident at an address, a BOLO that mentioned a tattoo, a unit's movements over the last hour. If each of those lives in a separate screen with its own search box, the dispatcher wastes the seconds they do not have. One search across everything the CAD knows is faster and more complete.

The hard parts are permission and honesty. Public-safety records are not all equally open: juvenile names and medical notes are restricted, and one agency's records are not another's to read. A search has to respect those rules without the user having to think about them. And it has to be honest about matches — a partial plate or a common name produces possibilities, not certainties, and the search must never quietly turn a possibility into a confirmed link.

Searching inside documents raises its own problem. Much of what matters is locked in PDFs and photographs: a scanned warrant, a picture of a plate. Finding those words needs text extraction from PDFs and optical character recognition for images.

## How it works

A unified search indexes every searchable field across the record types: incident numbers, types, addresses, narratives and comments; message and BOLO text across all versions; people by names, aliases, dates of birth, phones and descriptions; vehicles by plate, VIN, make, model and color; attachments by file name, caption and the words inside; and units by call sign, radio identifier and who is aboard.

Permission is applied as the index is queried, not after. The search only reaches the agencies the user covers and watches, and restricted fields are withheld from users without the right, so nothing restricted is even counted.

For each result the search explains why it matched and how strongly, and keeps possible matches visibly separate from confirmed ones. Partial plates are supported with a wildcard for a character the dispatcher could not read. Every query and every record opened from a result is recorded, which is both an audit requirement and a deterrent against misuse.

## Why it matters in a 911 center

An officer reads a partial plate over the radio — three characters and a guess at a fourth. A search that accepts the partial and returns ranked possibilities, clearly marked as possibilities, lets the dispatcher help without asserting a match that is not there.

History saves lives. A dispatcher who can instantly pull every prior incident at an address knows before they send a crew that the last call there involved a weapon. One search across incidents, BOLOs and people surfaces that in seconds.

The audit of queries protects both the public and the dispatcher. Public-safety databases have been misused to look up neighbors and exes. A recorded trail of who searched for what, and what they opened, is the accountability that keeps access honest.

## Where CAD systems usually fall short

Search is often fragmented: one box for incidents, another for people, another for vehicles, and nothing that spans them. The dispatcher has to guess where a thing lives before they can find it.

Match quality is frequently opaque. A result appears with no reason and no strength, so the dispatcher cannot tell a strong hit from a coincidence, and some products overstate matches as confirmed when they are merely possible — a dangerous thing in a system that decides who gets stopped.

Attachments are usually a dead zone: file names are searchable but the words inside a PDF or a photo are not. And permission is often enforced in the interface alone, so a restricted record is hidden from the screen but still returned to a direct request.

## How bitdispatcher does it

bitdispatcher has one **Search**, reached from the magnifier in the status bar or by Recall in the classic key profile. It spans incidents, messages and BOLOs across every version, people, vehicles, attachments and units in a single query.

**What it searches.** Incidents by number, type, address, narrative and comments. People by names in any order, aliases, birth dates, phones, addresses, identifiers and descriptions. Vehicles by plate, **partial plate with a `?`** for a character you do not know, VIN, make, model and color. Attachments by file name, caption, and the words inside them — plain-text files, the text of PDFs, and the words in pictures where the server reads them. Units by call sign, radio identifier and who is aboard.

**Match reasons and strength; possible versus confirmed.** Each result says why it matched and how strongly. **Possible matches** are marked as such, and the search never links a person, vehicle or incident to anything on its own. Expired, updated and canceled BOLOs say so in capitals, so an old warning never looks current.

**Permission-aware.** You search the agencies you cover and watch; searching **beyond my agencies** is only available where you may work them, and every result names its agency. Juvenile names and medical notes are withheld from people without the right, and nothing restricted is counted or shown.

**Audit.** Every search, and every record opened from it, is recorded in the ledger.

**Unit recall timeline.** From a unit in the results, or from a unit's History, recall shows one unit over a chosen period in a single timeline: its statuses and mark-outs, the incidents with their type and address, who staffed it and in which position, status checks, messages, and where it was by AVL or radio. Narrow it by responder, status or incident, and download it as CSV. That timeline answers the questions that come up after a shift — where was this car, who was aboard, what did it handle — from one place.

**A worked example.** An officer calls out a partial plate: ABC followed by a digit they could not read, on a gray sedan. The dispatcher searches `ABC?` with make and color, and the search returns ranked possibilities, each marked **possible** and each carrying the reason it matched — the partial plate, the color, a prior BOLO that named a gray sedan. None is asserted as the vehicle. If one of those results is an open officer-safety BOLO, it shows in capitals with its priority, and opening it is recorded in the ledger along with the query itself. The dispatcher relays the possibilities as possibilities, and the record afterward shows exactly what was searched and what was opened.

The same search reaches an incident's history. Typing an address returns every prior incident there — with the type, the narrative and the comments searched — so the dispatcher sending a crew to a house can see at a glance that the last call involved a weapon. Because attachments are indexed by the words inside them, a scanned warrant that names a person, or a photograph of a plate, surfaces in the same results rather than hiding in a file no search could reach. The text inside a PDF is read on the server, and the words in a photograph are extracted by optical character recognition, so a document is as findable as a typed note.

## Why bitdispatcher fits

One search across everything is configured by nothing: it works out of the box over the records the CAD already keeps. Permission is enforced on the server as the query runs, so a restricted record is never returned, not merely hidden. Match reasons and the possible-versus-confirmed distinction keep the search honest. OCR and PDF text extraction reach inside documents. And because every query and every opened record is a hash-chained ledger event, the search is as accountable as the rest of the system.

## What has been tested, and what has not

Search and recall are tested in the Go suites and in the browser against the built-in simulators. OCR was checked with Tesseract on a rendered plate picture, and PDF text extraction on a browser-made PDF. The ledger that records queries and opened records is covered by the Go suites, which verify that the state rebuilt from the ledger equals the live state. bitdispatcher is version 0.0.0, built and tested but not yet in service, and it is not certified; the sign-in, audit and records handling have not been reviewed against the CJIS Security Policy.

## Try it

Open the [live demo](https://demo.bitdispatcher.com) and search for a partial plate with a `?`, or recall a unit's day. The relevant manual chapters are [Search: incidents, BOLOs, people, vehicles and units](/manual/#search), [A unit's status history](/manual/#unit-history), and [Ledger and audit](/manual/#ledger).

Related: [Messages and BOLOs](/tech/messages-and-bolos/), [Multi-Agency](/tech/multi-agency/), [Event Ledger](/tech/event-ledger/), [Staffing and Qualifications](/tech/staffing-and-qualifications/)
