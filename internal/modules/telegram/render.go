package telegram

import (
	"html"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf16"
)

// Rendering the assistant's Markdown for Telegram.
//
// Telegram's HTML mode is strict: an unbalanced or unknown tag and the whole
// message is refused, which from the user's side is a question that was never
// answered. So this does not try to be a Markdown implementation. It escapes
// everything first, then adds back only tags it can prove are balanced — every
// inline tag opens and closes on the same line, and the one multi-line tag,
// <pre>, is tracked explicitly so a message split in the middle of a code block
// closes it and reopens it on the other side.

// MaxMessageUnits is the budget per Telegram message, below the API's 4096
// limit, which is counted in UTF-16 code units after entity parsing. The
// margin absorbs the <pre> tags a split may add.
const MaxMessageUnits = 3800

var (
	reHeading = regexp.MustCompile(`^#{1,6}\s+(.*)$`)
	reBullet  = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)$`)
	reTableHR = regexp.MustCompile(`^\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$`)
	reBold    = regexp.MustCompile(`\*\*([^*\n]+?)\*\*|__([^_\n]+?)__`)
	reLink    = regexp.MustCompile(`\[([^\]\n]+)\]\(([^)\s]+)\)`)
)

// RenderMarkdown converts assistant Markdown to Telegram HTML chunks, each
// within MaxMessageUnits.
func RenderMarkdown(md string) []string {
	md = strings.ReplaceAll(strings.TrimSpace(md), "\r\n", "\n")
	if md == "" {
		return nil
	}

	var lines []renderedLine
	inPre := false
	for _, raw := range strings.Split(md, "\n") {
		if strings.HasPrefix(strings.TrimSpace(raw), "```") {
			inPre = !inPre
			continue
		}
		if inPre {
			lines = append(lines, renderedLine{text: html.EscapeString(raw), pre: true})
			continue
		}
		if reTableHR.MatchString(raw) && strings.Contains(raw, "-") && strings.Contains(raw, "|") {
			continue
		}
		lines = append(lines, renderedLine{text: renderLine(raw)})
	}
	return chunk(lines, MaxMessageUnits)
}

type renderedLine struct {
	text string
	pre  bool
}

// renderLine formats one non-code line. Inline code is cut out first so
// nothing inside backticks is ever treated as formatting.
func renderLine(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if m := reHeading.FindStringSubmatch(trimmed); m != nil {
		return "<b>" + renderInline(m[1]) + "</b>"
	}
	if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") {
		cells := strings.Split(strings.Trim(trimmed, "|"), "|")
		for i := range cells {
			cells[i] = renderInline(strings.TrimSpace(cells[i]))
		}
		return strings.Join(cells, "  ·  ")
	}
	if m := reBullet.FindStringSubmatch(raw); m != nil {
		indent := ""
		if len(m[1]) >= 2 {
			indent = "  "
		}
		return indent + "• " + renderInline(m[2])
	}
	return renderInline(raw)
}

func renderInline(s string) string {
	parts := strings.Split(s, "`")
	if len(parts)%2 == 0 {
		// An unmatched backtick is literal text, not the start of code.
		parts[len(parts)-2] += "`" + parts[len(parts)-1]
		parts = parts[:len(parts)-1]
	}
	var b strings.Builder
	for i, p := range parts {
		if i%2 == 1 {
			b.WriteString("<code>" + html.EscapeString(p) + "</code>")
			continue
		}
		b.WriteString(formatText(p))
	}
	return b.String()
}

// formatText handles links and bold in a span that contains no code.
func formatText(s string) string {
	var b strings.Builder
	last := 0
	for _, loc := range reLink.FindAllStringSubmatchIndex(s, -1) {
		b.WriteString(boldAndEscape(s[last:loc[0]]))
		label, target := s[loc[2]:loc[3]], s[loc[4]:loc[5]]
		if safe := safeHTTPURL(target); safe != "" {
			b.WriteString(`<a href="` + html.EscapeString(safe) + `">` + boldAndEscape(label) + "</a>")
		} else {
			b.WriteString(boldAndEscape(label))
		}
		last = loc[1]
	}
	b.WriteString(boldAndEscape(s[last:]))
	return b.String()
}

func boldAndEscape(s string) string {
	var b strings.Builder
	last := 0
	for _, loc := range reBold.FindAllStringSubmatchIndex(s, -1) {
		b.WriteString(html.EscapeString(s[last:loc[0]]))
		inner := ""
		if loc[2] >= 0 {
			inner = s[loc[2]:loc[3]]
		} else {
			inner = s[loc[4]:loc[5]]
		}
		b.WriteString("<b>" + html.EscapeString(inner) + "</b>")
		last = loc[1]
	}
	b.WriteString(html.EscapeString(s[last:]))
	return b.String()
}

// safeHTTPURL returns u when it is an absolute http(s) URL, and "" otherwise.
// A javascript: or tg: target written by the model never becomes a link.
func safeHTTPURL(u string) string {
	parsed, err := url.Parse(strings.TrimSpace(u))
	if err != nil || parsed.Host == "" {
		return ""
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return ""
	}
	return parsed.String()
}

// chunk packs lines into messages. A line that alone exceeds the budget is
// hard-split as plain text, because any tags inside it could be cut in half.
func chunk(lines []renderedLine, budget int) []string {
	var (
		out     []string
		b       strings.Builder
		size    int
		openPre bool
	)
	flush := func() {
		if openPre {
			b.WriteString("</pre>")
			openPre = false
		}
		if s := strings.TrimSpace(b.String()); s != "" {
			out = append(out, s)
		}
		b.Reset()
		size = 0
	}
	add := func(l renderedLine) {
		if l.pre && !openPre {
			b.WriteString("<pre>")
			openPre = true
		} else if !l.pre && openPre {
			b.WriteString("</pre>")
			openPre = false
		}
		b.WriteString(l.text)
		b.WriteString("\n")
		size += units(l.text) + 1
	}

	for _, l := range lines {
		n := units(l.text) + 1
		if n > budget {
			flush()
			for _, piece := range splitUnits(plain(l.text), budget) {
				out = append(out, html.EscapeString(piece))
			}
			continue
		}
		if size+n > budget {
			flush()
		}
		add(l)
	}
	flush()
	return out
}

var reTag = regexp.MustCompile(`<[^>]+>`)

// plain strips tags and unescapes, for text that must be re-escaped whole.
func plain(s string) string { return html.UnescapeString(reTag.ReplaceAllString(s, "")) }

func units(s string) int {
	n := 0
	for _, r := range s {
		n += len(utf16.Encode([]rune{r}))
	}
	return n
}

// splitUnits cuts s into pieces whose escaped form stays within budget.
// Escaping can grow a piece (& becomes &amp;), so the budget is halved for the
// raw text; a line this long is already an outlier.
func splitUnits(s string, budget int) []string {
	limit := budget / 2
	var (
		out  []string
		cur  []rune
		size int
	)
	for _, r := range s {
		u := len(utf16.Encode([]rune{r}))
		if size+u > limit {
			out = append(out, string(cur))
			cur, size = nil, 0
		}
		cur = append(cur, r)
		size += u
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}

// escape is the single-string helper for text the bot writes itself.
func escape(s string) string { return html.EscapeString(s) }
