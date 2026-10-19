// Package table reads the spreadsheets an earlier system exports: CSV or tab-separated, with a header
// row, in UTF-8 (a byte-order mark is skipped) or Windows-1252, and guesses which column is which.
package table

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

// File is an open table.
type File struct {
	Headers []string
	r       *csv.Reader
	f       *os.File
	latin1  bool
	Row     int // the last row read, counting the header as row 1
}

// Open opens a table and reads its header.
func Open(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	br := bufio.NewReaderSize(f, 1<<16)
	head, _ := br.Peek(64 << 10)
	head = bytes.TrimPrefix(head, []byte("\xef\xbb\xbf"))
	if bom, _ := br.Peek(3); bytes.Equal(bom, []byte("\xef\xbb\xbf")) {
		br.Discard(3)
	}
	t := &File{f: f, latin1: !utf8.Valid(head)}
	// The delimiter is whichever of comma, tab, semicolon and pipe the first line has most of.
	first, _, _ := bytes.Cut(head, []byte("\n"))
	delim, best := ',', 0
	for _, d := range []rune{',', '\t', ';', '|'} {
		if n := bytes.Count(first, []byte(string(d))); n > best {
			delim, best = d, n
		}
	}
	t.r = csv.NewReader(br)
	t.r.Comma = delim
	t.r.FieldsPerRecord = -1
	t.r.LazyQuotes = true
	t.r.ReuseRecord = false
	h, err := t.Next()
	if err != nil {
		f.Close()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("the file is empty")
		}
		return nil, err
	}
	for i := range h {
		h[i] = strings.TrimSpace(h[i])
		if h[i] == "" {
			h[i] = fmt.Sprintf("column %d", i+1)
		}
	}
	t.Headers = h
	return t, nil
}

// Next reads a row; io.EOF at the end.
func (t *File) Next() ([]string, error) {
	rec, err := t.r.Read()
	if err != nil {
		return nil, err
	}
	t.Row++
	if t.latin1 {
		for i, v := range rec {
			rec[i] = fromLatin1(v)
		}
	}
	return rec, nil
}

// Map reads the next row as column name -> value.
func (t *File) Map() (map[string]string, error) {
	rec, err := t.Next()
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(t.Headers))
	for i, h := range t.Headers {
		if i < len(rec) {
			m[h] = strings.TrimSpace(rec[i])
		}
	}
	return m, nil
}

// Close closes the file.
func (t *File) Close() error { return t.f.Close() }

// Windows-1252 differs from Latin-1 in 0x80-0x9f.
var cp1252 = map[byte]rune{0x80: '€', 0x82: '‚', 0x83: 'ƒ', 0x84: '„', 0x85: '…', 0x86: '†', 0x87: '‡', 0x88: 'ˆ', 0x89: '‰', 0x8a: 'Š', 0x8b: '‹', 0x8c: 'Œ',
	0x8e: 'Ž', 0x91: '\u2018', 0x92: '\u2019', 0x93: '\u201c', 0x94: '\u201d', 0x95: '•', 0x96: '–', 0x97: '—', 0x98: '˜', 0x99: '™', 0x9a: 'š', 0x9b: '›', 0x9c: 'œ', 0x9e: 'ž', 0x9f: 'Ÿ'}

func fromLatin1(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if r, ok := cp1252[c]; ok {
			b.WriteRune(r)
		} else {
			b.WriteRune(rune(c))
		}
	}
	return b.String()
}

// Guess maps our fields to the table's columns, from lists of likely names (upper case, without spaces
// or punctuation), best first. A column is used once.
func Guess(headers []string, likely map[string][]string, order []string) map[string]string {
	norm := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' {
				return r - 32
			}
			if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, s)
	}
	byNorm := map[string]string{}
	for _, h := range headers {
		if _, ok := byNorm[norm(h)]; !ok {
			byNorm[norm(h)] = h
		}
	}
	out := map[string]string{}
	used := map[string]bool{}
	for _, ours := range order {
		for _, l := range likely[ours] {
			if h, ok := byNorm[norm(l)]; ok && !used[h] {
				out[ours], used[h] = h, true
				break
			}
		}
	}
	return out
}

// Sample is the first rows of a table and how many there are in all.
func Sample(path string, n int) (headers []string, rows []map[string]string, count int, err error) {
	t, err := Open(path)
	if err != nil {
		return nil, nil, 0, err
	}
	defer t.Close()
	for {
		m, err := t.Map()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return t.Headers, rows, count, fmt.Errorf("row %d: %w", t.Row, err)
		}
		if len(rows) < n {
			rows = append(rows, m)
		}
		count++
	}
	return t.Headers, rows, count, nil
}
