package postgres_test

import (
	"context"
	"fmt"
	"os"

	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/catalog/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
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

	// Deliberately no superuser skip.
	//
	// The RLS suite skips for a superuser because a superuser bypasses
	// row-level security, so it cannot prove isolation either way. That
	// reasoning does not transfer here: these tests check that the SQL is
	// correct -- columns exist, types scan, money round-trips -- and a
	// superuser answers those questions perfectly well. Copying the skip meant
	// these tests reported `ok` while executing nothing, which is the exact
	// failure mode they were written to prevent.

	pending, err := db.PendingCount(ctx, migrations)
	if err != nil {
		t.Fatalf("cannot read migration state: %v", err)
	}
	if pending > 0 {
		t.Fatalf("%d migrations pending", pending)
	}
	return db
}

// Fixture ids sit far above anything the application will generate, so a
// failed run leaves them behind for inspection and the next run clears them.
const (
	testCustomerOrgID int64 = 88190
	testUserID        int64 = 88191
)

func resetFixtures(t *testing.T, db *database.DB, orgID int64) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		// Clean catalog tables
		if _, err := tx.Exec(txCtx, `DELETE FROM catalog.product_alerts WHERE product_id IN (SELECT id FROM catalog.products WHERE organization_id = $1)`, orgID); err != nil {
			return fmt.Errorf("delete product_alerts: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM catalog.customer_product_mappings WHERE organization_id = $1`, orgID); err != nil {
			return fmt.Errorf("delete customer_product_mappings: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM catalog.product_variants WHERE organization_id = $1`, orgID); err != nil {
			return fmt.Errorf("delete product_variants: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM catalog.products WHERE organization_id = $1`, orgID); err != nil {
			return fmt.Errorf("delete products: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM catalog.categories WHERE id >= 88100 AND id <= 88199`); err != nil {
			return fmt.Errorf("delete categories: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM catalog.brands WHERE id >= 88100 AND id <= 88199`); err != nil {
			return fmt.Errorf("delete brands: %w", err)
		}
		if _, err := tx.Exec(txCtx,
			`INSERT INTO org.organizations (id, name) VALUES ($1, '{"ar":"مؤسسة الفهرس","en":"Catalog Test Org"}'::jsonb)
			 ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`, orgID); err != nil {
			return fmt.Errorf("insert org: %w", err)
		}
		if _, err := tx.Exec(txCtx,
			`INSERT INTO org.organizations (id, name) VALUES ($1, '{"ar":"عميل الفهرس","en":"Catalog Test Customer"}'::jsonb)
			 ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`, testCustomerOrgID); err != nil {
			return fmt.Errorf("insert cust org: %w", err)
		}
		if _, err := tx.Exec(txCtx,
			`INSERT INTO identity.users (id, email, password_hash, name)
			 VALUES ($1, 'catalog-fixture@dawa24.test', '$2y$10$fixture', '{"ar":"مستخدم","en":"Catalog Fixture"}'::jsonb)
			 ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email`, testUserID); err != nil {
			return fmt.Errorf("insert user: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reset fixtures failed: %v", err)
	}
}

func TestCatalogRepository(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	const orgID int64 = 88101
	resetFixtures(t, db, orgID)

	ctx := database.WithTenant(context.Background(), orgID)
	repo := postgres.NewRepository(db)

	// Ids are assigned by PostgreSQL and scanned back by the repository, so
	// they cannot be pinned in the test. Subtests share what was actually
	// created. The original version hardcoded 88100+ and then looked those ids
	// up, which found nothing and cascaded into foreign-key failures on every
	// later subtest.
	var (
		categoryID int64
		brandID    int64
		productID  int64
		variantID  int64
	)

	t.Run("Categories", func(t *testing.T) {
		cat := &catalog.Category{
			Name:   i18n.Text{"en": "Test Category"},
			Status: "active",
		}
		err := repo.CreateCategory(ctx, cat)
		if err != nil {
			t.Fatalf("CreateCategory failed: %v", err)
		}
		if cat.ID == 0 {
			t.Fatal("CreateCategory did not return a generated id")
		}
		categoryID = cat.ID

		got, err := repo.GetCategoryByID(ctx, categoryID)
		if err != nil {
			t.Fatalf("GetCategoryByID failed: %v", err)
		}
		if got.Name["en"] != "Test Category" {
			t.Errorf("expected name 'Test Category', got %q", got.Name["en"])
		}

		cat.Name = i18n.Text{"en": "Updated Category"}
		err = repo.UpdateCategory(ctx, cat)
		if err != nil {
			t.Fatalf("UpdateCategory failed: %v", err)
		}

		list, err := repo.ListCategories(ctx)
		if err != nil {
			t.Fatalf("ListCategories failed: %v", err)
		}
		if len(list) == 0 {
			t.Error("expected at least 1 category in list")
		}

		count, err := repo.CountProductsInCategory(ctx, categoryID)
		if err != nil {
			t.Fatalf("CountProductsInCategory failed: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 products, got %d", count)
		}

		// Keep it around for product tests
	})

	t.Run("Brands", func(t *testing.T) {
		brand := &catalog.Brand{
			Name:   i18n.Text{"en": "Test Brand"},
			Status: "active",
		}
		err := repo.CreateBrand(ctx, brand)
		if err != nil {
			t.Fatalf("CreateBrand failed: %v", err)
		}

		if brand.ID == 0 {
			t.Fatal("CreateBrand did not return a generated id")
		}
		brandID = brand.ID

		got, err := repo.GetBrandByID(ctx, brandID)
		if err != nil {
			t.Fatalf("GetBrandByID failed: %v", err)
		}
		if got.Name["en"] != "Test Brand" {
			t.Errorf("expected name 'Test Brand', got %q", got.Name["en"])
		}

		brand.Name = i18n.Text{"en": "Updated Brand"}
		err = repo.UpdateBrand(ctx, brand)
		if err != nil {
			t.Fatalf("UpdateBrand failed: %v", err)
		}

		list, err := repo.ListBrands(ctx)
		if err != nil {
			t.Fatalf("ListBrands failed: %v", err)
		}
		if len(list) == 0 {
			t.Error("expected at least 1 brand in list")
		}

		count, err := repo.CountProductsInBrand(ctx, brandID)
		if err != nil {
			t.Fatalf("CountProductsInBrand failed: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 products, got %d", count)
		}
	})

	t.Run("Products", func(t *testing.T) {
		catID := categoryID
		p := &catalog.Product{
			OrganizationID: orgID,
			CategoryID:     &catID,
			BrandID:        &brandID,
			Name:           i18n.Text{"en": "Test Product"},
			Price:          money.FromMinor(1050), // 10.50
			Status:         catalog.StatusActive,
			Description:    i18n.Text{}, // Nullable equivalent
		}
		err := repo.CreateProduct(ctx, p)
		if err != nil {
			t.Fatalf("CreateProduct failed: %v", err)
		}

		productID = p.ID

		got, err := repo.GetProductByID(ctx, productID)
		if err != nil {
			t.Fatalf("GetProductByID failed: %v", err)
		}
		if got.Price.Minor() != 1050 {
			t.Errorf("money round-trip failed: got %v", got.Price)
		}
		if got.CategoryID == nil || *got.CategoryID != catID {
			t.Errorf("nullable column read failed")
		}

		p.Name = i18n.Text{"en": "Updated Product"}
		err = repo.UpdateProduct(ctx, p)
		if err != nil {
			t.Fatalf("UpdateProduct failed: %v", err)
		}

		list, err := repo.ListProducts(ctx, string(catalog.StatusActive), 10, 0)
		if err != nil {
			t.Fatalf("ListProducts failed: %v", err)
		}
		if len(list) == 0 {
			t.Error("expected at least 1 product")
		}

		orgIDPtr := orgID
		search, err := repo.SearchProducts(ctx, catalog.SearchParams{
			OrganizationID: &orgIDPtr,
			Limit:          10,
		})
		if err != nil {
			t.Fatalf("SearchProducts failed: %v", err)
		}
		if len(search) == 0 {
			t.Error("expected at least 1 product in search")
		}

		updated, err := repo.SetProductsStatus(ctx, []int64{productID}, catalog.StatusInactive)
		if err != nil {
			t.Fatalf("SetProductsStatus failed: %v", err)
		}
		if updated != 1 {
			t.Errorf("expected 1 row updated, got %d", updated)
		}
	})

	t.Run("Variants", func(t *testing.T) {
		costP := money.FromMinor(300)
		v := &catalog.ProductVariant{
			OrganizationID: orgID,
			ProductID:      productID,
			Name:           i18n.Text{"en": "Variant 1"},
			Price:          money.FromMinor(500),
			CostPrice:      &costP,
			Status:         catalog.StatusActive,
		}
		err := repo.CreateVariant(ctx, v)
		if err != nil {
			t.Fatalf("CreateVariant failed: %v", err)
		}

		variantID = v.ID

		got, err := repo.GetVariantByID(ctx, variantID)
		if err != nil {
			t.Fatalf("GetVariantByID failed: %v", err)
		}
		if got.Price.Minor() != 500 {
			t.Errorf("money round-trip failed: got %v", got.Price)
		}

		v.Name = i18n.Text{"en": "Updated Variant"}
		err = repo.UpdateVariant(ctx, v)
		if err != nil {
			t.Fatalf("UpdateVariant failed: %v", err)
		}

		variants, err := repo.ListVariantsByProduct(ctx, productID)
		if err != nil {
			t.Fatalf("ListVariantsByProduct failed: %v", err)
		}
		if len(variants) == 0 {
			t.Error("expected at least 1 variant")
		}

		err = repo.DeleteVariant(ctx, variantID)
		if err != nil {
			t.Fatalf("DeleteVariant failed: %v", err)
		}
	})

	t.Run("Cleanup", func(t *testing.T) {
		err := repo.DeleteProduct(ctx, productID)
		if err != nil {
			t.Fatalf("DeleteProduct failed: %v", err)
		}
		err = repo.DeleteCategory(ctx, categoryID)
		if err != nil {
			t.Fatalf("DeleteCategory failed: %v", err)
		}
		err = repo.DeleteBrand(ctx, brandID)
		if err != nil {
			t.Fatalf("DeleteBrand failed: %v", err)
		}
	})
}
