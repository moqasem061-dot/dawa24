package postgres

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// cancelOwnNegotiationPart cancels the rejecting supplier's open shipments on an
// order. The supplier must be on the order, and every one of its shipments must
// still be open for a decision (pending, processing or on hold). A shipment past
// that point refuses the rejection, so a confirmed or charged shipment is never
// cancelled here (Batch 3, negotiation decision).
func cancelOwnNegotiationPart(ctx context.Context, tx pgx.Tx, orderID, vendorOrgID int64) error {
	rows, err := tx.Query(ctx, `
		SELECT status FROM commerce.order_shipments
		WHERE order_id = $1 AND organization_id = $2;`, orderID, vendorOrgID)
	if err != nil {
		return err
	}
	var count int
	var closed bool
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			rows.Close()
			return err
		}
		count++
		switch status {
		case "pending", "processing", "on_hold":
		default:
			closed = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if count == 0 {
		return apperr.Forbidden("order.negotiation_not_party",
			"هذا الطلب لا يتضمن شحنة من مورّدك. This order has no shipment from your organization.")
	}
	if closed {
		return apperr.Conflict("shipment.negotiation_closed",
			"لا يمكن رفض التفاوض على شحنة في حالتها الحالية. This shipment is past the point where a negotiation can be rejected.")
	}
	_, err = tx.Exec(ctx, `
		UPDATE commerce.order_shipments
		SET status = 'cancelled', updated_at = now()
		WHERE order_id = $1 AND organization_id = $2
		  AND status IN ('pending', 'processing', 'on_hold');`, orderID, vendorOrgID)
	return err
}

// settlePartialNegotiationRejection finishes a rejection that leaves other
// suppliers on the order. If they have all decided, the order goes ahead with
// them. If not, it stays pending until they do. Nothing is refunded: a shipment
// is charged only when its supplier accepts, and the rejecting supplier never did.
func settlePartialNegotiationRejection(
	ctx context.Context, tx pgx.Tx, orderID int64, customerOrgID *int64, reason string, actorID int64,
) error {
	var awaiting int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM commerce.order_shipments
		WHERE order_id = $1 AND status IN ('pending', 'processing', 'on_hold');`, orderID).Scan(&awaiting); err != nil {
		return err
	}
	if awaiting == 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE commerce.orders
			SET negotiation_status = 'accepted',
			    status = CASE WHEN status IN ('pending', 'processing', 'on_hold') THEN 'confirmed' ELSE status END,
			    updated_at = now()
			WHERE id = $1 AND negotiation_status = 'pending';`, orderID); err != nil {
			return err
		}
	}
	if customerOrgID != nil && *customerOrgID > 0 {
		_ = database.WriteAudit(ctx, tx, database.AuditEntry{
			OrganizationID: customerOrgID,
			ActorUserID:    actorID,
			Action:         "order.negotiation_supplier_rejected",
			EntityType:     "order",
			EntityID:       strconv.FormatInt(orderID, 10),
			After:          map[string]any{"reason": reason, "order_left_open": awaiting > 0},
		})
	}
	return nil
}
