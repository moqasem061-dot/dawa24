package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
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
	testOrgID int64 = 88700
)

func resetFixtures(t *testing.T, db *database.DB) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `DELETE FROM promo.offer_sponsorships WHERE organization_id = $1`, testOrgID); err != nil {
			return fmt.Errorf("delete offer_sponsorships: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM promo.offer_products WHERE offer_id IN (SELECT id FROM promo.offers WHERE organization_id = $1)`, testOrgID); err != nil {
			return fmt.Errorf("delete offer_products: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM promo.offers WHERE organization_id = $1`, testOrgID); err != nil {
			return fmt.Errorf("delete offers: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM promo.offer_packages WHERE name->>'en' = 'Promo Test Package'`); err != nil {
			return fmt.Errorf("delete offer_packages: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM promo.ads WHERE title = 'Test Ad'`); err != nil {
			return fmt.Errorf("delete ads: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM promo.highlight_sections WHERE slug = 'test-curated'`); err != nil {
			return fmt.Errorf("delete highlight_sections: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM org.organizations WHERE id = $1`, testOrgID); err != nil {
			return fmt.Errorf("delete org: %w", err)
		}

		if _, err := tx.Exec(txCtx,
			`INSERT INTO org.organizations (id, name, type, status) VALUES ($1, '{"ar":"مؤسسة العروض","en":"Promo Test Org"}'::jsonb, 'vendor', 'approved')
			 ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, type = EXCLUDED.type, status = EXCLUDED.status`, testOrgID); err != nil {
			return fmt.Errorf("insert org: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("resetFixtures: %v", err)
	}
}

func TestPromoRepository(t *testing.T) {
	db := getTestDB(t)
	resetFixtures(t, db)

	repo := NewRepository(db)
	ctx := database.WithTenant(context.Background(), testOrgID)

	var offerID int64
	var packageID int64

	t.Run("Create and Get Package", func(t *testing.T) {
		pkgPrice, _ := money.Parse("199.99")
		pkg := &promo.OfferPackage{
			Name:         i18n.Text{"en": "Promo Test Package"},
			Price:        pkgPrice,
			DurationDays: 30,
			MaxOffers:    5,
			IsActive:     true,
		}

		if err := repo.CreatePackage(ctx, pkg); err != nil {
			t.Fatalf("CreatePackage failed: %v", err)
		}
		if pkg.ID <= 0 {
			t.Fatalf("expected positive package ID, got %d", pkg.ID)
		}
		packageID = pkg.ID

		list, err := repo.ListPackages(ctx)
		if err != nil {
			t.Fatalf("ListPackages failed: %v", err)
		}
		if len(list) == 0 {
			t.Fatal("expected at least one package in list")
		}
	})

	t.Run("Create and Get Offer", func(t *testing.T) {
		now := time.Now().UTC()
		discVal, _ := money.Parse("15.00")
		minOrderVal, _ := money.Parse("100.00")

		o := &promo.Offer{
			OrganizationID: testOrgID,
			Title:          i18n.Text{"en": "15% Summer Sale"},
			Description:    i18n.Text{"en": "Seasonal discounts on select products"},
			DiscountType:   promo.DiscountFixed,
			DiscountValue:  discVal,
			MinOrderAmount: minOrderVal,
			StartsAt:       now.Add(-time.Hour),
			ExpiresAt:      now.Add(24 * time.Hour),
			IsActive:       true,
		}

		if err := repo.CreateOffer(ctx, o); err != nil {
			t.Fatalf("CreateOffer failed: %v", err)
		}
		if o.ID <= 0 {
			t.Fatalf("expected positive offer ID, got %d", o.ID)
		}
		offerID = o.ID

		got, err := repo.GetOfferByID(ctx, o.ID)
		if err != nil {
			t.Fatalf("GetOfferByID failed: %v", err)
		}
		if got.DiscountValue != discVal {
			t.Errorf("got discount %s, want %s", got.DiscountValue, discVal)
		}

		err = repo.IncrementOfferEngagement(ctx, o.ID, false)
		if err != nil {
			t.Fatalf("IncrementOfferEngagement view failed: %v", err)
		}
		err = repo.IncrementOfferEngagement(ctx, o.ID, true)
		if err != nil {
			t.Fatalf("IncrementOfferEngagement click failed: %v", err)
		}
	})

	t.Run("Create Sponsorship", func(t *testing.T) {
		now := time.Now().UTC()
		s := &promo.OfferSponsorship{
			OrganizationID: testOrgID,
			OfferID:        offerID,
			PackageID:      packageID,
			StartsAt:       now,
			ExpiresAt:      now.Add(30 * 24 * time.Hour),
			Status:         "active",
		}

		if err := repo.CreateSponsorship(ctx, s); err != nil {
			t.Fatalf("CreateSponsorship failed: %v", err)
		}
		if s.ID <= 0 {
			t.Fatalf("expected positive sponsorship ID, got %d", s.ID)
		}
	})

	t.Run("Create and List Highlight Sections", func(t *testing.T) {
		h := &promo.HighlightSection{
			Title:        i18n.Text{"en": "Trending Medicine"},
			Slug:         "test-curated",
			DisplayOrder: 1,
			IsActive:     true,
		}

		if err := repo.CreateHighlightSection(ctx, h); err != nil {
			t.Fatalf("CreateHighlightSection failed: %v", err)
		}
		if h.ID <= 0 {
			t.Fatalf("expected positive section ID, got %d", h.ID)
		}

		list, err := repo.ListHighlightSections(ctx)
		if err != nil {
			t.Fatalf("ListHighlightSections failed: %v", err)
		}
		if len(list) == 0 {
			t.Fatal("expected at least one highlight section")
		}
	})

	t.Run("ListActiveOffers", func(t *testing.T) {
		activeOffers, err := repo.ListActiveOffers(ctx, 10, 0)
		if err != nil {
			t.Fatalf("ListActiveOffers failed: %v", err)
		}
		if len(activeOffers) == 0 {
			t.Fatal("expected at least one active offer")
		}
	})

	t.Run("Ads_Operations", func(t *testing.T) {
		// Clean and insert test ad
		err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, `DELETE FROM promo.ad_clicks WHERE user_agent = 'TestAgent'`)
			_, _ = tx.Exec(txCtx, `DELETE FROM promo.ads WHERE title = 'Banner Ad Test'`)
			var adID int64
			err := tx.QueryRow(txCtx, `
				INSERT INTO promo.ads (organization_id, title, image_url, target_url, position, is_active, starts_at, expires_at)
				VALUES ($1, 'Banner Ad Test', 'https://cdn.example.com/ad.jpg', 'https://dawa24.test/sale', 'top_banner', true, now() - interval '1 hour', now() + interval '1 day')
				RETURNING id;
			`, testOrgID).Scan(&adID)
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			t.Fatalf("setup test ad failed: %v", err)
		}

		ads, err := repo.ListActiveAds(ctx, "top_banner")
		if err != nil {
			t.Fatalf("ListActiveAds failed: %v", err)
		}
		if len(ads) == 0 {
			t.Fatal("expected at least 1 active ad")
		}

		err = repo.RecordAdClick(ctx, ads[0].ID, nil, "127.0.0.1", "TestAgent")
		if err != nil {
			t.Fatalf("RecordAdClick failed: %v", err)
		}
	})

	t.Run("Expire Promotions", func(t *testing.T) {
		_, err := repo.ExpirePromotions(ctx)
		if err != nil {
			t.Fatalf("ExpirePromotions failed: %v", err)
		}
	})


	t.Run("CreateSpecialOffer_PercentageAndFixedDiscounts", func(t *testing.T) {
		// Test creating special offer with 15% discount (DiscountAmount is 0.00)
		start := time.Now()
		end := time.Now().Add(30 * 24 * time.Hour)
		sp := &promo.SpecialOffer{
			OrganizationID:     testOrgID,
			Title:              i18n.Text{"ar": "عرض خاص خصم 15%", "en": "Special Offer 15%"},
			Description:        i18n.Text{"ar": "وصف العرض", "en": "Offer Description"},
			DiscountPercentage: 15.0,
			DiscountAmount:     money.Zero, // 0.00 minor units
			TotalPrice:         money.FromMajor(100),
			MinOrderAmount:     money.FromMajor(50),
			StartDate:          &start,
			EndDate:            &end,
			Status:             "active",
			AdminStatus:        "approved",
		}

		err := repo.CreateSpecialOffer(ctx, sp)
		if err != nil {
			t.Fatalf("CreateSpecialOffer with percentage discount failed: %v", err)
		}
		if sp.ID <= 0 {
			t.Fatalf("expected positive offer ID, got %d", sp.ID)
		}

		loaded, err := repo.GetSpecialOfferByID(ctx, sp.ID)
		if err != nil {
			t.Fatalf("GetSpecialOfferByID failed: %v", err)
		}
		if loaded.DiscountPercentage != 15.0 {
			t.Errorf("expected 15.0 discount percentage, got %f", loaded.DiscountPercentage)
		}

		// Test creating special offer with fixed discount amount (e.g. 25 EGP)
		spFixed := &promo.SpecialOffer{
			OrganizationID:     testOrgID,
			Title:              i18n.Text{"ar": "عرض خاص خصم 25 ج.م", "en": "Special Offer 25 EGP"},
			DiscountPercentage: 0,
			DiscountAmount:     money.FromMajor(25),
			TotalPrice:         money.FromMajor(150),
			MinOrderAmount:     money.FromMajor(50),
			StartDate:          &start,
			EndDate:            &end,
			Status:             "active",
			AdminStatus:        "approved",
		}

		err = repo.CreateSpecialOffer(ctx, spFixed)
		if err != nil {
			t.Fatalf("CreateSpecialOffer with fixed discount failed: %v", err)
		}
		if spFixed.ID <= 0 {
			t.Fatalf("expected positive offer ID, got %d", spFixed.ID)
		}
	})
}
