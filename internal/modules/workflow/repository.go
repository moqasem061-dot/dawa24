package workflow

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Repository defines storage operations for workflow tasks and schedules.
type Repository interface {
	CreatePriorityRequest(ctx context.Context, r *PurchasePriorityRequest) error
	GetPriorityRequestByID(ctx context.Context, id int64) (*PurchasePriorityRequest, error)
	ListPriorityRequestsByUser(ctx context.Context, userID int64, limit, offset int) ([]*PurchasePriorityRequest, error)
	UpdatePriorityRequestStatus(ctx context.Context, id int64, status string, notes string, processedBy *int64, results map[string]any) error
	GetCandidateProducts(ctx context.Context, userID int64, authorizedWorkIDs []int64, preferredSupplierIDs []int64, budget *money.Amount, limit int) ([]CandidateProduct, error)

	SaveWeeklyCoverage(ctx context.Context, c *WeeklyCoverage) error
	SaveBatchWeeklyCoverage(ctx context.Context, coverages []*WeeklyCoverage) error
	UpdateWeeklyCoverage(ctx context.Context, c *WeeklyCoverage) error
	DeleteWeeklyCoverage(ctx context.Context, id int64) error
	DeleteAllCoverageForOrganization(ctx context.Context, orgID int64) error
	ToggleWeeklyCoverage(ctx context.Context, id int64, isActive bool) error
	GetWeeklyCoverageByID(ctx context.Context, id int64) (*WeeklyCoverage, error)
	ListWeeklyCoverage(ctx context.Context, branchID int64) ([]*WeeklyCoverage, error)
	// BranchOrganization is the organisation a live branch belongs to, or 0.
	BranchOrganization(ctx context.Context, branchID int64) (int64, error)
	ListCoverageForOrganization(ctx context.Context, orgID int64) ([]*CoverageView, error)
	ListCoverageForOrganizationWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*CoverageView, int, error)

	CreateIssue(ctx context.Context, i *ReportIssue) error
	GetIssueByID(ctx context.Context, id int64) (*ReportIssue, error)
	ListIssues(ctx context.Context, limit, offset int) ([]*ReportIssue, error)
	ListIssuesByReporter(ctx context.Context, userID int64, limit, offset int) ([]*ReportIssue, error)
	ListIssuesDetailed(ctx context.Context, filter ReportIssueFilter) ([]*ReportIssueDetail, int, error)
	GetIssueStats(ctx context.Context) (*ReportIssueStats, error)
	UpdateIssueStatus(ctx context.Context, id int64, status, responseNotes string) error

	CreateRequest(ctx context.Context, r *Request) error
	GetRequestByID(ctx context.Context, id int64) (*Request, error)
	ListRequestsByOrg(ctx context.Context, orgID int64, status string, limit, offset int) ([]*Request, error)
	ListRequestsByOrgWithTotal(ctx context.Context, orgID int64, status string, limit, offset int) ([]*Request, int, error)
	UpdateRequestStatus(ctx context.Context, id int64, status RequestStatus) error
}
