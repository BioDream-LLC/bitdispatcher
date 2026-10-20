// Package foia helps records staff prepare Freedom-of-Information (FOIA)
// disclosure copies of 911 incident records.
//
// When a member of the public asks for a 911 record, an agency must release it
// save for the parts the law lets it withhold: personal identifiers, medical
// details, information about children, and so on. Deciding what to withhold is
// a legal judgement that belongs to the agency and its counsel, not to a
// machine. This package therefore does not decide anything. It holds the
// agency's own disclosure rules (see [Rule]), finds every passage those rules
// match (see [Propose]), and lets a reviewer accept, reject, or add redactions
// by hand (see [Decision] and [Apply]). Nothing is hardcoded as always exempt.
//
// The built-in [Categories] are detectors only: they find things that often
// matter, such as a Social Security number or a date of birth, but whether a
// found passage is actually exempt is entirely the rule's decision. A rule
// pairs a detector (or a plain word list, or a regular expression) with the
// legal basis the agency relies on and a plain-words explanation.
//
// Redaction is irreversible. [Apply] replaces each accepted passage with a
// block that names the legal basis and keeps nothing of the original text, not
// even its length. [Validate] then checks a finished copy, and any metadata
// shipped with it, for redacted text that slipped through.
//
// The [SampleRules] are worked examples only. They are shipped disabled and
// marked as samples; an agency's counsel must review and adapt them before
// they are used. They are not legal advice.
//
// Everything here is deterministic: the same input always gives the same
// proposals, in the same order, with the same identifiers.
package foia

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Rule kinds, used in [Rule.Kind].
const (
	// KindPattern matches a Go regular expression held in [Rule.Pattern].
	KindPattern = "pattern"
	// KindWords matches any of the words or phrases in [Rule.Words],
	// case-insensitively and on whole words only.
	KindWords = "words"
	// KindCategory runs one of the built-in detectors named in
	// [Rule.Category]; see [Categories].
	KindCategory = "category"
)

// Confidence levels reported in [Proposal.Confidence].
const (
	// ConfidenceHigh marks a proposal from a precise source: a regular
	// expression, a word list, or an exact detector such as ssn, email, vin,
	// or phone.
	ConfidenceHigh = "high"
	// ConfidenceReview marks a proposal from a fuzzy detector, such as a name
	// pair, an address, a medical word, a juvenile reference, or a date of
	// birth with no label. A reviewer should look at these with extra care.
	ConfidenceReview = "review"
)

// Rule is one agency disclosure or exemption rule, maintained by the agency
// administrator and reviewed by counsel. A rule says what to look for (a
// detector, a word list, or a regular expression), the legal basis for
// withholding it, and when the rule is in effect.
type Rule struct {
	// ID identifies the rule and is woven into the identifier of every
	// proposal the rule makes.
	ID string `json:"id"`
	// Name is a short human-readable label.
	Name string `json:"name"`
	// Jurisdiction is the body of law the rule belongs to, for example
	// "Virginia". It is informational only.
	Jurisdiction string `json:"jurisdiction,omitempty"`
	// RecordTypes limits the rule to these record types, for example
	// "incident", "call", or "narrative". An empty list means every type.
	RecordTypes []string `json:"recordTypes,omitempty"`
	// Kind selects how the rule matches: [KindPattern], [KindWords], or
	// [KindCategory].
	Kind string `json:"kind"`
	// Pattern is a Go regular expression, used when Kind is [KindPattern].
	Pattern string `json:"pattern,omitempty"`
	// Words is a list of words or phrases, used when Kind is [KindWords].
	Words []string `json:"words,omitempty"`
	// Category names a built-in detector, used when Kind is [KindCategory];
	// see [Categories].
	Category string `json:"category,omitempty"`
	// Basis is the legal citation the agency relies on, for example
	// "Va. Code § 2.2-3706.1(B)(1)".
	Basis string `json:"basis"`
	// Explain says, in plain words, why the rule withholds what it matches.
	Explain string `json:"explain"`
	// From and To bound the dates the rule is in effect. A zero From means it
	// has always been in effect; a zero To means it has no end.
	From time.Time `json:"from,omitzero"`
	To   time.Time `json:"to,omitzero"`
	// Enabled reports whether the rule is live. Disabled rules are skipped.
	Enabled bool `json:"enabled"`
	// Sample marks a rule shipped as an example. The agency's counsel must
	// review it before use.
	Sample bool `json:"sample,omitempty"`
}

// Context describes the record being prepared and what is known about it.
type Context struct {
	// RecordType is the kind of record, for example "incident", "call", or
	// "narrative". It is matched against [Rule.RecordTypes].
	RecordType string
	// At is the moment the record is being prepared. A rule applies only if At
	// falls within its From and To dates.
	At time.Time
	// Names lists person names known for this record. The "name" detector
	// looks for these.
	Names []string
}

// Proposal is one proposed redaction: a byte range of the text that a rule
// matched, together with the rule's legal basis and explanation. A proposal is
// only a suggestion; a reviewer decides whether to accept it (see [Decision]).
type Proposal struct {
	// ID is a stable identifier derived from the rule, the byte range, and the
	// matched text: twelve hexadecimal characters. The same match from the
	// same rule always has the same ID.
	ID string `json:"id"`
	// Start and End are the byte offsets of the matched passage in the text,
	// as a half-open range text[Start:End].
	Start int `json:"start"`
	End   int `json:"end"`
	// Text is the matched passage.
	Text string `json:"text"`
	// Rule is the ID of the rule that made the proposal.
	Rule string `json:"rule"`
	// Basis is the rule's legal citation.
	Basis string `json:"basis"`
	// Explain is the rule's plain-words explanation.
	Explain string `json:"explain"`
	// Confidence is [ConfidenceHigh] or [ConfidenceReview].
	Confidence string `json:"confidence"`
	// Source is "rule" for proposals from this package. A caller may add its
	// own proposals from, say, a model and mark them differently.
	Source string `json:"source"`
}

// Decision is the reviewer's choice for a proposal, or a passage the reviewer
// marked by hand. A decision names the byte range, the text that range should
// still hold, whether to accept it, and who made the call.
type Decision struct {
	// ID is the proposal's ID, or empty for a hand-marked passage.
	ID string `json:"id"`
	// Start and End are the byte offsets of the passage.
	Start int `json:"start"`
	End   int `json:"end"`
	// Text is what text[Start:End] is expected to hold. [Apply] rejects the
	// decision if the text has changed, to guard against stale offsets.
	Text string `json:"text"`
	// Accept reports whether the passage is to be redacted.
	Accept bool `json:"accept"`
	// Basis is the legal citation shown in the redaction block.
	Basis string `json:"basis"`
	// Note is an optional free-text note from the reviewer.
	Note string `json:"note,omitempty"`
	// By names the reviewer.
	By string `json:"by"`
	// At is when the decision was made.
	At time.Time `json:"at"`
}

// Leak is a redacted passage that was still found in a released copy or its
// metadata.
type Leak struct {
	// Text is the redacted text that leaked.
	Text string `json:"text"`
	// Where says where it was found: "text" for the body, or the metadata
	// field name.
	Where string `json:"where"`
}

// Categories returns the names of the built-in detectors, sorted. Each is a
// value that [Rule.Category] may take when [Rule.Kind] is [KindCategory]. A
// detector only finds text; whether that text is exempt is the rule's
// decision.
func Categories() []string {
	cats := make([]string, 0, len(detectors))
	for name := range detectors {
		cats = append(cats, name)
	}
	sort.Strings(cats)
	return cats
}

// match is a raw hit from a detector, a word list, or a regular expression,
// before it is turned into a [Proposal].
type match struct {
	start, end int
	text       string
	high       bool // confidence: true for high, false for review
}

// detector finds passages of one built-in category in text. cx carries the
// known names and other context.
type detector func(text string, cx Context) []match

// detectors holds every built-in category detector by name. It is the single
// source of truth for [Categories].
var detectors = map[string]detector{
	"ssn":      detectSSN,
	"dob":      detectDOB,
	"phone":    detectPhone,
	"email":    detectEmail,
	"plate":    detectPlate,
	"vin":      detectVIN,
	"dl":       detectDL,
	"address":  detectAddress,
	"juvenile": detectJuvenile,
	"medical":  detectMedical,
	"name":     detectName,
}

// Propose finds every passage matched by the rules that apply to the record.
// A rule applies when it is enabled, in effect for cx.At (its From and To
// bracket cx.At), and either has no record-type restriction or lists
// cx.RecordType.
//
// Proposals are sorted by Start, and for equal Start the longest passage
// first. Overlapping proposals are kept; the reviewer sees them all. A rule
// whose Kind is [KindPattern] with an invalid regular expression is skipped and
// the error is returned in errs; the other rules still run.
func Propose(text string, rules []Rule, cx Context) (props []Proposal, errs []error) {
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if !ruleInEffect(r, cx.At) {
			continue
		}
		if !ruleCoversType(r, cx.RecordType) {
			continue
		}

		var ms []match
		switch r.Kind {
		case KindPattern:
			re, err := regexp.Compile(r.Pattern)
			if err != nil {
				errs = append(errs, fmt.Errorf("rule %q: invalid pattern: %w", r.ID, err))
				continue
			}
			for _, loc := range re.FindAllStringIndex(text, -1) {
				ms = append(ms, match{start: loc[0], end: loc[1], text: text[loc[0]:loc[1]], high: true})
			}
		case KindWords:
			ms = matchWords(text, r.Words)
		case KindCategory:
			det, ok := detectors[r.Category]
			if !ok {
				errs = append(errs, fmt.Errorf("rule %q: unknown category %q", r.ID, r.Category))
				continue
			}
			ms = det(text, cx)
		default:
			errs = append(errs, fmt.Errorf("rule %q: unknown kind %q", r.ID, r.Kind))
			continue
		}

		for _, m := range ms {
			if m.start < 0 || m.end > len(text) || m.start >= m.end {
				continue
			}
			conf := ConfidenceReview
			if m.high {
				conf = ConfidenceHigh
			}
			props = append(props, Proposal{
				ID:         proposalID(r.ID, m.start, m.end, m.text),
				Start:      m.start,
				End:        m.end,
				Text:       m.text,
				Rule:       r.ID,
				Basis:      r.Basis,
				Explain:    r.Explain,
				Confidence: conf,
				Source:     "rule",
			})
		}
	}

	sort.SliceStable(props, func(i, j int) bool {
		if props[i].Start != props[j].Start {
			return props[i].Start < props[j].Start
		}
		// Longest first for equal starts.
		if props[i].End != props[j].End {
			return props[i].End > props[j].End
		}
		return props[i].Rule < props[j].Rule
	})
	return props, errs
}

// ruleInEffect reports whether rule r is in effect at the moment at. A zero
// From means no start bound; a zero To means no end bound. The window is
// inclusive of From and To.
func ruleInEffect(r Rule, at time.Time) bool {
	if !r.From.IsZero() && at.Before(r.From) {
		return false
	}
	if !r.To.IsZero() && at.After(r.To) {
		return false
	}
	return true
}

// ruleCoversType reports whether rule r applies to the given record type. An
// empty RecordTypes list means the rule applies to every type.
func ruleCoversType(r Rule, recordType string) bool {
	if len(r.RecordTypes) == 0 {
		return true
	}
	for _, t := range r.RecordTypes {
		if strings.EqualFold(t, recordType) {
			return true
		}
	}
	return false
}

// proposalID derives a stable twelve-character identifier from the rule ID, the
// byte range, and the matched text.
func proposalID(ruleID string, start, end int, text string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d|%s", ruleID, start, end, text)))
	return hex.EncodeToString(sum[:])[:12]
}

// Apply returns the text with every accepted decision's range replaced,
// irreversibly, by a block "[REDACTED: <basis>]". The original characters are
// not kept in any form, not even their length.
//
// Each accepted decision is validated against the text: its Start and End must
// be a valid range, and text[Start:End] must equal Decision.Text. A decision
// that fails is reported in errs and skipped; this guards against the text
// changing under the reviewer after the decision was made. Rejected decisions
// (Accept false) are ignored.
//
// Overlapping accepted ranges are merged into one block, and their bases are
// joined with "; " in first-seen order with duplicates removed.
func Apply(text string, ds []Decision) (out string, errs []error) {
	type span struct {
		start, end int
		bases      []string
	}
	var spans []span
	for _, d := range ds {
		if !d.Accept {
			continue
		}
		if d.Start < 0 || d.End > len(text) || d.Start >= d.End {
			errs = append(errs, fmt.Errorf("decision %q: invalid range [%d:%d] for text of length %d", d.ID, d.Start, d.End, len(text)))
			continue
		}
		if text[d.Start:d.End] != d.Text {
			errs = append(errs, fmt.Errorf("decision %q: text changed under reviewer at [%d:%d]", d.ID, d.Start, d.End))
			continue
		}
		spans = append(spans, span{start: d.Start, end: d.End, bases: []string{d.Basis}})
	}
	if len(spans) == 0 {
		return text, errs
	}

	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return spans[i].end < spans[j].end
	})

	// Merge overlapping or touching spans.
	merged := []span{spans[0]}
	for _, s := range spans[1:] {
		last := &merged[len(merged)-1]
		if s.start <= last.end {
			if s.end > last.end {
				last.end = s.end
			}
			last.bases = append(last.bases, s.bases...)
		} else {
			merged = append(merged, s)
		}
	}

	var b strings.Builder
	prev := 0
	for _, s := range merged {
		b.WriteString(text[prev:s.start])
		b.WriteString("[REDACTED: ")
		b.WriteString(joinBases(s.bases))
		b.WriteString("]")
		prev = s.end
	}
	b.WriteString(text[prev:])
	return b.String(), errs
}

// joinBases joins legal bases with "; ", keeping first-seen order and dropping
// duplicates and empties.
func joinBases(bases []string) string {
	seen := make(map[string]bool)
	var kept []string
	for _, base := range bases {
		if base == "" || seen[base] {
			continue
		}
		seen[base] = true
		kept = append(kept, base)
	}
	return strings.Join(kept, "; ")
}

// Validate checks a released copy, and the values of its metadata fields, for
// any accepted redacted text. It catches text that was supposed to be withheld
// but still appears in the finished product.
//
// Matching is case-insensitive and whitespace-normalised. For passages that are
// mostly digits, such as phone numbers or Social Security numbers of seven or
// more digits, a digits-only form is also sought, so that "540-555-2348"
// catches "5405552348" and the reverse. Rejected decisions are ignored.
//
// Every leak found is returned, in the order the decisions were given and, for
// each decision, body before metadata (metadata fields in sorted name order).
func Validate(released string, meta map[string]string, ds []Decision) []Leak {
	bodyNorm := normalise(released)
	bodyDigits := digitsOnly(released)

	metaNames := make([]string, 0, len(meta))
	for name := range meta {
		metaNames = append(metaNames, name)
	}
	sort.Strings(metaNames)

	type normField struct {
		where       string
		norm, digit string
	}
	metaFields := make([]normField, 0, len(metaNames))
	for _, name := range metaNames {
		metaFields = append(metaFields, normField{
			where: name,
			norm:  normalise(meta[name]),
			digit: digitsOnly(meta[name]),
		})
	}

	var leaks []Leak
	seen := make(map[string]bool) // text+where, so a passage is reported once per place
	for _, d := range ds {
		if !d.Accept || d.Text == "" {
			continue
		}
		needle := normalise(d.Text)
		needleDigits := digitsOnly(d.Text)
		useDigits := len(needleDigits) >= 7

		check := func(where, norm, digit string) {
			found := needle != "" && strings.Contains(norm, needle)
			if !found && useDigits && len(digit) > 0 {
				found = strings.Contains(digit, needleDigits)
			}
			if found {
				key := d.Text + "\x00" + where
				if !seen[key] {
					seen[key] = true
					leaks = append(leaks, Leak{Text: d.Text, Where: where})
				}
			}
		}

		check("text", bodyNorm, bodyDigits)
		for _, f := range metaFields {
			check(f.where, f.norm, f.digit)
		}
	}
	return leaks
}

// normalise lower-cases text and collapses every run of whitespace to a single
// space, trimming the ends, so that matching ignores case and layout.
func normalise(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// digitsOnly returns just the decimal digits of s, in order.
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteByte(byte(r))
		}
	}
	return b.String()
}

// matchWords finds whole-word, case-insensitive matches of any word or phrase
// in words.
func matchWords(text string, words []string) []match {
	var ms []match
	for _, w := range words {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		re, err := regexp.Compile(`(?i)` + wordBoundary(regexp.QuoteMeta(w)))
		if err != nil {
			continue
		}
		for _, loc := range re.FindAllStringIndex(text, -1) {
			ms = append(ms, match{start: loc[0], end: loc[1], text: text[loc[0]:loc[1]], high: true})
		}
	}
	return ms
}

// wordBoundary wraps a (possibly multi-word) quoted expression so that it only
// matches on whole-word boundaries. A plain \b does not work when the phrase
// starts or ends with a non-word character, so we anchor on the edge
// characters of the phrase.
func wordBoundary(quoted string) string {
	return `\b` + quoted + `\b`
}

// --- Built-in detectors ---------------------------------------------------
//
// Each detector returns raw matches. A detector only finds text; whether a
// found passage is exempt is the rule's decision. "high" marks a precise find;
// "review" marks a fuzzy one the reviewer should check.

var (
	reSSNFormatted = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	reSSNBare      = regexp.MustCompile(`\b\d{9}\b`)
	reEmail        = regexp.MustCompile(`(?i)\b[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}\b`)
	rePhone        = regexp.MustCompile(`(?:\+1[\s.\-]?)?(?:\(\d{3}\)\s?|\d{3}[\s.\-])\d{3}[\s.\-]\d{4}\b`)
	reVIN          = regexp.MustCompile(`\b[A-HJ-NPR-Z0-9]{17}\b`)
	rePlate        = regexp.MustCompile(`(?i)\b(?:plate|tag|registration)\b[\s:]*(?:[A-Z]{2}\s+)?([A-Z0-9]{2,3}[\s\-]?[A-Z0-9]{2,4})`)
	reDL           = regexp.MustCompile(`(?i)\b(?:dl|driver'?s?\s+licen[cs]e|licen[cs]e)\b(?:\s*(?:no\.?|number|#|:))?\s*((?:[A-Z]{1,2}\s+)?[A-Z]?\d[\dA-Z]{4,}(?:\s+[A-Z]?\d[\dA-Z]{3,})?)`)
	reAddress      = regexp.MustCompile(`(?i)\b\d{1,6}\s+(?:[A-Z0-9][A-Za-z0-9'.]*\s+){0,4}(?:St|Street|Ave|Avenue|Rd|Road|Ln|Lane|Dr|Drive|Ct|Court|Blvd|Boulevard|Way|Pl|Place|Pkwy|Parkway|Cir|Circle|Hwy|Highway|Ter|Terrace|Trl|Trail)\b\.?(?:\s+(?:NE|NW|SE|SW|N|S|E|W))?`)
	reDOBLabelled  = regexp.MustCompile(`(?i)\b(?:DOB|D\.O\.B\.?|date of birth|born)\b[\s:]*(\d{1,2}[/\-.]\d{1,2}[/\-.]\d{2,4}|[A-Z][a-z]+\.?\s+\d{1,2},?\s+\d{4})`)
	reDOBBare      = regexp.MustCompile(`\b\d{1,2}[/\-.]\d{1,2}[/\-.]\d{4}\b`)
	reJuvAge       = regexp.MustCompile(`(?i)\b(\d{1,2})[\s\-]?(?:year[\s\-]?old|yo|y/o)\b`)
	reJuvAgeOf     = regexp.MustCompile(`(?i)\bage[d]?\s+(\d{1,2})\b`)
	reJuvWord      = regexp.MustCompile(`(?i)\b(?:juvenile|minor child|minor|unaccompanied minor)\b`)
	reNamePair     = regexp.MustCompile(`\b[A-Z][A-Z'\-]+,\s+[A-Z][A-Z'\-]+\b`)
)

// detectSSN finds Social Security numbers: the dashed form always, and a bare
// nine-digit run only when the letters "SSN" appear close by.
func detectSSN(text string, _ Context) []match {
	var ms []match
	for _, loc := range reSSNFormatted.FindAllStringIndex(text, -1) {
		ms = append(ms, match{start: loc[0], end: loc[1], text: text[loc[0]:loc[1]], high: true})
	}
	for _, loc := range reSSNBare.FindAllStringIndex(text, -1) {
		if hasNearby(text, loc[0], loc[1], 15, "ssn", "social security") {
			ms = append(ms, match{start: loc[0], end: loc[1], text: text[loc[0]:loc[1]], high: true})
		}
	}
	return ms
}

// detectDOB finds dates of birth. A date next to a DOB/born label is a high
// confidence find; a bare date with no label is offered for review.
func detectDOB(text string, _ Context) []match {
	var ms []match
	labelled := make(map[int]bool)
	for _, loc := range reDOBLabelled.FindAllStringSubmatchIndex(text, -1) {
		s, e := loc[2], loc[3]
		ms = append(ms, match{start: s, end: e, text: text[s:e], high: true})
		labelled[s] = true
	}
	for _, loc := range reDOBBare.FindAllStringIndex(text, -1) {
		if !labelled[loc[0]] {
			ms = append(ms, match{start: loc[0], end: loc[1], text: text[loc[0]:loc[1]], high: false})
		}
	}
	return ms
}

// detectPhone finds US telephone numbers, with or without a +1 country code
// and in the common bracket, dot, dash, or space layouts.
func detectPhone(text string, _ Context) []match {
	var ms []match
	for _, loc := range rePhone.FindAllStringIndex(text, -1) {
		ms = append(ms, match{start: loc[0], end: loc[1], text: text[loc[0]:loc[1]], high: true})
	}
	return ms
}

// detectEmail finds email addresses.
func detectEmail(text string, _ Context) []match {
	var ms []match
	for _, loc := range reEmail.FindAllStringIndex(text, -1) {
		ms = append(ms, match{start: loc[0], end: loc[1], text: text[loc[0]:loc[1]], high: true})
	}
	return ms
}

// detectPlate finds a licence plate given near the words plate, tag, or
// registration, returning just the plate itself.
func detectPlate(text string, _ Context) []match {
	var ms []match
	for _, loc := range rePlate.FindAllStringSubmatchIndex(text, -1) {
		s, e := loc[2], loc[3]
		if s < 0 {
			continue
		}
		ms = append(ms, match{start: s, end: e, text: text[s:e], high: true})
	}
	return ms
}

// detectVIN finds seventeen-character vehicle identification numbers. VINs omit
// the letters I, O, and Q.
func detectVIN(text string, _ Context) []match {
	var ms []match
	for _, loc := range reVIN.FindAllStringIndex(text, -1) {
		s := text[loc[0]:loc[1]]
		// A VIN must contain at least one digit; a plain 17-letter word is not
		// a VIN.
		if !strings.ContainsAny(s, "0123456789") {
			continue
		}
		ms = append(ms, match{start: loc[0], end: loc[1], text: s, high: true})
	}
	return ms
}

// detectDL finds driver's licence numbers given near "DL" or "licence",
// returning just the number.
func detectDL(text string, _ Context) []match {
	var ms []match
	for _, loc := range reDL.FindAllStringSubmatchIndex(text, -1) {
		s, e := loc[2], loc[3]
		if s < 0 {
			continue
		}
		ms = append(ms, match{start: s, end: e, text: text[s:e], high: true})
	}
	return ms
}

// detectAddress finds street addresses: a house number followed by a street
// name and a suffix such as St, Ave, Rd, Ln, or Dr. Addresses are fuzzy, so
// they are offered for review.
func detectAddress(text string, _ Context) []match {
	var ms []match
	for _, loc := range reAddress.FindAllStringIndex(text, -1) {
		ms = append(ms, match{start: loc[0], end: loc[1], text: text[loc[0]:loc[1]], high: false})
	}
	return ms
}

// detectJuvenile finds references to a person under eighteen: an age such as
// "15-year-old" or "age 12", or the words juvenile or minor child. It returns
// the matched phrase. Such references are fuzzy, so they are offered for
// review.
func detectJuvenile(text string, _ Context) []match {
	var ms []match
	add := func(s, e int) {
		ms = append(ms, match{start: s, end: e, text: text[s:e], high: false})
	}
	for _, loc := range reJuvAge.FindAllStringSubmatchIndex(text, -1) {
		if age, ok := atoiSmall(text[loc[2]:loc[3]]); ok && age < 18 {
			add(loc[0], loc[1])
		}
	}
	for _, loc := range reJuvAgeOf.FindAllStringSubmatchIndex(text, -1) {
		if age, ok := atoiSmall(text[loc[2]:loc[3]]); ok && age < 18 {
			add(loc[0], loc[1])
		}
	}
	for _, loc := range reJuvWord.FindAllStringIndex(text, -1) {
		add(loc[0], loc[1])
	}
	return ms
}

// medicalWords are common words and phrases that signal a medical condition.
// They are matched whole-word and case-insensitively.
var medicalWords = []string{
	"overdose", "overdosed", "seizure", "seizures", "diabetic", "diabetes",
	"psychiatric", "suicidal", "suicide", "HIV", "AIDS", "pregnant",
	"pregnancy", "cardiac arrest", "heart attack", "stroke", "unconscious",
	"unresponsive", "mental health", "intoxicated", "narcan", "naloxone",
	"epileptic", "epilepsy", "dementia", "Alzheimer's", "schizophrenia",
	"bipolar", "depression", "self-harm", "overdosing",
}

var reMedical = func() *regexp.Regexp {
	parts := make([]string, len(medicalWords))
	for i, w := range medicalWords {
		parts[i] = regexp.QuoteMeta(w)
	}
	return regexp.MustCompile(`(?i)\b(?:` + strings.Join(parts, "|") + `)\b`)
}()

// detectMedical finds common medical-condition words. Medical detail is fuzzy,
// so matches are offered for review.
func detectMedical(text string, _ Context) []match {
	var ms []match
	for _, loc := range reMedical.FindAllStringIndex(text, -1) {
		ms = append(ms, match{start: loc[0], end: loc[1], text: text[loc[0]:loc[1]], high: false})
	}
	return ms
}

// detectName finds person names. Exact names given in cx.Names are matched
// whole-word and case-insensitively and are a high confidence find. Upper-case
// "LASTNAME, FIRSTNAME" pairs are also found and offered for review.
func detectName(text string, cx Context) []match {
	var ms []match
	for _, name := range cx.Names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		re, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(name) + `\b`)
		if err != nil {
			continue
		}
		for _, loc := range re.FindAllStringIndex(text, -1) {
			ms = append(ms, match{start: loc[0], end: loc[1], text: text[loc[0]:loc[1]], high: true})
		}
	}
	for _, loc := range reNamePair.FindAllStringIndex(text, -1) {
		ms = append(ms, match{start: loc[0], end: loc[1], text: text[loc[0]:loc[1]], high: false})
	}
	return ms
}

// hasNearby reports whether any of the given needles (already lower-case)
// appears within radius bytes either side of the range [start:end].
func hasNearby(text string, start, end, radius int, needles ...string) bool {
	lo := start - radius
	if lo < 0 {
		lo = 0
	}
	hi := end + radius
	if hi > len(text) {
		hi = len(text)
	}
	window := strings.ToLower(text[lo:hi])
	for _, n := range needles {
		if strings.Contains(window, n) {
			return true
		}
	}
	return false
}

// atoiSmall parses a short run of ASCII digits into an int, reporting whether
// it was valid. It avoids pulling in strconv for a tiny job and never panics.
func atoiSmall(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}

// SampleRules returns worked example rules. Each is shipped disabled and
// marked as a sample, with a placeholder basis telling the agency to have its
// counsel confirm the citation. They cover the ssn, dob, phone, juvenile,
// medical, plate, and name detectors. They are examples, not legal advice.
func SampleRules() []Rule {
	const confirm = "(SAMPLE — have agency counsel confirm the correct citation before use)"
	note := func(s string) string {
		return "Example only, not legal advice. " + s + " Your agency's counsel must review and adapt this rule before use."
	}
	return []Rule{
		{
			ID: "sample-ssn", Name: "Social Security numbers", Kind: KindCategory, Category: "ssn",
			Basis: confirm, Explain: note("Social Security numbers are a personal identifier agencies commonly withhold."),
			Enabled: false, Sample: true,
		},
		{
			ID: "sample-dob", Name: "Dates of birth", Kind: KindCategory, Category: "dob",
			Basis: confirm, Explain: note("A date of birth is a personal identifier agencies commonly withhold."),
			Enabled: false, Sample: true,
		},
		{
			ID: "sample-phone", Name: "Telephone numbers", Kind: KindCategory, Category: "phone",
			Basis: confirm, Explain: note("Personal telephone numbers are contact details agencies commonly withhold."),
			Enabled: false, Sample: true,
		},
		{
			ID: "sample-juvenile", Name: "References to children", Kind: KindCategory, Category: "juvenile",
			Basis: confirm, Explain: note("Information identifying a child is commonly withheld to protect minors."),
			Enabled: false, Sample: true,
		},
		{
			ID: "sample-medical", Name: "Medical information", Kind: KindCategory, Category: "medical",
			Basis: confirm, Explain: note("Medical detail is health information agencies commonly withhold."),
			Enabled: false, Sample: true,
		},
		{
			ID: "sample-plate", Name: "Vehicle licence plates", Kind: KindCategory, Category: "plate",
			Basis: confirm, Explain: note("A licence plate can identify a private individual and is commonly withheld."),
			Enabled: false, Sample: true,
		},
		{
			ID: "sample-name", Name: "Person names", Kind: KindCategory, Category: "name",
			Basis: confirm, Explain: note("Names of private individuals are commonly withheld."),
			Enabled: false, Sample: true,
		},
	}
}
