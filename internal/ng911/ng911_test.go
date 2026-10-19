package ng911

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"
)

// The testdata files were written by BitVoice's own EIDO and ALI encoders (internal/i3), so these tests pin
// bitdispatcher to what BitVoice really sends.

func TestParseEIDOFromBitVoice(t *testing.T) {
	b, err := os.ReadFile("testdata/eido-bitvoice.json")
	if err != nil {
		t.Fatal(err)
	}
	c, err := ParseEIDO(b)
	if err != nil {
		t.Fatal(err)
	}
	if c.CallID != "abc123" || c.Callback != "+17175550100" || c.Agency != "psap.example.gov" || c.Version != "1.0" {
		t.Fatalf("call %+v", c)
	}
	if c.Agent != "jlee@psap.example.gov" || c.Position != "101" || c.Queue != "911" {
		t.Fatalf("agent %+v", c)
	}
	if !c.Answer.Equal(time.Date(2026, 10, 12, 13, 0, 30, 0, time.UTC)) {
		t.Fatalf("answer %v", c.Answer)
	}
	l := c.Location
	if !l.HasPoint() || *l.Lat != 40.2633 || *l.Lon != -76.8823 || l.Radius != 35 || l.Confidence != 90 || l.Method != "GPS" || l.Source != "eido" {
		t.Fatalf("location %+v", l)
	}
	if l.Address != "123 N Front St, Floor 2, Harrisburg PA 17101" {
		t.Fatalf("address %q", l.Address)
	}
	if c.Additional == nil || c.Additional.Class() != "WRLS" || c.Additional.Subscriber.Name != "Jordan Example" || c.Additional.Providers[0].Name != "Example Wireless" {
		t.Fatalf("additional %+v", c.Additional)
	}
	if len(c.Media) != 2 || c.Media[1].Purpose != "caller-video-live" || c.Media[0].URL != "https://psap.example.gov/m/tok/1" {
		t.Fatalf("media %+v", c.Media)
	}
	if len(c.Notes) != 1 || c.Notes[0] != "caller on 2nd floor <smoke>" {
		t.Fatalf("notes %q", c.Notes)
	}
}

func TestParseEIDORejectsJunk(t *testing.T) {
	for _, s := range []string{`{}`, `not json`, `{"$id":"x"}`} {
		if _, err := ParseEIDO([]byte(s)); err == nil {
			t.Errorf("%q accepted", s)
		}
	}
}

func TestParseSpillFromBitVoice(t *testing.T) {
	b, _ := os.ReadFile("testdata/ali-bitvoice.txt")
	a := ParseSpill(string(b))
	if a.Position != "101" || a.Time != "10/12 09:00:05" || a.Callback != "+17175550100" || a.Class != "WRLS" || a.Name != "JORDAN EXAMPLE" || a.Provider != "EXAMPLE WIRELESS" {
		t.Fatalf("spill %+v", a)
	}
	l := a.Location
	if l == nil || !l.HasPoint() || *l.Lat != 40.2633 || *l.Lon != -76.8823 || l.Radius != 35 || l.Confidence != 90 {
		t.Fatalf("location %+v", l)
	}
	c := l.Civic
	if c.Number != "123" || c.PreDir != "N" || c.Street != "FRONT" || c.StreetType != "ST" || c.Floor != "2" || c.City != "HARRISBURG" || c.State != "PA" || c.PostalCode != "17101" {
		t.Fatalf("civic %+v", c)
	}
	b, _ = os.ReadFile("testdata/ali-bitvoice-nolocation.txt")
	if n := ParseSpill(string(b)); !n.NoLocation || n.Location != nil || n.Class != "VOIP" || n.Callback != "+17175550111" {
		t.Fatalf("no-location spill %+v", n)
	}
}

func TestParsePIDF(t *testing.T) {
	b, _ := os.ReadFile("testdata/pidf-civic-circle.xml")
	l, err := ParsePIDF(b)
	if err != nil || !l.HasPoint() || l.Civic.City != "Harrisburg" || !l.At.Equal(time.Date(2026, 10, 12, 13, 0, 20, 0, time.UTC)) {
		t.Fatalf("%+v %v", l, err)
	}
	poly := `<presence entity="x"><gml:Polygon><gml:exterior><gml:LinearRing><gml:posList>40.0 -76.0 40.0 -75.99 40.01 -75.99 40.01 -76.0 40.0 -76.0</gml:posList></gml:LinearRing></gml:exterior></gml:Polygon></presence>`
	p, err := ParsePIDF([]byte(poly))
	if err != nil || !p.HasPoint() || p.Radius < 600 || p.Radius > 800 {
		t.Fatalf("polygon %+v %v", p, err)
	}
	if _, err := ParsePIDF([]byte(`<other/>`)); err == nil {
		t.Fatal("not-PIDF accepted")
	}
}

func TestALIListener(t *testing.T) {
	var mu sync.Mutex
	var got []ALI
	l, err := ListenALI("127.0.0.1:0", func(a ALI, from string) {
		mu.Lock()
		got = append(got, a)
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	b, _ := os.ReadFile("testdata/ali-bitvoice.txt")
	// BitVoice writes the frame itself (STX ... ETX); SendSpill adds framing, so strip it first.
	text := string(b[1 : len(b)-1])
	if err := SendSpill(context.Background(), l.Addr(), text); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0].Callback != "+17175550100" {
		t.Fatalf("got %+v", got)
	}
}
