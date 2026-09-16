package postgres

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

// How a catalogue page is ordered.
//
// Split from repository.go for the 400-line rule.

// searchOrderPrefix and searchOrderSuffix decide where availability sits
// relative to text relevance.
//
// With no query the caller is browsing, and the only useful first page is the
// one they can buy from, so availability leads. With a query they are looking
// for a named medicine, and a relevant out-of-stock result beats an irrelevant
// in-stock one — so relevance leads and availability breaks its ties. Putting
// availability first unconditionally would have made search stop working.
func searchOrderPrefix(query, sort string) string {
	if strings.TrimSpace(query) == "" {
		if sort != "" && sort != "newest" && sort != "default" {
			return ""
		}
		return stockFirstOrder + ","
	}
	return ""
}

func searchOrderSuffix(query string) string {
	if strings.TrimSpace(query) == "" {
		return ""
	}
	return stockFirstOrder + ", "
}

// catalogOrderBy maps a whitelisted sort key onto a safe ORDER BY clause.
func catalogOrderBy(sort string) string {
	switch sort {
	case "price_asc":
		return "price ASC, id ASC"
	case "price_desc":
		return "price DESC, id DESC"
	case "newest":
		return "created_at DESC, id DESC"
	case "oldest":
		return "created_at ASC, id ASC"
	case "name_asc", "name":
		return "COALESCE(name->>'ar', name->>'en') ASC, id ASC"
	case "name_desc":
		return "COALESCE(name->>'ar', name->>'en') DESC, id DESC"
	case "scientific_asc":
		return "COALESCE(scientific_name, '') ASC, id ASC"
	case "scientific_desc":
		return "COALESCE(scientific_name, '') DESC, id DESC"
	case "dosage_asc":
		return "COALESCE(dosage_form, '') ASC, id ASC"
	case "dosage_desc":
		return "COALESCE(dosage_form, '') DESC, id DESC"
	case "category_asc":
		return "category_id ASC NULLS LAST, id ASC"
	case "category_desc":
		return "category_id DESC NULLS LAST, id DESC"
	case "mfg_asc":
		return "COALESCE(manufacturing_companies, '') ASC, id ASC"
	case "mfg_desc":
		return "COALESCE(manufacturing_companies, '') DESC, id DESC"
	case "status_asc":
		return "status ASC, id ASC"
	case "status_desc":
		return "status DESC, id DESC"
	default:
		return "sold_times DESC, created_at DESC"
	}
}

// CountProductsByOrg returns how many products an organization has in a status.
//
// The supplier dashboard previously derived this by iterating a page capped at
// 100 rows, so a supplier with more products than that saw "100" and no way to
// tell it was a ceiling rather than a count.
func (r *Repository) CountProductsByOrg(ctx context.Context, orgID int64, status string) (int, error) {
	var total int
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT COUNT(*) FROM catalog.products
			WHERE organization_id = $1
			  AND deleted_at IS NULL
			  AND ($2::text = '' OR status = $2);
		`
		return tx.QueryRow(txCtx, query, orgID, status).Scan(&total)
	})
	return total, err
}
