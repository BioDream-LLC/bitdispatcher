package acn

import (
	"strings"
	"testing"
)

// boolPtr returns a pointer to b, for setting Crash.Belted in tests.
func boolPtr(b bool) *bool { return &b }

// containsReason reports whether any reason contains the given substring.
func containsSub(ss []string, sub string) bool {
	for _, s := range ss {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func TestAssessRisk(t *testing.T) {
	th := DefaultThresholds()
	belted := true
	unbelted := false

	tests := []struct {
		name string
		c    Crash
		want string
	}{
		{
			name: "high by delta-V alone",
			c:    Crash{DeltaVKmh: 56, Direction: "front", Belted: &belted},
			want: RiskHigh,
		},
		{
			name: "high by rollover with low delta-V",
			c:    Crash{DeltaVKmh: 10, Rollover: true},
			want: RiskHigh,
		},
		{
			name: "high by medium delta-V plus side impact",
			c:    Crash{DeltaVKmh: 25, Direction: "left"},
			want: RiskHigh,
		},
		{
			name: "high by medium delta-V plus unbelted",
			c:    Crash{DeltaVKmh: 25, Belted: &unbelted},
			want: RiskHigh,
		},
		{
			name: "high by medium delta-V plus multiple impacts",
			c:    Crash{DeltaVKmh: 25, MultipleImpacts: true},
			want: RiskHigh,
		},
		{
			name: "medium by delta-V between thresholds, belted front",
			c:    Crash{DeltaVKmh: 25, Direction: "front", Belted: &belted},
			want: RiskMedium,
		},
		{
			name: "medium by airbag deployment with no delta-V",
			c:    Crash{Airbags: []string{"front"}},
			want: RiskMedium,
		},
		{
			name: "low by small delta-V no airbags",
			c:    Crash{DeltaVKmh: 10},
			want: RiskLow,
		},
		{
			name: "unknown with nothing to go on",
			c:    Crash{Provider: "OnStar", Callback: "+15551234567"},
			want: RiskUnknown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Assess(tc.c, th)
			if got.Risk != tc.want {
				t.Fatalf("Risk = %q, want %q (reasons: %v)", got.Risk, tc.want, got.Reasons)
			}
			if got.Summary == "" {
				t.Errorf("Summary should not be empty")
			}
		})
	}
}

func TestAssessReasonsAndSuggest(t *testing.T) {
	th := DefaultThresholds()
	unbelted := false

	c := Crash{
		DeltaVKmh:       56,
		Direction:       "left",
		Rollover:        true,
		MultipleImpacts: true,
		Airbags:         []string{"side", "curtain"},
		Occupants:       2,
		Belted:          &unbelted,
		Fuel:            "electric",
	}
	a := Assess(c, th)

	if a.Risk != RiskHigh {
		t.Fatalf("Risk = %q, want high", a.Risk)
	}
	// delta-V formatted "56 km/h (35 mph)".
	if !containsSub(a.Reasons, "delta-V 56 km/h (35 mph)") {
		t.Errorf("missing formatted delta-V reason: %v", a.Reasons)
	}
	if !containsSub(a.Reasons, "rollover") {
		t.Errorf("missing rollover reason: %v", a.Reasons)
	}
	if !containsSub(a.Reasons, "side airbags deployed") {
		t.Errorf("missing side-impact-with-airbags reason: %v", a.Reasons)
	}
	if !containsSub(a.Reasons, "unbelted") {
		t.Errorf("missing unbelted reason: %v", a.Reasons)
	}
	if !containsSub(a.Reasons, "several impacts") {
		t.Errorf("missing several impacts reason: %v", a.Reasons)
	}
	// Suggestions.
	if !containsSub(a.Suggest, "ALS") {
		t.Errorf("missing ALS suggestion: %v", a.Suggest)
	}
	if !containsSub(a.Suggest, "extrication") {
		t.Errorf("missing extrication suggestion: %v", a.Suggest)
	}
	if !containsSub(a.Suggest, "occupants: consider more than one ambulance") {
		t.Errorf("missing multi-occupant suggestion: %v", a.Suggest)
	}
	if !containsSub(a.Suggest, "air medical") {
		t.Errorf("missing air medical suggestion: %v", a.Suggest)
	}
}

func TestEVSuggestionRegardlessOfRisk(t *testing.T) {
	th := DefaultThresholds()
	// A low-risk crash still gets the EV fire/extrication note.
	c := Crash{DeltaVKmh: 10, Fuel: "electric"}
	a := Assess(c, th)
	if a.Risk != RiskLow {
		t.Fatalf("Risk = %q, want low", a.Risk)
	}
	if !containsSub(a.Suggest, "electric vehicle: tell fire") {
		t.Errorf("missing EV suggestion on low-risk crash: %v", a.Suggest)
	}

	// Hydrogen likewise.
	h := Assess(Crash{DeltaVKmh: 10, Fuel: "hydrogen"}, th)
	if !containsSub(h.Suggest, "hydrogen vehicle: tell fire") {
		t.Errorf("missing hydrogen suggestion: %v", h.Suggest)
	}
}

func TestDefaultThresholds(t *testing.T) {
	th := DefaultThresholds()
	if th.HighDeltaVKmh != 40 {
		t.Errorf("HighDeltaVKmh = %v, want 40", th.HighDeltaVKmh)
	}
	if th.MediumDeltaVKmh != 20 {
		t.Errorf("MediumDeltaVKmh = %v, want 20", th.MediumDeltaVKmh)
	}
}

func TestParseNativeJSON(t *testing.T) {
	in := []byte(`{
		"provider": "OnStar",
		"deltaVKmh": 56,
		"direction": "front",
		"rollover": true,
		"airbags": ["front", "side"],
		"occupants": 2,
		"vehicle": {"year": 2022, "make": "Chevrolet", "model": "Bolt", "vin": "1G1"},
		"fuel": "electric",
		"lat": 41.5,
		"lon": -81.7
	}`)
	c, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse native JSON: %v", err)
	}
	if c.Provider != "OnStar" {
		t.Errorf("Provider = %q", c.Provider)
	}
	if c.DeltaVKmh != 56 {
		t.Errorf("DeltaVKmh = %v, want 56", c.DeltaVKmh)
	}
	if !c.Rollover {
		t.Errorf("Rollover = false, want true")
	}
	if c.Vehicle.Make != "Chevrolet" || c.Vehicle.Year != 2022 {
		t.Errorf("Vehicle = %+v", c.Vehicle)
	}
	if c.Lat == nil || *c.Lat != 41.5 {
		t.Errorf("Lat = %v, want 41.5", c.Lat)
	}
	if c.Fuel != "electric" {
		t.Errorf("Fuel = %q", c.Fuel)
	}
}

func TestParseFlatJSONMphAndPDOF(t *testing.T) {
	// delta_v_mph in mph (35 mph -> ~56.3 km/h), pdof in degrees (90 -> right).
	in := []byte(`{
		"delta_v_mph": 35,
		"pdof": 90,
		"rollover_indicator": "true",
		"airbag_deployed": true,
		"seatbelt": "unbelted",
		"number_of_occupants": 3,
		"vehicle_year": 2023,
		"vehicle_make": "Tesla",
		"fuel_type": "BEV",
		"latitude": 40.0,
		"longitude": -75.0
	}`)
	c, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse flat JSON: %v", err)
	}
	wantKmh := 35 * 1.609344
	if c.DeltaVKmh < wantKmh-0.01 || c.DeltaVKmh > wantKmh+0.01 {
		t.Errorf("DeltaVKmh = %v, want ~%v", c.DeltaVKmh, wantKmh)
	}
	if c.Direction != "right" {
		t.Errorf("Direction = %q, want right (pdof 90)", c.Direction)
	}
	if !c.Rollover {
		t.Errorf("Rollover = false, want true")
	}
	if len(c.Airbags) != 1 || c.Airbags[0] != "front" {
		t.Errorf("Airbags = %v, want [front]", c.Airbags)
	}
	if c.Belted == nil || *c.Belted {
		t.Errorf("Belted = %v, want false", c.Belted)
	}
	if c.Occupants != 3 {
		t.Errorf("Occupants = %v, want 3", c.Occupants)
	}
	if c.Fuel != "electric" {
		t.Errorf("Fuel = %q, want electric (from BEV)", c.Fuel)
	}
	if c.Lat == nil || *c.Lat != 40.0 {
		t.Errorf("Lat = %v", c.Lat)
	}

	// pdof direction boundaries.
	for deg, want := range map[float64]string{0: "front", 350: "front", 90: "right", 180: "rear", 270: "left"} {
		if got := directionFromDegrees(deg); got != want {
			t.Errorf("directionFromDegrees(%v) = %q, want %q", deg, got, want)
		}
	}
}

func TestParseKeyValueLines(t *testing.T) {
	in := []byte("delta_v_mph=30\n" +
		"pdof=180\n" +
		"rollover=no\n" +
		"number_of_occupants=1\n" +
		"vehicle_make=Ford\n" +
		"vin=1FTFW1E\n" +
		"fuel_type=diesel\n" +
		"# a comment line\n" +
		"latitude=35.2\n" +
		"longitude=-97.4\n")
	c, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse key=value: %v", err)
	}
	if c.Direction != "rear" {
		t.Errorf("Direction = %q, want rear (pdof 180)", c.Direction)
	}
	if c.Rollover {
		t.Errorf("Rollover = true, want false")
	}
	if c.Vehicle.Make != "Ford" || c.Vehicle.VIN != "1FTFW1E" {
		t.Errorf("Vehicle = %+v", c.Vehicle)
	}
	if c.Fuel != "diesel" {
		t.Errorf("Fuel = %q", c.Fuel)
	}
	if c.Lat == nil || *c.Lat != 35.2 {
		t.Errorf("Lat = %v", c.Lat)
	}
	wantKmh := 30 * 1.609344
	if c.DeltaVKmh < wantKmh-0.01 || c.DeltaVKmh > wantKmh+0.01 {
		t.Errorf("DeltaVKmh = %v, want ~%v", c.DeltaVKmh, wantKmh)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"garbage text", "the quick brown fox"},
		{"empty", ""},
		{"empty JSON object", "{}"},
		{"json with no recognised keys", `{"weather":"sunny","note":"nothing useful"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse([]byte(tc.in)); err == nil {
				t.Errorf("Parse(%q) = nil error, want error", tc.in)
			}
		})
	}
}

func TestParseEndToEndAssess(t *testing.T) {
	// A flat notification parsed and then assessed should land high risk.
	c, err := Parse([]byte("delta_v_mph=35\npdof=270\nfuel_type=electric\nnumber_of_occupants=2\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	a := Assess(c, DefaultThresholds())
	if a.Risk != RiskHigh {
		t.Fatalf("Risk = %q, want high", a.Risk)
	}
	if !containsSub(a.Suggest, "electric vehicle: tell fire") {
		t.Errorf("missing EV suggestion: %v", a.Suggest)
	}
}

func TestAssessListsNeverNil(t *testing.T) {
	a := Assess(Crash{}, DefaultThresholds())
	if a.Reasons == nil || a.Suggest == nil {
		t.Fatal("nil lists reach the console as null")
	}
}
