package catalog

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// CreateVariant creates a product variant.
func (s *Service) CreateVariant(ctx context.Context, v *ProductVariant) (*ProductVariant, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	v.OrganizationID = orgID

	if v.ProductID <= 0 {
		return nil, apperr.Validation("variant.product_required", "Parent product ID is required.", nil)
	}
	if v.Name.IsEmpty() {
		return nil, apperr.Validation("variant.name_required", "Variant name is required.", nil)
	}
	if v.Status == "" {
		v.Status = StatusActive
	}

	if err := s.repo.CreateVariant(ctx, v); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "variant created", "variant_id", v.ID, "product_id", v.ProductID)
	return v, nil
}

// GetVariantBySKUOrBarcode retrieves a variant by SKU or Barcode within an organization.
func (s *Service) GetVariantBySKUOrBarcode(ctx context.Context, orgID int64, sku, barcode string) (*ProductVariant, error) {
	return s.repo.GetVariantBySKUOrBarcode(ctx, orgID, sku, barcode)
}

// GetVariantByProductAndOrg retrieves a variant by parent product ID and organization ID.
func (s *Service) GetVariantByProductAndOrg(ctx context.Context, orgID int64, productID int64) (*ProductVariant, error) {
	return s.repo.GetVariantByProductAndOrg(ctx, orgID, productID)
}

// ListVariantsByProduct retrieves all variants for a parent product.
func (s *Service) ListVariantsByProduct(ctx context.Context, productID int64) ([]*ProductVariant, error) {
	return s.repo.ListVariantsByProduct(ctx, productID)
}

// ListVariantsByOrganization retrieves all variants sold by a supplier with pagination and search.
func (s *Service) ListVariantsByOrganization(ctx context.Context, orgID int64, params VariantSearchParams) ([]*ProductVariant, int, error) {
	return s.repo.ListVariantsByOrganization(ctx, orgID, params)
}

// ListAllVariants retrieves all variants across all vendors with pagination and search (for admin panel).
func (s *Service) ListAllVariants(ctx context.Context, params VariantSearchParams) ([]*ProductVariant, int, error) {
	return s.repo.ListAllVariants(ctx, params)
}

// ListCategories returns all categories.
func (s *Service) ListCategories(ctx context.Context) ([]*Category, error) {
	if s.cache != nil {
		var cached []*Category
		if err := s.cache.GetJSON(ctx, "catalog:global:categories", &cached); err == nil && len(cached) > 0 {
			return cached, nil
		}
	}
	cats, err := s.repo.ListCategories(ctx)
	if err != nil {
		return nil, err
	}
	if s.cache != nil && len(cats) > 0 {
		_ = s.cache.SetJSON(ctx, "catalog:global:categories", cats, 30*time.Minute)
	}
	return cats, nil
}

// CreateCategory adds a category.
func (s *Service) CreateCategory(ctx context.Context, c *Category) (*Category, error) {
	if c.Name.IsEmpty() {
		return nil, apperr.Validation("category.name_required", "Category name is required.", nil)
	}
	if err := s.repo.CreateCategory(ctx, c); err != nil {
		return nil, err
	}
	if s.cache != nil {
		_ = s.cache.Delete(ctx, "catalog:global:categories")
	}
	return c, nil
}

// GetCategory retrieves a category.
func (s *Service) GetCategory(ctx context.Context, id int64) (*Category, error) {
	return s.repo.GetCategoryByID(ctx, id)
}

// UpdateCategory modifies category data.
func (s *Service) UpdateCategory(ctx context.Context, c *Category) error {
	if err := s.repo.UpdateCategory(ctx, c); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.Delete(ctx, "catalog:global:categories")
	}
	return nil
}

// CountProductsInCategory returns the number of products linked to a category.
func (s *Service) CountProductsInCategory(ctx context.Context, categoryID int64) (int, error) {
	return s.repo.CountProductsInCategory(ctx, categoryID)
}

// ListCategoriesWithProductCount returns paginated categories with live product counts and total matching count.
func (s *Service) ListCategoriesWithProductCount(ctx context.Context, search, status string, limit, offset int) ([]*CategoryWithCount, int, error) {
	return s.repo.ListCategoriesWithProductCount(ctx, search, status, limit, offset)
}

// ListBrands returns all brands.
func (s *Service) ListBrands(ctx context.Context) ([]*Brand, error) {
	if s.cache != nil {
		var cached []*Brand
		if err := s.cache.GetJSON(ctx, "catalog:global:brands", &cached); err == nil && len(cached) > 0 {
			return cached, nil
		}
	}
	brands, err := s.repo.ListBrands(ctx)
	if err != nil {
		return nil, err
	}
	if s.cache != nil && len(brands) > 0 {
		_ = s.cache.SetJSON(ctx, "catalog:global:brands", brands, 30*time.Minute)
	}
	return brands, nil
}

// ListBrandsWithProductCount returns paginated brands with live product counts and total matching count.
func (s *Service) ListBrandsWithProductCount(ctx context.Context, search, status string, limit, offset int) ([]*BrandWithCount, int, error) {
	return s.repo.ListBrandsWithProductCount(ctx, search, status, limit, offset)
}

// CreateBrand adds a brand.
func (s *Service) CreateBrand(ctx context.Context, b *Brand) (*Brand, error) {
	if b.Name.IsEmpty() {
		return nil, apperr.Validation("brand.name_required", "Brand name is required.", nil)
	}
	if err := s.repo.CreateBrand(ctx, b); err != nil {
		return nil, err
	}
	if s.cache != nil {
		_ = s.cache.Delete(ctx, "catalog:global:brands")
	}
	return b, nil
}

// GetBrand retrieves a brand.
func (s *Service) GetBrand(ctx context.Context, id int64) (*Brand, error) {
	return s.repo.GetBrandByID(ctx, id)
}

// UpdateBrand modifies brand data.
func (s *Service) UpdateBrand(ctx context.Context, b *Brand) error {
	if err := s.repo.UpdateBrand(ctx, b); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.Delete(ctx, "catalog:global:brands")
	}
	return nil
}

// CountProductsInBrand returns the number of active products linked to a brand.
func (s *Service) CountProductsInBrand(ctx context.Context, brandID int64) (int, error) {
	return s.repo.CountProductsInBrand(ctx, brandID)
}

// SetCustomerPricing stores customer-specific custom price or discount terms.
func (s *Service) SetCustomerPricing(ctx context.Context, m *CustomerProductMapping) error {
	if m.OrganizationID <= 0 || m.CustomerOrgID == nil || *m.CustomerOrgID <= 0 || m.ProductID <= 0 {
		return apperr.Validation("mapping.invalid", "Organization, customer, and product IDs are required.", nil)
	}
	return s.repo.SetCustomerPricing(ctx, m)
}

// GetCustomerPricing looks up custom pricing for a customer.
func (s *Service) GetCustomerPricing(ctx context.Context, vendorOrgID, customerOrgID, productID int64) (*CustomerProductMapping, error) {
	return s.repo.GetCustomerPricing(ctx, vendorOrgID, customerOrgID, productID)
}

// CreateProductAlert registers a price/stock alert.
func (s *Service) CreateProductAlert(ctx context.Context, a *ProductAlert) (*ProductAlert, error) {
	if a.UserID <= 0 || a.ProductID <= 0 || a.AlertType == "" {
		return nil, apperr.Validation("alert.invalid", "User ID, product ID, and alert type are required.", nil)
	}
	if err := s.repo.CreateProductAlert(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// ListProductAlerts returns active alerts for a user.
func (s *Service) ListProductAlerts(ctx context.Context, userID int64) ([]*ProductAlert, error) {
	return s.repo.ListProductAlertsByUser(ctx, userID)
}

// CountProductsByOrg returns an organization's product total for a status.
// Pass an empty status for every product.
func (s *Service) CountProductsByOrg(ctx context.Context, orgID int64, status string) (int, error) {
	return s.repo.CountProductsByOrg(ctx, orgID, status)
}

// CreateSavingProduct registers a saving target product.
func (s *Service) CreateSavingProduct(ctx context.Context, sp *SavingProduct) error {
	if sp.NameProduct == "" {
		return apperr.Validation("name_product.required", i18n.TDefault("w4_mod.w4str_116_116"), nil)
	}
	return s.repo.CreateSavingProduct(ctx, sp)
}

// UpdateSavingProduct updates an existing saving product.
func (s *Service) UpdateSavingProduct(ctx context.Context, sp *SavingProduct) error {
	if sp.NameProduct == "" {
		return apperr.Validation("name_product.required", i18n.TDefault("w4_mod.w4str_116_116"), nil)
	}
	return s.repo.UpdateSavingProduct(ctx, sp)
}

// ListSavingProducts returns saving products for an organization.
func (s *Service) ListSavingProducts(ctx context.Context, orgID int64, limit, offset int) ([]*SavingProduct, error) {
	return s.repo.ListSavingProductsByOrg(ctx, orgID, limit, offset)
}

// ListSavingProductsEnriched returns enriched saving products with linked details and counts.
func (s *Service) ListSavingProductsEnriched(ctx context.Context, orgID int64, search, filter string, limit, offset int) ([]*SavingProductEnriched, *SavingProductStats, error) {
	return s.repo.ListSavingProductsEnriched(ctx, orgID, search, filter, limit, offset)
}

// GetSavingProduct returns a single saving product by ID.
func (s *Service) GetSavingProduct(ctx context.Context, id int64) (*SavingProduct, error) {
	return s.repo.GetSavingProductByID(ctx, id)
}

// DeleteSavingProduct removes a saving product record.
func (s *Service) DeleteSavingProduct(ctx context.Context, id, orgID int64) error {
	return s.repo.DeleteSavingProduct(ctx, id, orgID)
}

// DeleteAllSavingProducts removes all saving product records for an organization.
func (s *Service) DeleteAllSavingProducts(ctx context.Context, orgID int64) error {
	return s.repo.DeleteAllSavingProducts(ctx, orgID)
}

// GetProductProviders returns all suppliers and variants selling a master catalog product.
func (s *Service) GetProductProviders(ctx context.Context, productID int64) ([]*ProductProviderInfo, error) {
	return s.repo.GetProductProviders(ctx, productID)
}

// BatchUpsertSavingProducts inserts or updates saving products in bulk for an organization.
func (s *Service) BatchUpsertSavingProducts(ctx context.Context, orgID int64, userID *int64, items []*SavingProduct) (added, updated int, err error) {
	return s.repo.BatchUpsertSavingProducts(ctx, orgID, userID, items)
}

// ListAllSavingProductsAdmin returns all saving products across all users and organizations for platform administration.
func (s *Service) ListAllSavingProductsAdmin(ctx context.Context, userID *int64, orgID *int64, search string, filter string, limit, offset int) ([]*SavingProductAdminView, *SavingProductAdminStats, error) {
	return s.repo.ListAllSavingProductsAdmin(ctx, userID, orgID, search, filter, limit, offset)
}

// ListAllMasterProductsForMatching retrieves all master products for high-speed in-memory matching.
func (s *Service) ListAllMasterProductsForMatching(ctx context.Context) ([]*CatalogMatchSource, error) {
	return s.repo.ListAllMasterProductsForMatching(ctx)
}

// DeleteAllVariantsByOrg soft-deletes all variants of an organization.
func (s *Service) DeleteAllVariantsByOrg(ctx context.Context, orgID int64) (int64, error) {
	return s.repo.DeleteAllVariantsByOrg(ctx, orgID)
}

// ActivateAllVariantsByOrg activates all variants of an organization.
func (s *Service) ActivateAllVariantsByOrg(ctx context.Context, orgID int64) (int64, error) {
	return s.repo.ActivateAllVariantsByOrg(ctx, orgID)
}

// DeleteAllProducts soft-deletes all master products in the system (Super Admin).
func (s *Service) DeleteAllProducts(ctx context.Context) (int64, error) {
	return s.repo.DeleteAllProducts(ctx)
}

// GetProductBySKU looks up a master product by its exact SKU or barcode.
func (s *Service) GetProductBySKU(ctx context.Context, sku string) (*Product, error) {
	return s.repo.GetProductBySKU(ctx, sku)
}

// GetProductByBarcode looks up a master product by its exact barcode.
func (s *Service) GetProductByBarcode(ctx context.Context, barcode string) (*Product, error) {
	return s.repo.GetProductByBarcode(ctx, barcode)
}

// UpdateProductImageBySKU downloads and sets product image by its matching SKU.
func (s *Service) UpdateProductImageBySKU(ctx context.Context, sku string, imagePath string, imageLink string) (*Product, error) {
	return s.repo.UpdateProductImageBySKU(ctx, sku, imagePath, imageLink)
}

// UpdateProductImageByID downloads and sets product image by product ID.
func (s *Service) UpdateProductImageByID(ctx context.Context, id int64, imagePath string, imageLink string) (*Product, error) {
	return s.repo.UpdateProductImageByID(ctx, id, imagePath, imageLink)
}

// ListMatchDecisions returns platform-wide decision memory records.
func (s *Service) ListMatchDecisions(ctx context.Context, search string, limit, offset int) ([]*MatchDecisionView, int, error) {
	return s.repo.ListMatchDecisions(ctx, search, limit, offset)
}

// ListMatchDecisionsForOrg returns decision memories scoped to an organization.
func (s *Service) ListMatchDecisionsForOrg(ctx context.Context, orgID int64, search string, limit, offset int) ([]*MatchDecisionView, int, error) {
	return s.repo.ListMatchDecisionsForOrg(ctx, orgID, search, limit, offset)
}

// DeleteMatchDecision removes a single match decision from the system cache.
func (s *Service) DeleteMatchDecision(ctx context.Context, id int64) error {
	return s.repo.DeleteMatchDecision(ctx, id)
}

// DeleteMatchDecisionForOrg removes a single match decision belonging to an organization.
func (s *Service) DeleteMatchDecisionForOrg(ctx context.Context, orgID, id int64) error {
	return s.repo.DeleteMatchDecisionForOrg(ctx, orgID, id)
}

// ClearMatchDecisions purges all cached matching decisions from the system.
func (s *Service) ClearMatchDecisions(ctx context.Context) error {
	return s.repo.ClearMatchDecisions(ctx)
}

// ClearMatchDecisionsForOrg purges all cached matching decisions for a single organization.
func (s *Service) ClearMatchDecisionsForOrg(ctx context.Context, orgID int64) error {
	return s.repo.ClearMatchDecisionsForOrg(ctx, orgID)
}

// SaveManualDecision records a user-indicated match decision.
func (s *Service) SaveManualDecision(ctx context.Context, orgID, userID int64, rawName string, productID int64, reason string) error {
	return s.repo.SaveManualDecision(ctx, orgID, userID, rawName, productID, reason)
}

// IsDecisionMemoryEnabled checks if the decision memory feature is enabled in system settings.
func (s *Service) IsDecisionMemoryEnabled(ctx context.Context) bool {
	return s.repo.IsDecisionMemoryEnabled(ctx)
}

// SetDecisionMemoryEnabled updates the decision memory feature state in system settings.
func (s *Service) SetDecisionMemoryEnabled(ctx context.Context, enabled bool) error {
	return s.repo.SetDecisionMemoryEnabled(ctx, enabled)
}

// ListCustomerMappings returns the saved learned matching decisions for a customer/vendor organization.
func (s *Service) ListCustomerMappings(ctx context.Context, orgID int64, search string, limit, offset int) ([]*CustomerMappingView, int, error) {
	return s.repo.ListCustomerMappings(ctx, orgID, search, limit, offset)
}

// DeleteCustomerMapping removes a saved product mapping for an organization.
func (s *Service) DeleteCustomerMapping(ctx context.Context, orgID, id int64) error {
	return s.repo.DeleteCustomerMapping(ctx, orgID, id)
}

// ClearCustomerMappings purges all saved product mappings for an organization.
func (s *Service) ClearCustomerMappings(ctx context.Context, orgID int64) error {
	return s.repo.ClearCustomerMappings(ctx, orgID)
}
