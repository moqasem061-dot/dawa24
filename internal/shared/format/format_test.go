package format

import (
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestMoney(t *testing.T) {
	tests := []struct {
		amount   money.Amount
		lang     string
		expected string
	}{
		{money.FromMinor(0), "ar", "0.00 ج.م"},
		{money.FromMinor(500), "ar", "5.00 ج.م"},
		{money.FromMinor(123450), "ar", "1,234.50 ج.م"},
		{money.FromMinor(123456789), "ar", "1,234,567.89 ج.م"},
		{money.FromMinor(123456789), "en", "1,234,567.89 EGP"},
		{money.FromMinor(-4500), "ar", "-45.00 ج.م"},
	}

	for _, tt := range tests {
		actual := Money(tt.amount, tt.lang)
		if actual != tt.expected {
			t.Errorf("Money(%v, %q) = %q, expected %q", tt.amount, tt.lang, actual, tt.expected)
		}
	}
}

func TestInteger(t *testing.T) {
	tests := []struct {
		n        int64
		expected string
	}{
		{0, "0"},
		{5, "5"},
		{45, "45"},
		{450, "450"},
		{1000, "1,000"},
		{12345, "12,345"},
		{1234567, "1,234,567"},
		{-9876543, "-9,876,543"},
	}

	for _, tt := range tests {
		actual := Integer(tt.n, "ar")
		if actual != tt.expected {
			t.Errorf("Integer(%d) = %q, expected %q", tt.n, actual, tt.expected)
		}
	}
}

func TestDistance(t *testing.T) {
	tests := []struct {
		metres   int
		lang     string
		expected string
	}{
		{450, "ar", "450 م"},
		{450, "en", "450 m"},
		{1000, "ar", "1.0 كم"},
		{3200, "ar", "3.2 كم"},
		{3200, "en", "3.2 km"},
	}

	for _, tt := range tests {
		actual := Distance(tt.metres, tt.lang)
		if actual != tt.expected {
			t.Errorf("Distance(%d, %q) = %q, expected %q", tt.metres, tt.lang, actual, tt.expected)
		}
	}
}

func TestDate(t *testing.T) {
	dt := time.Date(2026, time.August, 17, 14, 30, 0, 0, time.UTC)
	if actual := Date(dt, "ar"); actual != "17 أغسطس 2026" {
		t.Errorf("Date ar = %q, expected 17 أغسطس 2026", actual)
	}
	if actual := Date(dt, "en"); actual != "Aug 17, 2026" {
		t.Errorf("Date en = %q, expected Aug 17, 2026", actual)
	}
}
