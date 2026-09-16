package pages

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
)

// The quota screen must render its numbers, not just its chrome.
//
// A templ page that compiles can still panic on a nil map or silently print
// nothing for a field that was never wired to the handler, and neither shows up
// in a build. This renders the branch tab with a full row and asserts the
// figures a supplier actually acts on are on the page.

func quotaTestData() VendorQuotasData {
	last := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	released := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	return VendorQuotasData{
		ActiveTab: "branches",
		Summary: commerce.QuotaSummary{
			VariantsWithQuota: 2, BranchesConsuming: 3,
			BranchesExhausted: 1, TotalUnitsUsed: 47, ReleasesMade: 1,
		},
		BranchRows: []*commerce.BranchQuotaRow{
			{
				VariantID: 10, VariantName: "عبوة 20 قرص", ProductID: 5, ProductName: "بانادول",
				SKU: "PAN-20", QuotaLimit: 10,
				BranchID: 3, BranchName: "فرع المعادي",
				CustomerOrgID: 44, CustomerName: "صيدليات النور",
				Used: 10, OrderCount: 2, LastOrderAt: &last,
			},
			{
				VariantID: 10, VariantName: "عبوة 20 قرص", ProductID: 5, ProductName: "بانادول",
				SKU: "PAN-20", QuotaLimit: 10,
				BranchID: 4, BranchName: "فرع مدينة نصر",
				CustomerOrgID: 44, CustomerName: "صيدليات النور",
				Used: 3, OrderCount: 1, LastOrderAt: &last,
				ReleasedAt: &released, ReleaseCount: 1, ReleasedUnits: 6,
			},
		},
		VariantOptions: []commerce.QuotaOption{{ID: 10, Label: "بانادول عبوة 20 قرص"}},
		BranchOptions:  []commerce.QuotaOption{{ID: 3, Label: "صيدليات النور — فرع المعادي"}},
		Page:           1,
		PerPage:        25,
		TotalCount:     2,
		CanManage:      true,
	}
}

func renderQuotas(t *testing.T, data VendorQuotasData) string {
	t.Helper()
	var sb strings.Builder
	if err := VendorQuotas(data, "ar", "rtl").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render vendor quotas: %v", err)
	}
	return sb.String()
}

func TestVendorQuotasRendersBranchUsage(t *testing.T) {
	html := renderQuotas(t, quotaTestData())

	for _, want := range []string{
		"فرع المعادي",            // the branch
		"صيدليات النور",          // the company it belongs to
		"بانادول",                // the restricted item
		"استنفدت",                // branch 3 is at its limit
		"محرَّرة",                // branch 4 carries a reset
		"/vendor/quotas/release", // the reset action
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}

	// The remaining allowance for the branch that still has room. A row showing
	// the limit but not the remainder is the one number a supplier cannot work
	// without.
	if !strings.Contains(html, ">7<") && !strings.Contains(html, "7\n") {
		t.Error("the remaining allowance (7) does not appear on the page")
	}
}

// A viewer without vendor.quota.manage sees the figures and no buttons. The
// route gate is the control; this proves the page does not offer a door that
// will not open.
func TestVendorQuotasHidesActionsWithoutPermission(t *testing.T) {
	data := quotaTestData()
	data.CanManage = false
	html := renderQuotas(t, data)

	if strings.Contains(html, "/vendor/quotas/release") {
		t.Error("a viewer who cannot manage quotas must not be offered the reset button")
	}
	if !strings.Contains(html, "فرع المعادي") {
		t.Error("the figures must still be readable without the manage permission")
	}
}

// A supplier who has set no quotas at all gets the empty state rather than an
// empty table with a pager reading "0 of 0".
func TestVendorQuotasEmptyState(t *testing.T) {
	html := renderQuotas(t, VendorQuotasData{ActiveTab: "branches", Page: 1, PerPage: 25})
	if !strings.Contains(html, "لا توجد حصص بعد") {
		t.Error("expected the no-quotas-yet empty state")
	}
	if strings.Contains(html, "data-table") {
		t.Error("no table should render when the supplier has set no quota")
	}
}

// The items tab is where the cap itself is changed or lifted.
func TestVendorQuotasVariantTab(t *testing.T) {
	data := quotaTestData()
	data.ActiveTab = "variants"
	data.BranchRows = nil
	data.VariantRows = []*commerce.QuotaVariantRow{{
		VariantID: 10, VariantName: "عبوة 20 قرص", ProductID: 5, ProductName: "بانادول",
		SKU: "PAN-20", QuotaLimit: 10, BranchCount: 3, ExhaustedBranches: 1, TotalUsed: 23,
	}}
	html := renderQuotas(t, data)

	if !strings.Contains(html, "/vendor/quotas/limit") {
		t.Error("the items tab must offer the change/remove quota actions")
	}
	if !strings.Contains(html, "إلغاء الحصة") {
		t.Error("the items tab must offer removing the quota entirely")
	}
}

// The filters have to survive a tab switch and a page change, or a supplier who
// filtered to one branch loses it on every click.
func TestVendorQuotasKeepsFiltersAcrossTabs(t *testing.T) {
	data := quotaTestData()
	data.Query = "بانادول"
	data.BranchID = 3
	data.State = commerce.QuotaStateExhausted

	tabURL := data.quotaTabURL("variants")
	for _, want := range []string{"tab=variants", "branch=3", "state=exhausted"} {
		if !strings.Contains(tabURL, want) {
			t.Errorf("tab link %q is missing %q", tabURL, want)
		}
	}
	if strings.Contains(tabURL, "page=") {
		t.Errorf("switching tab must return to page 1, got %q", tabURL)
	}

	pager := data.PagerValues()
	if pager.Get("tab") != "branches" || pager.Get("branch") != "3" {
		t.Errorf("pager values lost the filter state: %v", pager)
	}
}
