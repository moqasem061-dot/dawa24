package integration_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// TestTenantIsolation_SavingsMatchDecision verifies that catalog.saving_products
// enforces row-level security for the newly added match decision columns:
// a row inserted by tenant A is visible to tenant A and completely invisible to tenant B.
func TestTenantIsolation_SavingsMatchDecision(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const orgA int64 = 89301
	const orgB int64 = 89302

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Org A"}') ON CONFLICT DO NOTHING;`, orgA)
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Org B"}') ON CONFLICT DO NOTHING;`, orgB)
		_, _ = tx.Exec(txCtx, `DELETE FROM catalog.saving_products WHERE organization_id IN ($1, $2);`, orgA, orgB)
		return nil
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	var productID int64
	ctxA := database.WithTenant(ctx, orgA)

	err = db.InTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO catalog.saving_products (
				organization_id, name_product, sku, qty, price,
				match_state, match_basis, match_granularity, match_revision,
				human_locked, ai_state, match_decision, baseline_decision
			) VALUES (
				$1, 'بانادول إكسترا', 'PAN-24', 10.00, 35.50,
				'accepted', 'exact_name', 'product', 1,
				false, 'off', '{"v":1,"state":"accepted"}'::jsonb, '{"v":1,"state":"accepted"}'::jsonb
			)
			RETURNING id;
		`, orgA).Scan(&productID)
	})
	if err != nil {
		t.Fatalf("insert catalog.saving_products for Org A failed: %v", err)
	}

	// 1. Assert Tenant A can read its own row and the new columns
	err = db.InReadTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		var state, basis, granularity, aiState string
		var revision int
		var locked bool
		var decisionBytes, baselineBytes []byte

		err := tx.QueryRow(txCtx, `
			SELECT match_state, match_basis, match_granularity, match_revision,
			       human_locked, ai_state, match_decision, baseline_decision
			FROM catalog.saving_products
			WHERE id = $1`, productID).Scan(
			&state, &basis, &granularity, &revision,
			&locked, &aiState, &decisionBytes, &baselineBytes,
		)
		if err != nil {
			return err
		}

		if state != "accepted" || basis != "exact_name" || granularity != "product" {
			t.Errorf("unexpected column values: state=%s, basis=%s, granularity=%s", state, basis, granularity)
		}
		if revision != 1 || locked || aiState != "off" {
			t.Errorf("unexpected audit values: revision=%d, locked=%v, aiState=%s", revision, locked, aiState)
		}
		if len(decisionBytes) == 0 || len(baselineBytes) == 0 {
			t.Errorf("expected json decisions to be persisted")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read catalog.saving_products as Org A failed: %v", err)
	}

	// 2. Assert Tenant B gets ZERO rows (RLS isolation)
	ctxB := database.WithTenant(ctx, orgB)
	err = db.InReadTx(ctxB, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM catalog.saving_products WHERE id = $1", productID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("SECURITY LEAK: Org B read Org A's saving_product! count = %d; want 0", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read catalog.saving_products as Org B failed: %v", err)
	}
}
