package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
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
	LEFT JOIN catalog.products p ON p.id = r.matched_product_id AND p.deleted_at IS NULL
		AND (r.match_state IN ('accepted', 'confirmed') OR r.match_state IS NULL)
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

// marketWarehouseDiscountSQL preserves valid imported percentage points,
// including zero and 100. Incorrect mappings must be fixed at import time.
const marketWarehouseDiscountSQL = `
 CASE WHEN COALESCE(r.discount, 0) >= 0 AND COALESCE(r.discount, 0) <= 100
      THEN COALESCE(r.discount, 0) ELSE 0 END`

// marketWarehouseNetSQL is the price after the sheet's discount.
//
// price_after_discount is trusted when the upload carried one and recomputed
// from the percentage otherwise, because the two disagree on files whose
// columns were mapped before the mapping was corrected.
const marketWarehouseNetSQL = `
	CASE
	  WHEN r.discount = 100 THEN 0
	  WHEN COALESCE(r.price_after_discount, 0) > 0 THEN r.price_after_discount
	  WHEN COALESCE(r.discount, 0) > 0 AND COALESCE(r.discount, 0) <= 100
	    THEN ROUND(r.price * (100.0 - r.discount) / 100.0, 2)
	  ELSE r.price
	END`

// MarketBoardStamp names the board's current contents, cheaply enough to read
// on every page view.
//
// It answers "has any temporary warehouse appeared, disappeared, been archived
// or been restored since this number was cached?", which is the whole of what
// can change how many rows the board holds. A price edit does not change it, on
// purpose: prices are never cached, so invalidating the count for one would only
// buy a recount of the same number.
//
// It reads compare.files — a few hundred rows — not compare.file_rows, so it
// costs a fraction of a millisecond and never touches the 100,000-row side of
// the join. compare.Service puts it in the cache key; that is the one place it
// is used.
func (r *Repository) MarketBoardStamp(ctx context.Context) (string, error) {
	const sql = `
		SELECT count(*)::text || ':' || COALESCE(max(id), 0)::text || ':' ||
		       count(*) FILTER (
		           WHERE deleted_at IS NULL AND archived_at IS NULL AND status = 'ready'
		       )::text
		FROM compare.files
		WHERE is_temp_warehouse = TRUE;`

	var stamp string
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx, sql).Scan(&stamp); err != nil {
			return fmt.Errorf("compare postgres: market board stamp: %w", err)
		}
		return nil
	})
	return stamp, err
}

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
func (r *Repository) CountMarketDiscounts(ctx context.Context, filter compare.MarketDiscountsFilter) (compare.MarketCount, error) {
	sql, args := buildMarketDiscountsCountQuery(filter)
	var count compare.MarketCount
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx, sql, args...).Scan(&count.Total, &count.MaxDiscount); err != nil {
			return fmt.Errorf("compare postgres: count market discounts: %w", err)
		}
		return nil
	})
	return count, err
}

// MarketProductNames reads a catalogue product's two names, for the board's
// "الصنف: <name>" chip. A deleted or unknown product yields empty names and no
// error: the filter is then simply a filter that matches nothing.
func (r *Repository) MarketProductNames(ctx context.Context, productID int64) (ar, en string, err error) {
	err = r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		qerr := tx.QueryRow(txCtx, `
			SELECT COALESCE(name->>'ar', ''), COALESCE(name->>'en', '')
			FROM catalog.products WHERE id = $1 AND deleted_at IS NULL`, productID).Scan(&ar, &en)
		if errors.Is(qerr, pgx.ErrNoRows) {
			return nil
		}
		return qerr
	})
	return ar, en, err
}

// ListMarketDiscounts returns one page of temporary-warehouse rows.
//
// Rows only: the total and the supplier list are read by compare.Service, which
// can cache them. This reads live, always — a price on this board is never
// served from a cache.
//
// The board is the only screen that reads compare.file_rows.file_uploaded_at
// (migration 246), and a deploy can serve traffic before that migration has run:
// the server starts as the least-privilege role, which cannot run DDL, so the
// column arrives when the deploy's migrate step applies it, not when the image
// goes live. On a database without the column this query failed with SQLSTATE
// 42703 and the page answered 503 for that whole window. So the ordering is a
// runtime choice now: the denormalised column when it is there, the pre-246 join
// on compare.files.created_at when it is not. Same rows in the same order; the
// fallback sorts, and it is noticed once and remembered.
func (r *Repository) ListMarketDiscounts(
	ctx context.Context, filter compare.MarketDiscountsFilter,
) (*compare.MarketDiscountsResult, error) {
	result, err := r.listMarketDiscounts(ctx, filter, r.marketOrdering())
	if err != nil && r.marketOrdering() == orderByUploadDate && isMissingUploadDate(err) {
		r.legacyUntil.Store(time.Now().Add(legacyOrderingWindow).UnixNano())
		slog.WarnContext(ctx,
			"market board: compare.file_rows.file_uploaded_at is missing (migration 246 has not been applied); "+
				"ordering by compare.files.created_at until it runs",
			"error", err)
		return r.listMarketDiscounts(ctx, filter, orderByWarehouseDate)
	}
	return result, err
}

// legacyOrderingWindow is how long the board stays on the fallback ordering
// before trying the denormalised column again.
//
// Long enough that a pending migration costs one failed query per window rather
// than one per page view; short enough that the board picks up the fast path on
// its own once the deploy's migrate step has run, without waiting for a restart.
const legacyOrderingWindow = 10 * time.Minute

// marketOrdering reports which chronological ordering this database can serve.
func (r *Repository) marketOrdering() marketOrdering {
	if until := r.legacyUntil.Load(); until > 0 && time.Now().UnixNano() < until {
		return orderByWarehouseDate
	}
	return orderByUploadDate
}

// isMissingUploadDate reports whether err is the database saying the
// denormalised upload date does not exist — that is, migration 246 is pending.
//
// Matched on the SQLSTATE and the column name rather than on the message alone:
// the message is translated by the server's locale, the code is not.
func isMissingUploadDate(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42703" && strings.Contains(pgErr.Message, "file_uploaded_at")
	}
	return strings.Contains(err.Error(), "file_uploaded_at")
}

func (r *Repository) listMarketDiscounts(
	ctx context.Context, filter compare.MarketDiscountsFilter, ordering marketOrdering,
) (*compare.MarketDiscountsResult, error) {
	sql, args, page, limit := buildMarketDiscountsOrderedQuery(filter, ordering)

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
				&matchedID, &item.UploadedAt, &item.CatalogNameAR, &item.CatalogNameEN,
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
