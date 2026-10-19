// Package ng911 reads what call handling sends a CAD: PIDF-LO locations (RFC 4119, 5139, 5491), NENA EIDO
// documents (NENA-STA-024) and legacy ALI spills. It reads; it never decides anything.
package ng911

import (
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/geo"
)

// Civic is a civic address (RFC 5139, NENA CLDXF).
type Civic struct {
	Country    string `json:"country,omitempty"`
	State      string `json:"state,omitempty"`  // A1
	County     string `json:"county,omitempty"` // A2
	City       string `json:"city,omitempty"`   // A3
	Number     string `json:"number,omitempty"` // HNO
	NumberSuf  string `json:"numberSuffix,omitempty"`
	PreDir     string `json:"preDir,omitempty"`
	Street     string `json:"street,omitempty"` // RD
	StreetType string `json:"streetType,omitempty"`
	PostDir    string `json:"postDir,omitempty"`
	Landmark   string `json:"landmark,omitempty"`
	Location   string `json:"location,omitempty"` // LOC
	Name       string `json:"name,omitempty"`     // NAM
	PostalCode string `json:"postalCode,omitempty"`
	Building   string `json:"building,omitempty"`
	Unit       string `json:"unit,omitempty"`
	Floor      string `json:"floor,omitempty"`
	Room       string `json:"room,omitempty"`
}

// StreetLine is the number, directions, street and type.
func (c Civic) StreetLine() string {
	return strings.Join(strings.Fields(strings.Join([]string{c.Number + c.NumberSuf, c.PreDir, c.Street, c.StreetType, c.PostDir}, " ")), " ")
}

// String is the address on one line.
func (c Civic) String() string {
	var parts []string
	if s := c.StreetLine(); s != "" {
		parts = append(parts, s)
	}
	for _, p := range [][2]string{{"Bldg ", c.Building}, {"Unit ", c.Unit}, {"Floor ", c.Floor}, {"Room ", c.Room}, {"", c.Location}} {
		if p[1] != "" {
			parts = append(parts, p[0]+p[1])
		}
	}
	if city := strings.Join(strings.Fields(c.City+" "+c.State+" "+c.PostalCode), " "); city != "" {
		parts = append(parts, city)
	}
	if c.Landmark != "" {
		parts = append(parts, "("+c.Landmark+")")
	}
	return strings.Join(parts, ", ")
}

// Location is where a caller is, as their network, device or a person said, with where it came from.
type Location struct {
	Address    string   `json:"address,omitempty"` // one line, for display
	Civic      *Civic   `json:"civic,omitempty"`
	Lat        *float64 `json:"lat,omitempty"`
	Lon        *float64 `json:"lon,omitempty"`
	Radius     float64  `json:"radius,omitempty"`     // metres of uncertainty
	Confidence int      `json:"confidence,omitempty"` // percent
	Method     string   `json:"method,omitempty"`     // GPS, Cell, DBH (device-based hybrid), Manual, ...
	// Altitude is metres above the WGS 84 ellipsoid, when the caller's device gave a 3-D fix (the z-axis
	// FCC rule): with VRadius, its uncertainty, it tells a dispatcher which floor of a tall building.
	Altitude *float64  `json:"altitude,omitempty"`
	VRadius  float64   `json:"vRadius,omitempty"`
	Source   string    `json:"source,omitempty"` // eido, ali, rebid, bitvoice, dispatcher
	At       time.Time `json:"at,omitzero"`      // when the location was determined
}

// HasPoint reports a geodetic position.
func (l *Location) HasPoint() bool { return l != nil && l.Lat != nil && l.Lon != nil }

// Point returns a location at a position.
func Point(lat, lon float64) *Location { return &Location{Lat: &lat, Lon: &lon} }

var errNotPIDF = errors.New("ng911: not a PIDF-LO document")

// ParsePIDF reads the first civic address and the first geodetic shape of a PIDF-LO document.
func ParsePIDF(doc []byte) (*Location, error) {
	d := xml.NewDecoder(strings.NewReader(string(doc)))
	d.Strict = false
	loc := &Location{}
	var civic Civic
	var stack []string
	sawPresence, sawCivic, sawPoint := false, false, false
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			if t.Name.Local == "presence" {
				sawPresence = true
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			v := strings.TrimSpace(string(t))
			if v == "" {
				continue
			}
			name := stack[len(stack)-1]
			inCivic := contains(stack, "civicAddress")
			if inCivic {
				sawCivic = true
				setCivic(&civic, name, v)
				continue
			}
			switch name {
			case "pos":
				if !sawPoint {
					if f := strings.Fields(v); len(f) >= 2 {
						la, e1 := strconv.ParseFloat(f[0], 64)
						lo, e2 := strconv.ParseFloat(f[1], 64)
						if e1 == nil && e2 == nil {
							loc.Lat, loc.Lon, sawPoint = &la, &lo, true
						}
						if len(f) >= 3 {
							if alt, err := strconv.ParseFloat(f[2], 64); err == nil {
								loc.Altitude = &alt
							}
						}
					}
				}
			case "posList":
				if !sawPoint {
					// A polygon: its centre stands in for the point; the radius covers its farthest vertex.
					if la, lo, r, ok := centroid(v); ok {
						loc.Lat, loc.Lon, loc.Radius, sawPoint = &la, &lo, r, true
					}
				}
			case "radius", "semiMajorAxis":
				if r, err := strconv.ParseFloat(v, 64); err == nil && loc.Radius == 0 {
					loc.Radius = r
				}
			case "verticalAxis", "height":
				if r, err := strconv.ParseFloat(v, 64); err == nil && loc.VRadius == 0 {
					loc.VRadius = r
				}
			case "confidence":
				if c, err := strconv.Atoi(v); err == nil {
					loc.Confidence = c
				}
			case "method":
				loc.Method = v
			case "timestamp":
				if ts, err := time.Parse(time.RFC3339, v); err == nil {
					loc.At = ts
				}
			}
		}
	}
	if !sawPresence {
		return nil, errNotPIDF
	}
	if sawCivic {
		loc.Civic = &civic
		loc.Address = civic.String()
	}
	return loc, nil
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func setCivic(c *Civic, name, v string) {
	switch name {
	case "country":
		c.Country = v
	case "A1":
		c.State = v
	case "A2":
		c.County = v
	case "A3":
		c.City = v
	case "HNO":
		c.Number = v
	case "HNS":
		c.NumberSuf = v
	case "PRD":
		c.PreDir = v
	case "RD", "A6":
		if c.Street == "" {
			c.Street = v
		}
	case "STS":
		c.StreetType = v
	case "POD":
		c.PostDir = v
	case "LMK":
		c.Landmark = v
	case "LOC":
		c.Location = v
	case "NAM":
		c.Name = v
	case "PC":
		c.PostalCode = v
	case "BLD":
		c.Building = v
	case "UNIT":
		c.Unit = v
	case "FLR":
		c.Floor = v
	case "ROOM":
		c.Room = v
	}
}

func centroid(posList string) (lat, lon, radius float64, ok bool) {
	f := strings.Fields(posList)
	var pts [][2]float64
	for i := 0; i+1 < len(f); i += 2 {
		a, e1 := strconv.ParseFloat(f[i], 64)
		b, e2 := strconv.ParseFloat(f[i+1], 64)
		if e1 != nil || e2 != nil {
			return 0, 0, 0, false
		}
		pts = append(pts, [2]float64{a, b})
	}
	if n := len(pts); n > 3 && pts[0] == pts[n-1] {
		pts = pts[:n-1] // a closed ring repeats its first vertex
	}
	if len(pts) < 3 {
		return 0, 0, 0, false
	}
	for _, p := range pts {
		lat += p[0]
		lon += p[1]
	}
	lat /= float64(len(pts))
	lon /= float64(len(pts))
	for _, p := range pts {
		if d := geo.Distance(lat, lon, p[0], p[1]); d > radius {
			radius = d
		}
	}
	return lat, lon, radius, true
}
