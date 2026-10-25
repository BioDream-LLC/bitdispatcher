package main

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// The shared layout: head, menu across the top, footer. Every page is wrapped here, so all of them have the
// same menu and nothing drifts.

// cssVersion is a hash of the stylesheet, put in its URL: a changed stylesheet is a new URL, so nobody sees
// new pages drawn with yesterday's styles.
var cssVersion = "dev"

// icons are 24-unit stroke icons drawn for this site, coloured by currentColor.
var icons = map[string]string{
	"console":   `<rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4M7 8h4M7 11h6"/>`,
	"monitors":  `<rect x="2" y="5" width="9" height="7" rx="1.2"/><rect x="13" y="5" width="9" height="7" rx="1.2"/><path d="M6.5 12v3M17.5 12v3M4 15h5M15 15h5M7.5 19h9"/>`,
	"siren":     `<path d="M7 18v-6a5 5 0 0 1 10 0v6"/><path d="M5 18h14v3H5zM12 3v2M4.2 6.2l1.4 1.4M19.8 6.2l-1.4 1.4"/>`,
	"map":       `<path d="M9 4 3 6v14l6-2 6 2 6-2V4l-6 2-6-2z"/><path d="M9 4v14M15 6v14"/>`,
	"pin":       `<path d="M12 21s-7-6.2-7-11.5a7 7 0 0 1 14 0C19 14.8 12 21 12 21z"/><circle cx="12" cy="9.5" r="2.5"/>`,
	"truck":     `<path d="M3 6h11v10H3zM14 9h4l3 3v4h-7"/><circle cx="7" cy="17.5" r="1.8"/><circle cx="17" cy="17.5" r="1.8"/>`,
	"file":      `<path d="M6 3h8l4 4v14H6z"/><path d="M14 3v4h4M9 12h6M9 15h6M9 18h4"/>`,
	"spark":     `<path d="M12 3v4M12 17v4M3 12h4M17 12h4M6 6l2.5 2.5M15.5 15.5 18 18M6 18l2.5-2.5M15.5 8.5 18 6"/>`,
	"shield":    `<path d="M12 3l7 3v5c0 5-3.2 8.4-7 10-3.8-1.6-7-5-7-10V6l7-3z"/><path d="M9 12l2 2 4-4"/>`,
	"check":     `<circle cx="12" cy="12" r="9"/><path d="M8 12.5l2.8 2.8L16.5 9"/>`,
	"book":      `<path d="M4 5a2 2 0 0 1 2-2h13v16H6a2 2 0 0 0-2 2V5z"/><path d="M8 7h7"/>`,
	"play":      `<circle cx="12" cy="12" r="9"/><path d="M10 8.5v7l6-3.5z"/>`,
	"github":    `<path d="M9 19c-4 1.3-4-2-6-2.5M15 21v-3.5c0-1 .1-1.4-.5-2 2.8-.3 5.5-1.4 5.5-6a4.6 4.6 0 0 0-1.3-3.2 4.3 4.3 0 0 0-.1-3.2s-1-.3-3.4 1.3a11.7 11.7 0 0 0-6.2 0C6.6 2.8 5.6 3.1 5.6 3.1a4.3 4.3 0 0 0-.1 3.2A4.6 4.6 0 0 0 4.2 9.5c0 4.6 2.7 5.7 5.5 6-.6.6-.6 1.2-.5 2V21"/>`,
	"code":      `<path d="M9 8l-5 4 5 4M15 8l5 4-5 4"/>`,
	"phone":     `<path d="M5 4h4l2 5-2.5 1.5a11 11 0 0 0 5 5L15 13l5 2v4a2 2 0 0 1-2 2A16 16 0 0 1 3 6a2 2 0 0 1 2-2"/>`,
	"radio":     `<rect x="5" y="8" width="14" height="13" rx="2"/><path d="M9 8 15 3M9 13h6M9 16.5h3"/>`,
	"clock":     `<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>`,
	"route":     `<circle cx="6" cy="18" r="2.2"/><circle cx="18" cy="6" r="2.2"/><path d="M8 18h7a3 3 0 0 0 0-6H9a3 3 0 0 1 0-6h7"/>`,
	"ledger":    `<rect x="4" y="3" width="16" height="18" rx="2"/><path d="M8 7h8M8 11h8M8 15h5"/><path d="M15.5 16.5l1.5 1.5 3-3"/>`,
	"menu":      `<path d="M4 7h16M4 12h16M4 17h16"/>`,
	"keyboard":  `<rect x="2" y="6" width="20" height="12" rx="2"/><path d="M6 10h1M10 10h1M14 10h1M18 10h0M6 14h12"/>`,
	"users":     `<circle cx="9" cy="8" r="3.2"/><path d="M3 20a6 6 0 0 1 12 0M16 4.5a3.2 3.2 0 0 1 0 6.3M18 14a6 6 0 0 1 3 6"/>`,
	"bell":      `<path d="M6 16V11a6 6 0 0 1 12 0v5l2 2H4z"/><path d="M10 21h4"/>`,
	"search":    `<circle cx="10.5" cy="10.5" r="6.5"/><path d="M15.5 15.5 21 21"/>`,
	"mail":      `<rect x="3" y="5" width="18" height="14" rx="2"/><path d="m3.5 6 8.5 7 8.5-7"/>`,
	"drone":     `<circle cx="5" cy="5" r="2.5"/><circle cx="19" cy="5" r="2.5"/><circle cx="5" cy="19" r="2.5"/><circle cx="19" cy="19" r="2.5"/><path d="M7 7l3 3M17 7l-3 3M7 17l3-3M17 17l-3-3"/><rect x="9.5" y="9.5" width="5" height="5" rx="1"/>`,
	"car":       `<path d="M5 16V12l2-5h10l2 5v4M3 16h18v3H3z"/><circle cx="7.5" cy="16" r="1.5"/><circle cx="16.5" cy="16" r="1.5"/>`,
	"globe":     `<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"/>`,
	"lock":      `<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3M12 15v2"/>`,
	"server":    `<rect x="3" y="4" width="18" height="7" rx="1.5"/><rect x="3" y="13" width="18" height="7" rx="1.5"/><path d="M7 7.5h.01M7 16.5h.01M11 7.5h6M11 16.5h6"/>`,
	"flask":     `<path d="M9 3h6M10 3v6L4.5 19a1.5 1.5 0 0 0 1.3 2h12.4a1.5 1.5 0 0 0 1.3-2L14 9V3"/><path d="M7 15h10"/>`,
	"heart":     `<path d="M12 20s-7-4.4-7-10a4 4 0 0 1 7-2.6A4 4 0 0 1 19 10c0 5.6-7 10-7 10z"/>`,
	"eye":       `<path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/>`,
	"layers":    `<path d="m12 3 9 5-9 5-9-5z"/><path d="m3 13 9 5 9-5"/>`,
	"mic":       `<rect x="9" y="3" width="6" height="11" rx="3"/><path d="M5 11a7 7 0 0 0 14 0M12 18v3M8 21h8"/>`,
	"broadcast": `<circle cx="12" cy="12" r="2"/><path d="M8.5 15.5a5 5 0 0 1 0-7M15.5 8.5a5 5 0 0 1 0 7M5.6 18.4a9 9 0 0 1 0-12.8M18.4 5.6a9 9 0 0 1 0 12.8"/>`,
	"plug":      `<path d="M9 3v5M15 3v5M6 8h12v3a6 6 0 0 1-12 0zM12 17v4"/>`,
	"arrow":     `<path d="M5 12h14M13 6l6 6-6 6"/>`,
}

func icon(name string) string {
	return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">` +
		icons[name] + `</svg>`
}

var iconRE = regexp.MustCompile(`ICON:[a-z]+`)

// navItem is one entry of a menu panel: an icon, a title and a line about it.
type navItem struct{ title, href, icon, about string }

// navMenu is one item across the top: a section and its panel.
type navMenu struct {
	key, title string
	items      []navItem
}

// langs is the manual's translations, for the menus and the footer.
var langs []manualLang

// menus is the menu across the top. The technology groups come from the articles; key matches a page's
// section, so the menu marks where the reader is.
func menus() []navMenu {
	var out []navMenu
	for _, g := range groups {
		m := navMenu{key: g.key, title: g.title}
		for _, slug := range g.slugs {
			a := bySlug[slug]
			m.items = append(m.items, navItem{a.tile, "/tech/" + a.slug + "/", a.icon, a.summary})
		}
		out = append(out, m)
	}
	out = append(out,
		navMenu{"proof", "Proof", []navItem{
			{"How it is tested", "/proof/", "check", "The tests every change passes, and what has not been tried yet"},
			{"With BitVoice, and without", "/bitvoice/", "phone", "Calls from BitVoice or any standards-compliant call handling"},
			{"Continuous integration", "https://github.com/BioDream-LLC/bitdispatcher/actions/workflows/ci.yml", "server", "Every change built and tested in public"},
		}},
		navMenu{"learn", "Learn", []navItem{
			{"All the technology", "/tech/", "layers", "Every article, grouped"},
			{"Everything it does", "/features/", "spark", "Every feature, and every chapter of the manual"},
			{"Manual", "/manual/", "book", "Every screen, every setting, in one document"},
			{"Manual as PDF", "/manual/bitdispatcher-manual.pdf", "file", "The same manual, for printing"},
			{"Manual in other languages", "/manual/" + firstLang() + "/", "globe", langNames(4)},
			{"Get it", "/get/", "code", "The live demo, or build it from source"},
		}},
	)
	return out
}

func firstLang() string {
	for _, l := range langs {
		if l.code == "es" {
			return "es"
		}
	}
	if len(langs) > 0 {
		return langs[0].code
	}
	return ""
}

// langNames is the first n translations' names, and how many more.
func langNames(n int) string {
	var names []string
	for _, l := range langs {
		if l.code == "es" {
			names = append([]string{l.name}, names...)
		} else {
			names = append(names, l.name)
		}
	}
	if len(names) <= n {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:n], ", "), len(names)-n)
}

// layout wraps a page body in the head, menu and footer.
func layout(base, path, section, title, description, body string) string {
	if pngFallback {
		body = strings.ReplaceAll(body, ".webp", ".png")
	}
	canonical := base + path
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	fmt.Fprintf(&b, "<title>%s</title>\n<meta name=\"description\" content=\"%s\">\n", html.EscapeString(title), html.EscapeString(description))
	fmt.Fprintf(&b, "<link rel=\"canonical\" href=\"%s\">\n", html.EscapeString(canonical))
	b.WriteString("<link rel=\"icon\" type=\"image/svg+xml\" href=\"/assets/bitdispatcher-logo.svg\">\n")
	fmt.Fprintf(&b, "<link rel=\"stylesheet\" href=\"/site.css?v=%s\">\n", cssVersion)
	fmt.Fprintf(&b, "<meta property=\"og:type\" content=\"website\">\n<meta property=\"og:title\" content=\"%s\">\n<meta property=\"og:description\" content=\"%s\">\n<meta property=\"og:url\" content=\"%s\">\n",
		html.EscapeString(title), html.EscapeString(description), html.EscapeString(canonical))
	fmt.Fprintf(&b, "<meta property=\"og:image\" content=\"%s/assets/screens/console.%s\">\n", base, map[bool]string{true: "png", false: "webp"}[pngFallback])
	b.WriteString("<meta name=\"author\" content=\"BioDream LLC\">\n</head>\n<body>\n<a class=\"skip\" href=\"#main\">Skip to content</a>\n")

	b.WriteString("<header class=\"top\"><div class=\"wrap\">\n<a class=\"brand\" href=\"/\"><img src=\"/assets/bitdispatcher-logo.svg\" alt=\"\" width=\"30\" height=\"30\"><span>bit<b>dispatcher</b></span></a>\n<nav aria-label=\"Main\"><ul class=\"menu\">\n")
	for _, m := range menus() {
		cur := ""
		if m.key == section {
			cur = ` aria-current="page"`
		}
		wide := ""
		if len(m.items) > 4 {
			wide = " wide"
		}
		fmt.Fprintf(&b, "<li><a class=\"top-link\" href=\"%s\"%s aria-haspopup=\"true\">%s</a><div class=\"panel%s\"><p class=\"panel-head\">%s</p><ul>", m.items[0].href, cur, m.title, wide, html.EscapeString(panelHead(m)))
		for _, it := range m.items {
			fmt.Fprintf(&b, `<li><a href="%s">%s<span><b>%s</b><small>%s</small></span></a></li>`, it.href, icon(it.icon), html.EscapeString(it.title), html.EscapeString(it.about))
		}
		b.WriteString("</ul></div></li>\n")
	}
	b.WriteString("</ul></nav>\n<div class=\"cta\">")
	b.WriteString(`<a class="btn btn-ghost" href="https://github.com/BioDream-LLC/bitdispatcher">` + icon("github") + `GitHub</a>`)
	b.WriteString(`<a class="btn btn-primary" href="` + demoURL + `">` + icon("play") + `Try the demo</a></div>` + "\n")
	b.WriteString(`<details class="mobile"><summary aria-label="Menu"><svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">` + icons["menu"] + `</svg></summary><nav aria-label="Main, small screens">`)
	for _, m := range menus() {
		fmt.Fprintf(&b, "<p>%s</p>", m.title)
		for _, it := range m.items {
			fmt.Fprintf(&b, "<a href=\"%s\">%s</a>", it.href, html.EscapeString(it.title))
		}
	}
	b.WriteString(`<p>bitdispatcher</p><a href="` + demoURL + `">Try the demo</a><a href="https://github.com/BioDream-LLC/bitdispatcher">GitHub</a></nav></details>` + "\n</div></header>\n")

	b.WriteString("<main id=\"main\">\n" + body + "\n</main>\n")
	b.WriteString(footer())
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

func panelHead(m navMenu) string {
	if g := groupOf(m.key); g.key != "" {
		return g.blurb
	}
	if m.key == "proof" {
		return "What is proven, and what is not yet"
	}
	return "The manual, the articles and the source"
}

// footer is the menu across the bottom: every section, the manual's languages, and who makes it.
func footer() string {
	var b strings.Builder
	b.WriteString(`<footer class="site"><div class="wrap">
<div class="cols">
<div class="about"><a class="brand" href="/"><img src="/assets/bitdispatcher-logo.svg" alt="" width="30" height="30"><span>bit<b>dispatcher</b></span></a>
<p>Free, open-source 911 computer-aided dispatch in one program, from BioDream LLC. Apache License 2.0.</p></div>
`)
	for _, g := range groups {
		fmt.Fprintf(&b, "<div><b>%s</b>", g.title)
		for _, slug := range g.slugs {
			fmt.Fprintf(&b, `<a href="/tech/%s/">%s</a>`, slug, html.EscapeString(bySlug[slug].tile))
		}
		b.WriteString("</div>\n")
	}
	b.WriteString(`<div><b>Product</b><a href="/features/">Everything it does</a><a href="/bitvoice/">With BitVoice</a><a href="` + demoURL + `">Live demonstration</a><a href="/get/">Get it</a>`)
	b.WriteString(`<b class="second">Evidence</b><a href="/proof/">How it is tested</a><a href="https://github.com/BioDream-LLC/bitdispatcher/actions/workflows/ci.yml">Continuous integration</a></div>` + "\n")
	b.WriteString(`<div><b>Manual</b><a href="/manual/">English</a>`)
	for _, l := range langs {
		fmt.Fprintf(&b, `<a href="/manual/%s/" hreflang="%s" lang="%s">%s</a>`, l.code, l.code, l.code, html.EscapeString(l.name))
	}
	b.WriteString(`<a href="/manual/bitdispatcher-manual.pdf">PDF</a></div>` + "\n")
	b.WriteString(`<div><b>Help</b><a href="/tech/">The technology</a><a href="https://github.com/BioDream-LLC/bitdispatcher">GitHub</a><a href="https://github.com/BioDream-LLC/bitdispatcher/issues/new">Ask a question</a><a href="https://biodream.io/support/">Support from BioDream</a></div>
</div>
<div class="maker"><a class="maker-logo" href="https://biodream.io"><img src="/assets/biodream-wordmark.png" alt="BioDream" width="88" height="60" loading="lazy"></a>
<p>bitdispatcher is a product of <a href="https://biodream.io">BioDream LLC</a>, which builds free and source-available software for healthcare, telecom and 911, and the support to run it. <a href="https://biodream.io/support/">Support from BioDream</a></p></div>
<div class="legal">© BioDream LLC. bitdispatcher is free software under the Apache License 2.0. It is not certified for live 911 dispatch and is not yet in service. NENA, CJIS, NCIC, IPAWS, PremierOne and other names belong to their owners; bitdispatcher is not affiliated with or endorsed by them.</div>
</div></footer>
`)
	return b.String()
}
