package pages_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

func marketBoardResult() *compare.MarketDiscountsResult {
	matched := int64(7)
	return &compare.MarketDiscountsResult{
		Items: []*compare.MarketDiscountRow{{
			ID: 1, SupplierName: "مستودع الاختبار", ProductName: "بانادول", CatalogNameAR: "بانادول 500",
			OriginalPrice: money.MustParse("100.00"), DiscountPercent: 10,
			PriceAfterDiscount: money.MustParse("90.00"), MatchedProductID: &matched, InCatalog: true,
			UploadedAt: time.Now(),
		}},
		AvailableSuppliers: []string{"مستودع الاختبار"},
		Page:               2,
		Limit:              24,
	}
}

func renderMarketBoard(t *testing.T, result *compare.MarketDiscountsResult) string {
	t.Helper()
	actor := authctx.Actor{UserID: 1, OrganizationID: 10, OrgType: "vendor"}
	var buf bytes.Buffer
	filter := compare.MarketDiscountsFilter{Page: result.Page, Limit: result.Limit}
	if err := pages.MarketDiscountsPage("ar", "rtl", actor, result, filter, "list").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render market board: %v", err)
	}
	return buf.String()
}

// With an exact total the page shows the pager and the count.
func TestMarketBoardShowsThePagerWhenTheTotalIsKnown(t *testing.T) {
	result := marketBoardResult()
	result.TotalCount = 240
	result.TotalPages = 10
	result.TotalKnown = true
	result.HasPrev = true
	result.HasNext = true

	html := renderMarketBoard(t, result)
	if !strings.Contains(html, "b2b-pagination") {
		t.Error("expected the pager when the total is known")
	}
	if strings.Contains(html, "تعذّر حساب إجمالي النتائج الآن") {
		t.Error("the degraded footer appeared even though the total was known")
	}
	if !strings.Contains(html, "240") {
		t.Error("expected the header to show the exact total")
	}
}

// Without a total the rows are still the answer: the page must render the prices,
// say the count is unavailable, and still offer a way to the next page.
func TestMarketBoardStillPagesWithoutATotal(t *testing.T) {
	result := marketBoardResult()
	result.TotalKnown = false
	result.HasPrev = true
	result.HasNext = true

	html := renderMarketBoard(t, result)
	if !strings.Contains(html, "90.00") {
		t.Error("the price of the only row is missing")
	}
	if strings.Contains(html, "b2b-pagination") {
		t.Error("the pager needs a total; it must not be drawn without one")
	}
	if !strings.Contains(html, "تعذّر حساب إجمالي النتائج الآن") {
		t.Error("expected the footer to say the total is unavailable")
	}
	if !strings.Contains(html, "page=3") {
		t.Error("expected a next-page link built from the current filter")
	}
	if !strings.Contains(html, "page=1") {
		t.Error("expected a previous-page link built from the current filter")
	}
}
