package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// TestTenantIsolation_MatchAIAttempts verifies that catalog.match_ai_attempts enforces
// strict tenant isolation via FORCE RLS (T119).
// Org A's attempt records must return zero rows when queried under Org B's context.
func TestTenantIsolation_MatchAIAttempts(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const orgA int64 = 89301
	const orgB int64 = 89302

	var runIDA int64

	// Setup clean state
	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Org A"}') ON CONFLICT DO NOTHING;`, orgA)
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Org B"}') ON CONFLICT DO NOTHING;`, orgB)

		_, _ = tx.Exec(txCtx, `DELETE FROM catalog.match_ai_attempts WHERE organization_id IN ($1, $2);`, orgA, orgB)
		_, _ = tx.Exec(txCtx, `DELETE FROM catalog.match_runs WHERE organization_id IN ($1, $2);`, orgA, orgB)

		row := tx.QueryRow(txCtx, `
			INSERT INTO catalog.match_runs (
				organization_id, workflow, entry_point, consumer_ref,
				policy_version, catalog_revision, normalizer_version, state
			) VALUES (
				$1, 'vendor', 'vendor_import', 'ingest.vendor:1',
				'v1', 1, 'v1', 'enhancing'
			) RETURNING id;
		`, orgA)
		return row.Scan(&runIDA)
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// 1. Insert an attempt under Org A's tenant context
	var attemptIDA int64
	ctxA := database.WithTenant(ctx, orgA)
	deadline := time.Now().Add(15 * time.Second)

	err = db.InTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx, `
			INSERT INTO catalog.match_ai_attempts (
				run_id, organization_id, attempt_no, kind,
				question_count, deadline_at, outcome, request_sha256
			) VALUES (
				$1, $2, 1, 'proposal',
				5, $3, 'ok', 'sha256_dummy_test'
			) RETURNING id;
		`, runIDA, orgA, deadline)
		return row.Scan(&attemptIDA)
	})
	if err != nil {
		t.Fatalf("Org A attempt insertion failed: %v", err)
	}

	// 2. Org A can read its own attempt
	err = db.InTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		err := tx.QueryRow(txCtx, `SELECT COUNT(*) FROM catalog.match_ai_attempts WHERE id = $1;`, attemptIDA).Scan(&count)
		if err != nil {
			return err
		}
		if count != 1 {
			t.Fatalf("Org A expected to see 1 attempt row, got %d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Org A read failed: %v", err)
	}

	// 3. Org B must see 0 rows for Org A's attempt (RLS isolation)
	ctxB := database.WithTenant(ctx, orgB)
	err = db.InTx(ctxB, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		err := tx.QueryRow(txCtx, `SELECT COUNT(*) FROM catalog.match_ai_attempts WHERE id = $1;`, attemptIDA).Scan(&count)
		if err != nil {
			return err
		}
		if count != 0 {
			t.Fatalf("cross-tenant read returned %d rows; expected 0 (RLS breach)", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Org B read query failed: %v", err)
	}

	// 4. Org B attempting to insert for Org A must be blocked by RLS WITH CHECK
	err = db.InTx(ctxB, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			INSERT INTO catalog.match_ai_attempts (
				run_id, organization_id, attempt_no, kind,
				question_count, deadline_at, outcome, request_sha256
			) VALUES (
				$1, $2, 2, 'proposal',
				5, $3, 'ok', 'sha256_breach_attempt'
			);
		`, runIDA, orgA, deadline)
		return err
	})
	if err == nil {
		t.Fatal("expected cross-tenant insert to be rejected by RLS WITH CHECK, but it succeeded")
	}
}
