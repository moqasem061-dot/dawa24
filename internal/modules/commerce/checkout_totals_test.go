package commerce

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// An order's money figures have to agree with the sentence the order screen
// prints: إجمالي الأصناف − إجمالي الخصم + الشحن + الضريبة = الصافي.
//
// Checkout stored a subtotal already net of line discounts and the discount
// total beside it, so that sum came out short by exactly the discount. Editing
// the same order recomputed the subtotal gross, so one order's subtotal meant
// two different things depending on whether anyone had touched it. This pins
// the gross convention on the side that creates orders; the postgres package's
// order_edit_test.go pins the side that changes them.
func TestCheckoutStoresAGrossSubtotal(t *testing.T) {
	body, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatalf("read service.go: %v", err)
	}
	src := string(body)

	if !strings.Contains(src, "orderSubtotal, addErr = orderSubtotal.Add(lineSubtotal)") {
		t.Error("the order subtotal is not accumulated gross")
	}
	if strings.Contains(src, "orderSubtotal, addErr = orderSubtotal.Add(lineTotal)") {
		t.Error("the order subtotal is still accumulated net of line discounts")
	}
	if !strings.Contains(src, "shipmentSubtotal, addErr = shipmentSubtotal.Add(lineGross)") {
		t.Error("a shipment's subtotal is not accumulated gross")
	}

	// The discount has to be stored, not zeroed: a subtotal that is gross and a
	// DiscountAmount of zero would report the pre-discount figure as the amount
	// owed on every screen that reads those two fields.
	if strings.Contains(src, "DiscountAmount:    money.Zero,") {
		t.Error("the order's discount is zeroed beside a gross subtotal")
	}

	// The offer's minimum is about what the customer actually pays, so the gate
	// stays on the net figure even though the stored subtotal is gross.
	if !strings.Contains(src, "orderNet.Minor() < input.MinOrderAmount.Minor()") {
		t.Error("the offer minimum is no longer measured against the net total")
	}
}

// TestCheckoutPreDiscountedCatalogPricingNotDoubleDiscounted verifies that when items
// are added from cart/checkout with UnitPrice already reflecting the catalog discount,
// the discount is NOT deducted a second time from the order total.
func TestCheckoutPreDiscountedCatalogPricingNotDoubleDiscounted(t *testing.T) {
	repo := newMockCommerceRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	pID := int64(10)
	vID := int64(101)

	// Item: Public List Price = 100.00 EGP, Effective Unit Price = 80.00 EGP (20% discount = 20.00 EGP/unit)
	// Quantity: 2 units
	// Gross Subtotal should be: 200.00 EGP
	// Total Discount should be: 40.00 EGP
	// Net Customer Total should be: 160.00 EGP (NOT 120.00 EGP!)
	input := CheckoutInput{
		CustomerID:    1,
		PaymentMethod: "cash_on_delivery",
		Items: []CheckoutLineItem{
			{
				VendorOrgID:      5,
				ProductID:        &pID,
				ProductVariantID: &vID,
				Quantity:         2,
				UnitPrice:        money.MustParse("80.00"),
				ListPrice:        money.MustParse("100.00"),
				DiscountAmount:   money.MustParse("40.00"), // (100 - 80) * 2
			},
		},
	}

	order, err := svc.Checkout(context.Background(), input)
	if err != nil {
		t.Fatalf("Checkout failed: %v", err)
	}

	wantSubtotal := money.MustParse("200.00")
	wantDiscount := money.MustParse("40.00")
	wantTotal := money.MustParse("160.00")

	if order.Subtotal != wantSubtotal {
		t.Errorf("order.Subtotal = %v, want %v (gross retail price)", order.Subtotal, wantSubtotal)
	}
	if order.TotalDiscount != wantDiscount {
		t.Errorf("order.TotalDiscount = %v, want %v (granted discount)", order.TotalDiscount, wantDiscount)
	}
	if order.DiscountAmount != wantDiscount {
		t.Errorf("order.DiscountAmount = %v, want %v", order.DiscountAmount, wantDiscount)
	}
	if order.TotalAmount != wantTotal {
		t.Errorf("order.TotalAmount = %v, want %v (net amount customer pays; MUST NOT BE DOUBLE-DISCOUNTED to 120)", order.TotalAmount, wantTotal)
	}
	if order.FinalPrice != wantTotal {
		t.Errorf("order.FinalPrice = %v, want %v", order.FinalPrice, wantTotal)
	}

	lines := repo.lines[order.ID]
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if lines[0].TotalPrice != wantTotal {
		t.Errorf("lines[0].TotalPrice = %v, want %v", lines[0].TotalPrice, wantTotal)
	}

	shipments := repo.shipments[order.ID]
	if len(shipments) != 1 {
		t.Fatalf("expected 1 shipment, got %d", len(shipments))
	}
	if shipments[0].Subtotal != wantSubtotal {
		t.Errorf("shipment.Subtotal = %v, want %v", shipments[0].Subtotal, wantSubtotal)
	}
	if shipments[0].TotalAmount != wantTotal {
		t.Errorf("shipment.TotalAmount = %v, want %v", shipments[0].TotalAmount, wantTotal)
	}
}
