---
title: NERIS and NEMSIS, fire and EMS reporting from CAD
tile: NERIS and NEMSIS
summary: Fire incidents to NERIS on close, and NEMSIS 3.5 patient-care times from every EMS unit to the agency's ePCR system.
description: How bitdispatcher reports fire incidents to NERIS, the system replacing NFIRS, and sends NEMSIS 3.5 dispatch and time records from each EMS unit.
icon: truck
---

## What it is

Fire and EMS agencies both have a national reporting obligation, and both standards are in motion.

**NERIS**, the National Emergency Response Information System, is the fire service's new national data platform. It replaces **NFIRS**, the National Fire Incident Reporting System, which collected fire department activity data for decades in a fixed coded format. NFIRS was built in a different era, and the US Fire Administration, working with the Fire Safety Research Institute, moved the fire service to NERIS to collect richer, more structured data through modern interfaces, with NFIRS being retired. Where NFIRS was a flat coded record filed in batches, NERIS is an API-driven platform that takes incident data, including a dispatch, or CAD, record, and lets the department complete the rest.

**NEMSIS**, the National EMS Information System, is the EMS equivalent: the national standard for the electronic patient care report (ePCR). The current version is **NEMSIS version 3.5** (the 3.5.1 schema in detail), an XML standard with hundreds of defined elements grouped into sections, demographics, dispatch, times, the patient, the assessment, treatments and the outcome. A NEMSIS record is how an EMS run is reported to the state and onward to the national database, and how patient-care software exchanges data.

The two standards divide the work the same way: the dispatch center owns the dispatch facts and the times, and the responders own what happened on scene and with the patient.

The move from NFIRS to NERIS is a genuine shift, not a renamed export. NFIRS codes were designed to be entered by hand after the fact from a paper or screen form, and the data model reflected the questions of its day. NERIS is designed around data flowing in from the systems that already hold it, CAD above all, with the fire department completing only what CAD cannot know. For a dispatch center that means the integration is no longer "produce a file and upload it" but "deliver an accurate CAD record to a live platform," which raises the value of getting the dispatch facts and times exactly right at the source.

## How it works

For fire, a NERIS CAD record carries the skeleton of the response: when the call came in, when units were notified, en route, on scene and cleared, where the incident was, its disposition and the notes. It identifies the department by its **NERIS ID**, an FD prefix and eight digits, and authenticates with a CAD integration's client ID and secret issued by NERIS. NERIS accepts the CAD record and holds it **pending** until the fire department completes the incident with the fire-specific detail NERIS asks for. So the CAD system's job is to deliver an accurate, timely dispatch record and the times, not to write the whole fire report.

For EMS, a NEMSIS 3.5 record from CAD carries the dispatch section and the time stamps: the call time, when the unit was notified, en route, on scene, when it left the scene, arrived at the hospital and went back in service, along with the location and the dispatch reason coded to NEMSIS. The record follows the NEMSIS 3.5.1 schema for those sections. It is then imported into or sent to the agency's ePCR system, where the crew completes the patient sections, the assessment, the vitals, the treatments, the narrative, which only they can write. The dispatch reason comes from the call type's NEMSIS code.

## Why it matters in a 911 center

Times are the spine of both reports, and times are exactly what a dispatch center records to the second: dispatch to en route, en route to on scene, on scene to transport, transport to hospital. If those are hand-copied from CAD into a reporting system later, they drift, and a response-time report built on drifted times is not defensible. Taking them straight from the dispatch record keeps them honest.

The reporting is also a deadline the center cannot ignore. Fire and EMS funding, state compliance and the national picture all depend on records being filed completely and on schedule. Anything that lets a shift's closed incidents flow into the reporting pipeline without re-keying removes a standing source of error and a recurring chore.

And the division of labor matters. A dispatcher should never be asked to fill in patient-care fields, and a medic should not have to re-enter the dispatch times the center already captured. Each side contributing the part it owns, from the same incident, is how the record ends up both complete and correct.

There is also a practical rhythm to it. An EMS unit clears, and the record for that unit is ready at that moment, not at the end of a batch run. A fire incident closes, and its CAD record can go the instant it is final. Reporting that happens as incidents close, rather than as a separate monthly chore, keeps the backlog from building and keeps the data closest to the events it describes. On a busy shift, the difference between reporting that flows automatically and reporting that waits for a clerk is the difference between a file that is current and one that is perpetually behind.

## Where CAD systems usually fall short

Fire and EMS reporting is often a separate manual step done after the fact. A clerk opens the reporting software, finds the incident, and types in times that CAD already recorded, so the numbers can quietly disagree between the two systems. When standards change, as NFIRS to NERIS and across NEMSIS versions, agencies can be left on an old export format, or dependent on a vendor to ship an update before they can file.

Where CAD and the ePCR do not talk, the dispatch section of every patient-care report is retyped by the crew, which is both slow and error-prone at the worst possible time. And many products treat the reporting integration as a premium add-on, licensed separately, so a small department pays again for the privilege of meeting a mandate.

## How bitdispatcher does it

For fire, NERIS is set up in Admin, Integrations with the department's **NERIS ID** (FD and eight digits), a CAD integration's client ID and secret from NERIS, and the dispatch center's name. With **Send each fire or EMS incident when it closes** turned on, every closed incident that has a fire or EMS call type or unit is sent to NERIS as a CAD record, the call, unit and clear times, the place, the disposition and the notes, with police units left out. NERIS keeps the record pending until the fire department completes it. A **Check the last closed incident** action asks NERIS to validate one, so an administrator can confirm the connection is working. Incidents can also be sent by hand rather than automatically on close.

For EMS, the patient-care report is produced per unit. **NEMSIS** on an EMS unit of the incident downloads that unit's response as a **NEMSIS 3.5** record, ready to import into any ePCR that takes NEMSIS files. With a patient-care report system configured in Admin, Integrations, Patient-care reports, **Send** posts it there, and **automatic** sends each EMS unit's response when it clears. What the ePCR system answered is recorded on the incident. The record carries the dispatch, times and scene only; the crew completes the patient sections in their own system. The dispatch reason is the call type's NEMSIS code, set in Admin, Call types, or suggested from the call type's name.

## Why bitdispatcher fits

Both reports are built from the same [ledger](/manual/#ledger) that records the dispatch in the first place, so the times in a NERIS record or a NEMSIS file are the times the center actually logged, not a retyped copy. Reporting is part of the one program, set up in the browser, with no separate records server and no per-seat reporting license. Sending on close, by hand or automatically, means a shift's work reaches the national systems without a clerk reopening every incident. And because the standards live at the boundary, an agency can report to NERIS and hand off NEMSIS records without changing how its dispatchers work.

## What has been tested, and what has not

Both integrations are **tested against stand-ins, not the real systems**. NERIS's test environment needs an agency's own credentials, so bitdispatcher has been exercised against a stand-in rather than NERIS itself. The NEMSIS record has been checked against the **NEMSIS 3.5.1 schema** for the sections it carries, and sent to a stand-in, not to a vendor's ePCR system.

bitdispatcher is version 0.0.0 and not yet in service anywhere, and nothing here is certified. The ePCR system itself, and any NERIS or state onward submission, remains the agency's responsibility with its own licensed tools. These are the honest limits: the standards are followed and the records are built to them, but they have not been accepted by a live national endpoint.

## Try it

On the [live demonstration](https://demo.bitdispatcher.com), sign in as an EMS or fire dispatcher, close an incident with an EMS unit, and download its NEMSIS record from the unit; the fire reporting settings and the per-unit NEMSIS action are in the admin and incident screens. The relevant manual chapters are [Patient-care reports (NEMSIS)](/manual/#epcr), [Integrations](/manual/#admin-integrations), [Agencies](/manual/#admin-agencies) and [Incident reports (RMS)](/manual/#rms).

Related: [NIBRS incident-based crime reporting](/tech/nibrs-reporting/), [Staffing and qualifications](/tech/staffing-and-qualifications/), [The append-only event ledger](/tech/event-ledger/), [Public records and FOIA requests](/tech/records-requests/)
