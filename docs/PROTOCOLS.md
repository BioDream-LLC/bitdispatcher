# Call-taking protocols

## The agency's own protocols

Admin › Call-taking protocols holds the agency's question sets. Each has questions; each answer leads to
the next question or ends at a **determinant**: a code, what it means, optionally a priority and a call
type it sets, and the instructions for the caller. A protocol is checked when saved (every answer goes
somewhere that exists, no question loops) and is offered on calls only when marked **approved**, for the
call types it lists (none listed: all).

On the call-taker's Entry tab the Protocol panel asks one question at a time once the call is submitted;
answers are buttons or number keys 1–9. Every answer is on the incident's timeline with who gave it and
when, and the determinant shows on the dispatcher's incident panel.

bitdispatcher ships one example ("EXAMPLE: breathing problem (not for use)"), only to show the shape. It
is not a medical protocol. An agency writes its own under its medical director, or licenses one.

## Licensed protocol software (ProQA and others)

Priority Dispatch's ProQA and similar products are licensed, and their interfaces are given only to
licensed vendors; bitdispatcher does not include or copy any of their content, and has not been tested
with them. What it offers is a place for the determinant to land, which an interface or a small bridge
calls:

```
POST /api/hooks/determinant
Authorization: Bearer <hook token>

{"incident": "26-000123",      // or "call": "<call id>" for the call the incident came from
 "system": "ProQA",
 "code": "10-D-1", "text": "Chest pain, not alert",
 "priority": 1,                  // optional, 1–5
 "callType": "CARD",             // optional; ignored if the agency has no such call type
 "instructions": ["…"]}          // optional
```

The determinant goes on the incident's timeline marked with the system, and sets the priority and call
type when given.
