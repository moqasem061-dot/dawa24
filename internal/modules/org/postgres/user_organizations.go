package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// CreateUserOrganization inserts or updates a customer user to vendor organization link.
func (r *Repository) CreateUserOrganization(ctx context.Context, uo *org.UserOrganization) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO org.user_organizations (
				user_id, customer_org_id, vendor_org_id, organization_number, status, notes, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, now(), now()
			)
			ON CONFLICT (user_id, vendor_org_id) WHERE deleted_at IS NULL
			DO UPDATE SET
				customer_org_id = EXCLUDED.customer_org_id,
				organization_number = EXCLUDED.organization_number,
				status = EXCLUDED.status,
				notes = EXCLUDED.notes,
				updated_at = now()
			RETURNING id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			uo.UserID, uo.CustomerOrgID, uo.VendorOrgID, uo.OrganizationNumber, string(uo.Status), uo.Notes,
		).Scan(&uo.ID, &uo.CreatedAt, &uo.UpdatedAt)
	})
}

// GetUserOrganizationByID retrieves a single UserOrganization link by ID.
func (r *Repository) GetUserOrganizationByID(ctx context.Context, id int64) (*org.UserOrganization, error) {
	var uo org.UserOrganization
	var status string
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT uo.id, uo.user_id, COALESCE(u.name->>'ar', u.name->>'en', u.name::text, ''), COALESCE(u.email::text, ''),
			       uo.customer_org_id, COALESCE(NULLIF(co.legal_name, ''), co.name->>'ar', co.name->>'en', ''),
			       uo.vendor_org_id, COALESCE(NULLIF(vo.legal_name, ''), vo.name->>'ar', vo.name->>'en', ''), COALESCE(vo.type, 'vendor'),
			       uo.organization_number, uo.status, uo.notes, uo.created_at, uo.updated_at
			FROM org.user_organizations uo
			LEFT JOIN identity.users u ON u.id = uo.user_id
			LEFT JOIN org.organizations co ON co.id = uo.customer_org_id
			LEFT JOIN org.organizations vo ON vo.id = uo.vendor_org_id
			WHERE uo.id = $1 AND uo.deleted_at IS NULL;
		`
		return tx.QueryRow(txCtx, query, id).Scan(
			&uo.ID, &uo.UserID, &uo.UserName, &uo.UserEmail,
			&uo.CustomerOrgID, &uo.CustomerOrgName,
			&uo.VendorOrgID, &uo.VendorOrgName, &uo.VendorOrgType,
			&uo.OrganizationNumber, &status, &uo.Notes, &uo.CreatedAt, &uo.UpdatedAt,
		)
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperr.NotFound("user_organization")
		}
		return nil, err
	}
	uo.Status = org.UserOrganizationStatus(status)
	return &uo, nil
}

// UpdateUserOrganization updates the code, status, or notes of a link.
func (r *Repository) UpdateUserOrganization(ctx context.Context, id int64, orgNumber string, status org.UserOrganizationStatus, notes string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE org.user_organizations
			SET organization_number = CASE WHEN $2 <> '' THEN $2 ELSE organization_number END,
			    status = CASE WHEN $3 <> '' THEN $3 ELSE status END,
			    notes = CASE WHEN $4 <> '' THEN $4 ELSE notes END,
			    updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL;
		`
		cmd, err := tx.Exec(txCtx, query, id, orgNumber, string(status), notes)
		if err != nil {
			return err
		}
		if cmd.RowsAffected() == 0 {
			return apperr.NotFound("user_organization")
		}
		return nil
	})
}

// DeleteUserOrganization marks a link as deleted.
func (r *Repository) DeleteUserOrganization(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE org.user_organizations
			SET deleted_at = now(), updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL;
		`
		cmd, err := tx.Exec(txCtx, query, id)
		if err != nil {
			return err
		}
		if cmd.RowsAffected() == 0 {
			return apperr.NotFound("user_organization")
		}
		return nil
	})
}

// ListUserOrganizationsByUser returns all links for a given user.
func (r *Repository) ListUserOrganizationsByUser(ctx context.Context, userID int64) ([]*org.UserOrganization, error) {
	var list []*org.UserOrganization
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT uo.id, uo.user_id, COALESCE(u.name->>'ar', u.name->>'en', u.name::text, ''), COALESCE(u.email::text, ''),
			       uo.customer_org_id, COALESCE(NULLIF(co.legal_name, ''), co.name->>'ar', co.name->>'en', ''),
			       uo.vendor_org_id, COALESCE(NULLIF(vo.legal_name, ''), vo.name->>'ar', vo.name->>'en', ''), COALESCE(vo.type, 'vendor'),
			       uo.organization_number, uo.status, uo.notes, uo.created_at, uo.updated_at
			FROM org.user_organizations uo
			LEFT JOIN identity.users u ON u.id = uo.user_id
			LEFT JOIN org.organizations co ON co.id = uo.customer_org_id
			LEFT JOIN org.organizations vo ON vo.id = uo.vendor_org_id
			WHERE uo.user_id = $1 AND uo.deleted_at IS NULL
			ORDER BY uo.created_at DESC;
		`
		rows, err := tx.Query(txCtx, query, userID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var uo org.UserOrganization
			var status string
			if err := rows.Scan(
				&uo.ID, &uo.UserID, &uo.UserName, &uo.UserEmail,
				&uo.CustomerOrgID, &uo.CustomerOrgName,
				&uo.VendorOrgID, &uo.VendorOrgName, &uo.VendorOrgType,
				&uo.OrganizationNumber, &status, &uo.Notes, &uo.CreatedAt, &uo.UpdatedAt,
			); err != nil {
				return err
			}
			uo.Status = org.UserOrganizationStatus(status)
			list = append(list, &uo)
		}
		return rows.Err()
	})
	return list, err
}

// ListUserOrganizationsByVendor returns all customer links for a vendor with optional status filter.
func (r *Repository) ListUserOrganizationsByVendor(ctx context.Context, vendorOrgID int64, statusFilter string) ([]*org.UserOrganization, error) {
	list, _, err := r.ListUserOrganizationsByVendorWithTotal(ctx, vendorOrgID, statusFilter, 500, 0)
	return list, err
}

// ListUserOrganizationsByVendorWithTotal returns paginated customer links for a vendor with total count.
func (r *Repository) ListUserOrganizationsByVendorWithTotal(ctx context.Context, vendorOrgID int64, statusFilter string, limit, offset int) ([]*org.UserOrganization, int, error) {
	var list []*org.UserOrganization
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		countQ := "SELECT count(*) FROM org.user_organizations WHERE vendor_org_id = $1 AND deleted_at IS NULL"
		countArgs := []interface{}{vendorOrgID}
		if statusFilter != "" && statusFilter != "all" {
			countQ += " AND status = $2"
			countArgs = append(countArgs, statusFilter)
		}
		if err := tx.QueryRow(txCtx, countQ, countArgs...).Scan(&total); err != nil {
			return err
		}

		baseQuery := `
			SELECT uo.id, uo.user_id, COALESCE(u.name->>'ar', u.name->>'en', u.name::text, ''), COALESCE(u.email::text, ''),
			       uo.customer_org_id, COALESCE(NULLIF(co.legal_name, ''), co.name->>'ar', co.name->>'en', ''),
			       uo.vendor_org_id, COALESCE(NULLIF(vo.legal_name, ''), vo.name->>'ar', vo.name->>'en', ''), COALESCE(vo.type, 'vendor'),
			       uo.organization_number, uo.status, uo.notes, uo.created_at, uo.updated_at
			FROM org.user_organizations uo
			LEFT JOIN identity.users u ON u.id = uo.user_id
			LEFT JOIN org.organizations co ON co.id = uo.customer_org_id
			LEFT JOIN org.organizations vo ON vo.id = uo.vendor_org_id
			WHERE uo.vendor_org_id = $1 AND uo.deleted_at IS NULL
		`
		args := []interface{}{vendorOrgID}
		if statusFilter != "" && statusFilter != "all" {
			baseQuery += " AND uo.status = $2"
			args = append(args, statusFilter)
		}
		baseQuery += fmt.Sprintf(" ORDER BY uo.created_at DESC, uo.id DESC LIMIT $%d OFFSET $%d;", len(args)+1, len(args)+2)
		args = append(args, limit, offset)

		rows, err := tx.Query(txCtx, baseQuery, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var uo org.UserOrganization
			var status string
			if err := rows.Scan(
				&uo.ID, &uo.UserID, &uo.UserName, &uo.UserEmail,
				&uo.CustomerOrgID, &uo.CustomerOrgName,
				&uo.VendorOrgID, &uo.VendorOrgName, &uo.VendorOrgType,
				&uo.OrganizationNumber, &status, &uo.Notes, &uo.CreatedAt, &uo.UpdatedAt,
			); err != nil {
				return err
			}
			uo.Status = org.UserOrganizationStatus(status)
			list = append(list, &uo)
		}
		return rows.Err()
	})
	return list, total, err
}

// ListAllUserOrganizations returns all links across the platform (for admin oversight).
func (r *Repository) ListAllUserOrganizations(ctx context.Context, statusFilter string) ([]*org.UserOrganization, error) {
	list, _, err := r.ListAllUserOrganizationsWithTotal(ctx, statusFilter, 500, 0)
	return list, err
}

// ListAllUserOrganizationsWithTotal returns paginated links across the platform with total count.
func (r *Repository) ListAllUserOrganizationsWithTotal(ctx context.Context, statusFilter string, limit, offset int) ([]*org.UserOrganization, int, error) {
	var list []*org.UserOrganization
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		countQ := "SELECT count(*) FROM org.user_organizations WHERE deleted_at IS NULL"
		countArgs := []interface{}{}
		if statusFilter != "" && statusFilter != "all" {
			countQ += " AND status = $1"
			countArgs = append(countArgs, statusFilter)
		}
		if err := tx.QueryRow(txCtx, countQ, countArgs...).Scan(&total); err != nil {
			return err
		}

		baseQuery := `
			SELECT uo.id, uo.user_id, COALESCE(u.name->>'ar', u.name->>'en', u.name::text, ''), COALESCE(u.email::text, ''),
			       uo.customer_org_id, COALESCE(NULLIF(co.legal_name, ''), co.name->>'ar', co.name->>'en', ''),
			       uo.vendor_org_id, COALESCE(NULLIF(vo.legal_name, ''), vo.name->>'ar', vo.name->>'en', ''), COALESCE(vo.type, 'vendor'),
			       uo.organization_number, uo.status, uo.notes, uo.created_at, uo.updated_at
			FROM org.user_organizations uo
			LEFT JOIN identity.users u ON u.id = uo.user_id
			LEFT JOIN org.organizations co ON co.id = uo.customer_org_id
			LEFT JOIN org.organizations vo ON vo.id = uo.vendor_org_id
			WHERE uo.deleted_at IS NULL
		`
		args := []interface{}{}
		if statusFilter != "" && statusFilter != "all" {
			baseQuery += " AND uo.status = $1"
			args = append(args, statusFilter)
		}
		baseQuery += fmt.Sprintf(" ORDER BY uo.created_at DESC, uo.id DESC LIMIT $%d OFFSET $%d;", len(args)+1, len(args)+2)
		args = append(args, limit, offset)

		rows, err := tx.Query(txCtx, baseQuery, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var uo org.UserOrganization
			var status string
			if err := rows.Scan(
				&uo.ID, &uo.UserID, &uo.UserName, &uo.UserEmail,
				&uo.CustomerOrgID, &uo.CustomerOrgName,
				&uo.VendorOrgID, &uo.VendorOrgName, &uo.VendorOrgType,
				&uo.OrganizationNumber, &status, &uo.Notes, &uo.CreatedAt, &uo.UpdatedAt,
			); err != nil {
				return err
			}
			uo.Status = org.UserOrganizationStatus(status)
			list = append(list, &uo)
		}
		return rows.Err()
	})
	return list, total, err
}
