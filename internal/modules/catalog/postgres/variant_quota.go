package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
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

// SetVariantQuotaLimits writes the per-branch caps of many variants in one
// statement — the ERP sync calls it once per chunk. A nil q clears the column;
// the organization clause keeps the bulk write tenant-scoped like the
// single-variant write above, and id columns not belonging to the caller's
// organization are silently left alone (their item then reports its write
// failure rather than another tenant's row being touched by a stranger's id).
func (r *Repository) SetVariantQuotaLimits(ctx context.Context, orgID int64, limits map[int64]*int) error {
	if len(limits) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(limits))
	caps := make([]*int, 0, len(limits))
	for id, limit := range limits {
		ids = append(ids, id)
		caps = append(caps, limit)
	}
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE catalog.product_variants AS pv
			SET quota_limit = u.q, updated_at = now()
			FROM unnest($2::bigint[], $3::int[]) AS u(id, q)
			WHERE pv.id = u.id
			  AND pv.deleted_at IS NULL
			  AND ($1::bigint = 0 OR pv.organization_id = $1);
		`, orgID, ids, caps)
		if err != nil {
			return fmt.Errorf("catalog postgres: set variant quota limits: %w", err)
		}
		return nil
	})
}
