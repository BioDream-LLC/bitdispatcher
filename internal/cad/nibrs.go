package cad

// The NIBRS data values a report is written in, from the FBI's NIBRS Technical Specification as
// Virginia publishes it (2025.2 Virginia Incident-Based Reporting System Technical Specification,
// Virginia State Police). Virginia's additions are marked VA.

// NIBRSCodes are the code lists the report screen offers.
type NIBRSCodes struct {
	Using         map[string]string `json:"using"`         // 8 offender suspected of using
	Bias          map[string]string `json:"bias"`          // 8A
	Activity      map[string]string `json:"activity"`      // 12 criminal activity / gang information
	Weapons       map[string]string `json:"weapons"`       // 13
	Loss          map[string]string `json:"loss"`          // 14 our words -> the code
	Property      map[string]string `json:"property"`      // 15
	Drugs         map[string]string `json:"drugs"`         // 20 (VA list)
	DrugUnits     map[string]string `json:"drugUnits"`     // 22
	VictimTypes   map[string]string `json:"victimTypes"`   // 25
	Officer       map[string]string `json:"officer"`       // 25A
	Assignment    map[string]string `json:"assignment"`    // 25B
	Races         map[string]string `json:"races"`         // 28
	Ethnicities   map[string]string `json:"ethnicities"`   // 29
	Residents     map[string]string `json:"residents"`     // 30
	Circumstances map[string]string `json:"circumstances"` // 31
	Injuries      map[string]string `json:"injuries"`      // 33
	Relations     map[string]string `json:"relations"`     // 35
	ArrestTypes   map[string]string `json:"arrestTypes"`   // 43
	Armed         map[string]string `json:"armed"`         // 46
	Juvenile      map[string]string `json:"juvenile"`      // 52
	Clearance     map[string]string `json:"clearance"`     // 4
	Security      map[string]string `json:"security"`      // VA 54
	Points        map[string]string `json:"points"`        // VA 55A/55B
	LeftBy        map[string]string `json:"leftBy"`        // VA 56
}

// Codes is every list.
var Codes = NIBRSCodes{
	Using: map[string]string{"A": "Alcohol", "C": "Computer equipment (handheld devices)", "D": "Drugs/narcotics", "R": "Drone", "N": "Not applicable"},
	Bias: map[string]string{
		"88": "None (no bias)", "99": "Unknown", "11": "Anti-White", "12": "Anti-Black or African American", "13": "Anti-American Indian or Alaska Native", "14": "Anti-Asian",
		"15": "Anti-multiple races", "16": "Anti-Native Hawaiian or Other Pacific Islander", "31": "Anti-Arab", "32": "Anti-Hispanic or Latino", "33": "Anti-other race/ethnicity/ancestry",
		"21": "Anti-Jewish", "22": "Anti-Catholic", "23": "Anti-Protestant", "24": "Anti-Islamic (Muslim)", "25": "Anti-other religion", "26": "Anti-multiple religions", "27": "Anti-atheism/agnosticism",
		"28": "Anti-Church of Jesus Christ", "29": "Anti-Jehovah's Witness", "81": "Anti-Eastern Orthodox", "82": "Anti-other Christian", "83": "Anti-Buddhist", "84": "Anti-Hindu", "85": "Anti-Sikh",
		"41": "Anti-gay (male)", "42": "Anti-lesbian (female)", "43": "Anti-LGBT (mixed group)", "44": "Anti-heterosexual", "45": "Anti-bisexual", "51": "Anti-physical disability",
		"52": "Anti-mental disability", "61": "Anti-male", "62": "Anti-female", "71": "Anti-transgender", "72": "Anti-gender non-conforming",
	},
	Activity: map[string]string{
		"B": "Buying/receiving", "C": "Cultivating/manufacturing/publishing", "D": "Distributing/selling", "E": "Exploiting children", "O": "Operating/promoting/assisting",
		"P": "Possessing/concealing", "T": "Transporting/transmitting/importing", "U": "Using/consuming", "A": "Simple/gross neglect (animal)", "F": "Organized abuse (animal fighting)",
		"I": "Intentional abuse and torture (animal)", "S": "Animal sexual abuse", "J": "Juvenile gang", "G": "Other gang", "N": "None/unknown (gang)",
	},
	Weapons: map[string]string{
		"11": "Firearm", "12": "Handgun", "13": "Rifle", "14": "Shotgun", "15": "Other firearm", "20": "Knife/cutting instrument", "30": "Blunt object", "35": "Motor vehicle/vessel",
		"40": "Personal weapons (hands, feet)", "50": "Poison", "60": "Explosives", "65": "Fire/incendiary device", "70": "Drugs/narcotics/sleeping pills", "85": "Asphyxiation",
		"90": "Other", "95": "Unknown", "99": "None",
	},
	Loss: map[string]string{"none": "1", "burned": "2", "counterfeited": "3", "damaged": "4", "recovered": "5", "seized": "6", "stolen": "7", "unknown": "8"},
	Property: map[string]string{
		"01": "Aircraft", "02": "Alcohol", "03": "Automobiles", "04": "Bicycles", "05": "Buses", "06": "Clothes/furs", "07": "Computer hardware/software", "08": "Consumable goods",
		"09": "Credit/debit cards", "10": "Drugs/narcotics", "11": "Drug/narcotic equipment", "12": "Farm equipment", "13": "Firearms", "14": "Gambling equipment",
		"15": "Heavy construction/industrial equipment", "16": "Household goods", "17": "Jewelry/precious metals/gems", "18": "Livestock", "19": "Merchandise", "20": "Money",
		"21": "Negotiable instruments", "22": "Nonnegotiable instruments", "23": "Office-type equipment", "24": "Other motor vehicles", "25": "Purses/handbags/wallets",
		"26": "Radios/TVs/VCRs/DVD players", "27": "Recordings (audio/visual)", "28": "Recreational vehicles", "29": "Structures: single occupancy dwellings", "30": "Structures: other dwellings",
		"31": "Structures: other commercial/business", "32": "Structures: industrial/manufacturing", "33": "Structures: public/community", "34": "Structures: storage", "35": "Structures: other",
		"36": "Tools", "37": "Trucks", "38": "Vehicle parts/accessories", "39": "Watercraft", "41": "Aircraft parts/accessories", "42": "Artistic supplies/accessories", "43": "Building materials",
		"44": "Camping/hunting/fishing equipment", "45": "Chemicals", "46": "Collections/collectibles", "47": "Crops", "48": "Documents (personal or business)", "49": "Explosives",
		"59": "Firearm accessories", "64": "Fuel", "65": "Identity documents", "66": "Identity (intangible)", "67": "Law enforcement equipment", "68": "Lawn/yard/garden equipment",
		"69": "Logging equipment", "70": "Medical/medical lab equipment", "71": "Metals, non-precious", "72": "Musical instruments", "73": "Pets", "74": "Photographic/optical equipment",
		"75": "Portable electronic communications", "76": "Recreational/sports equipment", "77": "Other", "78": "Trailers", "79": "Watercraft equipment/parts", "80": "Weapons (other)",
		"88": "Pending inventory",
	},
	Drugs: map[string]string{
		"A": "Crack cocaine", "B": "Cocaine (except crack)", "C": "Hashish and synthetic cannabinoids", "D": "Heroin", "E": "Cannabis/marijuana", "F": "Morphine", "G": "Opium",
		"H": "Other narcotics", "W": "Oxycodone and other licit opioids", "I": "LSD", "J": "PCP", "K": "Miscellaneous hallucinogens", "L": "Methamphetamine", "M": "Other stimulants",
		"Z": "Khat and other cathinones", "N": "Barbiturates", "O": "Miscellaneous depressants", "P": "Other drug types", "Q": "Amphetamine", "R": "Benzodiazepine",
		"S": "Fentanyl (and its generics)", "T": "GHB/GBL/BD", "U": "Unknown drug type", "V": "MDA/MDMA/MDE (ecstasy)", "Y": "Steroids",
	},
	DrugUnits:   map[string]string{"GM": "Gram", "KG": "Kilogram", "OZ": "Ounce", "LB": "Pound", "MG": "Milligram", "ML": "Milliliter", "LT": "Liter", "FO": "Fluid ounce", "GL": "Gallon", "DU": "Dosage units/items", "NP": "Number of plants", "PC": "Pills/capsules", "XX": "Not reported"},
	VictimTypes: map[string]string{"I": "Individual", "L": "Law enforcement officer", "B": "Business", "F": "Financial institution", "G": "Government", "R": "Religious organization", "S": "Society/public", "O": "Other", "U": "Unknown"},
	Officer: map[string]string{
		"01": "Responding to disturbance call", "02": "Burglaries in progress", "03": "Robberies in progress", "04": "Attempting other arrests", "05": "Civil disorder",
		"06": "Handling, transporting, custody of prisoners", "07": "Investigating suspicious persons", "08": "Ambush, no warning", "09": "Handling persons with mental illness",
		"10": "Traffic pursuits and stops", "11": "All other",
	},
	Assignment:  map[string]string{"F": "Two-officer vehicle", "G": "One-officer vehicle (alone)", "H": "One-officer vehicle (assisted)", "I": "Detective or special assignment (alone)", "J": "Detective or special assignment (assisted)", "K": "Other (alone)", "L": "Other (assisted)"},
	Races:       map[string]string{"W": "White", "B": "Black or African American", "I": "American Indian or Alaska Native", "A": "Asian", "P": "Native Hawaiian or Other Pacific Islander", "U": "Unknown"},
	Ethnicities: map[string]string{"H": "Hispanic or Latino", "N": "Not Hispanic or Latino", "U": "Unknown"},
	Residents:   map[string]string{"R": "Resident", "N": "Nonresident", "U": "Unknown"},
	Circumstances: map[string]string{
		"01": "Argument", "02": "Assault on law enforcement officer", "03": "Drug dealing", "04": "Gangland", "05": "Juvenile gang", "06": "Domestic violence", "07": "Mercy killing",
		"08": "Other felony involved", "09": "Other circumstances", "10": "Unknown circumstances", "20": "Criminal killed by private citizen", "21": "Criminal killed by police officer",
		"30": "Child playing with weapon", "31": "Gun-cleaning accident", "32": "Hunting accident", "33": "Other negligent weapon handling", "34": "Other negligent killing",
	},
	Injuries: map[string]string{"N": "None", "M": "Apparent minor injury", "B": "Apparent broken bones", "G": "Gunshot wound", "I": "Possible internal injury", "L": "Severe laceration", "O": "Other major injury", "T": "Loss of teeth", "U": "Unconsciousness"},
	Relations: map[string]string{
		"SE": "Spouse", "CS": "Common-law spouse", "PA": "Parent", "SB": "Sibling", "CH": "Child", "GP": "Grandparent", "GC": "Grandchild", "IL": "In-law", "SP": "Step-parent",
		"SC": "Step-child", "SS": "Step-sibling", "OF": "Other family member", "FP": "Foster parent", "FC": "Foster child", "AQ": "Acquaintance", "FR": "Friend", "NE": "Neighbor",
		"BE": "Babysittee", "BG": "Boyfriend/girlfriend", "CF": "Child of boyfriend/girlfriend", "XR": "Ex-relationship", "XS": "Ex-spouse", "CO": "Cohabitant (non-intimate)",
		"EE": "Employee", "ER": "Employer", "OK": "Otherwise known", "ST": "Stranger", "VO": "Victim was offender", "RU": "Relationship unknown",
	},
	ArrestTypes: map[string]string{"O": "On-view arrest", "S": "Summoned/cited", "T": "Taken into custody (warrant)"},
	Armed: map[string]string{
		"01": "Unarmed", "11": "Firearm", "12": "Handgun", "13": "Rifle", "14": "Shotgun", "15": "Other firearm", "16": "Lethal cutting instrument", "17": "Club/blackjack/brass knuckles",
	},
	Juvenile:  map[string]string{"H": "Handled within the department", "R": "Referred to other authorities"},
	Clearance: map[string]string{"arrest": "Cleared by arrest", "A": "Death of offender", "B": "Prosecution declined", "C": "In custody of another jurisdiction", "D": "Victim refused to cooperate", "E": "Juvenile, no custody"},
	Security: map[string]string{
		"A": "Alarm (audio)", "B": "Alarm (silent)", "C": "Bars/grate", "D": "Camera", "E": "Dog", "F": "Dead bolt", "G": "Locked", "H": "Unlocked", "I": "Exterior lights",
		"J": "Interior lights", "K": "Fence", "L": "Guard", "M": "Neighborhood watch", "N": "Other", "O": "None",
	},
	Points: map[string]string{
		"01": "Front", "02": "Rear", "03": "Side", "04": "Attic", "05": "Vent/A.C.", "06": "Window", "07": "Door", "08": "Patio/sliding door", "09": "Balcony/fire escape",
		"10": "Attached garage", "11": "Wall", "12": "Vehicle", "13": "Floor", "14": "Roof/skylight", "15": "Hidden within", "16": "Other", "17": "Unknown",
	},
	LeftBy: map[string]string{"1": "Auto", "2": "Truck", "3": "Van", "4": "Motorcycle", "5": "Bicycle", "6": "Foot", "7": "Moped", "8": "Other", "9": "Unknown"},
}

// Offence groupings the checks use.
var (
	// The offences that need the weapon or force (data element 13).
	nibrsNeedWeapon = []string{"09A", "09B", "09C", "100", "11A", "11B", "11C", "11D", "120", "13A", "13B", "210", "520", "64A", "64B"}
	// The offences that need criminal activity (12): the trade in something.
	nibrsNeedActivity = []string{"250", "280", "35A", "35B", "39C", "370", "520"}
	// The offences that need gang information (12).
	nibrsNeedGang = []string{"09A", "09B", "100", "11A", "11B", "11C", "11D", "120", "13A", "13B", "13C"}
	// The offences that need the victim's injury (33).
	nibrsNeedInjury = []string{"100", "11A", "11B", "11C", "11D", "120", "13A", "13B", "210", "64A", "64B"}
	// The offences with a Property Segment (crimes against property, kidnapping, drugs, gambling).
	nibrsPropertyOffense = []string{"100", "120", "200", "210", "220", "23A", "23B", "23C", "23D", "23E", "23F", "23G", "23H", "240", "250", "26A", "26B", "26C", "26D", "26E", "26F", "26G",
		"270", "280", "290", "35A", "35B", "39A", "39B", "39C", "39D", "510"}
)

// NIBRSNeeds tells the report screen which offences need which details, so it shows only those.
var NIBRSNeeds = map[string][]string{"weapon": nibrsNeedWeapon, "activity": nibrsNeedActivity, "gang": nibrsNeedGang, "injury": nibrsNeedInjury,
	"circumstances": {"09A", "09B", "09C", "13A"}, "relation": {"09A", "09B", "09C", "100", "11A", "11B", "11C", "11D", "120", "13A", "13B", "13C", "36A", "36B", "64A", "64B"}}

// NIBRSInjuryOffenses are the offences reported with the victim's injury.
var NIBRSInjuryOffenses = nibrsNeedInjury

// NIBRSNeedsProperty reports whether an offence is reported with a Property Segment.
func NIBRSNeedsProperty(code string) bool {
	for _, c := range nibrsPropertyOffense {
		if c == code {
			return true
		}
	}
	return false
}
