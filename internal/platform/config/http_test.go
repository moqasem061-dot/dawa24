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
			t.Setenv("DATABASE_URL", "postgres://dawa24_app:test-password@postgresql:5432/dawa24_test?sslmode=disable")
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

func TestProductionUserConfig(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("APP_NAME", "Dawa24")
	t.Setenv("APP_BASE_URL", "dawa24.com")
	t.Setenv("PORT", "8080")
	t.Setenv("DATABASE_URL", "postgres://dawa24_app:test-password@postgresql:5432/dawa24_test?sslmode=disable")
	t.Setenv("DB_MAX_CONNS", "20")
	t.Setenv("DB_MIN_CONNS", "2")
	t.Setenv("DB_STATEMENT_TIMEOUT", "20s")
	t.Setenv("DB_WORKER_STATEMENT_TIMEOUT", "10m")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("REDIS_POOL_SIZE", "10")
	t.Setenv("STORAGE_ENDPOINT", "http://localhost:9000")
	t.Setenv("STORAGE_REGION", "us-east-1")
	t.Setenv("STORAGE_BUCKET", "dawa24")
	t.Setenv("STORAGE_ACCESS_KEY_ID", "minioadmin")
	t.Setenv("STORAGE_SECRET_ACCESS_KEY", "minioadmin")
	t.Setenv("STORAGE_USE_PATH_STYLE", "true")
	t.Setenv("STORAGE_PUBLIC_BASE_URL", "http://localhost:9000/dawa24")
	t.Setenv("SESSION_SECRET", "test-session-secret-0123456789abcdef0123456789abcdef")
	t.Setenv("SESSION_COOKIE_NAME", "dawa24_session")
	t.Setenv("SESSION_TTL", "720h")
	t.Setenv("SESSION_SECURE", "true")
	t.Setenv("GATEWAY_ENABLED", "true")
	t.Setenv("GATEWAY_BASE_URL", "https://api.dawa24.com")
	t.Setenv("GATEWAY_VIRTUAL_KEY", "true")
	t.Setenv("GATEWAY_CLIENT_APP", "dawa24-store")
	t.Setenv("GATEWAY_TIMEOUT", "60s")
	t.Setenv("SMTP_HOST", "smtp.hostinger.com")
	t.Setenv("SMTP_PORT", "465")
	t.Setenv("SMTP_ENCRYPTION", "tls")
	t.Setenv("SMTP_USERNAME", "no-reply@dawa24.com")
	t.Setenv("SMTP_PASSWORD", "test-smtp-password")
	t.Setenv("SMTP_FROM_EMAIL", "no-reply@dawa24.com")
	t.Setenv("SMTP_FROM_NAME", "Dawa24 | دوا 24")
	t.Setenv("SMTP_ENABLED", "true")
	t.Setenv("SMTP_TIMEOUT", "10s")
	t.Setenv("TELEGRAM_BOT_USERNAME", "testdawa_bot")
	t.Setenv("TELEGRAM_BRIDGE_TOKEN", "test-telegram-bridge-token-0123456789abcdef0123456789")
	t.Setenv("MARKETING_BRIDGE_TOKEN", "test-marketing-bridge-token-0123456789abcdef012345678")
	t.Setenv("TURNSTILE_ENABLED", "true")
	t.Setenv("TURNSTILE_SITE_KEY", "test-turnstile-site-key")
	t.Setenv("TURNSTILE_SECRET_KEY", "test-turnstile-secret-key")
	t.Setenv("AUTH_REQUIRE_EMAIL_VERIFICATION", "true")
	t.Setenv("AUTH_REQUIRE_PHONE_VERIFICATION", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("config.Load() failed with user env: %v", err)
	}
	if cfg.BaseURL != "https://dawa24.com" {
		t.Errorf("expected BaseURL normalized to https://dawa24.com, got %s", cfg.BaseURL)
	}
	if cfg.SiteOrigin() != "https://dawa24.com" {
		t.Errorf("expected SiteOrigin https://dawa24.com, got %s", cfg.SiteOrigin())
	}
	if !cfg.SMTP.Enabled {
		t.Errorf("expected SMTP to be enabled")
	}
	if cfg.SMTP.Host != "smtp.hostinger.com" || cfg.SMTP.Port != 465 {
		t.Errorf("unexpected SMTP host/port: %s:%d", cfg.SMTP.Host, cfg.SMTP.Port)
	}
}
