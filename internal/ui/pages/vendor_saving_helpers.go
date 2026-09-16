package pages

import (
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/ui/components"
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

func savingFilterBorder(current, target string) string {
	if current == target || ((current == "" || current == "all") && target == "all") {
		return "var(--accent)"
	}
	return "var(--border)"
}

func savingProductIDStr(pid *int64) string {
	if pid != nil && *pid > 0 {
		return fmt.Sprintf("%d", *pid)
	}
	return ""
}
