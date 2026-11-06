package http

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

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
	// Wallet orders are refused for every actor and every status: the website
	// moves the money on confirmation and cancellation, and this route does not.
	if err := commerce.ValidateAPIWalletOrder(order); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	if isVendor && !isStaff {
		if err := h.transitionVendorOrderStatus(r.Context(), actor, order, req); err != nil {
			httpx.Error(w, r, h.log, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "success"})
		return
	}
	if isBuyer && !isStaff && !isVendor {
		if req.Status != commerce.StatusCancelled {
			httpx.Error(w, r, h.log, apperr.Forbidden("order.status_transition_denied", "Buyers may only cancel pending orders"))
			return
		}
	}

	actorUserID := actor.UserID
	if req.Status == commerce.StatusCancelled {
		if err := h.service.CancelAPIOrder(r.Context(), commerce.APICancelRequest{OrderID: id, UserID: actor.UserID, OrganizationID: actor.OrganizationID, Staff: isStaff, Reason: req.Notes}); err != nil {
			httpx.Error(w, r, h.log, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "success"})
		return
	}
	if err := h.service.TransitionOrderStatus(r.Context(), id, req.Status, &actorUserID, req.Notes); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{
		"status": "success",
	})
}

func (h *Handler) transitionVendorOrderStatus(ctx context.Context, actor authctx.Actor, order *commerce.Order, request TransitionStatusRequest) error {
	if !actor.Can("vendor.order.update") {
		return apperr.Forbidden("shipment.unauthorized", "Not authorized to update this shipment")
	}
	var own *commerce.OrderShipment
	for _, shipment := range order.Shipments {
		if shipment == nil || shipment.OrganizationID != actor.OrganizationID {
			continue
		}
		if own != nil {
			return apperr.Conflict("shipment.ambiguous", "يرجى تحديث كل شحنة من مسارها المخصص. Please use the shipment status endpoint for this order.")
		}
		own = shipment
	}
	if own == nil {
		return apperr.Forbidden("shipment.unauthorized", "Not authorized to update this shipment")
	}
	if request.Status == commerce.StatusCancelled {
		return h.service.CancelAPIVendorShipment(ctx, commerce.APIVendorCancelRequest{OrderID: order.ID, ShipmentID: own.ID, VendorOrgID: actor.OrganizationID, UserID: actor.UserID, Reason: request.Notes})
	}
	actorUserID := actor.UserID
	_, err := h.service.TransitionShipmentStatus(ctx, own.ID, request.Status, &actorUserID, request.Notes)
	return err
}
