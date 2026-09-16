package commerce

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// TestCheckout_DocumentsGate verifies the §4.2 gate: the injected checker
// refuses checkout for organizations with missing mandatory documents and is
// bypassed when the buyer org cannot be determined.
func TestCheckout_DocumentsGate(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pID := int64(10)
	input := CheckoutInput{
		CustomerID:    100,
		CustomerOrgID: 7,
		PaymentMethod: "cod",
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

	t.Run("gate blocks missing documents", func(t *testing.T) {
		svc := NewService(newMockCommerceRepo(), logger)
		blocked := false
		svc.SetRequiredDocsChecker(func(ctx context.Context, orgID int64, orgType string) error {
			blocked = orgID == 7 && orgType == "customer"
			return errors.New("documents.incomplete: mandatory documents missing")
		})

		if _, err := svc.Checkout(context.Background(), input); err == nil {
			t.Fatal("checkout must be refused when the documents gate fails")
		}
		if !blocked {
			t.Fatal("the gate must have been consulted with the buyer org")
		}
	})

	t.Run("gate passes and checkout proceeds", func(t *testing.T) {
		svc := NewService(newMockCommerceRepo(), logger)
		svc.SetRequiredDocsChecker(func(context.Context, int64, string) error { return nil })
		if _, err := svc.Checkout(context.Background(), input); err != nil {
			t.Fatalf("checkout must proceed when documents are complete: %v", err)
		}
	})

	t.Run("no checker means no gate", func(t *testing.T) {
		svc := NewService(newMockCommerceRepo(), logger)
		if _, err := svc.Checkout(context.Background(), input); err != nil {
			t.Fatalf("checkout must proceed without a checker installed: %v", err)
		}
	})

	t.Run("unknown buyer org bypasses the gate", func(t *testing.T) {
		svc := NewService(newMockCommerceRepo(), logger)
		consulted := false
		svc.SetRequiredDocsChecker(func(context.Context, int64, string) error {
			consulted = true
			return errors.New("must not be consulted")
		})
		in := input
		in.CustomerOrgID = 0
		if _, err := svc.Checkout(context.Background(), in); err != nil {
			t.Fatalf("checkout must proceed when the buyer org is unknown: %v", err)
		}
		if consulted {
			t.Fatal("the gate must not run without a buyer organization")
		}
	})
}
