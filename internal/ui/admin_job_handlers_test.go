package ui_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type mockAdminHRRepo struct {
	hr.Repository
	jobs         []*hr.JobOffer
	applications []*hr.JobApplication
}

func (m *mockAdminHRRepo) ListAllJobsFiltered(ctx context.Context, filter hr.AdminJobFilter) ([]*hr.JobOffer, int, error) {
	var result []*hr.JobOffer
	for _, j := range m.jobs {
		if filter.OrganizationID > 0 && j.OrganizationID != filter.OrganizationID {
			continue
		}
		if filter.Status != "" && j.Status != filter.Status {
			continue
		}
		if filter.Search != "" && !strings.Contains(j.Title.Get(i18n.AR), filter.Search) && !strings.Contains(j.Location, filter.Search) {
			continue
		}
		result = append(result, j)
	}
	return result, len(result), nil
}

func (m *mockAdminHRRepo) GetJobOfferByID(ctx context.Context, id int64) (*hr.JobOffer, error) {
	for _, j := range m.jobs {
		if j.ID == id {
			return j, nil
		}
	}
	return nil, nil
}

func (m *mockAdminHRRepo) CreateJobOffer(ctx context.Context, j *hr.JobOffer) error {
	j.ID = int64(len(m.jobs) + 1)
	j.CreatedAt = time.Now()
	j.UpdatedAt = time.Now()
	m.jobs = append(m.jobs, j)
	return nil
}

func (m *mockAdminHRRepo) UpdateJobOffer(ctx context.Context, j *hr.JobOffer) error {
	for i, existing := range m.jobs {
		if existing.ID == j.ID {
			m.jobs[i] = j
			return nil
		}
	}
	return nil
}

func (m *mockAdminHRRepo) ToggleJobOfferStatus(ctx context.Context, orgID, jobID int64) error {
	for _, j := range m.jobs {
		if j.ID == jobID {
			if j.Status == "published" {
				j.Status = "closed"
			} else {
				j.Status = "published"
			}
			return nil
		}
	}
	return nil
}

func (m *mockAdminHRRepo) DeleteJobOffer(ctx context.Context, orgID, jobID int64) error {
	var remaining []*hr.JobOffer
	for _, j := range m.jobs {
		if j.ID != jobID {
			remaining = append(remaining, j)
		}
	}
	m.jobs = remaining
	return nil
}

func (m *mockAdminHRRepo) CountApplicationsByOffer(ctx context.Context, offerID int64) (int, error) {
	cnt := 0
	for _, a := range m.applications {
		if a.JobOfferID == offerID {
			cnt++
		}
	}
	return cnt, nil
}

func (m *mockAdminHRRepo) ListApplicationsByOffer(ctx context.Context, offerID int64, limit, offset int) ([]*hr.JobApplication, error) {
	var result []*hr.JobApplication
	for _, a := range m.applications {
		if a.JobOfferID == offerID {
			result = append(result, a)
		}
	}
	return result, nil
}

func (m *mockAdminHRRepo) AcceptAndOnboardApplicant(ctx context.Context, in hr.AcceptApplicantInput) (*hr.JobApplication, error) {
	for _, a := range m.applications {
		if a.ID == in.ApplicationID {
			a.Status = "accepted"
			a.AssignedRoleKey = in.RoleKey
			a.JobTitle = in.JobTitle
			return a, nil
		}
	}
	return nil, nil
}

func (m *mockAdminHRRepo) RejectApplicant(ctx context.Context, orgID, appID int64, notes string) (*hr.JobApplication, error) {
	for _, a := range m.applications {
		if a.ID == appID {
			a.Status = "rejected"
			a.Notes = notes
			return a, nil
		}
	}
	return nil, nil
}

type mockAdminOrgRepo struct {
	org.Repository
	orgs []*org.Organization
}

func (m *mockAdminOrgRepo) ListOrganizations(ctx context.Context, orgType *org.OrganizationType, status *org.OrganizationStatus, limit, offset int) ([]*org.Organization, error) {
	return m.orgs, nil
}

func (m *mockAdminOrgRepo) GetOrganizationsByIDs(ctx context.Context, ids []int64) ([]*org.Organization, error) {
	var res []*org.Organization
	for _, o := range m.orgs {
		for _, id := range ids {
			if o.ID == id {
				res = append(res, o)
			}
		}
	}
	return res, nil
}

func (m *mockAdminOrgRepo) GetOrganizationByID(ctx context.Context, id int64) (*org.Organization, error) {
	for _, o := range m.orgs {
		if o.ID == id {
			return o, nil
		}
	}
	return nil, nil
}

func setupAdminJobsTest() (*chi.Mux, *mockAdminHRRepo, *mockAdminOrgRepo, authctx.Actor) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	org1 := &org.Organization{
		ID:        1,
		LegalName: "United Pharma",
		TradeName: i18n.Text{"ar": "شركة المتحدون للأدوية"},
		Type:      org.TypeVendor,
	}
	org2 := &org.Organization{
		ID:        2,
		LegalName: "Al-Amal Pharmacy",
		TradeName: i18n.Text{"ar": "صيدلية الأمل"},
		Type:      org.TypeCustomer,
	}

	job1 := &hr.JobOffer{
		ID:             1,
		OrganizationID: 1,
		Title:          i18n.Text{"ar": "مدير مبيعات", "en": "Sales Manager"},
		Description:    "Sales job description",
		Requirements:   "Requirements here",
		Location:       "القاهرة - المعادي",
		SalaryMin:      money.FromMajor(15000),
		SalaryMax:      money.FromMajor(20000),
		Status:         "published",
		CreatedAt:      time.Now(),
	}
	job2 := &hr.JobOffer{
		ID:             2,
		OrganizationID: 2,
		Title:          i18n.Text{"ar": "صيدلي مسائي", "en": "Evening Pharmacist"},
		Description:    "Pharmacy job description",
		Requirements:   "Pharmacy degree",
		Location:       "الجيزة - الدقي",
		SalaryMin:      money.FromMajor(7000),
		SalaryMax:      money.FromMajor(9000),
		Status:         "closed",
		CreatedAt:      time.Now(),
	}

	app1 := &hr.JobApplication{
		ID:             10,
		JobOfferID:     1,
		OrganizationID: 1,
		ApplicantName:  "Ahmed Ali",
		ApplicantEmail: "ahmed@example.com",
		ApplicantPhone: "01012345678",
		Status:         "pending",
		CreatedAt:      time.Now(),
	}

	hrRepo := &mockAdminHRRepo{
		jobs:         []*hr.JobOffer{job1, job2},
		applications: []*hr.JobApplication{app1},
	}
	orgRepo := &mockAdminOrgRepo{
		orgs: []*org.Organization{org1, org2},
	}

	hrSvc := hr.NewService(hrRepo, logger)
	orgSvc := org.NewService(orgRepo, logger)

	handler := ui.NewUIHandler(
		nil, orgSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, hrSvc, nil, logger,
	)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)

	adminActor := authctx.Actor{
		UserID:      100,
		IsStaff:     true,
		Role:        "super_admin",
		Permissions: []string{"*"},
	}

	return r, hrRepo, orgRepo, adminActor
}

func TestAdminJobs_ListingAndFilters(t *testing.T) {
	r, _, _, adminActor := setupAdminJobsTest()

	t.Run("GET /admin/jobs returns 200 and lists vacancies", func(t *testing.T) {
		ctx := authctx.WithActor(context.Background(), adminActor)
		req, _ := http.NewRequestWithContext(ctx, "GET", "/admin/jobs", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "مدير مبيعات") || !strings.Contains(body, "صيدلي مسائي") {
			t.Errorf("expected job titles in HTML body")
		}
		if !strings.Contains(body, "شركة المتحدون للأدوية") {
			t.Errorf("expected company name in HTML body")
		}
	})

	t.Run("GET /admin/jobs with org_id filter", func(t *testing.T) {
		ctx := authctx.WithActor(context.Background(), adminActor)
		req, _ := http.NewRequestWithContext(ctx, "GET", "/admin/jobs?org_id=1", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "مدير مبيعات") {
			t.Errorf("expected org 1 job in HTML")
		}
		if strings.Contains(body, "صيدلي مسائي") {
			t.Errorf("expected org 2 job to be filtered out")
		}
	})

	t.Run("GET /admin/jobs with status filter", func(t *testing.T) {
		ctx := authctx.WithActor(context.Background(), adminActor)
		req, _ := http.NewRequestWithContext(ctx, "GET", "/admin/jobs?status=closed", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "صيدلي مسائي") {
			t.Errorf("expected closed job in HTML")
		}
		if strings.Contains(body, "مدير مبيعات") {
			t.Errorf("expected published job to be filtered out")
		}
	})
}

func TestAdminJobs_DetailAndApplications(t *testing.T) {
	r, _, _, adminActor := setupAdminJobsTest()

	t.Run("GET /admin/jobs/1 returns 200 with job details and applications", func(t *testing.T) {
		ctx := authctx.WithActor(context.Background(), adminActor)
		req, _ := http.NewRequestWithContext(ctx, "GET", "/admin/jobs/1", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "مدير مبيعات") {
			t.Errorf("expected job title in detail HTML")
		}
		if !strings.Contains(body, "Ahmed Ali") {
			t.Errorf("expected applicant name in applications table")
		}
	})

	t.Run("GET /admin/jobs/999 redirects when not found", func(t *testing.T) {
		ctx := authctx.WithActor(context.Background(), adminActor)
		req, _ := http.NewRequestWithContext(ctx, "GET", "/admin/jobs/999", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rr.Code)
		}
	})

	t.Run("GET /admin/jobs/1/applications returns JSON array", func(t *testing.T) {
		ctx := authctx.WithActor(context.Background(), adminActor)
		req, _ := http.NewRequestWithContext(ctx, "GET", "/admin/jobs/1/applications", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		var apps []*hr.JobApplication
		if err := json.Unmarshal(rr.Body.Bytes(), &apps); err != nil {
			t.Fatalf("failed to decode JSON response: %v", err)
		}
		if len(apps) != 1 || apps[0].ApplicantName != "Ahmed Ali" {
			t.Errorf("unexpected applications: %+v", apps)
		}
	})
}

func TestAdminJobs_CRUD(t *testing.T) {
	r, hrRepo, _, adminActor := setupAdminJobsTest()

	t.Run("POST /admin/jobs/new creates vacancy for specified org", func(t *testing.T) {
		ctx := authctx.WithActor(context.Background(), adminActor)
		form := url.Values{
			"organization_id": {"2"},
			"title_ar":        {"مساعد صيدلي"},
			"title_en":        {"Pharmacy Assistant"},
			"location":        {"مدينة نصر"},
			"salary_min":      {"5000"},
			"salary_max":      {"6000"},
			"status":          {"published"},
			"description":     {"Assistant duties"},
			"requirements":    {"Diploma"},
		}
		req, _ := http.NewRequestWithContext(ctx, "POST", "/admin/jobs/new", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rr.Code)
		}

		// Verify job in repository
		if len(hrRepo.jobs) != 3 {
			t.Fatalf("expected 3 jobs, got %d", len(hrRepo.jobs))
		}
		newJob := hrRepo.jobs[2]
		if newJob.Title.Get(i18n.AR) != "مساعد صيدلي" || newJob.OrganizationID != 2 {
			t.Errorf("unexpected created job: %+v", newJob)
		}
	})

	t.Run("POST /admin/jobs/1/edit updates vacancy", func(t *testing.T) {
		ctx := authctx.WithActor(context.Background(), adminActor)
		form := url.Values{
			"title_ar":     {"مدير مبيعات أول"},
			"title_en":     {"Senior Sales Manager"},
			"location":     {"التجمع الخامس"},
			"salary_min":   {"18000"},
			"salary_max":   {"25000"},
			"status":       {"published"},
			"description":  {"Updated description"},
			"requirements": {"Updated reqs"},
		}
		req, _ := http.NewRequestWithContext(ctx, "POST", "/admin/jobs/1/edit", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rr.Code)
		}

		job1 := hrRepo.jobs[0]
		if job1.Title.Get(i18n.AR) != "مدير مبيعات أول" || job1.Location != "التجمع الخامس" {
			t.Errorf("job was not updated: %+v", job1)
		}
	})

	t.Run("POST /admin/jobs/1/toggle toggles status", func(t *testing.T) {
		ctx := authctx.WithActor(context.Background(), adminActor)
		req, _ := http.NewRequestWithContext(ctx, "POST", "/admin/jobs/1/toggle", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rr.Code)
		}

		job1 := hrRepo.jobs[0]
		if job1.Status != "closed" {
			t.Errorf("expected status 'closed', got '%s'", job1.Status)
		}
	})

	t.Run("POST /admin/jobs/2/delete deletes vacancy", func(t *testing.T) {
		ctx := authctx.WithActor(context.Background(), adminActor)
		req, _ := http.NewRequestWithContext(ctx, "POST", "/admin/jobs/2/delete", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rr.Code)
		}

		for _, j := range hrRepo.jobs {
			if j.ID == 2 {
				t.Errorf("job 2 was not deleted")
			}
		}
	})
}

func TestAdminJobs_ApplicationAcceptAndReject(t *testing.T) {
	r, hrRepo, _, adminActor := setupAdminJobsTest()

	t.Run("POST /admin/jobs/1/applications/10/accept", func(t *testing.T) {
		ctx := authctx.WithActor(context.Background(), adminActor)
		form := url.Values{
			"role_key":    {"org_sales_rep"},
			"job_title":   {"Senior Sales Rep"},
			"base_salary": {"12000"},
			"notes":       {"Hired on probation"},
		}
		req, _ := http.NewRequestWithContext(ctx, "POST", "/admin/jobs/1/applications/10/accept", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rr.Code)
		}

		app := hrRepo.applications[0]
		if app.Status != "accepted" || app.AssignedRoleKey != "org_sales_rep" {
			t.Errorf("application was not accepted properly: %+v", app)
		}
	})

	t.Run("POST /admin/jobs/1/applications/10/reject", func(t *testing.T) {
		// Reset application status
		hrRepo.applications[0].Status = "pending"

		ctx := authctx.WithActor(context.Background(), adminActor)
		form := url.Values{
			"notes": {"Does not meet requirements"},
		}
		req, _ := http.NewRequestWithContext(ctx, "POST", "/admin/jobs/1/applications/10/reject", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rr.Code)
		}

		app := hrRepo.applications[0]
		if app.Status != "rejected" || app.Notes != "Does not meet requirements" {
			t.Errorf("application was not rejected properly: %+v", app)
		}
	})
}
