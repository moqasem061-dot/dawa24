package pages_test

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

func renderResults(t *testing.T, lang string, result *compare.MarketDiscountsResult, filter compare.MarketDiscountsFilter, view string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := pages.MarketDiscountsResults(lang, result, filter, view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render results: %v", err)
	}
	return buf.String()
}

func statValue(t *testing.T, html, label string) string {
	t.Helper()
	m := regexp.MustCompile(`<dt>` + regexp.QuoteMeta(label) + `</dt>\s*<dd[^>]*>([^<]*)</dd>`).FindStringSubmatch(html)
	if m == nil {
		t.Fatalf("stat %q not found in:\n%s", label, html)
	}
	return strings.TrimSpace(m[1])
}

// "أعلى خصم" is the board's highest discount for the current filter, not the
// highest on the page on show: this page's best is 10 %, the board's is 63 %.
func TestMarketResultsShowTheBoardWideMaximumDiscount(t *testing.T) {
	result := marketBoardResult()
	result.TotalKnown = true
	result.TotalCount = 240
	result.MaxDiscount = 63

	html := renderResults(t, "ar", result, compare.MarketDiscountsFilter{Page: 2, Limit: 24}, "list")
	if got := statValue(t, html, "أعلى خصم"); got != "63%" {
		t.Errorf("highest discount = %q, want 63%% (the board's, not the page's 10%%)", got)
	}
	if got := statValue(t, html, "أصناف مرصودة"); got != "240" {
		t.Errorf("count = %q, want 240", got)
	}
}

// With no exact total there is no board-wide maximum either; the page shows a
// dash rather than the page's own best.
func TestMarketResultsShowADashWhenTheMaximumIsUnknown(t *testing.T) {
	result := marketBoardResult()
	result.TotalKnown = false
	result.MaxDiscount = 0

	html := renderResults(t, "ar", result, compare.MarketDiscountsFilter{Page: 2, Limit: 24}, "list")
	if got := statValue(t, html, "أعلى خصم"); got != "—" {
		t.Errorf("highest discount = %q, want a dash", got)
	}
}

// The product filter survives paging, and the chip that shows it links to the
// same board without it.
func TestMarketResultsCarryTheProductFilterThroughThePagerAndChips(t *testing.T) {
	result := marketBoardResult()
	result.TotalKnown = true
	result.TotalCount = 240
	result.MaxDiscount = 63
	result.ProductNameAR = "بانادول إكسترا"
	product := int64(7)
	filter := compare.MarketDiscountsFilter{ProductID: &product, SortBy: "discount_desc", Page: 2, Limit: 24}

	html := renderResults(t, "ar", result, filter, "grid")
	if !strings.Contains(html, "بانادول إكسترا") {
		t.Error("the product chip does not name the product")
	}
	pager := html[strings.Index(html, "b2b-pagination"):]
	for _, link := range regexp.MustCompile(`href="(/market-discounts\?[^"]*page=[^"]*)"`).FindAllStringSubmatch(pager, -1) {
		if !strings.Contains(link[1], "product=7") || !strings.Contains(link[1], "sort=discount_desc") || !strings.Contains(link[1], "view=grid") {
			t.Errorf("pager link %q dropped a filter", link[1])
		}
	}
	chip := regexp.MustCompile(`class="md-chip"\s+href="([^"]+)"`).FindStringSubmatch(html)
	if chip == nil {
		t.Fatalf("no chip in:\n%s", html)
	}
	if strings.Contains(chip[1], "product=") || !strings.Contains(chip[1], "sort=discount_desc") || strings.Contains(chip[1], "page=") {
		t.Errorf("the chip's link %q should drop the product and the page and keep the rest", chip[1])
	}
}

func TestMarketResultsOneArticlePerOfferAndLayoutByClass(t *testing.T) {
	result := marketBoardResult()
	result.Items = append(result.Items, &compare.MarketDiscountRow{ID: 2, SupplierName: "مخزن", ProductName: "x", OriginalPrice: money.MustParse("5.00")})
	result.TotalKnown, result.TotalCount = true, 2
	for view, class := range map[string]string{"list": "is-list", "grid": "is-grid", "": "is-list"} {
		html := renderResults(t, "ar", result, compare.MarketDiscountsFilter{Page: 1, Limit: 24}, view)
		if strings.Count(html, "<article") != 2 {
			t.Errorf("view %q: %d articles, want one per offer", view, strings.Count(html, "<article"))
		}
		if !strings.Contains(html, `class="md-offers `+class+`"`) {
			t.Errorf("view %q: want container class %s", view, class)
		}
	}
}

// The empty state is keyed to whether any filter is active.
func TestMarketResultsEmptyStateFollowsTheFilters(t *testing.T) {
	empty := &compare.MarketDiscountsResult{Items: []*compare.MarketDiscountRow{}, Page: 1, Limit: 24, TotalKnown: true}
	filtered := renderResults(t, "ar", empty, compare.MarketDiscountsFilter{Query: "غير موجود", Page: 1, Limit: 24}, "list")
	if !strings.Contains(filtered, "لم نجد أي خصومات مطابقة") || !strings.Contains(filtered, "عرض كافة خصومات السوق") {
		t.Error("a filtered empty board must say nothing matched and offer to show everything")
	}
	plain := renderResults(t, "ar", empty, compare.MarketDiscountsFilter{Page: 1, Limit: 24}, "list")
	if strings.Contains(plain, "لم نجد أي خصومات مطابقة") || !strings.Contains(plain, "لا توجد كشوف أسعار منشورة بعد") {
		t.Error("an empty board with no filter must not claim a search found nothing")
	}
	en := renderResults(t, "en", empty, compare.MarketDiscountsFilter{Query: "zzz", Page: 1, Limit: 24}, "list")
	if !strings.Contains(en, "No matching discounts found") {
		t.Error("the empty state is not translated")
	}
}

func TestMarketSuggestionsRender(t *testing.T) {
	id := int64(7)
	items := []compare.MarketSuggestion{
		{ProductID: &id, NameAR: "بانادول", NameEN: "Panadol", FileName: "panadol 500", Offers: 3, Warehouses: 2, BestDiscount: 63, LowestPrice: money.MustParse("90.00")},
		{FileName: "unmatched <b>name</b>", Offers: 1, Warehouses: 1},
	}
	var buf bytes.Buffer
	if err := pages.MarketSuggestions("ar", `pan"adol`, items, false).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{`id="md-opt-0"`, `data-md-kind="all"`, `id="md-opt-1"`, `data-md-kind="product"`, `data-md-product="7"`, `id="md-opt-2"`, `data-md-kind="text"`, "63", "90.00", "3 عرض", "2 مستودع"} {
		if !strings.Contains(html, want) {
			t.Errorf("suggestions lack %s:\n%s", want, html)
		}
	}
	if strings.Contains(html, "<b>name</b>") || strings.Contains(html, `pan"adol`) {
		t.Error("user-controlled text is not escaped")
	}

	buf.Reset()
	_ = pages.MarketSuggestions("en", "zzz", nil, false).Render(context.Background(), &buf)
	if !strings.Contains(buf.String(), `data-md-note="none"`) || !strings.Contains(buf.String(), "No matching suggestions") {
		t.Errorf("empty state: %s", buf.String())
	}
	buf.Reset()
	_ = pages.MarketSuggestions("ar", "zzz", nil, true).Render(context.Background(), &buf)
	if !strings.Contains(buf.String(), `data-md-note="error"`) || !strings.Contains(buf.String(), "تعذّر جلب الاقتراحات") {
		t.Errorf("failure state: %s", buf.String())
	}
}
