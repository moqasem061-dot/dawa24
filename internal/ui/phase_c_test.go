package ui_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// TestPhaseC_VendorContentAndPolicies verifies Task C.5: Vendor policies and social media forms.
func TestPhaseC_VendorContentAndPolicies(t *testing.T) {
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(handler)
	vendorActor := authctx.Actor{UserID: 10, OrganizationID: 2, Role: "vendor", OrgType: "vendor", Permissions: []string{"vendor.*"}}

	// GET /vendor/policies renders
	rec := doGET(t, r, "/vendor/policies", vendorActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "سياسات التوريد والدفع والاسترجاع")

	// POST /vendor/policies submits and obeys Law 3 (returns error notice when service is unavailable)
	rec = doPOST(t, r, "/vendor/policies", url.Values{
		"return_policy":   []string{"سياسة الاسترجاع"},
		"shipping_policy": []string{"سياسة الشحن والتوصيل"},
	}, vendorActor)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "/vendor/policies")
	assert.Contains(t, rec.Header().Get("Location"), "notice=error")
}

// TestPhaseC_VendorSavingProducts verifies Task C.11: Vendor saving products screens.
func TestPhaseC_VendorSavingProducts(t *testing.T) {
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(handler)
	vendorActor := authctx.Actor{UserID: 10, OrganizationID: 2, Role: "vendor", OrgType: "vendor", Permissions: []string{"vendor.*"}}

	// GET /vendor/saving-products renders
	rec := doGET(t, r, "/vendor/saving-products", vendorActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "منتجات التوفير")

	// GET /vendor/saving-products/import renders the import wizard page
	rec = doGET(t, r, "/vendor/saving-products/import", vendorActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "استيراد")

	// POST /vendor/saving-products/bulk-delete redirects to list
	rec = doPOST(t, r, "/vendor/saving-products/bulk-delete", url.Values{"selected_ids": []string{"201", "202"}}, vendorActor)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.True(t, strings.HasPrefix(rec.Header().Get("Location"), "/vendor/saving-products"))
}

// TestPhaseC_VendorPharmacyCoverage verifies Task C.12: the pharmacy coverage
// screen. The standalone institutional-work page was removed — a branch's
// corporate business now lives only on its card in the Supplier Branches page.
func TestPhaseC_VendorPharmacyCoverage(t *testing.T) {
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(handler)
	vendorActor := authctx.Actor{UserID: 10, OrganizationID: 2, Role: "vendor", OrgType: "vendor", Permissions: []string{"vendor.*"}}

	// GET /vendor/pharmacy-coverage renders
	rec := doGET(t, r, "/vendor/pharmacy-coverage", vendorActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "المشمولة في نطاق التغطية الأسبوعية")
}

// TestPhaseC_AdminReferenceData verifies Task C.6: Admin countries, social media, highlight sections, API integrations.
func TestPhaseC_AdminReferenceData(t *testing.T) {
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(handler)
	adminActor := authctx.Actor{UserID: 1, IsStaff: true, Role: "super_admin", Permissions: []string{"*"}}

	// Categories
	rec := doGET(t, r, "/admin/categories", adminActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "فئات المنتجات")

	// Highlight Sections (Redirects to Content sections tab)
	rec = doGET(t, r, "/admin/highlight-sections", adminActor)
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)

	// Content & Sections Management
	rec = doGET(t, r, "/admin/content", adminActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "محتوى الصفحات والأقسام")

	// API Integrations (Redirects to Developers)
	rec = doGET(t, r, "/admin/api-integrations", adminActor)
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)
}

// TestPhaseC_AdminFinanceScreens verifies Task C.3: Admin Invoices, Payments, Wallets, Plans.
func TestPhaseC_AdminFinanceScreens(t *testing.T) {
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(handler)
	adminActor := authctx.Actor{UserID: 1, IsStaff: true, Role: "super_admin", Permissions: []string{"*"}}

	// Unified Finance Hub
	rec := doGET(t, r, "/admin/finance", adminActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "المركز المالي والمحافظ والتحصيل")

	// Unified Plans Hub
	rec = doGET(t, r, "/admin/plans", adminActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "باقات واشتراكات المنظومة")

	// Legacy routes redirect to unified hubs
	rec = doGET(t, r, "/admin/invoices", adminActor)
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)

	rec = doGET(t, r, "/admin/payments", adminActor)
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)

	rec = doGET(t, r, "/admin/wallets", adminActor)
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)

	rec = doGET(t, r, "/admin/plans-info", adminActor)
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)
}

// TestAdminPlans_CardGrid_ResponsiveWrapping verifies WO-32:
// /admin/plans renders 3, 6, and 9 plan cards legibly using .admin-plans-grid
// without breaking the layout or horizontal overflow.
func TestAdminPlans_CardGrid_ResponsiveWrapping(t *testing.T) {
	for _, count := range []int{3, 6, 9} {
		t.Run(fmt.Sprintf("%d_plans", count), func(t *testing.T) {
			plans := make([]*billing.Plan, count)
			for i := 0; i < count; i++ {
				slug := fmt.Sprintf("tier-%d", i+1)
				plans[i] = &billing.Plan{
					ID:               int64(i + 1),
					Slug:             slug,
					Name:             i18n.Text{"ar": fmt.Sprintf("باقة المستوى %d", i+1), "en": fmt.Sprintf("Tier %d Plan", i+1)},
					PriceMonth:       money.MustParse(fmt.Sprintf("%d00.00", i+1)),
					PriceYear:        money.MustParse(fmt.Sprintf("%d000.00", i+1)),
					DurationDays:     30,
					MaxLoginSessions: 3 + i,
					MaxDevices:       5 + i,
					AIPlanID:         "plan-dev",
					IsActive:         true,
					IsDefault:        i == 0,
				}
			}

			data := pages.AdminPlansData{
				ActiveTab: "plans",
				Plans:     plans,
			}

			var buf bytes.Buffer
			err := pages.AdminPlansHub(data, "ar", "rtl").Render(context.Background(), &buf)
			assert.NoError(t, err)

			html := buf.String()
			// Must use the responsive auto-fill admin-plans-grid
			assert.Contains(t, html, "admin-plans-grid")
			// Must use responsive admin-kpi-grid for top metrics
			assert.Contains(t, html, "admin-kpi-grid")

			// Every plan card must be rendered
			for i := 0; i < count; i++ {
				assert.Contains(t, html, fmt.Sprintf("tier-%d", i+1))
				assert.Contains(t, html, fmt.Sprintf("باقة المستوى %d", i+1))
			}
		})
	}
}
