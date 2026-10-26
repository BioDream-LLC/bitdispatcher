// Command bitdispatchersite builds the bitdispatcher.com website into a directory of static files.
//
// The site is made from what this repository already publishes, so it cannot drift from it: the feature
// list, the tests and the status come out of README.md, the manual is the one docs/manual holds, and the
// screenshots are docs/screenshots. The pages themselves are HTML fragments in site/pages, wrapped in one
// layout (menu, footer, head) in layout.go.
//
//	go run ./cmd/bitdispatchersite -out dist-site -base https://bitdispatcher.com
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"html"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// demoURL is the public demonstration, named on every page.
const demoURL = "https://demo.bitdispatcher.com"

// page is one page of the site: a fragment in site/pages, wrapped in the layout.
type page struct {
	src, path, section, title, description, priority string
}

var pages = []page{
	{"home.html", "/", "", "bitdispatcher: open-source 911 computer-aided dispatch in one program",
		"bitdispatcher is a free, Apache-2.0 computer-aided dispatch system for 911 centres: incidents, units recommended by road time, an offline dispatch map, a screen for every monitor, mark-outs, a mobile terminal, NIBRS and NCIC, all from the browser.", "1.0"},
	{"features.html", "/features/", "learn", "Everything bitdispatcher does",
		"The dispatcher's console, screens for several monitors, fire houses, police mark-outs, the supervisor board, the offline map, the mobile terminal, records and reporting, itemised.", "0.9"},
	{"bitvoice.html", "/bitvoice/", "proof", "bitdispatcher with BitVoice, and without it",
		"bitdispatcher takes calls from any standards-compliant call-handling system through ALI, NENA i3 and EIDO, and uses every hook BitVoice, BioDream's open-source phone system and 911 call handling, offers a CAD.", "0.8"},
	{"proof.html", "/proof/", "proof", "How bitdispatcher is tested, and what has not been tested",
		"The tests every change must pass, what bitdispatcher is checked against, and what it has not been tried with yet. Not certified, not yet in service.", "0.8"},
	{"get.html", "/get/", "learn", "Get bitdispatcher",
		"Try the live demonstration in your browser, or build bitdispatcher from source: one Go program with its web interface inside, free under Apache 2.0.", "0.9"},
}

func main() {
	out := flag.String("out", "dist-site", "directory to write the site into (emptied first)")
	base := flag.String("base", "https://bitdispatcher.com", "the site's canonical origin, without a trailing slash")
	root := flag.String("root", ".", "the repository checkout")
	flag.Parse()
	if err := build(*root, *out, strings.TrimSuffix(*base, "/")); err != nil {
		fmt.Fprintln(os.Stderr, "bitdispatchersite:", err)
		os.Exit(1)
	}
}

// build writes the whole site.
func build(root, out, base string) error {
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		return err
	}
	md := string(readme)
	parts := map[string]string{}
	for key, h := range map[string][2]string{
		"FEATURES": {"## Everything it does", "## What it looks like"},
		"TESTED":   {"## How it is tested", "## Status"},
		"STATUS":   {"## Status", "## Documentation"},
		"BITVOICE": {"## With BitVoice and without it", "## How it is tested"},
	} {
		s, err := section(md, h[0], h[1])
		if err != nil {
			return err
		}
		parts[key] = s
	}
	if err := os.RemoveAll(out); err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	css, err := os.ReadFile(filepath.Join(root, "site/static/site.css"))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(css)
	cssVersion = hex.EncodeToString(sum[:])[:10]
	if err := os.WriteFile(filepath.Join(out, "site.css"), css, 0o644); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(root, "web/public/favicon.svg"), filepath.Join(out, "assets/bitdispatcher-logo.svg")); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(root, "site/static/biodream-wordmark.png"), filepath.Join(out, "assets/biodream-wordmark.png")); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(root, "site/static/social-card.png"), filepath.Join(out, "assets/social-card.png")); err != nil {
		return err
	}
	if err := copyShots(root, out); err != nil {
		return err
	}
	if err := loadArticles(root); err != nil {
		return err
	}
	if langs, err = manualLangs(root); err != nil {
		return err
	}
	manualHTML, err := os.ReadFile(filepath.Join(root, "docs/manual/index.html"))
	if err != nil {
		return err
	}
	var langLinks strings.Builder
	for _, l := range langs {
		fmt.Fprintf(&langLinks, `<a class="btn btn-line" href="/manual/%s/" hreflang="%s" lang="%s">%s</a>`, l.code, l.code, l.code, html.EscapeString(l.name))
	}

	fill := strings.NewReplacer(
		"{{DEMO}}", demoURL,
		"{{FEATURES}}", features(parts["FEATURES"]),
		"{{FEATURE_NAV}}", featureNav(parts["FEATURES"]),
		"{{TESTED}}", markdown(parts["TESTED"]),
		"{{STATUS}}", markdown(parts["STATUS"]),
		"{{BITVOICE}}", markdown(parts["BITVOICE"]),
		"{{BITVOICE_ABOUT}}", bitvoiceAbout(false),
		"{{BITVOICE_ABOUT_HERE}}", bitvoiceAbout(true),
		"{{TECH_TILES}}", tiles(),
		"{{TECH_COUNT}}", fmt.Sprint(len(arts)),
		"{{MANUAL_INDEX}}", manualIndex(string(manualHTML)),
		"{{LANGS}}", langLinks.String(),
		"{{LANG_COUNT}}", fmt.Sprint(len(langs)+1),
	)
	var sitemap strings.Builder
	sitemap.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n")
	for _, p := range pages {
		raw, err := os.ReadFile(filepath.Join(root, "site/pages", p.src))
		if err != nil {
			return err
		}
		body := iconRE.ReplaceAllStringFunc(fill.Replace(string(raw)), func(s string) string { return icon(strings.TrimPrefix(s, "ICON:")) })
		if strings.Contains(body, "{{") {
			return fmt.Errorf("%s: a placeholder was not filled", p.src)
		}
		if err := write(out, p.path, layout(base, p.path, p.section, p.title, p.description, body)); err != nil {
			return err
		}
		fmt.Fprintf(&sitemap, "<url><loc>%s%s</loc><priority>%s</priority></url>\n", base, p.path, p.priority)
	}
	// The technology articles, and their index.
	if err := write(out, "/tech/", layout(base, "/tech/", "learn", "The technology inside bitdispatcher, explained",
		"How a 911 dispatch system works, one technology at a time: NG9-1-1 and EIDO, caller location, the offline map, road-time recommendation, the ledger, NIBRS, NCIC and more.", techIndex())); err != nil {
		return err
	}
	fmt.Fprintf(&sitemap, "<url><loc>%s/tech/</loc><priority>0.8</priority></url>\n", base)
	for _, a := range arts {
		path := "/tech/" + a.slug + "/"
		if err := write(out, path, layout(base, path, a.group, a.title+" · bitdispatcher", a.description, articlePage(a))); err != nil {
			return err
		}
		fmt.Fprintf(&sitemap, "<url><loc>%s%s</loc><priority>0.7</priority></url>\n", base, path)
	}
	// The manual, as docs/manual holds it: one HTML file with its own styles, and the PDF; and each
	// translation (its PDF when make manual has built it).
	for _, f := range []string{"index.html", "bitdispatcher-manual.pdf"} {
		if err := copyFile(filepath.Join(root, "docs/manual", f), filepath.Join(out, "manual", f)); err != nil {
			return err
		}
	}
	fmt.Fprintf(&sitemap, "<url><loc>%s/manual/</loc><priority>0.8</priority></url>\n", base)
	for _, l := range langs {
		if err := copyFile(filepath.Join(root, "docs/manual", l.code, "index.html"), filepath.Join(out, "manual", l.code, "index.html")); err != nil {
			return fmt.Errorf("the %s manual is not built (make manual): %w", l.code, err)
		}
		if pdf := filepath.Join(root, "docs/manual", l.code, "bitdispatcher-manual.pdf"); fileExists(pdf) {
			if err := copyFile(pdf, filepath.Join(out, "manual", l.code, "bitdispatcher-manual.pdf")); err != nil {
				return err
			}
		}
		fmt.Fprintf(&sitemap, "<url><loc>%s/manual/%s/</loc><priority>0.5</priority></url>\n", base, l.code)
	}
	sitemap.WriteString("</urlset>\n")
	if err := os.WriteFile(filepath.Join(out, "sitemap.xml"), []byte(sitemap.String()), 0o644); err != nil {
		return err
	}
	robots := "User-agent: *\nAllow: /\nSitemap: " + base + "/sitemap.xml\n"
	if err := os.WriteFile(filepath.Join(out, "robots.txt"), []byte(robots), 0o644); err != nil {
		return err
	}
	notFound := `<section class="hero small"><div class="wrap"><p class="eyebrow">404</p><h1>That page is not here</h1>
<p class="lede">It may have moved when the site was rebuilt.</p>
<div class="actions"><a class="btn btn-primary" href="/">Home</a><a class="btn btn-ghost" href="/manual/">The manual</a></div></div></section>`
	return os.WriteFile(filepath.Join(out, "404.html"), []byte(layout(base, "/404.html", "", "Page not found · bitdispatcher", "This page does not exist.", notFound)), 0o644)
}

// write puts a page at its path: "/features/" is features/index.html.
func write(out, path, content string) error {
	f := filepath.Join(out, filepath.FromSlash(strings.TrimPrefix(path, "/")), "index.html")
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		return err
	}
	return os.WriteFile(f, []byte(content), 0o644)
}

// section is the README between two headings.
func section(readme, from, to string) (string, error) {
	i := strings.Index(readme, from)
	if i < 0 {
		return "", fmt.Errorf("README.md has no %q", from)
	}
	rest := readme[i+len(from):]
	j := strings.Index(rest, to)
	if j < 0 {
		return "", fmt.Errorf("README.md has no %q after %q", to, from)
	}
	return strings.TrimSpace(rest[:j]), nil
}

// area is one bold-headed group of the README's feature list.
type area struct {
	id, title string
	items     []string
}

var areaHeadRE = regexp.MustCompile(`^\*\*([^*]+)\*\*$`)

// areas splits the feature list into its groups: a line in bold, then its bullets.
func areas(md string) []area {
	var out []area
	for _, line := range strings.Split(md, "\n") {
		line = strings.TrimSpace(line)
		if m := areaHeadRE.FindStringSubmatch(line); m != nil {
			out = append(out, area{id: slug(m[1]), title: m[1]})
		} else if strings.HasPrefix(line, "- ") && len(out) > 0 {
			out[len(out)-1].items = append(out[len(out)-1].items, strings.TrimPrefix(line, "- "))
		}
	}
	return out
}

// areaIcons gives each group of the feature list its icon on the features page.
var areaIcons = map[string]string{
	"the-dispatchers-console": "console", "screens-for-several-monitors": "monitors", "fire-ems-and-police": "siren",
	"the-map-with-no-internet": "map", "in-the-field": "truck", "records-and-reporting": "file",
	"assisting-the-dispatcher": "spark", "built-to-keep-running": "shield", "call-taking-and-roles": "phone",
	"planning-and-the-numbers-to-call": "clock", "the-call-without-the-typing": "mic", "reviewing-and-releasing-the-record": "eye",
	"looking-after-the-people": "heart", "working-with-others": "globe",
}

// features turns the README's feature list into the site's sections: one per group, every item a line.
func features(md string) string {
	var b strings.Builder
	for _, a := range areas(md) {
		ic := areaIcons[a.id]
		if ic == "" {
			ic = "check"
		}
		fmt.Fprintf(&b, "<section class=\"area\" id=\"%s\">\n<div class=\"area-head\"><div class=\"icon\">%s</div><h2>%s</h2></div>\n<ul class=\"ticks\">\n", a.id, icon(ic), html.EscapeString(a.title))
		for _, it := range a.items {
			fmt.Fprintf(&b, "<li>%s</li>\n", inline(it))
		}
		b.WriteString("</ul>\n</section>\n")
	}
	return b.String()
}

// featureNav is the row of buttons to each group.
func featureNav(md string) string {
	var b strings.Builder
	for _, a := range areas(md) {
		fmt.Fprintf(&b, `<a class="btn btn-line" href="#%s">%s</a>`, a.id, html.EscapeString(a.title))
	}
	return b.String()
}

// markdown is enough of Markdown for the README's sections: paragraphs and bullet lists.
func markdown(md string) string {
	var b strings.Builder
	inList := false
	for _, block := range strings.Split(md, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- ") {
				if !inList {
					b.WriteString("<ul class=\"ticks\">\n")
					inList = true
				}
				fmt.Fprintf(&b, "<li>%s</li>\n", inline(strings.TrimPrefix(line, "- ")))
				continue
			}
			if inList {
				b.WriteString("</ul>\n")
				inList = false
			}
			fmt.Fprintf(&b, "<p>%s</p>\n", inline(line))
		}
	}
	if inList {
		b.WriteString("</ul>\n")
	}
	return b.String()
}

var (
	codeRE = regexp.MustCompile("`([^`]+)`")
	boldRE = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	linkRE = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
)

// inline is the README's inline Markdown as HTML: code, bold and links. Links to the README's own headings
// or files go to the site's pages, or to the file on GitHub.
func inline(s string) string {
	s = html.EscapeString(s)
	s = codeRE.ReplaceAllString(s, "<code>$1</code>")
	s = boldRE.ReplaceAllString(s, "<b>$1</b>")
	return linkRE.ReplaceAllStringFunc(s, func(m string) string {
		p := linkRE.FindStringSubmatch(m)
		return `<a href="` + siteLink(html.UnescapeString(p[2])) + `">` + p[1] + `</a>`
	})
}

func siteLink(u string) string {
	switch {
	case u == "#status", u == "#how-it-is-tested":
		return "/proof/"
	case u == "#with-bitvoice-and-without-it":
		return "/bitvoice/"
	case strings.HasPrefix(u, "#"):
		return "/features/"
	case strings.HasPrefix(u, "docs/manual/"):
		return "/manual/" + strings.TrimPrefix(u, "docs/manual/")
	case strings.HasPrefix(u, "/"):
		return html.EscapeString(u)
	case !strings.Contains(u, "://"):
		return "https://github.com/BioDream-LLC/bitdispatcher/blob/main/" + u
	}
	return html.EscapeString(u)
}

func slug(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case r == '\'' || r == '’':
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// copyShots copies docs/screenshots as WebP at the size they are shown (cwebp, else ImageMagick), or as
// the PNGs when neither is installed.
func copyShots(root, out string) error {
	shots, _ := filepath.Glob(filepath.Join(root, "docs/screenshots/*.png"))
	if len(shots) == 0 {
		return errors.New("no screenshots in docs/screenshots")
	}
	cwebp, _ := exec.LookPath("cwebp")
	convert, _ := exec.LookPath("convert")
	for _, f := range shots {
		name := strings.TrimSuffix(filepath.Base(f), ".png")
		dst := filepath.Join(out, "assets/screens", name+".webp")
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		var cmd *exec.Cmd
		switch {
		case cwebp != "" && widerThan(f, 1600):
			cmd = exec.Command(cwebp, "-quiet", "-q", "82", "-resize", "1600", "0", f, "-o", dst)
		case cwebp != "":
			cmd = exec.Command(cwebp, "-quiet", "-q", "82", f, "-o", dst) // never enlarged
		case convert != "":
			cmd = exec.Command(convert, f, "-resize", "1600x>", "-quality", "82", dst)
		}
		if cmd != nil {
			if b, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %v %s", name, err, b)
			}
			continue
		}
		if err := copyFile(f, filepath.Join(out, "assets/screens", name+".png")); err != nil {
			return err
		}
		pngFallback = true
	}
	return nil
}

// widerThan says whether a PNG is wider than w pixels (unreadable: assumed so).
func widerThan(path string, w int) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()
	c, err := png.DecodeConfig(f)
	return err != nil || c.Width > w
}

// pngFallback is set when the screenshots were copied as PNGs; the layout then rewrites the pages' .webp references.
var pngFallback bool

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	o, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(o, in); err != nil {
		o.Close()
		return err
	}
	return o.Close()
}
