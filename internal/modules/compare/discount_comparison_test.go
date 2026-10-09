package compare

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

func TestLiteralImportedDiscount(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want float64
	}{
		{"25", 25}, {"0.25", 0.25}, {"25%", 25}, {"٢٥٫٣٥٪", 25.35},
		{"12,34", 12.34}, {"100", 100}, {"0", 0}, {"-5", 0},
		{"1250", 0}, {"NaN", 0}, {"Inf", 0}, {"25 / 30", 0},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			if got := ParseImportDiscount(tc.raw); got != tc.want {
				t.Fatalf("discount = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDiscountComparisonIgnoresConflictingPrices(t *testing.T) {
	for _, tc := range []struct {
		offer, yours float64
		want         BenchmarkBucket
	}{
		{12.35, 12.34, BucketBetter}, {12.34, 12.35, BucketWorse},
		{0.25, 0.25, BucketEqual}, {100, 99.99, BucketBetter},
	} {
		if got := discountBucket(tc.offer, tc.yours); got != tc.want {
			t.Fatalf("bucket(%v, %v) = %s, want %s", tc.offer, tc.yours, got, tc.want)
		}
	}
	if got := ClassifyMarketComparison(money.FromMinor(1), money.FromMinor(99999), 10, 20, true); got != MarketFilterLowerDiscount {
		t.Fatalf("a cheap price changed the discount classification: %s", got)
	}
}

func TestBenchmarkUsesHighestDiscountPerSupplier(t *testing.T) {
	id := int64(123)
	ds := BuildMarketDataset([]MarketOffer{
		{ProductID: &id, SupplierName: "One", Price: money.FromMinor(1000), Discount: 10, NetPrice: money.FromMinor(900)},
		{ProductID: &id, SupplierName: " ONE ", Price: money.FromMinor(100000), Discount: 30, NetPrice: money.FromMinor(70000)},
		{ProductID: &id, SupplierName: "Two", Price: money.FromMinor(1000), Discount: 20, NetPrice: money.FromMinor(800)},
	})
	r := benchmarkRow(&CompareFileRow{MatchedProductID: &id, Price: money.FromMinor(10000), Discount: 20}, ds, "Mine")
	if r.BetterOffers != 1 || r.EqualOffers != 1 || r.WorseOffers != 0 || r.Classification != BenchHigher || r.BestDiscount != 30 {
		t.Fatalf("incorrect discount benchmark: %+v", r)
	}
}

func TestDiscountOnlyAndFullDiscountOffersAreCompared(t *testing.T) {
	id := int64(555)
	ds := buildDiscountDataset([]MarketOffer{
		{ProductID: &id, SupplierName: "Discount only", Discount: 25.35},
		{ProductID: &id, SupplierName: "Full discount", Discount: 100},
	})
	row := benchmarkRow(&CompareFileRow{MatchedProductID: &id, Discount: 25.35}, ds, "Mine")
	if row.BetterOffers != 1 || row.EqualOffers != 1 || row.BestDiscount != 100 {
		t.Fatalf("discount-only benchmark: %+v", row)
	}
}
