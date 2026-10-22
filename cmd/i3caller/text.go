package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Text on a voice call, three ways a 911 caller can type: a TTY's tones on the voice itself (5-bit
// Baudot, 45.45 bit/s, mark 1400 Hz, space 1800 Hz), real-time text beside the voice (RFC 4103,
// T.140 over RTP, a character at a time), and SIP MESSAGE inside the call.

// The US TTY code: letters and figures share the 5-bit codes, LTRS and FIGS switch between them.
const (
	ttyLTRS  = 0x1f
	ttyFIGS  = 0x1b
	ttySpace = 0x04
	ttyCR    = 0x08
	ttyLF    = 0x02
)

var ttyLetters = map[rune]byte{
	'E': 0x01, 'A': 0x03, 'S': 0x05, 'I': 0x06, 'U': 0x07, 'D': 0x09, 'R': 0x0a, 'J': 0x0b, 'N': 0x0c, 'F': 0x0d,
	'C': 0x0e, 'K': 0x0f, 'T': 0x10, 'Z': 0x11, 'L': 0x12, 'W': 0x13, 'H': 0x14, 'Y': 0x15, 'P': 0x16, 'Q': 0x17,
	'O': 0x18, 'B': 0x19, 'G': 0x1a, 'M': 0x1c, 'X': 0x1d, 'V': 0x1e,
}

// Figures common to the US TTY and ITA2 (the punctuation that differs between them is left out).
var ttyFigures = map[rune]byte{
	'3': 0x01, '-': 0x03, '8': 0x06, '7': 0x07, '4': 0x0a, ',': 0x0c, '5': 0x10, '2': 0x13, '6': 0x15, '0': 0x16,
	'1': 0x17, '9': 0x18, '?': 0x19, '.': 0x1c,
}

const (
	ttyRate    = 8000.0
	ttyBit     = ttyRate / 45.45 // samples a bit
	ttyMark    = 1400.0
	ttySpaceHz = 1800.0
	ttyLevel   = 9000.0
)

// ttyTones is what a TTY sends for text: a moment of mark, then each character as a start bit (space),
// five data bits (least significant first, 1 = mark) and one and a half stop bits (mark), switching
// between letters and figures when it must. Characters a TTY cannot send are dropped.
func ttyTones(text string) []int16 {
	var out []int16
	phase := 0.0
	tone := func(hz, n float64) {
		for i := 0; i < int(math.Round(n)); i++ {
			out = append(out, int16(ttyLevel*math.Sin(phase)))
			phase += 2 * math.Pi * hz / ttyRate
		}
	}
	char := func(code byte) {
		tone(ttySpaceHz, ttyBit)
		for b := 0; b < 5; b++ {
			if code>>b&1 == 1 {
				tone(ttyMark, ttyBit)
			} else {
				tone(ttySpaceHz, ttyBit)
			}
		}
		tone(ttyMark, 1.5*ttyBit)
	}
	tone(ttyMark, 0.15*ttyRate) // the carrier before the first character
	figs := false
	char(ttyLTRS)
	for _, r := range strings.ToUpper(text) {
		switch {
		case r == ' ':
			char(ttySpace)
		case r == '\n':
			char(ttyCR)
			char(ttyLF)
		default:
			if c, ok := ttyLetters[r]; ok {
				if figs {
					char(ttyLTRS)
					figs = false
				}
				char(c)
			} else if c, ok := ttyFigures[r]; ok {
				if !figs {
					char(ttyFIGS)
					figs = true
				}
				char(c)
			}
		}
	}
	tone(ttyMark, 0.15*ttyRate)
	return out
}

// The real-time text stream: T.140 in RTP, payload type rttPT, without redundancy.
const rttPT = 98

var reText = regexp.MustCompile(`m=text (\d+)`)

// rttOffer is the SDP media line offering real-time text on port.
func rttOffer(port int) string {
	return "m=text " + strconv.Itoa(port) + " RTP/AVP " + strconv.Itoa(rttPT) + "\r\na=rtpmap:" + strconv.Itoa(rttPT) + " t140/1000\r\na=sendrecv\r\n"
}

// rtt types text into the PSAP's text stream a character at a time, as a person types, and prints
// what the call-taker types back.
func (c *caller) rtt(conn *net.UDPConn, answer, text string, quit chan struct{}) {
	ip, port := reConn.FindStringSubmatch(answer), reText.FindStringSubmatch(answer)
	if ip == nil || port == nil || port[1] == "0" {
		fmt.Println("i3caller: the PSAP did not take real-time text")
		return
	}
	p, _ := strconv.Atoi(port[1])
	dst := &net.UDPAddr{IP: net.ParseIP(ip[1]), Port: p}
	go func() {
		b := make([]byte, 2048)
		var line strings.Builder
		for {
			n, _, err := conn.ReadFromUDP(b)
			if err != nil {
				return
			}
			if n <= 12 || int(b[1]&0x7f) != rttPT {
				continue
			}
			for _, r := range string(b[12:n]) {
				switch r {
				case '\u2028', '\n', '\r':
					if line.Len() > 0 {
						fmt.Println("i3caller: the call-taker typed:", line.String())
						line.Reset()
					}
				case '\b':
				case '\ufeff':
				default:
					line.WriteRune(r)
				}
			}
		}
	}()
	select {
	case <-quit:
		return
	case <-time.After(2 * time.Second):
	}
	seq, start := uint16(time.Now().UnixNano()), time.Now()
	first := true
	for _, r := range text + "\u2028" {
		select {
		case <-quit:
			return
		case <-time.After(120 * time.Millisecond):
		}
		pkt := make([]byte, 12, 16)
		pkt[0], pkt[1] = 0x80, rttPT
		if first {
			pkt[1] |= 0x80 // the marker: the start of the text
			first = false
		}
		binary.BigEndian.PutUint16(pkt[2:], seq)
		binary.BigEndian.PutUint32(pkt[4:], uint32(time.Since(start).Milliseconds()))
		binary.BigEndian.PutUint32(pkt[8:], 0x77e7ca11)
		pkt = append(pkt, string(r)...)
		conn.WriteToUDP(pkt, dst)
		seq++
	}
	fmt.Println("i3caller: typed (real-time text):", text)
}

// sipMessage sends text to the PSAP as a SIP MESSAGE inside the call.
func (c *caller) sipMessage(text string) {
	c.mu.Lock()
	to := c.toTag
	c.cseq++
	n := c.cseq
	c.mu.Unlock()
	c.send("MESSAGE", c.target(), n, map[string]string{"From": c.fromHdr(), "To": to, "Content-Type": "text/plain;charset=UTF-8"}, text)
	fmt.Println("i3caller: sent (SIP MESSAGE):", text)
}

// ttyQuiet is the tone power below which the line is quiet.
const ttyQuiet = 1e9

// power of one frequency in a stretch of samples (Goertzel).
func power(s []int16, hz float64) float64 {
	k := 2 * math.Cos(2*math.Pi*hz/ttyRate)
	var a, b float64
	for _, v := range s {
		a, b = float64(v)+k*a-b, a
	}
	return a*a + b*b - k*a*b
}

// readTTY decodes a TTY's tones (as ttyTones makes them): each character's start bit, five data bits and stop, at the bit
// centres, and the letters and figures shifts.
func readTTY(pcm []int16) string {
	mark := func(at float64) bool {
		i := int(at)
		w := pcm[max(0, i-40):min(len(pcm), i+40)]
		m, sp := power(w, ttyMark), power(w, ttySpaceHz)
		return m > sp || sp < ttyQuiet // a quiet line is idle, as mark is
	}
	letters, figures := map[byte]rune{}, map[byte]rune{}
	for r, c := range ttyLetters {
		letters[c] = r
	}
	for r, c := range ttyFigures {
		figures[c] = r
	}
	var out []rune
	figs := false
	for i := 0.0; i+7.5*ttyBit < float64(len(pcm)); {
		if mark(i + ttyBit/2) {
			i += ttyBit / 8
			continue
		}
		var code byte
		for b := 0; b < 5; b++ {
			if mark(i + (float64(b)+1.5)*ttyBit) {
				code |= 1 << b
			}
		}
		switch code {
		case ttyLTRS:
			figs = false
		case ttyFIGS:
			figs = true
		case ttySpace:
			out = append(out, ' ')
		default:
			if figs {
				out = append(out, figures[code])
			} else {
				out = append(out, letters[code])
			}
		}
		i += 7.5 * ttyBit
	}
	return string(out)
}
