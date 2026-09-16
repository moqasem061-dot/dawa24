package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// VendorDashboardData is the supplier dashboard view model.
type VendorDashboardData struct {
	ActiveProducts        int
	PendingShipments      int
	DeliveredShipments    int
	TotalShipmentsCount   int
	PendingOrdersTotal    money.Amount
	MonthSales            money.Amount
	MonthNetProfit        money.Amount
	MonthProfitMargin     float64
	MonthCOGS             money.Amount
	WalletBalance         money.Amount
	HasWallet             bool
	LowStockCount         int
	ActiveWarehousesCount int
	Shipments             []*commerce.OrderShipment
	LowStock              []*inventory.Stock
	LowStockProductNames  map[int64]string
	Offers                []*promo.Offer
	UnreadQuotes          int
	PendingDocRequests    []*attachments.DocumentRequest
	Subscription          *OrgSubscriptionView
}

// PharmacyDashboardData is the pharmacy dashboard view model.
type PharmacyDashboardData struct {
	// Top 8 KPI Metrics
	TotalOrders       int
	CompletedOrders   int
	ActiveOrders      int
	CancelledOrders   int
	TotalSpend        money.Amount
	MonthSpend        money.Amount
	WalletBalance     money.Amount
	HasWallet         bool
	TotalOrderedItems int
	SmartOrdersCount  int

	// Pharmacy & Branch Context
	CustomerOrgName  string
	ActiveBranchID   int64
	ActiveBranchName string
	TotalBranches    int
	ActiveBranches   int
	Branches         []*org.Branch

	// Recent Orders Table
	Orders []*commerce.Order

	// Wallet Details
	PendingDepositsTotal money.Amount
	PendingDepositsCount int
	RecentTransactions   []*billing.WalletTransaction

	// Smart Orders Section
	SmartOrdersTotal       int
	SmartOrdersProcessing  int
	SmartOrdersCompleted   int
	SmartOrdersNeedsReview int
	RecentSmartOrders      []*smartorder.Run

	// Legacy & Supplemental
	Favorites          int
	ActiveOffers       int
	Offers             []*promo.BuyerOffer
	PendingDocRequests []*attachments.DocumentRequest
	Subscription       *OrgSubscriptionView
}

// FormatSmartOrderStatusLabel returns the localized label for Smart Order RunStatus.
func FormatSmartOrderStatusLabel(status smartorder.RunStatus, langOpt ...string) string {
	lang := "ar"
	if len(langOpt) > 0 && langOpt[0] != "" {
		lang = langOpt[0]
	}
	switch status {
	case smartorder.StatusPlaced, smartorder.StatusCompleted, smartorder.StatusFinalizing:
		return i18n.T(lang, "smartorder.status_placed")
	case smartorder.StatusProcessing, smartorder.StatusQueued:
		return i18n.T(lang, "smartorder.status_processing")
	case smartorder.StatusMapping:
		return i18n.T(lang, "smartorder.status_mapping")
	case smartorder.StatusDraft:
		return i18n.T(lang, "smartorder.status_draft")
	case smartorder.StatusStale:
		return i18n.T(lang, "smartorder.status_stale")
	case smartorder.StatusFailed:
		return i18n.T(lang, "smartorder.status_failed")
	default:
		return string(status)
	}
}

// FormatSmartOrderStatusTone returns the badge tone for Smart Order RunStatus.
func FormatSmartOrderStatusTone(status smartorder.RunStatus) string {
	switch status {
	case smartorder.StatusPlaced, smartorder.StatusCompleted:
		return "emerald"
	case smartorder.StatusProcessing, smartorder.StatusQueued, smartorder.StatusFinalizing:
		return "amber"
	case smartorder.StatusMapping, smartorder.StatusDraft:
		return "sky"
	case smartorder.StatusStale, smartorder.StatusFailed:
		return "rose"
	default:
		return "slate"
	}
}

// FormatTxTypeLabel returns the localized label for a wallet transaction type.
func FormatTxTypeLabel(t billing.TransactionType, langOpt ...string) string {
	lang := "ar"
	if len(langOpt) > 0 && langOpt[0] != "" {
		lang = langOpt[0]
	}
	switch t {
	case billing.TxDeposit:
		return i18n.T(lang, "tx.deposit")
	case billing.TxWithdrawal:
		return i18n.T(lang, "tx.withdrawal")
	case billing.TxPurchase:
		return i18n.T(lang, "tx.purchase")
	case billing.TxRefund:
		return i18n.T(lang, "tx.refund")
	case billing.TxBonus:
		return i18n.T(lang, "tx.bonus")
	case billing.TxPenalty:
		return i18n.T(lang, "tx.penalty")
	case billing.TxTransferIn:
		return i18n.T(lang, "tx.transfer_in")
	case billing.TxTransferOut:
		return i18n.T(lang, "tx.transfer_out")
	case billing.TxAdjustment:
		return i18n.T(lang, "tx.adjustment")
	default:
		return string(t)
	}
}

// CoveredPharmacyItem represents one pharmacy or vendor branch covered by the vendor's distribution network.
type CoveredPharmacyItem struct {
	PharmacyID         int64
	PharmacyName       string
	PharmacyTradeName  string
	OrgType            string
	IsVendor           bool
	BranchID           int64
	BranchName         string
	Address            string
	Phone              string
	CityID             *int64
	CityName           string
	CoveringBranchID   int64
	CoveringBranchName string
	DistanceMeters     int
	DistanceKM         float64
	CoveredDays        []int
	CoveredDaysLabels  []string
	TimeWindow         string
	IsCoveredToday     bool
	MatchReason        string
}

// VendorPharmacyCoverageData encapsulates all data for the /vendor/pharmacy-coverage page.
type VendorPharmacyCoverageData struct {
	Pharmacies        []CoveredPharmacyItem
	TotalPharmacies   int
	TotalVendors      int
	TotalFacilities   int
	CoveredTodayCount int
	CoveredCities     []string
	CoveredBranches   []string
	FilterDay         string
	FilterBranch      string
	FilterCity        string
	FilterType        string
	SearchQuery       string
}
