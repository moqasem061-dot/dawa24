package timeutil

import (
	"testing"
	"time"
)

func TestLocation(t *testing.T) {
	if Location == nil {
		t.Fatal("expected Location to not be nil")
	}
	if Location.String() != "Africa/Cairo" && Location.String() != "EET" {
		t.Errorf("unexpected location name: %s", Location.String())
	}

	now := Now()
	if now.Location().String() != Location.String() {
		t.Errorf("expected now location to be %s, got %s", Location.String(), now.Location().String())
	}

	utcTime := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	cairoTime := InCairo(utcTime)
	// Cairo is UTC+3 in summer (August)
	if cairoTime.Hour() != 15 {
		t.Errorf("expected 15:00 for August UTC+3 in Cairo, got %d", cairoTime.Hour())
	}

	// Zero time handling
	var zero time.Time
	if !InCairo(zero).IsZero() {
		t.Error("expected zero time to remain zero")
	}
}
