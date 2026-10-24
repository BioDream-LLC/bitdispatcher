// Package ledger is bitdispatcher's append-only event ledger: every action that takes attention, changes
// state, creates accountability or feeds a report is one event (docs/OPERATIONS.md section 2).
//
// The ledger is a file of JSON lines. Each event carries the SHA-256 hash of the one before it, so any
// change to a written event, any removal and any reordering breaks the chain and is found by Verify. Every
// append is synced to disk before it returns: an event the caller was told about survives a power cut.
package ledger

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Event is one thing that happened.
type Event struct {
	Seq      uint64          `json:"seq"`
	ID       string          `json:"id"`
	At       time.Time       `json:"at"`
	Type     string          `json:"type"`
	Actor    string          `json:"actor"`             // a username, or system, bitvoice, ali, radio
	Role     string          `json:"role,omitempty"`    // the actor's active role
	Session  string          `json:"session,omitempty"` // the authenticated session (a hash, never the token)
	Console  string          `json:"console,omitempty"` // the position
	Duty     string          `json:"duty,omitempty"`    // the work the actor signed in to do (call-taker, police dispatcher...)
	Agency   string          `json:"agency,omitempty"`  // the agency the actor was working for, when agencies are configured
	Incident string          `json:"incident,omitempty"`
	Unit     string          `json:"unit,omitempty"`
	Call     string          `json:"call,omitempty"`
	Parent   string          `json:"parent,omitempty"` // a batch's parent event
	Site     string          `json:"site,omitempty"`   // the site that wrote it, when there are several
	Data     json.RawMessage `json:"data,omitempty"`
	Prev     string          `json:"prev"`
	Hash     string          `json:"hash"`
}

// Ledger is an open ledger file.
type Ledger struct {
	mu   sync.Mutex
	f    *os.File
	seq  uint64
	last string
	now  func() time.Time
}

// ErrBroken is returned when the chain does not verify.
var ErrBroken = errors.New("ledger: hash chain broken")

func hashOf(e Event) (string, error) {
	e.Hash = ""
	b, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// NewID returns a random identifier.
func NewID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // the system's random source failing is not recoverable
	}
	return hex.EncodeToString(b[:])
}

// Open opens (or creates) the ledger at path, verifies it and returns every event in it.
//
// A final line cut short by a crash mid-write (no newline, not valid JSON) was never acknowledged to anyone,
// so it is moved to path+".torn" and removed from the ledger. Anything else that fails to verify stops the
// open: a broken chain needs a person, not an automatic repair.
func Open(path string) (*Ledger, []Event, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	if n := len(data); n > 0 && data[n-1] != '\n' {
		cut := bytes.LastIndexByte(data, '\n') + 1
		var probe Event
		if json.Unmarshal(data[cut:], &probe) == nil {
			// A complete event missing only its newline: keep it.
			if _, err := f.Write([]byte("\n")); err != nil {
				f.Close()
				return nil, nil, err
			}
			data = append(data, '\n')
		} else {
			if err := os.WriteFile(path+".torn", data[cut:], 0o600); err != nil {
				f.Close()
				return nil, nil, err
			}
			if err := f.Truncate(int64(cut)); err != nil {
				f.Close()
				return nil, nil, err
			}
			data = data[:cut]
		}
	}
	events, err := parse(data)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		f.Close()
		return nil, nil, err
	}
	l := &Ledger{f: f, now: time.Now}
	if n := len(events); n > 0 {
		l.seq, l.last = events[n-1].Seq, events[n-1].Hash
	}
	return l, events, nil
}

func parse(data []byte) ([]Event, error) {
	var out []Event
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
	prev := ""
	for line := 1; sc.Scan(); line++ {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("ledger line %d: %w", line, err)
		}
		h, err := hashOf(e)
		if err != nil {
			return nil, err
		}
		if e.Prev != prev || e.Hash != h || e.Seq != uint64(len(out))+1 {
			return nil, fmt.Errorf("%w at line %d (seq %d)", ErrBroken, line, e.Seq)
		}
		prev = e.Hash
		out = append(out, e)
	}
	return out, sc.Err()
}

// Append writes an event, filling in its sequence number, ID, time, chain and hash, and syncs it to disk.
func (l *Ledger) Append(e Event) (Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return Event{}, errors.New("ledger: closed")
	}
	if len(e.Data) > 0 {
		var buf bytes.Buffer
		if err := json.Compact(&buf, e.Data); err != nil {
			return Event{}, fmt.Errorf("ledger: event data: %w", err)
		}
		e.Data = buf.Bytes()
	}
	e.Seq = l.seq + 1
	if e.ID == "" {
		e.ID = NewID()
	}
	if e.At.IsZero() {
		e.At = l.now()
	}
	e.At = e.At.UTC()
	e.Prev = l.last
	h, err := hashOf(e)
	if err != nil {
		return Event{}, err
	}
	e.Hash = h
	b, err := json.Marshal(e)
	if err != nil {
		return Event{}, err
	}
	if _, err := l.f.Write(append(b, '\n')); err != nil {
		return Event{}, err
	}
	if err := l.f.Sync(); err != nil {
		return Event{}, err
	}
	l.seq, l.last = e.Seq, e.Hash
	return e, nil
}

// Replica is a copy of another site's ledger: its events kept exactly as that site wrote them, each
// checked against the chain before it is added, so a copy can be verified like the original.
type Replica struct {
	l *Ledger
}

// OpenReplica opens or creates a replica file and returns its events.
func OpenReplica(path string) (*Replica, []Event, error) {
	l, evs, err := Open(path)
	if err != nil {
		return nil, nil, err
	}
	return &Replica{l: l}, evs, nil
}

// Add appends another site's event after checking that it follows the last one and that its hash is
// its own.
func (r *Replica) Add(e Event) error {
	r.l.mu.Lock()
	defer r.l.mu.Unlock()
	if e.Seq != r.l.seq+1 {
		return fmt.Errorf("ledger: replica expected event %d, got %d", r.l.seq+1, e.Seq)
	}
	if e.Prev != r.l.last {
		return ErrBroken
	}
	if h, err := hashOf(e); err != nil || h != e.Hash {
		return ErrBroken
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := r.l.f.Write(append(b, '\n')); err != nil {
		return err
	}
	if err := r.l.f.Sync(); err != nil {
		return err
	}
	r.l.seq, r.l.last = e.Seq, e.Hash
	return nil
}

// Seq is the last event copied.
func (r *Replica) Seq() uint64 { return r.l.Seq() }

// Close closes the replica.
func (r *Replica) Close() error { return r.l.Close() }

// Seq is the sequence number of the last event written.
func (l *Ledger) Seq() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.seq
}

// Close closes the file.
func (l *Ledger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// ReadAll reads and verifies the ledger at path without changing it.
func ReadAll(path string) ([]Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if n := len(data); n > 0 && data[n-1] != '\n' {
		data = data[:bytes.LastIndexByte(data, '\n')+1] // a write in progress
	}
	return parse(data)
}

// Report is the result of verifying a ledger file.
type Report struct {
	Events   int       `json:"events"`
	OK       bool      `json:"ok"`
	Problem  string    `json:"problem,omitempty"`
	LastHash string    `json:"lastHash,omitempty"`
	Checked  time.Time `json:"checked"`
}

// Verify re-reads the file at path from the start and checks every hash and link.
func Verify(path string) Report {
	r := Report{Checked: time.Now().UTC()}
	data, err := os.ReadFile(path)
	if err != nil {
		r.Problem = err.Error()
		return r
	}
	events, err := parse(data)
	if err != nil {
		r.Problem = err.Error()
		return r
	}
	r.Events, r.OK = len(events), true
	if len(events) > 0 {
		r.LastHash = events[len(events)-1].Hash
	}
	return r
}
