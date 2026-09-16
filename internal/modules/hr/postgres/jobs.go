package postgres

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

const jobColumns = `id, public_id, organization_id, category_id, title, description, requirements, salary_min, salary_max, location, status, created_at, updated_at`

// ListPublishedJobs returns published vacancies for the public job board.
func (r *Repository) ListPublishedJobs(ctx context.Context, limit, offset int) ([]*hr.JobOffer, error) {
	var list []*hr.JobOffer
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT ` + jobColumns + ` FROM hr.job_offers WHERE status IN ('published', 'active', 'open') AND deleted_at IS NULL ORDER BY created_at DESC LIMIT $1 OFFSET $2;`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
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
	return list, err
}

// GetJobOfferByID fetches one vacancy.
func (r *Repository) GetJobOfferByID(ctx context.Context, id int64) (*hr.JobOffer, error) {
	var j hr.JobOffer
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT ` + jobColumns + ` FROM hr.job_offers WHERE id = $1 AND deleted_at IS NULL;`
		err := tx.QueryRow(txCtx, query, id).Scan(&j.ID, &j.PublicID, &j.OrganizationID, &j.CategoryID, &j.Title, &j.Description, &j.Requirements, &j.SalaryMin, &j.SalaryMax, &j.Location, &j.Status, &j.CreatedAt, &j.UpdatedAt)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("job_offer")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// CreateJobOffer inserts a vacancy.
func (r *Repository) CreateJobOffer(ctx context.Context, j *hr.JobOffer) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO hr.job_offers (organization_id, category_id, title, description, requirements, salary_min, salary_max, location, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, public_id, created_at, updated_at;
		`
		if err := tx.QueryRow(txCtx, query, j.OrganizationID, j.CategoryID, j.Title, j.Description, j.Requirements, j.SalaryMin, j.SalaryMax, j.Location, j.Status).
			Scan(&j.ID, &j.PublicID, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return err
		}
		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: &j.OrganizationID,
			Action:         "hr.job.create",
			EntityType:     "job_offer",
			EntityID:       strconv.FormatInt(j.ID, 10),
			After:          j,
		})
		return nil
	})
}

// ListJobsByOrg returns a tenant's own postings.
func (r *Repository) ListJobsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*hr.JobOffer, error) {
	var list []*hr.JobOffer
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT ` + jobColumns + ` FROM hr.job_offers WHERE organization_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT $2 OFFSET $3;`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
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
	return list, err
}

// CreateJobApplication inserts an application.
func (r *Repository) CreateJobApplication(ctx context.Context, a *hr.JobApplication) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO hr.job_applications (job_offer_id, organization_id, applicant_user_id, applicant_name, applicant_email, applicant_phone, cv_storage_key, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query, a.JobOfferID, a.OrganizationID, a.ApplicantUserID, a.ApplicantName, a.ApplicantEmail, a.ApplicantPhone, a.CVStorageKey, a.Status).
			Scan(&a.ID, &a.PublicID, &a.CreatedAt, &a.UpdatedAt)
	})
}

// GetApplicationByID fetches a single application.
func (r *Repository) GetApplicationByID(ctx context.Context, id int64) (*hr.JobApplication, error) {
	var a hr.JobApplication
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT a.id, a.public_id, a.job_offer_id, a.organization_id, a.applicant_user_id,
			       a.applicant_name, a.applicant_email, a.applicant_phone, a.cv_storage_key,
			       a.status, a.notes, a.branch_id, COALESCE(b.name->>'ar', b.name->>'en', ''),
			       COALESCE(a.assigned_role_key, ''), COALESCE(jo.title->>'ar', jo.title->>'en', ''),
			       a.created_at, a.updated_at
			FROM hr.job_applications a
			LEFT JOIN hr.job_offers jo ON jo.id = a.job_offer_id
			LEFT JOIN org.branches b ON b.id = a.branch_id
			WHERE a.id = $1;
		`
		return tx.QueryRow(txCtx, query, id).Scan(
			&a.ID, &a.PublicID, &a.JobOfferID, &a.OrganizationID, &a.ApplicantUserID,
			&a.ApplicantName, &a.ApplicantEmail, &a.ApplicantPhone, &a.CVStorageKey,
			&a.Status, &a.Notes, &a.BranchID, &a.BranchName,
			&a.AssignedRoleKey, &a.JobTitle,
			&a.CreatedAt, &a.UpdatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("job_application")
		}
		return nil, err
	}
	return &a, nil
}

// ListApplicationsByOffer returns applications for a vacancy.
func (r *Repository) ListApplicationsByOffer(ctx context.Context, offerID int64, limit, offset int) ([]*hr.JobApplication, error) {
	var list []*hr.JobApplication
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT a.id, a.public_id, a.job_offer_id, a.organization_id, a.applicant_user_id,
			       a.applicant_name, a.applicant_email, a.applicant_phone, a.cv_storage_key,
			       a.status, a.notes, a.branch_id, COALESCE(b.name->>'ar', b.name->>'en', ''),
			       COALESCE(a.assigned_role_key, ''), COALESCE(jo.title->>'ar', jo.title->>'en', ''),
			       a.created_at, a.updated_at
			FROM hr.job_applications a
			LEFT JOIN hr.job_offers jo ON jo.id = a.job_offer_id
			LEFT JOIN org.branches b ON b.id = a.branch_id
			WHERE a.job_offer_id = $1
			ORDER BY a.created_at DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 50
		}
		rows, err := tx.Query(txCtx, query, offerID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a hr.JobApplication
			if err := rows.Scan(
				&a.ID, &a.PublicID, &a.JobOfferID, &a.OrganizationID, &a.ApplicantUserID,
				&a.ApplicantName, &a.ApplicantEmail, &a.ApplicantPhone, &a.CVStorageKey,
				&a.Status, &a.Notes, &a.BranchID, &a.BranchName,
				&a.AssignedRoleKey, &a.JobTitle,
				&a.CreatedAt, &a.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &a)
		}
		return rows.Err()
	})
	return list, err
}

// ListApplicationsByUser returns all applications submitted by a user.
func (r *Repository) ListApplicationsByUser(ctx context.Context, userID int64) ([]*hr.JobApplication, error) {
	var list []*hr.JobApplication
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT a.id, a.public_id, a.job_offer_id, a.organization_id, a.applicant_user_id,
			       a.applicant_name, a.applicant_email, a.applicant_phone, a.cv_storage_key,
			       a.status, a.notes, a.branch_id, COALESCE(b.name->>'ar', b.name->>'en', ''),
			       COALESCE(a.assigned_role_key, ''), COALESCE(jo.title->>'ar', jo.title->>'en', ''),
			       a.created_at, a.updated_at
			FROM hr.job_applications a
			LEFT JOIN hr.job_offers jo ON jo.id = a.job_offer_id
			LEFT JOIN org.branches b ON b.id = a.branch_id
			WHERE a.applicant_user_id = $1
			ORDER BY a.created_at DESC;
		`
		rows, err := tx.Query(txCtx, query, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a hr.JobApplication
			if err := rows.Scan(
				&a.ID, &a.PublicID, &a.JobOfferID, &a.OrganizationID, &a.ApplicantUserID,
				&a.ApplicantName, &a.ApplicantEmail, &a.ApplicantPhone, &a.CVStorageKey,
				&a.Status, &a.Notes, &a.BranchID, &a.BranchName,
				&a.AssignedRoleKey, &a.JobTitle,
				&a.CreatedAt, &a.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &a)
		}
		return rows.Err()
	})
	return list, err
}

// AcceptAndOnboardApplicant accepts an application, links the user as an org member at the branch/role, and creates an employee record.
func (r *Repository) AcceptAndOnboardApplicant(ctx context.Context, in hr.AcceptApplicantInput) (*hr.JobApplication, error) {
	var app hr.JobApplication
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Fetch and lock application
		query := `
			SELECT a.id, a.public_id, a.job_offer_id, a.organization_id, a.applicant_user_id,
			       a.applicant_name, a.applicant_email, a.applicant_phone, a.cv_storage_key,
			       a.status, a.notes, COALESCE(jo.title->>'ar', jo.title->>'en', '')
			FROM hr.job_applications a
			JOIN hr.job_offers jo ON jo.id = a.job_offer_id
			WHERE a.id = $1 AND a.organization_id = $2
			FOR UPDATE OF a;
		`
		var offerTitle string
		err := tx.QueryRow(txCtx, query, in.ApplicationID, in.OrganizationID).Scan(
			&app.ID, &app.PublicID, &app.JobOfferID, &app.OrganizationID, &app.ApplicantUserID,
			&app.ApplicantName, &app.ApplicantEmail, &app.ApplicantPhone, &app.CVStorageKey,
			&app.Status, &app.Notes, &offerTitle,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("job_application")
			}
			return err
		}

		app.JobTitle = offerTitle
		if in.JobTitle != "" {
			app.JobTitle = in.JobTitle
		}

		roleKey := in.RoleKey
		if roleKey == "" {
			roleKey = "org_employee"
		}

		// 2. Update job application to accepted / hired
		updateAppQuery := `
			UPDATE hr.job_applications
			SET status = 'accepted', branch_id = $1, assigned_role_key = $2,
			    notes = CASE WHEN $3 <> '' THEN $3 ELSE notes END,
			    updated_at = now()
			WHERE id = $4
			RETURNING updated_at;
		`
		if err := tx.QueryRow(txCtx, updateAppQuery, in.BranchID, roleKey, in.Notes, in.ApplicationID).Scan(&app.UpdatedAt); err != nil {
			return err
		}
		app.Status = "accepted"
		app.BranchID = in.BranchID
		app.AssignedRoleKey = roleKey

		// 3. Onboard user into org.members and activate identity account
		var uid int64
		if app.ApplicantUserID != nil && *app.ApplicantUserID > 0 {
			uid = *app.ApplicantUserID
		} else if strings.TrimSpace(app.ApplicantEmail) != "" {
			_ = tx.QueryRow(txCtx, `SELECT id FROM identity.users WHERE lower(email) = lower($1) AND deleted_at IS NULL LIMIT 1;`, strings.TrimSpace(app.ApplicantEmail)).Scan(&uid)
		}

		if uid > 0 {
			// Upsert org.members
			memberQuery := `
				INSERT INTO org.members (organization_id, user_id, branch_id, role_key, status, joined_at, updated_at)
				VALUES ($1, $2, $3, $4, 'active', now(), now())
				ON CONFLICT (organization_id, user_id)
				DO UPDATE SET
					branch_id = EXCLUDED.branch_id,
					role_key = EXCLUDED.role_key,
					status = 'active',
					updated_at = now();
			`
			if _, err := tx.Exec(txCtx, memberQuery, in.OrganizationID, uid, in.BranchID, roleKey); err != nil {
				return err
			}

			// Activate account in identity.users immediately so it is not left pending review.
			userActivateQuery := `
				UPDATE identity.users
				SET status = 'active',
				    role = CASE WHEN role = 'job_seeker' THEN 'user' ELSE role END,
				    updated_at = now()
				WHERE id = $1 AND (status = 'pending' OR role = 'job_seeker');
			`
			if _, err := tx.Exec(txCtx, userActivateQuery, uid); err != nil {
				return err
			}
		}

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: &in.OrganizationID,
			Action:         "hr.application.accept",
			EntityType:     "job_application",
			EntityID:       strconv.FormatInt(in.ApplicationID, 10),
			After:          app,
		})

		return nil
	})
	if err != nil {
		return nil, err
	}
	return &app, nil
}

// RejectApplicant sets the application status to rejected with optional notes.
func (r *Repository) RejectApplicant(ctx context.Context, orgID, appID int64, notes string) (*hr.JobApplication, error) {
	var app hr.JobApplication
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE hr.job_applications
			SET status = 'rejected',
			    notes = CASE WHEN $1 <> '' THEN $1 ELSE notes END,
			    updated_at = now()
			WHERE id = $2 AND organization_id = $3
			RETURNING id, public_id, job_offer_id, organization_id, applicant_user_id,
			          applicant_name, applicant_email, applicant_phone, cv_storage_key,
			          status, notes, branch_id, assigned_role_key, created_at, updated_at;
		`
		if err := tx.QueryRow(txCtx, query, notes, appID, orgID).Scan(
			&app.ID, &app.PublicID, &app.JobOfferID, &app.OrganizationID, &app.ApplicantUserID,
			&app.ApplicantName, &app.ApplicantEmail, &app.ApplicantPhone, &app.CVStorageKey,
			&app.Status, &app.Notes, &app.BranchID, &app.AssignedRoleKey,
			&app.CreatedAt, &app.UpdatedAt,
		); err != nil {
			return err
		}

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: &orgID,
			Action:         "hr.application.reject",
			EntityType:     "job_application",
			EntityID:       strconv.FormatInt(appID, 10),
			After:          app,
		})

		return nil
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("job_application")
		}
		return nil, err
	}
	return &app, nil
}
