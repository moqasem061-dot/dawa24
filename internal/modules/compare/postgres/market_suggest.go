package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// marketSuggestSample is how many matching offers the dropdown looks at before
// grouping them. It is what keeps a suggestion a lookup: a short word such as
// «يوني» matches hundreds of offers, and grouping every one of them (a full
// aggregate over the match, with normalize_arabic per row) took longer than
// the dropdown's whole budget, so every keystroke ended in "تعذّر جلب
// الاقتراحات". The sample only chooses which lines to show; the numbers on a
// product line are then read exactly, through the matched-product index.
const marketSuggestSample = 300

// buildMarketSuggestQuery picks up to limit dropdown lines for the typed text.
//
//  1. hits — at most marketSuggestSample offers matching the board's own text
//     match over the board's own source clause (so a suggestion can never name
//     a row the board would not show, in particular never a supplier's private
//     compare upload). No ORDER BY, so the scan stops as soon as it has enough.
//  2. ranked — the hits grouped into one line per product (or, for offers no
//     product was matched to, per normalised file name), ranked by whether the
//     name starts with what was typed, then by how many offers they cover.
//  3. exact — for the product lines that made the cut, the real figures over
//     every eligible offer of that product: the same rows the board shows when
//     the suggestion is picked (the product filter's own condition). Unmatched
//     lines keep the sample's figures.
//
// There is deliberately no COUNT of the board.
func buildMarketSuggestQuery(query string, limit int) (string, []any) {
	extra, args := marketFilterClauses(compare.MarketDiscountsFilter{Query: query})
	sampleArg := len(args) + 1
	args = append(args, marketSuggestSample)
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
			       (%[1]s) AS discount,
			       %[2]s AS supplier
			%[3]s%[4]s
			LIMIT $%[5]d
		),
		grouped AS (
			SELECT product_id,
			       COALESCE(MIN(name_ar), '') AS name_ar,
			       COALESCE(MIN(name_en), '') AS name_en,
			       MIN(raw_name) AS raw_name,
			       COUNT(*) AS offers,
			       COUNT(DISTINCT supplier) AS warehouses,
			       MAX(discount) AS best,
			       MIN(price) AS lowest
			FROM hits
			GROUP BY product_id,
			         CASE WHEN product_id IS NULL THEN lower(platform.normalize_arabic(raw_name)) END
		),
		ranked AS (
			SELECT g.*,
			       ROW_NUMBER() OVER (
			           ORDER BY CASE WHEN %[6]s OR %[7]s OR %[8]s THEN 0 ELSE 1 END,
			                    g.offers DESC, g.raw_name ASC
			       ) AS pos
			FROM grouped g
		),
		picked AS (
			SELECT * FROM ranked WHERE pos <= $%[9]d
		),
		exact AS (
			SELECT r.matched_product_id AS product_id,
			       COUNT(*) AS offers,
			       COUNT(DISTINCT %[2]s) AS warehouses,
			       MAX(%[1]s) AS best,
			       MIN(r.price) AS lowest
			%[3]s
			  AND r.matched_product_id IN (SELECT product_id FROM picked WHERE product_id IS NOT NULL)
			  AND p.id IS NOT NULL
			GROUP BY r.matched_product_id
		)
		SELECT t.product_id, t.name_ar, t.name_en, t.raw_name,
		       COALESCE(e.offers, t.offers), COALESCE(e.warehouses, t.warehouses),
		       COALESCE(e.best, t.best)::float8, COALESCE(e.lowest, t.lowest)
		FROM picked t
		LEFT JOIN exact e ON e.product_id = t.product_id
		ORDER BY t.pos;`,
		marketWarehouseDiscountSQL, marketWarehouseSupplierSQL, marketWarehouseFrom, extra, sampleArg,
		startsWith("g.name_ar"), startsWith("g.name_en"), startsWith("g.raw_name"),
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
