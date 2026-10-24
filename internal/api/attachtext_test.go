package api

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func samplePDF(text string) string {
	content := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", text)
	objs := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content), "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"}
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	var offs []int
	for i, o := range objs {
		offs = append(offs, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	x := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, x)
	return b.String()
}

// Words inside attachments are found by search, with where they matched; with a ClamAV daemon set (BD_CLAMD,
// host:port), the standard test file is refused on both kinds of upload and the test button reports it.
func TestAttachmentTextAndScan(t *testing.T) {
	ts, _, pw := newServer(t)
	d1 := login(t, ts, "d1", pw["d1"])
	admin := login(t, ts, "admin", pw["admin"])
	_, b := d1.do("POST", "/api/incidents", map[string]any{"type": "SF", "location": map[string]any{"lat": 40.27, "lon": -76.88}}, true)
	var inc struct{ Number string }
	json.Unmarshal(b, &inc)
	if code, b := d1.upload("/api/incidents/"+inc.Number+"/files?name=statement.txt", "Witness saw a blue pickup, plate KX44ZZ, leave north."); code != 200 {
		t.Fatalf("text upload %d %s", code, b)
	}
	code, b := d1.upload("/api/comms/files?name=report.pdf", samplePDF("Lab report: latent print matched case QW1987"))
	if code != 200 {
		t.Fatalf("pdf upload %d %s", code, b)
	}
	var pdf struct{ ID string }
	json.Unmarshal(b, &pdf)
	find := func(q string) string {
		for i := 0; i < 40; i++ {
			_, b := d1.do("POST", "/api/search", map[string]any{"q": q}, true)
			if strings.Contains(string(b), "text") && strings.Contains(string(b), `"attachments"`) {
				return string(b)
			}
			time.Sleep(50 * time.Millisecond)
		}
		return ""
	}
	if r := find("KX44ZZ"); !strings.Contains(r, "in the file's text") || !strings.Contains(r, "blue pickup") {
		t.Fatalf("the statement's text was not found: %s", r)
	}
	// A communication's attachment is found only through a communication the person may see; this one is on
	// none yet, so it is not offered.
	if _, b := d1.do("POST", "/api/search", map[string]any{"q": "QW1987"}, true); strings.Contains(string(b), "report.pdf") {
		t.Fatalf("an attachment on no communication was found: %s", b)
	}

	if code, b := d1.do("POST", "/api/comms", map[string]any{"kind": "message", "priority": "low", "subject": "Lab", "body": "See the report", "to": []string{"@user:d1"}, "files": []string{pdf.ID}}, true); code != 200 {
		t.Fatalf("comm %d %s", code, b)
	}
	if r := find("QW1987"); !strings.Contains(r, "in the attachment's text") {
		t.Fatalf("the PDF's text was not found: %s", r)
	}

	addr := os.Getenv("BD_CLAMD")
	if addr == "" {
		t.Log("BD_CLAMD not set: the virus scan is not tested against a ClamAV daemon")
		return
	}
	if code, b := admin.do("PUT", "/api/admin/connections", map[string]any{"clamav": addr}, true); code != 200 || !strings.Contains(string(b), addr) {
		t.Fatalf("set clamd %d %s", code, b)
	}
	_, b = admin.do("POST", "/api/admin/connections/clamav/test", map[string]any{}, true)
	var tr struct {
		OK       bool
		Version  string
		TestFile string
	}
	json.Unmarshal(b, &tr)
	if !tr.OK || !strings.HasPrefix(tr.Version, "ClamAV") || tr.TestFile == "" {
		t.Fatalf("clamd test: %s", b)
	}
	t.Logf("clamd %s caught the test file as %s", tr.Version, tr.TestFile)
	if code, b := d1.upload("/api/incidents/"+inc.Number+"/files?name=eicar.txt", string(eicar())); code != 422 || !strings.Contains(string(b), "virus scan") {
		t.Fatalf("incident file not refused: %d %s", code, b)
	}
	if code, b := d1.upload("/api/comms/files?name=eicar.txt", string(eicar())); code != 422 {
		t.Fatalf("attachment not refused: %d %s", code, b)
	}
	if code, b := d1.upload("/api/incidents/"+inc.Number+"/files?name=clean.txt", "nothing here"); code != 200 {
		t.Fatalf("clean file refused: %d %s", code, b)
	}
	if code, _ := admin.do("PUT", "/api/admin/connections", map[string]any{"clamav": "not an address"}, true); code != 422 {
		t.Fatal("a bad clamd address was taken")
	}
}
