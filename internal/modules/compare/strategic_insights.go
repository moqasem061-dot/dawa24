package compare

import (
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// The generated commentary: التحليل الاستراتيجي التلقائي, نصيحة استراتيجية and
// توجيهات الشراء المثالي.
//
// What was here before was a slice of three constant Arabic sentences,
// returned unconditionally, recommending that the reader consolidate purchasing
// on high-spread items and focus on local manufacturers. Both sentences may
// well be good advice. Neither was derived from the reader's data, both
// appeared on an empty market as readily as on a full one, and a report that
// says the same thing whatever it is looking at teaches its reader to stop
// reading it.
//
// Every sentence below is a function of numbers already computed. Where the
// numbers do not support a statement, no statement is produced — an analysis
// section with two entries because only two things could be said is more useful
// than one with six because six were written down in advance.
//
// The wording lives in internal/shared/i18n (catalog_market_intel.go) rather
// than here: this is a page a supplier reads, and every user-facing string on
// this platform is bilingual. The argument ORDER differs between the two
// languages for several of these, so the keys carry both renderings and the
// arguments are passed in the order the key documents.

// emptyMarketAnalysis explains an empty report in terms of what is missing,
// which is the only thing a reader can act on.
func emptyMarketAnalysis(lang string, c MarketCoverage) []Insight {
	switch {
	case c.Offers == 0:
		return []Insight{{Tone: ToneWarn, Text: i18n.T(lang, "market.intel.empty.no_offers")}}
	case c.Products <= 1:
		return []Insight{{
			Tone: ToneWarn,
			Text: fmt.Sprintf(i18n.T(lang, "market.intel.empty.single_supplier"), c.Offers),
		}}
	default:
		return []Insight{{
			Tone: ToneWarn,
			Text: fmt.Sprintf(i18n.T(lang, "market.intel.empty.nothing_shared"), c.Offers, c.Suppliers),
		}}
	}
}

// strategicAnalysis is التحليل الاستراتيجي التلقائي.
func strategicAnalysis(lang string, r *StrategicSavingReport) []Insight {
	var out []Insight

	if r.PotentialSavings.IsPositive() {
		out = append(out, Insight{
			Tone: ToneGood,
			Text: fmt.Sprintf(i18n.T(lang, "market.intel.analysis.savings"), r.WorstCost.String(), r.OptimalCost.String(),
				r.PotentialSavings.String(), r.SavingsPercent, r.Coverage.ComparableProduct),
		})
	} else {
		out = append(out, Insight{
			Tone: ToneNeutral,
			Text: fmt.Sprintf(i18n.T(lang, "market.intel.analysis.no_spread"), r.Coverage.ComparableProduct),
		})
	}

	// How much of the market the comparison actually reached.
	if r.Coverage.ExclusiveProduct > 0 {
		out = append(out, Insight{
			Tone: ToneNeutral,
			Text: fmt.Sprintf(i18n.T(lang, "market.intel.analysis.coverage"), r.Coverage.Products, r.Coverage.ComparableProduct, r.Coverage.ExclusiveProduct),
		})
	}

	// The honest statement about matching coverage, which is the single biggest
	// determinant of whether this report is trustworthy.
	matched := r.Coverage.MatchedPercent()
	switch {
	case r.Coverage.Offers == 0:
	case matched >= 70:
		out = append(out, Insight{
			Tone: ToneGood,
			Text: fmt.Sprintf(i18n.T(lang, "market.intel.analysis.matched_high"), matched, r.Coverage.MatchedOffers, r.Coverage.Offers),
		})
	case matched > 0:
		out = append(out, Insight{
			Tone: ToneWarn,
			Text: fmt.Sprintf(i18n.T(lang, "market.intel.analysis.matched_low"), matched, r.Coverage.MatchedOffers, r.Coverage.Offers),
		})
	default:
		out = append(out, Insight{
			Tone: ToneWarn,
			Text: fmt.Sprintf(i18n.T(lang, "market.intel.analysis.matched_none"), r.Coverage.Offers),
		})
	}

	// The rejected rows. Naming them is the difference between a supplier
	// concluding the tool is broken and a supplier fixing their column mapping.
	if r.Coverage.Rejected > 0 {
		var b strings.Builder
		b.WriteString(fmt.Sprintf(i18n.T(lang, "market.intel.analysis.rejected"), r.Coverage.Rejected))
		if len(r.RemapNeeded) > 0 {
			b.WriteString(i18n.T(lang, "market.intel.analysis.rejected_worst"))
			for i, sr := range r.RemapNeeded {
				if i > 0 {
					b.WriteString(i18n.T(lang, "common.list_separator"))
				}
				b.WriteString(fmt.Sprintf(i18n.T(lang, "market.intel.analysis.rejected_entry"), sr.SupplierName, sr.Rows))
			}
			b.WriteString(i18n.T(lang, "market.intel.analysis.rejected_fix"))
		}
		out = append(out, Insight{Tone: ToneWarn, Text: b.String()})
	}

	if !r.Coverage.AIEnabled {
		out = append(out, Insight{Tone: ToneWarn, Text: i18n.T(lang, "market.intel.analysis.ai_off")})
	}

	return out
}

// strategicAdvice is نصيحة استراتيجية: what to do about the numbers above.
func strategicAdvice(lang string, r *StrategicSavingReport) []Insight {
	var out []Insight

	// The negotiation opportunity, named with the actual product and figures.
	if len(r.TopSavings) > 0 {
		t := r.TopSavings[0]
		out = append(out, Insight{
			Tone: ToneGood,
			Text: fmt.Sprintf(i18n.T(lang, "market.intel.advice.negotiate"), t.ProductName, t.BestNet.String(), t.BestSupplier,
				t.WorstNet.String(), t.WorstSupplier,
				t.PriceDifference.String(), t.PricePercent),
		})
	}

	// A supplier who is systematically dearer is a supplier to renegotiate with
	// or replace, and naming them with the figure is the whole point.
	if worst := dearestStanding(r.Standings); worst != nil && worst.AvgPremium >= 5 {
		out = append(out, Insight{
			Tone: ToneWarn,
			Text: fmt.Sprintf(i18n.T(lang, "market.intel.advice.dearest"), worst.SupplierName, worst.AvgPremium, worst.Offers,
				worst.BasketCost.String(), worst.BasketBest.String(), worst.Excess().String()),
		})
	}

	// Discount terms as distinct from price. A supplier may be cheap on list
	// price and stingy on terms, and buyers negotiate the terms.
	if d := widestDiscountGap(r.TopSavings); d != nil && d.DiscountDifference >= 5 {
		out = append(out, Insight{
			Tone: ToneGood,
			Text: fmt.Sprintf(i18n.T(lang, "market.intel.advice.discount_gap"), d.ProductName, d.BestDiscount, d.BestSupplier,
				d.WorstDiscount, d.WorstSupplier, d.DiscountDifference),
		})
	}

	if r.Coverage.ExclusiveProduct > 0 {
		out = append(out, Insight{
			Tone: ToneNeutral,
			Text: fmt.Sprintf(i18n.T(lang, "market.intel.advice.exclusives"), r.Coverage.ExclusiveProduct, len(r.Exclusives)),
		})
	}

	return out
}

// purchasingGuidance is توجيهات الشراء المثالي: the concrete plan.
func purchasingGuidance(lang string, r *StrategicSavingReport) []Insight {
	var out []Insight

	if b := r.BestSupplier; b != nil {
		out = append(out, Insight{
			Tone: ToneGood,
			Text: fmt.Sprintf(i18n.T(lang, "market.intel.guidance.best_supplier"), b.SupplierName, b.Offers, b.Wins, b.WinRate, b.AvgPremium, b.AvgDiscount),
		})
	}

	if r.PotentialSavings.IsPositive() && len(r.TopSavings) > 0 {
		// Concentration: how much of the total prize sits in the visible table.
		var top int64
		for _, t := range r.TopSavings {
			top += t.PriceDifference.Minor()
		}
		share := float64(top) / float64(r.PotentialSavings.Minor()) * 100
		out = append(out, Insight{
			Tone: ToneGood,
			Text: fmt.Sprintf(i18n.T(lang, "market.intel.guidance.concentration"), len(r.TopSavings), share),
		})
	}

	if len(r.Standings) >= 2 && r.PotentialSavings.IsPositive() {
		// How many suppliers the optimal basket actually draws on, not how many
		// were ranked. "split across 15 suppliers" when only four of them are
		// ever the cheapest is a sentence that misstates the work involved.
		winners := 0
		for _, st := range r.Standings {
			if st.Wins > 0 {
				winners++
			}
		}
		if winners < 2 {
			winners = 2
		}
		out = append(out, Insight{
			Tone: ToneNeutral,
			Text: fmt.Sprintf(i18n.T(lang, "market.intel.guidance.split_versus_single"), winners, r.PotentialSavings.String(), r.Standings[0].SupplierName),
		})
	}

	return out
}

// dearestStanding is the ranked supplier with the largest average premium.
func dearestStanding(standings []*SupplierStanding) *SupplierStanding {
	var worst *SupplierStanding
	for _, s := range standings {
		if worst == nil || s.AvgPremium > worst.AvgPremium {
			worst = s
		}
	}
	return worst
}

// widestDiscountGap is the listed opportunity with the largest gap in terms.
func widestDiscountGap(items []*SavingOpportunity) *SavingOpportunity {
	var best *SavingOpportunity
	for _, it := range items {
		if best == nil || it.DiscountDifference > best.DiscountDifference {
			best = it
		}
	}
	return best
}
