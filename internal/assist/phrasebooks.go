package assist

// The phrasebooks of more languages: twelve more besides Spanish, each with the call-taker's ready phrases
// and the words of emergencies a caller says, in phrasebooks/<code>.json. They were machine-written and
// have not been checked by native speakers, so what they give is marked: a caller's words become a gist
// (the emergency words found, in order, with "…" for what the phrasebook does not know), and a call-taker's
// words are said in the language only when they are one of the ready phrases. Anything else needs the
// language model or an interpreter. Spanish keeps its fuller phrasebook (translate.go).

import (
	"embed"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"unicode"
)

//go:embed phrasebooks/*.json
var phrasebookFS embed.FS

// Language is a language the phrasebook knows.
type Language struct {
	Code    string `json:"code"`
	Name    string `json:"name"`    // its own name
	English string `json:"english"` // its name in English
	Dir     string `json:"dir"`     // ltr or rtl
	// Full is true for Spanish (a word-by-word phrasebook both ways); the others give a gist one way and
	// ready phrases the other.
	Full bool `json:"full"`
}

type phrasebook struct {
	Lang    string            `json:"lang"`
	Name    string            `json:"name"`
	English string            `json:"english"`
	Dir     string            `json:"dir"`
	Markers []string          `json:"markers"`
	Ready   map[string]string `json:"ready"`
	Caller  [][2]string       `json:"caller"`

	markers map[string]bool
	spot    []spotEntry // longest first
	byEN    map[string]string
	cjk     bool            // written without spaces between words: matched by characters
	vocab   map[string]bool // every word of its phrases and markers
}

type spotEntry struct {
	native []rune
	en     string
}

var books = map[string]*phrasebook{}

func init() {
	ents, err := phrasebookFS.ReadDir("phrasebooks")
	if err != nil {
		panic(err)
	}
	for _, e := range ents {
		raw, err := phrasebookFS.ReadFile("phrasebooks/" + e.Name())
		if err != nil {
			panic(err)
		}
		var b phrasebook
		if err := json.Unmarshal(raw, &b); err != nil {
			panic("assist: phrasebook " + e.Name() + ": " + err.Error())
		}
		b.cjk = b.Lang == "zh" || b.Lang == "ja"
		b.markers = map[string]bool{}
		for _, m := range b.Markers {
			b.markers[normWords(m)] = true
		}
		b.byEN = map[string]string{}
		for _, p := range Phrases {
			if t := strings.TrimSpace(b.Ready[p.Key]); t != "" {
				b.byEN[fold(p.EN)] = t
			}
		}
		for _, c := range b.Caller {
			n := normWords(c[0])
			if n == "" {
				continue
			}
			b.spot = append(b.spot, spotEntry{native: []rune(n), en: strings.TrimSpace(c[1])})
			if _, ok := b.byEN[fold(c[1])]; !ok {
				b.byEN[fold(c[1])] = strings.TrimSpace(c[0])
			}
		}
		sort.SliceStable(b.spot, func(i, j int) bool { return len(b.spot[i].native) > len(b.spot[j].native) })
		books[b.Lang] = &b
	}
}

var markersOnce sync.Once

// extendMarkers makes the words of each Latin-script book's phrases markers too, unless another language
// uses them. It runs on first use, after the Spanish and English markers are built.
func extendMarkers() {
	uses := map[string]int{}
	words := map[string][]string{}
	for code, b := range books {
		if b.cjk {
			continue
		}
		seen := map[string]bool{}
		b.vocab = map[string]bool{}
		for m := range b.markers {
			b.vocab[m] = true
		}
		add := func(text string) {
			for _, w := range strings.Fields(normWords(text)) {
				b.vocab[w] = true
				if !seen[w] {
					seen[w] = true
					uses[w]++
					words[code] = append(words[code], w)
				}
			}
		}
		for _, t := range b.Ready {
			add(t)
		}
		for _, c := range b.Caller {
			add(c[0])
		}
	}
	for code, ws := range words {
		for _, w := range ws {
			if uses[w] == 1 && !esMarkers[fold(w)] && !enMarkers[w] && esEnWords[fold(w)] == "" {
				books[code].markers[w] = true
			}
		}
	}
}

// Languages are the languages the phrasebook knows, English and Spanish first, then by English name.
func Languages() []Language {
	out := []Language{{Code: "en", Name: "English", English: "English", Dir: "ltr", Full: true}, {Code: "es", Name: "Español", English: "Spanish", Dir: "ltr", Full: true}}
	var more []Language
	for _, b := range books {
		more = append(more, Language{Code: b.Lang, Name: b.Name, English: b.English, Dir: b.Dir})
	}
	sort.Slice(more, func(i, j int) bool { return more[i].English < more[j].English })
	return append(out, more...)
}

// Knows reports whether the phrasebook can translate from a language to English.
func Knows(lang string) bool { return lang == "en" || lang == "es" || books[lang] != nil }

// LanguageName is a language's English name, or the code.
func LanguageName(code string) string {
	for _, l := range Languages() {
		if l.Code == code {
			return l.English
		}
	}
	return code
}

// ReadyIn is a ready phrase in a language, by its key; "" when the phrasebook has none.
func ReadyIn(key, lang string) string {
	for _, p := range Phrases {
		if p.Key != key {
			continue
		}
		switch lang {
		case "en":
			return p.EN
		case "es":
			return p.ES
		}
		if b := books[lang]; b != nil {
			return b.Ready[key]
		}
	}
	return ""
}

// normWords is text in lower case with its punctuation made into single spaces. Accents are kept: in
// Vietnamese, Polish or French they tell words apart.
func normWords(s string) string {
	var b strings.Builder
	space := true
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r) || r == '\'' || r == '’' {
			if r == '’' {
				r = '\''
			}
			b.WriteRune(r)
			space = false
		} else if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// translateBook translates with one of the twelve phrasebooks.
func translateBook(text, from, to string) Translation {
	if to == "en" {
		if b := books[from]; b != nil {
			return b.gist(text)
		}
		return Translation{}
	}
	if from == "en" {
		if b := books[to]; b != nil {
			if t, ok := b.byEN[fold(text)]; ok {
				return Translation{Text: t, Coverage: 1}
			}
		}
	}
	return Translation{}
}

// gist finds the emergency words of a caller's line, longest first, and says them in English in the
// order they were said; what it does not know becomes "…".
func (b *phrasebook) gist(text string) Translation {
	n := []rune(normWords(text))
	if len(n) == 0 {
		return Translation{}
	}
	letters := 0
	for _, r := range n {
		if r != ' ' {
			letters++
		}
	}
	used := make([]bool, len(n))
	type hit struct {
		at int
		en string
	}
	var hits []hit
	boundary := func(i int) bool { return b.cjk || i < 0 || i >= len(n) || n[i] == ' ' }
	for _, e := range b.spot {
		l := len(e.native)
		for i := 0; i+l <= len(n); i++ {
			if !boundary(i-1) || !boundary(i+l) {
				continue
			}
			free := true
			for k := i; k < i+l; k++ {
				if used[k] || n[k] != e.native[k-i] {
					free = false
					break
				}
			}
			if !free {
				continue
			}
			for k := i; k < i+l; k++ {
				used[k] = true
			}
			hits = append(hits, hit{i, e.en})
			i += l - 1
		}
	}
	if len(hits) == 0 {
		return Translation{Text: "…", Coverage: 0}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].at < hits[j].at })
	known := 0
	for i, r := range n {
		if used[i] && r != ' ' {
			known++
		}
	}
	var parts []string
	gap := func(from, to int) bool {
		for k := from; k < to; k++ {
			if !used[k] && n[k] != ' ' {
				return true
			}
		}
		return false
	}
	last := 0
	for _, h := range hits {
		if gap(last, h.at) {
			parts = append(parts, "…")
		}
		parts = append(parts, h.en)
		last = h.at
		for last < len(n) && used[last] {
			last++
		}
	}
	if gap(last, len(n)) {
		parts = append(parts, "…")
	}
	return Translation{Text: strings.Join(parts, " · "), Coverage: float64(known) / float64(max(1, letters))}
}

// detectScript names the language of text written in a script only one phrasebook language uses.
func detectScript(text string) string {
	var han, kana, hangul, arabic, deva, cyr, letters int
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Hangul, r):
			hangul++
		case unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r):
			kana++
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.Is(unicode.Arabic, r):
			arabic++
		case unicode.Is(unicode.Devanagari, r):
			deva++
		case unicode.Is(unicode.Cyrillic, r):
			cyr++
		}
		if unicode.IsLetter(r) {
			letters++
		}
	}
	if letters == 0 {
		return ""
	}
	half := func(n int) bool { return n*2 >= letters }
	switch {
	case hangul > 0 && half(hangul):
		return "ko"
	case kana > 0 && half(kana+han):
		return "ja"
	case han > 0 && half(han):
		return "zh"
	case half(arabic):
		return "ar"
	case half(deva):
		return "hi"
	case half(cyr):
		return "ru"
	}
	return ""
}

// detectLatin scores text in the Latin script against the marker words of the phrasebook languages
// written in it (French, Haitian Creole, Polish, Portuguese, Tagalog, Vietnamese); the best wins when it
// has at least two markers and more than the Spanish or English words it does not share.
func detectLatin(text string) string {
	markersOnce.Do(extendMarkers)
	words := strings.Fields(normWords(text))
	best, bestN, second := "", 0, 0
	for code, b := range books {
		if b.cjk || code == "ar" || code == "hi" || code == "ru" || code == "ko" {
			continue
		}
		// Spanish and English words count against it only when this language does not use them too.
		n, es, en := 0, 0, 0
		for _, w := range words {
			if b.markers[w] {
				n++
			}
			if !b.vocab[w] && esMarkers[fold(w)] {
				es++
			}
			if !b.vocab[w] && enMarkers[w] {
				en++
			}
		}
		// Letters only one of them uses count double.
		for _, r := range text {
			switch {
			case code == "vi" && strings.ContainsRune("ăâđêôơưạảấầẩẫậắằẳẵặẹẻẽếềểễệỉịọỏốồổỗộớờởỡợụủứừửữựỳỵỷỹ", unicode.ToLower(r)):
				n += 2
			case code == "pl" && strings.ContainsRune("ąćęłńśźż", unicode.ToLower(r)):
				n += 2
			case code == "pt" && strings.ContainsRune("ãõ", unicode.ToLower(r)):
				n += 2
			case code == "fr" && strings.ContainsRune("èêëîïôûùœç", unicode.ToLower(r)):
				n++
			case code == "ht" && strings.ContainsRune("ò", unicode.ToLower(r)):
				n++
			}
		}
		if n < 2 || n <= es || n <= en {
			continue
		}
		if n > bestN {
			best, second, bestN = code, bestN, n
		} else if n > second {
			second = n
		}
	}
	if bestN > second {
		return best
	}
	return ""
}
