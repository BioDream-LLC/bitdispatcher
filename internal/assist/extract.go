// Package assist pulls key details out of a 911 call transcript and offers an offline emergency
// phrasebook translator, so a call-taker can read back facts and send ready phrases without a
// language model configured. Everything here is deterministic: the same input always gives the same
// output, nothing learns or changes itself, and no input can make it panic.
//
// This is an independent implementation for bitdispatcher (Apache-2.0).
package assist

import (
	"regexp"
	"strings"
)

// Detail is one fact found in what a caller said.
type Detail struct {
	Kind   string `json:"kind"`             // address, weapon, vehicle, plate, person, injury, hazard, count, direction, name
	Text   string `json:"text"`             // as normalised for display, e.g. "210 Church Ave SW", "handgun", "red Ford pickup", "VA ABC-1234", "not breathing", "2 people", "northbound on Williamson Rd", "Maria Lopez"
	Quote  string `json:"quote,omitempty"`  // the words it was found in (a short span, <= 80 chars)
	Urgent bool   `json:"urgent,omitempty"` // a weapon, not breathing, unconscious, fire with people inside, etc.
}

// maxInput caps how much of the transcript is processed, so a very long input cannot be slow.
const maxInput = 20000

// quoteCap is the longest quote span kept with a detail.
const quoteCap = 80

// suffixes maps a street-suffix word (lower case) to its normalised display form.
var suffixes = map[string]string{
	"st": "St", "street": "St",
	"ave": "Ave", "avenue": "Ave", "av": "Ave",
	"rd": "Rd", "road": "Rd",
	"blvd": "Blvd", "boulevard": "Blvd",
	"dr": "Dr", "drive": "Dr",
	"ln": "Ln", "lane": "Ln",
	"ct": "Ct", "court": "Ct",
	"way":  "Way",
	"pkwy": "Pkwy", "parkway": "Pkwy",
	"hwy": "Hwy", "highway": "Hwy",
	"pl": "Pl", "place": "Pl",
	"cir": "Cir", "circle": "Cir",
	"ter": "Ter", "terrace": "Ter",
}

// directions maps a spelt-out or abbreviated compass direction to its normalised display form.
var directions = map[string]string{
	"ne": "NE", "northeast": "NE",
	"nw": "NW", "northwest": "NW",
	"se": "SE", "southeast": "SE",
	"sw": "SW", "southwest": "SW",
	"n": "N", "north": "N",
	"s": "S", "south": "S",
	"e": "E", "east": "E",
	"w": "W", "west": "W",
}

// states maps US state names and abbreviations (lower case) to their two-letter form, for plates.
var states = map[string]string{
	"alabama": "AL", "al": "AL",
	"alaska": "AK", "ak": "AK",
	"arizona": "AZ", "az": "AZ",
	"arkansas": "AR", "ar": "AR",
	"california": "CA", "ca": "CA",
	"colorado": "CO", "co": "CO",
	"connecticut": "CT", "ct": "CT",
	"delaware": "DE", "de": "DE",
	"florida": "FL", "fl": "FL",
	"georgia": "GA", "ga": "GA",
	"hawaii": "HI", "hi": "HI",
	"idaho": "ID", "id": "ID",
	"illinois": "IL", "il": "IL",
	"indiana": "IN", "in": "IN",
	"iowa": "IA", "ia": "IA",
	"kansas": "KS", "ks": "KS",
	"kentucky": "KY", "ky": "KY",
	"louisiana": "LA", "la": "LA",
	"maine": "ME", "me": "ME",
	"maryland": "MD", "md": "MD",
	"massachusetts": "MA", "ma": "MA",
	"michigan": "MI", "mi": "MI",
	"minnesota": "MN", "mn": "MN",
	"mississippi": "MS", "ms": "MS",
	"missouri": "MO", "mo": "MO",
	"montana": "MT", "mt": "MT",
	"nebraska": "NE", "ne": "NE",
	"nevada": "NV", "nv": "NV",
	"new hampshire": "NH", "nh": "NH",
	"new jersey": "NJ", "nj": "NJ",
	"new mexico": "NM", "nm": "NM",
	"new york": "NY", "ny": "NY",
	"north carolina": "NC", "nc": "NC",
	"north dakota": "ND", "nd": "ND",
	"ohio": "OH", "oh": "OH",
	"oklahoma": "OK", "ok": "OK",
	"oregon": "OR", "or": "OR",
	"pennsylvania": "PA", "pa": "PA",
	"rhode island": "RI", "ri": "RI",
	"south carolina": "SC", "sc": "SC",
	"south dakota": "SD", "sd": "SD",
	"tennessee": "TN", "tn": "TN",
	"texas": "TX", "tx": "TX",
	"utah": "UT", "ut": "UT",
	"vermont": "VT", "vt": "VT",
	"virginia": "VA", "va": "VA",
	"washington": "WA", "wa": "WA",
	"west virginia": "WV", "wv": "WV",
	"wisconsin": "WI", "wi": "WI",
	"wyoming": "WY", "wy": "WY",
}

// numberWords maps spelt-out counts one..ten to their value.
var numberWords = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
	"six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
}

// vehicleMakes is the set of car makes recognised, lower case, with the normalised display form.
var vehicleMakes = map[string]string{
	"ford": "Ford", "chevy": "Chevy", "chevrolet": "Chevrolet",
	"toyota": "Toyota", "honda": "Honda", "nissan": "Nissan",
	"dodge": "Dodge", "ram": "Ram", "jeep": "Jeep", "gmc": "GMC",
	"hyundai": "Hyundai", "kia": "Kia", "subaru": "Subaru",
	"tesla": "Tesla", "bmw": "BMW", "mercedes": "Mercedes",
	"vw": "VW", "volkswagen": "Volkswagen",
}

// vehicleBodies is the set of body styles recognised, lower case.
var vehicleBodies = map[string]bool{
	"car": true, "truck": true, "pickup": true, "suv": true,
	"van": true, "minivan": true, "sedan": true, "motorcycle": true,
}

// colours is the set of car colours recognised, lower case.
var colours = map[string]bool{
	"red": true, "orange": true, "yellow": true, "green": true,
	"blue": true, "purple": true, "black": true, "white": true,
	"silver": true, "grey": true, "gray": true, "brown": true,
	"gold": true, "tan": true, "beige": true, "maroon": true,
}

// Precompiled patterns. All are case-insensitive.
var (
	// An address: a house number, one or more capitalised-or-plain street-name words, a suffix,
	// an optional direction, and an optional unit.
	reAddress = regexp.MustCompile(`(?i)\b(\d{1,6})\s+([a-z][a-z'.-]*(?:\s+[a-z][a-z'.-]*){0,3}?)\s+(st|street|ave|avenue|av|rd|road|blvd|boulevard|dr|drive|ln|lane|ct|court|way|pkwy|parkway|hwy|highway|pl|place|cir|circle|ter|terrace)\b\.?(\s+(?:northeast|northwest|southeast|southwest|north|south|east|west|ne|nw|se|sw|n|s|e|w)\b)?((?:\s*,?\s*(?:apt\.?|apartment|unit|suite|ste\.?|#)\s*[a-z0-9-]+))?`)

	// A mile marker on a numbered route, or an exit number.
	reMileMarker = regexp.MustCompile(`(?i)\b((?:i-?\s?\d{1,3}\s+)?mile\s*marker\s+\d{1,4})\b`)
	reExit       = regexp.MustCompile(`(?i)\b(exit\s+\d{1,4}[a-z]?)\b`)

	// An intersection phrase "corner of X and Y" or "X and Y" where both look like streets.
	reIntersection = regexp.MustCompile(`(?i)\b(?:corner of\s+)?([a-z][a-z'.-]*(?:\s+[a-z][a-z'.-]*){0,2}\s+(?:st|street|ave|avenue|av|rd|road|blvd|boulevard|dr|drive|ln|lane|ct|court|way|pkwy|parkway|hwy|highway|pl|place|cir|circle|ter|terrace))\s+and\s+([a-z][a-z'.-]*(?:\s+[a-z][a-z'.-]*){0,2}\s+(?:st|street|ave|avenue|av|rd|road|blvd|boulevard|dr|drive|ln|lane|ct|court|way|pkwy|parkway|hwy|highway|pl|place|cir|circle|ter|terrace))\b`)

	// A plate: an optional state, a lead word (plate/tag/license), and the plate characters.
	rePlate = regexp.MustCompile(`(?i)\b([a-z]+(?:\s[a-z]+)?\s+)?(?:license plate|number plate|plate|tag(?:\s+number)?|license|licence|registration)\s*(?:number\s+|#\s*)?([a-z0-9]{2,3}[-\s]?[a-z0-9]{2,4})\b`)

	// A person count: a number (digits or words) or "nobody"/"no one" with a people noun, or a
	// location word such as "inside" (so "nobody inside" and "two people inside" both count).
	reCount = regexp.MustCompile(`(?i)\b(nobody|no one|no-one|\d{1,3}|one|two|three|four|five|six|seven|eight|nine|ten)\s+(people|person|kids?|children|child|adults?|men|man|women|woman|victims?|occupants?|passengers?|students?|babies|baby|folks|guys|inside|outside|trapped|in the (?:house|building|car))\b`)

	// A direction of travel.
	reDirection = regexp.MustCompile(`(?i)\b(?:heading|headed|going|running|ran|runs|run|drove|driving|drives|drive|fled|fleeing|travelling|traveling|went|moving)\s+((?:north|south|east|west|northbound|southbound|eastbound|westbound|ne|nw|se|sw)\b(?:\s+(?:on|toward|towards|up|down|along)\s+[a-z0-9][a-z0-9'-]*(?:\s+[a-z0-9'-]+){0,3})?)`)

	// A caller giving a name.
	reName = regexp.MustCompile(`(?i)\b(?:my name is|this is|i am|i'm|name's|it's)\s+([A-Z][a-zA-Z'.-]+(?:\s+[A-Z][a-zA-Z'.-]+){0,2})`)

	// A plate's characters must contain at least one digit to be plausible.
	reHasDigit = regexp.MustCompile(`\d`)
)

// Extract pulls the facts a call-taker would otherwise type out of a transcript. It is deterministic
// and keyword/regex based, works on English input (including the English translation of a Spanish
// call), de-duplicates by kind and lower-case text keeping first-seen order, and never panics. Input
// beyond 20000 characters is ignored.
func Extract(text string) []Detail {
	if len(text) > maxInput {
		text = text[:maxInput]
	}
	lower := strings.ToLower(text)

	var out []Detail
	seen := map[string]bool{}
	add := func(d Detail) {
		key := d.Kind + "\x00" + strings.ToLower(d.Text)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, d)
	}

	extractAddresses(text, add)
	extractWeapons(text, lower, add)
	extractVehicles(text, add)
	extractPlates(text, add)
	extractCounts(text, add)
	extractInjuries(text, lower, add)
	extractHazards(text, lower, add)
	extractDirections(text, add)
	extractNames(text, add)

	// "bleeding badly" says all "bleeding" does.
	severe := false
	for _, d := range out {
		severe = severe || d.Kind == "injury" && d.Text == "bleeding badly"
	}
	if severe {
		kept := out[:0]
		for _, d := range out {
			if !(d.Kind == "injury" && d.Text == "bleeding") {
				kept = append(kept, d)
			}
		}
		out = kept
	}
	return out
}

// shortQuote trims a found span to the quote cap, on a word boundary where it can.
func shortQuote(s string) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	if len(s) <= quoteCap {
		return s
	}
	cut := s[:quoteCap]
	if i := strings.LastIndex(cut, " "); i > quoteCap/2 {
		cut = cut[:i]
	}
	return cut
}

// titleStreet normalises a street-name run to display casing: each word capitalised, except known
// lower-case joiners.
func titleStreet(name string) string {
	fields := strings.Fields(name)
	for i, w := range fields {
		lw := strings.ToLower(w)
		if i > 0 && (lw == "of" || lw == "the" || lw == "and") {
			fields[i] = lw
			continue
		}
		fields[i] = strings.ToUpper(lw[:1]) + lw[1:]
	}
	return strings.Join(fields, " ")
}

// extractAddresses finds street addresses, mile markers, exits and plausible intersections.
func extractAddresses(text string, add func(Detail)) {
	for _, m := range reAddress.FindAllStringSubmatch(text, -1) {
		num := m[1]
		name := titleStreet(m[2])
		suf := suffixes[strings.ToLower(strings.TrimSuffix(m[3], "."))]
		display := num + " " + name + " " + suf
		if d := strings.TrimSpace(m[4]); d != "" {
			if norm := directions[strings.ToLower(d)]; norm != "" {
				display += " " + norm
			}
		}
		if u := strings.TrimSpace(m[5]); u != "" {
			display += " " + normaliseUnit(u)
		}
		add(Detail{Kind: "address", Text: display, Quote: shortQuote(m[0])})
	}
	for _, m := range reMileMarker.FindAllStringSubmatch(text, -1) {
		add(Detail{Kind: "address", Text: normaliseMarker(m[1]), Quote: shortQuote(m[0])})
	}
	for _, m := range reExit.FindAllStringSubmatch(text, -1) {
		fields := strings.Fields(m[1])
		add(Detail{Kind: "address", Text: "Exit " + fields[len(fields)-1], Quote: shortQuote(m[0])})
	}
	for _, m := range reIntersection.FindAllStringSubmatch(text, -1) {
		a := titleStreetWithSuffix(m[1])
		b := titleStreetWithSuffix(m[2])
		add(Detail{Kind: "address", Text: a + " and " + b, Quote: shortQuote(m[0])})
	}
}

// titleStreetWithSuffix normalises a full street phrase that ends in a suffix word.
func titleStreetWithSuffix(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return s
	}
	last := strings.ToLower(fields[len(fields)-1])
	if norm, ok := suffixes[last]; ok {
		return titleStreet(strings.Join(fields[:len(fields)-1], " ")) + " " + norm
	}
	return titleStreet(s)
}

// normaliseUnit tidies a unit span such as "apartment 4" or "#B" into "Apt 4" or "#B".
func normaliseUnit(u string) string {
	u = strings.TrimSpace(strings.TrimLeft(u, " ,"))
	lu := strings.ToLower(u)
	switch {
	case strings.HasPrefix(lu, "apartment"), strings.HasPrefix(lu, "apt"):
		rest := strings.TrimSpace(u[strings.IndexAny(u, " ")+1:])
		if i := strings.IndexAny(lu, " "); i >= 0 {
			rest = strings.TrimSpace(u[i+1:])
		}
		return "Apt " + strings.ToUpper(rest)
	case strings.HasPrefix(lu, "unit"):
		return "Unit " + strings.ToUpper(strings.TrimSpace(u[4:]))
	case strings.HasPrefix(lu, "suite"), strings.HasPrefix(lu, "ste"):
		i := strings.IndexAny(u, " ")
		return "Suite " + strings.ToUpper(strings.TrimSpace(u[i+1:]))
	case strings.HasPrefix(u, "#"):
		return "#" + strings.ToUpper(strings.TrimSpace(u[1:]))
	}
	return u
}

// normaliseMarker tidies a mile-marker span into "I-81 mile marker 140" or "Mile marker 140".
func normaliseMarker(s string) string {
	fields := strings.Fields(s)
	n := fields[len(fields)-1]
	route := ""
	// A route prefix is only the leading field when it is an interstate token such as "I-81" or
	// "I81"; the words "mile" and "marker" are never a route.
	if len(fields) >= 1 {
		first := strings.ToLower(fields[0])
		if strings.HasPrefix(first, "i") && reHasDigit.MatchString(first) {
			r := strings.ToUpper(strings.ReplaceAll(first, " ", ""))
			if !strings.Contains(r, "-") && len(r) > 1 {
				r = r[:1] + "-" + r[1:]
			}
			route = r + " "
		}
	}
	return route + "Mile marker " + n
}

// extractWeapons finds mentions of weapons, all of which are urgent.
func extractWeapons(text, lower string, add func(Detail)) {
	weapons := []string{"handgun", "shotgun", "rifle", "pistol", "machete", "firearm", "gun", "knife", "bat", "crowbar", "hammer", "sword", "axe"}
	for _, w := range weapons {
		re := regexp.MustCompile(`(?i)\b` + w + `s?\b`)
		if loc := re.FindStringIndex(lower); loc != nil {
			add(Detail{Kind: "weapon", Text: w, Quote: shortQuote(spanAround(text, loc[0], loc[1])), Urgent: true})
		}
	}
}

// spanAround returns a few words of context either side of a byte range.
func spanAround(text string, start, end int) string {
	lo := start - 25
	if lo < 0 {
		lo = 0
	}
	hi := end + 25
	if hi > len(text) {
		hi = len(text)
	}
	return text[lo:hi]
}

// extractVehicles finds vehicle descriptions: an optional colour, an optional make, an optional body.
func extractVehicles(text string, add func(Detail)) {
	words := tokenise(text)
	for i := 0; i < len(words); i++ {
		var colour, make, body string
		var start, end int = -1, -1
		j := i
		if colours[strings.ToLower(words[j].text)] {
			colour = strings.ToLower(words[j].text)
			start, end = words[j].start, words[j].end
			j++
		}
		if j < len(words) {
			if mk, ok := vehicleMakes[strings.ToLower(words[j].text)]; ok {
				make = mk
				if start < 0 {
					start = words[j].start
				}
				end = words[j].end
				j++
				// A model word may follow the make; keep it when the body style follows or it is
				// a plain capitalised token such as "Civic".
				if j < len(words) && !vehicleBodies[strings.ToLower(words[j].text)] && isModelWord(words[j].text) {
					make += " " + words[j].text
					end = words[j].end
					j++
				}
			}
		}
		if j < len(words) {
			if vehicleBodies[strings.ToLower(words[j].text)] {
				body = strings.ToLower(words[j].text)
				if start < 0 {
					start = words[j].start
				}
				end = words[j].end
				j++
			}
		}
		if make == "" && body == "" {
			continue
		}
		var parts []string
		if colour != "" {
			parts = append(parts, colour)
		}
		if make != "" {
			parts = append(parts, make)
		}
		if body != "" {
			parts = append(parts, body)
		}
		add(Detail{Kind: "vehicle", Text: strings.Join(parts, " "), Quote: shortQuote(text[start:end])})
		i = j - 1
	}
}

// isModelWord reports whether a token looks like a car model: capitalised or alphanumeric, short.
func isModelWord(s string) bool {
	if len(s) == 0 || len(s) > 12 {
		return false
	}
	r := s[0]
	return (r >= 'A' && r <= 'Z')
}

// token is a word with its byte span in the original text.
type token struct {
	text       string
	start, end int
}

// reWord splits text into word tokens while remembering their positions.
var reWord = regexp.MustCompile(`[A-Za-z0-9'.-]+`)

// tokenise splits text into positioned word tokens.
func tokenise(text string) []token {
	locs := reWord.FindAllStringIndex(text, -1)
	toks := make([]token, 0, len(locs))
	for _, l := range locs {
		toks = append(toks, token{text: text[l[0]:l[1]], start: l[0], end: l[1]})
	}
	return toks
}

// extractPlates finds licence plates, uppercased, with an optional state prefix.
func extractPlates(text string, add func(Detail)) {
	for _, m := range rePlate.FindAllStringSubmatch(text, -1) {
		chars := strings.ToUpper(strings.TrimSpace(m[2]))
		if !reHasDigit.MatchString(chars) {
			continue
		}
		chars = strings.ReplaceAll(chars, " ", "-")
		display := chars
		if pre := strings.TrimSpace(m[1]); pre != "" {
			if st, ok := states[strings.ToLower(pre)]; ok {
				display = st + " " + chars
			}
		}
		add(Detail{Kind: "plate", Text: display, Quote: shortQuote(m[0])})
	}
}

// extractCounts finds how many people are involved, including the meaningful zero of "nobody inside".
func extractCounts(text string, add func(Detail)) {
	for _, m := range reCount.FindAllStringSubmatch(text, -1) {
		word := strings.ToLower(m[1])
		noun := strings.ToLower(m[2])
		var n int
		switch word {
		case "nobody", "no one", "no-one":
			n = 0
		default:
			if v, ok := numberWords[word]; ok {
				n = v
			} else {
				n = atoiSafe(word)
			}
		}
		label := countNoun(noun, n)
		add(Detail{Kind: "count", Text: itoa(n) + " " + label, Quote: shortQuote(m[0])})
	}
}

// countNoun maps a matched noun to a stable display noun, singular or plural by count.
func countNoun(noun string, n int) string {
	switch noun {
	case "kid", "kids", "child", "children":
		if n == 1 {
			return "child"
		}
		return "children"
	case "person", "people", "folks", "guys":
		if n == 1 {
			return "person"
		}
		return "people"
	case "baby", "babies":
		if n == 1 {
			return "baby"
		}
		return "babies"
	}
	plural := map[string][2]string{"adult": {"adult", "adults"}, "adults": {"adult", "adults"}, "man": {"man", "men"}, "men": {"man", "men"},
		"woman": {"woman", "women"}, "women": {"woman", "women"}, "victim": {"victim", "victims"}, "victims": {"victim", "victims"},
		"occupant": {"occupant", "occupants"}, "occupants": {"occupant", "occupants"}, "passenger": {"passenger", "passengers"},
		"passengers": {"passenger", "passengers"}, "student": {"student", "students"}, "students": {"student", "students"}}
	if f, ok := plural[noun]; ok {
		if n == 1 {
			return f[0]
		}
		return f[1]
	}
	if n == 1 {
		return "person"
	}
	return "people"
}

// injuryRule is one injury keyword, its display text, and whether it is urgent.
type injuryRule struct {
	pattern *regexp.Regexp
	text    string
	urgent  bool
}

// injuryRules are the medical conditions recognised, evaluated in order.
var injuryRules = []injuryRule{
	{regexp.MustCompile(`(?i)\bnot breathing\b|\bisn't breathing\b|\bis not breathing\b|\bstopped breathing\b`), "not breathing", true},
	{regexp.MustCompile(`(?i)\bunconscious\b|\bunresponsive\b|\bpassed out\b|\bwon't wake up\b|\bwont wake up\b`), "unconscious", true},
	{regexp.MustCompile(`(?i)\bcan't breathe\b|\bcannot breathe\b|\bcant breathe\b|\btrouble breathing\b|\bhard to breathe\b|\bstruggling to breathe\b`), "trouble breathing", true},
	{regexp.MustCompile(`(?i)\boverdose\b|\bod'?ing\b|\bod'?ed\b`), "overdose", true},
	{regexp.MustCompile(`(?i)\bseizure\b|\bconvulsing\b|\bconvulsions?\b`), "seizure", false},
	{regexp.MustCompile(`(?i)\bchest pain\b|\bpain in (?:his|her|my|their) chest\b`), "chest pain", false},
	{regexp.MustCompile(`(?i)\bstroke\b`), "stroke", false},
	{regexp.MustCompile(`(?i)\bbleeding (?:badly|a lot|heavily|everywhere)\b|\bbleeding out\b`), "bleeding badly", true},
	{regexp.MustCompile(`(?i)\bbleeding\b|\bbleed\b`), "bleeding", false},
	{regexp.MustCompile(`(?i)\b(?:in labor|in labour|going into labor|going into labour)\b`), "in labour", false},
	{regexp.MustCompile(`(?i)\bpregnant\b`), "pregnant", false},
	{regexp.MustCompile(`(?i)\bfell\b|\bhad a fall\b|\btook a fall\b|\bfallen\b`), "fell", false},
	{regexp.MustCompile(`(?i)\bbroken\b|\bbroke (?:his|her|my|their|a)\b|\bfractured?\b`), "broken bone", false},
	{regexp.MustCompile(`(?i)\bburns?\b|\bburned\b|\bburnt\b`), "burn", false},
}

// extractInjuries finds medical conditions in priority order.
func extractInjuries(text, lower string, add func(Detail)) {
	for _, r := range injuryRules {
		if loc := r.pattern.FindStringIndex(text); loc != nil {
			add(Detail{Kind: "injury", Text: r.text, Quote: shortQuote(text[loc[0]:loc[1]]), Urgent: r.urgent})
		}
	}
}

// hazardRule is one hazard keyword, its display text, and whether it is urgent on its own.
type hazardRule struct {
	pattern *regexp.Regexp
	text    string
	urgent  bool
}

// hazardRules are the scene hazards recognised, evaluated in order.
var hazardRules = []hazardRule{
	{regexp.MustCompile(`(?i)\bflames?\b`), "fire", false},
	{regexp.MustCompile(`(?i)\bfire\b|\bon fire\b|\bburning\b`), "fire", false},
	{regexp.MustCompile(`(?i)\bsmoke\b`), "smoke", false},
	{regexp.MustCompile(`(?i)\btrapped\b|\bstuck inside\b|\bpinned\b`), "trapped", true},
	{regexp.MustCompile(`(?i)\bsmell(?:s|ing)? (?:of )?gas\b|\bgas smell\b|\bgas leak\b|\bsmell gas\b`), "gas smell", true},
	{regexp.MustCompile(`(?i)\bpower lines?\b|\bwires? (?:are )?down\b|\bdowned? (?:power )?lines?\b|\blines? (?:are )?down\b`), "downed power lines", true},
	{regexp.MustCompile(`(?i)\bchemical\b|\bchemicals\b`), "chemical", false},
	{regexp.MustCompile(`(?i)\boxygen tank\b`), "oxygen tank", false},
	{regexp.MustCompile(`(?i)\belectric car\b|\bEV battery\b|\bEV fire\b|\belectric vehicle\b`), "EV battery", false},
	{regexp.MustCompile(`(?i)\bdogs?\b`), "dog", false},
}

// reInside matches wording that puts people at the scene of a fire.
var reInside = regexp.MustCompile(`(?i)\b(?:inside|trapped|still in|in the house|in the building|upstairs|downstairs|people|kids?|children|someone|somebody)\b`)

// extractHazards finds scene hazards. Fire is urgent when people are reported inside or trapped.
func extractHazards(text, lower string, add func(Detail)) {
	peopleInside := reInside.MatchString(lower)
	for _, r := range hazardRules {
		if loc := r.pattern.FindStringIndex(text); loc != nil {
			urgent := r.urgent
			if (r.text == "fire" || r.text == "smoke") && peopleInside {
				urgent = true
			}
			add(Detail{Kind: "hazard", Text: r.text, Quote: shortQuote(text[loc[0]:loc[1]]), Urgent: urgent})
		}
	}
}

// extractDirections finds a direction of travel, normalised for display.
func extractDirections(text string, add func(Detail)) {
	for _, m := range reDirection.FindAllStringSubmatch(text, -1) {
		add(Detail{Kind: "direction", Text: normaliseDirection(m[1]), Quote: shortQuote(m[0])})
	}
}

// normaliseDirection tidies a travel phrase, keeping a trailing street name's casing.
func normaliseDirection(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return s
	}
	head := strings.ToLower(fields[0])
	switch head {
	case "north":
		head = "northbound"
	case "south":
		head = "southbound"
	case "east":
		head = "eastbound"
	case "west":
		head = "westbound"
	case "ne":
		head = "northeast-bound"
	case "nw":
		head = "northwest-bound"
	case "se":
		head = "southeast-bound"
	case "sw":
		head = "southwest-bound"
	}
	if len(fields) == 1 {
		return head
	}
	rest := fields[1:]
	// "on"/"toward" join word stays lower case; the street name after it gets display casing.
	for i, w := range rest {
		lw := strings.ToLower(w)
		if lw == "on" || lw == "toward" || lw == "towards" || lw == "up" || lw == "down" || lw == "along" {
			rest[i] = lw
			continue
		}
		rest[i] = titleStreet(w)
	}
	return head + " " + strings.Join(rest, " ")
}

// extractNames finds a caller stating their own or someone's name.
func extractNames(text string, add func(Detail)) {
	for _, m := range reName.FindAllStringSubmatch(text, -1) {
		name := trimName(strings.TrimSpace(m[1]))
		if name == "" || looksLikeStopword(name) {
			continue
		}
		add(Detail{Kind: "name", Text: name, Quote: shortQuote(m[0])})
	}
}

// trimName drops trailing words that are verbs or fillers rather than part of a name, such as the
// "calling" in "this is John Smith calling".
func trimName(name string) string {
	fields := strings.Fields(name)
	for len(fields) > 0 {
		last := strings.ToLower(fields[len(fields)-1])
		if trailingStops[last] {
			fields = fields[:len(fields)-1]
			continue
		}
		break
	}
	return strings.Join(fields, " ")
}

// trailingStops are words that are never the last token of a name.
var trailingStops = map[string]bool{
	"calling": true, "speaking": true, "here": true, "again": true,
	"now": true, "and": true, "at": true, "from": true, "on": true,
	"please": true, "trying": true, "with": true,
}

// stopNames are capitalised words that follow "this is"/"I am" but are not names.
var stopNames = map[string]bool{
	"calling": true, "trying": true, "the": true, "not": true, "at": true,
	"going": true, "really": true, "so": true, "very": true, "an": true,
	"a": true, "here": true, "there": true, "with": true, "in": true,
}

// looksLikeStopword reports whether a captured name is actually a non-name word.
func looksLikeStopword(name string) bool {
	first := strings.ToLower(strings.Fields(name)[0])
	return stopNames[first]
}

// atoiSafe parses a small decimal integer, returning 0 on failure.
func atoiSafe(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
		if n > 100000 {
			return n
		}
	}
	return n
}

// itoa renders a small non-negative integer without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
