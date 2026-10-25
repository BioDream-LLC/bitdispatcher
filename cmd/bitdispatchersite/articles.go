package main

import (
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The technology articles: one Markdown file each in site/articles, opened from a tile on the home page
// and from the menus. Each says what a technology is, how it works, and how bitdispatcher does it.

// article is one of site/articles/<slug>.md.
type article struct {
	slug, title, tile, summary, description, icon string
	group                                         string
	body                                          string // Markdown, without the front matter or the Related line
	related                                       []string
	words                                         int
}

// group is a set of articles shown together: one menu, one row of tiles.
type group struct {
	key, title, blurb string
	slugs             []string
}

// groups orders the articles. Every article must be in exactly one; build fails otherwise.
var groups = []group{
	{"dispatch", "Dispatch", "The console, the command line and the decisions on the floor", []string{
		"command-line-and-keys", "road-time-recommendation", "road-closures-and-traffic", "multi-monitor", "multi-agency", "messages-and-bolos", "search-and-recall"}},
	{"calls", "Calls", "Where the caller is, what they said, and what the map shows", []string{
		"nena-i3-eido", "caller-location", "location-links", "transcripts-and-translation", "alarms-and-permits", "cameras-near-an-incident",
		"crash-and-drones", "offline-map", "aerial-and-oblique-imagery"}},
	{"field", "Field", "Crews, stations and the public", []string{
		"mobile-data-terminal", "staffing-and-qualifications", "station-alerting-and-paging", "non-emergency-intake", "public-alerting-cap-ipaws",
		"public-incident-feed"}},
	{"records", "Records", "Reports, inquiries and the record afterwards", []string{
		"nibrs-reporting", "neris-and-nemsis", "ncic", "records-requests"}},
	{"platform", "Platform", "What keeps it running, honest and changeable", []string{
		"event-ledger", "security", "active-active-sites", "scripts-and-advisories", "training-and-staged-changes", "responder-wellbeing", "migration", "self-service-setup"}},
}

// arts is every article in group order, and bySlug finds one.
var (
	arts   []*article
	bySlug = map[string]*article{}
)

var (
	frontRE   = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n(.*)$`)
	relatedRE = regexp.MustCompile(`(?m)^Related: (.*)$`)
	techRE    = regexp.MustCompile(`\]\(/tech/([a-z0-9-]+)/?\)`)
)

// loadArticles reads site/articles in the order of groups.
func loadArticles(root string) error {
	arts, bySlug = nil, map[string]*article{}
	files, _ := filepath.Glob(filepath.Join(root, "site/articles/*.md"))
	have := map[string]bool{}
	for _, f := range files {
		have[strings.TrimSuffix(filepath.Base(f), ".md")] = true
	}
	for _, g := range groups {
		for _, slug := range g.slugs {
			if !have[slug] {
				return fmt.Errorf("site/articles/%s.md is missing", slug)
			}
			delete(have, slug)
			raw, err := os.ReadFile(filepath.Join(root, "site/articles", slug+".md"))
			if err != nil {
				return err
			}
			m := frontRE.FindStringSubmatch(strings.ReplaceAll(string(raw), "\r\n", "\n"))
			if m == nil {
				return fmt.Errorf("%s.md has no front matter", slug)
			}
			a := &article{slug: slug, group: g.key}
			for _, line := range strings.Split(m[1], "\n") {
				k, v, ok := strings.Cut(line, ": ")
				if !ok {
					continue
				}
				switch k {
				case "title":
					a.title = v
				case "tile":
					a.tile = v
				case "summary":
					a.summary = v
				case "description":
					a.description = v
				case "icon":
					a.icon = v
				}
			}
			if a.title == "" || a.tile == "" || a.summary == "" || a.description == "" {
				return fmt.Errorf("%s.md: front matter needs title, tile, summary and description", slug)
			}
			if _, ok := icons[a.icon]; !ok {
				return fmt.Errorf("%s.md: no icon %q", slug, a.icon)
			}
			body := m[2]
			if r := relatedRE.FindStringSubmatch(body); r != nil {
				for _, t := range techRE.FindAllStringSubmatch(r[1], -1) {
					a.related = append(a.related, t[1])
				}
				body = relatedRE.ReplaceAllString(body, "")
			}
			a.body = strings.TrimSpace(body)
			a.words = len(strings.Fields(a.body))
			arts = append(arts, a)
			bySlug[slug] = a
		}
	}
	for slug := range have {
		return fmt.Errorf("site/articles/%s.md is in no group", slug)
	}
	for _, a := range arts {
		for _, r := range a.related {
			if bySlug[r] == nil {
				return fmt.Errorf("%s.md: related article %q does not exist", a.slug, r)
			}
		}
	}
	return nil
}

func groupOf(key string) group {
	for _, g := range groups {
		if g.key == key {
			return g
		}
	}
	return group{}
}

// tile is an article's square: icon, group, title, summary.
func tile(a *article, n int) string {
	return fmt.Sprintf(`<a class="tile" href="/tech/%s/"><span class="tag">%s · %02d</span><span class="tile-icon">%s</span><h3>%s</h3><p>%s</p><span class="more">Read the article</span></a>`,
		a.slug, html.EscapeString(strings.ToUpper(groupOf(a.group).title)), n, icon(a.icon), html.EscapeString(a.tile), html.EscapeString(a.summary))
}

// tiles is every article as a square, in group order.
func tiles() string {
	var b strings.Builder
	b.WriteString(`<div class="tiles">`)
	for i, a := range arts {
		b.WriteString(tile(a, i+1))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// techIndex is the /tech/ page: every group with its tiles.
func techIndex() string {
	var b strings.Builder
	b.WriteString(`<section class="hero small"><div class="wrap"><p class="eyebrow">The technology inside</p><h1>How bitdispatcher works, one technology at a time</h1>
<p class="lede">Each article explains a technology a 911 centre depends on: what it is, the standard behind it, where dispatch systems usually fall short, how bitdispatcher does it, and how far that has been tested.</p></div></section>` + "\n")
	n := 0
	for _, g := range groups {
		fmt.Fprintf(&b, `<section class="band" id="%s"><div class="wrap"><div class="intro"><p class="eyebrow">%s</p><h2>%s</h2></div><div class="tiles">`, g.key, html.EscapeString(g.title), html.EscapeString(g.blurb))
		for _, slug := range g.slugs {
			n++
			b.WriteString(tile(bySlug[slug], n))
		}
		b.WriteString("</div></div></section>\n")
	}
	return b.String()
}

// articlePage is one article: its heading, the text with a contents list beside it, and related articles.
func articlePage(a *article) string {
	body, heads := articleHTML(a.body)
	g := groupOf(a.group)
	var b strings.Builder
	fmt.Fprintf(&b, `<section class="hero small article-hero"><div class="wrap"><p class="eyebrow"><a href="/tech/">Technology</a> / <a href="/tech/#%s">%s</a></p><h1>%s</h1><p class="lede">%s</p><p class="meta"><span class="tile-icon">%s</span>%d minute read</p></div></section>`+"\n",
		g.key, html.EscapeString(g.title), html.EscapeString(a.title), html.EscapeString(a.summary), icon(a.icon), max(1, (a.words+219)/220))
	if strings.Contains(a.body, "BitVoice") {
		body += bitvoiceAbout(false) // every article that names BitVoice says what it is
	}
	b.WriteString(`<div class="wrap article-wrap"><article class="prose">` + body + `</article><aside class="toc-side"><div class="sticky"><b>On this page</b><ol>`)
	for _, h := range heads {
		fmt.Fprintf(&b, `<li><a href="#%s">%s</a></li>`, h[0], h[1])
	}
	b.WriteString(`</ol><a class="btn btn-primary" href="` + demoURL + `">` + icon("play") + `Try the demo</a><a class="btn btn-line" href="/manual/">` + icon("book") + `The manual</a></div></aside></div>` + "\n")
	if len(a.related) > 0 {
		b.WriteString(`<section class="band soft"><div class="wrap"><div class="intro"><p class="eyebrow">Read next</p><h2>Related technology</h2></div><div class="tiles">`)
		for _, r := range a.related {
			b.WriteString(tile(bySlug[r], indexOf(bySlug[r])+1))
		}
		b.WriteString("</div></div></section>\n")
	}
	return b.String()
}

func indexOf(a *article) int {
	for i, x := range arts {
		if x == a {
			return i
		}
	}
	return 0
}

// articleHTML is the articles' Markdown as HTML: headings (with ids, returned for the contents list),
// paragraphs, lists, tables, notes and code. Anything else is escaped as text.
func articleHTML(md string) (string, [][2]string) {
	var b strings.Builder
	var heads [][2]string
	used := map[string]int{}
	lines := strings.Split(md, "\n")
	special := func(l string) bool {
		return strings.HasPrefix(l, "#") || strings.HasPrefix(l, "- ") || strings.HasPrefix(l, "|") || strings.HasPrefix(l, "> ") ||
			strings.HasPrefix(l, "```") || olRE.MatchString(l)
	}
	for i := 0; i < len(lines); {
		l := strings.TrimRight(lines[i], " ")
		switch {
		case strings.TrimSpace(l) == "":
			i++
		case strings.HasPrefix(l, "```"):
			var code []string
			for i++; i < len(lines) && !strings.HasPrefix(lines[i], "```"); i++ {
				code = append(code, lines[i])
			}
			i++
			b.WriteString("<pre><code>" + html.EscapeString(strings.Join(code, "\n")) + "</code></pre>\n")
		case strings.HasPrefix(l, "## ") || strings.HasPrefix(l, "### "):
			lvl, text := "h2", strings.TrimPrefix(l, "## ")
			if strings.HasPrefix(l, "### ") {
				lvl, text = "h3", strings.TrimPrefix(l, "### ")
			}
			id := slug(text)
			if used[id]++; used[id] > 1 {
				id = fmt.Sprintf("%s-%d", id, used[id])
			}
			if lvl == "h2" {
				heads = append(heads, [2]string{id, html.EscapeString(text)})
			}
			fmt.Fprintf(&b, "<%s id=\"%s\">%s</%s>\n", lvl, id, inline(text), lvl)
			i++
		case strings.HasPrefix(l, "- ") || olRE.MatchString(l):
			tag, re := "ul", ulRE
			if olRE.MatchString(l) {
				tag, re = "ol", olRE
			}
			b.WriteString("<" + tag + ">")
			for ; i < len(lines) && re.MatchString(lines[i]); i++ {
				item := re.ReplaceAllString(lines[i], "")
				// A wrapped item continues on indented lines.
				for i+1 < len(lines) && strings.HasPrefix(lines[i+1], "  ") && strings.TrimSpace(lines[i+1]) != "" {
					i++
					item += " " + strings.TrimSpace(lines[i])
				}
				b.WriteString("<li>" + inline(item) + "</li>")
			}
			b.WriteString("</" + tag + ">\n")
		case strings.HasPrefix(l, "|"):
			var rows [][]string
			for ; i < len(lines) && strings.HasPrefix(lines[i], "|"); i++ {
				cells := strings.Split(strings.Trim(strings.TrimSpace(lines[i]), "|"), "|")
				sep := true
				for j := range cells {
					cells[j] = strings.TrimSpace(cells[j])
					if !sepRE.MatchString(cells[j]) {
						sep = false
					}
				}
				if !sep {
					rows = append(rows, cells)
				}
			}
			b.WriteString(`<div class="table"><table><thead><tr>`)
			for _, c := range rows[0] {
				b.WriteString("<th>" + inline(c) + "</th>")
			}
			b.WriteString("</tr></thead><tbody>")
			for _, r := range rows[1:] {
				b.WriteString("<tr>")
				for _, c := range r {
					b.WriteString("<td>" + inline(c) + "</td>")
				}
				b.WriteString("</tr>")
			}
			b.WriteString("</tbody></table></div>\n")
		case strings.HasPrefix(l, "> "):
			var note []string
			for ; i < len(lines) && strings.HasPrefix(lines[i], "> "); i++ {
				note = append(note, strings.TrimPrefix(lines[i], "> "))
			}
			b.WriteString("<blockquote>" + inline(strings.Join(note, " ")) + "</blockquote>\n")
		default:
			var p []string
			for ; i < len(lines) && strings.TrimSpace(lines[i]) != "" && (len(p) == 0 || !special(lines[i])); i++ {
				p = append(p, strings.TrimSpace(lines[i]))
			}
			b.WriteString("<p>" + inline(strings.Join(p, " ")) + "</p>\n")
		}
	}
	return b.String(), heads
}

var (
	ulRE  = regexp.MustCompile(`^- `)
	olRE  = regexp.MustCompile(`^\d+\. `)
	sepRE = regexp.MustCompile(`^:?-+:?$`)
)

// manualLang is one translation of the manual, read from docs/manual/i18n.
type manualLang struct{ code, name string }

var langNameRE = regexp.MustCompile(`"name":\s*"([^"]+)"`)

// manualLangs lists the translations, in the order of their codes.
func manualLangs(root string) ([]manualLang, error) {
	files, _ := filepath.Glob(filepath.Join(root, "docs/manual/i18n/*.json"))
	var out []manualLang
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		m := langNameRE.FindSubmatch(b)
		if m == nil {
			return nil, errors.New(f + ": no name")
		}
		out = append(out, manualLang{strings.TrimSuffix(filepath.Base(f), ".json"), string(m[1])})
	}
	return out, nil
}

var (
	tocPartRE = regexp.MustCompile(`<li class="tp"><a href="#part-\d+">([^<]+)</a><ol>(.*?)</ol></li>`)
	tocItemRE = regexp.MustCompile(`<li><a href="#([^"]+)"><span>\d+\.</span> ([^<]+)</a></li>`)
)

// manualIndex lists every chapter of the manual by part, each linked, from the built manual's contents.
func manualIndex(manualHTML string) string {
	var b strings.Builder
	b.WriteString(`<div class="chapters">`)
	for _, p := range tocPartRE.FindAllStringSubmatch(manualHTML, -1) {
		fmt.Fprintf(&b, `<div class="chapter-part"><h3>%s</h3><ul>`, p[1])
		for _, it := range tocItemRE.FindAllStringSubmatch(p[2], -1) {
			fmt.Fprintf(&b, `<li><a href="/manual/#%s">%s</a></li>`, it[1], it[2])
		}
		b.WriteString("</ul></div>")
	}
	b.WriteString("</div>")
	return b.String()
}
