package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// The S2 tables get the same proof the S1 tables have: a transaction pinned to
// organization B reads zero rows of organization A (rule 19; time-based ids).
// The ERP tables are covered in erp_isolation_test.go; this file exists so both
// stay under the 400-line ceiling (T097).

func TestTenantIsolation_ERPProductLinks(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("isolation test requires a database")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	orgA := time.Now().UnixNano() / 1000
	orgB := orgA + 1
	var id int64
	err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"ERP sync A"}'), ($2, '{"en":"ERP sync B"}')`, orgA, orgB); err != nil {
			return err
		}
		var integration, product, variant int64
		if err := tx.QueryRow(ctx, `INSERT INTO erp.integrations (organization_id, provider) VALUES ($1, 'orgasoft') RETURNING id`, orgA).Scan(&integration); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO catalog.products (organization_id, name) VALUES ($1, '{"en":"P"}') RETURNING id`, orgA).Scan(&product); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO catalog.product_variants (organization_id, product_id, name, sku) VALUES ($1, $2, '{"en":"V"}', $3) RETURNING id`, orgA, product, fmt.Sprintf("sku-%d", orgA)).Scan(&variant); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO erp.product_links (organization_id, integration_id, external_code, variant_id, link_source) VALUES ($1, $2, $3, $4, 'sku_match') RETURNING id`, orgA, integration, fmt.Sprintf("code-%d", orgA), variant).Scan(&id)
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	cleanupERPOrgs(t, db, ctx, orgA, orgB)
	assertERPCrossTenantZero(t, db, ctx, "product_links", id, orgA, orgB)
}

func TestTenantIsolation_ERPSyncBatches(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("isolation test requires a database")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	orgA := time.Now().UnixNano()/1000 + 100
	orgB := orgA + 1
	var id int64
	err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"ERP sync A"}'), ($2, '{"en":"ERP sync B"}')`, orgA, orgB); err != nil {
			return err
		}
		var integrationID int64
		if err := tx.QueryRow(ctx, `INSERT INTO erp.integrations (organization_id, provider) VALUES ($1, 'orgasoft') RETURNING id`, orgA).Scan(&integrationID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO erp.sync_batches (organization_id, integration_id, warehouse_id, branch_id, body_sha256, item_count, chunk_count, accepted_at) VALUES ($1, $2, 1, 1, 'x', 0, 0, now()) RETURNING id`, orgA, integrationID).Scan(&id)
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	cleanupERPOrgs(t, db, ctx, orgA, orgB)
	assertERPCrossTenantZero(t, db, ctx, "sync_batches", id, orgA, orgB)
}

func TestTenantIsolation_ERPSyncItems(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("isolation test requires a database")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	orgA := time.Now().UnixNano()/1000 + 200
	orgB := orgA + 1
	var id int64
	err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"ERP sync A"}'), ($2, '{"en":"ERP sync B"}')`, orgA, orgB); err != nil {
			return err
		}
		var integrationID, batchID int64
		if err := tx.QueryRow(ctx, `INSERT INTO erp.integrations (organization_id, provider) VALUES ($1, 'orgasoft') RETURNING id`, orgA).Scan(&integrationID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO erp.sync_batches (organization_id, integration_id, warehouse_id, branch_id, body_sha256, item_count, chunk_count, accepted_at) VALUES ($1, $2, 1, 1, 'x', 0, 0, now()) RETURNING id`, orgA, integrationID).Scan(&batchID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO erp.sync_items (organization_id, batch_id, position, chunk_no, external_code, state) VALUES ($1, $2, 0, 0, $3, 'pending') RETURNING id`, orgA, batchID, fmt.Sprintf("code-%d", orgA)).Scan(&id)
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	cleanupERPOrgs(t, db, ctx, orgA, orgB)
	assertERPCrossTenantZero(t, db, ctx, "sync_items", id, orgA, orgB)
}

func TestTenantIsolation_ERPReviewItems(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("isolation test requires a database")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	orgA := time.Now().UnixNano()/1000 + 300
	orgB := orgA + 1
	var id int64
	err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"ERP sync A"}'), ($2, '{"en":"ERP sync B"}')`, orgA, orgB); err != nil {
			return err
		}
		var integrationID int64
		if err := tx.QueryRow(ctx, `INSERT INTO erp.integrations (organization_id, provider) VALUES ($1, 'orgasoft') RETURNING id`, orgA).Scan(&integrationID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO erp.review_items (organization_id, integration_id, external_code, payload, reason_code) VALUES ($1, $2, $3, '{"code":"x"}', 'ERP_ITEM_MATCH_REVIEW') RETURNING id`, orgA, integrationID, fmt.Sprintf("code-%d", orgA)).Scan(&id)
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	cleanupERPOrgs(t, db, ctx, orgA, orgB)
	assertERPCrossTenantZero(t, db, ctx, "review_items", id, orgA, orgB)
}

// cleanupERPOrgs removes the fixture organizations; the erp tables cascade.
func cleanupERPOrgs(t *testing.T, db interface {
	InTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error
}, ctx context.Context, orgs ...int64) {
	t.Helper()
	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `DELETE FROM org.organizations WHERE id = ANY($1)`, orgs)
			return err
		})
	})
}

// assertERPCrossTenantZero is the shared assertion: organization A reads its
// row, organization B reads zero of them.
func assertERPCrossTenantZero(t *testing.T, db interface {
	InReadTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error
}, ctx context.Context, table string, id, orgA, orgB int64) {
	t.Helper()
	for _, tc := range []struct {
		org    int64
		expect int
	}{{orgA, 1}, {orgB, 0}} {
		err := db.InReadTx(database.WithTenant(ctx, tc.org), func(ctx context.Context, tx pgx.Tx) error {
			var count int
			if err := tx.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM erp.%s WHERE id = $1", table), id).Scan(&count); err != nil {
				return err
			}
			if count != tc.expect {
				return fmt.Errorf("organization %d sees %d rows of %s.%d, want %d", tc.org, count, table, id, tc.expect)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
