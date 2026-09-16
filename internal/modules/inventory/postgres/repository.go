package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Repository implements inventory.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a PostgreSQL inventory repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// CreateWarehouse creates a storage warehouse for an organization.
func (r *Repository) CreateWarehouse(ctx context.Context, w *inventory.Warehouse) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO inventory.warehouses (
				organization_id, branch_id, name, code, address, phone,
				latitude, longitude, is_active
			) VALUES (
				$1,
				COALESCE($2, (SELECT b.id FROM org.branches b WHERE b.organization_id = $1 AND b.deleted_at IS NULL ORDER BY b.is_main DESC, b.id ASC LIMIT 1)),
				$3, $4, $5, $6, $7, $8, $9
			)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			w.OrganizationID, w.BranchID, w.Name, w.Code, w.Address, w.Phone,
			w.Latitude, w.Longitude, w.IsActive,
		).Scan(&w.ID, &w.PublicID, &w.CreatedAt, &w.UpdatedAt)
	})
}

// GetWarehouseByID retrieves a warehouse by ID.
func (r *Repository) GetWarehouseByID(ctx context.Context, id int64) (*inventory.Warehouse, error) {
	var w inventory.Warehouse
	tenantOrgID, hasTenant := database.TenantFrom(ctx)
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var query string
		var args []any
		if hasTenant && tenantOrgID > 0 && !database.IsSystem(ctx) {
			query = `
				SELECT w.id, w.public_id, w.organization_id, w.branch_id,
				       COALESCE(b.name->>'ar', b.name->>'en', ''),
				       w.name, w.code, w.address, w.phone,
				       w.latitude, w.longitude, w.is_active, w.created_at, w.updated_at, w.deleted_at
				FROM inventory.warehouses w
				LEFT JOIN org.branches b ON b.id = w.branch_id AND b.deleted_at IS NULL
				WHERE w.id = $1 AND w.organization_id = $2 AND w.deleted_at IS NULL;
			`
			args = []any{id, tenantOrgID}
		} else {
			query = `
				SELECT w.id, w.public_id, w.organization_id, w.branch_id,
				       COALESCE(b.name->>'ar', b.name->>'en', ''),
				       w.name, w.code, w.address, w.phone,
				       w.latitude, w.longitude, w.is_active, w.created_at, w.updated_at, w.deleted_at
				FROM inventory.warehouses w
				LEFT JOIN org.branches b ON b.id = w.branch_id AND b.deleted_at IS NULL
				WHERE w.id = $1 AND w.deleted_at IS NULL;
			`
			args = []any{id}
		}
		err := tx.QueryRow(txCtx, query, args...).Scan(
			&w.ID, &w.PublicID, &w.OrganizationID, &w.BranchID, &w.BranchName, &w.Name, &w.Code,
			&w.Address, &w.Phone, &w.Latitude, &w.Longitude, &w.IsActive,
			&w.CreatedAt, &w.UpdatedAt, &w.DeletedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("warehouse")
			}
			return fmt.Errorf("inventory postgres: get warehouse: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// ListWarehouses lists all active warehouses for the current tenant.
func (r *Repository) ListWarehouses(ctx context.Context) ([]*inventory.Warehouse, error) {
	list, _, err := r.ListWarehousesWithTotal(ctx, 1000, 0)
	return list, err
}

// ListWarehousesWithTotal lists paginated active warehouses for the current tenant with total count.
func (r *Repository) ListWarehousesWithTotal(ctx context.Context, limit, offset int) ([]*inventory.Warehouse, int, error) {
	var list []*inventory.Warehouse
	var total int
	tenantOrgID, hasTenant := database.TenantFrom(ctx)

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var countQuery string
		var countArgs []any
		if hasTenant && tenantOrgID > 0 && !database.IsSystem(ctx) {
			countQuery = `SELECT count(*) FROM inventory.warehouses WHERE deleted_at IS NULL AND organization_id = $1;`
			countArgs = []any{tenantOrgID}
		} else if database.IsSystem(ctx) {
			countQuery = `SELECT count(*) FROM inventory.warehouses WHERE deleted_at IS NULL;`
		} else {
			return nil
		}

		if err := tx.QueryRow(txCtx, countQuery, countArgs...).Scan(&total); err != nil {
			return err
		}
		if total == 0 {
			return nil
		}

		if limit <= 0 || limit > 1000 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}

		var query string
		var queryArgs []any
		if hasTenant && tenantOrgID > 0 && !database.IsSystem(ctx) {
			query = `
				SELECT w.id, w.public_id, w.organization_id, w.branch_id,
				       COALESCE(b.name->>'ar', b.name->>'en', ''),
				       w.name, w.code, w.address, w.phone,
				       w.latitude, w.longitude, w.is_active, w.created_at, w.updated_at, w.deleted_at
				FROM inventory.warehouses w
				LEFT JOIN org.branches b ON b.id = w.branch_id AND b.deleted_at IS NULL
				WHERE w.deleted_at IS NULL AND w.organization_id = $1
				ORDER BY w.name ASC, w.id DESC
				LIMIT $2 OFFSET $3;
			`
			queryArgs = []any{tenantOrgID, limit, offset}
		} else {
			query = `
				SELECT w.id, w.public_id, w.organization_id, w.branch_id,
				       COALESCE(b.name->>'ar', b.name->>'en', ''),
				       w.name, w.code, w.address, w.phone,
				       w.latitude, w.longitude, w.is_active, w.created_at, w.updated_at, w.deleted_at
				FROM inventory.warehouses w
				LEFT JOIN org.branches b ON b.id = w.branch_id AND b.deleted_at IS NULL
				WHERE w.deleted_at IS NULL
				ORDER BY w.name ASC, w.id DESC
				LIMIT $1 OFFSET $2;
			`
			queryArgs = []any{limit, offset}
		}

		rows, err := tx.Query(txCtx, query, queryArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var w inventory.Warehouse
			if err := rows.Scan(
				&w.ID, &w.PublicID, &w.OrganizationID, &w.BranchID, &w.BranchName, &w.Name, &w.Code,
				&w.Address, &w.Phone, &w.Latitude, &w.Longitude, &w.IsActive,
				&w.CreatedAt, &w.UpdatedAt, &w.DeletedAt,
			); err != nil {
				return err
			}
			list = append(list, &w)
		}
		return rows.Err()
	})
	return list, total, err
}

// GetStock retrieves a stock record for a given warehouse and product variant.
func (r *Repository) GetStock(ctx context.Context, warehouseID, variantID int64) (*inventory.Stock, error) {
	var s inventory.Stock
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, organization_id, warehouse_id, product_id, product_variant_id,
			       quantity, min_threshold, negotiation, created_at, updated_at, deleted_at
			FROM inventory.stocks
			WHERE warehouse_id = $1 AND product_variant_id = $2 AND deleted_at IS NULL;
		`
		err := tx.QueryRow(txCtx, query, warehouseID, variantID).Scan(
			&s.ID, &s.OrganizationID, &s.WarehouseID, &s.ProductID, &s.ProductVariantID,
			&s.Quantity, &s.MinThreshold, &s.Negotiation, &s.CreatedAt, &s.UpdatedAt, &s.DeletedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("stock")
			}
			return fmt.Errorf("inventory postgres: get stock: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetStockByID loads one live stock row, scoped to the tenant when there is one.
func (r *Repository) GetStockByID(ctx context.Context, id int64) (*inventory.Stock, error) {
	var tenant *int64
	if orgID, ok := database.TenantFrom(ctx); ok {
		tenant = &orgID
	}
	var s inventory.Stock
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(txCtx, `
			SELECT id, organization_id, warehouse_id, product_id, product_variant_id,
			       quantity, min_threshold, negotiation, created_at, updated_at, deleted_at
			FROM inventory.stocks
			WHERE id = $1 AND deleted_at IS NULL AND ($2::bigint IS NULL OR organization_id = $2);`,
			id, tenant).Scan(
			&s.ID, &s.OrganizationID, &s.WarehouseID, &s.ProductID, &s.ProductVariantID,
			&s.Quantity, &s.MinThreshold, &s.Negotiation, &s.CreatedAt, &s.UpdatedAt, &s.DeletedAt,
		)
		if database.IsNotFound(err) {
			return apperr.NotFound("stock")
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// UpsertStock creates or updates initial stock metadata.
func (r *Repository) UpsertStock(ctx context.Context, s *inventory.Stock) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO inventory.stocks (
				organization_id, warehouse_id, product_id, product_variant_id,
				quantity, min_threshold, negotiation
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (warehouse_id, product_variant_id) DO UPDATE SET
				min_threshold = EXCLUDED.min_threshold,
				negotiation = EXCLUDED.negotiation,
				updated_at = now()
			RETURNING id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			s.OrganizationID, s.WarehouseID, s.ProductID, s.ProductVariantID,
			s.Quantity, s.MinThreshold, s.Negotiation,
		).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
	})
}

// ClearWarehouseStocks deletes all stock records for a warehouse (for clear_and_add import mode).
func (r *Repository) ClearWarehouseStocks(ctx context.Context, warehouseID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM inventory.stocks WHERE warehouse_id = $1;`
		_, err := tx.Exec(txCtx, query, warehouseID)
		return err
	})
}

// AdjustStock updates stock atomically and records an entry in the stock movements ledger.
func (r *Repository) AdjustStock(ctx context.Context, stockID int64, delta int, movement inventory.StockMovement) (*inventory.Stock, error) {
	var updatedStock inventory.Stock
	// A tenant caller may only adjust its own stock. The stock id arrives in a
	// URL, and row-level security does not stop a superuser connection, so the
	// organisation is part of the lookup rather than trusted from the caller.
	// System callers (no tenant) keep their cross-organisation reach.
	var tenant *int64
	if orgID, ok := database.TenantFrom(ctx); ok {
		tenant = &orgID
	}
	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		querySelect := `
			SELECT id, organization_id, warehouse_id, product_id, product_variant_id,
			       quantity, min_threshold, negotiation, created_at, updated_at
			FROM inventory.stocks
			WHERE id = $1 AND ($2::bigint IS NULL OR organization_id = $2)
			FOR UPDATE;
		`
		err := tx.QueryRow(txCtx, querySelect, stockID, tenant).Scan(
			&updatedStock.ID, &updatedStock.OrganizationID, &updatedStock.WarehouseID,
			&updatedStock.ProductID, &updatedStock.ProductVariantID, &updatedStock.Quantity,
			&updatedStock.MinThreshold, &updatedStock.Negotiation, &updatedStock.CreatedAt,
			&updatedStock.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("stock")
			}
			return fmt.Errorf("inventory postgres: lock stock: %w", err)
		}

		newQuantity := updatedStock.Quantity + delta
		if newQuantity < 0 {
			return apperr.Validation("stock.insufficient", "Insufficient stock quantity for this operation.", map[string]string{
				"available": strconv.Itoa(updatedStock.Quantity),
				"requested": strconv.Itoa(-delta),
			})
		}

		queryUpdate := `UPDATE inventory.stocks SET quantity = $2, updated_at = now() WHERE id = $1 RETURNING updated_at;`
		if err := tx.QueryRow(txCtx, queryUpdate, stockID, newQuantity).Scan(&updatedStock.UpdatedAt); err != nil {
			return fmt.Errorf("inventory postgres: update quantity: %w", err)
		}
		updatedStock.Quantity = newQuantity

		queryMovement := `
			INSERT INTO inventory.stock_movements (
				organization_id, stock_id, type, quantity_delta, balance_after,
				details, reference_type, reference_id, user_id
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
		`
		_, err = tx.Exec(txCtx, queryMovement,
			updatedStock.OrganizationID, stockID, string(movement.Type), delta,
			newQuantity, movement.Details, movement.ReferenceType, movement.ReferenceID, movement.UserID,
		)
		if err != nil {
			return fmt.Errorf("inventory postgres: record movement: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return &updatedStock, nil
}

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
