package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// buildMarketSuggestQuery groups the offers that match the typed text into one
// line per product (or, for offers no product was matched to, one line per
// normalised file name).
//
// The text match is the board's own (marketSearchFilter: every word must occur
// in the imported name or in either catalogue name, via the trigram indexes of
// migration 243), over the board's own source clause, so a suggestion can never
// name a row the board would not show — in particular never a supplier's
// private compare upload. There is deliberately no COUNT of the board and no
// ORDER BY over the offers: the work is the indexed match plus one hash
// aggregate over its hits.
//
// Lines are ranked by whether the name starts with what was typed, then by how
// many offers they cover.
func buildMarketSuggestQuery(query string, limit int) (string, []any) {
	extra, args := marketFilterClauses(compare.MarketDiscountsFilter{Query: query})
	limitArg := len(args) + 1
	args = append(args, limit)
	startsWith := func(expr string) string {
		return fmt.Sprintf("lower(platform.normalize_arabic(%s)) LIKE lower(platform.normalize_arabic($1)) || '%%'", expr)
	}
	sql := fmt.Sprintf(`
		WITH hits AS (
			SELECT p.id AS product_id,
			       p.name->>'ar' AS name_ar,
			       p.name->>'en' AS name_en,
			       TRIM(r.raw_name) AS raw_name,
			       r.price AS price,
			       (%s) AS discount,
			       %s AS supplier
			%s%s
		)
		SELECT product_id,
		       COALESCE(MIN(name_ar), ''), COALESCE(MIN(name_en), ''), MIN(raw_name),
		       COUNT(*), COUNT(DISTINCT supplier), MAX(discount)::float8, MIN(price)
		FROM hits
		GROUP BY product_id,
		         CASE WHEN product_id IS NULL THEN lower(platform.normalize_arabic(raw_name)) END
		ORDER BY CASE WHEN %s OR %s OR %s THEN 0 ELSE 1 END,
		         COUNT(*) DESC, MIN(raw_name) ASC
		LIMIT $%d;`,
		marketWarehouseDiscountSQL, marketWarehouseSupplierSQL, marketWarehouseFrom, extra,
		startsWith("COALESCE(MIN(name_ar), '')"), startsWith("COALESCE(MIN(name_en), '')"), startsWith("MIN(raw_name)"),
		limitArg)
	return sql, args
}

// SuggestMarketDiscounts returns the dropdown's lines. See buildMarketSuggestQuery.
func (r *Repository) SuggestMarketDiscounts(ctx context.Context, query string, limit int) ([]compare.MarketSuggestion, error) {
	sql, args := buildMarketSuggestQuery(query, limit)
	var out []compare.MarketSuggestion
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, sql, args...)
		if err != nil {
			return fmt.Errorf("compare postgres: suggest market discounts: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var s compare.MarketSuggestion
			if err := rows.Scan(&s.ProductID, &s.NameAR, &s.NameEN, &s.FileName,
				&s.Offers, &s.Warehouses, &s.BestDiscount, &s.LowestPrice); err != nil {
				return err
			}
			out = append(out, s)
		}
		return rows.Err()
	})
	return out, err
}
