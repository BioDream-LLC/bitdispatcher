package ibr

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

var est = time.FixedZone("EST", -5*3600)

func at(s string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02 15:04", s, est)
	return t
}

// field reads positions from..to (1-based, inclusive), as the specification numbers them.
func field(line string, from, to int) string { return line[from-1 : to] }

func lines(t *testing.T, b []byte) []string {
	t.Helper()
	ls := strings.Split(strings.TrimSuffix(string(b), "\r\n"), "\r\n")
	for _, l := range ls {
		if n := len(l); field(l, 1, 4) != fmt.Sprintf("%04d", n) {
			t.Fatalf("segment length %q is not its length %d: %q", field(l, 1, 4), n, l)
		}
		if field(l, 7, 12) != "102026" || field(l, 17, 25) != "VA0770000" {
			t.Fatalf("header %q", l[:37])
		}
	}
	return ls
}

func TestBurglaryAssaultAndArrest(t *testing.T) {
	burglary := cad.RMSReport{
		Number: "R26-000007", Occurred: at("2026-10-04 02:30"), LocationType: "20", Narrative: "Rear door forced.",
		Offenses: []cad.RMSOffense{{Code: "220", Using: []string{"N"}, Bias: "88", Entry: "F", EntryPts: []string{"2", "07"}, Security: []string{"G"}, LeftBy: "6"}},
		People:   []cad.RMSPerson{{Role: "victim", Name: "DOE, JANE", DOB: "1980-05-01", Sex: "F", Race: "W", Ethnicity: "N", Resident: "R"}},
		Property: []cad.RMSProperty{
			{Loss: "stolen", Type: "26", Value: 400.4, Quantity: 1},
			{Loss: "stolen", Type: "26", Value: 200, Quantity: 1}, // the same kind adds up
			{Loss: "stolen", Type: "20", Value: 85, Quantity: 1},
			{Loss: "recovered", Type: "26", Value: 400, Quantity: 1, RecoveredOn: "2026-10-06"},
			{Loss: "evidence", Type: "77", Description: "pry bar"},
		},
	}
	assault := cad.RMSReport{
		Number: "R26-000009", Occurred: at("2026-10-11 23:10"), LocationType: "13", Narrative: "Fight outside the bar.", Cleared: "arrest",
		Offenses: []cad.RMSOffense{{Code: "13A", Using: []string{"A"}, Bias: "88", Weapons: []string{"20"}, Activity: []string{"N"}}},
		People: []cad.RMSPerson{
			{Role: "victim", Name: "ROE, RICHARD", Age: "30-40", Sex: "M", Race: "B", Injuries: []string{"L", "M"}, Circumstances: []string{"01"}, Relations: []cad.RMSRelation{{Suspect: 1, Code: "AQ"}}},
			{Role: "suspect", Name: "POE, PAT", Age: "25", Sex: "M", Race: "W", Ethnicity: "N"},
			{Role: "arrestee", Name: "POE, PAT", Age: "25", Sex: "M", Race: "W", ArrestDate: "2026-10-12", ArrestType: "O", Armed: []string{"16"}},
		},
	}
	dui := cad.RMSReport{
		Number: "R26-000010", Occurred: at("2026-10-15 01:00"), Narrative: "Traffic stop.", Cleared: "arrest",
		Offenses: []cad.RMSOffense{{Code: "90D"}},
		People:   []cad.RMSPerson{{Role: "arrestee", Name: "LOE, LEE", DOB: "2010-01-01", Sex: "F", Race: "A", ArrestDate: "2026-10-15", ArrestType: "S", Armed: []string{"01"}, Juvenile: "R"}},
	}
	b, sum, err := Write(Submission{ORI: "VA0770000", Year: 2026, Month: 10, Reports: []cad.RMSReport{dui, assault, burglary}, Loc: est})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Incidents != 2 || sum.Arrests != 1 || sum.Zero || len(sum.Skipped) != 0 {
		t.Fatalf("summary %+v", sum)
	}
	ls := lines(t, b)
	var levels string
	for _, l := range ls {
		levels += l[4:5]
	}
	// Burglary: 1 2 3 (recovered) 3 (stolen) 4 5; assault: 1 2 4 5 6; then the Group B arrest.
	if levels != "123345124567" {
		t.Fatalf("levels %s\n%s", levels, b)
	}
	adm, off, rec, stolen, vic, unk := ls[0], ls[1], ls[2], ls[3], ls[4], ls[5]
	if field(adm, 26, 37) != "R26-000007  " || field(adm, 38, 45) != "20261004" || field(adm, 47, 48) != "02" || field(adm, 49, 49) != "N" {
		t.Fatalf("administrative %q", adm)
	}
	if field(off, 38, 40) != "220" || field(off, 41, 41) != "C" || field(off, 42, 42) != "N" || field(off, 45, 46) != "20" || field(off, 49, 49) != "F" ||
		field(off, 62, 63) != "88" || field(off, 72, 72) != "G" || field(off, 74, 77) != "0207" || field(off, 82, 82) != "6" || field(off, 83, 84) != "N " {
		t.Fatalf("offence %q", off)
	}
	if field(rec, 38, 38) != "5" || field(rec, 39, 40) != "26" || field(rec, 41, 49) != "000000400" || field(rec, 50, 57) != "20261006" ||
		field(rec, 308, 310) != "220" || field(rec, 311, 316) != "000001" {
		t.Fatalf("recovered %q", rec)
	}
	if field(stolen, 38, 38) != "7" || field(stolen, 39, 49) != "26000000600" || field(stolen, 58, 68) != "20000000085" || field(stolen, 320, 325) != "000001" {
		t.Fatalf("stolen %q", stolen)
	}
	if field(vic, 38, 40) != "001" || field(vic, 41, 43) != "220" || field(vic, 71, 75) != "I46  " || field(vic, 76, 79) != "FWNR" || field(vic, 90, 93) != "    " {
		t.Fatalf("victim %q", vic)
	}
	if field(unk, 38, 39) != "00" || len(unk) != 45 {
		t.Fatalf("unknown offender %q", unk)
	}
	av, ao, aa := ls[8], ls[9], ls[10]
	if field(av, 41, 43) != "13A" || field(av, 72, 75) != "3040" || field(av, 80, 81) != "01" || field(av, 85, 86) != "LM" || field(av, 90, 93) != "01AQ" {
		t.Fatalf("assault victim %q", av)
	}
	if len(ao) != 46 || field(ao, 38, 46) != "0125  MWN" {
		t.Fatalf("offender %q", ao)
	}
	if field(aa, 38, 39) != "01" || field(aa, 40, 51) != "R26-000009  " || field(aa, 52, 59) != "20261012" || field(aa, 60, 64) != "ON13A" ||
		field(aa, 65, 67) != "16 " || field(aa, 71, 76) != "25  MW" {
		t.Fatalf("arrestee %q", aa)
	}
	gb := ls[11]
	if field(gb, 26, 37) != "R26-000010  " || field(gb, 38, 47) != "0120261015" || field(gb, 48, 51) != "S90D" || field(gb, 52, 53) != "01" ||
		field(gb, 58, 63) != "16  FA" || field(gb, 66, 66) != "R" {
		t.Fatalf("group B %q", gb)
	}
}

func TestDrugsZeroAndDelete(t *testing.T) {
	drugs := cad.RMSReport{
		Number: "R26-000011", Occurred: at("2026-10-20 14:00"), LocationType: "18", Narrative: "Seized on a stop.", Cleared: "arrest",
		Offenses: []cad.RMSOffense{{Code: "35A", Using: []string{"D"}, Bias: "88", Activity: []string{"P"}}},
		Property: []cad.RMSProperty{{Loss: "seized", Type: "10", Drug: "S", DrugQty: 2.5, DrugUnit: "GM"}},
		People:   []cad.RMSPerson{{Role: "arrestee", Age: "33", Sex: "M", Race: "W", ArrestDate: "2026-10-20", ArrestType: "O", Armed: []string{"01"}, ArrestDrugs: []string{"S"}}},
	}
	b, sum, err := Write(Submission{ORI: "VA0770000", Year: 2026, Month: 10, Reports: []cad.RMSReport{drugs}, Loc: est})
	if err != nil || sum.Incidents != 1 {
		t.Fatal(err, sum)
	}
	ls := lines(t, b)
	var prop, vic, arr string
	for _, l := range ls {
		switch l[4] {
		case '3':
			prop = l
		case '4':
			vic = l
		case '6':
			arr = l
		}
	}
	if field(prop, 38, 40) != "610" || field(prop, 233, 247) != "S000000002500GM" || field(prop, 308, 310) != "35A" {
		t.Fatalf("seized drugs %q", prop)
	}
	if field(vic, 41, 43) != "35A" || field(vic, 71, 71) != "S" {
		t.Fatalf("society victim %q", vic)
	}
	if field(arr, 80, 81) != "PS" {
		t.Fatalf("VA arrest drug %q", arr)
	}

	// An incomplete report is left out, and said so; nothing left is a zero report.
	bad := drugs
	bad.Narrative = ""
	b, sum, err = Write(Submission{ORI: "VA0770000", Year: 2026, Month: 10, Reports: []cad.RMSReport{bad}, Loc: est})
	if err != nil || !sum.Zero || len(sum.Skipped) != 1 {
		t.Fatal(err, sum)
	}
	ls = lines(t, b)
	if len(ls) != 1 || field(ls[0], 5, 6) != "0A" || field(ls[0], 26, 43) != "000000000000102026" {
		t.Fatalf("zero report %q", ls)
	}
	// A withdrawn incident is deleted by its number and date.
	b, sum, _ = Write(Submission{ORI: "VA0770000", Year: 2026, Month: 10, Deletes: []Delete{{Number: "R26-000003", Occurred: at("2026-09-30 10:00")}}, Loc: est})
	ls = lines(t, b)
	if sum.Deletes != 1 || sum.Zero || field(ls[0], 5, 6) != "1D" || field(ls[0], 26, 45) != "R26-000003  20260930" {
		t.Fatalf("delete %q %+v", ls, sum)
	}
	if _, _, err := Write(Submission{ORI: "VA077", Year: 2026, Month: 10}); err == nil {
		t.Fatal("a short ORI")
	}
}
