package identity

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// Batch 2 (S22): a session plan that is switched off cannot be applied to a user.
// The paid-plan behaviour is unchanged; see the batch 2 report.

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

func (r *sessionPlanRepo) SetMaxLoginSessions(_ context.Context, _ int64, max int) error {
	r.sets++
	r.max = max
	return nil
}

func TestPurchaseSessionPlanRefusesAnInactivePlan(t *testing.T) {
	repo := &sessionPlanRepo{mockRepo: &mockRepo{}, active: false}
	svc := NewService(repo, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if err := svc.PurchaseSessionPlan(context.Background(), 7, 3); err == nil {
		t.Fatal("an inactive session plan was applied")
	}
	if repo.sets != 0 {
		t.Fatalf("max login sessions were written %d times for an inactive plan, want 0", repo.sets)
	}
}

func TestPurchaseSessionPlanAppliesAnActivePlan(t *testing.T) {
	repo := &sessionPlanRepo{mockRepo: &mockRepo{}, active: true}
	svc := NewService(repo, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if err := svc.PurchaseSessionPlan(context.Background(), 7, 3); err != nil {
		t.Fatalf("active plan refused: %v", err)
	}
	if repo.sets != 1 || repo.max != 50 {
		t.Fatalf("max login sessions written %d times with %d, want once with 50", repo.sets, repo.max)
	}
}
