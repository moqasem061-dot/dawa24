package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/modules/identity/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// Batch 3 (S08): a deactivated membership (is_active = false) grants no
// organization at sign-in or on a switch, even though its status is still active.
func TestDeactivatedMembershipGrantsNoOrganization(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	ctx := context.Background()
	repo := postgres.NewRepository(db)

	const orgID int64 = 88311
	user := &identity.User{
		Email: "test-identity-deactivated@example.com", PasswordHash: "x",
		Name: i18n.New("عضو", "Member"), Role: "user", Status: identity.StatusActive, Language: i18n.Lang("ar"), Timezone: "Africa/Cairo",
	}
	_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `DELETE FROM org.members WHERE organization_id = $1`, orgID)
		_, _ = tx.Exec(txCtx, `DELETE FROM identity.users WHERE email = $1`, user.Email)
		_, _ = tx.Exec(txCtx, `DELETE FROM org.organizations WHERE id = $1`, orgID)
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"ar":"فحص","en":"Deactivated Check"}'::jsonb)`, orgID)
		return nil
	})
	if err := repo.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	setActive := func(active bool) {
		t.Helper()
		if err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(txCtx, `
				INSERT INTO org.members (organization_id, user_id, role_key, status, is_active)
				VALUES ($1, $2, 'org_employee', 'active', $3)
				ON CONFLICT (organization_id, user_id) DO UPDATE SET is_active = EXCLUDED.is_active, status = 'active';`,
				orgID, user.ID, active)
			return err
		}); err != nil {
			t.Fatalf("set membership active=%v: %v", active, err)
		}
	}

	setActive(false)
	belongs, err := repo.UserBelongsToOrg(ctx, user.ID, orgID)
	if err != nil {
		t.Fatalf("UserBelongsToOrg: %v", err)
	}
	if belongs {
		t.Fatal("deactivated member still belongs to the organization")
	}
	defOrg, _, _, err := repo.DefaultOrgInfoForUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("DefaultOrgInfoForUser: %v", err)
	}
	if defOrg == orgID {
		t.Fatal("deactivated member's organization is still the default")
	}

	setActive(true)
	belongs, err = repo.UserBelongsToOrg(ctx, user.ID, orgID)
	if err != nil || !belongs {
		t.Fatalf("active member: belongs=%v err=%v, want true", belongs, err)
	}
}
