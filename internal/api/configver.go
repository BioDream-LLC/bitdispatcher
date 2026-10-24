package api

// Configuration versions: each publish from the staging board records what it replaced, so an
// administrator can put the previous approved version back (with a reason, as its own ledger event). A
// publish for one agency carries only that agency's changes, never another agency's or the centre's.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

// configParts in the order they are applied: agencies before what belongs to them, stations before units.
var configParts = []string{"agencies", "settings", "stations", "calltypes", "units", "plans", "guidance", "facilities"}

// itemAgencies are the agencies an item of configuration belongs to ("" for the centre's).
func itemAgencies(part, k string, st *cad.State) []string {
	switch part {
	case "agencies":
		return []string{k}
	case "units":
		if u := st.Units[k]; u != nil {
			return []string{u.Agency}
		}
	case "stations":
		if x := st.Stations[k]; x != nil {
			return []string{x.Agency}
		}
	case "calltypes":
		if x := st.CallTypes[k]; x != nil {
			if len(x.Agencies) == 0 {
				return []string{""}
			}
			return x.Agencies
		}
	case "plans":
		if x := st.Plans[k]; x != nil {
			return []string{x.Agency}
		}
	}
	return nil
}

// onlyAgency keeps the changes that belong to one agency, as staged and as live: an item that is (or
// was) another agency's, or the centre's, is left for its own publish.
func onlyAgency(diff map[string][]string, agency string, staged, live *cad.State) map[string][]string {
	out := map[string][]string{}
	for part, lines := range diff {
		for _, l := range lines {
			_, k, _ := strings.Cut(l, " ")
			ok := true
			seen := false
			for _, st := range []*cad.State{staged, live} {
				for _, g := range itemAgencies(part, k, st) {
					seen = true
					if g != agency {
						ok = false
					}
				}
			}
			if ok && seen {
				out[part] = append(out[part], l)
			}
		}
	}
	return out
}

// configBefore is the live version of each item a publish changes (null when it did not exist).
func configBefore(diff map[string][]string, live *cad.State) map[string]map[string]json.RawMessage {
	out := map[string]map[string]json.RawMessage{}
	for part, lines := range diff {
		out[part] = map[string]json.RawMessage{}
		for _, l := range lines {
			_, k, _ := strings.Cut(l, " ")
			var v any
			switch part {
			case "settings":
				v = live.Settings
			case "agencies":
				if x := live.Agencies[k]; x != nil {
					v = x
				}
			case "units":
				if x := live.Units[k]; x != nil {
					v = x.UnitDef
				}
			case "stations":
				if x := live.Stations[k]; x != nil {
					v = x
				}
			case "calltypes":
				if x := live.CallTypes[k]; x != nil {
					v = x
				}
			case "plans":
				if x := live.Plans[k]; x != nil {
					v = x
				}
			case "guidance":
				if x := live.Guidance[k]; x != nil {
					v = x
				}
			case "facilities":
				if x := live.Facilities[k]; x != nil {
					v = x
				}
			}
			b, _ := json.Marshal(v)
			out[part][k] = b
		}
	}
	return out
}

type publishRecord struct {
	Parts   map[string][]string                   `json:"parts"`
	Why     string                                `json:"why"`
	Changes int                                   `json:"changes"`
	Agency  string                                `json:"agency,omitempty"`
	Before  map[string]map[string]json.RawMessage `json:"before,omitempty"`
	// RollbackOf is the publish a rollback undid.
	RollbackOf uint64 `json:"rollbackOf,omitempty"`
}

// restore puts the recorded versions back.
func restore(a cad.Actor, live *cad.Engine, before map[string]map[string]json.RawMessage) (int, error) {
	n := 0
	for _, part := range configParts {
		for k, raw := range before[part] {
			var err error
			gone := string(raw) == "null" || len(raw) == 0
			switch part {
			case "settings":
				var v cad.Settings
				if err = json.Unmarshal(raw, &v); err == nil {
					err = live.SaveSettings(a, v)
				}
			case "agencies":
				if gone {
					err = live.RemoveAgency(a, k)
				} else {
					var v cad.Agency
					if err = json.Unmarshal(raw, &v); err == nil {
						err = live.SaveAgency(a, v)
					}
				}
			case "units":
				if gone {
					err = live.RemoveUnit(a, k)
				} else {
					var v cad.UnitDef
					if err = json.Unmarshal(raw, &v); err == nil {
						err = live.SaveUnit(a, v)
					}
				}
			case "stations":
				if gone {
					err = live.RemoveStation(a, k)
				} else {
					var v cad.Station
					if err = json.Unmarshal(raw, &v); err == nil {
						err = live.SaveStation(a, v)
					}
				}
			case "calltypes":
				if gone {
					err = live.RemoveCallType(a, k)
				} else {
					var v cad.CallType
					if err = json.Unmarshal(raw, &v); err == nil {
						err = live.SaveCallType(a, v)
					}
				}
			case "plans":
				if gone {
					err = live.RemovePlan(a, k)
				} else {
					var v cad.ResponsePlan
					if err = json.Unmarshal(raw, &v); err == nil {
						_, err = live.SavePlan(a, v)
					}
				}
			case "guidance":
				if gone {
					err = live.RemoveGuidance(a, k)
				} else {
					var v cad.GuidanceDoc
					if err = json.Unmarshal(raw, &v); err == nil {
						err = live.SaveGuidance(a, v)
					}
				}
			case "facilities":
				if gone {
					err = live.RemoveFacility(a, k)
				} else {
					var v cad.Facility
					if err = json.Unmarshal(raw, &v); err == nil {
						err = live.SaveFacility(a, v)
					}
				}
			}
			if err != nil {
				return n, fmt.Errorf("%s %s: %w", part, k, err)
			}
			n++
		}
	}
	return n, nil
}

// configVersionRoutes are on the live server: the publishes made, and rolling one back.
func (s *Server) configVersionRoutes() {
	type pub struct {
		Seq      uint64              `json:"seq"`
		At       string              `json:"at"`
		By       string              `json:"by"`
		Why      string              `json:"why"`
		Agency   string              `json:"agency,omitempty"`
		Changes  int                 `json:"changes"`
		Parts    map[string][]string `json:"parts"`
		Rollback uint64              `json:"rollbackOf,omitempty"`
		Undone   uint64              `json:"undoneBy,omitempty"`
	}
	s.handle("GET /api/admin/config/versions", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		evs, err := s.Eng.Events(0, 0, func(e ledger.Event) bool { return e.Type == cad.EvConfigStaged })
		if err != nil {
			writeErr(w, err)
			return
		}
		out := []*pub{}
		bySeq := map[uint64]*pub{}
		for _, e := range evs {
			var p publishRecord
			json.Unmarshal(e.Data, &p)
			x := &pub{Seq: e.Seq, At: e.At.Format("2006-01-02T15:04:05Z07:00"), By: e.Actor, Why: p.Why, Agency: p.Agency, Changes: p.Changes, Parts: p.Parts, Rollback: p.RollbackOf}
			bySeq[e.Seq] = x
			if o := bySeq[p.RollbackOf]; o != nil {
				o.Undone = e.Seq
			}
			out = append(out, x)
		}
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
		writeJSON(w, 200, map[string]any{"items": out})
	})
	s.handle("POST /api/admin/config/versions/{seq}/rollback", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Why string `json:"why"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		if strings.TrimSpace(b.Why) == "" {
			fail(w, 422, "say why the change is rolled back")
			return
		}
		var seq uint64
		fmt.Sscan(r.PathValue("seq"), &seq)
		evs, err := s.Eng.Events(0, 0, func(e ledger.Event) bool { return e.Type == cad.EvConfigStaged })
		if err != nil {
			writeErr(w, err)
			return
		}
		var p *publishRecord
		for _, e := range evs {
			var x publishRecord
			json.Unmarshal(e.Data, &x)
			if e.Seq == seq {
				p = &x
			}
			if x.RollbackOf == seq && seq != 0 {
				fail(w, 409, "that change was already rolled back")
				return
			}
		}
		if p == nil || p.Before == nil {
			fail(w, 404, "no publish with its earlier versions recorded at that number")
			return
		}
		var refuse bool
		s.Eng.Read(func(st *cad.State) {
			refuse = !st.AllAgencies(se.Username) && (p.Agency == "" || !st.MayWork(se.Username, p.Agency))
		})
		if refuse {
			fail(w, http.StatusForbidden, "you may roll back only your agencies' changes")
			return
		}
		var cur *cad.State
		s.Eng.Read(func(st *cad.State) { cur = snapshot(st) })
		parts := map[string][]string{}
		for part, items := range p.Before {
			for k := range items {
				parts[part] = append(parts[part], "restored "+k)
			}
		}
		now := configBefore(parts, cur)
		n, err := restore(a, s.Eng, p.Before)
		if err != nil {
			writeErr(w, err)
			return
		}
		s.Eng.Record(a, cad.EvConfigStaged, publishRecord{Parts: parts, Why: strings.TrimSpace(b.Why), Changes: n, Agency: p.Agency, Before: now, RollbackOf: seq})
		writeJSON(w, 200, map[string]int{"changes": n})
	})
}
