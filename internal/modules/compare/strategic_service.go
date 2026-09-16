package compare

import (
	"context"
	"fmt"
	"sort"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Computing تقرير التوفير الاستراتيجي from the market as it actually is.

// topSavingsShown is how many rows the "largest saving gap" table carries.
const topSavingsShown = 25

// exclusivesShown bounds the sole-supplier list.
const exclusivesShown = 25

// minSpreadPercent is the gap below which a price difference is noise rather
// than an opportunity.
//
// Two suppliers quoting the same medicine within one per cent of each other are
// not a negotiating position, they are a market working. Listing them pads the
// table and buries the rows that matter.
const minSpreadPercent = 1.0

// BuildStrategicReport is تقرير ذكاء السوق, computed over the whole market.
func (s *Service) BuildStrategicReport(
	ctx context.Context, orgID *int64, lang string,
) (*StrategicSavingReport, error) {
	offers, err := s.repo.LoadMarketOffers(ctx, MarketScanOptions{OrganizationID: orgID})
	if err != nil {
		return nil, fmt.Errorf("compare: load market offers: %w", err)
	}
	ds := BuildMarketDataset(offers)

	files := map[int64]bool{}
	for _, o := range offers {
		files[o.FileID] = true
	}

	return ReportFromDataset(ds, len(files), s.AIMatchingAvailable(), lang), nil
}

// topSavingOpportunities ranks products by what buying them wrong costs.
//
// Ranked by absolute money per unit rather than by percentage: a 60% gap on a
// four-pound sachet is a rounding error next to a 9% gap on a nine-hundred-pound
// oncology pack, and a purchasing manager reading the table needs the second one
// first. The percentage is shown beside it because it is what a negotiation is
// conducted in.
func topSavingOpportunities(products []*MarketProduct) []*SavingOpportunity {
	out := make([]*SavingOpportunity, 0, len(products))
	for _, p := range products {
		if p.SpreadPercent() < minSpreadPercent {
			continue
		}
		out = append(out, opportunityOf(p))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].PriceDifference.Minor() != out[j].PriceDifference.Minor() {
			return out[i].PriceDifference.Minor() > out[j].PriceDifference.Minor()
		}
		return out[i].PricePercent > out[j].PricePercent
	})
	if len(out) > topSavingsShown {
		out = out[:topSavingsShown]
	}
	return out
}

func opportunityOf(p *MarketProduct) *SavingOpportunity {
	return &SavingOpportunity{
		ProductName:        p.DisplayName,
		SKU:                p.SKU,
		InCatalog:          p.ProductID != nil && *p.ProductID > 0,
		BestSupplier:       p.Best.SupplierName,
		BestPrice:          p.Best.Price,
		BestNet:            p.Best.NetPrice,
		BestDiscount:       round2(p.Best.Discount),
		WorstSupplier:      p.Worst.SupplierName,
		WorstPrice:         p.Worst.Price,
		WorstNet:           p.Worst.NetPrice,
		WorstDiscount:      round2(p.Worst.Discount),
		PriceDifference:    p.Spread(),
		PricePercent:       p.SpreadPercent(),
		DiscountDifference: p.DiscountSpread(),
		SupplierCount:      p.SupplierCount(),
	}
}

// exclusiveOpportunities lists the products one supplier alone carries.
func exclusiveOpportunities(ds *MarketDataset) []*SavingOpportunity {
	ex := ds.Exclusive()
	sort.SliceStable(ex, func(i, j int) bool {
		return ex[i].Best.NetPrice.Minor() > ex[j].Best.NetPrice.Minor()
	})
	if len(ex) > exclusivesShown {
		ex = ex[:exclusivesShown]
	}
	out := make([]*SavingOpportunity, 0, len(ex))
	for _, p := range ex {
		out = append(out, opportunityOf(p))
	}
	return out
}

// supplierStandings ranks suppliers by what a basket of the products they
// actually quote costs from them against what it costs bought optimally.
//
// This is the honest form of "best supplier", and it is not the same as "most
// discounts" or "cheapest on the most rows". A supplier quoting forty cheap
// products and winning all of them is not a better partner than one quoting
// four hundred and losing narrowly on half. Comparing each supplier's own
// basket against the optimal cost of that same basket removes the size effect:
// the question becomes "when I buy what they sell, how much more do I pay?",
// which is the question a purchasing manager is actually asking.
//
// A supplier is ranked only once they quote a meaningful share of what is being
// compared, so a one-product supplier who happens to be cheapest on it cannot
// be crowned the market's best.
func supplierStandings(products []*MarketProduct) []*SupplierStanding {
	type acc struct {
		offers       int
		wins         int
		discountSum  float64
		premiumSum   float64
		basketMinor  int64
		optimalMinor int64
	}
	by := map[string]*acc{}

	for _, p := range products {
		best := p.Best.NetPrice.Minor()
		// One entry per supplier per product: a supplier listing the same
		// medicine on three of their files must not be counted three times, and
		// their cheapest row is the one they would actually sell at.
		cheapest := map[string]MarketOffer{}
		for _, o := range p.Offers {
			if o.SupplierName == "" {
				continue
			}
			if cur, ok := cheapest[o.SupplierName]; !ok || cheaper(o, cur) {
				cheapest[o.SupplierName] = o
			}
		}
		for name, o := range cheapest {
			a, ok := by[name]
			if !ok {
				a = &acc{}
				by[name] = a
			}
			a.offers++
			a.discountSum += o.Discount
			a.basketMinor += o.NetPrice.Minor()
			a.optimalMinor += best
			if o.NetPrice.Minor() == best {
				a.wins++
			}
			if best > 0 {
				a.premiumSum += float64(o.NetPrice.Minor()-best) / float64(best) * 100
			}
		}
	}

	// A supplier must quote at least this share of the comparable market, or
	// twenty products, before they are ranked at all.
	threshold := max(20, len(products)/20)

	out := make([]*SupplierStanding, 0, len(by))
	for name, a := range by {
		if a.offers < threshold {
			continue
		}
		st := &SupplierStanding{
			SupplierName: name,
			Offers:       a.offers,
			Wins:         a.wins,
			WinRate:      round2(float64(a.wins) / float64(a.offers) * 100),
			AvgDiscount:  round2(a.discountSum / float64(a.offers)),
			AvgPremium:   round2(a.premiumSum / float64(a.offers)),
			BasketCost:   money.FromMinor(a.basketMinor),
			BasketBest:   money.FromMinor(a.optimalMinor),
		}
		out = append(out, st)
	}

	// Nobody cleared the bar — a market of many small suppliers. Rank everyone
	// rather than returning an empty table, which reads as a broken page.
	if len(out) == 0 {
		for name, a := range by {
			out = append(out, &SupplierStanding{
				SupplierName: name,
				Offers:       a.offers,
				Wins:         a.wins,
				WinRate:      round2(float64(a.wins) / float64(a.offers) * 100),
				AvgDiscount:  round2(a.discountSum / float64(a.offers)),
				AvgPremium:   round2(a.premiumSum / float64(a.offers)),
				BasketCost:   money.FromMinor(a.basketMinor),
				BasketBest:   money.FromMinor(a.optimalMinor),
			})
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AvgPremium != out[j].AvgPremium {
			return out[i].AvgPremium < out[j].AvgPremium
		}
		return out[i].Offers > out[j].Offers
	})
	return out
}

// ReportFromDataset builds the report from an already-loaded market.
//
// BuildStrategicReport is the production entry point and owns the loading; this
// is the same computation over a dataset a caller already has, which is what
// makes the aggregation testable against a fixture — and lets an operator run
// it over a hypothetical market without touching a row.
func ReportFromDataset(ds *MarketDataset, files int, aiEnabled bool, lang string) *StrategicSavingReport {
	report := &StrategicSavingReport{
		Coverage: MarketCoverage{
			Offers:        ds.Offers,
			MatchedOffers: ds.MatchedOffers,
			Products:      len(ds.Products),
			Suppliers:     len(ds.Suppliers),
			Files:         files,
			Rejected:      ds.Rejected,
			AIEnabled:     aiEnabled,
		},
		RemapNeeded: ds.WorstRejectedSuppliers(5),
	}

	comparable := ds.Comparable()
	report.Coverage.ComparableProduct = len(comparable)
	report.Coverage.ExclusiveProduct = len(ds.Products) - len(comparable)
	if len(comparable) == 0 {
		report.Analysis = emptyMarketAnalysis(lang, report.Coverage)
		return report
	}

	var optimal, worst int64
	for _, p := range comparable {
		optimal += p.Best.NetPrice.Minor()
		worst += p.Worst.NetPrice.Minor()
	}
	report.OptimalCost = money.FromMinor(optimal)
	report.WorstCost = money.FromMinor(worst)
	if worst > optimal {
		report.PotentialSavings = money.FromMinor(worst - optimal)
		report.SavingsPercent = round2(float64(worst-optimal) / float64(worst) * 100)
	}

	report.TopSavings = topSavingOpportunities(comparable)
	report.Standings = supplierStandings(comparable)
	if len(report.Standings) > 0 {
		report.BestSupplier = report.Standings[0]
	}
	report.Exclusives = exclusiveOpportunities(ds)

	report.Analysis = strategicAnalysis(lang, report)
	report.Advice = strategicAdvice(lang, report)
	report.Guidance = purchasingGuidance(lang, report)
	return report
}
