package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// chargeShipment charges one supplier's shipment of a wallet order to the buyer's
// wallet, when that supplier confirms it (Batch 2, S02). It runs in the caller's
// transaction, which holds the order row lock. The debit and the per-shipment
// record are written together with the confirmation: if the debit is refused the
// caller returns the error and the shipment is not confirmed.
//
// A shipment is charged once: wallet_paid_at is set in the same transaction and
// a shipment that already has it is skipped. An order already marked paid is left
// alone, which keeps orders paid before this rule as they were.
func (r *Repository) chargeShipment(
	ctx context.Context,
	tx pgx.Tx,
	order *commerce.Order,
	shipment *commerce.OrderShipment,
	walletUserID, actorUserID int64,
) error {
	if !commerce.IsWalletPayment(order.PaymentMethod) || shipment.WalletPaidAt != nil {
		return nil
	}
	var paymentStatus string
	if err := tx.QueryRow(ctx, `SELECT payment_status FROM commerce.orders WHERE id = $1;`, order.ID).Scan(&paymentStatus); err != nil {
		return err
	}
	if paymentStatus == string(commerce.PaymentPaid) {
		return nil
	}
	goods, err := shipment.WalletGoodsAmount()
	if err != nil {
		return apperr.Internal(err)
	}
	if goods.IsPositive() {
		if walletUserID <= 0 {
			return apperr.Conflict("wallet.payer_required",
				"تعذر تحديد محفظة الصيدلية لسداد هذه الشحنة. The buyer's wallet for this shipment could not be identified.")
		}
		if r.payer == nil {
			return apperr.Internal(fmt.Errorf("commerce: wallet payments are not configured for order %d", order.ID))
		}
		desc := fmt.Sprintf("سداد قيمة مشتريات الشحنة رقم %d من الطلب رقم %s من المحفظة المؤسسية", shipment.ID, orderNumberOrID(order))
		if err := r.payer.DebitOrderShipment(ctx, tx, walletUserID, order.ID, shipment.ID, goods, desc, actorUserID); err != nil {
			return err
		}
	}
	res, err := tx.Exec(ctx, `
		UPDATE commerce.order_shipments
		SET wallet_paid_amount = $2, wallet_paid_at = now(), updated_at = now()
		WHERE id = $1 AND wallet_paid_at IS NULL;`, shipment.ID, goods)
	if err != nil {
		return err
	}
	if res.RowsAffected() != 1 {
		return apperr.Conflict("shipment.already_paid", "قيمة هذه الشحنة مسددة من المحفظة بالفعل. This shipment has already been paid from the wallet.")
	}
	now := time.Now().UTC()
	shipment.WalletPaidAmount = goods
	shipment.WalletPaidAt = &now
	return settleWalletPayment(ctx, tx, order.ID)
}

// settleWalletPayment sets a wallet order's payment status from its per-shipment
// records. An order with no shipment charge (a legacy order) is left as it is.
// Otherwise the status is refunded or partially refunded once credits have been
// given back, and paid or partially paid while shipments are still to be charged.
func settleWalletPayment(ctx context.Context, tx pgx.Tx, orderID int64) error {
	var charged, refunded money.Amount
	var chargedCount, unpaidLive int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(wallet_paid_amount) FILTER (WHERE wallet_paid_at IS NOT NULL), 0.00),
		       COALESCE(SUM(wallet_refunded_amount), 0.00),
		       COUNT(*) FILTER (WHERE wallet_paid_at IS NOT NULL),
		       COUNT(*) FILTER (WHERE wallet_paid_at IS NULL
		                          AND status NOT IN ('cancelled', 'failed', 'returned', 'refunded'))
		FROM commerce.order_shipments
		WHERE order_id = $1;`, orderID).Scan(&charged, &refunded, &chargedCount, &unpaidLive); err != nil {
		return err
	}
	if chargedCount == 0 {
		return nil
	}
	var status commerce.PaymentStatus
	switch {
	case refunded.IsPositive() && refunded.Minor() >= charged.Minor():
		status = commerce.PaymentRefunded
	case refunded.IsPositive():
		status = commerce.PaymentPartiallyRefunded
	case unpaidLive == 0:
		status = commerce.PaymentPaid
	default:
		status = commerce.PaymentPartiallyPaid
	}
	_, err := tx.Exec(ctx, `UPDATE commerce.orders SET payment_status = $2, updated_at = now() WHERE id = $1;`, orderID, string(status))
	return err
}

// findOrderShipment returns the shipment with the given id from a locked order.
func findOrderShipment(order *commerce.Order, id int64) *commerce.OrderShipment {
	for _, s := range order.Shipments {
		if s != nil && s.ID == id {
			return s
		}
	}
	return nil
}
