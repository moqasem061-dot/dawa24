package antiscrape

import (
	"encoding/json"
	"html"
	"net/http"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Refusal reasons. They are logged, not shown: telling a scraper which signal
// caught it is telling it what to change.
const (
	reasonAutomation = "automation_user_agent"
	reasonBudget     = "request_budget_exceeded"
	reasonPenalty    = "penalised_caller"
)

// retryAfterSeconds is what a rate-limited caller is told to wait. It matches
// the burst window: a browser that hit the limit is browsing again in ten
// seconds, and a crawler that honours the header backs off correctly.
const retryAfterSeconds = "10"

func statusFor(reason string) int {
	if reason == reasonBudget {
		return http.StatusTooManyRequests
	}
	return http.StatusForbidden
}

// refuse ends the request and records why.
func (g *Guard) refuse(w http.ResponseWriter, r *http.Request, reason string, class Class) {
	status := statusFor(reason)

	g.log.WarnContext(r.Context(), "antiscrape: request refused",
		"reason", reason,
		"class", class.String(),
		"status", status,
		"path", r.URL.Path,
		"ip", httpx.ClientIP(r, g.hops),
		"user_agent", r.Header.Get("User-Agent"),
	)

	h := w.Header()
	// An error page must never become the indexed version of a guarded route.
	h.Set("X-Robots-Tag", "noindex, nofollow")
	h.Set("Cache-Control", "no-store")
	if status == http.StatusTooManyRequests {
		h.Set("Retry-After", retryAfterSeconds)
	}
	// A refusal that is only a refusal is a dead end for a legitimate
	// integrator. This platform publishes a documented API and an agent
	// discovery surface, so the refusal says where to go instead — in a header
	// a machine reads, and in prose a person reads.
	h.Set("Link", `</api/v1/openapi.json>; rel="service-desc", </docs/api>; rel="help"`)

	// The keys are spelled out on both branches rather than selected into a
	// variable: i18n.T takes format arguments, so a non-constant key is a
	// printf-style vet failure.
	lang := httpx.LangFrom(r.Context())
	title := i18n.T(lang, "security.antiscrape.blocked_title")
	body := i18n.T(lang, "security.antiscrape.blocked_body")
	if status == http.StatusTooManyRequests {
		title = i18n.T(lang, "security.antiscrape.throttled_title")
		body = i18n.T(lang, "security.antiscrape.throttled_body")
	}

	if wantsJSON(r) {
		h.Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"code":         "request.refused",
				"message":      body,
				"service_desc": "/api/v1/openapi.json",
				"docs":         "/docs/api",
			},
		})
		return
	}

	h.Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(refusalPage(string(lang), lang.Dir(), title, body)))
}

// wantsJSON decides the response shape. htmx is deliberately not counted: it
// swaps HTML into the page, so a JSON envelope would render as raw text.
func wantsJSON(r *http.Request) bool {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		return true
	}
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json") && !strings.Contains(accept, "text/html")
}

// refusalPage is a self-contained page: no stylesheet, no script, no layout
// import. platform/ must not reach into the UI layer, and a refused request is
// the wrong moment to load six assets.
func refusalPage(lang, dir, title, body string) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="`)
	b.WriteString(html.EscapeString(lang))
	b.WriteString(`" dir="`)
	b.WriteString(html.EscapeString(dir))
	b.WriteString(`"><head><meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1">`)
	b.WriteString(`<meta name="robots" content="noindex, nofollow"><title>`)
	b.WriteString(html.EscapeString(title))
	b.WriteString(`</title><style>body{margin:0;min-height:100vh;display:flex;align-items:center;` +
		`justify-content:center;background:#f8fafc;color:#0f172a;` +
		`font-family:system-ui,-apple-system,'Segoe UI',Roboto,sans-serif}` +
		`main{max-width:32rem;padding:2rem;text-align:center}` +
		`h1{font-size:1.25rem;margin:0 0 .75rem}` +
		`p{font-size:.875rem;line-height:1.7;color:#475569;margin:0 0 1.5rem}` +
		`a{display:inline-block;margin:0 .25rem;padding:.6rem 1.25rem;border-radius:.75rem;` +
		`background:#0284c7;color:#fff;text-decoration:none;font-weight:700}` +
		`a.secondary{background:transparent;color:#0284c7;border:1px solid #cbd5e1}` +
		`</style></head><body><main><h1>`)
	b.WriteString(html.EscapeString(title))
	b.WriteString(`</h1><p>`)
	b.WriteString(html.EscapeString(body))
	b.WriteString(`</p><a href="/">`)
	b.WriteString(html.EscapeString(i18n.T(lang, "security.antiscrape.back_home")))
	b.WriteString(`</a> <a class="secondary" href="/docs/api">`)
	b.WriteString(html.EscapeString(i18n.T(lang, "security.antiscrape.use_the_api")))
	b.WriteString(`</a></main></body></html>`)
	return b.String()
}
