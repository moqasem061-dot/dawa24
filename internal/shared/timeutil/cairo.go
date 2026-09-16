// Package timeutil manages the platform's standard timezone (Africa/Cairo)
// and ensures consistent local time handling across PostgreSQL queries,
// background workers, and UI formatters.
package timeutil

import (
	"time"
	_ "time/tzdata" // Embed IANA timezone database so Africa/Cairo is always available
)

// Location represents the standard Egypt timezone (Africa/Cairo, UTC+2 / UTC+3 DST).
var Location = func() *time.Location {
	loc, err := time.LoadLocation("Africa/Cairo")
	if err != nil {
		// Fallback to Eastern European Time (UTC+2) if lookup fails
		return time.FixedZone("EET", 2*60*60)
	}
	return loc
}()

func init() {
	// Set time.Local so standard library functions like time.Now() and
	// pgx TIMESTAMPTZ decoders default to Egypt local time.
	time.Local = Location
}

// Now returns the current time in Africa/Cairo timezone.
func Now() time.Time {
	return time.Now().In(Location)
}

// InCairo converts a time.Time to the Africa/Cairo timezone.
// Zero-value times are returned unchanged.
func InCairo(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.In(Location)
}
