package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// QuotaVariantOptions lists the supplier's restricted items for the filter box.
func (r *Repository) QuotaVariantOptions(ctx context.Context, vendorOrgID int64) ([]commerce.QuotaOption, error) {
	return r.quotaOptions(ctx, `
		SELECT v.id,
		       TRIM(BOTH ' ' FROM
		            COALESCE(NULLIF(pr.name->>'ar', ''), NULLIF(pr.name->>'en', ''), '') || ' ' ||
		            COALESCE(NULLIF(v.name->>'ar', ''), NULLIF(v.name->>'en', ''), v.sku, ''))
		FROM catalog.product_variants v
		LEFT JOIN catalog.products pr ON pr.id = v.product_id
		WHERE v.organization_id = $1 AND v.deleted_at IS NULL AND v.quota_limit IS NOT NULL
		ORDER BY 2 ASC
		LIMIT 500;
	`, vendorOrgID, "quota variant options")
}

// QuotaCustomerOptions lists the buying organizations whose branches have consumed
// any of this supplier's restricted items, for the filter box.
func (r *Repository) QuotaCustomerOptions(ctx context.Context, vendorOrgID int64) ([]commerce.QuotaOption, error) {
	base := replaceReleasing(quotaConsumptionCTE, "$2")
	return r.quotaOptions(ctx, base+`
		SELECT DISTINCT co.id,
		       COALESCE(NULLIF(co.name->>'ar', ''), NULLIF(co.name->>'en', ''), 'منشأة #' || co.id::text)
		FROM (SELECT DISTINCT branch_id FROM pairings) p
		JOIN org.branches b ON b.id = p.branch_id
		JOIN org.organizations co ON co.id = b.organization_id
		ORDER BY 2 ASC
		LIMIT 500;
	`, vendorOrgID, "quota customer options", commerce.QuotaReleasingStatusStrings())
}

// QuotaBranchOptions lists the buying branches that have consumed any of this
// supplier's restricted items, for the filter box.
func (r *Repository) QuotaBranchOptions(ctx context.Context, vendorOrgID int64) ([]commerce.QuotaOption, error) {
	base := replaceReleasing(quotaConsumptionCTE, "$2")
	return r.quotaOptions(ctx, base+`
		SELECT b.id,
		       TRIM(BOTH ' ' FROM
		            COALESCE(NULLIF(co.name->>'ar', ''), NULLIF(co.name->>'en', ''), '') || ' - ' ||
		            COALESCE(NULLIF(b.name->>'ar', ''), NULLIF(b.name->>'en', ''), b.code, ''))
		FROM (SELECT DISTINCT branch_id FROM pairings) p
		JOIN org.branches b ON b.id = p.branch_id
		LEFT JOIN org.organizations co ON co.id = b.organization_id
		ORDER BY 2 ASC
		LIMIT 500;
	`, vendorOrgID, "quota branch options", commerce.QuotaReleasingStatusStrings())
}

func (r *Repository) quotaOptions(
	ctx context.Context, query string, vendorOrgID int64, what string, extra ...any,
) ([]commerce.QuotaOption, error) {
	if vendorOrgID <= 0 {
		return nil, nil
	}
	args := append([]any{vendorOrgID}, extra...)
	var out []commerce.QuotaOption
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var o commerce.QuotaOption
			if err := rows.Scan(&o.ID, &o.Label); err != nil {
				return err
			}
			out = append(out, o)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("commerce postgres: %s: %w", what, err)
	}
	return out, nil
}
