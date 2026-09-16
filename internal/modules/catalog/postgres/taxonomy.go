package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/cache"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

var (
	categoriesCache = cache.NewL1[[]*catalog.Category]()
	brandsCache     = cache.NewL1[[]*catalog.Brand]()
)

// CreateCategory adds a category.
func (r *Repository) CreateCategory(ctx context.Context, c *catalog.Category) error {
	defer categoriesCache.InvalidateAll()
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO catalog.categories (
				parent_id, name, description, icon, image, status, sort_order
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7
			) RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			c.ParentID, c.Name, c.Description, c.Icon, c.Image, c.Status, c.SortOrder,
		).Scan(&c.ID, &c.PublicID, &c.CreatedAt, &c.UpdatedAt)
	})
}

// ListCategories returns all categories, served from in-memory L1 cache with 60s TTL
// and direct unscoped reads to eliminate database round trips.
func (r *Repository) ListCategories(ctx context.Context) ([]*catalog.Category, error) {
	return categoriesCache.Remember("all", 60*time.Second, func() ([]*catalog.Category, error) {
		query := `
			SELECT id, public_id, parent_id, name, description, icon, image, status,
			       sort_order, created_at, updated_at, deleted_at
			FROM catalog.categories
			WHERE deleted_at IS NULL
			ORDER BY sort_order ASC, name->>'ar' ASC;
		`
		rows, err := r.db.QueryUnscoped(ctx, query)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var categories []*catalog.Category
		for rows.Next() {
			var c catalog.Category
			if err := rows.Scan(
				&c.ID, &c.PublicID, &c.ParentID, &c.Name, &c.Description, &c.Icon,
				&c.Image, &c.Status, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt, &c.DeletedAt,
			); err != nil {
				return nil, err
			}
			categories = append(categories, &c)
		}
		return categories, rows.Err()
	})
}

// ListCategoriesWithProductCount returns paginated categories with product counts and total matching count.
func (r *Repository) ListCategoriesWithProductCount(
	ctx context.Context,
	search, status string,
	limit, offset int,
) ([]*catalog.CategoryWithCount, int, error) {
	var list []*catalog.CategoryWithCount
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		countQuery := `
			SELECT count(*)
			FROM catalog.categories c
			WHERE c.deleted_at IS NULL
			  AND ($1 = '' OR c.status = $1)
			  AND ($2 = '' OR c.name->>'ar' ILIKE '%' || $2 || '%' OR c.name->>'en' ILIKE '%' || $2 || '%' OR c.description->>'ar' ILIKE '%' || $2 || '%' OR c.description->>'en' ILIKE '%' || $2 || '%');
		`
		if err := tx.QueryRow(txCtx, countQuery, status, search).Scan(&total); err != nil {
			return err
		}

		if limit <= 0 || limit > 100 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}

		query := `
			SELECT c.id, c.public_id, c.parent_id, c.name, c.description, c.icon, c.image, c.status,
			       c.sort_order, c.created_at, c.updated_at, c.deleted_at,
			       COUNT(DISTINCT p.id) AS product_count
			FROM catalog.categories c
			LEFT JOIN catalog.products p ON p.category_id = c.id AND p.deleted_at IS NULL
			WHERE c.deleted_at IS NULL
			  AND ($1 = '' OR c.status = $1)
			  AND ($2 = '' OR c.name->>'ar' ILIKE '%' || $2 || '%' OR c.name->>'en' ILIKE '%' || $2 || '%' OR c.description->>'ar' ILIKE '%' || $2 || '%' OR c.description->>'en' ILIKE '%' || $2 || '%')
			GROUP BY c.id
			ORDER BY c.sort_order ASC, c.name->>'ar' ASC, c.id ASC
			LIMIT $3 OFFSET $4;
		`
		rows, err := tx.Query(txCtx, query, status, search, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c catalog.Category
			var productCount int
			if err := rows.Scan(
				&c.ID, &c.PublicID, &c.ParentID, &c.Name, &c.Description, &c.Icon,
				&c.Image, &c.Status, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt, &c.DeletedAt,
				&productCount,
			); err != nil {
				return err
			}
			list = append(list, &catalog.CategoryWithCount{
				Category:     &c,
				ProductCount: productCount,
			})
		}
		return rows.Err()
	})
	return list, total, err
}

// CreateBrand adds a brand.
func (r *Repository) CreateBrand(ctx context.Context, b *catalog.Brand) error {
	defer brandsCache.InvalidateAll()
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO catalog.brands (name, description, image, status)
			VALUES ($1, $2, $3, $4)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query, b.Name, b.Description, b.Image, b.Status).Scan(
			&b.ID, &b.PublicID, &b.CreatedAt, &b.UpdatedAt,
		)
	})
}

// ListBrands returns all brands, served from in-memory L1 cache with 60s TTL
// and direct unscoped reads to eliminate database round trips.
func (r *Repository) ListBrands(ctx context.Context) ([]*catalog.Brand, error) {
	return brandsCache.Remember("all", 60*time.Second, func() ([]*catalog.Brand, error) {
		query := `
			SELECT id, public_id, name, description, image, status, created_at, updated_at, deleted_at
			FROM catalog.brands
			WHERE deleted_at IS NULL
			ORDER BY name->>'ar' ASC;
		`
		rows, err := r.db.QueryUnscoped(ctx, query)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var brands []*catalog.Brand
		for rows.Next() {
			var b catalog.Brand
			if err := rows.Scan(
				&b.ID, &b.PublicID, &b.Name, &b.Description, &b.Image, &b.Status,
				&b.CreatedAt, &b.UpdatedAt, &b.DeletedAt,
			); err != nil {
				return nil, err
			}
			brands = append(brands, &b)
		}
		return brands, rows.Err()
	})
}

// ListBrandsWithProductCount returns paginated brands with joined active product counts and total matching count.
func (r *Repository) ListBrandsWithProductCount(ctx context.Context, search, status string, limit, offset int) ([]*catalog.BrandWithCount, int, error) {
	var list []*catalog.BrandWithCount
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		countQuery := `
			SELECT count(*)
			FROM catalog.brands b
			WHERE b.deleted_at IS NULL
			  AND ($1 = '' OR b.name->>'ar' ILIKE '%' || $1 || '%' OR b.name->>'en' ILIKE '%' || $1 || '%' OR b.description->>'ar' ILIKE '%' || $1 || '%' OR b.description->>'en' ILIKE '%' || $1 || '%')
			  AND ($2 = '' OR $2 = 'all' OR b.status = $2);
		`
		if err := tx.QueryRow(txCtx, countQuery, search, status).Scan(&total); err != nil {
			return err
		}

		if limit <= 0 || limit > 100 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}

		query := `
			SELECT b.id, b.public_id, b.name, b.description, b.image, b.status,
			       b.created_at, b.updated_at, b.deleted_at,
			       COUNT(p.id) AS product_count
			FROM catalog.brands b
			LEFT JOIN catalog.products p ON p.brand_id = b.id AND p.deleted_at IS NULL
			WHERE b.deleted_at IS NULL
			  AND ($1 = '' OR b.name->>'ar' ILIKE '%' || $1 || '%' OR b.name->>'en' ILIKE '%' || $1 || '%' OR b.description->>'ar' ILIKE '%' || $1 || '%' OR b.description->>'en' ILIKE '%' || $1 || '%')
			  AND ($2 = '' OR $2 = 'all' OR b.status = $2)
			GROUP BY b.id
			ORDER BY b.name->>'ar' ASC, b.id DESC
			LIMIT $3 OFFSET $4;
		`
		rows, err := tx.Query(txCtx, query, search, status, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var b catalog.Brand
			var productCount int
			if err := rows.Scan(
				&b.ID, &b.PublicID, &b.Name, &b.Description, &b.Image, &b.Status,
				&b.CreatedAt, &b.UpdatedAt, &b.DeletedAt,
				&productCount,
			); err != nil {
				return err
			}
			list = append(list, &catalog.BrandWithCount{
				Brand:        &b,
				ProductCount: productCount,
			})
		}
		return rows.Err()
	})
	return list, total, err
}

// GetCategoryByID retrieves a category by ID.
func (r *Repository) GetCategoryByID(ctx context.Context, id int64) (*catalog.Category, error) {
	var c catalog.Category
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, public_id, parent_id, name, description, icon, image, status, sort_order, created_at, updated_at, deleted_at FROM catalog.categories WHERE id = $1 AND deleted_at IS NULL;`
		return tx.QueryRow(txCtx, query, id).Scan(&c.ID, &c.PublicID, &c.ParentID, &c.Name, &c.Description, &c.Icon, &c.Image, &c.Status, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt, &c.DeletedAt)
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateCategory updates category details.
func (r *Repository) UpdateCategory(ctx context.Context, c *catalog.Category) error {
	defer categoriesCache.InvalidateAll()
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE catalog.categories SET parent_id = $1, name = $2, description = $3, icon = $4, image = $5, status = $6, sort_order = $7, updated_at = now() WHERE id = $8;`
		_, err := tx.Exec(txCtx, query, c.ParentID, c.Name, c.Description, c.Icon, c.Image, c.Status, c.SortOrder, c.ID)
		return err
	})
}

// GetBrandByID retrieves a brand by ID.
func (r *Repository) GetBrandByID(ctx context.Context, id int64) (*catalog.Brand, error) {
	var b catalog.Brand
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, public_id, name, description, image, status, created_at, updated_at, deleted_at FROM catalog.brands WHERE id = $1 AND deleted_at IS NULL;`
		return tx.QueryRow(txCtx, query, id).Scan(&b.ID, &b.PublicID, &b.Name, &b.Description, &b.Image, &b.Status, &b.CreatedAt, &b.UpdatedAt, &b.DeletedAt)
	})
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// UpdateBrand updates brand details.
func (r *Repository) UpdateBrand(ctx context.Context, b *catalog.Brand) error {
	defer brandsCache.InvalidateAll()
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE catalog.brands SET name = $1, description = $2, image = $3, status = $4, updated_at = now() WHERE id = $5;`
		_, err := tx.Exec(txCtx, query, b.Name, b.Description, b.Image, b.Status, b.ID)
		return err
	})
}
