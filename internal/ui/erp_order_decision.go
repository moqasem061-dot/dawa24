package ui

import (
	"context"
	"errors"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// ErpDecideShipment confirms or rejects a supplier shipment for the ERP API (S4).
// It applies the rules a supplier follows on the website: only a pending shipment
// can move, a confirmation charges a wallet order to the buyer, and a rejection
// moves a pending shipment to cancelled. The status history records "ERP: <reason>"
// and the buyer's notice carries the reason alone. The audit row in d.Audit is
// written inside the transition's own transaction.
func (h *UIHandler) ErpDecideShipment(ctx context.Context, d erp.ShipmentDecision) (erp.ShipmentDecisionResult, error) {
	// The ERP request carries no tenant. The supplier's own tenant is what the
	// website's supplier path runs under, so the transition's ownership check and
	// row-level security see the same organization.
	ctx = database.WithTenant(ctx, d.OrganizationID)
	shipment, err := h.commSvc.GetShipment(database.AsSystem(ctx), d.ShipmentID)
	if err != nil || shipment == nil || shipment.OrganizationID != d.OrganizationID {
		return erp.ShipmentDecisionResult{}, erp.NewError("ERP_ORDER_NOT_FOUND", 404, nil)
	}
	to := commerce.StatusConfirmed
	result := "confirmed"
	if d.Status == "rejected" {
		to, result = commerce.StatusCancelled, "rejected"
	}

	if repeat := decisionRepeat(shipment.Status, to); repeat {
		return erp.ShipmentDecisionResult{Result: "already_" + result, OrderStatus: string(shipment.Status)}, nil
	}
	if shipment.Status != commerce.StatusPending {
		return erp.ShipmentDecisionResult{}, erp.NewError("ERP_ORDER_STATE_CONFLICT", 409, map[string]any{"currentStatus": string(shipment.Status)})
	}
	order, err := h.commSvc.GetOrder(database.AsSystem(ctx), shipment.OrderID)
	if err != nil || order == nil {
		return erp.ShipmentDecisionResult{}, erp.NewError("ERP_ORDER_NOT_FOUND", 404, nil)
	}
	if order.IsNegotiation && order.NegotiationStatus == "pending" {
		return erp.ShipmentDecisionResult{}, erp.NewError("ERP_ORDER_NEGOTIATION_PENDING", 409, nil)
	}

	vendorName := h.resolveOrgName(ctx, d.OrganizationID)
	hook := erp.AuditTxHook(d.OrganizationID, d.Audit)
	err = h.transitionVendorShipmentBy(ctx, d.OrganizationID, false, nil, d.ShipmentID, to,
		"ERP: "+d.Reason, d.Reason, "", "", vendorName, hook)
	if err != nil {
		return erp.ShipmentDecisionResult{}, h.decisionError(ctx, d, err)
	}
	return erp.ShipmentDecisionResult{Result: result, OrderStatus: string(to)}, nil
}

// decisionRepeat reports a decision that the shipment already has: confirming a
// confirmed shipment, or cancelling a cancelled one. The caller answers with
// the already_* result and writes nothing.
func decisionRepeat(current, to commerce.OrderStatus) bool {
	return current == to && (to == commerce.StatusConfirmed || to == commerce.StatusCancelled)
}

// decisionError maps the website's refusal to the ERP contract. A wallet
// shortfall carries both amounts; a state change that lost a race reports the
// status the shipment has now; anything else is unavailable.
func (h *UIHandler) decisionError(ctx context.Context, d erp.ShipmentDecision, err error) error {
	var short walletShortfallError
	if errors.As(err, &short) {
		return erp.NewError("ERP_WALLET_INSUFFICIENT", 409, map[string]any{
			"requiredAmount":  short.Required.String(),
			"availableAmount": short.Available.String(),
		})
	}
	if apperr.KindOf(err) == apperr.KindConflict {
		if now, nErr := h.commSvc.GetShipment(database.AsSystem(ctx), d.ShipmentID); nErr == nil && now != nil {
			return erp.NewError("ERP_ORDER_STATE_CONFLICT", 409, map[string]any{"currentStatus": string(now.Status)})
		}
	}
	h.log.ErrorContext(ctx, "erp order decision refused", "error", err, "shipment_id", d.ShipmentID)
	return erp.NewError("ERP_UNAVAILABLE", 503, nil)
}
