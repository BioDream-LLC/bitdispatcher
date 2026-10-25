---
title: Station alerting and paging the crews who respond
tile: Station alerting and paging
summary: Tone out the firehouse and page volunteers by text, email, push and webhook, with responders answering from a link.
description: How bitdispatcher tones out firehouses, reads the call aloud, and pages volunteers and off-duty crews by text, email, push and webhook with live answers.
icon: bell
---

## What it is

Station alerting and paging are the two ways a dispatch floor reaches the people who put apparatus on the road. They answer the same question from opposite ends: how does a crew learn, in the first seconds, that a call is theirs.

**Station alerting** is the signal into the firehouse. A unit is dispatched, and the station where it sits in quarters lights up, sounds a tone, and announces the call over the house speakers. This is the oldest part of fire dispatch, older than computers: the plectron receiver, the house gong, the voice on the box. Modern alerting keeps the shape and adds timing, ramped audio that will not startle a sleeping crew into a cardiac event, and a record that the station was alerted and acknowledged.

**Paging** is the signal to people who are not in the station at all: volunteer firefighters and off-duty crews carrying a phone. A page goes to a group, and each person answers whether they are coming, where to, and when. Volunteer and combination departments live or die on this. If six people get paged and the dispatcher cannot see how many are actually rolling, the incident commander is planning blind.

Neither of these is governed by a single national standard the way caller location or crime reporting is. Alerting hardware is a field of competing vendor controllers; paging has grown from numeric pagers through SMS to smartphone apps and services like IamResponding and Active911. The common thread is that both must be fast, both must be recorded, and both must tell the dispatcher what happened.

## How it works

Station alerting splits into the signal and the sound. The signal is a message from the CAD to a controller or a screen in the station, triggered the instant units are dispatched. The sound is what the crew hears: a tone sequence to wake and orient them, then a voice announcement of the call, usually synthesized or pre-recorded phrases stitched together.

Good alerting ramps the tone rather than blasting it, repeats the essential facts, and shows the call in large type for the crew walking to the apparatus. It distinguishes priority: a working structure fire should not sound like a lift-assist. And it closes the loop: the station acknowledges, and dispatch knows the house is awake and moving.

Paging works on delivery and answer. A paging group lists its members and the channels pages travel on: SMS through a carrier gateway, email, mobile push, a webhook to another system, or formatted email into a responder service. A page carries enough to decide, usually the call type, the address, and a short summary. Each recipient gets a way to answer: responding to the station, going straight to the scene, or not available, often with an estimated time. The CAD collects those answers and shows the incident commander a count and a roster.

The hard parts are reliability and identity. Did the page actually deliver, or did the gateway drop it. Is the person answering really the person paged, without making them log in during an emergency. And can the dispatcher pull more people without starting over.

## Why it matters in a 911 center

On a career fire floor, station alerting is about seconds and about safety. Turnout time is measured, and a slow or confusing alert costs it. Ramped audio matters because crews alerted dozens of times a night at full volume suffer for it. The read-aloud matters because a firefighter pulling on gear is not looking at a screen; the call has to reach the ear.

On a volunteer or combination floor, paging is the whole response. The dispatcher sends the page and then waits to learn whether anyone is coming. When the answers come back as a live count, the dispatcher and the chief can decide early whether to page a second department or strike a mutual-aid box, instead of discovering an empty station after the apparatus should already be rolling.

Both feed the record. When a call goes bad and the review asks when the station was alerted and who responded, the answer has to be in the timeline, not in someone's memory.

## Where CAD systems usually fall short

Station alerting is often a separate product bolted on, with its own server, its own contract, and its own failure modes. When the CAD and the alerting system disagree about what was dispatched, the station gets a stale or wrong alert. Configuration lives in a vendor's tool rather than beside the units and stations in the CAD.

Paging is frequently an afterthought or a third-party subscription that the CAD merely nudges. The dispatcher sees that a page was sent but not who answered, so the live picture of who is coming lives in a separate app the dispatcher is not watching. Delivery failures vanish silently. And tying pages to run cards, so the right group is paged automatically when its units are due, is often manual.

## How bitdispatcher does it

Station alerting is built in. When units are dispatched, their stations are toned out where an administrator has turned it on in Admin › Integrations. In the firehouse, a screen on a read-only account at `#/station` shows the station's units and their statuses. On an alert it plays **rising tones**, twice as many for a priority 1, then **reads the call aloud twice**: the units, the call type and the street. The call shows in large type with priority, type, address, the units and the latest priority notes. **Acknowledge** stops it and writes *ST1 acknowledged the alert* on the incident; **Read it again** repeats it. Because browsers play sound only after a press, the crew presses **Press to turn the sound on** once when the screen opens, and the console idle lock leaves a station screen alone.

`TONE` on the command line tones out the stations again (`TONE E1`, `TONE #26-000123`), and every tone-out lands on the incident's timeline. A hardware alerting controller from a vendor can receive the same alert at its own address.

Paging is also built in, covered by the roster and the paging groups. A **paging group** names its **members** from the agency roster and its **gateways**: a text by **Twilio**, an **email**, a push to phones by **ntfy**, a **webhook**, or **IamResponding and Active911**, both of which take dispatch by email in their documented formats. A group can page **automatically** when one of its units or stations is dispatched, or when an incident of one of its call types gets units; each group is paged once per incident by its rule.

A page's subject is the call type and address; its text is the incident summary and units. Each person gets a link to answer **Responding to the station**, **Going straight to the scene**, or **Not available**, with when they expect to arrive. There is no sign-in: the link is the key and works only for that person and that page. On the incident, **Responders** shows how many were paged and responding, each person's answer and minutes away; **Page** pages more groups, and `PAGE <group> / text` does it from the command line. A page nobody answers after five minutes goes to the supervisor's exceptions, and failed deliveries land on the timeline. Administrators must set the **address responders open** to the address phones can reach the server at; without it, pages go out with no answer link.

## Why bitdispatcher fits

Alerting and paging are not separate products here. They are part of one program, configured in the same browser as the units, stations and run cards they depend on, so the group that pages and the station that tones out are defined right next to the units they belong to. There is no second server and no per-seat licence.

Because everything runs through the one system, the dispatcher sees the whole picture on the incident: the station acknowledged, the page sent, the responders answering, the count climbing. Every alert, tone-out, page and answer is written to the append-only, hash-chained ledger, so the question of when the house was alerted and who was coming has an answer that cannot be quietly changed.

Paging reaches people through channels an agency already uses, text, email and push, rather than forcing one app. IamResponding and Active911 are reached in their documented email formats, so a department already on either keeps it.

## What has been tested, and what has not

Paging is tested in the browser and against stand-ins for Twilio, ntfy, email and a webhook; the demonstration uses a simulated pager whose simulated responders answer. It is **not certified by IamResponding or Active911**. Station alerting controllers are tested against a stand-in, not against a specific vendor's controller. The priority tones are measured as rendered by Chromium's audio engine for rhythm, level and absence of clipping, and have not yet been judged by ear on a real dispatch floor. Twilio, like any carrier gateway, needs the agency's own account.

## Try it

On the [live demo](https://demo.bitdispatcher.com), dispatch a fire unit and watch the station screen, or open the paging view and page a group to see the simulated responders answer. The relevant manual chapters are [The station screen and toning out](/manual/#station), [Paging responders](/manual/#admin-paging), and [Personnel and crews](/manual/#admin-personnel).

Related: [Staffing and Qualifications](/tech/staffing-and-qualifications/), [Public Alerting with CAP and IPAWS](/tech/public-alerting-cap-ipaws/), [Messages and BOLOs](/tech/messages-and-bolos/)
