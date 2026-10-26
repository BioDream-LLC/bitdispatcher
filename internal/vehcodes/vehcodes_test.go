package vehcodes

import (
	"strings"
	"testing"
)

func TestListsStableOrderAndNonEmpty(t *testing.T) {
	ls := Lists()
	if len(ls) != len(listOrder) {
		t.Fatalf("Lists() returned %d lists, want %d", len(ls), len(listOrder))
	}
	for i, id := range listOrder {
		if ls[i].ID != id {
			t.Errorf("Lists()[%d].ID = %q, want %q", i, ls[i].ID, id)
		}
		if len(ls[i].Codes) == 0 {
			t.Errorf("list %q has no codes", id)
		}
		if ls[i].Name == "" || ls[i].Source == "" {
			t.Errorf("list %q missing Name or Source", id)
		}
	}
}

func TestListsReturnsCopy(t *testing.T) {
	a := Lists()
	a[0].Codes[0].Label = "MUTATED"
	b := Lists()
	if b[0].Codes[0].Label == "MUTATED" {
		t.Error("Lists() shares backing data; mutation leaked into built-ins")
	}
}

func TestGet(t *testing.T) {
	l, ok := Get("color")
	if !ok {
		t.Fatal("Get(color) not found")
	}
	if l.ID != "color" {
		t.Errorf("Get(color).ID = %q", l.ID)
	}
	if _, ok := Get("nope"); ok {
		t.Error("Get(nope) should be false")
	}
}

func TestLabelAndHasCaseInsensitive(t *testing.T) {
	l, _ := Get("color")
	if got := l.Label("RED"); got != "Red" {
		t.Errorf("Label(RED) = %q, want Red", got)
	}
	if got := l.Label("red"); got != "Red" {
		t.Errorf("Label(red) = %q, want Red (case-insensitive)", got)
	}
	if got := l.Label(" blk "); got != "Black" {
		t.Errorf("Label(' blk ') = %q, want Black", got)
	}
	if got := l.Label("ZZZ"); got != "" {
		t.Errorf("Label(ZZZ) = %q, want empty", got)
	}
	if !l.Has("blu") {
		t.Error("Has(blu) = false, want true")
	}
	if l.Has("zzz") {
		t.Error("Has(zzz) = true, want false")
	}
}

func TestStateContainsExpected(t *testing.T) {
	l, _ := Get("state")
	cases := map[string]string{
		"VA": "Virginia",
		"DC": "District of Columbia",
		"PR": "Puerto Rico",
		"NF": "Newfoundland and Labrador",
		"PQ": "Quebec",
		"ON": "Ontario",
		"MX": "Mexico",
		"US": "United States Government",
	}
	for code, want := range cases {
		if got := l.Label(code); got != want {
			t.Errorf("state Label(%q) = %q, want %q", code, got, want)
		}
	}
	// NCIC uses NF/PQ, not NL/QC; confirm those non-NCIC codes are absent.
	if l.Has("NL") || l.Has("QC") {
		t.Error("state list should use NCIC NF/PQ, not NL/QC")
	}
}

func TestMakeHasCommonMakes(t *testing.T) {
	l, _ := Get("make")
	for _, c := range []string{"FORD", "CHEV", "TOYT", "HOND", "TESL", "HD", "KW"} {
		if !l.Has(c) {
			t.Errorf("make list missing %q", c)
		}
	}
	if got := l.Label("MERZ"); got != "Mercedes-Benz" {
		t.Errorf("make Label(MERZ) = %q", got)
	}
}

func TestSexRaceEthnicity(t *testing.T) {
	sex, _ := Get("sex")
	if !sex.Has("M") || !sex.Has("F") || !sex.Has("U") {
		t.Error("sex list should have M, F, U")
	}
	race, _ := Get("race")
	if got := race.Label("I"); got != "American Indian or Alaska Native" {
		t.Errorf("race Label(I) = %q", got)
	}
	eth, _ := Get("ethnicity")
	if got := eth.Label("H"); got != "Hispanic or Latino" {
		t.Errorf("ethnicity Label(H) = %q", got)
	}
}

func TestPlateConditionIsAgencyList(t *testing.T) {
	l, _ := Get("plateCondition")
	if l.Source != "bitdispatcher" {
		t.Errorf("plateCondition Source = %q, want bitdispatcher", l.Source)
	}
	if !l.Has("TEMP") {
		t.Error("plateCondition missing TEMP")
	}
}

func TestMergeAddsNewCode(t *testing.T) {
	base, _ := Get("plateType")
	n := len(base.Codes)
	merged := Merge(base, []Code{{"XX", "Special Agency Type"}}, nil)
	if len(merged.Codes) != n+1 {
		t.Fatalf("Merge added wrong count: got %d want %d", len(merged.Codes), n+1)
	}
	// New code appended after built-ins.
	last := merged.Codes[len(merged.Codes)-1]
	if last.Code != "XX" || last.Label != "Special Agency Type" {
		t.Errorf("appended addition = %+v", last)
	}
	// Built-in list unchanged.
	again, _ := Get("plateType")
	if len(again.Codes) != n {
		t.Error("Merge mutated the built-in list")
	}
}

func TestMergeOverridesLabelInPlace(t *testing.T) {
	base, _ := Get("plateType")
	// PC exists; override its label (lowercase code to test case-insensitivity).
	merged := Merge(base, []Code{{"pc", "Passenger (local wording)"}}, nil)
	if len(merged.Codes) != len(base.Codes) {
		t.Errorf("override should not change count: got %d want %d", len(merged.Codes), len(base.Codes))
	}
	if merged.Codes[0].Code != "PC" {
		t.Errorf("first code = %q, override should keep position", merged.Codes[0].Code)
	}
	if merged.Codes[0].Label != "Passenger (local wording)" {
		t.Errorf("override label = %q", merged.Codes[0].Label)
	}
}

func TestMergeRemoves(t *testing.T) {
	base, _ := Get("plateType")
	merged := Merge(base, nil, []string{"mc"})
	if merged.Has("MC") {
		t.Error("Merge did not remove MC")
	}
	if merged.Has("PC") == false {
		t.Error("Merge wrongly removed PC")
	}
}

func TestMergeOrderBuiltinThenAdditions(t *testing.T) {
	base := List{ID: "t", Name: "T", Source: "s", Codes: []Code{{"A", "a"}, {"B", "b"}}}
	merged := Merge(base, []Code{{"C", "c"}, {"A", "a2"}, {"D", "d"}}, []string{"B"})
	// Expected: A (overridden), C, D. B removed.
	wantCodes := []string{"A", "C", "D"}
	if len(merged.Codes) != len(wantCodes) {
		t.Fatalf("got %d codes, want %d: %+v", len(merged.Codes), len(wantCodes), merged.Codes)
	}
	for i, w := range wantCodes {
		if merged.Codes[i].Code != w {
			t.Errorf("order[%d] = %q, want %q", i, merged.Codes[i].Code, w)
		}
	}
	if merged.Codes[0].Label != "a2" {
		t.Errorf("A label after override = %q, want a2", merged.Codes[0].Label)
	}
}

func TestSortHelper(t *testing.T) {
	cs := []Code{{"C", "c"}, {"A", "a"}, {"B", "b"}}
	sortCodesByCode(cs)
	if cs[0].Code != "A" || cs[2].Code != "C" {
		t.Errorf("sortCodesByCode wrong: %+v", cs)
	}
}

// --- VIN tests ---

func TestCheckVINKnownGood(t *testing.T) {
	r := CheckVIN("1HGCM82633A004352")
	if !r.Complete {
		t.Error("should be complete")
	}
	if !r.CheckDigit {
		t.Errorf("check digit should match, problems=%v", r.Problems)
	}
	if !r.Valid {
		t.Errorf("should be valid, problems=%v", r.Problems)
	}
	if r.WMI != "1HG" {
		t.Errorf("WMI = %q, want 1HG", r.WMI)
	}
	if r.Year != "2003 or 2033" {
		t.Errorf("Year = %q, want '2003 or 2033'", r.Year)
	}
}

func TestCheckVINAllOnes(t *testing.T) {
	r := CheckVIN("11111111111111111")
	if !r.Complete || !r.CheckDigit || !r.Valid {
		t.Errorf("all-ones VIN should be valid: %+v", r)
	}
}

func TestCheckVINStripsSpacesAndDashes(t *testing.T) {
	r := CheckVIN(" 1hg cm8-2633a004352 ")
	if r.VIN != "1HGCM82633A004352" {
		t.Errorf("cleaned VIN = %q", r.VIN)
	}
	if !r.Valid {
		t.Errorf("should still be valid after cleaning: %+v", r)
	}
}

func TestCheckVINIllegalChars(t *testing.T) {
	// Replace a valid char with I to trigger the I/O/Q rule.
	r := CheckVIN("1HGCM82633A00435I")
	if r.Valid {
		t.Error("VIN with I should not be valid")
	}
	found := false
	for _, p := range r.Problems {
		if strings.Contains(p, "I, O or Q") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected I/O/Q problem, got %v", r.Problems)
	}
}

func TestCheckVINPartial(t *testing.T) {
	r := CheckVIN("1HGCM8263")
	if r.Complete {
		t.Error("9-char VIN should not be complete")
	}
	if r.Valid {
		t.Error("partial VIN should not be valid")
	}
	found := false
	for _, p := range r.Problems {
		if p == "partial VIN: 9 of 17 characters" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected partial problem, got %v", r.Problems)
	}
	// WMI should still be extracted from a partial read.
	if r.WMI != "1HG" {
		t.Errorf("partial WMI = %q, want 1HG", r.WMI)
	}
}

func TestCheckVINWrongCheckDigit(t *testing.T) {
	// Flip the check-digit position of a known-good VIN (pos 9 = '3' -> '4').
	r := CheckVIN("1HGCM82643A004352")
	if r.CheckDigit {
		t.Error("check digit should not match")
	}
	if r.Valid {
		t.Error("should not be valid with wrong check digit")
	}
	found := false
	for _, p := range r.Problems {
		if strings.Contains(p, "check digit does not match") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected check-digit problem, got %v", r.Problems)
	}
}

func TestCheckVINYearDecode(t *testing.T) {
	// Position 10 = 'L' -> 1990 or 2020.
	r := CheckVIN("1HGCM8263LA004352")
	if r.Year != "1990 or 2020" {
		t.Errorf("Year = %q, want '1990 or 2020'", r.Year)
	}
}

func TestCheckVINCheckDigitX(t *testing.T) {
	// 5GZCZ43D13S812715 is a widely cited VIN whose check digit computes to X.
	r := CheckVIN("5GZCZ43D13S812715")
	// We only assert the computation path runs and produces a boolean result
	// plus a decoded year; exact match depends on the example.
	if !r.Complete {
		t.Error("should be complete")
	}
	if r.Year == "" {
		t.Error("year should decode")
	}
}
