package postgres_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	catalogPG "github.com/muhiya/dawa24-store/internal/modules/catalog/postgres"
	ingestPG "github.com/muhiya/dawa24-store/internal/modules/ingest/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

func getScopeTestDB(t *testing.T) *database.DB {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("TEST_DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := config.Database{
		URL:              dbURL,
		MaxConns:         5,
		MinConns:         1,
		MaxConnLifetime:  time.Hour,
		MaxConnIdleTime:  30 * time.Minute,
		StatementTimeout: 10 * time.Second,
	}

	db, err := database.Open(ctx, cfg)
	if err != nil {
		t.Skipf("cannot connect to database: %v", err)
	}
	return db
}

func TestDecisionMemory_ScopeAndOptOutLifecycle(t *testing.T) {
	db := getScopeTestDB(t)
	catRepo := catalogPG.NewRepository(db)
	ingestRepo := ingestPG.NewRepository(db)
	ctx := context.Background()

	rnd := rand.New(rand.NewSource(time.Now().UnixNano())).Int63n(999999)
	key := fmt.Sprintf("test_key_%d", rnd)
	normName := fmt.Sprintf("test_medicine_%d", rnd)

	var orgA, orgB, adminUserID int64
	var prodA, prodB int64
	err := db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `SELECT id FROM org.organizations ORDER BY id LIMIT 2`)
		if err != nil {
			return err
		}
		if rows.Next() {
			_ = rows.Scan(&orgA)
		}
		if rows.Next() {
			_ = rows.Scan(&orgB)
		}
		rows.Close()
		if err := tx.QueryRow(txCtx, `SELECT id FROM identity.users ORDER BY id LIMIT 1`).Scan(&adminUserID); err != nil {
			return err
		}
		return nil
	})
	if err != nil || orgA == 0 || orgB == 0 {
		t.Skipf("cannot find 2 organizations in org.organizations: %v", err)
	}

	// Clean up after test
	defer func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, `DELETE FROM catalog.match_decisions WHERE norm_name = $1 OR decision_key = $2;`, normName, key)
			_, _ = tx.Exec(txCtx, `DELETE FROM catalog.decision_memory_preferences WHERE organization_id IN ($1, $2);`, orgA, orgB)
			return nil
		})
	}()

	err = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT id FROM catalog.products ORDER BY id LIMIT 1`).Scan(&prodA)
	})
	if err != nil {
		t.Skipf("no products in catalog.products: %v", err)
	}
	_ = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT id FROM catalog.products WHERE id <> $1 ORDER BY id LIMIT 1`, prodA).Scan(&prodB)
	})
	if prodB <= 0 {
		prodB = prodA
	}

	// 1. Organization A saves a manual decision
	err = catRepo.SaveManualDecision(ctx, orgA, 1, normName, prodA, "manually matched by A")
	if err != nil {
		t.Fatalf("SaveManualDecision failed: %v", err)
	}

	// Verify it was saved with scope 'org'
	var decID int64
	var scope string
	err = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT id, scope FROM catalog.match_decisions
			WHERE organization_id = $1 AND norm_name = $2;
		`, orgA, normName).Scan(&decID, &scope)
	})
	if err != nil {
		t.Fatalf("could not find saved decision for org A: %v", err)
	}
	if scope != "org" {
		t.Errorf("expected scope 'org', got %q", scope)
	}

	// 2. Query as Org B before promotion: should NOT find it
	ctxOrgB := authctx.WithActor(ctx, authctx.Actor{OrganizationID: orgB, UserID: 2})
	decisionsB, err := ingestRepo.LookupDecisions(ctxOrgB, []string{"manual:" + normName})
	if err != nil {
		t.Fatalf("LookupDecisions for Org B failed: %v", err)
	}
	if len(decisionsB) != 0 {
		t.Fatalf("Org B should not see Org A's unpromoted decision; got %d rows", len(decisionsB))
	}

	// 3. Admin promotes the decision to platform scope
	if err := catRepo.PromoteMatchDecision(ctx, decID, adminUserID); err != nil {
		t.Fatalf("PromoteMatchDecision failed: %v", err)
	}

	// Verify promoted columns
	var promScope string
	var promBy *int64
	_ = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT scope, promoted_by FROM catalog.match_decisions WHERE id = $1;
		`, decID).Scan(&promScope, &promBy)
	})
	if promScope != "platform" {
		t.Errorf("expected promoted scope 'platform', got %q", promScope)
	}
	if promBy == nil || *promBy != adminUserID {
		t.Errorf("expected promoted_by %d, got %v", adminUserID, promBy)
	}

	// 4. Now Org B looks up the decision: must find it as platform decision!
	decisionsB, err = ingestRepo.LookupDecisions(ctxOrgB, []string{"manual:" + normName})
	if err != nil {
		t.Fatalf("LookupDecisions for Org B failed: %v", err)
	}
	if len(decisionsB) != 1 {
		t.Fatalf("Org B should see the promoted platform decision; got %d", len(decisionsB))
	}
	foundB := decisionsB["manual:"+normName]
	if foundB.ChosenProductID == nil || *foundB.ChosenProductID != prodA {
		t.Errorf("expected chosen product %d, got %v", prodA, foundB.ChosenProductID)
	}
	if foundB.Scope != "platform" {
		t.Errorf("expected scope 'platform', got %q", foundB.Scope)
	}

	// 5. Org B toggles platform memory OFF via preference
	if err := catRepo.SetDecisionMemoryPreference(ctx, orgB, false, 2); err != nil {
		t.Fatalf("SetDecisionMemoryPreference(false) failed: %v", err)
	}
	prefB, err := catRepo.GetDecisionMemoryPreference(ctx, orgB)
	if err != nil || prefB != false {
		t.Fatalf("expected pref false, got %v (err: %v)", prefB, err)
	}

	// 6. Org B looks up again: should NOT see the platform decision (opt-out honored!)
	decisionsB, err = ingestRepo.LookupDecisions(ctxOrgB, []string{"manual:" + normName})
	if err != nil {
		t.Fatalf("LookupDecisions for Org B failed: %v", err)
	}
	if len(decisionsB) != 0 {
		t.Fatalf("Org B opted out of platform memory; expected 0 answers, got %d", len(decisionsB))
	}

	// 7. Org B toggles platform memory back ON
	if err := catRepo.SetDecisionMemoryPreference(ctx, orgB, true, 2); err != nil {
		t.Fatalf("SetDecisionMemoryPreference(true) failed: %v", err)
	}

	// 8. Org B now records its OWN decision for the same key (different product)
	err = catRepo.SaveManualDecision(ctx, orgB, 2, normName, prodB, "Org B customized match")
	if err != nil {
		t.Fatalf("SaveManualDecision for Org B failed: %v", err)
	}

	// 9. Org B looks up the decision: Org B's OWN decision must OVERRIDE the platform decision!
	decisionsB, err = ingestRepo.LookupDecisions(ctxOrgB, []string{"manual:" + normName})
	if err != nil {
		t.Fatalf("LookupDecisions for Org B failed: %v", err)
	}
	if len(decisionsB) != 1 {
		t.Fatalf("expected 1 decision for Org B, got %d", len(decisionsB))
	}
	foundB = decisionsB["manual:"+normName]
	if foundB.ChosenProductID == nil || *foundB.ChosenProductID != prodB {
		t.Errorf("Org B's own decision must override platform on collision! Want product %d, got %v", prodB, foundB.ChosenProductID)
	}
	if foundB.Scope != "org" {
		t.Errorf("expected scope 'org' for Org B's override, got %q", foundB.Scope)
	}
}

func TestDecisionMemory_AdminFilteredListAndRelink(t *testing.T) {
	db := getScopeTestDB(t)
	catRepo := catalogPG.NewRepository(db)
	ctx := context.Background()

	rnd := rand.New(rand.NewSource(time.Now().UnixNano())).Int63n(999999)
	rawName := fmt.Sprintf("panadol_test_%d", rnd)

	var orgID, userID int64
	var prod1, prod2 int64
	err := db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx, `SELECT id FROM org.organizations ORDER BY id LIMIT 1`).Scan(&orgID); err != nil {
			return err
		}
		if err := tx.QueryRow(txCtx, `SELECT id FROM identity.users ORDER BY id LIMIT 1`).Scan(&userID); err != nil {
			return err
		}
		if err := tx.QueryRow(txCtx, `SELECT id FROM catalog.products ORDER BY id LIMIT 1`).Scan(&prod1); err != nil {
			return err
		}
		_ = tx.QueryRow(txCtx, `SELECT id FROM catalog.products WHERE id <> $1 ORDER BY id LIMIT 1`, prod1).Scan(&prod2)
		return nil
	})
	if err != nil || orgID == 0 {
		t.Skipf("cannot find organization or product: %v", err)
	}
	if prod2 <= 0 {
		prod2 = prod1
	}

	defer func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, `DELETE FROM catalog.match_decisions WHERE norm_name = $1;`, rawName)
			return nil
		})
	}()

	// Save a test decision
	if err := catRepo.SaveManualDecision(ctx, orgID, userID, rawName, prod1, "initial manual link"); err != nil {
		t.Fatalf("SaveManualDecision failed: %v", err)
	}

	// Test rich filtering: search by rawName
	f := catalog.DecisionMemoryFilter{
		Search:         rawName,
		OrganizationID: &orgID,
		Scope:          "org",
		Limit:          10,
	}
	items, total, err := catRepo.ListMatchDecisionsFiltered(ctx, f)
	if err != nil {
		t.Fatalf("ListMatchDecisionsFiltered failed: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("expected 1 filtered match, got total=%d, items=%d", total, len(items))
	}
	item := items[0]
	if item.NormName != rawName {
		t.Errorf("norm_name = %q, want %q", item.NormName, rawName)
	}

	// Relink to another product as admin
	if err := catRepo.RelinkMatchDecision(ctx, item.ID, &prod2, 1); err != nil {
		t.Fatalf("RelinkMatchDecision failed: %v", err)
	}

	// Verify relinked product and source
	items, _, _ = catRepo.ListMatchDecisionsFiltered(ctx, f)
	if len(items) != 1 || items[0].ChosenProductID == nil || *items[0].ChosenProductID != prod2 {
		t.Errorf("expected relinked product %d, got %v", prod2, items[0].ChosenProductID)
	}
	if items[0].Source != "admin" {
		t.Errorf("expected source 'admin', got %q", items[0].Source)
	}

	// Demote and Bulk delete test
	if err := catRepo.DemoteMatchDecision(ctx, item.ID); err != nil {
		t.Fatalf("DemoteMatchDecision failed: %v", err)
	}
	affected, err := catRepo.BulkDeleteMatchDecisions(ctx, []int64{item.ID})
	if err != nil || affected != 1 {
		t.Fatalf("BulkDeleteMatchDecisions failed: affected=%d, err=%v", affected, err)
	}
}
