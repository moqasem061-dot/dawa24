package ui_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

func TestCustomerProductDetailPage_AmazonLayout(t *testing.T) {
	prod := &catalog.Product{
		ID:                     12345,
		Name:                   i18n.Text{"ar": "بنادول إكسترا أقراص مسكنة", "en": "Panadol Extra Tablets"},
		SKU:                    "PAN-EXT-500",
		Barcode:                "6221000123456",
		Price:                  money.MustParse("45.00"),
		Image:                  "/uploads/products/panadol.png",
		Status:                 catalog.StatusActive,
		ManufacturingCompanies: "GSK Consumer Healthcare",
		DosageForm:             "أقراص",
		Active:                 "Paracetamol 500mg + Caffeine 65mg",
		ScientificName:         "Paracetamol + Caffeine",
	}

	offers := []pages.SupplierOffer{
		{
			VariantID:      101,
			SupplierID:     20,
			SupplierName:   "شركة المتحدة للتوزيع الدوائي",
			Price:          money.MustParse("38.00"),
			OldPrice:       money.MustParse("45.00"),
			DiscountAmount: money.MustParse("7.00"),
			DiscountBPS:    1550,
			AvailableStock: 150,
			MinOrderQty:    5,
			ExpiryDate:     "2027-08",
			IsCovered:      true,
			CanAddToCart:   true,
			IsVerified:     true,
			SupplierRating: 4.9,
			ReviewCount:    32,
		},
		{
			VariantID:      102,
			SupplierID:     25,
			SupplierName:   "مستودع ابن سينا فارما",
			Price:          money.MustParse("39.50"),
			OldPrice:       money.MustParse("45.00"),
			DiscountAmount: money.MustParse("5.50"),
			DiscountBPS:    1220,
			AvailableStock: 80,
			MinOrderQty:    10,
			ExpiryDate:     "2027-05",
			IsCovered:      true,
			CanAddToCart:   true,
			IsVerified:     true,
			SupplierRating: 4.7,
			ReviewCount:    18,
		},
	}

	var buf bytes.Buffer
	err := pages.CustomerProductDetail(prod, nil, offers, "ar", "rtl").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render CustomerProductDetail: %v", err)
	}

	html := buf.String()

	// 1. Must contain Amazon-style 3-column layout grid
	if !strings.Contains(html, "grid grid-cols-1 lg:grid-cols-12") {
		t.Errorf("expected 12-column grid layout for Amazon-style showcase")
	}

	// 2. Must render product image and title
	if !strings.Contains(html, "/uploads/products/panadol.png") {
		t.Errorf("expected product image url in HTML")
	}
	if !strings.Contains(html, "بنادول إكسترا أقراص مسكنة") {
		t.Errorf("expected product title in HTML")
	}

	// 3. Must render clinical specifications
	if !strings.Contains(html, "Paracetamol 500mg + Caffeine 65mg") {
		t.Errorf("expected active ingredient in HTML")
	}
	if !strings.Contains(html, "6221000123456") {
		t.Errorf("expected barcode in HTML")
	}

	// 4. Must render Buy Box with winning offer pricing, supplier, stock, and add-to-cart button
	if !strings.Contains(html, "product-detail-buy-box") {
		t.Errorf("expected product-detail-buy-box in HTML")
	}
	if !strings.Contains(html, "38.00") {
		t.Errorf("expected winning offer price 38.00 in Buy Box")
	}
	if !strings.Contains(html, "شركة المتحدة للتوزيع الدوائي") {
		t.Errorf("expected winning supplier name in Buy Box")
	}

	// 5. Must render competing offers table
	if !strings.Contains(html, "مستودع ابن سينا فارما") {
		t.Errorf("expected competing supplier in offers table")
	}

	// 6. Muhiya design B2B showcase layout & specs table
	if !strings.Contains(html, "b2b-customer-showcase") {
		t.Errorf("expected 'b2b-customer-showcase' layout container")
	}
	if !strings.Contains(html, "b2b-product-media-card") {
		t.Errorf("expected 'b2b-product-media-card' fixed container")
	}
	if !strings.Contains(html, "b2b-specs-table") {
		t.Errorf("expected 'b2b-specs-table' clean specification table")
	}
}
