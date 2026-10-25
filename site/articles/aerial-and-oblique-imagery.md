---
title: Aerial, oblique and street views of every address
tile: Aerial imagery
summary: Orthophotos, EagleView obliques, Panoramax and Mapillary street photos, and crews' own pre-plan photos.
description: How bitdispatcher records orthophotos in tiers and adds EagleView obliques, Panoramax and Mapillary street photos and crews' pre-plan photos.
icon: layers
---

## What it is

Dispatchers and crews navigate by what a place looks like, not by its coordinates: the white house behind the church, the second driveway, the loading dock at the back. A good picture of an address is worth as much as its street map, and there are several kinds, each answering a different question.

- **Orthophotos** are aerial photographs corrected so every point is seen straight down, like a map. They show rooftops, driveways, pools and outbuildings. In the US, **VBMP** is Virginia's statewide program and **USGS** imagery (including NAIP) covers the rest.
- **Oblique imagery** is shot at an angle from the four compass directions, so you see the sides of buildings, not just roofs: doors, windows, how many stories, where the entrance is. **EagleView** (historically Pictometry) is the common commercial source, licensed per agency.
- **Street-level photos** look at a place from the road, the view a crew gets arriving. **Panoramax** and **Mapillary** are open, crowd-sourced street photo platforms.
- **Pre-plan photos** are the agency's own: a crew photographs a premise, the Knox box, the standpipe, the hazardous-storage room, so the next crew sees it before arrival.

## How it works

bitdispatcher records aerial photography **in tiers**: full detail near the center, where most calls are, and less detail out to the edge of the recorded map, because full resolution over a 100-mile circle would be hundreds of gigabytes. The tiers let a center spend its disk where its calls are: the densest, highest-resolution imagery over the urban core and the immediate response area, a middle tier over the suburbs and towns, and a coarse tier to the edge, so there is always a picture under the cursor without the storage cost of uniform detail everywhere. Photo tiles are stored in bitdispatcher's own store so they show with no internet, under the street map, on both the console and the mobile terminal.

The orthophoto sources are public services read as Web Mercator tiles. bitdispatcher knows a catalog of them, best first, and a tile is taken from the first source that covers it and has it: Virginia's VBMP statewide imagery where it applies, and USGS imagery elsewhere. Where a source carries a date index of its photo footprints, the console can say how old the photo under the cursor is, which matters because a two-year-old ortho may predate the new building a crew is standing in front of.

The other views are layered on the same address:

- **Obliques** from the agency's own **EagleView** license give the view from north, east, south and west.
- **Street photos** come from Panoramax or Mapillary, looking at the place from the road.
- **Pre-plan photos** are taken by crews from the mobile terminal and attached to the premise.

These views are reachable where they are needed: on the incident, in the call-taker's Hazards tab, on the mobile terminal, and from a right-click on the map. Each view records where and when it was taken. Buildings can also be shown in 3D from OpenStreetMap.

Because looking a place up tells the provider where the incident is, each online provider is **off until an administrator turns it on**.

## Why it matters in a 911 center

On a working structure fire, an incident commander who can see the back of the building before the first unit arrives can place crews and plan access instead of walking the perimeter to learn it. Obliques show the number of stories, the roofline and the exposures that an overhead ortho flattens away.

On a rural address with a long driveway and no visible number, a street-level photo and a rooftop ortho together let a dispatcher talk a crew in: past the red barn, the house is set back on the left. Minutes spent finding an address are minutes not spent on the patient or the fire.

Pre-plan photos turn institutional knowledge into something every shift has. The senior firefighter who knows the school's standpipe connections retires; the photos the crew took stay on the premise record for everyone who runs that building afterward. The same is true for hazards a dispatcher needs before units are even sent: a photo of the oxygen-storage shed, the locked gate code location, the bariatric patient's wide-door entrance. Attached to the premise, they surface on the call before the first unit rolls.

Angle also changes what a picture can answer. A straight-down ortho is the right view for counting cars in a lot or seeing a backyard pool, but it cannot tell a commander which side of a building has the loading dock or how many floors have windows. An oblique from four directions answers exactly those questions, and the combination of the two is far more useful than either alone.

## Where CAD systems usually fall short

Imagery is often online-only, so it disappears in the outage that takes the rest of the connected map, exactly when a commander might want the clearest possible picture. Obliques and street views, when offered, are frequently separate applications in other windows rather than layers on the same incident, so a dispatcher has to leave the CAD and copy an address across.

Pre-plan photos are often trapped in a records system or a binder in the station, not on the live incident where the responding crew can see them. And where online providers are wired in, the privacy cost, that each lookup discloses the incident location, is rarely surfaced or controllable.

Storage is the other quiet problem. High-resolution orthophotography over a large response area is enormous, so a system that records it uniformly either runs out of disk or quietly downsamples everything, including the dense core where detail is most useful. Treating detail as uniform across a hundred-mile area wastes storage on empty forest while under-serving the downtown blocks that generate most calls.

## How bitdispatcher does it

Aerial photos are recorded in tiers (Virginia's VBMP, USGS elsewhere) into the local store and drawn under the map on the console and the mobile terminal. The **Views** feature brings the four EagleView obliques, Panoramax or Mapillary street photos, and the crews' pre-plan photos together for a place, reachable from the incident, the Hazards tab, the terminal and a right-click on the map, each stamped with where and when. 3D buildings come from OpenStreetMap.

Crews capture pre-plan photos from the mobile terminal, and, like all incident files, each attachment's SHA-256 goes in the ledger. Every provider is configured in the GUI and left off until an administrator enables it, because each lookup discloses the incident location.

## Why bitdispatcher fits

The views are part of the one program, on the same incident, map and mobile terminal as everything else, and the aerial base is recorded to work offline alongside the street map. Setup is in the GUI, the open sources (USGS, Panoramax, Mapillary, OpenStreetMap buildings) need no commercial license, and the agency controls which online providers are on, with the location-disclosure trade-off made explicit. Pre-plan photos use the same attachment-and-ledger path as any other incident file.

The honest cost is the commercial pieces: EagleView obliques and a full orthophoto program need the agency's own license, and Virginia's VBMP terms outside Virginia are an open question.

Because the aerial base, the oblique and street views and the pre-plan photos all live on the same incident and the same map as the units and the call, a dispatcher does not leave the CAD to see any of them, and a crew on the mobile terminal gets the same pictures in the field. The record of who looked at what, and when each photo was taken and where, is part of the same ledger that records everything else, so imagery is evidence handled like evidence rather than a convenience bolted on the side.

## What has been tested, and what has not

bitdispatcher is **not certified** and not in live service. The views have mixed test status: **EagleView** is built to its published Imagery API and tested against a stand-in, **not** yet against EagleView itself, which needs an agency's license or sandbox credentials; **Panoramax** is tested live; **Mapillary** is tested against a stand-in. Because a lookup tells the provider where the incident is, each provider is off until an administrator turns it on. The orthophoto recorder and tiering run in the demonstration.

## Try it

The [demonstration](https://demo.bitdispatcher.com) runs with the recorded aerial photos over Roanoke. The relevant manual chapters are [Aerial photos](/manual/#photos), [Views: the place from every side](/manual/#views), [The dispatch map](/manual/#map) and [Files and photos on an incident](/manual/#files).

Related: [The offline map](/tech/offline-map/), [Where the caller is](/tech/caller-location/), [Multiple monitors](/tech/multi-monitor/), [The event ledger](/tech/event-ledger/)
