package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type mockCoverageRepo struct {
	workflow.Repository
	coverages map[int64]*workflow.WeeklyCoverage
	nextID    int64
}

func newMockCoverageRepo() *mockCoverageRepo {
	return &mockCoverageRepo{
		coverages: map[int64]*workflow.WeeklyCoverage{
			1: {
				ID:             1,
				OrganizationID: 10,
				BranchID:       100,
				DayOfWeek:      1,
				DistanceMeters: 25000,
				IsActive:       true,
			},
		},
		nextID: 2,
	}
}

func (m *mockCoverageRepo) CreatePriorityRequest(_ context.Context, _ *workflow.PurchasePriorityRequest) error {
	return nil
}
func (m *mockCoverageRepo) GetPriorityRequestByID(_ context.Context, _ int64) (*workflow.PurchasePriorityRequest, error) {
	return nil, nil
}
func (m *mockCoverageRepo) SaveWeeklyCoverage(_ context.Context, c *workflow.WeeklyCoverage) error {
	c.ID = m.nextID
	m.nextID++
	m.coverages[c.ID] = c
	return nil
}
func (m *mockCoverageRepo) SaveBatchWeeklyCoverage(ctx context.Context, coverages []*workflow.WeeklyCoverage) error {
	for _, c := range coverages {
		if err := m.SaveWeeklyCoverage(ctx, c); err != nil {
			return err
		}
	}
	return nil
}
func (m *mockCoverageRepo) UpdateWeeklyCoverage(_ context.Context, c *workflow.WeeklyCoverage) error {
	if _, ok := m.coverages[c.ID]; !ok {
		return apperr.NotFound("weekly_coverage")
	}
	m.coverages[c.ID] = c
	return nil
}
func (m *mockCoverageRepo) DeleteWeeklyCoverage(_ context.Context, id int64) error {
	if _, ok := m.coverages[id]; !ok {
		return apperr.NotFound("weekly_coverage")
	}
	delete(m.coverages, id)
	return nil
}
func (m *mockCoverageRepo) ToggleWeeklyCoverage(_ context.Context, id int64, isActive bool) error {
	c, ok := m.coverages[id]
	if !ok {
		return apperr.NotFound("weekly_coverage")
	}
	c.IsActive = isActive
	return nil
}
func (m *mockCoverageRepo) GetWeeklyCoverageByID(_ context.Context, id int64) (*workflow.WeeklyCoverage, error) {
	c, ok := m.coverages[id]
	if !ok {
		return nil, apperr.NotFound("weekly_coverage")
	}
	return c, nil
}
func (m *mockCoverageRepo) BranchOrganization(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}

func (m *mockCoverageRepo) ListWeeklyCoverage(_ context.Context, branchID int64) ([]*workflow.WeeklyCoverage, error) {
	var list []*workflow.WeeklyCoverage
	for _, c := range m.coverages {
		if c.BranchID == branchID {
			list = append(list, c)
		}
	}
	return list, nil
}
func (m *mockCoverageRepo) ListCoverageForOrganization(_ context.Context, orgID int64) ([]*workflow.CoverageView, error) {
	var list []*workflow.CoverageView
	for _, c := range m.coverages {
		if c.OrganizationID == orgID {
			list = append(list, &workflow.CoverageView{
				WeeklyCoverage: *c,
				BranchName:     "Main Warehouse",
				CityName:       "Cairo",
			})
		}
	}
	return list, nil
}
func (m *mockCoverageRepo) ListCoverageForOrganizationWithTotal(ctx context.Context, orgID int64, _, _ int) ([]*workflow.CoverageView, int, error) {
	list, err := m.ListCoverageForOrganization(ctx, orgID)
	return list, len(list), err
}
func (m *mockCoverageRepo) CreateIssue(_ context.Context, _ *workflow.ReportIssue) error {
	return nil
}
func (m *mockCoverageRepo) GetIssueByID(_ context.Context, _ int64) (*workflow.ReportIssue, error) {
	return nil, nil
}
func (m *mockCoverageRepo) ListIssues(_ context.Context, _, _ int) ([]*workflow.ReportIssue, error) {
	return nil, nil
}
func (m *mockCoverageRepo) CreateRequest(_ context.Context, _ *workflow.Request) error { return nil }
func (m *mockCoverageRepo) GetRequestByID(_ context.Context, _ int64) (*workflow.Request, error) {
	return nil, nil
}
func (m *mockCoverageRepo) ListRequestsByOrg(_ context.Context, _ int64, _ string, _, _ int) ([]*workflow.Request, error) {
	return nil, nil
}
func (m *mockCoverageRepo) ListRequestsByOrgWithTotal(_ context.Context, _ int64, _ string, _, _ int) ([]*workflow.Request, int, error) {
	return nil, 0, nil
}
func (m *mockCoverageRepo) UpdateRequestStatus(_ context.Context, _ int64, _ workflow.RequestStatus) error {
	return nil
}
func (m *mockCoverageRepo) ListPriorityRequestsByUser(_ context.Context, _ int64, _, _ int) ([]*workflow.PurchasePriorityRequest, error) {
	return nil, nil
}
func (m *mockCoverageRepo) UpdatePriorityRequestStatus(_ context.Context, _ int64, _ string, _ string, _ *int64, _ map[string]any) error {
	return nil
}
func (m *mockCoverageRepo) GetCandidateProducts(_ context.Context, _ int64, _ []int64, _ []int64, _ *money.Amount, _ int) ([]workflow.CandidateProduct, error) {
	return nil, nil
}

type mockOrgCoverageRepo struct {
	org.Repository
}

func (mockOrgCoverageRepo) GetBranchByID(_ context.Context, id int64) (*org.Branch, error) {
	if id == 100 {
		return &org.Branch{ID: 100, OrganizationID: 10, Name: i18n.New("Main Warehouse", "Main Warehouse")}, nil
	}
	if id == 200 {
		return &org.Branch{ID: 200, OrganizationID: 99, Name: i18n.New("Other Org Warehouse", "Other Org Warehouse")}, nil
	}
	return nil, apperr.NotFound("branch")
}
func (mockOrgCoverageRepo) ListBranchesByOrg(_ context.Context, orgID int64) ([]*org.Branch, error) {
	if orgID == 10 {
		return []*org.Branch{{ID: 100, OrganizationID: 10, Name: i18n.New("Main Warehouse", "Main Warehouse")}}, nil
	}
	return nil, nil
}
func (mockOrgCoverageRepo) GetDeliveryBands(_ context.Context, orgID int64) ([]*org.DeliveryBand, error) {
	return []*org.DeliveryBand{
		{ID: 1, OrganizationID: orgID, FromMeters: 0, ToMeters: 10000, Fee: money.FromMinor(3000), IsActive: true},
	}, nil
}
func (mockOrgCoverageRepo) SaveDeliveryBands(_ context.Context, orgID int64, bands []*org.DeliveryBand) error {
	return nil
}

func newTestCoverageRouter(actor *authctx.Actor, wfRepo workflow.Repository, orgRepo org.Repository) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	wfSvc := workflow.NewService(wfRepo, logger)
	orgSvc := org.NewService(orgRepo, logger)

	handler := ui.NewUIHandler(
		nil, orgSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, wfSvc, nil, nil, logger,
	)

	stubAuth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if actor != nil {
				ctx := authctx.WithActor(req.Context(), *actor)
				if actor.OrganizationID > 0 {
					ctx = database.WithTenant(ctx, actor.OrganizationID)
				}
				next.ServeHTTP(w, req.WithContext(ctx))
				return
			}
			next.ServeHTTP(w, req)
		})
	}

	r := chi.NewRouter()
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(stubAuth)
		uiRouter.Use(authctx.RequireCustomer(logger))
		uiRouter.Use(authctx.RequireApproved(logger))
		handler.RegisterCustomerRoutes(uiRouter)
	})
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(stubAuth)
		uiRouter.Use(authctx.RequireVendor(logger))
		uiRouter.Use(authctx.RequireApproved(logger))
		handler.RegisterVendorRoutes(uiRouter)
	})
	return r
}
