package ng911

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// HELD (RFC 5985) asks a location information server (LIS) where a device is: an NG9-1-1 LIS, or a
// clearinghouse that answers HELD for its partners. The device is named by its telephone number (RFC 6155).

// HELDContentType is the media type of HELD requests and answers.
const HELDContentType = "application/held+xml;charset=utf-8"

// HELDRequest is a location request for the device with this number. responseTime is "emergencyDispatch"
// (the best location, however long it takes the server) or "emergencyRouting" (fast, for routing).
func HELDRequest(callback, responseTime string) []byte {
	if responseTime == "" {
		responseTime = "emergencyDispatch"
	}
	num := strings.TrimPrefix(strings.TrimSpace(callback), "tel:")
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<locationRequest xmlns="urn:ietf:params:xml:ns:geopriv:held" responseTime="`)
	xml.EscapeText(&b, []byte(responseTime))
	b.WriteString(`"><locationType exact="false">geodetic civic</locationType>`)
	b.WriteString(`<device xmlns="urn:ietf:params:xml:ns:geopriv:held:id"><uri>tel:`)
	xml.EscapeText(&b, []byte(num))
	b.WriteString(`</uri></device></locationRequest>`)
	return []byte(b.String())
}

// HELDError is the server's answer when it has no location.
type HELDError struct {
	Code    string // locationUnknown, cannotProvideLiType, notLocatable, requestError, ...
	Message string
}

func (e *HELDError) Error() string {
	if e.Message != "" {
		return "held: " + e.Code + ": " + e.Message
	}
	return "held: " + e.Code
}

// ParseHELD reads a location response: the PIDF-LO it carries, or the server's error.
func ParseHELD(doc []byte) (*Location, error) {
	d := xml.NewDecoder(strings.NewReader(string(doc)))
	d.Strict = false
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "error":
			e := &HELDError{}
			for _, a := range se.Attr {
				if a.Name.Local == "code" {
					e.Code = a.Value
				}
			}
			var inner struct {
				Message string `xml:"message"`
			}
			_ = d.DecodeElement(&inner, &se)
			e.Message = strings.TrimSpace(inner.Message)
			if e.Code == "" {
				e.Code = "error"
			}
			return nil, e
		case "locationResponse":
			loc, err := ParsePIDF(doc)
			if err != nil {
				return nil, fmt.Errorf("held: the location in the answer: %w", err)
			}
			if !loc.HasPoint() && loc.Civic == nil {
				return nil, &HELDError{Code: "locationUnknown", Message: "the answer has no location"}
			}
			return loc, nil
		}
	}
	return nil, errors.New("held: not a location response")
}
