package integration_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	orgPostgres "github.com/muhiya/dawa24-store/internal/modules/org/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// rbacTestDB opens the database for these tests.
//
// Deliberately not getTestDB: that helper skips when connected as a superuser,
// because the RLS suite it serves can prove nothing under one. These tests are
// about a different mechanism. The application connects to PostgreSQL as a
// superuser in every environment (see the note in platform/authctx), which is
// precisely why isolation here rests on the organization id in each WHERE
// clause rather than on row-level security — so a superuser connection is the
// realistic case to test, not a disqualifying one.
func rbacTestDB(t *testing.T) *database.DB {
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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := database.Open(ctx, config.Database{
		URL: dbURL, MaxConns: 5, MinConns: 1,
		MaxConnLifetime: time.Hour, MaxConnIdleTime: 30 * time.Minute,
		StatementTimeout: 30 * time.Second,
	})
	if err != nil {
		if os.Getenv("CI") == "true" {
			t.Fatalf("CI failed to connect to the database: %v", err)
		}
		t.Skipf("cannot connect to the database: %v; skipping", err)
		return nil
	}
	t.Cleanup(db.Close)
	return db
}

// Company isolation and revocation, proved against a real database.
//
// These are the two properties that cannot be checked by reading the code.
// Isolation depends on every query carrying an organization id, and the
// application connects to PostgreSQL as a superuser — so row-level security is
// inert and the WHERE clauses are the only boundary there is. Revocation
// depends on the version counter actually reaching a second process.

// rbacFixture builds two unrelated companies, each with an owner and a limited
// employee, and returns their ids. Everything is removed afterwards.
type rbacFixture struct {
	vendorOrg, pharmacyOrg   int64
	vendorOwner, vendorClerk int64
	pharmacyOwner            int64
	vendorClerkRole          int64
	pharmacyPrivilegedRole   int64
}

func setupRBACFixture(t *testing.T, db *database.DB) rbacFixture {
	t.Helper()
	ctx := context.Background()
	var f rbacFixture
	suffix := time.Now().UnixNano()

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		mkOrg := func(name, orgType string) (int64, error) {
			var id int64
			err := tx.QueryRow(txCtx, `
				INSERT INTO org.organizations (name, legal_name, trade_name, type, status)
				VALUES (jsonb_build_object('ar',$1::text,'en',$1::text), $1,
				        jsonb_build_object('ar',$1::text,'en',$1::text), $2, 'approved')
				RETURNING id;`, name, orgType).Scan(&id)
			return id, err
		}
		mkUser := func(email string) (int64, error) {
			var id int64
			err := tx.QueryRow(txCtx, `
				INSERT INTO identity.users (name, email, password_hash, role, status)
				VALUES ('{"ar":"اختبار","en":"Test"}'::jsonb, $1, 'x', 'user', 'active')
				RETURNING id;`, email).Scan(&id)
			return id, err
		}
		mkMember := func(orgID, userID int64, roleKey string) error {
			_, err := tx.Exec(txCtx, `
				INSERT INTO org.members (organization_id, user_id, role_key, status, is_active)
				VALUES ($1, $2, $3, 'active', true);`, orgID, userID, roleKey)
			return err
		}

		var err error
		if f.vendorOrg, err = mkOrg(fmt.Sprintf("rbac_vendor_%d", suffix), "vendor"); err != nil {
			return err
		}
		if f.pharmacyOrg, err = mkOrg(fmt.Sprintf("rbac_pharmacy_%d", suffix), "customer"); err != nil {
			return err
		}
		if f.vendorOwner, err = mkUser(fmt.Sprintf("rbac_vowner_%d@dawa24.test", suffix)); err != nil {
			return err
		}
		if f.vendorClerk, err = mkUser(fmt.Sprintf("rbac_vclerk_%d@dawa24.test", suffix)); err != nil {
			return err
		}
		if f.pharmacyOwner, err = mkUser(fmt.Sprintf("rbac_powner_%d@dawa24.test", suffix)); err != nil {
			return err
		}
		if err := mkMember(f.vendorOrg, f.vendorOwner, "org_owner"); err != nil {
			return err
		}
		if err := mkMember(f.vendorOrg, f.vendorClerk, "org_warehouse"); err != nil {
			return err
		}
		return mkMember(f.pharmacyOrg, f.pharmacyOwner, "org_owner")
	})
	if err != nil {
		t.Fatalf("seeding the RBAC fixture: %v", err)
	}

	if err := rbac.EnsureCompanyRoles(ctx, db, f.vendorOrg, "vendor"); err != nil {
		t.Fatalf("seeding vendor roles: %v", err)
	}
	if err := rbac.EnsureCompanyRoles(ctx, db, f.pharmacyOrg, "customer"); err != nil {
		t.Fatalf("seeding pharmacy roles: %v", err)
	}

	_ = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_ = tx.QueryRow(txCtx, `SELECT id FROM org.roles WHERE organization_id=$1 AND key='org_warehouse';`,
			f.vendorOrg).Scan(&f.vendorClerkRole)
		_ = tx.QueryRow(txCtx, `SELECT id FROM org.roles WHERE organization_id=$1 AND key='org_manager';`,
			f.pharmacyOrg).Scan(&f.pharmacyPrivilegedRole)
		return nil
	})

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, `DELETE FROM org.organizations WHERE id = ANY($1::bigint[]);`,
				[]int64{f.vendorOrg, f.pharmacyOrg})
			_, _ = tx.Exec(txCtx, `DELETE FROM identity.users WHERE id = ANY($1::bigint[]);`,
				[]int64{f.vendorOwner, f.vendorClerk, f.pharmacyOwner})
			return nil
		})
	})
	return f
}

// TestRolesAreIsolatedBetweenCompanies.
//
// The repository used to read `WHERE id = $1` under database.AsSystem, with no
// organization in the clause at all. Any signed-in user who guessed a role id
// could read another company's role, rewrite its permissions, or delete it.
func TestRolesAreIsolatedBetweenCompanies(t *testing.T) {
	db := rbacTestDB(t)
	if db == nil {
		return
	}
	ctx := context.Background()
	f := setupRBACFixture(t, db)
	repo := orgRoleRepo(db)

	// The pharmacy's role, addressed from the vendor's company, must not exist.
	if _, err := repo.GetRole(ctx, f.vendorOrg, f.pharmacyPrivilegedRole); err == nil {
		t.Error("a vendor read a pharmacy's role by id")
	}
	if err := repo.DeleteRole(ctx, f.vendorOrg, f.pharmacyPrivilegedRole); err == nil {
		t.Error("a vendor deleted a pharmacy's role by id")
	}
	if err := repo.AssignMemberRole(ctx, f.vendorOrg, f.vendorClerk, f.pharmacyPrivilegedRole); err == nil {
		t.Error("a vendor assigned their employee a role belonging to a pharmacy")
	}

	// And the pharmacy's role is still intact and still the pharmacy's.
	role, err := repo.GetRole(ctx, f.pharmacyOrg, f.pharmacyPrivilegedRole)
	if err != nil {
		t.Fatalf("the owning company could not read its own role: %v", err)
	}
	if role.OrganizationID != f.pharmacyOrg {
		t.Errorf("role belongs to organization %d, want %d", role.OrganizationID, f.pharmacyOrg)
	}
	if len(role.Permissions) == 0 {
		t.Error("the seeded manager role holds no permissions")
	}
	for _, p := range role.Permissions {
		if pd, ok := rbac.Default().Lookup(p); ok && !pd.InScope(rbac.ScopePharmacy) {
			t.Errorf("a pharmacy role holds %q, which is not a pharmacy permission", p)
		}
	}

	// Every role in each list belongs to the company that asked.
	for _, tc := range []struct {
		orgID int64
		scope rbac.Scope
	}{{f.vendorOrg, rbac.ScopeVendor}, {f.pharmacyOrg, rbac.ScopePharmacy}} {
		list, err := repo.ListRoles(ctx, tc.orgID)
		if err != nil {
			t.Fatalf("listing roles for %d: %v", tc.orgID, err)
		}
		if len(list) == 0 {
			t.Fatalf("organization %d has no seeded roles", tc.orgID)
		}
		for _, r := range list {
			if r.OrganizationID != tc.orgID {
				t.Errorf("ListRoles(%d) returned a role owned by %d", tc.orgID, r.OrganizationID)
			}
		}
	}
}

// TestResolvedPermissionsAreScopedToTheCompanyDashboard.
func TestResolvedPermissionsAreScopedToTheCompanyDashboard(t *testing.T) {
	db := rbacTestDB(t)
	if db == nil {
		return
	}
	ctx := context.Background()
	f := setupRBACFixture(t, db)
	res := rbac.NewResolver(db)

	owner, err := res.Resolve(ctx, f.vendorOwner, f.vendorOrg)
	if err != nil {
		t.Fatalf("resolving the vendor owner: %v", err)
	}
	if !owner.IsOrgOwner || owner.Scope != rbac.ScopeVendor {
		t.Fatalf("vendor owner resolved as owner=%v scope=%q", owner.IsOrgOwner, owner.Scope)
	}
	for _, want := range []string{"vendor.wallet.manage", "vendor.role.update", "vendor.order.view"} {
		if !owner.Can(want) {
			t.Errorf("the vendor owner does not hold %q", want)
		}
	}
	// Nothing from another dashboard, however the company is addressed.
	for _, forbidden := range []string{"platform.setting.update", "identity.user.delete", "pharmacy.order.create"} {
		if owner.Can(forbidden) {
			t.Errorf("the vendor owner holds %q, which belongs to another dashboard", forbidden)
		}
	}

	// A limited employee holds their role and nothing more.
	clerk, err := res.Resolve(ctx, f.vendorClerk, f.vendorOrg)
	if err != nil {
		t.Fatalf("resolving the vendor clerk: %v", err)
	}
	if clerk.IsOrgOwner {
		t.Error("a warehouse clerk resolved as the company owner")
	}
	if !clerk.Can("vendor.inventory.view") {
		t.Error("the warehouse clerk cannot see the inventory their role is for")
	}
	for _, forbidden := range []string{"vendor.wallet.manage", "vendor.role.update", "vendor.team.delete"} {
		if clerk.Can(forbidden) {
			t.Errorf("a warehouse clerk holds %q", forbidden)
		}
	}

	// Asking about a company they do not belong to yields nothing at all.
	stranger, err := res.Resolve(ctx, f.vendorOwner, f.pharmacyOrg)
	if err != nil {
		t.Fatalf("resolving across companies: %v", err)
	}
	if len(stranger.Keys) != 0 {
		t.Errorf("a vendor owner resolved %d permissions inside a pharmacy they do not belong to",
			len(stranger.Keys))
	}
}

// TestRevocationIsVisibleToAnotherProcess.
//
// Permissions used to be computed once, at login, and copied into the Redis
// session. Sessions last 720 hours, so revoking a role changed nothing for
// anyone already signed in. Two resolvers here stand in for two application
// instances: the one that did not perform the write must still see it.
func TestRevocationIsVisibleToAnotherProcess(t *testing.T) {
	db := rbacTestDB(t)
	if db == nil {
		return
	}
	ctx := context.Background()
	f := setupRBACFixture(t, db)

	writer := rbac.NewResolver(db)
	reader := rbac.NewResolver(db) // a second process, with its own cache

	before, err := reader.Resolve(ctx, f.vendorClerk, f.vendorOrg)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if !before.Can("vendor.inventory.view") {
		t.Fatal("the clerk did not start with the permission this test revokes")
	}

	repo := orgRoleRepo(db)
	role, err := repo.GetRole(ctx, f.vendorOrg, f.vendorClerkRole)
	if err != nil {
		t.Fatalf("reading the clerk role: %v", err)
	}
	role.Permissions = []string{"vendor.dashboard.view"}
	if err := repo.UpdateRole(ctx, f.vendorOrg, role); err != nil {
		t.Fatalf("revoking: %v", err)
	}
	_ = writer // the write went through the repository, not this resolver

	// The version counter is cached briefly, so a revocation becomes visible
	// within that window rather than instantly. Waiting for it is the point:
	// the guarantee is "seconds", not "next login".
	deadline := time.Now().Add(20 * time.Second)
	for {
		after, err := reader.Resolve(ctx, f.vendorClerk, f.vendorOrg)
		if err != nil {
			t.Fatalf("re-resolve: %v", err)
		}
		if !after.Can("vendor.inventory.view") {
			if !after.Can("vendor.dashboard.view") {
				t.Error("the revocation also removed a permission the role still grants")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("a revoked permission was still held 20s later by a process that did not perform the write")
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// orgRoleRepo builds the repository under test. It is a helper so the three
// tests above read as scenarios rather than as wiring.
func orgRoleRepo(db *database.DB) *orgPostgres.Repository {
	return orgPostgres.NewRepository(db)
}
