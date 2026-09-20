package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
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

