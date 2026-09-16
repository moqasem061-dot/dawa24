package pages

import (
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

func rowQuantityValue(row *ingest.RowOutcome) string {
	if row != nil && row.Payload != nil && row.Payload.HasQuantity {
		return fmt.Sprint(row.Payload.Quantity)
	}
	return ""
}

// rowPriceValue is the public price shown in the "سعر الجمهور" column.
func rowPriceValue(row *ingest.RowOutcome) string {
	if row != nil && row.Payload != nil {
		if row.Payload.PublicPrice.IsPositive() {
			return row.Payload.PublicPrice.String()
		}
		if row.Payload.NetPrice.IsPositive() {
			return row.Payload.NetPrice.String()
		}
	}
	return ""
}

// rowDiscountPercentValue derives the discount percentage.
func rowDiscountPercentValue(row *ingest.RowOutcome) string {
	if row == nil || row.Payload == nil {
		return "0.0"
	}
	if row.Payload.DiscountBps > 0 {
		return fmt.Sprintf("%.1f", float64(row.Payload.DiscountBps)/100.0)
	}
	list := row.Payload.PublicPrice
	net := row.Payload.NetPrice
	if list.IsPositive() && net.IsPositive() && net.Minor() < list.Minor() {
		pct := (1.0 - float64(net.Minor())/float64(list.Minor())) * 100.0
		return fmt.Sprintf("%.1f", pct)
	}
	return "0.0"
}

// rowNetPriceValue calculates the price after discount.
func rowNetPriceValue(row *ingest.RowOutcome) string {
	if row == nil || row.Payload == nil {
		return "0.00"
	}
	if row.Payload.NetPrice.IsPositive() {
		return row.Payload.NetPrice.String()
	}
	list := row.Payload.PublicPrice
	if list.IsPositive() {
		if row.Payload.DiscountBps > 0 {
			net := list.ApplyPercent(10000 - row.Payload.DiscountBps)
			return net.String()
		}
		return list.String()
	}
	return "0.00"
}

// rowCostPriceValue returns the cost price to display in the review table.
func rowCostPriceValue(row *ingest.RowOutcome) string {
	if row != nil && row.Payload != nil && row.Payload.CostPrice.IsPositive() {
		return row.Payload.CostPrice.String()
	}
	return ""
}

// rowCostDiscountPercentValue returns the cost discount percentage.
func rowCostDiscountPercentValue(row *ingest.RowOutcome) string {
	if row != nil && row.Payload != nil && row.Payload.CostDiscountBps > 0 {
		return fmt.Sprintf("%.1f", float64(row.Payload.CostDiscountBps)/100.0)
	}
	return "0.0"
}

// rowEffectiveCostValue calculates unit cost based on public price and cost discount, or cost price.
func rowEffectiveCostValue(row *ingest.RowOutcome) string {
	if row == nil || row.Payload == nil {
		return "0.00"
	}
	if row.Payload.CostDiscountBps > 0 && row.Payload.PublicPrice.IsPositive() {
		discMinor := int64(float64(row.Payload.PublicPrice.Minor()) * (float64(row.Payload.CostDiscountBps) / 10000.0))
		cost := money.FromMinor(row.Payload.PublicPrice.Minor() - discMinor)
		return cost.String()
	}
	if row.Payload.CostPrice.IsPositive() {
		return row.Payload.CostPrice.String()
	}
	return "0.00"
}

// reviewRowWillImport reports whether the commit would write this row.
func reviewRowWillImport(row *ingest.RowOutcome) bool {
	if row == nil || row.IsExcluded || row.ProductID == nil || *row.ProductID <= 0 {
		return false
	}
	if row.IsManuallyMatched {
		return true
	}
	return productmatch.MatchLevel(row.MatchLevel).Settled()
}

// ImportStockModeLabel returns a friendly label for the stock mode in the given language.
func ImportStockModeLabel(mode inventory.StockMode, langOpt ...string) string {
	if len(langOpt) > 0 && langOpt[0] == "en" {
		switch mode {
		case inventory.StockAdd:
			return "Add to current stock (+)"
		case inventory.StockKeep:
			return "Ignore quantities and keep current balances"
		default:
			return "Replace balance with incoming quantity"
		}
	}
	switch mode {
	case inventory.StockAdd:
		return "إضافة إلى الرصيد الحالي (+)"
	case inventory.StockKeep:
		return "تجاهل الكميات والإبقاء على الأرصدة الحالية"
	default:
		return "استبدال الرصيد بالكمية الواردة"
	}
}

// planRetire is how many of the vendor's items a commit would take off sale,
// and zero where no plan could be computed.
func planRetire(plan *ingest.CommitPlan) int {
	if plan == nil {
		return 0
	}
	return plan.Retire
}

// commitButtonLabel names the button after what pressing it does under the
// chosen mode in the given language.
func commitButtonLabel(view VendorImportView, langOpt ...string) string {
	isEN := (len(langOpt) > 0 && langOpt[0] == "en") || view.Lang == "en"
	if view.Session == nil {
		if isEN {
			return "Confirm & Save"
		}
		return "اعتماد وحفظ"
	}
	switch view.Session.Settings.Mode {
	case ingest.ModeAddOnly:
		if isEN {
			return "Confirm & Add New Products"
		}
		return "اعتماد وإضافة الأصناف الجديدة"
	case ingest.ModeUpdateOnly:
		if isEN {
			return "Confirm & Update Existing Products"
		}
		return "اعتماد وتحديث الأصناف الموجودة"
	case ingest.ModeReplace:
		if isEN {
			return "Clear Warehouse & Commit New File"
		}
		return "مسح المخزن واعتماد الملف الجديد"
	default:
		if isEN {
			return "Confirm & Save Matched Products"
		}
		return "اعتماد وحفظ الأصناف المطابقة"
	}
}

// ImportModeOptions are the reconciliation strategies for the settings screen.
func ImportModeOptions(langOpt ...string) []ingest.ModeOption {
	lang := "ar"
	if len(langOpt) > 0 && langOpt[0] != "" {
		lang = langOpt[0]
	}
	return ingest.ModeOptionsForLang(lang)
}

// ImportModeOption returns the option descriptor for a given mode.
func ImportModeOption(mode ingest.Mode, langOpt ...string) ingest.ModeOption {
	opts := ImportModeOptions(langOpt...)
	for _, o := range opts {
		if o.Mode == mode {
			return o
		}
	}
	return opts[0]
}

// importResultIsFailure reports whether the results screen's leading sentence
// is about something that went wrong.
func importResultIsFailure(view VendorImportView) bool {
	if view.Session == nil {
		return false
	}
	return view.Session.Phase == ingest.PhaseFailed || view.Session.ErrorRows > 0
}

