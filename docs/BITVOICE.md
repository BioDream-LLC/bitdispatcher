# bitdispatcher and BitVoice

Status: tested against a live BitVoice on 2026-10-21 (see section 6). Companion to [DESIGN.md](DESIGN.md).

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

BitVoice's HTTP client verifies TLS certificates, so an https address it posts to must carry a certificate
BitVoice trusts. When both run on the same computer, `bitdispatcher -hooks-addr 127.0.0.1:9450` also takes
the two pushes over plain HTTP on loopback only (it refuses any other address, and serves nothing but
`/api/ng911/eido` and `/api/radio/events`, each still needing its token): set BitVoice's `psap.eidoUrl` to
`http://127.0.0.1:9450/api/ng911/eido` and `radio.cadUrl` to `http://127.0.0.1:9450/api/radio/events`.
Where neither is possible, bitdispatcher still gets every call and radio event through the pull path
(section 2), which BitVoice cannot fail to deliver while the link is up.

## 2. What bitdispatcher pulls from BitVoice

bitdispatcher signs in to BitVoice with an API token (`Authorization: Bearer bv_...`, viewer role is
enough to read; acknowledging radio emergencies needs admin). Each request a person causes carries
`X-BitVoice-On-Behalf-Of: Name (username)`; BitVoice honours it from a token made with "Names the person
it acts for", so its exposure log, text calls taken and audit log name the dispatcher.

- **Event stream** `GET /api/events` (server-sent events). bitdispatcher uses:
  - `call.transcript`: live lines while a 911 call is answered (BitVoice Settings › AI, live transcripts).
  - `call.ended` carries `cause` and `endedBy` (BitVoice ends a call whose caller's audio stopped);
    bitdispatcher shows the cause on the call and its silent-line advice says the line went silent.
  - `call.started`, `call.updated`, `call.ended`: the full call snapshot, including `emergency` (location,
    Additional Data, text, video, ALI class) for 911 calls. Ringing 911 calls appear in the dispatcher's
    queue before a call-taker answers.
  - `emergency.answered`: then bitdispatcher fetches `GET /api/calls/{id}/eido` so the EIDO arrives even
    when BitVoice's push did not.
  - `call.media`: a caller sent a photo or clip; bitdispatcher reads `GET /api/calls/{id}/media` (hash,
    the camera's time and GPS).
  - `transcript.done`, `call.briefing`, `call.recorded`: the recording, its transcript and its summary.
  - `esrp.route`: how the ESInet's routing chose a PSAP for the call. `core.states`: bitdispatcher reads
    `GET /api/core/states` (each PSAP's NENA queue and service state).
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
| Take a text-to-911 call waiting on BitVoice's 911 screen | `POST /api/calls/{id}/take` |
| Reveal a caller's photo or clip (each reveal in bitdispatcher's ledger, and in BitVoice's exposure record) | `GET /api/calls/{id}/media/{n}` |
| Hold the caller's photos and clips as evidence | `POST /api/calls/{id}/media/hold` |
| Unblur the caller's live video (recorded in BitVoice's exposure record) | `POST /api/calls/{id}/video/reveal` |
| Check the caller's civic address and position (LVF) | `POST /api/core/validate` |
| Answer-time report, exposure record, i3 log events, beside CAD reports | `GET /api/psap/report`, `/api/psap/exposure`, `/api/core/i3log` |

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

## 6. Tested live (2026-10-21)

Against BitVoice built from main (fc96b19) on another computer on the LAN, with a lab ESInet trunk, a
registered call-taker phone, a P25 stand-in, and `cmd/i3caller` placing NG9-1-1 calls (SIP with PIDF-LO,
RFC 7852 Additional Data, verstat, G.711 voice or MSRP text, location by reference). bitdispatcher pulls
everything; BitVoice did not push (it needs to reach and trust bitdispatcher: see section 1).

| What | Result |
|---|---|
| A ringing 911 call in the queue before answer; caller number, name, provider, class, GPS location; EIDO pulled at answer | Works |
| The caller ID's STIR/SHAKEN check (verified, attestation) on the call and the queue | Works |
| Rebid by reference (HELD), BitVoice's check of the new location against the old | Works |
| LVF: address checked against the address data, typos with suggested corrections, position against the address point | Works (with lab address data loaded in BitVoice) |
| Caller video over WHEP: link sent, caller streams from a browser, dispatcher watches blurred and reveals | Works (a fake camera in Chromium) |
| Caller photo: SHA-256, the camera's time and GPS, shown as facts until revealed, each reveal logged; held as evidence | Works; hash matches |
| Text to 911 over MSRP: arrives, taken from bitdispatcher, two-way | Works |
| P25 radio over the ISSI: a radio's emergency (private call), its alias, mapped to the unit; acknowledged from bitdispatcher and cleared on BitVoice's console | Works (a P25 stand-in, no voice) |
| Radio unit positions on the map | Works |
| Answer-time report, exposure record, i3 log in Reports | Works |
| Silent-line and machine-against-caller advice | bitdispatcher's own advisory panel (unit tested), advice only |
| Real-time text, TTY, SIP MESSAGE, CAMA, ALI lookup by callback, ESRP routing and PSAP states | Built to BitVoice's payloads and unit tested; not exercised live (needs a TTY, a CAMA trunk, an ALI link or an ESRP policy) |
| BitVoice pushing EIDO, ALI spill and radio events to bitdispatcher | Not tested here: bitdispatcher ran where BitVoice could not reach it (see the retest below) |

Retested against BitVoice a83099c (its fixes for what this testing found):

| What | Result |
|---|---|
| A 911 route set to hang up: Health warns, the call rings every call-taker and is answered, the routing trail says why | Works |
| A caller who vanishes without a hang-up: BitVoice ends the call after 60 s of silence, bitdispatcher shows "the caller's audio stopped" and raises the silent-line advice | Works |
| The dispatcher named in BitVoice's exposure log, text calls taken and audit log (token that names the person) | Works |
| The video link says why it was not texted | Works |
| BitVoice pushing to bitdispatcher on the same computer (`-hooks-addr`): the EIDO and the ALI spill at answer, a P25 radio emergency by webhook | Works: the call came by all three paths (event stream, ALI, EIDO), the emergency as M1 "Medic 1" |
| Live transcripts (`call.transcript`): a caller speaking (a recorded sentence, `i3caller -wav`), written out by BitVoice's local speech-to-text and shown on the call as the caller's words | Works: the sentence arrived word for word while the call was in progress |
| The recording, its transcript and its AI summary after the call (`call.recorded`, `transcript.done`; BitVoice recording all calls, speech-to-text on its own computer, the summary by a hosted language model) | Works: a few seconds after the caller hung up, the call carried its recording and a summary marked "BitVoice (AI)" |
| A briefing when a call is transferred (`call.briefing`): the lab phone (`i3caller -phone 100 -transfer 101`) passes an answered call on | Works: about ten seconds after the transfer, while the call was still going, the call carried the briefing, marked "BitVoice briefing (AI)". After the call, the fuller summary takes its place |

Retested against BitVoice 29f2ae7 (its rule against plain http off this computer): the EIDO, ALI spill and
radio pushes to `http://127.0.0.1:9450` still arrive; saving an `http://` EIDO or radio address to another
computer is refused (422 naming `system.psap.eidoUrl` or `system.radio.cadUrl`); an ALI spill address on
another computer saves with a Health warning that it crosses the network unencrypted.
