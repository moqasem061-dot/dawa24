package identity

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// purchaseTokenPattern is the shape of the token a paid purchase must carry. A
// repeat of a token is the same purchase. No page renders this form yet, so a
// future purchase form must send a new token each time it is rendered.
var purchaseTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// SessionPlanPurchase is one request to apply a session plan. It never carries a
// price: the stored plan supplies the price inside the transaction.
type SessionPlanPurchase struct {
	// BuyerUserID is the account whose sign-in limit is raised.
	BuyerUserID int64
	// PayerUserID is whose wallet pays. The caller resolves it the way wallet
	// subscriptions do: the organisation's owner for a company buyer, otherwise
	// the buyer.
	PayerUserID int64
	PlanID      int64
	// Token identifies one submit of the form. A paid purchase writes its ledger
	// row under a key that contains it, so a repeat of the same token is never
	// charged twice.
	Token string
	// PayerAuthorized is true when the buyer may spend the payer's wallet. The
	// caller decides it from the buyer's permissions.
	PayerAuthorized bool
}

// SessionPlanCharge is the wallet debit a paid plan needs.
type SessionPlanCharge struct {
	PayerUserID int64
	BuyerUserID int64
	PlanID      int64
	Amount      money.Amount
	// Key is the ledger idempotency key of this purchase.
	Key         string
	Description string
}

// SessionPlanDebiter debits the payer's wallet inside the caller's transaction. It
// locks the wallet, refuses a short balance and writes nothing when it refuses.
// replayed is true when a ledger row with the same key already exists: nothing is
// debited, and the plan must not be applied a second time. The billing module
// implements it and cmd/server wires it, because identity must not import billing.
type SessionPlanDebiter func(ctx context.Context, tx pgx.Tx, c SessionPlanCharge) (replayed bool, err error)

// SessionPlanDecider is called by the repository inside the purchase transaction,
// with the plan row locked. It returns replayed=true when the purchase was already
// charged, in which case the buyer's limit is left alone.
type SessionPlanDecider func(ctx context.Context, tx pgx.Tx, plan *SessionPlan) (replayed bool, err error)

// SetSessionPlanDebiter installs the wallet debit. Without one a paid plan is
// refused, so a paid plan is never applied for free by mistake.
func (s *Service) SetSessionPlanDebiter(fn SessionPlanDebiter) { s.planDebiter = fn }

// PurchaseSessionPlan applies a plan to the buyer. A paid plan is charged to the
// payer's wallet and the buyer's limit is raised in one transaction: both happen,
// or neither does.
func (s *Service) PurchaseSessionPlan(ctx context.Context, p SessionPlanPurchase) error {
	return s.repo.PurchaseSessionPlan(ctx, p.BuyerUserID, p.PlanID, func(txCtx context.Context, tx pgx.Tx, plan *SessionPlan) (bool, error) {
		return s.decideSessionPlan(txCtx, tx, p, plan)
	})
}

// decideSessionPlan works out what a purchase costs and charges it. A free plan
// has no debit row. A paid plan needs the buyer to be allowed to spend the payer's
// wallet and a token for the charge key.
func (s *Service) decideSessionPlan(ctx context.Context, tx pgx.Tx, p SessionPlanPurchase, plan *SessionPlan) (bool, error) {
	if !plan.IsActive {
		return false, apperr.Validation("session_plan.inactive",
			"هذه الخطة غير متاحة حالياً. This session plan is not available.", nil)
	}
	price := plan.Price
	if plan.IsFree {
		price = money.Zero
	}
	if price.IsNegative() {
		return false, apperr.Internal(fmt.Errorf("identity: session plan %d has a negative price", plan.ID))
	}
	if price.IsZero() {
		return false, nil
	}
	if !p.PayerAuthorized {
		return false, apperr.Forbidden("session_plan.payer_not_permitted",
			"لا تملك صلاحية الدفع من محفظة الشركة. You do not have permission to pay for this from the company wallet.")
	}
	if !purchaseTokenPattern.MatchString(p.Token) {
		return false, apperr.Validation("session_plan.token_required",
			"تعذر إتمام الشراء. أعد تحميل الصفحة وحاول مرة أخرى. The purchase could not be completed. Reload the page and try again.", nil)
	}
	if s.planDebiter == nil {
		return false, apperr.Internal(errors.New("identity: no wallet debiter is wired for paid session plans"))
	}
	name := plan.Name.Get("ar")
	if name == "" {
		name = plan.Name.Get("en")
	}
	if name == "" {
		name = fmt.Sprintf("#%d", plan.ID)
	}
	return s.planDebiter(ctx, tx, SessionPlanCharge{
		PayerUserID: p.PayerUserID,
		BuyerUserID: p.BuyerUserID,
		PlanID:      plan.ID,
		Amount:      price,
		Key:         fmt.Sprintf("session-plan:%d:%d:%d:%s", p.PayerUserID, p.BuyerUserID, plan.ID, p.Token),
		Description: fmt.Sprintf("خطة جلسات متزامنة: %s. Concurrent session plan: %s", name, name),
	})
}
