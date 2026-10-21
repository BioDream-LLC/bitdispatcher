package cad

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// Handling is what the call-handling system knows about a 911 call beyond its number and location: the
// caller ID's verification (STIR/SHAKEN), how a legacy call's location was looked up, the call-handling
// system's own check of a changed location, the ways text came, the ESInet's routing, the caller's photos
// and clips, the recording and its summary. Each is kept as the call-handling system said it.
type Handling struct {
	CallerID  *CallerIDCheck `json:"callerId,omitempty"`
	ESInet    bool           `json:"esinet,omitempty"`  // came from the emergency services network
	Service   string         `json:"service,omitempty"` // urn:service:sos, sos.fire ...
	CAMA      string         `json:"cama,omitempty"`    // a legacy CAMA trunk call: its information digits ("-": none sent)
	ALILookup string         `json:"aliLookup,omitempty"`
	LocCheck  *LocCheck      `json:"locCheck,omitempty"`
	TextVia   []string       `json:"textVia,omitempty"` // tty, rtt, msrp, sip
	Routes    []Route        `json:"routes,omitempty"`  // the ESInet's routing of the call (ESRP)
	Evidence  *Evidence      `json:"evidence,omitempty"`
	Recording string         `json:"recording,omitempty"` // the recording's name in the call-handling system
	Summary   string         `json:"summary,omitempty"`   // its summary (AI), labelled as such on screen
	SummaryBy string         `json:"summaryBy,omitempty"`
	Taker     string         `json:"taker,omitempty"`    // who answered, in the call-handling system
	LVF       *LVFCheck      `json:"lvf,omitempty"`      // the address checked against the authoritative address data
	EndCause  string         `json:"endCause,omitempty"` // why the call ended ("the caller's audio stopped")
	EndedBy   string         `json:"endedBy,omitempty"`  // caller, callee, system, or a person
}

// LVFCheck is the location validation function's answer (NENA i3 LVF) for the caller's civic address,
// and for the position against that address's point.
type LVFCheck struct {
	Address     string          `json:"address"` // what was checked
	Valid       bool            `json:"valid"`
	Matched     string          `json:"matched,omitempty"` // address point, road centerline
	Invalid     []string        `json:"invalid,omitempty"` // the address parts not found (RD, HNO ...)
	Position    string          `json:"position,omitempty"`
	Within      *bool           `json:"within,omitempty"` // the position is near the address's point
	Suggestions []LVFSuggestion `json:"suggestions,omitempty"`
	At          time.Time       `json:"at"`
}

// LVFSuggestion is a corrected address the LVF offers.
type LVFSuggestion struct {
	Address string  `json:"address"`
	Why     string  `json:"why"`
	Lat     float64 `json:"lat,omitempty"`
	Lon     float64 `json:"lon,omitempty"`
}

// CallerIDCheck is the caller ID's STIR/SHAKEN verification.
type CallerIDCheck struct {
	Verified string `json:"verified,omitempty"` // passed, failed, or empty: nothing to check
	Attest   string `json:"attest,omitempty"`   // A, B, C
	Why      string `json:"why,omitempty"`
}

// LocCheck is the call-handling system's judgement of a changed location: which to show first, and why.
type LocCheck struct {
	Show     string          `json:"show"` // new, old, both
	Text     string          `json:"text"`
	Reasons  []string        `json:"reasons,omitempty"`
	Previous *ng911.Location `json:"previous,omitempty"`
}

// Route is one ESRP routing decision for the call.
type Route struct {
	Service string    `json:"service,omitempty"`
	PSAP    string    `json:"psap,omitempty"`
	Why     string    `json:"why"`
	At      time.Time `json:"at"`
}

// Evidence is the caller's photos and clips, held in the call-handling system.
type Evidence struct {
	Held  bool           `json:"held"`
	Items []EvidenceItem `json:"items"`
}

// EvidenceItem is one photo or clip. The file stays in the call-handling system; bitdispatcher shows it
// blurred until a person reveals it, and records each reveal in its ledger.
type EvidenceItem struct {
	N          int       `json:"n"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	Kind       string    `json:"kind,omitempty"` // photo, video, file
	Size       int64     `json:"size"`
	SHA256     string    `json:"sha256"`
	Received   time.Time `json:"received"`
	Recording  bool      `json:"recording,omitempty"` // recorded from the caller's live video
	Taken      time.Time `json:"taken,omitzero"`
	TakenLocal string    `json:"takenLocal,omitempty"` // the camera's own clock, no zone
	HasGPS     bool      `json:"hasGps,omitempty"`
	Lat        float64   `json:"lat,omitempty"`
	Lon        float64   `json:"lon,omitempty"`
}

// mergeHandling adds what a call-handling update says to what is known.
func mergeHandling(c *Call, h *Handling) {
	if h == nil {
		return
	}
	if c.Handling == nil {
		c.Handling = &Handling{}
	}
	o := c.Handling
	if h.CallerID != nil {
		v := *h.CallerID
		o.CallerID = &v
	}
	o.ESInet = o.ESInet || h.ESInet
	for _, p := range []struct {
		dst *string
		v   string
	}{{&o.Service, h.Service}, {&o.CAMA, h.CAMA}, {&o.ALILookup, h.ALILookup},
		{&o.Recording, h.Recording}, {&o.Summary, h.Summary}, {&o.SummaryBy, h.SummaryBy}, {&o.Taker, h.Taker},
		{&o.EndCause, h.EndCause}, {&o.EndedBy, h.EndedBy}} {
		if p.v != "" {
			*p.dst = p.v
		}
	}
	if h.LocCheck != nil {
		v := *h.LocCheck
		o.LocCheck = &v
	}
	for _, t := range h.TextVia {
		if !slices.Contains(o.TextVia, t) {
			o.TextVia = append(o.TextVia, t)
		}
	}
	for _, r := range h.Routes {
		if !slices.ContainsFunc(o.Routes, func(x Route) bool { return x.Why == r.Why && x.At.Equal(r.At) }) {
			o.Routes = append(o.Routes, r)
		}
	}
	if h.LVF != nil {
		v := *h.LVF
		o.LVF = &v
	}
	if h.Evidence != nil {
		v := *h.Evidence
		v.Items = slices.Clone(h.Evidence.Items)
		o.Evidence = &v
	}
}

// handlingChanges says whether an update adds anything.
func handlingChanges(c *Call, h *Handling) bool {
	if h == nil {
		return false
	}
	before, _ := json.Marshal(c.Handling)
	cp := *c
	if c.Handling != nil {
		hh := *c.Handling
		cp.Handling = &hh
	}
	mergeHandling(&cp, h)
	after, _ := json.Marshal(cp.Handling)
	return string(before) != string(after)
}
