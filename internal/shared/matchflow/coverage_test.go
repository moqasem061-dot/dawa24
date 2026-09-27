package matchflow_test

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

// Every row with any retrieved candidate is put to the model, in every tool:
// no plausibility floor drops the unmatched and needs-review rows.
func TestNoProfileDropsWeakRowsBeforeTheModel(t *testing.T) {
	for _, p := range profiles {
		if got := matchflow.For(p).MinPlausible; got != 0 {
			t.Errorf("%s: MinPlausible %.2f, want 0 so every row is asked", p, got)
		}
	}
}
