package vehcodes

import (
	"fmt"
	"strings"
)

// VINCheck is the result of validating a vehicle identification number. A VIN
// that is not 17 characters (for example a documented partial read or a
// pre-1981 VIN) is reported with Complete=false and Valid=false but is not
// treated as an error that blocks saving: the CAD allows documented partial
// VINs.
type VINCheck struct {
	VIN        string   `json:"vin"`
	Complete   bool     `json:"complete"` // 17 characters
	Valid      bool     `json:"valid"`    // complete, allowed chars, check digit ok
	CheckDigit bool     `json:"checkDigit"`
	Year       string   `json:"year,omitempty"` // model year from position 10
	WMI        string   `json:"wmi,omitempty"`
	Problems   []string `json:"problems,omitempty"`
}

// transliteration maps each allowed VIN letter to its numeric value for the
// check-digit calculation per 49 CFR 565. I, O and Q are never allowed in a
// VIN and are deliberately absent.
var transliteration = map[rune]int{
	'A': 1, 'B': 2, 'C': 3, 'D': 4, 'E': 5, 'F': 6, 'G': 7, 'H': 8,
	'J': 1, 'K': 2, 'L': 3, 'M': 4, 'N': 5, 'P': 7, 'R': 9,
	'S': 2, 'T': 3, 'U': 4, 'V': 5, 'W': 6, 'X': 7, 'Y': 8, 'Z': 9,
}

// positionWeights are the weighting factors applied to each of the 17 VIN
// positions per 49 CFR 565. Position 9 (index 8) carries the check digit and
// has weight 0.
var positionWeights = [17]int{8, 7, 6, 5, 4, 3, 2, 10, 0, 9, 8, 7, 6, 5, 4, 3, 2}

// yearCodes maps the model-year character in position 10 to the two years in
// the 30-year cycle that share that code. The letters I, O, Q, U, Z and the
// digit 0 are not used as model-year codes.
var yearCodes = map[byte][2]string{
	'A': {"1980", "2010"},
	'B': {"1981", "2011"},
	'C': {"1982", "2012"},
	'D': {"1983", "2013"},
	'E': {"1984", "2014"},
	'F': {"1985", "2015"},
	'G': {"1986", "2016"},
	'H': {"1987", "2017"},
	'J': {"1988", "2018"},
	'K': {"1989", "2019"},
	'L': {"1990", "2020"},
	'M': {"1991", "2021"},
	'N': {"1992", "2022"},
	'P': {"1993", "2023"},
	'R': {"1994", "2024"},
	'S': {"1995", "2025"},
	'T': {"1996", "2026"},
	'V': {"1997", "2027"},
	'W': {"1998", "2028"},
	'X': {"1999", "2029"},
	'Y': {"2000", "2030"},
	'1': {"2001", "2031"},
	'2': {"2002", "2032"},
	'3': {"2003", "2033"},
	'4': {"2004", "2034"},
	'5': {"2005", "2035"},
	'6': {"2006", "2036"},
	'7': {"2007", "2037"},
	'8': {"2008", "2038"},
	'9': {"2009", "2039"},
}

// isAllowedVINChar reports whether c is a legal VIN character: a digit, or an
// uppercase letter other than I, O or Q.
func isAllowedVINChar(c byte) bool {
	switch {
	case c >= '0' && c <= '9':
		return true
	case c >= 'A' && c <= 'Z':
		return c != 'I' && c != 'O' && c != 'Q'
	default:
		return false
	}
}

// CheckVIN validates a vehicle identification number. Input is uppercased and
// spaces and dashes are stripped. The returned VINCheck records whether the VIN
// is complete (17 characters), whether the check digit matches, the decoded
// model year, the World Manufacturer Identifier, and any problems found.
//
// A VIN is Valid only when it is complete, uses allowed characters, and its
// check digit matches. Non-North-American VINs may not use a real check digit;
// such a mismatch is reported in CheckDigit and Problems rather than as a
// character error.
func CheckVIN(s string) VINCheck {
	clean := strings.ToUpper(s)
	clean = strings.ReplaceAll(clean, " ", "")
	clean = strings.ReplaceAll(clean, "-", "")

	r := VINCheck{VIN: clean}

	// Detect disallowed characters. I, O and Q get a specific message.
	var badChars []string
	seenIOQ := false
	for _, c := range []byte(clean) {
		if c == 'I' || c == 'O' || c == 'Q' {
			if !seenIOQ {
				r.Problems = append(r.Problems, "contains I, O or Q, which are not allowed in a VIN")
				seenIOQ = true
			}
			continue
		}
		if !isAllowedVINChar(c) {
			badChars = append(badChars, string(c))
		}
	}
	if len(badChars) > 0 {
		r.Problems = append(r.Problems, fmt.Sprintf("contains characters not allowed in a VIN: %s", strings.Join(badChars, " ")))
	}
	charsOK := !seenIOQ && len(badChars) == 0

	r.Complete = len(clean) == 17
	if !r.Complete {
		r.Problems = append(r.Problems, fmt.Sprintf("partial VIN: %d of 17 characters", len(clean)))
	}

	// WMI (first 3 chars) and model year (position 10) when they are present
	// and all characters so far are legal.
	if charsOK {
		if len(clean) >= 3 {
			r.WMI = clean[:3]
		}
		if len(clean) >= 10 {
			if years, ok := yearCodes[clean[9]]; ok {
				r.Year = years[0] + " or " + years[1]
			}
		}
	}

	// Check digit: only meaningful for a complete VIN with legal characters.
	if r.Complete && charsOK {
		sum := 0
		valid := true
		for i := 0; i < 17; i++ {
			c := rune(clean[i])
			var val int
			switch {
			case c >= '0' && c <= '9':
				val = int(c - '0')
			default:
				v, ok := transliteration[c]
				if !ok {
					valid = false
					break
				}
				val = v
			}
			sum += val * positionWeights[i]
		}
		if valid {
			rem := sum % 11
			var want byte
			if rem == 10 {
				want = 'X'
			} else {
				want = byte('0' + rem)
			}
			if clean[8] == want {
				r.CheckDigit = true
			} else {
				r.CheckDigit = false
				r.Problems = append(r.Problems, "check digit does not match (vehicles built for outside North America may not use one)")
			}
		}
	}

	r.Valid = r.Complete && charsOK && r.CheckDigit
	return r
}
