package erp

import (
	"context"
	"errors"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

var errMatcherDown = errors.New("matcher down")

// Gateway-off discipline (trap 18): the matcher port has no enhancement
// method, so an ERP sync runs to completion with the Gateway down — items that
// cannot be matched deterministically land in the supplier's review queue.

var _ Matcher = gatewayOffMatcher{}

// gatewayOffMatcher has exactly the Prepare method. Compile-time proof that
// the Matcher port carries nothing else.
type gatewayOffMatcher struct{}

func (gatewayOffMatcher) Prepare(_ context.Context, _ productmatch.PrepareRequest) (productmatch.PrepareResult, error) {
	return productmatch.PrepareResult{}, nil
}

func TestSyncWithGatewayOff(t *testing.T) {
	f := newProcessFixture(t, matchflow.StateNeedsReview) // matcher "down" -> review
	items := make([]map[string]any, 0, 20)
	for i := 0; i < 20; i++ {
		items = append(items, intakeItem(offCode(i), "2.50"))
	}
	batch := f.accept(items...)
	if err := f.svc.ProcessChunk(f.tenantCtx, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	final, err := f.repo.GetBatchByID(testCtx, f.org, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.State != "completed" {
		t.Errorf("state = %q, want completed", final.State)
	}
	if final.NeedsReviewCount != 20 {
		t.Errorf("needs_review = %d, want 20", final.NeedsReviewCount)
	}
	pending, err := f.repo.ListSyncItems(testCtx, f.org, batch.ID, 0, "pending", 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("pending items = %d, want 0", len(pending))
	}
	if len(f.repo.reviewRows) != 20 {
		t.Errorf("review rows = %d, want 20", len(f.repo.reviewRows))
	}
}

func TestSyncMatcherErrorKeepsReview(t *testing.T) {
	f := newProcessFixture(t, matchflow.StateNeedsReview)
	f.matcher.err = errMatcherDown
	batch := f.accept(intakeItem("DOWN1", "2.50"), intakeItem("DOWN2", "2.50"))
	if err := f.svc.ProcessChunk(f.tenantCtx, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	final, _ := f.repo.GetBatchByID(testCtx, f.org, batch.ID)
	if final.State != "completed" {
		t.Errorf("state = %q, want completed (a matcher outage is not an outage)", final.State)
	}
	for _, code := range []string{"DOWN1", "DOWN2"} {
		row := f.item(code)
		if row.State != "needs_review" || row.Code == nil || *row.Code != "ERP_ITEM_MATCHER_UNAVAILABLE" {
			t.Errorf("%s = %+v, want needs_review ERP_ITEM_MATCHER_UNAVAILABLE", code, row)
		}
	}
}

func TestSyncNeverCallsEnhancement(t *testing.T) {
	// The Matcher interface must satisfy a struct that has only Prepare: the
	// compile-time half of the enhancement ban. The textual half lives in
	// seam_test.go's TestNeverCallsEnhancement.
	var _ Matcher = prepareOnlyMatcher{}
}

type prepareOnlyMatcher struct{}

func (prepareOnlyMatcher) Prepare(_ context.Context, _ productmatch.PrepareRequest) (productmatch.PrepareResult, error) {
	return productmatch.PrepareResult{}, nil
}

func offCode(i int) string { return "OFF" + itoa(int64(i)) }
