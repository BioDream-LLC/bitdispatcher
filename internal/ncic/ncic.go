// Package ncic formats NCIC 2000 inquiries and sends them to the state's message switch. bitdispatcher is
// not a CJIS interface on its own: a state's switch (VCIN in Virginia) carries the messages to NCIC and
// the state files, under that state's agreement, and every inquiry is logged with its purpose. The
// built-in simulator answers test inquiries for training and is marked as such in every answer.
package ncic

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Kind is an inquiry type, by its NCIC message key.
type Kind struct {
	Key    string     `json:"key"`
	Name   string     `json:"name"`
	Fields []string   `json:"fields"` // message field codes, in order
	Need   [][]string `json:"need"`   // at least one of these sets must be complete
}

// Kinds are the inquiries a dispatcher makes most.
var Kinds = []Kind{
	{Key: "QW", Name: "Wanted person", Fields: []string{"NAM", "SEX", "RAC", "DOB", "OLN", "OLS"}, Need: [][]string{{"NAM", "DOB"}, {"NAM", "SEX", "RAC"}, {"OLN", "OLS"}}},
	{Key: "QV", Name: "Vehicle", Fields: []string{"LIC", "LIS", "LIY", "LIT", "VIN"}, Need: [][]string{{"LIC", "LIS"}, {"VIN"}}},
	{Key: "QG", Name: "Gun", Fields: []string{"SER", "MAK", "CAL"}, Need: [][]string{{"SER"}}},
	{Key: "QA", Name: "Article", Fields: []string{"SER", "TYP"}, Need: [][]string{{"SER"}}},
}

// FieldNames say what each field code is.
var FieldNames = map[string]string{
	"NAM": "Name (LAST, FIRST MIDDLE)", "SEX": "Sex", "RAC": "Race", "DOB": "Date of birth (YYYYMMDD)", "OLN": "Licence number", "OLS": "Licence state",
	"LIC": "Plate", "LIS": "Plate state", "LIY": "Plate year", "LIT": "Plate type", "VIN": "VIN", "SER": "Serial number", "MAK": "Make", "CAL": "Calibre", "TYP": "Article type",
}

// Purposes are the purpose codes an inquiry is made for.
var Purposes = map[string]string{"C": "Criminal justice", "J": "Criminal justice employment", "F": "Firearms-related", "D": "Domestic violence and stalking", "H": "Housing"}

// Query is one inquiry.
type Query struct {
	Key     string            `json:"key"`
	Fields  map[string]string `json:"fields"`
	Purpose string            `json:"purpose"`
	Reason  string            `json:"reason"` // the incident or why: CJIS asks for it
}

var reField = regexp.MustCompile(`^[A-Z0-9 ,.\-'/]*$`)

func kind(key string) (Kind, bool) {
	for _, k := range Kinds {
		if k.Key == key {
			return k, true
		}
	}
	return Kind{}, false
}

// Check validates a query.
func (q *Query) Check() error {
	k, ok := kind(q.Key)
	if !ok {
		return fmt.Errorf("unknown inquiry %q", q.Key)
	}
	if _, ok := Purposes[q.Purpose]; !ok {
		return errors.New("choose the purpose of the inquiry")
	}
	if strings.TrimSpace(q.Reason) == "" {
		return errors.New("give the reason (the incident number or what it is for)")
	}
	clean := map[string]string{}
	for f, v := range q.Fields {
		v = strings.ToUpper(strings.TrimSpace(v))
		if v == "" {
			continue
		}
		if !strings.Contains(strings.Join(k.Fields, " "), f) {
			return fmt.Errorf("%s has no field %s", q.Key, f)
		}
		if !reField.MatchString(v) || len(v) > 40 {
			return fmt.Errorf("%s: letters, digits and , . - ' / only", f)
		}
		clean[f] = v
	}
	q.Fields = clean
	for _, set := range k.Need {
		all := true
		for _, f := range set {
			all = all && clean[f] != ""
		}
		if all {
			return nil
		}
	}
	var opts []string
	for _, set := range k.Need {
		opts = append(opts, strings.Join(set, "+"))
	}
	return fmt.Errorf("%s needs %s", k.Name, strings.Join(opts, " or "))
}

// Format writes the inquiry in NCIC 2000 form: KEY.ORI.FLD/value.FLD/value.
func Format(q Query, ori string) string {
	k, _ := kind(q.Key)
	parts := []string{q.Key, strings.ToUpper(ori)}
	for _, f := range k.Fields {
		if v := q.Fields[f]; v != "" {
			parts = append(parts, f+"/"+v)
		}
	}
	return strings.Join(parts, ".")
}

// Hash is what the ledger keeps of the terms: enough to show two inquiries were the same, nothing more.
func Hash(q Query) string {
	keys := make([]string, 0, len(q.Fields))
	for k := range q.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(q.Key)
	for _, k := range keys {
		b.WriteString("|" + k + "=" + q.Fields[k])
	}
	h := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(h[:12])
}

// Answer is what the switch said.
type Answer struct {
	Text      string    `json:"text"`
	Hit       bool      `json:"hit"`       // a record was returned: confirm it before acting
	Simulated bool      `json:"simulated"` // from the training simulator, not NCIC
	At        time.Time `json:"at"`
	Message   string    `json:"message"` // what was sent
}

// Switch sends formatted inquiries.
type Switch interface {
	Send(ctx context.Context, msg string) (string, error)
}

// Sim is the training simulator. Plates, names and serials starting with TEST return a record; everything
// else returns no record. Every answer says it is simulated.
type Sim struct{}

// Send answers like a switch would, from fixed test data.
func (Sim) Send(ctx context.Context, msg string) (string, error) {
	f := strings.Split(msg, ".")
	key := f[0]
	val := map[string]string{}
	for _, x := range f[2:] {
		if k, v, ok := strings.Cut(x, "/"); ok {
			val[k] = v
		}
	}
	hit := false
	for _, v := range val {
		hit = hit || strings.HasPrefix(v, "TEST")
	}
	head := "*** SIMULATED: TRAINING ANSWER, NOT FROM NCIC ***\n"
	if !hit {
		switch key {
		case "QV":
			return head + fmt.Sprintf("NO NCIC WANT LIC/%s LIS/%s VIN/%s", val["LIC"], val["LIS"], val["VIN"]), nil
		case "QW":
			return head + fmt.Sprintf("NO NCIC WANT NAM/%s DOB/%s", val["NAM"], val["DOB"]), nil
		default:
			return head + fmt.Sprintf("NO NCIC RECORD SER/%s", val["SER"]), nil
		}
	}
	switch key {
	case "QV":
		return head + "MKE/STOLEN VEHICLE\nORI/VA0000000 TEST PD\nLIC/" + val["LIC"] + " LIS/" + val["LIS"] + " LIY/2026 LIT/PC\nVYR/2019 VMA/TOYT VMO/CAM VST/4D VCO/GRY\nDOT/20261001 OCA/TEST-0001\nIMMEDIATE CONFIRMATION REQUIRED WITH ENTERING AGENCY BEFORE ANY ACTION", nil
	case "QW":
		return head + "MKE/WANTED PERSON\nORI/VA0000000 TEST SO\nNAM/" + val["NAM"] + " SEX/M RAC/W DOB/19800101\nOFF/PROBATION VIOLATION DOW/20260901 OCA/TEST-0002\nWARNING: CONFIRM WITH ENTERING AGENCY BEFORE ANY ACTION", nil
	default:
		return head + "MKE/STOLEN ARTICLE\nORI/VA0000000 TEST PD\nSER/" + val["SER"] + "\nCONFIRM WITH ENTERING AGENCY", nil
	}
}

// Line sends to a switch adapter over TCP: the message and a newline, the answer until a blank line.
// A state's own switch protocol is spoken by the adapter on the other end.
type Line struct {
	Addr string
}

// Send sends one message.
func (l Line) Send(ctx context.Context, msg string) (string, error) {
	d := net.Dialer{Timeout: 5 * time.Second}
	c, err := d.DialContext(ctx, "tcp", l.Addr)
	if err != nil {
		return "", fmt.Errorf("the message switch at %s: %w", l.Addr, err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(30 * time.Second))
	if _, err := fmt.Fprintf(c, "%s\n", msg); err != nil {
		return "", err
	}
	var out []string
	sc := bufio.NewScanner(c)
	for sc.Scan() {
		if sc.Text() == "" {
			break
		}
		out = append(out, sc.Text())
	}
	return strings.Join(out, "\n"), sc.Err()
}

// IsHit decides whether an answer returned a record.
func IsHit(text string) bool {
	t := strings.ToUpper(text)
	return strings.Contains(t, "MKE/") && !strings.Contains(t, "NO NCIC")
}
