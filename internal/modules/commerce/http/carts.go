package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// GetCart returns customer cart and items.
func (h *Handler) GetCart(w http.ResponseWriter, r *http.Request) {
	// The acting user comes from the authenticated session, never from the
	// request. Reading it from the query string let any caller act as any
	// user by changing a number.
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	cart, err := h.service.GetCart(r.Context(), userID, buyerOrgOf(r))
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, cart)
}

// AddCartItem adds or updates item quantity in the cart.
func (h *Handler) AddCartItem(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProductID        int64        `json:"product_id"`
		ProductVariantID int64        `json:"product_variant_id"`
		Quantity         int          `json:"quantity"`
		UnitPrice        money.Amount `json:"unit_price,omitempty"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	item := &commerce.CartItem{
		ProductID:        body.ProductID,
		ProductVariantID: body.ProductVariantID,
		Quantity:         body.Quantity,
		UnitPrice:        body.UnitPrice,
	}

	cart, err := h.service.AddToCart(r.Context(), userID, buyerOrgOf(r), item)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, cart)
}

// RemoveCartItem removes a variant from the cart.
func (h *Handler) RemoveCartItem(w http.ResponseWriter, r *http.Request) {
	variantIDStr := chi.URLParam(r, "variantId")
	variantID, err := strconv.ParseInt(variantIDStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("variant_id.invalid", "Invalid variant ID", nil))
		return
	}

	// The acting user comes from the authenticated session, never from the
	// request. Reading it from the query string let any caller act as any
	// user by changing a number.
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	cart, err := h.service.RemoveFromCart(r.Context(), userID, variantID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, cart)
}

// ClearCart empties the shopping cart.
func (h *Handler) ClearCart(w http.ResponseWriter, r *http.Request) {
	// The acting user comes from the authenticated session, never from the
	// request. Reading it from the query string let any caller act as any
	// user by changing a number.
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if err := h.service.ClearCart(r.Context(), userID); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{"status": "cleared"})
}

// buyerOrgOf is the company the caller is buying for, or 0 when they belong to
// none. The cart rules that depend on it — hiding and refusing the caller's own
// stock — then simply do not apply, which is correct: a caller with no company
// supplies nothing.
func buyerOrgOf(r *http.Request) int64 {
	actor, ok := authctx.From(r.Context())
	if !ok {
		return 0
	}
	return actor.OrganizationID
}
