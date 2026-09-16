package telegram

import (
	"regexp"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
)

func TestRenderEscapesEverythingItDoesNotFormat(t *testing.T) {
	out := strings.Join(RenderMarkdown(`<b onclick="x">hi</b> & <script>alert(1)</script>`), "")
	if strings.Contains(out, "<script") || strings.Contains(out, "<b onclick") {
		t.Fatalf("raw HTML survived: %s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") || !strings.Contains(out, "&amp;") {
		t.Fatalf("expected escaped output: %s", out)
	}
}

func TestRenderFormatsTheAssistantsMarkdown(t *testing.T) {
	md := "## ملخص الطلبات\n" +
		"- طلب **PO-12** بقيمة `1,250.00`\n" +
		"| الحالة | العدد |\n|---|---|\n| قيد التنفيذ | 3 |\n" +
		"[افتح](https://dawa24.test/orders/1) و [خطر](javascript:alert(1))\n" +
		"```\nSELECT <x>\n```"
	out := strings.Join(RenderMarkdown(md), "\n")
	for _, want := range []string{
		"<b>ملخص الطلبات</b>",
		"• طلب <b>PO-12</b> بقيمة <code>1,250.00</code>",
		"الحالة  ·  العدد",
		`<a href="https://dawa24.test/orders/1">افتح</a>`,
		"<pre>SELECT &lt;x&gt;\n</pre>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "javascript") && strings.Contains(out, "href=\"javascript") {
		t.Fatalf("javascript: link rendered: %s", out)
	}
	if strings.Contains(out, "|---|") {
		t.Fatalf("table rule rendered: %s", out)
	}
}

var reOpen = regexp.MustCompile(`<(b|code|pre|a)[ >]`)
var reClose = regexp.MustCompile(`</(b|code|pre|a)>`)

func TestChunksStayWithinLimitAndBalanced(t *testing.T) {
	var md strings.Builder
	md.WriteString("```\n")
	for i := 0; i < 400; i++ {
		md.WriteString("سطر برمجي طويل نسبياً رقم ")
		md.WriteString(strings.Repeat("x", 10))
		md.WriteString("\n")
	}
	md.WriteString("```\n")
	for i := 0; i < 300; i++ {
		md.WriteString("- **بند** رقم مع نص عربي للتجربة\n")
	}
	md.WriteString(strings.Repeat("ع", 9000)) // one line longer than a message

	chunks := RenderMarkdown(md.String())
	if len(chunks) < 3 {
		t.Fatalf("expected several chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if n := units(c); n > 4096 {
			t.Errorf("chunk %d is %d UTF-16 units", i, n)
		}
		if o, cl := len(reOpen.FindAllString(c, -1)), len(reClose.FindAllString(c, -1)); o != cl {
			t.Errorf("chunk %d has %d opening and %d closing tags", i, o, cl)
		}
	}
}

func TestRenderLinksOnlyHTTP(t *testing.T) {
	out := renderLinks([]chatbridge.AnswerLink{
		{Title: "طلب 1", URL: "https://dawa24.test/customer/orders/1"},
		{Title: "x", URL: "tg://resolve?domain=evil"},
		{Title: "<b>", URL: "https://dawa24.test/a?b=1&c=2"},
	})
	if strings.Contains(out, "tg://") {
		t.Fatalf("non-http link rendered: %s", out)
	}
	if !strings.Contains(out, "&amp;c=2") || !strings.Contains(out, "&lt;b&gt;") {
		t.Fatalf("link not escaped: %s", out)
	}
}

func TestFormatAnswerIncludesFailure(t *testing.T) {
	msgs := formatAnswer(chatbridge.Answer{Markdown: "جزء", Failure: "انتهت المهلة <x>"})
	joined := strings.Join(msgs, "\n")
	if !strings.Contains(joined, "جزء") || !strings.Contains(joined, "&lt;x&gt;") {
		t.Fatalf("got %q", joined)
	}
	if got := formatAnswer(chatbridge.Answer{}); len(got) != 1 {
		t.Fatalf("empty answer must still reply once, got %d", len(got))
	}
}
