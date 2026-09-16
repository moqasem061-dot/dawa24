package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// ListAllSavingProductsAdmin returns all saving products across all users and organizations with search and filtering.
func (r *Repository) ListAllSavingProductsAdmin(ctx context.Context, userID *int64, orgID *int64, search string, filter string, limit, offset int) ([]*catalog.SavingProductAdminView, *catalog.SavingProductAdminStats, error) {
	var list []*catalog.SavingProductAdminView
	stats := &catalog.SavingProductAdminStats{}

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Overall stats across platform
		statsQuery := `
			SELECT 
				COUNT(sp.id),
				COUNT(DISTINCT sp.user_id),
				COUNT(DISTINCT sp.organization_id),
				COALESCE(SUM(sp.qty), 0),
				COUNT(CASE WHEN sp.product_id IS NOT NULL THEN 1 END),
				COUNT(CASE WHEN sp.product_id IS NULL THEN 1 END),
				ROUND(COALESCE(SUM(sp.qty * COALESCE(sp.price, 0)), 0), 2)::text
			FROM catalog.saving_products sp
			WHERE sp.deleted_at IS NULL;
		`
		var totalQty float64
		var totalValStr string
		if err := tx.QueryRow(txCtx, statsQuery).Scan(
			&stats.TotalProducts,
			&stats.TotalUsers,
			&stats.TotalOrganizations,
			&totalQty,
			&stats.CountLinked,
			&stats.CountUnlinked,
			&totalValStr,
		); err != nil {
			return err
		}
		stats.TotalQuantity = totalQty
		if parsedVal, err := money.Parse(totalValStr); err == nil {
			stats.TotalValue = parsedVal
		}

		// 2. Query with filters
		baseQuery := `
			SELECT 
				sp.id, sp.public_id, sp.user_id,
				COALESCE(NULLIF(u.name->>'ar', ''), NULLIF(u.name->>'en', ''), NULLIF(u.email, ''), 'مستخدم #' || COALESCE(sp.user_id, 0)) AS user_name,
				COALESCE(u.email, '') AS user_email,
				COALESCE(u.phone, '') AS user_phone,
				sp.organization_id,
				COALESCE(NULLIF(o.legal_name, ''), NULLIF(o.trade_name->>'ar', ''), NULLIF(o.trade_name->>'en', ''), 'منشأة #' || sp.organization_id) AS org_name,
				COALESCE(o.type, '') AS org_type,
				sp.product_id,
				COALESCE(p.name->>'ar', p.name->>'en', '') AS master_product_name,
				COALESCE(p.sku, '') AS master_product_sku,
				sp.name_product,
				COALESCE(sp.sku, '') AS sku,
				sp.qty,
				COALESCE(sp.price::text, '0') AS price,
				sp.created_at,
				sp.updated_at
			FROM catalog.saving_products sp
			LEFT JOIN identity.users u ON u.id = sp.user_id
			LEFT JOIN org.organizations o ON o.id = sp.organization_id
			LEFT JOIN catalog.products p ON p.id = sp.product_id
			WHERE sp.deleted_at IS NULL
		`
		var conditions []string
		var args []any
		argIdx := 1

		if userID != nil && *userID > 0 {
			conditions = append(conditions, fmt.Sprintf("sp.user_id = $%d", argIdx))
			args = append(args, *userID)
			argIdx++
		}
		if orgID != nil && *orgID > 0 {
			conditions = append(conditions, fmt.Sprintf("sp.organization_id = $%d", argIdx))
			args = append(args, *orgID)
			argIdx++
		}
		if search != "" {
			term := "%" + strings.ToLower(strings.TrimSpace(search)) + "%"
			conditions = append(conditions, fmt.Sprintf("(LOWER(sp.name_product) LIKE $%d OR LOWER(COALESCE(sp.sku, '')) LIKE $%d OR LOWER(COALESCE(u.name->>'ar', '')) LIKE $%d OR LOWER(COALESCE(u.name->>'en', '')) LIKE $%d OR LOWER(COALESCE(u.email, '')) LIKE $%d OR LOWER(COALESCE(o.legal_name, '')) LIKE $%d OR LOWER(COALESCE(o.trade_name->>'ar', '')) LIKE $%d)", argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx))
			args = append(args, term)
			argIdx++
		}
		if filter == "linked" {
			conditions = append(conditions, "sp.product_id IS NOT NULL")
		} else if filter == "unlinked" {
			conditions = append(conditions, "sp.product_id IS NULL")
		}

		if len(conditions) > 0 {
			baseQuery += " AND " + strings.Join(conditions, " AND ")
		}

		// Compute filtered count for pagination
		if len(conditions) == 0 {
			stats.FilteredCount = stats.TotalProducts
		} else {
			countQuery := `
				SELECT COUNT(sp.id)
				FROM catalog.saving_products sp
				LEFT JOIN identity.users u ON u.id = sp.user_id
				LEFT JOIN org.organizations o ON o.id = sp.organization_id
				WHERE sp.deleted_at IS NULL AND ` + strings.Join(conditions, " AND ")
			if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&stats.FilteredCount); err != nil {
				return err
			}
		}

		baseQuery += fmt.Sprintf(" ORDER BY sp.id DESC LIMIT $%d OFFSET $%d;", argIdx, argIdx+1)
		if limit <= 0 || limit > 500 {
			limit = 100
		}
		args = append(args, limit, offset)

		rows, err := tx.Query(txCtx, baseQuery, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		var runningTotalValMinor int64
		for rows.Next() {
			var v catalog.SavingProductAdminView
			if err := rows.Scan(
				&v.ID, &v.PublicID, &v.UserID,
				&v.UserName, &v.UserEmail, &v.UserPhone,
				&v.OrganizationID, &v.OrganizationName, &v.OrganizationType,
				&v.ProductID, &v.MasterProductName, &v.MasterProductSKU,
				&v.NameProduct, &v.SKU, &v.Quantity, &v.Price,
				&v.CreatedAt, &v.UpdatedAt,
			); err != nil {
				return err
			}
			valMinor := int64(v.Quantity * float64(v.Price.Minor()))
			v.TotalValue = money.FromMinor(valMinor)
			runningTotalValMinor += valMinor
			list = append(list, &v)
		}
		if stats.TotalValue.IsZero() && runningTotalValMinor > 0 {
			stats.TotalValue = money.FromMinor(runningTotalValMinor)
		}
		return rows.Err()
	})
	return list, stats, err
}

// ListAllMasterProductsForMatching retrieves all active master catalog products for high-performance in-memory matching.
//
// PERF OPTIMIZATION NOTE (Priority 10):
// ListAllMasterProductsForMatching loads the entire master catalogue into memory for in-memory matching.
// As the catalogue grows beyond 50,000+ items, this unbounded scan causes substantial memory allocation
// and query latency. Future refactor: stream rows via cursor/channel, or use database-side trigram/fulltext
// similarity matching in batches rather than full in-memory caching.
func (r *Repository) ListAllMasterProductsForMatching(ctx context.Context) ([]*catalog.CatalogMatchSource, error) {
	list := make([]*catalog.CatalogMatchSource, 0, 1024)
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, COALESCE(sku, ''), COALESCE(barcode, ''),
			       COALESCE(name->>'ar', ''), COALESCE(name->>'en', '')
			FROM catalog.products
			WHERE deleted_at IS NULL;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var it catalog.CatalogMatchSource
			if err := rows.Scan(&it.ID, &it.SKU, &it.Barcode, &it.NameAr, &it.NameEn); err != nil {
				return err
			}
			list = append(list, &it)
		}
		return rows.Err()
	})
	return list, err
}
