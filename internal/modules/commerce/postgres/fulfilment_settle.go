package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// syncParentOrderStatus moves the parent order's status to match its shipments
// after a shipment changes to status to. It runs inside the caller's transaction.
func syncParentOrderStatus(txCtx context.Context, tx pgx.Tx, to commerce.OrderStatus, history commerce.OrderStatusHistory) {
	// Synchronize parent order status if appropriate
	switch to {
	case commerce.StatusDelivered:
		var nonDeliveredCount int
		_ = database.Soft(txCtx, tx, func() error {
			return tx.QueryRow(txCtx, `SELECT COUNT(*) FROM commerce.order_shipments WHERE order_id = $1 AND status != 'delivered';`, history.OrderID).Scan(&nonDeliveredCount)
		})
		if nonDeliveredCount == 0 {
			_ = database.SoftExec(txCtx, tx, `UPDATE commerce.orders SET status = 'delivered', delivered_at = now(), updated_at = now() WHERE id = $1;`, history.OrderID)
		}
	case commerce.StatusShipped, commerce.StatusInTransit, commerce.StatusOutForDelivery:
		var nonMatchingCount int
		_ = database.Soft(txCtx, tx, func() error {
			return tx.QueryRow(txCtx, `
			SELECT COUNT(*)
			FROM commerce.order_shipments
			WHERE order_id = $1
			  AND status NOT IN ($2, 'delivered', 'completed', 'cancelled');
		`, history.OrderID, string(to)).Scan(&nonMatchingCount)
		})
		if nonMatchingCount == 0 {
			_ = database.SoftExec(txCtx, tx, `UPDATE commerce.orders SET status = $2, delivered_at = NULL, updated_at = now() WHERE id = $1;`, history.OrderID, string(to))
		} else {
			_ = database.SoftExec(txCtx, tx, `UPDATE commerce.orders SET status = 'shipped', delivered_at = NULL, updated_at = now() WHERE id = $1 AND status IN ('pending', 'processing', 'confirmed');`, history.OrderID)
		}
	case commerce.StatusConfirmed:
		_ = database.SoftExec(txCtx, tx, `UPDATE commerce.orders SET status = 'confirmed', updated_at = now() WHERE id = $1 AND status = 'pending';`, history.OrderID)
	case commerce.StatusFailed:
		var nonFailedCount int
		_ = database.Soft(txCtx, tx, func() error {
			return tx.QueryRow(txCtx, `SELECT COUNT(*) FROM commerce.order_shipments WHERE order_id = $1 AND status NOT IN ('failed', 'cancelled', 'returned');`, history.OrderID).Scan(&nonFailedCount)
		})
		if nonFailedCount == 0 {
			_ = database.SoftExec(txCtx, tx, `UPDATE commerce.orders SET status = 'failed', notes = CASE WHEN $2 != '' THEN COALESCE(notes || E'\n' || $2, $2) ELSE notes END, updated_at = now() WHERE id = $1;`, history.OrderID, history.Notes)
		}
	case commerce.StatusCancelled:
		var nonCancelledCount int
		_ = database.Soft(txCtx, tx, func() error {
			return tx.QueryRow(txCtx, `SELECT COUNT(*) FROM commerce.order_shipments WHERE order_id = $1 AND status NOT IN ('failed', 'cancelled', 'returned');`, history.OrderID).Scan(&nonCancelledCount)
		})
		if nonCancelledCount == 0 {
			_ = database.SoftExec(txCtx, tx, `UPDATE commerce.orders SET status = 'cancelled', updated_at = now() WHERE id = $1;`, history.OrderID)
		}
	}
}

// settleShipmentWallet refunds or reverses wallet debits when a wallet-paid
// shipment is cancelled or refunded. It runs inside the caller's transaction.
func (r *Repository) settleShipmentWallet(
	txCtx context.Context,
	tx pgx.Tx,
	order *commerce.Order,
	id int64,
	from, to commerce.OrderStatus,
	actor int64,
	history commerce.OrderStatusHistory,
) error {
	if !commerce.IsWalletPayment(order.PaymentMethod) {
		return nil
	}
	switch {
	case to == commerce.StatusCancelled && from != commerce.StatusCancelled:
		var nowStatus string
		if err := tx.QueryRow(txCtx, `SELECT status FROM commerce.orders WHERE id = $1;`, order.ID).Scan(&nowStatus); err != nil {
			return err
		}
		if nowStatus == string(commerce.StatusCancelled) && order.Status != commerce.StatusCancelled {
			_, err := r.refundWalletDebits(txCtx, tx, order, nil, orderCancelKey(order.ID), actor, history.Notes)
			return err
		}
		_, err := r.refundWalletDebits(txCtx, tx, order, &id, shipmentCancelKey(order.ID, id), actor, history.Notes)
		return err
	case to == commerce.StatusRefunded && from != commerce.StatusRefunded:
		credited, err := r.refundWalletDebits(txCtx, tx, order, &id, shipmentRefundKey(order.ID, id), actor, history.Notes)
		if err != nil {
			return err
		}
		if !credited.IsPositive() {
			return apperr.Conflict("shipment.refund_nothing_to_refund",
				"لا توجد مدفوعات من المحفظة لاسترجاعها لهذه الشحنة. There is no wallet payment to refund for this shipment.")
		}
	}
	return nil
}
