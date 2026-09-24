package pages

import (
	"fmt"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/ui/components"
)

type VendorSavingPageData struct {
	Items        []*catalog.SavingProductEnriched
	Stats        *catalog.SavingProductStats
	SearchQuery  string
	FilterStatus string
	NoticeType   string
	NoticeMsg    string
	Pagination   components.PaginationProps
}

// savingFilterClass marks the filter card that matches the active filter.
func savingFilterClass(current, target string) string {
	if current == target || ((current == "" || current == "all") && target == "all") {
		return "s-filter-card-on"
	}
	return "s-filter-card-off"
}

func savingProductIDStr(pid *int64) string {
	if pid != nil && *pid > 0 {
		return fmt.Sprintf("%d", *pid)
	}
	return ""
}
