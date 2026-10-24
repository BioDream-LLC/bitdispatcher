# Roadmap

What large CAD systems commonly have, in the order it was built. All eleven were built in October 2026
(see the manual's part *Fire ground, crews and the police desk*); what remains is the last section.

| # | Feature | Built |
|---|---|---|
| 1 | Second and third alarms | Yes: one command upgrades a fire to the next alarm level and sends that level of the run card |
| 2 | Crews and staffing | Yes: who is riding each unit this shift, staffing minimums per unit, crews moved between units |
| 3 | Paging responders | Yes: pages to volunteer and off-duty crews by text, push and email; IAmResponding and Active911 |
| 4 | Fire-ground accountability | Yes: roll-call (PAR) timers and the crews on scene at a working fire |
| 5 | Tow rotation | Yes: the police wrecker rotation, with skips and its history |
| 6 | Scheduled calls and special events | Yes: planned transports and events, incidents prepared ahead to start at a set time |
| 7 | Contacts directory | Yes: utilities, agencies and on-call lists, with a call-out list per incident type |
| 8 | Geofences | Yes: alerts when a unit enters or leaves an area; caution notes by area |
| 9 | Unit status history | Yes: one unit's statuses over a shift or a day, from the ledger |
| 10 | Patient reports (NEMSIS) | Yes: the incident's times and details sent to the ambulance crews' patient-care reports |
| 11 | Warrants and civil process | Yes: warrants and papers to serve, with the address and the person |

## Still to do

Not only code: licensed medical-dispatch protocols (bitdispatcher has its own protocol engine; the scripts
most centres use are licensed), each state's NCIC switch and NIBRS submission (the state's approval; the
NIBRS file is checked with SEARCH's validator in the FBI's layout, and NCIC against our own simulator), and
IPAWS itself for public alerts (FEMA's agreement: bitdispatcher writes and checks the messages to its
profile, publishes a CAP feed and sends to CAP receivers and subscriber lists).
