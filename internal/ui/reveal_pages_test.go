package ui

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	platformadmin "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin"
	"github.com/moqasem061-dot/dawa24/internal/shared/mask"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// The three pages that carry a reveal (plan.md S8, SC-010). Each page is rendered
// from the same view the handler builds, so the masked values here are the ones a
// real request would get. The clear values must not appear anywhere in the markup.

const revealPageVirtualKey = "fixture-virtual-marker-do-not-show"

func revealRenderHTML(t *testing.T, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func assertRevealHidden(t *testing.T, page string, clear ...string) {
	t.Helper()
	for _, v := range clear {
		if strings.Contains(page, v) {
			t.Fatalf("page source holds the clear value %q", v)
		}
	}
}

func TestRevealPagesHideValues(t *testing.T) {
	t.Run("admin organization detail", func(t *testing.T) {
		o := &org.Organization{
			ID: revealOrgID, LegalName: "صيدلية الاختبار",
			TaxNumber: revealTaxValue, CommercialRegister: revealCRValue, PharmacistLicense: revealLicenseValue,
			Phone: revealPhoneValue, Email: revealEmailValue,
		}
		page := revealRenderHTML(t, pages.AdminOrganizationDetailPage(
			pages.AdminOrgDetailData{Organization: maskedOrganization(o), CanReveal: true}, "ar", "rtl"))
		assertRevealHidden(t, page, revealTaxValue, revealCRValue, revealLicenseValue, revealPhoneValue, revealEmailValue)
		for _, masked := range []string{mask.Number(revealTaxValue), mask.Number(revealCRValue), mask.Number(revealLicenseValue)} {
			if !strings.Contains(page, masked) {
				t.Fatalf("masked form %q is missing from the page", masked)
			}
		}
		if !strings.Contains(page, `hx-post="/reveal"`) {
			t.Fatalf("the page has no reveal control")
		}
	})

	t.Run("vendor order detail", func(t *testing.T) {
		sh := &commerce.OrderShipment{
			ID: 700, OrderID: revealOrderID, OrganizationID: revealOrgID,
			ShipmentNumber: "SH-700", CustomerBranchPhone: revealBranchPhone,
		}
		page := revealRenderHTML(t, pages.VendorOrderDetail(pages.VendorOrderDetailData{
			Shipment: maskedShipment(sh), CanReveal: true, Lang: "ar", Dir: "rtl",
		}))
		assertRevealHidden(t, page, revealBranchPhone)
		if strings.Contains(page, "tel:"+revealBranchPhone) {
			t.Fatalf("the buyer branch phone is a tel: link before it is revealed")
		}
		if !strings.Contains(page, mask.Phone(revealBranchPhone)) {
			t.Fatalf("masked branch phone is missing from the page")
		}
	})

	t.Run("admin developers AI tab", func(t *testing.T) {
		values := pages.AdminDevelopersValues{
			GatewaySettings: &platformadmin.GatewaySettings{
				EndpointURL: "https://api.dawa24.com",
				APIKey:      revealGatewayUser + ":" + revealGatewayPass,
				VirtualKey:  revealPageVirtualKey,
				IsActive:    true,
			},
			GatewayAdminMasked: mask.Number(revealGatewayUser),
			CanRevealGateway:   true,
		}
		page := revealRenderHTML(t, pages.AdminDevelopersAITab(values, "ar"))
		assertRevealHidden(t, page, revealGatewayUser, revealGatewayPass, revealPageVirtualKey, revealGatewayUser+":"+revealGatewayPass)
		if !strings.Contains(page, mask.Number(revealGatewayUser)) {
			t.Fatalf("masked gateway username is missing from the page")
		}
	})
}

func TestRevealEveryGrantAuditsOnce(t *testing.T) {
	f := newRevealFixture(t)
	const grants = 3
	for i := 0; i < grants; i++ {
		rec := revealPost(t, f, revealStaff(), "org.tax_number", "3")
		if rec.Code != http.StatusOK {
			t.Fatalf("reveal %d: status = %d, want 200", i, rec.Code)
		}
	}
	rows := f.audit.rows()
	if len(rows) != grants {
		t.Fatalf("audit rows = %d for %d reveals, want one row each", len(rows), grants)
	}
	for i, row := range rows {
		if row.Action != "reveal.grant" {
			t.Fatalf("row %d action = %s, want reveal.grant", i, row.Action)
		}
	}
}

// The supplier order list and the dashboard order preview show the buyer branch
// phone masked with the reveal control (owner decision 2026-11-08). The delivery
// screens keep the clear number so a driver can call.
func TestRevealSupplierListsHideBranchPhone(t *testing.T) {
	sh := &commerce.OrderShipment{
		ID: 701, OrderID: revealOrderID, OrganizationID: revealOrgID,
		ShipmentNumber: "SH-701", CustomerBranchPhone: revealBranchPhone,
	}
	pagesToCheck := map[string]templ.Component{
		"order list card": pages.VendorOrderShipmentCard(maskedShipment(sh),
			pages.VendorOrdersData{CanReveal: true}, "ar"),
		"dashboard preview": pages.VendorDashboardShipmentModals(
			pages.VendorDashboardData{Shipments: []*commerce.OrderShipment{maskedShipment(sh)}, CanReveal: true}, "ar", "rtl"),
	}
	for name, c := range pagesToCheck {
		t.Run(name, func(t *testing.T) {
			page := revealRenderHTML(t, c)
			assertRevealHidden(t, page, revealBranchPhone)
			if !strings.Contains(page, mask.Phone(revealBranchPhone)) {
				t.Fatalf("masked branch phone is missing")
			}
			if !strings.Contains(page, `hx-post="/reveal"`) {
				t.Fatalf("no reveal control")
			}
		})
	}
}
