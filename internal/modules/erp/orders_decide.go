package erp

import (
	"context"
	"errors"
)

// DecideResult is the answer to one confirm or reject. Result is "confirmed",
// "rejected", "already_confirmed" or "already_rejected"; OrderStatus is the
// shipment's status afterwards.
type DecideResult struct {
	OrderNumber, Result, OrderStatus string
}

// Decide confirms or rejects one of the supplier's shipments through the
// website's rules (S4). Only the two decision statuses are accepted; a delivery
// step is refused because it needs the buyer's PIN. The shipment is resolved by
// the key's organization, never by a caller-supplied id. The decider runs the
// website transition and writes the audit row in the same transaction.
func (s *Service) Decide(ctx context.Context, p Principal, orderNumber, status, reason string) (DecideResult, error) {
	if err := requireScope(p, ScopeOrdersDecide); err != nil {
		return DecideResult{}, err
	}
	if err := requireVendor(p); err != nil {
		return DecideResult{}, err
	}
	if err := checkDecideStatus(status); err != nil {
		return DecideResult{}, err
	}
	if s.ports.Decider == nil {
		return DecideResult{}, NewError("ERP_UNAVAILABLE", 503, nil)
	}
	row, err := s.repo.GetSalesShipmentAsSystem(ctx, p.OrganizationID, orderNumber)
	if errors.Is(err, ErrNotFound) {
		return DecideResult{}, NewError("ERP_ORDER_NOT_FOUND", 404, nil)
	}
	if err != nil {
		return DecideResult{}, NewError("ERP_INTERNAL", 500, nil)
	}

	keyID := p.KeyID
	action := "erp.order.confirm"
	if status == "rejected" {
		action = "erp.order.reject"
	}
	audit := AuditRecord{Action: action, EntityType: "order_shipment", EntityID: row.ShipmentNumber, ActorKeyID: &keyID, ActorKind: "api_key"}
	res, err := s.ports.Decider.Decide(ctx, ShipmentDecision{
		ShipmentID: row.ShipmentID, OrganizationID: p.OrganizationID, Status: status, Reason: reason, Audit: audit,
	})
	if err != nil {
		return DecideResult{}, s.decideError(err)
	}
	return DecideResult{OrderNumber: orderNumber, Result: res.Result, OrderStatus: res.OrderStatus}, nil
}

// checkDecideStatus names the refusals in the contract: delivery steps are 403,
// any other value is 422 with the allowed list.
func checkDecideStatus(status string) error {
	switch status {
	case "confirmed", "rejected":
		return nil
	case "delivered", "shipped", "completed", "out_for_delivery", "in_transit":
		return NewError("ERP_DELIVERY_NOT_ALLOWED", 403, nil)
	default:
		return NewError("ERP_STATUS_NOT_ALLOWED", 422, map[string]any{"allowed": []string{"confirmed", "rejected"}})
	}
}

// decideError keeps an erp error from the decider as it is. Anything else is a
// dependency failure: it is logged with its cause and reported as unavailable.
func (s *Service) decideError(err error) error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	if s.log != nil {
		s.log.Error("erp order decision failed", "error", err)
	}
	return NewError("ERP_UNAVAILABLE", 503, nil)
}
