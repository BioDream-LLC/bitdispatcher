// Package textract reads the words inside an attachment so search can find them: plain text as it is, the
// text layer of a PDF, and (when an administrator turns it on and Tesseract is installed) the words in a
// picture. Files come from the field and the public, so every reader is bounded in time and size, and a
// file that cannot be read gives no text rather than an error to the person who sent it.
package textract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

// MaxText is the most text kept for one file.
const MaxText = 256 << 10

// Options say what may be used beyond the built-in readers.
type Options struct {
	// OCR reads the words in pictures with the tesseract program on this server, if it is installed.
	OCR bool
	// Timeout bounds one file (default 30 s).
	Timeout time.Duration
}

// ErrNoReader is returned for a kind of file no reader handles (audio, video, or a picture without OCR).
var ErrNoReader = errors.New("textract: nothing reads this kind of file")

// Extract returns the text inside a file of the given content type.
func Extract(ctx context.Context, typ string, body []byte, o Options) (string, error) {
	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	var s string
	var err error
	switch {
	case typ == "text/plain":
		s = string(body)
	case typ == "application/pdf":
		s, err = pdfText(ctx, body)
	case strings.HasPrefix(typ, "image/") && o.OCR:
		s, err = ocr(ctx, body)
	default:
		return "", ErrNoReader
	}
	if err != nil {
		return "", err
	}
	return clean(s), nil
}

// OCRAvailable reports whether the tesseract program is installed, and its version line.
func OCRAvailable() (bool, string) {
	p, err := exec.LookPath("tesseract")
	if err != nil {
		return false, ""
	}
	out, _ := exec.Command(p, "--version").CombinedOutput()
	line, _, _ := strings.Cut(string(out), "\n")
	return true, strings.TrimSpace(line)
}

// pdfText reads a PDF's text layer in its own goroutine, so a malformed file that stalls or panics the
// parser costs a timeout, not the server.
func pdfText(ctx context.Context, body []byte) (string, error) {
	type res struct {
		s   string
		err error
	}
	ch := make(chan res, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				ch <- res{err: fmt.Errorf("textract: unreadable PDF: %v", p)}
			}
		}()
		r, err := pdf.NewReader(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			ch <- res{err: err}
			return
		}
		var b strings.Builder
		for i := 1; i <= r.NumPage() && b.Len() < MaxText; i++ {
			if ctx.Err() != nil {
				break
			}
			p := r.Page(i)
			if p.V.IsNull() {
				continue
			}
			t, err := p.GetPlainText(nil)
			if err != nil {
				continue
			}
			b.WriteString(t)
			b.WriteByte('\n')
		}
		ch <- res{s: b.String()}
	}()
	select {
	case x := <-ch:
		return x.s, x.err
	case <-ctx.Done():
		return "", fmt.Errorf("textract: the PDF took too long to read")
	}
}

// ocr runs tesseract on a picture given on its standard input; its output is the words it read.
func ocr(ctx context.Context, body []byte) (string, error) {
	p, err := exec.LookPath("tesseract")
	if err != nil {
		return "", ErrNoReader
	}
	cmd := exec.CommandContext(ctx, p, "stdin", "stdout", "--psm", "3")
	cmd.Stdin = bytes.NewReader(body)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &limited{w: &out, n: MaxText}, &limited{w: &errb, n: 4096}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("textract: tesseract: %v %s", err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// clean keeps valid, printable text with single spaces, up to MaxText.
func clean(s string) string {
	if len(s) > MaxText*2 {
		s = s[:MaxText*2]
	}
	s = strings.ToValidUTF8(s, " ")
	var b strings.Builder
	space := false
	for _, r := range s {
		if unicode.IsSpace(r) || !unicode.IsPrint(r) {
			space = b.Len() > 0
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		if b.Len()+utf8.RuneLen(r) > MaxText {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

type limited struct {
	w io.Writer
	n int
}

func (l *limited) Write(p []byte) (int, error) {
	k := len(p)
	if len(p) > l.n {
		p = p[:l.n]
	}
	l.n -= len(p)
	if len(p) > 0 {
		l.w.Write(p)
	}
	return k, nil
}
