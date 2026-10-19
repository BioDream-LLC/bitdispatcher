package ng911

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ALI is one legacy ALI spill, read.
type ALI struct {
	Position   string    `json:"position,omitempty"`
	Time       string    `json:"time,omitempty"` // as printed (MM/DD HH:MM:SS, no year or zone)
	Callback   string    `json:"callback,omitempty"`
	Class      string    `json:"class,omitempty"` // RESD, BUSN, WRLS, VOIP...
	Name       string    `json:"name,omitempty"`
	Location   *Location `json:"location,omitempty"`
	Provider   string    `json:"provider,omitempty"`
	NoLocation bool      `json:"noLocation,omitempty"`
	Raw        string    `json:"raw"`
}

const stx, etx = 0x02, 0x03

var (
	rePos      = regexp.MustCompile(`^POS\s+(\S+)\s+(\d\d/\d\d\s+\d\d:\d\d:\d\d)`)
	reCallback = regexp.MustCompile(`^\(?(\d{3})\)?[ -]?(\d{3})-?(\d{4})\s+([A-Z]{2,5})?`)
	reCity     = regexp.MustCompile(`^(.*?)\s+([A-Z]{2})\s+(\d{5}(?:-\d{4})?)$`)
	reUnc      = regexp.MustCompile(`^UNC\s+(\d+)M(?:\s+CONF\s+(\d+)%)?`)
)

// ParseSpill reads a spill in the layout CAD vendors already read: the position and time, the callback
// number and class of service, an optional name, the street and its building, unit and floor, the city
// line, the latitude, longitude and uncertainty, and the provider. STX and ETX framing is optional.
func ParseSpill(text string) ALI {
	raw := strings.Trim(text, "\x02\x03")
	a := ALI{Raw: raw}
	var free []string
	var lat, lon *float64
	var radius float64
	var conf int
	for _, line := range strings.FieldsFunc(raw, func(r rune) bool { return r == '\r' || r == '\n' }) {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case a.Position == "" && rePos.MatchString(line):
			m := rePos.FindStringSubmatch(line)
			a.Position, a.Time = m[1], strings.Join(strings.Fields(m[2]), " ")
		case a.Callback == "" && reCallback.MatchString(line):
			m := reCallback.FindStringSubmatch(line)
			a.Callback, a.Class = "+1"+m[1]+m[2]+m[3], m[4]
		case strings.HasPrefix(line, "LAT "):
			if v, err := strconv.ParseFloat(strings.TrimSpace(line[4:]), 64); err == nil {
				lat = &v
			}
		case strings.HasPrefix(line, "LON "):
			if v, err := strconv.ParseFloat(strings.TrimSpace(line[4:]), 64); err == nil {
				lon = &v
			}
		case reUnc.MatchString(line):
			m := reUnc.FindStringSubmatch(line)
			radius, _ = strconv.ParseFloat(m[1], 64)
			conf, _ = strconv.Atoi(m[2])
		case strings.HasPrefix(line, "CO "):
			a.Provider = strings.TrimSpace(line[3:])
		case line == "NO LOCATION RECEIVED":
			a.NoLocation = true
		default:
			free = append(free, line)
		}
	}
	// The free lines are, in order: an optional name, the street, an optional building/unit/floor line,
	// and the city line. The street is the first line starting with a house number.
	var civic Civic
	haveCivic := false
	street := -1
	for i, l := range free {
		if l != "" && l[0] >= '0' && l[0] <= '9' {
			street = i
			break
		}
	}
	for i, l := range free {
		switch {
		case street >= 0 && i < street:
			if a.Name == "" {
				a.Name = l
			}
		case i == street:
			parseStreet(&civic, l)
			haveCivic = true
		case reCity.MatchString(l):
			m := reCity.FindStringSubmatch(l)
			civic.City, civic.State, civic.PostalCode = m[1], m[2], m[3]
			haveCivic = true
		case street >= 0 && i > street:
			parseExtra(&civic, l)
		case street < 0 && a.Name == "":
			a.Name = l
		}
	}
	if haveCivic || lat != nil {
		loc := &Location{Lat: lat, Lon: lon, Radius: radius, Confidence: conf, Source: "ali"}
		if lat == nil || lon == nil {
			loc.Lat, loc.Lon = nil, nil
		}
		if haveCivic {
			loc.Civic = &civic
			loc.Address = civic.String()
		}
		a.Location = loc
	}
	return a
}

var directions = map[string]bool{"N": true, "S": true, "E": true, "W": true, "NE": true, "NW": true, "SE": true, "SW": true}

func parseStreet(c *Civic, l string) {
	f := strings.Fields(l)
	if len(f) == 0 {
		return
	}
	c.Number, f = f[0], f[1:]
	if len(f) > 1 && directions[f[0]] {
		c.PreDir, f = f[0], f[1:]
	}
	if len(f) > 1 && directions[f[len(f)-1]] {
		c.PostDir, f = f[len(f)-1], f[:len(f)-1]
	}
	if len(f) > 1 {
		c.StreetType, f = f[len(f)-1], f[:len(f)-1]
	}
	c.Street = strings.Join(f, " ")
}

func parseExtra(c *Civic, l string) {
	f := strings.Fields(l)
	var loc []string
	for i := 0; i < len(f); i++ {
		if i+1 < len(f) {
			switch f[i] {
			case "BLDG":
				c.Building = f[i+1]
				i++
				continue
			case "UNIT", "APT", "STE":
				c.Unit = f[i+1]
				i++
				continue
			case "FLR":
				c.Floor = f[i+1]
				i++
				continue
			case "RM":
				c.Room = f[i+1]
				i++
				continue
			}
		}
		loc = append(loc, f[i])
	}
	if len(loc) > 0 {
		c.Location = strings.Join(loc, " ")
	}
}

// ALIListener accepts ALI spill connections (call handling dials the CAD) and hands each spill to a
// function. Each connection may carry many spills, each framed by STX and ETX.
type ALIListener struct {
	ln   net.Listener
	wg   sync.WaitGroup
	done chan struct{}
}

// ListenALI starts listening on addr (host:port).
func ListenALI(addr string, handle func(ALI, string)) (*ALIListener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	l := &ALIListener{ln: ln, done: make(chan struct{})}
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				time.Sleep(100 * time.Millisecond)
				continue
			}
			l.wg.Add(1)
			go func() {
				defer l.wg.Done()
				defer c.Close()
				readSpills(c, l.done, func(s string) { handle(ParseSpill(s), c.RemoteAddr().String()) })
			}()
		}
	}()
	return l, nil
}

// Addr is the address being listened on.
func (l *ALIListener) Addr() string { return l.ln.Addr().String() }

// Close stops listening and waits for open connections to finish.
func (l *ALIListener) Close() error {
	close(l.done)
	err := l.ln.Close()
	l.wg.Wait()
	return err
}

func readSpills(c net.Conn, done chan struct{}, emit func(string)) {
	r := bufio.NewReader(c)
	var buf []byte
	in := false
	for {
		select {
		case <-done:
			return
		default:
		}
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		b, err := r.ReadByte()
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			if err != io.EOF || len(buf) == 0 {
				return
			}
			emit(string(buf)) // a sender that closes without ETX still delivered its spill
			return
		}
		switch {
		case b == stx:
			in, buf = true, buf[:0]
		case b == etx:
			if in || len(buf) > 0 {
				emit(string(buf))
			}
			in, buf = false, buf[:0]
		case len(buf) < 4096:
			buf = append(buf, b)
		}
	}
}

// SendSpill writes one framed spill to addr, for tests and the simulator.
func SendSpill(ctx context.Context, addr, text string) error {
	var d net.Dialer
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer c.Close()
	c.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, err = c.Write([]byte("\x02" + text + "\x03"))
	return err
}
