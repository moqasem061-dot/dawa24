package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// CountOrganizations returns how many organizations match the filter.
//
// The admin dashboard previously derived this from len() of a page capped at
// 100 rows, so every figure on it stopped counting at 100 and quietly
// under-reported from the hundred-and-first organization onward. A count
// belongs in SQL.
func (r *Repository) CountOrganizations(
	ctx context.Context,
	orgType *org.OrganizationType,
	status *org.OrganizationStatus,
) (int, error) {
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT COUNT(*)
			FROM org.organizations
			WHERE ($1::text IS NULL OR type = $1)
			  AND ($2::text IS NULL OR status = $2);
		`
		var typeStr, statusStr *string
		if orgType != nil {
			s := string(*orgType)
			typeStr = &s
		}
		if status != nil {
			s := string(*status)
			statusStr = &s
		}
		return tx.QueryRow(txCtx, query, typeStr, statusStr).Scan(&total)
	})
	return total, err
}

// GetDeliveryBands retrieves active delivery bands for an organization.
func (r *Repository) GetDeliveryBands(ctx context.Context, orgID int64) ([]*org.DeliveryBand, error) {
	var list []*org.DeliveryBand
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, organization_id, from_meters, to_meters, fee, is_active, created_at, updated_at FROM org.delivery_bands WHERE organization_id = $1 AND is_active = true ORDER BY from_meters ASC;`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var b org.DeliveryBand
			if err := rows.Scan(&b.ID, &b.OrganizationID, &b.FromMeters, &b.ToMeters, &b.Fee, &b.IsActive, &b.CreatedAt, &b.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &b)
		}
		return rows.Err()
	})
	return list, err
}

// ListBranchesWithTotal returns paginated branches matching filter and total count.
func (r *Repository) ListBranchesWithTotal(ctx context.Context, filter org.BranchFilter, limit, offset int) ([]*org.Branch, int, error) {
	var list []*org.Branch
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var conditions []string
		var args []any
		argIdx := 1

		conditions = append(conditions, "b.deleted_at IS NULL")

		if filter.OrganizationID > 0 {
			conditions = append(conditions, fmt.Sprintf("b.organization_id = $%d", argIdx))
			args = append(args, filter.OrganizationID)
			argIdx++
		}

		if filter.Status != "" {
			conditions = append(conditions, fmt.Sprintf("b.status = $%d", argIdx))
			args = append(args, filter.Status)
			argIdx++
		}

		if filter.SearchQuery != "" {
			conditions = append(conditions, fmt.Sprintf("(b.name->>'ar' ILIKE $%d OR b.name->>'en' ILIKE $%d OR b.code ILIKE $%d OR b.address ILIKE $%d OR o.legal_name ILIKE $%d)", argIdx, argIdx, argIdx, argIdx, argIdx))
			args = append(args, "%"+filter.SearchQuery+"%")
			argIdx++
		}

		whereClause := strings.Join(conditions, " AND ")

		countQuery := `SELECT count(*) FROM org.branches b LEFT JOIN org.organizations o ON o.id = b.organization_id WHERE ` + whereClause
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		if limit <= 0 || limit > 100 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}

		dataQuery := fmt.Sprintf(`
			SELECT b.id, b.public_id, b.organization_id, b.name,
			       COALESCE(b.code, ''), COALESCE(b.address, ''), b.city_id,
			       COALESCE(b.latitude, c.latitude) AS latitude,
			       COALESCE(b.longitude, c.longitude) AS longitude,
			       COALESCE(b.google_maps_url, ''), b.manager_id,
			       COALESCE(b.warehouse_type, 'warehouse'),
			       COALESCE(b.has_cold_storage, false),
			       COALESCE(b.capacity_sqm, 0),
			       COALESCE(b.operating_hours, ''),
			       COALESCE(b.status, 'active'),
			       COALESCE(b.is_main, false),
			       COALESCE(b.phone, ''),
			       b.created_at, b.updated_at,
			       COALESCE((SELECT array_agg(DISTINCT COALESCE(w.institutional_work_id::text, w.work_category))
			                 FROM org.branch_institutional_works w
			                 WHERE w.branch_id = b.id), '{}')
			FROM org.branches b
			LEFT JOIN platform_admin.cities c ON c.id = b.city_id
			LEFT JOIN org.organizations o ON o.id = b.organization_id
			WHERE %s
			ORDER BY b.created_at DESC, b.id DESC
			LIMIT $%d OFFSET $%d;
		`, whereClause, argIdx, argIdx+1)

		queryArgs := append(args, limit, offset)
		rows, err := tx.Query(txCtx, dataQuery, queryArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var b org.Branch
			if err := rows.Scan(
				&b.ID, &b.PublicID, &b.OrganizationID, &b.Name, &b.Code, &b.Address, &b.CityID, &b.Latitude, &b.Longitude,
				&b.GoogleMapsURL, &b.ManagerID, &b.WarehouseType, &b.HasColdStorage, &b.CapacitySQM,
				&b.OperatingHours, &b.Status, &b.IsMain, &b.Phone, &b.CreatedAt, &b.UpdatedAt,
				&b.InstitutionalWorks,
			); err != nil {
				return err
			}
			list = append(list, &b)
		}
		return rows.Err()
	})
	return list, total, err
}

// AdminBranchStats aggregates branch metrics for platform admin in a single query.
func (r *Repository) AdminBranchStats(ctx context.Context) (org.AdminBranchStatsResult, error) {
	var stats org.AdminBranchStatsResult
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT 
				COUNT(*),
				COUNT(*) FILTER (WHERE b.status = 'active' OR b.status IS NULL OR b.status = ''),
				COUNT(*) FILTER (WHERE o.type IN ('customer', 'pharmacy', 'chain_pharmacy')),
				COUNT(*) FILTER (WHERE o.type IN ('vendor', 'supplier', 'company', 'agency'))
			FROM org.branches b
			LEFT JOIN org.organizations o ON o.id = b.organization_id
			WHERE b.deleted_at IS NULL;
		`
		return tx.QueryRow(txCtx, query).Scan(&stats.TotalBranches, &stats.ActiveBranches, &stats.PharmacyBranches, &stats.VendorWarehouses)
	})
	return stats, err
}

// SaveDeliveryBands replaces the delivery bands for an organization.
func (r *Repository) SaveDeliveryBands(ctx context.Context, orgID int64, bands []*org.DeliveryBand) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		delQuery := `DELETE FROM org.delivery_bands WHERE organization_id = $1;`
		if _, err := tx.Exec(txCtx, delQuery, orgID); err != nil {
			return err
		}

		insertQuery := `
			INSERT INTO org.delivery_bands (organization_id, from_meters, to_meters, fee, is_active)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, created_at, updated_at;
		`
		for _, b := range bands {
			b.OrganizationID = orgID
			err := tx.QueryRow(txCtx, insertQuery, orgID, b.FromMeters, b.ToMeters, b.Fee, b.IsActive).
				Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// GetReviewCriteria retrieves all evaluation criteria for a context (e.g. supplier, pharmacy, product).
func (r *Repository) GetReviewCriteria(ctx context.Context, contextType string) ([]*org.ReviewCriterion, error) {
	var list []*org.ReviewCriterion
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT key, name, context, weight, sort_order, is_active FROM org.review_criteria WHERE (context = $1 OR $1 = '') AND is_active = true ORDER BY sort_order ASC;`
		rows, err := tx.Query(txCtx, query, contextType)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c org.ReviewCriterion
			if err := rows.Scan(&c.Key, &c.Name, &c.Context, &c.Weight, &c.SortOrder, &c.IsActive); err != nil {
				return err
			}
			list = append(list, &c)
		}
		return rows.Err()
	})
	return list, err
}

// AddReviewWithRatings adds a comprehensive multi-criteria review.
func (r *Repository) AddReviewWithRatings(ctx context.Context, rev *org.Review, ratings []org.ReviewRating) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO org.organization_reviews (
				organization_id, user_id, order_id, product_id, title, rating, review_text, is_verified, is_public, status, context
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
			)
			RETURNING id, public_id, created_at, updated_at;
		`
		err := tx.QueryRow(txCtx, query,
			rev.OrganizationID, rev.UserID, rev.OrderID, rev.ProductID,
			rev.Title, rev.Rating, rev.ReviewText, rev.IsVerified, rev.IsPublic, rev.Status, rev.Context,
		).Scan(&rev.ID, &rev.PublicID, &rev.CreatedAt, &rev.UpdatedAt)
		if err != nil {
			return err
		}

		for _, rat := range ratings {
			queryRat := `INSERT INTO org.review_ratings (review_id, criterion, score) VALUES ($1, $2, $3);`
			if _, err := tx.Exec(txCtx, queryRat, rev.ID, rat.Criterion, rat.Score); err != nil {
				return err
			}
		}

		return nil
	})
}

// ReplyToReview adds a vendor reply to a review.
func (r *Repository) ReplyToReview(ctx context.Context, reviewID, orgID int64, response string, responderID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE org.organization_reviews
			SET response = $1, response_at = now(), responded_by = $2, updated_at = now()
			WHERE id = $3 AND organization_id = $4;
		`
		tag, err := tx.Exec(txCtx, query, response, responderID, reviewID, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("review")
		}
		return nil
	})
}
