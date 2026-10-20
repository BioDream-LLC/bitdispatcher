// Package guidance is a 911 centre's library of agency-approved policies,
// call-taking guides, emergency response guides and checklists. When a
// call-taker enters an incident, the console shows the specific relevant
// passages (not whole manuals), each with a citation back to the approved
// document it came from.
//
// The guiding rule is that nothing here is invented. Every passage the console
// shows is verbatim from an approved document: this package only selects and
// ranks passages, it never rewrites or summarises them. A document carries a
// kind that says what weight its words carry (a [Policy] is mandatory, a
// [Guide] is operational guidance, a [Prompt] suggests questions to ask, a
// [Checklist] lists steps), an approval record, and effective and expiry
// dates so that only current, approved text is shown.
//
// A document's Markdown text is divided into sections by its headings (see
// [Sections]). Each document, and optionally each section, is mapped to the
// incidents it applies to by call type, modifier and trigger word. Given an
// incident (see [Context]), [Match] returns the passages that apply, ranked,
// with the reasons they matched and a status that flags anything outdated.
//
// The thresholds and wording are deliberately plain so that a call-taker under
// pressure can read a passage, see why it was shown, and trust that it is the
// agency's approved text.
package guidance

import (
	"sort"
	"strings"
	"time"
)

// Kinds of document: what weight a document's words carry. The kind also sets
// the order in which passages of equal score are shown (policy first).
const (
	// Policy is mandatory: the agency requires it be followed.
	Policy = "policy"
	// Guide is operational guidance for handling a call.
	Guide = "guide"
	// Prompt holds suggested questions or prompts to put to the caller.
	Prompt = "prompt"
	// Checklist is a list of steps to work through.
	Checklist = "checklist"
)

// Doc is one approved document in the library.
type Doc struct {
	// ID identifies the document, for example "SOP-210".
	ID string `json:"id"`
	// Title is the human-readable name.
	Title string `json:"title"`
	// Kind is one of the kind constants: policy, guide, prompt, or checklist.
	Kind string `json:"kind"`
	// Version is the document's version string, for example "3.1".
	Version string `json:"version"`
	// Effective is the date from which the document applies. A document whose
	// Effective date is in the future is never shown.
	Effective time.Time `json:"effective"`
	// Expires is the date after which the document no longer applies, if set.
	// An expired document is still returned, but marked outdated.
	Expires time.Time `json:"expires,omitzero"`
	// Supersedes is the ID of the document this one replaces, if any. A
	// document that is superseded by another approved, in-effect document is
	// marked outdated.
	Supersedes string `json:"supersedes,omitempty"`
	// Source says where the full approved copy lives, as a URL or file name.
	Source string `json:"source,omitempty"`
	// Approved reports that the document has been approved for use. An
	// unapproved document is never shown.
	Approved bool `json:"approved"`
	// ApprovedBy names the person or body that approved the document.
	ApprovedBy string `json:"approvedBy,omitempty"`
	// ApprovedAt is when the document was approved.
	ApprovedAt time.Time `json:"approvedAt,omitzero"`
	// Text is the full document in Markdown. Lines beginning with '#', '##' or
	// '###' are headings that divide it into sections (see [Sections]).
	Text string `json:"text"`
	// CallTypes lists the call types the document applies to, for example
	// "CARD" for a cardiac call. A section may narrow this (see [Sections]).
	CallTypes []string `json:"callTypes,omitempty"`
	// Modifiers lists the incident modifiers the document applies to, for
	// example "weapons".
	Modifiers []string `json:"modifiers,omitempty"`
	// Words lists trigger words or phrases that, found in the incident
	// narrative, make the document apply, for example "not breathing".
	Words []string `json:"words,omitempty"`
	// Always reports that the document applies to every incident, for example
	// a standing caller-safety reminder.
	Always bool `json:"always,omitempty"`
}

// Section is one passage of a document: the text under a single heading,
// together with the mapping that decides when it applies.
type Section struct {
	// Doc is the ID of the document the passage comes from.
	Doc string `json:"doc"`
	// Heading is the section's own heading text.
	Heading string `json:"heading"`
	// Path is the trail of headings down to this section, from the top-level
	// heading to this one.
	Path []string `json:"path"`
	// Anchor is a slug for the section, unique within the document, suitable
	// for a citation link.
	Anchor string `json:"anchor"`
	// Text is the passage verbatim, with any "Applies:" metadata line removed.
	Text string `json:"text"`
	// CallTypes, Modifiers and Words are the section's own mapping, parsed
	// from its "Applies:" line. They are not shown to the operator; they only
	// decide when the section applies. When a section has its own mapping it
	// overrides the document mapping for that section.
	CallTypes []string `json:"-"`
	Modifiers []string `json:"-"`
	Words     []string `json:"-"`
}

// Context is the incident as entered so far, used to select passages.
type Context struct {
	// CallType is the call type, for example "CARD".
	CallType string
	// Modifiers are the incident modifiers that apply, for example "weapons".
	Modifiers []string
	// Text is the narrative, transcript and details. [Match] lower-cases it
	// before searching for trigger words.
	Text string
	// Priority is the incident priority, carried through for the caller's use;
	// it does not affect matching.
	Priority int
	// At is the time to judge the incident at: a document's effective and
	// expiry dates are compared against it. A zero At means "now".
	At time.Time
}

// Passage is one passage selected for an incident, with its citation, the reasons
// it matched, its score, and its status.
type Passage struct {
	// Doc, Title, Kind, Version, Effective and Source cite the source
	// document.
	Doc       string    `json:"doc"`
	Title     string    `json:"title"`
	Kind      string    `json:"kind"`
	Version   string    `json:"version"`
	Effective time.Time `json:"effective"`
	Source    string    `json:"source,omitempty"`
	// Heading, Path, Anchor and Text cite the passage within the document.
	Heading string   `json:"heading"`
	Path    []string `json:"path"`
	Anchor  string   `json:"anchor"`
	Text    string   `json:"text"`
	// Why lists, in plain words, the reasons the passage matched, for example
	// "call type CARD", "modifier weapons" or "words: not breathing".
	Why []string `json:"why"`
	// Score is the match score; higher means a stronger match.
	Score int `json:"score"`
	// Status is one of "current" (approved, in effect, not superseded) or
	// "outdated" (expired at the incident time, or superseded by another
	// approved in-effect document). Unapproved and not-yet-effective passages
	// are never returned, so their statuses "unapproved" and "not-yet" do not
	// appear in a result.
	Status string `json:"status"`
	// Conflict is set when two current policy passages match the same call
	// type from different documents. It names the other document's ID so the
	// operator knows to check which applies.
	Conflict string `json:"conflict,omitempty"`
}

// Status values used in [Passage.Status]. Only "current" and "outdated" are
// ever returned; the others describe passages that are filtered out.
const (
	statusCurrent  = "current"
	statusOutdated = "outdated"
	statusNotYet   = "not-yet"
	statusUnapprvd = "unapproved"
)

// Result is everything selected for an incident.
type Result struct {
	// Matches are the passages that apply, ranked.
	Matches []Passage `json:"matches"`
	// NoMatch reports that no approved, current passage applies.
	NoMatch bool `json:"noMatch"`
	// Note is a plain-words explanation shown when nothing applies.
	Note string `json:"note,omitempty"`
}

// knownKind reports whether k is one of the document kinds.
func knownKind(k string) bool {
	switch k {
	case Policy, Guide, Prompt, Checklist:
		return true
	}
	return false
}

// kindRank orders kinds for the tie-break when scores are equal: policy first,
// then checklist, then guide, then prompt. A lower number sorts earlier.
func kindRank(k string) int {
	switch k {
	case Policy:
		return 0
	case Checklist:
		return 1
	case Guide:
		return 2
	case Prompt:
		return 3
	}
	return 4
}

// Sections splits a document's Markdown text into its passages.
//
// Lines beginning with '#', '##' or '###' are headings, each starting a new
// section; deeper headings nest under shallower ones, and [Section.Path]
// records the trail from the top-level heading down to the section. A document
// with no headings is returned as a single section whose heading is the
// document title. Any introductory text before the first heading is also
// returned as a section under the document title.
//
// Directly under a heading, a line of the form
//
//	Applies: CARD, CHEST; modifiers: weapons; words: not breathing, cpr
//
// is section metadata: it sets the section's own call types, modifiers and
// trigger words, and is removed from the section text so it is never shown.
// The three parts are each optional; bare items before the first labelled part
// are read as call types, so "Applies: CARD, CHEST" sets the call types.
//
// Anchors are slugs of the heading path, made unique within the document by
// appending "-2", "-3" and so on to later duplicates.
func Sections(d Doc) []Section {
	lines := strings.Split(d.Text, "\n")

	type raw struct {
		level int
		head  string
		body  []string
	}
	var raws []raw
	cur := raw{level: 0, head: d.Title} // intro / no-heading section

	flush := func() {
		raws = append(raws, cur)
	}

	started := false
	for _, ln := range lines {
		level, head, ok := heading(ln)
		if ok {
			if started || len(strings.TrimSpace(strings.Join(cur.body, "\n"))) > 0 {
				flush()
			}
			cur = raw{level: level, head: head}
			started = true
			continue
		}
		cur.body = append(cur.body, ln)
	}
	flush()

	// Drop a leading empty intro section (no heading text and no body) that
	// appears only because the document starts with a heading.
	if len(raws) > 1 && raws[0].level == 0 &&
		strings.TrimSpace(strings.Join(raws[0].body, "\n")) == "" {
		raws = raws[1:]
	}

	var secs []Section
	var trail []string // headings by level, index 1.. = level 1..
	seen := map[string]int{}

	for _, r := range raws {
		// Maintain the heading trail. Level 0 (intro / title) sits at the top.
		if r.level == 0 {
			trail = []string{r.head}
		} else {
			if len(trail) < r.level {
				grown := make([]string, r.level)
				copy(grown, trail)
				trail = grown
			} else {
				trail = trail[:r.level]
			}
			trail[r.level-1] = r.head
		}
		path := append([]string(nil), nonEmpty(trail)...)

		body, cts, mods, words := takeApplies(r.body)

		anchor := slug(strings.Join(path, "-"))
		if anchor == "" {
			anchor = "section"
		}
		seen[anchor]++
		if n := seen[anchor]; n > 1 {
			anchor = anchor + "-" + itoa(n)
		}

		secs = append(secs, Section{
			Doc:       d.ID,
			Heading:   r.head,
			Path:      path,
			Anchor:    anchor,
			Text:      strings.Trim(strings.Join(body, "\n"), "\n"),
			CallTypes: cts,
			Modifiers: mods,
			Words:     words,
		})
	}

	return secs
}

// nonEmpty returns the trail with any empty entries removed, preserving order.
func nonEmpty(trail []string) []string {
	out := trail[:0:0]
	for _, s := range trail {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// heading reports whether a line is a Markdown heading of level 1, 2 or 3, and
// returns the level and the trimmed heading text.
func heading(line string) (level int, text string, ok bool) {
	t := strings.TrimRight(line, " \t")
	i := 0
	for i < len(t) && t[i] == '#' {
		i++
	}
	if i < 1 || i > 3 {
		return 0, "", false
	}
	if i < len(t) && t[i] != ' ' {
		return 0, "", false
	}
	return i, strings.TrimSpace(t[i:]), true
}

// takeApplies looks for an "Applies:" line among the leading non-empty lines of
// a section body, parses it, and returns the body with that line removed along
// with the parsed call types, modifiers and words.
func takeApplies(body []string) (rest []string, callTypes, modifiers, words []string) {
	idx := -1
	for i, ln := range body {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		if cts, mods, wds, ok := parseApplies(ln); ok {
			idx = i
			callTypes, modifiers, words = cts, mods, wds
		}
		break // only the first non-empty line can be the Applies line
	}
	if idx < 0 {
		return body, nil, nil, nil
	}
	rest = make([]string, 0, len(body)-1)
	rest = append(rest, body[:idx]...)
	rest = append(rest, body[idx+1:]...)
	return rest, callTypes, modifiers, words
}

// parseApplies parses an "Applies:" metadata line. It understands three
// labelled, semicolon-separated parts in any order: an unlabelled or
// "Applies:" part of call types, a "modifiers:" part, and a "words:" part.
func parseApplies(line string) (callTypes, modifiers, words []string, ok bool) {
	t := strings.TrimSpace(line)
	low := strings.ToLower(t)
	if !strings.HasPrefix(low, "applies:") {
		return nil, nil, nil, false
	}
	t = strings.TrimSpace(t[len("applies:"):])

	for _, part := range strings.Split(t, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lowPart := strings.ToLower(part)
		switch {
		case strings.HasPrefix(lowPart, "modifiers:"):
			modifiers = splitList(part[len("modifiers:"):])
		case strings.HasPrefix(lowPart, "modifier:"):
			modifiers = splitList(part[len("modifier:"):])
		case strings.HasPrefix(lowPart, "words:"):
			words = splitList(part[len("words:"):])
		case strings.HasPrefix(lowPart, "word:"):
			words = splitList(part[len("word:"):])
		default:
			callTypes = append(callTypes, splitList(part)...)
		}
	}
	return callTypes, modifiers, words, true
}

// splitList splits a comma-separated list into trimmed, non-empty items.
func splitList(s string) []string {
	var out []string
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

// slug turns heading text into a lower-case, hyphen-separated anchor slug.
func slug(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	lastHyphen := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen && b.Len() > 0 {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// itoa formats a small non-negative integer without importing strconv for one
// call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// Match ranks the library's passages for an incident and returns the ones that
// apply.
//
// Scoring, for each section, uses the section's own mapping when it has one and
// otherwise the document's mapping:
//
//   - a call-type match scores 50;
//   - each matching modifier scores 20;
//   - each matching trigger word or phrase scores 10, matched whole-word and
//     case-insensitively against the incident narrative;
//   - an "always" document scores 5.
//
// A policy passage gets a +3 tie-break so that, all else equal, mandatory text
// is shown first. Sections with a zero score are dropped.
//
// Unapproved documents and documents whose effective date is after the
// incident time are never returned. A document that has expired by the
// incident time, or that is superseded by another approved, in-effect
// document, is returned but marked outdated and sorted after current passages.
//
// When two current policy passages match the same call type from different
// documents, each is given a [Passage.Conflict] naming the other document, so
// the operator knows to check which applies.
//
// Results are returned in a deterministic order: by score descending, then by
// kind (policy, checklist, guide, prompt), then by document ID, then by anchor.
// At most limit matches are returned; a limit of zero or less means 20. When no
// current passage applies, NoMatch is set and a plain-words note is given.
func Match(docs []Doc, cx Context, limit int) Result {
	if limit <= 0 {
		limit = 20
	}
	at := cx.At
	if at.IsZero() {
		at = time.Now()
	}
	narrative := strings.ToLower(cx.Text)
	wantCall := strings.TrimSpace(cx.CallType)

	// Work out which approved documents are in effect at the incident time, so
	// we can tell what supersedes what.
	inEffect := map[string]bool{} // doc ID -> approved and in effect at `at`
	for _, d := range docs {
		if d.Approved && !effectiveAfter(d, at) && !expired(d, at) {
			inEffect[d.ID] = true
		}
	}
	superseded := map[string]bool{} // doc ID -> replaced by an approved in-effect doc
	for _, d := range docs {
		if d.Approved && d.Supersedes != "" && inEffect[d.ID] {
			superseded[d.Supersedes] = true
		}
	}

	var matches []Passage
	for _, d := range docs {
		if !d.Approved {
			continue // unapproved: never shown
		}
		if effectiveAfter(d, at) {
			continue // not yet effective: never shown
		}
		status := statusCurrent
		if expired(d, at) || superseded[d.ID] {
			status = statusOutdated
		}

		for _, sec := range Sections(d) {
			cts, mods, words := effectiveMapping(d, sec)
			score, why := scoreSection(d, cts, mods, words, wantCall, cx.Modifiers, narrative)
			if score == 0 {
				continue
			}
			matches = append(matches, Passage{
				Doc:       d.ID,
				Title:     d.Title,
				Kind:      d.Kind,
				Version:   d.Version,
				Effective: d.Effective,
				Source:    d.Source,
				Heading:   sec.Heading,
				Path:      sec.Path,
				Anchor:    sec.Anchor,
				Text:      sec.Text,
				Why:       why,
				Score:     score,
				Status:    status,
			})
		}
	}

	markConflicts(matches, wantCall)
	sortMatches(matches)

	if len(matches) > limit {
		matches = matches[:limit]
	}

	// NoMatch when there is no current passage to show.
	hasCurrent := false
	for _, m := range matches {
		if m.Status == statusCurrent {
			hasCurrent = true
			break
		}
	}
	res := Result{Matches: matches}
	if !hasCurrent {
		res.NoMatch = true
		res.Note = "No approved guidance found for this call type."
	}
	return res
}

// effectiveMapping returns the mapping to use for a section: the section's own
// mapping when it has any, otherwise the document's mapping. A section is
// considered to have its own mapping when any of its call types, modifiers or
// words is set.
func effectiveMapping(d Doc, sec Section) (callTypes, modifiers, words []string) {
	if len(sec.CallTypes) > 0 || len(sec.Modifiers) > 0 || len(sec.Words) > 0 {
		return sec.CallTypes, sec.Modifiers, sec.Words
	}
	return d.CallTypes, d.Modifiers, d.Words
}

// scoreSection scores one section's mapping against the incident and returns
// the score and the plain-words reasons.
func scoreSection(d Doc, callTypes, modifiers, words []string, wantCall string, cxMods []string, narrative string) (int, []string) {
	var why []string
	score := 0

	if wantCall != "" && containsFold(callTypes, wantCall) {
		score += 50
		why = append(why, "call type "+wantCall)
	}

	for _, want := range cxMods {
		want = strings.TrimSpace(want)
		if want == "" {
			continue
		}
		if containsFold(modifiers, want) {
			score += 20
			why = append(why, "modifier "+want)
		}
	}

	var hitWords []string
	for _, w := range words {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		if wholeWord(narrative, strings.ToLower(w)) {
			score += 10
			hitWords = append(hitWords, w)
		}
	}
	if len(hitWords) > 0 {
		why = append(why, "words: "+strings.Join(hitWords, ", "))
	}

	if d.Always {
		score += 5
		why = append(why, "always")
	}

	if d.Kind == Policy && score > 0 {
		score += 3 // tie-break: mandatory text shown first
	}

	return score, why
}

// markConflicts sets Conflict on current policy passages that match the same
// call type from different documents.
func markConflicts(matches []Passage, wantCall string) {
	if wantCall == "" {
		return
	}
	// Collect the distinct document IDs of current policy passages that
	// matched on the call type.
	docsHit := map[string]bool{}
	var order []string
	for _, m := range matches {
		if m.Status != statusCurrent || m.Kind != Policy {
			continue
		}
		if !hasReason(m.Why, "call type "+wantCall) {
			continue
		}
		if !docsHit[m.Doc] {
			docsHit[m.Doc] = true
			order = append(order, m.Doc)
		}
	}
	if len(order) < 2 {
		return
	}
	for i := range matches {
		m := &matches[i]
		if m.Status != statusCurrent || m.Kind != Policy {
			continue
		}
		if !hasReason(m.Why, "call type "+wantCall) {
			continue
		}
		// Name the first other document in the matched set.
		for _, id := range order {
			if id != m.Doc {
				m.Conflict = id
				break
			}
		}
	}
}

// hasReason reports whether why contains the given reason.
func hasReason(why []string, reason string) bool {
	for _, w := range why {
		if w == reason {
			return true
		}
	}
	return false
}

// sortMatches orders matches: current before outdated, then score descending,
// then kind (policy, checklist, guide, prompt), then document ID, then anchor.
func sortMatches(matches []Passage) {
	sort.SliceStable(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		if (a.Status == statusOutdated) != (b.Status == statusOutdated) {
			return b.Status == statusOutdated // current first
		}
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if ka, kb := kindRank(a.Kind), kindRank(b.Kind); ka != kb {
			return ka < kb
		}
		if a.Doc != b.Doc {
			return a.Doc < b.Doc
		}
		return a.Anchor < b.Anchor
	})
}

// effectiveAfter reports whether the document's effective date is after the
// given time (so it is not yet in force).
func effectiveAfter(d Doc, at time.Time) bool {
	return d.Effective.After(at)
}

// expired reports whether the document has an expiry date that is at or before
// the given time.
func expired(d Doc, at time.Time) bool {
	return !d.Expires.IsZero() && !at.Before(d.Expires)
}

// containsFold reports whether list contains s, comparing case-insensitively
// and ignoring surrounding spaces.
func containsFold(list []string, s string) bool {
	s = strings.TrimSpace(s)
	for _, v := range list {
		if strings.EqualFold(strings.TrimSpace(v), s) {
			return true
		}
	}
	return false
}

// wholeWord reports whether phrase appears in text as a whole word or phrase:
// its boundaries must not be letters or digits. Both arguments are expected to
// be lower-cased already.
func wholeWord(text, phrase string) bool {
	if phrase == "" {
		return false
	}
	from := 0
	for {
		i := strings.Index(text[from:], phrase)
		if i < 0 {
			return false
		}
		start := from + i
		end := start + len(phrase)
		if !isWordByte(boundaryByte(text, start-1)) && !isWordByte(boundaryByte(text, end)) {
			return true
		}
		from = start + 1
		if from >= len(text) {
			return false
		}
	}
}

// boundaryByte returns the byte at index i, or 0 when i is out of range so that
// the start or end of the text counts as a word boundary.
func boundaryByte(s string, i int) byte {
	if i < 0 || i >= len(s) {
		return 0
	}
	return s[i]
}

// isWordByte reports whether b is a letter or digit, for whole-word matching.
func isWordByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	}
	return false
}

// Validate checks a document before it is saved and returns any problems in
// plain words. An empty result means the document is well formed. It checks
// that the ID, title, kind, version, effective date and text are present, that
// the kind is one of the known kinds, and that the text yields at least one
// section.
func Validate(d Doc) []string {
	var problems []string
	if strings.TrimSpace(d.ID) == "" {
		problems = append(problems, "the document needs an ID")
	}
	if strings.TrimSpace(d.Title) == "" {
		problems = append(problems, "the document needs a title")
	}
	if strings.TrimSpace(d.Kind) == "" {
		problems = append(problems, "the document needs a kind")
	} else if !knownKind(d.Kind) {
		problems = append(problems, "the kind must be one of policy, guide, prompt or checklist")
	}
	if strings.TrimSpace(d.Version) == "" {
		problems = append(problems, "the document needs a version")
	}
	if d.Effective.IsZero() {
		problems = append(problems, "the document needs an effective date")
	}
	if strings.TrimSpace(d.Text) == "" {
		problems = append(problems, "the document needs some text")
	} else if len(Sections(d)) == 0 {
		problems = append(problems, "the text yields no sections")
	}
	return problems
}

// Examples returns two small example documents for the demo. They are clearly
// titled so no one mistakes them for approved text, and they are not approved,
// so [Match] will never show them until an agency replaces them with its own
// approved documents.
func Examples() []Doc {
	return []Doc{
		{
			ID:        "EX-GUIDE-CARD",
			Title:     "EXAMPLE — replace with your agency's approved text",
			Kind:      Guide,
			Version:   "0.0-example",
			Effective: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
			Source:    "example (not for operational use)",
			Approved:  false,
			CallTypes: []string{"CARD", "BRTH"},
			Text: "# Cardiac arrest call-taking guide\n" +
				"Applies: CARD, BRTH; words: not breathing, cpr\n" +
				"\n" +
				"This is EXAMPLE text. Do not use it on a live call.\n" +
				"\n" +
				"## Confirm the problem\n" +
				"Ask: is the patient breathing normally? Are they responsive?\n" +
				"\n" +
				"## Start CPR\n" +
				"Applies: CARD; words: not breathing\n" +
				"If the patient is not breathing, be ready to coach hands-only CPR:\n" +
				"push hard and fast in the centre of the chest.\n",
		},
		{
			ID:        "EX-POL-SAFETY",
			Title:     "EXAMPLE — replace with your agency's approved text",
			Kind:      Policy,
			Version:   "0.0-example",
			Effective: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
			Source:    "example (not for operational use)",
			Approved:  false,
			CallTypes: []string{"SHOOT", "STAB", "ROB", "DOM"},
			Modifiers: []string{"weapons"},
			Text: "# Officer safety on weapons calls\n" +
				"Applies: SHOOT, STAB, ROB, DOM; modifiers: weapons\n" +
				"\n" +
				"This is EXAMPLE text. Do not use it on a live call.\n" +
				"\n" +
				"Confirm the type and location of any weapon. Keep the caller on\n" +
				"the line, away from the threat where safe, and pass weapon\n" +
				"details to responding units before they arrive.\n",
		},
	}
}
