package postgres

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// CreateBranch inserts a branch.
func (r *Repository) CreateBranch(ctx context.Context, b *org.Branch) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO org.branches (
				organization_id, name, code, address, city_id, latitude, longitude,
				google_maps_url, manager_id, manager_name, warehouse_type, has_cold_storage,
				capacity_sqm, operating_hours, status, is_main, phone
			) VALUES (
				$1, COALESCE($2, '{"ar":"الفرع","en":"Branch"}'::jsonb), NULLIF($3, ''), $4, $5, $6, $7,
				COALESCE($8, ''), $9, COALESCE($10, ''), COALESCE(NULLIF($11, ''), 'warehouse'), COALESCE($12, false),
				COALESCE($13, 0), COALESCE($14, ''), COALESCE(NULLIF($15, ''), 'active'), $16, $17
			)
			RETURNING id, public_id, created_at, updated_at;
		`
		if err := tx.QueryRow(txCtx, query,
			b.OrganizationID, b.Name, b.Code, b.Address, b.CityID, b.Latitude, b.Longitude,
			b.GoogleMapsURL, b.ManagerID, b.ManagerName, b.WarehouseType, b.HasColdStorage,
			b.CapacitySQM, b.OperatingHours, b.Status, b.IsMain, b.Phone,
		).Scan(&b.ID, &b.PublicID, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return err
		}

		if err := saveBranchInstitutionalWorksTx(txCtx, tx, b.ID, b.InstitutionalWorks); err != nil {
			return err
		}
		return nil
	})
}

// UpdateBranch updates an existing branch.
func (r *Repository) UpdateBranch(ctx context.Context, b *org.Branch) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE org.branches
			SET name = COALESCE($1, name),
			    code = COALESCE(NULLIF($2, ''), code),
			    address = COALESCE($3, address),
			    city_id = $4,
			    latitude = $5,
			    longitude = $6,
			    google_maps_url = COALESCE($7, google_maps_url),
			    manager_id = $8,
			    manager_name = COALESCE($9, manager_name),
			    warehouse_type = COALESCE(NULLIF($10, ''), warehouse_type),
			    has_cold_storage = COALESCE($11, has_cold_storage),
			    capacity_sqm = COALESCE($12, capacity_sqm),
			    operating_hours = COALESCE($13, operating_hours),
			    status = COALESCE(NULLIF($14, ''), status),
			    is_main = COALESCE($15, is_main),
			    phone = COALESCE($16, phone),
			    updated_at = now()
			WHERE id = $17 AND organization_id = $18;
		`
		tag, err := tx.Exec(txCtx, query,
			b.Name, b.Code, b.Address, b.CityID, b.Latitude, b.Longitude,
			b.GoogleMapsURL, b.ManagerID, b.ManagerName, b.WarehouseType, b.HasColdStorage,
			b.CapacitySQM, b.OperatingHours, b.Status, b.IsMain, b.Phone,
			b.ID, b.OrganizationID,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("branch")
		}

		if err := saveBranchInstitutionalWorksTx(txCtx, tx, b.ID, b.InstitutionalWorks); err != nil {
			return err
		}
		return nil
	})
}

// DeleteBranch soft-deletes a branch.
func (r *Repository) DeleteBranch(ctx context.Context, id, orgID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE org.branches SET deleted_at = now(), status = 'inactive' WHERE id = $1 AND organization_id = $2;`
		tag, err := tx.Exec(txCtx, query, id, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("branch")
		}
		return nil
	})
}

// UnsetMainBranches unsets is_main on all branches for an organization.
func (r *Repository) UnsetMainBranches(ctx context.Context, orgID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE org.branches SET is_main = false WHERE organization_id = $1;`
		_, err := tx.Exec(txCtx, query, orgID)
		return err
	})
}

// AssignBranchManager assigns a designated manager user to a branch.
func (r *Repository) AssignBranchManager(ctx context.Context, orgID, branchID int64, managerUserID *int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE org.branches SET manager_id = $1, updated_at = now() WHERE id = $2 AND organization_id = $3;`
		tag, err := tx.Exec(txCtx, query, managerUserID, branchID, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("branch")
		}
		return nil
	})
}

// GetBranchByID retrieves a branch by ID with manager info.
func (r *Repository) GetBranchByID(ctx context.Context, id int64) (*org.Branch, error) {
	var b org.Branch
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
			       COALESCE(b.status, 'active'), b.is_main, COALESCE(b.phone, ''), b.created_at, b.updated_at
			FROM org.branches b
			LEFT JOIN platform_admin.cities c ON c.id = b.city_id
			LEFT JOIN identity.users u ON u.id = b.manager_id
			WHERE b.id = $1 AND b.deleted_at IS NULL;
		`
		err := tx.QueryRow(txCtx, query, id).Scan(
			&b.ID, &b.PublicID, &b.OrganizationID, &b.Name,
			&b.Code, &b.Address, &b.CityID, &b.Latitude, &b.Longitude,
			&b.GoogleMapsURL, &b.ManagerID, &b.ManagerName,
			&b.ManagerEmail, &b.ManagerPhone,
			&b.WarehouseType, &b.HasColdStorage,
			&b.CapacitySQM, &b.OperatingHours,
			&b.Status, &b.IsMain, &b.Phone, &b.CreatedAt, &b.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("branch")
			}
			return err
		}

		iwRows, _ := tx.Query(txCtx, `SELECT DISTINCT COALESCE(institutional_work_id::text, work_category) FROM org.branch_institutional_works WHERE branch_id = $1`, b.ID)
		if iwRows != nil {
			for iwRows.Next() {
				var cat string
				if err := iwRows.Scan(&cat); err == nil && cat != "" {
					b.InstitutionalWorks = append(b.InstitutionalWorks, cat)
				}
			}
			iwRows.Close()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// ListBranchesByOrg returns all active branches for an organization.
// Institutional works ride along as a single aggregate subquery instead of a
// follow-up query per branch.
func (r *Repository) ListBranchesByOrg(ctx context.Context, orgID int64) ([]*org.Branch, error) {
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
			WHERE ($1::bigint = 0 OR b.organization_id = $1) AND b.deleted_at IS NULL
			ORDER BY b.is_main DESC, b.id ASC;
		`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
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
	return list, err
}

// AddMember adds a user to an organization with full employee attributes.
func (r *Repository) AddMember(ctx context.Context, m *org.Member) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO org.members (
				organization_id, user_id, branch_id, role_id, org_role_id, role_key,
				employee_code, job_title, base_salary, variable_salary, is_active
			) VALUES (
				$1, $2, $3, $4, $5, COALESCE(NULLIF($6, ''), 'org_employee'),
				NULLIF($7, ''), NULLIF($8, ''), $9, $10, $11
			)
			ON CONFLICT (organization_id, user_id) DO UPDATE
			SET branch_id = EXCLUDED.branch_id,
			    role_id = EXCLUDED.role_id,
			    org_role_id = EXCLUDED.org_role_id,
			    role_key = EXCLUDED.role_key,
			    employee_code = COALESCE(NULLIF(EXCLUDED.employee_code, ''), org.members.employee_code),
			    job_title = COALESCE(NULLIF(EXCLUDED.job_title, ''), org.members.job_title),
			    base_salary = CASE WHEN EXCLUDED.base_salary > 0 THEN EXCLUDED.base_salary ELSE org.members.base_salary END,
			    is_active = EXCLUDED.is_active,
			    updated_at = now()
			RETURNING id, created_at, updated_at;
		`
		var roleID *int64
		if m.RoleID > 0 {
			roleID = &m.RoleID
		}
		var orgRoleID *int64
		if m.OrgRoleID != nil && *m.OrgRoleID > 0 {
			orgRoleID = m.OrgRoleID
		} else if roleID != nil {
			orgRoleID = roleID
		}
		if err := tx.QueryRow(txCtx, query,
			m.OrganizationID, m.UserID, m.BranchID, roleID, orgRoleID, m.RoleKey,
			m.EmployeeCode, m.JobTitle, m.BaseSalary, m.VariableSalary, m.IsActive,
		).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return err
		}

		if m.IsActive {
			_, _ = tx.Exec(txCtx, `
				UPDATE identity.users
				SET status = 'active',
				    role = CASE WHEN role = 'job_seeker' THEN 'user' ELSE role END,
				    updated_at = now()
				WHERE id = $1 AND (status = 'pending' OR role = 'job_seeker');
			`, m.UserID)
		}
		return rbac.BumpVersion(txCtx, tx, rbac.OrgVersionKey(m.OrganizationID))
	})
}

func saveBranchInstitutionalWorksTx(ctx context.Context, tx pgx.Tx, branchID int64, works []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM org.branch_institutional_works WHERE branch_id = $1;`, branchID); err != nil {
		return err
	}
	for _, cat := range works {
		cat = strings.TrimSpace(cat)
		if cat == "" {
			continue
		}
		wID, _ := strconv.ParseInt(cat, 10, 64)
		tag, err := tx.Exec(ctx, `
			INSERT INTO org.branch_institutional_works (branch_id, work_category, institutional_work_id)
			SELECT $1, $2, iw.id
			FROM org.institutional_works iw
			WHERE (iw.id = $3 OR iw.slug = $2) AND iw.deleted_at IS NULL
			LIMIT 1
			ON CONFLICT (branch_id, work_category) DO UPDATE SET institutional_work_id = EXCLUDED.institutional_work_id;
		`, branchID, cat, wID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			if _, err := tx.Exec(ctx, `
				INSERT INTO org.branch_institutional_works (branch_id, work_category)
				VALUES ($1, $2) ON CONFLICT (branch_id, work_category) DO NOTHING;
			`, branchID, cat); err != nil {
				return err
			}
		}
	}
	return nil
}
