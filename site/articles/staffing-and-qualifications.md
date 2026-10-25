---
title: Who is on duty, on each unit, and qualified to be there
tile: Staffing and qualifications
summary: Who is on duty and on each unit, sign-on and relief without gaps, double-assignment blocked, and live capability from the crew aboard.
description: How bitdispatcher tracks staffing and qualifications: sign-on, gapless relief, blocked double-assignment, credentials with expiry and live unit capability.
icon: users
---

## What it is

A call sign on a board does not respond to a call — the people aboard it do. Knowing who is on duty, who rides each unit, and in which crew position, is what turns a list of apparatus into a picture of real capability. A medic unit with no paramedic aboard is not an advanced-life-support unit, however it is labeled.

Staffing is the record of people on duty and assigned to units for a shift: who signed on, in what role, until when. Qualifications are the controlled record of what each person is trained and credentialed to do — a paramedic certification, a hazardous-materials team membership, a field-training role — each with a state and often an expiry date. Together they let the CAD derive a unit's live capability from the qualified people validly aboard right now.

The standards here are the agency's own and the credentialing bodies': certification levels such as EMT and paramedic, special-team qualifications, and the renewal periods set by the issuer. The CAD's job is to hold those accurately, enforce them in dispatch, and keep the record of who responded truthful after the fact.

## How it works

A staffing model records sign-on events — person, unit, position, kind of duty, shift, until when — and sign-off events, as a timeline rather than a snapshot. Relief and moves are recorded with effective times so coverage can be shown with no gaps and no double-counting. One person is on one active unit at a time unless a secondary assignment is explicitly recorded with a reason.

A qualifications model holds each skill or credential with its levels, its owning agency, how long it lasts, whether it must be verified before it counts, and its prerequisites. A person's credentials carry a state — trainee, qualified, instructor, restricted, suspended, expired — with dates and attached documents.

Live capability is computed, not stored: a unit's capability is the apparatus plus the people validly qualified aboard now. When the required qualification is not present, the unit drops to a limited capability with the reason, and the recommendation uses that live capability rather than the label on the unit.

## Why it matters in a 911 center

Sending the wrong capability costs time at best and harm at worst. If the closest medic has no paramedic aboard this shift, recommending it as advanced life support delays the right response. Capability derived from the crew prevents that.

Relief without gaps keeps the board honest through a shift change. A crew hands off at seven; if the record shows a gap or a double, coverage reports and the supervisor's view are wrong exactly when they need to be right.

After a critical call, the record of who responded has to be exact — who was aboard, what they were qualified for at that moment. Freezing the crew on the incident as it was, and never rewriting it, is what makes the after-action record trustworthy.

## Where CAD systems usually fall short

Many CAD products track units but not people, so the board shows a staffed engine when the engine is short, and capability is a static label that does not reflect who is actually aboard. Credentials, if tracked at all, live in a separate personnel system the dispatcher never sees, so an expired certification does not stop a bad dispatch.

Double-assignment is often unguarded — the same person can appear on two units with nothing to flag it. Relief is a manual edit that leaves gaps. And mutual-aid crews from another agency usually cannot be credited at all, because the receiving system has no way to accept another agency's qualifications.

Minimum staffing is frequently advisory text rather than an enforced rule, so a unit stays available when it should be limited or out of service, and supervisors learn about it only after a call goes badly.

## How bitdispatcher does it

**Staffing**, reached from the people button in the status bar, shows who is on duty, where, and who actually staffs each unit. The crew follows the unit into incidents, recommendations, history and reports — a call sign alone is not enough.

**Sign-on and approval.** A person signs on with the kind of duty (regular, overtime, holdover, a temporary detail, a call-back), the shift, until when, and availability. A crew may sign itself on from the terminal where the agency allows it; the agency decides per kind of unit whether that is allowed, needs a supervisor's approval, or is for dispatch only. **Edit crew** names the people on a unit and their positions — driver, officer, firefighter, medic, team leader, or the agency's own.

**Relief without gaps; double-assignment blocked.** One person is on one active unit at a time. Putting them on another asks whether to **move them**, for relief or a swap, or to record a **secondary assignment** with a reason. Each change carries a reason and an effective time, kept beside when it was entered, so coverage shows no gaps. A lost connection signs no one off.

**Minimum staffing to limited or out of service.** A unit left short — below its minimum, or a required position empty — is made **limited** or put **out of service**, as the agency decides, unless a supervisor records an **override** with a reason. The agency sets whether sending a short unit is only warned, needs a supervisor's override, or is blocked.

**Crews frozen on incidents.** When a unit is assigned, the incident keeps who was aboard and what they were qualified for at that moment; later changes never rewrite it, and changes during the incident are on its timeline. A supervisor can **correct** a crew record with a reason, and the original stays beside the correction.

**Qualifications and credentials.** Each agency keeps a controlled skills file: special teams, specialties, training roles and credentials, each with its levels, owning agency, how many months it lasts, renewal, prerequisites, whether it must be verified, and the documents it needs. A person's credential has a **state** — trainee, qualified, instructor, evaluator, restricted, temporarily unavailable, suspended, expired — with issuer, dates and restrictions. A supervisor marks one **verified**; an unverified self-entry never counts where verification is required. Supervisors are reminded once when a credential's renewal comes due and once when it expires.

**Teams, equivalency and live capability.** A unit's live capability is **Full**, **Limited** or **Unavailable**, computed from the apparatus and the qualified people aboard now: a hazardous-materials unit without its required technicians is not that-capable and shows **LIMITED** with the reason. The recommendation uses the live capability. For mutual aid, another agency's qualification can be accepted as an **equivalency**, with its scope and expiry; the person's own record is never merged or overwritten. Supervisors also see **coverage**: the minimum qualified people the agency wants on duty, the gaps, expiring credentials and each unit's capability.

**Mark-out maximum duration.** A mark-out kind can carry the longest it may last, and a unit out longer than that shows **OVER** on the boards and the supervisors are told once.

**A worked example.** A medic unit begins the shift with a paramedic and an EMT aboard, and shows **Full** capability — it can be recommended for an advanced-life-support call. Midway through the shift the paramedic is relieved by an EMT, recorded as a move with an effective time. The moment the crew changes, the unit's live capability drops to **Limited** with the reason, and the recommendation stops offering it as advanced life support; it remains available for basic life support and transport. When the agency's minimum for that unit type is a paramedic, the unit is instead made out of service unless a supervisor records an override with a reason. None of this rewrites the earlier incident the unit ran as a full medic: that call still shows the paramedic who was aboard at the time. Everything — the relief, the capability change, the override — is a recorded event with who, when and why.

## Why bitdispatcher fits

Staffing, crews and qualifications are all set up and worked in the browser, with no separate personnel system to integrate. Capability is derived live from the crew, so the recommendation reflects reality. Sign-ons, reliefs, corrections and credential changes are hash-chained ledger events, and the crew on an incident is frozen as it was, so the after-action record is exact. Equivalencies make mutual aid work without overwriting anyone's record.

## What has been tested, and what has not

The staffing and qualifications features are tested in the Go suites and in the browser against the built-in simulators. The ledger that records sign-ons, crews and corrections is covered by the Go suites, which verify that the state rebuilt from the ledger equals the live state. The crew and roll-call features are roadmap items tested in the browser and against stand-ins. bitdispatcher is version 0.0.0, built and tested but not yet in service, and it is not certified.

## Try it

Open the [live demo](https://demo.bitdispatcher.com), open Staffing, and put a crew on a unit. The relevant manual chapters are [Staffing: who is on duty and on each unit](/manual/#staffing), [Qualifications, special teams and credentials](/manual/#admin-qualifications), and [Personnel and crews](/manual/#admin-personnel).

Related: [Mobile Data Terminal](/tech/mobile-data-terminal/), [Multi-Agency](/tech/multi-agency/), [Road-Time Recommendation](/tech/road-time-recommendation/), [Search and Recall](/tech/search-and-recall/)
