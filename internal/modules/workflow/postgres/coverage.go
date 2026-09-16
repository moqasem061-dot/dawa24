package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// coverageColumns is the canonical SELECT list for workflow.weekly_coverages.
//
// coverage_from and coverage_to are Postgres TIME columns and MUST be read
// through to_char. Scanning a TIME (OID 1083) straight into a Go *string fails
// — pgx maps TIME to pgtype.Time — which is what made the whole coverage screen
// error out. Every read of this table goes through this constant so a new query
// cannot reintroduce the bug.
const coverageColumns = `id, public_id, organization_id, branch_id, governorate_id, city_id, day_of_week,
	       to_char(coverage_from, 'HH24:MI') AS coverage_from,
	       to_char(coverage_to,   'HH24:MI') AS coverage_to,
	       address, latitude, longitude, distance_meters, is_active, created_at, updated_at`

// coverageTimeParam casts a bound parameter to TIME for the write path. The
// domain sends *string ("HH:MM" or nil); NULLIF guards the case where a caller
// bypasses workflow.TimeOfDay and passes an empty string, which Postgres would
// otherwise reject with `invalid input syntax for type time: ""`.
func coverageTimeParam(n int) string {
	return fmt.Sprintf("NULLIF($%d, '')::time", n)
}

// SaveWeeklyCoverage creates or updates weekly route coverage for a branch.
func (r *Repository) SaveWeeklyCoverage(ctx context.Context, c *workflow.WeeklyCoverage) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO workflow.weekly_coverages (
				organization_id, branch_id, governorate_id, city_id, day_of_week, coverage_from, coverage_to,
				address, latitude, longitude, distance_meters, is_active
			) VALUES ($1, $2, $3, $4, $5, ` + coverageTimeParam(6) + `, ` + coverageTimeParam(7) + `, $8, $9, $10, $11, $12)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			c.OrganizationID, c.BranchID, c.GovernorateID, c.CityID, c.DayOfWeek, c.CoverageFrom, c.CoverageTo,
			c.Address, c.Latitude, c.Longitude, c.DistanceMeters, c.IsActive,
		).Scan(&c.ID, &c.PublicID, &c.CreatedAt, &c.UpdatedAt)
	})
}

// SaveBatchWeeklyCoverage inserts multiple weekly coverage records within a single transaction.
func (r *Repository) SaveBatchWeeklyCoverage(ctx context.Context, coverages []*workflow.WeeklyCoverage) error {
	if len(coverages) == 0 {
		return nil
	}
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO workflow.weekly_coverages (
				organization_id, branch_id, governorate_id, city_id, day_of_week, coverage_from, coverage_to,
				address, latitude, longitude, distance_meters, is_active
			) VALUES ($1, $2, $3, $4, $5, ` + coverageTimeParam(6) + `, ` + coverageTimeParam(7) + `, $8, $9, $10, $11, $12)
			RETURNING id, public_id, created_at, updated_at;
		`
		for _, c := range coverages {
			if err := tx.QueryRow(txCtx, query,
				c.OrganizationID, c.BranchID, c.GovernorateID, c.CityID, c.DayOfWeek, c.CoverageFrom, c.CoverageTo,
				c.Address, c.Latitude, c.Longitude, c.DistanceMeters, c.IsActive,
			).Scan(&c.ID, &c.PublicID, &c.CreatedAt, &c.UpdatedAt); err != nil {
				return err
			}
		}
		return nil
	})
}

// UpdateWeeklyCoverage updates an existing weekly coverage record.
func (r *Repository) UpdateWeeklyCoverage(ctx context.Context, c *workflow.WeeklyCoverage) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE workflow.weekly_coverages
			SET branch_id = $1, governorate_id = $2, city_id = $3, day_of_week = $4,
			    coverage_from = ` + coverageTimeParam(5) + `, coverage_to = ` + coverageTimeParam(6) + `,
			    address = $7, latitude = $8, longitude = $9, distance_meters = $10, is_active = $11,
			    updated_at = now()
			WHERE id = $12 AND organization_id = $13
			RETURNING updated_at;
		`
		err := tx.QueryRow(txCtx, query,
			c.BranchID, c.GovernorateID, c.CityID, c.DayOfWeek, c.CoverageFrom, c.CoverageTo,
			c.Address, c.Latitude, c.Longitude, c.DistanceMeters, c.IsActive,
			c.ID, c.OrganizationID,
		).Scan(&c.UpdatedAt)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("weekly_coverage")
			}
			return err
		}
		return nil
	})
}

// DeleteWeeklyCoverage removes a weekly coverage entry.
func (r *Repository) DeleteWeeklyCoverage(ctx context.Context, id int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM workflow.weekly_coverages WHERE id = $1;`
		ct, err := tx.Exec(txCtx, query, id)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return apperr.NotFound("weekly_coverage")
		}
		return nil
	})
}

// DeleteAllCoverageForOrganization removes all weekly coverage records belonging to an organization.
func (r *Repository) DeleteAllCoverageForOrganization(ctx context.Context, orgID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM workflow.weekly_coverages WHERE organization_id = $1;`
		_, err := tx.Exec(txCtx, query, orgID)
		return err
	})
}

// ToggleWeeklyCoverage toggles the active state of a coverage record.
func (r *Repository) ToggleWeeklyCoverage(ctx context.Context, id int64, isActive bool) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE workflow.weekly_coverages SET is_active = $1, updated_at = now() WHERE id = $2;`
		ct, err := tx.Exec(txCtx, query, isActive, id)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return apperr.NotFound("weekly_coverage")
		}
		return nil
	})
}

// GetWeeklyCoverageByID retrieves a single weekly coverage record.
func (r *Repository) GetWeeklyCoverageByID(ctx context.Context, id int64) (*workflow.WeeklyCoverage, error) {
	var c workflow.WeeklyCoverage
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ` + coverageColumns + `
			FROM workflow.weekly_coverages
			WHERE id = $1;
		`
		err := tx.QueryRow(txCtx, query, id).Scan(
			&c.ID, &c.PublicID, &c.OrganizationID, &c.BranchID, &c.GovernorateID, &c.CityID, &c.DayOfWeek,
			&c.CoverageFrom, &c.CoverageTo, &c.Address, &c.Latitude, &c.Longitude,
			&c.DistanceMeters, &c.IsActive, &c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("weekly_coverage")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListWeeklyCoverage retrieves route schedules for a branch.
func (r *Repository) ListWeeklyCoverage(ctx context.Context, branchID int64) ([]*workflow.WeeklyCoverage, error) {
	var list []*workflow.WeeklyCoverage
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ` + coverageColumns + `
			FROM workflow.weekly_coverages
			WHERE branch_id = $1
			ORDER BY day_of_week ASC;
		`
		rows, err := tx.Query(txCtx, query, branchID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c workflow.WeeklyCoverage
			if err := rows.Scan(
				&c.ID, &c.PublicID, &c.OrganizationID, &c.BranchID, &c.GovernorateID, &c.CityID, &c.DayOfWeek,
				&c.CoverageFrom, &c.CoverageTo, &c.Address, &c.Latitude, &c.Longitude,
				&c.DistanceMeters, &c.IsActive, &c.CreatedAt, &c.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &c)
		}
		return rows.Err()
	})
	return list, err
}

// ListCoverageForOrganization retrieves all weekly coverage records for an organization with joined branch, governorate, city and org names.
func (r *Repository) ListCoverageForOrganization(ctx context.Context, orgID int64) ([]*workflow.CoverageView, error) {
	var list []*workflow.CoverageView
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT wc.id, wc.public_id, wc.organization_id, wc.branch_id, wc.governorate_id, wc.city_id,
			       wc.day_of_week,
			       to_char(wc.coverage_from, 'HH24:MI') AS coverage_from,
			       to_char(wc.coverage_to,   'HH24:MI') AS coverage_to,
			       wc.address,
			       COALESCE(wc.latitude, c.latitude) AS latitude,
			       COALESCE(wc.longitude, c.longitude) AS longitude,
			       wc.distance_meters, wc.is_active,
			       wc.created_at, wc.updated_at,
			       COALESCE(b.name->>'ar', b.name->>'en', b.name::text, '') AS branch_name,
			       COALESCE(g.name->>'ar', g.name->>'en', '') AS gov_name_ar,
			       COALESCE(g.name->>'en', g.name->>'ar', '') AS gov_name_en,
			       COALESCE(c.name->>'ar', c.name->>'en', '') AS city_name_ar,
			       COALESCE(c.name->>'en', c.name->>'ar', '') AS city_name_en,
			       COALESCE(o.legal_name, o.name->>'ar', o.name->>'en', '') AS org_name
			FROM workflow.weekly_coverages wc
			LEFT JOIN org.organizations o ON o.id = wc.organization_id
			LEFT JOIN org.branches b ON b.id = wc.branch_id AND b.deleted_at IS NULL
			LEFT JOIN platform_admin.cities c ON c.id = wc.city_id
			LEFT JOIN platform_admin.governorates g ON g.id = COALESCE(wc.governorate_id, c.governorate_id)
			WHERE ($1 = 0 OR wc.organization_id = $1)
			ORDER BY wc.day_of_week ASC, gov_name_ar ASC, city_name_ar ASC, branch_name ASC;
		`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v workflow.CoverageView
			if err := rows.Scan(
				&v.ID, &v.PublicID, &v.OrganizationID, &v.BranchID, &v.GovernorateID, &v.CityID,
				&v.DayOfWeek, &v.CoverageFrom, &v.CoverageTo, &v.Address,
				&v.Latitude, &v.Longitude, &v.DistanceMeters, &v.IsActive,
				&v.CreatedAt, &v.UpdatedAt,
				&v.BranchName, &v.GovernorateNameAr, &v.GovernorateName, &v.CityNameAr, &v.CityNameEn, &v.OrgName,
			); err != nil {
				return err
			}
			v.CityName = v.CityNameAr
			if v.CityName == "" {
				v.CityName = v.CityNameEn
			}
			list = append(list, &v)
		}
		return rows.Err()
	})
	return list, err
}

// ListCoverageForOrganizationWithTotal retrieves paginated weekly coverage records for an organization with total count.
func (r *Repository) ListCoverageForOrganizationWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*workflow.CoverageView, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}

	var list []*workflow.CoverageView
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const countQuery = `SELECT count(*) FROM workflow.weekly_coverages wc WHERE ($1 = 0 OR wc.organization_id = $1);`
		if err := tx.QueryRow(txCtx, countQuery, orgID).Scan(&total); err != nil {
			return err
		}

		query := `
			SELECT wc.id, wc.public_id, wc.organization_id, wc.branch_id, wc.governorate_id, wc.city_id,
			       wc.day_of_week,
			       to_char(wc.coverage_from, 'HH24:MI') AS coverage_from,
			       to_char(wc.coverage_to,   'HH24:MI') AS coverage_to,
			       wc.address,
			       COALESCE(wc.latitude, b.latitude, c.latitude, 30.0444) AS latitude,
			       COALESCE(wc.longitude, b.longitude, c.longitude, 31.2357) AS longitude,
			       wc.distance_meters, wc.is_active,
			       wc.created_at, wc.updated_at,
			       COALESCE(b.name->>'ar', b.name->>'en', b.name::text, '') AS branch_name,
			       COALESCE(g.name->>'ar', g.name->>'en', '') AS gov_name_ar,
			       COALESCE(g.name->>'en', g.name->>'ar', '') AS gov_name_en,
			       COALESCE(c.name->>'ar', c.name->>'en', '') AS city_name_ar,
			       COALESCE(c.name->>'en', c.name->>'ar', '') AS city_name_en,
			       COALESCE(o.legal_name, o.name->>'ar', o.name->>'en', '') AS org_name
			FROM workflow.weekly_coverages wc
			LEFT JOIN org.organizations o ON o.id = wc.organization_id
			LEFT JOIN org.branches b ON b.id = wc.branch_id AND b.deleted_at IS NULL
			LEFT JOIN platform_admin.cities c ON c.id = wc.city_id
			LEFT JOIN platform_admin.governorates g ON g.id = COALESCE(wc.governorate_id, c.governorate_id)
			WHERE ($1 = 0 OR wc.organization_id = $1)
			ORDER BY wc.day_of_week ASC, wc.id DESC
			LIMIT $2 OFFSET $3;
		`
		rows, err := tx.Query(txCtx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v workflow.CoverageView
			if err := rows.Scan(
				&v.ID, &v.PublicID, &v.OrganizationID, &v.BranchID, &v.GovernorateID, &v.CityID,
				&v.DayOfWeek, &v.CoverageFrom, &v.CoverageTo, &v.Address,
				&v.Latitude, &v.Longitude, &v.DistanceMeters, &v.IsActive,
				&v.CreatedAt, &v.UpdatedAt,
				&v.BranchName, &v.GovernorateNameAr, &v.GovernorateName, &v.CityNameAr, &v.CityNameEn, &v.OrgName,
			); err != nil {
				return err
			}
			v.CityName = v.CityNameAr
			if v.CityName == "" {
				v.CityName = v.CityNameEn
			}
			list = append(list, &v)
		}
		return rows.Err()
	})
	return list, total, err
}

// BranchOrganization is the organisation a live branch belongs to, or 0.
func (r *Repository) BranchOrganization(ctx context.Context, branchID int64) (int64, error) {
	var orgID int64
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT organization_id FROM org.branches WHERE id = $1 AND deleted_at IS NULL`, branchID).Scan(&orgID)
	})
	if database.IsNotFound(err) {
		return 0, nil
	}
	return orgID, err
}
