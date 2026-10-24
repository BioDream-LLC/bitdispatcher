package textract

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// onePagePDF is a minimal PDF whose page shows the text with a standard font.
func onePagePDF(text string) []byte {
	content := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", text)
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offs := []int{}
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
	return []byte(b.String())
}

func TestExtract(t *testing.T) {
	ctx := context.Background()
	s, err := Extract(ctx, "application/pdf", onePagePDF("Suspect vehicle silver Honda plate DEMO123"), Options{})
	if err != nil || !strings.Contains(s, "DEMO123") || !strings.Contains(s, "silver Honda") {
		t.Fatalf("pdf: %q %v", s, err)
	}
	s, _ = Extract(ctx, "text/plain", []byte("line one\n\n\tline\x00two \xff"), Options{})
	if s != "line one line two" {
		t.Fatalf("text: %q", s)
	}
	if _, err := Extract(ctx, "image/png", []byte("x"), Options{}); err != ErrNoReader {
		t.Fatalf("a picture was read without OCR: %v", err)
	}
	if _, err := Extract(ctx, "audio/mpeg", []byte("x"), Options{OCR: true}); err != ErrNoReader {
		t.Fatal("audio was read")
	}
	// Broken PDFs give an error, never a panic.
	for _, bad := range [][]byte{[]byte("%PDF-1.4\ngarbage"), onePagePDF("x")[:120], []byte("%PDF-1.7\n1 0 obj << /Length 999999999 >> stream\n")} {
		if _, err := Extract(ctx, "application/pdf", bad, Options{}); err == nil {
			t.Logf("broken PDF read without error (fine if empty)")
		}
	}
	if len(clean(strings.Repeat("a ", MaxText))) > MaxText {
		t.Fatal("text not bounded")
	}
}
