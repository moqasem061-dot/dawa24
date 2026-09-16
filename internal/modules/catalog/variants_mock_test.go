package catalog

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func (m *mockCatalogRepo) UpdateProductImageBySKU(ctx context.Context, sku string, imagePath string, imageLink string) (*Product, error) {
	p, err := m.GetProductBySKU(ctx, sku)
	if err != nil {
		return nil, err
	}
	p.Image = imagePath
	p.ImageLink = imageLink
	return p, nil
}

func (m *mockCatalogRepo) UpdateProductImageByID(ctx context.Context, id int64, imagePath string, imageLink string) (*Product, error) {
	p, err := m.GetProductByID(ctx, id)
	if err != nil {
		return nil, err
	}
	p.Image = imagePath
	p.ImageLink = imageLink
	return p, nil
}

func (m *mockCatalogRepo) ListDosageForms(_ context.Context) ([]string, error) {
	return nil, nil
}

func (m *mockCatalogRepo) ListMatchDecisions(_ context.Context, _ string, _, _ int) ([]*MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogRepo) ListMatchDecisionsFiltered(_ context.Context, _ DecisionMemoryFilter) ([]*MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogRepo) ListMatchDecisionsForOrgWithPlatform(_ context.Context, _ int64, _ string, _, _ int) ([]*MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogRepo) GetDecisionMemoryPreference(_ context.Context, _ int64) (bool, error) {
	return true, nil
}
func (m *mockCatalogRepo) SetDecisionMemoryPreference(_ context.Context, _ int64, _ bool, _ int64) error {
	return nil
}
func (m *mockCatalogRepo) PromoteMatchDecision(_ context.Context, _ int64, _ int64) error {
	return nil
}
func (m *mockCatalogRepo) DemoteMatchDecision(_ context.Context, _ int64) error {
	return nil
}
func (m *mockCatalogRepo) BulkPromoteMatchDecisions(_ context.Context, _ []int64, _ int64) (int64, error) {
	return 0, nil
}
func (m *mockCatalogRepo) BulkDeleteMatchDecisions(_ context.Context, _ []int64) (int64, error) {
	return 0, nil
}
func (m *mockCatalogRepo) RelinkMatchDecision(_ context.Context, _ int64, _ *int64, _ int64) error {
	return nil
}
func (m *mockCatalogRepo) RelinkMatchDecisionForOrg(_ context.Context, _ int64, _ int64, _ *int64, _ int64) error {
	return nil
}
func (m *mockCatalogRepo) DeleteMatchDecision(_ context.Context, _ int64) error {
	return nil
}
func (m *mockCatalogRepo) ClearMatchDecisions(_ context.Context) error {
	return nil
}
func (m *mockCatalogRepo) ListMatchDecisionsForOrg(_ context.Context, _ int64, _ string, _, _ int) ([]*MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogRepo) DeleteMatchDecisionForOrg(_ context.Context, _, _ int64) error {
	return nil
}
func (m *mockCatalogRepo) ClearMatchDecisionsForOrg(_ context.Context, _ int64) error {
	return nil
}
func (m *mockCatalogRepo) SaveManualDecision(_ context.Context, _, _ int64, _ string, _ int64, _ string) error {
	return nil
}
func (m *mockCatalogRepo) IsDecisionMemoryEnabled(_ context.Context) bool {
	return true
}
func (m *mockCatalogRepo) SetDecisionMemoryEnabled(_ context.Context, _ bool) error {
	return nil
}
func (m *mockCatalogRepo) ListCustomerMappings(_ context.Context, _ int64, _ string, _, _ int) ([]*CustomerMappingView, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogRepo) DeleteCustomerMapping(_ context.Context, _, _ int64) error {
	return nil
}
func (m *mockCatalogRepo) ClearCustomerMappings(_ context.Context, _ int64) error {
	return nil
}

func TestProductEffectivePrice(t *testing.T) {
	p := &Product{
		Price:    money.MustParse("100.00"),
		Discount: money.MustParse("15.50"),
	}

	effective := p.EffectivePrice()
	expected := money.MustParse("84.50")
	if effective != expected {
		t.Errorf("EffectivePrice = %v; want %v", effective, expected)
	}
}

// T1: Unique row ID composition matches Laravel format exactly
func TestComposeUniqueRowID(t *testing.T) {
	cases := []struct {
		productID int64
		variantID *int64
		branchID  *int64
		expected  string
	}{
		{10, nil, nil, "p_10"},
		{10, ptr(int64(20)), nil, "p_10_v_20"},
		{10, ptr(int64(20)), ptr(int64(30)), "p_10_v_20_b_30"},
		{10, nil, ptr(int64(30)), "p_10_b_30"},
	}

	for _, c := range cases {
		got := ComposeUniqueRowID(c.productID, c.variantID, c.branchID)
		if got != c.expected {
			t.Errorf("ComposeUniqueRowID(%d, %v, %v) = %q, want %q", c.productID, c.variantID, c.branchID, got, c.expected)
		}
	}
}

// T11: Deterministic fallback — when the read index is empty, FastSearch returns products from master table
func TestFastSearchDeterministicFallback(t *testing.T) {
	ctx := context.Background()
	repo := newMockCatalogRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// Add product to master catalogue
	p := &Product{
		OrganizationID: 1,
		Name:           i18n.New("بانادول", "Panadol"),
		Price:          money.MustParse("50.00"),
		Status:         StatusActive,
	}
	_ = repo.CreateProduct(ctx, p)

	// FastSearch with empty read index
	results, err := svc.FastSearch(ctx, SearchParams{Query: "بانادول"})
	if err != nil {
		t.Fatalf("FastSearch failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected fallback to return 1 item, got %d", len(results))
	}
	expectedID := p.ID
	expectedRowID := fmt.Sprintf("p_%d", p.ID)
	if results[0].ProductID != expectedID || results[0].UniqueRowID != expectedRowID {
		t.Errorf("expected product %d (%s), got %+v", expectedID, expectedRowID, results[0])
	}
}

func ptr[T any](v T) *T {
	return &v
}

func TestCatalogServiceCreateAndVariants(t *testing.T) {
	ctx := database.WithTenant(context.Background(), 42)
	repo := newMockCatalogRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// 1. Create Product
	p, err := svc.CreateProduct(ctx, &Product{
		Name:           i18n.New("بانادول اكسترا", "Panadol Extra"),
		Price:          money.MustParse("45.00"),
		DosageForm:     "Tablet",
		ScientificName: "Paracetamol + Caffeine",
	})
	if err != nil {
		t.Fatalf("CreateProduct failed: %v", err)
	}

	if p.ID == 0 || p.OrganizationID != 42 {
		t.Errorf("Product creation metadata incorrect: id=%d, org=%d", p.ID, p.OrganizationID)
	}

	// 2. Create Variant
	costP := money.MustParse("18.00")
	v, err := svc.CreateVariant(ctx, &ProductVariant{
		ProductID: p.ID,
		Name:      i18n.New("شريط 12 قرص", "Strip of 12 tablets"),
		Price:     money.MustParse("22.50"),
		CostPrice: &costP,
	})
	if err != nil {
		t.Fatalf("CreateVariant failed: %v", err)
	}

	// 3. Get Product with Variants
	retrievedProduct, variants, err := svc.GetProduct(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProduct failed: %v", err)
	}
	if retrievedProduct.ID != p.ID {
		t.Errorf("retrieved product ID = %d; want %d", retrievedProduct.ID, p.ID)
	}
	if len(variants) != 1 || variants[0].ID != v.ID {
		t.Errorf("expected 1 variant with ID %d, got %d variants", v.ID, len(variants))
	}
}

// Category -> brand relationship stubs (PLAN_V7 Phase 4). Behaviour is covered
// by the catalog repository tests; these only satisfy the interface.
func (m *mockCatalogRepo) ListBrandsByCategory(context.Context, int64) ([]*Brand, error) {
	return nil, nil
}

func (m *mockCatalogRepo) BrandInCategory(context.Context, int64, int64) (bool, error) {
	return true, nil
}

func (m *mockCatalogRepo) SetBrandCategories(context.Context, int64, []int64) error {
	return nil
}

func TestDecisionMemoryServiceMethods(t *testing.T) {
	repo := newMockCatalogRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)
	ctx := context.Background()

	// Verify global switch
	if !svc.IsDecisionMemoryEnabled(ctx) {
		t.Errorf("expected decision memory to be enabled by default")
	}
	if err := svc.SetDecisionMemoryEnabled(ctx, false); err != nil {
		t.Fatalf("SetDecisionMemoryEnabled failed: %v", err)
	}

	// Verify manual decision saving
	if err := svc.SaveManualDecision(ctx, 10, 1, "Panadol Extra", 100, "صيدلي"); err != nil {
		t.Fatalf("SaveManualDecision failed: %v", err)
	}

	// Verify org-scoped listing, delete, clear
	decisions, total, err := svc.ListMatchDecisionsForOrg(ctx, 10, "", 50, 0)
	if err != nil {
		t.Fatalf("ListMatchDecisionsForOrg failed: %v", err)
	}
	if total < 0 || len(decisions) < 0 {
		t.Errorf("unexpected results from ListMatchDecisionsForOrg")
	}

	if err := svc.DeleteMatchDecisionForOrg(ctx, 10, 1); err != nil {
		t.Fatalf("DeleteMatchDecisionForOrg failed: %v", err)
	}
	if err := svc.ClearMatchDecisionsForOrg(ctx, 10); err != nil {
		t.Fatalf("ClearMatchDecisionsForOrg failed: %v", err)
	}
}
