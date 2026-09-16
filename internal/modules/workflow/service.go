package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Service coordinates purchasing optimization, weekly coverage, and issue resolution.
type Service struct {
	repo     Repository
	log      *slog.Logger
	instGate InstitutionalGate
}

// NewService creates a new workflow service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// CreatePriorityRequest starts a purchasing optimization computation.
func (s *Service) CreatePriorityRequest(ctx context.Context, userID int64, req *PurchasePriorityRequest) (*PurchasePriorityRequest, error) {
	req.UserID = userID
	orgID, ok := database.TenantFrom(ctx)
	if ok {
		req.OrganizationID = &orgID
	}
	req.Status = "pending"
	req.RequestNumber = fmt.Sprintf("REQ-%s-%04d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)

	if err := s.repo.CreatePriorityRequest(ctx, req); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "purchase priority request submitted", "id", req.ID, "number", req.RequestNumber)
	return req, nil
}

// SetWeeklyCoverage configures geographic delivery windows for a branch.
func (s *Service) SetWeeklyCoverage(ctx context.Context, c *WeeklyCoverage) error {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return database.ErrNoTenant
	}
	if err := s.ownBranch(ctx, orgID, c.BranchID); err != nil {
		return err
	}
	c.OrganizationID = orgID
	c.IsActive = true
	if err := c.Validate(); err != nil {
		return err
	}
	return s.repo.SaveWeeklyCoverage(ctx, c)
}

// CreateWeeklyCoverage creates a new branch weekly coverage entry.
func (s *Service) CreateWeeklyCoverage(ctx context.Context, c *WeeklyCoverage) error {
	if err := c.Validate(); err != nil {
		return err
	}
	return s.repo.SaveWeeklyCoverage(ctx, c)
}

// CreateBatchWeeklyCoverage creates multiple branch weekly coverage entries.
func (s *Service) CreateBatchWeeklyCoverage(ctx context.Context, coverages []*WeeklyCoverage) error {
	for _, c := range coverages {
		if err := c.Validate(); err != nil {
			return err
		}
	}
	return s.repo.SaveBatchWeeklyCoverage(ctx, coverages)
}

// UpdateWeeklyCoverage updates an existing branch weekly coverage entry.
func (s *Service) UpdateWeeklyCoverage(ctx context.Context, c *WeeklyCoverage) error {
	if err := c.Validate(); err != nil {
		return err
	}
	return s.repo.UpdateWeeklyCoverage(ctx, c)
}

// DeleteWeeklyCoverage deletes a weekly coverage entry.
func (s *Service) DeleteWeeklyCoverage(ctx context.Context, id int64) error {
	if id <= 0 {
		return apperr.Validation("coverage.id_invalid", "Invalid coverage ID.", nil)
	}
	return s.repo.DeleteWeeklyCoverage(ctx, id)
}

// DeleteAllCoverageForOrganization deletes all weekly coverage entries for an organization.
func (s *Service) DeleteAllCoverageForOrganization(ctx context.Context, orgID int64) error {
	if orgID <= 0 {
		return apperr.Validation("coverage.org_id_invalid", "Invalid organization ID.", nil)
	}
	return s.repo.DeleteAllCoverageForOrganization(ctx, orgID)
}

// ToggleWeeklyCoverage toggles the active state of a coverage entry.
func (s *Service) ToggleWeeklyCoverage(ctx context.Context, id int64, isActive bool) error {
	if id <= 0 {
		return apperr.Validation("coverage.id_invalid", "Invalid coverage ID.", nil)
	}
	return s.repo.ToggleWeeklyCoverage(ctx, id, isActive)
}

// GetWeeklyCoverage retrieves a weekly coverage record by ID.
func (s *Service) GetWeeklyCoverage(ctx context.Context, id int64) (*WeeklyCoverage, error) {
	if id <= 0 {
		return nil, apperr.Validation("coverage.id_invalid", "Invalid coverage ID.", nil)
	}
	return s.repo.GetWeeklyCoverageByID(ctx, id)
}

// GetBranchCoverage lists coverage windows for a branch.
func (s *Service) GetBranchCoverage(ctx context.Context, branchID int64) ([]*WeeklyCoverage, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	if err := s.ownBranch(ctx, orgID, branchID); err != nil {
		return nil, err
	}
	return s.repo.ListWeeklyCoverage(ctx, branchID)
}

// ownBranch refuses a branch of another organisation. Coverage written against
// a competitor's branch id made that branch's listings count as delivered
// wherever the writer delivered.
func (s *Service) ownBranch(ctx context.Context, orgID, branchID int64) error {
	owner, err := s.repo.BranchOrganization(ctx, branchID)
	if err != nil {
		return err
	}
	if owner == 0 || owner != orgID {
		return apperr.NotFound("branch")
	}
	return nil
}

// ListCoverageForOrganization lists all weekly coverage records for an organization with joined branch names.
func (s *Service) ListCoverageForOrganization(ctx context.Context, orgID int64) ([]*CoverageView, error) {
	return s.repo.ListCoverageForOrganization(ctx, orgID)
}

// ListCoverageForOrganizationWithTotal lists paginated weekly coverage records for an organization with total count.
func (s *Service) ListCoverageForOrganizationWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*CoverageView, int, error) {
	return s.repo.ListCoverageForOrganizationWithTotal(ctx, orgID, limit, offset)
}

// ReportIssue logs a support inquiry or product defect.
func (s *Service) ReportIssue(ctx context.Context, i *ReportIssue) (*ReportIssue, error) {
	if err := i.Validate(); err != nil {
		return nil, err
	}
	i.Status = "pending"
	if i.Priority == "" {
		i.Priority = "medium"
	}

	if err := s.repo.CreateIssue(ctx, i); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "issue reported", "issue_id", i.ID, "type", i.IssueType)
	return i, nil
}

// GetIssueByID retrieves a specific issue report ticket.
func (s *Service) GetIssueByID(ctx context.Context, id int64) (*ReportIssue, error) {
	if id <= 0 {
		return nil, apperr.Validation("issue.id_required", "Issue ID is required.", nil)
	}
	return s.repo.GetIssueByID(ctx, id)
}

// ListIssues retrieves issues with pagination.
func (s *Service) ListIssues(ctx context.Context, limit, offset int) ([]*ReportIssue, error) {
	return s.repo.ListIssues(ctx, limit, offset)
}

// ListIssuesByReporter retrieves tickets submitted by a specific user.
func (s *Service) ListIssuesByReporter(ctx context.Context, userID int64, limit, offset int) ([]*ReportIssue, error) {
	if userID <= 0 {
		return nil, nil
	}
	return s.repo.ListIssuesByReporter(ctx, userID, limit, offset)
}

// ListIssuesAdmin retrieves filtered tickets with rich user/org metadata and counts.
func (s *Service) ListIssuesAdmin(ctx context.Context, filter ReportIssueFilter) ([]*ReportIssueDetail, int, *ReportIssueStats, error) {
	items, total, err := s.repo.ListIssuesDetailed(ctx, filter)
	if err != nil {
		return nil, 0, nil, err
	}
	stats, err := s.repo.GetIssueStats(ctx)
	if err != nil {
		return nil, 0, nil, err
	}
	return items, total, stats, nil
}

// UpdateIssueStatus updates the ticket status and response notes.
func (s *Service) UpdateIssueStatus(ctx context.Context, id int64, status, responseNotes string) error {
	if id <= 0 {
		return apperr.Validation("issue.id_required", "Issue ID is required.", nil)
	}
	if status != "pending" && status != "in_progress" && status != "resolved" {
		return apperr.Validation("issue.invalid_status", "Invalid status.", nil)
	}
	if err := s.repo.UpdateIssueStatus(ctx, id, status, responseNotes); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "issue status updated", "issue_id", id, "status", status)
	return nil
}

// CreateRequest sends a document/action request to another organization.
func (s *Service) CreateRequest(ctx context.Context, fromUserID, fromOrgID, toOrgID int64, typ RequestType, title i18n.Text, description string) (*Request, error) {
	r := &Request{
		FromUserID:  fromUserID,
		FromOrgID:   fromOrgID,
		ToOrgID:     toOrgID,
		Type:        typ,
		Title:       title,
		Description: description,
		Status:      RequestPending,
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.CreateRequest(ctx, r); err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "request created", "request_id", r.ID, "from", fromOrgID, "to", toOrgID)
	return r, nil
}

// ListInbox returns requests sent to or from the caller's organization.
func (s *Service) ListInbox(ctx context.Context, orgID int64, status string, limit, offset int) ([]*Request, error) {
	return s.repo.ListRequestsByOrg(ctx, orgID, status, limit, offset)
}

// ListInboxWithTotal returns requests sent to or from the caller's organization with total count.
func (s *Service) ListInboxWithTotal(ctx context.Context, orgID int64, status string, limit, offset int) ([]*Request, int, error) {
	return s.repo.ListRequestsByOrgWithTotal(ctx, orgID, status, limit, offset)
}

// RespondRequest accepts or declines a request.
func (s *Service) RespondRequest(ctx context.Context, id int64, status RequestStatus) error {
	if status != RequestAccepted && status != RequestDeclined && status != RequestCancelled {
		return apperr.Validation("request.status_invalid", "Invalid request status.", nil)
	}
	return s.repo.UpdateRequestStatus(ctx, id, status)
}
