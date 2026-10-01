package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// TestTenantIsolation_CompareMatchDecision verifies that compare.file_rows
// enforces row-level security for the newly added match decision and group_basis columns:
// a row inserted by tenant A is visible to tenant A and completely invisible to tenant B.
func TestTenantIsolation_CompareMatchDecision(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const orgA int64 = 89501
	const orgB int64 = 89502

	var fileIDA, fileIDB int64

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Org A"}') ON CONFLICT DO NOTHING;`, orgA)
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Org B"}') ON CONFLICT DO NOTHING;`, orgB)

		// Create files for Org A and Org B
		errA := tx.QueryRow(txCtx, `
			INSERT INTO compare.files (
				organization_id, user_id, supplier_name, original_filename, storage_key, mime_type, status
			) VALUES ($1, 1, 'المورد أ', 'file_a.xlsx', 'key_a', 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', 'ready')
			RETURNING id;
		`, orgA).Scan(&fileIDA)
		if errA != nil {
			return errA
		}

		errB := tx.QueryRow(txCtx, `
			INSERT INTO compare.files (
				organization_id, user_id, supplier_name, original_filename, storage_key, mime_type, status
			) VALUES ($1, 1, 'المورد ب', 'file_b.xlsx', 'key_b', 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', 'ready')
			RETURNING id;
		`, orgB).Scan(&fileIDB)
		if errB != nil {
			return errB
		}

		return nil
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	var rowID int64
	ctxA := database.WithTenant(ctx, orgA)

	err = db.InTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO compare.file_rows (
				file_id, organization_id, row_number, raw_name, normalized_name, price, discount, price_after_discount,
				match_state, match_basis, match_granularity, match_revision,
				human_locked, ai_state, match_decision, baseline_decision, group_basis
			) VALUES (
				$1, $2, 1, 'كتافلام 50 مجم 20 قرص', 'كتافلام 50 مجم 20 قرص', 50.00, 10.00, 45.00,
				'accepted', 'exact_name', 'pack', 1,
				false, 'off', '{"v":1,"state":"accepted"}'::jsonb, '{"v":1,"state":"accepted"}'::jsonb, 'canonical_pack'
			)
			RETURNING id;
		`, fileIDA, orgA).Scan(&rowID)
	})
	if err != nil {
		t.Fatalf("insert compare.file_rows for Org A failed: %v", err)
	}

	// 1. Assert Tenant A can read its own row and the new columns
	err = db.InReadTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		var state, basis, granularity, aiState, groupBasis string
		var revision int
		var locked bool
		var decisionBytes, baselineBytes []byte

		err := tx.QueryRow(txCtx, `
			SELECT match_state, match_basis, match_granularity, match_revision,
			       human_locked, ai_state, match_decision, baseline_decision, group_basis
			FROM compare.file_rows
			WHERE id = $1`, rowID).Scan(
			&state, &basis, &granularity, &revision,
			&locked, &aiState, &decisionBytes, &baselineBytes, &groupBasis,
		)
		if err != nil {
			return err
		}

		if state != "accepted" || basis != "exact_name" || granularity != "pack" || groupBasis != "canonical_pack" {
			t.Errorf("unexpected column values: state=%s, basis=%s, granularity=%s, group_basis=%s", state, basis, granularity, groupBasis)
		}
		if revision != 1 || locked != false || aiState != "off" {
			t.Errorf("unexpected audit values: revision=%d, locked=%v, aiState=%s", revision, locked, aiState)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tenant A read failed: %v", err)
	}

	// 2. Assert Tenant B CANNOT read Tenant A's row (must return ErrNoRows under RLS)
	ctxB := database.WithTenant(ctx, orgB)
	err = db.InReadTx(ctxB, func(txCtx context.Context, tx pgx.Tx) error {
		var state string
		err := tx.QueryRow(txCtx, `SELECT match_state FROM compare.file_rows WHERE id = $1`, rowID).Scan(&state)
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("expected ErrNoRows for tenant B reading tenant A row, got %v (state=%s)", err, state)
		}

		// Also check that tenant B sees 0 rows in compare.file_rows for file A
		var count int
		if err := tx.QueryRow(txCtx, `SELECT count(*) FROM compare.file_rows WHERE file_id = $1`, fileIDA).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("tenant B can count tenant A rows: got %d, want 0", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tenant B read failed: %v", err)
	}
}
