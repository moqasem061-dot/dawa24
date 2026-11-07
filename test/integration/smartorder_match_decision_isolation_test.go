package integration_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// TestTenantIsolation_SmartOrderMatchDecision verifies that smartorder.run_lines
// enforces row-level security for the newly added match decision and offer_state columns:
// a row inserted by tenant A is visible to tenant A and completely invisible to tenant B.
func TestTenantIsolation_SmartOrderMatchDecision(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const orgA int64 = 89401
	const orgB int64 = 89402

	var runIDA, runIDB int64

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Org A"}') ON CONFLICT DO NOTHING;`, orgA)
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Org B"}') ON CONFLICT DO NOTHING;`, orgB)

		_, _ = tx.Exec(txCtx, `INSERT INTO identity.users (id, email, password_hash, name) VALUES (1, 'smartorder-iso-1@example.com', 'x', '{"ar":"","en":""}') ON CONFLICT DO NOTHING;`)
		_, _ = tx.Exec(txCtx, `INSERT INTO org.branches (id, organization_id, name, is_main) VALUES (1, $1, '{"en":"Isolation branch"}', false) ON CONFLICT DO NOTHING;`, orgA)
		// Runs from an earlier execution keep their run numbers, which are unique.
		// Clear them first so the fixed numbers below can be inserted again.
		if _, err := tx.Exec(txCtx, `DELETE FROM smartorder.runs WHERE run_number IN ('SO-TEST-ISO-A', 'SO-TEST-ISO-B')`); err != nil {
			return err
		}
		// Create runs for Org A and Org B
		errA := tx.QueryRow(txCtx, `
			INSERT INTO smartorder.runs (
				organization_id, user_id, branch_id, run_number, original_filename, status
			) VALUES ($1, 1, 1, 'SO-TEST-ISO-A', 'file_a.xlsx', 'draft')
			RETURNING id;
		`, orgA).Scan(&runIDA)
		if errA != nil {
			return errA
		}

		errB := tx.QueryRow(txCtx, `
			INSERT INTO smartorder.runs (
				organization_id, user_id, branch_id, run_number, original_filename, status
			) VALUES ($1, 1, 1, 'SO-TEST-ISO-B', 'file_b.xlsx', 'draft')
			RETURNING id;
		`, orgB).Scan(&runIDB)
		if errB != nil {
			return errB
		}

		return nil
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	var lineID int64
	ctxA := database.WithTenant(ctx, orgA)

	err = db.InTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO smartorder.run_lines (
				run_id, organization_id, row_number, raw_name, effective_qty,
				match_state, match_basis, match_granularity, match_revision,
				human_locked, ai_state, match_decision, baseline_decision, offer_state
			) VALUES (
				$1, $2, 1, 'بروفين 400 مجم 30 قرص', 5.00,
				'accepted', 'exact_name', 'pack', 1,
				false, 'off', '{"v":1,"state":"accepted"}'::jsonb, '{"v":1,"state":"accepted"}'::jsonb, 'identified_no_offer'
			)
			RETURNING id;
		`, runIDA, orgA).Scan(&lineID)
	})
	if err != nil {
		t.Fatalf("insert smartorder.run_lines for Org A failed: %v", err)
	}

	// 1. Assert Tenant A can read its own row and the new columns
	err = db.InReadTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		var state, basis, granularity, aiState, offerState string
		var revision int
		var locked bool
		var decisionBytes, baselineBytes []byte

		err := tx.QueryRow(txCtx, `
			SELECT match_state, match_basis, match_granularity, match_revision,
			       human_locked, ai_state, match_decision, baseline_decision, offer_state
			FROM smartorder.run_lines
			WHERE id = $1`, lineID).Scan(
			&state, &basis, &granularity, &revision,
			&locked, &aiState, &decisionBytes, &baselineBytes, &offerState,
		)
		if err != nil {
			return err
		}

		if state != "accepted" || basis != "exact_name" || granularity != "pack" {
			t.Errorf("unexpected column values: state=%s, basis=%s, granularity=%s", state, basis, granularity)
		}
		if revision != 1 || locked || aiState != "off" || offerState != "identified_no_offer" {
			t.Errorf("unexpected audit values: revision=%d, locked=%v, aiState=%s, offerState=%s", revision, locked, aiState, offerState)
		}
		if len(decisionBytes) == 0 || len(baselineBytes) == 0 {
			t.Errorf("expected json decisions to be persisted")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read smartorder.run_lines as Org A failed: %v", err)
	}

	// 2. Assert Tenant B gets ZERO rows (RLS isolation)
	ctxB := database.WithTenant(ctx, orgB)
	err = db.InReadTx(ctxB, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM smartorder.run_lines WHERE id = $1", lineID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("SECURITY LEAK: Org B read Org A's smartorder.run_line! count = %d; want 0", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read smartorder.run_lines as Org B failed: %v", err)
	}
}
