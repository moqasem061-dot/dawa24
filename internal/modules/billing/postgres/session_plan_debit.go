package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// DebitSessionPlanInTx debits a payer's wallet for one session plan purchase. It
// runs inside the caller's transaction, which is the identity purchase that also
// raises the buyer's limit, so the debit and the limit commit together or not at
// all. The wallet is locked before its balance is checked.
//
// key is the ledger idempotency key of the purchase. When a row with that key
// already exists, nothing is written and replayed is true. A short balance is
// refused with a bilingual message, and nothing is written.
func (r *Repository) DebitSessionPlanInTx(
	ctx context.Context,
	tx pgx.Tx,
	payerUserID, buyerUserID, planID int64,
	amount money.Amount,
	key, description string,
) (bool, error) {
	if payerUserID <= 0 {
		return false, apperr.Validation("wallet.payer_required", "تعذر تحديد المحفظة المراد الخصم منها. The wallet to debit could not be identified.", nil)
	}
	if !amount.IsPositive() {
		return false, apperr.Internal(fmt.Errorf("billing: session plan %d charge must be positive, got %s", planID, amount.String()))
	}
	walletID, err := ensureWalletTx(ctx, tx, payerUserID, "EGP")
	if err != nil {
		return false, err
	}
	if err := lockWalletRow(ctx, tx, walletID); err != nil {
		return false, err
	}
	var seen bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM billing.wallet_transactions WHERE idempotency_key = $1);`, key).Scan(&seen); err != nil {
		return false, err
	}
	if seen {
		return true, nil
	}
	negDelta, err := money.Zero.Sub(amount)
	if err != nil {
		return false, apperr.Internal(err)
	}
	planRef := planID
	if _, err := r.recordLedgerRow(ctx, tx, walletID, billing.TxPurchase, negDelta, "session_plan", &planRef, description, key); err != nil {
		if e, ok := apperr.As(err); ok && e.Code == "wallet.insufficient_funds" {
			return false, apperr.Validation("session_plan.insufficient_funds",
				"رصيد المحفظة غير كافٍ لشراء هذه الخطة. Insufficient wallet balance for this session plan.", nil)
		}
		return false, err
	}
	return false, database.WriteAudit(ctx, tx, database.AuditEntry{
		ActorUserID: buyerUserID,
		Action:      "billing.wallet.session_plan_debited",
		EntityType:  "user",
		EntityID:    strconv.FormatInt(buyerUserID, 10),
		After: map[string]string{
			"plan_id":         strconv.FormatInt(planID, 10),
			"payer_user_id":   strconv.FormatInt(payerUserID, 10),
			"amount":          amount.String(),
			"idempotency_key": key,
		},
	})
}
