package workflow

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockWorkflowRepo struct {
	requests map[int64]*PurchasePriorityRequest
	coverage map[int64][]*WeeklyCoverage
	issues   map[int64]*ReportIssue
	nextID   int64
}

func newMockWorkflowRepo() *mockWorkflowRepo {
	return &mockWorkflowRepo{
		requests: map[int64]*PurchasePriorityRequest{},
		coverage: map[int64][]*WeeklyCoverage{},
		issues:   map[int64]*ReportIssue{},
		nextID:   1,
	}
}

func (m *mockWorkflowRepo) CreatePriorityRequest(_ context.Context, r *PurchasePriorityRequest) error {
	r.ID = m.nextID
	m.nextID++
	m.requests[r.ID] = r
	return nil
}

func (m *mockWorkflowRepo) GetPriorityRequestByID(_ context.Context, id int64) (*PurchasePriorityRequest, error) {
	r, ok := m.requests[id]
	if !ok {
		return nil, apperr.NotFound("request")
	}
	return r, nil
}

func (m *mockWorkflowRepo) SaveWeeklyCoverage(_ context.Context, c *WeeklyCoverage) error {
	c.ID = m.nextID
	m.nextID++
	m.coverage[c.BranchID] = append(m.coverage[c.BranchID], c)
	return nil
}

func (m *mockWorkflowRepo) SaveBatchWeeklyCoverage(ctx context.Context, coverages []*WeeklyCoverage) error {
	for _, c := range coverages {
		if err := m.SaveWeeklyCoverage(ctx, c); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockWorkflowRepo) UpdateWeeklyCoverage(_ context.Context, c *WeeklyCoverage) error {
	for i, existing := range m.coverage[c.BranchID] {
		if existing.ID == c.ID {
			m.coverage[c.BranchID][i] = c
			return nil
		}
	}
	return apperr.NotFound("weekly_coverage")
}

func (m *mockWorkflowRepo) DeleteWeeklyCoverage(_ context.Context, id int64) error {
	for bID, list := range m.coverage {
		for i, c := range list {
			if c.ID == id {
				m.coverage[bID] = append(list[:i], list[i+1:]...)
				return nil
			}
		}
	}
	return apperr.NotFound("weekly_coverage")
}

func (m *mockWorkflowRepo) DeleteAllCoverageForOrganization(_ context.Context, orgID int64) error {
	for bID, list := range m.coverage {
		filtered := make([]*WeeklyCoverage, 0, len(list))
		for _, c := range list {
			if c.OrganizationID != orgID {
				filtered = append(filtered, c)
			}
		}
		m.coverage[bID] = filtered
	}
	return nil
}

func (m *mockWorkflowRepo) ToggleWeeklyCoverage(_ context.Context, id int64, isActive bool) error {
	for _, list := range m.coverage {
		for _, c := range list {
			if c.ID == id {
				c.IsActive = isActive
				return nil
			}
		}
	}
	return apperr.NotFound("weekly_coverage")
}

func (m *mockWorkflowRepo) GetWeeklyCoverageByID(_ context.Context, id int64) (*WeeklyCoverage, error) {
	for _, list := range m.coverage {
		for _, c := range list {
			if c.ID == id {
				return c, nil
			}
		}
	}
	return nil, apperr.NotFound("weekly_coverage")
}

func (m *mockWorkflowRepo) ListCoverageForOrganization(_ context.Context, orgID int64) ([]*CoverageView, error) {
	var list []*CoverageView
	for _, covList := range m.coverage {
		for _, c := range covList {
			if c.OrganizationID == orgID {
				list = append(list, &CoverageView{
					WeeklyCoverage: *c,
					BranchName:     "Main Branch",
					CityName:       "Cairo",
				})
			}
		}
	}
	return list, nil
}

func (m *mockWorkflowRepo) ListCoverageForOrganizationWithTotal(ctx context.Context, orgID int64, _, _ int) ([]*CoverageView, int, error) {
	list, err := m.ListCoverageForOrganization(ctx, orgID)
	return list, len(list), err
}

// BranchOrganization reports every branch as the test tenant's (org 20).
func (m *mockWorkflowRepo) BranchOrganization(_ context.Context, _ int64) (int64, error) {
	return 20, nil
}

func (m *mockWorkflowRepo) ListWeeklyCoverage(_ context.Context, branchID int64) ([]*WeeklyCoverage, error) {
	return m.coverage[branchID], nil
}

func (m *mockWorkflowRepo) CreateIssue(_ context.Context, i *ReportIssue) error {
	i.ID = m.nextID
	m.nextID++
	m.issues[i.ID] = i
	return nil
}

func (m *mockWorkflowRepo) GetIssueByID(_ context.Context, id int64) (*ReportIssue, error) {
	i, ok := m.issues[id]
	if !ok {
		return nil, apperr.NotFound("issue")
	}
	return i, nil
}

func (m *mockWorkflowRepo) ListIssues(_ context.Context, limit, offset int) ([]*ReportIssue, error) {
	var list []*ReportIssue
	for _, i := range m.issues {
		list = append(list, i)
	}
	return list, nil
}

func (m *mockWorkflowRepo) ListIssuesByReporter(_ context.Context, userID int64, limit, offset int) ([]*ReportIssue, error) {
	var list []*ReportIssue
	for _, i := range m.issues {
		if i.ReportedBy == userID {
			list = append(list, i)
		}
	}
	return list, nil
}

func (m *mockWorkflowRepo) ListIssuesDetailed(_ context.Context, filter ReportIssueFilter) ([]*ReportIssueDetail, int, error) {
	var list []*ReportIssueDetail
	for _, i := range m.issues {
		list = append(list, &ReportIssueDetail{ReportIssue: *i})
	}
	return list, len(list), nil
}

func (m *mockWorkflowRepo) GetIssueStats(_ context.Context) (*ReportIssueStats, error) {
	return &ReportIssueStats{Total: len(m.issues)}, nil
}

func (m *mockWorkflowRepo) UpdateIssueStatus(_ context.Context, id int64, status, responseNotes string) error {
	i, ok := m.issues[id]
	if !ok {
		return apperr.NotFound("issue")
	}
	i.Status = status
	i.ResponseNotes = responseNotes
	return nil
}

func (m *mockWorkflowRepo) CreateRequest(_ context.Context, r *Request) error {
	r.ID = 1
	return nil
}

func (m *mockWorkflowRepo) GetRequestByID(_ context.Context, _ int64) (*Request, error) {
	return nil, apperr.NotFound("request")
}

func (m *mockWorkflowRepo) ListRequestsByOrg(_ context.Context, _ int64, _ string, _, _ int) ([]*Request, error) {
	return nil, nil
}

func (m *mockWorkflowRepo) ListRequestsByOrgWithTotal(_ context.Context, _ int64, _ string, _, _ int) ([]*Request, int, error) {
	return nil, 0, nil
}

func (m *mockWorkflowRepo) UpdateRequestStatus(_ context.Context, _ int64, _ RequestStatus) error {
	return nil
}

func (m *mockWorkflowRepo) ListPriorityRequestsByUser(_ context.Context, userID int64, limit, offset int) ([]*PurchasePriorityRequest, error) {
	var list []*PurchasePriorityRequest
	for _, req := range m.requests {
		if req.UserID == userID {
			list = append(list, req)
		}
	}
	return list, nil
}

func (m *mockWorkflowRepo) UpdatePriorityRequestStatus(_ context.Context, id int64, status string, notes string, processedBy *int64, results map[string]any) error {
	req, ok := m.requests[id]
	if !ok {
		return apperr.NotFound("priority_request")
	}
	req.Status = status
	if recs, ok := results["recommendations"]; ok {
		if recsMap, ok := recs.(map[string]any); ok {
			req.Recommendations = recsMap
		}
	}
	return nil
}

func (m *mockWorkflowRepo) GetCandidateProducts(_ context.Context, userID int64, authorizedWorkIDs []int64, preferredSupplierIDs []int64, budget *money.Amount, limit int) ([]CandidateProduct, error) {
	return []CandidateProduct{
		{
			ProductID:            1,
			ProductName:          "Panadol Extra",
			ProductPrice:         money.MustParse("100.00"),
			ProductPriceDiscount: money.MustParse("80.00"),
			OrganizationID:       1,
			EstimatedDelivery:    1,
		},
	}, nil
}

func TestWorkflowPurchasePriorityAndCoverage(t *testing.T) {
	ctx := database.WithTenant(context.Background(), 20)
	repo := newMockWorkflowRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// 1. Create Priority Request
	req, err := svc.CreatePriorityRequest(ctx, 100, &PurchasePriorityRequest{
		PriorityHighestDiscount: true,
	})
	if err != nil {
		t.Fatalf("CreatePriorityRequest failed: %v", err)
	}
	if req.RequestNumber == "" || req.Status != "pending" {
		t.Errorf("unexpected request state: %+v", req)
	}

	// 2. Set Weekly Coverage
	err = svc.SetWeeklyCoverage(ctx, &WeeklyCoverage{
		BranchID:       5,
		DayOfWeek:      1, // Monday
		CoverageFrom:   workflow_ptr("09:00"),
		CoverageTo:     workflow_ptr("17:00"),
		Address:        "Nasr City Zone 1",
		DistanceMeters: 3000,
	})
	if err != nil {
		t.Fatalf("SetWeeklyCoverage failed: %v", err)
	}

	coverageList, err := svc.GetBranchCoverage(ctx, 5)
	if err != nil || len(coverageList) != 1 {
		t.Fatalf("GetBranchCoverage failed: %v", err)
	}

	// 3. Issues & Support
	issue, err := svc.ReportIssue(ctx, &ReportIssue{
		ReportedBy:  100,
		IssueType:   "damaged_item",
		Description: "One box arrived damaged",
		Priority:    "high",
	})
	if err != nil {
		t.Fatalf("ReportIssue failed: %v", err)
	}
	if issue.Status != "pending" {
		t.Errorf("got status %s, want pending", issue.Status)
	}

	issues, err := svc.ListIssues(ctx, 10, 0)
	if err != nil || len(issues) != 1 {
		t.Fatalf("ListIssues failed: %v", err)
	}
}

// workflow_ptr returns a pointer to s. Coverage window bounds are *string so a
// blank form field can mean NULL rather than an empty TIME.
func workflow_ptr(s string) *string { return &s }
