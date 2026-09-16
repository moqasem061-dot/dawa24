package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// resolveCategories links products to categories by their folded name, creating
// the missing ones only when the admin allowed it.
//
// It mirrors resolveBrands exactly, and for the same reason: an import that
// runs twice must land on the same taxonomy rows, not a second copy of them.
func resolveCategories(
	ctx context.Context, tx pgx.Tx, prods []*catalog.Product, allowCreate bool,
) (int, error) {
	needed := false
	for _, p := range prods {
		if p.SourceCategory != "" && (p.CategoryID == nil || *p.CategoryID <= 0) {
			needed = true
			break
		}
	}
	if !needed {
		return 0, nil
	}

	categories, err := loadCategoryIndex(ctx, tx)
	if err != nil {
		return 0, err
	}

	created := 0
	for _, p := range prods {
		if p.SourceCategory == "" || (p.CategoryID != nil && *p.CategoryID > 0) {
			continue
		}
		key := catalog.NormalizeKey(p.SourceCategory)
		if len([]rune(key)) < 2 {
			continue
		}

		id, known := categories[key]
		if !known {
			if !allowCreate {
				continue
			}
			if id, err = insertCategory(ctx, tx, p.SourceCategory); err != nil {
				return created, err
			}
			categories[key] = id
			created++
		}
		resolved := id
		p.CategoryID = &resolved
	}
	return created, nil
}

// loadCategoryIndex reads every live category into a folded-name lookup.
func loadCategoryIndex(ctx context.Context, tx pgx.Tx) (map[string]int64, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, COALESCE(name->>'ar', ''), COALESCE(name->>'en', '')
		 FROM catalog.categories WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("catalog postgres: load categories: %w", err)
	}
	defer rows.Close()

	categories := map[string]int64{}
	for rows.Next() {
		var id int64
		var nameAR, nameEN string
		if err := rows.Scan(&id, &nameAR, &nameEN); err != nil {
			return nil, fmt.Errorf("catalog postgres: scan category: %w", err)
		}
		for _, name := range []string{nameAR, nameEN} {
			key := catalog.NormalizeKey(name)
			if key == "" {
				continue
			}
			if _, taken := categories[key]; !taken {
				categories[key] = id
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog postgres: read categories: %w", err)
	}
	return categories, nil
}

func insertCategory(ctx context.Context, tx pgx.Tx, name string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO catalog.categories (name, status)
		VALUES (jsonb_build_object('ar', $1::text, 'en', $1::text), 'active')
		RETURNING id
	`, name).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("catalog postgres: register category %q: %w", name, err)
	}
	return id, nil
}

// loadBrandIndex reads every live brand into a folded-name lookup, so the loop
// above resolves a manufacturer without a query per product.
func loadBrandIndex(ctx context.Context, tx pgx.Tx) (map[string]int64, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, COALESCE(name->>'ar', ''), COALESCE(name->>'en', '')
		 FROM catalog.brands WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("catalog postgres: load brands: %w", err)
	}
	defer rows.Close()

	brands := map[string]int64{}
	for rows.Next() {
		var id int64
		var nameAR, nameEN string
		if err := rows.Scan(&id, &nameAR, &nameEN); err != nil {
			return nil, fmt.Errorf("catalog postgres: scan brand: %w", err)
		}
		for _, name := range []string{nameAR, nameEN} {
			key := catalog.NormalizeKey(name)
			if key == "" {
				continue
			}
			if _, taken := brands[key]; !taken {
				brands[key] = id
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog postgres: read brands: %w", err)
	}
	return brands, nil
}

func insertBrand(ctx context.Context, tx pgx.Tx, name string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO catalog.brands (name, status)
		VALUES (jsonb_build_object('ar', $1::text, 'en', $1::text), 'active')
		RETURNING id
	`, name).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("catalog postgres: register brand %q: %w", name, err)
	}
	return id, nil
}

// productMatch is an incoming row tied to a catalogue row already on file.
type productMatch struct {
	id     int64
	reason catalog.MatchReason
}

// resolveExistingProducts finds, for each incoming product, the catalogue row it
// should update — matching on SKU first, then barcode, then normalised name.
//
// The lookups are set-based: three statements per organisation regardless of how
// many rows the file holds. Matching row by row would be nine thousand round
// trips for one of the real supplier files.
//
// Name matching runs platform.normalize_arabic on both sides rather than folding
// in Go and comparing. Same function, same input, so the two can never drift —
// and drift here would mean silently duplicating products whose names differ
// only by a hamza.
func resolveExistingProducts(ctx context.Context, tx pgx.Tx, prods []*catalog.Product) (map[int]productMatch, error) {
	byOrg := map[int64][]int{}
	for i, p := range prods {
		byOrg[p.OrganizationID] = append(byOrg[p.OrganizationID], i)
	}

	m := &productMatcher{ctx: ctx, tx: tx, prods: prods, matches: map[int]productMatch{}}
	for orgID, idxs := range byOrg {
		if err := m.matchOrg(orgID, idxs); err != nil {
			return nil, err
		}
	}
	return m.matches, nil
}

// productMatcher resolves incoming rows to catalogue ids one organisation at a
// time, accumulating decisions as it goes.
type productMatcher struct {
	ctx     context.Context
	tx      pgx.Tx
	prods   []*catalog.Product
	matches map[int]productMatch
}

// matchOrg applies the three strategies in strength order. SKU and barcode are
// exact identifiers; a name match is a fallback and must never override one, so
// an index already matched is left alone by every later strategy.
func (m *productMatcher) matchOrg(orgID int64, idxs []int) error {
	for _, column := range []struct {
		reason catalog.MatchReason
		name   string
		key    func(*catalog.Product) string
	}{
		{catalog.MatchSKU, "sku", func(p *catalog.Product) string { return p.SKU }},
		{catalog.MatchBarcode, "barcode", func(p *catalog.Product) string { return p.Barcode }},
	} {
		wanted := m.pending(idxs, func(p *catalog.Product) string {
			return strings.ToLower(strings.TrimSpace(column.key(p)))
		})
		if len(wanted) == 0 {
			continue
		}
		found, err := lookupByColumn(m.ctx, m.tx, orgID, column.name, keysOf(wanted))
		if err != nil {
			return err
		}
		m.record(wanted, found, column.reason)
	}

	wanted := m.pending(idxs, func(p *catalog.Product) string { return p.Name.Get(i18n.AR) })
	if len(wanted) == 0 {
		return nil
	}
	found, err := lookupByName(m.ctx, m.tx, orgID, keysOf(wanted))
	if err != nil {
		return err
	}
	m.record(wanted, found, catalog.MatchName)
	return nil
}

// pending groups the still-unmatched rows by the key one strategy looks them up
// with, so the lookup is a single set-based query rather than one per row.
func (m *productMatcher) pending(idxs []int, key func(*catalog.Product) string) map[string][]int {
	wanted := map[string][]int{}
	for _, i := range idxs {
		if _, done := m.matches[i]; done {
			continue
		}
		if k := key(m.prods[i]); strings.TrimSpace(k) != "" {
			wanted[k] = append(wanted[k], i)
		}
	}
	return wanted
}

func (m *productMatcher) record(wanted map[string][]int, found map[string]int64, reason catalog.MatchReason) {
	for k, id := range found {
		for _, i := range wanted[k] {
			if _, done := m.matches[i]; !done {
				m.matches[i] = productMatch{id: id, reason: reason}
			}
		}
	}
}

func keysOf(m map[string][]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// lookupByColumn resolves an exact identifier column to product ids. The column
// name is chosen from a fixed set in the caller, never from input.
func lookupByColumn(ctx context.Context, tx pgx.Tx, orgID int64, column string, keys []string) (map[string]int64, error) {
	query := fmt.Sprintf(`
		SELECT lower(btrim(p.%s)) AS k, min(p.id) AS id
		FROM catalog.products p
		WHERE p.organization_id = $1
		  AND p.deleted_at IS NULL
		  AND btrim(p.%s) <> ''
		  AND lower(btrim(p.%s)) = ANY($2::text[])
		GROUP BY 1
	`, column, column, column)

	rows, err := tx.Query(ctx, query, orgID, keys)
	if err != nil {
		return nil, fmt.Errorf("catalog postgres: match products by %s: %w", column, err)
	}
	defer rows.Close()

	out := make(map[string]int64, len(keys))
	for rows.Next() {
		var key string
		var id int64
		if err := rows.Scan(&key, &id); err != nil {
			return nil, fmt.Errorf("catalog postgres: scan %s match: %w", column, err)
		}
		out[key] = id
	}
	return out, rows.Err()
}

// lookupByName resolves products by Arabic name, normalised on both sides by
// the database so the folding is identical.
func lookupByName(ctx context.Context, tx pgx.Tx, orgID int64, names []string) (map[string]int64, error) {
	const query = `
		SELECT k.raw, min(p.id) AS id
		FROM unnest($2::text[]) AS k(raw)
		JOIN catalog.products p
		  ON p.organization_id = $1
		 AND p.deleted_at IS NULL
		 AND platform.normalize_arabic(p.name->>'ar') = platform.normalize_arabic(k.raw)
		WHERE platform.normalize_arabic(k.raw) <> ''
		GROUP BY k.raw
	`

	rows, err := tx.Query(ctx, query, orgID, names)
	if err != nil {
		return nil, fmt.Errorf("catalog postgres: match products by name: %w", err)
	}
	defer rows.Close()

	out := make(map[string]int64, len(names))
	for rows.Next() {
		var name string
		var id int64
		if err := rows.Scan(&name, &id); err != nil {
			return nil, fmt.Errorf("catalog postgres: scan name match: %w", err)
		}
		out[name] = id
	}
	return out, rows.Err()
}

func queueInsert(batch *pgx.Batch, p *catalog.Product) {
	batch.Queue(insertProductQuery,
		p.OrganizationID, p.CategoryID, p.BrandID, p.BranchID, p.Name, p.Description,
		p.SKU, p.Barcode, p.Price, p.Discount, p.OldPrice, p.Image, p.ImageLink,
		string(insertStatus(p)), p.IsFeatured, p.DosageForm, p.ScientificName,
		p.Pharmacology, p.Active, p.Concentration, p.Unit, p.ManufacturingCompanies,
		p.InstitutionalWorkIDs,
	)
}
