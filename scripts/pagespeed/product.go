package main

import (
	"log"
	"net/http"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/ui/layouts"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

func productFixture(w http.ResponseWriter, r *http.Request) {
	lang, dir := productFixtureLocale(r)
	ctx := layouts.WithPath(r.Context(), r.URL.Path)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerProductDetail(productFixtureCatalogItem(), nil, productFixtureOffers(), lang, dir).Render(ctx, w); err != nil {
		log.Print(err)
	}
}

func productFixtureLocale(r *http.Request) (string, string) {
	if r.URL.Query().Get("lang") == "en" {
		return "en", "ltr"
	}
	return "ar", "rtl"
}

func productFixtureCatalogItem() *catalog.Product {
	return &catalog.Product{
		ID: 12345, Name: i18n.Text{"ar": "بنادول إكسترا أقراص مسكنة", "en": "Panadol Extra Tablets"},
		SKU: "PAN-EXT-500", Barcode: "6221000123456", Price: money.MustParse("45.00"),
		Image: "/static/images/product-fixture-missing.png", Status: catalog.StatusActive,
		ManufacturingCompanies: "GSK Consumer Healthcare", DosageForm: "أقراص",
		Active: "Paracetamol 500mg + Caffeine 65mg", ScientificName: "Paracetamol + Caffeine",
	}
}

func productFixtureOffers() []pages.SupplierOffer {
	return []pages.SupplierOffer{
		{VariantID: 101, SupplierID: 20, SupplierName: "شركة المتحدة للتوزيع الدوائي", Price: money.MustParse("38.00"), OldPrice: money.MustParse("45.00"), DiscountAmount: money.MustParse("7.00"), DiscountBPS: 1550, AvailableStock: 150, MinOrderQty: 5, MaxOrderQty: 80, IsCovered: true, CanAddToCart: true},
		{VariantID: 102, SupplierID: 25, SupplierName: "مستودع ابن سينا فارما", Price: money.MustParse("39.50"), OldPrice: money.MustParse("45.00"), DiscountAmount: money.MustParse("5.50"), DiscountBPS: 1220, AvailableStock: 0, MinOrderQty: 10, MaxOrderQty: 60, CanAddToCart: false},
	}
}
