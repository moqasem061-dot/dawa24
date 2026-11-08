package erp

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

// The supplier's review screen: open items page, resolve links the chosen
// product through the same write path and stores the human_confirm knowledge,
// dismiss closes without linking, and another organization's item id is
// indistinguishable from a missing one (US1.3).

func reviewFixture(t *testing.T) (*processFixture, int64) {
	t.Helper()
	f := newProcessFixture(t, matchflow.StateNeedsReview) // everything lands in review
	batch := f.accept(intakeItem("REV10", "5.00"), intakeItem("REV11", "6.00"))
	if err := f.svc.ProcessChunk(f.tenantCtx, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	items, err := f.repo.ListReview(testCtx, f.org, Page{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("open items = %d, want 2", len(items))
	}
	return f, items[0].ID
}

func TestReviewListPagesOpenItems(t *testing.T) {
	f, firstID := reviewFixture(t)
	page1, err := f.svc.ListReview(database.WithTenant(testCtx, f.org), f.org, Page{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page1) != 1 {
		t.Fatalf("page 1 = %d items, want 1", len(page1))
	}
	page2, err := f.svc.ListReview(database.WithTenant(testCtx, f.org), f.org, Page{After: firstID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 1 {
		t.Fatalf("page 2 = %d items, want 1", len(page2))
	}
	if page1[0].ID == page2[0].ID {
		t.Error("the same item appeared on both pages")
	}
}

func TestReviewResolveLinksAndRemembers(t *testing.T) {
	f, id := reviewFixture(t)
	writesBefore := len(f.writer.written)
	if err := f.svc.ResolveReview(testCtx, 7, id, 777, 9); err != nil {
		t.Fatal(err)
	}
	// The resolution links, stores the knowledge, marks resolved…
	if len(f.repo.linkWrites) == 0 {
		t.Error("no link write on resolve")
	}
	if len(f.memory.kinds) == 0 || f.memory.kinds[len(f.memory.kinds)-1] != "human_confirm" {
		t.Errorf("Remember kinds = %v, want human_confirm last", f.memory.kinds)
	}
	review, err := f.repo.GetReview(testCtx, 7, id)
	if err != nil {
		t.Fatal(err)
	}
	if review.State != "resolved" || review.ResolvedProductID == nil || *review.ResolvedProductID != 777 {
		t.Errorf("review = %+v", review)
	}
	if len(f.repo.fakeSync.audits) == 0 || f.repo.fakeSync.audits[len(f.repo.fakeSync.audits)-1].Action == "" {
		t.Error("resolve wrote no audit row")
	}
	if writesBefore == len(f.writer.written) {
		t.Error("resolve did not create the variant through the writer")
	}
	// Resolving again is not found: it is no longer open.
	if err := f.svc.ResolveReview(testCtx, 7, id, 777, 9); err == nil {
		t.Error("resolved item accepted a second resolve")
	}
}

func TestReviewDismiss(t *testing.T) {
	f, id := reviewFixture(t)
	if err := f.svc.DismissReview(testCtx, 7, id, 9); err != nil {
		t.Fatal(err)
	}
	review, err := f.repo.GetReview(testCtx, 7, id)
	if err != nil {
		t.Fatal(err)
	}
	if review.State != "dismissed" {
		t.Errorf("state = %q, want dismissed", review.State)
	}
	if len(f.memory.kinds) != 0 {
		t.Errorf("dismiss must not Remember, got %v", f.memory.kinds)
	}
}

func TestReviewForeignItemIsNotFound(t *testing.T) {
	f, id := reviewFixture(t)
	if err := f.svc.ResolveReview(testCtx, 8, id, 777, 9); err == nil {
		t.Error("another organization resolved the item")
	}
	if err := f.svc.DismissReview(testCtx, 8, id, 9); err == nil {
		t.Error("another organization dismissed the item")
	}
	items, _ := f.svc.ListReview(database.WithTenant(testCtx, 8), 8, Page{Limit: 10})
	if len(items) != 0 {
		t.Errorf("org 8 sees %d review items", len(items))
	}
}
