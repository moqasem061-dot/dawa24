package matchflow_test

import (
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

var profiles = []matchflow.Profile{
	matchflow.ProfileOrder, matchflow.ProfileVendor,
	matchflow.ProfileSaving, matchflow.ProfileCatalog,
}

// Every tool's AI stage finishes inside fifteen minutes. The engine enforces
// fourteen regardless; a profile asking for more is a profile that has
// forgotten the budget.
func TestEveryProfileFitsTheFifteenMinuteBudget(t *testing.T) {
	for _, p := range profiles {
		c := matchflow.For(p)
		if c.MaxWallClock <= 0 || c.MaxWallClock > 14*time.Minute {
			t.Errorf("%s: wall clock %s, want (0, 14m]", p, c.MaxWallClock)
		}
	}
}

// A batch is sized by attention. No profile sends more than sixty lines in one
// request, and no file size makes a batch larger than its profile's.
func TestBatchesNeverGrowWithTheFile(t *testing.T) {
	for _, p := range profiles {
		base := matchflow.For(p)
		if base.MaxItemsPerRequest > 60 {
			t.Errorf("%s: %d items per request, want at most 60", p, base.MaxItemsPerRequest)
		}
		for _, n := range []int{1, 50, 500, 5_000, 50_000} {
			c := matchflow.Adaptive(p, n)
			if c.MaxItemsPerRequest > base.MaxItemsPerRequest {
				t.Errorf("%s at %d rows: %d items per request, above the profile's %d",
					p, n, c.MaxItemsPerRequest, base.MaxItemsPerRequest)
			}
		}
	}
}

// A small file is answered in small batches, which come back sooner.
func TestSmallFilesGetSmallBatches(t *testing.T) {
	for _, p := range profiles {
		if got := matchflow.Adaptive(p, 50).MaxItemsPerRequest; got > 20 {
			t.Errorf("%s: %d items per request for a 50-row file, want at most 20", p, got)
		}
	}
}

// The master catalogue is where a wrong match cannot be seen afterwards, so it
// applies the least.
func TestTheCatalogueImportIsTheStrictest(t *testing.T) {
	cat := matchflow.For(matchflow.ProfileCatalog).MinApplyConfidence
	for _, p := range profiles {
		if c := matchflow.For(p).MinApplyConfidence; c > cat {
			t.Errorf("%s applies at %.2f, stricter than the catalogue's %.2f", p, c, cat)
		}
	}
}
