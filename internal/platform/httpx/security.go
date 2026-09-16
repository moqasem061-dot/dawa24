package httpx

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/a-h/templ"
)

// CSPReportPath is where browsers deliver Content-Security-Policy violation
// reports. It is mounted on every host; see ReportHostOnly for the host that
// serves nothing else.
const CSPReportPath = "/api/v1/csp-report"

// SecurityOptions configures SecurityHeaders.
type SecurityOptions struct {
	// ReportURL is the absolute URL violation reports are sent to. It should
	// live on another origin than the site (CSP_REPORT_URL): a report endpoint
	// on the page's own origin lets anyone who can inject markup flood the
	// application's logs and rate limiter from its users' browsers, and it
	// shares the site's cookies. Empty falls back to CSPReportPath on the
	// site's own origin, which is what development uses.
	ReportURL string
}

// SecurityHeaders applies baseline hardening and a strict, nonce-based
// Content-Security-Policy.
//
// A fresh 16-byte CSP nonce is generated for every request and stored in the
// context (WithNonce, and templ.WithNonce for the elements templ emits).
// Templates read it with layouts.Nonce(ctx) to stamp <script> and <style>
// elements, and the policy allows nothing inline that does not carry it:
//
//   - script-src has no 'unsafe-inline' and no 'unsafe-eval'. Alpine is the
//     @alpinejs/csp build, which interprets expressions without eval, and
//     htmx runs with allowEval off.
//   - script-src-attr 'none': inline event handlers (onclick=...) never run.
//     Pages bind behaviour with Alpine directives or data-action attributes
//     handled by /static/js/actions.js.
//   - style-src has no 'unsafe-inline' and style-src-attr is 'none', so an
//     injected <style> or style="" attribute is never applied — CSS
//     injection is an exfiltration channel (attribute selectors, @font-face
//     unicode-range). Dynamic geometry is set through the CSSOM, which CSP
//     does not govern.
//   - require-trusted-types-for 'script': DOM sinks (innerHTML, script.src,
//     ...) only accept values minted by the policies in
//     /static/js/trusted-types.js.
func SecurityHeaders(opts SecurityOptions) func(http.Handler) http.Handler {
	reportURL := opts.ReportURL
	if reportURL == "" {
		reportURL = CSPReportPath
	}
	// Both endpoints go to the same collector. "csp" carries the policy's
	// violation reports; "default" is where browsers send deprecation,
	// intervention and crash reports, which the collector accepts and
	// discards at debug level.
	reportingEndpoints := `csp="` + reportURL + `", default="` + reportURL + `"`
	staticDirectives := cspStaticDirectives(reportURL)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "SAMEORIGIN")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			// microphone=(self) and not microphone=(): the assistant's voice input
			// calls getUserMedia({audio:true}) from this origin. With the
			// microphone disabled for every origin including our own, that call
			// rejected before the browser ever showed a permission prompt, so the
			// microphone button did nothing at all and the failure was invisible.
			// Camera stays closed: nothing here opens a camera stream, and file
			// attachment via <input capture> goes through the OS picker instead.
			h.Set("Permissions-Policy", "geolocation=(self), camera=(), microphone=(self)")
			h.Set("Cross-Origin-Opener-Policy", "same-origin-allow-popups")
			h.Set("Content-Signal", "ai-train=no, search=yes, ai-input=no")
			// HSTS: one year, including subdomains. Without this header, a single
			// plaintext HTTP request — a bare hostname bookmark, an image on a
			// subdomain — transmits a 30-day session token in cleartext.
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			h.Set("X-Permitted-Cross-Domain-Policies", "none")
			h.Set("Origin-Agent-Cluster", "?1")
			h.Set("Reporting-Endpoints", reportingEndpoints)

			// 16 random bytes → 22-char base64 is the minimum OWASP
			// recommends; crypto/rand is the only acceptable source.
			var nonceBuf [16]byte
			_, _ = rand.Read(nonceBuf[:])
			nonce := base64.RawStdEncoding.EncodeToString(nonceBuf[:])

			// 'report-sample' asks supporting browsers to include the first
			// characters of the blocked code; 'report-sha256' asks for the
			// hash of every script that runs, for auditing.
			h.Set("Content-Security-Policy",
				"script-src 'self' 'nonce-"+nonce+"' 'report-sample' 'report-sha256'; "+
					"style-src 'self' 'nonce-"+nonce+"' https://fonts.googleapis.com 'report-sample'; "+
					staticDirectives)
			if PrivateArea(r.URL.Path) {
				h.Set("X-Robots-Tag", "noindex, nofollow")
			}

			ctx := WithNonce(r.Context(), nonce)
			ctx = templ.WithNonce(ctx, nonce)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// cspStaticDirectives holds the directives that do not change per request.
// Built once at startup to avoid string joins on every response.
func cspStaticDirectives(reportURL string) string {
	return strings.Join([]string{
		"default-src 'none'",
		"script-src-attr 'none'",
		"style-src-attr 'none'",
		// Remote images are restricted to self, data/blob, and explicit OpenStreetMap tiles.
		// Subdomain wildcards and bare 'https:' are disallowed to prevent image-based exfiltration.
		"img-src 'self' data: blob: https://a.tile.openstreetmap.org https://b.tile.openstreetmap.org https://c.tile.openstreetmap.org https://tile.openstreetmap.org",
		"media-src 'self' blob:",
		"font-src 'self' data: https://fonts.gstatic.com",
		// Same-origin XHR, fetch and the assistant's event streams, plus the
		// address lookup the branch map performs in the browser.
		"connect-src 'self' https://nominatim.openstreetmap.org",
		// Pin frame sources to exact map and embed providers without wildcard subdomains.
		"frame-src 'self' https://www.google.com https://maps.google.com https://www.openstreetmap.org",
		"worker-src 'self'",
		"manifest-src 'self'",
		"object-src 'none'",
		// No page uses <base>; an injected one would re-point every relative
		// script and form URL.
		"base-uri 'none'",
		"form-action 'self'",
		"frame-ancestors 'self'",
		"upgrade-insecure-requests",
		// Every DOM sink that can run script takes a Trusted Type. The only
		// policies allowed to mint one are the two in trusted-types.js.
		"require-trusted-types-for 'script'",
		"trusted-types dawa-sanitize default",
		"report-to csp",
		"report-uri " + reportURL,
	}, "; ")
}

// ReportHostOnly confines a host to the CSP report endpoint. The report
// collector's subdomain points at this same process; without this guard the
// whole site would also answer on that second origin, which has none of the
// site's cookies and would duplicate every public page.
func ReportHostOnly(host string) func(http.Handler) http.Handler {
	host = strings.ToLower(host)
	return func(next http.Handler) http.Handler {
		if host == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.EqualFold(stripPort(r.Host), host) {
				next.ServeHTTP(w, r)
				return
			}
			if r.URL.Path != CSPReportPath {
				http.NotFound(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func stripPort(hostport string) string {
	if strings.HasPrefix(hostport, "[") {
		if i := strings.Index(hostport, "]"); i > 0 {
			return hostport[1:i]
		}
		return hostport
	}
	if i := strings.LastIndexByte(hostport, ':'); i >= 0 {
		return hostport[:i]
	}
	return hostport
}

// privateAreas are the path prefixes that are never search results: dashboards,
// administration, account screens and the API.
var privateAreas = []string{"/admin", "/vendor", "/customer", "/account", "/api", "/moderator", "/settings"}

// PrivateArea reports whether a path lies in an area that must not be indexed.
// The OpenAPI description is the exception: it is published for agents.
func PrivateArea(path string) bool {
	if strings.HasPrefix(path, "/api/v1/openapi.") {
		return false
	}
	for _, prefix := range privateAreas {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
