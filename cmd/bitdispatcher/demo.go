package main

import (
	"encoding/json"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/advisory"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// seedDemo adds demonstration stations and units to a system that has none. They are placed around
// Roanoke, Virginia, the development map, at the positions of real stations so drive times look right, but
// the units and their names are fictional and the agency name says "demo".
func seedDemo(eng *cad.Engine) error {
	empty := true
	var settings cad.Settings
	eng.Read(func(s *cad.State) { empty, settings = len(s.Units) == 0 && len(s.Stations) == 0, s.Settings })
	if !empty {
		return nil
	}
	a := cad.SystemActor
	settings.Agency = "Roanoke Valley Demo 911 (demo data)"
	settings.MapCenter = [2]float64{37.2710, -79.9414}
	settings.MapZoom = 13
	settings.ORI, settings.NCICMode = "VA0000000", "sim" // the training simulator: TEST plates and names return a record
	if err := eng.SaveSettings(a, settings); err != nil {
		return err
	}
	// The demo shows the advisory panel live; a real system starts in shadow mode.
	cb := advisory.Default()
	cb.Rules = append(cb.Rules, demoScript)
	raw, _ := json.Marshal(cb)
	if err := eng.SaveCodebook(a, cad.CodebookData{Mode: "live", Rules: raw, Note: "demo: live from the start"}); err != nil {
		return err
	}
	stations := []cad.Station{
		{ID: "ST1", Name: "Station 1 Downtown", Address: "Demo station, Church Ave SW", Lat: 37.26722, Lon: -79.94578},
		{ID: "ST2", Name: "Station 2 Peters Creek", Address: "Demo station, Peters Creek Rd NW", Lat: 37.32324, Lon: -79.96896},
		{ID: "ST3", Name: "Station 3 Garden City", Address: "Demo station, Garden City Blvd SE", Lat: 37.25063, Lon: -79.91657},
		{ID: "ST4", Name: "Station 4 Salem", Address: "Demo station, E Main St, Salem", Lat: 37.29182, Lon: -80.05668},
		{ID: "ST5", Name: "Station 5 Vinton", Address: "Demo station, Pollard St, Vinton", Lat: 37.27956, Lon: -79.89908},
		{ID: "PD", Name: "Police headquarters", Address: "Demo HQ, Campbell Ave SW", Lat: 37.27108, Lon: -79.94760},
	}
	for _, s := range stations {
		if err := eng.SaveStation(a, s); err != nil {
			return err
		}
	}
	units := []cad.UnitDef{
		{ID: "E1", Type: "engine", Station: "ST1"}, {ID: "E2", Type: "engine", Station: "ST2"}, {ID: "E3", Type: "engine", Station: "ST3"},
		{ID: "E4", Type: "engine", Station: "ST4"}, {ID: "E5", Type: "engine", Station: "ST5"},
		{ID: "L1", Type: "ladder", Station: "ST1"}, {ID: "L3", Type: "ladder", Station: "ST3"},
		{ID: "R1", Type: "rescue", Station: "ST1"}, {ID: "BC1", Type: "battalion", Station: "ST1"},
		{ID: "T5", Type: "tanker", Station: "ST5"}, {ID: "BR4", Type: "brush", Station: "ST4"},
		{ID: "M1", Type: "medic", Station: "ST1", RadioID: 101}, {ID: "M2", Type: "medic", Station: "ST2", RadioID: 102},
		{ID: "M3", Type: "medic", Station: "ST3", RadioID: 103}, {ID: "M4", Type: "medic", Station: "ST4", RadioID: 104},
		{ID: "A5", Type: "ambulance", Station: "ST5"}, {ID: "EMS1", Type: "ems-supervisor", Station: "ST2"},
		{ID: "2A11", Type: "patrol", Station: "PD", RadioID: 197, RadioAlias: "Adam 11"},
		{ID: "2A12", Type: "patrol", Station: "PD", RadioID: 198, RadioAlias: "Adam 12"},
		{ID: "2A13", Type: "patrol", Station: "PD", RadioID: 199}, {ID: "2A14", Type: "patrol", Station: "PD", RadioID: 200},
		{ID: "2A15", Type: "patrol", Station: "PD"}, {ID: "2S1", Type: "police-supervisor", Station: "PD", RadioID: 210},
		{ID: "K91", Type: "k9", Station: "PD"},
	}
	for _, u := range units {
		if err := eng.SaveUnit(a, u); err != nil {
			return err
		}
		st := cad.Available
		if u.ID == "E5" {
			st = cad.OutOfService
		}
		if err := eng.SetStatus(a, u.ID, st, "", "demo"); err != nil {
			return err
		}
	}
	// Two police cars on mark-outs of their own, so the police board has something to show.
	for _, m := range []struct {
		unit, code, addr, note string
		lat, lon               float64
	}{
		{"2A13", "MEAL", "Demo: Market St SE", "", 37.27099, -79.93919},
		{"2A15", "COURT", "Demo: Roanoke City Courthouse, Campbell Ave SW", "General District Court", 37.27113, -79.94370},
	} {
		lat, lon := m.lat, m.lon
		if err := eng.MarkOutUnit(a, m.unit, m.code, &ng911.Location{Address: m.addr, Lat: &lat, Lon: &lon, Source: "demo"}, m.note); err != nil {
			return err
		}
	}
	return seedDemoPlaces(eng, a)
}

// seedDemoPlaces gives some of the simulator's addresses premise information and earlier calls, as an
// import from an earlier system would (invented, marked demo).
func seedDemoPlaces(eng *cad.Engine, a cad.Actor) error {
	premises := []cad.Premise{
		{ID: "DEMO-1", Name: "Demo residence", Address: "2001 Crystal Spring Ave SW", Lat: 37.25027, Lon: -79.94605, Hazards: []string{"Oxygen in use", "Large dog"},
			Notes: "Bedroom on the first floor, rear. Demo data.", Contacts: []string{"Key holder: neighbour at 2003 (demo)"}},
		{ID: "DEMO-2", Name: "Demo warehouse", Address: "4333 Old Cave Spring Rd", Lat: 37.22889, Lon: -80.01241, Hazards: []string{"Hazardous materials: pool chemicals", "Knox box"},
			Notes: "Knox box left of the loading dock. Sprinklered. Demo data."},
		{ID: "DEMO-3", Address: "526 Orange Ave NE", Lat: 37.28271, Lon: -79.93232, Hazards: []string{"Officer safety: history of assault on officers"}, Notes: "Two officers. Demo data."},
	}
	if _, err := eng.ImportPremises(a, "demo", premises); err != nil {
		return err
	}
	at := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 14, 0, 0, 0, time.Local) }
	history := []cad.History{
		{Number: "19-004411", Type: "BRTH", Address: "2001 Crystal Spring Ave SW", Lat: 37.25027, Lon: -79.94605, At: at(2019, 3, 4), Disposition: "Transported"},
		{Number: "21-018822", Type: "FALL", Address: "2001 Crystal Spring Ave SW", Lat: 37.25027, Lon: -79.94605, At: at(2021, 11, 20), Disposition: "Lift assist"},
		{Number: "23-002201", Type: "FA", Address: "4333 Old Cave Spring Rd", Lat: 37.22889, Lon: -80.01241, At: at(2023, 1, 9), Disposition: "False alarm"},
	}
	_, err := eng.ImportHistory(a, "demo", history)
	return err
}

// demoScript is an example of an agency's own rule, in JavaScript.
var demoScript = advisory.Rule{ID: "script-hazards", Kind: advisory.KindScript, Name: "Hazards at the address", Enabled: true, Severity: "urgent", Lang: "javascript", Hook: "incident", TimeoutMs: 50,
	Explain: "Demo script: an incident at a premises with hazards, before any unit is sent, gets a reminder to tell the crew.",
	Source: `// Called for each open incident. Return nothing, or {title, detail, severity, action: {label, command}}.
function advise(i) {
  if (i.hazards.length === 0 || i.units.length > 0) return
  return { title: i.number + ": hazards at this address", detail: "Tell the crew: " + i.hazards.join(", "),
    action: { label: "Add a priority note", command: "PN Hazards: " + i.hazards.join(", ") } }
}`}
