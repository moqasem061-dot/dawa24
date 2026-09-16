package pages

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// The review screen must not print a product name beside a row the commit will
// silently drop. This pins the predicate it prints it by.
func TestReviewRowWillImport(t *testing.T) {
	id := int64(7)
	cases := []struct {
		name string
		row  *ingest.RowOutcome
		want bool
	}{
		{"settled strong match", &ingest.RowOutcome{
			ProductID: &id, MatchLevel: string(productmatch.MatchStrong)}, true},
		{"barcode match", &ingest.RowOutcome{
			ProductID: &id, MatchLevel: string(productmatch.MatchBarcode)}, true},
		{"needs review, suggested product", &ingest.RowOutcome{
			ProductID: &id, MatchLevel: string(productmatch.MatchReview)}, false},
		{"ambiguous, suggested product", &ingest.RowOutcome{
			ProductID: &id, MatchLevel: string(productmatch.MatchAmbiguous)}, false},
		{"needs review but confirmed by hand", &ingest.RowOutcome{
			ProductID: &id, MatchLevel: string(productmatch.MatchReview), IsManuallyMatched: true}, true},
		{"settled but excluded", &ingest.RowOutcome{
			ProductID: &id, MatchLevel: string(productmatch.MatchStrong), IsExcluded: true}, false},
		{"no product at all", &ingest.RowOutcome{
			MatchLevel: string(productmatch.MatchNone)}, false},
	}
	for _, c := range cases {
		if got := reviewRowWillImport(c.row); got != c.want {
			t.Errorf("%s: reviewRowWillImport = %v, want %v", c.name, got, c.want)
		}
	}
}

// The badge colours must describe the gate rather than a separate opinion about
// what a good score is.
func TestMatchScoreBadgeTracksThresholds(t *testing.T) {
	if tone := matchScoreBadgeTone(productmatch.DefaultMinStrong); tone != "badge-emerald" {
		t.Errorf("a score at the applied threshold rendered %q, want badge-emerald", tone)
	}
	if tone := matchScoreBadgeTone(productmatch.DefaultMinStrong - 0.01); tone != "badge-amber" {
		t.Errorf("a score just under the applied threshold rendered %q, want badge-amber", tone)
	}
	if tone := matchScoreBadgeTone(productmatch.DefaultMinReview - 0.01); tone != "badge-rose" {
		t.Errorf("a score under the review floor rendered %q, want badge-rose", tone)
	}
}
