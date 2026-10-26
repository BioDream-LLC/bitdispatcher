// Package vehcodes provides controlled code lists for a 911 CAD vehicle and
// person entry form. Each list pairs an approved short code (NCIC 2000 style)
// with a plain-language label so a dispatcher sees both the code that will be
// transmitted and what it means. Only codes that are well established in the
// FBI NCIC 2000 Code Manual are built in; agencies add the remainder in Admin,
// and Merge applies those agency additions and overrides on top of a built-in
// list. The package also validates vehicle identification numbers (see vin.go).
//
// The package uses the standard library only and holds no mutable global state:
// Lists and Get return copies safe for the caller to read, and Merge returns a
// new list without changing its input.
package vehcodes

import (
	"sort"
	"strings"
)

// Code is one entry in a controlled list: the approved short code and its
// plain-language label.
type Code struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

// List is one controlled list of codes with identifying metadata. Source names
// where the codes come from in honest terms.
type List struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source"`
	Codes  []Code `json:"codes"`
}

// ncicSource is the honest provenance string for lists drawn from the NCIC 2000
// Code Manual: only common, high-confidence codes are built in.
const ncicSource = "NCIC 2000 Code Manual (common codes; agencies add the rest in Admin)"

// listOrder fixes the stable order in which Lists returns the built-in lists.
var listOrder = []string{
	"state",
	"plateType",
	"plateCondition",
	"color",
	"body",
	"make",
	"hair",
	"eyes",
	"sex",
	"race",
	"ethnicity",
}

// builtins holds every built-in list keyed by ID.
var builtins = map[string]List{
	"state": {
		ID:     "state",
		Name:   "State / Province",
		Source: ncicSource,
		Codes: []Code{
			// US states by USPS 2-letter code.
			{"AL", "Alabama"},
			{"AK", "Alaska"},
			{"AZ", "Arizona"},
			{"AR", "Arkansas"},
			{"CA", "California"},
			{"CO", "Colorado"},
			{"CT", "Connecticut"},
			{"DE", "Delaware"},
			{"DC", "District of Columbia"},
			{"FL", "Florida"},
			{"GA", "Georgia"},
			{"HI", "Hawaii"},
			{"ID", "Idaho"},
			{"IL", "Illinois"},
			{"IN", "Indiana"},
			{"IA", "Iowa"},
			{"KS", "Kansas"},
			{"KY", "Kentucky"},
			{"LA", "Louisiana"},
			{"ME", "Maine"},
			{"MD", "Maryland"},
			{"MA", "Massachusetts"},
			{"MI", "Michigan"},
			{"MN", "Minnesota"},
			{"MS", "Mississippi"},
			{"MO", "Missouri"},
			{"MT", "Montana"},
			{"NE", "Nebraska"},
			{"NV", "Nevada"},
			{"NH", "New Hampshire"},
			{"NJ", "New Jersey"},
			{"NM", "New Mexico"},
			{"NY", "New York"},
			{"NC", "North Carolina"},
			{"ND", "North Dakota"},
			{"OH", "Ohio"},
			{"OK", "Oklahoma"},
			{"OR", "Oregon"},
			{"PA", "Pennsylvania"},
			{"RI", "Rhode Island"},
			{"SC", "South Carolina"},
			{"SD", "South Dakota"},
			{"TN", "Tennessee"},
			{"TX", "Texas"},
			{"UT", "Utah"},
			{"VT", "Vermont"},
			{"VA", "Virginia"},
			{"WA", "Washington"},
			{"WV", "West Virginia"},
			{"WI", "Wisconsin"},
			{"WY", "Wyoming"},
			// US territories.
			{"PR", "Puerto Rico"},
			{"GU", "Guam"},
			{"VI", "U.S. Virgin Islands"},
			{"AS", "American Samoa"},
			{"MP", "Northern Mariana Islands"},
			// Canadian provinces and territories. NCIC uses NF for
			// Newfoundland and PQ for Quebec; NU for Nunavut is omitted as
			// lower confidence.
			{"AB", "Alberta"},
			{"BC", "British Columbia"},
			{"MB", "Manitoba"},
			{"NB", "New Brunswick"},
			{"NF", "Newfoundland and Labrador"},
			{"NT", "Northwest Territories"},
			{"NS", "Nova Scotia"},
			{"ON", "Ontario"},
			{"PE", "Prince Edward Island"},
			{"PQ", "Quebec"},
			{"SK", "Saskatchewan"},
			{"YT", "Yukon"},
			// Other.
			{"MX", "Mexico"},
			{"US", "United States Government"},
		},
	},
	"plateType": {
		ID:     "plateType",
		Name:   "License Plate Type",
		Source: ncicSource,
		Codes: []Code{
			{"PC", "Passenger"},
			{"TK", "Truck"},
			{"MC", "Motorcycle"},
			{"TL", "Trailer"},
			{"AP", "Apportioned"},
			{"DL", "Dealer"},
			{"CO", "Commercial"},
			{"AM", "Amateur Radio"},
		},
	},
	"plateCondition": {
		ID:     "plateCondition",
		Name:   "Plate Condition",
		Source: "bitdispatcher",
		Codes: []Code{
			{"NONE", "No plate"},
			{"TEMP", "Temporary"},
			{"PARTIAL", "Partial"},
			{"UNREADABLE", "Unreadable"},
			{"UNKNOWN", "Unknown"},
			{"MULTIPLE", "Multiple"},
		},
	},
	"color": {
		ID:     "color",
		Name:   "Color",
		Source: ncicSource,
		Codes: []Code{
			{"BLK", "Black"},
			{"BLU", "Blue"},
			{"BRO", "Brown"},
			{"GRN", "Green"},
			{"GRY", "Gray"},
			{"MAR", "Maroon"},
			{"ONG", "Orange"},
			{"PNK", "Pink"},
			{"PLE", "Purple"},
			{"RED", "Red"},
			{"SIL", "Silver"},
			{"TAN", "Tan"},
			{"WHI", "White"},
			{"YEL", "Yellow"},
			{"GLD", "Gold"},
			{"BGE", "Beige"},
			{"TRQ", "Turquoise"},
			{"CRM", "Cream or Ivory"},
			{"DBL", "Dark Blue"},
			{"LBL", "Light Blue"},
			{"DGR", "Dark Green"},
			{"LGR", "Light Green"},
			{"TEA", "Teal"},
			{"BRZ", "Bronze"},
			{"CAM", "Camouflage"},
		},
	},
	"body": {
		ID:     "body",
		Name:   "Body Style",
		Source: ncicSource,
		Codes: []Code{
			{"2D", "Two-door"},
			{"4D", "Four-door"},
			{"CV", "Convertible"},
			{"HB", "Hatchback"},
			{"SW", "Station Wagon"},
			{"PK", "Pickup"},
			{"VN", "Van"},
			{"UT", "Utility (SUV)"},
		},
	},
	"make": {
		ID:     "make",
		Name:   "Vehicle Make",
		Source: ncicSource,
		Codes: []Code{
			{"ACUR", "Acura"},
			{"AUDI", "Audi"},
			{"BMW", "BMW"},
			{"BUIC", "Buick"},
			{"CADI", "Cadillac"},
			{"CHEV", "Chevrolet"},
			{"CHRY", "Chrysler"},
			{"DODG", "Dodge"},
			{"FORD", "Ford"},
			{"GMC", "GMC"},
			{"HOND", "Honda"},
			{"HYUN", "Hyundai"},
			{"INFI", "Infiniti"},
			{"JAGU", "Jaguar"},
			{"JEEP", "Jeep"},
			{"KIA", "Kia"},
			{"LEXS", "Lexus"},
			{"LINC", "Lincoln"},
			{"MAZD", "Mazda"},
			{"MERZ", "Mercedes-Benz"},
			{"MERC", "Mercury"},
			{"MITS", "Mitsubishi"},
			{"NISS", "Nissan"},
			{"PONT", "Pontiac"},
			{"PORS", "Porsche"},
			{"SATR", "Saturn"},
			{"SUBA", "Subaru"},
			{"SUZI", "Suzuki"},
			{"TESL", "Tesla"},
			{"TOYT", "Toyota"},
			{"VOLK", "Volkswagen"},
			{"VOLV", "Volvo"},
			{"HD", "Harley-Davidson"},
			{"KAWK", "Kawasaki"},
			{"YAMA", "Yamaha"},
			{"FRHT", "Freightliner"},
			{"INTL", "International"},
			{"KW", "Kenworth"},
			{"PTRB", "Peterbilt"},
			{"MACK", "Mack"},
		},
	},
	"hair": {
		ID:     "hair",
		Name:   "Hair Color",
		Source: ncicSource,
		Codes: []Code{
			{"BLK", "Black"},
			{"BLN", "Blond or Strawberry"},
			{"BRO", "Brown"},
			{"GRY", "Gray or Partially Gray"},
			{"RED", "Red or Auburn"},
			{"SDY", "Sandy"},
			{"WHI", "White"},
			{"BAL", "Bald"},
			{"XXX", "Unknown"},
		},
	},
	"eyes": {
		ID:     "eyes",
		Name:   "Eye Color",
		Source: ncicSource,
		Codes: []Code{
			{"BLK", "Black"},
			{"BLU", "Blue"},
			{"BRO", "Brown"},
			{"GRN", "Green"},
			{"GRY", "Gray"},
			{"HAZ", "Hazel"},
			{"MAR", "Maroon"},
			{"PNK", "Pink"},
			{"MUL", "Multicolored"},
			{"XXX", "Unknown"},
		},
	},
	"sex": {
		ID:     "sex",
		Name:   "Sex",
		Source: ncicSource,
		Codes: []Code{
			{"M", "Male"},
			{"F", "Female"},
			{"U", "Unknown"},
		},
	},
	"race": {
		ID:     "race",
		Name:   "Race",
		Source: ncicSource,
		Codes: []Code{
			{"W", "White"},
			{"B", "Black"},
			{"I", "American Indian or Alaska Native"},
			{"A", "Asian"},
			{"P", "Native Hawaiian or Other Pacific Islander"},
			{"U", "Unknown"},
		},
	},
	"ethnicity": {
		ID:     "ethnicity",
		Name:   "Ethnicity",
		Source: "bitdispatcher",
		Codes: []Code{
			{"H", "Hispanic or Latino"},
			{"N", "Not Hispanic or Latino"},
			{"U", "Unknown"},
		},
	},
}

// cloneCodes returns a deep copy of a code slice so callers cannot mutate the
// built-in data through a returned list.
func cloneCodes(in []Code) []Code {
	if in == nil {
		return nil
	}
	out := make([]Code, len(in))
	copy(out, in)
	return out
}

// Lists returns every built-in controlled list in a stable order. The returned
// lists are copies; mutating them does not affect the built-in data.
func Lists() []List {
	out := make([]List, 0, len(listOrder))
	for _, id := range listOrder {
		l := builtins[id]
		l.Codes = cloneCodes(l.Codes)
		out = append(out, l)
	}
	return out
}

// Get returns the built-in list with the given ID. The second result is false
// if no such list exists. The returned list is a copy.
func Get(id string) (List, bool) {
	l, ok := builtins[id]
	if !ok {
		return List{}, false
	}
	l.Codes = cloneCodes(l.Codes)
	return l, true
}

// Label returns the plain-language label for a code, or "" when the code is not
// in the list. The lookup is case-insensitive on the code.
func (l List) Label(code string) string {
	want := strings.ToUpper(strings.TrimSpace(code))
	for _, c := range l.Codes {
		if strings.ToUpper(c.Code) == want {
			return c.Label
		}
	}
	return ""
}

// Has reports whether the code is present in the list. The lookup is
// case-insensitive on the code.
func (l List) Has(code string) bool {
	want := strings.ToUpper(strings.TrimSpace(code))
	for _, c := range l.Codes {
		if strings.ToUpper(c.Code) == want {
			return true
		}
	}
	return false
}

// Merge returns a copy of l with agency additions and overrides applied. An
// entry in add whose code (case-insensitively) matches an existing code
// replaces that entry's label in place; an entry with a new code is appended
// after the built-in codes. Codes listed in remove are dropped. The result is
// ordered built-in codes first (in their original order), then additions in the
// order given. The input list is not modified.
func Merge(l List, add []Code, remove []string) List {
	removeSet := make(map[string]bool, len(remove))
	for _, r := range remove {
		removeSet[strings.ToUpper(strings.TrimSpace(r))] = true
	}

	// Split additions into overrides (match existing codes) and new codes.
	overrides := make(map[string]string, len(add))
	var newCodes []Code
	seenAdd := make(map[string]bool, len(add))
	// Index of the existing (built-in) codes for override detection.
	existing := make(map[string]bool, len(l.Codes))
	for _, c := range l.Codes {
		existing[strings.ToUpper(c.Code)] = true
	}
	for _, c := range add {
		key := strings.ToUpper(strings.TrimSpace(c.Code))
		if key == "" {
			continue
		}
		if existing[key] {
			overrides[key] = c.Label
			continue
		}
		// Later additions with the same new code override earlier ones.
		if seenAdd[key] {
			for i := range newCodes {
				if strings.ToUpper(newCodes[i].Code) == key {
					newCodes[i].Label = c.Label
					break
				}
			}
			continue
		}
		seenAdd[key] = true
		newCodes = append(newCodes, Code{Code: c.Code, Label: c.Label})
	}

	out := List{ID: l.ID, Name: l.Name, Source: l.Source}
	for _, c := range l.Codes {
		key := strings.ToUpper(c.Code)
		if removeSet[key] {
			continue
		}
		label := c.Label
		if ov, ok := overrides[key]; ok {
			label = ov
		}
		out.Codes = append(out.Codes, Code{Code: c.Code, Label: label})
	}
	for _, c := range newCodes {
		if removeSet[strings.ToUpper(c.Code)] {
			continue
		}
		out.Codes = append(out.Codes, c)
	}
	return out
}

// sortCodesByCode is a small helper available for callers that want a code list
// sorted alphabetically; it is used by tests and kept unexported.
func sortCodesByCode(cs []Code) {
	sort.Slice(cs, func(i, j int) bool { return cs[i].Code < cs[j].Code })
}
