package config

import (
	"net/url"
	"strings"
)

// DatabasePassword extracts the password from a PostgreSQL DSN.
//
// It exists so the Gateway settings screen can be told the one secret it must
// never accept: the password this process connects to its own database with.
// A malformed or password-less DSN yields "", which disables that check rather
// than blocking configuration.
func DatabasePassword(dsn string) string {
	trimmed := strings.TrimSpace(dsn)
	if trimmed == "" {
		return ""
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.User == nil {
		return ""
	}
	pass, ok := u.User.Password()
	if !ok {
		return ""
	}
	return pass
}
