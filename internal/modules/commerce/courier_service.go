package commerce

import (
	"context"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// The delivery representative's use cases.
//
// Two callers reach these: a مندوب working their own round, and a dispatcher
// planning everyone's. The difference between them is a permission, checked at
// the route; what this file enforces is the rule a permission cannot express —
// a courier acts on the parcels they are carrying and no others. Every method
// that changes a parcel therefore takes both the supplier and the acting user,
// and proves the pair against the row before it writes.

// AssignShipmentToCourier hands one parcel to a delivery representative.
//
// Passing a nil courier returns the parcel to the unassigned pool. The caller
// is expected to have verified that the courier is a member of this supplier
// holding the delivery grant — that is an org question and this module cannot
// see org.members — but the parcel's own ownership is proved here.
func (s *Service) AssignShipmentToCourier(
	ctx context.Context,
	shipmentID, vendorOrgID int64,
	courierUserID *int64,
	assignedByUserID int64,
) (*OrderShipment, error) {
	if vendorOrgID <= 0 || shipmentID <= 0 {
		return nil, apperr.Validation("delivery.assign_invalid",
			"A shipment and a supplier are both required to assign a delivery.", nil)
	}

	shipment, err := s.repo.GetVendorShipment(ctx, shipmentID, vendorOrgID)
	if err != nil {
		return nil, err
	}
	// A parcel that is cancelled or returned cannot be reassigned.
	if shipment.Status == StatusCancelled || shipment.Status == StatusReturned || shipment.Status == StatusRefunded {
		return nil, apperr.Conflict("delivery.shipment_closed",
			"This shipment is cancelled or returned and cannot be reassigned.")
	}
	if courierUserID != nil && *courierUserID <= 0 {
		return nil, apperr.Validation("delivery.courier_required",
			"Choose a delivery representative.", map[string]string{"courier_user_id": "required"})
	}

	if err := s.repo.AssignShipmentCourier(ctx, shipmentID, vendorOrgID, courierUserID, assignedByUserID); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "shipment courier assignment changed",
		"shipment_id", shipmentID, "organization_id", vendorOrgID,
		"courier_user_id", courierUserID, "assigned_by", assignedByUserID)

	return s.repo.GetVendorShipment(ctx, shipmentID, vendorOrgID)
}

// ListCourierQueue reads one page of the dispatch board.
func (s *Service) ListCourierQueue(ctx context.Context, f CourierQueueFilter) ([]*OrderShipment, int, error) {
	if f.VendorOrgID <= 0 {
		return nil, 0, apperr.Validation("delivery.org_required",
			"A supplier is required to read the dispatch board.", nil)
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.repo.ListCourierQueue(ctx, f)
}

// CourierQueueCounts returns the tab badges for one caller's board.
func (s *Service) CourierQueueCounts(ctx context.Context, vendorOrgID, courierUserID int64) (CourierQueueCounts, error) {
	if vendorOrgID <= 0 {
		return CourierQueueCounts{}, apperr.Validation("delivery.org_required",
			"A supplier is required to read the dispatch board.", nil)
	}
	return s.repo.CourierQueueCounts(ctx, vendorOrgID, courierUserID)
}

// ListCourierWorkload summarises what is out with each representative.
func (s *Service) ListCourierWorkload(ctx context.Context, vendorOrgID int64) ([]*CourierWorkload, error) {
	if vendorOrgID <= 0 {
		return nil, apperr.Validation("delivery.org_required",
			"A supplier is required to read the dispatch board.", nil)
	}
	return s.repo.ListCourierWorkload(ctx, vendorOrgID)
}

// GetCourierShipment reads one parcel for a member of the supplier.
//
// mustOwn is what separates the two audiences. A مندوب passes their own user
// id and may only open a parcel on their own round; a dispatcher passes zero
// and may open any parcel the company has. The check is here rather than in
// the handler because it is the same rule for the page, the status change and
// the handover, and a rule written three times is a rule that will be written
// wrong once.
func (s *Service) GetCourierShipment(ctx context.Context, shipmentID, vendorOrgID, mustOwn int64) (*OrderShipment, error) {
	shipment, err := s.repo.GetVendorShipment(ctx, shipmentID, vendorOrgID)
	if err != nil {
		return nil, err
	}
	if mustOwn > 0 && !shipment.IsAssignedTo(mustOwn) {
		return nil, apperr.Forbidden("delivery.not_assigned",
			"This shipment is not assigned to you.")
	}
	return shipment, nil
}

// AdvanceCourierShipment moves a parcel on the caller's round through
// fulfilment — picked up, on the way, at the door.
//
// It reuses the order state machine rather than defining a courier-specific
// one: a parcel a مندوب marks "out for delivery" is in exactly the state the
// supplier's own screen would put it in, and the same history row records it.
func (s *Service) AdvanceCourierShipment(
	ctx context.Context,
	shipmentID, vendorOrgID, courierUserID int64,
	to OrderStatus,
	notes string,
) (*OrderShipment, error) {
	shipment, err := s.GetCourierShipment(ctx, shipmentID, vendorOrgID, courierUserID)
	if err != nil {
		return nil, err
	}
	if !IsValidStatusTransition(shipment.Status, to) {
		return nil, apperr.Conflict("shipment.invalid_transition",
			"A shipment cannot move from "+string(shipment.Status)+" to "+string(to)+".")
	}
	// Handover is the one transition a courier may not simply declare: it
	// needs the pharmacy's confirmation code, which CompleteCourierDelivery
	// verifies. Allowing it here would be a way around the PIN.
	if to == StatusDelivered || to == StatusCompleted {
		return nil, apperr.Conflict("delivery.code_required",
			"Confirm the handover with the pharmacy's delivery code.")
	}

	from := string(shipment.Status)
	history := OrderStatusHistory{
		OrderID:         shipment.OrderID,
		ShipmentID:      &shipment.ID,
		FromStatus:      &from,
		ToStatus:        string(to),
		Notes:           notes,
		ChangedByUserID: &courierUserID,
	}
	if err := s.repo.UpdateShipmentStatus(ctx, shipmentID, shipment.Status, to, history); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "courier advanced shipment",
		"shipment_id", shipmentID, "from", from, "to", to, "courier_user_id", courierUserID)
	return s.repo.GetVendorShipment(ctx, shipmentID, vendorOrgID)
}

// CompleteCourierDelivery closes a parcel against the pharmacy's PIN.
//
// It is VerifyAndCompleteDelivery with the ownership question answered first.
// The old portal could not ask that question — anyone holding a waybill number
// could close the parcel — which is precisely what this feature replaces.
func (s *Service) CompleteCourierDelivery(
	ctx context.Context,
	shipmentID, vendorOrgID, courierUserID int64,
	deliveryCode, notes string,
	collectedAmount ...money.Amount,
) (*OrderShipment, error) {
	code := strings.TrimSpace(deliveryCode)
	if code == "" {
		return nil, apperr.Validation("delivery.code_required",
			"Enter the pharmacy's six-digit delivery code.",
			map[string]string{"delivery_code": "required"})
	}

	shipment, err := s.GetCourierShipment(ctx, shipmentID, vendorOrgID, courierUserID)
	if err != nil {
		return nil, err
	}
	if shipment.IsClosed() {
		return nil, apperr.Conflict("delivery.shipment_closed",
			"This shipment has already been closed.")
	}

	var colMinor int64 = -1
	if len(collectedAmount) > 0 {
		colMinor = collectedAmount[0].Minor()
	}

	completed, err := s.repo.VerifyAndCompleteDelivery(ctx, shipment.ID, code, strings.TrimSpace(notes), colMinor)
	if err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "courier completed delivery",
		"shipment_id", shipment.ID, "shipment_number", shipment.ShipmentNumber,
		"order_id", shipment.OrderID, "courier_user_id", courierUserID,
		"collected_minor", colMinor)
	return completed, nil
}

// routePlanLimit caps how many parcels one plan considers.
//
// The repository refuses a page larger than this, and it is far beyond a real
// round: a courier carrying a hundred parcels has a dispatch problem, not a
// routing one. Reading a bounded page rather than "everything" is what keeps
// one courier's bad day from becoming a slow query for the whole supplier.
const routePlanLimit = 100

// PlanCourierRoute orders the caller's open round into a journey.
//
// origin is the courier's live position when the browser gave one and the
// warehouse otherwise; an unset origin is not an error, it produces an
// unordered plan the screen labels as such. The parcels are the same rows the
// "طرودي الجارية" tab reads, so a courier cannot be routed to something their
// board does not show them.
func (s *Service) PlanCourierRoute(
	ctx context.Context,
	vendorOrgID, courierUserID int64,
	origin GeoPoint,
) (*CourierRoute, error) {
	if vendorOrgID <= 0 || courierUserID <= 0 {
		return nil, apperr.Validation("delivery.route_actor_required",
			"A supplier and a delivery representative are both required to plan a route.", nil)
	}
	shipments, _, err := s.repo.ListCourierQueue(ctx, CourierQueueFilter{
		VendorOrgID:   vendorOrgID,
		CourierUserID: courierUserID,
		Queue:         CourierQueueMine,
		Limit:         routePlanLimit,
	})
	if err != nil {
		return nil, err
	}
	return BuildCourierRoute(origin, shipments), nil
}
