---
title: Public records requests for 911 data, redacted and released
tile: Records requests
summary: FOIA requests for 911 data: redactions proposed by the agency's rules, accepted one by one, released only on approval.
description: How bitdispatcher answers public records and FOIA requests for 911 data with rule-proposed redactions, per-item approval, a leak check and working-day due dates.
icon: file
---

## What it is

A 911 center's records are public records, and the public has a legal right to ask for them. In the United States that right runs through state **FOIA**, or freedom-of-information, laws (the federal FOIA covers federal agencies; state and local bodies answer to their own state acts, such as the Virginia Freedom of Information Act). A citizen, a journalist, an attorney or an insurer asks for the record of an incident, a call, a response, and the agency must respond within a legal deadline, usually counted in working days.

The hard part is not finding the record; it is releasing the right thing. A 911 record contains material that is genuinely public and material that is exempt or must be withheld: a caller's phone number, personal details of victims, medical information, an open investigation, the home address of a protected person. The agency's job is to release what is disclosable and **redact** what is not, applying its own legally reviewed rules, and to do so without accidentally releasing something it meant to withhold.

Redaction for a records release is permanent and one-directional: the released copy must not carry the hidden text in any form a reader could recover, and the act of deciding what to withhold, on what legal basis, has to be defensible if challenged.

## How it works

A records request names a requester, a contact, what is being asked for, and which incidents it covers. The clock starts, and the agency tracks the deadline, which is counted in **working days** rather than calendar days, since weekends and holidays do not count against most FOIA timelines.

The agency assembles a release copy from the record. It decides which view of the incident to release, a short summary or the full detail, and then works through the material deciding what to withhold. Each proposed redaction needs a **legal basis**: the specific exemption in the statute that justifies withholding that passage. The agency's exemptions are not universal; they are its own, reviewed by its counsel, matched to its state law and its record types.

When the decisions are made, they are applied. The release copy has the withheld passages removed permanently, and before it goes out it is checked to make sure none of the withheld text leaked through into the copy that will be handed over. Only then, on a person's explicit authorization, is the copy released, and the agency keeps a record of exactly what was released and that it was approved.

## Why it matters in a 911 center

A missed FOIA deadline is a legal problem, and the deadlines are short. A center that cannot see, at a glance, how many working days are left on each open request will miss one. Tracking the due date in working days, and flagging the ones that are close or overdue, is the difference between a routine response and a statutory violation.

A redaction mistake is worse. Releasing a caller's phone number, a victim's medical detail, or the identity of a protected person is a harm that cannot be taken back once the copy is out. The process has to make it hard to release the wrong thing: propose carefully, decide deliberately item by item, check for leaks, and release only on a deliberate approval. And because records work is sensitive, the center needs to show that every review and every release was authorized and logged, not something a clerk did quietly.

There is also the everyday need simply to read a record back, for a review, a subpoena, an internal question, without disturbing it. Looking at a closed incident should never risk reopening or changing it.

## Where CAD systems usually fall short

Redaction is often done by exporting the record to a document and blacking out text in a separate editor, where "redaction" can mean a black rectangle laid over text that is still present underneath and trivially recovered. That is how agencies accidentally release what they meant to hide. Where the CAD has no records workflow at all, the whole process is manual, off to the side, and unlogged, so the agency cannot prove what it released or on whose authority.

Due-date tracking is frequently a spreadsheet, counted in calendar days or not at all. Exemptions are hardcoded or absent, giving the agency no way to apply its own counsel-reviewed rules. And reading a record back for review sometimes means opening it in a way that can change it, blurring the line between the archived truth and the working copy.

## How bitdispatcher does it

The records area reads a finished incident and answers requests for it, all from the same tamper-evident [ledger](/manual/#ledger) the rest of the system writes to. Reviewing a record shows one incident four ways, and opening or exporting any view is itself recorded:

| View | What it shows |
|---|---|
| As first entered | The incident as first created, before any later change |
| Summary | The short account |
| Detailed | Everything on the incident |
| Full audit | Every ledger event, with a hash-chain check |

Printing or downloading text keeps a copy, and both are recorded. A closed incident can also be **corrected without reopening it**, and any review view can be read without disturbing the record.

A request names a requester, a contact, what is asked for, the incidents and whether the **summary** or **detailed** view is released. Each request shows the **working days left** until it is due, amber when close and rose when overdue, and the working time taken so far. The release then goes step by step:

1. **Assemble** builds the release copy from the chosen view and proposes redactions by the agency's **disclosure rules**; the original record is untouched.
2. For each incident, **accept or reject each proposed redaction** one by one, giving the **basis** required for anything withheld, or mark a passage by selecting it. A live preview shows the redacted copy.
3. **Save decisions** records every decision, accepted and rejected.
4. **Approve and release** is the moment a person authorizes the release: accepted redactions are applied **permanently**, and the copy is **checked for leaks** before it leaves. The released copy can then be downloaded, and its hash is shown.

The disclosure rules themselves are the agency's own. In Admin, Disclosure rules, nothing is hardcoded as exempt; a rule matches by category, words or a pattern, carries a required **legal basis**, and stays disabled until the agency's counsel has reviewed it. Records work needs the records duty, a supervisor or an administrator.

## Why bitdispatcher fits

The release is built from the same append-only [ledger](/manual/#ledger) that holds the incident, so what is released traces back to a verifiable record, and every review and release is logged. Redaction is permanent and leak-checked, not a rectangle over live text. Approval is an explicit, recorded act by an authorized person, so the agency can always show what went out and who allowed it. Due dates are counted in working days and flagged, so deadlines are visible. And because exemptions are the agency's own counsel-reviewed rules, the system proposes and people decide, rather than the software pretending to know the law. It is all part of the one program, set up in the browser.

## What has been tested, and what has not

The end-to-end records flow, review views and a records request, is covered by the Playwright browser tests, and opening a view and releasing a copy are exercised against a running instance with demo data.

The limits are important. The **sample disclosure rules and example guidance are placeholders, not legal advice or approved text**; the agency's counsel must review and enable each rule. An optional language model for redaction proposals is **off by default**, and when turned on it sends the record's text to that model. bitdispatcher is version 0.0.0 and not yet in service anywhere, and nothing here is certified. The software supports the process; the legal judgment of what to withhold remains the agency's.

## Try it

On the [live demonstration](https://demo.bitdispatcher.com), sign in with the records duty, open a closed incident's review views, then build a records request, work through the proposed redactions, and approve a release. The relevant manual chapters are [Records and requests](/manual/#records), [Disclosure rules](/manual/#admin-disclosure), [Views](/manual/#views) and [The event ledger](/manual/#ledger).

Related: [NIBRS incident-based crime reporting](/tech/nibrs-reporting/), [The append-only event ledger](/tech/event-ledger/), [Security](/tech/security/), [NCIC inquiries through the state switch](/tech/ncic/)
