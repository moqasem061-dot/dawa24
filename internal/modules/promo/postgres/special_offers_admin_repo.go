package postgres

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// ListAllSpecialOffers returns all special offers across suppliers with organizations, products and locations for admin.
func (r *Repository) ListAllSpecialOffers(ctx context.Context, limit, offset int) ([]*promo.SpecialOffer, error) {
	offers, _, err := r.ListAllSpecialOffersWithTotal(ctx, "", limit, offset)
	return offers, err
}

// ListAllSpecialOffersWithTotal returns a paginated slice of special offers with status filter and total count.
func (r *Repository) ListAllSpecialOffersWithTotal(ctx context.Context, statusFilter string, limit, offset int) ([]*promo.SpecialOffer, int, error) {
	var list []*promo.SpecialOffer
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		where := []string{"o.deleted_at IS NULL"}

		switch statusFilter {
		case "pending":
			where = append(where, "(o.admin_status = 'pending' OR o.admin_status IS NULL OR o.admin_status = '')")
		case "active":
			where = append(where, "(o.admin_status = 'approved' AND o.is_active = true AND o.is_draft = false)")
		case "rejected":
			where = append(where, "(o.admin_status = 'rejected')")
		case "draft":
			where = append(where, "(o.is_draft = true OR o.is_active = false)")
		}

		clause := ""
		for i, w := range where {
			if i > 0 {
				clause += " AND "
			}
			clause += w
		}

		countSQL := "SELECT count(*) FROM promo.offers o WHERE " + clause + ";"
		if err := tx.QueryRow(txCtx, countSQL).Scan(&total); err != nil {
			return err
		}

		if limit <= 0 || limit > 100 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}

		query := `
			SELECT o.id, o.public_id, o.organization_id, COALESCE(org.legal_name, ''), o.branch_id, COALESCE(b.name->>'ar', ''),
			       o.title, o.description,
			       CASE WHEN o.discount_type = 'percentage' THEN o.discount_value ELSE 0 END,
			       CASE WHEN o.discount_type = 'fixed'      THEN o.discount_value ELSE 0 END,
			       COALESCE(o.min_order_amount, 0), COALESCE(o.total_price, 0),
			       o.starts_at, o.expires_at,
			       CASE WHEN o.is_draft   THEN 'draft'
			            WHEN o.is_active  THEN 'active'
			            ELSE 'inactive' END,
			       o.admin_status, COALESCE(o.admin_notes, ''), COALESCE(o.image, ''),
			       o.created_at, o.updated_at
			FROM promo.offers o
			LEFT JOIN org.organizations org ON org.id = o.organization_id
			LEFT JOIN org.branches b ON b.id = o.branch_id
			WHERE ` + clause + `
			ORDER BY o.created_at DESC, o.id DESC
			LIMIT $1 OFFSET $2;
		`
		rows, err := tx.Query(txCtx, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var o promo.SpecialOffer
			if err := rows.Scan(
				&o.ID, &o.PublicID, &o.OrganizationID, &o.OrganizationName, &o.BranchID, &o.BranchName,
				&o.Title, &o.Description, &o.DiscountPercentage,
				&o.DiscountAmount, &o.MinOrderAmount, &o.TotalPrice,
				&o.StartDate, &o.EndDate, &o.Status, &o.AdminStatus, &o.AdminNotes, &o.Image,
				&o.CreatedAt, &o.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &o)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		// PERF OPTIMIZATION NOTE (Priority 10):
		// This loop executes 2 queries per offer (loadSpecialOfferProducts + offer_location_covers).
		// For 50 offers, this triggers a 2N+1 query pattern (1 + 50*2 = 101 queries).
		// Future refactor: bulk-load products and locations using WHERE offer_id = ANY($1)
		// with all offer IDs collected from 'list', then group in-memory by offer_id.
		// Populate products and locations for each offer
		for _, offer := range list {
			prods, err := loadSpecialOfferProducts(txCtx, tx, offer.ID)
			if err != nil {
				return err
			}
			offer.Products = prods

			lRows, _ := tx.Query(txCtx, `
				SELECT l.id, l.offer_id, l.city_id, COALESCE(c.name->>'ar', ''),
				       l.address_ar, l.address_en, l.latitude, l.longitude, l.radius_meters,
				       l.day_of_week + 1, COALESCE(to_char(l.time_from, 'HH24:MI'), ''), COALESCE(to_char(l.time_to, 'HH24:MI'), ''),
				       l.status, l.admin_status, l.created_at
				FROM promo.offer_location_covers l
				LEFT JOIN platform_admin.cities c ON c.id = l.city_id
				WHERE l.offer_id = $1;
			`, offer.ID)
			if lRows != nil {
				for lRows.Next() {
					var loc promo.SpecialOfferLocation
					if err := lRows.Scan(
						&loc.ID, &loc.OfferID, &loc.CityID, &loc.CityName,
						&loc.AddressAr, &loc.AddressEn, &loc.Latitude, &loc.Longitude, &loc.Radius,
						&loc.DayOfWeek, &loc.TimeFrom, &loc.TimeTo,
						&loc.Status, &loc.AdminStatus, &loc.CreatedAt,
					); err == nil {
						offer.Locations = append(offer.Locations, &loc)
					}
				}
				lRows.Close()
			}
		}

		return nil
	})
	return list, total, err
}

// UpdateSpecialOfferAdminStatus updates the moderation state (approved/rejected) of a special offer.
func (r *Repository) UpdateSpecialOfferAdminStatus(ctx context.Context, id int64, adminStatus, notes string, approvedBy int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE promo.offers
			SET admin_status = $1, admin_notes = $2, approved_by = $3,
			    approved_at = CASE WHEN $1 = 'approved' THEN now() ELSE approved_at END,
			    rejected_at = CASE WHEN $1 = 'rejected' THEN now() ELSE rejected_at END,
			    updated_at = now()
			WHERE id = $4;
		`
		tag, err := tx.Exec(txCtx, query, adminStatus, notes, approvedBy, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("special_offer")
		}

		auditAction := "promo.offer.reject"
		if adminStatus == "approved" {
			auditAction = "promo.offer.approve"
		}
		var orgID *int64
		_ = tx.QueryRow(txCtx, `SELECT organization_id FROM promo.offers WHERE id = $1;`, id).Scan(&orgID)

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: orgID,
			ActorUserID:    approvedBy,
			Action:         auditAction,
			EntityType:     "special_offer",
			EntityID:       strconv.FormatInt(id, 10),
			After:          map[string]any{"status": adminStatus, "notes": notes},
		})

		return nil
	})
}

// ToggleSpecialOfferStatus activates or deactivates a special offer.
func (r *Repository) ToggleSpecialOfferStatus(ctx context.Context, id int64, isActive bool) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `UPDATE promo.offers SET is_active = $1, updated_at = now() WHERE id = $2;`, isActive, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("special_offer")
		}
		return nil
	})
}

// UpdateSpecialOffer updates a vendor's special offer and replaces bundled products, resetting admin_status to 'pending'.
func (r *Repository) UpdateSpecialOffer(ctx context.Context, o *promo.SpecialOffer) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		discType := "percentage"
		discVal := money.FromMinor(int64(math.Round(o.DiscountPercentage * 100)))
		if o.DiscountAmount.IsPositive() {
			discType = "fixed"
			discVal = o.DiscountAmount
		}

		query := `
			UPDATE promo.offers
			SET branch_id = $1, title = $2, description = $3,
			    discount_type = $4, discount_value = $5,
			    min_order_amount = $6, total_price = $7,
			    starts_at = COALESCE($8, starts_at),
			    expires_at = COALESCE($9, expires_at),
			    is_active = (COALESCE($10, 'active') = 'active'),
			    is_draft = (COALESCE($10, 'active') = 'draft'),
			    admin_status = 'pending',
			    image = CASE WHEN $11 <> '' THEN $11 ELSE image END,
			    updated_at = now()
			WHERE id = $12 AND organization_id = $13;
		`
		tag, err := tx.Exec(txCtx, query,
			o.BranchID, o.Title, o.Description,
			discType, discVal,
			o.MinOrderAmount, o.TotalPrice,
			o.StartDate, o.EndDate,
			o.Status, o.Image,
			o.ID, o.OrganizationID,
		)
		if err != nil {
			return fmt.Errorf("update special offer: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("special_offer")
		}

		// Replace products
		if _, err := tx.Exec(txCtx, `DELETE FROM promo.offer_products WHERE offer_id = $1;`, o.ID); err != nil {
			return fmt.Errorf("clear offer products: %w", err)
		}

		// A variant that is not this vendor's, or no longer exists, is skipped
		// rather than aborting the offer with a NOT NULL violation on product_id.
		if _, err := insertSpecialOfferProducts(txCtx, tx, o.ID, o.OrganizationID, o.Products); err != nil {
			return err
		}

		return nil
	})
}

// ListAdminOfferLocations returns all geographic coverage records with joined offers, suppliers, and cities for admin.
func (r *Repository) ListAdminOfferLocations(
	ctx context.Context,
	filter promo.OfferLocationsFilter,
) ([]*promo.OfferLocationAdminRow, promo.OfferLocationsStats, int, error) {
	var list []*promo.OfferLocationAdminRow
	var stats promo.OfferLocationsStats
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Calculate overall stats
		statsQuery := `
			SELECT
				COUNT(*),
				COUNT(*) FILTER (WHERE l.status = 'active'),
				COUNT(DISTINCT l.city_id),
				COUNT(DISTINCT c.governorate_id),
				COUNT(DISTINCT l.offer_id)
			FROM promo.offer_location_covers l
			LEFT JOIN platform_admin.cities c ON c.id = l.city_id;
		`
		if err := tx.QueryRow(txCtx, statsQuery).Scan(
			&stats.TotalLocations,
			&stats.ActiveLocations,
			&stats.CoveredCities,
			&stats.CoveredGovernorates,
			&stats.ActiveOffers,
		); err != nil {
			return fmt.Errorf("calculate offer location stats: %w", err)
		}

		// 2. Build where clause
		var whereClauses []string
		var args []any
		argIdx := 1

		if filter.OfferID > 0 {
			whereClauses = append(whereClauses, fmt.Sprintf("l.offer_id = $%d", argIdx))
			args = append(args, filter.OfferID)
			argIdx++
		}
		if filter.Status != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("l.status = $%d", argIdx))
			args = append(args, filter.Status)
			argIdx++
		}
		if filter.AdminStatus != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("l.admin_status = $%d", argIdx))
			args = append(args, filter.AdminStatus)
			argIdx++
		}
		if filter.Governorate != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("g.name::text ILIKE $%d", argIdx))
			args = append(args, "%"+filter.Governorate+"%")
			argIdx++
		}
		if filter.Search != "" {
			searchPattern := "%" + filter.Search + "%"
			whereClauses = append(whereClauses, fmt.Sprintf(
				"(o.title::text ILIKE $%d OR org.legal_name ILIKE $%d OR org.name::text ILIKE $%d OR c.name::text ILIKE $%d OR l.address_ar ILIKE $%d)",
				argIdx, argIdx, argIdx, argIdx, argIdx,
			))
			args = append(args, searchPattern)
			argIdx++
		}

		whereStr := ""
		if len(whereClauses) > 0 {
			whereStr = "WHERE " + strings.Join(whereClauses, " AND ")
		}

		// 3. Count matching rows
		countQuery := `
			SELECT count(*)
			FROM promo.offer_location_covers l
			LEFT JOIN promo.offers o ON o.id = l.offer_id
			LEFT JOIN org.organizations org ON org.id = l.organization_id
			LEFT JOIN platform_admin.cities c ON c.id = l.city_id
			LEFT JOIN platform_admin.governorates g ON g.id = c.governorate_id
			` + whereStr + `;`
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return fmt.Errorf("count admin offer locations: %w", err)
		}

		// 4. Fetch page of rows
		limit := filter.Limit
		if limit <= 0 || limit > 100 {
			limit = 25
		}
		offset := filter.Offset
		if offset < 0 {
			offset = 0
		}

		args = append(args, limit, offset)
		query := `
			SELECT l.id, l.offer_id, COALESCE(o.title->>'ar', o.title->>'en', o.title::text, ''),
			       l.organization_id, COALESCE(org.legal_name, org.name->>'ar', ''),
			       l.city_id, COALESCE(c.name->>'ar', c.name->>'en', ''),
			       COALESCE(g.id, 0), COALESCE(g.name->>'ar', g.name->>'en', ''),
			       COALESCE(l.address_ar, ''), COALESCE(l.address_en, ''),
			       COALESCE(l.latitude, 0), COALESCE(l.longitude, 0), COALESCE(l.radius_meters, 0),
			       COALESCE(l.day_of_week, 0) + 1,
			       COALESCE(to_char(l.time_from, 'HH24:MI'), ''),
			       COALESCE(to_char(l.time_to, 'HH24:MI'), ''),
			       COALESCE(l.status, 'active'), COALESCE(l.admin_status, 'approved'), l.created_at
			FROM promo.offer_location_covers l
			LEFT JOIN promo.offers o ON o.id = l.offer_id
			LEFT JOIN org.organizations org ON org.id = l.organization_id
			LEFT JOIN platform_admin.cities c ON c.id = l.city_id
			LEFT JOIN platform_admin.governorates g ON g.id = c.governorate_id
			` + whereStr + fmt.Sprintf(`
			ORDER BY l.created_at DESC, l.id DESC
			LIMIT $%d OFFSET $%d;
		`, argIdx, argIdx+1)

		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return fmt.Errorf("query admin offer locations: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var row promo.OfferLocationAdminRow
			if err := rows.Scan(
				&row.ID, &row.OfferID, &row.OfferTitle,
				&row.OrganizationID, &row.SupplierName,
				&row.CityID, &row.CityName,
				&row.GovernorateID, &row.GovernorateName,
				&row.AddressAr, &row.AddressEn,
				&row.Latitude, &row.Longitude, &row.RadiusMeters,
				&row.DayOfWeek, &row.TimeFrom, &row.TimeTo,
				&row.Status, &row.AdminStatus, &row.CreatedAt,
			); err != nil {
				return fmt.Errorf("scan admin offer location row: %w", err)
			}
			list = append(list, &row)
		}
		return rows.Err()
	})

	return list, stats, total, err
}
