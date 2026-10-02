package layouts

import (
	"context"
	"html"
	"io"

	"github.com/a-h/templ"
)

func homeCriticalStyle() templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		// This is generated repository CSS, never administrator input.
		_, err := io.WriteString(w, `<style id="home-critical" data-clarity-unmask="true" nonce="`+
			html.EscapeString(Nonce(ctx))+`">`+HomeCriticalCSS()+`</style>`)
		return err
	})
}
