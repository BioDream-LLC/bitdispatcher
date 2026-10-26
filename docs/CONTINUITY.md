# Continuity: outages, the backup site, overflow and map updates

How bitdispatcher keeps a centre working when the internet, the server or the building is lost, and how two
centres that both run bitdispatcher take each other's calls. Everything here is set in the web interface
(Admin › Continuity, Admin › Map, Admin › Sites, Admin › Integrations).

## The design, and why

NENA's continuity-of-operations guidance and the state COOP guides built on it ask a centre to plan for three
things: losing a link, losing its systems, and losing its building. Vendors answer the last two in different
ways: a backup centre, a cloud service the centre moves to, or a neighbouring centre taking the calls (for
example CAD-to-CAD between neighbouring counties in a storm). Industry writing on cloud CAD stresses *local
survivability*: a centre whose CAD lives only in the cloud stops when its internet does. bitdispatcher takes
all three, in this order:

1. **Local first.** bitdispatcher runs on the centre's own server, with its own recorded street map, routing
   and ledger, so losing the internet does not stop dispatch. Consoles also keep the last board and an
   offline log if the server itself cannot be reached (manual: *Working through an outage*).
2. **A second site for the building.** The backup is another bitdispatcher site (active-active, Admin ›
   Sites) in a building the county owns or in the cloud. It already holds every event, so staff who move there
   carry on where they were. A supervisor declares the move, which every console shows, and works a checklist.
3. **A neighbour for the load.** When the centre is overwhelmed, or has no backup site yet, a neighbouring
   centre running bitdispatcher takes calls for it ("overflow"). Each call becomes an incident in the asking
   centre's CAD; the neighbour sees only the calls it took.

The 911 calls themselves move only when the phone network moves them: alternate routing set up by the 911
authority, or BitVoice's ESRP policy in NG911 (tested in the lab: PSAP A's queue Full sent the next call to
PSAP B, docs/BITVOICE.md). A CAD cannot pull calls from another centre's lines, and bitdispatcher does not try.

## Links and outages

Admin › Continuity › *Internet checks*: up to 10 https addresses, checked every 30 seconds (10 to 600). Any
one answering means the internet works; any HTTP answer counts. BitVoice, the other sites and each overflow
neighbour are checked as well. Each change of a link is a `continuity.link` event; the Continuity page lists
the links and outages. When the internet goes or comes back the supervisors get a message, and every console
shows a red *Internet down* chip (another link down is amber).

With the internet down, these keep working: call entry, dispatch, unit status, the recorded map, routing and
addresses, the ledger, printing. These wait: online map tiles, pictures not recorded, traffic, map updates,
neighbours and overflow, NERIS, PSC PIP, and BitVoice or paging where they are reached over the internet.

## Moving to the backup site

`POST /api/continuity/relocation {active, note}` (supervisors) records `continuity.relocation`; every site
shows *At backup site*, and everyone on duty gets a message. The checklist (Admin › Continuity) is ticked
step by step, each tick a `continuity.step` event.

## The continuity pack

`GET /api/continuity/pack` is a printable HTML page: the open incidents (with callers' names and callbacks;
Social Security and licence numbers masked as on the board), every unit and its status, the checklist, the
contacts, the neighbours and blank log lines. Every opening is recorded. It can be written every 1 to 240
minutes to the data directory and up to five folders (a shared drive, a USB disk), with mode 0600 and an
atomic rename. It holds personal data: keep printed and copied packs as records.

## Scheduled map updates

Admin › Map › *Automatic updates*: off, daily or weekly at an hour, in the agency's time zone. A run applies
OpenStreetMap's daily change files (Geofabrik) and builds a new store beside the one in use. The extract is
updated beside the old one, which is kept as `.prev` until the new map is built; a failure puts it back. The
new map replaces the old only if it is complete and has not lost more than 10% (configurable, 1 to 90) of
its road segments or addresses. A run that fails, has no internet, or finds a recording under way leaves the
map in use untouched; the next window tries again. Runs are `map.checked`, `map.recorded` and `map.failed`
events; a failed scheduled run messages supervisors and administrators. Tests: a stand-in change-file
server for success, a corrupt file, no internet and lost roads, and a second run refused while one runs.

## Overflow

### Agreeing it

Both centres add each other in Admin › Integrations › Neighbouring agencies: the other CAD's address
`https://their-cad/api/cad/eido/<the ID they know us by>`, a token each way, and, if its certificate is its
own rather than from an authority, its SHA-256 fingerprint to pin it. Then in Admin › Continuity, per
neighbour:

| Setting | Means |
|---|---|
| We may ask them to take our calls | Our supervisors see *Ask … to take our calls* |
| They may assign our units | While they take our calls, they may assign our shared units |
| We take their calls when they ask | Their requests reach us |
| Accept their requests on our own | No supervisor needed to accept (an agreement that says yes in advance) |
| Our agencies | Whose calls they take, and whose units are shared (empty: all) |
| Call types | Which of our call types they may use (empty: all) |

An overflow lasts 240 minutes unless the asking supervisor chooses otherwise (Admin › Continuity sets the
default) or either side ends it.

### What crosses

Only the calls the helper takes, and what becomes of each: the asking centre's status, units, notes and
close. Nothing else: no other incidents, no records, no person history, no NCIC returns. Every request,
answer, call, note and assignment is in both ledgers (`overflow.session`, `overflow.call`,
`overflow.incident`, and the asking centre's own incident events, under the helper's call taker as
`<THEIR-ID>:<username>`).

### The exchange

Each centre serves `/api/overflow-link/<partner ID>/…`, authenticated by that partner's token
(`Authorization: Bearer`), refused with 401 for an unknown partner or wrong token and 403 when overflow is
not set up with it.

| Call | From | Does |
|---|---|---|
| `GET ping` | either | Link check |
| `POST session {direction, status, note, until, dispatch, agencies, callTypes, by}` | either | Ask (`out`, `asked`), answer (`in`, `active` or `declined`), end (`ended`). The answer says the state at the receiver |
| `POST calls {ref, type, priority, address, lat, lon, callerName, callback, narrative, by}` | helper | Creates the incident at the asking centre; the same `ref` again returns the same incident |
| `POST calls/<ref>/notes {text, by}` | helper | A note on that incident |
| `GET units`, `POST calls/<ref>/dispatch {units, by}` | helper | Only when the asking centre lets the helper assign its units |
| `POST updates {ref, number, status, units, closed, disposition, notes}` | asking centre | What became of the call, at most every 5 seconds per incident, retried until delivered |

A call the asking centre cannot be reached for is kept at the helper (`delivered: false` with the reason) and
sent every 20 seconds until it is; the call taker is told to pass urgent calls by radio or phone meanwhile. A
call taken while the overflow was live and delivered after it ended is still accepted, so none is lost.

### What is tested

The Go suite runs two centres over HTTPS (`TestOverflowBetweenTwoCentres`): a request refused before the
helper agreed and by a dispatcher, accepted, a call becoming the asking centre's incident with its caller and
callback, a repeat not making a second incident, a note, one of the asking centre's units assigned by the
helper, the status, units and close coming back, a call kept while the asking centre refused it and delivered
after, and no calls after the end. The browser suite runs the same path through the screens with the test
server as its own neighbour, over HTTPS with its certificate pinned. Not yet tested between two centres on
different networks, or with a real 911 authority's alternate routing.
