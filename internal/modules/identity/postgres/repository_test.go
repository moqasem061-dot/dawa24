package postgres_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/identity/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func getTestDB(t *testing.T) *database.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database integration test in short mode")
	}
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

	migrations, err := database.LoadMigrations(dbfs.Migrations, "migrations")
	if err != nil {
		t.Fatalf("failed to load migrations: %v", err)
	}

	pending, err := db.PendingCount(ctx, migrations)
	if err != nil {
		t.Fatalf("cannot read migration state: %v", err)
	}
	if pending > 0 {
		t.Fatalf("%d migrations pending", pending)
	}
	return db
}

const (
	testOrgID     int64 = 88690
	testProductID int64 = 88691
)

func resetFixtures(t *testing.T, db *database.DB) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `DELETE FROM identity.user_favorites WHERE user_id IN (SELECT id FROM identity.users WHERE email LIKE 'test-identity-%@example.com')`); err != nil {
			return fmt.Errorf("delete user_favorites: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM identity.user_addresses WHERE user_id IN (SELECT id FROM identity.users WHERE email LIKE 'test-identity-%@example.com')`); err != nil {
			return fmt.Errorf("delete user_addresses: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM identity.user_mfa WHERE user_id IN (SELECT id FROM identity.users WHERE email LIKE 'test-identity-%@example.com')`); err != nil {
			return fmt.Errorf("delete user_mfa: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM identity.user_security WHERE user_id IN (SELECT id FROM identity.users WHERE email LIKE 'test-identity-%@example.com')`); err != nil {
			return fmt.Errorf("delete user_security: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM org.members WHERE organization_id = $1`, testOrgID); err != nil {
			return fmt.Errorf("delete org.members: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM identity.users WHERE email LIKE 'test-identity-%@example.com'`); err != nil {
			return fmt.Errorf("delete identity.users: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM catalog.products WHERE id = $1`, testProductID); err != nil {
			return fmt.Errorf("delete catalog.products: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM org.organizations WHERE id = $1`, testOrgID); err != nil {
			return fmt.Errorf("delete org.organizations: %w", err)
		}

		// user_addresses.city_id is a real foreign key, so the address subtest
		// needs a city (and the country it hangs off) to exist.
		if _, err := tx.Exec(txCtx, `INSERT INTO platform_admin.countries (id, code, name, phone_code) VALUES (1, 'EG', '{"ar":"مصر","en":"Egypt"}'::jsonb, '+20') ON CONFLICT (id) DO NOTHING`); err != nil {
			return fmt.Errorf("insert country: %w", err)
		}
		if _, err := tx.Exec(txCtx, `INSERT INTO platform_admin.cities (id, country_id, name) VALUES (1, 1, '{"ar":"القاهرة","en":"Cairo"}'::jsonb) ON CONFLICT (id) DO NOTHING`); err != nil {
			return fmt.Errorf("insert city: %w", err)
		}

		// Create org and product for membership and favorite checks
		if _, err := tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"ar":"مؤسسة الفحص","en":"Identity Test Org"}'::jsonb) ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`, testOrgID); err != nil {
			return fmt.Errorf("insert org: %w", err)
		}
		if _, err := tx.Exec(txCtx, `INSERT INTO catalog.products (id, organization_id, name, dosage_form) VALUES ($1, $2, '{"ar":"دواء الفحص","en":"Identity Test Prod"}'::jsonb, 'tablet') ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`, testProductID, testOrgID); err != nil {
			return fmt.Errorf("insert product: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reset fixtures: %v", err)
	}
}

func TestIdentityRepository(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	resetFixtures(t, db)

	repo := postgres.NewRepository(db)
	ctx := context.Background()

	var userID int64

	t.Run("Create and Get User", func(t *testing.T) {
		u := &identity.User{
			Email:        "test-identity-1@example.com",
			PasswordHash: "hash123",
			Status:       identity.StatusActive,
			Language:     "en",
			Role:         "user",
			Timezone:     "Africa/Cairo",
			Phone:        "+201000000001",
		}
		if err := repo.CreateUser(ctx, u); err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		if u.ID == 0 {
			t.Fatal("expected generated user ID")
		}
		userID = u.ID

		fetched, err := repo.GetUserByID(ctx, userID)
		if err != nil {
			t.Fatalf("failed to get user: %v", err)
		}
		if fetched.Email != u.Email {
			t.Errorf("got %q, want %q", fetched.Email, u.Email)
		}

		byEmail, err := repo.GetUserByEmail(ctx, u.Email)
		if err != nil {
			t.Fatalf("failed to get user by email: %v", err)
		}
		if byEmail.ID != userID {
			t.Errorf("got %d, want %d", byEmail.ID, userID)
		}

		u.Phone = "+201000000002"
		if err := repo.UpdateUser(ctx, u); err != nil {
			t.Fatalf("failed to update user: %v", err)
		}
	})

	t.Run("Security and MFA", func(t *testing.T) {
		maxSessions := 5
		sec := &identity.UserSecurity{
			UserID:              userID,
			LoginAttempts:       2,
			LastUserAgent:       "TestAgent",
			MaxLoginSessions:    &maxSessions,
			PasswordChangeCount: 1,
		}
		if err := repo.UpsertSecurity(ctx, sec); err != nil {
			t.Fatalf("failed to upsert security: %v", err)
		}

		gotSec, err := repo.GetSecurity(ctx, userID)
		if err != nil {
			t.Fatalf("failed to get security: %v", err)
		}
		if gotSec.LoginAttempts != 2 {
			t.Errorf("got %d login attempts, want 2", gotSec.LoginAttempts)
		}

		mfaConfirmedAt := time.Now().UTC()
		mfa := &identity.UserMFA{
			UserID:        userID,
			TOTPSecret:    []byte("SECRETKEY"),
			RecoveryCodes: []byte(`["REC1","REC2"]`),
			Enabled:       true,
			// mfa_enabled_requires_secret refuses an enabled record with no
			// confirmation, which is correct: MFA is not on until it is proven.
			ConfirmedAt: &mfaConfirmedAt,
		}
		if err := repo.UpsertMFA(ctx, mfa); err != nil {
			t.Fatalf("failed to upsert mfa: %v", err)
		}

		gotMFA, err := repo.GetMFA(ctx, userID)
		if err != nil {
			t.Fatalf("failed to get mfa: %v", err)
		}
		if !gotMFA.Enabled {
			t.Error("expected MFA to be enabled")
		}
	})

	t.Run("Roles and Permissions", func(t *testing.T) {
		roles, err := repo.GetRolesForUser(ctx, userID)
		if err != nil {
			t.Fatalf("failed to get roles: %v", err)
		}
		if len(roles) == 0 {
			t.Error("expected at least customer role")
		}

		perms, err := repo.GetPermissionsForUser(ctx, userID, testOrgID)
		if err != nil {
			t.Fatalf("failed to get permissions: %v", err)
		}
		_ = perms

		belongs, err := repo.UserBelongsToOrg(ctx, userID, testOrgID)
		if err != nil {
			t.Fatalf("failed to check org membership: %v", err)
		}
		if belongs {
			t.Error("user should not belong to org yet")
		}
	})

	t.Run("Addresses", func(t *testing.T) {
		addr := &identity.UserAddress{
			UserID:    userID,
			Title:     "Home",
			Recipient: "Test User",
			Phone:     "+201000000001",
			Address:   "123 Main St",
			CityID:    1,
			IsDefault: true,
		}
		if err := repo.CreateAddress(ctx, addr); err != nil {
			t.Fatalf("failed to create address: %v", err)
		}

		gotAddr, err := repo.GetAddressByID(ctx, addr.ID, userID)
		if err != nil {
			t.Fatalf("failed to get address: %v", err)
		}
		if gotAddr.Title != "Home" {
			t.Errorf("got %q, want Home", gotAddr.Title)
		}

		list, err := repo.ListAddresses(ctx, userID)
		if err != nil || len(list) == 0 {
			t.Fatalf("failed to list addresses: %v", err)
		}

		addr.Title = "Office"
		if err := repo.UpdateAddress(ctx, addr); err != nil {
			t.Fatalf("failed to update address: %v", err)
		}

		if err := repo.DeleteAddress(ctx, addr.ID, userID); err != nil {
			t.Fatalf("failed to delete address: %v", err)
		}
	})

	t.Run("Favorites", func(t *testing.T) {
		if err := repo.AddFavorite(ctx, userID, testProductID); err != nil {
			t.Fatalf("failed to add favorite: %v", err)
		}

		favs, err := repo.ListFavorites(ctx, userID)
		if err != nil {
			t.Fatalf("failed to list favorites: %v", err)
		}
		if len(favs) == 0 || favs[0] != testProductID {
			t.Errorf("expected favorite product %d, got %v", testProductID, favs)
		}

		if err := repo.RemoveFavorite(ctx, userID, testProductID); err != nil {
			t.Fatalf("failed to remove favorite: %v", err)
		}
	})

	t.Run("Admin Operations", func(t *testing.T) {
		users, err := repo.AdminListUsers(ctx, "customer", "active")
		if err != nil {
			t.Fatalf("AdminListUsers failed: %v", err)
		}
		_ = users

		if err := repo.AdminUpdateUserStatus(ctx, userID, "suspended", 1); err != nil {
			t.Fatalf("AdminUpdateUserStatus failed: %v", err)
		}

		if err := repo.AdminResetMFA(ctx, userID, 1); err != nil {
			t.Fatalf("AdminResetMFA failed: %v", err)
		}

		if err := repo.AdminAssignRole(ctx, userID, "vendor", 1); err != nil {
			t.Fatalf("AdminAssignRole failed: %v", err)
		}
	})
}

func TestRegisterOrganizationLive(t *testing.T) {
	db := getTestDB(t)
	repo := postgres.NewRepository(db)
	ctx := context.Background()

	lat := 30.0444
	lon := 31.2357
	branchCount := 1
	var cityID int64 = 1

	u := &identity.User{
		Email:        fmt.Sprintf("test-identity-reg-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuvwxyz123456",
		Name:         i18n.New("د. أحمد", "Dr. Ahmed"),
		Role:         "user",
		Status:       identity.StatusActive,
		Language:     i18n.Lang("ar"),
		Timezone:     "Africa/Cairo",
		Phone:        "01012345678",
	}

	orgIn := identity.RegisterOrgInput{
		Type:               "customer",
		LegalName:          "صيدلية الشفاء التجريبية",
		TradeNameAr:        "صيدلية الشفاء",
		TradeNameEn:        "Al Shefa Pharmacy",
		CommercialRegister: fmt.Sprintf("CR-%d", time.Now().UnixNano()),
		TaxNumber:          "123-456-789",
		PharmacistLicense:  "LIC-12345",
		CityID:             &cityID,
		BranchCount:        &branchCount,
		Address:            "شارع التحرير، الدقي، الجيزة",
		Latitude:           &lat,
		Longitude:          &lon,
		GoogleMapsURL:      "https://google.com/maps",
	}

	res, err := repo.RegisterOrganization(ctx, u, orgIn)
	if err != nil {
		t.Fatalf("RegisterOrganization failed: %+v", err)
	}
	t.Logf("Registered Org successfully: OrgID=%d, UserID=%d, Type=%s, Status=%s", res.OrganizationID, u.ID, res.OrganizationType, res.OrganizationStatus)
}
