package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// SetCartItemQuantity changes quantity without touching unit_price.
//
// Leaving unit_price alone is the entire point: it holds the price captured
// when the item was added, and a quantity change must not reprice the line.
func (r *Repository) SetCartItemQuantity(ctx context.Context, cartID, variantID int64, quantity int) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE commerce.cart_items
			SET quantity = $3, updated_at = now()
			WHERE cart_id = $1 AND product_variant_id = $2;
		`
		res, err := tx.Exec(txCtx, query, cartID, variantID, quantity)
		if err != nil {
			return fmt.Errorf("commerce postgres: set cart quantity: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("cart item")
		}
		return nil
	})
}

// GetShipmentByID returns one shipment.
func (r *Repository) GetShipmentByID(ctx context.Context, id int64) (*commerce.OrderShipment, error) {
	var s commerce.OrderShipment
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, order_id, organization_id, branch_id, shipment_number,
			       status, subtotal, shipping_fee, total_amount, tracking_number,
			       carrier_name, delivery_code, delivery_attempts, delivery_locked_until,
			       delivery_notes, collected_amount_minor, delivered_by_courier_at,
			       shipped_at, delivered_at, created_at, updated_at
			FROM commerce.order_shipments
			WHERE id = $1;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&s.ID, &s.PublicID, &s.OrderID, &s.OrganizationID, &s.BranchID, &s.ShipmentNumber,
			&statusStr, &s.Subtotal, &s.ShippingFee, &s.TotalAmount, &s.TrackingNumber,
			&s.CarrierName, &s.DeliveryCode, &s.DeliveryAttempts, &s.DeliveryLockedUntil,
			&s.DeliveryNotes, &s.CollectedAmountMinor, &s.DeliveredByCourierAt,
			&s.ShippedAt, &s.DeliveredAt, &s.CreatedAt, &s.UpdatedAt,
		)
		if err != nil {
			return err
		}
		s.Status = commerce.OrderStatus(statusStr)
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("shipment")
		}
		return nil, fmt.Errorf("commerce postgres: get shipment: %w", err)
	}
	return &s, nil
}

// UpdateShipmentStatus advances a shipment and records the transition.
//
// The status change, its history row, any wallet charge or refund and the order's
// payment status are written in one transaction, so the audit trail cannot
// disagree with the current state. The `status = $2` predicate makes it a
// compare-and-swap: two vendor staff clicking at once produce one transition and
// one conflict, not two history rows.
func (r *Repository) UpdateShipmentStatus(
	ctx context.Context,
	id int64,
	from, to commerce.OrderStatus,
	history commerce.OrderStatusHistory,
) error {
	return r.updateShipmentStatus(ctx, id, from, to, history, 0)
}

// UpdateShipmentStatusWithWallet is UpdateShipmentStatus for a supplier's move on
// a wallet order. A confirmation charges this shipment to the buyer's wallet
// (walletUserID) in the same transaction; when the charge is refused, the shipment
// stays as it was.
func (r *Repository) UpdateShipmentStatusWithWallet(
	ctx context.Context,
	id int64,
	from, to commerce.OrderStatus,
	history commerce.OrderStatusHistory,
	walletUserID int64,
) error {
	return r.updateShipmentStatus(ctx, id, from, to, history, walletUserID)
}

func (r *Repository) updateShipmentStatus(
	ctx context.Context,
	id int64,
	from, to commerce.OrderStatus,
	history commerce.OrderStatusHistory,
	walletUserID int64,
) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// The order is locked before the shipment, as in every other path. The write
		// runs as system because a cancel refunds the buyer's wallet. The caller has
		// already checked that the shipment belongs to its tenant.
		order, err := lockAPICancellationOrder(txCtx, tx, history.OrderID)
		if err != nil {
			return err
		}
		actor := actorOrZero(history.ChangedByUserID)
		if shipment := findOrderShipment(order, id); shipment != nil &&
			to == commerce.StatusConfirmed && from != commerce.StatusConfirmed &&
			commerce.IsWalletPayment(order.PaymentMethod) {
			if err := r.chargeShipment(txCtx, tx, order, shipment, walletUserID, actor); err != nil {
				return err
			}
		}
		update := `
			UPDATE commerce.order_shipments
			SET status = $3,
			    delivery_notes = CASE WHEN $4 != '' THEN $4 ELSE delivery_notes END,
			    shipped_at   = CASE WHEN $3 = 'shipped'   THEN now() ELSE shipped_at   END,
			    delivered_at = CASE WHEN $3 = 'delivered' THEN now() WHEN $3 IN ('shipped', 'in_transit', 'out_for_delivery') THEN NULL ELSE delivered_at END
			WHERE id = $1 AND status = $2;
		`
		res, err := tx.Exec(txCtx, update, id, string(from), string(to), history.Notes)
		if err != nil {
			return fmt.Errorf("commerce postgres: update shipment status: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.Conflict("shipment.state_changed",
				"This shipment was already updated by someone else. Reload and try again.")
		}

		insert := `
			INSERT INTO commerce.order_status_history
				(order_id, shipment_id, from_status, to_status, notes, changed_by_user_id)
			VALUES ($1, $2, $3, $4, $5, $6);
		`
		if _, err := tx.Exec(txCtx, insert,
			history.OrderID, history.ShipmentID, history.FromStatus,
			history.ToStatus, history.Notes, history.ChangedByUserID,
		); err != nil {
			return fmt.Errorf("commerce postgres: insert shipment history: %w", err)
		}

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
	})
}

// ListOrderHistory returns every status transition for an order, oldest first.
func (r *Repository) ListOrderHistory(ctx context.Context, orderID int64) ([]*commerce.OrderStatusHistory, error) {
	var list []*commerce.OrderStatusHistory
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, order_id, shipment_id, from_status, to_status, notes,
			       changed_by_user_id, created_at
			FROM commerce.order_status_history
			WHERE order_id = $1
			ORDER BY created_at ASC, id ASC;
		`
		rows, err := tx.Query(txCtx, query, orderID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var h commerce.OrderStatusHistory
			if err := rows.Scan(
				&h.ID, &h.OrderID, &h.ShipmentID, &h.FromStatus, &h.ToStatus,
				&h.Notes, &h.ChangedByUserID, &h.CreatedAt,
			); err != nil {
				return err
			}
			list = append(list, &h)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("commerce postgres: list order history: %w", err)
	}
	return list, nil
}

// RateOrder records a customer rating on their own order (audit §3.3).
//
// Validates that the order is delivered and hasn't already been rated.
// The customer_id predicate stops one buyer rating another's order.
func (r *Repository) RateOrder(ctx context.Context, orderID, customerID int64, rating float64, review string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var status string
		var ratedAt *time.Time
		err := tx.QueryRow(txCtx, `SELECT status, rated_at FROM commerce.orders WHERE id = $1 AND customer_id = $2;`, orderID, customerID).Scan(&status, &ratedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.NotFound("order")
			}
			return fmt.Errorf("commerce postgres: check order: %w", err)
		}
		if status != string(commerce.StatusDelivered) {
			return apperr.Validation("order.not_delivered", "Only delivered orders can be rated.", nil)
		}
		if ratedAt != nil {
			return apperr.Validation("order.already_rated", "This order has already been rated.", nil)
		}
		query := `
			UPDATE commerce.orders
			SET rating = $3, review = $4, rated_at = now(), updated_at = now()
			WHERE id = $1 AND customer_id = $2;
		`
		res, err := tx.Exec(txCtx, query, orderID, customerID, rating, review)
		if err != nil {
			return fmt.Errorf("commerce postgres: rate order: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("order")
		}
		return nil
	})
}

// SetShipmentTracking records the carrier and tracking number for a shipment.
func (r *Repository) SetShipmentTracking(ctx context.Context, id int64, carrier, tracking string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `UPDATE commerce.order_shipments SET carrier_name = $2, tracking_number = $3 WHERE id = $1;`, id, carrier, tracking)
		return err
	})
}

// CountVendorShipmentsByStatus counts a vendor's shipments in the given
// statuses. Counting a capped page instead reports the cap, not the total.
func (r *Repository) CountVendorShipmentsByStatus(ctx context.Context, orgID int64, statuses []string) (int, error) {
	var total int
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT COUNT(*) FROM commerce.order_shipments
			WHERE organization_id = $1 AND status = ANY($2);
		`
		return tx.QueryRow(txCtx, query, orgID, statuses).Scan(&total)
	})
	return total, err
}
