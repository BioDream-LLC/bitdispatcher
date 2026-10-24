// Package ibr writes incident-based reporting submissions: the FBI NIBRS position-based flat file with
// Virginia's additions, as the Virginia State Police take them through the Virginia Crime Repository
// (2025.2 Virginia Incident-Based Reporting System Technical Specification).
//
// A submission is one month's reports for one ORI. Every segment is a fixed-length record, initialised
// to blanks, numeric codes right-justified with zero fill, each on its own line. Group A incidents
// (segments 1 to 6, in level order) come first, then Group B arrests (level 7); a month with neither
// gets a zero report (level 0).
package ibr

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// Submission is what to write.
type Submission struct {
	ORI     string          // the agency's nine-character ORI
	Year    int             // the month the data was collected
	Month   int             //
	Reports []cad.RMSReport // approved reports
	Deletes []Delete        // reports sent before and since changed or withdrawn
	Loc     *time.Location  // the agency's time zone, for incident dates and hours
	// FBI writes the FBI's own layout, without Virginia's additions: for states that take the national
	// flat file, and for checking with the FBI's and SEARCH's validators.
	FBI bool
}

// fbiLength is each level's length in the FBI layout. Virginia's additions all follow the FBI's fields
// (VA 2025.2 Technical Specification), so the FBI segment is the Virginia one cut short; the Group A
// arrestee is longer, with its clearance fields (80 to 110) left blank.
var fbiLength = map[byte]int{'0': 43, '1': 87, '2': 71, '3': 307, '4': 141, '6': 110, '7': 66}

// Delete withdraws an incident sent before (Group A: its number and incident date).
type Delete struct {
	Number   string
	Occurred time.Time
}

// Summary says what a submission holds.
type Summary struct {
	Incidents int      `json:"incidents"` // Group A incident reports
	Arrests   int      `json:"arrests"`   // Group B arrest segments
	Deletes   int      `json:"deletes"`
	Zero      bool     `json:"zero"` // nothing to report: a zero report
	Segments  int      `json:"segments"`
	Skipped   []string `json:"skipped,omitempty"` // reports left out, with why
}

var oriRe = regexp.MustCompile(`^[A-Z]{2}[A-Z0-9]{7}$`)

// Write makes the submission file.
func Write(s Submission) ([]byte, Summary, error) {
	var sum Summary
	if !oriRe.MatchString(s.ORI) {
		return nil, sum, fmt.Errorf("the ORI %q is nine characters: two letters for the state, then seven (Settings)", s.ORI)
	}
	if s.Month < 1 || s.Month > 12 || s.Year < 1991 {
		return nil, sum, fmt.Errorf("no such month %d-%02d", s.Year, s.Month)
	}
	if s.Loc == nil {
		s.Loc = time.Local
	}
	w := &writer{s: s}
	reports := slices.Clone(s.Reports)
	sort.Slice(reports, func(i, j int) bool { return reports[i].Number < reports[j].Number })
	for _, d := range s.Deletes {
		seg := w.seg(87, '1', 'D', d.Number)
		seg.date(38, d.Occurred.In(s.Loc))
		w.add(seg)
		sum.Deletes++
	}
	var groupB []cad.RMSReport
	for _, r := range reports {
		if p := cad.CheckReport(r); len(p) > 0 {
			sum.Skipped = append(sum.Skipped, fmt.Sprintf("%s: %s", r.Number, p[0].Message))
			continue
		}
		if !hasGroupA(r) {
			groupB = append(groupB, r)
			continue
		}
		w.incident(r)
		sum.Incidents++
	}
	for _, r := range groupB {
		sum.Arrests += w.groupB(r)
	}
	if sum.Incidents == 0 && sum.Arrests == 0 && sum.Deletes == 0 {
		seg := w.seg(43, '0', 'A', "000000000000")
		seg.num(38, 2, s.Month)
		seg.num(40, 4, s.Year)
		w.add(seg)
		sum.Zero = true
	}
	sum.Segments = w.n
	return w.buf.Bytes(), sum, nil
}

func hasGroupA(r cad.RMSReport) bool {
	for _, o := range r.Offenses {
		if info, ok := cad.NIBRSOffenseInfo(o.Code); ok && info.Group == "A" {
			return true
		}
	}
	return false
}

type writer struct {
	s   Submission
	buf bytes.Buffer
	n   int
}

// segment is one fixed-length record; positions are the specification's, from 1.
type segment []byte

func (w *writer) seg(length int, level, action byte, number string) segment {
	b := segment(bytes.Repeat([]byte{' '}, length))
	b.num(1, 4, length)
	b[4], b[5] = level, action
	b.num(7, 2, w.s.Month)
	b.num(9, 4, w.s.Year)
	b.text(17, 9, w.s.ORI)
	b.text(26, 12, incidentNumber(number))
	return b
}

func (w *writer) add(b segment) {
	if n, ok := fbiLength[b[4]]; ok && w.s.FBI {
		if b[4] == '6' {
			copy(b[79:85], "      ") // Virginia's drug pairs
		}
		if len(b) > n {
			b = b[:n]
		} else {
			b = append(b, bytes.Repeat([]byte{' '}, n-len(b))...)
		}
		b.num(1, 4, n)
	}
	w.buf.Write(b)
	w.buf.WriteString("\r\n")
	w.n++
}

// text writes left-justified, blank-filled, in capitals.
func (b segment) text(pos, n int, v string) {
	v = strings.ToUpper(v)
	if len(v) > n {
		v = v[:n]
	}
	copy(b[pos-1:pos-1+n], v)
}

// code writes a code right-justified with zero fill ("5" in two places is "05"); blank stays blank.
func (b segment) code(pos, n int, v string) {
	v = strings.TrimSpace(v)
	if v == "" {
		return
	}
	if _, err := strconv.Atoi(v); err == nil && len(v) < n {
		v = strings.Repeat("0", n-len(v)) + v
	}
	b.text(pos, n, v)
}

func (b segment) num(pos, n, v int) {
	b.text(pos, n, fmt.Sprintf("%0*d", n, v))
}

func (b segment) date(pos int, t time.Time) {
	b.text(pos, 8, t.Format("20060102"))
}

// incidentNumber is up to twelve characters of A-Z, 0-9 and hyphen.
func incidentNumber(s string) string {
	s = strings.ToUpper(s)
	s = strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return -1
	}, s)
	if len(s) > 12 {
		s = s[len(s)-12:]
	}
	return s
}

func (w *writer) incident(r cad.RMSReport) {
	at := r.Occurred.In(w.s.Loc)
	var victims, suspects, arrestees []cad.RMSPerson
	for _, p := range r.People {
		switch p.Role {
		case "victim":
			victims = append(victims, p)
		case "suspect":
			suspects = append(suspects, p)
		case "arrestee":
			arrestees = append(arrestees, p)
		}
	}
	var offenses []cad.RMSOffense
	for _, o := range r.Offenses {
		if info, _ := cad.NIBRSOffenseInfo(o.Code); info.Group == "A" {
			offenses = append(offenses, o)
		}
	}

	// Level 1, administrative.
	adm := w.seg(87, '1', 'I', r.Number)
	adm.date(38, at)
	if !r.HourUnknown {
		adm.num(47, 2, at.Hour())
	}
	switch r.Cleared {
	case "A", "B", "C", "D", "E":
		adm.text(49, 1, r.Cleared)
		if d, err := time.Parse("2006-01-02", r.ClearedOn); err == nil {
			adm.date(50, d)
		}
	default:
		adm.text(49, 1, "N")
	}
	w.add(adm)

	// Level 2, one per offence.
	for _, o := range offenses {
		g := w.seg(84, '2', 'I', r.Number)
		g.text(38, 3, o.Code)
		g.text(41, 1, map[bool]string{true: "A", false: "C"}[o.Attempted])
		for i, u := range first(o.Using, 3) {
			g.text(42+i, 1, u)
		}
		g.code(45, 2, r.LocationType)
		if o.Code == "220" {
			if (r.LocationType == "14" || r.LocationType == "19") && o.Premises > 0 {
				g.num(47, 2, o.Premises)
			}
			g.text(49, 1, o.Entry)
		}
		for i, a := range first(o.Activity, 3) {
			g.text(50+i, 1, a)
		}
		for i, wp := range first(o.Weapons, 3) {
			auto := strings.HasSuffix(wp, "A")
			g.code(53+3*i, 2, strings.TrimSuffix(wp, "A"))
			if auto {
				g.text(55+3*i, 1, "A")
			}
		}
		g.code(62, 2, firstNonEmpty(o.Bias, "88"))
		for i, x := range first(o.Security, 2) {
			g.text(72+i, 1, x)
		}
		for i, x := range first(o.EntryPts, 2) {
			g.code(74+2*i, 2, x)
		}
		for i, x := range first(o.ExitPts, 2) {
			g.code(78+2*i, 2, x)
		}
		g.code(82, 1, o.LeftBy)
		g.text(83, 1, map[bool]string{true: "L", false: "N"}[o.LPR])
		w.add(g)
	}

	// Level 3, one per kind of loss, for the offences that have property.
	needProperty := false
	for _, o := range offenses {
		needProperty = needProperty || cad.NIBRSNeedsProperty(o.Code)
	}
	if needProperty {
		w.property(r, offenses, at)
	}

	// Level 4, victims; crimes against society have society as the victim.
	type vic struct {
		p   cad.RMSPerson
		typ string
	}
	var vs []vic
	for _, v := range victims {
		vs = append(vs, vic{v, firstNonEmpty(v.VictimType, "I")})
	}
	society := false
	for _, o := range offenses {
		info, _ := cad.NIBRSOffenseInfo(o.Code)
		society = society || info.Against == "society"
	}
	hasSociety := slices.ContainsFunc(vs, func(v vic) bool { return v.typ == "S" })
	if society && !hasSociety {
		vs = append(vs, vic{cad.RMSPerson{Role: "victim", VictimType: "S"}, "S"})
	}
	for i, v := range vs {
		g := w.seg(172, '4', 'I', r.Number)
		g.num(38, 3, i+1)
		k := 0
		for _, o := range offenses {
			info, _ := cad.NIBRSOffenseInfo(o.Code)
			ok := false
			switch info.Against {
			case "person":
				ok = v.typ == "I" || v.typ == "L"
			case "society":
				ok = v.typ == "S"
			default:
				ok = v.typ != "S"
			}
			if ok && k < 10 {
				g.text(41+3*k, 3, o.Code)
				k++
			}
		}
		g.text(71, 1, v.typ)
		person := v.typ == "I" || v.typ == "L"
		if person {
			g.text(72, 4, age(v.p, r.Occurred, true))
			g.text(76, 1, firstNonEmpty(v.p.Sex, "U"))
			g.text(77, 1, firstNonEmpty(v.p.Race, "U"))
			g.text(78, 1, v.p.Ethnicity)
			g.text(79, 1, v.p.Resident)
			for j, c := range first(v.p.Circumstances, 2) {
				g.code(80+2*j, 2, c)
			}
			inj := v.p.Injuries
			if len(inj) == 0 && len(v.p.Injury) == 1 {
				inj = []string{v.p.Injury}
			}
			if anyPersonCrime(offenses, cad.NIBRSInjuryOffenses) {
				for j, x := range first(inj, 5) {
					g.text(85+j, 1, x)
				}
			}
			if anyPersonCrime(offenses, nil) || slices.ContainsFunc(offenses, func(o cad.RMSOffense) bool { return o.Code == "120" }) {
				for k := range min(len(suspects), 10) {
					rel := cad.RelationTo(v.p, k+1)
					if rel == "" {
						rel = "RU"
					}
					g.num(90+4*k, 2, k+1)
					g.text(92+4*k, 2, rel)
				}
			}
		}
		if v.typ == "L" {
			g.code(130, 2, v.p.OfficerAct)
			g.text(132, 1, v.p.OfficerAssign)
			g.text(133, 9, v.p.OfficerORI)
			g.text(142, 1, v.p.AssaultStatus)
			n := 0
			for k := range min(len(suspects), 10) {
				g.text(143+3*n, 3, fmt.Sprintf("O%02d", k+1))
				n++
			}
		}
		w.add(g)
	}

	// Every arrestee is an offender (NIBRS 656): one arrested and not listed as a suspect is one too.
	for _, p := range arrestees[min(len(suspects), len(arrestees)):] {
		p.Role = "suspect"
		suspects = append(suspects, p)
	}

	// Level 5, offenders: 00 when nothing is known.
	if len(suspects) == 0 {
		g := w.seg(45, '5', 'I', r.Number)
		g.text(38, 2, "00")
		w.add(g)
	}
	for i, p := range suspects {
		n := 45
		if p.Ethnicity != "" {
			n = 46
		}
		g := w.seg(n, '5', 'I', r.Number)
		g.num(38, 2, i+1)
		g.text(40, 4, age(p, r.Occurred, false))
		g.text(44, 1, firstNonEmpty(p.Sex, "U"))
		g.text(45, 1, firstNonEmpty(p.Race, "U"))
		if n == 46 {
			g.text(46, 1, p.Ethnicity)
		}
		w.add(g)
	}

	// Level 6, arrestees.
	for i, p := range arrestees {
		g := w.seg(85, '6', 'I', r.Number)
		g.num(38, 2, i+1)
		w.arrestee(g, p, r, i, 40, offenses)
		w.add(g)
	}
}

// arrestee fills an arrestee's data elements: from 40 in a level 6 segment, from 26 in level 7.
func (w *writer) arrestee(g segment, p cad.RMSPerson, r cad.RMSReport, i, pos int, offenses []cad.RMSOffense) {
	grpB := pos == 26
	no := firstNonEmpty(p.ArrestNo, r.Number)
	if grpB {
		g.text(26, 12, incidentNumber(no))
		g.num(38, 2, i+1)
		pos = 40
	} else {
		g.text(pos, 12, incidentNumber(no))
		pos += 12
	}
	if d, err := time.Parse("2006-01-02", p.ArrestDate); err == nil {
		g.date(pos, d)
	}
	g.text(pos+8, 1, p.ArrestType)
	off := firstNonEmpty(p.ArrestOffense, firstOf(r))
	if !grpB {
		g.text(pos+9, 1, "N")
		pos++
	}
	g.text(pos+9, 3, off)
	for k, a := range first(p.Armed, 2) {
		g.code(pos+12+3*k, 2, strings.TrimSuffix(a, "A"))
		if strings.HasSuffix(a, "A") {
			g.text(pos+14+3*k, 1, "A")
		}
	}
	at := r.Occurred
	if d, err := time.Parse("2006-01-02", p.ArrestDate); err == nil {
		at = d
	}
	ag := age(p, at, false)
	g.text(pos+18, 4, ag)
	g.text(pos+22, 1, p.Sex)
	g.text(pos+23, 1, firstNonEmpty(p.Race, "U"))
	g.text(pos+24, 1, p.Ethnicity)
	g.text(pos+25, 1, p.Resident)
	if n, err := strconv.Atoi(strings.TrimSpace(ag[:2])); err == nil && n > 0 && n < 18 {
		g.text(pos+26, 1, p.Juvenile)
	}
	// VA 62/63: the drug arrest's activity and drug, in pairs.
	if off == "35A" {
		var act []string
		for _, o := range offenses {
			if o.Code == "35A" {
				act = o.Activity
			}
		}
		for k, d := range first(p.ArrestDrugs, 3) {
			a := "P"
			if k < len(act) {
				a = act[k]
			} else if len(act) > 0 {
				a = act[0]
			}
			g.text(pos+27+2*k, 1, a)
			g.text(pos+28+2*k, 1, d)
		}
	}
}

func (w *writer) groupB(r cad.RMSReport) int {
	n := 0
	for _, p := range r.People {
		if p.Role != "arrestee" {
			continue
		}
		g := w.seg(72, '7', 'A', "")
		w.arrestee(g, p, r, n, 26, nil)
		w.add(g)
		n++
	}
	return n
}

func (w *writer) property(r cad.RMSReport, offenses []cad.RMSOffense, at time.Time) {
	type line struct {
		typ       string
		value     float64
		qty       int
		recovered string
	}
	byLoss := map[string][]*line{}
	var drugs []cad.RMSProperty
	for _, p := range r.Property {
		code, ok := cad.Codes.Loss[p.Loss]
		if !ok {
			continue // evidence and the like are not reported
		}
		if code == "6" && p.Type == "10" && p.Drug != "" {
			drugs = append(drugs, p)
		}
		var l *line
		for _, x := range byLoss[code] {
			if x.typ == p.Type {
				l = x
			}
		}
		if l == nil {
			l = &line{typ: p.Type}
			byLoss[code] = append(byLoss[code], l)
		}
		l.value += p.Value
		l.qty += max(p.Quantity, 0)
		if p.RecoveredOn > l.recovered {
			l.recovered = p.RecoveredOn
		}
	}
	if len(byLoss) == 0 {
		// Attempted, or nothing taken: one Property Segment with loss 1 = none.
		byLoss["1"] = nil
	}
	losses := make([]string, 0, len(byLoss))
	for k := range byLoss {
		losses = append(losses, k)
	}
	sort.Strings(losses)
	motor := []string{"03", "05", "24", "28", "37"}
	has240 := slices.ContainsFunc(offenses, func(o cad.RMSOffense) bool { return o.Code == "240" })
	for _, loss := range losses {
		g := w.seg(397, '3', 'I', r.Number)
		g.text(38, 1, loss)
		vehicles := 0
		for i, l := range first(byLoss[loss], 10) {
			if loss == "1" || loss == "8" {
				break
			}
			b := 39 + 19*i
			g.code(b, 2, l.typ)
			if !(loss == "6" && l.typ == "10") || l.value > 0 {
				g.num(b+2, 9, int(l.value+0.5))
			}
			if loss == "5" && l.recovered != "" {
				if d, err := time.Parse("2006-01-02", l.recovered); err == nil {
					g.date(b+11, d)
				}
			}
			if slices.Contains(motor, l.typ) {
				vehicles += max(l.qty, 1)
			}
			// VA 65/60: the offence this property belongs to, and how many.
			v := 308 + 9*i
			g.text(v, 3, propertyOffense(loss, l.typ, offenses))
			if l.qty > 0 {
				g.num(v+3, 6, min(l.qty, 999999))
			}
		}
		if has240 && vehicles > 0 {
			switch loss {
			case "7":
				g.num(229, 2, min(vehicles, 99))
			case "5":
				g.num(231, 2, min(vehicles, 99))
			}
		}
		if loss == "6" {
			if len(drugs) > 3 {
				// More than three drugs: the two most important, then X.
				drugs = append(drugs[:2], cad.RMSProperty{Drug: "X"})
			}
			for i, d := range drugs {
				b := 233 + 15*i
				g.text(b, 1, d.Drug)
				if d.Drug != "X" {
					g.text(b+1, 12, fmt.Sprintf("%09d%03d", int(d.DrugQty), int((d.DrugQty-float64(int(d.DrugQty)))*1000+0.5)))
					g.text(b+13, 2, d.DrugUnit)
				}
			}
		}
		w.add(g)
	}
}

// propertyOffense is the offence a property line belongs to (VA 65): vandalism for damage, the drug
// offence for drugs, else the first property offence.
func propertyOffense(loss, typ string, offenses []cad.RMSOffense) string {
	has := func(c string) bool {
		return slices.ContainsFunc(offenses, func(o cad.RMSOffense) bool { return o.Code == c })
	}
	switch {
	case loss == "4":
		return "290"
	case typ == "10" && has("35A"):
		return "35A"
	case typ == "11" && has("35B"):
		return "35B"
	}
	for _, o := range offenses {
		if cad.NIBRSNeedsProperty(o.Code) && o.Code != "290" && o.Code != "35A" && o.Code != "35B" {
			return o.Code
		}
	}
	for _, o := range offenses {
		if cad.NIBRSNeedsProperty(o.Code) {
			return o.Code
		}
	}
	return ""
}

func anyPersonCrime(offenses []cad.RMSOffense, among []string) bool {
	for _, o := range offenses {
		info, _ := cad.NIBRSOffenseInfo(o.Code)
		if among != nil {
			if slices.Contains(among, o.Code) {
				return true
			}
			continue
		}
		if info.Against == "person" {
			return true
		}
	}
	return false
}

// age is the four-character age: "34  ", a range "3040", "00  " unknown, and for a victim under a
// year NN (under a day), NB (under a week), BB (under a year).
func age(p cad.RMSPerson, at time.Time, victim bool) string {
	if victim {
		if d := cad.PersonAgeDays(p, at); d >= 0 && d < 365 {
			switch {
			case d < 1:
				return "NN  "
			case d < 7:
				return "NB  "
			default:
				return "BB  "
			}
		}
	}
	a := strings.TrimSpace(cad.PersonAge(p, at))
	if lo, hi, ok := strings.Cut(a, "-"); ok {
		x, e1 := strconv.Atoi(strings.TrimSpace(lo))
		y, e2 := strconv.Atoi(strings.TrimSpace(hi))
		if e1 == nil && e2 == nil {
			return fmt.Sprintf("%02d%02d", x, y)
		}
	}
	n, err := strconv.Atoi(a)
	if err != nil {
		return "00  "
	}
	if n > 98 {
		return "99  "
	}
	return fmt.Sprintf("%02d  ", n)
}

func first[T any](xs []T, n int) []T {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}

func firstOf(r cad.RMSReport) string {
	if len(r.Offenses) > 0 {
		return r.Offenses[0].Code
	}
	return ""
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}
