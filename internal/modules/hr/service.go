package hr

import (
	"context"
	"log/slog"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Service coordinates HR operations, payroll configuration, and business hours.
type Service struct {
	repo Repository
	log  *slog.Logger
}

// NewService creates a new HR service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// CreateEmployee onboards a new staff member.
func (s *Service) CreateEmployee(ctx context.Context, e *Employee) (*Employee, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	e.OrganizationID = orgID

	if err := e.Validate(); err != nil {
		return nil, err
	}
	if e.Status == "" {
		e.Status = "active"
	}

	if err := s.repo.CreateEmployee(ctx, e); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "employee onboarded", "employee_id", e.ID, "code", e.EmployeeCode)
	return e, nil
}

// GetEmployee retrieves an employee by ID.
func (s *Service) GetEmployee(ctx context.Context, id int64) (*Employee, error) {
	return s.repo.GetEmployeeByID(ctx, id)
}

// ListEmployees retrieves paginated employees for the tenant.
func (s *Service) ListEmployees(ctx context.Context, limit, offset int) ([]*Employee, error) {
	return s.repo.ListEmployees(ctx, limit, offset)
}

// SaveWorkTimes updates weekly business shifts.
func (s *Service) SaveWorkTimes(ctx context.Context, times []*WorkTime) error {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return database.ErrNoTenant
	}
	for _, wt := range times {
		wt.OrganizationID = orgID
	}
	return s.repo.SaveWorkTimes(ctx, times)
}

// ListWorkTimes returns business operating hours.
func (s *Service) ListWorkTimes(ctx context.Context) ([]*WorkTime, error) {
	return s.repo.ListWorkTimes(ctx)
}

// ListPublishedJobs returns published vacancies for the public job board.
func (s *Service) ListPublishedJobs(ctx context.Context, limit, offset int) ([]*JobOffer, error) {
	return s.repo.ListPublishedJobs(ctx, limit, offset)
}

// ListPublishedJobsWithTotal returns published vacancies and total count.
func (s *Service) ListPublishedJobsWithTotal(ctx context.Context, limit, offset int) ([]*JobOffer, int, error) {
	return s.repo.ListPublishedJobsWithTotal(ctx, limit, offset)
}

// ListAllJobsWithTotal returns all vacancies for platform admin.
func (s *Service) ListAllJobsWithTotal(ctx context.Context, limit, offset int) ([]*JobOffer, int, error) {
	return s.repo.ListAllJobsWithTotal(ctx, limit, offset)
}

// ListAllJobsFiltered returns filtered vacancies for platform admin.
func (s *Service) ListAllJobsFiltered(ctx context.Context, filter AdminJobFilter) ([]*JobOffer, int, error) {
	return s.repo.ListAllJobsFiltered(ctx, filter)
}

// GetJobOffer returns one vacancy.
func (s *Service) GetJobOffer(ctx context.Context, id int64) (*JobOffer, error) {
	return s.repo.GetJobOfferByID(ctx, id)
}

// CreateJobOffer publishes a vacancy for an organization.
func (s *Service) CreateJobOffer(ctx context.Context, orgID int64, j *JobOffer) (*JobOffer, error) {
	j.OrganizationID = orgID
	if j.Status == "" {
		j.Status = "published"
	}
	if err := s.repo.CreateJobOffer(ctx, j); err != nil {
		return nil, err
	}
	return j, nil
}

// ListOrgJobs returns a tenant's own postings.
func (s *Service) ListOrgJobs(ctx context.Context, orgID int64, limit, offset int) ([]*JobOffer, error) {
	return s.repo.ListJobsByOrg(ctx, orgID, limit, offset)
}

// ListOrgJobsWithTotal returns a tenant's own postings with total count.
func (s *Service) ListOrgJobsWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*JobOffer, int, error) {
	return s.repo.ListJobsByOrgWithTotal(ctx, orgID, limit, offset)
}

// GetJobStatsByOrg aggregates statistics for a tenant's job postings.
func (s *Service) GetJobStatsByOrg(ctx context.Context, orgID int64) (JobStatsResult, error) {
	return s.repo.GetJobStatsByOrg(ctx, orgID)
}

// DeleteJobOffer removes a job vacancy.
func (s *Service) DeleteJobOffer(ctx context.Context, orgID, jobID int64) error {
	return s.repo.DeleteJobOffer(ctx, orgID, jobID)
}

// ToggleJobOfferStatus toggles a job vacancy status between published and closed.
func (s *Service) ToggleJobOfferStatus(ctx context.Context, orgID, jobID int64) error {
	return s.repo.ToggleJobOfferStatus(ctx, orgID, jobID)
}

// UpdateJobOffer updates an existing job vacancy belonging to an organization.
func (s *Service) UpdateJobOffer(ctx context.Context, orgID int64, j *JobOffer) error {
	existing, err := s.repo.GetJobOfferByID(ctx, j.ID)
	if err != nil {
		return err
	}
	if existing == nil || existing.OrganizationID != orgID {
		return apperr.Forbidden("hr.forbidden", "not allowed to edit this job")
	}
	j.OrganizationID = orgID
	return s.repo.UpdateJobOffer(ctx, j)
}

// CountApplications returns application count for a vacancy.
func (s *Service) CountApplications(ctx context.Context, offerID int64) (int, error) {
	return s.repo.CountApplicationsByOffer(ctx, offerID)
}

// ApplyToJob records an application for a vacancy.
func (s *Service) ApplyToJob(ctx context.Context, a *JobApplication) error {
	if a.ApplicantName == "" || a.ApplicantEmail == "" {
		return apperr.Validation("job.apply_required", "Name and email are required.", nil)
	}
	if a.Status == "" {
		a.Status = "pending"
	}
	return s.repo.CreateJobApplication(ctx, a)
}

// ListApplicationsByOffer returns applications for a specific job offer.
func (s *Service) ListApplicationsByOffer(ctx context.Context, offerID int64, limit, offset int) ([]*JobApplication, error) {
	return s.repo.ListApplicationsByOffer(ctx, offerID, limit, offset)
}

// ListApplicationsByUser returns applications submitted by a user.
func (s *Service) ListApplicationsByUser(ctx context.Context, userID int64) ([]*JobApplication, error) {
	return s.repo.ListApplicationsByUser(ctx, userID)
}

// ListApplications returns applications for a vacancy.
func (s *Service) ListApplications(ctx context.Context, offerID int64, limit, offset int) ([]*JobApplication, error) {
	return s.repo.ListApplicationsByOffer(ctx, offerID, limit, offset)
}

// GetApplicationByID retrieves an application by ID.
func (s *Service) GetApplicationByID(ctx context.Context, id int64) (*JobApplication, error) {
	return s.repo.GetApplicationByID(ctx, id)
}

// AcceptAndOnboardApplicant accepts a job applicant and links them as an employee in the specified branch.
func (s *Service) AcceptAndOnboardApplicant(ctx context.Context, in AcceptApplicantInput) (*JobApplication, error) {
	if in.ApplicationID <= 0 || in.OrganizationID <= 0 {
		return nil, apperr.Validation("hr.invalid_accept_input", "Application and Organization IDs are required.", nil)
	}
	app, err := s.repo.AcceptAndOnboardApplicant(ctx, in)
	if err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "job applicant accepted and onboarded",
		"application_id", in.ApplicationID, "org_id", in.OrganizationID,
		"branch_id", in.BranchID, "role_key", in.RoleKey)
	return app, nil
}

// RejectApplicant rejects a job application with optional review notes.
func (s *Service) RejectApplicant(ctx context.Context, orgID, appID int64, notes string) (*JobApplication, error) {
	if appID <= 0 || orgID <= 0 {
		return nil, apperr.Validation("hr.invalid_reject_input", "Application and Organization IDs are required.", nil)
	}
	app, err := s.repo.RejectApplicant(ctx, orgID, appID, notes)
	if err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "job applicant rejected", "application_id", appID, "org_id", orgID)
	return app, nil
}

// GetJobSeekerProfile retrieves the seeker profile for a user.
func (s *Service) GetJobSeekerProfile(ctx context.Context, userID int64) (*JobSeekerProfile, error) {
	return s.repo.GetJobSeekerProfile(ctx, userID)
}

// SaveJobSeekerProfile saves or updates a job seeker profile.
func (s *Service) SaveJobSeekerProfile(ctx context.Context, p *JobSeekerProfile) error {
	if p.UserID <= 0 {
		return apperr.Validation("seeker.user_required", "User ID is required.", nil)
	}
	return s.repo.UpsertJobSeekerProfile(ctx, p)
}
