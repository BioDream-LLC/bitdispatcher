# bitdispatcher and BitVoice

Status: draft, 2026-10-18. Companion to [DESIGN.md](DESIGN.md).

bitdispatcher works with any standards-compliant call-handling system. When the call-handling system is
BitVoice, bitdispatcher uses every hook BitVoice offers a CAD. Every one of them is a published standard
(ALI spill, NENA EIDO, NENA i3) or BitVoice's public REST API and event stream; nothing is private, and
bitdispatcher never needs BitVoice to run.

## 1. What BitVoice sends to bitdispatcher

| Hook | Transport | bitdispatcher endpoint | Auth | Notes |
|---|---|---|---|---|
| ALI spill | TCP (or serial via a gateway), STX/ETX framed, 32-column lines | ALI listener, address set in Admin › Integrations | none (legacy) | Sent once at answer. Parsed into a call; no retry on BitVoice's side |
| EIDO | HTTPS POST, `application/emergency.eido+json`, NENA-STA-024 v1.0 | `POST /api/ng911/eido` | `Authorization: Bearer <token>` (BitVoice `psap.eidoToken`) | Sent at answer and again whenever caller media changes. No retry |
| Radio events | HTTPS POST, JSON `{type, agency, data}` | `POST /api/radio/events` | `Authorization: Bearer <secret>` (BitVoice `radio.cadSecret`) | `radio.unit`, `radio.emergency` (3 tries), `radio.emergency.ack`, `radio.transmission`, `radio.location` |

BitVoice's HTTP client verifies TLS certificates, so the bitdispatcher address it posts to must carry a
certificate BitVoice trusts. Where that is not yet possible, bitdispatcher still gets every call and radio
event through the pull path (section 2), which BitVoice cannot fail to deliver while the link is up.

## 2. What bitdispatcher pulls from BitVoice

bitdispatcher signs in to BitVoice with an API token (`Authorization: Bearer bv_...`, viewer role is
enough to read; acknowledging radio emergencies needs admin).

- **Event stream** `GET /api/events` (server-sent events). bitdispatcher uses:
  - `call.started`, `call.updated`, `call.ended`: the full call snapshot, including `emergency` (location,
    Additional Data, text, video, ALI class) for 911 calls. Ringing 911 calls appear in the dispatcher's
    queue before a call-taker answers.
  - `emergency.answered`: then bitdispatcher fetches `GET /api/calls/{id}/eido` so the EIDO arrives even
    when BitVoice's push did not.
  - `emergency.cad_failed`, `radio.cad_failed`: BitVoice could not deliver to bitdispatcher. Shown to
    the dispatcher and the administrator at once, with the reason.
  - `radio.*`: the same radio events as the webhook, so a missed webhook is filled from the stream.
  - `call.media`, `call.video`: caller photos, clips and live video.
  - `emergency.mode`: BitVoice's emergency mode, shown in the status bar.
  - `resync`: bitdispatcher reloads `GET /api/calls` and `GET /api/radio/log?since=`.
- **Calls in progress** `GET /api/calls`, at start and after every reconnect or resync.
- **Radio log catch-up** `GET /api/radio/log?since=...` after a gap.
- **Radio positions** `GET /api/radio/locations` at start.

Every BitVoice payload is deduplicated by call ID or event identity, so the push and the pull paths can
both run without double entries.

## 3. What a dispatcher can do in BitVoice from bitdispatcher

| Action | BitVoice call |
|---|---|
| Rebid the caller's location | `POST /api/calls/{id}/rebid` |
| Text the caller (TTY, RTT, MSRP, SIP) | `POST /api/calls/{id}/text` `{via, text}` |
| Send the caller a video or photo link | `POST /api/calls/{id}/video/link` |
| Watch the caller's live video (blurred until revealed) | WHEP view link from the call's video |
| Acknowledge a radio emergency | `POST /api/radio/channels/{id}/ack` |
| Refresh the EIDO | `GET /api/calls/{id}/eido` |
| Answer-time report beside CAD reports | `GET /api/psap/report` |

Every action is an event in bitdispatcher's ledger (who, when, which call, result), and a failed action
says why on screen.

## 4. Mapping

- A 911 call becomes a **call** in bitdispatcher, keyed by BitVoice's call ID. The dispatcher creates an
  incident from it, or attaches it to an existing incident. Nothing creates an incident on its own
  (DESIGN.md rule 5).
- Caller location keeps its source (EIDO PIDF-LO, ALI, rebid, SSE snapshot), uncertainty radius,
  confidence, method and time. A location older than the configured limit is shown as stale, never as
  current.
- Radio unit IDs and aliases map to bitdispatcher units through each unit's radio ID (Admin › Units).
  An emergency button from a radio not mapped to a unit is still shown, as an unknown radio.
- `radio.location` updates a unit's AVL position with source `radio`.
- `radio.transmission` sets the unit's last-heard time, which feeds the status-timer and loss-of-contact
  checks.

## 5. Health

The status bar shows each hook's state and age of its last message: event stream connected or not, last
EIDO, last ALI spill, last radio event, and delivery failures BitVoice reported. Admin › Integrations
shows the full detail and a test for each hook.
