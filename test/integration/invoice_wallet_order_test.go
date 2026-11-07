package integration_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	billingPostgres "github.com/moqasem061-dot/dawa24/internal/modules/billing/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// A6: an invoice payment recorded by an admin on a wallet order must not overwrite
// the order's wallet-derived payment status. The shipments decide that status.
func TestInvoicePaymentDoesNotOverwriteWalletOrderStatus(t *testing.T) {
	w := newWRWorld(t)
	vendorB := w.secondVendor(t)
	orderID, ships := w.shipmentOrder(t, []int64{w.vendorOrg, vendorB}, []int64{50000, 30000}, []string{"pending", "pending"}, "pending", false)
	if err := w.confirmAs(w.vendorOrg, ships[0]); err != nil {
		t.Fatalf("confirm supplier A: %v", err)
	}
	ctx := database.AsSystem(context.Background())
	var invoiceID int64
	if err := w.db.Pool().QueryRow(ctx, `INSERT INTO billing.invoices (organization_id, order_id, invoice_number, subtotal, total_amount, status, issue_date, due_date)
		VALUES ($1, $2, $3, 800, 800, 'issued', now(), now() + interval '30 days') RETURNING id`,
		w.buyerOrg, orderID, fmt.Sprintf("INV-A6-%d", orderID)).Scan(&invoiceID); err != nil {
		t.Fatal(err)
	}
	repo := billingPostgres.NewRepository(w.db)
	if _, err := repo.RecordInvoicePayment(ctx, billing.RecordInvoicePaymentRequest{
		InvoiceID:      invoiceID,
		OrganizationID: w.buyerOrg,
		UserID:         w.buyerUser,
		Amount:         money.FromMinor(80000),
		Method:         "cash",
	}); err != nil {
		t.Fatalf("record invoice payment: %v", err)
	}
	if s := w.paymentStatus(t, orderID); s != "partially_paid" {
		t.Fatalf("payment status %q after a full invoice payment, want partially_paid (supplier B is not charged)", s)
	}
}
