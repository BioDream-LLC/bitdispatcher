package ng911

import (
	"encoding/json"
	"errors"
	"html"
	"strings"
	"time"
)

// Media is a piece of caller media named in an EIDO by reference.
type Media struct {
	ID      string `json:"id"`
	URL     string `json:"url"`
	Purpose string `json:"purpose"` // caller-photo, caller-video, caller-video-live
}

// Additional is the RFC 7852 Additional Data a CAD shows: who provides the service, what kind it is,
// the device and the subscriber.
type Additional struct {
	Providers []struct {
		Name    string `json:"name,omitempty"`
		Contact string `json:"contact,omitempty"`
	} `json:"providers,omitempty"`
	Service *struct {
		Environment string   `json:"environment,omitempty"`
		Types       []string `json:"types,omitempty"`
		Mobility    string   `json:"mobility,omitempty"`
	} `json:"service,omitempty"`
	Device *struct {
		Class string `json:"class,omitempty"`
		Maker string `json:"maker,omitempty"`
		Model string `json:"model,omitempty"`
	} `json:"device,omitempty"`
	Subscriber *struct {
		Name     string `json:"name,omitempty"`
		Address  string `json:"address,omitempty"`
		Language string `json:"language,omitempty"`
	} `json:"subscriber,omitempty"`
}

// Class is the class of service as an ALI shows it: WRLS, VOIP, RESD or BUSN.
func (a Additional) Class() string {
	if s := a.Service; s != nil {
		for _, t := range s.Types {
			switch strings.ToLower(t) {
			case "wireless":
				return "WRLS"
			case "voip", "nomadic":
				return "VOIP"
			case "pots", "coin":
				if strings.EqualFold(s.Environment, "business") {
					return "BUSN"
				}
				return "RESD"
			}
		}
	}
	return ""
}

// EIDOCall is what a CAD takes from an EIDO.
type EIDOCall struct {
	IncidentURN string      `json:"incidentUrn"`
	CallID      string      `json:"callId"` // the call-handling system's call identifier
	Agency      string      `json:"agency,omitempty"`
	Element     string      `json:"element,omitempty"`
	Callback    string      `json:"callback,omitempty"`
	State       string      `json:"state,omitempty"`
	Queue       string      `json:"queue,omitempty"`
	Agent       string      `json:"agent,omitempty"`
	Position    string      `json:"position,omitempty"`
	Start       time.Time   `json:"start,omitzero"`
	Answer      time.Time   `json:"answer,omitzero"`
	Updated     time.Time   `json:"updated,omitzero"`
	Location    *Location   `json:"location,omitempty"`
	Additional  *Additional `json:"additional,omitempty"`
	Media       []Media     `json:"media,omitempty"`
	Notes       []string    `json:"notes,omitempty"`
	Version     string      `json:"version,omitempty"`
}

type eidoRef struct {
	Ref string `json:"$ref"`
}

type eidoDoc struct {
	ID         string  `json:"$id"`
	Version    string  `json:"eidoVersion"`
	Element    string  `json:"issuingElementIdentification"`
	LastUpdate string  `json:"lastUpdateTimeStamp"`
	AgencyRef  eidoRef `json:"updatedByAgencyReference"`
	CallComp   []struct {
		ID         string  `json:"$id"`
		Queue      string  `json:"queueIdentifier"`
		Start      string  `json:"callStartTimestamp"`
		Answer     string  `json:"answerDate"`
		State      string  `json:"callStateRegistryText"`
		CallbackRf eidoRef `json:"callBackReference"`
	} `json:"callComponent"`
	Callback []struct {
		ID  string   `json:"$id"`
		URI []string `json:"callBackInformationUri"`
	} `json:"callbackComponent"`
	Agent []struct {
		ID       string `json:"$id"`
		Position string `json:"agentWorkstationPositionIdentification"`
	} `json:"agentComponent"`
	Location []struct {
		ID      string `json:"$id"`
		ByValue string `json:"locationByValue"`
		Type    string `json:"locationTypeDescriptionRegistryText"`
	} `json:"locationComponent"`
	Additional []struct {
		ID      string `json:"$id"`
		ByValue string `json:"additionalDataByValue"`
		URL     string `json:"additionalDataByReferenceUrl"`
		Purpose string `json:"urlPurpose"`
	} `json:"additionalDataComponent"`
	Notes []struct {
		Text string `json:"notesActionComments"`
	} `json:"notesComponent"`
}

// ParseEIDO reads an EIDO (NENA-STA-024 JSON).
func ParseEIDO(doc []byte) (*EIDOCall, error) {
	var d eidoDoc
	if err := json.Unmarshal(doc, &d); err != nil {
		return nil, err
	}
	if d.ID == "" || len(d.CallComp) == 0 {
		return nil, errors.New("ng911: an EIDO needs an $id and a callComponent")
	}
	out := &EIDOCall{IncidentURN: d.ID, Agency: d.AgencyRef.Ref, Element: d.Element, Version: d.Version, Updated: parseTS(d.LastUpdate)}
	call := d.CallComp[0]
	out.CallID = urnPart(call.ID, "callid")
	if out.CallID == "" {
		out.CallID = urnPart(d.ID, "incidentid")
	}
	if out.CallID == "" {
		out.CallID = call.ID
	}
	out.Queue, out.State = call.Queue, call.State
	out.Start, out.Answer = parseTS(call.Start), parseTS(call.Answer)
	for _, cb := range d.Callback {
		if call.CallbackRf.Ref != "" && cb.ID != call.CallbackRf.Ref {
			continue
		}
		for _, u := range cb.URI {
			if n, ok := strings.CutPrefix(u, "tel:"); ok {
				out.Callback = n
				break
			}
		}
	}
	if len(d.Agent) > 0 {
		out.Agent, out.Position = d.Agent[0].ID, d.Agent[0].Position
	}
	for _, l := range d.Location {
		if l.ByValue == "" {
			continue
		}
		if loc, err := ParsePIDF([]byte(l.ByValue)); err == nil {
			loc.Source = "eido"
			out.Location = loc
			break
		}
	}
	for _, a := range d.Additional {
		switch {
		case a.ByValue != "":
			var ad Additional
			if json.Unmarshal([]byte(a.ByValue), &ad) == nil {
				out.Additional = &ad
			}
		case a.URL != "":
			out.Media = append(out.Media, Media{ID: a.ID, URL: a.URL, Purpose: a.Purpose})
		}
	}
	for _, n := range d.Notes {
		if n.Text != "" {
			out.Notes = append(out.Notes, html.UnescapeString(n.Text))
		}
	}
	return out, nil
}

// urnPart takes the identifier out of urn:emergency:uid:<kind>:<id>:<element>.
func urnPart(urn, kind string) string {
	rest, ok := strings.CutPrefix(urn, "urn:emergency:uid:"+kind+":")
	if !ok {
		return ""
	}
	if i := strings.IndexByte(rest, ':'); i >= 0 {
		return rest[:i]
	}
	return rest
}

func parseTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02T15:04:05.000Z07:00", time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
