package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A lab PSAP on the ESInet: it answers the 911 calls an ESRP routes to it (sending silence back until
// the call ends) and tells subscribers its NENA states (NENA-STA-010.3 emergency-QueueState and
// emergency-ServiceState, by SIP SUBSCRIBE and NOTIFY). With its queue Full or its service Down, an
// ESRP watching it passes it over for its overflow PSAP.

type site struct {
	conn        *net.UDPConn
	local, name string
	queue, svc  string
	rtp         *net.UDPConn
	answerAfter time.Duration
	calls       map[string]chan struct{}
	tag         string
	mu          sync.Mutex
	subs        map[string]subscriber // Call-ID: who subscribed, to be told when the state changes
}

type subscriber struct {
	Msg  string `json:"msg"` // its SUBSCRIBE
	From string `json:"from"`
	Ev   string `json:"ev"`
}

// subsFile keeps the subscribers across a restart, so a changed state still reaches them.
func (s *site) subsFile() string {
	return "psap-" + strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, s.name) + ".subs.json"
}

func (s *site) saveSubs() {
	b, _ := json.Marshal(s.subs)
	_ = os.WriteFile(s.subsFile(), b, 0o600)
}

// tell sends every subscriber the current state.
func (s *site) tell() {
	for _, sb := range s.subs {
		to, err := net.ResolveUDPAddr("udp", sb.From)
		if err != nil {
			continue
		}
		if body, ct := s.stateBody(sb.Ev); body != "" {
			s.notify(sb.Msg, to, sb.Ev, ct, body, "active;expires="+subExpires)
		}
	}
}

// subExpires is how long a subscription lasts before the subscriber renews it.
const subExpires = "300"

var reHdr = map[string]*regexp.Regexp{}

func hdrAll(msg, h string) []string {
	re, ok := reHdr[h]
	if !ok {
		re = regexp.MustCompile(`(?im)^` + h + `:\s*(.*)\r$`)
		reHdr[h] = re
	}
	var out []string
	for _, m := range re.FindAllStringSubmatch(msg, -1) {
		out = append(out, m[1])
	}
	return out
}

func runSite(addr, name, queue, svc string, answerAfter time.Duration) error {
	ua, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", ua)
	if err != nil {
		return err
	}
	rtp, err := net.ListenUDP("udp", &net.UDPAddr{IP: ua.IP})
	if err != nil {
		return err
	}
	s := &site{conn: conn, local: conn.LocalAddr().String(), name: name, queue: queue, svc: svc, rtp: rtp, answerAfter: answerAfter,
		calls: map[string]chan struct{}{}, tag: token(4), subs: map[string]subscriber{}}
	// SIGUSR1 switches the queue between Active and Full and tells the subscribers, as a PSAP whose
	// call-takers are all busy and then free again.
	sig := make(chan os.Signal, 1)
	notifyQueueToggle(sig)
	go func() {
		for range sig {
			s.mu.Lock()
			if s.queue == "Full" {
				s.queue = "Active"
			} else {
				s.queue = "Full"
			}
			fmt.Printf("psap: %s: queue now %s, telling %d subscriptions\n", s.name, s.queue, len(s.subs))
			s.tell()
			s.mu.Unlock()
		}
	}()
	go func() {
		b := make([]byte, 2048)
		for {
			if _, _, err := rtp.ReadFromUDP(b); err != nil {
				return
			}
		}
	}()
	fmt.Printf("psap: %s on %s, queue %s, service %s\n", name, s.local, queue, svc)
	if b, err := os.ReadFile(s.subsFile()); err == nil && json.Unmarshal(b, &s.subs) == nil && len(s.subs) > 0 {
		s.mu.Lock()
		fmt.Printf("psap: telling the %d subscriptions it had before\n", len(s.subs))
		s.tell()
		s.mu.Unlock()
	}
	buf := make([]byte, 65535)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			return err
		}
		msg := string(buf[:n])
		first, _, _ := strings.Cut(msg, "\r\n")
		if strings.TrimSpace(first) == "" {
			continue // a keep-alive
		}
		if strings.HasPrefix(first, "SIP/2.0 ") {
			if !strings.HasPrefix(first, "SIP/2.0 2") {
				fmt.Printf("psap: %s: %s answered %s\n", s.name, strings.TrimPrefix(first, "SIP/2.0 "), header(msg, "CSeq"))
			}
			continue // the answers to its NOTIFYs
		}
		s.handle(strings.Fields(first)[0], msg, from)
	}
}

// respond answers a request, its To tagged with this site's tag.
func (s *site) respond(msg string, to *net.UDPAddr, status, extra, body string) {
	var b strings.Builder
	b.WriteString("SIP/2.0 " + status + "\r\n")
	for _, h := range []string{"Via", "From", "To", "Call-ID", "CSeq", "Record-Route"} {
		for _, v := range hdrAll(msg, h) {
			if h == "To" && !strings.Contains(v, "tag=") && !strings.HasPrefix(status, "100") {
				v += ";tag=" + s.tag
			}
			b.WriteString(h + ": " + v + "\r\n")
		}
	}
	b.WriteString(extra)
	fmt.Fprintf(&b, "Content-Length: %d\r\n\r\n%s", len(body), body)
	s.conn.WriteToUDP([]byte(b.String()), to)
}

func (s *site) handle(method, msg string, from *net.UDPAddr) {
	id := header(msg, "Call-ID")
	contact := "Contact: <sip:psap@" + s.local + ">\r\n"
	switch method {
	case "SUBSCRIBE":
		ev := strings.TrimSpace(strings.SplitN(header(msg, "Event"), ";", 2)[0])
		expires := header(msg, "Expires")
		if n, err := strconv.Atoi(expires); err != nil || n > 300 {
			expires = subExpires
		}
		s.mu.Lock()
		body, ct := s.stateBody(ev)
		s.mu.Unlock()
		if body == "" {
			s.respond(msg, from, "489 Bad Event", "", "")
			return
		}
		s.respond(msg, from, "200 OK", contact+"Expires: "+expires+"\r\n", "")
		state := "active;expires=" + expires
		if expires == "0" {
			state = "terminated;reason=timeout"
		}
		s.notify(msg, from, ev, ct, body, state)
		s.mu.Lock()
		if expires == "0" {
			delete(s.subs, id)
		} else {
			s.subs[id] = subscriber{Msg: msg, From: from.String(), Ev: ev}
		}
		s.saveSubs()
		s.mu.Unlock()
		fmt.Printf("psap: %s subscribed to %s: told it %s\n", from, ev, body)
	case "INVITE":
		if _, ok := s.calls[id]; ok {
			s.respond(msg, from, "200 OK", contact, "")
			return
		}
		done := make(chan struct{})
		s.calls[id] = done
		s.respond(msg, from, "180 Ringing", contact, "")
		fmt.Printf("psap: %s: a 911 call from %s\n", s.name, header(msg, "From"))
		_, offer, _ := strings.Cut(msg, "\r\n\r\n")
		go func() {
			select {
			case <-done:
				return
			case <-time.After(s.answerAfter):
			}
			host, _, _ := net.SplitHostPort(s.local)
			port := s.rtp.LocalAddr().(*net.UDPAddr).Port
			sdp := "v=0\r\no=psap 1 1 IN IP4 " + host + "\r\ns=-\r\nc=IN IP4 " + host + "\r\nt=0 0\r\nm=audio " + strconv.Itoa(port) + " RTP/AVP 0\r\na=rtpmap:0 PCMU/8000\r\na=sendrecv\r\n"
			s.respond(msg, from, "200 OK", contact+"Content-Type: application/sdp\r\n", sdp)
			fmt.Printf("psap: %s answered it\n", s.name)
			ip, pt := reConn.FindStringSubmatch(offer), reAudio.FindStringSubmatch(offer)
			if ip == nil || pt == nil {
				return
			}
			p, _ := strconv.Atoi(pt[1])
			dst := &net.UDPAddr{IP: net.ParseIP(ip[1]), Port: p}
			t := time.NewTicker(20 * time.Millisecond)
			defer t.Stop()
			seq, ts := uint16(1), uint32(0)
			for {
				select {
				case <-done:
					return
				case <-t.C:
				}
				pkt := make([]byte, 172)
				pkt[0] = 0x80
				seq++
				ts += 160
				pkt[2], pkt[3] = byte(seq>>8), byte(seq)
				pkt[4], pkt[5], pkt[6], pkt[7] = byte(ts>>24), byte(ts>>16), byte(ts>>8), byte(ts)
				for j := 12; j < 172; j++ {
					pkt[j] = 0xff
				}
				s.rtp.WriteToUDP(pkt, dst)
			}
		}()
	case "BYE", "CANCEL":
		s.respond(msg, from, "200 OK", "", "")
		if d, ok := s.calls[id]; ok {
			close(d)
			delete(s.calls, id)
			fmt.Printf("psap: %s: the call ended\n", s.name)
		}
	case "ACK":
	default:
		s.respond(msg, from, "200 OK", "", "")
	}
}

// stateBody is the site's state for an event, and its content type ("" for an event it does not offer).
func (s *site) stateBody(ev string) (string, string) {
	var v any
	var ct string
	switch strings.ToLower(ev) {
	case "emergency-queuestate":
		max := 10
		length := 0
		if s.queue == "Full" {
			length = max
		}
		v = map[string]any{"queueState": map[string]any{"queueUri": "sip:sos@" + s.local, "queueLength": length, "queueMaxLength": max, "state": s.queue}}
		ct = "application/EmergencyCallData.queuestate+json"
	case "emergency-servicestate":
		v = map[string]any{"service": map[string]any{"name": "PSAP", "domain": "lab.invalid", "serviceId": s.name},
			"serviceState": map[string]any{"state": s.svc, "reason": "lab"}, "securityPosture": map[string]any{"posture": "Green"}}
		ct = "application/EmergencyCallData.ServiceState+json"
	default:
		return "", ""
	}
	b, _ := json.Marshal(v)
	return string(b), ct
}

// notify sends the subscriber a NOTIFY in the subscription's dialog.
func (s *site) notify(sub string, to *net.UDPAddr, ev, ct, body, state string) {
	target := to.String()
	if m := reURI.FindStringSubmatch(header(sub, "Contact")); m != nil {
		target = strings.TrimPrefix(m[1], "sip:")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "NOTIFY sip:%s SIP/2.0\r\nVia: SIP/2.0/UDP %s;branch=z9hG4bK%s;rport\r\nMax-Forwards: 70\r\n", target, s.local, token(6))
	from := regexp.MustCompile(`;tag=[^;]*`).ReplaceAllString(header(sub, "To"), "")
	fmt.Fprintf(&b, "From: %s;tag=%s\r\nTo: %s\r\nCall-ID: %s\r\nCSeq: %d NOTIFY\r\n", from, s.tag, header(sub, "From"), header(sub, "Call-ID"), time.Now().Unix()%100000)
	fmt.Fprintf(&b, "Contact: <sip:psap@%s>\r\nEvent: %s\r\nSubscription-State: %s\r\nContent-Type: %s\r\n", s.local, ev, state, ct)
	fmt.Fprintf(&b, "Content-Length: %d\r\n\r\n%s", len(body), body)
	s.conn.WriteToUDP([]byte(b.String()), to)
}
