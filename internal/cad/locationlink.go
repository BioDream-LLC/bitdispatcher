package cad

// Location links: when a call brings no usable location, the call-taker sends the caller a link (texted
// through BitVoice, or read out). Opening it on the phone and pressing Share sends the phone's own GPS
// position to the call, again as the caller moves, until the link expires or the caller stops. Only the
// link's hash is kept; the link itself is shown once to the call-taker.

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvLocLinkMade    = "call.loclink.made"
	EvLocLinkEnded   = "call.loclink.ended"
	keepLocLinkCalls = 500
)

// LocationLink is a link sent to a caller to share their location.
type LocationLink struct {
	ID      string     `json:"id"`
	Call    string     `json:"call"`
	Hash    string     `json:"hash"` // sha256 of the link's secret
	Expires time.Time  `json:"expires"`
	By      string     `json:"by"`
	At      time.Time  `json:"at"`
	Sent    string     `json:"sent,omitempty"` // "texted", or why not
	Ended   *time.Time `json:"ended,omitempty"`
	Why     string     `json:"why,omitempty"` // why it ended: the caller stopped, the call-taker ended it
}

// Live reports whether the link still takes positions.
func (l *LocationLink) Live(t time.Time) bool { return l.Ended == nil && t.Before(l.Expires) }

// HashSecret is how a link's secret is kept.
func HashSecret(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

// MakeLocationLink records a link for a call; the caller gives the secret's hash and its life.
func (e *Engine) MakeLocationLink(a Actor, call, hash string, life time.Duration, sent string) (LocationLink, error) {
	if life < time.Minute || life > 24*time.Hour {
		return LocationLink{}, problem("minutes", "a link lasts from a minute to a day")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	c := e.st.Calls[call]
	if c == nil {
		return LocationLink{}, problem("call", "no call %s", call)
	}
	if c.State == "ended" {
		return LocationLink{}, problem("call", "call %s has ended", call)
	}
	now := e.now()
	l := LocationLink{ID: e.newID("LL", func(id string) bool { return e.st.LocLinks[id] != nil }), Call: call, Hash: hash,
		Expires: now.Add(life), By: a.User, At: now, Sent: sent}
	_, err := e.write(a, EvLocLinkMade, refs{Call: call, Incident: c.Incident}, l)
	return l, err
}

// EndLocationLink stops a link taking positions.
func (e *Engine) EndLocationLink(a Actor, id, why string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	l := e.st.LocLinks[id]
	if l == nil {
		return problem("id", "no link %s", id)
	}
	if l.Ended != nil {
		return nil
	}
	_, err := e.write(a, EvLocLinkEnded, refs{Call: l.Call}, map[string]string{"id": id, "why": why})
	return err
}

// LocationLinkByHash is the link with a secret's hash.
func (s *State) LocationLinkByHash(hash string) *LocationLink {
	for _, l := range s.LocLinks {
		if l.Hash == hash {
			return l
		}
	}
	return nil
}

// LinksFor are a call's location links, newest first.
func (s *State) LinksFor(call string) []*LocationLink {
	var out []*LocationLink
	for _, l := range s.LocLinks {
		if l.Call == call {
			out = append(out, l)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].At.After(out[j-1].At); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func (s *State) applyLocLinks(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvLocLinkMade:
		l, err := decode[LocationLink](e)
		if err != nil {
			return true, err
		}
		s.LocLinks[l.ID] = &l
		if len(s.LocLinks) > keepLocLinkCalls { // the oldest ended ones go; the ledger keeps them all
			var old *LocationLink
			for _, x := range s.LocLinks {
				if !x.Live(e.At) && (old == nil || x.At.Before(old.At)) {
					old = x
				}
			}
			if old != nil {
				delete(s.LocLinks, old.ID)
			}
		}
	case EvLocLinkEnded:
		v, err := decode[map[string]string](e)
		if err != nil {
			return true, err
		}
		if l := s.LocLinks[v["id"]]; l != nil {
			at := e.At
			l.Ended, l.Why = &at, v["why"]
		}
	default:
		return false, nil
	}
	return true, nil
}
