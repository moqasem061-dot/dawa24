package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// GetMe returns the authenticated user's profile and active permissions.
func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	sess, ok := SessionFrom(r.Context())
	if !ok || sess == nil {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	var activeOrg *int64
	if sess.ActiveOrgID > 0 {
		activeOrg = &sess.ActiveOrgID
	}

	resp, err := h.service.GetMe(r.Context(), sess.UserID, activeOrg)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// UpdateMe updates user profile information.
func (h *Handler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	sess, ok := SessionFrom(r.Context())
	if !ok || sess == nil {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	var body struct {
		NameAr   string `json:"name_ar"`
		NameEn   string `json:"name_en"`
		Phone    string `json:"phone"`
		Timezone string `json:"timezone"`
		Language string `json:"language"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	updated, err := h.service.UpdateProfile(r.Context(), sess.UserID, body.NameAr, body.NameEn, body.Phone, body.Timezone, body.Language)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, updated)
}

// ListAddresses returns user shipping addresses.
func (h *Handler) ListAddresses(w http.ResponseWriter, r *http.Request) {
	sess, ok := SessionFrom(r.Context())
	if !ok || sess == nil {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	addrs, err := h.service.ListAddresses(r.Context(), sess.UserID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"addresses": addrs, "count": len(addrs)})
}

// CreateAddress saves a new address.
func (h *Handler) CreateAddress(w http.ResponseWriter, r *http.Request) {
	sess, ok := SessionFrom(r.Context())
	if !ok || sess == nil {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	var addr identity.UserAddress
	if err := httpx.DecodeJSON(w, r, &addr); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	addr.UserID = sess.UserID

	created, err := h.service.CreateAddress(r.Context(), &addr)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, created)
}

// UpdateAddress modifies an address.
func (h *Handler) UpdateAddress(w http.ResponseWriter, r *http.Request) {
	sess, ok := SessionFrom(r.Context())
	if !ok || sess == nil {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid address ID", nil))
		return
	}

	var addr identity.UserAddress
	if err := httpx.DecodeJSON(w, r, &addr); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	addr.ID = id
	addr.UserID = sess.UserID

	if err := h.service.UpdateAddress(r.Context(), &addr); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteAddress deletes an address.
func (h *Handler) DeleteAddress(w http.ResponseWriter, r *http.Request) {
	sess, ok := SessionFrom(r.Context())
	if !ok || sess == nil {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid address ID", nil))
		return
	}

	if err := h.service.DeleteAddress(r.Context(), id, sess.UserID); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListFavorites returns favorited product IDs.
func (h *Handler) ListFavorites(w http.ResponseWriter, r *http.Request) {
	sess, ok := SessionFrom(r.Context())
	if !ok || sess == nil {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	ids, err := h.service.ListFavorites(r.Context(), sess.UserID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"product_ids": ids, "count": len(ids)})
}

// AddFavorite adds a product to favorites.
func (h *Handler) AddFavorite(w http.ResponseWriter, r *http.Request) {
	sess, ok := SessionFrom(r.Context())
	if !ok || sess == nil {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	var body struct {
		ProductID int64 `json:"product_id"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if err := h.service.AddFavorite(r.Context(), sess.UserID, body.ProductID); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// RemoveFavorite removes a product from favorites.
func (h *Handler) RemoveFavorite(w http.ResponseWriter, r *http.Request) {
	sess, ok := SessionFrom(r.Context())
	if !ok || sess == nil {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	pid, err := strconv.ParseInt(chi.URLParam(r, "productId"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("product_id.invalid", "Invalid product ID", nil))
		return
	}

	if err := h.service.RemoveFavorite(r.Context(), sess.UserID, pid); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
