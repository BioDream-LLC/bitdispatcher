---
title: One console, many monitors, one shared selection
tile: Multiple monitors
summary: A screen per monitor, found with the Window Management API, selection synced, and layouts remembered per position.
description: How bitdispatcher spreads a dispatch console across monitors using the Window Management API, with a synced selection and per-position saved layouts.
icon: monitors
---

## What it is

A dispatch console is rarely one screen. A position often has four, five or six monitors: the call and incident queues on one, the map on another, the unit status board on a third, a supervisor's overview on a fourth. Each monitor shows a different window into the same live situation, arranged for one job.

The hard parts are three. The windows must share one sign-in and one **selection**, so choosing an incident on the queue opens it on the incident screen and the map at once, rather than making the dispatcher re-select it on every monitor. The browser must be able to place each window on the right physical monitor, which needs the **Window Management API**, a web platform capability that lets a page see the arrangement of a multi-screen setup and open a window on a chosen screen. And the layout must be **remembered**, because a dispatcher should not rebuild their wall every shift.

bitdispatcher runs the whole console in the browser, and spreads it across monitors as a set of screens.

## How it works

A position opens **Screens** from the status-bar monitor button, the command palette, or **Ctrl+Shift+S**. Each screen is a window of the same live picture, chosen from a fixed set of jobs:

| Screen | For |
|---|---|
| Incidents and call entry | 911 calls and incidents beside the one being worked |
| Fire houses | Every station, its apparatus, who is covering, the empty houses |
| Police status and mark-outs | Every car: what, where, how long, and when a status check is due |
| Unit status | The unit board, large |
| Map | The map alone, following the selection |
| Incident and situation | The selected incident in full |
| Supervisor board | Totals at a glance, read-only |

- **Detect monitors**, in Chrome and Edge, uses the Window Management API to show the monitors as the computer arranges them. Assign a screen to a monitor and it opens there, filling it. In other browsers you drag each window into place once, and it opens there next time.
- **Selection is shared across screens.** Choose an incident on any of the position's windows and it opens on the incident screen and the map, because every window shares one sign-in and one selection.
- **Each screen can be limited to disciplines** (all, fire, EMS, police, other), showing those units and the incidents that need them. An all-discipline incident such as a crash with injuries appears on both the fire and the police screen. Every screen always shows every **emergency** and every 911 call not yet on an incident.
- The browser asks once to allow pop-ups and, in Chrome and Edge, window management.

**Per-position memory.** The screen layout is kept on the workstation for that position, so the next shift at the desk gets the same wall. With **Offer to open these screens at sign-in**, the full console offers to open them all in one click after signing in. The memory is tied to the position, not the person, because a given desk tends to do a given job: the fire-dispatch desk wants the fire-house screen and the map, and it should get them whoever sits down.

A word on the Window Management API itself. Before it existed, a web page could open a new window but could not know how many monitors a computer had or where they were, so it could not reliably put the map window on the left monitor and the incident window on the right. The API, available in Chromium-based browsers, exposes the screen arrangement to a page that has been granted permission, which is what lets bitdispatcher map a named screen to a specific physical monitor and open it there filling the display. Where the API is not available, the one-time manual drag achieves the same result with one setup step.

**Duty presets.** A person's workspace for each duty is saved with their account and opens at any console. A preset stays **locked** unless deliberately edited, saved and locked again. Trainees work the agency's assigned preset, and supervisory views are the agency's to grant and are refused on the server without the right permission.

## Why it matters in a 911 center

A dispatcher works several things at once: a queue, a map, the units, a specific incident. Spreading them across monitors means none of them is buried behind another, and the eye moves between fixed places instead of toggling tabs. When a call is bad, the fewer deliberate actions it takes to see the whole picture, the better.

Shared selection is the feature that saves the seconds. Without it, picking an incident on the queue and then re-finding it on the map and again on the incident panel is three actions on three screens; with it, it is one. Across a shift of hundreds of calls that is a large amount of friction removed, and friction removed under load is errors avoided.

Per-position memory and duty presets matter because 911 is a 24-hour operation with rotating staff. A relief dispatcher sitting down at a strange desk should get a familiar, correct layout, not spend the first minutes of a shift rebuilding it. Locked presets keep a trainee on the agency's approved workspace.

## Where CAD systems usually fall short

Multi-monitor support is often just the operating system stretching one window across screens, with no notion of a screen per job and no shared selection, so the dispatcher re-selects the same incident on each monitor. Thick-client CADs may support multiple windows but tie the layout to one machine, so it does not follow the person to another desk. Web-based CADs have historically struggled to place a window on a specific physical monitor at all, because the browser did not expose the screen arrangement until the Window Management API.

Layout memory, where it exists, is frequently per-user-per-machine with no separation between a trainee's locked preset and a supervisor's view, so permissions on what a position may show are enforced loosely, in the client rather than on the server. A dispatcher who moves to a different desk often finds a blank slate and rebuilds the wall from scratch at the start of a shift.

## How bitdispatcher does it

The console is in the browser, and each screen is a window of the same live state. Monitors are found with the **Window Management API** in Chrome and Edge, and told apart by position, so two identical monitors are never confused; in other browsers a one-time drag places each window. Selecting an incident anywhere opens it on every screen. Layouts are kept per console position on the workstation, and duty presets are kept with the account and open at any console, locked unless deliberately edited. Which views a position may show, including supervisory views, is enforced on the server, not just hidden in the client.

## Why bitdispatcher fits

This works because the whole console is one web application sharing one live state, so a selection naturally propagates to every window, and because it uses a standard browser capability rather than a special client. It is free and open, set up in the GUI, with presets and layouts that follow the person, and the permission checks sit on the server. Nothing extra is installed on the workstation beyond a current Chromium-based browser for full monitor detection, which keeps the console easy to stand up on ordinary hardware.

## What has been tested, and what has not

Multiple monitors have been tested with **Chrome's emulated monitors**, not yet on a real console's physical monitors. The browser end-to-end suite includes a case with **three emulated monitors**, checking that each screen lands on its own and that the selection is shared across the windows. bitdispatcher is **not certified** and not in live service. The Window Management API and automatic monitor detection are a Chrome and Edge capability; other browsers use the one-time manual placement described above.

## Try it

On the [demonstration](https://demo.bitdispatcher.com), open Screens and add the map and incident screens to see the shared selection, best in Chrome or Edge. The relevant manual chapters are [Screens on several monitors](/manual/#screens), [Map screen](/manual/#screen-map), [Your own layout](/manual/#layout) and [Signing in: your duty, codes and the lock](/manual/#signin).

Related: [The command line and keys](/tech/command-line-and-keys/), [The offline map](/tech/offline-map/), [Aerial and oblique imagery](/tech/aerial-and-oblique-imagery/), [Road time, not straight line](/tech/road-time-recommendation/)
