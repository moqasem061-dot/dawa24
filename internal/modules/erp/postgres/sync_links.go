package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// LoadLinksByCode reads the existing links for one chunk's codes in one query
// (plan S2 step 7 (c)), carrying the computed VariantDeleted flag so a link
// whose variant was soft-deleted routes to review instead of resurrecting it.
func (r *Repository) LoadLinksByCode(ctx context.Context, orgID, integrationID int64, codes []string) (map[string]erp.ProductLink, error) {
	out := map[string]erp.ProductLink{}
	if len(codes) == 0 {
		return out, nil
	}
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		cursor, err := tx.Query(ctx, `SELECT l.id, l.organization_id, l.integration_id, l.external_code,
			l.variant_id, l.product_id, l.tax_percent, l.link_source, l.last_hash, l.last_synced_at,
			(v.id IS NULL OR v.deleted_at IS NOT NULL) AS variant_deleted
			FROM erp.product_links l
			LEFT JOIN catalog.product_variants v ON v.id = l.variant_id
			WHERE l.organization_id = $1 AND l.integration_id = $2 AND l.external_code = ANY($3)`, orgID, integrationID, codes)
		if err != nil {
			return err
		}
		defer cursor.Close()
		for cursor.Next() {
			var link erp.ProductLink
			if err := cursor.Scan(&link.ID, &link.OrganizationID, &link.IntegrationID, &link.ExternalCode,
				&link.VariantID, &link.ProductID, &link.TaxPercent, &link.LinkSource,
				&link.LastHash, &link.LastSyncedAt, &link.VariantDeleted); err != nil {
				return err
			}
			out[link.ExternalCode] = link
		}
		return cursor.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("erp: load links: %w", err)
	}
	return out, nil
}
