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

// CreateCategory adds a category.
func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var c catalog.Category
	if err := httpx.DecodeJSON(w, r, &c); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	created, err := h.service.CreateCategory(r.Context(), &c)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, created)
}

// CreateBrand adds a brand.
func (h *Handler) CreateBrand(w http.ResponseWriter, r *http.Request) {
	var b catalog.Brand
	if err := httpx.DecodeJSON(w, r, &b); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	created, err := h.service.CreateBrand(r.Context(), &b)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, created)
}

// SetCustomerPricing sets custom pricing.
func (h *Handler) SetCustomerPricing(w http.ResponseWriter, r *http.Request) {
	var m catalog.CustomerProductMapping
	if err := httpx.DecodeJSON(w, r, &m); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	if err := h.service.SetCustomerPricing(r.Context(), &m); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// GetCustomerPricing gets custom pricing.
func (h *Handler) GetCustomerPricing(w http.ResponseWriter, r *http.Request) {
	// The vendor is the caller's active tenant. Accepting it from the query
	// string let one supplier read the private per-customer pricing another
	// supplier had negotiated.
	vendorOrgID, ok := database.TenantFrom(r.Context())
	if !ok {
		httpx.Error(w, r, h.log, database.ErrNoTenant)
		return
	}
	customerOrgID, _ := strconv.ParseInt(r.URL.Query().Get("customer_org_id"), 10, 64)
	productID, _ := strconv.ParseInt(r.URL.Query().Get("product_id"), 10, 64)

	m, err := h.service.GetCustomerPricing(r.Context(), vendorOrgID, customerOrgID, productID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m)
}

// CreateProductAlert registers price/stock alert.
func (h *Handler) CreateProductAlert(w http.ResponseWriter, r *http.Request) {
	var a catalog.ProductAlert
	if err := httpx.DecodeJSON(w, r, &a); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	created, err := h.service.CreateProductAlert(r.Context(), &a)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, created)
}

// ListProductAlerts lists alerts for user.
func (h *Handler) ListProductAlerts(w http.ResponseWriter, r *http.Request) {
	// The acting user comes from the authenticated session, never from the
	// request. Reading it from the query string let any caller act as any
	// user by changing a number.
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	alerts, err := h.service.ListProductAlerts(r.Context(), userID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"alerts": alerts, "count": len(alerts)})
}

// UpdateProduct handles product modifications.
func (h *Handler) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid product ID", nil))
		return
	}
	var p catalog.Product
	if err := httpx.DecodeJSON(w, r, &p); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	p.ID = id
	if err := h.service.UpdateProduct(r.Context(), &p); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteProduct soft-deletes a product.
func (h *Handler) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid product ID", nil))
		return
	}
	if err := h.service.DeleteProduct(r.Context(), id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
