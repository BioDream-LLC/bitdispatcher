// Command i3caller places a lab NG9-1-1 call (urn:service:sos) at a PSAP's call-handling system, as an
// ESInet's border would deliver it: SIP over UDP, the caller's location by value (PIDF-LO, RFC 5491),
// Additional Data by value (RFC 7852: service, device, subscriber, provider), the number's STIR/SHAKEN
// verification (verstat), and voice (G.711 RTP) or text (MSRP, RFC 4975).
//
// It is for testing a call-handling system and the CAD behind it in a lab. It never calls a real 911:
// it sends only to the address given, and the PSAP's trunk must trust that address.
//
//	i3caller -psap 127.0.0.1:5060 -from +15405550123 -lat 37.2712 -lon -79.9418 -hold 120
//	i3caller -psap 127.0.0.1:5060 -text "there is a man with a gun outside" -hold 90
package main

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"math"
	mrand "math/rand/v2"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type opts struct {
	psap, bind, from, name, verstat, class, method, civic, text, provider, device string
	lat, lon, radius, alt                                                         float64
	hold                                                                          time.Duration
	byRef                                                                         bool
	speech                                                                        []int16 // -wav: what the caller says, 8 kHz
}

func main() {
	var o opts
	flag.StringVar(&o.psap, "psap", "127.0.0.1:5060", "the call-handling system's SIP address (host:port, UDP)")
	flag.StringVar(&o.bind, "bind", "127.0.0.1", "the local address to send from (the PSAP's trunk must allow it)")
	flag.StringVar(&o.from, "from", "+15405550123", "the caller's number")
	flag.StringVar(&o.name, "name", "TEST CALLER", "the subscriber's name (Additional Data)")
	flag.StringVar(&o.verstat, "verstat", "TN-Validation-Passed", "STIR/SHAKEN verification of the number (TN-Validation-Passed, TN-Validation-Failed, No-TN-Validation, or empty)")
	flag.StringVar(&o.class, "class", "Wireless", "service type (Additional Data ServiceInfo): Wireless, Wireline, VOIP, ...")
	flag.StringVar(&o.method, "method", "GPS", "location method: GPS, Cell, DBH, Manual, ...")
	flag.Float64Var(&o.lat, "lat", 37.27120, "latitude")
	flag.Float64Var(&o.lon, "lon", -79.94180, "longitude")
	flag.Float64Var(&o.radius, "radius", 8, "uncertainty radius, metres")
	flag.Float64Var(&o.alt, "alt", 0, "altitude in metres (0: none)")
	flag.StringVar(&o.civic, "civic", "215|Church|Ave|SW|Roanoke|VA|24011", "civic address: number|street|suffix|postdir|city|state|zip (empty: none)")
	flag.StringVar(&o.text, "text", "", "text to 911 over MSRP instead of voice: the caller's first message (more lines: type them while it runs)")
	flag.StringVar(&o.provider, "provider", "Lab Wireless Co", "the service provider (Additional Data ProviderInfo)")
	flag.StringVar(&o.device, "device", "Lab handset", "the device (Additional Data DeviceInfo)")
	wavFile := flag.String("wav", "", "voice calls: say this recording (a WAV, 16-bit mono 8 kHz, such as macOS: say -o f.wav --data-format=LEI16@8000 \"...\") after the answer, instead of the tone")
	flag.DurationVar(&o.hold, "hold", 2*time.Minute, "how long to stay on the call before hanging up")
	flag.BoolVar(&o.byRef, "by-ref", true, "also give the location by reference (a HELD server here), so a rebid gets a newer one: the caller moves about 40 m north-east each time")
	phoneExt := flag.String("phone", "", "instead of calling: register as this extension (a call-taker's phone) and answer every call")
	secretFile := flag.String("secret-file", "", "with -phone: a file holding the extension's SIP password")
	ring := flag.Duration("ring", 4*time.Second, "with -phone: how long a call rings before it is answered")
	radioAt := flag.String("radio", "", "instead of calling: be a P25 radio system's home RFSS at this address (host:port), and have a radio call the console")
	issi := flag.String("issi", "127.0.0.1:5062", "with -radio: the console's ISSI SIP address")
	wacn := flag.String("wacn", "BEE00", "with -radio: the WACN (5 hex digits)")
	sysID := flag.String("system", "1A2", "with -radio: the system ID (3 hex digits)")
	consoleUnit := flag.Int("console-unit", 9999, "with -radio: the console's unit ID")
	radioUnit := flag.Int("unit", 101, "with -radio: the radio's unit ID")
	emergency := flag.Bool("emergency", true, "with -radio: the radio's call is an emergency")
	esinetAt := flag.String("esinet", "", "instead of calling: stand in for the ESInet at a trunk's address (host:port), answering the call-handling system's OPTIONS keepalives")
	after := flag.Duration("after", 5*time.Second, "with -radio: when the radio calls")
	flag.Parse()
	if *wavFile != "" {
		pcm, err := readWAV(*wavFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "i3caller:", err)
			os.Exit(2)
		}
		o.speech = pcm
	}
	if *esinetAt != "" {
		if err := runESInet(*esinetAt); err != nil {
			fmt.Fprintln(os.Stderr, "i3caller:", err)
			os.Exit(1)
		}
		return
	}
	if *radioAt != "" {
		if err := runRadio(*radioAt, *issi, *wacn, *sysID, *consoleUnit, *radioUnit, *emergency, *after); err != nil {
			fmt.Fprintln(os.Stderr, "i3caller:", err)
			os.Exit(1)
		}
		return
	}
	if *phoneExt != "" {
		b, err := os.ReadFile(*secretFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "i3caller: the extension's password:", err)
			os.Exit(1)
		}
		if err := runPhone(o.psap, o.bind, *phoneExt, strings.TrimSpace(string(b)), *ring); err != nil {
			fmt.Fprintln(os.Stderr, "i3caller:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "i3caller:", err)
		os.Exit(1)
	}
}

func token(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func pidf(o opts, entity string) string {
	var civ string
	if o.civic != "" {
		p := append(strings.Split(o.civic, "|"), make([]string, 7)...)
		tags := []string{"HNO", "RD", "STS", "POD", "A3", "A1", "PC"}
		var b strings.Builder
		b.WriteString(`<ca:civicAddress xmlns:ca="urn:ietf:params:xml:ns:pidf:geopriv10:civicAddr"><ca:country>US</ca:country>`)
		for i, t := range tags {
			if p[i] != "" {
				fmt.Fprintf(&b, "<ca:%s>%s</ca:%s>", t, xmlEsc(p[i]), t)
			}
		}
		b.WriteString(`</ca:civicAddress>`)
		civ = b.String()
	}
	pos := fmt.Sprintf("%.6f %.6f", o.lat, o.lon)
	if o.alt != 0 {
		pos += fmt.Sprintf(" %.1f", o.alt)
	}
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<presence xmlns="urn:ietf:params:xml:ns:pidf" xmlns:gp="urn:ietf:params:xml:ns:pidf:geopriv10" xmlns:gml="http://www.opengis.net/gml" xmlns:gs="http://www.opengis.net/pidflo/1.0" entity="` + entity + `">` +
		`<tuple id="loc"><status><gp:geopriv><gp:location-info>` +
		`<gs:Circle srsName="urn:ogc:def:crs:EPSG::4326"><gml:pos>` + pos + `</gml:pos><gs:radius uom="urn:ogc:def:uom:EPSG::9001">` + strconv.FormatFloat(o.radius, 'f', 0, 64) + `</gs:radius></gs:Circle>` +
		civ + `</gp:location-info><gp:usage-rules/><gp:method>` + o.method + `</gp:method></gp:geopriv></status>` +
		`<timestamp>` + time.Now().UTC().Format(time.RFC3339) + `</timestamp></tuple></presence>`
}

func xmlEsc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

func additional(o opts) map[string]string {
	return map[string]string{
		"ServiceInfo":    `<?xml version="1.0"?><svc:EmergencyCallData.ServiceInfo xmlns:svc="urn:ietf:params:xml:ns:EmergencyCallData:ServiceInfo"><svc:DataProviderReference>p1</svc:DataProviderReference><svc:ServiceEnvironment>Residence</svc:ServiceEnvironment><svc:ServiceType>` + xmlEsc(strings.ToLower(o.class)) + `</svc:ServiceType><svc:ServiceMobility>Mobile</svc:ServiceMobility></svc:EmergencyCallData.ServiceInfo>`,
		"ProviderInfo":   `<?xml version="1.0"?><pi:EmergencyCallData.ProviderInfo xmlns:pi="urn:ietf:params:xml:ns:EmergencyCallData:ProviderInfo"><pi:DataProviderReference>p1</pi:DataProviderReference><pi:DataProviderString>` + xmlEsc(o.provider) + `</pi:DataProviderString><pi:ProviderID>LAB01</pi:ProviderID><pi:ProviderIDSeries>NENA</pi:ProviderIDSeries><pi:TypeOfProvider>Service Provider</pi:TypeOfProvider><pi:ContactURI>tel:+15405550000</pi:ContactURI></pi:EmergencyCallData.ProviderInfo>`,
		"DeviceInfo":     `<?xml version="1.0"?><dev:EmergencyCallData.DeviceInfo xmlns:dev="urn:ietf:params:xml:ns:EmergencyCallData:DeviceInfo"><dev:DataProviderReference>p1</dev:DataProviderReference><dev:DeviceClassification>` + xmlEsc(o.device) + `</dev:DeviceClassification></dev:EmergencyCallData.DeviceInfo>`,
		"SubscriberInfo": `<?xml version="1.0"?><sub:EmergencyCallData.SubscriberInfo xmlns:sub="urn:ietf:params:xml:ns:EmergencyCallData:SubscriberInfo" privacyRequested="false"><sub:DataProviderReference>p1</sub:DataProviderReference><sub:SubscriberData><vcard xmlns="urn:ietf:params:xml:ns:vcard-4.0"><fn><text>` + xmlEsc(o.name) + `</text></fn><tel><uri>tel:` + o.from + `</uri></tel></vcard></sub:SubscriberData></sub:EmergencyCallData.SubscriberInfo>`,
	}
}

type caller struct {
	o                  opts
	conn               *net.UDPConn
	psap               *net.UDPAddr
	callID, tag, local string
	mu                 sync.Mutex
	toTag, contact     string
	cseq               int
	inviteVia          string
	answered           chan string // the answer's SDP
	ended              chan string
}

func run(o opts) error {
	psap, err := net.ResolveUDPAddr("udp", o.psap)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(o.bind)})
	if err != nil {
		return err
	}
	defer conn.Close()
	c := &caller{o: o, conn: conn, psap: psap, callID: "i3c-" + token(8), tag: token(4), local: conn.LocalAddr().String(), cseq: 1,
		answered: make(chan string, 1), ended: make(chan string, 1)}
	go c.read()

	// Media: an RTP port for voice, or an MSRP listener for text.
	var rtp *net.UDPConn
	var msrpLn net.Listener
	var sdpBody string
	host := o.bind
	sess := strconv.FormatInt(time.Now().Unix(), 10)
	if o.text == "" {
		rtp, err = net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(o.bind)})
		if err != nil {
			return err
		}
		defer rtp.Close()
		port := rtp.LocalAddr().(*net.UDPAddr).Port
		sdpBody = "v=0\r\no=i3caller " + sess + " " + sess + " IN IP4 " + host + "\r\ns=-\r\nc=IN IP4 " + host + "\r\nt=0 0\r\nm=audio " + strconv.Itoa(port) +
			" RTP/AVP 0 101\r\na=rtpmap:0 PCMU/8000\r\na=rtpmap:101 telephone-event/8000\r\na=sendrecv\r\n"
	} else {
		msrpLn, err = net.Listen("tcp", net.JoinHostPort(o.bind, "0"))
		if err != nil {
			return err
		}
		defer msrpLn.Close()
		port := msrpLn.Addr().(*net.TCPAddr).Port
		sdpBody = "v=0\r\no=i3caller " + sess + " " + sess + " IN IP4 " + host + "\r\ns=-\r\nc=IN IP4 " + host + "\r\nt=0 0\r\nm=message " + strconv.Itoa(port) +
			" TCP/MSRP *\r\na=accept-types:text/plain\r\na=path:msrp://" + host + ":" + strconv.Itoa(port) + "/" + c.tag + ";tcp\r\na=setup:active\r\n"
	}

	// The location by reference: a HELD server that answers with the caller's position, moving it each time.
	var locRef string
	if o.byRef {
		ln, err := net.Listen("tcp", net.JoinHostPort(o.bind, "0"))
		if err != nil {
			return err
		}
		defer ln.Close()
		path := "/held/" + token(8)
		locRef = "http://" + ln.Addr().String() + path
		var mu sync.Mutex
		n := 0
		mux := http.NewServeMux()
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			n++
			cur := o
			cur.lat += 0.00025 * float64(n)
			cur.lon += 0.00030 * float64(n)
			cur.radius = 5
			k := n
			mu.Unlock()
			fmt.Printf("i3caller: rebid %d: now at %.5f, %.5f\n", k, cur.lat, cur.lon)
			w.Header().Set("Content-Type", "application/held+xml;charset=utf-8")
			io.WriteString(w, `<?xml version="1.0"?><locationResponse xmlns="urn:ietf:params:xml:ns:geopriv:held">`+
				strings.TrimPrefix(pidf(cur, "pres:"+strings.TrimPrefix(o.from, "+")+"@i3caller"), `<?xml version="1.0" encoding="UTF-8"?>`)+`</locationResponse>`)
		})
		go http.Serve(ln, mux)
	}

	// The INVITE: SDP, the PIDF-LO, and Additional Data, each a part.
	boundary := "i3c-" + token(6)
	var body strings.Builder
	part := func(ct, cid, data string) {
		fmt.Fprintf(&body, "--%s\r\nContent-Type: %s\r\n", boundary, ct)
		if cid != "" {
			fmt.Fprintf(&body, "Content-ID: <%s>\r\n", cid)
		}
		fmt.Fprintf(&body, "\r\n%s\r\n", data)
	}
	part("application/sdp", "", sdpBody)
	locCID := "loc-" + c.tag + "@i3caller"
	part("application/pidf+xml", locCID, pidf(o, "pres:"+strings.TrimPrefix(o.from, "+")+"@i3caller"))
	var callInfo []string
	for k, v := range additional(o) {
		cid := strings.ToLower(k) + "-" + c.tag + "@i3caller"
		part("application/EmergencyCallData."+k+"+xml", cid, v)
		callInfo = append(callInfo, "<cid:"+cid+">;purpose=EmergencyCallData."+k)
	}
	fmt.Fprintf(&body, "--%s--\r\n", boundary)
	user := o.from
	if o.verstat != "" {
		user += ";verstat=" + o.verstat
	}
	hdr := map[string]string{
		"From":                c.fromHdr(),
		"To":                  "<urn:service:sos>",
		"P-Asserted-Identity": "<sip:" + user + "@esinet.lab;user=phone>",
		"Geolocation":         geoHeader("<cid:"+locCID+">", locRef),
		"Geolocation-Routing": "yes",
		"Call-Info":           strings.Join(callInfo, ", "),
		"Content-Type":        "multipart/mixed;boundary=" + boundary,
	}
	fmt.Printf("i3caller: calling %s from %s at %.5f, %.5f (%s)\n", o.psap, o.from, o.lat, o.lon, map[bool]string{true: "text", false: "voice"}[o.text != ""])
	c.send("INVITE", "urn:service:sos", 1, hdr, body.String())

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	deadline := time.After(o.hold)
	var answer string
	select {
	case answer = <-c.answered:
		fmt.Println("i3caller: answered")
	case why := <-c.ended:
		return fmt.Errorf("the call ended before an answer: %s", why)
	case <-deadline:
		c.send("CANCEL", "urn:service:sos", 1, map[string]string{"From": c.fromHdr(), "To": "<urn:service:sos>"}, "")
		fmt.Println("i3caller: no answer; cancelled")
		return nil
	case <-stop:
		c.send("CANCEL", "urn:service:sos", 1, map[string]string{"From": c.fromHdr(), "To": "<urn:service:sos>"}, "")
		return nil
	}

	quit := make(chan struct{})
	if rtp != nil {
		go c.voice(rtp, answer, quit)
	} else {
		go c.textChat(msrpLn, answer, quit)
	}
	select {
	case why := <-c.ended:
		close(quit)
		fmt.Println("i3caller: the PSAP hung up:", why)
		return nil
	case <-deadline:
	case <-stop:
	}
	close(quit)
	c.bye()
	fmt.Println("i3caller: hung up")
	time.Sleep(300 * time.Millisecond)
	return nil
}

func geoHeader(byValue, ref string) string {
	if ref == "" {
		return byValue
	}
	return byValue + ", <" + ref + ">"
}

func (c *caller) fromHdr() string { return "<sip:" + c.o.from + "@esinet.lab;user=phone>;tag=" + c.tag }

func (c *caller) send(method, ruri string, cseq int, hdr map[string]string, body string) {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s SIP/2.0\r\n", method, ruri)
	fmt.Fprintf(&b, "Via: SIP/2.0/UDP %s;branch=z9hG4bK%s;rport\r\n", c.local, map[bool]string{true: c.callID, false: token(6)}[method == "CANCEL"])
	fmt.Fprintf(&b, "Max-Forwards: 70\r\nCall-ID: %s\r\nCSeq: %d %s\r\nContact: <sip:i3caller@%s>\r\nUser-Agent: bitdispatcher-i3caller\r\n", c.callID, cseq, method, c.local)
	for k, v := range hdr {
		fmt.Fprintf(&b, "%s: %s\r\n", k, v)
	}
	fmt.Fprintf(&b, "Content-Length: %d\r\n\r\n%s", len(body), body)
	if method == "INVITE" {
		c.inviteVia = fmt.Sprintf("SIP/2.0/UDP %s;branch=z9hG4bK%s;rport", c.local, c.callID)
		// the INVITE and its CANCEL share a branch
		s := b.String()
		s = regexp.MustCompile(`Via: [^\r]*`).ReplaceAllString(s, "Via: "+c.inviteVia)
		c.conn.WriteToUDP([]byte(s), c.psap)
		return
	}
	c.conn.WriteToUDP([]byte(b.String()), c.psap)
}

func (c *caller) read() {
	buf := make([]byte, 65535)
	for {
		n, from, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		msg := string(buf[:n])
		first, _, _ := strings.Cut(msg, "\r\n")
		if strings.TrimSpace(first) == "" {
			continue // a keep-alive
		}
		get := func(h string) string {
			m := regexp.MustCompile(`(?im)^` + h + `:\s*(.*)\r$`).FindStringSubmatch(msg)
			if m == nil {
				return ""
			}
			return m[1]
		}
		_, sdpPart, _ := strings.Cut(msg, "\r\n\r\n")
		switch {
		case strings.HasPrefix(first, "SIP/2.0 "):
			code, _ := strconv.Atoi(strings.Fields(first)[1])
			if !strings.Contains(get("CSeq"), "INVITE") {
				continue
			}
			switch {
			case code < 200:
				if code == 180 || code == 183 {
					fmt.Println("i3caller: ringing")
				}
			case code < 300:
				c.mu.Lock()
				to := get("To")
				c.toTag = to
				if m := regexp.MustCompile(`<([^>]+)>`).FindStringSubmatch(get("Contact")); m != nil {
					c.contact = m[1]
				}
				c.mu.Unlock()
				c.send("ACK", c.target(), 1, map[string]string{"From": c.fromHdr(), "To": to}, "")
				select {
				case c.answered <- sdpPart:
				default:
				}
			default:
				// a final failure: ACK it
				c.send("ACK", "urn:service:sos", 1, map[string]string{"From": c.fromHdr(), "To": get("To")}, "")
				if code != 487 {
					select {
					case c.ended <- first:
					default:
					}
				}
			}
		default:
			method := strings.Fields(first)[0]
			resp := "SIP/2.0 200 OK\r\n"
			for _, h := range []string{"Via", "From", "To", "Call-ID", "CSeq"} {
				for _, v := range regexp.MustCompile(`(?im)^`+h+`:\s*(.*)\r$`).FindAllStringSubmatch(msg, -1) {
					resp += h + ": " + v[1] + "\r\n"
				}
			}
			switch method {
			case "BYE":
				c.conn.WriteToUDP([]byte(resp+"Content-Length: 0\r\n\r\n"), from)
				select {
				case c.ended <- "BYE":
				default:
				}
			case "INVITE":
				// a re-INVITE (hold, transfer): keep the same media
				c.conn.WriteToUDP([]byte(resp+"Contact: <sip:i3caller@"+c.local+">\r\nContent-Length: 0\r\n\r\n"), from)
			case "ACK":
			default:
				c.conn.WriteToUDP([]byte(resp+"Content-Length: 0\r\n\r\n"), from)
			}
		}
	}
}

func (c *caller) target() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.contact != "" {
		return c.contact
	}
	return "urn:service:sos"
}

func (c *caller) bye() {
	c.mu.Lock()
	to := c.toTag
	c.cseq++
	n := c.cseq
	c.mu.Unlock()
	c.send("BYE", c.target(), n, map[string]string{"From": c.fromHdr(), "To": to}, "")
}

var reConn = regexp.MustCompile(`c=IN IP4 (\S+)`)
var reAudio = regexp.MustCompile(`m=audio (\d+)`)

// voice sends a soft 440 Hz tone, then silence, so the call has sound to record and transcribe.
func (c *caller) voice(rtp *net.UDPConn, answer string, quit chan struct{}) {
	ip, port := reConn.FindStringSubmatch(answer), reAudio.FindStringSubmatch(answer)
	if ip == nil || port == nil {
		fmt.Println("i3caller: no audio in the answer")
		return
	}
	p, _ := strconv.Atoi(port[1])
	dst := &net.UDPAddr{IP: net.ParseIP(ip[1]), Port: p}
	go func() { // read and drop what the PSAP sends
		b := make([]byte, 2048)
		for {
			if _, _, err := rtp.ReadFromUDP(b); err != nil {
				return
			}
		}
	}()
	seq, ts := uint16(time.Now().UnixNano()), uint32(0)
	t := time.NewTicker(20 * time.Millisecond)
	defer t.Stop()
	start := time.Now()
	for {
		select {
		case <-quit:
			return
		case <-t.C:
		}
		pkt := make([]byte, 12+160)
		pkt[0], pkt[1] = 0x80, 0
		binary.BigEndian.PutUint16(pkt[2:], seq)
		binary.BigEndian.PutUint32(pkt[4:], ts)
		binary.BigEndian.PutUint32(pkt[8:], 0x1e3ca11e)
		tone := time.Since(start) < 2*time.Second
		for j := 0; j < 160; j++ {
			var s int16
			switch {
			case c.o.speech != nil:
				// the recording, a second after the answer, then quiet (a little noise, so the line is not dead)
				if k := int(ts) + j - 8000; k >= 0 && k < len(c.o.speech) {
					s = c.o.speech[k]
				} else {
					s = int16(mrand.IntN(61) - 30)
				}
			case tone:
				s = int16(0.2 * math.Sin(2*math.Pi*440*float64(int(ts)+j)/8000) * 32767)
			}
			pkt[12+j] = linearToMuLaw(s)
		}
		rtp.WriteToUDP(pkt, dst)
		seq++
		ts += 160
	}
}

func linearToMuLaw(s int16) byte {
	const bias, clip = 0x84, 32635
	sign := byte(0)
	x := int(s)
	if x < 0 {
		x, sign = -x, 0x80
	}
	if x > clip {
		x = clip
	}
	x += bias
	exp := 7
	for mask := 0x4000; x&mask == 0 && exp > 0; mask >>= 1 {
		exp--
	}
	mant := (x >> (exp + 3)) & 0x0f
	return ^(sign | byte(exp<<4) | byte(mant))
}

var rePath = regexp.MustCompile(`a=path:(\S+)`)

// textChat connects to the PSAP's MSRP path, sends the first line and each line typed, and prints replies.
func (c *caller) textChat(ln net.Listener, answer string, quit chan struct{}) {
	m := rePath.FindStringSubmatch(answer)
	if m == nil {
		fmt.Println("i3caller: no MSRP path in the answer")
		return
	}
	to := m[1]
	hp := regexp.MustCompile(`msrp://([^/;]+)/`).FindStringSubmatch(to)
	if hp == nil {
		return
	}
	conn, err := net.Dial("tcp", hp[1])
	if err != nil {
		fmt.Println("i3caller: MSRP:", err)
		return
	}
	defer conn.Close()
	from := "msrp://" + ln.Addr().String() + "/" + c.tag + ";tcp"
	send := func(text string) {
		tid := token(5)
		n := len(text)
		fmt.Fprintf(conn, "MSRP %s SEND\r\nTo-Path: %s\r\nFrom-Path: %s\r\nMessage-ID: %s\r\nByte-Range: 1-%d/%d\r\nContent-Type: text/plain\r\n\r\n%s\r\n-------%s$\r\n", tid, to, from, tid, n, n, text, tid)
	}
	go func() {
		r := bufio.NewReader(conn)
		re := regexp.MustCompile(`(?s)MSRP (\S+) SEND\r\n.*?\r\n\r\n(.*?)\r\n-------(\S+)\$\r\n`)
		var buf string
		b := make([]byte, 4096)
		for {
			n, err := r.Read(b)
			if err != nil {
				return
			}
			buf += string(b[:n])
			for {
				mm := re.FindStringSubmatchIndex(buf)
				if mm == nil {
					break
				}
				tid, text := buf[mm[2]:mm[3]], buf[mm[4]:mm[5]]
				fmt.Println("911:", text)
				fmt.Fprintf(conn, "MSRP %s 200 OK\r\nTo-Path: %s\r\nFrom-Path: %s\r\n-------%s$\r\n", tid, from, to, tid)
				buf = buf[mm[1]:]
			}
		}
	}()
	send(c.o.text)
	lines := make(chan string)
	go func() {
		s := bufio.NewScanner(os.Stdin)
		for s.Scan() {
			lines <- s.Text()
		}
	}()
	for {
		select {
		case <-quit:
			return
		case l := <-lines:
			if strings.TrimSpace(l) != "" {
				send(l)
			}
		}
	}
}

// readWAV reads a 16-bit mono 8 kHz PCM WAV file's samples.
func readWAV(name string) ([]int16, error) {
	b, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, fmt.Errorf("%s is not a WAV file", name)
	}
	var fmtOK bool
	for i := 12; i+8 <= len(b); {
		id, n := string(b[i:i+4]), int(binary.LittleEndian.Uint32(b[i+4:]))
		body := b[i+8 : min(len(b), i+8+n)]
		switch id {
		case "fmt ":
			if len(body) < 16 || binary.LittleEndian.Uint16(body[0:]) != 1 || binary.LittleEndian.Uint16(body[2:]) != 1 ||
				binary.LittleEndian.Uint32(body[4:]) != 8000 || binary.LittleEndian.Uint16(body[14:]) != 16 {
				return nil, fmt.Errorf("%s: want 16-bit PCM, mono, 8000 Hz", name)
			}
			fmtOK = true
		case "data":
			if !fmtOK {
				return nil, fmt.Errorf("%s: no format before the data", name)
			}
			out := make([]int16, len(body)/2)
			for k := range out {
				out[k] = int16(binary.LittleEndian.Uint16(body[2*k:]))
			}
			return out, nil
		}
		i += 8 + n + n%2
	}
	return nil, fmt.Errorf("%s: no audio data", name)
}
