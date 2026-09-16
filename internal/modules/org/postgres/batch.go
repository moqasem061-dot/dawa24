package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// GetOrganizationsByIDs retrieves many organizations in a single query.
// Storefront offer rendering resolves supplier names for every variant on the
// page; fetching them one by one turned each catalog view into hundreds of
// round trips.
func (r *Repository) GetOrganizationsByIDs(ctx context.Context, ids []int64) ([]*org.Organization, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var list []*org.Organization
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, legal_name, trade_name, tax_number, commercial_register,
			       COALESCE(pharmacist_license, ''),
			       COALESCE(verification_notes, ''), COALESCE(rejection_reason, ''),
			       COALESCE(owner_id, 0), approved_at, approved_by,
			       COALESCE(ai_virtual_key, ''), COALESCE(ai_user_id, ''),
			       type, status, credit_limit, payment_terms_days, created_at, updated_at
			FROM org.organizations
			WHERE id = ANY($1);
		`
		rows, err := tx.Query(txCtx, query, ids)
		if err != nil {
			return fmt.Errorf("org postgres: get organizations by ids: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var o org.Organization
			var typeStr, statusStr string
			if err := rows.Scan(
				&o.ID, &o.PublicID, &o.LegalName, &o.TradeName, &o.TaxNumber, &o.CommercialRegister,
				&o.PharmacistLicense, &o.VerificationNotes, &o.RejectionReason,
				&o.OwnerID, &o.ApprovedAt, &o.ApprovedBy,
				&o.AIVirtualKey, &o.AIUserID,
				&typeStr, &statusStr, &o.CreditLimit, &o.PaymentTermsDays, &o.CreatedAt, &o.UpdatedAt,
			); err != nil {
				return err
			}
			o.Type = org.OrganizationType(typeStr)
			o.Status = org.OrganizationStatus(statusStr)
			list = append(list, &o)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

// GetBranchesByIDs retrieves many branches — with their institutional works —
// in a single query. The works ride along as one aggregate instead of the
// per-branch follow-up query GetBranchByID issues.
func (r *Repository) GetBranchesByIDs(ctx context.Context, ids []int64) ([]*org.Branch, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var list []*org.Branch
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT b.id, b.public_id, b.organization_id, b.name,
			       COALESCE(b.code, ''), b.address, b.city_id,
			       COALESCE(b.latitude, c.latitude) AS latitude,
			       COALESCE(b.longitude, c.longitude) AS longitude,
			       COALESCE(b.google_maps_url, ''), b.manager_id,
			       COALESCE(NULLIF(u.name->>'ar', ''), NULLIF(u.name->>'en', ''), b.manager_name, ''),
			       COALESCE(u.email, ''), COALESCE(u.phone, ''),
			       COALESCE(b.warehouse_type, 'warehouse'), COALESCE(b.has_cold_storage, false),
			       COALESCE(b.capacity_sqm, 0), COALESCE(b.operating_hours, ''),
			       COALESCE(b.status, 'active'), b.is_main, COALESCE(b.phone, ''), b.created_at, b.updated_at,
			       COALESCE((SELECT array_agg(DISTINCT COALESCE(w.institutional_work_id::text, w.work_category))
			                 FROM org.branch_institutional_works w
			                 WHERE w.branch_id = b.id), '{}')
			FROM org.branches b
			LEFT JOIN platform_admin.cities c ON c.id = b.city_id
			LEFT JOIN identity.users u ON u.id = b.manager_id
			WHERE b.id = ANY($1) AND b.deleted_at IS NULL;
		`
		rows, err := tx.Query(txCtx, query, ids)
		if err != nil {
			return fmt.Errorf("org postgres: get branches by ids: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var b org.Branch
			if err := rows.Scan(
				&b.ID, &b.PublicID, &b.OrganizationID, &b.Name,
				&b.Code, &b.Address, &b.CityID, &b.Latitude, &b.Longitude,
				&b.GoogleMapsURL, &b.ManagerID, &b.ManagerName,
				&b.ManagerEmail, &b.ManagerPhone,
				&b.WarehouseType, &b.HasColdStorage,
				&b.CapacitySQM, &b.OperatingHours,
				&b.Status, &b.IsMain, &b.Phone, &b.CreatedAt, &b.UpdatedAt,
				&b.InstitutionalWorks,
			); err != nil {
				return err
			}
			list = append(list, &b)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}
