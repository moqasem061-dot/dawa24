package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// ListOrganizationsWithTotal returns paginated organizations matching search & filters with total count.
func (r *Repository) ListOrganizationsWithTotal(
	ctx context.Context,
	search string,
	orgType *org.OrganizationType,
	status *org.OrganizationStatus,
	limit, offset int,
) ([]*org.Organization, int, error) {
	var list []*org.Organization
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var typeStr, statusStr *string
		if orgType != nil {
			s := string(*orgType)
			typeStr = &s
		}
		if status != nil {
			s := string(*status)
			statusStr = &s
		}

		countQuery := `
			SELECT count(*)
			FROM org.organizations o
			WHERE ($1::text IS NULL OR o.type = $1)
			  AND ($2::text IS NULL OR o.status = $2)
			  AND ($3 = '' OR o.legal_name ILIKE '%' || $3 || '%' OR o.name->>'ar' ILIKE '%' || $3 || '%' OR o.trade_name->>'ar' ILIKE '%' || $3 || '%' OR o.commercial_register ILIKE '%' || $3 || '%' OR o.tax_number ILIKE '%' || $3 || '%' OR o.pharmacist_license ILIKE '%' || $3 || '%');
		`
		if err := tx.QueryRow(txCtx, countQuery, typeStr, statusStr, search).Scan(&total); err != nil {
			return err
		}

		if limit <= 0 || limit > 100 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}

		query := `
			SELECT o.id, o.public_id, COALESCE(NULLIF(o.legal_name, ''), NULLIF(o.name->>'ar', ''), NULLIF(o.trade_name->>'ar', ''), ''), o.trade_name, o.tax_number, o.commercial_register,
			       COALESCE(o.pharmacist_license, ''),
			       COALESCE(o.verification_notes, ''), COALESCE(o.rejection_reason, ''),
			       COALESCE(o.owner_id, 0), o.approved_at, o.approved_by,
			       COALESCE(o.ai_virtual_key, ''), COALESCE(o.ai_user_id, ''),
			       o.type, o.status, o.credit_limit, o.payment_terms_days, o.created_at, o.updated_at
			FROM org.organizations o
			WHERE ($1::text IS NULL OR o.type = $1)
			  AND ($2::text IS NULL OR o.status = $2)
			  AND ($3 = '' OR o.legal_name ILIKE '%' || $3 || '%' OR o.name->>'ar' ILIKE '%' || $3 || '%' OR o.trade_name->>'ar' ILIKE '%' || $3 || '%' OR o.commercial_register ILIKE '%' || $3 || '%' OR o.tax_number ILIKE '%' || $3 || '%' OR o.pharmacist_license ILIKE '%' || $3 || '%')
			ORDER BY o.created_at DESC, o.id DESC
			LIMIT $4 OFFSET $5;
		`
		rows, err := tx.Query(txCtx, query, typeStr, statusStr, search, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var o org.Organization
			var tStr, sStr string
			if err := rows.Scan(
				&o.ID, &o.PublicID, &o.LegalName, &o.TradeName, &o.TaxNumber, &o.CommercialRegister,
				&o.PharmacistLicense,
				&o.VerificationNotes, &o.RejectionReason,
				&o.OwnerID, &o.ApprovedAt, &o.ApprovedBy,
				&o.AIVirtualKey, &o.AIUserID,
				&tStr, &sStr, &o.CreditLimit, &o.PaymentTermsDays, &o.CreatedAt, &o.UpdatedAt,
			); err != nil {
				return err
			}
			o.Type = org.OrganizationType(tStr)
			o.Status = org.OrganizationStatus(sStr)
			list = append(list, &o)
		}
		return rows.Err()
	})
	return list, total, err
}

// AdminOrgStats computes platform-wide organization metrics in a single aggregation.
func (r *Repository) AdminOrgStats(ctx context.Context) (org.AdminOrgStatsResult, error) {
	var stats org.AdminOrgStatsResult
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT
				COUNT(*),
				COUNT(*) FILTER (WHERE type != 'vendor'),
				COUNT(*) FILTER (WHERE type = 'vendor'),
				COUNT(*) FILTER (WHERE status = 'pending'),
				COUNT(*) FILTER (WHERE status = 'approved')
			FROM org.organizations;
		`
		return tx.QueryRow(txCtx, query).Scan(
			&stats.TotalOrgs,
			&stats.TotalPharmacies,
			&stats.TotalVendors,
			&stats.PendingCount,
			&stats.ApprovedCount,
		)
	})
	return stats, err
}

// CountBranchesByOrg aggregates branch counts across all organizations.
func (r *Repository) CountBranchesByOrg(ctx context.Context) (map[int64]int, error) {
	counts := make(map[int64]int)
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT organization_id, count(*)
			FROM org.branches
			WHERE deleted_at IS NULL
			GROUP BY organization_id;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var orgID int64
			var cnt int
			if err := rows.Scan(&orgID, &cnt); err != nil {
				return err
			}
			counts[orgID] = cnt
		}
		return rows.Err()
	})
	return counts, err
}

// ListOrganizations returns filtered organizations.
func (r *Repository) ListOrganizations(
	ctx context.Context,
	orgType *org.OrganizationType,
	status *org.OrganizationStatus,
	limit, offset int,
) ([]*org.Organization, error) {
	var list []*org.Organization
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, COALESCE(NULLIF(legal_name, ''), NULLIF(name->>'ar', ''), NULLIF(trade_name->>'ar', ''), ''), trade_name, tax_number, commercial_register,
			       COALESCE(pharmacist_license, ''),
			       COALESCE(verification_notes, ''), COALESCE(rejection_reason, ''),
			       COALESCE(owner_id, 0), approved_at, approved_by,
			       COALESCE(ai_virtual_key, ''), COALESCE(ai_user_id, ''),
			       type, status, credit_limit, payment_terms_days, created_at, updated_at
			FROM org.organizations
			WHERE ($1::text IS NULL OR type = $1)
			  AND ($2::text IS NULL OR status = $2)
			ORDER BY created_at DESC
			LIMIT $3 OFFSET $4;
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
		if limit <= 0 {
			limit = 20
		}
		if limit > 1000 {
			limit = 1000
		}
		rows, err := tx.Query(txCtx, query, typeStr, statusStr, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var o org.Organization
			var tStr, sStr string
			if err := rows.Scan(
				&o.ID, &o.PublicID, &o.LegalName, &o.TradeName, &o.TaxNumber, &o.CommercialRegister,
				&o.PharmacistLicense, &o.VerificationNotes, &o.RejectionReason,
				&o.OwnerID, &o.ApprovedAt, &o.ApprovedBy,
				&o.AIVirtualKey, &o.AIUserID,
				&tStr, &sStr, &o.CreditLimit, &o.PaymentTermsDays, &o.CreatedAt, &o.UpdatedAt,
			); err != nil {
				return err
			}
			o.Type = org.OrganizationType(tStr)
			o.Status = org.OrganizationStatus(sStr)
			list = append(list, &o)
		}
		return rows.Err()
	})
	return list, err
}
