// Package geo has the distance and travel-time arithmetic bitdispatcher needs, with no map data.
package geo

import "math"

const earthRadius = 6371008.8 // metres, the mean radius

// Distance is the great-circle distance in metres between two points in degrees.
func Distance(lat1, lon1, lat2, lon2 float64) float64 {
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadius * math.Asin(math.Min(1, math.Sqrt(a)))
}

// Estimate is a travel estimate without a road network: the straight line stretched by a road factor and
// driven at an average speed. It is labelled as such wherever it is shown.
type Estimate struct {
	Metres  float64 `json:"metres"`  // along the road, estimated
	Seconds float64 `json:"seconds"` // driving time, estimated
}

// Drive estimates driving distance and time. roadFactor is road length over straight-line length (1.3 to
// 1.5 in most road networks); kmh is the average emergency driving speed.
func Drive(lat1, lon1, lat2, lon2, roadFactor, kmh float64) Estimate {
	if roadFactor < 1 {
		roadFactor = 1
	}
	if kmh <= 0 {
		kmh = 50
	}
	m := Distance(lat1, lon1, lat2, lon2) * roadFactor
	return Estimate{Metres: m, Seconds: m / (kmh * 1000 / 3600)}
}
