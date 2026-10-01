package smartorder

import (
	"context"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

type mockMatchRevalidator struct {
	revalidateFunc func(ctx context.Context, s productmatch.Scope, ref productmatch.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error)
}

func (m *mockMatchRevalidator) Revalidate(ctx context.Context, s productmatch.Scope, ref productmatch.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
	if m.revalidateFunc != nil {
		return m.revalidateFunc(ctx, s, ref, d)
	}
	return d, nil
}

func TestFinalize_Revalidation_Success(t *testing.T) {
	repo := oneLineSetup()
	repo.lines[0].MatchState = "accepted"
	repo.lines[0].MatchRevision = 1
	repo.lines[0].MatchGranularity = "pack"

	placer := &stubPlacer{orderID: 501}
	reval := &mockMatchRevalidator{
		revalidateFunc: func(_ context.Context, _ productmatch.Scope, _ productmatch.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
			return d, nil
		},
	}

	fin := NewFinalizer(repo, placer, stubRecheck{ok: true})
	fin.SetRevalidator(reval)

	orderID, stale, err := fin.Finalize(context.Background(), readyRun())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("expected 0 stale lines, got %d: %+v", len(stale), stale)
	}
	if orderID != 501 {
		t.Fatalf("expected orderID 501, got %d", orderID)
	}
}

func TestFinalize_Revalidation_CatalogChanged_ReturnsToReview(t *testing.T) {
	repo := oneLineSetup()
	repo.lines[0].MatchState = "accepted"
	repo.lines[0].MatchRevision = 1
	repo.lines[0].MatchGranularity = "pack"

	placer := &stubPlacer{orderID: 501}
	reval := &mockMatchRevalidator{
		revalidateFunc: func(_ context.Context, _ productmatch.Scope, _ productmatch.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
			d.Flags = append(d.Flags, "catalog_changed")
			return d, nil
		},
	}

	fin := NewFinalizer(repo, placer, stubRecheck{ok: true})
	fin.SetRevalidator(reval)

	orderID, stale, err := fin.Finalize(context.Background(), readyRun())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if orderID != 0 {
		t.Fatalf("order should not be placed when revalidation detects catalog change")
	}
	if len(stale) != 1 {
		t.Fatalf("expected 1 stale line, got %d", len(stale))
	}
	if stale[0].LineID != repo.lines[0].ID {
		t.Errorf("expected line ID %d, got %d", repo.lines[0].ID, stale[0].LineID)
	}

	// Line must be returned to review (marked as unmatched so buyer reviews it)
	if repo.lines[0].Outcome != OutcomeUnmatched {
		t.Errorf("expected line OutcomeUnmatched, got %s", repo.lines[0].Outcome)
	}
	if len(repo.updatedLines) == 0 || repo.updatedLines[0].Outcome != OutcomeUnmatched {
		t.Errorf("expected updatedLines to persist OutcomeUnmatched")
	}
}

func TestFinalize_Revalidation_StateNotAccepted_ReturnsToReview(t *testing.T) {
	repo := oneLineSetup()
	repo.lines[0].MatchState = "accepted"
	repo.lines[0].MatchRevision = 1

	placer := &stubPlacer{orderID: 501}
	reval := &mockMatchRevalidator{
		revalidateFunc: func(_ context.Context, _ productmatch.Scope, _ productmatch.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
			d.State = matchflow.StateNeedsReview
			return d, nil
		},
	}

	fin := NewFinalizer(repo, placer, stubRecheck{ok: true})
	fin.SetRevalidator(reval)

	orderID, stale, err := fin.Finalize(context.Background(), readyRun())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if orderID != 0 {
		t.Fatalf("order should not be placed when match is not accepted")
	}
	if len(stale) != 1 {
		t.Fatalf("expected 1 stale line, got %d", len(stale))
	}
	if repo.lines[0].Outcome != OutcomeUnmatched {
		t.Errorf("expected line OutcomeUnmatched, got %s", repo.lines[0].Outcome)
	}
}

func TestFinalize_Recheck_StockStale_ReturnsToReview(t *testing.T) {
	repo := oneLineSetup()
	placer := &stubPlacer{orderID: 501}

	fin := NewFinalizer(repo, placer, stubRecheck{ok: false, reason: ReasonStock})

	orderID, stale, err := fin.Finalize(context.Background(), readyRun())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if orderID != 0 {
		t.Fatalf("order should not be placed when stock is stale")
	}
	if len(stale) != 1 {
		t.Fatalf("expected 1 stale line, got %d", len(stale))
	}
	if stale[0].Reason != ReasonStock {
		t.Errorf("expected ReasonStock, got %s", stale[0].Reason)
	}
	if repo.lines[0].Outcome != OutcomeOutOfStock {
		t.Errorf("expected line OutcomeOutOfStock, got %s", repo.lines[0].Outcome)
	}
	if len(repo.updatedLines) == 0 || repo.updatedLines[0].Outcome != OutcomeOutOfStock {
		t.Errorf("expected repo.UpdateLines to be called with OutcomeOutOfStock")
	}
}
