package layouts

import (
	"context"
	"html"
	"io"

	"github.com/a-h/templ"
)

// SecurityScriptSource is installed by ui from the minified embedded asset.
// Keeping the policy synchronous catches image errors during HTML parsing and
// installs Trusted Types before Clarity, HTMX, and page scripts execute.
var SecurityScriptSource func() string

func SecurityScript() templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		opening := `<script data-security-runtime nonce="` + html.EscapeString(Nonce(ctx)) + `"`
		if SecurityScriptSource == nil {
			_, err := io.WriteString(w, opening+` src="`+html.EscapeString(Asset("/static/js/security.js"))+`"></script>`)
			return err
		}
		// Only compiled repository source reaches this component; never user HTML.
		_, err := io.WriteString(w, opening+`>`+SecurityScriptSource()+`</script>`)
		return err
	})
}
