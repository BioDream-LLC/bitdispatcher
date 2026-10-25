---
title: A command line and key profiles for the dispatch floor
tile: Command line and keys
summary: A keyboard-first console: one-line commands, duty-specific key profiles, and a PremierOne-style classic layout.
description: How bitdispatcher drives dispatch from the keyboard: one-line commands, duty and screen key profiles, a classic layout, field-initiate and unit status.
icon: keyboard
---

## What it is

A computer-aided dispatch console lives or dies by how fast a dispatcher can tell it what happened. The mouse is slower than the hand already on the keyboard, and on a busy floor the difference is counted in seconds. For that reason the oldest and best CAD systems are built around a command line and a set of function keys, where a short string of letters dispatches a unit, marks it out, or opens a traffic stop.

This is not nostalgia. A trained dispatcher types a command without looking, the same way a stenographer does. The command language becomes muscle memory, and muscle memory survives stress better than a changing arrangement of buttons. The pattern goes back to the mainframe CAD systems of the 1970s and 1980s, and the muscle memory of a whole shift is often tied to one such layout.

The standard that matters here is not a published document but the agency's own convention: the order of fields in a status message, the delimiter between them, the status codes a unit uses, and which function key does what. Those conventions differ by agency and by discipline, and a good CAD honors them rather than forcing a new one.

## How it works

A command line reads a terse grammar. A verb comes first (dispatch, en route, on scene, out of service), then the units and any arguments. The console parses it, checks that the signed-in person's duty allows it, and either acts or explains why it refused. Completion fills in the rest of a verb or a call sign; history recalls earlier lines with the arrow keys.

Function keys are bound to the same actions the command line and the on-screen buttons perform. A key profile maps each key, and a layer such as Shift gives a second action to each key, doubling the available bindings without new keys.

Field commands pack several values into one line. A traffic stop needs a unit, a location, a plate and sometimes a state, a comment and a status. Rather than open a form, the dispatcher types them separated by a delimiter, in the agency's order, and the console builds the incident. Two delimiters together skip an optional field.

## Why it matters in a 911 center

A dispatcher working ten active units does not have time to point and click for each status change. Typing `OS E1` to put Engine 1 on scene is one motion. When a second call drops while the first is still being entered, the keyboard keeps up where the mouse falls behind.

Officer-initiated activity is where this shows most. An officer calls out a traffic stop on the radio with a location and a plate. The dispatcher must capture it instantly, start a status-check timer, and have the plate ready to query. One line does all of that before the officer has finished reading the plate.

Keyboard-first also matters for accountability. Every command, refusal and status is a recorded event, so the record of a shift is complete and in order, not a reconstruction from memory.

## Where CAD systems usually fall short

Many modern CAD products were redesigned around the mouse and lost the command line, or kept a weak one. Dispatchers who trained on a terse grammar find themselves slower, and the agencies feel it on a bad night.

Others hard-wire one agency's conventions. The field order, the delimiter and the status codes are fixed in the product, so an agency has to abandon the muscle memory of a whole room to adopt the tool. Function keys are often fixed too, with no way to match the layout a room has used for twenty years.

A third problem is silence on refusal. A command fails and nothing says why, or the reason is buried in a log the dispatcher never sees. On a dispatch floor an unexplained refusal is worse than an error message.

## How bitdispatcher does it

bitdispatcher is keyboard-first throughout. Type any letter on the dispatcher screen and the command line takes it, or press `/` to focus it. `Tab` completes, the arrow keys recall earlier commands, and `HELP` lists every verb. When a command is refused the console shows the reason, and the attempt is recorded.

The grammar is short and consistent:

| Command | Does |
|---|---|
| `D E1 M2` | Dispatch units to the selected incident |
| `REC` | Dispatch the recommendation |
| `ER E1` · `OS E1` · `TR M1` · `AD M1` | En route, on scene, transporting, at destination |
| `AV E1 M2` | Available: clears them from their incident |
| `OOS E5 brakes` | Out of service with a reason |
| `NEW SF 12 Main St` | New incident |
| `CL Handled` · `RO #26-000123 reason` | Close, reopen |
| `MU E4 ST1` · `RET E4` | Move up to cover a station; send home |

The command palette opens with `Ctrl K`: jump to an incident, a unit, a screen, or a chapter of the manual. Standard keys cover the common moves: `F2` for a new incident, `Ctrl Enter` to dispatch the selected units, `[` and `]` to step through open incidents, `Esc` to clear the selection.

**Key profiles per duty and screen.** The keys follow the agency's key profile for the duty you signed in as, set in Admin › Operations rules. A call-taker, a police dispatcher and a fire dispatcher can each have their own. A key does exactly what the matching command or button does, and the server checks your right either way, so a trainee works the profile as configured with no extra privilege.

**The classic (PremierOne-style) profile.** An agency moving from an older product can start from a PremierOne-style key profile and change it. It is a compatibility layout only: bitdispatcher is not a Motorola product and is not endorsed by Motorola. In this profile the function keys follow the documented classic defaults, and a **Shift** layer gives each key a second action:

| Key | Does | Shift + key |
|---|---|---|
| F1 | Home | Help |
| F2 | Queries (NCIC) | Unit-to-unit message |
| F3 | Update | Recall (search) |
| F6 | Messages and BOLOs | Refresh pending |
| F7 | **Field initiate**: processes the command line | Previous incidents |
| F11 | **Unit status**: processes the command line | The 911 call |

**Field-initiate a traffic stop.** `FI UNIT1.8TH/STEWART..ABC1234` creates a stop for UNIT1 at 8th and Stewart with plate ABC1234, the type left to the agency's default. The two dots together skip the optional field between location and plate. With the classic keys, the dispatcher types the line and presses `F7`. The incident is created with the unit on scene, the plate attached, and the stop's own status-check timer, without a second screen. It becomes a full incident later by changing its type, keeping everything. Where the agency asks, the plate is queried in NCIC at once.

**Unit status in one line.** `US UNIT1.AS` marks UNIT1 arrived on scene of its incident, using the agency's status codes. With the classic keys, the dispatcher types the line and presses `F11`. A unit typed alone opens the status form rather than guessing. A mark-out code works the same way.

The agency sets the delimiter, the field order for both field-initiate and unit status, the stop's call type, one-pass or two-pass behavior, the automatic plate inquiry, and the status codes including retired ones, all in Admin › Operations rules. A retired or unknown code, or a command the duty may not give, is refused by the server and recorded; nothing changes. A **Check** step finds a key doing two things on one screen, a key the browser reserves, or an unknown action, and refuses to save the profile until it is fixed.

## Why bitdispatcher fits

The whole console is configured in the browser: the key profile, the delimiter, the field order and the status codes all have a screen, with no file to edit. The command grammar and the classic layout let a room keep its muscle memory while moving to a free, open-source program. Every command, refusal and status is a hash-chained ledger event, so keyboard speed does not come at the cost of accountability. And because the console runs locally against a recorded map, the keyboard keeps working when the internet does not.

## What has been tested, and what has not

The keys and one-line commands are tested in the Go suites and in the browser against the built-in simulators. The classic key layout follows the documented PremierOne defaults; it is not a Motorola product and is not endorsed by Motorola, and it has not been checked against a live Motorola system. bitdispatcher is version 0.0.0, built and tested but not yet in service in any live 911 center, and it is not certified.

## Try it

Sign in to the [live demo](https://demo.bitdispatcher.com), choose a police or fire dispatcher duty, and type a command. The relevant manual chapters are [The command line](/manual/#command), [Keyboard](/manual/#keys), and [Operations rules](/manual/#admin-operations).

Related: [Mobile Data Terminal](/tech/mobile-data-terminal/), [Multi-Agency](/tech/multi-agency/), [Messages and BOLOs](/tech/messages-and-bolos/), [Search and Recall](/tech/search-and-recall/)
