package nemsis

import "strings"

// ansiStateCodes maps the two-letter USPS state/territory code to its ANSI (FIPS) two-digit code, which
// NEMSIS uses for dAgency.04 and eScene.18. These are the standard FIPS 5-2 state/territory numbers.
var ansiStateCodes = map[string]string{
	"AL": "01", "AK": "02", "AZ": "04", "AR": "05", "CA": "06", "CO": "08",
	"CT": "09", "DE": "10", "DC": "11", "FL": "12", "GA": "13", "HI": "15",
	"ID": "16", "IL": "17", "IN": "18", "IA": "19", "KS": "20", "KY": "21",
	"LA": "22", "ME": "23", "MD": "24", "MA": "25", "MI": "26", "MN": "27",
	"MS": "28", "MO": "29", "MT": "30", "NE": "31", "NV": "32", "NH": "33",
	"NJ": "34", "NM": "35", "NY": "36", "NC": "37", "ND": "38", "OH": "39",
	"OK": "40", "OR": "41", "PA": "42", "RI": "44", "SC": "45", "SD": "46",
	"TN": "47", "TX": "48", "UT": "49", "VT": "50", "VA": "51", "WA": "53",
	"WV": "54", "WI": "55", "WY": "56",
	// Territories and associated areas.
	"AS": "60", "GU": "66", "MP": "69", "PR": "72", "VI": "78",
}

// ansiState returns the ANSI (FIPS) state code for a two-letter USPS code, or "" when unknown. A value
// that is already a two-digit ANSI code is accepted as is.
func ansiState(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	if code, ok := ansiStateCodes[s]; ok {
		return code
	}
	if len(s) == 2 && s[0] >= '0' && s[0] <= '9' && s[1] >= '0' && s[1] <= '9' {
		for _, code := range ansiStateCodes {
			if code == s {
				return s
			}
		}
	}
	return ""
}
