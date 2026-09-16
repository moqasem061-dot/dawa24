package postgres

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CreateSavingProduct inserts a saving product record.
func (r *Repository) CreateSavingProduct(ctx context.Context, sp *catalog.SavingProduct) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO catalog.saving_products (
				user_id, organization_id, product_id, name_product, sku, qty, price, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, now(), now())
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(
			txCtx, query,
			sp.UserID, sp.OrganizationID, sp.ProductID, sp.NameProduct, sp.SKU, sp.Quantity, sp.Price,
		).Scan(&sp.ID, &sp.PublicID, &sp.CreatedAt, &sp.UpdatedAt)
	})
}

// UpdateSavingProduct updates an existing saving product.
func (r *Repository) UpdateSavingProduct(ctx context.Context, sp *catalog.SavingProduct) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE catalog.saving_products
			SET name_product = $1, sku = $2, qty = $3, price = $4, product_id = $5, updated_at = now()
			WHERE id = $6 AND organization_id = $7 AND deleted_at IS NULL;
		`
		tag, err := tx.Exec(txCtx, query,
			sp.NameProduct, sp.SKU, sp.Quantity, sp.Price, sp.ProductID, sp.ID, sp.OrganizationID,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("saving_product")
		}
		return nil
	})
}

// ListSavingProductsByOrg returns saving products for an organization.
func (r *Repository) ListSavingProductsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*catalog.SavingProduct, error) {
	var list []*catalog.SavingProduct
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, user_id, organization_id, product_id, name_product,
			       COALESCE(sku, ''), qty, price, created_at, updated_at
			FROM catalog.saving_products
			WHERE organization_id = $1 AND deleted_at IS NULL
			ORDER BY id DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 1000 {
			limit = 50
		}
		rows, err := tx.Query(txCtx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var sp catalog.SavingProduct
			if err := rows.Scan(
				&sp.ID, &sp.PublicID, &sp.UserID, &sp.OrganizationID, &sp.ProductID,
				&sp.NameProduct, &sp.SKU, &sp.Quantity, &sp.Price, &sp.CreatedAt, &sp.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &sp)
		}
		return rows.Err()
	})
	return list, err
}

// ListSavingProductsEnriched returns saving products enriched with linked catalog product info, supplier counts, and filter stats.
func (r *Repository) ListSavingProductsEnriched(ctx context.Context, orgID int64, search, filter string, limit, offset int) ([]*catalog.SavingProductEnriched, *catalog.SavingProductStats, error) {
	var list []*catalog.SavingProductEnriched
	stats := &catalog.SavingProductStats{}

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Calculate overall stats for this organization
		statsQuery := `
			SELECT COUNT(*),
			       COUNT(CASE WHEN product_id IS NOT NULL THEN 1 END),
			       COUNT(CASE WHEN product_id IS NULL THEN 1 END),
			       COALESCE(SUM(qty), 0),
			       COALESCE(ROUND(SUM(qty * price), 2), 0)
			FROM catalog.saving_products
			WHERE organization_id = $1 AND deleted_at IS NULL;
		`
		if err := tx.QueryRow(txCtx, statsQuery, orgID).Scan(
			&stats.CountAll, &stats.CountLinked, &stats.CountUnlinked,
			&stats.TotalQuantity, &stats.TotalValue,
		); err != nil {
			return err
		}

		// 2. Fetch filtered rows & filtered count
		search = strings.TrimSpace(search)
		if filter == "" {
			filter = "all"
		}

		if search != "" {
			countFilteredQuery := `
				SELECT COUNT(*)
				FROM catalog.saving_products sp
				LEFT JOIN catalog.products p ON p.id = sp.product_id AND p.deleted_at IS NULL
				WHERE sp.organization_id = $1 AND sp.deleted_at IS NULL
				  AND (sp.name_product ILIKE '%' || $2 || '%' OR sp.sku ILIKE '%' || $2 || '%' OR p.name->>'ar' ILIKE '%' || $2 || '%' OR p.name->>'en' ILIKE '%' || $2 || '%')
				  AND (
				      $3 = 'all' OR
				      ($3 = 'linked' AND sp.product_id IS NOT NULL) OR
				      ($3 = 'unlinked' AND sp.product_id IS NULL)
				  );
			`
			if err := tx.QueryRow(txCtx, countFilteredQuery, orgID, search, filter).Scan(&stats.FilteredCount); err != nil {
				return err
			}
		} else {
			switch filter {
			case "linked":
				stats.FilteredCount = stats.CountLinked
			case "unlinked":
				stats.FilteredCount = stats.CountUnlinked
			default:
				stats.FilteredCount = stats.CountAll
			}
		}

		query := `
			SELECT sp.id, sp.public_id, sp.user_id, sp.organization_id, sp.product_id, sp.name_product,
			       COALESCE(sp.sku, ''), sp.qty, sp.price, sp.created_at, sp.updated_at,
			       COALESCE(p.name, '{"ar":"","en":""}'::jsonb) AS linked_name,
			       COALESCE(p.sku, '') AS linked_sku,
			       COALESCE(prov.org_count, 0) AS providing_orgs_count
			FROM catalog.saving_products sp
			LEFT JOIN catalog.products p ON p.id = sp.product_id AND p.deleted_at IS NULL
			LEFT JOIN LATERAL (
			    SELECT COUNT(DISTINCT organization_id) as org_count
			    FROM catalog.product_variants
			    WHERE product_id = sp.product_id AND deleted_at IS NULL
			) prov ON sp.product_id IS NOT NULL
			WHERE sp.organization_id = $1 AND sp.deleted_at IS NULL
			  AND ($2 = '' OR sp.name_product ILIKE '%' || $2 || '%' OR sp.sku ILIKE '%' || $2 || '%' OR p.name->>'ar' ILIKE '%' || $2 || '%' OR p.name->>'en' ILIKE '%' || $2 || '%')
			  AND (
			      $3 = 'all' OR
			      ($3 = 'linked' AND sp.product_id IS NOT NULL) OR
			      ($3 = 'unlinked' AND sp.product_id IS NULL)
			  )
			ORDER BY sp.id DESC
			LIMIT $4 OFFSET $5;
		`
		if limit <= 0 || limit > 1000 {
			limit = 50
		}
		rows, err := tx.Query(txCtx, query, orgID, search, filter, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var spe catalog.SavingProductEnriched
			if err := rows.Scan(
				&spe.ID, &spe.PublicID, &spe.UserID, &spe.OrganizationID, &spe.ProductID,
				&spe.NameProduct, &spe.SKU, &spe.Quantity, &spe.Price, &spe.CreatedAt, &spe.UpdatedAt,
				&spe.LinkedProductName, &spe.LinkedProductSKU, &spe.ProvidingOrgsCount,
			); err != nil {
				return err
			}
			if spe.Quantity > 0 && spe.Price.IsPositive() {
				spe.TotalValue = money.FromMinor(int64(spe.Quantity * float64(spe.Price.Minor())))
			}
			list = append(list, &spe)
		}
		return rows.Err()
	})

	return list, stats, err
}

// GetSavingProductByID retrieves a saving product by ID.
func (r *Repository) GetSavingProductByID(ctx context.Context, id int64) (*catalog.SavingProduct, error) {
	var sp catalog.SavingProduct
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, user_id, organization_id, product_id, name_product,
			       COALESCE(sku, ''), qty, price, created_at, updated_at
			FROM catalog.saving_products
			WHERE id = $1 AND deleted_at IS NULL;
		`
		err := tx.QueryRow(txCtx, query, id).Scan(
			&sp.ID, &sp.PublicID, &sp.UserID, &sp.OrganizationID, &sp.ProductID,
			&sp.NameProduct, &sp.SKU, &sp.Quantity, &sp.Price, &sp.CreatedAt, &sp.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("saving_product")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &sp, nil
}

// DeleteSavingProduct soft-deletes a saving product record.
func (r *Repository) DeleteSavingProduct(ctx context.Context, id, orgID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE catalog.saving_products SET deleted_at = now() WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL;`
		tag, err := tx.Exec(txCtx, query, id, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("saving_product")
		}
		return nil
	})
}

// DeleteAllSavingProducts soft-deletes all saving products for an organization.
func (r *Repository) DeleteAllSavingProducts(ctx context.Context, orgID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE catalog.saving_products SET deleted_at = now() WHERE organization_id = $1 AND deleted_at IS NULL;`
		_, err := tx.Exec(txCtx, query, orgID)
		return err
	})
}

// GetProductProviders retrieves all suppliers/organizations offering the specified master catalog product.
func (r *Repository) GetProductProviders(ctx context.Context, productID int64) ([]*catalog.ProductProviderInfo, error) {
	var providers []*catalog.ProductProviderInfo
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT pv.id, pv.organization_id,
			       COALESCE(org.trade_name, jsonb_build_object('ar', org.legal_name, 'en', org.legal_name), '{"ar":"مورد معتمد","en":"Approved Supplier"}'::jsonb) AS org_name,
			       COALESCE(pv.name, '{"ar":"العبوة القياسية","en":"Standard Pack"}'::jsonb) AS variant_name,
			       COALESCE(pv.sku, '') AS sku,
			       COALESCE(pv.unit, 'عبوة') AS unit,
			       pv.price,
			       COALESCE(pv.discount, 0) AS discount,
			       pv.status,
			       COALESCE(st.quantity, 0) AS stock_qty,
			       COALESCE(b.name, '{"ar":"الفرع الرئيسي","en":"Main Branch"}'::jsonb) AS branch_name
			FROM catalog.product_variants pv
			LEFT JOIN org.organizations org ON org.id = pv.organization_id
			LEFT JOIN org.branches b ON b.id = pv.branch_id
			LEFT JOIN LATERAL (
			    SELECT COALESCE(SUM(quantity), 0) as quantity FROM inventory.stocks WHERE product_variant_id = pv.id
			) st ON true
			WHERE pv.product_id = $1 AND pv.deleted_at IS NULL
			ORDER BY pv.price ASC;
		`
		rows, err := tx.Query(txCtx, query, productID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var p catalog.ProductProviderInfo
			var statusStr string
			if err := rows.Scan(
				&p.VariantID, &p.OrgID, &p.OrgName, &p.VariantName, &p.SKU, &p.Unit,
				&p.Price, &p.Discount, &statusStr, &p.StockQuantity, &p.BranchName,
			); err != nil {
				return err
			}
			p.Status = statusStr
			if p.Discount.IsPositive() && p.Discount.Minor() < p.Price.Minor() {
				p.PriceAfterDiscount, _ = p.Price.Sub(p.Discount)
			} else {
				p.PriceAfterDiscount = p.Price
			}
			providers = append(providers, &p)
		}
		return rows.Err()
	})
	return providers, err
}

// BatchUpsertSavingProducts inserts or updates saving products in bulk for an organization.
func (r *Repository) BatchUpsertSavingProducts(ctx context.Context, orgID int64, userID *int64, items []*catalog.SavingProduct) (added, updated int, err error) {
	err = r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		for _, item := range items {
			item.OrganizationID = orgID
			item.UserID = userID

			// Check if exists by SKU or by Name
			var existingID int64
			var checkErr error
			if item.SKU != "" {
				checkErr = tx.QueryRow(txCtx, `
					SELECT id FROM catalog.saving_products
					WHERE organization_id = $1 AND sku = $2 AND deleted_at IS NULL
					LIMIT 1;
				`, orgID, item.SKU).Scan(&existingID)
			}
			if existingID == 0 {
				checkErr = tx.QueryRow(txCtx, `
					SELECT id FROM catalog.saving_products
					WHERE organization_id = $1 AND name_product = $2 AND deleted_at IS NULL
					LIMIT 1;
				`, orgID, item.NameProduct).Scan(&existingID)
			}

			if checkErr == nil && existingID > 0 {
				// Update existing
				updateQuery := `
					UPDATE catalog.saving_products
					SET name_product = $1, sku = $2, qty = $3, price = $4,
					    product_id = COALESCE($5, product_id), updated_at = now()
					WHERE id = $6;
				`
				if _, err := tx.Exec(txCtx, updateQuery, item.NameProduct, item.SKU, item.Quantity, item.Price, item.ProductID, existingID); err != nil {
					return err
				}
				updated++
			} else {
				// Insert new
				insertQuery := `
					INSERT INTO catalog.saving_products (
						user_id, organization_id, product_id, name_product, sku, qty, price, created_at, updated_at
					) VALUES ($1, $2, $3, $4, $5, $6, $7, now(), now());
				`
				if _, err := tx.Exec(txCtx, insertQuery, userID, orgID, item.ProductID, item.NameProduct, item.SKU, item.Quantity, item.Price); err != nil {
					return err
				}
				added++
			}
		}
		return nil
	})
	return added, updated, err
}
