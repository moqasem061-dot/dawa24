package commerce

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Fulfilment methods of mockCommerceRepo.

func (m *mockCommerceRepo) SetCartItemQuantity(_ context.Context, _, _ int64, _ int) error {
	return nil
}

func (m *mockCommerceRepo) GetShipmentByID(_ context.Context, id int64) (*OrderShipment, error) {
	for _, list := range m.shipments {
		for _, s := range list {
			if s.ID == id {
				// A copy, as a real repository returns. Handing back the stored
				// pointer would let the service mutate the fake database before
				// the compare-and-swap ran.
				copied := *s
				return &copied, nil
			}
		}
	}
	return nil, apperr.NotFound("shipment")
}

func (m *mockCommerceRepo) UpdateShipmentStatus(
	_ context.Context,
	id int64,
	from, to OrderStatus,
	history OrderStatusHistory,
) error {
	for _, list := range m.shipments {
		for _, s := range list {
			if s.ID != id {
				continue
			}
			if s.Status != from {
				return apperr.Conflict("shipment.state_changed",
					"This shipment was already updated by someone else.")
			}
			s.Status = to
			m.history[history.OrderID] = append(m.history[history.OrderID], &history)
			return nil
		}
	}
	return nil
}

func (m *mockCommerceRepo) ListOrderHistory(_ context.Context, orderID int64) ([]*OrderStatusHistory, error) {
	return m.history[orderID], nil
}

func (m *mockCommerceRepo) RateOrder(_ context.Context, orderID, customerID int64, rating float64, review string) error {
	o, ok := m.orders[orderID]
	if !ok || o.CustomerID != customerID {
		return apperr.NotFound("order")
	}
	if o.Status != StatusDelivered {
		return apperr.Validation("order.not_delivered", "Only delivered orders can be rated.", nil)
	}
	if o.RatedAt != nil {
		return apperr.Validation("order.already_rated", "This order has already been rated.", nil)
	}
	now := time.Now().UTC()
	o.Rating = &rating
	o.Review = &review
	o.RatedAt = &now
	return nil
}

func (m *mockCommerceRepo) CountOrders(_ context.Context) (int, error) { return len(m.orders), nil }

func (m *mockCommerceRepo) CountVendorShipmentsByStatus(_ context.Context, _ int64, _ []string) (int, error) {
	return 0, nil
}

func (m *mockCommerceRepo) AcceptNegotiation(_ context.Context, orderID, actorID int64) error {
	o, ok := m.orders[orderID]
	if !ok {
		return apperr.NotFound("order")
	}
	o.NegotiationStatus = "accepted"
	o.Status = StatusConfirmed
	return nil
}

func (m *mockCommerceRepo) RejectNegotiation(_ context.Context, orderID int64, reason string, actorID int64) error {
	o, ok := m.orders[orderID]
	if !ok {
		return apperr.NotFound("order")
	}
	o.NegotiationStatus = "rejected"
	o.Status = StatusCancelled
	o.NegotiationNotes = reason
	return nil
}

func (m *mockCommerceRepo) UpdateCustomerPendingOrder(_ context.Context, order *Order, lines []OrderLineEditItem, changedByUserID int64) (*Order, error) {
	o, ok := m.orders[order.ID]
	if !ok {
		return nil, apperr.NotFound("order")
	}
	return o, nil
}

func (m *mockCommerceRepo) VerifyAndCompleteDelivery(
	_ context.Context,
	shipmentID int64,
	deliveryCode string,
	notes string,
	collectedAmountMinor int64,
) (*OrderShipment, error) {
	for _, list := range m.shipments {
		for _, s := range list {
			if s.ID == shipmentID {
				if s.DeliveryCode != "" && s.DeliveryCode != deliveryCode {
					s.DeliveryAttempts++
					if s.DeliveryAttempts >= 5 {
						now := time.Now().Add(15 * time.Minute)
						s.DeliveryLockedUntil = &now
						return nil, apperr.Conflict("delivery.locked", "تم تجاوز الحد الأقصى للمحاولات الخاطئة.")
					}
					return nil, apperr.Validation("delivery.invalid_code", "كود تأكيد الاستلام غير صحيح.", nil)
				}
				s.Status = StatusDelivered
				now := time.Now()
				s.DeliveredAt = &now
				s.DeliveredByCourierAt = &now
				s.DeliveryNotes = notes
				s.CollectedAmountMinor = collectedAmountMinor
				copied := *s
				return &copied, nil
			}
		}
	}
	return nil, apperr.NotFound("shipment")
}

// Courier assignment — the dispatch board. These mocks predate it and none of
// their tests exercise it; the queue itself is covered in courier_test.go.

func (m *mockCommerceRepo) GetVendorShipment(_ context.Context, _, _ int64) (*OrderShipment, error) {
	return nil, apperr.NotFound("shipment")
}

func (m *mockCommerceRepo) GetVendorShipmentByOrderID(_ context.Context, _, _ int64) (*OrderShipment, error) {
	return nil, apperr.NotFound("shipment")
}

func (m *mockCommerceRepo) AssignShipmentCourier(_ context.Context, _, _ int64, _ *int64, _ int64) error {
	return nil
}

func (m *mockCommerceRepo) ListCourierQueue(_ context.Context, _ CourierQueueFilter) ([]*OrderShipment, int, error) {
	return nil, 0, nil
}

func (m *mockCommerceRepo) CourierQueueCounts(_ context.Context, _, _ int64) (CourierQueueCounts, error) {
	return CourierQueueCounts{}, nil
}

func (m *mockCommerceRepo) ListCourierWorkload(_ context.Context, _ int64) ([]*CourierWorkload, error) {
	return nil, nil
}
