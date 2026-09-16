package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Warehouse lifecycle and reporting queries. Kept out of repository.go to stay
// within the 400-line file limit.
//
// Every statement here relies on row-level security for tenant scoping — the
// transaction helpers set app.current_org_id, and the policies compare against
// it. A missing organization_id predicate therefore returns zero rows rather
// than another tenant's data.

// UpdateWarehouse persists mutable warehouse fields.
func (r *Repository) UpdateWarehouse(ctx context.Context, w *inventory.Warehouse) error {
	tenantOrgID, hasTenant := database.TenantFrom(ctx)
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var query string
		var args []any
		if hasTenant && tenantOrgID > 0 && !database.IsSystem(ctx) {
			query = `
				UPDATE inventory.warehouses
				SET name = $2, code = $3, address = $4, phone = $5,
				    latitude = $6, longitude = $7, is_active = $8, branch_id = $9,
				    updated_at = now()
				WHERE id = $1 AND organization_id = $10 AND deleted_at IS NULL
				RETURNING updated_at;
			`
			args = []any{
				w.ID, w.Name, w.Code, w.Address, w.Phone,
				w.Latitude, w.Longitude, w.IsActive, w.BranchID, tenantOrgID,
			}
		} else {
			query = `
				UPDATE inventory.warehouses
				SET name = $2, code = $3, address = $4, phone = $5,
				    latitude = $6, longitude = $7, is_active = $8, branch_id = $9,
				    updated_at = now()
				WHERE id = $1 AND deleted_at IS NULL
				RETURNING updated_at;
			`
			args = []any{
				w.ID, w.Name, w.Code, w.Address, w.Phone,
				w.Latitude, w.Longitude, w.IsActive, w.BranchID,
			}
		}
		err := tx.QueryRow(txCtx, query, args...).Scan(&w.UpdatedAt)

		if err != nil {
			if err == pgx.ErrNoRows {
				return apperr.NotFound("warehouse")
			}
			return fmt.Errorf("inventory postgres: update warehouse: %w", err)
		}
		return nil
	})
}

// SoftDeleteWarehouse marks a warehouse deleted, preserving its history.
//
// Hard deletion would cascade into the stock movement ledger, which is an
// append-only audit record and must survive the warehouse it describes.
func (r *Repository) SoftDeleteWarehouse(ctx context.Context, id int64) error {
	tenantOrgID, hasTenant := database.TenantFrom(ctx)
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var query string
		var args []any
		if hasTenant && tenantOrgID > 0 && !database.IsSystem(ctx) {
			query = `
				UPDATE inventory.warehouses
				SET deleted_at = now(), is_active = false, updated_at = now()
				WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL;
			`
			args = []any{id, tenantOrgID}
		} else {
			query = `
				UPDATE inventory.warehouses
				SET deleted_at = now(), is_active = false, updated_at = now()
				WHERE id = $1 AND deleted_at IS NULL;
			`
			args = []any{id}
		}
		res, err := tx.Exec(txCtx, query, args...)
		if err != nil {
			return fmt.Errorf("inventory postgres: soft delete warehouse: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("warehouse")
		}
		return nil
	})
}

// CountStockInWarehouse counts stock rows still holding quantity.
func (r *Repository) CountStockInWarehouse(ctx context.Context, warehouseID int64) (int, error) {
	var count int
	tenantOrgID, hasTenant := database.TenantFrom(ctx)
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var query string
		var args []any
		if hasTenant && tenantOrgID > 0 && !database.IsSystem(ctx) {
			query = `
				SELECT COUNT(*) FROM inventory.stocks
				WHERE warehouse_id = $1 AND organization_id = $2 AND deleted_at IS NULL AND quantity > 0;
			`
			args = []any{warehouseID, tenantOrgID}
		} else {
			query = `
				SELECT COUNT(*) FROM inventory.stocks
				WHERE warehouse_id = $1 AND deleted_at IS NULL AND quantity > 0;
			`
			args = []any{warehouseID}
		}
		return tx.QueryRow(txCtx, query, args...).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("inventory postgres: count stock in warehouse: %w", err)
	}
	return count, nil
}

// ListLowStock returns stock at or below its reorder threshold.
func (r *Repository) ListLowStock(ctx context.Context, limit, offset int) ([]*inventory.Stock, error) {
	list, _, err := r.ListLowStockWithTotal(ctx, limit, offset)
	return list, err
}

// ListLowStockWithTotal returns paginated stock at or below its reorder threshold with total count.
func (r *Repository) ListLowStockWithTotal(ctx context.Context, limit, offset int) ([]*inventory.Stock, int, error) {
	var list []*inventory.Stock
	var total int
	tenantOrgID, hasTenant := database.TenantFrom(ctx)

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var countQuery string
		var countArgs []any
		if hasTenant && tenantOrgID > 0 {
			countQuery = `
				SELECT count(*)
				FROM inventory.stocks s
				JOIN catalog.products p ON p.id = s.product_id AND p.deleted_at IS NULL
				JOIN catalog.product_variants v ON v.id = s.product_variant_id AND v.deleted_at IS NULL
				JOIN inventory.warehouses w ON w.id = s.warehouse_id AND w.deleted_at IS NULL
				WHERE s.organization_id = $1 AND s.deleted_at IS NULL AND s.quantity <= s.min_threshold;
			`
			countArgs = append(countArgs, tenantOrgID)
		} else {
			countQuery = `
				SELECT count(*)
				FROM inventory.stocks s
				JOIN catalog.products p ON p.id = s.product_id AND p.deleted_at IS NULL
				JOIN catalog.product_variants v ON v.id = s.product_variant_id AND v.deleted_at IS NULL
				JOIN inventory.warehouses w ON w.id = s.warehouse_id AND w.deleted_at IS NULL
				WHERE s.deleted_at IS NULL AND s.quantity <= s.min_threshold;
			`
		}
		if err := tx.QueryRow(txCtx, countQuery, countArgs...).Scan(&total); err != nil {
			return err
		}

		if limit <= 0 || limit > 100 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}

		var query string
		var queryArgs []any
		if hasTenant && tenantOrgID > 0 {
			query = `
				SELECT s.id, s.organization_id, s.warehouse_id, s.product_id, s.product_variant_id,
				       s.quantity, s.min_threshold, s.negotiation, s.created_at, s.updated_at, s.deleted_at
				FROM inventory.stocks s
				JOIN catalog.products p ON p.id = s.product_id AND p.deleted_at IS NULL
				JOIN catalog.product_variants v ON v.id = s.product_variant_id AND v.deleted_at IS NULL
				JOIN inventory.warehouses w ON w.id = s.warehouse_id AND w.deleted_at IS NULL
				WHERE s.organization_id = $1 AND s.deleted_at IS NULL AND s.quantity <= s.min_threshold
				ORDER BY (s.quantity - s.min_threshold) ASC, s.id ASC
				LIMIT $2 OFFSET $3;
			`
			queryArgs = append(queryArgs, tenantOrgID, limit, offset)
		} else {
			query = `
				SELECT s.id, s.organization_id, s.warehouse_id, s.product_id, s.product_variant_id,
				       s.quantity, s.min_threshold, s.negotiation, s.created_at, s.updated_at, s.deleted_at
				FROM inventory.stocks s
				JOIN catalog.products p ON p.id = s.product_id AND p.deleted_at IS NULL
				JOIN catalog.product_variants v ON v.id = s.product_variant_id AND v.deleted_at IS NULL
				JOIN inventory.warehouses w ON w.id = s.warehouse_id AND w.deleted_at IS NULL
				WHERE s.deleted_at IS NULL AND s.quantity <= s.min_threshold
				ORDER BY (s.quantity - s.min_threshold) ASC, s.id ASC
				LIMIT $1 OFFSET $2;
			`
			queryArgs = append(queryArgs, limit, offset)
		}
		rows, err := tx.Query(txCtx, query, queryArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var s inventory.Stock
			if err := rows.Scan(
				&s.ID, &s.OrganizationID, &s.WarehouseID, &s.ProductID, &s.ProductVariantID,
				&s.Quantity, &s.MinThreshold, &s.Negotiation,
				&s.CreatedAt, &s.UpdatedAt, &s.DeletedAt,
			); err != nil {
				return err
			}
			list = append(list, &s)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, fmt.Errorf("inventory postgres: list low stock: %w", err)
	}
	return list, total, nil
}

// ListMovementsByOrg returns the organisation-wide movement ledger.
func (r *Repository) ListMovementsByOrg(ctx context.Context, limit, offset int) ([]*inventory.StockMovement, error) {
	var list []*inventory.StockMovement
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, organization_id, stock_id, type, quantity_delta, balance_after,
			       details, reference_type, reference_id, user_id, created_at
			FROM inventory.stock_movements
			ORDER BY created_at DESC, id DESC
			LIMIT $1 OFFSET $2;
		`
		rows, err := tx.Query(txCtx, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m inventory.StockMovement
			if err := rows.Scan(
				&m.ID, &m.OrganizationID, &m.StockID, &m.Type, &m.QuantityDelta,
				&m.BalanceAfter, &m.Details, &m.ReferenceType, &m.ReferenceID,
				&m.UserID, &m.CreatedAt,
			); err != nil {
				return err
			}
			list = append(list, &m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("inventory postgres: list org movements: %w", err)
	}
	return list, nil
}

// ListTransfers returns warehouse transfers, optionally filtered by status.
func (r *Repository) ListTransfers(ctx context.Context, status string, limit, offset int) ([]*inventory.WarehouseTransfer, error) {
	var list []*inventory.WarehouseTransfer
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		// An empty status means "all". Passing it as a parameter rather than
		// concatenating keeps this one prepared statement instead of two.
		query := `
			SELECT id, organization_id, from_warehouse_id, to_warehouse_id,
			       product_id, product_variant_id, quantity, status,
			       initiated_by, notes, created_at, updated_at
			FROM inventory.warehouse_transfers
			WHERE ($1 = '' OR status = $1)
			ORDER BY created_at DESC, id DESC
			LIMIT $2 OFFSET $3;
		`
		rows, err := tx.Query(txCtx, query, status, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var t inventory.WarehouseTransfer
			if err := rows.Scan(
				&t.ID, &t.OrganizationID, &t.FromWarehouseID, &t.ToWarehouseID,
				&t.ProductID, &t.ProductVariantID, &t.Quantity, &t.Status,
				&t.InitiatedBy, &t.Notes, &t.CreatedAt, &t.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &t)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("inventory postgres: list transfers: %w", err)
	}
	return list, nil
}

// ListTransfersWithTotal returns warehouse transfers and total count, optionally filtered by status.
func (r *Repository) ListTransfersWithTotal(ctx context.Context, status string, limit, offset int) ([]*inventory.WarehouseTransfer, int, error) {
	var list []*inventory.WarehouseTransfer
	var total int
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		countQuery := `
			SELECT count(*)
			FROM inventory.warehouse_transfers
			WHERE ($1 = '' OR status = $1);
		`
		if err := tx.QueryRow(txCtx, countQuery, status).Scan(&total); err != nil {
			return err
		}

		if limit <= 0 || limit > 100 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}

		query := `
			SELECT id, organization_id, from_warehouse_id, to_warehouse_id,
			       product_id, product_variant_id, quantity, status,
			       initiated_by, notes, created_at, updated_at
			FROM inventory.warehouse_transfers
			WHERE ($1 = '' OR status = $1)
			ORDER BY created_at DESC, id DESC
			LIMIT $2 OFFSET $3;
		`
		rows, err := tx.Query(txCtx, query, status, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var t inventory.WarehouseTransfer
			if err := rows.Scan(
				&t.ID, &t.OrganizationID, &t.FromWarehouseID, &t.ToWarehouseID,
				&t.ProductID, &t.ProductVariantID, &t.Quantity, &t.Status,
				&t.InitiatedBy, &t.Notes, &t.CreatedAt, &t.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &t)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, fmt.Errorf("inventory postgres: list transfers: %w", err)
	}
	return list, total, nil
}
