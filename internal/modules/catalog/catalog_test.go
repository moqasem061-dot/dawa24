package catalog

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type mockCatalogRepo struct {
	products        map[int64]*Product
	variants        map[int64]*ProductVariant
	categories      map[int64]*Category
	brands          map[int64]*Brand
	customerPricing map[string]*CustomerProductMapping
	alerts          map[int64]*ProductAlert
	nextID          int64
}

func newMockCatalogRepo() *mockCatalogRepo {
	return &mockCatalogRepo{
		products:        map[int64]*Product{},
		variants:        map[int64]*ProductVariant{},
		categories:      map[int64]*Category{},
		brands:          map[int64]*Brand{},
		customerPricing: map[string]*CustomerProductMapping{},
		alerts:          map[int64]*ProductAlert{},
		nextID:          1,
	}
}

func (m *mockCatalogRepo) CreateProduct(_ context.Context, p *Product) error {
	p.ID = m.nextID
	m.nextID++
	m.products[p.ID] = p
	return nil
}

func (m *mockCatalogRepo) BulkUpsertProducts(_ context.Context, prods []*Product, _ BulkWriteOptions) (BulkWriteResult, error) {
	for _, p := range prods {
		p.ID = m.nextID
		m.nextID++
		m.products[p.ID] = p
	}
	return BulkWriteResult{Inserted: len(prods), Matches: map[int]MatchReason{}}, nil
}

func (m *mockCatalogRepo) GetProductByID(_ context.Context, id int64) (*Product, error) {
	p, ok := m.products[id]
	if !ok {
		return nil, apperr.NotFound("product")
	}
	return p, nil
}

func (m *mockCatalogRepo) UpdateProduct(_ context.Context, p *Product) error {
	m.products[p.ID] = p
	return nil
}

func (m *mockCatalogRepo) DeleteProduct(_ context.Context, id int64) error {
	delete(m.products, id)
	return nil
}

func (m *mockCatalogRepo) SearchProducts(_ context.Context, params SearchParams) ([]*Product, error) {
	var list []*Product
	for _, p := range m.products {
		list = append(list, p)
	}
	return list, nil
}

func (m *mockCatalogRepo) CountProducts(_ context.Context, params SearchParams) (int, error) {
	return len(m.products), nil
}

func (m *mockCatalogRepo) CreateVariant(_ context.Context, v *ProductVariant) error {
	v.ID = m.nextID
	m.nextID++
	m.variants[v.ID] = v
	return nil
}

func (m *mockCatalogRepo) GetVariantByID(_ context.Context, id int64) (*ProductVariant, error) {
	v, ok := m.variants[id]
	if !ok {
		return nil, apperr.NotFound("product_variant")
	}
	return v, nil
}

func (m *mockCatalogRepo) GetVariantBySKUOrBarcode(_ context.Context, orgID int64, sku, barcode string) (*ProductVariant, error) {
	for _, v := range m.variants {
		if v.OrganizationID == orgID {
			if (sku != "" && v.SKU == sku) || (barcode != "" && v.Barcode == barcode) {
				return v, nil
			}
		}
	}
	return nil, apperr.NotFound("product_variant")
}

func (m *mockCatalogRepo) GetVariantByProductAndOrg(_ context.Context, orgID int64, productID int64) (*ProductVariant, error) {
	for _, v := range m.variants {
		if v.OrganizationID == orgID && v.ProductID == productID {
			return v, nil
		}
	}
	return nil, apperr.NotFound("product_variant")
}

func (m *mockCatalogRepo) ListVariantsByProduct(_ context.Context, productID int64) ([]*ProductVariant, error) {
	var list []*ProductVariant
	for _, v := range m.variants {
		if v.ProductID == productID {
			list = append(list, v)
		}
	}
	return list, nil
}

func (m *mockCatalogRepo) ListVariantsByProducts(_ context.Context, productIDs []int64) ([]*ProductVariant, error) {
	wanted := make(map[int64]struct{}, len(productIDs))
	for _, id := range productIDs {
		wanted[id] = struct{}{}
	}
	var list []*ProductVariant
	for _, v := range m.variants {
		if _, ok := wanted[v.ProductID]; ok {
			list = append(list, v)
		}
	}
	return list, nil
}

func (m *mockCatalogRepo) GetVariantsByIDs(context.Context, []int64) (map[int64]*ProductVariant, error) {
	return map[int64]*ProductVariant{}, nil
}

func (m *mockCatalogRepo) ListVariantsByOrganization(_ context.Context, orgID int64, params VariantSearchParams) ([]*ProductVariant, int, error) {
	var list []*ProductVariant
	for _, v := range m.variants {
		if v.OrganizationID == orgID {
			list = append(list, v)
		}
	}
	return list, len(list), nil
}

func (m *mockCatalogRepo) ListAllVariants(_ context.Context, params VariantSearchParams) ([]*ProductVariant, int, error) {
	var list []*ProductVariant
	for _, v := range m.variants {
		list = append(list, v)
	}
	return list, len(list), nil
}

func (m *mockCatalogRepo) UpdateVariant(_ context.Context, v *ProductVariant) error {
	m.variants[v.ID] = v
	return nil
}

func (m *mockCatalogRepo) DeleteVariant(_ context.Context, id int64) error {
	delete(m.variants, id)
	return nil
}

func (m *mockCatalogRepo) CreateCategory(_ context.Context, c *Category) error {
	c.ID = m.nextID
	m.nextID++
	m.categories[c.ID] = c
	return nil
}

func (m *mockCatalogRepo) GetCategoryByID(_ context.Context, id int64) (*Category, error) {
	c, ok := m.categories[id]
	if !ok {
		return nil, apperr.NotFound("category")
	}
	return c, nil
}

func (m *mockCatalogRepo) UpdateCategory(_ context.Context, c *Category) error {
	m.categories[c.ID] = c
	return nil
}

func (m *mockCatalogRepo) ListCategories(_ context.Context) ([]*Category, error) {
	var list []*Category
	for _, c := range m.categories {
		list = append(list, c)
	}
	return list, nil
}

func (m *mockCatalogRepo) ListCategoriesWithProductCount(ctx context.Context, _ string, _ string, limit, offset int) ([]*CategoryWithCount, int, error) {
	cats, err := m.ListCategories(ctx)
	if err != nil {
		return nil, 0, err
	}
	var res []*CategoryWithCount
	for _, c := range cats {
		res = append(res, &CategoryWithCount{Category: c, ProductCount: 0})
	}
	return res, len(res), nil
}

func (m *mockCatalogRepo) CreateBrand(_ context.Context, b *Brand) error {
	b.ID = m.nextID
	m.nextID++
	m.brands[b.ID] = b
	return nil
}

func (m *mockCatalogRepo) GetBrandByID(_ context.Context, id int64) (*Brand, error) {
	b, ok := m.brands[id]
	if !ok {
		return nil, apperr.NotFound("brand")
	}
	return b, nil
}

func (m *mockCatalogRepo) UpdateBrand(_ context.Context, b *Brand) error {
	m.brands[b.ID] = b
	return nil
}

func (m *mockCatalogRepo) ListBrands(_ context.Context) ([]*Brand, error) {
	var list []*Brand
	for _, b := range m.brands {
		list = append(list, b)
	}
	return list, nil
}

func (m *mockCatalogRepo) ListBrandsWithProductCount(_ context.Context, _, _ string, _, _ int) ([]*BrandWithCount, int, error) {
	var list []*BrandWithCount
	for _, b := range m.brands {
		list = append(list, &BrandWithCount{Brand: b, ProductCount: 0})
	}
	return list, len(list), nil
}

func (m *mockCatalogRepo) SetCustomerPricing(_ context.Context, cm *CustomerProductMapping) error {
	key := fmt.Sprintf("%d:%d:%d", cm.OrganizationID, deref64(cm.CustomerOrgID), cm.ProductID)
	m.customerPricing[key] = cm
	return nil
}

func (m *mockCatalogRepo) GetCustomerPricing(_ context.Context, vendorOrgID, customerOrgID, productID int64) (*CustomerProductMapping, error) {
	key := fmt.Sprintf("%d:%d:%d", vendorOrgID, customerOrgID, productID)
	cm, ok := m.customerPricing[key]
	if !ok {
		return nil, apperr.NotFound("customer_pricing")
	}
	return cm, nil
}

func deref64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func (m *mockCatalogRepo) CreateProductAlert(_ context.Context, a *ProductAlert) error {
	a.ID = m.nextID
	m.nextID++
	m.alerts[a.ID] = a
	return nil
}

func (m *mockCatalogRepo) ListProductAlertsByUser(_ context.Context, userID int64) ([]*ProductAlert, error) {
	var list []*ProductAlert
	for _, a := range m.alerts {
		if a.UserID == userID {
			list = append(list, a)
		}
	}
	return list, nil
}

func (m *mockCatalogRepo) UpsertProductIndex(_ context.Context, item *ProductIndexItem) error {
	return nil
}

func (m *mockCatalogRepo) DeleteProductIndex(_ context.Context, _ string) error {
	return nil
}

func (m *mockCatalogRepo) DeleteProductIndexByProduct(_ context.Context, _ int64) error {
	return nil
}

func (m *mockCatalogRepo) SearchProductIndex(_ context.Context, _ SearchParams) ([]*ProductIndexItem, error) {
	return nil, nil // returns empty to trigger deterministic fallback in tests
}

func (m *mockCatalogRepo) RebuildProductIndex(_ context.Context) (int64, error) {
	return int64(len(m.products)), nil
}

func (m *mockCatalogRepo) CreateSavingProduct(_ context.Context, sp *SavingProduct) error {
	sp.ID = m.nextID
	m.nextID++
	return nil
}

func (m *mockCatalogRepo) UpdateSavingProduct(_ context.Context, sp *SavingProduct) error {
	return nil
}

func (m *mockCatalogRepo) ListSavingProductsByOrg(_ context.Context, _ int64, _, _ int) ([]*SavingProduct, error) {
	return nil, nil
}

func (m *mockCatalogRepo) ListSavingProductsEnriched(_ context.Context, orgID int64, search, filter string, limit, offset int) ([]*SavingProductEnriched, *SavingProductStats, error) {
	return nil, &SavingProductStats{}, nil
}

func (m *mockCatalogRepo) GetSavingProductByID(_ context.Context, _ int64) (*SavingProduct, error) {
	return nil, nil
}

func (m *mockCatalogRepo) DeleteSavingProduct(_ context.Context, _, _ int64) error {
	return nil
}

func (m *mockCatalogRepo) DeleteAllSavingProducts(_ context.Context, _ int64) error {
	return nil
}

func (m *mockCatalogRepo) GetProductProviders(_ context.Context, productID int64) ([]*ProductProviderInfo, error) {
	return nil, nil
}

func (m *mockCatalogRepo) BatchUpsertSavingProducts(_ context.Context, orgID int64, userID *int64, items []*SavingProduct) (int, int, error) {
	return len(items), 0, nil
}

func (m *mockCatalogRepo) ListAllSavingProductsAdmin(_ context.Context, _ *int64, _ *int64, _ string, _ string, _, _ int) ([]*SavingProductAdminView, *SavingProductAdminStats, error) {
	return nil, &SavingProductAdminStats{}, nil
}

func (m *mockCatalogRepo) DeleteAllVariantsByOrg(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}

func (m *mockCatalogRepo) ActivateAllVariantsByOrg(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}

func (m *mockCatalogRepo) ToggleVariantStatus(_ context.Context, _, _ int64) (ProductStatus, error) {
	return StatusActive, nil
}

func (m *mockCatalogRepo) DeleteAllProducts(_ context.Context) (int64, error) {
	return 0, nil
}

func (m *mockCatalogRepo) GetProductBySKU(_ context.Context, sku string) (*Product, error) {
	for _, p := range m.products {
		if p.SKU == sku || p.Barcode == sku {
			return p, nil
		}
	}
	return nil, apperr.NotFound("product")
}

func (m *mockCatalogRepo) GetProductByBarcode(_ context.Context, barcode string) (*Product, error) {
	for _, p := range m.products {
		if p.Barcode == barcode {
			return p, nil
		}
	}
	return nil, apperr.NotFound("product")
}

func (m *mockCatalogRepo) ListBuyerOffers(_ context.Context, _ BuyerOfferQuery) ([]*BuyerOffer, int, error) {
	return nil, 0, nil
}
