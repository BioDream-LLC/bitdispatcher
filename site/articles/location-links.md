---
title: A link that lets a caller show 911 exactly where they are
tile: Location links
summary: When a call has no usable location, text the caller a link; one tap shares the phone's own GPS to the call.
description: How bitdispatcher sends a caller a location link that shares the phone's GPS to the call, with a hashed secret, a short life and a stop button.
icon: pin
---

## What it is

Most of the time a 911 call arrives with a location attached. A wireless call brings a device-based position from the carrier; a wired call brings the address on the line. But not always. A landline gives the billing address, not where the caller is standing. A **VoIP** call may carry an address that was set once and never updated. A device-based fix can be slow, wide or missing indoors. And some callers simply do not know where they are: a hiker, a passenger, a child, someone new to the area.

A **location link** is for exactly those calls. When a call has no usable location, the call-taker sends the caller a short web link. The caller opens it on their phone, presses one button, and the phone's own satellite position goes straight to the call, and keeps going as the caller moves. There is no app to install and no sign-in. It is a last resort for the hardest calls to place, not a replacement for the location that usually arrives on its own.

## How it works

On the call, the call-taker chooses **Send location link**. If the call can take a text, bitdispatcher texts the link through **BitVoice**, the call-handling side, with a plain default message:

> 911: to show us exactly where you are, open this link and press Share: {link}

If the call cannot take a text, the same link is shown on screen for the call-taker to read out.

The link points at a tiny page served by bitdispatcher itself. That page is deliberately small: no sign-in, no app, its own short script, and a strict **content security policy** that lets the page load nothing but its own code and talk to nowhere but bitdispatcher. The address the link uses is the public address an administrator sets under **Paging groups**, the same outside-facing address used for other things a member of the public may open.

When the caller presses **Share my location**, the page asks the phone for permission in the browser's own prompt. If the caller allows it, the page uses the browser's `watchPosition` with high accuracy turned on. Each fix, the latitude, longitude, accuracy and altitude, is sent back to the call and lands on it marked **Shared by the caller (phone's location, by link)**, so a dispatcher can see at a glance that this position came from the caller's own phone rather than the carrier or the address on the line. Because it is `watchPosition` and not a single reading, the position updates again as the caller moves, which matters for someone walking, driving or being carried. A **Stop sharing** button ends it from the caller's side, and the page keeps the caller informed: how many fixes have been sent and roughly how accurate the latest one is.

The design keeps the link cheap to issue and hard to misuse:

- The link carries an **18-byte random secret**. Only its **SHA-256 hash** is kept in the ledger; the link itself is shown once to the call-taker and never stored in full.
- A link lasts **30 minutes by default**, and an administrator can set anything from a minute up to a full day.
- It accepts at most **900 positions**, and fixes closer together than **2 seconds** are dropped, so a phone sending rapidly cannot flood the call.
- The call-taker can **end** the link at any time, and the caller can stop sharing. An ended or expired link returns a plain "this link has ended" page and refuses any further position.

A made-up or guessed link does not work: the page is served only for a secret whose hash is on record and still live, and anything else is refused.

## Why it matters in a 911 center

The first question on almost every call is "where are you?", and the answer decides everything that follows. A call-taker who cannot place the caller cannot send help, however clearly they understand what is wrong. Minutes spent talking a disoriented caller through cross streets and landmarks are minutes a unit is not moving.

Device-based location from carriers has improved, and for most wireless calls it is good enough to dispatch on. The location link is for the gap it leaves: the landline that reports a billing address across town, the VoIP line with a stale address, the weak or wide fix, and the caller who genuinely cannot say where they are. For those calls, a position straight from the caller's phone, accurate to a few meters and updating as they move, can be the difference between a fast response and a long search.

It also helps on calls where the caller is moving. A stranded driver, a lost hiker, a person being driven to a meeting point: a single address does not describe them, but a live position that follows them does. The call-taker watches the marker move on the same map they use for everything else.

## Where CAD systems usually fall short

Placing a hard-to-locate caller is often left entirely to the call-taker's skill with questions, or to a separate product the center has to buy and bolt on. Link-based live location is sold by vendors as an add-on, with its own portal, its own sign-in and its own store of where people were. That raises a fair privacy question, because a location is sensitive data, and it adds another outside dependency to a center that may already have too many.

Where a location tool does exist, the caller's experience can be heavy: install an app, create an account, grant broad permissions, at the worst possible moment. Anything that asks a frightened caller to do more than press one button is likely to fail when it is needed most. And the record of who was located, when and under what link is not always kept in the same place as the rest of the call, so it is hard to answer later who shared what.

## How bitdispatcher does it

The location link is part of the one program, not a separate service. It reuses what is already there: **BitVoice** sends the text, the **Paging groups** public address gives the link its home, and the shared position lands on the call through the same path as every other location, so it shows on the dispatch map with no special handling.

The caller's page is built to be safe and small. It is unlisted to search engines, serves nothing but its own short script under a strict content security policy, and does one thing: ask the phone's permission and send the fixes back. The caller is never asked to sign in or install anything. Everything is recorded in the ledger as a link made and a link ended, with only the secret's hash, never the secret, so the record shows that a link existed and what became of it without keeping the key that opened it.

The limits are set to protect the caller as much as the system. A link that lives for half an hour by default, caps its positions and can be ended from either side keeps a location channel from staying open longer than the call needs it.

## Why bitdispatcher fits

A location link is a small feature that only matters on a handful of calls, which is exactly why it belongs inside a free, open CAD rather than behind a separate purchase. A small or rural center that could never justify a standalone location product gets this as part of the same program it already runs, set up in the GUI, with no extra contract and no outside portal holding its callers' positions.

It also fits bitdispatcher's habit of keeping sensitive data close and recording what happens to it. The secret is hashed, the link is short-lived, the caller is in control of their own sharing, and the ledger holds the account of it. The position that comes back is treated like any other location on the call, so it needs no special screen and no new habit to use.

## What has been tested, and what has not

The API tests cover the link's life: a link is made for a call, the caller's page is served for a live link, a made-up link is refused with a **410 Gone**, a shared fix lands on the right call, a second fix sent too soon is dropped, and an ended link stops taking positions. A browser test drives the whole flow with an **emulated phone** that grants geolocation: the caller presses Share, the call receives the position, and Stop ends the sharing.

What has **not** been tested is the part that only real phones on a real network can show. It has not been run on actual phones over the carrier network, and it has not been through the real iOS and Android location-permission prompts, which differ by phone, browser and operating system version and can behave in ways an emulator does not. bitdispatcher is not certified and is not in live service anywhere, so the location link has never been used on a real 911 call. A center adopting it should test on the phones and browsers its own callers are likely to use.

## Try it

Run bitdispatcher on your own computer with `-demo` and the BitVoice simulator (see [Get it](/get/)), open a call with no good location, make a location link, open it in a second browser tab as the caller, press Share, and watch the position land on the call and the map. The public address the link uses is set by an administrator. The relevant manual chapter is [Location links](/manual/#location-link). The source is on [GitHub](https://github.com/BioDream-LLC/bitdispatcher).

Related: [Where the caller is](/tech/caller-location/), [Station alerting and paging](/tech/station-alerting-and-paging/), [The event ledger](/tech/event-ledger/), [Cameras near an incident](/tech/cameras-near-an-incident/), [Non-emergency intake](/tech/non-emergency-intake/)
