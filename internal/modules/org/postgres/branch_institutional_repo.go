package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// AssignBranchInstitutionalWorks sets the institutional work categories for a branch.
func (r *Repository) AssignBranchInstitutionalWorks(ctx context.Context, branchID int64, workIDs []int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `DELETE FROM org.branch_institutional_works WHERE branch_id = $1;`, branchID); err != nil {
			return err
		}
		for _, wID := range workIDs {
			if wID > 0 {
				if _, err := tx.Exec(txCtx, `
					INSERT INTO org.branch_institutional_works (branch_id, institutional_work_id, work_category)
					VALUES ($1, $2::bigint, $2::bigint::text)
					ON CONFLICT (branch_id, work_category) DO UPDATE SET institutional_work_id = EXCLUDED.institutional_work_id;
				`, branchID, wID); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// GetBranchInstitutionalWorks retrieves all institutional works assigned to a branch.
func (r *Repository) GetBranchInstitutionalWorks(ctx context.Context, branchID int64) ([]*org.InstitutionalWork, error) {
	var list []*org.InstitutionalWork
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT iw.id, iw.public_id, iw.title, iw.description, iw.icon, iw.pricing_type,
			       iw.is_active, iw.view_type, iw.slug, iw.parent_id, '', 0, iw.created_at, iw.updated_at
			FROM org.institutional_works iw
			JOIN org.branch_institutional_works biw ON (biw.institutional_work_id = iw.id OR biw.work_category = iw.id::text OR biw.work_category = iw.slug)
			WHERE biw.branch_id = $1 AND iw.deleted_at IS NULL;
		`
		rows, err := tx.Query(txCtx, query, branchID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item org.InstitutionalWork
			var pricingStr string
			if err := rows.Scan(
				&item.ID, &item.PublicID, &item.Title, &item.Description, &item.Icon,
				&pricingStr, &item.IsActive, &item.ViewType, &item.Slug, &item.ParentID,
				&item.ParentTitle, &item.BranchCount, &item.CreatedAt, &item.UpdatedAt,
			); err != nil {
				return err
			}
			item.PricingType = org.PricingType(pricingStr)
			list = append(list, &item)
		}
		return rows.Err()
	})
	return list, err
}

// AssignEmployeeInstitutionalWork assigns an institutional work category to a user.
func (r *Repository) AssignEmployeeInstitutionalWork(ctx context.Context, orgID, userID, workID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO org.employee_institutional_works (organization_id, user_id, institutional_work_id, created_at, updated_at, deleted_at)
			VALUES ($1, $2, $3, now(), now(), NULL)
			ON CONFLICT (user_id, institutional_work_id) WHERE deleted_at IS NULL
			DO UPDATE SET organization_id = EXCLUDED.organization_id, updated_at = now(), deleted_at = NULL;
		`
		_, err := tx.Exec(txCtx, query, orgID, userID, workID)
		return err
	})
}

// RemoveEmployeeInstitutionalWork removes an institutional work assignment from a user.
func (r *Repository) RemoveEmployeeInstitutionalWork(ctx context.Context, orgID, userID, workID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			UPDATE org.employee_institutional_works
			SET deleted_at = now(), updated_at = now()
			WHERE user_id = $1 AND institutional_work_id = $2 AND (organization_id = $3 OR $3 = 0) AND deleted_at IS NULL;
		`
		_, err := tx.Exec(txCtx, query, userID, workID, orgID)
		return err
	})
}

// ListEmployeeInstitutionalWorks lists all institutional works assigned to a user.
func (r *Repository) ListEmployeeInstitutionalWorks(ctx context.Context, userID int64) ([]*org.EmployeeInstitutionalWork, error) {
	var list []*org.EmployeeInstitutionalWork
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT eiw.id, eiw.organization_id, eiw.user_id, eiw.institutional_work_id,
			       COALESCE(iw.title->>'ar', iw.title->>'en', ''), eiw.created_at, eiw.updated_at
			FROM org.employee_institutional_works eiw
			JOIN org.institutional_works iw ON eiw.institutional_work_id = iw.id
			WHERE eiw.user_id = $1 AND eiw.deleted_at IS NULL AND iw.deleted_at IS NULL
			ORDER BY eiw.id ASC;
		`
		rows, err := tx.Query(txCtx, query, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item org.EmployeeInstitutionalWork
			if err := rows.Scan(
				&item.ID, &item.OrganizationID, &item.UserID, &item.InstitutionalWorkID,
				&item.WorkTitle, &item.CreatedAt, &item.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &item)
		}
		return rows.Err()
	})
	return list, err
}

// ListOrgEmployeeInstitutionalWorks lists all institutional work assignments for an organization.
func (r *Repository) ListOrgEmployeeInstitutionalWorks(ctx context.Context, orgID int64) ([]*org.EmployeeInstitutionalWork, error) {
	var list []*org.EmployeeInstitutionalWork
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT eiw.id, eiw.organization_id, eiw.user_id, eiw.institutional_work_id,
			       COALESCE(iw.title->>'ar', iw.title->>'en', ''), eiw.created_at, eiw.updated_at
			FROM org.employee_institutional_works eiw
			JOIN org.institutional_works iw ON eiw.institutional_work_id = iw.id
			WHERE eiw.organization_id = $1 AND eiw.deleted_at IS NULL AND iw.deleted_at IS NULL
			ORDER BY eiw.id ASC;
		`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item org.EmployeeInstitutionalWork
			if err := rows.Scan(
				&item.ID, &item.OrganizationID, &item.UserID, &item.InstitutionalWorkID,
				&item.WorkTitle, &item.CreatedAt, &item.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &item)
		}
		return rows.Err()
	})
	return list, err
}

// GetUserInstitutionalWorkIDs fetches active work IDs for a user.
func (r *Repository) GetUserInstitutionalWorkIDs(ctx context.Context, userID int64) ([]int64, error) {
	var ids []int64
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT institutional_work_id
			FROM org.employee_institutional_works
			WHERE user_id = $1 AND deleted_at IS NULL;
		`
		rows, err := tx.Query(txCtx, query, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	return ids, err
}

// GetConnectedInstitutionalWorkIDs resolves to_institutional_work_id values for given source IDs.
func (r *Repository) GetConnectedInstitutionalWorkIDs(ctx context.Context, fromWorkIDs []int64) ([]int64, error) {
	if len(fromWorkIDs) == 0 {
		return []int64{}, nil
	}
	var ids []int64
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT DISTINCT to_institutional_work_id
			FROM org.institutional_work_connections
			WHERE from_institutional_work_id = ANY($1)
			ORDER BY to_institutional_work_id ASC;
		`
		rows, err := tx.Query(txCtx, query, fromWorkIDs)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	return ids, err
}

// AnyBranchHasInstitutionalWork checks if any branch in branchIDs holds any institutional work in workIDs.
func (r *Repository) AnyBranchHasInstitutionalWork(ctx context.Context, branchIDs []int64, workIDs []int64) (bool, error) {
	if len(branchIDs) == 0 || len(workIDs) == 0 {
		return false, nil
	}
	var exists bool
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT EXISTS (
				SELECT 1
				FROM org.branch_institutional_works biw
				LEFT JOIN org.institutional_works iw ON (biw.work_category = iw.slug OR biw.work_category = iw.id::text) AND iw.deleted_at IS NULL
				WHERE biw.branch_id = ANY($1)
				  AND COALESCE(biw.institutional_work_id, iw.id) = ANY($2)
			);
		`
		return tx.QueryRow(txCtx, query, branchIDs, workIDs).Scan(&exists)
	})
	return exists, err
}

// ListBranchesWithoutInstitutionalWorks lists active branches that have zero resolvable institutional works.
func (r *Repository) ListBranchesWithoutInstitutionalWorks(ctx context.Context) ([]*org.BranchWithoutWorks, error) {
	var list []*org.BranchWithoutWorks
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT b.id,
			       COALESCE(NULLIF(b.name->>'ar', ''), NULLIF(b.name->>'en', ''), 'الفرع'),
			       b.organization_id,
			       COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.trade_name->>'en', ''), NULLIF(o.legal_name, ''), 'المنشأة'),
			       o.type,
			       COALESCE(b.warehouse_type, 'warehouse')
			FROM org.branches b
			JOIN org.organizations o ON o.id = b.organization_id
			WHERE b.deleted_at IS NULL AND b.status <> 'inactive'
			  AND NOT EXISTS (
			      SELECT 1
			      FROM org.branch_institutional_works biw
			      LEFT JOIN org.institutional_works iw ON (biw.work_category = iw.slug OR biw.work_category = iw.id::text) AND iw.deleted_at IS NULL
			      WHERE biw.branch_id = b.id
			        AND COALESCE(biw.institutional_work_id, iw.id) IS NOT NULL
			  )
			ORDER BY o.id, b.id;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item org.BranchWithoutWorks
			if err := rows.Scan(
				&item.BranchID, &item.BranchName, &item.OrgID,
				&item.OrgName, &item.OrgType, &item.WarehouseType,
			); err != nil {
				return err
			}
			list = append(list, &item)
		}
		return rows.Err()
	})
	return list, err
}

// GetReachableBuyerWorksForBranch retrieves all buyer institutional works that have a connection
// to this branch's assigned institutional works.
func (r *Repository) GetReachableBuyerWorksForBranch(ctx context.Context, branchID int64) ([]*org.InstitutionalWork, error) {
	var list []*org.InstitutionalWork
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT DISTINCT iw.id, iw.public_id, iw.title, iw.description, iw.icon, iw.pricing_type,
			       iw.is_active, iw.view_type, iw.slug, iw.parent_id, '', 0, iw.created_at, iw.updated_at
			FROM org.institutional_works iw
			JOIN org.institutional_work_connections iwc ON iwc.from_institutional_work_id = iw.id
			JOIN org.branch_institutional_works biw ON (biw.institutional_work_id = iwc.to_institutional_work_id)
			WHERE biw.branch_id = $1 AND iw.deleted_at IS NULL
			ORDER BY iw.id ASC;
		`
		rows, err := tx.Query(txCtx, query, branchID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item org.InstitutionalWork
			var pricingStr string
			if err := rows.Scan(
				&item.ID, &item.PublicID, &item.Title, &item.Description, &item.Icon,
				&pricingStr, &item.IsActive, &item.ViewType, &item.Slug, &item.ParentID,
				&item.ParentTitle, &item.BranchCount, &item.CreatedAt, &item.UpdatedAt,
			); err != nil {
				return err
			}
			item.PricingType = org.PricingType(pricingStr)
			list = append(list, &item)
		}
		return rows.Err()
	})
	return list, err
}
