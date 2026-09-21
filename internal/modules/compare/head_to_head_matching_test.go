package compare_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestHeadToHead_EnhancedMatching(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	// Source File: Supplier A (سينا فارم)
	f1 := &compare.CompareFile{
		SupplierName: "مخزن سينا فارم",
		Status:       compare.FileReady,
	}
	_ = repo.CreateFile(ctx, f1)

	// Target File: Supplier B (طلعت فارما)
	f2 := &compare.CompareFile{
		SupplierName: "طلعت فارما",
		Status:       compare.FileReady,
	}
	_ = repo.CreateFile(ctx, f2)

	// Test Cases: pairs of real Egyptian pharmaceutical product names that must match
	positivePairs := []struct {
		sourceName string
		targetName string
		price      int64 // in EGP major units
	}{
		// 1. Pack count omitted in one supplier's ERP
		{"كتافلام 50 مجم 20 قرص", "كتافلام 50 مجم", 50},
		// 2. Pack count with word vs bare number
		{"بانادول اكسترا 24 قرص", "بانادول اكسترا 24", 45},
		// 3. Form word vs pack count
		{"كونجستال 20 قرص", "كونجستال اقراص", 30},
		// 4. Augmentin 1g with pack count omitted in one
		{"اوجمنتين 1 جم 14 قرص", "اوجمنتين 1 جم", 110},
		// 5. 3-consonant skeleton drug cross-script (Mobic: mbk)
		{"Mobic 15mg", "موبيك 15 مجم", 45},
		// 6. 3-consonant skeleton drug cross-script (Curam: krm)
		{"Curam 1gm", "كيورام 1 جم", 95},
		// 7. 3-consonant skeleton drug cross-script (Tareg: trg)
		{"Tareg 80mg", "تارج 80 مجم", 80},
		// 8. 3-consonant skeleton drug cross-script (Daonil: dnl)
		{"Daonil 5mg", "داونيل 5 مجم", 25},
		// 9. Manufacturer noise word in row
		{"فولتارين 100 مجم (نوفارتس)", "فولتارين 100 مجم", 60},
		// 10. Marketing noise word in row
		{"بروفين 400 مجم سعر جديد", "بروفين 400 مجم", 40},
	}

	var sourceRows []*compare.CompareFileRow
	var targetRows []*compare.CompareFileRow

	for i, pair := range positivePairs {
		rowNum := i + 1
		p := money.FromMajor(pair.price)
		sourceRows = append(sourceRows, &compare.CompareFileRow{
			FileID:             f1.ID,
			RowNumber:          rowNum,
			RawName:            pair.sourceName,
			NormalizedName:     pair.sourceName,
			SKU:                "", // No SKU overlap to simulate real supplier files
			Price:              p,
			Discount:           20.0,
			PriceAfterDiscount: compare.CalculatePriceAfterDiscount(p, 20.0),
		})
		targetRows = append(targetRows, &compare.CompareFileRow{
			FileID:             f2.ID,
			RowNumber:          rowNum,
			RawName:            pair.targetName,
			NormalizedName:     pair.targetName,
			SKU:                "",
			Price:              p,
			Discount:           15.0,
			PriceAfterDiscount: compare.CalculatePriceAfterDiscount(p, 15.0),
		})
	}

	_ = repo.InsertFileRows(ctx, sourceRows)
	_ = repo.InsertFileRows(ctx, targetRows)

	res, err := svc.RunSupplierVsSupplierDetailed(ctx, compare.HeadToHeadFilter{
		SourceFileID: f1.ID,
		TargetFileID: f2.ID,
	})
	if err != nil {
		t.Fatalf("failed to run head-to-head comparison: %v", err)
	}

	if res.TotalShared != len(positivePairs) {
		t.Errorf("expected %d shared products matched, got %d", len(positivePairs), res.TotalShared)
		for _, r := range res.Rows {
			t.Logf("Matched: %s", r.ProductName)
		}
	}
}

func TestHeadToHead_SafetyGuarantees_NeverFalseMatch(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	f1 := &compare.CompareFile{SupplierName: "المصدر", Status: compare.FileReady}
	_ = repo.CreateFile(ctx, f1)
	f2 := &compare.CompareFile{SupplierName: "المنافس", Status: compare.FileReady}
	_ = repo.CreateFile(ctx, f2)

	// Negative Test Cases: products that MUST NEVER MATCH
	negativePairs := []struct {
		sourceName string
		targetName string
		sPrice     int64
		tPrice     int64
		reason     string
	}{
		// 1. Different doses
		{"اتاكاند 16 مجم", "اتاكاند 32 مجم", 50, 75, "different strength"},
		// 2. Different line extensions
		{"بانادول 24 قرص", "بانادول اكسترا 24 قرص", 30, 45, "different modifier"},
		// 3. Different identity letters
		{"بتنوفيت ان كريم", "بتنوفيت سي كريم", 20, 22, "different letter"},
		// 4. Conflicting pack counts (20 vs 200)
		{"اليرجيل 4 مجم 20 قرص", "اليرجيل 4 مجم 200 قرص", 15, 120, "conflicting pack count"},
		// 5. Omitted pack count with 8x price disparity (indicating different pack)
		{"اليرجيل 4 مجم 20 قرص", "اليرجيل 4 مجم", 15, 120, "omitted pack count with 8x price difference"},
	}

	for i, pair := range negativePairs {
		// Run each negative test in isolation to ensure zero cross-matching
		sRow := &compare.CompareFileRow{
			FileID:             f1.ID,
			RowNumber:          1,
			RawName:            pair.sourceName,
			NormalizedName:     pair.sourceName,
			Price:              money.FromMajor(pair.sPrice),
			Discount:           10.0,
			PriceAfterDiscount: money.FromMajor(pair.sPrice),
		}
		tRow := &compare.CompareFileRow{
			FileID:             f2.ID,
			RowNumber:          1,
			RawName:            pair.targetName,
			NormalizedName:     pair.targetName,
			Price:              money.FromMajor(pair.tPrice),
			Discount:           10.0,
			PriceAfterDiscount: money.FromMajor(pair.tPrice),
		}

		_ = repo.DeleteFileRows(ctx, f1.ID)
		_ = repo.DeleteFileRows(ctx, f2.ID)
		_ = repo.InsertFileRows(ctx, []*compare.CompareFileRow{sRow})
		_ = repo.InsertFileRows(ctx, []*compare.CompareFileRow{tRow})

		res, err := svc.RunSupplierVsSupplierDetailed(ctx, compare.HeadToHeadFilter{
			SourceFileID: f1.ID,
			TargetFileID: f2.ID,
		})
		if err != nil {
			t.Fatalf("case %d (%s) failed: %v", i, pair.reason, err)
		}

		if res.TotalShared > 0 {
			t.Errorf("SAFETY VIOLATION in case %d (%s): %q matched with %q!",
				i, pair.reason, pair.sourceName, pair.targetName)
		}
	}
}
