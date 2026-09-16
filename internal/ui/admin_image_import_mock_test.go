package ui

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type mockCatalogImageRepo struct {
	products map[string]*catalog.Product
}

func newMockCatalogImageRepo() *mockCatalogImageRepo {
	return &mockCatalogImageRepo{
		products: make(map[string]*catalog.Product),
	}
}

func (m *mockCatalogImageRepo) CreateProduct(_ context.Context, p *catalog.Product) error {
	m.products[p.SKU] = p
	return nil
}
func (m *mockCatalogImageRepo) BulkUpsertProducts(_ context.Context, _ []*catalog.Product, _ catalog.BulkWriteOptions) (catalog.BulkWriteResult, error) {
	return catalog.BulkWriteResult{}, nil
}
func (m *mockCatalogImageRepo) GetProductByID(_ context.Context, _ int64) (*catalog.Product, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) UpdateProduct(_ context.Context, p *catalog.Product) error {
	m.products[p.SKU] = p
	return nil
}
func (m *mockCatalogImageRepo) DeleteProduct(_ context.Context, _ int64) error { return nil }
func (m *mockCatalogImageRepo) SearchProducts(_ context.Context, _ catalog.SearchParams) ([]*catalog.Product, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) CountProducts(_ context.Context, _ catalog.SearchParams) (int, error) {
	return 0, nil
}
func (m *mockCatalogImageRepo) ListProducts(_ context.Context, _ string, _, _ int) ([]*catalog.Product, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) SetProductsStatus(_ context.Context, _ []int64, _ catalog.ProductStatus) (int64, error) {
	return 0, nil
}
func (m *mockCatalogImageRepo) CreateVariant(_ context.Context, _ *catalog.ProductVariant) error {
	return nil
}
func (m *mockCatalogImageRepo) GetVariantByID(_ context.Context, _ int64) (*catalog.ProductVariant, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) GetVariantBySKUOrBarcode(_ context.Context, _ int64, _, _ string) (*catalog.ProductVariant, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) GetVariantByProductAndOrg(_ context.Context, _, _ int64) (*catalog.ProductVariant, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) ListVariantsByProduct(_ context.Context, _ int64) ([]*catalog.ProductVariant, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) ListVariantsByProducts(_ context.Context, _ []int64) ([]*catalog.ProductVariant, error) {
	return nil, nil
}

func (m *mockCatalogImageRepo) GetVariantsByIDs(context.Context, []int64) (map[int64]*catalog.ProductVariant, error) {
	return map[int64]*catalog.ProductVariant{}, nil
}
func (m *mockCatalogImageRepo) ListVariantsByOrganization(_ context.Context, _ int64, _ catalog.VariantSearchParams) ([]*catalog.ProductVariant, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogImageRepo) ListAllVariants(_ context.Context, _ catalog.VariantSearchParams) ([]*catalog.ProductVariant, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogImageRepo) UpdateVariant(_ context.Context, _ *catalog.ProductVariant) error {
	return nil
}
func (m *mockCatalogImageRepo) DeleteVariant(_ context.Context, _ int64) error { return nil }
func (m *mockCatalogImageRepo) ToggleVariantStatus(_ context.Context, _, _ int64) (catalog.ProductStatus, error) {
	return catalog.StatusActive, nil
}
func (m *mockCatalogImageRepo) DeleteAllVariantsByOrg(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}
func (m *mockCatalogImageRepo) ActivateAllVariantsByOrg(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}
func (m *mockCatalogImageRepo) DeleteAllProducts(_ context.Context) (int64, error) { return 0, nil }
func (m *mockCatalogImageRepo) CreateCategory(_ context.Context, _ *catalog.Category) error {
	return nil
}
func (m *mockCatalogImageRepo) GetCategoryByID(_ context.Context, _ int64) (*catalog.Category, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) UpdateCategory(_ context.Context, _ *catalog.Category) error {
	return nil
}
func (m *mockCatalogImageRepo) DeleteCategory(_ context.Context, _ int64) error { return nil }
func (m *mockCatalogImageRepo) ListCategories(_ context.Context) ([]*catalog.Category, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) ListCategoriesWithProductCount(_ context.Context, _ string, _ string, _ int, _ int) ([]*catalog.CategoryWithCount, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogImageRepo) CountProductsByOrg(_ context.Context, _ int64, _ string) (int, error) {
	return 0, nil
}
func (m *mockCatalogImageRepo) CountProductsInCategory(_ context.Context, _ int64) (int, error) {
	return 0, nil
}
func (m *mockCatalogImageRepo) CreateBrand(_ context.Context, _ *catalog.Brand) error { return nil }
func (m *mockCatalogImageRepo) GetBrandByID(_ context.Context, _ int64) (*catalog.Brand, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) UpdateBrand(_ context.Context, _ *catalog.Brand) error { return nil }
func (m *mockCatalogImageRepo) DeleteBrand(_ context.Context, _ int64) error          { return nil }
func (m *mockCatalogImageRepo) ListBrandsByCategory(_ context.Context, _ int64) ([]*catalog.Brand, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) BrandInCategory(_ context.Context, _, _ int64) (bool, error) {
	return false, nil
}
func (m *mockCatalogImageRepo) SetBrandCategories(_ context.Context, _ int64, _ []int64) error {
	return nil
}
func (m *mockCatalogImageRepo) ListDosageForms(_ context.Context) ([]string, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) ListBrands(_ context.Context) ([]*catalog.Brand, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) ListBrandsWithProductCount(_ context.Context, _, _ string, _, _ int) ([]*catalog.BrandWithCount, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogImageRepo) CountProductsInBrand(_ context.Context, _ int64) (int, error) {
	return 0, nil
}
func (m *mockCatalogImageRepo) SetCustomerPricing(_ context.Context, _ *catalog.CustomerProductMapping) error {
	return nil
}
func (m *mockCatalogImageRepo) GetCustomerPricing(_ context.Context, _, _, _ int64) (*catalog.CustomerProductMapping, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) CreateProductAlert(_ context.Context, _ *catalog.ProductAlert) error {
	return nil
}
func (m *mockCatalogImageRepo) ListProductAlertsByUser(_ context.Context, _ int64) ([]*catalog.ProductAlert, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) UpsertProductIndex(_ context.Context, _ *catalog.ProductIndexItem) error {
	return nil
}
func (m *mockCatalogImageRepo) DeleteProductIndex(_ context.Context, _ string) error { return nil }
func (m *mockCatalogImageRepo) DeleteProductIndexByProduct(_ context.Context, _ int64) error {
	return nil
}
func (m *mockCatalogImageRepo) SearchProductIndex(_ context.Context, _ catalog.SearchParams) ([]*catalog.ProductIndexItem, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) RebuildProductIndex(_ context.Context) (int64, error) { return 0, nil }
func (m *mockCatalogImageRepo) CreateSavingProduct(_ context.Context, _ *catalog.SavingProduct) error {
	return nil
}
func (m *mockCatalogImageRepo) UpdateSavingProduct(_ context.Context, _ *catalog.SavingProduct) error {
	return nil
}
func (m *mockCatalogImageRepo) ListSavingProductsByOrg(_ context.Context, _ int64, _, _ int) ([]*catalog.SavingProduct, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) ListSavingProductsEnriched(_ context.Context, _ int64, _, _ string, _, _ int) ([]*catalog.SavingProductEnriched, *catalog.SavingProductStats, error) {
	return nil, nil, nil
}
func (m *mockCatalogImageRepo) GetSavingProductByID(_ context.Context, _ int64) (*catalog.SavingProduct, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) DeleteSavingProduct(_ context.Context, _, _ int64) error  { return nil }
func (m *mockCatalogImageRepo) DeleteAllSavingProducts(_ context.Context, _ int64) error { return nil }
func (m *mockCatalogImageRepo) GetProductProviders(_ context.Context, _ int64) ([]*catalog.ProductProviderInfo, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) BatchUpsertSavingProducts(_ context.Context, _ int64, _ *int64, _ []*catalog.SavingProduct) (int, int, error) {
	return 0, 0, nil
}
func (m *mockCatalogImageRepo) ListAllSavingProductsAdmin(_ context.Context, _ *int64, _ *int64, _, _ string, _, _ int) ([]*catalog.SavingProductAdminView, *catalog.SavingProductAdminStats, error) {
	return nil, nil, nil
}
func (m *mockCatalogImageRepo) ListAllMasterProductsForMatching(_ context.Context) ([]*catalog.CatalogMatchSource, error) {
	return nil, nil
}

func (m *mockCatalogImageRepo) GetProductBySKU(_ context.Context, sku string) (*catalog.Product, error) {
	if p, ok := m.products[sku]; ok {
		return p, nil
	}
	return nil, apperr.NotFound("product")
}

func (m *mockCatalogImageRepo) GetProductByBarcode(_ context.Context, barcode string) (*catalog.Product, error) {
	for _, p := range m.products {
		if p.Barcode == barcode {
			return p, nil
		}
	}
	return nil, apperr.NotFound("product")
}

func (m *mockCatalogImageRepo) UpdateProductImageBySKU(_ context.Context, sku string, imagePath, imageLink string) (*catalog.Product, error) {
	if p, ok := m.products[sku]; ok {
		p.Image = imagePath
		p.ImageLink = imageLink
		return p, nil
	}
	return nil, apperr.NotFound("product")
}

func (m *mockCatalogImageRepo) UpdateProductImageByID(_ context.Context, id int64, imagePath, imageLink string) (*catalog.Product, error) {
	for _, p := range m.products {
		if p.ID == id {
			p.Image = imagePath
			p.ImageLink = imageLink
			return p, nil
		}
	}
	return nil, apperr.NotFound("product")
}

func (m *mockCatalogImageRepo) ListMatchDecisions(_ context.Context, _ string, _, _ int) ([]*catalog.MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogImageRepo) ListMatchDecisionsFiltered(_ context.Context, _ catalog.DecisionMemoryFilter) ([]*catalog.MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogImageRepo) ListMatchDecisionsForOrgWithPlatform(_ context.Context, _ int64, _ string, _, _ int) ([]*catalog.MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogImageRepo) GetDecisionMemoryPreference(_ context.Context, _ int64) (bool, error) {
	return true, nil
}
func (m *mockCatalogImageRepo) SetDecisionMemoryPreference(_ context.Context, _ int64, _ bool, _ int64) error {
	return nil
}
func (m *mockCatalogImageRepo) PromoteMatchDecision(_ context.Context, _ int64, _ int64) error {
	return nil
}
func (m *mockCatalogImageRepo) DemoteMatchDecision(_ context.Context, _ int64) error {
	return nil
}
func (m *mockCatalogImageRepo) BulkPromoteMatchDecisions(_ context.Context, _ []int64, _ int64) (int64, error) {
	return 0, nil
}
func (m *mockCatalogImageRepo) BulkDeleteMatchDecisions(_ context.Context, _ []int64) (int64, error) {
	return 0, nil
}
func (m *mockCatalogImageRepo) RelinkMatchDecision(_ context.Context, _ int64, _ *int64, _ int64) error {
	return nil
}
func (m *mockCatalogImageRepo) RelinkMatchDecisionForOrg(_ context.Context, _ int64, _ int64, _ *int64, _ int64) error {
	return nil
}
func (m *mockCatalogImageRepo) DeleteMatchDecision(_ context.Context, _ int64) error {
	return nil
}
func (m *mockCatalogImageRepo) ClearMatchDecisions(_ context.Context) error {
	return nil
}
func (m *mockCatalogImageRepo) ListMatchDecisionsForOrg(_ context.Context, _ int64, _ string, _, _ int) ([]*catalog.MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogImageRepo) DeleteMatchDecisionForOrg(_ context.Context, _, _ int64) error {
	return nil
}
func (m *mockCatalogImageRepo) ClearMatchDecisionsForOrg(_ context.Context, _ int64) error {
	return nil
}
func (m *mockCatalogImageRepo) SaveManualDecision(_ context.Context, _, _ int64, _ string, _ int64, _ string) error {
	return nil
}
func (m *mockCatalogImageRepo) IsDecisionMemoryEnabled(_ context.Context) bool {
	return true
}
func (m *mockCatalogImageRepo) SetDecisionMemoryEnabled(_ context.Context, _ bool) error {
	return nil
}
func (m *mockCatalogImageRepo) ListCustomerMappings(_ context.Context, _ int64, _ string, _, _ int) ([]*catalog.CustomerMappingView, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogImageRepo) DeleteCustomerMapping(_ context.Context, _, _ int64) error {
	return nil
}
func (m *mockCatalogImageRepo) ClearCustomerMappings(_ context.Context, _ int64) error {
	return nil
}
func (m *mockCatalogImageRepo) ListBuyerOffers(_ context.Context, _ catalog.BuyerOfferQuery) ([]*catalog.BuyerOffer, int, error) {
	return nil, 0, nil
}
