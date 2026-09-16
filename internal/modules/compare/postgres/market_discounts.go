package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// خصومات السوق العامة — the temporary warehouses, and nothing else.
//
// This screen shows the price lists moderators and administrators upload as
// **temporary warehouses**: rows of compare.file_rows belonging to a
// compare.files row with is_temp_warehouse = TRUE. That is the board's whole
// purpose — a market-wide view of what the trade is quoting — and it is what
// the people who maintain it put into it.
//
// Two things are deliberately excluded and both are load-bearing:
//
//   - **Ordinary Compare Tool uploads.** A supplier's own price list is theirs.
//     is_temp_warehouse = TRUE is the only gate, it lives in the FROM clause
//     rather than in a WHERE the caller can influence, and no request parameter
//     reaches it. TestMarketDiscountsShowsOnlyTemporaryWarehouses pins that.
//   - **Archived and deleted files.** A warehouse someone withdrew stops being
//     part of the market the moment they withdraw it.
//
// It is worth being explicit about what this board is not: it is price
// intelligence, not a catalogue. A row here is a line from a spreadsheet, not a
// variant a pharmacy can add to a basket, and it is only orderable when the
// matching pipeline has bound it to a catalogue product. The card says so
// rather than offering a button that goes nowhere.

// marketWarehouseFrom is the source, shared by the listing and the supplier
// filter so the two can never disagree about what "the market" contains.
const marketWarehouseFrom = `
	FROM compare.file_rows r
	JOIN compare.files f ON f.id = r.file_id
	WHERE f.is_temp_warehouse = TRUE
	  AND f.deleted_at IS NULL
	  AND f.archived_at IS NULL
	  AND f.status = 'ready'
	  AND r.price > 0
	  AND COALESCE(TRIM(r.raw_name), '') <> ''`

// marketWarehouseSupplierSQL is the uploading warehouse's name. A temporary
// warehouse has no organization_id — it is a moderator's upload — so there is
// no org.organizations row to read a trade name from.
const marketWarehouseSupplierSQL = `COALESCE(NULLIF(TRIM(f.supplier_name), ''), f.original_filename)`

// marketWarehouseDiscountSQL is the discount a card may show.
//
// It is clamped to 0 outside 0..100, and that is not defensive tidying: 6,182
// of the 46,862 live rows carry exactly 100.00, which is a column that was
// mapped to the wrong place rather than a supplier giving stock away. Those
// rows are what produced the "46,000 lines at 100% خصم / 0.00 ج.م" this board
// was once abandoned over.
//
// Every use goes through this — the column, the sort and the range filter — so
// the board cannot rank by a number it refuses to print. Sorting by the raw
// column put all 6,182 of them on page one of "الأعلى خصماً", each showing 0%.
const marketWarehouseDiscountSQL = `
	CASE WHEN COALESCE(r.discount, 0) > 0 AND COALESCE(r.discount, 0) < 100
	     THEN r.discount ELSE 0 END`

// marketWarehouseNetSQL is the price after the sheet's discount.
//
// price_after_discount is trusted when the upload carried one and recomputed
// from the percentage otherwise, because the two disagree on files whose
// columns were mapped before the mapping was corrected.
const marketWarehouseNetSQL = `
	CASE
	  WHEN COALESCE(r.price_after_discount, 0) > 0 THEN r.price_after_discount
	  WHEN COALESCE(r.discount, 0) > 0 AND COALESCE(r.discount, 0) < 100
	    THEN ROUND(r.price * (100.0 - r.discount) / 100.0, 2)
	  ELSE r.price
	END`

// ListDistinctSuppliers names the temporary warehouses currently on the board.
//
// It reads compare.files — 202 rows — and asks each one whether it has a row
// the board would show. It used to read the join, which meant scanning 98,370
// file_rows to return 79 names, on every single page view of the board.
// Measured on the production data, best of five with both queries warm:
// 44.2 ms for the join, 0.97 ms for this. The two return the identical set —
// 79 names either way, zero difference in either direction.
//
// The EXISTS carries the same two row conditions as marketWarehouseFrom, which
// is what idx_compare_file_rows_market_price is partial on, so the probe is an
// index lookup rather than a scan of the file's rows.
func (r *Repository) ListDistinctSuppliers(ctx context.Context) ([]string, error) {
	var suppliers []string
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		sql := `
			SELECT DISTINCT TRIM(` + marketWarehouseSupplierSQL + `) AS supplier_name
			FROM compare.files f
			WHERE f.is_temp_warehouse = TRUE
			  AND f.deleted_at IS NULL
			  AND f.archived_at IS NULL
			  AND f.status = 'ready'
			  AND TRIM(COALESCE(` + marketWarehouseSupplierSQL + `, '')) <> ''
			  AND EXISTS (
			      SELECT 1 FROM compare.file_rows r
			      WHERE r.file_id = f.id
			        AND r.price > 0
			        AND COALESCE(TRIM(r.raw_name), '') <> ''
			  )
			ORDER BY 1 ASC;`
		rows, err := tx.Query(txCtx, sql)
		if err != nil {
			return fmt.Errorf("compare postgres: list market suppliers: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var sup string
			if err := rows.Scan(&sup); err != nil {
				return err
			}
			if strings.TrimSpace(sup) != "" {
				suppliers = append(suppliers, sup)
			}
		}
		return rows.Err()
	})
	return suppliers, err
}

// CountMarketDiscounts counts the rows the board would page through.
//
// Separate from the listing on purpose. It used to be a COUNT(*) OVER() window
// inside the listing statement, which forced every one of the 98,370 matching
// rows through a window aggregate before the LIMIT 24 could discard all but
// twenty-four of them — 15.9 MB spilled to disk at work_mem = 4 MB, and 146 ms
// of database CPU for a page showing 24 cards. The listing alone, once
// idx_compare_file_rows_market_sort can serve its ORDER BY, is 0.41 ms.
//
// The count is the expensive half now, and it is the half that can be cached:
// it changes only when a moderator uploads or archives a temporary warehouse.
// compare.Service does that; see marketCountTTL.
func (r *Repository) CountMarketDiscounts(ctx context.Context, filter compare.MarketDiscountsFilter) (int64, error) {
	sql, args := buildMarketDiscountsCountQuery(filter)
	var total int64
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx, sql, args...).Scan(&total); err != nil {
			return fmt.Errorf("compare postgres: count market discounts: %w", err)
		}
		return nil
	})
	return total, err
}

// ListMarketDiscounts returns one page of temporary-warehouse rows.
//
// Rows only: the total and the supplier list are read by compare.Service, which
// can cache them. This reads live, always — a price on this board is never
// served from a cache.
func (r *Repository) ListMarketDiscounts(
	ctx context.Context, filter compare.MarketDiscountsFilter,
) (*compare.MarketDiscountsResult, error) {
	sql, args, page, limit := buildMarketDiscountsQuery(filter)

	result := &compare.MarketDiscountsResult{
		Items:      make([]*compare.MarketDiscountRow, 0, limit),
		Page:       page,
		Limit:      limit,
		TotalPages: 1,
	}

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, sql, args...)
		if err != nil {
			return fmt.Errorf("compare postgres: list market discounts: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var (
				item      compare.MarketDiscountRow
				matchedID *int64
			)
			if err := rows.Scan(
				&item.ID, &item.FileID, &item.SupplierName, &item.ProductName,
				&item.OriginalPrice, &item.DiscountPercent, &item.PriceAfterDiscount,
				&matchedID, &item.UploadedAt,
			); err != nil {
				return err
			}

			item.MatchedProductID = matchedID
			// Orderable only once the matching pipeline has bound this line to a
			// catalogue product. Until then it is a quoted price, and the card
			// says so instead of offering a button that leads nowhere.
			item.InCatalog = matchedID != nil && *matchedID > 0
			if item.OriginalPrice.Minor() > item.PriceAfterDiscount.Minor() {
				item.DiscountValue = money.FromMinor(
					item.OriginalPrice.Minor() - item.PriceAfterDiscount.Minor())
			}
			result.Items = append(result.Items, &item)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
