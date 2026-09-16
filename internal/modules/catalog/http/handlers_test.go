package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	catalogHttp "github.com/muhiya/dawa24-store/internal/modules/catalog/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) CreateProduct(ctx context.Context, p *catalog.Product) error {
	r.fail("CreateProduct")
	return nil
}
func (r stubRepo) BulkUpsertProducts(ctx context.Context, prods []*catalog.Product, _ catalog.BulkWriteOptions) (catalog.BulkWriteResult, error) {
	r.fail("BulkUpsertProducts")
	return catalog.BulkWriteResult{}, nil
}
func (r stubRepo) GetProductByID(ctx context.Context, id int64) (*catalog.Product, error) {
	r.fail("GetProductByID")
	return nil, nil
}
func (r stubRepo) UpdateProduct(ctx context.Context, p *catalog.Product) error {
	r.fail("UpdateProduct")
	return nil
}
func (r stubRepo) DeleteProduct(ctx context.Context, id int64) error {
	r.fail("DeleteProduct")
	return nil
}
func (r stubRepo) SearchProducts(ctx context.Context, params catalog.SearchParams) ([]*catalog.Product, error) {
	r.fail("SearchProducts")
	return nil, nil
}
func (r stubRepo) CountProducts(ctx context.Context, params catalog.SearchParams) (int, error) {
	r.fail("CountProducts")
	return 0, nil
}
func (r stubRepo) ListProducts(ctx context.Context, status string, limit, offset int) ([]*catalog.Product, error) {
	r.fail("ListProducts")
	return nil, nil
}
func (r stubRepo) SetProductsStatus(ctx context.Context, ids []int64, status catalog.ProductStatus) (int64, error) {
	r.fail("SetProductsStatus")
	return 0, nil
}

func (r stubRepo) CreateVariant(ctx context.Context, v *catalog.ProductVariant) error {
	r.fail("CreateVariant")
	return nil
}
func (r stubRepo) GetVariantByID(ctx context.Context, id int64) (*catalog.ProductVariant, error) {
	r.fail("GetVariantByID")
	return nil, nil
}
func (r stubRepo) GetVariantBySKUOrBarcode(ctx context.Context, orgID int64, sku, barcode string) (*catalog.ProductVariant, error) {
	r.fail("GetVariantBySKUOrBarcode")
	return nil, nil
}
func (r stubRepo) GetVariantByProductAndOrg(ctx context.Context, orgID int64, productID int64) (*catalog.ProductVariant, error) {
	r.fail("GetVariantByProductAndOrg")
	return nil, nil
}
func (r stubRepo) ListVariantsByProduct(ctx context.Context, productID int64) ([]*catalog.ProductVariant, error) {
	r.fail("ListVariantsByProduct")
	return nil, nil
}
func (r stubRepo) ListVariantsByProducts(ctx context.Context, productIDs []int64) ([]*catalog.ProductVariant, error) {
	r.fail("ListVariantsByProducts")
	return nil, nil
}

func (r stubRepo) GetVariantsByIDs(context.Context, []int64) (map[int64]*catalog.ProductVariant, error) {
	return map[int64]*catalog.ProductVariant{}, nil
}
func (r stubRepo) ListVariantsByOrganization(ctx context.Context, orgID int64, params catalog.VariantSearchParams) ([]*catalog.ProductVariant, int, error) {
	r.fail("ListVariantsByOrganization")
	return nil, 0, nil
}
func (r stubRepo) ListBuyerOffers(ctx context.Context, q catalog.BuyerOfferQuery) ([]*catalog.BuyerOffer, int, error) {
	r.fail("ListBuyerOffers")
	return nil, 0, nil
}
func (r stubRepo) ListAllVariants(ctx context.Context, params catalog.VariantSearchParams) ([]*catalog.ProductVariant, int, error) {
	r.fail("ListAllVariants")
	return nil, 0, nil
}
func (r stubRepo) UpdateVariant(ctx context.Context, v *catalog.ProductVariant) error {
	r.fail("UpdateVariant")
	return nil
}
func (r stubRepo) DeleteVariant(ctx context.Context, id int64) error {
	r.fail("DeleteVariant")
	return nil
}
func (r stubRepo) ToggleVariantStatus(ctx context.Context, orgID, variantID int64) (catalog.ProductStatus, error) {
	r.fail("ToggleVariantStatus")
	return "", nil
}

func (r stubRepo) CreateCategory(ctx context.Context, c *catalog.Category) error {
	r.fail("CreateCategory")
	return nil
}
func (r stubRepo) GetCategoryByID(ctx context.Context, id int64) (*catalog.Category, error) { r.fail("GetCategoryByID"); return nil, nil }
func (r stubRepo) UpdateCategory(ctx context.Context, c *catalog.Category) error { r.fail("UpdateCategory"); return nil }
func (r stubRepo) DeleteCategory(ctx context.Context, id int64) error { r.fail("DeleteCategory"); return nil }
func (r stubRepo) ListCategories(ctx context.Context) ([]*catalog.Category, error) {
	r.fail("ListCategories")
	return nil, nil
}
func (r stubRepo) ListCategoriesWithProductCount(ctx context.Context, _ string, _ string, limit, offset int) ([]*catalog.CategoryWithCount, int, error) {
	r.fail("ListCategoriesWithProductCount")
	return nil, 0, nil
}
func (r stubRepo) CountProductsInCategory(ctx context.Context, categoryID int64) (int, error) {
	r.fail("CountProductsInCategory")
	return 0, nil
}

func (r stubRepo) CreateBrand(ctx context.Context, b *catalog.Brand) error {
	r.fail("CreateBrand")
	return nil
}
func (r stubRepo) GetBrandByID(ctx context.Context, id int64) (*catalog.Brand, error) {
	r.fail("GetBrandByID")
	return nil, nil
}
func (r stubRepo) UpdateBrand(ctx context.Context, b *catalog.Brand) error {
	r.fail("UpdateBrand")
	return nil
}
func (r stubRepo) DeleteBrand(ctx context.Context, id int64) error {
	r.fail("DeleteBrand")
	return nil
}
func (r stubRepo) ListBrands(ctx context.Context) ([]*catalog.Brand, error) {
	r.fail("ListBrands")
	return nil, nil
}
func (r stubRepo) ListBrandsWithProductCount(ctx context.Context, _, _ string, _, _ int) ([]*catalog.BrandWithCount, int, error) {
	r.fail("ListBrandsWithProductCount")
	return nil, 0, nil
}

func (r stubRepo) SetCustomerPricing(ctx context.Context, m *catalog.CustomerProductMapping) error {
	r.fail("SetCustomerPricing")
	return nil
}
func (r stubRepo) GetCustomerPricing(ctx context.Context, organizationID, customerOrgID, productID int64) (*catalog.CustomerProductMapping, error) {
	r.fail("GetCustomerPricing")
	return nil, nil
}
func (r stubRepo) CreateProductAlert(ctx context.Context, a *catalog.ProductAlert) error {
	r.fail("CreateProductAlert")
	return nil
}
func (r stubRepo) ListProductAlertsByUser(ctx context.Context, userID int64) ([]*catalog.ProductAlert, error) {
	r.fail("ListProductAlertsByUser")
	return nil, nil
}

func (r stubRepo) CountProductsInBrand(ctx context.Context, brandID int64) (int, error) {
	r.fail("CountProductsInBrand")
	return 0, nil
}
func (r stubRepo) UpsertProductIndex(ctx context.Context, item *catalog.ProductIndexItem) error {
	r.fail("UpsertProductIndex")
	return nil
}
func (r stubRepo) DeleteProductIndex(ctx context.Context, uniqueRowID string) error {
	r.fail("DeleteProductIndex")
	return nil
}
func (r stubRepo) DeleteProductIndexByProduct(ctx context.Context, productID int64) error {
	r.fail("DeleteProductIndexByProduct")
	return nil
}
func (r stubRepo) SearchProductIndex(ctx context.Context, params catalog.SearchParams) ([]*catalog.ProductIndexItem, error) {
	r.fail("SearchProductIndex")
	return nil, nil
}
func (r stubRepo) RebuildProductIndex(ctx context.Context) (int64, error) {
	r.fail("RebuildProductIndex")
	return 0, nil
}
func (r stubRepo) CreateSavingProduct(ctx context.Context, sp *catalog.SavingProduct) error {
	r.fail("CreateSavingProduct")
	return nil
}
func (r stubRepo) ListSavingProductsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*catalog.SavingProduct, error) {
	r.fail("ListSavingProductsByOrg")
	return nil, nil
}
func (r stubRepo) GetSavingProductByID(ctx context.Context, id int64) (*catalog.SavingProduct, error) {
	r.fail("GetSavingProductByID")
	return nil, nil
}
func (r stubRepo) DeleteSavingProduct(ctx context.Context, id, orgID int64) error {
	r.fail("DeleteSavingProduct")
	return nil
}
func (r stubRepo) UpdateSavingProduct(ctx context.Context, sp *catalog.SavingProduct) error {
	r.fail("UpdateSavingProduct")
	return nil
}
func (r stubRepo) ListSavingProductsEnriched(ctx context.Context, orgID int64, search, filter string, limit, offset int) ([]*catalog.SavingProductEnriched, *catalog.SavingProductStats, error) {
	r.fail("ListSavingProductsEnriched")
	return nil, nil, nil
}
func (r stubRepo) GetProductProviders(ctx context.Context, productID int64) ([]*catalog.ProductProviderInfo, error) {
	r.fail("GetProductProviders")
	return nil, nil
}
func (r stubRepo) BatchUpsertSavingProducts(ctx context.Context, orgID int64, userID *int64, items []*catalog.SavingProduct) (int, int, error) {
	r.fail("BatchUpsertSavingProducts")
	return 0, 0, nil
}
func (r stubRepo) ListAllSavingProductsAdmin(ctx context.Context, _ *int64, _ *int64, _ string, _ string, _, _ int) ([]*catalog.SavingProductAdminView, *catalog.SavingProductAdminStats, error) {
	r.fail("ListAllSavingProductsAdmin")
	return nil, nil, nil
}
func (r stubRepo) ListAllMasterProductsForMatching(ctx context.Context) ([]*catalog.CatalogMatchSource, error) {
	return nil, nil
}
func (r stubRepo) DeleteAllVariantsByOrg(ctx context.Context, orgID int64) (int64, error) {
	return 0, nil
}
func (r stubRepo) ActivateAllVariantsByOrg(ctx context.Context, orgID int64) (int64, error) {
	return 0, nil
}
func (r stubRepo) DeleteAllProducts(ctx context.Context) (int64, error) {
	return 0, nil
}
func (r stubRepo) DeleteAllSavingProducts(ctx context.Context, orgID int64) error {
	return nil
}
func (r stubRepo) GetProductBySKU(ctx context.Context, sku string) (*catalog.Product, error) {
	return nil, apperr.NotFound("product")
}
func (r stubRepo) GetProductByBarcode(ctx context.Context, barcode string) (*catalog.Product, error) {
	return nil, apperr.NotFound("product")
}
func (r stubRepo) UpdateProductImageBySKU(ctx context.Context, sku string, imagePath, imageLink string) (*catalog.Product, error) {
	return nil, nil
}
func (r stubRepo) UpdateProductImageByID(ctx context.Context, id int64, imagePath, imageLink string) (*catalog.Product, error) {
	return nil, nil
}
func (r stubRepo) ListMatchDecisions(ctx context.Context, search string, limit, offset int) ([]*catalog.MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (r stubRepo) ListMatchDecisionsFiltered(ctx context.Context, f catalog.DecisionMemoryFilter) ([]*catalog.MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (r stubRepo) ListMatchDecisionsForOrgWithPlatform(ctx context.Context, orgID int64, search string, limit, offset int) ([]*catalog.MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (r stubRepo) GetDecisionMemoryPreference(ctx context.Context, orgID int64) (bool, error) {
	return true, nil
}
func (r stubRepo) SetDecisionMemoryPreference(ctx context.Context, orgID int64, usePlatform bool, updatedBy int64) error {
	return nil
}
func (r stubRepo) PromoteMatchDecision(ctx context.Context, id int64, adminUserID int64) error {
	return nil
}
func (r stubRepo) DemoteMatchDecision(ctx context.Context, id int64) error {
	return nil
}
func (r stubRepo) BulkPromoteMatchDecisions(ctx context.Context, ids []int64, adminUserID int64) (int64, error) {
	return 0, nil
}
func (r stubRepo) BulkDeleteMatchDecisions(ctx context.Context, ids []int64) (int64, error) {
	return 0, nil
}
func (r stubRepo) RelinkMatchDecision(ctx context.Context, id int64, productID *int64, adminUserID int64) error {
	return nil
}
func (r stubRepo) RelinkMatchDecisionForOrg(ctx context.Context, orgID int64, id int64, productID *int64, userID int64) error {
	return nil
}
func (r stubRepo) DeleteMatchDecision(ctx context.Context, id int64) error {
	return nil
}
func (r stubRepo) ClearMatchDecisions(ctx context.Context) error {
	return nil
}
func (r stubRepo) ListMatchDecisionsForOrg(ctx context.Context, orgID int64, search string, limit, offset int) ([]*catalog.MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (r stubRepo) DeleteMatchDecisionForOrg(ctx context.Context, orgID, id int64) error {
	return nil
}
func (r stubRepo) ClearMatchDecisionsForOrg(ctx context.Context, orgID int64) error {
	return nil
}
func (r stubRepo) SaveManualDecision(ctx context.Context, orgID, userID int64, rawName string, productID int64, reason string) error {
	return nil
}
func (r stubRepo) IsDecisionMemoryEnabled(ctx context.Context) bool { return true }
func (r stubRepo) SetDecisionMemoryEnabled(ctx context.Context, enabled bool) error { return nil }
func (r stubRepo) ListCustomerMappings(ctx context.Context, orgID int64, search string, limit, offset int) ([]*catalog.CustomerMappingView, int, error) {
	return nil, 0, nil
}
func (r stubRepo) DeleteCustomerMapping(ctx context.Context, orgID, id int64) error { return nil }
func (r stubRepo) ClearCustomerMappings(ctx context.Context, orgID int64) error { return nil }
func (r stubRepo) ListDosageForms(ctx context.Context) ([]string, error) { return nil, nil }

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := catalog.NewService(stubRepo{t: t}, log)
	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("dawa24_session")
			if err != nil || cookie.Value == "" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			if cookie.Value == "forged-token-that-was-never-issued" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			// A valid session now also has to carry the permissions the write
			// routes require. They used to require none: the catalogue's JSON
			// writes were mounted behind authentication alone, which is the
			// hole RegisterRoutes closed. The tests in this file are about the
			// error envelope and the body decoding, so the actor holds the
			// catalogue's API key and the assertions still measure what they
			// were written to measure.
			ctx := authctx.WithActor(r.Context(), authctx.Actor{
				UserID:         1,
				OrganizationID: 1,
				IsStaff:        true,
				Role:           "admin",
				Permissions:    []string{"catalog.admin"},
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	catalogHttp.NewHandler(svc, log).RegisterRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/catalog/search"},
	{http.MethodGet, "/api/v1/catalog/products/1"},
	{http.MethodPut, "/api/v1/catalog/products/1"},
	{http.MethodDelete, "/api/v1/catalog/products/1"},
	{http.MethodPost, "/api/v1/catalog/products"},
	{http.MethodGet, "/api/v1/catalog/products"},
	{http.MethodPost, "/api/v1/catalog/products/bulk-status"},
	{http.MethodPost, "/api/v1/catalog/products/1/variants"},
	{http.MethodGet, "/api/v1/catalog/products/1/variants/1"},
	{http.MethodPut, "/api/v1/catalog/products/1/variants/1"},
	{http.MethodDelete, "/api/v1/catalog/products/1/variants/1"},
	{http.MethodGet, "/api/v1/catalog/categories/1"},
	{http.MethodPut, "/api/v1/catalog/categories/1"},
	{http.MethodDelete, "/api/v1/catalog/categories/1"},
	{http.MethodGet, "/api/v1/catalog/brands/1"},
	{http.MethodPut, "/api/v1/catalog/brands/1"},
	{http.MethodDelete, "/api/v1/catalog/brands/1"},
	{http.MethodGet, "/api/v1/catalog/categories"},
	{http.MethodPost, "/api/v1/catalog/categories"},
	{http.MethodGet, "/api/v1/catalog/brands"},
	{http.MethodPost, "/api/v1/catalog/brands"},
	{http.MethodPost, "/api/v1/catalog/pricing/customer"},
	{http.MethodGet, "/api/v1/catalog/pricing/customer"},
	{http.MethodPost, "/api/v1/catalog/alerts"},
	{http.MethodGet, "/api/v1/catalog/alerts"},
}
