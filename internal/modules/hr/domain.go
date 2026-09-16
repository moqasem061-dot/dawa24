// Package hr manages employee profiles, salary compensation, and organizational work schedules.
package hr

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Employee represents a staff member within an organization.
type Employee struct {
	ID             int64        `json:"id"`
	PublicID       string       `json:"public_id"`
	OrganizationID int64        `json:"organization_id"`
	UserID         int64        `json:"user_id"`
	EmployeeCode   string       `json:"employee_code"`
	JobTitle       string       `json:"job_title"`
	BaseSalary     money.Amount `json:"base_salary"`
	VariableSalary money.Amount `json:"variable_salary"`
	Status         string       `json:"status"`
	HiredAt        *time.Time   `json:"hired_at,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// WorkTime represents recurring branch business hours.
type WorkTime struct {
	ID             int64     `json:"id"`
	PublicID       string    `json:"public_id"`
	OrganizationID int64     `json:"organization_id"`
	DayNameAr      string    `json:"day_name_ar"`
	DayNameEn      string    `json:"day_name_en"`
	OpenTime       string    `json:"open_time,omitempty"`
	CloseTime      string    `json:"close_time,omitempty"`
	IsClosed       bool      `json:"is_closed"`
	SortOrder      int       `json:"sort_order"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Validate ensures employee profile data is valid.
func (e *Employee) Validate() error {
	if e.UserID <= 0 {
		return apperr.Validation("employee.user_required", "User ID is required.", nil)
	}
	if e.EmployeeCode == "" {
		return apperr.Validation("employee.code_required", "Employee code is required.", nil)
	}
	if e.JobTitle == "" {
		return apperr.Validation("employee.title_required", "Job title is required.", nil)
	}
	return nil
}

// JobOffer is a published vacancy (hr.job_offers).
type JobOffer struct {
	ID             int64        `json:"id"`
	PublicID       string       `json:"public_id"`
	OrganizationID int64        `json:"organization_id"`
	CategoryID     *int64       `json:"category_id,omitempty"`
	Title          i18n.Text    `json:"title"`
	Description    string       `json:"description"`
	Requirements   string       `json:"requirements"`
	SalaryMin      money.Amount `json:"salary_min,omitempty"`
	SalaryMax      money.Amount `json:"salary_max,omitempty"`
	Location       string       `json:"location"`
	Status         string       `json:"status"` // published, closed
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// JobCategory is a grouping of job offers (hr.job_categories).
type JobCategory struct {
	ID   int64     `json:"id"`
	Name i18n.Text `json:"name"`
	Slug string    `json:"slug"`
}

// JobStatsResult aggregates job offer counts and application totals.
type JobStatsResult struct {
	PublishedCount    int
	ClosedCount       int
	TotalApplications int
}

// AdminJobFilter specifies filter options for listing vacancies in platform administration.
type AdminJobFilter struct {
	OrganizationID int64
	Status         string
	Search         string
	Limit          int
	Offset         int
}

// JobApplication is an application to a vacancy (hr.job_applications).
type JobApplication struct {
	ID              int64     `json:"id"`
	PublicID        string    `json:"public_id"`
	JobOfferID      int64     `json:"job_offer_id"`
	OrganizationID  int64     `json:"organization_id"`
	ApplicantUserID *int64    `json:"applicant_user_id,omitempty"`
	ApplicantName   string    `json:"applicant_name"`
	ApplicantEmail  string    `json:"applicant_email"`
	ApplicantPhone  string    `json:"applicant_phone"`
	CVStorageKey    string    `json:"cv_storage_key"`
	Status          string    `json:"status"` // pending, reviewed, hired, accepted, rejected
	Notes           string    `json:"notes"`
	BranchID        *int64    `json:"branch_id,omitempty"`
	BranchName      string    `json:"branch_name,omitempty"`
	AssignedRoleKey string    `json:"assigned_role_key,omitempty"`
	JobTitle        string    `json:"job_title,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// AcceptApplicantInput captures parameters for accepting an applicant and onboarding them as an employee.
type AcceptApplicantInput struct {
	ApplicationID  int64        `json:"application_id"`
	OrganizationID int64        `json:"organization_id"`
	BranchID       *int64       `json:"branch_id,omitempty"`
	RoleKey        string       `json:"role_key"`
	JobTitle       string       `json:"job_title"`
	BaseSalary     money.Amount `json:"base_salary"`
	Notes          string       `json:"notes"`
}

// JobSeekerProfile represents an individual professional seeking pharmacy/medical employment.
type JobSeekerProfile struct {
	ID              int64        `json:"id"`
	UserID          int64        `json:"user_id"`
	Specialisation  string       `json:"specialisation"` // pharmacist, assistant_pharmacist, sales_rep, accountant, warehouse_keeper
	YearsExperience int          `json:"years_experience"`
	CVDocumentID    *int64       `json:"cv_document_id,omitempty"`
	IsOpenToWork    bool         `json:"is_open_to_work"`
	ExpectedSalary  money.Amount `json:"expected_salary"`
	PreferredCityID *int64       `json:"preferred_city_id,omitempty"`
	Bio             string       `json:"bio"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}
