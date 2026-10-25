---
title: Securing a CAD: sign-in, audit and the CJIS control areas
tile: Security
summary: How a CAD is secured: duty-based access, enrolled workstations, one-time codes, a hash-chained audit ledger and scanned attachments.
description: How to secure a 911 CAD: duty-based sign-in, enrolled workstations, one-time codes, console lock, TLS, a hash-chained audit ledger and scanned files.
icon: lock
---

## What it is

Security in a computer-aided dispatch (CAD) system is not one feature. It is the set of controls that decide who may sign in, what each person may do, how the system proves what happened, and how it protects the data it holds. A 911 CAD touches criminal justice information (CJI), caller locations, medical details and the record that an agency will rely on in court, so the controls have to be real and recorded, not cosmetic.

The reference most US agencies work to is the **FBI CJIS Security Policy** (version 6.1, dated 2026-06-25). It is organized around the NIST SP 800-53 control families: access control (AC), identification and authentication (IA), audit and accountability (AU), system and communications protection (SC), media protection (MP), and configuration management (CM), among others. Compliance belongs to the agency and is audited by its state CJIS Systems Agency. There is no FBI certification for software or vendors; a vendor's staff with access sign the CJIS Security Addendum. That distinction matters: a product can be *built to support* an agency's compliance without ever being "CJIS certified," because no such certification exists.

## How it works

Good CAD security layers several controls so that no single mistake exposes the system.

- **Authentication** proves who a person is: a personal account, a strong password, and a second factor such as a one-time code from an authenticator app.
- **Authorization** decides what that person may do, enforced on the server so that hiding a button on the screen is never the only barrier.
- **Session controls** lock an idle console and end stale sessions, so an unattended position does not stay open.
- **Transport protection** puts every byte over TLS, so credentials and CJI are not readable on the wire.
- **Audit** records who did what, when, and to which record, in a form that cannot be quietly changed.
- **Content controls** check files coming in, so an attachment cannot carry malware into the center.

Each control maps to a CJIS area: access control and session lock to AC, passwords and one-time codes to IA, the audit trail to AU, TLS to SC, and file handling to MP. Setting the idle timeout, writing the system-use notice and turning on multi-factor for everyone are the agency's tasks; the software's job is to make them possible and to record them.

## Why it matters in a 911 center

A dispatch floor is a shared, always-on space. Positions are left for breaks, shifts change mid-incident, and more than one person may sit at the same console in a day. A control that depends on a dispatcher remembering to lock a screen will fail. The audit trail matters because every dispatch decision can be questioned later: who changed a priority, who closed an incident, who looked at a caller's photo. If that record can be edited, it is worth nothing in a dispute. And because the center runs CJI inquiries and holds evidence files, a single malicious attachment or an unmonitored outbound connection is a path straight into protected data.

## Where CAD systems usually fall short

Across the industry, security problems tend to repeat. Permissions are enforced in the browser but not on the server, so a crafted request does what the screen hid. Audit logs are mutable database rows that an administrator can change without trace. Multi-factor is offered but never turned on. Attachments are accepted and shown without being scanned, or scanned only when a scanner happens to be running. Outbound integrations are left on by default, quietly sending data to third parties. And the configuration that governs all of this is edited in files on the server, outside any audit, so no one can say when a rule changed or why.

## How bitdispatcher does it

bitdispatcher enforces access on the server for every duty. A person signs in choosing the duty they are working: call-taker, police, fire or EMS dispatcher, all-discipline dispatcher, supervisor, administrator or records. Each duty opens its own workspace and the server decides what it may do, so a call-taker cannot dispatch unless the agency allows it, and a records duty changes no incident. A supervisory view a person has no right to is refused on the server and the attempt is recorded, not merely hidden from the menu.

**Enrolled workstations.** An administrator enrolls each console once from Admin › Workstations, giving it a short id such as `CT1` and, if wanted, the duties that position may take. An enrolled browser signs in as its position, so nobody is asked which console they are at and every action is recorded against a known position. A browser that is not enrolled gets a web position of its own (`WEB-` and four letters). Each enrolment and revocation is a ledger event.

**One-time codes.** When the agency turns them on (Settings › Roles and sign-in), the first sign-in shows a QR code to scan with any authenticator app; after that the current six-digit code is typed each time and a code works once. The agency can require codes for everyone or only for supervisors and administrators. CJIS requires multi-factor for all users, so set it to everyone. An administrator resets the code for a lost phone.

**Console lock.** `Ctrl+Shift+L`, or the agency's set idle minutes, locks the console: nothing is shown or changed until the same person unlocks it with their password and code. What was open, including half-typed text, stays underneath. Signing in and out, locking and unlocking are all in the ledger with the duty and workstation.

**TLS and credentials.** bitdispatcher serves over HTTPS only, with HSTS and a strict content-security policy, and uses TLS 1.2 or later with pinned certificates to BitVoice and other sites. Passwords are at least 12 characters, hashed with PBKDF2-SHA256 at 600,000 iterations; five wrong tries lock an account for 15 minutes.

**The ledger as audit.** Every action that changes state or creates accountability is an event in an append-only, SHA-256 hash-chained ledger: sign-ins, locks, configuration changes, review views opened, records released, NCIC inquiries with purpose and reason, files added and hidden. Each event carries the hash of the one before it, so a change anywhere breaks the chain, and **Verify** in Admin › Ledger reports the first broken link. The live state is rebuilt from the ledger at every start, and reports are built from it.

**Attachments.** Files on an incident or a message are limited by type and size, and the server decides what a file is by its content, not its name. Each file's SHA-256 is written to the ledger; a file changed on disk afterwards is refused, not shown. Where ClamAV is set up, uploads are virus-scanned with its `clamd` daemon, and an upload is refused when the scanner is down rather than let through unchecked. A supervisor can hide a file with a reason, but it is never deleted from the record.

**Sandboxed scripts.** Agency scripts for advisories and reports run with no files, network, processes or environment, only their input and a `log()` call, under a time limit of 50 ms by default and at most one second. A script that fails or times out is reported and the system carries on without it.

These controls map onto the CJIS areas (AC, IA, AU, SC, MP, CM); **this is building to the policy, not a claim of compliance or certification.**

## Why bitdispatcher fits

bitdispatcher is one free, open-source binary with no separate database or application server, so the security surface is small and inspectable: the source is published under Apache 2.0, and the ledger is a file whose integrity anyone can verify. Every control is set in the web interface and recorded in the ledger, including the configuration itself, so an auditor can see when a rule changed and why. Outbound connections that could carry CJI or caller data are each off until an administrator turns them on, and each is recorded.

## What has been tested, and what has not

The one-time codes, console lock, sign-in notice and audit records have not been reviewed against the CJIS Security Policy; there is no FBI certification for software. **FIPS 140-3:** `make fips` builds bitdispatcher with the Go Cryptographic Module v1.0.0, which holds FIPS 140-3 certificate #5247 from NIST's validation program, and runs it in FIPS 140-3 mode. The Go tests and the full browser suite pass in FIPS 140-only mode, where every non-approved algorithm is refused; for that, authenticators set up on such a server use HMAC-SHA-256 instead of SHA-1, which not every authenticator app supports. The go-live check says which mode the server is in. The certificate covers the module, not bitdispatcher, and the server's disks still need encrypting with a validated module. Every CI run checks the Go code, its modules, the standard library and the web interface's packages for known vulnerabilities (govulncheck and pnpm audit). The ledger, the rebuilt state and the server-enforced agency-scoped board are covered by the Go suites and tested through the API; the console lock and sign-in are exercised end to end with Playwright. The attachment virus scan was run against ClamAV 1.5.4's `clamd` with full signatures: a clean file passes and the standard EICAR test file is refused on both kinds of upload. What each CJIS control area does and where the gaps are is written up in `docs/COMPLIANCE.md`.

## Try it

On the demonstration, sign in and choose a duty to see how the server shapes each workspace, then press `Ctrl+Shift+L` to lock the console and unlock it again. Open the ledger to watch sign-ins and locks recorded.

- [Signing in: your duty, codes and the lock](/manual/#signin)
- [Workstations](/manual/#admin-workstations)
- [Ledger and audit](/manual/#ledger)
- [Files and photos on an incident](/manual/#files)
- [Settings](/manual/#admin-settings)

Related: [The event ledger](/tech/event-ledger/), [Several sites, active-active](/tech/active-active-sites/), [Agency scripts and advisories](/tech/scripts-and-advisories/), [Multi-agency dispatch](/tech/multi-agency/)
