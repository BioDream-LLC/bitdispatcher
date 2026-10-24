package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

func TestTwoSitesOverHTTPS(t *testing.T) {
	tsA, sA, pwA := newServer(t)
	tsB, sB, pwB := newServer(t)
	fpA := sha256.Sum256(tsA.Certificate().Raw)
	fpB := sha256.Sum256(tsB.Certificate().Raw)
	a := login(t, tsA, "admin", pwA["admin"])
	b := login(t, tsB, "admin", pwB["admin"])
	// Each site first gets its name (and its token, made for it).
	var va, vb SitesView
	_, body := a.do("PUT", "/api/admin/sites", SiteConfig{Name: "north", Prefix: "N"}, true)
	json.Unmarshal(body, &va)
	_, body = b.do("PUT", "/api/admin/sites", SiteConfig{Name: "south", Prefix: "S"}, true)
	json.Unmarshal(body, &vb)
	if va.Token == "" || vb.Token == "" {
		t.Fatalf("no tokens: %+v %+v", va, vb)
	}
	// Then each names the other, with its token and certificate.
	if code, body := a.do("PUT", "/api/admin/sites", SiteConfig{Name: "north", Prefix: "N", Token: va.Token,
		Peers: []Peer{{Name: "south", URL: tsB.URL, Token: vb.Token, Fingerprint: hex.EncodeToString(fpB[:])}}}, true); code != 200 {
		t.Fatalf("north: %d %s", code, body)
	}
	if code, body := b.do("PUT", "/api/admin/sites", SiteConfig{Name: "south", Prefix: "S", Token: vb.Token,
		Peers: []Peer{{Name: "north", URL: tsA.URL, Token: va.Token, Fingerprint: hex.EncodeToString(fpA[:])}}}, true); code != 200 {
		t.Fatalf("south: %d %s", code, body)
	}
	if code, _ := a.do("PUT", "/api/admin/sites", SiteConfig{Name: "renamed", Prefix: "N", Token: va.Token}, true); code != 409 {
		t.Fatalf("renamed a site in use: %d", code)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sA.runSites(ctx)
	go sB.runSites(ctx)
	// A station saved at north appears at south; an incident created at south appears at north.
	sA.Eng.SaveStation(cad.SystemActor, cad.Station{ID: "ST9", Name: "North station", Lat: 37.3, Lon: -79.9})
	num, _ := sB.Eng.CreateIncident(cad.SystemActor, cad.NewIncident{Type: "SF"})
	if num != "S26-000001" && num[0] != 'S' {
		t.Fatalf("south's number %s", num)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		var okA, okB bool
		sA.Eng.Read(func(st *cad.State) { _, okA = st.Incidents[num] })
		sB.Eng.Read(func(st *cad.State) { _, okB = st.Stations["ST9"] })
		if okA && okB {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not copied: north has the incident %v, south has the station %v; health %+v %+v", okA, okB, sA.siteHealth(), sB.siteHealth())
		}
		time.Sleep(50 * time.Millisecond)
	}
	// North's health is set by its own poll of south, which can finish after the copies arrive.
	for {
		h := sA.siteHealth()
		if len(h) == 1 && h[0].OK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("health %+v", h)
		}
		time.Sleep(50 * time.Millisecond)
	}
	// A wrong token is refused.
	anon := &client{t: t, base: tsA.URL, c: tsA.Client()}
	if code, _ := anon.do("GET", "/api/peer/events?after=0", nil, false); code != 401 {
		t.Fatalf("peer events without the token: %d", code)
	}
}
