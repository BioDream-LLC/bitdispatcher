package ng911

import (
	"errors"
	"strings"
	"testing"
)

func TestHELD(t *testing.T) {
	req := string(HELDRequest("+1 540 555 0100", ""))
	if !strings.Contains(req, `responseTime="emergencyDispatch"`) || !strings.Contains(req, "<uri>tel:+1 540 555 0100</uri>") {
		t.Fatal(req)
	}
	ok := `<?xml version="1.0"?>
<locationResponse xmlns="urn:ietf:params:xml:ns:geopriv:held">
 <presence xmlns="urn:ietf:params:xml:ns:pidf" entity="pres:x@lis.example">
  <tuple id="t"><status><geopriv xmlns="urn:ietf:params:xml:ns:pidf:geopriv10"><location-info>
   <Circle xmlns="http://www.opengis.net/pidflo/1.0" srsName="urn:ogc:def:crs:EPSG::4326"><pos xmlns="http://www.opengis.net/gml">37.27312 -79.94021</pos><radius uom="urn:ogc:def:uom:EPSG::9001">8</radius></Circle>
  </location-info><method>DBH</method></geopriv></status></tuple>
 </presence>
</locationResponse>`
	loc, err := ParseHELD([]byte(ok))
	if err != nil || *loc.Lat != 37.27312 || loc.Radius != 8 {
		t.Fatalf("%+v %v", loc, err)
	}
	_, err = ParseHELD([]byte(`<error xmlns="urn:ietf:params:xml:ns:geopriv:held" code="locationUnknown"><message xml:lang="en">No fix</message></error>`))
	var he *HELDError
	if !errors.As(err, &he) || he.Code != "locationUnknown" || he.Message != "No fix" {
		t.Fatalf("%v", err)
	}
}
