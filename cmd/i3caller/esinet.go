package main

import (
	"fmt"
	"log"
	"net"
	"strings"
)

// runESInet stands in for the ESInet's border at a trunk's address: it answers the call-handling
// system's OPTIONS keepalives with 200 OK, so the trunk shows as connected, and refuses anything else
// sent to it (calls come from i3caller itself, not from here).
func runESInet(listen string) error {
	conn, err := net.ListenUDP("udp", mustUDP(listen))
	if err != nil {
		return err
	}
	log.Printf("ESInet stand-in answering OPTIONS on %s", conn.LocalAddr())
	buf := make([]byte, 65535)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			return err
		}
		msg := string(buf[:n])
		line, _, _ := strings.Cut(msg, "\r\n")
		f := strings.Fields(line)
		if len(f) < 3 || strings.HasPrefix(line, "SIP/2.0") || f[0] == "ACK" {
			continue // a response, or an ACK: nothing to answer
		}
		code, reason := "200", "OK"
		if f[0] != "OPTIONS" {
			code, reason = "405", "Method Not Allowed"
		}
		to := header(msg, "To")
		if !strings.Contains(to, ";tag=") {
			to += ";tag=" + token(8)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "SIP/2.0 %s %s\r\n", code, reason)
		for _, v := range viaHeaders(msg) {
			fmt.Fprintf(&b, "Via: %s\r\n", v)
		}
		fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nCall-ID: %s\r\nCSeq: %s\r\n", header(msg, "From"), to, header(msg, "Call-ID"), header(msg, "CSeq"))
		b.WriteString("Allow: OPTIONS\r\nAccept: application/sdp\r\nUser-Agent: i3caller ESInet stand-in\r\nContent-Length: 0\r\n\r\n")
		conn.WriteToUDP([]byte(b.String()), from)
	}
}

// viaHeaders returns every Via of a request, in order, for its response.
func viaHeaders(msg string) []string {
	head, _, _ := strings.Cut(msg, "\r\n\r\n")
	var out []string
	for _, l := range strings.Split(head, "\r\n") {
		k, v, ok := strings.Cut(l, ":")
		if ok && (strings.EqualFold(strings.TrimSpace(k), "Via") || strings.TrimSpace(k) == "v") {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}
