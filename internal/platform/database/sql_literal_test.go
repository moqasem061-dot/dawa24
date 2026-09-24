package database

import "testing"

func TestQuoteLiteralDoublesSingleQuotes(t *testing.T) {
	for in, want := range map[string]string{
		"plain":         "'plain'",
		"o'brien":       "'o''brien'",
		"'; DROP x; --": "'''; DROP x; --'",
		"":              "''",
	} {
		if got := QuoteLiteral(in); got != want {
			t.Errorf("QuoteLiteral(%q) = %s, want %s", in, got, want)
		}
	}
}
