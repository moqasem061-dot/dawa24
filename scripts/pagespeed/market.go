package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/ui/layouts"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// marketOffers is the fixture board. By default it is three offers, which the
// layout checks (market.test.cjs) read. With fixture=many it is seventy, for
// the paging checks (scripts/test-market-discounts.cjs): the discounts are
// spread so that the board's highest discount is not on the first page.
func marketOffers(many bool) []*compare.MarketDiscountRow {
	id := int64(7)
	first := &compare.MarketDiscountRow{ID: 1, ProductName: "اسم المورد في الملف — بانادول ٥٠٠", CatalogNameAR: "بانادول 500 مجم أقراص", CatalogNameEN: "Panadol 500 mg tablets", SupplierName: "مستودع الاختبار", OriginalPrice: money.MustParse("123.45"), PriceAfterDiscount: money.MustParse("111.10"), DiscountPercent: 10, InCatalog: true, MatchedProductID: &id, UploadedAt: time.Now()}
	second := &compare.MarketDiscountRow{ID: 2, ProductName: "منتج لم تتأكد مطابقته", SupplierName: "مستودع آخر", OriginalPrice: money.MustParse("100.00"), PriceAfterDiscount: money.MustParse("80.00"), DiscountPercent: 20, UploadedAt: time.Now()}
	third := *first
	third.ID = 3
	third.SupplierName = "مستودع العرض الثاني"
	third.ProductName = "Panadol supplier spelling"
	third.OriginalPrice = money.MustParse("150.00")
	third.DiscountPercent = 20
	third.PriceAfterDiscount = money.MustParse("120.00")
	items := []*compare.MarketDiscountRow{first, second, &third}
	if !many {
		return items
	}
	for i := int64(4); i <= 70; i++ {
		items = append(items, &compare.MarketDiscountRow{
			ID: i, ProductName: fmt.Sprintf("صنف رقم %d", i), SupplierName: "مستودع الاختبار",
			OriginalPrice: money.FromMajor(10 + i), PriceAfterDiscount: money.FromMajor(9 + i),
			DiscountPercent: float64((i * 7) % 60), UploadedAt: time.Now(),
		})
	}
	return items
}

func marketFixture(w http.ResponseWriter, r *http.Request) {
	lang, dir := "ar", "rtl"
	if r.URL.Query().Get("lang") == "en" {
		lang, dir = "en", "ltr"
	}
	actor := authctx.Actor{UserID: 1, OrganizationID: 10, IsStaff: true, Role: "super_admin"}
	qs := r.URL.Query()
	limit := 24
	if l, err := strconv.Atoi(qs.Get("limit")); err == nil && (l == 24 || l == 48 || l == 96) {
		limit = l
	}
	page, _ := strconv.Atoi(qs.Get("page"))
	if page < 1 {
		page = 1
	}
	filter := compare.MarketDiscountsFilter{Query: strings.TrimSpace(qs.Get("q")), SortBy: "newest", Page: page, Limit: limit}
	if v := qs.Get("sort"); v != "" {
		filter.SortBy = v
	}
	if id, err := strconv.ParseInt(qs.Get("product"), 10, 64); err == nil && id > 0 {
		filter.ProductID = &id
	}
	view := qs.Get("view")
	if view != "grid" {
		view = "list"
	}

	// The fixture's dataset sticks in a cookie: the pager's links carry only the
	// parameters the real board knows, so a query-string switch would be lost on
	// the first page change.
	many := qs.Get("fixture") == "many"
	if qs.Has("fixture") {
		value := "small"
		if many {
			value = "many"
		}
		http.SetCookie(w, &http.Cookie{Name: "md_fixture", Value: value, Path: "/"})
	} else if c, err := r.Cookie("md_fixture"); err == nil {
		many = c.Value == "many"
	}
	if v, err := strconv.ParseFloat(qs.Get("min_discount"), 64); err == nil {
		filter.MinDiscount = &v
	}

	var matched []*compare.MarketDiscountRow
	var highest float64
	for _, item := range marketOffers(many) {
		if filter.MinDiscount != nil && item.DiscountPercent < *filter.MinDiscount {
			continue
		}
		if filter.ProductID != nil && (item.MatchedProductID == nil || *item.MatchedProductID != *filter.ProductID) {
			continue
		}
		if filter.Query != "" && !strings.Contains(strings.ToLower(item.ProductName+" "+item.CatalogNameEN+" "+item.CatalogNameAR), strings.ToLower(filter.Query)) {
			continue
		}
		matched = append(matched, item)
		if item.DiscountPercent > highest {
			highest = item.DiscountPercent
		}
	}
	from, to := (page-1)*limit, page*limit
	if from > len(matched) {
		from = len(matched)
	}
	if to > len(matched) {
		to = len(matched)
	}
	result := &compare.MarketDiscountsResult{
		Items: matched[from:to], TotalCount: int64(len(matched)), TotalKnown: true, MaxDiscount: highest,
		Page: page, Limit: limit, TotalPages: (len(matched) + limit - 1) / limit,
		AvailableSuppliers: []string{"مستودع الاختبار", "مستودع آخر", "مستودع العرض الثاني"},
	}
	result.HasPrev, result.HasNext = page > 1, page < result.TotalPages
	if filter.ProductID != nil {
		result.ProductNameAR, result.ProductNameEN = "بانادول 500 مجم أقراص", "Panadol 500 mg tablets"
	}

	ctx := layouts.WithPath(authctx.WithActor(r.Context(), actor), r.URL.Path)
	w.Header().Add("Vary", "HX-Request")
	w.Header().Add("Vary", "HX-Target")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Target") == "md-results" {
		// Slow pages, for the stale-response checks.
		if d, err := strconv.Atoi(qs.Get("delay")); err == nil && d > 0 && d <= 3000 {
			time.Sleep(time.Duration(d) * time.Millisecond)
		}
		if err := pages.MarketDiscountsResults(lang, result, filter, view).Render(ctx, w); err != nil {
			log.Print(err)
		}
		return
	}
	if err := pages.MarketDiscountsPage(lang, dir, actor, result, filter, view).Render(ctx, w); err != nil {
		log.Print(err)
	}
}

// marketSuggestFixture answers the search box. A query containing "slow" waits
// 600 ms, so a test can type a second query while the first is in flight and
// check that the slow answer is dropped.
func marketSuggestFixture(w http.ResponseWriter, r *http.Request) {
	lang := "ar"
	if r.URL.Query().Get("lang") == "en" {
		lang = "en"
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if strings.Contains(q, "slow") {
		time.Sleep(600 * time.Millisecond)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if len([]rune(q)) < 2 {
		return
	}
	id := int64(7)
	items := []compare.MarketSuggestion{
		{ProductID: &id, NameAR: "بانادول 500 مجم أقراص", NameEN: "Panadol 500 mg tablets", FileName: "بانادول ٥٠٠", Offers: 2, Warehouses: 2, BestDiscount: 20, LowestPrice: money.MustParse("120.00")},
		{FileName: "منتج لم تتأكد مطابقته", Offers: 1, Warehouses: 1, BestDiscount: 20, LowestPrice: money.MustParse("80.00")},
	}
	if strings.Contains(q, "none") {
		items = nil
	}
	if err := pages.MarketSuggestions(lang, q, items, false).Render(r.Context(), w); err != nil {
		log.Print(err)
	}
}
