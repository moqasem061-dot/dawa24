package corpus

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestReportedReviewRowsAgainstRealCatalogue(t *testing.T) {
	if !Available() {
		t.Skip("catalogue snapshot unavailable")
	}
	index, err := LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, price string
		id          int64
		state       matchflow.State
	}{
		// The first row omits the IU unit and misspells its brand. Keep it as a
		// reviewable suggestion, with its real score, rather than asserting certainty.
		{"ايفاسي5000حقن ابيكو", "500.00", 315115, matchflow.StateNeedsReview},
		{"ابيكساتراك 2.5مجم30ق", "366.00", 310107, matchflow.StateAccepted},
		{"ابيكوبريد 20 مجم بديل س", "104.00", 315099, matchflow.StateAccepted},
		{"ابيكوبريد5 مجم -- بديل س", "69.00", 315100, matchflow.StateAccepted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decision := index.Decide(&productmatch.Row{Name: tc.name, PublicPrice: money.MustParse(tc.price)}, productmatch.DefaultPolicy())
			if decision.State != tc.state || len(decision.Candidates) == 0 || decision.Candidates[0].ProductID != tc.id {
				t.Fatalf("expected product %d: %+v", tc.id, decision)
			}
			if tc.state == matchflow.StateAccepted && (decision.ProductID == nil || *decision.ProductID != tc.id) {
				t.Fatalf("accepted row did not link product %d: %+v", tc.id, decision)
			}
		})
	}
}
