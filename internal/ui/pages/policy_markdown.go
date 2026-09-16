package pages

import (
	"context"
	"html"
	"io"
	"regexp"
	"strings"

	"github.com/a-h/templ"
)

var (
	boldRe   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicRe = regexp.MustCompile(`\*([^*]+)\*`)
)

// RenderPolicyMarkdown converts policy markdown content into beautifully formatted,
// accessible, semantic HTML components while stripping redundant metadata lines.
func RenderPolicyMarkdown(raw string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		htmlOutput := formatPolicyMarkdownToHTML(raw)
		_, err := io.WriteString(w, htmlOutput)
		return err
	})
}

func formatPolicyMarkdownToHTML(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(raw, "\n")

	var cleanLines []string
	inPreamble := true

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Filter out redundant top-level metadata lines that are already rendered in the page header
		if inPreamble {
			if strings.HasPrefix(trimmed, "# ") ||
				strings.Contains(trimmed, "تاريخ آخر تحديث") ||
				strings.Contains(trimmed, "اسم المنصة:") ||
				strings.Contains(trimmed, "الموقع الإلكتروني:") ||
				strings.Contains(trimmed, "الجهة المالكة والمشغلة:") ||
				strings.Contains(trimmed, "العنوان القانوني:") ||
				strings.Contains(trimmed, "رقم السجل التجاري:") ||
				strings.Contains(trimmed, "البطاقة الضريبية:") ||
				strings.Contains(trimmed, "نطاق الخدمة") ||
				strings.Contains(trimmed, "طبيعة المنصة:") {
				continue
			}
			if trimmed == "---" {
				continue
			}
			if trimmed != "" {
				inPreamble = false
			}
		}

		cleanLines = append(cleanLines, line)
	}

	var sb strings.Builder
	var listItems []string
	isOrderedList := false

	flushList := func() {
		if len(listItems) == 0 {
			return
		}
		if isOrderedList {
			sb.WriteString("<ol class=\"policy-ol\">\n")
			for _, item := range listItems {
				sb.WriteString("  <li class=\"policy-li\">" + parseInline(item) + "</li>\n")
			}
			sb.WriteString("</ol>\n")
		} else {
			sb.WriteString("<ul class=\"policy-ul\">\n")
			for _, item := range listItems {
				sb.WriteString("  <li class=\"policy-li\">" + parseInline(item) + "</li>\n")
			}
			sb.WriteString("</ul>\n")
		}
		listItems = nil
		isOrderedList = false
	}

	var pBuffer []string
	flushP := func() {
		if len(pBuffer) == 0 {
			return
		}
		pText := strings.TrimSpace(strings.Join(pBuffer, " "))
		if pText != "" {
			sb.WriteString("<p class=\"policy-p\">" + parseInline(pText) + "</p>\n")
		}
		pBuffer = nil
	}

	for _, line := range cleanLines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			flushP()
			flushList()
			continue
		}

		if trimmed == "---" {
			flushP()
			flushList()
			sb.WriteString("<hr class=\"policy-divider\"/>\n")
			continue
		}

		if strings.HasPrefix(trimmed, "### ") {
			flushP()
			flushList()
			heading := strings.TrimPrefix(trimmed, "### ")
			sb.WriteString("<h3 class=\"policy-h3\">" + parseInline(heading) + "</h3>\n")
			continue
		}

		if strings.HasPrefix(trimmed, "## ") {
			flushP()
			flushList()
			heading := strings.TrimPrefix(trimmed, "## ")
			sb.WriteString("<h2 class=\"policy-h2\">" + parseInline(heading) + "</h2>\n")
			continue
		}

		if strings.HasPrefix(trimmed, "# ") {
			flushP()
			flushList()
			heading := strings.TrimPrefix(trimmed, "# ")
			sb.WriteString("<h2 class=\"policy-h2\">" + parseInline(heading) + "</h2>\n")
			continue
		}

		// Check unordered list
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			flushP()
			if isOrderedList {
				flushList()
			}
			itemText := strings.TrimSpace(trimmed[2:])
			listItems = append(listItems, itemText)
			continue
		}

		// Check ordered list (e.g. "1. ", "2. ")
		if isNumericListStart(trimmed) {
			flushP()
			if !isOrderedList && len(listItems) > 0 {
				flushList()
			}
			isOrderedList = true
			dotIdx := strings.Index(trimmed, ".")
			itemText := strings.TrimSpace(trimmed[dotIdx+1:])
			listItems = append(listItems, itemText)
			continue
		}

		// Normal paragraph line
		if len(listItems) > 0 {
			flushList()
		}
		pBuffer = append(pBuffer, trimmed)
	}

	flushP()
	flushList()

	return sb.String()
}

func isNumericListStart(s string) bool {
	dotIdx := strings.Index(s, ".")
	if dotIdx <= 0 || dotIdx > 3 {
		return false
	}
	for i := 0; i < dotIdx; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > dotIdx+1 && (s[dotIdx+1] == ' ' || s[dotIdx+1] == '\t')
}

func parseInline(s string) string {
	s = html.EscapeString(s)
	s = boldRe.ReplaceAllString(s, "<strong>$1</strong>")
	s = italicRe.ReplaceAllString(s, "<em>$1</em>")
	return s
}
