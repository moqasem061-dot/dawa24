package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
)

// Bulk stock persistence for the vendor catalogue import.

// errStockRejected marks a batch in which at least one statement was refused,
// so it can be rolled back and retried row by row.
var errStockRejected = errors.New("inventory postgres: a row in the batch was rejected")

// upsertStockSQL writes one balance and records the change in the movement
// ledger, atomically.
//
// The ledger entry is why this is a CTE rather than the plain upsert it used to
// be: an import that replaces nine thousand balances wrote zero ledger rows,
// so "why did this balance change overnight?" had no answer on file. The
// previous balance is read first, the upsert runs against it, and both the
// delta and the resulting balance land in inventory.stock_movements inside the
// same transaction.
//
// The quantity expression is chosen by mode rather than parameterised, because
// Postgres cannot take an operator as a parameter and the three forms are a
// closed set defined below.
const upsertStockSQL = `
	WITH prev AS (
		SELECT id, quantity
		FROM inventory.stocks
		WHERE organization_id = $1 AND warehouse_id = $2 AND product_variant_id = $4
		ORDER BY id
		LIMIT 1
	),
	up AS (
		INSERT INTO inventory.stocks (
			organization_id, warehouse_id, product_id, product_variant_id,
			quantity, min_threshold, negotiation
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (warehouse_id, product_variant_id) DO UPDATE SET
			quantity = %s,
			min_threshold = CASE WHEN EXCLUDED.min_threshold > 0
			                     THEN EXCLUDED.min_threshold
			                     ELSE inventory.stocks.min_threshold END,
			product_id = EXCLUDED.product_id,
			deleted_at = NULL,
			updated_at = now()
		RETURNING id, quantity
	),
	movement AS (
		INSERT INTO inventory.stock_movements (
			organization_id, stock_id, type, quantity_delta, balance_after,
			details, reference_type
		)
		SELECT $1, up.id,
		       CASE
		           WHEN up.quantity - COALESCE(prev.quantity, 0) > 0 THEN 'in'
		           WHEN up.quantity - COALESCE(prev.quantity, 0) < 0 THEN 'out'
		           ELSE 'adjustment'
		       END,
		       up.quantity - COALESCE(prev.quantity, 0),
		       up.quantity,
		       'استيراد كتالوج المورّد',
		       'vendor_catalog_import'
		FROM up LEFT JOIN prev ON true
		RETURNING 1
	)
	SELECT up.id FROM up`

// quantityExpression is what an existing balance becomes under each mode.
func quantityExpression(mode inventory.StockMode, hasQuantity bool) string {
	if !hasQuantity {
		// The file said nothing about this row's balance. Whatever the mode, a
		// missing cell must not be written as a zero.
		return "inventory.stocks.quantity"
	}
	switch mode {
	case inventory.StockAdd:
		return "GREATEST(inventory.stocks.quantity + EXCLUDED.quantity, 0)"
	case inventory.StockKeep:
		return "inventory.stocks.quantity"
	default:
		return "GREATEST(EXCLUDED.quantity, 0)"
	}
}

// BulkWriteStocks writes a batch of balances, isolating a refused row rather
// than losing the batch to it.
func (r *Repository) BulkWriteStocks(
	ctx context.Context, mode inventory.StockMode, rows []inventory.StockWriteRow,
) (inventory.StockWriteResult, error) {
	result, err := r.batchStocks(ctx, mode, rows)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, errStockRejected) {
		return result, fmt.Errorf("inventory postgres: bulk write stocks: %w", err)
	}
	return r.writeStocksOneByOne(ctx, mode, rows)
}

func (r *Repository) batchStocks(
	ctx context.Context, mode inventory.StockMode, rows []inventory.StockWriteRow,
) (inventory.StockWriteResult, error) {
	var result inventory.StockWriteResult
	if len(rows) == 0 {
		return result, nil
	}

	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		batch := &pgx.Batch{}
		queued := 0
		for _, row := range rows {
			if row.Stock == nil || row.Stock.ProductVariantID <= 0 || row.Stock.WarehouseID <= 0 {
				continue
			}
			queueStock(batch, mode, row)
			queued++
		}
		if queued == 0 {
			return nil
		}

		br := tx.SendBatch(txCtx, batch)
		var rejected error
		for i := 0; i < queued; i++ {
			var id int64
			if err := br.QueryRow().Scan(&id); err != nil && rejected == nil {
				rejected = err
			}
		}
		if closeErr := br.Close(); closeErr != nil && rejected == nil {
			rejected = closeErr
		}
		if rejected != nil {
			return errStockRejected
		}
		result.Written = queued
		return nil
	})
	if err != nil {
		return inventory.StockWriteResult{}, err
	}
	return result, nil
}

func (r *Repository) writeStocksOneByOne(
	ctx context.Context, mode inventory.StockMode, rows []inventory.StockWriteRow,
) (inventory.StockWriteResult, error) {
	var result inventory.StockWriteResult
	for _, row := range rows {
		if row.Stock == nil || row.Stock.ProductVariantID <= 0 || row.Stock.WarehouseID <= 0 {
			continue
		}
		err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
			batch := &pgx.Batch{}
			queueStock(batch, mode, row)
			br := tx.SendBatch(txCtx, batch)
			var id int64
			if err := br.QueryRow().Scan(&id); err != nil {
				_ = br.Close()
				return err
			}
			return br.Close()
		})
		if err != nil {
			result.Failures = append(result.Failures, inventory.StockFailure{
				Ref:     row.Ref,
				Message: stockFailureMessage(err),
			})
			continue
		}
		result.Written++
	}
	return result, nil
}

func queueStock(batch *pgx.Batch, mode inventory.StockMode, row inventory.StockWriteRow) {
	s := row.Stock
	quantity := s.Quantity
	if quantity < 0 {
		quantity = 0
	}
	// The INSERT half of the upsert is what a variant with no balance yet gets,
	// and the mode governs it as much as it governs the UPDATE half. "Keep the
	// balance I already have" cannot mean "and take the file's figure where I
	// have none" — that is the file setting the stock, which is the one thing
	// this mode exists to prevent. Same for a row whose quantity cell was blank.
	if mode == inventory.StockKeep || !row.HasQuantity {
		quantity = 0
	}
	batch.Queue(
		fmt.Sprintf(upsertStockSQL, quantityExpression(mode, row.HasQuantity)),
		s.OrganizationID, s.WarehouseID, s.ProductID, s.ProductVariantID,
		quantity, maxInt(s.MinThreshold, 0), s.Negotiation)
}

func stockFailureMessage(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "stocks_quantity_non_negative"):
		return i18n.TDefault("w4_mod.s_402_402")
	case strings.Contains(msg, "violates foreign key"):
		return i18n.TDefault("w4_mod.s_403_403")
	case strings.Contains(msg, "duplicate key"):
		return i18n.TDefault("w4_mod.s_404_404")
	}
	return i18n.TDefault("w4_mod.w4str_226_226")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// VariantIDsInWarehouse lists the variants that already hold a balance row in
// one warehouse.
//
// It is what lets an import tell "this vendor stocks this product" from "this
// vendor stocks this product HERE". Without it, a vendor with the same product
// under two variants — two pack sizes, two batches — had the importer refuse to
// choose between them and insert a third, so the quantity in the file landed on
// a brand-new variant while the balance the pharmacy actually buys from never
// moved. The variant that already has a row in the warehouse the import is
// writing to is the one the file is talking about.
func (r *Repository) VariantIDsInWarehouse(
	ctx context.Context, warehouseID int64,
) (map[int64]bool, error) {
	out := map[int64]bool{}
	if warehouseID <= 0 {
		return out, nil
	}
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT product_variant_id
			FROM inventory.stocks
			WHERE warehouse_id = $1 AND deleted_at IS NULL`, warehouseID)
		if err != nil {
			return fmt.Errorf("inventory postgres: list warehouse variants: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			out[id] = true
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
