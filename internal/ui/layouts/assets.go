package layouts

import (
	"context"
	"fmt"
	"io"

	"github.com/a-h/templ"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Asset resolves a static asset path to the URL the browser should request.
//
// It is a variable rather than a direct call because the asset table lives in
// package ui, which imports this package: calling into it from here would be a
// cycle. ui installs the real resolver at start-up (see internal/ui/static.go);
// until then, and in any test that renders a layout without the rest of the
// server, the identity function below returns a path that still resolves.
//
// The alternative was the previous arrangement, where every asset URL carried a
// version string typed into the layout by hand — and those had already drifted
// apart from each other by two days.
var Asset = func(path string) string { return path }

// clientI18nScript emits an inline script defining window.dawaT and client-side strings.
// The script tag includes the per-request CSP nonce so it executes under the
// nonce-based Content-Security-Policy.
func clientI18nScript(lang, dir string) templ.Component {
	data := fmt.Sprintf(`window.__DAWA_LANG__=%q;window.__DAWA_DIR__=%q;window.__DAWA_I18N__={
"toast.network_error":%q,
"toast.success":%q,
"toast.unauthorized":%q,
"toast.forbidden":%q,
"toast.not_found":%q,
"toast.conflict":%q,
"toast.validation_error":%q,
"toast.server_error":%q,
"toast.unexpected_error":%q,
"preview.image":%q,
"preview.document":%q,
"preview.digital_file":%q
};window.dawaT=function(k,fallback){return (window.__DAWA_I18N__&&window.__DAWA_I18N__[k])||fallback||k;};`,
		lang, dir,
		i18n.T(lang, "toast.network_error"),
		i18n.T(lang, "toast.success"),
		i18n.T(lang, "toast.unauthorized"),
		i18n.T(lang, "toast.forbidden"),
		i18n.T(lang, "toast.not_found"),
		i18n.T(lang, "toast.conflict"),
		i18n.T(lang, "toast.validation_error"),
		i18n.T(lang, "toast.server_error"),
		i18n.T(lang, "toast.unexpected_error"),
		i18n.T(lang, "preview.image"),
		i18n.T(lang, "preview.document"),
		i18n.T(lang, "preview.digital_file"),
	)
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		nonce := Nonce(ctx)
		if nonce != "" {
			_, err := io.WriteString(w, `<script nonce="`+nonce+`">`+data+`</script>`)
			return err
		}
		_, err := io.WriteString(w, "<script>"+data+"</script>")
		return err
	})
}

