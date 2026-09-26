package ui

import (
	"context"
	"errors"
	"net/http"

	"github.com/a-h/templ"
)

// renderPage writes a rendered templ component as an HTML response.
//
// It replaces 189 copies of the same three lines: set the content type, render,
// log on failure. Copies are how a response path drifts — a handler that forgets
// the content type serves HTML as plain text, and one that drops the error check
// loses the only signal that a page failed halfway through. Having one path also
// means the next improvement to it (a request id in the log line, a metric, a
// partial-render guard) is made once rather than 189 times.
//
// what names the page for the log and is the message the call site used before,
// so log output is unchanged.
//
// A render error is logged, not surfaced. By the time templ fails, bytes are
// usually already on the wire: the status line is sent and the response cannot
// become a 500. The honest options are a truncated page with a log line or a
// buffer big enough to hold every page before writing any of it, and the second
// costs more than the failure it prevents. Handlers that need to fail *before*
// rendering should call renderError instead, which still can.
func (h *UIHandler) renderPage(ctx context.Context, w http.ResponseWriter, what string, page templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := page.Render(renderContext(ctx), w); err != nil {
		h.log.ErrorContext(ctx, what, "error", err)
	}
}

// logLoadErr records a read a page renders around rather than fails on: the
// section shows empty, and the reason is in the log.
//
// Four hundred call sites used to discard these errors outright, so a database
// outage presented as an empty dashboard with nothing in the logs to say why.
// A cancelled request is the client leaving, not a failure, and is not logged.
func (h *UIHandler) logLoadErr(ctx context.Context, what string, err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	h.log.WarnContext(ctx, "page data unavailable", "what", what, "error", err)
}

// renderContext is the context a page renders under: the request's values —
// the CSP nonce, the signed-in actor, the language — without its deadline.
//
// templ checks ctx.Err() at the start of every component and stops rendering
// the moment it is set. Under the request deadline that meant a page whose
// data took most of the budget to load was cut off mid-render with no error
// shown: the vendor import review opened on "All" straight after staging, cold
// caches made the commit preview slow, and the page ended after the bulk bar
// with its table missing. The deadline exists to bound database work, which
// is finished by the time rendering starts; rendering is in-memory and takes
// milliseconds, and a page that has begun must be finished.
func renderContext(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}
