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

// InPolygon reports whether a point is inside a polygon ring of [lon, lat] points (the ring may be open
// or closed). Points on an edge may fall either side.
func InPolygon(lat, lon float64, ring [][2]float64) bool {
	in := false
	n := len(ring)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		xi, yi := ring[i][0], ring[i][1]
		xj, yj := ring[j][0], ring[j][1]
		if (yi > lat) != (yj > lat) && lon < (xj-xi)*(lat-yi)/(yj-yi)+xi {
			in = !in
		}
	}
	return in
}

// Bearing is the initial compass bearing in degrees (0 north, 90 east) from the first point to the second.
func Bearing(lat1, lon1, lat2, lon2 float64) float64 {
	p1, p2 := lat1*math.Pi/180, lat2*math.Pi/180
	dl := (lon2 - lon1) * math.Pi / 180
	y := math.Sin(dl) * math.Cos(p2)
	x := math.Cos(p1)*math.Sin(p2) - math.Sin(p1)*math.Cos(p2)*math.Cos(dl)
	b := math.Atan2(y, x) * 180 / math.Pi
	if b < 0 {
		b += 360
	}
	return b
}
