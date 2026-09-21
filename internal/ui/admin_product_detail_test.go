package ui_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestAdminProductDetailPage_EnlargedImageAndNoOldPrice(t *testing.T) {
	prod := &catalog.Product{
		ID:                     369372,
		Name:                   i18n.Text{"ar": "كونجستال أقراص للبرد والاحتقان", "en": "Congestal Tablets"},
		SKU:                    "CONG-650",
		Barcode:                "6221234567890",
		Price:                  money.MustParse("35.00"),
		OldPrice:               money.MustParse("40.00"), // old price data in DB
		Discount:               money.MustParse("5.00"),
		Image:                  "/uploads/products/congestal.png",
		Status:                 catalog.StatusActive,
		SoldTimes:              120,
		ManufacturingCompanies: "Sigma Pharmaceuticals",
		DosageForm:             "أقراص",
		ScientificName:         "Paracetamol + Pseudoephedrine",
	}

	variants := []*catalog.ProductVariant{
		{
			ID:             1,
			ProductID:      369372,
			OrganizationID: 10,
			Name:           i18n.Text{"ar": "عرض توريد صيدليات القاهرة"},
			Price:          money.MustParse("31.50"),
			Discount:       money.MustParse("3.50"),
			StockQty:       50,
			Status:         "active",
		},
	}

	orgNames := map[int64]string{
		10: "مستودع المتحدة للأدوية",
	}
	branchNames := map[int64]string{
		1: "فرع العبور",
	}
	warehouseNames := map[int64]string{
		1: "مخزن العبور المركزي (50)",
	}

	var buf bytes.Buffer
	err := pages.AdminProductDetailPage(prod, variants, orgNames, branchNames, warehouseNames, "ar", "rtl").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render AdminProductDetailPage: %v", err)
	}

	html := buf.String()

	// 1. "السعر القديم" MUST NOT appear anywhere on the product detail page
	if strings.Contains(html, "السعر القديم") {
		t.Errorf("expected 'السعر القديم' to be removed from product detail page, but found it in HTML")
	}

	// 2. Product image container MUST be a prominent full-height showcase (min-h-[340px] md:min-h-[380px])
	if !strings.Contains(html, "min-h-[340px] md:min-h-[380px]") {
		t.Errorf("expected product image container to have full-height showcase dimensions (min-h-[340px] md:min-h-[380px])")
	}

	// 3. Official Public Price MUST be rendered
	if !strings.Contains(html, "سعر الجمهور الرسمي") || !strings.Contains(html, "35.00") {
		t.Errorf("expected official public price 35.00 with label 'سعر الجمهور الرسمي'")
	}

	// 4. Clean stat labels for active offers and sales count
	if !strings.Contains(html, "عروض التوريد النشطة") {
		t.Errorf("expected stat card 'عروض التوريد النشطة'")
	}
	if !strings.Contains(html, "مرات الطلب والبيع") {
		t.Errorf("expected stat card 'مرات الطلب والبيع'")
	}

	// 5. Vendor offers table MUST show real organization name, warehouse name, price, and stock qty
	if !strings.Contains(html, "مستودع المتحدة للأدوية") {
		t.Errorf("expected organization real name 'مستودع المتحدة للأدوية' in vendor offers table")
	}
	if !strings.Contains(html, "مخزن العبور المركزي") {
		t.Errorf("expected warehouse name 'مخزن العبور المركزي' in vendor offers table")
	}
	if !strings.Contains(html, "50 وحدة") {
		t.Errorf("expected stock quantity '50 وحدة' in vendor offers table")
	}
	if !strings.Contains(html, "31.50") {
		t.Errorf("expected price '31.50' in vendor offers table")
	}
}

func TestAdminProductDetailPage_ImageLinkFallback(t *testing.T) {
	prod := &catalog.Product{
		ID:        102674,
		Name:      i18n.Text{"ar": "يوفي برو اويل كنترول", "en": "Uvepro Oil Control"},
		SKU:       "6224003890152",
		Barcode:   "6224003890152",
		Price:     money.MustParse("289.00"),
		Image:     "", // Empty Image
		ImageLink: "uploads/products/uvepro.jpg", // Relative ImageLink without leading slash
		Status:    catalog.StatusActive,
	}

	var buf bytes.Buffer
	err := pages.AdminProductDetailPage(prod, nil, nil, nil, nil, "ar", "rtl").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render AdminProductDetailPage: %v", err)
	}

	html := buf.String()

	// Must resolve ImageLink and normalize to /uploads/products/uvepro.jpg
	if !strings.Contains(html, "/uploads/products/uvepro.jpg") {
		t.Errorf("expected normalized /uploads/products/uvepro.jpg in HTML, got: %s", html)
	}

	// Must contain fallback container with class product-img-fallback
	if !strings.Contains(html, "product-img-fallback") {
		t.Errorf("expected product-img-fallback container in HTML")
	}
}
