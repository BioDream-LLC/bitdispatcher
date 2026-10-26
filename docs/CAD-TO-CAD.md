# CAD-to-CAD: incidents with neighbouring agencies

bitdispatcher sends an incident to a neighbouring agency's CAD, and takes the incidents a neighbour
sends, as an **EIDO** (Emergency Incident Data Object, NENA-STA-024, JSON). Each neighbour is set up in
Admin › Integrations › Neighbouring agencies with an ID, a name, the address its CAD takes EIDOs at, and
a pair of tokens.

## Sending

On an open incident, **Send to a neighbouring agency…** (dispatchers) asks which agency and why, and posts:

```
POST <their address>
Content-Type: application/emergency.eido+json
Authorization: Bearer <the token we send>
```

The EIDO carries an incident component (our incident number, the type in words, the status, the
priority), a call component, the callback, the location by value (PIDF-LO, with the point, its radius
and the civic address when known), and notes: why it was sent, our units on it, and the incident's notes.
The incident's timeline says it was sent, or why it could not be.

## Receiving

A neighbour posts its EIDO to `POST /api/cad/eido/<their ID>` with `Authorization: Bearer <the token
they send>`. It arrives in the 911 calls list as a call, data only, marked as from that agency, with
their incident number, type, priority, location and notes, for a dispatcher to make an incident from or
attach. The same incident sent again updates the same call. A wrong token or an unknown agency is refused.

## What is not done yet

- Updates do not flow back on their own; each send is a press. Closing, unit status and comments are not
  exchanged continuously. Between two centres that both run bitdispatcher, overflow (docs/CONTINUITY.md)
  does exchange them, for the calls one centre takes for the other.
- bitdispatcher's EIDO has been checked against its own reader and the EIDOs BitVoice writes; it has not
  been tested against another vendor's CAD.
