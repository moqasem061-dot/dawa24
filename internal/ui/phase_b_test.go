package ui_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// TestPhaseB_UserListAliases_Redirect301 verifies Task B.1: user aliases return 301 to canonical /admin/users.
func TestPhaseB_UserListAliases_Redirect301(t *testing.T) {
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(handler)
	adminActor := authctx.Actor{UserID: 1, IsStaff: true, Role: "super_admin", Permissions: []string{"*"}}

	tests := []struct {
		path           string
		expectedTarget string
	}{
		{"/admin/full-user", "/admin/users"},
		{"/admin/full-user/new-clients", "/admin/users?type=new"},
		{"/admin/customer-list", "/admin/users?type=customer"},
		{"/admin/vendor-list", "/admin/users?type=vendor"},
		{"/admin/admin-list", "/admin/users?type=staff"},
		{"/admin/admins", "/admin/users?type=staff"},
		{"/admin/customer-list/42", "/admin/users/42"},
		{"/admin/vendor-list/42", "/admin/users/42"},
		{"/admin/admins/42", "/admin/users/42"},
		{"/admin/admins/42/edit", "/admin/users/42/edit"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := doGET(t, r, tt.path, adminActor)
			assert.Equal(t, http.StatusMovedPermanently, rec.Code)
			assert.Equal(t, tt.expectedTarget, rec.Header().Get("Location"))
		})
	}
}

// TestPhaseB_OrgAliases_Redirect301 verifies Task B.2: /admin/vendors and /admin/suppliers return 301 to /admin/organizations?type=vendor.
func TestPhaseB_OrgAliases_Redirect301(t *testing.T) {
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(handler)
	adminActor := authctx.Actor{UserID: 1, IsStaff: true, Role: "super_admin", Permissions: []string{"*"}}

	for _, path := range []string{"/admin/vendors", "/admin/suppliers"} {
		rec := doGET(t, r, path, adminActor)
		assert.Equal(t, http.StatusMovedPermanently, rec.Code)
		assert.Equal(t, "/admin/organizations?type=vendor", rec.Header().Get("Location"))
	}
}

// TestPhaseB_SponsorshipAndSavingAliases_Redirect301 verifies Task B.3: monetization and saving aliases return 301.
func TestPhaseB_SponsorshipAndSavingAliases_Redirect301(t *testing.T) {
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(handler)
	adminActor := authctx.Actor{UserID: 1, IsStaff: true, Role: "super_admin", Permissions: []string{"*"}}

	// 1. Sponsorships
	rec := doGET(t, r, "/admin/offer-sponsorships", adminActor)
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)
	assert.Equal(t, "/admin/offers-packages/sponsorships", rec.Header().Get("Location"))

	// 2. Saving Products typo alias
	rec = doGET(t, r, "/admin/saveing-products", adminActor)
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)
	assert.Equal(t, "/admin/saving-products", rec.Header().Get("Location"))
}

// TestPhaseB_PoliciesDuplication_Resolved verifies Task B.4: policy editing via settings is deleted.
func TestPhaseB_PoliciesDuplication_Resolved(t *testing.T) {
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(handler)
	adminActor := authctx.Actor{UserID: 1, IsStaff: true, Role: "super_admin", Permissions: []string{"*"}}

	// POST /admin/settings/policy is deleted and returns 404/405
	rec := doPOST(t, r, "/admin/settings/policy", url.Values{
		"policy_key": []string{"terms"},
		"title_ar":   []string{"الشروط"},
		"content_ar": []string{"المحتوى"},
	}, adminActor)
	assert.True(t, rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed)

	// GET /admin/settings body does not contain the old hardcoded fallback text
	rec = doGET(t, r, "/admin/settings?tab=policies", adminActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.NotContains(t, body, "يخضع استخدام هذه المنصة لكافة الضوابط واللوائح الصيدلانية والتجارية الصادرة عن هيئة الدواء")
}

// TestPhaseB_DeadLinks_Resolved verifies Task B.7: previously dead sidebar targets are reachable.
func TestPhaseB_DeadLinks_Resolved(t *testing.T) {
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(handler)
	adminActor := authctx.Actor{UserID: 1, IsStaff: true, Role: "super_admin", Permissions: []string{"*"}}
	customerActor := authctx.Actor{UserID: 2, OrganizationID: 10, OrgType: "customer", Permissions: []string{"pharmacy.*"}}

	// 1. /admin/notifications is reachable
	rec := doGET(t, r, "/admin/notifications", adminActor)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. /admin/products/import is reachable
	rec = doGET(t, r, "/admin/products/import", adminActor)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 3. /admin/import redirects 301 to /admin/products/import
	rec = doGET(t, r, "/admin/import", adminActor)
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)
	assert.Equal(t, "/admin/products/import", rec.Header().Get("Location"))

	// 4. POST /customer/branches/active is registered
	rec = doPOST(t, r, "/customer/branches/active", url.Values{"branch_id": []string{"1"}}, customerActor)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
}
