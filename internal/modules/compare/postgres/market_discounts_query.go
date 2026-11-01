package postgres

import (
	"fmt"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
)

// marketFilterClauses turns the caller's filter into AND-clauses appended to
// marketWarehouseFrom, plus their arguments.
//
// Every clause is parameterised and every one of them can only narrow the
// board: nothing here can reach is_temp_warehouse, which lives in the constant
// FROM clause and is what keeps a supplier's private upload off a public board.
func marketFilterClauses(filter compare.MarketDiscountsFilter) (extra string, args []any) {
	search, args := marketSearchFilter(filter)
	argIdx := len(args) + 1
	var where []string
	if search != "" {
		where = append(where, search)
	}

	if sup := strings.TrimSpace(filter.Supplier); sup != "" {
		where = append(where, fmt.Sprintf("TRIM(%s) = $%d", marketWarehouseSupplierSQL, argIdx))
		args = append(args, sup)
		argIdx++
	}

	if filter.DateFrom != "" {
		where = append(where, fmt.Sprintf("f.created_at >= ($%d::date::timestamp AT TIME ZONE 'Africa/Cairo')", argIdx))
		args = append(args, filter.DateFrom)
		argIdx++
	}
	if filter.DateTo != "" {
		where = append(where, fmt.Sprintf("f.created_at < (($%d::date + 1)::timestamp AT TIME ZONE 'Africa/Cairo')", argIdx))
		args = append(args, filter.DateTo)
		argIdx++
	}
	if filter.MinPrice != nil {
		where = append(where, fmt.Sprintf("r.price >= $%d", argIdx))
		args = append(args, filter.MinPrice.String())
		argIdx++
	}
	if filter.MaxPrice != nil {
		where = append(where, fmt.Sprintf("r.price <= $%d", argIdx))
		args = append(args, filter.MaxPrice.String())
		argIdx++
	}
	if filter.MinDiscount != nil {
		where = append(where, fmt.Sprintf("(%s) >= $%d", marketWarehouseDiscountSQL, argIdx))
		args = append(args, *filter.MinDiscount)
		argIdx++
	}
	if filter.MaxDiscount != nil {
		where = append(where, fmt.Sprintf("(%s) <= $%d", marketWarehouseDiscountSQL, argIdx))
		args = append(args, *filter.MaxDiscount)
	}

	if len(where) > 0 {
		extra = "\n\t\t  AND " + strings.Join(where, "\n\t\t  AND ")
	}
	return extra, args
}

// buildMarketDiscountsCountQuery counts the rows the listing would page
// through, using the same clauses the listing does.
func buildMarketDiscountsCountQuery(filter compare.MarketDiscountsFilter) (string, []any) {
	extra, args := marketFilterClauses(filter)
	return "SELECT COUNT(*)" + marketWarehouseFrom + extra + ";", args
}

// Building one page of خصومات السوق العامة.
//
// The source clause is a constant spliced in whole (marketWarehouseFrom), never
// assembled from anything the caller sent. Filters can only ever append to it,
// so is_temp_warehouse = TRUE cannot be relaxed by a request parameter — which
// is the property that keeps a supplier's own Compare Tool upload off a board
// every other supplier reads.
//
// The listing and the count are built from ONE clause builder
// (marketFilterClauses) rather than from two copies of the same conditions.
// They used to be a single statement carrying COUNT(*) OVER(), which kept them
// in step by construction; splitting them is what made the page fast, and this
// is what keeps the split honest. A count that filters differently from the
// rows it counts is a pager that runs off the end of the results.
func buildMarketDiscountsQuery(filter compare.MarketDiscountsFilter) (sql string, args []any, page, limit int) {
	limit = filter.Limit
	if limit != 24 && limit != 48 && limit != 96 {
		limit = 24
	}
	page = filter.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	extra, args := marketFilterClauses(filter)
	argIdx := len(args) + 1

	// "newest" is the warehouse's upload date, and the row carries it as
	// file_uploaded_at (migration 246) so that this ordering can be an index
	// scan. It used to order by f.created_at across the join — correct, and
	// 118 ms of sorting per page view, because no index on compare.file_rows can
	// order it by a column that lives on compare.files.
	//
	// "oldest" is the same index read backwards. The NULLS placements are
	// written out on both sides because that is what makes the backward scan an
	// exact match: DESC NULLS LAST reversed is ASC NULLS FIRST.
	orderBy := "r.file_uploaded_at DESC NULLS LAST, r.id DESC"
	switch filter.SortBy {
	case "oldest":
		orderBy = "r.file_uploaded_at ASC NULLS FIRST, r.id ASC"
	case "price_asc":
		orderBy = fmt.Sprintf("r.price ASC, (%s) DESC, r.id DESC", marketWarehouseDiscountSQL)
	case "price_desc":
		orderBy = fmt.Sprintf("r.price DESC, (%s) DESC, r.id DESC", marketWarehouseDiscountSQL)
	case "discount_asc":
		orderBy = fmt.Sprintf("(%s) ASC, (%s) ASC, r.id ASC", marketWarehouseDiscountSQL, marketWarehouseNetSQL)
	case "discount_desc", "relevance":
		// The clamped expression, not the raw column: ranking by a number the
		// card refuses to print is how 6,182 rows showing "0%" reached the top
		// of "الأعلى خصماً".
		orderBy = fmt.Sprintf("(%s) DESC, (%s) ASC, r.id DESC", marketWarehouseDiscountSQL, marketWarehouseNetSQL)
	}
	if filter.SortBy == "relevance" {
		orderBy = marketRelevanceOrder(filter) + orderBy
	}

	sql = fmt.Sprintf(`
		SELECT
			r.id,
			r.file_id,
			%s AS supplier_name,
			TRIM(r.raw_name) AS product_name,
			r.price,
			%s AS discount_percent,
			%s AS price_after_discount,
			p.id,
			f.created_at AS uploaded_at,
			COALESCE(p.name->>'ar', ''), COALESCE(p.name->>'en', '')
		%s%s
		ORDER BY %s
		LIMIT $%d OFFSET $%d;`,
		marketWarehouseSupplierSQL, marketWarehouseDiscountSQL, marketWarehouseNetSQL,
		marketWarehouseFrom, extra, orderBy, argIdx, argIdx+1)

	args = append(args, limit, offset)
	return sql, args, page, limit
}
