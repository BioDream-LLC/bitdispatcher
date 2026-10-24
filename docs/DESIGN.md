# bitdispatcher design

Status: draft, 2026-10-06.

## 1. What bitdispatcher is

A computer-aided dispatch (CAD) system in Go. It turns calls into incidents, recommends and assigns units,
tracks unit status and location, shows everything on a map, and records every action. It is one binary
with a built-in web interface.

bitdispatcher is stand-alone. It does not need BitVoice, and BitVoice does not need it; they meet only
through published standards.

## 2. Rules

1. **Own the code.** Go standard library only for the Go code. Any exception needs a written reason; the
   exceptions so far are the two script engines (section 5.1) and `modernc.org/sqlite` (BSD-3-Clause, pure
   Go, no cgo) for the recorded map (section 3.5), the same driver Perfuse and BitVoice use, and
   `github.com/ledongthuc/pdf` (BSD-3-Clause, pure Go, pinned) to read the text of PDF attachments for
   search: a reader of real PDFs (fonts' Unicode maps, compressed object streams) is too large to own
   well, and it runs on uploaded files only in its own goroutine, bounded in time and output. The event
   ledger stays a standard-library file.
2. **Never our own crypto primitives.** TLS, encryption at rest and signatures use the standard library.
3. **911 reliability.** No single point of failure; a site that is cut off keeps working.
4. **Standards at every boundary.** No private APIs to call handling, radio, RMS or state systems.
5. **The dispatcher decides.** No feature dispatches a unit, closes an incident or changes a priority on
   its own.
6. **Small agencies first.** Everything a small agency needs, done reliably, before features that exist
   only for large multi-agency deployments.
7. **Everything in the GUI.** Every setting and every operation (run cards, response areas, call types,
   units, stations, users, interfaces, reports) can be done from the web interface. A feature is not
   finished until its screen exists. Configuration files and the API reach the same state, never the
   only way.
8. **API first.** The web interface uses the same public API (REST plus a server-sent event stream) that
   scripts and outside programs use.
9. **Every pixel works for the dispatcher.** The dispatcher screen has no decorative or empty space.
   Every region shows something the dispatcher can act on or needs to know: queues, timers, unit
   status, caller location and its uncertainty, link health, emergencies. Empty states say what is
   being watched ("No pending calls, 14 units available"), never just "nothing here".
10. **Apache-2.0, all of it.** Every line and every bundled component is Apache-2.0 or compatible
    (MIT, BSD, ISC). No PolyForm or other source-available code (so none of BioDream's PolyForm reasoning engines): the
    evidence-rules engine (section 3.3) is written here, independently.
11. **BitVoice to the full, never required.** When the call-handling system is BitVoice, every hook it
    offers a CAD is used (docs/BITVOICE.md). Without BitVoice, the same features work from any
    standards-compliant source or are shown as unavailable.

## 3. Scope

### 3.1 Core

- **Incidents.** Create from a call or by hand, call types and priorities, narrative and timeline,
  linked calls, premise hazards and history at the address, close-out codes.
- **Units.** Status (available, dispatched, en route, on scene, transporting, out of service), timers,
  AVL positions, crews and capabilities.
- **Unit recommendation.** Run cards by call type and response area, drive-time routing over the road
  network (OpenStreetMap or the county's own road data), closest-available and capability matching,
  move-up and cover when a station empties. The recommendation is shown with its reasons; the
  dispatcher accepts or changes it.
- **Map.** Incidents, units, response areas and premises, drawn from the recorded map (section 3.5) and,
  later, the customer's GIS data.
- **Event ledger.** Every action that takes attention, changes state, creates accountability or feeds a
  report is an append-only, attributed event; reports, training feeds and analytics are built from it
  (docs/OPERATIONS.md).
- **Audit.** Every action time-stamped and attributed, tamper-evident, exportable for review and
  public-records requests.

### 3.2 Interfaces

- **Call handling:** ALI spill (the serial-over-IP and TCP formats CAD systems already read), NENA i3
  location (PIDF-LO), EIDO in and out. With BitVoice, also its event stream, ringing-call queue, rebid,
  text-to-911, caller video and radio data (docs/BITVOICE.md).
- **State and federal records (NCIC/CJIS):** through each state's message switch, one state at a time.
  The software supports the CJIS Security Policy controls (MFA, FIPS 140 validated encryption, audit,
  session limits); compliance itself belongs to each agency, and BioDream staff with access sign the
  CJIS Security Addendum.
- **Radio:** unit status, emergency-button alerts and radio GPS first; P25 console interfaces later.
- **Mobile:** a browser-based mobile data terminal for units in the field.
- **RMS:** built in (section 3.4).

### 3.3 Advisory features

An advisory panel beside the dispatcher. It never acts on its own.

- Flags calls that may be the same incident, for the dispatcher to confirm. Never merges silently.
- Shows where machine evidence (crash notification, alarm panels, gunshot detection, telematics)
  disagrees with callers.
- Never treats the absence of an alarm or a report as evidence that nothing is wrong.
- Reviews dispatch protocols for steps where a single person is the only safeguard.
- Runs in shadow mode at a pilot site, with output hidden, before any dispatcher sees it.

**Evidence rules (the codebook).** The panel's reasoning comes from a written, versioned codebook of
rules saying which evidence outweighs which when sources conflict: callers, machine evidence, unit
reports and records. For example, a driver on the line saying they are unhurt outweighs a crash alert's
severity estimate for that vehicle, but not for its passengers; and a silent alarm panel never outweighs
a caller who can see smoke.

- **Explains itself.** Every suggestion shows the evidence it relied on, the evidence it assumed was
  absent, and the rules that applied, in plain words a reviewer can check afterwards.
- **Deterministic.** The same evidence always gives the same suggestion. It does not learn from past
  incidents or change behaviour on its own.
- **"Cannot determine" is an answer.** When the rules do not settle a conflict, the panel says so and
  the dispatcher decides, exactly as they would without it.
- **Edited in the GUI** like every other setting: versioned, tested against recorded incidents in
  shadow mode before going live, every change audited, one-click rollback. Agencies start from a
  supplied codebook and adapt it to their own protocols.
- **Advises only** (rule 5). It never dispatches, merges incidents, closes calls or changes a priority,
  and it can be switched off entirely.

### 3.4 Records management (RMS)

A built-in law-enforcement records module, switched on per agency (fire/EMS-only agencies leave it off).
CAD and RMS share one record from first call to closed case: no export, no re-keying, one audit trail and
one CJIS boundary.

- **Incident and offence reports** created from the CAD incident: narrative, offences, people, property,
  vehicles, with a supervisor approval workflow.
- **Master indexes** of people, vehicles, locations and organisations shared by CAD and RMS, with
  duplicate merging that a person confirms.
- **Arrests and bookings**: charges, custody, mugshot and fingerprint references.
- **Property and evidence**: chain of custody, barcodes, disposition.
- **Case management**: assignments, follow-ups, warrants.
- **Citations, crash reports and field interviews** in each state's forms.
- **NIBRS** reporting to the FBI, in the national format and each state's variant.
- **Report writing in the field** through the browser mobile terminal.
- **Public records release** with protected fields redacted.

Fire reporting (NERIS) and EMS patient care reports are separate later modules, not part of RMS.

### 3.5 The recorded map

Decided 2026-10-18. bitdispatcher does not depend on a map service. An administrator chooses a centre and
a radius (100 miles by default) in Admin › Map; bitdispatcher downloads the OpenStreetMap extracts that
cover the circle (Geofabrik state extracts, checked against their published checksums), reads them with
its own PBF reader (`internal/osm`, standard library) and records what lies inside the circle into its
own SQLite store (`internal/mapdata`). After that the internet is not needed.

- **One store, four uses.** Vector tiles in a dispatch style made from the store (`/api/map/tiles`),
  road routing over the recorded network (one-way streets, posted or typical speeds), address,
  intersection, street and landmark search, and "what is here" for a point.
- **Format.** Features with a compact delta-encoded geometry in three R-trees by zoom band, an address
  table, an FTS5 name index, intersections of differently named roads, and the road graph as one blob.
  Tiles are made on first request and kept in the store.
- **Refresh on demand only.** Recording again is an administrator action, recorded in the ledger
  (`map.recording`, `map.recorded`, `map.failed`). Each recording is a new file; the map in use is
  replaced only by a complete one.
- **Beyond the recorded area** the console shows the online tile source from Settings, muted, with the
  edge of the recorded map drawn and labelled. Drive times outside it are straight-line estimates and
  say so.
- **Look.** The dispatch style (`web/src/console/mapStyle.ts`) has night and day palettes: roads by
  class with casings, route shields, one-way arrows, county and town lines, house numbers, hydrants and
  public-safety places in their own colours. Fonts (Noto Sans, SIL OFL) are built into the binary.
- **Development map.** Roanoke, Virginia, 100-mile radius (Virginia, West Virginia, North Carolina): about
  134,000 road miles, 398,000 addresses and 750,000 buildings, recorded in about 90 seconds into a
  620 MB store.

### 3.6 Aerial photos

Built 2026-10-18 (internal/imagery). Dispatchers navigate by rooftops, so photos become a base layer recorded like the
map, with the dispatch roads, labels and house numbers drawn over them.

- **Sources, best first, chosen by where the area lies.** The state's own programme where there is one:
  for Virginia, VGIN's Virginia Base Mapping Program (1-foot statewide, 6- and 3-inch in places, spring
  2022 to 2025, served as Web Mercator tiles to zoom 19 at
  `vginmaps.vdem.virginia.gov/arcgis/rest/services/VBMP_Imagery/MostRecentImagery_WGS/MapServer`).
  Elsewhere USGS The National Map (`USGSImageryOnly`, NAIP and partner imagery, public domain, tiles to
  zoom 16). Each source's terms of use are confirmed before it is offered.
- **Tiered detail to keep the store a sensible size.** Full detail (zoom 19, rooftops and cars) within
  a core radius chosen by the administrator (20 miles by default, about 10 GB); zoom 17 to the edge of the
  recorded map; the online photos beyond. A full 100-mile circle at zoom 19 would be about 250 GB.
- **Polite recording.** Tiles are fetched with a fixed, low request rate and a clear User-Agent, resumed
  after interruption, and kept in their own store file beside the map.
- **On the console.** A Photos button switches the base; the roads become thin casings and the labels
  gain dark halos so they read over any photo. The photo date is shown, so no one mistakes an old photo
  for today.


- **Safety case as data.** Every reliability claim names the test that proves it and what that test does
  not cover. The build fails if a claim has no test or a test backs no claim.
- **Never silent, confident and wrong.** Missing or stale data (no location, a unit not reporting, a
  partitioned site) is shown as missing or stale, never as current.
- **Failure falls back to the status quo.** If an assisting feature fails, the dispatcher is left exactly
  where they would be without it.
- **Bounds, not samples.** Where timing matters (recommendation, failover), state and test a worst-case
  bound, not the best measured run.
- **Shadow mode first** for every assisting feature.

## 5. Web interface

Held to the same standard as BitVoice (BitVoice DESIGN.md 4.5) and Perfuse:

- React, TypeScript, Tailwind and Vite with pinned versions, built into `internal/web/dist` and embedded
  with `go:embed`. The bundle is committed; the build fails if it is missing and CI fails if it is stale.
  No CDN at runtime.
- Dark high-contrast theme by default, plus light and night, from one colour ramp redefined per theme.
- Keyboard first: every action has a shortcut, plus a command palette. WCAG 2.2 AA; colour is never the
  only signal of status.
- Screens: dispatcher (incident queue, incident, units, map), supervisor wallboard, mobile terminal,
  admin, reports.
- Maps work offline from local tiles and the customer's GIS data.
- Vitest unit tests and a Playwright sweep of every screen that exercises each feature, not just loads it.
- Screenshots and demos set an explicit instance name and are checked for hostnames and local paths.

### 5.0 The dispatcher screen

One screen, no page changes during a call, laid out for a 1920×1080 or wider console monitor and still
usable at 1366×768:

- **Status bar** (top): position and user, local time and UTC, BitVoice and every interface's health with
  the age of its last message, counts of pending calls, active incidents and available units by type,
  and any open emergency.
- **Emergency banner**: a radio emergency or panic alert takes the full width in red until a person
  acknowledges it; it is never hidden by a dialog.
- **Call and incident queue** (left): ringing and answered 911 calls from call handling (callback,
  class of service, location and its uncertainty, text and video badges, age), then pending and active
  incidents by priority and age, each with its timers.
- **Map** (centre): incidents, units by status, caller location with its uncertainty circle, stale
  positions drawn as stale.
- **Incident panel** (right): call type, priority, location, caller, timeline, assigned units with their
  status timers, the recommendation with its reasons, and caller actions (rebid, text, video).
- **Unit board** (bottom): every unit as a tile with status, time in status, incident, location age and
  last radio contact; overdue timers stand out.
- **Command line** (bottom edge): the keyboard path for everything (`D E1 M2`, `OS E1`, `N text`),
  with completion and the result of each command.


### 5.1 Scripting (built 2026-10-19: internal/script, script rules in the advisory codebook)

Sites change behaviour with a script in the GUI rather than a vendor change request. Modelled on
Perfuse's script package and identical to BitVoice's (BitVoice DESIGN.md 4.6).

- **JavaScript and Lua**, behind one shared definition of what a script sees, may do, how long it may
  take and what its result means. Engines: goja and gopher-lua, pinned, pure Go (the named exceptions to
  rule 1).
- **Sandboxed** (no file system, process, environment, network or debug access unless an explicit,
  audited binding grants it) and **always time-limited**.
- **Hooks.** Incident validation and enrichment, call-type and priority suggestions, adjustments to unit
  recommendation, notifications, reports, outbound integrations.
- **A script advises, it never acts** (rule 5). It can suggest a priority or reorder a recommendation;
  the dispatcher still decides. If a script errors or times out, the CAD behaves exactly as it would
  without it, and says so on screen.
- **In the GUI.** Browser code editor, testing against recorded or synthetic incidents before saving,
  versions with one-click rollback, shadow mode before going live, every change audited.

### 5.2 Multi-tenancy

Tenants with their own units, stations, run cards, users and admins on one cluster: for a BioDream-hosted
service, and for regional consortia where several small agencies share one CAD.

## 6. Open decisions

Operational and reporting questions are listed in docs/OPERATIONS.md section 11.


- **Hosting.** A BioDream-hosted, CJIS-compliant service (for example AWS GovCloud) for agencies without
  IT staff, alongside self-hosting.
- Repository hosting: local server now; GitHub under BioDream-LLC later.
- **Ending ALI-only calls.** A call known only from the ALI spill has no end event; when it should leave
  the queue (a fixed time, when dismissed, or when its incident closes) is undecided.
- **Terms for state imagery outside its state.** VGIN's photography is offered to Virginia localities;
  other states' programmes need their terms checked before they join the photo catalogue.
- **Script bindings that reach outside** (notifications, outbound integrations): each needs an explicit,
  audited binding; none is offered yet, so scripts only advise.
- **Virginia certification**: the VCIN interface certification with the Virginia State Police, and a
  test IBR file to their Data Analysis and Reporting Team before the first production upload.

Decided since (2026-10-18): multi-site replication (section 3.7), map refresh by change files and county
GIS import (section 3.5), aerial photos (section 3.6). On 2026-10-19: **Virginia is the launch state**:
VCIN for NCIC, Virginia's incident-based reporting format (section 3.9), VGIN data for maps and
addresses.

### 3.9 Built on 2026-10-19

- **Virginia incident-based reporting** (internal/ibr): the month's approved reports as the FBI NIBRS
  flat file with Virginia's additions (2025.2 Virginia IBR technical specification), for the Virginia
  Crime Repository; the report screen checks the matching edits.
- **Centreline address ranges** place numbers without points of their own and give a road point its
  number; **Overture addresses** read straight from Overture's GeoParquet by byte range; CSV layers.
- **Premises and hazards**, shown with every incident at a place, and **migration** of an earlier
  system's premises, history and run cards from its CSV exports.
- **Scripting** (section 5.1): JavaScript and Lua script rules in the advisory codebook, for incidents,
  recommendations and reports.
- **Screens for several monitors** (from a 911 dispatcher's review): each position opens chosen screens
  (incidents and call entry, fire houses, police status and mark-outs, units, map, incident, supervisor
  board) as windows sharing one sign-in and one selection (BroadcastChannel), each optionally limited to
  disciplines; placed by the Window Management API where the browser has it, else where last left; kept
  per position on the workstation. **Mark-outs** are a unit status (`markout`) with a kind, place and
  note; every kind has its own status-check time, and **status checks** (`unit.checked`) restart a
  unit's timer. The **supervisor board** counts open, waiting and free units by the county's zones, or by
  station area (Voronoi of the stations) when none are loaded.

## 7. Phases

1. **Core CAD.** Event ledger and taxonomy first, then incidents, units, map, run cards, audit, web
   interface, ALI spill in, dispatcher activity and the Daily Operations Report. Usable by a
   small fire/EMS agency with no NCIC need.
2. **Recommendation and failover.** Drive-time routing, move-up and cover, two-site active-active
   (built; field trial pending).
3. **NG911 and mobile.** PIDF-LO, EIDO, mobile data terminal, AVL (built; certification pending).
4. **Law enforcement.** One state's NCIC switch, CJIS controls, RMS incident reports, master indexes
   and approval workflow, NIBRS for the launch state.
5. **RMS depth.** Arrests, property and evidence, case management, citations and crash reports.
6. **Advisory panel and radio.** Advisory features in shadow mode; radio status and emergency alerts.
7. **Certification track.** State approvals, CJIS audits at customers, SOC 2 for BioDream LLC.
