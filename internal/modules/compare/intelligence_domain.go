package compare

import (
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// HeadToHeadComparisonResult holds the full head-to-head analysis payload with stats.
type HeadToHeadComparisonResult struct {
	SourceSupplierName string           `json:"source_supplier_name"`
	TargetSupplierName string           `json:"target_supplier_name"`
	SourceFileID       int64            `json:"source_file_id"`
	TargetFileID       int64            `json:"target_file_id"`
	Rows               []*HeadToHeadRow `json:"rows"`
	TotalShared        int              `json:"total_shared"`
	YourBetterCount    int              `json:"your_better_count"`
	EqualCount         int              `json:"equal_count"`
	CompetitorBetter   int              `json:"competitor_better_count"`
	WinRatePercent     float64          `json:"win_rate_percent"`
	TotalSavings       money.Amount     `json:"total_savings"`
	SourceTotal        money.Amount     `json:"source_total"`
	TargetTotal        money.Amount     `json:"target_total"`
}

// MarketBenchmarkFilter specifies filters for comparing a supplier against all market suppliers.
type MarketBenchmarkFilter struct {
	FileID      int64    `json:"file_id"`
	Query       string   `json:"query"`
	MinPrice    *float64 `json:"min_price"`
	MaxPrice    *float64 `json:"max_price"`
	MinDiscount *float64 `json:"min_discount"`
	MaxDiscount *float64 `json:"max_discount"`
	Tab         string   `json:"tab"` // "all", "higher", "equal", "lower", "exclusives"
}

// MarketBenchmarkRow represents a supplier's product benchmarked against market metrics.
type MarketBenchmarkRow struct {
	ProductID          *int64       `json:"product_id,omitempty"`
	ProductName        string       `json:"product_name"`
	SKU                string       `json:"sku,omitempty"`
	YourPrice          money.Amount `json:"your_price"`
	YourDiscount       float64      `json:"your_discount"`
	YourNetPrice       money.Amount `json:"your_net_price"`
	MarketAvgDiscount  float64      `json:"market_avg_discount"`
	MarketBestDiscount float64      `json:"market_best_discount"`
	MarketBestSupplier string       `json:"market_best_supplier"`
	MarketBestNetPrice money.Amount `json:"market_best_net_price"`
	DiffVsMarketAvg    float64      `json:"diff_vs_market_avg"`
	Classification     string       `json:"classification"` // "higher", "equal", "lower", "exclusive"
	SupplierCount      int          `json:"supplier_count"`
}

// MarketBenchmarkResult holds the benchmark dataset for a supplier file.
type MarketBenchmarkResult struct {
	SupplierName       string                `json:"supplier_name"`
	FileID             int64                 `json:"file_id"`
	Rows               []*MarketBenchmarkRow `json:"rows"`
	TotalItems         int                   `json:"total_items"`
	HigherThanMarket   int                   `json:"higher_than_market"`
	EqualToMarket      int                   `json:"equal_to_market"`
	LowerThanMarket    int                   `json:"lower_than_market"`
	ExclusivesCount    int                   `json:"exclusives_count"`
	AvgSupplierDisc    float64               `json:"avg_supplier_disc"`
	AvgMarketDisc      float64               `json:"avg_market_disc"`
	AvailableSuppliers []string              `json:"available_suppliers"`
}

// ArbitrageOpportunity represents a high-value savings deal with notable discount variance.
type ArbitrageOpportunity struct {
	ProductName    string       `json:"product_name"`
	SKU            string       `json:"sku,omitempty"`
	BestSupplier   string       `json:"best_supplier"`
	BestDiscount   float64      `json:"best_discount"`
	BestNetPrice   money.Amount `json:"best_net_price"`
	WorstSupplier  string       `json:"worst_supplier"`
	WorstDiscount  float64      `json:"worst_discount"`
	WorstNetPrice  money.Amount `json:"worst_net_price"`
	DiscountSpread float64      `json:"discount_spread"` // BestDiscount - WorstDiscount
	UnitSavings    money.Amount `json:"unit_savings"`    // WorstNet - BestNet
}

// BrandDiscountStat holds aggregated discount statistics for a pharmaceutical manufacturer/brand.
type BrandDiscountStat struct {
	BrandName    string  `json:"brand_name"`
	ProductCount int     `json:"product_count"`
	AvgDiscount  float64 `json:"avg_discount"`
	MaxDiscount  float64 `json:"max_discount"`
	TopSupplier  string  `json:"top_supplier"`
}

// MarketGapItem represents a product with limited or exclusive availability across active suppliers.
type MarketGapItem struct {
	ProductName  string       `json:"product_name"`
	SKU          string       `json:"sku,omitempty"`
	SoleSupplier string       `json:"sole_supplier"`
	Price        money.Amount `json:"price"`
	Discount     float64      `json:"discount"`
}

// MarketVitalKPIs holds platform-wide summary metrics for market intelligence.
type MarketVitalKPIs struct {
	TotalTrackedProducts int          `json:"total_tracked_products"`
	TotalActiveSuppliers int          `json:"total_active_suppliers"`
	OverallAvgDiscount   float64      `json:"overall_avg_discount"`
	HighestMarketDisc    float64      `json:"highest_market_disc"`
	TotalArbitrageDeals  int          `json:"total_arbitrage_deals"`
	TotalMarketGaps      int          `json:"total_market_gaps"`
	EstimatedMarketValue money.Amount `json:"estimated_market_value"`
}

// MarketIntelligenceReport provides full executive and tactical insight across market suppliers.
type MarketIntelligenceReport struct {
	KPIs            MarketVitalKPIs         `json:"kpis"`
	TopArbitrage    []*ArbitrageOpportunity `json:"top_arbitrage"`
	BrandStats      []*BrandDiscountStat    `json:"brand_stats"`
	MarketGaps      []*MarketGapItem        `json:"market_gaps"`
	Recommendations []string                `json:"recommendations"`
}

// ClassifyMarketComparison classifies a supplier row against market baseline into one of the 5 filter modes (Plan V5 §2.5.2).
func ClassifyMarketComparison(supplierNet, marketNet money.Amount, supplierDiscount, marketDiscount float64, hasMarketOffer bool) MarketComparisonFilter {
	if !hasMarketOffer {
		return MarketFilterExclusives
	}
	if supplierNet.Minor() < marketNet.Minor() || supplierDiscount > marketDiscount {
		return MarketFilterHigherDiscount
	}
	if supplierNet.Minor() > marketNet.Minor() || supplierDiscount < marketDiscount {
		return MarketFilterLowerDiscount
	}
	return MarketFilterEqualToMarket
}
