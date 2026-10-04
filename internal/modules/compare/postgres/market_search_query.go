package postgres

import (
	"fmt"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
)

const marketRowSearch = `lower(platform.normalize_arabic(r.raw_name))`
const marketCatalogSearch = `lower(platform.normalize_arabic(COALESCE(cp.name->>'ar', '') || ' ' || COALESCE(cp.name->>'en', '')))`

// Literal LIKE patterns use the same normalized expressions as migration 243.
// Each word must occur, regardless of word order; '%' and '_' are not operators
// when a pharmacist types them into the search box.
func marketSearchFilter(filter compare.MarketDiscountsFilter) (string, []any) {
	query := strings.TrimSpace(filter.Query)
	if query == "" {
		return "", nil
	}
	args := []any{query}
	var rowTerms, catalogTerms []string
	for _, word := range strings.Fields(query) {
		pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(word) + "%"
		args = append(args, pattern)
		param := fmt.Sprintf("lower(platform.normalize_arabic($%d))", len(args))
		rowTerms = append(rowTerms, marketRowSearch+" LIKE "+param)
		catalogTerms = append(catalogTerms, marketCatalogSearch+" LIKE "+param)
	}
	args = append(args, filter.CatalogIDs)
	clause := fmt.Sprintf(`(length($1::text) > 0 AND ((%s) OR r.matched_product_id IN (
		SELECT cp.id FROM catalog.products cp WHERE cp.deleted_at IS NULL AND (%s)
	) OR r.matched_product_id = ANY($%d::bigint[])))`, strings.Join(rowTerms, " AND "), strings.Join(catalogTerms, " AND "), len(args))
	return clause, args
}

func marketRelevanceOrder(filter compare.MarketDiscountsFilter) string {
	if strings.TrimSpace(filter.Query) == "" {
		return ""
	}
	_, args := marketSearchFilter(filter)
	return fmt.Sprintf(`CASE
		WHEN %s = lower(platform.normalize_arabic($1)) THEN 0
		WHEN strpos(%s, lower(platform.normalize_arabic($1))) = 1 THEN 1
		WHEN r.matched_product_id = ANY($%d::bigint[]) THEN 2
		ELSE 3 END,
		array_position($%d::bigint[], r.matched_product_id) ASC NULLS LAST, `,
		marketRowSearch, marketRowSearch, len(args), len(args))
}
