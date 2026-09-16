package commerce

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestCheckoutCalculationsAndSplitting(t *testing.T) {
	repo := newMockCommerceRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	ctx := context.Background()

	p1ID := int64(101)
	p2ID := int64(102)

	input := CheckoutInput{
		CustomerID:    123,
		PaymentMethod: "cash_on_delivery",
		Items: []CheckoutLineItem{
			{
				VendorOrgID: 10,
				ProductID:   &p1ID,
				ProductName: i18n.New("بانادول", "Panadol"),
				UnitPrice:   money.MustParse("20.00"),
				Quantity:    3,
			},
			{
				VendorOrgID: 20,
				ProductID:   &p2ID,
				ProductName: i18n.New("أوجمنتين", "Augmentin"),
				UnitPrice:   money.MustParse("50.00"),
				Quantity:    2,
			},
		},
	}

	order, err := svc.Checkout(ctx, input)
	if err != nil {
		t.Fatalf("Checkout failed: %v", err)
	}

	// 3 * 20.00 = 60.00; 2 * 50.00 = 100.00 => Total 160.00
	expectedSubtotal := money.MustParse("160.00")
	if order.Subtotal != expectedSubtotal {
		t.Errorf("Subtotal = %v; want %v", order.Subtotal, expectedSubtotal)
	}

	shipments := repo.shipments[order.ID]
	if len(shipments) != 2 {
		t.Fatalf("expected 2 shipments for 2 vendors, got %d", len(shipments))
	}

	// Sum of shipments must equal order subtotal
	var sumShipments money.Amount
	for _, s := range shipments {
		var err error
		sumShipments, err = sumShipments.Add(s.Subtotal)
		if err != nil {
			t.Fatalf("money add failed: %v", err)
		}
	}

	if sumShipments != order.Subtotal {
		t.Errorf("Sum of shipment subtotals (%v) != Order subtotal (%v)", sumShipments, order.Subtotal)
	}
}

// TestIsValidStatusTransition pins the Laravel-parity DAG (migration 063).
func TestIsValidStatusTransition(t *testing.T) {
	valid := []struct{ from, to OrderStatus }{
		{StatusPending, StatusProcessing},
		{StatusPending, StatusConfirmed},
		{StatusPending, StatusOnHold},
		{StatusPending, StatusCancelled},
		{StatusProcessing, StatusConfirmed},
		{StatusProcessing, StatusOnHold},
		{StatusProcessing, StatusCancelled},
		{StatusProcessing, StatusFailed},
		{StatusConfirmed, StatusShipped},
		{StatusConfirmed, StatusOnHold},
		{StatusConfirmed, StatusCancelled},
		{StatusConfirmed, StatusFailed},
		{StatusPending, StatusDelivered},
		{StatusConfirmed, StatusDelivered},
		{StatusShipped, StatusDelivered},
		{StatusShipped, StatusCompleted},
		{StatusOnHold, StatusProcessing},
		{StatusOnHold, StatusConfirmed},
		{StatusOnHold, StatusCancelled},
		{StatusOnHold, StatusFailed},
		{StatusShipped, StatusInTransit},
		{StatusShipped, StatusOutForDelivery},
		{StatusShipped, StatusReturned},
		{StatusShipped, StatusFailed},
		{StatusInTransit, StatusOutForDelivery},
		{StatusInTransit, StatusReturned},
		{StatusInTransit, StatusFailed},
		{StatusOutForDelivery, StatusDelivered},
		{StatusOutForDelivery, StatusReturned},
		{StatusOutForDelivery, StatusFailed},
		{StatusDelivered, StatusCompleted},
		{StatusDelivered, StatusReturned},
		{StatusDelivered, StatusRefunded},
		{StatusCompleted, StatusRefunded},
	}
	for _, tc := range valid {
		if !IsValidStatusTransition(tc.from, tc.to) {
			t.Errorf("IsValidStatusTransition(%q, %q) = false; want true", tc.from, tc.to)
		}
	}

	invalid := []struct{ from, to OrderStatus }{
		{StatusCancelled, StatusPending},                 // terminal
		{StatusFailed, StatusRefunded},                   // terminal
		{StatusReturned, StatusRefunded},                 // terminal
		{StatusRefunded, StatusCompleted},                // terminal
		{StatusCompleted, StatusDelivered},               // after completion
		{StatusPending, StatusRefunded},                  // refund without delivery
		{StatusDelivered, StatusCancelled},               // cancelled after delivery
		{OrderStatus("ready_for_pickup"), StatusShipped}, // removed legacy state
	}
	for _, tc := range invalid {
		if IsValidStatusTransition(tc.from, tc.to) {
			t.Errorf("IsValidStatusTransition(%q, %q) = true; want false", tc.from, tc.to)
		}
	}
}

// TestCheckoutOfferModel covers the §3.3 behavior: order carries offer/branch/
// address identity, lines carry the offer snapshots, TotalDiscount/FinalPrice
// reproduce the invoice, and the min-order gate refuses short checkouts.
func TestCheckoutOfferModel(t *testing.T) {
	repo := newMockCommerceRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	ctx := context.Background()
	variantID := int64(33)
	offerProductID := int64(7)
	offerID := int64(9)
	branchID := int64(12)
	vendorBranchID := int64(14)
	addressID := int64(5)

	input := CheckoutInput{
		CustomerID:     123,
		OfferID:        offerID,
		BranchID:       &branchID,
		VendorBranchID: &vendorBranchID,
		UserAddressID:  &addressID,
		PaymentMethod:  "cash_on_delivery",
		MinOrderAmount: money.MustParse("500.00"),
		Items: []CheckoutLineItem{
			{
				VendorOrgID:      10,
				ProductVariantID: &variantID,
				ProductName:      i18n.New("بانادول", "Panadol"),
				UnitPrice:        money.MustParse("45.00"),
				Quantity:         20,
				OfferProductID:   &offerProductID,
				DiscountAmount:   money.MustParse("4.00"),
				ListPrice:        money.MustParse("49.00"),
				OriginalPrice:    money.MustParse("50.00"),
				OriginalDiscount: money.MustParse("5.00"),
			},
		},
	}

	order, err := svc.Checkout(ctx, input)
	if err != nil {
		t.Fatalf("Checkout failed: %v", err)
	}

	if order.OfferID != offerID {
		t.Errorf("Order.OfferID = %d; want %d", order.OfferID, offerID)
	}
	if order.BranchID == nil || *order.BranchID != branchID {
		t.Errorf("Order.BranchID = %v; want %d", order.BranchID, branchID)
	}
	if order.VendorBranchID == nil || *order.VendorBranchID != vendorBranchID {
		t.Errorf("Order.VendorBranchID = %v; want %d", order.VendorBranchID, vendorBranchID)
	}
	if order.UserAddressID == nil || *order.UserAddressID != addressID {
		t.Errorf("Order.UserAddressID = %v; want %d", order.UserAddressID, addressID)
	}

	wantTotalDiscount := money.MustParse("4.00")
	if order.TotalDiscount != wantTotalDiscount {
		t.Errorf("Order.TotalDiscount = %v; want %v", order.TotalDiscount, wantTotalDiscount)
	}
	wantFinal := money.MustParse("896.00") // 20 × 45.00 − 4.00 discount
	if order.FinalPrice != wantFinal {
		t.Errorf("Order.FinalPrice = %v; want %v", order.FinalPrice, wantFinal)
	}

	lines := repo.lines[order.ID]
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	line := lines[0]
	if line.OfferProductID == nil || *line.OfferProductID != offerProductID {
		t.Errorf("Line.OfferProductID = %v; want %d", line.OfferProductID, offerProductID)
	}
	if line.ListPrice != money.MustParse("49.00") {
		t.Errorf("Line.ListPrice = %v; want 49.00", line.ListPrice)
	}
	if line.OriginalPrice != money.MustParse("50.00") {
		t.Errorf("Line.OriginalPrice = %v; want 50.00", line.OriginalPrice)
	}

	// Min-order gate: same order below the minimum is refused.
	lowInput := input
	lowInput.MinOrderAmount = money.MustParse("1000.00")
	if _, err := svc.Checkout(ctx, lowInput); err == nil {
		t.Fatal("Checkout below min_order_amount succeeded; want validation error")
	}

	// A zero/absent minimum does not gate.
	noMin := input
	noMin.MinOrderAmount = money.Zero
	if _, err := svc.Checkout(ctx, noMin); err != nil {
		t.Fatalf("Checkout without min_order_amount failed: %v", err)
	}
}

// T1: Exactness of 3-criteria rating average (audit §3.3)
func TestCalculateAverageRatingExactness(t *testing.T) {
	cases := []struct {
		name     string
		ratings  []int
		expected float64
	}{
		{"4+5+3 exact 4.00", []int{4, 5, 3}, 4.00},
		{"5+4+4 exact 4.33", []int{5, 4, 4}, 4.33},
		{"5+5+5 exact 5.00", []int{5, 5, 5}, 5.00},
		{"1+1+2 exact 1.33", []int{1, 1, 2}, 1.33},
		{"1+2+3 exact 2.00", []int{1, 2, 3}, 2.00},
		{"clamps out-of-range lower", []int{0, 4, 4}, 4.33}, // 0 clamps to 1 -> (1+4+4)/3 = 3.00? Wait, 1+4+4 = 9/3 = 3.00! Wait: (1+4+4)/3 = 3.00
		{"clamps out-of-range upper", []int{6, 5, 5}, 5.00}, // 6 clamps to 5
		{"empty returns 0.00", []int{}, 0.00},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CalculateAverageRating(c.ratings...)
			if c.name == "clamps out-of-range lower" {
				// 0 -> 1 => (1+4+4)/3 = 3.00
				if got != 3.00 {
					t.Errorf("got %v, want 3.00", got)
				}
				return
			}
			if got != c.expected {
				t.Errorf("CalculateAverageRating(%v) = %v; want %v", c.ratings, got, c.expected)
			}
		})
	}
}

// T2 & T6: Order rating lifecycle, validation, and double-rating prevention
func TestRateOrderValidationAndDoubleRating(t *testing.T) {
	ctx := context.Background()
	repo := newMockCommerceRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	order := &Order{
		ID:          50,
		CustomerID:  10,
		Status:      StatusPending,
		TotalAmount: money.MustParse("200.00"),
	}
	repo.orders[50] = order

	// 1. Rating undelivered order fails (T6)
	_, err := svc.RateOrderWithCriteria(ctx, 50, 10, 5, 4, 5, "ممتاز جداً")
	if err == nil {
		t.Fatal("expected error rating undelivered order, got nil")
	}

	// 2. Mark order as delivered
	order.Status = StatusDelivered

	// 3. Invalid rating out of range (<1 or >5) fails
	if err := svc.RateOrder(ctx, 50, 10, 0.5, "ضعيف"); err == nil {
		t.Fatal("expected error for rating 0.5, got nil")
	}
	if err := svc.RateOrder(ctx, 50, 10, 5.5, "ممتاز"); err == nil {
		t.Fatal("expected error for rating 5.5, got nil")
	}

	// 4. Rate delivered order successfully (T2)
	avg, err := svc.RateOrderWithCriteria(ctx, 50, 10, 5, 4, 5, "خدمة ممتازة وسريعة")
	if err != nil {
		t.Fatalf("unexpected error rating order: %v", err)
	}
	if avg != 4.67 { // (5+4+5)/3 = 14/3 = 4.67
		t.Errorf("expected average 4.67, got %v", avg)
	}
	if order.Rating == nil || *order.Rating != 4.67 {
		t.Errorf("order.Rating = %v; want 4.67", order.Rating)
	}
	if order.Review == nil || *order.Review != "خدمة ممتازة وسريعة" {
		t.Errorf("order.Review = %v; want 'خدمة ممتازة وسريعة'", order.Review)
	}
	if order.RatedAt == nil {
		t.Error("expected order.RatedAt to be set")
	}

	// 5. Re-rating the same order fails (T6)
	_, err = svc.RateOrderWithCriteria(ctx, 50, 10, 5, 5, 5, "محاولة تقييم ثانية")
	if err == nil {
		t.Fatal("expected error on re-rating already rated order, got nil")
	}
}
