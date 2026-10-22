// Command bvsim stands in for BitVoice's CAD hooks: its event stream, calls in progress, EIDO, rebid, text,
// video link, radio positions and acknowledgements, with BitVoice's payload shapes. It makes 911 calls and
// radio traffic on its own, so bitdispatcher can be shown and tested without a phone system. Everything it
// makes is fictional and marked SIMULATED.
package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"math"
	"math/big"
	mrand "math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

type civic struct {
	Country    string `json:"country,omitempty"`
	State      string `json:"state,omitempty"`
	City       string `json:"city,omitempty"`
	Number     string `json:"number,omitempty"`
	PreDir     string `json:"preDir,omitempty"`
	Street     string `json:"street,omitempty"`
	StreetType string `json:"streetType,omitempty"`
	PostDir    string `json:"postDir,omitempty"`
	PostalCode string `json:"postalCode,omitempty"`
	Floor      string `json:"floor,omitempty"`
	Unit       string `json:"unit,omitempty"`
}

type location struct {
	Civic      *civic    `json:"civic,omitempty"`
	Lat        *float64  `json:"lat,omitempty"`
	Lon        *float64  `json:"lon,omitempty"`
	Radius     float64   `json:"radius,omitempty"`
	Confidence int       `json:"confidence,omitempty"`
	Method     string    `json:"method,omitempty"`
	Time       time.Time `json:"time,omitempty"`
	Altitude   *float64  `json:"altitude,omitempty"`
	VRadius    float64   `json:"vRadius,omitempty"`
}

type textLine struct {
	At   time.Time `json:"at"`
	From string    `json:"from"`
	Name string    `json:"name,omitempty"`
	Via  string    `json:"via"`
	Text string    `json:"text"`
}

type emergency struct {
	Location    *location      `json:"location,omitempty"`
	LocationURI string         `json:"locationUri,omitempty"`
	Additional  map[string]any `json:"additional"`
	ESInet      bool           `json:"esinet"`
	TakenBy     string         `json:"takenBy,omitempty"`
	TakenName   string         `json:"takenName,omitempty"`
	Text        []textLine     `json:"text,omitempty"`
	TextOnly    bool           `json:"textOnly,omitempty"`
	TextVia     []string       `json:"textVia,omitempty"`
	LocCheck    map[string]any `json:"locationCheck,omitempty"`
	Video       *struct {
		ViewToken string `json:"viewToken,omitempty"`
		Live      bool   `json:"live"`
		Source    string `json:"source,omitempty"`
	} `json:"video,omitempty"`
}

type call struct {
	ID        string            `json:"id"`
	State     string            `json:"state"`
	Direction string            `json:"direction"`
	From      string            `json:"from"`
	FromName  string            `json:"fromName"`
	To        string            `json:"to"`
	Start     time.Time         `json:"start"`
	Answer    time.Time         `json:"answer,omitzero"`
	Screen    map[string]string `json:"screen,omitempty"`
	Emergency *emergency        `json:"emergency,omitempty"`
}

type radioUnit struct {
	ID       int
	Alias    string
	Lat, Lon float64
	Heading  float64
}

type sim struct {
	mu     sync.Mutex
	emUnit string // the radio emergency the channel holds until acknowledged, as BitVoice keeps one per channel
	calls  map[string]*call
	subs   map[chan string]struct{}
	seq    uint64
	units  []*radioUnit
	token  string
	agency string
	base   string
	push   *pusher
	n      int
	// What callers say: the story of each call, and the address they give.
	story      map[string]story
	addr       map[string]string
	droneToken string
	// Callers' photos, as BitVoice keeps them per call.
	media map[string][]photo
	held  map[string]bool
}

// photo is a simulated caller's photo: a small picture, its hash and where the "camera" was.
type photo struct {
	n          int
	data       []byte
	sha        string
	at         time.Time
	lat, lon   float64
	takenLocal string
}

// simPhoto draws a small picture marked SIMULATED (a striped JPEG), as a caller's photo.
func simPhoto() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 160, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 160; x++ {
			c := color.RGBA{0x33, 0x44, 0x55, 0xff}
			if (x+y)/12%2 == 0 {
				c = color.RGBA{0xe0, 0x8a, 0x2c, 0xff}
			}
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	jpeg.Encode(&b, img, &jpeg.Options{Quality: 70})
	return b.Bytes()
}

type pusher struct {
	eidoURL, eidoToken, radioURL, radioSecret, ali string
	hookURL, hookToken                             string // bitdispatcher's hooks: crash data, drones
	client                                         *http.Client
}

func (s *sim) publish(typ string, data any) {
	s.mu.Lock()
	s.seq++
	b, _ := json.Marshal(map[string]any{"seq": s.seq, "at": time.Now().UTC(), "type": typ, "data": data})
	frame := "event: " + typ + "\ndata: " + string(b) + "\n\n"
	for ch := range s.subs {
		select {
		case ch <- frame:
		default:
		}
	}
	s.mu.Unlock()
	if s.push != nil && strings.HasPrefix(typ, "radio.") && s.push.radioURL != "" {
		go s.push.post(s.push.radioURL, s.push.radioSecret, "application/json", mustJSON(map[string]any{"type": typ, "agency": s.agency, "data": data}))
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func (p *pusher) post(url, token, ct string, body []byte) {
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", ct)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		log.Printf("push %s: %v", url, err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		log.Printf("push %s: %s", url, resp.Status)
	}
}

func (s *sim) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+s.token {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(401)
			w.Write([]byte(`{"message":"sign in or send an API token"}`))
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// streets are real addresses in and around Roanoke, Virginia (the development map), so a simulated
// caller's location matches the recorded map. The calls themselves are fictional.
var streets = []struct {
	num, pre, name, typ, post, city, zip string
	lat, lon                             float64
}{
	{"18", "", "Kirk", "Ave", "SW", "Roanoke", "24011", 37.27109, -79.94141},
	{"526", "", "Orange", "Ave", "NE", "Roanoke", "24016", 37.28271, -79.93232},
	{"2001", "", "Crystal Spring", "Ave", "SW", "Roanoke", "24014", 37.25027, -79.94605},
	{"2038", "", "Brandon", "Ave", "SW", "Roanoke", "24015", 37.25827, -79.98500},
	{"1519", "", "Rosewalk", "Ln", "SE", "Roanoke", "24014", 37.24368, -79.91013},
	{"4201", "", "Williamson", "Rd", "NW", "Roanoke", "24012", 37.31123, -79.94980},
	{"211", "E", "Main", "St", "", "Salem", "24153", 37.29356, -80.05488},
	{"106", "E", "Lee", "Ave", "", "Vinton", "24179", 37.28009, -79.89728},
	{"4333", "", "Old Cave Spring", "Rd", "", "Roanoke", "24018", 37.22889, -80.01241},
	{"6415", "", "Peters Creek", "Rd", "NW", "Roanoke", "24019", 37.34396, -79.96281},
	{"1916", "", "Orange", "Ave", "NW", "Roanoke", "24017", 37.28535, -79.96835},
	{"3620", "", "Franklin", "Rd", "SW", "Roanoke", "24014", 37.23505, -79.96982},
}

var names = []string{"Alex Example", "Sam Sample", "Jordan Demo", "Casey Test", "Riley Placeholder", "Morgan Fiction"}

func (s *sim) newCall() {
	s.mu.Lock()
	s.n++
	id := fmt.Sprintf("sim%s-%04d", runID, s.n)
	st := streets[mrand.IntN(len(streets))]
	kind := []string{"wireless", "wireless", "wireless", "pots", "voip"}[mrand.IntN(5)]
	from := fmt.Sprintf("+1540555%04d", 100+mrand.IntN(9800))
	c := &call{ID: id, State: "ringing", Direction: "in", From: from, To: "911", Start: time.Now().UTC(), Emergency: &emergency{ESInet: true,
		Additional: map[string]any{"providers": []any{map[string]any{"name": "Simulated Carrier"}}, "service": map[string]any{"types": []string{kind}}}}}
	if mrand.IntN(3) == 0 {
		c.Emergency.Additional["subscriber"] = map[string]any{"name": names[mrand.IntN(len(names))] + " (SIMULATED)"}
	}
	lat := st.lat + (mrand.Float64()-0.5)*0.002
	lon := st.lon + (mrand.Float64()-0.5)*0.002
	sto := stories[mrand.IntN(len(stories))]
	s.story[id] = sto
	s.addr[id] = strings.Join(strings.Fields(st.num+" "+st.pre+" "+st.name+" "+st.typ+" "+st.post), " ")
	if sto.line != "" {
		c.To = sto.line
	}
	switch kind {
	case "wireless":
		r := 8 + mrand.Float64()*120
		c.Emergency.Location = &location{Lat: &lat, Lon: &lon, Radius: math.Round(r), Confidence: 90, Method: "GPS", Time: time.Now().UTC()}
		if mrand.IntN(2) == 0 {
			// A device-based hybrid fix: the phone's own position, a few metres, in 3-D.
			alt := 290 + float64(mrand.IntN(6))*3.2
			c.Emergency.Location.Radius, c.Emergency.Location.Method = math.Round(3+mrand.Float64()*12), "DBH"
			c.Emergency.Location.Altitude, c.Emergency.Location.VRadius = &alt, 3
		}
		c.Emergency.LocationURI = "https://lis.sim.invalid/loc/" + id
	default:
		la, lo := st.lat, st.lon
		c.Emergency.Location = &location{Civic: &civic{Country: "US", State: "VA", City: st.city, Number: st.num, PreDir: st.pre, Street: st.name, StreetType: st.typ, PostDir: st.post, PostalCode: st.zip},
			Lat: &la, Lon: &lo, Method: "Manual", Time: time.Now().UTC()}
	}
	// The caller ID's STIR/SHAKEN check, as the carrier reported it: most pass, a few fail.
	switch mrand.IntN(10) {
	case 0:
		c.Screen = map[string]string{"verified": "failed", "attest": "C", "why": "SIMULATED: the caller ID's signature did not verify"}
	case 1, 2:
	default:
		c.Screen = map[string]string{"verified": "passed", "attest": "A", "why": "SIMULATED: the provider verified the caller ID"}
	}
	if mrand.IntN(8) == 0 {
		c.Emergency.TextOnly = true
		c.Emergency.TextVia = []string{"msrp"}
		c.Emergency.Text = []textLine{{At: time.Now().UTC(), From: "caller", Via: "msrp", Text: "SIMULATED: I can't talk, someone is in my house"}}
	}
	s.calls[id] = c
	s.mu.Unlock()
	log.Printf("call %s from %s ringing (%s, %s)", id, from, kind, sto.name)
	s.publish("call.started", c)
	go s.lifecycle(id)
	// Others call about the same thing, from near it.
	for i, words := range sto.dups {
		go func(i int, words string) {
			time.Sleep(time.Duration(8+i*9+mrand.IntN(6)) * time.Second)
			s.dupCall(st.lat, st.lon, words)
		}(i, words)
	}
}

// dupCall is another caller about the same thing, a short way off.
func (s *sim) dupCall(lat, lon float64, words string) {
	s.mu.Lock()
	s.n++
	id := fmt.Sprintf("sim%s-%04d", runID, s.n)
	la, lo := lat+(mrand.Float64()-0.5)*0.0015, lon+(mrand.Float64()-0.5)*0.0015
	c := &call{ID: id, State: "ringing", Direction: "in", From: fmt.Sprintf("+1540555%04d", 100+mrand.IntN(9800)), To: "911", Start: time.Now().UTC(),
		Emergency: &emergency{ESInet: true, Additional: map[string]any{"service": map[string]any{"types": []string{"wireless"}}},
			Location: &location{Lat: &la, Lon: &lo, Radius: math.Round(20 + mrand.Float64()*60), Confidence: 90, Method: "Cell", Time: time.Now().UTC()}}}
	s.calls[id] = c
	s.story[id] = story{name: "another caller", lines: []line{{1, "taker", "911, what is the address of your emergency?"}, {4, "caller", words}}}
	s.mu.Unlock()
	s.publish("call.started", c)
	go s.lifecycle(id)
}

func (s *sim) lifecycle(id string) {
	time.Sleep(time.Duration(5+mrand.IntN(8)) * time.Second)
	s.mu.Lock()
	c, ok := s.calls[id]
	if !ok {
		s.mu.Unlock()
		return
	}
	c.State, c.Answer = "answered", time.Now().UTC()
	c.Emergency.TakenBy, c.Emergency.TakenName = "taker1", "Call-taker 1"
	s.mu.Unlock()
	s.publish("call.updated", c)
	s.publish("emergency.answered", map[string]any{"call": id, "agent": "taker1", "from": c.From})
	s.mu.Lock()
	sto, addr := s.story[id], s.addr[id]
	s.mu.Unlock()
	if !c.Emergency.TextOnly {
		go s.transcribe(id, firstNonEmpty(addr, "the corner here"), sto, sto.lines)
	}
	if s.push != nil && s.push.eidoURL != "" {
		go s.push.post(s.push.eidoURL, s.push.eidoToken, "application/emergency.eido+json", s.eido(c))
	}
	if s.push != nil && s.push.ali != "" {
		go s.spill(c)
	}
	// A wireless caller's position improves after a few seconds.
	time.Sleep(time.Duration(10+mrand.IntN(10)) * time.Second)
	s.mu.Lock()
	if l := c.Emergency.Location; l != nil && l.Radius > 20 && c.State != "ended" {
		l.Radius = math.Round(l.Radius / 3)
		l.Time = time.Now().UTC()
		s.mu.Unlock()
		s.publish("call.updated", c)
	} else {
		s.mu.Unlock()
	}
	time.Sleep(time.Duration(90+mrand.IntN(120)) * time.Second)
	s.mu.Lock()
	delete(s.calls, id)
	delete(s.story, id)
	delete(s.addr, id)
	s.mu.Unlock()
	c.State = "ended"
	s.publish("call.ended", c)
}

func firstNonEmpty(v ...string) string {
	for _, x := range v {
		if x != "" {
			return x
		}
	}
	return ""
}

func (s *sim) spill(c *call) {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("POS 101  %s\r\n", time.Now().Format("01/02 15:04:05")))
	d := strings.TrimPrefix(c.From, "+1")
	b.WriteString(fmt.Sprintf("(%s) %s-%s  WRLS\r\n", d[:3], d[3:6], d[6:]))
	if l := c.Emergency.Location; l != nil && l.Lat != nil {
		b.WriteString(fmt.Sprintf("LAT %+.6f\r\nLON %+.6f\r\n", *l.Lat, *l.Lon))
	}
	b.WriteString("CO SIMULATED CARRIER\r\n")
	if err := ng911.SendSpill(context.Background(), s.push.ali, b.String()); err != nil {
		log.Printf("ali: %v", err)
	}
}

// eido writes an EIDO in the shape BitVoice's internal/i3 encoder writes.
func (s *sim) eido(c *call) []byte {
	ts := func(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z07:00") }
	agency := map[string]string{"$ref": s.agency}
	callID := "urn:emergency:uid:callid:" + c.ID + ":bvsim"
	callComp := map[string]any{"$id": callID, "lastUpdateTimeStamp": ts(time.Now()), "updatedByAgencyReference": agency, "queueIdentifier": "911",
		"standardPrimaryCallType": "emergency", "direction": "incoming", "callStartTimestamp": ts(c.Start), "callStateRegistryText": "callAnswered",
		"callBackReference": map[string]string{"$ref": "callback-" + c.ID}, "answerDate": ts(c.Answer)}
	doc := map[string]any{"$id": "urn:emergency:uid:incidentid:" + c.ID + ":bvsim", "eidoVersion": "1.0", "issuingElementIdentification": "bvsim",
		"lastUpdateTimeStamp": ts(time.Now()), "updatedByAgencyReference": agency, "callComponent": []any{callComp},
		"callbackComponent": []any{map[string]any{"$id": "callback-" + c.ID, "callBackInformationUri": []string{"tel:" + c.From}}},
		"agentComponent":    []any{map[string]any{"$id": "taker1@" + s.agency, "agentWorkstationPositionIdentification": "101"}}}
	if l := c.Emergency.Location; l != nil {
		var pidf strings.Builder
		pidf.WriteString(`<presence xmlns="urn:ietf:params:xml:ns:pidf" entity="pres:` + c.From + `@sim.invalid"><tuple id="t"><status><geopriv><location-info>`)
		if cv := l.Civic; cv != nil {
			pidf.WriteString(fmt.Sprintf(`<civicAddress><country>US</country><A1>%s</A1><A3>%s</A3><HNO>%s</HNO><PRD>%s</PRD><RD>%s</RD><STS>%s</STS><PC>%s</PC></civicAddress>`,
				cv.State, cv.City, cv.Number, cv.PreDir, html.EscapeString(cv.Street), cv.StreetType, cv.PostalCode))
		}
		if l.Lat != nil && l.Altitude != nil {
			pidf.WriteString(fmt.Sprintf(`<Ellipsoid srsName="urn:ogc:def:crs:EPSG::4979"><pos>%f %f %.1f</pos><semiMajorAxis>%.0f</semiMajorAxis><semiMinorAxis>%.0f</semiMinorAxis><verticalAxis>%.0f</verticalAxis><orientation>0</orientation></Ellipsoid><confidence>%d</confidence>`,
				*l.Lat, *l.Lon, *l.Altitude, l.Radius, l.Radius, l.VRadius, l.Confidence))
		} else if l.Lat != nil {
			pidf.WriteString(fmt.Sprintf(`<Circle><pos>%f %f</pos><radius>%.0f</radius></Circle><confidence>%d</confidence>`, *l.Lat, *l.Lon, l.Radius, l.Confidence))
		}
		pidf.WriteString(`</location-info><method>` + l.Method + `</method></geopriv></status><timestamp>` + l.Time.Format(time.RFC3339) + `</timestamp></tuple></presence>`)
		callComp["locationReference"] = []any{map[string]string{"$ref": "location-" + c.ID}}
		doc["locationComponent"] = []any{map[string]any{"$id": "location-" + c.ID, "locationByValue": pidf.String()}}
	}
	add, _ := json.Marshal(c.Emergency.Additional)
	adds := []any{map[string]any{"$id": "additional-" + c.ID, "additionalDataByValue": string(add)}}
	if v := c.Emergency.Video; v != nil {
		adds = append(adds, map[string]any{"$id": "media-" + c.ID + "-live", "additionalDataByReferenceUrl": s.base + "/whep/" + v.ViewToken, "urlPurpose": "caller-video-live"})
	}
	doc["additionalDataComponent"] = adds
	b, _ := json.MarshalIndent(doc, "", "  ")
	return b
}

func (s *sim) radioLoop(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s.mu.Lock()
		units := append([]*radioUnit{}, s.units...)
		s.mu.Unlock()
		for _, u := range units {
			// A slow random drive: units on patrol move, others drift slightly.
			step := 0.0003
			if u.ID >= 197 {
				step = 0.0012
			}
			u.Heading += (mrand.Float64() - 0.5) * 60
			u.Lat += step * math.Cos(u.Heading*math.Pi/180)
			u.Lon += step * math.Sin(u.Heading*math.Pi/180)
			s.publish("radio.location", map[string]any{"unit": u.ID, "alias": u.Alias, "lat": u.Lat, "lon": u.Lon, "speed": math.Round(step * 111000 / 5 * 3.6), "heading": math.Mod(u.Heading+360, 360), "at": time.Now().UTC(), "source": "SIMULATED AVL"})
			if mrand.IntN(10) == 0 {
				now := time.Now().UTC()
				s.publish("radio.transmission", map[string]any{"id": fmt.Sprintf("tx%d", now.UnixNano()), "channel": "tg1", "channelName": "County dispatch", "direction": "rx",
					"start": now.Add(-3 * time.Second), "end": now, "seconds": 3, "unit": fmt.Sprintf("bee07.ad5.%06x", u.ID), "unitName": u.Alias, "text": radioText(u.Alias)})
			}
		}
	}
}

func big1() *big.Int {
	n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 100))
	return n
}

func selfSigned() (tls.Certificate, string) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big1(), Subject: pkix.Name{CommonName: "bvsim (SIMULATED BitVoice)"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(0, 1, 0),
		DNSNames: []string{"localhost"}, IPAddresses: nil, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, _ := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	sum := sha256.Sum256(der)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, hex.EncodeToString(sum[:])
}

func main() {
	addr := flag.String("addr", "127.0.0.1:9444", "where to serve the simulated BitVoice API (HTTPS, self-signed)")
	token := flag.String("token", "bv_simulated", "the API token bitdispatcher must send")
	every := flag.Duration("every", 40*time.Second, "a new 911 call this often (0: none)")
	first := flag.Int("calls", 2, "911 calls to start with")
	eidoURL := flag.String("eido-url", "", "also push EIDOs here (like BitVoice's psap.eidoUrl)")
	eidoToken := flag.String("eido-token", "", "bearer token for -eido-url")
	radioURL := flag.String("radio-url", "", "also push radio events here (like BitVoice's radio.cadUrl)")
	radioSecret := flag.String("radio-secret", "", "bearer secret for -radio-url")
	ali := flag.String("ali", "", "also send ALI spills to this host:port")
	hookURL := flag.String("hook-url", "", "bitdispatcher's hooks (https://host/api/hooks): send crash notifications and drone reports there")
	hookToken := flag.String("hook-token", "", "bearer token for -hook-url")
	crashEvery := flag.Int("crash-every", 7, "every this many calls, a vehicle crash notification instead (0: none)")
	droneAddr := flag.String("drone-addr", "", "serve a simulated drone system here (plain HTTP on this host:port; POST /launch)")
	droneToken := flag.String("drone-token", "", "bearer token the drone system expects")
	flag.Parse()
	cert, fp := selfSigned()
	s := &sim{calls: map[string]*call{}, subs: map[chan string]struct{}{}, token: *token, agency: "psap.sim.invalid", base: "https://" + *addr,
		story: map[string]story{}, addr: map[string]string{}, droneToken: *droneToken, media: map[string][]photo{}, held: map[string]bool{}}
	if *eidoURL != "" || *radioURL != "" || *ali != "" || *hookURL != "" {
		s.push = &pusher{eidoURL: *eidoURL, eidoToken: *eidoToken, radioURL: *radioURL, radioSecret: *radioSecret, ali: *ali,
			hookURL: strings.TrimRight(*hookURL, "/"), hookToken: *hookToken,
			client: &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}} // a simulator talking to a local test server
	}
	for _, u := range []radioUnit{{101, "Medic 1", 37.26722, -79.94578, 0}, {102, "Medic 2", 37.32324, -79.96896, 90}, {103, "Medic 3", 37.25063, -79.91657, 180},
		{104, "Medic 4", 37.29182, -80.05668, 270}, {197, "Adam 11", 37.27108, -79.94760, 45}, {198, "Adam 12", 37.28300, -79.93300, 135},
		{199, "Adam 13", 37.25300, -79.96000, 225}, {200, "Adam 14", 37.30500, -79.95000, 315}, {210, "Sam 1", 37.27108, -79.94760, 10}} {
		u := u
		s.units = append(s.units, &u)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/events", s.auth(func(w http.ResponseWriter, r *http.Request) {
		fl := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		ch := make(chan string, 256)
		s.mu.Lock()
		s.subs[ch] = struct{}{}
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			delete(s.subs, ch)
			s.mu.Unlock()
		}()
		fmt.Fprint(w, "event: hello\ndata: {}\n\n")
		fl.Flush()
		ping := time.NewTicker(20 * time.Second)
		defer ping.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ping.C:
				fmt.Fprint(w, ": ping\n\n")
			case f := <-ch:
				fmt.Fprint(w, f)
			}
			fl.Flush()
		}
	}))
	mux.HandleFunc("GET /api/calls", s.auth(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		out := []*call{}
		for _, c := range s.calls {
			out = append(out, c)
		}
		s.mu.Unlock()
		writeJSON(w, map[string]any{"items": out})
	}))
	mux.HandleFunc("GET /api/calls/{id}/eido", s.auth(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		c, ok := s.calls[r.PathValue("id")]
		s.mu.Unlock()
		if !ok {
			http.Error(w, `{"message":"no such call"}`, 404)
			return
		}
		w.Header().Set("Content-Type", "application/emergency.eido+json")
		w.Write(s.eido(c))
	}))
	mux.HandleFunc("POST /api/calls/{id}/rebid", s.auth(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		c, ok := s.calls[r.PathValue("id")]
		if !ok || c.Emergency.LocationURI == "" {
			s.mu.Unlock()
			http.Error(w, `{"message":"this call has no location reference"}`, 409)
			return
		}
		l := c.Emergency.Location
		la, lo := *l.Lat+(mrand.Float64()-0.5)*0.0002, *l.Lon+(mrand.Float64()-0.5)*0.0002
		nl := &location{Lat: &la, Lon: &lo, Radius: math.Max(5, math.Round(l.Radius/2)), Confidence: 95, Method: "GPS", Time: time.Now().UTC()}
		c.Emergency.LocCheck = map[string]any{"show": "new", "text": "Show the new location first", "previous": l,
			"reasons": []string{"SIMULATED: the new location is the latest and as precise as the first."}}
		c.Emergency.Location = nl
		s.mu.Unlock()
		s.publish("call.updated", c)
		writeJSON(w, map[string]any{"location": nl})
	}))
	mux.HandleFunc("POST /api/calls/{id}/text", s.auth(func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Via, Text string }
		json.NewDecoder(r.Body).Decode(&b)
		s.mu.Lock()
		c, ok := s.calls[r.PathValue("id")]
		if !ok {
			s.mu.Unlock()
			http.Error(w, `{"message":"the call has ended"}`, 409)
			return
		}
		via := b.Via
		if via == "" {
			via = "msrp"
		}
		c.Emergency.Text = append(c.Emergency.Text, textLine{At: time.Now().UTC(), From: "taker", Name: "Dispatcher", Via: via, Text: b.Text})
		s.mu.Unlock()
		s.publish("call.updated", c)
		go func() {
			time.Sleep(4 * time.Second)
			s.mu.Lock()
			c.Emergency.Text = append(c.Emergency.Text, textLine{At: time.Now().UTC(), From: "caller", Via: via, Text: "SIMULATED reply: yes, upstairs bedroom"})
			s.mu.Unlock()
			s.publish("call.updated", c)
		}()
		writeJSON(w, map[string]any{})
	}))
	mux.HandleFunc("POST /api/calls/{id}/video/link", s.auth(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		c, ok := s.calls[r.PathValue("id")]
		if !ok {
			s.mu.Unlock()
			http.Error(w, `{"message":"the call has ended"}`, 409)
			return
		}
		c.Emergency.Video = &struct {
			ViewToken string `json:"viewToken,omitempty"`
			Live      bool   `json:"live"`
			Source    string `json:"source,omitempty"`
		}{ViewToken: "view-" + c.ID, Source: "link"}
		s.mu.Unlock()
		s.publish("call.video", map[string]any{"call": c.ID})
		s.publish("call.updated", c)
		writeJSON(w, map[string]any{"url": s.base + "/v/tok-" + c.ID, "viewUrl": s.base + "/whep/view-" + c.ID, "sent": true})
	}))
	mux.HandleFunc("POST /whep/{tok}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "the simulator has no video", 503)
	})
	// The rest of what BitVoice offers a CAD, as an empty lab system would answer.
	mux.HandleFunc("GET /api/calls/{id}/media", s.auth(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		s.mu.Lock()
		items := []any{}
		for _, p := range s.media[id] {
			items = append(items, map[string]any{"n": p.n, "name": fmt.Sprintf("caller-photo-%d.jpg", p.n), "type": "image/jpeg", "kind": "photo", "size": len(p.data),
				"sha256": p.sha, "received": p.at, "takenLocal": p.takenLocal, "hasGps": true, "lat": p.lat, "lon": p.lon})
		}
		held := s.held[id]
		s.mu.Unlock()
		writeJSON(w, map[string]any{"call": id, "held": held, "items": items})
	}))
	mux.HandleFunc("GET /api/calls/{id}/media/{n}", s.auth(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		var data []byte
		for _, p := range s.media[r.PathValue("id")] {
			if fmt.Sprint(p.n) == r.PathValue("n") {
				data = p.data
			}
		}
		s.mu.Unlock()
		if data == nil {
			http.Error(w, `{"message":"no such photo"}`, 404)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(data)
	}))
	mux.HandleFunc("POST /api/calls/{id}/media/hold", s.auth(func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Hold bool }
		json.NewDecoder(r.Body).Decode(&b)
		s.mu.Lock()
		s.held[r.PathValue("id")] = b.Hold
		s.mu.Unlock()
		writeJSON(w, map[string]any{"held": b.Hold})
	}))
	mux.HandleFunc("POST /api/calls/{id}/video/reveal", s.auth(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]any{}) }))
	mux.HandleFunc("POST /api/core/validate", s.auth(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"no address data is loaded"}`, 409)
	}))
	mux.HandleFunc("GET /api/core/states", s.auth(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]any{"items": []any{}}) }))
	mux.HandleFunc("GET /api/core/i3log", s.auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"day": time.Now().UTC().Format("2006-01-02"), "items": []any{}})
	}))
	mux.HandleFunc("GET /api/psap/exposure", s.auth(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]any{"people": []any{}}) }))
	mux.HandleFunc("GET /api/psap/report", s.auth(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		n, ans, sum := 0, 0, 0.0
		for _, c := range s.calls {
			n++
			if !c.Answer.IsZero() {
				ans++
				sum += c.Answer.Sub(c.Start).Seconds()
			}
		}
		s.mu.Unlock()
		avg := 0.0
		if ans > 0 {
			avg = sum / float64(ans)
		}
		writeJSON(w, map[string]any{"target": 15, "calls": n, "answered": ans, "abandoned": n - ans, "withinTarget": 100.0, "within20": 100.0, "average": avg, "longest": avg, "byTaker": []any{}})
	}))
	mux.HandleFunc("GET /api/radio/locations", s.auth(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		out := []any{}
		for _, u := range s.units {
			out = append(out, map[string]any{"unit": u.ID, "alias": u.Alias, "lat": u.Lat, "lon": u.Lon, "at": time.Now().UTC(), "source": "SIMULATED AVL"})
		}
		s.mu.Unlock()
		writeJSON(w, map[string]any{"items": out})
	}))
	mux.HandleFunc("GET /api/radio", s.auth(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		em := s.emUnit
		s.mu.Unlock()
		writeJSON(w, map[string]any{"channels": []any{map[string]any{"id": "tg1", "name": "County dispatch", "emergencyUnit": em}}, "positions": []any{}})
	}))
	mux.HandleFunc("POST /api/radio/channels/{id}/ack", s.auth(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.emUnit = ""
		s.mu.Unlock()
		s.publish("radio.emergency.ack", map[string]any{"channel": r.PathValue("id"), "channelName": "County dispatch", "by": "bitdispatcher", "at": time.Now().UTC()})
		writeJSON(w, map[string]any{})
	}))
	// Demonstration triggers.
	mux.HandleFunc("POST /sim/emergency", func(w http.ResponseWriter, r *http.Request) {
		id := 197
		fmt.Sscan(r.URL.Query().Get("unit"), &id)
		s.mu.Lock()
		s.emUnit = fmt.Sprintf("bee07.ad5.%06x", id)
		s.mu.Unlock()
		s.publish("radio.emergency", map[string]any{"channel": "tg1", "channelName": "County dispatch", "unit": fmt.Sprintf("bee07.ad5.%06x", id), "unitId": id, "emergency": true, "at": time.Now().UTC()})
		w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("POST /sim/call", func(w http.ResponseWriter, r *http.Request) {
		s.newCall()
		w.Write([]byte("ok\n"))
	})
	// A caller sends a photo (to the newest call in progress, or ?call=ID).
	mux.HandleFunc("POST /sim/photo", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		id := r.URL.Query().Get("call")
		var newest *call
		for _, c := range s.calls {
			if c.State != "ended" && (id == "" || c.ID == id) && (newest == nil || c.Start.After(newest.Start)) {
				newest = c
			}
		}
		if newest == nil {
			s.mu.Unlock()
			http.Error(w, "no call in progress", 409)
			return
		}
		data := simPhoto()
		sum := sha256.Sum256(data)
		lat, lon := 37.27, -79.94
		if l := newest.Emergency.Location; l != nil && l.Lat != nil {
			lat, lon = *l.Lat, *l.Lon
		}
		p := photo{n: len(s.media[newest.ID]) + 1, data: data, sha: hex.EncodeToString(sum[:]), at: time.Now().UTC(), lat: lat, lon: lon,
			takenLocal: time.Now().Add(-2 * time.Minute).Format("2006-01-02 15:04:05")}
		s.media[newest.ID] = append(s.media[newest.ID], p)
		id = newest.ID
		s.mu.Unlock()
		s.publish("call.media", map[string]any{"call": id, "n": p.n})
		writeJSON(w, map[string]any{"call": id, "n": p.n, "sha256": p.sha})
	})
	mux.HandleFunc("POST /sim/cadfailed", func(w http.ResponseWriter, r *http.Request) {
		s.publish("emergency.cad_failed", map[string]any{"call": "", "what": "ALI", "error": "SIMULATED: dial tcp: connection refused"})
		w.Write([]byte("ok\n"))
	})
	ctx := context.Background()
	go s.radioLoop(ctx)
	go func() {
		time.Sleep(2 * time.Second)
		for i := 0; i < *first; i++ {
			s.newCall()
		}
		if *every <= 0 {
			return
		}
		n := 0
		for range time.Tick(*every) {
			n++
			if *crashEvery > 0 && n%*crashEvery == 0 {
				s.crash()
				continue
			}
			s.newCall()
		}
	}()
	if *droneAddr != "" {
		d := newDrones(s)
		dm := http.NewServeMux()
		dm.HandleFunc("POST /launch", d.launch)
		go d.fly()
		go func() {
			ds := &http.Server{Addr: *droneAddr, Handler: dm, ReadHeaderTimeout: 5 * time.Second}
			log.Printf("bvsim: SIMULATED drone system at http://%s/launch", *droneAddr)
			log.Fatal(ds.ListenAndServe())
		}()
	}
	mux.HandleFunc("POST /sim/crash", func(w http.ResponseWriter, r *http.Request) {
		s.crash()
		w.Write([]byte("ok\n"))
	})
	srv := &http.Server{Addr: *addr, Handler: mux, TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}}, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("bvsim: SIMULATED BitVoice at https://%s  token %s  certificate sha256 %s", *addr, *token, fp)
	log.Fatal(srv.ListenAndServeTLS("", ""))
}
