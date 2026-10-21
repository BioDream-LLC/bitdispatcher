package main

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// A lab stand-in for a P25 radio system over the ISSI (TIA-102.BACA): it answers the console's
// talkgroup call as the home RFSS would, then a radio unit makes a private call to the console, with the
// emergency flag when asked. It carries no voice: enough for a CAD to see who called and the emergency.

func runRadio(listen, console, wacn, system string, consoleUnit, unit int, emergency bool, after time.Duration) error {
	conn, err := net.ListenUDP("udp", mustUDP(listen))
	if err != nil {
		return err
	}
	rtp, err := net.ListenUDP("udp", &net.UDPAddr{IP: conn.LocalAddr().(*net.UDPAddr).IP})
	if err != nil {
		return err
	}
	go func() {
		b := make([]byte, 2048)
		for {
			if _, _, err := rtp.ReadFromUDP(b); err != nil {
				return
			}
		}
	}()
	host := conn.LocalAddr().(*net.UDPAddr).IP.String()
	rport := rtp.LocalAddr().(*net.UDPAddr).Port
	sdpBody := func() string {
		return "v=0\r\no=rfss 1 1 IN IP4 " + host + "\r\ns=-\r\nc=IN IP4 " + host + "\r\nt=0 0\r\nm=audio " + strconv.Itoa(rport) + " RTP/AVP 100\r\na=rtpmap:100 X-TIA-P25-IMBE/8000\r\na=sendrecv\r\n"
	}
	id := func(u int) string { return fmt.Sprintf("%05x%03x%06x", hexOf(wacn), hexOf(system), u) }
	fmt.Printf("radio: home RFSS at %s; radio %d calls console %d%s after %s\n", conn.LocalAddr(), unit, consoleUnit, map[bool]string{true: " with EMERGENCY", false: ""}[emergency], after)
	go func() {
		time.Sleep(after)
		dst := mustUDP(console)
		callID, tag := "p25-"+token(8), token(4)
		body := sdpBody()
		pri := "normal"
		if emergency {
			pri = "emergency;e"
		}
		msg := fmt.Sprintf("INVITE sip:%s@%s;user=TIA-P25-SU SIP/2.0\r\nVia: SIP/2.0/UDP %s;branch=z9hG4bK%s;rport\r\nMax-Forwards: 70\r\n"+
			"From: <sip:%s@p25dr;user=TIA-P25-SU>;tag=%s\r\nTo: <sip:%s@p25dr;user=TIA-P25-SU>\r\nCall-ID: %s\r\nCSeq: 1 INVITE\r\n"+
			"Contact: <sip:%s@%s;user=TIA-P25-SU>\r\nPriority: %s\r\nContent-Type: application/sdp\r\nContent-Length: %d\r\n\r\n%s",
			id(consoleUnit), console, conn.LocalAddr(), token(6), id(unit), tag, id(consoleUnit), callID, id(unit), conn.LocalAddr(), pri, len(body), body)
		conn.WriteToUDP([]byte(msg), dst)
		fmt.Println("radio: private call sent")
	}()
	buf := make([]byte, 65535)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			return err
		}
		msg := string(buf[:n])
		first, _, _ := strings.Cut(msg, "\r\n")
		if strings.TrimSpace(first) == "" {
			continue
		}
		if strings.HasPrefix(first, "SIP/2.0 ") {
			fmt.Println("radio: console answered:", first)
			if strings.Contains(header(msg, "CSeq"), "INVITE") && strings.Contains(first, " 200 ") {
				ack := fmt.Sprintf("ACK sip:%s SIP/2.0\r\nVia: SIP/2.0/UDP %s;branch=z9hG4bK%s\r\nFrom: %s\r\nTo: %s\r\nCall-ID: %s\r\nCSeq: 1 ACK\r\nContent-Length: 0\r\n\r\n",
					console, conn.LocalAddr(), token(6), header(msg, "From"), header(msg, "To"), header(msg, "Call-ID"))
				conn.WriteToUDP([]byte(ack), from)
			}
			continue
		}
		method := strings.Fields(first)[0]
		var b strings.Builder
		b.WriteString("SIP/2.0 200 OK\r\n")
		for _, h := range []string{"Via", "From", "To", "Call-ID", "CSeq"} {
			for _, v := range regexp.MustCompile(`(?im)^`+h+`:\s*(.*)\r$`).FindAllStringSubmatch(msg, -1) {
				val := v[1]
				if h == "To" && !strings.Contains(val, "tag=") {
					val += ";tag=" + token(4)
				}
				b.WriteString(h + ": " + val + "\r\n")
			}
		}
		if method == "INVITE" {
			fmt.Println("radio: the console joined a talkgroup:", strings.Fields(first)[1])
			body := sdpBody()
			fmt.Fprintf(&b, "Contact: <sip:rfss@%s>\r\nContent-Type: application/sdp\r\nContent-Length: %d\r\n\r\n%s", conn.LocalAddr(), len(body), body)
		} else {
			b.WriteString("Content-Length: 0\r\n\r\n")
		}
		if method != "ACK" {
			conn.WriteToUDP([]byte(b.String()), from)
		}
	}
}

func hexOf(s string) uint64 { v, _ := strconv.ParseUint(s, 16, 64); return v }

func mustUDP(a string) *net.UDPAddr {
	u, err := net.ResolveUDPAddr("udp", a)
	if err != nil {
		panic(err)
	}
	return u
}
