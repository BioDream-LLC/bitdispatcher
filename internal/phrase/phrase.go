// Package phrase detects agency-configured critical phrases in 911 CAD call
// and radio transcripts.
//
// A public-safety answering point configures Rules describing critical
// phrases such as "shots fired", "officer down", "bomb", "not breathing" or
// "CPR". Check scans a piece of transcript (a Text) for every applicable rule
// and returns the matches, handling case, punctuation, word-boundary and
// prefix-wildcard matching, and sentence-scoped negation ("there are no shots
// fired" is matched but reported with Negated=true so a supervisor can review
// it). Matching is performed on normalized, lowercased, Unicode-aware word
// tokens; sentence boundaries are . ! ? ; and newlines.
//
// Copyright BioDream LLC. Licensed under the Apache License, Version 2.0.
package phrase

import (
	"sort"
	"strings"
	"unicode"
)

// Rule is one agency phrase rule.
type Rule struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`                     // shown to the supervisor, e.g. "Shots fired"
	Phrases        []string `json:"phrases"`                  // variants, e.g. "shots fired", "shot fired", "gun shots"; matched case-insensitively on word boundaries; '*' at the end of a word is a prefix wildcard ("shoot*")
	Negations      []string `json:"negations,omitempty"`      // words/phrases that, when they occur within NegationWindow words BEFORE the match in the same sentence, negate it, e.g. "no", "not", "never", "negative", "false", "no sign of", "cancel"; empty -> DefaultNegations
	NegationWindow int      `json:"negationWindow,omitempty"` // words; 0 -> 3
	Sources        []string `json:"sources,omitempty"`        // "call", "radio", "text", "narrative"; empty = all
	CallTypes      []string `json:"callTypes,omitempty"`      // empty = all
	MinConfidence  float64  `json:"minConfidence,omitempty"`  // transcript confidence 0..1 below which a match is reported as Uncertain (not dropped)
	Severity       string   `json:"severity,omitempty"`       // urgent, high, medium, low; empty -> high
	Testing        bool     `json:"testing,omitempty"`        // testing mode: matches are reported with Testing=true so they are not alerted live
	Off            bool     `json:"off,omitempty"`
}

// DefaultNegations is used when a Rule has no Negations configured.
var DefaultNegations = []string{"no", "not", "never", "negative", "false", "no sign of", "no report of", "cancel", "cancelled", "unfounded", "without"}

// Text is one piece of transcript to check.
type Text struct {
	Source, CallType string
	Text             string
	Confidence       float64 // 0 = unknown
}

// Match is one rule hit.
type Match struct {
	Rule      string `json:"rule"`
	Name      string `json:"name"`
	Phrase    string `json:"phrase"`  // the variant that matched
	Found     string `json:"found"`   // the exact text matched
	Start     int    `json:"start"`   // byte offset in Text
	End       int    `json:"end"`     // byte offset in Text
	Context   string `json:"context"` // up to ~8 words either side, with "…" when cut
	Negated   bool   `json:"negated"`
	Negation  string `json:"negation,omitempty"` // the negating word found
	Uncertain bool   `json:"uncertain"`          // confidence known and below MinConfidence
	Testing   bool   `json:"testing"`
	Severity  string `json:"severity"`
}

const (
	defaultNegationWindow = 3
	defaultSeverity       = "high"
	contextWords          = 8
)

// token is a normalized word with its byte span and sentence index in the
// original text.
type token struct {
	norm     string // normalized (lowercased) word
	start    int    // byte offset of first rune in the original text
	end      int    // byte offset just past the last rune in the original text
	sentence int    // sentence index (increments at . ! ? ; and newlines)
}

// tokenize splits s into word tokens. Words are runs of letters, digits and
// apostrophes (Unicode-aware). Sentence boundaries are . ! ? ; and newlines.
func tokenize(s string) []token {
	var toks []token
	sentence := 0
	wordStart := -1
	for i, r := range s {
		if isWordRune(r) {
			if wordStart < 0 {
				wordStart = i
			}
			continue
		}
		// Non-word rune: close any open word.
		if wordStart >= 0 {
			toks = append(toks, token{
				norm:     strings.ToLower(s[wordStart:i]),
				start:    wordStart,
				end:      i,
				sentence: sentence,
			})
			wordStart = -1
		}
		if isSentenceBoundary(r) {
			sentence++
		}
	}
	if wordStart >= 0 {
		toks = append(toks, token{
			norm:     strings.ToLower(s[wordStart:]),
			start:    wordStart,
			end:      len(s),
			sentence: sentence,
		})
	}
	return toks
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\''
}

func isSentenceBoundary(r rune) bool {
	switch r {
	case '.', '!', '?', ';', '\n':
		return true
	}
	return false
}

// phraseWord is one normalized token of a phrase variant, with an optional
// prefix-wildcard flag.
type phraseWord struct {
	text   string // normalized word (without trailing '*')
	prefix bool   // true if this was a "word*" prefix wildcard
}

// parsePhrase turns a phrase variant into its sequence of words. A trailing
// '*' on a word marks a prefix wildcard. Returns nil if the phrase has no
// words.
func parsePhrase(p string) []phraseWord {
	toks := tokenizePhrase(p)
	if len(toks) == 0 {
		return nil
	}
	out := make([]phraseWord, 0, len(toks))
	for _, w := range toks {
		pw := phraseWord{}
		if strings.HasSuffix(w.raw, "*") {
			pw.prefix = true
		}
		pw.text = w.norm
		out = append(out, pw)
	}
	return out
}

// rawToken carries the raw (original-case) slice plus its normalized form so
// phrase parsing can see a trailing '*'.
type rawToken struct {
	raw  string
	norm string
}

// tokenizePhrase splits a phrase variant into words, keeping a trailing '*'
// attached to its word so prefix wildcards can be detected.
func tokenizePhrase(s string) []rawToken {
	var toks []rawToken
	wordStart := -1
	flush := func(end int) {
		if wordStart < 0 {
			return
		}
		raw := s[wordStart:end]
		toks = append(toks, rawToken{raw: raw, norm: strings.ToLower(strings.TrimSuffix(raw, "*"))})
		wordStart = -1
	}
	for i, r := range s {
		if isWordRune(r) || r == '*' {
			if wordStart < 0 {
				wordStart = i
			}
			continue
		}
		flush(i)
	}
	flush(len(s))
	return toks
}

// matchAt reports whether the phrase words match the token slice starting at
// index i, returning the number of tokens consumed (0 if no match).
func matchAt(toks []token, i int, words []phraseWord) int {
	if i+len(words) > len(toks) {
		return 0
	}
	for j, w := range words {
		tk := toks[i+j]
		if w.prefix {
			if !strings.HasPrefix(tk.norm, w.text) {
				return 0
			}
		} else if tk.norm != w.text {
			return 0
		}
	}
	return len(words)
}

// Check returns every match of every applicable rule. Rules that are Off, or
// whose Sources/CallTypes exclude the text, are skipped. Negated matches are
// returned with Negated=true (the caller decides what to do with them).
// Matches are ordered by Start then rule ID; one rule yields at most one match
// per occurrence position (overlapping variants of the same rule at the same
// place collapse to the longest).
func Check(rules []Rule, t Text) []Match {
	toks := tokenize(t.Text)
	var out []Match

	for _, r := range rules {
		if r.Off {
			continue
		}
		if !includes(r.Sources, t.Source) {
			continue
		}
		if !includes(r.CallTypes, t.CallType) {
			continue
		}

		negations := r.Negations
		if len(negations) == 0 {
			negations = DefaultNegations
		}
		window := r.NegationWindow
		if window == 0 {
			window = defaultNegationWindow
		}
		severity := r.Severity
		if severity == "" {
			severity = defaultSeverity
		}

		// Parse variants once.
		type variant struct {
			raw   string
			words []phraseWord
		}
		var variants []variant
		for _, p := range r.Phrases {
			words := parsePhrase(p)
			if len(words) == 0 {
				continue
			}
			variants = append(variants, variant{raw: p, words: words})
		}

		// For each starting token position, find the longest matching
		// variant of this rule (collapse overlapping variants at the same
		// place to the longest).
		for i := 0; i < len(toks); i++ {
			bestLen := 0
			bestVariant := -1
			for vi, v := range variants {
				n := matchAt(toks, i, v.words)
				if n > bestLen {
					bestLen = n
					bestVariant = vi
				}
			}
			if bestVariant < 0 {
				continue
			}
			v := variants[bestVariant]
			startTok := toks[i]
			endTok := toks[i+bestLen-1]

			m := Match{
				Rule:     r.ID,
				Name:     r.Name,
				Phrase:   v.raw,
				Found:    t.Text[startTok.start:endTok.end],
				Start:    startTok.start,
				End:      endTok.end,
				Context:  buildContext(t.Text, toks, i, i+bestLen-1),
				Testing:  r.Testing,
				Severity: severity,
			}

			// Negation: look back within the same sentence, within window
			// words BEFORE the match. Ignore negation words that are part of
			// the matched phrase itself (tokens i..i+bestLen-1).
			neg, negWord := findNegation(toks, i, bestLen, window, negations)
			if neg {
				m.Negated = true
				m.Negation = negWord
			}

			// Confidence.
			if t.Confidence > 0 && r.MinConfidence > 0 && t.Confidence < r.MinConfidence {
				m.Uncertain = true
			}

			out = append(out, m)

			// Advance past this match to avoid re-matching inside it; the
			// next iteration's i++ moves to the token after the match.
			i += bestLen - 1
		}
	}

	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Start != out[b].Start {
			return out[a].Start < out[b].Start
		}
		return out[a].Rule < out[b].Rule
	})
	return out
}

// findNegation scans the window tokens before the match (indices start-1 down
// to start-window) within the same sentence for any negation word or phrase.
// Negation tokens that fall inside the matched phrase span itself are not
// considered. Negation entries may be multi-word; a multi-word negation must
// appear contiguously and end before the match.
func findNegation(toks []token, start, matchLen, window int, negations []string) (bool, string) {
	if start == 0 {
		return false, ""
	}
	sentence := toks[start].sentence
	// Lowest token index we are allowed to inspect (same sentence, within
	// window words before the match).
	lo := start - window
	if lo < 0 {
		lo = 0
	}
	// Try each negation entry. Prefer the one closest to the match (largest
	// end index), scanning positions from just before the match backwards.
	for pos := start - 1; pos >= lo; pos-- {
		if toks[pos].sentence != sentence {
			break
		}
		for _, neg := range negations {
			nwords := negationWords(neg)
			if len(nwords) == 0 {
				continue
			}
			// The negation phrase ends at pos, so it occupies tokens
			// pos-len+1 .. pos.
			begin := pos - len(nwords) + 1
			if begin < 0 {
				continue
			}
			if toks[begin].sentence != sentence {
				continue
			}
			if matchNegation(toks, begin, nwords) {
				return true, neg
			}
		}
	}
	return false, ""
}

// negationWords normalizes a negation entry into its word tokens.
func negationWords(neg string) []string {
	toks := tokenize(neg)
	out := make([]string, 0, len(toks))
	for _, t := range toks {
		out = append(out, t.norm)
	}
	return out
}

func matchNegation(toks []token, begin int, words []string) bool {
	if begin+len(words) > len(toks) {
		return false
	}
	for j, w := range words {
		if toks[begin+j].norm != w {
			return false
		}
	}
	return true
}

// buildContext returns up to contextWords tokens on either side of the matched
// span, using the original text slice, with a leading/trailing "…" when the
// context was cut.
func buildContext(s string, toks []token, startIdx, endIdx int) string {
	lo := startIdx - contextWords
	trimLeft := lo > 0
	if lo < 0 {
		lo = 0
	}
	hi := endIdx + contextWords
	trimRight := hi < len(toks)-1
	if hi > len(toks)-1 {
		hi = len(toks) - 1
	}
	ctx := s[toks[lo].start:toks[hi].end]
	ctx = strings.TrimSpace(ctx)
	if trimLeft {
		ctx = "…" + ctx
	}
	if trimRight {
		ctx = ctx + "…"
	}
	return ctx
}

// includes reports whether value is allowed given a filter list. An empty
// filter list allows everything. Comparison is case-insensitive. An empty
// value is allowed only when the filter is empty.
func includes(filter []string, value string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, f := range filter {
		if strings.EqualFold(strings.TrimSpace(f), strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}

// Validate reports problems with a rule as human-readable sentences. It
// checks for a missing ID, missing name, missing phrases, empty phrase
// variants, a bad severity, a wildcard '*' that is not at the end of a word,
// and a negation window greater than 10.
func Validate(r Rule) []string {
	var problems []string
	if strings.TrimSpace(r.ID) == "" {
		problems = append(problems, "The rule has no ID.")
	}
	if strings.TrimSpace(r.Name) == "" {
		problems = append(problems, "The rule has no name.")
	}
	if len(r.Phrases) == 0 {
		problems = append(problems, "The rule has no phrases.")
	}
	for _, p := range r.Phrases {
		if strings.TrimSpace(p) == "" {
			problems = append(problems, "The rule has an empty phrase.")
			continue
		}
		if badWildcard(p) {
			problems = append(problems, "The phrase "+quote(p)+" has a wildcard '*' that is not at the end of a word.")
		}
	}
	switch r.Severity {
	case "", "urgent", "high", "medium", "low":
		// valid
	default:
		problems = append(problems, "The severity "+quote(r.Severity)+" is not one of urgent, high, medium or low.")
	}
	if r.NegationWindow > 10 {
		problems = append(problems, "The negation window is greater than 10 words.")
	}
	return problems
}

// badWildcard reports whether the phrase contains a '*' that is not at the end
// of a word (a '*' is only valid as a trailing prefix-wildcard marker).
func badWildcard(p string) bool {
	runes := []rune(p)
	for i, r := range runes {
		if r != '*' {
			continue
		}
		// A '*' is valid only if it is the last rune of the string or the
		// next rune is a non-word, non-'*' rune (i.e. it ends a word).
		if i == len(runes)-1 {
			continue
		}
		next := runes[i+1]
		if isWordRune(next) || next == '*' {
			return true
		}
	}
	return false
}

func quote(s string) string {
	return "\"" + s + "\""
}

// Examples returns a starter set of rules marked Testing=true.
func Examples() []Rule {
	return []Rule{
		{
			ID:       "shots-fired",
			Name:     "Shots fired",
			Phrases:  []string{"shots fired", "shot fired", "gun shots", "gunshots", "shoot*"},
			Severity: "urgent",
			Testing:  true,
		},
		{
			ID:       "officer-down",
			Name:     "Officer down / officer needs assistance",
			Phrases:  []string{"officer down", "officer needs assistance", "officer needs help", "man down"},
			Severity: "urgent",
			Testing:  true,
		},
		{
			ID:       "bomb",
			Name:     "Bomb / explosive device",
			Phrases:  []string{"bomb", "explosive device", "explosives", "ied"},
			Severity: "urgent",
			Testing:  true,
		},
		{
			ID:   "not-breathing",
			Name: "Not breathing / CPR",
			// "not breathing" must NOT be self-negated: "not" is part of the
			// phrase. The negation check ignores negation words inside the
			// matched phrase span.
			Phrases:  []string{"not breathing", "cpr", "cpr in progress"},
			Severity: "high",
			Testing:  true,
		},
		{
			ID:       "threat-officer",
			Name:     "Threat against an officer",
			Phrases:  []string{"threat against an officer", "threaten* an officer", "kill the officer", "kill the cop"},
			Severity: "high",
			Testing:  true,
		},
		{
			ID:       "use-of-force",
			Name:     "Use of force",
			Phrases:  []string{"use of force", "used force", "taser deployed", "shots by officer"},
			Severity: "medium",
			Testing:  true,
		},
		{
			ID:   "child-not-breathing",
			Name: "Child not breathing",
			Phrases: []string{
				"child not breathing", "child is not breathing",
				"baby not breathing", "baby is not breathing",
				"infant not breathing", "infant is not breathing",
			},
			Severity: "urgent",
			Testing:  true,
		},
	}
}
