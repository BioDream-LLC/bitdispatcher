---
title: A public incident feed that protects the caller
tile: Public incident feed
summary: Share open incidents with a website or apps like PulsePoint using hundred-block addresses and coarse points, never callers or notes.
description: How bitdispatcher publishes a public incident feed for websites and apps like PulsePoint, with hundred-block addresses, coarse points and privacy by design.
icon: broadcast
---

## What it is

A public incident feed is a stream of what the dispatch center is working on right now, published for people outside the center: a county website's active-calls page, a transparency portal, or an app on a resident's phone. It answers a civic demand that has grown steadily: people want to know what is happening in their neighborhood, and agencies increasingly want to show them, within limits.

The most familiar consumer of such a feed is **PulsePoint**, an app that shows nearby incidents and, in its best-known feature, alerts trained bystanders to a nearby cardiac arrest in a public place so they can start CPR before the ambulance arrives. That CPR-alert function has a direct, documented effect on survival, because minutes without compressions cost lives. The same feed also drives ordinary situational-awareness displays and public dashboards.

There is no single national standard for a public incident feed; it is a documented data format that each consumer agrees to. What defines a good one is not the format but the discipline around it. A public feed is the one place where a dispatch system deliberately shares data outward, so it must share only what is safe to share, and it must be off until someone decides to turn it on.

The hard part is the privacy balance. The public interest in knowing that there is a structure fire on the 400 block of Main Street is real. The private interest of the person who called about a domestic dispute, or an overdose, or a mental-health crisis, is also real, and far more fragile. A feed that leaks a caller's name, an exact address, or a dispatcher's notes does lasting harm. The whole design problem is drawing that line and holding it.

## How it works

A public feed publishes a filtered, coarsened view of open incidents. Filtering decides which incidents appear at all; many agencies exclude sensitive call types entirely. Coarsening blurs what remains: an exact house number becomes a hundred-block, and a precise latitude and longitude becomes a point rounded to a neighborhood-scale radius rather than a doorstep.

What never appears is as important as what does. Caller identity, callback numbers, dispatcher and responder notes, medical detail, and the identities of people involved stay inside the center. The feed carries the type of incident, a coarse location, and a time, and little else.

Distribution is usually a pull feed, a URL that consumers poll, or a push to a partner that ingests it. Either way the feed exposes only current activity and drops incidents as they close, rather than building a public history.

The governing principle is that a public feed is opt-in at every level: off by default, turned on deliberately, and configured conservatively, because the cost of over-sharing is paid by the most vulnerable callers. The default should fail safe. If nobody has made a decision, the feed publishes nothing, because the harm of accidental exposure is permanent and the harm of a quiet feed is merely that a website is blank until someone acts.

It helps to think about who the consumers are. A public dashboard wants breadth and refresh, and tolerates coarse detail. A CPR-alert app wants a narrow slice, public-place cardiac arrests, delivered fast. A neighborhood-awareness app wants type and rough location. None of them needs, or should get, the caller's name or the dispatcher's notes, so a single conservative feed serves all of them without a special arrangement for each.

## Why it matters in a 911 center

For the public, a feed satisfies a legitimate appetite for awareness and transparency without a records request for every curiosity. A resident seeing an active road closure or a brush fire nearby can make a sensible decision without calling the center and tying up a line.

For bystander CPR, the stakes are concrete. An app that alerts a nearby trained person to a cardiac arrest in a public place can put hands on a chest in the gap before EMS arrives, and that gap is where survival is won or lost. A center that can feed such an app is contributing directly to outcomes.

But the center also carries the duty of care to the caller who did not consent to be publicized. The reason the feed matters is also the reason it must be conservative: the same pipe that informs the public can expose a victim. Getting the balance right is the point.

## Where CAD systems usually fall short

Public feeds are frequently an afterthought, exported with more detail than is safe because the coarsening was never built, so exact addresses or caller-adjacent detail slip out. Others swing the other way and share nothing, leaving agencies to satisfy public demand with manual postings. The on/off control is sometimes all-or-nothing rather than a per-field, per-call-type decision an administrator can reason about. And where a feed exists, it is often a bespoke integration for one consumer rather than a clean, documented stream.

## How bitdispatcher does it

bitdispatcher's public incident feed is configured in Admin › Integrations. It publishes open incidents for the public or for apps like PulsePoint, and it is built around the privacy line: **hundred-block addresses**, points coarsened **to about 100 m**, and **no callers or notes**. There is an optional token and push address, and a **See what it shows** control to inspect exactly what the feed exposes before it is turned on.

Most important, the feed is **off until turned on**. Nothing leaves the center until an administrator deliberately enables it, inspects what it would publish, and accepts it. This matches the project's rule for every outward-facing provider: looking a place up, or publishing it, tells someone outside where the incident is, so it stays off until an administrator decides otherwise.

The feed is a documented format that apps such as PulsePoint can consume. It carries incident type, the coarsened location and timing, and omits the fields that would identify or endanger a caller. Because the rest of bitdispatcher is one program with a hash-chained ledger, the decision to enable the feed and its configuration are themselves recorded, so there is an account of when sharing was turned on and how it was set.

The coarsening is not a cosmetic rounding applied at the edge. A hundred-block address and a point to about 100 m are what the feed holds, so there is no precise location anywhere in the published data to leak through a misconfiguration or a careless consumer. The exclusion of callers and notes works the same way: those fields are never placed in the feed, rather than being present and filtered, so there is nothing to accidentally expose. This is the difference between a feed that is private by construction and one that is private only if every setting is correct.

## Why bitdispatcher fits

The privacy discipline is built into the feed rather than left to the operator to remember: coarse addresses, coarse points, and no caller data or notes are what the feed produces, not options to be gotten right. It is off by default, inspectable before it goes live, and enabled as a deliberate act that the ledger records. It is part of the one free program, so there is no separate integration server and no per-consumer bespoke build. And it draws from the same incidents the dispatcher is working, so the public view is consistent with the operational one without a second pipeline to keep in sync.

## What has been tested, and what has not

The public incident feed is **a documented feed, not PulsePoint's partner interface**, and it is tested against a stand-in rather than against PulsePoint itself. That means the format, the coarsening, the exclusion of callers and notes, and the off-by-default behavior are exercised in testing, but a live partnership with PulsePoint or any other consumer has not been established or certified. The feed is off for every center until an administrator turns it on and inspects what it shows.

A note on scope: sharing incidents with neighboring dispatch centers is a different mechanism entirely and is covered in its own article. The public feed is one-way and coarsened for the public; CAD-to-CAD is a two-way exchange of full incident data between agencies using NENA EIDOs, with its own addressing and tokens.

## Try it

On the [live demo](https://demo.bitdispatcher.com), an administrator can open Admin › Integrations and use See what it shows to inspect the feed's output. The manual chapters are [Integrations](/manual/#admin-integrations) and, for the separate neighboring-agency exchange, [Incidents with neighbouring agencies](/manual/#cad-to-cad).

Related: [Public Alerting with CAP and IPAWS](/tech/public-alerting-cap-ipaws/), [Security](/tech/security/), [The Event Ledger](/tech/event-ledger/)
