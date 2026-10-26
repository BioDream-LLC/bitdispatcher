package phrase

import (
	"strings"
	"testing"
)

// rule helpers ---------------------------------------------------------------

func shotsFired() Rule {
	return Rule{
		ID:       "shots-fired",
		Name:     "Shots fired",
		Phrases:  []string{"shots fired", "shot fired", "gun shots", "shoot*"},
		Severity: "urgent",
	}
}

func findMatch(ms []Match, ruleID string) (Match, bool) {
	for _, m := range ms {
		if m.Rule == ruleID {
			return m, true
		}
	}
	return Match{}, false
}

// basic match ----------------------------------------------------------------

func TestBasicMatch(t *testing.T) {
	ms := Check([]Rule{shotsFired()}, Text{Text: "We have shots fired at the scene."})
	if len(ms) != 1 {
		t.Fatalf("want 1 match, got %d: %+v", len(ms), ms)
	}
	m := ms[0]
	if m.Rule != "shots-fired" {
		t.Errorf("rule = %q", m.Rule)
	}
	if m.Found != "shots fired" {
		t.Errorf("found = %q, want %q", m.Found, "shots fired")
	}
	if m.Phrase != "shots fired" {
		t.Errorf("phrase = %q", m.Phrase)
	}
	if m.Severity != "urgent" {
		t.Errorf("severity = %q, want urgent", m.Severity)
	}
	if m.Negated {
		t.Errorf("unexpectedly negated")
	}
}

func TestDefaultSeverity(t *testing.T) {
	r := shotsFired()
	r.Severity = ""
	ms := Check([]Rule{r}, Text{Text: "shots fired"})
	if len(ms) != 1 || ms[0].Severity != "high" {
		t.Fatalf("want default high severity, got %+v", ms)
	}
}

// case / punctuation ---------------------------------------------------------

func TestCaseAndPunctuation(t *testing.T) {
	ms := Check([]Rule{shotsFired()}, Text{Text: "Shots-fired!"})
	if len(ms) != 1 {
		t.Fatalf("want 1 match, got %d: %+v", len(ms), ms)
	}
	if ms[0].Found != "Shots-fired" {
		t.Errorf("found = %q, want %q", ms[0].Found, "Shots-fired")
	}
}

// wildcard -------------------------------------------------------------------

func TestWildcard(t *testing.T) {
	r := Rule{ID: "shoot", Name: "Shoot", Phrases: []string{"shoot*"}}
	for _, word := range []string{"shooting", "shooter", "shoot", "shootout"} {
		ms := Check([]Rule{r}, Text{Text: "there was a " + word + " downtown"})
		if len(ms) != 1 {
			t.Errorf("%q: want 1 match, got %d", word, len(ms))
			continue
		}
		if ms[0].Found != word {
			t.Errorf("%q: found = %q", word, ms[0].Found)
		}
	}
	// A non-prefix word must not match.
	ms := Check([]Rule{r}, Text{Text: "we reshoot the scene"})
	// "reshoot" does not start with "shoot", so no match.
	if len(ms) != 0 {
		t.Errorf("want no match for 'reshoot', got %+v", ms)
	}
}

// multi-variant longest collapse ---------------------------------------------

func TestMultiVariantLongestCollapse(t *testing.T) {
	// Two overlapping variants of the SAME rule at the same place collapse to
	// the longest.
	r := Rule{
		ID:      "ofc",
		Name:    "Officer",
		Phrases: []string{"officer", "officer needs assistance"},
	}
	ms := Check([]Rule{r}, Text{Text: "officer needs assistance now"})
	if len(ms) != 1 {
		t.Fatalf("want 1 collapsed match, got %d: %+v", len(ms), ms)
	}
	if ms[0].Found != "officer needs assistance" {
		t.Errorf("found = %q, want longest variant", ms[0].Found)
	}
}

// negation before ------------------------------------------------------------

func TestNegationBefore(t *testing.T) {
	ms := Check([]Rule{shotsFired()}, Text{Text: "there are no shots fired"})
	if len(ms) != 1 {
		t.Fatalf("want 1 match, got %d: %+v", len(ms), ms)
	}
	if !ms[0].Negated {
		t.Errorf("want negated, got %+v", ms[0])
	}
	if ms[0].Negation != "no" {
		t.Errorf("negation = %q, want %q", ms[0].Negation, "no")
	}
}

func TestNegationPreviousSentenceDoesNotApply(t *testing.T) {
	ms := Check([]Rule{shotsFired()}, Text{Text: "No. Shots fired now"})
	if len(ms) != 1 {
		t.Fatalf("want 1 match, got %d: %+v", len(ms), ms)
	}
	if ms[0].Negated {
		t.Errorf("negation in previous sentence must not apply: %+v", ms[0])
	}
}

func TestNegationAfterDoesNotApply(t *testing.T) {
	// "no bomb found": negation BEFORE negates; here test the opposite: a
	// negation word AFTER the match must not negate.
	r := Rule{ID: "bomb", Name: "Bomb", Phrases: []string{"bomb"}}
	ms := Check([]Rule{r}, Text{Text: "bomb, no doubt"})
	if len(ms) != 1 {
		t.Fatalf("want 1 match, got %d", len(ms))
	}
	if ms[0].Negated {
		t.Errorf("negation after match must not negate: %+v", ms[0])
	}
	// And the "no bomb found" case IS negated.
	ms = Check([]Rule{r}, Text{Text: "no bomb found"})
	if len(ms) != 1 || !ms[0].Negated {
		t.Fatalf("want negated match, got %+v", ms)
	}
}

func TestNegationWindow(t *testing.T) {
	// Default window is 3. A negation 4 words before should not negate.
	ms := Check([]Rule{shotsFired()}, Text{Text: "no one could really confirm shots fired"})
	if len(ms) != 1 {
		t.Fatalf("want 1 match, got %d: %+v", len(ms), ms)
	}
	if ms[0].Negated {
		t.Errorf("negation outside window must not negate: %+v", ms[0])
	}
}

// not breathing not self-negated ---------------------------------------------

func TestChildNotBreathingNotSelfNegated(t *testing.T) {
	rules := Examples()
	ms := Check(rules, Text{Text: "the child is not breathing"})
	m, ok := findMatch(ms, "child-not-breathing")
	if !ok {
		t.Fatalf("want child-not-breathing match, got %+v", ms)
	}
	if m.Negated {
		t.Errorf("'not' is part of the phrase and must not self-negate: %+v", m)
	}
	if m.Found != "child is not breathing" {
		t.Errorf("found = %q", m.Found)
	}
}

func TestNotBreathingNotSelfNegated(t *testing.T) {
	rules := Examples()
	ms := Check(rules, Text{Text: "patient is not breathing"})
	m, ok := findMatch(ms, "not-breathing")
	if !ok {
		t.Fatalf("want not-breathing match, got %+v", ms)
	}
	if m.Negated {
		t.Errorf("'not breathing' must not self-negate: %+v", m)
	}
}

func TestNoCPRNegated(t *testing.T) {
	rules := Examples()
	ms := Check(rules, Text{Text: "no CPR in progress"})
	m, ok := findMatch(ms, "not-breathing")
	if !ok {
		t.Fatalf("want not-breathing (CPR) match, got %+v", ms)
	}
	if !m.Negated {
		t.Errorf("'no CPR in progress' should be negated: %+v", m)
	}
	// Longest collapse: should match "cpr in progress", not just "cpr".
	if m.Found != "CPR in progress" {
		t.Errorf("found = %q, want longest 'CPR in progress'", m.Found)
	}
	if m.Negation != "no" {
		t.Errorf("negation = %q", m.Negation)
	}
}

// source / calltype filtering ------------------------------------------------

func TestSourceFiltering(t *testing.T) {
	r := shotsFired()
	r.Sources = []string{"radio"}
	// Matching source.
	ms := Check([]Rule{r}, Text{Source: "radio", Text: "shots fired"})
	if len(ms) != 1 {
		t.Errorf("radio source should match, got %d", len(ms))
	}
	// Excluded source.
	ms = Check([]Rule{r}, Text{Source: "call", Text: "shots fired"})
	if len(ms) != 0 {
		t.Errorf("call source should be excluded, got %+v", ms)
	}
}

func TestCallTypeFiltering(t *testing.T) {
	r := shotsFired()
	r.CallTypes = []string{"shooting"}
	ms := Check([]Rule{r}, Text{CallType: "shooting", Text: "shots fired"})
	if len(ms) != 1 {
		t.Errorf("matching call type should match, got %d", len(ms))
	}
	ms = Check([]Rule{r}, Text{CallType: "traffic", Text: "shots fired"})
	if len(ms) != 0 {
		t.Errorf("non-matching call type should be excluded, got %+v", ms)
	}
}

func TestOffRuleSkipped(t *testing.T) {
	r := shotsFired()
	r.Off = true
	ms := Check([]Rule{r}, Text{Text: "shots fired"})
	if len(ms) != 0 {
		t.Errorf("Off rule should be skipped, got %+v", ms)
	}
}

// confidence uncertain -------------------------------------------------------

func TestConfidenceUncertain(t *testing.T) {
	r := shotsFired()
	r.MinConfidence = 0.8
	// Below threshold and known -> uncertain (still returned).
	ms := Check([]Rule{r}, Text{Text: "shots fired", Confidence: 0.5})
	if len(ms) != 1 || !ms[0].Uncertain {
		t.Fatalf("want uncertain match, got %+v", ms)
	}
	// Above threshold -> not uncertain.
	ms = Check([]Rule{r}, Text{Text: "shots fired", Confidence: 0.9})
	if len(ms) != 1 || ms[0].Uncertain {
		t.Fatalf("want certain match, got %+v", ms)
	}
	// Unknown confidence (0) -> not uncertain.
	ms = Check([]Rule{r}, Text{Text: "shots fired", Confidence: 0})
	if len(ms) != 1 || ms[0].Uncertain {
		t.Fatalf("unknown confidence should not be uncertain, got %+v", ms)
	}
}

// testing flag ---------------------------------------------------------------

func TestTestingFlag(t *testing.T) {
	r := shotsFired()
	r.Testing = true
	ms := Check([]Rule{r}, Text{Text: "shots fired"})
	if len(ms) != 1 || !ms[0].Testing {
		t.Fatalf("want Testing=true on match, got %+v", ms)
	}
}

// Validate errors ------------------------------------------------------------

func TestValidate(t *testing.T) {
	// A fully valid rule produces no problems.
	if p := Validate(shotsFired()); len(p) != 0 {
		t.Errorf("valid rule should have no problems, got %v", p)
	}

	bad := Rule{
		Phrases:        []string{"", "sho*ts"},
		Severity:       "extreme",
		NegationWindow: 11,
	}
	p := Validate(bad)
	joined := strings.Join(p, " | ")
	wantContains := []string{
		"no ID",
		"no name",
		"empty phrase",
		"wildcard",
		"severity",
		"negation window",
	}
	for _, w := range wantContains {
		if !strings.Contains(joined, w) {
			t.Errorf("Validate output missing %q; got: %s", w, joined)
		}
	}

	// No phrases.
	p = Validate(Rule{ID: "x", Name: "X"})
	if !contains(p, "no phrases") {
		t.Errorf("want 'no phrases' problem, got %v", p)
	}
}

func contains(ss []string, sub string) bool {
	for _, s := range ss {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// context ellipsis -----------------------------------------------------------

func TestContextEllipsis(t *testing.T) {
	// Long text on both sides -> both ellipses.
	long := "one two three four five six seven eight nine ten shots fired eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty"
	ms := Check([]Rule{shotsFired()}, Text{Text: long})
	if len(ms) != 1 {
		t.Fatalf("want 1 match, got %d", len(ms))
	}
	ctx := ms[0].Context
	if !strings.HasPrefix(ctx, "…") {
		t.Errorf("context should start with ellipsis: %q", ctx)
	}
	if !strings.HasSuffix(ctx, "…") {
		t.Errorf("context should end with ellipsis: %q", ctx)
	}
	if !strings.Contains(ctx, "shots fired") {
		t.Errorf("context should contain the match: %q", ctx)
	}

	// Short text -> no ellipses.
	ms = Check([]Rule{shotsFired()}, Text{Text: "shots fired now"})
	if strings.Contains(ms[0].Context, "…") {
		t.Errorf("short context should have no ellipsis: %q", ms[0].Context)
	}
}

// byte offsets ---------------------------------------------------------------

func TestByteOffsets(t *testing.T) {
	s := "We have shots fired at the scene."
	ms := Check([]Rule{shotsFired()}, Text{Text: s})
	if len(ms) != 1 {
		t.Fatalf("want 1 match, got %d", len(ms))
	}
	m := ms[0]
	if s[m.Start:m.End] != m.Found {
		t.Errorf("offsets s[%d:%d]=%q != Found %q", m.Start, m.End, s[m.Start:m.End], m.Found)
	}
	if m.Found != "shots fired" {
		t.Errorf("found = %q", m.Found)
	}
}

func TestByteOffsetsNonASCII(t *testing.T) {
	// Multi-byte runes before the match must be accounted for in byte offsets.
	s := "Señor Núñez reports — shots fired café"
	ms := Check([]Rule{shotsFired()}, Text{Text: s})
	if len(ms) != 1 {
		t.Fatalf("want 1 match, got %d: %+v", len(ms), ms)
	}
	m := ms[0]
	if s[m.Start:m.End] != "shots fired" {
		t.Errorf("offsets s[%d:%d]=%q, want %q", m.Start, m.End, s[m.Start:m.End], "shots fired")
	}
	if m.Found != "shots fired" {
		t.Errorf("found = %q", m.Found)
	}
}

// ordering --------------------------------------------------------------------

func TestOrderingByStartThenRule(t *testing.T) {
	r1 := Rule{ID: "a-rule", Name: "A", Phrases: []string{"fire"}}
	r2 := Rule{ID: "b-rule", Name: "B", Phrases: []string{"fire"}}
	// Both rules match at the same position; order by rule ID.
	ms := Check([]Rule{r2, r1}, Text{Text: "fire"})
	if len(ms) != 2 {
		t.Fatalf("want 2 matches, got %d: %+v", len(ms), ms)
	}
	if ms[0].Rule != "a-rule" || ms[1].Rule != "b-rule" {
		t.Errorf("want ordered by rule ID, got %q then %q", ms[0].Rule, ms[1].Rule)
	}

	// Different positions: order by Start.
	ms = Check([]Rule{shotsFired(), {ID: "down", Name: "Down", Phrases: []string{"down"}}},
		Text{Text: "officer is down and shots fired"})
	if len(ms) != 2 {
		t.Fatalf("want 2 matches, got %d: %+v", len(ms), ms)
	}
	if ms[0].Rule != "down" {
		t.Errorf("want 'down' first by Start, got %+v", ms)
	}
}

// Examples -------------------------------------------------------------------

func TestExamplesAllTesting(t *testing.T) {
	ex := Examples()
	if len(ex) == 0 {
		t.Fatal("Examples returned nothing")
	}
	for _, r := range ex {
		if !r.Testing {
			t.Errorf("example %q should be Testing=true", r.ID)
		}
		if p := Validate(r); len(p) != 0 {
			t.Errorf("example %q should validate cleanly, got %v", r.ID, p)
		}
	}
	// Spot-check required rules exist.
	ids := map[string]bool{}
	for _, r := range ex {
		ids[r.ID] = true
	}
	for _, want := range []string{"shots-fired", "officer-down", "bomb", "not-breathing", "child-not-breathing"} {
		if !ids[want] {
			t.Errorf("Examples missing rule %q", want)
		}
	}
}
