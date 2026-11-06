package postgres

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// CancelAPIOrder locks the order and its shipments, applies the shared
// cancellation rule and cancels the order. A wallet-paid order is refunded in
// the same transaction, so a cancel never commits without its refund.
func (r *Repository) CancelAPIOrder(ctx context.Context, request commerce.APICancelRequest) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		order, err := lockAPICancellationOrder(txCtx, tx, request.OrderID)
		if err != nil {
			return err
		}
		if !request.Staff && order.CustomerID != request.UserID && (order.OrganizationID == nil || request.OrganizationID <= 0 || *order.OrganizationID != request.OrganizationID) {
			return apperr.Forbidden("order.unauthorized", "Not authorized to cancel this order")
		}
		if err := commerce.CheckCancellation(order); err != nil {
			return err
		}
		if _, err := tx.Exec(txCtx, `UPDATE commerce.orders SET status = 'cancelled', updated_at = now() WHERE id = $1`, request.OrderID); err != nil {
			return err
		}
		if _, err := tx.Exec(txCtx, `UPDATE commerce.order_shipments SET status = 'cancelled', updated_at = now() WHERE order_id = $1`, request.OrderID); err != nil {
			return err
		}
		if _, err := tx.Exec(txCtx, `INSERT INTO commerce.order_status_history (order_id, from_status, to_status, notes, changed_by_user_id) VALUES ($1, $2, 'cancelled', $3, $4)`, request.OrderID, string(order.Status), request.Reason, request.UserID); err != nil {
			return err
		}
		orgs := map[int64]bool{}
		if order.OrganizationID != nil {
			orgs[*order.OrganizationID] = true
		}
		for _, shipment := range order.Shipments {
			orgs[shipment.OrganizationID] = true
		}
		for orgID := range orgs {
			if orgID <= 0 {
				continue
			}
			if err := database.WriteAudit(txCtx, tx, database.AuditEntry{OrganizationID: &orgID, ActorUserID: request.UserID, Action: "order.cancelled", EntityType: "order", EntityID: strconv.FormatInt(request.OrderID, 10), Before: map[string]any{"status": order.Status}, After: map[string]any{"status": commerce.StatusCancelled, "notes": request.Reason}}); err != nil {
				return err
			}
		}
		_, err = r.refundWalletDebits(txCtx, tx, order, nil, orderCancelKey(request.OrderID), request.UserID, request.Reason)
		return err
	})
}

// lockAPICancellationOrder locks the order row first and then its shipments, in
// that order everywhere, so two transactions cannot wait on each other.
func lockAPICancellationOrder(ctx context.Context, tx pgx.Tx, orderID int64) (*commerce.Order, error) {
	order := &commerce.Order{ID: orderID}
	err := tx.QueryRow(ctx, `SELECT customer_id, organization_id, status, payment_method, payment_status, COALESCE(order_number, '') FROM commerce.orders WHERE id = $1 FOR UPDATE`, orderID).Scan(&order.CustomerID, &order.OrganizationID, &order.Status, &order.PaymentMethod, &order.PaymentStatus, &order.OrderNumber)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("order")
	}
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id, organization_id, status, subtotal, shipping_fee, total_amount, wallet_paid_amount, wallet_paid_at, wallet_refunded_amount FROM commerce.order_shipments WHERE order_id = $1 ORDER BY id FOR UPDATE`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		shipment := &commerce.OrderShipment{}
		if err := rows.Scan(&shipment.ID, &shipment.OrganizationID, &shipment.Status, &shipment.Subtotal, &shipment.ShippingFee, &shipment.TotalAmount, &shipment.WalletPaidAmount, &shipment.WalletPaidAt, &shipment.WalletRefundedAmount); err != nil {
			return nil, err
		}
		order.Shipments = append(order.Shipments, shipment)
	}
	return order, rows.Err()
}

// CancelAPIVendorShipment cancels one supplier's shipment under the shared rule.
// When no other shipment is left, the order is cancelled and refunded in full.
// Otherwise only this shipment's goods are refunded, and never more than the
// ledger shows as debited.
func (r *Repository) CancelAPIVendorShipment(ctx context.Context, request commerce.APIVendorCancelRequest) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		order, err := lockAPICancellationOrder(txCtx, tx, request.OrderID)
		if err != nil {
			return err
		}
		if err := commerce.CheckCancellation(order); err != nil {
			return err
		}
		var own *commerce.OrderShipment
		remaining := 0
		for _, shipment := range order.Shipments {
			if shipment.ID == request.ShipmentID && shipment.OrganizationID == request.VendorOrgID {
				own = shipment
				continue
			}
			if shipment.Status != commerce.StatusCancelled && shipment.Status != commerce.StatusFailed && shipment.Status != commerce.StatusReturned {
				remaining++
			}
		}
		if own == nil {
			return apperr.Forbidden("shipment.unauthorized", "Not authorized to update this shipment")
		}
		if own.Status == commerce.StatusCancelled || !commerce.IsValidStatusTransition(own.Status, commerce.StatusCancelled) {
			return apperr.Conflict("shipment.invalid_transition", "لا يمكن إلغاء الشحنة في حالتها الحالية. Shipment cannot be cancelled in its current state.")
		}
		if _, err := tx.Exec(txCtx, `UPDATE commerce.order_shipments SET status='cancelled', updated_at=now() WHERE id=$1`, own.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(txCtx, `INSERT INTO commerce.order_status_history (order_id,shipment_id,from_status,to_status,notes,changed_by_user_id) VALUES ($1,$2,$3,'cancelled',$4,$5)`, order.ID, own.ID, string(own.Status), request.Reason, request.UserID); err != nil {
			return err
		}
		if remaining == 0 {
			if _, err := tx.Exec(txCtx, `UPDATE commerce.orders SET status='cancelled',updated_at=now() WHERE id=$1`, order.ID); err != nil {
				return err
			}
			_, err = r.refundWalletDebits(txCtx, tx, order, nil, orderCancelKey(order.ID), request.UserID, request.Reason)
			return err
		}
		_, err = r.refundWalletDebits(txCtx, tx, order, &own.ID, shipmentCancelKey(order.ID, own.ID), request.UserID, request.Reason)
		return err
	})
}
