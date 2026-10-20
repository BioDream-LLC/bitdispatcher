package foia

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// narrative is a realistic 911 narrative used across the detector tests.
const narrative = "Caller DOE, JANE (DOB 03/14/1988) at 1519 Rosewalk Ln SE, " +
	"callback (540) 555-2348, 15-year-old son overdosed, plate VA KTR-4471, " +
	"VIN 1HGCM82633A004352, email jane.doe@example.com, SSN 123-45-6789, " +
	"license no. B9823451."

// at is a fixed moment for deterministic in-effect checks.
var at = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

// categoryRule builds a single enabled category rule for a test.
func categoryRule(cat string) Rule {
	return Rule{ID: "r-" + cat, Name: cat, Kind: KindCategory, Category: cat, Basis: "b", Explain: "e", Enabled: true}
}

// proposeCat runs one category detector over text and returns its proposals.
func proposeCat(t *testing.T, text, cat string, cx Context) []Proposal {
	t.Helper()
	props, errs := Propose(text, []Rule{categoryRule(cat)}, cx)
	if len(errs) != 0 {
		t.Fatalf("category %q: unexpected errors: %v", cat, errs)
	}
	return props
}

func TestCategories(t *testing.T) {
	got := Categories()
	want := []string{"address", "dl", "dob", "email", "juvenile", "medical", "name", "phone", "plate", "ssn", "vin"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Categories() = %v, want %v", got, want)
	}
	// Must be sorted.
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Fatalf("Categories() not sorted: %v", got)
		}
	}
}

func TestDetectors(t *testing.T) {
	cx := Context{RecordType: "narrative", At: at, Names: []string{"Jane Doe"}}
	tests := []struct {
		cat       string
		text      string
		wantText  string // first proposal's text
		wantConf  string
		wantCount int
	}{
		{"ssn", narrative, "123-45-6789", ConfidenceHigh, 1},
		{"ssn", "SSN 987654321 on file", "987654321", ConfidenceHigh, 1},
		{"ssn", "order number 987654321 here", "", "", 0}, // bare 9 digits, no SSN label
		{"dob", narrative, "03/14/1988", ConfidenceHigh, 1},
		{"dob", "incident on 05/06/2024 was busy", "05/06/2024", ConfidenceReview, 1}, // unlabelled date -> review
		{"phone", narrative, "(540) 555-2348", ConfidenceHigh, 1},
		{"phone", "call +1 540.555.1234 or 540-555-0000", "+1 540.555.1234", ConfidenceHigh, 2},
		{"email", narrative, "jane.doe@example.com", ConfidenceHigh, 1},
		{"plate", narrative, "KTR-4471", ConfidenceHigh, 1},
		{"plate", "tag ABC123 seen", "ABC123", ConfidenceHigh, 1},
		{"vin", narrative, "1HGCM82633A004352", ConfidenceHigh, 1},
		{"vin", "the ABCDEFGHJKLMNPRST word", "", "", 0}, // 17 letters, no digit -> not a VIN
		{"dl", narrative, "B9823451", ConfidenceHigh, 1},
		{"address", narrative, "1519 Rosewalk Ln SE", ConfidenceReview, 1},
		{"address", "went to 42 Baker Street", "42 Baker Street", ConfidenceReview, 1},
		{"juvenile", narrative, "15-year-old", ConfidenceReview, 1},
		{"juvenile", "age 12 and a juvenile present", "", ConfidenceReview, 2}, // age 12 + juvenile
		{"juvenile", "a 40-year-old man", "", "", 0},                           // adult, not under 18
		{"medical", narrative, "overdosed", ConfidenceReview, 1},
		{"medical", "patient had a seizure and is diabetic", "seizure", ConfidenceReview, 2},
		{"name", narrative, "DOE, JANE", ConfidenceReview, 1}, // upper pair; "Jane Doe" absent in narrative
	}
	for _, tc := range tests {
		t.Run(tc.cat+"/"+tc.wantText, func(t *testing.T) {
			props := proposeCat(t, tc.text, tc.cat, cx)
			if len(props) != tc.wantCount {
				t.Fatalf("got %d proposals %v, want %d", len(props), texts(props), tc.wantCount)
			}
			if tc.wantCount == 0 {
				return
			}
			if tc.wantText != "" && props[0].Text != tc.wantText {
				t.Errorf("first text = %q, want %q (all: %v)", props[0].Text, tc.wantText, texts(props))
			}
			if tc.wantConf != "" && props[0].Confidence != tc.wantConf {
				t.Errorf("first confidence = %q, want %q", props[0].Confidence, tc.wantConf)
			}
		})
	}
}

func TestNameExactAndPair(t *testing.T) {
	text := "Jane Doe called. Also DOE, JANE. And jane doe again."
	cx := Context{RecordType: "narrative", At: at, Names: []string{"Jane Doe"}}
	props := proposeCat(t, text, "name", cx)
	// Two exact (case-insensitive) matches of "Jane Doe" + one upper pair.
	var high, review int
	for _, p := range props {
		switch p.Confidence {
		case ConfidenceHigh:
			high++
		case ConfidenceReview:
			review++
		}
	}
	if high != 2 {
		t.Errorf("exact name matches = %d, want 2 (%v)", high, texts(props))
	}
	if review != 1 {
		t.Errorf("name-pair matches = %d, want 1 (%v)", review, texts(props))
	}
}

func TestWordsWholeWordOnly(t *testing.T) {
	r := Rule{ID: "w", Name: "cat words", Kind: KindWords, Words: []string{"cat", "fire alarm"}, Basis: "b", Explain: "e", Enabled: true}
	cx := Context{RecordType: "narrative", At: at}
	// "cat" must match as a whole word, not inside "category" or "scatter".
	text := "the cat sat; a category; scatter; a fire alarm sounded"
	props, errs := Propose(text, []Rule{r}, cx)
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	got := texts(props)
	want := []string{"cat", "fire alarm"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("words matches = %v, want %v", got, want)
	}
	for _, p := range props {
		if p.Confidence != ConfidenceHigh {
			t.Errorf("words proposal %q confidence = %q, want high", p.Text, p.Confidence)
		}
	}
}

func TestPatternRule(t *testing.T) {
	r := Rule{ID: "p", Name: "case no", Kind: KindPattern, Pattern: `C-\d{4}`, Basis: "b", Explain: "e", Enabled: true}
	cx := Context{RecordType: "narrative", At: at}
	props, errs := Propose("cases C-1234 and C-5678", []Rule{r}, cx)
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	if got := texts(props); strings.Join(got, "|") != "C-1234|C-5678" {
		t.Fatalf("pattern matches = %v", got)
	}
	for _, p := range props {
		if p.Confidence != ConfidenceHigh {
			t.Errorf("pattern confidence = %q, want high", p.Confidence)
		}
	}
}

func TestInvalidPatternReported(t *testing.T) {
	r := Rule{ID: "bad", Name: "bad", Kind: KindPattern, Pattern: `C-(\d{4}`, Basis: "b", Explain: "e", Enabled: true}
	good := Rule{ID: "good", Name: "good", Kind: KindPattern, Pattern: `\d+`, Basis: "b", Explain: "e", Enabled: true}
	cx := Context{RecordType: "narrative", At: at}
	props, errs := Propose("abc 123", []Rule{r, good}, cx)
	if len(errs) != 1 {
		t.Fatalf("errs = %v, want exactly 1", errs)
	}
	if !strings.Contains(errs[0].Error(), "bad") {
		t.Errorf("error should name the bad rule: %v", errs[0])
	}
	if len(props) != 1 || props[0].Text != "123" {
		t.Errorf("good rule should still run: %v", texts(props))
	}
}

func TestEffectiveDates(t *testing.T) {
	cx := Context{RecordType: "narrative", At: at}
	base := Rule{ID: "d", Kind: KindPattern, Pattern: `\d+`, Basis: "b", Explain: "e", Enabled: true}

	future := base
	future.From = at.Add(24 * time.Hour) // not yet in effect
	expired := base
	expired.To = at.Add(-24 * time.Hour) // already expired
	current := base
	current.From = at.Add(-24 * time.Hour)
	current.To = at.Add(24 * time.Hour)
	open := base // zero From/To = always in effect

	for _, tc := range []struct {
		name string
		r    Rule
		want int
	}{
		{"future", future, 0},
		{"expired", expired, 0},
		{"current", current, 1},
		{"open", open, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			props, _ := Propose("x 7", []Rule{tc.r}, cx)
			if len(props) != tc.want {
				t.Fatalf("got %d proposals, want %d", len(props), tc.want)
			}
		})
	}
}

func TestRecordTypeFilter(t *testing.T) {
	r := Rule{ID: "t", Kind: KindPattern, Pattern: `\d+`, RecordTypes: []string{"incident", "call"}, Basis: "b", Explain: "e", Enabled: true}
	for _, tc := range []struct {
		rt   string
		want int
	}{
		{"incident", 1},
		{"call", 1},
		{"narrative", 0},
		{"INCIDENT", 1}, // case-insensitive
	} {
		props, _ := Propose("x 7", []Rule{r}, Context{RecordType: tc.rt, At: at})
		if len(props) != tc.want {
			t.Errorf("record type %q: got %d, want %d", tc.rt, len(props), tc.want)
		}
	}

	// Empty RecordTypes applies to any type.
	rAll := r
	rAll.RecordTypes = nil
	props, _ := Propose("x 7", []Rule{rAll}, Context{RecordType: "anything", At: at})
	if len(props) != 1 {
		t.Errorf("empty RecordTypes should apply to all: got %d", len(props))
	}
}

func TestDisabledRuleSkipped(t *testing.T) {
	r := Rule{ID: "off", Kind: KindPattern, Pattern: `\d+`, Basis: "b", Explain: "e", Enabled: false}
	props, _ := Propose("x 7", []Rule{r}, Context{RecordType: "narrative", At: at})
	if len(props) != 0 {
		t.Fatalf("disabled rule should be skipped: %v", texts(props))
	}
}

func TestProposalOrderingAndOverlap(t *testing.T) {
	// Two rules matching overlapping/nested ranges to exercise the sort.
	text := "ABCDEFG"
	long := Rule{ID: "long", Kind: KindPattern, Pattern: `ABCDE`, Basis: "b", Explain: "e", Enabled: true}
	short := Rule{ID: "short", Kind: KindPattern, Pattern: `ABC`, Basis: "b", Explain: "e", Enabled: true}
	mid := Rule{ID: "mid", Kind: KindPattern, Pattern: `CDE`, Basis: "b", Explain: "e", Enabled: true}
	props, _ := Propose(text, []Rule{short, mid, long}, Context{RecordType: "x", At: at})
	if len(props) != 3 {
		t.Fatalf("want 3 proposals, got %d", len(props))
	}
	// Sorted by Start asc, then longest (larger End) first.
	// Start 0: ABCDE (end 5) before ABC (end 3). Then start 2: CDE.
	if props[0].Text != "ABCDE" || props[1].Text != "ABC" || props[2].Text != "CDE" {
		t.Fatalf("ordering wrong: %v", texts(props))
	}
	for i := 1; i < len(props); i++ {
		if props[i].Start < props[i-1].Start {
			t.Fatalf("not sorted by Start: %v", props)
		}
	}
}

func TestDeterministicAndStableIDs(t *testing.T) {
	cx := Context{RecordType: "narrative", At: at, Names: []string{"Jane Doe"}}
	rules := SampleRulesEnabled()
	a, _ := Propose(narrative, rules, cx)
	b, _ := Propose(narrative, rules, cx)
	if len(a) != len(b) {
		t.Fatalf("non-deterministic count: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("proposal %d differs between runs:\n%+v\n%+v", i, a[i], b[i])
		}
	}
	// IDs are 12 hex chars and stable for the same (rule, range, text).
	idRE := regexp.MustCompile(`^[0-9a-f]{12}$`)
	for _, p := range a {
		if !idRE.MatchString(p.ID) {
			t.Errorf("ID %q is not 12 hex chars", p.ID)
		}
	}
	// Known stable ID for a fixed input.
	id1 := proposalID("r", 3, 7, "1234")
	id2 := proposalID("r", 3, 7, "1234")
	if id1 != id2 || len(id1) != 12 {
		t.Fatalf("proposalID not stable: %q %q", id1, id2)
	}
	if proposalID("r", 3, 7, "1234") == proposalID("r", 3, 8, "1234") {
		t.Errorf("IDs should differ when range differs")
	}
}

// SampleRulesEnabled returns the sample rules enabled, for proposal tests.
func SampleRulesEnabled() []Rule {
	rs := SampleRules()
	for i := range rs {
		rs[i].Enabled = true
	}
	return rs
}

func TestApplyBasic(t *testing.T) {
	text := "SSN 123-45-6789 here"
	ds := []Decision{{
		ID: "x", Start: 4, End: 15, Text: "123-45-6789", Accept: true,
		Basis: "Va. Code § 2.2-3706.1(B)(1)", By: "clerk", At: at,
	}}
	out, errs := Apply(text, ds)
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	want := "SSN [REDACTED: Va. Code § 2.2-3706.1(B)(1)] here"
	if out != want {
		t.Fatalf("Apply = %q, want %q", out, want)
	}
	if strings.Contains(out, "123-45-6789") {
		t.Fatal("redacted text must not survive in output")
	}
}

func TestApplyRejectedIgnored(t *testing.T) {
	text := "keep 123 please"
	ds := []Decision{{ID: "x", Start: 5, End: 8, Text: "123", Accept: false, Basis: "b", By: "c", At: at}}
	out, errs := Apply(text, ds)
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	if out != text {
		t.Fatalf("rejected decision changed text: %q", out)
	}
}

func TestApplyOverlapMerge(t *testing.T) {
	text := "0123456789"
	ds := []Decision{
		{ID: "a", Start: 2, End: 6, Text: "2345", Accept: true, Basis: "B1", By: "c", At: at},
		{ID: "b", Start: 4, End: 8, Text: "4567", Accept: true, Basis: "B2", By: "c", At: at},
		{ID: "c", Start: 5, End: 7, Text: "56", Accept: true, Basis: "B1", By: "c", At: at}, // nested, dup basis
	}
	out, errs := Apply(text, ds)
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	// Ranges [2:6],[4:8],[5:7] merge to [2:8]; bases joined, dedup, order B1;B2.
	want := "01[REDACTED: B1; B2]89"
	if out != want {
		t.Fatalf("Apply overlap = %q, want %q", out, want)
	}
}

func TestApplyMismatchedTextRejected(t *testing.T) {
	text := "SSN 123-45-6789 here"
	ds := []Decision{
		{ID: "stale", Start: 4, End: 15, Text: "999-99-9999", Accept: true, Basis: "b", By: "c", At: at}, // text changed
		{ID: "oob", Start: 4, End: 999, Text: "x", Accept: true, Basis: "b", By: "c", At: at},            // out of range
		{ID: "good", Start: 0, End: 3, Text: "SSN", Accept: true, Basis: "b", By: "c", At: at},
	}
	out, errs := Apply(text, ds)
	if len(errs) != 2 {
		t.Fatalf("want 2 errors, got %d: %v", len(errs), errs)
	}
	// Good decision still applied; original SSN untouched by the stale decision.
	if !strings.HasPrefix(out, "[REDACTED: b]") {
		t.Fatalf("good decision not applied: %q", out)
	}
	if !strings.Contains(out, "123-45-6789") {
		t.Fatalf("stale decision must not redact changed text: %q", out)
	}
}

func TestApplyNeverContainsRedactedText(t *testing.T) {
	cx := Context{RecordType: "narrative", At: at, Names: []string{"Jane Doe"}}
	props, _ := Propose(narrative, SampleRulesEnabled(), cx)
	if len(props) == 0 {
		t.Fatal("expected some proposals")
	}
	var ds []Decision
	for _, p := range props {
		ds = append(ds, Decision{
			ID: p.ID, Start: p.Start, End: p.End, Text: p.Text,
			Accept: true, Basis: p.Basis, By: "clerk", At: at,
		})
	}
	out, errs := Apply(narrative, ds)
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	// None of the accepted passages may survive anywhere in the output.
	for _, p := range props {
		if strings.Contains(out, p.Text) {
			t.Errorf("redacted text %q survived in output: %q", p.Text, out)
		}
	}
}

func TestValidateFindsLeaks(t *testing.T) {
	ds := []Decision{
		{ID: "1", Text: "123-45-6789", Accept: true, Basis: "b", By: "c", At: at},
		{ID: "2", Text: "(540) 555-2348", Accept: true, Basis: "b", By: "c", At: at},
		{ID: "3", Text: "Jane Doe", Accept: true, Basis: "b", By: "c", At: at},
		{ID: "4", Text: "overdosed", Accept: false, Basis: "b", By: "c", At: at}, // rejected -> ignored
	}

	// Leak in the body (case/space normalised), in metadata, and digits-only form.
	released := "Subject   JANE   DOE was involved." // name leaked, different spacing/case
	meta := map[string]string{
		"title":    "Call from 5405552348",   // phone leaked in digits-only form
		"filename": "report_123-45-6789.pdf", // SSN leaked literally
	}
	leaks := Validate(released, meta, ds)

	found := map[string]bool{}
	for _, l := range leaks {
		found[l.Text+"@"+l.Where] = true
	}
	if !found["Jane Doe@text"] {
		t.Errorf("should find name leak in body; leaks=%v", leaks)
	}
	if !found["(540) 555-2348@title"] {
		t.Errorf("should find phone leak in title via digits-only; leaks=%v", leaks)
	}
	if !found["123-45-6789@filename"] {
		t.Errorf("should find SSN leak in filename; leaks=%v", leaks)
	}
	// Rejected decision must never produce a leak.
	for _, l := range leaks {
		if l.Text == "overdosed" {
			t.Errorf("rejected decision should not be checked: %v", l)
		}
	}
}

func TestValidateClean(t *testing.T) {
	ds := []Decision{{ID: "1", Text: "123-45-6789", Accept: true, Basis: "b", By: "c", At: at}}
	leaks := Validate("a clean release with no identifiers", map[string]string{"title": "fine"}, ds)
	if len(leaks) != 0 {
		t.Fatalf("expected no leaks, got %v", leaks)
	}
}

func TestSampleRules(t *testing.T) {
	rs := SampleRules()
	if len(rs) == 0 {
		t.Fatal("no sample rules")
	}
	wantCats := map[string]bool{"ssn": true, "dob": true, "phone": true, "juvenile": true, "medical": true, "plate": true, "name": true}
	gotCats := map[string]bool{}
	for _, r := range rs {
		if r.Enabled {
			t.Errorf("sample rule %q must ship disabled", r.ID)
		}
		if !r.Sample {
			t.Errorf("sample rule %q must be marked Sample", r.ID)
		}
		if r.Basis == "" {
			t.Errorf("sample rule %q must carry a placeholder basis", r.ID)
		}
		low := strings.ToLower(r.Explain)
		if !strings.Contains(low, "example") || !strings.Contains(low, "not legal advice") {
			t.Errorf("sample rule %q Explain must say it is an example and not legal advice: %q", r.ID, r.Explain)
		}
		if r.Kind == KindCategory {
			gotCats[r.Category] = true
		}
	}
	for c := range wantCats {
		if !gotCats[c] {
			t.Errorf("sample rules should cover category %q", c)
		}
	}
}

func TestNormaliseAndDigits(t *testing.T) {
	if got := normalise("  Hello\tWORLD \n foo "); got != "hello world foo" {
		t.Errorf("normalise = %q", got)
	}
	if got := digitsOnly("(540) 555-2348"); got != "5405552348" {
		t.Errorf("digitsOnly = %q", got)
	}
}

// texts extracts proposal texts for readable assertions.
func texts(ps []Proposal) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Text
	}
	return out
}
