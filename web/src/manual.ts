// The built-in manual, in the same form as BitVoice's: each article is short markdown (headings,
// paragraphs, lists, `code`, **bold**, [links](#/route) to screens and [links](#article) to chapters, > notes,
// | tables, ::figure lines). `route` ties an article to the screen whose Help link opens it. Help shows it
// in the product; `make manual` prints the same articles as docs/manual/index.html and a PDF.

export interface Article { id: string; title: string; route?: string; keys: string; body: string }

/** The manual's parts, in reading order. Every article is in exactly one part. */
export const parts: { title: string; intro: string; ids: string[] }[] = [
  { title: 'Dispatching', intro: 'The dispatcher\u2019s screen and everything done from it: calls, incidents, units, the map and the command line.',
    ids: ['start', 'install', 'signin', 'taker', 'resources', 'console', 'layout', 'working-agency', 'console-route', 'dispatching', 'screens', 'screen-calls', 'screen-stations', 'screen-police', 'screen-units', 'screen-map', 'screen-incident', 'calls', 'incidents', 'recommend', 'advisory', 'moveup', 'units', 'map', 'road-closures', 'cameras', 'photos', 'views', 'messages', 'inbox', 'search', 'alerts', 'station', 'cad-to-cad', 'files', 'command', 'keys'] },
  { title: 'Locations, people, requests and review', intro: 'The place and how a location is settled, flags, persons and vehicles, NCIC requests, Street View, key tests, and the supervisor\u2019s review.',
    ids: ['place', 'flags', 'persons-vehicles', 'requests', 'streetview', 'keytest', 'supervisor', 'admin-callentry', 'admin-critical', 'admin-pip'] },
  { title: 'In the field', intro: 'The mobile data terminal a crew uses from the vehicle.', ids: ['mdt', 'rms', 'ncic'] },
  { title: 'Supervising', intro: 'Reports built from the ledger, and the ledger itself.', ids: ['board', 'reports', 'admin-advisory', 'ledger', 'records', 'admin-disclosure'] },
  { title: 'What 911 centres asked for', intro: 'Transcripts and translation, callers grouped and screened, crash data, drones, care for the people, training, quality review, outages and line attacks.',
    ids: ['transcript', 'translation', 'caller-location', 'location-link', 'grouping', 'nonemergency', 'admin-intake', 'crash', 'drones', 'admin-drones', 'radio-text', 'summary', 'care', 'breaks', 'training', 'exercises', 'quality', 'offline', 'continuity', 'overflow', 'tdos', 'hooks'] },
  { title: 'Fire ground, crews and the police desk', intro: 'Bigger fires, who is aboard each unit, roll calls, paging volunteers, tows, the schedule, the numbers to call, areas on the map, a unit\u2019s day, patient-care reports, papers to serve and alerts to the public.',
    ids: ['alarms', 'admin-personnel', 'staffing', 'admin-qualifications', 'accountability', 'admin-paging', 'admin-tows', 'schedule', 'admin-directory', 'admin-geofences', 'admin-roads', 'admin-cameras', 'admin-alarms', 'unit-history', 'epcr', 'papers', 'public-alerts', 'admin-publicalerts'] },
  { title: 'Setting up', intro: 'Everything is set up on these screens; there is no configuration file to edit.',
    ids: ['self-service', 'admin-setup', 'admin-golive', 'admin-support', 'admin-agencies', 'admin-operations', 'admin-units', 'admin-stations', 'admin-facilities', 'admin-calltypes', 'admin-plans', 'admin-map', 'admin-gis', 'admin-settings', 'admin-integrations', 'bitvoice', 'admin-users', 'admin-workstations', 'admin-sites', 'admin-continuity', 'admin-premises', 'admin-migrate', 'admin-guidance', 'admin-protocols', 'help'] },
]

export const manual: Article[] = [
  {
    id: 'place', title: 'Locations and the place', keys: 'location address verified estimated forced unverified map point caller place building floor apartment unit suite complex gate exception',
    body: `The incident location is the place responders go to, not only its street address.

## How the location is settled
| Choice | Means |
|---|---|
| Verified address point | Picked from the agency's address data and confirmed with the caller |
| Estimated (address range) | The address search placed it along a block range |
| The caller's device location | The phone's position, with no exact address |
| A point I placed on the map | Right-click the map |
| Forced for this incident | Not in the address data: new construction, a rural place, the data is wrong |
| Not verified yet | Typed and not checked |

**Forced for this incident** needs a point on the map and a reason from the agency's list (and a note when the agency asks). The address is kept exactly as typed with the map point, the nearest verified address or cross streets and the alternatives offered. It is never replaced by a nearby address. The call is sent at once.

Every location that is not a verified address point is labelled in capitals (**FORCED LOCATION**, **ESTIMATED LOCATION**) on call entry, the dispatcher's incident, the mobile terminal and in print, until the agency verifies it. Only duties the agency allows may force a location; a refused attempt is recorded on the incident and the call information stays as it was.

## The place
Business or building name, complex or campus, building, floor, apartment or room (the kind and the number) and entrance, gate or access detail are separate fields. They are searchable and they decide which flags apply. A change here never changes the agency's master address records.

## What happens next
Each such location opens a **location exception** and alerts the on-duty supervisor at once (see [Supervisor review](#supervisor)). When the agency later verifies or merges the address, the incident links to the master point; what was entered and what responders were shown stay on record.`,
  },
  {
    id: 'flags', title: 'Premise and safety flags', route: '/admin/flags', keys: 'flags premise hazard officer safety medical business keyholder unsafe structure domestic violence dangerous animal polling place evacuation scope version restricted',
    body: `A flag is the agency's warning about a verified address or a person: **premise, location, hazard, officer safety, medical, business, keyholder or maintenance, unsafe structure, domestic violence, dangerous animal, polling place, evacuation location** (the agency can rename, add or turn off categories).

- **Scope**: each flag is put at the narrowest correct place: the whole property, a complex, a named business, a building, a floor or one apartment. An apartment's warning is never shown for the rest of the complex.
- **When it shows**: as soon as the call-taker verifies the address, before the call is sent, as a line on every tab, then on the dispatcher's incident and the mobile terminal. Critical and high flags are in that line; the Hazards tab lists every one with what responders should do.
- **What it says**: severity and category in words, a title, what the concern is and why, and what responders should do. Open it for the source and details.
- **Restricted categories** (medical, domestic violence, officer safety, keyholder): those the agency does not allow see the warning and what to do, not the reason or source. Opening the detail is recorded; a refused request is too.
- **Versions**: changing, renewing, retiring or restoring a flag makes a new version. Each incident keeps the version it showed, so later changes never rewrite what dispatchers saw.
- **Unverified locations** never get a permanent flag. Flags near an unverified point show only as **POSSIBLE MATCH NEARBY**; an incident-only note can be kept instead.
- **Who may**: each category says which duties create, approve and see it. Others (and trainees) propose; a supervisor approves or rejects. Two flags of one category on one scope are shown for a decision rather than one overwriting the other.

Add a flag from call entry (Hazards, **Add a flag or a note for this place**) or here in Admin, where proposed flags wait for approval. [Open Flags](#/admin/flags)`,
  },
  {
    id: 'persons-vehicles', title: 'Persons and vehicles', keys: 'persons vehicles first middle last name suffix alias phone description ssn social security licence date of birth age officer safety plate state vin owner driver return dmv ncic tow',
    body: `## Persons, top to bottom
Role (more than one when it fits), then **First, Middle and Last name** in separate fields, suffix and aliases; where the person is (the incident, somewhere else with its building, floor and unit, unknown, transient); telephone numbers (several, with type, extension, source and whether verified); the description together (sex, race, ethnicity, height, weight, build, hair, eyes, clothing, scars, marks and tattoos, with the approved codes); then the identifiers: Social Security number, operator's licence and state, other identification, the date of birth and, separately, a reported approximate age. Age is worked out from the date of birth. Notes last.

**Officer-safety concern** opens a required section: a short alert title, what happened and why it is a concern, weapons or threat, what responders should do, the source and its reliability. It cannot be saved without them. Choose **This incident only** or **Propose a permanent warning**: a proposal goes to the agency's review as a person flag. Everywhere the alert shows as **OFFICER SAFETY:** and its title, and opens to the reason, source and instruction.

**Search** each name part on its own (First name **John** finds every John), with the date of birth, approximate age, phone, address, licence, sex, race, a linked plate, sounds-like and partial matching. **Possible same person** shows while you type a name and never blocks saving.

Social Security and licence numbers show masked on every shared screen. **Open (recorded)** shows them to the duties the agency allows, with the purpose recorded.

## Vehicles, top to bottom
Roles (suspect, victim, stolen, recovered, traffic stop, evidence, towed and more), the plate with the agency's state first, the plate type and condition (temporary, partial, unreadable, none, several), the VIN (checked as you type; a partial VIN is kept with a note), then **Run DMV** or **Run NCIC** once the plate and state or a full VIN are in. The return attaches to that vehicle with its time, who ran it and who asked. Official values show **beside** what was reported and a difference is named; nothing reported is overwritten. A run again is a new version.

Link drivers, owners, lessees, passengers and occupants to people on the call; each person shows their vehicles too. A possible stolen or wanted hit shows as **POSSIBLE HIT: not confirmed** until a hit confirmation is answered (see [NCIC and state requests](#requests)).

**Tow** on a vehicle opens the tow for that vehicle: the next company on the agency's rotation at the top with its phone and last turn, then Accepted, Declined (with why) or No answer, each step's time, the driver, truck, storage lot, impound number, hold and release restriction. A declined company loses its turn and the next is offered. Each vehicle keeps its own tows. On the command line \`SE.AD6\` then F10 opens the same tow for AD6 on the selected incident and asks which vehicle.`,
  },
  {
    id: 'requests', title: 'NCIC and state requests', route: '/requests', keys: 'ncic vcic dmv nlets request queue hit confirmation ori extradite handoff sign off shift change overdue',
    body: `One working queue for every DMV, VCIC, NCIC, Nlets and other-state transaction about people, vehicles, plates, VINs, articles, guns, boats and securities.

- The queue shows **metadata only**: the request type, incident, record, who asked (unit or officer), when, the responsible dispatcher, who sent it and where (ORI), who received the answer and when, the status and the disposition. The return itself is opened from the person or vehicle record by those allowed.
- Statuses: New, Assigned, Sent, Awaiting return, Possible hit, Hit confirmation sent, Confirmed valid, Not valid, Will not extradite, Extradition limited, Pending further information, No record, Error, Cancelled, Reassigned, Complete (the agency may rename them).
- **Nearly due** and **OVERDUE** show in words. An overdue request tells its owner and the on-duty supervisor once.

## Hit confirmation
From the exact person or vehicle record (or the queue): the record and return being confirmed, the requester, the entering agency's ORI, the method, **urgent (10 minutes)** or **routine (one hour)**, as NCIC policy sets, and notes. Record the answer with who answered and when: Confirmed valid, Not valid, Will not extradite, Extradition limited, Cancelled or Pending further information, with notes for extradition limits, pickup radius, caution and reference numbers. Sending a confirmation never makes a hit confirmed.

## At shift change
Every open request has one responsible dispatcher. **Signing off is refused** while you own open requests: finish each one, or hand it to another dispatcher (who accepts it) or to a queue, with a note of what is outstanding. The whole chain of owners stays on the request. [Open the requests](#/requests)

> In the demonstration the switch is the training simulator: a plate or name starting with TEST answers as a possible hit, and every answer is marked simulated.`,
  },
  {
    id: 'streetview', title: 'Street View beside the call', keys: 'street view google imagery views north east south west blank page',
    body: `The Hazards tab docks Street View for the incident's current location, with the address, coordinates and the imagery date. It never leaves you on a blank page and never touches the call.

- With a Google Maps key (Admin › Integrations › Views, restricted to this site) bitdispatcher first asks Google whether there is imagery within 50 m, then shows it docked.
- When there is none, it times out after 10 seconds, or no key is set, it says so and offers **Retry**, **Aerial photos on the map**, **Open the map** in another window and **Return to call**.
- When the location changes (corrected, forced, rebid, moved on the map) it shows the new place and says the location changed.
- The **N, E, S, W** views of the place show the agency's or provider's pictures for each side, and say which sides have none.`,
  },
  {
    id: 'keytest', title: 'Testing the keys on a workstation', keys: 'key test function keys fn lock laptop volume microphone f2 f4 shortcut workstation',
    body: `Before a position goes into service, test that each function key in the duty's key profile reaches bitdispatcher. Open the key map (**Ctrl+/**) and press **Test the keys here**, then press each key it asks for.

- A key that arrives is marked as working.
- A key the laptop keeps for itself (F2 as volume, F4 as the microphone) is recorded with the fix: hold **Fn**, or turn on Fn Lock (often Fn+Esc), or use the agency's alternate key.
- After the test, buttons and the key map show the combination that works on this workstation (for example **Fn+F4**).
- The result is kept on the workstation and on the server; supervisors see every workstation's latest test in Supervisor review.`,
  },
  {
    id: 'supervisor', title: 'Supervisor review', route: '/supervisor', keys: 'supervisor location exception critical incident review qa phrase alert shift intelligence report handoff individual workload follow-up',
    body: `One screen for the supervisor's review work, in tabs. Each item stays until it is dealt with.

## Location exceptions
Every forced, estimated or otherwise unverified incident location alerts the on-duty supervisor at once (an urgent or high message) and waits here with what was typed, the accepted location and map point, the source and confidence, the nearest verified alternatives, the caller's coordinates, who entered it at which position, the reason, the units and the incident status. Opening the incident does not clear it. **Acknowledge** it, then answer two separate questions:

1. **Was the best available location information used?** Yes, or no with what was better.
2. **Does the agency's address source or workflow need follow-up?** No further action, Submit address or GIS update, Correct or merge master record, Refer to another jurisdiction, Training or workflow review, System defect or Pending follow-up, each with an owner and a due date, then **done**.

The address queue moves it through New, Under review, Verified, Rejected or Merged; verifying or merging links the incident to the master address (an active incident's dispatchers and units see the change) and keeps what was entered.

## Critical incidents
The agency says which call types, modifiers, priorities, events (a unit emergency) and phrases make an incident critical (Admin › Critical incidents). A trigger alerts the supervisor and opens **one** review per incident; more triggers add to it. A supervisor can also mark an incident critical, or remove it, with a reason; the triggers stay on record.

**Open full review** assembles it when asked: the source inventory, the people and their duties, call-handling milestones with the event each comes from, every unit's times, tagged radio traffic (machine tags, an aid only), requests (metadata), the QA matrix and the evidence limitations. Each calculated time shows the events it was measured between. Answer each QA measure Met, Not met, Not applicable, Unable to determine or Needs follow-up with a comment and the evidence it rests on. Choose an evidence cutoff and **Issue** a version (its hash is kept); later evidence makes a new version. **Records-request copy** and **Accreditation copy** apply the agency's disclosure rules to a separate copy with a redaction log; the review itself is never changed. **Offer support** and **Open after-action** record operational actions only; no clinical or wellness detail goes in CAD.

## Phrase alerts
Configured phrases heard in call or radio transcripts or written in the narrative alert the supervisor with the context, source, channel, speaker and time. A negated phrase ("no shots fired") is labelled **NEGATED** for review, and a low-confidence one **UNCERTAIN**. Dispose of each: Reviewed no action, Critical incident opened, Linked to existing review, False positive or Follow-up required.

## Shift Intelligence
A report for each operational period (Admin › Call entry rules sets the periods), assembled from the records: major and open incidents, location exceptions and their review, critical incidents, new and critical flags, BOLOs and urgent messages, units short of staff, overdue status checks, open requests and the follow-up owed, each with its source, time, status, owner, due date and next action. Run it for the current period, an earlier one or a custom range. Add the supervisor's summary and handoff notes (kept apart from the facts) and **Finalize**; later changes are addenda with a reason. The incoming supervisor **acknowledges** it and accepts or reassigns each open follow-up, which carries forward, one item per source, until done. Restricted detail shows only to those allowed.

## People
Each person's period: duties and positions, sign-ons, calls answered and their times, incidents made, dispatches, status changes, requests, critical incidents and their part in them, **formal breaks** and, apart from them, **console locks** (a lock is not a break). A possible workload-sharing gap shows as a review prompt with the context that explains it, never as a score or a ranking. People add context or corrections to their own record.

## Follow-up and key tests
Every tracked follow-up, with its history, owner and due date; and every workstation's latest [key test](#keytest).`,
  },
  {
    id: 'admin-callentry', title: 'Admin: Call entry rules', route: '/admin/callentry', keys: 'force location reasons duties dispositions identifiers vehicle codes plate state requests hit confirmation minutes operational periods shift report',
    body: `The rules behind call entry and the supervisor's review, each section saved on its own:

- **Unverified locations**: which duties may force a location (and whether trainees may), the reasons offered, whether a note is always needed, whether the supervisor is alerted as urgent, the follow-up dispositions, the default follow-up days and the sublocation kinds.
- **Protected identifiers**: the duties that may open full Social Security and licence numbers.
- **Vehicles**: the plate state offered first and the agency's additions to the code lists (states, plate types, colours, makes, bodies, hair and eye colours).
- **NCIC requests**: the names of the statuses, the handoff queues, the hit-confirmation minutes (NCIC policy: 10 minutes urgent, 1 hour routine), how long an ordinary request may stay open, when it shows as nearly due, and the sign-off check.
- **Operational periods**: the periods and their start times, the time zone, the report's sections, what counts as a major incident, whether a summary is required, who may read the report, and the workload prompt's ratio. [Open Call entry rules](#/admin/callentry)`,
  },
  {
    id: 'admin-critical', title: 'Admin: Critical incidents', route: '/admin/critical', keys: 'critical rules phrases negation radio tags qa template measures standards',
    body: `- **Rules**: a call type, a modifier, a priority (this one or more urgent), an event (a unit emergency) or an alert phrase makes an incident critical.
- **Alert phrases**: the words, their variants (a trailing * matches the start of a word), the negation words looked for before them in the same sentence, the sources (call, radio, text, narrative), the call types, a confidence floor and the severity. **Testing** keeps a phrase from alerting while it is tried. **Try it** checks any text without alerting anyone. The starter phrases arrive in testing.
- **Radio tags** mark descriptions, weapons, direction of travel, threats, use of force, CPR, hazards, requests, transports and location changes in radio traffic for the review.
- **QA templates**: per call type, the measures (an interval between two moments with its standard, or a check), whether each is scored, the not-applicable reasons allowed and which need a supervisor or QA specialist.

Saving makes a new policy version; a review opened earlier keeps the version in force when it opened. [Open Critical incidents](#/admin/critical)`,
  },
  {
    id: 'admin-pip', title: 'Admin: PSC PIP connection', route: '/admin/pip', keys: 'psc pip cede arc roc reset wellness connection metadata pseudonym',
    body: `An optional connection that sends PSC PIP operational metadata and source references only: the period's counts for CEDE, location exceptions as context, critical incidents (and, when the agency marks one as qualifying, to RESET), after-action workflows a supervisor opened (ARC), documented missed breaks (ROC) and support offers (RESET), with versioned corrections.

- The agency chooses which event classes cross, whether people are named or sent as a stable pseudonym, and whether only period totals are sent. **Preview** shows exactly what would be sent.
- No caller, patient or victim details, narratives, transcripts, exact addresses, wellness or clinical content, and no scores. PSC PIP's own logic stays in PSC PIP. Each transfer is recorded in the ledger without the people in plain text.
- With the connection off, every supervisor alert, review and report works the same.

The contract is in docs/PSC-PIP.md. It has been tested against a local test receiver only, not a real PSC PIP system. [Open PSC PIP](#/admin/pip)`,
  },
  {
    id: 'start', title: 'Getting started', keys: 'overview first steps install begin introduction',
    body: `bitdispatcher is a computer-aided dispatch (CAD) system for 911 centres: one program with its own web interface. It creates and tracks incidents, recommends units by drive time over the road network, shows units, calls and incidents on a dispatch map with aerial photos, gives field crews a mobile terminal, and records every action in a tamper-evident ledger that reports are built from.

::figure lifecycle

## The first hour
1. **Sign in** as the administrator with the password printed on first start, and choose **Administrator**. See [Signing in](#signin).
2. **Record the map**: Admin › Map, choose the centre and radius, press Record. About two minutes for 100 miles. [Open Map](#/admin/map)
3. **Stations and units**: where units are based and what they are. [Open Stations](#/admin/stations) [Open Units](#/admin/units)
4. **Call types and run cards**: what each kind of incident needs. The defaults are a sensible start. [Open Call types](#/admin/calltypes)
5. **Users**: dispatchers, supervisors and field crews. [Open Users](#/admin/users)
6. **Call handling**: connect BitVoice, or point any call-handling system's ALI spill and EIDO at bitdispatcher. [Open Integrations](#/admin/integrations)

Admin › Setup checklist follows these steps and the rest of what comes before the first live call, and Admin › Go-live check tests the result. See [Self-service setup](#self-service). [Open the checklist](#/admin/setup)

> bitdispatcher advises; the dispatcher decides. No feature dispatches a unit, closes an incident or changes a priority on its own.

## Demo mode
Started with \`-demo\`, bitdispatcher seeds a fictional agency on the Roanoke, Virginia map (stations at real station positions, made-up units). With \`bvsim\`, a simulated BitVoice, calls ring and radios report positions; everything simulated is marked SIMULATED.`,
  },
  {
    id: 'install', title: 'Installing it', keys: 'install installer setup download windows linux macos mac deb rpm pkg service upgrade uninstall remove port firewall reset password',
    body: `bitdispatcher runs as a service on one computer, and every console opens it in a browser. Each installer sets it up to start with the computer, before anyone signs in, and shows the first administrator's password once, at the end. Upgrading means running a newer installer: the records, the settings and the users are kept. Uninstalling keeps the records unless you confirm twice that they should go.

Download them from the latest release at github.com/BioDream-LLC/bitdispatcher/releases, where SHA256SUMS lists each file's checksum. Or build them from the source on any computer with Go (see the README): \`make windows\` (needs NSIS), \`make linux\` and \`make macos\` (on a Mac) write them to the bin folder.

| | Windows | Linux | macOS |
|---|---|---|---|
| File | bitdispatcher-setup.exe | .deb, .rpm, or a .tar.gz with install.sh | bitdispatcher-VERSION-macos.pkg |
| Systems | 64-bit Windows, desktop or Server | systemd: Debian, Ubuntu, Red Hat and alike, Intel or Arm | macOS 12 or later, Apple silicon or Intel |
| Service | Windows service, restarted if it stops | systemd service, hardened, its own user | launchd service, its own hidden account |
| Records | C:\\ProgramData\\bitdispatcher | /var/lib/bitdispatcher | /Library/Application Support/bitdispatcher |
| Settings | the installer's Settings page | /etc/default/bitdispatcher | /usr/local/etc/bitdispatcher.conf |
| Log | the records folder, logs | journalctl -u bitdispatcher | /Library/Logs/bitdispatcher |

In every case the records folder is closed to everyone but the service and administrators: it holds criminal justice and personal information. The service listens on port 8443 (HTTPS) unless you choose another.

## Windows
Run **bitdispatcher-setup.exe** and approve the administrator prompt. The pages: licence, what to install, the folder, **Settings** (the port and the records folder; a port another program holds is refused), and the **First sign-in** password. It adds a firewall rule for domain and private networks, and Start menu entries: bitdispatcher, its manual, and **Reset admin password**. It is removed from Settings › Apps.

For IT, a silent install: \`bitdispatcher-setup.exe /S /PORT=8443 /DATA=D:\\bitdispatcher /NOFIREWALL\` (each option may be left out). A silent install shows no password: run Reset admin password afterwards. A silent uninstall keeps the records.

## Linux
With the package manager: \`sudo apt install ./bitdispatcher_VERSION_amd64.deb\` or \`sudo dnf install ./bitdispatcher-VERSION-1.x86_64.rpm\`. The password is printed at the end. Without one: unpack the .tar.gz and run \`sudo ./install.sh\`, with \`--port 9443\` for another port and \`--open-firewall\` to open it in ufw or firewalld; \`sudo ./uninstall.sh\` removes it, \`--purge\` deletes the records after you type DELETE twice.

Change the port or the records folder in /etc/default/bitdispatcher, then \`sudo systemctl restart bitdispatcher\`. A lost password: \`sudo systemctl stop bitdispatcher\`, \`sudo -u bitdispatcher bitdispatcher reset-password -data /var/lib/bitdispatcher admin\`, then start it again.

## macOS
Open the .pkg and follow it. At the end a dialog shows the password, with **Copy password** and **Open bitdispatcher**. If port 8443 is taken it uses the first free port from 9443 and says so. It keeps the Mac awake while the service runs (a dispatch server must answer at any time; the display may still turn off): set BITDISPATCHER_KEEP_AWAKE=no in the settings file to let it sleep. If macOS's firewall is on, bitdispatcher is allowed through it.

**Applications › bitdispatcher** holds four things: **bitdispatcher** opens the console, **Reset admin password**, **Uninstall bitdispatcher** (it asks twice before deleting the records) and the manual. From Terminal: \`sudo installer -pkg FILE -target /\` installs without dialogs and leaves the password in /usr/local/etc/bitdispatcher.first-sign-in for root to read once; delete that file afterwards. After changing the settings file: \`sudo launchctl kickstart -k system/com.biodream.bitdispatcher\`.

## The certificate
On first start bitdispatcher makes its own certificate, so each browser warns once until it trusts it. Give it one your network trusts with the \`-tls-cert\` and \`-tls-key\` options (on Linux and macOS in BITDISPATCHER_FLAGS in the settings file).

## What has been tested
- **Windows:** installing, three upgrades, the password reset and uninstalling, on one Windows desktop computer; and on every change, a silent install, upgrade and uninstall on Windows Server 2022 in CI.
- **Linux:** the Intel .deb, .rpm and .tar.gz in Debian 12 and Red Hat UBI 9 containers running systemd (install, upgrade, reset, remove); the Arm ones in the same containers on an emulated Arm processor, not yet on Arm hardware; the .deb on Ubuntu 24.04 in CI.
- **macOS:** installing, two upgrades, opening the console, the password reset and uninstalling with the records kept, on one Apple silicon Mac with macOS 26; in CI also a reinstall over the kept records and a full removal. Not yet on an Intel Mac, and not with macOS's firewall on. On macOS 26, deleting the records also tries to remove the hidden _bitdispatcher account, and macOS can refuse that even for an administrator; the uninstaller then says so. The account has no password and owns nothing, and a later install reuses it.
- **Not signed.** Windows SmartScreen warns about the installer, and macOS asks before opening a downloaded .pkg (System Settings › Privacy & Security › Open Anyway). Signed installers need code-signing certificates, which BioDream has not bought yet.`,
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
    id: 'layout', title: 'Your own layout', keys: 'layout arrange drag drop snap sections palette lock unlock stock ready-made simple overwhelming new dispatcher getting started workspace',
    body: `The dispatcher screen shows only the sections you want, where you want them. Someone new can start with two and add more when ready.

## Your saved preset for each duty
Your layout is a **preset** kept with your account, one for each duty you work (call-taker, police, fire and EMS dispatcher, supervisor…). Sign in for a duty at any console and your preset for it opens by itself: nothing to rebuild at the start of a shift.

- It is **locked** in normal work: nothing moves, resizes or disappears by accident.
- To change it: the **lock** button (*Layout locked: edit it*), arrange, then **Save & lock**. Choosing a ready-made layout also starts editing. **Cancel** puts the saved one back. Each save is in the ledger with who saved it and when.
- A **trainee** works the agency's assigned training preset; the layout controls are not shown. Someone with no preset of their own starts from the agency's preset for the duty.
- An administrator can **save the layout as the agency's preset** for a duty (the building button while editing) and **protect** a person's preset so only administrators change it (Admin › Operations rules).
- The screens spread over several monitors are the workstation's (see [Several screens](#screens)): each console remembers its own monitors, so a preset placed on a console with fewer or other monitors falls back to what fits there without changing what you saved.

Open **Layout** at the top left of the status bar.

- **Ready-made layouts**: three for the duty you signed in as, drawn as small pictures. One click shows one. Every duty has **Getting started**: just the calls and the incident being worked.
- **Sections**: calls and incidents, map, incident, unit board, fire houses, police board and the command line. Click one to add it or take it away, or drag it onto the screen.

While the layout is **unlocked** (the lock button beside Layout, amber when unlocked), each section has a title bar:

| To | Do |
|---|---|
| Put a section beside, above or below another | Drag it (from Layout, or by its title bar) to that edge of the other; the highlight shows where it will go |
| Make two trade places | Drop one on the middle of the other |
| Resize | Drag the line between two sections |
| Take one away | Its **×** |
| Start again from nothing | **Clear the screen** in Layout |

Sections snap into place: nothing overlaps and nothing is left empty. **Lock** the layout when it is right: nothing can be moved, resized or taken away until it is unlocked again. Everyone starts locked on their duty's first ready-made layout.

Each person's layout is kept per duty, in this browser. The status bar and the emergency banners are always shown. The layout lock is not the screen lock (Ctrl Shift L), which hides the screen until you sign in again.`,
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

Mark-outs are also on every unit's right-click menu and in the unit details, and a crew marks itself out from the mobile terminal (**F7**). A mark-out that can be pulled (a meal, a report) is offered by the recommendation when no available unit can fill a line: it is never sent without a dispatcher choosing it. The kinds, their times and which can be pulled are set in Admin › Settings, with the **longest** each may last: a unit out past it shows **OVER** on the unit board and the supervisor board, and the supervisors get a high-priority message once.`,
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
- **People**: each signed-in position's workload and breaks, and the care list after critical calls. See [Breaks and workload](#breaks) and [Critical calls and care](#care). A possible line attack shows across the top ([TDoS](#tdos)).

Choose the disciplines across the top, or open it from Screens with them. **Full screen** is beside the clock.`,
  },
  {
    id: 'calls', title: '911 calls', keys: 'call ali eido pidf location callback ringing rebid queue',
    body: `Calls arrive from BitVoice's event stream, from EIDO pushed by any call-handling system, and from the ALI spill. The same call from several sources is merged into one line.

- **Ringing** calls show before they are answered, so the dispatcher sees what is coming.
- **Location**: the caller's position and uncertainty circle, the address when there is one, and **STALE** when the location is older than the setting in Admin › Settings. Rebid asks for a new one (BitVoice).
- **+ Incident** on a call creates an incident from it, with the location and callback filled in. Calls already attached show their incident.
- Text to the caller, a video link and live video are on the call when BitVoice offers them.
- Its [transcript](#transcript) and what was heard, [translation](#translation), the [device location and floor](#caller-location), [crash data](#crash), callers [grouped](#grouping) and [non-emergency](#nonemergency) calls are described in their own chapters.`,
  },
  {
    id: 'incidents', title: 'Incidents', keys: 'incident create new close reopen replay playback correct correction amend edit closed priority note narrative timeline disposition',
    body: `**New incident**: \`F2\`, the New button, + Incident on a call, right-click on the map (the address is filled in), a search result's New incident, or \`NEW SF 2001 Crystal Spring Ave\` on the command line. An address the recorded map knows exactly is placed on the map at once, so units are ranked by road straight away; anything else stays text to be placed.

The incident panel shows the recommendation, the units on it with their times, the caller and the timeline. Notes (\`N text\`) and priority notes (\`PN text\`) go on the timeline; priority notes are shown to every dispatcher and unit.

**Close** with a disposition (\`CL Handled\`); units still on it become available. Supervisors can reopen (\`RO #26-000123 reason\`).

**Correct the record** (supervisors, on a closed incident) changes the call type, priority, disposition, address or a unit's times without reopening it: say why, and the timeline keeps what each was. Reports already sent (NERIS, NIBRS) are sent again by hand.

**Replay** (supervisors, on a closed incident) plays it back from the ledger: the units move on the map along their recorded positions (about every 30 seconds) while the timeline fills in, at 10× to 120×, with a slider to any moment.`,
  },
  {
    id: 'recommend', title: 'Unit recommendations', keys: 'recommend run card closest unit drive time routing eta',
    body: `For the selected incident bitdispatcher builds a recommendation from its call type's run card: for each line (\"3 × engine\", \"1 × als\"), the available units with that capability, closest first. Units already on the incident fill lines first, and scarce capabilities are filled before common ones so a multi-role unit is not spent where another would do.

- **Drive times** come from the recorded road network: posted or typical speeds, one-way streets obeyed, from the unit's fresh AVL position or its station. The reason says so: \"1.7 mi by road, 4 min via Franklin Road\".
- **Closed roads and traffic** count at once: a drive goes around a [road closure](#road-closures), and a road live traffic reports slow takes longer. The source line under the recommendation says when they changed it (\"around 1 road closure, with live traffic\").
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
- It advises only. It never merges, retypes, reprioritises or dispatches by itself, and core dispatch works the same when it is off or down.

When an upgrade supplies a new rule (such as *Silent or abandoned 911 call*), Admin › Advisory panel offers it above the rules: **Add it**, then **Save version**. A saved codebook never changes by itself.`,
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
- **Right-click** anywhere: what is here (the nearest address or road), New incident here, **Close a road from here** and **Close the roads around here** ([Road closures](#road-closures)), and [Views of this place](#views) when a provider is on.
- **Closed roads** are drawn in red dashes, and **traffic** slowdowns in yellow, orange and red under them; click a closed road for why, and Lift.
- **Routes**: for the selected incident, the road route of each recommended, chosen and assigned unit, with its arrival time on the unit.
- **Day / Night** switches the palette; **Photos** (\`F7\`) puts aerial photos under the streets; **3D** stands the buildings up (OpenStreetMap's storeys where mapped) and tilts the map, and right-drag turns it; **Fit all** shows every unit and incident; OOS and Stations show or hide those.
- The dashed cyan line is the edge of the recorded map: beyond it the online map is shown, muted, and drive times are estimates.`,
  },
  {
    id: 'road-closures', title: 'Road closures and traffic', keys: 'road closure closed road detour blocked flood crash traffic jam waze live traffic drive time routing lift',
    body: `A closed road changes drive times and unit recommendations at once: the router goes around it, with no rebuild of the map.

- **Close a stretch**: right-click the map on the road, **Close a road from here**, then click along the closed stretch and **Done**. Say why, until when (empty: until someone lifts it), the detour, and whether to note it on the selected incident.
- **Close an area**: right-click, **Close the roads around here**, and give the radius: every road with any part in the circle closes (a flood, a fire, a scene).
- The closed roads are drawn in red dashes. Click one for why and since when, and **Lift** to open it again. A closure with an end time lifts itself.
- A unit can still be sent to a place on a closed road: where a drive starts and ends is never refused, only the roads in between.
- Crews see the roads closed now, with the detours, on the [mobile terminal](#mdt).
- **Live traffic**, when the agency has a feed ([Road closures & traffic](#admin-roads)), slows the roads it reports: a road's drive time is its recorded time times how much slower it is, up to ten times. The source line of a recommendation says when traffic and closures changed it.
- Every closure, change and lift is an attributed event in the ledger.`,
  },
  {
    id: 'cameras', title: 'Cameras near an incident', keys: 'camera cameras cctv traffic camera live view still picture near incident purpose audit',
    body: `When the agency has switched cameras on ([Cameras](#admin-cameras)), an incident with a map position lists **Cameras near**: the agency's cameras within its radius, those looking toward the place first, with how far and whether they look this way.

- Press one: it asks **why** (the incident, or a reason), then opens it. A camera with a still picture shows what it sees now, refreshed every 5 seconds; one with a live view opens it in the camera system, which signs you in itself.
- Every opening is recorded with who, when, why and the incident, and goes on the incident's timeline. Supervisors see the record in [Admin › Cameras](#admin-cameras).
- bitdispatcher keeps no video and records nothing from the camera; the still picture is fetched for the person who opened the camera, and only for half an hour.`,
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
    id: 'views', title: 'Views: the place from every side', keys: 'views oblique pictometry eagleview street view panoramax mapillary side back of house pre-plan preplan photo 3d buildings',
    body: `Aerial photos show the roof. **Views** show the front, the back and the sides: where the back door is, the fence, the stairs, the side the hydrant is on.

- **Views** on the incident panel (beside the location), in the call-taker's **Hazards** tab, on the crews' [mobile terminal](#mdt), and from a right-click on the map.
- **N E S W**: the four oblique aerial photos, looking north, east, south and west, from the agency's own EagleView licence. Looking north shows the south side of a building. The arrow keys turn the compass.
- **Street** photos that look at the place, nearest first and at most two per direction, from Panoramax or Mapillary (open photos, CC-BY-SA, coverage depends on who has photographed the street).
- **Pre-plan** photos: the agency's own pictures of a premise (the Knox box, the gas shut-off, the rear entrance), offline and on this server. Crews add them from the terminal with the device's camera, dispatchers and administrators from here or [Premises](#admin-premises); supervisors remove them.
- Every picture says where it came from and when it was taken, so an old picture is never taken for today. The expand button shows it full size.
- **Google Street View**, when offered, opens Google in a new tab; nothing from Google is shown inside bitdispatcher.

> Looking a place up tells the provider where the incident is. Each provider is off until an administrator turns it on in [Integrations](#admin-integrations), and the pictures pass through this server, so keys never reach a browser.`,
  },
  {
    id: 'working-agency', title: 'The agency you are working for', keys: 'agency agencies working covered monitored watched coverage mutual aid primary assisting home agency rpd rcpd',
    body: `Where the centre dispatches for several agencies (Admin › Agencies), the status bar shows the agency you are **working** for: its rules apply to what you do (its incident numbers, call types, timers and response plans). Click it to see and change:

- **Working**: one agency, always shown.
- **Covered**: the agencies you dispatch for at this position.
- **Watched**: agencies shown for awareness only.

Your usual coverage loads by itself at sign-in, from your profile (Admin › Agencies › People) and the workstation's (Admin › Workstations). A change is recorded. You may only cover agencies you are authorised to work, and watch those, or those an administrator gave you or the workstation to watch.

## What the board shows
Someone limited to some agencies sees only their part of the shared board: those agencies' incidents and units, incidents with no agency yet, units of other agencies sent to their incidents, and 911 calls until they become another agency's incident. The server enforces it: another agency's incident or unit is refused by a direct link or on the command line, and the attempt recorded. Someone who works every agency sees everything, and can choose **The board shows › Only the agencies covered and watched here** to narrow their own board without losing access.

An incident has a **primary** agency (from its call type, or your working agency) and may have **assisting** agencies. Every unit keeps its home agency: a unit of another agency sent to the incident shows it as assisting by itself. **Agencies…** under the units changes them (a new primary agency needs a reason).`,
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
| \`TG <talkgroup>\` | Set the selected incident's talkgroup |
| \`BOLO <text>\` | Issue a BOLO for the selected incident, or none |
| \`ATTACH <call> #26-000123\` | Attach a 911 call to an incident |
| \`ACK\` | Acknowledge the emergency in the red bar (the newest) |
| \`ALARM\` · \`ALARM 3\` · \`2ND\` | Upgrade the selected incident to its next alarm (or a level); \`REC\` then sends the extra units. See [Alarm levels](#alarms) |
| \`PAR\` · \`PAR MISSING E3\` · \`PAR FOUND E3\` | Record a roll call of the crews on scene. See [Accountability](#accountability) |
| \`BM UC\` | Record a fire-ground benchmark (UC fire under control, PS primary search…) |
| \`TOW\` · \`TOW heavy North / VA ABC-1234\` | Call the next tow company on the rotation. See [Tow rotation](#admin-tows) |
| \`PAGE <group> / text\` | Page a responder group about the selected incident. See [Paging](#admin-paging) |
| \`DIR water\` | Look up the contacts directory. See [Directory](#admin-directory) |
| \`FI UNIT1.8TH/STEWART..ABC1234\` | Field initiate: a traffic stop (or other officer-initiated incident) in one line. With the classic keys, type the line and press \`F7\` |
| \`US UNIT1.AS\` | Unit status in one line, by the agency's status codes. With the classic keys, the line then \`F11\` |

## One-line field commands
The identifiers are separated by the agency's delimiter (a full stop unless it chose another), in the agency's order; two delimiters together skip an optional field (Admin › Operations rules › Command line):

- **Field initiate**: unit, location, type, plate, state, comment, plate type, status. \`UNIT1.8TH/STEWART..ABC1234\` is UNIT1 at 8th and Stewart, the type left to the agency's (a traffic stop), plate ABC1234. It creates the incident with the unit on scene, the plate on the incident, and the stop's own status-check timer, without another screen; it becomes a full incident by changing its type (\`T\`), keeping everything. In **two-pass** agencies it only marks the unit out on the stop. Where the agency asks, the plate is queried in NCIC at once.
- **Unit status**: unit, status, disposition, location, type, comment. \`UNIT1.AS\` is UNIT1 arrived on scene of its incident. A unit alone (\`UNIT1\` then F11) opens the unit status form instead of guessing. A mark-out code works too: \`UNIT1.ADMIN..HQ ANNEX...paperwork\`.

A retired or unknown code, or a command your duty may not give, is refused by the server and the attempt recorded; nothing changes.`,
  },
  {
    id: 'keys', title: 'Keyboard', keys: 'keyboard shortcuts keys hotkeys f2 ctrl k f7',
    body: `The keys follow the agency's **key profile** for your duty (Admin › Operations rules). **Ctrl /** shows the map for the screen you are on, with a search box. A key does exactly what the same command or button does; the server checks your right either way, and a trainee works the profile as set.

The standard keys:

| Key | Does |
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
| \`F1\`–\`F6\` (mobile terminal) | En route, on scene, transporting, at hospital, available, out of service |

## The classic CAD profile (PremierOne-style)
An agency moving from PremierOne can choose this profile as a starting point and change it:

| Key | Does | Shift + key |
|---|---|---|
| F1 | Home | Help |
| F2 | Queries (NCIC) | Unit-to-unit message |
| F3 | Update | Recall (search) |
| F4 | Audit trail | Comments |
| F5 | Clear | Premise information |
| F6 | Messages and BOLOs | Refresh pending |
| F7 | **Field initiate**: processes the command line | Previous incidents |
| F8 | New incident | Next incident |
| F9 | Dispatch the recommendation | Previous incident |
| F10 | Process the command line | Premise and location |
| F11 | **Unit status**: processes the command line | The 911 call |
| F12 | Submit | Previous plates |`,
  },
  {
    id: 'files', title: 'Files and photos on an incident', keys: 'file attachment photo picture upload document pdf video audio evidence camera scene',
    body: `An incident keeps files: a crew's photos of the scene, a PDF, an audio or video clip.

- **Add**: on the incident panel, **Add** (with a caption if you like); on the mobile terminal, **Photo or file** opens the camera. Crews add to incidents they are on. Up to 25 MB each: pictures, PDF, plain text, MP3, WAV, MP4, WebM. The server goes by what the file is, not its name.
- Pictures show as thumbnails (click for full size); others open or download. Each file is on the timeline with who added it.
- Each file's SHA-256 is in the ledger: a file changed on disk is refused, not shown.
- A supervisor removes a file with why. It stays in the record, hidden from everyone below supervisor.`,
  },
  {
    id: 'cad-to-cad', title: 'Incidents with neighbouring agencies', keys: 'cad to cad cad-to-cad neighbouring agency mutual aid eido interoperability county line send share',
    body: `Incidents go to and come from neighbouring agencies' CADs as EIDOs, the NENA standard (Admin › Integrations › Neighbouring agencies; docs/CAD-TO-CAD.md).

- **Send**: on an open incident, **Send to a neighbouring agency…**, choose the agency and say why (*mutual aid, need a ladder*; *on your side of the line*). It goes with the location, the callback, our units on it and the notes; the timeline says it was sent.
- **Receive**: what a neighbour sends arrives in the 911 calls list as a call from that agency, data only, with their incident number, type and notes. Make the incident from it or attach it as from any call.
- Each send is a press: closing and unit status do not flow back on their own here. For a neighbour that also runs bitdispatcher, [overflow](#overflow) goes further: they take calls for us, and what becomes of each comes back to them.`,
  },
  {
    id: 'station', title: 'The station screen and toning out', route: '/station', keys: 'station alerting tone out tones firehouse quarters voice alert speaker phoenix locution acknowledge',
    body: `When units are dispatched, their stations are toned out (where an administrator turned it on in Integrations). In the firehouse, a screen on a read-only account at **#/station** (choose the station once; it is remembered) shows the station's units and their statuses, and on an alert:

- rising **tones**, twice as many for a priority 1, then the call **read aloud** twice: the units, the call type and the street;
- the call in large type: priority, type, address, the units, the latest priority notes;
- **Acknowledge** stops it and puts *ST1 acknowledged the alert* on the incident; **Read it again** repeats it.

Press **Press to turn the sound on** once after the screen opens (browsers play sound only after a press). The inactivity lock leaves a station screen on a read-only account alone.

- \`TONE\` on the command line tones out the stations of the units on the selected incident again (\`TONE E1\`, \`TONE #26-000123\`); each tone-out is on the incident's timeline.
- An alerting controller from a vendor gets the same alert at its address (docs/STATION-ALERTING.md).`,
  },
  {
    id: 'alerts', title: 'Alarms, plate readers and gunshot detection', keys: 'alarm alarms asap alarm company central station plate reader lpr flock hotlist gunshot shotspotter soundthinking camera alert',
    body: `Alarm companies, plate readers and gunshot detection send their alerts to bitdispatcher (Admin › Integrations; the contract is docs/ALERTS.md). Each arrives in the 911 calls list as a call, **data only**, with what it is at a glance: the alarm type and zone, VERIFIED when the alarm company verified it, the plate and its hotlist, the number of rounds.

- Select it: the incident panel shows everything the sender gave (premise, account, permit, keyholder, the operator's call-back line, the camera and heading, a link to the reader's picture).
- Make the incident from it, or attach it to one, as from any call; the agency's own alarm and shots-fired types are suggested when they exist.
- An update from the sender changes the same call; a **cancel** from an alarm company goes on the incident as a priority note.
- Alarm companies are told what became of their alarm: accepted, dispatched, on scene, closed.
- Each alarm is checked against the agency's **permit register**: *Alarm permit P-1: valid*, *EXPIRED*, *SUSPENDED*, or *none on the register*. Its address's false alarms in the last year are counted; at the agency's limit the note says **REPEAT FALSE ALARMS** ([Alarm companies & permits](#admin-alarms)).
- **Phoned alarm** (above the 911 calls): when an alarm company's operator calls on the phone, type its alarm on the alarm card (company, operator, event number, alarm, zone, premise, address, permit, keyholder). It goes in the queue like a sent one, with the same checks, and says who typed it.
- An alert nobody acts on leaves the queue after 30 minutes; it stays in the ledger.

> ASAP over Nlets, the national standard for alarms, is not connected yet: it needs the standard's schema and the state's message switch. Until then each company can have its own address and token over HTTPS ([Alarm companies & permits](#admin-alarms)).`,
  },
  {
    id: 'messages', title: 'Messages to and from the crews', keys: 'message messages msg text chat silent dispatch terminal mdt unit to unit urgent read receipt self attach take it',
    body: `Messages go from the floor to the crews' terminals, and from crews to dispatch or to another unit, without the radio.

- **Write** in Messages (Situation panel): to one unit or several (\`E1 M2\`), a discipline (\`ALL-FIRE\`, \`ALL-EMS\`, \`ALL-POLICE\`) or everyone (\`ALL\`). Leave To empty to write to the units on the selected incident. **!** makes it urgent.
- **Command line**: \`MSG E1 M2 / Stage at the north lot\`, \`MSG ALL-EMS ! Carilion on diversion\`, \`MSG #26-000123 / Caller says the back door is open\`.
- **From the crews**: messages to dispatch are bold until a dispatcher clicks them, and announced as they arrive. A message on an incident is also written on its timeline.
- **Read**: the terminal marks a message read when it is shown (an urgent one when the crew presses Got it); the floor sees *read by E1*, or *not read yet* after two minutes.
- Every message and every read is a ledger event. Crews write to dispatch or to units, never to everyone.

For messages with a subject, priority, attachments, expiry and versions, and for BOLOs and officer-safety notices, use [Messages and BOLOs](#inbox).`,
  },
  {
    id: 'inbox', title: 'Messages and BOLOs: the inbox', keys: 'inbox messages bolo bolos officer safety administrative notice priority urgent high medium low acknowledge read receipt attachment photo pdf expire cancel update version email compose cc',
    body: `An inbox like email inside CAD, for **messages**, **BOLOs**, **officer-safety** and **administrative** notices (the envelope in the status bar; the agency can add its own kinds).

## Writing one
**Compose**: the kind, the **priority**, a subject and the text (the essential warning goes in the text, never only in an attachment), **To** and **CC**, the owning agency, the incident and related incidents, and for a BOLO the plate, vehicle, person and description. Addresses can be units, \`ALL\`, \`ALL-FIRE\`, \`ALL-EMS\`, \`ALL-POLICE\`, \`DISPATCH\`, an agency (\`@agency:RPD\`: its CAD positions and its units' terminals), a role, a duty, a named person or a console. A BOLO with no one named goes to every position and terminal of the owning agency. **Attachments** (pictures, PDFs and the types the agency allows) are checked by their content, scanned where the agency runs a virus scanner, and kept with their hash. **Effective** and **expires** dates; *no expiration* where the agency allows it. **Save draft** keeps it under Drafts.

## Priorities
| Priority | Shows and sounds |
|---|---|
| **Urgent** | An interruptive banner and a repeating alarm until acknowledged; supervisors are told when no one acknowledges it in time |
| **High** | A banner and a distinct tone; acknowledgement may be required, repeated and escalated |
| **Medium** | Highlighted, one tone; unread until opened |
| **Low** | An ordinary unread mark, a soft tick or silence |

The agency sets each level's tone, repeat, acknowledgement and escalation (Admin › Operations rules, where **Play** lets you hear each one). The tones differ by rhythm and loudness, not only pitch: the alarm is five quick pulses, the alert three, the chime two soft notes, the tick one quiet pulse, each with harmonics so it carries over a room and through a headset. Several arriving together sound once, with the highest one's tone. Every priority is shown in words and with an icon, never by colour alone; only levels the agency allows can be silenced.

Browsers keep a page silent until someone has used it. After a reload, the first key or click turns sound on; until then an urgent banner says **the browser is keeping this console silent** with a button to turn the sounds on.

## Folders, receipts and versions
**Inbox**, **Drafts**, **Sent**, **Active BOLOs** and **Archived** (expired, cancelled and replaced ones, labelled as such). Delivery, opening and acknowledgement are recorded for the person and the duty they were working, not just the screen. **Update** sends a new version and **Cancel** a cancellation; earlier versions are kept, shown in the history, and stay on the incident's timeline with the attachments (not uploaded twice). A person sees a communication only when it is for them, or as a supervisor or records staff of its agency. Everything is in the ledger.`,
  },
  {
    id: 'search', title: 'Search: incidents, BOLOs, people, vehicles and units', keys: 'search recall find plate partial plate vin name alias phone address description clothing tattoo attachment unit incident number cross agency possible match',
    body: `One search (the magnifier in the status bar, or Recall in the classic keys) across incidents (number, type, address, narrative and comments), messages and BOLOs (every version), people (names in any order, aliases, birth dates, phones, addresses, identifiers, descriptions), vehicles (plate, partial plate with \`?\` for a character you do not know, VIN, make, model, colour), attachments (file names, captions, and the words inside them: plain-text files, the text of PDFs, and the words in pictures where the server reads them) and units (call sign, radio ID, who is aboard).

- Results are grouped, with filters for the kinds, agency, dates and status.
- Each says **why** it matched (same plate, partial VIN, shared phone, similar description, *in the file's text* with the words around it) and how strongly: **exact**, **partial** or **similar** (similar spellings when asked).
- **Possible matches** are marked as such: a search never links a person, vehicle or incident to anything.
- Expired, updated and cancelled BOLOs say so in capitals; an old warning never looks current.
- You search the agencies you cover and watch; **beyond my agencies** only where you may work them, and every result names its agency. Juvenile names and medical notes are withheld from people without the right, and nothing restricted is counted or shown.
- Every search, and every record opened from it, is recorded in the ledger.

**Unit recall** (from a unit in the results, or a unit's History): one unit over a period, in one timeline: its statuses and mark-outs, the incidents with their type and address, who staffed it and in what position, status checks, messages, and where it was by AVL or radio. Narrow it by responder, status or incident; download it as CSV.`,
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
- **Messages**: what dispatch (or another unit) writes to this unit, with a reply line and quick replies (Copy, Delayed, Need PD…). An urgent message fills the top of the screen and sounds until the crew presses Got it; see [Messages](#messages).
- **Messages and BOLOs**: the communications addressed to this unit or its agency's terminals (messages, BOLOs, officer-safety notices), highest priority first, with the agency's tones, repeats and acknowledgement as on the console. An urgent or high one that needs acknowledging stays at the top until the crew presses **Acknowledge**; opening one shows every version, the plate and description, and the attachments. Opening and acknowledging are recorded against the unit's sign-on; see [Messages and BOLOs](#inbox).
- **Roads closed**: every [road closure](#road-closures) in force, with why, the detour and until when, refreshed every 30 seconds; the route drawn goes around them.
- **Take it**: where the agency allows crews to attach themselves (Settings › Roles), an available unit takes a waiting incident from the list in one press; it is recorded as a dispatch by the crew, self-attached.

A crew acts only as its own unit. Dispatchers can open the terminal at #/mdt to see what a unit sees (the position names the unit).`,
  },
  {
    id: 'rms', title: 'Incident reports (RMS)', route: '/rms', keys: 'rms report records nibrs offense victim suspect property narrative approve draft ai model',
    body: `Officers and dispatchers write incident reports at #/rms, or with **Report** on an incident (the incident's location and time are filled in). A crew on a mobile terminal writes its own from the car.

- **Sections**: when and where (location type, whether it was cleared), offences, people, property, vehicles and the narrative. Codes are shown in words and picked in one click; the screen asks only for what the offence needs (a weapon for an assault, the method of entry for a burglary, the drug for a seizure, the injury and the relationship to each suspect for a crime against a person).
- **Saved as you type**: a draft saves itself a moment after typing stops.
- **Before it is submitted** lists what is still missing for incident-based reporting: the FBI's NIBRS data elements and edits, with Virginia's additions (property quantity, the licence plate reader indicator, drug types). Submit is available once the list is empty.
- **Approval**: a supervisor other than the author approves it, or returns it with what to fix. Every save and decision is a ledger event.

### The state file

Supervisors see **State IBR file** under the list. Choose the month and press **File**: bitdispatcher writes the month's approved reports in Virginia's incident-based reporting format (the FBI's position-based flat file with Virginia's additions, 2025.2 specification): Group A incidents with their offence, property, victim, offender and arrestee segments, Group B arrests, or a zero report when there is nothing. Upload it to the Virginia Crime Repository by the 15th of the next month. Reports that fail the checks are left out and listed. Making the file is a ledger event, with its SHA-256.

**Format: FBI national** writes the same reports in the FBI's own flat-file layout, without Virginia's additions, for states that take the national file. That file has been checked with SEARCH's NIBRS Pre-Certification Tool validator, which applies the FBI's edits (\`scripts/nibrs-validator.sh\`); passing it is not FBI or state certification.

> The settings need the agency's nine-character ORI (Admin › Settings). Before the first production upload, the Virginia State Police ask for a test file by email (dart@vsp.virginia.gov).

**Draft from the incident** (on a report written from an incident) fills the narrative with a first draft: the type, place, times, units, people, vehicles and notes from the record. With the language model on (Admin › Integrations) the model writes it from the same facts and is told to add nothing, writing *[to be completed]* where something is missing. Either way it is a proposal: check every line and finish it in your words.`,
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
    id: 'admin-agencies', title: 'Agencies', route: '/admin/agencies', keys: 'agencies agency ori neris incident prefix numbering dispositions timers talkgroups records multi-agency people staff profile trainee configuration versions rollback publish staging',
    body: `Every organisation the centre dispatches for is an **agency** (RPD, RCPD, SPD, RFD, RCFR, SFD, a public works department, and the centre itself): police, fire and EMS are disciplines, not agencies. A centre with no agencies works as one, with the centre-wide settings.

- **An agency**: its code and name, disciplines, active or not, ORI, NERIS ID and other identifiers, its own incident-number prefix, dispositions, status-check timers, talkgroups and records rules.
- **What belongs to it**: units, stations, call types and response plans name their agency (chosen from the list, not typed). An agency's response plan is only for its incidents; its timers only for its units.
- **People**: the agencies each person may work, the one they usually work for, those covered and watched by default, whether they are a **trainee**, and their roster entry. A person limited to some agencies changes only those agencies' configuration and sees only their supervisory views.
- **Workstations** can say which agencies they cover and watch (Admin › Workstations).

## Change control
Configuration changes can be drafted on the training board, tried there, and **published** to the live board with a reason, all at once or **for one agency** only: a publish for one agency never carries another's or the centre's changes. Each publish keeps the versions it replaced: **Roll back** puts the earlier approved version back, with a reason, and is itself recorded.`,
  },
  {
    id: 'admin-operations', title: 'Operations rules', route: '/admin/operations', keys: 'operations rules supervisory views board reports ledger quality presets protected working calls limit priorities tones escalation communication kinds attachments staffing policy short override block command line grammar delimiter status codes retired key profile classic premierone f7 f11',
    body: `The agency's rules for daily work (Admin › Operations rules; administrators).

- **Supervisory views**: the supervisor board, the operations report, quality review and care, the ledger and staffing coverage belong to the agency. Without a rule, supervisors see them; with one, only the roles, people and agencies it names. Administrators always see them. A person without the right neither sees the view nor reaches it by a direct link: the server refuses, and records the attempt.
- **Presets**: the agency's assigned preset for each duty (what trainees work and others start from), and protecting a person's preset so only administrators change it.
- **Working calls**: on or off, how many by duty and for trainees, and after how long a working call left by someone signed out is offered to supervisors.
- **Communications**: the kinds (who may send them, their default priority and expiry, whether they wait for people signed off), each priority's tone, repeat, acknowledgement and escalation (with a button to hear each tone), and the attachments allowed.
- **Staffing**: what happens to a unit left short (warn, limited, out of service), whether short units may be sent (warn, a supervisor's override, block), and the minimum qualified people the centre wants on duty.
- **Command line and keys**: the delimiter, the order of the identifiers for field initiate and unit status, the stop's call type, one or two passes, the automatic plate inquiry, several units at once, the status codes (and retired ones), and the **key profile** for each duty, standard or classic (PremierOne-style) or the agency's own copy. **Check** finds a key doing two things on one screen, a key the browser keeps for itself, or an unknown action; the settings are refused until it is fixed.`,
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
    body: `A call type is a kind of incident (\`SF\` structure fire, \`CHEST\` chest pain) with a default priority, a discipline and a run card: how many units of each capability it needs (\"3 × engine, 1 × ladder, 1 × als\"). Recommendations fill the run card. Capabilities come from unit types (Admin › Settings) and units.

**Critical call** puts the people who work one on the supervisor's [care list](#care); **Send a drone** has the drone launch rule ask for a [drone](#drones).

**Auto-dispatch** (priority 3 to 5 types only): when no dispatcher has sent units within the wait in Settings (60 seconds unless changed), the recommended units go by themselves, logged as *auto-dispatch*. Only a complete recommendation without warnings is sent (a verified location, every line filled); otherwise the timeline says why and a dispatcher sends it. It tries once per incident.`,
  },
  {
    id: 'admin-map', title: 'Recording the map', route: '/admin/map', keys: 'map record openstreetmap geofabrik radius centre photos imagery offline',
    body: `bitdispatcher records the OpenStreetMap data for a circle around your centre into its own store, then draws the dispatch map, routes units by road and finds addresses from that store with no internet.

::figure map

1. Choose the centre (a preset, the console map centre, or a position) and the radius (100 miles covers a county and its mutual-aid neighbours).
2. The downloads the area needs are listed at once (state extracts from Geofabrik). Press **Record**.
3. Recording takes a few minutes; dispatching carries on with the current map, which stays until the new one is complete. Each recording is a ledger event.

**Bring up to date** applies OpenStreetMap's daily change files (Geofabrik publishes one a day per extract) since the map was recorded: a few megabytes instead of the whole extracts, then the store is built again. Record again for a new area, or when the map is more than a year behind; downloads already made are checked against their published checksums and reused.

## Automatic updates
**Automatic updates** does the same on a schedule: off, daily or weekly, at the hour you choose, in the agency's time zone (Admin › Call entry rules › operational periods).
- The new map is built beside the one in use, which keeps working the whole time. It is switched to only when it is complete and has not lost more than the percentage you set (10 by default) of its road segments or addresses; a damaged change file shows as roads gone, and is refused.
- A run that fails, has no internet, or finds a recording already under way leaves the map in use as it was, and the next window tries again. A failed scheduled run messages the supervisors and administrators.
- Each run is in the ledger. The panel shows the next run and the last one's outcome, and so does the [Continuity](#continuity) page.

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
    id: 'admin-settings', title: 'Settings', route: '/admin/settings', keys: 'settings agency timers stale road factor speed tiles shifts unit types mark-outs markout sign-in notice banner cjis auto-dispatch',
    body: `- **Agency** name and incident number prefix.
- **Status timers**: minutes in each status before the dispatcher is prompted for a status check.
- **Mark-outs**: the kinds a unit can be marked out for (code, name, status-check minutes, which units, whether it can be pulled for a call, whether it is retired, and the longest it may last before supervisors are told). See Police status and mark-outs.
- **Stale** ages for AVL and caller locations.
- **Shift changes** (\`07:00 19:00\`) for the operations report's shift periods.
- **Travel estimates** outside the recorded map: road factor and average speed.
- **Calls**: how near and how soon callers are [grouped](#grouping); the [non-emergency](#nonemergency) lines, words and reply.
- **People**: the [critical-call](#care) words, the support line, and when to suggest a [break](#breaks).
- **Quality review** standards and the questions to listen for ([Quality review](#quality)); **crash data** thresholds ([Vehicle crash data](#crash)); the [line-attack](#tdos) warning level.
- **Map beyond the recorded area**: the online tile source (fetched and cached by the server, never by browsers), and the map's starting centre and zoom.
- **Unit types**: code, name, discipline, capabilities, and whether stations keep them covered (move-up suggestions).
- **Dispositions** an incident can be closed with.

**Sign-in notice** is the system use notice shown on the sign-in page before anyone signs in: authorised use only, use is monitored and recorded (the CJIS Security Policy asks for one). **Auto-dispatch waits** is how long a call of an auto-dispatch type waits for a dispatcher (see Call types).`,
  },
  {
    id: 'admin-integrations', title: 'Integrations', route: '/admin/integrations', keys: 'integrations bitvoice ali eido radio webhook token certificate views eagleview panoramax mapillary neris nfirs fire reporting public feed pulsepoint cpr neighbouring rapidsos held lis supplemental location',
    body: `How bitdispatcher connects to call handling and radio. Each integration shows its live state and a checklist of the hooks in use.

- **Attachments** (under Connections): a **ClamAV daemon** (clamd, \`host:port\` or its socket) scans incident files and message attachments before they are kept; a file it reports is refused and recorded, and if clamd cannot be reached the upload is refused rather than kept unscanned. **Test** checks a clean file passes and the standard anti-virus test file is caught. **Read the words in pictures** uses the Tesseract program on the server for search (pictures never leave it); plain text and PDFs are read without it.

- **BitVoice**: its address and API token, and its certificate pinned with Read it. See [BitVoice](#bitvoice).
- **More BitVoice systems**: Add a BitVoice follows another one too, such as a neighbouring PSAP's: its name, address, API token, and its own EIDO token and radio secret (New makes them). Its calls show its name in the call queue, and what you do with one of them (text, rebid, video, photos) goes back to that BitVoice.
- **Neighbouring agencies**: their CADs, for sending and receiving incidents. See [Incidents with neighbouring agencies](#cad-to-cad).
- **Supplemental location (RapidSOS, NG9-1-1 LIS)**: a location server asked by HELD for where the caller's phone is: its address and the credentials the provider gives you, **Ask for every new 911 call**, and **Try a number**. The call-taker can press **Ask <name>** on the ANI / ALI panel. Not yet tested against RapidSOS itself (docs/SUPPLEMENTAL-LOCATION.md).
- **Public incident feed**: open incidents for the public or apps like PulsePoint: hundred-block addresses, points to about 100 m, no callers or notes; optional token and push address; **See what it shows** before turning it on.
- **EIDO**: the token a call-handling system uses to push EIDO to \`/api/ng911/eido\`.
- **Radio**: the secret for radio webhooks to \`/api/radio/events\`.
- **ALI spill**: the TCP address bitdispatcher listens on, and Test.
- **Hooks, drones and translation**: the hook token for transcripts, crash data, drone reports and radio words, the drone system, and the optional language model. See [Hooks and connections](#hooks).

**Views** (the place from every side, see [Views](#views)):

- **EagleView**: an application on EagleView's developer portal with Imagery API access under the agency's licence; its client ID and secret. **Sandbox** uses EagleView's test area (Omaha) to try it.
- **Panoramax**: no key; empty uses the catalogue of every instance, or give the agency's own instance.
- **Mapillary**: a client token from Mapillary's developer dashboard.
- **Google Street View link**: a link only.
- **Test at the map's centre** looks the centre up with each provider that is on and says what came back. Save first.

**NERIS** (fire and EMS reporting): the department's NERIS ID (FD and eight digits), a CAD integration's client ID and secret from NERIS, and the dispatch centre's name. With *Send each fire or EMS incident when it closes*, every closed incident with a fire or EMS call type or unit goes to NERIS as a CAD record (call, unit and clear times, the place, the disposition, the notes; police units left out); NERIS keeps it pending until the fire department completes it. **Check the last closed incident** asks NERIS to validate one without keeping it. Supervisors see *NERIS sent* (or why it was refused) on a closed incident, with **Send to NERIS** to send it again; a second send updates the same record. Use NERIS's test environment until NERIS has checked the integration.`,
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

Delivery failures BitVoice reports show on the Situation panel.

**What a 911 call from BitVoice shows** on the call (incident panel and the call-taker's ANI / ALI):

- **number verified** (or **NOT verified**) from the caller ID's STIR/SHAKEN check, with the attestation. A failed check is a reason to doubt the number, not the emergency.
- **ESInet**, the service asked for, a **CAMA** legacy trunk and how its location was looked up, and the ways text came (MSRP, SIP, RTT, TTY).
- **Address in the address data** or **NOT**, from BitVoice's LVF: missing parts, *Did they mean…* corrections, and whether the caller's position agrees with the address.
- **Location check (BitVoice)** after a rebid: which location to show first and why.
- **Caller photos and clips**: each with its SHA-256, the camera's time and GPS, shown as facts until you press **Reveal (recorded)**; every reveal is in the ledger with who looked. **Hold as evidence** keeps them past BitVoice's retention.
- Caller video opens blurred and muted; **Reveal** is recorded in BitVoice's exposure record.
- A text-to-911 call waiting for a call-taker: **Take this text**, then reply in the box.
- When a call-taker passes the call on: BitVoice's briefing for whoever takes it, and who passed it on.
- After the call: the recording's name and its summary. The briefing and the summary are both kept, each marked as written by a language model.

If BitVoice ends a call because **the caller's audio stopped** (the line went silent and no hang-up came), the call says so and the advisory panel raises it as a silent 911 call: call back.

Reports › Operations report shows BitVoice's answer times (the NENA standard: 90% within 15 seconds), who looked at callers' photos and video, and the i3 log events received. The BitVoice light in the status bar turns amber when a PSAP the ESInet routes to is full, down or unstaffed.`,
  },
  {
    id: 'admin-users', title: 'Users and positions', route: '/admin/users', keys: 'users roles password viewer field dispatcher supervisor admin positions sessions demo demonstration public',
    body: `Each person holds one or more **duties**: call-taker, police, fire or EMS dispatcher, dispatcher (all disciplines), supervisor, administrator, records, field crew or viewer. They choose one at [sign-in](#signin); the rights follow it. Their role, the most they may ever do, is the least that holds all their duties:

| Role | Can |
|---|---|
| Viewer | See everything, change nothing (records staff are viewers who also work records) |
| Field crew | The [mobile terminal](#mdt) for its own unit |
| Dispatcher | Work incidents, units and calls |
| Supervisor | Also reopen incidents, read the ledger and reports, certify reports |
| Administrator | Also change configuration and users |

**One-time codes**: the list shows who has an authenticator set up; **Reset** for a lost phone. You cannot change your own duties.

Passwords are generated or at least 12 characters. Five wrong passwords lock an account for 15 minutes. The positions list shows who is signed in where.

**A public demonstration account.** Tick **Public demonstration account** on a viewer, dispatcher or supervisor and the sign-in page offers **Sign in as the demo user**, with a choice of its duties: one press signs a visitor in, with no password, at a position of their own (WEB- and four letters) unless they typed one. One account at most, never an administrator or a field crew, and wrong passwords typed for it do not lock the button out. Use it only on a demonstration server: anyone who can reach the server can then work as that user.`,
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
    id: 'admin-continuity', title: 'Admin: continuity and overflow', route: '/admin/continuity', keys: 'continuity internet check checklist contacts continuity pack folder overflow neighbour partner ask help accept dispatch call types agencies certificate fingerprint',
    body: `What the [Continuity](#continuity) and [Overflow](#overflow) pages use.

## Internet checks
Up to 10 https addresses bitdispatcher asks every 30 seconds (10 to 600); any one answering means the internet works. Choose ones the centre relies on: the state message switch's portal, the paging provider, a large public site. With none set, the consoles cannot say whether the internet is up; BitVoice, the other sites and the overflow neighbours are checked anyway.

## Moving to the backup site
- **Checklist**: one step a line, ticked on the Continuity page when the centre moves.
- **Contacts**: the backup site, the neighbours, the phone company, the 911 authority; printed on the continuity pack.

## Continuity pack
How often it is written (1 to 240 minutes, or not on a schedule) and up to 5 folders, as full paths, beside the data directory. Files are readable by the server's account only.

## Overflow
First add the neighbour in Admin › Integrations › Neighbouring agencies, with its CAD's address (https://their-cad/api/cad/eido/(the ID they gave us)) and a token each way; if their certificate is their own rather than from an authority, add its SHA-256 fingerprint to pin it. Then, for each neighbour:
- **We may ask them to take our calls**, and **they may assign our units** while they do.
- **We take their calls when they ask**, and **accept their requests on our own** for an agreement that says yes in advance.
- **Our agencies** they may take calls for (empty: all; with one, their calls are its incidents and only its units are shared), and **call types** they may use (empty: all).
- **An overflow lasts** 240 minutes unless set otherwise or ended.

> Agree overflow in writing with the neighbour, and with the 911 authority, which sets up the phone network's alternate routing that sends the 911 calls over.`,
  },
  {
    id: 'admin-premises', title: 'Premises and hazards', route: '/admin/premises', keys: 'premise premises hazard hazards caution officer safety knox box dog oxygen contacts key holder',
    body: `A premise is what the centre knows about a place before anyone calls: **hazards** to the crews (a dog, oxygen in use, an armed occupant, a Knox box), **notes** and **contacts**. It is shown with every incident within 40 m of it, on the dispatcher's incident panel and on the mobile terminal, hazards first in red. An expired premise (a temporary hazard past its date) is shown as expired.

- **Add a premise**: type the address and pick it from the suggestions (that places it), click the common hazards or type others, add notes and contacts.
- **Search** by address, name, hazard or id; click one to change or remove it.
- Supervisors and administrators change premises; everyone sees them. Each change is a ledger event.
- **Pre-plan photos and views**: on a premise, add photos (with a caption and the side they show it from) and see the [views](#views) of the place. A photo's SHA-256 is in the ledger: a file changed on the server is refused, not shown.

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
    id: 'admin-workstations', title: 'Workstations', route: '/admin/workstations', keys: 'workstation workstations console position enrol enroll code browser sign in duties this computer',
    body: `A **workstation** is one of the agency's console positions. A browser enrolled as a workstation signs in there, and nobody is asked which console they are at: every action is recorded with the position already known.

- **Add** a workstation with a short id (\`CT1\`, \`DISP2\`) and, if you like, a name for where it sits.
- **Duties offered** narrow what a sign-in at that position may take: Call-taker, Police, Fire or EMS dispatcher, Dispatcher (all disciplines), Supervisor, Administrator, Records. Pick none to offer every duty.
- **Enrol this browser** makes the computer you are on that workstation; it is kept here, so it is remembered next time.
- **Get a code for another computer** shows a one-time code. On the other computer, open the sign-in page, choose "Enrol this computer" and type it. It works once, for 24 hours.
- **Enrolled browsers** are listed with their label and when; revoke any that should no longer sign in there.

Every enrolment and revocation is a ledger event.`,
  },
  {
    id: 'admin-plans', title: 'Response plans', route: '/admin/plans', keys: 'plan plans response run order due order recommendation preferred units area address time',
    body: `A **response plan** is a run order: the units an agency wants, in due order, when a plan applies. The recommendation follows a plan's due order when one applies; if a planned unit cannot go it says why and who goes instead. A person always decides.

A plan applies when all of its conditions match the incident:
- **Call types**: the selected codes (empty means every type).
- **Modifiers**: the selected modifiers, if any.
- **First-due station area**: a station's area, or Anywhere.
- **Addresses**: the incident address starts with one of these lines (one per line), for an address-specific plan.
- **From / To**: a time of day window (HH:MM), optional.

The **due order** is the unit ids in the order they are wanted; add from the list, reorder with the up and down buttons, remove any. A plan without an order narrows nothing and is ignored.`,
  },
  {
    id: 'admin-facilities', title: 'Receiving facilities', route: '/admin/facilities', keys: 'facility facilities hospital trauma burn stroke cardiac psychiatric diversion closed transport destination patients',
    body: `**Receiving facilities** are the hospitals and specialty centres a patient may be taken to, and whether each is taking patients. Crews and the console see the status beside a transport decision.

- **Add** a facility with an id, name and kind (hospital, trauma, burn, stroke, cardiac, psychiatric or other), its phone and position.
- **Status** is a coloured chip: open (taking patients), on diversion, or closed. A quick button changes it in one click, and the change is an attributed ledger event.

Nothing here decides a destination; it tells the people who do what is open.`,
  },
  {
    id: 'admin-protocols', title: 'Call-taking protocols', route: '/admin/protocols', keys: 'protocol question determinant proqa emd medical priority dispatch call taking script card interview',
    body: `Your agency's question sets, worked on the call-taker's Entry tab once the call is submitted (docs/PROTOCOLS.md).

- **New protocol** or **Start from the example** (marked *not for use*). Give it an ID, a name and a version, and choose the call types it is offered for.
- **Questions**: each answer leads to the next question or ends at a **determinant**. **Make first** chooses where it starts.
- **Determinants**: a code, what it means, the priority and call type it sets (or keeps), and the instructions for the caller, one per line.
- Tick **Approved for calls** when your medical director or chief has approved it; drafts are not offered. Saving checks that every answer goes somewhere and nothing loops.
- On a call: one question at a time, answers on buttons or keys 1–9; every answer is on the timeline, and the determinant shows on the dispatcher's incident.
- Protocol software you license (ProQA and others) can post its determinant to the hook instead; it shows the same way, marked with the system.`,
  },
  {
    id: 'admin-guidance', title: 'Approved guidance', route: '/admin/guidance', keys: 'guidance policy guide prompt checklist passage citation approved resources call taker effective expires supersedes markdown',
    body: `**Approved guidance** is the agency's policies, call-taking guides, response guides and checklists. The call-taker's Resources tab shows the matching **passages** with citations; nothing is invented, only approved text is shown.

A document has an id, title, **kind** (Policy (mandatory), Guide, Prompts or Checklist), version, effective date, an optional expiry, what it supersedes, and the source where the full approved copy is. Narrow it to call types, modifiers or words, or mark it **always shown**. Only a document marked **Approved for use** is shown to call-takers.

The **text** is Markdown with \`#\`/\`##\`/\`###\` headings. Headings divide it into passages. Under a heading, a line like \`Applies: CARD, CHEST; modifiers: WEAPONS; words: not breathing\` narrows that passage.

- **Add the examples** loads clearly-titled example documents as drafts (not approved), to see how it works.
- **Try it** matches a call type, modifiers and some text and shows the passages a call-taker would see, with their title, heading, kind, version and why, or a note when nothing matches.

A document's status is a chip: approved or draft, and outdated once its expiry has passed.`,
  },
  {
    id: 'admin-disclosure', title: 'Disclosure rules', route: '/admin/disclosure', keys: 'disclosure foia records request exempt redact rule category words pattern legal basis counsel jurisdiction',
    body: `**Disclosure rules** are the agency's legally reviewed rules for records requests (FOIA). Nothing is hardcoded as exempt; the system proposes, people decide, and your counsel reviews every rule before it is enabled.

A rule has an id, name, jurisdiction and record types, and matches by:
- **Category**: a named category of the record.
- **Words**: any of a comma-separated list.
- **Pattern**: a Go regular expression.

A **legal basis** is required, with an explanation, an optional effective window, and an enabled switch. A rule stays disabled until counsel has reviewed it.

- **Add the sample rules (disabled)** loads examples; they are examples, not legal advice.
- **Try it** pastes a record's text and shows it with the proposed passages highlighted \u2014 amber where the confidence is "review", rose where it is "high" \u2014 and a list of each with its text, rule, basis and confidence.`,
  },
  {
    id: 'signin', title: 'Signing in: your duty, codes and the lock', keys: 'sign in login duty role call-taker dispatcher supervisor mfa one-time code authenticator totp lock unlock inactivity workstation position',
    body: `Type your username and password. If you hold more than one duty, choose the one you are working: **Call-taker**, **Police**, **Fire** or **EMS dispatcher**, **Dispatcher (all disciplines)**, **Supervisor**, **Administrator** or **Records** (number keys choose). Your workspace opens for it, with your saved preset for that duty ([Your own layout](#layout)) and your usual agencies ([The agency you are working for](#working-agency)), and what you may do follows it: the server enforces it, not only the screen.

| Duty | Opens | May |
|---|---|---|
| Call-taker | [Call entry](#taker) | Enter and update incidents; not send units (unless the agency allows it) |
| Police, fire, EMS dispatcher | The console with that discipline | Send and change their discipline's units (any, if the agency allows it) |
| Dispatcher (all disciplines) | The console | Everything on the floor |
| Supervisor | The console, the [board](#board) | Also intervene anywhere, reopen, read the ledger, run exercises |
| Administrator | Administration | Configuration; works the floor only when signed in for a floor duty |
| Records | [Records](#records) | Review views and records requests; changes no incident |

A supervisor covering a position keeps supervisory rights. You are never asked which console you are at: an [enrolled workstation](#admin-workstations) gives the position; a browser that is not one keeps a web position of its own (WEB- and four letters).

**One-time codes.** When the agency asks for them (Settings › Roles and sign-in), the first sign-in shows a QR code: scan it with any authenticator app and type its six digits. After that, type the current code each time; a code works once. An administrator resets it for a lost phone.

**The lock.** **Ctrl+Shift+L**, or the agency's minutes without input, locks the console: nothing is shown or changed until you unlock it with your password (and code). Everything stays open underneath, including what you were typing. Signing in and out, locking and unlocking are all in the ledger, with the duty and workstation.

> Before relying on these for criminal justice information, confirm your state's CJIS and access requirements.`,
  },
  {
    id: 'taker', title: 'Call entry: the call-taker\u2019s workspace', route: '/taker', keys: 'call taker call entry tabs persons vehicles previous hazards search comments history resources requests send submit f4 f2 f5 rebid f6 modifiers duplicate close call',
    body: `A calm workspace for calls: the call list on the left, the working calls beside it, ten tabs across the top with only the chosen one shown, and the call (its location, transcript and what was heard) with the **narrative** always on the right.

## Top to bottom, in the order a call is handled
1. **The call attaches itself.** A call answered at your position opens on call entry at once with its ANI, ALI and any supplementary location, each with its source. No second choice of the call is needed. A walk-in or a radio report is typed without a call.
2. **The incident location first** (see [Locations and the place](#place)): the address, its point on the map, how it is settled (a verified address point, an estimated range, the caller's device, a map point, or forced for this incident) and the place within it: business or building name, complex, building, floor, apartment or room, and entrance or gate detail. Flags at a verified place show at once, before the call is sent, on every tab.
3. **The caller and the callback**, read back and ticked as confirmed.
4. **The call type** (type a code or a word; arrows and Enter choose), the priority and the **modifiers** (some raise the priority, some add people to notify; officer-safety ones show red).
5. **Send** (F4 or Ctrl+Enter). The narrative on the right takes comments all along: **Enter adds a comment with its time**, there is no second button. Comments written before sending go with the call.

When something the call needs is missing, the message names it and the cursor goes to that field. The only thing a call must have to be sent is its type; an unverified or forced location never holds it up.

## Several working calls
To the left of call entry, **Working calls** holds each call in progress in its own screen, up to the number the agency allows for the duty (three for a call-taker unless it says otherwise). Each keeps its own caller, location, type, people, vehicles and comments.

- **Next call** (F2 or F5) opens a new call-entry screen and keeps the current one exactly as it is, sent or not. When the agency's limit is reached it asks whether to close out a sent call to make room.
- A sent call **stays open** for more comments and caller updates until you press **Close call**. Sending, choosing another call or opening another screen never closes it.
- Each screen shows its number, the type or incident, the place, how long it has been open, **ACTIVE**, **SENT**, and **!** when it still needs a type or address.
- Closing an unsent screen with information in it asks first and asks why; the ledger records each one opened, moved, sent, closed or discarded. They come back after a reload or at another console.

| Key | Does |
|---|---|
| F4 or Ctrl+Enter | Send the call; after it, save changes |
| F2 or F5 | Next call: a new call-entry screen, this one kept |
| F6 | Rebid the caller's location |
| F8 | Write in the narrative |
| F3 | CAD search |
| Alt+1 \u2026 Alt+9, Alt+0 | Change tab (Alt+0: NCIC Requests) |

A laptop that uses F2 for volume or F4 for the microphone shows **Fn+F2** on the button once the [key test](#keytest) has found it. The keys are the agency's profile for the duty (Ctrl+/ shows them).

**Before a second incident.** Send first looks for open incidents at the same address, nearby or from the same callback. **Add this call to it** puts the call (and what you wrote) on that incident; Send again makes a new one.

**The other tabs.** **Persons** and **Vehicles** (see [Persons and vehicles](#persons-vehicles)), **Previous**, **Hazards** (flags, premises, officer-safety people, BOLO matches and docked [Street View](#streetview)), **CAD Search**, **Comments**, **History**, **Resources** (the agency's [approved guidance](#resources)) and **NCIC Requests** for the call (see [NCIC and state requests](#requests)).`,
  },
  {
    id: 'resources', title: 'Resources: approved guidance with the call', keys: 'resources guidance policy procedure call taking guide checklist prompts citation version effective',
    body: `As the call type, modifiers and what is said change, the **Resources** tab finds the passages of the agency's approved policies, call-taking guides, response guides and checklists that apply, and lights up when there is something new. Each passage shows its document, section, version and effective date, whether it is **policy (mandatory)**, a guide, a checklist or suggested prompts, and why it matched; **The whole document** opens the full approved text at that section. Outdated passages and two policies that both apply are marked. When nothing applies it says so.

Only approved text is shown, word for word; nothing is generated. Opening the tab records which passages, at which versions, were shown for the incident, so a reviewer later sees what was available at the time. Documents are kept in [Approved guidance](#admin-guidance).`,
  },
  {
    id: 'dispatching', title: 'Dispatching: reasons, warnings, plans and acknowledgement', keys: 'dispatch reason override warning confirm verified location plan run order first due acknowledge ack no ack talkgroup bolo facilities diversion passdown notify mutual aid',
    body: `- **Send rec** on a pending incident in the queue dispatches its recommendation in one press.
- **A reason.** Sending units other than the recommendation asks why (quick choices, or your own words); on the command line write it after a slash: \`D E5 / closer by radio\`. The reason and the response plan followed are kept with the dispatch.
- **Warnings.** When the location is not verified, is uncertain or is missing, the recommendation says so and you confirm before sending.
- **Response plans.** When a [plan](#admin-plans) applies, its due order leads; a planned unit that cannot go is struck through with why, and the unit going instead says whose place it takes. When the recommendation changes, the change is shown until you mark it seen.
- **Acknowledgement.** A dispatched unit that has not acknowledged in the agency's seconds flashes **NO ACK**; press it (or \`ACK E1\`) when it does. A crew acknowledges on its terminal.
- **Call entry facts** on the incident: modifiers, people and vehicles, a changed location (in amber for half an hour), where the caller is, BOLO matches, the talkgroup (\`TG FIRE-OPS2\`) and notifications still to make, each with **done**.
- **Situation**: the shift **passdown** log, active **BOLOs** (issue, cancel; \`BOLO text\` on the command line; crews see them) and **facilities** with one-press open / diversion / closed.`,
  },
  {
    id: 'exercises', title: 'Exercises, injects and staging', keys: 'exercise instructor inject drill training timeline debrief staging publish configuration change control',
    body: `**An exercise** is one training board for everyone who joins it: tick **Training** at sign-in and type the exercise's name (any name; the first to join makes it). Trainees sign in with their own duties, so a call-taker, dispatchers and a supervisor work it together, and crews can join on the [mobile terminal](#mdt) the same way. Practising alone, leave the name empty.

**The instructor** (a supervisor or administrator) presses **Inject**: a new caller (where, who, and what they say as the exercise goes), the caller saying more or moving, a unit going out of service, an emergency button, or a unit on the radio. **Timeline** downloads every event for the debrief, and when the exercise ends its timeline is kept (Supervisors: \`/api/exercises\`).

**Staging.** An administrator signed in with Training changes call types, units, stations, response plans, guidance or facilities on the training board, tries them there, then opens **Staged changes**: it lists what differs from the live board; choose the parts, say why, and **Publish**. With *Change configuration only through staging* on (Settings), those changes cannot be made on the live board directly.

> The training board is not a backup of the live board. A continuity site is a separate, tested arrangement: see [Several sites](#admin-sites).`,
  },
  {
    id: 'console-route', title: 'The full console', route: '/console', keys: 'console full dispatcher screen call taker switch',
    body: `**#/console** always opens the full dispatcher screen, whatever duty you signed in for: a call-taker can look at the whole board there, and come back to [call entry](#taker) at **#/taker**.`,
  },
  {
    id: 'self-service', title: 'Self-service setup', keys: 'self-service setup install go live remote support on site checklist parallel run cutover backup restore pilot',
    body: `An agency can set bitdispatcher up by itself, with no one from outside on site. Everything is configured on the admin screens, and the [setup checklist](#admin-setup) shows what is left. Support, whether from community support or a support agreement, is remote: e-mail, phone or a video call.

## Who does what
- **Your administrator** works through the checklist on the admin screens: the map, stations, units, call types, users and connections.
- **Your IT staff** run the server (or two, for a [second site](#admin-sites)), its backups and its certificate.
- **Your phone, radio and paging vendors** connect their systems to bitdispatcher's documented interfaces: ALI spills or EIDO for calls, the station alerting gateway, paging gateways. Anything that needs hands, cables or a site visit is theirs.
- **Whoever supports you** answers questions and reads the [support bundle](#admin-support) you send. No one needs to see your screen or sign in to your system.

## The order that works
1. **Install** one server and sign in as the administrator. See [Getting started](#start).
2. **Work the checklist** from the top: Admin › Setup checklist. Each step opens its page and its chapter here. [Open the checklist](#/admin/setup)
3. **Bring in the old system's data**: premises and hazards, call types and run cards and, if you want them, past incidents ([Migration](#admin-migrate)); alarm permits ([Alarm companies & permits](#admin-alarms)).
4. **Train** in Training mode: a copy of the system where nothing reaches real units. See [Training](#training).
5. **Run beside the old system** for a few weeks: enter calls in both, and compare the recommendations and the times.
6. **Pass the go-live check**: Admin › Go-live check. Fix what fails and run it again. [Open the check](#/admin/golive)
7. **Switch over** at a quiet hour, with the old system still running read-only for a day.

## Backups
The [ledger](#ledger) is the system: every setting, unit and incident is rebuilt from it. Copy the whole data directory off the server every night, keep several nights, and once in a while restore one on a spare computer and sign in. A second site is not a backup: a mistake is copied to both at once. The go-live check proves the ledger replays; it cannot see copies kept elsewhere, so the checklist asks you to confirm them.

> bitdispatcher sends nothing to BioDream or anyone else by itself. A support bundle leaves your system only when an administrator downloads it and chooses to send it.`,
  },
  {
    id: 'admin-setup', title: 'Setup checklist', route: '/admin/setup', keys: 'setup checklist steps progress done required recommended optional not needed mark done confirm',
    body: `Admin › Setup checklist lists everything to do before the first live call, in order. bitdispatcher checks most steps itself as you work: a step is done when its page has what it needs.

- **Required** steps must be done before going live: the agency's name, the recorded map, stations, units, call types with run cards, the people (administrators with an authenticator), no demonstration data, nightly backups, training, and a passed [go-live check](#admin-golive) from the last week.
- **Recommended** steps make the system much more useful: the county's GIS, the phone system, station alerting and paging, a second site, running beside the old system, and who supports you.
- **Optional**: NCIC through the state message switch, which the state's CJIS office has to approve.

Each step says why it matters and what bitdispatcher found (*3 stations; not on the map: Station 4*). **Open** goes to the page that does it; the **?** opens its chapter.

**Mark done** is for what only a person can know: training given, backups copied off the server, the parallel run finished. Add a note if you like (*all three shifts*). **Not needed** is for a recommended or optional step the agency does without, with the reason (*no paging: stations are alerted by radio*). Steps bitdispatcher checks itself cannot be marked done by hand. Every mark is in the ledger with who made it; **Undo** takes it back.`,
  },
  {
    id: 'admin-golive', title: 'Go-live check', route: '/admin/golive', keys: 'go-live check test ready restore ledger verify test calls certificate authenticator mfa paging sites disk demo',
    body: `Admin › Go-live check runs automatic tests before the first live call. Run it again after any big change: a new map, new stations, a new phone system. **Run the check** takes a few seconds; the result is in the ledger, and the [setup checklist](#admin-setup) counts it as done when nothing failed in the last week.

Each test passes, has something to look at, or fails:
- **The ledger is intact**: every event's hash and link, from the first.
- **The ledger restores**: a copy of the ledger is replayed on its own and compared with the running system (stations, units, call types, incidents).
- **Room on the disk**: fails under 2 GB free, warns under 10 GB.
- **No demonstration data**: no demo stations, premises, accounts or NCIC simulator.
- **Administrators use an authenticator** (CJIS asks for a second factor).
- **FIPS 140-3 cryptography**: passes when the server was built with \`make fips\` (the Go Cryptographic Module under FIPS 140-3 certificate #5247); warns otherwise. CJIS asks for validated cryptography for criminal justice information.
- **A trusted certificate**: warns when the server uses its own self-signed one.
- **The recorded map covers every station.**
- **Test calls at every station**: every call type with a run card, at every station that has units. Each is worked out and thrown away: nothing is written to the ledger and no unit is touched. It warns when drive times are straight lines, when another station's unit comes before the station's own (a [response plan](#admin-plans) can be the reason), and when a run card asks for units the agency does not have available.
- **The phone system is connected**, **station alerting and paging work** (a page delivered in the last 30 days) and **a second site is copying** (in the last 2 minutes).

**Fix** opens the page that fixes a test. The check never pages, texts or queries anyone. It cannot see backups kept elsewhere, the phone system's own wiring or the radio: confirm those on the checklist.`,
  },
  {
    id: 'admin-support', title: 'Support and the support bundle', route: '/admin/support', keys: 'support bundle help contact remote logs diagnostics download redacted secrets privacy',
    body: `Admin › Support says who your agency asks for help and builds a support bundle to send them.

**Who to ask for help** is the agency's own text: the IT help desk, a support agreement's number and hours. Empty, it points to community support on GitHub (never post caller data, addresses or secrets there). Support for bitdispatcher is remote; anything that needs hands is done by your own staff and vendors.

**Download the bundle** builds one zip file with what a helper needs to find a problem without seeing your screen: the version, operating system and uptime; the ledger's size and check; disk space; the setup checklist and the last go-live check; the link to the phone system and the other sites; counts of units, stations and incidents and the kinds of events in the last 7 days; the settings, stations, units, call types and response plans; the connections; and the server's recent warnings and errors.

**What is left out**: calls, incidents, transcripts, recordings, caller names, numbers and locations, premise notes, usernames, passwords, tokens and keys. Every token, password and key in the connections becomes *(set)*. E-mail addresses, phone numbers and anything that looks like a token are removed from every file.

**Read it before you send it.** A name someone typed into a setting or a call type's notify list stays in. bitdispatcher sends nothing by itself: you download the bundle and decide whether to send it, and to whom. Each download is recorded in the ledger with who made it.`,
  },
  {
    id: 'help', title: 'This manual', route: '/help', keys: 'help manual documentation guide search',
    body: `This manual is built into bitdispatcher, so it is there wherever it runs, with no internet. Every screen's Help link opens its chapter; the palette (\`Ctrl K\`) finds chapters by name. The same chapters are printed as docs/manual/index.html and a PDF by \`make manual\`.`,
  },
  {
    id: 'transcript', title: 'Transcripts and what was heard', keys: 'transcript speech text details extraction address weapon vehicle plate typing',
    body: `When call handling transcribes calls, each call shows its **transcript** live, the caller in white and the call-taker in blue, and **Heard on the call**: the details pulled out of it, so the call-taker types less.

- **What is pulled out**: the address, weapons, vehicles, plates, the direction someone went, how many people, injuries (not breathing, unconscious, bleeding), hazards (fire, trapped, gas, a dog, an electric car) and the caller's name. Urgent ones are red and come first, and show on the call in the queue.
- **Use**: on an address, places the call's incident there (a map search, exact when the map knows the number; check it on the map). Before there is an incident it copies the address.
- **Where transcripts come from**: BitVoice's live transcript when it sends one, its transcript of a recorded call afterwards, or any speech-to-text engine posting to the transcript hook ([Hooks](#hooks)).
- Rules, not a model, pull out the details: the same words always give the same details, and each shows the words it came from.

> A transcript is what a machine heard. The call-taker's own judgement and questions still decide.`,
  },
  {
    id: 'translation', title: 'Callers in another language', keys: 'translation spanish language interpreter phrasebook reply speak model',
    body: `A caller speaking another language is translated line by line on the transcript, marked with the language (**Spanish · translated**); **Show original** shows what was said.

- **The phrasebook** built into bitdispatcher works offline. For **Spanish** it translates the words of emergencies both ways; when it knows too few of a line's words it says **gist only**: get an interpreter.
- **Twelve more languages**: Arabic, Chinese (Mandarin), French, Haitian Creole, Hindi, Japanese, Korean, Polish, Portuguese, Russian, Tagalog and Vietnamese. The language is recognised from its script or its words. A caller's line becomes a **gist**: the emergency words found, in order, with … for the rest (\"fire · my house · … · please hurry\"). The call-taker's **ready phrases** are said in each of them; typed English is not, because the phrasebook will not make up a sentence it does not have. These phrasebooks were machine-written and have not been checked by native speakers.
- **Say it in the caller's language**: press a ready phrase (Help is on the way, Is the person breathing?, Get out of the house now…) or, in Spanish or with the language model, type in English; the translation shows as you type. It is kept on the transcript, texted when **also text it** is ticked (BitVoice), and the speaker button says it aloud on this console in the browser's voice for the language.
- **A language model** (optional, Admin › Integrations) translates what the phrasebook cannot, on an OpenAI-compatible chat API: one on your own server, or a service under your agency's agreement. What callers say is sent to it, so it is off until an administrator switches it on.`,
  },
  {
    id: 'caller-location', title: 'Where the caller is: device location and floor', keys: 'location dbh device hybrid gps floor altitude z-axis trail fixes',
    body: `Each call says how its position was found: **device location** (the phone's own fix, often a few metres, the FCC's dispatchable location), GPS, cell sector or on file.

- **Floor**: a civic floor from call handling, and the **altitude** of a 3-D fix with its uncertainty, which tells which floor of a tall building.
- **Trail**: a caller who moves leaves a dotted trail on the map of every position received, the selected call's brighter.
- **Rebid** asks for a new position; **STALE** marks an old one (Admin › Settings).
- No good position? **Send location link** ([Location links](#location-link)) lets the caller's phone send its own.`,
  },
  {
    id: 'location-link', title: 'Location links: the caller shares where they are', keys: 'location link share location gps caller phone browser text link here',
    body: `When a call brings no usable position (a landline forwarded from a mobile, a caller who does not know where they are, a poor cell fix), **Send location link** on the call sends the caller a link: texted through BitVoice when the call can take a text, or shown to read out.

- The caller opens it and presses **Share my location**; the phone asks to allow it. Its own position (GPS, a few metres outdoors) comes to the call, marked *Shared by the caller (phone's location, by link)*, and again as the caller moves, about every few seconds while the page is open.
- The call shows the link's state; **end it** stops it. The caller can press **Stop sharing**. A link lasts 30 minutes (Admin › Settings › Calls: location links) and takes at most 900 positions.
- The link holds a long random secret; only its hash is kept. It opens a small page with no sign-in and nothing else on it.
- The link uses the public address in [Paging groups](#admin-paging): the caller's phone must be able to reach this server at it, with a certificate the phone trusts.`,
  },
  {
    id: 'grouping', title: 'Many callers, one incident', keys: 'duplicate callers group cluster attach all same fire',
    body: `When several people call about the same thing, the queue groups them under **N callers about one place**: calls near each other (300 m by default) within a short time (10 minutes).

- Make the incident from one call; the group then offers **Attach N to** that incident, one press for the rest.
- With an incident selected, the group offers to attach all of them to it.
- Calls already on different incidents are never grouped together. Change the distance and time in [Admin › Settings](#/admin/settings).`,
  },
  {
    id: 'nonemergency', title: 'Non-emergency calls', keys: 'non-emergency screening noise complaint parking admin line deflection',
    body: `Calls on a **non-emergency line**, or whose caller says something that is seldom an emergency (a noise complaint, parking, a power outage), go to the **Non-emergency** list under the 911 calls, with the reason.

- Anything urgent said on the call (a weapon, not breathing, fire) keeps it an emergency, whatever else was said; so does crash data.
- **It is an emergency** moves it back; a person's decision always stands and is recorded.
- **Send the usual reply** texts the reply set in Settings, such as where to report online.
- Set the lines, the words and the reply in [Admin › Settings](#/admin/settings).
- Reports taken by the [non-emergency intake](#admin-intake) wait here too, marked *a report taken by the non-emergency intake*, with every answer; they do not time out of the list.`,
  },
  {
    id: 'admin-intake', title: 'Non-emergency intake', route: '/admin/intake', keys: 'non-emergency intake bot ai receptionist report questions transfer emergency words token try',
    body: `A voice or text bot on the non-emergency line (BitVoice's AI receptionist, or another) can take reports. The bot carries the voice; bitdispatcher decides what it asks.

- **Questions**: asked in order, each with the kind of answer (words, an address, a phone number, yes or no). *needed* asks once more when the answer is empty. A yes/no question marked **yes = hand over** sends the caller to a person on yes, or when the answer is unclear. The keys \`where\`, \`phone\` and \`name\` fill the report's address (placed on the map when the recorded map finds it exactly), callback and caller.
- **Handing over**: the moment a caller says one of the emergency words (by default: emergency, hurt, bleeding, fire, gun, breaking in, right now… and the agency's critical words), or anything the transcript reader marks urgent, the bot is told to say the hand-over line and transfer the caller to a call-taker. A word just after *no*, *not* or *nobody* (\"nobody is hurt\") does not count.
- **A finished report** is a call in the [non-emergency list](#nonemergency) with every answer, the conversation and a report number the caller is told. Nothing is dispatched by it; a person makes an incident of it or closes it. A conversation handed over is a call marked emergency, with what was said.
- **The bot's token**: **Make the token** (shown once). The bot calls \`POST /api/intake/sessions\` to start and \`POST /api/intake/sessions/<id>\` with each thing the caller says; each answer says what to say next and whether to hand over (docs/INTAKE.md).
- **Try it** runs the saved script in a chat, as a caller would, and makes no report.`,
  },
  {
    id: 'crash', title: 'Vehicle crash data', keys: 'acn aacn crash telematics delta-v rollover airbag injury risk electric vehicle onstar',
    body: `A vehicle's automatic crash notification arrives as a call with **CRASH DATA**: the vehicle, delta-V (the change of speed in the crash), the direction of the impact, rollover, airbags, occupants and belts, and its power (electric, hybrid).

- **Injury risk** is high for delta-V of 40 km/h (25 mph) or more, a rollover, or 20 km/h with no belt, several impacts or a side impact; medium for 20 km/h or any airbag. Suggestions follow: ALS, extrication, air medical, more than one ambulance, and for an electric car a word to the fire crews.
- It is a rule of thumb from field-triage criteria to help pick the response, not a diagnosis. Change the thresholds in [Admin › Settings](#/admin/settings).
- Crash notifications reach bitdispatcher through the crash hook ([Hooks](#hooks)).`,
  },
  {
    id: 'drones', title: 'Drones', keys: 'drone dfr first responder launch aerial skydio brinc video',
    body: `With a drone system connected ([Admin › Integrations](#/admin/integrations)), an incident on the map shows **Send a drone**. bitdispatcher sends the drone system the incident, its type, priority, position and summary; the drone system decides which drone flies.

- **Launch rule**: incidents of call types marked **Send a drone** (Call types) ask by themselves, once each.
- The drone reports back to the drone hook: its status, position, battery and live picture. It flies on the map, its status goes on the incident's timeline, and **Live picture** opens its video.`,
  },
  {
    id: 'admin-drones', title: 'Admin: Drones', route: '/admin/drones', keys: 'drone dfr mavlink px4 ardupilot rtsp camera signing key pilot approval launch altitude battery record video evidence sha-256 dronesim',
    body: `Drones bitdispatcher flies itself, over MAVLink, the protocol of the PX4 and ArduPilot autopilots. Each drone has:

- **MAVLink link**: \`udp-listen:0.0.0.0:14550\`, \`udp:host:port\`, \`tcp:host:port\` or \`serial:/dev/ttyUSB0:57600\`, and the autopilot's **system id** (0 takes the first one heard).
- **Signing key**: 64 hex digits for MAVLink 2 signing. Turn it on: without it anyone who can reach the link can command the drone. A drone signing with another key is ignored.
- **Camera**: an RTSP address for H.264 video. The password is masked here and never logged.
- **Height** (10 to 120 m) and the **lowest battery** for a launch.
- **Needs pilot approval** and the **pilots**: listed pilots approve a launch; with none listed, supervisors do. A launch rule always waits for a person.
- **Record video**: while the drone is on an incident its video is kept with the incident, with its size and SHA-256 written to the ledger every 10 seconds. A recording that no longer matches is not served.

On an incident, **Send a drone** picks the nearest ready drone. Dispatchers can press **Hold** or **Return home**; the pilot's own controller can take over at any time. Training boards never fly real drones. Check Part 107, Remote ID and your state's drone laws before flying.

Tested with the bundled simulator (\`bin/dronesim\`) only, not yet a real drone. Details in docs/DRONES.md. [Open Drones](#/admin/drones)`,
  },
  {
    id: 'radio-text', title: 'Radio transcripts', keys: 'radio transcription speech words transmission timeline',
    body: `When BitVoice or the radio system transcribes transmissions, a unit's words go on its incident's timeline in italics (\`E1: on scene, smoke showing\`), so the record holds what was said on the air. The words come with BitVoice's radio events, or through the radio-words hook ([Hooks](#hooks)).`,
  },
  {
    id: 'summary', title: 'Incident summaries', keys: 'summary paragraph handoff crews mdt',
    body: `Every open incident has a **Summary**: what and where, what the callers said that matters (the urgent first), the priority notes, the hazards on file at the place, who is on scene and who is responding, and a drone over it.

- It is built from the record by rules, so it says nothing the record does not; it changes as the incident does.
- It shows on the incident panel (Copy puts it on the clipboard) and at the top of the crews' [mobile terminal](#mdt).`,
  },
  {
    id: 'care', title: 'Critical calls and care', keys: 'critical call trauma wellness peer support debrief break cism eap',
    body: `Some calls stay with the people who take them. An incident is a **critical call** when its call type is marked critical (cardiac arrest, shooting and stabbing to begin with) or when words such as not breathing, CPR, officer down or infant are said on it.

- Everyone who worked it (the call-taker and the dispatchers who entered, sent or noted) goes on the **care list** on the [supervisor board](#board), with the reason.
- The supervisor **offers a break** to each person, **arranges a debrief** (with a note of when and where) and marks it **Done**. Each is recorded.
- Once the incident closes, each person sees a quiet line on their own console: that was a hard call, take a moment, and the support line set in Settings. **Take a break** puts them on a break.
- Set the words, the support line and the call types in [Admin › Settings](#/admin/settings) and [Call types](#/admin/calltypes).`,
  },
  {
    id: 'breaks', title: 'Breaks and workload', keys: 'break workload fatigue relief supervisor people overtime',
    body: `The cup on the status bar puts you **on a break** and back; the supervisor board shows it.

The board's **People** panel lists every signed-in position: calls in the last hour, actions, time since the last break, critical calls in four hours, and a state:

- **watch**: two hours without a break (Settings), a critical call, or a very busy hour;
- **relieve**: three hours without a break, or two critical calls.

It names the least busy positions free to relieve someone.`,
  },
  {
    id: 'training', title: 'Training', keys: 'training scenario simulation practice trainee score reset',
    body: `Tick **Training** when signing in. The console opens a board of its own, marked TRAINING in violet: the agency's units, stations, call types, premises and map, on a ledger of its own. Each trainee has their own board, even on a shared account; one left idle for two hours is cleared. Nothing done there reaches the live board. Administration there is staging: an administrator's changes stay on the training board until published (see [Exercises, injects and staging](#exercises)); users, connections and maps are the live board's.

- **Scenarios** play calls, transcripts, crash data and radio traffic into it on the agency's own map: a house fire with three callers, a cardiac arrest in Spanish, an interstate crash with crash data, an armed robbery, and a flood of calls on the line.
- **Score** checks what was done: every caller on an incident, the call type, the units sent, a reply in the caller's language, and each incident's [quality review](#quality).
- **Reset** empties your board and starts it again; other trainees' boards are untouched.`,
  },
  {
    id: 'quality', title: 'Quality review', route: '/admin/quality', keys: 'quality assurance qa review score coaching standards questions',
    body: `Every incident checked against the centre's standards:

| Check | Standard (Settings) |
|---|---|
| Answered in time | 15 s, NENA's call-answering standard |
| Entered in time | answer to incident, 90 s |
| Dispatched in time | entry to the first unit, 60 s, priority 1 and 2 |
| Location verified, callback, disposition | captured |
| Questions asked | heard on the transcript: the address, the callback, and by discipline (breathing and awake for EMS; anyone inside for fire; weapons, description and direction for police) |

Each incident gets a score over the checks that applied; the page lists them for a period, each call-taker's average and each check's pass rate. Press an incident for its checks. A review is a coaching aid: a missed question may simply not be on the transcript.`,
  },
  {
    id: 'offline', title: 'Working through an outage', keys: 'offline outage server down network failover paper outbox service worker',
    body: `Each console keeps the last board it had. When the server cannot be reached, an amber bar says so and the console shows that board, as it was, with the time. A console reloaded during the outage still opens.

- **Offline log**: new incidents, notes and unit statuses are kept in this browser, in order.
- When the server is back the bar offers **Send them now**: each goes in marked "entered offline at 14:02 by jlee"; anything the server refuses stays, with the reason.
- For a server that never goes away, run bitdispatcher at two sites ([Several sites](#admin-sites)): each keeps working when the other is cut off.
- When the internet, rather than the server, is gone, or the centre has to leave its building, see [Continuity](#continuity).`,
  },
  {
    id: 'continuity', title: 'Continuity: outages and the backup site', route: '/continuity', keys: 'continuity outage internet down backup site relocation evacuate coop checklist continuity pack paper links survivability',
    body: `The **Continuity** page (the life-ring button in the status bar, or a chip there when something is wrong) is what keeps the centre working when a link or the building is lost.

## When the internet goes
bitdispatcher runs on the centre's own server, so losing the internet does not stop dispatch. Call entry, incidents, units, the recorded street map, routing and addresses, the ledger and printing keep working.
- What waits for the internet: online map tiles, Street View and aerial photos not recorded, traffic, map updates, neighbouring centres and overflow, NERIS, PSC PIP, and BitVoice or paging when they are reached over the internet.
- bitdispatcher checks the addresses an administrator sets ([Admin › Continuity](#admin-continuity)), BitVoice, the other sites and each overflow neighbour. When none of the internet checks answers, every console shows **Internet down** in red and the supervisors get a message; another link down shows as an amber chip.
- Each link going down and coming back is in the ledger; the page lists the links and the outages.

## Moving to the backup site
When the centre has to leave its building, staff go to the backup site: a second bitdispatcher site the county owns or runs in the cloud ([Several sites](#admin-sites)). It already holds every record, so work carries on from where it was.
1. At the backup site, a supervisor presses **We are working from the backup site**, with a note. Every console at every site shows **At backup site**, and everyone on duty gets a message.
2. The checklist set in Admin › Continuity is ticked as it is done (rerouting 911, radio control stations, telling the field); each tick is in the ledger.
3. Back home, **We are back in our building** ends it.

> The 911 calls reach the backup site only when the phone network sends them there: the 911 authority's alternate routing, or BitVoice's routing in NG911. bitdispatcher cannot move the calls itself.

## The continuity pack
**Open to print** gives a page of the open incidents (with callers' names and callbacks), every unit and its status, the checklist, the contacts and blank log lines, for working on paper if the CAD cannot be reached at all. It can be written on a schedule to folders an administrator sets: a shared drive or a USB disk on another machine keeps a recent copy if this server is lost. Treat printed copies as records.`,
  },
  {
    id: 'overflow', title: 'Overflow to a neighbouring centre', route: '/overflow', keys: 'overflow overwhelmed surge neighbour neighbouring county centre psap mutual aid take calls help federate backup',
    body: `When the centre is overwhelmed, or out of its building, a neighbouring centre that also runs bitdispatcher can take its calls; and this centre can take theirs. It is set up for each neighbour in [Admin › Continuity](#admin-continuity), after both centres agree it in writing.

## Asking for help
1. On the **Overflow** page a supervisor presses **Ask (neighbour) to take our calls**, says why and for how long (4 hours unless set otherwise).
2. The neighbour's supervisor accepts or declines; a neighbour whose settings accept on their own starts at once. Both centres' status bars show it: *(neighbour) is taking our calls*.
3. Each call they take becomes an incident here, under our numbers and call types, marked **Taken for us by (neighbour)** with their call taker's name. Our dispatchers send our units, unless we let the neighbour assign them too.
4. A supervisor at either centre presses **End** when it is over.

## Taking calls for a neighbour
While it lasts, the Overflow page shows **Take a call for (neighbour)**: the location, their call type, the caller, the callback and what happened. **Send** makes the incident in their CAD and shows its number.
- What becomes of the call comes back: its status, the units on it, their notes and the close. Notes added here go to them.
- If they let us, **Their units** lists their available units to assign.
- If their CAD cannot be reached, the call is kept here and sent again every 20 seconds; tell them by radio or phone if it is urgent.

## What crosses
Only the calls the neighbour takes, and what becomes of those: no other incidents, no records, no person history, no NCIC returns. Every request, answer, call, note and assignment is in both centres' ledgers.

> The 911 calls themselves reach the neighbour by the phone network's alternate routing, which the 911 authority sets up, or by BitVoice's routing in NG911. Overflow carries the work, not the calls.`,
  },
  {
    id: 'tdos', title: 'Line attacks (TDoS)', keys: 'tdos denial of service flood robocall repeat caller silent calls attack',
    body: `A telephony denial-of-service fills the lines so real callers cannot get through. bitdispatcher watches the calls and raises **Possible telephony denial-of-service** on every console and the supervisor board when:

- far more calls arrive in a minute than usual (five times the last hour's rate, at least 12, or the number set in Settings);
- one number calls five times or more in ten minutes (shown as ×5 on its calls);
- eight or more silent or very short calls arrive in five minutes.

A supervisor **acknowledges** it; it clears after ten quiet minutes. Keep answering: a real emergency can be among them.`,
  },
  {
    id: 'hooks', title: 'Hooks and connections', keys: 'hooks api transcript acn drone radio token connections webhook',
    body: `Other systems send to bitdispatcher with \`Authorization: Bearer\` and the hook token set in [Admin › Integrations](#/admin/integrations). Without a token the hooks are off.

| Hook | Sends |
|---|---|
| \`/api/hooks/transcript\` | \`{"call": "id", "speaker": "caller", "text": "…", "lang": "es"}\`, or \`lines\`: a call's words |
| \`/api/hooks/acn\` | a crash notification: JSON or key=value, common telematics names (delta_v_mph, pdof, rollover, airbag_deployed…) |
| \`/api/hooks/drone\` | \`{"id", "status", "lat", "lon", "battery", "incident", "videoUrl"}\` |
| \`/api/hooks/radio-text\` | \`{"unit" or "radioId", "channel", "text"}\` |

The drone system's launch address and token, the launch rule, and the optional translation model are set on the same screen.`,
  },
  {
    id: 'records', title: 'Records and requests', route: '/records', keys: 'records foia freedom of information review audit release redaction disclosure request requester',
    body: `The records area reads a finished incident and answers requests for it, all from the same tamper-evident [ledger](#ledger) the rest of the system writes to.

## Reviewing a record
The review screen shows one incident four ways, and opening or exporting a view is itself recorded in the ledger.

| View | What it shows |
|---|---|
| As first entered | The incident as it was first created, before any later change |
| Summary | The short account |
| Detailed | Everything on the incident |
| Full audit | Every ledger event, with a hash-chain check: it confirms the number of events and reports any break |

**Print** or **Download text** keeps a copy; both are recorded. Open any incident by its number.

## Records requests
A request names a requester, a contact, what is asked for, the incidents, and whether the **summary** or **detailed** view is released. Each request shows the working days left until it is due (amber when close, rose when overdue) and the working time taken so far.

1. **Assemble** builds the release copy from the chosen view and proposes redactions by the agency's disclosure rules; the original record is untouched.
2. For each incident, accept or reject each proposed redaction, giving the **basis** required for anything withheld, or **mark a passage** by selecting it in the record. A live preview shows the redacted copy.
3. **Save decisions** records every decision, accepted and rejected.
4. **Approve and release** is the moment a person authorises the release: accepted redactions are applied permanently and the copy is checked for leaks before it leaves. A released copy can then be downloaded, and its hash is shown.

> Records work needs the records duty, a supervisor or an administrator.`,
  },
  {
    id: 'alarms', title: 'Alarm levels: 2nd and 3rd alarms', keys: 'alarm 2nd 3rd second third upgrade greater alarm box working fire run card',
    body: `A working fire outgrows its first run card. Each call type may have **further alarms** (Admin › Call types): the 2nd alarm, the 3rd, each adding units to what the incident needs. The first alarm is the call type's own run card.

## Upgrading
- On the incident, **Upgrade to 2nd alarm** says what it adds (\`+2 engine, 1 ladder…\`); or type \`ALARM\` (\`2ND\`, \`3RD\`, \`ALARM 3\` for a level).
- The upgrade is a priority line on the timeline every dispatcher and crew sees, with what it adds.
- The recommendation then fills the bigger run card, counting the units already there, and \`REC\` (or Ctrl Enter) sends the extra units. Paging groups and station alerts follow the dispatch as usual.
- **Bring down** goes back a level, with a reason.


> Setting up: Admin › Call types › a type › Further alarms. Each alarm has a name and run-card lines (capability and count). The structure fire type comes with a 2nd and 3rd alarm to edit.`,
  },
  {
    id: 'admin-personnel', title: 'Personnel and crews', route: '/admin/personnel', keys: 'personnel roster crew staffing members qualifications paramedic minimum staffing shift riding',
    body: `The **roster** is the agency's people: a number, name, rank, qualifications (paramedic, EMT, driver, officer…), home station, and how to page them (a phone for texts, an email, a push topic). Add them one by one or **Import from CSV** (columns: id, name, rank, qualifications separated by ;, station, phone, email, volunteer).

## Who is aboard
On any unit (the unit's panel › Crew › change) or from the crew's own terminal (**Crew**), pick who rides it this shift. A member is on one unit at a time: putting them on another moves them. The incident's timeline records crew changes on a unit working it.

## What staffing changes
- **Minimum aboard** (Admin › Settings › Unit types, or per unit in Admin › Units): a unit below it is shown short-staffed, in the recommendation's reasons and on the supervisor's exceptions.
- **What a capability needs** (Admin › Settings › Crews): by default ALS needs a paramedic aboard. A medic whose recorded crew has no paramedic is not recommended for an ALS line (it is still BLS and transport).
- A unit with **no crew recorded** is taken as staffed: an agency that does not keep rosters here sees no change.

> Dispatchers see the roster without phone numbers and emails; supervisors see them.`,
  },
  {
    id: 'accountability', title: 'Fire-ground accountability: roll calls (PAR)', keys: 'par personnel accountability report roll call benchmark elapsed time mayday missing crew fire ground',
    body: `At a working fire the incident commander takes a **roll call** (a PAR) of every crew on scene at set intervals, and dispatch keeps the clock.

- A call type marked **Fire-ground accountability** (Admin › Call types) starts the clock when the first unit arrives on scene; **Start roll calls** starts it on any fire incident.
- The incident shows the time since the clock started and when the next PAR is due; it turns amber when overdue, and the supervisor's board lists it.
- **All accounted for** (or \`PAR\`) records a roll call of every crew on scene. A crew that did not answer: click it, then **Report … missing** (or \`PAR MISSING E3\`). That is a red, flashing line on the incident and an urgent exception for the supervisor until **E3 found** (\`PAR FOUND E3\`).
- **Benchmarks** the commander reports (primary search complete, fire under control, loss stopped…) are one click each, or \`BM UC\`, with the time since the incident started.
- The clock stops when the incident closes; it cannot be stopped while a crew is missing.

> The interval is 20 minutes unless changed in Admin › Settings or on the call type.`,
  },
  {
    id: 'staffing', title: 'Staffing: who is on duty and on each unit', route: '/staffing', keys: 'staffing duty roster sign on sign off crew position driver officer relief swap secondary detail overtime holdover availability minimum staffing out of service limited capability override shift change template correction',
    body: `**Staffing** (the people button in the status bar) shows who is on duty, where, and who actually staffs each unit. A call sign alone is not enough: the crew follows the unit into incidents, recommendations, history and reports.

## People on duty
**Sign on** with the kind of duty (regular, overtime, holdover, a temporary detail and where, a call-back), the shift, until when and availability (available, assigned, unavailable, training, court, meal, station duty, administrative). **Sign off** clears the person from any unit. Search by name, qualification, agency, station, unit and availability.

## Crews
**Edit crew** names the people on a unit and their **positions** (driver, officer, firefighter, medic, partner, team leader, or the agency's). The kind of unit may require positions and a minimum (Admin › Settings › Unit types). A crew may sign itself on from the terminal; the agency decides per kind of unit whether that is allowed, needs a supervisor's approval (**Sign-ons waiting**) or is for dispatch only.

- One person is on one active unit at a time. Putting them on another asks: **move them** (relief, a swap) or a **secondary assignment** with a reason.
- Each change has a reason and an **effective** time, kept beside when it was entered.
- A unit left short (below its minimum, a position empty) is **limited**, or put **out of service**, as the agency says, unless a supervisor records an **override** with a reason.
- A lost connection signs no one off.

## On incidents and later
When a unit is assigned, the incident keeps who was aboard and what they were qualified for at that moment; later changes never rewrite it, and changes during the incident (who joined, who left, positions) are on its timeline. A supervisor can **correct** a crew record with a reason: the original stays beside the correction, with the entered and effective times. Unit recall and reports show the people as well as the call sign.

Supervisors also see **saved crews** for a **shift change** in one step (each person and time still recorded), and **coverage**: the minimum qualified people the agency wants on duty, gaps, expiring credentials and each unit's capability (a supervisory view the agency controls).`,
  },
  {
    id: 'admin-qualifications', title: 'Qualifications, special teams and credentials', route: '/admin/qualifications', keys: 'qualifications teams special swift water hazmat technical rescue dive k9 swat uas drone evidence technician fire investigator negotiator tactical dispatcher cto fto instructor evaluator preceptor emt paramedic credential certificate expiry renewal verify suspend equivalency mutual aid capability',
    body: `Each agency keeps a controlled skills file (Admin › Qualifications & teams; supervisors).

- **Qualifications**: special teams (Swift Water, HazMat, Technical Rescue, Dive, K-9, SWAT, UAS), specialties (evidence technician, fire investigator, crisis negotiator, tactical dispatcher), training roles (CTO, FTO, instructor, evaluator) and credentials (EMT, paramedic…), each with its levels, the agency that owns it, how many months it lasts, when renewal is due, its prerequisites, whether it must be **verified** before it counts, and the documents it needs.
- **A person's credentials**: the level, state (trainee, qualified, instructor, evaluator, restricted, temporarily unavailable, suspended, expired), issuer, dates, restrictions and attached certificates. A supervisor marks one **verified**; an unverified self-entry never counts where verification is required. Every change is kept.
- **Equivalencies**: another agency's qualification accepted as one of ours, with its scope and expiry, for mutual aid; the person's own record is never merged or overwritten.
- **Reminders**: supervisors are told once when a credential's renewal comes due and when it expires.

## Live capability
A unit's capability comes from the apparatus and the people validly qualified aboard now: a HazMat unit without its required technicians is not HazMat-capable, and shows **LIMITED** with the reason (what each capability needs of a crew is set in Personnel). The recommendation uses the live capability; the agency decides whether sending a short unit is only warned, needs a supervisor's override, or is blocked. Dispatchers see the capability and the qualification names; credential details are for supervisors and the person.`,
  },
  {
    id: 'admin-paging', title: 'Paging responders', route: '/admin/paging', keys: 'paging page pager volunteer off duty sms text email push ntfy twilio iamresponding active911 responding groups',
    body: `Volunteer and off-duty firefighters and EMTs are called by **page**. A **paging group** says who is paged together and how:
- **Members** from the roster ([Personnel](#admin-personnel)).
- **Gateways**: how pages go out, set up by an administrator in Admin › Integrations › Paging: a text by Twilio, an email, a push to phones by ntfy, a webhook, or IamResponding and Active911 (both take dispatch by email in their documented formats).
- **Automatic**: page the group when one of its units or stations is dispatched, or when an incident of one of its call types gets units. Each group is paged once per incident by its rule.

## A page
The page's subject is the call type and address, the text the incident's summary and units. Each person gets a link to answer: **Responding to the station**, **Going straight to the scene**, or **Not available**, with when they expect to arrive. No sign-in: the link is the key, and works only for that person and that page.

On the incident, **Responders** shows how many were paged and responding, each person's answer and minutes away; **Page** pages more groups. \`PAGE <group> / text\` does it from the command line. A page no one has answered after five minutes is on the supervisor's exceptions; failed deliveries are on the timeline.

> Set the **address responders open** (Admin › Integrations › Paging) to the address phones reach this server at; without it pages go out with no answer link. Tested against stand-ins for Twilio, ntfy, email and a webhook; not certified by IamResponding or Active911.`,
  },
  {
    id: 'admin-tows', title: 'Tow rotation', route: '/admin/tows', keys: 'tow rotation wrecker towing company class zone heavy light owner choice declined turn',
    body: `Police call wreckers from approved companies **in turn**. Each company tows some classes (light, medium, heavy) in some zones (Admin › Settings lists both; one zone for the whole area by default).

- **Next on rotation** on the incident (or \`TOW\`, \`TOW heavy North\`) gives the company whose last turn in that class and zone is the oldest, with its number to call, and records the call.
- Mark what they said: **Accepted**, **Declined** or **No answer** (both lose the turn; ask for the next), then **Arrived** or **Cancel**.
- An **owner's choice** or **officer's choice** of company is recorded but takes no one's turn.
- A suspended company is passed over, and the rotation screen says why each company is or is not offered.

Every request and answer is on the incident's timeline and in the ledger, so the rotation's fairness can be shown.`,
  },
  {
    id: 'schedule', title: 'Scheduled calls and special events', keys: 'schedule scheduled call transport event special event standby fair parade repeat daily weekly',
    body: `Work known ahead of time (the calendar button in the status bar):
- A **scheduled call** (a planned transport, an escort, a business check) becomes an incident by itself at its time, or a set number of minutes early, and sends its units if they are available (those that are not are named on the incident).
- A **special event** (a fair, a parade, a game) becomes a standby incident for its hours, and is listed as running on every console.
- Either can repeat every day, on weekdays or every week, until a last day.

**Scheduled today** on the situation panel lists what is coming in the next 24 hours and what is running now. An occurrence missed by more than an hour (the server was down) is not made late; it is skipped and recorded.`,
  },
  {
    id: 'admin-directory', title: 'Contacts directory and on-call lists', route: '/admin/directory', keys: 'directory contacts phone numbers utility on call rota call out list notify agency medical examiner interpreter',
    body: `The numbers dispatch calls: utilities, agencies, the medical examiner, interpreters, public works. Each entry has numbers with labels (after hours, cell), an email, notes and tags.

- **On-call lists** say who answers tonight: shifts of directory entries by date and time, with a backup when no shift covers the time.
- An entry or list can be on the **call-out list** of call types: every incident of the type lists **To call** with the person on call now and the number (a tap calls it on a phone), and **Called** logs it on the incident. A call type's notifications (Admin › Call types › Notify) are matched to entries by name, organisation or tag.
- The directory button in the status bar (or \`DIR water\`) searches by name, organisation, tag or number; the situation panel shows who is on call now.`,
  },
  {
    id: 'admin-geofences', title: 'Areas on the map (geofences)', route: '/admin/geofences', keys: 'geofence area zone polygon alert enter leave caution school zone map draw',
    body: `Areas drawn on the map:
- An **alert area** tells dispatch when a unit enters or leaves it (a unit leaving the county, entering a hazard zone, arriving at a hospital): a notice on every console, the situation panel's **Area alerts**, and a line on the unit's incident. It can watch only some disciplines or units.
- A **caution area** puts its note on every incident inside it (a school zone, a firing range, a block known for trouble), on the console and the crews' terminals.

**Draw a new area**: click the corners on the map, then Finish. Areas show on the console map, dashed. Whether a unit is inside is worked out from its positions as they arrive, so an area drawn around a unit already there is not an arrival.`,
  },
  {
    id: 'admin-roads', title: 'Road closures & traffic', route: '/admin/roads', keys: 'road closures traffic waze for cities geojson feed lift remove detour',
    body: `Every road closed now, with why, the roads it takes, since when, until when, the detour and who closed it; and those ended in the last day.

- **Lift** opens a road. **Remove** is for a closure entered by mistake (it stays in the ledger); a closure that was real should be lifted, so the record says it was closed.
- A closure whose line or circle covers no recorded road says so: draw it again along the road.

**Live traffic** (administrators):
- **Waze for Cities** is Waze's free data-sharing programme for public agencies. Once the agency has joined, Waze gives it the address of its traffic feed (JSON); paste it here. Each jam's speed slows the roads under it; a road Waze reports closed is closed.
- **GeoJSON**: any other source, a FeatureCollection of lines with \`speed_kmh\`, \`factor\` (times slower) or \`closed: true\`, and \`name\`.
- Fetched every 2 minutes by default (30 seconds to an hour), with a token sent as Bearer when the feed needs one. The status says when it was last fetched, how many slowdowns it had, or why it failed. A feed that stops leaves the recorded drive times.`,
  },
  {
    id: 'admin-cameras', title: 'Cameras', route: '/admin/cameras', keys: 'cameras cctv traffic cameras policy radius audit who looked live view still picture',
    body: `The cameras dispatchers may open for an incident ([Cameras near an incident](#cameras)): traffic cameras, the agency's buildings, schools and partners that agreed to share.

- **Offer cameras near incidents** is off until an administrator switches it on; the radius (400 m by default) says how far from an incident cameras are offered.
- **Add a camera**: its name, owner, where it is, which way it looks (degrees from north; empty if it turns), how wide and how far it sees, and its **live view** address (opened in a new window in the camera system) and/or its **still picture** address (a JPEG of now, fetched through bitdispatcher). Only https addresses, or http on the agency's own network.
- **Who looked, and why** lists every opening. Agree a policy before switching cameras on: which cameras, for which calls, and who reviews this record.`,
  },
  {
    id: 'admin-alarms', title: 'Alarm companies & permits', route: '/admin/alarms', keys: 'alarm company central station token answer permit register false alarm repeat csv asap',
    body: `**Companies**: each central station the agency works with gets its own address (\`/api/hooks/alarm/<company>\`) and token, shown once when it is made; its alarms arrive with its name, and what became of them goes to its own answer address. A new token stops the old one. Companies not on the list can still use the shared hook token (Admin › Integrations).

**Permits**: load the agency's alarm permit register as CSV, with a heading row: number, address, holder, company, expires (YYYY-MM-DD), status (active, suspended, revoked). Loading replaces the register. Each alarm is matched by its permit number, else its street address.

**False alarms**: an address's incidents in the last year whose disposition is a false alarm are counted (by default, dispositions with \"false\" in them); at the limit (3 by default) the alarm says **REPEAT FALSE ALARMS**.

Phoned-in alarms are typed on the alarm card ([Alarms](#alerts)).`,
  },
  {
    id: 'unit-history', title: "A unit's status history", keys: 'unit status history day utilisation timeline committed time csv',
    body: `**History** on a unit's panel shows its day from the ledger: a bar of its statuses across the day, every status with when, how long, the incident and who changed it, its status checks, crew changes and move-ups, the incidents it worked, and the share of its known time spent on incidents. Pick another day with the date; **CSV** downloads the same.

**Unit recall** ([Search](#search)) gives any period in one timeline for front-line use, with who staffed the unit, its messages and its locations.`,
  },
  {
    id: 'epcr', title: 'Patient-care reports (NEMSIS)', keys: 'epcr nemsis patient care report ems hand off times eTimes eDispatch imagetrend eso',
    body: `An EMS crew writes its patient-care report in its ePCR system; the dispatch part (when the call came in, when the unit was notified, en route, on scene, left the scene, at the hospital, back in service, where, and the dispatch reason) comes from bitdispatcher.

- **NEMSIS** on an EMS unit of the incident downloads its response as a NEMSIS 3.5 record, to import into any ePCR that takes NEMSIS files.
- With a patient-care report system set up (Admin › Integrations › Patient-care reports), **Send** posts it there, and **automatic** sends each EMS unit's response when it clears. What the system answered is on the incident.
- The dispatch reason is the call type's NEMSIS code (Admin › Call types), or suggested from its name.

> The record carries dispatch, times and the scene only; the crew completes the patient sections. It follows the NEMSIS 3.5.1 schema for those sections; tested against a stand-in, not a vendor's system.`,
  },
  {
    id: 'papers', title: 'Warrants and papers to serve', route: '/papers', keys: 'warrant warrants civil process paper service protective order eviction summons subpoena serve attempt caution',
    body: `The papers a sheriff's office or police department holds to serve: arrest and bench warrants, protective orders, evictions, summonses and subpoenas (the gavel button in the status bar). Each has the person, where to serve it, the court and number, any **caution** (armed, violent, dog), and every attempt.

- An incident near the address of an active paper (or naming its person) shows **active paper at this address** with the caution to every console and to the police crews' terminals; **Details** (police dispatch, records and supervisors) shows the papers.
- **Record the attempt**: the unit, the result (no answer, not there, moved, refused, served) and a note. Served closes it; **Recalled** and **Returned unserved** need a reason.
- Every look at the papers is recorded in the ledger.

> Confirm every warrant with the issuing agency (and NCIC where it applies) before acting on it.`,
  },
  {
    id: 'public-alerts', title: 'Alerting the public', keys: 'public alert mass notification warn shelter in place evacuate wea wireless emergency alert eas cap ipaws amber 911 outage megaphone cancel update approve',
    body: `Supervisors warn the public from the console: **shelter in place**, **evacuate**, police activity, a hazardous spill, a missing child, 911 being down. Press the **megaphone** in the status bar, or **Alert the public** on an incident.

## Writing an alert
- **Real alert**, **Exercise** or **Test**: an exercise or test is marked as one everywhere it goes. Check this first.
- **Kind of alert**: the SAME event code (SPW shelter in place, EVI evacuation, LEW law enforcement, HMW hazardous materials, CAE child abduction, TOE 911 outage…). Choosing one fills the wording, in English and Spanish, to start from.
- **Headline**, **what is happening** and **what to do**.
- **Phone alert**: what phones show first (Wireless Emergency Alerts, 90 characters) and what text messages and push notifications say; the counter turns red past the limit. A **longer phone alert** (360) is optional.
- **Also in Spanish** adds a Spanish version; people on the lists who chose Spanish get it.
- **Where**: around the incident (a quarter mile to two miles), an area drawn on the map (Admin › Geofences), or **everyone** in the jurisdiction (its county codes).
- **Expires after** one to 24 hours.
- **Send to** the subscriber lists and the CAP systems (sirens, alert software) chosen; **keep it off some channels** asks IPAWS alert software to leave out radio and TV, weather radio, phones or public sites.

**Check** shows the CAP message it makes and anything wrong with it, and how many people it reaches. **Send** asks once more, with the exact phone text and area. With **two-person approval** on, the alert waits until a second supervisor presses **Approve** (the megaphone shows *to approve*); its author can withdraw it, and the second supervisor can reject it with a reason.

## In force
An alert in force shows red on the map (amber for a test) and on the megaphone, and is in the public alert feed. **Update** sends a new version to the same people that replaces it; **Cancel** sends the all-clear. Every alert, approval and delivery is in the ledger and on the incident's timeline.

> Sending to IPAWS itself (Wireless Emergency Alerts and the EAS) needs FEMA's agreement and the agency's own alert origination software; bitdispatcher writes its messages to the IPAWS profile for that software, and checks them with Google's CAP library and its IPAWS profile and with EAS Station's feed reader. No message has been sent to IPAWS.`,
  },
  {
    id: 'admin-publicalerts', title: 'Public alerts: settings and lists', route: '/admin/publicalerts', keys: 'public alerts settings cap sender same county code feed atom ipaws receivers subscribers list csv two person approval time zone',
    body: `How public alerts are written and where they go (Admin › Public alerts). Administrators change the settings; supervisors keep the lists.

- **Sender**: the agency's CAP sender ID, an address it owns (alerts@county.gov), and its name.
- **Jurisdiction** and **county codes**: the six-digit SAME codes of the counties covered (0, the state's FIPS code and the county's: 051770 is Roanoke city), used for an alert to everyone and added to every alert for radio and TV.
- **Time zone**: CAP times carry the agency's offset.
- **Two-person approval** (recommended): a second supervisor approves each alert before it goes.
- **Public alert feed**: an Atom feed linking to each CAP message, and an IPAWS-style feed with the messages inside, for sirens, broadcasters' EAS encoders and apps. They show only alerts in force.
- **CAP receivers**: systems each alert is POSTed to as CAP (with a Bearer token); **Send a test** sends a test message.
- **Subscriber lists**: people who asked for alerts, reached through the paging gateways (text, email, push). A person with a place given gets only alerts whose area contains it. **Import a CSV** with the columns name, phone, email, topic, lang, lat, lon. A push gateway with no one listed publishes to its own topic, a channel anyone can follow.

> The feeds are public by design: they hold only what was sent to the public.`,
  },
]

/** The chapter for a screen, if it has one. */
export function articleFor(path: string): Article | undefined {
  return manual.find((a) => a.route === path)
}
