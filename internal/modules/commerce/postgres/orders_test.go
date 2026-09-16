package postgres_test

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/commerce/postgres"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestCommerceOrdersAndFulfilment(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	resetCommerceFixtures(t, db)

	ctx := context.Background()
	repo := postgres.NewRepository(db)

	var createdOrderID int64
	var orderNumber string
	var shipmentID int64
	custOrgID := testCommerceCustID
	userID := testCommerceUserID

	t.Run("CreateOrder_MultiVendorAndLineTotals", func(t *testing.T) {
		orderNumber = "ORD-2026-TEST-001"
		subtotal := money.MustParse("200.00")
		shipping := money.MustParse("20.00")
		tax := money.MustParse("0.00")
		discount := money.MustParse("10.00")
		total := money.MustParse("210.00") // 200 - 10 + 20

		order := &commerce.Order{
			OrderNumber:    orderNumber,
			CustomerID:     testCommerceUserID,
			OrganizationID: &custOrgID,
			Status:         commerce.StatusPending,
			Subtotal:       subtotal,
			DiscountAmount: discount,
			ShippingFee:    shipping,
			TaxAmount:      tax,
			TotalAmount:    total,
			PaymentMethod:  "cash_on_delivery",
			PaymentStatus:  commerce.PaymentUnpaid,
			Notes:          "Deliver to front desk",
		}

		shipments := []*commerce.OrderShipment{
			{
				OrganizationID: testCommerceVendorID,
				Status:         commerce.StatusPending,
				Subtotal:       subtotal,
				ShippingFee:    shipping,
				TotalAmount:    total,
			},
		}

		pName := i18n.Text{"ar": "منتج", "en": "Comm Prod"}
		vName := i18n.Text{"ar": "افتراضي", "en": "Default"}
		prodID := testCommerceProdID
		varID := testCommerceVarID
		lineTotal, _ := subtotal.Sub(discount)

		lines := []*commerce.OrderLine{
			{
				OrganizationID:   testCommerceVendorID,
				ProductID:        &prodID,
				ProductVariantID: &varID,
				ProductName:      pName,
				VariantName:      vName,
				SKU:              "COMM-SKU-1",
				UnitPrice:        money.MustParse("100.00"),
				Quantity:         2,
				DiscountAmount:   discount,
				TotalPrice:       lineTotal, // 190.00
			},
		}

		err := repo.CreateOrder(ctx, order, shipments, lines)
		if err != nil {
			t.Fatalf("CreateOrder failed: %v", err)
		}
		if order.ID == 0 {
			t.Fatal("expected assigned order ID")
		}
		createdOrderID = order.ID
		if len(shipments) > 0 && shipments[0].ID != 0 {
			shipmentID = shipments[0].ID
		}

		// Verify order fetch by ID
		fetched, err := repo.GetOrderByID(ctx, createdOrderID)
		if err != nil {
			t.Fatalf("GetOrderByID failed: %v", err)
		}
		if fetched.OrderNumber != orderNumber {
			t.Errorf("got order number %q, want %q", fetched.OrderNumber, orderNumber)
		}
		if fetched.TotalAmount.Minor() != total.Minor() {
			t.Errorf("got total %v, want %v", fetched.TotalAmount, total)
		}

		// Verify order fetch by Number
		fetchedByNum, err := repo.GetOrderByNumber(ctx, orderNumber)
		if err != nil {
			t.Fatalf("GetOrderByNumber failed: %v", err)
		}
		if fetchedByNum.ID != createdOrderID {
			t.Errorf("got ID %d, want %d", fetchedByNum.ID, createdOrderID)
		}

		// List orders by customer
		custOrders, err := repo.ListOrdersByCustomer(ctx, testCommerceUserID, 10, 0)
		if err != nil {
			t.Fatalf("ListOrdersByCustomer failed: %v", err)
		}
		if len(custOrders) == 0 {
			t.Fatal("expected at least 1 order for customer")
		}

		// List shipments by vendor
		vendorShipments, err := repo.ListShipmentsByVendor(ctx, testCommerceVendorID, 10, 0)
		if err != nil {
			t.Fatalf("ListShipmentsByVendor failed: %v", err)
		}
		if len(vendorShipments) == 0 {
			t.Fatal("expected at least 1 shipment for vendor")
		}
	})

	t.Run("OrderStatusTransitions_AndHistory", func(t *testing.T) {
		fromStatus := string(commerce.StatusPending)
		history := commerce.OrderStatusHistory{
			OrderID:         createdOrderID,
			FromStatus:      &fromStatus,
			ToStatus:        string(commerce.StatusConfirmed),
			Notes:           "Vendor confirmed the order",
			ChangedByUserID: &userID,
		}

		err := repo.UpdateOrderStatus(ctx, createdOrderID, commerce.StatusConfirmed, history)
		if err != nil {
			t.Fatalf("UpdateOrderStatus to confirmed failed: %v", err)
		}

		histList, err := repo.ListOrderHistory(ctx, createdOrderID)
		if err != nil {
			t.Fatalf("ListOrderHistory failed: %v", err)
		}
		if len(histList) < 2 { // initial + transition
			t.Fatalf("expected at least 2 history records, got %d", len(histList))
		}

		// Invalid status transition should fail
		invalidHistory := commerce.OrderStatusHistory{
			OrderID:  createdOrderID,
			ToStatus: string(commerce.StatusPending),
		}
		err = repo.UpdateOrderStatus(ctx, createdOrderID, commerce.StatusPending, invalidHistory)
		if err == nil {
			t.Error("expected error on invalid status transition (confirmed -> pending), got nil")
		}
	})

	t.Run("Shipment_StatusAndConflict", func(t *testing.T) {
		if shipmentID == 0 {
			t.Skip("no shipment ID available")
		}

		shipment, err := repo.GetShipmentByID(ctx, shipmentID)
		if err != nil {
			t.Fatalf("GetShipmentByID failed: %v", err)
		}
		if shipment.ID != shipmentID {
			t.Errorf("got shipment ID %d, want %d", shipment.ID, shipmentID)
		}

		// Transition shipment from pending to confirmed
		fromStatus := string(commerce.StatusPending)
		shipHist := commerce.OrderStatusHistory{
			OrderID:         createdOrderID,
			ShipmentID:      &shipmentID,
			FromStatus:      &fromStatus,
			ToStatus:        string(commerce.StatusConfirmed),
			Notes:           "Shipment confirmed",
			ChangedByUserID: &userID,
		}
		err = repo.UpdateShipmentStatus(ctx, shipmentID, commerce.StatusPending, commerce.StatusConfirmed, shipHist)
		if err != nil {
			t.Fatalf("UpdateShipmentStatus failed: %v", err)
		}

		// Concurrent/stale transition should return conflict error
		staleHist := commerce.OrderStatusHistory{
			OrderID:    createdOrderID,
			ShipmentID: &shipmentID,
			FromStatus: &fromStatus,
			ToStatus:   string(commerce.StatusProcessing),
		}
		err = repo.UpdateShipmentStatus(ctx, shipmentID, commerce.StatusPending, commerce.StatusProcessing, staleHist)
		if err == nil {
			t.Error("expected conflict error for stale shipment status update, got nil")
		}
		if apperr.KindOf(err) != apperr.KindConflict {
			t.Errorf("expected conflict apperr, got: %v", err)
		}
	})

	t.Run("RateOrder_Validations", func(t *testing.T) {
		// Transition order to delivered first so it can be rated
		delivHist := commerce.OrderStatusHistory{
			OrderID:  createdOrderID,
			ToStatus: string(commerce.StatusDelivered),
			Notes:    "Delivered to customer",
		}
		_ = repo.UpdateOrderStatus(ctx, createdOrderID, commerce.StatusDelivered, delivHist)

		// Rate the order
		err := repo.RateOrder(ctx, createdOrderID, testCommerceUserID, 5, "Excellent service and fast delivery")
		if err != nil {
			t.Fatalf("RateOrder failed: %v", err)
		}

		// Wrong customer should get not found error
		err = repo.RateOrder(ctx, createdOrderID, 999999, 4, "Not my order")
		if err == nil {
			t.Error("expected error rating order with wrong customer ID, got nil")
		}
	})
}
