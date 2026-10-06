package main

import (
	"log"
	"net/http"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/ui/layouts"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

func marketFixture(w http.ResponseWriter, r *http.Request) {
	lang, dir := "ar", "rtl"
	if r.URL.Query().Get("lang") == "en" {
		lang, dir = "en", "ltr"
	}
	actor := authctx.Actor{UserID: 1, OrganizationID: 10, IsStaff: true, Role: "super_admin"}
	id := int64(7)
	result := &compare.MarketDiscountsResult{Items: []*compare.MarketDiscountRow{{ID: 1, ProductName: "اسم المورد في الملف — بانادول ٥٠٠", CatalogNameAR: "بانادول 500 مجم أقراص", CatalogNameEN: "Panadol 500 mg tablets", SupplierName: "مستودع الاختبار", OriginalPrice: money.MustParse("123.45"), PriceAfterDiscount: money.MustParse("111.10"), DiscountPercent: 10, InCatalog: true, MatchedProductID: &id, UploadedAt: time.Now()}, {ID: 2, ProductName: "منتج لم تتأكد مطابقته", SupplierName: "مستودع آخر", OriginalPrice: money.MustParse("100.00"), PriceAfterDiscount: money.MustParse("80.00"), DiscountPercent: 20, UploadedAt: time.Now()}}, TotalCount: 2, Page: 1, Limit: 24, TotalPages: 1}
	result.AvailableSuppliers = []string{"مستودع الاختبار", "مستودع آخر", "مستودع العرض الثاني"}
	secondOffer := *result.Items[0]
	secondOffer.ID = 3
	secondOffer.SupplierName = "مستودع العرض الثاني"
	secondOffer.ProductName = "Panadol supplier spelling"
	secondOffer.OriginalPrice = money.MustParse("150.00")
	secondOffer.DiscountPercent = 20
	secondOffer.PriceAfterDiscount = money.MustParse("120.00")
	result.Items = append(result.Items, &secondOffer)
	result.TotalCount = int64(len(result.Items))
	ctx := layouts.WithPath(authctx.WithActor(r.Context(), actor), r.URL.Path)
	filter := compare.MarketDiscountsFilter{Query: r.URL.Query().Get("q"), SortBy: "relevance"}
	view := r.URL.Query().Get("view")
	if view != "grid" {
		view = "list"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.MarketDiscountsPage(lang, dir, actor, result, filter, view).Render(ctx, w); err != nil {
		log.Print(err)
	}
}
