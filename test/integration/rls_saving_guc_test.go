package integration_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// TestTenantIsolation_SavingProducts proves that catalog.saving_products is
// visible to its owner, invisible to another tenant, and visible to AsSystem.
// Before migration 254 the policy read app.current_tenant, which the
// application never sets, so even the owner saw zero rows.
func TestTenantIsolation_SavingProducts(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const orgA int64 = 8821
	const orgB int64 = 8822

	setupOrgs(ctx, t, db, orgA, orgB)

	ctxA := database.WithTenant(ctx, orgA)
	var rowID int64
	err := db.InTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO catalog.saving_products (organization_id, name_product, qty, price)
			VALUES ($1, 'isolation probe', 1, 10)
			RETURNING id;
		`, orgA).Scan(&rowID)
	})
	if err != nil {
		t.Fatalf("insert saving product as org A: %v", err)
	}

	countAs := func(name string, tctx context.Context, want int) {
		t.Helper()
		err := db.InReadTx(tctx, func(txCtx context.Context, tx pgx.Tx) error {
			var count int
			if err := tx.QueryRow(txCtx, `SELECT count(*) FROM catalog.saving_products WHERE id = $1`, rowID).Scan(&count); err != nil {
				return err
			}
			if count != want {
				t.Errorf("%s: saw %d rows, want %d", name, count, want)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("%s: read failed: %v", name, err)
		}
	}

	countAs("owner org A", ctxA, 1)
	countAs("other org B", database.WithTenant(ctx, orgB), 0)
	countAs("no tenant", ctx, 0)
	countAs("AsSystem", database.AsSystem(ctx), 1)

	// Writing into org A's rows while authenticated as org B must be refused.
	err = db.InTx(database.WithTenant(ctx, orgB), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `INSERT INTO catalog.saving_products (organization_id, name_product) VALUES ($1, 'intruder')`, orgA)
		return err
	})
	if err == nil {
		t.Errorf("SECURITY LEAK: org B inserted a saving product owned by org A")
	}
}

// TestTenantIsolation_EmployeeInstitutionalWorks is the same proof for
// org.employee_institutional_works, which had the same GUC mismatch.
func TestTenantIsolation_EmployeeInstitutionalWorks(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const orgA int64 = 8823
	const orgB int64 = 8824
	const userA int64 = 88231

	setupOrgs(ctx, t, db, orgA, orgB)
	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `
			INSERT INTO identity.users (id, email, password_hash, name)
			VALUES ($1, 'user88231@example.com', '$2y$10$xyz', '{"ar":"موظف","en":"Employee"}')
			ON CONFLICT (id) DO NOTHING;`, userA); err != nil {
			return err
		}
		_, err := tx.Exec(txCtx, `
			INSERT INTO org.institutional_works (id, slug)
			VALUES (88231, 'isolation-probe-88231')
			ON CONFLICT (id) DO NOTHING;`)
		return err
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	err = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `DELETE FROM org.employee_institutional_works WHERE organization_id = $1`, orgA)
		return err
	})
	if err != nil {
		t.Fatalf("reset: %v", err)
	}

	ctxA := database.WithTenant(ctx, orgA)
	var rowID int64
	err = db.InTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO org.employee_institutional_works (organization_id, user_id, institutional_work_id)
			VALUES ($1, $2, 88231)
			RETURNING id;`, orgA, userA).Scan(&rowID)
	})
	if err != nil {
		t.Fatalf("insert as org A: %v", err)
	}

	countAs := func(name string, tctx context.Context, want int) {
		t.Helper()
		err := db.InReadTx(tctx, func(txCtx context.Context, tx pgx.Tx) error {
			var count int
			if err := tx.QueryRow(txCtx, `SELECT count(*) FROM org.employee_institutional_works WHERE id = $1`, rowID).Scan(&count); err != nil {
				return err
			}
			if count != want {
				t.Errorf("%s: saw %d rows, want %d", name, count, want)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("%s: read failed: %v", name, err)
		}
	}

	countAs("owner org A", ctxA, 1)
	countAs("other org B", database.WithTenant(ctx, orgB), 0)
	countAs("no tenant", ctx, 0)
	countAs("AsSystem", database.AsSystem(ctx), 1)
}

// setupOrgs makes the two test organizations exist; the rows under test are
// created by each test and removed by the reset above.
func setupOrgs(ctx context.Context, t *testing.T, db *database.DB, orgs ...int64) {
	t.Helper()
	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		for _, id := range orgs {
			if _, err := tx.Exec(txCtx, `
				INSERT INTO org.organizations (id, name)
				VALUES ($1, '{"ar":"مؤسسة","en":"Org"}')
				ON CONFLICT (id) DO NOTHING;`, id); err != nil {
				return err
			}
		}
		_, err := tx.Exec(txCtx, `DELETE FROM catalog.saving_products WHERE organization_id = ANY($1)`, orgs)
		return err
	})
	if err != nil {
		t.Fatalf("setup orgs: %v", err)
	}
}
