package catalog

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestCatalogServiceCategoriesAndBrands(t *testing.T) {
	ctx := database.WithTenant(context.Background(), 42)
	repo := newMockCatalogRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// Categories
	cat := &Category{
		Name: i18n.New("مسكنات", "Painkillers"),
	}
	createdCat, err := svc.CreateCategory(ctx, cat)
	if err != nil {
		t.Fatalf("CreateCategory failed: %v", err)
	}

	gotCat, err := svc.GetCategory(ctx, createdCat.ID)
	if err != nil {
		t.Fatalf("GetCategory failed: %v", err)
	}
	if gotCat.ID != createdCat.ID {
		t.Errorf("got cat id %d, want %d", gotCat.ID, createdCat.ID)
	}

	cats, err := svc.ListCategories(ctx)
	if err != nil || len(cats) != 1 {
		t.Fatalf("ListCategories failed: %v", err)
	}

	// Brands
	brand := &Brand{
		Name: i18n.New("فايزر", "Pfizer"),
	}
	createdBrand, err := svc.CreateBrand(ctx, brand)
	if err != nil {
		t.Fatalf("CreateBrand failed: %v", err)
	}

	gotBrand, err := svc.GetBrand(ctx, createdBrand.ID)
	if err != nil {
		t.Fatalf("GetBrand failed: %v", err)
	}
	if gotBrand.ID != createdBrand.ID {
		t.Errorf("got brand id %d, want %d", gotBrand.ID, createdBrand.ID)
	}

	brands, err := svc.ListBrands(ctx)
	if err != nil || len(brands) != 1 {
		t.Fatalf("ListBrands failed: %v", err)
	}

	// Delete category & brand when empty
	if err := svc.DeleteCategory(ctx, createdCat.ID); err != nil {
		t.Fatalf("DeleteCategory failed: %v", err)
	}
	if err := svc.DeleteBrand(ctx, createdBrand.ID); err != nil {
		t.Fatalf("DeleteBrand failed: %v", err)
	}
}

func TestCatalogServiceVariantValidationAndLifecycle(t *testing.T) {
	ctx := database.WithTenant(context.Background(), 42)
	repo := newMockCatalogRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	p, _ := svc.CreateProduct(ctx, &Product{
		Name:  i18n.New("كونجستال", "Congestal"),
		Price: money.MustParse("30.00"),
	})

	v, _ := svc.CreateVariant(ctx, &ProductVariant{
		ProductID: p.ID,
		Name:      i18n.New("علبة 20 قرص", "Box 20 tabs"),
		Price:     money.MustParse("30.00"),
	})

	// 1. Discount exceeds 100% should be rejected
	vInvalid := &ProductVariant{
		Name:     i18n.New("علبة 20 قرص", "Box 20 tabs"),
		Price:    money.MustParse("30.00"),
		Discount: money.MustParse("105.00"), // 105% > 100%!
	}
	_, err := svc.UpdateVariant(ctx, v.ID, vInvalid)
	if err == nil {
		t.Fatal("expected error when discount exceeds 100%, got nil")
	}

	// 2. Valid update
	vValid := &ProductVariant{
		Name:     i18n.New("علبة 20 قرص", "Box 20 tabs"),
		Price:    money.MustParse("30.00"),
		Discount: money.MustParse("5.00"),
	}
	updated, err := svc.UpdateVariant(ctx, v.ID, vValid)
	if err != nil {
		t.Fatalf("UpdateVariant valid failed: %v", err)
	}
	if updated.Discount != money.MustParse("5.00") {
		t.Errorf("got discount %v, want 5.00", updated.Discount)
	}

	// 3. Delete Variant
	if err := svc.DeleteVariant(ctx, v.ID); err != nil {
		t.Fatalf("DeleteVariant failed: %v", err)
	}
}

func TestCatalogServiceCustomerPricingAndAlerts(t *testing.T) {
	ctx := database.WithTenant(context.Background(), 42)
	repo := newMockCatalogRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// Customer Pricing
	cpPrice, _ := money.Parse("38.00")
	customerOrgID := int64(100)
	cp := &CustomerProductMapping{
		OrganizationID: 42,
		CustomerOrgID:  &customerOrgID,
		ProductID:      1,
		Price:          cpPrice,
	}
	if err := svc.SetCustomerPricing(ctx, cp); err != nil {
		t.Fatalf("SetCustomerPricing failed: %v", err)
	}

	gotCP, err := svc.GetCustomerPricing(ctx, 42, 100, 1)
	if err != nil {
		t.Fatalf("GetCustomerPricing failed: %v", err)
	}
	if gotCP.Price != cpPrice {
		t.Errorf("got custom price %v, want %v", gotCP.Price, cpPrice)
	}

	// Alerts
	alert := &ProductAlert{
		UserID:    500,
		ProductID: 1,
		AlertType: "price_drop",
	}
	createdAlert, err := svc.CreateProductAlert(ctx, alert)
	if err != nil {
		t.Fatalf("CreateProductAlert failed: %v", err)
	}
	if createdAlert.ID <= 0 {
		t.Fatalf("expected positive alert ID, got %d", createdAlert.ID)
	}

	alerts, err := svc.ListProductAlerts(ctx, 500)
	if err != nil || len(alerts) != 1 {
		t.Fatalf("ListProductAlerts failed: %v", err)
	}
}

func (m *mockCatalogRepo) CountProductsByOrg(_ context.Context, _ int64, _ string) (int, error) {
	return 0, nil
}
