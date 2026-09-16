package compare_test

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// The three bugs that made both market screens useless, each pinned.

func offer(id int64, supplier, name string, productID *int64, price int64, discount float64) compare.MarketOffer {
	p := money.FromMajor(price)
	return compare.MarketOffer{
		RowID: id, FileID: id, SupplierName: supplier, ProductName: name,
		ProductID: productID, Price: p, Discount: discount,
		NetPrice: compare.CalculatePriceAfterDiscount(p, discount),
	}
}

func ptr(v int64) *int64 { return &v }

// A row whose net price is zero is a mapping error, not the market's best offer.
//
// 6,182 rows on the live platform — 12.5% of every priced row — carry a
// discount of 100 or more because their file's "discount" column holds
// something that is not a discount. Left in, each of them wins every comparison
// it enters: the optimal basket collapses toward zero, the best supplier is
// whoever has the most broken file, and the savings table fills with rows
// reading "free versus 1,382.00, save 100%".
func TestZeroNetOffersAreRejectedNotWinners(t *testing.T) {
	ds := compare.BuildMarketDataset([]compare.MarketOffer{
		offer(1, "مورد سليم", "بانادول إكسترا", ptr(101), 100, 20),  // net 80.00
		offer(2, "مورد أغلى", "بانادول إكسترا", ptr(101), 100, 10),  // net 90.00
		offer(3, "ملف معطوب", "بانادول إكسترا", ptr(101), 100, 100), // net 0.00
	})

	if ds.Rejected != 1 {
		t.Fatalf("Rejected = %d, want 1", ds.Rejected)
	}
	if got := ds.RejectedBySupplier["ملف معطوب"]; got != 1 {
		t.Fatalf("RejectedBySupplier[ملف معطوب] = %d, want 1", got)
	}

	p, ok := ds.Lookup(ptr(101), "")
	if !ok {
		t.Fatal("product 101 missing from dataset")
	}
	if p.Best.SupplierName != "مورد سليم" {
		t.Errorf("best supplier = %q, want the cheapest usable offer", p.Best.SupplierName)
	}
	if p.Best.NetPrice.String() != "80.00" {
		t.Errorf("best net = %s, want 80.00", p.Best.NetPrice)
	}

	// And the broken file is named, so someone can go and fix its mapping
	// rather than concluding the tool is wrong.
	worst := ds.WorstRejectedSuppliers(3)
	if len(worst) != 1 || worst[0].SupplierName != "ملف معطوب" {
		t.Errorf("WorstRejectedSuppliers = %+v, want the broken file named", worst)
	}
}

// Grouping is by catalogue product, so two spellings of one medicine compare.
//
// The old code grouped by three text keys derived from the raw name. Two
// suppliers writing the same product differently produced two groups, each with
// one supplier, each classified حصري — which is how a fourteen-hundred-row list
// came back with nothing to compare.
func TestGroupingUsesCatalogueIDAcrossSpellings(t *testing.T) {
	ds := compare.BuildMarketDataset([]compare.MarketOffer{
		offer(1, "مورد أ", "بانادول اكسترا 24 قرص", ptr(101), 100, 30),
		offer(2, "مورد ب", "Panadol Extra 24 Tab", ptr(101), 100, 10),
	})

	if got := len(ds.Products); got != 1 {
		t.Fatalf("products = %d, want 1: two spellings of one catalogue product", got)
	}
	comparable := ds.Comparable()
	if len(comparable) != 1 {
		t.Fatalf("comparable = %d, want 1", len(comparable))
	}
	if got := comparable[0].SupplierCount(); got != 2 {
		t.Errorf("SupplierCount = %d, want 2", got)
	}
	if got := comparable[0].Spread().String(); got != "20.00" {
		t.Errorf("Spread = %s, want 20.00 (90.00 − 70.00)", got)
	}
}

// Unmatched rows still compare, on the normalised name, and stay distinguishable.
func TestUnmatchedRowsGroupByNameAndAreFlagged(t *testing.T) {
	ds := compare.BuildMarketDataset([]compare.MarketOffer{
		offer(1, "مورد أ", "فيتامين سي 1000", nil, 50, 20),
		offer(2, "مورد ب", "فيتامين سي 1000", nil, 50, 10),
	})
	comparable := ds.Comparable()
	if len(comparable) != 1 {
		t.Fatalf("comparable = %d, want 1", len(comparable))
	}
	if ds.MatchedOffers != 0 {
		t.Errorf("MatchedOffers = %d, want 0: neither row is anchored to the catalogue", ds.MatchedOffers)
	}
	// And a catalogue id never collides with a name that folds to a number.
	if _, clash := ds.Products["p:101"]; clash {
		t.Error("name group leaked into the catalogue-id key space")
	}
}

// The best offer is the cheapest, not the most discounted.
//
// A 40% discount off an inflated list price is not a saving. Ranking by
// discount is how a comparison tool recommends the dearest house on the
// platform, which is what the old aggregation did throughout.
func TestBestOfferIsCheapestNotMostDiscounted(t *testing.T) {
	ds := compare.BuildMarketDataset([]compare.MarketOffer{
		offer(1, "خصم كبير سعر أعلى", "دواء", ptr(7), 200, 40), // net 120.00
		offer(2, "خصم صغير سعر أقل", "دواء", ptr(7), 110, 10),  // net 99.00
	})
	p, _ := ds.Lookup(ptr(7), "")
	if p.Best.SupplierName != "خصم صغير سعر أقل" {
		t.Errorf("best = %q, want the cheaper net price", p.Best.SupplierName)
	}
}

// An empty market is explained, not filled with plausible advice.
func TestEmptyMarketProducesNoFabricatedAdvice(t *testing.T) {
	report := compare.ReportFromDataset(compare.BuildMarketDataset(nil), 0, false, "ar")
	if report.HasData() {
		t.Fatal("HasData() on an empty market")
	}
	if len(report.Advice) != 0 || len(report.Guidance) != 0 || len(report.TopSavings) != 0 {
		t.Error("an empty market produced advice, guidance or opportunities")
	}
	if len(report.Analysis) != 1 {
		t.Fatalf("Analysis = %d entries, want exactly the one explaining the emptiness", len(report.Analysis))
	}
	if report.Analysis[0].Tone != compare.ToneWarn {
		t.Errorf("empty-market analysis tone = %q, want warn", report.Analysis[0].Tone)
	}
}

// Every number in the report is derived, and the arithmetic is exact.
func TestStrategicReportArithmetic(t *testing.T) {
	report := compare.ReportFromDataset(compare.BuildMarketDataset([]compare.MarketOffer{
		offer(1, "أ", "دواء واحد", ptr(1), 100, 20), // net 80.00 ← best
		offer(2, "ب", "دواء واحد", ptr(1), 100, 0),  // net 100.00 ← worst
		offer(3, "أ", "دواء اثنان", ptr(2), 50, 10), // net 45.00 ← best
		offer(4, "ب", "دواء اثنان", ptr(2), 50, 0),  // net 50.00 ← worst
	}), 2, true, "ar")

	if got := report.OptimalCost.String(); got != "125.00" {
		t.Errorf("OptimalCost = %s, want 125.00", got)
	}
	if got := report.WorstCost.String(); got != "150.00" {
		t.Errorf("WorstCost = %s, want 150.00", got)
	}
	if got := report.PotentialSavings.String(); got != "25.00" {
		t.Errorf("PotentialSavings = %s, want 25.00", got)
	}
	if report.SavingsPercent < 16.6 || report.SavingsPercent > 16.7 {
		t.Errorf("SavingsPercent = %v, want ~16.67", report.SavingsPercent)
	}

	// Supplier أ is cheapest on both products, so it is the best overall and
	// carries no excess over the optimal basket.
	if report.BestSupplier == nil || report.BestSupplier.SupplierName != "أ" {
		t.Fatalf("BestSupplier = %+v, want أ", report.BestSupplier)
	}
	if got := report.BestSupplier.Excess().String(); got != "0.00" {
		t.Errorf("best supplier excess = %s, want 0.00", got)
	}
	if got := report.Standings[1].Excess().String(); got != "25.00" {
		t.Errorf("second supplier excess = %s, want 25.00", got)
	}
}
