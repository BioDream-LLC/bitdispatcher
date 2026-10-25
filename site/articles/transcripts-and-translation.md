---
title: Live call transcripts and translating the caller
tile: Transcripts and translation
summary: Transcribe the call live, pull out the facts by rule, and bridge callers in thirteen languages with built-in phrasebooks.
description: How bitdispatcher transcribes 911 calls, extracts address, weapons and hazards by rule, and bridges callers in thirteen languages with offline phrasebooks.
icon: mic
---

## What it is

A 911 call is spoken, and the call-taker turns speech into a typed record while listening, questioning, calming and dispatching all at once. Transcription and translation are two aids aimed at that moment: one writes down what is said, the other bridges a language the call-taker does not speak.

**Live transcription** is speech-to-text applied to the call as it happens. The caller's words and the call-taker's words appear on screen in near real time. On top of the raw transcript sits **extraction**: pulling the facts a dispatcher needs, the address, a weapon, a vehicle and plate, a direction of travel, how many people, injuries and hazards, out of the flowing text so the call-taker does not have to type them from memory.

**Translation** is the harder problem. A large share of the United States speaks a language other than English at home. Spanish is by far the most common, but a center may hear many others. The traditional answer is a three-way call with a telephone interpreter, which works but costs seconds and a conference bridge. A built-in translation aid tries to cover the first moments: understand enough of what the caller said to act, and offer phrases to say back.

None of this is a standard the way CAP or EIDO is; it is a set of techniques. What matters is accuracy, honesty about accuracy, and the rule that a human being always decides. A transcript is what a machine heard, not what happened, and a translation is an aid, not a certified interpreter.

## How it works

Transcription needs a speech engine. The audio stream is fed to an automatic speech recognition system, and text comes back, ideally labeled by speaker so the caller and the call-taker are told apart. Latency matters: a transcript that lags ten seconds behind is a record, not an aid.

Extraction is where design choices diverge. One approach uses a language model to read the transcript and guess the salient facts. The other uses deterministic rules: patterns that recognize an address, a caliber, a plate format, the phrase "not breathing." Rules are narrower but have a property that matters in public safety: the same words always produce the same result, and every extracted fact can point back to the words it came from. A dispatcher can see why the system thinks there is a weapon.

Translation can likewise be a model or a phrasebook. A general model translates anything but can confidently mistranslate, and it sends the caller's words to wherever the model runs. A curated emergency phrasebook covers the vocabulary of emergencies, breathing, bleeding, fire, trapped, address, offline and bounded, and, crucially, can admit when a line falls outside what it knows.

The honest systems make the uncertainty visible: urgent facts flagged, low-confidence translations marked, and a clear path to a live interpreter when the aid runs out. The worst failure mode is confident wrongness, a clean sentence that is simply not what the caller said, because it reads as authoritative and invites the dispatcher to act on it.

## Why it matters in a 911 center

On a chaotic call, the call-taker's hands cannot keep up with the caller's mouth. Extraction that places the address, flags the weapon and surfaces "not breathing" lets the call-taker keep listening instead of transcribing. When the extracted address can be dropped onto the map with one press, the incident starts seconds sooner.

On a call with no interpreter yet on the line, a phrasebook that conveys the gist of what the caller said, and offers "Help is on the way" and "Is the person breathing?" to say back, can carry the first minute until an interpreter joins. That minute is often the one where the dispatch decision is made. For the languages a center hears rarely, even a rough bridge in those first seconds is better than silence while the conference call connects.

Accountability runs through both. The transcript becomes part of the record, and the review later can see what was said, what the machine pulled out, and what words it pulled it from. The judgment stays with the person.

## Where CAD systems usually fall short

Transcription is often a separate recording-and-logging product, useful after the fact but not live on the call-taker's screen where it could save typing. When extraction exists, it is frequently a black box: it asserts a weapon or an address without showing why, so a cautious dispatcher cannot trust it and an incautious one trusts it too much.

Translation is commonly outsourced entirely to a telephone interpreter line, with nothing on the screen in the gap before the interpreter answers. Where machine translation is offered, it is often an unbounded model that will translate confidently whether or not it got it right, and that quietly sends caller audio or text to an outside service, a privacy question agencies are rarely asked to answer deliberately.

## How bitdispatcher does it

When call handling transcribes calls, each call shows its **transcript** live, the caller in white and the call-taker in blue, and a **Heard on the call** panel. The panel holds the facts pulled out of the call: the address, weapons, vehicles, plates, the direction someone went, how many people, injuries (not breathing, unconscious, bleeding), hazards (fire, trapped, gas, a dog, an electric car), and the caller's name. Urgent ones are red and come first, and show on the call in the queue. Pressing an address places the call's incident there with a map search, exact when the map knows the number; before there is an incident it copies the address to paste.

The extraction is done by **rules, not a model**: the same words always give the same facts, and each extracted detail shows the words it came from. Transcripts come from BitVoice's live transcript when it sends one, from its transcript of a recorded call afterward, or from **any speech-to-text engine** posting to the transcript hook. The manual states the principle plainly: a transcript is what a machine heard, and the call-taker's own judgment and questions still decide.

**Spanish** keeps the fuller phrasebook: a word-by-word bridge both ways, offline, so a Spanish line is translated on the transcript and the call-taker can type English and get Spanish back. Each line is marked **Spanish · translated**, with **Show original** to see what was said.

Besides Spanish, the phrasebook now covers **twelve more languages**: Arabic, Chinese (Mandarin), French, Haitian Creole, Hindi, Japanese, Korean, Polish, Portuguese, Russian, Tagalog and Vietnamese. These are narrower than the Spanish phrasebook by design, and they work like this:

- **The language is recognized** from the caller's text, by its script for the languages that have their own (Arabic, Chinese, Hindi, Japanese, Korean, Russian), or by marker words for the ones written in the Latin alphabet (French, Haitian Creole, Polish, Portuguese, Tagalog, Vietnamese).
- **A caller's line becomes a gist, not a sentence.** The phrasebook finds the emergency words it knows in the line and lists them in English in the order they were said, with "..." standing in for the words it does not know. It does not try to reconstruct a fluent sentence, so what the call-taker reads is honest about how much was understood.
- **The twenty ready phrases are said in each language.** "Help is on the way," "Is the person breathing?", "Get out of the house now" and the rest of the reply buttons are translated into all thirteen languages, to say back or show the caller.
- **Typed English is not translated into these twelve.** Unlike Spanish, the call-taker cannot type a free English sentence and get it rendered in Vietnamese or Korean. The phrasebook will not make up a sentence it cannot stand behind; only the ready phrases come back. Free English is refused.

A plain honesty note belongs on these twelve: the phrasebooks were **machine-written and have not been checked by native speakers**. They are a bridge for the first minute, not a substitute for an interpreter, and the gist-only behavior is there so no one mistakes them for one. Browser speech reads the ready phrases aloud in the language's own voice when the console's speaker button is used.

An optional **language model** (Admin › Integrations) translates what the phrasebooks cannot, over an OpenAI-compatible chat API, either on the agency's own server or a service under the agency's agreement. Because what callers say is sent to it, it is **off until an administrator switches it on**. Separately, when BitVoice or the radio system transcribes transmissions, a unit's words go on its incident's timeline in italics, so the record holds what was said on the air.

## Why bitdispatcher fits

The transcript, the extracted facts and the translation are on the call-taker's screen inside the one program, not in a separate logger. Extraction is rule-based and explainable: every fact points to its words, so a dispatcher can trust it or reject it on sight. Translation starts with offline phrasebooks that need no outside service and admit their limits, so the first minute of a call in any of thirteen languages is covered without sending anyone's words anywhere. The twelve added languages are deliberately kept to a gist and the ready phrases rather than pretending to be fluent. The optional model is a separate, administrator-controlled choice, with the privacy cost stated. And a person always decides: nothing in the transcript dispatches, closes or reprioritizes on its own.

## What has been tested, and what has not

Transcripts are **tested live with BitVoice's own speech-to-text** using a recorded caller. The Spanish phrasebook translates the language of emergencies, not everything, and says so when it has only the gist. The twelve added phrasebooks are covered by Go tests: each language is detected from an emergency line, the gist that comes back contains English emergency words, every one of the twenty ready phrases round-trips into the language and back, and free English into those languages is refused. Those tests confirm the mechanics; they do not confirm the quality of the wording, because the phrasebooks were machine-written and **not checked by native speakers**. The optional language model is off by default and, when on, sends the record's text to that model. Any speech-to-text engine can post to the transcript hook, but engines other than BitVoice's have not each been exercised. The extraction rules are deterministic and tested, but they recognize patterns, not meaning, so the call-taker's judgment remains the decision. bitdispatcher is not certified and not in live service.

## Try it

On the [live demo](https://demo.bitdispatcher.com), simulated calls arrive every 35 seconds; open one to see the live transcript and the Heard on the call panel. The manual chapters are [Transcripts and what was heard](/manual/#transcript), [Callers in another language](/manual/#translation), and [Radio transcripts](/manual/#radio-text).

Related: [Caller Location](/tech/caller-location/), [Crash Data and Drones](/tech/crash-and-drones/), [NENA i3 and EIDO](/tech/nena-i3-eido/)
