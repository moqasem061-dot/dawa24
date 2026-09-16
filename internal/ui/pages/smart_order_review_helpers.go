package pages

import (
	"encoding/json"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// SmartOrderReviewLine is one line as the review screen shows it.
type SmartOrderReviewLine struct {
	Line           *smartorder.Line
	VendorName     string
	AvailableStock int
	UnitPrice      money.Amount
	DiscountPct    float64
	LineNet        money.Amount
	DecidedBy      smartorder.DecidedBy
	SkippedName    string
	SkippedExcess  float64
	Alternatives   int
}

// SmartOrderReviewGroup is the lines going to one supplier.
type SmartOrderReviewGroup struct {
	VendorName string
	Lines      []SmartOrderReviewLine
	TotalCount int
	Subtotal   money.Amount
}

// SmartOrderReviewData is the review screen's payload.
type SmartOrderReviewData struct {
	Run        *smartorder.Run
	Groups     []SmartOrderReviewGroup
	AllGroups  []SmartOrderReviewGroup
	Excluded   []*smartorder.Line
	BranchName string
	Stale      []smartorder.StaleLine
	Error      string
	Page       int
	PerPage    int
	TotalLines int
}

func decidedLabel(d smartorder.DecidedBy, langOpt ...string) string {
	isEn := len(langOpt) > 0 && langOpt[0] == "en"
	switch d {
	case smartorder.DecidedLowestPrice:
		if isEn {
			return "Lowest Price"
		}
		return "أقل سعر"
	case smartorder.DecidedHighestDiscount:
		if isEn {
			return "Highest Discount"
		}
		return "أعلى خصم"
	case smartorder.DecidedFollowedSuppliers:
		if isEn {
			return "Preferred Supplier"
		}
		return "مورد مفضل"
	case smartorder.DecidedOnlyCandidate:
		if isEn {
			return "Sole Supplier"
		}
		return "المورد الوحيد"
	case smartorder.DecidedUser:
		if isEn {
			return "Manual Selection"
		}
		return "اختيار يدوي"
	case smartorder.DecidedDefault:
		if isEn {
			return "Automatic"
		}
		return "تلقائي"
	}
	return "—"
}

func totalOrderableItems(groups []SmartOrderReviewGroup) int {
	total := 0
	for _, g := range groups {
		total += len(g.Lines)
	}
	return total
}

// GroupLineIDsJSON returns a JSON array of line ID strings for a review group.
func GroupLineIDsJSON(g SmartOrderReviewGroup) string {
	ids := make([]string, 0, len(g.Lines))
	for _, l := range g.Lines {
		ids = append(ids, fmt.Sprint(l.Line.ID))
	}
	b, _ := json.Marshal(ids)
	return string(b)
}

func reviewQtyMax(stock int) string {
	if stock > 0 {
		return fmt.Sprintf("%d", stock)
	}
	return ""
}
