package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestBlockAddress(t *testing.T) {
	for in, want := range map[string]string{
		"215 Church Ave SW, Roanoke VA": "200 block Church Ave SW",
		"12 Mill Rd APT 4":              "0 block Mill Rd",
		"1408-B Main St":                "1400 block Main St",
		"Valley View Mall":              "Valley View Mall",
	} {
		if got := BlockAddress(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// The feed is off until turned on; it has no caller, no notes and no house number; police only when
// chosen; with a token it is refused without it.
func TestPublicFeed(t *testing.T) {
	ts, _, pw := newServer(t)
	admin := login(t, ts, "admin", pw["admin"])
	d1 := login(t, ts, "d1", pw["d1"])
	get := func(tok string) (int, string) {
		req, _ := http.NewRequest("GET", ts.URL+"/api/public/incidents", nil)
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var b strings.Builder
		buf := make([]byte, 1<<16)
		for {
			n, err := resp.Body.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				break
			}
		}
		return resp.StatusCode, b.String()
	}
	if code, _ := get(""); code != 404 {
		t.Fatalf("feed on by default: %d", code)
	}
	d1.do("POST", "/api/incidents", map[string]any{"type": "SF", "caller": map[string]any{"name": "Jane Roe", "callback": "+15405550111"}, "narrative": "smoke from the attic",
		"location": map[string]any{"lat": 40.271234, "lon": -76.881234, "address": "215 Church Ave SW, Roanoke VA"}}, true)
	d1.do("POST", "/api/incidents", map[string]any{"type": "BURG", "location": map[string]any{"lat": 40.27, "lon": -76.88, "address": "9 Elm St"}}, true)
	if code, b := admin.do("PUT", "/api/admin/public-feed", map[string]any{"on": true, "cprTypes": []string{"SF"}}, true); code != 200 {
		t.Fatalf("config %d %s", code, b)
	}
	code, body := get("")
	if code != 200 {
		t.Fatalf("feed %d %s", code, body)
	}
	for _, leak := range []string{"Jane", "5550111", "attic", "215 Church", "Elm", "40.271234"} {
		if strings.Contains(body, leak) {
			t.Fatalf("the feed shows %q: %s", leak, body)
		}
	}
	var doc PublicFeedDoc
	json.Unmarshal([]byte(body), &doc)
	if len(doc.Incidents) != 1 || doc.Incidents[0].Address != "200 block Church Ave SW" || *doc.Incidents[0].Lat != 40.271 || !doc.Incidents[0].CPR {
		t.Fatalf("%s", body)
	}
	admin.do("PUT", "/api/admin/public-feed", map[string]any{"on": true, "police": true, "token": "feed-tok"}, true)
	if code, _ := get(""); code != 401 {
		t.Fatalf("no token: %d", code)
	}
	if code, body := get("feed-tok"); code != 200 || !strings.Contains(body, "0 block Elm St") {
		t.Fatalf("with the token and police: %d %s", code, body)
	}
}
