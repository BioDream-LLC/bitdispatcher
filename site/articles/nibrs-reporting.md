---
title: NIBRS incident-based crime reporting, from the record out
tile: NIBRS reporting
summary: Incident reports with the FBI's NIBRS data elements checked, supervisor approval, and Virginia's monthly IBR file in one click.
description: How bitdispatcher builds NIBRS-ready incident reports, checks the FBI data elements, drafts a narrative and writes Virginia's monthly IBR file.
icon: file
---

## What it is

**NIBRS** is the FBI's National Incident-Based Reporting System: the way American police agencies report crime to the national Uniform Crime Reporting program. It replaced the old **UCR Summary** system, in which an agency reported monthly tallies and only the single most serious offense in an incident was counted under the "hierarchy rule." The FBI set January 1, 2021 as the date it would collect crime data in NIBRS only, retiring Summary reporting after decades.

The difference is the word *incident-based*. Instead of a count of robberies this month, NIBRS carries the whole incident: every offense in it, each victim, each known offender, every arrestee, the property taken or recovered, the weapon, the location type, the relationships between victims and offenders. An incident becomes a set of linked records rather than a mark in a column. This produces far richer data: multiple offenses are all counted, and analysts can ask questions the old tallies could not answer.

NIBRS defines **Group A** offenses, reported in full incident detail, and **Group B** offenses, reported only when an arrest is made. Each data element has codes, allowed values and **edits**, the FBI's validation rules that a submission must pass. States that collect their own incident-based data, like Virginia, add their own required fields on top of the national set.

## How it works

A NIBRS submission is a structured file, historically a position-based flat file where each line is a segment of a fixed layout: an administrative segment for the incident, then offense, property, victim, offender and arrestee segments, each a string of fields in known character positions. The national specification is versioned; agencies file to the version their state accepts.

The data elements are specific and interdependent. A victim segment carries the victim type, and for a crime against a person, the injury and the relationship to each offender. An offense segment carries the offense code, whether it was attempted or completed, the location type, the weapon or force, and the number of premises entered for a burglary. A property segment carries a loss type, the property description, value and quantity, and for drugs the type and measured amount.

The **edits** are where most agencies lose time. The FBI's rules reject a file for internal contradictions: an offense that requires a victim with none attached, a property loss inconsistent with the offense, a value out of range, a mandatory field left blank. A single bad incident can bounce a whole monthly file. The practical task for an agency is to catch those errors at the desk, when the officer still remembers the call, rather than weeks later when the state rejects the batch.

## Why it matters in a 911 center

For a dispatch center that also handles records for its agencies, NIBRS is a monthly deadline with teeth. Virginia agencies upload to the Virginia Crime Repository by the 15th of the following month, and a file that fails validation is not a filed file. Funding, grant eligibility and the state's crime picture all rest on reporting being complete and on time.

The quality of the data also depends on capture at the source. If an officer writes a free-text narrative and a records clerk later codes it into NIBRS fields, meaning is lost and errors creep in. The closer the structured data is captured to the incident, the better it holds up. A report that already knows the incident's location, time and units, and that asks only for the fields the specific offense needs, gets more right with less effort.

## Where CAD systems usually fall short

NIBRS is often an afterthought bolted onto a separate records system. The officer writes the report in one product, the data is exported to another, and the validation happens somewhere downstream, so the first time anyone sees an edit failure is after the file is rejected. Fixing it means reopening cases and re-keying fields long after the facts are fresh.

Many systems also present every field for every offense, so the writer wades through properties and weapons that do not apply, and still misses the one mandatory field that the specific offense needed. State-specific additions are frequently unsupported, forcing a second manual step or a third-party conversion. And where the CAD and the report live in different databases, the incident's own facts, times, location, units, are typed a second time, inviting them to disagree with the dispatch record.

## How bitdispatcher does it

Incident reports are written at the **Incident reports (RMS)** screen, or with **Report** on an incident, which fills in the incident's location and time. A crew writes its own report from the car on the mobile terminal. The report has sections for when and where, offenses, people, property, vehicles and the narrative. Codes are shown in words and picked in one click, and the screen asks only for what the offense needs: a weapon for an assault, the method of entry for a burglary, the drug for a seizure, the injury and the relationship to each suspect for a crime against a person. A draft saves itself a moment after typing stops.

**Before it is submitted**, the screen lists what is still missing for incident-based reporting: the FBI's NIBRS data elements and edits, with Virginia's additions, property quantity, the license plate reader indicator, drug types. Submit is available only once that list is empty, so the edits are satisfied at the desk, not after a state rejection. A supervisor other than the author then approves the report, or returns it with what to fix. Every save and decision is an event in the [ledger](/manual/#ledger).

The monthly file is one action. A supervisor opens **State IBR file**, chooses the month, and presses **File**. bitdispatcher writes the month's approved reports in **Virginia's incident-based reporting format**, the FBI's position-based flat file with Virginia's additions, to the 2025.2 specification: Group A incidents with their offense, property, victim, offender and arrestee segments, Group B arrests, or a zero report when there is nothing to send. The file is then uploaded to the Virginia Crime Repository by the 15th of the next month. Reports that fail the checks are left out and listed, so one bad case does not sink the batch. Making the file is itself a ledger event, with its SHA-256. A **FBI national** format writes the same reports in the FBI's own layout, without Virginia's additions, for states that take the national file.

The narrative gets a running start. **Draft from the incident**, on a report written from an incident, fills the narrative with a first draft built from the record: the type, place, times, units, people, vehicles and notes. The writer then corrects and finishes it; the draft is a starting point, not the report. Where the agency has configured a language model, the same facts can be used to draft the narrative instead, again for the writer to correct. The settings need the agency's nine-character ORI, set in Admin, Settings.

## Why bitdispatcher fits

The report and the dispatch record are the same record. Because the incident's location, times and units come straight from the [ledger](/manual/#ledger), the report does not re-key what CAD already holds, and it cannot quietly disagree with the dispatch history. The validation runs where the writer is, so edit failures are fixed while the call is fresh. The state file and the national file are both produced from the approved reports, with no separate conversion tool. And because everything, every save, every approval, every file generated, is a recorded event, the agency can show exactly what was reported and when.

It is also free and open. There is no per-seat records module to license and no separate server to stand up; the report screen is part of the one program, set up in the browser.

## What has been tested, and what has not

bitdispatcher's NIBRS output in the FBI's national layout has been checked with **SEARCH's NIBRS Pre-Certification Tool** validator, which applies the FBI's edits (`scripts/nibrs-validator.sh`), and the Go test suite covers the NIBRS file and the Virginia file. Passing the validator is **not** FBI or state certification.

The honest limits matter here. The **Virginia IBR file has not yet been accepted by the State Police**; before a first production upload, the Virginia State Police ask for a test file by email. bitdispatcher is version 0.0.0, not yet in service anywhere. The optional language model for the narrative draft is the agency's own choice and sends the incident's facts to that model; the built-in draft uses only the record. None of this is a substitute for the agency's own review of what it reports.

## Try it

On the [live demonstration](https://demo.bitdispatcher.com), sign in with a records or supervisor duty, open an incident with a police call type, write the report, watch the missing-elements list clear, and draft the narrative from the record. Then open **State IBR file** to see the monthly file built. The relevant manual chapters are [Incident reports (RMS)](/manual/#rms), [Operations report](/manual/#reports), [Records and requests](/manual/#records) and [The event ledger](/manual/#ledger).

Related: [NCIC inquiries through the state switch](/tech/ncic/), [Fire and EMS reporting with NERIS and NEMSIS](/tech/neris-and-nemsis/), [Public records and FOIA requests](/tech/records-requests/), [The append-only event ledger](/tech/event-ledger/)
