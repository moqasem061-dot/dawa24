package http

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Handler handles commerce HTTP endpoints.
type Handler struct {
	service *commerce.Service
	log     *slog.Logger
}

// NewHandler creates a commerce HTTP handler.
func NewHandler(service *commerce.Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes registers commerce endpoints on a Chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/api/v1/commerce/checkout", h.Checkout)

	r.Patch("/api/v1/commerce/cart/items/{variantId}", h.SetCartQuantity)
	r.Get("/api/v1/commerce/orders/{id}/history", h.GetOrderHistory)
	r.Post("/api/v1/commerce/orders/{id}/rate", h.RateOrder)
	r.Get("/api/v1/commerce/shipments/{id}", h.GetShipment)
	r.Post("/api/v1/commerce/shipments/{id}/status", h.TransitionShipmentStatus)
	r.Get("/api/v1/commerce/orders/{id}", h.GetOrder)
	r.Get("/api/v1/commerce/orders", h.ListCustomerOrders)
	r.Post("/api/v1/commerce/orders/{id}/status", h.TransitionStatus)
	r.Post("/api/v1/commerce/orders/{id}/cancel", h.CancelOrder)
	r.Get("/api/v1/commerce/vendor/shipments", h.ListVendorShipments)

	r.Get("/api/v1/commerce/cart", h.GetCart)
	r.Post("/api/v1/commerce/cart/items", h.AddCartItem)
	r.Delete("/api/v1/commerce/cart/items/{variantId}", h.RemoveCartItem)
	r.Delete("/api/v1/commerce/cart", h.ClearCart)

	r.Get("/api/v1/commerce/wishlist", h.GetWishlist)
	r.Post("/api/v1/commerce/wishlist", h.AddToWishlist)
	r.Delete("/api/v1/commerce/wishlist/{productId}", h.RemoveFromWishlist)

	r.Post("/api/v1/commerce/quotes", h.CreateQuoteRequest)
	// Responding to a price request is a commercial commitment by the supplier.
	// It took nothing but approved-organisation membership.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission(
			"vendor.purchase_request.respond", "commerce.quote.manage", "commerce.admin"))
		g.Post("/api/v1/commerce/quotes/{id}/respond", h.RespondQuote)
	})
	r.Get("/api/v1/commerce/quotes", h.ListQuotes)

	h.RegisterAdminRoutes(r)
}

// Checkout handles order placement.
func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	var input commerce.CheckoutInput
	if err := httpx.DecodeJSON(w, r, &input); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	actor, ok := authctx.From(r.Context())
	if !ok {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	// Always bind customer identity strictly to the authenticated actor
	input.CustomerID = actor.UserID
	if actor.OrganizationID > 0 {
		input.CustomerOrgID = actor.OrganizationID
		input.CustomerOrgType = actor.OrgType
	}

	order, err := h.service.Checkout(r.Context(), input)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, order)
}

// GetOrder returns order details.
func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid order ID", nil))
		return
	}

	actor, ok := authctx.From(r.Context())
	if !ok {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	order, err := h.service.GetOrder(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if !actor.IsStaff && !actor.Can("commerce.admin") {
		if order.CustomerID != actor.UserID && (order.OrganizationID == nil || *order.OrganizationID != actor.OrganizationID) {
			httpx.Error(w, r, h.log, apperr.Forbidden("order.unauthorized", "Not authorized to view this order"))
			return
		}
	}

	httpx.JSON(w, http.StatusOK, order)
}

// ListCustomerOrders returns orders for a customer.
func (h *Handler) ListCustomerOrders(w http.ResponseWriter, r *http.Request) {
	// The customer is whoever is authenticated. Taking it from ?customer_id=
	// let any caller read any buyer's order history by changing a number.
	customerID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	orders, err := h.service.ListCustomerOrders(r.Context(), customerID, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"orders": orders,
		"count":  len(orders),
	})
}

// TransitionStatusRequest represents a status change payload.
type TransitionStatusRequest struct {
	Status          commerce.OrderStatus `json:"status"`
	ChangedByUserID *int64               `json:"changed_by_user_id,omitempty"`
	Notes           string               `json:"notes,omitempty"`
}

// TransitionStatus transitions an order status.
func (h *Handler) TransitionStatus(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid order ID", nil))
		return
	}

	actor, ok := authctx.From(r.Context())
	if !ok {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	order, err := h.service.GetOrder(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var req TransitionStatusRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	isStaff := actor.IsStaff || actor.Can("commerce.admin")
	isBuyer := order.CustomerID == actor.UserID || (order.OrganizationID != nil && *order.OrganizationID == actor.OrganizationID)
	isVendor := isFulfillingVendor(order, actor.OrganizationID)

	if !isStaff && !isVendor && !isBuyer {
		httpx.Error(w, r, h.log, apperr.Forbidden("order.unauthorized", "Not authorized to update this order"))
		return
	}
	if isBuyer && !isStaff && !isVendor {
		if req.Status != commerce.StatusCancelled {
			httpx.Error(w, r, h.log, apperr.Forbidden("order.status_transition_denied", "Buyers may only cancel pending orders"))
			return
		}
		if order.Status != commerce.StatusPending && order.Status != commerce.StatusProcessing {
			httpx.Error(w, r, h.log, apperr.Forbidden("order.cannot_cancel", "Only pending orders can be cancelled by the buyer"))
			return
		}
	}

	actorUserID := actor.UserID
	if err := h.service.TransitionOrderStatus(r.Context(), id, req.Status, &actorUserID, req.Notes); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{
		"status": "success",
	})
}

// ListVendorShipments returns shipments partitioned for a vendor.
func (h *Handler) ListVendorShipments(w http.ResponseWriter, r *http.Request) {
	actor, ok := authctx.From(r.Context())
	if !ok {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	vendorIDStr := r.URL.Query().Get("vendor_id")
	var targetVendorID int64
	if vendorIDStr != "" {
		parsed, err := strconv.ParseInt(vendorIDStr, 10, 64)
		if err != nil || parsed <= 0 {
			httpx.Error(w, r, h.log, apperr.Validation("vendor_id.invalid", "Valid vendor_id query parameter required", nil))
			return
		}
		targetVendorID = parsed
	}

	var vendorOrgID int64
	if actor.IsStaff || actor.Can("commerce.admin") {
		if targetVendorID <= 0 {
			httpx.Error(w, r, h.log, apperr.Validation("vendor_id.required", "vendor_id query parameter is required for staff", nil))
			return
		}
		vendorOrgID = targetVendorID
	} else if actor.IsVendor() && actor.OrganizationID > 0 {
		if targetVendorID > 0 && targetVendorID != actor.OrganizationID {
			h.log.WarnContext(r.Context(), "vendor attempted to access another vendor's shipments",
				"actor_user_id", actor.UserID,
				"actor_org_id", actor.OrganizationID,
				"requested_vendor_id", targetVendorID,
			)
			httpx.Error(w, r, h.log, apperr.Forbidden("shipments.unauthorized", "Not authorized to view shipments for another vendor"))
			return
		}
		vendorOrgID = actor.OrganizationID
	} else {
		h.log.WarnContext(r.Context(), "non-vendor attempted to list vendor shipments",
			"actor_user_id", actor.UserID,
			"actor_org_id", actor.OrganizationID,
			"requested_vendor_id", targetVendorID,
		)
		httpx.Error(w, r, h.log, apperr.Forbidden("shipments.unauthorized", "Not authorized to view vendor shipments"))
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	shipments, err := h.service.ListVendorShipments(r.Context(), vendorOrgID, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"shipments": shipments,
		"count":     len(shipments),
	})
}

// AddToWishlist adds a product to customer wishlist.
func (h *Handler) AddToWishlist(w http.ResponseWriter, r *http.Request) {
	// The acting user comes from the authenticated session, never from the
	// request. Reading it from the request body let any caller act as any
	// user by changing a number.
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var input struct {
		ProductID int64 `json:"product_id"`
	}
	if err := httpx.DecodeJSON(w, r, &input); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	if input.ProductID <= 0 {
		httpx.Error(w, r, h.log, apperr.Validation("product_id.invalid", "Valid product_id required", nil))
		return
	}
	if err := h.service.AddToWishlist(r.Context(), userID, input.ProductID); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]string{"status": "added"})
}

// RemoveFromWishlist removes a product from customer wishlist.
func (h *Handler) RemoveFromWishlist(w http.ResponseWriter, r *http.Request) {
	productIDStr := chi.URLParam(r, "productId")
	productID, err := strconv.ParseInt(productIDStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("product_id.invalid", "Invalid product ID", nil))
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
	if err := h.service.RemoveFromWishlist(r.Context(), userID, productID); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// GetWishlist returns all wishlist items for a user.
func (h *Handler) GetWishlist(w http.ResponseWriter, r *http.Request) {
	// The acting user comes from the authenticated session, never from the
	// request. Reading it from the query string let any caller act as any
	// user by changing a number.
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	items, err := h.service.GetWishlist(r.Context(), userID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"wishlist": items, "count": len(items)})
}

// CancelOrder handles customer/admin order cancellation.
func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid order ID", nil))
		return
	}

	actor, ok := authctx.From(r.Context())
	if !ok {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	order, err := h.service.GetOrder(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if !actor.IsStaff && !actor.Can("commerce.admin") {
		if order.CustomerID != actor.UserID && (order.OrganizationID == nil || *order.OrganizationID != actor.OrganizationID) {
			httpx.Error(w, r, h.log, apperr.Forbidden("order.unauthorized", "Not authorized to cancel this order"))
			return
		}
	}

	var body struct {
		Reason string `json:"reason,omitempty"`
	}
	_ = httpx.DecodeJSON(w, r, &body)

	actorUserID := actor.UserID
	if err := h.service.CancelOrder(r.Context(), id, &actorUserID, body.Reason); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}
