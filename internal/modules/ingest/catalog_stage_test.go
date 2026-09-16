package ingest

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

func TestCatalogImportSettingsMinScore(t *testing.T) {
	s := DefaultSettings()
	if s.MinMatchScore != productmatch.DefaultMinStrong {
		t.Fatalf("DefaultSettings().MinMatchScore = %v, want the shared default %v",
			s.MinMatchScore, productmatch.DefaultMinStrong)
	}
	// The shared default is the unified control every import tool starts on.
	if productmatch.DefaultMinStrong != 0.60 {
		t.Fatalf("shared default is %v, want 0.60", productmatch.DefaultMinStrong)
	}

	// A vendor's own threshold is preserved as long as it is at or above the
	// shared review floor.
	custom := Settings{MinMatchScore: 0.45}.Normalize()
	if custom.MinMatchScore != 0.45 {
		t.Fatalf("Normalize() changed 0.45 to %v", custom.MinMatchScore)
	}

	// Below the review floor it is lifted to it: a threshold under the point
	// where the engine stops believing its own answer imports the review queue.
	tooLow := Settings{MinMatchScore: 0.05}.Normalize()
	if tooLow.MinMatchScore != 0.25 {
		t.Fatalf("Normalize() on 0.05 gave %v, want the 0.25 review floor", tooLow.MinMatchScore)
	}

	// Test zero defaults to the shared default
	zeroSetting := Settings{}.Normalize()
	if zeroSetting.MinMatchScore != productmatch.DefaultMinStrong {
		t.Fatalf("Normalize() on zero score gave %v, want %v",
			zeroSetting.MinMatchScore, productmatch.DefaultMinStrong)
	}
}

func TestPhaseReviewState(t *testing.T) {
	if !PhaseReview.Open() {
		t.Fatal("PhaseReview.Open() should be true")
	}
	if PhaseReview.Terminal() {
		t.Fatal("PhaseReview.Terminal() should be false")
	}
	if PhaseReview.Label() == "" {
		t.Fatal("PhaseReview.Label() should have Arabic description")
	}
}

func TestRowOutcomeEffectiveVariantName(t *testing.T) {
	row := &RowOutcome{
		DisplayName:       "Panadol Extra 500mg",
		CustomVariantName: "بنادول اكسترا احمر - صيدليتي",
	}
	if got := row.EffectiveVariantName(); got != "بنادول اكسترا احمر - صيدليتي" {
		t.Fatalf("EffectiveVariantName() = %q, want custom name", got)
	}

	rowNoCustom := &RowOutcome{
		DisplayName: "Panadol Extra 500mg",
	}
	if got := rowNoCustom.EffectiveVariantName(); got != "Panadol Extra 500mg" {
		t.Fatalf("EffectiveVariantName() = %q, want original display name", got)
	}
}
