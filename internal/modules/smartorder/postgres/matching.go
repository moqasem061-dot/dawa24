package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// ResolveByCodes matches SKUs and barcodes across the whole file in one query.
//
// Both parent products and variants carry codes, and a pharmacy's file may use
// either. A code that resolves to two different products is deliberately dropped
// rather than arbitrarily picked: an ambiguous code is not a match, and guessing
// is how the wrong medicine gets ordered.
func (r *Repository) ResolveByCodes(ctx context.Context, skus, barcodes []string) (map[string]int64, error) {
	if len(skus) == 0 && len(barcodes) == 0 {
		return map[string]int64{}, nil
	}
	out := make(map[string]int64, len(skus)+len(barcodes))
	ambiguous := make(map[string]bool)

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT key, product_id FROM (
				SELECT lower(trim(p.sku)) AS key, p.id AS product_id
				FROM catalog.products p
				WHERE p.deleted_at IS NULL AND p.status = 'active'
				  AND lower(trim(p.sku)) = ANY($1::text[])
				UNION ALL
				SELECT lower(trim(v.sku)), v.product_id
				FROM catalog.product_variants v
				WHERE v.deleted_at IS NULL AND v.status = 'active'
				  AND lower(trim(v.sku)) = ANY($1::text[])
				UNION ALL
				SELECT lower(trim(p.barcode)), p.id
				FROM catalog.products p
				WHERE p.deleted_at IS NULL AND p.status = 'active'
				  AND lower(trim(p.barcode)) = ANY($2::text[])
				UNION ALL
				SELECT lower(trim(v.barcode)), v.product_id
				FROM catalog.product_variants v
				WHERE v.deleted_at IS NULL AND v.status = 'active'
				  AND lower(trim(v.barcode)) = ANY($2::text[])
			) hits WHERE key <> '';`,
			lowerAll(skus), lowerAll(barcodes))
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var key string
			var productID int64
			if err := rows.Scan(&key, &productID); err != nil {
				return err
			}
			if existing, seen := out[key]; seen && existing != productID {
				ambiguous[key] = true
				continue
			}
			out[key] = productID
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	for key := range ambiguous {
		delete(out, key)
	}
	return out, nil
}

// ResolveBySaving matches against the buyer's own Saving Products list.
//
// Only entries that already carry a product_id count: an unlinked entry is the
// buyer's private label with nothing behind it, and the line falls through to
// the ordinary ladder (FR-015 scenario 2). On the live database 8,777 of 8,778
// entries are linked, which is why this tier resolves so much of a typical file.
//
// Tenant-scoped, not AsSystem: one pharmacy's private naming must never leak to
// another.
func (r *Repository) ResolveBySaving(ctx context.Context, orgID int64, names, skus []string) (map[string]int64, error) {
	if len(names) == 0 && len(skus) == 0 {
		return map[string]int64{}, nil
	}
	out := make(map[string]int64, len(names)+len(skus))

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT key, product_id FROM (
				SELECT platform.normalize_arabic(lower(trim(sp.name_product))) AS key,
				       sp.product_id
				FROM catalog.saving_products sp
				WHERE sp.organization_id = $1
				  AND sp.deleted_at IS NULL
				  AND sp.product_id IS NOT NULL
				  AND platform.normalize_arabic(lower(trim(sp.name_product))) = ANY($2::text[])
				UNION ALL
				SELECT lower(trim(sp.sku)), sp.product_id
				FROM catalog.saving_products sp
				WHERE sp.organization_id = $1
				  AND sp.deleted_at IS NULL
				  AND sp.product_id IS NOT NULL
				  AND lower(trim(sp.sku)) = ANY($3::text[])
			) hits WHERE key <> '';`,
			orgID, lowerAll(names), lowerAll(skus))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			var productID int64
			if err := rows.Scan(&key, &productID); err != nil {
				return err
			}
			out[key] = productID
		}
		return rows.Err()
	})
	return out, err
}

// ResolveByLearned applies corrections this buyer has confirmed before.
//
// This is what makes the third import of a recurring file need no manual work:
// every correction becomes a permanent answer for that buyer's own vocabulary.
func (r *Repository) ResolveByLearned(ctx context.Context, orgID int64, names []string) (map[string]int64, error) {
	if len(names) == 0 {
		return map[string]int64{}, nil
	}
	out := make(map[string]int64, len(names))

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Check match_decisions for both organization and platform decisions
		mRows, mErr := tx.Query(txCtx, `
			SELECT platform.normalize_arabic(lower(trim(d.norm_name))) AS key, d.chosen_product_id
			FROM catalog.match_decisions d
			WHERE (d.organization_id = $1 OR d.scope = 'platform')
			  AND d.chosen_product_id IS NOT NULL AND d.chosen_product_id > 0
			  AND (d.confidence >= 0.80 OR d.source = 'manual')
			  AND (platform.normalize_arabic(lower(trim(d.norm_name))) = ANY($2::text[])
			       OR ('manual:' || lower(trim(d.norm_name))) = ANY($2::text[])
			       OR d.decision_key = ANY($2::text[]))
			ORDER BY (d.organization_id = $1) ASC, (d.source = 'manual') ASC, d.confidence ASC;`,
			orgID, lowerAll(names))
		if mErr == nil {
			for mRows.Next() {
				var key string
				var productID int64
				if scanErr := mRows.Scan(&key, &productID); scanErr == nil {
					out[key] = productID
				}
			}
			mRows.Close()
		}

		// 2. Customer product mappings take highest precedence
		rows, err := tx.Query(txCtx, `
			SELECT platform.normalize_arabic(lower(trim(m.raw_name))) AS key, m.product_id
			FROM catalog.customer_product_mappings m
			WHERE m.customer_org_id = $1
			  AND m.product_id IS NOT NULL
			  AND m.is_active
			  AND platform.normalize_arabic(lower(trim(m.raw_name))) = ANY($2::text[]);`,
			orgID, lowerAll(names))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			var productID int64
			if err := rows.Scan(&key, &productID); err != nil {
				return err
			}
			out[key] = productID
		}
		return rows.Err()
	})
	return out, err
}

// ResolveByAlias applies aliases confirmed against the shared catalogue.
//
// Cross-tenant by design: an alias is knowledge about the catalogue, not about
// a buyer, and every pharmacy benefits from it. AI-derived aliases are excluded
// until a person has accepted them — AI output never becomes ground truth on its
// own.
func (r *Repository) ResolveByAlias(ctx context.Context, names []string) (map[string]int64, error) {
	if len(names) == 0 {
		return map[string]int64{}, nil
	}
	out := make(map[string]int64, len(names))

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT a.alias, a.product_id
			FROM catalog.product_aliases a
			WHERE a.source <> 'ai_confirmed'
			  AND a.alias = ANY($1::text[]);`, lowerAll(names))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var alias string
			var productID int64
			if err := rows.Scan(&alias, &productID); err != nil {
				return err
			}
			out[alias] = productID
		}
		return rows.Err()
	})
	return out, err
}

// ResolveByExactName matches normalised names against the catalogue's own
// Arabic and English product names in one query, prioritizing the configured
// language setting.
//
// This is the single most impactful tier that was missing: when a pharmacy
// types i18n.TDefault("w4_mod.40_454") and the catalogue has exactly that, no fuzzy scorer
// or AI is needed — a normalised string comparison settles it. On the live
// catalogue of ~20,000 products, this tier alone resolves thousands of lines
// that previously fell through to the scorer or were left unmatched.
//
// The query is cross-tenant (AsSystem) because catalogue products are shared.
// Ambiguous names (mapping to more than one product) are dropped: guessing
// between two products is worse than leaving the line for a human.
func (r *Repository) ResolveByExactName(ctx context.Context, names []string, matchLang string) (map[string]int64, error) {
	if len(names) == 0 {
		return map[string]int64{}, nil
	}
	out := make(map[string]int64, len(names))
	ambiguous := make(map[string]bool)

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var sqlQuery string
		switch matchLang {
		case "ar":
			sqlQuery = `
				SELECT platform.normalize_arabic(lower(trim(p.name->>'ar'))) AS key, p.id AS product_id
				FROM catalog.products p
				WHERE p.deleted_at IS NULL AND p.status = 'active'
				  AND platform.normalize_arabic(lower(trim(p.name->>'ar'))) = ANY($1::text[]);`
		case "en":
			sqlQuery = `
				SELECT lower(trim(p.name->>'en')) AS key, p.id AS product_id
				FROM catalog.products p
				WHERE p.deleted_at IS NULL AND p.status = 'active'
				  AND lower(trim(p.name->>'en')) = ANY($1::text[]);`
		default:
			sqlQuery = `
				SELECT key, product_id FROM (
					SELECT platform.normalize_arabic(lower(trim(p.name->>'ar'))) AS key, p.id AS product_id
					FROM catalog.products p
					WHERE p.deleted_at IS NULL AND p.status = 'active'
					  AND platform.normalize_arabic(lower(trim(p.name->>'ar'))) = ANY($1::text[])
					UNION ALL
					SELECT lower(trim(p.name->>'en')) AS key, p.id AS product_id
					FROM catalog.products p
					WHERE p.deleted_at IS NULL AND p.status = 'active'
					  AND lower(trim(p.name->>'en')) = ANY($1::text[])
				) hits WHERE key <> '';`
		}
		rows, err := tx.Query(txCtx, sqlQuery, lowerAll(names))
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var key string
			var productID int64
			if err := rows.Scan(&key, &productID); err != nil {
				return err
			}
			if existing, seen := out[key]; seen && existing != productID {
				ambiguous[key] = true
				continue
			}
			out[key] = productID
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	for key := range ambiguous {
		delete(out, key)
	}
	return out, nil
}
