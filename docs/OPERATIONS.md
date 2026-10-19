# bitdispatcher: operations, reporting and intelligence

Status: draft, 2026-10-06. Companion to [DESIGN.md](DESIGN.md).

## 1. Principle

Every action that consumes a person's attention, changes operational state, creates accountability or
supplies a required report becomes a structured, time-stamped, attributable event. Free text stays
available for context, but it is never the only place important work is recorded.

One verified event can then serve the live response, shift turnover, the Daily Operations Report,
supervisory review, training, investigations and long-term workload analysis, with no duplicate entry.

Users served: dispatchers, communications supervisors, patrol supervisors, field and communications
training officers, fire and EMS leaders, investigators, analysts, records staff, accreditation staff and
administrators.

The claim is not that other CAD systems cannot search or report. It is that bitdispatcher is designed from
the start to make cross-role work visible, attributable and reusable, under role and purpose controls,
instead of leaving agencies to assemble the picture from audit tables, add-ons and professional services.

## 2. The event ledger

The ledger is bitdispatcher's foundation and is designed before any report or dashboard: reports cannot
recover events that were never captured.

- **Append-only.** Corrections and late entries are new linked events; history is never overwritten.
- **Action separate from outcome.** Initiating an action, receiving a result, viewing a record, attaching a
  record and communicating information are distinct events.
- **Batches are a parent plus children.** Eleven records attached in one action are eleven attachments
  plus one batch event.
- **Stable identifiers** across CAD, RMS, mobile, personnel, units and external interfaces.
- **Sensitive payloads stored apart** from reportable metadata.

| Field group | Minimum fields |
|---|---|
| Identity | Event ID, event type, action subtype, actor personnel ID, active role, agency, console, position or board, authenticated session ID |
| Time | System time, effective time, source-system time, time zone, late-entry or backdated flag |
| Operational link | Incident, unit, person, vehicle, location, case or report number, related event |
| Action and result | Initiated, result received, viewed, attached, communicated; success, failure, timeout or cancellation reason |
| Source and destination | CAD, RMS, mobile, radio, telephony, AVL, state or federal justice systems, motor vehicle agency, imported interface, export destination |
| Governance | Authorisation role, purpose code where required, dissemination restriction, sealed or juvenile flag, retention class, audit correlation ID |

## 3. Event families

### 3.1 Calls and incidents

- Created, accepted, reviewed, updated, reclassified, reprioritised, transferred, queued, dispatched,
  cancelled, closed.
- Comments and narrative added, edited or marked priority; important information acknowledged or relayed.
- Person, vehicle, location, premise, hazard, keyholder, warrant and attachment records created, viewed,
  linked, updated or removed.
- Cross-board and cross-agency assistance, credited even when the helper does not own the incident.

### 3.2 Unit safety and status

- Every status: dispatched, en route, arrived, transporting, at destination, available, out of service,
  reassigned, preempted, stacked, cleared, manually changed.
- Status timer warning, timeout, status check started, each contact attempt, acknowledgement, resolution.
- Emergency button, duress, missed acknowledgement, backup request and resolution.
- Location exceptions: stale AVL, unit mismatch, radio affiliation change, loss of contact.
- Supervisor overrides, backdated statuses and corrections, each with a reason and approver.

### 3.3 Criminal justice information

Authorised justice-information work (state message switch, NCIC, Nlets, motor vehicle agency) is counted
and preserved without exposing restricted content to unauthorised reports. State system names are
configurable labels, not product terminology.

| Event | Must be distinguishable |
|---|---|
| Query initiated | Requester, role, purpose or incident, query type, source interface, time |
| Response received | Response class, hit or no-hit, correlation ID, receipt time; payload stays protected |
| Response reviewed | Authorised user, view time, acknowledgement where required |
| Record attached | Each discrete response linked to the incident, counted individually |
| Information relayed | Recipient or unit, method, time, restricted-content handling |
| Record corrected or removed | Who, why, when, and whether the source response remains in the controlled audit record |

### 3.4 Advisory and reference activity

- Candidate association generated, with matching attributes, conflicts, time and geography preserved.
- Association presented, acknowledged, accepted and linked, rejected, deferred or escalated, with actor,
  reason and time.
- Manual association searches: filters, results viewed, incidents opened or linked.
- Hazmat reference lookups suggested or submitted; candidate guide, source edition, confirmation,
  correction and transmission to responders with acknowledgement.
- BOLOs and temporary map controls created, reviewed, updated, extended, cancelled, expired or replaced;
  stale-notice prompts and their outcomes.

## 4. Reports

### 4.1 Daily Operations Report

Generated from verified events, then reviewed and certified by an authorised supervisor; nobody retypes
what CAD already holds. Always written in full to keep it distinct from the training Daily Observation
Report (section 4.8).

- Shift date and period, supervisor, staffing by role and console, vacancies, overtime, holdover,
  consolidated positions.
- Priority incidents and agency-defined significant categories (major fire or EMS events, pursuits,
  weapons, major arrests, missing or endangered persons).
- Timed-out units, status checks, emergency activations, contact failures and resolutions.
- System, radio, telephone, interface or mapping interruptions and periods of manual operation.
- Justice-information activity as permitted categories and counts only.
- Pending calls, warrants, BOLOs, unit and equipment issues, and obligations passed to the next shift.
- Supervisor context, certification, amendment history and distribution record.
- **Drill-down.** Every total links an authorised user to its contributing events. Distributed copies hold
  only the detail appropriate to their audience.

### 4.2 Automated shift debrief

At the configured end of each shift, bitdispatcher assembles a review-ready debrief: staffing and
positions, calls by type and priority, significant and carried-over incidents, timeouts and extended
durations, outages and manual periods, assists, demand peaks, training activity and follow-ups. The
supervisor reviews, adds context, corrects or excludes items and certifies it. Destination is the
agency's choice (section 6.2).

### 4.3 Dispatcher and communications activity

Calls entered, reviewed, dispatched, updated, transferred and closed; touches on owned and non-owned
incidents; comments, record tabs, attachments and queries; status actions, checks, timeouts, emergencies
and assists; concurrency and interruptions by time block and position; an end-of-shift outstanding-work
and turnover report.

### 4.4 Patrol supervisor and unit activity

Select a unit, officer, squad, period, area or event type and review activity in context:

- Traffic stops: how initiated, location, start and clear, duration, disposition, citation or warning,
  backup, related query count.
- Dispatched versus self-initiated, declined or cancelled calls, response and on-scene times, reports,
  assists.
- Status history, extended durations, repeated timeouts and supervisor-reviewed explanations.
- Compared with assignment, beat and available workload. Never a stop quota or a context-free ranking.

### 4.5 Training review

Training views support coaching, not covert surveillance. An authorised trainer or supervisor builds a
review set from selected events and annotates it without altering the incident record.

- Filter by trainee, trainer, competency, event type, priority, unit role and period.
- Replay a timeline of actions, acknowledgements, status changes and decisions.
- Attach policy or training objectives; coaching notes stay separate from the official narrative.
- Separate competency frameworks for police, fire, EMS and communications.

### 4.6 Investigative description search

Authorised investigators search structured person, vehicle and incident attributes and, where policy
permits, approved narrative fields ("tall male, red hoodie, facial tattoo, eastbound, possibly a dark
SUV").

| Capability | Requirement |
|---|---|
| Person descriptors | Height range, build, age range, locally configured sex or gender field, race or ethnicity where legally and operationally permitted, hair, eyes, clothing, scars, marks, tattoos, accessories, direction of travel |
| Vehicle descriptors | Plate or partial plate, state, VIN, make, model, style, year range, colour, damage, decals, linked people and incidents |
| Narrative | Exact phrase, keyword, synonym and similarity search with highlighted source text |
| Link analysis | Repeat locations, vehicles, people, phone numbers and identifiers, under access rules |
| Results | Source, agency, date, match basis, record status and restrictions; filter and exclude |
| Audit | Requester, terms (or a protected hash where policy requires), purpose, records viewed, exports |

A match is an investigative lead, never a confirmed identity.

### 4.7 Engagement, staffing and unit demand

bitdispatcher distinguishes an incident existing, its screen having focus, active engagement and discrete
actions. A screen left open is not proof of work.

| Measure | Meaning |
|---|---|
| Incident open time | Creation to closure |
| Screen focused time | Workspace in focus, possibly passive viewing |
| Active engagement time | Reviewing, entering, querying, attaching, communicating or changing, after configured idle exclusions |
| Action evidence | The time-stamped events themselves |
| Concurrency | Overlapping incidents, radio and phone traffic, queries and safety monitoring |

- **Staffing demand** aggregates human attention by position and time block (engagement, radio and phone,
  justice queries, safety monitoring, required administrative work, concurrency, interruptions) and
  compares it with usable staffed capacity after breaks, training, leave and outages. Radio and phone
  demand come from BitVoice or any call-handling system through standard interfaces.
- **Responder unit demand** sums each resource's committed time by phase (travel, staging, on scene,
  transport, destination, reports) and role (primary, backup, cover, supervisor, specialty, training),
  including stacked incidents and mutual aid: police beat demand, apparatus demand, EMS unit-hour
  utilisation, supervisor span of control. Communications and field time are combined without being
  treated as interchangeable.

### 4.8 Duration and safety review

The agency sets thresholds by call type, priority, role and phase. When one is exceeded, the authorised
supervisor receives an awareness prompt asking for human review. It never declares poor performance or
creates a disciplinary finding.

- Examples: a call-taker still connected after essential information seems complete; a traffic stop past
  its interval; a fire or EMS unit long staged, transporting or at destination; a dispatcher carrying
  several extended incidents.
- Supervisors acknowledge, extend, suppress or escalate with a reason. Thresholds, exemptions, escalation
  paths and recipients are versioned; sensitive call types can have their own rules and visibility.
- It measures operating conditions, never emotion, diagnosis or mental health. Repeated patterns point to
  workflow, staffing and training before individual blame.

### 4.9 Training evidence feed

bitdispatcher supplies read-only operational evidence to the training system; it is never the official
evaluation record and never assigns an evaluation (SEG) rating. The training platform (PIP or another)
holds the Daily Observation Report, where the trainer adds judgement, coaching and ratings.

- Trainer access comes only from a formal assignment with effective dates, phase, discipline, shift,
  unit or position, permitted information and automatic expiry. Trainees see their own released
  evaluations as policy permits.
- Every view, annotation, export and evaluation action is audited.

## 5. Advisory functions

Four functions help with time-critical work. They are built from three different kinds of component,
under one set of safeguards.

| Function | What it does | Built with |
|---|---|---|
| Critical-incident association | As information is entered on an agency-designated high-priority call, finds candidate related incidents (a stolen vehicle and a later driving complaint; an abduction and a tip) | Indexed and fuzzy search over authorised data, with the match explained |
| Fact proposals and conflict review | Proposes structured facts from narrative (person, vehicle, direction, weapon, hazard, injury, child involved) and flags narrative that conflicts with the call type or priority | A local language model proposes; the evidence rules (DESIGN.md 3.3) judge conflicts |
| Hazmat (ERG) lookup | Manual search by UN/NA number or material name, plus suggestions from entered text, producing a confirmed responder card | A versioned local reference, available offline |
| BOLO and closure hygiene | Notices when a BOLO or road closure may be stale (vehicle recovered, new sighting, description changed, incident closed, review time passed) and asks a person to verify | The evidence rules |

### 5.1 Shared safeguards

- **Proposes, explains, asks.** The authorised user decides. Nothing here merges or alters incidents,
  reclassifies, reprioritises, cancels a BOLO, changes wanted status, clears responders, removes a hazard,
  reopens a road or changes evacuation distances, units or tactics.
- **Shows why.** Exact and partial matches, conflicts, source, age, geography and record status; never an
  unexplained score.
- **Rejecting is as easy as accepting**, and both are audited with user, time, reason, source text and
  rules or model version.
- **Not identity, not probable cause.** A candidate is a lead. "This identifier may need an authorised
  query" is allowed; "this person is wanted" without authoritative verification is not.
- **Off the critical path.** Runs asynchronously; never blocks or delays call entry, dispatch, emergency
  traffic or status changes; core CAD works without it. A visible degraded-mode indicator and outage log
  show what was unavailable, and old suggestions never reappear as current after recovery.
- **Corrections propagate.** When text is corrected, negated or superseded, affected suggestions and
  responder cards are marked updated, withdrawn or no longer supported, and recipients are notified when
  it matters.
- **Alert fatigue controls.** Severity tiers, deduplication, bundling, cooldowns, suppression rules, and
  urgent safety prompts presented apart from informational ones.
- **Validated locally first.** A non-production mode over representative local incidents measures false
  positives, missed conditions, response time and user burden before each function is enabled.
  Disagreeing with a suggestion is never treated as employee error.
- **Configured per agency:** eligible call types, priorities, fields, lookback, geography, thresholds,
  recipients, escalation, automatic or user-initiated. Every rule, model, vocabulary and threshold change
  is versioned, approved, tested and reversible.
- **Protected identifiers** (for example Social Security numbers) use masked structured fields, purpose
  controls and restricted logging, never narrative or broadly visible alerts.

### 5.2 Hazmat reference

- Responder card: material, guide number, primary hazards, public-safety guidance, initial isolation and
  protective actions, with source edition and lookup time.
- Ambiguous input shows several candidates or "no reliable match", never a silent best guess; a person
  confirms before anything reaches responders.
- Versioned, validated source with controlled updates, rollback and offline availability; stale or
  unavailable reference conditions are logged and visible.
- Decision support for the first phase of an incident only, never a substitute for agency procedure,
  incident command, shipping papers, safety data sheets or specialists.
- Open: the terms for embedding and caching the current guidebook, which is a joint US, Canadian and
  Mexican publication.

### 5.3 BOLOs and map controls as records

Every BOLO and closure has a stable identifier, originating incident, owner or responsible role,
effective time, last-verified time, status, review interval, expiry behaviour and authorised audience.

- BOLO statuses: draft, active, updated, extended, cancelled, expired, superseded. Map controls: planned,
  active, partial, reopening pending, reopened.
- Updates are versioned and pushed to everyone who received the original, with delivery and
  acknowledgement recorded where policy requires.
- Maps distinguish verified current restrictions from stale, pending-review and historical ones.
- Closing an incident never reopens a road by itself, and a sighting never cancels a BOLO by itself.

## 6. Architecture and delivery

### 6.1 Components

- Operational systems publish normalised events to the append-only ledger through authenticated
  interfaces.
- An identity and authorisation service resolves actor, active role, assignment, agency and data scope.
- The advisory service performs policy-scoped matching and reference lookup asynchronously, publishes its
  health and latency, and fails open.
- A reporting store receives approved fields and derived measures; restricted payloads stay in controlled
  stores.
- A semantic layer defines every measure (touch, query, attachment, timeout, status check, traffic stop,
  training event) once, so dashboards, reports, feeds and exports agree and keep drill-down lineage.
- An immutable audit trail records searches, views, reports, exports, corrections, certifications and
  administrative changes.

### 6.2 Agency-defined measures and delivery

- The agency, not the vendor, defines measures such as "calls handled": definitions, thresholds,
  exclusions, effective dates and audiences are configurable and versioned so past results reproduce.
  bitdispatcher reports evidence; a supervisor or trainer draws the conclusion.
- One canonical structured report payload is built first, then rendered or routed, so no two outputs
  ever use different calculations.

| Configuration | Output |
|---|---|
| bitdispatcher alone | Word, PDF, Excel or CSV |
| With PIP | Secure structured push to the Daily Observation Report, employee performance, Daily Operations Report, shift debrief or another PIP destination |
| With another platform | API, webhook, secure file transfer or supported connector, after agency security review |
| Scheduled | Generated and delivered at shift end or another interval |

Routing can follow purpose and role: trainee evidence to the Daily Observation Report, established-staff
measures to performance, shift-level evidence to the debrief, agency-level data to analytics. Failed or
partial deliveries retry safely and stay visible to administrators.

## 7. Counting rules

| Measure | Rule |
|---|---|
| Incident touch | An intentional action that views or changes operational content; refreshes, passive presence and polling never count |
| Query | Each submitted transaction; class and outcome kept separately |
| Attachment | Each discrete external record; a batch of 11 is 11 plus one optional batch event |
| Status check | The check and its resolution; each contact attempt is a child event |
| Timed-out unit | Once per continuous timeout episode |
| Traffic stop | Once per stop; queries, units, duration and dispositions are linked measures |
| Cross-board assist | Credited to the person acting; the owning position is preserved |
| Engagement time | Active focus intervals and meaningful activity after idle exclusions; screen-focused time reported separately |
| Unit demand | Sum of each resource's committed time by phase and role |
| Workload | Volume, duration, concurrency, priority and exceptions shown together, never one activity score |

## 8. Security, privacy and evidence

- Role, agency, assignment and purpose-based access; least privilege; rapid revocation.
- Current FBI CJIS Security Policy, state and federal justice-information rules, motor vehicle agency
  agreements, local policy, public-records law, discovery and retention schedules.
- MFA where required, device and session controls, encryption in transit and at rest.
- Immutable audit of queries, views, links, exports, prints and administrative actions.
- Field-level controls for juvenile, sealed, protected, medical, personnel, intelligence and victim data.
- Reason-for-access and purpose codes where required. Linking records in CAD never grants permission to
  view or disseminate them.
- Every suggestion, alert, extracted fact, BOLO and map change keeps its source, basis, version, prior
  versions and human disposition, so legal review can reconstruct what happened.
- Discipline, evaluation, enforcement and training conclusions always need human review, with correction
  and dispute processes.

## 9. Build sequence

| Step | Scope | Accepted when |
|---|---|---|
| 1 Event foundation | Identity, sessions, calls, units, touches, status changes, queries, attachments | Every selected action yields exactly one validated, traceable event |
| 2 Operational reports | Dispatcher activity, engagement, turnover, duration alerts, timeouts, Daily Operations Report, shift debrief | A supervisor can reproduce every total from its events and resolve alerts with context |
| 3 Field and training | Unit demand, traffic stops, assignment-limited training views, training evidence feed | Trainers see only assigned trainees; evidence reaches the training record without altering incidents |
| 4 Integration and delivery | Canonical payload, Word and PDF, structured exports, PIP and third-party routing | One versioned calculation, consistent outputs, visible delivery failures |
| 5 Advisory and analytics | Associations, fact proposals, conflict review, BOLO and closure hygiene, hazmat cards, description search, concurrency, staffing models | Every suggestion shows its basis and needs a human decision; none can delay core CAD |

These steps map onto DESIGN.md section 7: step 1 is part of phase 1, steps 2 to 4 run through phases 1 to
4, and step 5 is phase 6.

## 10. Decisions to keep now

- Design the event taxonomy before dashboards.
- Stable identifiers everywhere; sensitive payloads apart from metadata.
- Batches are a parent event plus discrete children.
- Raw events immutable; report definitions versioned; every derived measure explainable and drillable.
- Source incidents stay distinct; associations are reviewed links, never destructive merges.
- Advisory output is a candidate until a person confirms it, and stays off the critical path.
- Design the full alert lifecycle now: creation, delivery, acknowledgement, correction, withdrawal,
  recipient update, disposition, audit.
- BOLOs and map controls are versioned records with owners, review intervals and explicit authority.
- Hazmat lookup is a versioned, resilient reference with manual search and suggestions.
- Roles, purpose, retention and sharing are policy configuration, never hard-coded.
- The Daily Operations Report and the Daily Observation Report are distinct objects, workflows and scopes.
- One canonical report payload before any rendering or routing.
- Validate terms and workflows with dispatch, police, fire, EMS, records, training, IT and legal staff.

## 11. Open questions

- What each agency puts in its Daily Operations Report and shift debrief, and which fields are restricted
  by audience.
- Which evidence may populate a Daily Observation Report, and which evaluation categories may receive
  suggested evidence (never a rating).
- How agencies define active engagement, idle time, concurrency, calls handled and justified extended
  duration.
- Launch report destinations and their authentication, retry and delivery-confirmation contracts.
- Which justice-information transactions may appear as metadata in reports, and which stay only in
  controlled audit systems.
- Which traffic-stop outcomes and officer identifiers originate in CAD versus RMS or e-citation.
- How training assignments for each discipline begin and end.
- Which narrative fields may be indexed for search, and how sealed or corrected records are reindexed.
- Which call types activate association automatically; which attributes, lookback, geography, thresholds
  and recipients apply; who may link incidents and when a supervisor is notified.
- Which narrative details may become proposed facts, which conflicts prompt review, and who confirms.
- The maximum acceptable advisory response time, how degraded mode is shown, and how stale suggestions
  are prevented after recovery.
- The local validation data, false-positive tolerance, missed-condition review and alert-burden limit
  each function must meet before it is enabled.
- Who may issue, update, extend and cancel each BOLO type; what verification follows a located person or
  vehicle; who may reopen a closure and what clearance is required.
- How BOLO and map updates reach CAD, mobile, radio, partner agencies and public maps, and which
  destinations need acknowledgement.
- Which hazmat reference format may be embedded, cached or licensed; how updates are validated; the
  offline requirement; which fields are responder-facing and who confirms transmission.
- Which actions count as cognitive workload and which passive events are excluded.
- Retention, discovery, public-records and employee-access rules for derived activity and training
  reports.
