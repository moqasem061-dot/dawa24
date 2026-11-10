package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// purchaseScope is the idempotency scope of purchase orders (data-model.md).
const purchaseScope = "purchase_order"

// BeginIdempotency inserts the in_progress row, or reads the row that exists.
// Every statement runs under the buyer's tenant.
func (r *Repository) BeginIdempotency(ctx context.Context, orgID, integrationID int64, ref, requestSHA string) (erp.IdemState, error) {
	var state erp.IdemState
	err := r.db.InTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO erp.idempotency_records (organization_id, integration_id, scope, external_reference, request_sha256, state)
			VALUES ($1, $2, $3, $4, $5, 'in_progress') ON CONFLICT DO NOTHING`, orgID, integrationID, purchaseScope, ref, requestSHA)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 1 {
			state.Created = true
			state.RequestSHA = requestSHA
			return nil
		}
		var stored string
		var age float64
		var status string
		err = tx.QueryRow(ctx, `SELECT request_sha256, state, COALESCE(response::text, ''), EXTRACT(EPOCH FROM (now() - created_at))
			FROM erp.idempotency_records WHERE integration_id = $1 AND scope = $2 AND external_reference = $3`,
			integrationID, purchaseScope, ref).Scan(&state.RequestSHA, &status, &stored, &age)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("idempotency row vanished for reference %s", ref)
		}
		if err != nil {
			return err
		}
		switch {
		case status == "done":
			state.Replay, state.Response = true, []byte(stored)
		case age > erp.IdempotencyStaleAfter.Seconds():
			state.InProgress, state.Stale = true, true
		default:
			state.InProgress = true
		}
		return nil
	})
	return state, err
}

// FinishIdempotency stores the answer, upserts the buyer's item codes and, when
// the audit names an action, writes the audit row, all in one transaction.
// Codes are keyed by the buyer organization and the numeric variant id.
func (r *Repository) FinishIdempotency(ctx context.Context, orgID, integrationID int64, ref string, orderID int64, orderNumber string, response []byte, audit erp.AuditRecord, codes []erp.ItemCode) error {
	return r.db.InTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE erp.idempotency_records SET state = 'done', order_id = $4, order_number = $5, response = $6::jsonb, updated_at = now()
			WHERE integration_id = $1 AND scope = $2 AND external_reference = $3`,
			integrationID, purchaseScope, ref, orderID, orderNumber, string(response)); err != nil {
			return err
		}
		for _, c := range codes {
			if _, err := tx.Exec(ctx, `INSERT INTO erp.item_codes (organization_id, variant_id, integration_id, external_code, updated_at)
				VALUES ($1, $2, $3, $4, now())
				ON CONFLICT (organization_id, variant_id) DO UPDATE SET external_code = EXCLUDED.external_code, integration_id = EXCLUDED.integration_id, updated_at = now()`,
				orgID, c.VariantID, integrationID, c.Code); err != nil {
				return fmt.Errorf("erp: upsert item code: %w", err)
			}
		}
		if audit.Action == "" {
			return nil
		}
		return writeAudit(ctx, tx, orgID, audit)
	})
}

// DeleteIdempotency frees a reference after a refusal. A finished row is never
// deleted: its replay is the answer.
func (r *Repository) DeleteIdempotency(ctx context.Context, orgID, integrationID int64, ref string) error {
	return r.db.InTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM erp.idempotency_records WHERE integration_id = $1 AND scope = $2 AND external_reference = $3 AND state = 'in_progress'`,
			integrationID, purchaseScope, ref)
		return err
	})
}

// FindOrderByNoteMarker finds the buyer's order whose note starts with the marker
// (the note is written first, so a crashed attempt can be found by it).
func (r *Repository) FindOrderByNoteMarker(ctx context.Context, buyerOrgID int64, marker string) (int64, string, bool, error) {
	var id int64
	var number string
	err := r.db.InReadTx(database.WithTenant(ctx, buyerOrgID), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id, order_number FROM commerce.orders
			WHERE organization_id = $1 AND deleted_at IS NULL AND starts_with(notes, $2)
			ORDER BY id LIMIT 1`, buyerOrgID, marker).Scan(&id, &number)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	return id, number, true, nil
}

// SupplierLinked reports whether the buyer has a pending or approved link to the
// supplier. org.user_organizations has no row-level security (research D-20).
func (r *Repository) SupplierLinked(ctx context.Context, buyerOrgID, supplierOrgID int64) (bool, error) {
	var linked bool
	err := r.db.InReadTx(database.WithTenant(ctx, buyerOrgID), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM org.user_organizations
			WHERE vendor_org_id = $2 AND customer_org_id = $1 AND status IN ('pending', 'approved') AND deleted_at IS NULL)`,
			buyerOrgID, supplierOrgID).Scan(&linked)
	})
	return linked, err
}
