package integration_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// TestTenantIsolation_MatchShadowOutcomes verifies that catalog.match_shadow_outcomes enforces
// strict tenant isolation via FORCE RLS (T150).
// Org A's shadow outcome records must return zero rows when queried under Org B's context.
func TestTenantIsolation_MatchShadowOutcomes(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const orgA int64 = 89401
	const orgB int64 = 89402

	var runIDA int64

	// Setup clean state
	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Org A"}') ON CONFLICT DO NOTHING;`, orgA)
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Org B"}') ON CONFLICT DO NOTHING;`, orgB)

		// Seed a policy version if not present
		_, _ = tx.Exec(txCtx, `
			INSERT INTO catalog.match_policies (version, body, created_by)
			VALUES ('shadow-test-v1', '{"thresholds":{"auto":0.95}}'::jsonb, 'test')
			ON CONFLICT (version) DO NOTHING;
		`)

		_, _ = tx.Exec(txCtx, `DELETE FROM catalog.match_shadow_outcomes WHERE organization_id IN ($1, $2);`, orgA, orgB)
		_, _ = tx.Exec(txCtx, `DELETE FROM catalog.match_runs WHERE organization_id IN ($1, $2);`, orgA, orgB)

		row := tx.QueryRow(txCtx, `
			INSERT INTO catalog.match_runs (
				organization_id, workflow, entry_point, consumer_ref,
				policy_version, catalog_revision, normalizer_version, state
			) VALUES (
				$1, 'vendor', 'vendor_import', 'ingest.vendor:shadow_test',
				'shadow-test-v1', 1, 'v1', 'reviewable'
			) RETURNING id;
		`, orgA)
		return row.Scan(&runIDA)
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// 1. Insert a shadow outcome under Org A's tenant context
	var outcomeIDA int64
	ctxA := database.WithTenant(ctx, orgA)

	err = db.InTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx, `
			INSERT INTO catalog.match_shadow_outcomes (
				run_id, organization_id, row_ref, shadow_policy_version, decision
			) VALUES (
				$1, $2, 'row_101', 'shadow-test-v1', '{"state":"suggested","product_id":555}'::jsonb
			) RETURNING id;
		`, runIDA, orgA)
		return row.Scan(&outcomeIDA)
	})
	if err != nil {
		t.Fatalf("Org A shadow outcome insertion failed: %v", err)
	}

	// 2. Org A can read its own shadow outcome
	err = db.InTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		err := tx.QueryRow(txCtx, `SELECT COUNT(*) FROM catalog.match_shadow_outcomes WHERE id = $1;`, outcomeIDA).Scan(&count)
		if err != nil {
			return err
		}
		if count != 1 {
			t.Fatalf("Org A expected to see 1 shadow outcome row, got %d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Org A read failed: %v", err)
	}

	// 3. Org B must see 0 rows for Org A's shadow outcome (RLS isolation)
	ctxB := database.WithTenant(ctx, orgB)
	err = db.InTx(ctxB, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		err := tx.QueryRow(txCtx, `SELECT COUNT(*) FROM catalog.match_shadow_outcomes WHERE id = $1;`, outcomeIDA).Scan(&count)
		if err != nil {
			return err
		}
		if count != 0 {
			t.Fatalf("cross-tenant read returned %d rows; expected 0 (RLS breach)", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Org B isolation query failed: %v", err)
	}

	// 4. Org B cannot update or delete Org A's shadow outcome
	err = db.InTx(ctxB, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `DELETE FROM catalog.match_shadow_outcomes WHERE id = $1;`, outcomeIDA)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 0 {
			t.Fatalf("cross-tenant delete affected %d rows; expected 0 (RLS breach)", tag.RowsAffected())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Org B delete query failed: %v", err)
	}
}
