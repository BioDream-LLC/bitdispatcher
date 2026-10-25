package cad

// The non-emergency intake: a short conversation that takes a non-emergency report, for a voice or text
// bot on the non-emergency line (BitVoice's AI receptionist, or any other) or a person testing the script.
// The bot carries the voice; bitdispatcher decides what to ask next. It asks the agency's questions one at
// a time, and the moment the caller says anything that sounds like an emergency, or answers yes to a
// danger question, it stops and says to transfer the caller to a call-taker. A finished report becomes a
// call in the non-emergency list, with every answer, for a person to act on: nothing is dispatched by it.

import (
	"strings"

	"github.com/biodream-llc/bitdispatcher/internal/assist"
)

// IntakeLine is the line an intake report is on.
const IntakeLine = "non-emergency intake"

// IntakeScript is the conversation.
type IntakeScript struct {
	On        bool             `json:"on,omitempty"`
	Greeting  string           `json:"greeting,omitempty"`
	Questions []IntakeQuestion `json:"questions,omitempty"`
	Closing   string           `json:"closing,omitempty"`  // {ref} is the report's reference
	Transfer  string           `json:"transfer,omitempty"` // said before handing the caller to a person
	// EmergencyWords hand the caller to a person at once (empty: the defaults, with the critical words).
	EmergencyWords []string `json:"emergencyWords,omitempty"`
}

// IntakeQuestion is one question.
type IntakeQuestion struct {
	Key  string `json:"key"`
	Ask  string `json:"ask"`
	Kind string `json:"kind"` // text, address, phone or yesno
	// Required asks again (once) when the answer is empty. Danger: a yes hands the caller to a person.
	Required bool `json:"required,omitempty"`
	Danger   bool `json:"danger,omitempty"`
}

// DefaultIntake is the script until the agency writes its own.
func DefaultIntake() IntakeScript {
	return IntakeScript{
		Greeting: "You have reached the non-emergency line. If this is an emergency, say emergency now, or hang up and dial 911.",
		Questions: []IntakeQuestion{
			{Key: "danger", Ask: "Is anyone hurt or in danger right now?", Kind: "yesno", Required: true, Danger: true},
			{Key: "what", Ask: "Briefly, what would you like to report?", Kind: "text", Required: true},
			{Key: "where", Ask: "What is the address or place where this happened?", Kind: "address", Required: true},
			{Key: "when", Ask: "When did it happen?", Kind: "text"},
			{Key: "name", Ask: "What is your name?", Kind: "text"},
			{Key: "phone", Ask: "What is the best phone number to reach you?", Kind: "phone", Required: true},
		},
		Closing:  "Thank you. Your report number is {ref}. Someone will contact you if more is needed. If anything changes and it becomes an emergency, dial 911.",
		Transfer: "I am connecting you to a 911 call-taker now. Please stay on the line.",
	}
}

// DefaultEmergencyWords hand an intake caller to a person.
var DefaultEmergencyWords = []string{"emergency", "help me", "hurt", "injured", "bleeding", "can't breathe", "cannot breathe", "chest pain",
	"unconscious", "fire", "smoke", "gun", "knife", "weapon", "shot", "stabbed", "breaking in", "break in", "right now", "happening now",
	"attack", "hitting me", "threatening", "overdose", "suicide", "kill", "dying", "not breathing", "ambulance"}

// IntakeOrDefault is the script with its defaults filled in.
func (s Settings) IntakeOrDefault() IntakeScript {
	d := DefaultIntake()
	sc := s.Next.Intake
	if sc.Greeting == "" {
		sc.Greeting = d.Greeting
	}
	if len(sc.Questions) == 0 {
		sc.Questions = d.Questions
	}
	if sc.Closing == "" {
		sc.Closing = d.Closing
	}
	if sc.Transfer == "" {
		sc.Transfer = d.Transfer
	}
	if len(sc.EmergencyWords) == 0 {
		sc.EmergencyWords = append(append([]string{}, DefaultEmergencyWords...), s.criticalWords()...)
	}
	return sc
}

// CheckIntake checks a script an administrator wrote.
func CheckIntake(sc IntakeScript) error {
	if len(sc.Questions) > 20 {
		return problem("intake", "up to 20 questions")
	}
	keys := map[string]bool{}
	for i, q := range sc.Questions {
		switch {
		case strings.TrimSpace(q.Key) == "" || strings.TrimSpace(q.Ask) == "":
			return problem("intake", "question %d needs a key and the words to ask", i+1)
		case keys[q.Key]:
			return problem("intake", "two questions are called %s", q.Key)
		case q.Kind != "text" && q.Kind != "address" && q.Kind != "phone" && q.Kind != "yesno":
			return problem("intake", "question %s: a question is text, address, phone or yes/no", q.Key)
		case q.Danger && q.Kind != "yesno":
			return problem("intake", "question %s: only a yes/no question can hand the caller over", q.Key)
		}
		keys[q.Key] = true
	}
	return nil
}

// IntakeSession is one conversation.
type IntakeSession struct {
	Step    int               `json:"step"`  // the question being asked
	Asked   int               `json:"asked"` // how many times it was asked
	Answers map[string]string `json:"answers"`
	Lines   []TranscriptLine  `json:"lines"`
	Done    bool              `json:"done"`
	Handoff bool              `json:"handoff"` // handed to a person
	Reason  string            `json:"reason,omitempty"`
}

// IntakeTurn is what the bot says next.
type IntakeTurn struct {
	Say      string `json:"say"`
	Done     bool   `json:"done"`
	Transfer bool   `json:"transfer"` // hand the caller to a call-taker now
	Reason   string `json:"reason,omitempty"`
	Ask      string `json:"ask,omitempty"`  // the key of the question asked
	Kind     string `json:"kind,omitempty"` // and its kind, for the bot's speech recogniser
}

// Start is the greeting and the first question.
func (sc IntakeScript) Start(ss *IntakeSession) IntakeTurn {
	ss.Answers = map[string]string{}
	t := sc.ask(ss)
	t.Say = strings.TrimSpace(sc.Greeting + " " + t.Say)
	ss.Lines = append(ss.Lines, TranscriptLine{Speaker: "taker", Text: t.Say})
	return t
}

func (sc IntakeScript) ask(ss *IntakeSession) IntakeTurn {
	q := sc.Questions[ss.Step]
	ss.Asked++
	return IntakeTurn{Say: q.Ask, Ask: q.Key, Kind: q.Kind}
}

// Hear takes what the caller said and returns what to say next.
func (sc IntakeScript) Hear(ss *IntakeSession, said string) IntakeTurn {
	said = strings.TrimSpace(said)
	if len(said) > 2000 {
		said = said[:2000]
	}
	if ss.Done {
		return IntakeTurn{Done: true, Transfer: ss.Handoff, Reason: ss.Reason}
	}
	ss.Lines = append(ss.Lines, TranscriptLine{Speaker: "caller", Text: said})
	say := func(t IntakeTurn) IntakeTurn {
		ss.Lines = append(ss.Lines, TranscriptLine{Speaker: "taker", Text: t.Say})
		return t
	}
	handoff := func(why string) IntakeTurn {
		ss.Done, ss.Handoff, ss.Reason = true, true, why
		return say(IntakeTurn{Say: sc.Transfer, Done: true, Transfer: true, Reason: why})
	}
	if w := sc.emergency(said); w != "" {
		return handoff("the caller said “" + w + "”")
	}
	q := sc.Questions[ss.Step]
	ans := said
	if q.Kind == "yesno" {
		switch yesNo(said) {
		case "yes":
			ans = "yes"
			if q.Danger {
				return handoff("yes to: " + q.Ask)
			}
		case "no":
			ans = "no"
		default:
			ans = ""
		}
	}
	if q.Kind == "phone" && len(digits(said)) < 7 {
		ans = ""
	}
	if ans == "" && (q.Required || q.Kind == "yesno") {
		if ss.Asked < 2 {
			return say(sc.ask(ss))
		}
		if q.Danger { // not sure whether someone is in danger: a person decides
			return handoff("no clear answer to: " + q.Ask)
		}
	}
	ss.Answers[q.Key] = ans
	ss.Step, ss.Asked = ss.Step+1, 0
	if ss.Step >= len(sc.Questions) {
		ss.Done = true
		return IntakeTurn{Done: true} // the closing, with the reference, is said once the report is made
	}
	return say(sc.ask(ss))
}

// emergency is the emergency word a caller said, or "". A word with a negation just before it ("no one
// is hurt", "not bleeding") does not count.
func (sc IntakeScript) emergency(said string) string {
	toks := strings.Fields(strings.Map(func(r rune) rune {
		if strings.ContainsRune(".,!?;:\"“”()", r) {
			return ' '
		}
		return r
	}, strings.ToLower(said)))
	negated := func(at int) bool {
		for k := max(0, at-3); k < at; k++ {
			switch t := toks[k]; t {
			case "no", "not", "nobody", "none", "never", "without", "nothing":
				return true
			default:
				if strings.HasSuffix(t, "n't") || strings.HasSuffix(t, "n’t") {
					return true
				}
			}
		}
		return false
	}
	for _, w := range sc.EmergencyWords {
		ws := strings.Fields(strings.ToLower(w))
		if len(ws) == 0 {
			continue
		}
		for i := 0; i+len(ws) <= len(toks); i++ {
			match := true
			for k, x := range ws {
				if toks[i+k] != x {
					match = false
					break
				}
			}
			if match && !negated(i) {
				return w
			}
		}
	}
	for _, d := range assist.Extract(said) {
		if d.Urgent {
			return d.Text
		}
	}
	return ""
}

func yesNo(s string) string {
	l := strings.ToLower(strings.Trim(strings.TrimSpace(s), ".!?"))
	for _, y := range []string{"yes", "yeah", "yep", "yup", "correct", "sí", "si", "they are", "someone is", "he is", "she is"} {
		if l == y || strings.HasPrefix(l, y+" ") || strings.HasPrefix(l, y+",") {
			return "yes"
		}
	}
	for _, n := range []string{"no", "nope", "nobody", "no one", "not really", "nah", "negative", "everyone is fine", "we're fine", "we are fine"} {
		if l == n || strings.HasPrefix(l, n+" ") || strings.HasPrefix(l, n+",") {
			return "no"
		}
	}
	return ""
}
