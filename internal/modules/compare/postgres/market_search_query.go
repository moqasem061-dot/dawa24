package postgres

import (
	"fmt"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
)

const marketRowSearch = `lower(platform.normalize_arabic(r.raw_name))`
const marketCatalogSearch = `lower(platform.normalize_arabic(COALESCE(cp.name->>'ar', '') || ' ' || COALESCE(cp.name->>'en', '')))`

// Each word must occur somewhere in the name, regardless of word order, and it
// matches as a substring of the normalised text (same expressions as migration
// 243).
//
// The wildcards sit outside platform.normalize_arabic. That function turns '%',
// '_' and '\' into spaces, so a pattern normalised whole lost its wildcards and
// matched only a name equal to the word. Normalising the word first removes
// every LIKE metacharacter, so the word needs no escaping, and a pharmacist's
// '%' or '_' is literal input.
func marketSearchFilter(filter compare.MarketDiscountsFilter) (string, []any) {
	query := strings.TrimSpace(filter.Query)
	if query == "" {
		return "", nil
	}
	args := []any{query}
	var rowTerms, catalogTerms []string
	for _, word := range strings.Fields(query) {
		args = append(args, word)
		param := fmt.Sprintf("('%%' || lower(platform.normalize_arabic($%d)) || '%%')", len(args))
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
