package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// ListShipmentsByVendor retrieves shipment partitions for a vendor organization.
func (r *Repository) ListShipmentsByVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*commerce.OrderShipment, error) {
	shipments, _, err := r.ListShipmentsByVendorWithTotal(ctx, vendorOrgID, "", limit, offset)
	return shipments, err
}

// ListShipmentsByVendorWithTotal retrieves one page of a supplier's shipments,
// optionally filtered by status, with the total that matched.
//
// It reads through the shared enriched projection rather than a projection of
// its own. The supply-orders screen now shows who is carrying each parcel and
// since when, and a second hand-written SELECT over the same table is how that
// column would have been added to one screen and forgotten on the other.
func (r *Repository) ListShipmentsByVendorWithTotal(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.OrderShipment, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}

	var (
		shipments []*commerce.OrderShipment
		total     int
	)
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const countSQL = `
			SELECT count(*)
			  FROM commerce.order_shipments s
			 WHERE s.organization_id = $1
			   AND ($2::text = '' OR s.status = $2 OR ($2 = 'failed' AND s.status = 'returned'));
		`
		if err := tx.QueryRow(txCtx, countSQL, vendorOrgID, status).Scan(&total); err != nil {
			return err
		}

		query := shipmentDetailSelect + `
	WHERE s.organization_id = $1
	  AND ($2::text = '' OR s.status = $2 OR ($2 = 'failed' AND s.status = 'returned'))
	ORDER BY s.created_at DESC, s.id DESC
	LIMIT $3 OFFSET $4;`
		rows, err := tx.Query(txCtx, query, vendorOrgID, status, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			sh, err := scanShipmentDetail(rows)
			if err != nil {
				return err
			}
			shipments = append(shipments, sh)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, fmt.Errorf("commerce postgres: list vendor shipments: %w", err)
	}

	ids := make([]int64, 0, len(shipments))
	for _, sh := range shipments {
		ids = append(ids, sh.ID)
	}
	lines, err := r.shipmentLines(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	for _, sh := range shipments {
		sh.Lines = lines[sh.ID]
	}
	return shipments, total, nil
}

// CountOrders returns the total number of orders on the platform.
//
// Used by the admin dashboard, which previously counted len() of a page capped
// at 100 and so reported 100 for any platform with more than that.
func (r *Repository) CountOrders(ctx context.Context) (int, error) {
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT COUNT(*) FROM commerce.orders;`).Scan(&total)
	})
	return total, err
}

// RejectNegotiation rejects a negotiated order and sets its status to cancelled.
func (r *Repository) RejectNegotiation(ctx context.Context, orderID int64, reason string, actorID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var customerOrgID *int64
		var customerUserID int64
		var isNegotiation bool
		var negotiationStatus string
		err := tx.QueryRow(txCtx, `
			SELECT organization_id, customer_id, is_negotiation, COALESCE(negotiation_status, 'none')
			FROM commerce.orders WHERE id = $1 FOR UPDATE;`, orderID).Scan(&customerOrgID, &customerUserID, &isNegotiation, &negotiationStatus)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("order")
			}
			return err
		}
		// Only a negotiation still waiting for a decision can be rejected. An
		// accepted or already rejected one must not be cancelled and refunded.
		if !isNegotiation || negotiationStatus != "pending" {
			return apperr.Conflict("order.negotiation_not_pending",
				"لا يوجد تفاوض على الأسعار بانتظار القرار لهذا الطلب. This order has no price negotiation waiting for a decision.")
		}

		if reason == "" {
			reason = i18n.TDefault("w4_mod.s_360_360")
		}
		query := `
			UPDATE commerce.orders
			SET negotiation_status = 'rejected',
			    status = 'cancelled',
			    negotiation_notes = $2,
			    updated_at = now()
			WHERE id = $1;
		`
		ct, err := tx.Exec(txCtx, query, orderID, reason)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return apperr.NotFound("order")
		}

		// Also update shipments
		_ = database.SoftExec(txCtx, tx, `UPDATE commerce.order_shipments SET status = 'cancelled', updated_at = now() WHERE order_id = $1;`, orderID)

		// Record in history
		_ = database.SoftExec(txCtx, tx, `
			INSERT INTO commerce.order_status_history (order_id, to_status, notes, changed_by_user_id)
			VALUES ($1, 'cancelled', $2, $3);
		`, orderID, reason, actorID)

		// Audit logs for customer and vendor
		orderIDStr := strconv.FormatInt(orderID, 10)
		if customerOrgID != nil && *customerOrgID > 0 {
			_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
				OrganizationID: customerOrgID,
				ActorUserID:    actorID,
				Action:         "order.negotiation_rejected",
				EntityType:     "order",
				EntityID:       orderIDStr,
				After:          map[string]any{"negotiation_status": "rejected", "status": "cancelled", "reason": reason},
			})
		}

		// The supplier ids are read into memory first: a pgx transaction cannot
		// run the audit insert while this result set is still open ("conn busy").
		var vendorOrgIDs []int64
		if rows, err := tx.Query(txCtx, `SELECT DISTINCT organization_id FROM commerce.order_shipments WHERE order_id = $1`, orderID); err == nil {
			for rows.Next() {
				var vOrgID int64
				if err := rows.Scan(&vOrgID); err == nil && vOrgID > 0 {
					vendorOrgIDs = append(vendorOrgIDs, vOrgID)
				}
			}
			rows.Close()
		}
		for _, vOrgID := range vendorOrgIDs {
			_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
				OrganizationID: &vOrgID,
				ActorUserID:    actorID,
				Action:         "order.negotiation_rejected",
				EntityType:     "order",
				EntityID:       orderIDStr,
				After:          map[string]any{"negotiation_status": "rejected", "status": "cancelled", "reason": reason},
			})
		}

		locked, err := lockAPICancellationOrder(txCtx, tx, orderID)
		if err != nil {
			return err
		}
		_, err = r.refundWalletDebits(txCtx, tx, locked, nil, orderCancelKey(orderID), actorID, reason)
		return err
	})
}
