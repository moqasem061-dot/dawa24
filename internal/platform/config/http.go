package config

import (
	"net/url"
	"strings"
	"time"
)

// The HTTP server's shape: ports, deadlines, and how much of the request the
// process is allowed to believe.
//
// Split from config.go, which was at the 400-line ceiling.

type HTTP struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	// RequestTimeout is the deadline put on every request's context. It must
	// stay below WriteTimeout so the application ends a slow request before the
	// socket does — see httpx.RequestTimeout for why that is the difference
	// between a rendered error and a 502.
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
	TrustedProxies  []string
	// TrustedProxyHops is how many reverse proxies sit in front of this
	// process. Every per-address defence depends on it: X-Forwarded-For is
	// written by the caller and appended to by each proxy, so the real client
	// is the entry this many places from the right. Set it wrong and either
	// every visitor shares one bucket (too high) or a scraper mints a fresh
	// identity per request (too low, or unset with a proxy in front).
	//
	// One is right for the current deployment: Elest.io's proxy and nothing
	// else. Zero is right when the process is exposed directly.
	TrustedProxyHops int
	// ModuleAPI mounts the per-module JSON APIs under /api/v1 (catalog,
	// inventory, commerce, billing, ingest, promo, workflow, hr, platform,
	// notifications, organizations). The web dashboard uses none of them — it
	// is server-rendered — and they were written for row-level security the
	// database does not enforce (the application connects as a superuser), so
	// several answered another organisation's records by id. Off in production
	// unless MODULE_API_ENABLED is set; the assistant, smart-order, attachment,
	// identity and integration APIs are mounted regardless.
	ModuleAPI bool
	// CSPReportURL is the absolute https URL browsers deliver
	// Content-Security-Policy violation reports to (CSP_REPORT_URL). It
	// belongs on another origin than APP_BASE_URL — a subdomain such as
	// https://csp.dawa24.com/api/v1/csp-report pointed at this same process,
	// which then answers nothing else on that host (httpx.ReportHostOnly).
	// Empty keeps reports on the site's own origin.
	CSPReportURL string
	// TLS configuration for direct HTTPS termination in the Go server
	// with Perfect Forward Secrecy and Post-Quantum Cryptography.
	TLSEnabled  bool
	TLSCertFile string
	TLSKeyFile  string
}

// CSPReportHost is the host of CSPReportURL when that is a different host
// from the site's, or "" when reports stay on the site's origin.
func (c *Config) CSPReportHost() string {
	if c.HTTP.CSPReportURL == "" {
		return ""
	}
	report, err := url.Parse(c.HTTP.CSPReportURL)
	if err != nil {
		return ""
	}
	if site, err := url.Parse(c.BaseURL); err == nil && strings.EqualFold(site.Hostname(), report.Hostname()) {
		return ""
	}
	return report.Hostname()
}

// SiteOrigin is the scheme://host[:port] of APP_BASE_URL.
func (c *Config) SiteOrigin() string {
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func validateHTTP(cfg *Config, fail func(string, ...any)) {
	raw := cfg.HTTP.CSPReportURL
	if raw != "" {
		u, err := url.Parse(raw)
		switch {
		case err != nil || !u.IsAbs() || u.Host == "":
			fail("CSP_REPORT_URL must be an absolute URL, got %q", raw)
		case u.Scheme != "https" && cfg.Env.IsProd():
			fail("CSP_REPORT_URL must use https in production")
		case strings.ContainsAny(raw, "\";, \t\r\n"):
			// The value is spliced into two response headers verbatim.
			fail("CSP_REPORT_URL must not contain quotes, commas, semicolons or whitespace")
		}
	}

	if cfg.HTTP.TLSEnabled || cfg.HTTP.TLSCertFile != "" || cfg.HTTP.TLSKeyFile != "" {
		if cfg.HTTP.TLSCertFile == "" || cfg.HTTP.TLSKeyFile == "" {
			fail("both TLS_CERT_FILE and TLS_KEY_FILE must be set when TLS is enabled")
		}
	}
}

func loadHTTP(env Env) HTTP {
	return HTTP{
		Port:             getInt("PORT", 8080),
		ReadTimeout:      getDuration("HTTP_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:     getDuration("HTTP_WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:      getDuration("HTTP_IDLE_TIMEOUT", 120*time.Second),
		RequestTimeout:   getDuration("HTTP_REQUEST_TIMEOUT", 25*time.Second),
		ShutdownTimeout:  getDuration("HTTP_SHUTDOWN_TIMEOUT", 20*time.Second),
		TrustedProxies:   getCSV("TRUSTED_PROXIES"),
		TrustedProxyHops: getInt("TRUSTED_PROXY_HOPS", 1),
		ModuleAPI:        getBool("MODULE_API_ENABLED", env != EnvProd),
		CSPReportURL:     getStr("CSP_REPORT_URL", ""),
		TLSEnabled:       getBool("TLS_ENABLED", false),
		TLSCertFile:      getStr("TLS_CERT_FILE", ""),
		TLSKeyFile:       getStr("TLS_KEY_FILE", ""),
	}
}
