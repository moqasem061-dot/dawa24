package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type mockOrgRepoJobs struct {
	org.Repository
	branches []*org.Branch
}

func (m *mockOrgRepoJobs) ListBranchesByOrg(ctx context.Context, orgID int64) ([]*org.Branch, error) {
	return m.branches, nil
}

type mockHRRepoJobs struct {
	hr.Repository
	jobs        []*hr.JobOffer
	createdJobs []*hr.JobOffer
}

func (m *mockHRRepoJobs) ListJobsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*hr.JobOffer, error) {
	return m.jobs, nil
}

func (m *mockHRRepoJobs) ListJobsByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*hr.JobOffer, int, error) {
	return m.jobs, len(m.jobs), nil
}

func (m *mockHRRepoJobs) GetJobStatsByOrg(ctx context.Context, orgID int64) (hr.JobStatsResult, error) {
	return hr.JobStatsResult{
		PublishedCount:    len(m.jobs),
		ClosedCount:       0,
		TotalApplications: 0,
	}, nil
}

func (m *mockHRRepoJobs) CountApplicationsByOffer(ctx context.Context, offerID int64) (int, error) {
	return 0, nil
}

func (m *mockHRRepoJobs) CreateJobOffer(ctx context.Context, j *hr.JobOffer) error {
	j.ID = int64(len(m.createdJobs) + 1)
	m.createdJobs = append(m.createdJobs, j)
	return nil
}

func (m *mockHRRepoJobs) UpdateJobOffer(ctx context.Context, j *hr.JobOffer) error {
	for i, existing := range m.createdJobs {
		if existing.ID == j.ID {
			m.createdJobs[i] = j
			return nil
		}
	}
	return nil
}

func setupJobsTestRouter(orgRepo *mockOrgRepoJobs, hrRepo *mockHRRepoJobs) (*ui.UIHandler, http.Handler) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	orgSvc := org.NewService(orgRepo, logger)
	hrSvc := hr.NewService(hrRepo, logger)

	handler := ui.NewUIHandler(nil, orgSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, hrSvc, nil, logger)

	r := chi.NewRouter()
	r.Get("/customer/jobs", handler.CustomerJobsPage)
	r.Post("/customer/jobs", handler.CustomerJobCreateSubmit)
	r.Post("/customer/jobs/{id}/edit", handler.CustomerJobUpdateSubmit)

	return handler, r
}

func TestCustomerJobs_BranchDropdown_OwnerSeesAllBranches(t *testing.T) {
	branch1 := &org.Branch{ID: 101, OrganizationID: 10, Name: i18n.New("فرع المعادي", "Maadi Branch"), IsMain: true}
	branch2 := &org.Branch{ID: 102, OrganizationID: 10, Name: i18n.New("فرع التجمع", "Tagamoa Branch")}

	orgRepo := &mockOrgRepoJobs{branches: []*org.Branch{branch1, branch2}}
	hrRepo := &mockHRRepoJobs{}
	_, router := setupJobsTestRouter(orgRepo, hrRepo)

	req := httptest.NewRequest(http.MethodGet, "/customer/jobs", nil)
	ownerActor := authctx.Actor{
		UserID:         1,
		OrganizationID: 10,
		OrgType:        "customer",
		IsOwner:        true,
	}
	req = req.WithContext(authctx.WithActor(req.Context(), ownerActor))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "فرع المعادي") || !strings.Contains(body, "101") {
		t.Errorf("expected branch 101 in dropdown, body: %s", body)
	}
	if !strings.Contains(body, "فرع التجمع") || !strings.Contains(body, "102") {
		t.Errorf("expected branch 102 in dropdown, body: %s", body)
	}
}

func TestCustomerJobs_BranchDropdown_ScopedEmployeeOnlySeesAssignedBranch(t *testing.T) {
	branch1 := &org.Branch{ID: 101, OrganizationID: 10, Name: i18n.New("فرع المعادي", "Maadi Branch")}
	branch2 := &org.Branch{ID: 102, OrganizationID: 10, Name: i18n.New("فرع التجمع", "Tagamoa Branch")}

	orgRepo := &mockOrgRepoJobs{branches: []*org.Branch{branch1, branch2}}
	hrRepo := &mockHRRepoJobs{}
	_, router := setupJobsTestRouter(orgRepo, hrRepo)

	req := httptest.NewRequest(http.MethodGet, "/customer/jobs", nil)
	bID := int64(101)
	scopedEmployee := authctx.Actor{
		UserID:         2,
		OrganizationID: 10,
		OrgType:        "customer",
		BranchID:       &bID,
		Role:           "org_pharmacist",
	}
	req = req.WithContext(authctx.WithActor(req.Context(), scopedEmployee))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "فرع المعادي") || !strings.Contains(body, "101") {
		t.Errorf("expected branch 101 in dropdown")
	}
	if strings.Contains(body, "value=\"102\"") {
		t.Errorf("expected branch 102 NOT to be in dropdown for scoped employee")
	}
}

func TestCustomerJobs_CreateJob_ValidBranch(t *testing.T) {
	branch1 := &org.Branch{ID: 101, OrganizationID: 10, Name: i18n.New("فرع المعادي", "Maadi Branch")}
	orgRepo := &mockOrgRepoJobs{branches: []*org.Branch{branch1}}
	hrRepo := &mockHRRepoJobs{}
	_, router := setupJobsTestRouter(orgRepo, hrRepo)

	form := url.Values{}
	form.Set("title_ar", "صيدلي مسائي")
	form.Set("branch_id", "101")
	form.Set("status", "published")

	req := httptest.NewRequest(http.MethodPost, "/customer/jobs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	actor := authctx.Actor{
		UserID:         1,
		OrganizationID: 10,
		OrgType:        "customer",
		IsOwner:        true,
	}
	req = req.WithContext(authctx.WithActor(req.Context(), actor))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303, got %d", rec.Code)
	}

	if len(hrRepo.createdJobs) != 1 {
		t.Fatalf("expected 1 created job, got %d", len(hrRepo.createdJobs))
	}
	job := hrRepo.createdJobs[0]
	if job.Location != "فرع المعادي" {
		t.Errorf("expected Location 'فرع المعادي', got %q", job.Location)
	}
}

func TestCustomerJobs_CreateJob_UnauthorizedBranch_Rejected(t *testing.T) {
	branch1 := &org.Branch{ID: 101, OrganizationID: 10, Name: i18n.New("فرع المعادي", "Maadi Branch")}
	branch2 := &org.Branch{ID: 102, OrganizationID: 10, Name: i18n.New("فرع التجمع", "Tagamoa Branch")}
	orgRepo := &mockOrgRepoJobs{branches: []*org.Branch{branch1, branch2}}
	hrRepo := &mockHRRepoJobs{}
	_, router := setupJobsTestRouter(orgRepo, hrRepo)

	form := url.Values{}
	form.Set("title_ar", "صيدلي مسائي")
	form.Set("branch_id", "102") // Attempts to publish on branch 102
	form.Set("status", "published")

	req := httptest.NewRequest(http.MethodPost, "/customer/jobs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	bID := int64(101)
	scopedActor := authctx.Actor{
		UserID:         2,
		OrganizationID: 10,
		OrgType:        "customer",
		BranchID:       &bID, // Scoped to branch 101 only
		Role:           "org_pharmacist",
	}
	req = req.WithContext(authctx.WithActor(req.Context(), scopedActor))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "notice=error") {
		t.Errorf("expected redirect with error notice, got Location: %s", loc)
	}

	if len(hrRepo.createdJobs) != 0 {
		t.Errorf("expected 0 created jobs due to authorization failure, got %d", len(hrRepo.createdJobs))
	}
}
