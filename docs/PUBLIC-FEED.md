# The public incident feed (PulsePoint and others)

Admin › Integrations › Public incident feed publishes the open incidents for the public, a website, or an
app. It is off until turned on.

## What it shows, and what it never shows

Each open incident: its number, the call type's name, the discipline, the address by its **hundred
block** ("215 Church Ave SW" is "200 block Church Ave SW"; apartments and the city are dropped), the point
to three decimals (about 100 m), when it was received, the units on it with their status, and
`cprNeeded` for the call types chosen to ask for CPR-trained bystanders.

Never: callers, callbacks, notes, narrative, persons, vehicles, files, or a house number. Police
incidents only when **Include police incidents** is ticked. Call types under **Never show these** are left
out whatever their discipline. Closed incidents leave the feed.

## Reading it

```
GET /api/public/incidents
Authorization: Bearer <readers' token>      (only when a token is set)
```

```json
{"agency": "Roanoke Valley 911", "generated": "2026-10-20T23:00:00Z",
 "incidents": [{"id": "26-000123", "type": "Structure fire", "discipline": "fire",
   "address": "200 block Church Ave SW", "lat": 37.271, "lon": -79.942,
   "received": "2026-10-20T22:58:10Z", "cprNeeded": false,
   "units": [{"unit": "E1", "status": "enroute"}]}]}
```

Without a token, anyone on the internet can read it (and browsers on other sites may, through CORS); the
settings page says so. With a token it is for one partner. Responses may be cached for 15 seconds.

**Push**: with a push address, the same document is posted there (with the push token as Bearer) when
the open incidents change, at most every 5 seconds.

## PulsePoint

PulsePoint connects to a CAD through its own interface, given to its CAD partners; its specification is
not public, and bitdispatcher has not been tested with it. This feed is what a PulsePoint bridge, or any
other app, reads. Its CPR alerts are meant for public places; an agency that turns `cprNeeded` on should
agree the call types with its medical director and with PulsePoint.
