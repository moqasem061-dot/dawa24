package http

import (
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Handler handles catalog HTTP requests.
type Handler struct {
	service *catalog.Service
	log     *slog.Logger
}

// NewHandler creates a catalog HTTP handler.
func NewHandler(service *catalog.Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes registers catalog endpoints on a Chi router.
//
// Reads are open to any authenticated, approved caller — the catalogue is the
// platform's shared reference and every dashboard reads it. Writes are not, and
// until now they were: every route below the read block was mounted with
// authentication and an approved-organisation check and NOTHING ELSE, so any
// employee of any approved pharmacy could POST a product into the shared
// catalogue, PUT over an existing one, or DELETE a category every other tenant
// depends on. The HTML admin screens have always been gated on
// catalog.product.* — the JSON API was simply the way around them.
//
// Two audiences, two gates:
//
//   - catalog.products / categories / brands are the ADMINISTRATOR's catalogue.
//     Gated on the same catalog.*.create/update/delete keys the admin pages use.
//   - product VARIANTS are a vendor's own offer against a master product, which
//     is why they take the vendor keys instead. RequirePermission rather than
//     RequireAPITenantPermission: a staff member fixing a supplier's listing
//     holds catalog.vendor_product.update and must not be refused for being
//     staff.
func (h *Handler) RegisterRoutes(r chi.Router) {
	// --- reads -------------------------------------------------------------
	// Feeds the cascading category -> brand selector in every product form.
	r.Get("/api/v1/catalog/categories/{id}/brands", h.ListBrandsByCategory)
	r.Get("/api/v1/catalog/search", h.Search)
	r.Get("/api/v1/catalog/products/{id}", h.GetProduct)
	r.Get("/api/v1/catalog/products", h.ListProducts)
	r.Get("/api/v1/catalog/products/{id}/variants/{variantId}", h.GetVariant)
	r.Get("/api/v1/catalog/categories/{id}", h.GetCategory)
	r.Get("/api/v1/catalog/brands/{id}", h.GetBrand)
	r.Get("/api/v1/catalog/categories", h.ListCategories)
	r.Get("/api/v1/catalog/brands", h.ListBrands)
	r.Get("/api/v1/catalog/pricing/customer", h.GetCustomerPricing)
	r.Get("/api/v1/catalog/alerts", h.ListProductAlerts)

	// --- master catalogue writes -------------------------------------------
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("catalog.product.create", "catalog.admin"))
		g.Post("/api/v1/catalog/products", h.CreateProduct)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("catalog.product.update", "catalog.admin"))
		g.Put("/api/v1/catalog/products/{id}", h.UpdateProduct)
		g.Post("/api/v1/catalog/products/bulk-status", h.SetProductsStatus)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("catalog.product.delete", "catalog.admin"))
		g.Delete("/api/v1/catalog/products/{id}", h.DeleteProduct)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("catalog.category.update", "catalog.admin"))
		g.Post("/api/v1/catalog/categories", h.CreateCategory)
		g.Put("/api/v1/catalog/categories/{id}", h.UpdateCategory)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("catalog.category.delete", "catalog.admin"))
		g.Delete("/api/v1/catalog/categories/{id}", h.DeleteCategory)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("catalog.brand.update", "catalog.brand.manage", "catalog.admin"))
		g.Post("/api/v1/catalog/brands", h.CreateBrand)
		g.Put("/api/v1/catalog/brands/{id}", h.UpdateBrand)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("catalog.brand.delete", "catalog.admin"))
		g.Delete("/api/v1/catalog/brands/{id}", h.DeleteBrand)
	})

	// --- a vendor's own variants and pricing --------------------------------
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("vendor.product.create", "catalog.vendor_product.update", "catalog.admin"))
		g.Post("/api/v1/catalog/products/{id}/variants", h.CreateVariant)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("vendor.product.update", "catalog.vendor_product.update", "catalog.admin"))
		g.Put("/api/v1/catalog/products/{id}/variants/{variantId}", h.UpdateVariant)
		g.Post("/api/v1/catalog/pricing/customer", h.SetCustomerPricing)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("vendor.product.delete", "catalog.vendor_product.update", "catalog.admin"))
		g.Delete("/api/v1/catalog/products/{id}/variants/{variantId}", h.DeleteVariant)
	})

	// A product alert is a caller's own reminder about a product. Any
	// authenticated member may set one for themselves; the service scopes it.
	r.Post("/api/v1/catalog/alerts", h.CreateProductAlert)

	h.RegisterAdminRoutes(r)
}

// Search handles product search queries.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	var categoryID *int64
	if catStr := r.URL.Query().Get("category_id"); catStr != "" {
		if val, err := strconv.ParseInt(catStr, 10, 64); err == nil {
			categoryID = &val
		}
	}

	var brandID *int64
	if brandStr := r.URL.Query().Get("brand_id"); brandStr != "" {
		if val, err := strconv.ParseInt(brandStr, 10, 64); err == nil {
			brandID = &val
		}
	}

	products, err := h.service.Search(r.Context(), catalog.SearchParams{
		Query:      query,
		CategoryID: categoryID,
		BrandID:    brandID,
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"products": products,
		"count":    len(products),
	})
}

// GetProduct retrieves a product and its variants.
func (h *Handler) GetProduct(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid product ID", nil))
		return
	}

	product, variants, err := h.service.GetProduct(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"product":  product,
		"variants": variants,
	})
}

// CreateProduct creates a new product.
func (h *Handler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	var p catalog.Product
	if err := httpx.DecodeJSON(w, r, &p); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	created, err := h.service.CreateProduct(r.Context(), &p)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, created)
}

// CreateVariant creates a product variant.
func (h *Handler) CreateVariant(w http.ResponseWriter, r *http.Request) {
	productIDStr := chi.URLParam(r, "id")
	productID, err := strconv.ParseInt(productIDStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid product ID", nil))
		return
	}

	var v catalog.ProductVariant
	if err := httpx.DecodeJSON(w, r, &v); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	v.ProductID = productID

	created, err := h.service.CreateVariant(r.Context(), &v)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, created)
}

// ListCategories lists product categories.
func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := h.service.ListCategories(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"categories": categories,
	})
}

// ListBrands lists product brands.
func (h *Handler) ListBrands(w http.ResponseWriter, r *http.Request) {
	brands, err := h.service.ListBrands(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"brands": brands,
	})
}

// ListBrandsByCategory returns the manufacturers that operate in one category,
// for the product form's brand selector.
func (h *Handler) ListBrandsByCategory(w http.ResponseWriter, r *http.Request) {
	categoryID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || categoryID <= 0 {
		httpx.Error(w, r, h.log, apperr.Validation("catalog.category_invalid",
			"Category id is invalid.", map[string]string{"id": i18n.TDefault("w4_mod.s_342_342")}))
		return
	}
	brands, err := h.service.ListBrandsByCategory(r.Context(), categoryID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"brands": brands})
}
