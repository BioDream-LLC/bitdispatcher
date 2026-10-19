package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAppendReopenVerify(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	l, events, err := Open(path)
	if err != nil || len(events) != 0 {
		t.Fatalf("open empty: %v %d", err, len(events))
	}
	for i := 0; i < 5; i++ {
		data, _ := json.Marshal(map[string]any{"n": i, "text": "<b>& tags"})
		if _, err := l.Append(Event{Type: "test", Actor: "tester", Data: data}); err != nil {
			t.Fatal(err)
		}
	}
	l.Close()
	l2, events, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	if len(events) != 5 || events[4].Seq != 5 || events[0].Prev != "" || events[1].Prev != events[0].Hash {
		t.Fatalf("bad chain after reopen: %+v", events)
	}
	if e, err := l2.Append(Event{Type: "after"}); err != nil || e.Seq != 6 || e.Prev != events[4].Hash {
		t.Fatalf("append after reopen: %+v %v", e, err)
	}
	if r := Verify(path); !r.OK || r.Events != 6 {
		t.Fatalf("verify: %+v", r)
	}
}

func TestTamperIsFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	l, _, _ := Open(path)
	for _, who := range []string{"a", "b", "c"} {
		l.Append(Event{Type: "test", Actor: who})
	}
	l.Close()
	data, _ := os.ReadFile(path)
	os.WriteFile(path, bytes.Replace(data, []byte(`"actor":"b"`), []byte(`"actor":"x"`), 1), 0o600)
	if r := Verify(path); r.OK {
		t.Fatal("a changed actor verified")
	}
	if _, _, err := Open(path); !errors.Is(err, ErrBroken) {
		t.Fatalf("open of tampered ledger: %v", err)
	}
	// Removing a line breaks the chain too.
	lines := bytes.SplitAfter(data, []byte("\n"))
	os.WriteFile(path, append(lines[0], lines[2]...), 0o600)
	if r := Verify(path); r.OK {
		t.Fatal("a removed event verified")
	}
}

func TestTornLastLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	l, _, _ := Open(path)
	l.Append(Event{Type: "one"})
	l.Close()
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	f.Write([]byte(`{"seq":2,"id":"x","ty`))
	f.Close()
	l2, events, err := Open(path)
	if err != nil || len(events) != 1 {
		t.Fatalf("torn line: %v %d", err, len(events))
	}
	if e, err := l2.Append(Event{Type: "two"}); err != nil || e.Seq != 2 {
		t.Fatalf("append after torn: %+v %v", e, err)
	}
	l2.Close()
	if _, err := os.Stat(path + ".torn"); err != nil {
		t.Fatal("torn fragment was not kept")
	}
	if r := Verify(path); !r.OK || r.Events != 2 {
		t.Fatalf("verify after torn: %+v", r)
	}
}
