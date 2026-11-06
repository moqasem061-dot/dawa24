package identity

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// purchaseRepo stands in for the purchase transaction: it runs the decision on one
// plan and counts the limit writes that the transaction would make.
type purchaseRepo struct {
	*mockRepo
	plan *SessionPlan
	sets int
}

func (r *purchaseRepo) GetSessionPlanByID(_ context.Context, id int64) (*SessionPlan, error) {
	p := *r.plan
	p.ID = id
	return &p, nil
}

func (r *purchaseRepo) PurchaseSessionPlan(ctx context.Context, _, planID int64, decide SessionPlanDecider) error {
	plan, err := r.GetSessionPlanByID(ctx, planID)
	if err != nil {
		return err
	}
	replayed, err := decide(ctx, nil, plan)
	if err != nil || replayed {
		return err
	}
	r.sets++
	return nil
}

// purchaseSetup builds a service over one plan. debit is wired only when non-nil.
func purchaseSetup(plan *SessionPlan, debit SessionPlanDebiter) (*Service, *purchaseRepo) {
	repo := &purchaseRepo{mockRepo: &mockRepo{}, plan: plan}
	svc := NewService(repo, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if debit != nil {
		svc.SetSessionPlanDebiter(debit)
	}
	return svc, repo
}

func activePlan(price string, isFree bool) *SessionPlan {
	return &SessionPlan{MaxLoginSessions: 5, Price: money.MustParse(price), IsFree: isFree, IsActive: true}
}

func validPaidPurchase() SessionPlanPurchase {
	return SessionPlanPurchase{BuyerUserID: 7, PayerUserID: 9, PlanID: 3, Token: "tok-unit-0001", PayerAuthorized: true}
}

// errCode returns the apperr code of err, or "" when err is not an apperr.
func errCode(err error) string {
	if e, ok := apperr.As(err); ok {
		return e.Code
	}
	return ""
}

func TestSessionPlanFreeHasNoDebitAndNeedsNoToken(t *testing.T) {
	debits := 0
	svc, repo := purchaseSetup(activePlan("250.50", true), func(context.Context, pgx.Tx, SessionPlanCharge) (bool, error) {
		debits++
		return false, nil
	})
	if err := svc.PurchaseSessionPlan(context.Background(), SessionPlanPurchase{BuyerUserID: 7, PayerUserID: 9, PlanID: 3}); err != nil {
		t.Fatalf("free plan refused: %v", err)
	}
	if debits != 0 {
		t.Fatalf("free plan debited %d times, want 0", debits)
	}
	if repo.sets != 1 {
		t.Fatalf("free plan limit written %d times, want 1", repo.sets)
	}
}

func TestSessionPlanPaidRefusedWithoutPayerPermission(t *testing.T) {
	debits := 0
	svc, repo := purchaseSetup(activePlan("250.50", false), func(context.Context, pgx.Tx, SessionPlanCharge) (bool, error) {
		debits++
		return false, nil
	})
	p := validPaidPurchase()
	p.PayerAuthorized = false
	if code := errCode(svc.PurchaseSessionPlan(context.Background(), p)); code != "session_plan.payer_not_permitted" {
		t.Fatalf("error code = %q, want session_plan.payer_not_permitted", code)
	}
	if debits != 0 || repo.sets != 0 {
		t.Fatalf("debits = %d, limit writes = %d; want nothing written", debits, repo.sets)
	}
}

func TestSessionPlanPaidRefusedWithoutAValidToken(t *testing.T) {
	for _, token := range []string{"", "short", "has space 1234", "bad/token/1234"} {
		debits := 0
		svc, repo := purchaseSetup(activePlan("250.50", false), func(context.Context, pgx.Tx, SessionPlanCharge) (bool, error) {
			debits++
			return false, nil
		})
		p := validPaidPurchase()
		p.Token = token
		if code := errCode(svc.PurchaseSessionPlan(context.Background(), p)); code != "session_plan.token_required" {
			t.Fatalf("token %q: error code = %q, want session_plan.token_required", token, code)
		}
		if debits != 0 || repo.sets != 0 {
			t.Fatalf("token %q: debits = %d, limit writes = %d; want nothing written", token, debits, repo.sets)
		}
	}
}

func TestSessionPlanPaidRefusedWithoutAWalletDebiter(t *testing.T) {
	svc, repo := purchaseSetup(activePlan("250.50", false), nil)
	if err := svc.PurchaseSessionPlan(context.Background(), validPaidPurchase()); err == nil {
		t.Fatal("a paid plan was applied with no wallet debiter wired")
	}
	if repo.sets != 0 {
		t.Fatalf("limit written %d times without a charge, want 0", repo.sets)
	}
}

func TestSessionPlanChargeIsTheStoredPriceUnderTheStableKey(t *testing.T) {
	var got SessionPlanCharge
	svc, repo := purchaseSetup(activePlan("250.50", false), func(_ context.Context, _ pgx.Tx, c SessionPlanCharge) (bool, error) {
		got = c
		return false, nil
	})
	if err := svc.PurchaseSessionPlan(context.Background(), validPaidPurchase()); err != nil {
		t.Fatalf("paid plan refused: %v", err)
	}
	if got.Amount.String() != "250.50" || got.PayerUserID != 9 || got.BuyerUserID != 7 || got.PlanID != 3 {
		t.Fatalf("charge = %+v, want 250.50 from payer 9 for buyer 7, plan 3", got)
	}
	if want := "session-plan:9:7:3:tok-unit-0001"; got.Key != want {
		t.Fatalf("charge key = %q, want %q", got.Key, want)
	}
	if repo.sets != 1 {
		t.Fatalf("limit written %d times, want 1", repo.sets)
	}
}

func TestSessionPlanReplayedChargeLeavesTheLimitAlone(t *testing.T) {
	debits := 0
	svc, repo := purchaseSetup(activePlan("250.50", false), func(context.Context, pgx.Tx, SessionPlanCharge) (bool, error) {
		debits++
		return true, nil
	})
	if err := svc.PurchaseSessionPlan(context.Background(), validPaidPurchase()); err != nil {
		t.Fatalf("replayed purchase refused: %v", err)
	}
	if debits != 1 {
		t.Fatalf("debiter called %d times, want 1", debits)
	}
	if repo.sets != 0 {
		t.Fatalf("limit written %d times on a replay, want 0", repo.sets)
	}
}

func TestSessionPlanNegativePriceIsRefused(t *testing.T) {
	svc, repo := purchaseSetup(activePlan("-1.00", false), func(context.Context, pgx.Tx, SessionPlanCharge) (bool, error) {
		t.Fatal("a negative price reached the wallet")
		return false, nil
	})
	if err := svc.PurchaseSessionPlan(context.Background(), validPaidPurchase()); err == nil {
		t.Fatal("a negative price was accepted")
	}
	if repo.sets != 0 {
		t.Fatalf("limit written %d times for a negative price, want 0", repo.sets)
	}
}
