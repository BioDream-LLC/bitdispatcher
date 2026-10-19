package cad

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Records management: incident reports written from CAD incidents, with the data elements the FBI's
// National Incident-Based Reporting System (NIBRS) asks for, checked before a report is submitted. The
// state's own incident-based reporting variant and its submission format depend on the launch state
// (DESIGN.md 6); the checks here are the federal data elements every state's variant builds on.

var rptNumber = regexp.MustCompile(`^R([A-Z]*)(\d{2})-(\d{6})$`)

// Report statuses.
const (
	RptDraft     = "draft"
	RptSubmitted = "submitted" // to a supervisor
	RptApproved  = "approved"
	RptReturned  = "returned" // sent back to the author with a note
)

// RMSReport is an incident report.
type RMSReport struct {
	Number       string        `json:"number"` // R26-000001
	Incident     string        `json:"incident,omitempty"`
	Status       string        `json:"status"`
	Author       string        `json:"author"`
	Created      time.Time     `json:"created"`
	Updated      time.Time     `json:"updated"`
	Approver     string        `json:"approver,omitempty"`
	Decided      time.Time     `json:"decided,omitzero"`
	Note         string        `json:"note,omitempty"` // the supervisor's, when returned
	Occurred     time.Time     `json:"occurred,omitzero"`
	HourUnknown  bool          `json:"hourUnknown,omitempty"` // the date is known but not the hour
	Location     string        `json:"location,omitempty"`
	LocationType string        `json:"locationType,omitempty"` // NIBRS location code
	Narrative    string        `json:"narrative,omitempty"`
	Offenses     []RMSOffense  `json:"offenses"`
	People       []RMSPerson   `json:"people"`
	Property     []RMSProperty `json:"property"`
	Vehicles     []RMSVehicle  `json:"vehicles"`
	Cleared      string        `json:"cleared,omitempty"`   // "arrest", or an exceptional clearance code A-E (Codes.Clearance)
	ClearedOn    string        `json:"clearedOn,omitempty"` // YYYY-MM-DD, for an exceptional clearance
}

// RMSOffense is one offence.
type RMSOffense struct {
	Code      string   `json:"code"` // NIBRS offence code: 13A, 23F
	Attempted bool     `json:"attempted,omitempty"`
	Weapons   []string `json:"weapons,omitempty"`  // 13: up to 3; a trailing "A" marks an automatic firearm (11A)
	Bias      string   `json:"bias,omitempty"`     // 8A: 88 none
	Using     []string `json:"using,omitempty"`    // 8: A C D R, or N
	Activity  []string `json:"activity,omitempty"` // 12: up to 3
	Entry     string   `json:"entry,omitempty"`    // 11 burglary: F force, N no force
	Premises  int      `json:"premises,omitempty"` // 10 burglary of a hotel or storage facility: how many entered
	Security  []string `json:"security,omitempty"` // VA 54
	EntryPts  []string `json:"entryPts,omitempty"` // VA 55A
	ExitPts   []string `json:"exitPts,omitempty"`  // VA 55B
	LeftBy    string   `json:"leftBy,omitempty"`   // VA 56
	LPR       bool     `json:"lpr,omitempty"`      // VA 66: a licence plate reader was involved
}

// RMSPerson is someone in a report. Victims, suspects (offenders) and arrestees are the NIBRS segments;
// their sequence numbers are their order among people of the same role.
type RMSPerson struct {
	Role      string `json:"role"` // victim, suspect, arrestee, witness, reporting, other
	Name      string `json:"name"`
	DOB       string `json:"dob,omitempty"` // YYYY-MM-DD
	Age       string `json:"age,omitempty"` // when the date of birth is unknown: 34, or 30-40
	Sex       string `json:"sex,omitempty"` // M F U
	Race      string `json:"race,omitempty"`
	Ethnicity string `json:"ethnicity,omitempty"`
	Address   string `json:"address,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Injury    string `json:"injury,omitempty"`   // older reports: one injury in words
	Relation  string `json:"relation,omitempty"` // older reports: to the first suspect

	// Victims.
	VictimType    string        `json:"victimType,omitempty"` // 25: I unless said
	Resident      string        `json:"resident,omitempty"`   // 30
	Injuries      []string      `json:"injuries,omitempty"`   // 33
	Circumstances []string      `json:"circumstances,omitempty"`
	Relations     []RMSRelation `json:"relations,omitempty"`     // 34/35, to each suspect
	OfficerAct    string        `json:"officerAct,omitempty"`    // 25A
	OfficerAssign string        `json:"officerAssign,omitempty"` // 25B
	OfficerORI    string        `json:"officerOri,omitempty"`    // 25C, an officer from another agency
	AssaultStatus string        `json:"assaultStatus,omitempty"` // VA 57: C cleared, P pending

	// Arrestees.
	ArrestNo      string   `json:"arrestNo,omitempty"`      // 41; the report number when empty
	ArrestDate    string   `json:"arrestDate,omitempty"`    // 42 YYYY-MM-DD
	ArrestType    string   `json:"arrestType,omitempty"`    // 43
	ArrestOffense string   `json:"arrestOffense,omitempty"` // 45; the first offence when empty
	Armed         []string `json:"armed,omitempty"`         // 46: 01 unarmed
	Juvenile      string   `json:"juvenile,omitempty"`      // 52, under 18
	ArrestDrugs   []string `json:"arrestDrugs,omitempty"`   // VA 63 for 35A, with VA 62 the offence's activity
}

// RMSRelation is a victim's relationship to one suspect, by the suspect's number (1 is the first).
type RMSRelation struct {
	Suspect int    `json:"suspect"`
	Code    string `json:"code"`
}

// RMSProperty is property lost, damaged, recovered or seized.
type RMSProperty struct {
	Loss        string  `json:"loss"` // stolen, recovered, damaged, burned, seized, counterfeited, none, unknown (evidence: not reported)
	Type        string  `json:"type"` // NIBRS property description code
	Description string  `json:"description,omitempty"`
	Value       float64 `json:"value,omitempty"`
	Quantity    int     `json:"quantity,omitempty"`    // VA 60
	RecoveredOn string  `json:"recoveredOn,omitempty"` // 17 YYYY-MM-DD
	Serial      string  `json:"serial,omitempty"`
	Drug        string  `json:"drug,omitempty"`     // 20, for seized drugs
	DrugQty     float64 `json:"drugQty,omitempty"`  // 21
	DrugUnit    string  `json:"drugUnit,omitempty"` // 22
}

// RMSVehicle is a vehicle in a report.
type RMSVehicle struct {
	Role  string `json:"role,omitempty"` // stolen, suspect, victim, recovered, involved
	Plate string `json:"plate,omitempty"`
	State string `json:"state,omitempty"`
	Year  string `json:"year,omitempty"`
	Make  string `json:"make,omitempty"`
	Model string `json:"model,omitempty"`
	Color string `json:"color,omitempty"`
	VIN   string `json:"vin,omitempty"`
}

// NIBRSOffense describes an offence code.
type NIBRSOffense struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Against string `json:"against"` // person, property, society
	Group   string `json:"group"`   // A, or B (arrest only)
}

// NIBRSOffenses is the offence code list (FBI NIBRS User Manual).
var NIBRSOffenses = []NIBRSOffense{
	{"09A", "Murder and nonnegligent manslaughter", "person", "A"}, {"09B", "Negligent manslaughter", "person", "A"}, {"09C", "Justifiable homicide", "person", "A"},
	{"100", "Kidnapping/abduction", "person", "A"},
	{"11A", "Rape", "person", "A"}, {"11B", "Sodomy", "person", "A"}, {"11C", "Sexual assault with an object", "person", "A"}, {"11D", "Fondling", "person", "A"},
	{"120", "Robbery", "property", "A"},
	{"13A", "Aggravated assault", "person", "A"}, {"13B", "Simple assault", "person", "A"}, {"13C", "Intimidation", "person", "A"},
	{"200", "Arson", "property", "A"}, {"210", "Extortion/blackmail", "property", "A"}, {"220", "Burglary/breaking and entering", "property", "A"},
	{"23A", "Pocket-picking", "property", "A"}, {"23B", "Purse-snatching", "property", "A"}, {"23C", "Shoplifting", "property", "A"},
	{"23D", "Theft from building", "property", "A"}, {"23E", "Theft from coin-operated machine or device", "property", "A"},
	{"23F", "Theft from motor vehicle", "property", "A"}, {"23G", "Theft of motor vehicle parts or accessories", "property", "A"}, {"23H", "All other larceny", "property", "A"},
	{"240", "Motor vehicle theft", "property", "A"}, {"250", "Counterfeiting/forgery", "property", "A"},
	{"26A", "False pretenses/swindle/confidence game", "property", "A"}, {"26B", "Credit card/ATM fraud", "property", "A"}, {"26C", "Impersonation", "property", "A"},
	{"26D", "Welfare fraud", "property", "A"}, {"26E", "Wire fraud", "property", "A"}, {"26F", "Identity theft", "property", "A"}, {"26G", "Hacking/computer invasion", "property", "A"},
	{"270", "Embezzlement", "property", "A"}, {"280", "Stolen property offenses", "property", "A"}, {"290", "Destruction/damage/vandalism of property", "property", "A"},
	{"35A", "Drug/narcotic violations", "society", "A"}, {"35B", "Drug equipment violations", "society", "A"},
	{"36A", "Incest", "person", "A"}, {"36B", "Statutory rape", "person", "A"}, {"370", "Pornography/obscene material", "society", "A"},
	{"39A", "Betting/wagering", "society", "A"}, {"39B", "Operating/promoting/assisting gambling", "society", "A"}, {"39C", "Gambling equipment violations", "society", "A"}, {"39D", "Sports tampering", "society", "A"},
	{"40A", "Prostitution", "society", "A"}, {"40B", "Assisting or promoting prostitution", "society", "A"}, {"40C", "Purchasing prostitution", "society", "A"},
	{"510", "Bribery", "property", "A"}, {"520", "Weapon law violations", "society", "A"},
	{"64A", "Human trafficking, commercial sex acts", "person", "A"}, {"64B", "Human trafficking, involuntary servitude", "person", "A"},
	{"720", "Animal cruelty", "society", "A"},
	{"90B", "Curfew/loitering/vagrancy violations", "society", "B"}, {"90C", "Disorderly conduct", "society", "B"}, {"90D", "Driving under the influence", "society", "B"},
	{"90F", "Family offenses, nonviolent", "society", "B"}, {"90G", "Liquor law violations", "society", "B"}, {"90J", "Trespass of real property", "society", "B"}, {"90Z", "All other offenses", "society", "B"},
}

// NIBRSLocations is the location type list.
var NIBRSLocations = map[string]string{
	"01": "Air/bus/train terminal", "02": "Bank/savings and loan", "03": "Bar/nightclub", "04": "Church/synagogue/temple/mosque", "05": "Commercial/office building",
	"06": "Construction site", "07": "Convenience store", "08": "Department/discount store", "09": "Drug store/doctor's office/hospital", "10": "Field/woods",
	"11": "Government/public building", "12": "Grocery/supermarket", "13": "Highway/road/alley/street/sidewalk", "14": "Hotel/motel", "15": "Jail/prison/corrections facility",
	"16": "Lake/waterway/beach", "17": "Liquor store", "18": "Parking/drop lot/garage", "19": "Rental storage facility", "20": "Residence/home", "21": "Restaurant",
	"23": "Service/gas station", "24": "Specialty store", "25": "Other/unknown", "37": "Abandoned/condemned structure", "38": "Amusement park",
	"39": "Arena/stadium/fairgrounds", "40": "ATM separate from bank", "41": "Auto dealership", "42": "Camp/campground", "44": "Daycare facility",
	"45": "Dock/wharf/freight terminal", "46": "Farm facility", "47": "Gambling facility/casino/race track", "48": "Industrial site", "49": "Military installation",
	"50": "Park/playground", "51": "Rest area", "52": "School: college/university", "53": "School: elementary/secondary", "54": "Shelter: mission/homeless",
	"55": "Shopping mall", "56": "Tribal lands", "57": "Community center", "58": "Cyberspace",
}

func nibrsOffense(code string) (NIBRSOffense, bool) {
	for _, o := range NIBRSOffenses {
		if o.Code == code {
			return o, true
		}
	}
	return NIBRSOffense{}, false
}

// RMSCheck is one problem a report has before submission.
type RMSCheck struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// CheckReport lists what a report lacks for incident-based reporting: the FBI's NIBRS data elements
// and edits, and Virginia's additions, enough that the state's repository accepts it. A Group B offence
// (arrest only) needs the arrestee and nothing else.
func CheckReport(r RMSReport) []RMSCheck {
	var out []RMSCheck
	add := func(f, m string, a ...any) { out = append(out, RMSCheck{f, fmt.Sprintf(m, a...)}) }
	if strings.TrimSpace(r.Narrative) == "" {
		add("narrative", "write the narrative")
	}
	if r.Occurred.IsZero() {
		add("occurred", "give the date and time it happened")
	}
	if len(r.Offenses) == 0 {
		add("offenses", "add at least one offence")
	}
	groupA := false
	for _, o := range r.Offenses {
		if info, ok := nibrsOffense(o.Code); ok && info.Group == "A" {
			groupA = true
		}
	}
	if _, ok := NIBRSLocations[r.LocationType]; !ok && groupA {
		add("locationType", "choose the location type")
	}
	var victims, suspects, arrestees []RMSPerson
	for i, p := range r.People {
		who := fmt.Sprintf("person %d (%s)", i+1, firstNonEmpty(p.Name, p.Role))
		switch p.Role {
		case "victim":
			victims = append(victims, p)
		case "suspect":
			suspects = append(suspects, p)
		case "arrestee":
			arrestees = append(arrestees, p)
		}
		individual := p.Role == "arrestee" || (p.Role == "victim" && (p.VictimType == "" || p.VictimType == "I" || p.VictimType == "L"))
		if individual {
			if p.DOB == "" && p.Age == "" {
				add("people", "%s: give the date of birth or the age", who)
			}
			if p.Sex == "" {
				add("people", "%s: give the sex", who)
			}
			if p.Race == "" {
				add("people", "%s: give the race", who)
			}
		}
		if p.Role == "arrestee" && p.Sex == "U" {
			add("people", "%s: an arrestee's sex is M or F", who)
		}
		if p.DOB != "" {
			if _, err := time.Parse("2006-01-02", p.DOB); err != nil {
				add("people", "%s: the date of birth is YYYY-MM-DD", who)
			}
		}
		if p.Age != "" {
			if _, _, ok := parseAge(p.Age); !ok {
				add("people", "%s: the age is a number of years or a range such as 30-40", who)
			}
		}
		if p.VictimType != "" {
			if _, ok := Codes.VictimTypes[p.VictimType]; !ok {
				add("people", "%s: %s is not a victim type", who, p.VictimType)
			}
		}
		if p.Role == "victim" && p.VictimType == "L" {
			if p.OfficerAct == "" || p.OfficerAssign == "" {
				add("people", "%s: an officer victim needs the activity and the assignment", who)
			}
		}
		for _, rel := range p.Relations {
			if rel.Suspect < 1 || rel.Suspect > countRole(r.People, "suspect") {
				add("people", "%s: the relationship is to suspect %d, who is not in the report", who, rel.Suspect)
			} else if _, ok := Codes.Relations[rel.Code]; !ok {
				add("people", "%s: choose the relationship to suspect %d", who, rel.Suspect)
			}
		}
		if p.Role == "arrestee" {
			if _, err := time.Parse("2006-01-02", p.ArrestDate); err != nil {
				add("people", "%s: give the arrest date", who)
			}
			if _, ok := Codes.ArrestTypes[p.ArrestType]; !ok {
				add("people", "%s: give the type of arrest", who)
			}
			if len(p.Armed) == 0 {
				add("people", "%s: say what the arrestee was armed with (or unarmed)", who)
			}
			if p.ArrestOffense != "" {
				if _, ok := nibrsOffense(p.ArrestOffense); !ok {
					add("people", "%s: %s is not an offence code", who, p.ArrestOffense)
				}
			}
			if lo, _, ok := parseAge(personAge(p, r.Occurred)); ok && lo < 18 && lo > 0 && p.Juvenile == "" {
				add("people", "%s: under 18: say how the arrestee was handled", who)
			}
			if firstNonEmpty(p.ArrestOffense, firstCode(r)) == "35A" && len(p.ArrestDrugs) == 0 {
				add("people", "%s: a drug arrest needs the drug type", who)
			}
		}
	}
	hasLoss := func(kinds ...string) bool {
		for _, pr := range r.Property {
			if slices.Contains(kinds, pr.Loss) {
				return true
			}
		}
		return false
	}
	for _, o := range r.Offenses {
		info, ok := nibrsOffense(o.Code)
		if !ok {
			add("offenses", "%s is not a NIBRS offence code", o.Code)
			continue
		}
		if info.Group == "B" {
			if len(arrestees) == 0 {
				add("offenses", "%s (%s) is reported only with an arrest: add the arrestee", o.Code, info.Name)
			}
			continue
		}
		if len(o.Using) == 0 {
			add("offenses", "%s: say whether the offender was suspected of using alcohol, drugs or a computer (or not applicable)", o.Code)
		} else if slices.Contains(o.Using, "N") && len(o.Using) > 1 {
			add("offenses", "%s: 'not applicable' cannot go with another suspected-of-using", o.Code)
		}
		if o.Bias == "" {
			add("offenses", "%s: give the bias motivation (none, for most)", o.Code)
		}
		if info.Against == "person" && len(victims) == 0 {
			add("people", "%s (%s) is a crime against a person: add the victim", o.Code, info.Name)
		}
		switch {
		case o.Code == "200" && !o.Attempted && !hasLoss("burned"):
			add("property", "arson needs the burned property")
		case o.Code == "290" && !o.Attempted && !hasLoss("damaged"):
			add("property", "vandalism needs the damaged property")
		case (o.Code == "120" || o.Code == "220" || o.Code == "240" || strings.HasPrefix(o.Code, "23")) && !o.Attempted && !hasLoss("stolen", "recovered"):
			add("property", "%s (%s) needs the stolen property (or mark it attempted)", o.Code, info.Name)
		case (o.Code == "35A" || o.Code == "35B") && !o.Attempted && !hasLoss("seized", "none"):
			add("property", "%s needs the seized drugs or equipment", o.Code)
		}
		if slices.Contains(nibrsNeedWeapon, o.Code) && len(o.Weapons) == 0 {
			add("offenses", "%s (%s) needs the weapon or force used", o.Code, info.Name)
		}
		if o.Code == "13B" {
			for _, w := range o.Weapons {
				if !slices.Contains([]string{"40", "90", "95", "99"}, strings.TrimSuffix(w, "A")) {
					add("offenses", "a simple assault is with personal weapons, other, unknown or none: a weapon makes it aggravated (13A)")
					break
				}
			}
		}
		if slices.Contains(nibrsNeedActivity, o.Code) && len(o.Activity) == 0 {
			add("offenses", "%s (%s) needs the criminal activity: possessing, selling, using…", o.Code, info.Name)
		}
		if slices.Contains(nibrsNeedGang, o.Code) && len(o.Activity) == 0 {
			add("offenses", "%s (%s) needs the gang information (none/unknown, for most)", o.Code, info.Name)
		}
		if o.Code == "720" && len(o.Activity) == 0 {
			add("offenses", "animal cruelty needs the type: neglect, fighting, abuse or sexual abuse")
		}
		if o.Code == "220" {
			if o.Entry != "F" && o.Entry != "N" {
				add("offenses", "burglary needs the method of entry: force or no force")
			}
			if (r.LocationType == "14" || r.LocationType == "19") && o.Premises == 0 {
				add("offenses", "a burglary of a hotel or storage facility needs the number of premises entered")
			}
		}
		if o.Code == "240" && len(r.Vehicles) == 0 {
			add("vehicles", "motor vehicle theft needs the vehicle")
		}
		if slices.Contains(nibrsNeedInjury, o.Code) {
			for i, v := range victims {
				if (v.VictimType == "" || v.VictimType == "I" || v.VictimType == "L") && len(v.Injuries) == 0 && v.Injury == "" {
					add("people", "victim %d: %s needs the injury (or none)", i+1, o.Code)
				}
			}
		}
		if o.Code == "13A" || o.Code == "09A" || o.Code == "09B" || o.Code == "09C" {
			for i, v := range victims {
				if len(v.Circumstances) == 0 {
					add("people", "victim %d: %s needs the circumstances", i+1, o.Code)
				}
			}
		}
		if info.Against == "person" && len(suspects) > 0 {
			for i, v := range victims {
				if v.VictimType != "" && v.VictimType != "I" && v.VictimType != "L" {
					continue
				}
				for k := range suspects {
					if relationTo(v, k+1) == "" {
						add("people", "victim %d: give the relationship to suspect %d", i+1, k+1)
					}
				}
			}
		}
	}
	if groupA && len(victims) == 0 {
		for _, o := range r.Offenses {
			if info, ok := nibrsOffense(o.Code); ok && info.Group == "A" && info.Against == "property" {
				add("people", "add the victim: the person, business or other owner (%s)", o.Code)
				break
			}
		}
	}
	for i, pr := range r.Property {
		if pr.Loss == "evidence" {
			continue
		}
		if _, ok := Codes.Loss[pr.Loss]; !ok || pr.Type == "" {
			add("property", "property %d: give the loss and the type", i+1)
		}
		if pr.Value < 0 {
			add("property", "property %d: the value cannot be negative", i+1)
		}
		if pr.Loss == "recovered" && pr.RecoveredOn != "" {
			if _, err := time.Parse("2006-01-02", pr.RecoveredOn); err != nil {
				add("property", "property %d: the recovery date is YYYY-MM-DD", i+1)
			}
		}
		if pr.Type == "10" && pr.Loss == "seized" && (pr.Drug == "" || pr.DrugUnit == "") {
			add("property", "property %d: seized drugs need the drug type, quantity and measurement", i+1)
		}
		if pr.Type != "" && pr.Type != "10" && pr.Loss != "none" && pr.Quantity == 0 {
			add("property", "property %d: give the quantity (Virginia)", i+1)
		}
	}
	if len(r.Property) > 10*len(Codes.Loss) {
		add("property", "too many property lines")
	}
	switch r.Cleared {
	case "", "arrest":
		if r.Cleared == "arrest" && len(arrestees) == 0 {
			add("cleared", "cleared by arrest needs the arrestee")
		}
	case "A", "B", "C", "D", "E":
		if _, err := time.Parse("2006-01-02", r.ClearedOn); err != nil {
			add("cleared", "an exceptional clearance needs its date")
		}
		if len(arrestees) > 0 {
			add("cleared", "a report with an arrest is cleared by arrest, not exceptionally")
		}
	default:
		add("cleared", "%s is not a clearance", r.Cleared)
	}
	return out
}

func countRole(people []RMSPerson, role string) int {
	n := 0
	for _, p := range people {
		if p.Role == role {
			n++
		}
	}
	return n
}

func firstCode(r RMSReport) string {
	if len(r.Offenses) > 0 {
		return r.Offenses[0].Code
	}
	return ""
}

// relationTo is a victim's relationship to suspect k (1-based); older reports kept one, to the first.
func relationTo(v RMSPerson, k int) string {
	for _, rel := range v.Relations {
		if rel.Suspect == k {
			return rel.Code
		}
	}
	if k == 1 {
		if _, ok := Codes.Relations[strings.ToUpper(v.Relation)]; ok {
			return strings.ToUpper(v.Relation)
		}
	}
	return ""
}

// RelationTo is RMSPerson's relationship to suspect k, for the writer of the state file.
func RelationTo(v RMSPerson, k int) string { return relationTo(v, k) }

// parseAge reads "34", or "30-40", in years.
func parseAge(s string) (lo, hi int, ok bool) {
	s = strings.TrimSpace(s)
	a, b, isRange := strings.Cut(s, "-")
	x, err := strconv.Atoi(strings.TrimSpace(a))
	if err != nil || x < 0 || x > 120 {
		return 0, 0, false
	}
	if !isRange {
		return x, x, true
	}
	y, err := strconv.Atoi(strings.TrimSpace(b))
	if err != nil || x < 1 || y > 98 || x >= y {
		return 0, 0, false
	}
	return x, y, true
}

// personAge is the age at the time of the offence: from the date of birth, else as given.
func personAge(p RMSPerson, at time.Time) string {
	if d, err := time.Parse("2006-01-02", p.DOB); err == nil && !at.IsZero() {
		y := at.Year() - d.Year()
		if at.YearDay() < d.YearDay() {
			y--
		}
		if y < 0 {
			y = 0
		}
		return fmt.Sprint(y)
	}
	return p.Age
}

// PersonAge is personAge for the writer of the state file.
func PersonAge(p RMSPerson, at time.Time) string { return personAge(p, at) }

// PersonAgeDays is how many days old someone with a date of birth was, for infants (NN, NB, BB); -1 unknown.
func PersonAgeDays(p RMSPerson, at time.Time) int {
	d, err := time.Parse("2006-01-02", p.DOB)
	if err != nil || at.IsZero() {
		return -1
	}
	return int(at.Sub(d).Hours() / 24)
}

// NIBRSOffenseInfo looks up an offence code.
func NIBRSOffenseInfo(code string) (NIBRSOffense, bool) { return nibrsOffense(code) }

type rptData struct {
	Report RMSReport `json:"report"`
}

type rptStatus struct {
	Number string `json:"number"`
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
}

// SaveReport creates or changes a report. A new report takes the incident's location and time.
func (e *Engine) SaveReport(a Actor, r RMSReport) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	if r.Number == "" {
		key := fmt.Sprintf("%s|%02d", e.sitePrefix, now.Year()%100)
		e.st.rptCount[key]++
		r.Number = fmt.Sprintf("R%s%02d-%06d", e.sitePrefix, now.Year()%100, e.st.rptCount[key])
		r.Status, r.Author, r.Created = RptDraft, a.User, now
		if in, ok := e.st.Incidents[r.Incident]; ok {
			if r.Location == "" && in.Location != nil {
				r.Location = in.Location.Address
			}
			if r.Occurred.IsZero() {
				r.Occurred = in.Created
			}
		} else if r.Incident != "" {
			return "", problem("incident", "no incident %s", r.Incident)
		}
	} else {
		old, ok := e.st.Reports[r.Number]
		if !ok {
			return "", problem("number", "no report %s", r.Number)
		}
		if old.Status == RptApproved || old.Status == RptSubmitted {
			return "", problem("status", "%s is %s: it cannot be changed now", r.Number, old.Status)
		}
		r.Status, r.Author, r.Created, r.Incident = old.Status, old.Author, old.Created, old.Incident
		if r.Status == RptReturned {
			r.Status = RptDraft
		}
	}
	if len(r.Narrative) > 100000 {
		return "", problem("narrative", "the narrative is too long")
	}
	r.Updated = now
	for _, x := range []*[]RMSOffense{&r.Offenses} {
		if *x == nil {
			*x = []RMSOffense{}
		}
	}
	if r.People == nil {
		r.People = []RMSPerson{}
	}
	if r.Property == nil {
		r.Property = []RMSProperty{}
	}
	if r.Vehicles == nil {
		r.Vehicles = []RMSVehicle{}
	}
	_, err := e.write(a, EvReportSaved, refs{Incident: r.Incident}, rptData{r})
	return r.Number, err
}

// SetReportStatus submits a report (its author), or approves or returns it (a supervisor).
func (e *Engine) SetReportStatus(a Actor, number, status, note string, supervisor bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	r, ok := e.st.Reports[number]
	if !ok {
		return problem("number", "no report %s", number)
	}
	switch status {
	case RptSubmitted:
		if r.Status != RptDraft && r.Status != RptReturned {
			return problem("status", "%s is %s", number, r.Status)
		}
		if p := CheckReport(*r); len(p) > 0 {
			return problem(p[0].Field, "%s (%d to fix)", p[0].Message, len(p))
		}
	case RptApproved, RptReturned:
		if !supervisor {
			return problem("status", "a supervisor approves or returns reports")
		}
		if r.Status != RptSubmitted {
			return problem("status", "%s has not been submitted", number)
		}
		if a.User == r.Author {
			return problem("status", "a report is approved by someone other than its author")
		}
		if status == RptReturned && strings.TrimSpace(note) == "" {
			return problem("note", "say what to fix")
		}
	default:
		return problem("status", "unknown status %s", status)
	}
	_, err := e.write(a, EvReportStatus, refs{Incident: r.Incident}, rptStatus{Number: number, Status: status, Note: strings.TrimSpace(note)})
	return err
}

func (s *State) applyReport(at time.Time, actor string, v rptData) {
	r := v.Report
	s.Reports[r.Number] = &r
	if m := rptNumber.FindStringSubmatch(r.Number); m != nil {
		key := m[1] + "|" + m[2]
		var n int
		fmt.Sscan(strings.TrimLeft(m[3], "0"), &n)
		if n > s.rptCount[key] {
			s.rptCount[key] = n
		}
	}
	if in, ok := s.Incidents[r.Incident]; ok && !slices.Contains(in.Reports, r.Number) {
		in.Reports = append(in.Reports, r.Number)
	}
}

func (s *State) applyReportStatus(at time.Time, actor string, v rptStatus) {
	r, ok := s.Reports[v.Number]
	if !ok {
		return
	}
	r.Status, r.Updated = v.Status, at
	switch v.Status {
	case RptApproved, RptReturned:
		r.Approver, r.Decided, r.Note = actor, at, v.Note
	case RptSubmitted:
		r.Note = ""
	}
}
