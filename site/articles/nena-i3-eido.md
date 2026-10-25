---
title: NG9-1-1, NENA i3 and EIDO at the CAD boundary
tile: NG9-1-1 and EIDO
summary: How calls reach the CAD over an ESInet with i3 location and EIDO, and move on to the field and to neighbours.
description: How bitdispatcher takes NG9-1-1 calls over the ESInet using NENA i3 PIDF-LO and EIDO, with ALI as the legacy path, from any compliant call handling.
icon: phone
---

## What it is

For decades a 911 call arrived over the phone network and the caller's location came from a database lookup called **ALI** (Automatic Location Identification), keyed by the number from **ANI** (Automatic Number Identification). That world is being replaced by **NG9-1-1**: 911 carried as data over IP networks called **ESInets** (Emergency Services IP networks).

The architecture for NG9-1-1 is NENA's **i3** standard, published as **NENA-STA-010** (i3 Standard for Next Generation 9-1-1). In i3, a call is a **SIP** session, and the caller's location travels with it as **PIDF-LO** (Presence Information Data Format Location Object), an XML document that can hold a geodetic point with an uncertainty, or a civic address, and the method used to find it. Additional data about the call, caller and device travels alongside under **RFC 7852**.

Above the raw call sits the **EIDO** (Emergency Incident Data Object), NENA's **NENA-STA-024** standard, a JSON document that represents an incident and everything known about it. EIDO is how an incident is passed from call handling to a CAD, and from one CAD to another, in a vendor-neutral form.

bitdispatcher is a CAD. It does not answer the phone; it takes what call handling hands it. The point of these standards is that it can take that hand-off from **any compliant call-handling system**, not only from a matched product.

## How it works

bitdispatcher accepts caller location and incidents in three forms, from oldest to newest:

- **ALI spill** over TCP or serial, the legacy path, for a center still fed by a traditional or transitional system.
- **NENA i3 PIDF-LO**, the location object from an i3 call.
- **EIDO** (NENA-STA-024), a full incident object, which arrives in the 911 calls list as a data-only call ready to be made into an incident or attached to one.

A location, however it arrives, becomes the call's current location. When a newer one comes in, more than a set distance from where the incident sits, the call-taker is **offered** it: Use it moves the incident, and the old location stays in the timeline; Keep leaves it. Nothing moves an incident without a person.

For the i3 way of asking a location server for a better fix, bitdispatcher speaks **HELD** (RFC 5985), naming the phone by its number (RFC 6155) and reading the PIDF-LO that comes back. That is covered in depth in the caller-location article.

For working with neighbors, bitdispatcher sends and receives EIDOs. An open incident can be sent to a neighboring agency's CAD as an `application/emergency.eido+json` POST carrying the incident, the call, the callback, the location by value and the notes. A neighbor's EIDO posted to bitdispatcher arrives as a call in the queue, marked as from that agency.

A note on how the pieces fit together. In i3, the call is a SIP session and the location rides with it as PIDF-LO, either by value, the location object embedded in the message, or by reference, a URI the center dereferences to fetch a current fix. Location by reference is what makes rebids work: each time the center dereferences the URI it can get a fresher position for a caller who is moving. EIDO sits a level up, describing the incident as a whole rather than a single call, which is why it is the right unit to exchange between two CADs: it carries the incident number, type, status and priority, not just a phone number and a point.

## Why it matters in a 911 center

The caller location decides which units go and how fast they find the door. An i3 PIDF-LO with a tight geodetic point and an uncertainty radius is a different thing from a legacy ALI record that resolves to a tower sector, and the dispatcher needs to see which one they have and how much to trust it.

The standards also decide whether an agency is trapped. If the only way a call reaches the CAD is a proprietary link to one phone system, replacing either product is a large project. If the CAD takes i3 and EIDO at the boundary, the call-handling system and the CAD can be chosen and replaced independently, which is the whole promise of NG9-1-1.

EIDO between CADs matters most at a county line. When a crash straddles two jurisdictions, the agency that took the call can push the incident to the neighbor's CAD as structured data, instead of reading an address over a recorded line and hoping it is transcribed correctly.

The legacy ALI path still matters because the transition to full NG9-1-1 is uneven. Many centers run for years taking some calls as i3 and some as ALI, and a dispatcher cannot afford to care which is which in the middle of a call. Treating all three inputs, ALI spill, i3 location and EIDO, as the same thing once they are on the call, a location the call-taker can accept or keep, keeps the transition out of the dispatcher's way.

## Where CAD systems usually fall short

Standards support is often claimed broadly and delivered narrowly. A product may take i3 location only from its own phone system, or accept EIDO only from a partner it has tested with, so interoperability exists on the data sheet but not between two agencies that bought different brands. CAD-to-CAD is frequently a bespoke, paid integration per neighbor rather than a standard both sides already speak.

The legacy path also lingers awkwardly. Many centers are mid-transition, taking some calls as ALI and some as i3, and a CAD that treats the two as separate worlds makes the call-taker reconcile them by hand.

## How bitdispatcher does it

Caller location arrives as ALI spill, i3 PIDF-LO or an EIDO, and all three end the same way: a location on the call that the call-taker can accept or keep. Integrations are configured in the browser under **Admin › Integrations**, including a call-handling system and neighboring agencies with their own addresses and token pairs.

With [BitVoice](https://bitvoice.org), BioDream's open-source phone system and 911 call handling, bitdispatcher uses the full live stream: calls ringing before answer, rebids, STIR/SHAKEN verification status, text-to-911, and more. But BitVoice is never required. Any system that sends ALI spill, i3 location or EIDO works on its own.

For testing without a live network, the source includes `cmd/i3caller`, a lab tool that places NG9-1-1 calls over SIP with a PIDF-LO location, RFC 7852 additional data and a STIR/SHAKEN verstat, serving location by reference so each rebid finds the caller farther along. It is explicitly for a lab, never a real 911 line.

## Why bitdispatcher fits

bitdispatcher puts the standards at the boundary and nothing proprietary in the middle. It is one free, open program, so a small or mid-size center can take i3 and EIDO without a per-seat contract or a separate integration engine. Every integration is set up in the GUI and recorded in the ledger, and the EIDO and i3 encoders are in the open source, so an agency can see exactly what is sent and received.

Because CAD-to-CAD is plain EIDO over HTTPS with per-agency tokens, a neighbor only needs to be reachable and to speak the standard, not to run the same product.

## What has been tested, and what has not

bitdispatcher is **written to** NENA i3 (STA-010) and EIDO (STA-024); it is **not** NENA-conformance certified and is not in live 911 service. Testing has been done against simulators and samples:

- A live BitVoice in a lab, with `cmd/i3caller` placing the NG9-1-1 calls and standing in for the radio system. What works end to end, and what is built but not yet exercised, such as CAMA, which needs a T1 card, is written down in the BitVoice notes.
- ALI from sample spills.
- EIDO checked against bitdispatcher's own reader and the EIDOs BitVoice writes; it has **not** been tested against another vendor's CAD, so CAD-to-CAD is proven bitdispatcher-to-bitdispatcher only.
- Golden files for i3 and EIDO generated by BitVoice's own encoder.

CAD-to-CAD updates do not yet flow back on their own; each send is a press, and closing, unit status and comments are not exchanged continuously.

## Try it

The [demonstration](https://demo.bitdispatcher.com) runs the BitVoice simulator beside it, so simulated 911 calls arrive with location. The relevant manual chapters are [911 calls](/manual/#calls), [Integrations](/manual/#admin-integrations), [BitVoice](/manual/#bitvoice) and [Incidents with neighbouring agencies](/manual/#cad-to-cad).

Related: [Where the caller is](/tech/caller-location/), [Many agencies on one board](/tech/multi-agency/), [The event ledger](/tech/event-ledger/), [Several sites, active-active](/tech/active-active-sites/)
