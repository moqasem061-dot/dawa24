package config

import (
	"strings"
)

// Turnstile configures Cloudflare Turnstile bot verification on sensitive forms (login, register).
type Turnstile struct {
	SiteKey   string
	SecretKey string
	Enabled   bool
}

func loadTurnstile(fail func(string, ...any)) Turnstile {
	siteKey := strings.TrimSpace(getStr("TURNSTILE_SITE_KEY", ""))
	secretKey := strings.TrimSpace(getStr("TURNSTILE_SECRET_KEY", ""))
	enabled := getBool("TURNSTILE_ENABLED", secretKey != "")

	t := Turnstile{
		SiteKey:   siteKey,
		SecretKey: secretKey,
		Enabled:   enabled,
	}

	if t.Enabled && t.SecretKey == "" {
		fail("TURNSTILE_ENABLED=true requires TURNSTILE_SECRET_KEY")
	}
	return t
}
