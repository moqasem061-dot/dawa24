package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Repository implements hr.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates an HR PostgreSQL repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// CreateEmployee inserts an employee under the active tenant. Employees live
// on org.members since 069; a user who is already a member is upgraded with
// the HR fields instead of conflicting on (organization_id, user_id).
func (r *Repository) CreateEmployee(ctx context.Context, e *hr.Employee) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO org.members (
				organization_id, user_id, role_key, status,
				employee_code, job_title, base_salary, variable_salary, hired_at
			) VALUES ($1, $2, 'org_pharmacist', $3, $4, $5, $6, $7, $8)
			ON CONFLICT (organization_id, user_id) DO UPDATE SET
				employee_code = EXCLUDED.employee_code,
				job_title = EXCLUDED.job_title,
				base_salary = EXCLUDED.base_salary,
				variable_salary = EXCLUDED.variable_salary,
				status = EXCLUDED.status,
				hired_at = EXCLUDED.hired_at,
				updated_at = now()
			RETURNING id, public_id, created_at, updated_at;
		`
		if err := tx.QueryRow(txCtx, query,
			e.OrganizationID, e.UserID, e.Status, e.EmployeeCode, e.JobTitle,
			e.BaseSalary, e.VariableSalary, e.HiredAt,
		).Scan(&e.ID, &e.PublicID, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return err
		}

		if e.Status == "active" {
			_, _ = tx.Exec(txCtx, `
				UPDATE identity.users
				SET status = 'active',
				    role = CASE WHEN role = 'job_seeker' THEN 'user' ELSE role END,
				    updated_at = now()
				WHERE id = $1 AND (status = 'pending' OR role = 'job_seeker');
			`, e.UserID)
		}
		return nil
	})
}

// GetEmployeeByID retrieves an employee profile (org.members since 069).
func (r *Repository) GetEmployeeByID(ctx context.Context, id int64) (*hr.Employee, error) {
	var e hr.Employee
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, user_id, employee_code, job_title,
			       base_salary, variable_salary, status, hired_at, created_at, updated_at
			FROM org.members
			WHERE id = $1;
		`
		return tx.QueryRow(txCtx, query, id).Scan(
			&e.ID, &e.PublicID, &e.OrganizationID, &e.UserID, &e.EmployeeCode, &e.JobTitle,
			&e.BaseSalary, &e.VariableSalary, &e.Status, &e.HiredAt, &e.CreatedAt, &e.UpdatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("employee")
		}
		return nil, err
	}
	return &e, nil
}

// ListEmployees retrieves paginated employees for the tenant (org.members
// since 069).
func (r *Repository) ListEmployees(ctx context.Context, limit, offset int) ([]*hr.Employee, error) {
	var list []*hr.Employee
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, user_id, employee_code, job_title,
			       base_salary, variable_salary, status, hired_at, created_at, updated_at
			FROM org.members
			WHERE employee_code <> ''
			ORDER BY id ASC
			LIMIT $1 OFFSET $2;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var e hr.Employee
			if err := rows.Scan(
				&e.ID, &e.PublicID, &e.OrganizationID, &e.UserID, &e.EmployeeCode, &e.JobTitle,
				&e.BaseSalary, &e.VariableSalary, &e.Status, &e.HiredAt, &e.CreatedAt, &e.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &e)
		}
		return rows.Err()
	})
	return list, err
}

// SaveWorkTimes saves recurring business hours.
//
// open_time/close_time are Postgres TIME columns while hr.WorkTime carries them
// as Go strings, so both directions need a conversion: NULLIF(...)::time on the
// way in (a closed day submits "", which Postgres rejects for TIME) and
// to_char(..., 'HH24:MI') on the way out (pgx maps TIME to pgtype.Time, never to
// string). This is the same defect that broke workflow.weekly_coverages — see
// workflow/postgres.coverageColumns.
func (r *Repository) SaveWorkTimes(ctx context.Context, times []*hr.WorkTime) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		for _, wt := range times {
			query := `
				INSERT INTO hr.work_times (
					organization_id, day_name_ar, day_name_en, open_time, close_time, is_closed, sort_order
				) VALUES ($1, $2, $3, NULLIF($4, '')::time, NULLIF($5, '')::time, $6, $7)
				RETURNING id, public_id, created_at, updated_at;
			`
			if err := tx.QueryRow(txCtx, query,
				wt.OrganizationID, wt.DayNameAr, wt.DayNameEn, wt.OpenTime, wt.CloseTime, wt.IsClosed, wt.SortOrder,
			).Scan(&wt.ID, &wt.PublicID, &wt.CreatedAt, &wt.UpdatedAt); err != nil {
				return err
			}
		}
		return nil
	})
}

// ListWorkTimes returns business hours for the active tenant.
func (r *Repository) ListWorkTimes(ctx context.Context) ([]*hr.WorkTime, error) {
	var list []*hr.WorkTime
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, day_name_ar, day_name_en,
			       COALESCE(to_char(open_time,  'HH24:MI'), '') AS open_time,
			       COALESCE(to_char(close_time, 'HH24:MI'), '') AS close_time,
			       is_closed, sort_order, created_at, updated_at
			FROM hr.work_times
			ORDER BY sort_order ASC;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var wt hr.WorkTime
			if err := rows.Scan(
				&wt.ID, &wt.PublicID, &wt.OrganizationID, &wt.DayNameAr, &wt.DayNameEn,
				&wt.OpenTime, &wt.CloseTime, &wt.IsClosed, &wt.SortOrder, &wt.CreatedAt, &wt.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &wt)
		}
		return rows.Err()
	})
	return list, err
}

// GetJobSeekerProfile retrieves the seeker profile for a user.
func (r *Repository) GetJobSeekerProfile(ctx context.Context, userID int64) (*hr.JobSeekerProfile, error) {
	var p hr.JobSeekerProfile
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, user_id, specialisation, years_experience, cv_document_id,
			       is_open_to_work, expected_salary, preferred_city_id, bio, created_at, updated_at
			FROM hr.job_seeker_profiles
			WHERE user_id = $1;
		`
		return tx.QueryRow(txCtx, query, userID).Scan(
			&p.ID, &p.UserID, &p.Specialisation, &p.YearsExperience, &p.CVDocumentID,
			&p.IsOpenToWork, &p.ExpectedSalary, &p.PreferredCityID, &p.Bio, &p.CreatedAt, &p.UpdatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("job_seeker_profile")
		}
		return nil, err
	}
	return &p, nil
}

// UpsertJobSeekerProfile saves or updates the job seeker profile.
func (r *Repository) UpsertJobSeekerProfile(ctx context.Context, p *hr.JobSeekerProfile) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO hr.job_seeker_profiles (
				user_id, specialisation, years_experience, cv_document_id,
				is_open_to_work, expected_salary, preferred_city_id, bio
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (user_id) DO UPDATE
			SET specialisation = EXCLUDED.specialisation,
			    years_experience = EXCLUDED.years_experience,
			    cv_document_id = EXCLUDED.cv_document_id,
			    is_open_to_work = EXCLUDED.is_open_to_work,
			    expected_salary = EXCLUDED.expected_salary,
			    preferred_city_id = EXCLUDED.preferred_city_id,
			    bio = EXCLUDED.bio,
			    updated_at = now()
			RETURNING id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			p.UserID, p.Specialisation, p.YearsExperience, p.CVDocumentID,
			p.IsOpenToWork, p.ExpectedSalary, p.PreferredCityID, p.Bio,
		).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	})
}
