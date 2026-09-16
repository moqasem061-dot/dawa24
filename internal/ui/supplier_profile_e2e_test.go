package ui_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// TestSupplierProfile_RenderingE2E verifies that SupplierProfile template renders stock badge and add to cart form.
func TestSupplierProfile_RenderingE2E(t *testing.T) {
	exp := time.Now().Add(365 * 24 * time.Hour)
	v := &catalog.ProductVariant{
		ID:          501,
		ProductID:   200,
		Name:        i18n.Text{"ar": "كونجستال 20 قرص", "en": "Congestal 20 Tabs"},
		Price:       money.FromMajor(35),
		StockQty:    75,
		MinOrderQty: 2,
		BatchNumber: "BCH-2026-99",
		ExpiryDate:  &exp,
		Unit:        "علبة",
	}

	p := &catalog.Product{
		ID:             200,
		Name:           i18n.Text{"ar": "كونجستال أقراص", "en": "Congestal Tabs"},
		Price:          money.FromMajor(45),
		DosageForm:     "أقراص",
		ScientificName: "Paracetamol 500mg",
	}

	data := pages.SupplierProfileData{
		Org: &org.Organization{
			ID:        10,
			LegalName: "Al-Ahram Pharma Distribution",
			TradeName: i18n.Text{"ar": "مؤسسة الأهرام للتوزيع الدوائي"},
			Status:    org.StatusApproved,
		},
		Variants:    []*catalog.ProductVariant{v},
		ProductsMap: map[int64]*catalog.Product{200: p},
		VariantMeta: map[int64]pages.SupplierVariantMeta{
			501: {
				AvailableStock: 75,
				MinOrderQty:    2,
				IsCovered:      true,
				CanAddToCart:   true,
			},
		},
		CurrentPage: 1,
		TotalPages:  1,
	}

	var buf bytes.Buffer
	err := pages.SupplierProfile("ar", "rtl", data).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render SupplierProfile: %v", err)
	}

	html := buf.String()

	// Verify product name and details
	if !strings.Contains(html, "كونجستال 20 قرص") {
		t.Errorf("rendered HTML missing product variant name")
	}
	if !strings.Contains(html, "BCH-2026-99") {
		t.Errorf("rendered HTML missing batch number")
	}

	// Verify in-stock badge
	if !strings.Contains(html, "متوفر (75 عبوة)") {
		t.Errorf("rendered HTML missing available stock badge (75 عبوة)")
	}

	// Verify Add to Cart form attributes: action, variant_id, min, max
	if !strings.Contains(html, `action="/cart/add"`) {
		t.Errorf("rendered HTML missing cart add form action")
	}
	if !strings.Contains(html, `value="501"`) {
		t.Errorf("rendered HTML missing variant ID input")
	}
	if !strings.Contains(html, `max="75"`) {
		t.Errorf("rendered HTML missing max stock limit constraint on quantity input")
	}
	if !strings.Contains(html, `+ إضافة للسلة`) {
		t.Errorf("rendered HTML missing + إضافة للسلة button")
	}
}

// TestAddToCartSubmit_HTMX_And_Persistence tests the AddToCartSubmit handler with HTMX.
func TestAddToCartSubmit_HTMX_And_Persistence(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	commRepo := newMockCommerceRepo()
	commSvc := commerce.NewService(commRepo, log)

	commSvc.SetAvailabilityProbe(&mockAvailabilityProbe{availQty: 100})

	handler := ui.NewUIHandler(
		nil, nil, nil, commSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, log,
	)

	branchID := int64(123)
	// Create customer actor context
	customerActor := authctx.Actor{
		UserID:         42,
		Email:          "pharmacy@example.com",
		Role:           "customer",
		OrgType:        "customer",
		OrganizationID: 99,
		BranchID:       &branchID,
	}
	ctx := authctx.WithActor(context.Background(), customerActor)

	// Prepare HTMX POST request
	form := url.Values{}
	form.Set("variant_id", "501")
	form.Set("product_id", "200")
	form.Set("vendor_org_id", "10")
	form.Set("quantity", "4")
	form.Set("return_to", "/suppliers/10")

	req := httptest.NewRequest(http.MethodPost, "/cart/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.AddToCartSubmit(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK for HTMX add to cart, got %d", res.StatusCode)
	}

	// Verify HX-Trigger header contains showToast and cartUpdated
	hxTrigger := res.Header.Get("HX-Trigger")
	if !strings.Contains(hxTrigger, "showToast") || !strings.Contains(hxTrigger, "cartUpdated") {
		t.Errorf("expected HX-Trigger to contain showToast and cartUpdated, got: %s", hxTrigger)
	}

	// Verify item was saved to database cart
	cart, err := commSvc.GetCart(ctx, 42, 0)
	if err != nil {
		t.Fatalf("failed to retrieve cart: %v", err)
	}
	if cart == nil || len(cart.Items) == 0 {
		t.Fatalf("cart is empty in database, item was not persisted")
	}
	if cart.Items[0].ProductVariantID != 501 || cart.Items[0].Quantity != 4 {
		t.Errorf("expected variant 501 with qty 4, got variant %d qty %d", cart.Items[0].ProductVariantID, cart.Items[0].Quantity)
	}
}
