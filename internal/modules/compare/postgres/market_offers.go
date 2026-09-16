package postgres

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Loading the market for analysis, as opposed to for display.
//
// ListMarketDiscounts is a paginated feed: it clamps its limit to 100 because
// nobody should be handed fifty thousand rows in a web page. Both analytics
// screens called it with Limit: 50000 and got a hundred rows back, silently,
// and reported the result as "the market". See market_dataset.go for what that
// did to each screen.
//
// This is the other query: no pagination, no total count, no window function,
// only the columns an aggregation reads. Bounded by marketScanLimit rather than
// by a page size, so the bound is a real safety valve rather than a display
// choice that quietly became a correctness bug.

// marketScanLimit caps one analytical scan.
//
// A quarter of a million offers is far beyond the platform's present size
// (49,761 rows across 158 files) and is still only a few tens of megabytes held
// briefly. If it is ever reached the aggregation is still correct over what it
// read; it is simply no longer over everything, which is why the count comes
// back with the data and every screen prints it.
const marketScanLimit = 250000

// LoadMarketOffers reads every usable market offer for aggregation.
func (r *Repository) LoadMarketOffers(
	ctx context.Context, opts compare.MarketScanOptions,
) ([]compare.MarketOffer, error) {
	// The market for comparison is strictly compare files (is_temp_warehouse = FALSE)
	// belonging to the organization (or all compare files if no organization specified).
	// Temporary warehouses of supervisors/admins are never included here.
	const sql = `
		SELECT r.id, r.file_id, COALESCE(f.supplier_name, ''), r.raw_name,
		       COALESCE(r.sku, ''), r.price, COALESCE(r.discount, 0),
		       COALESCE(NULLIF(r.price_after_discount, 0),
		                r.price * (100.0 - COALESCE(r.discount, 0)) / 100.0),
		       r.matched_product_id
		FROM compare.file_rows r
		JOIN compare.files f ON f.id = r.file_id
		WHERE f.deleted_at IS NULL
		  AND f.status = 'ready'
		  AND f.is_temp_warehouse = FALSE
		  AND r.price > 0
		  AND ($1::bigint IS NULL OR f.organization_id = $1)
		  AND ($2::bigint IS NULL OR r.file_id <> $2)
		  AND ($4::text IS NULL OR LOWER(TRIM(f.supplier_name)) <> LOWER(TRIM($4)))
		ORDER BY r.id
		LIMIT $3;`

	var (
		orgID      *int64
		excludeF   *int64
		excludeSup *string
	)
	if opts.OrganizationID != nil && *opts.OrganizationID > 0 {
		orgID = opts.OrganizationID
	}
	if opts.ExcludeFileID > 0 {
		id := opts.ExcludeFileID
		excludeF = &id
	}
	if s := strings.TrimSpace(opts.ExcludeSupplierName); s != "" {
		excludeSup = &s
	}

	var offers []compare.MarketOffer
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, sql, orgID, excludeF, marketScanLimit, excludeSup)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var o compare.MarketOffer
			if err := rows.Scan(
				&o.RowID, &o.FileID, &o.SupplierName, &o.ProductName, &o.SKU,
				&o.Price, &o.Discount, &o.NetPrice, &o.ProductID,
			); err != nil {
				return err
			}
			offers = append(offers, o)
		}
		return rows.Err()
	})
	return offers, err
}
