package compare

import (
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// تقرير التوفير الاستراتيجي والتوصيات — the market intelligence report, rebuilt.
//
// What it replaced was a page of plausible-looking numbers computed over the
// hundred most recent rows on the platform (see market_dataset.go), with a
// "brand statistics" section driven by a hardcoded list of twenty-four company
// names and a "recommendations" section that was three fixed Arabic sentences
// printed on every load whatever the data said. Neither the brands nor the
// recommendations were derived from anything; a supplier reading them learned
// nothing about their market and had no way to tell.
//
// Every field below is computed from the market as loaded. Where a figure
// cannot be computed the section is absent, not filled with a plausible
// sentence. Coverage is reported alongside the answer, because a report over a
// market that is 5% anchored to the catalogue and one that is 90% anchored are
// different reports and the reader is entitled to know which they have.

// SavingOpportunity is one product and what buying it from the wrong supplier
// costs.
type SavingOpportunity struct {
	ProductName string `json:"product_name"`
	SKU         string `json:"sku,omitempty"`
	// InCatalog says the market's view of this product came from the matching
	// engine rather than from two rows having been spelled the same way.
	InCatalog bool `json:"in_catalog"`

	BestSupplier string       `json:"best_supplier"`
	BestPrice    money.Amount `json:"best_price"`
	BestNet      money.Amount `json:"best_net"`
	BestDiscount float64      `json:"best_discount"`

	WorstSupplier string       `json:"worst_supplier"`
	WorstPrice    money.Amount `json:"worst_price"`
	WorstNet      money.Amount `json:"worst_net"`
	WorstDiscount float64      `json:"worst_discount"`

	// PriceDifference is per unit, on the net price a pharmacy actually pays.
	PriceDifference money.Amount `json:"price_difference"`
	// PricePercent is that difference as a share of the dearer price.
	PricePercent float64 `json:"price_percent"`
	// DiscountDifference is the gap in commercial terms, in points.
	DiscountDifference float64 `json:"discount_difference"`

	SupplierCount int `json:"supplier_count"`
}

// SupplierStanding is one supplier's record across every product compared.
type SupplierStanding struct {
	SupplierName string `json:"supplier_name"`
	// Offers is how many comparable products this supplier quotes.
	Offers int `json:"offers"`
	// Wins is how many of those they are the cheapest on.
	Wins int `json:"wins"`
	// WinRate is Wins over Offers, as a percentage.
	WinRate float64 `json:"win_rate"`
	// AvgDiscount is the mean discount across their comparable offers.
	AvgDiscount float64 `json:"avg_discount"`
	// AvgPremium is how much dearer this supplier is than the market's best,
	// averaged over the products they quote, as a percentage. Zero means they
	// are the cheapest wherever they compete; it is the number that decides the
	// overall standing.
	AvgPremium float64 `json:"avg_premium"`
	// BasketCost is what buying one of each product they quote would cost from
	// them, and BasketBest what the same basket costs bought optimally. The
	// pair is what makes "best overall" defensible rather than a ranking.
	BasketCost money.Amount `json:"basket_cost"`
	BasketBest money.Amount `json:"basket_best"`
}

// Excess is what this supplier's basket costs above the optimal one.
func (s SupplierStanding) Excess() money.Amount {
	if s.BasketCost.Minor() <= s.BasketBest.Minor() {
		return money.Zero
	}
	return money.FromMinor(s.BasketCost.Minor() - s.BasketBest.Minor())
}

// InsightTone colours a generated statement.
type InsightTone string

const (
	ToneGood    InsightTone = "good"
	ToneWarn    InsightTone = "warn"
	ToneNeutral InsightTone = "neutral"
)

// Insight is one generated sentence with the figures that produced it.
//
// Every insight on the page is one of these, and every one of them is built by
// a function that had to read the data to say anything. There is no list of
// fixed sentences to fall back on: when nothing can be said, nothing is.
type Insight struct {
	Tone InsightTone `json:"tone"`
	Text string      `json:"text"`
}

// MarketCoverage describes what the report was computed over.
type MarketCoverage struct {
	Offers            int `json:"offers"`
	MatchedOffers     int `json:"matched_offers"`
	Products          int `json:"products"`
	ComparableProduct int `json:"comparable_products"`
	// ExclusiveProduct is how many products only one supplier carries. It is
	// the real total, not the length of the truncated display list — a report
	// that says "25 items are single-sourced" when 26,097 are is worse than one
	// that says nothing.
	ExclusiveProduct int `json:"exclusive_products"`
	Suppliers        int `json:"suppliers"`
	Files            int `json:"files"`
	// Rejected is how many offers were thrown out as unusable — almost always
	// a discount column that does not hold a discount. See MarketOffer.Usable.
	Rejected int `json:"rejected"`
	// AIEnabled is whether the platform can run the AI matching tier at all.
	AIEnabled bool `json:"ai_enabled"`
}

// MatchedPercent is how much of the market is anchored to the shared catalogue.
func (c MarketCoverage) MatchedPercent() float64 {
	if c.Offers == 0 {
		return 0
	}
	return round2(float64(c.MatchedOffers) / float64(c.Offers) * 100)
}

// StrategicSavingReport is the whole of تقرير ذكاء السوق.
type StrategicSavingReport struct {
	Coverage MarketCoverage `json:"coverage"`

	// OptimalCost is one of every comparable product bought from whichever
	// supplier is cheapest for it — شراء مجزأ, split purchasing.
	OptimalCost money.Amount `json:"optimal_cost"`
	// WorstCost is the same basket bought at each product's dearest offer. It
	// is the ceiling, not a prediction: nobody buys this way deliberately, and
	// the gap between the two is the whole value the tool exists to expose.
	WorstCost money.Amount `json:"worst_cost"`
	// PotentialSavings is WorstCost − OptimalCost.
	PotentialSavings money.Amount `json:"potential_savings"`
	// SavingsPercent is that as a share of WorstCost.
	SavingsPercent float64 `json:"savings_percent"`

	// Analysis is التحليل الاستراتيجي التلقائي.
	Analysis []Insight `json:"analysis"`
	// TopSavings is المنتجات ذات فارق الوفر الأكبر بالسوق.
	TopSavings []*SavingOpportunity `json:"top_savings"`
	// BestSupplier is المورد الأفضل إجمالاً, absent when nothing comparable was
	// found rather than defaulted to whoever happened to sort first.
	BestSupplier *SupplierStanding `json:"best_supplier,omitempty"`
	// Standings is the full table behind that choice.
	Standings []*SupplierStanding `json:"standings"`
	// Advice is نصيحة استراتيجية.
	Advice []Insight `json:"advice"`
	// Guidance is توجيهات الشراء المثالي.
	Guidance []Insight `json:"guidance"`

	// Exclusives are the products one supplier alone carries: no comparison is
	// possible, which is itself the finding.
	Exclusives []*SavingOpportunity `json:"exclusives"`

	// RemapNeeded names the suppliers whose files contributed the most
	// unusable rows. It is the actionable half of the rejection count: the fix
	// is to re-map that file's discount column, and this says which file.
	RemapNeeded []SupplierRejects `json:"remap_needed,omitempty"`
}

// HasData reports whether the report has anything to show.
func (r *StrategicSavingReport) HasData() bool {
	return r != nil && r.Coverage.ComparableProduct > 0
}
