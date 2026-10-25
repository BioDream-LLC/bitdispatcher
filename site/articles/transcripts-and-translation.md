---
title: Live call transcripts and translating the caller
tile: Transcripts and translation
summary: Transcribe the call live, pull out the facts by rule, and translate Spanish callers line by line with a built-in phrasebook.
description: How bitdispatcher transcribes 911 calls, extracts address, weapons and hazards by rule, and translates Spanish callers with an offline emergency phrasebook.
icon: mic
---

## What it is

A 911 call is spoken, and the call-taker turns speech into a typed record while listening, questioning, calming and dispatching all at once. Transcription and translation are two aids aimed at that moment: one writes down what is said, the other bridges a language the call-taker does not speak.

**Live transcription** is speech-to-text applied to the call as it happens. The caller's words and the call-taker's words appear on screen in near real time. On top of the raw transcript sits **extraction**: pulling the facts a dispatcher needs, the address, a weapon, a vehicle and plate, a direction of travel, how many people, injuries and hazards, out of the flowing text so the call-taker does not have to type them from memory.

**Translation** is the harder problem. A large share of the United States speaks a language other than English at home, and Spanish is by far the most common. The traditional answer is a three-way call with a telephone interpreter, which works but costs seconds and a conference bridge. A built-in translation aid tries to cover the first moments: understand enough of what the caller said to act, and offer phrases to say back.

None of this is a standard the way CAP or EIDO is; it is a set of techniques. What matters is accuracy, honesty about accuracy, and the rule that a human being always decides. A transcript is what a machine heard, not what happened, and a translation is an aid, not a certified interpreter.

## How it works

Transcription needs a speech engine. The audio stream is fed to an automatic speech recognition system, and text comes back, ideally labeled by speaker so the caller and the call-taker are told apart. Latency matters: a transcript that lags ten seconds behind is a record, not an aid.

Extraction is where design choices diverge. One approach uses a language model to read the transcript and guess the salient facts. The other uses deterministic rules: patterns that recognize an address, a caliber, a plate format, the phrase "not breathing." Rules are narrower but have a property that matters in public safety: the same words always produce the same result, and every extracted fact can point back to the words it came from. A dispatcher can see why the system thinks there is a weapon.

Translation can likewise be a model or a phrasebook. A general model translates anything but can confidently mistranslate, and it sends the caller's words to wherever the model runs. A curated emergency phrasebook covers the vocabulary of emergencies, breathing, bleeding, fire, trapped, address, offline and bounded, and, crucially, can admit when a line falls outside what it knows.

The honest systems make the uncertainty visible: urgent facts flagged, low-confidence translations marked, and a clear path to a live interpreter when the aid runs out. The worst failure mode is confident wrongness, a clean sentence that is simply not what the caller said, because it reads as authoritative and invites the dispatcher to act on it.

## Why it matters in a 911 center

On a chaotic call, the call-taker's hands cannot keep up with the caller's mouth. Extraction that places the address, flags the weapon and surfaces "not breathing" lets the call-taker keep listening instead of transcribing. When the extracted address can be dropped onto the map with one press, the incident starts seconds sooner.

On a Spanish call with no interpreter yet on the line, a phrasebook that conveys the gist, and offers "Help is on the way" and "Is the person breathing?" to say back, can carry the first minute until an interpreter joins. That minute is often the one where the dispatch decision is made.

Accountability runs through both. The transcript becomes part of the record, and the review later can see what was said, what the machine pulled out, and what words it pulled it from. The judgment stays with the person.

## Where CAD systems usually fall short

Transcription is often a separate recording-and-logging product, useful after the fact but not live on the call-taker's screen where it could save typing. When extraction exists, it is frequently a black box: it asserts a weapon or an address without showing why, so a cautious dispatcher cannot trust it and an incautious one trusts it too much.

Translation is commonly outsourced entirely to a telephone interpreter line, with nothing on the screen in the gap before the interpreter answers. Where machine translation is offered, it is often an unbounded model that will translate confidently whether or not it got it right, and that quietly sends caller audio or text to an outside service, a privacy question agencies are rarely asked to answer deliberately.

## How bitdispatcher does it

When call handling transcribes calls, each call shows its **transcript** live, the caller in white and the call-taker in blue, and a **Heard on the call** panel. The panel holds the facts pulled out of the call: the address, weapons, vehicles, plates, the direction someone went, how many people, injuries (not breathing, unconscious, bleeding), hazards (fire, trapped, gas, a dog, an electric car), and the caller's name. Urgent ones are red and come first, and show on the call in the queue. Pressing an address places the call's incident there with a map search, exact when the map knows the number; before there is an incident it copies the address to paste.

The extraction is done by **rules, not a model**: the same words always give the same facts, and each extracted detail shows the words it came from. Transcripts come from BitVoice's live transcript when it sends one, from its transcript of a recorded call afterward, or from **any speech-to-text engine** posting to the transcript hook. The manual states the principle plainly: a transcript is what a machine heard, and the call-taker's own judgment and questions still decide.

For a Spanish caller, each line is translated on the transcript and marked **Spanish · translated**, with **Show original** to see what was said. The **phrasebook** built into bitdispatcher translates the words of emergencies between Spanish and English, offline; when it knows too few of a line's words it says **gist only**, meaning get an interpreter. **Say it in Spanish** offers ready phrases (Help is on the way, Is the person breathing?, Get out of the house now) or lets the call-taker type in English and see the translation; the phrase is kept on the transcript, texted when **also text it** is ticked (with BitVoice), and spoken aloud on the console by the speaker button.

An optional **language model** (Admin › Integrations) translates what the phrasebook cannot, over an OpenAI-compatible chat API, either on the agency's own server or a service under the agency's agreement. Because what callers say is sent to it, it is **off until an administrator switches it on**. Separately, when BitVoice or the radio system transcribes transmissions, a unit's words go on its incident's timeline in italics, so the record holds what was said on the air.

## Why bitdispatcher fits

The transcript, the extracted facts and the translation are on the call-taker's screen inside the one program, not in a separate logger. Extraction is rule-based and explainable: every fact points to its words, so a dispatcher can trust it or reject it on sight. Translation starts with an offline phrasebook that needs no outside service and admits its limits, so the first minute of a Spanish call is covered without sending anyone's words anywhere. The optional model is a deliberate, administrator-controlled choice, with the privacy cost stated. And a person always decides: nothing in the transcript dispatches, closes or reprioritizes on its own.

## What has been tested, and what has not

Transcripts are **tested live with BitVoice's own speech-to-text** using a recorded caller. The phrasebook translates **the language of emergencies, not everything**, and says so when it has only the gist. The optional language model is off by default and, when on, sends the record's text to that model. Any speech-to-text engine can post to the transcript hook, but engines other than BitVoice's have not each been exercised. The extraction rules are deterministic and tested, but they recognize patterns, not meaning, so the call-taker's judgment remains the decision.

## Try it

On the [live demo](https://demo.bitdispatcher.com), simulated calls arrive every 35 seconds; open one to see the live transcript and the Heard on the call panel. The manual chapters are [Transcripts and what was heard](/manual/#transcript), [Callers in another language](/manual/#translation), and [Radio transcripts](/manual/#radio-text).

Related: [Caller Location](/tech/caller-location/), [Crash Data and Drones](/tech/crash-and-drones/), [NENA i3 and EIDO](/tech/nena-i3-eido/)
