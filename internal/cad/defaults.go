package cad

// DefaultSettings are what a new system starts with. Every value can be changed in Admin › Settings.
func DefaultSettings() Settings {
	return Settings{
		Agency:         "",
		IncidentPrefix: "",
		Timers: map[string]int{
			Dispatched:    2, // not acknowledged
			Enroute:       12,
			OnScene:       20, // a status check on a police unit; agencies set their own
			Transporting:  30,
			AtDestination: 30,
			MarkedOut:     30, // a mark-out of a type without its own time
		},
		MarkOuts:        DefaultMarkOuts(),
		AVLStaleSeconds: 120,
		LocStaleSeconds: 300,
		RoadFactor:      1.4,
		SpeedKmh:        55,
		MapCenter:       [2]float64{37.2710, -79.9414},
		MapZoom:         12,
		Shifts:          []string{"07:00", "19:00"},
		Qualifications:  DefaultQualifications(),
		Dispositions: []string{
			"Handled", "Transported", "Report taken", "Arrest", "Citation", "Warning", "Unfounded",
			"Gone on arrival", "Cancelled en route", "Referred", "False alarm", "Duplicate",
		},
		UnitTypes: []UnitType{
			{Code: "engine", Name: "Engine", Discipline: "fire", Capabilities: []string{"engine", "fire", "bls"}, MinStaff: 3},
			{Code: "ladder", Name: "Ladder / truck", Discipline: "fire", Capabilities: []string{"ladder", "fire"}, MinStaff: 3},
			{Code: "rescue", Name: "Rescue", Discipline: "fire", Capabilities: []string{"rescue", "extrication", "fire"}},
			{Code: "battalion", Name: "Battalion chief", Discipline: "fire", Capabilities: []string{"command"}},
			{Code: "tanker", Name: "Tanker / tender", Discipline: "fire", Capabilities: []string{"water"}},
			{Code: "brush", Name: "Brush truck", Discipline: "fire", Capabilities: []string{"brush", "fire"}},
			{Code: "medic", Name: "Medic (ALS)", Discipline: "ems", Capabilities: []string{"als", "bls", "transport"}, MinStaff: 2},
			{Code: "ambulance", Name: "Ambulance (BLS)", Discipline: "ems", Capabilities: []string{"bls", "transport"}, MinStaff: 2},
			{Code: "ems-supervisor", Name: "EMS supervisor", Discipline: "ems", Capabilities: []string{"als", "ems-command"}},
			{Code: "patrol", Name: "Patrol", Discipline: "police", Capabilities: []string{"police"}},
			{Code: "police-supervisor", Name: "Police supervisor", Discipline: "police", Capabilities: []string{"police", "police-command"}},
			{Code: "k9", Name: "K-9", Discipline: "police", Capabilities: []string{"police", "k9"}},
		},
	}
}

// DefaultCallTypes are a starting set of call types with run cards. Agencies replace them with their own.
func DefaultCallTypes() []CallType {
	r := func(c string, n int) Requirement { return Requirement{Capability: c, Count: n} }
	return []CallType{
		{Code: "SF", Name: "Structure fire", Priority: 1, Discipline: "fire", Drone: true, PAR: true, RunCard: []Requirement{r("engine", 3), r("ladder", 1), r("rescue", 1), r("command", 1), r("als", 1)},
			Alarms: []AlarmLevel{
				{Name: "2nd alarm", RunCard: []Requirement{r("engine", 2), r("ladder", 1), r("command", 1), r("als", 1)}},
				{Name: "3rd alarm", RunCard: []Requirement{r("engine", 2), r("ladder", 1), r("water", 1)}},
			}},
		{Code: "VF", Name: "Vehicle fire", Priority: 2, Discipline: "fire", RunCard: []Requirement{r("engine", 1)}},
		{Code: "BF", Name: "Brush fire", Priority: 2, Discipline: "fire", RunCard: []Requirement{r("brush", 1), r("engine", 1)}},
		{Code: "FA", Name: "Fire alarm", Priority: 2, Discipline: "fire", RunCard: []Requirement{r("engine", 1), r("ladder", 1)}},
		{Code: "GAS", Name: "Gas leak / odour", Priority: 2, Discipline: "fire", RunCard: []Requirement{r("engine", 1)}},
		{Code: "HAZ", Name: "Hazardous materials", Priority: 1, Discipline: "fire", Drone: true, PAR: true, RunCard: []Requirement{r("engine", 2), r("command", 1), r("als", 1)}},
		{Code: "MVA", Name: "Vehicle crash, injuries", Priority: 1, Discipline: "fire", Drone: true, RunCard: []Requirement{r("engine", 1), r("rescue", 1), r("als", 1), r("police", 1)}},
		{Code: "MVAPIN", Name: "Vehicle crash, entrapment", Priority: 1, Discipline: "fire", Drone: true, RunCard: []Requirement{r("engine", 1), r("extrication", 1), r("als", 2), r("command", 1), r("police", 1)}},
		{Code: "CARD", Name: "Cardiac arrest", Priority: 1, Discipline: "ems", Critical: true, RunCard: []Requirement{r("als", 1), r("bls", 2)}},
		{Code: "BRTH", Name: "Breathing problem", Priority: 1, Discipline: "ems", RunCard: []Requirement{r("als", 1)}},
		{Code: "CHEST", Name: "Chest pain", Priority: 1, Discipline: "ems", RunCard: []Requirement{r("als", 1)}},
		{Code: "STROKE", Name: "Stroke", Priority: 1, Discipline: "ems", RunCard: []Requirement{r("als", 1)}},
		{Code: "FALL", Name: "Fall", Priority: 3, Discipline: "ems", RunCard: []Requirement{r("transport", 1)}},
		{Code: "SICK", Name: "Sick person", Priority: 3, Discipline: "ems", RunCard: []Requirement{r("transport", 1)}},
		{Code: "OD", Name: "Overdose / poisoning", Priority: 1, Discipline: "ems", RunCard: []Requirement{r("als", 1), r("police", 1)}},
		{Code: "SHOOT", Name: "Shooting", Priority: 1, Discipline: "police", Critical: true, Drone: true, RunCard: []Requirement{r("police", 3), r("police-command", 1), r("als", 1)}},
		{Code: "STAB", Name: "Stabbing", Priority: 1, Discipline: "police", Critical: true, RunCard: []Requirement{r("police", 2), r("als", 1)}},
		{Code: "DOM", Name: "Domestic disturbance", Priority: 1, Discipline: "police", RunCard: []Requirement{r("police", 2)}},
		{Code: "ASLT", Name: "Assault", Priority: 2, Discipline: "police", RunCard: []Requirement{r("police", 2)}},
		{Code: "BURG", Name: "Burglary in progress", Priority: 1, Discipline: "police", Drone: true, RunCard: []Requirement{r("police", 2)}},
		{Code: "ROB", Name: "Robbery", Priority: 1, Discipline: "police", Drone: true, RunCard: []Requirement{r("police", 3)}},
		{Code: "THEFT", Name: "Theft report", Priority: 4, Discipline: "police", RunCard: []Requirement{r("police", 1)}},
		{Code: "SUSP", Name: "Suspicious person / vehicle", Priority: 3, Discipline: "police", RunCard: []Requirement{r("police", 1)}},
		{Code: "TS", Name: "Traffic stop", Priority: 3, Discipline: "police", RunCard: []Requirement{r("police", 1)}},
		{Code: "WELF", Name: "Welfare check", Priority: 3, Discipline: "police", RunCard: []Requirement{r("police", 1)}},
		{Code: "MISS", Name: "Missing / endangered person", Priority: 1, Discipline: "police", Drone: true, RunCard: []Requirement{r("police", 2), r("police-command", 1)}},
		{Code: "ALARM", Name: "Burglar alarm", Priority: 3, Discipline: "police", RunCard: []Requirement{r("police", 1)}},
		{Code: "PURSUIT", Name: "Pursuit", Priority: 1, Discipline: "police", Drone: true, RunCard: []Requirement{r("police", 2), r("police-command", 1)}},
		{Code: "EVENT", Name: "Special event / standby", Priority: 5, Discipline: "other", RunCard: nil},
		{Code: "TRANSFER", Name: "Scheduled transport", Priority: 4, Discipline: "ems", RunCard: []Requirement{r("transport", 1)}},
		{Code: "TOW", Name: "Tow request / disabled vehicle", Priority: 4, Discipline: "police", RunCard: []Requirement{r("police", 1)}},
		{Code: "PAPER", Name: "Paper service / warrant service", Priority: 4, Discipline: "police", RunCard: []Requirement{r("police", 1)}},
		{Code: "OTHER", Name: "Other / unknown", Priority: 3, Discipline: "other", RunCard: nil},
	}
}

// DefaultMarkOuts are the usual mark-outs; every agency edits its own in Admin › Settings.
func DefaultMarkOuts() []MarkOutType {
	return []MarkOutType{
		{Code: "MEAL", Name: "Meal break", Minutes: 30, Interruptible: true},
		{Code: "FUEL", Name: "Fuel", Minutes: 15, Interruptible: true},
		{Code: "RPT", Name: "Report writing", Minutes: 45, Interruptible: true},
		{Code: "FU", Name: "Follow-up", Minutes: 30, Discipline: "police", Interruptible: true},
		{Code: "TS", Name: "Traffic stop", Minutes: 10, Discipline: "police"},
		{Code: "SEC", Name: "Security / business check", Minutes: 15, Discipline: "police", Interruptible: true},
		{Code: "PTL", Name: "Directed patrol", Minutes: 30, Discipline: "police", Interruptible: true},
		{Code: "COURT", Name: "Court", Minutes: 120},
		{Code: "JAIL", Name: "Prisoner transport / jail", Minutes: 45, Discipline: "police"},
		{Code: "ADMIN", Name: "Administrative", Minutes: 30, Interruptible: true},
		{Code: "TRN", Name: "Training", Minutes: 120, Interruptible: true},
		{Code: "MAINT", Name: "Vehicle maintenance", Minutes: 60},
		{Code: "INSP", Name: "Inspection / pre-plan", Minutes: 60, Discipline: "fire", Interruptible: true},
	}
}
