package whatsapp

import (
	"net/url"
	"regexp"
	"strings"
	"unicode/utf16"
)

// Rendering the assistant's Markdown for WhatsApp.
//
// WhatsApp has no markup language to get wrong: *bold*, _italic_, `code` and
// ```blocks``` are plain characters it styles, and links are recognised in the
// text. So nothing needs escaping; what matters is that Markdown the model
// writes reads naturally (**x** becomes *x*, a table becomes lines) and that a
// message stays under the 4096-character limit, with a code block split across
// two messages closed and reopened.

// MaxMessageUnits is the budget per WhatsApp text message, below the API's
// 4096-character limit, counted in UTF-16 units to be safe for emoji.
const MaxMessageUnits = 3800

var (
	reHeading = regexp.MustCompile(`^#{1,6}\s+(.*)$`)
	reBullet  = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)$`)
	reTableHR = regexp.MustCompile(`^\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$`)
	reBold    = regexp.MustCompile(`\*\*([^*\n]+?)\*\*|__([^_\n]+?)__`)
	reLink    = regexp.MustCompile(`\[([^\]\n]+)\]\(([^)\s]+)\)`)
)

const fence = "```"

// RenderMarkdown converts assistant Markdown to WhatsApp text chunks, each
// within MaxMessageUnits.
func RenderMarkdown(md string) []string {
	md = strings.ReplaceAll(strings.TrimSpace(md), "\r\n", "\n")
	if md == "" {
		return nil
	}
	var lines []renderedLine
	inPre := false
	for _, raw := range strings.Split(md, "\n") {
		if strings.HasPrefix(strings.TrimSpace(raw), fence) {
			inPre = !inPre
			continue
		}
		switch {
		case inPre:
			lines = append(lines, renderedLine{text: raw, pre: true})
		case reTableHR.MatchString(raw) && strings.Contains(raw, "-") && strings.Contains(raw, "|"):
		default:
			lines = append(lines, renderedLine{text: renderLine(raw)})
		}
	}
	return chunk(lines, MaxMessageUnits)
}

type renderedLine struct {
	text string
	pre  bool
}

func renderLine(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if m := reHeading.FindStringSubmatch(trimmed); m != nil {
		return "*" + strings.Trim(renderInline(m[1]), "*") + "*"
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

// renderInline rewrites links and bold outside inline code.
func renderInline(s string) string {
	parts := strings.Split(s, "`")
	for i := 0; i < len(parts); i += 2 {
		if i == len(parts)-1 && len(parts)%2 == 0 {
			break
		}
		parts[i] = reLink.ReplaceAllStringFunc(parts[i], func(m string) string {
			sub := reLink.FindStringSubmatch(m)
			if u := safeHTTPURL(sub[2]); u != "" {
				return sub[1] + " (" + u + ")"
			}
			return sub[1]
		})
		parts[i] = reBold.ReplaceAllString(parts[i], "*$1$2*")
	}
	return strings.Join(parts, "`")
}

// safeHTTPURL returns u when it is an absolute http(s) URL, and "" otherwise.
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
// hard-split.
func chunk(lines []renderedLine, budget int) []string {
	var (
		out     []string
		b       strings.Builder
		size    int
		openPre bool
	)
	flush := func() {
		if openPre {
			b.WriteString(fence)
			openPre = false
		}
		if s := strings.TrimSpace(b.String()); s != "" {
			out = append(out, s)
		}
		b.Reset()
		size = 0
	}
	for _, l := range lines {
		n := units(l.text) + 1
		if n > budget-2*len(fence) {
			flush()
			out = append(out, splitUnits(l.text, budget)...)
			continue
		}
		if size+n+2*len(fence) > budget {
			flush()
		}
		if l.pre != openPre {
			b.WriteString(fence + "\n") // opens or closes the block
			openPre = l.pre
			size += len(fence) + 1
		}
		b.WriteString(l.text + "\n")
		size += n
	}
	flush()
	return out
}

func units(s string) int {
	n := 0
	for _, r := range s {
		n += len(utf16.Encode([]rune{r}))
	}
	return n
}

func splitUnits(s string, budget int) []string {
	var (
		out  []string
		cur  []rune
		size int
	)
	for _, r := range s {
		u := len(utf16.Encode([]rune{r}))
		if size+u > budget {
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
