package httpx_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/stretchr/testify/assert"
)

func TestCSPReportHandler_ValidReport(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := httpx.CSPReportHandler(log)

	payload := `{
		"csp-report": {
			"document-uri": "https://example.com/checkout",
			"referrer": "",
			"violated-directive": "script-src-elem",
			"effective-directive": "script-src-elem",
			"original-policy": "default-src 'none'; script-src 'self'",
			"disposition": "enforce",
			"blocked-uri": "https://evil.com/xss.js",
			"line-number": 42,
			"source-file": "https://example.com/checkout",
			"status-code": 200,
			"script-sample": ""
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/csp-report", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/csp-report")
	rec := httptest.NewRecorder()

	handler(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestCSPReportHandler_MethodNotAllowed(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := httpx.CSPReportHandler(log)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/csp-report", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestCSPReportHandler_PayloadTooLarge(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := httpx.CSPReportHandler(log)

	hugePayload := `{"csp-report":{"document-uri":"` + strings.Repeat("A", 35*1024) + `"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/csp-report", bytes.NewBufferString(hugePayload))
	rec := httptest.NewRecorder()

	handler(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSecurityHeaders_CSPHardenedDirectives(t *testing.T) {
	csp, res := serveSecurityHeaders(t, httpx.SecurityOptions{})

	assert.Equal(t, `csp="/api/v1/csp-report", default="/api/v1/csp-report"`, res.Header.Get("Reporting-Endpoints"))
	assert.Empty(t, res.Header.Get("Report-To"))
	assert.Equal(t, "none", res.Header.Get("X-Permitted-Cross-Domain-Policies"))
	assert.Equal(t, "?1", res.Header.Get("Origin-Agent-Cluster"))

	// Reporting directives
	assert.Contains(t, csp, "report-to csp")
	assert.Contains(t, csp, "report-uri /api/v1/csp-report")

	// Script hash auditing & report samples
	assert.Contains(t, csp, "'report-sample'")
	assert.Contains(t, csp, "'report-sha256'")

	// Deprecated child-src is gone
	assert.NotContains(t, csp, "child-src")

	// img-src allows no bare https: scheme and no wildcard subdomains
	assert.NotContains(t, csp, " https:;")
	assert.NotContains(t, csp, " https: ")
	assert.NotContains(t, csp, "https://*.tile")
	assert.Contains(t, csp, "img-src 'self' data: blob: https://a.tile.openstreetmap.org")

	// frame-src uses no subdomain wildcards
	assert.NotContains(t, csp, "https://*.google.com")
	assert.Contains(t, csp, "frame-src 'self' https://www.google.com https://maps.google.com https://www.openstreetmap.org")
}

// The scanner findings this policy was hardened against: nothing may relax
// script or style execution, and Trusted Types must be enforced.
func TestSecurityHeaders_NoUnsafeSources(t *testing.T) {
	csp, _ := serveSecurityHeaders(t, httpx.SecurityOptions{})

	assert.NotContains(t, csp, "'unsafe-eval'")
	assert.NotContains(t, csp, "'unsafe-inline'")
	assert.NotContains(t, csp, "'unsafe-hashes'")
	assert.NotContains(t, csp, "'wasm-unsafe-eval'")

	directives := map[string]string{}
	for _, d := range strings.Split(csp, ";") {
		name, value, _ := strings.Cut(strings.TrimSpace(d), " ")
		directives[name] = value
	}
	assert.Equal(t, "'none'", directives["script-src-attr"])
	assert.Equal(t, "'none'", directives["style-src-attr"])
	assert.Equal(t, "'none'", directives["default-src"])
	assert.Equal(t, "'none'", directives["object-src"])
	assert.Equal(t, "'none'", directives["base-uri"])
	assert.Equal(t, "'script'", directives["require-trusted-types-for"])
	assert.Equal(t, "dawa-sanitize default", directives["trusted-types"])
	assert.Regexp(t, `^'self' 'nonce-[A-Za-z0-9+/]{22}' `, directives["script-src"])
	assert.Regexp(t, `^'self' 'nonce-[A-Za-z0-9+/]{22}' `, directives["style-src"])
}

func TestSecurityHeaders_NonceIsFreshAndShared(t *testing.T) {
	var seen []string
	handler := httpx.SecurityHeaders(httpx.SecurityOptions{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := httpx.Nonce(r.Context())
		assert.Equal(t, nonce, templ.GetNonce(r.Context()), "templ-emitted elements must carry the same nonce")
		seen = append(seen, nonce)
	}))
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "'nonce-"+seen[i]+"'")
	}
	assert.NotEqual(t, seen[0], seen[1])
}

func TestSecurityHeaders_ReportsGoToConfiguredOrigin(t *testing.T) {
	const url = "https://csp.dawa24.com/api/v1/csp-report"
	csp, res := serveSecurityHeaders(t, httpx.SecurityOptions{ReportURL: url})

	assert.Contains(t, csp, "report-uri "+url)
	assert.NotContains(t, csp, "report-uri /")
	assert.Equal(t, `csp="`+url+`", default="`+url+`"`, res.Header.Get("Reporting-Endpoints"))
}

func serveSecurityHeaders(t *testing.T, opts httpx.SecurityOptions) (string, *http.Response) {
	t.Helper()
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	rec := httptest.NewRecorder()
	httpx.SecurityHeaders(opts)(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/dashboard", nil))
	res := rec.Result()
	t.Cleanup(func() { _ = res.Body.Close() })
	csp := res.Header.Get("Content-Security-Policy")
	assert.NotEmpty(t, csp)
	return csp, res
}

func TestCSPReportHandler_ReportingAPIBatch(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	payload := `[
		{"type":"csp-violation","url":"https://dawa24.com/cart","body":{
			"documentURL":"https://dawa24.com/cart","blockedURL":"inline",
			"effectiveDirective":"script-src-elem","disposition":"enforce",
			"sample":"alert(1)","lineNumber":7}},
		{"type":"deprecation","url":"https://dawa24.com/","body":{"id":"X","message":"old api"}}
	]`
	req := httptest.NewRequest(http.MethodPost, httpx.CSPReportPath, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/reports+json")
	rec := httptest.NewRecorder()

	httpx.CSPReportHandler(log)(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	out := logs.String()
	assert.Contains(t, out, "content security policy violation reported")
	assert.Contains(t, out, "effective_directive=script-src-elem")
	assert.Contains(t, out, "sample=alert(1)")
	assert.Contains(t, out, "browser report received")
	assert.Equal(t, 1, strings.Count(out, "content security policy violation reported"))
}

func TestCSPReportHandler_CORSPreflight(t *testing.T) {
	handler := httpx.CSPReportHandler(nil, "https://dawa24.com")

	req := httptest.NewRequest(http.MethodOptions, httpx.CSPReportPath, nil)
	req.Header.Set("Origin", "https://dawa24.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	handler(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "https://dawa24.com", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "POST", rec.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type", rec.Header().Get("Access-Control-Allow-Headers"))

	// Another origin gets no grant.
	req = httptest.NewRequest(http.MethodOptions, httpx.CSPReportPath, nil)
	req.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	handler(rec, req)
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestReportHostOnly(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	handler := httpx.ReportHostOnly("csp.dawa24.com")(inner)

	cases := []struct {
		host, path string
		want       int
	}{
		{"csp.dawa24.com", httpx.CSPReportPath, http.StatusTeapot},
		{"CSP.dawa24.com:443", httpx.CSPReportPath, http.StatusTeapot},
		{"csp.dawa24.com", "/", http.StatusNotFound},
		{"csp.dawa24.com", "/admin", http.StatusNotFound},
		{"dawa24.com", "/", http.StatusTeapot},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, c.path, nil)
		req.Host = c.host
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, c.want, rec.Code, "%s%s", c.host, c.path)
	}

	// No report host configured: a passthrough.
	rec := httptest.NewRecorder()
	httpx.ReportHostOnly("")(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusTeapot, rec.Code)
}
