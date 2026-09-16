package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

func (h *Handler) RegisterAdminRoutes(r chi.Router) {
	r.Group(func(admin chi.Router) {
		admin.Use(authctx.RequirePermission("catalog.admin"))

		admin.Get("/api/v1/admin/catalog/products", h.AdminListProducts)
		admin.Get("/api/v1/admin/catalog/products/{id}", h.AdminGetProduct)
		admin.Post("/api/v1/admin/catalog/products/{id}/deactivate", h.AdminDeactivateProduct)
		admin.Post("/api/v1/admin/catalog/categories", h.CreateCategory)
		admin.Put("/api/v1/admin/catalog/categories/{id}", h.UpdateCategory)
		admin.Delete("/api/v1/admin/catalog/categories/{id}", h.DeleteCategory)
		admin.Post("/api/v1/admin/catalog/brands", h.CreateBrand)
		admin.Put("/api/v1/admin/catalog/brands/{id}", h.UpdateBrand)
		admin.Delete("/api/v1/admin/catalog/brands/{id}", h.DeleteBrand)
	})
}

func (h *Handler) AdminListProducts(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())

	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	products, err := h.service.ListProducts(ctx, status, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"products": products})
}

func (h *Handler) AdminGetProduct(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid product ID", nil))
		return
	}

	product, variants, err := h.service.GetProduct(ctx, id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"product":  product,
		"variants": variants,
	})
}

func (h *Handler) AdminDeactivateProduct(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid product ID", nil))
		return
	}

	// Assuming ProductStatusInactive or "inactive" is valid
	if _, err := h.service.SetProductsStatus(ctx, []int64{id}, catalog.StatusInactive); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "deactivated"})
}
