---
title: Agency scripts and deterministic advisories on the dispatch floor
tile: Scripts and advisories
summary: Sandboxed agency scripts in JavaScript and Lua, and an advisory panel of deterministic rules from a versioned codebook, tested first.
description: How a CAD advises without deciding: sandboxed agency scripts in JS and Lua, plus deterministic evidence rules from a versioned codebook, tested first.
icon: spark
---

## What it is

Every 911 center wants its software to help spot the things a busy dispatcher can miss: a second call that is really the same incident, a fire described on a call typed as a medical, a priority-one incident still without a unit. The question is how that help is built. There are two broad approaches, and bitdispatcher uses both, deliberately in that order.

The first is **deterministic rules**: plain statements of evidence and threshold that always give the same answer for the same facts and can be read in words. The second is **agency scripts**: small programs an agency writes for its own local logic. Both advise only. Neither dispatches, closes, retypes or reprioritizes anything on its own. The dispatcher decides, which is the first rule of the system.

This is distinct from a protocol system such as licensed EMD software, which interviews the caller to a determinant. The advisory panel and scripts watch the incident and the evidence, and suggest.

## How it works

**The advisory panel** draws on a versioned codebook of rules. Each rule names the evidence it relies on, the source of that evidence (unit, caller, dispatcher or machine), and a threshold. When a rule fires, it offers a suggestion the dispatcher can open to see *why*: the evidence, what it assumed absent, and the rule in plain words. Sources have a precedence the agency orders, strongest first, so a stronger source outweighs a weaker one; a silent alarm panel never outweighs a person who can see smoke. When sources of equal weight disagree, the honest answer is **cannot determine**, and the dispatcher decides.

The panel runs in one of three modes: off; **shadow**, where suggestions are evaluated and kept in the ledger but shown only to supervisors, so the rules can be judged before anyone relies on them; and **live**, where dispatchers see them. A new system starts in shadow.

**Agency scripts** add a rule that is the agency's own code, in JavaScript or Lua, versioned and switched between shadow and live with the rest of the codebook. A script defines one function for its hook: `advise(i)` for each open incident, `recommend(r)` for a unit recommendation, or `check(r)` for an incident report. Accepting a suggestion from a script runs its command on the incident, as the person who accepts.

The sandbox is strict. A script has no files, no network, no processes, no environment and no way to load more code: only its input and a `log()` call. It always has a time limit, 50 ms by default and at most one second, and it runs after the CAD's live state is released, so a slow script never holds dispatch up. A script that fails or runs out of time is shown under its rule, and the panel or recommendation carries on without it and says so.

## Why it matters in a 911 center

On a 911 floor the cost of a wrong or unexplained automated action is measured in seconds and in safety. If software silently merges two incidents and one was really separate, a unit may never be sent. If it quietly downgrades a priority, help is slower. A dispatcher who cannot see why the machine suggested something cannot judge it, and under load will either obey it blindly or ignore it entirely. Both are dangerous.

Determinism is what makes a suggestion trustworthy at three in the morning. The same facts always give the same suggestion, each suggestion shows the words it came from, and a reviewer later can reconstruct exactly what the dispatcher was shown. That is also what makes a suggestion defensible afterward, when an incident is reviewed.

## Why deterministic rules before AI on a 911 floor

There is a strong case for proving deterministic rules before introducing a model that cannot explain itself. A deterministic rule is auditable: you can read it, test it against real history, and know it will behave the same tomorrow. A statistical model trained on past calls can be more flexible, but it is harder to explain, can drift as its inputs change, and can fail in ways that are difficult to predict from the incident in front of you. On a dispatch floor, where a suggestion can influence whether and how fast help goes, the ability to say precisely why the system said something is not a nicety. Start with rules you can read and verify; add anything less transparent only once the deterministic ground is solid and the rules have earned trust in shadow mode.

## Where CAD systems usually fall short

Industry advisory features often hide their logic, so a dispatcher sees a flag but not the reasoning, and cannot tell a solid inference from a guess. Rules are frequently baked in and un-versioned, so a center cannot tune them, cannot see when they changed, and cannot restore a known-good set. Scripting, where it exists, often runs with broad access to the system, which is a security and stability risk, and without a hard time limit, so a bad script can stall the floor. And new features sometimes act rather than advise, taking an action the dispatcher did not choose.

## How bitdispatcher does it

Admin › Advisory panel holds the codebook in plain words with thresholds. A supervisor sets each rule's mode, severity, distances, times and the words and call types it fits, and can edit its explanation. **Source precedence** is a drag-to-order list of unit, caller, dispatcher and machine evidence. **Test** replays the recorded ledger for the last day, week, month or quarter against the rules as they are on screen, saved or not, and counts what each rule would have suggested, so a rule is tried against real incidents before it goes live. **Save version** keeps the change with a note; every version is listed and restorable in one click, and rejecting a suggestion is never treated as an error.

For scripts, **Try it** runs the script once on the newest real incident or report and shows what it returned, what it logged, how long it took, and, with **show the input**, exactly what it saw. An incident script sees the incident's number, type, priority, status, address and position, its age in minutes, notes, the units on it, the hazards at its premises, and the caller's name and number of calls, and nothing else.

At the console, the advisory panel sits beside the incident and on the Situation panel. Each suggestion opens to its evidence and rule; **Accept** runs the suggested command as the dispatcher or just notes it, and **Reject** takes a one-click reason and is not shown again. Both are recorded. Core dispatch works the same when the panel is off or down.

The built-in rules cover the mistakes that recur on a busy floor: two incidents that may be the same, a call that may be about an already-open incident, words that do not fit the call type such as "smoke" on a fall or "not breathing" on a fire alarm, a repeat address, a priority-one incident still without a unit, and a unit reported on scene but not heard from. When a system upgrade supplies a new rule, such as one for a silent or abandoned 911 call, the codebook screen offers it above the existing rules so a supervisor can add it and save a new version deliberately; a saved codebook never changes by itself. Because each rule names its evidence and source, a center can tune a rule to its own call types and thresholds and read, in plain words, exactly what will make it fire.

## Why bitdispatcher fits

The codebook and the scripts are the agency's, set in the GUI, versioned and recorded in the ledger, so a center owns its own logic and can prove what it was. Everything advises and nothing acts on its own, which keeps the dispatcher in charge. Shadow mode and the replay test mean a rule proves itself against the center's own history before it reaches the floor. And the sandbox keeps local code from touching anything it should not.

## What has been tested, and what has not

The scripts engine, the sandbox and the deterministic rules are covered by the Go suites. An open design question is whether agency scripts may ever reach outside the sandbox; today they cannot, and that is the safe default. The advisory rules are aids, not judgments, and the center should run them in shadow and replay them against its own recorded incidents before trusting them live. Version 0.0.0 is not yet in service anywhere.

## Try it

On the demonstration, open an incident and watch the advisory panel: a possible duplicate, a call near an open incident, or words that do not fit the call type. Open a suggestion to see its evidence and the rule in plain words.

- [Advisory suggestions](/manual/#advisory)
- [The advisory codebook](/manual/#admin-advisory)
- [Operations rules](/manual/#admin-operations)
- [Ledger and audit](/manual/#ledger)

Related: [The event ledger](/tech/event-ledger/), [Security](/tech/security/), [Training and staged changes](/tech/training-and-staged-changes/), [Road-time recommendation](/tech/road-time-recommendation/)
