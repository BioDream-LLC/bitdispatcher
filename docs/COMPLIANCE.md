# Compliance and certification

**bitdispatcher is not certified by anyone, and no one has audited it.** This page says what each
framework covers, what bitdispatcher does towards it, and what is missing. It is not legal or compliance
advice. Sources are listed at the end; the CJIS Security Policy itself is the authority.

## What can and cannot be certified

| Framework | What it covers | Applies to bitdispatcher? |
|---|---|---|
| FBI CJIS Security Policy (v6.1, 2026-06-25) | Agencies, and the vendors and systems that handle criminal justice information (CJI) | Yes, through the agency. **There is no FBI certification for software or vendors.** Compliance is the agency's, audited by its state CJIS Systems Agency (CSA); vendor staff with access sign the CJIS Security Addendum. |
| FedRAMP / GovRAMP (formerly StateRAMP) | Cloud services sold to governments | Only if bitdispatcher were offered as a hosted service. Self-hosted on the agency's servers, they do not apply. |
| Cal OES 9-1-1 approval | Call-handling equipment (CPE) in California | No: that is call handling (BitVoice's side), not CAD. No Cal OES CAD certification was found. |
| ProQA CAD interface (Priority Dispatch) | A licensed interface to ProQA, certified by Priority Dispatch | Not held. bitdispatcher offers a determinant hook (docs/PROTOCOLS.md). |
| NENA standards (i3, EIDO, HELD use) | Interfaces | Followed where implemented; there is no NENA product certification. |
| SOC 2 | An organisation's controls, attested by an auditor | Possible for an organisation that hosts or supports bitdispatcher; not for the code alone. |
| State interfaces (VCIN in Virginia, the state IBR file) | Each state's own acceptance | Not yet accepted (README Status). |

Say "designed to support an agency's CJIS Security Policy compliance", never "CJIS certified" or "CJIS compliant".

## CJIS Security Policy: what bitdispatcher does

The policy is organised by NIST SP 800-53 control families. The software-relevant ones:

| Area | bitdispatcher | Gap or agency task |
|---|---|---|
| **AC: access control** | Roles and duties enforced by the server (call-taker, dispatchers by discipline, supervisor, administrator, records, field, viewer); workstations enrolled by an administrator; field crews limited to their own unit; disabled accounts; records requests released only by approval | Least-privilege role design and periodic account review are the agency's |
| **AC: session lock** | The console locks after the agency's idle time (and Ctrl+Shift+L) and unlocks only for the same person; sessions end after 12 hours idle or 14 hours | Set the idle time to the policy's value (Settings) |
| **AC: system use notification** | The agency's notice on the sign-in page, before anyone signs in (Admin › Settings › Sign-in notice) | The wording is the agency's |
| **IA: identification and authentication** | Personal accounts; passwords of at least 12 characters with variety, hashed with PBKDF2-SHA256 (600,000 iterations); 5 failures lock the account for 15 minutes | No check against breached-password lists; no forced rotation (current NIST guidance does not require it) |
| **IA: multi-factor** | TOTP one-time codes (authenticator app) for everyone or for supervisors and administrators; codes cannot be reused; an administrator resets a lost authenticator. In FIPS 140-only mode new authenticators use HMAC-SHA-256 (see FIPS 140-3 below) | CJIS requires MFA for all users: set it to everyone. TOTP has not been reviewed by a CSA as meeting the policy's authenticator assurance |
| **AU: audit and accountability** | Every change is an event in an append-only ledger, SHA-256 hash-chained and verifiable from the browser; sign-ins, locks, configuration changes, review views opened, records released, NCIC inquiries with purpose and reason, files added and hidden | Not every read is logged (opening an incident on the console is not); log review and retention are the agency's |
| **SC: protection in transit** | HTTPS only (HSTS, a strict Content-Security-Policy); TLS 1.2 or later to BitVoice and other sites, with pinned certificates. With `make fips`, the approved cryptography (TLS, hashing, signatures, random numbers) comes from the Go Cryptographic Module v1.0.0 (FIPS 140-3 certificate #5247) in FIPS 140-3 mode; see below | The default `make build` is not in FIPS mode. The certificate covers the module, not bitdispatcher, and only when operated in approved mode |
| **SC: protection at rest** | The ledger, files and photos are plain files with 0600/0640 permissions | Not encrypted by bitdispatcher: use full-disk encryption with a FIPS-validated module on the server |
| **MP: media protection** | Files on incidents are kept, hidden rather than deleted, with SHA-256 in the ledger | Backups, their encryption and their destruction are the agency's |
| **CM: configuration management** | Configuration changes are in the ledger; changes can be tried on a training board first and published with a reason. Dependency versions are pinned, and every CI run checks them and the Go standard library for known vulnerabilities (govulncheck, pnpm audit; `make vuln`) | The agency's change process, and installing updates |
| **IR, PE, PS, SA, SR** | Not software features | The agency's and the vendor's processes |

**Outbound connections that carry CJI or caller data** are each off until an administrator turns them
on, and each is recorded: the language model, NERIS, neighbouring agencies' CADs, the public feed, the
location server, the views providers, alarm answers, the station gateway. An agency should send CJI only
to systems covered by its own CJIS agreements.

## FIPS 140-3

`make fips` builds with `GOFIPS140=v1.0.0`: the frozen Go Cryptographic Module v1.0.0, which NIST's
Cryptographic Module Validation Program validated under FIPS 140-3 certificate
[#5247](https://csrc.nist.gov/projects/cryptographic-module-validation-program/certificate/5247)
(level 1, active until 26 April 2031). Such a binary runs in FIPS 140-3 mode: TLS, hashing, HMAC,
PBKDF2, random numbers and signatures go through the validated module, and TLS uses only approved
versions, cipher suites and curves. The startup log and the go-live check say which mode and module the
server runs.

`GODEBUG=fips140=only` at run time goes further: any non-approved algorithm is refused. bitdispatcher's
Go tests (`make test-fips`, in every CI run) and the full browser suite pass that way. Three things were
changed for it:

- **Authenticators.** HMAC-SHA-1, which every authenticator app uses for TOTP, is refused, so a server
  in FIPS 140-only mode sets up new authenticators with HMAC-SHA-256 (`algorithm=SHA256` in the QR
  code). Not every app supports that (some always use SHA-1 and show wrong codes); test the agency's app
  first. An authenticator set up before the switch is refused with a message, and an administrator
  resets it.
- **NEMSIS response UUIDs** are derived from SHA-256 instead of SHA-1, and marked version 4 because the
  NEMSIS XSDs accept only versions 1 to 5.
- **The map extract's MD5**, which only detects a download cut short (the download itself is over
  HTTPS), is computed outside the module's enforcement, as Go allows for uses that protect nothing.

Not covered: the certificate is the module's, not bitdispatcher's, and applies "when operated in
approved mode". Data at rest (the ledger, files, photos) is not encrypted by bitdispatcher. `cmd/bvsim`
and `cmd/i3caller` are test tools, not built in FIPS mode. The FIPS build has been tested on Linux on
Intel only.

## Before relying on it

1. Turn on MFA for everyone, set the idle lock, and write the sign-in notice.
2. Build with `make fips`, check that the go-live check's *FIPS 140-3 cryptography* passes, and put the
   server's disks under FIPS-validated encryption.
3. Have the CSA or a qualified assessor review the deployment; record the gaps above in the agency's
   plan of action.

## Sources

- FBI CJIS Security Policy Resource Center: https://le.fbi.gov/cjis-division/cjis-security-policy-resource-center
- MFA sanctionable from 2024-10-01 (California DOJ bulletin): https://oag.ca.gov/system/files/media/2024-isrs-001.pdf
- There is no CJIS certification for vendors (Louisiana State Police vendor vetting): https://lsp.org/about/leadershipsections/support/bcii/centralized-vendor-vetting-program/
- Cal OES CPE technical requirements: https://www.caloes.ca.gov/wp-content/uploads/PSC/Documents/EXHIBIT-B_TECHNICAL-REQUIREMENTS.A1.pdf
- Priority Dispatch CAD interface: https://www.prioritydispatch.net/en/cad-interface
