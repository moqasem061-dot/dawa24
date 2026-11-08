package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// The tier lookups (plan S2 step 7 (f)): live variants only — a soft-deleted
// twin must never be auto-linked; grouped by the wire code in Go.

type skuRow struct {
	SKU string
	ID  int64
}

func groupBy(list []skuRow, keyOf func(skuRow) string, into map[string][]int64) {
	for _, row := range list {
		key := keyOf(row)
		into[key] = append(into[key], row.ID)
	}
}

func (r *Repository) FindVariantsBySKU(ctx context.Context, orgID int64, codes []string) (map[string][]int64, error) {
	var rows []skuRow
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		cursor, err := tx.Query(ctx, `SELECT sku, id FROM catalog.product_variants
			WHERE organization_id = $1 AND deleted_at IS NULL AND sku IS NOT NULL AND sku = ANY($2)
			ORDER BY id`, orgID, codes)
		if err != nil {
			return err
		}
		defer cursor.Close()
		for cursor.Next() {
			var row skuRow
			if err := cursor.Scan(&row.SKU, &row.ID); err != nil {
				return err
			}
			rows = append(rows, row)
		}
		return cursor.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("erp: find variants by sku: %w", err)
	}
	grouped := map[string][]int64{}
	groupBy(rows, func(row skuRow) string { return row.SKU }, grouped)
	return grouped, nil
}

func (r *Repository) FindVariantsByBarcode(ctx context.Context, orgID int64, barcodes []string) (map[string][]int64, error) {
	var rows []skuRow
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		cursor, err := tx.Query(ctx, `SELECT barcode, id FROM catalog.product_variants
			WHERE organization_id = $1 AND deleted_at IS NULL AND barcode IS NOT NULL AND barcode = ANY($2)
			ORDER BY id`, orgID, barcodes)
		if err != nil {
			return err
		}
		defer cursor.Close()
		for cursor.Next() {
			var row skuRow
			if err := cursor.Scan(&row.SKU, &row.ID); err != nil {
				return err
			}
			rows = append(rows, row)
		}
		return cursor.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("erp: find variants by barcode: %w", err)
	}
	grouped := map[string][]int64{}
	groupBy(rows, func(row skuRow) string { return row.SKU }, grouped)
	return grouped, nil
}

var _ = pgx.ErrNoRows
