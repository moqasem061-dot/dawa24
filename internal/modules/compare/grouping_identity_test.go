package compare_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// T087 [US6]: Test: a shared prefix or manufacturer only => separate groups.
func TestGrouping_SharedPrefixOrManufacturerOnly_SeparateGroups(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	f1 := &compare.CompareFile{SupplierName: "المورد الأول", Status: compare.FileReady}
	f2 := &compare.CompareFile{SupplierName: "المورد الثاني", Status: compare.FileReady}
	_ = repo.CreateFile(ctx, f1)
	_ = repo.CreateFile(ctx, f2)

	cases := []struct {
		name       string
		sourceName string
		targetName string
	}{
		{
			name:       "shared brand prefix with different line modifier",
			sourceName: "بانادول اكسترا 24 قرص",
			targetName: "بانادول كولد اند فلو 24 قرص",
		},
		{
			name:       "shared manufacturer only with different products",
			sourceName: "فولتارين 50 مجم 20 قرص (نوفارتس)",
			targetName: "كتافلام 50 مجم 20 قرص (نوفارتس)",
		},
		{
			name:       "shared molecule prefix with different strength",
			sourceName: "أموكسيسيلين 500 مجم 12 كبسول",
			targetName: "أموكسيسيلين 250 مجم 12 كبسول",
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_ = repo.DeleteFileRows(ctx, f1.ID)
			_ = repo.DeleteFileRows(ctx, f2.ID)

			p := money.FromMajor(50)
			sRow := &compare.CompareFileRow{
				FileID:             f1.ID,
				RowNumber:          1,
				RawName:            tc.sourceName,
				NormalizedName:     tc.sourceName,
				Price:              p,
				Discount:           10.0,
				PriceAfterDiscount: compare.CalculatePriceAfterDiscount(p, 10.0),
			}
			tRow := &compare.CompareFileRow{
				FileID:             f2.ID,
				RowNumber:          1,
				RawName:            tc.targetName,
				NormalizedName:     tc.targetName,
				Price:              p,
				Discount:           15.0,
				PriceAfterDiscount: compare.CalculatePriceAfterDiscount(p, 15.0),
			}

			_ = repo.InsertFileRows(ctx, []*compare.CompareFileRow{sRow})
			_ = repo.InsertFileRows(ctx, []*compare.CompareFileRow{tRow})

			// 1. Head-to-head check
			h2hRes, err := svc.RunSupplierVsSupplierDetailed(ctx, compare.HeadToHeadFilter{
				SourceFileID: f1.ID,
				TargetFileID: f2.ID,
			})
			if err != nil {
				t.Fatalf("case %d head-to-head failed: %v", i, err)
			}
			if h2hRes.TotalShared > 0 {
				t.Errorf("expected 0 shared products for %q vs %q, got %d",
					tc.sourceName, tc.targetName, h2hRes.TotalShared)
			}

			// 2. Multi-supplier check
			multiRes, err := svc.RunMultiSupplierComparison(ctx, []int64{f1.ID, f2.ID})
			if err != nil {
				t.Fatalf("case %d multi-supplier failed: %v", i, err)
			}
			if multiRes.Summary.TotalProducts != 2 {
				t.Errorf("expected 2 distinct product groups in multi-supplier for %q vs %q, got %d",
					tc.sourceName, tc.targetName, multiRes.Summary.TotalProducts)
			}
		})
	}
}

// T087 [US6]: Strip vs box without a relationship => "not directly comparable" and excluded from savings.
func TestGrouping_StripVsBoxWithoutRelationship_NotDirectlyComparable(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	f1 := &compare.CompareFile{SupplierName: "المورد الأول", Status: compare.FileReady}
	f2 := &compare.CompareFile{SupplierName: "المورد الثاني", Status: compare.FileReady}
	_ = repo.CreateFile(ctx, f1)
	_ = repo.CreateFile(ctx, f2)

	p1 := money.FromMajor(15)
	p2 := money.FromMajor(30)

	// Catalog product 500 represents the drug identity (e.g. Congestal tablets)
	prodID := int64(500)

	// Supplier 1 sells strip of 10, Supplier 2 sells box of 20, no relationship defined
	sRow := &compare.CompareFileRow{
		FileID:             f1.ID,
		RowNumber:          1,
		RawName:            "كونجستال 10 اقراص شريط",
		NormalizedName:     "كونجستال 10 اقراص شريط",
		Price:              p1,
		Discount:           10.0,
		PriceAfterDiscount: compare.CalculatePriceAfterDiscount(p1, 10.0),
		MatchedProductID:   &prodID,
	}
	tRow := &compare.CompareFileRow{
		FileID:             f2.ID,
		RowNumber:          1,
		RawName:            "كونجستال 20 قرص علبة",
		NormalizedName:     "كونجستال 20 قرص علبة",
		Price:              p2,
		Discount:           15.0,
		PriceAfterDiscount: compare.CalculatePriceAfterDiscount(p2, 15.0),
		MatchedProductID:   &prodID,
	}

	_ = repo.InsertFileRows(ctx, []*compare.CompareFileRow{sRow})
	_ = repo.InsertFileRows(ctx, []*compare.CompareFileRow{tRow})

	res, err := svc.RunSupplierVsSupplierDetailed(ctx, compare.HeadToHeadFilter{
		SourceFileID: f1.ID,
		TargetFileID: f2.ID,
	})
	if err != nil {
		t.Fatalf("head-to-head failed: %v", err)
	}

	if res.TotalShared != 1 {
		t.Fatalf("expected 1 shared item, got %d", res.TotalShared)
	}

	row := res.Rows[0]
	if !row.NotComparable {
		t.Errorf("expected row to be marked NotComparable=true for strip vs box without relationship")
	}
	if row.GroupBasis != "not_comparable" {
		t.Errorf("expected GroupBasis='not_comparable', got %q", row.GroupBasis)
	}

	// Must be excluded from savings totals
	if !res.TotalSavings.IsZero() {
		t.Errorf("expected TotalSavings to be 0 for non-comparable pack differences, got %v", res.TotalSavings)
	}
}

// T087 [US6]: Two files with the same product absent from the catalogue => labelled source_equivalence, no catalogue link.
func TestGrouping_SameProductAbsentFromCatalog_SourceEquivalence(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	f1 := &compare.CompareFile{SupplierName: "مورد القاهرة", Status: compare.FileReady}
	f2 := &compare.CompareFile{SupplierName: "مورد الإسكندرية", Status: compare.FileReady}
	_ = repo.CreateFile(ctx, f1)
	_ = repo.CreateFile(ctx, f2)

	p := money.FromMajor(50)

	// Neither row has a matched product ID in the master catalogue
	sRow := &compare.CompareFileRow{
		FileID:             f1.ID,
		RowNumber:          1,
		RawName:            "كتافلام 50 مجم 20 قرص",
		NormalizedName:     "كتافلام 50 مجم 20 قرص",
		Price:              p,
		Discount:           15.0,
		PriceAfterDiscount: compare.CalculatePriceAfterDiscount(p, 15.0),
		MatchedProductID:   nil,
	}
	tRow := &compare.CompareFileRow{
		FileID:             f2.ID,
		RowNumber:          1,
		RawName:            "كتافلام 50 مجم 20 قرص",
		NormalizedName:     "كتافلام 50 مجم 20 قرص",
		Price:              p,
		Discount:           10.0,
		PriceAfterDiscount: compare.CalculatePriceAfterDiscount(p, 10.0),
		MatchedProductID:   nil,
	}

	_ = repo.InsertFileRows(ctx, []*compare.CompareFileRow{sRow})
	_ = repo.InsertFileRows(ctx, []*compare.CompareFileRow{tRow})

	// 1. Head-to-head check
	h2hRes, err := svc.RunSupplierVsSupplierDetailed(ctx, compare.HeadToHeadFilter{
		SourceFileID: f1.ID,
		TargetFileID: f2.ID,
	})
	if err != nil {
		t.Fatalf("head-to-head failed: %v", err)
	}

	if h2hRes.TotalShared != 1 {
		t.Fatalf("expected 1 shared item, got %d", h2hRes.TotalShared)
	}

	h2hRow := h2hRes.Rows[0]
	if h2hRow.ProductID != nil {
		t.Errorf("expected product ID to remain nil (no catalogue link), got %v", *h2hRow.ProductID)
	}
	if h2hRow.GroupBasis != "source_equivalence" {
		t.Errorf("expected GroupBasis='source_equivalence', got %q", h2hRow.GroupBasis)
	}
	if h2hRow.NotComparable {
		t.Errorf("expected NotComparable=false for identical sellable items")
	}

	// 2. Multi-supplier check
	multiRes, err := svc.RunMultiSupplierComparison(ctx, []int64{f1.ID, f2.ID})
	if err != nil {
		t.Fatalf("multi-supplier failed: %v", err)
	}

	if multiRes.Summary.TotalProducts != 1 {
		t.Fatalf("expected 1 aggregated product group, got %d", multiRes.Summary.TotalProducts)
	}

	multiRow := multiRes.Rows[0]
	if multiRow.MatchedProductID != nil {
		t.Errorf("expected multi-supplier MatchedProductID to remain nil, got %v", *multiRow.MatchedProductID)
	}
	if multiRow.GroupBasis != "source_equivalence" {
		t.Errorf("expected multi-supplier GroupBasis='source_equivalence', got %q", multiRow.GroupBasis)
	}
}

// T091 [US6]: Strip vs box with active pack relationship => convert unit prices only through productmatch.Convert.
// Assert exact money values without float comparisons.
func TestGrouping_StripVsBoxWithValidRelationship_ConvertedExactPrice(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	f1 := &compare.CompareFile{SupplierName: "المورد الأول", Status: compare.FileReady}
	f2 := &compare.CompareFile{SupplierName: "المورد الثاني", Status: compare.FileReady}
	_ = repo.CreateFile(ctx, f1)
	_ = repo.CreateFile(ctx, f2)

	prodID := int64(700)
	// Active pack relationship: 1 box = 2 strips (Factor 2)
	repo.packRels = []productmatch.PackRelationship{
		{
			ID:        101,
			ProductID: prodID,
			FromUnit:  "علبة",
			ToUnit:    "شريط",
			Factor:    2,
		},
	}

	p1 := money.FromMajor(50) // 50 EGP for 1 box
	p2 := money.FromMajor(30) // 30 EGP for 1 strip

	// Supplier 1 sells 1 box with 10% discount => net 45.00 EGP
	sRow := &compare.CompareFileRow{
		FileID:             f1.ID,
		RowNumber:          1,
		RawName:            "كونجستال 20 قرص علبة",
		NormalizedName:     "كونجستال 20 قرص علبة",
		Price:              p1,
		Discount:           10.0,
		PriceAfterDiscount: compare.CalculatePriceAfterDiscount(p1, 10.0),
		MatchedProductID:   &prodID,
	}
	// Supplier 2 sells 1 strip with 20% discount => net 24.00 EGP per strip
	// Converted to box (2 strips) => 24.00 * 2 = 48.00 EGP
	tRow := &compare.CompareFileRow{
		FileID:             f2.ID,
		RowNumber:          1,
		RawName:            "كونجستال 10 اقراص شريط",
		NormalizedName:     "كونجستال 10 اقراص شريط",
		Price:              p2,
		Discount:           20.0,
		PriceAfterDiscount: compare.CalculatePriceAfterDiscount(p2, 20.0),
		MatchedProductID:   &prodID,
	}

	_ = repo.InsertFileRows(ctx, []*compare.CompareFileRow{sRow})
	_ = repo.InsertFileRows(ctx, []*compare.CompareFileRow{tRow})

	// 1. Head-to-Head test
	res, err := svc.RunSupplierVsSupplierDetailed(ctx, compare.HeadToHeadFilter{
		SourceFileID: f1.ID,
		TargetFileID: f2.ID,
	})
	if err != nil {
		t.Fatalf("head-to-head failed: %v", err)
	}

	if res.TotalShared != 1 {
		t.Fatalf("expected 1 shared item, got %d", res.TotalShared)
	}

	row := res.Rows[0]
	if row.NotComparable {
		t.Errorf("expected NotComparable=false with valid relationship")
	}

	expectedYourNet := money.FromMajor(45)                     // 45.00 EGP
	expectedCompNet := money.FromMajor(48)                     // 24.00 * 2 = 48.00 EGP
	expectedSavings, _ := expectedCompNet.Sub(expectedYourNet) // 3.00 EGP

	if row.YourNetPrice != expectedYourNet {
		t.Errorf("expected YourNetPrice=%v, got %v", expectedYourNet, row.YourNetPrice)
	}
	if row.CompetitorNetPrice != expectedCompNet {
		t.Errorf("expected CompetitorNetPrice=%v, got %v", expectedCompNet, row.CompetitorNetPrice)
	}
	if row.SavingsDifference != expectedSavings {
		t.Errorf("expected SavingsDifference=%v, got %v", expectedSavings, row.SavingsDifference)
	}
	if res.TotalSavings != expectedSavings {
		t.Errorf("expected TotalSavings=%v, got %v", expectedSavings, res.TotalSavings)
	}

	// 2. Multi-supplier test
	multiRes, err := svc.RunMultiSupplierComparison(ctx, []int64{f1.ID, f2.ID})
	if err != nil {
		t.Fatalf("multi-supplier failed: %v", err)
	}
	if multiRes.Summary.TotalProducts != 1 {
		t.Fatalf("expected 1 product group, got %d", multiRes.Summary.TotalProducts)
	}
	if multiRes.Rows[0].NotComparable {
		t.Errorf("expected multi-supplier row to be comparable")
	}
}
