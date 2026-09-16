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

	"github.com/muhiya/dawa24-store/internal/modules/hr"
	hrHttp "github.com/muhiya/dawa24-store/internal/modules/hr/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) CreateEmployee(context.Context, *hr.Employee) error {
	r.fail("CreateEmployee")
	return nil
}
func (r stubRepo) GetEmployeeByID(context.Context, int64) (*hr.Employee, error) {
	r.fail("GetEmployeeByID")
	return nil, nil
}
func (r stubRepo) ListEmployees(context.Context, int, int) ([]*hr.Employee, error) {
	r.fail("ListEmployees")
	return nil, nil
}
func (r stubRepo) SaveWorkTimes(context.Context, []*hr.WorkTime) error {
	r.fail("SaveWorkTimes")
	return nil
}
func (r stubRepo) ListWorkTimes(context.Context) ([]*hr.WorkTime, error) {
	r.fail("ListWorkTimes")
	return nil, nil
}

func (r stubRepo) ListPublishedJobs(context.Context, int, int) ([]*hr.JobOffer, error) {
	r.fail("ListPublishedJobs")
	return nil, nil
}
func (r stubRepo) ListPublishedJobsWithTotal(context.Context, int, int) ([]*hr.JobOffer, int, error) {
	r.fail("ListPublishedJobsWithTotal")
	return nil, 0, nil
}
func (r stubRepo) ListAllJobsWithTotal(context.Context, int, int) ([]*hr.JobOffer, int, error) {
	r.fail("ListAllJobsWithTotal")
	return nil, 0, nil
}
func (r stubRepo) ListAllJobsFiltered(context.Context, hr.AdminJobFilter) ([]*hr.JobOffer, int, error) {
	r.fail("ListAllJobsFiltered")
	return nil, 0, nil
}
func (r stubRepo) GetJobOfferByID(context.Context, int64) (*hr.JobOffer, error) {
	r.fail("GetJobOfferByID")
	return nil, nil
}
func (r stubRepo) CreateJobOffer(context.Context, *hr.JobOffer) error {
	r.fail("CreateJobOffer")
	return nil
}
func (r stubRepo) UpdateJobOffer(context.Context, *hr.JobOffer) error {
	r.fail("UpdateJobOffer")
	return nil
}
func (r stubRepo) DeleteJobOffer(context.Context, int64, int64) error {
	r.fail("DeleteJobOffer")
	return nil
}
func (r stubRepo) ToggleJobOfferStatus(context.Context, int64, int64) error {
	r.fail("ToggleJobOfferStatus")
	return nil
}
func (r stubRepo) ListJobsByOrg(context.Context, int64, int, int) ([]*hr.JobOffer, error) {
	r.fail("ListJobsByOrg")
	return nil, nil
}
func (r stubRepo) ListJobsByOrgWithTotal(context.Context, int64, int, int) ([]*hr.JobOffer, int, error) {
	r.fail("ListJobsByOrgWithTotal")
	return nil, 0, nil
}
func (r stubRepo) GetJobStatsByOrg(context.Context, int64) (hr.JobStatsResult, error) {
	r.fail("GetJobStatsByOrg")
	return hr.JobStatsResult{}, nil
}
func (r stubRepo) CreateJobApplication(context.Context, *hr.JobApplication) error {
	r.fail("CreateJobApplication")
	return nil
}
func (r stubRepo) ListApplicationsByOffer(context.Context, int64, int, int) ([]*hr.JobApplication, error) {
	r.fail("ListApplicationsByOffer")
	return nil, nil
}
func (r stubRepo) CountApplicationsByOffer(context.Context, int64) (int, error) {
	r.fail("CountApplicationsByOffer")
	return 0, nil
}
func (r stubRepo) ListApplicationsByUser(context.Context, int64) ([]*hr.JobApplication, error) {
	r.fail("ListApplicationsByUser")
	return nil, nil
}
func (r stubRepo) GetApplicationByID(context.Context, int64) (*hr.JobApplication, error) {
	r.fail("GetApplicationByID")
	return nil, nil
}
func (r stubRepo) AcceptAndOnboardApplicant(context.Context, hr.AcceptApplicantInput) (*hr.JobApplication, error) {
	r.fail("AcceptAndOnboardApplicant")
	return nil, nil
}
func (r stubRepo) RejectApplicant(context.Context, int64, int64, string) (*hr.JobApplication, error) {
	r.fail("RejectApplicant")
	return nil, nil
}
func (r stubRepo) GetJobSeekerProfile(context.Context, int64) (*hr.JobSeekerProfile, error) {
	r.fail("GetJobSeekerProfile")
	return nil, nil
}
func (r stubRepo) UpsertJobSeekerProfile(context.Context, *hr.JobSeekerProfile) error {
	r.fail("UpsertJobSeekerProfile")
	return nil
}

type happyRepo struct{}

func (happyRepo) CreateEmployee(ctx context.Context, e *hr.Employee) error {
	e.ID = 1
	return nil
}
func (happyRepo) GetEmployeeByID(ctx context.Context, id int64) (*hr.Employee, error) {
	return &hr.Employee{ID: id, UserID: 1, EmployeeCode: "EMP-01", JobTitle: "Pharmacist", BaseSalary: money.MustParse("5000.00")}, nil
}
func (happyRepo) ListEmployees(ctx context.Context, limit, offset int) ([]*hr.Employee, error) {
	return []*hr.Employee{{ID: 1, UserID: 1, EmployeeCode: "EMP-01", JobTitle: "Pharmacist"}}, nil
}
func (happyRepo) SaveWorkTimes(ctx context.Context, times []*hr.WorkTime) error {
	return nil
}
func (happyRepo) ListWorkTimes(ctx context.Context) ([]*hr.WorkTime, error) {
	return []*hr.WorkTime{{ID: 1, DayNameEn: "Monday", DayNameAr: "الاثنين", OpenTime: "09:00", CloseTime: "17:00"}}, nil
}

func (happyRepo) ListPublishedJobs(ctx context.Context, limit, offset int) ([]*hr.JobOffer, error) {
	return []*hr.JobOffer{{ID: 1, Title: i18n.Text{"ar": "صيدلي"}, Status: "published"}}, nil
}
func (happyRepo) ListPublishedJobsWithTotal(ctx context.Context, limit, offset int) ([]*hr.JobOffer, int, error) {
	return []*hr.JobOffer{{ID: 1, Title: i18n.Text{"ar": "صيدلي"}, Status: "published"}}, 1, nil
}
func (happyRepo) ListAllJobsWithTotal(ctx context.Context, limit, offset int) ([]*hr.JobOffer, int, error) {
	return []*hr.JobOffer{{ID: 1, Title: i18n.Text{"ar": "صيدلي"}, Status: "published"}}, 1, nil
}
func (happyRepo) ListAllJobsFiltered(ctx context.Context, filter hr.AdminJobFilter) ([]*hr.JobOffer, int, error) {
	return []*hr.JobOffer{{ID: 1, Title: i18n.Text{"ar": "صيدلي"}, Status: "published"}}, 1, nil
}
func (happyRepo) GetJobOfferByID(ctx context.Context, id int64) (*hr.JobOffer, error) {
	return &hr.JobOffer{ID: id, Title: i18n.Text{"ar": "صيدلي"}, Status: "published"}, nil
}
func (happyRepo) CreateJobOffer(ctx context.Context, j *hr.JobOffer) error {
	j.ID = 1
	return nil
}
func (happyRepo) UpdateJobOffer(ctx context.Context, j *hr.JobOffer) error {
	return nil
}
func (happyRepo) DeleteJobOffer(ctx context.Context, orgID, jobID int64) error {
	return nil
}
func (happyRepo) ToggleJobOfferStatus(ctx context.Context, orgID, jobID int64) error {
	return nil
}
func (happyRepo) ListJobsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*hr.JobOffer, error) {
	return []*hr.JobOffer{{ID: 1, OrganizationID: orgID, Title: i18n.Text{"ar": "صيدلي"}, Status: "published"}}, nil
}
func (happyRepo) ListJobsByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*hr.JobOffer, int, error) {
	return []*hr.JobOffer{{ID: 1, OrganizationID: orgID, Title: i18n.Text{"ar": "صيدلي"}, Status: "published"}}, 1, nil
}
func (happyRepo) GetJobStatsByOrg(ctx context.Context, orgID int64) (hr.JobStatsResult, error) {
	return hr.JobStatsResult{PublishedCount: 1, ClosedCount: 0, TotalApplications: 1}, nil
}
func (happyRepo) CreateJobApplication(ctx context.Context, a *hr.JobApplication) error {
	a.ID = 1
	return nil
}
func (happyRepo) ListApplicationsByOffer(ctx context.Context, offerID int64, limit, offset int) ([]*hr.JobApplication, error) {
	return []*hr.JobApplication{{ID: 1, JobOfferID: offerID, ApplicantName: "أحمد", ApplicantEmail: "a@example.com"}}, nil
}
func (happyRepo) CountApplicationsByOffer(ctx context.Context, offerID int64) (int, error) {
	return 1, nil
}
func (happyRepo) ListApplicationsByUser(ctx context.Context, userID int64) ([]*hr.JobApplication, error) {
	return []*hr.JobApplication{{ID: 1, ApplicantName: "أحمد", ApplicantEmail: "a@example.com"}}, nil
}
func (happyRepo) GetApplicationByID(ctx context.Context, id int64) (*hr.JobApplication, error) {
	return &hr.JobApplication{ID: id, Status: "pending"}, nil
}
func (happyRepo) AcceptAndOnboardApplicant(ctx context.Context, in hr.AcceptApplicantInput) (*hr.JobApplication, error) {
	return &hr.JobApplication{ID: in.ApplicationID, Status: "accepted", AssignedRoleKey: in.RoleKey}, nil
}
func (happyRepo) RejectApplicant(ctx context.Context, orgID, appID int64, notes string) (*hr.JobApplication, error) {
	return &hr.JobApplication{ID: appID, Status: "rejected", Notes: notes}, nil
}
func (happyRepo) GetJobSeekerProfile(ctx context.Context, userID int64) (*hr.JobSeekerProfile, error) {
	return &hr.JobSeekerProfile{ID: 1, UserID: userID, Specialisation: "pharmacist"}, nil
}
func (happyRepo) UpsertJobSeekerProfile(ctx context.Context, p *hr.JobSeekerProfile) error {
	p.ID = 1
	return nil
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	hrSvc := hr.NewService(stubRepo{t: t}, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("dawa24_session")
			if err != nil || cookie.Value == "" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			if cookie.Value == "forged-token-that-was-never-issued" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	hrHttp.NewHandler(hrSvc, log).RegisterRoutes(r)
	return r
}

func newAuthedRouter(repo hr.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	hrSvc := hr.NewService(repo, log)

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
				Permissions:    []string{"admin", "hr.admin"},
			}
			ctx := authctx.WithActor(r.Context(), actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	hrHttp.NewHandler(hrSvc, log).RegisterRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodPost, "/api/v1/hr/employees"},
	{http.MethodGet, "/api/v1/hr/employees/1"},
	{http.MethodGet, "/api/v1/hr/employees"},
	{http.MethodPost, "/api/v1/hr/work-times"},
	{http.MethodGet, "/api/v1/hr/work-times"},
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hr/employees", nil)
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

func TestHRHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"CreateEmployee", http.MethodPost, "/api/v1/hr/employees", `{"user_id":1,"employee_code":"EMP-01","job_title":"Pharmacist","base_salary":"5000.00"}`, http.StatusCreated},
		{"GetEmployee", http.MethodGet, "/api/v1/hr/employees/1", "", http.StatusOK},
		{"ListEmployees", http.MethodGet, "/api/v1/hr/employees?limit=10&offset=0", "", http.StatusOK},
		{"SaveWorkTimes", http.MethodPost, "/api/v1/hr/work-times", `[{"day_name_ar":"الاثنين","day_name_en":"Monday","open_time":"09:00","close_time":"17:00"}]`, http.StatusOK},
		{"ListWorkTimes", http.MethodGet, "/api/v1/hr/work-times", "", http.StatusOK},
		{"AdminEmployees", http.MethodGet, "/api/v1/admin/hr/employees", "", http.StatusOK},
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
