---
title: Alarms that arrive checked, by company, permit and false-alarm history
tile: Alarms and permits
summary: Each alarm company gets its own address and token; alarms arrive matched to a permit and the address's false-alarm count.
description: How bitdispatcher takes alarms per company over HTTPS, checks the permit register and false-alarm history, and lets a call-taker type a phoned alarm.
icon: bell
---

## What it is

Alarm monitoring companies send a lot of calls to 911 centers: burglary, fire, medical, panic, holdup, carbon monoxide. Most never become an emergency, and many local governments charge for a permit to have an alarm and fine the address that cries wolf too often. So the question on an alarm is not only "what is it?" but "who is this company, does this address hold a current permit, and how many false alarms has it already had this year?"

In bitdispatcher, alarms already arrive as **data-only calls** over HTTPS: a dispatcher sees them in the queue and decides whether to make an incident, and nothing is dispatched on its own. What this adds is identity and context. Each alarm company gets **its own address and token**, so its alarms arrive stamped with its name. Every alarm is matched against the agency's **permit register** and the address's recent **false-alarm history**, and the answer is written on the call before a dispatcher reads it. A call-taker on the phone with an alarm operator can type the same alarm by hand and it is checked the same way.

## How it works

An alarm is a small piece of JSON posted to bitdispatcher. It becomes a call marked data only, and bitdispatcher sends the company back what happened to it: **accepted** when an incident is made, then **dispatched**, **on scene**, and **closed** with the disposition. That much was already in place. The new parts are these.

**A company's own address and token.** Each company the agency works with gets its own hook, `/api/hooks/alarm/<company>`, and its own bearer token, shown once when it is created. An alarm that arrives on that address is stamped with the company's name, and the answers about that alarm go only to that company's own answer address. A company that is switched off, or that presents the wrong token, is refused. So one center can take alarms from several companies at once without their traffic or their answers getting crossed.

**The permit register.** An administrator loads the agency's alarm permits as a **CSV**: number, address, holder, company, expires (as YYYY-MM-DD), and status (active, suspended or revoked). When an alarm arrives, bitdispatcher matches it first by **permit number** if the alarm carries one, and otherwise by **street address**, normalizing the address so that "Street" and "St" or "North" and "N" match. The note it writes on the call is one of:

- **valid**, with "until" and the date when the permit has an expiry,
- **EXPIRED**, with the date, when the expiry has passed,
- **SUSPENDED** (or whatever non-active status the register holds), or
- **none on the register** for this address, when the register is loaded but has no match.

If no register has been loaded at all, no permit note is written, so the feature stays quiet until an agency opts into it.

**False-alarm history.** bitdispatcher counts the address's own incidents in the **last year** whose disposition marks a false alarm. By default that means a disposition containing the word "false", and an agency can set its own words. At or above the limit, **3 by default**, the alarm is stamped **REPEAT FALSE ALARMS**, with the count and the agency's limit; below the limit but above zero it simply notes how many there have been. Many local ordinances fine an address, or stop responding to its alarms, after a set number of false alarms in a year, so putting the count in front of the dispatcher turns a policy that usually lives in a filing cabinet into something visible at the moment of the call.

**A phoned alarm.** Not every alarm company sends data; some still call a person. A call-taker with an alarm operator on the phone types the alarm on an **alarm card**: company, operator, callback, event number, alarm type, zone, premise, address, permit, account, keyholder, and whether the operator says it is verified. The typed alarm goes into the queue exactly like a sent one, gets the **same permit and false-alarm checks**, and records **who typed it**. The one difference is that no answers are sent back to a phoned alarm, because there is no machine on the other end: the company was told by phone, as it told the center by phone.

## Why it matters in a 911 center

Alarms are high in volume and mostly low in risk, which is a dangerous combination: it is easy for a center to treat them as routine and easy for a genuine one to be lost in the noise. Good handling is not about speed alone but about giving the dispatcher the context to weigh each one without leaving the screen.

A current permit, or the lack of one, is part of that context. So is a history of false alarms, because an address on its fifth false burglary alarm of the year is a different decision from a first-time call, both for how fast to commit scarce units and for the paperwork and billing that follow. When the permit status and the year's false-alarm count are already on the call, the dispatcher does not have to look them up, and the record of what was known at dispatch time is kept with the call.

Per-company identity matters for trust and for answers. When an alarm arrives stamped with the company that sent it, a dispatcher knows who to call back, and the company gets told what happened to its alarm without the center picking up the phone. Letting a call-taker type a phoned alarm onto the same card, with the same checks, means the busy mix of data alarms and phoned alarms all end up looking and behaving the same in the queue.

## Where CAD systems usually fall short

Alarm handling is often a bolt-on. Permit checking, where it exists, may be a separate records system a dispatcher has to search by hand, or a billing product with no link to the live call. False-alarm counts tend to surface only after the fact, in a monthly report, rather than on the call where they could change a decision.

Company identity is frequently coarse: alarms arrive through one shared channel, so the center cannot easily tell one company's traffic from another's or send tailored answers back. And phoned alarms usually live in a different place from data alarms, typed into a free-text call with none of the same structure or checks, so the center's picture of its alarm load is split across two habits.

The automated edge some vendors now sell is answering the alarm company's phone call with **AI**: a system that picks up the company's ten-digit line and takes the alarm by voice. bitdispatcher does not do that. It does not answer phones at all; call handling, such as BitVoice, is where voice lives. bitdispatcher's part is to take the alarm as data, or from a call-taker who answered, and check it.

## How bitdispatcher does it

All of this is set up in the GUI under **Admin › Alarms & permits**: add a company and get its address and token, load the permit CSV, and set the false-alarm words and limit. The alarm card for phoned alarms is on the dispatcher's side. Everything is written to the ledger: the alarm as a call, the checks as notes on it, the company configuration as an event, and who typed a phoned one.

The checks are built to fail safe and quiet. No permit note appears until a register is loaded. The false-alarm count uses the agency's own disposition words. A company must present its own token on its own address or be refused. The answers go out once per status, to the company that sent the alarm, and never to a phoned one.

To be honest about the standard: the national way for alarm companies to send alarms into a CAD is **ASAP**, the **APCO/TMA** Alarm Monitoring Company to PSAP interface, carried over **Nlets** through a state's message switch. bitdispatcher does not speak ASAP yet. Doing so needs the standard's schema and a connection to the state's message switch, neither of which is wired up. What bitdispatcher offers is the same exchange of alarm and answer over HTTPS, which an ASAP adapter could later map onto once the schema is in hand.

## Why bitdispatcher fits

Alarm handling with permit and false-alarm context is the kind of thing a small center usually cannot afford as a separate product, so it either goes without or chases the information by hand. Having it inside a free, open CAD, set up in the GUI and recorded in the ledger, puts the same context in front of a two-person center that a large one pays for.

It also fits bitdispatcher's rule that the system advises and a person decides. An alarm, however it arrives and however it checks out, is only ever a data-only call: the permit status, the false-alarm count and the company's name are there to inform the dispatcher, not to dispatch or to refuse a response on their own. The decision, and the record of it, stays with the center.

## What has been tested, and what has not

The API tests cover the pieces: a company's token is accepted on its own address and the **wrong token is refused**; an **expired permit** and **three false alarms** at an address are flagged on the alarm; and a **phoned alarm** typed by a call-taker, with a valid permit, is checked the same way and records who typed it. A browser test exercises the alarm card a call-taker uses to type a phoned alarm.

What has **not** been tested is bitdispatcher against a **real central station**: the API tests use alarms posted directly, not a monitoring company's live system, and no ASAP path exists to test at all. bitdispatcher is not certified and is not in live service anywhere, so no real alarm company has ever sent it an alarm. A center adopting this should run a trial with its own alarm companies and its own permit data before relying on it.

## Try it

On the [demonstration](https://demo.bitdispatcher.com), type a phoned alarm on the alarm card and watch it arrive in the queue. Alarm companies' own addresses and the permit list are set up by an administrator, and the public demonstration does not sign people in as administrators: to try those, run bitdispatcher on your own computer with `-demo` (see [Get it](/get/)) and sign in as the administrator. The relevant manual chapters are [Outside alerts](/manual/#alerts) and [Alarms and permits](/manual/#admin-alarms). The source is on [GitHub](https://github.com/BioDream-LLC/bitdispatcher).

Related: [Cameras near an incident](/tech/cameras-near-an-incident/), [Non-emergency intake](/tech/non-emergency-intake/), [The event ledger](/tech/event-ledger/), [Public incident feed](/tech/public-incident-feed/)
