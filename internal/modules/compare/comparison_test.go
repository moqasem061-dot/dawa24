package compare_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestCalculatePriceAfterDiscount_ExactMath(t *testing.T) {
	// Rule R1 / Single Currency math checks:
	// Price = 100.00 EGP (10000 cents), Discount = 15.00% -> Net = 85.00 EGP (8500 cents)
	p1 := money.FromMajor(100)
	net1 := compare.CalculatePriceAfterDiscount(p1, 15.0)
	if net1 != money.FromMajor(85) {
		t.Errorf("expected 85.00, got %v", net1)
	}

	// Price = 45.50 EGP (4550 cents), Discount = 10.00% -> Net = 40.95 EGP (4095 cents)
	p2 := money.FromMinor(4550)
	net2 := compare.CalculatePriceAfterDiscount(p2, 10.0)
	if net2 != money.FromMinor(4095) {
		t.Errorf("expected 40.95 (4095 cents), got %v", net2)
	}

	// Price = 200.00 EGP, Discount = 0% -> Net = 200.00 EGP
	net3 := compare.CalculatePriceAfterDiscount(money.FromMajor(200), 0)
	if net3 != money.FromMajor(200) {
		t.Errorf("expected 200.00, got %v", net3)
	}

	// Price = 150.00 EGP, Discount = 100% -> Net = 0
	net4 := compare.CalculatePriceAfterDiscount(money.FromMajor(150), 100)
	if net4 != money.Zero {
		t.Errorf("expected 0, got %v", net4)
	}
}

func TestMultiSupplierComparison(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	// File 1: Supplier A (United Pharma)
	f1 := &compare.CompareFile{
		SupplierName: "المتحدة للصيادلة",
		Status:       compare.FileReady,
	}
	_ = repo.CreateFile(ctx, f1)
	rows1 := []*compare.CompareFileRow{
		{
			FileID:             f1.ID,
			RowNumber:          1,
			RawName:            "Panadol Extra 24 Tab",
			NormalizedName:     "panadol extra 24 tab",
			SKU:                "SKU-PAN-EXT",
			Price:              money.FromMajor(100),
			Discount:           10.0, // Net 90.00
			PriceAfterDiscount: money.FromMajor(90),
		},
		{
			FileID:             f1.ID,
			RowNumber:          2,
			RawName:            "Cataflam 50mg 20 Tab",
			NormalizedName:     "cataflam 50mg 20 tab",
			SKU:                "SKU-CAT-50",
			Price:              money.FromMajor(50),
			Discount:           20.0, // Net 40.00
			PriceAfterDiscount: money.FromMajor(40),
		},
	}
	_ = repo.InsertFileRows(ctx, rows1)

	// File 2: Supplier B (Ibn Sina)
	f2 := &compare.CompareFile{
		SupplierName: "ابن سينا فارما",
		Status:       compare.FileReady,
	}
	_ = repo.CreateFile(ctx, f2)
	rows2 := []*compare.CompareFileRow{
		{
			FileID:             f2.ID,
			RowNumber:          1,
			RawName:            "Panadol Extra 24 Tab",
			NormalizedName:     "panadol extra 24 tab",
			SKU:                "SKU-PAN-EXT",
			Price:              money.FromMajor(100),
			Discount:           15.0, // Net 85.00 (Better than Supplier A)
			PriceAfterDiscount: money.FromMajor(85),
		},
		{
			FileID:             f2.ID,
			RowNumber:          2,
			RawName:            "Cataflam 50mg 20 Tab",
			NormalizedName:     "cataflam 50mg 20 tab",
			SKU:                "SKU-CAT-50",
			Price:              money.FromMajor(50),
			Discount:           10.0, // Net 45.00 (Worse than Supplier A)
			PriceAfterDiscount: money.FromMajor(45),
		},
	}
	_ = repo.InsertFileRows(ctx, rows2)

	// Run multi-supplier comparison
	res, err := svc.RunMultiSupplierComparison(ctx, []int64{f1.ID, f2.ID})
	if err != nil {
		t.Fatalf("failed to run comparison: %v", err)
	}

	if len(res.Rows) != 2 {
		t.Fatalf("expected 2 aggregated product rows, got %d", len(res.Rows))
	}
	if res.Summary.TotalProducts != 2 {
		t.Errorf("expected summary TotalProducts = 2, got %d", res.Summary.TotalProducts)
	}
	if res.Summary.TotalSuppliers != 2 {
		t.Errorf("expected summary TotalSuppliers = 2, got %d", res.Summary.TotalSuppliers)
	}

	// Verify Panadol Best Supplier is Ibn Sina (Net 85 vs 90)
	panadolRow := res.Rows[1] // Alphabetical: Panadol comes after Cataflam
	if panadolRow.ProductName != "Panadol Extra 24 Tab" {
		panadolRow = res.Rows[0]
	}
	if panadolRow.BestSupplier != "ابن سينا فارما" {
		t.Errorf("expected Panadol best supplier 'ابن سينا فارما', got %s", panadolRow.BestSupplier)
	}
	if panadolRow.BestNetPrice != money.FromMajor(85) {
		t.Errorf("expected Panadol best net price 85, got %v", panadolRow.BestNetPrice)
	}

	// Verify Cataflam Best Supplier is United Pharma (Net 40 vs 45)
	cataflamRow := res.Rows[0]
	if cataflamRow.ProductName != "Cataflam 50mg 20 Tab" {
		cataflamRow = res.Rows[1]
	}
	if cataflamRow.BestSupplier != "المتحدة للصيادلة" {
		t.Errorf("expected Cataflam best supplier 'المتحدة للصيادلة', got %s", cataflamRow.BestSupplier)
	}
	if cataflamRow.BestNetPrice != money.FromMajor(40) {
		t.Errorf("expected Cataflam best net price 40, got %v", cataflamRow.BestNetPrice)
	}
}

func TestSupplierVsSupplierHeadToHead(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	f1 := &compare.CompareFile{SupplierName: "المتحدة", Status: compare.FileReady}
	_ = repo.CreateFile(ctx, f1)
	rows1 := []*compare.CompareFileRow{
		{FileID: f1.ID, RawName: "Panadol", SKU: "SKU-1", Price: money.FromMajor(100), Discount: 20.0}, // Net 80
		{FileID: f1.ID, RawName: "Cataflam", SKU: "SKU-2", Price: money.FromMajor(50), Discount: 10.0}, // Net 45
	}
	_ = repo.InsertFileRows(ctx, rows1)

	f2 := &compare.CompareFile{SupplierName: "ابن سينا", Status: compare.FileReady}
	_ = repo.CreateFile(ctx, f2)
	rows2 := []*compare.CompareFileRow{
		{FileID: f2.ID, RawName: "Panadol", SKU: "SKU-1", Price: money.FromMajor(100), Discount: 10.0}, // Net 90 (Source f1 is better)
		{FileID: f2.ID, RawName: "Cataflam", SKU: "SKU-2", Price: money.FromMajor(50), Discount: 20.0}, // Net 40 (Target f2 is better)
	}
	_ = repo.InsertFileRows(ctx, rows2)

	items, stats, err := svc.RunSupplierVsSupplier(ctx, f1.ID, f2.ID)
	if err != nil {
		t.Fatalf("failed to run head to head: %v", err)
	}

	if stats.SharedCount != 2 {
		t.Errorf("expected shared count = 2, got %d", stats.SharedCount)
	}
	if stats.BetterCount != 1 {
		t.Errorf("expected better count = 1, got %d", stats.BetterCount)
	}
	if stats.QualityScore != 50.0 {
		t.Errorf("expected quality score = 50%%, got %f", stats.QualityScore)
	}
	// Source Total = 80 + 45 = 125. Target Total = 90 + 40 = 130. Total savings = 5.
	if stats.SourceTotal != money.FromMajor(125) {
		t.Errorf("expected source total 125, got %v", stats.SourceTotal)
	}
	if stats.TargetTotal != money.FromMajor(130) {
		t.Errorf("expected target total 130, got %v", stats.TargetTotal)
	}
	if stats.TotalSavings != money.FromMajor(5) {
		t.Errorf("expected total savings 5, got %v", stats.TotalSavings)
	}
	_ = items
}

func TestClassifyMarketComparison_FiveModes(t *testing.T) {
	// 1. Exclusives (No market offer)
	c1 := compare.ClassifyMarketComparison(money.FromMajor(50), money.Zero, 10, 0, false)
	if c1 != compare.MarketFilterExclusives {
		t.Errorf("expected exclusives, got %s", c1)
	}

	// 2. Higher discount than market (Supplier net lower or discount higher)
	c2 := compare.ClassifyMarketComparison(money.FromMajor(80), money.FromMajor(90), 20.0, 10.0, true)
	if c2 != compare.MarketFilterHigherDiscount {
		t.Errorf("expected higher_discount_than_market, got %s", c2)
	}

	// 3. Lower discount than market (Supplier net higher or discount lower)
	c3 := compare.ClassifyMarketComparison(money.FromMajor(95), money.FromMajor(85), 5.0, 15.0, true)
	if c3 != compare.MarketFilterLowerDiscount {
		t.Errorf("expected lower_discount_than_market, got %s", c3)
	}

	// 4. Equal to market
	c4 := compare.ClassifyMarketComparison(money.FromMajor(85), money.FromMajor(85), 15.0, 15.0, true)
	if c4 != compare.MarketFilterEqualToMarket {
		t.Errorf("expected equal_to_market, got %s", c4)
	}
}
