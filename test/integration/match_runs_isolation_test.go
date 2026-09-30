package integration_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// TestTenantIsolation_MatchRuns verifies that catalog.match_runs enforces
// row-level security: a run inserted by tenant A is completely invisible to tenant B.
func TestTenantIsolation_MatchRuns(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const orgA int64 = 89101
	const orgB int64 = 89102

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Org A"}') ON CONFLICT DO NOTHING;`, orgA)
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Org B"}') ON CONFLICT DO NOTHING;`, orgB)
		_, _ = tx.Exec(txCtx, `DELETE FROM catalog.match_runs WHERE organization_id IN ($1, $2);`, orgA, orgB)
		return nil
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	var runID int64
	ctxA := database.WithTenant(ctx, orgA)
	err = db.InTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx, `
			INSERT INTO catalog.match_runs (
				organization_id, workflow, entry_point, consumer_ref,
				source_revision, policy_version, catalog_revision, normalizer_version,
				state, stage_timings, counts, ai, created_by
			) VALUES (
				$1, 'vendor', 'test_entrypoint', 'test_consumer:1',
				1, 'pm-legacy-1', 1, 'n1',
				'preparing', '{}'::jsonb, '{}'::jsonb, '{}'::jsonb, 'test'
			)
			RETURNING id;
		`, orgA)
		return row.Scan(&runID)
	})
	if err != nil {
		t.Fatalf("insert match_run for Org A failed: %v", err)
	}

	// 1. Assert Tenant A can read its own run
	err = db.InReadTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM catalog.match_runs WHERE id = $1", runID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("Org A cannot read its own match_run! count = %d; want 1", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read match_run as Org A failed: %v", err)
	}

	// 2. Assert Tenant B gets ZERO rows (cross-tenant isolation)
	ctxB := database.WithTenant(ctx, orgB)
	err = db.InReadTx(ctxB, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM catalog.match_runs WHERE id = $1", runID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("SECURITY LEAK: Org B read Org A's match_run! count = %d; want 0", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read match_run as Org B failed: %v", err)
	}
}
