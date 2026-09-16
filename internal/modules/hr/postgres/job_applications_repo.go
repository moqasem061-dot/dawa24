package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// UpdateJobOffer modifies job vacancy details.
func (r *Repository) UpdateJobOffer(ctx context.Context, j *hr.JobOffer) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			UPDATE hr.job_offers
			SET title = $1, description = $2, requirements = $3, salary_min = $4, salary_max = $5, location = $6, status = $7, updated_at = now()
			WHERE id = $8 AND organization_id = $9;
		`
		tag, err := tx.Exec(txCtx, query, j.Title, j.Description, j.Requirements, j.SalaryMin, j.SalaryMax, j.Location, j.Status, j.ID, j.OrganizationID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("job_offer")
		}

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: &j.OrganizationID,
			Action:         "hr.job.update",
			EntityType:     "job_offer",
			EntityID:       strconv.FormatInt(j.ID, 10),
			After:          j,
		})

		return nil
	})
}

// DeleteJobOffer soft deletes a job vacancy.
func (r *Repository) DeleteJobOffer(ctx context.Context, orgID, jobID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `UPDATE hr.job_offers SET deleted_at = now(), updated_at = now() WHERE id = $1 AND organization_id = $2;`
		tag, err := tx.Exec(txCtx, query, jobID, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("job_offer")
		}

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: &orgID,
			Action:         "hr.job.delete",
			EntityType:     "job_offer",
			EntityID:       strconv.FormatInt(jobID, 10),
		})

		return nil
	})
}

// ToggleJobOfferStatus toggles status between 'published' and 'closed'.
func (r *Repository) ToggleJobOfferStatus(ctx context.Context, orgID, jobID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			UPDATE hr.job_offers
			SET status = CASE WHEN status = 'published' THEN 'closed' ELSE 'published' END,
			    updated_at = now()
			WHERE id = $1 AND organization_id = $2;
		`
		tag, err := tx.Exec(txCtx, query, jobID, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("job_offer")
		}

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: &orgID,
			Action:         "hr.job.toggle",
			EntityType:     "job_offer",
			EntityID:       strconv.FormatInt(jobID, 10),
		})

		return nil
	})
}

// CountApplicationsByOffer returns total applicants for a job.
func (r *Repository) CountApplicationsByOffer(ctx context.Context, offerID int64) (int, error) {
	var count int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT COUNT(*) FROM hr.job_applications WHERE job_offer_id = $1;`
		return tx.QueryRow(txCtx, query, offerID).Scan(&count)
	})
	return count, err
}

// ListPublishedJobsWithTotal returns published vacancies and total count.
func (r *Repository) ListPublishedJobsWithTotal(ctx context.Context, limit, offset int) ([]*hr.JobOffer, int, error) {
	var list []*hr.JobOffer
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const countQuery = `SELECT count(*) FROM hr.job_offers WHERE status IN ('published', 'active', 'open') AND deleted_at IS NULL;`
		if err := tx.QueryRow(txCtx, countQuery).Scan(&total); err != nil {
			return err
		}

		if limit <= 0 || limit > 100 {
			limit = 20
		}
		if offset < 0 {
			offset = 0
		}

		const query = `SELECT id, public_id, organization_id, category_id, title, description, requirements, salary_min, salary_max, location, status, created_at, updated_at FROM hr.job_offers WHERE status IN ('published', 'active', 'open') AND deleted_at IS NULL ORDER BY created_at DESC, id DESC LIMIT $1 OFFSET $2;`
		rows, err := tx.Query(txCtx, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var j hr.JobOffer
			if err := rows.Scan(&j.ID, &j.PublicID, &j.OrganizationID, &j.CategoryID, &j.Title, &j.Description, &j.Requirements, &j.SalaryMin, &j.SalaryMax, &j.Location, &j.Status, &j.CreatedAt, &j.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &j)
		}
		return rows.Err()
	})
	return list, total, err
}

// ListAllJobsWithTotal returns all vacancies across all tenants for admin.
func (r *Repository) ListAllJobsWithTotal(ctx context.Context, limit, offset int) ([]*hr.JobOffer, int, error) {
	var list []*hr.JobOffer
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const countQuery = `SELECT count(*) FROM hr.job_offers WHERE deleted_at IS NULL;`
		if err := tx.QueryRow(txCtx, countQuery).Scan(&total); err != nil {
			return err
		}

		if limit <= 0 || limit > 100 {
			limit = 20
		}
		if offset < 0 {
			offset = 0
		}

		const query = `SELECT id, public_id, organization_id, category_id, title, description, requirements, salary_min, salary_max, location, status, created_at, updated_at FROM hr.job_offers WHERE deleted_at IS NULL ORDER BY created_at DESC, id DESC LIMIT $1 OFFSET $2;`
		rows, err := tx.Query(txCtx, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var j hr.JobOffer
			if err := rows.Scan(&j.ID, &j.PublicID, &j.OrganizationID, &j.CategoryID, &j.Title, &j.Description, &j.Requirements, &j.SalaryMin, &j.SalaryMax, &j.Location, &j.Status, &j.CreatedAt, &j.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &j)
		}
		return rows.Err()
	})
	return list, total, err
}

// ListAllJobsFiltered returns vacancies across all tenants filtered by criteria.
func (r *Repository) ListAllJobsFiltered(ctx context.Context, filter hr.AdminJobFilter) ([]*hr.JobOffer, int, error) {
	var list []*hr.JobOffer
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		whereClauses := []string{"deleted_at IS NULL"}
		var args []any
		argIdx := 1

		if filter.OrganizationID > 0 {
			whereClauses = append(whereClauses, fmt.Sprintf("organization_id = $%d", argIdx))
			args = append(args, filter.OrganizationID)
			argIdx++
		}

		if filter.Status != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argIdx))
			args = append(args, filter.Status)
			argIdx++
		}

		if s := strings.TrimSpace(filter.Search); s != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("(title->>'ar' ILIKE $%d OR title->>'en' ILIKE $%d OR location ILIKE $%d)", argIdx, argIdx, argIdx))
			args = append(args, "%"+s+"%")
			argIdx++
		}

		whereSQL := strings.Join(whereClauses, " AND ")

		countQuery := fmt.Sprintf("SELECT count(*) FROM hr.job_offers WHERE %s;", whereSQL)
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		limit := filter.Limit
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		offset := filter.Offset
		if offset < 0 {
			offset = 0
		}

		dataQuery := fmt.Sprintf(`SELECT id, public_id, organization_id, category_id, title, description, requirements, salary_min, salary_max, location, status, created_at, updated_at FROM hr.job_offers WHERE %s ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d;`, whereSQL, argIdx, argIdx+1)
		dataArgs := append(args, limit, offset)

		rows, err := tx.Query(txCtx, dataQuery, dataArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var j hr.JobOffer
			if err := rows.Scan(&j.ID, &j.PublicID, &j.OrganizationID, &j.CategoryID, &j.Title, &j.Description, &j.Requirements, &j.SalaryMin, &j.SalaryMax, &j.Location, &j.Status, &j.CreatedAt, &j.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &j)
		}
		return rows.Err()
	})
	return list, total, err
}

// ListJobsByOrgWithTotal returns a tenant's postings and total count.
func (r *Repository) ListJobsByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*hr.JobOffer, int, error) {
	var list []*hr.JobOffer
	var total int

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const countQuery = `SELECT count(*) FROM hr.job_offers WHERE organization_id = $1 AND deleted_at IS NULL;`
		if err := tx.QueryRow(txCtx, countQuery, orgID).Scan(&total); err != nil {
			return err
		}

		if limit <= 0 || limit > 100 {
			limit = 20
		}
		if offset < 0 {
			offset = 0
		}

		const query = `SELECT id, public_id, organization_id, category_id, title, description, requirements, salary_min, salary_max, location, status, created_at, updated_at FROM hr.job_offers WHERE organization_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3;`
		rows, err := tx.Query(txCtx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var j hr.JobOffer
			if err := rows.Scan(&j.ID, &j.PublicID, &j.OrganizationID, &j.CategoryID, &j.Title, &j.Description, &j.Requirements, &j.SalaryMin, &j.SalaryMax, &j.Location, &j.Status, &j.CreatedAt, &j.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &j)
		}
		return rows.Err()
	})
	return list, total, err
}

// GetJobStatsByOrg aggregates statistics for a tenant's job postings.
func (r *Repository) GetJobStatsByOrg(ctx context.Context, orgID int64) (hr.JobStatsResult, error) {
	var stats hr.JobStatsResult
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT 
				COUNT(*) FILTER (WHERE status = 'published' AND deleted_at IS NULL),
				COUNT(*) FILTER (WHERE status != 'published' AND deleted_at IS NULL),
				COALESCE((
					SELECT COUNT(*) 
					FROM hr.job_applications a 
					JOIN hr.job_offers o ON o.id = a.job_offer_id 
					WHERE o.organization_id = $1 AND o.deleted_at IS NULL
				), 0)
			FROM hr.job_offers
			WHERE organization_id = $1 AND deleted_at IS NULL;
		`
		return tx.QueryRow(txCtx, query, orgID).Scan(&stats.PublishedCount, &stats.ClosedCount, &stats.TotalApplications)
	})
	return stats, err
}
