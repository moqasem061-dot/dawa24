package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

func commerceID(r *http.Request, name, entity string) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.Validation("id.invalid", "Invalid "+entity+" identifier.",
			map[string]string{name: "must be a positive integer"})
	}
	return id, nil
}

// SetCartQuantity changes the quantity of a cart line in place.
func (h *Handler) SetCartQuantity(w http.ResponseWriter, r *http.Request) {
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	variantID, err := commerceID(r, "variantId", "variant")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var body struct {
		Quantity int `json:"quantity"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	cart, err := h.service.SetCartQuantity(r.Context(), userID, variantID, body.Quantity)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, cart)
}

// GetShipment returns one vendor shipment.
func (h *Handler) GetShipment(w http.ResponseWriter, r *http.Request) {
	actor, ok := authctx.From(r.Context())
	if !ok {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	id, err := commerceID(r, "id", "shipment")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	shipment, err := h.service.GetShipment(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	allowed := actor.IsStaff || actor.Can("commerce.admin") ||
		(shipment.OrganizationID > 0 && shipment.OrganizationID == actor.OrganizationID)

	if !allowed && shipment.OrderID > 0 {
		if order, err := h.service.GetOrder(r.Context(), shipment.OrderID); err == nil && order != nil {
			if order.CustomerID == actor.UserID || (order.OrganizationID != nil && *order.OrganizationID == actor.OrganizationID) {
				allowed = true
			}
		}
	}

	if !allowed {
		h.log.WarnContext(r.Context(), "unauthorized shipment read attempt",
			"actor_user_id", actor.UserID,
			"actor_org_id", actor.OrganizationID,
			"shipment_id", id,
			"shipment_org_id", shipment.OrganizationID,
		)
		httpx.Error(w, r, h.log, apperr.Forbidden("shipment.unauthorized", "Not authorized to view this shipment"))
		return
	}

	httpx.JSON(w, http.StatusOK, shipment)
}

// TransitionShipmentStatus advances a vendor's shipment through fulfilment.
func (h *Handler) TransitionShipmentStatus(w http.ResponseWriter, r *http.Request) {
	actor, ok := authctx.From(r.Context())
	if !ok {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	id, err := commerceID(r, "id", "shipment")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	shipment, err := h.service.GetShipment(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	// Authorization: platform staff / commerce admin may transition any shipment.
	// A supplier organization member who owns the shipment must hold the tenant-scoped
	// permission "vendor.order.update" (declared in catalog_vendor.go).
	// NOTE: Do not use "commerce.order.*" here; "commerce.*" permissions are admin-scoped
	// (catalog_admin.go) and holding them is impossible for non-staff vendor members.
	allowed := actor.IsStaff || actor.Can("commerce.admin") ||
		(shipment.OrganizationID > 0 && shipment.OrganizationID == actor.OrganizationID && actor.Can("vendor.order.update"))

	if !allowed {
		h.log.WarnContext(r.Context(), "unauthorized shipment status transition attempt",
			"actor_user_id", actor.UserID,
			"actor_org_id", actor.OrganizationID,
			"shipment_id", id,
			"shipment_org_id", shipment.OrganizationID,
		)
		httpx.Error(w, r, h.log, apperr.Forbidden("shipment.unauthorized", "Not authorized to transition this shipment"))
		return
	}

	var body struct {
		Status string `json:"status"`
		Notes  string `json:"notes"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	reqCtx := r.Context()
	if actor.IsStaff || actor.Can("commerce.admin") {
		reqCtx = database.WithTenant(reqCtx, shipment.OrganizationID)
	}

	actorID := actor.UserID
	updatedShipment, err := h.service.TransitionShipmentStatus(
		reqCtx, id, commerce.OrderStatus(body.Status), &actorID, body.Notes)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, updatedShipment)
}

// GetOrderHistory returns the status audit trail for an order.
func (h *Handler) GetOrderHistory(w http.ResponseWriter, r *http.Request) {
	actor, ok := authctx.From(r.Context())
	if !ok {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	id, err := commerceID(r, "id", "order")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	order, err := h.service.GetOrder(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if !actor.IsStaff && !actor.Can("commerce.admin") {
		allowed := order.CustomerID == actor.UserID || (order.OrganizationID != nil && *order.OrganizationID == actor.OrganizationID)
		if !allowed && actor.IsVendor() && actor.OrganizationID > 0 {
			for _, s := range order.Shipments {
				if s.OrganizationID == actor.OrganizationID {
					allowed = true
					break
				}
			}
		}
		if !allowed {
			h.log.WarnContext(r.Context(), "unauthorized order history read attempt",
				"actor_user_id", actor.UserID,
				"actor_org_id", actor.OrganizationID,
				"order_id", id,
			)
			httpx.Error(w, r, h.log, apperr.Forbidden("order.unauthorized", "Not authorized to view this order history"))
			return
		}
	}

	history, err := h.service.GetOrderHistory(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"history": history})
}

// RateOrder records a customer rating for a delivered order.
func (h *Handler) RateOrder(w http.ResponseWriter, r *http.Request) {
	actor, ok := authctx.From(r.Context())
	if !ok {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	id, err := commerceID(r, "id", "order")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	order, err := h.service.GetOrder(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if !actor.IsStaff && !actor.Can("commerce.admin") {
		if order.CustomerID != actor.UserID && (order.OrganizationID == nil || *order.OrganizationID != actor.OrganizationID) {
			h.log.WarnContext(r.Context(), "unauthorized order rate attempt",
				"actor_user_id", actor.UserID,
				"order_id", id,
				"order_customer_id", order.CustomerID,
			)
			httpx.Error(w, r, h.log, apperr.Forbidden("order.unauthorized", "Not authorized to rate this order"))
			return
		}
	}

	userID := actor.UserID

	var body struct {
		Rating        *float64 `json:"rating,omitempty"`
		RepRating     int      `json:"rep_rating,omitempty"`
		SpeedRating   int      `json:"speed_rating,omitempty"`
		QualityRating int      `json:"quality_rating,omitempty"`
		Review        string   `json:"review"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var errRate error
	if body.RepRating > 0 && body.SpeedRating > 0 && body.QualityRating > 0 {
		_, errRate = h.service.RateOrderWithCriteria(r.Context(), id, userID, body.RepRating, body.SpeedRating, body.QualityRating, body.Review)
	} else if body.Rating != nil {
		errRate = h.service.RateOrder(r.Context(), id, userID, *body.Rating, body.Review)
	} else {
		httpx.Error(w, r, h.log, apperr.Validation("rating.required", "Either rating or 3 review criteria ratings must be provided", nil))
		return
	}

	if errRate != nil {
		httpx.Error(w, r, h.log, errRate)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
