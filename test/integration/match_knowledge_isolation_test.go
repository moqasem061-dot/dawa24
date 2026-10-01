package integration_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// TestTenantIsolation_MatchKnowledge asserts strict tenant isolation on catalog.match_knowledge (T107, US8).
// 1. Tenant A's knowledge rows are completely invisible to Tenant B under tenant transactions.
// 2. Platform lookup returns only organization_id IS NULL rows, respecting opt-out in decision_memory_preferences.
func TestTenantIsolation_MatchKnowledge(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const orgA int64 = 89201
	const orgB int64 = 89202

	// Setup clean state
	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Org A"}') ON CONFLICT DO NOTHING;`, orgA)
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Org B"}') ON CONFLICT DO NOTHING;`, orgB)
		_, _ = tx.Exec(txCtx, `DELETE FROM catalog.match_knowledge WHERE organization_id IN ($1, $2) OR question_signature LIKE 'test_sig_%';`, orgA, orgB)
		_, _ = tx.Exec(txCtx, `DELETE FROM catalog.decision_memory_preferences WHERE organization_id IN ($1, $2);`, orgA, orgB)
		return nil
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	sigOrg := "test_sig_org_a"
	sigPlat := "test_sig_platform"

	// 1. Insert Tenant A's private knowledge row
	var rowAID int64
	ctxA := database.WithTenant(ctx, orgA)
	err = db.InTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx, `
			INSERT INTO catalog.match_knowledge (
				organization_id, source_namespace, question_signature, kind,
				target_granularity, display_text, status, created_by
			) VALUES (
				$1, '*', $2, 'human_confirm',
				'product', 'Panadol Org A', 'active', 'user_a'
			)
			RETURNING id;
		`, orgA, sigOrg)
		return row.Scan(&rowAID)
	})
	if err != nil {
		t.Fatalf("insert match_knowledge for Org A failed: %v", err)
	}

	// 2. Insert Platform knowledge row (organization_id IS NULL) under AsSystem
	var rowPlatID int64
	err = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx, `
			INSERT INTO catalog.match_knowledge (
				organization_id, source_namespace, question_signature, kind,
				target_granularity, display_text, status, created_by
			) VALUES (
				NULL, '*', $1, 'platform_fact',
				'product', 'Concor Platform', 'active', 'admin'
			)
			RETURNING id;
		`, sigPlat)
		return row.Scan(&rowPlatID)
	})
	if err != nil {
		t.Fatalf("insert platform match_knowledge failed: %v", err)
	}

	// 3. Tenant A can read its own knowledge row
	err = db.InReadTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM catalog.match_knowledge WHERE id = $1", rowAID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("Org A cannot read its own match_knowledge row! count = %d; want 1", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read match_knowledge as Org A failed: %v", err)
	}

	// 4. Tenant B gets ZERO rows reading Tenant A's row (cross-tenant RLS isolation)
	ctxB := database.WithTenant(ctx, orgB)
	err = db.InReadTx(ctxB, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM catalog.match_knowledge WHERE id = $1", rowAID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("SECURITY LEAK: Org B read Org A's match_knowledge! count = %d; want 0", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read match_knowledge as Org B failed: %v", err)
	}

	// 5. Normal tenant read TX CANNOT see platform rows (organization_id IS NULL) because RLS requires current_org_id = organization_id
	err = db.InReadTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM catalog.match_knowledge WHERE id = $1", rowPlatID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("SECURITY LEAK: Normal tenant read TX read platform row directly without AsSystem! count = %d; want 0", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("direct platform read check failed: %v", err)
	}

	// 6. Platform lookup with opt-out disabled:
	// Insert preference for Org A: use_platform_memory = false
	err = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, execErr := tx.Exec(txCtx, `
			INSERT INTO catalog.decision_memory_preferences (organization_id, use_platform_memory)
			VALUES ($1, false)
			ON CONFLICT (organization_id) DO UPDATE SET use_platform_memory = false;
		`, orgA)
		return execErr
	})
	if err != nil {
		t.Fatalf("set opt-out preference failed: %v", err)
	}

	// Check preference query
	err = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var usePlatform bool
		queryPref := `SELECT COALESCE((SELECT use_platform_memory FROM catalog.decision_memory_preferences WHERE organization_id = $1), true)`
		if err := tx.QueryRow(txCtx, queryPref, orgA).Scan(&usePlatform); err != nil {
			return err
		}
		if usePlatform {
			t.Errorf("expected use_platform_memory=false for Org A; got true")
		}

		// Check for Org B (no preference row -> default true)
		if err := tx.QueryRow(txCtx, queryPref, orgB).Scan(&usePlatform); err != nil {
			return err
		}
		if !usePlatform {
			t.Errorf("expected use_platform_memory=true for Org B default; got false")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("preference lookup failed: %v", err)
	}
}
