package assist

import (
	"regexp"
	"strings"
	"unicode"
)

// DetectLang guesses the language of text: English ("en"), Spanish ("es"), or one of the phrasebook's
// other languages (phrasebooks.go) by its script or marker words; "" when it cannot tell (too short or
// none of them). It scores the text against small sets of marker words and characters, so it is a cheap
// hint, not a certainty.
func DetectLang(text string) string {
	if l := detectScript(text); l != "" {
		return l
	}
	norm := fold(text)
	words := strings.Fields(norm)
	if len(words) == 0 {
		return ""
	}

	if l := detectLatin(text); l != "" {
		return l
	}
	var es, en int
	for _, w := range words {
		if esMarkers[w] {
			es++
		}
		if enMarkers[w] {
			en++
		}
	}
	// Spanish-only letters and the inverted marks are strong signals.
	for _, r := range text {
		switch r {
		case 'ñ', 'Ñ', '¿', '¡':
			es += 2
		case 'á', 'é', 'í', 'ó', 'ú', 'ü', 'Á', 'É', 'Í', 'Ó', 'Ú':
			es++
		}
	}

	if es == 0 && en == 0 {
		return ""
	}
	if es > en {
		return "es"
	}
	if en > es {
		return "en"
	}
	// A tie: prefer Spanish only when an accent or inverted mark was seen.
	if strings.ContainsAny(text, "ñÑ¿¡áéíóúü") {
		return "es"
	}
	return ""
}

// Translation is a phrasebook translation.
type Translation struct {
	Text     string  `json:"text"`     // the translation
	Coverage float64 `json:"coverage"` // 0..1: the share of the words the phrasebook knew; below 0.6 the result is only a gist
}

// Translate translates between "es" and "en" with the phrasebook. It is NOT a general translator: it
// knows a fixed set of emergency phrases and words, matches multi-word phrases longest-first, then
// falls back to a word-by-word dictionary. Unknown words, numbers and capitalised proper names are
// kept as they are, and the Coverage reports how much of the input the phrasebook actually knew.
var reColourAfter = regexp.MustCompile(`(?i)\b(car|truck|pickup|van|minivan|suv|motorcycle|vehicle|house|shirt|hoodie|jacket|coat|hat|pants|dog)\s+(white|black|red|blue|green|gray|grey|silver|yellow|brown|orange|purple|gold)\b`)

func Translate(text, from, to string) Translation {
	if len(text) > maxInput {
		text = text[:maxInput]
	}
	if from != to && (books[from] != nil || books[to] != nil) {
		return translateBook(text, from, to)
	}
	if from == to || (from != "es" && from != "en") || (to != "es" && to != "en") {
		return Translation{Text: text, Coverage: 0}
	}

	// A whole-text match against a ready phrase returns its pair with full coverage.
	if norm := fold(text); norm != "" {
		if pair, ok := phraseExact[dirKey(from, to)][norm]; ok {
			return Translation{Text: pair, Coverage: 1}
		}
	}

	phrases, words := dictFor(from, to)

	// Work over positioned tokens so punctuation and spacing can be rebuilt.
	toks := splitKeep(text)
	known, total := 0, 0
	var b strings.Builder

	i := 0
	for i < len(toks) {
		t := toks[i]
		if !t.word {
			b.WriteString(t.text)
			i++
			continue
		}

		// Try a multi-word phrase starting here, longest-first.
		if adv, words, repl, ok := matchPhrase(toks, i, phrases); ok {
			b.WriteString(repl)
			total += words
			known += words
			i += adv
			continue
		}

		total++
		key := fold(t.text)
		if repl, ok := words[key]; ok {
			b.WriteString(preserveCase(t.text, repl))
			known++
		} else if isKeepAsIs(t.text) {
			b.WriteString(t.text)
			known++ // numbers and proper names are correctly carried across
		} else {
			b.WriteString(t.text)
		}
		i++
	}

	cov := 0.0
	if total > 0 {
		cov = float64(known) / float64(total)
	}
	out := strings.TrimSpace(b.String())
	if from == "es" {
		// Spanish puts the colour after the noun ("carro blanco"); English before it.
		out = reColourAfter.ReplaceAllString(out, "$2 $1")
	}
	return Translation{Text: out, Coverage: cov}
}

// matchPhrase tries to match the longest phrase beginning at token i. On a match it returns the token
// advance (how far to move i), the number of word tokens the phrase covered, the replacement, and
// true.
func matchPhrase(toks []piece, i int, phrases []phraseEntry) (adv, words int, repl string, ok bool) {
	// Build a folded word sequence from i onward, remembering the token index just past each word.
	var ws []string
	var spans []int
	j := i
	for j < len(toks) && len(ws) < maxPhraseWords {
		if toks[j].word {
			ws = append(ws, fold(toks[j].text))
			spans = append(spans, j+1)
		}
		j++
	}
	for n := len(ws); n >= 2; n-- { // phrases are at least two words
		cand := strings.Join(ws[:n], " ")
		for _, p := range phrases {
			if p.from == cand {
				end := spans[n-1]
				return end - i, n, p.to, true
			}
		}
	}
	return 0, 0, "", false
}

// maxPhraseWords bounds how many words a phrase may contain.
const maxPhraseWords = 10

// piece is a token that is either a word or a run of non-word characters (spaces, punctuation).
type piece struct {
	text string
	word bool
}

// splitKeep splits text into word and non-word pieces, preserving everything.
func splitKeep(text string) []piece {
	var out []piece
	var cur strings.Builder
	curWord := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, piece{text: cur.String(), word: curWord})
			cur.Reset()
		}
	}
	for _, r := range text {
		w := isWordRune(r)
		if cur.Len() == 0 {
			curWord = w
		}
		if w != curWord {
			flush()
			curWord = w
		}
		cur.WriteRune(r)
	}
	flush()
	return out
}

// isWordRune reports whether a rune is part of a word (letters, digits, and inner apostrophes).
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '\''
}

// fold lower-cases, strips Spanish accents and inverted marks, and collapses spacing, for matching.
func fold(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case 'á':
			b.WriteRune('a')
		case 'é':
			b.WriteRune('e')
		case 'í':
			b.WriteRune('i')
		case 'ó':
			b.WriteRune('o')
		case 'ú', 'ü':
			b.WriteRune('u')
		case 'ñ':
			b.WriteRune('n')
		case '¿', '¡', '?', '!', '.', ',', ';', ':', '"', '\'', '(', ')':
			// drop punctuation for matching
		default:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// preserveCase copies the leading capitalisation of the source word onto the replacement.
func preserveCase(src, repl string) string {
	if src == "" || repl == "" {
		return repl
	}
	runes := []rune(src)
	if unicode.IsUpper(runes[0]) {
		// Whole word upper?
		allUpper := true
		letters := 0
		for _, r := range runes {
			if unicode.IsLetter(r) {
				letters++
				if !unicode.IsUpper(r) {
					allUpper = false
				}
			}
		}
		rr := []rune(repl)
		if allUpper && letters > 1 {
			return strings.ToUpper(repl)
		}
		rr[0] = unicode.ToUpper(rr[0])
		return string(rr)
	}
	return repl
}

// isKeepAsIs reports whether a token is a number or a capitalised proper name that should be carried
// across untranslated but still count as covered.
func isKeepAsIs(s string) bool {
	if s == "" {
		return false
	}
	hasDigit := false
	for _, r := range s {
		if unicode.IsNumber(r) {
			hasDigit = true
		}
	}
	if hasDigit {
		return true
	}
	r := []rune(s)[0]
	return unicode.IsUpper(r)
}

// dirKey makes a direction key such as "es>en".
func dirKey(from, to string) string { return from + ">" + to }

// Phrase is one ready phrase in both languages, keyed for the reply buttons.
type Phrase struct {
	Key string `json:"key"`
	EN  string `json:"en"`
	ES  string `json:"es"`
}

// Phrases are the call-taker's ready phrases (English and Spanish), for the reply buttons.
var Phrases = []Phrase{
	{"help_on_way", "Help is on the way", "La ayuda va en camino"},
	{"ask_address", "What is the address of the emergency?", "¿Cuál es la dirección de la emergencia?"},
	{"ask_breathing", "Is the person breathing?", "¿La persona está respirando?"},
	{"ask_awake", "Is the person awake?", "¿La persona está despierta?"},
	{"ask_inside", "Is anyone inside?", "¿Hay alguien adentro?"},
	{"get_out", "Get out of the house now", "Salga de la casa ahora"},
	{"ask_weapons", "Are there weapons?", "¿Hay armas?"},
	{"stay_on_line", "Stay on the line", "Quédese en la línea"},
	{"ask_phone", "What is your phone number?", "¿Cuál es su número de teléfono?"},
	{"ask_hurt", "Is anyone hurt?", "¿Hay alguien herido?"},
	{"ask_floor", "Which floor?", "¿En qué piso?"},
	{"describe_vehicle", "Describe the vehicle", "Describa el vehículo"},
	{"ask_direction", "Which direction did they go?", "¿Hacia dónde se fueron?"},
	{"lock_doors", "Lock the doors", "Cierre las puertas con llave"},
	{"unlock_door", "Unlock the door for the crew", "Abra la puerta para los paramédicos"},
	{"put_dog_away", "Put the dog away", "Encierre al perro"},
	{"cpr", "Push hard and fast in the center of the chest", "Empuje fuerte y rápido en el centro del pecho"},
	{"do_not_move", "Do not move them", "No lo mueva"},
	{"apply_pressure", "Apply pressure to the wound", "Presione la herida"},
	{"interpreter", "An interpreter is joining", "Un intérprete se va a unir a la llamada"},
}

// phraseEntry is one directed phrasebook phrase: a folded source and its translation.
type phraseEntry struct {
	from string
	to   string
}

// dictFor returns the phrase list and word map for a translation direction.
func dictFor(from, to string) ([]phraseEntry, map[string]string) {
	if from == "es" {
		return esEnPhrases, esEnWords
	}
	return enEsPhrases, enEsWords
}

// phraseExact maps a direction to folded whole-text phrases and their paired translation.
var phraseExact = map[string]map[string]string{}

// esEnPhrases and enEsPhrases are the phrasebook's multi-word entries per direction, built at init.
var esEnPhrases []phraseEntry
var enEsPhrases []phraseEntry

// esMarkers and enMarkers are small marker-word sets used by DetectLang.
var esMarkers = map[string]bool{}
var enMarkers = map[string]bool{}

// init builds the direction-specific phrase lists and the exact whole-phrase lookup from Phrases and
// the shared phrase table, and fills the language-marker sets.
func init() {
	// Ready phrases become both exact whole-text matches and longest-first phrase entries.
	for _, p := range Phrases {
		registerPhrase(p.ES, p.EN)
	}
	for _, p := range extraPhrases {
		registerPhrase(p[1], p[0]) // table holds [en, es]
	}
	// Build exact whole-text lookups in both directions from every phrase pair.
	ensureExact()

	for w := range esEnWords {
		esMarkers[w] = true
	}
	for _, w := range []string{"hay", "está", "esta", "mi", "casa", "no", "respira", "el", "la", "en", "se", "fue", "con", "por", "favor", "que", "una", "un"} {
		esMarkers[fold(w)] = true
	}
	for _, w := range []string{"the", "is", "a", "an", "my", "house", "there", "not", "breathing", "he", "she", "at", "on", "in", "with", "please", "to", "of", "and"} {
		enMarkers[w] = true
	}
}

// registerPhrase adds a Spanish->English phrase to the phrase lists and the exact lookups.
func registerPhrase(es, en string) {
	fes, fen := fold(es), fold(en)
	esEnPhrases = append(esEnPhrases, phraseEntry{from: fes, to: en})
	enEsPhrases = append(enEsPhrases, phraseEntry{from: fen, to: es})
	if phraseExact[dirKey("es", "en")] == nil {
		phraseExact[dirKey("es", "en")] = map[string]string{}
		phraseExact[dirKey("en", "es")] = map[string]string{}
	}
	phraseExact[dirKey("es", "en")][fes] = en
	phraseExact[dirKey("en", "es")][fen] = es
}

// spanishOnly are Spanish->English phrases whose English reads better than a reversed pair: they take
// precedence over any other entry for the same Spanish words.
var spanishOnly = [][2]string{
	{"por favor", "please"},
	{"ayuda por favor", "help, please"},
	{"por favor rápido", "please hurry"},
	{"no respira", "is not breathing"},
	{"y no respira", "and is not breathing"},
	{"no responde", "is not responding"},
	{"no puede respirar", "cannot breathe"},
	{"se cayó", "fell"},
	{"se desmayó", "fainted"},
	{"está en el piso", "is on the floor"},
	{"en el piso de la cocina", "on the kitchen floor"},
	{"está en el piso de la cocina", "is on the kitchen floor"},
	{"está sangrando", "is bleeding"},
	{"estoy en la casa", "I am at the house"},
	{"tiene sesenta años", "is sixty years old"},
	{"tiene setenta años", "is seventy years old"},
	{"tiene ochenta años", "is eighty years old"},
	{"hubo un choque", "there was a crash"},
	{"hay un hombre herido", "a man is hurt"},
	{"hay una mujer herida", "a woman is hurt"},
}

func init() {
	for _, p := range spanishOnly {
		f := fold(p[0])
		kept := esEnPhrases[:0]
		for _, e := range esEnPhrases {
			if e.from != f {
				kept = append(kept, e)
			}
		}
		esEnPhrases = append(kept, phraseEntry{from: f, to: p[1]})
		if phraseExact[dirKey("es", "en")] == nil {
			phraseExact[dirKey("es", "en")] = map[string]string{}
		}
		phraseExact[dirKey("es", "en")][f] = p[1]
	}
}

// ensureExact is a placeholder kept for clarity; exact lookups are filled in registerPhrase.
func ensureExact() {}

// extraPhrases are further multi-word emergency phrases, as [English, Spanish] pairs. They widen the
// phrasebook's coverage beyond the reply buttons.
var extraPhrases = [][2]string{
	{"my husband is not breathing", "mi esposo no respira"},
	{"my wife is not breathing", "mi esposa no respira"},
	{"there is a fire", "hay un incendio"},
	{"there is a fire in my house", "hay un incendio en mi casa"},
	{"my house is on fire", "mi casa se está quemando"},
	{"car accident", "accidente de carro"},
	{"a car accident", "un choque"},
	{"chest pain", "dolor de pecho"},
	{"he can't breathe", "no puede respirar"},
	{"she can't breathe", "no puede respirar"},
	{"not breathing", "no respira"},
	{"he is not breathing", "no respira"},
	{"please hurry", "por favor apúrense"},
	{"come quickly", "vengan rápido"},
	{"my son", "mi hijo"},
	{"my daughter", "mi hija"},
	{"my mother", "mi madre"},
	{"my father", "mi padre"},
	{"he left", "se fue"},
	{"she left", "se fue"},
	{"he fell", "se cayó"},
	{"she fell", "se cayó"},
	{"near the", "cerca de"},
	{"i am", "estoy"},
	{"there is", "hay"},
	{"there are", "hay"},
	{"help me", "ayúdenme"},
}

// esEnWords is the Spanish->English word and short-phrase dictionary. Keys are folded (lower case, no
// accents, no punctuation). At least 150 entries cover the emergency vocabulary a call-taker needs.
var esEnWords = map[string]string{
	// greetings and core function words
	"ayuda":     "help",
	"ayudenme":  "help me",
	"ayudeme":   "help me",
	"auxilio":   "help",
	"socorro":   "help",
	"por favor": "please",
	"favor":     "please",
	"gracias":   "thank you",
	"si":        "yes",
	"no":        "no",
	"y":         "and",
	"o":         "or",
	"pero":      "but",
	"que":       "that",
	"el":        "the",
	"la":        "the",
	"los":       "the",
	"las":       "the",
	"un":        "a",
	"una":       "a",
	"unos":      "some",
	"unas":      "some",
	"de":        "of",
	"del":       "of the",
	"en":        "at",
	"con":       "with",
	"sin":       "without",
	"a":         "to",
	"para":      "for",
	"muy":       "very",
	"mucho":     "a lot",
	"ahora":     "now",
	"aqui":      "here",
	"alli":      "there",
	"ahi":       "there",

	// people
	"persona":     "person",
	"personas":    "people",
	"gente":       "people",
	"hombre":      "man",
	"hombres":     "men",
	"mujer":       "woman",
	"mujeres":     "women",
	"nino":        "child",
	"nina":        "child",
	"ninos":       "children",
	"ninas":       "children",
	"bebe":        "baby",
	"hijo":        "son",
	"hija":        "daughter",
	"esposo":      "husband",
	"esposa":      "wife",
	"marido":      "husband",
	"madre":       "mother",
	"mama":        "mum",
	"padre":       "father",
	"papa":        "dad",
	"abuelo":      "grandfather",
	"abuela":      "grandmother",
	"hermano":     "brother",
	"hermana":     "sister",
	"amigo":       "friend",
	"amiga":       "friend",
	"vecino":      "neighbour",
	"vecina":      "neighbour",
	"paramedicos": "paramedics",

	// verbs and states
	"es":          "is",
	"esta":        "is",
	"estan":       "are",
	"son":         "are",
	"estoy":       "i am",
	"hay":         "there is",
	"tiene":       "has",
	"tienen":      "have",
	"viene":       "is coming",
	"vienen":      "are coming",
	"necesito":    "i need",
	"necesitamos": "we need",
	"ven":         "come",
	"vengan":      "come",
	"venga":       "come",
	"salga":       "get out",
	"salgan":      "get out",
	"corre":       "run",
	"corran":      "run",

	// medical
	"respira":      "is breathing",
	"respirando":   "breathing",
	"respirar":     "breathe",
	"inconsciente": "unconscious",
	"desmayo":      "fainted",
	"desmayado":    "passed out",
	"responde":     "responds",
	"sangrando":    "bleeding",
	"sangre":       "blood",
	"sangra":       "is bleeding",
	"herido":       "hurt",
	"herida":       "wound",
	"lastimado":    "hurt",
	"lastimada":    "hurt",
	"dolor":        "pain",
	"pecho":        "chest",
	"convulsion":   "seizure",
	"convulsiones": "seizures",
	"embarazada":   "pregnant",
	"parto":        "labour",
	"sobredosis":   "overdose",
	"derrame":      "stroke",
	"ataque":       "attack",
	"cayo":         "fell",
	"caida":        "fall",
	"roto":         "broken",
	"rota":         "broken",
	"quemadura":    "burn",
	"quemaduras":   "burns",
	"medicina":     "medicine",
	"ambulancia":   "ambulance",
	"enfermo":      "sick",
	"enferma":      "sick",

	// weapons and crime
	"pistola":  "gun",
	"arma":     "gun",
	"armas":    "weapons",
	"revolver": "revolver",
	"escopeta": "shotgun",
	"rifle":    "rifle",
	"cuchillo": "knife",
	"navaja":   "knife",
	"machete":  "machete",
	"ladron":   "robber",
	"robo":     "robbery",
	"pelea":    "fight",
	"golpeo":   "hit",
	"disparo":  "shot",
	"disparos": "shots",

	// fire and hazards
	"fuego":      "fire",
	"incendio":   "fire",
	"humo":       "smoke",
	"llamas":     "flames",
	"quemando":   "burning",
	"gas":        "gas",
	"explosion":  "explosion",
	"atrapado":   "trapped",
	"atrapada":   "trapped",
	"atrapados":  "trapped",
	"perro":      "dog",
	"perros":     "dogs",
	"veneno":     "poison",
	"quimico":    "chemical",
	"inundacion": "flood",
	"agua":       "water",

	// vehicles
	"carro":       "car",
	"coche":       "car",
	"auto":        "car",
	"camioneta":   "truck",
	"troca":       "truck",
	"camion":      "lorry",
	"motocicleta": "motorcycle",
	"moto":        "motorcycle",
	"van":         "van",
	"placa":       "plate",
	"placas":      "plates",
	"choque":      "crash",
	"accidente":   "accident",
	"conductor":   "driver",
	"maneja":      "drives",
	"manejando":   "driving",

	// places and location
	"casa":        "house",
	"apartamento": "apartment",
	"edificio":    "building",
	"piso":        "floor",
	"calle":       "street",
	"avenida":     "avenue",
	"numero":      "number",
	"direccion":   "address",
	"adentro":     "inside",
	"dentro":      "inside",
	"afuera":      "outside",
	"fuera":       "outside",
	"arriba":      "upstairs",
	"abajo":       "downstairs",
	"cerca":       "near",
	"lejos":       "far",
	"esquina":     "corner",
	"puerta":      "door",
	"ventana":     "window",
	"cuarto":      "room",
	"bano":        "bathroom",
	"cocina":      "kitchen",
	"ciudad":      "city",
	"pueblo":      "town",

	// movement and directions
	"fue":       "left",
	"fueron":    "left",
	"se fue":    "left",
	"huyo":      "fled",
	"corrio":    "ran",
	"hacia":     "toward",
	"norte":     "north",
	"sur":       "south",
	"este":      "east",
	"oeste":     "west",
	"derecha":   "right",
	"izquierda": "left",
	"rapido":    "quickly",
	"apurense":  "hurry",
	"apurate":   "hurry",
	"despacio":  "slowly",

	// colours
	"rojo":     "red",
	"roja":     "red",
	"azul":     "blue",
	"verde":    "green",
	"amarillo": "yellow",
	"negro":    "black",
	"negra":    "black",
	"blanco":   "white",
	"blanca":   "white",
	"gris":     "grey",
	"plateado": "silver",
	"plata":    "silver",
	"cafe":     "brown",
	"marron":   "brown",
	"naranja":  "orange",

	// numbers
	"cero":   "zero",
	"uno":    "one",
	"dos":    "two",
	"tres":   "three",
	"cuatro": "four",
	"cinco":  "five",
	"seis":   "six",
	"siete":  "seven",
	"ocho":   "eight",
	"nueve":  "nine",
	"diez":   "ten",

	// pronouns and misc
	"yo":         "i",
	"tu":         "you",
	"usted":      "you",
	"ella":       "she",
	"ellos":      "they",
	"nosotros":   "we",
	"mi":         "my",
	"mis":        "my",
	"su":         "your",
	"nombre":     "name",
	"telefono":   "phone",
	"emergencia": "emergency",
	"policia":    "police",
	"bomberos":   "firefighters",
}

// enEsWords is the English->Spanish word dictionary for the reply phrases and common words. Keys are
// folded.
var enEsWords = map[string]string{
	"help":        "ayuda",
	"please":      "por favor",
	"thank":       "gracias",
	"yes":         "sí",
	"no":          "no",
	"and":         "y",
	"or":          "o",
	"the":         "el",
	"a":           "un",
	"an":          "un",
	"of":          "de",
	"at":          "en",
	"in":          "en",
	"on":          "en",
	"with":        "con",
	"to":          "a",
	"for":         "para",
	"now":         "ahora",
	"here":        "aquí",
	"there":       "allí",
	"is":          "está",
	"are":         "están",
	"am":          "estoy",
	"person":      "persona",
	"people":      "personas",
	"man":         "hombre",
	"woman":       "mujer",
	"child":       "niño",
	"children":    "niños",
	"baby":        "bebé",
	"son":         "hijo",
	"daughter":    "hija",
	"husband":     "esposo",
	"wife":        "esposa",
	"mother":      "madre",
	"father":      "padre",
	"breathing":   "respirando",
	"breathe":     "respirar",
	"unconscious": "inconsciente",
	"awake":       "despierto",
	"bleeding":    "sangrando",
	"blood":       "sangre",
	"hurt":        "herido",
	"wound":       "herida",
	"pain":        "dolor",
	"chest":       "pecho",
	"seizure":     "convulsión",
	"pregnant":    "embarazada",
	"overdose":    "sobredosis",
	"gun":         "pistola",
	"weapons":     "armas",
	"knife":       "cuchillo",
	"fire":        "incendio",
	"smoke":       "humo",
	"gas":         "gas",
	"trapped":     "atrapado",
	"dog":         "perro",
	"car":         "carro",
	"truck":       "camioneta",
	"vehicle":     "vehículo",
	"plate":       "placa",
	"accident":    "accidente",
	"house":       "casa",
	"apartment":   "apartamento",
	"building":    "edificio",
	"floor":       "piso",
	"street":      "calle",
	"avenue":      "avenida",
	"number":      "número",
	"address":     "dirección",
	"inside":      "adentro",
	"outside":     "afuera",
	"near":        "cerca",
	"door":        "puerta",
	"north":       "norte",
	"south":       "sur",
	"east":        "este",
	"west":        "oeste",
	"left":        "izquierda",
	"right":       "derecha",
	"red":         "rojo",
	"blue":        "azul",
	"green":       "verde",
	"black":       "negro",
	"white":       "blanco",
	"silver":      "plateado",
	"one":         "uno",
	"two":         "dos",
	"three":       "tres",
	"four":        "cuatro",
	"five":        "cinco",
	"six":         "seis",
	"seven":       "siete",
	"eight":       "ocho",
	"nine":        "nueve",
	"ten":         "diez",
	"name":        "nombre",
	"phone":       "teléfono",
	"emergency":   "emergencia",
	"police":      "policía",
	"ambulance":   "ambulancia",
	"hurry":       "rápido",
	"line":        "línea",
	"stay":        "quédese",
	"anyone":      "alguien",
	"someone":     "alguien",
	"crew":        "paramédicos",
	"he":          "él",
	"she":         "ella",
	"my":          "mi",
	"your":        "su",
	"i":           "yo",
	"you":         "usted",
	"we":          "nosotros",
}
