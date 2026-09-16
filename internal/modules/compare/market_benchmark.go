package compare

import (
	"context"
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// مقارنة قائمتي مع عروض السوق العامة — one of my lists against everyone else's.
//
// The previous implementation matched a supplier's rows to the market by
// building three text keys off the raw product name and consulting them in
// turn. It also asked for the market with Limit: 50000 against a method that
// clamps to 100, so a fourteen-hundred-row list was compared against a hundred
// offers and the other thirteen hundred rows came back classified حصري — "no
// comparison available" — which is what the screen was reported as: everything
// exclusive, nothing matched.
//
// It now compares against the whole market, grouped by catalogue product where
// the matching engine has resolved one and by normalised name where it has not
// (see market_dataset.go), and it reports per row how many market offers are
// better, equal and worse than the supplier's own. Counts rather than a single
// average: "the market averages 12%" tells a supplier nothing about whether
// they are beaten by one outlier or by forty houses.

// equalTolerancePercent is how close two net prices must be to count as the
// same offer.
//
// Half a per cent. Pharmaceutical list prices are set to the piastre and
// carried through discount arithmetic, so two suppliers quoting "the same"
// price routinely differ by a few piastres of rounding. Treating that as a win
// or a loss produces a comparison where nothing is ever equal.
const equalTolerancePercent = 0.5

// Benchmark classifications. These are stated in terms of the net price a
// pharmacy pays, not the discount percentage: a 40% discount off an inflated
// list is not a better offer, and the old code classified on the discount alone.
const (
	// BenchHigher — your price is above the market's best. This is the saving
	// opportunity, and it is what أسعارك أعلى من السوق means.
	BenchHigher = "higher"
	// BenchEqual — your price sits within tolerance of the market's best.
	BenchEqual = "equal"
	// BenchBetter — you are the cheapest offer in the market for this product.
	BenchBetter = "better"
	// BenchExclusive — nobody else in the market carries this product.
	BenchExclusive = "exclusive"
)

// BenchmarkRow is one of the supplier's products against the market.
type BenchmarkRow struct {
	RowID       int64  `json:"row_id"`
	ProductID   *int64 `json:"product_id,omitempty"`
	ProductName string `json:"product_name"`
	SKU         string `json:"sku,omitempty"`
	// InCatalog says the comparison rests on the matching engine rather than on
	// two suppliers having spelled the name the same way.
	InCatalog bool `json:"in_catalog"`

	YourPrice    money.Amount `json:"your_price"`
	YourDiscount float64      `json:"your_discount"`
	YourNet      money.Amount `json:"your_net"`

	// BetterOffers, EqualOffers and WorseOffers count the market's offers
	// against this row's own net price.
	BetterOffers int `json:"better_offers"`
	EqualOffers  int `json:"equal_offers"`
	WorseOffers  int `json:"worse_offers"`

	// BestSupplier and its figures are the market's cheapest offer for this
	// product, excluding the supplier's own file.
	BestSupplier string       `json:"best_supplier,omitempty"`
	BestNet      money.Amount `json:"best_net"`
	BestDiscount float64      `json:"best_discount"`

	// MarketSuppliers is how many distinct houses quote this product.
	MarketSuppliers int `json:"market_suppliers"`
	// PriceGap is your net price minus the market's best: positive means you
	// are dearer, which is the saving available to your buyer.
	PriceGap money.Amount `json:"price_gap"`
	// PriceGapPercent is that gap against your own price.
	PriceGapPercent float64 `json:"price_gap_percent"`
	// DiscountGap is your discount minus the market's best discount, in points.
	DiscountGap float64 `json:"discount_gap"`

	Classification string `json:"classification"`
}

// BenchmarkResult is the whole screen.
type BenchmarkResult struct {
	FileID       int64  `json:"file_id"`
	SupplierName string `json:"supplier_name"`

	Rows []*BenchmarkRow `json:"rows"`
	// Shown is len(Rows); Total is before filtering, so the screen can say
	// "24 of 1,473" rather than implying the list is fourteen hundred long.
	Shown int `json:"shown"`
	Total int `json:"total"`

	HigherCount    int `json:"higher_count"`
	EqualCount     int `json:"equal_count"`
	BetterCount    int `json:"better_count"`
	ExclusiveCount int `json:"exclusive_count"`

	AvgYourDiscount   float64 `json:"avg_your_discount"`
	AvgMarketDiscount float64 `json:"avg_market_discount"`
	// PotentialBuyerSaving is what a pharmacy would save by buying every one of
	// this supplier's dearer lines elsewhere. It is the supplier's competitive
	// exposure stated in money.
	PotentialBuyerSaving money.Amount `json:"potential_buyer_saving"`

	Coverage MarketCoverage `json:"coverage"`
	// ComparedAgainst is how many market offers the comparison actually read,
	// so an empty result can be explained rather than merely displayed.
	ComparedAgainst int `json:"compared_against"`
}

// BenchmarkFilter narrows the screen.
type BenchmarkFilter struct {
	FileID      int64
	Query       string
	MinPrice    *float64
	MaxPrice    *float64
	MinDiscount *float64
	MaxDiscount *float64
	// Tab is one of "", "all", "higher", "equal", "better", "exclusive".
	Tab string
	// Sort is "discount", "price", or "" for the default, which is by the size
	// of the gap: the rows a supplier most needs to see, first.
	Sort string
	// OrganizationID admits the caller's own files to the market.
	OrganizationID *int64
}

// RunMarketBenchmark compares one of the caller's files against the market.
func (s *Service) RunMarketBenchmark(
	ctx context.Context, filter BenchmarkFilter,
) (*BenchmarkResult, error) {
	if filter.FileID <= 0 {
		return nil, apperr.Validation("compare.invalid_file",
			"يرجى تحديد قائمتك للمقارنة مع السوق.", nil)
	}

	file, err := s.repo.GetFileByID(ctx, filter.FileID)
	if err != nil {
		return nil, fmt.Errorf("compare: load file: %w", err)
	}

	mine, err := s.repo.ListFileRows(ctx, filter.FileID, 100000, 0)
	if err != nil {
		return nil, fmt.Errorf("compare: load file rows: %w", err)
	}

	// The market, with this file and supplier taken out of it.
	offers, err := s.repo.LoadMarketOffers(ctx, MarketScanOptions{
		OrganizationID:      filter.OrganizationID,
		ExcludeFileID:       filter.FileID,
		ExcludeSupplierName: file.SupplierName,
	})
	if err != nil {
		return nil, fmt.Errorf("compare: load market offers: %w", err)
	}
	ds := BuildMarketDataset(offers)

	result := &BenchmarkResult{
		FileID:          filter.FileID,
		SupplierName:    file.SupplierName,
		ComparedAgainst: ds.Offers,
		Coverage: MarketCoverage{
			Offers:        ds.Offers,
			MatchedOffers: ds.MatchedOffers,
			Products:      len(ds.Products),
			Suppliers:     len(ds.Suppliers),
			AIEnabled:     s.AIMatchingAvailable(),
		},
	}

	var yourDiscountSum, marketDiscountSum float64
	var marketDiscountCount int
	var exposure int64

	all := make([]*BenchmarkRow, 0, len(mine))
	for _, r := range mine {
		if r == nil || !r.Price.IsPositive() {
			continue
		}
		row := benchmarkRow(r, ds, file.SupplierName)
		all = append(all, row)

		yourDiscountSum += r.Discount
		switch row.Classification {
		case BenchHigher:
			result.HigherCount++
			exposure += row.PriceGap.Minor()
		case BenchEqual:
			result.EqualCount++
		case BenchBetter:
			result.BetterCount++
		default:
			result.ExclusiveCount++
		}
		if row.Classification != BenchExclusive {
			marketDiscountSum += row.BestDiscount
			marketDiscountCount++
		}
	}

	result.Total = len(all)
	if len(all) > 0 {
		result.AvgYourDiscount = round2(yourDiscountSum / float64(len(all)))
	}
	if marketDiscountCount > 0 {
		result.AvgMarketDiscount = round2(marketDiscountSum / float64(marketDiscountCount))
	}
	result.PotentialBuyerSaving = money.FromMinor(exposure)

	result.Rows = applyBenchmarkFilter(all, filter)
	result.Shown = len(result.Rows)
	return result, nil
}

// benchmarkRow places one of the supplier's rows against the market.
func benchmarkRow(r *CompareFileRow, ds *MarketDataset, supplierName string) *BenchmarkRow {
	net := r.PriceAfterDiscount
	if net.IsZero() && r.Price.IsPositive() {
		net = CalculatePriceAfterDiscount(r.Price, r.Discount)
	}

	row := &BenchmarkRow{
		RowID:          r.ID,
		ProductID:      r.MatchedProductID,
		ProductName:    r.RawName,
		SKU:            r.SKU,
		InCatalog:      r.MatchedProductID != nil && *r.MatchedProductID > 0,
		YourPrice:      r.Price,
		YourDiscount:   round2(r.Discount),
		YourNet:        net,
		Classification: BenchExclusive,
	}

	p, ok := ds.Lookup(r.MatchedProductID, r.RawName)
	if !ok || len(p.Offers) == 0 {
		return row
	}

	tolerance := int64(float64(net.Minor()) * equalTolerancePercent / 100)
	seen := map[string]bool{}
	var validOffers []MarketOffer
	for _, o := range p.Offers {
		// Never benchmark against the supplier itself
		if supplierName != "" && strings.EqualFold(strings.TrimSpace(o.SupplierName), strings.TrimSpace(supplierName)) {
			continue
		}
		// One vote per supplier: a house that uploaded the same list twice must
		// not count as two better offers.
		if o.SupplierName != "" {
			if seen[o.SupplierName] {
				continue
			}
			seen[o.SupplierName] = true
		}
		validOffers = append(validOffers, o)
		switch d := o.NetPrice.Minor() - net.Minor(); {
		case d < -tolerance:
			row.BetterOffers++
		case d > tolerance:
			row.WorseOffers++
		default:
			row.EqualOffers++
		}
	}

	if len(validOffers) == 0 {
		return row
	}

	row.MarketSuppliers = len(validOffers)
	// Find best competitor offer
	best := validOffers[0]
	for _, o := range validOffers[1:] {
		if cheaper(o, best) {
			best = o
		}
	}
	row.BestSupplier = best.SupplierName
	row.BestNet = best.NetPrice
	row.BestDiscount = round2(best.Discount)

	gap := net.Minor() - best.NetPrice.Minor()
	if gap > 0 {
		row.PriceGap = money.FromMinor(gap)
		if net.Minor() > 0 {
			row.PriceGapPercent = round2(float64(gap) / float64(net.Minor()) * 100)
		}
	}
	row.DiscountGap = round2(r.Discount - best.Discount)

	switch {
	case row.BetterOffers == 0 && row.WorseOffers == 0 && row.EqualOffers == 0:
		row.Classification = BenchExclusive
	case row.BetterOffers > 0:
		row.Classification = BenchHigher
	case row.WorseOffers > 0 && row.EqualOffers == 0:
		row.Classification = BenchBetter
	default:
		row.Classification = BenchEqual
	}
	return row
}
