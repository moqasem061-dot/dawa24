package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// messageJSONText renders the {ar,en} message map as the JSON text the JSONB
// columns take.
func messageJSONText(m map[string]string) string {
	if m == nil {
		return "{}"
	}
	data, _ := json.Marshal(m)
	return string(data)
}

// reviewColumns mirrors erp.review_items.
const reviewColumns = `r.id, r.organization_id, r.integration_id, r.external_code, r.payload, r.candidates, r.reason_code, r.batch_id, r.state, r.resolved_product_id, r.resolved_variant_id, r.resolved_by, r.resolved_at, r.created_at, r.updated_at`

func scanReview(row scanner) (erp.ReviewItem, error) {
	var r erp.ReviewItem
	err := row.Scan(&r.ID, &r.OrganizationID, &r.IntegrationID, &r.ExternalCode, &r.Payload, &r.Candidates, &r.ReasonCode, &r.BatchID, &r.State, &r.ResolvedProductID, &r.ResolvedVariantID, &r.ResolvedBy, &r.ResolvedAt, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, erp.ErrNotFound
	}
	return r, err
}

// ListReview pages open items oldest-first with the shared keyset page.
func (r *Repository) ListReview(ctx context.Context, orgID int64, page erp.Page) ([]erp.ReviewItem, error) {
	limit := page.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []erp.ReviewItem
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+reviewColumns+`
			FROM erp.review_items r
			WHERE r.organization_id = $1 AND r.state = 'open' AND ($2 = 0 OR r.id > $2)
			ORDER BY r.id LIMIT $3`, orgID, page.After, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			item, err := scanReview(rows)
			if err != nil {
				return err
			}
			out = append(out, item)
		}
		return rows.Err()
	})
	return out, err
}

// GetReview reads one review row scoped to the organization.
func (r *Repository) GetReview(ctx context.Context, orgID, id int64) (erp.ReviewItem, error) {
	var out erp.ReviewItem
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = scanReview(tx.QueryRow(ctx, `SELECT `+reviewColumns+`
			FROM erp.review_items r WHERE r.organization_id = $1 AND r.id = $2`, orgID, id))
		return err
	})
	return out, err
}

// ResolveReviewRow closes the item as resolved — the variant and product the
// supplier picked — and writes the audit row in the same transaction (T114).
func (r *Repository) ResolveReviewRow(ctx context.Context, orgID, id, variantID, productID, actorUserID int64, audit erp.AuditRecord) error {
	return r.db.InTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE erp.review_items SET state = 'resolved',
				resolved_product_id = $3, resolved_variant_id = $4,
				resolved_by = NULLIF($5,0), resolved_at = now(), updated_at = now()
			WHERE organization_id = $1 AND id = $2 AND state = 'open'`,
			orgID, id, productID, variantID, actorUserID)
		if err != nil {
			return fmt.Errorf("erp: resolve review: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return erp.ErrNotFound
		}
		return database.WriteAudit(ctx, tx, database.AuditEntry{
			OrganizationID: &orgID, ActorUserID: actorUserID,
			Action: audit.Action, EntityType: audit.EntityType, EntityID: audit.EntityID,
			Before: audit.Before, After: audit.After,
		})
	})
}

// DismissReviewRow closes the item without linking anything, with its own audit
// row in the same transaction.
func (r *Repository) DismissReviewRow(ctx context.Context, orgID, id, actorUserID int64, audit erp.AuditRecord) error {
	return r.db.InTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE erp.review_items SET state = 'dismissed',
				resolved_by = NULLIF($3,0), resolved_at = now(), updated_at = now()
			WHERE organization_id = $1 AND id = $2 AND state = 'open'`,
			orgID, id, actorUserID)
		if err != nil {
			return fmt.Errorf("erp: dismiss review: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return erp.ErrNotFound
		}
		return database.WriteAudit(ctx, tx, database.AuditEntry{
			OrganizationID: &orgID, ActorUserID: actorUserID,
			Action: audit.Action, EntityType: audit.EntityType, EntityID: audit.EntityID,
			Before: audit.Before, After: audit.After,
		})
	})
}

var _ = time.Now
