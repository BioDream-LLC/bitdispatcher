package cad

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// Several sites, active-active (DESIGN.md rule 3): every site writes its own ledger and keeps a verified
// copy of every other site's. The state is the same function of all the events everywhere: they are
// applied in order of time (then site, then sequence), so sites that have the same events have the same
// state, whatever order the events arrived in. Each site numbers its own incidents under its own prefix.
// A site cut off from the others keeps working on its own events and catches up when the link returns.

// SetSite names this site and its incident number prefix ("A", "B"). It is set before anything is written.
func (e *Engine) SetSite(name, prefix string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.site, e.sitePrefix = name, prefix
}

// Site is this site's name.
func (e *Engine) Site() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.site
}

// OpenPeers opens the copies of the other sites' ledgers under dir and rebuilds the state with their
// events.
func (e *Engine) OpenPeers(dir string, names []string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.replicas == nil {
		e.replicas = map[string]*ledger.Replica{}
	}
	for _, n := range names {
		if _, ok := e.replicas[n]; ok || n == e.site {
			continue
		}
		r, evs, err := ledger.OpenReplica(filepath.Join(dir, n+".jsonl"))
		if err != nil {
			return fmt.Errorf("site %s: %w", n, err)
		}
		e.replicas[n] = r
		e.remote = append(e.remote, evs...)
	}
	return e.rebuild()
}

// PeerSeq is the last event copied from a site.
func (e *Engine) PeerSeq(name string) uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r, ok := e.replicas[name]; ok {
		return r.Seq()
	}
	return 0
}

// AddRemote copies events from another site and applies them. Events that are newer than everything
// applied are applied at once; an older one (from a site that was cut off) means rebuilding the state
// from all events, so every site ends with the same state.
func (e *Engine) AddRemote(name string, evs []ledger.Event) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	r, ok := e.replicas[name]
	if !ok {
		return 0, fmt.Errorf("cad: %s is not a known site", name)
	}
	n := 0
	inOrder := true
	for _, ev := range evs {
		if ev.Seq <= r.Seq() {
			continue // already have it
		}
		if err := r.Add(ev); err != nil {
			return n, fmt.Errorf("site %s event %d: %w", name, ev.Seq, err)
		}
		e.remote = append(e.remote, ev)
		n++
		if ev.At.Before(e.lastAt) {
			inOrder = false
		}
		if inOrder {
			if err := e.st.apply(ev); err != nil {
				return n, err
			}
			e.lastAt = ev.At
		}
	}
	if n == 0 {
		return 0, nil
	}
	if !inOrder {
		if err := e.rebuild(); err != nil {
			return n, err
		}
	}
	e.notify(Notice{Type: "resync"})
	return n, nil
}

func order(evs []ledger.Event) {
	sort.SliceStable(evs, func(i, j int) bool {
		a, b := evs[i], evs[j]
		if !a.At.Equal(b.At) {
			return a.At.Before(b.At)
		}
		if a.Site != b.Site {
			return a.Site < b.Site
		}
		return a.Seq < b.Seq
	})
}

// allEvents is every site's events in the order they apply.
func (e *Engine) allEvents() ([]ledger.Event, error) {
	e.mu.Lock()
	remote := slices.Clone(e.remote)
	e.mu.Unlock()
	return e.merged(remote)
}

func (e *Engine) merged(remote []ledger.Event) ([]ledger.Event, error) {
	local, err := ledger.ReadAll(e.path)
	if err != nil || len(remote) == 0 {
		return local, err
	}
	all := append(local, remote...)
	order(all)
	return all, nil
}

// rebuild applies every event again in order. The caller holds e.mu.
func (e *Engine) rebuild() error {
	all, err := e.merged(e.remote)
	if err != nil {
		return err
	}
	st := newState()
	for _, ev := range all {
		if err := st.apply(ev); err != nil {
			return fmt.Errorf("event %s %d: %w", ev.Site, ev.Seq, err)
		}
	}
	e.st = st
	if n := len(all); n > 0 {
		e.lastAt = all[n-1].At
	}
	return nil
}

// LocalEvents are the events this site wrote after seq, for the other sites to copy.
func (e *Engine) LocalEvents(after uint64, limit int) ([]ledger.Event, error) {
	evs, err := ledger.ReadAll(e.path)
	if err != nil {
		return nil, err
	}
	out := []ledger.Event{}
	for _, ev := range evs {
		if ev.Seq > after {
			out = append(out, ev)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}
