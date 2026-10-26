---
title: Setting up a 911 CAD yourself, with remote support
tile: Self-service setup
summary: A setup checklist, a go-live check and a support bundle, so an agency can set bitdispatcher up itself and get help remotely.
description: How an agency sets up bitdispatcher itself: a setup checklist, a go-live check, and remote help through a support bundle with no secrets or caller data.
icon: check
---

## What it is

Buying a CAD system usually means buying the setup too. The vendor sends people to load the map, enter the stations and units, write the run cards, connect the phones and stand behind the dispatchers on the night of the switch. That work is real, and it is often the most expensive line in the quote.

bitdispatcher is free, and nothing about it requires anyone to come on site. Everything is configured on its admin screens. Three things make it practical for an agency to do that setup itself and to get help from a distance:

- a **setup checklist** that shows what is left before the first live call,
- a **go-live check** that tests the result automatically, and
- a **support bundle** that gives whoever helps the agency what they need to find a problem, without secrets or caller data and without anyone seeing the agency's screen.

## How it works

**The setup checklist** (Admin › Setup checklist) lists every step in order: the agency's name, the recorded road map, the county's GIS, stations, units, call types and run cards, the people and their authenticators, removing the demonstration data, the phone system, station alerting and paging, NCIC, a second site, nightly backups, training, running beside the old system, the go-live check, and who supports the agency. Each step is marked required, recommended or optional, says why it matters, and links to the page that does it and to its chapter of the manual.

bitdispatcher checks most steps itself as the administrator works. A step is done when its page has what it needs, and the step says what it found: *6 stations; not on the map: Station 4*, or *administrators without an authenticator: admin*. Only a person can know that training was given, that backups are being copied off the server, or that the parallel run is finished, so those steps have **Mark done**, with an optional note. A recommended or optional step the agency does without can be marked **Not needed**, with the reason. A step bitdispatcher checks itself cannot be marked done by hand. Every mark is recorded in the ledger with who made it.

**The go-live check** (Admin › Go-live check) runs automatic tests in a few seconds:

- the ledger's hash chain, every event from the first;
- a **restore**: the ledger is copied, replayed on its own, and compared with the running system;
- room on the disk;
- no demonstration data left;
- every administrator has an authenticator;
- FIPS 140-3 mode, with the Go Cryptographic Module validated under certificate #5247;
- a trusted certificate rather than the server's own;
- the recorded map covers every station;
- **test calls**: every call type with a run card, at every station that has units. Each is worked out and thrown away, so nothing is written to the ledger and no unit is touched. The check warns when drive times are straight lines, when another station's unit comes before the station's own, and when a run card asks for units the agency does not have available;
- the phone system is connected, a page was delivered in the last 30 days, and the second site copied in the last 2 minutes.

Each test passes, has something to look at, or fails, with a **Fix** link to the page that fixes it. The check never pages, texts or queries anyone. Its result is recorded in the ledger, and the checklist counts it only when nothing failed in the last week.

**The support bundle** (Admin › Support) is one zip file: the version, operating system and uptime, the ledger's size and check, disk space, the checklist and the last go-live check, the state of the links to the phone system and the other sites, counts of what is configured and the kinds of events in the last seven days, the settings, stations, units, call types and response plans, the connections, and the server's recent warnings and errors.

## Why it matters in a 911 center

Most 911 centers are small. A center with four positions rarely has a project team, and the setup fee in a commercial quote can be more than a year of its software budget. When setup can be done by the agency's own administrator, with a checklist that says what is left and a check that says whether it worked, the cost of trying a CAD system drops to the time it takes.

Remote support only works if the person helping can see what is wrong. Screen sharing into a 911 system is a security problem in itself, and copying logs by hand tends to leak exactly what should not leave the building: callers' numbers, addresses, tokens in URLs. A bundle built for the purpose, with the dangerous parts removed by design, lets an agency ask for help without that risk.

## Where CAD systems usually fall short

Setup knowledge usually lives with the vendor's implementation team, in spreadsheets and statements of work rather than in the product. The agency cannot easily tell how far along it is, or what the vendor did on its behalf. Go-live readiness is a meeting, not a test. And diagnostics are often either a remote-access session or a log export that nobody has checked for caller data.

## How bitdispatcher does it

The checklist is worked out from the live configuration every time it is opened, so it cannot fall out of date. The only things stored are what people confirmed, and those are ledger events like any other change.

The go-live check uses the same recommendation engine dispatchers use, on a test call that is never created. It replays a copy of the ledger with the same code that starts the server, so a pass means the ledger really does rebuild the system. It is honest about its limits: it says plainly that it cannot see backup copies kept elsewhere, the phone system's own wiring or the radio, and the checklist asks a person to confirm those.

The support bundle is built on the server and downloaded by an administrator. **bitdispatcher sends nothing to BioDream or anyone else by itself.** In the bundle:

- every value under a name like token, secret, password or key becomes *(set)*;
- e-mail addresses, phone numbers, query strings and passwords in web addresses, and anything shaped like a token are removed from every file;
- calls, incidents, transcripts, recordings, caller names, numbers and locations, premise notes and usernames are never put in; users appear only as counts by role.

What an agency typed into its own settings or call types (a name on a call type's notify list, say) stays in, and the page says so: read the bundle before sending it. Each download is recorded in the ledger with who made it.

Support itself is remote. The Support page holds the agency's own text for who to call: its IT help desk, a support agreement, or, by default, community support on GitHub. Work that needs hands, like cabling, the phone system's configuration and the radio, belongs to the agency's own staff and its existing vendors, who connect to bitdispatcher's documented interfaces.

## Why bitdispatcher fits

bitdispatcher was built so that everything is configured in the GUI, with no configuration files to edit and no vendor tools. That is what makes self-service possible at all. The checklist, the check and the bundle add the missing pieces: knowing what is left, knowing it works, and getting help safely.

## What has been tested, and what has not

Go tests build a system on a small street grid and check that the checklist follows the configuration; that steps bitdispatcher checks cannot be ticked by hand and that "not needed" needs a reason; that the go-live check passes the ledger and restore, fails a missing authenticator, and writes exactly one event (its own result) with no test incident left behind; and that the support bundle contains none of a set of planted secrets, a planted e-mail address, phone number, token in a URL or a second administrator's name. Browser tests tick and untick a step, run the go-live check against the demonstration system (where the demo data correctly fails it), save a support contact and download a bundle.

What has **not** been tested is a real agency setting bitdispatcher up by itself from start to finish. No agency has done that yet, and no agency runs bitdispatcher on a live floor. bitdispatcher is **not certified**. The redaction removes the patterns it knows; it is not a guarantee, which is why the bundle is never sent automatically and the page asks the administrator to read it first.

## Try it

These are administrator pages, and the public [demonstration](https://demo.bitdispatcher.com) signs people in as dispatchers and supervisors, not administrators. To try them, run bitdispatcher on your own computer with `-demo` (see [Get it](/get/)), sign in as the administrator, and open **Admin › Setup checklist**, **Admin › Go-live check** (the demo's own data fails one test, as it should) and **Admin › Support** to download a bundle and look inside. The manual chapters are [Self-service setup](/manual/#self-service), [Setup checklist](/manual/#admin-setup), [Go-live check](/manual/#admin-golive) and [Support and the support bundle](/manual/#admin-support). The source is on [GitHub](https://github.com/BioDream-LLC/bitdispatcher).

Related: [Migration from an earlier CAD](/tech/migration/), [The event ledger](/tech/event-ledger/), [Two sites, both live](/tech/active-active-sites/), [Security](/tech/security/)
