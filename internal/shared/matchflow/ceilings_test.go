package matchflow_test

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

func TestProfileSavingCeilings(t *testing.T) {
	c := matchflow.For(matchflow.ProfileSaving)
	if c.MaxItemsPerRequest != 80 {
		t.Errorf("expected MaxItemsPerRequest 80, got %d", c.MaxItemsPerRequest)
	}
	if c.MaxConcurrent != 6 {
		t.Errorf("expected MaxConcurrent 6, got %d", c.MaxConcurrent)
	}
	if c.MaxRequestsPerRun != 60 {
		t.Errorf("expected MaxRequestsPerRun 60, got %d", c.MaxRequestsPerRun)
	}
}

func TestAdaptiveCeilings(t *testing.T) {
	t.Run("small batch clamps max items", func(t *testing.T) {
		c := matchflow.Adaptive(matchflow.ProfileSaving, 50)
		if c.MaxItemsPerRequest != 40 {
			t.Errorf("expected MaxItemsPerRequest 40 for small file, got %d", c.MaxItemsPerRequest)
		}
	})

	t.Run("medium batch scales up small profile", func(t *testing.T) {
		c := matchflow.Adaptive(matchflow.ProfileOrder, 500)
		if c.MaxItemsPerRequest != 75 {
			t.Errorf("expected MaxItemsPerRequest 75 for medium file, got %d", c.MaxItemsPerRequest)
		}
	})

	t.Run("large batch scales up high throughput", func(t *testing.T) {
		c := matchflow.Adaptive(matchflow.ProfileSaving, 2500)
		if c.MaxItemsPerRequest != 100 {
			t.Errorf("expected MaxItemsPerRequest 100 for large file, got %d", c.MaxItemsPerRequest)
		}
		if c.MaxRequestsPerRun != 60 {
			t.Errorf("expected MaxRequestsPerRun 60 for large file, got %d", c.MaxRequestsPerRun)
		}
	})
}
