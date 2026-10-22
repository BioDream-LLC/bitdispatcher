package main

import "testing"

func TestTTYTonesDecode(t *testing.T) {
	want := "HELP AT 215 CHURCH AVE. GA"
	if got := readTTY(ttyTones("help at 215 Church Ave. GA")); got != want {
		t.Fatalf("decoded %q, want %q", got, want)
	}
	// A character a TTY cannot send is left out.
	if got := readTTY(ttyTones("A@B")); got != "AB" {
		t.Fatalf("decoded %q", got)
	}
	// Quiet before and after (the line between turns) is not read as characters.
	quiet := make([]int16, 8000)
	pcm := append(append(append([]int16{}, quiet...), ttyTones("GA")...), quiet...)
	if got := readTTY(pcm); got != "GA" {
		t.Fatalf("with quiet around it: %q", got)
	}
}

// The TTY reply comes through G.711: the tones survive mu-law and back.
func TestTTYThroughMuLaw(t *testing.T) {
	var pcm []int16
	for _, v := range ttyTones("UNITS ON THE WAY GA") {
		pcm = append(pcm, muLawToLinear(linearToMuLaw(v)))
	}
	if got := readTTY(pcm); got != "UNITS ON THE WAY GA" {
		t.Fatalf("decoded %q", got)
	}
}
