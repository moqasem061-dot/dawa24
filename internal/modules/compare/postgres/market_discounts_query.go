package postgres

import (
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
)

// marketFilterClauses turns the caller's filter into AND-clauses appended to
// marketWarehouseFrom, plus their arguments.
//
// Every clause is parameterised and every one of them can only narrow the
// board: nothing here can reach is_temp_warehouse, which lives in the constant
// FROM clause and is what keeps a supplier's private upload off a public board.
func marketFilterClauses(filter compare.MarketDiscountsFilter) (extra string, args []any) {
	argIdx := 1
	var where []string

	if q := strings.TrimSpace(filter.Query); q != "" {
		where = append(where, fmt.Sprintf(
			`(r.raw_name ILIKE $%d
			  OR platform.normalize_arabic(r.raw_name) ILIKE platform.normalize_arabic($%d)
			  OR COALESCE(r.normalized_name, '') ILIKE $%d
			  OR %s ILIKE $%d)`,
			argIdx, argIdx, argIdx, marketWarehouseSupplierSQL, argIdx))
		args = append(args, "%"+q+"%")
		argIdx++
	}

	if sup := strings.TrimSpace(filter.Supplier); sup != "" {
		where = append(where, fmt.Sprintf("TRIM(%s) = $%d", marketWarehouseSupplierSQL, argIdx))
		args = append(args, sup)
		argIdx++
	}

	if filter.MinPrice != nil {
		where = append(where, fmt.Sprintf("r.price >= $%d", argIdx))
		args = append(args, *filter.MinPrice)
		argIdx++
	}
	if filter.MaxPrice != nil {
		where = append(where, fmt.Sprintf("r.price <= $%d", argIdx))
		args = append(args, *filter.MaxPrice)
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
		argIdx++
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

	// The date shown on a card is the upload date, so "newest" means the most
	// recently uploaded warehouse rather than the row's own insert order.
	orderBy := "f.created_at DESC, r.id DESC"
	switch filter.SortBy {
	case "oldest":
		orderBy = "f.created_at ASC, r.id ASC"
	case "price_asc":
		orderBy = fmt.Sprintf("r.price ASC, (%s) DESC", marketWarehouseDiscountSQL)
	case "price_desc":
		orderBy = fmt.Sprintf("r.price DESC, (%s) DESC", marketWarehouseDiscountSQL)
	case "discount_asc":
		orderBy = fmt.Sprintf("(%s) ASC, (%s) ASC", marketWarehouseDiscountSQL, marketWarehouseNetSQL)
	case "discount_desc", "":
		// The clamped expression, not the raw column: ranking by a number the
		// card refuses to print is how 6,182 rows showing "0%" reached the top
		// of "الأعلى خصماً".
		orderBy = fmt.Sprintf("(%s) DESC, (%s) ASC", marketWarehouseDiscountSQL, marketWarehouseNetSQL)
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
			r.matched_product_id,
			f.created_at AS uploaded_at
		%s%s
		ORDER BY %s
		LIMIT $%d OFFSET $%d;`,
		marketWarehouseSupplierSQL, marketWarehouseDiscountSQL, marketWarehouseNetSQL,
		marketWarehouseFrom, extra, orderBy, argIdx, argIdx+1)

	args = append(args, limit, offset)
	return sql, args, page, limit
}
