package commerce

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Cart quantity edits, vendor shipment fulfilment, order history and rating.

// SetCartQuantity changes the quantity of an item already in the cart.
//
// This exists so clients do not have to remove and re-add. That round trip
// re-reads the current price, so a vendor repricing between the two calls would
// silently change what the customer is paying for something already in their
// basket. Editing in place keeps the price captured at add time.
//
// A quantity of zero removes the line, which is what a stepper control sends
// when the customer decrements to nothing.
func (s *Service) SetCartQuantity(ctx context.Context, userID, variantID int64, quantity int) (*Cart, error) {
	if quantity < 0 {
		return nil, apperr.Validation("cart.quantity_negative",
			"Quantity cannot be negative.", map[string]string{"quantity": "must be zero or more"})
	}
	const maxLineQuantity = 100000
	if quantity > maxLineQuantity {
		return nil, apperr.Validation("cart.quantity_too_large",
			"That quantity is not orderable in a single line.", nil)
	}

	cart, err := s.repo.GetOrCreateCart(ctx, userID)
	if err != nil {
		return nil, err
	}

	if quantity == 0 {
		if err := s.repo.RemoveCartItem(ctx, cart.ID, variantID); err != nil {
			return nil, err
		}
		return s.repo.GetCartWithItems(ctx, cart.ID)
	}

	if err := s.repo.SetCartItemQuantity(ctx, cart.ID, variantID, quantity); err != nil {
		return nil, err
	}
	return s.repo.GetCartWithItems(ctx, cart.ID)
}

// GetShipment returns one vendor shipment.
func (s *Service) GetShipment(ctx context.Context, id int64) (*OrderShipment, error) {
	if _, ok := database.TenantFrom(ctx); !ok {
		return nil, database.ErrNoTenant
	}
	return s.repo.GetShipmentByID(ctx, id)
}

// TransitionShipmentStatus advances a vendor's shipment through fulfilment.
//
// Shipments carry status independently of their parent order because a
// multi-vendor order is fulfilled in parts: one supplier can have delivered
// while another has not yet shipped. The order's own status is a summary of
// its shipments, not a substitute for them.
func (s *Service) TransitionShipmentStatus(
	ctx context.Context,
	shipmentID int64,
	to OrderStatus,
	changedByUserID *int64,
	notes string,
) (*OrderShipment, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}

	shipment, err := s.repo.GetShipmentByID(ctx, shipmentID)
	if err != nil {
		return nil, err
	}

	// Row-level security already scopes the read to this tenant, so a mismatch
	// here means the id belongs to a shipment the caller cannot see. Checking
	// anyway keeps the rule visible in the domain rather than only in the
	// database.
	if shipment.OrganizationID != orgID {
		return nil, apperr.Forbidden("shipment.not_owned",
			"This shipment belongs to another organization.")
	}

	if !IsValidStatusTransition(shipment.Status, to) {
		return nil, apperr.Conflict("shipment.invalid_transition",
			"A shipment cannot move from "+string(shipment.Status)+" to "+string(to)+".")
	}

	from := string(shipment.Status)
	history := OrderStatusHistory{
		OrderID:         shipment.OrderID,
		ShipmentID:      &shipment.ID,
		FromStatus:      &from,
		ToStatus:        string(to),
		Notes:           notes,
		ChangedByUserID: changedByUserID,
	}

	if err := s.repo.UpdateShipmentStatus(ctx, shipmentID, shipment.Status, to, history); err != nil {
		return nil, err
	}

	shipment.Status = to
	s.log.InfoContext(ctx, "shipment status changed",
		"shipment_id", shipmentID, "from", from, "to", to)
	return shipment, nil
}

// GetOrderHistory returns the audit trail of status changes for an order.
func (s *Service) GetOrderHistory(ctx context.Context, orderID int64) ([]*OrderStatusHistory, error) {
	if _, err := s.repo.GetOrderByID(ctx, orderID); err != nil {
		return nil, err
	}
	return s.repo.ListOrderHistory(ctx, orderID)
}

// RateOrder records a customer's rating and review of a completed order (audit §3.3).
func (s *Service) RateOrder(ctx context.Context, orderID, customerID int64, rating float64, review string) error {
	if rating < 1.0 || rating > 5.0 {
		return apperr.Validation("order.rating_range",
			"Rating must be between 1 and 5.", map[string]string{"rating": "1-5"})
	}

	order, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return err
	}

	// Rating something that was never received is meaningless, and lets a
	// competitor damage a supplier's score by placing and cancelling orders.
	if order.Status != StatusDelivered {
		return apperr.Conflict("order.not_delivered",
			"An order can only be rated once it has been delivered.")
	}

	if order.RatedAt != nil {
		return apperr.Conflict("order.already_rated",
			"This order has already been rated.")
	}

	return s.repo.RateOrder(ctx, orderID, customerID, rating, review)
}

// RateOrderWithCriteria computes the exact 3-criteria average and rates the order (audit §3.3).
func (s *Service) RateOrderWithCriteria(ctx context.Context, orderID, customerID int64, repRating, speedRating, qualityRating int, review string) (float64, error) {
	avg := CalculateAverageRating(repRating, speedRating, qualityRating)
	if err := s.RateOrder(ctx, orderID, customerID, avg, review); err != nil {
		return 0, err
	}
	return avg, nil
}
