package identity

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// Batch 2 (S22): a session plan that is switched off cannot be applied to a user.
// S22 (owner decision, 2026-11-06): a paid plan is charged through the wallet debiter, and the limit is
// raised only when the charge is made (see session_plan_purchase.go).

type sessionPlanRepo struct {
	*mockRepo
	active bool
	sets   int
	max    int
}

func (r *sessionPlanRepo) GetSessionPlanByID(_ context.Context, id int64) (*SessionPlan, error) {
	return &SessionPlan{
		ID:               id,
		MaxLoginSessions: 50,
		Price:            money.MustParse("9999.00"),
		IsActive:         r.active,
	}, nil
}

// PurchaseSessionPlan stands in for the transaction: it reads the plan, runs the
// decision, and writes the limit only when the decision did not replay.
func (r *sessionPlanRepo) PurchaseSessionPlan(ctx context.Context, _, planID int64, decide SessionPlanDecider) error {
	plan, err := r.GetSessionPlanByID(ctx, planID)
	if err != nil {
		return err
	}
	replayed, err := decide(ctx, nil, plan)
	if err != nil || replayed {
		return err
	}
	r.sets++
	r.max = plan.MaxLoginSessions
	return nil
}

func TestPurchaseSessionPlanRefusesAnInactivePlan(t *testing.T) {
	repo := &sessionPlanRepo{mockRepo: &mockRepo{}, active: false}
	svc := NewService(repo, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if err := svc.PurchaseSessionPlan(context.Background(), SessionPlanPurchase{
		BuyerUserID: 7, PayerUserID: 7, PlanID: 3, Token: "tok-inactive-1", PayerAuthorized: true,
	}); err == nil {
		t.Fatal("an inactive session plan was applied")
	}
	if repo.sets != 0 {
		t.Fatalf("max login sessions were written %d times for an inactive plan, want 0", repo.sets)
	}
}

func TestPurchaseSessionPlanAppliesAnActivePlan(t *testing.T) {
	repo := &sessionPlanRepo{mockRepo: &mockRepo{}, active: true}
	svc := NewService(repo, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	debits := 0
	svc.SetSessionPlanDebiter(func(_ context.Context, _ pgx.Tx, c SessionPlanCharge) (bool, error) {
		debits++
		if c.Amount.String() != "9999.00" {
			t.Errorf("charged %s, want the stored price 9999.00", c.Amount.String())
		}
		return false, nil
	})

	if err := svc.PurchaseSessionPlan(context.Background(), SessionPlanPurchase{
		BuyerUserID: 7, PayerUserID: 7, PlanID: 3, Token: "tok-active-1", PayerAuthorized: true,
	}); err != nil {
		t.Fatalf("active plan refused: %v", err)
	}
	if debits != 1 {
		t.Fatalf("wallet debits = %d, want 1", debits)
	}
	if repo.sets != 1 || repo.max != 50 {
		t.Fatalf("max login sessions written %d times with %d, want once with 50", repo.sets, repo.max)
	}
}
