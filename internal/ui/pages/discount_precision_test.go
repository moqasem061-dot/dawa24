package pages

import "testing"

func TestDiscountPrecisionSurvivesDisplayAndFilters(t *testing.T) {
	for _, tc := range []struct {
		value float64
		want  string
	}{
		{0.25, "0.25"}, {12.34, "12.34"}, {20, "20"}, {100, "100"},
	} {
		if got := formatBenchPercent(tc.value); got != tc.want {
			t.Fatalf("display = %s, want %s", got, tc.want)
		}
		if got := formatOptionalFloat(&tc.value); got != tc.want {
			t.Fatalf("filter = %s, want %s", got, tc.want)
		}
	}
}
