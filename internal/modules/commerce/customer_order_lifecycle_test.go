package commerce

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// TestUpdateCustomerPendingOrder_ServiceLifecycleAndStatusLock verifies that:
// 1. Pending orders can be edited by the customer.
// 2. Confirmed or non-pending orders are strictly rejected and locked.
// 3. Unauthorized users cannot edit someone else's order.
func TestUpdateCustomerPendingOrder_ServiceLifecycleAndStatusLock(t *testing.T) {
	ctx := context.Background()
	repo := newMockCommerceRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	customerOrgID := int64(100)
	order := &Order{
		ID:             99,
		CustomerID:     42,
		OrganizationID: &customerOrgID,
		Status:         StatusPending,
		Subtotal:       money.MustParse("500.00"),
		TotalAmount:    money.MustParse("500.00"),
	}
	repo.orders[99] = order

	authCustomer := authctx.Actor{
		UserID:         42,
		OrganizationID: 100,
		Role:           "customer",
	}

	authOtherCustomer := authctx.Actor{
		UserID:         999,
		OrganizationID: 999,
		Role:           "customer",
	}

	editInput := UpdateCustomerOrderInput{
		OrderID: 99,
		Lines: []OrderLineEditItem{
			{ID: 1, ProductName: "Panadol 500mg", Quantity: 5, UnitPrice: money.MustParse("50.00")},
		},
	}

	// 1. Authorized customer editing pending order succeeds
	res, err := svc.UpdateCustomerPendingOrder(ctx, authCustomer, editInput)
	if err != nil {
		t.Fatalf("expected pending order edit to succeed, got error: %v", err)
	}
	if res.ID != 99 {
		t.Errorf("expected order ID 99, got %d", res.ID)
	}

	// 2. Unauthorized customer fails
	_, err = svc.UpdateCustomerPendingOrder(ctx, authOtherCustomer, editInput)
	if err == nil {
		t.Fatal("expected unauthorized customer edit to fail, got nil")
	}

	// 3. Vendor confirms order -> Status becomes Confirmed
	order.Status = StatusConfirmed

	// 4. Customer editing confirmed order is strictly rejected (status lock)
	_, err = svc.UpdateCustomerPendingOrder(ctx, authCustomer, editInput)
	if err == nil {
		t.Fatal("expected confirmed order edit to fail with locked error, got nil")
	}

	// 5. Customer editing processing/shipped/delivered order is also rejected
	for _, status := range []OrderStatus{StatusProcessing, StatusShipped, StatusDelivered, StatusCancelled} {
		order.Status = status
		_, err := svc.UpdateCustomerPendingOrder(ctx, authCustomer, editInput)
		if err == nil {
			t.Fatalf("expected order in status %s to be locked from customer editing, got nil", status)
		}
	}
}
