package main

// A simulated pager service for the demonstration: it takes pages the way ntfy does (POST /<topic>, the
// page text as the body, the answer link in the Click header), and its simulated responders answer each
// page through their link a little later: most responding, some straight to the scene, some not available.
// Plain HTTP on a loopback address; nothing leaves the computer.

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// runPager serves the simulated pager at addr; cad is bitdispatcher's address the answers go to (its
// API, whatever address the answer link names).
func runPager(addr, cad string) {
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}} // a simulator talking to a local server
	mux := http.NewServeMux()
	mux.HandleFunc("POST /{topic}", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 8<<10))
		link := r.Header.Get("Click")
		log.Printf("pager: page to %s: %s %q", r.PathValue("topic"), r.Header.Get("Title"), firstLine(string(body)))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"sim%d","topic":%q}`, time.Now().UnixNano(), r.PathValue("topic"))
		id, tok, ok := replyParts(link)
		if !ok {
			return
		}
		go func() {
			time.Sleep(time.Duration(20+rand.IntN(70)) * time.Second)
			answer, eta := "responding", 4+rand.IntN(10)
			switch n := rand.IntN(10); {
			case n < 2:
				answer, eta = "unavailable", 0
			case n < 4:
				answer = "scene"
			}
			b, _ := json.Marshal(map[string]any{"t": tok, "answer": answer, "eta": eta})
			req, _ := http.NewRequest("POST", strings.TrimRight(cad, "/")+"/api/page-reply/"+url.PathEscape(id), bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-BitDispatcher", "1")
			resp, err := client.Do(req)
			if err != nil {
				log.Printf("pager: answer: %v", err)
				return
			}
			resp.Body.Close()
		}()
	})
	log.Printf("simulated pager on http://%s (ntfy-style)", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

// replyParts takes the page and token from an answer link: https://host/#/r/<page>/<token>.
func replyParts(link string) (page, token string, ok bool) {
	i := strings.Index(link, "#/r/")
	if i < 0 {
		return "", "", false
	}
	parts := strings.Split(link[i+4:], "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}
