package pages_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestVendorOfferLocationsPageRendering(t *testing.T) {
	cairoGovID := int64(1)
	govName := i18n.Text{"ar": "القاهرة", "en": "Cairo"}
	cityName := i18n.Text{"ar": "مدينة نصر", "en": "Nasr City"}

	data := pages.VendorOfferLocationsData{
		Offer: &promo.SpecialOffer{
			ID:             16,
			OrganizationID: 2,
			Title:          i18n.Text{"ar": "عرض الجمعة البيضاء", "en": "White Friday Offer"},
			Status:         "active",
		},
		Governorates: []*platformadmin.Governorate{
			{ID: cairoGovID, Name: govName, Latitude: 30.0444, Longitude: 31.2357},
		},
		Cities: []*platformadmin.City{
			{
				ID:                   101,
				GovernorateID:        &cairoGovID,
				Name:                 cityName,
				Latitude:             30.0561,
				Longitude:            31.3301,
				CoverageRadiusMeters: 5000,
			},
		},
		Locations: []*promo.SpecialOfferLocation{
			{
				ID:              1,
				OfferID:         16,
				GovernorateID:   &cairoGovID,
				GovernorateName: "القاهرة",
				CityName:        "مدينة نصر",
				Radius:          5000,
				DayOfWeek:       1,
				Status:          "active",
			},
		},
	}

	var sb strings.Builder
	err := pages.VendorOfferLocationsPage(data, "ar", "rtl").Render(context.Background(), &sb)
	if err != nil {
		t.Fatalf("VendorOfferLocationsPage render error: %v", err)
	}

	html := sb.String()

	// 1. Must render cities-dataset containing JSON array of cities
	if !strings.Contains(html, `id="cities-dataset"`) {
		t.Errorf("expected HTML to contain #cities-dataset element")
	}
	if !strings.Contains(html, "مدينة نصر") {
		t.Errorf("expected HTML cities dataset to contain city name")
	}

	// 2. Must initialize Alpine with offerLocationsManager(16)
	if !strings.Contains(html, `x-data="offerLocationsManager(16)"`) {
		t.Errorf("expected x-data to be offerLocationsManager(16), got:\n%s", html)
	}

	// 3. Must contain day pill toggles
	if !strings.Contains(html, "toggleDay(") {
		t.Errorf("expected HTML to contain toggleDay handlers")
	}
	if !strings.Contains(html, "selectAllDays()") || !strings.Contains(html, "clearDays()") {
		t.Errorf("expected HTML to contain selectAllDays and clearDays controls")
	}

	// 4. Must contain governorate select and filteredCities loop
	if !strings.Contains(html, `x-model="selectedGovId"`) {
		t.Errorf("expected governorate select bound to selectedGovId")
	}
	if !strings.Contains(html, "filteredCities") {
		t.Errorf("expected template loop iterating over filteredCities")
	}

	// 5. Must NOT contain literal un-interpolated String(offerID)
	if strings.Contains(html, "String(offerID)") {
		t.Errorf("fatal: HTML contains un-interpolated String(offerID) causing ReferenceError")
	}
}

func TestInvoicePaymentModalAndTableIntegration(t *testing.T) {
	// 1. Verify AdminInvoicesTable renders safe onclick attributes
	adminData := pages.AdminFinanceData{
		Invoices: []*billing.AdminInvoiceView{
			{
				ID:              205,
				InvoiceNumber:   "INV-2026-00205",
				Status:          "issued",
				RemainingAmount: money.FromMinor(150000), // 1500.00 EGP
				DueDate:         time.Now().Add(48 * time.Hour),
				IssueDate:       time.Now(),
			},
		},
	}

	var sbTable strings.Builder
	err := pages.AdminInvoicesTable(adminData, "ar", "rtl").Render(context.Background(), &sbTable)
	if err != nil {
		t.Fatalf("AdminInvoicesTable render error: %v", err)
	}

	tableHTML := sbTable.String()

	// Must contain openRecordInvoicePaymentModal with valid JS arguments without premature quote close
	if !strings.Contains(tableHTML, "openRecordInvoicePaymentModal(205") {
		t.Errorf("expected table to contain openRecordInvoicePaymentModal(205, ...), got:\n%s", tableHTML)
	}
	if strings.Contains(tableHTML, `onclick="openRecordInvoicePaymentModal(205, "`) {
		t.Errorf("fatal: table has premature quote close inside onclick attribute!")
	}

	// 2. Verify InvoicePaymentModal renders window functions and dialog
	var sbModal strings.Builder
	err = pages.InvoicePaymentModal("/admin/finance/invoices").Render(context.Background(), &sbModal)
	if err != nil {
		t.Fatalf("InvoicePaymentModal render error: %v", err)
	}

	modalHTML := sbModal.String()
	if !strings.Contains(modalHTML, `id="record-invoice-payment-modal"`) {
		t.Errorf("expected modal to contain dialog id record-invoice-payment-modal")
	}
	if !strings.Contains(modalHTML, "window.openRecordInvoicePaymentModal = function") {
		t.Errorf("expected script to define window.openRecordInvoicePaymentModal")
	}
	if !strings.Contains(modalHTML, "window.closeRecordInvoicePaymentModal = function") {
		t.Errorf("expected script to define window.closeRecordInvoicePaymentModal")
	}
}
