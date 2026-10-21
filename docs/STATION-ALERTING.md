# Station alerting

When units are dispatched, bitdispatcher tones out their stations (Admin › Integrations › Station
alerting). Each tone-out is a ledger event and a line on the incident's timeline; so is the station's
acknowledgement.

## The station screen

Open `#/station?st=ST1` in a browser in the firehouse, signed in with a read-only (viewer) account. It
shows the station's units and, on an alert, sounds rising tones, reads the call aloud twice (the units,
the call type, the street) with the browser's speech, and shows the call in large type until someone
presses Acknowledge. The screen's position becomes `STATION-ST1`, and the inactivity lock leaves it
alone (a wall display nobody types at). Press the sound button once after opening it: browsers play
sound only after a press.

## An alerting controller

With an address set, bitdispatcher posts each alert (JSON, `Authorization: Bearer <token>` when one is set):

```json
{
  "id": "SA42", "at": "2026-10-20T23:05:00Z", "station": "ST1", "units": ["E1", "M1"],
  "incident": "26-000123", "type": "SF", "typeName": "Structure fire", "priority": 1,
  "address": "215 Church Ave SW, Roanoke VA 24011", "lat": 37.271, "lon": -79.941,
  "speech": "Engine 1, Medic 1. Structure fire, priority one. 215 Church Avenue Southwest.",
  "again": false
}
```

`speech` is the text to read aloud (abbreviations expanded); `again` is true when a dispatcher toned out
again (`TONE` on the command line). A controller acknowledges with
`POST /api/station-alerts/{id}/ack` as a signed-in account.

Vendors' station alerting controllers (Phoenix G2 by US Digital Designs, Locution and others) use their
own CAD interfaces, which are not published. Connecting one is a small adapter from this alert to the
vendor's interface; none is included, and none has been tested against a vendor's controller.
