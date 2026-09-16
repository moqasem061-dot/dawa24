package pages

import (
	"fmt"
	"net/url"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
)

// MarketBenchmarkPageData contains data for comparing list prices against the broader market.
type MarketBenchmarkPageData struct {
	Result *compare.BenchmarkResult
	// PagedRows is the current page slice of Result.Rows for display.
	PagedRows []*compare.BenchmarkRow
	// Failed separates a broken query from an empty market. Both used to render
	// as "لا توجد أصناف مطابقة", which is the single most misleading thing this
	// screen could say.
	Failed      bool
	Files       []*compare.CompareFile
	FileID      int64
	Query       string
	MinPrice    string
	MaxPrice    string
	MinDiscount string
	MaxDiscount string
	// ActiveTab is one of "all", "higher", "equal", "better", "exclusive".
	ActiveTab string
	// Sort is "", "discount" or "price".
	Sort       string
	IsCustomer    bool
	IsAdmin       bool
	TargetOrgID   int64
	TargetOrgName string
	Page          int
	Limit         int
	TotalCount    int
	TotalPages    int
}

func (d MarketBenchmarkPageData) FileCenterURL() string {
	if d.IsAdmin && d.TargetOrgID > 0 {
		return fmt.Sprintf("/admin/organizations/import/%d/compare", d.TargetOrgID)
	}
	return "/compare/tool"
}

func (d MarketBenchmarkPageData) SubtabURL(base string) string {
	vals := url.Values{}
	if d.IsAdmin && d.TargetOrgID > 0 {
		vals.Set("org_id", fmt.Sprintf("%d", d.TargetOrgID))
	}
	encoded := vals.Encode()
	if encoded == "" {
		return base
	}
	return base + "?" + encoded
}

func benchmarkURL(d MarketBenchmarkPageData, tab, sort string) string {
	vals := url.Values{}
	if d.FileID > 0 {
		vals.Set("file", fmt.Sprintf("%d", d.FileID))
	}
	if d.Query != "" {
		vals.Set("q", d.Query)
	}
	if d.MinPrice != "" {
		vals.Set("min_price", d.MinPrice)
	}
	if d.MaxPrice != "" {
		vals.Set("max_price", d.MaxPrice)
	}
	if d.MinDiscount != "" {
		vals.Set("min_discount", d.MinDiscount)
	}
	if d.MaxDiscount != "" {
		vals.Set("max_discount", d.MaxDiscount)
	}
	if tab != "" && tab != "all" {
		vals.Set("tab", tab)
	}
	if sort != "" {
		vals.Set("sort", sort)
	}
	if d.Limit > 0 {
		vals.Set("limit", fmt.Sprintf("%d", d.Limit))
	}
	if d.IsAdmin && d.TargetOrgID > 0 {
		vals.Set("org_id", fmt.Sprintf("%d", d.TargetOrgID))
	}
	if len(vals) == 0 {
		return "/compare/market-benchmark"
	}
	return "/compare/market-benchmark?" + vals.Encode()
}

func (d MarketBenchmarkPageData) ClearURL() string {
	vals := url.Values{}
	if d.FileID > 0 {
		vals.Set("file", fmt.Sprintf("%d", d.FileID))
	}
	if d.IsAdmin && d.TargetOrgID > 0 {
		vals.Set("org_id", fmt.Sprintf("%d", d.TargetOrgID))
	}
	if d.Limit > 0 && d.Limit != 25 {
		vals.Set("limit", fmt.Sprintf("%d", d.Limit))
	}
	if len(vals) == 0 {
		return "/compare/market-benchmark"
	}
	return "/compare/market-benchmark?" + vals.Encode()
}

func benchmarkPaginationQueryValues(d MarketBenchmarkPageData) url.Values {
	vals := url.Values{}
	if d.FileID > 0 {
		vals.Set("file", fmt.Sprintf("%d", d.FileID))
	}
	if d.Query != "" {
		vals.Set("q", d.Query)
	}
	if d.MinPrice != "" {
		vals.Set("min_price", d.MinPrice)
	}
	if d.MaxPrice != "" {
		vals.Set("max_price", d.MaxPrice)
	}
	if d.MinDiscount != "" {
		vals.Set("min_discount", d.MinDiscount)
	}
	if d.MaxDiscount != "" {
		vals.Set("max_discount", d.MaxDiscount)
	}
	if d.ActiveTab != "" && d.ActiveTab != "all" {
		vals.Set("tab", d.ActiveTab)
	}
	if d.Sort != "" {
		vals.Set("sort", d.Sort)
	}
	if d.Limit > 0 {
		vals.Set("limit", fmt.Sprintf("%d", d.Limit))
	}
	if d.IsAdmin && d.TargetOrgID > 0 {
		vals.Set("org_id", fmt.Sprintf("%d", d.TargetOrgID))
	}
	return vals
}

func benchmarkTabClass(active, tab string) string {
	if active == tab || (active == "" && tab == "all") {
		return "tab-btn active"
	}
	return "tab-btn"
}

func benchmarkRowTone(class string) string {
	switch class {
	case compare.BenchHigher:
		return "badge-rose"
	case compare.BenchBetter:
		return "badge-emerald"
	case compare.BenchEqual:
		return "badge-slate"
	}
	return "badge-amber"
}

func benchmarkRowLabel(class string) string {
	switch class {
	case compare.BenchHigher:
		return "سعرك أعلى"
	case compare.BenchBetter:
		return "أنت الأفضل"
	case compare.BenchEqual:
		return "مماثل للسوق"
	}
	return "حصري"
}

// formatBenchPercent strips unnecessary trailing decimal zeros so a clean
// column of discounts reads as 20% and 12.5%, not 20.0% and 12.5%.
func formatBenchPercent(pct float64) string {
	if pct == float64(int64(pct)) {
		return fmt.Sprintf("%d", int64(pct))
	}
	return fmt.Sprintf("%.1f", pct)
}

// benchmarkVerdictTone colours the one-word standing on the product name.
func benchmarkVerdictTone(class string) string {
	switch class {
	case compare.BenchHigher:
		return "is-worse"
	case compare.BenchBetter:
		return "is-better"
	case compare.BenchEqual:
		return "is-equal"
	}
	return "is-exclusive"
}
