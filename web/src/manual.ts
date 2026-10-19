// The built-in manual, in the same form as BitVoice's: each article is short markdown (headings,
// paragraphs, lists, `code`, **bold**, [links](#/route) to screens and [links](#article) to chapters, > notes,
// | tables, ::figure lines). `route` ties an article to the screen whose Help link opens it. Help shows it
// in the product; `make manual` prints the same articles as docs/manual/index.html and a PDF.

export interface Article { id: string; title: string; route?: string; keys: string; body: string }

/** The manual's parts, in reading order. Every article is in exactly one part. */
export const parts: { title: string; intro: string; ids: string[] }[] = [
  { title: 'Dispatching', intro: 'The dispatcher\u2019s screen and everything done from it: calls, incidents, units, the map and the command line.',
    ids: ['start', 'console', 'screens', 'screen-calls', 'screen-stations', 'screen-police', 'screen-units', 'screen-map', 'screen-incident', 'calls', 'incidents', 'recommend', 'advisory', 'moveup', 'units', 'map', 'photos', 'command', 'keys'] },
  { title: 'In the field', intro: 'The mobile data terminal a crew uses from the vehicle.', ids: ['mdt', 'rms', 'ncic'] },
  { title: 'Supervising', intro: 'Reports built from the ledger, and the ledger itself.', ids: ['board', 'reports', 'admin-advisory', 'ledger'] },
  { title: 'Setting up', intro: 'Everything is set up on these screens; there is no configuration file to edit.',
    ids: ['admin-units', 'admin-stations', 'admin-calltypes', 'admin-map', 'admin-gis', 'admin-settings', 'admin-integrations', 'bitvoice', 'admin-users', 'admin-sites', 'admin-premises', 'admin-migrate', 'help'] },
]

export const manual: Article[] = [
  {
    id: 'start', title: 'Getting started', keys: 'overview first steps install begin introduction',
    body: `bitdispatcher is a computer-aided dispatch (CAD) system for 911 centres: one program with its own web interface. It creates and tracks incidents, recommends units by drive time over the road network, shows units, calls and incidents on a dispatch map with aerial photos, gives field crews a mobile terminal, and records every action in a tamper-evident ledger that reports are built from.

::figure lifecycle

## The first hour
1. **Sign in** as the administrator with the password printed on first start. The position is the console you are at.
2. **Record the map**: Admin › Map, choose the centre and radius, press Record. About two minutes for 100 miles. [Open Map](#/admin/map)
3. **Stations and units**: where units are based and what they are. [Open Stations](#/admin/stations) [Open Units](#/admin/units)
4. **Call types and run cards**: what each kind of incident needs. The defaults are a sensible start. [Open Call types](#/admin/calltypes)
5. **Users**: dispatchers, supervisors and field crews. [Open Users](#/admin/users)
6. **Call handling**: connect BitVoice, or point any call-handling system's ALI spill and EIDO at bitdispatcher. [Open Integrations](#/admin/integrations)

> bitdispatcher advises; the dispatcher decides. No feature dispatches a unit, closes an incident or changes a priority on its own.

## Demo mode
Started with \`-demo\`, bitdispatcher seeds a fictional agency on the Roanoke, Virginia map (stations at real station positions, made-up units). With \`bvsim\`, a simulated BitVoice, calls ring and radios report positions; everything simulated is marked SIMULATED.`,
  },
  {
    id: 'console', title: 'The dispatcher screen', route: '/', keys: 'console screen layout dispatcher main panels',
    body: `Everything a dispatcher needs is on one screen, and every part of it shows something useful.

::figure console

| Area | What it shows |
|---|---|
| Status bar | Calls waiting, pending and active incidents, units available by discipline, the clock, and the health of every interface (BitVoice, radio, ALI, map, ledger) |
| Queue (left) | 911 calls, ringing first, with callback, location, uncertainty and age; incidents by priority with their timers |
| Map (centre) | Units by status, incidents, caller locations with uncertainty, road routes of recommended and assigned units with arrival times |
| Unit board (bottom) | Every unit by discipline: status, time in status, incident, AVL age; overdue timers in amber. **Drag a unit onto an incident** in the queue to send it; **right-click** a unit for its statuses, sending it to the selected incident, and moving it up |
| Right panel | The selected incident (recommendation, units, timeline, caller), or when nothing is selected the situation: what waits for units, overdue timers, [move-ups](#moveup), coverage by station, recent activity |
| Command line (bottom) | Type anywhere to start a command (see [The command line](#command)) |

Select an incident by clicking it, with \`[\` and \`]\`, or from the palette (\`Ctrl K\`). \`Esc\` clears the selection.`,
  },
  {
    id: 'screens', title: 'Screens on several monitors', keys: 'screens monitors multiple windows layout lens fire police discipline second screen multi-monitor',
    body: `A position can spread its work over as many monitors as it has. Each monitor shows one **screen**: a window of the same live picture, arranged for one job. All of a position's windows share one sign-in and one selection: choose an incident on the queue screen and it opens on the incident screen and the map.

Open **Screens** with the monitor button on the status bar, from the command palette, or with **Ctrl+Shift+S**.

| Screen | For |
|---|---|
| Incidents and call entry | 911 calls and incidents beside the incident being worked; F2 for a new incident |
| Fire houses | Every station, its apparatus, who is covering and the empty houses |
| Police status and mark-outs | Every car: what, where, how long, and when its status check is due |
| Unit status | The unit board, large |
| Map | The map alone, following the selection |
| Incident and situation | The selected incident in full, or what needs attention |
| Supervisor board | Totals at a glance, read-only |

- **Add a screen**, choose its **disciplines** (all, fire, EMS, police, other) and **Open** it. **Open all** opens every one.
- **Detect monitors** (Chrome and Edge) shows the monitors as the computer arranges them; give each screen its monitor and it opens there, filling it. In other browsers drag each window where you want it once: it opens there next time.
- The layout is kept on the workstation for the position, so the next shift at the desk gets the same screens. With **Offer to open these screens at sign-in**, the full console offers them in one click after signing in.
- The browser asks to allow pop-ups (and, in Chrome and Edge, window management) the first time.

## Disciplines
A screen limited to some disciplines shows their units, and the incidents that need them: an incident's own call type, the units sent to it, and the units its run card asks for (a crash with injuries is on the fire screen and on the police screen). The status bar shows the screen's disciplines as a coloured chip. Every screen still shows every **emergency** and every 911 call not yet on an incident.`,
  },
  {
    id: 'screen-calls', title: 'Incidents and call entry screen', route: '/screen/calls', keys: 'call entry incidents queue screen call taker',
    body: `The queue of 911 calls and incidents, with the incident being worked beside it at full height, the new-incident form (**F2**) and the command line. It is the call taker's and dispatcher's working screen when the map and units are on other monitors. Everything works as on the full console (see The dispatcher screen).`,
  },
  {
    id: 'screen-stations', title: 'Fire houses screen', route: '/screen/stations', keys: 'fire houses stations quarters empty station apparatus bays move up cover',
    body: `Every station as a card, with every unit that lives there or is covering there.

- **The bays**: one door per unit. Shut and green, it is in quarters and available; open, the stripe is the colour of its status.
- **The state**: IN QUARTERS (all home), PARTIAL, COVERED (a move-up is in) or EMPTY (no unit available in the house, with how long).
- **Tiles**: each unit's status, its time in it, and what it is doing (its incident, or where it is covering). Click, drag onto an incident or right-click as on the unit board.
- **Gaps**: a kind of unit the house should keep in quarters but does not, with the best **Move up** in one click (see Move-ups).
- Across the top: houses with a unit in, empty houses, units out, and each kind of unit available out of its total, with every status in a bar.`,
  },
  {
    id: 'screen-police', title: 'Police status and mark-outs', route: '/screen/police', keys: 'police status check mark out markout meal court fuel report traffic stop timer welfare check location',
    body: `Every police car in one table, with the map beside it: what it is doing, where, for how long, and when its **status check** is due.

- **Doing**: its incident (click to open it), or its **mark-out**: what it is doing on its own, such as a meal, court, fuel, a report or a follow-up.
- **Where**: the place given when it was marked out, its incident's address, or the nearest address to its AVL.
- **Status check**: a ring fills as the unit's time runs; amber near the end, red and flashing when the check is due, with how long past. A check is due after the status timer (Admin › Settings), or the mark-out's own time.
- **OK** records the status check (the unit is all right) and starts its timer again. **MO** marks it out; **AV** makes it available.
- The tiles across the top count available, on calls, marked out (by kind), out of service and checks due; click one to show only those.

## From the command line
- \`MO 2A12 MEAL 315 Church Ave\`: mark out, with where. Without a place the unit's AVL position is kept. Add a note after a slash: \`MO 2A15 COURT 315 Church Ave / Courtroom 2\`. \`MO\` alone lists the agency's kinds.
- \`SC 2A12\`: status check, all right. Several units at once: \`SC 2A12 2A13\`.

Mark-outs are also on every unit's right-click menu and in the unit details, and a crew marks itself out from the mobile terminal (**F7**). A mark-out that can be pulled (a meal, a report) is offered by the recommendation when no available unit can fill a line: it is never sent without a dispatcher choosing it. The kinds, their times and which can be pulled are set in Admin › Settings.`,
  },
  {
    id: 'screen-units', title: 'Unit status screen', route: '/screen/units', keys: 'unit status board large',
    body: `The unit board on its own, with larger tiles that also say what each unit is doing. Click, drag and right-click work as on the unit board of the full console.`,
  },
  {
    id: 'screen-map', title: 'Map screen', route: '/screen/map', keys: 'map screen monitor',
    body: `The dispatch map alone. It follows the incident or call selected on any of the position's screens, and draws marked-out units where they were marked out when their AVL is not fresh.`,
  },
  {
    id: 'screen-incident', title: 'Incident and situation screen', route: '/screen/incident', keys: 'incident panel situation screen',
    body: `The selected incident at full height (its calls, units, timeline, premises, recommendation and report), or, with nothing selected, the situation: what is waiting, status timers past due, move-ups and activity.`,
  },
  {
    id: 'board', title: 'The supervisor board', route: '/board', keys: 'supervisor board wall dashboard totals available waiting swamped busiest areas where situational awareness',
    body: `The whole picture at a glance, for a supervisor's second monitor or a wall display. It only shows: nothing on it changes anything.

- **Across the top**: 911 calls not yet on an incident (red when one is ringing), incidents waiting for a unit with their priorities and the oldest wait, active incidents, units available out of those in service, units marked out by kind, status checks due, and empty stations.
- **Where the work is**: the map shaded by how much is open in each area (open incidents plus twice those waiting), with each area's open, waiting and available units. Areas are the county's emergency service zones or beats when they are loaded (Admin › County GIS); otherwise each station's area, the part of the map nearest it. An area with incidents waiting and no unit free, or two waiting, is **SWAMPED**.
- **By discipline**: a ring of every status with the number free, and each kind of unit available out of its total.
- **Busiest areas**, worst first, and **new incidents** over the last six hours by priority.

Choose the disciplines across the top, or open it from Screens with them. **Full screen** is beside the clock.`,
  },
  {
    id: 'calls', title: '911 calls', keys: 'call ali eido pidf location callback ringing rebid queue',
    body: `Calls arrive from BitVoice's event stream, from EIDO pushed by any call-handling system, and from the ALI spill. The same call from several sources is merged into one line.

- **Ringing** calls show before they are answered, so the dispatcher sees what is coming.
- **Location**: the caller's position and uncertainty circle, the address when there is one, and **STALE** when the location is older than the setting in Admin › Settings. Rebid asks for a new one (BitVoice).
- **+ Incident** on a call creates an incident from it, with the location and callback filled in. Calls already attached show their incident.
- Text to the caller, a video link and live video are on the call when BitVoice offers them.`,
  },
  {
    id: 'incidents', title: 'Incidents', keys: 'incident create new close reopen priority note narrative timeline disposition',
    body: `**New incident**: \`F2\`, the New button, + Incident on a call, right-click on the map (the address is filled in), a search result's New incident, or \`NEW SF 2001 Crystal Spring Ave\` on the command line. An address the recorded map knows exactly is placed on the map at once, so units are ranked by road straight away; anything else stays text to be placed.

The incident panel shows the recommendation, the units on it with their times, the caller and the timeline. Notes (\`N text\`) and priority notes (\`PN text\`) go on the timeline; priority notes are shown to every dispatcher and unit.

**Close** with a disposition (\`CL Handled\`); units still on it become available. Supervisors can reopen (\`RO #26-000123 reason\`).`,
  },
  {
    id: 'recommend', title: 'Unit recommendations', keys: 'recommend run card closest unit drive time routing eta',
    body: `For the selected incident bitdispatcher builds a recommendation from its call type's run card: for each line (\"3 × engine\", \"1 × als\"), the available units with that capability, closest first. Units already on the incident fill lines first, and scarce capabilities are filled before common ones so a multi-role unit is not spent where another would do.

- **Drive times** come from the recorded road network: posted or typical speeds, one-way streets obeyed, from the unit's fresh AVL position or its station. The reason says so: \"1.7 mi by road, 4 min via Franklin Road\".
- Outside the recorded map, or with no road between, the time is a straight-line estimate and is marked \`~\`.
- A unit covering another station is ranked from where it is posted.
- Notes warn when no unit has a capability (consider mutual aid) or when sending the units would leave a station empty.

The recommended units are picked; **Dispatch** (\`Ctrl Enter\`, or \`REC\`) sends them. Click units to change the pick, **Use recommendation** to go back to it, or type \`D E1 M2\`. The ledger records what was recommended and what was sent.`,
  },
  {
    id: 'advisory', title: 'Advisory suggestions', keys: 'advisory suggestion duplicate words conflict evidence rules codebook accept reject',
    body: `Beside the incident and on the Situation panel, the advisory panel may suggest something: two incidents that may be the same, a call that may be about an open incident, words that do not fit the call type (\"smoke\" on a fire alarm, \"not breathing\" on a fall), a repeat address, a priority 1 still without a unit, a unit on scene not heard from.

- **Why**: open a suggestion to see the evidence it relied on (with its source: unit, caller, dispatcher, machine), what it assumed absent, and the rule in plain words.
- **Cannot determine** is an answer: when sources of equal weight disagree (\"smoke\" and \"no smoke\" from two callers), it says so and you decide.
- **Accept** runs the suggested command as you (change the type, attach the call, dispatch the recommendation), or just notes it. **Reject** with a one-click reason; it is not shown again. Both are recorded.
- It advises only. It never merges, retypes, reprioritises or dispatches by itself, and core dispatch works the same when it is off or down.`,
  },
  {
    id: 'moveup', title: 'Move-up and cover', keys: 'move up cover coverage station empty relocate post return',
    body: `When a station has none of a kind of unit it normally houses available (its engine is out on a call), the Situation panel shows a **Move-up** box with the move that fills the gap: the nearest unit of that kind by road, from a station that can spare one.

::figure moveup

- **Move E3 from ST3** posts the unit at the empty station in one click (\`MU E3 ST1\`). It stays available and is recommended from there.
- A move that would leave its own station empty is offered only after moves that empty nothing, and says \"empties ST4\".
- When the station's own unit is back, a **Home** button sends the cover home (\`RET E3\`).
- Which kinds are kept covered is set per unit type in Admin › Settings (front-line engines, ladders, medics, ambulances and patrol by default).`,
  },
  {
    id: 'units', title: 'Unit statuses', keys: 'status available dispatched enroute onscene transporting oos timers avl',
    body: `| Code | Status | Meaning |
|---|---|---|
| AV | Available | In quarters or in service, can be sent |
| DP | Dispatched | Sent, not yet acknowledged |
| ER | En route | On the way |
| OS | On scene | Arrived |
| TR | Transporting | Taking a patient to hospital |
| AD | At destination | At the hospital |
| OOS | Out of service | Not available |

Each status has a timer (Admin › Settings); a unit past its timer turns amber on the board and is listed under Status timers past due. Statuses are given on the command line (\`ER E1\`, \`AV E1 M2\`), from the unit's tile, or by the crew on the [mobile terminal](#mdt). AVL positions older than the stale setting are drawn dashed and not used to rank units.`,
  },
  {
    id: 'map', title: 'The dispatch map', keys: 'map search address intersection reverse right click route day night layers',
    body: `The map is drawn from bitdispatcher's own [recorded map](#admin-map): roads by class with route shields and one-way arrows, county and town lines, house numbers and hydrants at street level, fire, police, EMS and hospitals in their own colours.

- **Search** (top right): an address (\`2001 Crystal Spring Ave\`), an intersection (\`Orange & Williamson\`, \`and\`, \`/\` or \`at\`), a street, or a place. Arrow keys and Enter; New incident from a result. A house number not on file offers the nearest numbers on the same street.
- **Right-click** anywhere: what is here (the nearest address or road) and New incident here.
- **Routes**: for the selected incident, the road route of each recommended, chosen and assigned unit, with its arrival time on the unit.
- **Day / Night** switches the palette; **Photos** (\`F7\`) puts aerial photos under the streets; **Fit all** shows every unit and incident; OOS and Stations show or hide those.
- The dashed cyan line is the edge of the recorded map: beyond it the online map is shown, muted, and drive times are estimates.`,
  },
  {
    id: 'photos', title: 'Aerial photos', keys: 'photos aerial imagery satellite orthophoto rooftop vgin naip',
    body: `Dispatchers find places by rooftops (\"the white house behind the church\"). With **Photos** on (\`F7\` or the button on the map), aerial photos are the base and the streets, names and house numbers are drawn over them.

::figure photos

- Photos are recorded in Admin › Map: full detail near the centre, less detail out to the edge. Zooming in past what was recorded enlarges the photo below it, so the map never goes blank.
- The chip beside the button says when the photo at the map centre was taken, where the source publishes it (\"2023, 12 inch ortho\"): an old photo is never taken for today.
- Where nothing is recorded the server fetches the photo when asked and keeps it, unless that is switched off.`,
  },
  {
    id: 'command', title: 'The command line', keys: 'command line verbs keyboard type d rec er os new mu ret help',
    body: `Type a letter anywhere on the dispatcher screen and the command line takes it (or press \`/\`). \`Tab\` completes, arrow keys recall earlier commands, results stay on screen. \`HELP\` lists every verb.

| Command | Does |
|---|---|
| \`D E1 M2\` | Dispatch units to the selected incident (\`D E1 #26-000123\` names one) |
| \`REC\` | Dispatch the recommendation |
| \`ER E1\` · \`OS E1\` · \`TR M1\` · \`AD M1\` | En route, on scene, transporting, at destination |
| \`AV E1 M2\` | Available: clears them from their incident |
| \`OOS E5 brakes\` | Out of service with a reason |
| \`N text\` · \`PN text\` | Note, priority note |
| \`NEW SF 12 Main St\` | New incident (\`NEW CHEST P1 …\` sets the priority) |
| \`P 2\` · \`T VF\` | Change priority or call type |
| \`CL Handled\` · \`RO #26-000123 reason\` | Close, reopen |
| \`MU E4 ST1\` · \`RET E4\` | Move up to cover a station; send home |
| \`ATTACH <call> #26-000123\` | Attach a 911 call to an incident |
| \`ACK\` | Acknowledge the oldest open emergency |`,
  },
  {
    id: 'keys', title: 'Keyboard', keys: 'keyboard shortcuts keys hotkeys f2 ctrl k f7',
    body: `| Key | Does |
|---|---|
| Any letter | Starts a command on the command line |
| \`/\` | Focus the command line |
| \`F2\` | New incident |
| \`Ctrl Enter\` | Dispatch the units picked for the selected incident |
| \`Ctrl K\` | Palette: go to an incident, unit, screen or chapter of this manual |
| \`[\` \`]\` | Previous and next open incident |
| \`Esc\` | Clear the selection |
| \`F7\` | Aerial photos on or off |
| \`F9\` | NCIC inquiry, with the selected incident as the reason |
| \`F1\`–\`F6\` (mobile terminal) | En route, on scene, transporting, at hospital, available, out of service |`,
  },
  {
    id: 'mdt', title: 'Mobile data terminal', route: '/mdt', keys: 'mdt mobile terminal field crew vehicle laptop tablet gps',
    body: `A crew signs in on the vehicle's laptop or tablet with a **field** account (Admin › Users) and the unit's call sign as the position (\`E1\`). The terminal opens at once.

- **The assignment**: type, priority, address in large type, the road route drawn on the map with its time and the roads it takes, the callback, the other units on it, and the notes from dispatch, newest first. Priority notes stand out.
- **Status buttons**: one tap each, with the usual next status highlighted; \`F1\` to \`F6\` on a keyboard.
- **Notes to dispatch** go on the incident's timeline as the unit's.
- **GPS**: the device's position goes to bitdispatcher as AVL every few seconds (allow location when the browser asks).
- **Emergency**: hold the button for two seconds. Dispatch sees the emergency banner and acknowledges it.
- **Photos** on the map, as on the console.

A crew acts only as its own unit. Dispatchers can open the terminal at #/mdt to see what a unit sees (the position names the unit).`,
  },
  {
    id: 'rms', title: 'Incident reports (RMS)', route: '/rms', keys: 'rms report records nibrs offense victim suspect property narrative approve',
    body: `Officers and dispatchers write incident reports at #/rms, or with **Report** on an incident (the incident's location and time are filled in). A crew on a mobile terminal writes its own from the car.

- **Sections**: when and where (location type, whether it was cleared), offences, people, property, vehicles and the narrative. Codes are shown in words and picked in one click; the screen asks only for what the offence needs (a weapon for an assault, the method of entry for a burglary, the drug for a seizure, the injury and the relationship to each suspect for a crime against a person).
- **Saved as you type**: a draft saves itself a moment after typing stops.
- **Before it is submitted** lists what is still missing for incident-based reporting: the FBI's NIBRS data elements and edits, with Virginia's additions (property quantity, the licence plate reader indicator, drug types). Submit is available once the list is empty.
- **Approval**: a supervisor other than the author approves it, or returns it with what to fix. Every save and decision is a ledger event.

### The state file

Supervisors see **State IBR file** under the list. Choose the month and press **File**: bitdispatcher writes the month's approved reports in Virginia's incident-based reporting format (the FBI's position-based flat file with Virginia's additions, 2025.2 specification): Group A incidents with their offence, property, victim, offender and arrestee segments, Group B arrests, or a zero report when there is nothing. Upload it to the Virginia Crime Repository by the 15th of the next month. Reports that fail the checks are left out and listed. Making the file is a ledger event, with its SHA-256.

> The settings need the agency's nine-character ORI (Admin › Settings). Before the first production upload, the Virginia State Police ask for a test file by email (dart@vsp.virginia.gov).`,
  },
  {
    id: 'ncic', title: 'NCIC inquiries', keys: 'ncic vcin query wanted person vehicle plate gun article cjis hit confirmation',
    body: `**NCIC** (on the reports screen, F9 on the dispatcher screen, or on the mobile terminal) sends an NCIC 2000 inquiry through the state's message switch: wanted person (\`QW\`: name with date of birth, or name, sex and race), vehicle (\`QV\`: plate and state, or VIN), gun (\`QG\`) and article (\`QA\`) by serial.

- Each inquiry needs a **purpose** code and a **reason** (the incident number): CJIS requires both.
- The ledger keeps who asked, when, the purpose, the reason and whether a record came back. The terms are kept only as a fingerprint and the answer not at all.
- **A hit is a lead**: confirm it with the entering agency before any action.
- Administrators set the agency's ORI and the switch in Admin › Settings: off, the **training simulator** (plates, names and serials starting with TEST return a record; every answer says SIMULATED), or a switch adapter by TCP.`,
  },
  {
    id: 'reports', title: 'Operations report', route: '/admin/reports', keys: 'report daily operations shift debrief statistics response times certify supervisor',
    body: `Admin › Operations report builds the Daily Operations Report and shift debrief for a period from the ledger alone: nobody retypes what CAD already holds.

- **Periods**: this shift, last shift (from the shift change times in Settings), today, yesterday, last 7 days, or any range.
- **Contents**: 911 calls, incidents, dispatches and closes; incidents by hour, type, priority and disposition; response times (call to dispatch, dispatch to en route, call to on scene, on scene to clear) as median and 90th percentile; priority 1 incidents; incidents open at the end (the turnover); status timers run out, once per episode; emergency activations and how fast they were acknowledged; each person's actions; unit demand by phase and utilisation; system and configuration changes.
- **Drill-down**: click any number to see the ledger events it is made of.
- **Certify**: once the period has ended, a supervisor certifies it with optional context. The certification is a ledger event with the report's hash; if anything changes later, the report says it changed since it was certified.

> Activity counts intentional actions only and is never a score: it shows workload, not performance.`,
  },
  {
    id: 'admin-advisory', title: 'The advisory codebook', route: '/admin/advisory', keys: 'advisory codebook rules shadow live test precedence version restore script scripting javascript lua hook',
    body: `Admin › Advisory panel holds the codebook: the rules the panel uses, in plain words, with their thresholds.

- **Mode**: off; **shadow** (evaluated and kept in the ledger, shown to supervisors only, so the rules can be judged before anyone relies on them); **live** (dispatchers see suggestions). A new system starts in shadow.
- **Source precedence**: drag to order unit, caller, dispatcher and machine evidence, strongest first. A stronger source outweighs a weaker one; equal sources that disagree give \"cannot determine\". A silent alarm panel never outweighs a person who can see smoke.
- **Rules**: switch each on or off, set its severity, distances, times, words and the call types they already fit, and edit its explanation.
- **Test** replays the recorded ledger for the last day, week, month or quarter with the rules on screen, saved or not, and counts what each would have suggested.
- **Save version** keeps the change with a note; every version is listed and can be restored in one click. Decisions (accepted, rejected and why) are listed: rejecting a suggestion is never treated as an error.

### Agency scripts

**JavaScript script** and **Lua script** add a rule that is the agency's own code, versioned, tested and switched between shadow and live with the rest of the codebook. A script defines one function for its hook:

| Hook | Function | Returns |
|---|---|---|
| Each open incident | \`advise(i)\` | nothing, or \`{title, detail, severity, action: {label, command}}\` (or a list): a suggestion; accepting runs the command on the incident, as the person who accepts |
| Unit recommendation | \`recommend(r)\` | \`{prefer: [unit ids], notes: [text]}\`: a preferred unit among a line's next best takes a place; notes are shown with the recommendation |
| Incident reports | \`check(r)\` | a list of \`{field, message}\`: shown as **Agency checks** beside the state's; they advise and do not stop a submission |

An incident script sees the number, type and its name, priority, status, address and position, minutes since it was created, notes, units on it, hazards at its premises, caller name and number of calls. **Try it** runs the script once on the newest real incident (or report) and shows what it returned, what it logged with \`log()\`, and how long it took; **show the input** shows exactly what it saw.

- A script has no files, network, process, environment or code loading: only its input and \`log()\`.
- It always has a time limit (50 ms unless set, at most 1 s). It runs after the CAD's state is released, so a slow script never holds dispatch up.
- A script that fails or runs out of time is shown under its rule and in the test; the panel and the recommendation carry on without it and say so.`,
  },
  {
    id: 'ledger', title: 'Ledger and audit', route: '/admin/ledger', keys: 'ledger audit events hash chain verify tamper evident search',
    body: `Every action that changes state, takes attention or creates accountability is an event in an append-only ledger: who (user, role, position, session), when, what, on which incident, unit or call. Each event carries the hash of the one before it, so a change anywhere breaks the chain.

- **Verify** checks the whole chain and reports the first broken link, if any.
- **Search** by type, person or text; events are newest first.
- The current state is rebuilt from the ledger at every start, and reports are built from it.`,
  },
  {
    id: 'admin-units', title: 'Units', route: '/admin/units', keys: 'units apparatus call sign type station capabilities radio id',
    body: `Each unit has a call sign (\`E1\`, \`M12\`, \`2A31\`), a type (which gives its capabilities), a station, and optionally extra capabilities, the radio's unit ID and alias (so radio emergencies and positions find it) and an agency for mutual aid. New units start out of service.`,
  },
  {
    id: 'admin-stations', title: 'Stations', route: '/admin/stations', keys: 'stations quarters fire station base position',
    body: `Stations are where units are based. Their positions place units with no AVL, are where move-ups post covering units, and are where drive times start from when a unit's AVL is stale. Type the address and pick it from the recorded map's suggestions: the position fills in. Or type the position.`,
  },
  {
    id: 'admin-calltypes', title: 'Call types and run cards', route: '/admin/calltypes', keys: 'call types run cards priority capability requirement',
    body: `A call type is a kind of incident (\`SF\` structure fire, \`CHEST\` chest pain) with a default priority, a discipline and a run card: how many units of each capability it needs (\"3 × engine, 1 × ladder, 1 × als\"). Recommendations fill the run card. Capabilities come from unit types (Admin › Settings) and units.`,
  },
  {
    id: 'admin-map', title: 'Recording the map', route: '/admin/map', keys: 'map record openstreetmap geofabrik radius centre photos imagery offline',
    body: `bitdispatcher records the OpenStreetMap data for a circle around your centre into its own store, then draws the dispatch map, routes units by road and finds addresses from that store with no internet.

::figure map

1. Choose the centre (a preset, the console map centre, or a position) and the radius (100 miles covers a county and its mutual-aid neighbours).
2. The downloads the area needs are listed at once (state extracts from Geofabrik). Press **Record**.
3. Recording takes a few minutes; dispatching carries on with the current map, which stays until the new one is complete. Each recording is a ledger event.

**Bring up to date** applies OpenStreetMap's daily change files (Geofabrik publishes one a day per extract) since the map was recorded: a few megabytes instead of the whole extracts, then the store is built again. Record again for a new area, or when the map is more than a year behind; downloads already made are checked against their published checksums and reused.

## Aerial photos
Below the map, **Aerial photos** records photos around the same centre: full detail within the radius you choose (zoom 19: rooftops and cars) and less out to the edge (zoom 16). The estimate (tiles, size, time) follows the form; tiles already recorded are skipped, so a stopped recording carries on where it was. The sources and their terms are listed: VGIN's Virginia Base Mapping Program for Virginia, USGS The National Map (NAIP, public domain) elsewhere.`,
  },
  {
    id: 'admin-gis', title: 'County GIS', route: '/admin/gis', keys: 'gis county address points hydrants esz esn zones arcgis geojson import centreline centerline range interpolation overture',
    body: `The county's own GIS is more exact than OpenStreetMap where the county keeps it. Admin › County GIS imports its layers for the recorded map's area into a store of their own, kept when the map is recorded again.

| Kind | Used for |
|---|---|
| Address points | Searched first: an exact county address point wins over OpenStreetMap's; house numbers on the map |
| Hydrants | Drawn with their IDs at street level |
| Emergency service zones, beats | Drawn and labelled on the map |
| Common place names | Searched |
| Road centrelines | Their names are searched; with address ranges (from/to, left/right) they place a number that has no point of its own, on its side of the road, and give a point on a road its number. Shown as **county range: estimate** |
| Overture addresses | Open address points from the Overture Maps Foundation (in the US largely the National Address Database), for any area, without a county service. Used after the county's own, marked **Overture** |

1. Paste the layer's ArcGIS REST address (\`…/FeatureServer/0\`), choose a preset (Virginia's statewide address points and road centrelines from VGIN, or Overture's open addresses), or upload a GeoJSON file or a CSV with latitude and longitude columns. The layer is read at once: its name, how many records lie in the area, and a guess at the kind and the fields.
2. Check the field mapping against the sample values shown beside each field, and press **Add layer**.
3. **Import now** reads every layer that is on. The store in use stays until the new one is complete; each import is a ledger event.

Search results from county data are marked as such. ArcGIS layers are read with https only, in batches, so services that cap a query still give everything.`,
  },
  {
    id: 'admin-settings', title: 'Settings', route: '/admin/settings', keys: 'settings agency timers stale road factor speed tiles shifts unit types mark-outs markout',
    body: `- **Agency** name and incident number prefix.
- **Status timers**: minutes in each status before the dispatcher is prompted for a status check.
- **Mark-outs**: the kinds a unit can be marked out for (code, name, status-check minutes, which units, whether it can be pulled for a call). See Police status and mark-outs.
- **Stale** ages for AVL and caller locations.
- **Shift changes** (\`07:00 19:00\`) for the operations report's shift periods.
- **Travel estimates** outside the recorded map: road factor and average speed.
- **Map beyond the recorded area**: the online tile source (fetched and cached by the server, never by browsers), and the map's starting centre and zoom.
- **Unit types**: code, name, discipline, capabilities, and whether stations keep them covered (move-up suggestions).
- **Dispositions** an incident can be closed with.`,
  },
  {
    id: 'admin-integrations', title: 'Integrations', route: '/admin/integrations', keys: 'integrations bitvoice ali eido radio webhook token certificate',
    body: `How bitdispatcher connects to call handling and radio. Each integration shows its live state and a checklist of the hooks in use.

- **BitVoice**: its address and API token, and its certificate pinned with Read it. See [BitVoice](#bitvoice).
- **EIDO**: the token a call-handling system uses to push EIDO to \`/api/ng911/eido\`.
- **Radio**: the secret for radio webhooks to \`/api/radio/events\`.
- **ALI spill**: the TCP address bitdispatcher listens on, and Test.`,
  },
  {
    id: 'bitvoice', title: 'BitVoice', keys: 'bitvoice call handling integration hooks sse eido radio rebid text video',
    body: `bitdispatcher uses every hook BitVoice offers a CAD, and needs none of them: without BitVoice, ALI and EIDO from any call-handling system work the same way.

::figure bitvoice

| Hook | Way | Used for |
|---|---|---|
| Event stream | BitVoice → bitdispatcher | Calls ringing, answered, ended; locations; resync after a gap |
| EIDO | both | Incident data pushed on answer and media changes; pulled when missed |
| ALI spill | BitVoice → bitdispatcher | The classic location record, merged with the same call |
| Radio webhooks | BitVoice → bitdispatcher | Emergency button, acknowledgement, transmissions, GPS |
| Callbacks | bitdispatcher → BitVoice | Rebid, text to caller, video link, live video, emergency acknowledgement |

Delivery failures BitVoice reports show on the Situation panel.`,
  },
  {
    id: 'admin-users', title: 'Users and positions', route: '/admin/users', keys: 'users roles password viewer field dispatcher supervisor admin positions sessions',
    body: `| Role | Can |
|---|---|
| Viewer | See everything, change nothing |
| Field crew | The [mobile terminal](#mdt) for its own unit |
| Dispatcher | Work incidents, units and calls |
| Supervisor | Also reopen incidents, read the ledger and reports, certify reports |
| Administrator | Also change configuration and users |

Passwords are generated or at least 12 characters. Five wrong passwords lock an account for 15 minutes. The positions list shows who is signed in where.`,
  },
  {
    id: 'admin-sites', title: 'Several sites', route: '/admin/sites', keys: 'sites multi-site backup failover active active replication peer survivable',
    body: `bitdispatcher runs at two or more sites, all live at once: a main centre and a backup centre, or neighbouring counties sharing a system. There is no primary to fail over from.

- **Each site writes its own ledger** and copies the others' over https, every second, each event checked against its site's hash chain before it is kept.
- **The same picture everywhere**: every site applies all events in the same order (by time, then site), so sites that have the same events show the same state, whatever order they arrived in.
- **Cut off, a site keeps working** on its own; when the link returns the events are exchanged and both sites settle on the same state. If two sites changed the same unit meanwhile, the later change wins, at both.
- **Incident numbers** carry each site's prefix (N26-000001, S26-000001), so two sites never give the same number.
- **Setting up**: give this site a name and prefix and save; copy its token and certificate fingerprint to the other sites. Add each other site with its address, its token and its fingerprint (**Read it** reads the certificate; check it against the one shown on that site). The status bar shows how many sites are linked.

> A site's name and prefix are written into every event it makes, so they cannot change once in use.`,
  },
  {
    id: 'admin-premises', title: 'Premises and hazards', route: '/admin/premises', keys: 'premise premises hazard hazards caution officer safety knox box dog oxygen contacts key holder',
    body: `A premise is what the centre knows about a place before anyone calls: **hazards** to the crews (a dog, oxygen in use, an armed occupant, a Knox box), **notes** and **contacts**. It is shown with every incident within 40 m of it, on the dispatcher's incident panel and on the mobile terminal, hazards first in red. An expired premise (a temporary hazard past its date) is shown as expired.

- **Add a premise**: type the address and pick it from the suggestions (that places it), click the common hazards or type others, add notes and contacts.
- **Search** by address, name, hazard or id; click one to change or remove it.
- Supervisors and administrators change premises; everyone sees them. Each change is a ledger event.

Premises from an earlier system come in through **Migration**.`,
  },
  {
    id: 'admin-migrate', title: 'Migration from an earlier CAD', route: '/admin/migrate', keys: 'migration migrate import csv export earlier previous incumbent cad history run cards premises spreadsheet',
    body: `Admin › Migration brings an earlier CAD system's data over from the spreadsheets it exports, CSV or tab-separated (UTF-8 or Windows encoding).

1. **Drop the file** on the page. Its columns are matched to ours from their names, and every row is checked at once.
2. **Choose what it holds** if the guess is wrong: premises and hazards, incident history, or call types and run cards. Change any column in the table; the first row's value is shown beside it.
3. **Import** the rows that passed. The rows that did not are listed with why (no position and an address the map does not know, a date it cannot read).

| Kind | Needs | Notes |
|---|---|---|
| Premises and hazards | an address (or latitude and longitude) | hazards and contacts separated by ; — an address without a position is found on the map (county points, then address ranges, then the map's) |
| Incident history | number and date | counts as calls before at an address: the incident panel shows the last five within 40 m over ten years, and the advisory panel's repeat-address rule counts them |
| Call types and run cards | code and run card | "2 engine; ladder:1; als": capabilities and counts. A capability no unit type has is pointed out |

Importing the same file again replaces what it brought (premises by their id, call types by their code); history is added again, so import it once. Address points go to **County GIS**, which reads CSV files with latitude and longitude as well.`,
  },
  {
    id: 'help', title: 'This manual', route: '/help', keys: 'help manual documentation guide search',
    body: `This manual is built into bitdispatcher, so it is there wherever it runs, with no internet. Every screen's Help link opens its chapter; the palette (\`Ctrl K\`) finds chapters by name. The same chapters are printed as docs/manual/index.html and a PDF by \`make manual\`.`,
  },
]

/** The chapter for a screen, if it has one. */
export function articleFor(path: string): Article | undefined {
  return manual.find((a) => a.route === path)
}
