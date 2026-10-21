package ng911

import (
	"strings"
	"testing"
	"time"
)

// An incident written as an EIDO reads back the same through the parser call handling uses.
func TestSharedIncidentRoundTrip(t *testing.T) {
	loc := Point(37.2712, -79.9418)
	loc.Radius = 15
	loc.Address = "215 Church Ave SW, Roanoke VA"
	loc.Civic = &Civic{Number: "215", Street: "Church", StreetType: "Ave", PostDir: "SW", City: "Roanoke", State: "VA"}
	at := time.Date(2026, 10, 20, 23, 0, 0, 0, time.UTC)
	doc, err := EncodeEIDO(SharedIncident{Agency: "urn:nena:agency:roanoke-ecc", Element: "cad.roanoke.example", Number: "26-000123", Type: "Structure fire",
		Priority: 1, Status: "active", Created: at, Callback: "+15405550100", Location: loc, Notes: []string{"Mutual aid: second alarm, need an engine & a ladder <now>"}})
	if err != nil {
		t.Fatal(err)
	}
	c, err := ParseEIDO(doc)
	if err != nil {
		t.Fatal(err)
	}
	if c.CallID != "26-000123" || c.Callback != "+15405550100" || c.Agency != "urn:nena:agency:roanoke-ecc" || !c.Start.Equal(at) {
		t.Fatalf("call %+v", c)
	}
	if c.Location == nil || *c.Location.Lat != 37.2712 || c.Location.Radius != 15 || c.Location.Civic == nil || c.Location.Civic.Number != "215" {
		t.Fatalf("location %+v", c.Location)
	}
	if len(c.Notes) != 2 || !strings.Contains(c.Notes[1], "engine & a ladder <now>") {
		t.Fatalf("notes %q", c.Notes)
	}
	if i := ParseIncidentInfo(doc); i.Number != "26-000123" || i.Type != "Structure fire" || i.Priority != "1" {
		t.Fatalf("incident %+v", i)
	}
}
