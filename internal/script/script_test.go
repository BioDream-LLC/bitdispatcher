package script

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

var in = map[string]any{"type": "FALL", "priority": 3, "notes": []string{"Caller says not breathing"}, "age": 82}

func TestSameAnswerInBothLanguages(t *testing.T) {
	js := `function advise(i) {
		log("checking", i.type)
		if (i.notes.some(n => /not breathing/i.test(n))) return {title: "Not breathing: consider CARD", action: {label: "Change", command: "TYPE CARD"}}
	}`
	lu := `function advise(i)
		log("checking", i.type)
		for _, n in ipairs(i.notes) do
			if string.find(string.lower(n), "not breathing") then
				return {title = "Not breathing: consider CARD", action = {label = "Change", command = "TYPE CARD"}}
			end
		end
	end`
	for lang, src := range map[string]string{JS: js, Lua: lu} {
		s, err := Compile(lang, "incident", src)
		if err != nil {
			t.Fatal(lang, err)
		}
		r, err := s.Run(context.Background(), in, 0)
		if err != nil {
			t.Fatal(lang, err)
		}
		m, _ := r.Value.(map[string]any)
		a, _ := m["action"].(map[string]any)
		if m["title"] != "Not breathing: consider CARD" || a["command"] != "TYPE CARD" || len(r.Log) != 1 || r.Log[0] != "checking FALL" {
			t.Fatalf("%s: %+v", lang, r)
		}
	}
}

func TestLimits(t *testing.T) {
	for lang, src := range map[string]string{JS: `function advise(i) { while (true) {} }`, Lua: `function advise(i) while true do end end`} {
		s, _ := Compile(lang, "incident", src)
		start := time.Now()
		_, err := s.Run(context.Background(), in, 30*time.Millisecond)
		if !errors.Is(err, ErrTimeout) || time.Since(start) > time.Second {
			t.Fatalf("%s: a runaway script: %v after %s", lang, err, time.Since(start))
		}
	}
	// Nothing outside: no files, no loading code, no process.
	for lang, src := range map[string]string{
		JS:  `function advise(i) { return typeof require + typeof process + typeof fetch + typeof setTimeout }`,
		Lua: `function advise(i) return tostring(io) .. tostring(os) .. tostring(require) .. tostring(load) .. tostring(dofile) .. tostring(debug) end`,
	} {
		s, err := Compile(lang, "incident", src)
		if err != nil {
			t.Fatal(err)
		}
		r, err := s.Run(context.Background(), in, 0)
		if err != nil {
			t.Fatal(lang, err)
		}
		v, _ := r.Value.(string)
		if strings.ReplaceAll(strings.ReplaceAll(v, "undefined", ""), "nil", "") != "" {
			t.Fatalf("%s reaches outside: %q", lang, v)
		}
	}
	if _, err := Compile(JS, "incident", "function advise( {"); err == nil {
		t.Fatal("a syntax error compiled")
	}
	if _, err := Compile(Lua, "nohook", ""); err == nil {
		t.Fatal("an unknown hook")
	}
	s, _ := Compile(Lua, "report", `x = 1`)
	if _, err := s.Run(context.Background(), in, 0); err == nil || !strings.Contains(err.Error(), "check(") {
		t.Fatalf("no function: %v", err)
	}
}
