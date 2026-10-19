// Package acn turns Advanced Automatic Crash Notification (AACN) and vehicle
// telematics data into a field-triage style assessment of injury risk, to help
// a 911 dispatcher pick a response before anyone is on scene.
//
// When a modern vehicle crashes, a telematics provider (for example OnStar, an
// automaker's connected service, or a phone's crash detection) can reach a 911
// centre with sensor data: a change in velocity (delta-V), the principal
// direction of force, whether the vehicle rolled over, which airbags fired, and
// so on. This package reads that data (see [Parse]) and scores it (see
// [Assess]).
//
// The scoring is a rule of thumb, not a diagnosis. It is modelled on the
// vehicle-telematics criteria in the CDC / American College of Surgeons Field
// Triage Guidelines and on the AACN research literature, in which delta-V is the
// strongest single predictor of serious injury. The thresholds are configurable
// (see [Thresholds]) so an agency can tune them to local protocol. Treat the
// result as a dispatcher's aid that flags a likely-serious crash for a stronger
// response; it never replaces a clinician's assessment on scene.
package acn

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Risk levels returned in [Assessment.Risk].
const (
	// RiskHigh marks a crash likely to involve serious injury.
	RiskHigh = "high"
	// RiskMedium marks a crash with a meaningful chance of injury.
	RiskMedium = "medium"
	// RiskLow marks a minor crash on the available data.
	RiskLow = "low"
	// RiskUnknown marks a crash with nothing to go on.
	RiskUnknown = "unknown"
)

// Vehicle describes the vehicle that crashed. Every field is optional; a
// provider may send only what it knows.
type Vehicle struct {
	Year  int    `json:"year,omitempty"`
	Make  string `json:"make,omitempty"`
	Model string `json:"model,omitempty"`
	Color string `json:"color,omitempty"`
	VIN   string `json:"vin,omitempty"`
	Plate string `json:"plate,omitempty"`
}

// Crash is what a vehicle reported about a crash. Most fields are optional,
// reflecting that providers differ in what they can send.
type Crash struct {
	// Provider is who sent the notification: OnStar, an automaker's service, a
	// phone's crash detection, and so on.
	Provider string `json:"provider,omitempty"`
	// At is when the crash happened.
	At time.Time `json:"at,omitzero"`
	// Lat and Lon are the crash location in decimal degrees, nil if unknown.
	Lat *float64 `json:"lat,omitempty"`
	Lon *float64 `json:"lon,omitempty"`
	// Heading is the vehicle's travel direction in degrees, if known.
	Heading float64 `json:"heading,omitempty"`
	// DeltaVKmh is the change in velocity during the crash, in km/h. It is the
	// strongest single predictor of serious injury in the AACN literature.
	DeltaVKmh float64 `json:"deltaVKmh,omitempty"`
	// Direction is the principal direction of force: front, rear, left, or
	// right.
	Direction string `json:"direction,omitempty"`
	// Rollover reports that the vehicle rolled over.
	Rollover bool `json:"rollover,omitempty"`
	// MultipleImpacts reports that the vehicle was struck more than once.
	MultipleImpacts bool `json:"multipleImpacts,omitempty"`
	// Airbags lists the airbags that deployed: front, side, curtain, knee.
	Airbags []string `json:"airbags,omitempty"`
	// Occupants is the number of people in the vehicle, if known.
	Occupants int `json:"occupants,omitempty"`
	// Belted reports whether all front occupants were belted, nil if unknown.
	Belted *bool `json:"belted,omitempty"`
	// SpeedKmh is the vehicle's speed just before the crash, in km/h.
	SpeedKmh float64 `json:"speedKmh,omitempty"`
	// Vehicle identifies the vehicle.
	Vehicle Vehicle `json:"vehicle"`
	// Fuel is the propulsion type: gasoline, diesel, electric, hybrid,
	// hydrogen, or cng. It drives a fire/extrication hazard note.
	Fuel string `json:"fuel,omitempty"`
	// VoiceConnected reports that the vehicle's voice line reached a person.
	VoiceConnected bool `json:"voiceConnected,omitempty"`
	// Callback is a phone number the centre can call back.
	Callback string `json:"callback,omitempty"`
}

// Thresholds are the delta-V cut points the triage uses, in km/h. An agency can
// change them to match local protocol.
type Thresholds struct {
	// HighDeltaVKmh is the delta-V at or above which a crash is high risk.
	HighDeltaVKmh float64 `json:"highDeltaVKmh"`
	// MediumDeltaVKmh is the delta-V at or above which a crash is at least
	// medium risk.
	MediumDeltaVKmh float64 `json:"mediumDeltaVKmh"`
}

// DefaultThresholds returns the thresholds used when an agency has not set its
// own: 40 km/h (about 25 mph) for high risk and 20 km/h for medium.
//
// The high cut point reflects the AACN research and the Field Triage
// Guidelines, which associate a delta-V of roughly 25-30 mph (40-48 km/h) with
// a high risk of serious injury. The medium cut point is a lower, more
// cautious line for flagging crashes worth a closer look. These are rules of
// thumb; tune them to local protocol.
func DefaultThresholds() Thresholds {
	return Thresholds{HighDeltaVKmh: 40, MediumDeltaVKmh: 20}
}

// Assessment is the triage result: a risk level, the reasons behind it in plain
// words, concrete suggestions for the dispatcher, and a one-line summary.
type Assessment struct {
	// Risk is one of the Risk constants: high, medium, low, or unknown.
	Risk string `json:"risk"`
	// Reasons explains the risk in plain words, for example
	// "delta-V 56 km/h (35 mph): high risk of serious injury" or "rollover".
	Reasons []string `json:"reasons"`
	// Suggest holds concrete response suggestions, for example "ALS" or
	// "electric vehicle: tell fire (battery fire, extrication cut zones)".
	Suggest []string `json:"suggest"`
	// Summary is a one-line description of the crash and risk.
	Summary string `json:"summary"`
}

// sideImpact reports whether the principal direction of force was a side
// impact (left or right).
func sideImpact(direction string) bool {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "left", "right":
		return true
	}
	return false
}

// unbelted reports whether we know that not all front occupants were belted.
// A nil Belted (unknown) is not treated as unbelted.
func unbelted(c Crash) bool {
	return c.Belted != nil && !*c.Belted
}

// kmhToMph converts km/h to mph.
func kmhToMph(kmh float64) float64 {
	return kmh / 1.609344
}

// speed formats a km/h value as "56 km/h (35 mph)", rounding both to whole
// numbers.
func speed(kmh float64) string {
	return fmt.Sprintf("%d km/h (%d mph)",
		int(math.Round(kmh)), int(math.Round(kmhToMph(kmh))))
}

// fuelHazard returns a fire/extrication suggestion for the given fuel type, or
// the empty string when the fuel raises no special hazard note.
func fuelHazard(fuel string) string {
	switch strings.ToLower(strings.TrimSpace(fuel)) {
	case "electric", "hybrid":
		return "electric vehicle: tell fire (battery fire, extrication cut zones)"
	case "hydrogen":
		return "hydrogen vehicle: tell fire (pressurized fuel, fire/leak hazard)"
	case "cng":
		return "CNG vehicle: tell fire (pressurized fuel, fire/leak hazard)"
	}
	return ""
}

// anyAirbag reports whether any airbag deployed.
func anyAirbag(c Crash) bool {
	return len(c.Airbags) > 0
}

// Assess scores a crash against the thresholds and returns a triage result.
//
// The risk rules, in order, are:
//
//   - high if delta-V is at or above the high threshold, or the vehicle rolled
//     over, or delta-V is at or above the medium threshold and the crash was
//     unbelted, had multiple impacts, or was a side impact;
//   - otherwise medium if delta-V is at or above the medium threshold, or any
//     airbag deployed;
//   - otherwise low if delta-V is above zero but below the medium threshold and
//     no airbag deployed;
//   - otherwise unknown, when there is nothing to go on.
//
// A fuel hazard note for an electric, hybrid, hydrogen, or CNG vehicle is always
// added when relevant, regardless of risk level. The result is a rule of thumb
// to guide the response, not a diagnosis.
func Assess(c Crash, t Thresholds) Assessment {
	var a Assessment

	hasData := c.DeltaVKmh > 0 || c.Rollover || c.MultipleImpacts ||
		anyAirbag(c) || c.Direction != "" || c.Belted != nil

	high := false
	if c.DeltaVKmh >= t.HighDeltaVKmh && t.HighDeltaVKmh > 0 {
		high = true
	}
	if c.Rollover {
		high = true
	}
	if c.DeltaVKmh >= t.MediumDeltaVKmh && t.MediumDeltaVKmh > 0 &&
		(unbelted(c) || c.MultipleImpacts || sideImpact(c.Direction)) {
		high = true
	}

	medium := false
	if (c.DeltaVKmh >= t.MediumDeltaVKmh && t.MediumDeltaVKmh > 0) || anyAirbag(c) {
		medium = true
	}

	switch {
	case !hasData:
		a.Risk = RiskUnknown
	case high:
		a.Risk = RiskHigh
	case medium:
		a.Risk = RiskMedium
	case c.DeltaVKmh > 0 && c.DeltaVKmh < t.MediumDeltaVKmh && !anyAirbag(c):
		a.Risk = RiskLow
	default:
		a.Risk = RiskUnknown
	}

	// Reasons, in plain words.
	if c.DeltaVKmh > 0 {
		var tail string
		switch a.Risk {
		case RiskHigh:
			tail = ": high risk of serious injury"
		case RiskMedium:
			tail = ": moderate risk of injury"
		case RiskLow:
			tail = ": low risk of injury"
		}
		a.Reasons = append(a.Reasons, "delta-V "+speed(c.DeltaVKmh)+tail)
	}
	if c.Rollover {
		a.Reasons = append(a.Reasons, "rollover")
	}
	if sideImpact(c.Direction) {
		if anyAirbag(c) && hasAirbag(c, "side", "curtain") {
			a.Reasons = append(a.Reasons, c.Direction+" impact with side airbags deployed")
		} else {
			a.Reasons = append(a.Reasons, c.Direction+" impact")
		}
	} else if c.Direction != "" {
		a.Reasons = append(a.Reasons, c.Direction+" impact")
	}
	if unbelted(c) {
		a.Reasons = append(a.Reasons, "unbelted")
	}
	if c.MultipleImpacts {
		a.Reasons = append(a.Reasons, "several impacts")
	}
	if anyAirbag(c) && !sideImpact(c.Direction) {
		a.Reasons = append(a.Reasons, "airbags deployed: "+strings.Join(c.Airbags, ", "))
	}
	if c.Occupants > 1 {
		a.Reasons = append(a.Reasons, fmt.Sprintf("%d occupants", c.Occupants))
	}

	// Suggestions.
	if a.Risk == RiskHigh {
		a.Suggest = append(a.Suggest, "ALS")
		if c.Rollover || c.MultipleImpacts {
			a.Suggest = append(a.Suggest, "fire for extrication")
		}
		if rural(c) {
			a.Suggest = append(a.Suggest, "helicopter standby")
		} else {
			a.Suggest = append(a.Suggest, "consider air medical")
		}
		if c.Occupants > 1 {
			a.Suggest = append(a.Suggest,
				fmt.Sprintf("%d occupants: consider more than one ambulance", c.Occupants))
		}
	}
	// Fuel hazard note is added whenever relevant, regardless of risk.
	if h := fuelHazard(c.Fuel); h != "" {
		a.Suggest = append(a.Suggest, h)
	}

	a.Summary = summarize(c, a.Risk)
	return a
}

// hasAirbag reports whether any of the named airbags deployed.
func hasAirbag(c Crash, names ...string) bool {
	for _, got := range c.Airbags {
		g := strings.ToLower(strings.TrimSpace(got))
		for _, want := range names {
			if g == want {
				return true
			}
		}
	}
	return false
}

// rural reports a best-effort guess that the crash is rural, used only to
// choose between "helicopter standby" and "consider air medical". With no
// location data we cannot tell, so we default to the more cautious
// "consider air medical" by returning false.
func rural(c Crash) bool {
	// Without a reverse-geocode we cannot classify location; stay conservative.
	return false
}

// summarize builds the one-line summary, for example
// "High injury risk: front impact, delta-V 56 km/h (35 mph), rollover, 2 occupants".
func summarize(c Crash, risk string) string {
	var head string
	switch risk {
	case RiskHigh:
		head = "High injury risk"
	case RiskMedium:
		head = "Moderate injury risk"
	case RiskLow:
		head = "Low injury risk"
	default:
		head = "Unknown injury risk"
	}

	var parts []string
	if c.Direction != "" {
		parts = append(parts, c.Direction+" impact")
	}
	if c.DeltaVKmh > 0 {
		parts = append(parts, "delta-V "+speed(c.DeltaVKmh))
	}
	if c.Rollover {
		parts = append(parts, "rollover")
	}
	if c.MultipleImpacts {
		parts = append(parts, "several impacts")
	}
	if c.Occupants > 0 {
		parts = append(parts, fmt.Sprintf("%d occupants", c.Occupants))
	}
	if len(parts) == 0 {
		return head
	}
	return head + ": " + strings.Join(parts, ", ")
}

// Parse reads a crash notification in one of several shapes and returns a
// [Crash].
//
// It accepts this package's own JSON (the [Crash] fields), and also a flat
// alternative that uses common telematics key names, given either as JSON or as
// "key=value" lines. Recognised alternative keys include:
//
//   - delta_v_mph or deltaV (interpreted as mph when the key ends in _mph,
//     otherwise km/h);
//   - principal_direction_of_force or pdof, in degrees, mapped to a direction:
//     315-45 front, 45-135 right, 135-225 rear, 225-315 left;
//   - rollover or rollover_indicator;
//   - airbag_deployed, a bool that sets airbags to ["front"] when true;
//   - seatbelt or belt_status;
//   - number_of_occupants;
//   - vehicle_year, vehicle_make, vehicle_model, vehicle_color, vin;
//   - fuel_type or propulsion;
//   - latitude and longitude.
//
// Unknown keys are ignored. Parse returns an error only when the input is
// neither valid package JSON nor a recognised flat form.
func Parse(b []byte) (Crash, error) {
	trimmed := strings.TrimSpace(string(b))
	if trimmed == "" {
		return Crash{}, fmt.Errorf("acn: empty input")
	}

	// Try the package's own JSON first, but only accept it if it actually
	// carried a recognised field; an empty object is ambiguous.
	if strings.HasPrefix(trimmed, "{") {
		var native Crash
		if err := json.Unmarshal(b, &native); err == nil && !isZeroCrash(native) {
			return native, nil
		}

		// Fall back to a flat JSON object of common telematics keys.
		var flat map[string]json.RawMessage
		if err := json.Unmarshal(b, &flat); err == nil {
			kv := make(map[string]string, len(flat))
			for k, raw := range flat {
				kv[k] = rawToString(raw)
			}
			if c, ok := fromFlat(kv); ok {
				return c, nil
			}
		}
		return Crash{}, fmt.Errorf("acn: JSON input had no recognised crash fields")
	}

	// Otherwise treat it as "key=value" lines.
	kv, ok := parseKeyValues(trimmed)
	if !ok {
		return Crash{}, fmt.Errorf("acn: input is neither JSON nor key=value lines")
	}
	if c, ok := fromFlat(kv); ok {
		return c, nil
	}
	return Crash{}, fmt.Errorf("acn: no recognised crash fields in key=value input")
}

// isZeroCrash reports whether a Crash has none of the fields a real
// notification would carry.
func isZeroCrash(c Crash) bool {
	return c.Provider == "" && c.At.IsZero() && c.Lat == nil && c.Lon == nil &&
		c.Heading == 0 && c.DeltaVKmh == 0 && c.Direction == "" && !c.Rollover &&
		!c.MultipleImpacts && len(c.Airbags) == 0 && c.Occupants == 0 &&
		c.Belted == nil && c.SpeedKmh == 0 && c.Vehicle == (Vehicle{}) &&
		c.Fuel == "" && !c.VoiceConnected && c.Callback == ""
}

// rawToString renders a JSON raw value as a plain string: a JSON string becomes
// its contents, anything else becomes its literal JSON text.
func rawToString(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return str
	}
	return s
}

// parseKeyValues splits "key=value" lines into a map. It returns ok=false if no
// line contained a '=', which signals that the input was not key=value at all.
func parseKeyValues(s string) (map[string]string, bool) {
	kv := make(map[string]string)
	any := false
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		any = true
		key := strings.TrimSpace(line[:eq])
		val := strings.Trim(strings.TrimSpace(line[eq+1:]), `"'`)
		if key != "" {
			kv[strings.ToLower(key)] = val
		}
	}
	return kv, any
}

// fromFlat builds a Crash from a map of common telematics keys. It returns
// ok=false when no recognised key was present.
func fromFlat(kv map[string]string) (Crash, bool) {
	// Normalise keys to lower case for matching.
	m := make(map[string]string, len(kv))
	for k, v := range kv {
		m[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}

	var c Crash
	found := false

	get := func(keys ...string) (string, bool) {
		for _, k := range keys {
			if v, ok := m[k]; ok {
				return v, true
			}
		}
		return "", false
	}

	// delta-V: _mph suffix means miles per hour.
	for _, k := range []string{"delta_v_mph", "deltav_mph", "deltavmph", "deltav", "delta_v", "deltavkmh", "delta_v_kmh"} {
		if v, ok := m[k]; ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				if strings.HasSuffix(k, "_mph") || strings.HasSuffix(k, "mph") {
					c.DeltaVKmh = f * 1.609344
				} else {
					c.DeltaVKmh = f
				}
				found = true
			}
			break
		}
	}

	// Principal direction of force in degrees -> direction.
	if v, ok := get("principal_direction_of_force", "pdof"); ok {
		if deg, err := strconv.ParseFloat(v, 64); err == nil {
			c.Direction = directionFromDegrees(deg)
			found = true
		} else if d := normalizeDirection(v); d != "" {
			c.Direction = d
			found = true
		}
	} else if v, ok := get("direction"); ok {
		if d := normalizeDirection(v); d != "" {
			c.Direction = d
			found = true
		}
	}

	if v, ok := get("rollover", "rollover_indicator"); ok {
		c.Rollover = truthy(v)
		found = true
	}
	if v, ok := get("multiple_impacts", "multipleimpacts"); ok {
		c.MultipleImpacts = truthy(v)
		found = true
	}
	if v, ok := get("airbag_deployed", "airbag"); ok {
		if truthy(v) {
			c.Airbags = []string{"front"}
		}
		found = true
	}
	if v, ok := get("seatbelt", "belt_status", "belted"); ok {
		b := beltStatus(v)
		c.Belted = &b
		found = true
	}
	if v, ok := get("number_of_occupants", "occupants"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			c.Occupants = n
			found = true
		}
	}
	if v, ok := get("speed_kmh", "speedkmh"); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			c.SpeedKmh = f
			found = true
		}
	} else if v, ok := get("speed_mph"); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			c.SpeedKmh = f * 1.609344
			found = true
		}
	}

	// Vehicle fields.
	if v, ok := get("vehicle_year", "year"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			c.Vehicle.Year = n
			found = true
		}
	}
	if v, ok := get("vehicle_make", "make"); ok {
		c.Vehicle.Make = v
		found = true
	}
	if v, ok := get("vehicle_model", "model"); ok {
		c.Vehicle.Model = v
		found = true
	}
	if v, ok := get("vehicle_color", "color"); ok {
		c.Vehicle.Color = v
		found = true
	}
	if v, ok := get("vin"); ok {
		c.Vehicle.VIN = v
		found = true
	}
	if v, ok := get("plate", "vehicle_plate"); ok {
		c.Vehicle.Plate = v
		found = true
	}

	if v, ok := get("fuel_type", "propulsion", "fuel"); ok {
		c.Fuel = normalizeFuel(v)
		found = true
	}

	if v, ok := get("latitude", "lat"); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			lat := f
			c.Lat = &lat
			found = true
		}
	}
	if v, ok := get("longitude", "lon", "lng"); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			lon := f
			c.Lon = &lon
			found = true
		}
	}
	if v, ok := get("provider"); ok {
		c.Provider = v
		found = true
	}
	if v, ok := get("callback", "callback_number"); ok {
		c.Callback = v
		found = true
	}
	if v, ok := get("heading"); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			c.Heading = f
			found = true
		}
	}

	return c, found
}

// directionFromDegrees maps a principal direction of force in degrees to a
// direction: 315-45 front, 45-135 right, 135-225 rear, 225-315 left.
func directionFromDegrees(deg float64) string {
	d := math.Mod(deg, 360)
	if d < 0 {
		d += 360
	}
	switch {
	case d >= 315 || d < 45:
		return "front"
	case d < 135:
		return "right"
	case d < 225:
		return "rear"
	default:
		return "left"
	}
}

// normalizeDirection maps a free-text direction to front, rear, left, or right,
// or returns the empty string when it is not recognised.
func normalizeDirection(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "front", "frontal":
		return "front"
	case "rear", "back":
		return "rear"
	case "left", "driver":
		return "left"
	case "right", "passenger":
		return "right"
	}
	return ""
}

// normalizeFuel maps a free-text fuel or propulsion value to a canonical token.
func normalizeFuel(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "gas", "gasoline", "petrol":
		return "gasoline"
	case "diesel":
		return "diesel"
	case "electric", "ev", "bev":
		return "electric"
	case "hybrid", "phev", "hev":
		return "hybrid"
	case "hydrogen", "fcev":
		return "hydrogen"
	case "cng", "natural gas":
		return "cng"
	}
	return strings.ToLower(strings.TrimSpace(s))
}

// truthy reports whether a flat value means "true".
func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "t", "yes", "y", "on", "deployed", "detected":
		return true
	}
	return false
}

// beltStatus interprets a seatbelt value as "all front occupants belted".
func beltStatus(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "unbelted", "unbuckled", "0", "false", "no", "off", "none":
		return false
	default:
		return true
	}
}
