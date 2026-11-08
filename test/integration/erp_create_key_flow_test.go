package integration_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	erppg "github.com/moqasem061-dot/dawa24/internal/modules/erp/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// TestERPCreateKeyThroughService drives the owner's "new key" path end to end
// on the real schema as the application role: the dropdown read (KeyTargets),
// then Service.CreateKey with exactly what the form posts.
func TestERPCreateKeyThroughService(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	org := time.Now().UnixNano()/1000 + 7
	var userID int64
	err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO org.organizations (id, name, type, status) VALUES ($1, '{"ar":"مورد","en":"Vendor"}', 'vendor', 'approved')`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org.branches (organization_id, name, is_main) VALUES ($1, '{"ar":"الفرع","en":"Branch"}', true)`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO inventory.warehouses (organization_id, name) VALUES ($1, 'Main')`, org); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT id FROM identity.users ORDER BY id LIMIT 1`).Scan(&userID)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(ctx, `DELETE FROM erp.api_keys WHERE organization_id=$1`, org)
			_, _ = tx.Exec(ctx, `DELETE FROM erp.integrations WHERE organization_id=$1`, org)
			_, err := tx.Exec(ctx, `DELETE FROM org.organizations WHERE id=$1`, org)
			return err
		})
	})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := erp.NewService(erppg.New(db), erp.Ports{}, erp.Config{Pepper: []byte("integration pepper of at least thirty-two bytes"), Tag: "test", DefaultRate: 600, MaxBatchItems: 50000, Provider: provider.DefaultProvider}, log)

	set, err := svc.KeyTargets(ctx, org)
	warehouses, branches := set.Warehouses, set.Branches
	if err != nil {
		t.Fatalf("KeyTargets: %v", err)
	}
	if len(warehouses) != 1 || len(branches) != 1 || branches[0].Name["ar"] != "الفرع" || warehouses[0].Name["en"] != "Main" {
		t.Fatalf("KeyTargets = %+v / %+v", warehouses, branches)
	}
	created, err := svc.CreateKey(ctx, erp.CreateKeyInput{
		OrganizationID: org, ActorUserID: userID, ActorIsOwner: true,
		Name: "مفتاح أورجاسوفت", Provider: provider.DefaultProvider,
		WarehouseID: warehouses[0].ID, BranchID: branches[0].ID,
		Scopes: erp.DefaultScopes(true), RateLimitPerMinute: 600,
	})
	if err != nil {
		var e *erp.Error
		if errors.As(err, &e) {
			t.Fatalf("CreateKey: %s %v", e.Code, e.Details)
		}
		t.Fatalf("CreateKey: %v", err)
	}
	if created.FullKey == "" || created.Key.ID == 0 {
		t.Fatalf("created = %+v", created.Key)
	}
	keys, err := svc.ListKeys(ctx, org)
	if err != nil || len(keys) != 1 {
		t.Fatalf("ListKeys = %d, %v", len(keys), err)
	}
	// Pausing is visible to the keys page, and resuming clears it.
	for _, pause := range []bool{true, false} {
		if err := svc.PauseIntegration(ctx, org, pause, userID); err != nil {
			t.Fatalf("PauseIntegration(%v): %v", pause, err)
		}
		if got, err := svc.IntegrationPaused(ctx, org); err != nil || got != pause {
			t.Fatalf("IntegrationPaused = %v, %v; want %v", got, err, pause)
		}
	}
	// Health reads the key back the way the API does.
	row, found, err := erppg.New(db).FindKeyByPrefixAsSystem(ctx, created.Key.KeyPrefix)
	if err != nil || !found || row.WarehouseID != warehouses[0].ID {
		t.Fatalf("key lookup: found=%v warehouse=%d err=%v", found, row.WarehouseID, err)
	}
}

// TestERPPharmacyKeyWithoutWarehouse: a pharmacy key places purchase orders,
// never uploads stock, and so needs no warehouse; the members dropdown lists
// the pharmacy's own active members under the application role; and the
// database still refuses a product-upload key without a warehouse.
func TestERPPharmacyKeyWithoutWarehouse(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	org := time.Now().UnixNano()/1000 + 11
	var userID, branchID int64
	err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO org.organizations (id, name, type, status) VALUES ($1, '{"ar":"صيدلية","en":"Pharmacy"}', 'customer', 'approved')`, org); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO org.branches (organization_id, name, is_main) VALUES ($1, '{"ar":"الفرع","en":"Branch"}', true) RETURNING id`, org).Scan(&branchID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT id FROM identity.users ORDER BY id LIMIT 1`).Scan(&userID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO org.members (organization_id, user_id, role_key, status) VALUES ($1, $2, 'org_owner', 'active')`, org, userID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(ctx, `DELETE FROM erp.api_keys WHERE organization_id=$1`, org)
			_, _ = tx.Exec(ctx, `DELETE FROM erp.integrations WHERE organization_id=$1`, org)
			_, _ = tx.Exec(ctx, `DELETE FROM org.members WHERE organization_id=$1`, org)
			_, err := tx.Exec(ctx, `DELETE FROM org.organizations WHERE id=$1`, org)
			return err
		})
	})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := erppg.New(db)
	svc := erp.NewService(repo, erp.Ports{}, erp.Config{Pepper: []byte("integration pepper of at least thirty-two bytes"), Tag: "test", DefaultRate: 600, MaxBatchItems: 50000, Provider: provider.DefaultProvider}, log)

	set, err := svc.KeyTargets(ctx, org)
	if err != nil || len(set.Members) != 1 || set.Members[0].ID != userID || set.Members[0].Name["ar"] == "" {
		t.Fatalf("members = %+v, %v", set.Members, err)
	}
	buyer := userID
	created, err := svc.CreateKey(ctx, erp.CreateKeyInput{
		OrganizationID: org, ActorUserID: userID, ActorIsOwner: true, Name: "صيدلية",
		BranchID: branchID, BuyingUserID: &buyer, Scopes: erp.DefaultScopes(false),
	})
	if err != nil {
		t.Fatalf("pharmacy CreateKey: %v", err)
	}
	row, found, err := repo.FindKeyByPrefixAsSystem(ctx, created.Key.KeyPrefix)
	if err != nil || !found || row.WarehouseID != 0 {
		t.Fatalf("pharmacy key lookup: found=%v warehouse=%d err=%v", found, row.WarehouseID, err)
	}
	info, err := repo.HealthData(ctx, erp.Principal{OrganizationID: org, KeyID: created.Key.ID})
	if err != nil || info.WarehouseName != "" {
		t.Fatalf("pharmacy health: %+v, %v", info, err)
	}
	if _, err := svc.CreateKey(ctx, erp.CreateKeyInput{
		OrganizationID: org, ActorUserID: userID, ActorIsOwner: true, Name: "uploader",
		BranchID: branchID, Scopes: []string{erp.ScopeProductsSync},
	}); err == nil {
		t.Fatal("a product-upload key without a warehouse was created")
	}
	full, prefix, hash, _ := erp.GenerateKey("test", []byte("x"))
	_ = full
	if _, err := repo.InsertKey(ctx, org, erp.NewKey{APIKey: erp.APIKey{IntegrationID: created.Key.IntegrationID, Name: "raw", KeyPrefix: prefix, Scopes: []string{erp.ScopeProductsSync}, BranchID: branchID, RateLimitPerMinute: 600, MaxBatchItems: 50000}, KeyHash: hash}, erp.AuditRecord{Action: "erp.test", EntityType: "erp_test", ActorKind: "system"}); err == nil {
		t.Fatal("database accepted a product-upload key without a warehouse")
	}
}
