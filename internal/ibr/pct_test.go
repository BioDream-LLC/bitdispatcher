package ibr

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/cad"
)

// corpus is a month of reports that exercises most of the segments and edits: crimes against persons,
// property and society, burglary, motor vehicle theft with recovery, drugs, an officer assaulted, a
// juvenile arrestee, a Group B arrest and an exceptional clearance.
func corpus() []cad.RMSReport {
	return []cad.RMSReport{
		{Number: "R26-000101", Occurred: at("2026-10-04 02:30"), LocationType: "20", Narrative: "Rear door forced.",
			Offenses: []cad.RMSOffense{{Code: "220", Using: []string{"N"}, Bias: "88", Entry: "F", EntryPts: []string{"2"}, Security: []string{"G"}, LeftBy: "6"}},
			People:   []cad.RMSPerson{{Role: "victim", Name: "DOE, JANE", DOB: "1980-05-01", Sex: "F", Race: "W", Ethnicity: "N", Resident: "R"}},
			Property: []cad.RMSProperty{{Loss: "stolen", Type: "26", Value: 600, Quantity: 2}, {Loss: "stolen", Type: "20", Value: 85, Quantity: 1}, {Loss: "recovered", Type: "26", Value: 300, Quantity: 1, RecoveredOn: "2026-10-06"}}},
		{Number: "R26-000102", Occurred: at("2026-10-11 23:10"), LocationType: "13", Narrative: "Fight.", Cleared: "arrest",
			Offenses: []cad.RMSOffense{{Code: "13A", Using: []string{"A"}, Bias: "88", Weapons: []string{"20"}, Activity: []string{"N"}}},
			People: []cad.RMSPerson{
				{Role: "victim", Name: "ROE, RICHARD", Age: "30-40", Sex: "M", Race: "B", Ethnicity: "N", Resident: "R", Injuries: []string{"L"}, Circumstances: []string{"01"}, Relations: []cad.RMSRelation{{Suspect: 1, Code: "AQ"}}},
				{Role: "suspect", Name: "POE, PAT", Age: "25", Sex: "M", Race: "W", Ethnicity: "N"},
				{Role: "arrestee", Name: "POE, PAT", Age: "25", Sex: "M", Race: "W", Ethnicity: "N", Resident: "R", ArrestDate: "2026-10-12", ArrestType: "O", Armed: []string{"01"}}}},
		{Number: "R26-000103", Occurred: at("2026-10-20 14:00"), LocationType: "18", Narrative: "Seized on a stop.", Cleared: "arrest",
			Offenses: []cad.RMSOffense{{Code: "35A", Using: []string{"N"}, Bias: "88", Activity: []string{"P"}}},
			Property: []cad.RMSProperty{{Loss: "seized", Type: "10", Drug: "E", DrugQty: 2.5, DrugUnit: "GM"}},
			People:   []cad.RMSPerson{{Role: "arrestee", Age: "33", Sex: "M", Race: "W", Ethnicity: "N", Resident: "R", ArrestDate: "2026-10-20", ArrestType: "O", Armed: []string{"01"}, ArrestDrugs: []string{"E"}}}},
		{Number: "R26-000104", Occurred: at("2026-10-08 17:45"), LocationType: "18", Narrative: "Car taken from the lot.",
			Offenses: []cad.RMSOffense{{Code: "240", Using: []string{"N"}, Bias: "88"}},
			Vehicles: []cad.RMSVehicle{{Role: "stolen", Plate: "TEST123", State: "VA", Year: "2019", Make: "TOYT"}},
			People:   []cad.RMSPerson{{Role: "victim", Name: "LOE, SAM", DOB: "1975-02-02", Sex: "M", Race: "W", Ethnicity: "N", Resident: "R"}},
			Property: []cad.RMSProperty{{Loss: "stolen", Type: "03", Value: 12000, Quantity: 1}, {Loss: "recovered", Type: "03", Value: 9000, Quantity: 1, RecoveredOn: "2026-10-10"}}},
		{Number: "R26-000105", Occurred: at("2026-10-14 21:05"), LocationType: "13", Narrative: "Officer struck at a traffic stop.", Cleared: "arrest",
			Offenses: []cad.RMSOffense{{Code: "13B", Using: []string{"N"}, Bias: "88", Weapons: []string{"40"}, Activity: []string{"N"}}},
			People: []cad.RMSPerson{
				{Role: "victim", Name: "OFFICER, ONE", Age: "41", Sex: "F", Race: "W", Ethnicity: "N", Resident: "R", VictimType: "L", OfficerAct: "07", OfficerAssign: "F", Injuries: []string{"M"}, Relations: []cad.RMSRelation{{Suspect: 1, Code: "ST"}}},
				{Role: "suspect", Age: "17", Sex: "M", Race: "B", Ethnicity: "N"},
				{Role: "arrestee", Age: "17", Sex: "M", Race: "B", Ethnicity: "N", Resident: "R", ArrestDate: "2026-10-14", ArrestType: "O", Armed: []string{"01"}, Juvenile: "H"}}},
		{Number: "R26-000106", Occurred: at("2026-10-02 09:00"), LocationType: "05", Narrative: "Graffiti on the school.", Cleared: "B", ClearedOn: "2026-10-09",
			Offenses: []cad.RMSOffense{{Code: "290", Using: []string{"N"}, Bias: "88"}},
			People:   []cad.RMSPerson{{Role: "victim", VictimType: "G"}, {Role: "suspect", Age: "15", Sex: "M", Race: "W", Ethnicity: "N"}},
			Property: []cad.RMSProperty{{Loss: "damaged", Type: "39", Value: 800, Quantity: 1}}},
		{Number: "R26-000107", Occurred: at("2026-10-15 01:00"), Narrative: "Traffic stop.", Cleared: "arrest",
			Offenses: []cad.RMSOffense{{Code: "90D"}},
			People:   []cad.RMSPerson{{Role: "arrestee", Name: "LOE, LEE", DOB: "1990-01-01", Sex: "F", Race: "A", Ethnicity: "N", Resident: "R", ArrestDate: "2026-10-15", ArrestType: "S", Armed: []string{"01"}}}},
	}
}

// TestSEARCHValidator checks the FBI-layout file with SEARCH's NIBRS Pre-Certification Tool validator
// (its 400-odd FBI edits), an independent check of the layout and the edits. It runs when
// NIBRS_VALIDATOR names the command (scripts/nibrs-validator.sh builds one); CI does not have Java.
func TestSEARCHValidator(t *testing.T) {
	cmd := os.Getenv("NIBRS_VALIDATOR")
	if cmd == "" {
		t.Skip("NIBRS_VALIDATOR is not set (scripts/nibrs-validator.sh)")
	}
	est, _ := time.LoadLocation("America/New_York")
	for _, c := range []struct {
		name string
		s    Submission
	}{
		{"month", Submission{ORI: "VA0770000", Year: 2026, Month: 10, Reports: corpus(), Loc: est, FBI: true}},
		{"zero", Submission{ORI: "VA0770000", Year: 2026, Month: 11, Loc: est, FBI: true}},
		{"delete", Submission{ORI: "VA0770000", Year: 2026, Month: 10, Deletes: []Delete{{Number: "R26-000003", Occurred: at("2026-09-30 10:00")}}, Loc: est, FBI: true}},
	} {
		b, sum, err := Write(c.s)
		if err != nil || len(sum.Skipped) > 0 {
			t.Fatalf("%s: %v %v", c.name, err, sum.Skipped)
		}
		f := filepath.Join(t.TempDir(), c.name+".txt")
		os.WriteFile(f, b, 0o644)
		out, err := exec.Command(cmd, f).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", c.name, err, out)
		}
		var errs []string
		for _, l := range strings.Split(string(out), "\n") {
			// The report has one line per error, and a last line saying it ran.
			if strings.TrimSpace(l) != "" && !strings.Contains(l, "processed submission") {
				errs = append(errs, strings.TrimRight(l, " \r"))
			}
		}
		if len(errs) > 0 || !strings.Contains(string(out), "processed submission") {
			t.Errorf("%s: SEARCH's validator found %d errors:\n%s\n%s", c.name, len(errs), strings.Join(errs, "\n"), b)
		}
	}
}

// TestFBILayout checks the FBI layout's segment lengths and that it is the Virginia file cut to them.
func TestFBILayout(t *testing.T) {
	est, _ := time.LoadLocation("America/New_York")
	va, _, _ := Write(Submission{ORI: "VA0770000", Year: 2026, Month: 10, Reports: corpus(), Loc: est})
	fbi, _, _ := Write(Submission{ORI: "VA0770000", Year: 2026, Month: 10, Reports: corpus(), Loc: est, FBI: true})
	vl, fl := lines(t, va), lines(t, fbi)
	if len(vl) != len(fl) {
		t.Fatalf("%d segments, %d in the FBI layout", len(vl), len(fl))
	}
	for i, l := range fl {
		want, ok := fbiLength[l[4]]
		if !ok {
			want = len(vl[i])
		}
		if len(l) != want || l[:4] != fmt.Sprintf("%04d", want) {
			t.Fatalf("level %c is %d long (%s), want %d", l[4], len(l), l[:4], want)
		}
		n := min(len(l), len(vl[i]))
		if l[4] == '6' {
			n = 79
		}
		if l[4:n] != vl[i][4:n] {
			t.Fatalf("level %c differs from Virginia's within the FBI fields:\n%s\n%s", l[4], l, vl[i])
		}
	}
}
