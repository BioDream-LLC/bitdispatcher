package guidance

import (
	"strings"
	"testing"
	"time"
)

var ref = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

// doc is a small builder for an approved, in-effect document.
func doc(id, kind, text string) Doc {
	return Doc{
		ID:        id,
		Title:     id + " title",
		Kind:      kind,
		Version:   "1.0",
		Effective: ref.AddDate(0, 0, -30),
		Approved:  true,
		Text:      text,
	}
}

func TestSectionsHeadingsAndAnchors(t *testing.T) {
	d := doc("D1", Guide, "# Top\nintro text\n\n## Child A\nbody a\n\n### Grandchild\nbody g\n\n## Child B\nbody b\n")
	secs := Sections(d)
	if len(secs) != 4 {
		t.Fatalf("want 4 sections, got %d: %+v", len(secs), secs)
	}

	wantHeads := []string{"Top", "Child A", "Grandchild", "Child B"}
	for i, want := range wantHeads {
		if secs[i].Heading != want {
			t.Errorf("section %d heading = %q, want %q", i, secs[i].Heading, want)
		}
	}

	// Path should record the heading trail.
	if got := strings.Join(secs[2].Path, " > "); got != "Top > Child A > Grandchild" {
		t.Errorf("grandchild path = %q", got)
	}

	// Anchors must be unique.
	seen := map[string]bool{}
	for _, s := range secs {
		if s.Anchor == "" {
			t.Errorf("empty anchor for %q", s.Heading)
		}
		if seen[s.Anchor] {
			t.Errorf("duplicate anchor %q", s.Anchor)
		}
		seen[s.Anchor] = true
	}
}

func TestSectionsNoHeadings(t *testing.T) {
	d := doc("D1", Guide, "just some body text\nwith two lines\n")
	secs := Sections(d)
	if len(secs) != 1 {
		t.Fatalf("want 1 section, got %d", len(secs))
	}
	if secs[0].Heading != d.Title {
		t.Errorf("heading = %q, want doc title %q", secs[0].Heading, d.Title)
	}
	if !strings.Contains(secs[0].Text, "just some body text") {
		t.Errorf("body missing: %q", secs[0].Text)
	}
}

func TestSectionsDuplicateHeadingAnchors(t *testing.T) {
	d := doc("D1", Guide, "# Notes\na\n\n# Notes\nb\n\n# Notes\nc\n")
	secs := Sections(d)
	if len(secs) != 3 {
		t.Fatalf("want 3 sections, got %d", len(secs))
	}
	if secs[0].Anchor != "notes" || secs[1].Anchor != "notes-2" || secs[2].Anchor != "notes-3" {
		t.Errorf("anchors = %q, %q, %q", secs[0].Anchor, secs[1].Anchor, secs[2].Anchor)
	}
}

func TestSectionsAppliesLineParsedAndRemoved(t *testing.T) {
	d := doc("D1", Guide, "# Start CPR\nApplies: CARD, BRTH; modifiers: weapons; words: not breathing, cpr\nPush hard and fast.\n")
	secs := Sections(d)
	if len(secs) != 1 {
		t.Fatalf("want 1 section, got %d", len(secs))
	}
	s := secs[0]
	if strings.Contains(s.Text, "Applies:") {
		t.Errorf("Applies line not removed: %q", s.Text)
	}
	if s.Text != "Push hard and fast." {
		t.Errorf("text = %q", s.Text)
	}
	if strings.Join(s.CallTypes, ",") != "CARD,BRTH" {
		t.Errorf("call types = %v", s.CallTypes)
	}
	if strings.Join(s.Modifiers, ",") != "weapons" {
		t.Errorf("modifiers = %v", s.Modifiers)
	}
	if strings.Join(s.Words, ",") != "not breathing,cpr" {
		t.Errorf("words = %v", s.Words)
	}
}

func TestMatchCallTypeModifiersWordsScore(t *testing.T) {
	d := Doc{
		ID: "SOP-1", Title: "Cardiac guide", Kind: Guide, Version: "1.0",
		Effective: ref.AddDate(0, 0, -10), Approved: true,
		CallTypes: []string{"CARD"}, Modifiers: []string{"weapons"},
		Words: []string{"not breathing", "cpr"},
		Text:  "# Body\nhandle it\n",
	}
	cx := Context{CallType: "CARD", Modifiers: []string{"weapons"}, Text: "patient is NOT BREATHING, start CPR now", At: ref}
	res := Match([]Doc{d}, cx, 0)
	if len(res.Matches) != 1 {
		t.Fatalf("want 1 match, got %d: %+v", len(res.Matches), res)
	}
	m := res.Matches[0]
	// 50 (call type) + 20 (modifier) + 10 + 10 (two words) = 90. Guide: no +3.
	if m.Score != 90 {
		t.Errorf("score = %d, want 90; why=%v", m.Score, m.Why)
	}
	if m.Status != "current" {
		t.Errorf("status = %q", m.Status)
	}
	// Why should explain the match.
	joined := strings.Join(m.Why, "|")
	for _, want := range []string{"call type CARD", "modifier weapons", "not breathing", "cpr"} {
		if !strings.Contains(joined, want) {
			t.Errorf("why %q missing %q", joined, want)
		}
	}
}

func TestMatchWholeWordOnly(t *testing.T) {
	d := doc("D1", Guide, "# B\nx\n")
	d.CallTypes = []string{"CARD"}
	d.Words = []string{"cpr"}
	// "cardiopulmonary" contains "cpr"? no. But test a substring trap: "scpry".
	cx := Context{CallType: "CARD", Text: "the word scpry is not cpr related", At: ref}
	res := Match([]Doc{d}, cx, 0)
	// Should match the standalone "cpr" (10) + call type (50) = 60.
	if res.Matches[0].Score != 60 {
		t.Errorf("score = %d, want 60", res.Matches[0].Score)
	}

	cx2 := Context{CallType: "CARD", Text: "the word scpry only", At: ref}
	res2 := Match([]Doc{d}, cx2, 0)
	// No standalone cpr: only call type 50.
	if res2.Matches[0].Score != 50 {
		t.Errorf("score = %d, want 50 (no whole-word cpr)", res2.Matches[0].Score)
	}
}

func TestMatchPolicyTieBreak(t *testing.T) {
	g := doc("G1", Guide, "# B\nx\n")
	g.CallTypes = []string{"CARD"}
	p := doc("P1", Policy, "# B\nx\n")
	p.CallTypes = []string{"CARD"}
	cx := Context{CallType: "CARD", At: ref}
	res := Match([]Doc{g, p}, cx, 0)
	if len(res.Matches) != 2 {
		t.Fatalf("want 2, got %d", len(res.Matches))
	}
	// Policy gets +3 so scores 53 vs 50, and sorts first.
	if res.Matches[0].Doc != "P1" || res.Matches[0].Score != 53 {
		t.Errorf("first = %s score %d, want P1 53", res.Matches[0].Doc, res.Matches[0].Score)
	}
	if res.Matches[1].Doc != "G1" || res.Matches[1].Score != 50 {
		t.Errorf("second = %s score %d, want G1 50", res.Matches[1].Doc, res.Matches[1].Score)
	}
}

func TestMatchAlways(t *testing.T) {
	a := doc("A1", Guide, "# Caller safety\nkeep them safe\n")
	a.Always = true
	cx := Context{CallType: "ANYTHING", At: ref}
	res := Match([]Doc{a}, cx, 0)
	if len(res.Matches) != 1 || res.Matches[0].Score != 5 {
		t.Fatalf("want 1 match score 5, got %+v", res.Matches)
	}
	if !hasReason(res.Matches[0].Why, "always") {
		t.Errorf("why missing 'always': %v", res.Matches[0].Why)
	}
}

func TestMatchSectionOverridesDoc(t *testing.T) {
	// Doc maps to CARD; one section narrows to STROKE and should NOT match CARD.
	d := Doc{
		ID: "D1", Title: "T", Kind: Guide, Version: "1.0",
		Effective: ref.AddDate(0, 0, -5), Approved: true,
		CallTypes: []string{"CARD"},
		Text:      "# General\nuses doc mapping\n\n# Stroke only\nApplies: STROKE\nstroke specifics\n",
	}
	cx := Context{CallType: "CARD", At: ref}
	res := Match([]Doc{d}, cx, 0)
	if len(res.Matches) != 1 {
		t.Fatalf("want 1 match (General only), got %d: %+v", len(res.Matches), res.Matches)
	}
	if res.Matches[0].Heading != "General" {
		t.Errorf("matched %q, want General", res.Matches[0].Heading)
	}

	// With STROKE, only the overriding section matches.
	cx2 := Context{CallType: "STROKE", At: ref}
	res2 := Match([]Doc{d}, cx2, 0)
	if len(res2.Matches) != 1 || res2.Matches[0].Heading != "Stroke only" {
		t.Fatalf("want Stroke only section, got %+v", res2.Matches)
	}
}

func TestMatchUnapprovedExcluded(t *testing.T) {
	d := doc("D1", Guide, "# B\nx\n")
	d.CallTypes = []string{"CARD"}
	d.Approved = false
	res := Match([]Doc{d}, Context{CallType: "CARD", At: ref}, 0)
	if len(res.Matches) != 0 {
		t.Fatalf("unapproved should not match, got %+v", res.Matches)
	}
	if !res.NoMatch || res.Note == "" {
		t.Errorf("want NoMatch with note, got %+v", res)
	}
}

func TestMatchFutureExcluded(t *testing.T) {
	d := doc("D1", Guide, "# B\nx\n")
	d.CallTypes = []string{"CARD"}
	d.Effective = ref.AddDate(0, 0, 10) // future
	res := Match([]Doc{d}, Context{CallType: "CARD", At: ref}, 0)
	if len(res.Matches) != 0 {
		t.Fatalf("future doc should not match, got %+v", res.Matches)
	}
	if !res.NoMatch {
		t.Errorf("want NoMatch")
	}
}

func TestMatchExpiredOutdated(t *testing.T) {
	d := doc("D1", Guide, "# B\nx\n")
	d.CallTypes = []string{"CARD"}
	d.Expires = ref.AddDate(0, 0, -1) // expired yesterday
	res := Match([]Doc{d}, Context{CallType: "CARD", At: ref}, 0)
	if len(res.Matches) != 1 {
		t.Fatalf("expired doc should still be returned, got %+v", res.Matches)
	}
	if res.Matches[0].Status != "outdated" {
		t.Errorf("status = %q, want outdated", res.Matches[0].Status)
	}
	if !res.NoMatch {
		t.Errorf("want NoMatch since only outdated matched")
	}
}

func TestMatchSupersededOutdatedAndOrder(t *testing.T) {
	old := doc("OLD", Policy, "# B\nold text\n")
	old.CallTypes = []string{"CARD"}
	newer := doc("NEW", Policy, "# B\nnew text\n")
	newer.CallTypes = []string{"CARD"}
	newer.Supersedes = "OLD"

	res := Match([]Doc{old, newer}, Context{CallType: "CARD", At: ref}, 0)
	if len(res.Matches) != 2 {
		t.Fatalf("want 2 matches, got %d", len(res.Matches))
	}
	// Current one first.
	if res.Matches[0].Doc != "NEW" || res.Matches[0].Status != "current" {
		t.Errorf("first = %s/%s, want NEW/current", res.Matches[0].Doc, res.Matches[0].Status)
	}
	if res.Matches[1].Doc != "OLD" || res.Matches[1].Status != "outdated" {
		t.Errorf("second = %s/%s, want OLD/outdated", res.Matches[1].Doc, res.Matches[1].Status)
	}
	if res.NoMatch {
		t.Errorf("should not be NoMatch: a current passage exists")
	}
	// The outdated OLD must not count as a conflict partner for NEW.
	if res.Matches[0].Conflict != "" {
		t.Errorf("NEW should have no conflict (OLD is outdated), got %q", res.Matches[0].Conflict)
	}
}

func TestMatchConflictBetweenTwoPolicies(t *testing.T) {
	p1 := doc("POL-A", Policy, "# B\na\n")
	p1.CallTypes = []string{"DOM"}
	p2 := doc("POL-B", Policy, "# B\nb\n")
	p2.CallTypes = []string{"DOM"}
	res := Match([]Doc{p1, p2}, Context{CallType: "DOM", At: ref}, 0)
	if len(res.Matches) != 2 {
		t.Fatalf("want 2, got %d", len(res.Matches))
	}
	for _, m := range res.Matches {
		if m.Conflict == "" {
			t.Errorf("%s has no conflict set", m.Doc)
		}
		if m.Conflict == m.Doc {
			t.Errorf("%s conflicts with itself", m.Doc)
		}
	}
	// They should name each other.
	byID := map[string]Passage{}
	for _, m := range res.Matches {
		byID[m.Doc] = m
	}
	if byID["POL-A"].Conflict != "POL-B" || byID["POL-B"].Conflict != "POL-A" {
		t.Errorf("conflicts = A:%q B:%q", byID["POL-A"].Conflict, byID["POL-B"].Conflict)
	}
}

func TestMatchNoConflictForGuides(t *testing.T) {
	g1 := doc("G-A", Guide, "# B\na\n")
	g1.CallTypes = []string{"DOM"}
	g2 := doc("G-B", Guide, "# B\nb\n")
	g2.CallTypes = []string{"DOM"}
	res := Match([]Doc{g1, g2}, Context{CallType: "DOM", At: ref}, 0)
	for _, m := range res.Matches {
		if m.Conflict != "" {
			t.Errorf("guide %s should not flag conflict, got %q", m.Doc, m.Conflict)
		}
	}
}

func TestMatchNoMatchAndNote(t *testing.T) {
	d := doc("D1", Guide, "# B\nx\n")
	d.CallTypes = []string{"CARD"}
	res := Match([]Doc{d}, Context{CallType: "FIRE", At: ref}, 0)
	if len(res.Matches) != 0 {
		t.Fatalf("want 0 matches, got %+v", res.Matches)
	}
	if !res.NoMatch {
		t.Errorf("want NoMatch")
	}
	if res.Note != "No approved guidance found for this call type." {
		t.Errorf("note = %q", res.Note)
	}
}

func TestMatchZeroScoreExcluded(t *testing.T) {
	// Doc applies to CARD only; a BRTH incident gives zero score.
	d := doc("D1", Guide, "# One\nApplies: CARD\na\n\n# Two\nApplies: BRTH\nb\n")
	res := Match([]Doc{d}, Context{CallType: "BRTH", At: ref}, 0)
	if len(res.Matches) != 1 || res.Matches[0].Heading != "Two" {
		t.Fatalf("want only Two, got %+v", res.Matches)
	}
}

func TestMatchLimit(t *testing.T) {
	var docs []Doc
	for i := 0; i < 5; i++ {
		d := doc("D"+itoa(i), Guide, "# B\nx\n")
		d.CallTypes = []string{"CARD"}
		docs = append(docs, d)
	}
	res := Match(docs, Context{CallType: "CARD", At: ref}, 3)
	if len(res.Matches) != 3 {
		t.Fatalf("limit 3: got %d", len(res.Matches))
	}
	// Default limit kicks in for <= 0.
	res2 := Match(docs, Context{CallType: "CARD", At: ref}, 0)
	if len(res2.Matches) != 5 {
		t.Fatalf("default limit: got %d", len(res2.Matches))
	}
}

func TestMatchDeterministicOrder(t *testing.T) {
	// Equal score, differing kinds and IDs: check the full tie-break chain.
	mk := func(id, kind string) Doc {
		d := doc(id, kind, "# B\nx\n")
		d.CallTypes = []string{"CARD"}
		return d
	}
	docs := []Doc{
		mk("Z-GUIDE", Guide),
		mk("A-GUIDE", Guide),
		mk("M-CHECK", Checklist),
		mk("M-PROMPT", Prompt),
	}
	// Run several times; order must not vary.
	var first []string
	for run := 0; run < 5; run++ {
		res := Match(docs, Context{CallType: "CARD", At: ref}, 0)
		var got []string
		for _, m := range res.Matches {
			got = append(got, m.Doc)
		}
		if run == 0 {
			first = got
			continue
		}
		if strings.Join(got, ",") != strings.Join(first, ",") {
			t.Fatalf("order varied: %v vs %v", first, got)
		}
	}
	// Expected: checklist (M-CHECK) before guides, guides before prompt;
	// guides ordered by ID. Policy would be first but there is none here.
	want := []string{"M-CHECK", "A-GUIDE", "Z-GUIDE", "M-PROMPT"}
	if strings.Join(first, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", first, want)
	}
}

func TestMatchAtDefaultsToNow(t *testing.T) {
	d := doc("D1", Guide, "# B\nx\n")
	d.CallTypes = []string{"CARD"}
	d.Effective = time.Now().AddDate(0, 0, -1)
	res := Match([]Doc{d}, Context{CallType: "CARD"}, 0) // zero At -> now
	if len(res.Matches) != 1 {
		t.Fatalf("want 1 match with default now, got %+v", res.Matches)
	}
}

func TestValidate(t *testing.T) {
	good := doc("D1", Guide, "# B\nx\n")
	if p := Validate(good); len(p) != 0 {
		t.Errorf("good doc has problems: %v", p)
	}

	bad := Doc{Kind: "nonsense"}
	p := Validate(bad)
	joined := strings.Join(p, "|")
	for _, want := range []string{"ID", "title", "version", "effective date", "text"} {
		if !strings.Contains(joined, want) {
			t.Errorf("problems %q missing mention of %q", joined, want)
		}
	}
	if !strings.Contains(joined, "policy, guide, prompt or checklist") {
		t.Errorf("problems %q missing kind message", joined)
	}

	// Known-kind empty-text case still reports missing text (not kind).
	noText := Doc{ID: "X", Title: "T", Kind: Guide, Version: "1.0", Effective: ref}
	if p := Validate(noText); len(p) != 1 || !strings.Contains(p[0], "text") {
		t.Errorf("want single text problem, got %v", p)
	}
}

func TestExamplesAreUnapprovedAndValidShape(t *testing.T) {
	ex := Examples()
	if len(ex) != 2 {
		t.Fatalf("want 2 examples, got %d", len(ex))
	}
	for _, d := range ex {
		if d.Approved {
			t.Errorf("example %s must not be approved", d.ID)
		}
		if !strings.Contains(d.Title, "EXAMPLE") {
			t.Errorf("example %s title not clearly marked: %q", d.ID, d.Title)
		}
		if len(d.Text) >= 1200 {
			t.Errorf("example %s text is %d chars, want under 1200", d.ID, len(d.Text))
		}
		// Shape must validate (ID/title/kind/version/effective/text/sections).
		if p := Validate(d); len(p) != 0 {
			t.Errorf("example %s does not validate: %v", d.ID, p)
		}
	}

	// Examples never surface through Match, being unapproved.
	cx := Context{CallType: "CARD", Modifiers: []string{"weapons"}, Text: "not breathing, caller reports weapons", At: ref}
	if res := Match(ex, cx, 0); len(res.Matches) != 0 {
		t.Errorf("examples should never match, got %+v", res.Matches)
	}
}

func TestExamplesParseSectionsWithApplies(t *testing.T) {
	ex := Examples()
	card := ex[0]
	secs := Sections(card)
	// Find the "Start CPR" section and confirm its section-level mapping.
	var found bool
	for _, s := range secs {
		if s.Heading == "Start CPR" {
			found = true
			if !containsFold(s.CallTypes, "CARD") {
				t.Errorf("Start CPR call types = %v", s.CallTypes)
			}
			if !containsFold(s.Words, "not breathing") {
				t.Errorf("Start CPR words = %v", s.Words)
			}
			if strings.Contains(s.Text, "Applies:") {
				t.Errorf("Applies line leaked into text: %q", s.Text)
			}
		}
	}
	if !found {
		t.Errorf("Start CPR section not found in example")
	}
}
