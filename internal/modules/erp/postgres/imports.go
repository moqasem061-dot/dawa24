package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// UpsertOrderImport stores one acknowledgement (research D-18). The first call
// inserts and reports acknowledged. The same ERP reference again refreshes the
// acknowledged status, the time and the count, and reports already_acknowledged.
// A different reference changes nothing and reports the reference on record.
// The audit row is written in the same transaction as any change.
func (r *Repository) UpsertOrderImport(ctx context.Context, imp erp.OrderImport, audit erp.AuditRecord) (erp.AckOutcome, error) {
	var out erp.AckOutcome
	err := r.db.InTx(database.WithTenant(ctx, imp.OrganizationID), func(ctx context.Context, tx pgx.Tx) error {
		var inserted int64
		err := tx.QueryRow(ctx, `INSERT INTO erp.order_imports
			(organization_id, integration_id, direction, order_id, shipment_id, order_number, erp_reference, acked_status, api_key_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, 0))
			ON CONFLICT (integration_id, direction, order_id) DO NOTHING
			RETURNING id`,
			imp.OrganizationID, imp.IntegrationID, imp.Direction, imp.OrderID, imp.ShipmentID,
			imp.OrderNumber, imp.ErpReference, imp.AckedStatus, imp.APIKeyID).Scan(&inserted)
		if err == nil {
			out.Result = "acknowledged"
			return writeAudit(ctx, tx, imp.OrganizationID, audit)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		var current string
		if err := tx.QueryRow(ctx, `SELECT erp_reference FROM erp.order_imports
			WHERE organization_id = $1 AND integration_id = $2 AND direction = $3 AND order_id = $4
			FOR UPDATE`, imp.OrganizationID, imp.IntegrationID, imp.Direction, imp.OrderID).Scan(&current); err != nil {
			return err
		}
		if current != imp.ErpReference {
			out = erp.AckOutcome{Result: "conflict", Code: "ERP_ACK_REFERENCE_CONFLICT", ExistingReference: current}
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE erp.order_imports
			SET acked_status = $5, ack_count = ack_count + 1, acked_at = now(), api_key_id = NULLIF($6, 0),
			    shipment_id = COALESCE($7, shipment_id)
			WHERE organization_id = $1 AND integration_id = $2 AND direction = $3 AND order_id = $4`,
			imp.OrganizationID, imp.IntegrationID, imp.Direction, imp.OrderID, imp.AckedStatus, imp.APIKeyID, imp.ShipmentID); err != nil {
			return err
		}
		out.Result = "already_acknowledged"
		return writeAudit(ctx, tx, imp.OrganizationID, audit)
	})
	return out, err
}
