package ui

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestOrderLine_OfferDetailsButtonAndClickableProduct(t *testing.T) {
	prodID := int64(101)
	offerProductID := int64(202)

	order := &commerce.Order{
		ID:          50,
		OrderNumber: "ORD-2026-0050",
	}

	lineWithOffer := &commerce.OrderLine{
		ID:             1,
		OrderID:        50,
		ProductID:      &prodID,
		ProductName:    i18n.Text{"ar": "عرض بنادول إكسترا الترويجي", "en": "Panadol Extra Promo Offer"},
		OfferProductID: &offerProductID,
		UnitPrice:      money.MustParse("90.00"),
		Quantity:       3,
		DiscountAmount: money.MustParse("30.00"),
		TotalPrice:     money.MustParse("270.00"),
		ListPrice:      money.MustParse("100.00"),
	}

	lineWithoutOffer := &commerce.OrderLine{
		ID:          2,
		OrderID:     50,
		ProductID:   &prodID,
		ProductName: i18n.Text{"ar": "أوجمنتين 1 جم", "en": "Augmentin 1g"},
		UnitPrice:   money.MustParse("120.00"),
		Quantity:    2,
		TotalPrice:  money.MustParse("240.00"),
		ListPrice:   money.MustParse("120.00"),
	}

	// 1. Render EditableOrderLineRow with offer
	var bufWithOffer bytes.Buffer
	err := pages.EditableOrderLineRow(order, lineWithOffer, "ar", 0).Render(context.Background(), &bufWithOffer)
	if err != nil {
		t.Fatalf("failed to render EditableOrderLineRow with offer: %v", err)
	}
	htmlOffer := bufWithOffer.String()

	// Must contain clearly labelled button at normal size (btn-sm, not btn-xs)
	if !strings.Contains(htmlOffer, "عرض تفاصيل العرض") {
		t.Errorf("expected 'عرض تفاصيل العرض' button for line with offer")
	}
	if !strings.Contains(htmlOffer, "btn btn-secondary btn-sm") {
		t.Errorf("expected 'btn btn-secondary btn-sm' classes on offer button")
	}
	if !strings.Contains(htmlOffer, fmt.Sprintf("/customer/orders/%d/lines/%d/offer-details", order.ID, lineWithOffer.ID)) {
		t.Errorf("expected hx-get target URL for offer details on editable row")
	}
	// Product name must be a clickable link to catalog
	expectedLink := fmt.Sprintf("/customer/catalog/%d", prodID)
	if !strings.Contains(htmlOffer, expectedLink) {
		t.Errorf("expected clickable link to %s for product name", expectedLink)
	}
	// Noisy description line under product name must be removed
	if strings.Contains(htmlOffer, "باقة / عرض ترويجي (معاينة الأصناف)") {
		t.Errorf("expected noisy description badge 'باقة / عرض ترويجي (معاينة الأصناف)' to be removed")
	}

	// 2. Render EditableOrderLineRow without offer
	var bufWithoutOffer bytes.Buffer
	err = pages.EditableOrderLineRow(order, lineWithoutOffer, "ar", 1).Render(context.Background(), &bufWithoutOffer)
	if err != nil {
		t.Fatalf("failed to render EditableOrderLineRow without offer: %v", err)
	}
	htmlNoOffer := bufWithoutOffer.String()

	// Must NOT contain offer details button
	if strings.Contains(htmlNoOffer, "عرض تفاصيل العرض") {
		t.Errorf("expected NO 'عرض تفاصيل العرض' button for line without offer")
	}

	// 3. Render ReadOnlyOrderLineRow with offer
	var bufReadOnlyOffer bytes.Buffer
	err = pages.ReadOnlyOrderLineRow(order, lineWithOffer, "ar", 0).Render(context.Background(), &bufReadOnlyOffer)
	if err != nil {
		t.Fatalf("failed to render ReadOnlyOrderLineRow with offer: %v", err)
	}
	htmlReadOnly := bufReadOnlyOffer.String()

	if !strings.Contains(htmlReadOnly, "عرض تفاصيل العرض") {
		t.Errorf("expected 'عرض تفاصيل العرض' button in ReadOnlyOrderLineRow")
	}
	if !strings.Contains(htmlReadOnly, expectedLink) {
		t.Errorf("expected clickable link to %s in ReadOnlyOrderLineRow", expectedLink)
	}
	if strings.Contains(htmlReadOnly, "باقة / عرض ترويجي (معاينة الأصناف)") {
		t.Errorf("expected noisy badge to be removed from ReadOnlyOrderLineRow")
	}
}

func TestCustomerOrderOfferModal_FinancialReconciliationAndEvidence(t *testing.T) {
	now := time.Now()
	startsAt := now.Add(-5 * 24 * time.Hour)
	expiresAt := now.Add(25 * 24 * time.Hour)

	details := &commerce.OrderLineOfferDetails{
		OfferID:        15,
		LineID:         100,
		Title:          i18n.Text{"ar": "عرض الشتاء الترويجي للأدوية", "en": "Winter Promo Offer"},
		Description:    i18n.Text{"ar": "عرض خاص لخصومات أدوية البرد والمسكنات", "en": "Special seasonal discount"},
		VendorName:     "شركة ابن سينا فارما",
		DiscountType:   "percentage",
		DiscountValue:  money.MustParse("10.00"),
		ListPrice:      money.MustParse("200.00"),
		UnitPrice:      money.MustParse("180.00"),
		Quantity:       5,
		DiscountAmount: money.MustParse("100.00"),
		TotalPrice:     money.MustParse("900.00"),
		StartsAt:       &startsAt,
		ExpiresAt:      &expiresAt,
		Items: []commerce.OrderLineOfferItem{
			{
				ProductID:             501,
				ProductName:           i18n.Text{"ar": "بنادول كولد أند فلو", "en": "Panadol Cold & Flu"},
				Quantity:              2,
				CustomPrice:           money.MustParse("45.00"),
				CustomDiscountPercent: 10.0,
			},
			{
				ProductID:             502,
				ProductName:           i18n.Text{"ar": "كونجستال أقراص", "en": "Congestal Tablets"},
				Quantity:              3,
				CustomPrice:           money.MustParse("30.00"),
				CustomDiscountPercent: 10.0,
			},
		},
	}

	var buf bytes.Buffer
	err := pages.CustomerOrderOfferModal(details, "ar").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render CustomerOrderOfferModal: %v", err)
	}

	html := buf.String()

	// 1. Offer title & Vendor name
	if !strings.Contains(html, "عرض الشتاء الترويجي للأدوية") {
		t.Errorf("expected offer title in modal")
	}
	if !strings.Contains(html, "المورد: شركة ابن سينا فارما") {
		t.Errorf("expected vendor name in modal")
	}

	// 2. Offer mechanics
	if !strings.Contains(html, "باقة ترويجية مجمعة تشمل 2 أصناف") {
		t.Errorf("expected mechanics description in modal")
	}

	// 3. Validity window
	expectedValidity := fmt.Sprintf("من %s إلى %s", startsAt.Format("2006-01-02"), expiresAt.Format("2006-01-02"))
	if !strings.Contains(html, expectedValidity) {
		t.Errorf("expected validity window '%s' in modal", expectedValidity)
	}

	// 4. Financial reconciliation numbers: List price, Discount applied, Resulting unit price, Qty, Total
	if !strings.Contains(html, "200.00") {
		t.Errorf("expected list price 200.00 in modal")
	}
	if !strings.Contains(html, "100.00") {
		t.Errorf("expected discount amount 100.00 in modal")
	}
	if !strings.Contains(html, "180.00") {
		t.Errorf("expected resulting unit price 180.00 in modal")
	}
	if !strings.Contains(html, "5") {
		t.Errorf("expected quantity 5 in modal")
	}
	if !strings.Contains(html, "900.00") {
		t.Errorf("expected line total 900.00 in modal")
	}

	// 5. Bundled items manifest
	if !strings.Contains(html, "بنادول كولد أند فلو") || !strings.Contains(html, "كونجستال أقراص") {
		t.Errorf("expected bundled items names in manifest table")
	}
}

func TestVendorOrderShipmentCard_OfferDetailsAction(t *testing.T) {
	offerProductID := int64(88)

	sh := &commerce.OrderShipment{
		ID:             300,
		OrderID:        77,
		ShipmentNumber: "SH-0077-1",
		Status:         commerce.StatusConfirmed,
		TotalAmount:    money.MustParse("500.00"),
		Lines: []*commerce.OrderLine{
			{
				ID:             10,
				OrderID:        77,
				ProductName:    i18n.Text{"ar": "عرض باقة المضادات الحيوية", "en": "Antibiotic Bundle Offer"},
				OfferProductID: &offerProductID,
				Quantity:       2,
				UnitPrice:      money.MustParse("250.00"),
				TotalPrice:     money.MustParse("500.00"),
			},
			{
				ID:          11,
				OrderID:     77,
				ProductName: i18n.Text{"ar": "أمبيسيلين 500 مجم", "en": "Ampicillin 500mg"},
				Quantity:    5,
				UnitPrice:   money.MustParse("20.00"),
				TotalPrice:  money.MustParse("100.00"),
			},
		},
	}

	data := pages.VendorOrdersData{
		Shipments: []*commerce.OrderShipment{sh},
	}

	var buf bytes.Buffer
	err := pages.VendorOrderShipmentCard(sh, data, "ar").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render VendorOrderShipmentCard: %v", err)
	}

	html := buf.String()

	// Line 10 (with offer) must have "عرض تفاصيل العرض" button
	expectedGet := fmt.Sprintf("/vendor/orders/%d/lines/%d/offer-details", sh.OrderID, 10)
	if !strings.Contains(html, "عرض تفاصيل العرض") {
		t.Errorf("expected 'عرض تفاصيل العرض' button on vendor shipment line with offer")
	}
	if !strings.Contains(html, expectedGet) {
		t.Errorf("expected hx-get attribute '%s' on vendor offer button", expectedGet)
	}

	// Line 11 (without offer) must NOT have a separate offer details button
	countOfferButtons := strings.Count(html, "عرض تفاصيل العرض")
	if countOfferButtons != 1 {
		t.Errorf("expected exactly 1 offer button for line 10, got %d", countOfferButtons)
	}
}

func TestOrderOfferHandlers_AuthenticationEnforced(t *testing.T) {
	h := &UIHandler{}

	// 1. Unauthenticated customer request
	req1 := httptest.NewRequest("GET", "/customer/orders/1/lines/2/offer-details", nil)
	rr1 := httptest.NewRecorder()
	h.CustomerOrderLineOfferDetails(rr1, req1)
	if rr1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for unauthenticated customer, got %d", rr1.Code)
	}

	// 2. Unauthenticated vendor request
	req2 := httptest.NewRequest("GET", "/vendor/orders/1/lines/2/offer-details", nil)
	rr2 := httptest.NewRecorder()
	h.VendorOrderLineOfferDetails(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for unauthenticated vendor, got %d", rr2.Code)
	}

	// 3. Authenticated customer request with invalid params
	actor := authctx.Actor{
		UserID:         10,
		OrganizationID: 20,
		OrgType:        "customer",
	}
	req3 := httptest.NewRequest("GET", "/customer/orders/0/lines/0/offer-details", nil)
	req3 = req3.WithContext(authctx.WithActor(req3.Context(), actor))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "0")
	rctx.URLParams.Add("lineID", "0")
	req3 = req3.WithContext(context.WithValue(req3.Context(), chi.RouteCtxKey, rctx))

	rr3 := httptest.NewRecorder()
	h.CustomerOrderLineOfferDetails(rr3, req3)
	if rr3.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for zero IDs, got %d", rr3.Code)
	}
}
