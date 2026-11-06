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

// shipmentDebitKey is the ledger idempotency key of one supplier shipment's debit.
// The partial unique index on the key means a shipment can be debited once.
func shipmentDebitKey(orderID, shipmentID int64) string {
	return fmt.Sprintf("order-shipment-debit:%d:%d", orderID, shipmentID)
}

// DebitOrderShipment debits a buyer's wallet for one supplier shipment of a
// wallet order. It runs inside the caller's transaction, which the commerce
// repository uses to confirm the shipment and record its charge. The wallet is
// locked and its balance checked here, so the debit and the confirmation commit
// together or not at all. A refused debit returns an error and writes nothing.
func (r *Repository) DebitOrderShipment(
	ctx context.Context,
	tx pgx.Tx,
	walletUserID, orderID, shipmentID int64,
	amount money.Amount,
	description string,
	actorUserID int64,
) error {
	if walletUserID <= 0 || orderID <= 0 || shipmentID <= 0 {
		return apperr.Validation("wallet.payer_required", "تعذر تحديد محفظة الصيدلية لسداد هذه الشحنة. The buyer's wallet for this shipment could not be identified.", nil)
	}
	if !amount.IsPositive() {
		return apperr.Internal(fmt.Errorf("billing: shipment %d debit must be positive, got %s", shipmentID, amount.String()))
	}
	walletID, err := ensureWalletTx(ctx, tx, walletUserID, "EGP")
	if err != nil {
		return err
	}
	negDelta, err := money.Zero.Sub(amount)
	if err != nil {
		return apperr.Internal(err)
	}
	key := shipmentDebitKey(orderID, shipmentID)
	orderRef := orderID
	if _, err := r.recordLedgerRow(ctx, tx, walletID, billing.TxWithdrawal, negDelta, "order", &orderRef, description, key); err != nil {
		return err
	}
	return database.WriteAudit(ctx, tx, database.AuditEntry{
		ActorUserID: actorUserID,
		Action:      "billing.wallet.order_shipment_debited",
		EntityType:  "order",
		EntityID:    strconv.FormatInt(orderID, 10),
		After: map[string]string{
			"shipment_id":     strconv.FormatInt(shipmentID, 10),
			"amount":          amount.String(),
			"idempotency_key": key,
		},
	})
}

// OrderNetDebit returns what a wallet order still has debited in the ledger: its
// debits, less admin reversals and earlier refunds. Callers hold the order lock.
func (r *Repository) OrderNetDebit(ctx context.Context, tx pgx.Tx, orderID int64) (money.Amount, error) {
	return orderRemaining(ctx, tx, orderID)
}

// ensureWalletTx returns the id of a user's wallet in the given currency, creating
// it when it does not exist yet. It works inside the caller's transaction.
func ensureWalletTx(ctx context.Context, tx pgx.Tx, userID int64, currency string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO billing.wallets (user_id, currency)
		VALUES ($1, $2)
		ON CONFLICT (user_id, currency) DO UPDATE SET updated_at = now()
		RETURNING id;`, userID, currency).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("billing postgres: ensure wallet: %w", err)
	}
	return id, nil
}
