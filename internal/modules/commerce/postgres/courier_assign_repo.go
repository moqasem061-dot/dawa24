package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Handing a parcel to a delivery representative, and taking it back.

// AssignShipmentCourier puts one parcel on a courier's round, or returns it to
// the unassigned pool when courierUserID is nil.
//
// The supplier's id is part of the predicate rather than checked beforehand,
// so a shipment belonging to another company cannot be reassigned even if its
// id is guessed: the UPDATE simply matches no row and the caller is told the
// parcel was not found.
//
// The assignment timestamp is set by the database, not by the caller, because
// it is what the courier's queue is ordered by. A clock supplied by a request
// would let a parcel jump the queue.
//
// Reassigning an already-assigned parcel is allowed and re-stamps the date:
// the new courier has been holding it since they were handed it, not since it
// left the warehouse. Unassigning clears the date, so a parcel that comes back
// to the pool does not carry a stale age into someone else's round.
func (r *Repository) AssignShipmentCourier(
	ctx context.Context,
	shipmentID, vendorOrgID int64,
	courierUserID *int64,
	assignedBy int64,
) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			UPDATE commerce.order_shipments
			   SET courier_user_id     = $3,
			       courier_assigned_at = CASE WHEN $3::bigint IS NULL THEN NULL ELSE now() END,
			       courier_assigned_by = CASE WHEN $3::bigint IS NULL THEN NULL ELSE $4::bigint END,
			       updated_at          = now()
			 WHERE id = $1 AND organization_id = $2;
		`
		res, err := tx.Exec(txCtx, query, shipmentID, vendorOrgID, courierUserID, assignedBy)
		if err != nil {
			return fmt.Errorf("commerce postgres: assign shipment courier: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("shipment")
		}

		// The handover is part of the parcel's audit trail. It is recorded as
		// a history row with the status unchanged, so the order timeline shows
		// who was carrying it and from when, beside the status changes that
		// happened on their round.
		note := courierAssignmentNote(courierUserID)
		const history = `
			INSERT INTO commerce.order_status_history
				(order_id, shipment_id, from_status, to_status, notes, changed_by_user_id)
			SELECT s.order_id, s.id, s.status, s.status, $2, $3
			  FROM commerce.order_shipments s
			 WHERE s.id = $1;
		`
		if _, err := tx.Exec(txCtx, history, shipmentID, note, assignedBy); err != nil {
			return fmt.Errorf("commerce postgres: record courier assignment: %w", err)
		}
		return nil
	})
}

func courierAssignmentNote(courierUserID *int64) string {
	if courierUserID == nil {
		return i18n.TDefault("vendor.delivery.unassignment_note")
	}
	return fmt.Sprintf(i18n.TDefault("vendor.delivery.assignment_note"), *courierUserID)
}

// GetVendorShipment reads one parcel scoped to the supplier that owns it, with
// the full enrichment the courier screen renders.
//
// Ownership is a predicate rather than a check on the returned row: a member of
// company A asking for company B's shipment id gets "not found", which is the
// only answer that does not confirm the parcel exists.
func (r *Repository) GetVendorShipment(ctx context.Context, shipmentID, vendorOrgID int64) (*commerce.OrderShipment, error) {
	const where = `
	WHERE s.id = $1 AND s.organization_id = $2;`
	return r.shipmentDetail(ctx, shipmentDetailSelect+where, shipmentID, vendorOrgID)
}

// GetVendorShipmentByOrderID reads one parcel belonging to a parent order scoped to the supplier.
func (r *Repository) GetVendorShipmentByOrderID(ctx context.Context, orderID, vendorOrgID int64) (*commerce.OrderShipment, error) {
	const where = `
	WHERE s.order_id = $1 AND s.organization_id = $2 ORDER BY s.id ASC LIMIT 1;`
	return r.shipmentDetail(ctx, shipmentDetailSelect+where, orderID, vendorOrgID)
}
