---
title: A dispatch map that works when the internet fails
tile: Offline map
summary: OpenStreetMap, routing and county GIS recorded into the binary, so the map still works with no internet.
description: How bitdispatcher records OpenStreetMap, vector tiles, a routing graph and county GIS into its own store so the dispatch map works with no internet.
icon: map
---

## What it is

The dispatch map is where a call becomes a place and a place becomes a route. If that map depends on a live connection to an online tile service, then an internet outage, a cut fiber or a provider problem takes the map down at the exact moment a center may be busiest. For a 911 center, a map that needs the internet is a single point of failure in the wrong place.

bitdispatcher records the map it needs into its own store and uses it locally. The base is **OpenStreetMap** (OSM), the open, worldwide map built by a community of contributors and free to use and redistribute. OSM data describes roads, buildings, water and places as nodes, ways and relations, and from it bitdispatcher builds a display map, an address and intersection index, and a routing graph.

On top of OSM sits the authoritative local data: the **county's own GIS**, pulled from an **ArcGIS REST** service or a **GeoJSON** file. Counties maintain the data that matters most for dispatch, address points, hydrants, response zones and place names, and those are used first where the county keeps them.

## How it works

bitdispatcher records a circle around the center, **100 miles by default**, chosen on the map. Inside that circle it holds:

- **Vector tiles** for a crisp **day and night** dispatch map that redraws at any zoom without new downloads.
- A **routing graph** over the road network, so recommendations use real driving distance and time rather than a straight line.
- An **address, intersection and landmark index**, so a dispatcher can type a place and get a point.

The OSM data itself is read from a standard **PBF** extract. bitdispatcher's importer decodes the PBF wire format with the standard library and inflates the blocks across several processor cores, so a state-sized extract reads in seconds. Vector tiles, rather than pre-rendered image tiles, are what make the day and night styling possible: the same geometry is drawn either way, so switching a floor of dispatchers to a dark map for a night shift is a style change, not a second set of downloaded images. Vector tiles also keep labels crisp at every zoom and keep the recorded size manageable.

County GIS is layered in by kind, address, hydrant, zone, place or road, each mapped from the county's own field names to bitdispatcher's. The county's addresses are searched first because they are authoritative where the county keeps them. **Virginia's statewide address points** are a built-in preset. Where county data has gaps, two open sources fill them: road **centerline ranges**, which interpolate a house number along a street segment, and **Overture**'s open global address data. The result is an address search that tries the best source first and falls back gracefully.

All of this lives in bitdispatcher's own store in its data directory. A 100-mile recorded map is about 620 MB on disk. Outside the recorded area, the console can still show online tiles, clearly marked as online, so a center is never worse off than a connected map but is not dependent on one inside its own territory.

The split of responsibilities is deliberate. OpenStreetMap gives the worldwide base, the road geometry for routing and a first pass at addresses and places. The county's GIS overrides it where the county is authoritative, because a local government's address points and response zones are kept current by the people who assign the addresses. Centerline ranges and Overture fill the remaining gaps so a search rarely comes back empty. Because the search tries the best source first and falls back, a dispatcher gets one answer without having to know which dataset it came from.

## Why it matters in a 911 center

The map is in front of the dispatcher on every call. It is how a vague address is pinned, how the closest right units are found, how a crew is talked in to a rural driveway, and how a supervisor sees where the work is piling up. An outage that blanks the map removes all of that at once.

Outages are not rare or exotic. Construction cuts fiber. Upstream providers have incidents. Rural centers often have one circuit. A map that keeps working through the outage, with routing and address search intact, lets the center keep dispatching from the last good picture instead of falling back to paper and memory.

County GIS matters because the county's address points are usually more current and complete than any global base map, especially for new subdivisions, rural route addresses and the difference between where a parcel is labeled and where the driveway actually meets the road. Hydrant layers are life-safety data for fire; response zones decide which station owns a call.

Night styling is not a cosmetic preference either. A dispatch floor is often dimly lit so staff can read their screens for twelve hours without eye strain, and a bright white map in a dark room is tiring and washes out the colored status of units and incidents. A proper dark map keeps the important marks legible through a long overnight shift.

## Where CAD systems usually fall short

Many dispatch maps are thin clients over an online basemap, so they degrade or disappear when the link does. Offline support, where it exists, is often a stale cached area with no routing and no search, enough to show a picture but not to work a call.

Loading county GIS is frequently a professional-services project, with each layer and field mapping done by the vendor, so keeping the map current as the county republishes its data is slow and expensive. And address search often leans on a single online geocoder, which is both a dependency and a privacy question, because every lookup tells an outside service where the incident is. When that service is slow or down, the dispatcher's fastest way to place an address, typing it, is the part that fails first.

## How bitdispatcher does it

Recording the map is a screen, not a service ticket. Under **Admin › Map** the administrator picks the center and radius on the map and records it; under **Admin › County GIS** they point bitdispatcher at an ArcGIS REST service or upload a GeoJSON file and map the fields. Virginia's statewide address points are a preset to select.

The dispatch map supports day and night styling, address, intersection and landmark search, right-click to route, and 3D buildings from OSM. Aerial photography and oblique and street views are recorded alongside it and are covered in the aerial-and-oblique-imagery article. The same recorded map drives the routing behind unit recommendations, so the distances and times a dispatcher sees are real drives over the network.

Because the map is recorded locally, address search does not phone an outside geocoder for routine lookups, which keeps incident locations inside the center.

## Why bitdispatcher fits

The offline map is a direct expression of what bitdispatcher is: one program with no external dependencies to fail. The map, routing, address search and county GIS are recorded into its own store and used locally, set up entirely in the GUI, with the data sources, OpenStreetMap, Overture and the county's own GIS, all open or the agency's own. There is no map server to run and no per-map-view fee.

For a small or rural center on a single circuit, this is the feature that makes the difference between a CAD that keeps running through an outage and one that does not, and it costs nothing beyond the disk to hold the recorded area.

## What has been tested, and what has not

The Go test suite covers the OSM PBF importer and the router. The map, routing and address search run locally in the demonstration over the full 100-mile recorded area with Virginia's address points loaded. bitdispatcher is **not certified** and not in live service. VGIN's terms for using its data outside Virginia are an open question noted in the project, so a center outside Virginia should confirm its own data licensing. Online tiles outside the recorded area are shown only when the center chooses, and are marked as online.

## Try it

The [demonstration](https://demo.bitdispatcher.com) runs on the 100-mile recorded map around Roanoke, Virginia, with the statewide address points. The relevant manual chapters are [The dispatch map](/manual/#map), [Recording the map](/manual/#admin-map), [County GIS](/manual/#admin-gis) and [Map screen](/manual/#screen-map).

Related: [Road time, not straight line](/tech/road-time-recommendation/), [Aerial and oblique imagery](/tech/aerial-and-oblique-imagery/), [Where the caller is](/tech/caller-location/), [Several sites, active-active](/tech/active-active-sites/)
