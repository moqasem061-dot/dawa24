package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	catalogHttp "github.com/muhiya/dawa24-store/internal/modules/catalog/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func (happyRepo) GetVariantsByIDs(context.Context, []int64) (map[int64]*catalog.ProductVariant, error) {
	return map[int64]*catalog.ProductVariant{}, nil
}

type happyRepo struct{}

func (happyRepo) CreateProduct(ctx context.Context, p *catalog.Product) error {
	p.ID = 1
	return nil
}
func (happyRepo) GetProductByID(ctx context.Context, id int64) (*catalog.Product, error) {
	return &catalog.Product{ID: id, Name: i18n.Text{"en": "Panadol"}, Status: catalog.StatusActive}, nil
}
func (happyRepo) BulkUpsertProducts(ctx context.Context, prods []*catalog.Product, _ catalog.BulkWriteOptions) (catalog.BulkWriteResult, error) {
	return catalog.BulkWriteResult{Inserted: len(prods), Matches: map[int]catalog.MatchReason{}}, nil
}
func (happyRepo) UpdateProduct(ctx context.Context, p *catalog.Product) error { return nil }
func (happyRepo) DeleteProduct(ctx context.Context, id int64) error           { return nil }
func (happyRepo) SearchProducts(ctx context.Context, params catalog.SearchParams) ([]*catalog.Product, error) {
	return []*catalog.Product{{ID: 1, Name: i18n.Text{"en": "Panadol"}}}, nil
}
func (happyRepo) CountProducts(ctx context.Context, params catalog.SearchParams) (int, error) {
	return 1, nil
}
func (happyRepo) ListDosageForms(ctx context.Context) ([]string, error) {
	return []string{"tablet", "capsule"}, nil
}
func (happyRepo) ListProducts(ctx context.Context, status string, limit, offset int) ([]*catalog.Product, error) {
	return []*catalog.Product{{ID: 1, Name: i18n.Text{"en": "Panadol"}}}, nil
}
func (happyRepo) SetProductsStatus(ctx context.Context, ids []int64, status catalog.ProductStatus) (int64, error) {
	return int64(len(ids)), nil
}
func (happyRepo) CreateVariant(ctx context.Context, v *catalog.ProductVariant) error {
	v.ID = 1
	return nil
}
func (happyRepo) GetVariantByID(ctx context.Context, id int64) (*catalog.ProductVariant, error) {
	return &catalog.ProductVariant{ID: id, ProductID: 1, SKU: "SKU-1"}, nil
}
func (happyRepo) GetVariantBySKUOrBarcode(ctx context.Context, orgID int64, sku, barcode string) (*catalog.ProductVariant, error) {
	return &catalog.ProductVariant{ID: 1, OrganizationID: orgID, ProductID: 1, SKU: sku, Barcode: barcode}, nil
}
func (happyRepo) GetVariantByProductAndOrg(ctx context.Context, orgID int64, productID int64) (*catalog.ProductVariant, error) {
	return &catalog.ProductVariant{ID: 1, OrganizationID: orgID, ProductID: productID, SKU: "SKU-1"}, nil
}
func (happyRepo) ListVariantsByProduct(ctx context.Context, productID int64) ([]*catalog.ProductVariant, error) {
	return []*catalog.ProductVariant{{ID: 1, ProductID: productID, SKU: "SKU-1"}}, nil
}
func (happyRepo) ListVariantsByProducts(ctx context.Context, productIDs []int64) ([]*catalog.ProductVariant, error) {
	var list []*catalog.ProductVariant
	for _, id := range productIDs {
		list = append(list, &catalog.ProductVariant{ID: id, ProductID: id, SKU: "SKU-1"})
	}
	return list, nil
}
func (happyRepo) ListVariantsByOrganization(ctx context.Context, orgID int64, params catalog.VariantSearchParams) ([]*catalog.ProductVariant, int, error) {
	return []*catalog.ProductVariant{{ID: 1, OrganizationID: orgID, ProductID: 1, SKU: "SKU-1"}}, 1, nil
}
func (happyRepo) ListBuyerOffers(ctx context.Context, q catalog.BuyerOfferQuery) ([]*catalog.BuyerOffer, int, error) {
	return nil, 0, nil
}
func (happyRepo) ListAllVariants(ctx context.Context, params catalog.VariantSearchParams) ([]*catalog.ProductVariant, int, error) {
	return []*catalog.ProductVariant{{ID: 1, OrganizationID: 1, ProductID: 1, SKU: "SKU-1"}}, 1, nil
}
func (happyRepo) UpdateVariant(ctx context.Context, v *catalog.ProductVariant) error { return nil }
func (happyRepo) DeleteVariant(ctx context.Context, id int64) error                  { return nil }
func (happyRepo) ToggleVariantStatus(ctx context.Context, orgID, variantID int64) (catalog.ProductStatus, error) {
	return catalog.StatusActive, nil
}
func (happyRepo) CreateCategory(ctx context.Context, c *catalog.Category) error {
	c.ID = 1
	return nil
}
func (happyRepo) GetCategoryByID(ctx context.Context, id int64) (*catalog.Category, error) {
	return &catalog.Category{ID: id, Name: i18n.Text{"en": "Painkillers"}}, nil
}
func (happyRepo) UpdateCategory(ctx context.Context, c *catalog.Category) error { return nil }
func (happyRepo) DeleteCategory(ctx context.Context, id int64) error            { return nil }
func (happyRepo) ListCategories(ctx context.Context) ([]*catalog.Category, error) {
	return []*catalog.Category{{ID: 1, Name: i18n.Text{"en": "Painkillers"}}}, nil
}
func (happyRepo) ListCategoriesWithProductCount(ctx context.Context, _ string, _ string, limit, offset int) ([]*catalog.CategoryWithCount, int, error) {
	return []*catalog.CategoryWithCount{{Category: &catalog.Category{ID: 1, Name: i18n.Text{"en": "Painkillers"}}, ProductCount: 0}}, 1, nil
}
func (happyRepo) CountProductsInCategory(ctx context.Context, categoryID int64) (int, error) {
	return 0, nil
}
func (happyRepo) CountProductsInBrand(ctx context.Context, brandID int64) (int, error) {
	return 0, nil
}
func (happyRepo) CreateBrand(ctx context.Context, b *catalog.Brand) error {
	b.ID = 1
	return nil
}
func (happyRepo) GetBrandByID(ctx context.Context, id int64) (*catalog.Brand, error) {
	return &catalog.Brand{ID: id, Name: i18n.Text{"en": "GSK"}}, nil
}
func (happyRepo) UpdateBrand(ctx context.Context, b *catalog.Brand) error { return nil }
func (happyRepo) DeleteBrand(ctx context.Context, id int64) error         { return nil }
func (happyRepo) ListBrands(ctx context.Context) ([]*catalog.Brand, error) {
	return []*catalog.Brand{{ID: 1, Name: i18n.Text{"en": "GSK"}}}, nil
}
func (happyRepo) ListBrandsWithProductCount(ctx context.Context, _, _ string, _, _ int) ([]*catalog.BrandWithCount, int, error) {
	return []*catalog.BrandWithCount{{Brand: &catalog.Brand{ID: 1, Name: i18n.Text{"en": "GSK"}}, ProductCount: 5}}, 1, nil
}
func (happyRepo) SetCustomerPricing(ctx context.Context, m *catalog.CustomerProductMapping) error {
	return nil
}
func (happyRepo) GetCustomerPricing(ctx context.Context, organizationID, customerOrgID, productID int64) (*catalog.CustomerProductMapping, error) {
	return &catalog.CustomerProductMapping{ProductID: productID, CustomerOrgID: &customerOrgID, Price: money.MustParse("45.00")}, nil
}
func (happyRepo) CreateProductAlert(ctx context.Context, a *catalog.ProductAlert) error {
	a.ID = 1
	return nil
}
func (happyRepo) ListProductAlertsByUser(ctx context.Context, userID int64) ([]*catalog.ProductAlert, error) {
	return []*catalog.ProductAlert{{ID: 1, UserID: userID}}, nil
}

func (happyRepo) UpsertProductIndex(ctx context.Context, item *catalog.ProductIndexItem) error {
	return nil
}
func (happyRepo) DeleteProductIndex(ctx context.Context, uniqueRowID string) error {
	return nil
}
func (happyRepo) DeleteProductIndexByProduct(ctx context.Context, productID int64) error {
	return nil
}
func (happyRepo) SearchProductIndex(ctx context.Context, params catalog.SearchParams) ([]*catalog.ProductIndexItem, error) {
	return nil, nil
}
func (happyRepo) RebuildProductIndex(ctx context.Context) (int64, error) {
	return 0, nil
}
func (happyRepo) CreateSavingProduct(ctx context.Context, sp *catalog.SavingProduct) error {
	sp.ID = 1
	return nil
}
func (happyRepo) ListSavingProductsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*catalog.SavingProduct, error) {
	return []*catalog.SavingProduct{{ID: 1, NameProduct: "Panadol"}}, nil
}
func (happyRepo) GetSavingProductByID(ctx context.Context, id int64) (*catalog.SavingProduct, error) {
	return &catalog.SavingProduct{ID: id, NameProduct: "Panadol"}, nil
}
func (happyRepo) DeleteSavingProduct(ctx context.Context, id, orgID int64) error {
	return nil
}
func (happyRepo) UpdateSavingProduct(ctx context.Context, sp *catalog.SavingProduct) error {
	return nil
}
func (happyRepo) ListSavingProductsEnriched(ctx context.Context, orgID int64, search, filter string, limit, offset int) ([]*catalog.SavingProductEnriched, *catalog.SavingProductStats, error) {
	return nil, &catalog.SavingProductStats{}, nil
}
func (happyRepo) GetProductProviders(ctx context.Context, productID int64) ([]*catalog.ProductProviderInfo, error) {
	return nil, nil
}
func (happyRepo) BatchUpsertSavingProducts(ctx context.Context, orgID int64, userID *int64, items []*catalog.SavingProduct) (int, int, error) {
	return len(items), 0, nil
}
func (happyRepo) ListAllSavingProductsAdmin(ctx context.Context, _ *int64, _ *int64, _ string, _ string, _, _ int) ([]*catalog.SavingProductAdminView, *catalog.SavingProductAdminStats, error) {
	return nil, &catalog.SavingProductAdminStats{}, nil
}
func (happyRepo) ListAllMasterProductsForMatching(ctx context.Context) ([]*catalog.CatalogMatchSource, error) {
	return nil, nil
}
func (happyRepo) DeleteAllVariantsByOrg(ctx context.Context, orgID int64) (int64, error) {
	return 0, nil
}
func (happyRepo) ActivateAllVariantsByOrg(ctx context.Context, orgID int64) (int64, error) {
	return 0, nil
}
func (happyRepo) DeleteAllProducts(ctx context.Context) (int64, error) {
	return 0, nil
}
func (happyRepo) DeleteAllSavingProducts(ctx context.Context, orgID int64) error {
	return nil
}
func (happyRepo) GetProductBySKU(ctx context.Context, sku string) (*catalog.Product, error) {
	return nil, nil
}
func (happyRepo) GetProductByBarcode(ctx context.Context, barcode string) (*catalog.Product, error) {
	return nil, nil
}
func (happyRepo) UpdateProductImageBySKU(ctx context.Context, sku string, imagePath, imageLink string) (*catalog.Product, error) {
	return nil, nil
}
func (happyRepo) UpdateProductImageByID(ctx context.Context, id int64, imagePath, imageLink string) (*catalog.Product, error) {
	return nil, nil
}
func (happyRepo) ListMatchDecisions(ctx context.Context, search string, limit, offset int) ([]*catalog.MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (happyRepo) ListMatchDecisionsFiltered(ctx context.Context, f catalog.DecisionMemoryFilter) ([]*catalog.MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (happyRepo) ListMatchDecisionsForOrgWithPlatform(ctx context.Context, orgID int64, search string, limit, offset int) ([]*catalog.MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (happyRepo) GetDecisionMemoryPreference(ctx context.Context, orgID int64) (bool, error) {
	return true, nil
}
func (happyRepo) SetDecisionMemoryPreference(ctx context.Context, orgID int64, usePlatform bool, updatedBy int64) error {
	return nil
}
func (happyRepo) PromoteMatchDecision(ctx context.Context, id int64, adminUserID int64) error {
	return nil
}
func (happyRepo) DemoteMatchDecision(ctx context.Context, id int64) error {
	return nil
}
func (happyRepo) BulkPromoteMatchDecisions(ctx context.Context, ids []int64, adminUserID int64) (int64, error) {
	return 0, nil
}
func (happyRepo) BulkDeleteMatchDecisions(ctx context.Context, ids []int64) (int64, error) {
	return 0, nil
}
func (happyRepo) RelinkMatchDecision(ctx context.Context, id int64, productID *int64, adminUserID int64) error {
	return nil
}
func (happyRepo) RelinkMatchDecisionForOrg(ctx context.Context, orgID int64, id int64, productID *int64, userID int64) error {
	return nil
}
func (happyRepo) DeleteMatchDecision(ctx context.Context, id int64) error {
	return nil
}
func (happyRepo) ClearMatchDecisions(ctx context.Context) error {
	return nil
}
func (happyRepo) ListMatchDecisionsForOrg(ctx context.Context, orgID int64, search string, limit, offset int) ([]*catalog.MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (happyRepo) DeleteMatchDecisionForOrg(ctx context.Context, orgID, id int64) error {
	return nil
}
func (happyRepo) ClearMatchDecisionsForOrg(ctx context.Context, orgID int64) error {
	return nil
}
func (happyRepo) SaveManualDecision(ctx context.Context, orgID, userID int64, rawName string, productID int64, reason string) error {
	return nil
}
func (happyRepo) IsDecisionMemoryEnabled(ctx context.Context) bool {
	return true
}
func (happyRepo) SetDecisionMemoryEnabled(ctx context.Context, enabled bool) error {
	return nil
}
func (happyRepo) ListCustomerMappings(ctx context.Context, orgID int64, search string, limit, offset int) ([]*catalog.CustomerMappingView, int, error) {
	return nil, 0, nil
}
func (happyRepo) DeleteCustomerMapping(ctx context.Context, orgID, id int64) error {
	return nil
}
func (happyRepo) ClearCustomerMappings(ctx context.Context, orgID int64) error {
	return nil
}

func newAuthedRouter(repo catalog.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := catalog.NewService(repo, log)
	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := authctx.Actor{
				UserID:         1,
				OrganizationID: 1,
				Role:           "super_admin",
				Permissions:    []string{"admin", "super_admin", "catalog.admin"},
			}
			ctx := authctx.WithActor(r.Context(), actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	catalogHttp.NewHandler(svc, log).RegisterRoutes(r)
	return r
}

func TestCatalogHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"Search", http.MethodGet, "/api/v1/catalog/search?q=panadol", "", http.StatusOK},
		{"GetProduct", http.MethodGet, "/api/v1/catalog/products/1", "", http.StatusOK},
		{"ListProducts", http.MethodGet, "/api/v1/catalog/products?limit=10&offset=0", "", http.StatusOK},
		{"CreateProduct", http.MethodPost, "/api/v1/catalog/products", `{"organization_id":1,"name":{"en":"Panadol"},"dosage_form":"tablet"}`, http.StatusCreated},
		{"UpdateProduct", http.MethodPut, "/api/v1/catalog/products/1", `{"organization_id":1,"name":{"en":"Panadol Extra"},"dosage_form":"tablet"}`, http.StatusOK},
		{"DeleteProduct", http.MethodDelete, "/api/v1/catalog/products/1", "", http.StatusOK},
		{"BulkStatus", http.MethodPost, "/api/v1/catalog/products/bulk-status", `{"ids":[1,2],"status":"active"}`, http.StatusOK},
		{"CreateVariant", http.MethodPost, "/api/v1/catalog/products/1/variants", `{"organization_id":1,"name":{"en":"100mg"},"sku":"SKU-1","price":"50.00"}`, http.StatusCreated},
		{"GetVariant", http.MethodGet, "/api/v1/catalog/products/1/variants/1", "", http.StatusOK},
		{"UpdateVariant", http.MethodPut, "/api/v1/catalog/products/1/variants/1", `{"organization_id":1,"name":{"en":"100mg"},"sku":"SKU-1-UPDATED","price":"60.00"}`, http.StatusOK},
		{"DeleteVariant", http.MethodDelete, "/api/v1/catalog/products/1/variants/1", "", http.StatusNoContent},
		{"ListCategories", http.MethodGet, "/api/v1/catalog/categories", "", http.StatusOK},
		{"CreateCategory", http.MethodPost, "/api/v1/catalog/categories", `{"name":{"en":"Painkillers"},"status":"active"}`, http.StatusCreated},
		{"GetCategory", http.MethodGet, "/api/v1/catalog/categories/1", "", http.StatusOK},
		{"UpdateCategory", http.MethodPut, "/api/v1/catalog/categories/1", `{"name":{"en":"Painkillers Updated"},"status":"active"}`, http.StatusOK},
		{"DeleteCategory", http.MethodDelete, "/api/v1/catalog/categories/1", "", http.StatusNoContent},
		{"ListBrands", http.MethodGet, "/api/v1/catalog/brands", "", http.StatusOK},
		{"CreateBrand", http.MethodPost, "/api/v1/catalog/brands", `{"name":{"en":"GSK"},"status":"active"}`, http.StatusCreated},
		{"GetBrand", http.MethodGet, "/api/v1/catalog/brands/1", "", http.StatusOK},
		{"UpdateBrand", http.MethodPut, "/api/v1/catalog/brands/1", `{"name":{"en":"GSK Updated"},"status":"active"}`, http.StatusOK},
		{"DeleteBrand", http.MethodDelete, "/api/v1/catalog/brands/1", "", http.StatusNoContent},
		{"SetCustomerPricing", http.MethodPost, "/api/v1/catalog/pricing/customer", `{"organization_id":1,"customer_org_id":2,"product_id":1,"price":"45.00"}`, http.StatusOK},
		{"GetCustomerPricing", http.MethodGet, "/api/v1/catalog/pricing/customer?product_id=1&customer_org_id=2", "", http.StatusOK},
		{"CreateAlert", http.MethodPost, "/api/v1/catalog/alerts", `{"user_id":1,"product_id":1,"alert_type":"price_drop","target_price":"45.00"}`, http.StatusCreated},
		{"ListAlerts", http.MethodGet, "/api/v1/catalog/alerts", "", http.StatusOK},
		{"AdminProducts", http.MethodGet, "/api/v1/admin/catalog/products", "", http.StatusOK},
		{"AdminGetProduct", http.MethodGet, "/api/v1/admin/catalog/products/1", "", http.StatusOK},
		{"AdminDeactivate", http.MethodPost, "/api/v1/admin/catalog/products/1/deactivate", "", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tt.body != "" {
				bodyReader = strings.NewReader(tt.body)
			}
			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s got status %d, want %d (body: %s)", tt.method, tt.path, rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func (happyRepo) CountProductsByOrg(_ context.Context, _ int64, _ string) (int, error) { return 3, nil }

// Category -> brand relationship stubs (PLAN_V7 Phase 4). Behaviour is covered
// by the catalog repository tests; these only satisfy the interface.
func (h happyRepo) ListBrandsByCategory(context.Context, int64) ([]*catalog.Brand, error) {
	return nil, nil
}

func (h happyRepo) BrandInCategory(context.Context, int64, int64) (bool, error) {
	return true, nil
}

func (h happyRepo) SetBrandCategories(context.Context, int64, []int64) error {
	return nil
}
