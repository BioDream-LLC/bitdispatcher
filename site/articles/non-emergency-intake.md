---
title: A non-emergency line that takes the report and hands over the emergency
tile: Non-emergency intake
summary: A script takes a non-emergency report through a voice or text bot, and hands any emergency to a call-taker at once.
description: How bitdispatcher runs a non-emergency intake script through a voice or text bot, takes a report, and hands any emergency to a person at once.
icon: phone
---

## What it is

Not every call to a public safety agency is an emergency. A great many are a report of something that already happened: a car broken into overnight, a mailbox knocked down, a found wallet, a parking complaint. These belong on a non-emergency line, not on 911, and they still have to be written down, placed, and given to someone to act on.

Many agencies now put a voice or text bot on that non-emergency line to take the routine reports. The risk is obvious: a caller who dialed the wrong number, or whose situation has just turned into an emergency, must not be left talking to a bot. The safe design is one where the machine carries the routine conversation but steps aside the instant anything sounds like an emergency.

bitdispatcher's **non-emergency intake** is that design. A bot asks the agency's questions one at a time and writes down the answers, and the moment the caller says anything that sounds like an emergency, the intake stops and hands the caller to a call-taker. A finished report becomes a call in the non-emergency list for a person to act on. Nothing is dispatched by the intake itself.

## How it works

The intake is split in two on purpose. The **voice** belongs to a bot on the non-emergency line, which can be **BitVoice's AI receptionist or any other** voice or text system. bitdispatcher does not answer the phone. What bitdispatcher owns is the **script**: what to ask next, whether the conversation is finished, and whether to transfer the caller to a person.

The bot talks to bitdispatcher over two endpoints. It calls `POST /api/intake/sessions` to start a conversation, and gets back the greeting, the first question, and a session id. For each thing the caller then says, it calls `POST /api/intake/sessions/<id>` with the words, and gets back what to say next, whether the conversation is done, and whether to hand the caller over. A bearer token, made by an administrator and shown once, authorizes the bot. A conversation that nobody speaks in for 30 minutes is forgotten.

The **default script** opens with a greeting that says "If this is an emergency, say emergency now, or hang up and dial 911," then asks, in order: is anyone hurt or in danger right now, what would you like to report, what is the address or place, when did it happen, what is your name, and the best phone number to reach you. The agency edits this in the admin page: the wording of each question, its kind (words, address, phone, or yes/no), whether it is **required**, and, for a yes/no question, whether a **yes hands the caller over**. The closing tells the caller a report number; the hand-over line and the emergency words are editable too.

**Hand-over** is the part that keeps the intake safe. Three things send the caller to a call-taker at once:

- Any **emergency word** anywhere in what the caller says. The default list includes emergency, hurt, bleeding, fire, smoke, gun, knife, breaking in, right now, not breathing, overdose and others, plus the agency's own critical words.
- A **yes** to a question marked as a danger question (the default danger question is "Is anyone hurt or in danger right now?").
- Anything bitdispatcher's **transcript reader marks urgent**, such as a weapon or "not breathing," found in the caller's words by the same extraction rules used on live 911 calls.

The intake reads negation the way a person would: a word right after no, not, nobody, none, never or a contraction does not count, so "nobody is hurt" and "not bleeding" do not trip a hand-over. And when a danger question gets no clear answer after being asked twice, the intake does not guess; it hands the caller to a person.

A finished report becomes a **call in the non-emergency list** with every answer, the whole conversation, and a report number told to the caller. It does not time out of that list. The address is placed on the map when it is found exactly. Nothing is dispatched; a person decides what to do with the report. A handed-over conversation becomes a call **marked as an emergency**, carrying what was said so far, so the call-taker who picks it up starts with the story already written down.

## Why it matters in a 911 center

Non-emergency volume is real work. Reports that do not need a unit right now still need to be taken accurately, located, and queued, and every minute a call-taker spends typing a routine larceny report is a minute not spent on the next 911 call. A script that collects the facts in a consistent order, with the address and callback captured the same way every time, turns a scattered intake into a clean report a person can act on quickly.

The danger is the edge case, and it is the whole point of the design. A caller who dialed the non-emergency line because they did not think it was serious, and then realizes it is, cannot be trapped in a questionnaire. By treating any emergency word, any yes to a danger question, and any urgent fact as an immediate hand-over, the intake fails toward a person rather than toward a form. Reading negation so that "nobody is hurt" does not force a needless transfer keeps the hand-over from firing on every sentence, which would make it useless.

Keeping the report out of the dispatch flow matters too. The non-emergency list is a holding place, not a queue of pending dispatches. A person reads the report and decides whether to send a unit, call the reporter back, or file it. The intake never makes that decision.

## Where CAD systems usually fall short

The non-emergency line is often an afterthought. Reports come in by a web form that lands in an inbox, or by a voicemail that someone transcribes later, with no common structure and no location on the map. Where a bot is used, it is frequently a standalone product that collects a form and emails it, disconnected from the CAD where the report actually has to live.

The deeper problem is safety. A deflection bot that is good at keeping callers off 911 is dangerous precisely when a caller on the non-emergency line is having an emergency. Some systems hand off only when the caller explicitly asks, or after the bot has worked through its whole script, which is exactly backward: the hand-over has to be eager, not reluctant. And when the machine does the judging of whether something is urgent, it is often a model whose reasons cannot be inspected, so the agency cannot see why a call was or was not transferred.

## How bitdispatcher does it

The script lives in the one program. An administrator writes the questions, the greeting, the closing, the hand-over line and the emergency words under **Admin › Non-emergency intake**, and makes the bot's token there (shown once). The same extraction rules that read live 911 calls read the intake caller's words, so "he has a gun" or "she is not breathing" hands the caller over on the strength of the same deterministic patterns, and each urgent fact points back to the words it came from.

The hand-over rules are plain and editable rather than hidden in a model: a fixed list of emergency words, a flag on a yes/no question that makes it a danger question, and the urgent-fact reader. Negation handling is deliberate and tested. A report that finishes cleanly is a non-emergency call with the full transcript and the answers; a report that hands over is an emergency call with the story attached. The intake's whole take is that the **safe part is the script and the hand-over rules**, with the voice left to call handling, which is the reverse of selling an AI that decides on its own which callers deserve a person.

## Why bitdispatcher fits

The intake is part of bitdispatcher, not a bolt-on. The report it produces is a call in the same list, on the same map, with the same transcript panel as any other, so a non-emergency report is a first-class record and not an email. The voice is left to whatever answers the phone, so an agency can use BitVoice's receptionist or keep the bot it already has. It is free and open, set up entirely in the GUI, and the design rule that runs through the whole system holds here too: the intake advises and collects, and a person decides. Nothing the intake does dispatches, closes or re-prioritizes anything on its own.

## What has been tested, and what has not

The conversation logic is covered by Go tests: the questions are asked in order, a required answer is asked again, negation is read so "nobody is hurt" does not hand over, and a yes to the danger question, or the words bleeding, gun or smoke, hands the caller over. An API test drives a bot with the token through a full report, checks that it becomes a non-emergency call with the callback number, and checks that "breaking in ... right now" hands the caller over. A browser test exercises the Try it page.

What has **not** been tested is the thing that matters most in the field: the intake has not been run with BitVoice's AI receptionist or any real voice bot, and not with real callers. Speech recognition errors, accents, background noise and the pace of a real phone call are not exercised by these tests. bitdispatcher is **not certified** and is not in service anywhere. An agency considering the intake should treat the hand-over rules as the safety boundary and test them hard against its own voice bot and its own callers before trusting them.

## Try it

On the [demonstration](https://demo.bitdispatcher.com), open **Admin › Non-emergency intake** and use **Try it**: it runs the saved script as a chat and makes nothing, so you can see the questions, watch a hand-over fire on an emergency word, and read the report that would result. The relevant manual chapters are [The non-emergency intake](/manual/#nonemergency) and [Setting up the intake](/manual/#admin-intake). The source is on [GitHub](https://github.com/BioDream-LLC/bitdispatcher).

Related: [Alarms and permits](/tech/alarms-and-permits/), [Live call transcripts and translation](/tech/transcripts-and-translation/), [Where the caller is](/tech/caller-location/), [The event ledger](/tech/event-ledger/)
