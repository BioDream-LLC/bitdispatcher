---
title: Where the caller is: location from ALI to device fixes
tile: Caller location
summary: ALI/ANI, wireless Phase I and II, device-based hybrid fixes, HELD, altitude and trails for moving callers.
description: How bitdispatcher gets caller location: ALI/ANI, wireless Phase I and II, device-based hybrid fixes over HELD, altitude and floor, and moving callers.
icon: pin
---

## What it is

Everything on a 911 call starts with one question: where is the caller. The answer has grown more capable and more complicated over thirty years.

In the landline era, the caller's number (**ANI**) was looked up in the **ALI** database to return the service address. Wireless broke that link, so the FCC's **E911** rules created two phases: **Phase I** returns the caller's number and the cell tower and sector, which only narrows location to a wide wedge; **Phase II** requires a latitude and longitude with an accuracy standard. Carriers meet Phase II with a mix of satellite and network methods, but the fix still lands as a point with an uncertainty radius, and indoors it can be poor.

The newest source is the handset itself. Phones compute a **device-based hybrid** location from GNSS, Wi-Fi and the barometric pressure sensor, and this is delivered to 911 through a **location information server** that a center queries. In NG9-1-1 that query is **HELD** (HTTP-Enabled Location Delivery, RFC 5985), and the answer is a **PIDF-LO** location object. Device-based hybrid can also carry **altitude**, which, with building data, points to a floor in a high-rise.

## How it works

bitdispatcher treats location as something that updates over the life of a call, not a single lookup:

- **ALI/ANI and Phase I/II** arrive as the call's location through call handling, as an ALI spill or an i3 PIDF-LO. A point comes with its uncertainty, and the call-taker sees how tight or loose it is.
- **Supplemental, device-based location** is fetched with **HELD**. Under Admin › Integrations › Supplemental location, bitdispatcher sends a HELD request naming the phone by its `tel:` URI and reads the PIDF-LO that comes back, a point, circle or polygon, a civic address and the method. A HELD error such as `locationUnknown` is shown to the call-taker as "no location".
- **Altitude and floor** come in the location object: metres above the ellipsoid with their own uncertainty, which the console can relate to a floor.
- **Trails for moving callers.** Each newer location is kept, so a caller on the move leaves a trail on the map rather than jumping with no history.
- **Newer locations are offered, not forced.** When a fix arrives more than the agency's set distance from the incident (50 m by default), the call-taker is offered it: Use it makes it the incident's location, with the old one kept in the timeline and a "location changed" shown to dispatchers and units; Keep leaves the incident where it is. Nothing moves an incident without a person.

HELD can be asked automatically once for every new 911 call that has a callback, or by pressing **Ask** on the ANI/ALI panel, which can be pressed again for a fresher fix. A **Try a number** button on the settings page checks the address and credentials without a live call.

A second path, the **location hook** (`/api/hooks/location`), lets a provider or a small adapter post newer locations as JSON, with latitude and longitude, a radius, altitude and its uncertainty, a method such as `DBH`, an optional address, who says so, and when it was determined. Every location becomes a recorded event; an offer and the call-taker's decision are recorded too.

The uncertainty is treated as a first-class part of a location, not an afterthought. A fix is a point and a radius (or a circle or polygon), and the console shows the radius, so a dispatcher can tell a tight GNSS fix from a wide tower-sector estimate at a glance. The method, how the location was found, travels with it, which is why a device-based hybrid fix labeled `DBH` can be trusted differently from a cell estimate even when both land near the same spot.

## Why it matters in a 911 center

A wireless Phase II point can be good enough to find a house or wide enough to cover a city block and both sides of a divided highway. Knowing the uncertainty is the difference between sending a unit to the right door and sending it to a guess. A device-based fix that arrives seconds later can shrink that circle from a block to a driveway.

Floor matters in a way straight coordinates do not. On a cardiac arrest in a tower, a crew that knows it is the eleventh floor saves the minutes that otherwise go to searching stairwells, and those are the minutes that decide the outcome.

Moving callers are the hard case: a caller in a crashing or carjacked vehicle, or walking lost in the dark, or a child who cannot say where they are. A trail of fixes shows direction of travel, which is more useful than any single point, and lets the dispatcher put units ahead of the caller instead of behind, and describe to the caller what they should be passing.

## Where CAD systems usually fall short

Location is often treated as a single value that overwrites itself. A newer, worse fix can silently replace a good one, or a better fix can arrive and never be surfaced because the CAD took the first answer and stopped. Uncertainty is frequently dropped, so a tower-sector estimate and a GNSS point look identical on the map.

Device-based location often depends on a separate vendor portal in a browser tab beside the CAD, so the best fix the center has is never on the incident, never on the map with the units, and never in the record. Altitude, when it is delivered at all, tends to be discarded. And because the side portal is its own tool, the fact that a dispatcher looked up a better location, and when, is not part of the incident's history, so a review cannot reconstruct what the center actually knew and when.

## How bitdispatcher does it

bitdispatcher keeps every location on the call with its source, time and uncertainty, draws the current one and the trail on the map, and offers newer ones to the call-taker to accept or keep. The device-based fix is fetched over HELD from the center's configured location server and surfaced like any other newer location, so it lands on the incident and in the record rather than in a side tab.

The intended supplemental source for many US centers is **RapidSOS**, which describes its NG911 Clearinghouse as answering NENA i3 queries and issues each center its own credentials. If a center's interface is HELD, its address and token go in the settings; if it is something else, a small adapter can post to the location hook with a `source` of `rapidsos`. The device's own location and floor also arrive directly from a connected phone system such as BitVoice.

Additional data about the caller and device (NENA i3 ADR) arrives today through BitVoice's EIDO and the crash-data hook; dereferencing ADR URIs from a location server's answer is not done yet.

## Why bitdispatcher fits

Location is handled in the one program the dispatcher already works in, drawn on the same offline map as the units, and written to the same ledger as everything else, so every fix, offer and decision is attributable. The HELD client follows the RFCs, the location hook is a documented contract in the open source, and setup is in the GUI. Being free and open, a center can connect its own location server without a per-seat fee.

## What has been tested, and what has not

The HELD client is **written to** RFC 5985 and 6155 and has been tested against a HELD server written to the RFCs; it has **not** been tested against RapidSOS itself, because the partner API is behind a login. If a center is issued a HELD address and credentials, Try a number with the test number checks it.

The location hook is a contract, not a vendor integration: no provider's own API has been integrated or tested, and BitVoice's rebid already arrives as a call update. Device location and floor were tested live with BitVoice's own speech-to-text and a recorded caller. bitdispatcher is **not certified** and not in live service; test against a provider's own test data before relying on it.

## Try it

On the [demonstration](https://demo.bitdispatcher.com), simulated calls arrive with location and rebids, so you can accept a newer fix and watch the trail. The relevant manual chapters are [911 calls](/manual/#calls), [Where the caller is: device location and floor](/manual/#caller-location), [Integrations](/manual/#admin-integrations) and [Hooks and connections](/manual/#hooks).

Related: [NG9-1-1, i3 and EIDO](/tech/nena-i3-eido/), [The offline map](/tech/offline-map/), [The event ledger](/tech/event-ledger/), [Road time, not straight line](/tech/road-time-recommendation/)
