package commerce

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestCommerceOrderStateMachineTransitions(t *testing.T) {
	repo := newMockCommerceRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)
	ctx := context.Background()

	pID := int64(10)
	input := CheckoutInput{
		CustomerID:    100,
		PaymentMethod: "card",
		Items: []CheckoutLineItem{
			{
				VendorOrgID: 1,
				ProductID:   &pID,
				ProductName: i18n.New("بانادول", "Panadol"),
				UnitPrice:   money.MustParse("25.00"),
				Quantity:    2,
			},
		},
	}

	order, err := svc.Checkout(ctx, input)
	if err != nil {
		t.Fatalf("Checkout failed: %v", err)
	}

	// 1. Pending -> Confirmed (Allowed)
	userID := int64(999)
	err = svc.TransitionOrderStatus(ctx, order.ID, StatusConfirmed, &userID, "Confirmed")
	if err != nil {
		t.Fatalf("Transition to Confirmed failed: %v", err)
	}

	// 2. Confirmed -> Shipped (Allowed; the 063 DAG ships straight from confirmed)
	err = svc.TransitionOrderStatus(ctx, order.ID, StatusShipped, &userID, "Dispatched")
	if err != nil {
		t.Fatalf("Transition to Shipped failed: %v", err)
	}

	// 3. Shipped -> In Transit (Allowed)
	err = svc.TransitionOrderStatus(ctx, order.ID, StatusInTransit, &userID, "Picked up by courier")
	if err != nil {
		t.Fatalf("Transition to In Transit failed: %v", err)
	}

	// 4. In Transit -> Out For Delivery (Allowed)
	err = svc.TransitionOrderStatus(ctx, order.ID, StatusOutForDelivery, &userID, "With courier")
	if err != nil {
		t.Fatalf("Transition to Out For Delivery failed: %v", err)
	}

	// 5. Out For Delivery -> Delivered (Allowed)
	err = svc.TransitionOrderStatus(ctx, order.ID, StatusDelivered, &userID, "Delivered to buyer")
	if err != nil {
		t.Fatalf("Transition to Delivered failed: %v", err)
	}

	// 6. Delivered -> Processing (Disallowed conflict)
	err = svc.TransitionOrderStatus(ctx, order.ID, StatusProcessing, &userID, "Invalid rewind")
	if err == nil {
		t.Fatal("expected conflict on invalid status transition, got nil")
	}
}

func TestCommerceCartAndFulfilment(t *testing.T) {
	repo := newMockCommerceRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)
	ctx := context.Background()
	const buyerOrg int64 = 900

	// Cart operations
	cart, err := svc.GetCart(ctx, 100, buyerOrg)
	if err != nil {
		t.Fatalf("GetCart failed: %v", err)
	}
	if cart.UserID != 100 {
		t.Errorf("got user id %d, want 100", cart.UserID)
	}

	_, err = svc.AddToCart(ctx, 100, buyerOrg, &CartItem{
		ProductVariantID: 55,
		Quantity:         2,
		UnitPrice:        money.MustParse("15.00"),
	})
	if err != nil {
		t.Fatalf("AddToCart failed: %v", err)
	}

	_, err = svc.RemoveFromCart(ctx, 100, 55)
	if err != nil {
		t.Fatalf("RemoveFromCart failed: %v", err)
	}

	err = svc.ClearCart(ctx, 100)
	if err != nil {
		t.Fatalf("ClearCart failed: %v", err)
	}

	// Set quantity tests
	_, err = svc.SetCartQuantity(ctx, 100, 55, -1)
	if err == nil {
		t.Fatal("expected error on negative quantity, got nil")
	}

	_, err = svc.SetCartQuantity(ctx, 100, 55, 5)
	if err != nil {
		t.Fatalf("SetCartQuantity valid failed: %v", err)
	}

	_, err = svc.SetCartQuantity(ctx, 100, 55, 0)
	if err != nil {
		t.Fatalf("SetCartQuantity zero (remove) failed: %v", err)
	}

	// Wishlist operations
	if err := svc.AddToWishlist(ctx, 100, 10); err != nil {
		t.Fatalf("AddToWishlist failed: %v", err)
	}
	if err := svc.RemoveFromWishlist(ctx, 100, 10); err != nil {
		t.Fatalf("RemoveFromWishlist failed: %v", err)
	}

	// Lookups
	pID := int64(10)
	order, _ := svc.Checkout(ctx, CheckoutInput{
		CustomerID:    100,
		PaymentMethod: "cash",
		Items: []CheckoutLineItem{
			{
				VendorOrgID: 1,
				ProductID:   &pID,
				ProductName: i18n.New("دواء", "Medicine"),
				UnitPrice:   money.MustParse("50.00"),
				Quantity:    1,
			},
		},
	})

	gotOrder, err := svc.GetOrder(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetOrder failed: %v", err)
	}
	if gotOrder.ID != order.ID {
		t.Errorf("got order id %d, want %d", gotOrder.ID, order.ID)
	}

	orders, err := svc.ListCustomerOrders(ctx, 100, 10, 0)
	if err != nil || len(orders) == 0 {
		t.Fatalf("ListCustomerOrders failed: %v", err)
	}

	vendorShipments, err := svc.ListVendorShipments(ctx, 1, 10, 0)
	if err != nil || len(vendorShipments) == 0 {
		t.Fatalf("ListVendorShipments failed: %v", err)
	}

	// Rate order after delivery
	order.Status = StatusDelivered
	if err := svc.RateOrder(ctx, order.ID, 100, 5, "Excellent service"); err != nil {
		t.Fatalf("RateOrder failed: %v", err)
	}

	// Shipment status transition
	vendorCtx := database.WithTenant(ctx, 1)
	shipments := repo.shipments[order.ID]
	if len(shipments) > 0 {
		_, err = svc.GetShipment(vendorCtx, shipments[0].ID)
		if err != nil {
			t.Fatalf("GetShipment failed: %v", err)
		}

		userID := int64(1)
		_, err = svc.TransitionShipmentStatus(vendorCtx, shipments[0].ID, StatusConfirmed, &userID, "Confirmed shipment")
		if err != nil {
			t.Fatalf("TransitionShipmentStatus failed: %v", err)
		}
	}
}

func TestCommerceQuotesAndAdmin(t *testing.T) {
	repo := newMockCommerceRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)
	ctx := context.Background()

	// Quotes
	targetPrice, _ := money.Parse("1000.00")
	quote := &QuoteRequest{
		OrganizationID:    20,
		CustomerOrgID:     10,
		RequestedQuantity: 50,
		TargetUnitPrice:   targetPrice,
	}
	createdQuote, err := svc.CreateQuoteRequest(ctx, quote)
	if err != nil {
		t.Fatalf("CreateQuoteRequest failed: %v", err)
	}

	quotePrice, _ := money.Parse("950.00")
	if err := svc.RespondToQuote(ctx, createdQuote.ID, QuoteAccepted, quotePrice, "Special offer"); err != nil {
		t.Fatalf("RespondToQuote failed: %v", err)
	}

	quotes, err := svc.ListQuoteRequests(ctx, 10, false, 10, 0)
	if err != nil || len(quotes) != 1 {
		t.Fatalf("ListQuoteRequests failed: %v", err)
	}

	// Admin search
	results, err := svc.AdminSearchOrders(ctx, "", 10, 0)
	if err != nil {
		t.Fatalf("AdminSearchOrders failed: %v", err)
	}
	_ = results
}
