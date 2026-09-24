package compare_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

func TestSearchAcrossSuppliersAndCatalog_PublicPriceAndDiscount(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	userID := int64(10)

	// 1. Create an active compare file for a supplier
	file := &compare.CompareFile{
		UserID:       userID,
		SupplierName: "المتحدة للأدوية",
		Status:       compare.FileReady,
	}
	if err := repo.CreateFile(ctx, file); err != nil {
		t.Fatalf("create file failed: %v", err)
	}

	// 2. Add a row with public price (سعر الجمهور = 50.00 EGP) and discount (الخصم = 20.0%)
	publicPrice := money.FromMajor(50)
	netPrice := compare.CalculatePriceAfterDiscount(publicPrice, 20.0) // 40.00 EGP
	prodID := int64(45)

	rows := []*compare.CompareFileRow{
		{
			FileID:             file.ID,
			RowNumber:          1,
			RawName:            "بانادول اكسترا 24 قرص",
			NormalizedName:     "بانادول اكسترا 24 قرص",
			SKU:                "1001",
			Price:              publicPrice,
			Discount:           20.0,
			PriceAfterDiscount: netPrice,
			MatchedProductID:   &prodID,
		},
	}
	if err := repo.InsertFileRows(ctx, rows); err != nil {
		t.Fatalf("insert rows failed: %v", err)
	}

	// 3. Perform search
	res, err := svc.SearchAcrossSuppliersAndCatalog(ctx, userID, nil, "بانادول")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(res.Items) == 0 {
		t.Fatalf("expected at least 1 result, got 0")
	}

	item := res.Items[0]
	if item.BestPrice != publicPrice {
		t.Errorf("expected BestPrice (سعر الجمهور) %v, got %v", publicPrice, item.BestPrice)
	}
	if item.BestDiscount != 20.0 {
		t.Errorf("expected BestDiscount (الخصم) 20.0%%, got %v", item.BestDiscount)
	}
	if item.BestNetPrice != netPrice {
		t.Errorf("expected BestNetPrice (الصافي) %v, got %v", netPrice, item.BestNetPrice)
	}
	if item.BestSupplier != "المتحدة للأدوية" {
		t.Errorf("expected BestSupplier 'المتحدة للأدوية', got %q", item.BestSupplier)
	}

	// 4. Verify offers
	offer, ok := item.Offers["المتحدة للأدوية"]
	if !ok {
		t.Fatalf("expected offer from 'المتحدة للأدوية'")
	}
	if offer.Price != publicPrice {
		t.Errorf("expected offer Price %v, got %v", publicPrice, offer.Price)
	}
	if offer.Discount != 20.0 {
		t.Errorf("expected offer Discount 20.0%%, got %v", offer.Discount)
	}
	if offer.PriceAfterDiscount != netPrice {
		t.Errorf("expected offer PriceAfterDiscount %v, got %v", netPrice, offer.PriceAfterDiscount)
	}
}

func TestSearchAcrossSuppliersAndCatalog_DerivedPublicPrice(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	userID := int64(10)

	file := &compare.CompareFile{
		UserID:       userID,
		SupplierName: "مخزن سينا فارم تريد مدينه نصر",
		Status:       compare.FileReady,
	}
	if err := repo.CreateFile(ctx, file); err != nil {
		t.Fatalf("create file failed: %v", err)
	}

	// Net price = 15.87, Discount = 31.0%, Price = Zero
	netPrice := money.MustParse("15.87")
	rows := []*compare.CompareFileRow{
		{
			FileID:             file.ID,
			RowNumber:          1,
			RawName:            "دوكسيدوكس 100 مجم 10 كبسولة",
			NormalizedName:     "دوكسيدوكس 100 مجم 10 كبسولة",
			SKU:                "2464",
			Price:              money.Zero,
			Discount:           31.0,
			PriceAfterDiscount: netPrice,
		},
	}
	if err := repo.InsertFileRows(ctx, rows); err != nil {
		t.Fatalf("insert rows failed: %v", err)
	}

	res, err := svc.SearchAcrossSuppliersAndCatalog(ctx, userID, nil, "دوكسيدوكس")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(res.Items) == 0 {
		t.Fatalf("expected at least 1 result, got 0")
	}

	item := res.Items[0]
	// 15.87 / (1 - 0.31) = 23.00 EGP
	expectedPublicPrice := money.MustParse("23.00")
	if item.BestPrice != expectedPublicPrice {
		t.Errorf("expected derived BestPrice %v, got %v", expectedPublicPrice, item.BestPrice)
	}
	if item.BestDiscount != 31.0 {
		t.Errorf("expected BestDiscount 31.0%%, got %v", item.BestDiscount)
	}
	if item.BestNetPrice != netPrice {
		t.Errorf("expected BestNetPrice %v, got %v", netPrice, item.BestNetPrice)
	}
}
