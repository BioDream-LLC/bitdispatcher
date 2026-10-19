# bitdispatcher

### An open-source, single-binary 911 computer-aided dispatch (CAD) system

**A complete, Apache-2.0 licensed CAD for 911 centres and dispatch operations, run 100% from its web interface.** Incidents, unit recommendation by road time, a dispatch map that works with no internet, screens for every monitor at the console, mark-outs and status checks, a supervisor board, a mobile terminal for the crews, incident reports, NCIC inquiries and the state's crime reporting file. No per-seat licence. No server farm. No database to stand up first.

**One 35 MB file. 25 MB of memory at rest.** Start it, sign in, and a dispatcher has a working console; the map, the units, the run cards and everything else are set up from the browser.

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8.svg?logo=go&logoColor=white)](https://go.dev)
[![Version](https://img.shields.io/badge/version-v0.0.0-orange.svg)](#status)
[![Platforms](https://img.shields.io/badge/platforms-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey.svg)](#build-and-run)
[![Works with](https://img.shields.io/badge/works%20with-BitVoice-3b82f6.svg)](https://github.com/biodream-llc/bitvoice)
[![CI](https://github.com/biodream-llc/bitdispatcher/actions/workflows/ci.yml/badge.svg)](https://github.com/biodream-llc/bitdispatcher/actions/workflows/ci.yml)

**[[Try bitdispatcher now: the live demonstration, no install]](https://demo.bitdispatcher.com)** · **[bitdispatcher.com](https://bitdispatcher.com)**: the website, the manual and this page

**Press Sign in as the demo user**, or sign in as `demo` with password `try-bitdispatcher-demo` · a dispatcher's console on invented data around Roanoke, Virginia · simulated 911 calls arrive every 35 seconds · reset every night

**[Build bitdispatcher from source](#build-and-run)** · No release yet. Launch state: **Virginia**.

[**What it does**](#everything-it-does) · [**What it looks like**](#what-it-looks-like) · [**Build and run**](#build-and-run) · [**With BitVoice**](#with-bitvoice-and-without-it) · [**How it is tested**](#how-it-is-tested) · [**Status**](#status) · [**Manual (PDF)**](docs/manual/bitdispatcher-manual.pdf) · [**FAQ**](#faq) · [**bitdispatcher.com**](https://bitdispatcher.com)

---

## What is bitdispatcher?

**bitdispatcher is the software a 911 dispatcher works in all shift.** A call comes in with the caller's location; the dispatcher makes it an incident, bitdispatcher recommends the closest right units by road, the dispatcher sends them with one keystroke, and every status, note and decision after that is kept in a ledger that cannot be quietly changed.

**It is built for the agencies the big CAD vendors price out.** Small and mid-size counties and cities need everything a large centre needs (fire, EMS and police together, move-ups, mark-outs, reports, NCIC, the state's crime file) done reliably, without a seven-figure contract and a server room. bitdispatcher is one program, free, with its source.

**100% in the web interface.** That is the first rule of the project. Units, stations, call types and run cards, the recorded map, county GIS, users and positions, integrations, the advisory codebook, agency scripts, settings: every one has a screen, and a feature that can only be reached through a file or the command line is treated as a bug. Every screen has a chapter in the built-in manual and automated browser tests.

**The dispatcher decides.** Recommendations, advisory checks and agency scripts advise; nothing dispatches, closes or changes the priority of an incident on its own.

**Standards at every boundary.** Caller location arrives as ALI spill, NENA i3 PIDF-LO or an EIDO incident from any call-handling system. With [BitVoice](https://github.com/biodream-llc/bitvoice), BioDream's open-source phone system and 911 call handling, it uses every hook BitVoice offers a CAD; without it, it works on its own.

## Everything it does

Where something has been tested only against simulators or sample data rather than live systems, [it says so](#status).

**The dispatcher's console**
- The 911 call queue (ringing before answer), the incident queue with priority timers, the map, the incident panel and the unit status board on one screen; every part shows something the dispatcher acts on.
- A command line for everything (`D E1 M2`, `ER E1`, `OS E1`, `NEW SF 12 Main St`, `MO 2A12 MEAL`, `SC 2A12`), with completion, history and the reason shown when a command is refused; a ⌘K palette; drag a unit onto an incident to send it; a right-click menu on every unit.
- Run-card recommendation by capability and **road time**, counting units already on scene, flagging what is short and which stations would be left empty.
- Premises and hazards with every incident ("oxygen in use", "large dog", "Knox box"), and the address's history from the previous CAD.

**Screens for several monitors**
- Incidents and call entry, fire houses, police status and mark-outs, unit status, the map, the incident, and a supervisor board: each on its own monitor, opened with one click and remembered per console position.
- Each screen limited to fire, EMS or police if wished, with emergencies and unattached 911 calls always shown. Choosing an incident on one screen opens it on all the others.
- Monitors found with the browser's Window Management API, told apart by position, so two identical monitors are never confused.

**Fire, EMS and police**
- **Fire houses:** every station as a card with its bays, what is in quarters, covering units and gaps, with a one-click move-up by road time.
- **Police mark-outs:** meal, fuel, report writing, follow-up, traffic stop, business check, court, jail and more (the agency's own list), each with where the officer is and its own status-check timer; overdue checks flash; `SC` clears them in one keystroke; interruptible mark-outs are offered when no car is free.
- **Supervisor board:** what is free, what is waiting, status checks due, empty stations, and where the work is piling up, shaded by county zone or station area.

**The map, with no internet**
- OpenStreetMap recorded for a circle around the centre (100 miles by default) into bitdispatcher's own store: a night and day dispatch map, road routing, address, intersection and landmark search.
- County GIS (address points, hydrants, zones, place names) from ArcGIS REST or GeoJSON, searched first; Virginia's statewide address points are a preset. Centreline ranges and Overture's open addresses fill in the gaps.
- Aerial photos recorded in tiers (Virginia's VBMP, USGS elsewhere) under the map, on the console and the mobile terminal.

**In the field**
- A mobile data terminal for crews: assignment, route, premises, notes, one-tap statuses on F-keys, mark-outs, GPS, and the emergency button.

**Records and reporting**
- Incident reports with the NIBRS data elements checked and supervisor approval; Virginia's incident-based reporting file for the month in one click.
- NCIC inquiries through the state's switch or a training simulator, logged with purpose and reason.
- The Daily Operations Report and shift debrief, built from the ledger, with drill-down and supervisor certification.

**Assisting the dispatcher**
- An advisory panel: deterministic evidence rules from a versioned codebook (possible duplicate, a call near an open incident, fire described on a non-fire call, hazards at the address), in shadow or live mode, tested against recorded incidents first.
- Agency scripts in JavaScript or Lua, sandboxed and time-limited, for incidents, recommendations and reports; advice only.

**The call, without the typing**
- A live transcript of each call (from BitVoice or any speech-to-text engine), with the address, weapons, vehicles, plates, direction of travel, people and injuries pulled out by rules; urgent ones in red, the address placed with one press.
- Spanish callers translated line by line by a built-in emergency phrasebook (it says when it has only the gist), ready reply phrases in the caller's language, texted or spoken aloud; an optional language model of the agency's choosing for the rest.
- The phone's own device location and floor (altitude), and a trail for a caller on the move; several callers about one fire grouped and attached in one step; non-emergency calls kept to one side, with a person always deciding.
- Vehicle crash notifications rated for injury risk by delta-V and rollover, with the response to consider (an electric car flagged for the fire crews); drones launched by rule or by a press, flying on the map; radio transmissions' words on the incident; every incident summarised in a paragraph for the crews.

**Looking after the people**
- Critical calls (a cardiac arrest, a child, an officer down) put everyone who worked them on the supervisor's care list: a break offered, a debrief arranged, and a quiet word on their own console afterwards.
- Breaks and workload on the supervisor board, with who is free to relieve; a quality review of every incident against the centre's standards and the questions heard on the transcript.
- A training environment with scripted scenarios on the agency's own map and units, scored, never touching the live board.
- Working through an outage from the last board with an offline log sent when the server is back, and a warning of telephony denial-of-service floods.

**Built to keep running**
- An append-only, SHA-256 hash-chained event ledger, verified and searched from the browser.
- Several sites active-active, each keeping a verified copy of the others' ledgers; each keeps working when cut off.
- Migration of an earlier CAD's premises, history and run cards from its CSV exports.
- The manual built in: Help on every screen, served at `/manual` and printed as a PDF.

## What it looks like

Everything below ships inside the binary, shown with the Roanoke demonstration data.

**The console:** the call and incident queues, the map with the caller, the incident with its hazards, advisory checks and the units recommended by road.

![The bitdispatcher console with an incident open and two units recommended](docs/screenshots/console.png)

**The supervisor board:** totals across the top, where the work is on the map, each discipline's units, and the busiest areas.

![The supervisor board](docs/screenshots/board.png)

**Police status and mark-outs:** what every car is doing, where, and when it is due a status check.

![The police status and mark-out board](docs/screenshots/police.png)

**Fire houses:** each station's bays, what is in quarters, who is covering, and the move-ups.

![The fire house board](docs/screenshots/stations.png)

**Screens:** a layout for each console position, one screen per monitor.

![The Screens dialog](docs/screenshots/screens.png)

**Marking a unit out**, with where and a note.

![The mark-out dialog](docs/screenshots/markout.png)

**The mobile terminal** on a crew's tablet.

![The mobile data terminal](docs/screenshots/mdt.png)

**The operations report** and **the advisory panel**.

![The operations report](docs/screenshots/report.png)

![The advisory panel](docs/screenshots/advisory.png)

## Build and run

Go 1.26 or newer, and Node 24 with corepack (pnpm) for the web interface.

```sh
git clone https://github.com/biodream-llc/bitdispatcher && cd bitdispatcher
make web build sim
./bin/bitdispatcher -demo      # https://localhost:8443; the first start prints the admin password once
./bin/bvsim                    # optional: a simulated BitVoice with 911 calls and radio traffic
```

`-demo` adds fictional stations and units around Roanoke, Virginia, labelled as demo data. Then, in the browser:

1. **Admin › Map:** record the map for your area (the centre and radius are chosen on the map).
2. **Admin › Units, Stations, Call types & run cards:** your agency.
3. **Admin › Integrations:** BitVoice or another call-handling system; for the simulator, `https://127.0.0.1:9444` with the token `bv_simulated`, and press *Read it* to pin its certificate.

`bitdispatcher -data <dir>` chooses where the ledger, users and recorded map live; `-tls-cert` and `-tls-key` use your own certificate instead of the self-signed one; `bitdispatcher reset-password <user>` sets a new password from the command line.

**Memory.** 25 MB at rest with the demonstration units and no recorded map. With a 100-mile recorded map (about 620 MB on disk), aerial photos and county GIS loaded, about 500 MB.

## With BitVoice and without it

bitdispatcher takes caller location from any standards-compliant call-handling system: **ALI spill** over TCP or serial, **NENA i3 PIDF-LO**, and **EIDO** incidents (NENA-STA-024).

With [BitVoice](https://github.com/biodream-llc/bitvoice) it also uses BitVoice's live event stream (calls ringing before answer, rebids, caller photos), sends EIDO both ways, texts the caller, sends a video link and shows live video, takes radio emergencies, acknowledges them both ways and shows unit positions from the radio system. The full list is in [docs/BITVOICE.md](docs/BITVOICE.md). BitVoice is never required.

## How it is tested

- `make check`: gofmt, `go vet` and the Go unit suites (the ledger, the state rebuilt from the ledger equals the live state, recommendations, mark-outs, the map importer and router, NIBRS and the Virginia file, scripts, sites).
- `make web-test`: the web unit tests, including the manual checked against the screens.
- `make e2e`: Playwright against a real bitdispatcher with demo data and the BitVoice simulator: sign-in, call to incident to dispatch, emergencies both ways, the mobile terminal, mark-outs and status checks, and **three emulated monitors**, each screen landing on its own, the selection shared across windows.
- Golden files for i3 and EIDO generated by BitVoice's own encoder.

## Status

**Version 0.0.0: built and tested, not yet in service anywhere.** It is on a lab and design-partner track, like BitVoice's public-safety side. Before a centre relies on it:

- **Not certified.** Not CJIS-audited, no VCIN certification yet, and the Virginia IBR file has not yet been accepted by the State Police.
- **Tested against simulators and samples:** NCIC (training simulator), BitVoice (its simulator and its encoder's output, not yet a live BitVoice), ALI (sample spills).
- **Multiple monitors** tested with Chrome's emulated monitors, not yet on a real console's monitors.
- **Transcripts, crash data and drones** tested with the simulator's callers, crash notifications and drone system, not yet a live speech-to-text engine, telematics provider or drone system. The phrasebook translates the language of emergencies, not everything; the injury-risk rating and the quality review are aids, not judgements.
- **Open decisions:** when an ALI-only call (no hang-up signal) should end; whether agency scripts may reach outside the sandbox; VGIN's terms for its data outside Virginia.

See [docs/DESIGN.md](docs/DESIGN.md) for the design, the rules and the phases, and [docs/OPERATIONS.md](docs/OPERATIONS.md) for the ledger and how reports count.

## Documentation

| | |
|---|---|
| [**Manual (PDF)**](docs/manual/bitdispatcher-manual.pdf) | Every screen, with worked examples. Also `docs/manual/index.html`, and served at `/manual` and under Help by a running instance |
| [**Design**](docs/DESIGN.md) | What bitdispatcher is, its rules, its scope and the phases |
| [**Operations**](docs/OPERATIONS.md) | The event ledger, the reports and their counting rules |
| [**BitVoice**](docs/BITVOICE.md) | Every BitVoice hook and how bitdispatcher uses it |

## FAQ

**Is bitdispatcher really free? What is the licence?**
Yes. **Apache License 2.0**: commercial use, modification, distribution and private use are all permitted, with no fee and no per-seat or per-console pricing. See [LICENSE](LICENSE) and [NOTICE](NOTICE).

**Do I need BitVoice?**
No. Any call-handling system that sends ALI spill, i3 location or EIDO works. BitVoice adds the live call stream, radio and video.

**Do I need the internet?**
No. The map, routing, address search, aerial photos and county GIS are recorded into bitdispatcher's own store and used locally. Outside the recorded area it can show online tiles, marked as such.

**Do I need a database or an application server?**
No. One binary; the ledger is a file and the recorded map is SQLite, both in its data directory.

**Can a demonstration server let visitors in with one click?**
Yes: tick **Public demonstration account** on a viewer, dispatcher or supervisor in Admin › Users, and the sign-in page offers **Sign in as the demo user**, with no password. Never an administrator; only for a server that is meant to be public.

**What is the demonstration running on?**
One Lightsail instance with 1 GB of memory, with the 100-mile recorded map, Virginia's address points and the aerial photos; the BitVoice simulator beside it sends the 911 calls, ALI and radio traffic. See [deploy/demo](deploy/demo/README.md).

**Can we use it in a live 911 centre today?**
Not yet: see [Status](#status). It is ready for a centre to try beside its current CAD, on training positions, with simulated and recorded traffic.

**Can we move from our current CAD?**
Admin › Migration takes an earlier CAD's premises, address history and run cards from CSV exports; the address history shows with every incident.

**How do I report a bug?**
Open a [GitHub issue](https://github.com/biodream-llc/bitdispatcher/issues) with the output of `bitdispatcher -version` and what you did. Never include real caller or incident data.

---

## Contributions are not accepted

This is deliberate, and it is not a judgement about anybody's code. bitdispatcher is developed and maintained by BioDream LLC, and it stays that way. **Pull requests will not be reviewed or merged.**

The reason is the subject matter. This software sends help to people in emergencies, and every behaviour has to be verified, recorded and explainable. That standard is slow to enforce across contributors, and a half-enforced standard in a 911 system is worse than none.

**Bug reports are wanted.** A clear report of something that does not work is more valuable here than a patch.

### What the licence permits regardless

Apache 2.0 gives you the right to use, modify and redistribute this software, and closing this repository to contributions does not take that away. You may take a copy, change it to suit you, and run or publish the result, commercially or otherwise.

## Licence

[Apache License 2.0](LICENSE). Copyright 2026 BioDream LLC. See [NOTICE](NOTICE) for the third-party components and data sources.

bitdispatcher is not affiliated with or endorsed by NENA, the FBI's CJIS Division, the Virginia State Police or VGIN. Those names are used only to describe the standards and data it works with.

**Keywords**

**Category:** computer-aided dispatch · CAD · 911 CAD · open source CAD · free CAD software · public safety software · emergency dispatch software · PSAP software · dispatch console · ECC software · emergency communications centre

**Dispatch:** fire dispatch · EMS dispatch · police dispatch · run cards · unit recommendation · AVL · move-up and cover · station status board · mark-outs · officer status checks · supervisor board · multi-monitor dispatch · mobile data terminal · MDT · MDC

**Standards:** NG911 · NENA i3 · EIDO · NENA-STA-024 · PIDF-LO · ALI spill · NCIC · NIBRS · Virginia IBR · VCIN · CJIS

**Map:** offline dispatch map · OpenStreetMap · road routing · county GIS · address points · hydrants · aerial imagery · VBMP · VGIN

**Deployment:** single binary · Go · Golang · Apache 2.0 · self-hosted · on-premise · air-gapped · Linux · macOS · Windows · Virginia
