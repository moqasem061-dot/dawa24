package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	catalogPostgres "github.com/muhiya/dawa24-store/internal/modules/catalog/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestCustomerPricingAndAlerts(t *testing.T) {
	db := getTestDB(t)
	repo := catalogPostgres.NewRepository(db)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// These ids were pinned without ever being created, so every insert failed
	// on products_organization_id_fkey. Create the rows the foreign keys need.
	orgID := int64(88901)
	customerOrgID := int64(88902)
	userID := int64(88903)
	seedPricingFixtures(t, db, orgID, customerOrgID, userID)
	ctx = database.WithTenant(ctx, orgID)

	prod := &catalog.Product{
		OrganizationID: orgID,
		Name:           i18n.New("بانادول أزرق تسعير", "Panadol Blue Pricing"),
		Status:         catalog.StatusActive,
	}
	if err := repo.CreateProduct(ctx, prod); err != nil {
		t.Fatalf("CreateProduct failed: %v", err)
	}
	defer func() { _ = repo.DeleteProduct(ctx, prod.ID) }()

	t.Run("CustomerPricing", func(t *testing.T) {
		m := &catalog.CustomerProductMapping{
			OrganizationID: orgID,
			CustomerOrgID:  &customerOrgID,
			ProductID:      prod.ID,
			Price:          money.FromMinor(800),
			IsActive:       true,
		}
		if err := repo.SetCustomerPricing(ctx, m); err != nil {
			t.Fatalf("SetCustomerPricing failed: %v", err)
		}

		got, err := repo.GetCustomerPricing(ctx, orgID, customerOrgID, prod.ID)
		if err != nil {
			t.Fatalf("GetCustomerPricing failed: %v", err)
		}
		if got.Price.Minor() != 800 {
			t.Errorf("money round-trip failed: got %v", got.Price)
		}
	})

	t.Run("ProductAlerts", func(t *testing.T) {
		a := &catalog.ProductAlert{
			UserID:      userID,
			ProductID:   prod.ID,
			AlertType:   "price_drop",
			TargetPrice: money.FromMinor(900),
		}
		if err := repo.CreateProductAlert(ctx, a); err != nil {
			t.Fatalf("CreateProductAlert failed: %v", err)
		}

		alerts, err := repo.ListProductAlertsByUser(ctx, userID)
		if err != nil {
			t.Fatalf("ListProductAlertsByUser failed: %v", err)
		}
		if len(alerts) == 0 {
			t.Error("expected at least 1 alert")
		}
	})
}

// seedPricingFixtures creates the organization, customer organization and user
// that this suite's foreign keys require.
func seedPricingFixtures(t *testing.T, db *database.DB, orgID, customerOrgID, userID int64) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		for _, id := range []int64{orgID, customerOrgID} {
			if _, err := tx.Exec(txCtx, `
				INSERT INTO org.organizations (id, name, legal_name)
				VALUES ($1, '{"ar":"مؤسسة التسعير","en":"Pricing Fixture Org"}'::jsonb, 'Pricing Fixture Org')
				ON CONFLICT (id) DO NOTHING;`, id); err != nil {
				return err
			}
		}
		_, err := tx.Exec(txCtx, `
			INSERT INTO identity.users (id, email, password_hash, name)
			VALUES ($1, 'pricing-fixture@dawa24.test', 'x', '{"ar":"مستخدم","en":"Pricing User"}'::jsonb)
			ON CONFLICT (id) DO NOTHING;`, userID)
		return err
	})
	if err != nil {
		t.Fatalf("seed pricing fixtures: %v", err)
	}
}
