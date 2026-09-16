package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ListStockMovements retrieves ledger movements for an individual stock.
func (r *Repository) ListStockMovements(ctx context.Context, stockID int64, limit int) ([]*inventory.StockMovement, error) {
	var list []*inventory.StockMovement
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, organization_id, stock_id, type, quantity_delta, balance_after,
			       details, reference_type, reference_id, user_id, created_at
			FROM inventory.stock_movements
			WHERE stock_id = $1
			ORDER BY created_at DESC
			LIMIT $2;
		`
		if limit <= 0 || limit > 100 {
			limit = 50
		}
		rows, err := tx.Query(txCtx, query, stockID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m inventory.StockMovement
			var typeStr string
			if err := rows.Scan(
				&m.ID, &m.OrganizationID, &m.StockID, &typeStr, &m.QuantityDelta,
				&m.BalanceAfter, &m.Details, &m.ReferenceType, &m.ReferenceID,
				&m.UserID, &m.CreatedAt,
			); err != nil {
				return err
			}
			m.Type = inventory.MovementType(typeStr)
			list = append(list, &m)
		}
		return rows.Err()
	})
	return list, err
}

// CreateTransfer initiates an inter-warehouse transfer.
func (r *Repository) CreateTransfer(ctx context.Context, t *inventory.WarehouseTransfer) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO inventory.warehouse_transfers (
				organization_id, from_warehouse_id, to_warehouse_id, product_id,
				product_variant_id, quantity, status, initiated_by, notes
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			t.OrganizationID, t.FromWarehouseID, t.ToWarehouseID, t.ProductID,
			t.ProductVariantID, t.Quantity, string(t.Status), t.InitiatedBy, t.Notes,
		).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	})
}

// GetTransferByID retrieves a transfer record.
func (r *Repository) GetTransferByID(ctx context.Context, id int64) (*inventory.WarehouseTransfer, error) {
	var t inventory.WarehouseTransfer
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, organization_id, from_warehouse_id, to_warehouse_id, product_id,
			       product_variant_id, quantity, status, initiated_by, notes, created_at, updated_at
			FROM inventory.warehouse_transfers
			WHERE id = $1;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&t.ID, &t.OrganizationID, &t.FromWarehouseID, &t.ToWarehouseID, &t.ProductID,
			&t.ProductVariantID, &t.Quantity, &statusStr, &t.InitiatedBy, &t.Notes,
			&t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("warehouse_transfer")
			}
			return fmt.Errorf("inventory postgres: get transfer: %w", err)
		}
		t.Status = inventory.TransferStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// UpdateTransferStatus updates the transfer lifecycle state.
func (r *Repository) UpdateTransferStatus(ctx context.Context, id int64, from, to inventory.TransferStatus) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		// The `status = $2` predicate is the compare half of the swap. Without
		// it, two concurrent receives both succeed and the destination is
		// credited twice.
		query := `
			UPDATE inventory.warehouse_transfers
			SET status = $3, updated_at = now()
			WHERE id = $1 AND status = $2;
		`
		res, err := tx.Exec(txCtx, query, id, string(from), string(to))
		if err != nil {
			return fmt.Errorf("inventory postgres: update transfer status: %w", err)
		}
		if res.RowsAffected() == 0 {
			// Either the transfer does not exist, or another request changed
			// its state first. Both mean this caller must not proceed.
			return apperr.Conflict("transfer.state_changed",
				"This transfer was already updated by someone else. Reload and try again.")
		}
		return nil
	})
}

// AvailableQuantity totals the sellable stock of one variant across a supplier's
// warehouses.
//
// It exists because catalog.ProductVariant.StockQty is a field no query ever
// populates — the column does not exist on catalog.product_variants, and stock
// lives here in inventory.stocks. Anything that trusted that field was reading a
// permanent zero, which is why the old cart's stock guard
// (`if variant.StockQty > 0 && ...`) never fired.
//
// AsSystem: a pharmacy asking whether it may buy is by definition asking about
// another tenant's stock. Only the total crosses the boundary.
func (r *Repository) AvailableQuantity(ctx context.Context, variantID int64) (int, error) {
	var qty int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const q = `
			SELECT COALESCE(SUM(s.quantity), 0)
			FROM inventory.stocks s
			WHERE s.product_variant_id = $1
			  AND s.deleted_at IS NULL;
		`
		return tx.QueryRow(txCtx, q, variantID).Scan(&qty)
	})
	if qty < 0 {
		// A negative total means stock movements are inconsistent. Report none
		// rather than a negative, so callers cannot read it as availability.
		qty = 0
	}
	return qty, err
}

// ListStocksByOrgWithTotal retrieves paginated stocks matching warehouse/search with total count.
func (r *Repository) ListStocksByOrgWithTotal(
	ctx context.Context,
	orgID int64,
	warehouseID int64,
	search string,
	limit, offset int,
) ([]*inventory.Stock, int, error) {
	var list []*inventory.Stock
	var total int

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		countQuery := `
			SELECT count(*)
			FROM inventory.stocks s
			JOIN catalog.products p ON p.id = s.product_id AND p.deleted_at IS NULL
			JOIN catalog.product_variants v ON v.id = s.product_variant_id AND v.deleted_at IS NULL
			WHERE s.organization_id = $1
			  AND s.deleted_at IS NULL
			  AND ($2 = 0 OR s.warehouse_id = $2)
			  AND ($3 = '' OR v.name->>'ar' ILIKE '%' || $3 || '%' OR v.name->>'en' ILIKE '%' || $3 || '%' OR v.sku ILIKE '%' || $3 || '%' OR v.barcode ILIKE '%' || $3 || '%' OR v.batch_number ILIKE '%' || $3 || '%');
		`
		if err := tx.QueryRow(txCtx, countQuery, orgID, warehouseID, search).Scan(&total); err != nil {
			return err
		}

		if limit <= 0 || limit > 100 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}

		query := `
			SELECT s.id, s.organization_id, s.warehouse_id, s.product_id, s.product_variant_id,
			       s.quantity, s.min_threshold, s.negotiation, s.created_at, s.updated_at, s.deleted_at
			FROM inventory.stocks s
			JOIN catalog.products p ON p.id = s.product_id AND p.deleted_at IS NULL
			JOIN catalog.product_variants v ON v.id = s.product_variant_id AND v.deleted_at IS NULL
			WHERE s.organization_id = $1
			  AND s.deleted_at IS NULL
			  AND ($2 = 0 OR s.warehouse_id = $2)
			  AND ($3 = '' OR v.name->>'ar' ILIKE '%' || $3 || '%' OR v.name->>'en' ILIKE '%' || $3 || '%' OR v.sku ILIKE '%' || $3 || '%' OR v.barcode ILIKE '%' || $3 || '%' OR v.batch_number ILIKE '%' || $3 || '%')
			ORDER BY s.id DESC
			LIMIT $4 OFFSET $5;
		`
		rows, err := tx.Query(txCtx, query, orgID, warehouseID, search, limit, offset)
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
	return list, total, err
}

// ListLowStocksByOrgWithTotal retrieves paginated low stock rows (quantity <= min_threshold)
// matching warehouse/search with total count, ordered by deficit worst first.
func (r *Repository) ListLowStocksByOrgWithTotal(
	ctx context.Context,
	orgID int64,
	warehouseID int64,
	search string,
	limit, offset int,
) ([]*inventory.Stock, int, error) {
	var list []*inventory.Stock
	var total int

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		countQuery := `
			SELECT count(*)
			FROM inventory.stocks s
			JOIN catalog.products p ON p.id = s.product_id AND p.deleted_at IS NULL
			JOIN catalog.product_variants v ON v.id = s.product_variant_id AND v.deleted_at IS NULL
			JOIN inventory.warehouses w ON w.id = s.warehouse_id AND w.deleted_at IS NULL
			WHERE s.organization_id = $1
			  AND s.deleted_at IS NULL
			  AND s.quantity <= s.min_threshold
			  AND ($2 = 0 OR s.warehouse_id = $2)
			  AND ($3 = '' OR v.name->>'ar' ILIKE '%' || $3 || '%' OR v.name->>'en' ILIKE '%' || $3 || '%' OR v.sku ILIKE '%' || $3 || '%' OR v.barcode ILIKE '%' || $3 || '%' OR v.batch_number ILIKE '%' || $3 || '%');
		`
		if err := tx.QueryRow(txCtx, countQuery, orgID, warehouseID, search).Scan(&total); err != nil {
			return err
		}

		if limit <= 0 || limit > 100 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}

		query := `
			SELECT s.id, s.organization_id, s.warehouse_id, s.product_id, s.product_variant_id,
			       s.quantity, s.min_threshold, s.negotiation, s.created_at, s.updated_at, s.deleted_at
			FROM inventory.stocks s
			JOIN catalog.products p ON p.id = s.product_id AND p.deleted_at IS NULL
			JOIN catalog.product_variants v ON v.id = s.product_variant_id AND v.deleted_at IS NULL
			JOIN inventory.warehouses w ON w.id = s.warehouse_id AND w.deleted_at IS NULL
			WHERE s.organization_id = $1
			  AND s.deleted_at IS NULL
			  AND s.quantity <= s.min_threshold
			  AND ($2 = 0 OR s.warehouse_id = $2)
			  AND ($3 = '' OR v.name->>'ar' ILIKE '%' || $3 || '%' OR v.name->>'en' ILIKE '%' || $3 || '%' OR v.sku ILIKE '%' || $3 || '%' OR v.barcode ILIKE '%' || $3 || '%' OR v.batch_number ILIKE '%' || $3 || '%')
			ORDER BY (s.quantity - s.min_threshold) ASC, s.quantity ASC, s.id ASC
			LIMIT $4 OFFSET $5;
		`
		rows, err := tx.Query(txCtx, query, orgID, warehouseID, search, limit, offset)
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
	return list, total, err
}

