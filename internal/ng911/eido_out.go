package ng911

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// SharedIncident is an incident one CAD sends another (CAD-to-CAD), as an EIDO (NENA-STA-024).
type SharedIncident struct {
	Agency   string    // the sending agency, as a URN or name
	Element  string    // the sending system (an NG9-1-1 element identifier or host name)
	Number   string    // the sender's incident number
	Type     string    // the incident type, in words
	Priority int       // 1 highest
	Status   string    // pending, active, closed
	Created  time.Time // when the incident was created
	Updated  time.Time
	Callback string // the caller's number, when shared
	Location *Location
	Notes    []string // what the receiving centre needs to know (the reason first)
}

// EncodePIDF writes a location as a PIDF-LO document (RFC 4119/5491): the point (and its radius) and
// the civic address parts that are known.
func EncodePIDF(l *Location, entity string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<presence xmlns="urn:ietf:params:xml:ns:pidf" xmlns:gp="urn:ietf:params:xml:ns:pidf:geopriv10" xmlns:gml="http://www.opengis.net/gml" xmlns:gs="http://www.opengis.net/pidflo/1.0" xmlns:ca="urn:ietf:params:xml:ns:pidf:geopriv10:civicAddr" entity="`)
	xml.EscapeText(&b, []byte(entity))
	b.WriteString(`"><tuple id="loc"><status><gp:geopriv><gp:location-info>`)
	if l.HasPoint() {
		if l.Radius > 0 {
			fmt.Fprintf(&b, `<gs:Circle srsName="urn:ogc:def:crs:EPSG::4326"><gml:pos>%.7f %.7f</gml:pos><gs:radius uom="urn:ogc:def:uom:EPSG::9001">%.0f</gs:radius></gs:Circle>`, *l.Lat, *l.Lon, l.Radius)
		} else {
			fmt.Fprintf(&b, `<gml:Point srsName="urn:ogc:def:crs:EPSG::4326"><gml:pos>%.7f %.7f</gml:pos></gml:Point>`, *l.Lat, *l.Lon)
		}
	}
	if c := l.Civic; c != nil {
		b.WriteString(`<ca:civicAddress>`)
		for _, kv := range [][2]string{{"country", c.Country}, {"A1", c.State}, {"A2", c.County}, {"A3", c.City}, {"PRD", c.PreDir}, {"RD", c.Street}, {"STS", c.StreetType},
			{"POD", c.PostDir}, {"HNO", c.Number}, {"HNS", c.NumberSuf}, {"LMK", c.Landmark}, {"LOC", c.Location}, {"NAM", c.Name}, {"PC", c.PostalCode}} {
			if kv[1] != "" {
				b.WriteString("<ca:" + kv[0] + ">")
				xml.EscapeText(&b, []byte(kv[1]))
				b.WriteString("</ca:" + kv[0] + ">")
			}
		}
		b.WriteString(`</ca:civicAddress>`)
	}
	b.WriteString(`</gp:location-info><gp:usage-rules/>`)
	if l.Method != "" {
		b.WriteString(`<gp:method>`)
		xml.EscapeText(&b, []byte(l.Method))
		b.WriteString(`</gp:method>`)
	}
	b.WriteString(`</gp:geopriv></status>`)
	if !l.At.IsZero() {
		b.WriteString(`<timestamp>` + l.At.UTC().Format(time.RFC3339) + `</timestamp>`)
	}
	b.WriteString(`</tuple></presence>`)
	return b.String()
}

// EncodeEIDO writes a shared incident as an EIDO: the incident, a call component (receivers built for
// call handling expect one), the location by value, and the notes.
func EncodeEIDO(in SharedIncident) ([]byte, error) {
	element := firstNon(in.Element, "bitdispatcher")
	id := "urn:emergency:uid:incidentid:" + in.Number + ":" + element
	ts := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.UTC().Format("2006-01-02T15:04:05.000Z")
	}
	doc := map[string]any{
		"$id":                          id,
		"eidoVersion":                  "1.0",
		"issuingElementIdentification": element,
		"lastUpdateTimeStamp":          ts(firstTime(in.Updated, in.Created)),
		"updatedByAgencyReference":     map[string]string{"$ref": in.Agency},
		"incidentComponent": []map[string]any{{
			"$id": "urn:emergency:uid:incidentid:" + in.Number, "incidentTrackingIdentifier": in.Number,
			"incidentTypeCommonRegistryText": in.Type, "incidentStatusCommonRegistryText": in.Status,
			"incidentStartTimeStamp": ts(in.Created), "incidentPriorityCommonRegistryText": fmt.Sprint(in.Priority),
		}},
		"callComponent": []map[string]any{{
			"$id": "urn:emergency:uid:callid:" + in.Number + ":" + element, "callStartTimestamp": ts(in.Created),
			"callStateRegistryText": "cad-to-cad", "callBackReference": map[string]string{"$ref": "cb1"},
		}},
	}
	if in.Callback != "" {
		doc["callbackComponent"] = []map[string]any{{"$id": "cb1", "callBackInformationUri": []string{"tel:" + in.Callback}}}
	}
	if in.Location != nil {
		doc["locationComponent"] = []map[string]any{{"$id": "loc1", "locationByValue": EncodePIDF(in.Location, "pres:"+in.Number+"@"+element), "locationTypeDescriptionRegistryText": "incident"}}
	}
	var notes []map[string]string
	if in.Location != nil && in.Location.Address != "" {
		notes = append(notes, map[string]string{"notesActionComments": "Address: " + in.Location.Address})
	}
	for _, n := range in.Notes {
		if strings.TrimSpace(n) != "" {
			notes = append(notes, map[string]string{"notesActionComments": n})
		}
	}
	if len(notes) > 0 {
		doc["notesComponent"] = notes
	}
	return json.Marshal(doc)
}

func firstNon(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func firstTime(v ...time.Time) time.Time {
	for _, t := range v {
		if !t.IsZero() {
			return t
		}
	}
	return time.Time{}
}

// IncidentInfo is what a receiving CAD reads from a shared incident's EIDO beyond the call: the
// sender's incident number, type, status and priority.
type IncidentInfo struct {
	Number, Type, Status, Priority string
}

// ParseIncidentInfo reads the incident component of an EIDO, when there is one.
func ParseIncidentInfo(doc []byte) IncidentInfo {
	var d struct {
		Inc []struct {
			Number   string `json:"incidentTrackingIdentifier"`
			Type     string `json:"incidentTypeCommonRegistryText"`
			Status   string `json:"incidentStatusCommonRegistryText"`
			Priority string `json:"incidentPriorityCommonRegistryText"`
		} `json:"incidentComponent"`
	}
	if json.Unmarshal(doc, &d) != nil || len(d.Inc) == 0 {
		return IncidentInfo{}
	}
	i := d.Inc[0]
	return IncidentInfo{Number: i.Number, Type: i.Type, Status: i.Status, Priority: i.Priority}
}
