// Package mask hides sensitive values on screen. The functions are pure: they
// never reveal more than the last three characters of a value, and the clear
// value is never kept beside the masked one. The shared package imports nothing
// from this repository.
package mask

import "strings"

const (
	mask = "*"
	// keep is how many trailing characters stay visible.
	keep = 3
)

// Number masks all but the last three characters. Length is counted in runes, so
// Arabic-Indic digits are masked the same way as Latin ones.
func Number(s string) string {
	return maskKeepLast([]rune(s))
}

// Phone masks a phone number with the same rule as Number.
func Phone(s string) string {
	return Number(s)
}

// Email masks the local part of an address and keeps the domain, so a reader
// can tell which organization the address belongs to. A value without an "@"
// is masked as a number.
func Email(s string) string {
	at := strings.LastIndex(s, "@")
	if at <= 0 {
		return Number(s)
	}
	return maskKeepLast([]rune(s[:at])) + s[at:]
}

func maskKeepLast(r []rune) string {
	if len(r) == 0 {
		return ""
	}
	if len(r) <= keep {
		return strings.Repeat(mask, len(r))
	}
	return strings.Repeat(mask, len(r)-keep) + string(r[len(r)-keep:])
}
