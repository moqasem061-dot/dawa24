// Package integration_test verifies PostgreSQL row-level security policies.
//
// Every tenant-owned table must be tested here to prove that a query without
// tenant filtering returns zero rows for another tenant (ADR 0003).
package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

func getTestDB(t *testing.T) *database.DB {
	t.Helper()
	db := connectTestDB(t)
	if db == nil {
		return nil
	}

	// A superuser bypasses row-level security unconditionally, FORCE included.
	// Running these checks as one reports a leak on every assertion, which says
	// nothing about whether the policies are correct — only that the connection
	// is exempt from them. Skip with a clear reason rather than emit four
	// alarming failures that are really one configuration fact.
	//
	// It is a skip for *isolation* tests only. A test that merely needs a real
	// PostgreSQL to parse a query — see courier_queue_test.go — calls
	// connectTestDB directly, because skipping those on a superuser DSN means
	// never running them on the connection a developer actually has.
	var isSuper bool
	if err := db.Pool().QueryRow(context.Background(),
		`SELECT rolsuper FROM pg_roles WHERE rolname = current_user`).Scan(&isSuper); err == nil && isSuper {
		t.Skip("connected as a superuser, which bypasses row-level security; " +
			"point DATABASE_URL at the least-privilege application role to verify isolation")
		return nil
	}
	return db
}

// connectTestDB opens the test database and verifies its schema is current. It
// makes no claim about which role the caller is connected as.
func connectTestDB(t *testing.T) *database.DB {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping database integration test in short mode")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("TEST_DATABASE_URL")
	}
	if dbURL == "" {
		if os.Getenv("CI") == "true" {
			t.Fatal("DATABASE_URL or TEST_DATABASE_URL must be set in CI")
		}
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
		if os.Getenv("CI") == "true" {
			t.Fatalf("CI failed to connect to database at %s: %v", dbURL, err)
		}
		t.Skipf("cannot connect to database at %s: %v; skipping", dbURL, err)
		return nil
	}

	// Ensure migrations are applied
	migrations, err := database.LoadMigrations(dbfs.Migrations, "migrations")
	if err != nil {
		t.Fatalf("failed to load migrations: %v", err)
	}

	// Deliberately does NOT run migrations.
	//
	// Migrations are a deploy step performed by a privileged role (ADR 0004).
	// This suite must connect as the least-privilege application role, because
	// that is the only way it can prove anything: a superuser bypasses
	// row-level security unconditionally, so running these checks as one
	// reports leaks that do not exist in production and hides the ones that do.
	//
	// The application role cannot create schemas, so it verifies the schema is
	// current instead of making it so.
	pending, err := db.PendingCount(ctx, migrations)
	if err != nil {
		t.Fatalf("cannot read migration state: %v", err)
	}
	if pending > 0 {
		t.Fatalf("%d migrations pending; run `cli migrate` as an admin role before this suite", pending)
	}

	return db
}

// resetFixtures removes rows left by a previous run.
//
// The suite inserts at fixed ids so a failure is easy to inspect afterwards,
// which means it must clear them first or the second run collides with the
// first. Runs as system because it spans both test tenants deliberately.
func resetFixtures(t *testing.T, db *database.DB, orgs ...int64) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		for _, org := range orgs {
			if _, err := tx.Exec(txCtx, `DELETE FROM org.members WHERE organization_id = $1`, org); err != nil {
				return err
			}
			if _, err := tx.Exec(txCtx, `DELETE FROM org.branches WHERE organization_id = $1`, org); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reset fixtures: %v", err)
	}
}

func TestTenantIsolation_Branches(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const orgA int64 = 8801
	const orgB int64 = 8802

	// Setup: create test orgs and user in system context
	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `
			INSERT INTO identity.users (id, email, password_hash, name)
			VALUES (88001, 'user88001@example.com', '$2y$10$xyz', '{"ar":"مستخدم","en":"User"}')
			ON CONFLICT (id) DO NOTHING;
		`)
		_, _ = tx.Exec(txCtx, `
			INSERT INTO org.organizations (id, name)
			VALUES ($1, '{"ar":"مؤسسة أ","en":"Org A"}')
			ON CONFLICT (id) DO NOTHING;
		`, orgA)
		_, _ = tx.Exec(txCtx, `
			INSERT INTO org.organizations (id, name)
			VALUES ($1, '{"ar":"مؤسسة ب","en":"Org B"}')
			ON CONFLICT (id) DO NOTHING;
		`, orgB)
		return nil
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	resetFixtures(t, db, orgA, orgB)

	// Step 1: Insert branch for Org A under Org A tenant context
	ctxA := database.WithTenant(ctx, orgA)
	var branchID int64
	err = db.InTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx, `
			INSERT INTO org.branches (organization_id, name, is_main)
			VALUES ($1, '{"ar":"فرع القاهرة","en":"Cairo Branch"}', true)
			RETURNING id;
		`, orgA)
		return row.Scan(&branchID)
	})
	if err != nil {
		t.Fatalf("failed to insert branch for org A: %v", err)
	}

	// Step 2: Query branches from Org B tenant context — MUST RETURN 0 ROWS
	ctxB := database.WithTenant(ctx, orgB)
	err = db.InReadTx(ctxB, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM org.branches WHERE id = $1", branchID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("SECURITY LEAK: Org B read Org A's branch! count = %d; want 0", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read as Org B failed: %v", err)
	}

	// Step 3: Query branches without any tenant context — MUST RETURN 0 ROWS
	err = db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM org.branches WHERE id = $1", branchID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("SECURITY LEAK: Unauthenticated/tenant-less query read branch! count = %d; want 0", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read without tenant failed: %v", err)
	}

	// Step 4: Query branches with AsSystem context — MUST BE VISIBLE
	err = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM org.branches WHERE id = $1", branchID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("System query failed to find branch: count = %d; want 1", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read as system failed: %v", err)
	}

	// Step 5: Attempt to insert row for Org A while authenticated as Org B — MUST FAIL RLS WITH CHECK
	err = db.InTx(ctxB, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			INSERT INTO org.branches (organization_id, name, is_main)
			VALUES ($1, '{"ar":"فرع خبيث","en":"Malicious Branch"}', false);
		`, orgA)
		return err
	})
	if err == nil {
		t.Errorf("SECURITY LEAK: Org B was able to insert a branch owned by Org A!")
	}
}

func TestTenantIsolation_Members(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const orgA int64 = 8901
	const orgB int64 = 8902

	// Setup: create test orgs and user in system context
	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `
			INSERT INTO identity.users (id, email, password_hash, name)
			VALUES (89001, 'user89001@example.com', '$2y$10$xyz', '{"ar":"مستخدم","en":"User"}')
			ON CONFLICT (id) DO NOTHING;
		`)
		_, _ = tx.Exec(txCtx, `
			INSERT INTO org.organizations (id, name)
			VALUES ($1, '{"ar":"مؤسسة 1","en":"Org 1"}')
			ON CONFLICT (id) DO NOTHING;
		`, orgA)
		_, _ = tx.Exec(txCtx, `
			INSERT INTO org.organizations (id, name)
			VALUES ($1, '{"ar":"مؤسسة 2","en":"Org 2"}')
			ON CONFLICT (id) DO NOTHING;
		`, orgB)
		return nil
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	resetFixtures(t, db, orgA, orgB)

	// Insert member in Org A tenant context
	ctxA := database.WithTenant(ctx, orgA)
	var memberID int64
	err = db.InTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx, `
			INSERT INTO org.members (organization_id, user_id, role_key)
			VALUES ($1, 89001, 'org_manager')
			RETURNING id;
		`, orgA)
		return row.Scan(&memberID)
	})
	if err != nil {
		t.Fatalf("failed to insert member for org A: %v", err)
	}

	// Query from Org B context — MUST RETURN 0 ROWS
	ctxB := database.WithTenant(ctx, orgB)
	err = db.InReadTx(ctxB, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM org.members WHERE id = $1", memberID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("SECURITY LEAK: Org B read Org A's members! count = %d; want 0", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read as Org B failed: %v", err)
	}

	// Query with AsSystem — MUST BE VISIBLE
	err = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM org.members WHERE id = $1", memberID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("System query failed to find member: count = %d; want 1", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read as system failed: %v", err)
	}
}

// TestTenantIsolation_CustomerProductMappings proves the dual-org policy on
// catalog.customer_product_mappings (071): the supplier (organization_id) and
// the customer (customer_org_id) may both read the row; any third tenant and
// any tenant-less query must see nothing; and writes are owner-only.
func TestTenantIsolation_CustomerProductMappings(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const orgA int64 = 8701 // supplier
	const orgB int64 = 8702 // customer
	const orgC int64 = 8703 // unrelated tenant

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		for _, org := range []int64{orgA, orgB, orgC} {
			if _, err := tx.Exec(txCtx, `
				INSERT INTO org.organizations (id, name)
				VALUES ($1, '{"ar":"مؤسسة فصل","en":"Mapping Org"}')
				ON CONFLICT (id) DO NOTHING;`, org); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Cleanup leftovers from a previous run.
	err = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `DELETE FROM catalog.customer_product_mappings WHERE organization_id IN ($1,$2,$3) OR customer_org_id IN ($1,$2,$3)`, orgA, orgB, orgC); err != nil {
			return err
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM catalog.products WHERE organization_id = $1`, orgA); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reset fixtures: %v", err)
	}

	ctxA := database.WithTenant(ctx, orgA)
	var productID int64
	var mappingID int64
	err = db.InTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx, `
			INSERT INTO catalog.products (organization_id, name, status)
			VALUES ($1, '{"ar":"منتج خصم","en":"Priced Product"}', 'active')
			RETURNING id;`, orgA).Scan(&productID); err != nil {
			return err
		}
		// Supplier A prices a product for customer B.
		return tx.QueryRow(txCtx, `
			INSERT INTO catalog.customer_product_mappings (organization_id, customer_org_id, product_id, price, source, status, is_active)
			VALUES ($1, $2, $3, 45.00, 'manual', 'processed', true)
			RETURNING id;`, orgA, orgB, productID).Scan(&mappingID)
	})
	if err != nil {
		t.Fatalf("insert mapping as org A failed: %v", err)
	}

	// 1. Customer B may read the row (customer_org_id side).
	ctxB := database.WithTenant(ctx, orgB)
	err = db.InReadTx(ctxB, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM catalog.customer_product_mappings WHERE id = $1", mappingID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("Customer B should read supplier A's pricing row for it; count = %d; want 1", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read as customer B failed: %v", err)
	}

	// 2. Unrelated tenant C MUST NOT see the row.
	ctxC := database.WithTenant(ctx, orgC)
	err = db.InReadTx(ctxC, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM catalog.customer_product_mappings WHERE id = $1", mappingID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("SECURITY LEAK: Org C read supplier A's pricing row! count = %d; want 0", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read as org C failed: %v", err)
	}

	// 3. Tenant-less query MUST NOT see the row.
	err = db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM catalog.customer_product_mappings WHERE id = $1", mappingID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("SECURITY LEAK: tenant-less query read the pricing row! count = %d; want 0", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tenant-less read failed: %v", err)
	}

	// 4. System sees it.
	err = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM catalog.customer_product_mappings WHERE id = $1", mappingID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("System query failed to find the mapping: count = %d; want 1", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("system read failed: %v", err)
	}

	// 5. Org C must not be able to write a row owned by org A.
	err = db.InTx(ctxC, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			INSERT INTO catalog.customer_product_mappings (organization_id, customer_org_id, product_id, price, source, status, is_active)
			VALUES ($1, $2, $3, 40.00, 'manual', 'processed', true);`, orgA, orgB, productID)
		return err
	})
	if err == nil {
		t.Errorf("SECURITY LEAK: Org C inserted a pricing row owned by Org A!")
	}
}
