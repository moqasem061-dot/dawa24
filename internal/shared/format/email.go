package format

import "strings"

// NormalizeEmail is the one canonical form of an email address: trimmed and
// lower-cased. Accounts are looked up by it and OTPs are keyed on it, so every
// caller must use this rather than restating it.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
