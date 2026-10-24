// Package nemsis turns what a 911 CAD knows about one EMS unit's response into a NEMSIS v3.5
// EMSDataSet XML document and hands it to an ePCR system. This is the CAD-to-ePCR hand-off: the
// dispatch part of the record (who the agency is, the incident and unit, the call type, the response
// times and where it was). The crew completes the rest of the patient care report in their own ePCR,
// so this document carries only the eRecord, eResponse, eDispatch, eTimes and eScene sections.
//
// The element names, their order within each group, the code values (eResponse.05 type of service,
// eResponse.07 transport capability, eResponse.23 response mode, eDispatch.01 dispatch reason,
// eDispatch.02 EMD performed), the http://www.nemsis.org namespace and the null-value handling
// (xsi:nil="true" with an NV attribute of 7701001 Not Applicable or 7701003 Not Recorded) were taken
// from the published NEMSIS v3.5.1 XSDs, read element by element from these files:
//
//	https://nemsis.org/media/nemsis_v3/release-3.5.1/XSDs/NEMSIS_XSDs/EMSDataSet_v3.xsd
//	https://nemsis.org/media/nemsis_v3/release-3.5.1/XSDs/NEMSIS_XSDs/commonTypes_v3.xsd
//	https://nemsis.org/media/nemsis_v3/release-3.5.1/XSDs/NEMSIS_XSDs/dAgency_v3.xsd
//	https://nemsis.org/media/nemsis_v3/release-3.5.1/XSDs/NEMSIS_XSDs/eRecord_v3.xsd
//	https://nemsis.org/media/nemsis_v3/release-3.5.1/XSDs/NEMSIS_XSDs/eResponse_v3.xsd
//	https://nemsis.org/media/nemsis_v3/release-3.5.1/XSDs/NEMSIS_XSDs/eDispatch_v3.xsd
//	https://nemsis.org/media/nemsis_v3/release-3.5.1/XSDs/NEMSIS_XSDs/eTimes_v3.xsd
//	https://nemsis.org/media/nemsis_v3/release-3.5.1/XSDs/NEMSIS_XSDs/eScene_v3.xsd
//
// The document is only a partial patient care report by design, so a full EMSDataSet XSD validation of
// the output is expected to report the later required sections (ePatient, eDisposition and so on) as
// missing; the sections this package does produce follow the schema's names and order.
package nemsis

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NEMSIS constants: the data standard version this package was written against, the XML namespace and
// the schema-instance namespace used for the nil attribute.
const (
	Version      = "3.5.1"
	Namespace    = "http://www.nemsis.org"
	xsiNamespace = "http://www.w3.org/2001/XMLSchema-instance"
)

// Null values (the NV attribute). NEMSIS uses these when a required element has no real value.
const (
	NVNotApplicable = "7701001" // the element does not apply to this call
	NVNotRecorded   = "7701003" // the value was not captured
)

// timeFormat is the NEMSIS dateTime: ISO 8601 with a numeric offset, e.g. 2026-10-23T14:05:00-05:00.
const timeFormat = "2006-01-02T15:04:05-07:00"

// Config is what an administrator sets for the ePCR hand-off.
type Config struct {
	On            bool   `json:"on"`
	URL           string `json:"url"`             // the ePCR system's import endpoint, https
	Token         string `json:"token,omitempty"` // Bearer token
	Agency        string `json:"agency"`          // the EMS agency number (dAgency.02 / eResponse.01)
	AgencyStateID string `json:"agencyStateId"`   // dAgency.01, the agency's unique state ID
	State         string `json:"state"`           // two-letter, e.g. VA; sent as the ANSI (FIPS) state code
	Auto          bool   `json:"auto"`            // send when the unit clears
}

// Check says what is wrong with a configuration that is on.
func (c Config) Check() error {
	if !c.On {
		return nil
	}
	if strings.TrimSpace(c.Agency) == "" {
		return errors.New("the EMS agency number is required")
	}
	if ansiState(c.State) == "" {
		return errors.New("the agency state is not a state we know (use a two-letter code such as VA)")
	}
	u, err := url.Parse(strings.TrimSpace(c.URL))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("the ePCR import URL must be a web address")
	}
	if u.Scheme != "https" && !isLocal(u.Hostname()) {
		return errors.New("the ePCR import URL must be https unless it is 127.0.0.1 or localhost")
	}
	return nil
}

func isLocal(host string) bool {
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

// Response is what bitdispatcher knows of one unit's response, for NEMSIS.
type Response struct {
	Incident      string // the incident (CAD) number
	Unit          string // the unit identifier, used to make the PCR number stable
	CallSign      string // eResponse.14
	VehicleNumber string // eResponse.13
	Capability    string // als, bls, etc, optional; maps to eResponse.07
	Reason        string // the call type as known to CAD (free text, kept for SuggestReason)
	ReasonCode    string // eDispatch.01 code if the agency mapped one
	EMD           string // "", "yes", "no" for eDispatch.02
	LightsSirens  *bool  // eResponse.23 response mode to scene, if known

	PSAPCall         time.Time // eTimes.01
	DispatchNotified time.Time // eTimes.02
	UnitNotified     time.Time // eTimes.03 (required)
	EnRoute          time.Time // eTimes.05
	OnScene          time.Time // eTimes.06
	LeftScene        time.Time // eTimes.09
	AtDestination    time.Time // eTimes.11
	BackInService    time.Time // eTimes.13

	Address   string // eScene.15
	City      string // eScene.17
	StateCode string // eScene.18, two-letter; mapped to the ANSI (FIPS) code
	ZIP       string // eScene.19
	County    string // eScene.21
	Lat, Lon  *float64

	UUID string // stable per incident+unit; derived from them when empty
}

// Code is a code value with its name, for an administrator mapping call types.
type Code struct{ Code, Name string }

// Encode returns a full NEMSIS EMSDataSet XML document, with the XML declaration, for one response.
// version is the NEMSIS data standard version to record in the schema location; it falls back to the
// version this package follows when empty.
func Encode(c Config, r Response, version string) ([]byte, error) {
	if version == "" {
		version = Version
	}
	doc := build(c, r, version)
	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	out := append([]byte(xml.Header), body...)
	out = append(out, '\n')
	return out, nil
}

// Validate returns plain-English problems a dispatcher can fix before sending.
func Validate(r Response) []string {
	var p []string
	if strings.TrimSpace(r.Incident) == "" {
		p = append(p, "there is no incident number")
	}
	if strings.TrimSpace(r.Unit) == "" && strings.TrimSpace(r.CallSign) == "" {
		p = append(p, "there is no unit (call sign or unit number)")
	}
	if r.UnitNotified.IsZero() {
		p = append(p, "there is no time the unit was notified by dispatch (NEMSIS requires it)")
	}
	if r.PSAPCall.IsZero() {
		p = append(p, "there is no time the 911 call came in")
	}
	if !r.EnRoute.IsZero() && !r.UnitNotified.IsZero() && r.EnRoute.Before(r.UnitNotified) {
		p = append(p, "the unit went en route before it was notified")
	}
	if !r.OnScene.IsZero() && !r.EnRoute.IsZero() && r.OnScene.Before(r.EnRoute) {
		p = append(p, "the unit arrived on scene before it went en route")
	}
	if !r.BackInService.IsZero() && !r.UnitNotified.IsZero() && r.BackInService.Before(r.UnitNotified) {
		p = append(p, "the unit went back in service before it was notified")
	}
	return p
}

// Client talks to the ePCR system.
type Client struct {
	HTTP *http.Client
}

func (cl *Client) http() *http.Client {
	if cl.HTTP != nil {
		return cl.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// Send POSTs the document to the ePCR system as application/xml with a Bearer token. A non-2xx reply is
// an error naming the status; the returned body is trimmed to 2000 characters either way.
func (cl *Client) Send(ctx context.Context, c Config, doc []byte) (status int, body string, err error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.URL, bytes.NewReader(doc))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Accept", "application/xml")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := cl.http().Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	body = string(raw)
	if len(body) > 2000 {
		body = body[:2000]
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, body, fmt.Errorf("the ePCR system refused the record (%s)", resp.Status)
	}
	return resp.StatusCode, body, nil
}

// dispatchReasons are the eDispatch.01 Dispatch Reason codes from the NEMSIS v3.5.1 data dictionary.
var dispatchReasons = []Code{
	{"2301001", "Abdominal Pain/Problems"},
	{"2301003", "Allergic Reaction/Stings"},
	{"2301005", "Animal Bite"},
	{"2301007", "Assault"},
	{"2301009", "Automated Crash Notification"},
	{"2301011", "Back Pain (Non-Traumatic)"},
	{"2301013", "Breathing Problem"},
	{"2301015", "Burns/Explosion"},
	{"2301017", "Carbon Monoxide/Hazmat/Inhalation/CBRN"},
	{"2301019", "Cardiac Arrest/Death"},
	{"2301021", "Chest Pain (Non-Traumatic)"},
	{"2301023", "Choking"},
	{"2301025", "Convulsions/Seizure"},
	{"2301027", "Diabetic Problem"},
	{"2301029", "Electrocution/Lightning"},
	{"2301031", "Eye Problem/Injury"},
	{"2301033", "Falls"},
	{"2301035", "Fire"},
	{"2301037", "Headache"},
	{"2301039", "Healthcare Professional/Admission"},
	{"2301041", "Heart Problems/AICD"},
	{"2301043", "Heat/Cold Exposure"},
	{"2301045", "Hemorrhage/Laceration"},
	{"2301047", "Industrial Accident/Inaccessible Incident/Other Entrapments (Non-Vehicle)"},
	{"2301049", "Medical Alarm"},
	{"2301051", "No Other Appropriate Choice"},
	{"2301053", "Overdose/Poisoning/Ingestion"},
	{"2301055", "Pandemic/Epidemic/Outbreak"},
	{"2301057", "Pregnancy/Childbirth/Miscarriage"},
	{"2301059", "Psychiatric Problem/Abnormal Behavior/Suicide Attempt"},
	{"2301061", "Sick Person"},
	{"2301063", "Stab/Gunshot Wound/Penetrating Trauma"},
	{"2301065", "Standby"},
	{"2301067", "Stroke/CVA"},
	{"2301069", "Traffic/Transportation Incident"},
	{"2301071", "Transfer/Interfacility/Palliative Care"},
	{"2301073", "Traumatic Injury"},
	{"2301075", "Well Person Check"},
	{"2301077", "Unconscious/Fainting/Near-Fainting"},
	{"2301079", "Unknown Problem/Person Down"},
	{"2301081", "Drowning/Diving/SCUBA Accident"},
	{"2301083", "Airmedical Transport"},
	{"2301085", "Altered Mental Status"},
	{"2301087", "Intercept"},
	{"2301089", "Nausea"},
	{"2301091", "Vomiting"},
	{"2301093", "Hanging/Strangulation/Asphyxiation"},
	{"2301095", "Intoxicated Subject"},
	{"2301097", "EMS Requested by Law Enforcement"},
	{"2301099", "Active Shooter"},
}

// ReasonCodes lists the eDispatch.01 dispatch reason codes, for an administrator to map CAD call types.
func ReasonCodes() []Code {
	out := make([]Code, len(dispatchReasons))
	copy(out, dispatchReasons)
	return out
}

// SuggestReason makes a best-effort guess at an eDispatch.01 code from a CAD call type's code or name.
// It returns "" when it is not confident, so an administrator can map the rest by hand.
func SuggestReason(callTypeCode, callTypeName string) string {
	t := strings.ToUpper(callTypeCode + " " + callTypeName)
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(t, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("CARDIAC ARREST", "CARD ARREST", "FULL ARREST", "CPR"):
		return "2301019" // Cardiac Arrest/Death
	case has("CHEST"):
		return "2301021" // Chest Pain (Non-Traumatic)
	case has("CARD", "HEART", "AICD"):
		return "2301041" // Heart Problems/AICD
	case has("BRTH", "BREATH", "RESP", "SOB", "DIFF BREATH"):
		return "2301013" // Breathing Problem
	case has("STROKE", "CVA"):
		return "2301067" // Stroke/CVA
	case has("SEIZ", "CONVULS"):
		return "2301025" // Convulsions/Seizure
	case has("FALL"):
		return "2301033" // Falls
	case has("MVC", "MVA", "TRAFFIC", "VEHICLE", "COLLISION", "CRASH"):
		return "2301069" // Traffic/Transportation Incident
	case has("OD", "OVERDOSE", "POISON", "INGEST"):
		return "2301053" // Overdose/Poisoning/Ingestion
	case has("UNC", "UNCONS", "FAINT", "SYNCOPE"):
		return "2301077" // Unconscious/Fainting/Near-Fainting
	case has("ASSLT", "ASSAULT"):
		return "2301007" // Assault
	case has("BURN"):
		return "2301015" // Burns/Explosion
	case has("DIAB", "DIABET"):
		return "2301027" // Diabetic Problem
	case has("ALLRG", "ALLERG", "STING"):
		return "2301003" // Allergic Reaction/Stings
	case has("PSYCH", "SUICID", "BEHAV"):
		return "2301059" // Psychiatric Problem/Abnormal Behavior/Suicide Attempt
	case has("OB", "CHILDBIRTH", "PREGNAN", "LABOR", "MISCARR"):
		return "2301057" // Pregnancy/Childbirth/Miscarriage
	case has("TRAUMA", "TRAUMATIC"):
		return "2301073" // Traumatic Injury
	case has("FIRE", "STANDBY"):
		return "2301065" // Standby (fire standby)
	case has("STAB", "GSW", "GUNSHOT"):
		return "2301063" // Stab/Gunshot Wound/Penetrating Trauma
	case has("SICK", "ILL"):
		return "2301061" // Sick Person
	case has("ALT MENT", "ALTERED"):
		return "2301085" // Altered Mental Status
	default:
		return ""
	}
}

// --- XML shapes -----------------------------------------------------------------------------------
//
// These structs reproduce the NEMSIS element names and their schema order. Fields are laid out in the
// exact sequence the XSD requires; encoding/xml writes them in struct order.

type emsDataSet struct {
	XMLName        xml.Name `xml:"http://www.nemsis.org EMSDataSet"`
	Xsi            string   `xml:"xmlns:xsi,attr"`
	SchemaLocation string   `xml:"xsi:schemaLocation,attr"`
	Header         header   `xml:"Header"`
}

type header struct {
	Demographic demographicGroup `xml:"DemographicGroup"`
	PCR         pcr              `xml:"PatientCareReport"`
}

type demographicGroup struct {
	Agency01 string `xml:"dAgency.01"`
	Agency02 string `xml:"dAgency.02"`
	Agency04 string `xml:"dAgency.04"`
}

type pcr struct {
	UUID     string    `xml:"UUID,attr"`
	Record   eRecord   `xml:"eRecord"`
	Response eResponse `xml:"eResponse"`
	Dispatch eDispatch `xml:"eDispatch"`
	Times    eTimes    `xml:"eTimes"`
	Scene    eScene    `xml:"eScene"`
}

type eRecord struct {
	Record01 string      `xml:"eRecord.01"`
	Software softwareGrp `xml:"eRecord.SoftwareApplicationGroup"`
}

type softwareGrp struct {
	Creator string `xml:"eRecord.02"`
	Name    string `xml:"eRecord.03"`
	Version string `xml:"eRecord.04"`
}

type eResponse struct {
	Agency   agencyGrp  `xml:"eResponse.AgencyGroup"`
	Incident value      `xml:"eResponse.03"`
	Number   value      `xml:"eResponse.04"`
	Service  serviceGrp `xml:"eResponse.ServiceGroup"`
	Cap07    string     `xml:"eResponse.07"`
	// eResponse.08-12 (dispatch/response/scene/transport/turn-around delays) are required and
	// nillable; CAD does not track them, so they go out as Not Applicable nils.
	Delay08  value  `xml:"eResponse.08"`
	Delay09  value  `xml:"eResponse.09"`
	Delay10  value  `xml:"eResponse.10"`
	Delay11  value  `xml:"eResponse.11"`
	Delay12  value  `xml:"eResponse.12"`
	Vehicle  string `xml:"eResponse.13"`
	CallSign string `xml:"eResponse.14"`
	Mode23   string `xml:"eResponse.23"`
	// eResponse.24 (additional response mode descriptors) is required and nillable; CAD has none.
	Mode24 value `xml:"eResponse.24"`
}

type agencyGrp struct {
	Agency01 string `xml:"eResponse.01"`
}

type serviceGrp struct {
	Type05 string `xml:"eResponse.05"`
}

type eDispatch struct {
	Reason01 string `xml:"eDispatch.01"`
	EMD02    value  `xml:"eDispatch.02"`
}

type eTimes struct {
	PSAP01     value  `xml:"eTimes.01"`           // nillable
	Dispatch02 string `xml:"eTimes.02,omitempty"` // not nillable, optional: omit when unknown
	Notified03 string `xml:"eTimes.03,omitempty"` // not nillable, required: omit when unknown (Validate flags it)
	EnRoute05  value  `xml:"eTimes.05"`           // nillable
	OnScene06  value  `xml:"eTimes.06"`           // nillable
	// eTimes.07 (arrived at patient) is required and nillable; CAD does not track it.
	AtPatient07 value `xml:"eTimes.07"`
	LeftScene09 value `xml:"eTimes.09"` // nillable
	AtDest11    value `xml:"eTimes.11"` // nillable
	// eTimes.12 (destination patient transfer of care) is required and nillable; CAD does not track it.
	Transfer12  value  `xml:"eTimes.12"`
	BackInSvc13 string `xml:"eTimes.13,omitempty"` // not nillable, required: omit when unknown
}

type eScene struct {
	LocationType09 *value `xml:"eScene.09,omitempty"`
	GPS11          string `xml:"eScene.11,omitempty"`
	Street15       string `xml:"eScene.15,omitempty"`
	City17         string `xml:"eScene.17,omitempty"`
	State18        string `xml:"eScene.18,omitempty"`
	ZIP19          string `xml:"eScene.19,omitempty"`
	County21       string `xml:"eScene.21,omitempty"`
}

// value is an element that is either a real value or a NEMSIS nil with an NV attribute. It writes
// itself so a nil renders as xsi:nil="true" NV="..." the way NEMSIS defines it, rather than with an
// encoder-generated namespace prefix.
type value struct {
	nilled bool
	nv     string
	text   string
}

// real makes an element carrying a value.
func real(v string) value { return value{text: v} }

// nv makes a NEMSIS nil element with the given null value (defaulting to Not Recorded).
func nv(code string) value {
	if code == "" {
		code = NVNotRecorded
	}
	return value{nilled: true, nv: code}
}

// MarshalXML writes the element as either a plain value or a nil-with-NV element.
func (v value) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if v.nilled {
		start.Attr = []xml.Attr{
			{Name: xml.Name{Local: "xsi:nil"}, Value: "true"},
			{Name: xml.Name{Local: "NV"}, Value: v.nv},
		}
		if err := e.EncodeToken(start); err != nil {
			return err
		}
		return e.EncodeToken(start.End())
	}
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	if v.text != "" {
		if err := e.EncodeToken(xml.CharData(v.text)); err != nil {
			return err
		}
	}
	return e.EncodeToken(start.End())
}

// timeOrNV is a real dateTime when t is set, otherwise a nil with the given NV code.
func timeOrNV(t time.Time, code string) value {
	if t.IsZero() {
		return nv(code)
	}
	return real(t.Format(timeFormat))
}

// timeOrEmpty formats a dateTime, or "" when unset, for elements the schema does not mark nillable
// (so they are omitted rather than sent as a nil).
func timeOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(timeFormat)
}

// build assembles the document from the config and response.
func build(c Config, r Response, version string) emsDataSet {
	doc := emsDataSet{
		Xsi:            xsiNamespace,
		SchemaLocation: Namespace + " EMSDataSet_v" + version + ".xsd",
	}
	doc.Header.Demographic = demographicGroup{
		Agency01: c.AgencyStateID,
		Agency02: c.Agency,
		Agency04: ansiState(c.State),
	}

	p := &doc.Header.PCR
	p.UUID = responseUUID(r)

	// eRecord: the PCR number is the incident number and unit; the software group names this software.
	p.Record = eRecord{
		Record01: pcrNumber(r),
		Software: softwareGrp{Creator: "BioDream LLC", Name: "bitdispatcher", Version: version},
	}

	// eResponse. The delay elements (08-12) are required and nillable but unknown to CAD.
	p.Response = eResponse{
		Agency:   agencyGrp{Agency01: c.Agency},
		Incident: incidentValue(r.Incident),
		Number:   nv(NVNotRecorded),             // EMS response number: CAD does not assign one
		Service:  serviceGrp{Type05: "2205001"}, // Emergency Response (Primary Response Area)
		Cap07:    capability(r.Capability),
		Delay08:  nv(NVNotApplicable),
		Delay09:  nv(NVNotApplicable),
		Delay10:  nv(NVNotApplicable),
		Delay11:  nv(NVNotApplicable),
		Delay12:  nv(NVNotApplicable),
		Vehicle:  valueOr(r.VehicleNumber, r.Unit),
		CallSign: valueOr(r.CallSign, r.Unit),
		Mode23:   responseMode(r.LightsSirens),
		Mode24:   nv(NVNotApplicable),
	}

	// eDispatch.
	p.Dispatch = eDispatch{
		Reason01: reasonCode(r),
		EMD02:    emdValue(r.EMD),
	}

	// eTimes. The nillable times (01, 05, 06, 09, 11) go out as a value or a NEMSIS nil; the ones the
	// schema does not mark nillable (02, 03, 13) are plain dateTimes that we simply omit when unknown.
	p.Times = eTimes{
		PSAP01:      timeOrNV(r.PSAPCall, NVNotRecorded),
		Dispatch02:  timeOrEmpty(r.DispatchNotified),
		Notified03:  timeOrEmpty(r.UnitNotified),
		EnRoute05:   timeOrNV(r.EnRoute, NVNotRecorded),
		OnScene06:   timeOrNV(r.OnScene, NVNotRecorded),
		AtPatient07: nv(NVNotRecorded),
		LeftScene09: timeOrNV(r.LeftScene, NVNotRecorded),
		AtDest11:    timeOrNV(r.AtDestination, NVNotRecorded),
		Transfer12:  nv(NVNotRecorded),
		BackInSvc13: timeOrEmpty(r.BackInService),
	}

	// eScene. Only the parts CAD knows; the schema makes these optional (minOccurs=0) except state/zip,
	// which are required but may be nil when unknown.
	p.Scene = eScene{
		Street15: strings.TrimSpace(r.Address),
		City17:   strings.TrimSpace(r.City),
		ZIP19:    strings.TrimSpace(r.ZIP),
		County21: strings.TrimSpace(r.County),
	}
	if st := ansiState(r.StateCode); st != "" {
		p.Scene.State18 = st
	}
	if r.Lat != nil && r.Lon != nil {
		p.Scene.GPS11 = fmt.Sprintf("%.6f,%.6f", *r.Lat, *r.Lon)
	}

	return doc
}

// incidentValue is eResponse.03: a real value, or a nil when the incident number is missing.
func incidentValue(incident string) value {
	if strings.TrimSpace(incident) == "" {
		return nv(NVNotRecorded)
	}
	return real(incident)
}

// emdValue is eDispatch.02 from "yes"/"no"; anything else becomes a nil (Not Recorded).
func emdValue(emd string) value {
	switch strings.ToLower(strings.TrimSpace(emd)) {
	case "yes":
		return real("2302007") // Yes, Unknown if Pre-Arrival Instructions Given
	case "no":
		return real("2302001") // No
	default:
		return nv(NVNotRecorded)
	}
}

// reasonCode is eDispatch.01: the mapped code if valid, else guessed from the call type, else a
// no-other-choice code so the required element always carries a valid value.
func reasonCode(r Response) string {
	if validReason(r.ReasonCode) {
		return r.ReasonCode
	}
	if s := SuggestReason(r.ReasonCode, r.Reason); s != "" {
		return s
	}
	return "2301051" // No Other Appropriate Choice
}

func validReason(code string) bool {
	for _, c := range dispatchReasons {
		if c.Code == code {
			return true
		}
	}
	return false
}

// capability maps a loose capability word to an eResponse.07 code.
func capability(cap string) string {
	switch strings.ToLower(strings.TrimSpace(cap)) {
	case "als":
		return "2207015" // Ground Transport (ALS Equipped)
	case "bls":
		return "2207017" // Ground Transport (BLS Equipped)
	case "cct", "critical care", "criticalcare":
		return "2207019" // Ground Transport (Critical Care Equipped)
	case "air", "helicopter", "rotor":
		return "2207011" // Air Transport-Helicopter
	default:
		return "2207017" // default to BLS-equipped ground transport
	}
}

// responseMode maps the lights-and-sirens flag to an eResponse.23 code, defaulting to emergent.
func responseMode(ls *bool) string {
	if ls != nil && !*ls {
		return "2223005" // Non-Emergent
	}
	return "2223001" // Emergent (Immediate Response)
}

func valueOr(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return strings.TrimSpace(primary)
	}
	return strings.TrimSpace(fallback)
}

// pcrNumber is eRecord.01: the incident number and the unit, so it is unique per unit on a call. The
// schema requires at least three characters, so short inputs are padded.
func pcrNumber(r Response) string {
	unit := valueOr(r.Unit, r.CallSign)
	n := strings.TrimSpace(r.Incident)
	if unit != "" {
		n = strings.TrimSpace(n + "-" + unit)
	}
	n = strings.Trim(n, "-")
	if n == "" {
		n = "unknown"
	}
	for len(n) < 3 {
		n += "0"
	}
	if len(n) > 50 {
		n = n[:50]
	}
	return n
}

// responseUUID returns the given UUID when it is set, otherwise derives a stable RFC 4122 v5-shaped
// UUID from the incident and unit so the same response always carries the same UUID.
func responseUUID(r Response) string {
	if u := strings.TrimSpace(r.UUID); u != "" {
		return u
	}
	seed := strings.TrimSpace(r.Incident) + "\x00" + valueOr(r.Unit, r.CallSign)
	sum := sha1.Sum([]byte("bitdispatcher-nemsis\x00" + seed))
	var b [16]byte
	copy(b[:], sum[:16])
	b[6] = (b[6] & 0x0f) | 0x50 // version 5
	b[8] = (b[8] & 0x3f) | 0x80 // variant RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
