package mapdata

import (
	"strings"
	"unicode"
)

// words maps USPS street suffix and directional abbreviations to one spelling, so "123 Campbell Ave SW",
// "123 campbell avenue southwest" and "123 Campbell Av S.W." all find the same address.
var words = map[string]string{
	"st": "street", "str": "street", "rd": "road", "ave": "avenue", "av": "avenue", "avn": "avenue", "dr": "drive", "drv": "drive",
	"ln": "lane", "blvd": "boulevard", "boul": "boulevard", "ct": "court", "crt": "court", "pl": "place", "cir": "circle", "crcl": "circle",
	"pkwy": "parkway", "pky": "parkway", "hwy": "highway", "hiwy": "highway", "ter": "terrace", "terr": "terrace", "trl": "trail",
	"tr": "trail", "sq": "square", "aly": "alley", "expy": "expressway", "fwy": "freeway", "tpke": "turnpike", "cv": "cove",
	"xing": "crossing", "pt": "point", "mtn": "mountain", "mt": "mount", "ext": "extension", "hl": "hill", "hts": "heights",
	"vw": "view", "rdg": "ridge", "spg": "spring", "spgs": "springs", "lk": "lake", "lks": "lakes", "crk": "creek", "br": "branch",
	"brg": "bridge", "byp": "bypass", "cswy": "causeway", "ctr": "center", "est": "estate", "ests": "estates", "frk": "fork",
	"gdn": "garden", "gdns": "gardens", "grv": "grove", "hbr": "harbor", "holw": "hollow", "jct": "junction", "lndg": "landing",
	"loop": "loop", "mdw": "meadow", "mdws": "meadows", "ml": "mill", "orch": "orchard", "pass": "pass", "pike": "pike",
	"pne": "pine", "pnes": "pines", "riv": "river", "rte": "route", "rt": "route", "shr": "shore", "sta": "station", "vly": "valley",
	"vlg": "village", "wy": "way", "wls": "wells",
	"n": "north", "s": "south", "e": "east", "w": "west", "ne": "northeast", "nw": "northwest", "se": "southeast", "sw": "southwest",
	"ft": "fort", "sr": "state route", "i": "interstate",
}

// Normalize turns an address, street or place name into its search form: lower case, punctuation gone,
// abbreviations spelled out.
func Normalize(s string) string {
	s = strings.ToLower(s)
	// S.W. → sw before splitting on punctuation.
	s = strings.ReplaceAll(s, ".", "")
	f := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '&' })
	for i, w := range f {
		if v, ok := words[w]; ok {
			f[i] = v
		}
	}
	return strings.Join(f, " ")
}

// splitNumber separates a leading house number ("1234", "1234A", "1234-B") from the rest.
func splitNumber(q string) (num, rest string) {
	q = strings.TrimSpace(q)
	i := 0
	for i < len(q) && q[i] >= '0' && q[i] <= '9' {
		i++
	}
	if i == 0 {
		return "", q
	}
	j := i
	for j < len(q) && (q[j] == '-' || unicode.IsLetter(rune(q[j]))) && j-i < 2 {
		j++
	}
	switch strings.ToLower(q[i:j]) {
	case "st", "nd", "rd", "th":
		return "", q // "12th Street": a numbered street, not a house number
	}
	if j < len(q) && q[j] != ' ' {
		j = i // "12th street": a numbered street, not a house number
		if i < len(q) && q[i] != ' ' {
			return "", q
		}
	}
	return strings.ToUpper(strings.Trim(q[:j], "-")), strings.TrimSpace(q[j:])
}
