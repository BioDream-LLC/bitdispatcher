---
title: NCIC inquiries through the state switch, logged every time
tile: NCIC
summary: Wanted-person, vehicle, gun and article inquiries through the state message switch, each with a purpose and reason in the ledger.
description: How bitdispatcher sends NCIC inquiries through the state message switch, logs purpose and reason, and offers a training simulator for practice.
icon: search
---

## What it is

**NCIC**, the National Crime Information Center, is the FBI's nationwide database that police agencies query for wanted persons, stolen vehicles, stolen guns and other property, protection orders and more. A dispatcher or an officer runs an inquiry and gets back whether a matching record exists. NCIC has been the backbone of the "wants and warrants" check for decades, and it is one of the most consequential queries in public safety: the answer can change how an officer approaches a stopped car.

Agencies do not connect to NCIC directly. Each state runs a **message switch** that brokers traffic between local agencies, the state's own systems, the national NCIC, and **Nlets**, the international justice and public safety network that moves messages between states (for out-of-state registrations and licenses, for example). In Virginia the state system is **VCIN**. A local query goes to the state switch, which routes it to the right national or interstate system and returns the answer.

The whole system runs under the **FBI CJIS Security Policy**, the Criminal Justice Information Services policy that governs how criminal justice information must be protected: who may access it, how they are authenticated, what must be logged, how data is secured in transit and at rest, and how misuse is deterred. The policy is detailed and strict, because the data it protects can do real harm if misused.

## How it works

An NCIC inquiry is a coded transaction. The common query types are **QW** for a wanted person (by name with date of birth, or name with sex and race), **QV** for a vehicle (by plate and state, or by VIN), **QG** for a gun by serial number, and **QA** for an article by serial number. These follow the NCIC 2000 transaction formats. The query goes to the state switch addressed from the agency's **ORI**, its nine-character Originating Agency Identifier, which identifies the agency making the request.

Two things must accompany a query under policy: a **purpose code**, saying why the inquiry is being made (a criminal justice purpose, a specific authorized use), and a **reason**, typically the incident or case the query relates to. These are not optional niceties; they exist so every access to the data can be tied to a legitimate justification and audited later.

A match is called a **hit**. A hit is not proof and not grounds for action on its own: it is a lead that must be **confirmed with the entering agency**, which verifies the record is still valid and that the person or property is the one in hand, before anyone acts on it. The discipline of hit confirmation is central to how NCIC is meant to be used.

The query formats are terse and exacting, a legacy of the teletype era that the system grew out of. A wanted-person inquiry needs enough to identify a person without drowning the operator in false matches, which is why the common forms are a name with a date of birth, or a name with sex and race. A vehicle inquiry takes a plate and the state that issued it, or a VIN. Getting the format right matters, because a malformed query wastes seconds that count during a stop, and because an ambiguous query can return matches that are not the subject at all. This is one reason practice is valuable: the operator who knows the query shapes cold is faster and more accurate when it matters.

## Why it matters in a 911 center

An NCIC query often happens in the tense opening seconds of a stop. An officer calls in a plate; the dispatcher runs **QV**; the answer shapes what happens next. Speed matters, but so does getting the query right and reading the answer correctly, because a misread hit, or acting on a hit before confirmation, is how the wrong person gets stopped or detained.

Logging matters just as much. Because the data is sensitive and tightly governed, an agency has to be able to show, for any query, who ran it, when, for what purpose and against which case. If that record is thin or editable, the agency cannot answer the questions an audit or a complaint will ask. The center that captures purpose and reason on every inquiry, in a record that cannot be quietly changed, is in a far stronger position than one relying on memory or free text.

## Where CAD systems usually fall short

NCIC access is frequently handled in a separate terminal or application, disconnected from the incident the officer is on, so the dispatcher re-keys the plate and then re-keys the result into the CAD note by hand. That double entry is slow and loses the link between the query and the case. Logging is sometimes minimal, capturing the transaction but not the purpose and reason in a usable, tamper-evident form, which makes later audit painful.

Practicing is also hard. A dispatcher cannot safely learn the query flow on the live state switch, so training often means watching rather than doing, or risking real traffic. And where the query tool stores the full response, sensitive data sits around longer than it needs to.

## How bitdispatcher does it

NCIC inquiries are available on the reports screen, with **F9** on the dispatcher screen, and on the mobile terminal. An inquiry sends an NCIC 2000 transaction through the state's message switch: **QW** for a wanted person, **QV** for a vehicle, **QG** for a gun and **QA** for an article. Pressing F9 from a selected incident ties the query to that incident, so the reason is already in hand.

Every inquiry requires a **purpose** code and a **reason** (the incident number). The [ledger](/manual/#ledger) keeps who asked, when, the purpose, the reason and whether a record came back; the query terms are kept only as a fingerprint, and the answer is not kept at all. The screen states plainly that **a hit is a lead**, to be confirmed with the entering agency before any action.

Administrators set the agency's **ORI** and choose the switch in Admin, Settings. The switch can be off, a **training simulator**, or a switch adapter reached by TCP. The training simulator lets dispatchers practice the real query flow: plates, names and serials beginning with TEST return a record, and every answer is marked **SIMULATED**, so practice can never be mistaken for a live result and never touches a real system. A new operator can run dozens of queries, read the answers and learn the confirmation discipline without a single transaction reaching the state switch.

## Why bitdispatcher fits

The inquiry lives on the incident, so the plate or name and the result stay attached to the case, and the reason, the incident number, is already present. Because the purpose and reason are required and written to an append-only, hash-chained [ledger](/manual/#ledger), the agency has a durable, verifiable record of every access, and because the answer is not retained and the terms are kept only as a fingerprint, sensitive data does not linger. The training simulator means a dispatcher can learn and drill the exact query flow without any live traffic. All of it is part of the one program, configured in the browser.

## What has been tested, and what has not

This is where care is required, and the limits are stated plainly. bitdispatcher's NCIC function has been **tested only against its own training simulator**; no outside NCIC or Nlets simulator was reachable for testing, and it has not been exercised against a live switch. bitdispatcher is version 0.0.0 and not yet in service anywhere.

bitdispatcher makes **no compliance claim**. It is not CJIS-audited and holds no VCIN credential; there is no FBI approval for software. It can be built with a FIPS 140-3 validated cryptographic module (the Go Cryptographic Module, certificate #5247), but that does not make bitdispatcher itself certified. The one-time codes, console lock, sign-in notice and audit records have not been reviewed against the CJIS Security Policy. What the software does toward each control area, and the gaps that remain, are written down honestly in the project's compliance notes. Connecting to any live state switch is the agency's undertaking, subject to that state's own authorization and audit.

## Try it

On the [live demonstration](https://demo.bitdispatcher.com), sign in as a police dispatcher, select an incident, press F9, and run a query against the training simulator; names, plates and serials starting with TEST return a SIMULATED record. The relevant manual chapters are [NCIC inquiries](/manual/#ncic), [Settings](/manual/#admin-settings), [Signing in](/manual/#signin) and [The event ledger](/manual/#ledger).

Related: [NIBRS incident-based crime reporting](/tech/nibrs-reporting/), [Security](/tech/security/), [The append-only event ledger](/tech/event-ledger/), [The mobile data terminal](/tech/mobile-data-terminal/)
