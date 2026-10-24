package geo

import (
	"math"
	"testing"
)

func TestDistance(t *testing.T) {
	// Harrisburg PA to Philadelphia PA is about 151 km in a straight line.
	d := Distance(40.2732, -76.8867, 39.9526, -75.1652)
	if math.Abs(d-151000) > 3000 {
		t.Fatalf("distance %f", d)
	}
	if Distance(40, -76, 40, -76) != 0 {
		t.Fatal("same point not zero")
	}
}

func TestDrive(t *testing.T) {
	e := Drive(40.0, -76.0, 40.009, -76.0, 1.4, 50) // about 1 km north
	if math.Abs(e.Metres-1400) > 20 || math.Abs(e.Seconds-100.8) > 2 {
		t.Fatalf("estimate %+v", e)
	}
}

func TestInPolygon(t *testing.T) {
	sq := [][2]float64{{-80, 37}, {-79.9, 37}, {-79.9, 37.1}, {-80, 37.1}}
	if !InPolygon(37.05, -79.95, sq) {
		t.Error("centre should be inside")
	}
	if InPolygon(37.2, -79.95, sq) || InPolygon(37.05, -80.1, sq) {
		t.Error("outside points reported inside")
	}
	closed := append(sq, sq[0])
	if !InPolygon(37.05, -79.95, closed) {
		t.Error("closed ring")
	}
}
