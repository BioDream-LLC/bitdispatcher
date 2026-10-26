# PSC PIP connector

bitdispatcher can connect to PSC PIP, a separate and optional public-safety-communications
personnel program. This document explains exactly what bitdispatcher is willing to send to such a
program, what it will never send, and how an agency controls the connection.

bitdispatcher is the system of record. PSC PIP is a different product with its own logic. This
connector is a one-way door that lets bitdispatcher hand over a small, agency-approved set of
operational metadata and references to its own records. Nothing else crosses.

PSC PIP has four modules:

- CEDE: demand and exposure context. What an operational period looked like.
- ARC: after-action and readiness workflows.
- ROC: recovery opportunity. Breaks, overtime, consecutive duty.
- RESET: support offers after qualifying events.

## What crosses and what never crosses

What can cross, when the agency allows it:

- Stable references to bitdispatcher's own source records (an incident ID, an exception ID, a
  period ID, and so on). References only, never the content of those records.
- Counts for an operational period.
- A small set of short, structured facts drawn from a fixed allowed list.
- Duty roles involved, such as calltaker, police, fire, ems, supervisor, trainee, trainer.
- People, sent either as usernames or as per-agency pseudonyms, depending on the agency setting.

What never crosses, under any setting:

- Caller, victim or patient names.
- Exact addresses.
- Narratives, call notes, or transcripts.
- Any clinical or wellness content.
- Any score, readiness judgement, discipline action or diagnosis. bitdispatcher does not compute
  these and does not send them.

The connector enforces this. Facts are filtered against a fixed allowed list of keys, and any value
that looks like it carries content is dropped before anything is sent. See "Agency controls" below.

## Event classes

An event is one message to PSC PIP. Each event has a class. The examples below use fictional data
and no real names.

### period.summary (CEDE)

Counts for one operational period: incidents, critical incidents, location exceptions,
sustained-workload minutes, coordination contacts, open follow-ups, and mean exception resolution
minutes.

```json
{
  "id": "evt-period-2026-10-25-day",
  "class": "period.summary",
  "modules": ["cede"],
  "agency": "metro-comm",
  "period": "2026-10-25/day",
  "at": "2026-10-25T19:00:00Z",
  "version": 1,
  "sources": [{"kind": "period", "id": "2026-10-25/day"}],
  "counts": {
    "incidents": 142,
    "criticalIncidents": 3,
    "locationExceptions": 6,
    "sustainedWorkloadMinutes": 95,
    "coordinationContacts": 21,
    "openFollowups": 4,
    "meanExceptionResolutionMinutes": 7.5
  }
}
```

### location.exception (CEDE only)

A forced or estimated location exception, sent as operational context. This is never a performance
item.

```json
{
  "id": "evt-exc-00817",
  "class": "location.exception",
  "modules": ["cede"],
  "agency": "metro-comm",
  "period": "2026-10-25/day",
  "at": "2026-10-25T14:12:00Z",
  "version": 1,
  "roles": ["calltaker"],
  "sources": [{"kind": "exception", "id": "exc-00817"}],
  "facts": {"kind": "forced", "minutes": "9"}
}
```

### critical.incident (CEDE, and RESET only when qualifying)

A critical incident. It always goes to CEDE as demand and exposure context. It goes to RESET as well
only when the agency has marked the event as qualifying.

```json
{
  "id": "evt-crit-00231",
  "class": "critical.incident",
  "modules": ["cede", "reset"],
  "agency": "metro-comm",
  "period": "2026-10-25/night",
  "at": "2026-10-25T23:40:00Z",
  "version": 1,
  "roles": ["calltaker", "supervisor"],
  "people": ["5f9d2c...pseudonym..."],
  "sources": [{"kind": "critical", "id": "crit-00231"}],
  "facts": {"callType": "SHOTS", "priority": "1", "qualifying": "yes"}
}
```

### after.action (ARC)

Sent only when a supervisor opened an agency after-action or readiness workflow.

```json
{
  "id": "evt-aa-00042",
  "class": "after.action",
  "modules": ["arc"],
  "agency": "metro-comm",
  "period": "2026-10-25/night",
  "at": "2026-10-26T00:10:00Z",
  "version": 1,
  "roles": ["supervisor"],
  "sources": [{"kind": "critical", "id": "crit-00231"}],
  "facts": {"status": "opened", "trigger": "critical-incident"}
}
```

### recovery.effect (ROC)

A documented missed or delayed break, extended shift, overtime, consecutive duty, or recovery
action.

```json
{
  "id": "evt-rec-00588",
  "class": "recovery.effect",
  "modules": ["roc"],
  "agency": "metro-comm",
  "period": "2026-10-25/night",
  "at": "2026-10-25T21:30:00Z",
  "version": 1,
  "roles": ["calltaker"],
  "people": ["5f9d2c...pseudonym..."],
  "sources": [{"kind": "break", "id": "brk-00588"}],
  "facts": {"effect": "missed-break", "minutes": "45"}
}
```

### support.offer (RESET)

A support offer opened or linked. This is an operational action only. bitdispatcher records that an
offer happened. It does not record or send what the offer was about.

```json
{
  "id": "evt-sup-00099",
  "class": "support.offer",
  "modules": ["reset"],
  "agency": "metro-comm",
  "period": "2026-10-25/night",
  "at": "2026-10-26T00:20:00Z",
  "version": 1,
  "roles": ["supervisor"],
  "sources": [{"kind": "critical", "id": "crit-00231"}],
  "facts": {"status": "offered"}
}
```

### correction (any module)

A versioned correction of an earlier event. It carries the ID of the event it corrects, and it
routes to the same modules as the class it corrects.

```json
{
  "id": "evt-crit-00231-v2",
  "class": "correction",
  "modules": ["cede"],
  "agency": "metro-comm",
  "period": "2026-10-25/night",
  "at": "2026-10-26T01:05:00Z",
  "version": 2,
  "corrects": "evt-crit-00231",
  "sources": [{"kind": "critical", "id": "crit-00231", "version": 2}],
  "facts": {"class": "critical.incident", "qualifying": "no"}
}
```

## Module routing

The connector fills in the modules for an event from its class and facts. The agency and PSC PIP do
not need to agree on this ahead of time, because the routing is fixed and documented here.

| Class                | Modules                                              |
| -------------------- | ---------------------------------------------------- |
| period.summary       | cede                                                 |
| location.exception   | cede                                                 |
| critical.incident    | cede, and reset when the event is marked qualifying  |
| after.action         | arc                                                  |
| recovery.effect      | roc                                                  |
| support.offer        | reset                                                |
| correction           | the modules of the class it corrects, same rules     |

A correction names the corrected class in its facts. A correction with no corrected class, or one
that names itself, routes to nothing.

## Agency controls

The connection is configured in the CAD's GUI. The agency decides everything.

- Classes. The agency lists the event classes it allows to cross. An empty list means nothing
  crosses. A class that is not on the list is dropped before anything is built or sent. A correction
  crosses only when the class it corrects is on the list.
- Identified or pseudonymous. By default people are sent as a stable per-agency pseudonym. The
  pseudonym is an HMAC-SHA256 of the username keyed with the agency's salt. It is stable, so the same
  person reads the same way across events, and it differs by salt, so it cannot be matched across
  agencies. When the agency turns on the identified setting, usernames are sent as given instead.
- Aggregate. When the agency turns on aggregate mode, only period summaries ever cross. Every other
  class is dropped. This lets an agency share demand and exposure context at the period level without
  sending anything tied to a single event or person.

Facts are always filtered, under every setting. Only keys from a fixed allowed list may cross. The
allowed keys are: kind, callType, priority, qualifying, effect, status, disposition, trigger, class,
duty, minutes, reason. Any other key is dropped. A value is also dropped when it is longer than 64
characters, or when it looks like it carries content, meaning it contains a run of 7 or more digits
(phone or SSN shaped), an "@" sign, or more than 8 words. When a fact is dropped, the connector
records the dropped key names, and only the names, in a `_dropped` field, so the receiver can see
that filtering happened without seeing what was removed.

## Corrections

bitdispatcher never edits an event that has already been sent. Instead it sends a correction, a new
event with a higher version, the `corrects` field set to the ID of the earlier event, and the
`class` fact set to the class being corrected. The correction routes to the same modules as the
original class. This keeps a clear, versioned trail on both sides.

## Audit

Every transfer is recorded in bitdispatcher's ledger as an event named `pip.sent`. The ledger entry
holds the event IDs and the result status. It does not hold the payload. When people are sent as
pseudonyms, the ledger never records those people in plain text. The ledger answers "what was sent,
when, and did it succeed" without becoming a second copy of the data.

## Boundary

bitdispatcher sends operational metadata and references. It does not compute or send any wellness
score, any readiness judgement, any discipline action, or any diagnosis. Those are not bitdispatcher's
job. PSC PIP's logic stays in PSC PIP. If PSC PIP draws conclusions from what it receives, it does so
on its own, with its own rules, and bitdispatcher takes no part in that.

## Optional

The PSC PIP connection is optional. An agency can run bitdispatcher with it off and lose nothing in
the CAD itself. With the connection off:

- The CAD's supervisor alerts work the same.
- The Shift Intelligence report works the same.
- Reviews work the same.

The connector only ever hands a copy of agency-approved metadata to an outside program. It is never
a dependency of the CAD's own features.

## Tested

This connector has been tested against a local test receiver only, using an in-process HTTP test
server. It has not been tested against a real PSC PIP system. No public PSC PIP API specification
exists, so bitdispatcher publishes this contract as its own documented best effort. Before an agency
turns the connection on against a live PSC PIP system, the two sides should confirm that the message
shape, classes, and routing described here match what that system expects.
