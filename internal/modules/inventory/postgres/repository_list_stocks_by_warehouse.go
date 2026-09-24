package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/inventory"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// ListStocksByWarehouse retrieves all stocks located in a given warehouse.
func (r *Repository) ListStocksByWarehouse(ctx context.Context, warehouseID int64) ([]*inventory.Stock, error) {
	var list []*inventory.Stock
	tenantOrgID, hasTenant := database.TenantFrom(ctx)
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var query string
		var args []any
		if hasTenant && tenantOrgID > 0 && !database.IsSystem(ctx) {
			query = `
				SELECT s.id, s.organization_id, s.warehouse_id, s.product_id, s.product_variant_id,
				       s.quantity, s.min_threshold, s.negotiation, s.created_at, s.updated_at, s.deleted_at
				FROM inventory.stocks s
				JOIN catalog.products p ON p.id = s.product_id AND p.deleted_at IS NULL
				JOIN catalog.product_variants v ON v.id = s.product_variant_id AND v.deleted_at IS NULL
				WHERE s.warehouse_id = $1 AND s.organization_id = $2 AND s.deleted_at IS NULL
				ORDER BY s.id ASC;
			`
			args = []any{warehouseID, tenantOrgID}
		} else {
			query = `
				SELECT s.id, s.organization_id, s.warehouse_id, s.product_id, s.product_variant_id,
				       s.quantity, s.min_threshold, s.negotiation, s.created_at, s.updated_at, s.deleted_at
				FROM inventory.stocks s
				JOIN catalog.products p ON p.id = s.product_id AND p.deleted_at IS NULL
				JOIN catalog.product_variants v ON v.id = s.product_variant_id AND v.deleted_at IS NULL
				WHERE s.warehouse_id = $1 AND s.deleted_at IS NULL
				ORDER BY s.id ASC;
			`
			args = []any{warehouseID}
		}
		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var s inventory.Stock
			if err := rows.Scan(
				&s.ID, &s.OrganizationID, &s.WarehouseID, &s.ProductID, &s.ProductVariantID,
				&s.Quantity, &s.MinThreshold, &s.Negotiation, &s.CreatedAt, &s.UpdatedAt, &s.DeletedAt,
			); err != nil {
				return err
			}
			list = append(list, &s)
		}
		return rows.Err()
	})
	return list, err
}

// ListDetailedStocksByWarehouse retrieves joined variant, product, and stock information for a warehouse.
func (r *Repository) ListDetailedStocksByWarehouse(ctx context.Context, warehouseID int64) ([]*inventory.DetailedWarehouseStockView, error) {
	var list []*inventory.DetailedWarehouseStockView
	tenantOrgID, hasTenant := database.TenantFrom(ctx)
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var whereClause string
		var args []any
		if hasTenant && tenantOrgID > 0 && !database.IsSystem(ctx) {
			whereClause = "WHERE s.warehouse_id = $1 AND s.organization_id = $2 AND s.deleted_at IS NULL"
			args = []any{warehouseID, tenantOrgID}
		} else {
			whereClause = "WHERE s.warehouse_id = $1 AND s.deleted_at IS NULL"
			args = []any{warehouseID}
		}
		query := fmt.Sprintf(`
			SELECT s.id, s.warehouse_id, s.organization_id, s.product_id, s.product_variant_id,
			       COALESCE(p.name->>'ar', p.name->>'en', ''),
			       COALESCE(v.name->>'ar', v.name->>'en', ''),
			       COALESCE(p.scientific_name, ''),
			       COALESCE(p.dosage_form, ''),
			       COALESCE(p.concentration, ''),
			       COALESCE(p.manufacturing_companies, p.company, ''),
			       COALESCE(v.sku, p.sku, ''),
			       COALESCE(v.barcode, p.barcode, ''),
			       COALESCE(v.batch_number, ''),
			       v.expiry_date,
			       COALESCE(v.price::text, p.price::text, '0.00'),
			       COALESCE(v.cost_price::text, '0.00'),
			       COALESCE(p.old_price::text, p.price::text, '0.00'),
			       COALESCE(v.discount::text, p.discount::text, '0.00'),
			       s.quantity,
			       s.min_threshold,
			       COALESCE(v.is_negotiable, false),
			       COALESCE(v.status, 'active'),
			       s.updated_at
			FROM inventory.stocks s
			JOIN catalog.products p ON p.id = s.product_id AND p.deleted_at IS NULL
			JOIN catalog.product_variants v ON v.id = s.product_variant_id AND v.deleted_at IS NULL
			%s
			ORDER BY s.id ASC;
		`, whereClause)
		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v inventory.DetailedWarehouseStockView
			if err := rows.Scan(
				&v.StockID, &v.WarehouseID, &v.OrganizationID, &v.ProductID, &v.ProductVariantID,
				&v.ProductName, &v.VariantName, &v.ScientificName, &v.DosageForm, &v.Concentration,
				&v.Manufacturer, &v.SKU, &v.Barcode, &v.BatchNumber,
				&v.ExpiryDate, &v.PriceStr, &v.CostPriceStr, &v.PublicPriceStr, &v.DiscountStr,
				&v.Quantity, &v.MinThreshold, &v.IsNegotiable, &v.Status, &v.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &v)
		}
		return rows.Err()
	})
	return list, err
}

// ListStocksByOrg retrieves all stocks belonging to an organization across warehouses.
func (r *Repository) ListStocksByOrg(ctx context.Context, orgID int64) ([]*inventory.Stock, error) {
	var list []*inventory.Stock
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT s.id, s.organization_id, s.warehouse_id, s.product_id, s.product_variant_id,
			       s.quantity, s.min_threshold, s.negotiation, s.created_at, s.updated_at, s.deleted_at
			FROM inventory.stocks s
			JOIN catalog.products p ON p.id = s.product_id AND p.deleted_at IS NULL
			JOIN catalog.product_variants v ON v.id = s.product_variant_id AND v.deleted_at IS NULL
			WHERE s.organization_id = $1 AND s.deleted_at IS NULL
			ORDER BY s.id ASC;
		`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var s inventory.Stock
			if err := rows.Scan(
				&s.ID, &s.OrganizationID, &s.WarehouseID, &s.ProductID, &s.ProductVariantID,
				&s.Quantity, &s.MinThreshold, &s.Negotiation, &s.CreatedAt, &s.UpdatedAt, &s.DeletedAt,
			); err != nil {
				return err
			}
			list = append(list, &s)
		}
		return rows.Err()
	})
	return list, err
}
