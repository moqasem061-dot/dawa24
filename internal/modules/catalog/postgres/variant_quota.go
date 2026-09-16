package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// SetVariantQuotaLimit writes the per-branch cap on one of a supplier's variants.
//
// The organisation is part of the WHERE clause rather than only checked by the
// caller, for the same reason UpdateVariant scopes itself: this method is
// exported, and the clause is the only thing that makes it safe for whoever
// calls it next.
func (r *Repository) SetVariantQuotaLimit(ctx context.Context, orgID, variantID int64, limit *int) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx, `
			UPDATE catalog.product_variants
			SET quota_limit = $3, updated_at = now()
			WHERE id = $1
			  AND deleted_at IS NULL
			  AND ($2::bigint = 0 OR organization_id = $2);
		`, variantID, orgID, limit)
		if err != nil {
			return fmt.Errorf("catalog postgres: set variant quota: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("product_variant")
		}
		return nil
	})
}

// CountVariantsWithQuota counts a supplier's restricted items.
func (r *Repository) CountVariantsWithQuota(ctx context.Context, orgID int64) (int, error) {
	var n int
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT count(*)
			FROM catalog.product_variants
			WHERE organization_id = $1 AND deleted_at IS NULL AND quota_limit IS NOT NULL;
		`, orgID).Scan(&n)
	})
	if err != nil {
		return 0, fmt.Errorf("catalog postgres: count variants with quota: %w", err)
	}
	return n, nil
}
