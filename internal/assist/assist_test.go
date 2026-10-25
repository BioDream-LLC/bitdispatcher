package assist

import (
	"math/rand"
	"strings"
	"testing"
)

// hasDetail reports whether the slice holds a detail with the given kind and (lower-cased) text, and
// whether its urgent flag matches want.
func hasDetail(ds []Detail, kind, text string, urgent bool) bool {
	for _, d := range ds {
		if d.Kind == kind && strings.EqualFold(d.Text, text) && d.Urgent == urgent {
			return true
		}
	}
	return false
}

// findKind returns the first detail of a kind, or false.
func findKind(ds []Detail, kind string) (Detail, bool) {
	for _, d := range ds {
		if d.Kind == kind {
			return d, true
		}
	}
	return Detail{}, false
}

// TestExtractFlagship checks the spec's worked example end to end. The count detail is rendered as
// "2 children" (a spelt noun chosen for the display, see countNoun).
func TestExtractFlagship(t *testing.T) {
	in := "My name is Maria Lopez, there's a fire at 210 church avenue southwest apartment 4, two kids are still inside and my husband is not breathing"
	got := Extract(in)

	cases := []struct {
		kind, text string
		urgent     bool
	}{
		{"name", "Maria Lopez", false},
		{"address", "210 Church Ave SW Apt 4", false},
		{"hazard", "fire", true},
		{"count", "2 children", false},
		{"injury", "not breathing", true},
	}
	for _, c := range cases {
		if !hasDetail(got, c.kind, c.text, c.urgent) {
			t.Errorf("missing %s %q (urgent=%v) in %#v", c.kind, c.text, c.urgent, got)
		}
	}
}

// TestExtractVehiclePlateDirectionWeapon checks the second worked example.
func TestExtractVehiclePlateDirectionWeapon(t *testing.T) {
	in := "He has a gun, he left in a silver Honda Civic, Virginia plate ABC 1234, heading north on Williamson Rd"
	got := Extract(in)

	if !hasDetail(got, "weapon", "gun", true) {
		t.Errorf("want urgent weapon gun, got %#v", got)
	}
	if !hasDetail(got, "vehicle", "silver Honda Civic", false) {
		t.Errorf("want vehicle silver Honda Civic, got %#v", got)
	}
	if !hasDetail(got, "plate", "VA ABC-1234", false) {
		t.Errorf("want plate VA ABC-1234, got %#v", got)
	}
	if !hasDetail(got, "direction", "northbound on Williamson Rd", false) {
		t.Errorf("want direction northbound on Williamson Rd, got %#v", got)
	}
}

// TestExtractMileMarker checks route markers and exits.
func TestExtractMileMarker(t *testing.T) {
	got := Extract("single car crash on I-81 mile marker 140 near exit 143")
	if !hasDetail(got, "address", "I-81 Mile marker 140", false) {
		t.Errorf("want I-81 Mile marker 140, got %#v", got)
	}
	if !hasDetail(got, "address", "Exit 143", false) {
		t.Errorf("want Exit 143, got %#v", got)
	}

	got2 := Extract("she's stranded at mile marker 22")
	if !hasDetail(got2, "address", "Mile marker 22", false) {
		t.Errorf("want Mile marker 22, got %#v", got2)
	}
}

// TestExtractCountsAndTable exercises a spread of extraction rules as a table.
func TestExtractCountsAndTable(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		kind   string
		text   string
		urgent bool
	}{
		{"nobody inside", "the house is empty, nobody inside", "count", "0 people", false},
		{"three people", "three people in the car", "count", "3 people", false},
		{"one child", "one child is with her", "count", "1 child", false},
		{"intersection", "crash at the corner of Main St and Elm Ave", "address", "Main St and Elm Ave", false},
		{"unit hash", "come to 44 Maple Dr #B", "address", "44 Maple Dr #B", false},
		{"knife urgent", "he's got a knife", "weapon", "knife", true},
		{"unconscious urgent", "she is unconscious on the floor", "injury", "unconscious", true},
		{"bleeding badly urgent", "he is bleeding badly", "injury", "bleeding badly", true},
		{"chest pain", "my dad has chest pain", "injury", "chest pain", false},
		{"seizure", "my son is having a seizure", "injury", "seizure", false},
		{"gas smell urgent", "I smell gas in the kitchen", "hazard", "gas smell", true},
		{"power lines urgent", "there are downed power lines across the road", "hazard", "downed power lines", true},
		{"trapped urgent", "two workers are trapped", "hazard", "trapped", true},
		{"dog hazard", "watch out, there is a dog", "hazard", "dog", false},
		{"fire no people", "there is a fire in the dumpster out back", "hazard", "fire", false},
		{"vehicle body only", "a white van just pulled up", "vehicle", "white van", false},
		{"plate state", "Texas tag number XYZ 789", "plate", "TX XYZ-789", false},
		{"name this is", "this is John Smith calling", "name", "John Smith", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Extract(tc.in)
			if !hasDetail(got, tc.kind, tc.text, tc.urgent) {
				t.Errorf("Extract(%q): want %s %q urgent=%v, got %#v", tc.in, tc.kind, tc.text, tc.urgent, got)
			}
		})
	}
}

// TestExtractDeDuplicates checks that repeated facts appear once, in first-seen order.
func TestExtractDeDuplicates(t *testing.T) {
	got := Extract("gun! he has a gun, I said a GUN")
	n := 0
	for _, d := range got {
		if d.Kind == "weapon" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("want one weapon detail, got %d: %#v", n, got)
	}
}

// TestExtractQuoteCap checks quotes never exceed the cap.
func TestExtractQuoteCap(t *testing.T) {
	long := "my name is Maria Lopez " + strings.Repeat("and then ", 50)
	for _, d := range Extract(long) {
		if len(d.Quote) > quoteCap {
			t.Errorf("quote over cap (%d): %q", len(d.Quote), d.Quote)
		}
	}
}

// TestDetectLang checks the language hint on clear Spanish and English, and the empty answer.
func TestDetectLang(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Hay un incendio en mi casa y mi esposo no respira", "es"},
		{"¿Cuál es la dirección de la emergencia?", "es"},
		{"There is a fire in my house and he is not breathing", "en"},
		{"Is anyone inside the building right now", "en"},
		{"", ""},
		{"42 42 42", ""},
	}
	for _, tc := range tests {
		if got := DetectLang(tc.in); got != tc.want {
			t.Errorf("DetectLang(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestTranslateSpanishCall checks the spec's Spanish example translates with a gist or better.
func TestTranslateSpanishCall(t *testing.T) {
	tr := Translate("Hay un incendio en mi casa, mi esposo no respira", "es", "en")
	low := strings.ToLower(tr.Text)
	for _, want := range []string{"fire", "my house", "not breathing"} {
		if !strings.Contains(low, want) {
			t.Errorf("translation %q missing %q", tr.Text, want)
		}
	}
	if tr.Coverage < 0.6 {
		t.Errorf("coverage %.2f below 0.6 for %q", tr.Coverage, tr.Text)
	}
}

// TestTranslateKeepsUnknowns checks numbers and proper names survive translation.
func TestTranslateKeepsUnknowns(t *testing.T) {
	tr := Translate("mi casa en 210 Williamson", "es", "en")
	if !strings.Contains(tr.Text, "210") || !strings.Contains(tr.Text, "Williamson") {
		t.Errorf("want 210 and Williamson kept, got %q", tr.Text)
	}
}

// TestPhraseRoundTrip checks every ready phrase translates to its pair, both ways, with coverage 1.
func TestPhraseRoundTrip(t *testing.T) {
	for _, p := range Phrases {
		es := Translate(p.EN, "en", "es")
		if es.Text != p.ES {
			t.Errorf("EN->ES %q = %q, want %q", p.EN, es.Text, p.ES)
		}
		if es.Coverage != 1 {
			t.Errorf("EN->ES %q coverage = %.2f, want 1", p.EN, es.Coverage)
		}
		en := Translate(p.ES, "es", "en")
		if en.Text != p.EN {
			t.Errorf("ES->EN %q = %q, want %q", p.ES, en.Text, p.EN)
		}
		if en.Coverage != 1 {
			t.Errorf("ES->EN %q coverage = %.2f, want 1", p.ES, en.Coverage)
		}
	}
}

// TestPhraseMatchIgnoresCaseAndPunctuation checks a reworded-looking but equivalent phrase still
// matches when only case, accents and surrounding punctuation differ.
func TestPhraseMatchIgnoresCaseAndPunctuation(t *testing.T) {
	tr := Translate("cual es la direccion de la emergencia", "es", "en")
	if tr.Text != "What is the address of the emergency?" || tr.Coverage != 1 {
		t.Errorf("got %q cov %.2f", tr.Text, tr.Coverage)
	}
}

// TestPhrasebookSize checks the phrasebook meets the minimum sizes the spec requires.
func TestPhrasebookSize(t *testing.T) {
	if len(Phrases) < 20 {
		t.Errorf("want at least 20 ready phrases, got %d", len(Phrases))
	}
	if len(esEnWords) < 150 {
		t.Errorf("want at least 150 Spanish->English entries, got %d", len(esEnWords))
	}
}

// TestNoPanicRandom throws random bytes and a very long input at both entry points to prove they are
// panic-free, as the spec requires.
func TestNoPanicRandom(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 2000; i++ {
		b := make([]byte, r.Intn(64))
		for j := range b {
			b[j] = byte(r.Intn(256))
		}
		s := string(b)
		_ = Extract(s)
		_ = Translate(s, "es", "en")
		_ = Translate(s, "en", "es")
		_ = DetectLang(s)
	}

	// A 30000-char input, beyond the processing cap.
	big := strings.Repeat("fuego gun 210 Church Ave SW ñ ¿? ", 1000)
	if len(big) < 30000 {
		big += strings.Repeat("x", 30000-len(big))
	}
	_ = Extract(big)
	_ = Translate(big, "es", "en")
	_ = DetectLang(big)
}

// Sentences run together on a transcript: an address or a street does not swallow the next sentence.
func TestExtractSentenceEnds(t *testing.T) {
	ds := Extract("4333 Old Cave Spring Rd. Someone just robbed the store. He left heading north on Williamson Rd. He's wearing a gray hoodie. One man is bleeding badly. Somebody's parked in front of my driveway and I can't get out")
	got := map[string]bool{}
	for _, d := range ds {
		got[d.Kind+":"+d.Text] = true
	}
	for _, want := range []string{"address:4333 Old Cave Spring Rd", "direction:northbound on Williamson Rd", "count:1 man", "injury:bleeding badly"} {
		if !got[want] {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	for _, not := range []string{"injury:bleeding", "hazard:trapped"} {
		if got[not] {
			t.Errorf("unwanted %s", not)
		}
	}
}

func TestTranslateColourOrder(t *testing.T) {
	if got := Translate("Es un carro blanco y una camioneta roja", "es", "en").Text; !strings.Contains(got, "white car") || !strings.Contains(got, "red truck") {
		t.Fatalf("%q", got)
	}
}

func TestTranslateCallerSentences(t *testing.T) {
	for in, want := range map[string]string{
		"Ayuda por favor, mi esposo se cayó y no respira":             "is not breathing",
		"Estoy en la casa, está en el piso de la cocina, no responde": "kitchen floor",
		"Hubo un choque, hay un hombre herido, está sangrando":        "there was a crash",
	} {
		if got := Translate(in, "es", "en").Text; !strings.Contains(got, want) || strings.Contains(got, " por ") {
			t.Errorf("%q -> %q", in, got)
		}
	}
}

// The twelve more phrasebooks: each language is recognised from a caller's line, the line becomes a
// gist with its emergency words, and every ready phrase is said in each language.
func TestMoreLanguages(t *testing.T) {
	lines := map[string]string{
		"zh": "救命！我的房子着火了",
		"ja": "助けて！家が火事です",
		"ko": "도와주세요! 집에 불이 났어요",
		"vi": "Cứu tôi với! Nhà tôi bị cháy",
		"ar": "ساعدوني! بيتي يحترق",
		"ru": "Помогите! Мой дом горит",
		"hi": "मदद करो! मेरे घर में आग लगी है",
		"fr": "Au secours! Il y a le feu dans ma maison",
		"pt": "Socorro! A minha casa está pegando fogo",
		"pl": "Pomocy! Mój dom się pali",
		"tl": "Tulong! Nasusunog ang bahay ko",
		"ht": "Anmwe! Kay mwen ap boule",
	}
	for code, line := range lines {
		if got := DetectLang(line); got != code {
			t.Errorf("%s: %q detected as %q", code, line, got)
		}
		tr := Translate(line, code, "en")
		if tr.Coverage == 0 || !strings.ContainsAny(strings.ToLower(tr.Text), "abcdefghijklmnopqrstuvwxyz") {
			t.Errorf("%s: %q gave %+v", code, line, tr)
		}
		for _, p := range Phrases {
			if ReadyIn(p.Key, code) == "" {
				t.Errorf("%s has no ready phrase %s", code, p.Key)
			}
			if out := Translate(p.EN, "en", code); out.Coverage != 1 || out.Text != ReadyIn(p.Key, code) {
				t.Errorf("%s: %q said as %+v", code, p.EN, out)
			}
		}
		// Typed English that is not a ready phrase is not made up.
		if out := Translate("the blue truck behind the hardware store", "en", code); out.Coverage != 0 || out.Text != "" {
			t.Errorf("%s: free English was translated as %+v", code, out)
		}
	}
	// English and Spanish still are.
	if DetectLang("my husband is not breathing") != "en" || DetectLang("mi esposo no respira") != "es" {
		t.Error("English or Spanish no longer detected")
	}
	if len(Languages()) != 14 {
		t.Errorf("%d languages", len(Languages()))
	}
}
