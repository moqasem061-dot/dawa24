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

	if filter.ProductID != nil {
		// One catalogue product, every warehouse. The two conditions are both
		// there on purpose: the first lets the planner use
		// compare_file_rows_matched_product (migration 243), the second keeps the
		// same eligibility the LEFT JOIN applies, so the rows are exactly the ones
		// the board would show under that product's name.
		where = append(where, fmt.Sprintf("r.matched_product_id = $%d AND p.id IS NOT NULL", argIdx))
		args = append(args, *filter.ProductID)
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
// through, using the same clauses the listing does, and finds the highest
// discount among them in the same scan. The maximum uses the listing's clamped
// expression, so the header never prints a number the cards refuse to print.
func buildMarketDiscountsCountQuery(filter compare.MarketDiscountsFilter) (string, []any) {
	extra, args := marketFilterClauses(filter)
	return "SELECT COUNT(*), COALESCE(MAX(" + marketWarehouseDiscountSQL + "), 0)::float8" +
		marketWarehouseFrom + extra + ";", args
}

// marketOrdering selects the column the two chronological orderings sort by.
//
// Migration 246 adds compare.file_rows.file_uploaded_at — the warehouse's upload
// date, denormalised onto the row so that "newest" is an index scan instead of a
// 118 ms sort of every row. An image can be promoted before that migration has
// run (and on this platform it can: the server starts as the least-privilege
// role, which cannot run DDL, and the migration is applied by the deploy's own
// migrate step). When it has not run, the column does not exist, the query fails
// with SQLSTATE 42703, and the board — which is the only screen that reads it —
// answers 503.
//
// So the ordering is a choice rather than a constant, and the repository makes
// that choice at runtime: the denormalised column when it is there, the
// pre-246 join to compare.files.created_at when it is not. Same rows, same
// order, one of them slower.
type marketOrdering int

const (
	// orderByUploadDate is migration 246's column: an index scan.
	orderByUploadDate marketOrdering = iota
	// orderByWarehouseDate is the pre-246 ordering, read across the join. No
	// index on compare.file_rows can serve it, so it sorts.
	orderByWarehouseDate
)

// marketChronologicalOrder is the ordering for "newest" (and any sort this
// builder does not recognise) and for "oldest".
func marketChronologicalOrder(ordering marketOrdering, sortBy string) string {
	if ordering == orderByWarehouseDate {
		if sortBy == "oldest" {
			return "f.created_at ASC, r.id ASC"
		}
		return "f.created_at DESC, r.id DESC"
	}
	// "oldest" is the same index read backwards. The NULLS placements are
	// written out on both sides because that is what makes the backward scan an
	// exact match: DESC NULLS LAST reversed is ASC NULLS FIRST.
	if sortBy == "oldest" {
		return "r.file_uploaded_at ASC NULLS FIRST, r.id ASC"
	}
	return "r.file_uploaded_at DESC NULLS LAST, r.id DESC"
}

// buildMarketDiscountsQuery builds the page query on the current schema.
func buildMarketDiscountsQuery(filter compare.MarketDiscountsFilter) (sql string, args []any, page, limit int) {
	return buildMarketDiscountsOrderedQuery(filter, orderByUploadDate)
}

// buildMarketDiscountsOrderedQuery builds one page of خصومات السوق العامة.
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
func buildMarketDiscountsOrderedQuery(
	filter compare.MarketDiscountsFilter, ordering marketOrdering,
) (sql string, args []any, page, limit int) {
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

	orderBy := marketChronologicalOrder(ordering, filter.SortBy)
	switch filter.SortBy {
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
