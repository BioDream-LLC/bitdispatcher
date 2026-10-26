# Where each requirement stands

This page answers the review of the second wish document (updates 8 to 17). It uses the review's own
categories. Only the agency's reviewer can mark an item **Verified Complete**, because that needs every
acceptance test passed in the agency's environment and documented. What bitdispatcher can show is narrower:
whether each requirement is built, and which automated test exercises it.

Two kinds of evidence are listed:

- **Go test**: a unit or API test in this repository (`go test ./...`), run on every commit in CI.
- **Browser test**: an end-to-end Playwright test (`make e2e`). It drives a real bitdispatcher with demo data
  and the BitVoice simulator in Chromium, on Linux, at 1920×1080.

Nothing here was tested on the reviewer's laptops, keyboards or browsers, or against a live NCIC, VCIN or DMV
switch, a real PSC PIP system, Google Street View with an agency key, or EagleView. The NCIC tests use the
built-in training simulator.

## Updates 8 to 17

| Update | Built | Evidence | Not yet shown |
|---|---|---|---|
| 8 Priority, keys and one-line commands | Four-level priorities, agency key profiles per duty with Shift layer, F7 field initiate, F11 unit status, refusals recorded (built 24 Oct). New: a key test per workstation records keys the laptop keeps (F2 volume, F4 microphone) and the screen then shows Fn+key or the alternate; F5 new call; Ctrl+Enter and Alt+N alternates in the standard profile | Go: `TestFieldInitiateAndUnitStatus`; browser: wishes.spec "classic key profile", final.spec "key test" | Real laptop keyboards (a browser cannot see keys the hardware keeps; the test finds them by asking the person) |
| 9 Structured locations and flags | Place fields; verified, estimated, caller, map, forced or unverified, with reason and map point; never replaced by a nearby address; 12 flag categories with scope, versions, restricted detail, proposals and approval; possible matches near an unverified point | Go: `TestForcedLocationExceptionAndQueue`, `TestFlagsScopeVersionsAndRestriction`; browser: final.spec "forced location", "flag on a verified address" | Agency GIS verification queue against the agency's real address data |
| 10 Location exceptions, Shift Intelligence, PSC PIP | Supervisor alert and review with two separate decisions and dispositions; address queue; operational periods; report draft, finalize, addendum, acknowledgement, carry-forward; PSC PIP contract with pseudonyms, class filter, aggregate mode, corrections | Go: `TestForcedLocationExceptionAndQueue`, `TestShiftPeriodsFinalizeAddendumAndCarryForward`, `internal/pip` tests; API: `TestFinalWishesAPI` | A real PSC PIP receiver (tested against a local test receiver only) |
| 11 Critical review, QA, phrases, individual intelligence | Rules by call type, modifier, priority, event and phrase; one case per incident; phrase alerts with negation, uncertainty, testing mode and dispositions; full review on demand with milestones, units, tagged radio, requests, QA matrix with the events behind each interval, limitations, versions with hashes; derived records-request and accreditation copies; individual records with breaks apart from locks and review prompts | Go: `TestCriticalCaseOneAlertTwoTriggers`, `internal/phrase` tests; browser: final.spec "critical incident" | Live speech-to-text confidence (the sources do not report it); radio audio links (audio stays in the radio system) |
| 12 Call entry order, comments, keys, open calls | Location first, caller and callback read back, type; Enter adds a comment; answered calls at the position attach themselves; the cursor goes to what is missing; F2 and F5 open a new screen and keep the current one; a sent call stays open until Close call | Browser: entry.spec "call entry", wishes.spec "working calls", final.spec "forced location" | The reviewer's own workstations |
| 13 Persons | First, middle and last name; location; several phones; description; SSN, licence and DOB with a separate approximate age; masked identifiers opened with a recorded purpose; officer-safety concern that must be explained, incident-only or proposed as a permanent warning; first-name search; possible matches | Go: `TestPersonsSearchSafetyAndIdentifiers`; browser: final.spec "officer-safety concern" | |
| 14 Street View | Docked panel checked with Google first; plain status with Retry, aerial photos, map and Return to call; new location after a change; N, E, S, W say which sides have no picture | Browser: final.spec "Street View" (without a key) | Google Street View with an agency key |
| 15 Vehicles and tows | Roles; plate state default; NCIC code lists; VIN check; returns attached beside reported values with discrepancies; person links both ways; per-vehicle tows from the rotation with every company, step times, storage, hold; `SE.<unit>` then F10 | Go: `TestVehiclesReturnsHitConfirmationAndTows`; API: `TestFinalWishesAPI` | A real DMV or NCIC return format (the simulator's is used) |
| 16 Requests, hit confirmation, handoff | One metadata-only queue; hit confirmation from the record with urgent 10 minutes and routine 1 hour; outcomes; overdue warnings; sign-off refused with open requests; handoff and acceptance | Go: `TestRequestsHandoffAndSignOff`, `TestVehiclesReturnsHitConfirmationAndTows`; API: `TestFinalWishesAPI`; browser: final.spec "vehicle run through the simulator" | A live switch |
| 17 Reconciliation | This page; Resources now also open beside the dispatcher's incident | | |

## Wish list 1 items the review called partial

These were built on 24 October. The review found them partial or not verified because they were not
demonstrated end to end. Each now has the automated test named; the reviewer's acceptance test is still the
gate.

| Item | Evidence |
|---|---|
| 1 Saved and locked duty presets | browser: layout.spec; Go: `TestOperationalWishesAPI` (presets follow the person; trainee locked) |
| 2 Agency-specific configuration | Go: `TestOperationalWishesAPI` (agency-limited administrator refused), scope tests |
| 3 Several working calls | browser: wishes.spec "three working calls"; final.spec (F5 keeps the sent call) |
| 4 Communications and BOLO inbox | browser: wishes.spec "BOLO from the inbox"; field.spec "urgent BOLO" |
| 5 Search and recall | Go: `TestWorkingCallsAndSearch`; browser: wishes.spec (BOLO plate found by search) |
| 6 Duty and staffing | browser: wishes.spec "staffing"; Go: `TestStaffingAndQualifications` |
| 7 Qualifications and special teams | Go: `TestStaffingAndQualifications` |
| 8 Keys and commands | see update 8 above |
