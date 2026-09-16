package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
)

func catalogID(r *http.Request, name, entity string) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.Validation("id.invalid", "Invalid "+entity+" identifier.",
			map[string]string{name: "must be a positive integer"})
	}
	return id, nil
}

// ListProducts returns the vendor's own catalogue.
func (h *Handler) ListProducts(w http.ResponseWriter, r *http.Request) {
	p := pagination.FromRequest(r)

	products, err := h.service.ListProducts(r.Context(), r.URL.Query().Get("status"), p.Limit, p.Offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, pagination.Page[*catalog.Product]{
		Data:    products,
		HasMore: len(products) == p.Limit,
	})
}

// SetProductsStatus activates or deactivates several products at once.
func (h *Handler) SetProductsStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs    []int64 `json:"ids"`
		Status string  `json:"status"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	updated, err := h.service.SetProductsStatus(r.Context(), body.IDs, catalog.ProductStatus(body.Status))
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	// Reporting requested alongside updated tells the caller when some ids were
	// not theirs, without revealing which.
	httpx.JSON(w, http.StatusOK, map[string]any{
		"updated":   updated,
		"requested": len(body.IDs),
	})
}

// GetVariant returns one variant.
func (h *Handler) GetVariant(w http.ResponseWriter, r *http.Request) {
	id, err := catalogID(r, "variantId", "variant")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	variant, err := h.service.GetVariant(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, variant)
}

// UpdateVariant edits a variant.
func (h *Handler) UpdateVariant(w http.ResponseWriter, r *http.Request) {
	id, err := catalogID(r, "variantId", "variant")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var input catalog.ProductVariant
	if err := httpx.DecodeJSON(w, r, &input); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	updated, err := h.service.UpdateVariant(r.Context(), id, &input)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, updated)
}

// DeleteVariant soft-deletes a variant.
func (h *Handler) DeleteVariant(w http.ResponseWriter, r *http.Request) {
	id, err := catalogID(r, "variantId", "variant")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if err := h.service.DeleteVariant(r.Context(), id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetCategory returns one category.
func (h *Handler) GetCategory(w http.ResponseWriter, r *http.Request) {
	id, err := catalogID(r, "id", "category")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	category, err := h.service.GetCategory(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, category)
}

// UpdateCategory edits a category.
func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := catalogID(r, "id", "category")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var input catalog.Category
	if err := httpx.DecodeJSON(w, r, &input); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	input.ID = id

	if err := h.service.UpdateCategory(r.Context(), &input); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, input)
}

// DeleteCategory removes an unused category.
func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := catalogID(r, "id", "category")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if err := h.service.DeleteCategory(r.Context(), id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetBrand returns one brand.
func (h *Handler) GetBrand(w http.ResponseWriter, r *http.Request) {
	id, err := catalogID(r, "id", "brand")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	brand, err := h.service.GetBrand(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, brand)
}

// UpdateBrand edits a brand.
func (h *Handler) UpdateBrand(w http.ResponseWriter, r *http.Request) {
	id, err := catalogID(r, "id", "brand")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var input catalog.Brand
	if err := httpx.DecodeJSON(w, r, &input); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	input.ID = id

	if err := h.service.UpdateBrand(r.Context(), &input); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, input)
}

// DeleteBrand removes an unused brand.
func (h *Handler) DeleteBrand(w http.ResponseWriter, r *http.Request) {
	id, err := catalogID(r, "id", "brand")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if err := h.service.DeleteBrand(r.Context(), id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
