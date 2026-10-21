package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// A lab call-taker's phone: it registers as an extension (digest authentication) and answers each call
// after a few seconds, sending silence back, until the call ends. With it, a 911 call rings, is answered
// and is hung up as at a real position. With a transfer number it passes each answered call on (a blind
// transfer, SIP REFER) after a while, as a call-taker sending a caller to another position.

type phone struct {
	conn               *net.UDPConn
	server             *net.UDPAddr
	ext, secret, local string
	ringFor            time.Duration
	rtp                *net.UDPConn
	xferTo             string        // pass answered calls to this number ("": keep them)
	xferAfter          time.Duration // after this long
}

func md5hex(s string) string { h := md5.Sum([]byte(s)); return hex.EncodeToString(h[:]) }

var reAuth = regexp.MustCompile(`(\w+)="?([^",]*)"?`)

func digest(chal, method, uri, user, pass string) string {
	p := map[string]string{}
	for _, m := range reAuth.FindAllStringSubmatch(strings.TrimPrefix(chal, "Digest "), -1) {
		p[strings.ToLower(m[1])] = m[2]
	}
	ha1 := md5hex(user + ":" + p["realm"] + ":" + pass)
	ha2 := md5hex(method + ":" + uri)
	if strings.Contains(p["qop"], "auth") {
		nc, cn := "00000001", token(4)
		resp := md5hex(ha1 + ":" + p["nonce"] + ":" + nc + ":" + cn + ":auth:" + ha2)
		return fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s", algorithm=MD5, qop=auth, nc=%s, cnonce="%s"%s`,
			user, p["realm"], p["nonce"], uri, resp, nc, cn, opaque(p))
	}
	return fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s", algorithm=MD5%s`, user, p["realm"], p["nonce"], uri, md5hex(ha1+":"+p["nonce"]+":"+ha2), opaque(p))
}

func opaque(p map[string]string) string {
	if p["opaque"] == "" {
		return ""
	}
	return `, opaque="` + p["opaque"] + `"`
}

func header(msg, h string) string {
	m := regexp.MustCompile(`(?im)^` + h + `:\s*(.*)\r$`).FindStringSubmatch(msg)
	if m == nil {
		return ""
	}
	return m[1]
}

func runPhone(server, bind, ext, secret string, ringFor time.Duration, xferTo string, xferAfter time.Duration) error {
	srv, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(bind)})
	if err != nil {
		return err
	}
	rtp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(bind)})
	if err != nil {
		return err
	}
	p := &phone{conn: conn, server: srv, ext: ext, secret: secret, local: conn.LocalAddr().String(), ringFor: ringFor, rtp: rtp, xferTo: xferTo, xferAfter: xferAfter}
	go func() { // drop the caller's sound
		b := make([]byte, 2048)
		for {
			if _, _, err := rtp.ReadFromUDP(b); err != nil {
				return
			}
		}
	}()
	callID, tag := "i3p-"+token(8), token(4)
	domain := srv.IP.String()
	reg := func(cseq int, auth string) {
		var b strings.Builder
		fmt.Fprintf(&b, "REGISTER sip:%s SIP/2.0\r\nVia: SIP/2.0/UDP %s;branch=z9hG4bK%s;rport\r\nMax-Forwards: 70\r\n", domain, p.local, token(6))
		fmt.Fprintf(&b, "From: <sip:%s@%s>;tag=%s\r\nTo: <sip:%s@%s>\r\nCall-ID: %s\r\nCSeq: %d REGISTER\r\n", ext, domain, tag, ext, domain, callID, cseq)
		fmt.Fprintf(&b, "Contact: <sip:%s@%s>\r\nExpires: 300\r\nUser-Agent: bitdispatcher-i3caller-phone\r\n", ext, p.local)
		if auth != "" {
			fmt.Fprintf(&b, "Authorization: %s\r\n", auth)
		}
		b.WriteString("Content-Length: 0\r\n\r\n")
		conn.WriteToUDP([]byte(b.String()), srv)
	}
	cseq := 1
	reg(cseq, "")
	go func() {
		for range time.Tick(240 * time.Second) {
			cseq++
			reg(cseq, "")
		}
	}()
	fmt.Printf("phone: registering %s at %s\n", ext, server)
	buf := make([]byte, 65535)
	calls := map[string]chan struct{}{}
	refers := map[string]func(hdr, chal string){} // Call-ID: send the REFER again, answering the challenge
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
			code, _ := strconv.Atoi(strings.Fields(first)[1])
			if strings.Contains(header(msg, "CSeq"), "REFER") {
				if again := refers[header(msg, "Call-ID")]; again != nil && (code == 401 || code == 407) {
					hdr, chal := "Authorization", header(msg, "WWW-Authenticate")
					if code == 407 {
						hdr, chal = "Proxy-Authorization", header(msg, "Proxy-Authenticate")
					}
					delete(refers, header(msg, "Call-ID"))
					again(hdr, chal)
				} else if code >= 200 {
					fmt.Println("phone: transfer:", strings.TrimSpace(first))
				}
				continue
			}
			if strings.Contains(header(msg, "CSeq"), "REGISTER") {
				switch {
				case code == 401 || code == 407:
					chal := header(msg, "WWW-Authenticate")
					if chal == "" {
						chal = header(msg, "Proxy-Authenticate")
					}
					cseq++
					reg(cseq, digest(chal, "REGISTER", "sip:"+domain, ext, secret))
				case code == 200:
					fmt.Println("phone: registered")
				default:
					fmt.Println("phone: register:", first)
				}
			}
			continue
		}
		method := strings.Fields(first)[0]
		id := header(msg, "Call-ID")
		resp := func(status, extra, body string) {
			var b strings.Builder
			b.WriteString("SIP/2.0 " + status + "\r\n")
			for _, h := range []string{"Via", "From", "To", "Call-ID", "CSeq", "Record-Route"} {
				for _, v := range regexp.MustCompile(`(?im)^`+h+`:\s*(.*)\r$`).FindAllStringSubmatch(msg, -1) {
					val := v[1]
					if h == "To" && !strings.Contains(val, "tag=") && !strings.HasPrefix(status, "100") {
						val += ";tag=" + tag
					}
					b.WriteString(h + ": " + val + "\r\n")
				}
			}
			b.WriteString(extra)
			fmt.Fprintf(&b, "Content-Length: %d\r\n\r\n%s", len(body), body)
			conn.WriteToUDP([]byte(b.String()), from)
		}
		switch method {
		case "INVITE":
			if _, ok := calls[id]; ok { // a re-INVITE
				resp("200 OK", "Contact: <sip:"+ext+"@"+p.local+">\r\n", "")
				continue
			}
			done := make(chan struct{})
			calls[id] = done
			resp("180 Ringing", "Contact: <sip:"+ext+"@"+p.local+">\r\n", "")
			_, offer, _ := strings.Cut(msg, "\r\n\r\n")
			fmt.Printf("phone: ringing: %s\n", header(msg, "From"))
			go func(msg, offer string) {
				select {
				case <-done:
					return
				case <-time.After(p.ringFor):
				}
				port := p.rtp.LocalAddr().(*net.UDPAddr).Port
				host, _, _ := net.SplitHostPort(p.local)
				sdpAns := "v=0\r\no=phone 1 1 IN IP4 " + host + "\r\ns=-\r\nc=IN IP4 " + host + "\r\nt=0 0\r\nm=audio " + strconv.Itoa(port) + " RTP/AVP 0 101\r\na=rtpmap:0 PCMU/8000\r\na=rtpmap:101 telephone-event/8000\r\na=sendrecv\r\n"
				resp("200 OK", "Contact: <sip:"+ext+"@"+p.local+">\r\nContent-Type: application/sdp\r\n", sdpAns)
				fmt.Println("phone: answered")
				if p.xferTo != "" {
					go func() {
						select {
						case <-done:
							return
						case <-time.After(p.xferAfter):
						}
						fmt.Println("phone: transferring to", p.xferTo)
						p.refer(msg, tag, func(again func(hdr, chal string)) { refers[header(msg, "Call-ID")] = again })
					}()
				}
				if ip, pt := reConn.FindStringSubmatch(offer), reAudio.FindStringSubmatch(offer); ip != nil && pt != nil {
					n, _ := strconv.Atoi(pt[1])
					dst := &net.UDPAddr{IP: net.ParseIP(ip[1]), Port: n}
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
						p.rtp.WriteToUDP(pkt, dst)
					}
				}
			}(msg, offer)
		case "ACK":
		case "BYE", "CANCEL":
			resp("200 OK", "", "")
			if d, ok := calls[id]; ok {
				close(d)
				delete(calls, id)
				fmt.Println("phone: call ended")
			}
		default:
			resp("200 OK", "", "")
		}
	}
}

var reURI = regexp.MustCompile(`<([^>]+)>`)

// refer sends a blind transfer (REFER) inside the dialog the INVITE invite began, which this phone
// answered with the tag tag. keep is given a function that sends it again with a challenge's answer.
func (p *phone) refer(invite, tag string, keep func(func(hdr, chal string))) {
	target := p.server.String()
	if m := reURI.FindStringSubmatch(header(invite, "Contact")); m != nil {
		target = m[1]
	}
	if !strings.HasPrefix(target, "sip:") {
		target = "sip:" + target
	}
	referTo := "sip:" + p.xferTo + "@" + p.server.IP.String()
	send := func(cseq int, hdr, auth string) {
		var b strings.Builder
		fmt.Fprintf(&b, "REFER %s SIP/2.0\r\nVia: SIP/2.0/UDP %s;branch=z9hG4bK%s;rport\r\nMax-Forwards: 70\r\n", target, p.local, token(6))
		fmt.Fprintf(&b, "From: %s;tag=%s\r\nTo: %s\r\nCall-ID: %s\r\nCSeq: %d REFER\r\n", header(invite, "To"), tag, header(invite, "From"), header(invite, "Call-ID"), cseq)
		fmt.Fprintf(&b, "Contact: <sip:%s@%s>\r\nRefer-To: <%s>\r\nReferred-By: <sip:%s@%s>\r\n", p.ext, p.local, referTo, p.ext, p.server.IP.String())
		if auth != "" {
			fmt.Fprintf(&b, "%s: %s\r\n", hdr, auth)
		}
		b.WriteString("Content-Length: 0\r\n\r\n")
		p.conn.WriteToUDP([]byte(b.String()), p.server)
	}
	keep(func(hdr, chal string) { send(2, hdr, digest(chal, "REFER", target, p.ext, p.secret)) })
	send(1, "", "")
}
