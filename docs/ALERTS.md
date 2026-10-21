# Outside alerts: alarms, plate readers, gunshot detection

Alarm monitoring companies, licence-plate readers, gunshot detection and camera analytics send their
alerts to bitdispatcher over HTTPS. Each alert becomes a **call** in the dispatcher's queue, marked data
only, for a dispatcher to make an incident from or attach to one. Nothing is dispatched on its own.

Every sender uses the hook token from Admin › Integrations (`Authorization: Bearer <token>`).

| Hook | For |
|---|---|
| `POST /api/hooks/alarm` | an alarm company's alarm (burglary, fire, medical, panic, holdup, CO…) |
| `POST /api/hooks/lpr` | a plate reader's hotlist hit |
| `POST /api/hooks/gunshot` | a gunshot detection |
| `POST /api/hooks/alert` | any of these, with `kind` in the body (`alarm`, `lpr`, `gunshot`, `camera`, `other`) |

## The body (JSON)

Required: `sender` (the company or system) and `id` (its event ID). The same sender and ID again is an
update to the same call.

```json
{
  "sender": "Acme Monitoring", "id": "EV-1001", "event": "burglary",
  "premise": "Main Street Pharmacy", "zone": "3 rear door", "account": "A-77", "permit": "P-2026-14",
  "verified": false, "callback": "+18005550100", "operator": "Op 12", "keyholder": "J. Doe +15405550111",
  "address": "12 Main St, Roanoke VA", "lat": 37.27, "lon": -79.94, "radius": 10,
  "detail": "Rear door contact, then interior motion"
}
```

Plate readers add `plate`, `state`, `vehicle`, `hotlist`, `camera`, `heading` and `image` (an `https://`
link to the picture at the sender, opened by the dispatcher; other links are dropped). Gunshot detection
adds `rounds` and `confidence`.

**Cancelling**: send the same `sender` and `id` with `"cancel": true` (and why in `detail`). If the alarm is
already on an incident, a priority note goes on it: *ALARM CANCELLED by …*.

The answer: `{"call": "...", "status": "new|updated|cancelled", "incident": "26-000123", "suggestedType": "ALARM"}`.
The suggested type is the agency's own call type for that kind of alert, when one with a usual code exists.

## Answers to the alarm company

With an address set in Admin › Integrations, bitdispatcher posts what became of each alarm, once per
status, with the answer token as Bearer:

```json
{ "sender": "Acme Monitoring", "id": "EV-1001", "status": "dispatched", "incident": "26-000123",
  "units": ["2A11"], "disposition": "", "at": "2026-10-20T23:05:00Z" }
```

`status` is `accepted` (an incident was made), `dispatched`, `onscene`, `closed` (with the disposition), or
`ended` (the alert left the queue without an incident).

## ASAP

ASAP (APCO/CSAA ANS 2.101, the Automated Secure Alarm Protocol) is the national standard for alarm
companies to send alarms to a 911 centre's CAD, over Nlets through the state's message switch. Its schema
is sold by APCO and its message path needs the state's switch and Nlets. bitdispatcher does not speak ASAP
yet: this hook carries the same information over HTTPS, and an ASAP adapter can map onto it once the
schema is in hand.

## Plate readers and gunshot detection

Flock Safety and SoundThinking (ShotSpotter) publish their alert APIs. An adapter that subscribes to a
vendor's alerts and posts them here is the connection; none is included yet, and none has been tested
against a vendor's live system.
