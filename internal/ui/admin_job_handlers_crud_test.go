package ui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

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
