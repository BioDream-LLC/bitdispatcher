# The non-emergency intake

A voice or text bot on the non-emergency line (BitVoice's AI receptionist, or any other) can take reports.
The bot carries the voice: speech to text and text to speech, or the text messages. bitdispatcher decides
what it says: the agency's questions, one at a time, and when to hand the caller to a person. A finished
report is a call in the non-emergency list for a person to act on. Nothing is dispatched by it.

Set it up in Admin › Non-emergency intake: switch it on, write the questions, and make the bot's token.

## The conversation

Start one when the call is answered:

```
POST /api/intake/sessions
Authorization: Bearer <the intake token>
{ "channel": "voice", "from": "+15405550100" }
```

```json
{ "session": "3f9c…", "turn": { "say": "You have reached the non-emergency line. If this is an emergency, say emergency now, or hang up and dial 911. Is anyone hurt or in danger right now?", "done": false, "transfer": false, "ask": "danger", "kind": "yesno" } }
```

Then send each thing the caller says:

```
POST /api/intake/sessions/<session>
Authorization: Bearer <the intake token>
{ "text": "No, nobody is hurt" }
```

```json
{ "turn": { "say": "Briefly, what would you like to report?", "done": false, "transfer": false, "ask": "what", "kind": "text" } }
```

Say `turn.say` to the caller. `kind` (text, address, phone, yesno) says what the next answer is, for the
bot's speech recogniser. When `done` is true the conversation is over:

- `transfer: true`: say `turn.say` (the hand-over line) and **transfer the caller to a call-taker now**.
  `reason` says why. bitdispatcher has made a call marked emergency with what was said so far.
- `transfer: false`: say `turn.say` (the closing, with the report number) and end the call. The answer
  also carries `call` (the report's call ID) and `ref` (its number).

A conversation with no word for 30 minutes is forgotten. At most 200 run at once; beyond that, or when the
intake is switched off, starting one answers 503: hand the caller to a person.

## When the caller is handed over

- They say one of the emergency words (the agency's list, or by default: emergency, help me, hurt,
  injured, bleeding, can't breathe, chest pain, unconscious, fire, smoke, gun, knife, weapon, shot,
  stabbed, breaking in, right now, happening now, attack, hitting me, threatening, overdose, suicide,
  kill, dying, not breathing, ambulance, and the agency's critical words). A word within three words after
  *no*, *not*, *nobody*, *none*, *never*, *without*, *nothing* or a *n't* does not count ("nobody is hurt").
- bitdispatcher's transcript reader marks something urgent (a weapon, not breathing).
- They answer yes to a question marked *yes = hand over*, or give no clear answer to it twice.

When in doubt the intake hands over: a person is always the safe answer.

## The report

The report is a call in the non-emergency list (`source: intake`, line *non-emergency intake*) with every
answer as a note, the conversation as its transcript, the callback (the `phone` answer, else `from`), the
caller (`name`), and the place (`where`, placed on the map when the recorded map finds the address
exactly). It stays in the list until a person makes an incident of it or ends it; it does not time out like
outside alerts do.

## Testing

`internal/cad/next_test.go` (the conversation) and `internal/api/next_test.go` (a bot with the token) run
in `make test`; Admin › Non-emergency intake › Try it runs the saved script in the browser and makes no
report. It has not been run with BitVoice's AI receptionist or another voice bot, or with real callers.
