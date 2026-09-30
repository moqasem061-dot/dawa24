package matching_test

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
)

func TestLoaderStats_Defaults(t *testing.T) {
	stats := matching.LoaderStats{
		Rows:      100,
		Pages:     1,
		Truncated: false,
	}

	if stats.Rows != 100 || stats.Pages != 1 || stats.Truncated {
		t.Errorf("unexpected stats: %+v", stats)
	}
}

func TestLoader_Constants(t *testing.T) {
	if matching.DefaultPageSize != 5000 {
		t.Errorf("DefaultPageSize = %d, want 5000", matching.DefaultPageSize)
	}
	if matching.DefaultHardCeiling != 500000 {
		t.Errorf("DefaultHardCeiling = %d, want 500000", matching.DefaultHardCeiling)
	}
}
