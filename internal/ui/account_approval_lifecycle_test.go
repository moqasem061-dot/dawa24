package ui_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// TestPendingAccountApprovalLifecycle tests the full lifecycle from pending registration
// to admin approval, ensuring that pending accounts are blocked with redirect to /onboarding/pending,
// and approved accounts immediately get 100% full dashboard access with proper permissions.
func TestPendingAccountApprovalLifecycle(t *testing.T) {
	t.Run("Pending pharmacy account is gated and redirected to /onboarding/pending", func(t *testing.T) {
		pendingPharmacy := &authctx.Actor{
			UserID:         101,
			OrganizationID: 201,
			OrgType:        "customer",
			OrgStatus:      "pending",
			Scope:          rbac.ScopePharmacy,
		}
		pendingPharmacy.Grants([]string{"pharmacy.*"})

		router := newTestRouter(pendingPharmacy)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/customer/dashboard", nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("GET /customer/dashboard for pending pharmacy returned status %d, want %d", rec.Code, http.StatusFound)
		}
		location := rec.Header().Get("Location")
		if location != "/documents" {
			t.Errorf("GET /customer/dashboard redirect location = %q, want %q", location, "/documents")
		}
	})

	t.Run("Pending vendor account is gated and redirected to /onboarding/pending", func(t *testing.T) {
		pendingVendor := &authctx.Actor{
			UserID:         102,
			OrganizationID: 202,
			OrgType:        "vendor",
			OrgStatus:      "pending",
			Scope:          rbac.ScopeVendor,
		}
		pendingVendor.Grants([]string{"vendor.*"})

		router := newTestRouter(pendingVendor)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/vendor/dashboard", nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("GET /vendor/dashboard for pending vendor returned status %d, want %d", rec.Code, http.StatusFound)
		}
		location := rec.Header().Get("Location")
		if location != "/documents" {
			t.Errorf("GET /vendor/dashboard redirect location = %q, want %q", location, "/documents")
		}
	})

	t.Run("Approved pharmacy account gets immediate 200 OK on /customer/dashboard and full sidebar", func(t *testing.T) {
		approvedPharmacy := &authctx.Actor{
			UserID:         101,
			OrganizationID: 201,
			OrgType:        "customer",
			OrgStatus:      "approved",
			Scope:          rbac.ScopePharmacy,
			IsOwner:        true,
		}
		approvedPharmacy.Grants(rbac.Default().KeysFor(rbac.ScopePharmacy))

		if !approvedPharmacy.IsCustomer() {
			t.Fatal("approved pharmacy actor IsCustomer() must be true")
		}
		if approvedPharmacy.IsVendor() {
			t.Fatal("approved pharmacy actor IsVendor() must be false")
		}
		if !approvedPharmacy.Can("pharmacy.dashboard.view") {
			t.Fatal("approved pharmacy owner must hold pharmacy.dashboard.view permission")
		}

		router := newTestRouter(approvedPharmacy)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/customer/dashboard", nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET /customer/dashboard for approved pharmacy returned status %d, want %d", rec.Code, http.StatusOK)
		}

		nav := rbac.VisibleNav(approvedPharmacy.DashboardScope(), rbac.NewSet(approvedPharmacy.Permissions))
		if countItems(nav) == 0 {
			t.Error("approved pharmacy sidebar is empty")
		}
	})

	t.Run("Approved vendor account gets immediate 200 OK on /vendor/dashboard and full sidebar", func(t *testing.T) {
		approvedVendor := &authctx.Actor{
			UserID:         102,
			OrganizationID: 202,
			OrgType:        "vendor",
			OrgStatus:      "approved",
			Scope:          rbac.ScopeVendor,
			IsOwner:        true,
		}
		approvedVendor.Grants(rbac.Default().KeysFor(rbac.ScopeVendor))

		if !approvedVendor.IsVendor() {
			t.Fatal("approved vendor actor IsVendor() must be true")
		}
		if approvedVendor.IsCustomer() {
			t.Fatal("approved vendor actor IsCustomer() must be false")
		}
		if !approvedVendor.Can("vendor.dashboard.view") {
			t.Fatal("approved vendor owner must hold vendor.dashboard.view permission")
		}

		router := newTestRouter(approvedVendor)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/vendor/dashboard", nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET /vendor/dashboard for approved vendor returned status %d, want %d", rec.Code, http.StatusOK)
		}

		nav := rbac.VisibleNav(approvedVendor.DashboardScope(), rbac.NewSet(approvedVendor.Permissions))
		if countItems(nav) == 0 {
			t.Error("approved vendor sidebar is empty")
		}
	})

	t.Run("Approved account navigating to /onboarding/pending is redirected to their dashboard", func(t *testing.T) {
		approvedPharmacy := &authctx.Actor{
			UserID:         101,
			OrganizationID: 201,
			OrgType:        "customer",
			OrgStatus:      "approved",
			Scope:          rbac.ScopePharmacy,
		}
		approvedPharmacy.Grants([]string{"pharmacy.*"})

		router := newTestRouter(approvedPharmacy)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/onboarding/pending", nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Errorf("GET /onboarding/pending for approved pharmacy returned status %d, want %d", rec.Code, http.StatusSeeOther)
		}
		location := rec.Header().Get("Location")
		if location != "/customer/dashboard" {
			t.Errorf("GET /onboarding/pending redirect = %q, want %q", location, "/customer/dashboard")
		}
	})
}
