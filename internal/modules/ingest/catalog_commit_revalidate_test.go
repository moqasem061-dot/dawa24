package ingest

import (
	"context"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

type mockCommitMatcher struct {
	revalidateFunc func(ctx context.Context, s productmatch.Scope, ref productmatch.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error)
}

func (m *mockCommitMatcher) Prepare(_ context.Context, _ productmatch.PrepareRequest) (productmatch.PrepareResult, error) {
	return productmatch.PrepareResult{}, nil
}

func (m *mockCommitMatcher) MarkReviewable(_ context.Context, _ int64) error {
	return nil
}

func (m *mockCommitMatcher) Rematch(_ context.Context, _ productmatch.Scope, _ productmatch.Row, _ int) (matchflow.RowDecision, error) {
	return matchflow.RowDecision{}, nil
}

func (m *mockCommitMatcher) Revalidate(ctx context.Context, s productmatch.Scope, ref productmatch.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
	if m.revalidateFunc != nil {
		return m.revalidateFunc(ctx, s, ref, d)
	}
	return d, nil
}

// TestCommitRevalidate tests that before writing stock/price, Matcher.Revalidate is called
// and rejects rows whose match_revision changed, state is not accepted/confirmed, or granularity is below pack.
func TestCommitRevalidate(t *testing.T) {
	ctx := context.Background()

	t.Run("revision changed is rejected", func(t *testing.T) {
		p1 := int64(101)
		row := stagedRow(1, &p1, "SKU-1", "Row 1", 5, true)
		row.MatchRevision = 1
		row.MatchState = string(matchflow.StateAccepted)
		row.MatchGranularity = string(matchflow.GranularityPack)

		keys := []VariantKey{{ID: 501, ProductID: p1, SKU: "SKU-1", Active: true}}
		st := DefaultSettings()
		st.WarehouseID = 1
		sess := session("imp-rev-changed", st)
		sess.Phase = PhaseReview

		svc, cat, _, _ := commitFixture(sess, []*RowOutcome{row}, keys, map[int64]bool{501: true})
		matcher := &mockCommitMatcher{
			revalidateFunc: func(_ context.Context, _ productmatch.Scope, _ productmatch.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
				// Simulate revision increment/mismatch
				d.Revision = 2
				return d, nil
			},
		}
		svc.SetMatcher(matcher)

		res, err := svc.CommitImport(ctx, sess.PublicID)
		if err != nil {
			t.Fatalf("CommitImport failed: %v", err)
		}
		if res.InsertedRows != 0 || res.UpdatedRows != 0 {
			t.Fatalf("expected 0 written rows, got inserted=%d updated=%d", res.InsertedRows, res.UpdatedRows)
		}
		if res.SkippedRows != 1 {
			t.Fatalf("expected 1 skipped row, got %d", res.SkippedRows)
		}
		if len(cat.writtenVariants) != 0 {
			t.Fatalf("expected 0 written variants in catalog, got %d", len(cat.writtenVariants))
		}
		if !strings.Contains(res.ErrorMessage, "تم استبعاد 1 صنف") {
			t.Errorf("expected commit notice to report revalidation rejection, got %q", res.ErrorMessage)
		}
	})

	t.Run("state not accepted or confirmed is rejected", func(t *testing.T) {
		p1 := int64(101)
		row := stagedRow(1, &p1, "SKU-1", "Row 1", 5, true)
		row.MatchRevision = 1
		row.MatchState = string(matchflow.StateNeedsReview)
		row.MatchGranularity = string(matchflow.GranularityPack)

		keys := []VariantKey{{ID: 501, ProductID: p1, SKU: "SKU-1", Active: true}}
		st2 := DefaultSettings()
		st2.WarehouseID = 1
		sess := session("imp-state-rejected", st2)
		sess.Phase = PhaseReview

		svc, cat, _, _ := commitFixture(sess, []*RowOutcome{row}, keys, map[int64]bool{501: true})
		matcher := &mockCommitMatcher{
			revalidateFunc: func(_ context.Context, _ productmatch.Scope, _ productmatch.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
				d.State = matchflow.StateNeedsReview
				return d, nil
			},
		}
		svc.SetMatcher(matcher)

		res, err := svc.CommitImport(ctx, sess.PublicID)
		if err != nil {
			t.Fatalf("CommitImport failed: %v", err)
		}
		if res.InsertedRows != 0 || res.UpdatedRows != 0 {
			t.Fatalf("expected 0 written rows, got inserted=%d updated=%d", res.InsertedRows, res.UpdatedRows)
		}
		if res.SkippedRows != 1 {
			t.Fatalf("expected 1 skipped row, got %d", res.SkippedRows)
		}
		if len(cat.writtenVariants) != 0 {
			t.Fatalf("expected 0 written variants in catalog, got %d", len(cat.writtenVariants))
		}
	})

	t.Run("granularity below pack is rejected", func(t *testing.T) {
		p1 := int64(101)
		row := stagedRow(1, &p1, "SKU-1", "Row 1", 5, true)
		row.MatchRevision = 1
		row.MatchState = string(matchflow.StateAccepted)
		row.MatchGranularity = string(matchflow.GranularityProduct)

		keys := []VariantKey{{ID: 501, ProductID: p1, SKU: "SKU-1", Active: true}}
		st3 := DefaultSettings()
		st3.WarehouseID = 1
		sess := session("imp-granularity-rejected", st3)
		sess.Phase = PhaseReview

		svc, cat, _, _ := commitFixture(sess, []*RowOutcome{row}, keys, map[int64]bool{501: true})
		matcher := &mockCommitMatcher{
			revalidateFunc: func(_ context.Context, _ productmatch.Scope, _ productmatch.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
				d.Granularity.Achieved = matchflow.GranularityProduct
				return d, nil
			},
		}
		svc.SetMatcher(matcher)

		res, err := svc.CommitImport(ctx, sess.PublicID)
		if err != nil {
			t.Fatalf("CommitImport failed: %v", err)
		}
		if res.InsertedRows != 0 || res.UpdatedRows != 0 {
			t.Fatalf("expected 0 written rows, got inserted=%d updated=%d", res.InsertedRows, res.UpdatedRows)
		}
		if res.SkippedRows != 1 {
			t.Fatalf("expected 1 skipped row, got %d", res.SkippedRows)
		}
		if len(cat.writtenVariants) != 0 {
			t.Fatalf("expected 0 written variants in catalog, got %d", len(cat.writtenVariants))
		}
	})

	t.Run("valid row with pack granularity is committed", func(t *testing.T) {
		p1 := int64(101)
		row := stagedRow(1, &p1, "SKU-1", "Row 1", 5, true)
		row.MatchRevision = 1
		row.MatchState = string(matchflow.StateAccepted)
		row.MatchGranularity = string(matchflow.GranularityPack)

		keys := []VariantKey{{ID: 501, ProductID: p1, SKU: "SKU-1", Active: true}}
		st4 := DefaultSettings()
		st4.WarehouseID = 1
		sess := session("imp-valid-committed", st4)
		sess.Phase = PhaseReview

		svc, cat, _, _ := commitFixture(sess, []*RowOutcome{row}, keys, map[int64]bool{501: true})
		matcher := &mockCommitMatcher{
			revalidateFunc: func(_ context.Context, _ productmatch.Scope, _ productmatch.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
				d.State = matchflow.StateAccepted
				d.Revision = 1
				d.Granularity.Achieved = matchflow.GranularityPack
				return d, nil
			},
		}
		svc.SetMatcher(matcher)

		res, err := svc.CommitImport(ctx, sess.PublicID)
		if err != nil {
			t.Fatalf("CommitImport failed: %v", err)
		}
		if res.UpdatedRows != 1 {
			t.Fatalf("expected 1 updated row, got %d", res.UpdatedRows)
		}
		if res.SkippedRows != 0 {
			t.Fatalf("expected 0 skipped rows, got %d", res.SkippedRows)
		}
		if len(cat.writtenVariants) != 1 {
			t.Fatalf("expected 1 written variant, got %d", len(cat.writtenVariants))
		}
	})
}
