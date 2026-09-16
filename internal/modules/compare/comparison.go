package compare

import (
	"math"
	"sort"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// SupplierOffer represents a single supplier's offer for a specific product.
type SupplierOffer struct {
	SupplierName       string       `json:"supplier_name"`
	Price              money.Amount `json:"price"`
	Discount           float64      `json:"discount"` // percentage e.g. 15.50
	PriceAfterDiscount money.Amount `json:"price_after_discount"`
	StockQuantity      int          `json:"stock_quantity"`
}

// ProductComparisonRow represents the aggregated multi-supplier comparison row for a product.
type ProductComparisonRow struct {
	MatchedProductID   *int64                   `json:"matched_product_id,omitempty"`
	InCatalog          bool                     `json:"in_catalog"`
	CatalogStatus      CatalogStatus            `json:"catalog_status"`
	ProductName        string                   `json:"product_name"`
	SKU                string                   `json:"sku"`
	Offers             map[string]SupplierOffer `json:"offers"` // supplier_name -> offer
	BestPrice          money.Amount             `json:"best_price"`
	BestDiscount       float64                  `json:"best_discount"`
	BestNetPrice       money.Amount             `json:"best_net_price"`
	BestSupplier       string                   `json:"best_supplier"`
	TotalSuppliers     int                      `json:"total_suppliers"`
	MissingSuppliers   []string                 `json:"missing_suppliers"`
	AvailabilityStatus string                   `json:"availability_status"`
}

// ComparisonSummary represents the aggregate metrics across all analyzed supplier files.
type ComparisonSummary struct {
	TotalProducts        int            `json:"total_products"`
	TotalSuppliers       int            `json:"total_suppliers"`
	SuppliersList        []string       `json:"suppliers_list"`
	AverageDiscount      float64        `json:"average_discount"`
	BestOffersBySupplier map[string]int `json:"best_offers_by_supplier"`
	TotalPotentialSaving money.Amount   `json:"total_potential_saving"`
}

// ComparisonResultSet is the complete payload of a multi-supplier comparison run.
type ComparisonResultSet struct {
	Rows    []*ProductComparisonRow `json:"rows"`
	Summary ComparisonSummary       `json:"summary"`
}

// HeadToHeadItem represents one product's head-to-head comparison between two suppliers.
type HeadToHeadItem struct {
	ProductName    string       `json:"product_name"`
	SKU            string       `json:"sku"`
	SourcePrice    money.Amount `json:"source_price"`
	SourceDiscount float64      `json:"source_discount"`
	SourceNet      money.Amount `json:"source_net"`
	TargetPrice    money.Amount `json:"target_price"`
	TargetDiscount float64      `json:"target_discount"`
	TargetNet      money.Amount `json:"target_net"`
	IsBetter       bool         `json:"is_better"`  // true if SourceNet <= TargetNet
	PriceDiff      money.Amount `json:"price_diff"` // TargetNet - SourceNet
}

// HeadToHeadStats represents aggregate head-to-head metrics.
type HeadToHeadStats struct {
	SharedCount  int          `json:"shared_count"`
	BetterCount  int          `json:"better_count"`
	SourceTotal  money.Amount `json:"source_total"`
	TargetTotal  money.Amount `json:"target_total"`
	QualityScore float64      `json:"quality_score"` // (BetterCount / SharedCount) * 100
	TotalSavings money.Amount `json:"total_savings"` // TargetTotal - SourceTotal
}

// MarketComparisonFilter represents the 5 filter modes for comparing a supplier with the market baseline.
type MarketComparisonFilter string

const (
	MarketFilterAll            MarketComparisonFilter = "all"
	MarketFilterLowerDiscount  MarketComparisonFilter = "lower_discount_than_market"
	MarketFilterEqualToMarket  MarketComparisonFilter = "equal_to_market"
	MarketFilterHigherDiscount MarketComparisonFilter = "higher_discount_than_market"
	MarketFilterExclusives     MarketComparisonFilter = "exclusives"
)

// MarketComparisonItem represents a supplier product compared against the market baseline.
type MarketComparisonItem struct {
	ProductName      string                 `json:"product_name"`
	SKU              string                 `json:"sku"`
	SupplierPrice    money.Amount           `json:"supplier_price"`
	SupplierDiscount float64                `json:"supplier_discount"`
	SupplierNet      money.Amount           `json:"supplier_net"`
	MarketPrice      money.Amount           `json:"market_price"`
	MarketDiscount   float64                `json:"market_discount"`
	MarketNet        money.Amount           `json:"market_net"`
	Classification   MarketComparisonFilter `json:"classification"`
	HasMarketOffer   bool                   `json:"has_market_offer"`
}

// CalculatePriceAfterDiscount calculates the exact net price given a base price and discount percentage.
// Single currency invariant (Rule R1) using exact integer money math and banker-free half-up rounding.
func CalculatePriceAfterDiscount(price money.Amount, discountPercent float64) money.Amount {
	if discountPercent <= 0 {
		return price
	}
	if discountPercent >= 100.0 {
		return money.Zero
	}
	bps := int64(math.Round(discountPercent * 100.0))
	discountAmt := price.ApplyPercent(bps)
	net, err := price.Sub(discountAmt)
	if err != nil || net.IsNegative() {
		return money.Zero
	}
	return net
}

// getCoreDrugMatchKey is the identity two supplier files are grouped by.
//
// It used to be a matcher of its own, living here: a private normaliser, a
// private noise-word list, a private strength regex, and a table of about sixty
// Arabic brand names hand-mapped to their Latin spellings. That table was the
// only way a row written in Arabic could meet a row written in Latin, and sixty
// brands is not a market — everything outside it simply failed to group, so the
// same product from two suppliers arrived as two rows with one offer each,
// which is the one thing a price-comparison tool must never do.
//
// It also discarded any bare figure of three digits or fewer as noise, which
// merged "اتاكاند 16" with "اتاكاند 32" — one line carrying offers for two
// different strengths, with a "best price" that was the cheaper drug.
//
// internal/shared/productmatch answers this now, through ProductKey. The same
// consonant skeleton that lets the catalogue matcher read across both alphabets
// generically, the same curated modifier vocabulary that separates بانادول from
// بانادول اكسترا, the same identity letters that separate بتنوفيت ان from
// بتنوفيت سي, and the same dose reader that knows 10/20 and 20/10 are one
// combination while 10/40 is another.
//
// This function stays as the name the comparison code calls, so the grouping
// contract — a string, compared for equality — is unchanged.
func getCoreDrugMatchKey(name string) string {
	return productmatch.ProductKey(name)
}

// GetCoreDrugMatchKeyForTest exports getCoreDrugMatchKey for package tests.
func GetCoreDrugMatchKeyForTest(name string) string {
	return getCoreDrugMatchKey(name)
}

// getSortedWordsKey generates a bag-of-words key for order-independent name matching (e.g. "Panadol Extra" == "Extra Panadol").
func getSortedWordsKey(name string) string {
	norm := normalizeProductText(name)
	if norm == "" {
		return ""
	}
	words := strings.Fields(norm)
	sort.Strings(words)
	return strings.Join(words, " ")
}
