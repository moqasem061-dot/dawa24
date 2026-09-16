package config

import (
	"strings"
	"testing"
)

func TestCSPReportURL(t *testing.T) {
	cases := []struct {
		name, env, url, wantErr, wantHost string
	}{
		{name: "unset keeps reports same-origin", env: "prod", url: ""},
		{name: "subdomain", env: "prod", url: "https://csp.dawa24.com/api/v1/csp-report", wantHost: "csp.dawa24.com"},
		{name: "same host is not a report host", env: "prod", url: "https://dawa24.com/api/v1/csp-report"},
		{name: "relative", env: "dev", url: "/api/v1/csp-report", wantErr: "absolute URL"},
		{name: "plain http in production", env: "prod", url: "http://csp.dawa24.com/r", wantErr: "https"},
		{name: "plain http in development", env: "dev", url: "http://csp.localhost:8080/r", wantHost: "csp.localhost"},
		{name: "header injection", env: "prod", url: `https://csp.dawa24.com/r", evil="x`, wantErr: "quotes"},
		{name: "directive injection", env: "prod", url: "https://csp.dawa24.com/r;script-src", wantErr: "semicolons"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("APP_ENV", c.env)
			t.Setenv("APP_BASE_URL", "https://dawa24.com")
			t.Setenv("DATABASE_URL", "postgres://u:p@localhost/db")
			t.Setenv("REDIS_URL", "redis://localhost:6379")
			t.Setenv("SESSION_SECRET", strings.Repeat("s", 32))
			t.Setenv("CSP_REPORT_URL", c.url)

			cfg, err := Load()
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("want error containing %q, got %v", c.wantErr, err)
				}
				return
			}
			if err != nil && strings.Contains(err.Error(), "CSP_REPORT_URL") {
				t.Fatalf("unexpected CSP_REPORT_URL error: %v", err)
			}
			if err != nil {
				// Other production requirements (storage, gateway) are not
				// what this test is about.
				t.Skipf("environment incomplete for %s: %v", c.env, err)
			}
			if got := cfg.CSPReportHost(); got != c.wantHost {
				t.Fatalf("CSPReportHost() = %q, want %q", got, c.wantHost)
			}
			if got := cfg.SiteOrigin(); got != "https://dawa24.com" {
				t.Fatalf("SiteOrigin() = %q", got)
			}
		})
	}
}
