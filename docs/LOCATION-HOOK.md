# Newer caller locations: the location hook

bitdispatcher takes a caller's newer location after a call has started: a rebid, a supplemental location
service (a device-based location provider such as RapidSOS), or another system relaying one. It becomes
the call's location, and when the call is on an incident and the new position is more than the agency's
distance (Settings › Call entry, 50 m by default) from the incident's, the call-taker is offered it:
**Use it** makes it the incident's location (the old one stays in the timeline, and dispatchers and units
see "location changed"); **Keep** leaves the incident where it is. Nothing changes the incident's location
without a person.

**Status: a contract, not a vendor integration.** No provider's own API has been integrated or tested.
A provider (or a small adapter in front of one) posts to this hook; BitVoice's rebid already arrives as a
call update. Test against the provider's test data before relying on it.

## The request

```
POST /api/hooks/location
Authorization: Bearer <the hook token, Admin › Integrations › Connections>
Content-Type: application/json

{
  "call": "the bitdispatcher call ID",          // or, instead:
  "callback": "+15405550199",                   // the open call from this number (the newest)
  "lat": 37.27312, "lon": -79.94021,            // WGS 84 degrees (required)
  "radius": 6,                                  // metres of uncertainty
  "altitude": 318.5, "vRadius": 3,              // metres above the ellipsoid, and its uncertainty
  "method": "DBH",                              // device-based hybrid, GPS, Cell...
  "address": "optional one-line address",
  "source": "rapidsos",                         // who says so: shown on the screen and kept in the ledger
  "at": "2026-10-20T13:02:11Z"                  // when the position was determined
}
```

Answers: `200 {"call": "<id>"}`; `404` when there is no such open call; `422` for a missing or impossible
position; `401` for a wrong token; `503` when the hooks are off (no token set).

## What is recorded

Every location is a `call.updated` event with its source, time and uncertainty; an offer is
`incident.location-offered`; the call-taker's decision is `incident.location-decided` with who and why.
