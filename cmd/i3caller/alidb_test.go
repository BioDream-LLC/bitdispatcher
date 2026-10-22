package main

import (
	"net"
	"strings"
	"testing"
	"time"
)

// field reads a record field as a PSAP's layout places it: line and column from 1, length 0 = to the end.
func field(rec string, line, col, n int) string {
	ls := strings.Split(rec, "\r\n")
	l := ls[line-1]
	if col-1 >= len(l) {
		return ""
	}
	l = l[col-1:]
	if n > 0 && n < len(l) {
		l = l[:n]
	}
	return strings.TrimSpace(l)
}

// The record's fields sit where the layout in alidb.go says.
func TestALIRecordLayout(t *testing.T) {
	rec := aliRecord("5405550341")
	for _, f := range []struct {
		line, col, n int
		want         string
	}{
		{1, 1, 14, "(540) 555-0341"}, {1, 17, 4, "RESD"}, {2, 1, 0, "LAB SUBSCRIBER 0341"}, {3, 1, 0, "1910 WILLIAMSON RD NE"},
		{4, 1, 18, "ROANOKE"}, {4, 20, 2, "VA"}, {5, 5, 11, "+37.29450"}, {5, 21, 12, "-079.93850"},
		{6, 5, 4, "15M"}, {6, 14, 4, "101"}, {6, 19, 0, "LAB TELCO"},
	} {
		if got := field(rec, f.line, f.col, f.n); got != f.want {
			t.Errorf("line %d col %d: %q, want %q\n%s", f.line, f.col, got, f.want, rec)
		}
	}
	if !strings.HasPrefix(aliRecord("5405550000"), "NO RECORD FOUND") {
		t.Error("a number ending 0000 has no record")
	}
}

// A query over TCP: STX, ten digits, ETX in; the record framed the same way out.
func TestALIDBAnswers(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	go runALIDB(addr)
	var c net.Conn
	for i := 0; i < 50; i++ {
		if c, err = net.Dial("tcp", addr); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Write([]byte("\x025405550341\x03"))
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	var got []byte
	buf := make([]byte, 1024)
	for !strings.Contains(string(got), "\x03") {
		n, err := c.Read(buf)
		if err != nil {
			t.Fatalf("%v after %q", err, got)
		}
		got = append(got, buf[:n]...)
	}
	if !strings.HasPrefix(string(got), "\x02(540) 555-0341") || !strings.HasSuffix(string(got), "LAB TELCO\x03") {
		t.Fatalf("%q", got)
	}
}

// The lab PSAP's states, in NENA's JSON, for the two events it offers.
func TestSiteStates(t *testing.T) {
	s := &site{name: "PSAP A", local: "127.0.0.1:5070", queue: "Full", svc: "Normal"}
	b, ct := s.stateBody("emergency-QueueState")
	if ct != "application/EmergencyCallData.queuestate+json" || !strings.Contains(b, `"state":"Full"`) || !strings.Contains(b, `"queueLength":10`) {
		t.Fatalf("%s %s", ct, b)
	}
	if b, _ := s.stateBody("emergency-ServiceState"); !strings.Contains(b, `"serviceState":{"reason":"lab","state":"Normal"}`) {
		t.Fatal(b)
	}
	if b, _ := s.stateBody("emergency-ElementState"); b != "" {
		t.Fatal("an event it does not offer")
	}
}
