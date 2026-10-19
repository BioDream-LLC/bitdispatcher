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
	"console":  `<rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4M7 8h4M7 11h6"/>`,
	"monitors": `<rect x="2" y="5" width="9" height="7" rx="1.2"/><rect x="13" y="5" width="9" height="7" rx="1.2"/><path d="M6.5 12v3M17.5 12v3M4 15h5M15 15h5M7.5 19h9"/>`,
	"siren":    `<path d="M7 18v-6a5 5 0 0 1 10 0v6"/><path d="M5 18h14v3H5zM12 3v2M4.2 6.2l1.4 1.4M19.8 6.2l-1.4 1.4"/>`,
	"map":      `<path d="M9 4 3 6v14l6-2 6 2 6-2V4l-6 2-6-2z"/><path d="M9 4v14M15 6v14"/>`,
	"pin":      `<path d="M12 21s-7-6.2-7-11.5a7 7 0 0 1 14 0C19 14.8 12 21 12 21z"/><circle cx="12" cy="9.5" r="2.5"/>`,
	"truck":    `<path d="M3 6h11v10H3zM14 9h4l3 3v4h-7"/><circle cx="7" cy="17.5" r="1.8"/><circle cx="17" cy="17.5" r="1.8"/>`,
	"file":     `<path d="M6 3h8l4 4v14H6z"/><path d="M14 3v4h4M9 12h6M9 15h6M9 18h4"/>`,
	"spark":    `<path d="M12 3v4M12 17v4M3 12h4M17 12h4M6 6l2.5 2.5M15.5 15.5 18 18M6 18l2.5-2.5M15.5 8.5 18 6"/>`,
	"shield":   `<path d="M12 3l7 3v5c0 5-3.2 8.4-7 10-3.8-1.6-7-5-7-10V6l7-3z"/><path d="M9 12l2 2 4-4"/>`,
	"check":    `<circle cx="12" cy="12" r="9"/><path d="M8 12.5l2.8 2.8L16.5 9"/>`,
	"book":     `<path d="M4 5a2 2 0 0 1 2-2h13v16H6a2 2 0 0 0-2 2V5z"/><path d="M8 7h7"/>`,
	"play":     `<circle cx="12" cy="12" r="9"/><path d="M10 8.5v7l6-3.5z"/>`,
	"github":   `<path d="M9 19c-4 1.3-4-2-6-2.5M15 21v-3.5c0-1 .1-1.4-.5-2 2.8-.3 5.5-1.4 5.5-6a4.6 4.6 0 0 0-1.3-3.2 4.3 4.3 0 0 0-.1-3.2s-1-.3-3.4 1.3a11.7 11.7 0 0 0-6.2 0C6.6 2.8 5.6 3.1 5.6 3.1a4.3 4.3 0 0 0-.1 3.2A4.6 4.6 0 0 0 4.2 9.5c0 4.6 2.7 5.7 5.5 6-.6.6-.6 1.2-.5 2V21"/>`,
	"code":     `<path d="M9 8l-5 4 5 4M15 8l5 4-5 4"/>`,
	"phone":    `<path d="M5 4h4l2 5-2.5 1.5a11 11 0 0 0 5 5L15 13l5 2v4a2 2 0 0 1-2 2A16 16 0 0 1 3 6a2 2 0 0 1 2-2"/>`,
	"radio":    `<rect x="5" y="8" width="14" height="13" rx="2"/><path d="M9 8 15 3M9 13h6M9 16.5h3"/>`,
	"clock":    `<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>`,
	"route":    `<circle cx="6" cy="18" r="2.2"/><circle cx="18" cy="6" r="2.2"/><path d="M8 18h7a3 3 0 0 0 0-6H9a3 3 0 0 1 0-6h7"/>`,
	"ledger":   `<rect x="4" y="3" width="16" height="18" rx="2"/><path d="M8 7h8M8 11h8M8 15h5"/><path d="M15.5 16.5l1.5 1.5 3-3"/>`,
	"menu":     `<path d="M4 7h16M4 12h16M4 17h16"/>`,
}

func icon(name string) string {
	return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">` +
		icons[name] + `</svg>`
}

var iconRE = regexp.MustCompile(`ICON:[a-z]+`)

type navItem struct{ key, title, href string }

// menu is the top menu; key matches a page's section, so the menu marks where the reader is.
var menu = []navItem{
	{"features", "Features", "/features/"},
	{"bitvoice", "With BitVoice", "/bitvoice/"},
	{"proof", "Proof", "/proof/"},
	{"manual", "Manual", "/manual/"},
	{"get", "Get it", "/get/"},
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
	b.WriteString("<meta name=\"author\" content=\"BioDream Developer\">\n</head>\n<body>\n<a class=\"skip\" href=\"#main\">Skip to content</a>\n")

	b.WriteString("<header class=\"top\"><div class=\"wrap\">\n<a class=\"brand\" href=\"/\"><img src=\"/assets/bitdispatcher-logo.svg\" alt=\"\">bitdispatcher</a>\n<ul class=\"menu\">\n")
	for _, m := range menu {
		cur := ""
		if m.key == section {
			cur = ` aria-current="page"`
		}
		fmt.Fprintf(&b, "<li><a href=\"%s\"%s>%s</a></li>\n", m.href, cur, m.title)
	}
	b.WriteString("</ul>\n<div class=\"cta\">")
	b.WriteString(`<a class="btn btn-ghost" href="https://github.com/BioDream-LLC/bitdispatcher">` + icon("github") + `GitHub</a>`)
	b.WriteString(`<a class="btn btn-primary" href="` + demoURL + `">` + icon("play") + `Try the demo</a></div>` + "\n")
	b.WriteString(`<details class="mobile"><summary aria-label="Menu"><svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">` + icons["menu"] + `</svg></summary><nav>`)
	for _, m := range menu {
		fmt.Fprintf(&b, "<a href=\"%s\">%s</a>", m.href, m.title)
	}
	b.WriteString(`<a href="` + demoURL + `">Try the demo</a><a href="https://github.com/BioDream-LLC/bitdispatcher">GitHub</a></nav></details>` + "\n</div></header>\n")

	b.WriteString("<main id=\"main\">\n" + body + "\n</main>\n")
	b.WriteString(footer)
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

const footer = `<footer class="site"><div class="wrap">
<div class="cols">
<div><a class="brand" href="/"><img src="/assets/bitdispatcher-logo.svg" alt="">bitdispatcher</a>
<p>Open-source 911 computer-aided dispatch from BioDream LLC. Apache License 2.0.</p></div>
<div><b>Product</b><a href="/features/">Features</a><a href="/bitvoice/">With BitVoice</a><a href="` + demoURL + `">Live demonstration</a></div>
<div><b>Evidence</b><a href="/proof/">How it is tested</a><a href="https://github.com/BioDream-LLC/bitdispatcher/actions/workflows/ci.yml">Continuous integration</a></div>
<div><b>Help</b><a href="/manual/">Manual</a><a href="/manual/bitdispatcher-manual.pdf">Manual (PDF)</a><a href="/get/">Get it</a><a href="https://github.com/BioDream-LLC/bitdispatcher">GitHub</a></div>
</div>
<div class="legal">© BioDream LLC. bitdispatcher is free software under the Apache License 2.0. It is not certified for live 911 dispatch. NENA, CJIS, NCIC, VCIN and other names belong to their owners; bitdispatcher is not affiliated with or endorsed by them.</div>
</div></footer>
`
