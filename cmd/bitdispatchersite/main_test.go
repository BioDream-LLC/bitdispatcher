package main

import (
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The site builds from the checkout, every page and asset it links to exists, every feature of the README
// reaches the features page, and nothing personal or unfilled slips into it.
func TestBuild(t *testing.T) {
	out := t.TempDir()
	if err := build("../..", out, "https://bitdispatcher.com"); err != nil {
		t.Fatal(err)
	}
	readme, _ := os.ReadFile("../../README.md")
	feat, _ := section(string(readme), "## Everything it does", "## What it looks like")
	want := 0
	for _, a := range areas(feat) {
		want += len(a.items)
	}
	if want < 20 {
		t.Fatalf("only %d features read from the README", want)
	}
	page, err := os.ReadFile(filepath.Join(out, "features/index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(page), "<li>"); got < want {
		t.Errorf("features page has %d items, README %d", got, want)
	}
	link := regexp.MustCompile(`(?:href|src)="(/[^"#?]*)`)
	// Nothing of the machine that built it: its user name, home folder or host name, nor a placeholder.
	words := []string{`/Users/`, `/home/`, `\{\{`}
	if u, err := user.Current(); err == nil && len(u.Username) > 2 {
		words = append(words, regexp.QuoteMeta(u.Username))
	}
	if h, err := os.Hostname(); err == nil && len(h) > 2 {
		words = append(words, regexp.QuoteMeta(strings.Split(h, ".")[0]))
	}
	leak := regexp.MustCompile(`(?i)` + strings.Join(words, "|"))
	err = filepath.Walk(out, func(path string, info os.FileInfo, err error) error {
		if err != nil || !strings.HasSuffix(path, ".html") || strings.Contains(filepath.ToSlash(path), "/manual/") {
			return err
		}
		b, _ := os.ReadFile(path)
		if m := leak.Find(b); m != nil {
			t.Errorf("%s holds %q", path, m)
		}
		for _, m := range link.FindAllStringSubmatch(string(b), -1) {
			p := filepath.Join(out, filepath.FromSlash(m[1]))
			if strings.HasSuffix(m[1], "/") {
				p = filepath.Join(p, "index.html")
			}
			if _, err := os.Stat(p); err != nil {
				t.Errorf("%s links to %s, which is not in the site", strings.TrimPrefix(path, out), m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInline(t *testing.T) {
	for in, want := range map[string]string{
		"`MO 2A12` and **bold**":               "<code>MO 2A12</code> and <b>bold</b>",
		"[it says so](#status)":                `<a href="/proof/">it says so</a>`,
		"[docs/BITVOICE.md](docs/BITVOICE.md)": `<a href="https://github.com/BioDream-LLC/bitdispatcher/blob/main/docs/BITVOICE.md">docs/BITVOICE.md</a>`,
		"a <b> tag & more":                     "a &lt;b&gt; tag &amp; more",
	} {
		if got := inline(in); got != want {
			t.Errorf("inline(%q) = %q, want %q", in, got, want)
		}
	}
}

// Every technology article is a page with its contents list, is a tile on the home page and in a menu, and
// every translation of the manual is linked from the footer.
func TestArticlesAndLanguages(t *testing.T) {
	out := t.TempDir()
	if err := build("../..", out, "https://bitdispatcher.com"); err != nil {
		t.Fatal(err)
	}
	home, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if len(arts) < 20 {
		t.Fatalf("only %d articles", len(arts))
	}
	for _, a := range arts {
		p, err := os.ReadFile(filepath.Join(out, "tech", a.slug, "index.html"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(p), `class="toc-side"`) || strings.Count(string(p), "<h2 id=") < 5 {
			t.Errorf("%s: no contents list or too few sections", a.slug)
		}
		if a.words < 1200 {
			t.Errorf("%s: only %d words", a.slug, a.words)
		}
		if got := strings.Count(string(home), `href="/tech/`+a.slug+`/"`); got < 3 {
			t.Errorf("home page links %s %d times; want a tile, a menu entry and a footer link", a.slug, got)
		}
	}
	if len(langs) == 0 {
		t.Fatal("no translations of the manual")
	}
	for _, l := range langs {
		if !strings.Contains(string(home), `href="/manual/`+l.code+`/"`) {
			t.Errorf("home page does not link the %s manual", l.code)
		}
	}
}
