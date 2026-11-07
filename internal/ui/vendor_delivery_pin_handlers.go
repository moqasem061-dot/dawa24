package ui

import (
	"context"
	"net/http"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// vendorDeliverWithPIN closes a supplier's parcel against the buyer's PIN, from
// the supplier's order screens. A parcel under the delivery PIN rule cannot be
// closed any other way except by staff override.
func (h *UIHandler) vendorDeliverWithPIN(
	w http.ResponseWriter, r *http.Request, ctx context.Context,
	actor authctx.Actor, shipmentID int64, code, notes, returnTo string,
) {
	completed, err := h.commSvc.CompleteShipmentDelivery(ctx, shipmentID, actor.OrganizationID, code, notes)
	if err != nil {
		h.log.WarnContext(ctx, "vendor delivery with buyer pin failed",
			"error", err, "shipment_id", shipmentID, "user_id", actor.UserID)
		h.redirectWithNotice(w, r, returnTo, "error",
			i18n.T(langOf(r), "vendor.orders.update_shipment_status_error_prefix")+h.safeMessage(err, langOf(r)))
		return
	}
	h.notifyDeliveryCompleted(ctx, completed, actor.OrganizationID)
	h.redirectWithNotice(w, r, returnTo, "success", i18n.T(langOf(r), "vendor.orders.shipment_status_updated_success"))
}
