package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	erppg "github.com/moqasem061-dot/dawa24/internal/modules/erp/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

func TestERPKeysRepo(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	orgA := time.Now().UnixNano() / 1000
	orgB := orgA + 1
	var branches, warehouses [2]int64
	err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		for i, org := range []int64{orgA, orgB} {
			if _, err := tx.Exec(ctx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"ERP test"}')`, org); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `INSERT INTO org.branches (organization_id, name) VALUES ($1, '{"en":"Branch"}') RETURNING id`, org).Scan(&branches[i]); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `INSERT INTO inventory.warehouses (organization_id, name) VALUES ($1, 'Warehouse') RETURNING id`, org).Scan(&warehouses[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `DELETE FROM org.organizations WHERE id IN ($1,$2)`, orgA, orgB)
			return err
		}); err != nil {
			t.Error(err)
		}
	})
	repo := erppg.New(db)
	audit := erp.AuditRecord{Action: "erp.test", EntityType: "erp_test", EntityID: fmt.Sprint(orgA), ActorKind: "system"}
	integration, err := repo.EnsureIntegration(ctx, orgA, provider.DefaultProvider, 0, audit)
	if err != nil {
		t.Fatal(err)
	}
	again, err := repo.EnsureIntegration(ctx, orgA, provider.DefaultProvider, 0, audit)
	if err != nil || again.ID != integration.ID {
		t.Fatalf("integration not reused: %v", err)
	}
	other, err := repo.EnsureIntegration(ctx, orgB, provider.DefaultProvider, 0, audit)
	if err != nil {
		t.Fatal(err)
	}
	pepper := []byte("test pepper")
	create := func(org int64, integration erp.Integration, i int) (erp.APIKey, string) {
		t.Helper()
		full, prefix, hash, err := erp.GenerateKey("test", pepper)
		if err != nil {
			t.Fatal(err)
		}
		key, err := repo.InsertKey(ctx, org, erp.NewKey{APIKey: erp.APIKey{IntegrationID: integration.ID, Name: "test", KeyPrefix: prefix, Scopes: []string{erp.ScopeOrdersRead}, WarehouseID: warehouses[i], BranchID: branches[i], RateLimitPerMinute: 600, MaxBatchItems: 50000}, KeyHash: hash}, audit)
		if err != nil {
			t.Fatal(err)
		}
		return key, full
	}
	key, full := create(orgA, integration, 0)
	otherKey, _ := create(orgB, other, 1)
	row, found, err := repo.FindKeyByPrefixAsSystem(ctx, key.KeyPrefix)
	_, secret, valid := erp.ParseKey(full)
	if err != nil || !found || !valid || !erp.VerifyHash(pepper, secret, row.KeyHash) {
		t.Fatalf("key lookup failed: %v", err)
	}
	if _, found, err = repo.FindKeyByPrefixAsSystem(ctx, "zzzzzzzz"); err != nil || found {
		t.Fatalf("unknown prefix found: %v", err)
	}
	if err := repo.TouchLastUsed(ctx, orgA, key.ID, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	first, err := repo.GetKey(ctx, orgA, key.ID)
	if err != nil || first.LastUsedAt == nil {
		t.Fatalf("first touch: %v", err)
	}
	if err := repo.TouchLastUsed(ctx, orgA, key.ID, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	second, err := repo.GetKey(ctx, orgA, key.ID)
	if err != nil || second.LastUsedAt == nil || !second.LastUsedAt.Equal(*first.LastUsedAt) {
		t.Fatalf("touch not throttled: %v", err)
	}
	if err := repo.SetKeyRateLimit(ctx, orgA, key.ID, 700, audit); err != nil {
		t.Fatal(err)
	}
	updated, err := repo.GetKey(ctx, orgA, key.ID)
	if err != nil || updated.RateLimitPerMinute != 700 {
		t.Fatalf("rate update: %v", err)
	}
	replacement, replacementFull := create(orgA, integration, 0)
	expiry := time.Now().Add(time.Hour).Truncate(time.Microsecond)
	if err := repo.SetKeyExpiry(ctx, orgA, key.ID, expiry, audit); err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct {
		key  erp.APIKey
		full string
	}{{key, full}, {replacement, replacementFull}} {
		row, found, err := repo.FindKeyByPrefixAsSystem(ctx, pair.key.KeyPrefix)
		_, secret, ok := erp.ParseKey(pair.full)
		if err != nil || !found || !ok || row.Status != "active" || (row.ExpiresAt != nil && !row.ExpiresAt.After(time.Now())) || !erp.VerifyHash(pepper, secret, row.KeyHash) {
			t.Fatalf("rotation key unusable: %v", err)
		}
	}
	own, err := repo.ListKeys(ctx, orgA)
	if err != nil || len(own) != 2 {
		t.Fatalf("tenant list: %v, count %d", err, len(own))
	}
	for _, k := range own {
		if k.ID == otherKey.ID {
			t.Error("other tenant key leaked")
		}
	}
	staff, err := repo.ListKeysAsStaff(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for _, k := range staff {
		seen[k.Key.ID] = true
	}
	if !seen[key.ID] || !seen[otherKey.ID] {
		t.Error("staff list missing keys")
	}
	if err := repo.RevokeKey(ctx, orgA, key.ID, "test", 0, audit); err != nil {
		t.Fatal(err)
	}
	revoked, err := repo.GetKey(ctx, orgA, key.ID)
	if err != nil || revoked.Status != "revoked" {
		t.Fatalf("revoke: %v", err)
	}
	err = db.InReadTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM platform.audit_log WHERE entity_type = 'erp_test' AND entity_id = $1 AND actor_kind = 'system'`, audit.EntityID).Scan(&count); err != nil {
			return err
		}
		if count != 8 {
			return fmt.Errorf("audit rows = %d, want 8", count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
