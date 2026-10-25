---
title: Warning the public with CAP and the IPAWS profile
tile: Public alerts (CAP, IPAWS)
summary: Write a public warning once and send it as CAP 1.2 to IPAWS, sirens, alert software and subscribers in English and Spanish.
description: How bitdispatcher composes OASIS CAP 1.2 messages to the FEMA IPAWS profile, with WEA limits, bilingual text, map areas and second-supervisor approval.
icon: siren
---

## What it is

Public alerting is how a dispatch center warns the people in harm's way: shelter in place, evacuate, a hazardous spill, a missing child, police activity, or the 911 lines being down. It is the step beyond dispatching responders. Instead of sending help to one address, the center sends a message to everyone inside an area.

The standard underneath it is the **Common Alerting Protocol (CAP)**, an OASIS specification. **CAP 1.2** was approved by OASIS in 2010 and is the version the United States alerting infrastructure runs on. CAP is an XML format that describes an alert once, in a structured way, so that many different warning systems, sirens, broadcast equipment, phones, websites, can each render it in their own medium from the same source. It carries the event, the severity, the urgency, the certainty, the affected area as geocodes or polygons, and the message text, optionally in more than one language.

In the United States the national pipe for these alerts is **IPAWS**, the Integrated Public Alert and Warning System, run by FEMA. IPAWS takes CAP messages that conform to the **FEMA IPAWS CAP v1.2 USA Profile** and fans them out to **Wireless Emergency Alerts (WEA)** on mobile phones, to the **Emergency Alert System (EAS)** on radio and television, to NOAA weather radio, and to a public feed. An alerting authority does not talk to the phone carriers directly; it writes a conforming CAP message, and IPAWS does the distribution.

WEA has a hard constraint worth knowing. The original wireless alert was limited to **90 characters**. Since the 2019 enhancements, carriers and handsets can carry a **360-character** message, but the 90-character form remains in use, so a well-built alert supplies both.

## How it works

Composing an alert means filling in a CAP message correctly. The author chooses an event code, SPW for shelter in place, EVI for evacuation, CAE for a child abduction, TOE for a 911 outage, and so on, and writes the human text: a headline, what is happening, and what to do. The area is defined either as geocodes, the SAME (Specific Area Message Encoding) six-digit county codes used by EAS and weather radio, or as a polygon drawn on a map for tighter targeting.

The message then goes to its destinations. For IPAWS, the CAP is handed to the agency's alert origination software, which is authenticated to FEMA and relays it to the national system. For local systems, sirens, outdoor warning, mass-notification software, the same CAP can be POSTed directly. And for the public and for apps, a feed publishes the alerts currently in force.

The guardrails are where public alerting earns its reputation for care. A WEA goes to every capable phone in the area with no way to recall it from the handsets that already buzzed. So alerts are checked before they go: is the CAP valid, is the area right, how many people does it reach. Many authorities require a second person to approve. And an alert in force must be updatable and cancelable, because the all-clear matters as much as the warning.

A detail that trips up new alerting authorities: sending to IPAWS is not a technical connection alone. It requires a **Memorandum of Agreement (MOA)** with FEMA and a **Collaborative Operating Group (COG)** identity, plus certified alert origination software. Writing a valid CAP message is necessary but not sufficient.

## Why it matters in a 911 center

When a tanker overturns and a plume is drifting toward a subdivision, the difference between a timely shelter-in-place order and a late one is measured in exposures. The center that can warn the public from the same console where the incident is running, in the same minute, does not lose time handing the job to someone in another building.

Bilingual reach matters for the same reason. A warning only works if the person reading it understands it. In much of the country that means English and Spanish, side by side, written before the emergency rather than translated under pressure.

Accountability matters because public alerts are powerful and occasionally wrong. When an alert goes out in error, the review asks who wrote it, who approved it, what area it covered, and when the correction went. Those answers must be in the record.

## Where CAD systems usually fall short

Public alerting is usually a separate product with its own login, bought and operated apart from the CAD. The dispatcher leaves the incident, opens another tool, retypes the location, and composes the alert disconnected from the call that prompted it. The link between the incident and the warning is lost.

Separation also breaks the record. The alert lives in the notification vendor's logs, not on the incident timeline, so reconstructing the sequence later means stitching two systems together. Approval workflows vary in rigor, and the character limits and profile rules of WEA and IPAWS are easy to violate when the composing tool does not enforce them.

## How bitdispatcher does it

Supervisors warn the public from the console. The **megaphone** in the status bar, or **Alert the public** on an incident, opens the composer. The author first marks it a **Real alert**, an **Exercise** or a **Test**; an exercise or test is marked as one everywhere it goes. Choosing the **kind of alert** by its SAME event code fills the wording to start from, in English and Spanish.

The author writes a **headline**, **what is happening** and **what to do**. The **phone alert** field is what phones show first, held to **90 characters** with the counter turning red past the limit, and an optional **longer phone alert** of **360** characters. **Also in Spanish** adds a Spanish version for people on the lists who chose Spanish. The **where** is the area around the incident (a quarter mile to two miles), an area drawn on the map from Admin › Geofences, or **everyone** in the jurisdiction by its county codes. **Expires after** sets one to 24 hours.

**Check** shows the CAP message it builds, flags anything wrong, and estimates how many people it reaches. **Send** asks once more with the exact phone text and area. With **two-person approval** on, the alert waits until a second supervisor presses **Approve**; the author can withdraw it, and the second supervisor can reject it with a reason. An alert in force shows red on the map (amber for a test), appears in the public alert feed, and can be updated or canceled, the all-clear in one click. Every alert, approval and delivery is in the ledger and on the incident's timeline.

Settings live in Admin › Public alerts: the agency's CAP **sender** ID and address, its **jurisdiction** and six-digit **SAME county codes** (for example 051770 for Roanoke city), the **time zone** carried in CAP times, **two-person approval**, the **public alert feed** (an Atom feed linking to each message and an IPAWS-style feed carrying them inside, for sirens, EAS encoders and apps), **CAP receivers** each alert is POSTed to with a Bearer token, and **subscriber lists** reached through the paging gateways, where a person with a location given gets only alerts whose area contains it.

## Why bitdispatcher fits

The alert is composed on the same screen as the incident that prompted it, by the same supervisor, in one free program with no separate contract. The area can be the circle around the incident already on the map. The English and Spanish text, the WEA limits, and the CAP profile are enforced in the composer, so an invalid message is caught before it is sent. And because the whole sequence, authoring, approval, sending, updating, canceling, is written to the hash-chained ledger and the incident timeline, the record of a public warning is complete and tamper-evident.

## What needs an IPAWS MOA, and how it was tested

Sending to IPAWS itself, the Wireless Emergency Alerts and the EAS, needs **FEMA's agreement** (an MOA and a COG) and the agency's **own alert origination software**. bitdispatcher writes its messages **to the IPAWS profile** for that software to relay; it does not connect to IPAWS directly.

The messages are checked against **Google's CAP library with its IPAWS profile** (`scripts/cap-validator.sh`) and against **EAS Station's feed reader** (`scripts/eas-station.sh`). That is validation against a conformance checker and a feed reader, not certification. **No message has been sent to IPAWS.** Local CAP receivers and the feeds are tested against stand-ins. Direct siren or mass-notification hardware requires that system's own endpoint and token.

## Try it

On the [live demo](https://demo.bitdispatcher.com), sign in as a supervisor, press the megaphone, and compose a test alert to see the CAP message, the character counters and the map area. The manual chapters are [Alerting the public](/manual/#public-alerts) and [Public alerts: settings and lists](/manual/#admin-publicalerts).

Related: [Station Alerting and Paging](/tech/station-alerting-and-paging/), [The Public Incident Feed](/tech/public-incident-feed/), [Transcripts and Translation](/tech/transcripts-and-translation/)
