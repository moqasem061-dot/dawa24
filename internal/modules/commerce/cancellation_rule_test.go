package commerce

import "testing"

// The shared cancellation rule: an order may be cancelled only while its status
// is pending, processing, confirmed or on hold, and no shipment has shipped,
// is in transit, is out for delivery, is delivered or is completed.
func TestAPICancellationRuleOrderStatuses(t *testing.T) {
	allowed := []OrderStatus{StatusPending, StatusProcessing, StatusConfirmed, StatusOnHold}
	blocked := []OrderStatus{StatusShipped, StatusInTransit, StatusOutForDelivery, StatusDelivered,
		StatusCompleted, StatusCancelled, StatusFailed, StatusReturned, StatusRefunded}
	for _, status := range allowed {
		t.Run("allowed_"+string(status), func(t *testing.T) {
			order := &Order{Status: status, PaymentMethod: "cod", Shipments: []*OrderShipment{{Status: StatusPending}}}
			if err := ValidateAPICancellation(order); err != nil {
				t.Fatalf("order %s: want cancellable, got %v", status, err)
			}
		})
	}
	for _, status := range blocked {
		t.Run("blocked_"+string(status), func(t *testing.T) {
			order := &Order{Status: status, PaymentMethod: "cod", Shipments: []*OrderShipment{{Status: StatusPending}}}
			if err := ValidateAPICancellation(order); err == nil {
				t.Fatalf("order %s: want refused, got nil", status)
			}
		})
	}
}

func TestAPICancellationRuleShipmentStates(t *testing.T) {
	allowed := []OrderStatus{StatusPending, StatusProcessing, StatusConfirmed, StatusOnHold, StatusCancelled, StatusFailed, StatusReturned}
	blocked := []OrderStatus{StatusShipped, StatusInTransit, StatusOutForDelivery, StatusDelivered, StatusCompleted}
	for _, status := range allowed {
		t.Run("allowed_"+string(status), func(t *testing.T) {
			order := &Order{Status: StatusConfirmed, PaymentMethod: "cod", Shipments: []*OrderShipment{{Status: status}}}
			if err := ValidateAPICancellation(order); err != nil {
				t.Fatalf("shipment %s: want cancellable, got %v", status, err)
			}
		})
	}
	for _, status := range blocked {
		t.Run("blocked_"+string(status), func(t *testing.T) {
			order := &Order{Status: StatusConfirmed, PaymentMethod: "cod", Shipments: []*OrderShipment{{Status: status}}}
			if err := ValidateAPICancellation(order); err == nil {
				t.Fatalf("shipment %s: want refused, got nil", status)
			}
		})
	}
}

// Batch 1B: wallet-paid orders can be cancelled through the APIs, with their
// refund, under the same rule as every other order.
func TestWalletOrderCancellationUsesSharedRule(t *testing.T) {
	order := &Order{Status: StatusConfirmed, PaymentMethod: "wallet", Shipments: []*OrderShipment{{Status: StatusPending}}}
	if err := ValidateAPICancellation(order); err != nil {
		t.Fatalf("wallet order in confirmed status: want cancellable through the shared rule, got %v", err)
	}
	blocked := &Order{Status: StatusPending, PaymentMethod: "wallet", Shipments: []*OrderShipment{{Status: StatusShipped}}}
	if err := ValidateAPICancellation(blocked); err == nil {
		t.Fatal("wallet order with a shipped shipment: want refused")
	}
}

// Any status other than a cancellation stays refused on the APIs for wallet orders.
func TestWalletOrderOtherStatusesStayRefused(t *testing.T) {
	order := &Order{Status: StatusPending, PaymentMethod: "wallet"}
	if err := ValidateAPIWalletOrder(order); err == nil {
		t.Fatal("wallet order: non-cancel status change must stay refused on the APIs")
	}
	if !IsWalletPayment(" WALLET ") {
		t.Fatal("wallet detection must ignore case and spaces")
	}
}
