package ui_test

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// Mocks for this test suite are defined in vendor_team_e2e_mock_test.go

func TestVendorTeam_CompleteOverhaul_E2E(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	idRepo := newMockIdentityRepoTeamTest()
	idSvc := identity.NewService(idRepo, nil, logger)
	orgRepo := newMockOrgRepoTeamTest()
	orgSvc := org.NewService(orgRepo, logger)

	h := ui.NewUIHandler(
		nil, orgSvc, nil, nil, nil, idSvc, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)

	r := chi.NewRouter()
	h.RegisterVendorRoutes(r)

	vendorActor := authctx.Actor{UserID: 1, OrganizationID: 55, OrgType: "vendor", Permissions: []string{"vendor.*"}}

	// 1. GET /vendor/team renders page
	t.Run("GET /vendor/team renders 200 OK", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/vendor/team", nil)
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "فريق العمل وصلاحيات الموظفين") {
			t.Errorf("expected body to contain 'فريق العمل وصلاحيات الموظفين'")
		}
		if !strings.Contains(body, "مستودع القاهرة الرئيسي") {
			t.Errorf("expected branch option in select dropdown")
		}
		// Verify dynamic role options are rendered from the database
		if !strings.Contains(body, `value="2"`) || !strings.Contains(body, "مدير") {
			t.Errorf("expected dynamic role 'مدير' (ID: 2) to be in rendered page options")
		}
		if !strings.Contains(body, `value="3"`) || !strings.Contains(body, "أمين مخزن") {
			t.Errorf("expected dynamic role 'أمين مخزن' (ID: 3) to be in rendered page options")
		}
		if !strings.Contains(body, `value="4"`) || !strings.Contains(body, "موظف مبيعات وتوريد") {
			t.Errorf("expected dynamic role 'موظف مبيعات وتوريد' (ID: 4) to be in rendered page options")
		}
		// Owner role (ID: 1) must NEVER be in assignable role options
		if strings.Contains(body, `>مالك المنشأة</option>`) || strings.Contains(body, `<option value="1">مالك`) {
			t.Errorf("owner role (ID: 1) should NOT be rendered in role options")
		}
	})

	// 2. POST /vendor/team/new adds new employee with dynamic role_id
	t.Run("POST /vendor/team/new creates employee and links to org with dynamic role_id", func(t *testing.T) {
		form := url.Values{}
		form.Set("name", "د. أحمد جمال")
		form.Set("email", "ahmed@supplier.com")
		form.Set("phone", "01099887766")
		form.Set("job_title", "مسؤول مبيعات وتوريد")
		form.Set("employee_code", "EMP-901")
		form.Set("role_id", "3") // Dynamic role from DB: أمين مخزن (org_warehouse)
		form.Set("password", "SecurePassword123!")
		form.Set("branch_id", "1")

		req := httptest.NewRequest("POST", "/vendor/team/new", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rec.Code)
		}

		if len(orgRepo.members) == 0 {
			t.Fatalf("expected 1 member in org, got 0")
		}

		var createdMember *org.Member
		for _, m := range orgRepo.members {
			createdMember = m
			break
		}

		if createdMember.JobTitle != "مسؤول مبيعات وتوريد" {
			t.Errorf("expected JobTitle 'مسؤول مبيعات وتوريد', got %s", createdMember.JobTitle)
		}
		if createdMember.EmployeeCode != "EMP-901" {
			t.Errorf("expected EmployeeCode 'EMP-901', got %s", createdMember.EmployeeCode)
		}
		if createdMember.BranchID == nil || *createdMember.BranchID != 1 {
			t.Errorf("expected BranchID 1, got %v", createdMember.BranchID)
		}
		if createdMember.RoleID != 3 {
			t.Errorf("expected RoleID 3, got %d", createdMember.RoleID)
		}
		if createdMember.OrgRoleID == nil || *createdMember.OrgRoleID != 3 {
			t.Errorf("expected OrgRoleID 3, got %v", createdMember.OrgRoleID)
		}
		if createdMember.RoleKey != "org_warehouse" {
			t.Errorf("expected RoleKey 'org_warehouse', got %s", createdMember.RoleKey)
		}
	})

	// 2b. POST /vendor/team/new refuses to assign owner role and falls back to employee
	t.Run("POST /vendor/team/new rejects owner role assignment", func(t *testing.T) {
		form := url.Values{}
		form.Set("name", "محاولة تعيين مالك")
		form.Set("email", "fakeowner@supplier.com")
		form.Set("phone", "01011112222")
		form.Set("job_title", "موظف عادي")
		form.Set("role_id", "1") // Attempts to assign ID: 1 (org_owner)
		form.Set("password", "SecurePassword123!")

		req := httptest.NewRequest("POST", "/vendor/team/new", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rec.Code)
		}

		var fakeOwnerMember *org.Member
		for _, m := range orgRepo.members {
			if m.JobTitle == "موظف عادي" {
				fakeOwnerMember = m
				break
			}
		}
		if fakeOwnerMember == nil {
			t.Fatalf("expected member to be created")
		}
		if fakeOwnerMember.RoleKey == "org_owner" || fakeOwnerMember.RoleID == 1 {
			t.Errorf("CRITICAL SECURITY FLAW: member was assigned owner role! RoleKey: %s, RoleID: %d", fakeOwnerMember.RoleKey, fakeOwnerMember.RoleID)
		}
	})

	// 3. POST /vendor/team/{id}/edit updates employee with new dynamic role_id
	t.Run("POST /vendor/team/{id}/edit updates employee role to manager", func(t *testing.T) {
		var memberID int64
		for id := range orgRepo.members {
			memberID = id
			break
		}

		editForm := url.Values{}
		editForm.Set("name", "د. أحمد جمال بعد الترقية")
		editForm.Set("phone", "01099887766")
		editForm.Set("job_title", "مدير العمليات")
		editForm.Set("employee_code", "EMP-901")
		editForm.Set("role_id", "2") // Promote to: مدير (org_manager)
		editForm.Set("branch_id", "1")
		editForm.Set("is_active", "true")

		req := httptest.NewRequest("POST", fmt.Sprintf("/vendor/team/%d/edit", memberID), strings.NewReader(editForm.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rec.Code)
		}

		updated := orgRepo.members[memberID]
		if updated.RoleID != 2 {
			t.Errorf("expected updated RoleID 2, got %d", updated.RoleID)
		}
		if updated.OrgRoleID == nil || *updated.OrgRoleID != 2 {
			t.Errorf("expected updated OrgRoleID 2, got %v", updated.OrgRoleID)
		}
		if updated.RoleKey != "org_manager" {
			t.Errorf("expected updated RoleKey 'org_manager', got %s", updated.RoleKey)
		}
	})

	// 4. POST /vendor/team/new with existing email links cleanly
	t.Run("POST /vendor/team/new with existing email links existing user", func(t *testing.T) {
		form := url.Values{}
		form.Set("name", "د. أحمد جمال المحدث")
		form.Set("email", "ahmed@supplier.com") // Already exists
		form.Set("phone", "01099887766")
		form.Set("job_title", "مدير عمليات")
		form.Set("role_key", "org_manager")
		form.Set("password", "SecurePassword123!")

		req := httptest.NewRequest("POST", "/vendor/team/new", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rec.Code)
		}
	})

	// 5. POST /vendor/team/{id}/toggle toggles status
	t.Run("POST /vendor/team/{id}/toggle toggles status", func(t *testing.T) {
		var memberID int64
		for id := range orgRepo.members {
			memberID = id
			break
		}

		req := httptest.NewRequest("POST", fmt.Sprintf("/vendor/team/%d/toggle", memberID), nil)
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rec.Code)
		}
	})

	// 6. POST /vendor/team/{id}/delete removes member
	t.Run("POST /vendor/team/{id}/delete removes member", func(t *testing.T) {
		var memberIDs []int64
		for id := range orgRepo.members {
			memberIDs = append(memberIDs, id)
		}

		for _, id := range memberIDs {
			req := httptest.NewRequest("POST", fmt.Sprintf("/vendor/team/%d/delete", id), nil)
			req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusSeeOther {
				t.Fatalf("expected 303 redirect, got %d", rec.Code)
			}
		}

		if len(orgRepo.members) != 0 {
			t.Errorf("expected 0 members remaining, got %d", len(orgRepo.members))
		}
	})
}
