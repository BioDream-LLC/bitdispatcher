package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"-version"}, &out, &errOut); code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, errOut.String())
	}
	if !strings.HasPrefix(out.String(), "bitdispatcher ") {
		t.Fatalf("unexpected output %q", out.String())
	}
}

func TestUnknownFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"-nope"}, &out, &errOut); code != 2 {
		t.Fatalf("exit code %d, want 2", code)
	}
}

func TestHooksListenerIsLoopbackOnly(t *testing.T) {
	for _, a := range []string{"0.0.0.0:0", ":0", "192.0.2.1:0", "example.com:0"} {
		if ln, err := hooksListener(a); err == nil {
			ln.Close()
			t.Errorf("%s was accepted", a)
		}
	}
	for _, a := range []string{"127.0.0.1:0", "localhost:0", "[::1]:0"} {
		ln, err := hooksListener(a)
		if err != nil {
			if a == "[::1]:0" {
				continue // no IPv6 loopback here
			}
			t.Fatalf("%s: %v", a, err)
		}
		ln.Close()
	}
}
