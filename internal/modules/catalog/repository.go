package catalog

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// SearchParams holds filters for searching products.
type SearchParams struct {
	Query string
	// FirstWord, when non-empty, prioritizes names starting with / containing
	// the query's first word (catalog-linking UX: the brand comes first in
	// Arabic product names). Empty preserves legacy ranking exactly.
	FirstWord      string
	OrganizationID *int64
	CategoryID     *int64
	BrandID        *int64
	Status         string
	DosageForm     string
	Manufacturer   string
	MinPrice       *money.Amount
	MaxPrice       *money.Amount
	Sort           string // empty, price_asc, price_desc, newest, name
	Limit          int
	Offset         int
	AllowedWorkIDs []int64
	FilterMode     int // 0 = Simple (dashboard/catalog), 1 = WithConnections (purchase requests)
	InStock        bool
}

// VariantSearchParams holds filters for searching product variants.
type VariantSearchParams struct {
	Query      string
	CategoryID *int64
	Status     string
	Limit      int
	Offset     int
}

// Repository defines the persistence interface for the catalog bounded context.
type Repository interface {
	CreateProduct(ctx context.Context, p *Product) error
	BulkUpsertProducts(ctx context.Context, prods []*Product, opts BulkWriteOptions) (BulkWriteResult, error)
	GetProductByID(ctx context.Context, id int64) (*Product, error)
	UpdateProduct(ctx context.Context, p *Product) error
	DeleteProduct(ctx context.Context, id int64) error
	SearchProducts(ctx context.Context, params SearchParams) ([]*Product, error)
	CountProducts(ctx context.Context, params SearchParams) (int, error)
	ListDosageForms(ctx context.Context) ([]string, error)
	// ListProducts is the vendor's own catalogue, scoped by row-level security
	// to the active organization rather than by a caller-supplied id.
	ListProducts(ctx context.Context, status string, limit, offset int) ([]*Product, error)
	SetProductsStatus(ctx context.Context, ids []int64, status ProductStatus) (int64, error)

	CreateVariant(ctx context.Context, v *ProductVariant) error
	GetVariantByID(ctx context.Context, id int64) (*ProductVariant, error)
	GetVariantBySKUOrBarcode(ctx context.Context, orgID int64, sku, barcode string) (*ProductVariant, error)
	GetVariantByProductAndOrg(ctx context.Context, orgID int64, productID int64) (*ProductVariant, error)
	ListVariantsByProduct(ctx context.Context, productID int64) ([]*ProductVariant, error)
	ListVariantsByProducts(ctx context.Context, productIDs []int64) ([]*ProductVariant, error)
	// GetVariantsByIDs fetches many variants at once, keyed by id.
	GetVariantsByIDs(ctx context.Context, ids []int64) (map[int64]*ProductVariant, error)
	ListVariantsByOrganization(ctx context.Context, orgID int64, params VariantSearchParams) ([]*ProductVariant, int, error)
	ListBuyerOffers(ctx context.Context, q BuyerOfferQuery) ([]*BuyerOffer, int, error)
	ListAllVariants(ctx context.Context, params VariantSearchParams) ([]*ProductVariant, int, error)
	UpdateVariant(ctx context.Context, v *ProductVariant) error
	DeleteVariant(ctx context.Context, id int64) error
	DeleteAllVariantsByOrg(ctx context.Context, orgID int64) (int64, error)
	ActivateAllVariantsByOrg(ctx context.Context, orgID int64) (int64, error)
	ToggleVariantStatus(ctx context.Context, orgID, variantID int64) (ProductStatus, error)
	DeleteAllProducts(ctx context.Context) (int64, error)

	CreateCategory(ctx context.Context, c *Category) error
	GetCategoryByID(ctx context.Context, id int64) (*Category, error)
	UpdateCategory(ctx context.Context, c *Category) error
	DeleteCategory(ctx context.Context, id int64) error
	ListCategories(ctx context.Context) ([]*Category, error)
	ListCategoriesWithProductCount(ctx context.Context, search, status string, limit, offset int) ([]*CategoryWithCount, int, error)
	// CountProductsInCategory backs the refusal to delete a taxonomy row that
	// products still reference; deleting one would leave them uncategorised
	// with no way to find them in the vendor UI.
	CountProductsByOrg(ctx context.Context, orgID int64, status string) (int, error)
	CountProductsInCategory(ctx context.Context, categoryID int64) (int, error)

	CreateBrand(ctx context.Context, b *Brand) error
	GetBrandByID(ctx context.Context, id int64) (*Brand, error)
	UpdateBrand(ctx context.Context, b *Brand) error
	DeleteBrand(ctx context.Context, id int64) error
	// Category -> brand relationship (PLAN_V7 Phase 4). The product form offers
	// a category first, then only that category's manufacturers.
	ListBrandsByCategory(ctx context.Context, categoryID int64) ([]*Brand, error)
	BrandInCategory(ctx context.Context, categoryID, brandID int64) (bool, error)
	SetBrandCategories(ctx context.Context, brandID int64, categoryIDs []int64) error

	ListBrands(ctx context.Context) ([]*Brand, error)
	ListBrandsWithProductCount(ctx context.Context, search, status string, limit, offset int) ([]*BrandWithCount, int, error)
	CountProductsInBrand(ctx context.Context, brandID int64) (int, error)

	SetCustomerPricing(ctx context.Context, m *CustomerProductMapping) error
	GetCustomerPricing(ctx context.Context, vendorOrgID, customerOrgID, productID int64) (*CustomerProductMapping, error)

	CreateProductAlert(ctx context.Context, a *ProductAlert) error
	ListProductAlertsByUser(ctx context.Context, userID int64) ([]*ProductAlert, error)

	// Denormalized read model (catalog.product_index)
	UpsertProductIndex(ctx context.Context, item *ProductIndexItem) error
	DeleteProductIndex(ctx context.Context, uniqueRowID string) error
	DeleteProductIndexByProduct(ctx context.Context, productID int64) error
	SearchProductIndex(ctx context.Context, params SearchParams) ([]*ProductIndexItem, error)
	RebuildProductIndex(ctx context.Context) (int64, error)

	// Saving Products (catalog.saving_products)
	CreateSavingProduct(ctx context.Context, sp *SavingProduct) error
	UpdateSavingProduct(ctx context.Context, sp *SavingProduct) error
	ListSavingProductsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*SavingProduct, error)
	ListSavingProductsEnriched(ctx context.Context, orgID int64, search string, filter string, limit, offset int) ([]*SavingProductEnriched, *SavingProductStats, error)
	GetSavingProductByID(ctx context.Context, id int64) (*SavingProduct, error)
	DeleteSavingProduct(ctx context.Context, id, orgID int64) error
	DeleteAllSavingProducts(ctx context.Context, orgID int64) error
	GetProductProviders(ctx context.Context, productID int64) ([]*ProductProviderInfo, error)
	BatchUpsertSavingProducts(ctx context.Context, orgID int64, userID *int64, items []*SavingProduct) (added, updated int, err error)
	ListAllSavingProductsAdmin(ctx context.Context, userID *int64, orgID *int64, search string, filter string, limit, offset int) ([]*SavingProductAdminView, *SavingProductAdminStats, error)
	ListAllMasterProductsForMatching(ctx context.Context) ([]*CatalogMatchSource, error)
	GetProductBySKU(ctx context.Context, sku string) (*Product, error)
	GetProductByBarcode(ctx context.Context, barcode string) (*Product, error)
	UpdateProductImageBySKU(ctx context.Context, sku string, imagePath string, imageLink string) (*Product, error)
	UpdateProductImageByID(ctx context.Context, id int64, imagePath string, imageLink string) (*Product, error)

	// Decision Memory & Mappings Management
	ListMatchDecisions(ctx context.Context, search string, limit, offset int) ([]*MatchDecisionView, int, error)
	ListMatchDecisionsFiltered(ctx context.Context, f DecisionMemoryFilter) ([]*MatchDecisionView, int, error)
	ListMatchDecisionsForOrgWithPlatform(ctx context.Context, orgID int64, search string, limit, offset int) ([]*MatchDecisionView, int, error)
	GetDecisionMemoryPreference(ctx context.Context, orgID int64) (bool, error)
	SetDecisionMemoryPreference(ctx context.Context, orgID int64, usePlatform bool, updatedBy int64) error
	PromoteMatchDecision(ctx context.Context, id int64, adminUserID int64) error
	DemoteMatchDecision(ctx context.Context, id int64) error
	BulkPromoteMatchDecisions(ctx context.Context, ids []int64, adminUserID int64) (int64, error)
	BulkDeleteMatchDecisions(ctx context.Context, ids []int64) (int64, error)
	RelinkMatchDecision(ctx context.Context, id int64, productID *int64, adminUserID int64) error
	RelinkMatchDecisionForOrg(ctx context.Context, orgID int64, id int64, productID *int64, userID int64) error
	DeleteMatchDecision(ctx context.Context, id int64) error
	ClearMatchDecisions(ctx context.Context) error
	ListMatchDecisionsForOrg(ctx context.Context, orgID int64, search string, limit, offset int) ([]*MatchDecisionView, int, error)
	DeleteMatchDecisionForOrg(ctx context.Context, orgID, id int64) error
	ClearMatchDecisionsForOrg(ctx context.Context, orgID int64) error
	SaveManualDecision(ctx context.Context, orgID, userID int64, rawName string, productID int64, reason string) error
	IsDecisionMemoryEnabled(ctx context.Context) bool
	SetDecisionMemoryEnabled(ctx context.Context, enabled bool) error
	ListCustomerMappings(ctx context.Context, orgID int64, search string, limit, offset int) ([]*CustomerMappingView, int, error)
	DeleteCustomerMapping(ctx context.Context, orgID, id int64) error
	ClearCustomerMappings(ctx context.Context, orgID int64) error
}
