---
title: Road closures and live traffic that change drive times at once
tile: Closures and traffic
summary: Close a road from the map or set a radius, and drive times and unit recommendations go around it at once, with optional live traffic.
description: How bitdispatcher closes roads from the map, folds in Waze for Cities or GeoJSON traffic, and reroutes unit recommendations with no map rebuild.
icon: route
---

## What it is

A crash blocks all lanes of Main Street. An underpass floods. A parade takes six blocks for the afternoon. The moment that happens, the drive times a dispatcher relies on are wrong, and the closest unit by the map is no longer the closest unit by road. If the dispatcher has to remember the closure and mentally route around it on every call, the system is working against them at the worst time.

bitdispatcher lets a dispatcher **close a road on the map**, or close every road inside a circle, and from that moment drive times and unit recommendations go around the closure. Live **traffic** from a feed the agency subscribes to does the same thing for congestion: a slow road takes longer to drive, and a reported road closure closes it. Both are expressed as a factor on each road segment, so the router accounts for them on every search with no rebuild of the recorded map.

## How it works

A dispatcher right-clicks the map and chooses **Close a road from here**, then clicks along the stretch and presses Done. Or they choose **Close the roads around here** and give a radius, from **5 meters to 3 kilometers**. Either way they give a reason, an optional end time, an optional detour, and can note the closure on the selected incident. A closure is lifted from the map or from **Admin › Road closures & traffic**, and a closure with an end time lifts itself when that time passes. Remove is for one entered by mistake; it stays in the ledger either way. Every closure, change and lift is a ledger event.

Underneath, each road segment of the recorded road network carries a **factor** on its recorded driving time: **1** as recorded, **2** for twice as slow, and closed means not driven at all. The router reads this overlay on every search, so a change takes effect at once with no map rebuild. The geometry of a closure is matched to the network carefully:

- A **line closure** takes the segments whose midpoint is within **18 meters** of the drawn line and that run the same way as it, so a cross street passing through stays open.
- A **circle closure** takes every segment with any part inside the radius, which suits a flood or a fire scene that blocks a junction.

Where a drive starts and ends is never refused, so a unit can still be routed to a house on a closed road. The recommendation's source line says, for example, **"around 1 road closure, with live traffic"**. On the map, closed roads are drawn in red dashes and traffic in yellow, orange and red. Crews see **"Roads closed"** with detours on the mobile terminal, refreshed every 30 seconds, and their drawn route goes around the closure.

**Live traffic** comes from one of two kinds of feed:

- **Waze for Cities**, Waze's free data-sharing program for public agencies. The agency joins the program and is given a JSON feed URL.
- Any **GeoJSON** FeatureCollection of lines, each carrying `speed_kmh`, a `factor`, or `closed: true`.

The feed is fetched every **2 minutes** by default, adjustable from 30 seconds to an hour. A slow road's time is its recorded time multiplied by its recorded speed divided by the reported speed, **capped at 10x**. A Waze jam at level 5 caused by a road-closed report, or with a negative delay, closes the road outright. If a feed stops arriving, bitdispatcher falls back to the recorded times rather than leaving stale congestion in place.

## Why it matters in a 911 center

The closest unit by road time is the whole basis of a good recommendation, and a closure quietly breaks it. A dispatcher who sends the map's closest engine into a closed street has lost minutes before anyone notices. Letting the dispatcher mark the closure once, on the map, and have every later recommendation account for it, keeps the right unit the recommended unit through the event.

Closures also need to reach the people driving. A detour that lives only in the dispatcher's head is not on the mobile terminal when a crew hits the barricade. Drawing the closure and its detour for crews, and routing their own map around it, keeps the floor and the field working from the same picture.

Live traffic matters most in the places where congestion is unpredictable: a highway backing up after an earlier crash, a downtown grid at rush hour. A feed that nudges drive times up on the roads that are actually slow, and closes the ones that are actually closed, makes the recommendation reflect the city as it is right now, not as it is on an average Tuesday.

The guardrails matter too. Capping a slowdown at ten times recorded keeps one bad data point in a feed from making a road effectively infinite. Falling back to recorded times when a feed stops means a provider outage degrades gracefully instead of freezing the map in the last jam it saw.

## Where CAD systems usually fall short

In many CAD systems a road closure is a note on a call or a line in a log, not something the router knows about, so recommendations keep sending units down the closed road. Where closures do affect routing, they are often an administrator's task rather than something a dispatcher can do on the map in the moment a crash is reported.

Live traffic, where it exists, is frequently tied to one commercial provider and one contract, and it is not always clear what the agency is paying for or whether congestion actually changes the dispatch recommendation or only colors the map. Add-on modules for traffic can be a separate purchase, a separate login and a separate support line. And a traffic feed that fails can leave a center worse off than no feed at all if stale data is treated as current.

## How bitdispatcher does it

Closing a road is a right-click on the map, a few points or a radius, and a reason, and the recommendation changes on the next search. The closure is a ledger event with its reason, its times, who closed it and who lifted it, so the record of why a route changed is part of the permanent log.

Traffic is set up once in **Admin › Road closures & traffic**: pick Waze for Cities and paste the feed URL the agency was given, or point at a GeoJSON feed, and set how often to fetch. Both kinds become the same per-segment factors the router already uses for closures, so there is nothing new in the routing path to go wrong. The map draws closures and traffic, the mobile terminal shows closures and detours to crews, and the recommendation's source line names how many closures and whether live traffic is in play.

## Why bitdispatcher fits

Closures and traffic are part of the one program and the one recorded map, not a separate routing service or a paid traffic module. They are free, set up in the GUI, and recorded in the ledger. They build directly on the road-time recommendation: the same routing graph that finds the closest unit by driving time reads the same overlay that closures and traffic write, so there is one consistent answer for the dispatcher and the crew. Waze for Cities is the agency's own free partnership, and GeoJSON lets a center use a DOT or local feed it already has.

## What has been tested, and what has not

The Go test suite covers closures on a synthetic street grid: a closure sends drives around Main Street while side streets stay open, a slowdown doubles a drive, a circle takes the junction roads, and lifting a closure restores the original drive. API tests exercise a stand-in Waze feed, where a 5 km/h jam more than doubles a drive, and GeoJSON parsing. A browser test covers the closures page and lifting a closure. bitdispatcher is **not certified** and not in live service. What has **not** been tested is a real **Waze for Cities** feed, which needs the agency's own partnership, and use on a real dispatch floor.

## Try it

On the [demonstration](https://demo.bitdispatcher.com), right-click the map to close a stretch of road or a circle, then make an incident nearby and watch the recommended units route around it. The relevant manual chapters are [Road closures](/manual/#road-closures) and [Road closures and traffic](/manual/#admin-roads). The source is on [GitHub](https://github.com/BioDream-LLC/bitdispatcher).

Related: [Units by road time](/tech/road-time-recommendation/), [The offline map](/tech/offline-map/), [Cameras near an incident](/tech/cameras-near-an-incident/), [The event ledger](/tech/event-ledger/)