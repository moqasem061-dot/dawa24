package matchreview

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestEveryMatchingReasonIsLocalized(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.AR, i18n.EN} {
		for _, code := range productmatch.AllReasonCodes() {
			label := reasonText(lang, string(code))
			if label == string(code) || label == i18n.T(lang, "match.reason.unknown") || strings.Contains(label, "match.reason.") {
				t.Errorf("%s: untranslated reason %s", lang, code)
			}
		}
	}
}

func TestReasonRenderingUsesRequestLanguageAndSafeFallback(t *testing.T) {
	for _, tc := range []struct {
		lang      i18n.Lang
		text, dir string
	}{
		{i18n.AR, "اختلاف في كلمات الاسم", "rtl"},
		{i18n.EN, "Different name details", "ltr"},
	} {
		t.Run(string(tc.lang), func(t *testing.T) {
			props := ReasonsProps{
				Reasons: []matchflow.Reason{{Code: "word_mismatch"}, {Code: "new_internal_code", Params: map[string]string{"unmapped_internal_key": "<script>alert(1)</script>"}}},
				Flags:   []string{"word_mismatch", "retrieval_incomplete", "retrieval_incomplete"},
			}
			var buf bytes.Buffer
			if err := Reasons(props).Render(httpx.WithLang(context.Background(), tc.lang), &buf); err != nil {
				t.Fatal(err)
			}
			html := buf.String()
			for _, expected := range []string{tc.text, `dir="` + tc.dir + `"`, i18n.T(tc.lang, "match.reason.unknown"), i18n.T(tc.lang, "match.param.details")} {
				if !strings.Contains(html, expected) {
					t.Errorf("missing %q", expected)
				}
			}
			for _, leaked := range []string{"new_internal_code", "unmapped_internal_key", "word_mismatch", "<script>"} {
				if strings.Contains(html, leaked) {
					t.Errorf("leaked %q", leaked)
				}
			}
			if strings.Count(html, "<li ") != 3 {
				t.Errorf("expected three distinct reasons, got %s", html)
			}
		})
	}
}
