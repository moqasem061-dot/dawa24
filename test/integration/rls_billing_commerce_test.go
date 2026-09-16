package integration_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

func TestTenantIsolation_Invoices(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const orgA int64 = 89003
	const orgB int64 = 89004

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Org A"}') ON CONFLICT DO NOTHING;`, orgA)
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Org B"}') ON CONFLICT DO NOTHING;`, orgB)
		_, _ = tx.Exec(txCtx, `DELETE FROM billing.invoices WHERE organization_id IN ($1, $2);`, orgA, orgB)
		return nil
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	var invoiceID int64
	ctxA := database.WithTenant(ctx, orgA)
	err = db.InTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx, `
			INSERT INTO billing.invoices (organization_id, invoice_number, subtotal, status, issue_date, due_date)
			VALUES ($1, 'INV-RLS-89003', 150.00, 'draft', now(), now() + interval '30 days')
			RETURNING id;
		`, orgA)
		return row.Scan(&invoiceID)
	})
	if err != nil {
		t.Fatalf("insert invoice for Org A failed: %v", err)
	}

	ctxB := database.WithTenant(ctx, orgB)
	err = db.InReadTx(ctxB, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM billing.invoices WHERE id = $1", invoiceID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("SECURITY LEAK: Org B read Org A's invoice! count = %d; want 0", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read invoice as Org B failed: %v", err)
	}
}

func TestTenantIsolation_QuoteRequests(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const orgA int64 = 89005
	const orgB int64 = 89006

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Org A"}') ON CONFLICT DO NOTHING;`, orgA)
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Org B"}') ON CONFLICT DO NOTHING;`, orgB)
		_, _ = tx.Exec(txCtx, `DELETE FROM commerce.quote_requests WHERE organization_id IN ($1, $2) OR customer_org_id IN ($1, $2);`, orgA, orgB)
		return nil
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	var quoteID int64
	ctxA := database.WithTenant(ctx, orgA)
	err = db.InTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx, `
			INSERT INTO commerce.quote_requests (organization_id, customer_org_id, product_name, requested_quantity, status, buyer_notes)
			VALUES ($1, $1, 'Test Product', 1, 'pending', 'Test Quote RLS')
			RETURNING id;
		`, orgA)
		return row.Scan(&quoteID)
	})
	if err != nil {
		t.Fatalf("insert quote for Org A failed: %v", err)
	}

	ctxB := database.WithTenant(ctx, orgB)
	err = db.InReadTx(ctxB, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM commerce.quote_requests WHERE id = $1 AND organization_id = $2", quoteID, orgB).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("SECURITY LEAK: Org B claimed Org A's quote! count = %d; want 0", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read quote as Org B failed: %v", err)
	}
}
