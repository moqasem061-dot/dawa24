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
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type mockOrgRepoBranchSelector struct {
	org.Repository
	branches []*org.Branch
}

func (m *mockOrgRepoBranchSelector) ListBranchesByOrg(ctx context.Context, orgID int64) ([]*org.Branch, error) {
	var res []*org.Branch
	for _, b := range m.branches {
		if b.OrganizationID == orgID {
			res = append(res, b)
		}
	}
	return res, nil
}

func (m *mockOrgRepoBranchSelector) GetBranchByID(ctx context.Context, id int64) (*org.Branch, error) {
	for _, b := range m.branches {
		if b.ID == id {
			return b, nil
		}
	}
	return nil, apperr.NotFound("branch")
}

func setupBranchSelectorRouter(repo *mockOrgRepoBranchSelector) (*ui.UIHandler, http.Handler) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	orgSvc := org.NewService(repo, logger)

	handler := ui.NewUIHandler(nil, orgSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	r.Use(handler.BuyingBranchSelector)
	r.Post("/customer/set-branch", handler.SetBuyingBranchSubmit)
	r.Post("/vendor/set-branch", handler.SetBuyingBranchSubmit)

	return handler, r
}

func TestBranchSelector_EmployeeWithAllBranches_CanSwitch_Pharmacy(t *testing.T) {
	branch1 := &org.Branch{ID: 101, OrganizationID: 10, Name: i18n.New("فرع المعادي", "Maadi Branch"), IsMain: true, Status: "active"}
	branch2 := &org.Branch{ID: 102, OrganizationID: 10, Name: i18n.New("فرع التجمع", "Tagamoa Branch"), Status: "active"}

	repo := &mockOrgRepoBranchSelector{branches: []*org.Branch{branch1, branch2}}
	_, router := setupBranchSelectorRouter(repo)

	// Pharmacy employee who has ALL branches (BoundBranchID is nil)
	empActor := authctx.Actor{
		UserID:         2,
		OrganizationID: 10,
		OrgType:        "customer",
		Scope:          rbac.ScopePharmacy,
		IsOwner:        false,
		BoundBranchID:  nil, // All branches!
	}

	form := url.Values{}
	form.Set("branch_id", "102")
	form.Set("redirect_to", "/customer/dashboard")

	req := httptest.NewRequest(http.MethodPost, "/customer/set-branch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Set initial cookie to branch 101
	req.AddCookie(&http.Cookie{Name: "dawa24_buying_branch", Value: "101"})
	req = req.WithContext(authctx.WithActor(req.Context(), empActor))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 SeeOther redirect, got %d, body: %s", rec.Code, rec.Body.String())
	}

	loc := rec.Header().Get("Location")
	if loc != "/customer/dashboard" {
		t.Errorf("expected redirect to /customer/dashboard, got %q", loc)
	}

	// Verify cookie is set to 102
	cookies := rec.Result().Cookies()
	var branchCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "dawa24_buying_branch" {
			branchCookie = c
			break
		}
	}
	if branchCookie == nil || branchCookie.Value != "102" {
		t.Errorf("expected dawa24_buying_branch cookie to be '102', got %+v", branchCookie)
	}
}

func TestBranchSelector_EmployeeStrictlyBound_Locked_Pharmacy(t *testing.T) {
	branch1 := &org.Branch{ID: 101, OrganizationID: 10, Name: i18n.New("فرع المعادي", "Maadi Branch"), IsMain: true, Status: "active"}
	branch2 := &org.Branch{ID: 102, OrganizationID: 10, Name: i18n.New("فرع التجمع", "Tagamoa Branch"), Status: "active"}

	repo := &mockOrgRepoBranchSelector{branches: []*org.Branch{branch1, branch2}}
	_, router := setupBranchSelectorRouter(repo)

	// Pharmacy employee strictly bound to branch 101
	boundID := int64(101)
	empActor := authctx.Actor{
		UserID:         3,
		OrganizationID: 10,
		OrgType:        "customer",
		Scope:          rbac.ScopePharmacy,
		IsOwner:        false,
		BoundBranchID:  &boundID, // Bound strictly to 101!
	}

	form := url.Values{}
	form.Set("branch_id", "102") // Attempting to switch to branch 102

	req := httptest.NewRequest(http.MethodPost, "/customer/set-branch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(req.Context(), empActor))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Should be refused and redirected to /customer/dashboard without updating cookie to 102
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != "/customer/dashboard" {
		t.Errorf("expected redirect to home /customer/dashboard, got %q", loc)
	}

	cookies := rec.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "dawa24_buying_branch" && c.Value == "102" {
			t.Errorf("locked employee should not have cookie set to 102")
		}
	}
}

func TestBranchSelector_EmployeeWithAllBranches_CanSwitch_Vendor(t *testing.T) {
	branch1 := &org.Branch{ID: 201, OrganizationID: 20, Name: i18n.New("مخزن الدلتا", "Delta Warehouse"), IsMain: true, Status: "active"}
	branch2 := &org.Branch{ID: 202, OrganizationID: 20, Name: i18n.New("مخزن الصعيد", "Upper Egypt Warehouse"), Status: "active"}

	repo := &mockOrgRepoBranchSelector{branches: []*org.Branch{branch1, branch2}}
	_, router := setupBranchSelectorRouter(repo)

	// Vendor employee who has ALL branches (BoundBranchID is nil)
	empActor := authctx.Actor{
		UserID:         4,
		OrganizationID: 20,
		OrgType:        "vendor",
		Scope:          rbac.ScopeVendor,
		IsOwner:        false,
		BoundBranchID:  nil, // All branches!
	}

	form := url.Values{}
	form.Set("branch_id", "202")
	form.Set("redirect_to", "/vendor/inventory")

	req := httptest.NewRequest(http.MethodPost, "/vendor/set-branch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "dawa24_buying_branch", Value: "201"})
	req = req.WithContext(authctx.WithActor(req.Context(), empActor))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 SeeOther redirect, got %d, body: %s", rec.Code, rec.Body.String())
	}

	loc := rec.Header().Get("Location")
	if loc != "/vendor/inventory" {
		t.Errorf("expected redirect to /vendor/inventory, got %q", loc)
	}

	// Verify cookie is set to 202
	cookies := rec.Result().Cookies()
	var branchCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "dawa24_buying_branch" {
			branchCookie = c
			break
		}
	}
	if branchCookie == nil || branchCookie.Value != "202" {
		t.Errorf("expected dawa24_buying_branch cookie to be '202', got %+v", branchCookie)
	}
}

func TestBranchSelector_EmployeeStrictlyBound_Locked_Vendor(t *testing.T) {
	branch1 := &org.Branch{ID: 201, OrganizationID: 20, Name: i18n.New("مخزن الدلتا", "Delta Warehouse"), IsMain: true, Status: "active"}
	branch2 := &org.Branch{ID: 202, OrganizationID: 20, Name: i18n.New("مخزن الصعيد", "Upper Egypt Warehouse"), Status: "active"}

	repo := &mockOrgRepoBranchSelector{branches: []*org.Branch{branch1, branch2}}
	_, router := setupBranchSelectorRouter(repo)

	// Vendor employee strictly bound to branch 201
	boundID := int64(201)
	empActor := authctx.Actor{
		UserID:         5,
		OrganizationID: 20,
		OrgType:        "vendor",
		Scope:          rbac.ScopeVendor,
		IsOwner:        false,
		BoundBranchID:  &boundID,
	}

	form := url.Values{}
	form.Set("branch_id", "202")

	req := httptest.NewRequest(http.MethodPost, "/vendor/set-branch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(req.Context(), empActor))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != "/vendor/dashboard" {
		t.Errorf("expected redirect to vendor dashboard, got %q", loc)
	}

	cookies := rec.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "dawa24_buying_branch" && c.Value == "202" {
			t.Errorf("locked vendor employee should not have cookie set to 202")
		}
	}
}

func TestBranchSelector_Owner_CanSwitch(t *testing.T) {
	branch1 := &org.Branch{ID: 301, OrganizationID: 30, Name: i18n.New("فرع رئيسي", "Main"), IsMain: true, Status: "active"}
	branch2 := &org.Branch{ID: 302, OrganizationID: 30, Name: i18n.New("فرع فرعي", "Sub"), Status: "active"}

	repo := &mockOrgRepoBranchSelector{branches: []*org.Branch{branch1, branch2}}
	_, router := setupBranchSelectorRouter(repo)

	ownerActor := authctx.Actor{
		UserID:         1,
		OrganizationID: 30,
		OrgType:        "customer",
		Scope:          rbac.ScopePharmacy,
		IsOwner:        true,
	}

	form := url.Values{}
	form.Set("branch_id", "302")

	req := httptest.NewRequest(http.MethodPost, "/customer/set-branch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "http://localhost:8080/customer/branches")
	req = req.WithContext(authctx.WithActor(req.Context(), ownerActor))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 SeeOther redirect, got %d", rec.Code)
	}

	loc := rec.Header().Get("Location")
	if loc != "/customer/branches" {
		t.Errorf("expected referer redirect to /customer/branches, got %q", loc)
	}

	cookies := rec.Result().Cookies()
	var branchCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "dawa24_buying_branch" {
			branchCookie = c
			break
		}
	}
	if branchCookie == nil || branchCookie.Value != "302" {
		t.Errorf("expected dawa24_buying_branch cookie to be '302', got %+v", branchCookie)
	}
}
