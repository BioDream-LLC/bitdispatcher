package nemsis

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// est is the US Eastern offset used in the time tests.
var est = time.FixedZone("EST", -5*3600)

func fullResponse() Response {
	ls := true
	lat, lon := 37.5407, -77.4360
	return Response{
		Incident:         "2026-000123",
		Unit:             "M7",
		CallSign:         "Medic 7",
		VehicleNumber:    "307",
		Capability:       "als",
		Reason:           "chest pain",
		ReasonCode:       "2301021",
		EMD:              "yes",
		LightsSirens:     &ls,
		PSAPCall:         time.Date(2026, 10, 23, 14, 0, 0, 0, est),
		DispatchNotified: time.Date(2026, 10, 23, 14, 1, 0, 0, est),
		UnitNotified:     time.Date(2026, 10, 23, 14, 2, 0, 0, est),
		EnRoute:          time.Date(2026, 10, 23, 14, 3, 0, 0, est),
		OnScene:          time.Date(2026, 10, 23, 14, 9, 0, 0, est),
		LeftScene:        time.Date(2026, 10, 23, 14, 25, 0, 0, est),
		AtDestination:    time.Date(2026, 10, 23, 14, 40, 0, 0, est),
		BackInService:    time.Date(2026, 10, 23, 15, 0, 0, 0, est),
		Address:          "100 Main St",
		City:             "Richmond",
		StateCode:        "VA",
		ZIP:              "23219",
		County:           "Richmond City",
		Lat:              &lat,
		Lon:              &lon,
	}
}

func fullConfig() Config {
	return Config{On: true, URL: "https://epcr.example.org/import", Token: "secret-token", Agency: "VA0001", AgencyStateID: "AGENCY-123", State: "VA"}
}

// parsed mirrors the document for reading values back with encoding/xml.
type parsed struct {
	XMLName xml.Name `xml:"EMSDataSet"`
	Header  struct {
		Demographic struct {
			A01 string `xml:"dAgency.01"`
			A02 string `xml:"dAgency.02"`
			A04 string `xml:"dAgency.04"`
		} `xml:"DemographicGroup"`
		PCR struct {
			UUID   string `xml:"UUID,attr"`
			Record struct {
				Num  string `xml:"eRecord.01"`
				Soft struct {
					Creator string `xml:"eRecord.02"`
					Name    string `xml:"eRecord.03"`
					Version string `xml:"eRecord.04"`
				} `xml:"eRecord.SoftwareApplicationGroup"`
			} `xml:"eRecord"`
			Response struct {
				Agency struct {
					A01 string `xml:"eResponse.01"`
				} `xml:"eResponse.AgencyGroup"`
				Incident nilField `xml:"eResponse.03"`
				Number   nilField `xml:"eResponse.04"`
				Service  struct {
					Type string `xml:"eResponse.05"`
				} `xml:"eResponse.ServiceGroup"`
				Cap      string `xml:"eResponse.07"`
				Vehicle  string `xml:"eResponse.13"`
				CallSign string `xml:"eResponse.14"`
				Mode     string `xml:"eResponse.23"`
			} `xml:"eResponse"`
			Dispatch struct {
				Reason string   `xml:"eDispatch.01"`
				EMD    nilField `xml:"eDispatch.02"`
			} `xml:"eDispatch"`
			Times struct {
				PSAP     nilField `xml:"eTimes.01"`
				Dispatch string   `xml:"eTimes.02"`
				Notified string   `xml:"eTimes.03"`
				EnRoute  nilField `xml:"eTimes.05"`
				OnScene  nilField `xml:"eTimes.06"`
				Left     nilField `xml:"eTimes.09"`
				AtDest   nilField `xml:"eTimes.11"`
				Back     string   `xml:"eTimes.13"`
			} `xml:"eTimes"`
			Scene struct {
				GPS    string `xml:"eScene.11"`
				Street string `xml:"eScene.15"`
				City   string `xml:"eScene.17"`
				State  string `xml:"eScene.18"`
				ZIP    string `xml:"eScene.19"`
				County string `xml:"eScene.21"`
			} `xml:"eScene"`
		} `xml:"PatientCareReport"`
	} `xml:"Header"`
}

type nilField struct {
	Nil  string `xml:"nil,attr"`
	NV   string `xml:"NV,attr"`
	Text string `xml:",chardata"`
}

func mustEncode(t *testing.T, c Config, r Response) ([]byte, parsed) {
	t.Helper()
	doc, err := Encode(c, r, Version)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	var p parsed
	if err := xml.Unmarshal(doc, &p); err != nil {
		t.Fatalf("parse back: %v\n%s", err, doc)
	}
	return doc, p
}

func TestEncodeHeaderAndValues(t *testing.T) {
	doc, p := mustEncode(t, fullConfig(), fullResponse())

	if !strings.HasPrefix(string(doc), "<?xml ") {
		t.Errorf("document should start with an XML declaration, got %q", string(doc)[:20])
	}
	if !strings.Contains(string(doc), `xmlns="http://www.nemsis.org"`) {
		t.Errorf("namespace missing from document")
	}

	if p.Header.Demographic.A01 != "AGENCY-123" {
		t.Errorf("dAgency.01 = %q", p.Header.Demographic.A01)
	}
	if p.Header.Demographic.A02 != "VA0001" {
		t.Errorf("dAgency.02 = %q", p.Header.Demographic.A02)
	}
	if p.Header.Demographic.A04 != "51" {
		t.Errorf("dAgency.04 (ANSI VA) = %q, want 51", p.Header.Demographic.A04)
	}

	pcr := p.Header.PCR
	if pcr.Record.Num != "2026-000123-M7" {
		t.Errorf("eRecord.01 = %q", pcr.Record.Num)
	}
	if pcr.Record.Soft.Creator != "BioDream LLC" || pcr.Record.Soft.Name != "bitdispatcher" {
		t.Errorf("software group wrong: %+v", pcr.Record.Soft)
	}
	if pcr.Record.Soft.Version != Version {
		t.Errorf("eRecord.04 = %q, want %q", pcr.Record.Soft.Version, Version)
	}
	if pcr.Response.Agency.A01 != "VA0001" {
		t.Errorf("eResponse.01 = %q", pcr.Response.Agency.A01)
	}
	if pcr.Response.Incident.Text != "2026-000123" {
		t.Errorf("eResponse.03 = %q", pcr.Response.Incident.Text)
	}
	if pcr.Response.Service.Type != "2205001" {
		t.Errorf("eResponse.05 = %q, want 2205001 (Emergency Response)", pcr.Response.Service.Type)
	}
	if pcr.Response.Cap != "2207015" {
		t.Errorf("eResponse.07 (ALS) = %q, want 2207015", pcr.Response.Cap)
	}
	if pcr.Response.Vehicle != "307" {
		t.Errorf("eResponse.13 = %q", pcr.Response.Vehicle)
	}
	if pcr.Response.CallSign != "Medic 7" {
		t.Errorf("eResponse.14 = %q", pcr.Response.CallSign)
	}
	if pcr.Response.Mode != "2223001" {
		t.Errorf("eResponse.23 (lights/sirens) = %q, want 2223001 Emergent", pcr.Response.Mode)
	}
	if pcr.Dispatch.Reason != "2301021" {
		t.Errorf("eDispatch.01 = %q, want mapped 2301021", pcr.Dispatch.Reason)
	}
	if pcr.Dispatch.EMD.Text != "2302007" {
		t.Errorf("eDispatch.02 (EMD yes) = %q", pcr.Dispatch.EMD.Text)
	}
	if pcr.Scene.GPS != "37.540700,-77.436000" {
		t.Errorf("eScene.11 GPS = %q", pcr.Scene.GPS)
	}
	if pcr.Scene.Street != "100 Main St" || pcr.Scene.City != "Richmond" || pcr.Scene.State != "51" || pcr.Scene.ZIP != "23219" || pcr.Scene.County != "Richmond City" {
		t.Errorf("eScene address wrong: %+v", pcr.Scene)
	}
}

func TestEncodeElementOrder(t *testing.T) {
	doc, _ := mustEncode(t, fullConfig(), fullResponse())
	s := string(doc)
	order := []string{
		"<dAgency.01>", "<dAgency.02>", "<dAgency.04>",
		"<eRecord>", "<eRecord.01>", "<eRecord.SoftwareApplicationGroup>", "<eRecord.02>", "<eRecord.03>", "<eRecord.04>",
		"<eResponse>", "<eResponse.AgencyGroup>", "<eResponse.01>", "<eResponse.03>", "<eResponse.04",
		"<eResponse.ServiceGroup>", "<eResponse.05>", "<eResponse.07>",
		"<eResponse.08", "<eResponse.09", "<eResponse.10", "<eResponse.11", "<eResponse.12",
		"<eResponse.13>", "<eResponse.14>", "<eResponse.23>", "<eResponse.24",
		"<eDispatch>", "<eDispatch.01>", "<eDispatch.02",
		"<eTimes>", "<eTimes.01>", "<eTimes.02", "<eTimes.03>", "<eTimes.05", "<eTimes.06", "<eTimes.07", "<eTimes.09", "<eTimes.11", "<eTimes.12", "<eTimes.13",
		"<eScene>", "<eScene.11>", "<eScene.15>", "<eScene.17>", "<eScene.18>", "<eScene.19>", "<eScene.21>",
	}
	prev := -1
	for _, tag := range order {
		i := strings.Index(s, tag)
		if i < 0 {
			t.Fatalf("tag %q not found in document", tag)
		}
		if i < prev {
			t.Errorf("tag %q is out of schema order (at %d, previous at %d)", tag, i, prev)
		}
		prev = i
	}
}

func TestEncodeRequiredDelayNils(t *testing.T) {
	doc, _ := mustEncode(t, fullConfig(), fullResponse())
	s := string(doc)
	for _, tag := range []string{"eResponse.08", "eResponse.09", "eResponse.10", "eResponse.11", "eResponse.12"} {
		if !strings.Contains(s, `<`+tag+` xsi:nil="true" NV="`+NVNotApplicable+`">`) {
			t.Errorf("%s should be a Not Applicable nil", tag)
		}
	}
}

func TestEncodeOmitsUnsetNonNillableTimes(t *testing.T) {
	r := fullResponse()
	r.DispatchNotified = time.Time{}
	r.BackInService = time.Time{}
	r.UnitNotified = time.Time{}
	doc, p := mustEncode(t, fullConfig(), r)
	s := string(doc)
	// These three are not nillable in the schema, so an unknown value must be omitted, not sent as nil.
	if strings.Contains(s, "<eTimes.02") {
		t.Errorf("unset eTimes.02 should be omitted")
	}
	if strings.Contains(s, "<eTimes.03") {
		t.Errorf("unset eTimes.03 should be omitted")
	}
	if strings.Contains(s, "<eTimes.13") {
		t.Errorf("unset eTimes.13 should be omitted")
	}
	if p.Header.PCR.Times.Notified != "" || p.Header.PCR.Times.Back != "" {
		t.Errorf("omitted times should parse back empty")
	}
}

func TestEncodeNilTimes(t *testing.T) {
	r := fullResponse()
	r.EnRoute = time.Time{}
	r.OnScene = time.Time{}
	doc, p := mustEncode(t, fullConfig(), r)

	if p.Header.PCR.Times.Notified != "2026-10-23T14:02:00-05:00" {
		t.Errorf("eTimes.03 should keep its value, got %q", p.Header.PCR.Times.Notified)
	}
	if p.Header.PCR.Times.EnRoute.Nil != "true" || p.Header.PCR.Times.EnRoute.NV != NVNotRecorded {
		t.Errorf("missing eTimes.05 should be nil with NV %s, got nil=%q NV=%q", NVNotRecorded, p.Header.PCR.Times.EnRoute.Nil, p.Header.PCR.Times.EnRoute.NV)
	}
	if !strings.Contains(string(doc), `<eTimes.06 xsi:nil="true" NV="7701003">`) {
		t.Errorf("expected xsi:nil form in raw document for eTimes.06")
	}
}

func TestEncodeTimeFormatHasOffset(t *testing.T) {
	_, p := mustEncode(t, fullConfig(), fullResponse())
	got := p.Header.PCR.Times.PSAP.Text
	if _, err := time.Parse(timeFormat, got); err != nil {
		t.Errorf("eTimes.01 %q is not NEMSIS dateTime: %v", got, err)
	}
	if !strings.HasSuffix(got, "-05:00") {
		t.Errorf("eTimes.01 %q should carry the -05:00 offset", got)
	}
}

func TestUUIDStability(t *testing.T) {
	r := fullResponse()
	r.UUID = ""
	_, p1 := mustEncode(t, fullConfig(), r)
	_, p2 := mustEncode(t, fullConfig(), r)
	if p1.Header.PCR.UUID == "" {
		t.Fatal("UUID should be derived when empty")
	}
	if p1.Header.PCR.UUID != p2.Header.PCR.UUID {
		t.Errorf("derived UUID not stable: %q vs %q", p1.Header.PCR.UUID, p2.Header.PCR.UUID)
	}
	// A different unit should change it.
	r2 := r
	r2.Unit = "M8"
	r2.CallSign = ""
	_, p3 := mustEncode(t, fullConfig(), r2)
	if p3.Header.PCR.UUID == p1.Header.PCR.UUID {
		t.Errorf("different unit should give a different UUID")
	}
	// A supplied UUID is used verbatim.
	r.UUID = "12345678-1234-4123-8abc-1234567890ab"
	_, p4 := mustEncode(t, fullConfig(), r)
	if p4.Header.PCR.UUID != r.UUID {
		t.Errorf("supplied UUID not kept: %q", p4.Header.PCR.UUID)
	}
}

func TestUUIDLooksValid(t *testing.T) {
	_, p := mustEncode(t, fullConfig(), Response{Incident: "A1", Unit: "E1"})
	u := p.Header.PCR.UUID
	parts := strings.Split(u, "-")
	if len(parts) != 5 || len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		t.Errorf("UUID shape wrong: %q", u)
	}
	if parts[2][0] != '5' {
		t.Errorf("derived UUID should be version 5, got %q", u)
	}
	if !strings.ContainsRune("89ab", rune(parts[3][0])) {
		t.Errorf("derived UUID variant nibble wrong: %q", u)
	}
}

func TestReasonMappingAndNoOtherChoice(t *testing.T) {
	// Unmapped reason with an unknown call type falls back to No Other Appropriate Choice.
	r := fullResponse()
	r.ReasonCode = ""
	r.Reason = "something unusual"
	_, p := mustEncode(t, fullConfig(), r)
	if p.Header.PCR.Dispatch.Reason != "2301051" {
		t.Errorf("unmapped reason should be 2301051, got %q", p.Header.PCR.Dispatch.Reason)
	}
	// A recognisable name is suggested.
	r.Reason = "cardiac arrest"
	_, p = mustEncode(t, fullConfig(), r)
	if p.Header.PCR.Dispatch.Reason != "2301019" {
		t.Errorf("cardiac arrest should map to 2301019, got %q", p.Header.PCR.Dispatch.Reason)
	}
}

func TestValidate(t *testing.T) {
	var r Response
	msgs := Validate(r)
	joined := strings.Join(msgs, "\n")
	for _, want := range []string{"incident number", "unit", "notified by dispatch", "911 call"} {
		if !strings.Contains(joined, want) {
			t.Errorf("Validate should mention %q; got:\n%s", want, joined)
		}
	}

	// An out-of-order time is flagged.
	r = fullResponse()
	r.EnRoute = r.UnitNotified.Add(-time.Minute)
	msgs = Validate(r)
	if !strings.Contains(strings.Join(msgs, "\n"), "en route before it was notified") {
		t.Errorf("expected en-route-before-notified problem, got %v", msgs)
	}

	// A complete response has no problems.
	if got := Validate(fullResponse()); len(got) != 0 {
		t.Errorf("full response should validate clean, got %v", got)
	}
}

func TestCheck(t *testing.T) {
	cases := []struct {
		name string
		c    Config
		ok   bool
	}{
		{"off ignores everything", Config{On: false, URL: "nonsense"}, true},
		{"good https", fullConfig(), true},
		{"localhost http allowed", Config{On: true, URL: "http://localhost:8080/import", Agency: "A", State: "VA"}, true},
		{"loopback http allowed", Config{On: true, URL: "http://127.0.0.1/import", Agency: "A", State: "VA"}, true},
		{"plain http rejected", Config{On: true, URL: "http://epcr.example.org/import", Agency: "A", State: "VA"}, false},
		{"no agency", Config{On: true, URL: "https://x.example.org", State: "VA"}, false},
		{"unknown state", Config{On: true, URL: "https://x.example.org", Agency: "A", State: "ZZ"}, false},
		{"bad url", Config{On: true, URL: "::::", Agency: "A", State: "VA"}, false},
	}
	for _, tc := range cases {
		err := tc.c.Check()
		if tc.ok && err != nil {
			t.Errorf("%s: unexpected error %v", tc.name, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("%s: expected an error", tc.name)
		}
	}
}

func TestReasonCodes(t *testing.T) {
	codes := ReasonCodes()
	if len(codes) != 50 {
		t.Errorf("expected 50 dispatch reason codes, got %d", len(codes))
	}
	if codes[0].Code != "2301001" || codes[0].Name != "Abdominal Pain/Problems" {
		t.Errorf("first code wrong: %+v", codes[0])
	}
	// The returned slice should be a copy a caller cannot use to mutate the package state.
	codes[0].Name = "mutated"
	if ReasonCodes()[0].Name != "Abdominal Pain/Problems" {
		t.Errorf("ReasonCodes should return a copy")
	}
}

func TestSuggestReason(t *testing.T) {
	cases := map[[2]string]string{
		{"CHEST", "Chest Pain"}:        "2301021",
		{"CARD", "cardiac arrest"}:     "2301019",
		{"CARD", "cardiac"}:            "2301041",
		{"BRTH", "breathing problem"}:  "2301013",
		{"FALL", "fall victim"}:        "2301033",
		{"MVC", "traffic collision"}:   "2301069",
		{"SICK", "sick person"}:        "2301061",
		{"OD", "overdose"}:             "2301053",
		{"STROKE", "cva"}:              "2301067",
		{"SEIZ", "seizure"}:            "2301025",
		{"UNC", "unconscious"}:         "2301077",
		{"ASSLT", "assault"}:           "2301007",
		{"BURN", "burn"}:               "2301015",
		{"DIAB", "diabetic"}:           "2301027",
		{"ALLRG", "allergic reaction"}: "2301003",
		{"PSYCH", "suicide attempt"}:   "2301059",
		{"OB", "childbirth"}:           "2301057",
		{"", "zebra parade"}:           "",
	}
	for in, want := range cases {
		if got := SuggestReason(in[0], in[1]); got != want {
			t.Errorf("SuggestReason(%q,%q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

func TestSendSuccess(t *testing.T) {
	var gotAuth, gotType, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, "accepted")
	}))
	defer srv.Close()

	c := fullConfig()
	c.URL = srv.URL
	doc, _ := Encode(c, fullResponse(), Version)

	cl := &Client{HTTP: srv.Client()}
	status, body, err := cl.Send(context.Background(), c, doc)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if status != http.StatusAccepted {
		t.Errorf("status = %d", status)
	}
	if body != "accepted" {
		t.Errorf("body = %q", body)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("auth header = %q", gotAuth)
	}
	if gotType != "application/xml" {
		t.Errorf("content type = %q", gotType)
	}
	if !strings.Contains(gotBody, "<EMSDataSet") {
		t.Errorf("server did not receive the document")
	}
}

func TestSendError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, strings.Repeat("x", 5000))
	}))
	defer srv.Close()

	c := fullConfig()
	c.URL = srv.URL
	cl := &Client{HTTP: srv.Client()}
	status, body, err := cl.Send(context.Background(), c, []byte("<EMSDataSet/>"))
	if err == nil {
		t.Fatal("expected an error on 400")
	}
	if status != http.StatusBadRequest {
		t.Errorf("status = %d", status)
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should name the status, got %v", err)
	}
	if len(body) != 2000 {
		t.Errorf("body should be trimmed to 2000 chars, got %d", len(body))
	}
}

func TestSendNoToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := fullConfig()
	c.URL = srv.URL
	c.Token = ""
	cl := &Client{HTTP: srv.Client()}
	if _, _, err := cl.Send(context.Background(), c, []byte("<EMSDataSet/>")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("no token should mean no Authorization header, got %q", gotAuth)
	}
}

func TestPCRNumberPadsShort(t *testing.T) {
	_, p := mustEncode(t, fullConfig(), Response{Incident: "7"})
	if len(p.Header.PCR.Record.Num) < 3 {
		t.Errorf("eRecord.01 must be at least 3 chars, got %q", p.Header.PCR.Record.Num)
	}
}

func TestANSIState(t *testing.T) {
	if ansiState("va") != "51" {
		t.Errorf("va -> %q, want 51", ansiState("va"))
	}
	if ansiState("51") != "51" {
		t.Errorf("already-ANSI 51 should pass through, got %q", ansiState("51"))
	}
	if ansiState("ZZ") != "" {
		t.Errorf("unknown should be empty, got %q", ansiState("ZZ"))
	}
	if ansiState("99") != "" {
		t.Errorf("unknown numeric should be empty, got %q", ansiState("99"))
	}
}
