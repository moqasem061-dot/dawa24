package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// Idempotency keys for wallet credits made by commerce. A cancellation event
// gets one key, so it credits at most once even if it were run twice.
func orderCancelKey(orderID int64) string {
	return fmt.Sprintf("order-cancel:%d", orderID)
}

func orderRefundKey(orderID int64) string {
	return fmt.Sprintf("order-refund:%d", orderID)
}

func shipmentCancelKey(orderID, shipmentID int64) string {
	return fmt.Sprintf("shipment-cancel:%d:%d", orderID, shipmentID)
}

// refundWalletDebits returns to the buyer's wallet what the order was debited,
// inside tx, which must hold the order lock. Orders paid another way have no
// debit and return at once. A wallet order needs a refunder; without one the
// refund fails and the whole transaction rolls back. A capped refund (a
// supplier's partial cancel) never exceeds capAmount. When money is credited,
// the order's payment status becomes refunded, as the website always did.
func (r *Repository) refundWalletDebits(
	ctx context.Context,
	tx pgx.Tx,
	order *commerce.Order,
	capAmount *money.Amount,
	key string,
	actorUserID int64,
	reason string,
) (money.Amount, error) {
	if !commerce.IsWalletPayment(order.PaymentMethod) {
		return money.Zero, nil
	}
	if r.refunder == nil {
		return money.Zero, apperr.Internal(fmt.Errorf("commerce: wallet refunds are not configured for order %d", order.ID))
	}
	desc := fmt.Sprintf("استرداد قيمة مشتريات للطلب الملغي رقم %s إلى المحفظة المؤسسية", orderNumberOrID(order))
	if reason != "" {
		desc += " (" + reason + ")"
	}
	credited, err := r.refunder.CreditOrderRefund(ctx, tx, order.ID, capAmount, key, desc, actorUserID)
	if err != nil {
		return money.Zero, err
	}
	if credited.IsPositive() {
		if _, err := tx.Exec(ctx, `UPDATE commerce.orders SET payment_status = 'refunded', updated_at = now() WHERE id = $1;`, order.ID); err != nil {
			return money.Zero, err
		}
		order.PaymentStatus = commerce.PaymentRefunded
	}
	return credited, nil
}

func orderNumberOrID(order *commerce.Order) string {
	if order.OrderNumber != "" {
		return order.OrderNumber
	}
	return fmt.Sprintf("ORD-%d", order.ID)
}

// ReturnWalletDebits is the compensation path: it credits back a debit that
// was made just before a failed confirmation. With a shipment, the credit is
// capped at that shipment's goods. The debit is capped by the ledger, so it
// cannot exceed what was charged.
func (r *Repository) ReturnWalletDebits(ctx context.Context, orderID int64, shipmentID *int64, actorUserID int64, reason string) (money.Amount, error) {
	var credited money.Amount
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		order, err := lockAPICancellationOrder(txCtx, tx, orderID)
		if err != nil {
			return err
		}
		var capAmount *money.Amount
		if shipmentID != nil {
			for _, s := range order.Shipments {
				if s.ID != *shipmentID {
					continue
				}
				goods, gErr := s.WalletGoodsAmount()
				if gErr != nil {
					return apperr.Internal(gErr)
				}
				capAmount = &goods
			}
		}
		credited, err = r.refundWalletDebits(txCtx, tx, order, capAmount, "", actorUserID, reason)
		return err
	})
	return credited, err
}

func actorOrZero(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}
