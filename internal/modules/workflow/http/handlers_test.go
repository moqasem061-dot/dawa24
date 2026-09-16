package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	workflowHttp "github.com/muhiya/dawa24-store/internal/modules/workflow/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) CreatePriorityRequest(context.Context, *workflow.PurchasePriorityRequest) error {
	r.fail("CreatePriorityRequest")
	return nil
}
func (r stubRepo) GetPriorityRequestByID(context.Context, int64) (*workflow.PurchasePriorityRequest, error) {
	r.fail("GetPriorityRequestByID")
	return nil, nil
}
func (r stubRepo) SaveWeeklyCoverage(context.Context, *workflow.WeeklyCoverage) error {
	r.fail("SaveWeeklyCoverage")
	return nil
}
func (r stubRepo) SaveBatchWeeklyCoverage(context.Context, []*workflow.WeeklyCoverage) error {
	r.fail("SaveBatchWeeklyCoverage")
	return nil
}
func (r stubRepo) UpdateWeeklyCoverage(context.Context, *workflow.WeeklyCoverage) error {
	r.fail("UpdateWeeklyCoverage")
	return nil
}
func (r stubRepo) DeleteWeeklyCoverage(context.Context, int64) error {
	r.fail("DeleteWeeklyCoverage")
	return nil
}
func (r stubRepo) DeleteAllCoverageForOrganization(context.Context, int64) error {
	r.fail("DeleteAllCoverageForOrganization")
	return nil
}
func (r stubRepo) ToggleWeeklyCoverage(context.Context, int64, bool) error {
	r.fail("ToggleWeeklyCoverage")
	return nil
}
func (r stubRepo) GetWeeklyCoverageByID(context.Context, int64) (*workflow.WeeklyCoverage, error) {
	r.fail("GetWeeklyCoverageByID")
	return nil, nil
}
func (r stubRepo) BranchOrganization(context.Context, int64) (int64, error) {
	r.fail("BranchOrganization")
	return 0, nil
}

func (r stubRepo) ListWeeklyCoverage(context.Context, int64) ([]*workflow.WeeklyCoverage, error) {
	r.fail("ListWeeklyCoverage")
	return nil, nil
}
func (r stubRepo) ListCoverageForOrganization(context.Context, int64) ([]*workflow.CoverageView, error) {
	r.fail("ListCoverageForOrganization")
	return nil, nil
}
func (r stubRepo) ListCoverageForOrganizationWithTotal(context.Context, int64, int, int) ([]*workflow.CoverageView, int, error) {
	r.fail("ListCoverageForOrganizationWithTotal")
	return nil, 0, nil
}
func (r stubRepo) CreateIssue(context.Context, *workflow.ReportIssue) error {
	r.fail("CreateIssue")
	return nil
}
func (r stubRepo) GetIssueByID(context.Context, int64) (*workflow.ReportIssue, error) {
	r.fail("GetIssueByID")
	return nil, nil
}
func (r stubRepo) ListIssues(context.Context, int, int) ([]*workflow.ReportIssue, error) {
	r.fail("ListIssues")
	return nil, nil
}
func (r stubRepo) ListIssuesByReporter(context.Context, int64, int, int) ([]*workflow.ReportIssue, error) {
	r.fail("ListIssuesByReporter")
	return nil, nil
}
func (r stubRepo) ListIssuesDetailed(context.Context, workflow.ReportIssueFilter) ([]*workflow.ReportIssueDetail, int, error) {
	r.fail("ListIssuesDetailed")
	return nil, 0, nil
}
func (r stubRepo) GetIssueStats(context.Context) (*workflow.ReportIssueStats, error) {
	r.fail("GetIssueStats")
	return nil, nil
}
func (r stubRepo) UpdateIssueStatus(context.Context, int64, string, string) error {
	r.fail("UpdateIssueStatus")
	return nil
}

func (r stubRepo) CreateRequest(context.Context, *workflow.Request) error {
	r.fail("CreateRequest")
	return nil
}

func (r stubRepo) GetRequestByID(context.Context, int64) (*workflow.Request, error) {
	r.fail("GetRequestByID")
	return nil, nil
}

func (r stubRepo) ListRequestsByOrg(context.Context, int64, string, int, int) ([]*workflow.Request, error) {
	r.fail("ListRequestsByOrg")
	return nil, nil
}
func (r stubRepo) ListRequestsByOrgWithTotal(context.Context, int64, string, int, int) ([]*workflow.Request, int, error) {
	r.fail("ListRequestsByOrgWithTotal")
	return nil, 0, nil
}

func (r stubRepo) UpdateRequestStatus(context.Context, int64, workflow.RequestStatus) error {
	r.fail("UpdateRequestStatus")
	return nil
}
func (r stubRepo) ListPriorityRequestsByUser(ctx context.Context, userID int64, limit, offset int) ([]*workflow.PurchasePriorityRequest, error) {
	r.fail("ListPriorityRequestsByUser")
	return nil, nil
}
func (r stubRepo) UpdatePriorityRequestStatus(ctx context.Context, id int64, status string, notes string, processedBy *int64, results map[string]any) error {
	r.fail("UpdatePriorityRequestStatus")
	return nil
}
func (r stubRepo) GetCandidateProducts(ctx context.Context, userID int64, authorizedWorkIDs []int64, preferredSupplierIDs []int64, budget *money.Amount, limit int) ([]workflow.CandidateProduct, error) {
	r.fail("GetCandidateProducts")
	return nil, nil
}

type happyRepo struct{}

func (happyRepo) CreatePriorityRequest(ctx context.Context, r *workflow.PurchasePriorityRequest) error {
	r.ID = 1
	r.Status = "pending"
	return nil
}
func (happyRepo) GetPriorityRequestByID(ctx context.Context, id int64) (*workflow.PurchasePriorityRequest, error) {
	return &workflow.PurchasePriorityRequest{ID: id, UserID: 1, Status: "pending"}, nil
}
func (happyRepo) SaveWeeklyCoverage(ctx context.Context, c *workflow.WeeklyCoverage) error {
	c.ID = 1
	return nil
}
func (happyRepo) SaveBatchWeeklyCoverage(ctx context.Context, coverages []*workflow.WeeklyCoverage) error {
	return nil
}
func (happyRepo) UpdateWeeklyCoverage(ctx context.Context, c *workflow.WeeklyCoverage) error {
	return nil
}
func (happyRepo) DeleteWeeklyCoverage(ctx context.Context, id int64) error {
	return nil
}
func (happyRepo) DeleteAllCoverageForOrganization(ctx context.Context, orgID int64) error {
	return nil
}
func (happyRepo) ToggleWeeklyCoverage(ctx context.Context, id int64, isActive bool) error {
	return nil
}
func (happyRepo) GetWeeklyCoverageByID(ctx context.Context, id int64) (*workflow.WeeklyCoverage, error) {
	return &workflow.WeeklyCoverage{ID: id, BranchID: 1, DayOfWeek: 1, DistanceMeters: 5000, IsActive: true}, nil
}
func (happyRepo) BranchOrganization(context.Context, int64) (int64, error) { return 1, nil }

func (happyRepo) ListWeeklyCoverage(ctx context.Context, branchID int64) ([]*workflow.WeeklyCoverage, error) {
	return []*workflow.WeeklyCoverage{{ID: 1, BranchID: branchID, DayOfWeek: 1, DistanceMeters: 5000, IsActive: true}}, nil
}
func (happyRepo) ListCoverageForOrganization(ctx context.Context, orgID int64) ([]*workflow.CoverageView, error) {
	return []*workflow.CoverageView{{
		WeeklyCoverage: workflow.WeeklyCoverage{ID: 1, OrganizationID: orgID, BranchID: 1, DayOfWeek: 1, DistanceMeters: 5000, IsActive: true},
		BranchName:     "Branch 1",
		CityName:       "Cairo",
	}}, nil
}
func (happyRepo) ListCoverageForOrganizationWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*workflow.CoverageView, int, error) {
	views := []*workflow.CoverageView{{
		WeeklyCoverage: workflow.WeeklyCoverage{ID: 1, OrganizationID: orgID, BranchID: 1, DayOfWeek: 1, DistanceMeters: 5000, IsActive: true},
		BranchName:     "Branch 1",
		CityName:       "Cairo",
	}}
	return views, len(views), nil
}
func (happyRepo) CreateIssue(ctx context.Context, i *workflow.ReportIssue) error {
	i.ID = 1
	return nil
}
func (happyRepo) GetIssueByID(ctx context.Context, id int64) (*workflow.ReportIssue, error) {
	return &workflow.ReportIssue{ID: id, ReportedBy: 1, IssueType: "quality", Description: "Broken seal"}, nil
}
func (happyRepo) ListIssues(ctx context.Context, limit, offset int) ([]*workflow.ReportIssue, error) {
	return []*workflow.ReportIssue{{ID: 1, ReportedBy: 1, IssueType: "quality", Description: "Broken seal"}}, nil
}
func (happyRepo) ListIssuesByReporter(ctx context.Context, userID int64, limit, offset int) ([]*workflow.ReportIssue, error) {
	return []*workflow.ReportIssue{{ID: 1, ReportedBy: userID, IssueType: "quality", Description: "Broken seal"}}, nil
}
func (happyRepo) ListIssuesDetailed(ctx context.Context, filter workflow.ReportIssueFilter) ([]*workflow.ReportIssueDetail, int, error) {
	return []*workflow.ReportIssueDetail{{ReportIssue: workflow.ReportIssue{ID: 1, ReportedBy: 1, IssueType: "quality", Description: "Broken seal"}}}, 1, nil
}
func (happyRepo) GetIssueStats(ctx context.Context) (*workflow.ReportIssueStats, error) {
	return &workflow.ReportIssueStats{Total: 1, Pending: 1}, nil
}
func (happyRepo) UpdateIssueStatus(ctx context.Context, id int64, status, responseNotes string) error {
	return nil
}

func (happyRepo) CreateRequest(ctx context.Context, r *workflow.Request) error {
	r.ID = 1
	return nil
}

func (happyRepo) GetRequestByID(ctx context.Context, id int64) (*workflow.Request, error) {
	return &workflow.Request{ID: id, Type: workflow.RequestDocument, Status: workflow.RequestPending}, nil
}

func (happyRepo) ListRequestsByOrg(ctx context.Context, orgID int64, status string, limit, offset int) ([]*workflow.Request, error) {
	return []*workflow.Request{{ID: 1, Type: workflow.RequestDocument, Status: workflow.RequestPending}}, nil
}
func (happyRepo) ListRequestsByOrgWithTotal(ctx context.Context, orgID int64, status string, limit, offset int) ([]*workflow.Request, int, error) {
	return []*workflow.Request{{ID: 1, Type: workflow.RequestDocument, Status: workflow.RequestPending}}, 1, nil
}

func (happyRepo) UpdateRequestStatus(ctx context.Context, id int64, status workflow.RequestStatus) error {
	return nil
}
func (happyRepo) ListPriorityRequestsByUser(ctx context.Context, userID int64, limit, offset int) ([]*workflow.PurchasePriorityRequest, error) {
	return []*workflow.PurchasePriorityRequest{{ID: 1, UserID: userID, Status: "completed"}}, nil
}
func (happyRepo) UpdatePriorityRequestStatus(ctx context.Context, id int64, status string, notes string, processedBy *int64, results map[string]any) error {
	return nil
}
func (happyRepo) GetCandidateProducts(ctx context.Context, userID int64, authorizedWorkIDs []int64, preferredSupplierIDs []int64, budget *money.Amount, limit int) ([]workflow.CandidateProduct, error) {
	return []workflow.CandidateProduct{}, nil
}

const testCookieName = "dawa24_session"

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	wfSvc := workflow.NewService(stubRepo{t: t}, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(testCookieName)
			if err != nil || cookie.Value == "" || cookie.Value == "forged-token-that-was-never-issued" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	workflowHttp.NewHandler(wfSvc, log).RegisterRoutes(r)

	return r
}

func newAuthedRouter(repo workflow.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	wfSvc := workflow.NewService(repo, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := authctx.Actor{
				UserID:         1,
				OrganizationID: 1,
				Role:           "admin",
				Permissions:    []string{"admin", "workflow.admin"},
			}
			ctx := authctx.WithActor(r.Context(), actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	workflowHttp.NewHandler(wfSvc, log).RegisterRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodPost, "/api/v1/workflow/priority-requests"},
	{http.MethodPost, "/api/v1/workflow/branches/1/coverage"},
	{http.MethodGet, "/api/v1/workflow/branches/1/coverage"},
	{http.MethodPost, "/api/v1/workflow/issues"},
	{http.MethodGet, "/api/v1/workflow/issues"},
}

func TestProtectedRoutesRejectAnonymousCallers(t *testing.T) {
	router := newTestRouter(t)

	for _, route := range protectedRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("got %d, want 401 — this endpoint is reachable without a session", rec.Code)
			}
		})
	}
}

func TestProtectedRoutesRejectGarbageSessionToken(t *testing.T) {
	router := newTestRouter(t)

	for _, route := range protectedRoutes {
		req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "dawa24_session", Value: "forged-token-that-was-never-issued"})
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a forged token got %d, want 401", route.method, route.path, rec.Code)
		}
	}
}

func TestUnauthorizedResponseUsesTheErrorEnvelope(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflow/issues", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the JSON error envelope: %v (body: %s)", err, rec.Body.String())
	}
	if body.Error.Code == "" {
		t.Error("error envelope has no code")
	}
	if body.Error.RequestID == "" {
		t.Error("error envelope has no request_id")
	}
}

func TestWorkflowHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"CreatePriorityRequest", http.MethodPost, "/api/v1/workflow/priority-requests", `{"user_id":1,"priority_lowest_price":true,"budget_constraint":"1000.00"}`, http.StatusCreated},
		{"SetWeeklyCoverage", http.MethodPost, "/api/v1/workflow/branches/1/coverage", `{"branch_id":1,"day_of_week":1,"distance_meters":5000,"is_active":true}`, http.StatusOK},
		{"GetBranchCoverage", http.MethodGet, "/api/v1/workflow/branches/1/coverage", "", http.StatusOK},
		{"ReportIssue", http.MethodPost, "/api/v1/workflow/issues", `{"reported_by":1,"issue_type":"quality","description":"Broken seal","priority":"high"}`, http.StatusCreated},
		{"ListIssues", http.MethodGet, "/api/v1/workflow/issues?limit=10&offset=0", "", http.StatusOK},
		{"AdminIssues", http.MethodGet, "/api/v1/admin/workflow/issues", "", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tt.body != "" {
				bodyReader = strings.NewReader(tt.body)
			}
			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s got status %d, want %d (body: %s)", tt.method, tt.path, rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
