package postgres

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// AcceptNegotiation accepts the buyer's negotiated price on an order for one
// supplier. Only that supplier's shipments change: they are confirmed, and charged
// to the buyer's wallet (walletUserID) in this transaction when the order is paid
// from the wallet. Other suppliers' shipments, and an order that is delivered,
// cancelled or otherwise closed, are never changed (Batch 2, S04).
//
// The order must still have a pending negotiation and be open. The accepting
// supplier must be on the order with shipments that are still open.
func (r *Repository) AcceptNegotiation(ctx context.Context, orderID, vendorOrgID, actorID, walletUserID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		order, err := lockAPICancellationOrder(txCtx, tx, orderID)
		if err != nil {
			return err
		}
		var isNegotiation bool
		var negotiationStatus string
		var customerOrgID *int64
		if err := tx.QueryRow(txCtx, `
			SELECT is_negotiation, COALESCE(negotiation_status, 'none'), organization_id
			FROM commerce.orders WHERE id = $1;`, orderID).Scan(&isNegotiation, &negotiationStatus, &customerOrgID); err != nil {
			return err
		}
		if !isNegotiation || negotiationStatus != "pending" {
			return apperr.Conflict("order.negotiation_not_pending",
				"لا يوجد تفاوض على الأسعار بانتظار القرار لهذا الطلب. This order has no price negotiation waiting for a decision.")
		}
		switch order.Status {
		case commerce.StatusPending, commerce.StatusProcessing, commerce.StatusConfirmed, commerce.StatusOnHold:
		default:
			return apperr.Conflict("order.negotiation_closed",
				"لا يمكن قبول التفاوض على طلب في حالته الحالية. A negotiation cannot be accepted on an order in its current state.")
		}

		var own []*commerce.OrderShipment
		for _, s := range order.Shipments {
			if s != nil && s.OrganizationID == vendorOrgID {
				own = append(own, s)
			}
		}
		if len(own) == 0 {
			return apperr.Forbidden("order.negotiation_not_party",
				"هذا الطلب لا يتضمن شحنة من مورّدك. This order has no shipment from your organization.")
		}
		for _, s := range own {
			switch s.Status {
			case commerce.StatusPending, commerce.StatusProcessing, commerce.StatusOnHold, commerce.StatusConfirmed:
			default:
				return apperr.Conflict("shipment.negotiation_closed",
					"لا يمكن قبول التفاوض على شحنة في حالتها الحالية. This shipment is past the point where a negotiation can be accepted.")
			}
		}

		// Each of this supplier's shipments is charged once, as its confirmation
		// debits it. The charge and the confirmation commit together.
		for _, s := range own {
			if err := r.chargeShipment(txCtx, tx, order, s, walletUserID, actorID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(txCtx, `
			UPDATE commerce.order_shipments
			SET status = 'confirmed', updated_at = now()
			WHERE order_id = $1 AND organization_id = $2
			  AND status IN ('pending', 'processing', 'on_hold');`, orderID, vendorOrgID); err != nil {
			return err
		}
		res, err := tx.Exec(txCtx, `
			UPDATE commerce.orders
			SET negotiation_status = 'accepted',
			    status = CASE WHEN status IN ('pending', 'processing', 'on_hold') THEN 'confirmed' ELSE status END,
			    updated_at = now()
			WHERE id = $1 AND negotiation_status = 'pending';`, orderID)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.Conflict("order.negotiation_not_pending",
				"لا يوجد تفاوض على الأسعار بانتظار القرار لهذا الطلب. This order has no price negotiation waiting for a decision.")
		}

		_ = database.SoftExec(txCtx, tx, `
			INSERT INTO commerce.order_status_history (order_id, to_status, notes, changed_by_user_id)
			VALUES ($1, 'confirmed', 'تم قبول السعر المتفاوض عليه واعتماد الطلب من قبل المورد', $2);
		`, orderID, actorID)

		orderIDStr := strconv.FormatInt(orderID, 10)
		after := map[string]any{"negotiation_status": "accepted", "status": "confirmed"}
		if customerOrgID != nil && *customerOrgID > 0 {
			_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
				OrganizationID: customerOrgID,
				ActorUserID:    actorID,
				Action:         "order.negotiation_accepted",
				EntityType:     "order",
				EntityID:       orderIDStr,
				After:          after,
			})
		}
		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: &vendorOrgID,
			ActorUserID:    actorID,
			Action:         "order.negotiation_accepted",
			EntityType:     "order",
			EntityID:       orderIDStr,
			After:          after,
		})
		return nil
	})
}
