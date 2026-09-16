package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Shared-catalogue reads and writes for the vendor import.

// ListMatchProducts loads the shared catalogue as a matching projection.
//
// One query, no joins, eleven columns. Thirty thousand rows come back in about
// a second and a half over a remote connection, which is why the index is built
// once per import and not once per row.
func (r *Repository) ListMatchProducts(ctx context.Context) ([]catalog.MatchProduct, error) {
	var out []catalog.MatchProduct
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT id,
			       COALESCE(name->>'ar', ''), COALESCE(name->>'en', ''),
			       sku, barcode, scientific_name, dosage_form, concentration,
			       unit, manufacturing_companies, price::text
			FROM catalog.products
			WHERE deleted_at IS NULL AND status <> 'rejected'
			ORDER BY id`)
		if err != nil {
			return fmt.Errorf("catalog postgres: list match products: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var p catalog.MatchProduct
			if err := rows.Scan(&p.ID, &p.NameAR, &p.NameEN, &p.SKU, &p.Barcode,
				&p.Scientific, &p.DosageForm, &p.Concentration, &p.Unit,
				&p.Manufacturer, &p.PublicPrice); err != nil {
				return fmt.Errorf("catalog postgres: scan match product: %w", err)
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CreateImportProducts registers the products a vendor's file carried that the
// shared catalogue does not have, returning their ids in the order given.
//
// One batch, one transaction, and a failure isolates to its own row for the
// same reason the variant writer's does: one unparseable product must not cost
// a vendor the other eight thousand.
func (r *Repository) CreateImportProducts(
	ctx context.Context, orgID int64, prods []*catalog.Product,
) ([]int64, error) {
	if len(prods) == 0 {
		return nil, nil
	}
	ids := make([]int64, len(prods))
	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		batch := &pgx.Batch{}
		for _, p := range prods {
			batch.Queue(`
				INSERT INTO catalog.products (
					organization_id, name, sku, barcode, price, old_price, discount,
					status, dosage_form, scientific_name, active, concentration, unit,
					manufacturing_companies, institutional_work_ids
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, '', $11, $12, $13, '{}')
				RETURNING id`,
				orgID, p.Name, p.SKU, p.Barcode, p.Price, p.OldPrice, p.Discount,
				string(p.Status), p.DosageForm, p.ScientificName, p.Concentration,
				p.Unit, p.ManufacturingCompanies)
		}

		br := tx.SendBatch(txCtx, batch)
		var rejected error
		for i := range prods {
			var id int64
			if err := br.QueryRow().Scan(&id); err != nil && rejected == nil {
				rejected = err
			}
			ids[i] = id
		}
		if closeErr := br.Close(); closeErr != nil && rejected == nil {
			rejected = closeErr
		}
		if rejected != nil {
			return errRowRejected
		}
		for i, p := range prods {
			p.ID = ids[i]
		}
		return nil
	})
	if err == nil {
		return ids, nil
	}
	if err != errRowRejected {
		return nil, fmt.Errorf("catalog postgres: create import products: %w", err)
	}
	return r.createImportProductsOneByOne(ctx, orgID, prods)
}

// createImportProductsOneByOne is the isolation path. A product that cannot be
// written leaves a zero in its slot, and the caller reports that row rather
// than linking a variant to nothing.
func (r *Repository) createImportProductsOneByOne(
	ctx context.Context, orgID int64, prods []*catalog.Product,
) ([]int64, error) {
	ids := make([]int64, len(prods))
	for i, p := range prods {
		var id int64
		err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(txCtx, `
				INSERT INTO catalog.products (
					organization_id, name, sku, barcode, price, old_price, discount,
					status, dosage_form, scientific_name, active, concentration, unit,
					manufacturing_companies, institutional_work_ids
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, '', $11, $12, $13, '{}')
				RETURNING id`,
				orgID, p.Name, p.SKU, p.Barcode, p.Price, p.OldPrice, p.Discount,
				string(p.Status), p.DosageForm, p.ScientificName, p.Concentration,
				p.Unit, p.ManufacturingCompanies).Scan(&id)
		})
		if err != nil {
			continue
		}
		ids[i] = id
		p.ID = id
	}
	return ids, nil
}

// RetireVariantsExcept takes an organisation's variants off sale except the
// ones listed, and removes them from the selected warehouse's stocks and from
// the product variants catalog.
func (r *Repository) RetireVariantsExcept(
	ctx context.Context, orgID, warehouseID int64, keep []int64,
) ([]catalog.RetiredVariant, error) {
	if keep == nil {
		keep = []int64{}
	}
	var out []catalog.RetiredVariant
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var rows pgx.Rows
		var err error
		if warehouseID > 0 {
			// Scoped strictly to the selected warehouse: variants that have active stock in this warehouse.
			rows, err = tx.Query(txCtx, `
				SELECT v.id, v.product_id
				FROM catalog.product_variants v
				JOIN inventory.stocks s ON s.product_variant_id = v.id
				WHERE v.organization_id = $1
				  AND v.deleted_at IS NULL
				  AND s.warehouse_id = $3
				  AND s.deleted_at IS NULL
				  AND NOT (v.id = ANY($2))`, orgID, keep, warehouseID)
		} else {
			rows, err = tx.Query(txCtx, `
				SELECT v.id, v.product_id
				FROM catalog.product_variants v
				WHERE v.organization_id = $1
				  AND v.deleted_at IS NULL
				  AND NOT (v.id = ANY($2))`, orgID, keep)
		}
		if err != nil {
			return fmt.Errorf("catalog postgres: select variants to retire: %w", err)
		}
		defer rows.Close()

		var ids []int64
		for rows.Next() {
			var v catalog.RetiredVariant
			if err := rows.Scan(&v.ID, &v.ProductID); err != nil {
				return fmt.Errorf("catalog postgres: scan retired variant: %w", err)
			}
			out = append(out, v)
			ids = append(ids, v.ID)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		// 1. Completely remove stocks from the selected warehouse (not just zeroing quantity)
		if warehouseID > 0 {
			_, err = tx.Exec(txCtx, `
				UPDATE inventory.stocks
				SET deleted_at = now(), quantity = 0, updated_at = now()
				WHERE warehouse_id = $1 
				  AND NOT (product_variant_id = ANY($2))
				  AND deleted_at IS NULL`,
				warehouseID, keep)
			if err != nil {
				return fmt.Errorf("catalog postgres: remove warehouse stocks: %w", err)
			}
			_, _ = tx.Exec(txCtx, `
				DELETE FROM inventory.stocks
				WHERE warehouse_id = $1 
				  AND NOT (product_variant_id = ANY($2))
				  AND NOT EXISTS (SELECT 1 FROM inventory.stock_movements sm WHERE sm.stock_id = inventory.stocks.id)`,
				warehouseID, keep)
		} else if len(ids) > 0 {
			_, err = tx.Exec(txCtx, `
				UPDATE inventory.stocks
				SET deleted_at = now(), quantity = 0, updated_at = now()
				WHERE organization_id = $1 AND product_variant_id = ANY($2) AND deleted_at IS NULL`,
				orgID, ids)
			if err != nil {
				return fmt.Errorf("catalog postgres: remove org stocks: %w", err)
			}
			_, _ = tx.Exec(txCtx, `
				DELETE FROM inventory.stocks
				WHERE organization_id = $1 AND product_variant_id = ANY($2)
				  AND NOT EXISTS (SELECT 1 FROM inventory.stock_movements sm WHERE sm.stock_id = inventory.stocks.id)`,
				orgID, ids)
		}

		// 2. Soft-delete product variants from catalog.product_variants:
		// ONLY when warehouseID <= 0 (unscoped org-wide replace).
		// When warehouseID > 0, the catalog is warehouse-scoped and MUST NOT delete
		// or soft-delete the vendor's product variants from catalog.product_variants.
		if warehouseID <= 0 && len(ids) > 0 {
			_, err = tx.Exec(txCtx, `
				UPDATE catalog.product_variants
				SET deleted_at = now(), status = 'inactive', updated_at = now()
				WHERE id = ANY($1)
				  AND organization_id = $2
				  AND deleted_at IS NULL`, ids, orgID)
			if err != nil {
				return fmt.Errorf("catalog postgres: soft delete product variants: %w", err)
			}
		}

		return nil
	})
	return out, err
}
