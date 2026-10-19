package mapdata

import (
	"math"
	"strconv"
	"strings"
)

// Layers in the store and in vector tiles.
const (
	LWater    = "water"    // polygons
	LWaterway = "waterway" // lines: rivers, streams
	LLand     = "land"     // landuse and landcover polygons
	LBuilding = "building"
	LRoad     = "road"
	LRail     = "rail"
	LAeroway  = "aeroway"
	LBoundary = "boundary"
	LPlace    = "place" // settlement names
	LPOI      = "poi"   // public-safety places first: fire, police, hospitals, hydrants
	LAddr     = "addr"  // house numbers
	LLabel    = "label" // names of areas: lakes, parks, campuses
)

// spec is what a set of tags becomes.
type spec struct {
	layer, kind string
	minzoom     int
	area        bool // drawn as a polygon
	byArea      bool // minzoom chosen from the polygon's size
}

// roadClass gives each highway its kind and first zoom.
var roadClass = map[string]spec{
	"motorway": {LRoad, "motorway", 5, false, false}, "motorway_link": {LRoad, "motorway_link", 10, false, false},
	"trunk": {LRoad, "trunk", 6, false, false}, "trunk_link": {LRoad, "trunk_link", 10, false, false},
	"primary": {LRoad, "primary", 8, false, false}, "primary_link": {LRoad, "primary_link", 11, false, false},
	"secondary": {LRoad, "secondary", 9, false, false}, "secondary_link": {LRoad, "secondary_link", 12, false, false},
	"tertiary": {LRoad, "tertiary", 10, false, false}, "tertiary_link": {LRoad, "tertiary_link", 12, false, false},
	"residential": {LRoad, "minor", 12, false, false}, "unclassified": {LRoad, "minor", 11, false, false},
	"living_street": {LRoad, "minor", 13, false, false}, "road": {LRoad, "minor", 12, false, false},
	"service": {LRoad, "service", 14, false, false}, "track": {LRoad, "track", 13, false, false},
	"pedestrian": {LRoad, "path", 14, false, false}, "footway": {LRoad, "path", 15, false, false},
	"path": {LRoad, "path", 15, false, false}, "cycleway": {LRoad, "path", 15, false, false},
	"steps": {LRoad, "path", 15, false, false}, "bridleway": {LRoad, "path", 15, false, false},
	"construction": {LRoad, "construction", 13, false, false}, "busway": {LRoad, "service", 14, false, false},
}

// landKind maps landuse, natural, leisure and amenity area tags to a land kind.
func landKind(t map[string]string) string {
	switch t["landuse"] {
	case "forest":
		return "wood"
	case "residential":
		return "residential"
	case "commercial", "retail":
		return "commercial"
	case "industrial", "railway", "port":
		return "industrial"
	case "cemetery":
		return "cemetery"
	case "grass", "meadow", "recreation_ground", "village_green", "greenfield", "flowerbed":
		return "grass"
	case "farmland", "farmyard", "orchard", "vineyard", "allotments", "plant_nursery", "greenhouse_horticulture":
		return "farm"
	case "military":
		return "military"
	case "quarry", "landfill", "brownfield", "construction":
		return "bare"
	}
	switch t["natural"] {
	case "wood":
		return "wood"
	case "scrub", "heath", "grassland":
		return "scrub"
	case "wetland":
		return "wetland"
	case "bare_rock", "scree", "sand", "beach", "shingle", "cliff":
		return "bare"
	}
	switch t["leisure"] {
	case "park", "nature_reserve", "garden", "dog_park", "playground", "common":
		return "park"
	case "golf_course":
		return "golf"
	case "pitch", "sports_centre", "stadium", "track":
		return "pitch"
	}
	switch t["amenity"] {
	case "hospital":
		return "hospital"
	case "school", "college", "university", "kindergarten":
		return "school"
	case "parking":
		return "parking"
	case "prison":
		return "prison"
	}
	switch t["boundary"] {
	case "national_park", "protected_area":
		return "park"
	}
	if t["aeroway"] == "aerodrome" {
		return "airport"
	}
	if t["aeroway"] == "apron" {
		return "apron"
	}
	return ""
}

func isWater(t map[string]string) bool {
	return t["natural"] == "water" || t["waterway"] == "riverbank" || t["landuse"] == "reservoir" || t["landuse"] == "basin" ||
		t["natural"] == "bay"
}

// minzoomForArea picks the first zoom at which a polygon of a square metres is worth drawing.
func minzoomForArea(a float64, floor int) int {
	z := 15
	switch {
	case a > 200e6:
		z = 5
	case a > 30e6:
		z = 7
	case a > 5e6:
		z = 8
	case a > 1e6:
		z = 9
	case a > 250e3:
		z = 10
	case a > 60e3:
		z = 11
	case a > 15e3:
		z = 12
	case a > 4e3:
		z = 13
	case a > 1e3:
		z = 14
	}
	if z < floor {
		z = floor
	}
	return z
}

// areaSpec is the polygon a closed way or multipolygon makes, if any.
func areaSpec(t map[string]string) (spec, bool) {
	if t["area"] == "no" {
		return spec{}, false
	}
	if isWater(t) {
		return spec{LWater, waterKind(t), 0, true, true}, true
	}
	if b := t["building"]; b != "" && b != "no" {
		return spec{LBuilding, buildingKind(t), 14, true, false}, true
	}
	if k := landKind(t); k != "" {
		floor := 8
		switch k {
		case "parking", "pitch":
			floor = 14
		case "residential", "commercial", "industrial", "farm", "grass":
			floor = 10
		}
		return spec{LLand, k, floor, true, true}, true
	}
	switch t["aeroway"] {
	case "runway", "taxiway":
		if t["area"] == "yes" {
			return spec{LAeroway, t["aeroway"], 11, true, false}, true
		}
	}
	return spec{}, false
}

func waterKind(t map[string]string) string {
	if t["water"] == "river" || t["waterway"] == "riverbank" {
		return "river"
	}
	if t["leisure"] == "swimming_pool" || t["water"] == "pool" {
		return "pool"
	}
	return "lake"
}

func buildingKind(t map[string]string) string {
	switch t["building"] {
	case "hospital":
		return "hospital"
	case "school", "university", "college":
		return "school"
	case "fire_station":
		return "fire"
	case "industrial", "warehouse", "commercial", "retail", "office", "manufacture":
		return "commercial"
	case "church", "chapel", "cathedral", "mosque", "synagogue", "temple":
		return "worship"
	}
	switch t["amenity"] {
	case "hospital":
		return "hospital"
	case "fire_station", "police":
		return "fire"
	case "school":
		return "school"
	}
	return "building"
}

// lineSpec is the line a way makes, if any.
func lineSpec(t map[string]string) (spec, bool) {
	if h := t["highway"]; h != "" && t["area"] != "yes" {
		if h == "construction" && t["construction"] == "" {
			return spec{}, false
		}
		if s, ok := roadClass[h]; ok {
			if h == "service" {
				switch t["service"] {
				case "driveway", "parking_aisle", "drive-through":
					s.minzoom = 15
				}
			}
			return s, true
		}
	}
	switch t["railway"] {
	case "rail":
		if t["service"] != "" {
			return spec{LRail, "spur", 13, false, false}, true
		}
		return spec{LRail, "rail", 8, false, false}, true
	case "light_rail", "subway", "tram", "narrow_gauge", "preserved":
		return spec{LRail, "rail", 11, false, false}, true
	}
	switch t["waterway"] {
	case "river":
		return spec{LWaterway, "river", 8, false, false}, true
	case "canal":
		return spec{LWaterway, "canal", 10, false, false}, true
	case "stream":
		return spec{LWaterway, "stream", 12, false, false}, true
	case "ditch", "drain":
		return spec{LWaterway, "ditch", 14, false, false}, true
	case "dam", "weir":
		return spec{LWaterway, "dam", 13, false, false}, true
	}
	switch t["aeroway"] {
	case "runway":
		return spec{LAeroway, "runway", 10, false, false}, true
	case "taxiway":
		return spec{LAeroway, "taxiway", 13, false, false}, true
	}
	if t["barrier"] == "fence" || t["barrier"] == "wall" {
		return spec{}, false
	}
	return spec{}, false
}

// pointSpec is what a tagged node, or the centre of an area, is: a place, POI or nothing.
func pointSpec(t map[string]string) (spec, bool) {
	switch t["place"] {
	case "city":
		return spec{LPlace, "city", 5, false, false}, true
	case "town":
		return spec{LPlace, "town", 8, false, false}, true
	case "village":
		return spec{LPlace, "village", 10, false, false}, true
	case "hamlet":
		return spec{LPlace, "hamlet", 12, false, false}, true
	case "suburb", "quarter":
		return spec{LPlace, "suburb", 11, false, false}, true
	case "neighbourhood":
		return spec{LPlace, "neighbourhood", 13, false, false}, true
	case "locality", "isolated_dwelling", "farm":
		return spec{LPlace, "locality", 14, false, false}, true
	}
	switch t["emergency"] {
	case "fire_hydrant":
		return spec{LPOI, "hydrant", 16, false, false}, true
	case "ambulance_station":
		return spec{LPOI, "ems", 11, false, false}, true
	case "defibrillator":
		return spec{LPOI, "aed", 17, false, false}, true
	case "water_tank", "suction_point", "fire_water_pond":
		return spec{LPOI, "water", 15, false, false}, true
	case "phone":
		return spec{LPOI, "phone", 17, false, false}, true
	}
	switch t["amenity"] {
	case "fire_station":
		return spec{LPOI, "fire", 11, false, false}, true
	case "police":
		return spec{LPOI, "police", 11, false, false}, true
	case "hospital":
		return spec{LPOI, "hospital", 10, false, false}, true
	case "clinic", "doctors", "urgent_care":
		return spec{LPOI, "clinic", 15, false, false}, true
	case "school", "college", "university", "kindergarten":
		return spec{LPOI, "school", 13, false, false}, true
	case "nursing_home":
		return spec{LPOI, "care", 14, false, false}, true
	case "townhall", "courthouse":
		return spec{LPOI, "government", 14, false, false}, true
	case "prison":
		return spec{LPOI, "prison", 13, false, false}, true
	case "place_of_worship":
		return spec{LPOI, "worship", 15, false, false}, true
	case "fuel":
		return spec{LPOI, "fuel", 15, false, false}, true
	case "pharmacy":
		return spec{LPOI, "pharmacy", 16, false, false}, true
	case "library", "community_centre", "social_facility":
		return spec{LPOI, "civic", 15, false, false}, true
	case "restaurant", "fast_food", "cafe", "bank", "bar", "pub", "cinema", "theatre", "post_office":
		if t["name"] != "" {
			return spec{LPOI, "business", 16, false, false}, true
		}
	}
	switch t["aeroway"] {
	case "helipad":
		return spec{LPOI, "helipad", 13, false, false}, true
	case "aerodrome":
		return spec{LPOI, "airport", 9, false, false}, true
	}
	if t["healthcare"] == "hospital" {
		return spec{LPOI, "hospital", 10, false, false}, true
	}
	if t["social_facility"] == "nursing_home" || t["social_facility"] == "assisted_living" {
		return spec{LPOI, "care", 14, false, false}, true
	}
	if t["highway"] == "motorway_junction" {
		return spec{LPOI, "exit", 12, false, false}, true
	}
	if t["railway"] == "level_crossing" || t["railway"] == "crossing" {
		return spec{LPOI, "crossing", 16, false, false}, true
	}
	if t["barrier"] == "gate" || t["barrier"] == "lift_gate" {
		return spec{LPOI, "gate", 16, false, false}, true
	}
	if t["man_made"] == "water_tower" {
		return spec{LPOI, "tower", 15, false, false}, true
	}
	if t["shop"] != "" && t["name"] != "" {
		return spec{LPOI, "business", 16, false, false}, true
	}
	if t["tourism"] != "" && t["name"] != "" && (t["tourism"] == "hotel" || t["tourism"] == "motel" || t["tourism"] == "attraction" || t["tourism"] == "museum") {
		return spec{LPOI, "business", 15, false, false}, true
	}
	if t["leisure"] == "stadium" && t["name"] != "" {
		return spec{LPOI, "civic", 13, false, false}, true
	}
	return spec{}, false
}

// attrs are the properties a tile carries for a feature.
func attrs(layer string, t map[string]string) map[string]string {
	a := map[string]string{}
	set := func(k, v string) {
		if v != "" {
			a[k] = v
		}
	}
	switch layer {
	case LRoad:
		set("ref", t["ref"])
		if ow := oneway(t); ow != 0 {
			a["oneway"] = strconv.Itoa(ow)
		}
		if t["bridge"] != "" && t["bridge"] != "no" {
			a["bridge"] = "1"
		}
		if t["tunnel"] != "" && t["tunnel"] != "no" {
			a["tunnel"] = "1"
		}
		set("layer", t["layer"])
		if t["access"] == "private" || t["access"] == "no" {
			a["private"] = "1"
		}
		if t["surface"] == "unpaved" || t["surface"] == "gravel" || t["surface"] == "dirt" || t["surface"] == "ground" {
			a["unpaved"] = "1"
		}
		set("maxspeed", t["maxspeed"])
	case LPOI:
		set("ref", t["ref"])
		set("num", t["addr:housenumber"])
		set("op", firstOf(t, "operator", "fire_hydrant:type"))
	case LBuilding:
		set("num", t["addr:housenumber"])
		set("levels", t["building:levels"])
	case LPlace:
		set("pop", t["population"])
	case LBoundary:
		set("level", t["admin_level"])
	}
	return a
}

func firstOf(t map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := t[k]; v != "" {
			return v
		}
	}
	return ""
}

// oneway is 1 (with the way), -1 (against) or 0.
func oneway(t map[string]string) int {
	switch t["oneway"] {
	case "yes", "true", "1":
		return 1
	case "-1", "reverse":
		return -1
	case "no", "false", "0":
		return 0
	}
	if t["junction"] == "roundabout" || t["junction"] == "circular" || t["highway"] == "motorway" || t["highway"] == "motorway_link" {
		return 1
	}
	return 0
}

// name is the label shown: the name, else the ref for numbered roads.
func name(t map[string]string) string {
	if n := t["name"]; n != "" {
		return n
	}
	return ""
}

// ---- driving ----

// driveable highway kinds, and their default speed in km/h when maxspeed is not tagged.
var driveSpeed = map[string]float64{
	"motorway": 105, "motorway_link": 60, "trunk": 90, "trunk_link": 55, "primary": 70, "primary_link": 50,
	"secondary": 65, "secondary_link": 45, "tertiary": 55, "tertiary_link": 40, "unclassified": 50,
	"residential": 40, "living_street": 20, "road": 40, "service": 20, "track": 15, "busway": 30,
}

// speedKmh is how fast a vehicle drives a way, for routing.
func speedKmh(t map[string]string) (float64, bool) {
	h := t["highway"]
	def, ok := driveSpeed[h]
	if !ok {
		return 0, false
	}
	if t["access"] == "no" && t["emergency"] != "yes" && t["motor_vehicle"] != "yes" {
		return 0, false
	}
	if t["motor_vehicle"] == "no" && t["emergency"] != "yes" {
		return 0, false
	}
	if t["area"] == "yes" {
		return 0, false
	}
	if v := parseSpeed(t["maxspeed"]); v > 0 {
		def = v
	}
	switch t["surface"] {
	case "unpaved", "gravel", "dirt", "ground", "compacted":
		def = math.Min(def, 40)
	}
	if h == "service" {
		switch t["service"] {
		case "driveway", "parking_aisle", "drive-through":
			def = 10
		}
	}
	return def, true
}

// parseSpeed reads maxspeed values like "35 mph", "50", "50 km/h".
func parseSpeed(s string) float64 {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0
	}
	mph := strings.HasSuffix(s, "mph")
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(s, "mph"), "km/h"), "kmh"))
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v <= 0 || v > 200 {
		return 0
	}
	if mph {
		v *= 1.609344
	}
	return v
}
