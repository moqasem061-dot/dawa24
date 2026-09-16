package workflow

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type dummyInstGate struct {
	works []int64
}

func (d *dummyInstGate) AllowedWorkIDs(_ context.Context, _ int64, _ int) ([]int64, error) {
	return d.works, nil
}

func TestProcessPriorityEngineFullPipeline(t *testing.T) {
	repo := newMockWorkflowRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)
	svc.SetInstitutionalGate(&dummyInstGate{works: []int64{101, 102}})
	ctx := context.Background()

	budget := money.MustParse("500.00")
	prefs := Priorities{
		PriorityHighestDiscount:        true,
		PriorityLowestPrice:            true,
		PriorityFastestDelivery:        true,
		PriorityPreferredSuppliersOnly: true,
		BudgetConstraint:               &budget,
		PreferredSupplierIDs:           []int64{1},
	}

	orgID := int64(10)
	created, err := svc.CreatePriorityEngine(ctx, 100, &orgID, prefs)
	if err != nil {
		t.Fatalf("CreatePriorityEngine failed: %v", err)
	}

	if created.RequestNumber == "" {
		t.Errorf("expected request number to be set")
	}
	if created.Status != "pending" {
		t.Errorf("expected status 'pending', got %s", created.Status)
	}

	responderID := int64(999)
	summary, err := svc.ProcessPriorityEngine(ctx, created.ID, &responderID)
	if err != nil {
		t.Fatalf("ProcessPriorityEngine failed: %v", err)
	}

	if summary == nil {
		t.Fatal("expected summary, got nil")
	}
	if summary.TotalProductsAnalyzed != 1 {
		t.Errorf("expected 1 product analyzed, got %d", summary.TotalProductsAnalyzed)
	}
	if summary.RecommendationsGenerated != 1 {
		t.Errorf("expected 1 recommendation generated, got %d", summary.RecommendationsGenerated)
	}

	// Verify engine was marked completed
	completedReq, err := repo.GetPriorityRequestByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetPriorityRequestByID failed: %v", err)
	}
	if completedReq.Status != "completed" {
		t.Errorf("expected completed status, got %s", completedReq.Status)
	}

	// List engines for user
	list, err := svc.ListPriorityEngines(ctx, 100, 10, 0)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListPriorityEngines failed: %v", err)
	}
}
